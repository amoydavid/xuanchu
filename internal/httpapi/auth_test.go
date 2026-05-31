package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dajee/taskg/internal/app"
)

type httpTokenFixture struct {
	server *Server
	token  string
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
	}
}

func requestHTTP(t *testing.T, srv *Server, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
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
