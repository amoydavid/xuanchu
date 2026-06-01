package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dajee/taskg/internal/app"
)

type tokenResponse struct {
	ID           string   `json:"id"`
	Prefix       string   `json:"prefix"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	UserID       string   `json:"user_id"`
	WorkspaceIDs []string `json:"workspace_ids"`
	ProjectIDs   []string `json:"project_ids"`
	Scopes       []string `json:"scopes"`
	CreatedAt    int64    `json:"created_at"`
	ExpiresAt    *int64   `json:"expires_at,omitempty"`
	RevokedAt    *int64   `json:"revoked_at,omitempty"`
	LastUsedAt   *int64   `json:"last_used_at,omitempty"`
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
	scoped, _, err := s.scopedService(r, "token:read", app.PermissionTokenRead, "")
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
	scoped, authn, err := s.scopedService(r, "token:write", app.PermissionTokenWrite, "")
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
	scoped, authn, err := s.scopedService(r, "token:write", app.PermissionTokenWrite, "")
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

func tokenResponseFromView(view app.TokenView) tokenResponse {
	return tokenResponse{
		ID:           view.ID,
		Prefix:       view.Prefix,
		Name:         view.Name,
		Type:         view.Type,
		UserID:       view.UserID,
		WorkspaceIDs: append([]string(nil), view.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), view.ProjectIDs...),
		Scopes:       append([]string(nil), view.Scopes...),
		CreatedAt:    view.CreatedAt,
		ExpiresAt:    view.ExpiresAt,
		RevokedAt:    view.RevokedAt,
		LastUsedAt:   view.LastUsedAt,
	}
}
