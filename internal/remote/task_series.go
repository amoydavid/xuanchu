package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

// --- DTO（与 HTTP data 内层 JSON 同构）---

// TaskOccurrenceDTO 是 occurrence 的远程 DTO（spec §13.5）。
// id 始终存在；uuid/task_slug/project_seq 在 projected 时为 nil。
//
// DTO 与 HTTP TaskOccurrenceView 的 data 内层同构，所有时间字段使用 Unix 秒。
type TaskOccurrenceDTO struct {
	ID             string                `json:"id"`
	UUID           *string               `json:"uuid"`
	TaskSlug       *string               `json:"task_slug"`
	ProjectSeq     *int64                `json:"project_seq"`
	WorkspaceID    string                `json:"workspace_id,omitempty"`
	ProjectID      *string               `json:"project_id,omitempty"`
	Project        *string               `json:"project,omitempty"`
	Title          string                `json:"title"`
	Description    *string               `json:"description,omitempty"`
	Status         string                `json:"status"`
	Entry          *int64                `json:"entry"`    // projected 时为 null
	Modified       *int64                `json:"modified"` // projected 时为 null
	Start          *int64                `json:"start"`
	End            *int64                `json:"end"`
	Due            *int64                `json:"due,omitempty"` // 列表端点返回的 Unix 时间戳
	Wait           *int64                `json:"wait,omitempty"`
	Scheduled      *int64                `json:"scheduled,omitempty"`
	Until          *int64                `json:"until,omitempty"`
	Parent         *string               `json:"parent,omitempty"`
	Priority       *string               `json:"priority,omitempty"`
	Tags           []string              `json:"tags,omitempty"`
	Assignees      []task.JSONUserInfo   `json:"assignees,omitempty"`
	Depends        []string              `json:"depends,omitempty"`
	Annotations    []task.JSONAnnotation `json:"annotations,omitempty"`
	Links          []TaskLinkDTO         `json:"links,omitempty"`
	RecurrenceInfo *RecurrenceInfoDTO    `json:"recurrence_info,omitempty"`
	UDAs           map[string]string     `json:"udas,omitempty"`
}

// RecurrenceInfoDTO 描述 occurrence 的循环归属。
type RecurrenceInfoDTO struct {
	Role            string   `json:"role"`
	SeriesID        string   `json:"series_id"`
	SeriesTitle     string   `json:"series_title"`
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
	Range          *TaskViewRangeDTO   `json:"range,omitempty"`
}

type TaskViewRangeDTO struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// TaskSeriesDTO 是 series 的远程 DTO。
type TaskSeriesDTO struct {
	ID                         string              `json:"id"`
	WorkspaceID                string              `json:"workspace_id"`
	ProjectID                  string              `json:"project_id"`
	ProjectSlug                string              `json:"project_slug,omitempty"`
	SeriesSlug                 string              `json:"series_slug,omitempty"`
	Title                      string              `json:"title"`
	Description                *string             `json:"description,omitempty"`
	Status                     string              `json:"status"`
	RecurrenceRule             string              `json:"recurrence_rule"`
	FirstDue                   int64               `json:"first_due"`
	Until                      *int64              `json:"until,omitempty"`
	Priority                   *string             `json:"priority,omitempty"`
	Tags                       []string            `json:"tags,omitempty"`
	UDAs                       map[string]string   `json:"udas,omitempty"`
	OpenOccurrenceCount        int                 `json:"open_occurrence_count"`
	CompletedCount             int                 `json:"completed_count"`
	SkippedCount               int                 `json:"skipped_count"`
	OverdueCount               int                 `json:"overdue_count"`
	NextRecurrenceAt           *int64              `json:"next_recurrence_at,omitempty"`
	SuggestedRuleEffectiveFrom *int64              `json:"suggested_rule_effective_from,omitempty"`
	CreatedBy                  task.JSONUserInfo   `json:"created_by"`
	Assignees                  []task.JSONUserInfo `json:"assignees,omitempty"`
	CreatedAt                  int64               `json:"created_at"`
	ModifiedAt                 int64               `json:"modified_at"`
	OpenOccurrences            []TaskOccurrenceDTO `json:"open_occurrences,omitempty"`
	RecentCompleted            []TaskOccurrenceDTO `json:"recent_completed,omitempty"`
	RecentSkipped              []TaskOccurrenceDTO `json:"recent_skipped,omitempty"`
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
	Series          TaskSeriesDTO      `json:"series"`
	FirstOccurrence *TaskOccurrenceDTO `json:"first_occurrence,omitempty"`
}

// AddTaskSeriesInput 是创建 series 的输入。
type AddTaskSeriesInput struct {
	Title          string            `json:"title"`
	Description    *string           `json:"description,omitempty"`
	Project        *string           `json:"project,omitempty"`
	ProjectID      string            `json:"project_id,omitempty"`
	RecurrenceRule string            `json:"recurrence_rule"`
	FirstDue       *int64            `json:"first_due,omitempty"`
	FirstDueDate   *string           `json:"first_due_date,omitempty"`
	Until          *int64            `json:"until,omitempty"`
	UntilDate      *string           `json:"until_date,omitempty"`
	Priority       *string           `json:"priority,omitempty"`
	Assignees      []string          `json:"assignees,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	UDAs           map[string]string `json:"udas,omitempty"`
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

// TaskSeriesOccurrenceListInput 是 series occurrence 分页与范围查询。
type TaskSeriesOccurrenceListInput struct {
	Workspace string
	SeriesRef string
	Status    string
	DueAfter  string // YYYY-MM-DD, inclusive
	DueBefore string // YYYY-MM-DD, inclusive
	Limit     int
	Offset    int
}

// TaskQueryInput 是带 occurrence_mode/range 的任务查询。
type TaskQueryInput struct {
	Workspace      string
	Project        string
	ProjectID      string
	DueAfter       string // YYYY-MM-DD
	DueBefore      string // YYYY-MM-DD
	OccurrenceMode string // auto|materialized|expand
	Status         string
	Sort           string
	Limit          int
	Offset         int
	NoContext      bool
	Filters        []string // query/filter 表达式
	Target         string   // 解析为 UUID 的 target ref
	Report         string   // report name（/tasks?report=）
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
	if err := validateTaskSeriesPagination(input.Limit, input.Offset); err != nil {
		return TaskSeriesListPageDTO{}, err
	}
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
func (c *Client) ListTaskSeriesOccurrences(ctx context.Context, input TaskSeriesOccurrenceListInput) (TaskViewPageDTO, error) {
	if err := validateTaskSeriesPagination(input.Limit, input.Offset); err != nil {
		return TaskViewPageDTO{}, err
	}
	values := url.Values{}
	if input.Workspace != "" {
		values.Set("workspace", input.Workspace)
	}
	if input.Status != "" {
		values.Set("status", input.Status)
	}
	if input.DueAfter != "" {
		values.Set("due_after", input.DueAfter)
	}
	if input.DueBefore != "" {
		values.Set("due_before", input.DueBefore)
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
	if err := c.get(ctx, "/api/v1/task-series/"+url.PathEscape(input.SeriesRef)+"/occurrences", values, &resp); err != nil {
		return TaskViewPageDTO{}, err
	}
	return resp.Data, nil
}

func validateTaskSeriesPagination(limit, offset int) error {
	if limit < 0 || limit > 1000 {
		return fmt.Errorf("limit must be between 1 and 1000 when provided")
	}
	if offset < 0 {
		return fmt.Errorf("offset must be >= 0")
	}
	return nil
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
	if input.ProjectID != "" {
		values.Set("project_id", input.ProjectID)
	} else if input.Project != "" {
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
	if input.NoContext {
		values.Set("no_context", "true")
	}
	if input.Target != "" {
		values.Set("target", input.Target)
	}
	if input.Report != "" {
		values.Set("report", input.Report)
	}
	for _, f := range input.Filters {
		values.Add("query", f)
	}
	var resp struct {
		Data TaskViewPageDTO `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/tasks", values, &resp); err != nil {
		return TaskViewPageDTO{}, err
	}
	return resp.Data, nil
}

// GetTaskView 按 ref（UUID/slug/occurrence_ref）获取统一任务视图（spec §13.5）。
func (c *Client) GetTaskView(ctx context.Context, workspace, taskRef string) (TaskOccurrenceDTO, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/tasks/"+url.PathEscape(taskRef), values, &envelope); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	dto, err := parseTaskOccurrenceDTO(envelope.Data)
	if err != nil {
		return TaskOccurrenceDTO{}, err
	}
	return dto, nil
}

// parseTaskOccurrenceDTO 只接受统一 TaskOccurrenceView，不再猜测旧 JSONTask 时间形态。
func parseTaskOccurrenceDTO(raw json.RawMessage) (TaskOccurrenceDTO, error) {
	var dto TaskOccurrenceDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		return TaskOccurrenceDTO{}, err
	}
	if strings.TrimSpace(dto.ID) == "" {
		return TaskOccurrenceDTO{}, fmt.Errorf("task occurrence response is missing id")
	}
	if dto.TaskSlug != nil {
		project, seq, err := parseTaskSlug(*dto.TaskSlug)
		if err != nil {
			return TaskOccurrenceDTO{}, err
		}
		if dto.Project == nil || *dto.Project != project {
			return TaskOccurrenceDTO{}, fmt.Errorf("task_slug %q does not match project", *dto.TaskSlug)
		}
		if dto.ProjectSeq != nil && *dto.ProjectSeq != seq {
			return TaskOccurrenceDTO{}, fmt.Errorf("task_slug %q does not match project_seq", *dto.TaskSlug)
		}
		dto.ProjectSeq = &seq
	}
	return dto, nil
}

func parseTaskSlug(value string) (string, int64, error) {
	value = strings.TrimSpace(value)
	dash := strings.LastIndex(value, "-")
	if dash <= 0 || dash == len(value)-1 {
		return "", 0, fmt.Errorf("invalid task_slug %q", value)
	}
	seq, err := strconv.ParseInt(value[dash+1:], 10, 64)
	if err != nil || seq < 1 {
		return "", 0, fmt.Errorf("invalid task_slug %q", value)
	}
	return value[:dash], seq, nil
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

// ModifyTaskSeriesInput 是与 HTTP PATCH /task-series/{id} 同构的远程输入。
type ModifyTaskSeriesInput struct {
	Title             *string           `json:"title,omitempty"`
	Description       *string           `json:"description,omitempty"`
	Priority          *string           `json:"priority,omitempty"`
	Assignees         []string          `json:"assignees,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	UDAs              map[string]string `json:"udas,omitempty"`
	RecurrenceRule    *string           `json:"recurrence_rule,omitempty"`
	EffectiveFrom     *int64            `json:"effective_from,omitempty"`
	EffectiveFromDate *string           `json:"effective_from_date,omitempty"`
	Until             *int64            `json:"until,omitempty"`
	UntilDate         *string           `json:"until_date,omitempty"`
	Clear             []string          `json:"clear,omitempty"`
}

// ModifyTaskSeries 修改 series（spec §11.4）。
func (c *Client) ModifyTaskSeries(ctx context.Context, workspace, seriesRef string, input ModifyTaskSeriesInput) (TaskSeriesDTO, error) {
	path := "/api/v1/task-series/" + url.PathEscape(seriesRef) + "?workspace=" + url.QueryEscape(workspace)
	var resp struct {
		Data TaskSeriesDTO `json:"data"`
	}
	if err := c.patch(ctx, path, input, &resp); err != nil {
		return TaskSeriesDTO{}, err
	}
	return resp.Data, nil
}
