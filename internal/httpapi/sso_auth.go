package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/go-chi/chi/v5"
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
	cfgSvc := app.NewOIDCConfigService(storage.NewConfigRepository(s.store.DB()))
	sessionRepo := storage.NewSessionRepository(s.store.DB())
	s.oidcAuth = app.NewOIDCAuthService(s.store, sessionRepo, cfgSvc, nil)
	return s.oidcAuth
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
		writeError(w, http.StatusNotFound, "workspace_not_found", "workspace 不存在", nil)
		return
	}
	authURL, err := s.oidcAuthService().Start(r.Context(), wsID)
	if err != nil {
		redirectToSsoError(w, r, "sso_start_failed")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleSsoOidcCallback 处理 IdP 回调，成功后 set session+csrf cookie 并回到 console。
func (s *Server) handleSsoOidcCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		redirectToSsoError(w, r, "invalid_callback")
		return
	}
	login, err := s.oidcAuthService().Callback(r.Context(), state, code)
	if err != nil {
		redirectToSsoError(w, r, appErrorToSsoCode(err))
		return
	}
	setBrowserSessionCookies(w, login.RawSession, login.CSRFToken)
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

// handleAuthLogout 清除 browser session 与 cookie。
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_ = s.oidcAuthService().Logout(hashHexLocal(cookie.Value))
	}
	clearBrowserSessionCookies(w)
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

func setBrowserSessionCookies(w http.ResponseWriter, rawSession, csrfToken string) {
	// TODO: insecure_cookie 应从 workspace sso 配置读取；暂时默认 secure。
	secure := true
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: rawSession,
		Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 3600,
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: csrfToken,
		Path: "/", HttpOnly: false, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 3600,
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
	msg := err.Error()
	switch {
	case strings.Contains(msg, "identity_not_found"):
		return "identity_not_found"
	case strings.Contains(msg, "membership_inactive"):
		return "membership_inactive"
	case strings.Contains(msg, "invalid_state"):
		return "invalid_state"
	case strings.Contains(msg, "id_token_invalid"):
		return "id_token_invalid"
	default:
		return "sso_failed"
	}
}

// hashHexLocal 计算 SHA256 hex，用于把 cookie 里的明文 session token 转成存储 hash。
func hashHexLocal(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ensure chi import used（chi.URLParam 在 sso.go 使用）
var _ = chi.URLParam