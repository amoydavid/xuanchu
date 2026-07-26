package storage

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// Automation scope 常量。scope_id 在 workspace scope 下与 workspace_id 相同，
// 在 project scope 下为 project_id。
const (
	AutomationScopeWorkspace = "workspace"
	AutomationScopeProject   = "project"
)

// AutomationRuleRepository 是 AutomationRule 的统一仓储，覆盖 workspace/project
// 两种 scope 的查询、候选分页与调度扫描。
type AutomationRuleRepository struct{ db *gorm.DB }

// AutomationRuleListOptions 描述按 scope 查询规则的条件。
type AutomationRuleListOptions struct {
	WorkspaceID string
	ScopeType   string
	ScopeID     string
	Enabled     string // enabled|disabled|all，空等价于 enabled
	TriggerType string // schedule|event|空表示不过滤
}

// AutomationCandidateListOptions 描述模板候选分页查询条件，永远只查 project scope。
type AutomationCandidateListOptions struct {
	WorkspaceID string
	ProjectID   string
	Refs        []string
	Q           string
	Enabled     string // enabled|disabled|all
	TriggerType string // schedule|event|all
}

// AutomationCandidatePage 是模板候选分页结果。
type AutomationCandidatePage struct {
	Items  []AutomationRule
	Total  int64
	Limit  int
	Offset int
}

// NewAutomationRuleRepository 基于已有 *gorm.DB 构建规则仓储。
func NewAutomationRuleRepository(db *gorm.DB) *AutomationRuleRepository {
	return &AutomationRuleRepository{db: db}
}

// Create 写入一条规则。
func (r *AutomationRuleRepository) Create(row AutomationRule) error {
	return r.db.Create(&row).Error
}

// GetByID 按 ID 读取规则，未找到时返回 ErrNotFound。
func (r *AutomationRuleRepository) GetByID(id string) (AutomationRule, error) {
	var row AutomationRule
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AutomationRule{}, ErrNotFound
	}
	return row, err
}

// ListByIDs 批量读取当前 workspace 内的自动化规则。
// Project 归属由 App 层结合 Capture source project 再次校验。
func (r *AutomationRuleRepository) ListByIDs(workspaceID string, ids []string) ([]AutomationRule, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []AutomationRule
	err := r.db.Where("workspace_id = ? AND id IN ?", workspaceID, ids).
		Order("created_at ASC").Order("id ASC").Find(&rows).Error
	return rows, err
}

// List 按 scope 查询规则。
//
// 兼容旧签名：当 scopeType 为空、scopeID 非空时按 project 兼容路径查询，便于
// Task 1 期间 App 层未切换时直接复用。
func (r *AutomationRuleRepository) List(opts AutomationRuleListOptions) ([]AutomationRule, error) {
	if opts.WorkspaceID == "" {
		return nil, nil
	}
	query := r.db.Where("workspace_id = ?", opts.WorkspaceID)
	if opts.ScopeType != "" {
		query = query.Where("scope_type = ?", opts.ScopeType)
	}
	if opts.ScopeID != "" {
		query = query.Where("scope_id = ?", opts.ScopeID)
	}
	switch opts.Enabled {
	case "":
		query = query.Where("enabled = ?", true)
	case "enabled":
		query = query.Where("enabled = ?", true)
	case "disabled":
		query = query.Where("enabled = ?", false)
	case "all":
		// no-op
	}
	if opts.TriggerType != "" {
		query = query.Where("trigger_type = ?", opts.TriggerType)
	}
	var rows []AutomationRule
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

// ListScope 按 workspace/scope 联合过滤规则，includeDisabled 控制是否包含停用规则。
// 返回顺序按 created_at ASC，便于运行记录与列表稳定排序。
func (r *AutomationRuleRepository) ListScope(workspaceID, scopeType, scopeID string, includeDisabled bool) ([]AutomationRule, error) {
	if workspaceID == "" || scopeType == "" || scopeID == "" {
		return nil, nil
	}
	query := r.db.Where("workspace_id = ? AND scope_type = ? AND scope_id = ?", workspaceID, scopeType, scopeID)
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []AutomationRule
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

// ListCandidatePage 在数据库内完成项目自动化候选（Project scope）的过滤、计数和分页。
// Workspace scope 规则永远不进入候选。
func (r *AutomationRuleRepository) ListCandidatePage(opts AutomationCandidateListOptions, limit, offset int) (AutomationCandidatePage, error) {
	base := r.db.Model(&AutomationRule{}).Where(
		"workspace_id = ? AND scope_type = ? AND scope_id = ?",
		opts.WorkspaceID, AutomationScopeProject, opts.ProjectID,
	)
	if len(opts.Refs) > 0 {
		base = base.Where("id IN ?", opts.Refs)
	}
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
		return AutomationCandidatePage{}, err
	}
	var rows []AutomationRule
	if err := base.Session(&gorm.Session{}).Order("created_at ASC").Order("id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return AutomationCandidatePage{}, err
	}
	return AutomationCandidatePage{Items: rows, Total: total, Limit: limit, Offset: offset}, nil
}

// ListEnabled 返回所有启用规则，调度器扫描时使用。
func (r *AutomationRuleRepository) ListEnabled() ([]AutomationRule, error) {
	var rows []AutomationRule
	err := r.db.Where("enabled = ?", true).Order("created_at ASC").Find(&rows).Error
	return rows, err
}

// ListEnabledScanned 返回所有 enabled 的 schedule 规则，调度器按 scope 分发。
func (r *AutomationRuleRepository) ListEnabledScanned(triggerType string) ([]AutomationRule, error) {
	var rows []AutomationRule
	q := r.db.Where("enabled = ?", true)
	if triggerType != "" {
		q = q.Where("trigger_type = ?", triggerType)
	}
	err := q.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

// Update 覆盖保存规则。
func (r *AutomationRuleRepository) Update(row AutomationRule) error {
	return r.db.Save(&row).Error
}

// Delete 按 ID 删除规则。
func (r *AutomationRuleRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&AutomationRule{}).Error
}

// CountByProject 返回 project scope 下的规则数量，用于门控校验。
func (r *AutomationRuleRepository) CountByProject(workspaceID, projectID string) (int64, error) {
	var count int64
	err := r.db.Model(&AutomationRule{}).Where(
		"workspace_id = ? AND scope_type = ? AND scope_id = ?",
		workspaceID, AutomationScopeProject, projectID,
	).Count(&count).Error
	return count, err
}
