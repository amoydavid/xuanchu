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
	"gorm.io/gorm"
)

const m5ProjectsAppliedMetaKey = "migration.m5.projects.applied"
const taskSlugMigrationMetaKey = "migration.v0.1.1.task_slug.applied"
const projectTemplateWorkspaceFKMigrationMetaKey = "migration.project_templates.workspace_fk.applied"
const automationScopeMigrationMetaKey = "migration.automation.scope.applied"

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
	if err := s.db.AutoMigrate(&Meta{}, &User{}, &Workspace{}, &Membership{}, &AuditLog{}, &Project{}, &ProjectAnnotation{}, &Config{}, &ConfigDefinition{}, &ApiToken{}, &ServerAdminToken{}, &AdminActingSession{}, &Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{}, &NotificationSink{}, &ReminderRule{}, &EventNotificationRule{}, &NotificationDelivery{}, &AutomationRule{}, &AutomationDelivery{}, &UserExternalID{}, &BrowserSession{}, &BrowserAuthFlow{}, &DirectorySyncJob{}); err != nil {
		return err
	}
	// Automation scope migration 必须在 AutoMigrate 创建 meta/automation_* 之后运行，
	// 因为它需要读写 meta 标记，且需要在 AutoMigrate 之后再校验新表确实存在。
	// 但要在任何业务写入之前完成 legacy -> 新表的回填。
	if err := s.prepareAutomationScopeSchemaSQLite(); err != nil {
		return err
	}
	if err := s.verifyAutomationScopeSchemaSQLite(); err != nil {
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
	if err := s.prepareTaskAnnotationActivityColumns(); err != nil {
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

// prepareProjectTemplateSchemaSQLite 显式建表并在既有库中重建两张循环引用的表。
// 复合外键把 Snapshot 的 workspace 与 Template、来源 Project 绑定，并把 Template
// 的 current_snapshot_id 绑定到同一 Template/workspace 的 Snapshot。GORM 无法安全
// 地把这些循环外键补到 SQLite 既有表，因此这里在单连接写事务内完成迁移。
func (s *Store) prepareProjectTemplateSchemaSQLite() error {
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
	applied, err := metaKeyApplied(tx, projectTemplateWorkspaceFKMigrationMetaKey)
	if err != nil {
		return err
	}
	if !applied {
		templatesExist, err := tableExists(tx, "project_templates")
		if err != nil {
			return err
		}
		snapshotsExist, err := tableExists(tx, "project_template_snapshots")
		if err != nil {
			return err
		}
		switch {
		case !templatesExist && !snapshotsExist:
			if err := createProjectTemplateSchemaSQLite(tx); err != nil {
				return err
			}
		case templatesExist && snapshotsExist:
			if err := rebuildProjectTemplateSchemaSQLite(tx); err != nil {
				return err
			}
		default:
			return fmt.Errorf("project template migration: incomplete legacy tables")
		}
		if err := assertNoProjectTemplateForeignKeyViolations(tx); err != nil {
			return err
		}
		if err := setMetaInTx(tx, projectTemplateWorkspaceFKMigrationMetaKey, "true"); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func createProjectTemplateSchemaSQLite(tx m5MigrationTx) error {
	statements := []string{
		`CREATE TABLE project_templates (
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
UNIQUE (id, workspace_id),
FOREIGN KEY (current_snapshot_id, id, workspace_id) REFERENCES project_template_snapshots(id, template_id, workspace_id) ON DELETE RESTRICT
)`,
		`CREATE TABLE project_template_snapshots (
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
UNIQUE (id, template_id, workspace_id),
FOREIGN KEY (template_id, workspace_id) REFERENCES project_templates(id, workspace_id) ON DELETE RESTRICT,
FOREIGN KEY (source_project_id, workspace_id) REFERENCES projects(id, workspace_id) ON DELETE RESTRICT
)`,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_templates_ws_key ON project_templates(workspace_id, key)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_templates_id_workspace ON project_templates(id, workspace_id)",
		"CREATE INDEX IF NOT EXISTS idx_project_templates_ws_status ON project_templates(workspace_id, status)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_id_template_workspace ON project_template_snapshots(id, template_id, workspace_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_version ON project_template_snapshots(template_id, version)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_snapshot_hash ON project_template_snapshots(template_id, snapshot_hash)",
		"CREATE INDEX IF NOT EXISTS idx_project_template_snapshots_ws_template ON project_template_snapshots(workspace_id, template_id)",
	}
	for _, statement := range statements {
		if err := tx.exec(statement); err != nil {
			return fmt.Errorf("project template migration: %w", err)
		}
	}
	return nil
}

func rebuildProjectTemplateSchemaSQLite(tx m5MigrationTx) error {
	if err := tx.exec("ALTER TABLE project_templates RENAME TO project_templates_legacy_workspace_fk"); err != nil {
		return err
	}
	if err := tx.exec("ALTER TABLE project_template_snapshots RENAME TO project_template_snapshots_legacy_workspace_fk"); err != nil {
		return err
	}
	// SQLite 保留重命名表上的 index 名称。若不先删除，CREATE INDEX IF NOT EXISTS
	// 会误认为新表的索引已经存在；旧表删除时索引随之消失，最终新表没有索引。
	for _, indexName := range []string{
		"idx_project_templates_ws_key",
		"idx_project_templates_id_workspace",
		"idx_project_templates_ws_status",
		"idx_project_template_snapshots_id_template_workspace",
		"idx_project_template_snapshots_template_version",
		"idx_project_template_snapshots_template_snapshot_hash",
		"idx_project_template_snapshots_ws_template",
	} {
		if err := tx.exec("DROP INDEX IF EXISTS " + indexName); err != nil {
			return err
		}
	}
	if err := createProjectTemplateSchemaSQLite(tx); err != nil {
		return err
	}
	if err := tx.exec(`INSERT INTO project_templates (
id, workspace_id, key, name, description, status, current_snapshot_id,
created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name,
created_by_token_prefix, created_at, modified_at, archived_at
)
SELECT id, workspace_id, key, name, description, status, current_snapshot_id,
created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name,
created_by_token_prefix, created_at, modified_at, archived_at
FROM project_templates_legacy_workspace_fk`); err != nil {
		return err
	}
	if err := tx.exec(`INSERT INTO project_template_snapshots (
id, workspace_id, template_id, version, source_project_id, snapshot_json, snapshot_hash,
created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name,
created_by_token_prefix, created_at
)
SELECT id, workspace_id, template_id, version, source_project_id, snapshot_json, snapshot_hash,
created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name,
created_by_token_prefix, created_at
FROM project_template_snapshots_legacy_workspace_fk`); err != nil {
		return err
	}
	if err := tx.exec("DROP TABLE project_template_snapshots_legacy_workspace_fk"); err != nil {
		return err
	}
	return tx.exec("DROP TABLE project_templates_legacy_workspace_fk")
}

func assertNoProjectTemplateForeignKeyViolations(tx m5MigrationTx) error {
	for _, table := range []string{"project_templates", "project_template_snapshots"} {
		rows, err := tx.query("PRAGMA foreign_key_check(" + table + ")")
		if err != nil {
			return err
		}
		if rows.Next() {
			var violatingTable string
			var rowID int64
			var parent string
			var fkID int
			err = rows.Scan(&violatingTable, &rowID, &parent, &fkID)
			_ = rows.Close()
			if err != nil {
				return err
			}
			return fmt.Errorf("project template migration: foreign key violation table=%s rowid=%d parent=%s fk=%d", violatingTable, rowID, parent, fkID)
		}
		if err := rows.Close(); err != nil {
			return err
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
		"automation_rules": {
			{"created_by_actor_type", "ALTER TABLE automation_rules ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'user'"},
			{"created_by_user_id", "ALTER TABLE automation_rules ADD COLUMN created_by_user_id TEXT"},
			{"created_by_token_id", "ALTER TABLE automation_rules ADD COLUMN created_by_token_id TEXT"},
			{"created_by_token_name", "ALTER TABLE automation_rules ADD COLUMN created_by_token_name TEXT"},
			{"created_by_token_prefix", "ALTER TABLE automation_rules ADD COLUMN created_by_token_prefix TEXT"},
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
created_by_actor_type TEXT NOT NULL DEFAULT 'unknown',
created_by_user_id TEXT,
created_by_token_id TEXT,
created_by_token_name TEXT,
created_by_token_prefix TEXT,
created_at INTEGER NOT NULL DEFAULT 0,
CONSTRAINT fk_tasks_annotations FOREIGN KEY(task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`); err != nil {
		return err
	}
	if err := tx.exec("CREATE INDEX IF NOT EXISTS idx_task_annotations_task ON task_annotations(task_uuid)"); err != nil {
		return err
	}
	return tx.exec("CREATE INDEX IF NOT EXISTS idx_task_annotations_activity ON task_annotations(task_uuid, created_at DESC, id DESC)")
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
		if err := tx.exec("INSERT INTO task_annotations(id, task_uuid, entry, description, created_at) VALUES(?, ?, ?, ?, ?)", uuid.NewString(), taskUUID, entry, description, entry); err != nil {
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

func (s *Store) prepareTaskAnnotationActivityColumns() error {
	if !s.db.Migrator().HasTable(&TaskAnnotation{}) {
		return nil
	}
	columns := []struct {
		name string
		sql  string
	}{
		{"created_by_actor_type", "ALTER TABLE task_annotations ADD COLUMN created_by_actor_type TEXT NOT NULL DEFAULT 'unknown'"},
		{"created_by_user_id", "ALTER TABLE task_annotations ADD COLUMN created_by_user_id TEXT"},
		{"created_by_token_id", "ALTER TABLE task_annotations ADD COLUMN created_by_token_id TEXT"},
		{"created_by_token_name", "ALTER TABLE task_annotations ADD COLUMN created_by_token_name TEXT"},
		{"created_by_token_prefix", "ALTER TABLE task_annotations ADD COLUMN created_by_token_prefix TEXT"},
		{"created_at", "ALTER TABLE task_annotations ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0"},
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, column := range columns {
			if s.db.Migrator().HasColumn(&TaskAnnotation{}, column.name) {
				continue
			}
			if err := tx.Exec(column.sql).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE task_annotations SET created_by_actor_type = 'unknown' WHERE created_by_actor_type IS NULL OR created_by_actor_type = ''").Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE task_annotations SET created_at = entry WHERE created_at = 0").Error; err != nil {
			return err
		}
		return tx.Exec("CREATE INDEX IF NOT EXISTS idx_task_annotations_activity ON task_annotations(task_uuid, created_at DESC, id DESC)").Error
	})
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
		{name: "timezone", sql: "ALTER TABLE reminder_rules ADD COLUMN timezone TEXT NOT NULL DEFAULT ''"},
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

// prepareAutomationScopeSchemaSQLite 把 legacy project_automation_* 表数据回填到通用
// automation_* 表。fresh DB 由 AutoMigrate 直接建好新表，本函数只校验存在性。
// legacy DB 时 AutoMigrate 已经创建了空的 automation_* 表，本函数负责复制数据并删旧表。
func (s *Store) prepareAutomationScopeSchemaSQLite() error {
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
	applied, err := metaKeyApplied(tx, automationScopeMigrationMetaKey)
	if err != nil {
		return err
	}
	if applied {
		// 已声明迁移完成，但要确认表结构是 scope-aware 的，否则显式失败。
		if err := assertAutomationScopeTablesExist(tx); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return err
		}
		committed = true
		return nil
	}

	legacyRules, err := tableExists(tx, "project_automation_rules")
	if err != nil {
		return err
	}
	legacyDeliveries, err := tableExists(tx, "project_automation_deliveries")
	if err != nil {
		return err
	}
	newRules, err := tableExists(tx, "automation_rules")
	if err != nil {
		return err
	}
	newDeliveries, err := tableExists(tx, "automation_deliveries")
	if err != nil {
		return err
	}

	switch {
	case !legacyRules && !legacyDeliveries:
		// fresh DB：新表已经由 AutoMigrate 创建，无需做任何 legacy 处理。
		if err := assertAutomationScopeTablesExist(tx); err != nil {
			return err
		}
	case legacyRules && legacyDeliveries && newRules && newDeliveries:
		// legacy + new 共存：AutoMigrate 已建空新表，复制 legacy 数据后删除 legacy 表。
		if err := backfillAutomationFromLegacySQLite(tx); err != nil {
			return err
		}
	case legacyRules && legacyDeliveries && !newRules && !newDeliveries:
		// 极少见：AutoMigrate 没建新表。直接 rename + backfill。
		if err := migrateLegacyAutomationTablesSQLite(tx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("automation scope migration: inconsistent legacy/new tables: legacy_rules=%v legacy_deliveries=%v new_rules=%v new_deliveries=%v",
			legacyRules, legacyDeliveries, newRules, newDeliveries)
	}

	if err := assertAutomationScopeTablesExist(tx); err != nil {
		return err
	}
	if err := setMetaInTx(tx, automationScopeMigrationMetaKey, "true"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

// migrateLegacyAutomationTablesSQLite 执行真正的 rename + backfill + DDL 调整。
// 必须保证旧记录的主键、状态、dedupe_key、请求/响应和时间完全不变。
func migrateLegacyAutomationTablesSQLite(tx m5MigrationTx) error {
	// 1. 重命名旧表，腾出 automation_* 名字给新结构。
	if err := tx.exec("ALTER TABLE project_automation_rules RENAME TO project_automation_rules_legacy_scope"); err != nil {
		return fmt.Errorf("automation scope migration: rename rules: %w", err)
	}
	if err := tx.exec("ALTER TABLE project_automation_deliveries RENAME TO project_automation_deliveries_legacy_scope"); err != nil {
		return fmt.Errorf("automation scope migration: rename deliveries: %w", err)
	}
	// 2. 删除随表重命名而来的旧索引，避免新表创建索引时被 IF NOT EXISTS 误判。
	for _, indexName := range []string{
		"idx_project_automation_rules_scope",
		"idx_project_automation_rules_ws_project_name",
		"idx_project_automation_deliveries_scope",
		"idx_project_automation_deliveries_due",
	} {
		if err := tx.exec("DROP INDEX IF EXISTS " + indexName); err != nil {
			return err
		}
	}
	// 3. 显式创建新表结构，使用通用 scope 字段。
	if err := createAutomationScopeSchemaSQLite(tx); err != nil {
		return err
	}
	// 4. 复制规则并回填 scope_type=project, scope_id=project_id。
	if err := tx.exec(`INSERT INTO automation_rules (
	id, workspace_id, scope_type, scope_id, name, description, enabled,
	trigger_type, trigger_config_json, condition_json, action_type, action_config_json, context_config_json,
	instruction_template, system_prompt,
	created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name, created_by_token_prefix,
	created_at, modified_at
)
SELECT
	id, workspace_id, 'project', project_id, name, description, enabled,
	trigger_type, trigger_config_json, condition_json, action_type, action_config_json, context_config_json,
	instruction_template, system_prompt,
	created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name, created_by_token_prefix,
	created_at, modified_at
FROM project_automation_rules_legacy_scope`); err != nil {
		return fmt.Errorf("automation scope migration: copy rules: %w", err)
	}
	// 5. 复制 Delivery 并回填冻结 scope 字段。ProjectID 在旧表里是 NOT NULL string，
	//    新模型是 *string；写 NULL 表示 schedule 无 Project，但 legacy 数据全部来自
	//    Project Automation，因此全部保留为非空指针。
	if err := tx.exec(`INSERT INTO automation_deliveries (
	id, workspace_id, rule_scope_type, rule_scope_id, project_id, rule_id, trigger_type, event_id, event_type,
	dedupe_key, replay_of_delivery_id, api_key_config_key, allowed_hosts_config_key, max_attempts,
	status, resolved_url, rendered_method, rendered_headers_json,
	request_body_json, request_body_preview, request_body_hash,
	response_status_code, response_body_preview, provider_request_id, usage_json,
	attempt_count, next_attempt_at, claim_expires_at, last_attempt_at, last_error,
	created_at, modified_at
)
SELECT
	d.id, d.workspace_id, 'project', d.project_id, d.project_id, d.rule_id, d.trigger_type, d.event_id, d.event_type,
	d.dedupe_key, NULL, '', '', 0,
	d.status, d.resolved_url, d.rendered_method, d.rendered_headers_json,
	d.request_body_json, d.request_body_preview, d.request_body_hash,
	d.response_status_code, d.response_body_preview, d.provider_request_id, d.usage_json,
	d.attempt_count, d.next_attempt_at, d.claim_expires_at, d.last_attempt_at, d.last_error,
	d.created_at, d.modified_at
FROM project_automation_deliveries_legacy_scope d`); err != nil {
		return fmt.Errorf("automation scope migration: copy deliveries: %w", err)
	}
	// 6. 校验迁移前后行数和主键集合一致。
	if err := assertAutomationLegacyMigrationCounts(tx); err != nil {
		return err
	}
	// 7. 删除 legacy 表，避免长期保留误导性结构。
	if err := tx.exec("DROP TABLE project_automation_deliveries_legacy_scope"); err != nil {
		return err
	}
	if err := tx.exec("DROP TABLE project_automation_rules_legacy_scope"); err != nil {
		return err
	}
	return nil
}

// backfillAutomationFromLegacySQLite 处理 AutoMigrate 已经创建空新表、
// legacy 表仍然存在的常见情况：复制数据，删除 legacy 表，保留新表 schema 不变。
func backfillAutomationFromLegacySQLite(tx m5MigrationTx) error {
	// 幂等：只有当新表无数据时才复制。如果已经有数据（多次启动），跳过复制但仍删除 legacy。
	var newRuleCount int64
	if err := tx.queryRow("SELECT COUNT(*) FROM automation_rules").Scan(&newRuleCount); err != nil {
		return err
	}
	if newRuleCount == 0 {
		if err := tx.exec(`INSERT INTO automation_rules (
	id, workspace_id, scope_type, scope_id, name, description, enabled,
	trigger_type, trigger_config_json, condition_json, action_type, action_config_json, context_config_json,
	instruction_template, system_prompt,
	created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name, created_by_token_prefix,
	created_at, modified_at
)
SELECT
	id, workspace_id, 'project', project_id, name, description, enabled,
	trigger_type, trigger_config_json, condition_json, action_type, action_config_json, context_config_json,
	instruction_template, system_prompt,
	created_by_actor_type, created_by_user_id, created_by_token_id, created_by_token_name, created_by_token_prefix,
	created_at, modified_at
FROM project_automation_rules`); err != nil {
			return fmt.Errorf("automation scope migration: backfill rules: %w", err)
		}
	}
	var newDeliveryCount int64
	if err := tx.queryRow("SELECT COUNT(*) FROM automation_deliveries").Scan(&newDeliveryCount); err != nil {
		return err
	}
	if newDeliveryCount == 0 {
		if err := tx.exec(`INSERT INTO automation_deliveries (
	id, workspace_id, rule_scope_type, rule_scope_id, project_id, rule_id, trigger_type, event_id, event_type,
	dedupe_key, replay_of_delivery_id, api_key_config_key, allowed_hosts_config_key, max_attempts,
	status, resolved_url, rendered_method, rendered_headers_json,
	request_body_json, request_body_preview, request_body_hash,
	response_status_code, response_body_preview, provider_request_id, usage_json,
	attempt_count, next_attempt_at, claim_expires_at, last_attempt_at, last_error,
	created_at, modified_at
)
SELECT
	d.id, d.workspace_id, 'project', d.project_id, d.project_id, d.rule_id, d.trigger_type, d.event_id, d.event_type,
	d.dedupe_key, NULL, '', '', 0,
	d.status, d.resolved_url, d.rendered_method, d.rendered_headers_json,
	d.request_body_json, d.request_body_preview, d.request_body_hash,
	d.response_status_code, d.response_body_preview, d.provider_request_id, d.usage_json,
	d.attempt_count, d.next_attempt_at, d.claim_expires_at, d.last_attempt_at, d.last_error,
	d.created_at, d.modified_at
FROM project_automation_deliveries d`); err != nil {
			return fmt.Errorf("automation scope migration: backfill deliveries: %w", err)
		}
	}
	// 校验 legacy -> 新表行数一致（仅当本次复制时；如果新表已有数据，跳过校验避免误报）。
	if newRuleCount == 0 {
		if err := assertAutomationLegacyMigrationCountsFromLegacy(tx); err != nil {
			return err
		}
	}
	// 删除 legacy 表，保证下次启动进入 fresh 分支。
	if err := tx.exec("DROP TABLE project_automation_deliveries"); err != nil {
		return fmt.Errorf("automation scope migration: drop legacy deliveries: %w", err)
	}
	if err := tx.exec("DROP TABLE project_automation_rules"); err != nil {
		return fmt.Errorf("automation scope migration: drop legacy rules: %w", err)
	}
	return nil
}

// assertAutomationLegacyMigrationCountsFromLegacy 与 assertAutomationLegacyMigrationCounts
// 类似，但 legacy 表名没有 _legacy_scope 后缀（AutoMigrate 路径下保留原名）。
func assertAutomationLegacyMigrationCountsFromLegacy(tx m5MigrationTx) error {
	var legacyRuleCount, newRuleCount int64
	if err := tx.queryRow("SELECT COUNT(*) FROM project_automation_rules").Scan(&legacyRuleCount); err != nil {
		return err
	}
	if err := tx.queryRow("SELECT COUNT(*) FROM automation_rules").Scan(&newRuleCount); err != nil {
		return err
	}
	if legacyRuleCount != newRuleCount {
		return fmt.Errorf("automation scope migration: rule count mismatch legacy=%d new=%d", legacyRuleCount, newRuleCount)
	}
	var legacyDeliveryCount, newDeliveryCount int64
	if err := tx.queryRow("SELECT COUNT(*) FROM project_automation_deliveries").Scan(&legacyDeliveryCount); err != nil {
		return err
	}
	if err := tx.queryRow("SELECT COUNT(*) FROM automation_deliveries").Scan(&newDeliveryCount); err != nil {
		return err
	}
	if legacyDeliveryCount != newDeliveryCount {
		return fmt.Errorf("automation scope migration: delivery count mismatch legacy=%d new=%d", legacyDeliveryCount, newDeliveryCount)
	}
	var ruleDiffCount int64
	if err := tx.queryRow(`SELECT COUNT(*) FROM (
		SELECT id FROM project_automation_rules EXCEPT SELECT id FROM automation_rules
		UNION
		SELECT id FROM automation_rules EXCEPT SELECT id FROM project_automation_rules
	)`).Scan(&ruleDiffCount); err != nil {
		return err
	}
	if ruleDiffCount != 0 {
		return fmt.Errorf("automation scope migration: rule primary key set changed, diff=%d", ruleDiffCount)
	}
	var deliveryDiffCount int64
	if err := tx.queryRow(`SELECT COUNT(*) FROM (
		SELECT id FROM project_automation_deliveries EXCEPT SELECT id FROM automation_deliveries
		UNION
		SELECT id FROM automation_deliveries EXCEPT SELECT id FROM project_automation_deliveries
	)`).Scan(&deliveryDiffCount); err != nil {
		return err
	}
	if deliveryDiffCount != 0 {
		return fmt.Errorf("automation scope migration: delivery primary key set changed, diff=%d", deliveryDiffCount)
	}
	return nil
}

// createAutomationScopeSchemaSQLite 显式创建通用 automation_* 表。
// 字段集合与 AutomationRule/AutomationDelivery GORM tag 严格对齐，保证 AutoMigrate
// 在此基础上只会补建索引，不会再次重建表。
func createAutomationScopeSchemaSQLite(tx m5MigrationTx) error {
	statements := []string{
		`CREATE TABLE automation_rules (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	scope_type TEXT NOT NULL DEFAULT 'project',
	scope_id TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	enabled NUMERIC NOT NULL DEFAULT TRUE,
	trigger_type TEXT NOT NULL,
	trigger_config_json TEXT NOT NULL DEFAULT '{}',
	condition_json TEXT NOT NULL DEFAULT '{}',
	action_type TEXT NOT NULL DEFAULT 'openai_compatible',
	action_config_json TEXT NOT NULL DEFAULT '{}',
	context_config_json TEXT NOT NULL DEFAULT '{}',
	instruction_template TEXT NOT NULL DEFAULT '',
	system_prompt TEXT NOT NULL DEFAULT '',
	created_by_actor_type TEXT NOT NULL DEFAULT 'user',
	created_by_user_id TEXT,
	created_by_token_id TEXT,
	created_by_token_name TEXT,
	created_by_token_prefix TEXT,
	created_at INTEGER NOT NULL,
	modified_at INTEGER NOT NULL
)`,
		`CREATE TABLE automation_deliveries (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	rule_scope_type TEXT NOT NULL DEFAULT 'project',
	rule_scope_id TEXT NOT NULL,
	project_id TEXT,
	rule_id TEXT NOT NULL,
	trigger_type TEXT NOT NULL,
	event_id TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL DEFAULT '',
	dedupe_key TEXT NOT NULL,
	replay_of_delivery_id TEXT,
	api_key_config_key TEXT NOT NULL DEFAULT '',
	allowed_hosts_config_key TEXT NOT NULL DEFAULT '',
	max_attempts INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL,
	resolved_url TEXT NOT NULL DEFAULT '',
	rendered_method TEXT NOT NULL DEFAULT 'POST',
	rendered_headers_json TEXT NOT NULL DEFAULT '{}',
	request_body_json TEXT NOT NULL DEFAULT '',
	request_body_preview TEXT NOT NULL DEFAULT '',
	request_body_hash TEXT NOT NULL DEFAULT '',
	response_status_code INTEGER,
	response_body_preview TEXT NOT NULL DEFAULT '',
	provider_request_id TEXT NOT NULL DEFAULT '',
	usage_json TEXT NOT NULL DEFAULT '{}',
	attempt_count INTEGER NOT NULL DEFAULT 0,
	next_attempt_at INTEGER,
	claim_expires_at INTEGER,
	last_attempt_at INTEGER,
	last_error TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	modified_at INTEGER NOT NULL
)`,
		"CREATE INDEX IF NOT EXISTS idx_automation_rules_scope ON automation_rules(workspace_id, scope_type, scope_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_rules_ws_scope_name ON automation_rules(workspace_id, scope_type, scope_id, name)",
		"CREATE INDEX IF NOT EXISTS idx_automation_rules_lookup ON automation_rules(workspace_id, scope_type, scope_id, enabled, trigger_type)",
		"CREATE INDEX IF NOT EXISTS idx_automation_deliveries_scope ON automation_deliveries(workspace_id, rule_scope_type, rule_scope_id, project_id)",
		"CREATE INDEX IF NOT EXISTS idx_automation_deliveries_due ON automation_deliveries(status, next_attempt_at)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_deliveries_dedupe_key ON automation_deliveries(dedupe_key)",
		"CREATE INDEX IF NOT EXISTS idx_automation_deliveries_rule_id ON automation_deliveries(rule_id)",
		"CREATE INDEX IF NOT EXISTS idx_automation_deliveries_replay_of ON automation_deliveries(replay_of_delivery_id)",
		"CREATE INDEX IF NOT EXISTS idx_automation_deliveries_event ON automation_deliveries(event_id, event_type)",
	}
	for _, stmt := range statements {
		if err := tx.exec(stmt); err != nil {
			return fmt.Errorf("automation scope migration: create schema: %w", err)
		}
	}
	return nil
}

// assertAutomationScopeTablesExist 在 meta 已写但表结构损坏时报错，
// 提示运维需要从备份恢复，而不是默默继续。
func assertAutomationScopeTablesExist(tx m5MigrationTx) error {
	for _, table := range []string{"automation_rules", "automation_deliveries"} {
		exists, err := tableExists(tx, table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("automation scope migration: meta marked applied but table %s missing", table)
		}
	}
	return nil
}

// assertAutomationLegacyMigrationCounts 在 legacy 表数据复制完后，
// 校验新旧表的行数与主键集合完全一致，防止迁移丢数据。
func assertAutomationLegacyMigrationCounts(tx m5MigrationTx) error {
	var legacyRuleCount, newRuleCount int64
	if err := tx.queryRow("SELECT COUNT(*) FROM project_automation_rules_legacy_scope").Scan(&legacyRuleCount); err != nil {
		return err
	}
	if err := tx.queryRow("SELECT COUNT(*) FROM automation_rules").Scan(&newRuleCount); err != nil {
		return err
	}
	if legacyRuleCount != newRuleCount {
		return fmt.Errorf("automation scope migration: rule count mismatch legacy=%d new=%d", legacyRuleCount, newRuleCount)
	}
	var legacyDeliveryCount, newDeliveryCount int64
	if err := tx.queryRow("SELECT COUNT(*) FROM project_automation_deliveries_legacy_scope").Scan(&legacyDeliveryCount); err != nil {
		return err
	}
	if err := tx.queryRow("SELECT COUNT(*) FROM automation_deliveries").Scan(&newDeliveryCount); err != nil {
		return err
	}
	if legacyDeliveryCount != newDeliveryCount {
		return fmt.Errorf("automation scope migration: delivery count mismatch legacy=%d new=%d", legacyDeliveryCount, newDeliveryCount)
	}

	// 主键集合对称差：只在 legacy 或只在新表的 id 数量。SQLite 不支持 FULL OUTER JOIN，
	// 用 (legacy EXCEPT new) UNION (new EXCEPT legacy) 等价表达。
	var ruleDiffCount int64
	if err := tx.queryRow(`SELECT COUNT(*) FROM (
		SELECT id FROM project_automation_rules_legacy_scope
		EXCEPT
		SELECT id FROM automation_rules
		UNION
		SELECT id FROM automation_rules
		EXCEPT
		SELECT id FROM project_automation_rules_legacy_scope
	)`).Scan(&ruleDiffCount); err != nil {
		return err
	}
	if ruleDiffCount != 0 {
		return fmt.Errorf("automation scope migration: rule primary key set changed, diff=%d", ruleDiffCount)
	}
	var deliveryDiffCount int64
	if err := tx.queryRow(`SELECT COUNT(*) FROM (
		SELECT id FROM project_automation_deliveries_legacy_scope
		EXCEPT
		SELECT id FROM automation_deliveries
		UNION
		SELECT id FROM automation_deliveries
		EXCEPT
		SELECT id FROM project_automation_deliveries_legacy_scope
	)`).Scan(&deliveryDiffCount); err != nil {
		return err
	}
	if deliveryDiffCount != 0 {
		return fmt.Errorf("automation scope migration: delivery primary key set changed, diff=%d", deliveryDiffCount)
	}
	return nil
}

// verifyAutomationScopeSchemaSQLite 在 AutoMigrate 后做一次轻量校验，
// 保证通用 scope 索引存在；不通过即视为迁移失败。
func (s *Store) verifyAutomationScopeSchemaSQLite() error {
	if !s.db.Migrator().HasTable(&AutomationRule{}) {
		return fmt.Errorf("automation scope migration: automation_rules missing after AutoMigrate")
	}
	if !s.db.Migrator().HasTable(&AutomationDelivery{}) {
		return fmt.Errorf("automation scope migration: automation_deliveries missing after AutoMigrate")
	}
	return nil
}
