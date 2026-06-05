package remote

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskcontext"
	"git.dajee.net/dajee/xuanchu/internal/urgency"
)

type contextDTO struct {
	Name       string `json:"name"`
	Filter     string `json:"filter"`
	Active     bool   `json:"active"`
	CreatedAt  int64  `json:"created_at"`
	ModifiedAt int64  `json:"modified_at"`
}

type configValueInput struct {
	Value string `json:"value"`
}

func (c *Client) ListContexts(ctx context.Context, workspace string) ([]taskcontext.Context, string, error) {
	values := workspaceValues(workspace)
	var envelope apiEnvelope[[]contextDTO]
	if err := c.get(ctx, "/api/v1/contexts", values, &envelope); err != nil {
		return nil, "", err
	}
	rows := make([]taskcontext.Context, 0, len(envelope.Data))
	active := ""
	for _, row := range envelope.Data {
		rows = append(rows, taskcontext.Context{
			Name:         row.Name,
			FilterSource: row.Filter,
			CreatedAt:    row.CreatedAt,
			ModifiedAt:   row.ModifiedAt,
		})
		if row.Active {
			active = row.Name
		}
	}
	return rows, active, nil
}

func (c *Client) DefineContext(ctx context.Context, workspace, name, filter string) error {
	var envelope apiEnvelope[map[string]string]
	return c.post(ctx, pathWithWorkspace("/api/v1/contexts", workspace), map[string]string{"name": name, "filter": filter}, &envelope)
}

func (c *Client) UseContext(ctx context.Context, workspace, name string) error {
	return c.post(ctx, pathWithWorkspace("/api/v1/contexts/"+url.PathEscape(name)+"/use", workspace), map[string]any{}, nil)
}

func (c *Client) ClearContext(ctx context.Context, workspace string) error {
	return c.post(ctx, pathWithWorkspace("/api/v1/contexts/none", workspace), map[string]any{}, nil)
}

func (c *Client) DeleteContext(ctx context.Context, workspace, name string) error {
	return c.delete(ctx, pathWithWorkspace("/api/v1/contexts/"+url.PathEscape(name), workspace), nil)
}

func (c *Client) ListConfig(ctx context.Context, workspace string) (map[string]string, error) {
	var envelope apiEnvelope[map[string]string]
	if err := c.get(ctx, "/api/v1/config", workspaceValues(workspace), &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) GetConfig(ctx context.Context, workspace, key string) (string, error) {
	var envelope apiEnvelope[map[string]string]
	if err := c.get(ctx, pathWithWorkspace("/api/v1/config/"+url.PathEscape(key), workspace), nil, &envelope); err != nil {
		return "", err
	}
	return envelope.Data["value"], nil
}

func (c *Client) SetConfig(ctx context.Context, workspace, key, value string) error {
	var envelope apiEnvelope[map[string]string]
	return c.doJSON(ctx, "PUT", pathWithWorkspace("/api/v1/config/"+url.PathEscape(key), workspace), configValueInput{Value: value}, &envelope)
}

func (c *Client) UnsetConfig(ctx context.Context, workspace, key string) error {
	return c.delete(ctx, pathWithWorkspace("/api/v1/config/"+url.PathEscape(key), workspace), nil)
}

func (c *Client) ProjectConfigList(ctx context.Context, workspace, projectRef string) (map[string]string, error) {
	var envelope apiEnvelope[map[string]string]
	if err := c.get(ctx, pathWithWorkspace("/api/v1/projects/"+url.PathEscape(projectRef)+"/config", workspace), nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) ProjectConfigGet(ctx context.Context, workspace, projectRef, key string) (string, error) {
	var envelope apiEnvelope[map[string]string]
	if err := c.get(ctx, pathWithWorkspace("/api/v1/projects/"+url.PathEscape(projectRef)+"/config/"+url.PathEscape(key), workspace), nil, &envelope); err != nil {
		return "", err
	}
	return envelope.Data["value"], nil
}

func (c *Client) ProjectConfigSet(ctx context.Context, workspace, projectRef, key, value string) error {
	var envelope apiEnvelope[map[string]string]
	return c.doJSON(ctx, "PUT", pathWithWorkspace("/api/v1/projects/"+url.PathEscape(projectRef)+"/config/"+url.PathEscape(key), workspace), configValueInput{Value: value}, &envelope)
}

func (c *Client) ProjectConfigUnset(ctx context.Context, workspace, projectRef, key string) error {
	return c.delete(ctx, pathWithWorkspace("/api/v1/projects/"+url.PathEscape(projectRef)+"/config/"+url.PathEscape(key), workspace), nil)
}

func (c *Client) ExportTasks(ctx context.Context, workspace, project, projectID string) ([]task.Task, error) {
	values := workspaceValues(workspace)
	if projectID != "" {
		values.Set("project_id", projectID)
	} else if project != "" {
		values.Set("project", project)
	}
	var envelope apiEnvelope[[]task.JSONTask]
	if err := c.get(ctx, "/api/v1/export", values, &envelope); err != nil {
		return nil, err
	}
	return jsonTasksToTasks(envelope.Data)
}

func (c *Client) ImportTasks(ctx context.Context, workspace string, rows []task.JSONTask) (int, error) {
	var envelope apiEnvelope[map[string]int]
	if err := c.post(ctx, pathWithWorkspace("/api/v1/import", workspace), rows, &envelope); err != nil {
		return 0, err
	}
	return envelope.Data["imported"], nil
}

func (c *Client) ListAudit(ctx context.Context, workspace, project string, limit int) ([]app.AuditLogView, error) {
	values := workspaceValues(workspace)
	if project != "" {
		values.Set("project", project)
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	var envelope apiEnvelope[[]auditDTO]
	if err := c.get(ctx, "/api/v1/audit", values, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.AuditLogView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		view := app.AuditLogView{
			ID:               row.ID,
			WorkspaceID:      row.WorkspaceID,
			ProjectID:        row.ProjectID,
			Action:           row.Action,
			TargetType:       row.TargetType,
			TargetID:         row.TargetID,
			PayloadJSON:      string(row.Payload),
			DelegatorTokenID: row.DelegatorTokenID,
			CreatedAt:        row.CreatedAt,
		}
		if row.Actor != nil {
			var ui task.JSONUserInfo
			if json.Unmarshal(*row.Actor, &ui) == nil {
				info := task.UserInfoFromJSON(ui)
				view.Actor = &info
			}
		}
		if row.DelegatorUser != nil {
			var ui task.JSONUserInfo
			if json.Unmarshal(*row.DelegatorUser, &ui) == nil {
				info := task.UserInfoFromJSON(ui)
				view.DelegatorUser = &info
			}
		}
		out = append(out, view)
	}
	return out, nil
}

type auditDTO struct {
	ID               int64            `json:"id"`
	Actor            *json.RawMessage `json:"actor"`
	WorkspaceID      *string          `json:"workspace_id"`
	ProjectID        *string          `json:"project_id"`
	Action           string           `json:"action"`
	TargetType       string           `json:"target_type"`
	TargetID         string           `json:"target_id"`
	Payload          json.RawMessage  `json:"payload"`
	DelegatorTokenID *string          `json:"delegator_token_id,omitempty"`
	DelegatorUser    *json.RawMessage `json:"delegator_user,omitempty"`
	CreatedAt        int64            `json:"created_at"`
}

func (c *Client) ExplainUrgency(ctx context.Context, workspace, taskID string) (urgency.ExplainResult, error) {
	var envelope apiEnvelope[urgency.ExplainResult]
	if err := c.get(ctx, taskPathWithSuffix(workspace, taskID, "/urgency"), nil, &envelope); err != nil {
		return urgency.ExplainResult{}, err
	}
	return envelope.Data, nil
}

func workspaceValues(workspace string) url.Values {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	return values
}

func pathWithWorkspace(path, workspace string) string {
	if workspace == "" {
		return path
	}
	return path + "?workspace=" + url.QueryEscape(workspace)
}
