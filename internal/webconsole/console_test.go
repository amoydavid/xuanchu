package webconsole

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerServesConsoleAndFallback(t *testing.T) {
	handler := Handler(Options{
		Enabled:     true,
		BasePath:    "/console",
		AssetsCache: time.Hour,
	})

	for _, path := range []string{"/console/", "/console/tasks"} {
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
	handler := Handler(Options{Enabled: true, BasePath: "/console"})
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
	assetPath := findEmbeddedJSAsset(t)
	handler := Handler(Options{
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

func TestHandlerDisabledReturnsNotFound(t *testing.T) {
	handler := Handler(Options{Enabled: false, BasePath: "/console"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func findEmbeddedJSAsset(t *testing.T) string {
	t.Helper()
	entries, err := fs.ReadDir(embeddedDist, "dist/assets")
	if err != nil {
		t.Fatalf("ReadDir dist/assets: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") {
			return "assets/" + entry.Name()
		}
	}
	t.Fatal("no embedded JS asset found")
	return ""
}
