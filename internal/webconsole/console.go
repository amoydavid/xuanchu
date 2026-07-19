package webconsole

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

//go:embed all:dist
var embeddedDist embed.FS

type Options struct {
	Enabled     bool
	BasePath    string
	AssetsCache time.Duration
}

func Handler(opts Options) http.Handler {
	if !opts.Enabled {
		return http.NotFoundHandler()
	}
	dist, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return handlerWithDist(opts, dist)
}

// handlerWithDist 让单元测试注入最小前端产物，避免测试依赖真实构建后的 dist。
func handlerWithDist(opts Options, dist fs.FS) http.Handler {
	if !opts.Enabled {
		return http.NotFoundHandler()
	}
	basePath := strings.TrimRight(opts.BasePath, "/")
	if basePath == "" {
		basePath = "/"
	}
	return &handler{
		basePath:    basePath,
		assetsCache: opts.AssetsCache,
		dist:        dist,
	}
}

type handler struct {
	basePath    string
	assetsCache time.Duration
	dist        fs.FS
}

// systemPrefixes 是服务端独占、绝不能 fallback 到 SPA index.html 的路径前缀。
// 它们已在 httpapi router 里被优先分发给 API router（最长前缀匹配），
// webconsole 正常情况下收不到；这里保留一份防御性排除，避免未来路由重构
// 把这些前缀漏配时被前端 404 页面掩盖成"看起来正常的 SPA"。
var systemPrefixes = []string{
	"api/",
	"healthz",
	"sso/",
	"auth/",
	"mcp",
	"docs",
	"openapi.json",
	"openapi-3.0.json",
	"openapi.yaml",
	"openapi-3.0.yaml",
	"schemas/",
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.basePath == "/" {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if rel == "" {
			h.serveIndex(w, r)
			return
		}
		h.servePath(w, r, rel)
		return
	}
	if r.URL.Path == h.basePath {
		http.Redirect(w, r, h.basePath+"/", http.StatusPermanentRedirect)
		return
	}
	if !strings.HasPrefix(r.URL.Path, h.basePath+"/") {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, h.basePath+"/")
	if rel == "" {
		h.serveIndex(w, r)
		return
	}
	h.servePath(w, r, rel)
}

func (h *handler) servePath(w http.ResponseWriter, r *http.Request, rel string) {
	if strings.HasPrefix(rel, "assets/") {
		if _, err := fs.Stat(h.dist, path.Clean(rel)); err != nil {
			http.NotFound(w, r)
			return
		}
		h.setAssetCache(w)
		h.serveFile(w, r, rel)
		return
	}
	if stat, err := fs.Stat(h.dist, path.Clean(rel)); err == nil && !stat.IsDir() {
		h.setAssetCache(w)
		h.serveFile(w, r, rel)
		return
	}
	// 默认放行：dist 里没有同名静态文件、也不是系统前缀的路径，一律返回 index.html，
	// 让前端路由接管。新增前端顶层路由不再需要同步维护后端白名单。
	// 系统前缀（API/MCP/openapi 等）由 httpapi router 优先分发，这里只是防御性兜底。
	trimmed := strings.Trim(rel, "/")
	if isSystemPath(trimmed) {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

// isSystemPath 判断路径是否落在服务端独占前缀下，这些路径即便没命中静态文件
// 也不应 fallback 到 SPA，避免把 API 类请求伪装成前端页面。
func isSystemPath(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	for _, prefix := range systemPrefixes {
		if trimmed == strings.TrimSuffix(prefix, "/") ||
			strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	// CSP：附件图片通过鉴权 fetch + URL.createObjectURL 渲染，需要 blob:；
	// data: 仅用于编辑器内粘贴预览，持久化内容不得保留 data URL。
	w.Header().Set("Content-Security-Policy", consoleContentSecurityPolicy())
	h.serveBytes(w, "index.html")
}

// consoleContentSecurityPolicy 返回 Web Console 的 CSP 指令。
//
// img-src 同时允许 'self'、blob:（鉴权后的 object URL）和 data:（粘贴预览），
// 其它指令保持收紧。
func consoleContentSecurityPolicy() string {
	return "default-src 'self'; " +
		"img-src 'self' blob: data:; " +
		"style-src 'self' 'unsafe-inline'; " +
		"script-src 'self'; " +
		"connect-src 'self'; " +
		"font-src 'self' data:; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"frame-ancestors 'none'"
}

func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, rel string) {
	h.serveBytes(w, rel)
}

func (h *handler) setAssetCache(w http.ResponseWriter) {
	if h.assetsCache <= 0 {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age="+strconv.FormatInt(int64(h.assetsCache.Seconds()), 10))
}

func (h *handler) serveBytes(w http.ResponseWriter, rel string) {
	data, err := fs.ReadFile(h.dist, path.Clean(rel))
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	if strings.HasSuffix(rel, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if strings.HasSuffix(rel, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else if strings.HasSuffix(rel, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else if strings.HasSuffix(rel, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
	} else if strings.HasSuffix(rel, ".woff2") {
		w.Header().Set("Content-Type", "font/woff2")
	}
	_, _ = w.Write(data)
}
