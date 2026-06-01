package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/task"
)

type auditResponse struct {
	ID          int64           `json:"id"`
	ActorUserID *string         `json:"actor_user_id"`
	ActorName   string          `json:"actor_name"`
	WorkspaceID *string         `json:"workspace_id"`
	ProjectID   *string         `json:"project_id"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    string          `json:"target_id"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	CreatedAt   int64           `json:"created_at"`
}

const auditMaxLimit = 1000

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, "task:read", app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var input app.ExportInput
	if projectRef != "" {
		project, err := scoped.ProjectInfo(projectRef)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.ProjectID = &project.ID
	}
	rows, err := scoped.ExportWithInput(input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tasksToJSON(rows), nil)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "task:write", app.PermissionTaskWrite, requestProjectRef(r))
	if err != nil {
		writeAppError(w, err)
		return
	}
	var rows []task.JSONTask
	if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	count, err := scoped.Import(rows)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]int{"imported": count}, nil)
}

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "invalid limit", nil)
			return
		}
		if parsed > auditMaxLimit {
			writeError(w, http.StatusBadRequest, "api_bad_limit", fmt.Sprintf("limit must be <= %d", auditMaxLimit), nil)
			return
		}
		limit = parsed
	}
	scoped, _, err := s.scopedService(r, "audit:read", app.PermissionAuditRead, requestProjectRef(r))
	if err != nil {
		writeAppError(w, err)
		return
	}
	projectRef := r.URL.Query().Get("project")
	if projectRef == "" {
		projectRef = r.URL.Query().Get("project_id")
	}
	rows, err := scoped.ListAudit(app.AuditListInput{ProjectRef: projectRef, Limit: limit})
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]auditResponse, 0, len(rows))
	for _, row := range rows {
		item := auditResponse{
			ID:          row.ID,
			ActorUserID: row.ActorUserID,
			ActorName:   row.ActorName,
			WorkspaceID: row.WorkspaceID,
			ProjectID:   row.ProjectID,
			Action:      row.Action,
			TargetType:  row.TargetType,
			TargetID:    row.TargetID,
			CreatedAt:   row.CreatedAt,
		}
		if row.PayloadJSON != "" {
			item.Payload = json.RawMessage(row.PayloadJSON)
		}
		out = append(out, item)
	}
	writeSuccess(w, http.StatusOK, out, nil)
}
