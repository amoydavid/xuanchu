package httpapi

import (
	"encoding/json"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

type activeWorkspaceRequest struct {
	Workspace string `json:"workspace"`
}

func (s *Server) handleMeActiveWorkspace(w http.ResponseWriter, r *http.Request) {
	var req activeWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "workspace:write", app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.UseWorkspace(req.Workspace); err != nil {
		writeAppError(w, err)
		return
	}
	scoped2, _, err := s.scopedServiceWithWorkspace(r, "workspace:read", app.PermissionWorkspaceRead, req.Workspace, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped2.WorkspaceInfo(req.Workspace)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, workspaceResponseFromView(view), nil)
}
