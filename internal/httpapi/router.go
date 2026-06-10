package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/mcpserver"
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
	api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces", s.handleAdminWorkspaceCreate)
	api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces/{workspace}/agent-tokens", s.handleAdminAgentTokenCreate)
	api.With(s.authMiddleware).Get("/api/v1/me", s.handleMe)
	api.With(s.authMiddleware).Put("/api/v1/me/active_workspace", s.handleMeActiveWorkspace)
	api.With(s.authMiddleware).Get("/api/v1/tasks", s.handleTaskList)
	api.With(s.authMiddleware).Post("/api/v1/tasks", s.handleTaskAdd)
	api.With(s.authMiddleware).Get("/api/v1/tasks/{taskRef}", s.handleTaskInfo)
	api.With(s.authMiddleware).Patch("/api/v1/tasks/{taskRef}", s.handleTaskModify)
	api.With(s.authMiddleware).Delete("/api/v1/tasks/{taskRef}", s.handleTaskDelete)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskRef}/done", s.handleTaskDone)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskRef}/start", s.handleTaskStart)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskRef}/stop", s.handleTaskStop)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskRef}/annotations", s.handleTaskAnnotate)
	api.With(s.authMiddleware).Delete("/api/v1/tasks/{taskRef}/annotations/{annotationID}", s.handleTaskDenotate)
	api.With(s.authMiddleware).Get("/api/v1/tasks/{taskRef}/urgency", s.handleTaskUrgency)
	api.With(s.authMiddleware).Get("/api/v1/tasks/{taskRef}/links", s.handleTaskLinkList)
	api.With(s.authMiddleware).Post("/api/v1/tasks/{taskRef}/links", s.handleTaskLinkAdd)
	api.With(s.authMiddleware).Delete("/api/v1/tasks/{taskRef}/links/{linkID}", s.handleTaskLinkRemove)
	api.With(s.authMiddleware).Get("/api/v1/reports/{name}", s.handleReport)
	api.With(s.authMiddleware).Get("/api/v1/users", s.handleUserList)
	api.With(s.authMiddleware).Post("/api/v1/users", s.handleUserCreate)
	api.With(s.authMiddleware).Get("/api/v1/users/{user}", s.handleUserInfo)
	api.With(s.authMiddleware).Post("/api/v1/users/{user}/external-ids", s.handleExternalIDBind)
	api.With(s.authMiddleware).Delete("/api/v1/users/{user}/external-ids/{provider}/{externalID}", s.handleExternalIDUnbind)
	api.With(s.authMiddleware).Get("/api/v1/users/{user}/external-ids", s.handleExternalIDList)
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
	api.With(s.authMiddleware).Post("/api/v1/projects/{projectRef}/annotations", s.handleProjectAnnotationAdd)
	api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}/annotations", s.handleProjectAnnotationList)
	api.With(s.authMiddleware).Delete("/api/v1/projects/{projectRef}/annotations/{annotationID}", s.handleProjectAnnotationDelete)
	api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}/timeline", s.handleProjectTimeline)
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
	api.With(s.authMiddleware).Get("/api/v1/config-schema", s.handleConfigSchemaList)
	api.With(s.authMiddleware).Get("/api/v1/config-schema/{key}", s.handleConfigSchemaGet)
	api.With(s.authMiddleware).Put("/api/v1/config-schema/{key}", s.handleConfigSchemaSet)
	api.With(s.authMiddleware).Delete("/api/v1/config-schema/{key}", s.handleConfigSchemaDelete)
	api.With(s.authMiddleware).Get("/api/v1/export", s.handleExport)
	api.With(s.authMiddleware).Post("/api/v1/import", s.handleImport)
	api.With(s.authMiddleware).Get("/api/v1/audit", s.handleAuditList)
	api.With(s.authMiddleware).Get("/api/v1/tokens", s.handleTokenList)
	api.With(s.authMiddleware).Post("/api/v1/tokens", s.handleTokenCreate)
	api.With(s.authMiddleware).Patch("/api/v1/tokens/{tokenRef}", s.handleTokenModify)
	api.With(s.authMiddleware).Delete("/api/v1/tokens/{tokenRef}", s.handleTokenRevoke)
	api.With(s.authMiddleware).Get("/api/v1/hooks", s.handleHookList)
	api.With(s.authMiddleware).Post("/api/v1/hooks", s.handleHookCreate)
	api.With(s.authMiddleware).Get("/api/v1/hooks/{hookID}", s.handleHookInfo)
	api.With(s.authMiddleware).Patch("/api/v1/hooks/{hookID}", s.handleHookModify)
	api.With(s.authMiddleware).Delete("/api/v1/hooks/{hookID}", s.handleHookDelete)
	api.With(s.authMiddleware).Post("/api/v1/hooks/{hookID}/enable", s.handleHookEnable)
	api.With(s.authMiddleware).Post("/api/v1/hooks/{hookID}/disable", s.handleHookDisable)
	api.With(s.authMiddleware).Get("/api/v1/hooks/{hookID}/deliveries", s.handleHookDeliveryList)
	api.With(s.authMiddleware).Get("/api/v1/hook-deliveries/{deliveryID}", s.handleHookDeliveryInfo)
	api.With(s.authMiddleware).Post("/api/v1/hook-deliveries/{deliveryID}/replay", s.handleHookDeliveryReplay)
	api.With(s.authMiddleware).Get("/api/v1/notification-sinks", s.handleNotificationSinkList)
	api.With(s.authMiddleware).Post("/api/v1/notification-sinks", s.handleNotificationSinkCreate)
	api.With(s.authMiddleware).Get("/api/v1/notification-sinks/{sinkID}", s.handleNotificationSinkInfo)
	api.With(s.authMiddleware).Patch("/api/v1/notification-sinks/{sinkID}", s.handleNotificationSinkModify)
	api.With(s.authMiddleware).Delete("/api/v1/notification-sinks/{sinkID}", s.handleNotificationSinkDelete)
	api.With(s.authMiddleware).Post("/api/v1/notification-sinks/{sinkID}/enable", s.handleNotificationSinkEnable)
	api.With(s.authMiddleware).Post("/api/v1/notification-sinks/{sinkID}/disable", s.handleNotificationSinkDisable)
	api.With(s.authMiddleware).Get("/api/v1/reminder-rules", s.handleReminderRuleList)
	api.With(s.authMiddleware).Post("/api/v1/reminder-rules", s.handleReminderRuleCreate)
	api.With(s.authMiddleware).Get("/api/v1/reminder-rules/{ruleID}", s.handleReminderRuleInfo)
	api.With(s.authMiddleware).Patch("/api/v1/reminder-rules/{ruleID}", s.handleReminderRuleModify)
	api.With(s.authMiddleware).Post("/api/v1/reminder-rules/{ruleID}/enable", s.handleReminderRuleEnable)
	api.With(s.authMiddleware).Post("/api/v1/reminder-rules/{ruleID}/disable", s.handleReminderRuleDisable)
	api.With(s.authMiddleware).Delete("/api/v1/reminder-rules/{ruleID}", s.handleReminderRuleDelete)
	api.With(s.authMiddleware).Get("/api/v1/notification-rules", s.handleEventNotificationRuleList)
	api.With(s.authMiddleware).Post("/api/v1/notification-rules", s.handleEventNotificationRuleCreate)
	api.With(s.authMiddleware).Get("/api/v1/notification-rules/{ruleID}", s.handleEventNotificationRuleInfo)
	api.With(s.authMiddleware).Patch("/api/v1/notification-rules/{ruleID}", s.handleEventNotificationRuleModify)
	api.With(s.authMiddleware).Post("/api/v1/notification-rules/{ruleID}/enable", s.handleEventNotificationRuleEnable)
	api.With(s.authMiddleware).Post("/api/v1/notification-rules/{ruleID}/disable", s.handleEventNotificationRuleDisable)
	api.With(s.authMiddleware).Delete("/api/v1/notification-rules/{ruleID}", s.handleEventNotificationRuleDelete)
	api.With(s.authMiddleware).Get("/api/v1/notification-deliveries", s.handleNotificationDeliveryList)
	api.With(s.authMiddleware).Get("/api/v1/notification-deliveries/{deliveryID}", s.handleNotificationDeliveryInfo)
	api.With(s.authMiddleware).Post("/api/v1/notification-deliveries/{deliveryID}/replay", s.handleNotificationDeliveryReplay)
	api.With(s.authMiddleware).Handle("/mcp", s.handleMCP())

	root.Handle("/", api)
	return root
}

func (s *Server) handleMCP() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		authReq := r
		if authn, ok := authFromContext(r.Context()); ok {
			authReq = mcpserver.SetHTTPAuthContext(r, authn.Authn)
		}
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
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}
