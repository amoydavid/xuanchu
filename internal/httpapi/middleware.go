package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
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
	actorType          string
	actorID            string
	tokenID            string
	tokenName          string
	tokenPrefix        string
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
		state := &requestLogState{actorType: "user", actorID: "-", tokenID: "-"}
		next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), logStateContextKey, state)))

		extra := ""
		if state.actorType == "tenant_access_token" {
			extra = fmt.Sprintf(" actor_type=%s token_name=%s token_prefix=%s", state.actorType, state.tokenName, state.tokenPrefix)
		}
		if state.delegatorUserID != "" {
			extra += fmt.Sprintf(" delegator_user_id=%s delegator_token_id=%s", state.delegatorUserID, state.delegatorTokenID)
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
				"actor_type", state.actorType,
				"actor_user_id", state.actorID,
				"token_id", state.tokenID,
				"duration_ms", duration.Milliseconds(),
			}
			if state.actorType == "tenant_access_token" {
				args = append(args,
					"token_name", state.tokenName,
					"token_prefix", state.tokenPrefix,
				)
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
		fmt.Fprintf(s.stderr, "%s %s %d actor_type=%s actor_user_id=%s token_id=%s%s duration=%s\n",
			r.Method,
			r.URL.Path,
			recorder.status,
			state.actorType,
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
		limit := s.bodyLimitBytes
		// 附件上传走 multipart，需要更大的 limit。
		if s.attachmentUploadLimit() > 0 && strings.HasSuffix(r.URL.Path, "/attachments") && r.Method == http.MethodPost {
			limit = s.attachmentUploadLimit()
		}
		if r.ContentLength > limit {
			writeError(w, http.StatusRequestEntityTooLarge, "api_payload_too_large", "request payload too large", nil)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

// attachmentUploadLimit 返回附件上传路由应使用的 body limit。
//
// 至少要能容纳 MaxFileSizeBytes + multipart 开销；nil runtime 时返回 0（走全局上限）。
func (s *Server) attachmentUploadLimit() int64 {
	if s.attachments == nil || s.attachments.Config.MaxFileSizeBytes <= 0 {
		return 0
	}
	return s.attachments.Config.MaxFileSizeBytes + (1 << 20)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, hasBearer := bearerToken(r.Header.Get("Authorization"))

		if hasBearer {
			s.handleBearerAuth(w, r, raw, next)
			return
		}

		// 无 Bearer：cookie 只允许普通 /api/v1/* Web Console API，不允许 MCP/admin。
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
			writeError(w, http.StatusUnauthorized, authz.CodeAuthMissingToken, "missing bearer token", nil)
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			s.handleCookieAuth(w, r, cookie.Value, next)
			return
		}

		writeError(w, http.StatusUnauthorized, authz.CodeAuthMissingToken, "missing bearer token or session", nil)
	})
}

// handleBearerAuth 是原有 token 认证逻辑（从 authMiddleware 抽出，保持不变）。
func (s *Server) handleBearerAuth(w http.ResponseWriter, r *http.Request, raw string, next http.Handler) {
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
		if authn.TenantActor {
			state.actorType = "tenant_access_token"
			state.actorID = "-"
			state.tokenName = authn.Token.Name
			state.tokenPrefix = authn.Token.Prefix
		} else {
			state.actorType = "user"
			state.actorID = authn.User.ID
		}
		state.tokenID = authn.Token.ID
		state.workspaceID = effective.ID
		state.workspaceRef = effective.Slug
	}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, requestAuth{
		Authn:              authn,
		VisibleWorkspaces:  visible,
		EffectiveWorkspace: effective,
	})))
}

// handleCookieAuth 用 browser session 构造认证上下文。
// browser session 的授权由 membership role 决定（非 token scope），
// 因此 TokenView.Scopes 设为全集（不含 impersonate）、WorkspaceIDs 锁定 session 的 workspace。
func (s *Server) handleCookieAuth(w http.ResponseWriter, r *http.Request, rawCookie string, next http.Handler) {
	session, err := s.oidcAuthService().ResolveSession(rawCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session 无效或已过期", nil)
		return
	}
	// 写操作必须校验 CSRF（double-submit cookie + 头匹配 + 服务端 hash）
	if isApiWriteMethod(r.Method) && !s.validCSRF(r, session.CSRFHash) {
		writeError(w, http.StatusForbidden, "csrf_invalid", "页面会话已过期，请刷新后重试", nil)
		return
	}
	userRepo := storage.NewUserRepository(s.store.DB())
	user, err := userRepo.GetByID(session.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session 用户不存在", nil)
		return
	}
	workspace, err := storage.NewWorkspaceRepository(s.store.DB()).GetByID(session.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session workspace 不存在", nil)
		return
	}
	memberRepo := storage.NewMemberRepository(s.store.DB())
	membership, err := memberRepo.Get(user.ID, workspace.ID)
	if err != nil {
		writeError(w, http.StatusForbidden, "membership_inactive", "您不是该工作区的成员", nil)
		return
	}
	// 构造 AuthenticatedToken：browser session 凭证，scopes 全开放（授权由 role 决定），
	// workspace 锁定 session 的 workspace。
	authn := app.AuthenticatedToken{
		Token: app.TokenView{
			ID:           "browser_session:" + session.ID,
			Name:         "Browser Session",
			Type:         browserSessionTokenType,
			User:         task.UserInfo{ID: user.ID, Name: user.Name},
			WorkspaceIDs: []string{workspace.ID},
			Scopes:       browserSessionScopes(),
		},
		User: user,
	}
	if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok {
		state.actorType = browserSessionTokenType
		state.actorID = user.ID
		state.tokenID = authn.Token.ID
		state.workspaceID = workspace.ID
		state.workspaceRef = workspace.Slug
	}
	visible := []storage.WorkspaceWithRole{{Workspace: workspace, Role: membership.Role}}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, requestAuth{
		Authn:              authn,
		VisibleWorkspaces:  visible,
		EffectiveWorkspace: workspace,
	})))
}

// validCSRF 校验 double-submit CSRF：cookie 值 == X-Xuanchu-CSRF 头，且 hash 后等于 session 存储的 CSRFHash。
func (s *Server) validCSRF(r *http.Request, wantHash string) bool {
	return validCSRFRequest(r, wantHash)
}

func isApiWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// browserSessionScopes 返回 browser session 允许的 capability 集合。
// browser session 用于 Web Console 浏览器交互，授权最终由 membership role 决定。
// 这里只放行浏览器交互必需的 scope，刻意排除：
//   - impersonate：防止 SSO 用户冒充他人
//   - sso.config.*（workspace:write）：SSO 配置仅 owner/tenant actor 可改
//   - user:write：成员页创建用户走 member:write 下的受限聚合流程，不开放全局 user 写权限
//
// token:write 对 browser session 放行：owner/admin 需要在 Web Console 创建/修改/吊销
// PAT / Agent / tenant access token。capability 通过后，token 创建/管理的最终授权仍由
// app 层 tokenManageAllowed(role) 收紧——仅 owner/admin 可执行，member/viewer 仍被拒绝。
func browserSessionScopes() []string {
	return []string{
		auth.ScopeTaskRead, auth.ScopeTaskWrite,
		auth.ScopeProjectRead, auth.ScopeProjectWrite,
		auth.ScopeContextRead, auth.ScopeContextWrite,
		auth.ScopeConfigRead, auth.ScopeConfigWrite,
		auth.ScopeWorkspaceRead,
		auth.ScopeAuditRead,
		auth.ScopeUserRead,
		auth.ScopeMemberRead, auth.ScopeMemberWrite,
		auth.ScopeTokenRead, auth.ScopeTokenWrite,
		auth.ScopeHookRead, auth.ScopeHookWrite,
		auth.ScopeNotificationRead, auth.ScopeNotificationWrite,
		auth.ScopeReminderRead, auth.ScopeReminderWrite,
	}
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
	if authn.TenantActor || authn.Token.Type == "tenant_access_token" {
		if len(authn.Token.WorkspaceIDs) != 1 {
			return nil, storage.Workspace{}, app.RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "workspace scope denied"}
		}
		workspace, err := storage.NewWorkspaceRepository(s.store.DB()).GetByID(authn.Token.WorkspaceIDs[0])
		if err != nil {
			return nil, storage.Workspace{}, err
		}
		if workspace.ArchivedAt != nil {
			return nil, storage.Workspace{}, app.RuntimeError{Code: authz.CodeWorkspaceArchived, Message: "workspace is archived"}
		}
		return []storage.WorkspaceWithRole{{Workspace: workspace}}, workspace, nil
	}
	rows, err := storage.NewWorkspaceRepository(s.store.DB()).ListVisibleForUser(authn.User.ID, false)
	if err != nil {
		return nil, storage.Workspace{}, err
	}
	filtered := filterVisibleWorkspaces(rows, authn.Token.WorkspaceIDs)
	if len(filtered) == 0 {
		return nil, storage.Workspace{}, app.RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "workspace scope denied"}
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
