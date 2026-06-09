package httpapi

import (
	"io"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

const defaultBodyLimitBytes int64 = 10 << 20

type Options struct {
	Store          *storage.Store
	Clock          app.Clock
	Stderr         io.Writer
	BodyLimitBytes int64
	TestPanicRoute bool
	Logger         *logging.Logger
	Admin          config.AdminConfig
}

type Server struct {
	store          *storage.Store
	clock          app.Clock
	stderr         io.Writer
	bodyLimitBytes int64
	testPanicRoute bool
	logger         *logging.Logger
	admin          config.AdminConfig
	router         *http.ServeMux
}

func NewServer(opts Options) *Server {
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.BodyLimitBytes <= 0 {
		opts.BodyLimitBytes = defaultBodyLimitBytes
	}
	srv := &Server{
		store:          opts.Store,
		clock:          opts.Clock,
		stderr:         opts.Stderr,
		bodyLimitBytes: opts.BodyLimitBytes,
		testPanicRoute: opts.TestPanicRoute,
		logger:         opts.Logger,
		admin:          opts.Admin,
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
