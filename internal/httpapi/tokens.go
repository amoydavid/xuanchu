package httpapi

import (
	"net/http"

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
