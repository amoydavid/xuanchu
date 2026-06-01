package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dajee/taskg/internal/app"
)

type httpTokenFixture struct {
	server *Server
	token  string
	id     string
}

func newHTTPServerWithTokenFixture(t *testing.T, scopes ...string) httpTokenFixture {
	t.Helper()
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "http-test",
		Scopes:        scopes,
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return httpTokenFixture{
		server: NewServer(Options{Store: store}),
		token:  created.RawToken,
		id:     created.View.ID,
	}
}

func requestHTTP(t *testing.T, srv *Server, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return requestHTTPBody(t, srv, method, path, "", headers)
}

func requestHTTPBody(t *testing.T, srv *Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	return rr
}

func TestAuthRequiresBearerHeader(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", nil)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me?token=bad", nil)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "bad"})
	rr = httptest.NewRecorder()
	fixture.server.Router().ServeHTTP(rr, req)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "Basic abc"})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMeReturnsActorTokenAndWorkspace(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`"name":"local"`, `"type":"pat"`, `"slug":"local"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want %s", body, want)
		}
	}
}

func TestConfigEndpointDoesNotExposeOrWriteInternalMeta(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:read", "config:write")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/config", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, forbidden := range []string{"active_user_id", "active_workspace.", "active_context.", "database.path", "remote.token"} {
		if strings.Contains(rr.Body.String(), forbidden) {
			t.Fatalf("config response leaked %q: %s", forbidden, rr.Body.String())
		}
	}

	rr = requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/config/active_user_id", `{"value":"evil"}`, authHeader)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "config_scope_invalid")
	rr = requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/config/active_context.local.local", authHeader)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "config_scope_invalid")
}

func TestWorkspaceScopeDeniedIsForbidden(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	if err := fixture.server.store.DB().Exec(
		`UPDATE api_tokens SET workspace_ids_json = ? WHERE id = ?`,
		`["missing-workspace"]`,
		fixture.id,
	).Error; err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "Bearer " + fixture.token})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "workspace_scope_denied")
}
