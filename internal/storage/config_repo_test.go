package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConfigRepositoryScopesAndListOrder(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	projectID := "project-api"
	if err := store.DB().Create(&Project{
		ID:           projectID,
		WorkspaceID:  ws.ID,
		Slug:         "api",
		Name:         "API",
		Description:  "",
		Status:       "active",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	}).Error; err != nil {
		t.Fatalf("Create(project) error = %v", err)
	}

	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: projectID, Key: "zeta"}, "last"); err != nil {
		t.Fatalf("Set(project zeta) error = %v", err)
	}
	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: projectID, Key: "alpha"}, "first"); err != nil {
		t.Fatalf("Set(project alpha) error = %v", err)
	}
	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: projectID, Key: "middle"}, "middle"); err != nil {
		t.Fatalf("Set(project middle) error = %v", err)
	}

	var stored Config
	if err := store.DB().Where("scope = ? AND key = ?", string(ConfigScopeProject), "zeta").First(&stored).Error; err != nil {
		t.Fatalf("load raw config error = %v", err)
	}
	if stored.WorkspaceID != ws.ID || stored.ScopeID != projectID {
		t.Fatalf("stored project scope = (%q, %q), want (%q, %q)", stored.WorkspaceID, stored.ScopeID, ws.ID, projectID)
	}

	listed, err := repo.ListScope(ws.ID, ConfigScopeProject, projectID)
	if err != nil {
		t.Fatalf("ListScope(project) error = %v", err)
	}
	if got := keysOfStringMap(listed); strings.Join(got, ",") != "alpha,middle,zeta" {
		t.Fatalf("ListScope keys = %#v, want sorted alpha,middle,zeta", got)
	}
	if listed["alpha"] != "first" || listed["zeta"] != "last" {
		t.Fatalf("ListScope values = %#v", listed)
	}

	if err := repo.Unset(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: projectID, Key: "alpha"}); err != nil {
		t.Fatalf("Unset(project alpha) error = %v", err)
	}
	if _, ok, err := repo.Get(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: projectID, Key: "alpha"}); err != nil || ok {
		t.Fatalf("Get(after unset) = (_, %v, %v), want missing nil error", ok, err)
	}
}

func TestConfigTemplateCandidatePageScopesJoinsDefinitionAndFiltersMode(t *testing.T) {
	store := openIdentityTestStore(t)
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	repo := NewConfigRepository(store.DB())
	defs := NewConfigDefinitionRepository(store.DB())
	for i := 0; i < 105; i++ {
		key := fmt.Sprintf("deploy.key.%03d", i)
		if err := defs.Set(ConfigDefinition{WorkspaceID: ws.ID, Key: key, ValueType: "string", AllowedScopesJSON: `["project"]`, Label: "发布配置", Secret: i%2 == 0, CreatedAt: 1, ModifiedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: "source", Key: key}, "value"); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: "other", Key: "deploy.key.000"}, "other"); err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListCandidatePage(ConfigCandidateListOptions{WorkspaceID: ws.ID, ProjectID: "source", Q: "发布", Mode: "secret"}, 25, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 53 || len(page.Items) != 25 || page.Items[0].Config.Key != "deploy.key.020" {
		t.Fatalf("page = %#v", page)
	}
	for _, item := range page.Items {
		if item.Config.ScopeID != "source" || !item.Definition.Secret {
			t.Fatalf("config scope/mode leaked: %#v", item)
		}
	}
}

func TestConfigTemplateCandidatePageIncludesPromptOnlyDefinitionsWithoutValues(t *testing.T) {
	store := openIdentityTestStore(t)
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	repo := NewConfigRepository(store.DB())
	defs := NewConfigDefinitionRepository(store.DB())
	definitions := []ConfigDefinition{
		{WorkspaceID: ws.ID, Key: "a.project", ValueType: "string", AllowedScopesJSON: `["project"]`, Label: "项目值", CreatedAt: 1, ModifiedAt: 1},
		{WorkspaceID: ws.ID, Key: "b.workspace", ValueType: "string", AllowedScopesJSON: `["workspace","project"]`, Label: "继承值", CreatedAt: 1, ModifiedAt: 1},
		{WorkspaceID: ws.ID, Key: "c.default", ValueType: "string", AllowedScopesJSON: `["project"]`, Label: "默认值", DefaultValue: "fallback", HasDefault: true, CreatedAt: 1, ModifiedAt: 1},
		{WorkspaceID: ws.ID, Key: "d.missing_secret", ValueType: "string", AllowedScopesJSON: `["project"]`, Label: "待填写密钥", Secret: true, CreatedAt: 1, ModifiedAt: 1},
		{WorkspaceID: ws.ID, Key: "e.workspace_only", ValueType: "string", AllowedScopesJSON: `["workspace"]`, Label: "仅工作区", CreatedAt: 1, ModifiedAt: 1},
	}
	for _, definition := range definitions {
		if err := defs.Set(definition); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: "source", Key: "a.project"}, "project-value"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeWorkspace, Key: "b.workspace"}, "workspace-value"); err != nil {
		t.Fatal(err)
	}

	page, err := repo.ListCandidatePage(ConfigCandidateListOptions{WorkspaceID: ws.ID, ProjectID: "source", Mode: "prompt_available"}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || len(page.Items) != 4 {
		t.Fatalf("page = %#v", page)
	}
	wantSources := map[string]string{
		"a.project": "project", "b.workspace": "workspace", "c.default": "default", "d.missing_secret": "missing",
	}
	for _, item := range page.Items {
		if item.EffectiveSource != wantSources[item.Definition.Key] {
			t.Fatalf("candidate = %#v", item)
		}
		if item.HasProjectValue != (item.Definition.Key == "a.project") || item.CanFixed != item.HasProjectValue {
			t.Fatalf("fixed flags = %#v", item)
		}
		if item.Definition.Key != "a.project" && item.Config.Value != "" {
			t.Fatalf("candidate leaked effective value: %#v", item)
		}
	}

	fixed, err := repo.ListCandidatePage(ConfigCandidateListOptions{WorkspaceID: ws.ID, ProjectID: "source", Mode: "fixed_available"}, 20, 0)
	if err != nil || fixed.Total != 1 || fixed.Items[0].Definition.Key != "a.project" {
		t.Fatalf("fixed = %#v err=%v", fixed, err)
	}
	secret, err := repo.ListCandidatePage(ConfigCandidateListOptions{WorkspaceID: ws.ID, ProjectID: "source", Mode: "secret"}, 20, 0)
	if err != nil || secret.Total != 1 || secret.Items[0].Definition.Key != "d.missing_secret" || secret.Items[0].Config.Value != "" {
		t.Fatalf("secret = %#v err=%v", secret, err)
	}
}

func TestConfigRepositoryServerScopeUsesEmptyScopeAndUpserts(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigRepository(store.DB())

	key := ConfigKey{
		WorkspaceID: "ignored-workspace",
		Scope:       ConfigScopeServer,
		ScopeID:     "ignored-scope",
		Key:         "retention.days",
	}
	if err := repo.Set(key, "30"); err != nil {
		t.Fatalf("Set(server initial) error = %v", err)
	}
	if err := repo.Set(key, "60"); err != nil {
		t.Fatalf("Set(server update) error = %v", err)
	}

	var rows []Config
	if err := store.DB().Where("scope = ? AND key = ?", string(ConfigScopeServer), key.Key).Find(&rows).Error; err != nil {
		t.Fatalf("load raw server configs error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("server config rows = %d, want 1", len(rows))
	}
	if rows[0].WorkspaceID != "" || rows[0].ScopeID != "" || rows[0].Value != "60" {
		t.Fatalf("server config = %#v, want empty workspace/scope and updated value", rows[0])
	}
}

func TestConfigRepositoryRejectsEmptyWorkspaceForScopedConfig(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigRepository(store.DB())

	if err := repo.Set(ConfigKey{Scope: ConfigScopeProject, ScopeID: "project-api", Key: "x"}, "y"); err == nil {
		t.Fatal("Set(project without workspace) error = nil, want error")
	}
	if _, _, err := repo.Get(ConfigKey{Scope: ConfigScopeWorkspace, ScopeID: "ws-local", Key: "x"}); err == nil {
		t.Fatal("Get(workspace without workspace) error = nil, want error")
	}
	if err := repo.Unset(ConfigKey{Scope: ConfigScopeUser, ScopeID: "user-local", Key: "x"}); err == nil {
		t.Fatal("Unset(user without workspace) error = nil, want error")
	}
}

func TestConfigRepositoryRejectsInvalidScopeWithSentinel(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigRepository(store.DB())

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "set project without workspace",
			run: func() error {
				return repo.Set(ConfigKey{Scope: ConfigScopeProject, ScopeID: "project-api", Key: "x"}, "y")
			},
		},
		{
			name: "get workspace without workspace",
			run: func() error {
				_, _, err := repo.Get(ConfigKey{Scope: ConfigScopeWorkspace, Key: "x"})
				return err
			},
		},
		{
			name: "unset user without explicit scope id",
			run: func() error {
				return repo.Unset(ConfigKey{WorkspaceID: "ws-local", Scope: ConfigScopeUser, Key: "x"})
			},
		},
		{
			name: "list project without scope id",
			run: func() error {
				_, err := repo.ListScope("ws-local", ConfigScopeProject, "")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrInvalidConfigScope) {
				t.Fatalf("error = %v, want errors.Is ErrInvalidConfigScope", err)
			}
		})
	}
}

func TestConfigRepositoryNormalizesWorkspaceScopeID(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}

	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeWorkspace, ScopeID: "ignored", Key: "theme"}, "dark"); err != nil {
		t.Fatalf("Set(workspace) error = %v", err)
	}

	var stored Config
	if err := store.DB().Where("scope = ? AND key = ?", string(ConfigScopeWorkspace), "theme").First(&stored).Error; err != nil {
		t.Fatalf("load raw config error = %v", err)
	}
	if stored.ScopeID != ws.ID {
		t.Fatalf("workspace ScopeID = %q, want normalized workspace ID %q", stored.ScopeID, ws.ID)
	}
}

func TestConfigSchemaRejectsNullScopeColumns(t *testing.T) {
	store := openIdentityTestStore(t)

	assertRawDDLContainsNormalized(t, store, "configs", "PRIMARY KEY(workspace_id, scope, scope_id, key)")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES(NULL, 'server', '', 'x', 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', NULL, '', 'x', 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', 'server', NULL, 'x', 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', 'server', '', NULL, 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', 'server', '', 'x', NULL)")
}

func TestConfigRepositoryListScopeOrdersByKey(t *testing.T) {
	store := openIdentityTestStore(t)
	capture := &captureSQLLogger{Interface: logger.Default.LogMode(logger.Info)}
	session := gormSessionWithLogger(capture)
	repo := NewConfigRepository(store.DB().Session(&session))

	if _, err := repo.ListScope("ws-local", ConfigScopeProject, "project-api"); err != nil {
		t.Fatalf("ListScope() error = %v", err)
	}
	if got := normalizeSQL(capture.lastSQL); !strings.Contains(got, "order by key asc") {
		t.Fatalf("ListScope SQL = %q, want ORDER BY key ASC", capture.lastSQL)
	}
}

func TestConfigDefinitionRepositoryCRUD(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigDefinitionRepository(store.DB())

	def := ConfigDefinition{
		WorkspaceID:       "ws-a",
		Key:               "ads.roi_threshold",
		ValueType:         "number",
		AllowedScopesJSON: `["workspace","project"]`,
		Label:             "ROI Threshold",
		Description:       "minimum acceptable ROI",
		EnumValuesJSON:    "[]",
		DefaultValue:      "1.8",
		HasDefault:        true,
		Required:          false,
		Secret:            false,
		ShowOnConsoleHome: true,
		CreatedAt:         100,
		ModifiedAt:        100,
	}

	if err := repo.Set(def); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, ok, err := repo.Get("ws-a", "ads.roi_threshold")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() missing definition, want found")
	}
	if got.ValueType != "number" || got.DefaultValue != "1.8" || !got.HasDefault {
		t.Fatalf("Get() = %#v", got)
	}
	if !got.ShowOnConsoleHome {
		t.Fatal("ShowOnConsoleHome = false, want true")
	}

	listed, err := repo.List("ws-a")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].Key != "ads.roi_threshold" {
		t.Fatalf("List() = %#v, want single ads.roi_threshold", listed)
	}

	def.Label = "Updated"
	def.ModifiedAt = 200
	if err := repo.Set(def); err != nil {
		t.Fatalf("Set(update) error = %v", err)
	}
	got, ok, err = repo.Get("ws-a", "ads.roi_threshold")
	if err != nil || !ok {
		t.Fatalf("Get(after update) = %#v, %v, %v", got, ok, err)
	}
	if got.Label != "Updated" || got.ModifiedAt != 200 {
		t.Fatalf("updated definition = %#v", got)
	}

	if err := repo.Delete("ws-a", "ads.roi_threshold"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := repo.Get("ws-a", "ads.roi_threshold"); err != nil || ok {
		t.Fatalf("Get(after delete) = (_, %v, %v), want missing nil error", ok, err)
	}
}

func TestConfigRepositoryUsageAndValuesByKey(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	projectID := "project-api"
	if err := store.DB().Create(&Project{
		ID:           projectID,
		WorkspaceID:  ws.ID,
		Slug:         "api",
		Name:         "API",
		Status:       "active",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	}).Error; err != nil {
		t.Fatalf("Create(project) error = %v", err)
	}

	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeWorkspace, Key: "ads.budget"}, "100"); err != nil {
		t.Fatalf("Set(workspace) error = %v", err)
	}
	if err := repo.Set(ConfigKey{WorkspaceID: ws.ID, Scope: ConfigScopeProject, ScopeID: projectID, Key: "ads.budget"}, "200"); err != nil {
		t.Fatalf("Set(project) error = %v", err)
	}

	workspaceCount, projectCount, err := repo.CountByKey(ws.ID, "ads.budget")
	if err != nil {
		t.Fatalf("CountByKey() error = %v", err)
	}
	if workspaceCount != 1 || projectCount != 1 {
		t.Fatalf("CountByKey() = (%d, %d), want (1, 1)", workspaceCount, projectCount)
	}

	values, err := repo.ValuesByKey(ws.ID, "ads.budget")
	if err != nil {
		t.Fatalf("ValuesByKey() error = %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("ValuesByKey() = %#v, want 2 values", values)
	}
	gotValues := map[string]bool{values[0]: true, values[1]: true}
	if !gotValues["100"] || !gotValues["200"] {
		t.Fatalf("ValuesByKey() = %#v, want contain 100 and 200", values)
	}

	// 按 scope 拆分查询
	wsValues, err := repo.ValuesByKeyAndScope(ws.ID, "ads.budget", ConfigScopeWorkspace)
	if err != nil {
		t.Fatalf("ValuesByKeyAndScope(workspace) error = %v", err)
	}
	if len(wsValues) != 1 || wsValues[0] != "100" {
		t.Fatalf("ValuesByKeyAndScope(workspace) = %#v, want [100]", wsValues)
	}
	projectValues, err := repo.ValuesByKeyAndScope(ws.ID, "ads.budget", ConfigScopeProject)
	if err != nil {
		t.Fatalf("ValuesByKeyAndScope(project) error = %v", err)
	}
	if len(projectValues) != 1 || projectValues[0] != "200" {
		t.Fatalf("ValuesByKeyAndScope(project) = %#v, want [200]", projectValues)
	}
}

func TestConfigDefinitionRepositoryWorkspaceIsolationAndUniqueness(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewConfigDefinitionRepository(store.DB())

	base := ConfigDefinition{
		Key:               "ads.account_id",
		ValueType:         "string",
		AllowedScopesJSON: `["project"]`,
		EnumValuesJSON:    "[]",
		CreatedAt:         100,
		ModifiedAt:        100,
	}

	defA := base
	defA.WorkspaceID = "ws-a"
	if err := repo.Set(defA); err != nil {
		t.Fatalf("Set(ws-a) error = %v", err)
	}

	defB := base
	defB.WorkspaceID = "ws-b"
	if err := repo.Set(defB); err != nil {
		t.Fatalf("Set(ws-b) error = %v", err)
	}

	listA, err := repo.List("ws-a")
	if err != nil {
		t.Fatalf("List(ws-a) error = %v", err)
	}
	if len(listA) != 1 || listA[0].WorkspaceID != "ws-a" {
		t.Fatalf("List(ws-a) = %#v", listA)
	}

	listB, err := repo.List("ws-b")
	if err != nil {
		t.Fatalf("List(ws-b) error = %v", err)
	}
	if len(listB) != 1 || listB[0].WorkspaceID != "ws-b" {
		t.Fatalf("List(ws-b) = %#v", listB)
	}
}

func assertRawDDLContainsNormalized(t *testing.T, store *Store, table, want string) {
	t.Helper()
	var ddl string
	if err := store.DB().Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&ddl).Error; err != nil {
		t.Fatalf("read DDL for %s error = %v", table, err)
	}
	normalize := func(s string) string {
		s = strings.ReplaceAll(s, "`", "")
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.Join(strings.Fields(s), " ")
		s = strings.ReplaceAll(s, " (", "(")
		s = strings.ReplaceAll(s, "( ", "(")
		s = strings.ReplaceAll(s, " )", ")")
		s = strings.ReplaceAll(s, ", ", ",")
		return s
	}
	if !strings.Contains(normalize(ddl), normalize(want)) {
		t.Fatalf("%s DDL = %s, want contains %s", table, ddl, want)
	}
}

func keysOfStringMap(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type captureSQLLogger struct {
	logger.Interface
	lastSQL string
}

func (l *captureSQLLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.lastSQL = sql
}

func normalizeSQL(sql string) string {
	sql = strings.ToLower(sql)
	sql = strings.ReplaceAll(sql, "`", "")
	sql = strings.Join(strings.Fields(sql), " ")
	return sql
}

func gormSessionWithLogger(log logger.Interface) gorm.Session {
	return gorm.Session{Logger: log}
}
