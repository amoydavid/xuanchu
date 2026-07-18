package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func openHTTPTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func assertHTTPErrorCode(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, wantStatus, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"`+wantCode+`"`) {
		t.Fatalf("body = %s, want error code %q", rr.Body.String(), wantCode)
	}
}

var pascalCaseJSONKey = regexp.MustCompile(`"[A-Z][A-Za-z]*":`)

func assertSnakeCaseResponse(t *testing.T, body string) {
	t.Helper()
	if key := pascalCaseJSONKey.FindString(body); key != "" {
		t.Fatalf("response leaked PascalCase key %q in body=%s", key, body)
	}
}

func TestHealthzIsAnonymous(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "schema") || strings.Contains(rr.Body.String(), "database") {
		t.Fatalf("healthz leaked details: %s", rr.Body.String())
	}
}

func TestOpenAPIIsGeneratedFromRegisteredHTTPRoutes(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var doc struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"info"`
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid OpenAPI JSON: %v body=%s", err, rr.Body.String())
	}
	if doc.OpenAPI == "" || doc.Info.Title != "Xuanchu HTTP API" || doc.Info.Version != "v1" {
		t.Fatalf("unexpected OpenAPI metadata: %#v", doc)
	}
	for _, path := range []string{
		"/api/v1/tasks/{taskRef}/links",
		"/api/v1/tasks/{taskRef}/audit",
		"/api/v1/config-schema/{key}",
		"/api/v1/projects/{projectRef}/timeline",
	} {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("OpenAPI paths missing %s", path)
		}
	}
	if _, ok := doc.Paths["/mcp"]; ok {
		t.Fatalf("OpenAPI unexpectedly documented /mcp")
	}
	if _, ok := doc.Paths["/api/v1/__panic"]; ok {
		t.Fatalf("OpenAPI unexpectedly documented test panic route")
	}
}

func TestOpenAPIIncludesEveryRegisteredHTTPRoute(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var doc struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid OpenAPI JSON: %v body=%s", err, rr.Body.String())
	}
	for _, route := range srv.humaRoutes() {
		pathItem, ok := doc.Paths[route.Path]
		if !ok {
			t.Fatalf("OpenAPI paths missing %s", route.Path)
		}
		if _, ok := pathItem[strings.ToLower(route.Method)]; !ok {
			t.Fatalf("OpenAPI path %s missing method %s", route.Path, route.Method)
		}
	}
	if got, want := countOpenAPIOperations(doc.Paths), len(srv.humaRoutes()); got != want {
		t.Fatalf("OpenAPI operation count = %d, want %d", got, want)
	}
}

func TestOpenAPIDocumentsTaskSeriesAndOccurrenceContracts(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	operation := func(path, method string) map[string]any {
		t.Helper()
		return paths[path].(map[string]any)[method].(map[string]any)
	}
	parameter := func(op map[string]any, name string) map[string]any {
		t.Helper()
		rawParameters, ok := op["parameters"].([]any)
		if !ok {
			t.Fatalf("operation has no parameters: %#v", op)
		}
		for _, raw := range rawParameters {
			value := raw.(map[string]any)
			if value["name"] == name {
				return value
			}
		}
		t.Fatalf("parameter %q not found in %#v", name, op["parameters"])
		return nil
	}
	assertEnum := func(op map[string]any, name string, want []string) {
		t.Helper()
		raw := parameter(op, name)["schema"].(map[string]any)["enum"].([]any)
		got := make([]string, 0, len(raw))
		for _, value := range raw {
			got = append(got, value.(string))
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s enum = %#v, want %#v", name, got, want)
		}
	}
	assertRequired := func(schema map[string]any, name string) {
		t.Helper()
		raw, ok := schema["required"].([]any)
		if !ok {
			t.Fatalf("schema has no required list: %#v", schema)
		}
		for _, value := range raw {
			if value == name {
				return
			}
		}
		t.Fatalf("schema required missing %s: %#v", name, raw)
	}

	tasks := operation("/api/v1/tasks", "get")
	for _, name := range []string{"workspace", "project", "project_id", "report", "query", "status", "due_after", "due_before", "occurrence_mode", "task_type", "sort", "limit", "offset"} {
		_ = parameter(tasks, name)
	}
	assertEnum(tasks, "occurrence_mode", []string{"auto", "materialized", "expand"})
	assertEnum(tasks, "task_type", []string{"all", "normal", "occurrence"})

	taskRequestProperties := func(path, method string) map[string]any {
		t.Helper()
		requestBody, ok := operation(path, method)["requestBody"].(map[string]any)
		if !ok {
			t.Fatalf("%s %s missing requestBody", method, path)
		}
		content := requestBody["content"].(map[string]any)
		schema := content["application/json"].(map[string]any)["schema"].(map[string]any)
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s %s request schema has no properties: %#v", method, path, schema)
		}
		return properties
	}
	createTaskProps := taskRequestProperties("/api/v1/tasks", "post")
	for _, name := range []string{"title", "description", "project", "project_id", "priority", "due", "due_date", "assignees", "depends", "wait", "wait_date", "scheduled", "scheduled_date", "until", "until_date", "tags", "udas", "parent"} {
		if _, ok := createTaskProps[name]; !ok {
			t.Fatalf("task create request schema missing %s", name)
		}
	}
	modifyTaskProps := taskRequestProperties("/api/v1/tasks/{taskRef}", "patch")
	for _, name := range []string{"title", "description", "clear_description", "project", "project_id", "clear_project", "priority", "clear_priority", "due", "due_date", "clear_due", "assignees", "remove_assignees", "clear_assignees", "depends", "clear_depends", "tags", "remove_tags", "udas", "clear_udas"} {
		if _, ok := modifyTaskProps[name]; !ok {
			t.Fatalf("task modify request schema missing %s", name)
		}
	}
	for _, properties := range []map[string]any{createTaskProps, modifyTaskProps} {
		for _, removed := range []string{"recur", "clear_recur", "mask", "imask"} {
			if _, ok := properties[removed]; ok {
				t.Fatalf("generic task request schema unexpectedly contains %s", removed)
			}
		}
	}
	taskResponseProperties := func(path, method, status string) map[string]any {
		t.Helper()
		response := operation(path, method)["responses"].(map[string]any)[status].(map[string]any)
		schema := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		return schema["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	}
	for _, route := range []struct {
		path, method, status string
	}{
		{path: "/api/v1/tasks", method: "post", status: "201"},
		{path: "/api/v1/tasks/{taskRef}", method: "get", status: "200"},
		{path: "/api/v1/tasks/{taskRef}", method: "patch", status: "200"},
		{path: "/api/v1/tasks/{taskRef}", method: "delete", status: "200"},
		{path: "/api/v1/tasks/{taskRef}/done", method: "post", status: "200"},
		{path: "/api/v1/tasks/{taskRef}/start", method: "post", status: "200"},
		{path: "/api/v1/tasks/{taskRef}/stop", method: "post", status: "200"},
		{path: "/api/v1/tasks/{taskRef}/reopen", method: "post", status: "200"},
	} {
		properties := taskResponseProperties(route.path, route.method, route.status)
		for _, name := range []string{"id", "url", "uuid", "task_slug", "status", "recurrence_info"} {
			if _, ok := properties[name]; !ok {
				t.Fatalf("%s %s response schema missing %s", route.method, route.path, name)
			}
		}
	}

	report := operation("/api/v1/reports/{name}", "get")
	for _, name := range []string{"workspace", "project", "project_id", "query", "due_after", "due_before", "occurrence_mode", "task_type", "sort", "limit", "offset"} {
		_ = parameter(report, name)
	}

	series := operation("/api/v1/task-series", "get")
	assertEnum(series, "status", []string{"active", "ended", "stopped", "all"})
	assertEnum(series, "sort", []string{"next", "title", "modified"})
	for _, name := range []string{"workspace", "project", "project_id", "q", "assignee", "limit", "offset"} {
		_ = parameter(series, name)
	}
	seriesLimitSchema := parameter(series, "limit")["schema"].(map[string]any)
	if seriesLimitSchema["default"] != float64(200) || seriesLimitSchema["minimum"] != float64(1) || seriesLimitSchema["maximum"] != float64(1000) {
		t.Fatalf("series limit schema = %#v, want default=200 range=1..1000", seriesLimitSchema)
	}
	seriesOffsetSchema := parameter(series, "offset")["schema"].(map[string]any)
	if seriesOffsetSchema["default"] != float64(0) || seriesOffsetSchema["minimum"] != float64(0) {
		t.Fatalf("series offset schema = %#v, want default=0 minimum=0", seriesOffsetSchema)
	}
	occurrences := operation("/api/v1/task-series/{seriesRef}/occurrences", "get")
	assertEnum(occurrences, "status", []string{"pending", "waiting", "completed", "deleted", "all"})
	for _, name := range []string{"workspace", "due_after", "due_before", "limit", "offset"} {
		_ = parameter(occurrences, name)
	}

	requestSchema := operation("/api/v1/task-series", "post")["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	requestProps := requestSchema["properties"].(map[string]any)
	for _, name := range []string{"title", "description", "project", "project_id", "recurrence_rule", "first_due", "first_due_date", "until", "until_date", "priority", "assignees", "tags", "udas"} {
		if _, ok := requestProps[name]; !ok {
			t.Fatalf("task-series request schema missing %s", name)
		}
	}
	if _, ok := requestProps["wait"]; ok {
		t.Fatal("task-series request schema unexpectedly contains wait")
	}

	responseSchema := tasks["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	dataProps := responseSchema["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	for _, name := range []string{"items", "total", "limit", "offset", "occurrence_mode", "range"} {
		if _, ok := dataProps[name]; !ok {
			t.Fatalf("TaskViewPage schema missing %s", name)
		}
	}
	itemSchema := dataProps["items"].(map[string]any)["items"].(map[string]any)
	itemProps := itemSchema["properties"].(map[string]any)
	if _, ok := itemProps["url"]; !ok {
		t.Fatal("TaskOccurrence schema missing url")
	}
	assertRequired(itemSchema, "url")
	for name, typ := range map[string]string{"uuid": "string", "task_slug": "string", "project_seq": "integer", "entry": "integer", "modified": "integer", "start": "integer", "end": "integer", "recurrence_info": "object"} {
		types, ok := itemProps[name].(map[string]any)["type"].([]any)
		if !ok || len(types) != 2 || types[0] != typ || types[1] != "null" {
			t.Fatalf("%s type = %#v, want [%s null]", name, itemProps[name].(map[string]any)["type"], typ)
		}
	}
	for _, name := range []string{"parent", "depends_info", "parent_info", "blocked_by_info"} {
		if _, ok := itemProps[name]; !ok {
			t.Fatalf("TaskOccurrence schema missing %s", name)
		}
	}
	assigneeProps := itemProps["assignees"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, name := range []string{"id", "name", "display_name", "email", "external_ids"} {
		if _, ok := assigneeProps[name]; !ok {
			t.Fatalf("TaskOccurrence assignee schema missing %s", name)
		}
	}

	seriesResponseSchema := series["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	seriesDataProps := seriesResponseSchema["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	seriesItemSchema := seriesDataProps["items"].(map[string]any)["items"].(map[string]any)
	seriesItemProps := seriesItemSchema["properties"].(map[string]any)
	for _, name := range []string{
		"url",
		"open_occurrence_count", "completed_count", "skipped_count", "overdue_count",
		"next_recurrence_at", "suggested_rule_effective_from", "created_by",
		"open_occurrences", "recent_completed", "recent_skipped",
	} {
		if _, ok := seriesItemProps[name]; !ok {
			t.Fatalf("TaskSeries schema missing %s", name)
		}
	}
	assertRequired(seriesItemSchema, "url")
	createdByProps := seriesItemProps["created_by"].(map[string]any)["properties"].(map[string]any)
	for _, name := range []string{"id", "name", "display_name", "email", "external_ids"} {
		if _, ok := createdByProps[name]; !ok {
			t.Fatalf("TaskSeries created_by schema missing %s", name)
		}
	}

	projectOperation := operation("/api/v1/projects/{projectRef}", "get")
	projectResponse := projectOperation["responses"].(map[string]any)["200"].(map[string]any)
	projectContent := projectResponse["content"].(map[string]any)["application/json"].(map[string]any)
	projectSchema := projectContent["schema"].(map[string]any)
	projectData, ok := projectSchema["properties"].(map[string]any)["data"].(map[string]any)
	if !ok {
		t.Fatalf("project response has no data schema: %#v", projectSchema)
	}
	projectProps, ok := projectData["properties"].(map[string]any)
	if !ok {
		t.Fatalf("project response data has no properties: %#v", projectData)
	}
	if _, ok := projectProps["url"]; !ok {
		t.Fatal("Project schema missing url")
	}
	assertRequired(projectData, "url")
}

func countOpenAPIOperations(paths map[string]map[string]any) int {
	methods := map[string]struct{}{
		"get":     {},
		"post":    {},
		"put":     {},
		"patch":   {},
		"delete":  {},
		"head":    {},
		"options": {},
		"trace":   {},
	}
	total := 0
	for _, pathItem := range paths {
		for method := range pathItem {
			if _, ok := methods[method]; ok {
				total++
			}
		}
	}
	return total
}

func TestErrorEnvelopeForUnknownRoute(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "route_not_found")

	rr = httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	assertHTTPErrorCode(t, rr, http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestConsoleRoutesDoNotInterceptAPIOrMCP(t *testing.T) {
	srv := NewServer(Options{
		Store: openHTTPTestStore(t),
		Console: config.ConsoleConfig{
			Enabled:     true,
			BasePath:    "/",
			AssetsCache: time.Hour,
			AuthMode:    "bearer",
		},
		TestConsoleHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/", "/tasks":
				_, _ = w.Write([]byte("<div id=\"root\"></div>"))
			default:
				http.NotFound(w, r)
			}
		}),
	})

	for _, path := range []string{"/", "/tasks"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want console response body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "<div id=\"root\"></div>") {
				t.Fatalf("body = %s, want console html", rr.Body.String())
			}
		})
	}

	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/console", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("/console status = %d, want 404 body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "route_not_found")

	rr = httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`)))
	if rr.Code == http.StatusOK && strings.Contains(rr.Body.String(), "Xuanchu Console") {
		t.Fatalf("/mcp was served by console fallback: %s", rr.Body.String())
	}
}

func TestPanicIsRecoveredAsErrorEnvelope(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t), TestPanicRoute: true})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/__panic", nil))
	assertHTTPErrorCode(t, rr, http.StatusInternalServerError, "api_internal")
}
