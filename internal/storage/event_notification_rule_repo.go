package storage

import (
	"errors"

	"gorm.io/gorm"
)

type EventNotificationRuleRepository struct {
	db *gorm.DB
}

func NewEventNotificationRuleRepository(db *gorm.DB) *EventNotificationRuleRepository {
	return &EventNotificationRuleRepository{db: db}
}

func (r *EventNotificationRuleRepository) Create(row EventNotificationRule) error {
	return r.db.Create(&row).Error
}

func (r *EventNotificationRuleRepository) GetByID(id string) (EventNotificationRule, error) {
	var row EventNotificationRule
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return EventNotificationRule{}, ErrNotFound
	}
	return row, err
}

func (r *EventNotificationRuleRepository) GetByName(workspaceID, name string) (EventNotificationRule, error) {
	var row EventNotificationRule
	err := r.db.Where("workspace_id = ? AND name = ?", workspaceID, name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return EventNotificationRule{}, ErrNotFound
	}
	return row, err
}

func (r *EventNotificationRuleRepository) List(workspaceID string, projectID *string, includeDisabled bool) ([]EventNotificationRule, error) {
	query := r.db.Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		query = query.Where("project_id = ?", *projectID)
	}
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []EventNotificationRule
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *EventNotificationRuleRepository) ListMatching(workspaceID string, projectID *string, eventType string) ([]EventNotificationRule, error) {
	query := r.db.Where("workspace_id = ? AND enabled = ? AND event_type = ?", workspaceID, true, eventType).
		Where("project_id IS NULL")
	if projectID != nil && *projectID != "" {
		query = r.db.Where("workspace_id = ? AND enabled = ? AND event_type = ?", workspaceID, true, eventType).
			Where("(project_id IS NULL OR project_id = ?)", *projectID)
	}
	var rows []EventNotificationRule
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *EventNotificationRuleRepository) Update(row EventNotificationRule) error {
	return r.db.Save(&row).Error
}

func (r *EventNotificationRuleRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&EventNotificationRule{}).Error
}
