package sqlite

import (
	"path/filepath"
	"testing"

	domain "github.com/dajee/taskg/internal/task"
)

func TestTaskRepositoryCreateAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	repo := NewTaskRepository(store.DB())

	created, err := repo.Create(domain.Task{
		UUID:        "task-1",
		WorkspaceID: ws.ID,
		Description: "write spec",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Tags:        []string{"planning"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.UUID != "task-1" {
		t.Fatalf("UUID = %q", created.UUID)
	}

	tasks, err := repo.List(ws.ID, ListOptions{Status: domain.StatusPending})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "write spec" {
		t.Fatalf("tasks = %#v", tasks)
	}
	if len(tasks[0].Tags) != 1 || tasks[0].Tags[0] != "planning" {
		t.Fatalf("tags = %#v", tasks[0].Tags)
	}
}

func TestTaskRepositoryUpdateReplacesTags(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	repo := NewTaskRepository(store.DB())

	_, err = repo.Create(domain.Task{
		UUID: "task-1", WorkspaceID: ws.ID, Description: "write spec",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
		Tags: []string{"old"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tsk, err := repo.GetByUUID(ws.ID, "task-1")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	tsk.Tags = []string{"new"}
	tsk.Modified = 200
	if err := repo.Update(tsk); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByUUID(ws.ID, "task-1")
	if err != nil {
		t.Fatalf("GetByUUID() after update error = %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "new" {
		t.Fatalf("Tags = %#v", got.Tags)
	}
}
