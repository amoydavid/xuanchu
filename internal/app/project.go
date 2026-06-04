package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
)

type ProjectAnnotationInfo struct {
	ID        string
	ProjectID string
	Entry     int64
	Content   string
	CreatedBy task.UserInfo
	CreatedAt int64
}

type TimelineOptions struct {
	Limit  int
	Offset int
}

type TimelineEntry struct {
	SourceType  string       `json:"source_type"`
	SourceID    string       `json:"source_id"`
	SourceLabel string       `json:"source_label"`
	Entry       int64        `json:"entry"`
	Content     string       `json:"content"`
	CreatedBy   task.UserInfo `json:"created_by"`
}

type ProjectView struct {
	ID                string
	WorkspaceID       string
	Slug              string
	Name              string
	Description       string
	Status            string
	TaskCount         int
	CreatedAt         int64
	ModifiedAt        int64
	ArchivedAt        *int64
	RecentAnnotations []ProjectAnnotationInfo
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
	view := projectViewFromRow(project, counts[project.ID])
	repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
	recent, err := repo.RecentByProject(project.ID, 5)
	if err != nil {
		return ProjectView{}, err
	}
	for _, a := range recent {
		view.RecentAnnotations = append(view.RecentAnnotations, projectAnnotationInfoFromModel(a))
	}
	return view, nil
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
	if slug[0] >= '0' && slug[0] <= '9' {
		return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q must not start with a digit", slug)}
	}
	for _, ch := range slug {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q is invalid", slug)}
	}
	return slug, nil
}

func projectAnnotationInfoFromModel(m sqlite.ProjectAnnotation) ProjectAnnotationInfo {
	return ProjectAnnotationInfo{
		ID:        m.ID,
		ProjectID: m.ProjectID,
		Entry:     m.Entry,
		Content:   m.Content,
		CreatedBy: task.UserInfo{ID: m.CreatedBy},
		CreatedAt: m.CreatedAt,
	}
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

func (s *Service) ProjectAnnotate(projectRef, content string) (ProjectAnnotationInfo, error) {
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectAnnotationInfo{}, err
	}
	var result ProjectAnnotationInfo
	err := s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		annotation, project, err := tx.projectAnnotateLocked(projectRef, content)
		if err != nil {
			return nil, nil, err
		}
		result = annotation
		view := projectViewFromRow(project, 0)
		event := buildProjectAnnotatedHookEvent(view, annotation, tx.runtime, tx.clock.Unix())
		entry := AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
			Action:      "project.annotate",
			Payload: map[string]any{
				"annotation_id":    annotation.ID,
				"content_preview":  truncateString(annotation.Content, 200),
			},
		}
		return []AuditEntry{entry}, []HookEvent{event}, nil
	})
	return result, err
}

func (s *Service) projectAnnotateLocked(projectRef, content string) (ProjectAnnotationInfo, sqlite.Project, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return ProjectAnnotationInfo{}, sqlite.Project{}, RuntimeError{Code: "annotation_content_required", Message: "annotation content is required"}
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAnnotationInfo{}, sqlite.Project{}, err
	}
	if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
		return ProjectAnnotationInfo{}, sqlite.Project{}, RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
	now := s.clock.Unix()
	for attempts := 0; attempts < 3; attempts++ {
		entry := now + int64(attempts)
		annotation := sqlite.ProjectAnnotation{
			ID:        uuid.NewString(),
			ProjectID: project.ID,
			Entry:     entry,
			Content:   content,
			CreatedBy: s.runtime.ActorUserID,
			CreatedAt: now,
		}
		created, err := repo.Create(annotation)
		if err != nil {
			if sqlite.IsUniqueConstraintError(err) {
				continue
			}
			return ProjectAnnotationInfo{}, sqlite.Project{}, err
		}
		project.ModifiedAt = now
		if err := s.projectRepo.Update(project); err != nil {
			return ProjectAnnotationInfo{}, sqlite.Project{}, err
		}
		return projectAnnotationInfoFromModel(created), project, nil
	}
	return ProjectAnnotationInfo{}, sqlite.Project{}, RuntimeError{Code: "annotation_conflict", Message: "annotation conflict could not be resolved"}
}

func (s *Service) ProjectDenotate(projectRef, annotationID string) error {
	if err := s.Require(PermissionProjectManage); err != nil {
		return err
	}
	return s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		project, annotationID, err := tx.projectDenotateLocked(projectRef, annotationID)
		if err != nil {
			return nil, nil, err
		}
		view := projectViewFromRow(project, 0)
		event := buildProjectDenotatedHookEvent(view, annotationID, tx.runtime, tx.clock.Unix())
		entry := AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
			Action:      "project.denotate",
			Payload: map[string]any{
				"annotation_id": annotationID,
			},
		}
		return []AuditEntry{entry}, []HookEvent{event}, nil
	})
}

func (s *Service) projectDenotateLocked(projectRef, annotationID string) (sqlite.Project, string, error) {
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return sqlite.Project{}, "", err
	}
	if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
		return sqlite.Project{}, "", RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
	annotation, err := repo.GetByID(annotationID)
	if err != nil {
		return sqlite.Project{}, "", err
	}
	if annotation.ProjectID != project.ID {
		return sqlite.Project{}, "", RuntimeError{Code: "annotation_not_found", Message: fmt.Sprintf("annotation %q does not belong to project %q", annotationID, project.Slug)}
	}
	if err := repo.Delete(annotationID); err != nil {
		return sqlite.Project{}, "", err
	}
	project.ModifiedAt = s.clock.Unix()
	if err := s.projectRepo.Update(project); err != nil {
		return sqlite.Project{}, "", err
	}
	return project, annotationID, nil
}

func (s *Service) ProjectAnnotations(projectRef string) ([]ProjectAnnotationInfo, error) {
	if err := s.Require(PermissionProjectRead); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
	annotations, err := repo.ListByProject(project.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectAnnotationInfo, 0, len(annotations))
	for _, a := range annotations {
		out = append(out, projectAnnotationInfoFromModel(a))
	}
	return out, nil
}

func (s *Service) ProjectTimeline(projectRef string, opts TimelineOptions) ([]TimelineEntry, error) {
	if err := s.Require(PermissionProjectRead); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
	rows, err := repo.TimelineByProjectID(project.ID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	out := make([]TimelineEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, TimelineEntry{
			SourceType:  r.SourceType,
			SourceID:    r.SourceID,
			SourceLabel: r.SourceLabel,
			Entry:       r.Entry,
			Content:     r.Content,
			CreatedBy:   task.UserInfo{ID: r.CreatedBy},
		})
	}
	return out, nil
}
