package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func openHTTPTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func assertHTTPErrorCode(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, wantStatus, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"`+wantCode+`"`) {
		t.Fatalf("body = %s, want error code %q", rr.Body.String(), wantCode)
	}
}

var pascalCaseJSONKey = regexp.MustCompile(`"[A-Z][A-Za-z]*":`)

func assertSnakeCaseResponse(t *testing.T, body string) {
	t.Helper()
	if key := pascalCaseJSONKey.FindString(body); key != "" {
		t.Fatalf("response leaked PascalCase key %q in body=%s", key, body)
	}
}

func TestHealthzIsAnonymous(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "schema") || strings.Contains(rr.Body.String(), "database") {
		t.Fatalf("healthz leaked details: %s", rr.Body.String())
	}
}

func TestErrorEnvelopeForUnknownRoute(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "route_not_found")

	rr = httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	assertHTTPErrorCode(t, rr, http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestConsoleRoutesDoNotInterceptAPIOrMCP(t *testing.T) {
	srv := NewServer(Options{
		Store: openHTTPTestStore(t),
		Console: config.ConsoleConfig{
			Enabled:     true,
			BasePath:    "/console",
			AssetsCache: time.Hour,
			AuthMode:    "bearer",
		},
	})

	for _, path := range []string{"/console", "/console/", "/console/tasks"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusOK && rr.Code != http.StatusPermanentRedirect {
				t.Fatalf("status = %d, want console response body=%s", rr.Code, rr.Body.String())
			}
		})
	}

	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "route_not_found")

	rr = httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`)))
	if rr.Code == http.StatusOK && strings.Contains(rr.Body.String(), "Xuanchu Console") {
		t.Fatalf("/mcp was served by console fallback: %s", rr.Body.String())
	}
}

func TestPanicIsRecoveredAsErrorEnvelope(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t), TestPanicRoute: true})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/__panic", nil))
	assertHTTPErrorCode(t, rr, http.StatusInternalServerError, "api_internal")
}
