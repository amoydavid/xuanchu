package storage

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// prepareTaskSeriesSchema 把 tasks 表切换到 occurrence 模型，并建立 task_series 聚合（spec §7、§20.2）。
//
// 这是破坏性 schema 变更：检测到旧 status=recurring 数据时直接报错，不静默迁移（spec §20.2）。
// 在开发环境重建数据库即可，不提供生产兼容迁移。
//
// 注意：tasks 表由 M5 流程用原始 SQL 建立，这里不使用 GORM AutoMigrate(&Task{})——
// GORM 在 SQLite 上对差异较大的表会 rebuild，可能丢失 NOT NULL 约束。改为显式 ADD COLUMN。
func (s *Store) prepareTaskSeriesSchema() error {
	// 1. AutoMigrate Series 相关表（全新表，无 rebuild 风险）。
	if err := s.db.AutoMigrate(&TaskSeries{}, &TaskSeriesRuleVersion{}, &TaskSeriesAssignee{}, &TaskSeriesTag{}, &TaskSeriesUDAValue{}); err != nil {
		return fmt.Errorf("task series: AutoMigrate series: %w", err)
	}
	// 2. 检测旧循环任务数据。若存在则拒绝启动，提示重建开发数据库（spec §20.2）。
	if err := detectLegacyRecurringData(s.db); err != nil {
		return err
	}
	// 3. 删除旧 recur 索引和旧 parent+due 唯一索引（旧循环去重路径已移除）。
	if err := dropLegacyRecurringIndexes(s.db); err != nil {
		return fmt.Errorf("task series: drop legacy indexes: %w", err)
	}
	// 4. 给 tasks 表显式加 occurrence 列（避免 GORM rebuild）。
	if err := addOccurrenceColumns(s.db); err != nil {
		return fmt.Errorf("task series: add occurrence columns: %w", err)
	}
	// 5. 删除旧 recur/mask/i_mask 列（SQLite/PostgreSQL 都支持 DROP COLUMN）。
	if err := dropLegacyRecurringColumns(s.db); err != nil {
		return fmt.Errorf("task series: drop legacy columns: %w", err)
	}
	// 6. 建立 occurrence partial unique index（spec §7.3）。
	if err := createOccurrenceUniqueIndex(s.db); err != nil {
		return fmt.Errorf("task series: create occurrence unique index: %w", err)
	}
	// 7. 建立 due/recurrence_at 查询索引（model 的 GORM index tag 在显式 ADD COLUMN 场景不会自动生效）。
	if err := ensureOccurrenceLookupIndexes(s.db); err != nil {
		return fmt.Errorf("task series: ensure lookup indexes: %w", err)
	}
	return nil
}

// ensureOccurrenceLookupIndexes 建立 due 和 recurrence_at 的查询索引。
func ensureOccurrenceLookupIndexes(db *gorm.DB) error {
	for _, stmt := range []string{
		"CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(due)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_recurrence_at ON tasks(recurrence_at)",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

// addOccurrenceColumns 给 tasks 表加 series_id/recurrence_at/recurrence_rule_snapshot/
// recurrence_overrides_json 列。使用显式 SQL，避免 GORM AutoMigrate 的 rebuild 行为。
func addOccurrenceColumns(db *gorm.DB) error {
	dialect := db.Dialector.Name()
	columns := []struct {
		name string
		// sqlite 和 postgres 的 DDL 片段
		sqlite  string
		postgres string
	}{
		{"series_id", "TEXT", "text"},
		{"recurrence_at", "INTEGER", "bigint"},
		{"recurrence_rule_snapshot", "TEXT", "text"},
		{"recurrence_overrides_json", "TEXT NOT NULL DEFAULT '[]'", "text NOT NULL DEFAULT '[]'"},
	}
	for _, col := range columns {
		exists, err := taskSeriesColumnExists(db, dialect, "tasks", col.name)
		if err != nil {
			return fmt.Errorf("check column %s: %w", col.name, err)
		}
		if exists {
			continue
		}
		ddl := col.sqlite
		if dialect == "postgres" {
			ddl = col.postgres
		}
		if err := db.Exec(fmt.Sprintf("ALTER TABLE tasks ADD COLUMN %s %s", col.name, ddl)).Error; err != nil {
			return fmt.Errorf("add column %s: %w", col.name, err)
		}
	}
	return nil
}

// detectLegacyRecurringData 检查 tasks 表是否存在旧 status=recurring 数据。
// 列不存在或无数据时跳过；存在则返回明确错误。
func detectLegacyRecurringData(db *gorm.DB) error {
	if !db.Migrator().HasTable("tasks") {
		return nil
	}
	// 用原生 SQL 检查 status=recurring，避免依赖 model 字段（model 已不含旧字段）。
	// 若查询失败（如 status 列不存在），视为无旧数据。
	var count int64
	if err := db.Raw("SELECT count(*) FROM tasks WHERE status = 'recurring'").Scan(&count).Error; err != nil {
		return nil
	}
	if count > 0 {
		return errors.New("检测到旧循环任务数据（status=recurring），不支持自动迁移，请备份后重建开发数据库")
	}
	return nil
}

// dropLegacyRecurringIndexes 删除旧循环路径的索引。
func dropLegacyRecurringIndexes(db *gorm.DB) error {
	for _, idx := range []string{"idx_tasks_recur", "idx_task_parent_due_open"} {
		// 忽略不存在的索引。
		_ = db.Migrator().DropIndex("tasks", idx)
	}
	return nil
}

// dropLegacyRecurringColumns 删除旧 recur/mask/i_mask 列。
// 用原生 SQL 检测列是否存在，避免依赖 model 字段（model 已不含旧字段）。
func dropLegacyRecurringColumns(db *gorm.DB) error {
	dialect := db.Dialector.Name()
	for _, col := range []string{"recur", "mask", "i_mask"} {
		exists, err := taskSeriesColumnExists(db, dialect, "tasks", col)
		if err != nil {
			return fmt.Errorf("check column %s: %w", col, err)
		}
		if !exists {
			continue
		}
		if err := db.Exec(fmt.Sprintf("ALTER TABLE tasks DROP COLUMN %s", col)).Error; err != nil {
			return fmt.Errorf("drop column %s: %w", col, err)
		}
	}
	return nil
}

// taskSeriesColumnExists 用 dialect 无关方式检测列是否存在。
func taskSeriesColumnExists(db *gorm.DB, dialect, table, column string) (bool, error) {
	switch dialect {
	case "postgres":
		var n int64
		err := db.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, column).Scan(&n).Error
		return n > 0, err
	default: // sqlite
		rows, err := db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", table)).Rows()
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt interface{}
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				return false, err
			}
			if name == column {
				return true, nil
			}
		}
		return false, nil
	}
}

// createOccurrenceUniqueIndex 建立 partial unique index（spec §7.3）。
// SQLite 和 PostgreSQL 都支持 partial index 语法。
func createOccurrenceUniqueIndex(db *gorm.DB) error {
	idxSQL := "CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_ws_series_slot ON tasks(workspace_id, series_id, recurrence_at) WHERE series_id IS NOT NULL AND recurrence_at IS NOT NULL"
	return db.Exec(idxSQL).Error
}
