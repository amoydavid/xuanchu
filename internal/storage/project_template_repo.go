package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrProjectTemplateKeyConflict         = errors.New("project template key conflict")
	ErrProjectTemplateVersionConflict     = errors.New("project template snapshot version conflict")
	ErrProjectTemplateHashConflict        = errors.New("project template snapshot hash conflict")
	ErrProjectTemplateTransactionRequired = errors.New("project template operation requires transaction")
)

type ProjectTemplateListOptions struct {
	WorkspaceID string
	Status      string
	Q           string
	Limit       int
	Offset      int
}

type ProjectTemplatePage struct {
	Items  []ProjectTemplate
	Total  int64
	Limit  int
	Offset int
}

// ProjectTemplateRepository 只保存 Template 元数据与不透明的 Snapshot JSON。
type ProjectTemplateRepository struct{ db *gorm.DB }

func NewProjectTemplateRepository(db *gorm.DB) *ProjectTemplateRepository {
	return &ProjectTemplateRepository{db: db}
}

func (r *ProjectTemplateRepository) Create(row ProjectTemplate) error {
	if err := r.db.Create(&row).Error; err != nil {
		if isProjectTemplateUniqueError(err) {
			return ErrProjectTemplateKeyConflict
		}
		return err
	}
	return nil
}

func (r *ProjectTemplateRepository) GetByRef(workspaceID, ref string) (ProjectTemplate, error) {
	var row ProjectTemplate
	err := r.db.Where("workspace_id = ? AND (id = ? OR key = ?)", workspaceID, ref, ref).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectTemplate{}, ErrNotFound
	}
	return row, err
}

// LockByRef 在当前事务内锁住 Template，并返回锁后的最新状态。
// PostgreSQL 使用行级 FOR UPDATE；SQLite 通过无语义变更的 UPDATE 先取得
// writer lock，再读取 status/current_snapshot_id，避免读后再写之间的 TOCTOU。
func (r *ProjectTemplateRepository) LockByRef(workspaceID, ref string) (ProjectTemplate, error) {
	if !gormDBInTransaction(r.db) {
		return ProjectTemplate{}, ErrProjectTemplateTransactionRequired
	}
	if r.db.Dialector.Name() == "sqlite" {
		if err := r.acquireSQLiteTemplateWriterLock(workspaceID, ref); err != nil {
			return ProjectTemplate{}, err
		}
	}
	var row ProjectTemplate
	query := r.db.Where("workspace_id = ? AND (id = ? OR key = ?)", workspaceID, ref, ref)
	if r.db.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectTemplate{}, ErrNotFound
	} else if err != nil {
		return ProjectTemplate{}, err
	}
	return row, nil
}

func (r *ProjectTemplateRepository) acquireSQLiteTemplateWriterLock(workspaceID, ref string) error {
	const attempts = 200
	for attempt := 0; attempt < attempts; attempt++ {
		result := r.db.Model(&ProjectTemplate{}).
			Where("workspace_id = ? AND (id = ? OR key = ?)", workspaceID, ref, ref).
			UpdateColumn("modified_at", gorm.Expr("modified_at"))
		if result.Error == nil {
			if result.RowsAffected == 0 {
				return ErrNotFound
			}
			return nil
		}
		message := strings.ToLower(result.Error.Error())
		if !strings.Contains(message, "database is locked") &&
			!strings.Contains(message, "database table is locked") &&
			!strings.Contains(message, "sqlite_busy") &&
			!strings.Contains(message, "sqlite_locked") {
			return result.Error
		}
		if attempt+1 == attempts {
			return result.Error
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("unreachable SQLite project template writer lock retry")
}

func (r *ProjectTemplateRepository) List(options ProjectTemplateListOptions) (ProjectTemplatePage, error) {
	query := r.db.Model(&ProjectTemplate{}).Where("workspace_id = ?", options.WorkspaceID)
	if options.Status != "" {
		query = query.Where("status = ?", options.Status)
	}
	if q := strings.TrimSpace(options.Q); q != "" {
		query = query.Where("LOWER(key) LIKE ? OR LOWER(name) LIKE ?", "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%")
	}
	var page ProjectTemplatePage
	if err := query.Count(&page.Total).Error; err != nil {
		return ProjectTemplatePage{}, err
	}
	page.Limit, page.Offset = options.Limit, options.Offset
	if page.Offset < 0 {
		page.Offset = 0
	}
	if page.Limit > 0 {
		query = query.Limit(page.Limit)
	}
	if page.Offset > 0 {
		query = query.Offset(page.Offset)
	}
	if err := query.Order("modified_at DESC").Order("id ASC").Find(&page.Items).Error; err != nil {
		return ProjectTemplatePage{}, err
	}
	return page, nil
}

func (r *ProjectTemplateRepository) UpdateMetadata(workspaceID, id, name, description string, modifiedAt int64) error {
	result := r.db.Model(&ProjectTemplate{}).Where("workspace_id = ? AND id = ?", workspaceID, id).Updates(map[string]any{
		"name": name, "description": description, "modified_at": modifiedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectTemplateRepository) SetStatus(workspaceID, id, status string, archivedAt *int64, modifiedAt int64) error {
	result := r.db.Model(&ProjectTemplate{}).Where("workspace_id = ? AND id = ?", workspaceID, id).Updates(map[string]any{
		"status": status, "archived_at": archivedAt, "modified_at": modifiedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AppendSnapshotLocked 只可在 Store.Transaction 内调用。PostgreSQL 用 FOR UPDATE
// 锁定 Template 行；SQLite 先执行无语义变更的写入，取得外层事务的 writer lock，
// 再读取版本，确保并发追加不会分配同一 version。
func (r *ProjectTemplateRepository) AppendSnapshotLocked(workspaceID, templateID string, row ProjectTemplateSnapshot) (ProjectTemplateSnapshot, error) {
	if !gormDBInTransaction(r.db) {
		return ProjectTemplateSnapshot{}, ErrProjectTemplateTransactionRequired
	}
	if _, err := r.LockByRef(workspaceID, templateID); err != nil {
		return ProjectTemplateSnapshot{}, err
	}

	var latest sql.NullInt64
	if err := r.db.Model(&ProjectTemplateSnapshot{}).Where("workspace_id = ? AND template_id = ?", workspaceID, templateID).
		Select("MAX(version)").Scan(&latest).Error; err != nil {
		return ProjectTemplateSnapshot{}, err
	}
	row.WorkspaceID = workspaceID
	row.TemplateID = templateID
	row.Version = 1
	if latest.Valid {
		row.Version = latest.Int64 + 1
	}
	if err := r.db.Create(&row).Error; err != nil {
		return ProjectTemplateSnapshot{}, mapProjectTemplateSnapshotConflict(err)
	}
	result := r.db.Model(&ProjectTemplate{}).Where("workspace_id = ? AND id = ?", workspaceID, templateID).Updates(map[string]any{
		"current_snapshot_id": row.ID,
		"modified_at":         row.CreatedAt,
	})
	if result.Error != nil {
		return ProjectTemplateSnapshot{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ProjectTemplateSnapshot{}, ErrNotFound
	}
	return row, nil
}

func (r *ProjectTemplateRepository) GetSnapshot(workspaceID, templateID, snapshotID string) (ProjectTemplateSnapshot, error) {
	var row ProjectTemplateSnapshot
	err := r.db.Where("workspace_id = ? AND template_id = ? AND id = ?", workspaceID, templateID, snapshotID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectTemplateSnapshot{}, ErrNotFound
	}
	return row, err
}

// GetSnapshotLocked 在 Template 已锁住的事务内锁定目标不可变 Snapshot。
// PostgreSQL 的 SHARE lock 与未来维护/删除路径互斥；SQLite 已由 Template
// writer lock 串行化，不需要额外语句。
func (r *ProjectTemplateRepository) GetSnapshotLocked(workspaceID, templateID, snapshotID string) (ProjectTemplateSnapshot, error) {
	if !gormDBInTransaction(r.db) {
		return ProjectTemplateSnapshot{}, ErrProjectTemplateTransactionRequired
	}
	var row ProjectTemplateSnapshot
	query := r.db.Where("workspace_id = ? AND template_id = ? AND id = ?", workspaceID, templateID, snapshotID)
	if r.db.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	err := query.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectTemplateSnapshot{}, ErrNotFound
	}
	return row, err
}

func (r *ProjectTemplateRepository) ListSnapshots(workspaceID, templateID string) ([]ProjectTemplateSnapshot, error) {
	var rows []ProjectTemplateSnapshot
	err := r.db.Where("workspace_id = ? AND template_id = ?", workspaceID, templateID).Order("version DESC").Find(&rows).Error
	return rows, err
}

func gormDBInTransaction(db *gorm.DB) bool {
	_, ok := db.Statement.ConnPool.(*sql.Tx)
	return ok
}

func isProjectTemplateUniqueError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	text := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(text, "unique constraint") || strings.Contains(text, "duplicate key") || strings.Contains(text, "unique violation")
}

func mapProjectTemplateSnapshotConflict(err error) error {
	if !isProjectTemplateUniqueError(err) {
		return err
	}
	text := strings.ToLower(fmt.Sprint(err))
	switch {
	case strings.Contains(text, "snapshot_hash") || strings.Contains(text, "template_snapshot_hash"):
		return ErrProjectTemplateHashConflict
	case strings.Contains(text, "version") || strings.Contains(text, "template_version"):
		return ErrProjectTemplateVersionConflict
	default:
		return ErrProjectTemplateVersionConflict
	}
}
