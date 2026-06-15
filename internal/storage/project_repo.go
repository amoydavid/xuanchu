package storage

import (
	"errors"
	"fmt"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProjectStatus string

const (
	ProjectStatusPlanning  ProjectStatus = "planning"
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusArchived  ProjectStatus = "archived"
	ProjectStatusCancelled ProjectStatus = "cancelled"
)

func IsValidProjectStatus(status string) bool {
	switch ProjectStatus(status) {
	case ProjectStatusPlanning, ProjectStatusActive, ProjectStatusArchived, ProjectStatusCancelled:
		return true
	}
	return false
}

// IsProjectClosedStatus 判断状态是否为关闭态（archived/cancelled）。
// 约定：cancelled 不设 ArchivedAt，仅靠 status 判别；archived 必设 ArchivedAt。
// 所有写路径（UpdateStatus/Archive）必须保持 status 与 ArchivedAt 的一致性。
func IsProjectClosedStatus(status string) bool {
	return status == string(ProjectStatusArchived) || status == string(ProjectStatusCancelled)
}

var ErrAlreadyArchived = errors.New("project already archived")

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) Create(project Project) (Project, error) {
	if project.NextTaskSeq <= 0 {
		project.NextTaskSeq = 1
	}
	if err := r.db.Create(&project).Error; err != nil {
		return Project{}, err
	}
	return project, nil
}

func (r *ProjectRepository) AllocateProjectTaskSeqLocked(workspaceID, projectID string) (int64, error) {
	var project Project
	query := r.db.Where("workspace_id = ? AND id = ?", workspaceID, projectID)
	if r.db.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	seq := project.NextTaskSeq
	if seq <= 0 {
		seq = 1
	}
	result := r.db.Model(&Project{}).
		Where("workspace_id = ? AND id = ?", workspaceID, projectID).
		Update("next_task_seq", seq+1)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, ErrNotFound
	}
	return seq, nil
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
	nextTaskSeq := project.NextTaskSeq
	if nextTaskSeq <= 0 {
		nextTaskSeq = 1
	}
	result := r.db.Model(&Project{}).
		Where("id = ?", project.ID).
		Updates(map[string]any{
			"workspace_id":  project.WorkspaceID,
			"slug":          project.Slug,
			"name":          project.Name,
			"description":   project.Description,
			"status":        project.Status,
			"settings_json": project.SettingsJSON,
			"next_task_seq": nextTaskSeq,
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
	if IsProjectClosedStatus(project.Status) {
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

func (r *ProjectRepository) ListByStatus(workspaceID, statusFilter string) ([]Project, error) {
	var projects []Project
	query := r.db.Where("workspace_id = ?", workspaceID)
	switch statusFilter {
	case "all", "":
	case "open":
		query = query.Where("status IN ?", []string{string(ProjectStatusPlanning), string(ProjectStatusActive)})
	case "planning", "active", "archived", "cancelled":
		query = query.Where("status = ?", statusFilter)
	default:
		return nil, fmt.Errorf("invalid project status filter: %s", statusFilter)
	}
	if err := query.Order("slug ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *ProjectRepository) UpdateStatus(workspaceID, projectID, status string, now int64) error {
	if !IsValidProjectStatus(status) {
		return fmt.Errorf("invalid project status: %s", status)
	}
	updates := map[string]any{
		"status":      status,
		"modified_at": now,
	}
	if status == string(ProjectStatusArchived) {
		updates["archived_at"] = now
	} else {
		updates["archived_at"] = nil
	}
	result := r.db.Model(&Project{}).
		Where("workspace_id = ? AND id = ?", workspaceID, projectID).
		Updates(updates)
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
