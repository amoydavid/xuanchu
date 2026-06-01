package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dajee/taskg/internal/app"
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
	writeSuccess(w, http.StatusOK, rows, nil)
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
	writeSuccess(w, http.StatusOK, view, nil)
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
	writeSuccess(w, http.StatusCreated, view, nil)
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
	writeSuccess(w, http.StatusOK, view, nil)
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
	writeSuccess(w, http.StatusOK, rows, nil)
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
