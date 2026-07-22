package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ERemoteProjectTemplateJSONErrorsMatchLocalRuntimeErrors(t *testing.T) {
	bin := buildXuanchu(t)
	secret := "sk-remote-json-error-must-not-leak"
	var instantiateCurrentOnly bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/project-templates":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"permission_denied","message":"permission denied"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/project-templates/launch/instantiate":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			instantiateCurrentOnly, _ = body["current_only"].(bool)
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"code":"project_template_snapshot_hash_mismatch","message":"current snapshot changed"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"route_not_found","message":"unexpected route"}}`))
		}
	}))
	defer srv.Close()

	assertJSONError := func(t *testing.T, name string, args []string, stdin string, wantCode string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(bin, append([]string{"--server", srv.URL, "--token", "token", "--json"}, args...)...)
		cmd.Stdin = strings.NewReader(stdin)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err == nil {
			t.Fatalf("%s succeeded: stdout=%q stderr=%q", name, stdout.String(), stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("%s stdout = %q, want empty", name, stdout.String())
		}
		var payload map[string]string
		if err := json.Unmarshal(stderr.Bytes(), &payload); err != nil {
			t.Fatalf("%s stderr is not JSON: %v\nstderr=%q", name, err, stderr.String())
		}
		if payload["code"] != wantCode {
			t.Fatalf("%s error = %#v, want code %q", name, payload, wantCode)
		}
		if strings.Contains(stderr.String(), secret) {
			t.Fatalf("%s stderr leaked secret: %q", name, stderr.String())
		}
	}

	assertJSONError(t, "list permission", []string{"project", "template", "list"}, "", "permission_denied")
	assertJSONError(t, "instantiate hash drift", []string{
		"project", "template", "instantiate", "launch", "newproj", "name:新项目",
		"--snapshot", "snapshot-1", "--snapshot-hash", strings.Repeat("a", 64), "--start-date", "2026-08-01", "--input", "-",
	}, `{"secret_inputs":{"agent.api_key":"`+secret+`"}}`, "project_template_snapshot_hash_mismatch")
	if !instantiateCurrentOnly {
		t.Fatal("instantiate request did not preserve current_only")
	}
}

func TestE2EProjectTemplateRemoteCLIUsesCurrentOnlySurface(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	sourceSecret := "sk-source-must-not-leak"
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[security]\nconfig_secret_key = \"uH/SPe+6vEbjsJ7OljBz3SNy6BlKDInYsquKTz0MV/Q=\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "add", "tplsource", "name:模板来源")
	run(t, bin, "--db", serverDB, "--workspace", "local", "config", "schema", "set", "template.api_key", "type:string", "scopes:project", "secret:true")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "tplsource", "template.api_key", sourceSecret)
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "template-remote-e2e", "*"))
	server, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db", serverDB)
	defer stopXuanchuServer(t, server)
	headers := authHeaders(token)

	capture := map[string]any{
		"source_project": "tplsource", "anchor_date": "2026-07-20",
		"selection": map[string]any{
			"config_keys": []string{"template.api_key"}, "task_refs": []string{},
			"series_refs": []string{}, "automation_rule_ids": []string{},
		},
	}
	preview := httpJSON(t, http.MethodPost, baseURL+"/api/v1/project-templates/capture-preview?workspace=local", capture, headers)
	capture["expected_source_hash"] = preview["data"].(map[string]any)["source_hash"].(string)
	created := httpJSON(t, http.MethodPost, baseURL+"/api/v1/project-templates?workspace=local", map[string]any{
		"key": "launch", "name": "启动模板", "description": "远程 CLI 模板", "capture": capture,
	}, headers)
	historicalID, historicalHash := projectTemplateE2ECurrent(t, created)

	remoteList := run(t, bin, "--server", baseURL, "--token", token, "--workspace", "local", "--json", "project", "template", "list", "--q", "launch", "--limit", "10", "--offset", "0")
	localList := run(t, bin, "--db", serverDB, "--workspace", "local", "--json", "project", "template", "list", "--q", "launch", "--limit", "10", "--offset", "0")
	if remoteList != localList {
		t.Fatalf("local/remote template list mismatch\nlocal=%s\nremote=%s", localList, remoteList)
	}
	for _, forbidden := range []string{"snapshot_json", sourceSecret} {
		if strings.Contains(remoteList, forbidden) {
			t.Fatalf("template list exposed %q: %s", forbidden, remoteList)
		}
	}

	var stdout, stderr bytes.Buffer
	instantiate := exec.Command(bin,
		"--server", baseURL, "--token", token, "--workspace", "local",
		"project", "template", "instantiate", "launch", "remoteproj", "name:远程创建",
		"--snapshot", historicalID, "--snapshot-hash", historicalHash, "--start-date", "2026-08-01", "--input", "-",
	)
	instantiate.Stdin = strings.NewReader(`{"description":"覆盖说明"}`)
	instantiate.Stdout, instantiate.Stderr = &stdout, &stderr
	if err := instantiate.Run(); err != nil {
		t.Fatalf("remote instantiate error=%v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "remoteproj") || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, secret := range []string{sourceSecret} {
		if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Fatalf("secret %q leaked: stdout=%q stderr=%q", secret, stdout.String(), stderr.String())
		}
	}

	// 推进 current Snapshot 后，Remote 的单次 current_only 请求必须原子拒绝旧 ID/hash。
	httpJSON(t, http.MethodPatch, baseURL+"/api/v1/projects/tplsource?workspace=local", map[string]any{"description": "第二版来源"}, headers)
	preview = httpJSON(t, http.MethodPost, baseURL+"/api/v1/project-templates/launch/snapshots/capture-preview?workspace=local", capture, headers)
	capture["expected_source_hash"] = preview["data"].(map[string]any)["source_hash"].(string)
	appended := httpJSON(t, http.MethodPost, baseURL+"/api/v1/project-templates/launch/snapshots?workspace=local", capture, headers)
	currentID, _ := projectTemplateE2ECurrent(t, appended)
	if currentID == historicalID {
		t.Fatal("current snapshot did not advance")
	}

	stdout.Reset()
	stderr.Reset()
	stale := exec.Command(bin,
		"--server", baseURL, "--token", token, "--workspace", "local",
		"project", "template", "instantiate", "launch", "stalecreate", "name:过期创建",
		"--snapshot", historicalID, "--snapshot-hash", historicalHash, "--start-date", "2026-08-01", "--input", "-",
	)
	stale.Stdin = strings.NewReader(`{}`)
	stale.Stdout, stale.Stderr = &stdout, &stderr
	if err := stale.Run(); err == nil {
		t.Fatalf("stale remote instantiate succeeded: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "project_template_snapshot_hash_mismatch") {
		t.Fatalf("stale stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), sourceSecret) {
		t.Fatalf("stale error leaked source secret: %s", stderr.String())
	}
}

func projectTemplateE2ECurrent(t *testing.T, envelope map[string]any) (string, string) {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", envelope)
	}
	template, ok := data["template"].(map[string]any)
	if !ok {
		t.Fatalf("missing template: %#v", data)
	}
	current, ok := template["current_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("missing current_snapshot: %#v", template)
	}
	id, _ := current["id"].(string)
	hash, _ := current["hash"].(string)
	if id == "" || hash == "" {
		t.Fatalf("invalid current_snapshot: %#v", current)
	}
	return id, hash
}

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
	taskData, _ := tasks["data"].(map[string]any)
	taskRows, _ := taskData["items"].([]any)
	if !jsonArrayContainsString(taskRows, "title", "remote api golden task") {
		t.Fatalf("HTTP task list items = %#v, want remote task", taskRows)
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
