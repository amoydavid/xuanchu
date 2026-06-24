package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/mcpserver"
	"git.dajee.net/dajee/xuanchu/internal/webconsole"
	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) newRouter() *http.ServeMux {
	root := http.NewServeMux()
	api := chi.NewRouter()

	api.Use(s.shutdownMiddleware)
	api.Use(s.requestIDMiddleware)
	api.Use(s.recovererMiddleware)
	api.Use(s.accessLogMiddleware)
	api.Use(s.bodyLimitMiddleware)
	api.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "route_not_found", "route not found", nil)
	})
	api.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	})

	api.Get("/healthz", s.handleHealthz)
	if s.testPanicRoute {
		api.Get("/api/v1/__panic", func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})
	}
	s.registerHumaRoutes(api)
	api.With(s.mcpHostProtectionMiddleware, s.authMiddleware, s.rejectActingTokenMiddleware).Handle("/mcp", s.handleMCP())

	if s.console.Enabled {
		handler := s.consoleHandler()
		if strings.TrimRight(s.console.BasePath, "/") == "" {
			root.Handle("/api/", api)
			root.Handle("/healthz", api)
			root.Handle("/mcp", api)
			root.Handle("/docs", api)
			root.Handle("/openapi.json", api)
			root.Handle("/openapi-3.0.json", api)
			root.Handle("/openapi.yaml", api)
			root.Handle("/openapi-3.0.yaml", api)
			root.Handle("/schemas/", api)
			root.Handle("/", handler)
			return root
		}
		root.Handle(s.console.BasePath, handler)
		root.Handle(s.console.BasePath+"/", handler)
	}
	root.Handle("/", api)
	return root
}

func (s *Server) consoleHandler() http.Handler {
	if s.testConsoleHandler != nil {
		return s.testConsoleHandler
	}
	return webconsole.Handler(webconsole.Options{
		Enabled:     true,
		BasePath:    s.console.BasePath,
		AssetsCache: s.console.AssetsCache,
	})
}

func (s *Server) handleMCP() http.Handler {
	opts := &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}
	if len(s.mcpTrustedProxyHosts) > 0 {
		opts.DisableLocalhostProtection = true
	}
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		authReq := r
		if authn, ok := authFromContext(r.Context()); ok {
			authReq = mcpserver.SetHTTPAuthContext(r, authn.Authn)
		}
		authReq.Header.Set("X-Request-Id", requestIDFromContext(r.Context()))
		return mcpserver.NewServer(mcpserver.Options{
			Store:    s.store,
			Clock:    s.effectiveClock(),
			Version:  "dev",
			Mode:     mcpserver.ModeHTTP,
			Stderr:   s.stderr,
			Request:  authReq,
			Logger:   s.logger,
			Shutdown: s.shutdown,
		})
	}, opts)
}

// rejectActingTokenMiddleware 拒绝 acting token（xuanchu_act_）访问受保护端点。
// acting token 是浏览器短期委托凭证，只面向普通 HTTP API；
// HTTP MCP 和任何明确不信任 acting token 的端点都应挂上该中间件。
func (s *Server) rejectActingTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authn, ok := authFromContext(r.Context()); ok {
			if authn.Authn.Token.Type == auth.TokenTypeAdminActing {
				writeError(w, http.StatusUnauthorized, "admin_acting_not_allowed", "acting token is not allowed on this endpoint", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mcpHostProtectionMiddleware(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, host := range s.mcpTrustedProxyHosts {
		allowed[host] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestArrivedOnLoopback(r) {
			next.ServeHTTP(w, r)
			return
		}
		host := normalizeMCPHost(r.Host)
		if isLoopbackHTTPHost(host) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := allowed[host]; ok {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, fmt.Sprintf("Forbidden: invalid Host header %q", r.Host), http.StatusForbidden)
	})
}

func requestArrivedOnLoopback(r *http.Request) bool {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func isLoopbackHTTPHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func normalizeMCPTrustedProxyHosts(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		host := normalizeMCPHost(value)
		if host == "" || host == "*" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	return out
}

func normalizeMCPHost(value string) string {
	host := strings.ToLower(strings.TrimSpace(value))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.HasPrefix(host, "[") && strings.Contains(host, "]") {
		if end := strings.Index(host, "]"); end > 0 {
			host = host[1:end]
		}
	}
	return strings.Trim(host, "[]")
}
