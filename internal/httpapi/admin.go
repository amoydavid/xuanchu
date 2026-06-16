package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
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

type adminUserResponse struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
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
			"workspace_admin:create",
			"agent_token:create",
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

func adminUserResponseFromView(user app.UserView) adminUserResponse {
	extIDs := make([]externalIDResponse, 0, len(user.ExternalIDs))
	for _, eid := range user.ExternalIDs {
		extIDs = append(extIDs, externalIDResponse{Provider: eid.Provider, ExternalID: eid.ExternalID})
	}
	return adminUserResponse{
		ID:                 user.ID,
		Name:               user.Name,
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
