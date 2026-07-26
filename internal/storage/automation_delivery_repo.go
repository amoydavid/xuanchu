package storage

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 投递状态复用全局 delivery 状态常量，这里提供语义化别名方便 app 层引用。
const (
	AutomationStatusQueued     = DeliveryStatusQueued
	AutomationStatusDelivering = DeliveryStatusDelivering
	AutomationStatusRetryWait  = DeliveryStatusRetryWait
	AutomationStatusSucceeded  = DeliveryStatusSucceeded
	AutomationStatusDeadLetter = DeliveryStatusDeadLettered

	// AutomationStatusFailed 保持向后兼容，等价于 dead lettered。
	AutomationStatusFailed = DeliveryStatusDeadLettered

	// 旧别名保留，Task 2 之前 App 仍引用。
	ProjectAutomationStatusQueued     = DeliveryStatusQueued
	ProjectAutomationStatusDelivering = DeliveryStatusDelivering
	ProjectAutomationStatusRetryWait  = DeliveryStatusRetryWait
	ProjectAutomationStatusSucceeded  = DeliveryStatusSucceeded
	ProjectAutomationStatusFailed     = DeliveryStatusDeadLettered
)

// AutomationDeliveryRepository 负责自动化投递记录的入队、认领、状态流转和 replay。
type AutomationDeliveryRepository struct{ db *gorm.DB }

// AutomationDeliveryListOptions 描述投递记录列表查询条件。
type AutomationDeliveryListOptions struct {
	WorkspaceID string
	RuleScope   string // workspace|project，空表示不限
	RuleID      string
	ProjectID   *string // nil 表示不限；&"" 表示 schedule 无 Project；非空精确匹配
	Status      string
	TriggerType string
	Q           string // 对 delivery/event/provider request ID 做精确或前缀搜索
	Limit       int
	Offset      int
}

// AutomationLatestDelivery 是规则最近一次 Delivery 的安全摘要。
type AutomationLatestDelivery struct {
	RuleID             string
	DeliveryID         string
	Status             string
	ResponseStatusCode *int
	CreatedAt          int64
}

// NewAutomationDeliveryRepository 基于已有 *gorm.DB 构建投递仓储。
func NewAutomationDeliveryRepository(db *gorm.DB) *AutomationDeliveryRepository {
	return &AutomationDeliveryRepository{db: db}
}

// Enqueue 批量入队，按 dedupe_key 去重。
func (r *AutomationDeliveryRepository) Enqueue(rows []AutomationDelivery) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&rows).Error
}

// ExistsByDedupeKey 用于调度器判断同一天/同一事件是否已入队。
func (r *AutomationDeliveryRepository) ExistsByDedupeKey(key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var count int64
	err := r.db.Model(&AutomationDelivery{}).Where("dedupe_key = ?", key).Limit(1).Count(&count).Error
	return count > 0, err
}

// GetByID 按 ID 读取投递记录，未找到时返回 ErrNotFound。
func (r *AutomationDeliveryRepository) GetByID(id string) (AutomationDelivery, error) {
	var row AutomationDelivery
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AutomationDelivery{}, ErrNotFound
	}
	return row, err
}

// List 按条件查询投递记录，按创建时间倒序。
func (r *AutomationDeliveryRepository) List(opts AutomationDeliveryListOptions) ([]AutomationDelivery, error) {
	if opts.WorkspaceID == "" {
		return nil, nil
	}
	query := r.db.Where("workspace_id = ?", opts.WorkspaceID)
	if opts.RuleScope != "" {
		query = query.Where("rule_scope_type = ?", opts.RuleScope)
	}
	if opts.RuleID != "" {
		query = query.Where("rule_id = ?", opts.RuleID)
	}
	if opts.ProjectID != nil {
		if *opts.ProjectID == "" {
			query = query.Where("project_id IS NULL")
		} else {
			query = query.Where("project_id = ?", *opts.ProjectID)
		}
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	}
	if opts.TriggerType != "" {
		query = query.Where("trigger_type = ?", opts.TriggerType)
	}
	if q := strings.TrimSpace(opts.Q); q != "" {
		like := q + "%"
		query = query.Where(
			"id LIKE ? OR event_id LIKE ? OR provider_request_id LIKE ?",
			like, like, like,
		)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	var rows []AutomationDelivery
	err := query.Order("created_at DESC").Find(&rows).Error
	return rows, err
}

// Count 返回与 List 相同过滤条件下的总数，供分页 UI 使用。
func (r *AutomationDeliveryRepository) Count(opts AutomationDeliveryListOptions) (int64, error) {
	if opts.WorkspaceID == "" {
		return 0, nil
	}
	query := r.db.Model(&AutomationDelivery{}).Where("workspace_id = ?", opts.WorkspaceID)
	if opts.RuleScope != "" {
		query = query.Where("rule_scope_type = ?", opts.RuleScope)
	}
	if opts.RuleID != "" {
		query = query.Where("rule_id = ?", opts.RuleID)
	}
	if opts.ProjectID != nil {
		if *opts.ProjectID == "" {
			query = query.Where("project_id IS NULL")
		} else {
			query = query.Where("project_id = ?", *opts.ProjectID)
		}
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	}
	if opts.TriggerType != "" {
		query = query.Where("trigger_type = ?", opts.TriggerType)
	}
	if q := strings.TrimSpace(opts.Q); q != "" {
		like := q + "%"
		query = query.Where(
			"id LIKE ? OR event_id LIKE ? OR provider_request_id LIKE ?",
			like, like, like,
		)
	}
	var total int64
	return total, query.Count(&total).Error
}

// ClaimDue 认领到期的投递，置为 delivering 并增加尝试次数。
// 同时恢复 claim_expires_at 已过期的 stale delivering，避免 worker 崩溃留下僵尸记录。
func (r *AutomationDeliveryRepository) ClaimDue(now int64, claimExpiresAt int64, limit int) ([]AutomationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []AutomationDelivery
	err := r.db.Raw(`
UPDATE automation_deliveries
SET status = ?, claim_expires_at = ?, attempt_count = attempt_count + 1, modified_at = ?
WHERE id IN (
  SELECT id FROM (
    SELECT id FROM automation_deliveries
    WHERE (status IN (?, ?) AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
       OR (status = ? AND claim_expires_at IS NOT NULL AND claim_expires_at <= ?)
    ORDER BY created_at ASC
    LIMIT ?
  ) AS claim_candidates
)
RETURNING *`,
		AutomationStatusDelivering, claimExpiresAt, now,
		AutomationStatusQueued, AutomationStatusRetryWait, now,
		AutomationStatusDelivering, now,
		limit,
	).Scan(&rows).Error
	return rows, err
}

// MarkSucceeded 标记投递成功，记录响应摘要和 provider request id。
func (r *AutomationDeliveryRepository) MarkSucceeded(id string, now int64, statusCode int, providerRequestID string, responsePreview string, usageJSON string) error {
	return r.db.Model(&AutomationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":                AutomationStatusSucceeded,
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
func (r *AutomationDeliveryRepository) MarkRetry(id string, now int64, nextAttemptAt int64, statusCode *int, message string, responsePreview string) error {
	updates := map[string]any{
		"status":                AutomationStatusRetryWait,
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
	return r.db.Model(&AutomationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

// MarkFailed 标记投递进入死信，只允许手动 replay。
func (r *AutomationDeliveryRepository) MarkFailed(id string, now int64, statusCode *int, message string, responsePreview string) error {
	updates := map[string]any{
		"status":                AutomationStatusDeadLetter,
		"response_body_preview": responsePreview,
		"last_attempt_at":       now,
		"last_error":            message,
		"claim_expires_at":      nil,
		"modified_at":           now,
	}
	if statusCode != nil {
		updates["response_status_code"] = *statusCode
	}
	return r.db.Model(&AutomationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

// Requeue 将投递重新置为 queued，供调度器内部恢复使用。
func (r *AutomationDeliveryRepository) Requeue(id string, now int64) error {
	return r.db.Model(&AutomationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           AutomationStatusQueued,
		"next_attempt_at":  nil,
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}

// CreateReplay 复制原 Delivery 的不可变字段，生成一条新 queued Delivery。
// 原 Delivery 保持不动；新记录的 replay_of_delivery_id 指向原记录，dedupe key 重新生成。
func (r *AutomationDeliveryRepository) CreateReplay(original AutomationDelivery, newID, newDedupeKey string, now int64) (AutomationDelivery, error) {
	row := AutomationDelivery{
		ID:                    newID,
		WorkspaceID:           original.WorkspaceID,
		RuleScopeType:         original.RuleScopeType,
		RuleScopeID:           original.RuleScopeID,
		ProjectID:             original.ProjectID,
		RuleID:                original.RuleID,
		TriggerType:           original.TriggerType,
		EventID:               original.EventID,
		EventType:             original.EventType,
		DedupeKey:             newDedupeKey,
		ReplayOfDeliveryID:    strPtrOrNil(original.ID),
		APIKeyConfigKey:       original.APIKeyConfigKey,
		AllowedHostsConfigKey: original.AllowedHostsConfigKey,
		MaxAttempts:           original.MaxAttempts,
		Status:                AutomationStatusQueued,
		ResolvedURL:           original.ResolvedURL,
		RenderedMethod:        original.RenderedMethod,
		RenderedHeadersJSON:   original.RenderedHeadersJSON,
		RequestBodyJSON:       original.RequestBodyJSON,
		RequestBodyPreview:    original.RequestBodyPreview,
		RequestBodyHash:       original.RequestBodyHash,
		UsageJSON:             "{}",
		CreatedAt:             now,
		ModifiedAt:            now,
	}
	if err := r.db.Create(&row).Error; err != nil {
		return AutomationDelivery{}, err
	}
	return row, nil
}

// LatestByRuleIDs 一次批量返回每条 Rule 的最近 Delivery，避免 App 做 N+1 查询。
// 返回 map 的 key 是 rule_id，没有 Delivery 的规则不存在。
func (r *AutomationDeliveryRepository) LatestByRuleIDs(ruleIDs []string) (map[string]AutomationLatestDelivery, error) {
	out := make(map[string]AutomationLatestDelivery)
	if len(ruleIDs) == 0 {
		return out, nil
	}
	var rows []AutomationDelivery
	err := r.db.Where("rule_id IN ?", ruleIDs).Order("created_at DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, ok := out[row.RuleID]; ok {
			continue
		}
		out[row.RuleID] = AutomationLatestDelivery{
			RuleID:             row.RuleID,
			DeliveryID:         row.ID,
			Status:             row.Status,
			ResponseStatusCode: row.ResponseStatusCode,
			CreatedAt:          row.CreatedAt,
		}
	}
	return out, nil
}

// strPtrOrNil 把非空字符串转为 *string；空字符串返回 nil。
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
