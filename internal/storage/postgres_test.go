package storage

import (
	"os"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/task"
	"github.com/google/uuid"
)

func postgresTestURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TASKG_TEST_DB_URL")
	if url == "" {
		t.Skip("TASKG_TEST_DB_URL not set, skipping PostgreSQL tests")
	}
	return url
}

func TestOpen_Postgres(t *testing.T) {
	dbURL := postgresTestURL(t)
	store, err := Open(dbURL)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbURL, err)
	}
	defer store.Close()
	if store.Dialect() != "postgres" {
		t.Errorf("expected dialect postgres, got %q", store.Dialect())
	}
}

func TestPostgres_Migration(t *testing.T) {
	dbURL := postgresTestURL(t)
	store, err := Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.DB()
	tables := []string{"meta", "users", "workspaces", "memberships",
		"audit_logs", "projects", "project_annotations", "configs",
		"api_tokens", "contexts", "uda_definitions", "hook_definitions",
		"hook_deliveries", "user_external_ids", "tasks",
		"task_tags", "task_annotations", "task_dependencies",
		"task_assignees", "task_uda_values", "task_links"}
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("table %q not found after migration", table)
		}
	}
}

func TestPostgres_EnsureLocalIdentity(t *testing.T) {
	dbURL := postgresTestURL(t)
	store, err := Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace(): %v", err)
	}
	if ws.Slug != "local" {
		t.Errorf("expected local workspace slug, got %q", ws.Slug)
	}
}

func TestPostgres_TaskCRUD(t *testing.T) {
	dbURL := postgresTestURL(t)
	store, err := Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo := NewTaskRepository(store.DB())
	ws, _ := store.LocalWorkspace()
	now := time.Now().Unix()
	tsk := task.Task{
		UUID:        uuid.NewString(),
		WorkspaceID: ws.ID,
		Description: "PostgreSQL test task",
		Status:      "pending",
		Entry:       now,
		Modified:    now,
	}
	created, err := repo.Create(tsk)
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if created.UUID != tsk.UUID {
		t.Errorf("UUID mismatch: %q vs %q", created.UUID, tsk.UUID)
	}
}
