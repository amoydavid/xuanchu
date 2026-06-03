package remote

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

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
	NoContext bool
	Limit     int
}

type AddTaskInput struct {
	Description string            `json:"description"`
	Project     string            `json:"project,omitempty"`
	ProjectID   string            `json:"project_id,omitempty"`
	Priority    string            `json:"priority,omitempty"`
	Due         *int64            `json:"due,omitempty"`
	Assignees   []string          `json:"assignees,omitempty"`
	Depends     []string          `json:"depends,omitempty"`
	Wait        *int64            `json:"wait,omitempty"`
	Scheduled   *int64            `json:"scheduled,omitempty"`
	Until       *int64            `json:"until,omitempty"`
	Recur       *string           `json:"recur,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	UDAs        map[string]string `json:"udas,omitempty"`
}

type ModifyTaskInput struct {
	Description     *string           `json:"description,omitempty"`
	Project         *string           `json:"project,omitempty"`
	ProjectID       *string           `json:"project_id,omitempty"`
	Priority        *string           `json:"priority,omitempty"`
	ClearProject    bool              `json:"clear_project,omitempty"`
	ClearPriority   bool              `json:"clear_priority,omitempty"`
	Due             *int64            `json:"due,omitempty"`
	ClearDue        bool              `json:"clear_due,omitempty"`
	Wait            *int64            `json:"wait,omitempty"`
	ClearWait       bool              `json:"clear_wait,omitempty"`
	Scheduled       *int64            `json:"scheduled,omitempty"`
	ClearScheduled  bool              `json:"clear_scheduled,omitempty"`
	Until           *int64            `json:"until,omitempty"`
	ClearUntil      bool              `json:"clear_until,omitempty"`
	Assignees       []string          `json:"assignees,omitempty"`
	RemoveAssignees []string          `json:"remove_assignees,omitempty"`
	ClearAssignees  bool              `json:"clear_assignees,omitempty"`
	Depends         []string          `json:"depends,omitempty"`
	ClearDepends    bool              `json:"clear_depends,omitempty"`
	Recur           *string           `json:"recur,omitempty"`
	ClearRecur      bool              `json:"clear_recur,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	RemoveTags      []string          `json:"remove_tags,omitempty"`
	UDAs            map[string]string `json:"udas,omitempty"`
	ClearUDAs       []string          `json:"clear_udas,omitempty"`
}

type TextInput struct {
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
}

type TaskLinkDTO struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at"`
	CreatedBy string `json:"created_by"`
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
	if input.NoContext {
		values.Set("no_context", "true")
	}
	if input.Limit > 0 {
		values.Set("limit", strconv.Itoa(input.Limit))
	}
	for _, filter := range input.Filters {
		values.Add("query", filter)
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

func (c *Client) AddTaskLink(ctx context.Context, workspace, taskUUID, linkType, linkURL, title string) (TaskLinkDTO, error) {
	path := taskPathWithSuffix(workspace, taskUUID, "/links")
	body := map[string]string{"type": linkType, "url": linkURL}
	if title != "" {
		body["title"] = title
	}
	var envelope apiEnvelope[TaskLinkDTO]
	if err := c.post(ctx, path, body, &envelope); err != nil {
		return TaskLinkDTO{}, err
	}
	return envelope.Data, nil
}

func (c *Client) RemoveTaskLink(ctx context.Context, workspace, taskUUID, linkID string) (task.Task, error) {
	path := taskPathWithSuffix(workspace, taskUUID, "/links/"+url.PathEscape(linkID))
	var envelope apiEnvelope[task.JSONTask]
	if err := c.delete(ctx, path, &envelope); err != nil {
		return task.Task{}, err
	}
	return task.FromJSONStrict(envelope.Data)
}

func (c *Client) ListTaskLinks(ctx context.Context, workspace, taskUUID string) ([]TaskLinkDTO, error) {
	path := taskPathWithSuffix(workspace, taskUUID, "/links")
	var envelope apiEnvelope[[]TaskLinkDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}
