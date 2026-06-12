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
	basePath := strings.TrimRight(opts.BasePath, "/")
	if basePath == "" {
		basePath = "/"
	}
	dist, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return http.NotFoundHandler()
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

var spaRoutes = map[string]struct{}{
	"":              {},
	"admin":         {},
	"admin/login":   {},
	"admin/setup":   {},
	"audit":         {},
	"hooks":         {},
	"members":       {},
	"notifications": {},
	"projects":      {},
	"settings":      {},
	"tasks":         {},
	"tokens":        {},
	"workspaces":    {},
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
	if _, ok := spaRoutes[strings.Trim(rel, "/")]; !ok {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
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
