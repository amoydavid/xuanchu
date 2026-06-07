package storage

import (
	"path/filepath"
	"testing"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/google/uuid"
)

func TestProjectAnnotationRepoCreateAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	projectRepo := NewProjectRepository(store.DB())
	annoRepo := NewProjectAnnotationRepository(store.DB())

	project, err := projectRepo.Create(Project{
		ID: uuid.NewString(), WorkspaceID: ws.ID, Slug: "testproj", Name: "Test",
		Status: "active", SettingsJSON: "{}", CreatedAt: 100, ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	a1 := ProjectAnnotation{
		ID: uuid.NewString(), ProjectID: project.ID, Entry: 200,
		Content: "first", CreatedBy: "user-1", CreatedAt: 200,
	}
	a2 := ProjectAnnotation{
		ID: uuid.NewString(), ProjectID: project.ID, Entry: 300,
		Content: "second", CreatedBy: "user-1", CreatedAt: 300,
	}

	if _, err := annoRepo.Create(a1); err != nil {
		t.Fatalf("create a1: %v", err)
	}
	if _, err := annoRepo.Create(a2); err != nil {
		t.Fatalf("create a2: %v", err)
	}

	list, err := annoRepo.ListByProject(project.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(list))
	}
	if list[0].Entry > list[1].Entry {
		t.Fatal("expected annotations ordered by entry ASC")
	}
}

func TestProjectAnnotationRepoDelete(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	projectRepo := NewProjectRepository(store.DB())
	annoRepo := NewProjectAnnotationRepository(store.DB())

	project, err := projectRepo.Create(Project{
		ID: uuid.NewString(), WorkspaceID: ws.ID, Slug: "delproj", Name: "Del",
		Status: "active", SettingsJSON: "{}", CreatedAt: 100, ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	created, err := annoRepo.Create(ProjectAnnotation{
		ID: uuid.NewString(), ProjectID: project.ID, Entry: 200,
		Content: "to delete", CreatedBy: "user-1", CreatedAt: 200,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := annoRepo.Delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = annoRepo.GetByID(created.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestProjectAnnotationRepoDeleteNotFound(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	annoRepo := NewProjectAnnotationRepository(store.DB())

	err = annoRepo.Delete("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestProjectAnnotationRepoGetByID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	projectRepo := NewProjectRepository(store.DB())
	annoRepo := NewProjectAnnotationRepository(store.DB())

	project, err := projectRepo.Create(Project{
		ID: uuid.NewString(), WorkspaceID: ws.ID, Slug: "getproj", Name: "Get",
		Status: "active", SettingsJSON: "{}", CreatedAt: 100, ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	created, err := annoRepo.Create(ProjectAnnotation{
		ID: uuid.NewString(), ProjectID: project.ID, Entry: 200,
		Content: "find me", CreatedBy: "user-1", CreatedAt: 200,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	found, err := annoRepo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if found.Content != "find me" {
		t.Fatalf("expected content 'find me', got %q", found.Content)
	}
}

func TestProjectAnnotationRepoGetByIDNotFound(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	annoRepo := NewProjectAnnotationRepository(store.DB())

	_, err = annoRepo.GetByID("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestProjectAnnotationRepoRecentByProject(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	projectRepo := NewProjectRepository(store.DB())
	annoRepo := NewProjectAnnotationRepository(store.DB())

	project, err := projectRepo.Create(Project{
		ID: uuid.NewString(), WorkspaceID: ws.ID, Slug: "recentproj", Name: "Recent",
		Status: "active", SettingsJSON: "{}", CreatedAt: 100, ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	for i, entry := range []int64{200, 300, 400, 500} {
		_, err := annoRepo.Create(ProjectAnnotation{
			ID: uuid.NewString(), ProjectID: project.ID, Entry: entry,
			Content: "anno", CreatedBy: "u1", CreatedAt: entry,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	recent, err := annoRepo.RecentByProject(project.ID, 2)
	if err != nil {
		t.Fatalf("RecentByProject: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}
	if recent[0].Entry > recent[1].Entry {
		t.Fatal("expected recent results reversed to ASC order")
	}
	if recent[0].Entry != 400 || recent[1].Entry != 500 {
		t.Fatalf("expected entries 400,500 got %d,%d", recent[0].Entry, recent[1].Entry)
	}
}

func TestProjectAnnotationRepoTimeline(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	projectRepo := NewProjectRepository(store.DB())
	taskRepo := NewTaskRepository(store.DB())
	annoRepo := NewProjectAnnotationRepository(store.DB())

	project, err := projectRepo.Create(Project{
		ID: uuid.NewString(), WorkspaceID: ws.ID, Slug: "timeline", Name: "Timeline",
		Status: "active", SettingsJSON: "{}", CreatedAt: 100, ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	_, err = annoRepo.Create(ProjectAnnotation{
		ID: uuid.NewString(), ProjectID: project.ID, Entry: 150,
		Content: "project anno", CreatedBy: "user-1", CreatedAt: 150,
	})
	if err != nil {
		t.Fatalf("create project annotation: %v", err)
	}

	taskUUID := uuid.NewString()
	domainTask := mkDomainTask(taskUUID, ws.ID, "timeline task")
	pid := project.ID
	domainTask.ProjectID = &pid
	domainTask.Project = strPtr("timeline-proj")
	domainTask.Annotations = []domain.Annotation{{Entry: 250, Description: "task anno"}}
	_, err = taskRepo.Create(domainTask)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	rows, err := annoRepo.TimelineByProjectID(project.ID, 10, 0)
	if err != nil {
		t.Fatalf("TimelineByProjectID: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 timeline rows, got %d", len(rows))
	}
	if rows[0].SourceType != "project" || rows[0].Content != "project anno" {
		t.Fatalf("row[0] = %+v", rows[0])
	}
	if rows[1].SourceType != "task" || rows[1].Content != "task anno" {
		t.Fatalf("row[1] = %+v", rows[1])
	}
}
