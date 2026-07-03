package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
	fixture := newHTTPServerWithTokenFixture(t); srv := fixture.server
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
	fixture := newHTTPServerWithTokenFixture(t); srv := fixture.server
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
	fixture := newHTTPServerWithTokenFixture(t); srv := fixture.server
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
	fixture := newHTTPServerWithTokenFixture(t); srv := fixture.server
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no credential should be 401, got %d", rr.Code)
	}
}

func TestBearerStillWorks(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t); srv := fixture.server; token := fixture.token
	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{"Authorization": "Bearer " + token})
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("bearer should work, got %d", rr.Code)
	}
}
