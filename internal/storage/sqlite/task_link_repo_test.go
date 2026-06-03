package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestTaskLinkRepoCreateAndGet(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	taskRepo := NewTaskRepository(store.DB())
	linkRepo := NewTaskLinkRepository(store.DB())

	taskUUID := uuid.NewString()
	_, err = taskRepo.Create(mkDomainTask(taskUUID, ws.ID, "test task"))
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	link := TaskLink{
		ID:        uuid.NewString(),
		TaskUUID:  taskUUID,
		Type:      "document",
		URL:       "https://example.com/doc",
		Title:     "需求文档",
		CreatedAt: 1700000000,
		CreatedBy: "user-1",
	}

	created, err := linkRepo.Create(link)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	found, err := linkRepo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if found.TaskUUID != taskUUID {
		t.Fatalf("expected task_uuid %s, got %s", taskUUID, found.TaskUUID)
	}
	if found.Type != "document" {
		t.Fatalf("expected type document, got %s", found.Type)
	}
	if found.URL != "https://example.com/doc" {
		t.Fatalf("expected url https://example.com/doc, got %s", found.URL)
	}
}

func TestTaskLinkRepoCreateDuplicateURLFails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	taskRepo := NewTaskRepository(store.DB())
	linkRepo := NewTaskLinkRepository(store.DB())

	taskUUID := uuid.NewString()
	_, err = taskRepo.Create(mkDomainTask(taskUUID, ws.ID, "test task"))
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	link1 := TaskLink{
		ID: uuid.NewString(), TaskUUID: taskUUID, Type: "document",
		URL: "https://example.com/dup", Title: "", CreatedAt: 1700000000, CreatedBy: "user-1",
	}
	if _, err := linkRepo.Create(link1); err != nil {
		t.Fatalf("first create: %v", err)
	}

	link2 := TaskLink{
		ID: uuid.NewString(), TaskUUID: taskUUID, Type: "pr",
		URL: "https://example.com/dup", Title: "", CreatedAt: 1700000001, CreatedBy: "user-1",
	}
	if _, err := linkRepo.Create(link2); err == nil {
		t.Fatal("expected duplicate (task_uuid, url) to fail")
	}
}

func TestTaskLinkRepoListByTaskUUID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	taskRepo := NewTaskRepository(store.DB())
	linkRepo := NewTaskLinkRepository(store.DB())

	taskUUID := uuid.NewString()
	_, err = taskRepo.Create(mkDomainTask(taskUUID, ws.ID, "test task"))
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	for _, link := range []TaskLink{
		{ID: uuid.NewString(), TaskUUID: taskUUID, Type: "document", URL: "https://a.com", CreatedAt: 1700000000, CreatedBy: "u1"},
		{ID: uuid.NewString(), TaskUUID: taskUUID, Type: "pr", URL: "https://b.com", CreatedAt: 1700000001, CreatedBy: "u1"},
	} {
		if _, err := linkRepo.Create(link); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	links, err := linkRepo.ListByTaskUUID(taskUUID)
	if err != nil {
		t.Fatalf("list by task uuid: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].CreatedAt > links[1].CreatedAt {
		t.Fatal("expected links sorted by created_at ASC")
	}
}

func TestTaskLinkRepoGetByIDNotFound(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	linkRepo := NewTaskLinkRepository(store.DB())

	_, err = linkRepo.GetByID("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestTaskLinkRepoDelete(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	taskRepo := NewTaskRepository(store.DB())
	linkRepo := NewTaskLinkRepository(store.DB())

	taskUUID := uuid.NewString()
	_, err = taskRepo.Create(mkDomainTask(taskUUID, ws.ID, "test task"))
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	created, err := linkRepo.Create(TaskLink{
		ID: uuid.NewString(), TaskUUID: taskUUID, Type: "document",
		URL: "https://example.com/del", Title: "", CreatedAt: 1700000000, CreatedBy: "u1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := linkRepo.Delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = linkRepo.GetByID(created.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestTaskLinkRepoDeleteNotFound(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	linkRepo := NewTaskLinkRepository(store.DB())

	err = linkRepo.Delete("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestTaskLinkRepoLoadByTaskUUIDs(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	taskRepo := NewTaskRepository(store.DB())
	linkRepo := NewTaskLinkRepository(store.DB())

	task1 := uuid.NewString()
	task2 := uuid.NewString()
	_, _ = taskRepo.Create(mkDomainTask(task1, ws.ID, "task 1"))
	_, _ = taskRepo.Create(mkDomainTask(task2, ws.ID, "task 2"))

	linkRepo.Create(TaskLink{ID: uuid.NewString(), TaskUUID: task1, Type: "document", URL: "https://a.com", CreatedAt: 1700000000, CreatedBy: "u1"})
	linkRepo.Create(TaskLink{ID: uuid.NewString(), TaskUUID: task1, Type: "pr", URL: "https://b.com", CreatedAt: 1700000001, CreatedBy: "u1"})
	linkRepo.Create(TaskLink{ID: uuid.NewString(), TaskUUID: task2, Type: "ticket", URL: "https://c.com", CreatedAt: 1700000002, CreatedBy: "u1"})

	result, err := linkRepo.LoadByTaskUUIDs([]string{task1, task2})
	if err != nil {
		t.Fatalf("load by task uuids: %v", err)
	}
	if len(result[task1]) != 2 {
		t.Fatalf("expected 2 links for task1, got %d", len(result[task1]))
	}
	if len(result[task2]) != 1 {
		t.Fatalf("expected 1 link for task2, got %d", len(result[task2]))
	}
}

func TestTaskLinkRepoLoadByTaskUUIDsEmpty(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	linkRepo := NewTaskLinkRepository(store.DB())

	result, err := linkRepo.LoadByTaskUUIDs(nil)
	if err != nil {
		t.Fatalf("load by task uuids: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result))
	}
}
