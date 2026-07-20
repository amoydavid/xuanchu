package storage

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// ProjectAutomationRuleRepository 负责项目自动化规则的持久化。
type ProjectAutomationRuleRepository struct{ db *gorm.DB }

type ProjectAutomationCandidateListOptions struct {
	WorkspaceID string
	ProjectID   string
	Q           string
	Enabled     string // enabled|disabled|all
	TriggerType string // schedule|event|all
}

type ProjectAutomationCandidatePage struct {
	Items  []ProjectAutomationRule
	Total  int64
	Limit  int
	Offset int
}

// NewProjectAutomationRuleRepository 基于已有 *gorm.DB 构建规则仓储。
func NewProjectAutomationRuleRepository(db *gorm.DB) *ProjectAutomationRuleRepository {
	return &ProjectAutomationRuleRepository{db: db}
}

// Create 写入一条规则。
func (r *ProjectAutomationRuleRepository) Create(row ProjectAutomationRule) error {
	return r.db.Create(&row).Error
}

// GetByID 按 ID 读取规则，未找到时返回 ErrNotFound。
func (r *ProjectAutomationRuleRepository) GetByID(id string) (ProjectAutomationRule, error) {
	var row ProjectAutomationRule
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectAutomationRule{}, ErrNotFound
	}
	return row, err
}

// List 按 workspace/project 查询规则，includeDisabled 控制是否包含停用规则。
func (r *ProjectAutomationRuleRepository) List(workspaceID string, projectID *string, includeDisabled bool) ([]ProjectAutomationRule, error) {
	query := r.db.Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		query = query.Where("project_id = ?", *projectID)
	}
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []ProjectAutomationRule
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

// ListCandidatePage 在数据库内完成项目自动化候选的过滤、计数和分页。
func (r *ProjectAutomationRuleRepository) ListCandidatePage(opts ProjectAutomationCandidateListOptions, limit, offset int) (ProjectAutomationCandidatePage, error) {
	base := r.db.Model(&ProjectAutomationRule{}).Where("workspace_id = ? AND project_id = ?", opts.WorkspaceID, opts.ProjectID)
	if q := strings.TrimSpace(opts.Q); q != "" {
		like := "%" + q + "%"
		base = base.Where("(LOWER(name) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?))", like, like)
	}
	switch opts.Enabled {
	case "enabled":
		base = base.Where("enabled = ?", true)
	case "disabled":
		base = base.Where("enabled = ?", false)
	}
	if opts.TriggerType != "" && opts.TriggerType != "all" {
		base = base.Where("trigger_type = ?", opts.TriggerType)
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return ProjectAutomationCandidatePage{}, err
	}
	var rows []ProjectAutomationRule
	if err := base.Session(&gorm.Session{}).Order("created_at ASC").Order("id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return ProjectAutomationCandidatePage{}, err
	}
	return ProjectAutomationCandidatePage{Items: rows, Total: total, Limit: limit, Offset: offset}, nil
}

// ListEnabled 返回所有启用规则，调度器扫描时使用。
func (r *ProjectAutomationRuleRepository) ListEnabled() ([]ProjectAutomationRule, error) {
	var rows []ProjectAutomationRule
	err := r.db.Where("enabled = ?", true).Order("created_at ASC").Find(&rows).Error
	return rows, err
}

// Update 覆盖保存规则。
func (r *ProjectAutomationRuleRepository) Update(row ProjectAutomationRule) error {
	return r.db.Save(&row).Error
}

// Delete 按 ID 删除规则。
func (r *ProjectAutomationRuleRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&ProjectAutomationRule{}).Error
}
