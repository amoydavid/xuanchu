package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

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

// writeAppError 把 app 层错误映射为 HTTP 错误响应。落到 catch-all 分支的
// 未知错误意味着服务端 bug（或环境故障），必须留档：对外仍只返回通用
// api_internal，对内记录完整 error 与调用栈，否则线上/CI 只见 500 无从排查。
func (s *Server) writeAppError(w http.ResponseWriter, err error) {
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
	s.logUnhandledAppError(err)
	writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
}

func (s *Server) logUnhandledAppError(err error) {
	msg := fmt.Sprintf("unhandled app error: %v", err)
	stack := string(debug.Stack())
	if s.logger != nil {
		s.logger.Error(msg, "stack", stack)
	} else if s.stderr != nil {
		fmt.Fprintf(s.stderr, "%s\n%s", msg, stack)
	}
}
