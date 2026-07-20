package storage

import (
	"database/sql"

	"gorm.io/gorm"
)

func (s *Store) migratePostgres() error {
	if err := s.prepareAPITokenUserIDNullablePostgres(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(
		&Meta{}, &User{}, &Workspace{}, &Membership{},
		&AuditLog{}, &Project{}, &ProjectAnnotation{}, &Config{}, &ConfigDefinition{}, &ApiToken{}, &ServerAdminToken{}, &AdminActingSession{},
		&Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{},
		&NotificationSink{}, &ReminderRule{}, &EventNotificationRule{}, &NotificationDelivery{},
		&ProjectAutomationRule{}, &ProjectAutomationDelivery{},
		&UserExternalID{}, &BrowserSession{}, &BrowserAuthFlow{}, &DirectorySyncJob{}, &Task{},
	); err != nil {
		return err
	}
	if err := s.prepareProjectTemplateSchemaPostgres(); err != nil {
		return err
	}
	if err := s.prepareTaskAnnotationIDsPostgres(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(
		&TaskTag{}, &TaskAnnotation{}, &TaskDependency{},
		&TaskAssignee{}, &TaskUDAValue{}, &TaskLink{}, &Attachment{},
	); err != nil {
		return err
	}
	if err := s.prepareTaskSeriesSchema(); err != nil {
		return err
	}
	return s.prepareActorColumnsForP2Postgres()
}

// prepareProjectTemplateSchemaPostgres 分两步建立循环 RESTRICT 外键：先建
// Snapshot -> Template，再向 Template 补 current_snapshot_id 的引用。
func (s *Store) prepareProjectTemplateSchemaPostgres() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS project_templates (
id text PRIMARY KEY,
workspace_id text NOT NULL,
key text NOT NULL,
name text NOT NULL,
description text NOT NULL DEFAULT '',
status text NOT NULL,
current_snapshot_id text,
created_by_actor_type text NOT NULL DEFAULT 'user',
created_by_user_id text,
created_by_token_id text,
created_by_token_name text,
created_by_token_prefix text,
created_at bigint NOT NULL,
modified_at bigint NOT NULL,
archived_at bigint
)`,
		`CREATE TABLE IF NOT EXISTS project_template_snapshots (
id text PRIMARY KEY,
workspace_id text NOT NULL,
template_id text NOT NULL,
version bigint NOT NULL,
source_project_id text NOT NULL,
snapshot_json text NOT NULL,
snapshot_hash text NOT NULL,
created_by_actor_type text NOT NULL DEFAULT 'user',
created_by_user_id text,
created_by_token_id text,
created_by_token_name text,
created_by_token_prefix text,
created_at bigint NOT NULL,
CONSTRAINT fk_project_template_snapshots_template FOREIGN KEY (template_id) REFERENCES project_templates(id) ON DELETE RESTRICT
)`,
		`DO $$ BEGIN
ALTER TABLE project_templates ADD CONSTRAINT fk_project_templates_current_snapshot
FOREIGN KEY (current_snapshot_id) REFERENCES project_template_snapshots(id) ON DELETE RESTRICT;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$`,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_templates_ws_key ON project_templates(workspace_id, key)",
		"CREATE INDEX IF NOT EXISTS idx_project_templates_ws_status ON project_templates(workspace_id, status)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_version ON project_template_snapshots(template_id, version)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_snapshot_hash ON project_template_snapshots(template_id, snapshot_hash)",
		"CREATE INDEX IF NOT EXISTS idx_project_template_snapshots_ws_template ON project_template_snapshots(workspace_id, template_id)",
	}
	for _, statement := range statements {
		if err := s.db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) prepareActorColumnsForP2Postgres() error {
	statements := []string{
		"ALTER TABLE project_annotations ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE project_annotations ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE project_annotations ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE project_annotations ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE project_annotations ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
		"ALTER TABLE hook_definitions ADD COLUMN IF NOT EXISTS actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE hook_definitions ADD COLUMN IF NOT EXISTS actor_token_id text",
		"ALTER TABLE hook_definitions ADD COLUMN IF NOT EXISTS actor_token_name text",
		"ALTER TABLE hook_definitions ADD COLUMN IF NOT EXISTS actor_token_prefix text",
		"ALTER TABLE hook_deliveries ADD COLUMN IF NOT EXISTS actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE hook_deliveries ADD COLUMN IF NOT EXISTS actor_token_id text",
		"ALTER TABLE hook_deliveries ADD COLUMN IF NOT EXISTS actor_token_name text",
		"ALTER TABLE hook_deliveries ADD COLUMN IF NOT EXISTS actor_token_prefix text",
		"ALTER TABLE notification_sinks ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE notification_sinks ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE notification_sinks ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE notification_sinks ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE notification_sinks ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
		"ALTER TABLE reminder_rules ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE reminder_rules ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE reminder_rules ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE reminder_rules ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE reminder_rules ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
		"ALTER TABLE event_notification_rules ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE event_notification_rules ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE event_notification_rules ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE event_notification_rules ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE event_notification_rules ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
		"ALTER TABLE notification_deliveries ADD COLUMN IF NOT EXISTS actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE notification_deliveries ADD COLUMN IF NOT EXISTS actor_user_id text",
		"ALTER TABLE notification_deliveries ADD COLUMN IF NOT EXISTS actor_token_id text",
		"ALTER TABLE notification_deliveries ADD COLUMN IF NOT EXISTS actor_token_name text",
		"ALTER TABLE notification_deliveries ADD COLUMN IF NOT EXISTS actor_token_prefix text",
		"ALTER TABLE task_links ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE task_links ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE task_links ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE task_links ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE task_links ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
		"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
	}
	for _, stmt := range statements {
		if err := s.db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return s.db.Exec(`UPDATE notification_deliveries SET actor_user_id = recipient_user_id WHERE (actor_type = 'user' OR actor_type = '') AND (actor_user_id IS NULL OR actor_user_id = '') AND recipient_user_id <> ''`).Error
}

func (s *Store) prepareAPITokenUserIDNullablePostgres() error {
	var tableName sql.NullString
	if err := s.db.Raw("SELECT to_regclass('api_tokens')::text").Scan(&tableName).Error; err != nil {
		return err
	}
	if !postgresRegclassFound(tableName) {
		return nil
	}
	return s.db.Exec("ALTER TABLE api_tokens ALTER COLUMN user_id DROP NOT NULL").Error
}

func (s *Store) prepareTaskAnnotationIDsPostgres() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var tableName sql.NullString
		err := tx.Raw("SELECT to_regclass('task_annotations')::text").Scan(&tableName).Error
		if err != nil {
			return err
		}
		if !postgresRegclassFound(tableName) {
			return nil
		}
		var idColumnCount int64
		if err := tx.Raw(`
SELECT count(*)
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = 'task_annotations'
  AND column_name = 'id'`).Scan(&idColumnCount).Error; err != nil {
			return err
		}
		if idColumnCount > 0 {
			return nil
		}
		if err := tx.Exec("ALTER TABLE task_annotations ADD COLUMN id text").Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE task_annotations SET id = gen_random_uuid()::text WHERE id IS NULL OR id = ''").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE task_annotations ALTER COLUMN id SET NOT NULL").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE task_annotations DROP CONSTRAINT IF EXISTS task_annotations_pkey").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE task_annotations ADD CONSTRAINT task_annotations_pkey PRIMARY KEY (id)").Error; err != nil {
			return err
		}
		return tx.Exec("CREATE INDEX IF NOT EXISTS idx_task_annotations_task ON task_annotations(task_uuid)").Error
	})
}

func postgresRegclassFound(name sql.NullString) bool {
	return name.Valid && name.String != ""
}
