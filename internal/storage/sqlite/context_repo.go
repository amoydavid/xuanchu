package sqlite

import (
	"errors"

	domain "github.com/dajee/taskg/internal/taskcontext"
	"gorm.io/gorm"
)

type ContextRepository struct {
	db *gorm.DB
}

func NewContextRepository(db *gorm.DB) *ContextRepository {
	return &ContextRepository{db: db}
}

func (r *ContextRepository) Upsert(ctx domain.Context) error {
	return r.db.Save(&Context{
		WorkspaceID:  ctx.WorkspaceID,
		Name:         ctx.Name,
		FilterSource: ctx.FilterSource,
		CreatedAt:    ctx.CreatedAt,
		ModifiedAt:   ctx.ModifiedAt,
	}).Error
}

func (r *ContextRepository) Get(workspaceID, name string) (domain.Context, error) {
	var model Context
	err := r.db.Where("workspace_id = ? AND name = ?", workspaceID, name).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Context{}, ErrNotFound
	}
	if err != nil {
		return domain.Context{}, err
	}
	return fromContextModel(model), nil
}

func (r *ContextRepository) List(workspaceID string) ([]domain.Context, error) {
	var models []Context
	if err := r.db.Where("workspace_id = ?", workspaceID).Order("name ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Context, 0, len(models))
	for _, model := range models {
		out = append(out, fromContextModel(model))
	}
	return out, nil
}

func (r *ContextRepository) Delete(workspaceID, name string) error {
	return r.db.Where("workspace_id = ? AND name = ?", workspaceID, name).Delete(&Context{}).Error
}

func fromContextModel(model Context) domain.Context {
	return domain.Context{
		WorkspaceID:  model.WorkspaceID,
		Name:         model.Name,
		FilterSource: model.FilterSource,
		CreatedAt:    model.CreatedAt,
		ModifiedAt:   model.ModifiedAt,
	}
}
