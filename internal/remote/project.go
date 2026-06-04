package remote

import (
	"context"
	"net/url"
	"strconv"

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

type ProjectAnnotationDTO struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Entry     int64  `json:"entry"`
	Content   string `json:"content"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

type TimelineEntryDTO struct {
	SourceType  string `json:"source_type"`
	SourceID    string `json:"source_id"`
	SourceLabel string `json:"source_label"`
	Entry       int64  `json:"entry"`
	Content     string `json:"content"`
	CreatedBy   string `json:"created_by"`
}

func (c *Client) AnnotateProject(ctx context.Context, workspace, projectRef, content string) (ProjectAnnotationDTO, error) {
	path := projectPathWithSuffix(workspace, projectRef, "/annotations")
	var envelope apiEnvelope[ProjectAnnotationDTO]
	if err := c.post(ctx, path, map[string]string{"content": content}, &envelope); err != nil {
		return ProjectAnnotationDTO{}, err
	}
	return envelope.Data, nil
}

func (c *Client) DenotateProject(ctx context.Context, workspace, projectRef, annotationID string) error {
	path := projectPathWithSuffix(workspace, projectRef, "/annotations/"+url.PathEscape(annotationID))
	return c.delete(ctx, path, nil)
}

func (c *Client) ListProjectAnnotations(ctx context.Context, workspace, projectRef string) ([]ProjectAnnotationDTO, error) {
	path := projectPathWithSuffix(workspace, projectRef, "/annotations")
	var envelope apiEnvelope[[]ProjectAnnotationDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) ProjectTimeline(ctx context.Context, workspace, projectRef string, limit int) ([]TimelineEntryDTO, error) {
	suffix := "/timeline"
	if limit > 0 {
		suffix += "?limit=" + strconv.Itoa(limit)
		if workspace != "" {
			suffix += "&workspace=" + url.QueryEscape(workspace)
		}
		path := "/api/v1/projects/" + url.PathEscape(projectRef) + suffix
		var envelope apiEnvelope[[]TimelineEntryDTO]
		if err := c.get(ctx, path, nil, &envelope); err != nil {
			return nil, err
		}
		return envelope.Data, nil
	}
	path := projectPathWithSuffix(workspace, projectRef, "/timeline")
	var envelope apiEnvelope[[]TimelineEntryDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}
