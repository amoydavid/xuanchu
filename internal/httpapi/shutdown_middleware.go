package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
)

func (s *Server) shutdownMiddleware(next http.Handler) http.Handler {
	if s.shutdown == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done, ok := s.shutdown.Begin()
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "server_draining", "server is shutting down", nil)
			return
		}
		defer done()
		ctx, cancel := runtimeutil.ContextWithCancelOnEither(r.Context(), s.shutdown.Context())
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
