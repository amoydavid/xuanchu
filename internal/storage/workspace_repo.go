package storage

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

// ListAll 返回全部 workspace（跨用户），供 server admin 管控用。
// includeArchived=false 时只返回未归档 workspace，与 ListVisibleForUser 的归档语义一致。
func (r *WorkspaceRepository) ListAll(includeArchived bool) ([]Workspace, error) {
	query := r.db.Model(&Workspace{})
	if !includeArchived {
		query = query.Where("archived_at IS NULL")
	}
	query = query.Order("slug ASC")
	var rows []Workspace
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountMembersByRole 返回 workspace 内每个 role 的成员数，供 admin 控制面摘要用。
// 只统计 owner/admin/member/viewer 四种 role。
func (r *WorkspaceRepository) CountMembersByRole(workspaceID string) (map[string]int64, error) {
	type counts struct {
		Role  string
		Count int64
	}
	var rows []counts
	if err := r.db.Table("memberships").
		Select("role, count(*) as count").
		Where("workspace_id = ?", workspaceID).
		Group("role").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]int64{"owner": 0, "admin": 0, "member": 0, "viewer": 0}
	for _, row := range rows {
		out[row.Role] = row.Count
	}
	return out, nil
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
	result := r.db.Model(&Workspace{}).Where("id = ?", workspaceID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *WorkspaceRepository) Archive(workspaceID string, archivedAt int64) error {
	result := r.db.Model(&Workspace{}).Where("id = ?", workspaceID).Updates(map[string]any{
		"archived_at": archivedAt,
		"modified_at": archivedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
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
