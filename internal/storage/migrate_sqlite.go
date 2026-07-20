package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const m5ProjectsAppliedMetaKey = "migration.m5.projects.applied"
const taskSlugMigrationMetaKey = "migration.v0.1.1.task_slug.applied"

var m5ProjectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (s *Store) migrateSQLite() error {
	if err := s.prepareWorkspaceSchemaForM4(); err != nil {
		return err
	}
	if err := s.prepareReminderRuleScheduleColumns(); err != nil {
		return err
	}
	if err := s.prepareProjectNextTaskSeqColumnForV011(); err != nil {
		return err
	}
	if err := s.prepareAPITokenUserIDNullable(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&Meta{}, &User{}, &Workspace{}, &Membership{}, &AuditLog{}, &Project{}, &ProjectAnnotation{}, &Config{}, &ConfigDefinition{}, &ApiToken{}, &ServerAdminToken{}, &AdminActingSession{}, &Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{}, &NotificationSink{}, &ReminderRule{}, &EventNotificationRule{}, &NotificationDelivery{}, &ProjectAutomationRule{}, &ProjectAutomationDelivery{}, &UserExternalID{}, &BrowserSession{}, &BrowserAuthFlow{}, &DirectorySyncJob{}); err != nil {
		return err
	}
	if err := s.prepareProjectTemplateSchemaSQLite(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&TaskTag{}, &TaskDependency{}, &TaskAssignee{}, &TaskUDAValue{}, &TaskLink{}, &Attachment{}); err != nil {
		return err
	}
	if err := s.prepareActorColumnsForP2(); err != nil {
		return err
	}
	if err := s.prepareProjectSchemaForM5(); err != nil {
		return err
	}
	if err := s.prepareTaskSlugSchemaForV011(); err != nil {
		return err
	}
	if err := s.prepareTaskAnnotationIDsForV020(); err != nil {
		return err
	}
	if err := s.prepareActorColumnsForP2(); err != nil {
		return err
	}
	if err := s.prepareTaskSeriesSchema(); err != nil {
		return err
	}
	return nil
}

// prepareProjectTemplateSchemaSQLite 显式建表以保留 Template 与 Snapshot 的循环
// ON DELETE RESTRICT 外键。GORM 在 SQLite 上处理这种循环时可能通过 rebuild 表来
// 迁移，因此这里不使用 AutoMigrate。
func (s *Store) prepareProjectTemplateSchemaSQLite() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS project_templates (
id TEXT PRIMARY KEY,
workspace_id TEXT NOT NULL,
key TEXT NOT NULL,
name TEXT NOT NULL,
description TEXT NOT NULL DEFAULT '',
status TEXT NOT NULL,
current_snapshot_id TEXT,
created_by_actor_type TEXT NOT NULL DEFAULT 'user',
created_by_user_id TEXT,
created_by_token_id TEXT,
created_by_token_name TEXT,
created_by_token_prefix TEXT,
created_at INTEGER NOT NULL,
modified_at INTEGER NOT NULL,
archived_at INTEGER,
FOREIGN KEY (current_snapshot_id) REFERENCES project_template_snapshots(id) ON DELETE RESTRICT
)`,
		`CREATE TABLE IF NOT EXISTS project_template_snapshots (
id TEXT PRIMARY KEY,
workspace_id TEXT NOT NULL,
template_id TEXT NOT NULL,
version INTEGER NOT NULL,
source_project_id TEXT NOT NULL,
snapshot_json TEXT NOT NULL,
snapshot_hash TEXT NOT NULL,
created_by_actor_type TEXT NOT NULL DEFAULT 'user',
created_by_user_id TEXT,
created_by_token_id TEXT,
created_by_token_name TEXT,
created_by_token_prefix TEXT,
created_at INTEGER NOT NULL,
FOREIGN KEY (template_id) REFERENCES project_templates(id) ON DELETE RESTRICT
)`,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_templates_ws_key ON project_templates(workspace_id, key)",
		"CREATE INDEX IF NOT EXISTS idx_project_templates_ws_status ON project_templates(workspace_id, status)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_version ON project_template_snapshots(template_id, version)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_snapshot_hash ON project_template_snapshots(template_id, snapshot_hash)",
		"CREATE INDEX IF NOT EXISTS idx_project_template_snapshots_ws_template ON project_template_snapshots(workspace_id, template_id)",
	}
	for _, statement := range statements {
		if err := s.db.Exec(statement).Error; err != nil {
			return fmt.Errorf("project template migration: %w", err)
		}
	}
	return nil
}

func (s *Store) prepareActorColumnsForP2() error {
	columns := map[string][]struct {
		name string
		sql  string
	}{
		"project_annotations": {
			{"created_by_actor_type", "ALTER TABLE project_annotations ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"created_by_user_id", "ALTER TABLE project_annotations ADD COLUMN created_by_user_id TEXT"},
			{"created_by_token_id", "ALTER TABLE project_annotations ADD COLUMN created_by_token_id TEXT"},
			{"created_by_token_name", "ALTER TABLE project_annotations ADD COLUMN created_by_token_name TEXT"},
			{"created_by_token_prefix", "ALTER TABLE project_annotations ADD COLUMN created_by_token_prefix TEXT"},
		},
		"hook_definitions": {
			{"actor_type", "ALTER TABLE hook_definitions ADD COLUMN actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"actor_token_id", "ALTER TABLE hook_definitions ADD COLUMN actor_token_id TEXT"},
			{"actor_token_name", "ALTER TABLE hook_definitions ADD COLUMN actor_token_name TEXT"},
			{"actor_token_prefix", "ALTER TABLE hook_definitions ADD COLUMN actor_token_prefix TEXT"},
		},
		"hook_deliveries": {
			{"actor_type", "ALTER TABLE hook_deliveries ADD COLUMN actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"actor_token_id", "ALTER TABLE hook_deliveries ADD COLUMN actor_token_id TEXT"},
			{"actor_token_name", "ALTER TABLE hook_deliveries ADD COLUMN actor_token_name TEXT"},
			{"actor_token_prefix", "ALTER TABLE hook_deliveries ADD COLUMN actor_token_prefix TEXT"},
		},
		"notification_sinks": {
			{"created_by_actor_type", "ALTER TABLE notification_sinks ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"created_by_user_id", "ALTER TABLE notification_sinks ADD COLUMN created_by_user_id TEXT"},
			{"created_by_token_id", "ALTER TABLE notification_sinks ADD COLUMN created_by_token_id TEXT"},
			{"created_by_token_name", "ALTER TABLE notification_sinks ADD COLUMN created_by_token_name TEXT"},
			{"created_by_token_prefix", "ALTER TABLE notification_sinks ADD COLUMN created_by_token_prefix TEXT"},
		},
		"reminder_rules": {
			{"created_by_actor_type", "ALTER TABLE reminder_rules ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"created_by_user_id", "ALTER TABLE reminder_rules ADD COLUMN created_by_user_id TEXT"},
			{"created_by_token_id", "ALTER TABLE reminder_rules ADD COLUMN created_by_token_id TEXT"},
			{"created_by_token_name", "ALTER TABLE reminder_rules ADD COLUMN created_by_token_name TEXT"},
			{"created_by_token_prefix", "ALTER TABLE reminder_rules ADD COLUMN created_by_token_prefix TEXT"},
		},
		"event_notification_rules": {
			{"created_by_actor_type", "ALTER TABLE event_notification_rules ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"created_by_user_id", "ALTER TABLE event_notification_rules ADD COLUMN created_by_user_id TEXT"},
			{"created_by_token_id", "ALTER TABLE event_notification_rules ADD COLUMN created_by_token_id TEXT"},
			{"created_by_token_name", "ALTER TABLE event_notification_rules ADD COLUMN created_by_token_name TEXT"},
			{"created_by_token_prefix", "ALTER TABLE event_notification_rules ADD COLUMN created_by_token_prefix TEXT"},
		},
		"notification_deliveries": {
			{"actor_type", "ALTER TABLE notification_deliveries ADD COLUMN actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"actor_user_id", "ALTER TABLE notification_deliveries ADD COLUMN actor_user_id TEXT"},
			{"actor_token_id", "ALTER TABLE notification_deliveries ADD COLUMN actor_token_id TEXT"},
			{"actor_token_name", "ALTER TABLE notification_deliveries ADD COLUMN actor_token_name TEXT"},
			{"actor_token_prefix", "ALTER TABLE notification_deliveries ADD COLUMN actor_token_prefix TEXT"},
		},
		"task_links": {
			{"created_by_actor_type", "ALTER TABLE task_links ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"created_by_user_id", "ALTER TABLE task_links ADD COLUMN created_by_user_id TEXT"},
			{"created_by_token_id", "ALTER TABLE task_links ADD COLUMN created_by_token_id TEXT"},
			{"created_by_token_name", "ALTER TABLE task_links ADD COLUMN created_by_token_name TEXT"},
			{"created_by_token_prefix", "ALTER TABLE task_links ADD COLUMN created_by_token_prefix TEXT"},
		},
		"project_automation_rules": {
			{"created_by_actor_type", "ALTER TABLE project_automation_rules ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"created_by_user_id", "ALTER TABLE project_automation_rules ADD COLUMN created_by_user_id TEXT"},
			{"created_by_token_id", "ALTER TABLE project_automation_rules ADD COLUMN created_by_token_id TEXT"},
			{"created_by_token_name", "ALTER TABLE project_automation_rules ADD COLUMN created_by_token_name TEXT"},
			{"created_by_token_prefix", "ALTER TABLE project_automation_rules ADD COLUMN created_by_token_prefix TEXT"},
		},
	}
	for table, tableColumns := range columns {
		if !s.db.Migrator().HasTable(table) {
			continue
		}
		for _, column := range tableColumns {
			if s.db.Migrator().HasColumn(table, column.name) {
				continue
			}
			if err := s.db.Exec(column.sql).Error; err != nil {
				return err
			}
		}
	}
	if s.db.Migrator().HasTable("notification_deliveries") &&
		s.db.Migrator().HasColumn("notification_deliveries", "actor_user_id") &&
		s.db.Migrator().HasColumn("notification_deliveries", "recipient_user_id") {
		if err := s.db.Exec(`UPDATE notification_deliveries SET actor_user_id = recipient_user_id WHERE (actor_type = 'user' OR actor_type = '') AND (actor_user_id IS NULL OR actor_user_id = '') AND recipient_user_id <> ''`).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) prepareAPITokenUserIDNullable() error {
	sqlDB, err := s.sqlDB()
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	began := false
	committed := false
	defer func() {
		if began && !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
		_, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	}()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	began = true

	tx := m5MigrationTx{ctx: ctx, conn: conn}
	hasTable, err := tableExists(tx, "api_tokens")
	if err != nil {
		return err
	}
	if hasTable {
		userIDNotNull, err := columnNotNull(tx, "api_tokens", "user_id")
		if err != nil {
			return err
		}
		if userIDNotNull {
			if err := rebuildAPITokensWithNullableUserID(tx); err != nil {
				return err
			}
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func rebuildAPITokensWithNullableUserID(tx m5MigrationTx) error {
	if err := tx.exec("ALTER TABLE api_tokens RENAME TO api_tokens_old_nullable_user_id"); err != nil {
		return err
	}
	if err := createAPITokensWithNullableUserID(tx); err != nil {
		return err
	}
	if err := tx.exec(`INSERT INTO api_tokens (
id, user_id, name, type, token_prefix, token_hash, scopes_json, workspace_ids_json,
project_ids_json, issued_via, issued_by_admin_token_id, issued_by_admin_token_name, purpose,
created_at, expires_at, revoked_at, last_used_at
)
SELECT
id, user_id, name, type, token_prefix, token_hash, scopes_json, workspace_ids_json,
project_ids_json, 'user', NULL, NULL, 'api', created_at, expires_at, revoked_at, last_used_at
FROM api_tokens_old_nullable_user_id`); err != nil {
		return err
	}
	return tx.exec("DROP TABLE api_tokens_old_nullable_user_id")
}

func createAPITokensWithNullableUserID(tx m5MigrationTx) error {
	if err := tx.exec(`CREATE TABLE api_tokens (
id TEXT PRIMARY KEY,
user_id TEXT,
name TEXT NOT NULL,
type TEXT NOT NULL,
token_prefix TEXT NOT NULL,
token_hash TEXT NOT NULL,
scopes_json TEXT NOT NULL DEFAULT '[]',
workspace_ids_json TEXT NOT NULL DEFAULT '[]',
project_ids_json TEXT NOT NULL DEFAULT '[]',
issued_via TEXT NOT NULL DEFAULT 'user',
issued_by_admin_token_id TEXT,
issued_by_admin_token_name TEXT,
purpose TEXT NOT NULL DEFAULT 'api',
created_at INTEGER NOT NULL,
expires_at INTEGER,
revoked_at INTEGER,
last_used_at INTEGER
)`); err != nil {
		return err
	}
	if err := tx.exec("CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id)"); err != nil {
		return err
	}
	return tx.exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_api_tokens_prefix ON api_tokens(token_prefix)")
}

func (s *Store) prepareTaskAnnotationIDsForV020() error {
	sqlDB, err := s.sqlDB()
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	began := false
	committed := false
	defer func() {
		if began && !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
		_, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	}()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	began = true

	tx := m5MigrationTx{ctx: ctx, conn: conn}
	hasTable, err := tableExists(tx, "task_annotations")
	if err != nil {
		return err
	}
	if !hasTable {
		if err := createTaskAnnotationsWithIDs(tx); err != nil {
			return err
		}
	} else {
		hasID, err := columnExists(tx, "task_annotations", "id")
		if err != nil {
			return err
		}
		if !hasID {
			if err := rebuildTaskAnnotationsWithIDs(tx); err != nil {
				return err
			}
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func createTaskAnnotationsWithIDs(tx m5MigrationTx) error {
	if err := tx.exec(`CREATE TABLE task_annotations (
id TEXT PRIMARY KEY,
task_uuid TEXT NOT NULL,
entry INTEGER NOT NULL,
description TEXT NOT NULL,
CONSTRAINT fk_tasks_annotations FOREIGN KEY(task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`); err != nil {
		return err
	}
	return tx.exec("CREATE INDEX IF NOT EXISTS idx_task_annotations_task ON task_annotations(task_uuid)")
}

func rebuildTaskAnnotationsWithIDs(tx m5MigrationTx) error {
	if err := tx.exec("ALTER TABLE task_annotations RENAME TO task_annotations_old_v020"); err != nil {
		return err
	}
	if err := createTaskAnnotationsWithIDs(tx); err != nil {
		return err
	}
	rows, err := tx.query("SELECT task_uuid, entry, description FROM task_annotations_old_v020 ORDER BY task_uuid, entry, description")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var taskUUID, description string
		var entry int64
		if err := rows.Scan(&taskUUID, &entry, &description); err != nil {
			return err
		}
		if err := tx.exec("INSERT INTO task_annotations(id, task_uuid, entry, description) VALUES(?, ?, ?, ?)", uuid.NewString(), taskUUID, entry, description); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := tx.exec("DROP TABLE task_annotations_old_v020"); err != nil {
		return err
	}
	return tx.exec("CREATE INDEX IF NOT EXISTS idx_task_annotations_task ON task_annotations(task_uuid)")
}

func (s *Store) prepareReminderRuleScheduleColumns() error {
	if !s.db.Migrator().HasTable(&ReminderRule{}) {
		return nil
	}
	columns := []struct {
		name string
		sql  string
	}{
		{name: "schedule_type", sql: "ALTER TABLE reminder_rules ADD COLUMN schedule_type TEXT NOT NULL DEFAULT ''"},
		{name: "schedule_value", sql: "ALTER TABLE reminder_rules ADD COLUMN schedule_value TEXT NOT NULL DEFAULT ''"},
		{name: "filter_source", sql: "ALTER TABLE reminder_rules ADD COLUMN filter_source TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if s.db.Migrator().HasColumn(&ReminderRule{}, column.name) {
			continue
		}
		if err := s.db.Exec(column.sql).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) prepareProjectNextTaskSeqColumnForV011() error {
	if !s.db.Migrator().HasTable(&Project{}) {
		return nil
	}
	if s.db.Migrator().HasColumn(&Project{}, "next_task_seq") {
		return nil
	}
	return s.db.Exec("ALTER TABLE projects ADD COLUMN next_task_seq INTEGER NOT NULL DEFAULT 1").Error
}

func (s *Store) prepareTaskSlugSchemaForV011() error {
	sqlDB, err := s.sqlDB()
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	began := true
	committed := false
	defer func() {
		if began && !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()

	tx := m5MigrationTx{ctx: ctx, conn: conn}
	applied, err := metaKeyApplied(tx, taskSlugMigrationMetaKey)
	if err != nil {
		return err
	}
	if applied {
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return err
		}
		committed = true
		return nil
	}

	if exists, err := columnExists(tx, "projects", "next_task_seq"); err != nil {
		return err
	} else if !exists {
		if err := tx.exec("ALTER TABLE projects ADD COLUMN next_task_seq INTEGER NOT NULL DEFAULT 1"); err != nil {
			return err
		}
	}
	if exists, err := columnExists(tx, "tasks", "project_seq"); err != nil {
		return err
	} else if !exists {
		if err := tx.exec("ALTER TABLE tasks ADD COLUMN project_seq INTEGER"); err != nil {
			return err
		}
	}
	if err := validateV011ProjectSlugs(tx); err != nil {
		return err
	}
	if err := backfillProjectSeqs(tx); err != nil {
		return err
	}
	for _, indexSQL := range m5TaskIndexes {
		if err := tx.exec(indexSQL); err != nil {
			return err
		}
	}
	if err := setMetaInTx(tx, taskSlugMigrationMetaKey, "true"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) prepareWorkspaceSchemaForM4() error {
	if !s.db.Migrator().HasTable(&Workspace{}) {
		return nil
	}
	columns := []struct {
		name string
		sql  string
	}{
		{name: "name", sql: "ALTER TABLE workspaces ADD COLUMN name TEXT NOT NULL DEFAULT 'Local'"},
		{name: "created_by_user_id", sql: "ALTER TABLE workspaces ADD COLUMN created_by_user_id TEXT"},
		{name: "description", sql: "ALTER TABLE workspaces ADD COLUMN description TEXT"},
		{name: "visibility", sql: "ALTER TABLE workspaces ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private'"},
		{name: "settings_json", sql: "ALTER TABLE workspaces ADD COLUMN settings_json TEXT NOT NULL DEFAULT '{}'"},
		{name: "archived_at", sql: "ALTER TABLE workspaces ADD COLUMN archived_at INTEGER"},
		{name: "modified_at", sql: "ALTER TABLE workspaces ADD COLUMN modified_at INTEGER NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		if s.db.Migrator().HasColumn(&Workspace{}, column.name) {
			continue
		}
		if err := s.db.Exec(column.sql).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) prepareProjectSchemaForM5() error {
	sqlDB, err := s.sqlDB()
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	began := false
	committed := false
	defer func() {
		if began && !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
		_, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	}()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	began = true

	tx := m5MigrationTx{ctx: ctx, conn: conn}
	hasTasks, err := tableExists(tx, "tasks")
	if err != nil {
		return err
	}
	if !hasTasks {
		if err := createM5TasksSchema(tx); err != nil {
			return err
		}
		if err := rebuildM5TaskRelationForeignKeys(tx); err != nil {
			return err
		}
		if err := assertNoM5ForeignKeyViolations(tx); err != nil {
			return err
		}
		if err := setMetaInTx(tx, m5ProjectsSkippedMetaKey, "[]"); err != nil {
			return err
		}
		if err := setMetaInTx(tx, m5ProjectsAppliedMetaKey, "true"); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return err
		}
		committed = true
		return nil
	}

	applied, err := m5ProjectsMarkerApplied(tx)
	if err != nil {
		return err
	}
	if applied {
		if err := dropLegacyTaskProjectIndexes(tx); err != nil {
			return err
		}
		if err := rebuildM5TaskRelationForeignKeys(tx); err != nil {
			return err
		}
		if err := assertNoM5ForeignKeyViolations(tx); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return err
		}
		committed = true
		return nil
	}

	if err := migrateM4TaskProjectsToM5(tx); err != nil {
		return err
	}
	if err := assertNoM5ForeignKeyViolations(tx); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

type m5MigrationTx struct {
	ctx  context.Context
	conn *sql.Conn
}

func (tx m5MigrationTx) exec(query string, args ...any) error {
	_, err := tx.conn.ExecContext(tx.ctx, query, args...)
	return err
}

func (tx m5MigrationTx) query(query string, args ...any) (*sql.Rows, error) {
	return tx.conn.QueryContext(tx.ctx, query, args...)
}

func (tx m5MigrationTx) queryRow(query string, args ...any) *sql.Row {
	return tx.conn.QueryRowContext(tx.ctx, query, args...)
}

func migrateM4TaskProjectsToM5(tx m5MigrationTx) error {
	plans, skipped := collectM5ProjectPlans(tx)
	if err := skipped.err; err != nil {
		return err
	}
	for _, project := range plans {
		if err := tx.exec(`INSERT INTO projects(id, workspace_id, slug, name, description, status, settings_json, created_at, modified_at)
VALUES(?, ?, ?, ?, '', 'active', '{}', ?, ?)
ON CONFLICT(workspace_id, slug) DO NOTHING`, project.ID, project.WorkspaceID, project.Slug, project.Name, project.CreatedAt, project.ModifiedAt); err != nil {
			return err
		}
	}
	if err := tx.exec("ALTER TABLE tasks RENAME TO tasks_old_m5"); err != nil {
		return err
	}
	if err := dropM4TaskIndexes(tx); err != nil {
		return err
	}
	if err := createM5TasksSchema(tx); err != nil {
		return err
	}
	if err := copyM4TasksToM5(tx); err != nil {
		return err
	}
	if err := tx.exec("DROP TABLE tasks_old_m5"); err != nil {
		return err
	}
	if err := rebuildM5TaskRelationForeignKeys(tx); err != nil {
		return err
	}
	if err := writeM5SkippedReport(tx, skipped.rows); err != nil {
		return err
	}
	if err := setMetaInTx(tx, m5ProjectsAppliedMetaKey, "true"); err != nil {
		return err
	}
	return nil
}

type m5ProjectPlan struct {
	ID          string
	WorkspaceID string
	Slug        string
	Name        string
	CreatedAt   int64
	ModifiedAt  int64
}

type m5SkippedResult struct {
	rows []m5SkippedProject
	err  error
}

type m5ProjectCandidate struct {
	WorkspaceID string
	TaskUUID    string
	RawProject  string
	Trimmed     string
	Slug        string
	Entry       int64
}

func collectM5ProjectPlans(tx m5MigrationTx) ([]m5ProjectPlan, m5SkippedResult) {
	rows, err := tx.query(`SELECT workspace_id, uuid, project, entry FROM tasks WHERE project IS NOT NULL AND TRIM(project) != '' ORDER BY workspace_id, LOWER(TRIM(project)), entry, uuid`)
	if err != nil {
		return nil, m5SkippedResult{err: err}
	}
	defer rows.Close()

	candidates := map[string][]m5ProjectCandidate{}
	for rows.Next() {
		var workspaceID, taskUUID, rawProject string
		var entry int64
		if err := rows.Scan(&workspaceID, &taskUUID, &rawProject, &entry); err != nil {
			return nil, m5SkippedResult{err: err}
		}
		trimmed := strings.TrimSpace(rawProject)
		slug := strings.ToLower(trimmed)
		key := workspaceID + "\x00" + slug
		candidates[key] = append(candidates[key], m5ProjectCandidate{
			WorkspaceID: workspaceID,
			TaskUUID:    taskUUID,
			RawProject:  rawProject,
			Trimmed:     trimmed,
			Slug:        slug,
			Entry:       entry,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, m5SkippedResult{err: err}
	}

	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	plans := make([]m5ProjectPlan, 0, len(keys))
	skipped := make([]m5SkippedProject, 0)
	for _, key := range keys {
		group := candidates[key]
		if len(group) == 0 {
			continue
		}
		if !isValidM5ProjectSlug(group[0].Slug) {
			for _, candidate := range group {
				skipped = append(skipped, skippedProject(candidate, "invalid_slug"))
			}
			continue
		}
		rawNames := map[string]bool{}
		for _, candidate := range group {
			rawNames[candidate.RawProject] = true
		}
		if len(rawNames) > 1 {
			for _, candidate := range group {
				skipped = append(skipped, skippedProject(candidate, "conflict"))
			}
			continue
		}
		first := group[0]
		plans = append(plans, m5ProjectPlan{
			ID:          uuid.NewString(),
			WorkspaceID: first.WorkspaceID,
			Slug:        first.Slug,
			Name:        first.Trimmed,
			CreatedAt:   first.Entry,
			ModifiedAt:  first.Entry,
		})
	}
	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].WorkspaceID != skipped[j].WorkspaceID {
			return skipped[i].WorkspaceID < skipped[j].WorkspaceID
		}
		return skipped[i].TaskUUID < skipped[j].TaskUUID
	})
	return plans, m5SkippedResult{rows: skipped}
}

func skippedProject(candidate m5ProjectCandidate, reason string) m5SkippedProject {
	return m5SkippedProject{
		WorkspaceID: candidate.WorkspaceID,
		TaskUUID:    candidate.TaskUUID,
		RawProject:  candidate.RawProject,
		Reason:      reason,
	}
}

func isValidM5ProjectSlug(slug string) bool {
	if len(slug) < 1 || len(slug) > 64 {
		return false
	}
	switch slug {
	case "all", "none", "current", "default", "archived":
		return false
	}
	return m5ProjectSlugPattern.MatchString(slug)
}

func copyM4TasksToM5(tx m5MigrationTx) error {
	return tx.exec(`INSERT INTO tasks (
  uuid, workspace_id, title, description, status, entry, modified,
  end_ts, due, project, priority, start, wait, scheduled, until,
  recur, parent, mask, i_mask, project_id
)
SELECT
  t.uuid, t.workspace_id, t.title, t.description, t.status, t.entry, t.modified,
  t.end_ts, t.due,
  CASE
    WHEN p.id IS NOT NULL THEN p.slug
    ELSE NULL
  END AS project,
  t.priority, t.start, t.wait, t.scheduled, t.until,
  t.recur, t.parent, t.mask, t.i_mask,
  p.id AS project_id
FROM tasks_old_m5 t
LEFT JOIN projects p
  ON p.workspace_id = t.workspace_id
 AND p.slug = LOWER(TRIM(t.project))`)
}

func createM5TasksSchema(tx m5MigrationTx) error {
	if err := tx.exec(m5TasksDDL); err != nil {
		return err
	}
	for _, indexSQL := range m5TaskIndexes {
		if err := tx.exec(indexSQL); err != nil {
			return err
		}
	}
	return dropLegacyTaskProjectIndexes(tx)
}

func dropM4TaskIndexes(tx m5MigrationTx) error {
	for _, name := range m4TaskIndexNames {
		if err := tx.exec("DROP INDEX IF EXISTS " + name); err != nil {
			return err
		}
	}
	return nil
}

func dropLegacyTaskProjectIndexes(tx m5MigrationTx) error {
	if err := tx.exec("DROP INDEX IF EXISTS idx_tasks_workspace_id"); err != nil {
		return err
	}
	return tx.exec("DROP INDEX IF EXISTS idx_tasks_project")
}

type m5TaskRelationSchema struct {
	table   string
	ddl     string
	columns []string
	indexes []string
}

var m5TaskRelationSchemas = []m5TaskRelationSchema{
	{
		table:   "task_tags",
		columns: []string{"task_uuid", "tag"},
		ddl: `CREATE TABLE task_tags (
	task_uuid TEXT NOT NULL,
	tag TEXT NOT NULL,
	PRIMARY KEY (task_uuid, tag),
	CONSTRAINT fk_tasks_tags FOREIGN KEY (task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`,
	},
	{
		table:   "task_annotations",
		columns: []string{"id", "task_uuid", "entry", "description"},
		ddl: `CREATE TABLE task_annotations (
	id TEXT PRIMARY KEY,
	task_uuid TEXT NOT NULL,
	entry INTEGER NOT NULL,
	description TEXT NOT NULL,
	CONSTRAINT fk_tasks_annotations FOREIGN KEY (task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`,
		indexes: []string{"CREATE INDEX IF NOT EXISTS idx_task_annotations_task ON task_annotations(task_uuid)"},
	},
	{
		table:   "task_dependencies",
		columns: []string{"task_uuid", "depends_on"},
		ddl: `CREATE TABLE task_dependencies (
	task_uuid TEXT NOT NULL,
	depends_on TEXT NOT NULL,
	PRIMARY KEY (task_uuid, depends_on),
	CONSTRAINT fk_tasks_depends FOREIGN KEY (task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`,
		indexes: []string{"CREATE INDEX IF NOT EXISTS idx_task_dependencies_depends_on ON task_dependencies(depends_on)"},
	},
	{
		table:   "task_uda_values",
		columns: []string{"workspace_id", "task_uuid", "name", "value", "value_type", "orphan"},
		ddl: `CREATE TABLE task_uda_values (
	workspace_id TEXT NOT NULL,
	task_uuid TEXT NOT NULL,
	name TEXT NOT NULL,
	value TEXT NOT NULL,
	value_type TEXT,
	orphan NUMERIC NOT NULL DEFAULT false,
	PRIMARY KEY (task_uuid, name),
	CONSTRAINT fk_tasks_udas FOREIGN KEY (task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`,
		indexes: []string{
			"CREATE INDEX IF NOT EXISTS idx_task_uda_values_workspace_id ON task_uda_values(workspace_id)",
			"CREATE INDEX IF NOT EXISTS idx_task_uda_values_task_uuid ON task_uda_values(task_uuid)",
		},
	},
	{
		table:   "task_links",
		columns: []string{"id", "task_uuid", "type", "url", "title", "created_at", "created_by"},
		ddl: `CREATE TABLE task_links (
	id TEXT PRIMARY KEY,
	task_uuid TEXT NOT NULL,
	type TEXT NOT NULL,
	url TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	created_by TEXT NOT NULL,
	CONSTRAINT fk_tasks_links FOREIGN KEY (task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`,
		indexes: []string{
			"CREATE UNIQUE INDEX IF NOT EXISTS idx_task_links_task_url ON task_links(task_uuid, url)",
			"CREATE INDEX IF NOT EXISTS idx_task_links_task ON task_links(task_uuid)",
		},
	},
}

func rebuildM5TaskRelationForeignKeys(tx m5MigrationTx) error {
	for _, schema := range m5TaskRelationSchemas {
		exists, err := tableExists(tx, schema.table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if ok, err := relationHasTaskForeignKey(tx, schema.table); err != nil {
			return err
		} else if ok {
			continue
		}
		if err := rebuildM5TaskRelationTable(tx, schema); err != nil {
			return err
		}
	}
	return nil
}

func relationHasTaskForeignKey(tx m5MigrationTx, table string) (bool, error) {
	rows, err := tx.query("PRAGMA foreign_key_list(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, seq int
		var targetTable, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &targetTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return false, err
		}
		if targetTable == "tasks" && from == "task_uuid" && to == "uuid" {
			return true, nil
		}
	}
	return false, rows.Err()
}

func rebuildM5TaskRelationTable(tx m5MigrationTx, schema m5TaskRelationSchema) error {
	oldTable := schema.table + "_old_m5"
	if err := dropM5TaskRelationIndexes(tx, schema.table); err != nil {
		return err
	}
	if err := tx.exec("ALTER TABLE " + schema.table + " RENAME TO " + oldTable); err != nil {
		return err
	}
	if err := tx.exec(schema.ddl); err != nil {
		return err
	}
	columnList := strings.Join(schema.columns, ", ")
	copySQL := "INSERT INTO " + schema.table + "(" + columnList + ") SELECT " + columnList + " FROM " + oldTable
	if schema.table == "task_annotations" {
		hasID, err := columnExists(tx, oldTable, "id")
		if err != nil {
			return err
		}
		if !hasID {
			copySQL = "INSERT INTO task_annotations(id, task_uuid, entry, description) SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), task_uuid, entry, description FROM " + oldTable
		}
	}
	if err := tx.exec(copySQL); err != nil {
		return err
	}
	if err := tx.exec("DROP TABLE " + oldTable); err != nil {
		return err
	}
	for _, indexSQL := range schema.indexes {
		if err := tx.exec(indexSQL); err != nil {
			return err
		}
	}
	return nil
}

func dropM5TaskRelationIndexes(tx m5MigrationTx, table string) error {
	rows, err := tx.query("PRAGMA index_list(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	var indexes []string
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return err
		}
		if origin == "c" {
			indexes = append(indexes, name)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, index := range indexes {
		if err := tx.exec("DROP INDEX IF EXISTS " + index); err != nil {
			return err
		}
	}
	return nil
}

func assertNoM5ForeignKeyViolations(tx m5MigrationTx) error {
	rows, err := tx.query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID int64
		var parent string
		var fkID int
		if err := rows.Scan(&table, &rowID, &parent, &fkID); err != nil {
			return err
		}
		return fmt.Errorf("foreign key violation after M5 migration: table=%s rowid=%d parent=%s fk=%d", table, rowID, parent, fkID)
	}
	return rows.Err()
}

func writeM5SkippedReport(tx m5MigrationTx, rows []m5SkippedProject) error {
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return setMetaInTx(tx, m5ProjectsSkippedMetaKey, string(data))
}

func setMetaInTx(tx m5MigrationTx, key, value string) error {
	return tx.exec("INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
}

func metaKeyApplied(tx m5MigrationTx, key string) (bool, error) {
	var value string
	err := tx.queryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value == "true", nil
}

func columnExists(tx m5MigrationTx, table, column string) (bool, error) {
	rows, err := tx.query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func columnNotNull(tx m5MigrationTx, table, column string) (bool, error) {
	rows, err := tx.query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return notNull == 1, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, fmt.Errorf("%s.%s column not found", table, column)
}

func validateV011ProjectSlugs(tx m5MigrationTx) error {
	rows, err := tx.query("SELECT workspace_id, slug FROM projects ORDER BY workspace_id, slug")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var workspaceID, slug string
		if err := rows.Scan(&workspaceID, &slug); err != nil {
			return err
		}
		if !isValidTaskSlugProjectSlug(slug) {
			return fmt.Errorf("project_slug_migration_required: workspace %s project slug %q does not match ^[a-z][a-z0-9]{2,9}$", workspaceID, slug)
		}
	}
	return rows.Err()
}

func isValidTaskSlugProjectSlug(slug string) bool {
	if len(slug) < 3 || len(slug) > 10 {
		return false
	}
	if slug[0] < 'a' || slug[0] > 'z' {
		return false
	}
	for _, ch := range slug {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func backfillProjectSeqs(tx m5MigrationTx) error {
	if err := tx.exec("UPDATE tasks SET project_seq = NULL WHERE project_id IS NULL"); err != nil {
		return err
	}
	rows, err := tx.query(`SELECT uuid, workspace_id, project_id
FROM tasks
WHERE project_id IS NOT NULL
ORDER BY workspace_id ASC, project_id ASC, entry ASC, uuid ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	nextByProject := map[string]int64{}
	for rows.Next() {
		var uuidValue, workspaceID, projectID string
		if err := rows.Scan(&uuidValue, &workspaceID, &projectID); err != nil {
			return err
		}
		key := workspaceID + "\x00" + projectID
		next := nextByProject[key] + 1
		nextByProject[key] = next
		if err := tx.exec("UPDATE tasks SET project_seq = ? WHERE uuid = ?", next, uuidValue); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.exec(`UPDATE projects
SET next_task_seq = COALESCE((
	SELECT MAX(project_seq) + 1
	FROM tasks
	WHERE tasks.workspace_id = projects.workspace_id
	  AND tasks.project_id = projects.id
), 1)`)
}

func tableExists(tx m5MigrationTx, table string) (bool, error) {
	var count int
	if err := tx.queryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func m5ProjectsMarkerApplied(tx m5MigrationTx) (bool, error) {
	var value string
	err := tx.queryRow("SELECT value FROM meta WHERE key = ?", m5ProjectsAppliedMetaKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if value != "true" {
		return false, nil
	}
	return tasksHasM5ProjectForeignKey(tx)
}

func tasksHasM5ProjectForeignKey(tx m5MigrationTx) (bool, error) {
	rows, err := tx.query("PRAGMA foreign_key_list(tasks)")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	type fkCol struct {
		table string
		from  string
		to    string
	}
	colsByID := map[int][]fkCol{}
	for rows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return false, err
		}
		colsByID[id] = append(colsByID[id], fkCol{table: table, from: from, to: to})
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, cols := range colsByID {
		if len(cols) != 2 {
			continue
		}
		seenProjectID := false
		seenWorkspaceID := false
		tableOK := true
		for _, col := range cols {
			if col.table != "projects" {
				tableOK = false
			}
			if col.from == "project_id" && col.to == "id" {
				seenProjectID = true
			}
			if col.from == "workspace_id" && col.to == "workspace_id" {
				seenWorkspaceID = true
			}
		}
		if tableOK && seenProjectID && seenWorkspaceID {
			return true, nil
		}
	}
	return false, nil
}
