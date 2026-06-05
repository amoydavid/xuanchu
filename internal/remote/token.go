package remote

import (
	"context"
	"net/url"
	"time"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/task"
)

type tokenDTO struct {
	ID           string             `json:"id"`
	Prefix       string             `json:"prefix"`
	Name         string             `json:"name"`
	Type         string             `json:"type"`
	User         task.JSONUserInfo  `json:"user"`
	WorkspaceIDs []string           `json:"workspace_ids"`
	ProjectIDs   []string           `json:"project_ids"`
	Scopes       []string           `json:"scopes"`
	CreatedAt    int64              `json:"created_at"`
	ExpiresAt    *int64             `json:"expires_at"`
	RevokedAt    *int64             `json:"revoked_at"`
	LastUsedAt   *int64             `json:"last_used_at"`
}

type CreateTokenInput struct {
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"`
	User             string   `json:"user,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	WorkspaceIDs     []string `json:"workspace_ids,omitempty"`
	ProjectIDs       []string `json:"project_ids,omitempty"`
	ProjectRefs      []string `json:"projects,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

type CreatedToken struct {
	Token string
	View  app.TokenView
}

type createdTokenDTO struct {
	Token        string            `json:"token"`
	ID           string            `json:"id"`
	Prefix       string            `json:"prefix"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	User         task.JSONUserInfo `json:"user"`
	WorkspaceIDs []string          `json:"workspace_ids"`
	ProjectIDs   []string          `json:"project_ids"`
	Scopes       []string          `json:"scopes"`
	CreatedAt    int64             `json:"created_at"`
	ExpiresAt    *int64            `json:"expires_at"`
	RevokedAt    *int64            `json:"revoked_at"`
	LastUsedAt   *int64            `json:"last_used_at"`
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

func (c *Client) CreateToken(ctx context.Context, workspace string, input CreateTokenInput) (CreatedToken, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	path := "/api/v1/tokens"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var envelope apiEnvelope[createdTokenDTO]
	if err := c.post(ctx, path, input, &envelope); err != nil {
		return CreatedToken{}, err
	}
	return CreatedToken{Token: envelope.Data.Token, View: tokenDTOToView(tokenDTO{
		ID:           envelope.Data.ID,
		Prefix:       envelope.Data.Prefix,
		Name:         envelope.Data.Name,
		Type:         envelope.Data.Type,
		User:         envelope.Data.User,
		WorkspaceIDs: envelope.Data.WorkspaceIDs,
		ProjectIDs:   envelope.Data.ProjectIDs,
		Scopes:       envelope.Data.Scopes,
		CreatedAt:    envelope.Data.CreatedAt,
		ExpiresAt:    envelope.Data.ExpiresAt,
		RevokedAt:    envelope.Data.RevokedAt,
		LastUsedAt:   envelope.Data.LastUsedAt,
	})}, nil
}

type ModifyTokenInput struct {
	TokenID   string   `json:"-"`
	Name      *string  `json:"name,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresIn *int64   `json:"expires_in,omitempty"`
}

func (c *Client) ModifyToken(ctx context.Context, workspace string, input ModifyTokenInput) (*app.TokenView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	path := "/api/v1/tokens/" + url.PathEscape(input.TokenID)
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var envelope apiEnvelope[tokenDTO]
	if err := c.patch(ctx, path, input, &envelope); err != nil {
		return nil, err
	}
	view := tokenDTOToView(envelope.Data)
	return &view, nil
}

func (c *Client) RevokeToken(ctx context.Context, workspace, ref string) error {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	path := "/api/v1/tokens/" + url.PathEscape(ref)
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return c.delete(ctx, path, nil)
}

func DurationSecondsPtr(d *time.Duration) *int64 {
	if d == nil {
		return nil
	}
	value := int64(d.Seconds())
	return &value
}

func tokenDTOToView(row tokenDTO) app.TokenView {
	return app.TokenView{
		ID:           row.ID,
		Prefix:       row.Prefix,
		Name:         row.Name,
		Type:         row.Type,
		User:         task.UserInfoFromJSON(row.User),
		WorkspaceIDs: append([]string(nil), row.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), row.ProjectIDs...),
		Scopes:       append([]string(nil), row.Scopes...),
		CreatedAt:    row.CreatedAt,
		ExpiresAt:    row.ExpiresAt,
		RevokedAt:    row.RevokedAt,
		LastUsedAt:   row.LastUsedAt,
	}
}
