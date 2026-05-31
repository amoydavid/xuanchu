package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/google/uuid"
)

type requestContextKey string

const authContextKey requestContextKey = "httpapi.auth"
const logStateContextKey requestContextKey = "httpapi.log_state"

type requestAuth struct {
	Authn              app.AuthenticatedToken
	VisibleWorkspaces  []sqlite.WorkspaceWithRole
	EffectiveWorkspace sqlite.Workspace
}

type requestLogState struct {
	actorID string
	tokenID string
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
				fmt.Fprintf(s.stderr, "panic: %v\n%s", rec, debug.Stack())
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

		fmt.Fprintf(s.stderr, "%s %s %d actor_user_id=%s token_id=%s duration=%s\n",
			r.Method,
			r.URL.Path,
			recorder.status,
			state.actorID,
			state.tokenID,
			time.Since(start).Truncate(time.Millisecond),
		)
	})
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

		svc, err := app.NewService(app.ServiceOptions{Store: s.store, Clock: s.effectiveClock()})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
			return
		}
		authn, err := svc.AuthenticateBearerToken(raw)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		visible, effective, err := s.visibleAndEffectiveWorkspaces(authn)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		_ = sqlite.NewTokenRepository(s.store.DB()).TouchLastUsed(authn.Token.ID, s.effectiveClock().Unix())
		if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok {
			state.actorID = authn.User.ID
			state.tokenID = authn.Token.ID
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

func writeAuthError(w http.ResponseWriter, err error) {
	var runtimeErr app.RuntimeError
	if errors.As(err, &runtimeErr) {
		switch runtimeErr.Code {
		case "auth_invalid_token", "auth_token_expired", "auth_token_revoked":
			writeError(w, http.StatusUnauthorized, runtimeErr.Code, runtimeErr.Message, nil)
			return
		}
	}
	writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
}

func authFromContext(ctx context.Context) (requestAuth, bool) {
	value, ok := ctx.Value(authContextKey).(requestAuth)
	return value, ok
}

func (s *Server) visibleAndEffectiveWorkspaces(authn app.AuthenticatedToken) ([]sqlite.WorkspaceWithRole, sqlite.Workspace, error) {
	rows, err := sqlite.NewWorkspaceRepository(s.store.DB()).ListVisibleForUser(authn.User.ID, false)
	if err != nil {
		return nil, sqlite.Workspace{}, err
	}
	filtered := filterVisibleWorkspaces(rows, authn.Token.WorkspaceIDs)
	if len(filtered) == 0 {
		return nil, sqlite.Workspace{}, app.RuntimeError{Code: "workspace_scope_denied", Message: "workspace scope denied"}
	}
	return filtered, chooseEffectiveWorkspace(authn.User, filtered), nil
}

func filterVisibleWorkspaces(rows []sqlite.WorkspaceWithRole, allowedIDs []string) []sqlite.WorkspaceWithRole {
	if len(allowedIDs) == 0 {
		return rows
	}
	allowed := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = struct{}{}
	}
	out := make([]sqlite.WorkspaceWithRole, 0, len(rows))
	for _, row := range rows {
		if _, ok := allowed[row.Workspace.ID]; ok {
			out = append(out, row)
		}
	}
	return out
}

func chooseEffectiveWorkspace(user sqlite.User, rows []sqlite.WorkspaceWithRole) sqlite.Workspace {
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
