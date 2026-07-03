package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

const defaultBodyLimitBytes int64 = 10 << 20
const defaultAdminSetupTTL = 30 * time.Minute

type Options struct {
	Store          *storage.Store
	Clock          app.Clock
	Stderr         io.Writer
	BodyLimitBytes int64
	TestPanicRoute bool
	Logger         *logging.Logger
	Admin          config.AdminConfig
	AdminSetup     AdminSetupOptions
	Console        config.ConsoleConfig
	// TestConsoleHandler 仅供测试注入最小 Console，避免普通 Go 测试依赖前端 dist。
	TestConsoleHandler   http.Handler
	Shutdown             *runtimeutil.ShutdownCoordinator
	MCPTrustedProxyHosts []string
}

type AdminSetupOptions struct {
	Code string
	TTL  time.Duration
}

type Server struct {
	store                *storage.Store
	clock                app.Clock
	stderr               io.Writer
	bodyLimitBytes       int64
	testPanicRoute       bool
	logger               *logging.Logger
	admin                config.AdminConfig
	adminSetup           *adminSetupState
	console              config.ConsoleConfig
	testConsoleHandler   http.Handler
	shutdown             *runtimeutil.ShutdownCoordinator
	mcpTrustedProxyHosts []string
	router               *http.ServeMux
	oidcAuth             *app.OIDCAuthService // 懒加载，见 oidcAuthService()
}

func NewServer(opts Options) *Server {
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.BodyLimitBytes <= 0 {
		opts.BodyLimitBytes = defaultBodyLimitBytes
	}
	srv := &Server{
		store:                opts.Store,
		clock:                opts.Clock,
		stderr:               opts.Stderr,
		bodyLimitBytes:       opts.BodyLimitBytes,
		testPanicRoute:       opts.TestPanicRoute,
		logger:               opts.Logger,
		admin:                opts.Admin,
		adminSetup:           newAdminSetupState(opts.AdminSetup, opts.Clock),
		console:              opts.Console,
		testConsoleHandler:   opts.TestConsoleHandler,
		shutdown:             opts.Shutdown,
		mcpTrustedProxyHosts: normalizeMCPTrustedProxyHosts(opts.MCPTrustedProxyHosts),
	}
	srv.router = srv.newRouter()
	return srv
}

func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func generateAdminSetupCode() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
