package remote

import (
	"context"
	"net/url"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type userDTO struct {
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	DisplayName        string                `json:"display_name"`
	Email              *string               `json:"email,omitempty"`
	DefaultWorkspaceID *string               `json:"default_workspace_id,omitempty"`
	ExternalIDs        []task.JSONExternalID `json:"external_ids,omitempty"`
	Active             bool                  `json:"active"`
	CreatedAt          int64                 `json:"created_at"`
	ModifiedAt         int64                 `json:"modified_at"`
}

type AddUserInput struct {
	Name        string  `json:"name"`
	DisplayName *string `json:"display_name,omitempty"`
	Email       *string `json:"email,omitempty"`
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

func (c *Client) BindExternalID(ctx context.Context, userRef, provider, userType, externalID string) error {
	path := "/api/v1/users/" + url.PathEscape(userRef) + "/external-ids"
	body := map[string]string{"provider": provider, "external_id": externalID}
	if userType != "" {
		body["user_type"] = userType
	}
	var envelope apiEnvelope[any]
	return c.post(ctx, path, body, &envelope)
}

func (c *Client) UnbindExternalID(ctx context.Context, userRef, provider, externalID string) error {
	path := "/api/v1/users/" + url.PathEscape(userRef) + "/external-ids/" + url.PathEscape(provider) + "/" + url.PathEscape(externalID)
	var envelope apiEnvelope[any]
	return c.delete(ctx, path, &envelope)
}

func userDTOToView(row userDTO) app.UserView {
	extIDs := make([]task.ExternalIDInfo, 0, len(row.ExternalIDs))
	for _, eid := range row.ExternalIDs {
		extIDs = append(extIDs, task.ExternalIDInfo{Provider: eid.Provider, UserType: eid.UserType, ExternalID: eid.ExternalID})
	}
	return app.UserView{
		ID:                 row.ID,
		Name:               row.Name,
		DisplayName:        row.DisplayName,
		Email:              row.Email,
		DefaultWorkspaceID: row.DefaultWorkspaceID,
		ExternalIDs:        extIDs,
		Active:             row.Active,
		CreatedAt:          row.CreatedAt,
		ModifiedAt:         row.ModifiedAt,
	}
}
