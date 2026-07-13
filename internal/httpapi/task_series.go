package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// --- Series DTO ---

type taskSeriesRequest struct {
	Title          *string          `json:"title"`
	Description    *string          `json:"description,omitempty"`
	Project        *string          `json:"project,omitempty"`
	ProjectID      *string          `json:"project_id,omitempty"`
	RecurrenceRule *string          `json:"recurrence_rule"`
	FirstDue       *int64           `json:"first_due,omitempty"`
	FirstDueDate   *string          `json:"first_due_date,omitempty"`
	Until          *int64           `json:"until,omitempty"`
	UntilDate      *string          `json:"until_date,omitempty"`
	Priority       *string          `json:"priority,omitempty"`
	Assignees      []string         `json:"assignees,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
	UDAs           map[string]string `json:"udas,omitempty"`
	// 修改专用
	EffectiveFrom     *int64  `json:"effective_from,omitempty"`
	EffectiveFromDate *string `json:"effective_from_date,omitempty"`
	Clear             []string `json:"clear,omitempty"`
}

type taskOccurrenceJSON struct {
	ID         string           `json:"id"`
	UUID       *string          `json:"uuid,omitempty"`
	TaskSlug   *string          `json:"task_slug,omitempty"`
	ProjectSeq *int64           `json:"project_seq,omitempty"`
	ProjectID  *string          `json:"project_id,omitempty"`
	Project    *string          `json:"project,omitempty"`
	Title      string           `json:"title"`
	Status     string           `json:"status"`
	Entry      *int64           `json:"entry,omitempty"`
	Modified   *int64           `json:"modified,omitempty"`
	Due        *int64           `json:"due,omitempty"`
	Priority   *string          `json:"priority,omitempty"`
	Tags       []string         `json:"tags,omitempty"`
	Assignees  []task.JSONUserInfo `json:"assignees,omitempty"`
	RecurrenceInfo *recurrenceInfoJSON `json:"recurrence_info,omitempty"`
}

type recurrenceInfoJSON struct {
	Role            string   `json:"role"`
	SeriesID        string   `json:"series_id"`
	SeriesStatus    string   `json:"series_status"`
	Rule            string   `json:"rule"`
	RecurrenceAt    int64    `json:"recurrence_at"`
	Materialization string   `json:"materialization"`
	Overrides       []string `json:"overrides,omitempty"`
	Until           *int64   `json:"until,omitempty"`
}

type taskViewPageJSON struct {
	Items          []taskOccurrenceJSON `json:"items"`
	Total          int                  `json:"total"`
	Limit          int                  `json:"limit"`
	Offset         int                  `json:"offset"`
	OccurrenceMode string               `json:"occurrence_mode"`
	Range          *taskViewRangeJSON   `json:"range,omitempty"`
}

type taskViewRangeJSON struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type taskSeriesJSON struct {
	ID                  string         `json:"id"`
	WorkspaceID         string         `json:"workspace_id"`
	ProjectID           string         `json:"project_id"`
	Title               string         `json:"title"`
	Description         *string        `json:"description,omitempty"`
	Status              string         `json:"status"`
	RecurrenceRule      string         `json:"recurrence_rule"`
	FirstDue            int64          `json:"first_due"`
	Until               *int64         `json:"until,omitempty"`
	Priority            *string        `json:"priority,omitempty"`
	Tags                []string       `json:"tags,omitempty"`
	Assignees           []task.JSONUserInfo `json:"assignees,omitempty"`
	OpenOccurrenceCount int            `json:"open_occurrence_count"`
	CompletedCount      int            `json:"completed_count"`
	SkippedCount        int            `json:"skipped_count"`
	OverdueCount        int            `json:"overdue_count"`
	NextRecurrenceAt    *int64         `json:"next_recurrence_at,omitempty"`
	CreatedBy           task.JSONUserInfo `json:"created_by"`
	CreatedAt           int64          `json:"created_at"`
	ModifiedAt          int64          `json:"modified_at"`
}

type taskSeriesListPageJSON struct {
	Items  []taskSeriesJSON `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

type taskSeriesCreateResultJSON struct {
	Series          taskSeriesJSON     `json:"series"`
	FirstOccurrence *taskOccurrenceJSON `json:"first_occurrence,omitempty"`
}

// --- 视图序列化 ---

func occurrenceViewToJSON(v app.TaskOccurrenceView) taskOccurrenceJSON {
	out := taskOccurrenceJSON{
		ID: v.ID, UUID: v.UUID, TaskSlug: v.TaskSlug, ProjectSeq: v.ProjectSeq,
		ProjectID: v.ProjectID, Project: v.Project, Title: v.Title, Status: v.Status,
		Entry: v.Entry, Modified: v.Modified, Due: v.Due, Priority: v.Priority,
		Tags: v.Tags,
		Assignees: taskUserInfoListToJSON(v.Assignees),
	}
	if v.RecurrenceInfo != nil {
		out.RecurrenceInfo = &recurrenceInfoJSON{
			Role: v.RecurrenceInfo.Role, SeriesID: v.RecurrenceInfo.SeriesID,
			SeriesStatus: v.RecurrenceInfo.SeriesStatus, Rule: v.RecurrenceInfo.Rule,
			RecurrenceAt: v.RecurrenceInfo.RecurrenceAt, Materialization: v.RecurrenceInfo.Materialization,
			Overrides: v.RecurrenceInfo.Overrides, Until: v.RecurrenceInfo.Until,
		}
	}
	return out
}

func taskViewPageToJSON(page app.TaskViewPage) taskViewPageJSON {
	items := make([]taskOccurrenceJSON, 0, len(page.Items))
	for _, it := range page.Items {
		items = append(items, occurrenceViewToJSON(it))
	}
	out := taskViewPageJSON{
		Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset,
		OccurrenceMode: string(page.OccurrenceMode),
	}
	if page.Range != nil {
		out.Range = &taskViewRangeJSON{Start: page.Range.Start, End: page.Range.End}
	}
	return out
}

func seriesViewToJSON(v app.TaskSeriesView) taskSeriesJSON {
	assignees := taskUserInfoListToJSON(seriesAssigneesToUserInfo(v))
	return taskSeriesJSON{
		ID: v.ID, WorkspaceID: v.WorkspaceID, ProjectID: v.ProjectID, Title: v.Title,
		Description: v.Description, Status: v.Status, RecurrenceRule: v.RecurrenceRule,
		FirstDue: v.FirstDue, Until: v.Until, Priority: v.Priority, Tags: v.Tags,
		Assignees: assignees, OpenOccurrenceCount: v.OpenOccurrenceCount,
		CompletedCount: v.CompletedCount, SkippedCount: v.SkippedCount,
		OverdueCount: v.OverdueCount, NextRecurrenceAt: v.NextRecurrenceAt,
		CreatedBy: taskUserInfoToJSON(v.CreatedBy),
		CreatedAt: v.CreatedAt, ModifiedAt: v.ModifiedAt,
	}
}

func seriesAssigneesToUserInfo(v app.TaskSeriesView) []task.UserInfo {
	out := make([]task.UserInfo, 0, len(v.AssigneeIDs))
	// series 没有独立 assignee 解析；这里用 view 已有的信息。
	// TaskSeriesView.AssigneeIDs 是 user id 列表，无 name；保持 id fallback。
	for _, id := range v.AssigneeIDs {
		out = append(out, task.UserInfo{ID: id, Name: id})
	}
	return out
}

// --- handlers ---

func (s *Server) handleTaskSeriesList(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	q := r.URL.Query()
	input := app.TaskSeriesListInput{
		ProjectID: q.Get("project_id"),
		Project:   q.Get("project"),
		Status:    q.Get("status"),
		Q:         q.Get("q"),
		Assignee:  q.Get("assignee"),
		Sort:      q.Get("sort"),
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			input.Limit = n
		}
	}
	if raw := q.Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			input.Offset = n
		}
	}
	page, err := scoped.ListTaskSeries(input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	items := make([]taskSeriesJSON, 0, len(page.Items))
	for _, it := range page.Items {
		items = append(items, seriesViewToJSON(it))
	}
	writeSuccess(w, http.StatusOK, taskSeriesListPageJSON{
		Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset,
	}, nil)
}

func (s *Server) handleTaskSeriesAdd(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req taskSeriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_request", "invalid JSON body", nil)
		return
	}
	input, err := seriesRequestToInput(req, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	result, err := scoped.AddTaskSeries(input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	resp := taskSeriesCreateResultJSON{Series: seriesViewToJSON(result.Series)}
	if result.FirstOccurrence != nil {
		occ := occurrenceViewToJSON(*result.FirstOccurrence)
		resp.FirstOccurrence = &occ
	}
	writeSuccess(w, http.StatusCreated, resp, nil)
}

func (s *Server) handleTaskSeriesGet(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	seriesRef := pathParam(r, "seriesRef")
	detail, err := scoped.GetTaskSeries(seriesRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, seriesViewToJSON(detail.Series), nil)
}

func (s *Server) handleTaskSeriesModify(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	seriesRef := pathParam(r, "seriesRef")
	var req taskSeriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_request", "invalid JSON body", nil)
		return
	}
	input := app.ModifyTaskSeriesInput{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Assignees:   req.Assignees,
		Tags:        req.Tags,
		UDAs:        req.UDAs,
		Until:       req.Until,
	}
	if req.RecurrenceRule != nil {
		input.RecurrenceRule = req.RecurrenceRule
	}
	if req.EffectiveFrom != nil {
		input.EffectiveFrom = req.EffectiveFrom
	}
	if req.EffectiveFromDate != nil {
		ts, err := parseDeadlineDate(*req.EffectiveFromDate)
		if err != nil {
			writeAppError(w, app.RuntimeError{Code: "task_series_invalid_effective_from", Message: err.Error()})
			return
		}
		input.EffectiveFrom = &ts
	}
	view, err := scoped.ModifyTaskSeries(seriesRef, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, seriesViewToJSON(view), nil)
}

func (s *Server) handleTaskSeriesDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	seriesRef := pathParam(r, "seriesRef")
	deleteOpen := isTruthyQueryValue(r.URL.Query().Get("delete_open_occurrences"))
	view, err := scoped.StopTaskSeries(seriesRef, app.StopTaskSeriesInput{DeleteOpenOccurrences: &deleteOpen})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, seriesViewToJSON(view), nil)
}

func (s *Server) handleTaskSeriesOccurrencesList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	seriesRef := pathParam(r, "seriesRef")
	q := r.URL.Query()
	input := app.TaskSeriesOccurrenceListInput{Status: q.Get("status")}
	if raw := q.Get("due_after"); raw != "" {
		if ts, perr := parseDueAfter(raw); perr == nil {
			input.DueAfter = &ts
		}
	}
	if raw := q.Get("due_before"); raw != "" {
		if ts, perr := parseDueBefore(raw); perr == nil {
			input.DueBefore = &ts
		}
	}
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			input.Limit = n
		}
	}
	if raw := q.Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			input.Offset = n
		}
	}
	page, err := scoped.ListTaskSeriesOccurrences(seriesRef, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskViewPageToJSON(page), nil)
}

func (s *Server) handleTaskSeriesOccurrenceSkip(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	seriesRef := pathParam(r, "seriesRef")
	occurrenceRef := pathParam(r, "occurrenceRef")
	view, err := scoped.SkipTaskSeriesOccurrence(seriesRef, occurrenceRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, occurrenceViewToJSON(view), nil)
}

// --- 辅助 ---

func seriesRequestToInput(req taskSeriesRequest, projectRef string) (app.AddTaskSeriesInput, error) {
	input := app.AddTaskSeriesInput{
		Title: strValueOr(req.Title, ""),
		Description: req.Description,
		RecurrenceRule: strValueOr(req.RecurrenceRule, ""),
		Priority: req.Priority, Assignees: req.Assignees, Tags: req.Tags, UDAs: req.UDAs,
	}
	if projectRef != "" {
		input.Project = &projectRef
	}
	if req.ProjectID != nil {
		input.ProjectID = *req.ProjectID
	}
	if req.Project != nil {
		p := *req.Project
		input.Project = &p
	}
	// first_due（unix 或 date-only）。
	if req.FirstDue != nil {
		input.FirstDue = *req.FirstDue
	} else if req.FirstDueDate != nil {
		ts, err := parseDeadlineDate(*req.FirstDueDate)
		if err != nil {
			return app.AddTaskSeriesInput{}, app.RuntimeError{Code: "task_series_due_required", Message: err.Error()}
		}
		input.FirstDue = ts
	}
	if req.Until != nil {
		input.Until = req.Until
	} else if req.UntilDate != nil {
		ts, err := parseDeadlineDate(*req.UntilDate)
		if err != nil {
			return app.AddTaskSeriesInput{}, app.RuntimeError{Code: "task_series_invalid_until", Message: err.Error()}
		}
		input.Until = &ts
	}
	return input, nil
}

func parseDeadlineDate(date string) (int64, error) {
	return query.ResolveDeadlineDateValue(query.ParseDateValue(date), time.Now().Unix(), time.Local)
}

func parseDueAfter(date string) (int64, error) {
	// due_after = 当天 00:00 inclusive。
	return query.ResolveStartDateValue(query.ParseDateValue(date), time.Now().Unix(), time.Local)
}

func parseDueBefore(date string) (int64, error) {
	// due_before = 当天结束，转次日 00:00 exclusive。
	start, err := query.ResolveStartDateValue(query.ParseDateValue(date), time.Now().Unix(), time.Local)
	if err != nil {
		return 0, err
	}
	return start + 86400, nil
}

func strValueOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func pathParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}

// handleTaskListViewPage 处理带 occurrence_mode/due range 的任务查询，返回 TaskViewPage（spec §13.3）。
func (s *Server) handleTaskListViewPage(w http.ResponseWriter, r *http.Request, scoped *app.Service, projectRef string) {
	q := r.URL.Query()
	mode := app.OccurrenceMode(q.Get("occurrence_mode"))
	if mode == "" {
		mode = app.OccurrenceModeAuto
	}
	// occurrence range 只在 expand 模式下从 due_after/due_before 构建（spec §13.3）。
	// materialized/auto 模式下 due_after/due_before 作为普通 restful due filter。
	var rng *app.TaskViewRange
	if mode == app.OccurrenceModeExpand {
		if q.Get("due_after") != "" || q.Get("due_before") != "" {
			start, serr := parseDueAfterOrDefault(q.Get("due_after"))
			if serr != nil {
				writeError(w, http.StatusBadRequest, "api_bad_filter", serr.Error(), nil)
				return
			}
			end, eerr := parseDueBeforeOrDefault(q.Get("due_before"))
			if eerr != nil {
				writeError(w, http.StatusBadRequest, "api_bad_filter", eerr.Error(), nil)
				return
			}
			if start > 0 || end > 0 {
				if end == 0 {
					end = start + 366*86400
				}
				if start == 0 {
					start = end - 366*86400
				}
				rng = &app.TaskViewRange{Start: start, End: end}
			}
		}
	}
	// limit 校验（与旧 /tasks 行为一致）。
	limit := taskListDefaultLimit
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "invalid limit", nil)
			return
		}
		if parsed > taskListMaxLimit {
			writeError(w, http.StatusBadRequest, "api_bad_limit", fmt.Sprintf("limit must be <= %d", taskListMaxLimit), nil)
			return
		}
		limit = parsed
	}
	// query / filter（spec §17.1）。
	var queryExpr query.Expr
	// target：解析为 UUID 后作为 query 过滤（纯数字 target 会被拒绝）。
	if target := strings.TrimSpace(q.Get("target")); target != "" {
		tsk, err := scoped.ResolveProtocolTarget(target)
		if err != nil {
			writeAppError(w, err)
			return
		}
		queryExpr = query.And(queryExpr, query.Predicate{
			Attribute: query.AttrUUID, Operator: query.OpEqual, Value: query.StringValue(tsk.UUID),
		})
	}
	filters := q["query"]
	if len(filters) == 0 {
		filters = q["filter"]
	}
	if len(filters) > 0 {
		expr, err := query.ParseFilterExpr(filters)
		if err != nil {
			writeAppError(w, err)
			return
		}
		queryExpr = query.And(queryExpr, expr)
	}
	// restful 参数（status/priority/project/tag/due/wait 等）。
	restful, err := restfulTaskFilters(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_filter", err.Error(), nil)
		return
	}
	queryExpr = query.And(queryExpr, restful)
	input := app.TaskViewQuery{
		OccurrenceMode: mode, Range: rng,
		Status: q.Get("status"), Sort: q.Get("sort"),
		Query: queryExpr, Limit: limit,
		NoContext: isTruthyQueryValue(q.Get("no_context")),
	}
	if raw := q.Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			input.Offset = n
		}
	}
	if projectRef != "" {
		project, err := scoped.ProjectInfo(projectRef)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.ProjectID = project.ID
	}
	page, err := scoped.QueryTaskViews(input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskViewPageToJSON(page), nil)
}

func parseDueAfterOrDefault(date string) (int64, error) {
	if date == "" {
		return 0, nil
	}
	return parseDueAfter(date)
}

func parseDueBeforeOrDefault(date string) (int64, error) {
	if date == "" {
		return 0, nil
	}
	return parseDueBefore(date)
}

func taskUserInfoToJSON(u task.UserInfo) task.JSONUserInfo {
	return task.UserInfoToJSON(u)
}

func taskUserInfoListToJSON(in []task.UserInfo) []task.JSONUserInfo {
	out := make([]task.JSONUserInfo, 0, len(in))
	for _, u := range in {
		out = append(out, task.UserInfoToJSON(u))
	}
	return out
}
