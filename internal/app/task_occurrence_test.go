package app

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/query"
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

// --- Task 5: Series CRUD 测试 ---

func TestAddTaskSeriesCreatesFutureProjectedFirstOccurrence(t *testing.T) {
	svc, closeFn := newTestService(t, 1783785599-86400) // now 在 first_due 之前
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := int64(1783785599)
	result, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}
	// series 存在。
	if result.Series.ID == "" {
		t.Fatal("series ID 为空")
	}
	if result.Series.Status != taskseries.StatusActive {
		t.Fatalf("status = %q want active", result.Series.Status)
	}
	// 初始 rule version。
	if len(result.Series.RuleVersions) != 1 {
		t.Fatalf("rule versions len = %d want 1", len(result.Series.RuleVersions))
	}
	// 无 task 行（first_due 在未来）。
	if count := occurrenceRowCount(t, svc, ws.ID); count != 0 {
		t.Fatalf("future first_due 不应物化, count = %d", count)
	}
	// first occurrence 是 projected。
	if result.FirstOccurrence == nil {
		t.Fatal("first occurrence 为空")
	}
	if result.FirstOccurrence.RecurrenceInfo == nil || result.FirstOccurrence.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("first occurrence 应为 projected: %#v", result.FirstOccurrence.RecurrenceInfo)
	}
	if result.FirstOccurrence.UUID != nil {
		t.Fatal("projected first occurrence UUID 应为 nil")
	}
}

func TestAddTaskSeriesMaterializesFirstOccurrenceWhenDueEntered(t *testing.T) {
	svc, closeFn := newTestService(t, 1783785599+86400) // now 在 first_due 之后
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := int64(1783785599)
	result, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}
	// first_due 已进入执行期：物化。
	if result.FirstOccurrence == nil || result.FirstOccurrence.RecurrenceInfo == nil {
		t.Fatal("first occurrence / info 为空")
	}
	if result.FirstOccurrence.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("应物化: %#v", result.FirstOccurrence.RecurrenceInfo)
	}
	if result.FirstOccurrence.UUID == nil {
		t.Fatal("materialized 应有 UUID")
	}
	if count := occurrenceRowCount(t, svc, ws.ID); count != 1 {
		t.Fatalf("应物化 1 条, count = %d", count)
	}
}

func TestAddTaskSeriesRejectsInvalidRule(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "x", ProjectID: proj.ID, RecurrenceRule: "biweekly", FirstDue: 2000,
	})
	if err == nil {
		t.Fatal("biweekly 应被拒绝")
	}
}

func TestAddTaskSeriesRejectsUnsupportedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	wait := int64(500)
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "x", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000, Wait: &wait,
	})
	if err == nil {
		t.Fatal("wait 不被支持")
	}
}

func TestListTaskSeriesReturnsCreated(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListTaskSeries(TaskSeriesListInput{ProjectID: proj.ID})
	if err != nil {
		t.Fatalf("ListTaskSeries: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d want 1", len(page.Items))
	}
	if page.Total != 1 {
		t.Fatalf("total = %d want 1", page.Total)
	}
}

func TestGetTaskSeriesReturnsDetail(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetTaskSeries(created.Series.ID)
	if err != nil {
		t.Fatalf("GetTaskSeries: %v", err)
	}
	if detail.Series.ID != created.Series.ID {
		t.Fatalf("series ID 不匹配")
	}
}

// --- Task 6: 写前物化测试 ---

func TestMaterializeOccurrenceForWriteCreatesTaskForProjectedRef(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	ref := OccurrenceRef(series.ID, day2)

	tsk, existed, err := svc.MaterializeOccurrenceForWrite(ref)
	if err != nil {
		t.Fatalf("MaterializeOccurrenceForWrite: %v", err)
	}
	if existed {
		t.Fatal("projected 不应 existed=true")
	}
	if tsk.UUID == "" {
		t.Fatal("物化后应有 UUID")
	}
	if tsk.SeriesID == nil || *tsk.SeriesID != series.ID {
		t.Fatalf("series_id 不匹配: %#v", tsk.SeriesID)
	}
	if tsk.RecurrenceAt == nil || *tsk.RecurrenceAt != day2 {
		t.Fatalf("recurrence_at 不匹配: %#v", tsk.RecurrenceAt)
	}
	if tsk.ProjectSeq == nil {
		t.Fatal("物化应分配 project_seq")
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount+1 {
		t.Fatalf("应新增 1 行: before=%d after=%d", beforeCount, after)
	}
}

func TestMaterializeOccurrenceForWriteIdempotentForExistingSlot(t *testing.T) {
	svc, series, day1, _, _ := newOccurrenceMergeFixture(t)
	// day1 已物化（fixture 中物化了 day1）。
	ref := OccurrenceRef(series.ID, day1)
	tsk, existed, err := svc.MaterializeOccurrenceForWrite(ref)
	if err != nil {
		t.Fatalf("MaterializeOccurrenceForWrite: %v", err)
	}
	if !existed {
		t.Fatal("已物化槽位应 existed=true")
	}
	if tsk.UUID != "occ-day1" {
		t.Fatalf("应返回已存在的 UUID occ-day1, got %q", tsk.UUID)
	}
}

func TestMaterializeOccurrenceForWriteRejectsNonOccurrenceRef(t *testing.T) {
	svc, _, _, _, _ := newOccurrenceMergeFixture(t)
	_, _, err := svc.MaterializeOccurrenceForWrite("not-an-occ-ref")
	if err == nil {
		t.Fatal("非 occurrence_ref 应失败")
	}
}

func TestMaterializeOccurrenceForWriteRejectsInvalidSlot(t *testing.T) {
	svc, series, _, _, _ := newOccurrenceMergeFixture(t)
	// 槽位 9999 不在 daily 序列的合法位置（first_due 之后但需属于规则段）。
	// 实际 daily 每天一个槽位，9999 作为任意时间戳可能不在合法序列；用明显非法的负数。
	_, _, err := svc.MaterializeOccurrenceForWrite(OccurrenceRef(series.ID, -1))
	if err == nil {
		t.Fatal("非法槽位应失败")
	}
}

func TestWithTaskForWriteExecutesActionAndMaterializes(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	actionCalled := false
	view, err := svc.WithTaskForWrite(ref, func(s *Service, tsk domain.Task) error {
		actionCalled = true
		if tsk.UUID == "" {
			t.Fatal("action 收到的 task 应已物化")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTaskForWrite: %v", err)
	}
	if !actionCalled {
		t.Fatal("action 未被调用")
	}
	if view.UUID == nil {
		t.Fatal("返回 view 应有 UUID")
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("应为 materialized: %#v", view.RecurrenceInfo)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount+1 {
		t.Fatalf("应新增 1 行: before=%d after=%d", beforeCount, after)
	}
}

func TestWithTaskForWriteRollsBackOnActionError(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	_, err := svc.WithTaskForWrite(ref, func(s *Service, tsk domain.Task) error {
		return fmt.Errorf("simulated failure")
	})
	if err == nil {
		t.Fatal("action 错误应传播")
	}
	// 物化应回滚（无新行）。
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("action 失败应回滚物化: before=%d after=%d", beforeCount, after)
	}
}

func TestWithTaskForWriteWorksWithOrdinaryTaskUUID(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	created, err := svc.Add(AddInput{Title: "普通任务"})
	if err != nil {
		t.Fatal(err)
	}
	actionCalled := false
	view, err := svc.WithTaskForWrite(created.UUID, func(s *Service, tsk domain.Task) error {
		actionCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithTaskForWrite(ordinary): %v", err)
	}
	if !actionCalled {
		t.Fatal("action 未被调用")
	}
	if view.ID != created.UUID {
		t.Fatalf("view ID = %q want %q", view.ID, created.UUID)
	}
	if view.RecurrenceInfo != nil {
		t.Fatal("普通任务不应有 RecurrenceInfo")
	}
}

func TestWithExistingTaskForSubresourceWriteProjectedReturnsNotFound(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	_, err := svc.WithExistingTaskForSubresourceWrite(ref, func(s *Service, tsk domain.Task) error {
		t.Fatal("projected 子资源写不应调用 action")
		return nil
	})
	if err == nil {
		t.Fatal("projected 子资源写应返回 not found")
	}
	// 不物化。
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("projected 子资源写不应物化: before=%d after=%d", beforeCount, after)
	}
}

func TestWithExistingTaskForSubresourceWorksWithMaterialized(t *testing.T) {
	svc, series, day1, _, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day1)
	actionCalled := false
	_, err := svc.WithExistingTaskForSubresourceWrite(ref, func(s *Service, tsk domain.Task) error {
		actionCalled = true
		if tsk.UUID != "occ-day1" {
			t.Fatalf("UUID = %q want occ-day1", tsk.UUID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithExistingTaskForSubresourceWrite: %v", err)
	}
	if !actionCalled {
		t.Fatal("materialized 子资源写 action 未被调用")
	}
}

func TestResolveTaskForReadReturnsProjectedWithoutWrites(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	ref := OccurrenceRef(series.ID, day2)

	view, err := svc.ResolveTaskForRead(ref)
	if err != nil {
		t.Fatalf("ResolveTaskForRead: %v", err)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("应为 projected: %#v", view.RecurrenceInfo)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("读取不应物化: before=%d after=%d", beforeCount, after)
	}
}

// --- Task 7: reconcile + scheduler 测试 ---

func TestReconcileTaskSeriesMaterializesDueSlots(t *testing.T) {
	// daily series，first_due 在 3 天前，now 在今天。reconcile 应补齐 3 天。
	svc, closeFn := newTestService(t, 1783785599+2*86400) // now = first_due + 2天
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := int64(1783785599)
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}
	// first_due 已进入执行期，AddTaskSeries 已物化 1 条。再 reconcile 应补齐后 2 天。
	result, err := svc.ReconcileTaskSeries(series.Series.ID, svc.clock.Unix(), 100)
	if err != nil {
		t.Fatalf("ReconcileTaskSeries: %v", err)
	}
	// 至少创建 2 条（day2、day3）。
	if result.Created < 2 {
		t.Fatalf("Created = %d want >= 2: %#v", result.Created, result)
	}
	if count := occurrenceRowCount(t, svc, ws.ID); count < 3 {
		t.Fatalf("总物化数 = %d want >= 3", count)
	}
}

func TestReconcileTaskSeriesIdempotent(t *testing.T) {
	svc, closeFn := newTestService(t, 1783785599+2*86400)
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 1783785599,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ReconcileTaskSeries(series.Series.ID, svc.clock.Unix(), 100)
	after1 := occurrenceRowCount(t, svc, ws.ID)
	// 二次 reconcile 不应新增。
	_, err = svc.ReconcileTaskSeries(series.Series.ID, svc.clock.Unix(), 100)
	if err != nil {
		t.Fatalf("二次 reconcile: %v", err)
	}
	after2 := occurrenceRowCount(t, svc, ws.ID)
	if after2 != after1 {
		t.Fatalf("二次 reconcile 应幂等: after1=%d after2=%d", after1, after2)
	}
}

func TestReconcileTaskSeriesRespectsUntilAndEndsSeries(t *testing.T) {
	// until 在今天之前，所有槽位应已过，series ended。
	firstDue := int64(1783785599)
	until := firstDue + 86400 // first_due + 1 天
	now := firstDue + 3*86400 // 3 天后
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue, Until: &until,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ReconcileTaskSeries(series.Series.ID, now, 100)
	if err != nil {
		t.Fatalf("ReconcileTaskSeries: %v", err)
	}
	// until 是包含式：first_due + 1 天，共 2 个槽位。
	if result.Created < 1 {
		t.Fatalf("应补齐至少 1 条: %#v", result)
	}
	if !result.Ended {
		t.Fatalf("until 已过应 ended: %#v", result)
	}
}

func TestReconcileTaskSeriesRespectsPerSeriesLimit(t *testing.T) {
	// first_due 在 200 天前，daily，限制每轮 50 条 → backlog。
	firstDue := int64(1783785599)
	now := firstDue + 200*86400
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ReconcileTaskSeries(series.Series.ID, now, 50)
	if err != nil {
		t.Fatalf("ReconcileTaskSeries: %v", err)
	}
	if result.BacklogRemaining == 0 {
		t.Fatalf("应有余留 backlog: %#v", result)
	}
}

func TestStopTaskSeriesMarksStoppedAndStopsGeneration(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.StopTaskSeries(created.Series.ID, StopTaskSeriesInput{})
	if err != nil {
		t.Fatalf("StopTaskSeries: %v", err)
	}
	if view.Status != taskseries.StatusStopped {
		t.Fatalf("status = %q want stopped", view.Status)
	}
	if view.EffectiveEndAt == nil {
		t.Fatal("stopped 应有 effective_end_at")
	}
	// reconcile stopped series 不应生成。
	result, err := svc.ReconcileTaskSeries(created.Series.ID, 10000, 100)
	if err != nil {
		t.Fatalf("reconcile stopped: %v", err)
	}
	if result.Created != 0 {
		t.Fatalf("stopped series 不应生成: %#v", result)
	}
}

func TestStopTaskSeriesDeleteOpenOccurrences(t *testing.T) {
	// 用真实时间戳避免 1970 年边界问题。first_due 在 now 之前 3 天。
	firstDue := int64(1783785599) // 2026-07-11 23:59:59 +08:00
	now := firstDue + 3*86400     // 3 天后
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ReconcileTaskSeries(created.Series.ID, now, 100)
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	if beforeCount < 2 {
		t.Fatalf("前置物化不足: %d", beforeCount)
	}
	deleteOpen := true
	_, err = svc.StopTaskSeries(created.Series.ID, StopTaskSeriesInput{DeleteOpenOccurrences: &deleteOpen})
	if err != nil {
		t.Fatalf("StopTaskSeries delete_open: %v", err)
	}
	for slot := firstDue; slot <= now; slot += 86400 {
		occ, err := svc.taskOccurrenceRepo.GetOccurrence(ws.ID, created.Series.ID, slot)
		if err != nil {
			continue
		}
		if occ.Status == domain.StatusPending || occ.Status == domain.StatusWaiting {
			t.Fatalf("slot %d 仍为 open: %s", slot, occ.Status)
		}
	}
}

func TestSkipTaskSeriesOccurrenceCreatesTombstone(t *testing.T) {
	// first_due 在未来，AddTaskSeries 不物化；skip 物化为 tombstone。
	firstDue := int64(1783785599 + 30*86400) // 30 天后
	now := int64(1783785599)
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := OccurrenceRef(created.Series.ID, firstDue)
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	if beforeCount != 0 {
		t.Fatalf("未来 first_due 不应预物化: before=%d", beforeCount)
	}
	view, err := svc.SkipTaskSeriesOccurrence(created.Series.ID, ref)
	if err != nil {
		t.Fatalf("SkipTaskSeriesOccurrence: %v", err)
	}
	if view.Status != domain.StatusDeleted {
		t.Fatalf("status = %q want deleted", view.Status)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != 1 {
		t.Fatalf("应新增 1 条 tombstone: after=%d", after)
	}
}

func TestListTaskSeriesOccurrencesPaged(t *testing.T) {
	firstDue := int64(1000)
	now := int64(5000)
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ReconcileTaskSeries(created.Series.ID, now, 100)
	// 查 pending。
	page, err := svc.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{Status: "pending"})
	if err != nil {
		t.Fatalf("ListTaskSeriesOccurrences: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("应有 pending occurrence")
	}
	for _, it := range page.Items {
		if it.Status != domain.StatusPending {
			t.Fatalf("status = %q want pending", it.Status)
		}
		if it.RecurrenceInfo == nil {
			t.Fatal("occurrence 应有 recurrence_info")
		}
	}
}

func TestProjectTransitionStopsActiveSeries(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	// archive 项目应停止 series。
	if _, err := svc.TransitionProject(proj.Slug, "archived"); err != nil {
		t.Fatalf("TransitionProject archived: %v", err)
	}
	series, err := svc.taskSeriesRepo.Get(svc.workspaceID, created.Series.ID)
	if err != nil {
		t.Fatalf("Get series: %v", err)
	}
	if series.Status != taskseries.StatusStopped {
		t.Fatalf("series status = %q want stopped", series.Status)
	}
	if series.StopReason == nil || *series.StopReason != taskseries.StopReasonProjectArchived {
		t.Fatalf("stop_reason = %#v want project_archived", series.StopReason)
	}
	// stopped series 不再生成。
	result, err := svc.ReconcileTaskSeries(created.Series.ID, 10000, 100)
	if err != nil {
		t.Fatalf("reconcile after stop: %v", err)
	}
	if result.Created != 0 {
		t.Fatalf("stopped series 不应生成: %#v", result)
	}
}

func TestTaskSeriesSchedulerRunOnce(t *testing.T) {
	// daily series，first_due 在 3 天前。scheduler RunOnce 应补齐。
	firstDue := int64(1783785599)
	now := firstDue + 3*86400
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, now, "local", "local")
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := NewTaskSeriesScheduler(TaskSeriesSchedulerOptions{
		Store: store,
		Clock: FixedClock{NowUnix: now},
		ServiceFactory: func(workspaceID string) *Service {
			return svc
		},
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Created == 0 {
		t.Fatalf("应补齐至少 1 条: %#v", result)
	}
	_ = created
}

func TestTaskSeriesSchedulerRunOnceUsesDefaultServiceFactory(t *testing.T) {
	firstDue := int64(1783785599)
	now := firstDue + 2*86400
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, now, "local", "local")
	proj, err := svc.AddProject(AddProjectInput{Slug: "scheddef", Name: "Scheduler Default"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日默认工厂巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewTaskSeriesScheduler(TaskSeriesSchedulerOptions{
		Store: store,
		Clock: FixedClock{NowUnix: now},
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce with default factory: %v", err)
	}
	if result.Created == 0 {
		t.Fatalf("default factory should reconcile occurrences: %#v", result)
	}
}

func TestTaskSeriesSchedulerRunOnceRejectsMissingStore(t *testing.T) {
	scheduler := NewTaskSeriesScheduler(TaskSeriesSchedulerOptions{})
	if _, err := scheduler.RunOnce(t.Context()); err == nil {
		t.Fatal("missing store 应返回错误")
	}
}

func TestProjectScopedServiceCannotReadOrSkipForeignOccurrence(t *testing.T) {
	now := int64(1783785599)
	store := newTestStore(t)
	base := newTestServiceWithRuntime(t, store, now, "local", "local")
	allowed, err := base.AddProject(AddProjectInput{Slug: "allowed", Name: "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := base.AddProject(AddProjectInput{Slug: "foreign", Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := base.AddTaskSeries(AddTaskSeriesInput{
		Title: "外部项目巡检", ProjectID: foreign.ID,
		RecurrenceRule: "daily", FirstDue: now + 86400,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := created.FirstOccurrence.ID
	scope := RequestScope{ProjectIDs: []string{allowed.ID}, Capabilities: []string{"task:read", "task:write"}}
	scoped, err := NewService(ServiceOptions{
		Store: store, Clock: FixedClock{NowUnix: now}, RequestScope: &scope,
		Runtime:               &RuntimeContext{ActorType: "user", ActorUserID: "local", WorkspaceID: base.workspaceID, Role: RoleOwner},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.GetTaskView(ref); err == nil {
		t.Fatal("GetTaskView foreign occurrence error = nil")
	}
	if _, err := scoped.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{}); err == nil {
		t.Fatal("ListTaskSeriesOccurrences foreign series error = nil")
	}
	if _, err := scoped.SkipTaskSeriesOccurrence(created.Series.ID, ref); err == nil {
		t.Fatal("SkipTaskSeriesOccurrence foreign series error = nil")
	}
	if _, err := base.taskOccurrenceRepo.GetOccurrence(base.workspaceID, created.Series.ID, *created.FirstOccurrence.Due); err != storage.ErrOccurrenceNotFound {
		t.Fatalf("denied skip must not materialize occurrence, got %v", err)
	}
}

// --- Task 5: ModifyTaskSeries 测试 ---

func TestModifyTaskSeriesUpdatesSharedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newTitle := "每周巡检"
	view, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle})
	if err != nil {
		t.Fatalf("ModifyTaskSeries: %v", err)
	}
	if view.Title != "每周巡检" {
		t.Fatalf("title = %q want 每周巡检", view.Title)
	}
}

func TestModifyTaskSeriesRejectsInactive(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StopTaskSeries(created.Series.ID, StopTaskSeriesInput{}); err != nil {
		t.Fatal(err)
	}
	newTitle := "x"
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle})
	if err == nil {
		t.Fatal("stopped series 应拒绝修改")
	}
}

func TestModifyTaskSeriesRuleRequiresEffectiveFrom(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRule := "weekly"
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{RecurrenceRule: &newRule})
	if err == nil {
		t.Fatal("修改 rule 缺 effective_from 应失败")
	}
}

func TestModifyTaskSeriesRuleAppendsVersion(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRule := "weekly"
	eff := int64(10000)
	view, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		RecurrenceRule: &newRule, EffectiveFrom: &eff,
	})
	if err != nil {
		t.Fatalf("ModifyTaskSeries rule: %v", err)
	}
	if view.RecurrenceRule != "weekly" {
		t.Fatalf("rule = %q want weekly", view.RecurrenceRule)
	}
	// rule version 应有 2 段。
	if len(view.RuleVersions) != 2 {
		t.Fatalf("rule versions = %d want 2", len(view.RuleVersions))
	}
}

func TestModifyTaskSeriesRuleRejectsPastEffectiveFrom(t *testing.T) {
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 6000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRule := "weekly"
	pastEff := int64(4000) // 早于 now=5000
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		RecurrenceRule: &newRule, EffectiveFrom: &pastEff,
	})
	if err == nil {
		t.Fatal("effective_from 早于 now 应失败")
	}
}

func TestModifyTaskSeriesSyncsSharedFieldsToOpenOccurrences(t *testing.T) {
	// first_due 在过去，物化 first occurrence；修改 title 应同步到该 occurrence。
	firstDue := int64(1000)
	now := int64(5000)
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	// AddTaskSeries 已物化 first occurrence。修改 title。
	newTitle := "每周巡检"
	if _, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle}); err != nil {
		t.Fatalf("ModifyTaskSeries: %v", err)
	}
	// 验证 occurrence title 同步。
	occ, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, firstDue)
	if err != nil {
		t.Fatalf("GetOccurrence: %v", err)
	}
	if occ.Title != "每周巡检" {
		t.Fatalf("occurrence title = %q want 每周巡检", occ.Title)
	}
}

func TestModifyTaskSeriesRollsBackWhenAuditFails(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.auditRepo = &failingAuditRepo{}
	newTitle := "不应保留的标题"
	if _, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle}); err == nil {
		t.Fatal("audit 失败时 ModifyTaskSeries 应失败")
	}
	persisted, err := svc.taskSeriesRepo.Get(svc.workspaceID, created.Series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Title != "每日巡检" {
		t.Fatalf("series title = %q want rollback to 每日巡检", persisted.Title)
	}
}

func TestModifyTaskSeriesSyncsOccurrenceBeyondOneYear(t *testing.T) {
	now := int64(1000)
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "长期巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	farSlot := now + 2*365*86400
	rule := "daily"
	seriesID := created.Series.ID
	if _, _, err := svc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "far-occurrence", WorkspaceID: svc.workspaceID, ProjectID: &project.ID,
		Title: "长期巡检", Status: domain.StatusPending, Entry: now, Modified: now,
		Due: &farSlot, SeriesID: &seriesID, RecurrenceAt: &farSlot,
		RecurrenceRuleSnapshot: &rule,
	}); err != nil {
		t.Fatal(err)
	}
	newTitle := "长期巡检已更新"
	if _, err := svc.ModifyTaskSeries(seriesID, ModifyTaskSeriesInput{Title: &newTitle}); err != nil {
		t.Fatal(err)
	}
	occurrence, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, seriesID, farSlot)
	if err != nil {
		t.Fatal(err)
	}
	if occurrence.Title != newTitle {
		t.Fatalf("far occurrence title = %q want %q", occurrence.Title, newTitle)
	}
}

func TestTaskSeriesOccurrencesInheritUDAs(t *testing.T) {
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	if err := svc.DefineUDA("estimate", "numeric", "Estimate", nil, ""); err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 5000,
		UDAs: map[string]string{"estimate": "3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.FirstOccurrence == nil || created.FirstOccurrence.UDAs["estimate"].Raw != "3" {
		t.Fatalf("first occurrence UDAs = %#v", created.FirstOccurrence)
	}
	persisted, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.UDAs["estimate"].Raw != "3" || persisted.UDAs["estimate"].Type != "numeric" {
		t.Fatalf("persisted occurrence UDAs = %#v", persisted.UDAs)
	}
}

func TestModifyTaskSeriesClearsSharedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	description, priority, until := "旧说明", "H", int64(90000)
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", Description: &description, ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: 6000, Until: &until,
		Priority: &priority, Tags: []string{"ops"},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		ClearDescription: true, ClearPriority: true, ClearTags: true, ClearUntil: true,
	})
	if err != nil {
		t.Fatalf("ModifyTaskSeries clear: %v", err)
	}
	if view.Description != nil || view.Priority != nil || view.Until != nil || len(view.Tags) != 0 {
		t.Fatalf("clear result = %#v", view)
	}
}

// --- Task 4: RunTaskViewReport 测试 ---

func TestRunTaskViewReportReturnsPage(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	// 创建普通任务。
	if _, err := svc.Add(AddInput{Title: "普通任务", Project: &proj.Slug}); err != nil {
		t.Fatal(err)
	}
	// 运行 list report（status:pending）。
	page, err := svc.RunTaskViewReport(ReportViewInput{
		Name: "list", OccurrenceMode: OccurrenceModeMaterialized,
	})
	if err != nil {
		t.Fatalf("RunTaskViewReport: %v", err)
	}
	if page.Total == 0 {
		t.Fatal("应至少有 1 个任务")
	}
	if len(page.Items) != page.Total {
		t.Fatalf("items=%d total=%d 应一致（无分页）", len(page.Items), page.Total)
	}
	// 验证返回的是 TaskOccurrenceView。
	for _, it := range page.Items {
		if it.ID == "" {
			t.Fatal("item ID 为空")
		}
	}
}

func TestRunTaskViewReportAppliesQueryFilter(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Title: "巡检A", Project: &proj.Slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Title: "报告B", Project: &proj.Slug}); err != nil {
		t.Fatal(err)
	}
	// 用 list report + bare text 过滤只留"巡检"。
	queryExpr, err := query.ParseQuery("巡检")
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.RunTaskViewReport(ReportViewInput{
		Name: "list", Query: queryExpr, OccurrenceMode: OccurrenceModeMaterialized,
	})
	if err != nil {
		t.Fatalf("RunTaskViewReport: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("total=%d want 1（只匹配巡检A）", page.Total)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "巡检A" {
		t.Fatalf("items = %#v", page.Items)
	}
}

func TestRunTaskViewReportPreservesDependencyScopes(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	blocker, err := svc.Add(AddInput{Title: "blocker"})
	if err != nil {
		t.Fatal(err)
	}
	blockedTask, err := svc.Add(AddInput{Title: "blocked", Depends: []string{blocker.UUID}})
	if err != nil {
		t.Fatal(err)
	}

	blocked, err := svc.RunTaskViewReport(ReportViewInput{Name: "blocked"})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.Items) != 1 || blocked.Items[0].UUID == nil || *blocked.Items[0].UUID != blockedTask.UUID {
		t.Fatalf("blocked report = %#v, want %s", blocked.Items, blockedTask.UUID)
	}
	ready, err := svc.RunTaskViewReport(ReportViewInput{Name: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ready.Items) != 1 || ready.Items[0].UUID == nil || *ready.Items[0].UUID != blocker.UUID {
		t.Fatalf("ready report = %#v, want %s", ready.Items, blocker.UUID)
	}
}

func TestQueryTaskViewsExpandAppliesQueryFilter(t *testing.T) {
	svc, _, day1, _, _ := newOccurrenceMergeFixture(t)
	// day1 物化的 occurrence 标题为"每日巡检"。
	// 用 bare text query 过滤只匹配"每日巡检"。
	expr, err := query.ParseQuery("每日巡检")
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.QueryTaskViews(TaskViewQuery{
		OccurrenceMode: OccurrenceModeExpand,
		Range:          &TaskViewRange{Start: day1, End: day1 + 3*86400},
		Query:          expr,
	})
	if err != nil {
		t.Fatalf("QueryTaskViews: %v", err)
	}
	// 所有结果都应含"每日巡检"。
	for _, it := range page.Items {
		if !strings.Contains(it.Title, "每日巡检") {
			t.Fatalf("query 过滤失败，出现不匹配项: %q", it.Title)
		}
	}
	if page.Total == 0 {
		t.Fatal("应至少匹配 1 项")
	}
}

// TestProjectTaskSummaryExcludesOccurrencesAndReportsSeriesMetrics 验证 §17.4：
// 一次性进度计数排除 materialized occurrence，series 运行情况单独报告。
func TestProjectTaskSummaryExcludesOccurrencesAndReportsSeriesMetrics(t *testing.T) {
	now := int64(1783785599)
	svc, closeFn := newTestService(t, now)
	defer closeFn()

	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	duePast := now - 3600
	// 普通逾期任务。
	if _, err := svc.Add(AddInput{Title: "普通逾期", Project: &proj.Slug, Due: &duePast}); err != nil {
		t.Fatalf("Add(普通逾期): %v", err)
	}
	// 创建 series，first_due 在过去 → 物化 occurrence（pending）。
	firstDue := now - 86400
	if _, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	}); err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}

	view, err := svc.ProjectTaskSummary("ops")
	if err != nil {
		t.Fatalf("ProjectTaskSummary: %v", err)
	}
	// 普通任务计数：只有 1 条普通逾期任务。
	if view.OverdueCount != 1 {
		t.Fatalf("OverdueCount = %d, want 1（不应含 occurrence）", view.OverdueCount)
	}
	if view.HighPriorityOpenCount != 0 {
		t.Fatalf("HighPriorityOpenCount = %d, want 0", view.HighPriorityOpenCount)
	}
	if view.UnassignedOpenCount != 1 {
		t.Fatalf("UnassignedOpenCount = %d, want 1（只统计普通任务）", view.UnassignedOpenCount)
	}
	// series metrics。
	if view.SeriesMetrics.RecurringSeriesCount != 1 {
		t.Fatalf("RecurringSeriesCount = %d, want 1", view.SeriesMetrics.RecurringSeriesCount)
	}
	if view.SeriesMetrics.ActiveRecurringSeriesCount != 1 {
		t.Fatalf("ActiveRecurringSeriesCount = %d, want 1", view.SeriesMetrics.ActiveRecurringSeriesCount)
	}
	// 物化的 occurrence pending（due 在过去）→ open=1, overdue=1。
	if view.SeriesMetrics.OpenRecurringOccurrenceCount != 1 {
		t.Fatalf("OpenRecurringOccurrenceCount = %d, want 1", view.SeriesMetrics.OpenRecurringOccurrenceCount)
	}
	if view.SeriesMetrics.OverdueRecurringOccurrenceCount != 1 {
		t.Fatalf("OverdueRecurringOccurrenceCount = %d, want 1", view.SeriesMetrics.OverdueRecurringOccurrenceCount)
	}
}
