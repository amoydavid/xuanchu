package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type projectTemplateFixture struct {
	store *storage.Store
	owner *Service
}

func newProjectTemplateFixture(t *testing.T) projectTemplateFixture {
	t.Helper()
	store := newTestStore(t)
	owner, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return projectTemplateFixture{store: store, owner: owner}
}

func projectTemplateSnapshotFixture() projecttemplate.SnapshotV1 {
	configValue := "https://api.example.test/v1"
	return projecttemplate.SnapshotV1{
		Schema:     projecttemplate.SnapshotSchemaV1,
		AnchorDate: "2026-07-20",
		Project:    projecttemplate.ProjectBlueprintV1{Description: "项目说明"},
		Configs: []projecttemplate.ConfigBlueprintV1{
			{Key: "agent.base_url", Mode: "literal", Value: &configValue},
			{Key: "agent.api_key", Mode: "secret_input"},
		},
		Tasks:  []projecttemplate.TaskBlueprintV1{{Ref: "task-1", Title: "准备发布"}},
		Series: []projecttemplate.SeriesBlueprintV1{},
		Automations: []projecttemplate.AutomationBlueprintV1{{
			Ref: "automation-1", Name: "发布巡检", TriggerType: "schedule",
			Action:  projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "agent.base_url", APIKeyConfigKey: "agent.api_key", ModelConfigKey: "agent.model"},
			Context: projecttemplate.AutomationContextV1{Include: []string{"project"}}, InstructionTemplate: "检查发布状态",
		}},
	}
}

func projectTemplateSnapshotWithOnly(component string, assigneeID string) projecttemplate.SnapshotV1 {
	snapshot := projectTemplateSnapshotFixture()
	automation := snapshot.Automations[0]
	snapshot.Tasks = nil
	snapshot.Series = nil
	snapshot.Configs = nil
	snapshot.Automations = nil
	switch component {
	case "task":
		snapshot.Tasks = []projecttemplate.TaskBlueprintV1{{Ref: "task-1", Title: "仅详情任务标题", AssigneeIDs: []string{assigneeID}}}
	case "config":
		literal := "仅详情可见的 literal 配置值"
		snapshot.Configs = []projecttemplate.ConfigBlueprintV1{{Key: "task.only.config", Mode: "literal", Value: &literal}}
	case "automation":
		automation.Name = "仅详情自动化"
		automation.InstructionTemplate = "仅详情自动化指令"
		snapshot.Automations = []projecttemplate.AutomationBlueprintV1{automation}
	default:
		panic("unknown template component: " + component)
	}
	return snapshot
}

func seedProjectTemplate(t *testing.T, svc *Service, key string, snapshot projecttemplate.SnapshotV1) storage.ProjectTemplate {
	t.Helper()
	raw, hash, err := projecttemplate.EncodeV1(snapshot, projecttemplate.DefaultLimits)
	if err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}
	now := svc.Clock().Unix()
	userID := svc.Runtime().ActorUserID
	template := storage.ProjectTemplate{
		ID: uuid.NewString(), WorkspaceID: svc.Runtime().WorkspaceID, Key: key, Name: "发布模板", Description: "发布流程", Status: "active",
		CreatedByActorType: actorTypeUser, CreatedByUserID: &userID, CreatedAt: now, ModifiedAt: now,
	}
	repo := storage.NewProjectTemplateRepository(svc.store.DB())
	if err := repo.Create(template); err != nil {
		t.Fatalf("create template: %v", err)
	}
	source, err := svc.ResolveProject("src")
	sourceID := source.ID
	if err != nil {
		created, err := svc.AddProject(AddProjectInput{Slug: "src", Name: "模板来源"})
		if err != nil {
			t.Fatalf("create source project: %v", err)
		}
		sourceID = created.ID
	}
	if err := svc.store.Transaction(func(txStore *storage.Store) error {
		_, err := storage.NewProjectTemplateRepository(txStore.DB()).AppendSnapshotLocked(template.WorkspaceID, template.ID, storage.ProjectTemplateSnapshot{
			ID: uuid.NewString(), SourceProjectID: sourceID, SnapshotJSON: string(raw), SnapshotHash: hash,
			CreatedByActorType: actorTypeUser, CreatedByUserID: &userID, CreatedAt: now,
		})
		return err
	}); err != nil {
		t.Fatalf("append snapshot: %v", err)
	}
	row, err := repo.GetByRef(template.WorkspaceID, template.ID)
	if err != nil {
		t.Fatalf("get template: %v", err)
	}
	return row
}

func runtimeCode(err error) string {
	var runtimeErr RuntimeError
	if errors.As(err, &runtimeErr) {
		return runtimeErr.Code
	}
	var permissionErr PermissionError
	if errors.As(err, &permissionErr) {
		return permissionErr.Code
	}
	return ""
}

func TestProjectTemplateMetadataLifecycleAndRedaction(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())

	page, err := f.owner.ListProjectTemplates("all", "launch", 50, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListProjectTemplates = %#v, %v", page, err)
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "snapshot_json") {
		t.Fatalf("metadata leaked raw snapshot: %s", raw)
	}
	if page.Items[0].CreatedBy.User == nil || page.Items[0].CreatedBy.User.ID != f.owner.Runtime().ActorUserID {
		t.Fatalf("created_by = %#v", page.Items[0].CreatedBy)
	}

	archived, err := f.owner.ArchiveProjectTemplate(seeded.Key)
	if err != nil || archived.Template.Status != "archived" || archived.Template.ArchivedAt == nil {
		t.Fatalf("ArchiveProjectTemplate = %#v, %v", archived, err)
	}
	reactivated, err := f.owner.ReactivateProjectTemplate(seeded.ID)
	if err != nil || reactivated.Template.Status != "active" || reactivated.Template.ArchivedAt != nil {
		t.Fatalf("ReactivateProjectTemplate = %#v, %v", reactivated, err)
	}
}

func TestProjectTemplatePermissionScopeAndInstantiationList(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())

	viewerRuntime := f.owner.Runtime()
	viewerRuntime.Role = RoleViewer
	viewer, err := NewService(ServiceOptions{Store: f.store, Clock: FixedClock{NowUnix: 100}, Runtime: &viewerRuntime, DisableScopeBootstrap: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := viewer.ListProjectTemplates("active", "", 50, 0); err != nil {
		t.Fatalf("viewer list: %v", err)
	}
	if _, err := viewer.ArchiveProjectTemplate(seeded.Key); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("viewer archive error = %v", err)
	}
	if _, err := viewer.ProjectTemplateInfo(seeded.Key, nil); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("viewer detail error = %v", err)
	}

	instantiation, err := f.owner.ListProjectTemplatesForInstantiation(TemplateInstantiationListInput{Limit: 50})
	if err != nil || len(instantiation.Items) != 1 {
		t.Fatalf("ListProjectTemplatesForInstantiation = %#v, %v", instantiation, err)
	}
	current := instantiation.Items[0].CurrentSnapshot
	if current == nil || current.ID == "" || current.Version != 1 || current.Hash == "" || len(current.RequiredSecretKeys) != 1 || current.RequiredSecretKeys[0] != "agent.api_key" {
		t.Fatalf("instantiation current snapshot = %#v", current)
	}
	if strings.Contains(string(mustJSON(instantiation)), "snapshot_json") {
		t.Fatalf("instantiation list leaked snapshot: %s", mustJSON(instantiation))
	}

	projectScope := &RequestScope{WorkspaceIDs: []string{f.owner.Runtime().WorkspaceID}, ProjectIDs: []string{"only-project"}, Capabilities: []string{auth.ScopeProjectRead}}
	scopedRuntime := f.owner.Runtime()
	scoped, err := NewService(ServiceOptions{Store: f.store, Clock: FixedClock{NowUnix: 100}, Runtime: &scopedRuntime, RequestScope: projectScope, DisableScopeBootstrap: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.ListProjectTemplates("active", "", 50, 0); runtimeCode(err) != authz.CodeProjectScopeDenied {
		t.Fatalf("project-scoped list error = %v", err)
	}
}

func TestProjectTemplateProjectScopedTokenRejectsEveryPublicMetadataUseCase(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())
	projectScope := &RequestScope{
		WorkspaceIDs: []string{f.owner.Runtime().WorkspaceID},
		ProjectIDs:   []string{"only-project"},
		Capabilities: []string{
			auth.ScopeProjectRead,
			auth.ScopeProjectWrite,
			auth.ScopeTaskRead,
			auth.ScopeConfigRead,
			auth.ScopeHookRead,
		},
	}
	runtime := f.owner.Runtime()
	scoped, err := NewService(ServiceOptions{Store: f.store, Clock: FixedClock{NowUnix: 100}, Runtime: &runtime, RequestScope: projectScope, DisableScopeBootstrap: true})
	if err != nil {
		t.Fatal(err)
	}
	name := "不应修改"
	for _, test := range []struct {
		name string
		call func() error
	}{
		{"list", func() error { _, err := scoped.ListProjectTemplates("active", "", 50, 0); return err }},
		{"instantiation_list", func() error {
			_, err := scoped.ListProjectTemplatesForInstantiation(TemplateInstantiationListInput{Limit: 50})
			return err
		}},
		{"info", func() error { _, err := scoped.ProjectTemplateInfo(seeded.Key, nil); return err }},
		{"modify", func() error {
			_, err := scoped.ModifyProjectTemplate(seeded.Key, ModifyTemplateInput{Name: &name})
			return err
		}},
		{"archive", func() error { _, err := scoped.ArchiveProjectTemplate(seeded.Key); return err }},
		{"reactivate", func() error { _, err := scoped.ReactivateProjectTemplate(seeded.Key); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); runtimeCode(err) != authz.CodeProjectScopeDenied {
				t.Fatalf("error code = %q, err = %v", runtimeCode(err), err)
			}
		})
	}
}

func TestProjectTemplateModifyAndLifecycleAudit(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())
	name, description := "新版发布模板", "新的说明"
	modified, err := f.owner.ModifyProjectTemplate(seeded.Key, ModifyTemplateInput{Name: &name, Description: &description})
	if err != nil || modified.Template.Name != name || modified.Template.Description != description {
		t.Fatalf("ModifyProjectTemplate = %#v, %v", modified, err)
	}
	if _, err := f.owner.ArchiveProjectTemplate(seeded.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.ReactivateProjectTemplate(seeded.Key); err != nil {
		t.Fatal(err)
	}

	rows, err := storage.NewAuditRepository(f.store.DB()).List(storage.AuditListOptions{WorkspaceID: &seeded.WorkspaceID, TargetID: &seeded.ID, Limit: 10})
	if err != nil || len(rows) != 3 {
		t.Fatalf("lifecycle audit rows = %#v, %v", rows, err)
	}
	actions := map[string]int{}
	var foundModify, foundArchive, foundReactivate bool
	for _, row := range rows {
		actions[row.Action]++
		if strings.Contains(row.PayloadJSON, "snapshot_json") || strings.Contains(row.PayloadJSON, "agent.api_key") {
			t.Fatalf("audit leaked snapshot or secret: %s", row.PayloadJSON)
		}
		switch {
		case row.Action == "project_template.modify" && strings.Contains(row.PayloadJSON, `"name_after":"新版发布模板"`):
			foundModify = true
		case row.Action == "project_template.archive" && strings.Contains(row.PayloadJSON, `"status_before":"active"`) && strings.Contains(row.PayloadJSON, `"status_after":"archived"`):
			foundArchive = true
		case row.Action == "project_template.modify" && strings.Contains(row.PayloadJSON, `"status_before":"archived"`) && strings.Contains(row.PayloadJSON, `"status_after":"active"`):
			foundReactivate = true
		}
	}
	if actions["project_template.modify"] != 2 || actions["project_template.archive"] != 1 || !foundModify || !foundArchive || !foundReactivate {
		t.Fatalf("unexpected lifecycle audit actions=%#v rows=%#v", actions, rows)
	}
}

func TestProjectTemplateDetailRequiresComponentPermissionsAndStaysInWorkspace(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())

	tenantRead := newScopedTokenService(t, f.store, []string{auth.ScopeProjectRead}, auth.TokenTypeTenantAccess)
	if _, err := tenantRead.ListProjectTemplates("active", "", 50, 0); err != nil {
		t.Fatalf("tenant metadata list: %v", err)
	}
	if _, err := tenantRead.ProjectTemplateInfo(seeded.Key, nil); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("tenant detail without task/config/hook read = %v", err)
	}
	tenantDetail := newScopedTokenService(t, f.store, []string{auth.ScopeProjectRead, auth.ScopeTaskRead, auth.ScopeConfigRead, auth.ScopeHookRead}, auth.TokenTypeTenantAccess)
	detail, err := tenantDetail.ProjectTemplateInfo(seeded.ID, nil)
	if err != nil || detail.Snapshot == nil || len(detail.Snapshot.Configs) != 2 {
		t.Fatalf("tenant detail = %#v, %v", detail, err)
	}
	for _, config := range detail.Snapshot.Configs {
		if config.Key == "agent.api_key" && config.Value != nil {
			t.Fatalf("secret config value leaked: %#v", config)
		}
	}
	if strings.Contains(mustJSON(detail), "snapshot_json") {
		t.Fatalf("detail leaked raw snapshot: %s", mustJSON(detail))
	}

	otherWorkspace := mustCreateWorkspaceRecord(t, f.store, storage.Workspace{ID: "ws-other", Slug: "other", Name: "Other", CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, f.store, storage.Membership{UserID: f.owner.Runtime().ActorUserID, WorkspaceID: otherWorkspace.ID, Role: string(RoleOwner), JoinedAt: 100, ModifiedAt: 100})
	other := newTestServiceWithRuntime(t, f.store, 100, "local", otherWorkspace.Slug)
	otherTemplate := seedProjectTemplate(t, other, "other-launch", projectTemplateSnapshotFixture())
	if _, err := f.owner.ProjectTemplateInfo(otherTemplate.ID, nil); runtimeCode(err) != "project_template_not_found" {
		t.Fatalf("cross-workspace detail error = %v", err)
	}
}

func TestProjectTemplateDetailRequiresOnlyPermissionsForPresentComponents(t *testing.T) {
	for _, test := range []struct {
		name       string
		component  string
		capability string
	}{
		{"task_only", "task", auth.ScopeTaskRead},
		{"config_only", "config", auth.ScopeConfigRead},
		{"automation_only", "automation", auth.ScopeHookRead},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProjectTemplateFixture(t)
			seeded := seedProjectTemplate(t, f.owner, test.component, projectTemplateSnapshotWithOnly(test.component, f.owner.Runtime().ActorUserID))
			base := []string{auth.ScopeProjectRead}
			if _, err := newScopedTokenService(t, f.store, base, auth.TokenTypeTenantAccess).ProjectTemplateInfo(seeded.Key, nil); runtimeCode(err) != authz.CodePermissionDenied {
				t.Fatalf("missing %s error = %v", test.capability, err)
			}
			detail, err := newScopedTokenService(t, f.store, append(base, test.capability), auth.TokenTypeTenantAccess).ProjectTemplateInfo(seeded.Key, nil)
			if err != nil || detail.Snapshot == nil {
				t.Fatalf("detail with only %s = %#v, %v", test.capability, detail, err)
			}
		})
	}
}

func TestProjectTemplateDetailSeriesOnlyRequiresTaskRead(t *testing.T) {
	f := newProjectTemplateFixture(t)
	snapshot := projectTemplateSnapshotFixture()
	snapshot.Tasks = nil
	snapshot.Configs = nil
	snapshot.Automations = nil
	snapshot.Series = []projecttemplate.SeriesBlueprintV1{{
		Ref: "series-1", Title: "每日巡检", RecurrenceRule: "daily",
		FirstDue: projecttemplate.RelativeLocalTimeV1{LocalTime: "09:00:00"},
	}}
	seeded := seedProjectTemplate(t, f.owner, "series-only", snapshot)

	base := []string{auth.ScopeProjectRead}
	if _, err := newScopedTokenService(t, f.store, base, auth.TokenTypeTenantAccess).ProjectTemplateInfo(seeded.Key, nil); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("series-only detail without task:read error = %v", err)
	}
	detail, err := newScopedTokenService(t, f.store, append(base, auth.ScopeTaskRead), auth.TokenTypeTenantAccess).ProjectTemplateInfo(seeded.Key, nil)
	if err != nil || detail.Snapshot == nil || len(detail.Snapshot.Tasks) != 0 || len(detail.Snapshot.Series) != 1 {
		t.Fatalf("series-only detail with task:read = %#v, %v", detail, err)
	}
}

func TestProjectTemplateMetadataAndDetailDoNotLeakAcrossWorkspaces(t *testing.T) {
	f := newProjectTemplateFixture(t)
	local := seedProjectTemplate(t, f.owner, "local", projectTemplateSnapshotFixture())
	otherWorkspace := mustCreateWorkspaceRecord(t, f.store, storage.Workspace{ID: "ws-template-other", Slug: "template-other", Name: "Other", CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, f.store, storage.Membership{UserID: f.owner.Runtime().ActorUserID, WorkspaceID: otherWorkspace.ID, Role: string(RoleOwner), JoinedAt: 100, ModifiedAt: 100})
	other := newTestServiceWithRuntime(t, f.store, 100, "local", otherWorkspace.Slug)
	foreign := seedProjectTemplate(t, other, "foreign", projectTemplateSnapshotWithOnly("task", other.Runtime().ActorUserID))

	page, err := f.owner.ListProjectTemplates("all", "", 50, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != local.ID || strings.Contains(mustJSON(page), foreign.ID) {
		t.Fatalf("local metadata page = %#v, %v", page, err)
	}
	for _, test := range []struct {
		name string
		call func() error
	}{
		{"info", func() error { _, err := f.owner.ProjectTemplateInfo(foreign.ID, nil); return err }},
		{"modify", func() error {
			name := "leak"
			_, err := f.owner.ModifyProjectTemplate(foreign.ID, ModifyTemplateInput{Name: &name})
			return err
		}},
		{"archive", func() error { _, err := f.owner.ArchiveProjectTemplate(foreign.ID); return err }},
		{"reactivate", func() error { _, err := f.owner.ReactivateProjectTemplate(foreign.ID); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); runtimeCode(err) != "project_template_not_found" {
				t.Fatalf("error code = %q, err = %v", runtimeCode(err), err)
			}
		})
	}
}

func TestProjectTemplateViewsRedactSnapshotAndResolveIdentityShapes(t *testing.T) {
	f := newProjectTemplateFixture(t)
	snapshot := projectTemplateSnapshotFixture()
	snapshot.Tasks[0].Title = "仅详情任务标题"
	snapshot.Tasks[0].AssigneeIDs = []string{f.owner.Runtime().ActorUserID}
	snapshot.Automations[0].InstructionTemplate = "仅详情自动化指令"
	seeded := seedProjectTemplate(t, f.owner, "identity", snapshot)

	page, err := f.owner.ListProjectTemplates("all", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	listJSON := mustJSON(page)
	for _, leaked := range []string{"仅详情任务标题", "仅详情自动化指令", "https://api.example.test/v1", "snapshot_json"} {
		if strings.Contains(listJSON, leaked) {
			t.Fatalf("metadata list leaked %q: %s", leaked, listJSON)
		}
	}
	detail, err := f.owner.ProjectTemplateInfo(seeded.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	detailJSON := mustJSON(detail)
	if strings.Contains(detailJSON, "snapshot_json") {
		t.Fatalf("detail leaked secret or raw snapshot: %s", detailJSON)
	}
	for _, config := range detail.Snapshot.Configs {
		if config.Key == "agent.api_key" && config.Value != nil {
			t.Fatalf("detail exposed secret config value: %#v", config)
		}
	}
	if detail.Template.CreatedBy.Type != actorTypeUser || detail.Template.CreatedBy.User == nil || detail.Template.CreatedBy.User.ID == "" || detail.Template.CreatedBy.User.Name == "" {
		t.Fatalf("created_by user actor = %#v", detail.Template.CreatedBy)
	}
	assignees := detail.Snapshot.Tasks[0].Assignees
	if len(assignees) != 1 || assignees[0].ID != detail.Template.CreatedBy.User.ID || assignees[0].Name != detail.Template.CreatedBy.User.Name {
		t.Fatalf("resolved assignees = %#v, actor = %#v", assignees, detail.Template.CreatedBy)
	}
}

func TestProjectTemplateDetailResolvesFullUserInfoForCreatorsAndAssignees(t *testing.T) {
	f := newProjectTemplateFixture(t)
	email := "template-owner@example.test"
	actor := mustCreateUserRecord(t, f.store, storage.User{
		ID: "template-full-user", Name: "template-owner", DisplayName: "模板负责人", Email: &email, CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, f.store, storage.Membership{UserID: actor.ID, WorkspaceID: f.owner.Runtime().WorkspaceID, Role: string(RoleOwner), JoinedAt: 100, ModifiedAt: 100})
	owner := newTestServiceWithRuntime(t, f.store, 100, actor.ID, f.owner.Runtime().WorkspaceSlug)
	if err := owner.BindExternalID(actor.ID, "feishu", "open_id", "ou_template_owner"); err != nil {
		t.Fatalf("BindExternalID: %v", err)
	}

	snapshot := projectTemplateSnapshotFixture()
	snapshot.Tasks[0].AssigneeIDs = []string{actor.ID}
	snapshot.Series = []projecttemplate.SeriesBlueprintV1{{
		Ref: "series-1", Title: "每日巡检", AssigneeIDs: []string{actor.ID}, RecurrenceRule: "daily",
		FirstDue: projecttemplate.RelativeLocalTimeV1{LocalTime: "09:00:00"},
	}}
	seeded := seedProjectTemplate(t, owner, "full-user-info", snapshot)

	detail, err := owner.ProjectTemplateInfo(seeded.Key, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertProjectTemplateUserInfo(t, detail.Template.CreatedBy.User, actor.ID, "template-owner", "模板负责人", email, "feishu", "open_id", "ou_template_owner")
	if detail.Template.CurrentSnapshot == nil {
		t.Fatal("current snapshot is nil")
	}
	assertProjectTemplateUserInfo(t, detail.Template.CurrentSnapshot.CreatedBy.User, actor.ID, "template-owner", "模板负责人", email, "feishu", "open_id", "ou_template_owner")
	if len(detail.Versions) != 1 {
		t.Fatalf("versions = %#v", detail.Versions)
	}
	assertProjectTemplateUserInfo(t, detail.Versions[0].CreatedBy.User, actor.ID, "template-owner", "模板负责人", email, "feishu", "open_id", "ou_template_owner")
	if len(detail.Snapshot.Tasks) != 1 || len(detail.Snapshot.Series) != 1 {
		t.Fatalf("snapshot = %#v", detail.Snapshot)
	}
	assertProjectTemplateUserInfo(t, &detail.Snapshot.Tasks[0].Assignees[0], actor.ID, "template-owner", "模板负责人", email, "feishu", "open_id", "ou_template_owner")
	assertProjectTemplateUserInfo(t, &detail.Snapshot.Series[0].Assignees[0], actor.ID, "template-owner", "模板负责人", email, "feishu", "open_id", "ou_template_owner")

	tests := []struct {
		name  string
		wire  any
		actor bool
	}{
		{"project template created_by", task.ActorInfoToJSON(detail.Template.CreatedBy), true},
		{"current snapshot created_by", task.ActorInfoToJSON(detail.Template.CurrentSnapshot.CreatedBy), true},
		{"task assignee", task.UserInfoToJSON(detail.Snapshot.Tasks[0].Assignees[0]), false},
		{"series assignee", task.UserInfoToJSON(detail.Snapshot.Series[0].Assignees[0]), false},
	}
	for index, version := range detail.Versions {
		tests = append(tests, struct {
			name  string
			wire  any
			actor bool
		}{fmt.Sprintf("version %d created_by", index+1), task.ActorInfoToJSON(version.CreatedBy), true})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertProjectTemplateWireUserInfo(t, test.wire, test.actor)
		})
	}
}

func assertProjectTemplateUserInfo(t *testing.T, info *task.UserInfo, id, name, displayName, email, provider, userType, externalID string) {
	t.Helper()
	if info == nil || info.ID != id || info.Name != name || info.DisplayName != displayName || info.Email == nil || *info.Email != email || len(info.ExternalIDs) != 1 || info.ExternalIDs[0].Provider != provider || info.ExternalIDs[0].UserType != userType || info.ExternalIDs[0].ExternalID != externalID {
		t.Fatalf("full user info = %#v", info)
	}
}

func assertProjectTemplateWireUserInfo(t *testing.T, wire any, actor bool) {
	t.Helper()
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal wire user info: %v", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("unmarshal wire user info: %v", err)
	}
	if actor {
		if len(object) != 2 {
			t.Fatalf("actor wire top-level keys = %#v, want only type and user: %s", sortedProjectTemplateWireKeys(object), encoded)
		}
		for key := range object {
			if key != "type" && key != "user" {
				t.Fatalf("actor wire top-level key %q, want only type and user: %s", key, encoded)
			}
		}
		if got := string(object["type"]); got != `"user"` {
			t.Fatalf("actor wire type = %s, wire = %s", got, encoded)
		}
		if _, ok := object["token"]; ok {
			t.Fatalf("user actor wire unexpectedly contains token: %s", encoded)
		}
		user, ok := object["user"]
		if !ok {
			t.Fatalf("actor wire missing user: %s", encoded)
		}
		if err := json.Unmarshal(user, &object); err != nil {
			t.Fatalf("unmarshal actor user: %v", err)
		}
	}

	if got := string(object["id"]); got != `"template-full-user"` {
		t.Fatalf("wire user id = %s, wire = %s", got, encoded)
	}
	if got := string(object["name"]); got != `"template-owner"` {
		t.Fatalf("wire user name = %s, wire = %s", got, encoded)
	}
	if got := string(object["display_name"]); got != `"模板负责人"` {
		t.Fatalf("wire user display_name = %s, wire = %s", got, encoded)
	}
	if got := string(object["email"]); got != `"template-owner@example.test"` {
		t.Fatalf("wire user email = %s, wire = %s", got, encoded)
	}
	var externalIDs []map[string]string
	if err := json.Unmarshal(object["external_ids"], &externalIDs); err != nil {
		t.Fatalf("unmarshal wire external_ids: %v, wire = %s", err, encoded)
	}
	if len(externalIDs) != 1 || externalIDs[0]["provider"] != "feishu" || externalIDs[0]["user_type"] != "open_id" || externalIDs[0]["external_id"] != "ou_template_owner" {
		t.Fatalf("wire user external_ids = %#v, wire = %s", externalIDs, encoded)
	}
}

func sortedProjectTemplateWireKeys(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func TestProjectTemplateListUsesDatabaseOrderedPages(t *testing.T) {
	f := newProjectTemplateFixture(t)
	first := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())
	second := seedProjectTemplate(t, f.owner, "later", projectTemplateSnapshotFixture())
	if err := storage.NewProjectTemplateRepository(f.store.DB()).UpdateMetadata(second.WorkspaceID, second.ID, second.Name, second.Description, 200); err != nil {
		t.Fatal(err)
	}

	page, err := f.owner.ListProjectTemplates("all", "", 1, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second.ID || page.Total != 2 {
		t.Fatalf("first ordered page = %#v, %v", page, err)
	}
	next, err := f.owner.ListProjectTemplates("all", "", 1, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != first.ID {
		t.Fatalf("second ordered page = %#v, %v", next, err)
	}
}
