package httpapi

import (
	"encoding/json"
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

func TestOpenAPIIsGeneratedFromRegisteredHTTPRoutes(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var doc struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"info"`
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid OpenAPI JSON: %v body=%s", err, rr.Body.String())
	}
	if doc.OpenAPI == "" || doc.Info.Title != "Xuanchu HTTP API" || doc.Info.Version != "v1" {
		t.Fatalf("unexpected OpenAPI metadata: %#v", doc)
	}
	for _, path := range []string{
		"/api/v1/tasks/{taskRef}/links",
		"/api/v1/config-schema/{key}",
		"/api/v1/projects/{projectRef}/timeline",
	} {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("OpenAPI paths missing %s", path)
		}
	}
	if _, ok := doc.Paths["/mcp"]; ok {
		t.Fatalf("OpenAPI unexpectedly documented /mcp")
	}
	if _, ok := doc.Paths["/api/v1/__panic"]; ok {
		t.Fatalf("OpenAPI unexpectedly documented test panic route")
	}
}

func TestOpenAPIIncludesEveryRegisteredHTTPRoute(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var doc struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid OpenAPI JSON: %v body=%s", err, rr.Body.String())
	}
	for _, route := range srv.humaRoutes() {
		pathItem, ok := doc.Paths[route.Path]
		if !ok {
			t.Fatalf("OpenAPI paths missing %s", route.Path)
		}
		if _, ok := pathItem[strings.ToLower(route.Method)]; !ok {
			t.Fatalf("OpenAPI path %s missing method %s", route.Path, route.Method)
		}
	}
	if got, want := countOpenAPIOperations(doc.Paths), len(srv.humaRoutes()); got != want {
		t.Fatalf("OpenAPI operation count = %d, want %d", got, want)
	}
}

func countOpenAPIOperations(paths map[string]map[string]any) int {
	methods := map[string]struct{}{
		"get":     {},
		"post":    {},
		"put":     {},
		"patch":   {},
		"delete":  {},
		"head":    {},
		"options": {},
		"trace":   {},
	}
	total := 0
	for _, pathItem := range paths {
		for method := range pathItem {
			if _, ok := methods[method]; ok {
				total++
			}
		}
	}
	return total
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
			BasePath:    "/",
			AssetsCache: time.Hour,
			AuthMode:    "bearer",
		},
		TestConsoleHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/", "/tasks":
				_, _ = w.Write([]byte("<div id=\"root\"></div>"))
			default:
				http.NotFound(w, r)
			}
		}),
	})

	for _, path := range []string{"/", "/tasks"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want console response body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "<div id=\"root\"></div>") {
				t.Fatalf("body = %s, want console html", rr.Body.String())
			}
		})
	}

	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("/console status = %d, want 404 body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
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
