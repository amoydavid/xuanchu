package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

type humaRoute struct {
	Method  string
	Path    string
	Tag     string
	Summary string
	Handler http.HandlerFunc
	Public  bool
	Admin   bool
	Status  int
}

var pathParamPattern = regexp.MustCompile(`\{([^}/]+)\}`)

func (s *Server) registerHumaRoutes(r chi.Router) huma.API {
	cfg := huma.DefaultConfig("Xuanchu HTTP API", "v1")
	cfg.Servers = []*huma.Server{{URL: "http://127.0.0.1:8080"}}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "xuanchu_pat, xuanchu_agent, xuanchu_tenant, or xuanchu_act",
			Description:  "Ordinary workspace API endpoints accept PAT, Agent, tenant access tokens, or short-lived server-admin acting tokens.",
		},
		"serverAdminAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "xuanchu_admin",
		},
	}
	cfg.OpenAPI.Security = []map[string][]string{{"bearerAuth": []string{}}}
	cfg.DocsPath = "/docs"
	cfg.OpenAPIPath = "/openapi"

	api := humachi.New(r, cfg)
	for _, route := range s.humaRoutes() {
		s.registerHumaBridge(api, route)
	}
	return api
}

func (s *Server) registerHumaBridge(api huma.API, route humaRoute) {
	op := &huma.Operation{
		OperationID: operationID(route.Method, route.Path),
		Method:      route.Method,
		Path:        route.Path,
		Tags:        []string{route.Tag},
		Summary:     route.Summary,
		Parameters:  pathParameters(route.Path),
		Responses: map[string]*huma.Response{
			defaultResponseStatus(route): jsonResponse("Successful response."),
			"400":                        jsonResponse("Bad request."),
			"401":                        jsonResponse("Unauthorized."),
			"403":                        jsonResponse("Forbidden."),
			"404":                        jsonResponse("Not found."),
			"500":                        jsonResponse("Internal server error."),
		},
		Security: []map[string][]string{{"bearerAuth": []string{}}},
	}
	if route.Admin {
		op.Security = []map[string][]string{{"serverAdminAuth": []string{}}}
	}
	if route.Public {
		op.Security = []map[string][]string{}
	}
	if route.Method == http.MethodPost || route.Method == http.MethodPut || route.Method == http.MethodPatch {
		op.RequestBody = &huma.RequestBody{
			Description: "JSON request body.",
			Required:    route.Method != http.MethodPost || !strings.HasSuffix(route.Path, "/enable") && !strings.HasSuffix(route.Path, "/disable") && !strings.HasSuffix(route.Path, "/done") && !strings.HasSuffix(route.Path, "/start") && !strings.HasSuffix(route.Path, "/stop") && !strings.HasSuffix(route.Path, "/archive") && !strings.HasSuffix(route.Path, "/use") && !strings.HasSuffix(route.Path, "/none") && !strings.HasSuffix(route.Path, "/replay"),
			Content: map[string]*huma.MediaType{
				"application/json": {Schema: &huma.Schema{Type: "object"}},
			},
		}
	}
	api.OpenAPI().AddOperation(op)
	api.Adapter().Handle(op, func(ctx huma.Context) {
		req, res := humachi.Unwrap(ctx)
		handler := http.Handler(http.HandlerFunc(route.Handler))
		switch {
		case route.Public:
		case route.Admin:
			handler = s.adminAuthMiddleware(handler)
		default:
			handler = s.authMiddleware(handler)
		}
		handler.ServeHTTP(res, req)
	})
}

func defaultResponseStatus(route humaRoute) string {
	if route.Status != 0 {
		return fmt.Sprintf("%d", route.Status)
	}
	return "200"
}

func jsonResponse(description string) *huma.Response {
	return &huma.Response{
		Description: description,
		Content: map[string]*huma.MediaType{
			"application/json": {Schema: &huma.Schema{Type: "object"}},
		},
	}
}

func pathParameters(path string) []*huma.Param {
	matches := pathParamPattern.FindAllStringSubmatch(path, -1)
	params := make([]*huma.Param, 0, len(matches))
	for _, match := range matches {
		params = append(params, &huma.Param{
			Name:        match[1],
			In:          "path",
			Required:    true,
			Description: fmt.Sprintf("%s path parameter.", match[1]),
			Schema:      &huma.Schema{Type: "string"},
		})
	}
	return params
}

func operationID(method, path string) string {
	replacer := strings.NewReplacer(
		"/api/v1/", "",
		"/api/v1", "",
		"/", "-",
		"{", "",
		"}", "",
		"_", "-",
	)
	id := strings.Trim(replacer.Replace(path), "-")
	id = strings.ReplaceAll(id, "--", "-")
	if id == "" {
		id = strings.Trim(path, "/")
	}
	return strings.ToLower(method) + "-" + id
}

func (s *Server) humaRoutes() []humaRoute {
	return []humaRoute{
		{Method: http.MethodGet, Path: "/healthz", Tag: "Health", Summary: "Health check.", Handler: s.handleHealthz, Public: true},
		{Method: http.MethodGet, Path: "/api/v1/admin/status", Tag: "Admin", Summary: "Get server admin bootstrap status.", Handler: s.handleAdminStatus, Public: true},
		{Method: http.MethodPost, Path: "/api/v1/admin/setup", Tag: "Admin", Summary: "Complete server admin setup.", Handler: s.handleAdminSetup, Public: true, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/admin/session", Tag: "Admin", Summary: "Get current server admin session.", Handler: s.handleAdminSession, Admin: true},
		{Method: http.MethodPost, Path: "/api/v1/admin/workspaces", Tag: "Admin", Summary: "Create a workspace from server admin control plane.", Handler: s.handleAdminWorkspaceCreate, Admin: true, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/admin/workspaces", Tag: "Admin", Summary: "List workspaces from server admin control plane.", Handler: s.handleAdminWorkspaceList, Admin: true},
		{Method: http.MethodGet, Path: "/api/v1/admin/workspaces/{workspace}", Tag: "Admin", Summary: "Get server admin workspace details.", Handler: s.handleAdminWorkspaceInfo, Admin: true},
		{Method: http.MethodPatch, Path: "/api/v1/admin/workspaces/{workspace}/users/{user}", Tag: "Admin", Summary: "Update a workspace user's display fields from server admin control plane.", Handler: s.handleAdminWorkspaceUserModify, Admin: true},
		{Method: http.MethodPost, Path: "/api/v1/admin/workspaces/{workspace}/admins", Tag: "Admin", Summary: "Create or promote a workspace admin.", Handler: s.handleAdminWorkspaceAdminCreate, Admin: true, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/admin/workspaces/{workspace}/agent-tokens", Tag: "Admin", Summary: "Create a workspace agent token.", Handler: s.handleAdminAgentTokenCreate, Admin: true, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/admin/workspaces/{workspace}/acting-sessions", Tag: "Admin", Summary: "Create a server admin acting session.", Handler: s.handleAdminActingSessionCreate, Admin: true, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/admin/workspaces/{workspace}/tenant-access-sessions", Tag: "Admin", Summary: "Create a short-lived tenant access token for admin workspace switch.", Handler: s.handleAdminTenantAccessSessionCreate, Admin: true, Status: http.StatusCreated},
		{Method: http.MethodDelete, Path: "/api/v1/admin/acting-sessions/{sessionID}", Tag: "Admin", Summary: "Revoke a server admin acting session.", Handler: s.handleAdminActingSessionRevoke, Admin: true},
		{Method: http.MethodGet, Path: "/api/v1/admin/tokens", Tag: "Admin", Summary: "List server admin tokens.", Handler: s.handleAdminTokenList, Admin: true},
		{Method: http.MethodPatch, Path: "/api/v1/admin/tokens/{tokenRef}", Tag: "Admin", Summary: "Modify a server admin token.", Handler: s.handleAdminTokenModify, Admin: true},
		{Method: http.MethodDelete, Path: "/api/v1/admin/tokens/{tokenRef}", Tag: "Admin", Summary: "Revoke a server admin token.", Handler: s.handleAdminTokenRevoke, Admin: true},
		{Method: http.MethodGet, Path: "/api/v1/admin/tenant-access-tokens", Tag: "Admin", Summary: "List tenant access tokens.", Handler: s.handleAdminTenantTokenList, Admin: true},
		{Method: http.MethodPatch, Path: "/api/v1/admin/tenant-access-tokens/{tokenRef}", Tag: "Admin", Summary: "Modify a tenant access token.", Handler: s.handleAdminTenantTokenModify, Admin: true},
		{Method: http.MethodDelete, Path: "/api/v1/admin/tenant-access-tokens/{tokenRef}", Tag: "Admin", Summary: "Revoke a tenant access token.", Handler: s.handleAdminTenantTokenRevoke, Admin: true},
		{Method: http.MethodGet, Path: "/api/v1/credentials/current", Tag: "Credentials", Summary: "Get current credential metadata.", Handler: s.handleCredentialsCurrent},
		{Method: http.MethodGet, Path: "/api/v1/me", Tag: "Me", Summary: "Get current actor metadata.", Handler: s.handleMe},
		{Method: http.MethodPut, Path: "/api/v1/me/active_workspace", Tag: "Me", Summary: "Set active workspace.", Handler: s.handleMeActiveWorkspace},
		{Method: http.MethodGet, Path: "/api/v1/tasks", Tag: "Tasks", Summary: "List tasks.", Handler: s.handleTaskList},
		{Method: http.MethodPost, Path: "/api/v1/tasks", Tag: "Tasks", Summary: "Create a task.", Handler: s.handleTaskAdd, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}", Tag: "Tasks", Summary: "Get task details.", Handler: s.handleTaskInfo},
		{Method: http.MethodPatch, Path: "/api/v1/tasks/{taskRef}", Tag: "Tasks", Summary: "Modify a task.", Handler: s.handleTaskModify},
		{Method: http.MethodDelete, Path: "/api/v1/tasks/{taskRef}", Tag: "Tasks", Summary: "Delete a task.", Handler: s.handleTaskDelete},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/done", Tag: "Tasks", Summary: "Complete a task.", Handler: s.handleTaskDone},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/start", Tag: "Tasks", Summary: "Start a task.", Handler: s.handleTaskStart},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/stop", Tag: "Tasks", Summary: "Stop a task.", Handler: s.handleTaskStop},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/annotations", Tag: "Task Annotations", Summary: "Add a task annotation.", Handler: s.handleTaskAnnotate},
		{Method: http.MethodPatch, Path: "/api/v1/tasks/{taskRef}/annotations/{annotationID}", Tag: "Task Annotations", Summary: "Update a task annotation.", Handler: s.handleTaskAnnotationUpdate},
		{Method: http.MethodDelete, Path: "/api/v1/tasks/{taskRef}/annotations/{annotationID}", Tag: "Task Annotations", Summary: "Delete a task annotation.", Handler: s.handleTaskDenotate},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/annotations", Tag: "Task Annotations", Summary: "List task annotations.", Handler: s.handleTaskAnnotationList},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/urgency", Tag: "Tasks", Summary: "Explain task urgency.", Handler: s.handleTaskUrgency},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/links", Tag: "Task Links", Summary: "List task links.", Handler: s.handleTaskLinkList},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/links", Tag: "Task Links", Summary: "Add a task link.", Handler: s.handleTaskLinkAdd, Status: http.StatusCreated},
		{Method: http.MethodPatch, Path: "/api/v1/tasks/{taskRef}/links/{linkID}", Tag: "Task Links", Summary: "Update a task link.", Handler: s.handleTaskLinkUpdate},
		{Method: http.MethodDelete, Path: "/api/v1/tasks/{taskRef}/links/{linkID}", Tag: "Task Links", Summary: "Remove a task link.", Handler: s.handleTaskLinkRemove},
		{Method: http.MethodGet, Path: "/api/v1/reports/{name}", Tag: "Reports", Summary: "Run a report.", Handler: s.handleReport},
		{Method: http.MethodGet, Path: "/api/v1/users", Tag: "Users", Summary: "List users.", Handler: s.handleUserList},
		{Method: http.MethodPost, Path: "/api/v1/users", Tag: "Users", Summary: "Create a user.", Handler: s.handleUserCreate, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/users/{user}", Tag: "Users", Summary: "Get user details.", Handler: s.handleUserInfo},
		{Method: http.MethodPatch, Path: "/api/v1/users/{user}", Tag: "Users", Summary: "Update user display fields.", Handler: s.handleUserModify},
		{Method: http.MethodPost, Path: "/api/v1/users/{user}/external-ids", Tag: "Users", Summary: "Bind an external user ID.", Handler: s.handleExternalIDBind, Status: http.StatusCreated},
		{Method: http.MethodDelete, Path: "/api/v1/users/{user}/external-ids/{provider}/{externalID}", Tag: "Users", Summary: "Unbind an external user ID.", Handler: s.handleExternalIDUnbind},
		{Method: http.MethodGet, Path: "/api/v1/users/{user}/external-ids", Tag: "Users", Summary: "List external user IDs.", Handler: s.handleExternalIDList},
		{Method: http.MethodGet, Path: "/api/v1/workspaces", Tag: "Workspaces", Summary: "List workspaces.", Handler: s.handleWorkspaceList},
		{Method: http.MethodPost, Path: "/api/v1/workspaces", Tag: "Workspaces", Summary: "Create a workspace.", Handler: s.handleWorkspaceAdd, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/workspaces/{workspace}", Tag: "Workspaces", Summary: "Get workspace details.", Handler: s.handleWorkspaceInfo},
		{Method: http.MethodPatch, Path: "/api/v1/workspaces/{workspace}", Tag: "Workspaces", Summary: "Modify a workspace.", Handler: s.handleWorkspaceModify},
		{Method: http.MethodPost, Path: "/api/v1/workspaces/{workspace}/archive", Tag: "Workspaces", Summary: "Archive a workspace.", Handler: s.handleWorkspaceArchive},
		{Method: http.MethodGet, Path: "/api/v1/workspaces/{workspace}/members", Tag: "Members", Summary: "List workspace members.", Handler: s.handleMemberList},
		{Method: http.MethodPost, Path: "/api/v1/workspaces/{workspace}/members", Tag: "Members", Summary: "Add a workspace member.", Handler: s.handleMemberAdd, Status: http.StatusCreated},
		{Method: http.MethodPatch, Path: "/api/v1/workspaces/{workspace}/members/{user}", Tag: "Members", Summary: "Change a workspace member role.", Handler: s.handleMemberRole},
		{Method: http.MethodGet, Path: "/api/v1/workspaces/{workspace}/sso/config", Tag: "SSO", Summary: "读取 workspace SSO 配置（脱敏）", Handler: s.handleWorkspaceSsoConfigGet},
		{Method: http.MethodPut, Path: "/api/v1/workspaces/{workspace}/sso/config", Tag: "SSO", Summary: "写入 workspace SSO 配置", Handler: s.handleWorkspaceSsoConfigSet},
		{Method: http.MethodPost, Path: "/api/v1/workspaces/{workspace}/sso/sync", Tag: "SSO", Summary: "触发通讯录同步", Handler: s.handleWorkspaceSsoSync, Status: http.StatusAccepted},
		{Method: http.MethodGet, Path: "/api/v1/workspaces/{workspace}/sso/sync/jobs/{job_id}", Tag: "SSO", Summary: "查询同步任务状态", Handler: s.handleWorkspaceSsoSyncJob},
		{Method: http.MethodGet, Path: "/api/v1/projects", Tag: "Projects", Summary: "List projects.", Handler: s.handleProjectList},
		{Method: http.MethodPost, Path: "/api/v1/projects", Tag: "Projects", Summary: "Create a project.", Handler: s.handleProjectAdd, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}", Tag: "Projects", Summary: "Get project details.", Handler: s.handleProjectInfo},
		{Method: http.MethodPatch, Path: "/api/v1/projects/{projectRef}", Tag: "Projects", Summary: "Modify a project.", Handler: s.handleProjectModify},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/archive", Tag: "Projects", Summary: "Archive a project.", Handler: s.handleProjectArchive},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/transition", Tag: "Projects", Summary: "Transition a project.", Handler: s.handleProjectTransition},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/config", Tag: "Project Config", Summary: "List project config.", Handler: s.handleProjectConfigList},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/config/{key}", Tag: "Project Config", Summary: "Get project config.", Handler: s.handleProjectConfigGet},
		{Method: http.MethodPut, Path: "/api/v1/projects/{projectRef}/config/{key}", Tag: "Project Config", Summary: "Set project config.", Handler: s.handleProjectConfigSet},
		{Method: http.MethodDelete, Path: "/api/v1/projects/{projectRef}/config/{key}", Tag: "Project Config", Summary: "Unset project config.", Handler: s.handleProjectConfigUnset},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/annotations", Tag: "Project Annotations", Summary: "Add a project annotation.", Handler: s.handleProjectAnnotationAdd, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/annotations", Tag: "Project Annotations", Summary: "List project annotations.", Handler: s.handleProjectAnnotationList},
		{Method: http.MethodDelete, Path: "/api/v1/projects/{projectRef}/annotations/{annotationID}", Tag: "Project Annotations", Summary: "Delete a project annotation.", Handler: s.handleProjectAnnotationDelete},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/timeline", Tag: "Projects", Summary: "List project timeline.", Handler: s.handleProjectTimeline},
		{Method: http.MethodGet, Path: "/api/v1/contexts", Tag: "Contexts", Summary: "List contexts.", Handler: s.handleContextList},
		{Method: http.MethodPost, Path: "/api/v1/contexts", Tag: "Contexts", Summary: "Define a context.", Handler: s.handleContextDefine, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/contexts/none", Tag: "Contexts", Summary: "Clear active context.", Handler: s.handleContextNone},
		{Method: http.MethodGet, Path: "/api/v1/contexts/{name}", Tag: "Contexts", Summary: "Get context details.", Handler: s.handleContextInfo},
		{Method: http.MethodDelete, Path: "/api/v1/contexts/{name}", Tag: "Contexts", Summary: "Delete a context.", Handler: s.handleContextDelete},
		{Method: http.MethodPost, Path: "/api/v1/contexts/{name}/use", Tag: "Contexts", Summary: "Use a context.", Handler: s.handleContextUse},
		{Method: http.MethodGet, Path: "/api/v1/config", Tag: "Config", Summary: "List config.", Handler: s.handleConfigList},
		{Method: http.MethodGet, Path: "/api/v1/config/{key}", Tag: "Config", Summary: "Get config.", Handler: s.handleConfigGet},
		{Method: http.MethodPut, Path: "/api/v1/config/{key}", Tag: "Config", Summary: "Set config.", Handler: s.handleConfigSet},
		{Method: http.MethodDelete, Path: "/api/v1/config/{key}", Tag: "Config", Summary: "Unset config.", Handler: s.handleConfigUnset},
		{Method: http.MethodGet, Path: "/api/v1/config-schema", Tag: "Config Schema", Summary: "List config schema definitions.", Handler: s.handleConfigSchemaList},
		{Method: http.MethodGet, Path: "/api/v1/config-schema/{key}", Tag: "Config Schema", Summary: "Get a config schema definition.", Handler: s.handleConfigSchemaGet},
		{Method: http.MethodPut, Path: "/api/v1/config-schema/{key}", Tag: "Config Schema", Summary: "Set a config schema definition.", Handler: s.handleConfigSchemaSet},
		{Method: http.MethodDelete, Path: "/api/v1/config-schema/{key}", Tag: "Config Schema", Summary: "Delete a config schema definition.", Handler: s.handleConfigSchemaDelete},
		{Method: http.MethodGet, Path: "/api/v1/export", Tag: "Import Export", Summary: "Export tasks.", Handler: s.handleExport},
		{Method: http.MethodPost, Path: "/api/v1/import", Tag: "Import Export", Summary: "Import tasks.", Handler: s.handleImport},
		{Method: http.MethodGet, Path: "/api/v1/audit", Tag: "Audit", Summary: "List audit logs.", Handler: s.handleAuditList},
		{Method: http.MethodGet, Path: "/api/v1/tokens", Tag: "Tokens", Summary: "List API tokens.", Handler: s.handleTokenList},
		{Method: http.MethodPost, Path: "/api/v1/tokens", Tag: "Tokens", Summary: "Create an API token.", Handler: s.handleTokenCreate, Status: http.StatusCreated},
		{Method: http.MethodPatch, Path: "/api/v1/tokens/{tokenRef}", Tag: "Tokens", Summary: "Modify an API token.", Handler: s.handleTokenModify},
		{Method: http.MethodDelete, Path: "/api/v1/tokens/{tokenRef}", Tag: "Tokens", Summary: "Revoke an API token.", Handler: s.handleTokenRevoke},
		{Method: http.MethodGet, Path: "/api/v1/tenant-access-tokens", Tag: "Tenant Access Tokens", Summary: "List tenant access tokens.", Handler: s.handleTenantTokenList},
		{Method: http.MethodPost, Path: "/api/v1/tenant-access-tokens", Tag: "Tenant Access Tokens", Summary: "Create a tenant access token.", Handler: s.handleTenantTokenCreate, Status: http.StatusCreated},
		{Method: http.MethodPatch, Path: "/api/v1/tenant-access-tokens/{tokenRef}", Tag: "Tenant Access Tokens", Summary: "Modify a tenant access token.", Handler: s.handleTenantTokenModify},
		{Method: http.MethodDelete, Path: "/api/v1/tenant-access-tokens/{tokenRef}", Tag: "Tenant Access Tokens", Summary: "Revoke a tenant access token.", Handler: s.handleTenantTokenRevoke},
		{Method: http.MethodGet, Path: "/api/v1/hooks", Tag: "Hooks", Summary: "List hooks.", Handler: s.handleHookList},
		{Method: http.MethodPost, Path: "/api/v1/hooks", Tag: "Hooks", Summary: "Create a hook.", Handler: s.handleHookCreate, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/hooks/{hookID}", Tag: "Hooks", Summary: "Get hook details.", Handler: s.handleHookInfo},
		{Method: http.MethodPatch, Path: "/api/v1/hooks/{hookID}", Tag: "Hooks", Summary: "Modify a hook.", Handler: s.handleHookModify},
		{Method: http.MethodDelete, Path: "/api/v1/hooks/{hookID}", Tag: "Hooks", Summary: "Delete a hook.", Handler: s.handleHookDelete},
		{Method: http.MethodPost, Path: "/api/v1/hooks/{hookID}/enable", Tag: "Hooks", Summary: "Enable a hook.", Handler: s.handleHookEnable},
		{Method: http.MethodPost, Path: "/api/v1/hooks/{hookID}/disable", Tag: "Hooks", Summary: "Disable a hook.", Handler: s.handleHookDisable},
		{Method: http.MethodGet, Path: "/api/v1/hooks/{hookID}/deliveries", Tag: "Hooks", Summary: "List hook deliveries.", Handler: s.handleHookDeliveryList},
		{Method: http.MethodGet, Path: "/api/v1/hook-deliveries/{deliveryID}", Tag: "Hooks", Summary: "Get hook delivery details.", Handler: s.handleHookDeliveryInfo},
		{Method: http.MethodPost, Path: "/api/v1/hook-deliveries/{deliveryID}/replay", Tag: "Hooks", Summary: "Replay a hook delivery.", Handler: s.handleHookDeliveryReplay},
		{Method: http.MethodGet, Path: "/api/v1/notification-sinks", Tag: "Notification Sinks", Summary: "List notification sinks.", Handler: s.handleNotificationSinkList},
		{Method: http.MethodPost, Path: "/api/v1/notification-sinks", Tag: "Notification Sinks", Summary: "Create a notification sink.", Handler: s.handleNotificationSinkCreate, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/notification-sinks/{sinkID}", Tag: "Notification Sinks", Summary: "Get notification sink details.", Handler: s.handleNotificationSinkInfo},
		{Method: http.MethodPatch, Path: "/api/v1/notification-sinks/{sinkID}", Tag: "Notification Sinks", Summary: "Modify a notification sink.", Handler: s.handleNotificationSinkModify},
		{Method: http.MethodDelete, Path: "/api/v1/notification-sinks/{sinkID}", Tag: "Notification Sinks", Summary: "Delete a notification sink.", Handler: s.handleNotificationSinkDelete},
		{Method: http.MethodPost, Path: "/api/v1/notification-sinks/{sinkID}/enable", Tag: "Notification Sinks", Summary: "Enable a notification sink.", Handler: s.handleNotificationSinkEnable},
		{Method: http.MethodPost, Path: "/api/v1/notification-sinks/{sinkID}/disable", Tag: "Notification Sinks", Summary: "Disable a notification sink.", Handler: s.handleNotificationSinkDisable},
		{Method: http.MethodGet, Path: "/api/v1/reminder-rules", Tag: "Reminder Rules", Summary: "List reminder rules.", Handler: s.handleReminderRuleList},
		{Method: http.MethodPost, Path: "/api/v1/reminder-rules", Tag: "Reminder Rules", Summary: "Create a reminder rule.", Handler: s.handleReminderRuleCreate, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/reminder-rules/{ruleID}", Tag: "Reminder Rules", Summary: "Get reminder rule details.", Handler: s.handleReminderRuleInfo},
		{Method: http.MethodPatch, Path: "/api/v1/reminder-rules/{ruleID}", Tag: "Reminder Rules", Summary: "Modify a reminder rule.", Handler: s.handleReminderRuleModify},
		{Method: http.MethodPost, Path: "/api/v1/reminder-rules/{ruleID}/enable", Tag: "Reminder Rules", Summary: "Enable a reminder rule.", Handler: s.handleReminderRuleEnable},
		{Method: http.MethodPost, Path: "/api/v1/reminder-rules/{ruleID}/disable", Tag: "Reminder Rules", Summary: "Disable a reminder rule.", Handler: s.handleReminderRuleDisable},
		{Method: http.MethodDelete, Path: "/api/v1/reminder-rules/{ruleID}", Tag: "Reminder Rules", Summary: "Delete a reminder rule.", Handler: s.handleReminderRuleDelete},
		{Method: http.MethodGet, Path: "/api/v1/notification-rules", Tag: "Notification Rules", Summary: "List notification rules.", Handler: s.handleEventNotificationRuleList},
		{Method: http.MethodPost, Path: "/api/v1/notification-rules", Tag: "Notification Rules", Summary: "Create a notification rule.", Handler: s.handleEventNotificationRuleCreate, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/notification-rules/{ruleID}", Tag: "Notification Rules", Summary: "Get notification rule details.", Handler: s.handleEventNotificationRuleInfo},
		{Method: http.MethodPatch, Path: "/api/v1/notification-rules/{ruleID}", Tag: "Notification Rules", Summary: "Modify a notification rule.", Handler: s.handleEventNotificationRuleModify},
		{Method: http.MethodPost, Path: "/api/v1/notification-rules/{ruleID}/enable", Tag: "Notification Rules", Summary: "Enable a notification rule.", Handler: s.handleEventNotificationRuleEnable},
		{Method: http.MethodPost, Path: "/api/v1/notification-rules/{ruleID}/disable", Tag: "Notification Rules", Summary: "Disable a notification rule.", Handler: s.handleEventNotificationRuleDisable},
		{Method: http.MethodDelete, Path: "/api/v1/notification-rules/{ruleID}", Tag: "Notification Rules", Summary: "Delete a notification rule.", Handler: s.handleEventNotificationRuleDelete},
		{Method: http.MethodGet, Path: "/api/v1/notification-deliveries", Tag: "Notification Deliveries", Summary: "List notification deliveries.", Handler: s.handleNotificationDeliveryList},
		{Method: http.MethodGet, Path: "/api/v1/notification-deliveries/{deliveryID}", Tag: "Notification Deliveries", Summary: "Get notification delivery details.", Handler: s.handleNotificationDeliveryInfo},
		{Method: http.MethodPost, Path: "/api/v1/notification-deliveries/{deliveryID}/replay", Tag: "Notification Deliveries", Summary: "Replay a notification delivery.", Handler: s.handleNotificationDeliveryReplay},
	}
}
