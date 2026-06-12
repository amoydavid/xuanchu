package webconsole

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var testDist = fstest.MapFS{
	"assets/app.css":    {Data: []byte("body{color:#111}")},
	"assets/app.js":     {Data: []byte("document.body.dataset.app='xuanchu'")},
	"favicon.svg":       {Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")},
	"index.html":        {Data: []byte("<!doctype html><div id=\"root\"></div><script type=\"module\" src=\"/assets/app.js\"></script>")},
	"xuanchu-logo.svg":  {Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")},
	"assets/font.woff2": {Data: []byte("font")},
}

func TestHandlerServesConsoleAndFallback(t *testing.T) {
	handler := testHandler(Options{
		Enabled:     true,
		BasePath:    "/console",
		AssetsCache: time.Hour,
	})

	for _, path := range []string{"/console/", "/console/tasks", "/console/admin", "/console/admin/login", "/console/admin/setup"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
			}
			body := rr.Body.String()
			if !strings.Contains(body, "<div id=\"root\"></div>") {
				t.Fatalf("body = %q, want console html", rr.Body.String())
			}
			if got := rr.Header().Get("Cache-Control"); strings.Contains(got, "immutable") {
				t.Fatalf("index Cache-Control = %q, should not be immutable", got)
			}
		})
	}
}

func TestHandlerRedirectsBareBasePath(t *testing.T) {
	handler := testHandler(Options{Enabled: true, BasePath: "/console"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console", nil))
	if rr.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want 308", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/console/" {
		t.Fatalf("Location = %q, want /console/", got)
	}
}

func TestHandlerServesAssetsWithCache(t *testing.T) {
	assetPath := findTestAsset(t, ".js")
	handler := testHandler(Options{
		Enabled:     true,
		BasePath:    "/console",
		AssetsCache: time.Hour,
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console/"+assetPath, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}
	if got := strings.TrimSpace(rr.Body.String()); got == "" {
		t.Fatal("asset body is empty")
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want public, max-age=3600", got)
	}
}

func TestHandlerServesCSSWithContentType(t *testing.T) {
	assetPath := findTestAsset(t, ".css")
	handler := testHandler(Options{
		Enabled:     true,
		BasePath:    "/console",
		AssetsCache: time.Hour,
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console/"+assetPath, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/css; charset=utf-8", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want public, max-age=3600", got)
	}
}

func TestHandlerServesRootStaticAssetsWithCache(t *testing.T) {
	handler := testHandler(Options{
		Enabled:     true,
		BasePath:    "/console",
		AssetsCache: time.Hour,
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console/favicon.svg", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<svg") {
		t.Fatalf("body = %q, want svg", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("Content-Type = %q, want image/svg+xml", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want public, max-age=3600", got)
	}
}

func TestHandlerDisabledReturnsNotFound(t *testing.T) {
	handler := Handler(Options{Enabled: false, BasePath: "/console"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func testHandler(opts Options) http.Handler {
	return handlerWithDist(opts, testDist)
}

func findTestAsset(t *testing.T, suffix string) string {
	t.Helper()
	entries, err := fs.ReadDir(testDist, "assets")
	if err != nil {
		t.Fatalf("ReadDir assets: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return "assets/" + entry.Name()
		}
	}
	t.Fatalf("no embedded %s asset found", suffix)
	return ""
}
