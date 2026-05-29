package sqlite

import (
	"errors"

	"gorm.io/gorm"
)

type WorkspaceWithRole struct {
	Workspace Workspace
	Role      string
}

type WorkspaceMetadataUpdate struct {
	Name        *string
	Description *string
	Visibility  *string
}

type WorkspaceRepository struct {
	db *gorm.DB
}

func NewWorkspaceRepository(db *gorm.DB) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

func (r *WorkspaceRepository) Create(workspace Workspace) (Workspace, error) {
	if err := r.db.Create(&workspace).Error; err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func (r *WorkspaceRepository) GetByID(id string) (Workspace, error) {
	return r.find("id = ?", id)
}

func (r *WorkspaceRepository) GetBySlug(slug string) (Workspace, error) {
	return r.find("slug = ?", slug)
}

func (r *WorkspaceRepository) ListVisibleForUser(userID string, includeArchived bool) ([]WorkspaceWithRole, error) {
	type row struct {
		Workspace
		Role string
	}
	query := r.db.Table("workspaces").
		Select("workspaces.*, memberships.role").
		Joins("JOIN memberships ON memberships.workspace_id = workspaces.id").
		Where("memberships.user_id = ?", userID)
	if !includeArchived {
		query = query.Where("workspaces.archived_at IS NULL")
	}
	query = query.Order("workspaces.slug ASC")

	var rows []row
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]WorkspaceWithRole, 0, len(rows))
	for _, item := range rows {
		out = append(out, WorkspaceWithRole{
			Workspace: item.Workspace,
			Role:      item.Role,
		})
	}
	return out, nil
}

func (r *WorkspaceRepository) UpdateMetadata(workspaceID string, input WorkspaceMetadataUpdate, modifiedAt int64) error {
	updates := map[string]any{"modified_at": modifiedAt}
	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.Visibility != nil {
		updates["visibility"] = *input.Visibility
	}
	return r.db.Model(&Workspace{}).Where("id = ?", workspaceID).Updates(updates).Error
}

func (r *WorkspaceRepository) Archive(workspaceID string, archivedAt int64) error {
	return r.db.Model(&Workspace{}).Where("id = ?", workspaceID).Updates(map[string]any{
		"archived_at": archivedAt,
		"modified_at": archivedAt,
	}).Error
}

func (r *WorkspaceRepository) find(query string, args ...any) (Workspace, error) {
	var workspace Workspace
	err := r.db.Where(query, args...).First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}
