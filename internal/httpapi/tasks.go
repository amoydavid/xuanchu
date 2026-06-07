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

type addTaskRequest struct {
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

type modifyTaskRequest struct {
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
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	URL       string            `json:"url"`
	Title     string            `json:"title,omitempty"`
	CreatedAt string            `json:"created_at"`
	CreatedBy task.JSONUserInfo `json:"created_by"`
}

const (
	taskListDefaultLimit = 200
	taskListMaxLimit     = 1000
)

func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	limit := taskListDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
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
	input := app.ListInput{Sort: r.URL.Query().Get("sort"), Limit: limit}
	if isTruthyQueryValue(r.URL.Query().Get("no_context")) {
		input.NoContext = true
	}
	if target := strings.TrimSpace(r.URL.Query().Get("target")); target != "" {
		tsk, err := scoped.ResolveProtocolTarget(target)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.Target = &tsk.UUID
	}
	filters := r.URL.Query()["query"]
	if len(filters) == 0 {
		filters = r.URL.Query()["filter"]
	}
	if len(filters) > 0 {
		expr, err := query.ParseFilterExpr(filters)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.Query = expr
	}
	if projectRef != "" {
		project, err := scoped.ProjectInfo(projectRef)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.Query = query.And(input.Query, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(project.ID)})
	}
	reportName := strings.TrimSpace(r.URL.Query().Get("report"))
	var tasks []task.Task
	if reportName != "" {
		tasks, err = scoped.ListReport(reportName, input)
	} else {
		tasks, err = scoped.List(input)
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tasksToJSON(tasks), nil)
}

func isTruthyQueryValue(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value == "true" || value == "1" || value == "yes"
}

func (s *Server) handleTaskAdd(w http.ResponseWriter, r *http.Request) {
	var req addTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
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
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, projectRef)
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
	created, err := scoped.Add(app.AddInput{
		Description: strings.TrimSpace(req.Description),
		Project:     projectPtr,
		Priority:    priority,
		Due:         req.Due,
		Assignees:   req.Assignees,
		Depends:     req.Depends,
		Wait:        req.Wait,
		Scheduled:   req.Scheduled,
		Until:       req.Until,
		Recur:       req.Recur,
		Tags:        req.Tags,
		UDAs:        req.UDAs,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, task.ToJSON(created), nil)
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
		Capability:   "task:write",
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
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	tsk, err := scoped.ResolveProtocolTarget(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, task.ToJSON(tsk), nil)
}

func (s *Server) handleTaskModify(w http.ResponseWriter, r *http.Request) {
	var req modifyTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	projectRef := requestProjectRef(r)
	if projectRef == "" && req.ProjectID != nil {
		projectRef = strings.TrimSpace(*req.ProjectID)
	}
	if projectRef == "" && req.Project != nil {
		projectRef = strings.TrimSpace(*req.Project)
	}
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	resolved, err := scoped.ResolveProtocolTargetForWrite(taskRef)
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
	if err := scoped.Modify(resolved.UUID, app.ModifyInput{
		Description:     req.Description,
		Project:         project,
		ClearProject:    req.ClearProject,
		Priority:        req.Priority,
		ClearPriority:   req.ClearPriority,
		Due:             req.Due,
		ClearDue:        req.ClearDue,
		Wait:            req.Wait,
		ClearWait:       req.ClearWait,
		Scheduled:       req.Scheduled,
		ClearScheduled:  req.ClearScheduled,
		Until:           req.Until,
		ClearUntil:      req.ClearUntil,
		AddAssignees:    req.Assignees,
		RemoveAssignees: req.RemoveAssignees,
		ClearAssignees:  req.ClearAssignees,
		AddDepends:      req.Depends,
		ClearDepends:    req.ClearDepends,
		Recur:           req.Recur,
		ClearRecur:      req.ClearRecur,
		AddTags:         req.Tags,
		RemoveTags:      req.RemoveTags,
		UDAs:            req.UDAs,
		ClearUDAs:       req.ClearUDAs,
	}); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, resolved.UUID)
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
	index, err := strconv.Atoi(chi.URLParam(r, "index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "annotation_index_invalid", "annotation index is invalid", nil)
		return
	}
	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Denotate(id, index) })
}

func (s *Server) handleTaskUrgency(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	resolved, err := scoped.ResolveProtocolTarget(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	result, err := scoped.ExplainUrgency(resolved.UUID)
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
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	resolved, err := scoped.ResolveProtocolTargetForWrite(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	link, err := scoped.TaskAddLink(resolved.UUID, req.Type, req.URL, req.Title)
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
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	tsk, err := scoped.ResolveProtocolTarget(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskLinksToJSON(tsk.Links), nil)
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
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	resolved, err := scoped.ResolveProtocolTargetForWrite(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.TaskRemoveLink(resolved.UUID, linkID); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, resolved.UUID)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.ListInput{}
	filters := r.URL.Query()["query"]
	if len(filters) == 0 {
		filters = r.URL.Query()["filter"]
	}
	if len(filters) > 0 {
		expr, err := query.ParseFilterExpr(filters)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.Query = expr
	}
	if projectRef != "" {
		project, err := scoped.ProjectInfo(projectRef)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.Query = query.And(input.Query, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(project.ID)})
	}
	tasks, err := scoped.ListReport(chi.URLParam(r, "name"), input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tasksToJSON(tasks), nil)
}

func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request, fn func(*app.Service, string) error) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	resolved, err := scoped.ResolveProtocolTargetForWrite(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := fn(scoped, resolved.UUID); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, resolved.UUID)
}

func writeTaskAfterMutation(w http.ResponseWriter, svc *app.Service, taskRef string) {
	tsk, err := svc.Info(taskRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, task.ToJSON(tsk), nil)
}

func requireTaskRef(w http.ResponseWriter, r *http.Request) (string, bool) {
	taskRef := strings.TrimSpace(chi.URLParam(r, "taskRef"))
	if taskRef == "" {
		writeError(w, http.StatusBadRequest, "task_ref_invalid", "task reference is required", nil)
		return "", false
	}
	return taskRef, true
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
		CreatedBy: task.UserInfoToJSON(link.CreatedBy),
	}
}

func taskLinksToJSON(links []task.TaskLinkInfo) []linkJSON {
	out := make([]linkJSON, len(links))
	for i, link := range links {
		out[i] = taskLinkToJSON(link)
	}
	return out
}
