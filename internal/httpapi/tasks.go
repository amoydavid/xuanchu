package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/task"
)

type addTaskRequest struct {
	Description string            `json:"description"`
	Project     string            `json:"project,omitempty"`
	ProjectID   string            `json:"project_id,omitempty"`
	Priority    string            `json:"priority,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	UDAs        map[string]string `json:"udas,omitempty"`
}

type modifyTaskRequest struct {
	Description  *string           `json:"description,omitempty"`
	Project      *string           `json:"project,omitempty"`
	ProjectID    *string           `json:"project_id,omitempty"`
	Priority     *string           `json:"priority,omitempty"`
	ClearProject bool              `json:"clear_project,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	UDAs         map[string]string `json:"udas,omitempty"`
	ClearUDAs    []string          `json:"clear_udas,omitempty"`
}

type textRequest struct {
	Text        string `json:"text,omitempty"`
	Description string `json:"description,omitempty"`
}

func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.ListInput{Sort: r.URL.Query().Get("sort")}
	if target := strings.TrimSpace(r.URL.Query().Get("target")); target != "" {
		input.Target = &target
	}
	if filters := r.URL.Query()["filter"]; len(filters) > 0 {
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

func (s *Server) handleTaskAdd(w http.ResponseWriter, r *http.Request) {
	var req addTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
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
		Tags:        req.Tags,
		UDAs:        req.UDAs,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, task.ToJSON(created), nil)
}

func (s *Server) handleTaskInfo(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if _, err := uuid.Parse(taskID); err != nil {
		writeError(w, http.StatusBadRequest, "task_uuid_invalid", "task UUID is invalid", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	tsk, err := scoped.Info(taskID)
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
	taskID, ok := requireTaskUUID(w, r)
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
	if err := scoped.Modify(taskID, app.ModifyInput{
		Description:  req.Description,
		Project:      project,
		ClearProject: req.ClearProject,
		Priority:     req.Priority,
		AddTags:      req.Tags,
		UDAs:         req.UDAs,
		ClearUDAs:    req.ClearUDAs,
	}); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, taskID)
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
	taskID, ok := requireTaskUUID(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	result, err := scoped.ExplainUrgency(taskID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, result, nil)
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
	taskID, ok := requireTaskUUID(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := fn(scoped, taskID); err != nil {
		writeAppError(w, err)
		return
	}
	writeTaskAfterMutation(w, scoped, taskID)
}

func writeTaskAfterMutation(w http.ResponseWriter, svc *app.Service, taskID string) {
	tsk, err := svc.Info(taskID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, task.ToJSON(tsk), nil)
}

func requireTaskUUID(w http.ResponseWriter, r *http.Request) (string, bool) {
	taskID := chi.URLParam(r, "taskID")
	if _, err := uuid.Parse(taskID); err != nil {
		writeError(w, http.StatusBadRequest, "task_uuid_invalid", "task UUID is invalid", nil)
		return "", false
	}
	return taskID, true
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
