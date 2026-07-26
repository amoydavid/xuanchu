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
		Parameters:  append(pathParameters(route.Path), contractQueryParameters(route)...),
		Responses: map[string]*huma.Response{
			defaultResponseStatus(route): contractSuccessResponse(route),
			"400":                        jsonResponse("Bad request."),
			"401":                        jsonResponse("Unauthorized."),
			"403":                        jsonResponse("Forbidden."),
			"404":                        jsonResponse("Not found."),
			"409":                        jsonResponse("Conflict."),
			"413":                        jsonResponse("Request payload too large."),
			"422":                        jsonResponse("Unprocessable entity."),
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
		requestSchema := contractRequestSchema(route)
		if requestSchema == nil {
			requestSchema = &huma.Schema{Type: "object"}
		}
		op.RequestBody = &huma.RequestBody{
			Description: "JSON request body.",
			Required:    route.Method != http.MethodPost || !strings.HasSuffix(route.Path, "/enable") && !strings.HasSuffix(route.Path, "/disable") && !strings.HasSuffix(route.Path, "/done") && !strings.HasSuffix(route.Path, "/start") && !strings.HasSuffix(route.Path, "/stop") && !strings.HasSuffix(route.Path, "/archive") && !strings.HasSuffix(route.Path, "/reactivate") && !strings.HasSuffix(route.Path, "/use") && !strings.HasSuffix(route.Path, "/none") && !strings.HasSuffix(route.Path, "/replay"),
			Content: map[string]*huma.MediaType{
				"application/json": {Schema: requestSchema},
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

func contractQueryParameters(route humaRoute) []*huma.Param {
	stringParam := func(name, description string) *huma.Param {
		return &huma.Param{Name: name, In: "query", Description: description, Schema: &huma.Schema{Type: "string"}}
	}
	enumParam := func(name, description string, values ...string) *huma.Param {
		enums := make([]any, 0, len(values))
		for _, value := range values {
			enums = append(enums, value)
		}
		return &huma.Param{Name: name, In: "query", Description: description, Schema: &huma.Schema{Type: "string", Enum: enums}}
	}
	limitParam := func() *huma.Param {
		minimum, maximum := float64(1), float64(taskListMaxLimit)
		return &huma.Param{Name: "limit", In: "query", Description: "Maximum page size; defaults to 200.", Schema: &huma.Schema{
			Type: "integer", Format: "int32", Default: taskListDefaultLimit, Minimum: &minimum, Maximum: &maximum,
		}}
	}
	offsetParam := func() *huma.Param {
		minimum := float64(0)
		return &huma.Param{Name: "offset", In: "query", Description: "Zero-based page offset; defaults to 0.", Schema: &huma.Schema{
			Type: "integer", Format: "int32", Default: 0, Minimum: &minimum,
		}}
	}
	boolParam := func(name, description string) *huma.Param {
		return &huma.Param{Name: name, In: "query", Description: description, Schema: &huma.Schema{Type: "boolean"}}
	}
	multiStringParam := func(name, description string) *huma.Param {
		explode := true
		return &huma.Param{Name: name, In: "query", Description: description, Style: "form", Explode: &explode, Schema: &huma.Schema{Type: "array", Items: &huma.Schema{Type: "string"}}}
	}
	refParam := func() *huma.Param {
		explode := true
		maximum := 100
		return &huma.Param{Name: "ref", In: "query", Description: "Stable candidate reference; may be repeated, at most 100 values.", Style: "form", Explode: &explode, Schema: &huma.Schema{Type: "array", Items: &huma.Schema{Type: "string"}, MaxItems: &maximum}}
	}
	workspaceScope := func() []*huma.Param {
		return []*huma.Param{
			stringParam("workspace", "Workspace slug or UUID."),
			stringParam("project", "Project slug in the effective workspace."),
			stringParam("project_id", "Stable project UUID."),
		}
	}
	templateWorkspace := func() *huma.Param {
		return &huma.Param{Name: "workspace", In: "query", Required: true, Description: "Explicit workspace slug or UUID.", Schema: &huma.Schema{Type: "string"}}
	}
	templateLimit := func() *huma.Param {
		minimum, maximum := float64(1), float64(100)
		return &huma.Param{Name: "limit", In: "query", Description: "Bounded page size; defaults to 50.", Schema: &huma.Schema{Type: "integer", Format: "int32", Default: 50, Minimum: &minimum, Maximum: &maximum}}
	}
	templateOffset := func() *huma.Param {
		minimum := float64(0)
		return &huma.Param{Name: "offset", In: "query", Description: "Zero-based page offset.", Schema: &huma.Schema{Type: "integer", Format: "int32", Default: 0, Minimum: &minimum}}
	}
	activityLimit := func() *huma.Param {
		minimum, maximum := float64(1), float64(taskActivityMaxLimit)
		return &huma.Param{Name: "limit", In: "query", Description: "Activity page size; defaults to 30.", Schema: &huma.Schema{
			Type: "integer", Format: "int32", Default: 30, Minimum: &minimum, Maximum: &maximum,
		}}
	}
	taskViewParams := func(includeReport bool) []*huma.Param {
		params := workspaceScope()
		if includeReport {
			params = append(params, stringParam("report", "Optional saved report name."))
		}
		return append(params,
			stringParam("query", "Task query expression; may be repeated."),
			stringParam("status", "Explicit task status filter."),
			stringParam("priority", "Task priority filter."),
			stringParam("assignee", "Workspace user reference."),
			stringParam("q", "Bare title search."),
			stringParam("tags", "Comma-separated required tags."),
			stringParam("due_after", "Inclusive local date in YYYY-MM-DD form."),
			stringParam("due_before", "Inclusive local date in YYYY-MM-DD form."),
			enumParam("occurrence_mode", "Occurrence projection mode.", "auto", "materialized", "expand"),
			enumParam("task_type", "Normal task or recurring occurrence filter.", "all", "normal", "occurrence"),
			stringParam("sort", "Task/report sort expression."),
			limitParam(),
			offsetParam(),
			boolParam("no_context", "Ignore the active CLI/report context."),
		)
	}

	switch {
	case strings.HasPrefix(route.Path, "/api/v1/projects/{projectRef}/template-candidates/"):
		params := []*huma.Param{templateWorkspace()}
		if route.Method == http.MethodPost {
			return params
		}
		params = append(
			params,
			stringParam("q", "Case-insensitive candidate search."),
			refParam(),
		)
		switch {
		case strings.HasSuffix(route.Path, "/tasks"):
			params = append(params,
				enumParam("status", "Task lifecycle status.", "pending", "waiting", "completed", "all"),
				enumParam("priority", "Task priority.", "H", "M", "L", "all"),
				multiStringParam("assignee", "Workspace user reference; may be repeated."),
				multiStringParam("tags", "Required tags; may be repeated."),
				stringParam("due_after", "Inclusive local date in YYYY-MM-DD form."),
				stringParam("due_before", "Inclusive local date in YYYY-MM-DD form."),
				stringParam("query", "Task query expression."),
				enumParam("sort", "Candidate ordering.", "urgency", "entry", "due", "wait", "completed"),
			)
		case strings.HasSuffix(route.Path, "/series"):
			params = append(params,
				enumParam("status", "Series lifecycle status.", "active", "ended", "stopped", "all"),
				stringParam("assignee", "Workspace user reference."),
				enumParam("sort", "Series ordering.", "next", "title", "modified"),
			)
		case strings.HasSuffix(route.Path, "/configs"):
			params = append(params, enumParam("mode", "Config value mode.", "all", "literal", "secret"))
		case strings.HasSuffix(route.Path, "/automations"):
			params = append(params,
				enumParam("status", "Automation enabled status.", "enabled", "disabled", "all"),
				enumParam("trigger_type", "Automation trigger type.", "schedule", "event", "all"),
			)
		}
		return append(params, templateLimit(), templateOffset())
	case strings.HasPrefix(route.Path, "/api/v1/project-templates"):
		params := []*huma.Param{templateWorkspace()}
		if route.Method == http.MethodGet && route.Path == "/api/v1/project-templates" {
			return append(params,
				enumParam("status", "Template lifecycle status.", "active", "archived", "all"),
				stringParam("q", "Case-insensitive template search."), templateLimit(), templateOffset(),
			)
		}
		if route.Method == http.MethodGet && route.Path == "/api/v1/project-templates/{templateRef}" {
			return append(params, &huma.Param{Name: "snapshot_id", In: "query", Description: "Historical snapshot UUID; defaults to current snapshot.", Schema: &huma.Schema{Type: "string", Format: "uuid"}})
		}
		return params
	case route.Method == http.MethodGet && route.Path == "/api/v1/tasks":
		return append(taskViewParams(true), boolParam("include_deleted", "Include deleted tasks when no explicit status predicate is supplied."))
	case route.Method == http.MethodGet && route.Path == "/api/v1/tasks/{taskRef}/activity":
		return []*huma.Param{
			stringParam("workspace", "Workspace slug or UUID."),
			activityLimit(),
			stringParam("cursor", "Opaque cursor returned by the previous activity page."),
		}
	case route.Method == http.MethodGet && route.Path == "/api/v1/reports/{name}":
		return taskViewParams(false)
	case route.Method == http.MethodGet && route.Path == "/api/v1/task-series":
		return append(workspaceScope(),
			enumParam("status", "Series lifecycle status.", "active", "ended", "stopped", "all"),
			stringParam("q", "Case-insensitive title/description search."),
			stringParam("assignee", "Workspace user reference."),
			enumParam("sort", "Series ordering.", "next", "title", "modified"),
			limitParam(),
			offsetParam(),
		)
	case route.Method == http.MethodGet && route.Path == "/api/v1/task-series/{seriesRef}/occurrences":
		return []*huma.Param{
			stringParam("workspace", "Workspace slug or UUID."),
			enumParam("status", "Occurrence task status.", "pending", "waiting", "completed", "deleted", "all"),
			stringParam("due_after", "Inclusive local date in YYYY-MM-DD form."),
			stringParam("due_before", "Inclusive local date in YYYY-MM-DD form."),
			limitParam(),
			offsetParam(),
		}
	case route.Method == http.MethodDelete && route.Path == "/api/v1/task-series/{seriesRef}":
		return []*huma.Param{
			stringParam("workspace", "Workspace slug or UUID."),
			boolParam("delete_open_occurrences", "Also skip all open and entered projected occurrences."),
		}
	case strings.HasPrefix(route.Path, "/api/v1/udas"):
		return []*huma.Param{stringParam("workspace", "Workspace slug or UUID.")}
	case strings.HasPrefix(route.Path, "/api/v1/task-series") || strings.HasPrefix(route.Path, "/api/v1/tasks/{taskRef}"):
		return []*huma.Param{stringParam("workspace", "Workspace slug or UUID.")}
	default:
		return nil
	}
}

func contractRequestSchema(route humaRoute) *huma.Schema {
	stringField := func() *huma.Schema { return &huma.Schema{Type: "string"} }
	nullableStringField := func() *huma.Schema { return &huma.Schema{Type: "string", Nullable: true} }
	int64Field := func() *huma.Schema { return &huma.Schema{Type: "integer", Format: "int64", Nullable: true} }
	dateField := func() *huma.Schema { return &huma.Schema{Type: "string", Format: "date"} }
	stringArrayField := func() *huma.Schema {
		return &huma.Schema{Type: "array", Items: &huma.Schema{Type: "string"}}
	}
	boolField := func() *huma.Schema { return &huma.Schema{Type: "boolean"} }
	udaField := func() *huma.Schema {
		return &huma.Schema{Type: "object", AdditionalProperties: &huma.Schema{Type: "string"}}
	}
	if schema := projectTemplateRequestOpenAPISchema(route); schema != nil {
		return schema
	}
	if route.Path == "/api/v1/udas/{name}" && route.Method == http.MethodPut {
		return &huma.Schema{Type: "object", AdditionalProperties: false, Properties: map[string]*huma.Schema{
			"type":    {Type: "string", Enum: []any{"string", "numeric", "date", "duration"}},
			"label":   {Type: "string"},
			"values":  {Type: "array", Items: &huma.Schema{Type: "string"}},
			"default": {Type: "string"},
		}, Required: []string{"type"}}
	}

	if route.Path == "/api/v1/tasks" && route.Method == http.MethodPost {
		return &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"title":                   stringField(),
				"description":             nullableStringField(),
				"project":                 stringField(),
				"project_id":              {Type: "string", Format: "uuid"},
				"priority":                nullableStringField(),
				"due":                     int64Field(),
				"due_date":                dateField(),
				"assignees":               stringArrayField(),
				"depends":                 stringArrayField(),
				"wait":                    int64Field(),
				"wait_date":               dateField(),
				"scheduled":               int64Field(),
				"scheduled_date":          dateField(),
				"until":                   int64Field(),
				"until_date":              dateField(),
				"tags":                    stringArrayField(),
				"udas":                    udaField(),
				"parent":                  stringField(),
				"attachment_draft_target": {Type: "string", Format: "uuid"},
			},
			Required: []string{"title"},
		}
	}
	if route.Path == "/api/v1/tasks/{taskRef}" && route.Method == http.MethodPatch {
		return &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"title":             nullableStringField(),
				"description":       nullableStringField(),
				"clear_description": boolField(),
				"project":           nullableStringField(),
				"project_id":        {Type: "string", Format: "uuid", Nullable: true},
				"clear_project":     boolField(),
				"priority":          nullableStringField(),
				"clear_priority":    boolField(),
				"due":               int64Field(),
				"due_date":          dateField(),
				"clear_due":         boolField(),
				"wait":              int64Field(),
				"wait_date":         dateField(),
				"clear_wait":        boolField(),
				"scheduled":         int64Field(),
				"scheduled_date":    dateField(),
				"clear_scheduled":   boolField(),
				"until":             int64Field(),
				"until_date":        dateField(),
				"clear_until":       boolField(),
				"assignees":         stringArrayField(),
				"remove_assignees":  stringArrayField(),
				"clear_assignees":   boolField(),
				"depends":           stringArrayField(),
				"clear_depends":     boolField(),
				"tags":              stringArrayField(),
				"remove_tags":       stringArrayField(),
				"udas":              udaField(),
				"clear_udas":        stringArrayField(),
			},
		}
	}
	if route.Path != "/api/v1/task-series" && route.Path != "/api/v1/task-series/{seriesRef}" {
		return nil
	}
	props := map[string]*huma.Schema{
		"title":               {Type: "string"},
		"description":         {Type: "string", Nullable: true},
		"project":             {Type: "string"},
		"project_id":          {Type: "string", Format: "uuid"},
		"recurrence_rule":     {Type: "string", Pattern: `^(daily|weekly|monthly|[1-9][0-9]*(days|weeks|months))$`},
		"first_due":           {Type: "integer", Format: "int64"},
		"first_due_date":      {Type: "string", Format: "date"},
		"until":               {Type: "integer", Format: "int64", Nullable: true},
		"until_date":          {Type: "string", Format: "date", Nullable: true},
		"priority":            {Type: "string", Nullable: true},
		"assignees":           {Type: "array", Items: &huma.Schema{Type: "string"}},
		"tags":                {Type: "array", Items: &huma.Schema{Type: "string"}},
		"udas":                {Type: "object", AdditionalProperties: &huma.Schema{Type: "string"}},
		"effective_from":      {Type: "integer", Format: "int64"},
		"effective_from_date": {Type: "string", Format: "date"},
		"clear":               {Type: "array", Items: &huma.Schema{Type: "string"}},
	}
	schema := &huma.Schema{Type: "object", Properties: props}
	if route.Method == http.MethodPost {
		schema.Required = []string{"title", "recurrence_rule"}
		schema.AllOf = []*huma.Schema{
			{AnyOf: []*huma.Schema{{Required: []string{"project"}}, {Required: []string{"project_id"}}}},
			{AnyOf: []*huma.Schema{{Required: []string{"first_due"}}, {Required: []string{"first_due_date"}}}},
		}
	}
	return schema
}

func contractSuccessResponse(route humaRoute) *huma.Response {
	data := (*huma.Schema)(nil)
	switch {
	case route.Path == "/api/v1/udas" && route.Method == http.MethodGet:
		data = &huma.Schema{Type: "array", Items: workspaceUDAOpenAPISchema()}
	case route.Path == "/api/v1/udas/{name}" && route.Method == http.MethodPut:
		data = workspaceUDAOpenAPISchema()
	case projectTemplateRoute(route):
		data = projectTemplateSuccessOpenAPISchema(route)
	case route.Method == http.MethodGet && route.Path == "/api/v1/home":
		data = homeOpenAPISchema()
	case route.Method == http.MethodGet && route.Path == "/api/v1/projects":
		data = &huma.Schema{Type: "array", Items: projectOpenAPISchema()}
	case projectRouteReturnsProjectView(route):
		data = projectOpenAPISchema()
	case route.Method == http.MethodGet && (route.Path == "/api/v1/tasks" || route.Path == "/api/v1/reports/{name}" || route.Path == "/api/v1/task-series/{seriesRef}/occurrences"):
		data = taskViewPageOpenAPISchema()
	case taskRouteReturnsOccurrenceView(route):
		data = taskOccurrenceOpenAPISchema()
	case route.Method == http.MethodGet && route.Path == "/api/v1/tasks/{taskRef}/activity":
		data = taskActivityPageOpenAPISchema()
	case route.Method == http.MethodGet && route.Path == "/api/v1/task-series":
		data = taskSeriesPageOpenAPISchema()
	case route.Method == http.MethodPost && route.Path == "/api/v1/task-series":
		data = &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"series":           taskSeriesOpenAPISchema(),
			"first_occurrence": {Type: "object", Nullable: true, Properties: taskOccurrenceOpenAPISchema().Properties},
		}, Required: []string{"series"}}
	case strings.HasPrefix(route.Path, "/api/v1/task-series/{seriesRef}"):
		data = taskSeriesOpenAPISchema()
	}
	if data == nil {
		return jsonResponse("Successful response.")
	}
	return jsonResponseWithSchema("Successful response.", successEnvelopeOpenAPISchema(data))
}

func workspaceUDAOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", AdditionalProperties: false, Properties: map[string]*huma.Schema{
		"name":                      {Type: "string"},
		"type":                      {Type: "string", Enum: []any{"string", "numeric", "date", "duration"}},
		"label":                     {Type: "string"},
		"values":                    {Type: "array", Items: &huma.Schema{Type: "string"}},
		"default":                   {Type: "string"},
		"source":                    {Type: "string", Enum: []any{"database", "runtime", "database_override"}},
		"task_value_count":          {Type: "integer", Format: "int64"},
		"active_series_value_count": {Type: "integer", Format: "int64"},
	}, Required: []string{"name", "type", "label", "values", "default", "source", "task_value_count", "active_series_value_count"}}
}

func homeOpenAPISchema() *huma.Schema {
	reasons := &huma.Schema{Type: "array", Items: &huma.Schema{Type: "string", Enum: []any{"started", "overdue", "due_today", "high_priority"}}}
	taskSchema := taskOccurrenceOpenAPISchema()
	myWork := &huma.Schema{Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
		"open_count": {Type: "integer", Format: "int32"}, "started_count": {Type: "integer", Format: "int32"},
		"overdue_count": {Type: "integer", Format: "int32"}, "due_today_count": {Type: "integer", Format: "int32"},
		"high_priority_open_count": {Type: "integer", Format: "int32"},
		"items": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"task": taskSchema, "reasons": reasons,
		}, Required: []string{"task", "reasons"}}},
	}, Required: []string{"open_count", "started_count", "overdue_count", "due_today_count", "high_priority_open_count", "items"}}
	project := projectOpenAPISchema()
	series := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"recurring_series_count": {Type: "integer", Format: "int32"}, "active_recurring_series_count": {Type: "integer", Format: "int32"},
		"open_recurring_occurrence_count": {Type: "integer", Format: "int32"}, "overdue_recurring_occurrence_count": {Type: "integer", Format: "int32"},
	}, Required: []string{"recurring_series_count", "active_recurring_series_count", "open_recurring_occurrence_count", "overdue_recurring_occurrence_count"}}
	latest := &huma.Schema{Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "project_id": {Type: "string"}, "entry": {Type: "integer", Format: "int64"},
		"content": {Type: "string"}, "created_by": actorInfoOpenAPISchema(), "created_at": {Type: "integer", Format: "int64"},
	}, Required: []string{"id", "project_id", "entry", "content", "created_by", "created_at"}}
	attention := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"project": project, "overdue_count": {Type: "integer", Format: "int32"},
		"high_priority_open_count": {Type: "integer", Format: "int32"}, "wait_ready_count": {Type: "integer", Format: "int32"},
		"unassigned_open_count": {Type: "integer", Format: "int32"}, "series_metrics": series, "latest_update": latest,
	}, Required: []string{"project", "overdue_count", "high_priority_open_count", "wait_ready_count", "unassigned_open_count", "series_metrics"}}
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"generated_at": {Type: "integer", Format: "int64"}, "today": {Type: "string", Format: "date"},
		"actor_type": {Type: "string"}, "my_work": myWork,
		"project_attention": {Type: "array", Items: attention},
	}, Required: []string{"generated_at", "today", "actor_type", "my_work", "project_attention"}}
}

func projectRouteReturnsProjectView(route humaRoute) bool {
	switch route.Path {
	case "/api/v1/projects":
		return route.Method == http.MethodPost
	case "/api/v1/projects/{projectRef}":
		return route.Method == http.MethodGet || route.Method == http.MethodPatch
	case "/api/v1/projects/{projectRef}/archive", "/api/v1/projects/{projectRef}/transition":
		return route.Method == http.MethodPost
	default:
		return false
	}
}

func projectOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id":              {Type: "string"},
		"workspace_id":    {Type: "string"},
		"slug":            {Type: "string"},
		"name":            {Type: "string"},
		"url":             {Type: "string", Description: "Web Console relative URL."},
		"description":     {Type: "string"},
		"status":          {Type: "string"},
		"task_count":      {Type: "integer", Format: "int32"},
		"pending_count":   {Type: "integer", Format: "int32"},
		"completed_count": {Type: "integer", Format: "int32"},
		"created_at":      {Type: "integer", Format: "int64"},
		"modified_at":     {Type: "integer", Format: "int64"},
		"archived_at":     {Type: "integer", Format: "int64", Nullable: true},
	}, Required: []string{
		"id", "workspace_id", "slug", "name", "url", "status", "task_count",
		"pending_count", "completed_count", "created_at", "modified_at",
	}}
}

func taskRouteReturnsOccurrenceView(route humaRoute) bool {
	if route.Path == "/api/v1/tasks" && route.Method == http.MethodPost {
		return true
	}
	if route.Path == "/api/v1/tasks/{taskRef}" {
		return route.Method == http.MethodGet || route.Method == http.MethodPatch || route.Method == http.MethodDelete
	}
	if route.Method != http.MethodPost {
		return false
	}
	switch route.Path {
	case "/api/v1/tasks/{taskRef}/done",
		"/api/v1/tasks/{taskRef}/start",
		"/api/v1/tasks/{taskRef}/stop",
		"/api/v1/tasks/{taskRef}/reopen":
		return true
	default:
		return false
	}
}

func jsonResponseWithSchema(description string, schema *huma.Schema) *huma.Response {
	return &huma.Response{Description: description, Content: map[string]*huma.MediaType{
		"application/json": {Schema: schema},
	}}
}

func successEnvelopeOpenAPISchema(data *huma.Schema) *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"data": data,
		"meta": {Type: "object", Nullable: true, AdditionalProperties: true},
	}, Required: []string{"data"}}
}

func taskViewPageOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"items":           {Type: "array", Items: taskOccurrenceOpenAPISchema()},
		"total":           {Type: "integer", Format: "int32"},
		"limit":           {Type: "integer", Format: "int32"},
		"offset":          {Type: "integer", Format: "int32"},
		"occurrence_mode": {Type: "string", Enum: []any{"auto", "materialized", "expand"}},
		"range": {Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
			"start": {Type: "integer", Format: "int64"}, "end": {Type: "integer", Format: "int64"},
		}, Required: []string{"start", "end"}},
	}, Required: []string{"items", "total", "limit", "offset", "occurrence_mode"}}
}

func externalIDOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"provider":    {Type: "string", Enum: []any{"feishu", "wecom", "dingtalk"}},
		"user_type":   {Type: "string", Enum: []any{"user_id", "open_id", "union_id"}},
		"external_id": {Type: "string"},
	}, Required: []string{"provider", "external_id"}}
}

func userInfoOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id":           {Type: "string"},
		"name":         {Type: "string"},
		"display_name": {Type: "string"},
		"email":        {Type: "string", Nullable: true},
		"external_ids": {Type: "array", Items: externalIDOpenAPISchema()},
	}, Required: []string{"id", "name"}}
}

func taskRefOpenAPISchema(nullable bool) *huma.Schema {
	return &huma.Schema{Type: "object", Nullable: nullable, Properties: map[string]*huma.Schema{
		"uuid":      {Type: "string", Format: "uuid"},
		"title":     {Type: "string"},
		"task_slug": {Type: "string", Nullable: true},
	}, Required: []string{"uuid", "title"}}
}

func actorInfoOpenAPISchema() *huma.Schema {
	token := &huma.Schema{Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "name": {Type: "string"}, "prefix": {Type: "string"},
	}, Required: []string{"id", "name"}}
	user := userInfoOpenAPISchema()
	user.Nullable = true
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"type":  {Type: "string", Description: "Actor kind, for example user or tenant_access_token."},
		"user":  user,
		"token": token,
	}, Required: []string{"type"}}
}

func taskActivityPageOpenAPISchema() *huma.Schema {
	annotation := &huma.Schema{Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "description": {Type: "string"},
	}, Required: []string{"id", "description"}}
	link := &huma.Schema{Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "type": {Type: "string"}, "url": {Type: "string"}, "title": {Type: "string"},
	}, Required: []string{"id", "type", "url", "title"}}
	entry := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "kind": {Type: "string", Enum: []any{"lifecycle", "change", "relation", "annotation"}},
		"action": {Type: "string"}, "actor": actorInfoOpenAPISchema(), "occurred_at": {Type: "string", Format: "date-time"},
		"changes": {Type: "array", Items: &huma.Schema{Type: "object"}}, "annotation": annotation, "link": link,
	}, Required: []string{"id", "kind", "action", "actor", "occurred_at"}}
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"entries": {Type: "array", Items: entry}, "next_cursor": {Type: "string", Nullable: true},
	}, Required: []string{"entries", "next_cursor"}}
}

func taskOccurrenceOpenAPISchema() *huma.Schema {
	nullString := func() *huma.Schema { return &huma.Schema{Type: "string", Nullable: true} }
	nullInt := func() *huma.Schema { return &huma.Schema{Type: "integer", Format: "int64", Nullable: true} }
	recurrence := &huma.Schema{Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
		"role":            {Type: "string", Enum: []any{"occurrence"}},
		"series_id":       {Type: "string", Format: "uuid"},
		"series_title":    {Type: "string"},
		"series_status":   {Type: "string", Enum: []any{"active", "ended", "stopped"}},
		"rule":            {Type: "string"},
		"recurrence_at":   {Type: "integer", Format: "int64"},
		"materialization": {Type: "string", Enum: []any{"projected", "materialized"}},
		"overrides":       {Type: "array", Items: &huma.Schema{Type: "string"}},
		"until":           nullInt(),
	}, Required: []string{"role", "series_id", "series_title", "series_status", "rule", "recurrence_at", "materialization"}}
	annotation := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "entry": {Type: "string", Format: "date-time"}, "description": {Type: "string"},
	}, Required: []string{"entry", "description"}}
	link := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "type": {Type: "string"}, "url": {Type: "string", Format: "uri"},
		"title": {Type: "string"}, "created_at": {Type: "string", Format: "date-time"}, "created_by": actorInfoOpenAPISchema(),
	}, Required: []string{"id", "type", "url", "created_at", "created_by"}}
	parentInfo := taskRefOpenAPISchema(true)
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id":              {Type: "string", Description: "UUID for a normal task; stable occurrence_ref for a recurring occurrence."},
		"url":             {Type: "string", Description: "Web Console relative URL."},
		"uuid":            nullString(),
		"task_slug":       nullString(),
		"project_seq":     nullInt(),
		"workspace_id":    {Type: "string", Format: "uuid"},
		"project_id":      nullString(),
		"project":         nullString(),
		"title":           {Type: "string"},
		"description":     nullString(),
		"status":          {Type: "string", Enum: []any{"pending", "waiting", "completed", "deleted"}},
		"entry":           nullInt(),
		"modified":        nullInt(),
		"due":             nullInt(),
		"start":           nullInt(),
		"end":             nullInt(),
		"wait":            nullInt(),
		"scheduled":       nullInt(),
		"until":           nullInt(),
		"parent":          nullString(),
		"priority":        nullString(),
		"tags":            {Type: "array", Items: &huma.Schema{Type: "string"}},
		"assignees":       {Type: "array", Items: userInfoOpenAPISchema()},
		"depends":         {Type: "array", Items: &huma.Schema{Type: "string"}},
		"depends_info":    {Type: "array", Items: taskRefOpenAPISchema(false)},
		"parent_info":     parentInfo,
		"blocked_by_info": {Type: "array", Items: taskRefOpenAPISchema(false)},
		"annotations":     {Type: "array", Items: annotation},
		"links":           {Type: "array", Items: link},
		"udas":            {Type: "object", AdditionalProperties: &huma.Schema{Type: "string"}},
		"recurrence_info": recurrence,
		"urgency":         {Type: "number", Format: "double", Nullable: true},
	}, Required: []string{"id", "url", "uuid", "task_slug", "project_seq", "workspace_id", "title", "status", "entry", "modified", "start", "end"}}
}

func taskSeriesOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id":                            {Type: "string", Format: "uuid"},
		"url":                           {Type: "string", Description: "Web Console relative URL."},
		"workspace_id":                  {Type: "string", Format: "uuid"},
		"project_id":                    {Type: "string", Format: "uuid"},
		"title":                         {Type: "string"},
		"description":                   {Type: "string", Nullable: true},
		"status":                        {Type: "string", Enum: []any{"active", "ended", "stopped"}},
		"recurrence_rule":               {Type: "string"},
		"first_due":                     {Type: "integer", Format: "int64"},
		"until":                         {Type: "integer", Format: "int64", Nullable: true},
		"priority":                      {Type: "string", Nullable: true},
		"assignees":                     {Type: "array", Items: userInfoOpenAPISchema()},
		"tags":                          {Type: "array", Items: &huma.Schema{Type: "string"}},
		"udas":                          {Type: "object", AdditionalProperties: &huma.Schema{Type: "string"}},
		"open_occurrence_count":         {Type: "integer", Format: "int32"},
		"completed_count":               {Type: "integer", Format: "int32"},
		"skipped_count":                 {Type: "integer", Format: "int32"},
		"overdue_count":                 {Type: "integer", Format: "int32"},
		"next_recurrence_at":            {Type: "integer", Format: "int64", Nullable: true},
		"suggested_rule_effective_from": {Type: "integer", Format: "int64", Nullable: true},
		"created_by":                    userInfoOpenAPISchema(),
		"created_at":                    {Type: "integer", Format: "int64"},
		"modified_at":                   {Type: "integer", Format: "int64"},
		"open_occurrences":              {Type: "array", Items: taskOccurrenceOpenAPISchema()},
		"recent_completed":              {Type: "array", Items: taskOccurrenceOpenAPISchema()},
		"recent_skipped":                {Type: "array", Items: taskOccurrenceOpenAPISchema()},
	}, Required: []string{
		"id", "url", "workspace_id", "project_id", "title", "status", "recurrence_rule", "first_due",
		"open_occurrence_count", "completed_count", "skipped_count", "overdue_count", "created_by", "created_at", "modified_at",
	}}
}

func taskSeriesPageOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"items":  {Type: "array", Items: taskSeriesOpenAPISchema()},
		"total":  {Type: "integer", Format: "int32"},
		"limit":  {Type: "integer", Format: "int32"},
		"offset": {Type: "integer", Format: "int32"},
	}, Required: []string{"items", "total", "limit", "offset"}}
}

func projectTemplateRoute(route humaRoute) bool {
	return strings.HasPrefix(route.Path, "/api/v1/project-templates") || strings.HasPrefix(route.Path, "/api/v1/projects/{projectRef}/template-candidates/")
}

func projectTemplateRequestOpenAPISchema(route humaRoute) *huma.Schema {
	if !projectTemplateRoute(route) {
		return nil
	}
	stringField := func() *huma.Schema { return &huma.Schema{Type: "string"} }
	stringArray := func() *huma.Schema { return &huma.Schema{Type: "array", Items: stringField()} }
	taskFilter := map[string]*huma.Schema{
		"q": stringField(), "status": stringField(), "priority": stringField(), "assignees": stringArray(), "tags": stringArray(),
		"due_after": {Type: "string", Format: "date"}, "due_before": {Type: "string", Format: "date"},
		"query": stringField(), "sort": stringField(),
	}
	if strings.HasSuffix(route.Path, "/template-candidates/resolve-selection") {
		variant := func(kind string, filter map[string]*huma.Schema) *huma.Schema {
			return &huma.Schema{Type: "object", AdditionalProperties: false, Properties: map[string]*huma.Schema{
				"kind": {Type: "string", Enum: []any{kind}}, kind: {Type: "object", AdditionalProperties: false, Properties: filter},
			}, Required: []string{"kind", kind}}
		}
		return &huma.Schema{OneOf: []*huma.Schema{
			variant("task", taskFilter),
			variant("series", map[string]*huma.Schema{"q": stringField(), "status": stringField(), "assignee": stringField(), "sort": stringField()}),
			variant("config", map[string]*huma.Schema{"q": stringField(), "mode": stringField()}),
			variant("automation", map[string]*huma.Schema{"q": stringField(), "status": stringField(), "trigger_type": stringField()}),
		}}
	}
	switch {
	case route.Path == "/api/v1/project-templates" && route.Method == http.MethodPost:
		return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"key": {Type: "string", Pattern: `^(?:[a-z][a-z0-9-]{2,31})?$`}, "name": stringField(), "description": stringField(), "capture": projectTemplateCaptureRequestOpenAPISchema(true),
		}, Required: []string{"name", "capture"}}
	case route.Path == "/api/v1/project-templates/{templateRef}" && route.Method == http.MethodPatch:
		return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{"name": stringField(), "description": stringField()}}
	case strings.HasSuffix(route.Path, "/instantiate-preview"):
		return projectTemplateInstantiateRequestOpenAPISchema(false)
	case strings.HasSuffix(route.Path, "/instantiate"):
		return projectTemplateInstantiateRequestOpenAPISchema(true)
	case strings.Contains(route.Path, "capture-preview"):
		return projectTemplateCaptureRequestOpenAPISchema(false)
	case strings.HasSuffix(route.Path, "/snapshots"):
		return projectTemplateCaptureRequestOpenAPISchema(true)
	default:
		return nil
	}
}

func projectTemplateCaptureRequestOpenAPISchema(requireHash bool) *huma.Schema {
	stringArray := func() *huma.Schema { return &huma.Schema{Type: "array", Items: &huma.Schema{Type: "string"}} }
	selection := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"config_keys": stringArray(), "task_refs": stringArray(), "series_refs": stringArray(), "automation_rule_ids": stringArray(),
	}, Required: []string{"config_keys", "task_refs", "series_refs", "automation_rule_ids"}}
	relativeTime := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"day_offset": {Type: "integer", Format: "int32"}, "local_time": {Type: "string", Pattern: `^(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]$`},
	}, Required: []string{"day_offset", "local_time"}}
	resolution := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"drop_parent_task_refs": stringArray(),
		"drop_depends": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"source_task_ref": {Type: "string"}, "relation": {Type: "string"}, "target_task_ref": {Type: "string"},
		}, Required: []string{"source_task_ref", "relation", "target_task_ref"}}},
		"drop_content_task_refs": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"source_kind": {Type: "string"}, "source_ref": {Type: "string"}, "target_task_ref": {Type: "string"},
		}, Required: []string{"source_kind", "source_ref", "target_task_ref"}}},
		"task_date_overrides": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"source_task_ref": {Type: "string"}, "field": {Type: "string"}, "value": relativeTime,
		}, Required: []string{"source_task_ref", "field"}}},
		"series_schedule_overrides": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"source_series_ref": {Type: "string"}, "first_due": relativeTime, "until": relativeTime, "clear_until": {Type: "boolean"},
		}, Required: []string{"source_series_ref", "first_due"}}},
	}}
	configPolicies := &huma.Schema{Type: "array", Items: &huma.Schema{Type: "object", AdditionalProperties: false, Properties: map[string]*huma.Schema{
		"key": {Type: "string"}, "strategy": {Type: "string", Enum: []any{"fixed", "inherit", "prompt"}}, "required": {Type: "boolean"},
	}, Required: []string{"key", "strategy"}}}
	schema := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"source_project": {Type: "string"}, "anchor_date": {Type: "string", Format: "date"}, "selection": selection,
		"config_policies": configPolicies, "resolution": resolution, "expected_source_hash": {Type: "string", Pattern: `^[a-f0-9]{64}$`},
	}, Required: []string{"source_project", "anchor_date", "selection"}}
	if requireHash {
		schema.Required = append(schema.Required, "expected_source_hash")
	}
	return schema
}

func projectTemplateInstantiateRequestOpenAPISchema(allowCurrentOnly bool) *huma.Schema {
	properties := map[string]*huma.Schema{
		"snapshot_id":            {Type: "string", Format: "uuid"},
		"expected_snapshot_hash": {Type: "string", Pattern: `^[a-f0-9]{64}$`},
		"project_slug":           {Type: "string"},
		"project_name":           {Type: "string"},
		"description":            {Type: "string", Nullable: true},
		"start_date":             {Type: "string", Format: "date"},
		"config_inputs":          {Type: "object", AdditionalProperties: &huma.Schema{Type: "string"}},
		"secret_inputs":          {Type: "object", AdditionalProperties: &huma.Schema{Type: "string"}},
		"assignee_replacements":  {Type: "object", AdditionalProperties: &huma.Schema{Type: "string", Nullable: true}},
	}
	if allowCurrentOnly {
		properties["current_only"] = &huma.Schema{Type: "boolean", Description: "要求 snapshot_id 仍为模板当前版本；仅供收窄的 Remote/CLI 客户端使用。"}
	}
	return &huma.Schema{Type: "object", Properties: properties, Required: []string{"expected_snapshot_hash", "project_slug", "project_name", "start_date"}}
}

func projectTemplateSuccessOpenAPISchema(route humaRoute) *huma.Schema {
	page := func(item *huma.Schema) *huma.Schema {
		return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"items": {Type: "array", Items: item}, "total": {Type: "integer", Format: "int64"},
			"limit": {Type: "integer", Format: "int32"}, "offset": {Type: "integer", Format: "int32"},
		}, Required: []string{"items", "total", "limit", "offset"}}
	}
	if strings.Contains(route.Path, "/template-candidates/") {
		if strings.HasSuffix(route.Path, "/resolve-selection") {
			return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
				"refs": {Type: "array", Items: &huma.Schema{Type: "string"}}, "total": {Type: "integer", Format: "int32"},
				"source_hash": {Type: "string", Pattern: `^[a-f0-9]{64}$`},
			}, Required: []string{"refs", "total", "source_hash"}}
		}
		var item *huma.Schema
		switch {
		case strings.HasSuffix(route.Path, "/tasks"):
			item = &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
				"ref": {Type: "string"}, "project_id": {Type: "string", Format: "uuid"}, "project_seq": {Type: "integer", Format: "int64", Nullable: true},
				"series_id": {Type: "string", Format: "uuid", Nullable: true}, "title": {Type: "string"}, "status": {Type: "string"},
				"priority": {Type: "string", Nullable: true}, "due": {Type: "integer", Format: "int64", Nullable: true},
				"assignees": {Type: "array", Items: userInfoOpenAPISchema()}, "warning_count": {Type: "integer", Format: "int32"},
			}, Required: []string{"ref", "project_id", "title", "status", "assignees", "warning_count"}}
		case strings.HasSuffix(route.Path, "/series"):
			item = &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
				"ref": {Type: "string"}, "project_id": {Type: "string", Format: "uuid"}, "project_seq": {Type: "integer", Format: "int64", Nullable: true},
				"title": {Type: "string"}, "status": {Type: "string"}, "recurrence_rule": {Type: "string"}, "first_due": {Type: "integer", Format: "int64"},
				"assignees": {Type: "array", Items: userInfoOpenAPISchema()}, "created_by": userInfoOpenAPISchema(), "warning_count": {Type: "integer", Format: "int32"},
			}, Required: []string{"ref", "project_id", "title", "status", "recurrence_rule", "first_due", "assignees", "created_by", "warning_count"}}
		case strings.HasSuffix(route.Path, "/configs"):
			item = &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
				"ref": {Type: "string"}, "key": {Type: "string"}, "label": {Type: "string"}, "mode": {Type: "string", Enum: []any{"literal", "secret"}},
				"value_type": {Type: "string"}, "secret": {Type: "boolean"}, "has_project_value": {Type: "boolean"},
				"effective_source": {Type: "string"}, "can_fixed": {Type: "boolean"}, "warning_count": {Type: "integer", Format: "int32"},
			}, Required: []string{"ref", "key", "label", "mode", "value_type", "secret", "has_project_value", "effective_source", "can_fixed", "warning_count"}}
		default:
			item = &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
				"ref": {Type: "string"}, "id": {Type: "string", Format: "uuid"}, "project_id": {Type: "string", Format: "uuid"},
				"name": {Type: "string"}, "description": {Type: "string"}, "enabled": {Type: "boolean"}, "trigger_type": {Type: "string"},
				"created_by": userInfoOpenAPISchema(), "created_at": {Type: "integer", Format: "int64"}, "warning_count": {Type: "integer", Format: "int32"},
			}, Required: []string{"ref", "id", "project_id", "name", "description", "enabled", "trigger_type", "created_by", "created_at", "warning_count"}}
		}
		return page(item)
	}
	switch {
	case route.Path == "/api/v1/project-templates" && route.Method == http.MethodGet:
		return page(projectTemplateSummaryOpenAPISchema())
	case strings.Contains(route.Path, "capture-preview"):
		return projectTemplateCapturePreviewOpenAPISchema()
	case strings.HasSuffix(route.Path, "/instantiate-preview"):
		return projectTemplateInstantiatePreviewOpenAPISchema()
	case strings.HasSuffix(route.Path, "/instantiate"):
		return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"project": projectOpenAPISchema(), "counts": projectTemplateCountsOpenAPISchema(),
		}, Required: []string{"project", "counts"}}
	default:
		return projectTemplateDetailOpenAPISchema()
	}
}

func projectTemplateCountsOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"configs": {Type: "integer", Format: "int32"}, "tasks": {Type: "integer", Format: "int32"},
		"series": {Type: "integer", Format: "int32"}, "automations": {Type: "integer", Format: "int32"},
	}, Required: []string{"configs", "tasks", "series", "automations"}}
}

func projectTemplateIssueOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"code": {Type: "string"}, "severity": {Type: "string", Enum: []any{"blocking", "warning"}},
		"component": {Type: "string"}, "source_kind": {Type: "string"}, "source_ref": {Type: "string"},
		"target_ref": {Type: "string"}, "relation": {Type: "string"}, "field": {Type: "string"},
		"message": {Type: "string"}, "user": userInfoOpenAPISchema(),
	}, Required: []string{"code", "message"}}
}

func projectTemplateSnapshotSummaryOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id": {Type: "string", Format: "uuid"}, "version": {Type: "integer", Format: "int64"},
		"hash": {Type: "string", Pattern: `^[a-f0-9]{64}$`}, "source_project_id": {Type: "string", Format: "uuid"},
		"counts": projectTemplateCountsOpenAPISchema(), "required_secret_keys": {Type: "array", Items: &huma.Schema{Type: "string"}},
		"config_inputs": {Type: "array", Items: projectTemplateConfigInputOpenAPISchema()},
		"created_by":    actorInfoOpenAPISchema(), "created_at": {Type: "integer", Format: "int64"},
	}, Required: []string{"id", "version", "hash", "source_project_id", "counts", "required_secret_keys", "config_inputs", "created_by", "created_at"}}
}

func projectTemplateConfigInputOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"key": {Type: "string"}, "label": {Type: "string"}, "description": {Type: "string"},
		"value_type":  {Type: "string", Enum: []any{"string", "number", "boolean", "json", "date", "datetime"}},
		"enum_values": {Type: "array", Items: &huma.Schema{Type: "string"}}, "required": {Type: "boolean"},
		"secret": {Type: "boolean"}, "status": {Type: "string", Enum: []any{"ready", "definition_missing", "scope_invalid"}},
	}, Required: []string{"key", "label", "description", "value_type", "enum_values", "required", "secret", "status"}}
}

func projectTemplateSummaryOpenAPISchema() *huma.Schema {
	current := projectTemplateSnapshotSummaryOpenAPISchema()
	current.Nullable = true
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id": {Type: "string", Format: "uuid"}, "key": {Type: "string"}, "name": {Type: "string"},
		"description": {Type: "string"}, "status": {Type: "string", Enum: []any{"active", "archived"}},
		"current_snapshot": current, "created_by": actorInfoOpenAPISchema(), "created_at": {Type: "integer", Format: "int64"},
		"modified_at": {Type: "integer", Format: "int64"}, "archived_at": {Type: "integer", Format: "int64", Nullable: true},
	}, Required: []string{"id", "key", "name", "description", "status", "created_by", "created_at", "modified_at"}}
}

func projectTemplatePublicSnapshotOpenAPISchema() *huma.Schema {
	userList := &huma.Schema{Type: "array", Items: userInfoOpenAPISchema()}
	stringsSchema := &huma.Schema{Type: "array", Items: &huma.Schema{Type: "string"}}
	relativeTime := func(nullable bool) *huma.Schema {
		return &huma.Schema{Type: "object", Nullable: nullable, Properties: map[string]*huma.Schema{
			"day_offset": {Type: "integer", Format: "int32"}, "local_time": {Type: "string", Pattern: `^(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]$`},
		}, Required: []string{"day_offset", "local_time"}}
	}
	udas := &huma.Schema{Type: "object", AdditionalProperties: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"raw": {Type: "string"}, "type": {Type: "string"},
	}, Required: []string{"raw"}}}
	dates := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"due": relativeTime(true), "wait": relativeTime(true), "scheduled": relativeTime(true), "until": relativeTime(true),
	}}
	taskItem := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"ref": {Type: "string"}, "title": {Type: "string"}, "description": {Type: "string", Nullable: true},
		"priority": {Type: "string", Nullable: true}, "tags": stringsSchema, "assignees": userList, "udas": udas, "dates": dates,
		"parent_ref": {Type: "string", Nullable: true}, "depends_refs": stringsSchema,
		"links": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"type": {Type: "string"}, "url": {Type: "string"}, "title": {Type: "string"},
		}, Required: []string{"type", "url"}}},
	}, Required: []string{"ref", "title", "tags", "assignees", "udas", "dates", "depends_refs", "links"}}
	seriesItem := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"ref": {Type: "string"}, "title": {Type: "string"}, "description": {Type: "string", Nullable: true}, "priority": {Type: "string", Nullable: true},
		"tags": stringsSchema, "assignees": userList, "udas": udas, "recurrence_rule": {Type: "string"}, "first_due": relativeTime(false), "until": relativeTime(true),
	}, Required: []string{"ref", "title", "tags", "assignees", "udas", "recurrence_rule", "first_due"}}
	automationItem := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"ref": {Type: "string"}, "name": {Type: "string"}, "description": {Type: "string"}, "trigger_type": {Type: "string"},
		"trigger_config": {Type: "object", Properties: map[string]*huma.Schema{
			"schedule_type": {Type: "string"}, "schedule_value": {Type: "string"}, "timezone": {Type: "string"}, "event_type": {Type: "string"},
		}},
		"condition": {Type: "object", Properties: map[string]*huma.Schema{
			"task_filter": {Type: "string"}, "max_tasks": {Type: "integer", Format: "int32"}, "only_added_assignees": {Type: "boolean"},
		}},
		"action": {Type: "object", Properties: map[string]*huma.Schema{
			"protocol": {Type: "string"}, "base_url_config_key": {Type: "string"}, "api_key_config_key": {Type: "string"}, "model_config_key": {Type: "string"},
			"allowed_hosts_config_key": {Type: "string"}, "model_override": {Type: "string"}, "temperature": {Type: "number", Format: "double"},
			"max_attempts": {Type: "integer", Format: "int32"}, "attach_metadata": {Type: "boolean"},
		}, Required: []string{"protocol", "base_url_config_key", "api_key_config_key", "model_config_key", "temperature"}},
		"context":              {Type: "object", Properties: map[string]*huma.Schema{"include": stringsSchema}, Required: []string{"include"}},
		"instruction_template": {Type: "string"}, "system_prompt": {Type: "string"},
	}, Required: []string{"ref", "name", "trigger_type", "trigger_config", "condition", "action", "context", "instruction_template"}}
	return &huma.Schema{Type: "object", Nullable: true, Properties: map[string]*huma.Schema{
		"project": {Type: "object", Properties: map[string]*huma.Schema{"description": {Type: "string"}}, Required: []string{"description"}},
		"configs": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"key": {Type: "string"}, "mode": {Type: "string", Enum: []any{"literal", "secret_input"}}, "value": {Type: "string", Nullable: true},
		}, Required: []string{"key", "mode"}}},
		"tasks": {Type: "array", Items: taskItem}, "series": {Type: "array", Items: seriesItem},
		"automations": {Type: "array", Items: automationItem},
	}, Required: []string{"project", "configs", "tasks", "series", "automations"}}
}

func projectTemplateDetailOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"template": projectTemplateSummaryOpenAPISchema(), "snapshot": projectTemplatePublicSnapshotOpenAPISchema(),
		"versions": {Type: "array", Items: projectTemplateSnapshotSummaryOpenAPISchema()},
	}, Required: []string{"template"}}
}

func projectTemplateCapturePreviewOpenAPISchema() *huma.Schema {
	selection := projectTemplateCaptureRequestOpenAPISchema(false).Properties["selection"]
	issues := &huma.Schema{Type: "array", Items: projectTemplateIssueOpenAPISchema()}
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"selection": selection, "source_hash": {Type: "string", Pattern: `^[a-f0-9]{64}$`},
		"required_config_keys": {Type: "array", Items: &huma.Schema{Type: "string"}},
		"counts":               projectTemplateCountsOpenAPISchema(), "blocking_issues": issues, "warnings": issues,
		"snapshot": projectTemplatePublicSnapshotOpenAPISchema(),
	}, Required: []string{"selection", "required_config_keys", "source_hash", "counts", "blocking_issues", "warnings"}}
}

func projectTemplateInstantiatePreviewOpenAPISchema() *huma.Schema {
	issues := &huma.Schema{Type: "array", Items: projectTemplateIssueOpenAPISchema()}
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"template": projectTemplateSummaryOpenAPISchema(), "snapshot": projectTemplateSnapshotSummaryOpenAPISchema(),
		"project": {Type: "object", Properties: map[string]*huma.Schema{
			"slug": {Type: "string"}, "name": {Type: "string"}, "description": {Type: "string"}, "start_date": {Type: "string", Format: "date"},
		}, Required: []string{"slug", "name", "description", "start_date"}},
		"counts":        projectTemplateCountsOpenAPISchema(),
		"config_inputs": {Type: "array", Items: projectTemplateConfigInputOpenAPISchema()},
		"config_resolutions": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"key": {Type: "string"}, "required": {Type: "boolean"}, "secret": {Type: "boolean"},
			"status": {Type: "string", Enum: []any{"provided", "omitted", "invalid"}},
		}, Required: []string{"key", "required", "secret", "status"}}},
		"secret_resolutions": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"key": {Type: "string"}, "resolved_from": {Type: "string", Enum: []any{"input", "workspace", "default", "missing", "template", "unavailable"}},
		}, Required: []string{"key", "resolved_from"}}},
		"assignee_issues": {Type: "array", Items: &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
			"user": userInfoOpenAPISchema(), "affected_refs": {Type: "array", Items: &huma.Schema{Type: "string"}}, "resolution": {Type: "string"},
		}, Required: []string{"user", "affected_refs", "resolution"}}},
		"issues": issues, "warnings": issues,
	}, Required: []string{"template", "snapshot", "project", "counts", "config_inputs", "config_resolutions", "secret_resolutions", "assignee_issues", "issues", "warnings"}}
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
		description := fmt.Sprintf("%s path parameter.", match[1])
		switch match[1] {
		case "taskRef":
			description = "Task UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref. Numeric working-set IDs are not accepted."
		case "occurrenceRef":
			description = "Stable occurrence_ref (occ:<series_uuid>:<recurrence_at_unix>), URL-encoded when used in a path."
		}
		params = append(params, &huma.Param{
			Name:        match[1],
			In:          "path",
			Required:    true,
			Description: description,
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
		{Method: http.MethodGet, Path: "/api/v1/sso/workspace", Tag: "SSO", Summary: "Get the sole OIDC-enabled workspace.", Handler: s.handleSsoWorkspace, Public: true},
		{Method: http.MethodGet, Path: "/sso/oidc/start", Tag: "SSO", Summary: "Start OIDC login flow.", Handler: s.handleSsoOidcStart, Public: true},
		{Method: http.MethodGet, Path: "/sso/oidc/callback", Tag: "SSO", Summary: "OIDC login callback.", Handler: s.handleSsoOidcCallback, Public: true},
		{Method: http.MethodPost, Path: "/auth/logout", Tag: "SSO", Summary: "Logout browser session.", Handler: s.handleAuthLogout, Public: true},
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
		{Method: http.MethodGet, Path: "/api/v1/home", Tag: "Home", Summary: "Get the current actor's Web Console home summary.", Handler: s.handleHome},
		{Method: http.MethodGet, Path: "/api/v1/me", Tag: "Me", Summary: "Get current actor metadata.", Handler: s.handleMe},
		{Method: http.MethodPut, Path: "/api/v1/me/active_workspace", Tag: "Me", Summary: "Set active workspace.", Handler: s.handleMeActiveWorkspace},
		{Method: http.MethodGet, Path: "/api/v1/tasks", Tag: "Tasks", Summary: "List tasks.", Handler: s.handleTaskList},
		{Method: http.MethodPost, Path: "/api/v1/tasks", Tag: "Tasks", Summary: "Create a task.", Handler: s.handleTaskAdd, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/task-drafts/{draftRef}/attachments", Tag: "Attachments", Summary: "Upload a private task-creation draft attachment.", Handler: s.handleTaskCreationDraftAttachmentUpload, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/task-drafts/{draftRef}/attachments/import-url", Tag: "Attachments", Summary: "Import a remote image into a private task-creation draft.", Handler: s.handleTaskCreationDraftAttachmentImportURL, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}", Tag: "Tasks", Summary: "Get task details.", Handler: s.handleTaskInfo},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/activity", Tag: "Tasks", Summary: "List task activity timeline.", Handler: s.handleTaskActivity},
		{Method: http.MethodPatch, Path: "/api/v1/tasks/{taskRef}", Tag: "Tasks", Summary: "Modify a task.", Handler: s.handleTaskModify},
		{Method: http.MethodDelete, Path: "/api/v1/tasks/{taskRef}", Tag: "Tasks", Summary: "Delete a task.", Handler: s.handleTaskDelete},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/done", Tag: "Tasks", Summary: "Complete a task.", Handler: s.handleTaskDone},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/start", Tag: "Tasks", Summary: "Start a task.", Handler: s.handleTaskStart},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/stop", Tag: "Tasks", Summary: "Stop a task.", Handler: s.handleTaskStop},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/reopen", Tag: "Tasks", Summary: "Reopen a completed task.", Handler: s.handleTaskReopen},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/annotations", Tag: "Task Annotations", Summary: "Add a task annotation.", Handler: s.handleTaskAnnotate},
		{Method: http.MethodPatch, Path: "/api/v1/tasks/{taskRef}/annotations/{annotationID}", Tag: "Task Annotations", Summary: "Update a task annotation.", Handler: s.handleTaskAnnotationUpdate},
		{Method: http.MethodDelete, Path: "/api/v1/tasks/{taskRef}/annotations/{annotationID}", Tag: "Task Annotations", Summary: "Delete a task annotation.", Handler: s.handleTaskDenotate},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/annotations", Tag: "Task Annotations", Summary: "List task annotations.", Handler: s.handleTaskAnnotationList},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/urgency", Tag: "Tasks", Summary: "Explain task urgency.", Handler: s.handleTaskUrgency},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/children", Tag: "Tasks", Summary: "List child tasks of a task.", Handler: s.handleTaskChildren},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/links", Tag: "Task Links", Summary: "List task links.", Handler: s.handleTaskLinkList},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/links", Tag: "Task Links", Summary: "Add a task link.", Handler: s.handleTaskLinkAdd, Status: http.StatusCreated},
		{Method: http.MethodPatch, Path: "/api/v1/tasks/{taskRef}/links/{linkID}", Tag: "Task Links", Summary: "Update a task link.", Handler: s.handleTaskLinkUpdate},
		{Method: http.MethodDelete, Path: "/api/v1/tasks/{taskRef}/links/{linkID}", Tag: "Task Links", Summary: "Remove a task link.", Handler: s.handleTaskLinkRemove},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/attachments", Tag: "Attachments", Summary: "Upload a task attachment.", Handler: s.handleAttachmentUpload, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/attachments/import-url", Tag: "Attachments", Summary: "Import a remote image as an attachment.", Handler: s.handleAttachmentImportURL, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/attachments", Tag: "Attachments", Summary: "List task attachments.", Handler: s.handleAttachmentList},
		{Method: http.MethodGet, Path: "/api/v1/attachments/{attachmentID}", Tag: "Attachments", Summary: "Get attachment metadata.", Handler: s.handleAttachmentGet},
		{Method: http.MethodGet, Path: "/api/v1/attachments/{attachmentID}/content", Tag: "Attachments", Summary: "Download attachment content.", Handler: s.handleAttachmentContent},
		{Method: http.MethodPatch, Path: "/api/v1/attachments/{attachmentID}", Tag: "Attachments", Summary: "Rename an attachment.", Handler: s.handleAttachmentRename},
		{Method: http.MethodDelete, Path: "/api/v1/attachments/{attachmentID}", Tag: "Attachments", Summary: "Remove an attachment.", Handler: s.handleAttachmentRemove},
		{Method: http.MethodGet, Path: "/api/v1/content-references/suggestions", Tag: "Content References", Summary: "Suggest users or tasks for content references.", Handler: s.handleContentReferenceSuggestions},
		{Method: http.MethodPost, Path: "/api/v1/content-references/resolve", Tag: "Content References", Summary: "Batch resolve content references.", Handler: s.handleContentReferenceResolve},
		{Method: http.MethodGet, Path: "/api/v1/task-series", Tag: "Task Series", Summary: "List recurring task series.", Handler: s.handleTaskSeriesList},
		{Method: http.MethodPost, Path: "/api/v1/task-series", Tag: "Task Series", Summary: "Create a recurring task series.", Handler: s.handleTaskSeriesAdd, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/task-series/{seriesRef}", Tag: "Task Series", Summary: "Get series details.", Handler: s.handleTaskSeriesGet},
		{Method: http.MethodPatch, Path: "/api/v1/task-series/{seriesRef}", Tag: "Task Series", Summary: "Modify a series.", Handler: s.handleTaskSeriesModify},
		{Method: http.MethodDelete, Path: "/api/v1/task-series/{seriesRef}", Tag: "Task Series", Summary: "Stop a series.", Handler: s.handleTaskSeriesDelete},
		{Method: http.MethodGet, Path: "/api/v1/task-series/{seriesRef}/occurrences", Tag: "Task Series", Summary: "List series occurrences.", Handler: s.handleTaskSeriesOccurrencesList},
		{Method: http.MethodPost, Path: "/api/v1/task-series/{seriesRef}/occurrences/{occurrenceRef}/skip", Tag: "Task Series", Summary: "Skip an occurrence.", Handler: s.handleTaskSeriesOccurrenceSkip},
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
		{Method: http.MethodDelete, Path: "/api/v1/workspaces/{workspace}/members/{user}", Tag: "Members", Summary: "Remove a workspace member.", Handler: s.handleMemberDelete},
		{Method: http.MethodGet, Path: "/api/v1/workspaces/{workspace}/sso/config", Tag: "SSO", Summary: "读取 workspace SSO 配置（脱敏）", Handler: s.handleWorkspaceSsoConfigGet},
		{Method: http.MethodPut, Path: "/api/v1/workspaces/{workspace}/sso/config", Tag: "SSO", Summary: "写入 workspace SSO 配置", Handler: s.handleWorkspaceSsoConfigSet},
		{Method: http.MethodPost, Path: "/api/v1/workspaces/{workspace}/sso/sync", Tag: "SSO", Summary: "触发通讯录同步", Handler: s.handleWorkspaceSsoSync},
		{Method: http.MethodGet, Path: "/api/v1/workspaces/{workspace}/sso/sync/jobs/{job_id}", Tag: "SSO", Summary: "查询同步任务状态", Handler: s.handleWorkspaceSsoSyncJob},
		{Method: http.MethodGet, Path: "/api/v1/projects", Tag: "Projects", Summary: "List projects.", Handler: s.handleProjectList},
		{Method: http.MethodPost, Path: "/api/v1/projects", Tag: "Projects", Summary: "Create a project.", Handler: s.handleProjectAdd, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}", Tag: "Projects", Summary: "Get project details.", Handler: s.handleProjectInfo},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/task-summary", Tag: "Projects", Summary: "Get project task summary.", Handler: s.handleProjectTaskSummary},
		{Method: http.MethodPatch, Path: "/api/v1/projects/{projectRef}", Tag: "Projects", Summary: "Modify a project.", Handler: s.handleProjectModify},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/archive", Tag: "Projects", Summary: "Archive a project.", Handler: s.handleProjectArchive},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/transition", Tag: "Projects", Summary: "Transition a project.", Handler: s.handleProjectTransition},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/config", Tag: "Project Config", Summary: "List project config.", Handler: s.handleProjectConfigList},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/config/effective", Tag: "Project Config", Summary: "List effective project config.", Handler: s.handleProjectConfigEffective},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/config/{key}", Tag: "Project Config", Summary: "Get project config.", Handler: s.handleProjectConfigGet},
		{Method: http.MethodPut, Path: "/api/v1/projects/{projectRef}/config/{key}", Tag: "Project Config", Summary: "Set project config.", Handler: s.handleProjectConfigSet},
		{Method: http.MethodDelete, Path: "/api/v1/projects/{projectRef}/config/{key}", Tag: "Project Config", Summary: "Unset project config.", Handler: s.handleProjectConfigUnset},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/annotations", Tag: "Project Annotations", Summary: "Add a project annotation.", Handler: s.handleProjectAnnotationAdd, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/annotations", Tag: "Project Annotations", Summary: "List project annotations.", Handler: s.handleProjectAnnotationList},
		{Method: http.MethodDelete, Path: "/api/v1/projects/{projectRef}/annotations/{annotationID}", Tag: "Project Annotations", Summary: "Delete a project annotation.", Handler: s.handleProjectAnnotationDelete},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/timeline", Tag: "Projects", Summary: "List project timeline.", Handler: s.handleProjectTimeline},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/template-candidates/tasks", Tag: "Project Templates", Summary: "List bounded task candidates for a project template capture.", Handler: s.handleProjectTemplateTaskCandidates},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/template-candidates/series", Tag: "Project Templates", Summary: "List bounded series candidates for a project template capture.", Handler: s.handleProjectTemplateSeriesCandidates},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/template-candidates/configs", Tag: "Project Templates", Summary: "List bounded config candidates for a project template capture.", Handler: s.handleProjectTemplateConfigCandidates},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/template-candidates/automations", Tag: "Project Templates", Summary: "List bounded automation candidates for a project template capture.", Handler: s.handleProjectTemplateAutomationCandidates},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/template-candidates/resolve-selection", Tag: "Project Templates", Summary: "Resolve a candidate filter to explicit stable references.", Handler: s.handleProjectTemplateResolveSelection},
		{Method: http.MethodPost, Path: "/api/v1/project-templates/capture-preview", Tag: "Project Templates", Summary: "Preview a project template capture.", Handler: s.handleProjectTemplateCapturePreview},
		{Method: http.MethodPost, Path: "/api/v1/project-templates", Tag: "Project Templates", Summary: "Save a project template, creating it or appending its current snapshot.", Handler: s.handleProjectTemplateCreate, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/project-templates", Tag: "Project Templates", Summary: "List project template metadata.", Handler: s.handleProjectTemplateList},
		{Method: http.MethodGet, Path: "/api/v1/project-templates/{templateRef}", Tag: "Project Templates", Summary: "Get project template detail and a selected snapshot.", Handler: s.handleProjectTemplateInfo},
		{Method: http.MethodPatch, Path: "/api/v1/project-templates/{templateRef}", Tag: "Project Templates", Summary: "Modify project template metadata.", Handler: s.handleProjectTemplateModify},
		{Method: http.MethodPost, Path: "/api/v1/project-templates/{templateRef}/archive", Tag: "Project Templates", Summary: "Archive a project template.", Handler: s.handleProjectTemplateArchive},
		{Method: http.MethodPost, Path: "/api/v1/project-templates/{templateRef}/reactivate", Tag: "Project Templates", Summary: "Reactivate a project template.", Handler: s.handleProjectTemplateReactivate},
		{Method: http.MethodPost, Path: "/api/v1/project-templates/{templateRef}/snapshots/capture-preview", Tag: "Project Templates", Summary: "Preview a new immutable template snapshot.", Handler: s.handleProjectTemplateSnapshotCapturePreview},
		{Method: http.MethodPost, Path: "/api/v1/project-templates/{templateRef}/snapshots", Tag: "Project Templates", Summary: "Append an immutable template snapshot.", Handler: s.handleProjectTemplateSnapshotCreate, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/project-templates/{templateRef}/instantiate-preview", Tag: "Project Templates", Summary: "Preview instantiation from a selected snapshot.", Handler: s.handleProjectTemplateInstantiatePreview},
		{Method: http.MethodPost, Path: "/api/v1/project-templates/{templateRef}/instantiate", Tag: "Project Templates", Summary: "Instantiate a project from a selected snapshot.", Handler: s.handleProjectTemplateInstantiate, Status: http.StatusCreated},
		{Method: http.MethodGet, Path: "/api/v1/contexts", Tag: "Contexts", Summary: "List contexts.", Handler: s.handleContextList},
		{Method: http.MethodPost, Path: "/api/v1/contexts", Tag: "Contexts", Summary: "Define a context.", Handler: s.handleContextDefine, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/contexts/none", Tag: "Contexts", Summary: "Clear active context.", Handler: s.handleContextNone},
		{Method: http.MethodGet, Path: "/api/v1/contexts/{name}", Tag: "Contexts", Summary: "Get context details.", Handler: s.handleContextInfo},
		{Method: http.MethodDelete, Path: "/api/v1/contexts/{name}", Tag: "Contexts", Summary: "Delete a context.", Handler: s.handleContextDelete},
		{Method: http.MethodPost, Path: "/api/v1/contexts/{name}/use", Tag: "Contexts", Summary: "Use a context.", Handler: s.handleContextUse},
		{Method: http.MethodGet, Path: "/api/v1/config", Tag: "Config", Summary: "List config.", Handler: s.handleConfigList},
		{Method: http.MethodGet, Path: "/api/v1/config/effective", Tag: "Config", Summary: "List effective workspace config.", Handler: s.handleConfigEffective},
		{Method: http.MethodGet, Path: "/api/v1/config/{key}", Tag: "Config", Summary: "Get config.", Handler: s.handleConfigGet},
		{Method: http.MethodPut, Path: "/api/v1/config/{key}", Tag: "Config", Summary: "Set config.", Handler: s.handleConfigSet},
		{Method: http.MethodDelete, Path: "/api/v1/config/{key}", Tag: "Config", Summary: "Unset config.", Handler: s.handleConfigUnset},
		{Method: http.MethodGet, Path: "/api/v1/config-schema", Tag: "Config Schema", Summary: "List config schema definitions.", Handler: s.handleConfigSchemaList},
		{Method: http.MethodGet, Path: "/api/v1/config-schema/{key}/usage", Tag: "Config Schema", Summary: "Get config schema usage.", Handler: s.handleConfigSchemaUsage},
		{Method: http.MethodGet, Path: "/api/v1/config-schema/{key}", Tag: "Config Schema", Summary: "Get a config schema definition.", Handler: s.handleConfigSchemaGet},
		{Method: http.MethodPut, Path: "/api/v1/config-schema/{key}", Tag: "Config Schema", Summary: "Set a config schema definition.", Handler: s.handleConfigSchemaSet},
		{Method: http.MethodDelete, Path: "/api/v1/config-schema/{key}", Tag: "Config Schema", Summary: "Delete a config schema definition.", Handler: s.handleConfigSchemaDelete},
		{Method: http.MethodGet, Path: "/api/v1/udas", Tag: "Custom Fields", Summary: "List Workspace custom field definitions.", Handler: s.handleWorkspaceUDAList},
		{Method: http.MethodPut, Path: "/api/v1/udas/{name}", Tag: "Custom Fields", Summary: "Create or replace a Workspace custom field definition.", Handler: s.handleWorkspaceUDASet},
		{Method: http.MethodDelete, Path: "/api/v1/udas/{name}", Tag: "Custom Fields", Summary: "Delete a Workspace custom field definition.", Handler: s.handleWorkspaceUDADelete},
		{Method: http.MethodGet, Path: "/api/v1/export", Tag: "Import Export", Summary: "Export tasks.", Handler: s.handleExport},
		{Method: http.MethodPost, Path: "/api/v1/import", Tag: "Import Export", Summary: "Import tasks.", Handler: s.handleImport},
		{Method: http.MethodPost, Path: "/api/v1/task-imports", Tag: "Import Export", Summary: "Import ordinary tasks for Web Console.", Handler: s.handleOrdinaryTaskImport},
		{Method: http.MethodGet, Path: "/api/v1/audit", Tag: "Audit", Summary: "List audit logs.", Handler: s.handleAuditList},
		{Method: http.MethodGet, Path: "/api/v1/tokens", Tag: "Tokens", Summary: "List API tokens.", Handler: s.handleTokenList},
		{Method: http.MethodPost, Path: "/api/v1/tokens", Tag: "Tokens", Summary: "Create an API token.", Handler: s.handleTokenCreate, Status: http.StatusCreated},
		{Method: http.MethodPatch, Path: "/api/v1/tokens/{tokenRef}", Tag: "Tokens", Summary: "Modify an API token.", Handler: s.handleTokenModify},
		{Method: http.MethodDelete, Path: "/api/v1/tokens/{tokenRef}", Tag: "Tokens", Summary: "Revoke an API token.", Handler: s.handleTokenRevoke},
		{Method: http.MethodGet, Path: "/api/v1/tokens/{tokenRef}/mcp-config", Tag: "Tokens", Summary: "Reveal MCP configuration for an API token.", Handler: s.handleTokenMCPConfig},
		{Method: http.MethodGet, Path: "/api/v1/tenant-access-tokens", Tag: "Tenant Access Tokens", Summary: "List tenant access tokens.", Handler: s.handleTenantTokenList},
		{Method: http.MethodPost, Path: "/api/v1/tenant-access-tokens", Tag: "Tenant Access Tokens", Summary: "Create a tenant access token.", Handler: s.handleTenantTokenCreate, Status: http.StatusCreated},
		{Method: http.MethodPatch, Path: "/api/v1/tenant-access-tokens/{tokenRef}", Tag: "Tenant Access Tokens", Summary: "Modify a tenant access token.", Handler: s.handleTenantTokenModify},
		{Method: http.MethodDelete, Path: "/api/v1/tenant-access-tokens/{tokenRef}", Tag: "Tenant Access Tokens", Summary: "Revoke a tenant access token.", Handler: s.handleTenantTokenRevoke},
		{Method: http.MethodGet, Path: "/api/v1/tenant-access-tokens/{tokenRef}/mcp-config", Tag: "Tenant Access Tokens", Summary: "Reveal MCP configuration for a tenant access token.", Handler: s.handleTenantTokenMCPConfig},
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
		{Method: http.MethodPost, Path: "/api/v1/notification-sinks/{sinkID}/test", Tag: "Notification Sinks", Summary: "Test a notification sink by sending a sample delivery.", Handler: s.handleNotificationSinkTest},
		{Method: http.MethodGet, Path: "/api/v1/notification-template-vars", Tag: "Notification Sinks", Summary: "List available notification template variables.", Handler: s.handleNotificationTemplateVars},
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
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automations", Tag: "Project Automations", Summary: "List project automations.", Handler: s.handleProjectAutomationList},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations", Tag: "Project Automations", Summary: "Create a project automation.", Handler: s.handleProjectAutomationCreate, Status: http.StatusCreated},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/preview", Tag: "Project Automations", Summary: "Preview a project automation delivery.", Handler: s.handleProjectAutomationPreview},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/preview", Tag: "Project Automations", Summary: "Preview a saved project automation delivery.", Handler: s.handleProjectAutomationSavedPreview},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}", Tag: "Project Automations", Summary: "Get project automation details.", Handler: s.handleProjectAutomationInfo},
		{Method: http.MethodPatch, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}", Tag: "Project Automations", Summary: "Modify a project automation.", Handler: s.handleProjectAutomationModify},
		{Method: http.MethodDelete, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}", Tag: "Project Automations", Summary: "Delete a project automation.", Handler: s.handleProjectAutomationDelete},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/enable", Tag: "Project Automations", Summary: "Enable a project automation.", Handler: s.handleProjectAutomationEnable},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/disable", Tag: "Project Automations", Summary: "Disable a project automation.", Handler: s.handleProjectAutomationDisable},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/test", Tag: "Project Automations", Summary: "Test a project automation.", Handler: s.handleProjectAutomationTest},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automation-template-vars", Tag: "Project Automations", Summary: "List available automation template variables.", Handler: s.handleProjectAutomationTemplateVars},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automation-deliveries", Tag: "Project Automations", Summary: "List project automation deliveries.", Handler: s.handleProjectAutomationDeliveryList},
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automation-deliveries/{deliveryID}", Tag: "Project Automations", Summary: "Get project automation delivery details.", Handler: s.handleProjectAutomationDeliveryInfo},
		{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automation-deliveries/{deliveryID}/replay", Tag: "Project Automations", Summary: "Replay project automation delivery.", Handler: s.handleProjectAutomationDeliveryReplay},
	}
}
