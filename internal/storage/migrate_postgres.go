package storage

import (
	"database/sql"
	"fmt"

	"gorm.io/gorm"
)

func (s *Store) migratePostgres() error {
	if err := s.prepareAPITokenUserIDNullablePostgres(); err != nil {
		return err
	}
	if err := s.prepareAutomationScopeSchemaPostgres(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(
		&Meta{}, &User{}, &Workspace{}, &Membership{},
		&AuditLog{}, &Project{}, &ProjectAnnotation{}, &Config{}, &ConfigDefinition{}, &ApiToken{}, &ServerAdminToken{}, &AdminActingSession{},
		&Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{},
		&NotificationSink{}, &ReminderRule{}, &EventNotificationRule{}, &NotificationDelivery{},
		&AutomationRule{}, &AutomationDelivery{},
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
	if err := s.prepareTaskAnnotationActivityColumnsPostgres(); err != nil {
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

// prepareProjectTemplateSchemaPostgres 使用 workspace-aware 复合外键：Snapshot
// 必须属于同一 Template 和来源 Project，Template 的 current_snapshot_id 也只能
// 指向自己的 Snapshot。旧版单列外键会在此处被替换，因而既有 PostgreSQL 数据库
// 不会只停留在 CREATE TABLE IF NOT EXISTS 的旧约束上。
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
archived_at bigint,
CONSTRAINT uq_project_templates_id_workspace UNIQUE (id, workspace_id)
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
CONSTRAINT uq_project_template_snapshots_id_template_workspace UNIQUE (id, template_id, workspace_id)
)`,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_templates_ws_key ON project_templates(workspace_id, key)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_templates_id_workspace ON project_templates(id, workspace_id)",
		"CREATE INDEX IF NOT EXISTS idx_project_templates_ws_status ON project_templates(workspace_id, status)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_id_template_workspace ON project_template_snapshots(id, template_id, workspace_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_version ON project_template_snapshots(template_id, version)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_project_template_snapshots_template_snapshot_hash ON project_template_snapshots(template_id, snapshot_hash)",
		"CREATE INDEX IF NOT EXISTS idx_project_template_snapshots_ws_template ON project_template_snapshots(workspace_id, template_id)",
		"ALTER TABLE project_template_snapshots DROP CONSTRAINT IF EXISTS fk_project_template_snapshots_template",
		"ALTER TABLE project_template_snapshots DROP CONSTRAINT IF EXISTS fk_project_template_snapshots_source_project",
		"ALTER TABLE project_templates DROP CONSTRAINT IF EXISTS fk_project_templates_current_snapshot",
		`ALTER TABLE project_template_snapshots ADD CONSTRAINT fk_project_template_snapshots_template
FOREIGN KEY (template_id, workspace_id) REFERENCES project_templates(id, workspace_id) ON DELETE RESTRICT`,
		`ALTER TABLE project_template_snapshots ADD CONSTRAINT fk_project_template_snapshots_source_project
FOREIGN KEY (source_project_id, workspace_id) REFERENCES projects(id, workspace_id) ON DELETE RESTRICT`,
		`ALTER TABLE project_templates ADD CONSTRAINT fk_project_templates_current_snapshot
FOREIGN KEY (current_snapshot_id, id, workspace_id) REFERENCES project_template_snapshots(id, template_id, workspace_id) ON DELETE RESTRICT`,
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
		"ALTER TABLE automation_rules ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
		"ALTER TABLE automation_rules ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE automation_rules ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE automation_rules ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE automation_rules ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
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

func (s *Store) prepareTaskAnnotationActivityColumnsPostgres() error {
	var tableName sql.NullString
	if err := s.db.Raw("SELECT to_regclass('task_annotations')::text").Scan(&tableName).Error; err != nil {
		return err
	}
	if !postgresRegclassFound(tableName) {
		return nil
	}
	statements := []string{
		"ALTER TABLE task_annotations ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'unknown'",
		"ALTER TABLE task_annotations ADD COLUMN IF NOT EXISTS created_by_user_id text",
		"ALTER TABLE task_annotations ADD COLUMN IF NOT EXISTS created_by_token_id text",
		"ALTER TABLE task_annotations ADD COLUMN IF NOT EXISTS created_by_token_name text",
		"ALTER TABLE task_annotations ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
		"ALTER TABLE task_annotations ADD COLUMN IF NOT EXISTS created_at bigint NOT NULL DEFAULT 0",
		"UPDATE task_annotations SET created_by_actor_type = 'unknown' WHERE created_by_actor_type IS NULL OR created_by_actor_type = ''",
		"UPDATE task_annotations SET created_at = entry WHERE created_at = 0",
		"CREATE INDEX IF NOT EXISTS idx_task_annotations_activity ON task_annotations(task_uuid, created_at DESC, id DESC)",
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func postgresRegclassFound(name sql.NullString) bool {
	return name.Valid && name.String != ""
}

// prepareAutomationScopeSchemaPostgres 把 legacy project_automation_* 重命名为
// automation_*，回填 scope_type/scope_id，并把 delivery.project_id 改为可空。
// fresh DB 没有 legacy 表时直接返回；所有变更在单个事务内完成。
func (s *Store) prepareAutomationScopeSchemaPostgres() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 检测 legacy 表是否仍然存在（旧 binary 创建过）。
		var legacyRulesCount int64
		if err := tx.Raw(`SELECT count(*)::bigint FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'project_automation_rules'`).Scan(&legacyRulesCount).Error; err != nil {
			return err
		}
		var legacyDeliveriesCount int64
		if err := tx.Raw(`SELECT count(*)::bigint FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'project_automation_deliveries'`).Scan(&legacyDeliveriesCount).Error; err != nil {
			return err
		}
		var newRulesCount int64
		if err := tx.Raw(`SELECT count(*)::bigint FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'automation_rules'`).Scan(&newRulesCount).Error; err != nil {
			return err
		}
		var newDeliveriesCount int64
		if err := tx.Raw(`SELECT count(*)::bigint FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'automation_deliveries'`).Scan(&newDeliveriesCount).Error; err != nil {
			return err
		}

		legacyRules := legacyRulesCount > 0
		legacyDeliveries := legacyDeliveriesCount > 0
		newRules := newRulesCount > 0
		newDeliveries := newDeliveriesCount > 0
		switch {
		case !legacyRules && !legacyDeliveries:
			// fresh DB：交给 AutoMigrate 创建。
			return nil
		case legacyRules && legacyDeliveries && !newRules && !newDeliveries:
			// 继续 legacy -> 新表迁移。
		case newRules && newDeliveries:
			// 新表已存在（测试夹具或多次启动），不做迁移。
			return nil
		default:
			return fmt.Errorf("automation scope migration (postgres): inconsistent legacy/new tables: legacy_rules=%v legacy_deliveries=%v new_rules=%v new_deliveries=%v",
				legacyRules, legacyDeliveries, newRules, newDeliveries)
		}

		// 1. 创建新表。
		stmts := []string{
			`CREATE TABLE automation_rules (
	id text PRIMARY KEY,
	workspace_id text NOT NULL,
	scope_type text NOT NULL DEFAULT 'project',
	scope_id text NOT NULL,
	name text NOT NULL,
	description text NOT NULL DEFAULT '',
	enabled boolean NOT NULL DEFAULT TRUE,
	trigger_type text NOT NULL,
	trigger_config_json text NOT NULL DEFAULT '{}',
	condition_json text NOT NULL DEFAULT '{}',
	action_type text NOT NULL DEFAULT 'openai_compatible',
	action_config_json text NOT NULL DEFAULT '{}',
	context_config_json text NOT NULL DEFAULT '{}',
	instruction_template text NOT NULL DEFAULT '',
	system_prompt text NOT NULL DEFAULT '',
	created_by_actor_type text NOT NULL DEFAULT 'user',
	created_by_user_id text,
	created_by_token_id text,
	created_by_token_name text,
	created_by_token_prefix text,
	created_at bigint NOT NULL,
	modified_at bigint NOT NULL
)`,
			`CREATE TABLE automation_deliveries (
	id text PRIMARY KEY,
	workspace_id text NOT NULL,
	rule_scope_type text NOT NULL DEFAULT 'project',
	rule_scope_id text NOT NULL,
	project_id text,
	rule_id text NOT NULL,
	trigger_type text NOT NULL,
	event_id text NOT NULL DEFAULT '',
	event_type text NOT NULL DEFAULT '',
	dedupe_key text NOT NULL,
	replay_of_delivery_id text,
	api_key_config_key text NOT NULL DEFAULT '',
	allowed_hosts_config_key text NOT NULL DEFAULT '',
	max_attempts bigint NOT NULL DEFAULT 0,
	status text NOT NULL,
	resolved_url text NOT NULL DEFAULT '',
	rendered_method text NOT NULL DEFAULT 'POST',
	rendered_headers_json text NOT NULL DEFAULT '{}',
	request_body_json text NOT NULL DEFAULT '',
	request_body_preview text NOT NULL DEFAULT '',
	request_body_hash text NOT NULL DEFAULT '',
	response_status_code integer,
	response_body_preview text NOT NULL DEFAULT '',
	provider_request_id text NOT NULL DEFAULT '',
	usage_json text NOT NULL DEFAULT '{}',
	attempt_count bigint NOT NULL DEFAULT 0,
	next_attempt_at bigint,
	claim_expires_at bigint,
	last_attempt_at bigint,
	last_error text NOT NULL DEFAULT '',
	created_at bigint NOT NULL,
	modified_at bigint NOT NULL
)`,
		}
		for _, stmt := range stmts {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("automation scope migration (postgres): create schema: %w", err)
			}
		}
		// 2. 复制规则并回填 scope。
		if err := tx.Exec(`INSERT INTO automation_rules (
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
	COALESCE(created_by_actor_type, 'user'), created_by_user_id, created_by_token_id, created_by_token_name, created_by_token_prefix,
	created_at, modified_at
FROM project_automation_rules`).Error; err != nil {
			return fmt.Errorf("automation scope migration (postgres): copy rules: %w", err)
		}
		// 3. 复制 Delivery，回填冻结 scope；project_id 保持非空字符串以保留历史。
		if err := tx.Exec(`INSERT INTO automation_deliveries (
	id, workspace_id, rule_scope_type, rule_scope_id, project_id, rule_id, trigger_type, event_id, event_type,
	dedupe_key, replay_of_delivery_id, api_key_config_key, allowed_hosts_config_key, max_attempts,
	status, resolved_url, rendered_method, rendered_headers_json,
	request_body_json, request_body_preview, request_body_hash,
	response_status_code, response_body_preview, provider_request_id, usage_json,
	attempt_count, next_attempt_at, claim_expires_at, last_attempt_at, last_error,
	created_at, modified_at
)
SELECT
	id, workspace_id, 'project', project_id, project_id, rule_id, trigger_type, event_id, event_type,
	dedupe_key, NULL, '', '', 0,
	status, resolved_url, rendered_method, rendered_headers_json,
	request_body_json, request_body_preview, request_body_hash,
	response_status_code, response_body_preview, provider_request_id, usage_json,
	attempt_count, next_attempt_at, claim_expires_at, last_attempt_at, last_error,
	created_at, modified_at
FROM project_automation_deliveries`).Error; err != nil {
			return fmt.Errorf("automation scope migration (postgres): copy deliveries: %w", err)
		}
		// 4. 校验主键集合一致。
		var ruleDiff int64
		if err := tx.Raw(`SELECT count(*)::bigint FROM (
	SELECT id FROM project_automation_rules EXCEPT SELECT id FROM automation_rules
	UNION
	SELECT id FROM automation_rules EXCEPT SELECT id FROM project_automation_rules
)`).Scan(&ruleDiff).Error; err != nil {
			return err
		}
		if ruleDiff != 0 {
			return fmt.Errorf("automation scope migration (postgres): rule primary key set changed, diff=%d", ruleDiff)
		}
		var deliveryDiff int64
		if err := tx.Raw(`SELECT count(*)::bigint FROM (
	SELECT id FROM project_automation_deliveries EXCEPT SELECT id FROM automation_deliveries
	UNION
	SELECT id FROM automation_deliveries EXCEPT SELECT id FROM project_automation_deliveries
)`).Scan(&deliveryDiff).Error; err != nil {
			return err
		}
		if deliveryDiff != 0 {
			return fmt.Errorf("automation scope migration (postgres): delivery primary key set changed, diff=%d", deliveryDiff)
		}
		// 5. 删除 legacy 表。
		if err := tx.Exec("DROP TABLE project_automation_deliveries").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE project_automation_rules").Error; err != nil {
			return err
		}
		return nil
	})
}
