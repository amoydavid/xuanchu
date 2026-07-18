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

// TestHandlerServesWorkspaceDeepLinksAtRoot 覆盖 v0.4.2 的项目只读 deep link：
// 直接访问或刷新 /workspaces/{slug}/projects/{slug}[...] 必须返回 SPA index.html，
// 让前端路由接管，而不是 404。basePath="/" 是生产 server 的默认形态。
func TestHandlerServesWorkspaceDeepLinksAtRoot(t *testing.T) {
	handler := testHandler(Options{Enabled: true, BasePath: "/"})

	cases := []string{
		"/",
		"/tasks",
		"/my-tasks",
		"/admin/login",
		"/admin/setup",
		"/admin/workspaces",
		"/admin/workspaces/dajee",
		"/workspaces/acme",
		"/workspaces/acme/projects/agentapi",
		"/workspaces/acme/projects/agentapi/tasks/ag-23",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "<div id=\"root\"></div>") {
				t.Fatalf("body = %q, want SPA index.html", rr.Body.String())
			}
		})
	}
}

// TestHandlerServesWorkspaceDeepLinksWithBasePath 确认带 BasePath 时多段 deep link 同样 fallback。
func TestHandlerServesWorkspaceDeepLinksWithBasePath(t *testing.T) {
	handler := testHandler(Options{Enabled: true, BasePath: "/console"})

	for _, p := range []string{
		"/console/workspaces/acme/projects/agentapi",
		"/console/workspaces/acme/projects/agentapi/tasks/ag-23",
	} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200 body=%s", p, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "<div id=\"root\"></div>") {
			t.Fatalf("%s body = %q, want SPA index.html", p, rr.Body.String())
		}
	}
}

// TestHandlerRejectsSystemPrefixes 确认服务端独占前缀不会被 fallback 到 SPA。
// 这些前缀正常由 httpapi router 优先分发，webconsole 收到时必须 404，
// 避免未来路由漏配时把 API 请求伪装成前端页面。
func TestHandlerRejectsSystemPrefixes(t *testing.T) {
	handler := testHandler(Options{Enabled: true, BasePath: "/"})

	for _, p := range []string{
		"/api/v1/tasks",
		"/healthz",
		"/sso/login",
		"/auth/callback",
		"/mcp",
		"/docs",
		"/openapi.json",
		"/openapi.yaml",
		"/openapi-3.0.json",
		"/openapi-3.0.yaml",
		"/schemas/operation.json",
	} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404 (system prefix must not fallback)", p, rr.Code)
		}
	}
}

// TestHandlerFallsBackForUnknownSPAPath 确认默认放行策略：
// dist 里没有同名静态文件的非系统路径，一律 fallback 到 index.html，
// 由前端路由决定渲染或 404。新增前端顶层路由无需同步后端白名单。
func TestHandlerFallsBackForUnknownSPAPath(t *testing.T) {
	handler := testHandler(Options{Enabled: true, BasePath: "/"})

	for _, p := range []string{
		"/random-unknown-page",
		"/foo/bar/baz",
		"/workspaces2/evil",
		"/my-tasks",
		"/any/new/route/that/frontend/added",
	} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200 (fallback to index.html)", p, rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "<div id=\"root\"></div>") {
			t.Fatalf("%s body = %q, want SPA index.html", p, rr.Body.String())
		}
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
