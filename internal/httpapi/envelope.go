package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dajee/taskg/internal/app"
)

type successEnvelope struct {
	Data any            `json:"data"`
	Meta map[string]any `json:"meta,omitempty"`
}

type errorEnvelope struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func writeSuccess(w http.ResponseWriter, status int, data any, meta map[string]any) {
	writeJSON(w, status, successEnvelope{Data: data, Meta: meta})
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, errorEnvelope{
		Error: errorPayload{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAppError(w http.ResponseWriter, err error) {
	var runtimeErr app.RuntimeError
	if errors.As(err, &runtimeErr) {
		status := http.StatusBadRequest
		switch runtimeErr.Code {
		case "auth_missing_token", "auth_invalid_token", "auth_token_expired", "auth_token_revoked":
			status = http.StatusUnauthorized
		case "token_scope_denied", "workspace_scope_denied", "project_scope_denied", "membership_not_found":
			status = http.StatusForbidden
		case "workspace_not_found", "project_not_found", "task_not_found", "context_not_found":
			status = http.StatusNotFound
		case "route_not_found":
			status = http.StatusNotFound
		case "method_not_allowed":
			status = http.StatusMethodNotAllowed
		}
		writeError(w, status, runtimeErr.Code, runtimeErr.Message, nil)
		return
	}
	var permissionErr app.PermissionError
	if errors.As(err, &permissionErr) {
		writeError(w, http.StatusForbidden, permissionErr.Code, permissionErr.Message, nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
}
