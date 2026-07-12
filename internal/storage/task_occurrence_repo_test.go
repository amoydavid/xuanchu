package storage

import (
	"testing"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
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
