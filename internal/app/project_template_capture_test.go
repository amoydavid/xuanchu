package app

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
)

func captureFixture(t *testing.T) (*Service, ProjectView) {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	f := newProjectTemplateFixture(t)
	f.owner.clock = FixedClock{NowUnix: time.Date(2026, 7, 20, 12, 0, 0, 0, loc).Unix(), Loc: loc}
	project, err := f.owner.AddProject(AddProjectInput{Slug: "capsrc", Name: "模板来源", Description: "源项目说明"})
	if err != nil {
		t.Fatal(err)
	}
	return f.owner, project
}

func completeCaptureInput(projectRef, anchorDate string, selection CaptureSelection) CaptureInput {
	return CaptureInput{
		SourceProjectRef: projectRef,
		AnchorDate:       anchorDate,
		Selection:        selection,
		SelectionPresence: SelectionPresence{
			ConfigKeys: true, TaskRefs: true, SeriesRefs: true, AutomationRuleIDs: true,
		},
	}
}

func seedCaptureTask(t *testing.T, svc *Service, project ProjectView, title string, seq int64) task.Task {
	t.Helper()
	projectSlug, projectID := project.Slug, project.ID
	row, err := svc.repo.Create(task.Task{
		UUID: uuid.NewString(), WorkspaceID: project.WorkspaceID, Title: title, Status: task.StatusPending,
		Entry: int64(seq), Modified: int64(seq), Project: &projectSlug, ProjectID: &projectID, ProjectSeq: &seq,
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestCapturePreviewRequiresAllSelectionPresence(t *testing.T) {
	svc, project := captureFixture(t)
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{})
	input.SelectionPresence.AutomationRuleIDs = false
	_, err := svc.PreviewProjectTemplateCapture(input)
	if runtimeCode(err) != "project_template_selection_invalid" {
		t.Fatalf("error = %v", err)
	}
}

func TestCapturePreviewAndCreateUseOnlyExplicitSelection(t *testing.T) {
	svc, project := captureFixture(t)
	keep := seedCaptureTask(t, svc, project, "保留", 2)
	omitted := seedCaptureTask(t, svc, project, "忽略", 1)
	parent := seedCaptureTask(t, svc, project, "父任务", 3)
	keep.Parent = &parent.UUID
	keep.Status = task.StatusCompleted
	start := svc.clock.Unix()
	keep.Start = &start
	keep.Annotations = []task.Annotation{{ID: uuid.NewString(), Entry: start, Description: "不复制"}}
	if err := svc.repo.Update(keep); err != nil {
		t.Fatal(err)
	}

	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{parent.UUID, keep.UUID, keep.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Counts.Tasks != 2 || strings.Contains(string(mustJSON(preview)), omitted.UUID) {
		t.Fatalf("preview = %#v", preview)
	}
	input.ExpectedSourceHash = preview.SourceHash
	got, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "launch", Name: " 启动流程 ", Description: " 说明 ", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	if got.Template.Name != "启动流程" || got.Template.Description != "说明" || got.Template.CurrentSnapshot == nil || got.Template.CurrentSnapshot.Counts.Tasks != 2 {
		t.Fatalf("got = %#v", got)
	}
	if got.Snapshot == nil || got.Snapshot.Tasks[0].Ref != "task-1" || got.Snapshot.Tasks[0].Title != "保留" || got.Snapshot.Tasks[0].ParentRef == nil || *got.Snapshot.Tasks[0].ParentRef != "task-2" {
		t.Fatalf("snapshot tasks = %#v", got.Snapshot)
	}
}

func TestCaptureResolutionMustExactlyMatchPreviewIssues(t *testing.T) {
	svc, project := captureFixture(t)
	selected := seedCaptureTask(t, svc, project, "选中", 1)
	missing := seedCaptureTask(t, svc, project, "未选", 2)
	selected.Depends = []string{missing.UUID}
	description := "[未选任务](ref://task/" + missing.UUID + ")"
	selected.Description = &description
	if err := svc.repo.Update(selected); err != nil {
		t.Fatal(err)
	}
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{selected.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BlockingIssues) != 2 {
		t.Fatalf("issues = %#v", preview.BlockingIssues)
	}
	input.ExpectedSourceHash = preview.SourceHash
	input.Resolution = CaptureResolution{
		DropDepends:         []TaskRelationResolution{{SourceTaskRef: selected.UUID, Relation: "depends", TargetTaskRef: missing.UUID}},
		DropContentTaskRefs: []ContentRefResolution{{SourceKind: "task", SourceRef: selected.UUID, TargetTaskRef: missing.UUID}},
	}
	created, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "refs-ok", Name: "引用处理", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Snapshot.Tasks[0].DependsRefs) != 0 || strings.Contains(*created.Snapshot.Tasks[0].Description, "ref://task/") {
		t.Fatalf("resolved task = %#v", created.Snapshot.Tasks[0])
	}

	input.Resolution.DropDepends[0].TargetTaskRef = selected.UUID
	_, err = svc.CreateProjectTemplate(CreateTemplateInput{Key: "refs-bad", Name: "非法引用处理", Capture: input})
	if runtimeCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("unexpected resolution error = %v", err)
	}
}

func TestCaptureAttachmentBlocksAndMarkdownCodeBlockIsIgnored(t *testing.T) {
	svc, project := captureFixture(t)
	row := seedCaptureTask(t, svc, project, "附件", 1)
	attachmentID := uuid.NewString()
	description := "`[代码](ref://attachment/" + attachmentID + ")`\n\n[真实附件](ref://attachment/" + attachmentID + ")"
	row.Description = &description
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewProjectTemplateCapture(completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BlockingIssues) != 1 || preview.BlockingIssues[0].Code != "project_template_attachment_unsupported" {
		t.Fatalf("issues = %#v", preview.BlockingIssues)
	}
}

func TestCaptureRejectsChangedSourceAndNeverPersistsSecret(t *testing.T) {
	svc, project := captureFixture(t)
	const key, secret = "capture.secret", "sk-capture-plain-text"
	if err := svc.configDefRepo.Set(storage.ConfigDefinition{WorkspaceID: project.WorkspaceID, Key: key, ValueType: "string", AllowedScopesJSON: `["project"]`, Secret: true, CreatedAt: 1, ModifiedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := svc.configRepo.Set(storage.ConfigKey{WorkspaceID: project.WorkspaceID, Scope: storage.ConfigScopeProject, ScopeID: project.ID, Key: key}, secret); err != nil {
		t.Fatal(err)
	}
	row := seedCaptureTask(t, svc, project, "会变化", 1)
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{ConfigKeys: []string{key}, TaskRefs: []string{row.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustJSON(preview)), secret) {
		t.Fatalf("preview leaked secret: %s", mustJSON(preview))
	}
	row.Title = "已变化"
	row.Modified++
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	if _, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "drift", Name: "漂移", Capture: input}); runtimeCode(err) != "project_template_source_changed" {
		t.Fatalf("drift error = %v", err)
	}

	preview, err = svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	created, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "secret-safe", Name: "密钥安全", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustJSON(created)), secret) {
		t.Fatalf("view leaked secret: %s", mustJSON(created))
	}
	var snapshot storage.ProjectTemplateSnapshot
	if err := svc.store.DB().Where("id = ?", created.Template.CurrentSnapshot.ID).First(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot.SnapshotJSON, secret) || !strings.Contains(snapshot.SnapshotJSON, `"mode":"secret_input"`) {
		t.Fatalf("snapshot = %s", snapshot.SnapshotJSON)
	}
	var audits []storage.AuditLog
	if err := svc.store.DB().Where("target_id IN ?", []string{created.Template.ID, created.Template.CurrentSnapshot.ID}).Find(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if len(audits) != 2 || strings.Contains(string(mustJSON(audits)), secret) {
		t.Fatalf("audits = %#v", audits)
	}
}

func TestCaptureSeriesUsesCurrentRuleWithoutHistoryAndNeedsStoppedConfirmation(t *testing.T) {
	svc, project := captureFixture(t)
	loc := svc.clock.Location()
	first := time.Date(2026, 7, 1, 23, 59, 59, 0, loc).Unix()
	series, err := svc.taskSeriesRepo.Create(taskseries.Series{
		ID: uuid.NewString(), WorkspaceID: project.WorkspaceID, ProjectID: project.ID, ProjectSeq: int64Ptr(1),
		Title: "周报", Status: taskseries.StatusStopped, RecurrenceRule: "daily", FirstDue: first,
		CreatedBy: svc.Runtime().ActorUserID, CreatedAt: 1, ModifiedAt: 2,
		RuleVersions: []taskseries.RuleVersion{
			{ID: uuid.NewString(), EffectiveFrom: first, RecurrenceRule: "weekly", CreatedBy: svc.Runtime().ActorUserID, CreatedAt: 1},
			{ID: uuid.NewString(), EffectiveFrom: first + 7*86400, RecurrenceRule: "daily", CreatedBy: svc.Runtime().ActorUserID, CreatedAt: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{SeriesRefs: []string{series.ID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BlockingIssues) != 1 || preview.BlockingIssues[0].Code != "project_template_series_schedule_confirmation_required" {
		t.Fatalf("issues = %#v", preview.BlockingIssues)
	}
	input.ExpectedSourceHash = preview.SourceHash
	if _, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "series-no-confirm", Name: "未确认", Capture: input}); runtimeCode(err) != "project_template_series_schedule_confirmation_required" {
		t.Fatalf("error = %v", err)
	}
	input.Resolution.SeriesScheduleOverrides = []SeriesScheduleOverride{{SourceSeriesRef: series.ID, FirstDue: projecttemplate.RelativeLocalTimeV1{DayOffset: 0, LocalTime: "23:59:59"}, ClearUntil: true}}
	created, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "series-confirm", Name: "已确认", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	if created.Snapshot.Series[0].RecurrenceRule != "daily" || created.Snapshot.Series[0].FirstDue.DayOffset != 0 || created.Snapshot.Series[0].Until != nil {
		t.Fatalf("series snapshot = %#v", created.Snapshot.Series[0])
	}
	if strings.Contains(string(mustJSON(created)), "weekly") {
		t.Fatalf("series history leaked: %s", mustJSON(created))
	}
}

func TestCaptureAutomationMapsTypedDefinitionAndIgnoresEnabled(t *testing.T) {
	svc, project := captureFixture(t)
	enabled := true
	rule := storage.ProjectAutomationRule{
		ID: uuid.NewString(), WorkspaceID: project.WorkspaceID, ProjectID: project.ID, Name: "自动巡检", Enabled: &enabled,
		TriggerType: "schedule", TriggerConfigJSON: `{"schedule_type":"daily_at","schedule_value":"09:00","timezone":"Asia/Shanghai"}`,
		ConditionJSON: `{"max_tasks":10}`, ActionType: ProjectAutomationActionOpenAI,
		ActionConfigJSON:  `{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2}`,
		ContextConfigJSON: `{"include":["project"]}`, InstructionTemplate: "检查项目", CreatedAt: 1, ModifiedAt: 1,
	}
	if err := svc.projectAutomationRuleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{AutomationRuleIDs: []string{rule.ID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	created, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "automation", Name: "自动化", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	raw := string(mustJSON(created.Snapshot.Automations[0]))
	if strings.Contains(raw, "enabled") || strings.Contains(raw, "delivery") || !strings.Contains(raw, `"trigger_config"`) {
		t.Fatalf("automation = %s", raw)
	}
}

func TestProjectTemplateSnapshotAppendIsIdempotentAndRejectsHistoricalDuplicate(t *testing.T) {
	svc, project := captureFixture(t)
	row := seedCaptureTask(t, svc, project, "v1", 1)
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	created, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "versions", Name: "版本", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	current, err := svc.CreateProjectTemplateSnapshot(created.Template.ID, input)
	if err != nil || current.Template.CurrentSnapshot.Version != 1 {
		t.Fatalf("idempotent append = %#v, %v", current, err)
	}
	var idempotentAuditCount int64
	if err := svc.store.DB().Model(&storage.AuditLog{}).Where("target_id IN ?", []string{created.Template.ID, created.Template.CurrentSnapshot.ID}).Count(&idempotentAuditCount).Error; err != nil || idempotentAuditCount != 2 {
		t.Fatalf("idempotent audit count = %d, err=%v", idempotentAuditCount, err)
	}

	row.Title = "v2"
	row.Modified++
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	v2preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = v2preview.SourceHash
	v2, err := svc.CreateProjectTemplateSnapshot(created.Template.ID, input)
	if err != nil || v2.Template.CurrentSnapshot.Version != 2 {
		t.Fatalf("v2 = %#v, %v", v2, err)
	}

	row.Title = "v1"
	row.Modified--
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	v1again, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = v1again.SourceHash
	_, err = svc.CreateProjectTemplateSnapshot(created.Template.ID, input)
	if runtimeCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("historical duplicate error = %v", err)
	}
	after, err := svc.ProjectTemplateInfo(created.Template.ID, nil)
	if err != nil || after.Template.CurrentSnapshot.Version != 2 {
		t.Fatalf("current after duplicate = %#v, %v", after, err)
	}
}

func TestCaptureSourceHashUsesCanonicalSourceOrderAndTracksMembership(t *testing.T) {
	svc, project := captureFixture(t)
	second := seedCaptureTask(t, svc, project, "第二", 2)
	first := seedCaptureTask(t, svc, project, "第一", 1)
	first.Assignees = []task.AssigneeInfo{{UserID: svc.Runtime().ActorUserID}}
	if err := svc.repo.Update(first); err != nil {
		t.Fatal(err)
	}
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{second.UUID, first.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Selection.TaskRefs) != 2 || preview.Selection.TaskRefs[0] != first.UUID || preview.Selection.TaskRefs[1] != second.UUID {
		t.Fatalf("normalized selection = %#v", preview.Selection)
	}
	if err := svc.memberRepo.UpdateRole(svc.Runtime().ActorUserID, project.WorkspaceID, "admin", svc.clock.Unix()+1); err != nil {
		t.Fatal(err)
	}
	changed, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SourceHash == preview.SourceHash {
		t.Fatal("membership change did not change source hash")
	}
}

func TestCaptureTaskDateOverrideMustMatchPreviewAndKeepsSourceHash(t *testing.T) {
	svc, project := captureFixture(t)
	row := seedCaptureTask(t, svc, project, "日期", 1)
	due := time.Date(2026, 7, 19, 23, 59, 59, 0, svc.clock.Location()).Unix()
	row.Due = &due
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Warnings) != 1 || preview.Warnings[0].Code != "project_template_date_before_anchor" {
		t.Fatalf("warnings = %#v", preview.Warnings)
	}
	override := projecttemplate.RelativeLocalTimeV1{DayOffset: 2, LocalTime: "23:59:59"}
	input.Resolution.TaskDateOverrides = []TaskDateOverride{{SourceTaskRef: row.UUID, Field: "due", Value: &override}}
	resolved, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SourceHash != preview.SourceHash || resolved.Snapshot.Tasks[0].Dates.Due == nil || resolved.Snapshot.Tasks[0].Dates.Due.DayOffset != 2 {
		t.Fatalf("resolved = %#v", resolved)
	}
	input.Resolution.TaskDateOverrides[0].Field = "wait"
	if _, err := svc.PreviewProjectTemplateCapture(input); runtimeCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("unmatched override error = %v", err)
	}
}

func TestCaptureUserReferenceRequiresCurrentWorkspaceMemberAndIgnoresCode(t *testing.T) {
	svc, project := captureFixture(t)
	row := seedCaptureTask(t, svc, project, "用户引用", 1)
	missing := uuid.NewString()
	description := "`[代码](ref://user/" + missing + ")`\n\n[负责人](ref://user/" + svc.Runtime().ActorUserID + ")"
	row.Description = &description
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewProjectTemplateCapture(completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BlockingIssues) != 0 {
		t.Fatalf("code block user ref became issue: %#v", preview.BlockingIssues)
	}
	description = "[离开成员](ref://user/" + missing + ")"
	row.Description = &description
	row.Modified++
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	preview, err = svc.PreviewProjectTemplateCapture(completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BlockingIssues) != 1 || preview.BlockingIssues[0].Code != "project_template_member_unavailable" || preview.BlockingIssues[0].User == nil || preview.BlockingIssues[0].User.ID != missing {
		t.Fatalf("issues = %#v", preview.BlockingIssues)
	}
	raw := string(mustJSON(preview.BlockingIssues[0]))
	if strings.Contains(raw, `"user_id"`) || !strings.Contains(raw, `"user":{"id":"`+missing+`"`) {
		t.Fatalf("issue user shape = %s", raw)
	}
}

func TestCaptureStoppedSeriesWithFutureSlotDoesNotRequireFallbackConfirmation(t *testing.T) {
	svc, project := captureFixture(t)
	first := time.Date(2026, 7, 20, 23, 59, 59, 0, svc.clock.Location()).Unix()
	until := time.Date(2026, 7, 24, 23, 59, 59, 0, svc.clock.Location()).Unix()
	end := time.Date(2026, 7, 25, 0, 0, 0, 0, svc.clock.Location()).Unix()
	series, err := svc.taskSeriesRepo.Create(taskseries.Series{
		ID: uuid.NewString(), WorkspaceID: project.WorkspaceID, ProjectID: project.ID, ProjectSeq: int64Ptr(1),
		Title: "历史范围", Status: taskseries.StatusStopped, RecurrenceRule: "daily", FirstDue: first, Until: &until, EffectiveEndAt: &end,
		CreatedBy: svc.Runtime().ActorUserID, CreatedAt: 1, ModifiedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewProjectTemplateCapture(completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{SeriesRefs: []string{series.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BlockingIssues) != 0 || preview.Snapshot.Series[0].FirstDue.DayOffset != 0 || preview.Snapshot.Series[0].Until == nil {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestCreateProjectTemplateValidatesKeyAndRollsBackAuditFailure(t *testing.T) {
	svc, project := captureFixture(t)
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	if _, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "Bad", Name: "非法", Capture: input}); runtimeCode(err) != "project_template_key_invalid" {
		t.Fatalf("key error = %v", err)
	}

	svc.auditRepo = failingCaptureAuditRepository{AuditRepository: storage.NewAuditRepository(svc.store.DB())}
	_, err = svc.CreateProjectTemplate(CreateTemplateInput{Key: "rollback", Name: "回滚", Capture: input})
	if err == nil {
		t.Fatal("expected audit failure")
	}
	var count int64
	if dbErr := svc.store.DB().Model(&storage.ProjectTemplate{}).Where("key = ?", "rollback").Count(&count).Error; dbErr != nil || count != 0 {
		t.Fatalf("template count = %d, err=%v", count, dbErr)
	}
}

func TestProjectTemplateSnapshotAppendRejectsArchivedStateReadUnderTransactionLock(t *testing.T) {
	svc, project := captureFixture(t)
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	created, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "locked-archive", Name: "锁后归档", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ArchiveProjectTemplate(created.Template.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := svc.withStore(txStore)
		if err != nil {
			return err
		}
		locked, err := txSvc.lockProjectTemplateForSnapshotAppend(created.Template.ID)
		if err != nil {
			return err
		}
		if locked.Status != "archived" {
			t.Fatalf("locked status = %q", locked.Status)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProjectTemplateSnapshot(created.Template.ID, input); runtimeCode(err) != "project_template_archived" {
		t.Fatalf("append archived error = %v", err)
	}
	var versions int64
	if err := svc.store.DB().Model(&storage.ProjectTemplateSnapshot{}).Where("template_id = ?", created.Template.ID).Count(&versions).Error; err != nil || versions != 1 {
		t.Fatalf("versions = %d, err=%v", versions, err)
	}
}

func TestCreateProjectTemplateRechecksSourceInsideWriteTransaction(t *testing.T) {
	svc, project := captureFixture(t)
	row := seedCaptureTask(t, svc, project, "事务前", 1)
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash

	updated := make(chan struct{})
	release := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- svc.store.Transaction(func(txStore *storage.Store) error {
			if err := txStore.DB().Model(&storage.Task{}).Where("uuid = ?", row.UUID).Updates(map[string]any{"title": "事务中变化", "modified": row.Modified + 1}).Error; err != nil {
				return err
			}
			close(updated)
			<-release
			return nil
		})
	}()
	<-updated
	createDone := make(chan error, 1)
	go func() {
		_, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "tx-source", Name: "事务源", Capture: input})
		createDone <- err
	}()
	// 给 Capture 足够时间进入其事务读取路径；源 writer 仍持有 SQLite writer lock。
	time.Sleep(100 * time.Millisecond)
	close(release)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-createDone; runtimeCode(err) != "project_template_source_changed" {
		t.Fatalf("capture error = %v", err)
	}
	var templates int64
	if err := svc.store.DB().Model(&storage.ProjectTemplate{}).Where("key = ?", "tx-source").Count(&templates).Error; err != nil || templates != 0 {
		t.Fatalf("templates = %d, err=%v", templates, err)
	}
}

func TestCaptureSourceHashNormalizesHydratedUsersAndAutomationJSON(t *testing.T) {
	svc, project := captureFixture(t)
	row := seedCaptureTask(t, svc, project, "稳定 hash", 1)
	row.Assignees = []task.AssigneeInfo{{UserID: svc.Runtime().ActorUserID}}
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	enabled := true
	rule := storage.ProjectAutomationRule{
		ID: uuid.NewString(), WorkspaceID: project.WorkspaceID, ProjectID: project.ID, Name: "稳定自动化", Enabled: &enabled,
		TriggerType: "schedule", TriggerConfigJSON: `{"schedule_value":"09:00","schedule_type":"daily_at","timezone":"Asia/Shanghai"}`,
		ConditionJSON: `{"max_tasks":10,"task_filter":"status:pending"}`, ActionType: ProjectAutomationActionOpenAI,
		ActionConfigJSON:  `{"temperature":0.2,"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model"}`,
		ContextConfigJSON: `{"include":["project"]}`, InstructionTemplate: "检查", CreatedAt: 1, ModifiedAt: 1,
	}
	if err := svc.projectAutomationRuleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}, AutomationRuleIDs: []string{rule.ID}})
	before, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.store.DB().Model(&storage.User{}).Where("id = ?", svc.Runtime().ActorUserID).Update("display_name", "只改展示名").Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.store.DB().Model(&storage.ProjectAutomationRule{}).Where("id = ?", rule.ID).Updates(map[string]any{
		"trigger_config_json": `{"timezone":"Asia/Shanghai","schedule_type":"daily_at","schedule_value":"09:00"}`,
		"condition_json":      `{"task_filter":"status:pending","max_tasks":10}`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	after, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	if after.SourceHash != before.SourceHash {
		t.Fatalf("equivalent normalized source changed hash: before=%s after=%s", before.SourceHash, after.SourceHash)
	}
}

func TestCaptureContentDropDoesNotRewriteSentinelTextOutsideDestination(t *testing.T) {
	svc, project := captureFixture(t)
	row := seedCaptureTask(t, svc, project, "精确正文 drop", 1)
	missing := uuid.NewString()
	description := "`ref://task/task-2147483647`\n\n[缺失](ref://task/" + missing + ")"
	row.Description = &description
	if err := svc.repo.Update(row); err != nil {
		t.Fatal(err)
	}
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	input.Resolution.DropContentTaskRefs = []ContentRefResolution{{SourceKind: "task", SourceRef: row.UUID, TargetTaskRef: missing}}
	created, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "drop-exact", Name: "精确 drop", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	if created.Snapshot == nil || created.Snapshot.Tasks[0].Description == nil || !strings.Contains(*created.Snapshot.Tasks[0].Description, "ref://task/task-2147483647") {
		t.Fatalf("description = %#v", created.Snapshot)
	}
}

type failingCaptureAuditRepository struct{ *storage.AuditRepository }

func (f failingCaptureAuditRepository) Append(storage.AuditLogEntry) error {
	return errors.New("capture audit failed")
}

func TestCapturePreviewJSONShapeNeverContainsRawSnapshot(t *testing.T) {
	svc, project := captureFixture(t)
	preview, err := svc.PreviewProjectTemplateCapture(completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{}))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "snapshot_json") {
		t.Fatalf("preview leaked raw snapshot: %s", raw)
	}
}

func TestProjectTemplateCaptureRecognizesPostgresSerializationFailure(t *testing.T) {
	if !isProjectTemplateCaptureSerializationFailure(&pgconn.PgError{Code: "40001", Message: "could not serialize access due to concurrent update"}) {
		t.Fatal("SQLSTATE 40001 was not recognized")
	}
	if isProjectTemplateCaptureSerializationFailure(&pgconn.PgError{Code: "23505", Message: "unique violation"}) {
		t.Fatal("non-serialization PostgreSQL error was misclassified")
	}
}

func TestProjectTemplateCapturePostgresConcurrentSourceUpdateReturnsStableCode(t *testing.T) {
	dbURL := os.Getenv("XUANCHU_TEST_DB_URL")
	if dbURL == "" {
		t.Skip("XUANCHU_TEST_DB_URL not set, skipping PostgreSQL concurrency test")
	}
	store, err := storage.Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: time.Date(2026, 7, 20, 12, 0, 0, 0, loc).Unix(), Loc: loc}})
	if err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	project, err := svc.AddProject(AddProjectInput{Slug: "p" + suffix, Name: "PG Capture 并发"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.DB().Where("project_id = ?", project.ID).Delete(&storage.AuditLog{})
		store.DB().Where("project_id = ?", project.ID).Delete(&storage.Task{})
		store.DB().Where("id = ?", project.ID).Delete(&storage.Project{})
	})
	row := seedCaptureTask(t, svc, project, "PG 事务前", 1)
	input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{row.UUID}})
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash

	updated := make(chan struct{})
	release := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- store.Transaction(func(txStore *storage.Store) error {
			if err := txStore.DB().Model(&storage.Task{}).Where("uuid = ?", row.UUID).Updates(map[string]any{"title": "PG 事务中变化", "modified": row.Modified + 1}).Error; err != nil {
				return err
			}
			close(updated)
			<-release
			return nil
		})
	}()
	<-updated
	createDone := make(chan error, 1)
	go func() {
		_, err := svc.CreateProjectTemplate(CreateTemplateInput{Key: "pg-" + suffix, Name: "PG 并发模板", Capture: input})
		createDone <- err
	}()
	time.Sleep(100 * time.Millisecond)
	close(release)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-createDone; runtimeCode(err) != "project_template_source_changed" {
		t.Fatalf("capture error = %v", err)
	}
}
