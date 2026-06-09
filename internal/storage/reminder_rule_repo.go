package storage

import (
	"errors"

	"gorm.io/gorm"
)

type ReminderRuleRepository struct {
	db *gorm.DB
}

func NewReminderRuleRepository(db *gorm.DB) *ReminderRuleRepository {
	return &ReminderRuleRepository{db: db}
}

func (r *ReminderRuleRepository) Create(row ReminderRule) error {
	return r.db.Create(&row).Error
}

func (r *ReminderRuleRepository) GetByID(id string) (ReminderRule, error) {
	var row ReminderRule
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReminderRule{}, ErrNotFound
	}
	return row, err
}

func (r *ReminderRuleRepository) GetByName(workspaceID, name string) (ReminderRule, error) {
	var row ReminderRule
	err := r.db.Where("workspace_id = ? AND name = ?", workspaceID, name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReminderRule{}, ErrNotFound
	}
	return row, err
}

func (r *ReminderRuleRepository) List(workspaceID string, projectID *string, includeDisabled bool) ([]ReminderRule, error) {
	query := r.db.Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		query = query.Where("project_id = ?", *projectID)
	}
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []ReminderRule
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *ReminderRuleRepository) ListEnabledByWorkspace(workspaceID string) ([]ReminderRule, error) {
	var rows []ReminderRule
	err := r.db.Where("workspace_id = ? AND enabled = ?", workspaceID, true).Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *ReminderRuleRepository) ListEnabled() ([]ReminderRule, error) {
	var rows []ReminderRule
	err := r.db.Where("enabled = ?", true).Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *ReminderRuleRepository) Update(row ReminderRule) error {
	return r.db.Save(&row).Error
}

func (r *ReminderRuleRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&ReminderRule{}).Error
}
