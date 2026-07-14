package app

import (
	"testing"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
)

// TestTaskBundleRoundTrip 验证 series + rule versions + 普通任务 + materialized occurrence
// + tombstone + overrides 能完整 round-trip（spec §20.1、§23.2）。
func TestTaskBundleRoundTrip(t *testing.T) {
	// 源 workspace。
	sourceSvc, closeFn := newTestService(t, 1000)
	defer closeFn()
	sourceStore := sourceSvc.store
	sourceWs, _ := sourceStore.LocalWorkspace()
	proj, err := sourceSvc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}

	// 创建 series（含 rule version history）。
	until := int64(5000)
	series, err := sourceSvc.taskSeriesRepo.Create(taskseries.Series{
		WorkspaceID: sourceWs.ID, ProjectID: proj.ID, Title: "每日巡检",
		Status: taskseries.StatusActive, RecurrenceRule: "daily", FirstDue: 1000,
		Until: &until, CreatedBy: "local", CreatedAt: 100, ModifiedAt: 100,
		Tags: []string{"ops"}, AssigneeIDs: []string{"local"},
	})
	if err != nil {
		t.Fatalf("Create series: %v", err)
	}
	if err := sourceSvc.taskSeriesRepo.AppendRuleVersion(series.ID, taskseries.RuleVersion{
		EffectiveFrom: 3000, RecurrenceRule: "weekly", CreatedBy: "local", CreatedAt: 200,
	}); err != nil {
		t.Fatalf("AppendRuleVersion: %v", err)
	}

	// materialized occurrence。
	slot := int64(1000)
	rule := "daily"
	seriesID := series.ID
	if _, _, err := sourceSvc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "occ-1", WorkspaceID: sourceWs.ID, Title: "每日巡检", Status: domain.StatusPending,
		Entry: 100, Modified: 100, ProjectID: &proj.ID, Due: &slot,
		SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		RecurrenceOverrides: []string{"due"},
	}); err != nil {
		t.Fatalf("CreateOccurrence: %v", err)
	}

	// tombstone。
	tombSlot := int64(2000)
	if _, _, err := sourceSvc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "tomb-1", WorkspaceID: sourceWs.ID, Title: "每日巡检", Status: domain.StatusDeleted,
		Entry: 150, Modified: 150, ProjectID: &proj.ID,
		SeriesID: &seriesID, RecurrenceAt: &tombSlot, RecurrenceRuleSnapshot: &rule,
	}); err != nil {
		t.Fatalf("CreateOccurrence tombstone: %v", err)
	}

	// 普通任务。
	projectSlug := proj.Slug
	projectSeq := int64(999)
	if _, err := sourceSvc.repo.Create(domain.Task{
		UUID: "plain-1", WorkspaceID: sourceWs.ID, Title: "普通任务", Status: domain.StatusPending,
		Entry: 200, Modified: 200, Project: &projectSlug, ProjectID: &proj.ID, ProjectSeq: &projectSeq,
		Assignees:   []domain.AssigneeInfo{{UserID: "local"}},
		Annotations: []domain.Annotation{{ID: "annotation-1", Entry: 201, Description: "保留注解"}},
		Depends:     []string{"external-dependency"},
		UDAs: map[string]domain.UDAValue{
			"estimate": {Name: "estimate", Raw: "3", Type: "numeric", Orphan: true},
		},
	}); err != nil {
		t.Fatalf("Create plain task: %v", err)
	}
	if _, err := sourceSvc.TaskAddLink("plain-1", "document", "https://example.com/spec", "规格"); err != nil {
		t.Fatalf("TaskAddLink plain task: %v", err)
	}

	// 导出。
	bundle, err := sourceSvc.ExportTaskBundle()
	if err != nil {
		t.Fatalf("ExportTaskBundle: %v", err)
	}
	if bundle.Schema != TaskBundleSchemaV1 {
		t.Fatalf("schema = %q want %s", bundle.Schema, TaskBundleSchemaV1)
	}
	if len(bundle.TaskSeries) != 1 {
		t.Fatalf("series count = %d want 1", len(bundle.TaskSeries))
	}
	if bundle.TaskSeries[0].Project != "ops" {
		t.Fatalf("series project = %q want ops", bundle.TaskSeries[0].Project)
	}
	if len(bundle.TaskSeries[0].RuleVersions) != 2 {
		t.Fatalf("rule versions = %d want 2", len(bundle.TaskSeries[0].RuleVersions))
	}
	if len(bundle.Tasks) != 3 {
		t.Fatalf("tasks count = %d want 3 (1 occurrence + 1 tombstone + 1 plain)", len(bundle.Tasks))
	}
	var plainBundle TaskBundleTask
	for _, bundledTask := range bundle.Tasks {
		if bundledTask.UUID == "plain-1" {
			plainBundle = bundledTask
		}
	}
	if len(plainBundle.AssigneeIDs) != 1 || plainBundle.AssigneeIDs[0] != "local" {
		t.Fatalf("plain task assignees = %#v want [local]", plainBundle.AssigneeIDs)
	}

	// projected occurrence 不应出现在 bundle 中（本测试未创建 projected，但验证无多余行）。

	// 序列化/反序列化 round-trip。
	data, err := MarshalTaskBundle(bundle)
	if err != nil {
		t.Fatalf("MarshalTaskBundle: %v", err)
	}
	restored, err := UnmarshalTaskBundle(data)
	if err != nil {
		t.Fatalf("UnmarshalTaskBundle: %v", err)
	}
	if len(restored.TaskSeries) != 1 || len(restored.Tasks) != 3 {
		t.Fatalf("restored counts mismatch: series=%d tasks=%d", len(restored.TaskSeries), len(restored.Tasks))
	}

	// 导入到目标 workspace。
	targetSvc, targetClose := newTestService(t, 1000)
	defer targetClose()
	targetStore := targetSvc.store
	targetWs, _ := targetStore.LocalWorkspace()
	// 目标 workspace 只需有同 slug project；导入会重绑定不同的 project ID。
	targetProj, err := targetSvc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if restored.TaskSeries[0].ProjectID == targetProj.ID {
		t.Fatal("测试前提错误：源和目标 project ID 应不同")
	}
	result, err := targetSvc.ImportTaskBundle(restored)
	if err != nil {
		t.Fatalf("ImportTaskBundle: %v", err)
	}
	if result.SeriesImported != 1 {
		t.Fatalf("series imported = %d want 1", result.SeriesImported)
	}
	if result.TasksImported != 3 {
		t.Fatalf("tasks imported = %d want 3", result.TasksImported)
	}

	// 验证导入后的 occurrence 槽位和 overrides 保留。
	gotOcc, err := targetSvc.taskOccurrenceRepo.GetOccurrence(targetWs.ID, seriesID, slot)
	if err != nil {
		t.Fatalf("GetOccurrence after import: %v", err)
	}
	if gotOcc.UUID != "occ-1" {
		t.Fatalf("occ uuid = %q want occ-1", gotOcc.UUID)
	}
	if len(gotOcc.RecurrenceOverrides) != 1 || gotOcc.RecurrenceOverrides[0] != "due" {
		t.Fatalf("overrides = %#v want [due]", gotOcc.RecurrenceOverrides)
	}
	gotPlain, err := targetSvc.repo.GetByUUID(targetWs.ID, "plain-1")
	if err != nil {
		t.Fatalf("GetByUUID plain task after import: %v", err)
	}
	if len(gotPlain.Assignees) != 1 || gotPlain.Assignees[0].UserID != "local" {
		t.Fatalf("plain task assignees after import = %#v want local", gotPlain.Assignees)
	}
	if len(gotPlain.Annotations) != 1 || gotPlain.Annotations[0].Description != "保留注解" {
		t.Fatalf("plain task annotations after import = %#v", gotPlain.Annotations)
	}
	if len(gotPlain.Depends) != 1 || gotPlain.Depends[0] != "external-dependency" {
		t.Fatalf("plain task depends after import = %#v", gotPlain.Depends)
	}
	if gotPlain.UDAs["estimate"].Raw != "3" || !gotPlain.UDAs["estimate"].Orphan {
		t.Fatalf("plain task UDAs after import = %#v", gotPlain.UDAs)
	}
	if len(gotPlain.Links) != 1 || gotPlain.Links[0].URL != "https://example.com/spec" {
		t.Fatalf("plain task links after import = %#v", gotPlain.Links)
	}
}

func TestUnmarshalTaskBundleRejectsUnknownSchema(t *testing.T) {
	if _, err := UnmarshalTaskBundle([]byte(`{"schema":"unknown/v2","task_series":[],"tasks":[]}`)); err == nil {
		t.Fatal("未知 schema 应被拒绝")
	}
}

func TestImportTaskBundleRejectsMissingSeries(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	seriesID := "nonexistent-series"
	bundle := TaskBundleV1{
		Schema: TaskBundleSchemaV1,
		Tasks: []TaskBundleTask{
			{
				UUID: "orphan-occ", WorkspaceID: svc.workspaceID, Title: "orphan",
				Status: domain.StatusPending, Entry: 1, Modified: 1,
				SeriesID: &seriesID, RecurrenceAt: ptrInt64(100), RecurrenceRuleSnapshot: strptr("daily"),
			},
		},
	}
	_, err := svc.ImportTaskBundle(bundle)
	if err == nil {
		t.Fatal("引用不存在 series 的 occurrence 应被拒绝")
	}
}

func TestExportTaskBundleHonorsProjectScope(t *testing.T) {
	store := newTestStore(t)
	base := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	allowed, err := base.AddProject(AddProjectInput{Slug: "allowed", Name: "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := base.AddProject(AddProjectInput{Slug: "foreign", Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base.AddTaskSeries(AddTaskSeriesInput{
		Title: "允许的循环任务", ProjectID: allowed.ID, RecurrenceRule: "daily", FirstDue: 5000,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.AddTaskSeries(AddTaskSeriesInput{
		Title: "不可见循环任务", ProjectID: foreign.ID, RecurrenceRule: "daily", FirstDue: 5000,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Add(AddInput{Title: "允许的普通任务", Project: &allowed.Slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Add(AddInput{Title: "不可见普通任务", Project: &foreign.Slug}); err != nil {
		t.Fatal(err)
	}

	scope := RequestScope{ProjectIDs: []string{allowed.ID}, Capabilities: []string{"task:read"}}
	scoped, err := NewService(ServiceOptions{
		Store: store, Clock: FixedClock{NowUnix: 1000}, RequestScope: &scope,
		Runtime:               &RuntimeContext{ActorType: "user", ActorUserID: "local", WorkspaceID: base.workspaceID, Role: RoleOwner},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := scoped.ExportTaskBundle()
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.TaskSeries) != 1 || bundle.TaskSeries[0].ProjectID != allowed.ID {
		t.Fatalf("exported series = %#v want only allowed project", bundle.TaskSeries)
	}
	if len(bundle.Tasks) != 2 {
		t.Fatalf("exported tasks count = %d want allowed ordinary + materialized occurrence", len(bundle.Tasks))
	}
	for _, bundledTask := range bundle.Tasks {
		if bundledTask.ProjectID == nil || *bundledTask.ProjectID != allowed.ID {
			t.Fatalf("exported task = %#v want only allowed project", bundledTask)
		}
	}
}

func TestImportTaskBundleRejectsOccurrenceProjectMismatchAndRollsBack(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	seriesProject, err := svc.AddProject(AddProjectInput{Slug: "series", Name: "Series"})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, err := svc.AddProject(AddProjectInput{Slug: "other", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	seriesID := "bundle-series"
	recurrenceAt := int64(5000)
	rule := "daily"
	bundle := TaskBundleV1{
		Schema: TaskBundleSchemaV1,
		TaskSeries: []TaskSeriesBundle{{
			ID: seriesID, WorkspaceID: "foreign-workspace", ProjectID: seriesProject.ID,
			Title: "每日巡检", Status: taskseries.StatusActive, RecurrenceRule: rule,
			FirstDue: recurrenceAt, CreatedBy: "local", CreatedAt: 1, ModifiedAt: 1,
		}},
		Tasks: []TaskBundleTask{{
			UUID: "mismatched-occurrence", WorkspaceID: "foreign-workspace", ProjectID: &otherProject.ID,
			Title: "每日巡检", Status: domain.StatusPending, Entry: 1, Modified: 1,
			SeriesID: &seriesID, RecurrenceAt: &recurrenceAt, RecurrenceRuleSnapshot: &rule,
		}},
	}

	if _, err := svc.ImportTaskBundle(bundle); err == nil {
		t.Fatal("occurrence 与 series project 不一致应被拒绝")
	}
	if _, err := svc.taskSeriesRepo.Get(svc.workspaceID, seriesID); err == nil {
		t.Fatal("导入失败后先创建的 series 必须回滚")
	}
	if _, err := svc.repo.GetByUUID(svc.workspaceID, "mismatched-occurrence"); err == nil {
		t.Fatal("导入失败后 occurrence 不得残留")
	}
}
