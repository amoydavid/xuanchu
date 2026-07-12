package app

import (
	"reflect"
	"testing"

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
