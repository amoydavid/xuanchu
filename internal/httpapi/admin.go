package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type adminCreateWorkspaceRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Owner       struct {
		Name  string `json:"name"`
		Email string `json:"email,omitempty"`
	} `json:"owner"`
}

type adminCreateWorkspaceAdminRequest struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

type adminModifyWorkspaceUserRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
}

type adminUserResponse struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	DisplayName        string               `json:"display_name"`
	Email              *string              `json:"email,omitempty"`
	DefaultWorkspaceID *string              `json:"default_workspace_id,omitempty"`
	ExternalIDs        []externalIDResponse `json:"external_ids"`
	Active             bool                 `json:"active"`
	CreatedAt          int64                `json:"created_at"`
	ModifiedAt         int64                `json:"modified_at"`
}

type adminCreateAgentTokenRequest struct {
	Name             string   `json:"name"`
	User             string   `json:"user,omitempty"`
	Scopes           []string `json:"scopes"`
	ProjectRefs      []string `json:"project_refs,omitempty"`
	ProjectIDs       []string `json:"project_ids,omitempty"`
	ExpiresIn        string   `json:"expires_in,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	admin, _ := adminAuthFromContext(r.Context())
	writeSuccess(w, http.StatusOK, map[string]any{
		"token_name": admin.TokenName,
		"capabilities": []string{
			"workspace:create",
			"workspace:list",
			"workspace:read",
			"workspace_admin:create",
			"agent_token:create",
			"acting_session:create",
			"acting_session:revoke",
			"token:list",
			"token:modify",
			"token:revoke",
		},
	}, nil)
}

func (s *Server) handleAdminWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := app.NewService(app.ServiceOptions{
		Store:                 s.store,
		Clock:                 s.effectiveClock(),
		Runtime:               &app.RuntimeContext{ActorName: "server-admin"},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	result, err := svc.AdminCreateWorkspace(app.AdminCreateWorkspaceInput{
		AdminTokenName: admin.TokenName,
		Slug:           req.Slug,
		Name:           req.Name,
		Description:    req.Description,
		Visibility:     req.Visibility,
		Owner: app.AdminOwnerInput{
			Name:  req.Owner.Name,
			Email: req.Owner.Email,
		},
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, map[string]any{
		"workspace": workspaceResponseFromView(result.Workspace),
		"owner":     userResponseFromView(result.Owner),
	}, nil)
}

func (s *Server) handleAdminWorkspaceAdminCreate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateWorkspaceAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := app.NewService(app.ServiceOptions{
		Store:                 s.store,
		Clock:                 s.effectiveClock(),
		Runtime:               &app.RuntimeContext{ActorName: "server-admin"},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	result, err := svc.AdminCreateWorkspaceAdmin(app.AdminCreateWorkspaceAdminInput{
		AdminTokenName: admin.TokenName,
		WorkspaceRef:   chi.URLParam(r, "workspace"),
		Name:           req.Name,
		Email:          req.Email,
		Role:           app.Role(req.Role),
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, map[string]any{
		"workspace": workspaceResponseFromView(result.Workspace),
		"admin":     adminUserResponseFromView(result.Admin),
		"membership": map[string]any{
			"role":      result.Membership.Role,
			"joined_at": result.Membership.JoinedAt,
		},
	}, nil)
}

func (s *Server) handleAdminAgentTokenCreate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateAgentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	ttl, ok := parseAdminTokenTTL(w, req)
	if !ok {
		return
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := app.NewService(app.ServiceOptions{
		Store:                 s.store,
		Clock:                 s.effectiveClock(),
		Runtime:               &app.RuntimeContext{ActorName: "server-admin"},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	projectRefs := append([]string(nil), req.ProjectRefs...)
	projectRefs = append(projectRefs, req.ProjectIDs...)
	created, err := svc.AdminCreateWorkspaceAgentToken(app.AdminCreateAgentTokenInput{
		AdminTokenName: admin.TokenName,
		WorkspaceRef:   chi.URLParam(r, "workspace"),
		Name:           req.Name,
		UserRef:        req.User,
		Scopes:         req.Scopes,
		ProjectRefs:    projectRefs,
		ExpiresIn:      ttl,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, createdTokenResponse{
		Token:         created.RawToken,
		tokenResponse: tokenResponseFromView(created.View),
	}, nil)
}

func parseAdminTokenTTL(w http.ResponseWriter, req adminCreateAgentTokenRequest) (*time.Duration, bool) {
	if req.ExpiresIn != "" {
		value, err := time.ParseDuration(req.ExpiresIn)
		if err != nil || value <= 0 {
			writeError(w, http.StatusBadRequest, "admin_token_ttl_invalid", "expires_in is invalid", nil)
			return nil, false
		}
		return &value, true
	}
	if req.ExpiresInSeconds != nil {
		if *req.ExpiresInSeconds <= 0 {
			writeError(w, http.StatusBadRequest, "admin_token_ttl_invalid", "expires_in_seconds is invalid", nil)
			return nil, false
		}
		value := time.Duration(*req.ExpiresInSeconds) * time.Second
		return &value, true
	}
	return nil, true
}

// adminWorkspaceSummaryResponse 是 admin workspace 列表的一行。
type adminWorkspaceSummaryResponse struct {
	ID           string             `json:"id"`
	Slug         string             `json:"slug"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Visibility   string             `json:"visibility"`
	CreatedBy    *task.JSONUserInfo `json:"created_by,omitempty"`
	MemberCounts adminMemberCounts  `json:"member_counts"`
	TokenCounts  adminTokenCounts   `json:"token_counts"`
	ArchivedAt   *int64             `json:"archived_at,omitempty"`
	CreatedAt    int64              `json:"created_at"`
	ModifiedAt   int64              `json:"modified_at"`
}

type adminMemberCounts struct {
	Owner  int64 `json:"owner"`
	Admin  int64 `json:"admin"`
	Member int64 `json:"member"`
	Viewer int64 `json:"viewer"`
}

type adminTokenCounts struct {
	Active  int64 `json:"active"`
	Expired int64 `json:"expired"`
	Revoked int64 `json:"revoked"`
}

type adminWorkspaceMemberResponse struct {
	User       task.JSONUserInfo `json:"user"`
	Role       string            `json:"role"`
	JoinedAt   int64             `json:"joined_at"`
	ModifiedAt int64             `json:"modified_at"`
}

type adminActingCandidateResponse struct {
	User task.JSONUserInfo `json:"user"`
	Role string            `json:"role"`
}

type adminWorkspaceDetailResponse struct {
	Workspace        workspaceResponse              `json:"workspace"`
	Members          []adminWorkspaceMemberResponse `json:"members"`
	TokenCounts      adminTokenCounts               `json:"token_counts"`
	ActingCandidates []adminActingCandidateResponse `json:"acting_candidates"`
}

type adminCreateActingSessionRequest struct {
	User             string `json:"user,omitempty"`
	ExpiresIn        string `json:"expires_in,omitempty"`
	ExpiresInSeconds *int64 `json:"expires_in_seconds,omitempty"`
}

type adminActingSessionResponse struct {
	Token          string            `json:"token"`
	ExpiresAt      int64             `json:"expires_at"`
	Workspace      workspaceResponse `json:"workspace"`
	Actor          task.JSONUserInfo `json:"actor"`
	Role           string            `json:"role"`
	AdminTokenName string            `json:"admin_token_name"`
}

func newAdminWorkspaceService(s *Server, r *http.Request) (*app.Service, error) {
	return app.NewService(app.ServiceOptions{
		Store:                 s.store,
		Clock:                 s.effectiveClock(),
		Runtime:               &app.RuntimeContext{ActorName: "server-admin"},
		DisableScopeBootstrap: true,
	})
}

func (s *Server) handleAdminWorkspaceList(w http.ResponseWriter, r *http.Request) {
	svc, err := newAdminWorkspaceService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := svc.AdminListWorkspaces(r.URL.Query().Get("all") == "true")
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]adminWorkspaceSummaryResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, adminWorkspaceSummaryResponseFromView(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleAdminWorkspaceInfo(w http.ResponseWriter, r *http.Request) {
	svc, err := newAdminWorkspaceService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	detail, err := svc.AdminWorkspaceInfo(chi.URLParam(r, "workspace"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	members := make([]adminWorkspaceMemberResponse, 0, len(detail.Members))
	for _, m := range detail.Members {
		members = append(members, adminWorkspaceMemberResponse{
			User:       task.UserInfoToJSON(m.User),
			Role:       string(m.Role),
			JoinedAt:   m.JoinedAt,
			ModifiedAt: m.ModifiedAt,
		})
	}
	candidates := make([]adminActingCandidateResponse, 0, len(detail.ActingCandidates))
	for _, c := range detail.ActingCandidates {
		candidates = append(candidates, adminActingCandidateResponse{
			User: task.UserInfoToJSON(c.User),
			Role: string(c.Role),
		})
	}
	writeSuccess(w, http.StatusOK, adminWorkspaceDetailResponse{
		Workspace:        workspaceResponseFromView(detail.Workspace),
		Members:          members,
		TokenCounts:      adminTokenCounts(detail.TokenCounts),
		ActingCandidates: candidates,
	}, nil)
}

func (s *Server) handleAdminWorkspaceUserModify(w http.ResponseWriter, r *http.Request) {
	var req adminModifyWorkspaceUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := newAdminWorkspaceService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := svc.AdminModifyWorkspaceUser(app.AdminModifyWorkspaceUserInput{
		AdminTokenName: admin.TokenName,
		WorkspaceRef:   chi.URLParam(r, "workspace"),
		UserRef:        chi.URLParam(r, "user"),
		DisplayName:    req.DisplayName,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, adminUserResponseFromView(user), nil)
}

func (s *Server) handleAdminActingSessionCreate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateActingSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	ttl, ok := parseAdminActingTTL(w, req)
	if !ok {
		return
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := newAdminWorkspaceService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	created, err := svc.AdminCreateActingSession(app.AdminCreateActingSessionInput{
		AdminTokenID:   admin.TokenID,
		AdminTokenName: admin.TokenName,
		WorkspaceRef:   chi.URLParam(r, "workspace"),
		UserRef:        req.User,
		ExpiresIn:      ttl,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, adminActingSessionResponse{
		Token:          created.Token,
		ExpiresAt:      created.ExpiresAt,
		Workspace:      workspaceResponseFromView(created.Workspace),
		Actor:          task.UserInfoToJSON(created.Actor),
		Role:           string(created.Role),
		AdminTokenName: created.AdminTokenName,
	}, nil)
}

func (s *Server) handleAdminActingSessionRevoke(w http.ResponseWriter, r *http.Request) {
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := newAdminWorkspaceService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := svc.AdminRevokeActingSession(chi.URLParam(r, "sessionID"), admin.TokenName); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func parseAdminActingTTL(w http.ResponseWriter, req adminCreateActingSessionRequest) (*time.Duration, bool) {
	if req.ExpiresIn != "" {
		value, err := time.ParseDuration(req.ExpiresIn)
		if err != nil || value <= 0 {
			writeError(w, http.StatusBadRequest, "admin_token_ttl_invalid", "expires_in is invalid", nil)
			return nil, false
		}
		return &value, true
	}
	if req.ExpiresInSeconds != nil {
		if *req.ExpiresInSeconds <= 0 {
			writeError(w, http.StatusBadRequest, "admin_token_ttl_invalid", "expires_in_seconds is invalid", nil)
			return nil, false
		}
		value := time.Duration(*req.ExpiresInSeconds) * time.Second
		return &value, true
	}
	return nil, true
}

func adminWorkspaceSummaryResponseFromView(row app.AdminWorkspaceSummaryView) adminWorkspaceSummaryResponse {
	var createdBy *task.JSONUserInfo
	if row.CreatedBy != nil {
		jui := task.UserInfoToJSON(*row.CreatedBy)
		createdBy = &jui
	}
	return adminWorkspaceSummaryResponse{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Visibility:  row.Visibility,
		CreatedBy:   createdBy,
		MemberCounts: adminMemberCounts{
			Owner:  row.MemberCounts.Owner,
			Admin:  row.MemberCounts.Admin,
			Member: row.MemberCounts.Member,
			Viewer: row.MemberCounts.Viewer,
		},
		TokenCounts: adminTokenCounts{
			Active:  row.TokenCounts.Active,
			Expired: row.TokenCounts.Expired,
			Revoked: row.TokenCounts.Revoked,
		},
		ArchivedAt: row.ArchivedAt,
		CreatedAt:  row.CreatedAt,
		ModifiedAt: row.ModifiedAt,
	}
}

func adminUserResponseFromView(user app.UserView) adminUserResponse {
	extIDs := make([]externalIDResponse, 0, len(user.ExternalIDs))
	for _, eid := range user.ExternalIDs {
		extIDs = append(extIDs, externalIDResponse{Provider: eid.Provider, ExternalID: eid.ExternalID})
	}
	return adminUserResponse{
		ID:                 user.ID,
		Name:               user.Name,
		DisplayName:        user.DisplayName,
		Email:              user.Email,
		DefaultWorkspaceID: user.DefaultWorkspaceID,
		ExternalIDs:        extIDs,
		Active:             user.Active,
		CreatedAt:          user.CreatedAt,
		ModifiedAt:         user.ModifiedAt,
	}
}

// adminModifyTokenRequest admin 修改 token 请求体。
// 与普通 modifyTokenRequest 区别：无 workspaces/projects（admin 不改绑定）。
type adminModifyTokenRequest struct {
	Name             *string   `json:"name,omitempty"`
	Scopes           *[]string `json:"scopes,omitempty"`
	ExpiresInSeconds *int64    `json:"expires_in_seconds,omitempty"`
}

func newAdminTokenService(s *Server, r *http.Request) (*app.Service, error) {
	return app.NewService(app.ServiceOptions{
		Store:                 s.store,
		Clock:                 s.effectiveClock(),
		Runtime:               &app.RuntimeContext{ActorName: "server-admin"},
		DisableScopeBootstrap: true,
	})
}

func (s *Server) handleAdminTokenList(w http.ResponseWriter, r *http.Request) {
	svc, err := newAdminTokenService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := svc.AdminListTokens(r.URL.Query().Get("all") == "true")
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]tokenResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, tokenResponseFromView(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleAdminTokenModify(w http.ResponseWriter, r *http.Request) {
	var req adminModifyTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := newAdminTokenService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var ttl *time.Duration
	if req.ExpiresInSeconds != nil {
		value := time.Duration(*req.ExpiresInSeconds) * time.Second
		ttl = &value
	}
	view, err := svc.AdminModifyToken(app.AdminModifyTokenInput{
		TokenRef:       chi.URLParam(r, "tokenRef"),
		Name:           req.Name,
		Scopes:         req.Scopes,
		ExpiresIn:      ttl,
		AdminTokenName: admin.TokenName,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tokenResponseFromView(*view), nil)
}

func (s *Server) handleAdminTokenRevoke(w http.ResponseWriter, r *http.Request) {
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := newAdminTokenService(s, r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := svc.AdminRevokeToken(chi.URLParam(r, "tokenRef"), admin.TokenName); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}
