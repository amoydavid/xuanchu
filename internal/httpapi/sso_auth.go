package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

const (
	sessionCookieName = "xuanchu_session"
	csrfCookieName    = "xuanchu_csrf"
)

// oidcAuthService 懒加载 OIDCAuthService（生产模式，用真实 provider factory）。
func (s *Server) oidcAuthService() *app.OIDCAuthService {
	if s.oidcAuth != nil {
		return s.oidcAuth
	}
	cfgSvc := app.NewOIDCConfigService(storage.NewConfigRepository(s.store.DB()), s.secretKey)
	sessionRepo := storage.NewSessionRepository(s.store.DB())
	s.oidcAuth = app.NewOIDCAuthService(s.store, sessionRepo, cfgSvc, nil)
	return s.oidcAuth
}

// handleSsoWorkspace 查找唯一一个启用了 OIDC 的 workspace。
// 恰好 1 个时返回 {slug, name}；0 个或多个时返回 404。
func (s *Server) handleSsoWorkspace(w http.ResponseWriter, r *http.Request) {
	wsRepo := storage.NewWorkspaceRepository(s.store.DB())
	workspaces, err := wsRepo.ListAll(false)
	if err != nil {
		writeError(w, http.StatusNotFound, "no_sso_workspace", "", nil)
		return
	}
	cfgSvc := app.NewOIDCConfigService(storage.NewConfigRepository(s.store.DB()), s.secretKey)
	var found *storage.Workspace
	for i := range workspaces {
		ws := &workspaces[i]
		if _, enabled := cfgSvc.Get(ws.ID); enabled {
			if found != nil {
				// 多个 OIDC workspace → 404
				writeError(w, http.StatusNotFound, "multiple_sso_workspaces", "", nil)
				return
			}
			found = ws
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "no_sso_workspace", "", nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"slug": found.Slug, "name": found.Name}, nil)
}

// resolveWorkspaceIDByRef 把 slug 或 id 解析成 workspace ID。
func (s *Server) resolveWorkspaceIDByRef(ref string) (string, bool) {
	wsRepo := storage.NewWorkspaceRepository(s.store.DB())
	if ws, err := wsRepo.GetByID(ref); err == nil {
		return ws.ID, true
	}
	if ws, err := wsRepo.GetBySlug(ref); err == nil {
		return ws.ID, true
	}
	return "", false
}

// handleSsoOidcStart 生成 OIDC 授权跳转 URL，302 到 IdP。
func (s *Server) handleSsoOidcStart(w http.ResponseWriter, r *http.Request) {
	workspaceRef := r.URL.Query().Get("workspace")
	if workspaceRef == "" {
		writeError(w, http.StatusBadRequest, "missing_workspace", "workspace 参数必填", nil)
		return
	}
	wsID, ok := s.resolveWorkspaceIDByRef(workspaceRef)
	if !ok {
		redirectToSsoError(w, r, "workspace_not_found")
		return
	}
	authURL, err := s.oidcAuthService().Start(r.Context(), wsID)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("sso start failed", "err", err, "workspace_id", wsID)
		}
		redirectToSsoError(w, r, "sso_start_failed")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleSsoOidcCallback 处理 IdP 回调，成功后 set session+csrf cookie 并回到 console。
func (s *Server) handleSsoOidcCallback(w http.ResponseWriter, r *http.Request) {
	// IdP 可能返回 error 参数（而非 code+state），先检查
	if errCode := r.URL.Query().Get("error"); errCode != "" {
		if s.logger != nil {
			s.logger.Error("sso callback: IdP returned error", "error", errCode, "description", r.URL.Query().Get("error_description"))
		}
		redirectToSsoError(w, r, "idp_error_"+errCode)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if s.logger != nil {
		s.logger.Info("sso callback received", "has_state", state != "", "has_code", code != "", "query", r.URL.RawQuery)
	}
	if state == "" || code == "" {
		redirectToSsoError(w, r, "invalid_callback")
		return
	}
	login, err := s.oidcAuthService().Callback(r.Context(), state, code)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("sso callback failed", "err", err)
		}
		redirectToSsoError(w, r, appErrorToSsoCode(err))
		return
	}
	setBrowserSessionCookies(w, login.RawSession, login.CSRFToken, login.InsecureCookie, login.SessionMaxAge)
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

// handleAuthLogout 清除 browser session 与 cookie。
// logout 需要有效的 session cookie + CSRF（防 CSRF 强制登出）。
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		clearBrowserSessionCookies(w)
		http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
		return
	}
	session, err := s.oidcAuthService().ResolveSession(cookie.Value)
	if err != nil {
		clearBrowserSessionCookies(w)
		http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
		return
	}
	if !validCSRFRequest(r, session.CSRFHash) {
		writeError(w, http.StatusForbidden, "csrf_invalid", "页面会话已过期，请刷新后重试", nil)
		return
	}
	_ = s.oidcAuthService().Logout(hashHexLocal(cookie.Value))
	clearBrowserSessionCookies(w)
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

// validCSRFRequest 校验 double-submit CSRF（logout 等无 authMiddleware 的写端点用）。
func validCSRFRequest(r *http.Request, wantHash string) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	header := r.Header.Get("X-Xuanchu-CSRF")
	if header == "" || header != cookie.Value {
		return false
	}
	return hashHexLocal(header) == wantHash
}

func setBrowserSessionCookies(w http.ResponseWriter, rawSession, csrfToken string, insecureCookie bool, maxAge int) {
	secure := !insecureCookie
	if maxAge <= 0 {
		maxAge = 7 * 24 * 3600
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: rawSession,
		Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: csrfToken,
		Path: "/", HttpOnly: false, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	})
}

func clearBrowserSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", MaxAge: -1})
}

func redirectToSsoError(w http.ResponseWriter, r *http.Request, code string) {
	target := "/?sso_error=" + url.QueryEscape(code)
	http.Redirect(w, r, target, http.StatusFound)
}

func appErrorToSsoCode(err error) string {
	switch {
	case errors.Is(err, app.ErrIdentityNotFound):
		return "identity_not_found"
	case errors.Is(err, app.ErrMembershipInactive):
		return "membership_inactive"
	case errors.Is(err, app.ErrInvalidState):
		return "invalid_state"
	case errors.Is(err, app.ErrIDTokenInvalid):
		return "id_token_invalid"
	case errors.Is(err, app.ErrSsoNotEnabled):
		return "sso_not_enabled"
	default:
		return "sso_failed"
	}
}

// hashHexLocal 计算 SHA256 hex，用于把 cookie 里的明文 session token 转成存储 hash。
func hashHexLocal(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
