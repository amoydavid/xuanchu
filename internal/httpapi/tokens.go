package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type tokenResponse struct {
	ID           string            `json:"id"`
	Prefix       string            `json:"prefix"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	User         task.JSONUserInfo `json:"user"`
	WorkspaceIDs []string          `json:"workspace_ids"`
	ProjectIDs   []string          `json:"project_ids"`
	Scopes       []string          `json:"scopes"`
	CreatedAt    int64             `json:"created_at"`
	ExpiresAt    *int64            `json:"expires_at,omitempty"`
	RevokedAt    *int64            `json:"revoked_at,omitempty"`
	LastUsedAt   *int64            `json:"last_used_at,omitempty"`
}

type modifyTokenRequest struct {
	Name             *string   `json:"name,omitempty"`
	Scopes           *[]string `json:"scopes,omitempty"`
	Workspaces       *[]string `json:"workspaces,omitempty"`
	WorkspaceIDs     *[]string `json:"workspace_ids,omitempty"`
	Projects         *[]string `json:"projects,omitempty"`
	ProjectIDs       *[]string `json:"project_ids,omitempty"`
	ExpiresInSeconds *int64    `json:"expires_in_seconds,omitempty"`
}

type createTokenRequest struct {
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"`
	User             string   `json:"user,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	WorkspaceRefs    []string `json:"workspaces,omitempty"`
	WorkspaceIDs     []string `json:"workspace_ids,omitempty"`
	ProjectRefs      []string `json:"projects,omitempty"`
	ProjectIDs       []string `json:"project_ids,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

type createdTokenResponse struct {
	Token string `json:"token"`
	tokenResponse
}

func (s *Server) handleTokenList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTokenRead, app.PermissionTokenRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListTokens(app.ListTokensInput{
		IncludeRevoked: r.URL.Query().Get("all") == "true",
	})
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

func (s *Server) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, authn, err := s.scopedService(r, auth.ScopeTokenWrite, app.PermissionTokenWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	var ttl *time.Duration
	if req.ExpiresInSeconds != nil {
		value := time.Duration(*req.ExpiresInSeconds) * time.Second
		ttl = &value
	}
	workspaceRefs := append([]string(nil), req.WorkspaceRefs...)
	workspaceRefs = append(workspaceRefs, req.WorkspaceIDs...)
	projectRefs := append([]string(nil), req.ProjectRefs...)
	projectRefs = append(projectRefs, req.ProjectIDs...)
	created, err := scoped.CreateToken(app.CreateTokenInput{
		Name:          req.Name,
		Type:          req.Type,
		UserRef:       req.User,
		Scopes:        req.Scopes,
		WorkspaceRefs: workspaceRefs,
		ProjectRefs:   projectRefs,
		ExpiresIn:     ttl,
		ParentToken:   &authn.Authn.Token,
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

func (s *Server) handleTokenRevoke(w http.ResponseWriter, r *http.Request) {
	scoped, authn, err := s.scopedService(r, auth.ScopeTokenWrite, app.PermissionTokenWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.RevokeTokenWithLimit(chi.URLParam(r, "tokenRef"), &authn.Authn.Token); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleTokenModify(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTokenWrite, app.PermissionTokenWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req modifyTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	var ttl *time.Duration
	if req.ExpiresInSeconds != nil {
		value := time.Duration(*req.ExpiresInSeconds) * time.Second
		ttl = &value
	}
	view, err := scoped.ModifyToken(app.ModifyTokenInput{
		TokenID:       chi.URLParam(r, "tokenRef"),
		Name:          req.Name,
		Scopes:        req.Scopes,
		WorkspaceRefs: mergeRefs(req.Workspaces, req.WorkspaceIDs),
		ProjectRefs:   mergeRefs(req.Projects, req.ProjectIDs),
		ExpiresIn:     ttl,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tokenResponseFromView(*view), nil)
}

func tokenResponseFromView(view app.TokenView) tokenResponse {
	return tokenResponse{
		ID:           view.ID,
		Prefix:       view.Prefix,
		Name:         view.Name,
		Type:         view.Type,
		User:         task.UserInfoToJSON(view.User),
		WorkspaceIDs: append([]string(nil), view.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), view.ProjectIDs...),
		Scopes:       append([]string(nil), view.Scopes...),
		CreatedAt:    view.CreatedAt,
		ExpiresAt:    view.ExpiresAt,
		RevokedAt:    view.RevokedAt,
		LastUsedAt:   view.LastUsedAt,
	}
}

// mergeRefs 合并两组 ref（slugs 与 ids）。
// 两者都为 nil 时返回 nil（表示「不修改」）；
// 任一非 nil 时合并去重并返回非 nil（含结果为空切片，表示「清空」）。
func mergeRefs(primary, secondary *[]string) *[]string {
	if primary == nil && secondary == nil {
		return nil
	}
	merged := append([]string(nil), derefStrings(primary)...)
	merged = append(merged, derefStrings(secondary)...)
	seen := make(map[string]struct{}, len(merged))
	out := make([]string, 0, len(merged))
	for _, ref := range merged {
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return &out
}

func derefStrings(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}
