package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"strings"

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
	api.Get("/api/v1/admin/status", s.handleAdminStatus)
	api.Post("/api/v1/admin/setup", s.handleAdminSetup)
	api.With(s.adminAuthMiddleware).Get("/api/v1/admin/session", s.handleAdminSession)
	api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces", s.handleAdminWorkspaceCreate)
	api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces/{workspace}/admins", s.handleAdminWorkspaceAdminCreate)
	api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces/{workspace}/agent-tokens", s.handleAdminAgentTokenCreate)
	api.With(s.adminAuthMiddleware).Get("/api/v1/admin/tokens", s.handleAdminTokenList)
	api.With(s.adminAuthMiddleware).Patch("/api/v1/admin/tokens/{tokenRef}", s.handleAdminTokenModify)
	api.With(s.adminAuthMiddleware).Delete("/api/v1/admin/tokens/{tokenRef}", s.handleAdminTokenRevoke)
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
	api.With(s.authMiddleware).Post("/api/v1/projects/{projectRef}/transition", s.handleProjectTransition)
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
	api.With(s.mcpHostProtectionMiddleware, s.authMiddleware).Handle("/mcp", s.handleMCP())

	if s.console.Enabled {
		handler := s.consoleHandler()
		if strings.TrimRight(s.console.BasePath, "/") == "" {
			root.Handle("/api/", api)
			root.Handle("/healthz", api)
			root.Handle("/mcp", api)
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
