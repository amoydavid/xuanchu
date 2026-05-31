package remote

import (
	"context"
	"net/url"

	"github.com/dajee/taskg/internal/app"
)

type tokenDTO struct {
	ID           string   `json:"id"`
	Prefix       string   `json:"prefix"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	UserID       string   `json:"user_id"`
	WorkspaceIDs []string `json:"workspace_ids"`
	ProjectIDs   []string `json:"project_ids"`
	Scopes       []string `json:"scopes"`
	CreatedAt    int64    `json:"created_at"`
	ExpiresAt    *int64   `json:"expires_at"`
	RevokedAt    *int64   `json:"revoked_at"`
	LastUsedAt   *int64   `json:"last_used_at"`
}

func (c *Client) ListTokens(ctx context.Context, workspace string, includeRevoked bool) ([]app.TokenView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if includeRevoked {
		values.Set("all", "true")
	}
	var envelope apiEnvelope[[]tokenDTO]
	if err := c.get(ctx, "/api/v1/tokens", values, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.TokenView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		out = append(out, tokenDTOToView(row))
	}
	return out, nil
}

func tokenDTOToView(row tokenDTO) app.TokenView {
	return app.TokenView{
		ID:           row.ID,
		Prefix:       row.Prefix,
		Name:         row.Name,
		Type:         row.Type,
		UserID:       row.UserID,
		WorkspaceIDs: append([]string(nil), row.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), row.ProjectIDs...),
		Scopes:       append([]string(nil), row.Scopes...),
		CreatedAt:    row.CreatedAt,
		ExpiresAt:    row.ExpiresAt,
		RevokedAt:    row.RevokedAt,
		LastUsedAt:   row.LastUsedAt,
	}
}
