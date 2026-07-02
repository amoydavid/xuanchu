package app

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// AdminWorkspaceMemberCounts 是 workspace 内每个 role 的成员计数摘要。
type AdminWorkspaceMemberCounts struct {
	Owner  int64 `json:"owner"`
	Admin  int64 `json:"admin"`
	Member int64 `json:"member"`
	Viewer int64 `json:"viewer"`
}

// AdminWorkspaceTokenCounts 是 workspace 内 token 的状态计数摘要。
type AdminWorkspaceTokenCounts struct {
	Active  int64 `json:"active"`
	Expired int64 `json:"expired"`
	Revoked int64 `json:"revoked"`
}

// AdminWorkspaceSummaryView 是 admin workspace 列表的一行摘要。
type AdminWorkspaceSummaryView struct {
	ID           string
	Slug         string
	Name         string
	Description  string
	Visibility   string
	CreatedBy    *task.UserInfo
	MemberCounts AdminWorkspaceMemberCounts
	TokenCounts  AdminWorkspaceTokenCounts
	ArchivedAt   *int64
	CreatedAt    int64
	ModifiedAt   int64
}

// AdminWorkspaceMemberView 是 admin workspace 详情中的一个成员行。
type AdminWorkspaceMemberView struct {
	User       task.UserInfo
	Role       Role
	JoinedAt   int64
	ModifiedAt int64
}

// AdminActingCandidateView 是可被 acting session 选中的 owner/admin 用户。
type AdminActingCandidateView struct {
	User task.UserInfo
	Role Role
}

// AdminWorkspaceDetailView 是 admin workspace 详情页数据。
type AdminWorkspaceDetailView struct {
	Workspace        WorkspaceView
	Members          []AdminWorkspaceMemberView
	TokenCounts      AdminWorkspaceTokenCounts
	ActingCandidates []AdminActingCandidateView
}

type AdminModifyWorkspaceUserInput struct {
	AdminTokenName string
	WorkspaceRef   string
	UserRef        string
	DisplayName    *string
}

// AdminListWorkspaces 列出全部 workspace（server admin 视角，不按 actor membership 过滤）。
// includeArchived=false 时只返回未归档 workspace。
func (s *Service) AdminListWorkspaces(includeArchived bool) ([]AdminWorkspaceSummaryView, error) {
	rows, err := s.workspaceRepo.ListAll(includeArchived)
	if err != nil {
		return nil, err
	}
	tokens, err := s.tokenRepo.ListAll(true)
	if err != nil {
		return nil, err
	}
	tokenCountsByWorkspace := classifyTokensByWorkspace(tokens, s.clock.Unix())
	createdByIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.CreatedByUserID != nil {
			createdByIDs = append(createdByIDs, *row.CreatedByUserID)
		}
	}
	createdByInfos, err := s.resolveUserInfos(createdByIDs)
	if err != nil {
		return nil, err
	}
	out := make([]AdminWorkspaceSummaryView, 0, len(rows))
	for _, row := range rows {
		memberCounts, err := s.workspaceRepo.CountMembersByRole(row.ID)
		if err != nil {
			return nil, err
		}
		summary := AdminWorkspaceSummaryView{
			ID:          row.ID,
			Slug:        row.Slug,
			Name:        row.Name,
			Description: row.Description,
			Visibility:  row.Visibility,
			ArchivedAt:  row.ArchivedAt,
			CreatedAt:   row.CreatedAt,
			ModifiedAt:  row.ModifiedAt,
			MemberCounts: AdminWorkspaceMemberCounts{
				Owner:  memberCounts["owner"],
				Admin:  memberCounts["admin"],
				Member: memberCounts["member"],
				Viewer: memberCounts["viewer"],
			},
			TokenCounts: tokenCountsByWorkspace[row.ID],
		}
		if row.CreatedByUserID != nil {
			ui := createdByInfos[*row.CreatedByUserID]
			summary.CreatedBy = &ui
		}
		out = append(out, summary)
	}
	return out, nil
}

// AdminWorkspaceInfo 返回 workspace 详情：成员摘要、token 计数、acting candidates。
// workspace ref 支持 slug 或 UUID。
func (s *Service) AdminWorkspaceInfo(workspaceRef string) (AdminWorkspaceDetailView, error) {
	workspace, err := lookupWorkspace(s.workspaceRepo, strings.TrimSpace(workspaceRef))
	if err != nil {
		return AdminWorkspaceDetailView{}, err
	}
	members, err := s.memberRepo.List(workspace.ID)
	if err != nil {
		return AdminWorkspaceDetailView{}, err
	}
	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, m.User.ID)
	}
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return AdminWorkspaceDetailView{}, err
	}
	extByUser, err := s.loadExternalIDsByUsers(userIDs)
	if err != nil {
		return AdminWorkspaceDetailView{}, err
	}

	memberViews := make([]AdminWorkspaceMemberView, 0, len(members))
	candidates := make([]AdminActingCandidateView, 0)
	for _, m := range members {
		ui := userInfos[m.User.ID]
		if ui.ExternalIDs == nil {
			ui.ExternalIDs = extByUser[m.User.ID]
		}
		role := Role(m.Membership.Role)
		memberViews = append(memberViews, AdminWorkspaceMemberView{
			User:       ui,
			Role:       role,
			JoinedAt:   m.Membership.JoinedAt,
			ModifiedAt: m.Membership.ModifiedAt,
		})
		if role == RoleOwner || role == RoleAdmin {
			candidates = append(candidates, AdminActingCandidateView{User: ui, Role: role})
		}
	}

	tokens, err := s.tokenRepo.ListAll(true)
	if err != nil {
		return AdminWorkspaceDetailView{}, err
	}
	tokenCountsByWorkspace := classifyTokensByWorkspace(tokens, s.clock.Unix())

	return AdminWorkspaceDetailView{
		Workspace:        workspaceViewFromRow(workspace, "", false),
		Members:          memberViews,
		TokenCounts:      tokenCountsByWorkspace[workspace.ID],
		ActingCandidates: candidates,
	}, nil
}

func (s *Service) AdminModifyWorkspaceUser(input AdminModifyWorkspaceUserInput) (UserView, error) {
	adminTokenName := strings.TrimSpace(input.AdminTokenName)
	if adminTokenName == "" {
		return UserView{}, RuntimeError{Code: "admin_auth_required", Message: "admin token name is required"}
	}
	workspace, err := lookupWorkspace(s.workspaceRepo, strings.TrimSpace(input.WorkspaceRef))
	if err != nil {
		return UserView{}, err
	}
	user, err := s.resolveUser(input.UserRef)
	if err != nil {
		return UserView{}, err
	}
	if _, err := s.memberRepo.Get(user.ID, workspace.ID); err != nil {
		if err == storage.ErrNotFound {
			return UserView{}, RuntimeError{Code: authz.CodeMembershipNotFound, Message: "user is not a member of workspace"}
		}
		return UserView{}, err
	}
	if input.DisplayName == nil {
		return s.UserInfo(user.ID)
	}
	displayName := strings.TrimSpace(*input.DisplayName)
	err = s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		if err := txSvc.userRepo.UpdateDisplayName(user.ID, displayName, txSvc.clock.Unix()); err != nil {
			return err
		}
		return txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
			Action:      "admin.user.modify",
			WorkspaceID: &workspace.ID,
			TargetType:  "user",
			TargetID:    user.ID,
			Payload: map[string]any{
				"display_name": displayName,
			},
		}, adminTokenName)
	})
	if err != nil {
		return UserView{}, err
	}
	return s.UserInfo(user.ID)
}

// classifyTokensByWorkspace 把全部 token 按 workspace 统计 active/expired/revoked。
// 一个 token 的 workspace_ids 列表中的每个 workspace 都会 +1 计入。
// active = 未吊销且未过期；expired = 未吊销但已过期；revoked = 已吊销（吊销优先于过期）。
func classifyTokensByWorkspace(tokens []storage.ApiTokenEntry, now int64) map[string]AdminWorkspaceTokenCounts {
	out := map[string]AdminWorkspaceTokenCounts{}
	for _, token := range tokens {
		workspaceIDs := parseIDsFromJSON(token.WorkspaceIDsJSON)
		state := classifyTokenState(token, now)
		for _, workspaceID := range workspaceIDs {
			current := out[workspaceID]
			switch state {
			case tokenStateActive:
				current.Active++
			case tokenStateExpired:
				current.Expired++
			case tokenStateRevoked:
				current.Revoked++
			}
			out[workspaceID] = current
		}
	}
	return out
}

type tokenState int

const (
	tokenStateActive tokenState = iota
	tokenStateExpired
	tokenStateRevoked
)

func classifyTokenState(token storage.ApiTokenEntry, now int64) tokenState {
	if token.RevokedAt != nil {
		return tokenStateRevoked
	}
	if token.ExpiresAt != nil && *token.ExpiresAt < now {
		return tokenStateExpired
	}
	return tokenStateActive
}

// === Acting session service ===

// defaultActingSessionTTL 是 acting token 的默认有效期。
// 故意保持短：acting 是浏览器短期委托，不是长期 token。
const defaultActingSessionTTL = 2 * time.Hour
const maxTenantSwitchSessionTTL = 24 * time.Hour

// AdminCreateActingSessionInput 创建 acting session 的入参。
type AdminCreateActingSessionInput struct {
	AdminTokenID   *string
	AdminTokenName string
	WorkspaceRef   string
	// UserRef 可选；为空时选该 workspace 的第一个 owner，再退到第一个 admin。
	UserRef   string
	ExpiresIn *time.Duration
}

// AdminCreateActingSessionResult 是 acting session 创建结果。
// Token 是 raw acting token，只在创建响应中返回一次。
type AdminCreateActingSessionResult struct {
	SessionID      string
	Token          string
	TokenPrefix    string
	ExpiresAt      int64
	Workspace      WorkspaceView
	Actor          task.UserInfo
	Role           Role
	AdminTokenName string
}

type AdminCreateTenantAccessSessionInput struct {
	AdminTokenID   *string
	AdminTokenName string
	WorkspaceRef   string
	Name           string
	Scopes         []string
	ProjectRefs    []string
	ExpiresIn      *time.Duration
}

type AdminCreateTenantAccessSessionResult struct {
	Token          string
	View           TenantAccessTokenView
	Workspace      WorkspaceView
	AdminTokenID   *string
	AdminTokenName string
}

func (s *Service) AdminCreateTenantAccessSession(input AdminCreateTenantAccessSessionInput) (AdminCreateTenantAccessSessionResult, error) {
	workspaceRef := strings.TrimSpace(input.WorkspaceRef)
	adminTokenName := strings.TrimSpace(input.AdminTokenName)
	if adminTokenName == "" {
		return AdminCreateTenantAccessSessionResult{}, RuntimeError{Code: "admin_auth_required", Message: "admin token name is required"}
	}
	ttl := defaultActingSessionTTL
	if input.ExpiresIn != nil {
		if *input.ExpiresIn <= 0 || *input.ExpiresIn > maxTenantSwitchSessionTTL {
			return AdminCreateTenantAccessSessionResult{}, RuntimeError{Code: "tenant_console_session_invalid", Message: "expires_in must be between 1 second and 24 hours"}
		}
		ttl = *input.ExpiresIn
	}
	var result AdminCreateTenantAccessSessionResult
	err := s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		workspace, err := lookupWorkspace(txSvc.workspaceRepo, workspaceRef)
		if err != nil {
			return err
		}
		if workspace.ArchivedAt != nil {
			return RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
		}
		projectRefs := append([]string(nil), input.ProjectRefs...)
		projects, err := txSvc.resolveTokenProjects([]storage.Workspace{workspace}, projectRefs)
		if err != nil {
			return err
		}
		projectIDs := make([]string, 0, len(projects))
		for _, project := range projects {
			projectIDs = append(projectIDs, project.ID)
		}
		scopesInput := input.Scopes
		if len(scopesInput) == 0 {
			scopesInput = adminTenantSwitchDefaultScopes()
		}
		scopes, err := auth.ValidateTenantTokenScopes(scopesInput)
		if err != nil {
			return RuntimeError{Code: "tenant_token_scope_invalid", Message: err.Error()}
		}
		if scopes.Has(auth.ScopeTokenWrite) {
			return RuntimeError{Code: "tenant_token_scope_invalid", Message: "admin switch tenant token cannot have token:write scope"}
		}
		raw, prefix, hash, err := auth.GenerateToken(auth.TokenTypeTenantAccess)
		if err != nil {
			return err
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = "admin-switch:" + workspace.Slug + ":" + adminTokenName
		}
		now := txSvc.clock.Unix()
		expiresAt := now + int64(ttl.Seconds())
		scopesJSON, err := marshalStringSlice(scopes.Values())
		if err != nil {
			return err
		}
		workspaceJSON, err := marshalStringSlice([]string{workspace.ID})
		if err != nil {
			return err
		}
		projectJSON, err := marshalStringSlice(projectIDs)
		if err != nil {
			return err
		}
		stored := storage.ApiTokenEntry{
			ID:                     uuid.NewString(),
			UserID:                 nil,
			Name:                   name,
			Type:                   auth.TokenTypeTenantAccess,
			TokenPrefix:            prefix,
			TokenHash:              hash,
			ScopesJSON:             scopesJSON,
			WorkspaceIDsJSON:       workspaceJSON,
			ProjectIDsJSON:         projectJSON,
			IssuedVia:              "server_admin",
			IssuedByAdminTokenID:   input.AdminTokenID,
			IssuedByAdminTokenName: &adminTokenName,
			Purpose:                "admin_tenant_switch",
			CreatedAt:              now,
			ExpiresAt:              &expiresAt,
		}
		if err := txSvc.tokenRepo.Create(stored); err != nil {
			return err
		}
		if err := txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
			Action:      "admin.tenant_token.session_create",
			WorkspaceID: &workspace.ID,
			TargetType:  "tenant_token",
			TargetID:    stored.ID,
			Payload: map[string]any{
				"workspace_id":   workspace.ID,
				"workspace_slug": workspace.Slug,
				"token_name":     stored.Name,
				"purpose":        stored.Purpose,
				"expires_at":     expiresAt,
			},
		}, adminTokenName); err != nil {
			return err
		}
		result = AdminCreateTenantAccessSessionResult{
			Token:          raw,
			View:           tenantTokenViewFromEntry(stored, scopes.Values(), workspace.ID, projectIDs),
			Workspace:      workspaceViewFromRow(workspace, "", false),
			AdminTokenID:   input.AdminTokenID,
			AdminTokenName: adminTokenName,
		}
		return nil
	})
	return result, err
}

func adminTenantSwitchDefaultScopes() []string {
	return []string{
		auth.ScopeTaskRead, auth.ScopeTaskWrite,
		auth.ScopeProjectRead, auth.ScopeProjectWrite,
		auth.ScopeContextRead, auth.ScopeContextWrite,
		auth.ScopeConfigRead, auth.ScopeConfigWrite,
		auth.ScopeWorkspaceRead, auth.ScopeWorkspaceWrite,
		auth.ScopeAuditRead,
		auth.ScopeUserRead, auth.ScopeUserWrite,
		auth.ScopeMemberRead, auth.ScopeMemberWrite,
		auth.ScopeTokenRead,
		auth.ScopeHookRead,
		auth.ScopeNotificationRead,
		auth.ScopeReminderRead,
	}
}

// AdminCreateActingSession 为 owner/admin 用户创建短期 acting session。
// raw acting token 只在结果中返回一次；存储层只保存 hash。
func (s *Service) AdminCreateActingSession(input AdminCreateActingSessionInput) (AdminCreateActingSessionResult, error) {
	workspaceRef := strings.TrimSpace(input.WorkspaceRef)
	adminTokenName := strings.TrimSpace(input.AdminTokenName)
	if adminTokenName == "" {
		return AdminCreateActingSessionResult{}, RuntimeError{Code: "admin_auth_required", Message: "admin token name is required"}
	}
	ttl := defaultActingSessionTTL
	if input.ExpiresIn != nil {
		if *input.ExpiresIn <= 0 {
			return AdminCreateActingSessionResult{}, RuntimeError{Code: "admin_token_ttl_invalid", Message: "expires_in must be a positive duration"}
		}
		ttl = *input.ExpiresIn
	}

	var result AdminCreateActingSessionResult
	err := s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		workspace, err := lookupWorkspace(txSvc.workspaceRepo, workspaceRef)
		if err != nil {
			return err
		}
		if workspace.ArchivedAt != nil {
			return RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
		}
		actor, role, err := txSvc.resolveActingActor(workspace, strings.TrimSpace(input.UserRef))
		if err != nil {
			return err
		}
		raw, prefix, hash, err := auth.GenerateActingToken()
		if err != nil {
			return err
		}
		now := txSvc.clock.Unix()
		expiresAt := now + int64(ttl.Seconds())
		sessionID := uuid.NewString()
		if err := txSvc.adminActingSessionRepo.Create(storage.AdminActingSessionEntry{
			ID:             sessionID,
			TokenPrefix:    prefix,
			TokenHash:      hash,
			AdminTokenID:   input.AdminTokenID,
			AdminTokenName: adminTokenName,
			WorkspaceID:    workspace.ID,
			ActorUserID:    actor.ID,
			Role:           string(role),
			CreatedAt:      now,
			ExpiresAt:      expiresAt,
		}); err != nil {
			return err
		}
		extByUser, err := txSvc.loadExternalIDsByUsers([]string{actor.ID})
		if err != nil {
			return err
		}
		actorInfo := task.UserInfo{
			ID:          actor.ID,
			Name:        actor.Name,
			DisplayName: actor.DisplayName,
			Email:       actor.Email,
			ExternalIDs: extByUser[actor.ID],
		}
		if err := txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
			Action:      "admin.acting_session.create",
			WorkspaceID: &workspace.ID,
			TargetType:  "admin_acting_session",
			TargetID:    sessionID,
			Payload: map[string]any{
				"workspace_id":   workspace.ID,
				"workspace_slug": workspace.Slug,
				"actor_user_id":  actor.ID,
				"role":           string(role),
				"expires_at":     expiresAt,
			},
		}, adminTokenName); err != nil {
			return err
		}
		result = AdminCreateActingSessionResult{
			SessionID:      sessionID,
			Token:          raw,
			TokenPrefix:    prefix,
			ExpiresAt:      expiresAt,
			Workspace:      workspaceViewFromRow(workspace, "", false),
			Actor:          actorInfo,
			Role:           role,
			AdminTokenName: adminTokenName,
		}
		return nil
	})
	return result, err
}

// AdminRevokeActingSession 吊销一个 acting session。
func (s *Service) AdminRevokeActingSession(sessionID, adminTokenName string) error {
	sessionID = strings.TrimSpace(sessionID)
	return s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		session, err := txSvc.adminActingSessionRepo.GetByID(sessionID)
		if err != nil {
			if err == storage.ErrNotFound {
				return RuntimeError{Code: "admin_acting_session_not_found", Message: "acting session not found"}
			}
			return err
		}
		if err := txSvc.adminActingSessionRepo.Revoke(sessionID, txSvc.clock.Unix()); err != nil {
			return err
		}
		return txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
			Action:      "admin.acting_session.revoke",
			WorkspaceID: &session.WorkspaceID,
			TargetType:  "admin_acting_session",
			TargetID:    sessionID,
			Payload: map[string]any{
				"actor_user_id": session.ActorUserID,
			},
		}, adminTokenName)
	})
}

// resolveActingActor 解析 acting session 要扮演的用户。
// userRef 为空时按 owner → admin 顺序选第一个候选。
// 指定 userRef 时必须是该 workspace 的 owner 或 admin，否则 admin_acting_target_invalid。
func (s *Service) resolveActingActor(workspace storage.Workspace, userRef string) (storage.User, Role, error) {
	members, err := s.memberRepo.List(workspace.ID)
	if err != nil {
		return storage.User{}, "", err
	}
	if userRef == "" {
		// owner 优先，再退到 admin。
		for _, m := range members {
			if Role(m.Membership.Role) == RoleOwner {
				return m.User, RoleOwner, nil
			}
		}
		for _, m := range members {
			if Role(m.Membership.Role) == RoleAdmin {
				return m.User, RoleAdmin, nil
			}
		}
		return storage.User{}, "", RuntimeError{Code: "admin_acting_target_invalid", Message: "workspace has no owner or admin to act as"}
	}
	user, err := s.resolveUser(userRef)
	if err != nil {
		return storage.User{}, "", RuntimeError{Code: "admin_acting_target_invalid", Message: "acting target is not an owner or admin of this workspace"}
	}
	for _, m := range members {
		if m.User.ID != user.ID {
			continue
		}
		role := Role(m.Membership.Role)
		if role == RoleOwner || role == RoleAdmin {
			return m.User, role, nil
		}
		return storage.User{}, "", RuntimeError{Code: "admin_acting_target_invalid", Message: "acting target is not an owner or admin of this workspace"}
	}
	return storage.User{}, "", RuntimeError{Code: "admin_acting_target_invalid", Message: "acting target is not a member of this workspace"}
}
