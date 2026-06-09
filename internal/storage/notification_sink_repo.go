package storage

import (
	"errors"

	"gorm.io/gorm"
)

type NotificationSinkRepository struct {
	db *gorm.DB
}

func NewNotificationSinkRepository(db *gorm.DB) *NotificationSinkRepository {
	return &NotificationSinkRepository{db: db}
}

func (r *NotificationSinkRepository) Create(row NotificationSink) error {
	return r.db.Create(&row).Error
}

func (r *NotificationSinkRepository) GetByID(id string) (NotificationSink, error) {
	var row NotificationSink
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotificationSink{}, ErrNotFound
	}
	return row, err
}

func (r *NotificationSinkRepository) GetByName(workspaceID, name string) (NotificationSink, error) {
	var row NotificationSink
	err := r.db.Where("workspace_id = ? AND name = ?", workspaceID, name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotificationSink{}, ErrNotFound
	}
	return row, err
}

func (r *NotificationSinkRepository) List(workspaceID string, includeDisabled bool) ([]NotificationSink, error) {
	query := r.db.Where("workspace_id = ?", workspaceID)
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []NotificationSink
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *NotificationSinkRepository) Update(row NotificationSink) error {
	return r.db.Save(&row).Error
}

func (r *NotificationSinkRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&NotificationSink{}).Error
}
