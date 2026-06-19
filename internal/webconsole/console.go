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

// spaRoutes 是允许 fallback 到 index.html 的精确路径。
// spaPrefixes 是允许 fallback 的路径前缀，用于 /workspaces/.../projects/...
// 这类多段 deep link：直接访问或刷新这些 URL 时，服务端也要返回 index.html，
// 让前端路由接管，而不是 404。前缀用末尾 "/" 表示只匹配路径段，避免误命中
// 名字相近的静态文件或未来新增的顶层 API 前缀。
var spaRoutes = map[string]struct{}{
	"":                  {},
	"admin":             {},
	"admin/login":       {},
	"admin/setup":       {},
	"admin/tokens":      {},
	"admin/workspaces":  {},
	"audit":             {},
	"hooks":             {},
	"members":           {},
	"notifications":     {},
	"projects":          {},
	"settings":          {},
	"tasks":             {},
	"tokens":            {},
	"workspaces":        {},
}

var spaPrefixes = []string{
	// workspaces 下是多段 SPA 路由，例如
	// /workspaces/{slug}/projects/{slug} 和 /workspaces/{slug}/projects/{slug}/tasks/{ref}
	"workspaces/",
	// admin/workspaces/{slug} 是 workspace 控制面详情 deep link，刷新也要返回 index.html。
	"admin/workspaces/",
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
	trimmed := strings.Trim(rel, "/")
	if _, ok := spaRoutes[trimmed]; ok {
		h.serveIndex(w, r)
		return
	}
	for _, prefix := range spaPrefixes {
		if strings.HasPrefix(trimmed+"/", prefix) {
			h.serveIndex(w, r)
			return
		}
	}
	http.NotFound(w, r)
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	h.serveBytes(w, "index.html")
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
