package app

import (
	"errors"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// RequestScope 复用 authz.RequestScope，保持 app 层现有 API 稳定。
type RequestScope = authz.RequestScope

type RequestAuthorizationInput struct {
	Token              AuthenticatedToken
	RequiredCapability string
	RequiredPermission Permission
	WorkspaceRef       string
	ProjectRef         string
	ProjectRefIsID     bool
	SubjectUserRef     string
}

// AuthorizedRequest 是授权入口的输出。
//
// Scope 与 Decision.RequestScope 是同一个 effectiveScope 的两份引用：
// Scope 保留是为了兼容历史调用点（主要是测试），新代码应优先使用 Decision。
type AuthorizedRequest struct {
	Runtime   RuntimeContext
	Scope     RequestScope
	Workspace storage.Workspace
	Project   *storage.Project
	Decision  authz.Decision
}

type ResolveMode int

const (
	ResolveInteractive ResolveMode = iota
	ResolveProtocol
)

func NewRequestScope(token TokenView) RequestScope {
	return RequestScope{
		TokenID:      token.ID,
		TokenType:    token.Type,
		WorkspaceIDs: append([]string(nil), token.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), token.ProjectIDs...),
		Capabilities: append([]string(nil), token.Scopes...),
	}
}

func (s *Service) HasRequestCapability(capability string) bool {
	return s.requestScope != nil && s.requestScope.HasCapability(capability)
}

// requestScopeProjectFilterExpr 把 RequestScope 的 project allowlist
// 转换为 query.Expr。因为 RequestScope 现在是导入的类型别名，
// 不能直接给它加方法，所以使用自由函数。
func requestScopeProjectFilterExpr(scope *RequestScope) query.Expr {
	if scope == nil || !scope.RestrictsProjects() {
		return nil
	}
	var expr query.Expr
	for _, projectID := range scope.ProjectIDs {
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
		return AuthorizedRequest{}, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "token scope denied"}
	}
	subjectUserRef := strings.TrimSpace(input.SubjectUserRef)
	if subjectUserRef != "" {
		// acting token 是浏览器短期委托凭证，不允许与 X-Xuanchu-As 组合做 impersonation。
		if input.Token.Token.Type == auth.TokenTypeAdminActing {
			return AuthorizedRequest{}, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "acting token cannot impersonate"}
		}
		if input.Token.Token.Type != auth.TokenTypeAgent {
			return AuthorizedRequest{}, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "impersonation requires agent token"}
		}
		if !scope.HasCapability(auth.ScopeImpersonate) {
			return AuthorizedRequest{}, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "token does not have impersonate scope"}
		}
	}

	workspaceRef, projectRef := strings.TrimSpace(input.WorkspaceRef), strings.TrimSpace(input.ProjectRef)
	if input.ProjectRefIsID && projectRef != "" {
		project, err := s.projectRepo.GetByID(projectRef)
		if errors.Is(err, storage.ErrNotFound) {
			return AuthorizedRequest{}, RuntimeError{Code: "project_not_found", Message: "project not found"}
		}
		if err != nil {
			return AuthorizedRequest{}, err
		}
		if workspaceRef != "" {
			workspace, err := lookupWorkspace(s.workspaceRepo, workspaceRef)
			if err != nil {
				return AuthorizedRequest{}, err
			}
			if project.WorkspaceID != workspace.ID {
				return AuthorizedRequest{}, RuntimeError{Code: "project_workspace_mismatch", Message: "project does not belong to workspace"}
			}
		}
		workspaceRef = project.WorkspaceID
	}

	if input.Token.TenantActor || input.Token.Token.Type == auth.TokenTypeTenantAccess {
		return s.authorizeTenantTokenRequest(input, scope, workspaceRef, projectRef)
	}

	tokenUser := input.Token.User
	workspace, err := s.resolveRequestWorkspace(tokenUser, scope, workspaceRef)
	if err != nil {
		return AuthorizedRequest{}, err
	}

	var subjectUser storage.User
	var delegatorTokenID string
	var delegatorUserID string
	if subjectUserRef != "" {
		if workspaceRef == "" && scope.RestrictsWorkspaces() && len(scope.WorkspaceIDs) > 1 {
			return AuthorizedRequest{}, RuntimeError{Code: authz.CodeWorkspaceRequired, Message: "workspace must be specified for impersonation with multiple visible workspaces"}
		}
		subjectUser, err = s.resolveUser(subjectUserRef)
		if err != nil {
			return AuthorizedRequest{}, RuntimeError{Code: authz.CodeMembershipNotFound, Message: "impersonation target user not found"}
		}
		_, err = s.memberRepo.Get(subjectUser.ID, workspace.ID)
		if err == storage.ErrNotFound {
			return AuthorizedRequest{}, RuntimeError{Code: authz.CodeMembershipNotFound, Message: "impersonation target is not a member of workspace"}
		}
		if err != nil {
			return AuthorizedRequest{}, err
		}
		delegatorTokenID = input.Token.Token.ID
		delegatorUserID = tokenUser.ID
	} else {
		subjectUser = tokenUser
	}

	member, err := s.memberRepo.Get(subjectUser.ID, workspace.ID)
	if err == storage.ErrNotFound {
		return AuthorizedRequest{}, RuntimeError{Code: authz.CodeMembershipNotFound, Message: "user is not a member of workspace"}
	}
	if err != nil {
		return AuthorizedRequest{}, err
	}
	project, err := s.resolveRequestProject(workspace.ID, scope, projectRef)
	if err != nil {
		return AuthorizedRequest{}, err
	}
	effectiveScope := scope
	if project != nil {
		effectiveScope.ProjectIDs = []string{project.ID}
	}

	decision := authz.Decision{
		Actor: authz.Actor{
			Type:     authz.ActorUser,
			UserID:   subjectUser.ID,
			UserName: subjectUser.Name,
		},
		Principal: authz.Principal{
			UserID:   subjectUser.ID,
			UserName: subjectUser.Name,
		},
		Credential: authz.Credential{
			Kind:         credentialKindFromTokenType(input.Token.Token.Type),
			TokenID:      input.Token.Token.ID,
			TokenUserID:  tokenUser.ID,
			Capabilities: append([]string(nil), scope.Capabilities...),
			WorkspaceIDs: append([]string(nil), scope.WorkspaceIDs...),
			ProjectIDs:   append([]string(nil), scope.ProjectIDs...),
		},
		Tenant: authz.TenantScope{
			WorkspaceID:   workspace.ID,
			WorkspaceSlug: workspace.Slug,
		},
		Role:         Role(member.Role),
		RequestScope: effectiveScope,
	}
	if project != nil {
		projectID := project.ID
		decision.Tenant.ProjectID = &projectID
	}
	if delegatorTokenID != "" {
		decision.Delegator = &authz.Delegator{UserID: delegatorUserID, TokenID: delegatorTokenID}
	}

	runtime := runtimeContextFromDecision(decision)
	// acting token 的 server admin 委托链需要透传到 audit，与普通 user-agent impersonation 独立。
	if input.Token.AdminActingTrace != nil {
		runtime.AdminActingSessionID = input.Token.AdminActingTrace.SessionID
		runtime.DelegatorAdminTokenID = derefString(input.Token.AdminActingTrace.DelegatorAdminTokenID)
		runtime.DelegatorAdminTokenName = input.Token.AdminActingTrace.DelegatorAdminTokenName
	}
	if err := requireRolePermission(runtime.Role, input.RequiredPermission); err != nil {
		return AuthorizedRequest{}, err
	}
	return AuthorizedRequest{
		Runtime:   runtime,
		Scope:     effectiveScope,
		Workspace: workspace,
		Project:   project,
		Decision:  decision,
	}, nil
}

func (s *Service) authorizeTenantTokenRequest(input RequestAuthorizationInput, scope RequestScope, workspaceRef, projectRef string) (AuthorizedRequest, error) {
	workspace, err := s.resolveTenantRequestWorkspace(scope, workspaceRef)
	if err != nil {
		return AuthorizedRequest{}, err
	}
	project, err := s.resolveRequestProject(workspace.ID, scope, projectRef)
	if err != nil {
		return AuthorizedRequest{}, err
	}
	effectiveScope := scope
	if project != nil {
		effectiveScope.ProjectIDs = []string{project.ID}
	}
	decision := authz.Decision{
		Actor: authz.Actor{
			Type:        authz.ActorTenantAccessToken,
			TokenID:     input.Token.Token.ID,
			TokenName:   input.Token.Token.Name,
			TokenPrefix: input.Token.Token.Prefix,
		},
		Credential: authz.Credential{
			Kind:         authz.CredentialTenantAccess,
			TokenID:      input.Token.Token.ID,
			Capabilities: append([]string(nil), scope.Capabilities...),
			WorkspaceIDs: append([]string(nil), scope.WorkspaceIDs...),
			ProjectIDs:   append([]string(nil), scope.ProjectIDs...),
		},
		Tenant: authz.TenantScope{
			WorkspaceID:   workspace.ID,
			WorkspaceSlug: workspace.Slug,
		},
		RequestScope: effectiveScope,
	}
	if project != nil {
		projectID := project.ID
		decision.Tenant.ProjectID = &projectID
	}
	runtime := RuntimeContext{
		ActorType:        auth.TokenTypeTenantAccess,
		ActorName:        input.Token.Token.Name,
		ActorTokenID:     input.Token.Token.ID,
		ActorTokenName:   input.Token.Token.Name,
		ActorTokenPrefix: input.Token.Token.Prefix,
		WorkspaceID:      workspace.ID,
		WorkspaceSlug:    workspace.Slug,
	}
	return AuthorizedRequest{
		Runtime:   runtime,
		Scope:     effectiveScope,
		Workspace: workspace,
		Project:   project,
		Decision:  decision,
	}, nil
}

// credentialKindFromTokenType 把存储层 token type 映射为 authz.CredentialKind。
func credentialKindFromTokenType(tokenType string) authz.CredentialKind {
	switch tokenType {
	case auth.TokenTypeAgent:
		return authz.CredentialAgent
	case auth.TokenTypePAT:
		return authz.CredentialPAT
	case auth.TokenTypeTenantAccess:
		return authz.CredentialTenantAccess
	default:
		return authz.CredentialKind(tokenType)
	}
}

// runtimeContextFromDecision 由授权决策生成运行时上下文。
// Decision 是授权边界的统一输出，RuntimeContext 是 app service 内部执行所需的派生形态。
func runtimeContextFromDecision(decision authz.Decision) RuntimeContext {
	rt := RuntimeContext{
		ActorType:     string(authz.ActorUser),
		ActorUserID:   decision.Principal.UserID,
		ActorName:     decision.Principal.UserName,
		WorkspaceID:   decision.Tenant.WorkspaceID,
		WorkspaceSlug: decision.Tenant.WorkspaceSlug,
		Role:          decision.Role,
	}
	if decision.Delegator != nil {
		rt.DelegatorUserID = decision.Delegator.UserID
		rt.DelegatorTokenID = decision.Delegator.TokenID
	}
	return rt
}

func (s *Service) resolveTenantRequestWorkspace(scope RequestScope, ref string) (storage.Workspace, error) {
	ref = strings.TrimSpace(ref)
	if ref != "" {
		workspace, err := lookupWorkspace(s.workspaceRepo, ref)
		if err != nil {
			return storage.Workspace{}, err
		}
		if workspace.ArchivedAt != nil {
			return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
		}
		if !scope.AllowsWorkspace(workspace.ID) {
			return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "token cannot access workspace"}
		}
		return workspace, nil
	}
	if len(scope.WorkspaceIDs) != 1 {
		return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceRequired, Message: "workspace must be specified"}
	}
	workspace, err := s.workspaceRepo.GetByID(scope.WorkspaceIDs[0])
	if err != nil {
		return storage.Workspace{}, err
	}
	if workspace.ArchivedAt != nil {
		return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
	}
	return workspace, nil
}

func (s *Service) resolveRequestWorkspace(user storage.User, scope RequestScope, ref string) (storage.Workspace, error) {
	ref = strings.TrimSpace(ref)
	if ref != "" {
		workspace, err := lookupWorkspace(s.workspaceRepo, ref)
		if err != nil {
			return storage.Workspace{}, err
		}
		if workspace.ArchivedAt != nil {
			return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
		}
		if !scope.AllowsWorkspace(workspace.ID) {
			return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "token cannot access workspace"}
		}
		return workspace, nil
	}
	if len(scope.WorkspaceIDs) == 1 {
		workspace, err := s.workspaceRepo.GetByID(scope.WorkspaceIDs[0])
		if err != nil {
			return storage.Workspace{}, err
		}
		if workspace.ArchivedAt != nil {
			return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
		}
		return workspace, nil
	}
	if user.DefaultWorkspaceID == nil || strings.TrimSpace(*user.DefaultWorkspaceID) == "" {
		return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "workspace must be specified"}
	}
	workspace, err := s.workspaceRepo.GetByID(*user.DefaultWorkspaceID)
	if err != nil {
		return storage.Workspace{}, err
	}
	if workspace.ArchivedAt != nil {
		return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
	}
	if !scope.AllowsWorkspace(workspace.ID) {
		return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "token cannot access default workspace; specify workspace"}
	}
	return workspace, nil
}

func (s *Service) resolveRequestProject(workspaceID string, scope RequestScope, ref string) (*storage.Project, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	project, err := s.ResolveProjectInWorkspace(workspaceID, ref)
	if err != nil {
		return nil, err
	}
	if !scope.AllowsProject(project.ID) {
		return nil, RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot access project"}
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
	return requestScopeProjectFilterExpr(s.requestScope)
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
	return RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot access project"}
}

func (s *Service) ensureProjectScope(projectID *string) error {
	if s.allowsProjectID(projectID) {
		return nil
	}
	return RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot access project"}
}

func (s *Service) resolveTargetForRead(target string) (task.Task, error) {
	return s.resolveTaskRef(target, ResolveInteractive, false)
}

func (s *Service) resolveTargetForWrite(target string) (task.Task, error) {
	return s.resolveTaskRef(target, ResolveInteractive, true)
}

func (s *Service) ResolveProtocolTarget(target string) (task.Task, error) {
	return s.resolveTaskRef(target, ResolveProtocol, false)
}

func (s *Service) ResolveProtocolTargetForWrite(target string) (task.Task, error) {
	return s.resolveTaskRef(target, ResolveProtocol, true)
}

func (s *Service) resolveTaskRef(target string, mode ResolveMode, write bool) (task.Task, error) {
	target = strings.TrimSpace(target)
	if mode == ResolveProtocol && isDecimalDigits(target) {
		return task.Task{}, RuntimeError{Code: "task_ref_invalid", Message: "numeric task refs are not accepted by this endpoint"}
	}
	if n, ok := parseInteractiveNumericTaskRef(target); ok {
		tasks, err := s.defaultWorkingSet()
		if err != nil {
			return task.Task{}, err
		}
		if n > len(tasks) {
			return task.Task{}, taskNotFoundError()
		}
		tsk := tasks[n-1]
		if err := s.validateTaskProjectInvariant(tsk); err != nil {
			return task.Task{}, err
		}
		return tsk, nil
	}
	var (
		tsk task.Task
		err error
	)
	tsk, err = s.repo.GetByUUID(s.workspaceID, target)
	if errors.Is(err, storage.ErrNotFound) {
		tsk, err = s.resolveTaskSlug(target)
	}
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return task.Task{}, taskNotFoundError()
		}
		return task.Task{}, err
	}
	if write {
		if err := s.ensureWritableTaskScope(tsk); err != nil {
			return task.Task{}, err
		}
	} else {
		if err := s.ensureReadableTaskScope(tsk); err != nil {
			return task.Task{}, err
		}
	}
	if err := s.validateTaskProjectInvariant(tsk); err != nil {
		return task.Task{}, err
	}
	return tsk, nil
}

func (s *Service) resolveTaskSlug(ref string) (task.Task, error) {
	left, right, ok := strings.Cut(ref, "-")
	if !ok {
		return task.Task{}, storage.ErrNotFound
	}
	if last := strings.LastIndex(ref, "-"); last >= 0 {
		left = ref[:last]
		right = ref[last+1:]
	}
	seq, err := strconv.ParseInt(right, 10, 64)
	if err != nil || seq < 1 {
		return task.Task{}, storage.ErrNotFound
	}
	projectSlug, err := normalizeProjectSlug(left)
	if err != nil {
		return task.Task{}, storage.ErrNotFound
	}
	project, err := s.projectRepo.GetBySlug(s.workspaceID, projectSlug)
	if err != nil {
		return task.Task{}, err
	}
	return s.repo.GetByProjectSeq(s.workspaceID, project.ID, seq)
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

func parseInteractiveNumericTaskRef(target string) (int, bool) {
	if !isDecimalDigits(target) {
		return 0, false
	}
	n, err := strconv.Atoi(target)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func isDecimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
