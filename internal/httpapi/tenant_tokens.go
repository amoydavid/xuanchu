package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type tenantTokenResponse struct {
	ID                 string                    `json:"id"`
	Prefix             string                    `json:"prefix"`
	Name               string                    `json:"name"`
	Type               string                    `json:"type"`
	WorkspaceID        string                    `json:"workspace_id"`
	ProjectIDs         []string                  `json:"project_ids"`
	Scopes             []string                  `json:"scopes"`
	CreatedAt          int64                     `json:"created_at"`
	ExpiresAt          *int64                    `json:"expires_at,omitempty"`
	RevokedAt          *int64                    `json:"revoked_at,omitempty"`
	LastUsedAt         *int64                    `json:"last_used_at,omitempty"`
	IssuedVia          string                    `json:"issued_via,omitempty"`
	IssuedByAdminToken *adminTokenIssuerResponse `json:"issued_by_admin_token,omitempty"`
	Purpose            string                    `json:"purpose,omitempty"`
}

type createTenantTokenRequest struct {
	Name             string   `json:"name"`
	Scopes           []string `json:"scopes,omitempty"`
	Projects         []string `json:"projects,omitempty"`
	ProjectIDs       []string `json:"project_ids,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

type modifyTenantTokenRequest struct {
	Name             *string       `json:"name,omitempty"`
	Scopes           *[]string     `json:"scopes,omitempty"`
	Projects         *[]string     `json:"projects,omitempty"`
	ProjectIDs       *[]string     `json:"project_ids,omitempty"`
	ExpiresInSeconds optionalInt64 `json:"expires_in_seconds,omitempty"`
}

type createdTenantTokenResponse struct {
	Token string `json:"token"`
	tenantTokenResponse
}

func (s *Server) handleTenantTokenList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTokenRead, app.PermissionTokenRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListTenantAccessTokens(app.ListTenantAccessTokensInput{
		IncludeRevoked: r.URL.Query().Get("all") == "true",
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]tenantTokenResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, tenantTokenResponseFromView(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleTenantTokenCreate(w http.ResponseWriter, r *http.Request) {
	var req createTenantTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTokenWrite, app.PermissionTokenWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	var ttl *time.Duration
	if req.ExpiresInSeconds != nil {
		value := time.Duration(*req.ExpiresInSeconds) * time.Second
		ttl = &value
	}
	projectRefs := append([]string(nil), req.Projects...)
	projectRefs = append(projectRefs, req.ProjectIDs...)
	created, err := scoped.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:        req.Name,
		Scopes:      req.Scopes,
		ProjectRefs: projectRefs,
		ExpiresIn:   ttl,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, createdTenantTokenResponse{
		Token:               created.RawToken,
		tenantTokenResponse: tenantTokenResponseFromView(created.View),
	}, nil)
}

func (s *Server) handleTenantTokenModify(w http.ResponseWriter, r *http.Request) {
	var req modifyTenantTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTokenWrite, app.PermissionTokenWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	ttl := durationFromOptionalSeconds(req.ExpiresInSeconds)
	view, err := scoped.ModifyTenantAccessToken(app.ModifyTenantAccessTokenInput{
		TokenRef:    chi.URLParam(r, "tokenRef"),
		Name:        req.Name,
		Scopes:      req.Scopes,
		ProjectRefs: mergeRefs(req.Projects, req.ProjectIDs),
		ExpiresIn:   ttl,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tenantTokenResponseFromView(*view), nil)
}

func (s *Server) handleTenantTokenRevoke(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTokenWrite, app.PermissionTokenWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.RevokeTenantAccessToken(chi.URLParam(r, "tokenRef")); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func tenantTokenResponseFromView(view app.TenantAccessTokenView) tenantTokenResponse {
	var issuedBy *adminTokenIssuerResponse
	if view.IssuedByAdminTokenID != nil || view.IssuedByAdminTokenName != nil {
		issuedBy = &adminTokenIssuerResponse{
			ID:   derefString(view.IssuedByAdminTokenID),
			Name: derefString(view.IssuedByAdminTokenName),
		}
	}
	return tenantTokenResponse{
		ID:                 view.ID,
		Prefix:             view.Prefix,
		Name:               view.Name,
		Type:               view.Type,
		WorkspaceID:        view.WorkspaceID,
		ProjectIDs:         append([]string(nil), view.ProjectIDs...),
		Scopes:             append([]string(nil), view.Scopes...),
		CreatedAt:          view.CreatedAt,
		ExpiresAt:          view.ExpiresAt,
		RevokedAt:          view.RevokedAt,
		LastUsedAt:         view.LastUsedAt,
		IssuedVia:          view.IssuedVia,
		IssuedByAdminToken: issuedBy,
		Purpose:            view.Purpose,
	}
}

type adminTokenIssuerResponse struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
