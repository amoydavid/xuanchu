package remote

import (
	"context"
	"fmt"
	"net/url"

	"github.com/dajee/taskg/internal/task"
)

type ListTasksInput struct {
	Workspace string
	Project   string
	ProjectID string
	Report    string
	Target    string
	Filters   []string
	Sort      string
}

type AddTaskInput struct {
	Description string            `json:"description"`
	Project     string            `json:"project,omitempty"`
	ProjectID   string            `json:"project_id,omitempty"`
	Priority    string            `json:"priority,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	UDAs        map[string]string `json:"udas,omitempty"`
}

type ModifyTaskInput struct {
	Description  *string           `json:"description,omitempty"`
	Project      *string           `json:"project,omitempty"`
	ProjectID    *string           `json:"project_id,omitempty"`
	Priority     *string           `json:"priority,omitempty"`
	ClearProject bool              `json:"clear_project,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	UDAs         map[string]string `json:"udas,omitempty"`
	ClearUDAs    []string          `json:"clear_udas,omitempty"`
}

type TextInput struct {
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
}

func (c *Client) ListTasks(ctx context.Context, input ListTasksInput) ([]task.Task, error) {
	values := url.Values{}
	if input.Workspace != "" {
		values.Set("workspace", input.Workspace)
	}
	if input.ProjectID != "" {
		values.Set("project_id", input.ProjectID)
	} else if input.Project != "" {
		values.Set("project", input.Project)
	}
	if input.Report != "" {
		values.Set("report", input.Report)
	}
	if input.Target != "" {
		values.Set("target", input.Target)
	}
	if input.Sort != "" {
		values.Set("sort", input.Sort)
	}
	for _, filter := range input.Filters {
		values.Add("filter", filter)
	}
	var envelope apiEnvelope[[]task.JSONTask]
	if err := c.get(ctx, "/api/v1/tasks", values, &envelope); err != nil {
		return nil, err
	}
	return jsonTasksToTasks(envelope.Data)
}

func (c *Client) AddTask(ctx context.Context, workspace string, input AddTaskInput) (task.Task, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	var envelope apiEnvelope[task.JSONTask]
	if err := c.post(ctx, "/api/v1/tasks?"+values.Encode(), input, &envelope); err != nil {
		return task.Task{}, err
	}
	return task.FromJSONStrict(envelope.Data)
}

func (c *Client) GetTask(ctx context.Context, workspace, taskID string) (task.Task, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	path := "/api/v1/tasks/" + url.PathEscape(taskID)
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var envelope apiEnvelope[task.JSONTask]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return task.Task{}, err
	}
	return task.FromJSONStrict(envelope.Data)
}

func (c *Client) ModifyTask(ctx context.Context, workspace, taskID string, input ModifyTaskInput) (task.Task, error) {
	path := taskPath(workspace, taskID)
	var envelope apiEnvelope[task.JSONTask]
	if err := c.patch(ctx, path, input, &envelope); err != nil {
		return task.Task{}, err
	}
	return task.FromJSONStrict(envelope.Data)
}

func (c *Client) DoneTask(ctx context.Context, workspace, taskID string) (task.Task, error) {
	return c.postTaskAction(ctx, workspace, taskID, "done", nil)
}

func (c *Client) DeleteTask(ctx context.Context, workspace, taskID string) (task.Task, error) {
	path := taskPath(workspace, taskID)
	var envelope apiEnvelope[task.JSONTask]
	if err := c.delete(ctx, path, &envelope); err != nil {
		return task.Task{}, err
	}
	return task.FromJSONStrict(envelope.Data)
}

func (c *Client) StartTask(ctx context.Context, workspace, taskID string) (task.Task, error) {
	return c.postTaskAction(ctx, workspace, taskID, "start", nil)
}

func (c *Client) StopTask(ctx context.Context, workspace, taskID string) (task.Task, error) {
	return c.postTaskAction(ctx, workspace, taskID, "stop", nil)
}

func (c *Client) AnnotateTask(ctx context.Context, workspace, taskID, description string) (task.Task, error) {
	return c.postTaskAction(ctx, workspace, taskID, "annotations", TextInput{Description: description})
}

func (c *Client) DenotateTask(ctx context.Context, workspace, taskID string, index int) (task.Task, error) {
	path := taskPathWithSuffix(workspace, taskID, "/annotations/"+url.PathEscape(fmt.Sprintf("%d", index)))
	var envelope apiEnvelope[task.JSONTask]
	if err := c.delete(ctx, path, &envelope); err != nil {
		return task.Task{}, err
	}
	return task.FromJSONStrict(envelope.Data)
}

func (c *Client) postTaskAction(ctx context.Context, workspace, taskID, action string, body any) (task.Task, error) {
	if body == nil {
		body = map[string]any{}
	}
	path := taskPathWithSuffix(workspace, taskID, "/"+url.PathEscape(action))
	var envelope apiEnvelope[task.JSONTask]
	if err := c.post(ctx, path, body, &envelope); err != nil {
		return task.Task{}, err
	}
	return task.FromJSONStrict(envelope.Data)
}

func taskPath(workspace, taskID string) string {
	return taskPathWithSuffix(workspace, taskID, "")
}

func taskPathWithSuffix(workspace, taskID, suffix string) string {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	path := "/api/v1/tasks/" + url.PathEscape(taskID) + suffix
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path
}

func jsonTasksToTasks(rows []task.JSONTask) ([]task.Task, error) {
	out := make([]task.Task, 0, len(rows))
	for _, row := range rows {
		tsk, err := task.FromJSONStrict(row)
		if err != nil {
			return nil, err
		}
		out = append(out, tsk)
	}
	return out, nil
}
