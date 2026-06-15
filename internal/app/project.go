package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
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
	SourceType  string        `json:"source_type"`
	SourceID    string        `json:"source_id"`
	SourceLabel string        `json:"source_label"`
	Entry       int64         `json:"entry"`
	Content     string        `json:"content"`
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

func (s *Service) ListProjectsByStatus(statusFilter string) ([]ProjectView, error) {
	if err := s.Require(PermissionProjectRead); err != nil {
		return nil, err
	}
	if statusFilter == "" {
		statusFilter = "open"
	}
	if !isValidProjectStatusFilter(statusFilter) {
		return nil, RuntimeError{Code: "project_invalid_status_filter", Message: fmt.Sprintf("invalid project status filter %q", statusFilter)}
	}
	rows, err := s.projectRepo.ListByStatus(s.workspaceID, statusFilter)
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

func (s *Service) ListProjects(includeArchived bool) ([]ProjectView, error) {
	if includeArchived {
		return s.ListProjectsByStatus("all")
	}
	return s.ListProjectsByStatus("open")
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
	if !errors.Is(err, storage.ErrNotFound) {
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
	if isProjectClosed(project) {
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
	if isProjectClosed(project) {
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

func (s *Service) ResolveProject(ref string) (storage.Project, error) {
	return s.ResolveProjectInWorkspace(s.workspaceID, ref)
}

func (s *Service) ResolveProjectInWorkspace(workspaceID, ref string) (storage.Project, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.Project{}, RuntimeError{Code: "project_not_found", Message: "project reference is required"}
	}
	project, err := s.projectRepo.ResolveInWorkspace(workspaceID, ref)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.Project{}, RuntimeError{Code: "project_not_found", Message: fmt.Sprintf("project %q not found", ref)}
	}
	if err != nil {
		return storage.Project{}, err
	}
	if err := s.ensureProjectScope(&project.ID); err != nil {
		return storage.Project{}, err
	}
	return project, nil
}

func (s *Service) addProjectLocked(slug, name, description string) (storage.Project, error) {
	now := s.clock.Unix()
	project := storage.Project{
		ID:           uuid.NewString(),
		WorkspaceID:  s.workspaceID,
		Slug:         slug,
		Name:         name,
		Description:  description,
		Status:       string(storage.ProjectStatusPlanning),
		SettingsJSON: "{}",
		CreatedAt:    now,
		ModifiedAt:   now,
	}
	created, err := s.projectRepo.Create(project)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return storage.Project{}, RuntimeError{
				Code:    "project_already_exists",
				Message: fmt.Sprintf("project %q already exists in workspace %q", slug, s.runtime.WorkspaceSlug),
			}
		}
		return storage.Project{}, err
	}
	return created, nil
}

func (s *Service) modifyProjectLocked(project storage.Project, input ModifyProjectInput) error {
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
		if errors.Is(err, storage.ErrNotFound) {
			return RuntimeError{Code: "project_not_found", Message: fmt.Sprintf("project %q not found", project.Slug)}
		}
		return err
	}
	return nil
}

func (s *Service) archiveProjectLocked(project storage.Project) error {
	err := s.projectRepo.Archive(project.WorkspaceID, project.ID, s.clock.Unix())
	if errors.Is(err, storage.ErrAlreadyArchived) {
		return RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	if errors.Is(err, storage.ErrNotFound) {
		return RuntimeError{Code: "project_not_found", Message: fmt.Sprintf("project %q not found", project.Slug)}
	}
	return err
}

func (s *Service) projectViewForRow(project storage.Project) (ProjectView, error) {
	counts, err := s.projectRepo.TaskCounts(project.WorkspaceID, []string{project.ID})
	if err != nil {
		return ProjectView{}, err
	}
	view := projectViewFromRow(project, counts[project.ID])
	repo := storage.NewProjectAnnotationRepository(s.store.DB())
	recent, err := repo.RecentByProject(project.ID, 5)
	if err != nil {
		return ProjectView{}, err
	}
	for _, a := range recent {
		view.RecentAnnotations = append(view.RecentAnnotations, projectAnnotationInfoFromModel(a))
	}
	return view, nil
}

func projectViewFromRow(project storage.Project, taskCount int) ProjectView {
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
	if len(slug) < 3 || len(slug) > 10 {
		return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q must be 3-10 lowercase letters or digits", slug)}
	}
	if slug[0] < 'a' || slug[0] > 'z' {
		return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q must start with a letter", slug)}
	}
	for _, ch := range slug {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q is invalid", slug)}
	}
	return slug, nil
}

func projectAnnotationInfoFromModel(m storage.ProjectAnnotation) ProjectAnnotationInfo {
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
		userInfos, err := tx.resolveUserInfos([]string{annotation.CreatedBy.ID})
		if err != nil {
			return nil, nil, err
		}
		result = annotation
		if userInfo, ok := userInfos[annotation.CreatedBy.ID]; ok {
			annotation.CreatedBy = userInfo
			result = annotation
		}
		view := projectViewFromRow(project, 0)
		event := buildProjectAnnotatedHookEvent(view, annotation, tx.runtime, tx.clock.Unix())
		entry := AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
			Action:      "project.annotate",
			Payload: map[string]any{
				"annotation_id":   annotation.ID,
				"content_preview": truncateString(annotation.Content, 200),
			},
		}
		return []AuditEntry{entry}, []HookEvent{event}, nil
	})
	return result, err
}

func (s *Service) projectAnnotateLocked(projectRef, content string) (ProjectAnnotationInfo, storage.Project, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return ProjectAnnotationInfo{}, storage.Project{}, RuntimeError{Code: "annotation_content_required", Message: "annotation content is required"}
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAnnotationInfo{}, storage.Project{}, err
	}
	if isProjectClosed(project) {
		return ProjectAnnotationInfo{}, storage.Project{}, RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	created, err := s.writeProjectAnnotation(project, content)
	if err != nil {
		return ProjectAnnotationInfo{}, storage.Project{}, err
	}
	project.ModifiedAt = s.clock.Unix()
	if err := s.projectRepo.Update(project); err != nil {
		return ProjectAnnotationInfo{}, storage.Project{}, err
	}
	return projectAnnotationInfoFromModel(created), project, nil
}

// writeProjectAnnotation 写入一条项目注解，不做 closed 校验，也不更新 project.ModifiedAt。
// 调用方负责在必要时更新 project 的修改时间。TransitionProject 复用它记录状态变更。
func (s *Service) writeProjectAnnotation(project storage.Project, content string) (storage.ProjectAnnotation, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return storage.ProjectAnnotation{}, RuntimeError{Code: "annotation_content_required", Message: "annotation content is required"}
	}
	repo := storage.NewProjectAnnotationRepository(s.store.DB())
	now := s.clock.Unix()
	for attempts := 0; attempts < 5; attempts++ {
		maxEntry, err := repo.MaxEntryByProject(project.ID)
		if err != nil {
			return storage.ProjectAnnotation{}, err
		}
		entry := now + int64(attempts)
		if maxEntry+1 > entry {
			entry = maxEntry + 1
		}
		annotation := storage.ProjectAnnotation{
			ID:        uuid.NewString(),
			ProjectID: project.ID,
			Entry:     entry,
			Content:   content,
			CreatedBy: s.runtime.ActorUserID,
			CreatedAt: now,
		}
		created, err := repo.Create(annotation)
		if err != nil {
			if storage.IsUniqueConstraintError(err) {
				continue
			}
			return storage.ProjectAnnotation{}, err
		}
		return created, nil
	}
	return storage.ProjectAnnotation{}, RuntimeError{Code: "annotation_conflict", Message: "annotation conflict could not be resolved"}
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

func (s *Service) projectDenotateLocked(projectRef, annotationID string) (storage.Project, string, error) {
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return storage.Project{}, "", err
	}
	if isProjectClosed(project) {
		return storage.Project{}, "", RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	repo := storage.NewProjectAnnotationRepository(s.store.DB())
	annotation, err := repo.GetByID(annotationID)
	if err != nil {
		return storage.Project{}, "", err
	}
	if annotation.ProjectID != project.ID {
		return storage.Project{}, "", RuntimeError{Code: "annotation_not_found", Message: fmt.Sprintf("annotation %q does not belong to project %q", annotationID, project.Slug)}
	}
	if err := repo.Delete(annotationID); err != nil {
		return storage.Project{}, "", err
	}
	project.ModifiedAt = s.clock.Unix()
	if err := s.projectRepo.Update(project); err != nil {
		return storage.Project{}, "", err
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
	repo := storage.NewProjectAnnotationRepository(s.store.DB())
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
	repo := storage.NewProjectAnnotationRepository(s.store.DB())
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

func isProjectClosed(project storage.Project) bool {
	return storage.IsProjectClosedStatus(project.Status)
}

// IsProjectStatusClosed 判断给定状态是否属于关闭态（archived/cancelled），供其它包复用。
func IsProjectStatusClosed(status string) bool {
	return storage.IsProjectClosedStatus(status)
}

func isValidProjectStatusFilter(filter string) bool {
	switch filter {
	case "open", "planning", "active", "archived", "cancelled", "all":
		return true
	}
	return false
}

func projectStatusLabel(status string) string {
	switch storage.ProjectStatus(status) {
	case storage.ProjectStatusPlanning:
		return "预立项"
	case storage.ProjectStatusActive:
		return "立项在跑"
	case storage.ProjectStatusArchived:
		return "结束归档"
	case storage.ProjectStatusCancelled:
		return "取消"
	}
	return status
}

// TransitionProject 将项目转移到指定状态。任意状态间可自由转移（含 archived→active 重新激活）。
// 转移后自动追加一条项目变更注解，并触发 project.transitioned 事件与审计。
func (s *Service) TransitionProject(projectRef, toStatus string) (ProjectView, error) {
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectView{}, err
	}
	if !storage.IsValidProjectStatus(toStatus) {
		return ProjectView{}, RuntimeError{Code: "project_invalid_status", Message: fmt.Sprintf("invalid project status %q", toStatus)}
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectView{}, err
	}
	fromStatus := project.Status
	if fromStatus == toStatus {
		return ProjectView{}, RuntimeError{Code: "project_already_in_status", Message: fmt.Sprintf("project %q is already in status %q", project.Slug, toStatus)}
	}
	var result ProjectView
	err = s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		now := tx.clock.Unix()
		if err := tx.projectRepo.UpdateStatus(project.WorkspaceID, project.ID, toStatus, now); err != nil {
			return nil, nil, err
		}
		annotationContent := fmt.Sprintf("状态变更：%s → %s", projectStatusLabel(fromStatus), projectStatusLabel(toStatus))
		if _, err := tx.writeProjectAnnotation(project, annotationContent); err != nil {
			return nil, nil, err
		}
		updated, err := tx.projectRepo.GetByID(project.ID)
		if err != nil {
			return nil, nil, err
		}
		view, err := tx.projectViewForRow(updated)
		if err != nil {
			return nil, nil, err
		}
		result = view
		events := []HookEvent{buildProjectTransitionedHookEvent(view, fromStatus, toStatus, tx.runtime, now)}
		if toStatus == string(storage.ProjectStatusArchived) {
			events = append(events, buildProjectArchivedHookEvent(view, tx.runtime, now))
		}
		entry := AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
			Action:      "project.transition",
			Payload: map[string]any{
				"from_status": fromStatus,
				"to_status":   toStatus,
			},
		}
		return []AuditEntry{entry}, events, nil
	})
	return result, err
}
