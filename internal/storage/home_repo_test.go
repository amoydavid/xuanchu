package storage

import "testing"

func TestHomeRepositoryProjectMetricsBatch(t *testing.T) {
	store, projectRepo, ws := newProjectRepoTest(t)
	alpha, err := projectRepo.Create(testProject("home-alpha", ws.ID, "alpha", 100))
	if err != nil {
		t.Fatal(err)
	}
	beta, err := projectRepo.Create(testProject("home-beta", ws.ID, "beta", 101))
	if err != nil {
		t.Fatal(err)
	}
	now := int64(1_000)
	overdue := now - 1
	waitReady := now
	high := "H"
	alphaSlug, betaSlug := alpha.Slug, beta.Slug
	alphaSeq, betaSeq := int64(1), int64(1)
	seriesID := "series-alpha"
	recurrenceAt := int64(900)
	rows := []Task{
		{
			UUID: "home-task-alpha", WorkspaceID: ws.ID, Title: "alpha overdue occurrence",
			Status: "pending", Entry: 1, Modified: 1, Due: &overdue, Priority: &high,
			Project: &alphaSlug, ProjectID: &alpha.ID, ProjectSeq: &alphaSeq,
			SeriesID: &seriesID, RecurrenceAt: &recurrenceAt,
		},
		{
			UUID: "home-task-beta", WorkspaceID: ws.ID, Title: "beta ready",
			Status: "waiting", Entry: 2, Modified: 2, Wait: &waitReady,
			Project: &betaSlug, ProjectID: &beta.ID, ProjectSeq: &betaSeq,
		},
	}
	if err := store.DB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	got, err := NewHomeRepository(store.DB()).ProjectMetrics(ws.ID, []string{alpha.ID, beta.ID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got[alpha.ID].OverdueCount != 1 || got[alpha.ID].HighPriorityOpenCount != 1 {
		t.Fatalf("alpha metrics = %#v", got[alpha.ID])
	}
	if got[alpha.ID].OpenRecurringOccurrenceCount != 1 || got[alpha.ID].OverdueRecurringOccurrenceCount != 1 {
		t.Fatalf("alpha recurrence metrics = %#v", got[alpha.ID])
	}
	if got[beta.ID].WaitReadyCount != 1 || got[beta.ID].UnassignedOpenCount != 1 {
		t.Fatalf("beta metrics = %#v", got[beta.ID])
	}
}

func TestHomeRepositoryLatestProjectAnnotations(t *testing.T) {
	store, projectRepo, ws := newProjectRepoTest(t)
	alpha, err := projectRepo.Create(testProject("home-annotation-alpha", ws.ID, "annotation-alpha", 100))
	if err != nil {
		t.Fatal(err)
	}
	beta, err := projectRepo.Create(testProject("home-annotation-beta", ws.ID, "annotation-beta", 101))
	if err != nil {
		t.Fatal(err)
	}
	rows := []ProjectAnnotation{
		{ID: "alpha-old", ProjectID: alpha.ID, Entry: 1, Content: "old-a", CreatedBy: "local", CreatedAt: 1},
		{ID: "alpha-new", ProjectID: alpha.ID, Entry: 3, Content: "latest-a", CreatedBy: "local", CreatedAt: 3},
		{ID: "beta-new", ProjectID: beta.ID, Entry: 2, Content: "latest-b", CreatedBy: "local", CreatedAt: 2},
	}
	if err := store.DB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	got, err := NewHomeRepository(store.DB()).LatestProjectAnnotations([]string{alpha.ID, beta.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got[alpha.ID].Content != "latest-a" || got[beta.ID].Content != "latest-b" {
		t.Fatalf("latest annotations = %#v", got)
	}
}

func TestHomeRepositoryEmptyProjectIDs(t *testing.T) {
	store, _, _ := newProjectRepoTest(t)
	repo := NewHomeRepository(store.DB())
	metrics, err := repo.ProjectMetrics("workspace", nil, 100)
	if err != nil || len(metrics) != 0 {
		t.Fatalf("metrics = %#v, err = %v", metrics, err)
	}
	annotations, err := repo.LatestProjectAnnotations(nil)
	if err != nil || len(annotations) != 0 {
		t.Fatalf("annotations = %#v, err = %v", annotations, err)
	}
}
