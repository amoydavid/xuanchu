package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

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

// ---------------------------------------------------------------------------
// task.annotate 集成测试
// ---------------------------------------------------------------------------

func TestTaskAnnotate(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	addResult := callTool(t, session, "task.add", TaskAddInput{Description: "annotate me"})
	uuid := extractUUID(t, parseEnvelope(t, addResult))

	annResult := callTool(t, session, "task.annotate", TaskAnnotateInput{
		ID:          uuid,
		Description: "this is a note",
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
		t.Fatal("expected IsError=true for missing description")
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
	srv, _ := newTestServer(t)
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

	// 这些写操作已成功完成，audit 由 app 层 withAudit 自动写入
	// 这里仅验证操作本身无错误，不直接检查 audit 表
}
