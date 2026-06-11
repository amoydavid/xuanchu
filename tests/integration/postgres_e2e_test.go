package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func postgresE2EAdminURL(t *testing.T) string {
	t.Helper()
	adminURL := strings.TrimSpace(os.Getenv("XUANCHU_E2E_POSTGRES_ADMIN_URL"))
	if adminURL == "" {
		t.Skip("XUANCHU_E2E_POSTGRES_ADMIN_URL not set")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql not found; skipping PostgreSQL E2E")
	}
	return adminURL
}

func createPostgresE2EDatabase(t *testing.T, adminURL string) string {
	t.Helper()
	dbName := fmt.Sprintf("xuanchu_e2e_%d_%d", time.Now().UnixNano(), os.Getpid())
	psqlExec(t, adminURL, "CREATE DATABASE "+dbName)
	t.Cleanup(func() {
		psqlExec(t, adminURL, "DROP DATABASE IF EXISTS "+dbName)
	})
	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse admin postgres url: %v", err)
	}
	parsed.Path = "/" + dbName
	return parsed.String()
}

func psqlExec(t *testing.T, dbURL string, sql string) {
	t.Helper()
	cmd := exec.Command("psql", dbURL, "-v", "ON_ERROR_STOP=1", "-q", "-c", sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("psql %q error = %v\n%s", sql, err, out)
	}
}

func TestPostgresE2EServerHTTPMCPAndLogs(t *testing.T) {
	adminURL := postgresE2EAdminURL(t)
	dbURL := createPostgresE2EDatabase(t, adminURL)
	bin := buildXuanchu(t)
	dir := t.TempDir()
	configPath, logPath := writeE2ELogConfig(t, dir, "trusted.example")
	token := parseRawToken(t, createTokenJSON(t, bin, "--db-url", dbURL, "postgres-e2e", "*"))

	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db-url", dbURL)
	defer stopXuanchuServer(t, cmd)

	if status := httpStatus(t, http.MethodGet, baseURL+"/healthz", nil, nil); status != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", status)
	}
	me := httpJSON(t, http.MethodGet, baseURL+"/api/v1/me", nil, authHeaders(token))
	if !strings.Contains(toJSONString(t, me["data"]), `"name":"local"`) {
		t.Fatalf("PostgreSQL /me data = %#v", me["data"])
	}

	initBody := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}`)
	if status := httpStatus(t, http.MethodPost, baseURL+"/mcp", initBody, map[string]string{"Content-Type": "application/json", "Host": "evil.example"}); status != http.StatusForbidden {
		t.Fatalf("POST /mcp with evil Host status = %d, want 403", status)
	}
	initBody = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}`)
	if status := httpStatus(t, http.MethodPost, baseURL+"/mcp", initBody, map[string]string{"Content-Type": "application/json", "Host": "trusted.example"}); status != http.StatusUnauthorized {
		t.Fatalf("POST /mcp with trusted Host without token status = %d, want 401", status)
	}

	project := httpJSON(t, http.MethodPost, baseURL+"/api/v1/projects", map[string]any{
		"slug": "pge2e",
		"name": "PostgreSQL E2E",
	}, authHeaders(token))
	if nestedMap(t, project, "data")["slug"] != "pge2e" {
		t.Fatalf("PostgreSQL project add response = %#v", project)
	}
	task := httpJSON(t, http.MethodPost, baseURL+"/api/v1/tasks", map[string]any{
		"description": "postgres http api task",
		"project":     "pge2e",
	}, authHeaders(token))
	if nestedMap(t, task, "data")["description"] != "postgres http api task" {
		t.Fatalf("PostgreSQL task add response = %#v", task)
	}

	session, cancel := connectHTTPMCP(t, baseURL, token)
	defer cancel()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "task_add",
		Arguments: map[string]any{
			"description": "postgres mcp task",
			"project":     "pge2e",
		},
	})
	if err != nil {
		t.Fatalf("PostgreSQL HTTP MCP task_add error = %v", err)
	}
	env := mcpStructuredMap(t, result)
	if nestedMap(t, nestedMap(t, env, "data"), "task")["description"] != "postgres mcp task" {
		t.Fatalf("PostgreSQL HTTP MCP structured content = %#v", env)
	}

	stopXuanchuServer(t, cmd)
	assertLogContains(t, logPath,
		"operation=http_request",
		"workspace_id=",
		"workspace_ref=local",
		"operation=mcp_tool_call",
		"tool=task_add",
	)
}
