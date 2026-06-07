package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConfigDefinitionRepository struct {
	db *gorm.DB
}

func NewConfigDefinitionRepository(db *gorm.DB) *ConfigDefinitionRepository {
	return &ConfigDefinitionRepository{db: db}
}

func (r *ConfigDefinitionRepository) Get(workspaceID, key string) (ConfigDefinition, bool, error) {
	var row ConfigDefinition
	err := r.db.Where("workspace_id = ? AND key = ?", workspaceID, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConfigDefinition{}, false, nil
	}
	if err != nil {
		return ConfigDefinition{}, false, err
	}
	return row, true, nil
}

func (r *ConfigDefinitionRepository) Set(def ConfigDefinition) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "workspace_id"},
			{Name: "key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"value_type",
			"allowed_scopes_json",
			"label",
			"description",
			"enum_values_json",
			"default_value",
			"has_default",
			"required",
			"secret",
			"modified_at",
		}),
	}).Create(&def).Error
}

func (r *ConfigDefinitionRepository) Delete(workspaceID, key string) error {
	return r.db.Where("workspace_id = ? AND key = ?", workspaceID, key).Delete(&ConfigDefinition{}).Error
}

func (r *ConfigDefinitionRepository) List(workspaceID string) ([]ConfigDefinition, error) {
	var rows []ConfigDefinition
	if err := r.db.Where("workspace_id = ?", workspaceID).Order("key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
