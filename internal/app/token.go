package app

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type CreateTokenInput struct {
	Name          string
	Type          string
	UserRef       string
	Scopes        []string
	WorkspaceRefs []string
	ProjectRefs   []string
	ExpiresIn     *time.Duration
	ParentToken   *TokenView
}

type ListTokensInput struct {
	UserRef        string
	IncludeRevoked bool
}

type TokenView struct {
	ID           string
	Prefix       string
	Name         string
	Type         string
	User         task.UserInfo
	WorkspaceIDs []string
	ProjectIDs   []string
	Scopes       []string
	CreatedAt    int64
	ExpiresAt    *int64
	RevokedAt    *int64
	LastUsedAt   *int64
}

type CreatedToken struct {
	RawToken string
	View     TokenView
	Stored   storage.ApiTokenEntry
}

type AuthenticatedToken struct {
	Token TokenView
	User  storage.User
	// AdminActingTrace 在 acting token 鉴权时填充，用于把 server admin 来源
	// 透传到后续授权和审计。普通 PAT/Agent token 为 nil。
	AdminActingTrace *AdminActingTrace
}

// AdminActingTrace 携带 acting session 的委托链信息，授权时拷贝到 RuntimeContext。
type AdminActingTrace struct {
	SessionID               string
	DelegatorAdminTokenID   *string
	DelegatorAdminTokenName string
}

type createTokenStoredInput struct {
	Name         string
	TokenType    string
	UserID       string
	Scopes       []string
	WorkspaceIDs []string
	ProjectIDs   []string
	ExpiresIn    *time.Duration
}

func (s *Service) CreateToken(input CreateTokenInput) (CreatedToken, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CreatedToken{}, RuntimeError{Code: "token_name_required", Message: "token name is required"}
	}
	tokenType := strings.TrimSpace(input.Type)
	if tokenType == "" {
		tokenType = auth.TokenTypePAT
	}
	targetUser, err := s.resolveTokenTargetUser(input.UserRef)
	if err != nil {
		return CreatedToken{}, err
	}
	workspaces, err := s.resolveTokenWorkspaces(input.WorkspaceRefs)
	if err != nil {
		return CreatedToken{}, err
	}
	projects, err := s.resolveTokenProjects(workspaces, input.ProjectRefs)
	if err != nil {
		return CreatedToken{}, err
	}
	workspaceIDs := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
	}
	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ID)
	}
	scopes, err := auth.ValidateTokenCreate(auth.CreateTokenOptions{
		Type:         tokenType,
		Scopes:       input.Scopes,
		WorkspaceIDs: workspaceIDs,
	})
	if err != nil {
		return CreatedToken{}, classifyTokenCreateError(err)
	}
	if scopes.Has(auth.ScopeImpersonate) && !tokenManageAllowed(s.runtime.Role) {
		return CreatedToken{}, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "only admin or owner can create tokens with impersonate scope"}
	}
	if err := enforceTokenCreateLimit(input.ParentToken, scopes.Values(), workspaceIDs, projectIDs); err != nil {
		return CreatedToken{}, err
	}

	var created CreatedToken
	err = s.withAudit("token.create", func(tx *Service) (AuditEntry, error) {
		storedToken, err := tx.createTokenStored(createTokenStoredInput{
			Name:         name,
			TokenType:    tokenType,
			UserID:       targetUser.ID,
			Scopes:       scopes.Values(),
			WorkspaceIDs: workspaceIDs,
			ProjectIDs:   projectIDs,
			ExpiresIn:    input.ExpiresIn,
		})
		if err != nil {
			return AuditEntry{}, err
		}
		created = storedToken
		return AuditEntry{
			TargetType: "token",
			TargetID:   created.Stored.ID,
			Payload: map[string]any{
				"name":          created.Stored.Name,
				"type":          created.Stored.Type,
				"user_id":       created.Stored.UserID,
				"workspace_ids": workspaceIDs,
				"project_ids":   projectIDs,
				"scopes":        scopes.Values(),
				"expires_at":    created.Stored.ExpiresAt,
			},
		}, nil
	})
	return created, err
}

func (s *Service) createTokenStored(input createTokenStoredInput) (CreatedToken, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CreatedToken{}, RuntimeError{Code: "token_name_required", Message: "token name is required"}
	}
	tokenType := strings.TrimSpace(input.TokenType)
	if tokenType == "" {
		tokenType = auth.TokenTypePAT
	}
	scopes, err := auth.ValidateTokenCreate(auth.CreateTokenOptions{
		Type:         tokenType,
		Scopes:       input.Scopes,
		WorkspaceIDs: input.WorkspaceIDs,
	})
	if err != nil {
		return CreatedToken{}, classifyTokenCreateError(err)
	}
	raw, prefix, hash, err := auth.GenerateToken(tokenType)
	if err != nil {
		return CreatedToken{}, err
	}
	createdAt := s.clock.Unix()
	var expiresAt *int64
	if input.ExpiresIn != nil {
		value := createdAt + int64(input.ExpiresIn.Seconds())
		expiresAt = &value
	}
	scopesJSON, err := marshalStringSlice(scopes.Values())
	if err != nil {
		return CreatedToken{}, err
	}
	workspaceJSON, err := marshalStringSlice(input.WorkspaceIDs)
	if err != nil {
		return CreatedToken{}, err
	}
	projectJSON, err := marshalStringSlice(input.ProjectIDs)
	if err != nil {
		return CreatedToken{}, err
	}
	stored := storage.ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           input.UserID,
		Name:             name,
		Type:             tokenType,
		TokenPrefix:      prefix,
		TokenHash:        hash,
		ScopesJSON:       scopesJSON,
		WorkspaceIDsJSON: workspaceJSON,
		ProjectIDsJSON:   projectJSON,
		CreatedAt:        createdAt,
		ExpiresAt:        expiresAt,
	}
	if err := s.tokenRepo.Create(stored); err != nil {
		return CreatedToken{}, err
	}
	userInfo := task.UserInfo{ID: input.UserID}
	if user, err := s.userRepo.GetByID(input.UserID); err == nil {
		userInfo = task.UserInfo{ID: user.ID, Name: user.Name, DisplayName: user.DisplayName, Email: user.Email}
	}
	view := tokenViewFromEntry(stored, scopes.Values(), input.WorkspaceIDs, input.ProjectIDs)
	view.User = userInfo
	return CreatedToken{
		RawToken: raw,
		View:     view,
		Stored:   stored,
	}, nil
}

func (s *Service) ListTokens(input ListTokensInput) ([]TokenView, error) {
	targetUser, err := s.resolveTokenListUser(input.UserRef)
	if err != nil {
		return nil, err
	}
	rows, err := s.tokenRepo.ListByUser(targetUser.ID, input.IncludeRevoked)
	if err != nil {
		return nil, err
	}
	out := make([]TokenView, 0, len(rows))
	for _, row := range rows {
		scopes, err := unmarshalStringSlice(row.ScopesJSON)
		if err != nil {
			return nil, err
		}
		workspaceIDs, err := unmarshalStringSlice(row.WorkspaceIDsJSON)
		if err != nil {
			return nil, err
		}
		projectIDs, err := unmarshalStringSlice(row.ProjectIDsJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, tokenViewFromEntry(row, scopes, workspaceIDs, projectIDs))
	}
	return out, nil
}

func (s *Service) RevokeToken(ref string) error {
	return s.revokeToken(ref, nil)
}

func (s *Service) RevokeTokenWithLimit(ref string, limit *TokenView) error {
	return s.revokeToken(ref, limit)
}

func (s *Service) revokeToken(ref string, limit *TokenView) error {
	entry, err := s.tokenRepo.GetByIDOrPrefix(strings.TrimSpace(ref))
	if err != nil {
		return err
	}
	if entry.UserID != s.runtime.ActorUserID && !tokenManageAllowed(s.runtime.Role) {
		return PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
	}
	if err := enforceTokenRevokeLimit(limit, entry); err != nil {
		return err
	}
	return s.withAudit("token.revoke", func(tx *Service) (AuditEntry, error) {
		if err := tx.tokenRepo.Revoke(entry.ID, tx.clock.Unix()); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "token",
			TargetID:   entry.ID,
			Payload: map[string]any{
				"user_id": entry.UserID,
				"name":    entry.Name,
			},
		}, nil
	})
}

func enforceTokenCreateLimit(parent *TokenView, scopes, workspaceIDs, projectIDs []string) error {
	if parent == nil {
		return nil
	}
	for _, scope := range scopes {
		if !slices.Contains(parent.Scopes, scope) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "new token scope exceeds current token"}
		}
	}
	if err := requireSubsetWhenRestricted(parent.WorkspaceIDs, workspaceIDs, authz.CodeWorkspaceScopeDenied, "new token workspace scope exceeds current token"); err != nil {
		return err
	}
	return requireSubsetWhenRestricted(parent.ProjectIDs, projectIDs, authz.CodeProjectScopeDenied, "new token project scope exceeds current token")
}

func enforceTokenRevokeLimit(parent *TokenView, entry storage.ApiTokenEntry) error {
	if parent == nil {
		return nil
	}
	workspaceIDs, err := unmarshalStringSlice(entry.WorkspaceIDsJSON)
	if err != nil {
		return err
	}
	projectIDs, err := unmarshalStringSlice(entry.ProjectIDsJSON)
	if err != nil {
		return err
	}
	if err := requireSubsetWhenRestricted(parent.WorkspaceIDs, workspaceIDs, authz.CodeWorkspaceScopeDenied, "target token workspace scope is outside current token"); err != nil {
		return err
	}
	return requireSubsetWhenRestricted(parent.ProjectIDs, projectIDs, authz.CodeProjectScopeDenied, "target token project scope is outside current token")
}

func requireSubsetWhenRestricted(parent, child []string, code, message string) error {
	if len(parent) == 0 {
		return nil
	}
	if len(child) == 0 {
		return RuntimeError{Code: code, Message: message}
	}
	for _, id := range child {
		if !slices.Contains(parent, id) {
			return RuntimeError{Code: code, Message: message}
		}
	}
	return nil
}

func (s *Service) AuthenticateBearerToken(raw string) (AuthenticatedToken, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
	}
	if strings.HasPrefix(raw, auth.ActingTokenPrefix) {
		return s.authenticateActingToken(raw)
	}
	prefix := raw
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	row, err := s.tokenRepo.GetByPrefix(prefix)
	if err == storage.ErrNotFound {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
	}
	if err != nil {
		return AuthenticatedToken{}, err
	}
	if !auth.VerifyTokenHash(raw, row.TokenHash) {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
	}
	now := s.clock.Unix()
	if row.RevokedAt != nil {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthTokenRevoked, Message: "token revoked"}
	}
	if row.ExpiresAt != nil && now > *row.ExpiresAt {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthTokenExpired, Message: "token expired"}
	}
	user, err := s.userRepo.GetByID(row.UserID)
	if err != nil {
		return AuthenticatedToken{}, err
	}
	scopes, err := unmarshalStringSlice(row.ScopesJSON)
	if err != nil {
		return AuthenticatedToken{}, err
	}
	workspaceIDs, err := unmarshalStringSlice(row.WorkspaceIDsJSON)
	if err != nil {
		return AuthenticatedToken{}, err
	}
	projectIDs, err := unmarshalStringSlice(row.ProjectIDsJSON)
	if err != nil {
		return AuthenticatedToken{}, err
	}
	if err := s.tokenRepo.TouchLastUsed(row.ID, now); err == nil {
		row.LastUsedAt = &now
	}
	return AuthenticatedToken{
		Token: tokenViewFromEntry(row, scopes, workspaceIDs, projectIDs),
		User:  user,
	}, nil
}

// authenticateActingToken 解析 xuanchu_act_ 前缀的短期 acting token。
// 与普通 token 的差异：走 admin_acting_sessions；拒绝 revoked/expired；
// workspace scope 只绑定 session 中的单一 workspace；type 为 admin_acting；
// 携带 AdminActingTrace 供后续授权把 server admin 来源写入 audit。
// 注意：不信任 session.Role 快照；实际授权由 AuthorizeTokenRequest 用当前 membership role 决定。
func (s *Service) authenticateActingToken(raw string) (AuthenticatedToken, error) {
	prefix := raw
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	session, err := s.adminActingSessionRepo.GetByPrefix(prefix)
	if err == storage.ErrNotFound {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
	}
	if err != nil {
		return AuthenticatedToken{}, err
	}
	if !auth.VerifyActingToken(raw, session.TokenHash) {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
	}
	now := s.clock.Unix()
	if session.RevokedAt != nil {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "acting session revoked"}
	}
	if session.ExpiresAt <= now {
		return AuthenticatedToken{}, RuntimeError{Code: "admin_acting_session_expired", Message: "acting session expired"}
	}
	user, err := s.userRepo.GetByID(session.ActorUserID)
	if err != nil {
		if err == storage.ErrNotFound {
			return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
		}
		return AuthenticatedToken{}, err
	}
	// acting token 的能力上限 = HTTP console 的全部 scope（剥离 impersonate）。
	scopes := auth.ScopeRegistryValues()
	filtered := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == auth.ScopeImpersonate {
			continue
		}
		filtered = append(filtered, scope)
	}
	_ = s.adminActingSessionRepo.TouchLastUsed(session.ID, now)
	view := TokenView{
		ID:           session.ID,
		Prefix:       session.TokenPrefix,
		Name:         "admin-acting-" + session.AdminTokenName,
		Type:         auth.TokenTypeAdminActing,
		User:         task.UserInfo{ID: user.ID},
		WorkspaceIDs: []string{session.WorkspaceID},
		ProjectIDs:   nil,
		Scopes:       filtered,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    &session.ExpiresAt,
		LastUsedAt:   ptrInt64From(session.LastUsedAt, now),
	}
	return AuthenticatedToken{
		Token: view,
		User:  user,
		AdminActingTrace: &AdminActingTrace{
			SessionID:               session.ID,
			DelegatorAdminTokenID:   session.AdminTokenID,
			DelegatorAdminTokenName: session.AdminTokenName,
		},
	}, nil
}

func ptrInt64From(value *int64, fallback int64) *int64 {
	if value != nil {
		return value
	}
	return &fallback
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) resolveTokenTargetUser(ref string) (storage.User, error) {
	if strings.TrimSpace(ref) == "" {
		return s.userRepo.GetByID(s.runtime.ActorUserID)
	}
	user, err := s.resolveUser(ref)
	if err != nil {
		return storage.User{}, err
	}
	if user.ID != s.runtime.ActorUserID && !tokenManageAllowed(s.runtime.Role) {
		return storage.User{}, PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
	}
	return user, nil
}

func (s *Service) resolveTokenListUser(ref string) (storage.User, error) {
	if strings.TrimSpace(ref) == "" {
		return s.userRepo.GetByID(s.runtime.ActorUserID)
	}
	user, err := s.resolveUser(ref)
	if err != nil {
		return storage.User{}, err
	}
	if user.ID != s.runtime.ActorUserID && !tokenManageAllowed(s.runtime.Role) {
		return storage.User{}, PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
	}
	return user, nil
}

func (s *Service) resolveTokenWorkspaces(refs []string) ([]storage.Workspace, error) {
	out := make([]storage.Workspace, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		workspace, role, err := s.resolveWorkspaceForActor(ref)
		if err != nil {
			return nil, err
		}
		if err := requireRolePermission(role, PermissionTokenWrite); err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(out, func(item storage.Workspace) bool { return item.ID == workspace.ID }) {
			out = append(out, workspace)
		}
	}
	return out, nil
}

func (s *Service) resolveTokenProjects(workspaces []storage.Workspace, refs []string) ([]storage.Project, error) {
	allowedWorkspaceIDs := map[string]struct{}{}
	for _, workspace := range workspaces {
		allowedWorkspaceIDs[workspace.ID] = struct{}{}
	}
	out := make([]storage.Project, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		project, err := s.resolveTokenProject(ref, workspaces)
		if err != nil {
			return nil, err
		}
		if len(allowedWorkspaceIDs) > 0 {
			if _, ok := allowedWorkspaceIDs[project.WorkspaceID]; !ok {
				return nil, RuntimeError{Code: "token_project_scope_invalid", Message: "invalid token project scope"}
			}
		}
		if !slices.ContainsFunc(out, func(item storage.Project) bool { return item.ID == project.ID }) {
			out = append(out, project)
		}
	}
	return out, nil
}

func (s *Service) resolveTokenProject(ref string, workspaces []storage.Workspace) (storage.Project, error) {
	for _, workspace := range workspaces {
		project, err := s.projectRepo.ResolveInWorkspace(workspace.ID, ref)
		if err == nil {
			return project, nil
		}
		if err != storage.ErrNotFound {
			return storage.Project{}, err
		}
	}
	if project, err := s.projectRepo.GetByID(ref); err == nil {
		_, role, err := s.resolveWorkspaceForActor(project.WorkspaceID)
		if err != nil {
			return storage.Project{}, err
		}
		if err := requireRolePermission(role, PermissionTokenWrite); err != nil {
			return storage.Project{}, err
		}
		return project, nil
	}
	project, err := s.ResolveProject(ref)
	if err != nil {
		return storage.Project{}, RuntimeError{Code: "token_project_scope_invalid", Message: "invalid token project scope"}
	}
	return project, nil
}

func tokenViewFromEntry(row storage.ApiTokenEntry, scopes, workspaceIDs, projectIDs []string) TokenView {
	return TokenView{
		ID:           row.ID,
		Prefix:       row.TokenPrefix,
		Name:         row.Name,
		Type:         row.Type,
		User:         task.UserInfo{ID: row.UserID},
		WorkspaceIDs: append([]string(nil), workspaceIDs...),
		ProjectIDs:   append([]string(nil), projectIDs...),
		Scopes:       append([]string(nil), scopes...),
		CreatedAt:    row.CreatedAt,
		ExpiresAt:    row.ExpiresAt,
		RevokedAt:    row.RevokedAt,
		LastUsedAt:   row.LastUsedAt,
	}
}

func marshalStringSlice(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalStringSlice(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func classifyTokenCreateError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "workspace"):
		return RuntimeError{Code: "token_workspace_scope_invalid", Message: "invalid token workspace scope"}
	case strings.Contains(message, "scope"):
		return RuntimeError{Code: "token_scope_invalid", Message: "invalid token scope"}
	default:
		return RuntimeError{Code: "token_scope_invalid", Message: message}
	}
}

func tokenManageAllowed(role Role) bool {
	return role == RoleOwner || role == RoleAdmin
}

type ModifyTokenInput struct {
	TokenID       string
	Name          *string
	Scopes        *[]string
	WorkspaceRefs *[]string
	ProjectRefs   *[]string
	ExpiresIn     *time.Duration
}

func (s *Service) ModifyToken(input ModifyTokenInput) (*TokenView, error) {
	existing, err := s.tokenRepo.GetByID(input.TokenID)
	if err != nil {
		return nil, RuntimeError{Code: "token_not_found", Message: "token not found"}
	}

	// 只有 token 的 owner 或 admin/owner 角色可以修改，与 RevokeToken 保持一致。
	if existing.UserID != s.runtime.ActorUserID && !tokenManageAllowed(s.runtime.Role) {
		return nil, PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
	}

	if existing.RevokedAt != nil {
		return nil, RuntimeError{Code: "token_revoked", Message: "cannot modify a revoked token"}
	}
	if existing.ExpiresAt != nil && *existing.ExpiresAt < s.clock.Unix() {
		return nil, RuntimeError{Code: "token_expired", Message: "cannot modify an expired token"}
	}

	updates := storage.TokenUpdates{}
	dirty := false

	if input.Name != nil {
		updates.Name = input.Name
		dirty = true
	}

	// 解析 workspace：nil = 不改；非 nil = 替换（含空切片 = 清空）。
	// 最终 workspace 集合用于校验 scope 和 project 的一致性。
	// resolvedWorkspaces 复用：project 解析时若本次改了 workspace 就用新的，否则用 existing。
	finalWorkspaceIDs := parseIDsFromJSON(existing.WorkspaceIDsJSON)
	var resolvedWorkspaces []storage.Workspace
	if input.WorkspaceRefs != nil {
		resolvedWorkspaces, err = s.resolveTokenWorkspaces(*input.WorkspaceRefs)
		if err != nil {
			return nil, err
		}
		finalWorkspaceIDs = make([]string, 0, len(resolvedWorkspaces))
		for _, workspace := range resolvedWorkspaces {
			finalWorkspaceIDs = append(finalWorkspaceIDs, workspace.ID)
		}
		if existing.Type == auth.TokenTypeAgent && len(finalWorkspaceIDs) == 0 {
			return nil, RuntimeError{Code: "token_agent_requires_workspace", Message: "agent token requires at least one workspace"}
		}
		wj, _ := marshalStringSlice(finalWorkspaceIDs)
		updates.WorkspaceIDsJSON = &wj
		dirty = true
	}

	// 解析 project：nil = 不改；非 nil = 替换。project 必须属于最终 workspace 集合。
	var finalProjectIDs []string
	if input.ProjectRefs != nil {
		// 复用已解析的 workspace；若本次未改 workspace，则按 existing workspace ID 重新解析，
		// 保证 project 归属校验基于 token 当前的 workspace 绑定。
		workspacesForProjects := resolvedWorkspaces
		if workspacesForProjects == nil {
			workspacesForProjects, err = s.resolveTokenWorkspaces(parseIDsFromJSON(existing.WorkspaceIDsJSON))
			if err != nil {
				return nil, err
			}
		}
		projects, err := s.resolveTokenProjects(workspacesForProjects, *input.ProjectRefs)
		if err != nil {
			return nil, err
		}
		finalProjectIDs = make([]string, 0, len(projects))
		for _, project := range projects {
			finalProjectIDs = append(finalProjectIDs, project.ID)
		}
		pj, _ := marshalStringSlice(finalProjectIDs)
		updates.ProjectIDsJSON = &pj
		dirty = true
	}

	if input.Scopes != nil {
		scopes, err := auth.ValidateTokenCreate(auth.CreateTokenOptions{
			Type:         existing.Type,
			Scopes:       *input.Scopes,
			WorkspaceIDs: finalWorkspaceIDs,
		})
		if err != nil {
			return nil, RuntimeError{Code: "token_scope_invalid", Message: err.Error()}
		}
		if scopes.Has(auth.ScopeImpersonate) && !tokenManageAllowed(s.runtime.Role) {
			return nil, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "only admin or owner can assign impersonate scope"}
		}
		// 越权防护由 resolveTokenWorkspaces 的角色校验承担；modify 无父 token 概念，不调 enforceTokenCreateLimit。
		sj, _ := marshalStringSlice(scopes.Values())
		updates.ScopesJSON = &sj
		dirty = true
	}

	if input.ExpiresIn != nil {
		if *input.ExpiresIn == 0 {
			updates.ClearExpiresAt = true
		} else if *input.ExpiresIn < 0 {
			return nil, RuntimeError{Code: "token_scope_invalid", Message: "expires-in must be a positive duration"}
		} else {
			ts := s.clock.Unix() + int64(input.ExpiresIn.Seconds())
			updates.ExpiresAt = &ts
		}
		dirty = true
	}

	if dirty {
		err = s.withAudit("token.modified", func(tx *Service) (AuditEntry, error) {
			if err := tx.tokenRepo.Update(input.TokenID, updates); err != nil {
				return AuditEntry{}, err
			}
			return AuditEntry{
				TargetType: "token",
				TargetID:   existing.ID,
				Payload: map[string]any{
					"user_id": existing.UserID,
					"name":    existing.Name,
					"changes": updates.ChangedFields(),
				},
			}, nil
		})
		if err != nil {
			return nil, RuntimeError{Code: "token_update_failed", Message: "failed to update token"}
		}
	}

	updated, err := s.tokenRepo.GetByID(input.TokenID)
	if err != nil {
		return nil, RuntimeError{Code: "token_not_found", Message: "failed to reload token"}
	}

	view := tokenEntryToView(updated)
	return &view, nil
}

func tokenEntryToView(row storage.ApiTokenEntry) TokenView {
	scopes, _ := unmarshalStringSlice(row.ScopesJSON)
	workspaceIDs, _ := unmarshalStringSlice(row.WorkspaceIDsJSON)
	projectIDs, _ := unmarshalStringSlice(row.ProjectIDsJSON)
	return tokenViewFromEntry(row, scopes, workspaceIDs, projectIDs)
}

func parseIDsFromJSON(jsonStr string) []string {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return nil
	}
	var ids []string
	json.Unmarshal([]byte(jsonStr), &ids)
	return ids
}
