package sqlite

import (
	"errors"

	domain "github.com/dajee/taskg/internal/task"
	"gorm.io/gorm"
)

type ProjectStatus string

const (
	ProjectStatusActive   ProjectStatus = "active"
	ProjectStatusArchived ProjectStatus = "archived"
)

var ErrAlreadyArchived = errors.New("project already archived")

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) Create(project Project) (Project, error) {
	if err := r.db.Create(&project).Error; err != nil {
		return Project{}, err
	}
	return project, nil
}

func (r *ProjectRepository) List(workspaceID string, includeArchived bool) ([]Project, error) {
	var projects []Project
	query := r.db.Where("workspace_id = ?", workspaceID)
	if !includeArchived {
		query = query.Where("status = ?", string(ProjectStatusActive))
	}
	if err := query.Order("slug ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *ProjectRepository) GetByID(id string) (Project, error) {
	return r.find("id = ?", id)
}

func (r *ProjectRepository) GetBySlug(workspaceID, slug string) (Project, error) {
	return r.find("workspace_id = ? AND slug = ?", workspaceID, slug)
}

func (r *ProjectRepository) GetByRef(workspaceID, ref string) (Project, error) {
	return r.ResolveInWorkspace(workspaceID, ref)
}

func (r *ProjectRepository) ResolveInWorkspace(workspaceID, ref string) (Project, error) {
	return r.find("workspace_id = ? AND (id = ? OR slug = ?)", workspaceID, ref, ref)
}

func (r *ProjectRepository) Update(project Project) error {
	result := r.db.Model(&Project{}).
		Where("id = ?", project.ID).
		Updates(map[string]any{
			"workspace_id":  project.WorkspaceID,
			"slug":          project.Slug,
			"name":          project.Name,
			"description":   project.Description,
			"status":        project.Status,
			"settings_json": project.SettingsJSON,
			"created_at":    project.CreatedAt,
			"modified_at":   project.ModifiedAt,
			"archived_at":   project.ArchivedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectRepository) Archive(workspaceID, id string, now int64) error {
	project, err := r.ResolveInWorkspace(workspaceID, id)
	if err != nil {
		return err
	}
	if project.Status == string(ProjectStatusArchived) || project.ArchivedAt != nil {
		return ErrAlreadyArchived
	}
	result := r.db.Model(&Project{}).
		Where("workspace_id = ? AND id = ?", workspaceID, id).
		Updates(map[string]any{
			"status":      string(ProjectStatusArchived),
			"archived_at": now,
			"modified_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectRepository) TaskCounts(workspaceID string, projectIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(projectIDs))
	if len(projectIDs) == 0 {
		return counts, nil
	}
	for _, id := range projectIDs {
		counts[id] = 0
	}

	type row struct {
		ProjectID string
		Count     int
	}
	var rows []row
	if err := r.db.Model(&Task{}).
		Select("project_id, COUNT(*) AS count").
		Where("workspace_id = ? AND project_id IN ? AND status <> ?", workspaceID, projectIDs, domain.StatusDeleted).
		Group("project_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ProjectID] = row.Count
	}
	return counts, nil
}

func (r *ProjectRepository) find(query string, args ...any) (Project, error) {
	var project Project
	err := r.db.Where(query, args...).First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, err
	}
	return project, nil
}
