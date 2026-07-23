package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type workspaceUDARequest struct {
	Type    string   `json:"type"`
	Label   string   `json:"label"`
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

func (s *Server) handleWorkspaceUDAList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionUDARead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.WorkspaceListUDAs()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleWorkspaceUDASet(w http.ResponseWriter, r *http.Request) {
	var req workspaceUDARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeConfigWrite, app.PermissionUDAManage, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	row, err := scoped.WorkspaceSetUDA(chi.URLParam(r, "name"), app.WorkspaceUDAInput{
		Type: req.Type, Label: req.Label, Values: req.Values, Default: req.Default,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, row, nil)
}

func (s *Server) handleWorkspaceUDADelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigWrite, app.PermissionUDAManage, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.WorkspaceDeleteUDA(chi.URLParam(r, "name")); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}
