package remote

import (
	"context"
	"encoding/json"
	"net/url"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

type AddTaskInput struct {
	Title         string            `json:"title"`
	Description   *string           `json:"description,omitempty"`
	Project       string            `json:"project,omitempty"`
	ProjectID     string            `json:"project_id,omitempty"`
	Priority      string            `json:"priority,omitempty"`
	Due           *int64            `json:"due,omitempty"`
	DueDate       string            `json:"due_date,omitempty"`
	Assignees     []string          `json:"assignees,omitempty"`
	Depends       []string          `json:"depends,omitempty"`
	Wait          *int64            `json:"wait,omitempty"`
	WaitDate      string            `json:"wait_date,omitempty"`
	Scheduled     *int64            `json:"scheduled,omitempty"`
	ScheduledDate string            `json:"scheduled_date,omitempty"`
	Until         *int64            `json:"until,omitempty"`
	UntilDate     string            `json:"until_date,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	UDAs          map[string]string `json:"udas,omitempty"`
}

type ModifyTaskInput struct {
	Title            *string           `json:"title,omitempty"`
	Description      *string           `json:"description,omitempty"`
	ClearDescription bool              `json:"clear_description,omitempty"`
	Project          *string           `json:"project,omitempty"`
	ProjectID        *string           `json:"project_id,omitempty"`
	Priority         *string           `json:"priority,omitempty"`
	ClearProject     bool              `json:"clear_project,omitempty"`
	ClearPriority    bool              `json:"clear_priority,omitempty"`
	Due              *int64            `json:"due,omitempty"`
	DueDate          string            `json:"due_date,omitempty"`
	ClearDue         bool              `json:"clear_due,omitempty"`
	Wait             *int64            `json:"wait,omitempty"`
	WaitDate         string            `json:"wait_date,omitempty"`
	ClearWait        bool              `json:"clear_wait,omitempty"`
	Scheduled        *int64            `json:"scheduled,omitempty"`
	ScheduledDate    string            `json:"scheduled_date,omitempty"`
	ClearScheduled   bool              `json:"clear_scheduled,omitempty"`
	Until            *int64            `json:"until,omitempty"`
	UntilDate        string            `json:"until_date,omitempty"`
	ClearUntil       bool              `json:"clear_until,omitempty"`
	Assignees        []string          `json:"assignees,omitempty"`
	RemoveAssignees  []string          `json:"remove_assignees,omitempty"`
	ClearAssignees   bool              `json:"clear_assignees,omitempty"`
	Depends          []string          `json:"depends,omitempty"`
	ClearDepends     bool              `json:"clear_depends,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	RemoveTags       []string          `json:"remove_tags,omitempty"`
	UDAs             map[string]string `json:"udas,omitempty"`
	ClearUDAs        []string          `json:"clear_udas,omitempty"`
}

type TextInput struct {
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
}

type TaskLinkDTO struct {
	ID        string             `json:"id"`
	Type      string             `json:"type"`
	URL       string             `json:"url"`
	Title     string             `json:"title,omitempty"`
	CreatedAt string             `json:"created_at"`
	CreatedBy task.JSONActorInfo `json:"created_by"`
}

type taskOccurrenceEnvelope struct {
	Data json.RawMessage `json:"data"`
}

func (e taskOccurrenceEnvelope) taskView() (TaskOccurrenceDTO, error) {
	return parseTaskOccurrenceDTO(e.Data)
}

func (c *Client) AddTask(ctx context.Context, workspace string, input AddTaskInput) (TaskOccurrenceDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	var envelope taskOccurrenceEnvelope
	if err := c.post(ctx, "/api/v1/tasks?"+values.Encode(), input, &envelope); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return envelope.taskView()
}

func (c *Client) ModifyTask(ctx context.Context, workspace, taskID string, input ModifyTaskInput) (TaskOccurrenceDTO, error) {
	path := taskPath(workspace, taskID)
	var envelope taskOccurrenceEnvelope
	if err := c.patch(ctx, path, input, &envelope); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return envelope.taskView()
}

func (c *Client) DoneTask(ctx context.Context, workspace, taskID string) (TaskOccurrenceDTO, error) {
	return c.postTaskAction(ctx, workspace, taskID, "done", nil)
}

func (c *Client) DeleteTask(ctx context.Context, workspace, taskID string) (TaskOccurrenceDTO, error) {
	path := taskPath(workspace, taskID)
	var envelope taskOccurrenceEnvelope
	if err := c.delete(ctx, path, &envelope); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return envelope.taskView()
}

func (c *Client) StartTask(ctx context.Context, workspace, taskID string) (TaskOccurrenceDTO, error) {
	return c.postTaskAction(ctx, workspace, taskID, "start", nil)
}

func (c *Client) StopTask(ctx context.Context, workspace, taskID string) (TaskOccurrenceDTO, error) {
	return c.postTaskAction(ctx, workspace, taskID, "stop", nil)
}

func (c *Client) ReopenTask(ctx context.Context, workspace, taskID string) (TaskOccurrenceDTO, error) {
	return c.postTaskAction(ctx, workspace, taskID, "reopen", nil)
}

func (c *Client) AnnotateTask(ctx context.Context, workspace, taskID, description string) (TaskOccurrenceDTO, error) {
	return c.postTaskAction(ctx, workspace, taskID, "annotations", TextInput{Description: description})
}

func (c *Client) DenotateTask(ctx context.Context, workspace, taskID string, annotationID string) (TaskOccurrenceDTO, error) {
	path := taskPathWithSuffix(workspace, taskID, "/annotations/"+url.PathEscape(annotationID))
	var envelope taskOccurrenceEnvelope
	if err := c.delete(ctx, path, &envelope); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return envelope.taskView()
}

func (c *Client) postTaskAction(ctx context.Context, workspace, taskID, action string, body any) (TaskOccurrenceDTO, error) {
	if body == nil {
		body = map[string]any{}
	}
	path := taskPathWithSuffix(workspace, taskID, "/"+url.PathEscape(action))
	var envelope taskOccurrenceEnvelope
	if err := c.post(ctx, path, body, &envelope); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return envelope.taskView()
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

func (c *Client) RemoveTaskLink(ctx context.Context, workspace, taskUUID, linkID string) (TaskOccurrenceDTO, error) {
	path := taskPathWithSuffix(workspace, taskUUID, "/links/"+url.PathEscape(linkID))
	var envelope taskOccurrenceEnvelope
	if err := c.delete(ctx, path, &envelope); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return envelope.taskView()
}

func (c *Client) ListTaskLinks(ctx context.Context, workspace, taskUUID string) ([]TaskLinkDTO, error) {
	path := taskPathWithSuffix(workspace, taskUUID, "/links")
	var envelope apiEnvelope[[]TaskLinkDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}
