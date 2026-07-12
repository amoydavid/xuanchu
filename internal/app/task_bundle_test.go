package app

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
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
	if _, err := sourceSvc.repo.Create(domain.Task{
		UUID: "plain-1", WorkspaceID: sourceWs.ID, Title: "普通任务", Status: domain.StatusPending,
		Entry: 200, Modified: 200, ProjectID: &proj.ID,
	}); err != nil {
		t.Fatalf("Create plain task: %v", err)
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
	if len(bundle.TaskSeries[0].RuleVersions) != 2 {
		t.Fatalf("rule versions = %d want 2", len(bundle.TaskSeries[0].RuleVersions))
	}
	if len(bundle.Tasks) != 3 {
		t.Fatalf("tasks count = %d want 3 (1 occurrence + 1 tombstone + 1 plain)", len(bundle.Tasks))
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
	// 目标 workspace 需有同 ID project（occurrence 引用 project_id）。
	targetProj, err := targetSvc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	// 调整 bundle 中 project_id 指向目标 workspace 的 project。
	for i := range restored.TaskSeries {
		restored.TaskSeries[i].ProjectID = targetProj.ID
		restored.TaskSeries[i].WorkspaceID = targetWs.ID
	}
	for i := range restored.Tasks {
		restored.Tasks[i].WorkspaceID = targetWs.ID
		if restored.Tasks[i].ProjectID != nil {
			pid := targetProj.ID
			restored.Tasks[i].ProjectID = &pid
		}
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
