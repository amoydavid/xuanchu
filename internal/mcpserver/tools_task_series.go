package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// --- 输入类型 ---

type TaskSeriesAddInput struct {
	Workspace      string            `json:"workspace,omitempty"`
	Project        string            `json:"project,omitempty"`
	ProjectID      string            `json:"project_id,omitempty"`
	Title          string            `json:"title"`
	Description    *string           `json:"description,omitempty"`
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

func (in TaskSeriesAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskSeriesListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Q         string `json:"q,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
	Sort      string `json:"sort,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

func (in TaskSeriesListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskSeriesRefInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id"`
}

func (in TaskSeriesRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskSeriesStopInput struct {
	Workspace             string `json:"workspace,omitempty"`
	ID                    string `json:"id"`
	DeleteOpenOccurrences bool   `json:"delete_open_occurrences,omitempty"`
}

func (in TaskSeriesStopInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

type TaskSeriesOccurrenceListInput struct {
	Workspace string `json:"workspace,omitempty"`
	ID        string `json:"id"`
	Status    string `json:"status,omitempty"`
	DueAfter  string `json:"due_after,omitempty" jsonschema:"inclusive YYYY-MM-DD"`
	DueBefore string `json:"due_before,omitempty" jsonschema:"inclusive YYYY-MM-DD"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

func (in TaskSeriesOccurrenceListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

type TaskSeriesOccurrenceSkipInput struct {
	Workspace    string `json:"workspace,omitempty"`
	SeriesID     string `json:"series_id"`
	OccurrenceID string `json:"occurrence_id"`
}

func (in TaskSeriesOccurrenceSkipInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

// --- 注册 ---

func registerTaskSeriesTools(s *mcp.Server, opts Options) {
	registerTaskSeriesAdd(s, opts)
	registerTaskSeriesList(s, opts)
	registerTaskSeriesGet(s, opts)
	registerTaskSeriesModify(s, opts)
	registerTaskSeriesStop(s, opts)
	registerTaskSeriesListOccurrences(s, opts)
	registerTaskSeriesOccurrenceSkip(s, opts)
}

func registerTaskSeriesAdd(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name: "task_series_add", Description: "Create a recurring task series; each calendar slot is an independent task.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskSeriesAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		firstDue, err := resolveSeriesDueField(in.FirstDue, in.FirstDueDate)
		if err != nil {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "task_series_due_required", Message: err.Error()})
		}
		until, err := resolveSeriesDueField(in.Until, in.UntilDate)
		if err != nil {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "task_series_invalid_until", Message: err.Error()})
		}
		input := app.AddTaskSeriesInput{
			Title: in.Title, Description: in.Description,
			RecurrenceRule: in.RecurrenceRule, Priority: in.Priority,
			Assignees: in.Assignees, Tags: in.Tags, UDAs: in.UDAs,
		}
		if firstDue != nil {
			input.FirstDue = *firstDue
		}
		if in.Project != "" {
			p := in.Project
			input.Project = &p
		}
		if in.ProjectID != "" {
			input.ProjectID = in.ProjectID
		}
		if until != nil {
			input.Until = until
		}
		result, err := svc.AddTaskSeries(input)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rendered := fmt.Sprintf("循环任务已创建：%s（%s）\n状态：%s", result.Series.Title, result.Series.RecurrenceRule, result.Series.Status)
		if result.FirstOccurrence != nil && result.FirstOccurrence.RecurrenceInfo != nil {
			rendered += fmt.Sprintf("\n首次实例：%s（%s）", result.FirstOccurrence.ID, result.FirstOccurrence.RecurrenceInfo.Materialization)
		}
		return successResult(seriesCreateResultToMCPJSON(result), rendered)
	})
}

func registerTaskSeriesList(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name: "task_series_list", Description: "List recurring task series.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskSeriesListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		input := app.TaskSeriesListInput{
			Project: in.Project, ProjectID: in.ProjectID,
			Status: in.Status, Q: in.Q, Assignee: in.Assignee, Sort: in.Sort,
			Limit: in.Limit, Offset: in.Offset,
		}
		page, err := svc.ListTaskSeries(input)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		items := make([]map[string]any, 0, len(page.Items))
		for _, s := range page.Items {
			items = append(items, seriesViewToMCPJSON(s))
		}
		rendered := fmt.Sprintf("共 %d 个循环任务", page.Total)
		return successResult(map[string]any{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset}, rendered)
	})
}

func registerTaskSeriesGet(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name: "task_series_get", Description: "Get recurring task series details.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskSeriesRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		detail, err := svc.GetTaskSeries(in.ID)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rendered := fmt.Sprintf("循环任务：%s（%s）\n状态：%s\n未完成：%d 已完成：%d 已跳过：%d",
			detail.Series.Title, detail.Series.RecurrenceRule, detail.Series.Status,
			detail.Series.OpenOccurrenceCount, detail.Series.CompletedCount, detail.Series.SkippedCount)
		return successResult(seriesDetailToMCPJSON(detail), rendered)
	})
}

type TaskSeriesModifyInput struct {
	Workspace         string            `json:"workspace,omitempty"`
	ID                string            `json:"id"`
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

func (in TaskSeriesModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

func registerTaskSeriesModify(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name: "task_series_modify", Description: "Modify a recurring task series (shared fields; rule change requires effective_from).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskSeriesModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		effectiveFrom, err := resolveSeriesDueField(in.EffectiveFrom, in.EffectiveFromDate)
		if err != nil {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "task_series_invalid_effective_from", Message: err.Error()})
		}
		until, err := resolveSeriesDueField(in.Until, in.UntilDate)
		if err != nil {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "task_series_invalid_until", Message: err.Error()})
		}
		modify := app.ModifyTaskSeriesInput{
			Title: in.Title, Description: in.Description, Priority: in.Priority,
			Assignees: in.Assignees, Tags: in.Tags, UDAs: in.UDAs,
			RecurrenceRule: in.RecurrenceRule, EffectiveFrom: effectiveFrom,
			Until: until,
		}
		if err := app.ApplyTaskSeriesClearFields(&modify, in.Clear); err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ModifyTaskSeries(in.ID, modify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rendered := fmt.Sprintf("循环任务已修改：%s（%s）", view.Title, view.RecurrenceRule)
		return successResult(seriesViewToMCPJSON(view), rendered)
	})
}

func registerTaskSeriesStop(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name: "task_series_stop", Description: "Stop a recurring task series; no new occurrences are generated.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskSeriesStopInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		deleteOpen := in.DeleteOpenOccurrences
		view, err := svc.StopTaskSeries(in.ID, app.StopTaskSeriesInput{DeleteOpenOccurrences: &deleteOpen})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rendered := fmt.Sprintf("已停止循环任务：%s", view.Title)
		return successResult(seriesViewToMCPJSON(view), rendered)
	})
}

func registerTaskSeriesListOccurrences(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name: "task_series_list_occurrences", Description: "List materialized occurrences of a series.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskSeriesOccurrenceListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		dueAfter, err := parseOccurrenceRangeBoundary(in.DueAfter, false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		dueBefore, err := parseOccurrenceRangeBoundary(in.DueBefore, true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		input := app.TaskSeriesOccurrenceListInput{
			Status: in.Status, DueAfter: dueAfter, DueBefore: dueBefore,
			Limit: in.Limit, Offset: in.Offset,
		}
		page, err := svc.ListTaskSeriesOccurrences(in.ID, input)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		items := make([]map[string]any, 0, len(page.Items))
		for _, it := range page.Items {
			items = append(items, occurrenceViewToMCPJSON(it))
		}
		rendered := fmt.Sprintf("共 %d 条实例", page.Total)
		return successResult(map[string]any{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset}, rendered)
	})
}

func parseOccurrenceRangeBoundary(raw string, endExclusive bool) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return nil, app.RuntimeError{Code: "api_bad_filter", Message: "due range must use YYYY-MM-DD"}
	}
	if endExclusive {
		value = value.AddDate(0, 0, 1)
	}
	ts := value.Unix()
	return &ts, nil
}

func registerTaskSeriesOccurrenceSkip(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name: "task_series_occurrence_skip", Description: "Skip one occurrence of a series (creates a deleted tombstone).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskSeriesOccurrenceSkipInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.SkipTaskSeriesOccurrence(in.SeriesID, in.OccurrenceID)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rendered := fmt.Sprintf("已跳过实例：%s", view.ID)
		return successResult(occurrenceViewToMCPJSON(view), rendered)
	})
}

// --- 辅助 ---

func resolveSeriesDueField(instant *int64, date *string) (*int64, error) {
	if date == nil || strings.TrimSpace(*date) == "" {
		return instant, nil
	}
	return resolveToolDateField("due", instant, *date, true)
}

// seriesViewToMCPJSON 把 SeriesView 转为 MCP 输出 map。
func seriesViewToMCPJSON(v app.TaskSeriesView) map[string]any {
	out := map[string]any{
		"id": v.ID, "workspace_id": v.WorkspaceID, "project_id": v.ProjectID,
		"title": v.Title, "status": v.Status, "recurrence_rule": v.RecurrenceRule,
		"first_due":             v.FirstDue,
		"open_occurrence_count": v.OpenOccurrenceCount,
		"completed_count":       v.CompletedCount, "skipped_count": v.SkippedCount,
		"overdue_count": v.OverdueCount, "created_at": v.CreatedAt, "modified_at": v.ModifiedAt,
	}
	if v.ProjectSlug != "" {
		out["project_slug"] = v.ProjectSlug
	}
	if slug := app.SeriesSlugOf(v.Series); slug != "" {
		out["series_slug"] = slug
	}
	if v.Description != nil {
		out["description"] = *v.Description
	}
	if v.Until != nil {
		out["until"] = *v.Until
	}
	if v.Priority != nil {
		out["priority"] = *v.Priority
	}
	if len(v.Tags) > 0 {
		out["tags"] = v.Tags
	}
	if len(v.UDAs) > 0 {
		out["udas"] = v.UDAs
	}
	if len(v.Assignees) > 0 {
		out["assignees"] = userInfosForMCP(v.Assignees)
	}
	if v.CreatedBy.ID != "" {
		out["created_by"] = task.UserInfoToJSON(v.CreatedBy)
	}
	if v.NextRecurrenceAt != nil {
		out["next_recurrence_at"] = *v.NextRecurrenceAt
	}
	if v.SuggestedRuleEffectiveFrom != nil {
		out["suggested_rule_effective_from"] = *v.SuggestedRuleEffectiveFrom
	}
	return out
}

func seriesDetailToMCPJSON(detail app.TaskSeriesDetailView) map[string]any {
	out := seriesViewToMCPJSON(detail.Series)
	out["open_occurrences"] = occurrenceViewsForMCP(detail.OpenOccurrences)
	out["recent_completed"] = occurrenceViewsForMCP(detail.RecentCompleted)
	out["recent_skipped"] = occurrenceViewsForMCP(detail.RecentSkipped)
	return out
}

func occurrenceViewsForMCP(rows []app.TaskOccurrenceView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, occurrenceViewToMCPJSON(row))
	}
	return out
}

// occurrenceViewToMCPJSON 把 TaskOccurrenceView 转为 MCP 输出 map。
func occurrenceViewToMCPJSON(v app.TaskOccurrenceView) map[string]any {
	out := map[string]any{
		"id": v.ID, "workspace_id": v.WorkspaceID, "title": v.Title, "status": v.Status,
		"tags": v.Tags, "depends": v.Depends, "udas": udaValuesForMCP(v.UDAs),
		"assignees": userInfosForMCP(v.Assignees), "annotations": task.AnnotationsToJSON(v.Annotations),
		"links": taskLinksForMCP(v.Links),
		"uuid":  nil, "task_slug": nil, "project_seq": nil,
		"entry": nil, "modified": nil, "start": nil, "end": nil,
	}
	if v.Description != nil {
		out["description"] = *v.Description
	}
	if v.UUID != nil {
		out["uuid"] = *v.UUID
	}
	if v.Due != nil {
		out["due"] = *v.Due
	}
	for key, value := range map[string]*int64{
		"entry": v.Entry, "modified": v.Modified, "start": v.Start, "end": v.End,
		"wait": v.Wait, "scheduled": v.Scheduled, "until": v.Until,
	} {
		if value != nil {
			out[key] = *value
		}
	}
	if v.ProjectID != nil {
		out["project_id"] = *v.ProjectID
	}
	if v.Project != nil {
		out["project"] = *v.Project
	}
	if v.TaskSlug != nil {
		out["task_slug"] = *v.TaskSlug
	}
	if v.ProjectSeq != nil {
		out["project_seq"] = *v.ProjectSeq
	}
	if v.Priority != nil {
		out["priority"] = *v.Priority
	}
	if v.Parent != nil {
		out["parent"] = *v.Parent
	}
	if v.RecurrenceInfo != nil {
		out["recurrence_info"] = map[string]any{
			"role":            v.RecurrenceInfo.Role,
			"series_id":       v.RecurrenceInfo.SeriesID,
			"series_title":    v.RecurrenceInfo.SeriesTitle,
			"series_status":   v.RecurrenceInfo.SeriesStatus,
			"rule":            v.RecurrenceInfo.Rule,
			"recurrence_at":   v.RecurrenceInfo.RecurrenceAt,
			"materialization": v.RecurrenceInfo.Materialization,
			"overrides":       v.RecurrenceInfo.Overrides,
		}
		if v.RecurrenceInfo.Until != nil {
			out["recurrence_info"].(map[string]any)["until"] = *v.RecurrenceInfo.Until
		}
	}
	return out
}

// taskViewToMCPJSON 保持 task tool 既有的内层 task 字段，同时确保顶层和
// 内层都来自同一个 TaskOccurrenceView，而不是旧 task.JSONTask。
func taskViewToMCPJSON(view app.TaskOccurrenceView) map[string]any {
	out := occurrenceViewToMCPJSON(view)
	out["task"] = occurrenceViewToMCPJSON(view)
	out["completed"] = view.Status == task.StatusCompleted
	out["deleted"] = view.Status == task.StatusDeleted
	return out
}

func taskResolutionToMCPJSON(resolved app.TaskRefResolution) map[string]any {
	if resolved.Task != nil {
		return taskViewToMCPJSON(resolved.View)
	}
	return occurrenceViewToMCPJSON(resolved.View)
}

func userInfosForMCP(rows []task.UserInfo) []task.JSONUserInfo {
	out := make([]task.JSONUserInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, task.UserInfoToJSON(row))
	}
	return out
}

func udaValuesForMCP(values map[string]task.UDAValue) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[name] = value.Raw
	}
	return out
}

func taskLinksForMCP(rows []task.TaskLinkInfo) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		link := map[string]any{
			"id": row.ID, "type": row.Type, "url": row.URL,
			"created_at": row.CreatedAt, "created_by": task.ActorInfoToJSON(row.CreatedBy),
		}
		if row.Title != "" {
			link["title"] = row.Title
		}
		out = append(out, link)
	}
	return out
}

// formatOccurrenceViewText 把 occurrence view 渲染为人类可读文本。
func formatOccurrenceViewText(v app.TaskOccurrenceView) string {
	mat := "materialized"
	if v.RecurrenceInfo != nil {
		mat = v.RecurrenceInfo.Materialization
	}
	return fmt.Sprintf("任务 %s（%s）\n标题：%s\n状态：%s", v.ID, mat, v.Title, v.Status)
}

// seriesCreateResultToMCPJSON 把创建结果转为 MCP 输出 map。
func seriesCreateResultToMCPJSON(r app.TaskSeriesCreateResult) map[string]any {
	out := map[string]any{
		"series": seriesViewToMCPJSON(r.Series),
	}
	if r.FirstOccurrence != nil {
		out["first_occurrence"] = occurrenceViewToMCPJSON(*r.FirstOccurrence)
	}
	return out
}
