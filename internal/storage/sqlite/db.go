package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const localWorkspaceSlug = "local"
const localUserName = "local"
const m5ProjectsAppliedMetaKey = "migration.m5.projects.applied"
const m5ProjectsSkippedMetaKey = "migration.m5.projects.skipped"

var m5ProjectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Store struct {
	db *gorm.DB
}

type MigrationWarning struct {
	WorkspaceID string `json:"workspace_id"`
	TaskUUID    string `json:"task_uuid"`
	RawProject  string `json:"raw_project"`
	Reason      string `json:"reason"`
}

type m5SkippedProject struct {
	WorkspaceID string `json:"workspace_id"`
	TaskUUID    string `json:"task_uuid"`
	RawProject  string `json:"raw_project"`
	Reason      string `json:"reason"`
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(sqliteDSN(path)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.configure(); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.migrate(); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.ensureLocalIdentity(); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "foreign_keys(1)")
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + values.Encode()
}

func (s *Store) Close() error {
	sqlDB, err := s.sqlDB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) DB() *gorm.DB {
	return s.db
}

func (s *Store) Transaction(fn func(*Store) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

func (s *Store) LocalWorkspace() (Workspace, error) {
	var ws Workspace
	err := s.db.Where("slug = ?", localWorkspaceSlug).First(&ws).Error
	return ws, err
}

func (s *Store) GetMeta(key string) (string, bool, error) {
	var meta Meta
	err := s.db.Where("key = ?", key).First(&meta).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return meta.Value, true, nil
}

func (s *Store) SetMeta(key, value string) error {
	return s.db.Save(&Meta{Key: key, Value: value}).Error
}

func (s *Store) DeleteMeta(key string) error {
	return s.db.Delete(&Meta{Key: key}).Error
}

func (s *Store) ListMeta() (map[string]string, error) {
	var rows []Meta
	if err := s.db.Order("key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return values, nil
}

func (s *Store) M5ProjectMigrationReport() ([]MigrationWarning, error) {
	raw, ok, err := s.GetMeta(m5ProjectsSkippedMetaKey)
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var skipped []m5SkippedProject
	if err := json.Unmarshal([]byte(raw), &skipped); err != nil {
		return nil, err
	}
	out := make([]MigrationWarning, 0, len(skipped))
	for _, row := range skipped {
		out = append(out, MigrationWarning{
			WorkspaceID: row.WorkspaceID,
			TaskUUID:    row.TaskUUID,
			RawProject:  row.RawProject,
			Reason:      row.Reason,
		})
	}
	return out, nil
}

func (s *Store) configure() error {
	return s.db.Exec("PRAGMA foreign_keys = ON").Error
}

func (s *Store) migrate() error {
	if err := s.prepareWorkspaceSchemaForM4(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&Meta{}, &User{}, &Workspace{}, &Membership{}, &AuditLog{}, &Project{}, &Config{}, &Context{}, &UDADefinition{}); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&TaskTag{}, &TaskAnnotation{}, &TaskDependency{}, &TaskUDAValue{}); err != nil {
		return err
	}
	if err := s.prepareProjectSchemaForM5(); err != nil {
		return err
	}
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
	// BEGIN IMMEDIATE 在执行 tasks 表重建 DDL 前拿到写锁，避免两个进程交错执行 M5 迁移。
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
  uuid, workspace_id, description, status, entry, modified,
  end_ts, due, project, priority, start, wait, scheduled, until,
  recur, parent, mask, i_mask, project_id
)
SELECT
  t.uuid, t.workspace_id, t.description, t.status, t.entry, t.modified,
  t.end_ts, t.due,
  CASE
    WHEN p.id IS NOT NULL THEN p.slug
    WHEN t.project IS NOT NULL AND TRIM(t.project) != '' THEN t.project
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
		columns: []string{"task_uuid", "entry", "description"},
		ddl: `CREATE TABLE task_annotations (
	task_uuid TEXT NOT NULL,
	entry INTEGER NOT NULL,
	description TEXT NOT NULL,
	PRIMARY KEY (task_uuid, entry, description),
	CONSTRAINT fk_tasks_annotations FOREIGN KEY (task_uuid) REFERENCES tasks(uuid) ON DELETE CASCADE
)`,
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
	if err := tx.exec("INSERT INTO " + schema.table + "(" + columnList + ") SELECT " + columnList + " FROM " + oldTable); err != nil {
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

func (s *Store) ensureLocalIdentity() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()

		var user User
		err := tx.Where("name = ?", localUserName).First(&user).Error
		switch {
		case err == nil:
		case errors.Is(err, gorm.ErrRecordNotFound):
			user = User{
				ID:         uuid.NewString(),
				Name:       localUserName,
				CreatedAt:  now,
				ModifiedAt: now,
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		default:
			return err
		}

		var ws Workspace
		err = tx.Where("slug = ?", localWorkspaceSlug).First(&ws).Error
		switch {
		case err == nil:
			updates := map[string]any{}
			if ws.Visibility == "" {
				updates["visibility"] = "private"
			}
			if ws.SettingsJSON == "" {
				updates["settings_json"] = "{}"
			}
			if ws.Name == "" {
				updates["name"] = "Local"
			}
			if ws.CreatedByUserID == nil {
				updates["created_by_user_id"] = user.ID
			}
			if ws.ModifiedAt == 0 {
				updates["modified_at"] = ws.CreatedAt
			}
			if len(updates) > 0 {
				if err := tx.Model(&Workspace{}).Where("id = ?", ws.ID).Updates(updates).Error; err != nil {
					return err
				}
				if err := tx.Where("id = ?", ws.ID).First(&ws).Error; err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			ws = Workspace{
				ID:              uuid.NewString(),
				Slug:            localWorkspaceSlug,
				Name:            "Local",
				CreatedByUserID: &user.ID,
				Visibility:      "private",
				SettingsJSON:    "{}",
				CreatedAt:       now,
				ModifiedAt:      now,
			}
			if err := tx.Create(&ws).Error; err != nil {
				return err
			}
		default:
			return err
		}

		if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID != ws.ID {
			if err := tx.Model(&User{}).Where("id = ?", user.ID).Updates(map[string]any{
				"default_workspace_id": ws.ID,
				"modified_at":          now,
			}).Error; err != nil {
				return err
			}
		}

		member := Membership{
			UserID:      user.ID,
			WorkspaceID: ws.ID,
			Role:        "owner",
			JoinedAt:    now,
			ModifiedAt:  now,
		}
		if err := tx.Where("user_id = ? AND workspace_id = ?", user.ID, ws.ID).First(&Membership{}).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		var oldMeta Meta
		err = tx.Where("key = ?", "context.active").First(&oldMeta).Error
		switch {
		case err == nil:
			if err := tx.Save(&Meta{Key: "active_context." + user.ID + "." + ws.ID, Value: oldMeta.Value}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&Meta{Key: "context.active"}).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
		default:
			return err
		}
		return nil
	})
}

func (s *Store) sqlDB() (*sql.DB, error) {
	return s.db.DB()
}
