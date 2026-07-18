package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// fixedTestClock 返回固定时间对应的 testClock。
func fixedTestClock() testClock {
	return testClock{now: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC).Unix()}
}

// ptrStr 返回字符串指针。
func ptrStr(s string) *string { return &s }

func ensureMCPHookSink(t *testing.T, store *storage.Store) {
	t.Helper()
	svc, err := app.NewService(app.ServiceOptions{Store: store, Clock: fixedTestClock()})
	if err != nil {
		t.Fatal(err)
	}
	existing, err := svc.ListNotificationSinks(true)
	if err == nil {
		for _, sink := range existing {
			if sink.Name == "hook-sink" {
				return
			}
		}
	}
	if _, err := svc.AddNotificationSink(app.NotificationSinkAddInput{
		Name:         "hook-sink",
		Type:         app.NotificationSinkTypeWebhook,
		EndpointMode: app.NotificationEndpointStaticURL,
		URL:          "https://example.com/webhook",
		AllowedHosts: []string{"example.com"},
		Secret:       "hook-secret",
	}); err != nil {
		t.Fatal(err)
	}
}

// extractTask 从 envelope data 中提取 task 对象。
// task tools 为兼容既有调用方保留 {"task": {...}}，此函数提取内层 view。
func extractTask(t *testing.T, env ToolEnvelope) map[string]any {
	t.Helper()
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", env.Data)
	}
	taskObj, ok := dataMap["task"].(map[string]any)
	if !ok {
		t.Fatalf("task type = %T, want map", dataMap["task"])
	}
	return taskObj
}

// extractUUID 从 envelope data 中提取 task UUID。
func extractUUID(t *testing.T, env ToolEnvelope) string {
	t.Helper()
	taskObj := extractTask(t, env)
	uuid, ok := taskObj["uuid"].(string)
	if !ok || uuid == "" {
		t.Fatalf("uuid = %v, want non-empty string", taskObj["uuid"])
	}
	return uuid
}

// newTestServer 创建注册了所有工具的 MCP server。
func newTestServer(t *testing.T) (*mcp.Server, testClock) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	clock := fixedTestClock()
	ensureMCPHookSink(t, store)
	opts := Options{Store: store, Clock: clock, Version: "test"}
	srv := NewServer(opts)
	return srv, clock
}

// connectClient 创建 in-memory transport 连接，返回 client session。
func connectClient(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() {
		if err := srv.Run(context.Background(), serverTransport); err != nil {
			t.Logf("server run: %v", err)
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// callTool 是测试辅助，调用指定 tool 并解析 result。
func callTool(t *testing.T, session *mcp.ClientSession, toolName string, input any) *mcp.CallToolResult {
	t.Helper()
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	args := map[string]any{}
	if err := json.Unmarshal(inputJSON, &args); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("call tool %s: %v", toolName, err)
	}
	return result
}

// parseEnvelope 从 CallToolResult 的 StructuredContent 中提取 envelope。
func parseEnvelope(t *testing.T, result *mcp.CallToolResult) ToolEnvelope {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var env ToolEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return env
}

// parseError 从 IsError=true 的 result 中尝试提取 ToolError。
// 如果 StructuredContent 不是 ToolError 格式，返回零值 ToolError。
func parseError(t *testing.T, result *mcp.CallToolResult) ToolError {
	t.Helper()
	if !result.IsError {
		t.Fatalf("expected IsError=true")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var toolErr ToolError
	if err := json.Unmarshal(raw, &toolErr); err != nil {
		// SDK 级别验证错误，StructuredContent 可能不是 ToolError 格式
		return ToolError{Code: "sdk_validation", Message: string(raw)}
	}
	return toolErr
}

func envelopeData(t *testing.T, env ToolEnvelope) map[string]any {
	t.Helper()
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", env.Data)
	}
	return dataMap
}

func nestedMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s type = %T, want map", key, parent[key])
	}
	return value
}

func assertMCPSystemActor(t *testing.T, value any, tokenID string) {
	t.Helper()
	actor, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("actor type = %T, want map", value)
	}
	if actor["type"] != "tenant_access_token" {
		t.Fatalf("actor.type = %v, want tenant_access_token", actor["type"])
	}
	token, ok := actor["token"].(map[string]any)
	if !ok {
		t.Fatalf("actor.token type = %T, want map", actor["token"])
	}
	if token["id"] != tokenID || token["name"] != "tenant-p2" {
		t.Fatalf("actor.token = %#v, want id=%q name=tenant-p2", token, tokenID)
	}
}

func nestedSlice(t *testing.T, parent map[string]any, key string) []any {
	t.Helper()
	value, ok := parent[key].([]any)
	if !ok {
		t.Fatalf("%s type = %T, want []any", key, parent[key])
	}
	return value
}

func newTestServerWithOptions(t *testing.T, opts Options) (*mcp.Server, *storage.Store) {
	t.Helper()
	store := opts.Store
	if store == nil {
		var err error
		store, err = storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		opts.Store = store
	}
	if opts.Clock == nil {
		opts.Clock = fixedTestClock()
	}
	if opts.Version == "" {
		opts.Version = "test"
	}
	ensureMCPHookSink(t, store)
	return NewServer(opts), store
}

// ---------------------------------------------------------------------------
// Schema golden 测试
// ---------------------------------------------------------------------------

func TestListToolsWithRegistered(t *testing.T) {
	srv, _ := newTestServer(t)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() {
		if err := srv.Run(context.Background(), serverTransport); err != nil {
			t.Logf("server run: %v", err)
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	expectedTools := []string{
		"task_add", "task_query", "task_get",
		"task_modify", "task_done", "task_delete",
		"task_annotate", "task_denotate", "task_depends",
		"task_start", "task_stop", "task_reopen",
		"task_link_add", "task_link_list", "task_link_remove",
		"task_export", "task_import",
		"report_run", "urgency_explain",
		"workspace_list", "workspace_get_current",
		"workspace_info", "workspace_add", "workspace_modify", "workspace_archive", "workspace_use",
		"project_list", "project_get", "project_get_current",
		"project_add", "project_modify", "project_archive", "project_transition",
		"project_annotate", "project_denotate",
		"project_list_annotations", "project_list_timeline",
		"project_config_list", "project_config_set", "project_config_unset",
		"member_list", "member_add", "member_role",
		"user_list", "user_get", "user_add", "user_bind", "user_unbind",
		"user_use", "user_list_external_ids",
		"context_get", "context_set", "context_none",
		"context_list", "context_delete",
		"config_get", "config_set", "config_list", "config_unset",
		"config_schema_list", "config_schema_get", "config_schema_set", "config_schema_delete",
		"hook_list", "hook_add", "hook_info", "hook_modify", "hook_remove",
		"hook_test", "hook_delivery_list", "hook_delivery_info",
		"hook_delivery_redeliver", "hook_ping",
		"notification_sink_list", "notification_sink_add", "notification_sink_info",
		"notification_sink_modify", "notification_sink_enable", "notification_sink_disable",
		"notification_sink_remove",
		"reminder_rule_list", "reminder_rule_add", "reminder_rule_info",
		"reminder_rule_modify", "reminder_rule_enable", "reminder_rule_disable",
		"reminder_rule_remove",
		"notification_rule_list", "notification_rule_add", "notification_rule_info",
		"notification_rule_modify", "notification_rule_enable", "notification_rule_disable",
		"notification_rule_remove",
		"notification_delivery_list", "notification_delivery_info", "notification_delivery_replay",
		"token_list", "token_create", "token_modify", "token_revoke",
		"audit_list", "scope_list", "me_get",
		"task_series_add", "task_series_list", "task_series_get", "task_series_modify",
		"task_series_stop", "task_series_list_occurrences", "task_series_occurrence_skip",
	}
	if len(result.Tools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(result.Tools))
	}
	toolNames := make([]string, len(result.Tools))
	for i, tool := range result.Tools {
		toolNames[i] = tool.Name
	}
	for _, expected := range expectedTools {
		found := false
		for _, name := range toolNames {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing tool %q in %v", expected, toolNames)
		}
	}

	// 为每个 tool 生成 golden schema
	for _, tool := range result.Tools {
		t.Run("schema/"+tool.Name, func(t *testing.T) {
			got, err := json.MarshalIndent(tool, "", "  ")
			if err != nil {
				t.Fatalf("marshal tool: %v", err)
			}
			goldenName := fmt.Sprintf("%s.schema.json", tool.Name)
			want := goldenGet(t, goldenName, got)
			gotS := string(got)
			wantS := string(want)
			if gotS != wantS {
				t.Fatalf("golden mismatch for %s:\n--- want\n%s\n--- got\n%s", tool.Name, wantS, gotS)
			}
		})
	}
}

func TestMCPToolReturnsServerDrainingWhenShutdownStarted(t *testing.T) {
	shutdown := runtimeutil.NewShutdownCoordinator()
	shutdown.StopAccepting()
	srv, _ := newTestServerWithOptions(t, Options{Shutdown: shutdown})
	session := connectClient(t, srv)

	result := callTool(t, session, "task_query", map[string]any{})
	errResult := parseError(t, result)
	if errResult.Code != "server_draining" {
		t.Fatalf("error code = %q, want server_draining; message=%q", errResult.Code, errResult.Message)
	}
}

func TestMCPToolContextCanceledOnShutdownForceCancel(t *testing.T) {
	shutdown := runtimeutil.NewShutdownCoordinator()
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	started := make(chan struct{})
	done := make(chan struct{})
	addTool(srv, Options{Shutdown: shutdown}, &mcp.Tool{Name: "test_shutdown"}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, ToolEnvelope, error) {
		close(started)
		<-ctx.Done()
		close(done)
		return successWithEnvelope(map[string]any{"canceled": true}, "canceled")
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() {
		if err := srv.Run(context.Background(), serverTransport); err != nil {
			t.Logf("server run: %v", err)
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	callDone := make(chan struct{})
	go func() {
		_, _ = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "test_shutdown", Arguments: map[string]any{}})
		close(callDone)
	}()
	<-started
	shutdown.ForceCancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tool context was not canceled after ForceCancel")
	}
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("tool call did not return after ForceCancel")
	}
}

// ---------------------------------------------------------------------------
// task.add 集成测试
// ---------------------------------------------------------------------------

func TestTaskAddBasic(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task_add", TaskAddInput{
		Title: "buy milk",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	// task view 同时在 data 顶层和 data.task 中返回。
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", env.Data)
	}
	taskObj, ok := dataMap["task"].(map[string]any)
	if !ok {
		t.Fatalf("task type = %T, want map", dataMap["task"])
	}
	if taskObj["title"] != "buy milk" {
		t.Fatalf("title = %v, want buy milk", taskObj["title"])
	}
	if taskObj["uuid"] == nil || taskObj["uuid"] == "" {
		t.Fatal("uuid is empty")
	}
	if taskObj["url"] != "/tasks/"+taskObj["uuid"].(string) {
		t.Fatalf("task url = %#v", taskObj["url"])
	}
	if dataMap["id"] != taskObj["uuid"] || dataMap["uuid"] != taskObj["uuid"] {
		t.Fatalf("unified identity = id:%v uuid:%v task.uuid:%v", dataMap["id"], dataMap["uuid"], taskObj["uuid"])
	}
	if _, ok := taskObj["entry"].(float64); !ok {
		t.Fatalf("task.entry = %#v (%T), want Unix number", taskObj["entry"], taskObj["entry"])
	}
	if recurrence, exists := dataMap["recurrence_info"]; exists && recurrence != nil {
		t.Fatalf("recurrence_info = %#v, want absent or nil", recurrence)
	}
	if env.Rendered == "" {
		t.Fatal("rendered is empty")
	}
}

func TestTaskAddAndGetAssignees(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task_add", TaskAddInput{
		Title:     "assigned task",
		Assignees: []string{"local"},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	taskObj := extractTask(t, env)
	assignees, ok := taskObj["assignees"].([]any)
	if !ok || len(assignees) != 1 {
		t.Fatalf("assignees = %#v, want one assignee", taskObj["assignees"])
	}
	first, ok := assignees[0].(map[string]any)
	if !ok || first["name"] != "local" {
		t.Fatalf("first assignee = %#v, want local", assignees[0])
	}

	uuid := extractUUID(t, env)
	getResult := callTool(t, session, "task_get", TaskGetInput{ID: uuid})
	if getResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, getResult))
	}
	taskObj = extractTask(t, parseEnvelope(t, getResult))
	assignees, ok = taskObj["assignees"].([]any)
	if !ok || len(assignees) != 1 {
		t.Fatalf("assignees after get = %#v, want one assignee", taskObj["assignees"])
	}
}

func TestTaskAddMissingTitle(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task_add", TaskAddInput{})
	if !result.IsError {
		t.Fatal("expected IsError=true for missing title")
	}
}

func TestGenericTaskToolsRejectRetiredRecurrenceFields(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	invalidAdd := callTool(t, session, "task_add", map[string]any{
		"title": "legacy recurring task", "recur": "daily",
	})
	if !invalidAdd.IsError {
		t.Fatal("task_add accepted retired recur field")
	}
	if got := parseError(t, invalidAdd).Code; got != "task_series_endpoint_required" {
		t.Fatalf("task_add recur error code = %q", got)
	}
	if unknown := callTool(t, session, "task_add", map[string]any{
		"title": "unknown field", "unexpected": true,
	}); !unknown.IsError {
		t.Fatal("task_add accepted an argument outside its closed schema")
	}

	validAdd := callTool(t, session, "task_add", TaskAddInput{Title: "ordinary task"})
	if validAdd.IsError {
		t.Fatalf("valid task_add: %v", parseError(t, validAdd))
	}
	uuid := extractUUID(t, parseEnvelope(t, validAdd))
	invalidModify := callTool(t, session, "task_modify", map[string]any{
		"id": uuid, "recur": "weekly",
	})
	if !invalidModify.IsError {
		t.Fatal("task_modify accepted retired recur field")
	}
	if got := parseError(t, invalidModify).Code; got != "task_series_endpoint_required" {
		t.Fatalf("task_modify recur error code = %q", got)
	}

	got := callTool(t, session, "task_get", TaskGetInput{ID: uuid})
	if got.IsError {
		t.Fatalf("task_get after rejected modify: %v", parseError(t, got))
	}
	if title := extractTask(t, parseEnvelope(t, got))["title"]; title != "ordinary task" {
		t.Fatalf("task title after rejected modify = %v", title)
	}
	queryResult := callTool(t, session, "task_query", TaskQueryInput{})
	if queryResult.IsError {
		t.Fatalf("task_query: %v", parseError(t, queryResult))
	}
	data, ok := parseEnvelope(t, queryResult).Data.(map[string]any)
	if !ok || data["total"] != float64(1) {
		t.Fatalf("task_query data = %#v, want only the valid ordinary task", data)
	}
}

// ---------------------------------------------------------------------------
// task.get 集成测试
// ---------------------------------------------------------------------------

func TestTaskGetByID(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	// 先创建
	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "test task"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	// 再查询
	getResult := callTool(t, session, "task_get", TaskGetInput{ID: uuid})
	if getResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, getResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, getResult))
	if taskObj["title"] != "test task" {
		t.Fatalf("title = %v, want test task", taskObj["title"])
	}
}

func TestTaskGetHonorsExplicitProjectScope(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	alpha, err := svc.AddProject(app.AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := svc.AddProject(app.AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	taskA, err := svc.Add(app.AddInput{Title: "alpha task", Project: &alpha.Slug})
	if err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, svc, []string{"task:read"}, []string{"local"}, nil)
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	result := callTool(t, session, "task_get", TaskGetInput{ID: taskA.UUID, ProjectID: beta.ID})
	if !result.IsError {
		t.Fatal("task.get with mismatched explicit project_id should fail")
	}
	if code := parseError(t, result).Code; code != "task_not_found" {
		t.Fatalf("task.get code = %q, want task_not_found", code)
	}
}

func TestAddToolConvertsReturnedErrorToStructuredToolError(t *testing.T) {
	srv, _ := newTestServer(t)
	addTool(srv, Options{}, &mcp.Tool{Name: "test.error"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, ToolEnvelope, error) {
		return nil, ToolEnvelope{}, app.RuntimeError{Code: "synthetic_error", Message: "synthetic failure"}
	})
	session := connectClient(t, srv)

	result := callTool(t, session, "test.error", struct{}{})
	if !result.IsError {
		t.Fatal("test.error IsError = false, want true")
	}
	if code := parseError(t, result).Code; code != "synthetic_error" {
		t.Fatalf("test.error code = %q, want synthetic_error", code)
	}
}

func TestAddToolWritesOperationLog(t *testing.T) {
	var buf bytes.Buffer
	logger, closeLogger, err := logging.Setup(logging.LogConfig{Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLogger() })

	srv, _ := newTestServer(t)
	addTool(srv, Options{Mode: ModeStdio, Logger: logger}, &mcp.Tool{Name: "test_log"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, ToolEnvelope, error) {
		return successResult(map[string]any{"ok": true}, "ok")
	})
	session := connectClient(t, srv)

	result := callTool(t, session, "test_log", struct{}{})
	if result.IsError {
		t.Fatalf("test_log returned error: %s", renderedText(result))
	}

	logText := buf.String()
	for _, want := range []string{
		"component=mcp",
		"operation=mcp_tool_call",
		"tool=test_log",
		"mode=stdio",
		"result=success",
		"duration_ms=",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want substring %q", logText, want)
		}
	}
}

func TestTaskGetMissingID(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task_get", TaskGetInput{ID: ""})
	if !result.IsError {
		t.Fatal("expected IsError=true for missing id")
	}
}

// ---------------------------------------------------------------------------
// task.query 集成测试
// ---------------------------------------------------------------------------

func TestTaskQueryReturnsTasks(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task_add", TaskAddInput{Title: "task 1"})
	callTool(t, session, "task_add", TaskAddInput{Title: "task 2"})

	result := callTool(t, session, "task_query", TaskQueryInput{})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", env.Data)
	}
	count, _ := dataMap["count"].(float64)
	if int(count) < 2 {
		t.Fatalf("expected at least 2 tasks, got %d", int(count))
	}
}

func TestTaskQueryDefaultsToAllNonDeletedTasks(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	pendingUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "pending item"})))
	doneUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "done item"})))
	deleteUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "deleted item"})))
	if result := callTool(t, session, "task_done", TaskIDInput{ID: doneUUID}); result.IsError {
		t.Fatalf("task.done error: %v", parseError(t, result))
	}
	if result := callTool(t, session, "task_delete", TaskIDInput{ID: deleteUUID}); result.IsError {
		t.Fatalf("task.delete error: %v", parseError(t, result))
	}

	result := callTool(t, session, "task_query", TaskQueryInput{})
	if result.IsError {
		t.Fatalf("task.query error: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	if count := data["count"]; count != float64(2) {
		t.Fatalf("count = %v, want pending + completed", count)
	}
	items := nestedSlice(t, data, "items")
	ids := map[string]bool{}
	for _, raw := range items {
		item := raw.(map[string]any)
		if id, _ := item["uuid"].(string); id != "" {
			ids[id] = true
		}
	}
	if !ids[pendingUUID] || !ids[doneUUID] || ids[deleteUUID] {
		t.Fatalf("default ids = %#v, want pending/completed and no deleted", ids)
	}
}

func TestTaskQueryIncludeDeletedExtendsDefaultSet(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task_add", TaskAddInput{Title: "pending item"})
	doneUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "done item"})))
	deleteUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "deleted item"})))
	if result := callTool(t, session, "task_done", TaskIDInput{ID: doneUUID}); result.IsError {
		t.Fatalf("task.done error: %v", parseError(t, result))
	}
	if result := callTool(t, session, "task_delete", TaskIDInput{ID: deleteUUID}); result.IsError {
		t.Fatalf("task.delete error: %v", parseError(t, result))
	}

	result := callTool(t, session, "task_query", TaskQueryInput{IncludeDeleted: true})
	if result.IsError {
		t.Fatalf("task.query error: %v", parseError(t, result))
	}
	if count := envelopeData(t, parseEnvelope(t, result))["count"]; count != float64(3) {
		t.Fatalf("count = %v, want all three statuses", count)
	}
}

func TestTaskQueryExplicitStatusFilterOverridesDefaultVisibility(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	deleteUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "deleted item"})))
	if result := callTool(t, session, "task_delete", TaskIDInput{ID: deleteUUID}); result.IsError {
		t.Fatalf("task.delete error: %v", parseError(t, result))
	}

	result := callTool(t, session, "task_query", TaskQueryInput{Query: "status:deleted"})
	if result.IsError {
		t.Fatalf("task.query error: %v", parseError(t, result))
	}
	if count := envelopeData(t, parseEnvelope(t, result))["count"]; count != float64(1) {
		t.Fatalf("count = %v, want explicit deleted task", count)
	}
}

func TestTaskQueryExpandsOccurrencesAndAppliesTaskType(t *testing.T) {
	srv, store := newTestServerWithOptions(t, Options{})
	svc, err := app.NewService(app.ServiceOptions{Store: store, Clock: fixedTestClock()})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC).Unix()
	if _, err := svc.AddTaskSeries(app.AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: firstDue,
	}); err != nil {
		t.Fatal(err)
	}
	session := connectClient(t, srv)

	result := callTool(t, session, "task_query", TaskQueryInput{
		ProjectID: project.ID, DueAfter: "2030-01-01", DueBefore: "2030-01-02", TaskType: "occurrence",
	})
	if result.IsError {
		t.Fatalf("task_query occurrence error: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	if data["occurrence_mode"] != "expand" || data["total"].(float64) == 0 {
		t.Fatalf("task_query data = %#v want expanded occurrence", data)
	}
	for _, raw := range nestedSlice(t, data, "items") {
		item, ok := raw.(map[string]any)
		if !ok || item["recurrence_info"] == nil {
			t.Fatalf("task_type=occurrence item = %#v", raw)
		}
	}

	normal := callTool(t, session, "task_query", TaskQueryInput{
		ProjectID: project.ID, DueAfter: "2030-01-01", DueBefore: "2030-01-02", TaskType: "normal",
	})
	if normal.IsError {
		t.Fatalf("task_query normal error: %v", parseError(t, normal))
	}
	if got := envelopeData(t, parseEnvelope(t, normal))["total"]; got != float64(0) {
		t.Fatalf("task_type=normal total = %v want 0", got)
	}

	missingRange := callTool(t, session, "task_query", TaskQueryInput{
		ProjectID: project.ID, OccurrenceMode: "expand", DueAfter: "2030-01-01",
	})
	if got := parseError(t, missingRange).Code; got != "task_occurrence_range_required" {
		t.Fatalf("missing range code = %q want task_occurrence_range_required", got)
	}
}

func TestTaskQueryDueAfterIncludesExactLocalDayStart(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	due := time.Date(2030, 1, 1, 0, 0, 0, 0, time.Local).Unix()
	result := callTool(t, session, "task_add", TaskAddInput{
		Title: "day start", Due: &due,
	})
	if result.IsError {
		t.Fatalf("task_add error: %v", parseError(t, result))
	}

	result = callTool(t, session, "task_query", TaskQueryInput{
		DueAfter: "2030-01-01", DueBefore: "2030-01-01",
	})
	if result.IsError {
		t.Fatalf("task_query error: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	if data["total"] != float64(1) {
		t.Fatalf("task_query total = %v, want exact day-start task; data=%#v", data["total"], data)
	}
}

// ---------------------------------------------------------------------------
// task.done 集成测试
// ---------------------------------------------------------------------------

func TestTaskDone(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "finish report"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	doneResult := callTool(t, session, "task_done", TaskIDInput{ID: uuid})
	if doneResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, doneResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, doneResult))
	if taskObj["status"] != "completed" {
		t.Fatalf("status = %v, want completed", taskObj["status"])
	}
}

func TestTaskReopen(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "reopen me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	if result := callTool(t, session, "task_done", TaskIDInput{ID: uuid}); result.IsError {
		t.Fatalf("task_done error: %v", parseError(t, result))
	}

	reopenResult := callTool(t, session, "task_reopen", TaskIDInput{ID: uuid})
	if reopenResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, reopenResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, reopenResult))
	if taskObj["status"] != "pending" {
		t.Fatalf("status = %v, want pending", taskObj["status"])
	}
	if taskObj["end"] != nil {
		t.Fatalf("end = %v, want nil after reopen", taskObj["end"])
	}
}

// ---------------------------------------------------------------------------
// task.delete 集成测试
// ---------------------------------------------------------------------------

func TestTaskDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "delete me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	delResult := callTool(t, session, "task_delete", TaskIDInput{ID: uuid})
	if delResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, delResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, delResult))
	if taskObj["status"] != "deleted" {
		t.Fatalf("status = %v, want deleted", taskObj["status"])
	}
}

// ---------------------------------------------------------------------------
// task.modify 集成测试
// ---------------------------------------------------------------------------

func TestTaskModify(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "original"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	modResult := callTool(t, session, "task_modify", TaskModifyInput{
		ID:    uuid,
		Title: ptrStr("updated"),
	})
	if modResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, modResult))
	}

	// 验证更新
	getResult := callTool(t, session, "task_get", TaskGetInput{ID: uuid})
	taskObj := extractTask(t, parseEnvelope(t, getResult))
	if taskObj["title"] != "updated" {
		t.Fatalf("title = %v, want updated", taskObj["title"])
	}
}

func TestTaskAddDateFieldsUseFieldBoundaries(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{
		Title:         "date boundaries",
		DueDate:       "2030-06-15",
		UntilDate:     "2030-06-16",
		WaitDate:      "2030-06-17",
		ScheduledDate: "2030-06-18",
	})
	if addResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, addResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, addResult))
	assertMCPUnixLocalTime(t, "due", taskObj["due"], 23, 59, 59)
	assertMCPUnixLocalTime(t, "until", taskObj["until"], 23, 59, 59)
	assertMCPUnixLocalTime(t, "wait", taskObj["wait"], 0, 0, 0)
	assertMCPUnixLocalTime(t, "scheduled", taskObj["scheduled"], 0, 0, 0)
}

func assertMCPUnixLocalTime(t *testing.T, name string, value any, hour, minute, second int) {
	t.Helper()
	raw, ok := value.(float64)
	if !ok {
		t.Fatalf("%s = %#v, want Unix number", name, value)
	}
	local := time.Unix(int64(raw), 0).In(time.Local)
	if local.Hour() != hour || local.Minute() != minute || local.Second() != second {
		t.Fatalf("%s local time = %v, want %02d:%02d:%02d", name, local, hour, minute, second)
	}
}

func TestTaskModifyClearFields(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{
		Title:    "clear test",
		Priority: "H",
	})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	modResult := callTool(t, session, "task_modify", TaskModifyInput{
		ID:    uuid,
		Clear: []string{"priority"},
	})
	if modResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, modResult))
	}

	getResult := callTool(t, session, "task_get", TaskGetInput{ID: uuid})
	taskObj := extractTask(t, parseEnvelope(t, getResult))
	if taskObj["priority"] != nil {
		t.Fatalf("priority = %v, want nil after clear", taskObj["priority"])
	}
}

func TestTaskModifyAssigneesAndClear(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "assign later"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	modResult := callTool(t, session, "task_modify", TaskModifyInput{
		ID:        uuid,
		Assignees: []string{"local"},
	})
	if modResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, modResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, modResult))
	assignees, ok := taskObj["assignees"].([]any)
	if !ok || len(assignees) != 1 {
		t.Fatalf("assignees after modify = %#v, want one assignee", taskObj["assignees"])
	}

	clearResult := callTool(t, session, "task_modify", TaskModifyInput{
		ID:    uuid,
		Clear: []string{"assignees"},
	})
	if clearResult.IsError {
		t.Fatalf("unexpected clear error: %v", parseError(t, clearResult))
	}
	taskObj = extractTask(t, parseEnvelope(t, clearResult))
	assignees, ok = taskObj["assignees"].([]any)
	if !ok || len(assignees) != 0 {
		t.Fatalf("assignees after clear = %#v, want empty assignees", taskObj["assignees"])
	}
}

func TestTaskModifyRejectsUnknownClearField(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "unknown clear"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	result := callTool(t, session, "task_modify", TaskModifyInput{ID: uuid, Clear: []string{"priorty"}})
	if !result.IsError {
		t.Fatal("task.modify with unknown clear field error = nil")
	}
	if code := parseError(t, result).Code; code != "task_clear_field_unknown" {
		t.Fatalf("code = %q, want task_clear_field_unknown", code)
	}
}

// ---------------------------------------------------------------------------
// task.annotate 集成测试
// ---------------------------------------------------------------------------

func TestTaskAnnotate(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "annotate me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	annResult := callTool(t, session, "task_annotate", TaskAnnotateInput{
		ID:         uuid,
		Annotation: "this is a note",
	})
	if annResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, annResult))
	}
}

func TestTaskAnnotateMissingDescription(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "annotate me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	result := callTool(t, session, "task_annotate", TaskAnnotateInput{ID: uuid})
	if !result.IsError {
		t.Fatal("expected IsError=true for missing annotation")
	}
}

// ---------------------------------------------------------------------------
// task.start / task.stop 集成测试
// ---------------------------------------------------------------------------

func TestTaskStartStop(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "start stop"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	startResult := callTool(t, session, "task_start", TaskIDInput{ID: uuid})
	if startResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, startResult))
	}

	stopResult := callTool(t, session, "task_stop", TaskIDInput{ID: uuid})
	if stopResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, stopResult))
	}
}

// ---------------------------------------------------------------------------
// task.depends 集成测试
// ---------------------------------------------------------------------------

func TestTaskDepends(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	uuid1 := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "task 1"})))
	uuid2 := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "task 2"})))

	depResult := callTool(t, session, "task_depends", TaskDependsInput{
		ID:      uuid2,
		Depends: []string{uuid1},
	})
	if depResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, depResult))
	}

	// 清除依赖
	clearResult := callTool(t, session, "task_depends", TaskDependsInput{
		ID:           uuid2,
		ClearDepends: true,
	})
	if clearResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, clearResult))
	}
}

func TestTaskToolsAcceptTaskSlugRefs(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, session *mcp.ClientSession, slug, depUUID string)
	}{
		{name: "task_get", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "task_get", TaskGetInput{ID: slug})
			if result.IsError {
				t.Fatalf("task_get error: %v", parseError(t, result))
			}
		}},
		{name: "task_modify", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			desc := "updated"
			result := callTool(t, session, "task_modify", TaskModifyInput{ID: slug, Title: &desc})
			if result.IsError {
				t.Fatalf("task_modify error: %v", parseError(t, result))
			}
		}},
		{name: "task_done", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "task_done", TaskIDInput{ID: slug})
			if result.IsError {
				t.Fatalf("task_done error: %v", parseError(t, result))
			}
		}},
		{name: "task_delete", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "task_delete", TaskIDInput{ID: slug})
			if result.IsError {
				t.Fatalf("task_delete error: %v", parseError(t, result))
			}
		}},
		{name: "task_start", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "task_start", TaskIDInput{ID: slug})
			if result.IsError {
				t.Fatalf("task_start error: %v", parseError(t, result))
			}
		}},
		{name: "task_stop", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			if result := callTool(t, session, "task_start", TaskIDInput{ID: slug}); result.IsError {
				t.Fatalf("task_start setup error: %v", parseError(t, result))
			}
			result := callTool(t, session, "task_stop", TaskIDInput{ID: slug})
			if result.IsError {
				t.Fatalf("task_stop error: %v", parseError(t, result))
			}
		}},
		{name: "task_reopen", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			if result := callTool(t, session, "task_done", TaskIDInput{ID: slug}); result.IsError {
				t.Fatalf("task_done setup error: %v", parseError(t, result))
			}
			result := callTool(t, session, "task_reopen", TaskIDInput{ID: slug})
			if result.IsError {
				t.Fatalf("task_reopen error: %v", parseError(t, result))
			}
		}},
		{name: "task_annotate", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "task_annotate", TaskAnnotateInput{ID: slug, Annotation: "note"})
			if result.IsError {
				t.Fatalf("task_annotate error: %v", parseError(t, result))
			}
		}},
		{name: "task_denotate", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			annotateResult := callTool(t, session, "task_annotate", TaskAnnotateInput{ID: slug, Annotation: "note"})
			if annotateResult.IsError {
				t.Fatalf("task_annotate setup error: %v", parseError(t, annotateResult))
			}
			taskObj := extractTask(t, parseEnvelope(t, annotateResult))
			anns, ok := taskObj["annotations"].([]any)
			if !ok || len(anns) == 0 {
				t.Fatalf("annotations after annotate = %#v, want at least one", taskObj["annotations"])
			}
			firstAnn, ok := anns[0].(map[string]any)
			if !ok {
				t.Fatalf("first annotation type = %T, want object", anns[0])
			}
			annotationID, ok := firstAnn["id"].(string)
			if !ok || annotationID == "" {
				t.Fatalf("first annotation id = %#v, want non-empty string", firstAnn["id"])
			}
			result := callTool(t, session, "task_denotate", TaskDenotateInput{ID: slug, AnnotationID: annotationID})
			if result.IsError {
				t.Fatalf("task_denotate error: %v", parseError(t, result))
			}
		}},
		{name: "task_depends", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "task_depends", TaskDependsInput{ID: slug, Depends: []string{depUUID}})
			if result.IsError {
				t.Fatalf("task_depends error: %v", parseError(t, result))
			}
		}},
		{name: "task_link_add", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "task_link_add", TaskLinkAddInput{Task: slug, Type: "document", URL: "https://example.com/doc"})
			if result.IsError {
				t.Fatalf("task_link_add error: %v", parseError(t, result))
			}
		}},
		{name: "task_link_list", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			if result := callTool(t, session, "task_link_add", TaskLinkAddInput{Task: slug, Type: "document", URL: "https://example.com/doc"}); result.IsError {
				t.Fatalf("task_link_add setup error: %v", parseError(t, result))
			}
			result := callTool(t, session, "task_link_list", TaskLinkListInput{Task: slug})
			if result.IsError {
				t.Fatalf("task_link_list error: %v", parseError(t, result))
			}
		}},
		{name: "task_link_remove", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			add := callTool(t, session, "task_link_add", TaskLinkAddInput{Task: slug, Type: "document", URL: "https://example.com/doc"})
			if add.IsError {
				t.Fatalf("task_link_add setup error: %v", parseError(t, add))
			}
			data := envelopeData(t, parseEnvelope(t, add))
			link := nestedMap(t, data, "link")
			linkID, _ := link["id"].(string)
			if linkID == "" {
				t.Fatalf("link id missing: %#v", link)
			}
			result := callTool(t, session, "task_link_remove", TaskLinkRemoveInput{Task: slug, LinkID: linkID})
			if result.IsError {
				t.Fatalf("task_link_remove error: %v", parseError(t, result))
			}
		}},
		{name: "urgency_explain", run: func(t *testing.T, session *mcp.ClientSession, slug, depUUID string) {
			result := callTool(t, session, "urgency_explain", UrgencyExplainInput{ID: slug})
			if result.IsError {
				t.Fatalf("urgency_explain error: %v", parseError(t, result))
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			session := connectClient(t, srv)
			callTool(t, session, "project_add", ProjectAddInput{Slug: "api", Name: "API"})
			add := callTool(t, session, "task_add", TaskAddInput{Title: "slug task", Project: "api"})
			taskObj := extractTask(t, parseEnvelope(t, add))
			slug, _ := taskObj["task_slug"].(string)
			if slug != "api-1" {
				t.Fatalf("task_slug = %v, want api-1", taskObj["task_slug"])
			}
			depUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "dependency", Project: "api"})))
			tc.run(t, session, slug, depUUID)
		})
	}
}

func TestTaskToolsRejectNumericTaskRefs(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		input any
	}{
		{name: "task_get", tool: "task_get", input: TaskGetInput{ID: "1"}},
		{name: "task_modify", tool: "task_modify", input: TaskModifyInput{ID: "1", Title: ptrStr("updated")}},
		{name: "task_done", tool: "task_done", input: TaskIDInput{ID: "1"}},
		{name: "task_delete", tool: "task_delete", input: TaskIDInput{ID: "1"}},
		{name: "task_start", tool: "task_start", input: TaskIDInput{ID: "1"}},
		{name: "task_stop", tool: "task_stop", input: TaskIDInput{ID: "1"}},
		{name: "task_reopen", tool: "task_reopen", input: TaskIDInput{ID: "1"}},
		{name: "task_annotate", tool: "task_annotate", input: TaskAnnotateInput{ID: "1", Annotation: "note"}},
		{name: "task_denotate", tool: "task_denotate", input: TaskDenotateInput{ID: "1", AnnotationID: "annotation-id"}},
		{name: "task_depends", tool: "task_depends", input: TaskDependsInput{ID: "1", Depends: []string{"00000000-0000-0000-0000-000000000001"}}},
		{name: "task_link_add", tool: "task_link_add", input: TaskLinkAddInput{Task: "1", Type: "document", URL: "https://example.com/doc"}},
		{name: "task_link_list", tool: "task_link_list", input: TaskLinkListInput{Task: "1"}},
		{name: "task_link_remove", tool: "task_link_remove", input: TaskLinkRemoveInput{Task: "1", LinkID: "link"}},
		{name: "urgency_explain", tool: "urgency_explain", input: UrgencyExplainInput{ID: "1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			session := connectClient(t, srv)
			result := callTool(t, session, tc.tool, tc.input)
			if !result.IsError {
				t.Fatalf("%s expected error", tc.tool)
			}
			if got := parseError(t, result).Code; got != "task_ref_invalid" {
				t.Fatalf("%s error code = %q, want task_ref_invalid", tc.tool, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// report.run 集成测试
// ---------------------------------------------------------------------------

func TestReportRun(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task_add", TaskAddInput{Title: "report task"})

	result := callTool(t, session, "report_run", ReportRunInput{Name: "list"})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", env.Data)
	}
	total, _ := dataMap["total"].(float64)
	if int(total) < 1 {
		t.Fatalf("expected at least 1 task, got %d", int(total))
	}
	_ = nestedSlice(t, dataMap, "items")
}

func TestReportRunLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task_add", TaskAddInput{Title: "report task 1"})
	callTool(t, session, "task_add", TaskAddInput{Title: "report task 2"})

	result := callTool(t, session, "report_run", ReportRunInput{Name: "list", Limit: 1})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	tasks := nestedSlice(t, envelopeData(t, parseEnvelope(t, result)), "items")
	if len(tasks) != 1 {
		t.Fatalf("tasks len = %d, want 1", len(tasks))
	}
}

func TestReportRunExpandsOccurrencesWithTaskViewPageSemantics(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	project := callTool(t, session, "project_add", ProjectAddInput{Slug: "ops", Name: "Ops"})
	if project.IsError {
		t.Fatalf("project_add: %v", parseError(t, project))
	}
	firstDue := "2025-06-02"
	series := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", RecurrenceRule: "daily", FirstDueDate: &firstDue,
	})
	if series.IsError {
		t.Fatalf("task_series_add: %v", parseError(t, series))
	}

	result := callTool(t, session, "report_run", ReportRunInput{
		Name: "all", Query: "task_type:occurrence",
		DueAfter: "2025-06-02", DueBefore: "2025-06-03",
		OccurrenceMode: "expand", Sort: "due", Limit: 1, Offset: 1,
	})
	if result.IsError {
		t.Fatalf("report_run: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	items := nestedSlice(t, data, "items")
	if data["total"] != float64(2) || data["limit"] != float64(1) || data["offset"] != float64(1) || data["occurrence_mode"] != "expand" || len(items) != 1 {
		t.Fatalf("report page = %#v", data)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["uuid"] != nil || nestedMap(t, item, "recurrence_info")["materialization"] != "projected" {
		t.Fatalf("report occurrence = %#v", items[0])
	}
}

func TestReportRunDueBeforeAloneFiltersTasksInclusively(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	before := callTool(t, session, "task_add", TaskAddInput{Title: "report before", DueDate: "2025-06-01"})
	if before.IsError {
		t.Fatalf("task_add before: %v", parseError(t, before))
	}
	after := callTool(t, session, "task_add", TaskAddInput{Title: "report after", DueDate: "2030-06-15"})
	if after.IsError {
		t.Fatalf("task_add after: %v", parseError(t, after))
	}

	result := callTool(t, session, "report_run", ReportRunInput{Name: "all", DueBefore: "2025-06-01"})
	if result.IsError {
		t.Fatalf("report_run: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	items := nestedSlice(t, data, "items")
	if data["total"] != float64(1) || len(items) != 1 {
		t.Fatalf("report page = %#v", data)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["title"] != "report before" {
		t.Fatalf("report item = %#v", items[0])
	}
}

func TestReportRunMissingName(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "report_run", ReportRunInput{})
	if !result.IsError {
		t.Fatal("expected IsError=true for missing name")
	}
}

func TestReportRunBadLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "report_run", ReportRunInput{Name: "list", Limit: 9999})
	if !result.IsError {
		t.Fatal("expected IsError=true for bad limit")
	}
	// 验证是我们自己的 handler 返回的 api_bad_limit，不是 SDK 验证
	toolErr := parseError(t, result)
	if toolErr.Code != "api_bad_limit" {
		t.Fatalf("code = %q, want api_bad_limit", toolErr.Code)
	}
}

// ---------------------------------------------------------------------------
// urgency.explain 集成测试
// ---------------------------------------------------------------------------

func TestUrgencyExplain(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "urgency task"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	explainResult := callTool(t, session, "urgency_explain", UrgencyExplainInput{ID: uuid})
	if explainResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, explainResult))
	}
	explainEnv := parseEnvelope(t, explainResult)
	explainData, ok := explainEnv.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", explainEnv.Data)
	}
	if explainData["id"] != uuid || explainData["uuid"] != uuid {
		t.Fatalf("identity = %#v, want id/uuid %s", explainData, uuid)
	}
	if _, ok := explainData["total"].(float64); !ok {
		t.Fatalf("total = %#v", explainData["total"])
	}
	if _, ok := explainData["items"].([]any); !ok {
		t.Fatalf("items = %#v", explainData["items"])
	}
	if _, exists := explainData["urgency"]; exists {
		t.Fatalf("legacy urgency key remains: %#v", explainData)
	}
}

// ---------------------------------------------------------------------------
// 写操作 audit 验证
// ---------------------------------------------------------------------------

func TestWriteOperationsCreateAuditEntries(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	// 创建任务（写操作应产生 audit）
	addResult := callTool(t, session, "task_add", TaskAddInput{Title: "audit check"})
	if addResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, addResult))
	}
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	// done 应产生 audit
	doneResult := callTool(t, session, "task_done", TaskIDInput{ID: uuid})
	if doneResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, doneResult))
	}

	// 创建新任务并删除
	uuid2 := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "delete audit"})))

	delResult := callTool(t, session, "task_delete", TaskIDInput{ID: uuid2})
	if delResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, delResult))
	}

	audits, err := svc.ListAudit(app.AuditListInput{Limit: 50})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	var sawAdd, sawDone, sawDelete bool
	for _, row := range audits {
		sawAdd = sawAdd || row.Action == "task.add"
		sawDone = sawDone || row.Action == "task.done"
		sawDelete = sawDelete || row.Action == "task.delete"
	}
	if !sawAdd || !sawDone || !sawDelete {
		t.Fatalf("audit did not include task.add/task.done/task.delete: %#v", audits)
	}
}

// ---------------------------------------------------------------------------
// workspace/project/context/config tools 集成测试
// ---------------------------------------------------------------------------

func TestMCPWorkspaceTools(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if _, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"}); err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	list := callTool(t, session, "workspace_list", WorkspaceListInput{})
	if list.IsError {
		t.Fatalf("workspace.list error: %v", parseError(t, list))
	}
	workspaces := nestedSlice(t, envelopeData(t, parseEnvelope(t, list)), "workspaces")
	if len(workspaces) < 2 {
		t.Fatalf("workspace.list returned %d workspace(s), want at least 2", len(workspaces))
	}

	current := callTool(t, session, "workspace_get_current", WorkspaceCurrentInput{Workspace: "team"})
	if current.IsError {
		t.Fatalf("workspace.current error: %v", parseError(t, current))
	}
	workspace := nestedMap(t, envelopeData(t, parseEnvelope(t, current)), "workspace")
	if workspace["slug"] != "team" {
		t.Fatalf("workspace slug = %v, want team", workspace["slug"])
	}
}

func TestMCPProjectTools(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	project, err := svc.AddProject(app.AddProjectInput{Slug: "agent", Name: "Agent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectConfigSet(project.Slug, "agent.background", "Background"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectConfigSet(project.Slug, "context.default", "legacy"); err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	list := callTool(t, session, "project_list", ProjectListInput{})
	if list.IsError {
		t.Fatalf("project.list error: %v", parseError(t, list))
	}
	projects := nestedSlice(t, envelopeData(t, parseEnvelope(t, list)), "projects")
	if len(projects) != 1 {
		t.Fatalf("project.list count = %d, want 1", len(projects))
	}
	listProject, ok := projects[0].(map[string]any)
	if !ok {
		t.Fatalf("project.list item = %#v, want object", projects[0])
	}
	if listProject["task_count_scope"] != "all_tasks" {
		t.Fatalf("project list task_count_scope = %v, want all_tasks", listProject["task_count_scope"])
	}
	if listProject["url"] != "/workspaces/local/projects/agent" {
		t.Fatalf("project list url = %#v", listProject["url"])
	}

	got := callTool(t, session, "project_get", ProjectGetInput{ProjectID: project.ID})
	if got.IsError {
		t.Fatalf("project.get error: %v", parseError(t, got))
	}
	data := envelopeData(t, parseEnvelope(t, got))
	projectData := nestedMap(t, data, "project")
	if projectData["slug"] != "agent" {
		t.Fatalf("project slug = %v, want agent", projectData["slug"])
	}
	if projectData["url"] != "/workspaces/local/projects/agent" {
		t.Fatalf("project url = %#v", projectData["url"])
	}
	if projectData["task_count_scope"] != "all_tasks" {
		t.Fatalf("project task_count_scope = %v, want all_tasks", projectData["task_count_scope"])
	}
	configSummary := nestedMap(t, data, "config_summary")
	if configSummary["agent.background"] != "Background" {
		t.Fatalf("agent.background = %v, want Background", configSummary["agent.background"])
	}
	if configSummary["context.default"] != "legacy" {
		t.Fatalf("context.default = %v, want legacy (non-secret keys are exposed)", configSummary["context.default"])
	}

	missing := callTool(t, session, "project_get", ProjectGetInput{})
	if !missing.IsError {
		t.Fatal("project.get without project or project_id should fail")
	}

	current := callTool(t, session, "project_get_current", ProjectCurrentInput{})
	if current.IsError {
		t.Fatalf("project.current error: %v", parseError(t, current))
	}
	if envelopeData(t, parseEnvelope(t, current))["project"] != nil {
		t.Fatal("project.current without explicit scope should return null project")
	}

	current = callTool(t, session, "project_get_current", ProjectCurrentInput{ProjectID: project.ID})
	if current.IsError {
		t.Fatalf("project.current with scope error: %v", parseError(t, current))
	}
	currentProject := nestedMap(t, envelopeData(t, parseEnvelope(t, current)), "project")
	if currentProject["task_count_scope"] != "all_tasks" {
		t.Fatalf("current project task_count_scope = %v, want all_tasks", currentProject["task_count_scope"])
	}
}

func TestMCPContextTools(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if err := svc.DefineContext("sprint", "priority:H"); err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	set := callTool(t, session, "context_set", ContextSetInput{Name: "sprint"})
	if set.IsError {
		t.Fatalf("context.set error: %v", parseError(t, set))
	}
	show := callTool(t, session, "context_get", ContextShowInput{})
	if show.IsError {
		t.Fatalf("context.show error: %v", parseError(t, show))
	}
	contextData := nestedMap(t, envelopeData(t, parseEnvelope(t, show)), "context")
	if contextData["name"] != "sprint" || contextData["filter"] != "priority:H" || contextData["active"] != true {
		t.Fatalf("context = %#v, want active sprint priority:H", contextData)
	}

	clear := callTool(t, session, "context_set", ContextSetInput{Name: "none"})
	if clear.IsError {
		t.Fatalf("context.set none error: %v", parseError(t, clear))
	}
	if envelopeData(t, parseEnvelope(t, clear))["context"] != nil {
		t.Fatal("context.set none should return null context")
	}
}

func TestMCPConfigTools(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	project, err := svc.AddProject(app.AddProjectInput{Slug: "agent", Name: "Agent"})
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWithOptions(t, Options{
		Store: store,
		Mode:  ModeStdio,
		LocalRuntimeValues: map[string]string{
			"remote.server": "http://127.0.0.1:8080",
			"date.format":   "epoch",
		},
	})
	session := connectClient(t, srv)

	local := callTool(t, session, "config_get", ConfigGetInput{Scope: "local", Key: "remote.server"})
	if local.IsError {
		t.Fatalf("config.get local error: %v", parseError(t, local))
	}
	if got := envelopeData(t, parseEnvelope(t, local))["value"]; got != "http://127.0.0.1:8080" {
		t.Fatalf("local remote.server = %v", got)
	}

	workspacePersonal := callTool(t, session, "config_set", ConfigSetInput{Scope: "workspace", Key: "color", Value: "auto"})
	if !workspacePersonal.IsError {
		t.Fatal("config.set workspace color should fail")
	}
	if code := parseError(t, workspacePersonal).Code; code != "config_key_unsupported" {
		t.Fatalf("code = %q, want config_key_unsupported", code)
	}
	workspaceAgent := callTool(t, session, "config_set", ConfigSetInput{Scope: "workspace", Key: "agent.background", Value: "x"})
	if !workspaceAgent.IsError {
		t.Fatal("config.set workspace agent.background should fail")
	}
	if code := parseError(t, workspaceAgent).Code; code != "config_key_unsupported" {
		t.Fatalf("code = %q, want config_key_unsupported", code)
	}

	projectSet := callTool(t, session, "config_set", ConfigSetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff", Value: "handoff"})
	if projectSet.IsError {
		t.Fatalf("config.set project error: %v", parseError(t, projectSet))
	}
	projectGet := callTool(t, session, "config_get", ConfigGetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff"})
	if projectGet.IsError {
		t.Fatalf("config.get project error: %v", parseError(t, projectGet))
	}
	if got := envelopeData(t, parseEnvelope(t, projectGet))["value"]; got != "handoff" {
		t.Fatalf("agent.handoff = %v, want handoff", got)
	}
}

func TestMCPConfigGetLocalRejectedInHTTPMode(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	token := mustCreateMCPToken(t, svc, []string{"config:read"}, []string{"local"}, nil)
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	result := callTool(t, session, "config_get", ConfigGetInput{Scope: "local", Key: "date.format"})
	if !result.IsError {
		t.Fatal("config.get local in HTTP mode should fail")
	}
	if code := parseError(t, result).Code; code != "config_scope_invalid" {
		t.Fatalf("code = %q, want config_scope_invalid", code)
	}
}

func TestMCPTenantAccessTokenHTTPMode(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:write", "workspace:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	add := callTool(t, session, "task_add", TaskAddInput{Title: "tenant mcp task"})
	if add.IsError {
		t.Fatalf("task_add error: %v", parseError(t, add))
	}
	me := callTool(t, session, "me_get", MeGetInput{})
	if !me.IsError {
		t.Fatal("me_get with tenant token should fail")
	}
	if code := parseError(t, me).Code; code != "tenant_actor_not_user" {
		t.Fatalf("me_get code = %q, want tenant_actor_not_user", code)
	}

	assigned := callTool(t, session, "task_add", TaskAddInput{Title: "bad assignee", Assignees: []string{"me"}})
	if !assigned.IsError {
		t.Fatal("task_add assignees=me with tenant token should fail")
	}
	if code := parseError(t, assigned).Code; code != "tenant_actor_not_user" {
		t.Fatalf("task_add assignees=me code = %q, want tenant_actor_not_user", code)
	}
}

func TestMCPTenantAccessTokenCanQueryTasksByExplicitAssignee(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if _, err := svc.AddUser(app.AddUserInput{Name: "tenant-assignee"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMember(app.AddMemberInput{UserRef: "tenant-assignee", Role: app.RoleMember}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "assigned tenant-visible task", Assignees: []string{"tenant-assignee"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "unassigned tenant-visible task"}); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime-reader",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	result := callTool(t, session, "task_query", TaskQueryInput{Query: "assignee:tenant-assignee"})
	if result.IsError {
		t.Fatalf("task_query assignee error: %v", parseError(t, result))
	}
	tasks := nestedSlice(t, envelopeData(t, parseEnvelope(t, result)), "tasks")
	if len(tasks) != 1 {
		t.Fatalf("tasks len = %d, want 1", len(tasks))
	}
	taskObj, ok := tasks[0].(map[string]any)
	if !ok {
		t.Fatalf("task type = %T, want map", tasks[0])
	}
	if taskObj["title"] != "assigned tenant-visible task" {
		t.Fatalf("task title = %v, want assigned tenant-visible task", taskObj["title"])
	}
}

func TestMCPTenantAccessTokenTaskQueryIgnoresActorlessActiveContextState(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if err := svc.DefineContext("tenant-filter", "+ctx"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMeta("active_context.."+svc.Runtime().WorkspaceID, "tenant-filter"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "context-tagged task", Tags: []string{"ctx"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "plain tenant task"}); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime-reader",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	result := callTool(t, session, "task_query", TaskQueryInput{})
	if result.IsError {
		t.Fatalf("task_query error: %v", parseError(t, result))
	}
	tasks := nestedSlice(t, envelopeData(t, parseEnvelope(t, result)), "tasks")
	if len(tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2; tasks=%#v", len(tasks), tasks)
	}
}

func TestMCPTenantAccessTokenCanManageTenantOwnerTools(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name: "tenant-owner",
		Scopes: []string{
			"user:read", "user:write",
			"member:read", "member:write",
			"token:read", "token:write",
			"workspace:read", "workspace:write",
			"context:read", "context:write",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	addUser := callTool(t, session, "user_add", UserAddInput{Name: "tenant-managed", DisplayName: "Tenant Managed"})
	if addUser.IsError {
		t.Fatalf("user_add error: %v", parseError(t, addUser))
	}
	allowedCalls := []struct {
		name  string
		input any
	}{
		{name: "user_list", input: UserListInput{}},
		{name: "user_get", input: UserInfoInput{User: "tenant-managed"}},
		{name: "user_bind", input: UserBindInput{User: "tenant-managed", Provider: "feishu", UserType: "user_id", ExternalID: "tenant_user_1"}},
		{name: "user_list_external_ids", input: UserRefInput{User: "tenant-managed"}},
		{name: "user_unbind", input: UserUnbindInput{User: "tenant-managed", Provider: "feishu", ExternalID: "tenant_user_1"}},
		{name: "member_list", input: MemberListInput{}},
		{name: "member_add", input: MemberAddInput{User: "tenant-managed", Role: "member"}},
		{name: "member_role", input: MemberRoleInput{User: "tenant-managed", Role: "admin"}},
		{name: "workspace_info", input: WorkspaceRefInput{Workspace: "local"}},
		{name: "workspace_modify", input: WorkspaceModifyInput{Workspace: "local", Name: ptrStr("Local Tenant")}},
		{name: "token_list", input: TokenListInput{}},
	}
	for _, call := range allowedCalls {
		result := callTool(t, session, call.name, call.input)
		if result.IsError {
			t.Fatalf("%s error: %v", call.name, parseError(t, result))
		}
	}

	createToken := callTool(t, session, "token_create", TokenCreateInput{Name: "child-tenant", Scope: []string{"token:read"}})
	if createToken.IsError {
		t.Fatalf("token_create error: %v", parseError(t, createToken))
	}
	tokenObj := nestedMap(t, envelopeData(t, parseEnvelope(t, createToken)), "token")
	tokenID, _ := tokenObj["id"].(string)
	if tokenID == "" {
		t.Fatalf("created token id = %v, want non-empty string", tokenObj["id"])
	}
	if tokenObj["type"] != "tenant_access_token" {
		t.Fatalf("created token type = %v, want tenant_access_token", tokenObj["type"])
	}
	if _, ok := tokenObj["raw_token"].(string); !ok {
		t.Fatalf("created token raw_token = %T, want string", tokenObj["raw_token"])
	}

	modifyToken := callTool(t, session, "token_modify", TokenModifyInput{TokenRef: tokenID, Name: ptrStr("renamed-child-tenant")})
	if modifyToken.IsError {
		t.Fatalf("token_modify error: %v", parseError(t, modifyToken))
	}
	revokeToken := callTool(t, session, "token_revoke", TokenRevokeInput{TokenRef: tokenID})
	if revokeToken.IsError {
		t.Fatalf("token_revoke error: %v", parseError(t, revokeToken))
	}

	forbidden := map[string]any{
		"me_get":         MeGetInput{},
		"user_use":       UserUseInput{User: "tenant-managed"},
		"workspace_use":  WorkspaceRefInput{Workspace: "local"},
		"workspace_list": WorkspaceListInput{},
		"context_set":    ContextSetInput{Name: "tenant"},
		"context_none":   ContextShowInput{},
	}
	for name, input := range forbidden {
		result := callTool(t, session, name, input)
		if !result.IsError {
			t.Fatalf("%s with tenant token should fail", name)
		}
	}

	for _, scope := range []string{"impersonate"} {
		forbiddenScope := callTool(t, session, "token_create", TokenCreateInput{Name: "bad-scope", Scope: []string{scope}})
		if !forbiddenScope.IsError {
			t.Fatalf("token_create with %s tenant scope should fail", scope)
		}
		if code := parseError(t, forbiddenScope).Code; code != "tenant_token_scope_invalid" {
			t.Fatalf("token_create forbidden scope code = %q, want tenant_token_scope_invalid", code)
		}
	}
}

func TestMCPTenantAccessTokenScopeGatesUserMemberTools(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	userScoped, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "tenant-user-writer",
		Scopes: []string{"user:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+userScoped.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	addUser := callTool(t, session, "user_add", UserAddInput{Name: "tenant-scope-user"})
	if addUser.IsError {
		t.Fatalf("user_add with user:write error: %v", parseError(t, addUser))
	}
	listUsers := callTool(t, session, "user_list", UserListInput{})
	if !listUsers.IsError || parseError(t, listUsers).Code != "token_scope_denied" {
		t.Fatalf("user_list with only user:write = %#v, want token_scope_denied", parseError(t, listUsers))
	}

	workspaceScoped, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "tenant-workspace-writer",
		Scopes: []string{"workspace:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req2, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req2.Header.Set("Authorization", "Bearer "+workspaceScoped.RawToken)
	srv2, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req2})
	session2 := connectClient(t, srv2)
	if result := callTool(t, session2, "user_add", UserAddInput{Name: "wrong-scope-user"}); !result.IsError {
		t.Fatal("user_add with workspace:write should fail")
	} else if code := parseError(t, result).Code; code != "token_scope_denied" {
		t.Fatalf("user_add with workspace:write code=%q, want token_scope_denied", code)
	}
	if result := callTool(t, session2, "member_add", MemberAddInput{User: "tenant-scope-user", Role: "member"}); !result.IsError {
		t.Fatal("member_add with workspace:write should fail")
	} else if code := parseError(t, result).Code; code != "token_scope_denied" {
		t.Fatalf("member_add with workspace:write code=%q, want token_scope_denied", code)
	}

	memberScoped, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "tenant-member-writer",
		Scopes: []string{"member:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req3, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req3.Header.Set("Authorization", "Bearer "+memberScoped.RawToken)
	srv3, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req3})
	session3 := connectClient(t, srv3)
	addMember := callTool(t, session3, "member_add", MemberAddInput{User: "tenant-scope-user", Role: "member"})
	if addMember.IsError {
		t.Fatalf("member_add with member:write error: %v", parseError(t, addMember))
	}
}

func TestMCPBearerTokenCreateRevokeRespectsParentScope(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	limited, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "limited-manager",
		Type:          "agent",
		Scopes:        []string{"token:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"}); err != nil {
		t.Fatal(err)
	}
	target, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "team-token",
		Type:          "agent",
		Scopes:        []string{"token:write"},
		WorkspaceRefs: []string{"team"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+limited.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	createEscalated := callTool(t, session, "token_create", TokenCreateInput{Name: "bad-child", Scope: []string{"token:write", "task:read"}})
	if !createEscalated.IsError {
		t.Fatal("token_create exceeding bearer scope should fail")
	}
	if code := parseError(t, createEscalated).Code; code != "token_scope_denied" {
		t.Fatalf("token_create code = %q, want token_scope_denied", code)
	}

	revokeOutsideWorkspace := callTool(t, session, "token_revoke", TokenRevokeInput{TokenRef: target.View.ID})
	if !revokeOutsideWorkspace.IsError {
		t.Fatal("token_revoke outside bearer workspace scope should fail")
	}
	if code := parseError(t, revokeOutsideWorkspace).Code; code != "workspace_scope_denied" {
		t.Fatalf("token_revoke code = %q, want workspace_scope_denied", code)
	}
}

func TestMCPTokenRevokeRespectsToolProjectScope(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	projectOne, err := svc.AddProject(app.AddProjectInput{Slug: "one", Name: "One"})
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := svc.AddProject(app.AddProjectInput{Slug: "two", Name: "Two"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "project-manager",
		Type:          "agent",
		Scopes:        []string{"token:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{"one", "two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "two-token",
		Type:          "agent",
		Scopes:        []string{"token:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{"two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+parent.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	revokeOutsideToolProject := callTool(t, session, "token_revoke", TokenRevokeInput{Project: "one", TokenRef: target.View.ID})
	if !revokeOutsideToolProject.IsError {
		t.Fatalf("token_revoke with tool project %s should not revoke project %s token", projectOne.ID, projectTwo.ID)
	}
	if code := parseError(t, revokeOutsideToolProject).Code; code != "project_scope_denied" {
		t.Fatalf("token_revoke code = %q, want project_scope_denied", code)
	}
}

func TestMCPTenantAccessTokenCanCreateP2SystemActorResources(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	project, err := svc.AddProject(app.AddProjectInput{Slug: "tenantp2", Name: "Tenant P2"})
	if err != nil {
		t.Fatal(err)
	}
	taskRow, err := svc.Add(app.AddInput{Title: "tenant p2 link target", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name: "tenant-p2",
		Scopes: []string{
			"task:write",
			"project:write",
			"hook:write",
			"notification:write",
			"reminder:write",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	addSink := callTool(t, session, "notification_sink_add", NotificationSinkAddInput{
		Name:         "tenant-p2-sink",
		Type:         app.NotificationSinkTypeWebhook,
		EndpointMode: app.NotificationEndpointStaticURL,
		URL:          "https://example.com/webhook",
		AllowedHosts: []string{"example.com"},
		Secret:       "tenant-secret",
	})
	if addSink.IsError {
		t.Fatalf("notification_sink_add error: %v", parseError(t, addSink))
	}
	sink := nestedMap(t, envelopeData(t, parseEnvelope(t, addSink)), "sink")
	assertMCPSystemActor(t, sink["created_by"], created.View.ID)

	addHook := callTool(t, session, "hook_add", HookAddInput{
		Name:   "tenant-p2-hook",
		Sink:   "tenant-p2-sink",
		Events: []string{"task.created"},
	})
	if addHook.IsError {
		t.Fatalf("hook_add error: %v", parseError(t, addHook))
	}
	hook := nestedMap(t, envelopeData(t, parseEnvelope(t, addHook)), "hook")
	assertMCPSystemActor(t, hook["created_by"], created.View.ID)

	addReminder := callTool(t, session, "reminder_rule_add", ReminderRuleAddInput{
		Name:         "tenant-p2-reminder",
		TriggerType:  "overdue",
		RepeatPolicy: "once",
		AudienceType: "explicit_users",
		Recipients:   []string{"local"},
		Sink:         "tenant-p2-sink",
	})
	if addReminder.IsError {
		t.Fatalf("reminder_rule_add error: %v", parseError(t, addReminder))
	}
	reminder := nestedMap(t, envelopeData(t, parseEnvelope(t, addReminder)), "rule")
	assertMCPSystemActor(t, reminder["created_by"], created.View.ID)

	addNotificationRule := callTool(t, session, "notification_rule_add", NotificationRuleAddInput{
		Name:            "tenant-p2-notification",
		Event:           "task.created",
		Audience:        "actor",
		Sink:            "tenant-p2-sink",
		TemplateSubject: "Task created",
		TemplateBody:    "{{event.type}}",
	})
	if addNotificationRule.IsError {
		t.Fatalf("notification_rule_add error: %v", parseError(t, addNotificationRule))
	}
	notificationRule := nestedMap(t, envelopeData(t, parseEnvelope(t, addNotificationRule)), "rule")
	assertMCPSystemActor(t, notificationRule["created_by"], created.View.ID)

	addLink := callTool(t, session, "task_link_add", TaskLinkAddInput{
		Task:  taskRow.UUID,
		Type:  "document",
		URL:   "https://example.com/spec",
		Title: "Spec",
	})
	if addLink.IsError {
		t.Fatalf("task_link_add error: %v", parseError(t, addLink))
	}
	link := nestedMap(t, envelopeData(t, parseEnvelope(t, addLink)), "link")
	assertMCPSystemActor(t, link["created_by"], created.View.ID)

	addAnnotation := callTool(t, session, "project_annotate", ProjectAnnotateInput{
		Project: project.Slug,
		Content: "tenant p2 annotation",
	})
	if addAnnotation.IsError {
		t.Fatalf("project_annotate error: %v", parseError(t, addAnnotation))
	}
	annotation := nestedMap(t, envelopeData(t, parseEnvelope(t, addAnnotation)), "annotation")
	assertMCPSystemActor(t, annotation["created_by"], created.View.ID)
}

func TestMCPProjectConfigUsesConfigCapability(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	project, err := svc.AddProject(app.AddProjectInput{Slug: "agent", Name: "Agent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectConfigSet(project.Slug, "agent.handoff", "handoff"); err != nil {
		t.Fatal(err)
	}

	projectOnly := mustCreateMCPToken(t, svc, []string{"project:read", "project:write"}, []string{"local"}, nil)
	projectReq, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	projectReq.Header.Set("Authorization", "Bearer "+projectOnly)
	projectSrv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: projectReq})
	projectSession := connectClient(t, projectSrv)
	projectRead := callTool(t, projectSession, "config_get", ConfigGetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff"})
	if !projectRead.IsError {
		t.Fatal("config.get with project:read but without config:read should fail")
	}
	if code := parseError(t, projectRead).Code; code != "token_scope_denied" {
		t.Fatalf("config.get project-only code = %q, want token_scope_denied", code)
	}

	configToken := mustCreateMCPToken(t, svc, []string{"config:read", "config:write"}, []string{"local"}, nil)
	configReq, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	configReq.Header.Set("Authorization", "Bearer "+configToken)
	configSrv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: configReq})
	configSession := connectClient(t, configSrv)
	configRead := callTool(t, configSession, "config_get", ConfigGetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff"})
	if configRead.IsError {
		t.Fatalf("config.get with config:read error: %v", parseError(t, configRead))
	}
	configWrite := callTool(t, configSession, "config_set", ConfigSetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff", Value: "updated"})
	if configWrite.IsError {
		t.Fatalf("config.set with config:write error: %v", parseError(t, configWrite))
	}
}

func TestMCPAgentFlow(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	project, err := svc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectConfigSet(project.Slug, "agent.background", "remote docs"); err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	projectGet := callTool(t, session, "project_get", ProjectGetInput{ProjectID: project.ID})
	if projectGet.IsError {
		t.Fatalf("project.get error: %v", parseError(t, projectGet))
	}
	configSummary := nestedMap(t, envelopeData(t, parseEnvelope(t, projectGet)), "config_summary")
	if configSummary["agent.background"] != "remote docs" {
		t.Fatalf("agent.background = %v, want remote docs", configSummary["agent.background"])
	}

	queryEmpty := callTool(t, session, "task_query", TaskQueryInput{ProjectID: project.ID})
	if queryEmpty.IsError {
		t.Fatalf("task.query empty error: %v", parseError(t, queryEmpty))
	}
	if count := envelopeData(t, parseEnvelope(t, queryEmpty))["count"]; count != float64(0) {
		t.Fatalf("empty project task count = %v, want 0", count)
	}

	add := callTool(t, session, "task_add", TaskAddInput{Title: "ship MCP", ProjectID: project.ID})
	if add.IsError {
		t.Fatalf("task.add error: %v", parseError(t, add))
	}
	uuid := extractUUID(t, parseEnvelope(t, add))
	explain := callTool(t, session, "urgency_explain", UrgencyExplainInput{ID: uuid})
	if explain.IsError {
		t.Fatalf("urgency.explain error: %v", parseError(t, explain))
	}
	done := callTool(t, session, "task_done", TaskIDInput{ID: uuid})
	if done.IsError {
		t.Fatalf("task.done error: %v", parseError(t, done))
	}

	audits, err := svc.ListAudit(app.AuditListInput{Limit: 50})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	var sawAdd, sawDone bool
	for _, row := range audits {
		sawAdd = sawAdd || row.Action == "task.add"
		sawDone = sawDone || row.Action == "task.done"
	}
	if !sawAdd || !sawDone {
		t.Fatalf("audit did not include task.add and task.done: %#v", audits)
	}
}

func TestMCPProjectScope(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	projectA, err := owner.AddProject(app.AddProjectInput{Slug: "apia", Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := owner.AddProject(app.AddProjectInput{Slug: "apib", Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	taskB, err := owner.Add(app.AddInput{Title: "hidden", Project: &projectB.Slug})
	if err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, owner, []string{"task:read", "project:read"}, []string{"local"}, []string{projectA.ID})
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	query := callTool(t, session, "task_query", TaskQueryInput{ProjectID: projectB.ID})
	if !query.IsError {
		t.Fatal("task.query outside project allowlist should fail")
	}
	if code := parseError(t, query).Code; code != "project_scope_denied" {
		t.Fatalf("task.query code = %q, want project_scope_denied", code)
	}

	get := callTool(t, session, "task_get", TaskGetInput{ID: taskB.UUID})
	if !get.IsError {
		t.Fatal("task.get outside project allowlist should fail")
	}
	if code := parseError(t, get).Code; code != "task_not_found" {
		t.Fatalf("task.get code = %q, want task_not_found", code)
	}

	list := callTool(t, session, "project_list", ProjectListInput{})
	if list.IsError {
		t.Fatalf("project.list error: %v", parseError(t, list))
	}
	projects := nestedSlice(t, envelopeData(t, parseEnvelope(t, list)), "projects")
	if len(projects) != 1 {
		t.Fatalf("project.list count = %d, want 1", len(projects))
	}
	projectMap, ok := projects[0].(map[string]any)
	if !ok {
		t.Fatalf("project.list item type = %T, want map", projects[0])
	}
	if projectMap["slug"] != projectA.Slug {
		t.Fatalf("project.list projects = %#v, want only project A", projects)
	}
}

// ---------------------------------------------------------------------------
// task.denotate / task.link_list / task.export / task.import 集成测试
// ---------------------------------------------------------------------------

func TestTaskDenotate(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	uuid := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "denotate me"})))
	callTool(t, session, "task_annotate", TaskAnnotateInput{ID: uuid, Annotation: "note 1"})
	callTool(t, session, "task_annotate", TaskAnnotateInput{ID: uuid, Annotation: "note 2"})

	getResult := callTool(t, session, "task_get", TaskGetInput{ID: uuid})
	if getResult.IsError {
		t.Fatalf("task_get error: %v", parseError(t, getResult))
	}
	taskBefore := extractTask(t, parseEnvelope(t, getResult))
	annsBefore, ok := taskBefore["annotations"].([]any)
	if !ok || len(annsBefore) != 2 {
		t.Fatalf("annotations before denotate = %#v, want 2", taskBefore["annotations"])
	}
	firstAnn, ok := annsBefore[0].(map[string]any)
	if !ok {
		t.Fatalf("first annotation type = %T, want object", annsBefore[0])
	}
	annotationID, ok := firstAnn["id"].(string)
	if !ok || annotationID == "" {
		t.Fatalf("first annotation id = %#v, want non-empty string", firstAnn["id"])
	}

	result := callTool(t, session, "task_denotate", TaskDenotateInput{ID: uuid, AnnotationID: annotationID})
	if result.IsError {
		t.Fatalf("task_denotate error: %v", parseError(t, result))
	}
	taskObj := extractTask(t, parseEnvelope(t, result))
	anns, ok := taskObj["annotations"].([]any)
	if !ok || len(anns) != 1 {
		t.Fatalf("annotations after denotate = %#v, want 1", taskObj["annotations"])
	}
}

func TestTaskLinkList(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	uuid := extractUUID(t, parseEnvelope(t, callTool(t, session, "task_add", TaskAddInput{Title: "link test"})))
	callTool(t, session, "task_link_add", TaskLinkAddInput{Task: uuid, Type: "document", URL: "https://example.com/doc"})

	result := callTool(t, session, "task_link_list", TaskLinkListInput{Task: uuid})
	if result.IsError {
		t.Fatalf("task_link_list error: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	count, _ := data["count"].(float64)
	if int(count) != 1 {
		t.Fatalf("link count = %v, want 1", count)
	}
}

func TestTaskExportImport(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task_add", TaskAddInput{Title: "export me"})

	exportResult := callTool(t, session, "task_export", TaskExportInput{})
	if exportResult.IsError {
		t.Fatalf("task_export error: %v", parseError(t, exportResult))
	}
	exportData := envelopeData(t, parseEnvelope(t, exportResult))
	taskCount, _ := exportData["task_count"].(float64)
	if int(taskCount) < 1 {
		t.Fatalf("export task_count = %v, want at least 1", taskCount)
	}

	// native bundle import（spec §20）。
	importResult := callTool(t, session, "task_import", TaskImportInput{
		Bundle: app.TaskBundleV1{
			Schema: "xuanchu.task-bundle/v1",
			Tasks: []app.TaskBundleTask{
				{UUID: "imported-uuid-1", Title: "imported task", Status: "pending", Entry: 1748736000, Modified: 1748736000},
			},
		},
	})
	if importResult.IsError {
		t.Fatalf("task_import error: %v", parseError(t, importResult))
	}
	importData := envelopeData(t, parseEnvelope(t, importResult))
	imported, _ := importData["imported_tasks"].(float64)
	if int(imported) != 1 {
		t.Fatalf("imported = %v, want 1", imported)
	}
}

// ---------------------------------------------------------------------------
// workspace 补充工具集成测试
// ---------------------------------------------------------------------------

func TestWorkspaceAddInfoModifyArchiveUse(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	add := callTool(t, session, "workspace_add", WorkspaceAddInput{Slug: "test-ws", Name: "Test Workspace"})
	if add.IsError {
		t.Fatalf("workspace_add error: %v", parseError(t, add))
	}
	wsData := nestedMap(t, envelopeData(t, parseEnvelope(t, add)), "workspace")
	if wsData["slug"] != "test-ws" {
		t.Fatalf("slug = %v, want test-ws", wsData["slug"])
	}

	info := callTool(t, session, "workspace_info", WorkspaceRefInput{Workspace: "test-ws"})
	if info.IsError {
		t.Fatalf("workspace_info error: %v", parseError(t, info))
	}

	mod := callTool(t, session, "workspace_modify", WorkspaceModifyInput{Workspace: "test-ws", Name: ptrStr("Updated Name")})
	if mod.IsError {
		t.Fatalf("workspace_modify error: %v", parseError(t, mod))
	}
	modWs := nestedMap(t, envelopeData(t, parseEnvelope(t, mod)), "workspace")
	if modWs["name"] != "Updated Name" {
		t.Fatalf("name = %v, want Updated Name", modWs["name"])
	}

	use := callTool(t, session, "workspace_use", WorkspaceRefInput{Workspace: "test-ws"})
	if use.IsError {
		t.Fatalf("workspace_use error: %v", parseError(t, use))
	}

	archive := callTool(t, session, "workspace_archive", WorkspaceRefInput{Workspace: "test-ws"})
	if archive.IsError {
		t.Fatalf("workspace_archive error: %v", parseError(t, archive))
	}

	list := callTool(t, session, "workspace_list", WorkspaceListInput{IncludeArchived: true})
	if list.IsError {
		t.Fatalf("workspace_list error: %v", parseError(t, list))
	}
	_ = svc
}

// ---------------------------------------------------------------------------
// project 补充工具集成测试
// ---------------------------------------------------------------------------

func TestProjectAddModifyArchive(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	add := callTool(t, session, "project_add", ProjectAddInput{Slug: "myproj", Name: "My Project"})
	if add.IsError {
		t.Fatalf("project_add error: %v", parseError(t, add))
	}
	projData := nestedMap(t, envelopeData(t, parseEnvelope(t, add)), "project")
	if projData["slug"] != "myproj" {
		t.Fatalf("slug = %v, want myproj", projData["slug"])
	}

	mod := callTool(t, session, "project_modify", ProjectModifyInput{Project: "myproj", Name: ptrStr("Updated Project")})
	if mod.IsError {
		t.Fatalf("project_modify error: %v", parseError(t, mod))
	}
	modProj := nestedMap(t, envelopeData(t, parseEnvelope(t, mod)), "project")
	if modProj["name"] != "Updated Project" {
		t.Fatalf("name = %v, want Updated Project", modProj["name"])
	}

	archive := callTool(t, session, "project_archive", ProjectArchiveInput{Project: "myproj"})
	if archive.IsError {
		t.Fatalf("project_archive error: %v", parseError(t, archive))
	}
}

func TestProjectTransitionAndListStatus(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "project_add", ProjectAddInput{Slug: "trproj", Name: "TR Project"})

	// 新建默认 planning
	get := callTool(t, session, "project_get", ProjectGetInput{Project: "trproj"})
	projData := nestedMap(t, envelopeData(t, parseEnvelope(t, get)), "project")
	if projData["status"] != "planning" {
		t.Fatalf("new project status = %v, want planning", projData["status"])
	}

	// project_transition planning -> active
	tr := callTool(t, session, "project_transition", ProjectTransitionInput{Project: "trproj", Status: "active"})
	if tr.IsError {
		t.Fatalf("project_transition error: %v", parseError(t, tr))
	}
	trProj := nestedMap(t, envelopeData(t, parseEnvelope(t, tr)), "project")
	if trProj["status"] != "active" {
		t.Fatalf("status = %v, want active", trProj["status"])
	}

	// project_list status=active 含 trproj；status=planning 不含
	activeList := callTool(t, session, "project_list", ProjectListInput{Status: "active"})
	activeData := envelopeData(t, parseEnvelope(t, activeList))
	if count, _ := activeData["count"].(float64); count != 1 {
		t.Fatalf("project_list status=active count = %v, want 1", activeData["count"])
	}
	planningList := callTool(t, session, "project_list", ProjectListInput{Status: "planning"})
	planningData := envelopeData(t, parseEnvelope(t, planningList))
	if count, _ := planningData["count"].(float64); count != 0 {
		t.Fatalf("project_list status=planning count = %v, want 0", planningData["count"])
	}
}

func TestProjectAnnotateDenotateListTimeline(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "project_add", ProjectAddInput{Slug: "annproj", Name: "Ann Project"})

	ann := callTool(t, session, "project_annotate", ProjectAnnotateInput{Project: "annproj", Content: "project note"})
	if ann.IsError {
		t.Fatalf("project_annotate error: %v", parseError(t, ann))
	}
	annData := envelopeData(t, parseEnvelope(t, ann))
	annotationObj, ok := annData["annotation"].(map[string]any)
	if !ok {
		t.Fatalf("annotation type = %T, want map", annData["annotation"])
	}
	annID, _ := annotationObj["id"].(string)
	if annID == "" {
		t.Fatalf("annotation id is empty, annotationObj = %#v", annotationObj)
	}

	listAnn := callTool(t, session, "project_list_annotations", ProjectAnnotationsInput{Project: "annproj"})
	if listAnn.IsError {
		t.Fatalf("project_list_annotations error: %v", parseError(t, listAnn))
	}
	annListData := envelopeData(t, parseEnvelope(t, listAnn))
	annCount, _ := annListData["count"].(float64)
	if int(annCount) != 1 {
		t.Fatalf("annotation count = %v, want 1", annCount)
	}

	timeline := callTool(t, session, "project_list_timeline", ProjectTimelineInput{Project: "annproj"})
	if timeline.IsError {
		t.Fatalf("project_list_timeline error: %v", parseError(t, timeline))
	}

	denotate := callTool(t, session, "project_denotate", ProjectDenotateInput{Project: "annproj", AnnotationID: annID})
	if denotate.IsError {
		t.Fatalf("project_denotate error: %v", parseError(t, denotate))
	}
}

func TestProjectConfigSetUnsetList(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "project_add", ProjectAddInput{Slug: "cfgproj", Name: "Cfg Project"})

	set := callTool(t, session, "project_config_set", ProjectConfigSetInput{Project: "cfgproj", Key: "agent.background", Value: "test"})
	if set.IsError {
		t.Fatalf("project_config_set error: %v", parseError(t, set))
	}

	list := callTool(t, session, "project_config_list", ProjectConfigListInput{Project: "cfgproj"})
	if list.IsError {
		t.Fatalf("project_config_list error: %v", parseError(t, list))
	}
	cfgData := envelopeData(t, parseEnvelope(t, list))
	cfgMap, ok := cfgData["config"].(map[string]any)
	if !ok {
		t.Fatalf("config type = %T, want map", cfgData["config"])
	}
	if cfgMap["agent.background"] != "test" {
		t.Fatalf("agent.background = %v, want test", cfgMap["agent.background"])
	}

	unset := callTool(t, session, "project_config_unset", ProjectConfigUnsetInput{Project: "cfgproj", Key: "agent.background"})
	if unset.IsError {
		t.Fatalf("project_config_unset error: %v", parseError(t, unset))
	}

	list2 := callTool(t, session, "project_config_list", ProjectConfigListInput{Project: "cfgproj"})
	cfgData2 := envelopeData(t, parseEnvelope(t, list2))
	cfgMap2, _ := cfgData2["config"].(map[string]any)
	if _, exists := cfgMap2["agent.background"]; exists {
		t.Fatal("agent.background should be unset")
	}
}

// ---------------------------------------------------------------------------
// user 补充工具集成测试
// ---------------------------------------------------------------------------

func TestUserAddUseListGetExternalIDs(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	add := callTool(t, session, "user_add", UserAddInput{Name: "testuser"})
	if add.IsError {
		t.Fatalf("user_add error: %v", parseError(t, add))
	}

	list := callTool(t, session, "user_list", UserListInput{})
	if list.IsError {
		t.Fatalf("user_list error: %v", parseError(t, list))
	}
	usersData := envelopeData(t, parseEnvelope(t, list))
	userCount, _ := usersData["count"].(float64)
	if int(userCount) < 2 {
		t.Fatalf("user count = %v, want at least 2", userCount)
	}

	get := callTool(t, session, "user_get", UserInfoInput{User: "testuser"})
	if get.IsError {
		t.Fatalf("user_get error: %v", parseError(t, get))
	}

	use := callTool(t, session, "user_use", UserUseInput{User: "testuser"})
	if use.IsError {
		t.Fatalf("user_use error: %v", parseError(t, use))
	}

	extList := callTool(t, session, "user_list_external_ids", UserRefInput{User: "testuser"})
	if extList.IsError {
		t.Fatalf("user_list_external_ids error: %v", parseError(t, extList))
	}
}

func TestUserBindUnbind(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "user_add", UserAddInput{Name: "bindtest"})

	bind := callTool(t, session, "user_bind", UserBindInput{User: "bindtest", Provider: "feishu", ExternalID: "ou_12345"})
	if bind.IsError {
		t.Fatalf("user_bind error: %v", parseError(t, bind))
	}

	extList := callTool(t, session, "user_list_external_ids", UserRefInput{User: "bindtest"})
	extData := envelopeData(t, parseEnvelope(t, extList))
	extCount, _ := extData["count"].(float64)
	if int(extCount) != 1 {
		t.Fatalf("external ID count = %v, want 1", extCount)
	}

	unbind := callTool(t, session, "user_unbind", UserUnbindInput{User: "bindtest", Provider: "feishu", ExternalID: "ou_12345"})
	if unbind.IsError {
		t.Fatalf("user_unbind error: %v", parseError(t, unbind))
	}
}

// ---------------------------------------------------------------------------
// member_role 集成测试
// ---------------------------------------------------------------------------

func TestMemberRole(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "user_add", UserAddInput{Name: "roleuser"})
	callTool(t, session, "member_add", MemberAddInput{User: "roleuser", Role: "member"})

	result := callTool(t, session, "member_role", MemberRoleInput{User: "roleuser", Role: "admin"})
	if result.IsError {
		t.Fatalf("member_role error: %v", parseError(t, result))
	}
}

// ---------------------------------------------------------------------------
// context_none / context_list / context_delete 集成测试
// ---------------------------------------------------------------------------

func TestContextNoneListDelete(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if err := svc.DefineContext("sprint", "priority:H"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DefineContext("release", "priority:M"); err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	list := callTool(t, session, "context_list", ContextShowInput{})
	if list.IsError {
		t.Fatalf("context_list error: %v", parseError(t, list))
	}
	ctxData := envelopeData(t, parseEnvelope(t, list))
	ctxCount, _ := ctxData["count"].(float64)
	if int(ctxCount) != 2 {
		t.Fatalf("context count = %v, want 2", ctxCount)
	}

	none := callTool(t, session, "context_none", ContextShowInput{})
	if none.IsError {
		t.Fatalf("context_none error: %v", parseError(t, none))
	}

	del := callTool(t, session, "context_delete", ContextDeleteInput{Name: "release"})
	if del.IsError {
		t.Fatalf("context_delete error: %v", parseError(t, del))
	}
}

// ---------------------------------------------------------------------------
// config_unset / config_list 集成测试
// ---------------------------------------------------------------------------

func TestConfigUnsetList(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	_ = svc
	if err := newMCPTestService(t, store).SetConfig("urgency.priority.coeff", "5.0"); err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	list := callTool(t, session, "config_list", ConfigListInput{})
	if list.IsError {
		t.Fatalf("config_list error: %v", parseError(t, list))
	}

	unset := callTool(t, session, "config_unset", ConfigUnsetInput{Key: "urgency.priority.coeff"})
	if unset.IsError {
		t.Fatalf("config_unset error: %v", parseError(t, unset))
	}
}

// ---------------------------------------------------------------------------
// hook 全域集成测试
// ---------------------------------------------------------------------------

func TestHookFullLifecycle(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	list := callTool(t, session, "hook_list", HookListInput{})
	if list.IsError {
		t.Fatalf("hook_list error: %v", parseError(t, list))
	}

	add := callTool(t, session, "hook_add", HookAddInput{
		Name:   "test-hook",
		Sink:   "hook-sink",
		Events: []string{"task.created", "task.completed"},
	})
	if add.IsError {
		t.Fatalf("hook_add error: %v", parseError(t, add))
	}
	hookData := envelopeData(t, parseEnvelope(t, add))
	hookObj, ok := hookData["hook"].(map[string]any)
	if !ok {
		t.Fatalf("hook type = %T, want map", hookData["hook"])
	}
	hookID, _ := hookObj["id"].(string)

	info := callTool(t, session, "hook_info", HookRefInput{Hook: hookID})
	if info.IsError {
		t.Fatalf("hook_info error: %v", parseError(t, info))
	}

	mod := callTool(t, session, "hook_modify", HookModifyInput{Hook: hookID, Name: ptrStr("renamed-hook")})
	if mod.IsError {
		t.Fatalf("hook_modify error: %v", parseError(t, mod))
	}

	test := callTool(t, session, "hook_test", HookRefInput{Hook: hookID})
	if test.IsError {
		t.Fatalf("hook_test error: %v", parseError(t, test))
	}

	ping := callTool(t, session, "hook_ping", HookRefInput{Hook: hookID})
	if ping.IsError {
		t.Fatalf("hook_ping error: %v", parseError(t, ping))
	}

	deliveries := callTool(t, session, "hook_delivery_list", HookDeliveryListInput{Hook: hookID})
	if deliveries.IsError {
		t.Fatalf("hook_delivery_list error: %v", parseError(t, deliveries))
	}

	remove := callTool(t, session, "hook_remove", HookRefInput{Hook: hookID})
	if remove.IsError {
		t.Fatalf("hook_remove error: %v", parseError(t, remove))
	}

	list2 := callTool(t, session, "hook_list", HookListInput{})
	listData := envelopeData(t, parseEnvelope(t, list2))
	hookCount, _ := listData["count"].(float64)
	if int(hookCount) != 0 {
		t.Fatalf("hook count after remove = %v, want 0", hookCount)
	}
}

// ---------------------------------------------------------------------------
// notification/reminder 全域集成测试
// ---------------------------------------------------------------------------

func TestNotificationReminderFullLifecycle(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeStdio})
	session := connectClient(t, srv)

	addSink := callTool(t, session, "notification_sink_add", NotificationSinkAddInput{
		Name:           "openclaw",
		Type:           "webhook",
		EndpointMode:   "static_url",
		URL:            "https://example.com/xuanchu/notifications",
		Secret:         "secret-token",
		MaxConcurrency: 3,
	})
	if addSink.IsError {
		t.Fatalf("notification_sink_add error: %v", parseError(t, addSink))
	}
	sinkObj := nestedMap(t, envelopeData(t, parseEnvelope(t, addSink)), "sink")
	sinkID, _ := sinkObj["id"].(string)
	if sinkObj["max_concurrency"] != float64(3) {
		t.Fatalf("add sink max_concurrency = %v, want 3; sink=%#v", sinkObj["max_concurrency"], sinkObj)
	}

	maxConcurrency := 5
	modSink := callTool(t, session, "notification_sink_modify", NotificationSinkModifyInput{
		Sink:           sinkID,
		Name:           ptrStr("openclaw-renamed"),
		MaxConcurrency: &maxConcurrency,
	})
	if modSink.IsError {
		t.Fatalf("notification_sink_modify error: %v", parseError(t, modSink))
	}
	modSinkObj := nestedMap(t, envelopeData(t, parseEnvelope(t, modSink)), "sink")
	if modSinkObj["max_concurrency"] != float64(5) {
		t.Fatalf("modify sink max_concurrency = %v, want 5; sink=%#v", modSinkObj["max_concurrency"], modSinkObj)
	}
	infoSink := callTool(t, session, "notification_sink_info", NotificationSinkRefInput{Sink: sinkID})
	if infoSink.IsError {
		t.Fatalf("notification_sink_info error: %v", parseError(t, infoSink))
	}
	infoSinkObj := nestedMap(t, envelopeData(t, parseEnvelope(t, infoSink)), "sink")
	if infoSinkObj["max_concurrency"] != float64(5) {
		t.Fatalf("info sink max_concurrency = %v, want 5; sink=%#v", infoSinkObj["max_concurrency"], infoSinkObj)
	}
	listSinks := callTool(t, session, "notification_sink_list", NotificationSinkListInput{})
	if listSinks.IsError {
		t.Fatalf("notification_sink_list error: %v", parseError(t, listSinks))
	}
	listData := envelopeData(t, parseEnvelope(t, listSinks))
	sinks := nestedSlice(t, listData, "sinks")
	if len(sinks) == 0 {
		t.Fatalf("notification_sink_list returned no sinks")
	}
	var listSinkObj map[string]any
	for _, sink := range sinks {
		obj, ok := sink.(map[string]any)
		if !ok {
			t.Fatalf("listed sink type = %T, want map", sink)
		}
		if obj["id"] == sinkID {
			listSinkObj = obj
			break
		}
	}
	if listSinkObj == nil {
		t.Fatalf("notification_sink_list did not include sink %s: %#v", sinkID, sinks)
	}
	if listSinkObj["max_concurrency"] != float64(5) {
		t.Fatalf("list sink max_concurrency = %v, want 5; sink=%#v", listSinkObj["max_concurrency"], listSinkObj)
	}

	disableSink := callTool(t, session, "notification_sink_disable", NotificationSinkRefInput{Sink: sinkID})
	if disableSink.IsError {
		t.Fatalf("notification_sink_disable error: %v", parseError(t, disableSink))
	}
	enableSink := callTool(t, session, "notification_sink_enable", NotificationSinkRefInput{Sink: sinkID})
	if enableSink.IsError {
		t.Fatalf("notification_sink_enable error: %v", parseError(t, enableSink))
	}

	addRule := callTool(t, session, "reminder_rule_add", ReminderRuleAddInput{
		Name:          "due-soon-24h",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "end.isnull and start.isnull and due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		Sink:          sinkID,
	})
	if addRule.IsError {
		t.Fatalf("reminder_rule_add error: %v", parseError(t, addRule))
	}
	ruleObj := nestedMap(t, envelopeData(t, parseEnvelope(t, addRule)), "rule")
	ruleID, _ := ruleObj["id"].(string)
	if ruleObj["schedule_type"] != "daily_at" || ruleObj["schedule_value"] != "08:50" || ruleObj["filter_source"] == "" {
		t.Fatalf("reminder rule schedule/filter = %#v", ruleObj)
	}

	scheduleValue := "09:00"
	filterSource := "end.isnull and due.before:now"
	modRule := callTool(t, session, "reminder_rule_modify", ReminderRuleModifyInput{
		Rule:          ruleID,
		ScheduleValue: &scheduleValue,
		FilterSource:  &filterSource,
	})
	if modRule.IsError {
		t.Fatalf("reminder_rule_modify error: %v", parseError(t, modRule))
	}

	disableRule := callTool(t, session, "reminder_rule_disable", ReminderRuleRefInput{Rule: ruleID})
	if disableRule.IsError {
		t.Fatalf("reminder_rule_disable error: %v", parseError(t, disableRule))
	}
	enableRule := callTool(t, session, "reminder_rule_enable", ReminderRuleRefInput{Rule: ruleID})
	if enableRule.IsError {
		t.Fatalf("reminder_rule_enable error: %v", parseError(t, enableRule))
	}

	addNotificationRule := callTool(t, session, "notification_rule_add", NotificationRuleAddInput{
		Name:            "task-unblocked-openclaw",
		Event:           "task.unblocked",
		Filter:          "end.isnull",
		Audience:        "assignees",
		Sink:            sinkID,
		TemplateSubject: "任务已解除阻塞",
		TemplateBody:    "{{task.title}}",
	})
	if addNotificationRule.IsError {
		t.Fatalf("notification_rule_add error: %v", parseError(t, addNotificationRule))
	}
	notificationRuleObj := nestedMap(t, envelopeData(t, parseEnvelope(t, addNotificationRule)), "rule")
	notificationRuleID, _ := notificationRuleObj["id"].(string)
	if notificationRuleObj["event_type"] != "task.unblocked" || notificationRuleObj["audience_type"] != "assignees" || notificationRuleObj["filter_source"] != "end.isnull" {
		t.Fatalf("notification rule = %#v", notificationRuleObj)
	}

	notificationRules := callTool(t, session, "notification_rule_list", NotificationRuleListInput{})
	if notificationRules.IsError {
		t.Fatalf("notification_rule_list error: %v", parseError(t, notificationRules))
	}
	notificationRuleInfo := callTool(t, session, "notification_rule_info", NotificationRuleRefInput{Rule: notificationRuleID})
	if notificationRuleInfo.IsError {
		t.Fatalf("notification_rule_info error: %v", parseError(t, notificationRuleInfo))
	}

	notificationAudience := "actor"
	modNotificationRule := callTool(t, session, "notification_rule_modify", NotificationRuleModifyInput{
		Rule:     notificationRuleID,
		Audience: &notificationAudience,
	})
	if modNotificationRule.IsError {
		t.Fatalf("notification_rule_modify error: %v", parseError(t, modNotificationRule))
	}
	disableNotificationRule := callTool(t, session, "notification_rule_disable", NotificationRuleRefInput{Rule: notificationRuleID})
	if disableNotificationRule.IsError {
		t.Fatalf("notification_rule_disable error: %v", parseError(t, disableNotificationRule))
	}
	enableNotificationRule := callTool(t, session, "notification_rule_enable", NotificationRuleRefInput{Rule: notificationRuleID})
	if enableNotificationRule.IsError {
		t.Fatalf("notification_rule_enable error: %v", parseError(t, enableNotificationRule))
	}

	delivery := storage.NotificationDelivery{
		ID:                  "delivery-1",
		WorkspaceID:         svc.Runtime().WorkspaceID,
		RuleID:              ruleID,
		SinkID:              sinkID,
		TaskUUID:            "task-1",
		RecipientUserID:     svc.Runtime().ActorUserID,
		EventID:             "event-1",
		EventType:           "task.overdue",
		ActorType:           "user",
		ActorUserID:         ptrStr(svc.Runtime().ActorUserID),
		DedupeKey:           "delivery-1",
		ResolvedURL:         "https://example.com/xuanchu/notifications",
		RenderedMethod:      "POST",
		RenderedHeadersJSON: `{"Authorization":["Bearer secret-token"]}`,
		RenderedBody:        `{"ok":true}`,
		RenderedContentType: "application/json",
		PayloadJSON:         `{"event_type":"task.overdue"}`,
		Status:              storage.DeliveryStatusDeadLettered,
		CreatedAt:           100,
		ModifiedAt:          100,
	}
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	deliveryList := callTool(t, session, "notification_delivery_list", NotificationDeliveryListInput{Status: storage.DeliveryStatusDeadLettered})
	if deliveryList.IsError {
		t.Fatalf("notification_delivery_list error: %v", parseError(t, deliveryList))
	}
	deliveryRows := nestedSlice(t, envelopeData(t, parseEnvelope(t, deliveryList)), "deliveries")
	if len(deliveryRows) != 1 {
		t.Fatalf("delivery rows = %d, want 1", len(deliveryRows))
	}
	deliveryObj, ok := deliveryRows[0].(map[string]any)
	if !ok {
		t.Fatalf("delivery row type = %T, want map", deliveryRows[0])
	}
	actor := nestedMap(t, deliveryObj, "actor")
	if actor["type"] != "user" {
		t.Fatalf("delivery actor = %#v, want user actor", actor)
	}
	deliveryInfo := callTool(t, session, "notification_delivery_info", NotificationDeliveryRefInput{DeliveryID: delivery.ID})
	if deliveryInfo.IsError {
		t.Fatalf("notification_delivery_info error: %v", parseError(t, deliveryInfo))
	}
	infoObj := nestedMap(t, envelopeData(t, parseEnvelope(t, deliveryInfo)), "delivery")
	infoActor := nestedMap(t, infoObj, "actor")
	if infoActor["type"] != "user" {
		t.Fatalf("delivery info actor = %#v, want user actor", infoActor)
	}
	replay := callTool(t, session, "notification_delivery_replay", NotificationDeliveryRefInput{DeliveryID: delivery.ID})
	if replay.IsError {
		t.Fatalf("notification_delivery_replay error: %v", parseError(t, replay))
	}

	removeRule := callTool(t, session, "reminder_rule_remove", ReminderRuleRefInput{Rule: ruleID})
	if removeRule.IsError {
		t.Fatalf("reminder_rule_remove error: %v", parseError(t, removeRule))
	}
	removeNotificationRule := callTool(t, session, "notification_rule_remove", NotificationRuleRefInput{Rule: notificationRuleID})
	if removeNotificationRule.IsError {
		t.Fatalf("notification_rule_remove error: %v", parseError(t, removeNotificationRule))
	}
	removeSink := callTool(t, session, "notification_sink_remove", NotificationSinkRefInput{Sink: sinkID})
	if removeSink.IsError {
		t.Fatalf("notification_sink_remove error: %v", parseError(t, removeSink))
	}
}

// ---------------------------------------------------------------------------
// token 全域集成测试
// ---------------------------------------------------------------------------

func TestTokenFullLifecycle(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	list := callTool(t, session, "token_list", TokenListInput{})
	if list.IsError {
		t.Fatalf("token_list error: %v", parseError(t, list))
	}

	create := callTool(t, session, "token_create", TokenCreateInput{
		Name:  "test-token",
		Scope: []string{"task:read", "task:write"},
	})
	if create.IsError {
		t.Fatalf("token_create error: %v", parseError(t, create))
	}
	tokenData := envelopeData(t, parseEnvelope(t, create))
	tokenObj, ok := tokenData["token"].(map[string]any)
	if !ok {
		t.Fatalf("token type = %T, want map", tokenData["token"])
	}
	tokenID, _ := tokenObj["id"].(string)
	if _, hasRaw := tokenObj["raw_token"]; !hasRaw {
		t.Fatal("token_create should return raw_token")
	}

	mod := callTool(t, session, "token_modify", TokenModifyInput{
		TokenRef: tokenID,
		Name:     ptrStr("renamed-token"),
	})
	if mod.IsError {
		t.Fatalf("token_modify error: %v", parseError(t, mod))
	}

	revoke := callTool(t, session, "token_revoke", TokenRevokeInput{TokenRef: tokenID})
	if revoke.IsError {
		t.Fatalf("token_revoke error: %v", parseError(t, revoke))
	}
}

func TestMCPTokenModifyUpdatesProjectBinding(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	projectAdd := callTool(t, session, "project_add", ProjectAddInput{Slug: "demo", Name: "Demo"})
	if projectAdd.IsError {
		t.Fatalf("project_add error: %v", parseError(t, projectAdd))
	}
	projectID, _ := nestedMap(t, envelopeData(t, parseEnvelope(t, projectAdd)), "project")["id"].(string)
	if projectID == "" {
		t.Fatalf("project id is empty")
	}

	create := callTool(t, session, "token_create", TokenCreateInput{
		Name:  "project-bound-token",
		Scope: []string{"task:read"},
	})
	if create.IsError {
		t.Fatalf("token_create error: %v", parseError(t, create))
	}
	tokenID, _ := nestedMap(t, envelopeData(t, parseEnvelope(t, create)), "token")["id"].(string)
	if tokenID == "" {
		t.Fatalf("created token id is empty")
	}

	mod := callTool(t, session, "token_modify", TokenModifyInput{
		TokenRef: tokenID,
		Projects: []string{"demo"},
	})
	if mod.IsError {
		t.Fatalf("token_modify error: %v", parseError(t, mod))
	}
	tokenObj := nestedMap(t, envelopeData(t, parseEnvelope(t, mod)), "token")
	projectIDs, ok := tokenObj["project_ids"].([]any)
	if !ok {
		t.Fatalf("project_ids type = %T, want []any", tokenObj["project_ids"])
	}
	if len(projectIDs) != 1 || projectIDs[0] != projectID {
		t.Fatalf("project_ids = %v, want [%s]", projectIDs, projectID)
	}
}

func TestMCPHTTPTokenModifyProjectBindingUsesPatchRefs(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	projectOne, err := svc.AddProject(app.AddProjectInput{Slug: "one", Name: "One"})
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := svc.AddProject(app.AddProjectInput{Slug: "two", Name: "Two"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "project-manager",
		Type:          "agent",
		Scopes:        []string{"token:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{"one", "two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "two-token",
		Type:          "agent",
		Scopes:        []string{"token:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{"two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+parent.RawToken)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	mod := callTool(t, session, "token_modify", TokenModifyInput{
		TokenRef: target.View.ID,
		Projects: []string{"one"},
	})
	if mod.IsError {
		t.Fatalf("token_modify error: %v", parseError(t, mod))
	}
	tokenObj := nestedMap(t, envelopeData(t, parseEnvelope(t, mod)), "token")
	projectIDs := nestedSlice(t, tokenObj, "project_ids")
	if len(projectIDs) != 1 || projectIDs[0] != projectOne.ID {
		t.Fatalf("project_ids = %v, want [%s]", projectIDs, projectOne.ID)
	}
	if projectIDs[0] == projectTwo.ID {
		t.Fatalf("project_ids still contains old project %s", projectTwo.ID)
	}
}

// ---------------------------------------------------------------------------
// audit_list / scope_list / me_get 集成测试
// ---------------------------------------------------------------------------

func TestAuditList(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task_add", TaskAddInput{Title: "audit test task"})

	result := callTool(t, session, "audit_list", AuditListInput{})
	if result.IsError {
		t.Fatalf("audit_list error: %v", parseError(t, result))
	}
	auditData := envelopeData(t, parseEnvelope(t, result))
	entries, ok := auditData["entries"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("audit entries = %#v, want at least 1", auditData["entries"])
	}
}

func TestScopeList(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "scope_list", ScopeListInput{})
	if result.IsError {
		t.Fatalf("scope_list error: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	scopes, ok := data["scopes"].([]any)
	if !ok || len(scopes) == 0 {
		t.Fatalf("scopes = %#v, want non-empty", data["scopes"])
	}
}

func TestMeGet(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "me_get", MeGetInput{})
	if result.IsError {
		t.Fatalf("me_get error: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	userObj, ok := data["user"].(map[string]any)
	if !ok {
		t.Fatalf("user type = %T, want map", data["user"])
	}
	if userObj["name"] == "" {
		t.Fatal("user name is empty")
	}
}
