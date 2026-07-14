package storage

import (
	"testing"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
)

func TestCreateOccurrenceIsUniqueBySeriesSlotForever(t *testing.T) {
	store, ws, proj, series := newTaskSeriesRepoFixture(t)
	repo := NewTaskOccurrenceRepository(store.DB())
	taskRepo := NewTaskRepository(store.DB())

	slot := int64(1783785599)
	rule := "daily"
	seriesID := series.ID
	row := domain.Task{
		UUID: "occ-1", WorkspaceID: ws.ID, Title: "巡检", Status: domain.StatusPending,
		Entry: 1, Modified: 1, ProjectID: &proj.ID, Project: &proj.Slug,
		SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
	}
	first, existed, err := repo.CreateOccurrence(row)
	if err != nil || existed {
		t.Fatalf("first CreateOccurrence = (%#v, %v, %v), want (task, false, nil)", first, existed, err)
	}

	// 同槽位二次写入：应返回已存在行，existed=true。
	row.UUID = "occ-2"
	second, existed, err := repo.CreateOccurrence(row)
	if err != nil || !existed || second.UUID != first.UUID {
		t.Fatalf("second CreateOccurrence = (%#v, %v, %v), want (first, true, nil)", second, existed, err)
	}

	// 完成 first 后，同槽位仍应返回该 completed 行（completed 不释放槽位）。
	first.Status = domain.StatusCompleted
	if err := taskRepo.Update(first); err != nil {
		t.Fatalf("Update completed: %v", err)
	}
	row.UUID = "occ-3"
	third, existed, err := repo.CreateOccurrence(row)
	if err != nil || !existed || third.UUID != first.UUID {
		t.Fatalf("third CreateOccurrence after completed = (%#v, %v, %v), want (first, true, nil)", third, existed, err)
	}
}

func TestOccurrenceGetReturnsErrForMissing(t *testing.T) {
	store, ws, _, _ := newTaskSeriesRepoFixture(t)
	repo := NewTaskOccurrenceRepository(store.DB())
	if _, err := repo.GetOccurrence(ws.ID, "series-x", 1000); err != ErrOccurrenceNotFound {
		t.Fatalf("GetOccurrence(missing) err = %v, want ErrOccurrenceNotFound", err)
	}
}

func TestListOccurrenceExceptionsMatchesRecurrenceAtAndDue(t *testing.T) {
	store, ws, proj, series := newTaskSeriesRepoFixture(t)
	repo := NewTaskOccurrenceRepository(store.DB())
	taskRepo := NewTaskRepository(store.DB())
	seriesID := series.ID
	rule := "daily"

	// occurrence A：槽位在 [1000, 2000) 内。
	slotA := int64(1000)
	occA := domain.Task{
		UUID: "occ-a", WorkspaceID: ws.ID, Title: "A", Status: domain.StatusPending,
		Entry: 1, Modified: 1, ProjectID: &proj.ID,
		SeriesID: &seriesID, RecurrenceAt: &slotA, RecurrenceRuleSnapshot: &rule,
	}
	if _, _, err := repo.CreateOccurrence(occA); err != nil {
		t.Fatalf("CreateOccurrence A: %v", err)
	}

	// occurrence B：槽位 5000，但 due 被改到 1500（rescheduled，新 due 进入范围）。
	slotB := int64(5000)
	dueB := int64(1500)
	occB := domain.Task{
		UUID: "occ-b", WorkspaceID: ws.ID, Title: "B", Status: domain.StatusPending,
		Entry: 1, Modified: 1, ProjectID: &proj.ID, Due: &dueB,
		SeriesID: &seriesID, RecurrenceAt: &slotB, RecurrenceRuleSnapshot: &rule,
	}
	if _, _, err := repo.CreateOccurrence(occB); err != nil {
		t.Fatalf("CreateOccurrence B: %v", err)
	}
	// 确认 B 的 due 被持久化。
	gotB, err := repo.GetOccurrence(ws.ID, seriesID, slotB)
	if err != nil {
		t.Fatalf("GetOccurrence B: %v", err)
	}
	if gotB.Due == nil || *gotB.Due != dueB {
		t.Fatalf("occurrence B due round-trip = %#v", gotB.Due)
	}
	_ = taskRepo // 保持引用

	// 查 [1000, 2000)：A（槽位命中）和 B（due 命中）都应返回。
	exceptions, err := repo.ListOccurrenceExceptions(OccurrenceRangeOptions{
		WorkspaceID: ws.ID, SeriesID: seriesID, Start: 1000, End: 2000,
	})
	if err != nil {
		t.Fatalf("ListOccurrenceExceptions: %v", err)
	}
	uuids := map[string]bool{}
	for _, e := range exceptions {
		uuids[e.UUID] = true
	}
	if !uuids["occ-a"] || !uuids["occ-b"] {
		t.Fatalf("range [1000,2000) 应同时命中 A(槽位) 和 B(due)，got = %#v", uuids)
	}

	// 查 [100, 200)：都不命中（A 槽位 1000 不在，B due 1500 不在）。
	none, err := repo.ListOccurrenceExceptions(OccurrenceRangeOptions{
		WorkspaceID: ws.ID, SeriesID: seriesID, Start: 100, End: 200,
	})
	if err != nil {
		t.Fatalf("ListOccurrenceExceptions none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("range [100,200) 应无结果，got = %#v", none)
	}
}

func TestListOccurrenceExceptionsPaginatesInDueOrderAndCountsFullSet(t *testing.T) {
	store, ws, proj, series := newTaskSeriesRepoFixture(t)
	repo := NewTaskOccurrenceRepository(store.DB())
	seriesID, rule := series.ID, "daily"
	for index, due := range []int64{300, 100, 200} {
		slot := int64(1000 + index)
		if _, _, err := repo.CreateOccurrence(domain.Task{
			UUID: "occ-page-" + string(rune('a'+index)), WorkspaceID: ws.ID,
			Title: "occ", Status: domain.StatusCompleted, Entry: slot, Modified: slot,
			ProjectID: &proj.ID, Due: &due, SeriesID: &seriesID, RecurrenceAt: &slot,
			RecurrenceRuleSnapshot: &rule,
		}); err != nil {
			t.Fatal(err)
		}
	}
	opts := OccurrenceRangeOptions{
		WorkspaceID: ws.ID, SeriesID: seriesID, Start: 0, End: 10_000,
		Status: domain.StatusCompleted, Limit: 1, Offset: 1,
	}
	total, err := repo.CountOccurrenceExceptions(opts)
	if err != nil || total != 3 {
		t.Fatalf("count = %d err=%v, want 3", total, err)
	}
	page, err := repo.ListOccurrenceExceptions(opts)
	if err != nil || len(page) != 1 || page[0].Due == nil || *page[0].Due != 200 {
		t.Fatalf("ascending page = %#v err=%v, want due=200", page, err)
	}
	opts.Descending = true
	page, err = repo.ListOccurrenceExceptions(opts)
	if err != nil || len(page) != 1 || page[0].Due == nil || *page[0].Due != 200 {
		t.Fatalf("descending page = %#v err=%v, want due=200", page, err)
	}
}

func TestCountSeriesOccurrences(t *testing.T) {
	store, ws, proj, series := newTaskSeriesRepoFixture(t)
	occRepo := NewTaskOccurrenceRepository(store.DB())
	taskRepo := NewTaskRepository(store.DB())
	seriesID := series.ID
	rule := "daily"

	// 创建 pending、completed、deleted 各一个。
	mkOcc := func(uuid string, slot int64, status string) domain.Task {
		return domain.Task{
			UUID: uuid, WorkspaceID: ws.ID, Title: "occ", Status: status,
			Entry: 1, Modified: 1, ProjectID: &proj.ID,
			SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		}
	}
	if _, _, err := occRepo.CreateOccurrence(mkOcc("occ-pending", 1000, domain.StatusPending)); err != nil {
		t.Fatal(err)
	}
	comp := mkOcc("occ-completed", 1100, domain.StatusCompleted)
	if _, _, err := occRepo.CreateOccurrence(comp); err != nil {
		t.Fatal(err)
	}
	del := mkOcc("occ-deleted", 1200, domain.StatusDeleted)
	if _, _, err := occRepo.CreateOccurrence(del); err != nil {
		t.Fatal(err)
	}
	// 给 pending 加 overdue due。
	pending, _ := taskRepo.GetByUUID(ws.ID, "occ-pending")
	overdueDue := int64(500)
	pending.Due = &overdueDue
	if err := taskRepo.Update(pending); err != nil {
		t.Fatal(err)
	}

	counts, err := occRepo.CountSeriesOccurrences(ws.ID, seriesID, 1000)
	if err != nil {
		t.Fatalf("CountSeriesOccurrences: %v", err)
	}
	if counts.Pending != 1 || counts.Completed != 1 || counts.Deleted != 1 {
		t.Fatalf("counts = %#v", counts)
	}
	if counts.Open != 1 {
		t.Fatalf("open = %d want 1", counts.Open)
	}
	if counts.Overdue != 1 {
		t.Fatalf("overdue = %d want 1 (pending due 500 < now 1000)", counts.Overdue)
	}
}

func TestSummarizeSeriesOccurrencesReturnsCountsAndMaxForEveryRequestedSeries(t *testing.T) {
	store, ws, proj, firstSeries := newTaskSeriesRepoFixture(t)
	seriesRepo := NewTaskSeriesRepository(store.DB())
	occRepo := NewTaskOccurrenceRepository(store.DB())
	secondSeries, err := seriesRepo.Create(taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Title: "每周巡检",
		Status: taskseries.StatusActive, RecurrenceRule: "weekly", FirstDue: 700,
		CreatedBy: "user-1", CreatedAt: 100, ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("Create second series: %v", err)
	}

	rule := "daily"
	create := func(uuid, seriesID string, slot int64, status string, due *int64) {
		t.Helper()
		row := domain.Task{
			UUID: uuid, WorkspaceID: ws.ID, Title: uuid, Status: status,
			Entry: 1, Modified: 1, ProjectID: &proj.ID, Project: &proj.Slug, Due: due,
			SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		}
		if _, _, err := occRepo.CreateOccurrence(row); err != nil {
			t.Fatalf("CreateOccurrence(%s): %v", uuid, err)
		}
	}
	overdue := int64(500)
	future := int64(1500)
	create("first-pending-overdue", firstSeries.ID, 100, domain.StatusPending, &overdue)
	create("first-waiting-overdue", firstSeries.ID, 200, domain.StatusWaiting, &overdue)
	create("first-completed", firstSeries.ID, 300, domain.StatusCompleted, &overdue)
	create("first-deleted", firstSeries.ID, 400, domain.StatusDeleted, &overdue)
	create("first-pending-future", firstSeries.ID, 500, domain.StatusPending, &future)
	create("second-pending-no-due", secondSeries.ID, 700, domain.StatusPending, nil)

	summaries, err := occRepo.SummarizeSeriesOccurrences(
		ws.ID,
		[]string{firstSeries.ID, secondSeries.ID, "missing-series"},
		1000,
	)
	if err != nil {
		t.Fatalf("SummarizeSeriesOccurrences: %v", err)
	}
	if got := summaries[firstSeries.ID]; got.Counts != (OccurrenceCounts{
		Open: 3, Pending: 2, Waiting: 1, Completed: 1, Deleted: 1, Overdue: 2,
	}) || got.MaxRecurrenceAt != 500 {
		t.Fatalf("first summary = %#v", got)
	}
	if got := summaries[secondSeries.ID]; got.Counts != (OccurrenceCounts{
		Open: 1, Pending: 1,
	}) || got.MaxRecurrenceAt != 700 {
		t.Fatalf("second summary = %#v", got)
	}
	if got, ok := summaries["missing-series"]; !ok || got != (SeriesOccurrenceSummary{}) {
		t.Fatalf("missing summary = %#v, present=%v", got, ok)
	}
}
