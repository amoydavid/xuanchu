package remote

import (
	"context"
	"net/url"

	"github.com/dajee/taskg/internal/app"
)

type projectDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	TaskCount   int    `json:"task_count"`
	CreatedAt   int64  `json:"created_at"`
	ModifiedAt  int64  `json:"modified_at"`
	ArchivedAt  *int64 `json:"archived_at"`
}

type AddProjectInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ModifyProjectInput struct {
	Slug        *string `json:"slug,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (c *Client) ListProjects(ctx context.Context, workspace string, includeArchived bool) ([]app.ProjectView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if includeArchived {
		values.Set("all", "true")
	}
	var envelope apiEnvelope[[]projectDTO]
	if err := c.get(ctx, "/api/v1/projects", values, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.ProjectView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		out = append(out, projectDTOToView(row))
	}
	return out, nil
}

func (c *Client) AddProject(ctx context.Context, workspace string, input AddProjectInput) (app.ProjectView, error) {
	path := "/api/v1/projects"
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	var envelope apiEnvelope[projectDTO]
	if err := c.post(ctx, path, input, &envelope); err != nil {
		return app.ProjectView{}, err
	}
	return projectDTOToView(envelope.Data), nil
}

func (c *Client) GetProject(ctx context.Context, workspace, ref string) (app.ProjectView, error) {
	path := "/api/v1/projects/" + url.PathEscape(ref)
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	var envelope apiEnvelope[projectDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return app.ProjectView{}, err
	}
	return projectDTOToView(envelope.Data), nil
}

func (c *Client) ModifyProject(ctx context.Context, workspace, ref string, input ModifyProjectInput) (app.ProjectView, error) {
	path := projectPath(workspace, ref)
	var envelope apiEnvelope[projectDTO]
	if err := c.patch(ctx, path, input, &envelope); err != nil {
		return app.ProjectView{}, err
	}
	return projectDTOToView(envelope.Data), nil
}

func (c *Client) ArchiveProject(ctx context.Context, workspace, ref string) (app.ProjectView, error) {
	path := projectPathWithSuffix(workspace, ref, "/archive")
	var envelope apiEnvelope[projectDTO]
	if err := c.post(ctx, path, map[string]any{}, &envelope); err != nil {
		return app.ProjectView{}, err
	}
	return projectDTOToView(envelope.Data), nil
}

func projectPath(workspace, ref string) string {
	return projectPathWithSuffix(workspace, ref, "")
}

func projectPathWithSuffix(workspace, ref, suffix string) string {
	path := "/api/v1/projects/" + url.PathEscape(ref) + suffix
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	return path
}

func projectDTOToView(row projectDTO) app.ProjectView {
	return app.ProjectView{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Status:      row.Status,
		TaskCount:   row.TaskCount,
		CreatedAt:   row.CreatedAt,
		ModifiedAt:  row.ModifiedAt,
		ArchivedAt:  row.ArchivedAt,
	}
}
