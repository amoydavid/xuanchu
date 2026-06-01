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
	api.With(s.authMiddleware).Put("/api/v1/me/active_workspace", s.handleMeActiveWorkspace)
	api.With(s.authMiddleware).Get("/api/v1/tasks", s.handleTaskList)
	api.With(s.authMiddleware).Post("/api/v1/tasks", s.handleTaskAdd)
	api.With(s.authMiddleware).Get("/api/v1/tasks/{taskID}", s.handleTaskInfo)
	api.With(s.authMiddleware).Patch("/api/v1/tasks/{taskID}", s.handleTaskModify)
	api.With(s.authMiddleware).Delete("/api/v1/tasks/{taskID}", s.handleTaskDelete)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskID}/done", s.handleTaskDone)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskID}/start", s.handleTaskStart)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskID}/stop", s.handleTaskStop)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskID}/annotations", s.handleTaskAnnotate)
	api.With(s.authMiddleware).Delete("/api/v1/tasks/{taskID}/annotations/{index}", s.handleTaskDenotate)
	api.With(s.authMiddleware).Get("/api/v1/tasks/{taskID}/urgency", s.handleTaskUrgency)
	api.With(s.authMiddleware).Get("/api/v1/reports/{name}", s.handleReport)
	api.With(s.authMiddleware).Get("/api/v1/users", s.handleUserList)
	api.With(s.authMiddleware).Post("/api/v1/users", s.handleUserCreate)
	api.With(s.authMiddleware).Get("/api/v1/users/{user}", s.handleUserInfo)
	api.With(s.authMiddleware).Get("/api/v1/workspaces", s.handleWorkspaceList)
	api.With(s.authMiddleware).Post("/api/v1/workspaces", s.handleWorkspaceAdd)
	api.With(s.authMiddleware).Get("/api/v1/workspaces/{workspace}", s.handleWorkspaceInfo)
	api.With(s.authMiddleware).Patch("/api/v1/workspaces/{workspace}", s.handleWorkspaceModify)
	api.With(s.authMiddleware).Post("/api/v1/workspaces/{workspace}/archive", s.handleWorkspaceArchive)
	api.With(s.authMiddleware).Get("/api/v1/workspaces/{workspace}/members", s.handleMemberList)
	api.With(s.authMiddleware).Post("/api/v1/workspaces/{workspace}/members", s.handleMemberAdd)
	api.With(s.authMiddleware).Patch("/api/v1/workspaces/{workspace}/members/{user}", s.handleMemberRole)
	api.With(s.authMiddleware).Get("/api/v1/projects", s.handleProjectList)
	api.With(s.authMiddleware).Post("/api/v1/projects", s.handleProjectAdd)
	api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}", s.handleProjectInfo)
	api.With(s.authMiddleware).Patch("/api/v1/projects/{projectRef}", s.handleProjectModify)
	api.With(s.authMiddleware).Post("/api/v1/projects/{projectRef}/archive", s.handleProjectArchive)
	api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}/config", s.handleProjectConfigList)
	api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}/config/{key}", s.handleProjectConfigGet)
	api.With(s.authMiddleware).Put("/api/v1/projects/{projectRef}/config/{key}", s.handleProjectConfigSet)
	api.With(s.authMiddleware).Delete("/api/v1/projects/{projectRef}/config/{key}", s.handleProjectConfigUnset)
	api.With(s.authMiddleware).Get("/api/v1/contexts", s.handleContextList)
	api.With(s.authMiddleware).Post("/api/v1/contexts", s.handleContextDefine)
	api.With(s.authMiddleware).Post("/api/v1/contexts/none", s.handleContextNone)
	api.With(s.authMiddleware).Get("/api/v1/contexts/{name}", s.handleContextInfo)
	api.With(s.authMiddleware).Delete("/api/v1/contexts/{name}", s.handleContextDelete)
	api.With(s.authMiddleware).Post("/api/v1/contexts/{name}/use", s.handleContextUse)
	api.With(s.authMiddleware).Get("/api/v1/config", s.handleConfigList)
	api.With(s.authMiddleware).Get("/api/v1/config/{key}", s.handleConfigGet)
	api.With(s.authMiddleware).Put("/api/v1/config/{key}", s.handleConfigSet)
	api.With(s.authMiddleware).Delete("/api/v1/config/{key}", s.handleConfigUnset)
	api.With(s.authMiddleware).Get("/api/v1/export", s.handleExport)
	api.With(s.authMiddleware).Post("/api/v1/import", s.handleImport)
	api.With(s.authMiddleware).Get("/api/v1/audit", s.handleAuditList)
	api.With(s.authMiddleware).Get("/api/v1/tokens", s.handleTokenList)
	api.With(s.authMiddleware).Post("/api/v1/tokens", s.handleTokenCreate)
	api.With(s.authMiddleware).Delete("/api/v1/tokens/{tokenRef}", s.handleTokenRevoke)

	root.Handle("/", api)
	return root
}
