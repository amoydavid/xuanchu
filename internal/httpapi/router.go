package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) newRouter() *http.ServeMux {
	root := http.NewServeMux()
	api := chi.NewRouter()

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
	api.With(s.authMiddleware).Get("/api/v1/me", s.handleMe)
	api.With(s.authMiddleware).Get("/api/v1/tasks", s.handleTaskList)
	api.With(s.authMiddleware).Post("/api/v1/tasks", s.handleTaskAdd)
	api.With(s.authMiddleware).Get("/api/v1/tasks/{taskID}", s.handleTaskInfo)
	api.With(s.authMiddleware).Get("/api/v1/projects", s.handleProjectList)
	api.With(s.authMiddleware).Post("/api/v1/projects", s.handleProjectAdd)
	api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}", s.handleProjectInfo)
	api.With(s.authMiddleware).Get("/api/v1/tokens", s.handleTokenList)

	root.Handle("/", api)
	return root
}
