package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestIsPostgresURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"postgres://user:pass@host:5432/db", true},
		{"postgresql://user:pass@host:5432/db", true},
		{"", false},
		{"/path/to/xuanchu.db", false},
		{"xuanchu.db", false},
		{"mysql://host/db", false},
	}
	for _, tt := range tests {
		if got := isPostgresURL(tt.input); got != tt.want {
			t.Errorf("isPostgresURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestOpen_UnsupportedScheme(t *testing.T) {
	_, err := Open("mysql://host/db")
	if err == nil {
		t.Error("expected error for unsupported scheme")
	}
	if !strings.Contains(err.Error(), "unsupported database scheme") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTransactionPreservesDialect(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Transaction(func(txStore *Store) error {
		if got := txStore.Dialect(); got != store.Dialect() {
			t.Fatalf("transaction Dialect() = %q, want %q", got, store.Dialect())
		}
		return nil
	}); err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}
}

func TestOpenInitializesLocalUserWorkspaceAndMembership(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	if user.Email != nil {
		t.Fatalf("local email = %q, want nil", *user.Email)
	}

	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID != ws.ID {
		t.Fatalf("default workspace = %#v, want %s", user.DefaultWorkspaceID, ws.ID)
	}
	if ws.ID == "" {
		t.Fatal("workspace ID is empty")
	}
	if ws.Slug != "local" {
		t.Fatalf("Slug = %q, want local", ws.Slug)
	}
	if ws.Name != "Local" {
		t.Fatalf("Name = %q, want Local", ws.Name)
	}

	member, err := NewMemberRepository(store.DB()).Get(user.ID, ws.ID)
	if err != nil {
		t.Fatalf("Get(local membership) error = %v", err)
	}
	if member.Role != "owner" {
		t.Fatalf("role = %q, want owner", member.Role)
	}
}

func TestOpenCanReopenExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")

	store1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open first error = %v", err)
	}
	ws1, err := store1.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace first error = %v", err)
	}
	_ = store1.Close()

	store2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open second error = %v", err)
	}
	t.Cleanup(func() { _ = store2.Close() })
	ws2, err := store2.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace second error = %v", err)
	}
	if ws2.ID != ws1.ID {
		t.Fatalf("workspace ID changed: %q -> %q", ws1.ID, ws2.ID)
	}
}

func TestDBMigratesAuditDelegatorColumns(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if !store.DB().Migrator().HasColumn(&AuditLog{}, "delegator_token_id") {
		t.Fatal("audit_logs.delegator_token_id column missing after migration")
	}
	if !store.DB().Migrator().HasColumn(&AuditLog{}, "delegator_user_id") {
		t.Fatal("audit_logs.delegator_user_id column missing after migration")
	}
	for _, column := range []string{"actor_type", "actor_token_id", "actor_token_name", "actor_token_prefix"} {
		if !store.DB().Migrator().HasColumn(&AuditLog{}, column) {
			t.Fatalf("audit_logs.%s column missing after migration", column)
		}
	}
}

func TestOpenMigratesAPITokenUserIDNullable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.Exec(`
CREATE TABLE api_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  token_prefix TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  scopes_json TEXT NOT NULL DEFAULT '[]',
  workspace_ids_json TEXT NOT NULL DEFAULT '[]',
  project_ids_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  expires_at INTEGER,
  revoked_at INTEGER,
  last_used_at INTEGER
)`).Error; err != nil {
		t.Fatalf("create old api_tokens: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	assertColumnNullable(t, store, "api_tokens", "user_id", true)
	assertIndexColumns(t, store, "idx_api_tokens_user", []string{"user_id"})
	assertIndexColumns(t, store, "idx_api_tokens_prefix", []string{"token_prefix"})
}

func TestTaskAssigneeTableMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if !store.DB().Migrator().HasTable(&TaskAssignee{}) {
		t.Fatal("task_assignees table missing after migration")
	}
	if !store.DB().Migrator().HasIndex(&TaskAssignee{}, "idx_task_assignees_user_id") {
		t.Fatal("idx_task_assignees_user_id missing after migration")
	}
}

func TestUserExternalIDTableMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if !store.DB().Migrator().HasTable(&UserExternalID{}) {
		t.Fatal("user_external_ids table missing after migration")
	}
	if !store.DB().Migrator().HasIndex(&UserExternalID{}, "idx_user_ext_id_provider_value") {
		t.Fatal("idx_user_ext_id_provider_value missing after migration")
	}
}

func TestTaskLinkTableMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if !store.DB().Migrator().HasTable(&TaskLink{}) {
		t.Fatal("task_links table missing after migration")
	}
	if !store.DB().Migrator().HasIndex(&TaskLink{}, "idx_task_links_task_url") {
		t.Fatal("idx_task_links_task_url missing after migration")
	}
}

func TestAuditTargetTimeIndexMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if !store.DB().Migrator().HasIndex(&AuditLog{}, "idx_audit_target_time") {
		t.Fatal("idx_audit_target_time missing after migration")
	}
}

func TestConfigDefinitionTableMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if !store.DB().Migrator().HasTable(&ConfigDefinition{}) {
		t.Fatal("config_definitions table missing after migration")
	}
	if !store.DB().Migrator().HasColumn(&ConfigDefinition{}, "show_on_console_home") {
		t.Fatal("config_definitions.show_on_console_home column missing after migration")
	}
}

func TestNotificationSinkMaxConcurrencyColumnMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if !store.DB().Migrator().HasColumn(&NotificationSink{}, "max_concurrency") {
		t.Fatal("notification_sinks.max_concurrency column missing after migration")
	}

	var notNull int
	var defaultValue sql.NullString
	err = store.DB().Raw(`SELECT "notnull", dflt_value FROM pragma_table_info('notification_sinks') WHERE name = ?`, "max_concurrency").
		Row().
		Scan(&notNull, &defaultValue)
	if err != nil {
		t.Fatalf("pragma_table_info(max_concurrency) error = %v", err)
	}
	if notNull != 1 {
		t.Fatalf("max_concurrency notnull = %d, want 1", notNull)
	}
	if !defaultValue.Valid || defaultValue.String != "0" {
		t.Fatalf("max_concurrency default = %q valid=%v, want 0", defaultValue.String, defaultValue.Valid)
	}
}

func TestOpenEnablesForeignKeysForPooledConnections(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	sqlDB, err := store.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(2)

	ctx := context.Background()
	conn1, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Close()
	conn2, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()

	for i, conn := range []*sql.Conn{conn1, conn2} {
		var enabled int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			t.Fatalf("conn %d PRAGMA foreign_keys error = %v", i+1, err)
		}
		if enabled != 1 {
			t.Fatalf("conn %d foreign_keys = %d, want 1", i+1, enabled)
		}
	}
}

func TestM5MigrationRestoresForeignKeysWhenBeginImmediateFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	db, err := gorm.Open(sqlite.Open(sqliteDSN(dbPath)+"&_pragma=busy_timeout(1)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(2)

	ctx := context.Background()
	lockConn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lockConn.Close()
	if _, err := lockConn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("BEGIN IMMEDIATE lock holder error = %v", err)
	}
	defer func() { _, _ = lockConn.ExecContext(ctx, "ROLLBACK") }()

	store := &Store{db: db}
	if err := store.prepareProjectSchemaForM5(); err == nil {
		t.Fatal("prepareProjectSchemaForM5() succeeded, want BEGIN IMMEDIATE failure")
	}

	if _, err := lockConn.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatalf("ROLLBACK lock holder error = %v", err)
	}

	checkConn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer checkConn.Close()
	var enabled int
	if err := checkConn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("PRAGMA foreign_keys error = %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, want 1", enabled)
	}
}

func TestOpenMigratesContextActiveMeta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.SetMeta("context.active", "work"); err != nil {
		t.Fatalf("SetMeta(context.active) error = %v", err)
	}
	_ = store.Close()

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	user, err := NewUserRepository(reopened.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	ws, err := reopened.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	if _, ok, err := reopened.GetMeta("context.active"); err != nil {
		t.Fatalf("GetMeta(context.active) error = %v", err)
	} else if ok {
		t.Fatal("old context.active key still exists")
	}
	got, ok, err := reopened.GetMeta("active_context." + user.ID + "." + ws.ID)
	if err != nil || !ok || got != "work" {
		t.Fatalf("migrated active context = %q, %v, %v", got, ok, err)
	}
}

func TestOpenMigratesM3WorkspaceRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(initial) error = %v", err)
	}
	if err := store.DB().Migrator().DropTable(&AuditLog{}, &Membership{}, &User{}); err != nil {
		t.Fatalf("DropTable(identity) error = %v", err)
	}
	if err := store.DB().Migrator().DropColumn(&Workspace{}, "name"); err != nil {
		t.Fatalf("DropColumn(name) error = %v", err)
	}
	if err := store.DB().Migrator().DropColumn(&Workspace{}, "created_by_user_id"); err != nil {
		t.Fatalf("DropColumn(created_by_user_id) error = %v", err)
	}
	if err := store.DB().Migrator().DropColumn(&Workspace{}, "description"); err != nil {
		t.Fatalf("DropColumn(description) error = %v", err)
	}
	if err := store.DB().Migrator().DropColumn(&Workspace{}, "visibility"); err != nil {
		t.Fatalf("DropColumn(visibility) error = %v", err)
	}
	if err := store.DB().Migrator().DropColumn(&Workspace{}, "settings_json"); err != nil {
		t.Fatalf("DropColumn(settings_json) error = %v", err)
	}
	if err := store.DB().Migrator().DropColumn(&Workspace{}, "archived_at"); err != nil {
		t.Fatalf("DropColumn(archived_at) error = %v", err)
	}
	if err := store.DB().Migrator().DropColumn(&Workspace{}, "modified_at"); err != nil {
		t.Fatalf("DropColumn(modified_at) error = %v", err)
	}
	if err := store.SetMeta("context.active", "work"); err != nil {
		t.Fatalf("SetMeta(context.active) error = %v", err)
	}
	_ = store.Close()

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(migrated) error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	ws, err := reopened.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	if ws.Name != "Local" || ws.Visibility != "private" || ws.SettingsJSON != "{}" || ws.ModifiedAt != ws.CreatedAt {
		t.Fatalf("migrated workspace = %#v", ws)
	}
	user, err := NewUserRepository(reopened.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID != ws.ID {
		t.Fatalf("default workspace = %#v, want %s", user.DefaultWorkspaceID, ws.ID)
	}
	if _, ok, err := reopened.GetMeta("context.active"); err != nil {
		t.Fatalf("GetMeta(context.active) error = %v", err)
	} else if ok {
		t.Fatal("old context.active key still exists")
	}
}

func TestOpenCreatesM5ProjectAndConfigSchema(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, table := range []any{&Project{}, &Config{}} {
		if !store.DB().Migrator().HasTable(table) {
			t.Fatalf("missing table for %T", table)
		}
	}
	if !store.DB().Migrator().HasColumn(&AuditLog{}, "project_id") {
		t.Fatalf("missing audit_logs.project_id")
	}

	assertRawDDLContains(t, store, "configs", "PRIMARY KEY (`workspace_id`,`scope`,`scope_id`,`key`)")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES(NULL, 'server', '', 'x', 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', NULL, '', 'x', 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', 'server', NULL, 'x', 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', 'server', '', NULL, 'y')")
	assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES('', 'server', '', 'x', NULL)")
	assertIndexColumns(t, store, "idx_projects_ws_slug", []string{"workspace_id", "slug"})
	assertIndexColumns(t, store, "idx_projects_id_ws", []string{"id", "workspace_id"})
}

func TestOpenMigratesM4ProjectStrings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4DatabaseWithTasks(t, dbPath, []seedTask{
		{WorkspaceSlug: "local", UUID: "t1", Project: ptrString("CustomerA"), Entry: 10},
		{WorkspaceSlug: "local", UUID: "t2", Project: ptrString("customerb"), Entry: 20},
		{WorkspaceSlug: "local", UUID: "t3", Project: nil, Entry: 30},
	})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var projects []Project
	if err := store.DB().Order("slug").Find(&projects).Error; err != nil {
		t.Fatal(err)
	}
	if got := projectSlugs(projects); !reflect.DeepEqual(got, []string{"customera", "customerb"}) {
		t.Fatalf("project slugs = %#v", got)
	}
	assertTaskProject(t, store, "t1", "customera", projects[0].ID)
	assertTaskProject(t, store, "t2", "customerb", projects[1].ID)
	assertTaskHasNoProject(t, store, "t3")
	assertMigrationSkippedReport(t, store, nil)
}

func TestOpenMigratesM5ProjectStringsIdempotently(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4DatabaseWithTasks(t, dbPath, []seedTask{
		{WorkspaceSlug: "local", UUID: "t1", Project: ptrString("api"), Entry: 10},
		{WorkspaceSlug: "local", UUID: "t2", Project: ptrString("web"), Entry: 20},
	})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var first []Project
	if err := store.DB().Order("slug").Find(&first).Error; err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("first projects = %#v", first)
	}
	_ = store.Close()

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	var second []Project
	if err := reopened.DB().Order("slug").Find(&second).Error; err != nil {
		t.Fatal(err)
	}
	if got := projectSlugs(second); !reflect.DeepEqual(got, []string{"api", "web"}) {
		t.Fatalf("project slugs after reopen = %#v", got)
	}
	if second[0].ID != first[0].ID || second[1].ID != first[1].ID {
		t.Fatalf("project IDs changed: %#v -> %#v", first, second)
	}
	assertTaskProject(t, reopened, "t1", "api", second[0].ID)
	assertTaskProject(t, reopened, "t2", "web", second[1].ID)
}

func TestOpenMigratesInvalidAndConflictingProjects(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4DatabaseWithTasks(t, dbPath, []seedTask{
		{WorkspaceSlug: "local", UUID: "invalid", Project: ptrString("Bad Project!"), Entry: 10},
		{WorkspaceSlug: "local", UUID: "conflict-a", Project: ptrString("API"), Entry: 20},
		{WorkspaceSlug: "local", UUID: "conflict-b", Project: ptrString(" api "), Entry: 30},
		{WorkspaceSlug: "local", UUID: "valid", Project: ptrString("web"), Entry: 40},
	})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var projects []Project
	if err := store.DB().Order("slug").Find(&projects).Error; err != nil {
		t.Fatal(err)
	}
	if got := projectSlugs(projects); !reflect.DeepEqual(got, []string{"web"}) {
		t.Fatalf("project slugs = %#v", got)
	}
	assertTaskHasNoProject(t, store, "invalid")
	assertTaskHasNoProject(t, store, "conflict-a")
	assertTaskHasNoProject(t, store, "conflict-b")
	assertTaskProject(t, store, "valid", "web", projects[0].ID)
	assertMigrationSkippedReport(t, store, []string{
		"conflict-a:conflict",
		"conflict-b:conflict",
		"invalid:invalid_slug",
	})
}

func TestOpenMigratesTrimmedRawProjectConflicts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4DatabaseWithTasks(t, dbPath, []seedTask{
		{WorkspaceSlug: "local", UUID: "plain", Project: ptrString("api"), Entry: 10},
		{WorkspaceSlug: "local", UUID: "spaced", Project: ptrString(" api "), Entry: 20},
	})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var projects []Project
	if err := store.DB().Order("slug").Find(&projects).Error; err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("projects = %#v, want none because raw values conflict", projects)
	}
	assertTaskHasNoProject(t, store, "plain")
	assertTaskHasNoProject(t, store, "spaced")
	assertMigrationSkippedReport(t, store, []string{
		"plain:conflict",
		"spaced:conflict",
	})
}

func TestM5MigrationRollsBackOnCopyFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4DatabaseWithTasks(t, dbPath, []seedTask{
		{WorkspaceSlug: "local", UUID: "task-1", Project: ptrString("api"), Entry: 10},
	})
	mutateM4Database(t, dbPath, "ALTER TABLE tasks DROP COLUMN i_mask")

	store, err := Open(dbPath)
	if err == nil {
		_ = store.Close()
		t.Fatal("Open() succeeded, want migration copy failure")
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if !rawTableExists(t, db, "tasks") {
		t.Fatal("tasks table missing after failed migration rollback")
	}
	if rawTableExists(t, db, "tasks_old_m5") {
		t.Fatal("tasks_old_m5 left behind after failed migration rollback")
	}
	var markerCount int
	if err := db.Raw("SELECT COUNT(*) FROM meta WHERE key = ?", m5ProjectsAppliedMetaKey).Scan(&markerCount).Error; err != nil {
		t.Fatal(err)
	}
	if markerCount != 0 {
		t.Fatalf("migration marker count = %d, want 0", markerCount)
	}
}

func TestOpenEnablesForeignKeyChecks(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	project := Project{
		ID:           "project-1",
		WorkspaceID:  "other-workspace",
		Slug:         "api",
		Name:         "api",
		Description:  "",
		Status:       "active",
		SettingsJSON: "{}",
		CreatedAt:    1,
		ModifiedAt:   1,
	}
	if err := store.DB().Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	err = store.DB().Exec(`INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project, project_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		"task-1", ws.ID, "x", "pending", 1, 1, "api", project.ID).Error
	if err == nil {
		t.Fatal("cross-workspace task project_id insert succeeded, want FK failure")
	}
}

func TestM5MigrationColumnsMatchM4Snapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got := taskColumnNames(t, store)
	// spec 2026-07-11 后 tasks 表移除 recur/mask/i_mask，新增 occurrence 列。
	want := []string{
		"uuid", "workspace_id", "title", "description", "status", "entry", "modified",
		"end_ts", "due", "project", "priority",
		"start", "wait", "scheduled", "until",
		"parent",
		"project_seq", "project_id",
		"series_id", "recurrence_at", "recurrence_rule_snapshot", "recurrence_overrides_json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tasks columns = %#v, want %#v", got, want)
	}
}

func TestM5MigrationIndexesMatchM5Snapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got := taskIndexNames(t, store)
	want := []string{
		"idx_tasks_due",
		"idx_tasks_parent",
		"idx_tasks_recurrence_at",
		"idx_tasks_scheduled",
		"idx_tasks_status",
		"idx_tasks_until",
		"idx_tasks_wait",
		"idx_tasks_ws_project_id",
		"idx_tasks_ws_project_seq",
		"idx_tasks_ws_series_slot",
		"sqlite_autoindex_tasks_1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tasks indexes = %#v, want %#v", got, want)
	}
}

func TestTaskSlugMigrationBackfillsOldM5Schema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedOldM5DatabaseForTaskSlug(t, dbPath, "api")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if !store.DB().Migrator().HasColumn(&Task{}, "project_seq") {
		t.Fatal("tasks.project_seq missing after v0.1.1 migration")
	}
	if !store.DB().Migrator().HasColumn(&Project{}, "next_task_seq") {
		t.Fatal("projects.next_task_seq missing after v0.1.1 migration")
	}

	type seqRow struct {
		UUID       string
		ProjectSeq int64
	}
	var rows []seqRow
	if err := store.DB().Raw("SELECT uuid, project_seq FROM tasks ORDER BY entry ASC, uuid ASC").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, []seqRow{{UUID: "task-a", ProjectSeq: 1}, {UUID: "task-b", ProjectSeq: 2}}) {
		t.Fatalf("backfilled project_seq rows = %#v", rows)
	}
	var nextSeq int64
	if err := store.DB().Raw("SELECT next_task_seq FROM projects WHERE id = ?", "project-api").Scan(&nextSeq).Error; err != nil {
		t.Fatal(err)
	}
	if nextSeq != 3 {
		t.Fatalf("next_task_seq = %d, want 3", nextSeq)
	}
	if _, ok, err := store.GetMeta(taskSlugMigrationMetaKey); err != nil || !ok {
		t.Fatalf("task slug migration marker ok=%v err=%v, want marker", ok, err)
	}
	if !slices.Contains(taskIndexNames(t, store), "idx_tasks_ws_project_seq") {
		t.Fatalf("idx_tasks_ws_project_seq missing: %#v", taskIndexNames(t, store))
	}
}

func TestTaskSlugMigrationRejectsOldIllegalProjectSlug(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedOldM5DatabaseForTaskSlug(t, dbPath, "web-app")

	_, err := Open(dbPath)
	if err == nil || !strings.Contains(err.Error(), "project_slug_migration_required") {
		t.Fatalf("Open(illegal old slug) error = %v, want project_slug_migration_required", err)
	}
}

func TestTaskSlugMigrationNewSchemaIncludesProjectSeq(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if !store.DB().Migrator().HasColumn(&Task{}, "project_seq") {
		t.Fatal("new schema missing tasks.project_seq")
	}
	if !store.DB().Migrator().HasColumn(&Project{}, "next_task_seq") {
		t.Fatal("new schema missing projects.next_task_seq")
	}
	if !slices.Contains(taskIndexNames(t, store), "idx_tasks_ws_project_seq") {
		t.Fatalf("new schema missing idx_tasks_ws_project_seq: %#v", taskIndexNames(t, store))
	}
	if _, ok, err := store.GetMeta(taskSlugMigrationMetaKey); err != nil || !ok {
		t.Fatalf("task slug migration marker ok=%v err=%v, want marker", ok, err)
	}
}

func TestM5MigrationPreservesTaskIndexesFromM4Database(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4DatabaseWithTasks(t, dbPath, []seedTask{
		{WorkspaceSlug: "local", UUID: "task-1", Project: ptrString("api"), Entry: 10},
	})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got := taskIndexNames(t, store)
	want := []string{
		"idx_tasks_due",
		"idx_tasks_parent",
		"idx_tasks_recurrence_at",
		"idx_tasks_scheduled",
		"idx_tasks_status",
		"idx_tasks_until",
		"idx_tasks_wait",
		"idx_tasks_ws_project_id",
		"idx_tasks_ws_project_seq",
		"idx_tasks_ws_series_slot",
		"sqlite_autoindex_tasks_1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migrated tasks indexes = %#v, want %#v", got, want)
	}
}

func TestM5MigrationPreservesTaskRelations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4DatabaseWithTasks(t, dbPath, []seedTask{
		{WorkspaceSlug: "local", UUID: "dep", Project: ptrString("api"), Entry: 10},
		{
			WorkspaceSlug: "local",
			UUID:          "task-1",
			Project:       ptrString("api"),
			Entry:         20,
			Tags:          []string{"one", "two"},
			Annotations:   []seedAnnotation{{Entry: 21, Description: "note"}},
			Depends:       []string{"dep"},
			UDAs:          []seedUDA{{Name: "legacy", Value: "v", ValueType: "string", Orphan: true}},
		},
	})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var tags []string
	if err := store.DB().Raw("SELECT tag FROM task_tags WHERE task_uuid = ? ORDER BY tag", "task-1").Scan(&tags).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tags, []string{"one", "two"}) {
		t.Fatalf("tags = %#v", tags)
	}
	var note string
	if err := store.DB().Raw("SELECT description FROM task_annotations WHERE task_uuid = ?", "task-1").Scan(&note).Error; err != nil {
		t.Fatal(err)
	}
	if note != "note" {
		t.Fatalf("annotation = %q", note)
	}
	var dep string
	if err := store.DB().Raw("SELECT depends_on FROM task_dependencies WHERE task_uuid = ?", "task-1").Scan(&dep).Error; err != nil {
		t.Fatal(err)
	}
	if dep != "dep" {
		t.Fatalf("depends_on = %q", dep)
	}
	var uda TaskUDAValue
	if err := store.DB().Where("task_uuid = ? AND name = ?", "task-1", "legacy").First(&uda).Error; err != nil {
		t.Fatal(err)
	}
	if uda.Value != "v" || uda.ValueType != "string" || !uda.Orphan {
		t.Fatalf("uda = %#v", uda)
	}
}

func TestM5MigrationRebuildsM4RelationForeignKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seedM4GORMDatabaseWithTaskRelations(t, dbPath)

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, table := range []string{"task_tags", "task_annotations", "task_dependencies", "task_uda_values"} {
		assertRelationForeignKeysPointToTasks(t, store, table)
	}
	if rawTableExists(t, store.DB(), "tasks_old_m5") {
		t.Fatal("tasks_old_m5 still exists after migration")
	}

	var tagCount int
	if err := store.DB().Raw("SELECT COUNT(*) FROM task_tags WHERE task_uuid = ? AND tag = ?", "task-1", "one").Scan(&tagCount).Error; err != nil {
		t.Fatal(err)
	}
	if tagCount != 1 {
		t.Fatalf("task_tags preserved count = %d, want 1", tagCount)
	}
	var note string
	if err := store.DB().Raw("SELECT description FROM task_annotations WHERE task_uuid = ?", "task-1").Scan(&note).Error; err != nil {
		t.Fatal(err)
	}
	if note != "note" {
		t.Fatalf("annotation = %q, want note", note)
	}
	var dep string
	if err := store.DB().Raw("SELECT depends_on FROM task_dependencies WHERE task_uuid = ?", "task-1").Scan(&dep).Error; err != nil {
		t.Fatal(err)
	}
	if dep != "dep" {
		t.Fatalf("depends_on = %q, want dep", dep)
	}
	var udaValue string
	if err := store.DB().Raw("SELECT value FROM task_uda_values WHERE task_uuid = ? AND name = ?", "task-1", "legacy").Scan(&udaValue).Error; err != nil {
		t.Fatal(err)
	}
	if udaValue != "v" {
		t.Fatalf("uda value = %q, want v", udaValue)
	}

	if err := store.DB().Exec(`INSERT INTO task_tags(task_uuid, tag) VALUES(?, ?)`, "missing-task", "bad").Error; err == nil {
		t.Fatal("insert into task_tags with missing task_uuid succeeded, want FK failure")
	}
	if violations := foreignKeyViolations(t, store); len(violations) != 0 {
		t.Fatalf("foreign_key_check violations = %#v", violations)
	}
}

type seedTask struct {
	WorkspaceSlug string
	UUID          string
	Project       *string
	Entry         int64
	Tags          []string
	Annotations   []seedAnnotation
	Depends       []string
	UDAs          []seedUDA
}

type seedAnnotation struct {
	Entry       int64
	Description string
}

type seedUDA struct {
	Name      string
	Value     string
	ValueType string
	Orphan    bool
}

type oldM5ProjectForTaskSlug struct {
	ID           string `gorm:"primaryKey;uniqueIndex:idx_projects_id_ws,priority:1"`
	WorkspaceID  string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:1;uniqueIndex:idx_projects_id_ws,priority:2;index:idx_projects_ws_status,priority:1"`
	Slug         string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:2"`
	Name         string `gorm:"not null"`
	Description  string `gorm:"not null;default:''"`
	Status       string `gorm:"not null;default:'active';index:idx_projects_ws_status,priority:2"`
	SettingsJSON string `gorm:"not null;default:'{}'"`
	CreatedAt    int64  `gorm:"not null"`
	ModifiedAt   int64  `gorm:"not null"`
	ArchivedAt   *int64
}

func (oldM5ProjectForTaskSlug) TableName() string { return "projects" }

func seedOldM5DatabaseForTaskSlug(t *testing.T, dbPath, projectSlug string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	execSQL(t, db, `CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	execSQL(t, db, `INSERT INTO meta(key, value) VALUES(?, ?)`, m5ProjectsAppliedMetaKey, "true")
	execSQL(t, db, `CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT, default_workspace_id TEXT, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL)`)
	execSQL(t, db, `CREATE TABLE workspaces (id TEXT PRIMARY KEY, slug TEXT NOT NULL, name TEXT NOT NULL DEFAULT 'Local', created_by_user_id TEXT, description TEXT, visibility TEXT NOT NULL DEFAULT 'private', settings_json TEXT NOT NULL DEFAULT '{}', archived_at INTEGER, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL DEFAULT 0)`)
	execSQL(t, db, `CREATE UNIQUE INDEX idx_workspaces_slug ON workspaces(slug)`)
	execSQL(t, db, `CREATE TABLE memberships (user_id TEXT NOT NULL, workspace_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (user_id, workspace_id))`)
	execSQL(t, db, `CREATE TABLE audit_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, actor_user_id TEXT, workspace_id TEXT, action TEXT NOT NULL, target_type TEXT, target_id TEXT, payload_json TEXT, created_at INTEGER NOT NULL)`)
	execSQL(t, db, `CREATE TABLE contexts (workspace_id TEXT NOT NULL, name TEXT NOT NULL, filter_source TEXT NOT NULL, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (workspace_id, name))`)
	execSQL(t, db, `CREATE TABLE uda_definitions (workspace_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL, label TEXT, values_json TEXT, default_value TEXT, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (workspace_id, name))`)
	if err := db.AutoMigrate(&oldM5ProjectForTaskSlug{}); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `CREATE TABLE tasks (
	uuid TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	title TEXT NOT NULL,
	description TEXT,
	status TEXT NOT NULL,
	entry INTEGER NOT NULL,
	modified INTEGER NOT NULL,
	end_ts INTEGER,
	due INTEGER,
	project TEXT,
	priority TEXT,
	start INTEGER,
	wait INTEGER,
	scheduled INTEGER,
	until INTEGER,
	recur TEXT,
	parent TEXT,
	mask TEXT,
	i_mask INTEGER,
	project_id TEXT,
	FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)
)`)
	execSQL(t, db, `CREATE INDEX idx_tasks_ws_project_id ON tasks(workspace_id, project_id)`)
	execSQL(t, db, `CREATE TABLE task_tags (task_uuid TEXT NOT NULL, tag TEXT NOT NULL, PRIMARY KEY (task_uuid, tag))`)
	execSQL(t, db, `CREATE TABLE task_annotations (task_uuid TEXT NOT NULL, entry INTEGER NOT NULL, description TEXT NOT NULL, PRIMARY KEY (task_uuid, entry, description))`)
	execSQL(t, db, `CREATE TABLE task_dependencies (task_uuid TEXT NOT NULL, depends_on TEXT NOT NULL, PRIMARY KEY (task_uuid, depends_on))`)
	execSQL(t, db, `CREATE TABLE task_uda_values (workspace_id TEXT NOT NULL, task_uuid TEXT NOT NULL, name TEXT NOT NULL, value TEXT NOT NULL, value_type TEXT, orphan NUMERIC NOT NULL DEFAULT false, PRIMARY KEY (task_uuid, name))`)
	execSQL(t, db, `CREATE TABLE task_assignees (task_uuid TEXT NOT NULL, user_id TEXT NOT NULL, PRIMARY KEY (task_uuid, user_id))`)
	execSQL(t, db, `CREATE TABLE task_links (id TEXT PRIMARY KEY, task_uuid TEXT NOT NULL, type TEXT NOT NULL, url TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, created_by TEXT NOT NULL)`)

	execSQL(t, db, `INSERT INTO workspaces(id, slug, name, visibility, settings_json, created_at, modified_at) VALUES('ws-local', 'local', 'Local', 'private', '{}', 1, 1)`)
	execSQL(t, db, `INSERT INTO projects(id, workspace_id, slug, name, description, status, settings_json, created_at, modified_at) VALUES('project-api', 'ws-local', ?, 'API', '', 'active', '{}', 1, 1)`, projectSlug)
	execSQL(t, db, `INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project, project_id) VALUES('task-b', 'ws-local', 'task b', 'pending', 20, 20, ?, 'project-api')`, projectSlug)
	execSQL(t, db, `INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project, project_id) VALUES('task-a', 'ws-local', 'task a', 'pending', 10, 10, ?, 'project-api')`, projectSlug)
}

func seedM4DatabaseWithTasks(t *testing.T, dbPath string, tasks []seedTask) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	execSQL(t, db, `CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	execSQL(t, db, `CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT, default_workspace_id TEXT, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL)`)
	execSQL(t, db, `CREATE UNIQUE INDEX idx_users_name ON users(name)`)
	execSQL(t, db, `CREATE UNIQUE INDEX idx_users_email ON users(email)`)
	execSQL(t, db, `CREATE TABLE workspaces (id TEXT PRIMARY KEY, slug TEXT NOT NULL, name TEXT NOT NULL DEFAULT 'Local', created_by_user_id TEXT, description TEXT, visibility TEXT NOT NULL DEFAULT 'private', settings_json TEXT NOT NULL DEFAULT '{}', archived_at INTEGER, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL DEFAULT 0)`)
	execSQL(t, db, `CREATE UNIQUE INDEX idx_workspaces_slug ON workspaces(slug)`)
	execSQL(t, db, `CREATE TABLE memberships (user_id TEXT NOT NULL, workspace_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (user_id, workspace_id))`)
	execSQL(t, db, `CREATE INDEX idx_memberships_workspace_id ON memberships(workspace_id)`)
	execSQL(t, db, `CREATE INDEX idx_memberships_role ON memberships(role)`)
	execSQL(t, db, `CREATE TABLE audit_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, actor_user_id TEXT, workspace_id TEXT, action TEXT NOT NULL, target_type TEXT, target_id TEXT, payload_json TEXT, created_at INTEGER NOT NULL)`)
	execSQL(t, db, `CREATE TABLE contexts (workspace_id TEXT NOT NULL, name TEXT NOT NULL, filter_source TEXT NOT NULL, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (workspace_id, name))`)
	execSQL(t, db, `CREATE TABLE uda_definitions (workspace_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL, label TEXT, values_json TEXT, default_value TEXT, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (workspace_id, name))`)
	execSQL(t, db, m4TasksDDL)
	for _, indexSQL := range m4TaskIndexes {
		execSQL(t, db, indexSQL)
	}
	execSQL(t, db, `CREATE TABLE task_tags (task_uuid TEXT NOT NULL, tag TEXT NOT NULL, PRIMARY KEY (task_uuid, tag))`)
	execSQL(t, db, `CREATE TABLE task_annotations (task_uuid TEXT NOT NULL, entry INTEGER NOT NULL, description TEXT NOT NULL, PRIMARY KEY (task_uuid, entry, description))`)
	execSQL(t, db, `CREATE TABLE task_dependencies (task_uuid TEXT NOT NULL, depends_on TEXT NOT NULL, PRIMARY KEY (task_uuid, depends_on))`)
	execSQL(t, db, `CREATE INDEX idx_task_dependencies_depends_on ON task_dependencies(depends_on)`)
	execSQL(t, db, `CREATE TABLE task_uda_values (workspace_id TEXT NOT NULL, task_uuid TEXT NOT NULL, name TEXT NOT NULL, value TEXT NOT NULL, value_type TEXT, orphan NUMERIC NOT NULL DEFAULT false, PRIMARY KEY (task_uuid, name))`)
	execSQL(t, db, `CREATE INDEX idx_task_uda_values_workspace_id ON task_uda_values(workspace_id)`)
	execSQL(t, db, `CREATE INDEX idx_task_uda_values_task_uuid ON task_uda_values(task_uuid)`)

	workspaces := map[string]string{}
	for _, task := range tasks {
		slug := task.WorkspaceSlug
		if slug == "" {
			slug = "local"
		}
		if _, ok := workspaces[slug]; !ok {
			workspaces[slug] = "ws-" + slug
			execSQL(t, db, `INSERT INTO workspaces(id, slug, name, visibility, settings_json, created_at, modified_at) VALUES(?, ?, ?, 'private', '{}', 1, 1)`, workspaces[slug], slug, "Workspace "+slug)
		}
	}
	for _, task := range tasks {
		slug := task.WorkspaceSlug
		if slug == "" {
			slug = "local"
		}
		execSQL(t, db, `INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project) VALUES(?, ?, ?, 'pending', ?, ?, ?)`,
			task.UUID, workspaces[slug], "task "+task.UUID, task.Entry, task.Entry, task.Project)
		for _, tag := range task.Tags {
			execSQL(t, db, `INSERT INTO task_tags(task_uuid, tag) VALUES(?, ?)`, task.UUID, tag)
		}
		for _, annotation := range task.Annotations {
			execSQL(t, db, `INSERT INTO task_annotations(task_uuid, entry, description) VALUES(?, ?, ?)`, task.UUID, annotation.Entry, annotation.Description)
		}
		for _, dependsOn := range task.Depends {
			execSQL(t, db, `INSERT INTO task_dependencies(task_uuid, depends_on) VALUES(?, ?)`, task.UUID, dependsOn)
		}
		for _, uda := range task.UDAs {
			execSQL(t, db, `INSERT INTO task_uda_values(workspace_id, task_uuid, name, value, value_type, orphan) VALUES(?, ?, ?, ?, ?, ?)`, workspaces[slug], task.UUID, uda.Name, uda.Value, uda.ValueType, uda.Orphan)
		}
	}
}

type m4GORMMeta struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"not null"`
}

func (m4GORMMeta) TableName() string { return "meta" }

type m4GORMWorkspace struct {
	ID              string `gorm:"primaryKey"`
	Slug            string `gorm:"not null;uniqueIndex"`
	Name            string `gorm:"not null"`
	CreatedByUserID *string
	Description     string
	Visibility      string `gorm:"not null;default:'private'"`
	SettingsJSON    string `gorm:"not null;default:'{}'"`
	ArchivedAt      *int64
	CreatedAt       int64 `gorm:"not null"`
	ModifiedAt      int64 `gorm:"not null"`
}

func (m4GORMWorkspace) TableName() string { return "workspaces" }

type m4GORMUser struct {
	ID                 string  `gorm:"primaryKey"`
	Name               string  `gorm:"not null;uniqueIndex"`
	Email              *string `gorm:"uniqueIndex"`
	DefaultWorkspaceID *string
	CreatedAt          int64 `gorm:"not null"`
	ModifiedAt         int64 `gorm:"not null"`
}

func (m4GORMUser) TableName() string { return "users" }

type m4GORMMembership struct {
	UserID      string `gorm:"primaryKey;not null"`
	WorkspaceID string `gorm:"primaryKey;not null;index"`
	Role        string `gorm:"not null;index"`
	JoinedAt    int64  `gorm:"not null"`
	ModifiedAt  int64  `gorm:"not null"`
}

func (m4GORMMembership) TableName() string { return "memberships" }

type m4GORMAuditLog struct {
	ID          int64   `gorm:"primaryKey;autoIncrement"`
	ActorUserID *string `gorm:"index"`
	WorkspaceID *string `gorm:"index;index:idx_audit_ws_time,priority:1"`
	Action      string  `gorm:"not null;index"`
	TargetType  string
	TargetID    string
	PayloadJSON string
	CreatedAt   int64 `gorm:"not null;index;index:idx_audit_ws_time,priority:2,sort:desc"`
}

func (m4GORMAuditLog) TableName() string { return "audit_logs" }

type m4GORMContext struct {
	WorkspaceID  string `gorm:"primaryKey;not null"`
	Name         string `gorm:"primaryKey;not null"`
	FilterSource string `gorm:"not null"`
	CreatedAt    int64  `gorm:"not null"`
	ModifiedAt   int64  `gorm:"not null"`
}

func (m4GORMContext) TableName() string { return "contexts" }

type m4GORMUDADefinition struct {
	WorkspaceID  string `gorm:"primaryKey;not null"`
	Name         string `gorm:"primaryKey;not null"`
	Type         string `gorm:"not null"`
	Label        string
	ValuesJSON   string
	DefaultValue string
	CreatedAt    int64 `gorm:"not null"`
	ModifiedAt   int64 `gorm:"not null"`
}

func (m4GORMUDADefinition) TableName() string { return "uda_definitions" }

type m4GORMTask struct {
	UUID        string `gorm:"primaryKey"`
	WorkspaceID string `gorm:"not null;index"`
	Title       string `gorm:"not null"`
	Description *string
	Status      string `gorm:"not null;index"`
	Entry       int64  `gorm:"not null"`
	Modified    int64  `gorm:"not null"`
	EndTS       *int64
	Due         *int64
	Project     *string `gorm:"index"`
	Priority    *string
	Tags        []m4GORMTaskTag `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Start       *int64
	Wait        *int64  `gorm:"index"`
	Scheduled   *int64  `gorm:"index"`
	Until       *int64  `gorm:"index"`
	Recur       *string `gorm:"index"`
	Parent      *string `gorm:"index"`
	Mask        *string
	IMask       *int
	Annotations []m4GORMTaskAnnotation `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Depends     []m4GORMTaskDependency `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	UDAs        []m4GORMTaskUDAValue   `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
}

func (m4GORMTask) TableName() string { return "tasks" }

type m4GORMTaskTag struct {
	TaskUUID string `gorm:"primaryKey;not null"`
	Tag      string `gorm:"primaryKey;not null"`
}

func (m4GORMTaskTag) TableName() string { return "task_tags" }

type m4GORMTaskAnnotation struct {
	TaskUUID    string `gorm:"primaryKey;not null"`
	Entry       int64  `gorm:"primaryKey;not null"`
	Description string `gorm:"primaryKey;not null"`
}

func (m4GORMTaskAnnotation) TableName() string { return "task_annotations" }

type m4GORMTaskDependency struct {
	TaskUUID  string `gorm:"primaryKey;not null"`
	DependsOn string `gorm:"primaryKey;not null;index"`
}

func (m4GORMTaskDependency) TableName() string { return "task_dependencies" }

type m4GORMTaskUDAValue struct {
	WorkspaceID string `gorm:"not null;index"`
	TaskUUID    string `gorm:"primaryKey;not null;index"`
	Name        string `gorm:"primaryKey;not null"`
	Value       string `gorm:"not null"`
	ValueType   string
	Orphan      bool `gorm:"not null;default:false"`
}

func (m4GORMTaskUDAValue) TableName() string { return "task_uda_values" }

func seedM4GORMDatabaseWithTaskRelations(t *testing.T, dbPath string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if err := db.AutoMigrate(
		&m4GORMMeta{},
		&m4GORMUser{},
		&m4GORMWorkspace{},
		&m4GORMMembership{},
		&m4GORMAuditLog{},
		&m4GORMContext{},
		&m4GORMUDADefinition{},
		&m4GORMTask{},
		&m4GORMTaskTag{},
		&m4GORMTaskAnnotation{},
		&m4GORMTaskDependency{},
		&m4GORMTaskUDAValue{},
	); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO workspaces(id, slug, name, visibility, settings_json, created_at, modified_at) VALUES('ws-local', 'local', 'Local', 'private', '{}', 1, 1)`)
	execSQL(t, db, `INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project) VALUES('dep', 'ws-local', 'dep', 'pending', 1, 1, 'api')`)
	execSQL(t, db, `INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project) VALUES('task-1', 'ws-local', 'task 1', 'pending', 2, 2, 'api')`)
	execSQL(t, db, `INSERT INTO task_tags(task_uuid, tag) VALUES('task-1', 'one')`)
	execSQL(t, db, `INSERT INTO task_annotations(task_uuid, entry, description) VALUES('task-1', 3, 'note')`)
	execSQL(t, db, `INSERT INTO task_dependencies(task_uuid, depends_on) VALUES('task-1', 'dep')`)
	execSQL(t, db, `INSERT INTO task_uda_values(workspace_id, task_uuid, name, value, value_type, orphan) VALUES('ws-local', 'task-1', 'legacy', 'v', 'string', true)`)
}

func execSQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec %q error = %v", sql, err)
	}
}

func mutateM4Database(t *testing.T, dbPath, sql string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	execSQL(t, db, sql)
}

func rawTableExists(t *testing.T, db *gorm.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count > 0
}

func assertRawDDLContains(t *testing.T, store *Store, table, want string) {
	t.Helper()
	var ddl string
	if err := store.DB().Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&ddl).Error; err != nil {
		t.Fatal(err)
	}
	normalizedDDL := strings.ReplaceAll(ddl, " ", "")
	normalizedWant := strings.ReplaceAll(want, " ", "")
	if !strings.Contains(normalizedDDL, normalizedWant) {
		t.Fatalf("%s DDL = %s, want containing %s", table, ddl, want)
	}
}

func assertRawInsertNullRejected(t *testing.T, store *Store, sql string) {
	t.Helper()
	if err := store.DB().Exec(sql).Error; err == nil {
		t.Fatalf("raw insert succeeded, want NULL rejection: %s", sql)
	}
}

func assertIndexColumns(t *testing.T, store *Store, indexName string, want []string) {
	t.Helper()
	rows, err := store.DB().Raw("PRAGMA index_info(" + indexName + ")").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s columns = %#v, want %#v", indexName, got, want)
	}
}

func assertColumnNullable(t *testing.T, store *Store, table, column string, want bool) {
	t.Helper()
	rows, err := store.DB().Raw("PRAGMA table_info(" + table + ")").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			got := notNull == 0
			if got != want {
				t.Fatalf("%s.%s nullable = %v, want %v", table, column, got, want)
			}
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("%s.%s column not found", table, column)
}

func assertRelationForeignKeysPointToTasks(t *testing.T, store *Store, table string) {
	t.Helper()
	rows, err := store.DB().Raw("PRAGMA foreign_key_list(" + table + ")").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	foundTasks := false
	for rows.Next() {
		var id, seq int
		var targetTable, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &targetTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatal(err)
		}
		if targetTable == "tasks_old_m5" {
			t.Fatalf("%s foreign key points to tasks_old_m5", table)
		}
		if targetTable == "tasks" && from == "task_uuid" && to == "uuid" {
			foundTasks = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !foundTasks {
		t.Fatalf("%s has no task_uuid -> tasks.uuid foreign key", table)
	}
}

func foreignKeyViolations(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.DB().Raw("PRAGMA foreign_key_check").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var violations []string
	for rows.Next() {
		var table string
		var rowID int64
		var parent string
		var fkID int
		if err := rows.Scan(&table, &rowID, &parent, &fkID); err != nil {
			t.Fatal(err)
		}
		violations = append(violations, table+"->"+parent)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return violations
}

func assertTaskProject(t *testing.T, store *Store, taskUUID, wantSlug, wantProjectID string) {
	t.Helper()
	var row struct {
		Project   sql.NullString
		ProjectID sql.NullString
	}
	if err := store.DB().Raw("SELECT project, project_id FROM tasks WHERE uuid = ?", taskUUID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !row.Project.Valid || row.Project.String != wantSlug || !row.ProjectID.Valid || row.ProjectID.String != wantProjectID {
		t.Fatalf("task %s project = (%#v, %#v), want (%q, %q)", taskUUID, row.Project, row.ProjectID, wantSlug, wantProjectID)
	}
}

func assertTaskHasNoProject(t *testing.T, store *Store, taskUUID string) {
	t.Helper()
	var row struct {
		Project   sql.NullString
		ProjectID sql.NullString
	}
	if err := store.DB().Raw("SELECT project, project_id FROM tasks WHERE uuid = ?", taskUUID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Project.Valid || row.ProjectID.Valid {
		t.Fatalf("task %s project = (%#v, %#v), want NULLs", taskUUID, row.Project, row.ProjectID)
	}
}

func assertMigrationSkippedReport(t *testing.T, store *Store, want []string) {
	t.Helper()
	raw, ok, err := store.GetMeta("migration.m5.projects.skipped")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("missing migration.m5.projects.skipped")
	}
	var rows []m5SkippedProject
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		t.Fatalf("invalid skipped report %q: %v", raw, err)
	}
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.TaskUUID+":"+row.Reason)
	}
	if want == nil {
		want = []string{}
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("skipped report = %#v, want %#v (raw %s)", got, want, raw)
	}
}

func projectSlugs(projects []Project) []string {
	slugs := make([]string, 0, len(projects))
	for _, project := range projects {
		slugs = append(slugs, project.Slug)
	}
	return slugs
}

func ptrString(value string) *string {
	return &value
}

func taskColumnNames(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.DB().Raw("PRAGMA table_info(tasks)").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

func taskIndexNames(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.DB().Raw("PRAGMA index_list(tasks)").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}
