package app

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
)

type RequestScope struct {
	TokenID      string
	TokenType    string
	WorkspaceIDs []string
	ProjectIDs   []string
	Capabilities []string
}

type RequestAuthorizationInput struct {
	Token              AuthenticatedToken
	RequiredCapability string
	RequiredPermission Permission
	WorkspaceRef       string
	ProjectRef         string
}

type AuthorizedRequest struct {
	Runtime   RuntimeContext
	Scope     RequestScope
	Workspace sqlite.Workspace
	Project   *sqlite.Project
}

func NewRequestScope(token TokenView) RequestScope {
	return RequestScope{
		TokenID:      token.ID,
		TokenType:    token.Type,
		WorkspaceIDs: append([]string(nil), token.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), token.ProjectIDs...),
		Capabilities: append([]string(nil), token.Scopes...),
	}
}

func (s RequestScope) HasCapability(capability string) bool {
	if strings.TrimSpace(capability) == "" {
		return true
	}
	return slices.Contains(s.Capabilities, capability)
}

func (s RequestScope) RestrictsWorkspaces() bool {
	return len(s.WorkspaceIDs) > 0
}

func (s RequestScope) RestrictsProjects() bool {
	return len(s.ProjectIDs) > 0
}

func (s RequestScope) AllowsWorkspace(id string) bool {
	if !s.RestrictsWorkspaces() {
		return true
	}
	return slices.Contains(s.WorkspaceIDs, id)
}

func (s RequestScope) AllowsProject(id string) bool {
	if !s.RestrictsProjects() {
		return true
	}
	return slices.Contains(s.ProjectIDs, id)
}

func (s RequestScope) projectFilterExpr() query.Expr {
	if !s.RestrictsProjects() {
		return nil
	}
	var expr query.Expr
	for _, projectID := range s.ProjectIDs {
		predicate := query.Predicate{
			Attribute: query.AttrProjectID,
			Operator:  query.OpEqual,
			Value:     query.StringValue(projectID),
		}
		if expr == nil {
			expr = predicate
			continue
		}
		expr = query.Or(expr, predicate)
	}
	return expr
}

func (s *Service) AuthorizeTokenRequest(input RequestAuthorizationInput) (AuthorizedRequest, error) {
	scope := NewRequestScope(input.Token.Token)
	if !scope.HasCapability(input.RequiredCapability) {
		return AuthorizedRequest{}, RuntimeError{Code: "token_scope_denied", Message: "token scope denied"}
	}

	workspace, err := s.resolveRequestWorkspace(input.Token.User, scope, input.WorkspaceRef)
	if err != nil {
		return AuthorizedRequest{}, err
	}
	member, err := s.memberRepo.Get(input.Token.User.ID, workspace.ID)
	if err == sqlite.ErrNotFound {
		return AuthorizedRequest{}, RuntimeError{Code: "membership_not_found", Message: "user is not a member of workspace"}
	}
	if err != nil {
		return AuthorizedRequest{}, err
	}
	project, err := s.resolveRequestProject(workspace.ID, scope, input.ProjectRef)
	if err != nil {
		return AuthorizedRequest{}, err
	}

	runtime := RuntimeContext{
		ActorUserID:   input.Token.User.ID,
		ActorName:     input.Token.User.Name,
		WorkspaceID:   workspace.ID,
		WorkspaceSlug: workspace.Slug,
		Role:          Role(member.Role),
	}
	if err := requireRolePermission(runtime.Role, input.RequiredPermission); err != nil {
		return AuthorizedRequest{}, err
	}
	return AuthorizedRequest{
		Runtime:   runtime,
		Scope:     scope,
		Workspace: workspace,
		Project:   project,
	}, nil
}

func (s *Service) resolveRequestWorkspace(user sqlite.User, scope RequestScope, ref string) (sqlite.Workspace, error) {
	ref = strings.TrimSpace(ref)
	if ref != "" {
		workspace, err := lookupWorkspace(s.workspaceRepo, ref)
		if err != nil {
			return sqlite.Workspace{}, err
		}
		if workspace.ArchivedAt != nil {
			return sqlite.Workspace{}, RuntimeError{Code: "workspace_archived", Message: "workspace is archived"}
		}
		if !scope.AllowsWorkspace(workspace.ID) {
			return sqlite.Workspace{}, RuntimeError{Code: "workspace_scope_denied", Message: "token cannot access workspace"}
		}
		return workspace, nil
	}
	if len(scope.WorkspaceIDs) == 1 {
		workspace, err := s.workspaceRepo.GetByID(scope.WorkspaceIDs[0])
		if err != nil {
			return sqlite.Workspace{}, err
		}
		if workspace.ArchivedAt != nil {
			return sqlite.Workspace{}, RuntimeError{Code: "workspace_archived", Message: "workspace is archived"}
		}
		return workspace, nil
	}
	if user.DefaultWorkspaceID == nil || strings.TrimSpace(*user.DefaultWorkspaceID) == "" {
		return sqlite.Workspace{}, RuntimeError{Code: "workspace_scope_denied", Message: "workspace must be specified"}
	}
	workspace, err := s.workspaceRepo.GetByID(*user.DefaultWorkspaceID)
	if err != nil {
		return sqlite.Workspace{}, err
	}
	if workspace.ArchivedAt != nil {
		return sqlite.Workspace{}, RuntimeError{Code: "workspace_archived", Message: "workspace is archived"}
	}
	if !scope.AllowsWorkspace(workspace.ID) {
		return sqlite.Workspace{}, RuntimeError{Code: "workspace_scope_denied", Message: "token cannot access default workspace; specify workspace"}
	}
	return workspace, nil
}

func (s *Service) resolveRequestProject(workspaceID string, scope RequestScope, ref string) (*sqlite.Project, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	project, err := s.ResolveProjectInWorkspace(workspaceID, ref)
	if err != nil {
		return nil, err
	}
	if !scope.AllowsProject(project.ID) {
		return nil, RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
	}
	return &project, nil
}

func (s *Service) hasProjectScope() bool {
	return s.requestScope != nil && s.requestScope.RestrictsProjects()
}

func (s *Service) projectScopeExpr() query.Expr {
	if s.requestScope == nil {
		return nil
	}
	return s.requestScope.projectFilterExpr()
}

func (s *Service) allowsProjectID(projectID *string) bool {
	if !s.hasProjectScope() {
		return true
	}
	if projectID == nil || strings.TrimSpace(*projectID) == "" {
		return false
	}
	return s.requestScope.AllowsProject(*projectID)
}

func (s *Service) ensureReadableTaskScope(tsk task.Task) error {
	if s.allowsProjectID(tsk.ProjectID) {
		return nil
	}
	return RuntimeError{Code: "task_not_found", Message: "task not found"}
}

func (s *Service) ensureWritableTaskScope(tsk task.Task) error {
	if s.allowsProjectID(tsk.ProjectID) {
		return nil
	}
	return RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
}

func (s *Service) ensureProjectScope(projectID *string) error {
	if s.allowsProjectID(projectID) {
		return nil
	}
	return RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
}

func (s *Service) resolveTargetForRead(target string) (task.Task, error) {
	if n, err := strconv.Atoi(target); err == nil && n >= 1 {
		tasks, err := s.defaultWorkingSet()
		if err != nil {
			return task.Task{}, err
		}
		if n > len(tasks) {
			return task.Task{}, taskNotFoundError()
		}
		return tasks[n-1], nil
	}
	tsk, err := s.repo.GetByUUID(s.workspaceID, target)
	if err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			return task.Task{}, taskNotFoundError()
		}
		return task.Task{}, err
	}
	if err := s.ensureReadableTaskScope(tsk); err != nil {
		return task.Task{}, err
	}
	return tsk, nil
}

func (s *Service) resolveTargetForWrite(target string) (task.Task, error) {
	if n, err := strconv.Atoi(target); err == nil && n >= 1 {
		tasks, err := s.defaultWorkingSet()
		if err != nil {
			return task.Task{}, err
		}
		if n > len(tasks) {
			return task.Task{}, taskNotFoundError()
		}
		return tasks[n-1], nil
	}
	tsk, err := s.repo.GetByUUID(s.workspaceID, target)
	if err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			return task.Task{}, taskNotFoundError()
		}
		return task.Task{}, err
	}
	if err := s.ensureWritableTaskScope(tsk); err != nil {
		return task.Task{}, err
	}
	return tsk, nil
}

func taskNotFoundError() RuntimeError {
	return RuntimeError{Code: "task_not_found", Message: "task not found"}
}

func filterProjectsByScope(scope *RequestScope, projects []ProjectView) []ProjectView {
	if scope == nil || !scope.RestrictsProjects() {
		return projects
	}
	filtered := make([]ProjectView, 0, len(projects))
	for _, project := range projects {
		if scope.AllowsProject(project.ID) {
			filtered = append(filtered, project)
		}
	}
	return filtered
}

func filterAuditByScope(scope *RequestScope, rows []AuditLogView) []AuditLogView {
	if scope == nil || !scope.RestrictsProjects() {
		return rows
	}
	filtered := make([]AuditLogView, 0, len(rows))
	for _, row := range rows {
		if row.ProjectID != nil && scope.AllowsProject(*row.ProjectID) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func cloneRequestScope(scope *RequestScope) *RequestScope {
	if scope == nil {
		return nil
	}
	cloned := *scope
	cloned.WorkspaceIDs = append([]string(nil), scope.WorkspaceIDs...)
	cloned.ProjectIDs = append([]string(nil), scope.ProjectIDs...)
	cloned.Capabilities = append([]string(nil), scope.Capabilities...)
	return &cloned
}
