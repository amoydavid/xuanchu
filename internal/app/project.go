package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

type ProjectView struct {
	ID          string
	WorkspaceID string
	Slug        string
	Name        string
	Description string
	Status      string
	TaskCount   int
	CreatedAt   int64
	ModifiedAt  int64
	ArchivedAt  *int64
}

type AddProjectInput struct {
	Slug        string
	Name        string
	Description string
}

type ModifyProjectInput struct {
	Name        *string
	Description *string
	Slug        *string
}

func (s *Service) AddProject(input AddProjectInput) (ProjectView, error) {
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectView{}, err
	}
	if s.hasProjectScope() {
		return ProjectView{}, RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
	}
	slug, name, description, err := normalizeProjectCreateInput(input)
	if err != nil {
		return ProjectView{}, err
	}
	var created ProjectView
	err = s.withAudit("project.add", func(tx *Service) (AuditEntry, error) {
		project, err := tx.addProjectLocked(slug, name, description)
		if err != nil {
			return AuditEntry{}, err
		}
		created = projectViewFromRow(project, 0)
		return AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
		}, nil
	})
	return created, err
}

func (s *Service) ListProjects(includeArchived bool) ([]ProjectView, error) {
	if err := s.Require(PermissionProjectRead); err != nil {
		return nil, err
	}
	rows, err := s.projectRepo.List(s.workspaceID, includeArchived)
	if err != nil {
		return nil, err
	}
	projectIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		projectIDs = append(projectIDs, row.ID)
	}
	counts, err := s.projectRepo.TaskCounts(s.workspaceID, projectIDs)
	if err != nil {
		return nil, err
	}
	views := make([]ProjectView, 0, len(rows))
	for _, row := range rows {
		views = append(views, projectViewFromRow(row, counts[row.ID]))
	}
	return filterProjectsByScope(s.requestScope, views), nil
}

func (s *Service) ProjectInfo(ref string) (ProjectView, error) {
	if err := s.Require(PermissionProjectRead); err != nil {
		return ProjectView{}, err
	}
	project, err := s.projectRepo.GetByID(strings.TrimSpace(ref))
	if err == nil {
		if project.WorkspaceID != s.workspaceID {
			return ProjectView{}, RuntimeError{
				Code:    "project_workspace_mismatch",
				Message: fmt.Sprintf("project %q does not belong to workspace %q", ref, s.runtime.WorkspaceSlug),
			}
		}
		if err := s.ensureProjectScope(&project.ID); err != nil {
			return ProjectView{}, err
		}
		return s.projectViewForRow(project)
	}
	if !errors.Is(err, sqlite.ErrNotFound) {
		return ProjectView{}, err
	}
	project, err = s.ResolveProject(ref)
	if err != nil {
		return ProjectView{}, err
	}
	return s.projectViewForRow(project)
}

func (s *Service) ModifyProject(ref string, input ModifyProjectInput) error {
	if err := s.Require(PermissionProjectManage); err != nil {
		return err
	}
	project, err := s.ResolveProject(ref)
	if err != nil {
		return err
	}
	if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
		return RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	normalized, err := normalizeProjectModifyInput(input)
	if err != nil {
		return err
	}
	return s.withAudit("project.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.modifyProjectLocked(project, normalized); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
		}, nil
	})
}

func (s *Service) ArchiveProject(ref string) (ProjectView, error) {
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectView{}, err
	}
	project, err := s.ResolveProject(ref)
	if err != nil {
		return ProjectView{}, err
	}
	if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
		return ProjectView{}, RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	var archived ProjectView
	err = s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		if err := tx.archiveProjectLocked(project); err != nil {
			return nil, nil, err
		}
		archivedProject, err := tx.projectRepo.GetByID(project.ID)
		if err != nil {
			return nil, nil, err
		}
		view, err := tx.projectViewForRow(archivedProject)
		if err != nil {
			return nil, nil, err
		}
		archived = view
		event := buildProjectArchivedHookEvent(view, tx.runtime, tx.clock.Unix())
		entry := AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
		}
		return &entry, []HookEvent{event}, nil
	})
	return archived, err
}

func (s *Service) ResolveProject(ref string) (sqlite.Project, error) {
	return s.ResolveProjectInWorkspace(s.workspaceID, ref)
}

func (s *Service) ResolveProjectInWorkspace(workspaceID, ref string) (sqlite.Project, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return sqlite.Project{}, RuntimeError{Code: "project_not_found", Message: "project reference is required"}
	}
	project, err := s.projectRepo.ResolveInWorkspace(workspaceID, ref)
	if errors.Is(err, sqlite.ErrNotFound) {
		return sqlite.Project{}, RuntimeError{Code: "project_not_found", Message: fmt.Sprintf("project %q not found", ref)}
	}
	if err != nil {
		return sqlite.Project{}, err
	}
	if err := s.ensureProjectScope(&project.ID); err != nil {
		return sqlite.Project{}, err
	}
	return project, nil
}

func (s *Service) addProjectLocked(slug, name, description string) (sqlite.Project, error) {
	now := s.clock.Unix()
	project := sqlite.Project{
		ID:           uuid.NewString(),
		WorkspaceID:  s.workspaceID,
		Slug:         slug,
		Name:         name,
		Description:  description,
		Status:       string(sqlite.ProjectStatusActive),
		SettingsJSON: "{}",
		CreatedAt:    now,
		ModifiedAt:   now,
	}
	created, err := s.projectRepo.Create(project)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return sqlite.Project{}, RuntimeError{
				Code:    "project_already_exists",
				Message: fmt.Sprintf("project %q already exists in workspace %q", slug, s.runtime.WorkspaceSlug),
			}
		}
		return sqlite.Project{}, err
	}
	return created, nil
}

func (s *Service) modifyProjectLocked(project sqlite.Project, input ModifyProjectInput) error {
	if input.Name != nil {
		project.Name = *input.Name
	}
	if input.Description != nil {
		project.Description = *input.Description
	}
	project.ModifiedAt = s.clock.Unix()
	if err := s.projectRepo.Update(project); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return RuntimeError{Code: "project_already_exists", Message: fmt.Sprintf("project %q already exists in workspace %q", project.Slug, s.runtime.WorkspaceSlug)}
		}
		if errors.Is(err, sqlite.ErrNotFound) {
			return RuntimeError{Code: "project_not_found", Message: fmt.Sprintf("project %q not found", project.Slug)}
		}
		return err
	}
	return nil
}

func (s *Service) archiveProjectLocked(project sqlite.Project) error {
	err := s.projectRepo.Archive(project.WorkspaceID, project.ID, s.clock.Unix())
	if errors.Is(err, sqlite.ErrAlreadyArchived) {
		return RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	if errors.Is(err, sqlite.ErrNotFound) {
		return RuntimeError{Code: "project_not_found", Message: fmt.Sprintf("project %q not found", project.Slug)}
	}
	return err
}

func (s *Service) projectViewForRow(project sqlite.Project) (ProjectView, error) {
	counts, err := s.projectRepo.TaskCounts(project.WorkspaceID, []string{project.ID})
	if err != nil {
		return ProjectView{}, err
	}
	return projectViewFromRow(project, counts[project.ID]), nil
}

func projectViewFromRow(project sqlite.Project, taskCount int) ProjectView {
	return ProjectView{
		ID:          project.ID,
		WorkspaceID: project.WorkspaceID,
		Slug:        project.Slug,
		Name:        project.Name,
		Description: project.Description,
		Status:      project.Status,
		TaskCount:   taskCount,
		CreatedAt:   project.CreatedAt,
		ModifiedAt:  project.ModifiedAt,
		ArchivedAt:  project.ArchivedAt,
	}
}

func normalizeProjectCreateInput(input AddProjectInput) (string, string, string, error) {
	slug, err := normalizeProjectSlug(input.Slug)
	if err != nil {
		return "", "", "", err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", "", "", RuntimeError{Code: "project_name_required", Message: "project name is required"}
	}
	return slug, name, strings.TrimSpace(input.Description), nil
}

func normalizeProjectModifyInput(input ModifyProjectInput) (ModifyProjectInput, error) {
	if input.Slug != nil {
		return ModifyProjectInput{}, RuntimeError{Code: "project_slug_immutable", Message: "project slug is immutable"}
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return ModifyProjectInput{}, RuntimeError{Code: "project_name_required", Message: "project name is required"}
		}
		input.Name = &name
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		input.Description = &description
	}
	return input, nil
}

func normalizeProjectSlug(slug string) (string, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return "", RuntimeError{Code: "project_invalid_slug", Message: "project slug is invalid"}
	}
	for _, ch := range slug {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q is invalid", slug)}
	}
	return slug, nil
}
