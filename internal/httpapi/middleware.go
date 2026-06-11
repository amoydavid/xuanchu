package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

type requestContextKey string

const authContextKey requestContextKey = "httpapi.auth"
const logStateContextKey requestContextKey = "httpapi.log_state"

type requestAuth struct {
	Authn              app.AuthenticatedToken
	VisibleWorkspaces  []storage.WorkspaceWithRole
	EffectiveWorkspace storage.Workspace
}

type requestLogState struct {
	actorID            string
	tokenID            string
	workspaceID        string
	workspaceRef       string
	delegatorUserID    string
	delegatorTokenID   string
	impersonateAttempt string
}

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestContextKey("request_id"), requestID)))
	})
}

func (s *Server) recovererMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if s.logger != nil {
					s.logger.Error("panic recovered", "panic", rec, "stack", string(debug.Stack()), "path", r.URL.Path)
				} else {
					fmt.Fprintf(s.stderr, "panic: %v\n%s", rec, debug.Stack())
				}
				writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		state := &requestLogState{actorID: "-", tokenID: "-"}
		next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), logStateContextKey, state)))

		extra := ""
		if state.delegatorUserID != "" {
			extra = fmt.Sprintf(" delegator_user_id=%s delegator_token_id=%s", state.delegatorUserID, state.delegatorTokenID)
		}
		if state.impersonateAttempt != "" {
			extra += fmt.Sprintf(" impersonate_attempt=%s", state.impersonateAttempt)
		}
		duration := time.Since(start)
		if s.logger != nil {
			args := []any{
				"component", "http",
				"operation", "http_request",
				"request_id", requestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"actor_user_id", state.actorID,
				"token_id", state.tokenID,
				"duration_ms", duration.Milliseconds(),
			}
			if state.workspaceID != "" {
				args = append(args,
					"workspace_id", state.workspaceID,
					"workspace_ref", state.workspaceRef,
				)
			}
			if state.delegatorUserID != "" {
				args = append(args,
					"delegator_user_id", state.delegatorUserID,
					"delegator_token_id", state.delegatorTokenID,
				)
			}
			if state.impersonateAttempt != "" {
				args = append(args, "impersonate_attempt", state.impersonateAttempt)
			}
			s.logger.Info("http request", args...)
			return
		}
		fmt.Fprintf(s.stderr, "%s %s %d actor_user_id=%s token_id=%s%s duration=%s\n",
			r.Method,
			r.URL.Path,
			recorder.status,
			state.actorID,
			state.tokenID,
			extra,
			duration.Truncate(time.Millisecond),
		)
	})
}

func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestContextKey("request_id")).(string); ok && v != "" {
		return v
	}
	return "-"
}

func (s *Server) bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > s.bodyLimitBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "api_payload_too_large", "request payload too large", nil)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, s.bodyLimitBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "auth_missing_token", "missing bearer token", nil)
			return
		}

		svc, err := app.NewService(app.ServiceOptions{
			Store:                 s.store,
			Clock:                 s.effectiveClock(),
			Runtime:               &app.RuntimeContext{},
			DisableScopeBootstrap: true,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
			return
		}
		authn, err := svc.AuthenticateBearerToken(raw)
		if err != nil {
			writeAppError(w, err)
			return
		}
		visible, effective, err := s.visibleAndEffectiveWorkspaces(authn)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok {
			state.actorID = authn.User.ID
			state.tokenID = authn.Token.ID
			state.workspaceID = effective.ID
			state.workspaceRef = effective.Slug
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, requestAuth{
			Authn:              authn,
			VisibleWorkspaces:  visible,
			EffectiveWorkspace: effective,
		})))
	})
}

func (s *Server) effectiveClock() app.Clock {
	if s.clock != nil {
		return s.clock
	}
	return systemClock{}
}

func bearerToken(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	if parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func authFromContext(ctx context.Context) (requestAuth, bool) {
	value, ok := ctx.Value(authContextKey).(requestAuth)
	return value, ok
}

func (s *Server) visibleAndEffectiveWorkspaces(authn app.AuthenticatedToken) ([]storage.WorkspaceWithRole, storage.Workspace, error) {
	rows, err := storage.NewWorkspaceRepository(s.store.DB()).ListVisibleForUser(authn.User.ID, false)
	if err != nil {
		return nil, storage.Workspace{}, err
	}
	filtered := filterVisibleWorkspaces(rows, authn.Token.WorkspaceIDs)
	if len(filtered) == 0 {
		return nil, storage.Workspace{}, app.RuntimeError{Code: "workspace_scope_denied", Message: "workspace scope denied"}
	}
	return filtered, chooseEffectiveWorkspace(authn.User, filtered), nil
}

func filterVisibleWorkspaces(rows []storage.WorkspaceWithRole, allowedIDs []string) []storage.WorkspaceWithRole {
	if len(allowedIDs) == 0 {
		return rows
	}
	allowed := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = struct{}{}
	}
	out := make([]storage.WorkspaceWithRole, 0, len(rows))
	for _, row := range rows {
		if _, ok := allowed[row.Workspace.ID]; ok {
			out = append(out, row)
		}
	}
	return out
}

func chooseEffectiveWorkspace(user storage.User, rows []storage.WorkspaceWithRole) storage.Workspace {
	if user.DefaultWorkspaceID != nil {
		for _, row := range rows {
			if row.Workspace.ID == *user.DefaultWorkspaceID {
				return row.Workspace
			}
		}
	}
	return rows[0].Workspace
}

type systemClock struct{}

func (systemClock) Unix() int64 { return time.Now().Unix() }
func (systemClock) Location() *time.Location {
	return time.Local
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	return r.ResponseWriter.Write(data)
}
