package app

import (
	"encoding/json"
	"errors"
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
	Purpose      string
	// WebLoginDisabled 标记该 token 不能用于 Web Console 登录页登录。
	WebLoginDisabled bool
}

type CreatedToken struct {
	RawToken string
	View     TokenView
	Stored   storage.ApiTokenEntry
}

type CreateTenantAccessTokenInput struct {
	Name                   string
	Scopes                 []string
	WorkspaceRef           string
	ProjectRefs            []string
	ExpiresIn              *time.Duration
	IssuedVia              string
	IssuedByAdminTokenID   *string
	IssuedByAdminTokenName *string
	Purpose                string
}

type ModifyTenantAccessTokenInput struct {
	TokenRef    string
	Name        *string
	Scopes      *[]string
	ProjectRefs *[]string
	ExpiresIn   *time.Duration
}

type ListTenantAccessTokensInput struct {
	WorkspaceRef   string
	IncludeRevoked bool
}

type TenantAccessTokenView struct {
	ID                     string
	Prefix                 string
	Name                   string
	Type                   string
	WorkspaceID            string
	ProjectIDs             []string
	Scopes                 []string
	CreatedAt              int64
	ExpiresAt              *int64
	RevokedAt              *int64
	LastUsedAt             *int64
	IssuedVia              string
	IssuedByAdminTokenID   *string
	IssuedByAdminTokenName *string
	Purpose                string
}

type CreatedTenantAccessToken struct {
	RawToken string
	View     TenantAccessTokenView
	Stored   storage.ApiTokenEntry
}

type AuthenticatedToken struct {
	Token       TokenView
	User        storage.User
	TenantActor bool
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
	if err := s.rejectAdminSwitchTokenManagement(); err != nil {
		return CreatedToken{}, err
	}
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

func (s *Service) CreateTenantAccessToken(input CreateTenantAccessTokenInput) (CreatedTenantAccessToken, error) {
	if err := s.rejectAdminSwitchTenantTokenManagement(); err != nil {
		return CreatedTenantAccessToken{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CreatedTenantAccessToken{}, RuntimeError{Code: "tenant_token_name_required", Message: "tenant token name is required"}
	}
	workspace, err := s.resolveTenantTokenWorkspace(input.WorkspaceRef)
	if err != nil {
		return CreatedTenantAccessToken{}, err
	}
	projects, err := s.resolveTokenProjects([]storage.Workspace{workspace}, input.ProjectRefs)
	if err != nil {
		return CreatedTenantAccessToken{}, err
	}
	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ID)
	}
	scopes, err := auth.ValidateTenantTokenScopes(input.Scopes)
	if err != nil {
		return CreatedTenantAccessToken{}, RuntimeError{Code: "tenant_token_scope_invalid", Message: err.Error()}
	}
	if err := s.enforceTenantTokenWriteLimit(scopes.Values(), projectIDs); err != nil {
		return CreatedTenantAccessToken{}, err
	}

	var created CreatedTenantAccessToken
	err = s.withAudit("tenant_token.create", func(tx *Service) (AuditEntry, error) {
		raw, prefix, hash, err := auth.GenerateToken(auth.TokenTypeTenantAccess)
		if err != nil {
			return AuditEntry{}, err
		}
		secretCipher, err := tx.encryptRecoverableToken(raw)
		if err != nil {
			return AuditEntry{}, err
		}
		createdAt := tx.clock.Unix()
		var expiresAt *int64
		if input.ExpiresIn != nil {
			value := createdAt + int64(input.ExpiresIn.Seconds())
			expiresAt = &value
		}
		scopesJSON, err := marshalStringSlice(scopes.Values())
		if err != nil {
			return AuditEntry{}, err
		}
		workspaceJSON, err := marshalStringSlice([]string{workspace.ID})
		if err != nil {
			return AuditEntry{}, err
		}
		projectJSON, err := marshalStringSlice(projectIDs)
		if err != nil {
			return AuditEntry{}, err
		}
		stored := storage.ApiTokenEntry{
			ID:                     uuid.NewString(),
			UserID:                 nil,
			Name:                   name,
			Type:                   auth.TokenTypeTenantAccess,
			TokenPrefix:            prefix,
			TokenHash:              hash,
			TokenSecretCiphertext:  secretCipher,
			ScopesJSON:             scopesJSON,
			WorkspaceIDsJSON:       workspaceJSON,
			ProjectIDsJSON:         projectJSON,
			IssuedVia:              defaultString(input.IssuedVia, "user"),
			IssuedByAdminTokenID:   input.IssuedByAdminTokenID,
			IssuedByAdminTokenName: input.IssuedByAdminTokenName,
			Purpose:                defaultString(input.Purpose, "api"),
			CreatedAt:              createdAt,
			ExpiresAt:              expiresAt,
		}
		if err := tx.tokenRepo.Create(stored); err != nil {
			return AuditEntry{}, err
		}
		created = CreatedTenantAccessToken{
			RawToken: raw,
			View:     tenantTokenViewFromEntry(stored, scopes.Values(), workspace.ID, projectIDs),
			Stored:   stored,
		}
		return AuditEntry{
			TargetType:  "tenant_token",
			TargetID:    stored.ID,
			WorkspaceID: &workspace.ID,
			Payload: map[string]any{
				"name":         stored.Name,
				"type":         stored.Type,
				"workspace_id": workspace.ID,
				"project_ids":  projectIDs,
				"scopes":       scopes.Values(),
				"expires_at":   stored.ExpiresAt,
			},
		}, nil
	})
	return created, err
}

func (s *Service) ListTenantAccessTokens(input ListTenantAccessTokensInput) ([]TenantAccessTokenView, error) {
	workspace, err := s.resolveTenantTokenWorkspace(input.WorkspaceRef)
	if err != nil {
		return nil, err
	}
	rows, err := s.tokenRepo.ListTenantByWorkspace(workspace.ID, input.IncludeRevoked)
	if err != nil {
		return nil, err
	}
	out := make([]TenantAccessTokenView, 0, len(rows))
	for _, row := range rows {
		view, err := tenantTokenEntryToView(row)
		if err != nil {
			return nil, err
		}
		if s.requestScope != nil {
			if err := requireSubsetWhenRestricted(s.requestScope.ProjectIDs, view.ProjectIDs, authz.CodeProjectScopeDenied, "target tenant token project scope is outside current token"); err != nil {
				continue
			}
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) ModifyTenantAccessToken(input ModifyTenantAccessTokenInput) (*TenantAccessTokenView, error) {
	if err := s.rejectAdminSwitchTenantTokenManagement(); err != nil {
		return nil, err
	}
	existing, workspaceID, err := s.lookupTenantTokenForRuntime(input.TokenRef)
	if err != nil {
		return nil, err
	}
	if existing.RevokedAt != nil {
		return nil, RuntimeError{Code: "tenant_token_revoked", Message: "cannot modify a revoked tenant token"}
	}
	if existing.ExpiresAt != nil && *existing.ExpiresAt < s.clock.Unix() {
		return nil, RuntimeError{Code: "tenant_token_expired", Message: "cannot modify an expired tenant token"}
	}

	updates := storage.TokenUpdates{}
	dirty := false
	finalScopes, err := unmarshalStringSlice(existing.ScopesJSON)
	if err != nil {
		return nil, err
	}
	existingScopes := append([]string(nil), finalScopes...)
	finalProjectIDs, err := unmarshalStringSlice(existing.ProjectIDsJSON)
	if err != nil {
		return nil, err
	}
	existingProjectIDs := append([]string(nil), finalProjectIDs...)
	if input.Name != nil {
		updates.Name = input.Name
		dirty = true
	}
	if input.Scopes != nil {
		scopes, err := auth.ValidateTenantTokenScopes(*input.Scopes)
		if err != nil {
			return nil, RuntimeError{Code: "tenant_token_scope_invalid", Message: err.Error()}
		}
		finalScopes = scopes.Values()
		sj, _ := marshalStringSlice(scopes.Values())
		updates.ScopesJSON = &sj
		dirty = true
	}
	if input.ProjectRefs != nil {
		workspace, err := s.workspaceRepo.GetByID(workspaceID)
		if err != nil {
			return nil, err
		}
		projects, err := s.resolveTokenProjects([]storage.Workspace{workspace}, *input.ProjectRefs)
		if err != nil {
			return nil, err
		}
		projectIDs := make([]string, 0, len(projects))
		for _, project := range projects {
			projectIDs = append(projectIDs, project.ID)
		}
		finalProjectIDs = projectIDs
		pj, _ := marshalStringSlice(projectIDs)
		updates.ProjectIDsJSON = &pj
		dirty = true
	}
	if input.ExpiresIn != nil {
		if *input.ExpiresIn == 0 {
			updates.ClearExpiresAt = true
		} else if *input.ExpiresIn < 0 {
			return nil, RuntimeError{Code: "tenant_token_scope_invalid", Message: "expires-in must be a positive duration"}
		} else {
			ts := s.clock.Unix() + int64(input.ExpiresIn.Seconds())
			updates.ExpiresAt = &ts
		}
		dirty = true
	}
	if err := s.enforceTenantTokenModifyLimit(existingScopes, existingProjectIDs, finalScopes, finalProjectIDs); err != nil {
		return nil, err
	}
	if dirty {
		if err := s.withAudit("tenant_token.modify", func(tx *Service) (AuditEntry, error) {
			if err := tx.tokenRepo.Update(existing.ID, updates); err != nil {
				return AuditEntry{}, err
			}
			return AuditEntry{
				TargetType:  "tenant_token",
				TargetID:    existing.ID,
				WorkspaceID: &workspaceID,
				Payload: map[string]any{
					"name":    existing.Name,
					"changes": updates.ChangedFields(),
				},
			}, nil
		}); err != nil {
			return nil, err
		}
	}
	updated, err := s.tokenRepo.GetByID(existing.ID)
	if err != nil {
		return nil, err
	}
	view, err := tenantTokenEntryToView(updated)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) enforceTenantTokenModifyLimit(existingScopes, existingProjectIDs, finalScopes, finalProjectIDs []string) error {
	// 与 enforceTenantTokenWriteLimit 同理：browser session 跳过子集校验。
	if s.requestScope == nil || s.runtime.CredentialIsBrowserSession() {
		return nil
	}
	for _, scope := range existingScopes {
		if !s.requestScope.HasCapability(scope) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "target tenant token scope is outside current token"}
		}
	}
	if err := requireSubsetWhenRestricted(s.requestScope.ProjectIDs, existingProjectIDs, authz.CodeProjectScopeDenied, "target tenant token project scope is outside current token"); err != nil {
		return err
	}
	return s.enforceTenantTokenWriteLimit(finalScopes, finalProjectIDs)
}

func (s *Service) RevokeTenantAccessToken(ref string) error {
	if err := s.rejectAdminSwitchTenantTokenManagement(); err != nil {
		return err
	}
	entry, workspaceID, err := s.lookupTenantTokenForRuntime(ref)
	if err != nil {
		return err
	}
	scopes, err := unmarshalStringSlice(entry.ScopesJSON)
	if err != nil {
		return err
	}
	projectIDs, err := unmarshalStringSlice(entry.ProjectIDsJSON)
	if err != nil {
		return err
	}
	if err := s.enforceTenantTokenWriteLimit(scopes, projectIDs); err != nil {
		return err
	}
	return s.withAudit("tenant_token.revoke", func(tx *Service) (AuditEntry, error) {
		if err := tx.tokenRepo.Revoke(entry.ID, tx.clock.Unix()); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType:  "tenant_token",
			TargetID:    entry.ID,
			WorkspaceID: &workspaceID,
			Payload: map[string]any{
				"name": entry.Name,
			},
		}, nil
	})
}

func (s *Service) resolveTenantTokenWorkspace(ref string) (storage.Workspace, error) {
	if s.runtime.AdminActingSessionID != "" {
		return storage.Workspace{}, RuntimeError{Code: "admin_acting_not_allowed", Message: "admin acting session cannot manage tenant tokens"}
	}
	if !tokenManageAllowed(s.runtime.Role) {
		return storage.Workspace{}, PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
	}
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		trimmed = s.runtime.WorkspaceID
	}
	workspace, err := lookupWorkspace(s.workspaceRepo, trimmed)
	if err != nil {
		return storage.Workspace{}, err
	}
	if workspace.ID != s.runtime.WorkspaceID {
		return storage.Workspace{}, RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "tenant token workspace must match current workspace"}
	}
	return workspace, nil
}

func (s *Service) rejectAdminSwitchTenantTokenManagement() error {
	if s.runtime.IsTenantActor() && s.runtime.ActorTokenPurpose == "admin_tenant_switch" {
		return RuntimeError{Code: "tenant_token_management_denied", Message: "admin switch tenant token cannot manage tenant access tokens"}
	}
	return nil
}

func (s *Service) rejectAdminSwitchTokenManagement() error {
	if s.runtime.IsTenantActor() && s.runtime.ActorTokenPurpose == "admin_tenant_switch" {
		return RuntimeError{Code: "token_management_denied", Message: "admin switch tenant token cannot manage tokens"}
	}
	return nil
}

func (s *Service) enforceTenantTokenWriteLimit(scopes, projectIDs []string) error {
	// 子集校验防止一个受限凭证（PAT/Agent/tenant token）创建/修改出比自己权限更大的子 token。
	// browser session（Web Console 的 SSO 登录会话）不在此列：它的 capability 是交互层人为收紧，
	// 真实授权由 membership role 决定（tokenManageAllowed 已限定 owner/admin），不应被 capability
	// 子集卡住——否则 owner 在 Web Console 给 tenant token 勾选 workspace:write/user:write 等
	// browser session 刻意排除的 scope 时会误报 "scope exceeds current token"。
	if s.requestScope == nil || s.runtime.CredentialIsBrowserSession() {
		return nil
	}
	for _, scope := range scopes {
		if !s.requestScope.HasCapability(scope) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "new tenant token scope exceeds current token"}
		}
	}
	return requireSubsetWhenRestricted(s.requestScope.ProjectIDs, projectIDs, authz.CodeProjectScopeDenied, "new tenant token project scope exceeds current token")
}

func (s *Service) lookupTenantTokenForRuntime(ref string) (storage.ApiTokenEntry, string, error) {
	if _, err := s.resolveTenantTokenWorkspace(""); err != nil {
		return storage.ApiTokenEntry{}, "", err
	}
	entry, err := s.tokenRepo.GetByIDOrPrefix(strings.TrimSpace(ref))
	if err != nil {
		return storage.ApiTokenEntry{}, "", RuntimeError{Code: "tenant_token_not_found", Message: "tenant token not found"}
	}
	if entry.Type != auth.TokenTypeTenantAccess {
		return storage.ApiTokenEntry{}, "", RuntimeError{Code: "tenant_token_not_found", Message: "tenant token not found"}
	}
	workspaceID, err := tenantTokenWorkspaceID(entry)
	if err != nil {
		return storage.ApiTokenEntry{}, "", err
	}
	if workspaceID != s.runtime.WorkspaceID {
		return storage.ApiTokenEntry{}, "", RuntimeError{Code: "tenant_token_not_found", Message: "tenant token not found"}
	}
	return entry, workspaceID, nil
}

// TokenMCPConfigView 是 reveal token MCP 配置时返回的视图，含 raw token 与边界信息。
type TokenMCPConfigView struct {
	TokenID      string
	TokenName    string
	TokenType    string
	Prefix       string
	RawToken     string
	EndpointPath string
	Scopes       []string
	WorkspaceIDs []string
	ProjectIDs   []string
	ExpiresAt    *int64
	RevokedAt    *int64
}

func (s *Service) rejectAdminSwitchTokenReveal() error {
	if s.runtime.IsTenantActor() && s.runtime.ActorTokenPurpose == "admin_tenant_switch" {
		return RuntimeError{Code: "token_management_denied", Message: "admin switch tenant token cannot reveal token mcp config"}
	}
	return nil
}

// RevealTokenMCPConfig 解密目标普通 token（PAT/Agent）并返回 MCP 配置视图。
// 复用现有 token 管理 scope/workspace/project 校验，不拒绝已吊销/过期 token。
func (s *Service) RevealTokenMCPConfig(tokenRef string) (TokenMCPConfigView, error) {
	return s.revealTokenMCPConfigShared(tokenRef, false)
}

// RevealTenantTokenMCPConfig 解密目标 tenant access token 并返回 MCP 配置视图。
func (s *Service) RevealTenantTokenMCPConfig(tokenRef string) (TokenMCPConfigView, error) {
	return s.revealTokenMCPConfigShared(tokenRef, true)
}

func (s *Service) revealTokenMCPConfigShared(tokenRef string, tenant bool) (TokenMCPConfigView, error) {
	if err := s.rejectAdminSwitchTokenReveal(); err != nil {
		return TokenMCPConfigView{}, err
	}

	var view TokenMCPConfigView
	if tenant {
		err := s.withAudit("tenant_token.mcp_config_reveal", func(tx *Service) (AuditEntry, error) {
			// lookupTenantTokenForRuntime 内部复用 resolveTenantTokenWorkspace，
			// 校验 tokenManageAllowed(role) 与 workspace 归属。
			row, workspaceID, lookupErr := tx.lookupTenantTokenForRuntime(tokenRef)
			if lookupErr != nil {
				return AuditEntry{}, lookupErr
			}
			scopes, err := unmarshalStringSlice(row.ScopesJSON)
			if err != nil {
				return AuditEntry{}, err
			}
			projectIDs, err := unmarshalStringSlice(row.ProjectIDsJSON)
			if err != nil {
				return AuditEntry{}, err
			}
			if err := tx.enforceTenantTokenReadLimit(scopes, projectIDs); err != nil {
				return AuditEntry{}, err
			}
			raw, err := tx.decryptRecoverableToken(row)
			if err != nil {
				return AuditEntry{}, err
			}
			view = tokenMCPConfigViewFromEntry(row, raw, []string{workspaceID}, projectIDs, scopes)
			return AuditEntry{
				TargetType:  "tenant_token",
				TargetID:    row.ID,
				WorkspaceID: &workspaceID,
				Payload: map[string]any{
					"token_id":     row.ID,
					"token_name":   row.Name,
					"token_type":   row.Type,
					"token_prefix": row.TokenPrefix,
				},
			}, nil
		})
		return view, err
	}
	err := s.withAudit("token.mcp_config_reveal", func(tx *Service) (AuditEntry, error) {
		row, lookupErr := tx.tokenRepo.GetByIDOrPrefix(strings.TrimSpace(tokenRef))
		if lookupErr != nil {
			return AuditEntry{}, classifyTokenLookupError(lookupErr)
		}
		if row.Type != auth.TokenTypePAT && row.Type != auth.TokenTypeAgent {
			return AuditEntry{}, RuntimeError{Code: "token_not_found", Message: "token not found"}
		}
		// 与 RevokeToken/ModifyToken 一致：owner 可 reveal 自己的 token；
		// 非 owner 必须 admin/owner 角色。HTTP 层已用 PermissionTokenRead 拒绝 member/viewer。
		if derefString(row.UserID) != tx.runtime.ActorUserID && !tokenManageAllowed(tx.runtime.Role) {
			return AuditEntry{}, RuntimeError{Code: "token_not_found", Message: "token not found"}
		}
		scopes, err := unmarshalStringSlice(row.ScopesJSON)
		if err != nil {
			return AuditEntry{}, err
		}
		workspaceIDs, err := unmarshalStringSlice(row.WorkspaceIDsJSON)
		if err != nil {
			return AuditEntry{}, err
		}
		projectIDs, err := unmarshalStringSlice(row.ProjectIDsJSON)
		if err != nil {
			return AuditEntry{}, err
		}
		if err := enforceTokenScopeRequestScope(tx.requestScope, scopes, workspaceIDs, projectIDs); err != nil {
			return AuditEntry{}, err
		}
		raw, err := tx.decryptRecoverableToken(row)
		if err != nil {
			return AuditEntry{}, err
		}
		view = tokenMCPConfigViewFromEntry(row, raw, workspaceIDs, projectIDs, scopes)
		return AuditEntry{
			TargetType: "token",
			TargetID:   row.ID,
			Payload: map[string]any{
				"token_id":     row.ID,
				"token_name":   row.Name,
				"token_type":   row.Type,
				"token_prefix": row.TokenPrefix,
			},
		}, nil
	})
	return view, err
}

// enforceTenantTokenReadLimit 复用 write limit 的 scope/project 校验，但只读：
// tenant token reveal 仍只能触及当前凭证可管理的 workspace 内、project allowlist 内的 token。
func (s *Service) enforceTenantTokenReadLimit(scopes, projectIDs []string) error {
	if s.requestScope == nil {
		return nil
	}
	for _, scope := range scopes {
		if !s.requestScope.HasCapability(scope) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "target tenant token scope is outside current token"}
		}
	}
	return requireSubsetWhenRestricted(s.requestScope.ProjectIDs, projectIDs, authz.CodeProjectScopeDenied, "target tenant token project scope is outside current token")
}

func tokenMCPConfigViewFromEntry(row storage.ApiTokenEntry, raw string, workspaceIDs, projectIDs, scopes []string) TokenMCPConfigView {
	return TokenMCPConfigView{
		TokenID:      row.ID,
		TokenName:    row.Name,
		TokenType:    row.Type,
		Prefix:       row.TokenPrefix,
		RawToken:     raw,
		EndpointPath: "/mcp",
		Scopes:       append([]string(nil), scopes...),
		WorkspaceIDs: append([]string(nil), workspaceIDs...),
		ProjectIDs:   append([]string(nil), projectIDs...),
		ExpiresAt:    row.ExpiresAt,
		RevokedAt:    row.RevokedAt,
	}
}

// encryptRecoverableToken 用 token secret key 加密 raw token，得到可恢复 envelope。
// requireTokenSecret=true 且缺少 key 时返回 config_secret_key_missing；
// requireTokenSecret=false 且缺少 key 时返回空字符串（兼容无 reveal 需求的创建路径）。
func (s *Service) encryptRecoverableToken(raw string) (string, error) {
	if len(s.tokenSecretKey) != 32 {
		if s.requireTokenSecret {
			return "", RuntimeError{Code: "config_secret_key_missing", Message: "config secret key is required to create recoverable tokens"}
		}
		return "", nil
	}
	encrypted, err := EncryptConfigSecret(s.tokenSecretKey, raw)
	if err != nil {
		if errors.Is(err, ErrConfigSecretKeyMissing) || errors.Is(err, ErrConfigSecretKeyInvalid) {
			return "", RuntimeError{Code: "config_secret_key_missing", Message: "config secret key is required to create recoverable tokens"}
		}
		return "", err
	}
	return encrypted, nil
}

// decryptRecoverableToken 解密 envelope 还原 raw token；失败统一映射为安全错误码，
// 不把底层解密细节暴露给调用方。
func (s *Service) decryptRecoverableToken(row storage.ApiTokenEntry) (string, error) {
	if strings.TrimSpace(row.TokenSecretCiphertext) == "" {
		return "", RuntimeError{Code: "token_secret_unavailable", Message: "token secret is unavailable"}
	}
	raw, err := DecryptConfigSecret(s.tokenSecretKey, row.TokenSecretCiphertext)
	if err != nil {
		if errors.Is(err, ErrConfigSecretKeyMissing) || errors.Is(err, ErrConfigSecretKeyInvalid) {
			return "", RuntimeError{Code: "config_secret_key_missing", Message: "config secret key is required to reveal token"}
		}
		return "", RuntimeError{Code: "token_secret_unavailable", Message: "token secret is unavailable"}
	}
	return raw, nil
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
	secretCipher, err := s.encryptRecoverableToken(raw)
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
		ID:                    uuid.NewString(),
		UserID:                stringPtr(input.UserID),
		Name:                  name,
		Type:                  tokenType,
		TokenPrefix:           prefix,
		TokenHash:             hash,
		TokenSecretCiphertext: secretCipher,
		ScopesJSON:            scopesJSON,
		WorkspaceIDsJSON:      workspaceJSON,
		ProjectIDsJSON:        projectJSON,
		// SSO browser session 创建的 PAT/Agent token 禁止用于 Web Console 登录。
		WebLoginDisabled: s.runtime.WebLoginDisabled,
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
	if s.runtime.IsTenantActor() {
		return s.listTokensForTenantActor(input)
	}
	targetUser, err := s.resolveTokenListUser(input.UserRef)
	if err != nil {
		return nil, err
	}
	rows, err := s.tokenRepo.ListByUser(targetUser.ID, input.IncludeRevoked)
	if err != nil {
		return nil, err
	}
	userInfos, err := s.resolveUserInfos([]string{targetUser.ID})
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
		if !s.tokenEntryAllowedByRequestScope(workspaceIDs, projectIDs) {
			continue
		}
		view := tokenViewFromEntry(row, scopes, workspaceIDs, projectIDs)
		if ui := userInfos[derefString(row.UserID)]; ui.ID != "" {
			view.User = ui
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) tokenEntryAllowedByRequestScope(workspaceIDs, projectIDs []string) bool {
	if s.requestScope == nil {
		return true
	}
	if err := requireSubsetWhenRestricted(s.requestScope.WorkspaceIDs, workspaceIDs, authz.CodeWorkspaceScopeDenied, "target token workspace scope is outside current token"); err != nil {
		return false
	}
	if err := requireSubsetWhenRestricted(s.requestScope.ProjectIDs, projectIDs, authz.CodeProjectScopeDenied, "target token project scope is outside current token"); err != nil {
		return false
	}
	return true
}

func (s *Service) listTokensForTenantActor(input ListTokensInput) ([]TokenView, error) {
	if strings.TrimSpace(input.UserRef) != "" {
		return nil, tenantActorNotUserError()
	}
	rows, err := s.tokenRepo.ListAll(input.IncludeRevoked)
	if err != nil {
		return nil, err
	}
	scope := RequestScope{}
	if s.requestScope != nil {
		scope = *s.requestScope
	} else if s.runtime.WorkspaceID != "" {
		scope.WorkspaceIDs = []string{s.runtime.WorkspaceID}
	}
	userIDs := make([]string, 0, len(rows))
	entries := make([]storage.ApiTokenEntry, 0, len(rows))
	entryScopes := make(map[string][]string, len(rows))
	entryWorkspaces := make(map[string][]string, len(rows))
	entryProjects := make(map[string][]string, len(rows))
	for _, row := range rows {
		workspaceIDs, err := unmarshalStringSlice(row.WorkspaceIDsJSON)
		if err != nil {
			return nil, err
		}
		projectIDs, err := unmarshalStringSlice(row.ProjectIDsJSON)
		if err != nil {
			return nil, err
		}
		if err := requireSubsetWhenRestricted(scope.WorkspaceIDs, workspaceIDs, authz.CodeWorkspaceScopeDenied, "target token workspace scope is outside current token"); err != nil {
			continue
		}
		if err := requireSubsetWhenRestricted(scope.ProjectIDs, projectIDs, authz.CodeProjectScopeDenied, "target token project scope is outside current token"); err != nil {
			continue
		}
		scopes, err := unmarshalStringSlice(row.ScopesJSON)
		if err != nil {
			return nil, err
		}
		entries = append(entries, row)
		entryScopes[row.ID] = scopes
		entryWorkspaces[row.ID] = workspaceIDs
		entryProjects[row.ID] = projectIDs
		if userID := derefString(row.UserID); userID != "" {
			userIDs = append(userIDs, userID)
		}
	}
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]TokenView, 0, len(entries))
	for _, row := range entries {
		view := tokenViewFromEntry(row, entryScopes[row.ID], entryWorkspaces[row.ID], entryProjects[row.ID])
		if ui := userInfos[derefString(row.UserID)]; ui.ID != "" {
			view.User = ui
		}
		out = append(out, view)
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
	if err := s.rejectAdminSwitchTokenManagement(); err != nil {
		return err
	}
	entry, err := s.tokenRepo.GetByIDOrPrefix(strings.TrimSpace(ref))
	if err != nil {
		return err
	}
	if entry.Type == auth.TokenTypeTenantAccess {
		return RuntimeError{Code: "token_not_found", Message: "token not found"}
	}
	if derefString(entry.UserID) != s.runtime.ActorUserID && !tokenManageAllowed(s.runtime.Role) {
		return PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
	}
	if err := enforceTokenRevokeLimit(limit, entry); err != nil {
		return err
	}
	if err := s.enforceTokenRevokeRequestScope(entry); err != nil {
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
	scopes, err := unmarshalStringSlice(entry.ScopesJSON)
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		if !slices.Contains(parent.Scopes, scope) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "target token scope is outside current token"}
		}
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

func (s *Service) enforceTokenRevokeRequestScope(entry storage.ApiTokenEntry) error {
	if s.requestScope == nil {
		return nil
	}
	scopes, err := unmarshalStringSlice(entry.ScopesJSON)
	if err != nil {
		return err
	}
	workspaceIDs, err := unmarshalStringSlice(entry.WorkspaceIDsJSON)
	if err != nil {
		return err
	}
	projectIDs, err := unmarshalStringSlice(entry.ProjectIDsJSON)
	if err != nil {
		return err
	}
	return enforceTokenScopeRequestScope(s.requestScope, scopes, workspaceIDs, projectIDs)
}

// enforceTokenScopeRequestScope 校验目标 token 的 scope/workspace/project 是否都在
// 当前 request scope 内。revoke 与 reveal 共用，避免重复解 JSON 与重复校验逻辑。
func enforceTokenScopeRequestScope(scope *RequestScope, scopes, workspaceIDs, projectIDs []string) error {
	if scope == nil {
		return nil
	}
	for _, scopeName := range scopes {
		if !scope.HasCapability(scopeName) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "target token scope is outside current request scope"}
		}
	}
	if err := requireSubsetWhenRestricted(scope.WorkspaceIDs, workspaceIDs, authz.CodeWorkspaceScopeDenied, "target token workspace scope is outside current request scope"); err != nil {
		return err
	}
	return requireSubsetWhenRestricted(scope.ProjectIDs, projectIDs, authz.CodeProjectScopeDenied, "target token project scope is outside current request scope")
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
	prefix, err := auth.TokenLookupPrefix(raw)
	if err != nil {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
	}
	row, err := s.tokenRepo.GetByPrefix(prefix)
	if err == storage.ErrNotFound {
		legacyPrefix := raw
		if len(legacyPrefix) > 16 {
			legacyPrefix = legacyPrefix[:16]
		}
		if legacyPrefix != prefix {
			row, err = s.tokenRepo.GetByPrefix(legacyPrefix)
		}
	}
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
	if row.Type == auth.TokenTypeTenantAccess {
		if _, err := tenantTokenWorkspaceID(row); err != nil {
			return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
		}
		validatedScopes, err := auth.ValidateTenantTokenScopes(scopes)
		if err != nil {
			return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
		}
		scopes = validatedScopes.Values()
		return AuthenticatedToken{
			Token:       tokenViewFromEntry(row, scopes, workspaceIDs, projectIDs),
			TenantActor: true,
		}, nil
	}
	userID := derefString(row.UserID)
	if userID == "" {
		return AuthenticatedToken{}, RuntimeError{Code: authz.CodeAuthInvalidToken, Message: "invalid token"}
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return AuthenticatedToken{}, err
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
		ID:               row.ID,
		Prefix:           row.TokenPrefix,
		Name:             row.Name,
		Type:             row.Type,
		User:             task.UserInfo{ID: derefString(row.UserID)},
		WorkspaceIDs:     append([]string(nil), workspaceIDs...),
		ProjectIDs:       append([]string(nil), projectIDs...),
		Scopes:           append([]string(nil), scopes...),
		CreatedAt:        row.CreatedAt,
		ExpiresAt:        row.ExpiresAt,
		RevokedAt:        row.RevokedAt,
		LastUsedAt:       row.LastUsedAt,
		Purpose:          row.Purpose,
		WebLoginDisabled: row.WebLoginDisabled,
	}
}

func tenantTokenEntryToView(row storage.ApiTokenEntry) (TenantAccessTokenView, error) {
	scopes, err := unmarshalStringSlice(row.ScopesJSON)
	if err != nil {
		return TenantAccessTokenView{}, err
	}
	projectIDs, err := unmarshalStringSlice(row.ProjectIDsJSON)
	if err != nil {
		return TenantAccessTokenView{}, err
	}
	workspaceID, err := tenantTokenWorkspaceID(row)
	if err != nil {
		return TenantAccessTokenView{}, err
	}
	return tenantTokenViewFromEntry(row, scopes, workspaceID, projectIDs), nil
}

func tenantTokenViewFromEntry(row storage.ApiTokenEntry, scopes []string, workspaceID string, projectIDs []string) TenantAccessTokenView {
	return TenantAccessTokenView{
		ID:                     row.ID,
		Prefix:                 row.TokenPrefix,
		Name:                   row.Name,
		Type:                   row.Type,
		WorkspaceID:            workspaceID,
		ProjectIDs:             append([]string(nil), projectIDs...),
		Scopes:                 append([]string(nil), scopes...),
		CreatedAt:              row.CreatedAt,
		ExpiresAt:              row.ExpiresAt,
		RevokedAt:              row.RevokedAt,
		LastUsedAt:             row.LastUsedAt,
		IssuedVia:              row.IssuedVia,
		IssuedByAdminTokenID:   row.IssuedByAdminTokenID,
		IssuedByAdminTokenName: row.IssuedByAdminTokenName,
		Purpose:                row.Purpose,
	}
}

func tenantTokenWorkspaceID(row storage.ApiTokenEntry) (string, error) {
	workspaceIDs, err := unmarshalStringSlice(row.WorkspaceIDsJSON)
	if err != nil {
		return "", err
	}
	if len(workspaceIDs) != 1 || strings.TrimSpace(workspaceIDs[0]) == "" {
		return "", RuntimeError{Code: "tenant_token_workspace_invalid", Message: "tenant token must bind exactly one workspace"}
	}
	return workspaceIDs[0], nil
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
	if err := s.rejectAdminSwitchTokenManagement(); err != nil {
		return nil, err
	}
	existing, err := s.tokenRepo.GetByIDOrPrefix(input.TokenID)
	if err != nil {
		return nil, classifyTokenLookupError(err)
	}
	if existing.Type == auth.TokenTypeTenantAccess {
		return nil, RuntimeError{Code: "token_not_found", Message: "token not found"}
	}

	// 只有 token 的 owner 或 admin/owner 角色可以修改，与 RevokeToken 保持一致。
	if derefString(existing.UserID) != s.runtime.ActorUserID && !tokenManageAllowed(s.runtime.Role) {
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
	existingScopes, err := unmarshalStringSlice(existing.ScopesJSON)
	if err != nil {
		return nil, err
	}
	finalScopes := append([]string(nil), existingScopes...)
	existingWorkspaceIDs, err := unmarshalStringSlice(existing.WorkspaceIDsJSON)
	if err != nil {
		return nil, err
	}
	finalWorkspaceIDs := append([]string(nil), existingWorkspaceIDs...)
	existingProjectIDs, err := unmarshalStringSlice(existing.ProjectIDsJSON)
	if err != nil {
		return nil, err
	}
	finalProjectIDs := append([]string(nil), existingProjectIDs...)
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
		finalScopes = scopes.Values()
		sj, _ := marshalStringSlice(scopes.Values())
		updates.ScopesJSON = &sj
		dirty = true
	}

	if err := s.enforceTokenModifyLimit(existingScopes, existingWorkspaceIDs, existingProjectIDs, finalScopes, finalWorkspaceIDs, finalProjectIDs); err != nil {
		return nil, err
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
			if err := tx.tokenRepo.Update(existing.ID, updates); err != nil {
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

	updated, err := s.tokenRepo.GetByID(existing.ID)
	if err != nil {
		return nil, RuntimeError{Code: "token_not_found", Message: "failed to reload token"}
	}

	view := tokenEntryToView(updated)
	return &view, nil
}

func (s *Service) enforceTokenModifyLimit(existingScopes, existingWorkspaceIDs, existingProjectIDs, finalScopes, finalWorkspaceIDs, finalProjectIDs []string) error {
	if s.requestScope == nil {
		return nil
	}
	for _, scope := range existingScopes {
		if !s.requestScope.HasCapability(scope) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "target token scope is outside current token"}
		}
	}
	for _, scope := range finalScopes {
		if !s.requestScope.HasCapability(scope) {
			return RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "modified token scope exceeds current token"}
		}
	}
	if err := requireSubsetWhenRestricted(s.requestScope.WorkspaceIDs, existingWorkspaceIDs, authz.CodeWorkspaceScopeDenied, "target token workspace scope is outside current token"); err != nil {
		return err
	}
	if err := requireSubsetWhenRestricted(s.requestScope.WorkspaceIDs, finalWorkspaceIDs, authz.CodeWorkspaceScopeDenied, "modified token workspace scope exceeds current token"); err != nil {
		return err
	}
	if err := requireSubsetWhenRestricted(s.requestScope.ProjectIDs, existingProjectIDs, authz.CodeProjectScopeDenied, "target token project scope is outside current token"); err != nil {
		return err
	}
	return requireSubsetWhenRestricted(s.requestScope.ProjectIDs, finalProjectIDs, authz.CodeProjectScopeDenied, "modified token project scope exceeds current token")
}

func tokenEntryToView(row storage.ApiTokenEntry) TokenView {
	scopes, _ := unmarshalStringSlice(row.ScopesJSON)
	workspaceIDs, _ := unmarshalStringSlice(row.WorkspaceIDsJSON)
	projectIDs, _ := unmarshalStringSlice(row.ProjectIDsJSON)
	return tokenViewFromEntry(row, scopes, workspaceIDs, projectIDs)
}

func classifyTokenLookupError(err error) error {
	if errors.Is(err, storage.ErrAmbiguousTokenRef) {
		return RuntimeError{Code: "token_ambiguous_ref", Message: "token reference matches multiple tokens, use a longer prefix or full ID"}
	}
	return RuntimeError{Code: "token_not_found", Message: "token not found"}
}

func parseIDsFromJSON(jsonStr string) []string {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return nil
	}
	var ids []string
	json.Unmarshal([]byte(jsonStr), &ids)
	return ids
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
