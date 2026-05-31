package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/task"
)

type addTaskRequest struct {
	Description string   `json:"description"`
	Project     string   `json:"project,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, requestProjectRef(r))
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
