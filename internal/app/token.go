package app

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/auth"
	"github.com/dajee/taskg/internal/storage"
	"github.com/dajee/taskg/internal/task"
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
	if err := auth.ValidateTokenCreate(auth.CreateTokenOptions{
		Type:         tokenType,
		Scopes:       input.Scopes,
		WorkspaceIDs: workspaceIDs,
	}); err != nil {
		return CreatedToken{}, classifyTokenCreateError(err)
	}
	scopes, err := auth.ParseScopes(input.Scopes)
	if err != nil {
		return CreatedToken{}, RuntimeError{Code: "token_scope_invalid", Message: err.Error()}
	}
	if scopes.Has("impersonate") && !tokenManageAllowed(s.runtime.Role) {
		return CreatedToken{}, RuntimeError{Code: "token_scope_denied", Message: "only admin or owner can create tokens with impersonate scope"}
	}
	if err := enforceTokenCreateLimit(input.ParentToken, scopes.Values(), workspaceIDs, projectIDs); err != nil {
		return CreatedToken{}, err
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
	workspaceJSON, err := marshalStringSlice(workspaceIDs)
	if err != nil {
		return CreatedToken{}, err
	}
	projectJSON, err := marshalStringSlice(projectIDs)
	if err != nil {
		return CreatedToken{}, err
	}

	stored := storage.ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           targetUser.ID,
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

	var created CreatedToken
	err = s.withAudit("token.create", func(tx *Service) (AuditEntry, error) {
		if err := tx.tokenRepo.Create(stored); err != nil {
			return AuditEntry{}, err
		}
		created = CreatedToken{
			RawToken: raw,
			View:     tokenViewFromEntry(stored, scopes.Values(), workspaceIDs, projectIDs),
			Stored:   stored,
		}
		return AuditEntry{
			TargetType: "token",
			TargetID:   stored.ID,
			Payload: map[string]any{
				"name":          stored.Name,
				"type":          stored.Type,
				"user_id":       stored.UserID,
				"workspace_ids": workspaceIDs,
				"project_ids":   projectIDs,
				"scopes":        scopes.Values(),
				"expires_at":    expiresAt,
			},
		}, nil
	})
	return created, err
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
		return PermissionError{Code: "permission_denied", Message: "permission denied"}
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
			return RuntimeError{Code: "token_scope_denied", Message: "new token scope exceeds current token"}
		}
	}
	if err := requireSubsetWhenRestricted(parent.WorkspaceIDs, workspaceIDs, "workspace_scope_denied", "new token workspace scope exceeds current token"); err != nil {
		return err
	}
	return requireSubsetWhenRestricted(parent.ProjectIDs, projectIDs, "project_scope_denied", "new token project scope exceeds current token")
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
	if err := requireSubsetWhenRestricted(parent.WorkspaceIDs, workspaceIDs, "workspace_scope_denied", "target token workspace scope is outside current token"); err != nil {
		return err
	}
	return requireSubsetWhenRestricted(parent.ProjectIDs, projectIDs, "project_scope_denied", "target token project scope is outside current token")
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
		return AuthenticatedToken{}, RuntimeError{Code: "auth_invalid_token", Message: "invalid token"}
	}
	prefix := raw
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	row, err := s.tokenRepo.GetByPrefix(prefix)
	if err == storage.ErrNotFound {
		return AuthenticatedToken{}, RuntimeError{Code: "auth_invalid_token", Message: "invalid token"}
	}
	if err != nil {
		return AuthenticatedToken{}, err
	}
	if !auth.VerifyTokenHash(raw, row.TokenHash) {
		return AuthenticatedToken{}, RuntimeError{Code: "auth_invalid_token", Message: "invalid token"}
	}
	now := s.clock.Unix()
	if row.RevokedAt != nil {
		return AuthenticatedToken{}, RuntimeError{Code: "auth_token_revoked", Message: "token revoked"}
	}
	if row.ExpiresAt != nil && now > *row.ExpiresAt {
		return AuthenticatedToken{}, RuntimeError{Code: "auth_token_expired", Message: "token expired"}
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

func (s *Service) resolveTokenTargetUser(ref string) (storage.User, error) {
	if strings.TrimSpace(ref) == "" {
		return s.userRepo.GetByID(s.runtime.ActorUserID)
	}
	user, err := s.resolveUser(ref)
	if err != nil {
		return storage.User{}, err
	}
	if user.ID != s.runtime.ActorUserID && !tokenManageAllowed(s.runtime.Role) {
		return storage.User{}, PermissionError{Code: "permission_denied", Message: "permission denied"}
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
		return storage.User{}, PermissionError{Code: "permission_denied", Message: "permission denied"}
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
