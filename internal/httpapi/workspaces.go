package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type workspaceRequest struct {
	Slug        string  `json:"slug,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Visibility  *string `json:"visibility,omitempty"`
}

type memberRequest struct {
	User string `json:"user"`
	Role string `json:"role"`
}

type workspaceResponse struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Visibility  string             `json:"visibility"`
	CreatedBy   *task.JSONUserInfo `json:"created_by,omitempty"`
	ArchivedAt  *int64             `json:"archived_at,omitempty"`
	Role        string             `json:"role"`
	Active      bool               `json:"active"`
	CreatedAt   int64              `json:"created_at"`
	ModifiedAt  int64              `json:"modified_at"`
}

type memberResponse struct {
	UserID     string  `json:"user_id"`
	Name       string  `json:"name"`
	Email      *string `json:"email,omitempty"`
	Role       string  `json:"role"`
	JoinedAt   int64   `json:"joined_at"`
	ModifiedAt int64   `json:"modified_at"`
}

func (s *Server) handleWorkspaceList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "workspace:read", app.PermissionWorkspaceRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListWorkspaces(r.URL.Query().Get("all") == "true")
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, workspaceResponsesFromViews(rows), nil)
}

func (s *Server) handleWorkspaceInfo(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "workspace")
	scoped, _, err := s.scopedServiceWithWorkspace(r, "workspace:read", app.PermissionWorkspaceRead, ref, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.WorkspaceInfo(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, workspaceResponseFromView(view), nil)
}

func (s *Server) handleWorkspaceAdd(w http.ResponseWriter, r *http.Request) {
	var req workspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "workspace:write", app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.AddWorkspaceInput{Slug: req.Slug}
	if req.Name != nil {
		input.Name = *req.Name
	}
	if req.Description != nil {
		input.Description = *req.Description
	}
	if req.Visibility != nil {
		input.Visibility = *req.Visibility
	}
	view, err := scoped.AddWorkspace(input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, workspaceResponseFromView(view), nil)
}

func (s *Server) handleWorkspaceModify(w http.ResponseWriter, r *http.Request) {
	var req workspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	ref := chi.URLParam(r, "workspace")
	scoped, _, err := s.scopedServiceWithWorkspace(r, "workspace:write", app.PermissionWorkspaceModify, ref, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ModifyWorkspace(ref, app.ModifyWorkspaceInput{
		Name:        req.Name,
		Description: req.Description,
		Visibility:  req.Visibility,
	}); err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.WorkspaceInfo(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, workspaceResponseFromView(view), nil)
}

func (s *Server) handleWorkspaceArchive(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "workspace")
	scoped, _, err := s.scopedServiceWithWorkspace(r, "workspace:write", app.PermissionWorkspaceArchive, ref, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ArchiveWorkspace(ref); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleMemberList(w http.ResponseWriter, r *http.Request) {
	workspace := chi.URLParam(r, "workspace")
	scoped, _, err := s.scopedServiceWithWorkspace(r, "workspace:read", app.PermissionWorkspaceRead, workspace, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListMembers(workspace)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, memberResponsesFromViews(rows), nil)
}

func (s *Server) handleMemberAdd(w http.ResponseWriter, r *http.Request) {
	var req memberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	workspace := chi.URLParam(r, "workspace")
	scoped, _, err := s.scopedServiceWithWorkspace(r, "workspace:write", app.PermissionMemberManage, workspace, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.AddMember(app.AddMemberInput{WorkspaceRef: workspace, UserRef: req.User, Role: app.Role(req.Role)}); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleMemberRole(w http.ResponseWriter, r *http.Request) {
	var req memberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	workspace := chi.URLParam(r, "workspace")
	user := chi.URLParam(r, "user")
	scoped, _, err := s.scopedServiceWithWorkspace(r, "workspace:write", app.PermissionMemberManage, workspace, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ChangeMemberRole(app.ChangeMemberRoleInput{WorkspaceRef: workspace, UserRef: user, Role: app.Role(req.Role)}); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func workspaceResponsesFromViews(rows []app.WorkspaceView) []workspaceResponse {
	out := make([]workspaceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspaceResponseFromView(row))
	}
	return out
}

func workspaceResponseFromView(row app.WorkspaceView) workspaceResponse {
	role := string(row.Role)
	if role == "" {
		role = string(app.RoleViewer)
	}
	var createdBy *task.JSONUserInfo
	if row.CreatedBy != nil {
		jui := task.UserInfoToJSON(*row.CreatedBy)
		createdBy = &jui
	}
	return workspaceResponse{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Visibility:  row.Visibility,
		CreatedBy:   createdBy,
		ArchivedAt:  row.ArchivedAt,
		Role:        role,
		Active:      row.Active,
		CreatedAt:   row.CreatedAt,
		ModifiedAt:  row.ModifiedAt,
	}
}

func memberResponsesFromViews(rows []app.MemberView) []memberResponse {
	out := make([]memberResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, memberResponseFromView(row))
	}
	return out
}

func memberResponseFromView(row app.MemberView) memberResponse {
	return memberResponse{
		UserID:     row.UserID,
		Name:       row.Name,
		Email:      row.Email,
		Role:       string(row.Role),
		JoinedAt:   row.JoinedAt,
		ModifiedAt: row.ModifiedAt,
	}
}
