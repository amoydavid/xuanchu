package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPStdioAndHTTPReturnSameResourceURL 验证 stdio MCP 和 HTTP MCP 对同一 project
// 返回相同的绝对 URL（absolute-resource-url 计划 Task 4）。
func TestMCPStdioAndHTTPReturnSameResourceURL(t *testing.T) {
	t.Setenv("XUANCHU_PUBLIC_BASE_URL", integrationResourceBaseURL)
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "list")
	run(t, bin, "--db", db, "project", "add", "API", "name:API")
	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "e2e-mcp", "--scope", "project:read")
	tokenStr := parseRawToken(t, tokenOut)

	wantURL := integrationResourceBaseURL + "/workspaces/local/projects/api"

	// HTTP MCP
	srvPort := getFreeIntegrationPort(t)
	srvProc := exec.Command(bin, "--db", db, "server", "--listen", fmt.Sprintf("127.0.0.1:%d", srvPort))
	if err := srvProc.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() { _ = srvProc.Process.Kill() }()
	waitForHTTPServerSimple(t, fmt.Sprintf("http://127.0.0.1:%d/healthz", srvPort))

	baseAPIURL := fmt.Sprintf("http://127.0.0.1:%d", srvPort)
	httpSession, httpCancel := connectHTTPMCP(t, baseAPIURL, tokenStr)
	defer httpCancel()
	httpURL := mcpExtractURL(t, httpSession, "project_get", map[string]any{
		"workspace": "local",
		"project":   "api",
	})
	if httpURL != wantURL {
		t.Fatalf("HTTP MCP URL = %q, want %q", httpURL, wantURL)
	}

	// stdio MCP
	stdioCmd := exec.Command(bin, "--db", db, "mcp", "stdio")
	stdioSession, stdioCancel := connectStdioMCP(t, stdioCmd)
	defer stdioCancel()
	stdioURL := mcpExtractURL(t, stdioSession, "project_get", map[string]any{
		"workspace": "local",
		"project":   "api",
	})
	if stdioURL != wantURL {
		t.Fatalf("stdio MCP URL = %q, want %q", stdioURL, wantURL)
	}
	if httpURL != stdioURL {
		t.Fatalf("URL mismatch: HTTP MCP=%q stdio MCP=%q", httpURL, stdioURL)
	}
}

// TestMCPStdioAndHTTPReturnSameTaskURL 验证 stdio MCP 和 HTTP MCP 对同一 task
// 返回相同的绝对 URL。
func TestMCPStdioAndHTTPReturnSameTaskURL(t *testing.T) {
	t.Setenv("XUANCHU_PUBLIC_BASE_URL", integrationResourceBaseURL)
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "list")
	run(t, bin, "--db", db, "project", "add", "API", "name:API")
	run(t, bin, "--db", db, "add", "MCP URL task", "project:api", "+next")
	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "e2e-task-mcp", "--scope", "task:read")
	tokenStr := parseRawToken(t, tokenOut)

	wantURL := integrationResourceBaseURL + "/workspaces/local/projects/api/tasks/api-1"

	srvPort := getFreeIntegrationPort(t)
	srvProc := exec.Command(bin, "--db", db, "server", "--listen", fmt.Sprintf("127.0.0.1:%d", srvPort))
	if err := srvProc.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() { _ = srvProc.Process.Kill() }()
	waitForHTTPServerSimple(t, fmt.Sprintf("http://127.0.0.1:%d/healthz", srvPort))

	baseAPIURL := fmt.Sprintf("http://127.0.0.1:%d", srvPort)
	httpSession, httpCancel := connectHTTPMCP(t, baseAPIURL, tokenStr)
	defer httpCancel()
	httpURL := mcpExtractURL(t, httpSession, "task_get", map[string]any{
		"workspace": "local",
		"id":        "api-1",
	})
	if httpURL != wantURL {
		t.Fatalf("HTTP MCP task URL = %q, want %q", httpURL, wantURL)
	}

	stdioCmd := exec.Command(bin, "--db", db, "mcp", "stdio")
	stdioSession, stdioCancel := connectStdioMCP(t, stdioCmd)
	defer stdioCancel()
	stdioURL := mcpExtractURL(t, stdioSession, "task_get", map[string]any{
		"workspace": "local",
		"id":        "api-1",
	})
	if stdioURL != wantURL {
		t.Fatalf("stdio MCP task URL = %q, want %q", stdioURL, wantURL)
	}
	if httpURL != stdioURL {
		t.Fatalf("task URL mismatch: HTTP MCP=%q stdio MCP=%q", httpURL, stdioURL)
	}
}

// mcpExtractURL 通过 MCP ClientSession 调用 tool 并从 structuredContent 提取 url。
func mcpExtractURL(t *testing.T, session *mcp.ClientSession, toolName string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("MCP CallTool %s: %v", toolName, err)
	}
	if result.StructuredContent != nil {
		if data, ok := result.StructuredContent.(map[string]any); ok {
			if dataObj, ok := data["data"].(map[string]any); ok {
				// task_get 返回 data.url；project_get 返回 data.project.url
				if url, ok := dataObj["url"].(string); ok {
					return url
				}
				if project, ok := dataObj["project"].(map[string]any); ok {
					if url, ok := project["url"].(string); ok {
						return url
					}
				}
				if task, ok := dataObj["task"].(map[string]any); ok {
					if url, ok := task["url"].(string); ok {
						return url
					}
				}
			}
		}
	}
	if len(result.Content) > 0 {
		if text, ok := result.Content[0].(*mcp.TextContent); ok {
			var embedded map[string]any
			if err := json.Unmarshal([]byte(text.Text), &embedded); err == nil {
				if data, ok := embedded["data"].(map[string]any); ok {
					if url, ok := data["url"].(string); ok {
						return url
					}
				}
			}
		}
	}
	t.Fatalf("could not extract URL from MCP result: %+v", result)
	return ""
}
