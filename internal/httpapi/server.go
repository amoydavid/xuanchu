package httpapi

import (
	"io"
	"net/http"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/storage/sqlite"
)

const defaultBodyLimitBytes int64 = 10 << 20

type Options struct {
	Store          *sqlite.Store
	Clock          app.Clock
	Stderr         io.Writer
	BodyLimitBytes int64
	TestPanicRoute bool
}

type Server struct {
	store          *sqlite.Store
	clock          app.Clock
	stderr         io.Writer
	bodyLimitBytes int64
	testPanicRoute bool
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
