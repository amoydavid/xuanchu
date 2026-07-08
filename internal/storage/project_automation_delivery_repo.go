package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 投递状态复用全局 delivery 状态常量，这里提供语义化别名方便 app 层引用。
const (
	ProjectAutomationStatusQueued     = DeliveryStatusQueued
	ProjectAutomationStatusDelivering = DeliveryStatusDelivering
	ProjectAutomationStatusRetryWait  = DeliveryStatusRetryWait
	ProjectAutomationStatusSucceeded  = DeliveryStatusSucceeded
	ProjectAutomationStatusFailed     = DeliveryStatusDeadLettered
)

// ProjectAutomationDeliveryRepository 负责项目自动化投递记录的入队、认领和状态流转。
type ProjectAutomationDeliveryRepository struct{ db *gorm.DB }

// ProjectAutomationDeliveryListOptions 描述投递记录列表查询条件。
type ProjectAutomationDeliveryListOptions struct {
	WorkspaceID string
	ProjectID   *string
	RuleID      string
	Status      string
	Limit       int
	Offset      int
}

// NewProjectAutomationDeliveryRepository 基于已有 *gorm.DB 构建投递仓储。
func NewProjectAutomationDeliveryRepository(db *gorm.DB) *ProjectAutomationDeliveryRepository {
	return &ProjectAutomationDeliveryRepository{db: db}
}

// Enqueue 批量入队，按 dedupe_key 去重。
func (r *ProjectAutomationDeliveryRepository) Enqueue(rows []ProjectAutomationDelivery) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&rows).Error
}

// ExistsByDedupeKey 用于调度器判断同一天/同一事件是否已入队。
func (r *ProjectAutomationDeliveryRepository) ExistsByDedupeKey(key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var count int64
	err := r.db.Model(&ProjectAutomationDelivery{}).Where("dedupe_key = ?", key).Limit(1).Count(&count).Error
	return count > 0, err
}

// GetByID 按 ID 读取投递记录，未找到时返回 ErrNotFound。
func (r *ProjectAutomationDeliveryRepository) GetByID(id string) (ProjectAutomationDelivery, error) {
	var row ProjectAutomationDelivery
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectAutomationDelivery{}, ErrNotFound
	}
	return row, err
}

// List 按条件查询投递记录，按创建时间倒序。
func (r *ProjectAutomationDeliveryRepository) List(opts ProjectAutomationDeliveryListOptions) ([]ProjectAutomationDelivery, error) {
	query := r.db.Where("workspace_id = ?", opts.WorkspaceID)
	if opts.ProjectID != nil {
		query = query.Where("project_id = ?", *opts.ProjectID)
	}
	if opts.RuleID != "" {
		query = query.Where("rule_id = ?", opts.RuleID)
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	var rows []ProjectAutomationDelivery
	err := query.Order("created_at DESC").Find(&rows).Error
	return rows, err
}

// ClaimDue 认领到期的投递，置为 delivering 并增加尝试次数。
func (r *ProjectAutomationDeliveryRepository) ClaimDue(now int64, claimExpiresAt int64, limit int) ([]ProjectAutomationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []ProjectAutomationDelivery
	err := r.db.Raw(`
UPDATE project_automation_deliveries
SET status = ?, claim_expires_at = ?, attempt_count = attempt_count + 1, modified_at = ?
WHERE id IN (
	SELECT id
	FROM project_automation_deliveries
	WHERE status IN (?, ?)
	  AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
	ORDER BY created_at ASC
	LIMIT ?
)
RETURNING *`,
		ProjectAutomationStatusDelivering, claimExpiresAt, now,
		ProjectAutomationStatusQueued, ProjectAutomationStatusRetryWait, now, limit,
	).Scan(&rows).Error
	return rows, err
}

// MarkSucceeded 标记投递成功，记录响应摘要和 provider request id。
func (r *ProjectAutomationDeliveryRepository) MarkSucceeded(id string, now int64, statusCode int, providerRequestID string, responsePreview string, usageJSON string) error {
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":                ProjectAutomationStatusSucceeded,
		"response_status_code":  statusCode,
		"response_body_preview": responsePreview,
		"provider_request_id":   providerRequestID,
		"usage_json":            usageJSON,
		"last_attempt_at":       now,
		"last_error":            "",
		"claim_expires_at":      nil,
		"modified_at":           now,
	}).Error
}

// MarkRetry 标记投递进入重试等待，记录下次尝试时间。
func (r *ProjectAutomationDeliveryRepository) MarkRetry(id string, now int64, nextAttemptAt int64, statusCode *int, message string, responsePreview string) error {
	updates := map[string]any{
		"status":                ProjectAutomationStatusRetryWait,
		"next_attempt_at":       nextAttemptAt,
		"response_body_preview": responsePreview,
		"last_attempt_at":       now,
		"last_error":            message,
		"claim_expires_at":      nil,
		"modified_at":           now,
	}
	if statusCode != nil {
		updates["response_status_code"] = *statusCode
	}
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

// MarkFailed 标记投递进入死信，只允许手动 replay。
func (r *ProjectAutomationDeliveryRepository) MarkFailed(id string, now int64, statusCode *int, message string, responsePreview string) error {
	updates := map[string]any{
		"status":                ProjectAutomationStatusFailed,
		"response_body_preview": responsePreview,
		"last_attempt_at":       now,
		"last_error":            message,
		"claim_expires_at":      nil,
		"modified_at":           now,
	}
	if statusCode != nil {
		updates["response_status_code"] = *statusCode
	}
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

// Requeue 将投递重新置为 queued，供 replay 使用。
func (r *ProjectAutomationDeliveryRepository) Requeue(id string, now int64) error {
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           ProjectAutomationStatusQueued,
		"next_attempt_at":  nil,
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}
