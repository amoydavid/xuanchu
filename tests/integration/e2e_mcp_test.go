package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2EProjectTemplateMCPUsesNarrowCurrentOnlySurface(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "--workspace", "local", "project", "add", "tplsource", "name:模板来源")
	run(t, bin, "--db", db, "workspace", "add", "other", "name:其他空间")
	token := parseRawToken(t, run(t, bin,
		"--db", db, "--json", "--workspace", "local", "token", "create", "project-template-mcp-e2e",
		"--expires-in", "720h", "--scope", "*", "--workspace-id", "other",
	))

	server, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, server)
	headers := authHeaders(token)
	capture := map[string]any{
		"source_project": "tplsource", "anchor_date": "2026-07-20",
		"selection": map[string]any{
			"config_keys": []string{}, "task_refs": []string{},
			"series_refs": []string{}, "automation_rule_ids": []string{},
		},
	}
	preview := httpJSON(t, "POST", baseURL+"/api/v1/project-templates/capture-preview?workspace=local", capture, headers)
	capture["expected_source_hash"] = preview["data"].(map[string]any)["source_hash"].(string)
	created := httpJSON(t, "POST", baseURL+"/api/v1/project-templates?workspace=local", map[string]any{
		"key": "launch", "name": "启动模板", "description": "MCP E2E", "capture": capture,
	}, headers)
	wantSnapshotID, wantSnapshotHash := projectTemplateE2ECurrent(t, created)

	session, cancel := connectHTTPMCP(t, baseURL, token)
	defer cancel()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	var templateTools []string
	for _, tool := range tools.Tools {
		if strings.HasPrefix(tool.Name, "project_template_") {
			templateTools = append(templateTools, tool.Name)
		}
	}
	sort.Strings(templateTools)
	if want := []string{"project_template_instantiate", "project_template_list"}; !reflect.DeepEqual(templateTools, want) {
		t.Fatalf("project template tools = %v, want %v", templateTools, want)
	}

	listed := callMCPProjectTemplateE2E(t, session, "project_template_list", map[string]any{"workspace": "local", "limit": 20, "offset": 0})
	listedEnvelope := assertMCPProjectTemplateE2EEnvelopeEquivalent(t, listed)
	listedData := listedEnvelope["data"].(map[string]any)
	items := listedData["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("template items = %#v", items)
	}
	current := items[0].(map[string]any)["current_snapshot"].(map[string]any)
	if current["id"] != wantSnapshotID || current["hash"] != wantSnapshotHash {
		t.Fatalf("current snapshot = %#v", current)
	}

	isolated := callMCPProjectTemplateE2E(t, session, "project_template_list", map[string]any{"workspace": "other"})
	isolatedData := assertMCPProjectTemplateE2EEnvelopeEquivalent(t, isolated)["data"].(map[string]any)
	if otherItems := isolatedData["items"].([]any); len(otherItems) != 0 {
		t.Fatalf("other workspace template items = %#v", otherItems)
	}

	instantiated := callMCPProjectTemplateE2E(t, session, "project_template_instantiate", map[string]any{
		"workspace": "local", "template": "launch", "snapshot_id": wantSnapshotID,
		"expected_snapshot_hash": wantSnapshotHash, "project_slug": "newproj",
		"project_name": "MCP 创建项目", "start_date": "2026-08-01",
	})
	instantiatedData := assertMCPProjectTemplateE2EEnvelopeEquivalent(t, instantiated)["data"].(map[string]any)
	if project := instantiatedData["project"].(map[string]any); project["slug"] != "newproj" {
		t.Fatalf("instantiated project = %#v", project)
	}

	secret := "sk-project-template-mcp-e2e-must-not-leak"
	stale := callMCPProjectTemplateE2E(t, session, "project_template_instantiate", map[string]any{
		"workspace": "local", "template": "launch", "snapshot_id": "stale-snapshot",
		"expected_snapshot_hash": wantSnapshotHash, "project_slug": "staleproj",
		"project_name": "过期项目", "start_date": "2026-08-01",
		"secret_inputs": map[string]any{"unused.secret": secret},
	})
	if !stale.IsError {
		t.Fatal("stale snapshot instantiate succeeded")
	}
	structured, _ := json.Marshal(stale.StructuredContent)
	if !strings.Contains(string(structured), "project_template_snapshot_hash_mismatch") ||
		strings.Contains(string(structured), secret) || strings.Contains(mcpProjectTemplateE2EText(t, stale), secret) {
		t.Fatalf("stale error is unsafe or unstructured: structured=%s text=%q", structured, mcpProjectTemplateE2EText(t, stale))
	}
}

func callMCPProjectTemplateE2E(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func assertMCPProjectTemplateE2EEnvelopeEquivalent(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected MCP error: %#v", result.StructuredContent)
	}
	var textEnvelope map[string]any
	if err := json.Unmarshal([]byte(mcpProjectTemplateE2EText(t, result)), &textEnvelope); err != nil {
		t.Fatalf("decode text envelope: %v", err)
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var structuredEnvelope map[string]any
	if err := json.Unmarshal(structured, &structuredEnvelope); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(textEnvelope, structuredEnvelope) {
		t.Fatalf("text/structuredContent diverged: text=%#v structured=%#v", textEnvelope, structuredEnvelope)
	}
	return structuredEnvelope
}

func mcpProjectTemplateE2EText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("MCP result has no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("MCP first content = %T, want text", result.Content[0])
	}
	return text.Text
}

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
