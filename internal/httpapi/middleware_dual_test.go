package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// seedBrowserSession 在 store 里直接写一条 browser session，返回 raw cookie 值（明文）与 raw csrf 值。
func seedBrowserSession(t *testing.T, store *storage.Store, userID, workspaceID string) (rawSession, rawCSRF string) {
	t.Helper()
	rawSession = "raw-session-token-for-test"
	rawCSRF = "raw-csrf-token-for-test"
	sessionHash := hashHexLocal(rawSession)
	csrfHash := hashHexLocal(rawCSRF)
	repo := storage.NewSessionRepository(store.DB())
	if err := repo.CreateSession(sessionHash, userID, workspaceID, csrfHash, 1, 9999999999); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return rawSession, rawCSRF
}

func TestCookieGetAllowed(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	store := srv.store
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("local workspace: %v", err)
	}
	user, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("get local user: %v", err)
	}
	rawSession, _ := seedBrowserSession(t, store, user.ID, ws.ID)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawSession})
	srv.Router().ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("cookie GET should not be 401, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCookieWriteWithoutCSRFForbidden(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	store := srv.store
	ws, _ := store.LocalWorkspace()
	user, _ := storage.NewUserRepository(store.DB()).GetByName("local")
	rawSession, _ := seedBrowserSession(t, store, user.ID, ws.ID)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{}`))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawSession})
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF should be 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCookieWriteWithCSRFAllowed(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	store := srv.store
	ws, _ := store.LocalWorkspace()
	user, _ := storage.NewUserRepository(store.DB()).GetByName("local")
	rawSession, rawCSRF := seedBrowserSession(t, store, user.ID, ws.ID)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{}`))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawSession})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: rawCSRF})
	req.Header.Set("X-Xuanchu-CSRF", rawCSRF)
	srv.Router().ServeHTTP(rr, req)
	// 通过 CSRF 后，应进入业务逻辑（可能因 body 无效报 400，但不应是 403 csrf_invalid）
	if rr.Code == http.StatusForbidden {
		t.Fatalf("POST with valid CSRF should not be 403 csrf_invalid, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNoCredentialUnauthorized(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no credential should be 401, got %d", rr.Code)
	}
}

// TestCookieCredentialsCurrentRole 验证 browser session（cookie 认证）访问
// /api/v1/credentials/current 时返回的 effective_role 来自 membership 而非空字符串。
// 回归：handleCookieAuth 构造 VisibleWorkspaces 时漏填 Role，导致前端把整页锁成只读。
func TestCookieCredentialsCurrentRole(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	store := srv.store
	ws, _ := store.LocalWorkspace()
	user, _ := storage.NewUserRepository(store.DB()).GetByName("local")
	rawSession, _ := seedBrowserSession(t, store, user.ID, ws.ID)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credentials/current", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawSession})
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			EffectiveRole string   `json:"effective_role"`
			Capabilities  []string `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.EffectiveRole != "owner" {
		t.Fatalf("cookie session effective_role = %q, want owner (前端会因此把 task 编辑全部禁用)", payload.Data.EffectiveRole)
	}
	if !slices.Contains(payload.Data.Capabilities, "member:write") {
		t.Fatalf("cookie session capabilities = %v, want member:write", payload.Data.Capabilities)
	}
	// token:write 对 browser session 放行：owner/admin 需在 Web Console 创建/管理 token，
	// 最终授权由 app 层 tokenManageAllowed(role) 收紧。impersonate 仍排除。
	if !slices.Contains(payload.Data.Capabilities, "token:write") {
		t.Fatalf("cookie session capabilities = %v, want token:write", payload.Data.Capabilities)
	}
	if slices.Contains(payload.Data.Capabilities, "impersonate") {
		t.Fatalf("cookie session capabilities over-expanded: %v", payload.Data.Capabilities)
	}
}

// TestCookieSessionOwnerCanCreateTenantToken 回归：browser session（SSO 登录）放行 token:write 后，
// owner 可通过 cookie 创建 tenant access token。此前 browserSessionScopes 排除 token:write，
// 导致 owner 在 Web Console 创建 tenant token 报 403 token_scope_denied。
func TestCookieSessionOwnerCanCreateTenantToken(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	store := srv.store
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("local workspace: %v", err)
	}
	user, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("get local user: %v", err)
	}
	rawSession, rawCSRF := seedBrowserSession(t, store, user.ID, ws.ID)

	body := `{"name":"runtime","scopes":["task:read"],"expires_in_seconds":3600}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant-access-tokens", bytes.NewBufferString(body))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawSession})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: rawCSRF})
	req.Header.Set("X-Xuanchu-CSRF", rawCSRF)
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("owner cookie create tenant token status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"token":"xuanchu_tenant_`)) {
		t.Fatalf("expected tenant token in body, got %s", rr.Body.String())
	}
}

// TestCookieSessionMemberCannotCreateTenantToken 锁定 capability 放行后，
// member 仍被 app 层 tokenManageAllowed(role) 拒绝。
func TestCookieSessionMemberCannotCreateTenantToken(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	store := srv.store
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("local workspace: %v", err)
	}
	user, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("get local user: %v", err)
	}
	// 把 local 降级为 member，验证 app 层 role 收紧仍生效。
	if err := storage.NewMemberRepository(store.DB()).UpdateRole(user.ID, ws.ID, "member", 100); err != nil {
		t.Fatalf("demote local to member: %v", err)
	}
	rawSession, rawCSRF := seedBrowserSession(t, store, user.ID, ws.ID)

	body := `{"name":"runtime","scopes":["task:read"]}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant-access-tokens", bytes.NewBufferString(body))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawSession})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: rawCSRF})
	req.Header.Set("X-Xuanchu-CSRF", rawCSRF)
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("member cookie create tenant token status = %d, want 403, body=%s", rr.Code, rr.Body.String())
	}
}

func TestBearerStillWorks(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	srv := fixture.server
	token := fixture.token
	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{"Authorization": "Bearer " + token})
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("bearer should work, got %d", rr.Code)
	}
}
