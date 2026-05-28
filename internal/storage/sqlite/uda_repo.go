package sqlite

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/dajee/taskg/internal/task"
	"github.com/dajee/taskg/internal/uda"
	"gorm.io/gorm"
)

type UDARepository struct {
	db *gorm.DB
}

func NewUDARepository(db *gorm.DB) *UDARepository {
	return &UDARepository{db: db}
}

func (r *UDARepository) GetDefinition(workspaceID, name string) (uda.Definition, error) {
	var model UDADefinition
	err := r.db.Where("workspace_id = ? AND name = ?", workspaceID, name).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uda.Definition{}, ErrNotFound
	}
	if err != nil {
		return uda.Definition{}, err
	}
	return fromUDADefinitionModel(model), nil
}

func (r *UDARepository) ListDefinitions(workspaceID string) ([]uda.Definition, error) {
	var models []UDADefinition
	if err := r.db.Where("workspace_id = ?", workspaceID).Order("name ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]uda.Definition, 0, len(models))
	for _, model := range models {
		out = append(out, fromUDADefinitionModel(model))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *UDARepository) UpsertDefinition(workspaceID string, def uda.Definition, now int64) error {
	if err := uda.ValidateDefinition(def); err != nil {
		return err
	}
	existing := UDADefinition{}
	err := r.db.Where("workspace_id = ? AND name = ?", workspaceID, def.Name).First(&existing).Error
	createdAt := now
	if err == nil {
		createdAt = existing.CreatedAt
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	valuesJSON, err := uda.ValuesJSON(def.Values)
	if err != nil {
		return err
	}
	return r.db.Save(&UDADefinition{
		WorkspaceID:  workspaceID,
		Name:         def.Name,
		Type:         string(def.Type),
		Label:        def.Label,
		ValuesJSON:   valuesJSON,
		DefaultValue: def.Default,
		CreatedAt:    createdAt,
		ModifiedAt:   now,
	}).Error
}

func (r *UDARepository) DeleteDefinition(workspaceID, name string) error {
	return r.db.Where("workspace_id = ? AND name = ?", workspaceID, name).Delete(&UDADefinition{}).Error
}

func (r *UDARepository) GetTaskUDAs(workspaceID, taskUUID string) (map[string]task.UDAValue, error) {
	var rows []TaskUDAValue
	if err := r.db.Where("workspace_id = ? AND task_uuid = ?", workspaceID, taskUUID).Order("name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return udaValuesFromRows(rows), nil
}

func (r *UDARepository) LoadTaskUDAs(workspaceID string, uuids []string) (map[string]map[string]task.UDAValue, error) {
	out := make(map[string]map[string]task.UDAValue, len(uuids))
	if len(uuids) == 0 {
		return out, nil
	}
	var rows []TaskUDAValue
	if err := r.db.Where("workspace_id = ? AND task_uuid IN ?", workspaceID, uuids).Order("task_uuid ASC, name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if out[row.TaskUUID] == nil {
			out[row.TaskUUID] = map[string]task.UDAValue{}
		}
		out[row.TaskUUID][row.Name] = task.UDAValue{Name: row.Name, Raw: row.Value, Type: row.ValueType, Orphan: row.Orphan}
	}
	return out, nil
}

func (r *UDARepository) ReplaceTaskUDAs(workspaceID, taskUUID string, values map[string]task.UDAValue) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("workspace_id = ? AND task_uuid = ?", workspaceID, taskUUID).Delete(&TaskUDAValue{}).Error; err != nil {
			return err
		}
		for name, value := range values {
			name = strings.TrimSpace(name)
			if name == "" || value.Raw == "" {
				continue
			}
			if err := tx.Create(&TaskUDAValue{
				WorkspaceID: workspaceID,
				TaskUUID:    taskUUID,
				Name:        name,
				Value:       value.Raw,
				ValueType:   value.Type,
				Orphan:      value.Orphan,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *UDARepository) UniqueUDAValues(workspaceID, name string) ([]string, error) {
	var values []string
	err := r.db.Model(&TaskUDAValue{}).
		Where("workspace_id = ? AND name = ? AND value != ''", workspaceID, name).
		Distinct("value").
		Order("value ASC").
		Pluck("value", &values).Error
	return values, err
}

func fromUDADefinitionModel(model UDADefinition) uda.Definition {
	var values []string
	if model.ValuesJSON != "" {
		_ = json.Unmarshal([]byte(model.ValuesJSON), &values)
	}
	return uda.Definition{
		Name:    model.Name,
		Type:    uda.Type(model.Type),
		Label:   model.Label,
		Values:  values,
		Default: model.DefaultValue,
	}
}

func udaValuesFromRows(rows []TaskUDAValue) map[string]task.UDAValue {
	values := make(map[string]task.UDAValue, len(rows))
	for _, row := range rows {
		values[row.Name] = task.UDAValue{Name: row.Name, Raw: row.Value, Type: row.ValueType, Orphan: row.Orphan}
	}
	return values
}
