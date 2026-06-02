package sqlite

import (
	"errors"

	"gorm.io/gorm"
)

const (
	DeliveryStatusQueued          = "queued"
	DeliveryStatusDelivering      = "delivering"
	DeliveryStatusRetryWait       = "retry_wait"
	DeliveryStatusSucceeded       = "succeeded"
	DeliveryStatusDeadLettered    = "dead_lettered"
	DeliveryStatusDisabledSkipped = "disabled_skipped"
)

type HookDeliveryRepository struct {
	db *gorm.DB
}

func NewHookDeliveryRepository(db *gorm.DB) *HookDeliveryRepository {
	return &HookDeliveryRepository{db: db}
}

func (r *HookDeliveryRepository) Enqueue(rows []HookDelivery) error {
	return r.db.Create(&rows).Error
}

func (r *HookDeliveryRepository) GetByID(id string) (HookDelivery, error) {
	var row HookDelivery
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return HookDelivery{}, ErrNotFound
	}
	return row, err
}

func (r *HookDeliveryRepository) ListByHook(hookID string, status string, limit int) ([]HookDelivery, error) {
	query := r.db.Where("hook_id = ?", hookID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []HookDelivery
	err := query.Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (r *HookDeliveryRepository) ClaimDue(now int64, claimExpiresAt int64, limit int) ([]HookDelivery, error) {
	var rows []HookDelivery
	if limit <= 0 {
		return rows, nil
	}
	baseTTL := claimExpiresAt - now
	if baseTTL < 0 {
		baseTTL = 0
	}
	err := r.db.Raw(`
UPDATE hook_deliveries
SET
	status = ?,
	claim_expires_at = ? + MAX(?, COALESCE((SELECT timeout_seconds FROM hook_definitions WHERE hook_definitions.id = hook_deliveries.hook_id), 0) + 60),
	attempt_count = attempt_count + 1,
	modified_at = ?
WHERE id IN (
	SELECT id
	FROM hook_deliveries
	WHERE status IN (?, ?)
	  AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
	ORDER BY created_at ASC
	LIMIT ?
)
RETURNING *`,
		DeliveryStatusDelivering,
		now,
		baseTTL,
		now,
		DeliveryStatusQueued,
		DeliveryStatusRetryWait,
		now,
		limit,
	).Scan(&rows).Error
	return rows, err
}

func (r *HookDeliveryRepository) RecoverStaleDelivering(now int64) (int64, error) {
	result := r.db.Model(&HookDelivery{}).
		Where("status = ? AND claim_expires_at IS NOT NULL AND claim_expires_at < ?", DeliveryStatusDelivering, now).
		Updates(map[string]any{
			"status":           DeliveryStatusQueued,
			"claim_expires_at": nil,
			"modified_at":      now,
		})
	return result.RowsAffected, result.Error
}

func (r *HookDeliveryRepository) MarkSucceeded(id string, now int64, statusCode int) error {
	return r.db.Model(&HookDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           DeliveryStatusSucceeded,
		"last_attempt_at":  now,
		"last_status_code": statusCode,
		"last_error":       "",
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}

func (r *HookDeliveryRepository) MarkRetry(id string, now int64, nextAttemptAt int64, statusCode *int, message string) error {
	updates := map[string]any{
		"status":           DeliveryStatusRetryWait,
		"next_attempt_at":  nextAttemptAt,
		"last_attempt_at":  now,
		"last_error":       message,
		"claim_expires_at": nil,
		"modified_at":      now,
	}
	if statusCode != nil {
		updates["last_status_code"] = *statusCode
	}
	return r.db.Model(&HookDelivery{}).Where("id = ?", id).Updates(updates).Error
}

func (r *HookDeliveryRepository) MarkDeadLettered(id string, now int64, statusCode *int, message string) error {
	updates := map[string]any{
		"status":           DeliveryStatusDeadLettered,
		"last_attempt_at":  now,
		"last_error":       message,
		"claim_expires_at": nil,
		"modified_at":      now,
	}
	if statusCode != nil {
		updates["last_status_code"] = *statusCode
	}
	return r.db.Model(&HookDelivery{}).Where("id = ?", id).Updates(updates).Error
}

func (r *HookDeliveryRepository) MarkDisabledSkipped(id string, now int64) error {
	return r.db.Model(&HookDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           DeliveryStatusDisabledSkipped,
		"last_error":       "hook disabled",
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}

func (r *HookDeliveryRepository) Requeue(id string, now int64) error {
	return r.db.Model(&HookDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           DeliveryStatusQueued,
		"next_attempt_at":  nil,
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}
