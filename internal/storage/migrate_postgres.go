package storage

import (
	"database/sql"

	"gorm.io/gorm"
)

func (s *Store) migratePostgres() error {
	if err := s.db.AutoMigrate(
		&Meta{}, &User{}, &Workspace{}, &Membership{},
		&AuditLog{}, &Project{}, &ProjectAnnotation{}, &Config{}, &ConfigDefinition{}, &ApiToken{}, &ServerAdminToken{}, &AdminActingSession{},
		&Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{},
		&NotificationSink{}, &ReminderRule{}, &EventNotificationRule{}, &NotificationDelivery{},
		&UserExternalID{}, &Task{},
	); err != nil {
		return err
	}
	if err := s.prepareTaskAnnotationIDsPostgres(); err != nil {
		return err
	}
	return s.db.AutoMigrate(
		&TaskTag{}, &TaskAnnotation{}, &TaskDependency{},
		&TaskAssignee{}, &TaskUDAValue{}, &TaskLink{},
	)
}

func (s *Store) prepareTaskAnnotationIDsPostgres() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var tableName sql.NullString
		err := tx.Raw("SELECT to_regclass('public.task_annotations')::text").Scan(&tableName).Error
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
