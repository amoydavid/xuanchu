package app

import (
	"reflect"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
)

func TestOccurrenceRefRoundTrip(t *testing.T) {
	ref := OccurrenceRef("11111111-1111-1111-1111-111111111111", 1783785599)
	if !IsOccurrenceRef(ref) {
		t.Fatalf("IsOccurrenceRef(%q) = false", ref)
	}
	seriesID, slot, err := ParseOccurrenceRef(ref)
	if err != nil {
		t.Fatalf("ParseOccurrenceRef: %v", err)
	}
	if seriesID != "11111111-1111-1111-1111-111111111111" || slot != 1783785599 {
		t.Fatalf("ParseOccurrenceRef = %s %d", seriesID, slot)
	}
}

func TestParseOccurrenceRefRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"not-an-occ-ref", "occ:", "occ:onlyonepart", "occ:series:abc", "occ::123"} {
		if _, _, err := ParseOccurrenceRef(bad); err == nil {
			t.Fatalf("ParseOccurrenceRef(%q) 应失败", bad)
		}
	}
}

func TestTaskToViewOrdinaryTask(t *testing.T) {
	tsk := domain.Task{
		UUID: "task-1", WorkspaceID: "ws", Title: "普通任务", Status: domain.StatusPending,
		Entry: 100, Modified: 200, Project: strPtr("ops"), ProjectSeq: int64Ptr(17),
	}
	view := taskToView(tsk, nil)
	if view.ID != "task-1" {
		t.Fatalf("ID = %q want task-1", view.ID)
	}
	if view.UUID == nil || *view.UUID != "task-1" {
		t.Fatalf("UUID = %#v", view.UUID)
	}
	if view.TaskSlug == nil || *view.TaskSlug != "ops-17" {
		t.Fatalf("TaskSlug = %#v want ops-17", view.TaskSlug)
	}
	if view.RecurrenceInfo != nil {
		t.Fatalf("普通任务不应有 RecurrenceInfo: %#v", view.RecurrenceInfo)
	}
	if view.Entry == nil || *view.Entry != 100 {
		t.Fatalf("Entry = %#v", view.Entry)
	}
}

func TestTaskToViewMaterializedOccurrence(t *testing.T) {
	seriesID := "series-1"
	slot := int64(1783785599)
	rule := "daily"
	tsk := domain.Task{
		UUID: "occ-uuid", WorkspaceID: "ws", Title: "巡检", Status: domain.StatusPending,
		Entry: 100, Modified: 200,
		SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		RecurrenceOverrides: []string{"due"},
	}
	view := taskToView(tsk, nil)
	// 已物化 occurrence 的 ID 仍是 occurrence_ref（不变）。
	wantRef := OccurrenceRef(seriesID, slot)
	if view.ID != wantRef {
		t.Fatalf("ID = %q want %q", view.ID, wantRef)
	}
	if view.UUID == nil || *view.UUID != "occ-uuid" {
		t.Fatalf("UUID = %#v want occ-uuid", view.UUID)
	}
	if view.RecurrenceInfo == nil {
		t.Fatal("RecurrenceInfo 为空")
	}
	if view.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("Materialization = %q want materialized", view.RecurrenceInfo.Materialization)
	}
	if view.RecurrenceInfo.RecurrenceAt != slot {
		t.Fatalf("RecurrenceAt = %d want %d", view.RecurrenceInfo.RecurrenceAt, slot)
	}
	if !reflect.DeepEqual(view.RecurrenceInfo.Overrides, []string{"due"}) {
		t.Fatalf("Overrides = %#v", view.RecurrenceInfo.Overrides)
	}
}

func TestProjectedOccurrenceViewDoesNotAllocateIdentity(t *testing.T) {
	series := taskseries.Series{
		ID: "series-1", WorkspaceID: "ws", ProjectID: "proj", Title: "巡检",
		Status: taskseries.StatusActive, RecurrenceRule: "daily", FirstDue: 1783785599,
	}
	slot := taskseries.Slot{RecurrenceAt: 1783785599, Rule: "daily"}
	view := projectedOccurrenceView(series, slot, nil)
	wantRef := OccurrenceRef("series-1", 1783785599)
	if view.ID != wantRef {
		t.Fatalf("ID = %q want %q", view.ID, wantRef)
	}
	if view.UUID != nil {
		t.Fatalf("projected UUID 应为 nil: %#v", view.UUID)
	}
	if view.TaskSlug != nil {
		t.Fatalf("projected TaskSlug 应为 nil")
	}
	if view.ProjectSeq != nil {
		t.Fatalf("projected ProjectSeq 应为 nil")
	}
	if view.Entry != nil || view.Modified != nil {
		t.Fatalf("projected Entry/Modified 应为 nil: %#v %#v", view.Entry, view.Modified)
	}
	if view.Status != domain.StatusPending {
		t.Fatalf("projected Status = %q want pending", view.Status)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("RecurrenceInfo = %#v", view.RecurrenceInfo)
	}
	if view.Due == nil || *view.Due != 1783785599 {
		t.Fatalf("projected Due 应等于 recurrence_at: %#v", view.Due)
	}
}

// 辅助
func strPtr(s string) *string { return &s }

// newOccurrenceMergeFixture 建立一个含 series + 部分物化 occurrence 的 fixture。
func newOccurrenceMergeFixture(t *testing.T) (*Service, taskseries.Series, int64, int64, int64) {
	t.Helper()
	svc, closeFn := newTestService(t, 1783900000)
	t.Cleanup(closeFn)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	// 创建 daily series，first_due 在未来（不立即物化）。
	day1 := int64(1783785599) // 2026-07-11 23:59:59 +08:00
	day2 := day1 + 86400
	day3 := day2 + 86400
	series, err := svc.taskSeriesRepo.Create(taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Title: "每日巡检",
		Status: taskseries.StatusActive, RecurrenceRule: "daily", FirstDue: day1,
		CreatedBy: "local", CreatedAt: 1783785599, ModifiedAt: 1783785599,
	})
	if err != nil {
		t.Fatalf("Create series: %v", err)
	}
	// 物化 day1（pending）。
	rule := "daily"
	seriesID := series.ID
	_, _, err = svc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "occ-day1", WorkspaceID: ws.ID, Title: "每日巡检", Status: domain.StatusPending,
		Entry: 1, Modified: 1, ProjectID: &proj.ID,
		SeriesID: &seriesID, RecurrenceAt: &day1, RecurrenceRuleSnapshot: &rule,
	})
	if err != nil {
		t.Fatalf("materialize day1: %v", err)
	}
	return svc, series, day1, day2, day3
}

func TestQueryTaskViewsMaterializedModeReturnsOnlyMaterialized(t *testing.T) {
	svc, _, day1, _, _ := newOccurrenceMergeFixture(t)
	page, err := svc.QueryTaskViews(TaskViewQuery{OccurrenceMode: OccurrenceModeMaterialized})
	if err != nil {
		t.Fatalf("QueryTaskViews: %v", err)
	}
	if page.OccurrenceMode != OccurrenceModeMaterialized {
		t.Fatalf("mode = %q want materialized", page.OccurrenceMode)
	}
	// 只应有 day1 的 materialized occurrence。
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d want 1: %#v", len(page.Items), page.Items)
	}
	if page.Items[0].RecurrenceInfo == nil || page.Items[0].RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("item[0] 应为 materialized: %#v", page.Items[0].RecurrenceInfo)
	}
	if page.Items[0].RecurrenceInfo.RecurrenceAt != day1 {
		t.Fatalf("recurrence_at = %d want %d", page.Items[0].RecurrenceInfo.RecurrenceAt, day1)
	}
}

func TestQueryTaskViewsExpandMergesProjectedAndMaterializedWithoutWrites(t *testing.T) {
	svc, _, day1, day2, day3 := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	page, err := svc.QueryTaskViews(TaskViewQuery{
		OccurrenceMode: OccurrenceModeExpand,
		Range:          &TaskViewRange{Start: day1, End: day3 + 1},
	})
	if err != nil {
		t.Fatalf("QueryTaskViews expand: %v", err)
	}
	if page.OccurrenceMode != OccurrenceModeExpand {
		t.Fatalf("mode = %q want expand", page.OccurrenceMode)
	}
	// day1 materialized + day2/day3 projected = 3 条。
	if len(page.Items) != 3 {
		refs := make([]string, 0)
		for _, it := range page.Items {
			refs = append(refs, it.ID)
		}
		t.Fatalf("items len = %d want 3: %v", len(page.Items), refs)
	}
	// 不写库。
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("expand 写库了: before=%d after=%d", beforeCount, after)
	}
	// day1 为 materialized，day2/day3 为 projected。
	bySlot := map[int64]TaskOccurrenceView{}
	for _, it := range page.Items {
		if it.RecurrenceInfo != nil {
			bySlot[it.RecurrenceInfo.RecurrenceAt] = it
		}
	}
	if bySlot[day1].RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("day1 应为 materialized: %#v", bySlot[day1].RecurrenceInfo)
	}
	if bySlot[day2].RecurrenceInfo == nil || bySlot[day2].RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("day2 应为 projected: %#v", bySlot[day2].RecurrenceInfo)
	}
	if bySlot[day2].UUID != nil {
		t.Fatalf("projected UUID 应为 nil: %#v", bySlot[day2].UUID)
	}
}

func TestQueryTaskViewsExpandRejectsMissingRange(t *testing.T) {
	svc, _, _, _, _ := newOccurrenceMergeFixture(t)
	_, err := svc.QueryTaskViews(TaskViewQuery{OccurrenceMode: OccurrenceModeExpand})
	if err == nil {
		t.Fatal("expand 缺范围应失败")
	}
}

func TestQueryTaskViewsExpandRejectsRangeTooLarge(t *testing.T) {
	svc, _, day1, _, _ := newOccurrenceMergeFixture(t)
	// 400 天 > 366。
	_, err := svc.QueryTaskViews(TaskViewQuery{
		OccurrenceMode: OccurrenceModeExpand,
		Range:          &TaskViewRange{Start: day1, End: day1 + 400*86400},
	})
	if err == nil {
		t.Fatal("超出 366 天应失败")
	}
}

func TestGetTaskViewProjectedDoesNotWrite(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	ref := OccurrenceRef(series.ID, day2)
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView(projected): %v", err)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("应为 projected: %#v", view.RecurrenceInfo)
	}
	if view.UUID != nil {
		t.Fatalf("projected UUID 应为 nil")
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("GetTaskView(projected) 写库了: before=%d after=%d", beforeCount, after)
	}
}

func TestGetTaskViewMaterialized(t *testing.T) {
	svc, series, day1, _, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day1)
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView(materialized): %v", err)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("应为 materialized: %#v", view.RecurrenceInfo)
	}
}

func TestGetTaskViewRejectsInvalidOccurrenceRef(t *testing.T) {
	svc, _, _, _, _ := newOccurrenceMergeFixture(t)
	// 不存在的 series。
	_, err := svc.GetTaskView("occ:nonexistent-series:9999999")
	if err == nil {
		t.Fatal("不存在的 occurrence_ref 应失败")
	}
}

// occurrenceRowCount 统计 tasks 表中 occurrence 行数。
func occurrenceRowCount(t *testing.T, svc *Service, workspaceID string) int {
	t.Helper()
	tasks, err := svc.repo.List(workspaceID, storage.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	count := 0
	for _, tk := range tasks {
		if tk.SeriesID != nil {
			count++
		}
	}
	return count
}
