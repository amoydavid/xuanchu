package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
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
		writeError(w, statusForAppErrorCode(runtimeErr.Code), runtimeErr.Code, runtimeErr.Message, nil)
		return
	}
	var permissionErr app.PermissionError
	if errors.As(err, &permissionErr) {
		// PermissionError 的 Code 当前统一为 authz.CodePermissionDenied（→ 403）。
		// 若未来引入其他 code，需确认 statusForAppErrorCode 的映射仍合理。
		writeError(w, statusForAppErrorCode(permissionErr.Code), permissionErr.Code, permissionErr.Message, nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
}
