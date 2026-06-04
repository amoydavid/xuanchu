package remote

import (
	"context"
	"net/url"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/task"
)

type workspaceDTO struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Visibility  string             `json:"visibility"`
	CreatedBy   *task.JSONUserInfo `json:"created_by,omitempty"`
	ArchivedAt  *int64             `json:"archived_at,omitempty"`
	Role        string             `json:"role"`
	Active      bool               `json:"active"`
	CreatedAt   int64              `json:"created_at"`
	ModifiedAt  int64              `json:"modified_at"`
}

type AddWorkspaceInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

type ModifyWorkspaceInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Visibility  *string `json:"visibility,omitempty"`
}

func (c *Client) ListWorkspaces(ctx context.Context, includeArchived bool) ([]app.WorkspaceView, error) {
	values := url.Values{}
	if includeArchived {
		values.Set("all", "true")
	}
	var envelope apiEnvelope[[]workspaceDTO]
	if err := c.get(ctx, "/api/v1/workspaces", values, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.WorkspaceView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		out = append(out, workspaceDTOToView(row))
	}
	return out, nil
}

func (c *Client) AddWorkspace(ctx context.Context, input AddWorkspaceInput) (app.WorkspaceView, error) {
	var envelope apiEnvelope[workspaceDTO]
	if err := c.post(ctx, "/api/v1/workspaces", input, &envelope); err != nil {
		return app.WorkspaceView{}, err
	}
	return workspaceDTOToView(envelope.Data), nil
}

func (c *Client) WorkspaceInfo(ctx context.Context, ref string) (app.WorkspaceView, error) {
	path := "/api/v1/workspaces/" + url.PathEscape(ref)
	var envelope apiEnvelope[workspaceDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return app.WorkspaceView{}, err
	}
	return workspaceDTOToView(envelope.Data), nil
}

func (c *Client) ModifyWorkspace(ctx context.Context, ref string, input ModifyWorkspaceInput) error {
	path := "/api/v1/workspaces/" + url.PathEscape(ref)
	var envelope apiEnvelope[workspaceDTO]
	return c.patch(ctx, path, input, &envelope)
}

func (c *Client) UseWorkspace(ctx context.Context, workspace string) (app.WorkspaceView, error) {
	var envelope apiEnvelope[workspaceDTO]
	if err := c.doJSON(ctx, "PUT", "/api/v1/me/active_workspace", map[string]string{"workspace": workspace}, &envelope); err != nil {
		return app.WorkspaceView{}, err
	}
	return workspaceDTOToView(envelope.Data), nil
}

func (c *Client) ArchiveWorkspace(ctx context.Context, ref string) error {
	path := "/api/v1/workspaces/" + url.PathEscape(ref) + "/archive"
	return c.post(ctx, path, map[string]any{}, nil)
}

func workspaceDTOToView(row workspaceDTO) app.WorkspaceView {
	var createdBy *task.UserInfo
	if row.CreatedBy != nil {
		ui := task.UserInfoFromJSON(*row.CreatedBy)
		createdBy = &ui
	}
	return app.WorkspaceView{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Visibility:  row.Visibility,
		CreatedBy:   createdBy,
		ArchivedAt:  row.ArchivedAt,
		Role:        app.Role(row.Role),
		Active:      row.Active,
		CreatedAt:   row.CreatedAt,
		ModifiedAt:  row.ModifiedAt,
	}
}
