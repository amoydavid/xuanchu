package integration

import (
	"context"
	"os/exec"
	"path/filepath"
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
