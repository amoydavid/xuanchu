package storage

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"github.com/google/uuid"
)

func postgresTestURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("XUANCHU_TEST_DB_URL")
	if url == "" {
		t.Skip("XUANCHU_TEST_DB_URL not set, skipping PostgreSQL tests")
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
		"audit_logs", "projects", "project_annotations", "configs", "config_definitions",
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

func TestPostgresRegclassFoundHandlesNull(t *testing.T) {
	if postgresRegclassFound(sql.NullString{}) {
		t.Fatal("NULL regclass should be treated as table not found")
	}
	if !postgresRegclassFound(sql.NullString{String: "task_annotations", Valid: true}) {
		t.Fatal("valid regclass should be treated as table found")
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
		Title:       "PostgreSQL test task",
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

func TestPostgres_SummarizeSeriesOccurrences(t *testing.T) {
	dbURL := postgresTestURL(t)
	store, err := Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace: %v", err)
	}

	tx := store.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("Begin: %v", tx.Error)
	}
	defer tx.Rollback() //nolint:errcheck -- 测试事务只用于隔离 fixture。

	suffix := uuid.NewString()
	project := Project{
		ID: "project-" + suffix, WorkspaceID: ws.ID, Slug: "pg-" + suffix,
		Name: "PostgreSQL summary", Status: "active", CreatedAt: 1, ModifiedAt: 1,
	}
	if err := tx.Create(&project).Error; err != nil {
		t.Fatalf("Create project: %v", err)
	}
	series, err := NewTaskSeriesRepository(tx).Create(taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: project.ID, Title: "PostgreSQL summary",
		Status: taskseries.StatusActive, RecurrenceRule: "daily", FirstDue: 100,
		CreatedBy: "postgres-test", CreatedAt: 1, ModifiedAt: 1,
	})
	if err != nil {
		t.Fatalf("Create series: %v", err)
	}
	rule := "daily"
	overdue := int64(50)
	create := func(status string, slot int64, due *int64) {
		t.Helper()
		seriesID := series.ID
		row := task.Task{
			UUID: uuid.NewString(), WorkspaceID: ws.ID, Title: status, Status: status,
			Entry: 1, Modified: 1, ProjectID: &project.ID, Project: &project.Slug, Due: due,
			SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		}
		if _, _, err := NewTaskOccurrenceRepository(tx).CreateOccurrence(row); err != nil {
			t.Fatalf("CreateOccurrence(%s): %v", status, err)
		}
	}
	create(task.StatusPending, 100, &overdue)
	create(task.StatusWaiting, 200, &overdue)
	create(task.StatusCompleted, 300, &overdue)
	create(task.StatusDeleted, 400, &overdue)

	summaries, err := NewTaskOccurrenceRepository(tx).SummarizeSeriesOccurrences(ws.ID, []string{series.ID}, 100)
	if err != nil {
		t.Fatalf("SummarizeSeriesOccurrences: %v", err)
	}
	got := summaries[series.ID]
	want := OccurrenceCounts{Open: 2, Pending: 1, Waiting: 1, Completed: 1, Deleted: 1, Overdue: 2}
	if got.Counts != want || got.MaxRecurrenceAt != 400 {
		t.Fatalf("summary = %#v, want counts=%#v max=400", got, want)
	}
}

func TestPostgres_ConfigDefinitionCRUD(t *testing.T) {
	dbURL := postgresTestURL(t)
	store, err := Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	repo := NewConfigDefinitionRepository(store.DB())
	def := ConfigDefinition{
		WorkspaceID:       "ws-postgres",
		Key:               "ads.account_id",
		ValueType:         "string",
		AllowedScopesJSON: `["project"]`,
		EnumValuesJSON:    "[]",
		CreatedAt:         time.Now().Unix(),
		ModifiedAt:        time.Now().Unix(),
	}
	if err := repo.Set(def); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, ok, err := repo.Get("ws-postgres", "ads.account_id")
	if err != nil || !ok {
		t.Fatalf("Get() = %#v, %v, %v", got, ok, err)
	}
	if got.ValueType != "string" {
		t.Fatalf("ValueType = %q, want string", got.ValueType)
	}

	def.ShowOnConsoleHome = true
	def.ModifiedAt = time.Now().Unix()
	if err := repo.Set(def); err != nil {
		t.Fatalf("Set(show_on_console_home) error = %v", err)
	}
	got, _, err = repo.Get("ws-postgres", "ads.account_id")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !got.ShowOnConsoleHome {
		t.Fatal("ShowOnConsoleHome = false, want true")
	}
}
