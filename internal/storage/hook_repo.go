package storage

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

type HookRepository struct {
	db *gorm.DB
}

func NewHookRepository(db *gorm.DB) *HookRepository {
	return &HookRepository{db: db}
}

func (r *HookRepository) Create(row HookDefinition) error {
	return r.db.Create(&row).Error
}

func (r *HookRepository) GetByID(id string) (HookDefinition, error) {
	var row HookDefinition
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return HookDefinition{}, ErrNotFound
	}
	return row, err
}

func (r *HookRepository) List(workspaceID string, projectID *string, includeDisabled bool) ([]HookDefinition, error) {
	query := r.db.Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		query = query.Where("project_id = ?", *projectID)
	}
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []HookDefinition
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *HookRepository) ListMatching(workspaceID string, projectID *string, eventType string) ([]HookDefinition, error) {
	query := r.db.Where("workspace_id = ? AND enabled = ?", workspaceID, true)
	if projectID != nil {
		query = query.Where("project_id = ?", *projectID)
	}
	var rows []HookDefinition
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	var matched []HookDefinition
	for _, row := range rows {
		if !eventTypeInList(row.EventTypesJSON, eventType) {
			continue
		}
		matched = append(matched, row)
	}
	return matched, nil
}

func (r *HookRepository) Update(row HookDefinition) error {
	return r.db.Save(&row).Error
}

func (r *HookRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&HookDefinition{}).Error
}

func eventTypeInList(raw, eventType string) bool {
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return false
	}
	for _, item := range list {
		if item == eventType {
			return true
		}
	}
	return false
}
