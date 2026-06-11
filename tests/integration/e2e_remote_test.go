package integration

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ERemoteCLIAndHTTPAPIGoldenPath(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	clientDB := filepath.Join(dir, "client.db")
	configPath, logPath := writeE2ELogConfig(t, dir)
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "remote-e2e", "*"))

	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db", serverDB)
	defer stopXuanchuServer(t, cmd)

	projectOut := run(t, bin, "--db", clientDB, "--server", baseURL, "--token", token, "project", "add", "e2eremote", "name:E2E Remote")
	if !strings.Contains(projectOut, "e2eremote") {
		t.Fatalf("remote project add output = %q", projectOut)
	}
	taskOut := run(t, bin, "--db", clientDB, "--server", baseURL, "--token", token, "add", "remote api golden task", "project:e2eremote", "+e2e")
	if !strings.Contains(taskOut, "Created task") {
		t.Fatalf("remote task add output = %q", taskOut)
	}
	remoteList := run(t, bin, "--db", clientDB, "--server", baseURL, "--token", token, "list")
	if !strings.Contains(remoteList, "remote api golden task") || !strings.Contains(remoteList, "e2eremote-1") {
		t.Fatalf("remote list output = %q", remoteList)
	}

	tasks := httpJSON(t, http.MethodGet, baseURL+"/api/v1/tasks", nil, authHeaders(token))
	taskRows, _ := tasks["data"].([]any)
	if !jsonArrayContainsString(taskRows, "description", "remote api golden task") {
		t.Fatalf("HTTP task list data = %#v, want remote task", taskRows)
	}
	projects := httpJSON(t, http.MethodGet, baseURL+"/api/v1/projects", nil, authHeaders(token))
	projectRows, _ := projects["data"].([]any)
	if !jsonArrayContainsString(projectRows, "slug", "e2eremote") {
		t.Fatalf("HTTP project list data = %#v, want e2eremote", projectRows)
	}
	me := httpJSON(t, http.MethodGet, baseURL+"/api/v1/me", nil, authHeaders(token))
	if !strings.Contains(toJSONString(t, me["data"]), `"name":"local"`) {
		t.Fatalf("HTTP me data = %#v, want local user", me["data"])
	}

	localList := run(t, bin, "--db", clientDB, "list")
	if strings.Contains(localList, "remote api golden task") || strings.Contains(localList, "e2eremote") {
		t.Fatalf("remote operation polluted client DB list = %q", localList)
	}

	stopXuanchuServer(t, cmd)
	assertLogContains(t, logPath,
		"component=http",
		"operation=http_request",
		"path=/api/v1/tasks",
		"path=/api/v1/projects",
	)
}

func jsonArrayContainsString(rows []any, key, want string) bool {
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if got, _ := m[key].(string); got == want {
			return true
		}
	}
	return false
}

func toJSONString(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
