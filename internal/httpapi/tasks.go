package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type addTaskRequest struct {
	Title         string   `json:"title"`
	Description   *string  `json:"description,omitempty"`
	Project       string   `json:"project,omitempty"`
	ProjectID     string   `json:"project_id,omitempty"`
	Priority      string   `json:"priority,omitempty"`
	Due           *int64   `json:"due,omitempty"`
	DueDate       string   `json:"due_date,omitempty"`
	Assignees     []string `json:"assignees,omitempty"`
	Depends       []string `json:"depends,omitempty"`
	Wait          *int64   `json:"wait,omitempty"`
	WaitDate      string   `json:"wait_date,omitempty"`
	Scheduled     *int64   `json:"scheduled,omitempty"`
	ScheduledDate string   `json:"scheduled_date,omitempty"`
	Until         *int64   `json:"until,omitempty"`
	UntilDate     string   `json:"until_date,omitempty"`
	// recur 已移除（spec §11.1）：循环任务通过 /task-series 管理。
	// 保留字段用于检测并拒绝旧请求，不传递给 App 层。
	Recur                 *string           `json:"recur,omitempty"`
	Tags                  []string          `json:"tags,omitempty"`
	UDAs                  map[string]string `json:"udas,omitempty"`
	Parent                string            `json:"parent,omitempty"`
	AttachmentDraftTarget string            `json:"attachment_draft_target,omitempty"`
}

type modifyTaskRequest struct {
	Title            *string  `json:"title,omitempty"`
	Description      *string  `json:"description,omitempty"`
	ClearDescription bool     `json:"clear_description,omitempty"`
	Project          *string  `json:"project,omitempty"`
	ProjectID        *string  `json:"project_id,omitempty"`
	Priority         *string  `json:"priority,omitempty"`
	ClearProject     bool     `json:"clear_project,omitempty"`
	ClearPriority    bool     `json:"clear_priority,omitempty"`
	Due              *int64   `json:"due,omitempty"`
	DueDate          string   `json:"due_date,omitempty"`
	ClearDue         bool     `json:"clear_due,omitempty"`
	Wait             *int64   `json:"wait,omitempty"`
	WaitDate         string   `json:"wait_date,omitempty"`
	ClearWait        bool     `json:"clear_wait,omitempty"`
	Scheduled        *int64   `json:"scheduled,omitempty"`
	ScheduledDate    string   `json:"scheduled_date,omitempty"`
	ClearScheduled   bool     `json:"clear_scheduled,omitempty"`
	Until            *int64   `json:"until,omitempty"`
	UntilDate        string   `json:"until_date,omitempty"`
	ClearUntil       bool     `json:"clear_until,omitempty"`
	Assignees        []string `json:"assignees,omitempty"`
	RemoveAssignees  []string `json:"remove_assignees,omitempty"`
	ClearAssignees   bool     `json:"clear_assignees,omitempty"`
	Depends          []string `json:"depends,omitempty"`
	ClearDepends     bool     `json:"clear_depends,omitempty"`
	// recur/clear_recur 已移除（spec §11.1）：保留字段用于检测并拒绝旧请求。
	Recur      *string           `json:"recur,omitempty"`
	ClearRecur bool              `json:"clear_recur,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	RemoveTags []string          `json:"remove_tags,omitempty"`
	UDAs       map[string]string `json:"udas,omitempty"`
	ClearUDAs  []string          `json:"clear_udas,omitempty"`
}

type textRequest struct {
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
}

type addLinkRequest struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

type linkJSON struct {
	ID        string             `json:"id"`
	Type      string             `json:"type"`
	URL       string             `json:"url"`
	Title     string             `json:"title,omitempty"`
	CreatedAt string             `json:"created_at"`
	CreatedBy task.JSONActorInfo `json:"created_by"`
}

const (
	taskListDefaultLimit = 200
	taskListMaxLimit     = 1000
)

// restfulTaskFilters 把 restful 风格的 query 参数翻译成 query DSL，
// 与原有 query=/filter= 表达式合并。参数校验失败返回 error。
//
// 这些参数与 taskwarrior 风格的 query=/filter= 并存，翻译后用 query.And 合并，
// 便于前端用可读 URL（如 ?status=pending&priority=H）而不必直接拼 DSL。
func restfulTaskFilters(q url.Values) (query.Expr, error) {
	var expr query.Expr
	add := func(e query.Expr) { expr = query.And(expr, e) }

	if v := strings.TrimSpace(q.Get("status")); v != "" {
		add(query.Predicate{Attribute: query.AttrStatus, Operator: query.OpEqual, Value: query.StringValue(v)})
	}
	if v := strings.TrimSpace(q.Get("priority")); v != "" {
		add(query.Predicate{Attribute: query.AttrPriority, Operator: query.OpEqual, Value: query.StringValue(v)})
	}
	if v := strings.TrimSpace(q.Get("assignee")); v != "" {
		add(query.Predicate{Attribute: query.AttrAssignee, Operator: query.OpEqual, Value: query.StringValue(v)})
	}
	if v := strings.TrimSpace(q.Get("due_after")); v != "" {
		pred, err := inclusiveAfterDatePredicate(query.AttrDue, v)
		if err != nil {
			return nil, err
		}
		add(pred)
	}
	if v := strings.TrimSpace(q.Get("due_before")); v != "" {
		pred, err := inclusiveBeforeDatePredicate(query.AttrDue, v)
		if err != nil {
			return nil, err
		}
		add(pred)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		add(query.Predicate{Attribute: query.AttrBare, Operator: query.OpContains, Value: query.BareValue(v)})
	}
	if v := strings.TrimSpace(q.Get("tags")); v != "" {
		for _, tag := range strings.Split(v, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			add(query.Predicate{Attribute: query.AttrTag, Operator: query.OpHasTag, Value: query.StringValue(tag)})
		}
	}
	if v := strings.TrimSpace(q.Get("task_type")); v != "" && v != "all" {
		if v != "normal" && v != "occurrence" {
			return nil, fmt.Errorf("task_type must be all|normal|occurrence")
		}
		add(query.Predicate{Attribute: query.AttrTaskType, Operator: query.OpEqual, Value: query.StringValue(v)})
	}
	return expr, nil
}

func datePredicate(attr query.Attribute, op query.Operator, raw string) (query.Expr, error) {
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return nil, fmt.Errorf("invalid date %q (expected YYYY-MM-DD)", raw)
	}
	return query.Predicate{Attribute: attr, Operator: op, Value: query.DateValue(raw)}, nil
}

func inclusiveBeforeDatePredicate(attr query.Attribute, raw string) (query.Expr, error) {
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q (expected YYYY-MM-DD)", raw)
	}
	nextDay := parsed.AddDate(0, 0, 1).Format("2006-01-02")
	return query.Predicate{Attribute: attr, Operator: query.OpBefore, Value: query.DateValue(nextDay)}, nil
}

// inclusiveAfterDatePredicate 把 REST 的 due_after 日期解释为“从当天
// 00:00 起（含）”。查询语言的 after 仍保持严格比较，因此用“当天相等
// 或晚于当天起点”组合，确保与 due_before 的含当天边界对称。
func inclusiveAfterDatePredicate(attr query.Attribute, raw string) (query.Expr, error) {
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return nil, fmt.Errorf("invalid date %q (expected YYYY-MM-DD)", raw)
	}
	value := query.DateValue(raw)
	return query.Or(
		query.Predicate{Attribute: attr, Operator: query.OpEqual, Value: value},
		query.Predicate{Attribute: attr, Operator: query.OpAfter, Value: value},
	), nil
}

func resolveRequestDateField(field string, instant *int64, date string, endOfDay bool) (*int64, error) {
	date = strings.TrimSpace(date)
	if date == "" {
		return instant, nil
	}
	if instant != nil {
		return nil, fmt.Errorf("%s and %s_date cannot both be set", field, field)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, fmt.Errorf("%s_date must be YYYY-MM-DD", field)
	}
	var (
		value int64
		err   error
	)
	if endOfDay {
		value, err = query.ResolveDeadlineDateValue(query.ParseDateValue(date), time.Now().Unix(), time.Local)
	} else {
		value, err = query.ResolveStartDateValue(query.ParseDateValue(date), time.Now().Unix(), time.Local)
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func resolveTaskDateFields(due *int64, dueDate string, wait *int64, waitDate string, scheduled *int64, scheduledDate string, until *int64, untilDate string) (*int64, *int64, *int64, *int64, error) {
	resolvedDue, err := resolveRequestDateField("due", due, dueDate, true)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resolvedWait, err := resolveRequestDateField("wait", wait, waitDate, false)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resolvedScheduled, err := resolveRequestDateField("scheduled", scheduled, scheduledDate, false)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resolvedUntil, err := resolveRequestDateField("until", until, untilDate, true)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return resolvedDue, resolvedWait, resolvedScheduled, resolvedUntil, nil
}

func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	// report 路径仍走 RunTaskViewReport（spec §17.3）。
	if reportName := strings.TrimSpace(r.URL.Query().Get("report")); reportName != "" {
		s.handleTaskListReport(w, r, scoped, projectRef, reportName)
		return
	}
	// 所有其他路径统一走 QueryTaskViews，返回 TaskViewPage（spec §13.3、§17.3）。
	s.handleTaskListViewPage(w, r, scoped, projectRef)
}

// handleTaskListReport 处理 /tasks?report={name}（spec §17.3）。
func (s *Server) handleTaskListReport(w http.ResponseWriter, r *http.Request, scoped *app.Service, projectRef, reportName string) {
	q := r.URL.Query()
	// query / filter。
	var queryExpr query.Expr
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
	restful, err := restfulTaskFilters(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_filter", err.Error(), nil)
		return
	}
	queryExpr = query.And(queryExpr, restful)
	if projectRef != "" {
		project, err := scoped.ProjectInfo(projectRef)
		if err != nil {
			writeAppError(w, err)
			return
		}
		queryExpr = query.And(queryExpr, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(project.ID)})
	}
	reportInput := app.ReportViewInput{
		Name:      reportName,
		Query:     queryExpr,
		Sort:      q.Get("sort"),
		NoContext: isTruthyQueryValue(q.Get("no_context")),
	}
	// occurrence_mode / due range。
	if raw := q.Get("occurrence_mode"); raw != "" {
		reportInput.OccurrenceMode = app.OccurrenceMode(raw)
	}
	if da := q.Get("due_after"); da != "" {
		if start, serr := parseDueAfter(da); serr == nil {
			if end, eerr := parseDueBefore(q.Get("due_before")); eerr == nil && end > 0 {
				reportInput.Range = &app.TaskViewRange{Start: start, End: end}
			}
		}
	}
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			reportInput.Limit = n
		}
	}
	if raw := q.Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			reportInput.Offset = n
		}
	}
	page, err := scoped.RunTaskViewReport(reportInput)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskViewPageToJSON(page), nil)
}

func isTruthyQueryValue(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value == "true" || value == "1" || value == "yes"
}

func (s *Server) handleTaskAdd(w http.ResponseWriter, r *http.Request) {
	var req addTaskRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if req.Recur != nil {
		writeError(w, http.StatusBadRequest, "task_series_endpoint_required",
			"recur 字段已移除，循环任务请使用 POST /api/v1/task-series", nil)
		return
	}
	if ok := s.ensureTaskAddProjectRefs(w, r, req); !ok {
		return
	}
	projectRef := requestProjectRef(r)
	if projectRef == "" {
		projectRef = strings.TrimSpace(req.ProjectID)
	}
	if projectRef == "" {
		projectRef = strings.TrimSpace(req.Project)
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := ensureProjectRefsMatch(scoped, req.Project, req.ProjectID); err != nil {
		writeAppError(w, err)
		return
	}
	project := strings.TrimSpace(req.Project)
	if project == "" && strings.TrimSpace(req.ProjectID) != "" {
		view, err := scoped.ProjectInfo(req.ProjectID)
		if err != nil {
			writeAppError(w, err)
			return
		}
		project = view.Slug
	}
	var priority *string
	if value := strings.TrimSpace(req.Priority); value != "" {
		priority = &value
	}
	var projectPtr *string
	if project != "" {
		projectPtr = &project
	}
	due, wait, scheduled, until, err := resolveTaskDateFields(req.Due, req.DueDate, req.Wait, req.WaitDate, req.Scheduled, req.ScheduledDate, req.Until, req.UntilDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_date", err.Error(), nil)
		return
	}
	created, err := scoped.AddTaskView(app.AddInput{
		Title:                 strings.TrimSpace(req.Title),
		Description:           req.Description,
		Project:               projectPtr,
		Priority:              priority,
		Due:                   due,
		Assignees:             req.Assignees,
		Depends:               req.Depends,
		Wait:                  wait,
		Scheduled:             scheduled,
		Until:                 until,
		Tags:                  req.Tags,
		UDAs:                  req.UDAs,
		Parent:                stringPtrIfPresent(req.Parent),
		AttachmentDraftTarget: strings.TrimSpace(req.AttachmentDraftTarget),
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, occurrenceViewToJSON(created), nil)
}

func (s *Server) ensureTaskAddProjectRefs(w http.ResponseWriter, r *http.Request, req addTaskRequest) bool {
	if strings.TrimSpace(req.Project) == "" || strings.TrimSpace(req.ProjectID) == "" {
		return true
	}
	authn, ok := authFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return false
	}
	workspaceRef := requestWorkspaceRef(r)
	if workspaceRef == "" {
		workspaceRef = authn.EffectiveWorkspace.ID
	}
	preflightSvc, _, err := s.scopedServiceFor(r, scopedServiceInput{
		Capability:   auth.ScopeTaskWrite,
		Permission:   app.PermissionTaskWrite,
		WorkspaceRef: workspaceRef,
	})
	if err != nil {
		writeAppError(w, err)
		return false
	}
	if err := ensureProjectRefsMatch(preflightSvc, req.Project, req.ProjectID); err != nil {
		writeAppError(w, err)
		return false
	}
	return true
}

func (s *Server) handleTaskInfo(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	resolved, err := scoped.ResolveTaskReferenceForRead(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskResolutionToJSON(scoped, resolved), nil)
}

// taskToJSONWithRefs 序列化任务并填充 depends_info/parent_info/blocked_by_info，
// 把裸 UUID 展开为带描述和 task_slug 的可读引用（用于 web 详情页展示和跳转）。
// 找不到的引用 UUID 会被忽略，前端回退显示原始 UUID。
func taskToJSONWithRefs(svc *app.Service, tsk task.Task) task.JSONTask {
	out := task.ToJSON(tsk)
	refs, err := svc.ResolveTaskRefs(tsk.Depends)
	if err == nil && len(refs) > 0 {
		out.DependsInfo = refs
	}
	if tsk.Parent != nil {
		parentRefs, err := svc.ResolveTaskRefs([]string{*tsk.Parent})
		if err == nil && len(parentRefs) > 0 {
			out.ParentInfo = &parentRefs[0]
		}
	}
	blockedBy, err := svc.ResolveDependents(tsk.UUID)
	if err == nil && len(blockedBy) > 0 {
		out.BlockedByInfo = blockedBy
	}
	return out
}

func (s *Server) handleTaskModify(w http.ResponseWriter, r *http.Request) {
	var req modifyTaskRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if req.Recur != nil || req.ClearRecur {
		writeError(w, http.StatusBadRequest, "task_series_endpoint_required",
			"recur/clear_recur 字段已移除，循环任务请使用 /api/v1/task-series", nil)
		return
	}
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	// 请求 scope 只能来自 URL/鉴权上下文。body 中的 project/project_id 是
	// 修改目标，不能拿它来解析源任务，否则会在业务 invariant 之前隐藏源任务，
	// 也可能让仅获目标项目授权的 token 绕过源项目可见性检查。
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if req.Project != nil && req.ProjectID != nil {
		if err := ensureProjectRefsMatch(scoped, *req.Project, *req.ProjectID); err != nil {
			writeAppError(w, err)
			return
		}
	}
	project := req.Project
	if project == nil && req.ProjectID != nil && strings.TrimSpace(*req.ProjectID) != "" {
		view, err := scoped.ProjectInfo(*req.ProjectID)
		if err != nil {
			writeAppError(w, err)
			return
		}
		project = &view.Slug
	}
	due, wait, scheduled, until, err := resolveTaskDateFields(req.Due, req.DueDate, req.Wait, req.WaitDate, req.Scheduled, req.ScheduledDate, req.Until, req.UntilDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_date", err.Error(), nil)
		return
	}
	if err := scoped.Modify(taskRef, app.ModifyInput{
		Title:            req.Title,
		Description:      req.Description,
		ClearDescription: req.ClearDescription,
		Project:          project,
		ClearProject:     req.ClearProject,
		Priority:         req.Priority,
		ClearPriority:    req.ClearPriority,
		Due:              due,
		ClearDue:         req.ClearDue,
		Wait:             wait,
		ClearWait:        req.ClearWait,
		Scheduled:        scheduled,
		ClearScheduled:   req.ClearScheduled,
		Until:            until,
		ClearUntil:       req.ClearUntil,
		AddAssignees:     req.Assignees,
		RemoveAssignees:  req.RemoveAssignees,
		ClearAssignees:   req.ClearAssignees,
		AddDepends:       req.Depends,
		ClearDepends:     req.ClearDepends,
		AddTags:          req.Tags,
		RemoveTags:       req.RemoveTags,
		UDAs:             req.UDAs,
		ClearUDAs:        req.ClearUDAs,
	}); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, taskRef)
}

func decodeStrictJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func (s *Server) handleTaskDone(w http.ResponseWriter, r *http.Request) {
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Done(id) })
}

func (s *Server) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Delete(id) })
}

func (s *Server) handleTaskStart(w http.ResponseWriter, r *http.Request) {
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Start(id) })
}

func (s *Server) handleTaskStop(w http.ResponseWriter, r *http.Request) {
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Stop(id) })
}

func (s *Server) handleTaskReopen(w http.ResponseWriter, r *http.Request) {
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Reopen(id) })
}

func (s *Server) handleTaskAnnotate(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	text := req.Description
	if text == "" {
		text = req.Text
	}
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Annotate(id, text) })
}

func (s *Server) handleTaskDenotate(w http.ResponseWriter, r *http.Request) {
	annotationID := strings.TrimSpace(chi.URLParam(r, "annotationID"))
	if annotationID == "" {
		writeError(w, http.StatusBadRequest, "annotation_id_required", "annotation id is required", nil)
		return
	}
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Denotate(id, annotationID) })
}

func (s *Server) handleTaskAnnotationUpdate(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	annotationID := strings.TrimSpace(chi.URLParam(r, "annotationID"))
	if annotationID == "" {
		writeError(w, http.StatusBadRequest, "annotation_id_required", "annotation id is required", nil)
		return
	}
	text := req.Description
	if text == "" {
		text = req.Text
	}
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error {
		return svc.UpdateAnnotation(id, annotationID, text)
	})
}

// handleTaskAnnotationList 处理 GET /api/v1/tasks/{taskRef}/annotations，
// 按 entry 倒序分页返回注解。
func (s *Server) handleTaskAnnotationList(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "api_bad_offset", "invalid offset", nil)
			return
		}
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > 100 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "limit must be between 1 and 100", nil)
			return
		}
	}
	annotations, total, err := scoped.ListAnnotations(taskRef, offset, limit)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{
		"annotations": task.AnnotationsToJSON(annotations),
		"total":       total,
		"offset":      offset,
		"limit":       limit,
	}, nil)
}

func (s *Server) handleTaskUrgency(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	result, err := scoped.ExplainUrgency(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, result, nil)
}

func (s *Server) handleTaskLinkAdd(w http.ResponseWriter, r *http.Request) {
	var req addLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	link, err := scoped.TaskAddLink(taskRef, req.Type, req.URL, req.Title)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, taskLinkToJSON(link), nil)
}

func (s *Server) handleTaskLinkList(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.GetTaskView(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskLinksToJSON(view.Links), nil)
}

// handleTaskChildren 列出任务的直接子任务（手动 sub-task 与 recurring child）。
// include_closed=true 时返回 completed/deleted，默认只返回 open（spec §8.3）。
func (s *Server) handleTaskChildren(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	includeClosed := isTruthyQueryValue(r.URL.Query().Get("include_closed"))
	children, err := scoped.ListChildren(taskRef, includeClosed)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tasksToJSON(children), nil)
}

func (s *Server) handleTaskLinkRemove(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	linkID := chi.URLParam(r, "linkID")
	if linkID == "" {
		writeError(w, http.StatusBadRequest, "link_id_required", "link ID is required", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.TaskRemoveLink(taskRef, linkID); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, taskRef)
}

func (s *Server) handleTaskLinkUpdate(w http.ResponseWriter, r *http.Request) {
	var req addLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	linkID := strings.TrimSpace(chi.URLParam(r, "linkID"))
	if linkID == "" {
		writeError(w, http.StatusBadRequest, "link_id_required", "link ID is required", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	link, err := scoped.TaskUpdateLink(taskRef, linkID, req.Type, req.URL, req.Title)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskLinkToJSON(link), nil)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	// /reports/{name} 与 /tasks?report={name} 共用同一逻辑（spec §17.3）。
	s.handleTaskListReport(w, r, scoped, projectRef, chi.URLParam(r, "name"))
}

func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request, fn func(*app.Service, string) error) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := fn(scoped, taskRef); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, taskRef)
}

func writeTaskAfterMutation(w http.ResponseWriter, svc *app.Service, taskRef string) {
	resolved, err := svc.ResolveTaskReferenceForRead(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskResolutionToJSON(svc, resolved), nil)
}

func requireTaskRef(w http.ResponseWriter, r *http.Request) (string, bool) {
	taskRef, err := decodedPathParam(r, "taskRef")
	if err != nil {
		writeAppError(w, err)
		return "", false
	}
	if err := app.ValidateProtocolTaskRef(taskRef); err != nil {
		writeAppError(w, err)
		return "", false
	}
	return taskRef, true
}

// decodedPathParam 对结构化 path 参数只解码一次。
// 残留百分号代表双重编码或非法 escape；斜杠和控制字符不能进入资源引用。
func decodedPathParam(r *http.Request, name string) (string, error) {
	// %25 会在 net/url 或路由层先还原成字面量 %，随后再次 unescape 就会
	// 形成双重解码。先检查原始 escaped path，明确拒绝这种输入。
	if strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%25") {
		return "", app.RuntimeError{Code: "task_ref_invalid", Message: "invalid path parameter"}
	}
	raw := strings.TrimSpace(chi.URLParam(r, name))
	value, err := url.PathUnescape(raw)
	if err != nil || strings.Contains(value, "%") || strings.ContainsAny(value, "/\x00\r\n") {
		return "", app.RuntimeError{Code: "task_ref_invalid", Message: "invalid path parameter"}
	}
	return strings.TrimSpace(value), nil
}

func tasksToJSON(rows []task.Task) []task.JSONTask {
	out := make([]task.JSONTask, len(rows))
	for i, row := range rows {
		out[i] = task.ToJSON(row)
	}
	return out
}

func ensureProjectRefsMatch(svc *app.Service, projectSlug, projectID string) error {
	projectSlug = strings.TrimSpace(projectSlug)
	projectID = strings.TrimSpace(projectID)
	if projectSlug == "" || projectID == "" {
		return nil
	}
	bySlug, err := svc.ProjectInfo(projectSlug)
	if err != nil {
		return err
	}
	byID, err := svc.ProjectInfo(projectID)
	if err != nil {
		return err
	}
	if bySlug.ID != byID.ID {
		return app.RuntimeError{Code: "project_mismatch", Message: "project and project_id do not match"}
	}
	return nil
}

func taskLinkToJSON(link task.TaskLinkInfo) linkJSON {
	return linkJSON{
		ID:        link.ID,
		Type:      link.Type,
		URL:       link.URL,
		Title:     link.Title,
		CreatedAt: time.Unix(link.CreatedAt, 0).UTC().Format(time.RFC3339),
		CreatedBy: task.ActorInfoToJSON(link.CreatedBy),
	}
}

func taskLinksToJSON(links []task.TaskLinkInfo) []linkJSON {
	out := make([]linkJSON, len(links))
	for i, link := range links {
		out[i] = taskLinkToJSON(link)
	}
	return out
}

// stringPtrIfPresent 去空白后，非空则返回指针，否则 nil。
func stringPtrIfPresent(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
