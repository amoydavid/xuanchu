package storage

import "testing"

func TestBackfillOccurrenceProjectBindingsRestoresProjectSlug(t *testing.T) {
	store, ws, project, series := newTaskSeriesRepoFixture(t)
	slot := int64(1783958399)
	seq := int64(7)
	rule := "daily"
	overrides := "[]"
	row := Task{
		UUID: "occurrence-without-project-slug", WorkspaceID: ws.ID,
		Title: "每日巡检", Status: "pending", Entry: 1, Modified: 1,
		ProjectID: &project.ID, ProjectSeq: &seq,
		SeriesID: &series.ID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		RecurrenceOverridesJSON: &overrides,
	}
	if err := store.DB().Create(&row).Error; err != nil {
		t.Fatalf("create incomplete occurrence: %v", err)
	}

	if err := backfillOccurrenceProjectBindings(store.DB()); err != nil {
		t.Fatalf("backfillOccurrenceProjectBindings: %v", err)
	}

	var got Task
	if err := store.DB().First(&got, "uuid = ?", row.UUID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Project == nil || *got.Project != project.Slug {
		t.Fatalf("Project = %#v，期望 %q", got.Project, project.Slug)
	}
	if got.ProjectID == nil || *got.ProjectID != project.ID || got.ProjectSeq == nil || *got.ProjectSeq != seq {
		t.Fatalf("project binding = (%#v, %#v, %#v)", got.Project, got.ProjectID, got.ProjectSeq)
	}
}

func TestBackfillOccurrenceProjectBindingsRejectsPartialBinding(t *testing.T) {
	store, ws, _, series := newTaskSeriesRepoFixture(t)
	slot := int64(1783958399)
	seq := int64(7)
	rule := "daily"
	overrides := "[]"
	row := Task{
		UUID: "partial-binding-occurrence", WorkspaceID: ws.ID,
		Title: "每日巡检", Status: "pending", Entry: 1, Modified: 1,
		ProjectSeq: &seq,
		SeriesID:   &series.ID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		RecurrenceOverridesJSON: &overrides,
	}
	if err := store.DB().Create(&row).Error; err != nil {
		t.Fatalf("create partial occurrence: %v", err)
	}

	if err := backfillOccurrenceProjectBindings(store.DB()); err == nil {
		t.Fatal("backfillOccurrenceProjectBindings partial binding error = nil")
	}
}
