package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/storage/sqlite"
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

// extractTask 从 envelope data 中提取 task 对象。
// taskData 返回 {"task": {...}}，此函数提取内层 map。
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
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	clock := fixedTestClock()
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

func nestedSlice(t *testing.T, parent map[string]any, key string) []any {
	t.Helper()
	value, ok := parent[key].([]any)
	if !ok {
		t.Fatalf("%s type = %T, want []any", key, parent[key])
	}
	return value
}

func newTestServerWithOptions(t *testing.T, opts Options) (*mcp.Server, *sqlite.Store) {
	t.Helper()
	store := opts.Store
	if store == nil {
		var err error
		store, err = sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
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
		"task.add", "task.query", "task.get",
		"task.modify", "task.done", "task.delete",
		"task.annotate", "task.depends", "task.start", "task.stop",
		"report.run", "urgency.explain",
		"workspace.list", "workspace.current",
		"project.list", "project.get", "project.current",
		"member_list", "member_add",
		"user_list", "user_info",
		"context.show", "context.set",
		"config.get", "config.set",
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

// ---------------------------------------------------------------------------
// task.add 集成测试
// ---------------------------------------------------------------------------

func TestTaskAddBasic(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task.add", TaskAddInput{
		Description: "buy milk",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	// taskData wraps task in {"task": {...}}
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", env.Data)
	}
	taskObj, ok := dataMap["task"].(map[string]any)
	if !ok {
		t.Fatalf("task type = %T, want map", dataMap["task"])
	}
	if taskObj["description"] != "buy milk" {
		t.Fatalf("description = %v, want buy milk", taskObj["description"])
	}
	if taskObj["uuid"] == nil || taskObj["uuid"] == "" {
		t.Fatal("uuid is empty")
	}
	if env.Rendered == "" {
		t.Fatal("rendered is empty")
	}
}

func TestTaskAddAndGetAssignees(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task.add", TaskAddInput{
		Description: "assigned task",
		Assignees:   []string{"local"},
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
	getResult := callTool(t, session, "task.get", TaskGetInput{ID: uuid})
	if getResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, getResult))
	}
	taskObj = extractTask(t, parseEnvelope(t, getResult))
	assignees, ok = taskObj["assignees"].([]any)
	if !ok || len(assignees) != 1 {
		t.Fatalf("assignees after get = %#v, want one assignee", taskObj["assignees"])
	}
}

func TestTaskAddMissingDescription(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task.add", TaskAddInput{})
	if !result.IsError {
		t.Fatal("expected IsError=true for missing description")
	}
}

// ---------------------------------------------------------------------------
// task.get 集成测试
// ---------------------------------------------------------------------------

func TestTaskGetByID(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	// 先创建
	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "test task"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	// 再查询
	getResult := callTool(t, session, "task.get", TaskGetInput{ID: uuid})
	if getResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, getResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, getResult))
	if taskObj["description"] != "test task" {
		t.Fatalf("description = %v, want test task", taskObj["description"])
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
	taskA, err := svc.Add(app.AddInput{Description: "alpha task", Project: &alpha.Slug})
	if err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, svc, []string{"task:read"}, []string{"local"}, nil)
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	result := callTool(t, session, "task.get", TaskGetInput{ID: taskA.UUID, ProjectID: beta.ID})
	if !result.IsError {
		t.Fatal("task.get with mismatched explicit project_id should fail")
	}
	if code := parseError(t, result).Code; code != "task_not_found" {
		t.Fatalf("task.get code = %q, want task_not_found", code)
	}
}

func TestAddToolConvertsReturnedErrorToStructuredToolError(t *testing.T) {
	srv, _ := newTestServer(t)
	addTool(srv, &mcp.Tool{Name: "test.error"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, ToolEnvelope, error) {
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

func TestTaskGetMissingID(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "task.get", TaskGetInput{ID: ""})
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

	callTool(t, session, "task.add", TaskAddInput{Description: "task 1"})
	callTool(t, session, "task.add", TaskAddInput{Description: "task 2"})

	result := callTool(t, session, "task.query", TaskQueryInput{})
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

func TestTaskQueryCanIncludeCompletedAndDeleted(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	doneUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task.add", TaskAddInput{Description: "done item"})))
	deleteUUID := extractUUID(t, parseEnvelope(t, callTool(t, session, "task.add", TaskAddInput{Description: "deleted item"})))
	if result := callTool(t, session, "task.done", TaskIDInput{ID: doneUUID}); result.IsError {
		t.Fatalf("task.done error: %v", parseError(t, result))
	}
	if result := callTool(t, session, "task.delete", TaskIDInput{ID: deleteUUID}); result.IsError {
		t.Fatalf("task.delete error: %v", parseError(t, result))
	}

	result := callTool(t, session, "task.query", TaskQueryInput{IncludeCompleted: true, IncludeDeleted: true})
	if result.IsError {
		t.Fatalf("task.query error: %v", parseError(t, result))
	}
	data := envelopeData(t, parseEnvelope(t, result))
	if count := data["count"]; count != float64(2) {
		t.Fatalf("count = %v, want 2", count)
	}
}

// ---------------------------------------------------------------------------
// task.done 集成测试
// ---------------------------------------------------------------------------

func TestTaskDone(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "finish report"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	doneResult := callTool(t, session, "task.done", TaskIDInput{ID: uuid})
	if doneResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, doneResult))
	}
	taskObj := extractTask(t, parseEnvelope(t, doneResult))
	if taskObj["status"] != "completed" {
		t.Fatalf("status = %v, want completed", taskObj["status"])
	}
}

// ---------------------------------------------------------------------------
// task.delete 集成测试
// ---------------------------------------------------------------------------

func TestTaskDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "delete me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	delResult := callTool(t, session, "task.delete", TaskIDInput{ID: uuid})
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

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "original"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	modResult := callTool(t, session, "task.modify", TaskModifyInput{
		ID:          uuid,
		Description: ptrStr("updated"),
	})
	if modResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, modResult))
	}

	// 验证更新
	getResult := callTool(t, session, "task.get", TaskGetInput{ID: uuid})
	taskObj := extractTask(t, parseEnvelope(t, getResult))
	if taskObj["description"] != "updated" {
		t.Fatalf("description = %v, want updated", taskObj["description"])
	}
}

func TestTaskModifyClearFields(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task.add", TaskAddInput{
		Description: "clear test",
		Priority:    "H",
	})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	modResult := callTool(t, session, "task.modify", TaskModifyInput{
		ID:    uuid,
		Clear: []string{"priority"},
	})
	if modResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, modResult))
	}

	getResult := callTool(t, session, "task.get", TaskGetInput{ID: uuid})
	taskObj := extractTask(t, parseEnvelope(t, getResult))
	if taskObj["priority"] != nil {
		t.Fatalf("priority = %v, want nil after clear", taskObj["priority"])
	}
}

func TestTaskModifyAssigneesAndClear(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "assign later"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	modResult := callTool(t, session, "task.modify", TaskModifyInput{
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

	clearResult := callTool(t, session, "task.modify", TaskModifyInput{
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
	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "unknown clear"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	result := callTool(t, session, "task.modify", TaskModifyInput{ID: uuid, Clear: []string{"priorty"}})
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

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "annotate me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	annResult := callTool(t, session, "task.annotate", TaskAnnotateInput{
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

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "annotate me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	result := callTool(t, session, "task.annotate", TaskAnnotateInput{ID: uuid})
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

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "start stop"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	startResult := callTool(t, session, "task.start", TaskIDInput{ID: uuid})
	if startResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, startResult))
	}

	stopResult := callTool(t, session, "task.stop", TaskIDInput{ID: uuid})
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

	uuid1 := extractUUID(t, parseEnvelope(t, callTool(t, session, "task.add", TaskAddInput{Description: "task 1"})))
	uuid2 := extractUUID(t, parseEnvelope(t, callTool(t, session, "task.add", TaskAddInput{Description: "task 2"})))

	depResult := callTool(t, session, "task.depends", TaskDependsInput{
		ID:      uuid2,
		Depends: []string{uuid1},
	})
	if depResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, depResult))
	}

	// 清除依赖
	clearResult := callTool(t, session, "task.depends", TaskDependsInput{
		ID:           uuid2,
		ClearDepends: true,
	})
	if clearResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, clearResult))
	}
}

// ---------------------------------------------------------------------------
// report.run 集成测试
// ---------------------------------------------------------------------------

func TestReportRun(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task.add", TaskAddInput{Description: "report task"})

	result := callTool(t, session, "report.run", ReportRunInput{Name: "list"})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", env.Data)
	}
	count, _ := dataMap["count"].(float64)
	if int(count) < 1 {
		t.Fatalf("expected at least 1 task, got %d", int(count))
	}
}

func TestReportRunLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	callTool(t, session, "task.add", TaskAddInput{Description: "report task 1"})
	callTool(t, session, "task.add", TaskAddInput{Description: "report task 2"})

	result := callTool(t, session, "report.run", ReportRunInput{Name: "list", Limit: 1})
	if result.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, result))
	}
	tasks := nestedSlice(t, envelopeData(t, parseEnvelope(t, result)), "tasks")
	if len(tasks) != 1 {
		t.Fatalf("tasks len = %d, want 1", len(tasks))
	}
}

func TestReportRunMissingName(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "report.run", ReportRunInput{})
	if !result.IsError {
		t.Fatal("expected IsError=true for missing name")
	}
}

func TestReportRunBadLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "report.run", ReportRunInput{Name: "list", Limit: 9999})
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

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "urgency task"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	explainResult := callTool(t, session, "urgency.explain", UrgencyExplainInput{ID: uuid})
	if explainResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, explainResult))
	}
	explainEnv := parseEnvelope(t, explainResult)
	explainData, ok := explainEnv.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map", explainEnv.Data)
	}
	if explainData["uuid"] != uuid {
		t.Fatalf("uuid = %v, want %s", explainData["uuid"], uuid)
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
	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "audit check"})
	if addResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, addResult))
	}
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	// done 应产生 audit
	doneResult := callTool(t, session, "task.done", TaskIDInput{ID: uuid})
	if doneResult.IsError {
		t.Fatalf("unexpected error: %v", parseError(t, doneResult))
	}

	// 创建新任务并删除
	uuid2 := extractUUID(t, parseEnvelope(t, callTool(t, session, "task.add", TaskAddInput{Description: "delete audit"})))

	delResult := callTool(t, session, "task.delete", TaskIDInput{ID: uuid2})
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

	list := callTool(t, session, "workspace.list", WorkspaceListInput{})
	if list.IsError {
		t.Fatalf("workspace.list error: %v", parseError(t, list))
	}
	workspaces := nestedSlice(t, envelopeData(t, parseEnvelope(t, list)), "workspaces")
	if len(workspaces) < 2 {
		t.Fatalf("workspace.list returned %d workspace(s), want at least 2", len(workspaces))
	}

	current := callTool(t, session, "workspace.current", WorkspaceCurrentInput{Workspace: "team"})
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

	list := callTool(t, session, "project.list", ProjectListInput{})
	if list.IsError {
		t.Fatalf("project.list error: %v", parseError(t, list))
	}
	projects := nestedSlice(t, envelopeData(t, parseEnvelope(t, list)), "projects")
	if len(projects) != 1 {
		t.Fatalf("project.list count = %d, want 1", len(projects))
	}

	got := callTool(t, session, "project.get", ProjectGetInput{ProjectID: project.ID})
	if got.IsError {
		t.Fatalf("project.get error: %v", parseError(t, got))
	}
	data := envelopeData(t, parseEnvelope(t, got))
	projectData := nestedMap(t, data, "project")
	if projectData["slug"] != "agent" {
		t.Fatalf("project slug = %v, want agent", projectData["slug"])
	}
	configSummary := nestedMap(t, data, "config_summary")
	if configSummary["agent.background"] != "Background" {
		t.Fatalf("agent.background = %v, want Background", configSummary["agent.background"])
	}
	if _, ok := configSummary["context.default"]; ok {
		t.Fatal("context.default must not be exposed in MCP project config summary")
	}

	missing := callTool(t, session, "project.get", ProjectGetInput{})
	if !missing.IsError {
		t.Fatal("project.get without project or project_id should fail")
	}

	current := callTool(t, session, "project.current", ProjectCurrentInput{})
	if current.IsError {
		t.Fatalf("project.current error: %v", parseError(t, current))
	}
	if envelopeData(t, parseEnvelope(t, current))["project"] != nil {
		t.Fatal("project.current without explicit scope should return null project")
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

	set := callTool(t, session, "context.set", ContextSetInput{Name: "sprint"})
	if set.IsError {
		t.Fatalf("context.set error: %v", parseError(t, set))
	}
	show := callTool(t, session, "context.show", ContextShowInput{})
	if show.IsError {
		t.Fatalf("context.show error: %v", parseError(t, show))
	}
	contextData := nestedMap(t, envelopeData(t, parseEnvelope(t, show)), "context")
	if contextData["name"] != "sprint" || contextData["filter"] != "priority:H" || contextData["active"] != true {
		t.Fatalf("context = %#v, want active sprint priority:H", contextData)
	}

	clear := callTool(t, session, "context.set", ContextSetInput{Name: "none"})
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

	local := callTool(t, session, "config.get", ConfigGetInput{Scope: "local", Key: "remote.server"})
	if local.IsError {
		t.Fatalf("config.get local error: %v", parseError(t, local))
	}
	if got := envelopeData(t, parseEnvelope(t, local))["value"]; got != "http://127.0.0.1:8080" {
		t.Fatalf("local remote.server = %v", got)
	}

	workspacePersonal := callTool(t, session, "config.set", ConfigSetInput{Scope: "workspace", Key: "color", Value: "auto"})
	if !workspacePersonal.IsError {
		t.Fatal("config.set workspace color should fail")
	}
	if code := parseError(t, workspacePersonal).Code; code != "config_key_unsupported" {
		t.Fatalf("code = %q, want config_key_unsupported", code)
	}
	workspaceAgent := callTool(t, session, "config.set", ConfigSetInput{Scope: "workspace", Key: "agent.background", Value: "x"})
	if !workspaceAgent.IsError {
		t.Fatal("config.set workspace agent.background should fail")
	}
	if code := parseError(t, workspaceAgent).Code; code != "config_key_unsupported" {
		t.Fatalf("code = %q, want config_key_unsupported", code)
	}

	projectSet := callTool(t, session, "config.set", ConfigSetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff", Value: "handoff"})
	if projectSet.IsError {
		t.Fatalf("config.set project error: %v", parseError(t, projectSet))
	}
	projectGet := callTool(t, session, "config.get", ConfigGetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff"})
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

	result := callTool(t, session, "config.get", ConfigGetInput{Scope: "local", Key: "date.format"})
	if !result.IsError {
		t.Fatal("config.get local in HTTP mode should fail")
	}
	if code := parseError(t, result).Code; code != "config_scope_invalid" {
		t.Fatalf("code = %q, want config_scope_invalid", code)
	}
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
	projectRead := callTool(t, projectSession, "config.get", ConfigGetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff"})
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
	configRead := callTool(t, configSession, "config.get", ConfigGetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff"})
	if configRead.IsError {
		t.Fatalf("config.get with config:read error: %v", parseError(t, configRead))
	}
	configWrite := callTool(t, configSession, "config.set", ConfigSetInput{Scope: "project", ProjectID: project.ID, Key: "agent.handoff", Value: "updated"})
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

	projectGet := callTool(t, session, "project.get", ProjectGetInput{ProjectID: project.ID})
	if projectGet.IsError {
		t.Fatalf("project.get error: %v", parseError(t, projectGet))
	}
	configSummary := nestedMap(t, envelopeData(t, parseEnvelope(t, projectGet)), "config_summary")
	if configSummary["agent.background"] != "remote docs" {
		t.Fatalf("agent.background = %v, want remote docs", configSummary["agent.background"])
	}

	queryEmpty := callTool(t, session, "task.query", TaskQueryInput{ProjectID: project.ID})
	if queryEmpty.IsError {
		t.Fatalf("task.query empty error: %v", parseError(t, queryEmpty))
	}
	if count := envelopeData(t, parseEnvelope(t, queryEmpty))["count"]; count != float64(0) {
		t.Fatalf("empty project task count = %v, want 0", count)
	}

	add := callTool(t, session, "task.add", TaskAddInput{Description: "ship MCP", ProjectID: project.ID})
	if add.IsError {
		t.Fatalf("task.add error: %v", parseError(t, add))
	}
	uuid := extractUUID(t, parseEnvelope(t, add))
	explain := callTool(t, session, "urgency.explain", UrgencyExplainInput{ID: uuid})
	if explain.IsError {
		t.Fatalf("urgency.explain error: %v", parseError(t, explain))
	}
	done := callTool(t, session, "task.done", TaskIDInput{ID: uuid})
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
	projectA, err := owner.AddProject(app.AddProjectInput{Slug: "a", Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := owner.AddProject(app.AddProjectInput{Slug: "b", Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	taskB, err := owner.Add(app.AddInput{Description: "hidden", Project: &projectB.Slug})
	if err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, owner, []string{"task:read", "project:read"}, []string{"local"}, []string{projectA.ID})
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv, _ := newTestServerWithOptions(t, Options{Store: store, Mode: ModeHTTP, Request: req})
	session := connectClient(t, srv)

	query := callTool(t, session, "task.query", TaskQueryInput{ProjectID: projectB.ID})
	if !query.IsError {
		t.Fatal("task.query outside project allowlist should fail")
	}
	if code := parseError(t, query).Code; code != "project_scope_denied" {
		t.Fatalf("task.query code = %q, want project_scope_denied", code)
	}

	get := callTool(t, session, "task.get", TaskGetInput{ID: taskB.UUID})
	if !get.IsError {
		t.Fatal("task.get outside project allowlist should fail")
	}
	if code := parseError(t, get).Code; code != "task_not_found" {
		t.Fatalf("task.get code = %q, want task_not_found", code)
	}

	list := callTool(t, session, "project.list", ProjectListInput{})
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
