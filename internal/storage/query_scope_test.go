package storage

import (
	"errors"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/query"
	domain "github.com/dajee/taskg/internal/task"
)

func TestListWithQueryExprSupportsOrAndTags(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	mustCreate(t, repo, domain.Task{UUID: "1", WorkspaceID: ws.ID, Description: "work task", Status: domain.StatusPending, Entry: 1, Modified: 1, Tags: []string{"work"}})
	mustCreate(t, repo, domain.Task{UUID: "2", WorkspaceID: ws.ID, Description: "home task", Status: domain.StatusPending, Entry: 2, Modified: 2, Tags: []string{"home"}})
	t.Cleanup(func() { _ = store.Close() })

	expr, err := query.ParseQuery(`+work or /home/`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.List(ws.ID, ListOptions{Query: expr})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
}

func TestListWithQueryExprUsesBoundParameters(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	mustCreate(t, repo, domain.Task{UUID: "1", WorkspaceID: ws.ID, Description: "safe", Status: domain.StatusPending, Entry: 1, Modified: 1})
	t.Cleanup(func() { _ = store.Close() })

	expr, err := query.ParseQuery(`/x%' OR 1=1 --/`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.List(ws.ID, ListOptions{Query: expr})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("injection-like query matched tasks: %#v", tasks)
	}
}

func TestCompileQueryXorUsesBooleanCoalesce(t *testing.T) {
	expr, err := query.ParseQuery(`+work xor due:`)
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := CompileQuery(expr, QueryCompileOptions{NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	if !strings.Contains(sql, "COALESCE") || !strings.Contains(sql, "<>") {
		t.Fatalf("xor SQL = %s", sql)
	}
}

func TestCompileQueryDescriptionUsesSubstring(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	t.Cleanup(func() { _ = store.Close() })
	mustCreate(t, repo, domain.Task{UUID: "1", WorkspaceID: ws.ID, Description: "write spec", Status: domain.StatusPending, Entry: 1, Modified: 1})
	mustCreate(t, repo, domain.Task{UUID: "2", WorkspaceID: ws.ID, Description: "other", Status: domain.StatusPending, Entry: 1, Modified: 1})

	expr, err := query.ParseQuery(`description:spec`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.List(ws.ID, ListOptions{Query: expr})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != "1" {
		t.Fatalf("tasks = %#v, want only write spec", tasks)
	}
}

func TestCompileQueryDueEmptyIsNull(t *testing.T) {
	expr, err := query.ParseQuery(`due:`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	if sql != "due IS NULL" || len(args) != 0 {
		t.Fatalf("sql = %q args = %#v", sql, args)
	}
}

func TestCompileQueryRejectsUnresolvedProjectPredicate(t *testing.T) {
	expr, err := query.ParseQuery(`project:api`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "ws1"}); !errors.Is(err, query.ErrProjectPredicateUnresolved) {
		t.Fatalf("CompileQuery() err = %v, want ErrProjectPredicateUnresolved", err)
	}
}

func TestCompileQueryInjectsWorkspace(t *testing.T) {
	expr, err := query.ParseQuery(`+work`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	if !strings.Contains(sql, "workspace_id = ?") {
		t.Fatalf("sql = %q, want workspace predicate", sql)
	}
	if len(args) == 0 || args[0] != "ws1" {
		t.Fatalf("args = %#v, want workspace id first", args)
	}
}

func TestCompileQueryDateEqualUsesLocalDayRange(t *testing.T) {
	expr, err := query.ParseQuery(`due:2025-03-09`)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	_, args, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "ws1", NowUnix: time.Date(2025, 3, 1, 12, 0, 0, 0, loc).Unix(), Location: loc})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	if len(args) != 3 {
		t.Fatalf("args = %#v, want workspace + start/end", args)
	}
	start := time.Date(2025, 3, 9, 0, 0, 0, 0, loc).Unix()
	end := time.Date(2025, 3, 10, 0, 0, 0, 0, loc).Unix()
	if args[1] != start || args[2] != end {
		t.Fatalf("range args = %#v, want %d..%d", args[1:], start, end)
	}
}

func TestCompileQueryUDAFilters(t *testing.T) {
	expr, err := query.ParseQuery(`estimate:3 reviewed:2026-05-28 reviewed.notnull legacy:`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{
		WorkspaceID: "w1",
		NowUnix:     100,
		UDADefinitions: map[string]string{
			"estimate": "numeric",
			"reviewed": "date",
			"legacy":   "string",
		},
	})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	for _, part := range []string{"task_uda_values", "CAST(task_uda_values.value AS REAL)", "value >= ?", "value < ?", "NOT EXISTS"} {
		if !strings.Contains(sql, part) {
			t.Fatalf("sql = %s, missing %s", sql, part)
		}
	}
	if len(args) == 0 {
		t.Fatalf("args empty for sql %s", sql)
	}
}

func TestCompileQueryRejectsUnknownUDA(t *testing.T) {
	expr, err := query.ParseQuery(`estimate:3`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "w1"}); err == nil {
		t.Fatal("CompileQuery() error = nil, want unknown UDA")
	}
}

func TestCompileQueryOrphanUDANotNullDoesNotRequireSchema(t *testing.T) {
	expr, err := query.ParseQuery(`uda.legacy_field.notnull`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "w1"})
	if err != nil {
		t.Fatalf("CompileQuery(orphan notnull) error = %v", err)
	}
	if !strings.Contains(sql, "EXISTS") || !strings.Contains(sql, "task_uda_values.name = ?") {
		t.Fatalf("sql = %s", sql)
	}
	if len(args) == 0 || args[len(args)-1] != "legacy_field" {
		t.Fatalf("args = %#v, want legacy_field as UDA name", args)
	}
}

func TestCompileQueryOrphanUDAValueStillRequiresSchema(t *testing.T) {
	expr, err := query.ParseQuery(`legacy_field:old`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "w1"}); err == nil {
		t.Fatal("CompileQuery(orphan value) error = nil, want unknown UDA")
	}
}

func TestResolveDeadlineDateValueUsesEndOfDay(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	got, err := query.ResolveDeadlineDateValue(query.ParseDateValue("2025-03-09"), time.Date(2025, 3, 1, 12, 0, 0, 0, loc).Unix(), loc)
	if err != nil {
		t.Fatalf("ResolveDeadlineDateValue() error = %v", err)
	}
	want := time.Date(2025, 3, 9, 23, 59, 59, 0, loc).Unix()
	if got != want {
		t.Fatalf("deadline = %d, want %d", got, want)
	}
}

func TestQueryXorTruthTable(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	now := int64(1000)
	due := now + 3600
	mustCreate(t, repo, domain.Task{UUID: "a", WorkspaceID: ws.ID, Description: "both true", Status: domain.StatusPending, Entry: 1, Modified: 1, Tags: []string{"work"}})
	mustCreate(t, repo, domain.Task{UUID: "b", WorkspaceID: ws.ID, Description: "tag only", Status: domain.StatusPending, Entry: 1, Modified: 1, Due: &due, Tags: []string{"work"}})
	mustCreate(t, repo, domain.Task{UUID: "c", WorkspaceID: ws.ID, Description: "null due only", Status: domain.StatusPending, Entry: 1, Modified: 1})
	mustCreate(t, repo, domain.Task{UUID: "d", WorkspaceID: ws.ID, Description: "both false", Status: domain.StatusPending, Entry: 1, Modified: 1, Due: &due})
	t.Cleanup(func() { _ = store.Close() })

	expr, err := query.ParseQuery(`+work xor due:`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.List(ws.ID, ListOptions{Query: expr, NowUnix: now})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := taskUUIDs(tasks)
	sort.Strings(got)
	want := []string{"b", "c"}
	if !slices.Equal(got, want) {
		t.Fatalf("uuids = %#v, want %#v", got, want)
	}
}

func TestCompileQueryM2Fields(t *testing.T) {
	expr, err := query.ParseQuery(`start.notnull wait: depends:dep annotations:note recur:weekly parent:p1`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "w1", NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	for _, part := range []string{
		"start IS NOT NULL",
		"wait IS NULL",
		"task_dependencies",
		"task_annotations",
		"recur = ?",
		"parent = ?",
	} {
		if !strings.Contains(sql, part) {
			t.Fatalf("sql = %s, missing %s", sql, part)
		}
	}
	if len(args) == 0 {
		t.Fatalf("args empty for sql %s", sql)
	}
}

func TestCompileQueryAssigneePredicate(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	createQueryTestUser(t, store, User{ID: "user-alice", Name: "alice", CreatedAt: 100, ModifiedAt: 100})
	mustCreate(t, repo, domain.Task{
		UUID:        "task-a",
		WorkspaceID: ws.ID,
		Description: "task a",
		Status:      domain.StatusPending,
		Entry:       1,
		Modified:    1,
		Assignees:   []domain.AssigneeInfo{{UserID: "user-alice"}},
	})
	mustCreate(t, repo, domain.Task{
		UUID:        "task-b",
		WorkspaceID: ws.ID,
		Description: "task b",
		Status:      domain.StatusPending,
		Entry:       1,
		Modified:    1,
	})
	t.Cleanup(func() { _ = store.Close() })

	expr, err := query.ParseQuery(`assignee:user-alice`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: ws.ID})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	if !strings.Contains(sql, "task_assignees") || !strings.Contains(sql, "user_id = ?") {
		t.Fatalf("sql = %q", sql)
	}
	if len(args) < 2 || args[len(args)-1] != "user-alice" {
		t.Fatalf("args = %#v", args)
	}

	tasks, err := repo.List(ws.ID, ListOptions{Query: expr})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != "task-a" {
		t.Fatalf("tasks = %#v, want only task-a", tasks)
	}
}

func TestCompileQueryAssociationSubqueriesAreWorkspaceScoped(t *testing.T) {
	expr, err := query.ParseQuery(`+work depends:dep annotations:note`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "w1", NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	for _, part := range []string{
		"tag_tasks.workspace_id = ?",
		"dep_tasks.workspace_id = ?",
		"annotation_tasks.workspace_id = ?",
	} {
		if !strings.Contains(sql, part) {
			t.Fatalf("sql = %s, missing workspace scope %s", sql, part)
		}
	}
	if got := countArgs(args, "w1"); got != 4 {
		t.Fatalf("workspace args count = %d, args = %#v", got, args)
	}
}

func taskUUIDs(tasks []domain.Task) []string {
	out := make([]string, len(tasks))
	for i, tsk := range tasks {
		out[i] = tsk.UUID
	}
	return out
}

func countArgs(args []any, value string) int {
	count := 0
	for _, arg := range args {
		if arg == value {
			count++
		}
	}
	return count
}

func newQueryTestStore(t *testing.T) (*Store, *TaskRepository, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return store, NewTaskRepository(store.DB()), ws
}

func mustCreate(t *testing.T, repo *TaskRepository, task domain.Task) {
	t.Helper()
	if _, err := repo.Create(task); err != nil {
		t.Fatalf("Create(%s) error = %v", task.UUID, err)
	}
}

func TestCompileQuery_PostgresDialect(t *testing.T) {
	opts := QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "postgres"}
	expr := query.Predicate{Attribute: query.AttrBare, Operator: query.OpEqual, Value: query.ParseDateValue("hello")}
	sql, _, err := CompileQuery(expr, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "ILIKE") {
		t.Errorf("expected ILIKE, got: %s", sql)
	}
}

func TestCompileQuery_SQLiteDialect(t *testing.T) {
	opts := QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "sqlite"}
	expr := query.Predicate{Attribute: query.AttrBare, Operator: query.OpEqual, Value: query.ParseDateValue("hello")}
	sql, _, err := CompileQuery(expr, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, " LIKE ") {
		t.Errorf("expected LIKE, got: %s", sql)
	}
}

func TestCompileQuery_UDANumericPostgres(t *testing.T) {
	opts := QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "postgres", UDADefinitions: map[string]string{"score": "numeric"}}
	expr := query.Predicate{Attribute: query.AttrUDA, Field: "score", Operator: query.OpEqual, Value: query.ParseDateValue("42")}
	sql, _, err := CompileQuery(expr, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "DOUBLE PRECISION") {
		t.Errorf("expected DOUBLE PRECISION, got: %s", sql)
	}
}

func TestCompileQuery_CompareColumnContains(t *testing.T) {
	opts := QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "postgres"}
	expr := query.Predicate{Attribute: query.AttrStatus, Operator: query.OpContains, Value: query.ParseDateValue("pend")}
	sql, _, err := CompileQuery(expr, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "ILIKE") {
		t.Errorf("expected ILIKE for OpContains, got: %s", sql)
	}
}

func createQueryTestUser(t *testing.T, store *Store, user User) User {
	t.Helper()
	created, err := NewUserRepository(store.DB()).Create(user)
	if err != nil {
		t.Fatalf("Create(user %s) error = %v", user.ID, err)
	}
	return created
}
