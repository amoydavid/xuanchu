package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2EHTTPMCPTaskToolGoldenPath(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "xuanchu.db")
	configPath, logPath := writeE2ELogConfig(t, dir)
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", db, "http-mcp-e2e", "*"))

	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db", db)
	defer stopXuanchuServer(t, cmd)

	session, cancel := connectHTTPMCP(t, baseURL, token)
	defer cancel()
	add, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "task_add",
		Arguments: map[string]any{
			"title": "http mcp e2e task",
			"tags":  []string{"e2e"},
		},
	})
	if err != nil {
		t.Fatalf("HTTP MCP task_add error = %v", err)
	}
	addEnv := mcpStructuredMap(t, add)
	taskObj := nestedMap(t, nestedMap(t, addEnv, "data"), "task")
	if taskObj["uuid"] == "" || taskObj["title"] != "http mcp e2e task" {
		t.Fatalf("task_add structured content = %#v", addEnv)
	}

	query, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "task_query",
		Arguments: map[string]any{"query": "+e2e"},
	})
	if err != nil {
		t.Fatalf("HTTP MCP task_query error = %v", err)
	}
	queryEnv := mcpStructuredMap(t, query)
	tasks, _ := nestedMap(t, queryEnv, "data")["tasks"].([]any)
	if !jsonArrayContainsString(tasks, "title", "http mcp e2e task") {
		t.Fatalf("task_query structured content = %#v", queryEnv)
	}

	stopXuanchuServer(t, cmd)
	assertLogContains(t, logPath,
		"operation=mcp_tool_call",
		"tool=task_add",
		"tool=task_query",
	)
}

func TestE2EHTTPMCPProjectAllowlistRejectsCrossProject(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "project", "add", "allowed", "name:Allowed")
	run(t, bin, "--db", db, "project", "add", "blocked", "name:Blocked")
	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "http-mcp-project-e2e",
		"--scope", "*",
		"--project", "allowed",
		"--expires-in", "720h",
	)
	token := parseRawToken(t, tokenOut)

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	session, cancel := connectHTTPMCP(t, baseURL, token)
	defer cancel()
	allowed, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "task_add",
		Arguments: map[string]any{
			"project": "allowed",
			"title":   "allowed project task",
		},
	})
	if err != nil {
		t.Fatalf("HTTP MCP allowed task_add protocol error = %v", err)
	}
	_ = mcpStructuredMap(t, allowed)

	blocked, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "task_query",
		Arguments: map[string]any{
			"project": "blocked",
		},
	})
	if err != nil {
		t.Fatalf("HTTP MCP blocked task_query protocol error = %v", err)
	}
	if !blocked.IsError {
		t.Fatalf("blocked project query unexpectedly succeeded: %#v", blocked.StructuredContent)
	}
	errPayload := mcpStructuredMapAllowError(t, blocked)
	if errPayload["code"] != "project_scope_denied" {
		t.Fatalf("blocked project error = %#v, want project_scope_denied", errPayload)
	}
}

func TestE2EMCPStdioTaskToolGoldenPath(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "xuanchu.db")
	configPath, logPath := writeE2ELogConfig(t, dir)

	cmd := exec.Command(bin, "--config", configPath, "--db", db, "mcp", "stdio")
	session, cancel := connectStdioMCP(t, cmd)
	defer cancel()

	add, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "task_add",
		Arguments: map[string]any{"title": "stdio mcp e2e task"},
	})
	if err != nil {
		t.Fatalf("stdio MCP task_add error = %v", err)
	}
	addEnv := mcpStructuredMap(t, add)
	taskObj := nestedMap(t, nestedMap(t, addEnv, "data"), "task")
	if taskObj["uuid"] == "" || taskObj["title"] != "stdio mcp e2e task" {
		t.Fatalf("stdio task_add structured content = %#v", addEnv)
	}

	query, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "task_query",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("stdio MCP task_query error = %v", err)
	}
	queryEnv := mcpStructuredMap(t, query)
	tasks, _ := nestedMap(t, queryEnv, "data")["tasks"].([]any)
	if !jsonArrayContainsString(tasks, "title", "stdio mcp e2e task") {
		t.Fatalf("stdio task_query structured content = %#v", queryEnv)
	}

	assertLogContains(t, logPath,
		"operation=mcp_tool_call",
		"tool=task_add",
		"tool=task_query",
	)
}

func TestE2ERecurringQueriesReportsAndSeriesListsAreProtocolEquivalent(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "server.db")
	clientDB := filepath.Join(dir, "client.db")
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", db, "recurrence-contract-e2e", "*"))
	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	session, cancel := connectHTTPMCP(t, baseURL, token)
	defer cancel()
	callMCPToolData(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	addSeries := func(title, rule, firstDue string) string {
		t.Helper()
		data := callMCPToolData(t, session, "task_series_add", map[string]any{
			"project": "ops", "title": title, "recurrence_rule": rule, "first_due_date": firstDue,
			"assignees": []string{"local"},
		})
		series := nestedMap(t, data, "series")
		id, _ := series["id"].(string)
		if id == "" {
			t.Fatalf("task_series_add data = %#v", data)
		}
		return id
	}
	addSeries("Alpha recurring", "daily", "2030-01-01")
	addSeries("Beta recurring", "weekly", "2031-01-01")
	addSeries("Gamma recurring", "monthly", "2031-01-01")
	dependencyRoot := nestedMap(t, callMCPToolData(t, session, "task_add", map[string]any{
		"project": "ops", "title": "dependency root", "priority": "H",
	}), "task")
	blockedChild := nestedMap(t, callMCPToolData(t, session, "task_add", map[string]any{
		"project": "ops", "title": "blocked child", "priority": "L",
	}), "task")
	callMCPToolData(t, session, "task_modify", map[string]any{
		"id": blockedChild["uuid"], "depends": []string{dependencyRoot["uuid"].(string)},
	})

	queryValues := url.Values{
		"workspace":       {"local"},
		"project":         {"ops"},
		"due_after":       {"2030-01-01"},
		"due_before":      {"2030-01-02"},
		"occurrence_mode": {"expand"},
		"task_type":       {"occurrence"},
		"sort":            {"due"},
		"limit":           {"1"},
		"offset":          {"1"},
	}
	httpQuery := nestedMap(t, httpJSON(t, http.MethodGet, baseURL+"/api/v1/tasks?"+queryValues.Encode(), nil, authHeaders(token)), "data")
	mcpQuery := callMCPToolData(t, session, "task_query", map[string]any{
		"project": "ops", "due_after": "2030-01-01", "due_before": "2030-01-02",
		"occurrence_mode": "expand", "task_type": "occurrence", "sort": "due", "limit": 1, "offset": 1,
	})
	remoteQuery := parseJSONMap(t, run(t, bin,
		"--db", clientDB, "--server", baseURL, "--token", token, "--workspace", "local", "--project", "ops", "--json",
		"list", "task_type:occurrence", "--due-after", "2030-01-01", "--due-before", "2030-01-02",
		"--occurrence-mode", "expand", "--sort", "due", "--limit", "1", "--offset", "1",
	))
	assertTaskViewPagesEquivalent(t, "task query", httpQuery, mcpQuery, remoteQuery)

	reportValues := cloneURLValues(queryValues)
	reportValues.Del("task_type")
	reportValues.Add("query", "task_type:occurrence")
	httpReport := nestedMap(t, httpJSON(t, http.MethodGet, baseURL+"/api/v1/reports/all?"+reportValues.Encode(), nil, authHeaders(token)), "data")
	httpTaskReportValues := cloneURLValues(reportValues)
	httpTaskReportValues.Set("report", "all")
	httpTaskReport := nestedMap(t, httpJSON(t, http.MethodGet, baseURL+"/api/v1/tasks?"+httpTaskReportValues.Encode(), nil, authHeaders(token)), "data")
	mcpReport := callMCPToolData(t, session, "report_run", map[string]any{
		"project": "ops", "name": "all", "query": "task_type:occurrence",
		"due_after": "2030-01-01", "due_before": "2030-01-02", "occurrence_mode": "expand",
		"sort": "due", "limit": 1, "offset": 1,
	})
	remoteReport := parseJSONMap(t, run(t, bin,
		"--db", clientDB, "--server", baseURL, "--token", token, "--workspace", "local", "--project", "ops", "--json",
		"all", "task_type:occurrence", "--due-after", "2030-01-01", "--due-before", "2030-01-02",
		"--occurrence-mode", "expand", "--sort", "due", "--limit", "1", "--offset", "1",
	))
	assertTaskViewPagesEquivalent(t, "report", httpReport, httpTaskReport, mcpReport, remoteReport)

	seriesValues := url.Values{
		"workspace": {"local"}, "project": {"ops"}, "status": {"active"},
		"q": {"recurring"}, "assignee": {"local"}, "sort": {"title"}, "limit": {"1"}, "offset": {"1"},
	}
	httpSeries := nestedMap(t, httpJSON(t, http.MethodGet, baseURL+"/api/v1/task-series?"+seriesValues.Encode(), nil, authHeaders(token)), "data")
	mcpSeries := callMCPToolData(t, session, "task_series_list", map[string]any{
		"project": "ops", "status": "active", "q": "recurring", "assignee": "local", "sort": "title", "limit": 1, "offset": 1,
	})
	remoteSeries := parseJSONMap(t, run(t, bin,
		"--db", clientDB, "--server", baseURL, "--token", token, "--workspace", "local", "--project", "ops", "--json",
		"series", "list", "--status", "active", "--query", "recurring", "--assignee", "local", "--sort", "title", "--limit", "1", "--offset", "1",
	))
	assertSeriesPagesEquivalent(t, httpSeries, mcpSeries, remoteSeries)

	assertReport := func(name string, wantTotal int, wantFirstTitle string) {
		t.Helper()
		values := url.Values{
			"workspace": {"local"}, "project": {"ops"}, "sort": {"urgency-"}, "limit": {"10"}, "offset": {"0"},
		}
		httpPage := nestedMap(t, httpJSON(t, http.MethodGet,
			baseURL+"/api/v1/reports/"+name+"?"+values.Encode(), nil, authHeaders(token)), "data")
		taskReportValues := cloneURLValues(values)
		taskReportValues.Set("report", name)
		httpTaskPage := nestedMap(t, httpJSON(t, http.MethodGet,
			baseURL+"/api/v1/tasks?"+taskReportValues.Encode(), nil, authHeaders(token)), "data")
		mcpPage := callMCPToolData(t, session, "report_run", map[string]any{
			"project": "ops", "name": name, "sort": "urgency-", "limit": 10, "offset": 0,
		})
		remotePage := parseJSONMap(t, run(t, bin,
			"--db", clientDB, "--server", baseURL, "--token", token, "--workspace", "local", "--project", "ops", "--json",
			name, "--sort", "urgency-", "--limit", "10", "--offset", "0",
		))
		contract := assertTaskViewPagesCoreEquivalent(t, name, httpPage, httpTaskPage, mcpPage, remotePage)
		if contract.Total != wantTotal || len(contract.Items) == 0 || contract.Items[0].Title != wantFirstTitle {
			t.Fatalf("%s report = %#v, want total=%d first=%q", name, contract, wantTotal, wantFirstTitle)
		}
	}
	assertReport("ready", 1, "dependency root")
	assertReport("blocked", 1, "blocked child")
	assertReport("blocking", 1, "dependency root")
	assertReport("all", 2, "dependency root")
}

func callMCPToolData(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("MCP %s protocol error: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("MCP %s business error: %#v arguments=%#v", name, mcpStructuredMapAllowError(t, result), arguments)
	}
	return nestedMap(t, mcpStructuredMap(t, result), "data")
}

func cloneURLValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

type taskViewPageContract struct {
	Items []struct {
		ID         string  `json:"id"`
		UUID       *string `json:"uuid"`
		TaskSlug   *string `json:"task_slug"`
		ProjectSeq *int64  `json:"project_seq"`
		Title      string  `json:"title"`
		Status     string  `json:"status"`
		Due        *int64  `json:"due"`
		Priority   *string `json:"priority"`
		Entry      *int64  `json:"entry"`
		Modified   *int64  `json:"modified"`
		Start      *int64  `json:"start"`
		End        *int64  `json:"end"`
		Recurrence struct {
			SeriesID        string `json:"series_id"`
			RecurrenceAt    int64  `json:"recurrence_at"`
			Materialization string `json:"materialization"`
		} `json:"recurrence_info"`
	} `json:"items"`
	Total          int    `json:"total"`
	Limit          int    `json:"limit"`
	Offset         int    `json:"offset"`
	OccurrenceMode string `json:"occurrence_mode"`
}

func assertTaskViewPagesCoreEquivalent(t *testing.T, label string, pages ...map[string]any) taskViewPageContract {
	t.Helper()
	var want taskViewPageContract
	for index, page := range pages {
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		var got taskViewPageContract
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			want = got
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s page %d = %#v, want %#v", label, index, got, want)
		}
	}
	return want
}

func assertTaskViewPagesEquivalent(t *testing.T, label string, pages ...map[string]any) {
	t.Helper()
	var want taskViewPageContract
	for index, page := range pages {
		items, ok := page["items"].([]any)
		if !ok {
			t.Fatalf("%s page %d items = %#v", label, index, page["items"])
		}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("%s page %d item = %#v", label, index, raw)
			}
			for _, field := range []string{"uuid", "task_slug", "project_seq", "entry", "modified", "start", "end"} {
				if _, exists := item[field]; !exists {
					t.Fatalf("%s page %d item missing nullable %s: %#v", label, index, field, item)
				}
			}
		}
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		var got taskViewPageContract
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			want = got
			if want.Total != 2 || len(want.Items) != 1 || want.OccurrenceMode != "expand" {
				t.Fatalf("%s baseline = %#v", label, want)
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s page %d = %#v, want %#v", label, index, got, want)
		}
	}
}

type seriesPageContract struct {
	Items []struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		Status         string `json:"status"`
		RecurrenceRule string `json:"recurrence_rule"`
	} `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func assertSeriesPagesEquivalent(t *testing.T, pages ...map[string]any) {
	t.Helper()
	var want seriesPageContract
	for index, page := range pages {
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		var got seriesPageContract
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			want = got
			if want.Total != 3 || len(want.Items) != 1 || want.Items[0].Title != "Beta recurring" {
				t.Fatalf("series baseline = %#v", want)
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("series page %d = %#v, want %#v", index, got, want)
		}
	}
}

func nestedMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	child, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%q = %#v, want object", key, parent[key])
	}
	return child
}

func mcpStructuredMapAllowError(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if result.StructuredContent == nil {
		t.Fatalf("MCP error result missing structured content: %#v", result.Content)
	}
	raw := toJSONString(t, result.StructuredContent)
	payload := parseJSONMap(t, raw)
	return payload
}
