package remote

import (
	"context"
	"net/url"

	"github.com/dajee/taskg/internal/app"
)

type userDTO struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Email              *string `json:"email,omitempty"`
	DefaultWorkspaceID *string `json:"default_workspace_id,omitempty"`
	Active             bool    `json:"active"`
	CreatedAt          int64   `json:"created_at"`
	ModifiedAt         int64   `json:"modified_at"`
}

type AddUserInput struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

func (c *Client) ListUsers(ctx context.Context) ([]app.UserView, error) {
	var envelope apiEnvelope[[]userDTO]
	if err := c.get(ctx, "/api/v1/users", nil, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.UserView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		out = append(out, userDTOToView(row))
	}
	return out, nil
}

func (c *Client) AddUser(ctx context.Context, input AddUserInput) (app.UserView, error) {
	var envelope apiEnvelope[userDTO]
	if err := c.post(ctx, "/api/v1/users", input, &envelope); err != nil {
		return app.UserView{}, err
	}
	return userDTOToView(envelope.Data), nil
}

func (c *Client) UserInfo(ctx context.Context, ref string) (app.UserView, error) {
	path := "/api/v1/users/" + url.PathEscape(ref)
	var envelope apiEnvelope[userDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return app.UserView{}, err
	}
	return userDTOToView(envelope.Data), nil
}

func userDTOToView(row userDTO) app.UserView {
	return app.UserView{
		ID:                 row.ID,
		Name:               row.Name,
		Email:              row.Email,
		DefaultWorkspaceID: row.DefaultWorkspaceID,
		Active:             row.Active,
		CreatedAt:          row.CreatedAt,
		ModifiedAt:         row.ModifiedAt,
	}
}
