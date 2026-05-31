package remote

import (
	"context"
	"net/url"

	"github.com/dajee/taskg/internal/task"
)

type ListTasksInput struct {
	Workspace string
	Report    string
	Target    string
	Filters   []string
	Sort      string
}

type AddTaskInput struct {
	Description string   `json:"description"`
	Project     string   `json:"project,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func (c *Client) ListTasks(ctx context.Context, input ListTasksInput) ([]task.Task, error) {
	values := url.Values{}
	if input.Workspace != "" {
		values.Set("workspace", input.Workspace)
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
