package remote

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

// --- DTO（与 HTTP data 内层 JSON 同构）---

// TaskOccurrenceDTO 是 occurrence 的远程 DTO（spec §13.5）。
// id 始终存在；uuid/task_slug/project_seq 在 projected 时为 nil。
type TaskOccurrenceDTO struct {
	ID             string                   `json:"id"`
	UUID           *string                  `json:"uuid,omitempty"`
	TaskSlug       *string                  `json:"task_slug,omitempty"`
	ProjectSeq     *int64                   `json:"project_seq,omitempty"`
	ProjectID      *string                  `json:"project_id,omitempty"`
	Project        *string                  `json:"project,omitempty"`
	Title          string                   `json:"title"`
	Status         string                   `json:"status"`
	Entry          *int64                   `json:"entry,omitempty"`
	Modified       *int64                   `json:"modified,omitempty"`
	Due            *int64                   `json:"due,omitempty"`
	Priority       *string                  `json:"priority,omitempty"`
	Tags           []string                 `json:"tags,omitempty"`
	Assignees      []task.JSONUserInfo      `json:"assignees,omitempty"`
	RecurrenceInfo *RecurrenceInfoDTO       `json:"recurrence_info,omitempty"`
}

// RecurrenceInfoDTO 描述 occurrence 的循环归属。
type RecurrenceInfoDTO struct {
	Role            string   `json:"role"`
	SeriesID        string   `json:"series_id"`
	SeriesStatus    string   `json:"series_status"`
	Rule            string   `json:"rule"`
	RecurrenceAt    int64    `json:"recurrence_at"`
	Materialization string   `json:"materialization"`
	Overrides       []string `json:"overrides,omitempty"`
	Until           *int64   `json:"until,omitempty"`
}

// TaskViewPageDTO 是 TaskOccurrenceDTO 的分页结果。
type TaskViewPageDTO struct {
	Items          []TaskOccurrenceDTO `json:"items"`
	Total          int                 `json:"total"`
	Limit          int                 `json:"limit"`
	Offset         int                 `json:"offset"`
	OccurrenceMode string              `json:"occurrence_mode"`
}

// TaskSeriesDTO 是 series 的远程 DTO。
type TaskSeriesDTO struct {
	ID                  string              `json:"id"`
	WorkspaceID         string              `json:"workspace_id"`
	ProjectID           string              `json:"project_id"`
	Title               string              `json:"title"`
	Status              string              `json:"status"`
	RecurrenceRule      string              `json:"recurrence_rule"`
	FirstDue            int64               `json:"first_due"`
	Until               *int64              `json:"until,omitempty"`
	Priority            *string             `json:"priority,omitempty"`
	Tags                []string            `json:"tags,omitempty"`
	OpenOccurrenceCount int                 `json:"open_occurrence_count"`
	CompletedCount      int                 `json:"completed_count"`
	SkippedCount        int                 `json:"skipped_count"`
	OverdueCount        int                 `json:"overdue_count"`
	NextRecurrenceAt    *int64              `json:"next_recurrence_at,omitempty"`
	CreatedBy           task.JSONUserInfo   `json:"created_by"`
}

// TaskSeriesListPageDTO 是 series 列表分页。
type TaskSeriesListPageDTO struct {
	Items  []TaskSeriesDTO `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// TaskSeriesCreateResultDTO 是创建 series 的返回。
type TaskSeriesCreateResultDTO struct {
	Series          TaskSeriesDTO     `json:"series"`
	FirstOccurrence *TaskOccurrenceDTO `json:"first_occurrence,omitempty"`
}

// AddTaskSeriesInput 是创建 series 的输入。
type AddTaskSeriesInput struct {
	Title          string             `json:"title"`
	Description    *string            `json:"description,omitempty"`
	Project        *string            `json:"project,omitempty"`
	ProjectID      string             `json:"project_id,omitempty"`
	RecurrenceRule string             `json:"recurrence_rule"`
	FirstDue       *int64             `json:"first_due,omitempty"`
	FirstDueDate   *string            `json:"first_due_date,omitempty"`
	Until          *int64             `json:"until,omitempty"`
	UntilDate      *string            `json:"until_date,omitempty"`
	Priority       *string            `json:"priority,omitempty"`
	Assignees      []string           `json:"assignees,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
}

// TaskSeriesListInput 是 series 列表查询参数。
type TaskSeriesListInput struct {
	Workspace string
	Project   string
	ProjectID string
	Status    string
	Q         string
	Assignee  string
	Sort      string
	Limit     int
	Offset    int
}

// TaskQueryInput 是带 occurrence_mode/range 的任务查询。
type TaskQueryInput struct {
	Workspace      string
	Project        string
	DueAfter       string // YYYY-MM-DD
	DueBefore      string // YYYY-MM-DD
	OccurrenceMode string // auto|materialized|expand
	Status         string
	Sort           string
	Limit          int
	Offset         int
}

// --- Series 方法 ---

// AddTaskSeries 创建循环系列。
func (c *Client) AddTaskSeries(ctx context.Context, workspace string, input AddTaskSeriesInput) (TaskSeriesCreateResultDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	var resp struct {
		Data TaskSeriesCreateResultDTO `json:"data"`
	}
	if err := c.post(ctx, "/api/v1/task-series?"+values.Encode(), input, &resp); err != nil {
		return TaskSeriesCreateResultDTO{}, err
	}
	return resp.Data, nil
}

// ListTaskSeries 列出循环系列。
func (c *Client) ListTaskSeries(ctx context.Context, input TaskSeriesListInput) (TaskSeriesListPageDTO, error) {
	values := buildSeriesListValues(input)
	var resp struct {
		Data TaskSeriesListPageDTO `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/task-series", values, &resp); err != nil {
		return TaskSeriesListPageDTO{}, err
	}
	return resp.Data, nil
}

// GetTaskSeries 获取单个 series。
func (c *Client) GetTaskSeries(ctx context.Context, workspace, seriesRef string) (TaskSeriesDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	var resp struct {
		Data TaskSeriesDTO `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/task-series/"+url.PathEscape(seriesRef), values, &resp); err != nil {
		return TaskSeriesDTO{}, err
	}
	return resp.Data, nil
}

// StopTaskSeries 停止循环系列。
func (c *Client) StopTaskSeries(ctx context.Context, workspace, seriesRef string, deleteOpen bool) (TaskSeriesDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	values.Set("delete_open_occurrences", strconv.FormatBool(deleteOpen))
	var resp struct {
		Data TaskSeriesDTO `json:"data"`
	}
	if err := c.delete(ctx, "/api/v1/task-series/"+url.PathEscape(seriesRef)+"?"+values.Encode(), &resp); err != nil {
		return TaskSeriesDTO{}, err
	}
	return resp.Data, nil
}

// ListTaskSeriesOccurrences 列出 series 的 occurrence。
func (c *Client) ListTaskSeriesOccurrences(ctx context.Context, workspace, seriesRef string, status string, limit, offset int) (TaskViewPageDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if status != "" {
		values.Set("status", status)
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	var resp struct {
		Data TaskViewPageDTO `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/task-series/"+url.PathEscape(seriesRef)+"/occurrences", values, &resp); err != nil {
		return TaskViewPageDTO{}, err
	}
	return resp.Data, nil
}

// SkipTaskSeriesOccurrence 跳过一次 occurrence。
func (c *Client) SkipTaskSeriesOccurrence(ctx context.Context, workspace, seriesRef, occurrenceRef string) (TaskOccurrenceDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	path := "/api/v1/task-series/" + url.PathEscape(seriesRef) + "/occurrences/" + url.PathEscape(occurrenceRef) + "/skip?" + values.Encode()
	var resp struct {
		Data TaskOccurrenceDTO `json:"data"`
	}
	if err := c.post(ctx, path, nil, &resp); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return resp.Data, nil
}

// QueryTasks 按范围/occurrence_mode 查询任务，返回 TaskViewPage。
func (c *Client) QueryTasks(ctx context.Context, input TaskQueryInput) (TaskViewPageDTO, error) {
	values := url.Values{}
	if input.Workspace != "" {
		values.Set("workspace", input.Workspace)
	}
	if input.Project != "" {
		values.Set("project", input.Project)
	}
	if input.DueAfter != "" {
		values.Set("due_after", input.DueAfter)
	}
	if input.DueBefore != "" {
		values.Set("due_before", input.DueBefore)
	}
	if input.OccurrenceMode != "" {
		values.Set("occurrence_mode", input.OccurrenceMode)
	}
	if input.Status != "" {
		values.Set("status", input.Status)
	}
	if input.Sort != "" {
		values.Set("sort", input.Sort)
	}
	if input.Limit > 0 {
		values.Set("limit", strconv.Itoa(input.Limit))
	}
	if input.Offset > 0 {
		values.Set("offset", strconv.Itoa(input.Offset))
	}
	var resp struct {
		Data TaskViewPageDTO `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/tasks", values, &resp); err != nil {
		return TaskViewPageDTO{}, err
	}
	return resp.Data, nil
}

// GetTaskView 按 ref（UUID/slug/occurrence_ref）获取单个任务视图。
func (c *Client) GetTaskView(ctx context.Context, workspace, taskRef string) (TaskOccurrenceDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	var resp struct {
		Data TaskOccurrenceDTO `json:"data"`
	}
	// 注：当前 GET /tasks/{ref} 仍返回 task.JSONTask（旧 shape）。
	// 在 HTTP 层 fully 切到 TaskOccurrenceView 后此方法返回新 shape。
	// 这里先用 info 端点；occurrence_ref 的处理待 HTTP 层完善。
	if err := c.get(ctx, "/api/v1/tasks/"+url.PathEscape(taskRef), values, &resp); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return resp.Data, nil
}

func buildSeriesListValues(input TaskSeriesListInput) url.Values {
	values := url.Values{}
	if input.Workspace != "" {
		values.Set("workspace", input.Workspace)
	}
	if input.Project != "" {
		values.Set("project", input.Project)
	}
	if input.ProjectID != "" {
		values.Set("project_id", input.ProjectID)
	}
	if input.Status != "" {
		values.Set("status", input.Status)
	}
	if input.Q != "" {
		values.Set("q", input.Q)
	}
	if input.Assignee != "" {
		values.Set("assignee", input.Assignee)
	}
	if input.Sort != "" {
		values.Set("sort", input.Sort)
	}
	if input.Limit > 0 {
		values.Set("limit", strconv.Itoa(input.Limit))
	}
	if input.Offset > 0 {
		values.Set("offset", strconv.Itoa(input.Offset))
	}
	return values
}

// modifyTaskSeriesInput 是修改 series 的远程输入。
type modifyTaskSeriesInput struct {
	Title          *string          `json:"title,omitempty"`
	Description    *string          `json:"description,omitempty"`
	Priority       *string          `json:"priority,omitempty"`
	Assignees      []string         `json:"assignees,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
	RecurrenceRule *string          `json:"recurrence_rule,omitempty"`
	EffectiveFrom  *int64           `json:"effective_from,omitempty"`
	Until          *int64           `json:"until,omitempty"`
}

// ModifyTaskSeries 修改 series（spec §11.4）。
func (c *Client) ModifyTaskSeries(ctx context.Context, workspace, seriesRef string, input interface{}) (TaskSeriesDTO, error) {
	// input 是 app.ModifyTaskSeriesInput，这里用反射转 DTO。
	// 简化：直接 marshal/unmarshal 转换。
	body := modifyTaskSeriesInput{}
	if raw, err := json.Marshal(input); err == nil {
		_ = json.Unmarshal(raw, &body)
	}
	path := "/api/v1/task-series/" + url.PathEscape(seriesRef) + "?workspace=" + url.QueryEscape(workspace)
	var resp struct {
		Data TaskSeriesDTO `json:"data"`
	}
	if err := c.patch(ctx, path, body, &resp); err != nil {
		return TaskSeriesDTO{}, err
	}
	return resp.Data, nil
}
