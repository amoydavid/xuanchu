package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dajee/taskg/internal/app"
)

type addProjectRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type projectResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	TaskCount   int    `json:"task_count"`
	CreatedAt   int64  `json:"created_at"`
	ModifiedAt  int64  `json:"modified_at"`
	ArchivedAt  *int64 `json:"archived_at,omitempty"`
}

func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "project:read", app.PermissionProjectRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListProjects(r.URL.Query().Get("all") == "true")
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]projectResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectResponseFromView(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleProjectAdd(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "project:write", app.PermissionProjectManage, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req addProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	created, err := scoped.AddProject(app.AddProjectInput{
		Slug:        strings.TrimSpace(req.Slug),
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, projectResponseFromView(created), nil)
}

func (s *Server) handleProjectInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "project:read", app.PermissionProjectRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	project, err := scoped.ProjectInfo(chi.URLParam(r, "projectRef"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectResponseFromView(project), nil)
}

func projectResponseFromView(view app.ProjectView) projectResponse {
	return projectResponse{
		ID:          view.ID,
		WorkspaceID: view.WorkspaceID,
		Slug:        view.Slug,
		Name:        view.Name,
		Description: view.Description,
		Status:      view.Status,
		TaskCount:   view.TaskCount,
		CreatedAt:   view.CreatedAt,
		ModifiedAt:  view.ModifiedAt,
		ArchivedAt:  view.ArchivedAt,
	}
}
