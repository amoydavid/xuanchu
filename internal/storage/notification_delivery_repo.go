package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationDeliveryRepository struct {
	db *gorm.DB
}

type NotificationDeliveryListOptions struct {
	WorkspaceID string
	SinkID      string
	Status      string
	Limit       int
	Offset      int
}

func NewNotificationDeliveryRepository(db *gorm.DB) *NotificationDeliveryRepository {
	return &NotificationDeliveryRepository{db: db}
}

func (r *NotificationDeliveryRepository) Enqueue(rows []NotificationDelivery) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&rows).Error
}

func (r *NotificationDeliveryRepository) ExistsByDedupeKey(dedupeKey string) (bool, error) {
	if dedupeKey == "" {
		return false, nil
	}
	var count int64
	if err := r.db.Model(&NotificationDelivery{}).Where("dedupe_key = ?", dedupeKey).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
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
	return r.ListWithOptions(NotificationDeliveryListOptions{
		WorkspaceID: workspaceID,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	})
}

func (r *NotificationDeliveryRepository) ListWithOptions(opts NotificationDeliveryListOptions) ([]NotificationDelivery, error) {
	query := r.db.Where("workspace_id = ?", opts.WorkspaceID)
	if opts.SinkID != "" {
		query = query.Where("sink_id = ?", opts.SinkID)
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
	claim_expires_at = `+notificationClaimExpiresAtSQL()+`,
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

func notificationClaimExpiresAtSQL() string {
	return `CASE
		WHEN CAST(? AS BIGINT) > COALESCE((SELECT timeout_seconds FROM notification_sinks WHERE notification_sinks.id = notification_deliveries.sink_id), 0) + 60
		THEN CAST(? AS BIGINT) + CAST(? AS BIGINT)
		ELSE CAST(? AS BIGINT) + COALESCE((SELECT timeout_seconds FROM notification_sinks WHERE notification_sinks.id = notification_deliveries.sink_id), 0) + 60
	END`
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
	if message == "" {
		message = "notification disabled"
	}
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

func (r *NotificationDeliveryRepository) ReleaseClaim(id string, now int64) error {
	return r.db.Model(&NotificationDelivery{}).
		Where("id = ? AND status = ?", id, DeliveryStatusDelivering).
		Updates(map[string]any{
			"status":           DeliveryStatusQueued,
			"next_attempt_at":  nil,
			"claim_expires_at": nil,
			"attempt_count":    gorm.Expr("CASE WHEN attempt_count > 0 THEN attempt_count - 1 ELSE 0 END"),
			"modified_at":      now,
		}).Error
}
