package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationDeliveryRepository struct {
	db *gorm.DB
}

func NewNotificationDeliveryRepository(db *gorm.DB) *NotificationDeliveryRepository {
	return &NotificationDeliveryRepository{db: db}
}

func (r *NotificationDeliveryRepository) Enqueue(rows []NotificationDelivery) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "dedupe_key"}},
		DoNothing: true,
	}).Create(&rows).Error
}

func (r *NotificationDeliveryRepository) GetByID(id string) (NotificationDelivery, error) {
	var row NotificationDelivery
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotificationDelivery{}, ErrNotFound
	}
	return row, err
}

func (r *NotificationDeliveryRepository) List(workspaceID string, status string, limit int, offset int) ([]NotificationDelivery, error) {
	query := r.db.Where("workspace_id = ?", workspaceID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []NotificationDelivery
	err := query.Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (r *NotificationDeliveryRepository) ClaimDue(now int64, claimExpiresAt int64, limit int) ([]NotificationDelivery, error) {
	var rows []NotificationDelivery
	if limit <= 0 {
		return rows, nil
	}
	baseTTL := claimExpiresAt - now
	if baseTTL < 0 {
		baseTTL = 0
	}
	err := r.db.Raw(`
UPDATE notification_deliveries
SET
	status = ?,
	claim_expires_at = CASE
		WHEN CAST(? AS BIGINT) > 60
		THEN CAST(? AS BIGINT) + CAST(? AS BIGINT)
		ELSE CAST(? AS BIGINT) + 60
	END,
	attempt_count = attempt_count + 1,
	modified_at = ?
WHERE id IN (
	SELECT id
	FROM notification_deliveries
	WHERE status IN (?, ?)
	  AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
	ORDER BY created_at ASC
	LIMIT ?
)
RETURNING *`,
		DeliveryStatusDelivering,
		baseTTL,
		now,
		baseTTL,
		now,
		now,
		DeliveryStatusQueued,
		DeliveryStatusRetryWait,
		now,
		limit,
	).Scan(&rows).Error
	return rows, err
}

func (r *NotificationDeliveryRepository) RecoverStaleDelivering(now int64) (int64, error) {
	result := r.db.Model(&NotificationDelivery{}).
		Where("status = ? AND claim_expires_at IS NOT NULL AND claim_expires_at < ?", DeliveryStatusDelivering, now).
		Updates(map[string]any{
			"status":           DeliveryStatusQueued,
			"claim_expires_at": nil,
			"modified_at":      now,
		})
	return result.RowsAffected, result.Error
}

func (r *NotificationDeliveryRepository) MarkSucceeded(id string, now int64, statusCode int) error {
	return r.db.Model(&NotificationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           DeliveryStatusSucceeded,
		"last_attempt_at":  now,
		"last_status_code": statusCode,
		"last_error":       "",
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}

func (r *NotificationDeliveryRepository) MarkRetry(id string, now int64, nextAttemptAt int64, statusCode *int, message string) error {
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
	return r.db.Model(&NotificationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

func (r *NotificationDeliveryRepository) MarkDeadLettered(id string, now int64, statusCode *int, message string) error {
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
	return r.db.Model(&NotificationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

func (r *NotificationDeliveryRepository) MarkDisabledSkipped(id string, now int64, message string) error {
	return r.db.Model(&NotificationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           DeliveryStatusDisabledSkipped,
		"last_error":       message,
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}

func (r *NotificationDeliveryRepository) Requeue(id string, now int64) error {
	return r.db.Model(&NotificationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           DeliveryStatusQueued,
		"next_attempt_at":  nil,
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}
