package app

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/uda"
	"gorm.io/gorm"
)

func instantiateTemplateInput(t *testing.T, svc *Service, templateRef, slug string) InstantiateInput {
	t.Helper()
	template, err := svc.resolveProjectTemplate(templateRef)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.projectTemplateSnapshot(template, nil)
	if err != nil {
		t.Fatal(err)
	}
	return InstantiateInput{
		SnapshotID: snapshot.ID, ExpectedHash: snapshot.SnapshotHash,
		ProjectSlug: slug, ProjectName: "新项目", StartDate: "2026-08-01",
	}
}

func instantiateSnapshot() projecttemplate.SnapshotV1 {
	return projecttemplate.SnapshotV1{
		Schema: projecttemplate.SnapshotSchemaV1, AnchorDate: "2026-07-20",
		Project: projecttemplate.ProjectBlueprintV1{Description: "模板项目说明"},
		Configs: []projecttemplate.ConfigBlueprintV1{{Key: "agent.provider.api_key", Mode: "secret_input"}},
		Tasks: []projecttemplate.TaskBlueprintV1{{
			Ref: "task-1", Title: "准备发布",
			Dates: projecttemplate.TaskDatesV1{Due: &projecttemplate.RelativeLocalTimeV1{DayOffset: 3, LocalTime: "09:30:00"}},
		}},
		Series:      []projecttemplate.SeriesBlueprintV1{},
		Automations: []projecttemplate.AutomationBlueprintV1{},
	}
}

func fullInstantiateSnapshot(ownerID string) projecttemplate.SnapshotV1 {
	baseURL := "https://api.example.test/v1"
	parentRef := "task-3"
	taskDescription := "等待 [收尾](ref://task/task-3)"
	seriesDescription := "循环跟进 [收尾](ref://task/task-3)"
	return projecttemplate.SnapshotV1{
		Schema: projecttemplate.SnapshotSchemaV1, AnchorDate: "2026-07-20",
		Project: projecttemplate.ProjectBlueprintV1{Description: "完整模板项目"},
		Configs: []projecttemplate.ConfigBlueprintV1{
			{Key: "agent.provider.base_url", Mode: "literal", Value: &baseURL},
			{Key: "agent.provider.api_key", Mode: "secret_input"},
		},
		Tasks: []projecttemplate.TaskBlueprintV1{
			{Ref: "task-1", Title: "准备", Description: &taskDescription, AssigneeIDs: []string{ownerID}, DependsRefs: []string{"task-2"}, Links: []projecttemplate.TaskLinkBlueprintV1{{Type: "document", URL: "https://docs.example.test/launch", Title: "发布文档"}}},
			{Ref: "task-2", Title: "执行", ParentRef: &parentRef},
			{Ref: "task-3", Title: "收尾"},
		},
		Series: []projecttemplate.SeriesBlueprintV1{{
			Ref: "series-1", Title: "每日跟进", Description: &seriesDescription,
			RecurrenceRule: "daily", FirstDue: projecttemplate.RelativeLocalTimeV1{DayOffset: 0, LocalTime: "09:00:00"},
		}},
		Automations: []projecttemplate.AutomationBlueprintV1{{
			Ref: "automation-1", Name: "每日巡检", TriggerType: "schedule",
			TriggerConfig: projecttemplate.AutomationTriggerV1{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
			Action:        projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelOverride: "gpt-test"},
			Context:       projecttemplate.AutomationContextV1{Include: []string{"project"}}, InstructionTemplate: "检查项目",
		}},
	}
}

func TestProjectTemplateInstantiateCreatesFreshGraphWithoutHistory(t *testing.T) {
	f := newProjectTemplateFixture(t)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	f.owner.clock = FixedClock{NowUnix: time.Date(2026, 8, 5, 12, 0, 0, 0, loc).Unix(), Loc: loc}
	createHookTestSink(t, f.store, f.owner.workspaceID, f.owner.runtime.ActorUserID, "template-events")
	hook, err := f.owner.AddHook(HookAddInput{Name: "template-events", ScopeType: HookScopeWorkspace, EventTypes: []string{"task.created"}, SinkRef: "template-events"})
	if err != nil {
		t.Fatal(err)
	}
	seedProjectTemplate(t, f.owner, "launch", fullInstantiateSnapshot(f.owner.runtime.ActorUserID))
	input := instantiateTemplateInput(t, f.owner, "launch", "newproj")
	input.SecretInputs = map[string]string{"agent.provider.api_key": "secret-canary-must-not-leak"}

	got, err := f.owner.InstantiateCurrentProjectTemplate("launch", input)
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := ComponentCounts{Configs: 2, Tasks: 3, Series: 1, Automations: 1}
	if got.Project.Status != string(storage.ProjectStatusPlanning) || got.Counts != wantCounts {
		t.Fatalf("result = %#v", got)
	}

	var taskRows []storage.Task
	if err := f.store.DB().Where("workspace_id = ? AND project_id = ? AND series_id IS NULL", f.owner.workspaceID, got.Project.ID).Order("project_seq ASC").Find(&taskRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(taskRows) != 3 {
		t.Fatalf("tasks = %#v", taskRows)
	}
	byTitle := map[string]task.Task{}
	for i, row := range taskRows {
		if row.ProjectSeq == nil || *row.ProjectSeq != int64(i+1) {
			t.Fatalf("task seq order = %#v", taskRows)
		}
		if _, err := uuid.Parse(row.UUID); err != nil {
			t.Fatalf("task UUID = %q", row.UUID)
		}
		resolved, err := f.owner.repo.GetByUUID(f.owner.workspaceID, row.UUID)
		if err != nil {
			t.Fatal(err)
		}
		byTitle[row.Title] = resolved
	}
	if byTitle["准备"].Description == nil || !strings.Contains(*byTitle["准备"].Description, byTitle["收尾"].UUID) || len(byTitle["准备"].Depends) != 1 || byTitle["准备"].Depends[0] != byTitle["执行"].UUID {
		t.Fatalf("mapped task refs/depends = %#v", byTitle["准备"])
	}
	if byTitle["执行"].Parent == nil || *byTitle["执行"].Parent != byTitle["收尾"].UUID {
		t.Fatalf("mapped parent = %#v", byTitle["执行"])
	}
	if len(byTitle["准备"].Links) != 1 || byTitle["准备"].Links[0].ID == "" || byTitle["准备"].Links[0].CreatedBy.User == nil || byTitle["准备"].Links[0].CreatedBy.User.ID != f.owner.runtime.ActorUserID {
		t.Fatalf("fresh link/actor = %#v", byTitle["准备"].Links)
	}

	var seriesRows []storage.TaskSeries
	if err := f.store.DB().Where("workspace_id = ? AND project_id = ?", f.owner.workspaceID, got.Project.ID).Find(&seriesRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(seriesRows) != 1 || seriesRows[0].ProjectSeq == nil || *seriesRows[0].ProjectSeq != 1 || seriesRows[0].Status != "active" {
		t.Fatalf("series = %#v", seriesRows)
	}
	series, err := f.owner.taskSeriesRepo.Get(f.owner.workspaceID, seriesRows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(series.RuleVersions) != 1 || series.Description == nil || !strings.Contains(*series.Description, byTitle["收尾"].UUID) {
		t.Fatalf("series history/description = %#v", series)
	}
	var occurrenceCount int64
	if err := f.store.DB().Model(&storage.Task{}).Where("workspace_id = ? AND project_id = ? AND series_id IS NOT NULL", f.owner.workspaceID, got.Project.ID).Count(&occurrenceCount).Error; err != nil || occurrenceCount != 0 {
		t.Fatalf("occurrence count = %d, err=%v", occurrenceCount, err)
	}

	rules, err := f.owner.projectAutomationRuleRepo.List(f.owner.workspaceID, &got.Project.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Enabled == nil || *rules[0].Enabled {
		t.Fatalf("automation rules = %#v", rules)
	}
	var automationDeliveries int64
	if err := f.store.DB().Model(&storage.ProjectAutomationDelivery{}).Where("project_id = ?", got.Project.ID).Count(&automationDeliveries).Error; err != nil || automationDeliveries != 0 {
		t.Fatalf("automation deliveries = %d, err=%v", automationDeliveries, err)
	}

	deliveries, err := f.owner.hookDeliveryRepo.ListByHook(hook.ID, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 3 {
		t.Fatalf("task.created deliveries = %#v", deliveries)
	}
	template, err := f.owner.resolveProjectTemplate("launch")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.owner.projectTemplateSnapshot(template, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, delivery := range deliveries {
		var envelope map[string]any
		if err := json.Unmarshal([]byte(delivery.PayloadJSON), &envelope); err != nil {
			t.Fatal(err)
		}
		data, _ := envelope["data"].(map[string]any)
		if data["source_template_id"] != template.ID || data["source_template_snapshot_id"] != snapshot.ID || data["source_template_snapshot_hash"] != snapshot.SnapshotHash {
			t.Fatalf("source metadata = %#v", data)
		}
	}

	var audits []storage.AuditLogEntry
	if err := f.store.DB().Table("audit_logs").Where("project_id = ?", got.Project.ID).Order("id ASC").Find(&audits).Error; err != nil {
		t.Fatal(err)
	}
	actions := make([]string, 0, len(audits))
	joined := ""
	for _, audit := range audits {
		actions = append(actions, audit.Action)
		joined += audit.PayloadJSON
	}
	sort.Strings(actions)
	if !containsString(actions, "project_template.instantiate") || !containsString(actions, "project.add") || !containsString(actions, "task.add") || !containsString(actions, "task.series.created") || !containsString(actions, "project.config.set") || !containsString(actions, "task.link.add") {
		t.Fatalf("audit actions = %#v", actions)
	}
	joined += string(mustJSON(deliveries))
	if strings.Contains(joined, input.SecretInputs["agent.provider.api_key"]) {
		t.Fatalf("secret leaked to audit/event: %s", joined)
	}
}

func TestProjectTemplateInstantiateRollbackLeavesNothing(t *testing.T) {
	stages := []string{"project-create", "config-create", "series-create", "task-shell-create", "task-finalize", "link-create", "automation-create", "audit-write"}
	for index, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			f := newProjectTemplateFixture(t)
			seedProjectTemplate(t, f.owner, "launch", fullInstantiateSnapshot(f.owner.runtime.ActorUserID))
			slug := fmt.Sprintf("rollback%d", index)
			input := instantiateTemplateInput(t, f.owner, "launch", slug)
			input.SecretInputs = map[string]string{"agent.provider.api_key": "rollback-secret"}
			models := []any{&storage.Project{}, &storage.Config{}, &storage.Task{}, &storage.TaskLink{}, &storage.TaskSeries{}, &storage.TaskSeriesRuleVersion{}, &storage.ProjectAutomationRule{}, &storage.ProjectAutomationDelivery{}, &storage.AuditLog{}}
			before := make([]int64, len(models))
			for i, model := range models {
				if err := f.store.DB().Model(model).Count(&before[i]).Error; err != nil {
					t.Fatal(err)
				}
			}
			f.owner.projectTemplateInstantiateFailure = func(current string) error {
				if current == stage {
					return errors.New("injected " + stage + " failure")
				}
				return nil
			}
			if _, err := f.owner.InstantiateCurrentProjectTemplate("launch", input); err == nil || strings.Contains(err.Error(), input.SecretInputs["agent.provider.api_key"]) {
				t.Fatalf("instantiate error = %v", err)
			}
			if _, err := f.owner.ResolveProject(slug); runtimeCode(err) != "project_not_found" {
				t.Fatalf("rollback project remained: %v", err)
			}
			for i, model := range models {
				var count int64
				if err := f.store.DB().Model(model).Count(&count).Error; err != nil || count != before[i] {
					t.Fatalf("rollback residue %T before=%d after=%d err=%v", model, before[i], count, err)
				}
			}
		})
	}
}

func TestProjectTemplateInstantiateLocksTemplateBeforeRecheckingCurrentState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*storage.Store, storage.ProjectTemplate, projecttemplate.SnapshotV1) error
		check  func(*testing.T, *storage.Store, storage.ProjectTemplate)
	}{
		{
			name: "append-current-snapshot",
			mutate: func(store *storage.Store, template storage.ProjectTemplate, snapshot projecttemplate.SnapshotV1) error {
				snapshot.Project.Description = "并发追加的新 current"
				raw, hash, err := projecttemplate.EncodeV1(snapshot, projecttemplate.DefaultLimits)
				if err != nil {
					return err
				}
				current, err := storage.NewProjectTemplateRepository(store.DB()).GetSnapshot(template.WorkspaceID, template.ID, *template.CurrentSnapshotID)
				if err != nil {
					return err
				}
				return store.Transaction(func(txStore *storage.Store) error {
					_, err := storage.NewProjectTemplateRepository(txStore.DB()).AppendSnapshotLocked(template.WorkspaceID, template.ID, storage.ProjectTemplateSnapshot{
						ID: uuid.NewString(), SourceProjectID: current.SourceProjectID, SnapshotJSON: string(raw), SnapshotHash: hash,
						CreatedByActorType: actorTypeUser, CreatedAt: 101,
					})
					return err
				})
			},
			check: func(t *testing.T, store *storage.Store, template storage.ProjectTemplate) {
				var count int64
				if err := store.DB().Model(&storage.ProjectTemplateSnapshot{}).Where("template_id = ?", template.ID).Count(&count).Error; err != nil || count != 2 {
					t.Fatalf("snapshot count=%d err=%v, want 2", count, err)
				}
			},
		},
		{
			name: "archive-template",
			mutate: func(store *storage.Store, template storage.ProjectTemplate, _ projecttemplate.SnapshotV1) error {
				now := int64(101)
				return store.Transaction(func(txStore *storage.Store) error {
					return storage.NewProjectTemplateRepository(txStore.DB()).SetStatus(template.WorkspaceID, template.ID, "archived", &now, now)
				})
			},
			check: func(t *testing.T, store *storage.Store, template storage.ProjectTemplate) {
				row, err := storage.NewProjectTemplateRepository(store.DB()).GetByRef(template.WorkspaceID, template.ID)
				if err != nil || row.Status != "archived" {
					t.Fatalf("template=%#v err=%v, want archived", row, err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
			store, err := storage.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := instantiateSnapshot()
			snapshot.Configs = []projecttemplate.ConfigBlueprintV1{}
			template := seedProjectTemplate(t, svc, "locked-instantiate", snapshot)
			input := instantiateTemplateInput(t, svc, template.ID, "locked1")

			other, err := storage.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = other.Close() })

			readLockedState := make(chan struct{})
			releaseInstantiate := make(chan struct{})
			var pause atomic.Bool
			pause.Store(true)
			callbackName := "test:pause_project_template_instantiate_after_template_read"
			if err := store.DB().Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
				table := db.Statement.Table
				if table == "" && db.Statement.Schema != nil {
					table = db.Statement.Schema.Table
				}
				if table != "project_templates" || !pause.CompareAndSwap(true, false) {
					return
				}
				close(readLockedState)
				<-releaseInstantiate
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.DB().Callback().Query().Remove(callbackName) })

			instantiateDone := make(chan error, 1)
			go func() {
				_, err := svc.InstantiateCurrentProjectTemplate(template.ID, input)
				instantiateDone <- err
			}()
			select {
			case <-readLockedState:
			case <-time.After(2 * time.Second):
				t.Fatal("instantiate did not reach the locked template read")
			}

			mutationDone := make(chan error, 1)
			go func() { mutationDone <- tc.mutate(other, template, snapshot) }()
			select {
			case err := <-mutationDone:
				close(releaseInstantiate)
				t.Fatalf("concurrent template mutation crossed instantiate lock: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(releaseInstantiate)
			if err := <-instantiateDone; err != nil {
				t.Fatalf("instantiate error: %v", err)
			}
			if err := <-mutationDone; err != nil {
				t.Fatalf("concurrent mutation error: %v", err)
			}
			tc.check(t, other, template)
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasTemplateIssue(items []ProjectTemplateIssue, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func TestProjectTemplateInstantiatePreviewReturnsIssuesWithoutSecrets(t *testing.T) {
	f := newProjectTemplateFixture(t)
	snapshot := instantiateSnapshot()
	memberID := f.owner.Runtime().ActorUserID
	snapshot.Tasks[0].AssigneeIDs = []string{memberID}
	seedProjectTemplate(t, f.owner, "launch", snapshot)
	if err := f.owner.memberRepo.Delete(memberID, f.owner.Runtime().WorkspaceID); err != nil {
		t.Fatal(err)
	}

	input := instantiateTemplateInput(t, f.owner, "launch", "newproj")
	preview, err := f.owner.PreviewProjectTemplateInstantiation("launch", input)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTemplateIssue(preview.Issues, "project_template_member_unavailable") || !hasTemplateIssue(preview.Issues, "project_template_secret_required") {
		t.Fatalf("issues = %#v", preview.Issues)
	}
	if len(preview.SecretResolutions) != 1 || preview.SecretResolutions[0].ResolvedFrom != "missing" {
		t.Fatalf("secret resolutions = %#v", preview.SecretResolutions)
	}
	if len(preview.AssigneeIssues) != 1 || preview.AssigneeIssues[0].User.ID != memberID || preview.AssigneeIssues[0].Resolution != "unresolved" {
		t.Fatalf("assignee issues = %#v", preview.AssigneeIssues)
	}

	input.SecretInputs = map[string]string{"agent.provider.api_key": "sk-secret"}
	replacement := (*string)(nil)
	input.AssigneeReplacements = map[string]*string{memberID: replacement}
	preview, err = f.owner.PreviewProjectTemplateInstantiation("launch", input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Issues) != 0 || preview.SecretResolutions[0].ResolvedFrom != "input" || preview.AssigneeIssues[0].Resolution != "removed" {
		t.Fatalf("resolved preview = %#v", preview)
	}
	raw, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-secret") || strings.Contains(string(raw), `"value"`) {
		t.Fatalf("secret leaked: %s", raw)
	}
	if !strings.Contains(string(raw), `"assignee_issues":[{"user":{"id":"`+memberID+`"`) {
		t.Fatalf("user shape is not normalized: %s", raw)
	}
}

func TestProjectTemplateInstantiatePreviewPinsHistoricalSnapshotAndCurrentOnlyRejectsIt(t *testing.T) {
	f := newProjectTemplateFixture(t)
	first := seedProjectTemplate(t, f.owner, "versions", instantiateSnapshot())
	oldID := *first.CurrentSnapshotID
	old, err := f.owner.projectTemplateRepo.GetSnapshot(first.WorkspaceID, first.ID, oldID)
	if err != nil {
		t.Fatal(err)
	}
	next := instantiateSnapshot()
	next.Project.Description = "第二版"
	appendInstantiateSnapshot(t, f.owner, first, next)

	input := InstantiateInput{SnapshotID: old.ID, ExpectedHash: old.SnapshotHash, ProjectSlug: "oldproj", ProjectName: "旧版项目", StartDate: "2026-08-01"}
	preview, err := f.owner.PreviewProjectTemplateInstantiation(first.Key, input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Snapshot.ID != old.ID || preview.Snapshot.Hash != old.SnapshotHash || preview.Project.Description != "模板项目说明" {
		t.Fatalf("historical preview = %#v", preview)
	}
	if _, err := f.owner.buildInstantiatePlan(first.Key, input, true); runtimeCode(err) != "project_template_snapshot_hash_mismatch" {
		t.Fatalf("current-only historical error = %v", err)
	}

	input.SnapshotID = ""
	currentPreview, err := f.owner.PreviewProjectTemplateInstantiation(first.Key, input)
	if err == nil || runtimeCode(err) != "project_template_snapshot_hash_mismatch" {
		t.Fatalf("current with stale expected hash = %#v, %v", currentPreview, err)
	}
	input.ExpectedHash = strings.Repeat("A", 64)
	if _, err := f.owner.PreviewProjectTemplateInstantiation(first.Key, input); runtimeCode(err) != "project_template_snapshot_hash_mismatch" {
		t.Fatalf("uppercase hash error = %v", err)
	}
}

func TestProjectTemplateInstantiatePreviewValidatesConfigUDAAutomationAndDates(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	f := newProjectTemplateFixture(t)
	f.owner.clock = FixedClock{NowUnix: time.Date(2026, 1, 1, 12, 0, 0, 0, loc).Unix(), Loc: loc}
	literal := "not-a-number"
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{{Key: "launch.score", Mode: "literal", Value: &literal}}
	snapshot.Tasks[0].UDAs = map[string]projecttemplate.UDABlueprintV1{"estimate": {Raw: "abc", Type: "numeric"}}
	snapshot.Tasks[0].Dates.Due = &projecttemplate.RelativeLocalTimeV1{DayOffset: 1, LocalTime: "02:30:00"}
	snapshot.Automations = []projecttemplate.AutomationBlueprintV1{{
		Ref: "automation-1", Name: "巡检", TriggerType: "schedule",
		TriggerConfig: projecttemplate.AutomationTriggerV1{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:        projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "missing.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelOverride: "gpt-test"},
		Context:       projecttemplate.AutomationContextV1{Include: []string{}}, InstructionTemplate: "检查",
	}}
	seedProjectTemplate(t, f.owner, "invalid", snapshot)
	if err := f.owner.configDefRepo.Set(storage.ConfigDefinition{WorkspaceID: f.owner.workspaceID, Key: "launch.score", ValueType: "number", AllowedScopesJSON: `["project"]`, EnumValuesJSON: `[]`, CreatedAt: 1, ModifiedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := f.owner.udaRepo.UpsertDefinition(f.owner.workspaceID, uda.Definition{Name: "estimate", Type: uda.TypeNumeric}, 1); err != nil {
		t.Fatal(err)
	}
	input := instantiateTemplateInput(t, f.owner, "invalid", "invalidp")
	input.StartDate = "2026-03-07"
	preview, err := f.owner.PreviewProjectTemplateInstantiation("invalid", input)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"project_template_config_invalid", "project_template_uda_invalid", "project_template_automation_invalid", "project_template_date_out_of_range"} {
		if !hasTemplateIssue(preview.Issues, code) {
			t.Fatalf("missing %s in %#v", code, preview.Issues)
		}
	}
}

func TestProjectTemplateInstantiatePermissionAndValidationError(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "permission", instantiateSnapshot())
	input := instantiateTemplateInput(t, f.owner, seeded.Key, "permissionp")

	tenant := newScopedTokenService(t, f.store, []string{auth.ScopeProjectWrite}, auth.TokenTypeTenantAccess)
	if _, err := tenant.PreviewProjectTemplateInstantiation(seeded.Key, input); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("missing task/config permissions error = %v", err)
	}
	tenant = newScopedTokenService(t, f.store, []string{auth.ScopeProjectWrite, auth.ScopeTaskWrite, auth.ScopeConfigWrite}, auth.TokenTypeTenantAccess)
	preview, err := tenant.PreviewProjectTemplateInstantiation(seeded.Key, input)
	if err != nil || !hasTemplateIssue(preview.Issues, "project_template_secret_required") {
		t.Fatalf("tenant preview = %#v, %v", preview, err)
	}

	scope := &RequestScope{WorkspaceIDs: []string{f.owner.workspaceID}, ProjectIDs: []string{"only-project"}, Capabilities: []string{auth.ScopeProjectWrite, auth.ScopeTaskWrite, auth.ScopeConfigWrite}}
	runtime := f.owner.Runtime()
	scoped, err := NewService(ServiceOptions{Store: f.store, Clock: f.owner.clock, Runtime: &runtime, RequestScope: scope, DisableScopeBootstrap: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.PreviewProjectTemplateInstantiation(seeded.Key, input); runtimeCode(err) != authz.CodeProjectScopeDenied {
		t.Fatalf("project-scoped preview error = %v", err)
	}

	validationErr := ProjectTemplateValidationError{Issues: []ProjectTemplateIssue{{Code: "project_template_secret_required", Severity: "blocking", Message: "missing"}}}
	if validationErr.PrimaryCode() != "project_template_secret_required" || validationErr.Error() == "" {
		t.Fatalf("validation error = %#v", validationErr)
	}
	var target ProjectTemplateValidationError
	if !errors.As(validationErr, &target) {
		t.Fatal("validation error is not typed")
	}
}

func TestProjectTemplateInstantiatePreviewKeepsInheritedSecretOutOfProjectRows(t *testing.T) {
	f := newProjectTemplateFixture(t)
	baseURL, model, historicalSecret := "https://api.example.test", "gpt-test", "old-secret-must-not-leak"
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{
		{Key: "agent.provider.base_url", Mode: "literal", Value: &baseURL},
		// 模拟历史 Snapshot：Capture 时还是 literal，当前 schema 已改为 secret。
		{Key: "agent.provider.api_key", Mode: "literal", Value: &historicalSecret},
		{Key: "agent.provider.model", Mode: "literal", Value: &model},
	}
	snapshot.Automations = []projecttemplate.AutomationBlueprintV1{{
		Ref: "automation-1", Name: "巡检", TriggerType: "schedule",
		TriggerConfig: projecttemplate.AutomationTriggerV1{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:        projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", AllowedHostsConfigKey: "agent.provider.allowed_hosts"},
		Context:       projecttemplate.AutomationContextV1{Include: []string{}}, InstructionTemplate: "检查",
	}}
	seedProjectTemplate(t, f.owner, "inherit", snapshot)
	if err := f.owner.configRepo.Set(storage.ConfigKey{WorkspaceID: f.owner.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: f.owner.workspaceID, Key: "agent.provider.api_key"}, "workspace-secret"); err != nil {
		t.Fatal(err)
	}
	input := instantiateTemplateInput(t, f.owner, "inherit", "inheritp")
	plan, err := f.owner.buildInstantiatePlan("inherit", input, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Preview.Issues) != 0 || plan.Preview.Counts.Configs != 3 || plan.Preview.SecretResolutions[0].ResolvedFrom != "workspace" {
		t.Fatalf("preview = %#v", plan.Preview)
	}
	if _, copied := plan.ConfigValues["agent.provider.api_key"]; copied || len(plan.ConfigValues) != 2 {
		t.Fatalf("project config rows = %#v", plan.ConfigValues)
	}
	raw := string(mustJSON(plan.Preview))
	for _, secret := range []string{"workspace-secret", historicalSecret} {
		if strings.Contains(raw, secret) {
			t.Fatalf("secret %q leaked: %s", secret, raw)
		}
	}
	if len(plan.Automations) != 1 || plan.Automations[0].Input.Enabled {
		t.Fatalf("planned automations = %#v", plan.Automations)
	}
}

func TestProjectTemplateInstantiatePreviewReplacesOnlyUnavailableMembers(t *testing.T) {
	f := newProjectTemplateFixture(t)
	oldUser := storage.User{ID: uuid.NewString(), Name: "old-template-member", DisplayName: "旧成员", CreatedAt: 1, ModifiedAt: 1}
	newUser := storage.User{ID: uuid.NewString(), Name: "new-template-member", DisplayName: "新成员", CreatedAt: 1, ModifiedAt: 1}
	if _, err := f.owner.userRepo.Create(oldUser); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.userRepo.Create(newUser); err != nil {
		t.Fatal(err)
	}
	if err := f.owner.memberRepo.Upsert(storage.Membership{UserID: newUser.ID, WorkspaceID: f.owner.workspaceID, Role: string(RoleMember), JoinedAt: 1, ModifiedAt: 1}); err != nil {
		t.Fatal(err)
	}
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{}
	snapshot.Tasks[0].AssigneeIDs = []string{oldUser.ID}
	description := "请联系 [旧成员](ref://user/" + oldUser.ID + ")\n```md\n[示例](ref://user/" + oldUser.ID + ")\n```"
	snapshot.Tasks[0].Description = &description
	seedProjectTemplate(t, f.owner, "replace", snapshot)
	input := instantiateTemplateInput(t, f.owner, "replace", "replacep")
	input.AssigneeReplacements = map[string]*string{oldUser.ID: &newUser.ID}
	plan, err := f.owner.buildInstantiatePlan("replace", input, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Preview.Issues) != 0 || len(plan.Tasks) != 1 || len(plan.Tasks[0].AssigneeIDs) != 1 || plan.Tasks[0].AssigneeIDs[0] != newUser.ID || plan.Preview.AssigneeIssues[0].Resolution != "replaced" {
		t.Fatalf("replacement plan = %#v", plan)
	}
	if plan.Tasks[0].Description == nil || !strings.Contains(*plan.Tasks[0].Description, "[旧成员](ref://user/"+newUser.ID+")") || !strings.Contains(*plan.Tasks[0].Description, "```md\n[示例](ref://user/"+oldUser.ID+")") {
		t.Fatalf("member reference rewrite changed the wrong Markdown nodes: %q", optionalTextValue(plan.Tasks[0].Description))
	}

	input.AssigneeReplacements = map[string]*string{oldUser.ID: nil}
	removed, err := f.owner.buildInstantiatePlan("replace", input, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Tasks[0].Description == nil || !strings.Contains(*removed.Tasks[0].Description, "[旧成员](#)") || !strings.Contains(*removed.Tasks[0].Description, "```md\n[示例](ref://user/"+oldUser.ID+")") {
		t.Fatalf("member reference removal changed the wrong Markdown nodes: %q", optionalTextValue(removed.Tasks[0].Description))
	}

	ownerID := f.owner.Runtime().ActorUserID
	input.AssigneeReplacements = map[string]*string{ownerID: &newUser.ID}
	preview, err := f.owner.PreviewProjectTemplateInstantiation("replace", input)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTemplateIssue(preview.Issues, "project_template_member_unavailable") {
		t.Fatalf("unexpected replacement was accepted: %#v", preview)
	}
}

func TestProjectTemplateInstantiatePreviewBuildsFreshLocalReferencePlan(t *testing.T) {
	f := newProjectTemplateFixture(t)
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{}
	description := "等待 [后续任务](ref://task/task-2)"
	snapshot.Tasks = []projecttemplate.TaskBlueprintV1{
		{Ref: "task-1", Title: "前置", Description: &description, DependsRefs: []string{"task-2"}},
		{Ref: "task-2", Title: "后续"},
	}
	seedProjectTemplate(t, f.owner, "refs", snapshot)
	input := instantiateTemplateInput(t, f.owner, "refs", "refsproj")
	first, err := f.owner.buildInstantiatePlan("refs", input, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.owner.buildInstantiatePlan("refs", input, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(first.Preview)) != string(mustJSON(second.Preview)) {
		t.Fatalf("preview is not deterministic:\n%s\n%s", mustJSON(first.Preview), mustJSON(second.Preview))
	}
	if first.TaskIDs["task-1"] == first.TaskIDs["task-2"] || first.TaskIDs["task-1"] == second.TaskIDs["task-1"] {
		t.Fatalf("task IDs are not fresh: first=%#v second=%#v", first.TaskIDs, second.TaskIDs)
	}
	if len(first.Tasks[0].DependsIDs) != 1 || first.Tasks[0].DependsIDs[0] != first.TaskIDs["task-2"] || first.Tasks[0].Description == nil || !strings.Contains(*first.Tasks[0].Description, first.TaskIDs["task-2"]) {
		t.Fatalf("local refs were not rewritten: %#v", first.Tasks[0])
	}
}

func TestProjectTemplateInstantiatePreviewRejectsArchivedConflictAndInvalidHash(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seedProjectTemplate(t, f.owner, "guards", instantiateSnapshot())
	if _, err := f.owner.AddProject(AddProjectInput{Slug: "occupied", Name: "已占用"}); err != nil {
		t.Fatal(err)
	}
	input := instantiateTemplateInput(t, f.owner, "guards", "occupied")
	preview, err := f.owner.PreviewProjectTemplateInstantiation("guards", input)
	if err != nil || !hasTemplateIssue(preview.Issues, "project_already_exists") {
		t.Fatalf("slug conflict preview = %#v, %v", preview, err)
	}
	input.ExpectedHash = strings.Repeat("0", 64)
	if _, err := f.owner.PreviewProjectTemplateInstantiation("guards", input); runtimeCode(err) != "project_template_snapshot_hash_mismatch" {
		t.Fatalf("hash mismatch error = %v", err)
	}
	input = instantiateTemplateInput(t, f.owner, "guards", "newguard")
	if _, err := f.owner.ArchiveProjectTemplate("guards"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.PreviewProjectTemplateInstantiation("guards", input); runtimeCode(err) != "project_template_archived" {
		t.Fatalf("archived error = %v", err)
	}
}

func TestProjectTemplateInstantiatePreviewReturnsTypedIssueForLegacyRefCycle(t *testing.T) {
	f := newProjectTemplateFixture(t)
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{}
	snapshot.Tasks = []projecttemplate.TaskBlueprintV1{{Ref: "task-1", Title: "一"}, {Ref: "task-2", Title: "二"}}
	seeded := seedProjectTemplate(t, f.owner, "cycle", snapshot)
	current, err := f.owner.projectTemplateSnapshot(seeded, nil)
	if err != nil {
		t.Fatal(err)
	}
	parentOne, parentTwo := "task-2", "task-1"
	snapshot.Tasks[0].ParentRef, snapshot.Tasks[1].ParentRef = &parentOne, &parentTwo
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	hash := fmt.Sprintf("%x", sum[:])
	if err := f.store.DB().Model(&storage.ProjectTemplateSnapshot{}).Where("id = ?", current.ID).Updates(map[string]any{"snapshot_json": string(raw), "snapshot_hash": hash}).Error; err != nil {
		t.Fatal(err)
	}
	input := InstantiateInput{SnapshotID: current.ID, ExpectedHash: hash, ProjectSlug: "cycleproj", ProjectName: "循环项目", StartDate: "2026-08-01"}
	preview, err := f.owner.PreviewProjectTemplateInstantiation("cycle", input)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTemplateIssue(preview.Issues, "project_template_ref_cycle") {
		t.Fatalf("cycle preview = %#v", preview)
	}
	if plan, err := f.owner.buildInstantiatePlan("cycle", input, false); err != nil {
		t.Fatal(err)
	} else {
		var validationErr ProjectTemplateValidationError
		if !errors.As(plan.validationError(), &validationErr) || validationErr.PrimaryCode() != "project_template_ref_cycle" {
			t.Fatalf("validation error = %#v, %v", validationErr, plan.validationError())
		}
	}
}

func TestProjectTemplateInstantiatePreviewRejectsOversizedAgentConfigValues(t *testing.T) {
	f := newProjectTemplateFixture(t)
	oversized := strings.Repeat("x", agentConfigValueMaxBytes+1)
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{
		{Key: "agent.provider.base_url", Mode: "literal", Value: &oversized},
		{Key: "agent.provider.api_key", Mode: "secret_input"},
	}
	seedProjectTemplate(t, f.owner, "oversized", snapshot)
	input := instantiateTemplateInput(t, f.owner, "oversized", "largeconf")
	input.SecretInputs = map[string]string{"agent.provider.api_key": oversized}
	plan, err := f.owner.buildInstantiatePlan("oversized", input, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Preview.Counts.Configs != 0 || len(plan.ConfigValues) != 0 {
		t.Fatalf("oversized configs entered plan: counts=%#v values=%d", plan.Preview.Counts, len(plan.ConfigValues))
	}
	count := 0
	for _, issue := range plan.Preview.Issues {
		if issue.Code == "project_template_config_invalid" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("config issues = %#v", plan.Preview.Issues)
	}
	if strings.Contains(string(mustJSON(plan.Preview)), oversized) {
		t.Fatal("oversized secret leaked into preview")
	}
}

func TestProjectTemplateInstantiatePreviewUsesValidHistoricalWhenCurrentHasLegacyCycle(t *testing.T) {
	f := newProjectTemplateFixture(t)
	historicalSnapshot := instantiateSnapshot()
	historicalSnapshot.Configs = []projecttemplate.ConfigBlueprintV1{}
	seeded := seedProjectTemplate(t, f.owner, "legacy-current", historicalSnapshot)
	historical, err := f.owner.projectTemplateSnapshot(seeded, nil)
	if err != nil {
		t.Fatal(err)
	}
	currentSnapshot := historicalSnapshot
	currentSnapshot.Tasks = []projecttemplate.TaskBlueprintV1{{Ref: "task-1", Title: "一"}, {Ref: "task-2", Title: "二"}}
	current := appendInstantiateSnapshot(t, f.owner, seeded, currentSnapshot)
	parentOne, parentTwo := "task-2", "task-1"
	currentSnapshot.Tasks[0].ParentRef, currentSnapshot.Tasks[1].ParentRef = &parentOne, &parentTwo
	raw, err := json.Marshal(currentSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if err := f.store.DB().Model(&storage.ProjectTemplateSnapshot{}).Where("id = ?", current.ID).Updates(map[string]any{"snapshot_json": string(raw), "snapshot_hash": fmt.Sprintf("%x", sum[:])}).Error; err != nil {
		t.Fatal(err)
	}
	input := InstantiateInput{SnapshotID: historical.ID, ExpectedHash: historical.SnapshotHash, ProjectSlug: "historyok", ProjectName: "历史项目", StartDate: "2026-08-01"}
	preview, err := f.owner.PreviewProjectTemplateInstantiation(seeded.Key, input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Snapshot.ID != historical.ID || preview.Template.CurrentSnapshot == nil || preview.Template.CurrentSnapshot.ID != current.ID || len(preview.Issues) != 0 {
		t.Fatalf("historical preview = %#v", preview)
	}
}

func TestProjectTemplateInstantiatePreviewResolvesDefaultSecretWithoutCopyingIt(t *testing.T) {
	f := newProjectTemplateFixture(t)
	key := "template.default_secret"
	if err := f.owner.configDefRepo.Set(storage.ConfigDefinition{
		WorkspaceID: f.owner.workspaceID, Key: key, ValueType: "string",
		AllowedScopesJSON: `["project"]`, EnumValuesJSON: `[]`,
		DefaultValue: "sk-default-inherited-never-leak", HasDefault: true, Secret: true, CreatedAt: 1, ModifiedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{{Key: key, Mode: "secret_input"}}
	seedProjectTemplate(t, f.owner, "default-secret", snapshot)
	input := instantiateTemplateInput(t, f.owner, "default-secret", "defaultsec")
	plan, err := f.owner.buildInstantiatePlan("default-secret", input, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Preview.Issues) != 0 || len(plan.Preview.SecretResolutions) != 1 || plan.Preview.SecretResolutions[0].ResolvedFrom != "default" || plan.Preview.Counts.Configs != 1 {
		t.Fatalf("default secret preview = %#v", plan.Preview)
	}
	if len(plan.ConfigValues) != 0 || strings.Contains(string(mustJSON(plan.Preview)), "sk-default-inherited-never-leak") {
		t.Fatalf("default secret was copied or leaked: values=%#v preview=%s", plan.ConfigValues, mustJSON(plan.Preview))
	}
}

func TestProjectTemplateInstantiatePreviewValidatesAutomationAllowedHosts(t *testing.T) {
	f := newProjectTemplateFixture(t)
	baseURL, model, allowedHosts := "https://api.example.test", "gpt-test", `["other.example"]`
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{
		{Key: "agent.provider.base_url", Mode: "literal", Value: &baseURL},
		{Key: "agent.provider.api_key", Mode: "secret_input"},
		{Key: "agent.provider.model", Mode: "literal", Value: &model},
		{Key: "agent.provider.allowed_hosts", Mode: "literal", Value: &allowedHosts},
	}
	snapshot.Automations = []projecttemplate.AutomationBlueprintV1{{
		Ref: "automation-1", Name: "巡检", TriggerType: "schedule",
		TriggerConfig: projecttemplate.AutomationTriggerV1{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:        projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", AllowedHostsConfigKey: "agent.provider.allowed_hosts"},
		Context:       projecttemplate.AutomationContextV1{Include: []string{}}, InstructionTemplate: "检查",
	}}
	seedProjectTemplate(t, f.owner, "host-denied", snapshot)
	input := instantiateTemplateInput(t, f.owner, "host-denied", "hostdeny")
	input.SecretInputs = map[string]string{"agent.provider.api_key": "secret"}
	preview, err := f.owner.PreviewProjectTemplateInstantiation("host-denied", input)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTemplateIssue(preview.Issues, "project_template_automation_invalid") {
		t.Fatalf("allowed-host mismatch was accepted: %#v", preview)
	}
}

func TestProjectTemplateInstantiatePreviewRejectsInvalidAutomationAllowedHosts(t *testing.T) {
	f := newProjectTemplateFixture(t)
	baseURL, model := "https://api.example.test", "gpt-test"
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{
		{Key: "agent.provider.base_url", Mode: "literal", Value: &baseURL},
		{Key: "agent.provider.api_key", Mode: "secret_input"},
		{Key: "agent.provider.model", Mode: "literal", Value: &model},
	}
	snapshot.Automations = []projecttemplate.AutomationBlueprintV1{{
		Ref: "automation-1", Name: "巡检", TriggerType: "schedule",
		TriggerConfig: projecttemplate.AutomationTriggerV1{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:        projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", AllowedHostsConfigKey: "agent.provider.allowed_hosts"},
		Context:       projecttemplate.AutomationContextV1{Include: []string{}}, InstructionTemplate: "检查",
	}}
	seedProjectTemplate(t, f.owner, "host-invalid", snapshot)
	// 模拟 legacy/corrupt workspace row，绕过当前 JSON config 写入校验。
	if err := f.owner.configRepo.Set(storage.ConfigKey{WorkspaceID: f.owner.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: f.owner.workspaceID, Key: "agent.provider.allowed_hosts"}, "not-json"); err != nil {
		t.Fatal(err)
	}
	input := instantiateTemplateInput(t, f.owner, "host-invalid", "hostbad")
	input.SecretInputs = map[string]string{"agent.provider.api_key": "secret"}
	preview, err := f.owner.PreviewProjectTemplateInstantiation("host-invalid", input)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTemplateIssue(preview.Issues, "project_template_automation_invalid") {
		t.Fatalf("invalid allowed-host config was accepted: %#v", preview)
	}
}

func TestProjectTemplateInstantiatePreviewMarshalsCreatedByAsJSONActorInfo(t *testing.T) {
	f := newProjectTemplateFixture(t)
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{}
	seedProjectTemplate(t, f.owner, "actor-wire", snapshot)
	input := instantiateTemplateInput(t, f.owner, "actor-wire", "actorwire")
	preview, err := f.owner.PreviewProjectTemplateInstantiation("actor-wire", input)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Template struct {
			CreatedBy map[string]any `json:"created_by"`
		} `json:"template"`
		Snapshot struct {
			CreatedBy map[string]any `json:"created_by"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(mustJSON(preview)), &wire); err != nil {
		t.Fatal(err)
	}
	for name, actor := range map[string]map[string]any{"template": wire.Template.CreatedBy, "snapshot": wire.Snapshot.CreatedBy} {
		if _, ok := actor["type"]; !ok || actor["user"] == nil {
			t.Fatalf("%s created_by is not JSONActorInfo: %#v", name, actor)
		}
		for _, upper := range []string{"Type", "ID", "Name", "User", "Token"} {
			if _, leaked := actor[upper]; leaked {
				t.Fatalf("%s created_by leaked Go field %q: %#v", name, upper, actor)
			}
		}
	}
}

func TestProjectTemplateInstantiatePreviewRejectsNullAutomationAllowedHosts(t *testing.T) {
	f := newProjectTemplateFixture(t)
	baseURL, model := "https://api.example.test", "gpt-test"
	snapshot := instantiateSnapshot()
	snapshot.Configs = []projecttemplate.ConfigBlueprintV1{
		{Key: "agent.provider.base_url", Mode: "literal", Value: &baseURL},
		{Key: "agent.provider.api_key", Mode: "secret_input"},
		{Key: "agent.provider.model", Mode: "literal", Value: &model},
	}
	snapshot.Automations = []projecttemplate.AutomationBlueprintV1{{
		Ref: "automation-1", Name: "巡检", TriggerType: "schedule",
		TriggerConfig: projecttemplate.AutomationTriggerV1{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:        projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", AllowedHostsConfigKey: "agent.provider.allowed_hosts"},
		Context:       projecttemplate.AutomationContextV1{Include: []string{}}, InstructionTemplate: "检查",
	}}
	seedProjectTemplate(t, f.owner, "host-null", snapshot)
	if err := f.owner.configRepo.Set(storage.ConfigKey{WorkspaceID: f.owner.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: f.owner.workspaceID, Key: "agent.provider.allowed_hosts"}, "null"); err != nil {
		t.Fatal(err)
	}
	input := instantiateTemplateInput(t, f.owner, "host-null", "hostnull")
	input.SecretInputs = map[string]string{"agent.provider.api_key": "secret"}
	preview, err := f.owner.PreviewProjectTemplateInstantiation("host-null", input)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTemplateIssue(preview.Issues, "project_template_automation_invalid") {
		t.Fatalf("null allowed-host config was accepted: %#v", preview)
	}
}

func TestProjectTemplateInstantiatePreviewSafelyDegradesInvalidCurrentSummary(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"unknown_field", `{"schema":"xuanchu.project-template-snapshot/v1","anchor_date":"2026-07-20","project":{"description":"invalid-current-must-not-leak"},"configs":[],"tasks":[],"series":[],"automations":[],"unknown":"invalid-current-must-not-leak"}`},
		{"unsupported_schema", `{"schema":"xuanchu.project-template-snapshot/v999","invalid":"invalid-current-must-not-leak"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProjectTemplateFixture(t)
			historicalSnapshot := instantiateSnapshot()
			historicalSnapshot.Configs = []projecttemplate.ConfigBlueprintV1{}
			seeded := seedProjectTemplate(t, f.owner, "invalid-current", historicalSnapshot)
			historical, err := f.owner.projectTemplateSnapshot(seeded, nil)
			if err != nil {
				t.Fatal(err)
			}
			currentSnapshot := historicalSnapshot
			currentSnapshot.Project.Description = "current"
			current := appendInstantiateSnapshot(t, f.owner, seeded, currentSnapshot)
			sum := sha256.Sum256([]byte(test.raw))
			if err := f.store.DB().Model(&storage.ProjectTemplateSnapshot{}).Where("id = ?", current.ID).Updates(map[string]any{"snapshot_json": test.raw, "snapshot_hash": fmt.Sprintf("%x", sum[:])}).Error; err != nil {
				t.Fatal(err)
			}
			input := InstantiateInput{SnapshotID: historical.ID, ExpectedHash: historical.SnapshotHash, ProjectSlug: "safeold", ProjectName: "历史项目", StartDate: "2026-08-01"}
			preview, err := f.owner.PreviewProjectTemplateInstantiation(seeded.Key, input)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Template.CurrentSnapshot == nil || preview.Template.CurrentSnapshot.ID != current.ID || preview.Template.CurrentSnapshot.Counts != (ComponentCounts{}) || len(preview.Template.CurrentSnapshot.RequiredSecretKeys) != 0 {
				t.Fatalf("degraded current summary = %#v", preview.Template.CurrentSnapshot)
			}
			if strings.Contains(mustJSON(preview), "invalid-current-must-not-leak") {
				t.Fatalf("invalid current content leaked: %s", mustJSON(preview))
			}
		})
	}
}

func TestProjectTemplateInstantiatePreviewTreatsBlankInheritedSecretsAsMissing(t *testing.T) {
	for _, test := range []struct {
		name    string
		source  string
		value   string
		project string
	}{
		{"workspace_empty", "workspace", "", "wsempty"},
		{"workspace_whitespace", "workspace", " \t ", "wsblank"},
		{"default_empty", "default", "", "defempty"},
		{"default_whitespace", "default", " \t ", "defblank"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProjectTemplateFixture(t)
			key := "template.inherited_secret"
			def := storage.ConfigDefinition{
				WorkspaceID: f.owner.workspaceID, Key: key, ValueType: "string",
				AllowedScopesJSON: `["project","workspace"]`, EnumValuesJSON: `[]`, Secret: true, CreatedAt: 1, ModifiedAt: 1,
			}
			if test.source == "default" {
				def.DefaultValue, def.HasDefault = test.value, true
			}
			if err := f.owner.configDefRepo.Set(def); err != nil {
				t.Fatal(err)
			}
			if test.source == "workspace" {
				if err := f.owner.configRepo.Set(storage.ConfigKey{WorkspaceID: f.owner.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: f.owner.workspaceID, Key: key}, test.value); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := instantiateSnapshot()
			snapshot.Configs = []projecttemplate.ConfigBlueprintV1{{Key: key, Mode: "secret_input"}}
			seedProjectTemplate(t, f.owner, "blank-secret", snapshot)
			input := instantiateTemplateInput(t, f.owner, "blank-secret", test.project)
			plan, err := f.owner.buildInstantiatePlan("blank-secret", input, false)
			if err != nil {
				t.Fatal(err)
			}
			if !hasTemplateIssue(plan.Preview.Issues, "project_template_secret_required") || plan.Preview.Counts.Configs != 0 || len(plan.Preview.SecretResolutions) != 1 || plan.Preview.SecretResolutions[0].ResolvedFrom != "missing" {
				t.Fatalf("blank inherited secret preview = %#v", plan.Preview)
			}
			if _, copied := plan.ConfigValues[key]; copied {
				t.Fatalf("blank inherited secret entered project rows: %#v", plan.ConfigValues)
			}
		})
	}
}

func appendInstantiateSnapshot(t *testing.T, svc *Service, template storage.ProjectTemplate, snapshot projecttemplate.SnapshotV1) storage.ProjectTemplateSnapshot {
	t.Helper()
	raw, hash, err := projecttemplate.EncodeV1(snapshot, projecttemplate.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	current, err := svc.projectTemplateSnapshot(template, nil)
	if err != nil {
		t.Fatal(err)
	}
	row := storage.ProjectTemplateSnapshot{ID: uuid.NewString(), SourceProjectID: current.SourceProjectID, SnapshotJSON: string(raw), SnapshotHash: hash, CreatedByActorType: actorTypeUser, CreatedAt: svc.clock.Unix()}
	if err := svc.store.Transaction(func(txStore *storage.Store) error {
		var appendErr error
		row, appendErr = storage.NewProjectTemplateRepository(txStore.DB()).AppendSnapshotLocked(template.WorkspaceID, template.ID, row)
		return appendErr
	}); err != nil {
		t.Fatal(err)
	}
	return row
}
