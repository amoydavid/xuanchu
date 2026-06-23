package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gsqlite "github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	buildXuanchuOnce sync.Once
	buildXuanchuPath string
	buildXuanchuErr  error
)

func TestCLIAddListInfo(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "write", "spec", "+planning")
	out := run(t, bin, "--db", db, "list")
	if !strings.Contains(out, "write spec") {
		t.Fatalf("list output = %q", out)
	}
	if !strings.Contains(out, "planning") {
		t.Fatalf("list output missing tag = %q", out)
	}
	info := run(t, bin, "--db", db, "info", "1")
	if !strings.Contains(info, "UUID") || !strings.Contains(info, "write spec") {
		t.Fatalf("info output = %q", info)
	}
}

func TestCLITaskSlugTargets(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "API", "name:API")
	run(t, bin, "--db", db, "add", "Write docs", "project:api", "+next")

	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "api-1") {
		t.Fatalf("list output missing task slug: %q", list)
	}

	info := run(t, bin, "--db", db, "info", "api-1")
	if !strings.Contains(info, "Task slug:") || !strings.Contains(info, "api-1") {
		t.Fatalf("info api-1 output = %q", info)
	}
	listTarget := run(t, bin, "--db", db, "list", "api-1")
	if !strings.Contains(listTarget, "Write docs") || !strings.Contains(listTarget, "api-1") {
		t.Fatalf("list api-1 output = %q", listTarget)
	}

	uuidOut := strings.TrimSpace(run(t, bin, "--db", db, "_get", "api-1.uuid"))
	if uuidOut == "" || strings.Contains(uuidOut, "api-1") {
		t.Fatalf("_get api-1.uuid output = %q", uuidOut)
	}

	urgency := run(t, bin, "--db", db, "urgency", "api-1")
	if !strings.Contains(urgency, "tag.next") {
		t.Fatalf("urgency api-1 output = %q", urgency)
	}

	linkAdd := run(t, bin, "--db", db, "api-1", "link", "add", "--type", "doc", "--url", "https://example.com/spec")
	if !strings.Contains(linkAdd, "Added") {
		t.Fatalf("link add api-1 output = %q", linkAdd)
	}
	linkList := run(t, bin, "--db", db, "api-1", "link", "list")
	if !strings.Contains(linkList, "[doc]") || !strings.Contains(linkList, "https://example.com/spec") {
		t.Fatalf("link list api-1 output = %q", linkList)
	}

	infoJSON := run(t, bin, "--db", db, "--json", "info", "api-1")
	var infoPayload map[string]any
	if err := json.Unmarshal([]byte(infoJSON), &infoPayload); err != nil {
		t.Fatalf("info api-1 --json parse error = %v, output = %q", err, infoJSON)
	}
	if infoPayload["task_slug"] != "api-1" {
		t.Fatalf("task_slug = %v, want api-1", infoPayload["task_slug"])
	}
	linksRaw, _ := infoPayload["links"].([]any)
	if len(linksRaw) != 1 {
		t.Fatalf("links count = %d, want 1", len(linksRaw))
	}
	linkMap, _ := linksRaw[0].(map[string]any)
	linkID, _ := linkMap["id"].(string)
	if linkID == "" {
		t.Fatalf("link id missing in %s", infoJSON)
	}
	linkRemove := run(t, bin, "--db", db, "api-1", "link", "remove", linkID)
	if !strings.Contains(linkRemove, "Removed") {
		t.Fatalf("link remove api-1 output = %q", linkRemove)
	}

	run(t, bin, "--db", db, "api-1", "done")
}

func TestCLIAddWithAssignees(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "write", "spec", "@local")
	out := run(t, bin, "--db", db, "--json", "info", "1")

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("info --json output is not JSON: %v\n%s", err, out)
	}
	assignees, ok := payload["assignees"].([]any)
	if !ok || len(assignees) != 1 {
		t.Fatalf("assignees = %#v, want one assignee", payload["assignees"])
	}
	first, ok := assignees[0].(map[string]any)
	if !ok || first["name"] != "local" {
		t.Fatalf("first assignee = %#v, want name local", assignees[0])
	}
}

func TestCLIModifyAssignees(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "write", "spec")
	run(t, bin, "--db", db, "1", "modify", "+@local")
	out := run(t, bin, "--db", db, "--json", "info", "1")

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("info --json output is not JSON: %v\n%s", err, out)
	}
	assignees, ok := payload["assignees"].([]any)
	if !ok || len(assignees) != 1 {
		t.Fatalf("assignees after add = %#v, want one assignee", payload["assignees"])
	}

	run(t, bin, "--db", db, "1", "modify", "-@local")
	out = run(t, bin, "--db", db, "--json", "info", "1")
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("info --json output is not JSON after remove: %v\n%s", err, out)
	}
	assignees, ok = payload["assignees"].([]any)
	if !ok || len(assignees) != 0 {
		t.Fatalf("assignees after remove = %#v, want empty array", payload["assignees"])
	}
}

func TestCLIInfoShowsAssignees(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "write", "spec", "@local")
	info := run(t, bin, "--db", db, "info", "1")
	if !strings.Contains(info, "Assignees:") || !strings.Contains(info, "@local") {
		t.Fatalf("info output = %q", info)
	}
}

func TestCLIListByAssignee(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "my", "task", "@local")
	run(t, bin, "--db", db, "add", "other", "task")
	out := run(t, bin, "--db", db, "list", "assignee:me")
	if !strings.Contains(out, "my task") || strings.Contains(out, "other task") {
		t.Fatalf("list output = %q", out)
	}
}

func TestCLIShowAndConfig(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	out := run(t, bin, "--db", db, "show")
	if !strings.Contains(out, "database.path") {
		t.Fatalf("show output = %q", out)
	}
	run(t, bin, "--db", db, "config", "set", "date.format", "rfc3339")
	out = run(t, bin, "--db", db, "config", "get", "date.format")
	if strings.TrimSpace(out) != "rfc3339" {
		t.Fatalf("config get output = %q", out)
	}
}

func TestCLITokenCreateListRevoke(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	out := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "cli", "--scope", "task:read", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("missing raw token: %s", out)
	}
	list := run(t, bin, "--db", db, "token", "list")
	if !strings.Contains(list, "cli") || strings.Contains(list, token) {
		t.Fatalf("token list leaked raw token: %q", list)
	}
	run(t, bin, "--db", db, "token", "revoke", created["id"].(string))
	all := run(t, bin, "--db", db, "token", "list", "--all")
	if !strings.Contains(all, "cli") {
		t.Fatalf("token list --all output = %q", all)
	}
}

func TestCLIServerHealthz(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, string(body))
	}
}

func TestCLIServerWritesOperationLogsToConfiguredFile(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "xuanchu.db")
	logPath := filepath.Join(dir, "xuanchu.log")
	configPath := filepath.Join(dir, "xuanchu.toml")
	if err := os.WriteFile(configPath, []byte(strings.Join([]string{
		"[log]",
		`level = "info"`,
		`format = "text"`,
		`file = "` + logPath + `"`,
		`rotate = "none"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db", db)
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		stopXuanchuServer(t, cmd)
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	stopXuanchuServer(t, cmd)

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logBytes)
	for _, want := range []string{
		"component=server",
		"operation=server_listening",
		"component=http",
		"operation=http_request",
		"path=/healthz",
		"operation=shutdown_signal",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log file = %q, want substring %q", logText, want)
		}
	}
}

func TestMCPStdioListTools(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.Command(bin, "--db", db, "mcp", "stdio")
	client := mcp.NewClient(&mcp.Implementation{Name: "xuanchu-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect mcp stdio: %v", err)
	}
	defer session.Close()

	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatalf("ListTools: %v", err)
	}
}

func TestCLIServerMeWithBearerToken(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	out := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "http", "--scope", "task:read", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("missing raw token: %s", out)
	}

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), `"name":"local"`) || !strings.Contains(string(body), `"type":"pat"`) {
		t.Fatalf("body = %s", string(body))
	}
}

func TestCLIRemoteAddListInfoAndProject(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "remote", "--scope", "task:read,task:write,project:read,project:write", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("missing token in %s", tokenOut)
	}

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	run(t, bin, "--server", baseURL, "--token", token, "project", "add", "remote", "name:Remote")
	projectList := run(t, bin, "--server", baseURL, "--token", token, "project", "list")
	if !strings.Contains(projectList, "remote Remote") {
		t.Fatalf("remote project list = %q", projectList)
	}

	run(t, bin, "--server", baseURL, "--token", token, "add", "remote", "task", "project:remote", "+net")
	list := run(t, bin, "--server", baseURL, "--token", token, "list")
	if !strings.Contains(list, "remote task") {
		t.Fatalf("remote list = %q", list)
	}
	info := run(t, bin, "--server", baseURL, "--token", token, "info", "1")
	if !strings.Contains(info, "remote task") || !strings.Contains(info, "Project") {
		t.Fatalf("remote info = %q", info)
	}
	if errOut := runExpectError(t, bin, "--server", baseURL, "--token", token, "info", "999999999999999999999999999999"); !strings.Contains(errOut, "not found") {
		t.Fatalf("remote info huge numeric error = %q", errOut)
	}
	listTarget := run(t, bin, "--server", baseURL, "--token", token, "list", "1")
	if !strings.Contains(listTarget, "remote task") || !strings.Contains(listTarget, "remote-1") {
		t.Fatalf("remote list by numeric target = %q", listTarget)
	}
	slugInfo := run(t, bin, "--server", baseURL, "--token", token, "info", "remote-1")
	if !strings.Contains(slugInfo, "remote task") || !strings.Contains(slugInfo, "Task slug:") {
		t.Fatalf("remote info by task_slug = %q", slugInfo)
	}
	slugList := run(t, bin, "--server", baseURL, "--token", token, "list", "remote-1")
	if !strings.Contains(slugList, "remote task") || !strings.Contains(slugList, "remote-1") {
		t.Fatalf("remote list by task_slug = %q", slugList)
	}

	remoteInfoJSON := run(t, bin, "--server", baseURL, "--token", token, "--json", "project", "info", "remote")
	var remoteProject map[string]any
	if err := json.Unmarshal([]byte(remoteInfoJSON), &remoteProject); err != nil {
		t.Fatal(err)
	}
	run(t, bin, "--server", baseURL, "--token", token, "project", "add", "other", "name:Other")
	otherAddJSON := run(t, bin, "--server", baseURL, "--token", token, "--json", "add", "other", "task", "project:other")
	var otherTask map[string]any
	if err := json.Unmarshal([]byte(otherAddJSON), &otherTask); err != nil {
		t.Fatal(err)
	}
	if errOut := runExpectError(t, bin, "--server", baseURL, "--token", token, "--project-id", remoteProject["id"].(string), "info", otherTask["uuid"].(string)); !strings.Contains(errOut, "not found") {
		t.Fatalf("remote project scoped full UUID error = %q", errOut)
	}
}

func TestCLIRemoteTokenList(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "remote", "--scope", "token:read,task:read", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("missing token in %s", tokenOut)
	}

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	list := run(t, bin, "--server", baseURL, "--token", token, "token", "list")
	if !strings.Contains(list, "remote") {
		t.Fatalf("remote token list = %q", list)
	}
}

func TestCLIRemoteTargetActionWritesRemoteNotLocalDB(t *testing.T) {
	bin := buildXuanchu(t)
	serverDB := filepath.Join(t.TempDir(), "server.db")
	localDB := filepath.Join(t.TempDir(), "local.db")

	tokenOut := run(t, bin, "--db", serverDB, "--json", "--workspace", "local", "token", "create", "remote", "--scope", "task:read,task:write", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("missing token in %s", tokenOut)
	}

	cmd, baseURL := startXuanchuServer(t, bin, "--db", serverDB)
	defer stopXuanchuServer(t, cmd)

	run(t, bin, "--server", baseURL, "--token", token, "add", "remote", "safety")
	run(t, bin, "--db", localDB, "add", "local", "safety")
	run(t, bin, "--db", localDB, "--server", baseURL, "--token", token, "1", "done")
	list := run(t, bin, "--db", localDB, "list")
	if !strings.Contains(list, "local safety") {
		t.Fatalf("local task was modified by remote target action; list = %q", list)
	}
	remoteList := run(t, bin, "--server", baseURL, "--token", token, "list")
	if strings.Contains(remoteList, "remote safety") {
		t.Fatalf("remote task was not completed; list = %q", remoteList)
	}
}

func TestCLIRemoteFilteredIDsUseDefaultWorkingSetPositions(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "remote", "--scope", "task:read,task:write", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	run(t, bin, "--server", baseURL, "--token", token, "add", "plain", "task")
	run(t, bin, "--server", baseURL, "--token", token, "add", "tagged", "task", "+net")

	if ids := strings.TrimSpace(run(t, bin, "--server", baseURL, "--token", token, "_ids", "+net")); ids != "2" {
		t.Fatalf("remote _ids +net = %q, want 2", ids)
	}
	list := run(t, bin, "--server", baseURL, "--token", token, "list", "+net")
	if !strings.Contains(list, "\n2   ") {
		t.Fatalf("remote filtered list should render working-set ID 2: %q", list)
	}
	run(t, bin, "--server", baseURL, "--token", token, "2", "done")
	remaining := run(t, bin, "--server", baseURL, "--token", token, "list")
	if strings.Contains(remaining, "tagged task") || !strings.Contains(remaining, "plain task") {
		t.Fatalf("remote 2 done modified wrong task; list = %q", remaining)
	}
}

func TestCLIRemoteIDsAreSortedByWorkingSetPosition(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "remote", "--scope", "task:read,task:write", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	run(t, bin, "--server", baseURL, "--token", token, "add", "low", "match", "+x", "priority:L")
	run(t, bin, "--server", baseURL, "--token", token, "add", "high", "match", "+x", "priority:H")
	run(t, bin, "--server", baseURL, "--token", token, "add", "middle", "miss", "priority:M")

	if ids := strings.TrimSpace(run(t, bin, "--server", baseURL, "--token", token, "_ids", "priority:H", "or", "priority:L")); ids != "1\n2" {
		t.Fatalf("remote _ids order = %q, want 1\\n2", ids)
	}
}

func TestCLIRemoteAddAndModifyPreserveTaskwarriorFields(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "remote", "--scope", "task:read,task:write", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	run(t, bin, "--server", baseURL, "--token", token, "add", "remote", "deadline", "due:2030-01-01")
	run(t, bin, "--server", baseURL, "--token", token, "1", "modify", "wait:2030-01-02", "+blocked")
	info := run(t, bin, "--server", baseURL, "--token", token, "--json", "info", "1")
	var row map[string]any
	if err := json.Unmarshal([]byte(info), &row); err != nil {
		t.Fatal(err)
	}
	if row["due"] == nil || row["wait"] == nil {
		t.Fatalf("remote add/modify dropped due or wait: %s", info)
	}
	tags, _ := row["tags"].([]any)
	found := false
	for _, tag := range tags {
		if tag == "blocked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("remote modify dropped tag: %s", info)
	}
}

func TestCLIRemoteConfigImportTaskRCUnsupported(t *testing.T) {
	bin := buildXuanchu(t)
	serverDB := filepath.Join(t.TempDir(), "server.db")
	localDB := filepath.Join(t.TempDir(), "local.db")
	taskrcPath := filepath.Join(t.TempDir(), ".taskrc")
	if err := os.WriteFile(taskrcPath, []byte("dateformat=Y-M-D\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tokenOut := run(t, bin, "--db", serverDB, "--json", "--workspace", "local", "token", "create", "remote", "--scope", "config:write", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	cmd, baseURL := startXuanchuServer(t, bin, "--db", serverDB)
	defer stopXuanchuServer(t, cmd)

	errOut := runExpectError(t, bin, "--db", localDB, "--server", baseURL, "--token", token, "config", "import-taskrc", taskrcPath, "--dry-run")
	if !strings.Contains(errOut, "remote_unsupported_command") {
		t.Fatalf("remote config import-taskrc error = %q", errOut)
	}
	if _, err := os.Stat(localDB); !os.IsNotExist(err) {
		t.Fatalf("remote unsupported command touched local db; stat err=%v", err)
	}
}

func TestCLIRemoteUnsupportedManagementCommandsDoNotTouchLocalDB(t *testing.T) {
	bin := buildXuanchu(t)
	localDB := filepath.Join(t.TempDir(), "local.db")
	// user use 仍然不支持远程模式（因为需要切换本地身份）
	cases := [][]string{
		{"user", "use"},
		{"user", "use", "local"},
	}
	for _, args := range cases {
		full := append([]string{"--db", localDB, "--server", "http://127.0.0.1:1", "--token", "x"}, args...)
		out := runExpectError(t, bin, full...)
		if !strings.Contains(out, "remote_unsupported_command") {
			t.Fatalf("%v: expected remote_unsupported_command, got %q", args, out)
		}
	}
	if _, err := os.Stat(localDB); !os.IsNotExist(err) {
		t.Fatalf("remote unsupported commands touched local db; stat err=%v", err)
	}
}

func TestCLIRemoteManagementCommandsDoNotTouchLocalDB(t *testing.T) {
	bin := buildXuanchu(t)
	localDB := filepath.Join(t.TempDir(), "local.db")
	// 这些命令现在支持远程模式，但应使用远程服务器而不触碰本地 DB
	cases := [][]string{
		{"show"},
		{"user", "list"},
		{"user", "info"},
		{"workspace", "list"},
		{"workspace", "info"},
		{"member", "list"},
	}
	for _, args := range cases {
		full := append([]string{"--db", localDB, "--server", "http://127.0.0.1:1", "--token", "x"}, args...)
		out := runExpectError(t, bin, full...)
		// 不应包含 remote_unsupported_command，而是连接错误
		if strings.Contains(out, "remote_unsupported_command") {
			t.Fatalf("%v: should be remote-supported now, got %q", args, out)
		}
	}
	if _, err := os.Stat(localDB); !os.IsNotExist(err) {
		t.Fatalf("remote management commands touched local db; stat err=%v", err)
	}
}

func TestCLIRemoteContextConfigHelpersImportExportAndAudit(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	tokenOut := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "remote-full", "--scope", "task:read,task:write,project:read,project:write,context:read,context:write,config:read,config:write,audit:read", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(tokenOut), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("missing token in %s", tokenOut)
	}

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	run(t, bin, "--server", baseURL, "--token", token, "project", "add", "api", "name:API")
	run(t, bin, "--server", baseURL, "--token", token, "project", "config", "set", "api", "agent.background", "remote docs")
	if got := strings.TrimSpace(run(t, bin, "--server", baseURL, "--token", token, "project", "config", "get", "api", "agent.background")); got != "remote docs" {
		t.Fatalf("remote project config get = %q", got)
	}
	if list := run(t, bin, "--server", baseURL, "--token", token, "project", "config", "list", "api"); !strings.Contains(list, "agent.background=remote docs") {
		t.Fatalf("remote project config list = %q", list)
	}
	run(t, bin, "--server", baseURL, "--token", token, "project", "config", "unset", "api", "agent.background")

	run(t, bin, "--server", baseURL, "--token", token, "config", "set", "uda.estimate.type", "numeric")
	if got := strings.TrimSpace(run(t, bin, "--server", baseURL, "--token", token, "config", "get", "uda.estimate.type")); got != "numeric" {
		t.Fatalf("remote config get = %q", got)
	}
	if list := run(t, bin, "--server", baseURL, "--token", token, "config", "list"); !strings.Contains(list, "uda.estimate.type=numeric") {
		t.Fatalf("remote config list = %q", list)
	}

	run(t, bin, "--server", baseURL, "--token", token, "add", "remote", "helper", "project:api", "+net", "estimate:3")
	run(t, bin, "--server", baseURL, "--token", token, "context", "define", "api", "project:api")
	run(t, bin, "--server", baseURL, "--token", token, "context", "use", "api")
	if show := run(t, bin, "--server", baseURL, "--token", token, "context", "show"); !strings.Contains(show, "api project:api") {
		t.Fatalf("remote context show = %q", show)
	}
	if unique := strings.TrimSpace(run(t, bin, "--server", baseURL, "--token", token, "_unique", "estimate")); unique != "3" {
		t.Fatalf("remote _unique = %q", unique)
	}
	if ids := strings.TrimSpace(run(t, bin, "--server", baseURL, "--token", token, "_ids", "+net")); ids != "1" {
		t.Fatalf("remote _ids = %q", ids)
	}
	run(t, bin, "--server", baseURL, "--token", token, "add", "remote", "outside")
	if list := run(t, bin, "--server", baseURL, "--token", token, "list"); strings.Contains(list, "remote outside") {
		t.Fatalf("remote list ignored active context: %q", list)
	}
	if list := run(t, bin, "--server", baseURL, "--token", token, "--no-context", "list"); !strings.Contains(list, "remote outside") {
		t.Fatalf("remote --no-context list = %q", list)
	}
	if projects := run(t, bin, "--server", baseURL, "--token", token, "_projects"); !strings.Contains(projects, "api") {
		t.Fatalf("remote _projects = %q", projects)
	}
	run(t, bin, "--server", baseURL, "--token", token, "context", "none")

	exported := run(t, bin, "--server", baseURL, "--token", token, "export")
	importDB := filepath.Join(t.TempDir(), "import.db")
	importTokenOut := run(t, bin, "--db", importDB, "--json", "--workspace", "local", "token", "create", "remote-import", "--scope", "task:read,task:write,project:write", "--expires-in", "720h")
	var importCreated map[string]any
	if err := json.Unmarshal([]byte(importTokenOut), &importCreated); err != nil {
		t.Fatal(err)
	}
	importToken, _ := importCreated["token"].(string)
	importServer, importBaseURL := startXuanchuServer(t, bin, "--db", importDB)
	defer stopXuanchuServer(t, importServer)
	run(t, bin, "--server", importBaseURL, "--token", importToken, "project", "add", "api", "name:API")
	importPath := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(importPath, []byte(exported), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, bin, "--server", importBaseURL, "--token", importToken, "import", importPath)
	if imported := run(t, bin, "--server", importBaseURL, "--token", importToken, "list"); !strings.Contains(imported, "remote helper") {
		t.Fatalf("remote imported list = %q", imported)
	}

	if audit := run(t, bin, "--server", baseURL, "--token", token, "audit", "list"); !strings.Contains(audit, "task.add") {
		t.Fatalf("remote audit list = %q", audit)
	}
	if errOut := runExpectError(t, bin, "--server", baseURL, "--token", token, "_show", "database.path"); !strings.Contains(errOut, "remote_unsupported_command") {
		t.Fatalf("remote _show database.path error = %q", errOut)
	}
}

func TestCLITomlRuntimeAffectsServiceBehavior(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "xuanchu.db")
	configDir := filepath.Join(dir, "config", "xuanchu")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
		"[uda.estimate]",
		"type = \"numeric\"",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	run(t, bin, "--db", db, "project", "add", "work", "name:Work")
	run(t, bin, "--db", db, "project", "add", "home", "name:Home")
	run(t, bin, "--db", db, "context", "define", "work", "project:work")
	runWithEnv(t, map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config")}, bin, "--db", db, "add", "work", "task", "project:work", "estimate:3")
	runWithEnv(t, map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config")}, bin, "--db", db, "add", "home", "task", "project:home", "estimate:5")

	show := runWithEnv(t, map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config")}, bin, "--db", db, "_show", "active.context", "uda.estimate.type")
	if strings.TrimSpace(show) != "\nnumeric" && strings.TrimSpace(show) != "numeric" {
		t.Fatalf("_show from TOML = %q", show)
	}
	list := runWithEnv(t, map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config")}, bin, "--db", db, "list")
	if !strings.Contains(list, "work task") || !strings.Contains(list, "home task") {
		t.Fatalf("list should ignore TOML active context in M4: %q", list)
	}
	filtered := runWithEnv(t, map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config")}, bin, "--db", db, "estimate:3", "list")
	if !strings.Contains(filtered, "work task") || strings.Contains(filtered, "home task") {
		t.Fatalf("TOML UDA schema not applied to query: %q", filtered)
	}
}

func TestCLIShowDatabasePathUsesActualResolvedPath(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config", "xuanchu")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlDB := filepath.Join(dir, "toml.db")
	flagDB := filepath.Join(dir, "flag.db")
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte("[database]\npath = \""+tomlDB+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(runWithEnv(t, map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config")}, bin, "--db", flagDB, "_show", "database.path"))
	if got != flagDB {
		t.Fatalf("database.path = %q, want actual --db path %q", got, flagDB)
	}
	got = strings.TrimSpace(runWithEnv(t, map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config")}, bin, "--db", flagDB, "config", "get", "database.path"))
	if got != flagDB {
		t.Fatalf("config get database.path = %q, want actual --db path %q", got, flagDB)
	}
}

func TestCLIConfigListUnsetAndShow(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "config", "set", "date.format", "epoch")
	list := run(t, bin, "--db", db, "config", "list")
	if !strings.Contains(list, "date.format=epoch") || !strings.Contains(list, "database.path="+db) {
		t.Fatalf("config list output = %q", list)
	}
	show := run(t, bin, "--db", db, "show")
	if !strings.Contains(show, "date.format=epoch") {
		t.Fatalf("show output = %q", show)
	}
	run(t, bin, "--db", db, "config", "unset", "date.format")
	got := strings.TrimSpace(run(t, bin, "--db", db, "config", "get", "date.format"))
	if got != "rfc3339" {
		t.Fatalf("date.format after unset = %q", got)
	}
}

func TestCLIConfigSetRejectsUnsupportedBusinessKey(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	out := runExpectError(t, bin, "--db", db, "config", "set", "arbitrary.thing", "1")
	if !strings.Contains(out, "config_definition_not_found") {
		t.Fatalf("config set arbitrary.thing error = %q", out)
	}
	list := run(t, bin, "--db", db, "config", "list")
	if strings.Contains(list, "arbitrary.thing") {
		t.Fatalf("unsupported config key was persisted: %q", list)
	}
}

func TestCLIProjectsHelperListsAllSlugs(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "alpha", "name:Alpha")
	run(t, bin, "--db", db, "project", "add", "beta", "name:Beta")
	out := run(t, bin, "--db", db, "_projects")
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("_projects = %q, want both slugs", out)
	}
}

func TestCLIUDAConfigAndImport(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "config", "set", "uda.estimate.type", "numeric")
	run(t, bin, "--db", db, "config", "set", "uda.estimate.label", "Estimate")
	run(t, bin, "--db", db, "config", "set", "uda.estimate.values", "1,2,3,5,8")
	if got := strings.TrimSpace(run(t, bin, "--db", db, "config", "get", "uda.estimate.values")); got != "1,2,3,5,8" {
		t.Fatalf("config get uda values = %q", got)
	}
	run(t, bin, "--db", db, "add", "estimated", "task", "estimate:3")
	run(t, bin, "--db", db, "add", "bigger", "task", "estimate:5")
	exported := run(t, bin, "--db", db, "--json", "export")
	if !strings.Contains(exported, `"estimate": "3"`) {
		t.Fatalf("export missing estimate UDA: %q", exported)
	}
	list := run(t, bin, "--db", db, "estimate:3", "list")
	if !strings.Contains(list, "estimated task") || strings.Contains(list, "bigger task") {
		t.Fatalf("estimate query output = %q", list)
	}
	udas := run(t, bin, "--db", db, "_udas")
	if strings.TrimSpace(udas) != "estimate" {
		t.Fatalf("_udas output = %q", udas)
	}
	unique := run(t, bin, "--db", db, "_unique", "estimate")
	if strings.TrimSpace(unique) != "3\n5" {
		t.Fatalf("_unique estimate output = %q", unique)
	}

	path := filepath.Join(t.TempDir(), "orphan.json")
	if err := os.WriteFile(path, []byte(`[{"uuid":"u1","title":"legacy task","status":"pending","entry":"1970-01-01T00:01:40Z","modified":"1970-01-01T00:01:40Z","legacy_field":"old"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, bin, "--db", db, "import", path)
	legacyUUID := strings.TrimSpace(run(t, bin, "--db", db, "_uuids", "/legacy/"))
	got := run(t, bin, "--db", db, "_get", legacyUUID+".legacy_field")
	if strings.TrimSpace(got) != "old" {
		t.Fatalf("_get orphan UDA = %q", got)
	}
	cmd := exec.Command(bin, "--db", db, legacyUUID, "modify", "legacy_field:new")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("modify orphan UDA succeeded unexpectedly:\n%s", out)
	}
}

func TestCLIImportTaskRC(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	path := filepath.Join(t.TempDir(), ".taskrc")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"color=off",
		"dateformat=epoch",
		"context.work=project:work",
		"uda.estimate.type=numeric",
		"uda.estimate.values=1,2,3",
		"urgency.uda.estimate.coefficient=2",
		"report.next.columns=id,title",
		"unknown.value=yes",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	dry := run(t, bin, "--db", db, "--json", "config", "import-taskrc", path, "--dry-run")
	if !strings.Contains(dry, `"dry_run": true`) || !strings.Contains(dry, `"unknown.value"`) {
		t.Fatalf("dry-run output = %q", dry)
	}
	if got := run(t, bin, "--db", db, "_udas"); strings.TrimSpace(got) != "" {
		t.Fatalf("_udas after dry-run = %q", got)
	}
	out := run(t, bin, "--db", db, "config", "import-taskrc", path)
	if !strings.Contains(out, "imported:") || !strings.Contains(out, "skipped:") || !strings.Contains(out, "unknown:") {
		t.Fatalf("import-taskrc output = %q", out)
	}
	if got := strings.TrimSpace(run(t, bin, "--db", db, "config", "get", "uda.estimate.values")); got != "1,2,3" {
		t.Fatalf("uda values after import = %q", got)
	}
	run(t, bin, "--db", db, "project", "add", "work", "name:Work")
	run(t, bin, "--db", db, "add", "work", "task", "project:work", "estimate:2")
	run(t, bin, "--db", db, "context", "use", "work")
	if got := run(t, bin, "--db", db, "list"); !strings.Contains(got, "work task") {
		t.Fatalf("context imported filter did not work: %q", got)
	}
}

func TestCLIShowHelperVersionAndCompletion(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "config", "set", "date.format", "epoch")
	show := run(t, bin, "--db", db, "_show", "date.format", "database.path")
	lines := strings.Split(strings.TrimSpace(show), "\n")
	if len(lines) != 2 || lines[0] != "epoch" || lines[1] != db {
		t.Fatalf("_show output = %q", show)
	}
	version := strings.TrimSpace(run(t, bin, "_version"))
	if !strings.HasPrefix(version, "xuanchu ") {
		t.Fatalf("_version output = %q", version)
	}
	badDB := filepath.Join(t.TempDir(), "missing-parent", "xuanchu.db")
	completion := run(t, bin, "--db", badDB, "completion", "bash")
	if !strings.Contains(completion, "complete") || !strings.Contains(completion, "xuanchu") {
		t.Fatalf("completion output = %q", completion)
	}
}

func TestCLICompletionDoesNotOpenDatabase(t *testing.T) {
	bin := buildXuanchu(t)
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			badDB := filepath.Join(t.TempDir(), "missing-parent", "xuanchu.db")
			out := run(t, bin, "--db", badDB, "completion", shell)
			if !strings.Contains(out, "xuanchu") {
				t.Fatalf("completion %s output = %q", shell, out)
			}
		})
	}
}

func TestCLICompletionRejectsUnsupportedShell(t *testing.T) {
	bin := buildXuanchu(t)
	badDB := filepath.Join(t.TempDir(), "missing-parent", "xuanchu.db")
	cmd := exec.Command(bin, "--db", badDB, "completion", "bogus")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("completion bogus error = nil, output = %q", out)
	}
	if !strings.Contains(string(out), `unsupported shell "bogus"`) {
		t.Fatalf("completion bogus output = %q", out)
	}
}

func TestCLIContextCommands(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "work", "name:Work")
	run(t, bin, "--db", db, "project", "add", "home", "name:Home")
	run(t, bin, "--db", db, "add", "work", "task", "project:work")
	run(t, bin, "--db", db, "add", "home", "task", "project:home")
	homeID := strings.TrimSpace(run(t, bin, "--db", db, "_ids", "project:home"))
	if homeID == "" {
		t.Fatal("_ids project:home returned empty result")
	}
	if show := run(t, bin, "--db", db, "context", "show"); strings.TrimSpace(show) != "" {
		t.Fatalf("empty context show = %q", show)
	}
	run(t, bin, "--db", db, "context", "define", "work", "project:work")
	run(t, bin, "--db", db, "context", "use", "work")
	show := run(t, bin, "--db", db, "context", "show")
	if !strings.Contains(show, "work") || !strings.Contains(show, "project:work") {
		t.Fatalf("context show = %q", show)
	}
	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "work task") || strings.Contains(list, "home task") {
		t.Fatalf("context list output = %q", list)
	}
	all := run(t, bin, "--db", db, "all")
	if !strings.Contains(all, "work task") || strings.Contains(all, "home task") {
		t.Fatalf("context all output = %q", all)
	}
	info := run(t, bin, "--db", db, "info", homeID)
	if !strings.Contains(info, "home task") {
		t.Fatalf("explicit target info should ignore context, output = %q", info)
	}
	bypassed := run(t, bin, "--db", db, "--no-context", "list")
	if !strings.Contains(bypassed, "work task") || !strings.Contains(bypassed, "home task") {
		t.Fatalf("--no-context list output = %q", bypassed)
	}
	run(t, bin, "--db", db, "context", "delete", "work")
	if show := run(t, bin, "--db", db, "context", "show"); strings.TrimSpace(show) != "" {
		t.Fatalf("context show after delete = %q", show)
	}
}

func TestCLIExportImportRoundTrip(t *testing.T) {
	bin := buildXuanchu(t)
	db1 := filepath.Join(t.TempDir(), "one.db")
	db2 := filepath.Join(t.TempDir(), "two.db")
	run(t, bin, "--db", db1, "add", "write", "spec", "+planning")
	exported := run(t, bin, "--db", db1, "export")
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, []byte(exported), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, bin, "--db", db2, "import", path)
	out := run(t, bin, "--db", db2, "list")
	if !strings.Contains(out, "write spec") {
		t.Fatalf("list output = %q", out)
	}
}

func TestCLIUserWorkspaceLifecycle(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "user", "add", "alice", "email:alice@example.test")
	users := run(t, bin, "--db", db, "user", "list")
	if !strings.Contains(users, "alice") {
		t.Fatalf("user list output = %q", users)
	}
	run(t, bin, "--db", db, "user", "use", "alice")

	workspaces := run(t, bin, "--db", db, "workspace", "list")
	if !strings.Contains(workspaces, "alice") {
		t.Fatalf("workspace list output = %q", workspaces)
	}
	run(t, bin, "--db", db, "workspace", "add", "work", "name:Work", "visibility:team")
	run(t, bin, "--db", db, "workspace", "use", "work")
	workspaces = run(t, bin, "--db", db, "workspace", "list")
	if !strings.Contains(workspaces, "work") {
		t.Fatalf("workspace list missing work = %q", workspaces)
	}
}

func TestCLIWorkspaceErrorSemantics(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	cmd := exec.Command(bin, "--db", db, "--workspace", "nonexistent", "list")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "workspace_not_found") {
		t.Fatalf("nonexistent workspace error = %v, output = %q", err, out)
	}

	run(t, bin, "--db", db, "workspace", "add", "old")
	run(t, bin, "--db", db, "workspace", "add", "other")
	run(t, bin, "--db", db, "workspace", "archive", "old")
	cmd = exec.Command(bin, "--db", db, "--workspace", "old", "list")
	out, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "workspace_archived") {
		t.Fatalf("archived workspace error = %v, output = %q", err, out)
	}

	run(t, bin, "--db", db, "user", "add", "alice")
	run(t, bin, "--db", db, "user", "use", "alice")
	cmd = exec.Command(bin, "--db", db, "--workspace", "local", "list")
	out, err = cmd.CombinedOutput()
	if err == nil || (!strings.Contains(string(out), "membership_not_found") && !strings.Contains(string(out), "permission_denied")) {
		t.Fatalf("non-member workspace error = %v, output = %q", err, out)
	}

	cmd = exec.Command(bin, "--db", db, "--json", "--workspace", "nonexistent", "list")
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("--json nonexistent workspace error = nil, output = %q", out)
	}
	var payload map[string]any
	if json.Unmarshal(out, &payload) != nil || payload["code"] != "workspace_not_found" {
		t.Fatalf("--json error output = %q", out)
	}
}

func TestCLIConfigAndShowHideLegacyContextKeys(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "one")
	run(t, bin, "--db", db, "context", "define", "work", "title:one")
	run(t, bin, "--db", db, "context", "use", "work")

	cmd := exec.Command(bin, "--db", db, "_show", "context.active")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "unsupported legacy key") {
		t.Fatalf("_show context.active error = %v, output = %q", err, out)
	}
	list := run(t, bin, "--db", db, "config", "list")
	for _, forbidden := range []string{"context.active=", "active_user_id=", "active_context."} {
		if strings.Contains(list, forbidden) {
			t.Fatalf("config list output = %q, should not contain %q", list, forbidden)
		}
	}
}

func TestCLIMemberPermissions(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "user", "add", "bob")
	run(t, bin, "--db", db, "workspace", "add", "team")
	run(t, bin, "--db", db, "workspace", "use", "team")
	run(t, bin, "--db", db, "member", "add", "bob", "role:viewer")

	run(t, bin, "--db", db, "user", "use", "bob")
	out := run(t, bin, "--db", db, "--workspace", "team", "member", "list")
	if !strings.Contains(out, "bob") {
		t.Fatalf("member list output = %q", out)
	}
	cmd := exec.Command(bin, "--db", db, "--workspace", "team", "add", "viewer", "task")
	raw, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(raw), "permission_denied") {
		t.Fatalf("viewer add task error = %v, output = %q", err, raw)
	}

	run(t, bin, "--db", db, "user", "use", "local")
	run(t, bin, "--db", db, "--workspace", "team", "member", "role", "bob", "member")
	run(t, bin, "--db", db, "user", "use", "bob")
	cmd = exec.Command(bin, "--db", db, "--workspace", "team", "member", "add", "local")
	raw, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(raw), "permission_denied") {
		t.Fatalf("member manage members error = %v, output = %q", err, raw)
	}

	run(t, bin, "--db", db, "user", "use", "local")
	run(t, bin, "--db", db, "user", "add", "admin")
	run(t, bin, "--db", db, "--workspace", "team", "member", "add", "admin", "role:admin")
	run(t, bin, "--db", db, "user", "use", "admin")
	cmd = exec.Command(bin, "--db", db, "--workspace", "team", "workspace", "archive", "team")
	raw, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(raw), "permission_denied") {
		t.Fatalf("admin archive workspace error = %v, output = %q", err, raw)
	}

	run(t, bin, "--db", db, "user", "use", "local")
	run(t, bin, "--db", db, "workspace", "add", "backup")
	run(t, bin, "--db", db, "workspace", "archive", "team")
	list := run(t, bin, "--db", db, "workspace", "list", "--all")
	if !strings.Contains(list, "team") || !strings.Contains(list, "archived") {
		t.Fatalf("workspace list --all output = %q", list)
	}
}

func TestCLIAuditList(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "workspace", "add", "team")
	run(t, bin, "--db", db, "workspace", "use", "team")
	run(t, bin, "--db", db, "project", "add", "api", "name:API")
	run(t, bin, "--db", db, "add", "write", "spec")
	run(t, bin, "--db", db, "1", "modify", "project:api")
	run(t, bin, "--db", db, "user", "add", "alice")
	run(t, bin, "--db", db, "member", "add", "alice", "role:viewer")
	run(t, bin, "--db", db, "workspace", "modify", "team", "description:Team")

	out := run(t, bin, "--db", db, "--json", "audit", "list")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("audit list --json output is not JSON: %v\n%s", err, out)
	}
	actions := map[string]bool{}
	for _, row := range rows {
		if action, _ := row["action"].(string); action != "" {
			actions[action] = true
		}
	}
	for _, want := range []string{"task.add", "member.add", "workspace.modify"} {
		if !actions[want] {
			t.Fatalf("audit actions = %#v, missing %q", actions, want)
		}
	}

	human := run(t, bin, "--db", db, "audit", "list")
	if !strings.Contains(human, "local") {
		t.Fatalf("audit list output = %q, want actor name", human)
	}

	filtered := run(t, bin, "--db", db, "--json", "audit", "list", "--project", "api")
	var projectRows []map[string]any
	if err := json.Unmarshal([]byte(filtered), &projectRows); err != nil {
		t.Fatalf("audit list --project --json output is not JSON: %v\n%s", err, filtered)
	}
	if len(projectRows) == 0 {
		t.Fatalf("audit list --project returned no rows: %q", filtered)
	}
	for _, row := range projectRows {
		if row["project_id"] == nil {
			t.Fatalf("audit row missing project_id: %#v", row)
		}
	}
}

func TestCLIM5MigrationWarning(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	seedM4DatabaseWithTasksForCLI(t, db, []seedTask{
		{WorkspaceSlug: "local", UUID: "t1", Project: strptr("Good"), Entry: 10},
		{WorkspaceSlug: "local", UUID: "t2", Project: strptr("Bad Name"), Entry: 20},
	})

	cmd := exec.Command(bin, "--db", db, "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list after M5 migration error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "warning: M5 project migration skipped some legacy project strings") {
		t.Fatalf("migration warning output = %q", out)
	}
	report := run(t, bin, "--db", db, "config", "get", "migration.m5.projects.skipped")
	if !strings.Contains(report, "t2") || !strings.Contains(report, "invalid_slug") {
		t.Fatalf("migration report = %q", report)
	}
}

func TestCLIWorkspaceIsolation(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "config", "set", "uda.estimate.type", "numeric")
	run(t, bin, "--db", db, "project", "add", "same", "name:Same")
	run(t, bin, "--db", db, "add", "local", "task", "project:same", "+same", "estimate:1")
	run(t, bin, "--db", db, "context", "define", "same", "project:same")
	run(t, bin, "--db", db, "context", "use", "same")

	run(t, bin, "--db", db, "workspace", "add", "work")
	run(t, bin, "--db", db, "workspace", "use", "work")
	run(t, bin, "--db", db, "config", "set", "uda.estimate.type", "numeric")
	run(t, bin, "--db", db, "project", "add", "same", "name:Same")
	run(t, bin, "--db", db, "add", "work", "task", "project:same", "+same", "estimate:2")
	run(t, bin, "--db", db, "context", "define", "same", "project:same")
	run(t, bin, "--db", db, "context", "use", "same")

	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "work task") || strings.Contains(list, "local task") {
		t.Fatalf("work list output = %q", list)
	}
	localList := run(t, bin, "--db", db, "--workspace", "local", "list")
	if !strings.Contains(localList, "local task") || strings.Contains(localList, "work task") {
		t.Fatalf("local list output = %q", localList)
	}

	for _, tc := range []struct {
		name      string
		args      []string
		wantWork  string
		wantLocal string
	}{
		{name: "projects", args: []string{"_projects"}, wantWork: "same", wantLocal: "same"},
		{name: "tags", args: []string{"_tags"}, wantWork: "same", wantLocal: "same"},
		{name: "udas", args: []string{"_udas"}, wantWork: "estimate", wantLocal: "estimate"},
		{name: "unique", args: []string{"_unique", "estimate"}, wantWork: "2", wantLocal: "1"},
		{name: "ids", args: []string{"_ids", "project:same"}, wantWork: "1", wantLocal: "1"},
		{name: "uuids", args: []string{"_uuids", "project:same"}, wantWork: "", wantLocal: ""},
		{name: "get", args: []string{"_get", "1.title"}, wantWork: "work task", wantLocal: "local task"},
		{name: "urgency", args: []string{"_urgency", "1"}, wantWork: "", wantLocal: ""},
	} {
		workOut := run(t, bin, append([]string{"--db", db}, tc.args...)...)
		localOut := run(t, bin, append([]string{"--db", db, "--workspace", "local"}, tc.args...)...)
		if tc.wantWork != "" && !strings.Contains(workOut, tc.wantWork) {
			t.Fatalf("%s work output = %q, want %q", tc.name, workOut, tc.wantWork)
		}
		if tc.wantLocal != "" && !strings.Contains(localOut, tc.wantLocal) {
			t.Fatalf("%s local output = %q, want %q", tc.name, localOut, tc.wantLocal)
		}
		if tc.name == "uuids" && strings.TrimSpace(workOut) == strings.TrimSpace(localOut) {
			t.Fatalf("uuids output should differ by workspace: work=%q local=%q", workOut, localOut)
		}
		if tc.name == "urgency" && strings.TrimSpace(workOut) == "" {
			t.Fatalf("urgency work output = %q", workOut)
		}
	}
}

func TestCLIModifyDoneDelete(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "write", "spec")
	run(t, bin, "--db", db, "1", "modify", "priority:H", "+next")
	out := run(t, bin, "--db", db, "list")
	if !strings.Contains(out, "H") || !strings.Contains(out, "next") {
		t.Fatalf("list output = %q", out)
	}
	run(t, bin, "--db", db, "1", "done")
	out = run(t, bin, "--db", db, "list")
	if strings.Contains(out, "write spec") {
		t.Fatalf("done task still in default list: %q", out)
	}
}

func TestCLIJSONFlagProducesMachineReadableOutput(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	addOut := run(t, bin, "--db", db, "--json", "add", "write", "spec")
	var created map[string]any
	if err := json.Unmarshal([]byte(addOut), &created); err != nil {
		t.Fatalf("add --json output is not JSON: %v\n%s", err, addOut)
	}
	if created["title"] != "write spec" {
		t.Fatalf("created title = %#v", created["title"])
	}

	listOut := run(t, bin, "--db", db, "--json", "list")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list --json output is not JSON: %v\n%s", err, listOut)
	}
	if len(listed) != 1 || listed[0]["title"] != "write spec" {
		t.Fatalf("listed = %#v", listed)
	}
}

func TestCLIShowUsesParsedDBFlag(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "custom.db")

	out := run(t, bin, "--db", db, "show")
	if !strings.Contains(out, "database.path="+db) {
		t.Fatalf("show output = %q, want database.path=%s", out, db)
	}
}

func TestCLIPrefixFiltersAndTargetFilters(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "work", "task", "+work")
	run(t, bin, "--db", db, "add", "home", "task", "+home")

	workOut := run(t, bin, "--db", db, "+work", "list")
	if !strings.Contains(workOut, "work task") || strings.Contains(workOut, "home task") {
		t.Fatalf("+work list output = %q", workOut)
	}

	firstOut := run(t, bin, "--db", db, "1", "list")
	if !strings.Contains(firstOut, "work task") || strings.Contains(firstOut, "home task") {
		t.Fatalf("1 list output = %q", firstOut)
	}

	exported := run(t, bin, "--db", db, "export")
	var tasks []map[string]any
	if err := json.Unmarshal([]byte(exported), &tasks); err != nil {
		t.Fatalf("export output is not JSON: %v\n%s", err, exported)
	}
	uuid, _ := tasks[1]["uuid"].(string)
	if uuid == "" {
		t.Fatalf("exported tasks missing uuid: %#v", tasks)
	}
	uuidOut := run(t, bin, "--db", db, uuid, "list")
	if !strings.Contains(uuidOut, "home task") || strings.Contains(uuidOut, "work task") {
		t.Fatalf("uuid list output = %q", uuidOut)
	}
}

func TestCLINextCommandSortsByUrgency(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "future", "task", "due:2099-01-01")
	run(t, bin, "--db", db, "add", "urgent", "task", "+next", "priority:H")

	out := run(t, bin, "--db", db, "next")
	urgent := strings.Index(out, "urgent task")
	future := strings.Index(out, "future task")
	if urgent < 0 || future < 0 || urgent > future {
		t.Fatalf("next output order = %q", out)
	}
}

func TestCLIAcceptsDashTagAsModificationNotFlag(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "write", "spec", "-ignored")
	out := run(t, bin, "--db", db, "list")
	if strings.Contains(out, "ignored") {
		t.Fatalf("add -tag should not add tag, output = %q", out)
	}

	run(t, bin, "--db", db, "1", "modify", "+old")
	run(t, bin, "--db", db, "1", "modify", "-old")
	out = run(t, bin, "--db", db, "list")
	if strings.Contains(out, "old") {
		t.Fatalf("modify -tag should remove tag, output = %q", out)
	}
}

func TestCLIInfoShowsAllM0Fields(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "xuanchu", "name:Xuanchu")
	run(t, bin, "--db", db, "add", "write", "spec", "project:xuanchu", "priority:H", "due:2030-01-01", "+planning")
	out := run(t, bin, "--db", db, "info", "1")
	for _, want := range []string{"UUID:", "Status:", "Title:", "Description:", "Entry:", "Modified:", "Due:", "Project:", "Priority:", "Tags:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("info output missing %q: %q", want, out)
		}
	}
}

func TestCLIReportsAndUrgency(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "normal", "task")
	run(t, bin, "--db", db, "add", "next", "task", "+next", "priority:H")
	nextUUID := strings.TrimSpace(run(t, bin, "--db", db, "_urgency", "2"))
	nextUUID = strings.TrimSpace(nextUUID)
	// We just need to confirm urgency outputs a number; the UUID capture is from _uuids below
	uuids := strings.Split(strings.TrimSpace(run(t, bin, "--db", db, "--json", "export")), "\n")
	_ = uuids // keep reference
	_ = nextUUID
	run(t, bin, "--db", db, "1", "done")

	all := run(t, bin, "--db", db, "all")
	if !strings.Contains(all, "normal task") || !strings.Contains(all, "next task") {
		t.Fatalf("all output = %q", all)
	}
	completed := run(t, bin, "--db", db, "completed")
	if !strings.Contains(completed, "normal task") {
		t.Fatalf("completed output = %q", completed)
	}

	// Test urgency on the next task (which should still be ID 1 after completing the other)
	urg := run(t, bin, "--db", db, "urgency", "1")
	if !strings.Contains(urg, "tag.next") || !strings.Contains(urg, "priority.H") {
		t.Fatalf("urgency output = %q", urg)
	}

	// Test conflicting report filter
	conflict := run(t, bin, "--db", db, "completed", "status:pending")
	if strings.Contains(conflict, "normal task") || strings.Contains(conflict, "next task") {
		t.Fatalf("conflicting report filter output = %q", conflict)
	}
}

func TestCLIHelpers(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "project", "add", "work", "name:Work")
	run(t, bin, "--db", db, "project", "add", "home", "name:Home")
	run(t, bin, "--db", db, "add", "work", "task", "project:work", "+next")
	run(t, bin, "--db", db, "add", "home", "task", "project:home", "+later")

	nextID := strings.TrimSpace(run(t, bin, "--db", db, "_ids", "+next"))
	if nextID == "" {
		t.Fatal("_ids +next returned empty result")
	}
	got := run(t, bin, "--db", db, "_get", nextID+".title", nextID+".tag.next", nextID+".tag.missing", nextID+".urgency")
	if !strings.Contains(got, "work task") || !strings.Contains(got, "next") {
		t.Fatalf("_get output = %q", got)
	}
	ids := run(t, bin, "--db", db, "_ids", "+next")
	if strings.TrimSpace(ids) != nextID {
		t.Fatalf("_ids output = %q", ids)
	}
	projects := run(t, bin, "--db", db, "_projects")
	if !strings.Contains(projects, "home") || !strings.Contains(projects, "work") {
		t.Fatalf("_projects output = %q", projects)
	}
	run(t, bin, "--db", db, "project", "add", "legacy", "name:Legacy")
	run(t, bin, "--db", db, "project", "archive", "legacy")
	allProjects := run(t, bin, "--db", db, "_projects", "--all")
	if !strings.Contains(allProjects, "legacy") || !strings.Contains(allProjects, "home") || !strings.Contains(allProjects, "work") {
		t.Fatalf("_projects --all output = %q", allProjects)
	}
	uniqueProjects := run(t, bin, "--db", db, "_unique", "project")
	if !strings.Contains(uniqueProjects, "home") || !strings.Contains(uniqueProjects, "work") || strings.Contains(uniqueProjects, "legacy") {
		t.Fatalf("_unique project output = %q", uniqueProjects)
	}
	tags := run(t, bin, "--db", db, "_tags")
	if !strings.Contains(tags, "next") || !strings.Contains(tags, "later") {
		t.Fatalf("_tags output = %q", tags)
	}
	emptyDB := filepath.Join(t.TempDir(), "empty.db")
	if projects := run(t, bin, "--db", emptyDB, "_projects"); strings.TrimSpace(projects) != "" {
		t.Fatalf("empty _projects output = %q", projects)
	}
	if tags := run(t, bin, "--db", emptyDB, "_tags"); strings.TrimSpace(tags) != "" {
		t.Fatalf("empty _tags output = %q", tags)
	}
}

func TestCLICalc(t *testing.T) {
	bin := buildXuanchu(t)
	out := run(t, bin, "calc", "1 + 2 * 3")
	if strings.TrimSpace(out) != "7" {
		t.Fatalf("calc output = %q", out)
	}
	out = run(t, bin, "calc", "3 > 2 and 1 < 2")
	if strings.TrimSpace(out) != "true" {
		t.Fatalf("calc bool output = %q", out)
	}
}

func TestCLIM1QueryExamples(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "project", "add", "work", "name:Work")
	run(t, bin, "--db", db, "add", "urgent", "work", "task", "project:work", "+urgent", "priority:H")
	run(t, bin, "--db", db, "add", "later", "task", "+later")
	run(t, bin, "--db", db, "add", "next", "task", "+next", "due:2030-01-01")

	out := run(t, bin, "--db", db, "(project:work and +urgent) or priority:H", "list")
	if !strings.Contains(out, "urgent work task") || strings.Contains(out, "later task") {
		t.Fatalf("complex query output = %q", out)
	}
	out = run(t, bin, "--db", db, "+next", "list")
	if !strings.Contains(out, "next task") {
		t.Fatalf("+next list output = %q", out)
	}
	out = run(t, bin, "--db", db, "/later/", "list")
	if !strings.Contains(out, "later task") || strings.Contains(out, "urgent") {
		t.Fatalf("/later/ list output = %q", out)
	}
}

func TestCLIListQueryKeepsDefaultPendingFilter(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "alpha", "+work")
	run(t, bin, "--db", db, "add", "beta", "+work")
	run(t, bin, "--db", db, "1", "done")

	out := run(t, bin, "--db", db, "+work", "list")
	if strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("+work list output = %q", out)
	}
}

func TestCLIIDsReturnDefaultWorkingSetIDs(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "first")
	run(t, bin, "--db", db, "add", "second", "+next")

	ids := run(t, bin, "--db", db, "_ids", "+next")
	if strings.TrimSpace(ids) != "2" {
		t.Fatalf("_ids output = %q, want 2", ids)
	}
	get := run(t, bin, "--db", db, "_get", strings.TrimSpace(ids)+".title")
	if strings.TrimSpace(get) != "second" {
		t.Fatalf("_get for _ids output = %q, want second", get)
	}
}

func TestCLIIDsOnlyReturnDefaultWorkingSetIDsForCompletedQueries(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "first")
	run(t, bin, "--db", db, "add", "second")
	run(t, bin, "--db", db, "2", "done")

	ids := run(t, bin, "--db", db, "_ids", "status:completed")
	if strings.TrimSpace(ids) != "" {
		t.Fatalf("_ids completed output = %q, want empty", ids)
	}
	uuids := run(t, bin, "--db", db, "_uuids", "status:completed")
	if strings.TrimSpace(uuids) == "" {
		t.Fatalf("_uuids completed output = %q, want completed uuid", uuids)
	}
}

func TestCLIDueStoredAsEndOfDay(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "deadline", "due:2030-01-01")
	exported := run(t, bin, "--db", db, "--json", "export")
	var tasks []map[string]any
	if err := json.Unmarshal([]byte(exported), &tasks); err != nil {
		t.Fatalf("export not JSON: %v\n%s", err, exported)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v", tasks)
	}
	due, _ := tasks[0]["due"].(string)
	parsed, err := time.Parse(time.RFC3339, due)
	if err != nil {
		t.Fatalf("due %q not RFC3339: %v", due, err)
	}
	local := parsed.In(time.Local)
	if local.Year() != 2030 || local.Month() != time.January || local.Day() != 1 {
		t.Fatalf("due local date = %v, want 2030-01-01", local)
	}
	if local.Hour() != 23 || local.Minute() != 59 || local.Second() != 59 {
		t.Fatalf("due local time = %v, want 23:59:59", local)
	}
}

func TestCLITitleAttributeUsesSubstring(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "write", "spec")
	run(t, bin, "--db", db, "add", "other", "task")

	out := run(t, bin, "--db", db, "title:spec", "list")
	if !strings.Contains(out, "write spec") {
		t.Fatalf("title:spec list missing write spec: %q", out)
	}
	if strings.Contains(out, "other task") {
		t.Fatalf("title:spec list should not match other task: %q", out)
	}
}

func TestCLIM1QueryOverdueReport(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "old", "task", "due:2020-01-01")
	run(t, bin, "--db", db, "add", "future", "task", "due:2099-01-01")

	out := run(t, bin, "--db", db, "overdue")
	if !strings.Contains(out, "old task") {
		t.Fatalf("overdue output = %q", out)
	}
	if strings.Contains(out, "future task") {
		t.Fatalf("overdue should not include future tasks: %q", out)
	}
}

func TestCLIM2AddModifyFields(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "waiting task", "wait:tomorrow", "scheduled:eow", "until:eom")
	waiting := run(t, bin, "--db", db, "waiting")
	if !strings.Contains(waiting, "waiting task") {
		t.Fatalf("waiting output = %q", waiting)
	}
	run(t, bin, "--db", db, "1", "modify", "wait:")
	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "waiting task") {
		t.Fatalf("list output = %q", list)
	}
}

func TestCLIStartStopActive(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "active task")
	run(t, bin, "--db", db, "1", "start")
	active := run(t, bin, "--db", db, "active")
	if !strings.Contains(active, "active task") {
		t.Fatalf("active output = %q", active)
	}
	run(t, bin, "--db", db, "1", "stop")
	active = run(t, bin, "--db", db, "active")
	if strings.Contains(active, "active task") {
		t.Fatalf("stopped task still active: %q", active)
	}
}

func TestCLIAnnotateDenotate(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "annotated task")
	run(t, bin, "--db", db, "1", "annotate", "first note")
	exported := run(t, bin, "--db", db, "--json", "export")
	var tasks []struct {
		Annotations []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"annotations"`
	}
	if err := json.Unmarshal([]byte(exported), &tasks); err != nil {
		t.Fatalf("export JSON = %q: %v", exported, err)
	}
	if len(tasks) != 1 || len(tasks[0].Annotations) != 1 || tasks[0].Annotations[0].Description != "first note" {
		t.Fatalf("exported tasks = %#v, want one annotation", tasks)
	}
	annotation := tasks[0].Annotations[0]
	if annotation.ID == "" {
		t.Fatalf("annotation id is empty: %#v", annotation)
	}
	got := run(t, bin, "--db", db, "_get", "1.annotations")
	if !strings.Contains(got, annotation.ID) || !strings.Contains(got, "first note") {
		t.Fatalf("annotations output = %q, want id %q and note", got, annotation.ID)
	}
	run(t, bin, "--db", db, "1", "denotate", annotation.ID)
	exported = run(t, bin, "--db", db, "--json", "export")
	tasks = nil
	if err := json.Unmarshal([]byte(exported), &tasks); err != nil {
		t.Fatalf("export JSON after denotate = %q: %v", exported, err)
	}
	if len(tasks) != 1 || len(tasks[0].Annotations) != 0 {
		t.Fatalf("exported tasks after denotate = %#v, want no annotations", tasks)
	}
}

func TestCLIM2Reports(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "blocker")
	blockerUUID := strings.TrimSpace(run(t, bin, "--db", db, "_uuids", "/blocker/"))
	run(t, bin, "--db", db, "add", "blocked", "depends:"+blockerUUID)
	run(t, bin, "--db", db, "add", "waiting", "wait:tomorrow")
	run(t, bin, "--db", db, "add", "ready", "scheduled:2020-01-01")

	for name, want := range map[string]string{
		"blocked":  "blocked",
		"blocking": "blocker",
		"waiting":  "waiting",
		"ready":    "ready",
	} {
		out := run(t, bin, "--db", db, name)
		if !strings.Contains(out, want) {
			t.Fatalf("%s output = %q, want %q", name, out, want)
		}
	}
}

func TestCLIAddDependsAcceptsWorkingSetID(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "blocker")
	run(t, bin, "--db", db, "add", "blocked", "depends:1")
	blocked := run(t, bin, "--db", db, "blocked")
	if !strings.Contains(blocked, "blocked") {
		t.Fatalf("blocked output = %q", blocked)
	}
	blocking := run(t, bin, "--db", db, "blocking")
	if !strings.Contains(blocking, "blocker") {
		t.Fatalf("blocking output = %q", blocking)
	}
}

func TestCLIListShowsWorkingSetIDWhenWaitingTaskIsHidden(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "hidden waiting", "wait:tomorrow")
	run(t, bin, "--db", db, "add", "visible task")
	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "2") || !strings.Contains(list, "visible task") {
		t.Fatalf("list output = %q, want visible task with working-set ID 2", list)
	}
	got := run(t, bin, "--db", db, "_get", "2.title")
	if strings.TrimSpace(got) != "visible task" {
		t.Fatalf("_get 2.title = %q, want visible task", got)
	}
}

func TestCLINextRowIDMatchesWorkingSetIDUnderUrgencySort(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "low priority task")
	run(t, bin, "--db", db, "add", "high priority task")
	run(t, bin, "--db", db, "2", "modify", "priority:H")
	out := run(t, bin, "--db", db, "next")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// lines[0]=header, lines[1]=separator, lines[2..]=data rows
	if len(lines) < 4 {
		t.Fatalf("next output = %q, want header + separator + 2 rows", out)
	}
	first := lines[2]
	second := lines[3]
	if !strings.Contains(first, "high priority task") {
		t.Fatalf("first row = %q, want high priority task first under urgency sort", first)
	}
	if !strings.HasPrefix(strings.TrimSpace(first), "2") {
		t.Fatalf("first row = %q, want working-set ID 2 for high priority task", first)
	}
	if !strings.Contains(second, "low priority task") || !strings.HasPrefix(strings.TrimSpace(second), "1") {
		t.Fatalf("second row = %q, want low priority task with ID 1", second)
	}
}

func TestCLICompletedReportShowsDashID(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "done it")
	run(t, bin, "--db", db, "1", "done")
	out := run(t, bin, "--db", db, "completed")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// lines[0]=header, lines[1]=separator, lines[2..]=data rows
	if len(lines) < 3 {
		t.Fatalf("completed output = %q, want header + separator + 1 row", out)
	}
	row := strings.TrimSpace(lines[2])
	if !strings.HasPrefix(row, "-") {
		t.Fatalf("completed row = %q, want '-' as ID for non-working-set task", row)
	}
}

func TestCLIDOMUrgencyIncludesDependencyState(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "blocker")
	run(t, bin, "--db", db, "add", "blocked")
	run(t, bin, "--db", db, "2", "modify", "depends:1")
	getUrgency := strings.TrimSpace(run(t, bin, "--db", db, "_get", "2.urgency"))
	helperUrgency := strings.TrimSpace(run(t, bin, "--db", db, "_urgency", "2"))
	if getUrgency != helperUrgency {
		t.Fatalf("_get urgency = %q, _urgency = %q", getUrgency, helperUrgency)
	}
}

func TestCLIAppendPrepend(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "middle")
	run(t, bin, "--db", db, "1", "append", "end")
	run(t, bin, "--db", db, "1", "prepend", "start")
	got := run(t, bin, "--db", db, "_get", "1.title")
	if !strings.Contains(got, "start middle end") {
		t.Fatalf("title = %q", got)
	}
}

func TestCLIAppendPrependCommands(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "middle")
	run(t, bin, "--db", db, "append", "1", "tail", "text")
	run(t, bin, "--db", db, "prepend", "1", "head", "text")
	got := run(t, bin, "--db", db, "_get", "1.title")
	if !strings.Contains(got, "head text middle tail text") {
		t.Fatalf("title = %q", got)
	}
}

func TestCLIEditRejectsInvalidDate(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	editor := buildEditorHelper(t, `package main
import (
	"encoding/json"
	"os"
)
func main() {
	path := os.Args[1]
	data, err := os.ReadFile(path)
	if err != nil { panic(err) }
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil { panic(err) }
	doc["due"] = "not-a-date"
	data, err = json.Marshal(doc)
	if err != nil { panic(err) }
	if err := os.WriteFile(path, data, 0o600); err != nil { panic(err) }
}`)
	run(t, bin, "--db", db, "add", "editable", "due:2030-01-01")
	before := strings.TrimSpace(run(t, bin, "--db", db, "_get", "1.due"))
	cmd := exec.Command(bin, "--db", db, "1", "edit")
	cmd.Env = append(os.Environ(), "EDITOR="+editor)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("edit succeeded unexpectedly:\n%s", out)
	}
	after := strings.TrimSpace(run(t, bin, "--db", db, "_get", "1.due"))
	if after != before {
		t.Fatalf("due changed after invalid edit: before=%q after=%q", before, after)
	}
}

func TestCLIEditWithTestEditor(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	editor := buildEditorHelper(t, `package main
import (
	"encoding/json"
	"os"
)
func main() {
	path := os.Args[1]
	data, err := os.ReadFile(path)
	if err != nil { panic(err) }
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil { panic(err) }
	doc["title"] = "edited task"
	data, err = json.Marshal(doc)
	if err != nil { panic(err) }
	if err := os.WriteFile(path, data, 0o600); err != nil { panic(err) }
}`)
	run(t, bin, "--db", db, "add", "original task")
	cmd := exec.Command(bin, "--db", db, "1", "edit")
	cmd.Env = append(os.Environ(), "EDITOR="+editor)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("edit failed: %v\n%s", err, out)
	}
	got := run(t, bin, "--db", db, "_get", "1.title")
	if !strings.Contains(got, "edited task") {
		t.Fatalf("title = %q", got)
	}
}

func TestCLIRecurringDaily(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "daily task", "recur:daily", "due:2030-01-01", "until:2030-01-05")
	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "daily task") {
		t.Fatalf("list output = %q", list)
	}
	run(t, bin, "--db", db, "1", "done")
	list = run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "daily task") {
		t.Fatalf("next recurring child missing: %q", list)
	}
}

func TestCLIRecurringExportImport(t *testing.T) {
	bin := buildXuanchu(t)
	db1 := filepath.Join(t.TempDir(), "one.db")
	db2 := filepath.Join(t.TempDir(), "two.db")
	run(t, bin, "--db", db1, "add", "daily task", "recur:daily", "due:2030-01-01", "until:2030-01-05")
	exported := run(t, bin, "--db", db1, "export")
	path := filepath.Join(t.TempDir(), "recurring.json")
	if err := os.WriteFile(path, []byte(exported), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, bin, "--db", db2, "import", path)
	out := run(t, bin, "--db", db2, "export")
	if !strings.Contains(out, `"recur": "daily"`) {
		t.Fatalf("export output missing recur: %q", out)
	}
	if !strings.Contains(out, `"parent":`) {
		t.Fatalf("export output missing parent linkage: %q", out)
	}
}

func TestCLIProjectLifecycle(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	addOut := run(t, bin, "--db", db, "project", "add", "apiplat", "name:AI Agent Platform")
	if !strings.Contains(addOut, "Created project apiplat") {
		t.Fatalf("project add output = %q", addOut)
	}
	listOut := run(t, bin, "--db", db, "project", "list")
	if !strings.Contains(listOut, "apiplat") {
		t.Fatalf("project list output = %q", listOut)
	}
	infoJSON := run(t, bin, "--db", db, "--json", "project", "info", "apiplat")
	var info map[string]any
	if err := json.Unmarshal([]byte(infoJSON), &info); err != nil {
		t.Fatalf("json.Unmarshal(project info) error = %v", err)
	}
	if info["slug"] != "apiplat" || info["name"] != "AI Agent Platform" {
		t.Fatalf("project info --json output = %#v", info)
	}
	modOut := run(t, bin, "--db", db, "project", "modify", "apiplat", "description:Agent MCP platform")
	if !strings.Contains(modOut, "Modified project apiplat") {
		t.Fatalf("project modify output = %q", modOut)
	}
	run(t, bin, "--db", db, "add", "Design schema", "project:apiplat")
	archiveOut := run(t, bin, "--db", db, "project", "archive", "apiplat")
	if !strings.Contains(archiveOut, "Archived project apiplat") || !strings.Contains(archiveOut, "warning: archived project apiplat still has 1 non-deleted task") {
		t.Fatalf("project archive output = %q", archiveOut)
	}
	archiveJSON := run(t, bin, "--db", db, "--json", "project", "info", "apiplat")
	var archivedInfo map[string]any
	if err := json.Unmarshal([]byte(archiveJSON), &archivedInfo); err != nil {
		t.Fatalf("json.Unmarshal(archived project info) error = %v", err)
	}
	if archivedInfo["task_count"] != float64(1) {
		t.Fatalf("archived project task_count = %#v, want 1", archivedInfo["task_count"])
	}
	run(t, bin, "--db", db, "project", "add", "jsonarch", "name:JSON Archive")
	run(t, bin, "--db", db, "add", "JSON archive task", "project:jsonarch")
	archiveJSONOut := run(t, bin, "--db", db, "--json", "project", "archive", "jsonarch")
	var archivedArchive map[string]any
	if err := json.Unmarshal([]byte(archiveJSONOut), &archivedArchive); err != nil {
		t.Fatalf("json.Unmarshal(project archive --json) error = %v; output = %q", err, archiveJSONOut)
	}
	if archivedArchive["slug"] != "jsonarch" || archivedArchive["task_count"] != float64(1) || archivedArchive["status"] != "archived" {
		t.Fatalf("project archive --json output = %#v", archivedArchive)
	}
	if _, err := runErr(t, bin, "--db", db, "add", "Should fail", "project:apiplat"); err == nil {
		t.Fatal("add with archived project error = nil, want failure")
	}
	if _, err := runErr(t, bin, "--db", db, "project", "archive", "apiplat"); err == nil {
		t.Fatal("project archive twice error = nil, want failure")
	}
}

func TestCLIProjectTransition(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "crowdfund", "name:众筹产品")
	// 新建默认 planning
	infoJSON := run(t, bin, "--db", db, "--json", "project", "info", "crowdfund")
	var info map[string]any
	if err := json.Unmarshal([]byte(infoJSON), &info); err != nil {
		t.Fatalf("json.Unmarshal(info) error = %v", err)
	}
	if info["status"] != "planning" {
		t.Fatalf("new project status = %#v, want planning", info["status"])
	}

	// list 默认 open 含 planning
	listOut := run(t, bin, "--db", db, "project", "list")
	if !strings.Contains(listOut, "crowdfund") {
		t.Fatalf("project list (open) = %q, want to contain planning project", listOut)
	}

	// planning -> active
	transOut := run(t, bin, "--db", db, "project", "transition", "crowdfund", "active")
	if !strings.Contains(transOut, "Transitioned project crowdfund to active") {
		t.Fatalf("transition output = %q", transOut)
	}
	// active -> archived
	run(t, bin, "--db", db, "project", "transition", "crowdfund", "archived")
	// archived -> active (重新激活)
	run(t, bin, "--db", db, "project", "transition", "crowdfund", "active")
	// active -> cancelled
	run(t, bin, "--db", db, "project", "transition", "crowdfund", "cancelled")
	infoJSON = run(t, bin, "--db", db, "--json", "project", "info", "crowdfund")
	var cancelledInfo map[string]any
	if err := json.Unmarshal([]byte(infoJSON), &cancelledInfo); err != nil {
		t.Fatalf("json.Unmarshal(cancelled) error = %v", err)
	}
	if cancelledInfo["status"] != "cancelled" {
		t.Fatalf("status = %#v, want cancelled", cancelledInfo["status"])
	}

	// cancelled 禁止 annotate
	if _, err := runErr(t, bin, "--db", db, "project", "annotate", "crowdfund", "should fail"); err == nil {
		t.Fatal("annotate cancelled project error = nil, want failure")
	}

	// --status 过滤：crowdfund 处于 cancelled
	run(t, bin, "--db", db, "project", "add", "other", "name:Other")
	cancelledList := run(t, bin, "--db", db, "project", "list", "--status", "cancelled")
	if !strings.Contains(cancelledList, "crowdfund") || strings.Contains(cancelledList, "other") {
		t.Fatalf("--status cancelled = %q", cancelledList)
	}
	openList := run(t, bin, "--db", db, "project", "list", "--status", "open")
	if !strings.Contains(openList, "other") || strings.Contains(openList, "crowdfund") {
		t.Fatalf("--status open = %q", openList)
	}

	// cancelled 可复活
	run(t, bin, "--db", db, "project", "transition", "crowdfund", "active")

	// 非法状态
	if _, err := runErr(t, bin, "--db", db, "project", "transition", "crowdfund", "unknown"); err == nil {
		t.Fatal("transition to unknown error = nil, want failure")
	}

	// 转移写入项目变更注解（timeline 可见）
	timelineJSON := run(t, bin, "--db", db, "--json", "project", "timeline", "crowdfund", "--limit", "20")
	var timeline []map[string]any
	if err := json.Unmarshal([]byte(timelineJSON), &timeline); err != nil {
		t.Fatalf("json.Unmarshal(timeline) error = %v; output = %q", err, timelineJSON)
	}
	foundTransition := false
	for _, entry := range timeline {
		if content, _ := entry["content"].(string); strings.Contains(content, "状态变更") {
			foundTransition = true
			break
		}
	}
	if !foundTransition {
		t.Fatalf("timeline missing transition annotation: %#v", timeline)
	}
}

func TestCLIProjectWorkspaceIsolation(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "--workspace", "local", "project", "add", "api", "name:API")
	localInfo := run(t, bin, "--db", db, "--json", "--workspace", "local", "project", "info", "api")
	var localProject map[string]any
	if err := json.Unmarshal([]byte(localInfo), &localProject); err != nil {
		t.Fatalf("json.Unmarshal(local project info) error = %v", err)
	}
	if localProject["slug"] != "api" {
		t.Fatalf("local project info = %#v", localProject)
	}
	run(t, bin, "--db", db, "workspace", "add", "partner")
	if _, err := runErr(t, bin, "--db", db, "--workspace", "partner", "project", "info", "api"); err == nil {
		t.Fatal("partner project info by slug error = nil, want project_not_found")
	}

	projectID, _ := localProject["id"].(string)
	if projectID == "" {
		t.Fatalf("local project id missing in %q", localInfo)
	}
	if _, err := runErr(t, bin, "--db", db, "--workspace", "partner", "project", "info", projectID); err == nil {
		t.Fatal("partner project info by local id error = nil, want project_workspace_mismatch")
	}
}

func TestCLIProjectConfigLifecycle(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "apiplat", "name:AI Agent Platform")
	run(t, bin, "--db", db, "project", "config", "set", "apiplat", "agent.background", "Project background")
	got := strings.TrimSpace(run(t, bin, "--db", db, "project", "config", "get", "apiplat", "agent.background"))
	if got != "Project background" {
		t.Fatalf("project config get output = %q", got)
	}
	listOut := run(t, bin, "--db", db, "project", "config", "list", "apiplat")
	if !strings.Contains(listOut, "agent.background=Project background") {
		t.Fatalf("project config list output = %q", listOut)
	}
	run(t, bin, "--db", db, "project", "config", "unset", "apiplat", "agent.background")
	if _, err := runErr(t, bin, "--db", db, "project", "config", "get", "apiplat", "agent.background"); err == nil {
		t.Fatal("project config get after unset error = nil, want failure")
	}
}

func TestCLIConfigRejectsProjectScopedKeysWithoutScope(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "apiplat", "name:AI Agent Platform")
	for _, args := range [][]string{
		{"--db", db, "config", "set", "agent.background", "Background"},
		{"--db", db, "config", "get", "agent.background"},
		{"--db", db, "config", "unset", "agent.background"},
	} {
		out, err := runErr(t, bin, args...)
		if err == nil {
			t.Fatalf("%v error = nil, want config_scope_not_allowed", args)
		}
		if !strings.Contains(out, "config_scope_not_allowed") {
			t.Fatalf("%v output = %q", args, out)
		}
	}

	configList := run(t, bin, "--db", db, "config", "list")
	if strings.Contains(configList, "agent.background=") {
		t.Fatalf("config list should not include project config values: %q", configList)
	}
}

func TestCLILinkAddListRemove(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "task with links")

	out := run(t, bin, "--db", db, "1", "link", "add", "--type", "doc", "--url", "https://example.com/spec")
	if !strings.Contains(out, "Added") {
		t.Fatalf("link add output = %q", out)
	}

	out = run(t, bin, "--db", db, "1", "link", "list")
	if !strings.Contains(out, "[doc]") || !strings.Contains(out, "https://example.com/spec") {
		t.Fatalf("link list output = %q", out)
	}

	infoOut := run(t, bin, "--db", db, "--json", "info", "1")
	var infoPayload map[string]any
	if err := json.Unmarshal([]byte(infoOut), &infoPayload); err != nil {
		t.Fatalf("info JSON parse error = %v, output = %q", err, infoOut)
	}
	linksRaw, _ := infoPayload["links"].([]any)
	if len(linksRaw) != 1 {
		t.Fatalf("links count = %d, want 1", len(linksRaw))
	}
	linkMap, _ := linksRaw[0].(map[string]any)
	if linkMap["type"] != "doc" {
		t.Fatalf("link type = %v, want doc", linkMap["type"])
	}
	linkID, _ := linkMap["id"].(string)

	out = run(t, bin, "--db", db, "1", "link", "remove", linkID)
	if !strings.Contains(out, "Removed") {
		t.Fatalf("link remove output = %q", out)
	}

	out = run(t, bin, "--db", db, "1", "link", "list")
	if !strings.Contains(out, "No links") {
		t.Fatalf("expected no links, got = %q", out)
	}
}

func TestCLILinkAddRequiresTypeAndURL(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "task for link validation")

	out := runExpectError(t, bin, "--db", db, "1", "link", "add", "--url", "https://example.com")
	if !strings.Contains(out, "required") && !strings.Contains(out, "type") {
		t.Fatalf("expected type required error, got = %q", out)
	}

	out = runExpectError(t, bin, "--db", db, "1", "link", "add", "--type", "doc")
	if !strings.Contains(out, "required") && !strings.Contains(out, "url") {
		t.Fatalf("expected url required error, got = %q", out)
	}
}

func TestCLILinkInfoShowsLinks(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "task with info links")
	run(t, bin, "--db", db, "1", "link", "add", "--type", "doc", "--url", "https://example.com/spec")

	out := run(t, bin, "--db", db, "info", "1")
	if !strings.Contains(out, "Links") || !strings.Contains(out, "https://example.com/spec") {
		t.Fatalf("info output should contain link info, got = %q", out)
	}
}

func TestCLILinkListEmpty(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "task with no links")

	out := run(t, bin, "--db", db, "1", "link", "list")
	if !strings.Contains(out, "No links") {
		t.Fatalf("expected no links message, got = %q", out)
	}
}

func TestCLILinkAddRejectsCompletedTask(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "completed task for link")
	taskUUID := strings.TrimSpace(run(t, bin, "--db", db, "_uuids", "/completed task for link/"))
	run(t, bin, "--db", db, "1", "done")

	_, err := runErr(t, bin, "--db", db, taskUUID, "link", "add", "--type", "doc", "--url", "https://example.com")
	if err == nil {
		t.Fatal("link add on completed task should fail")
	}
}

func TestCLIProjectAnnotateDenotate(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "testproj", "name:Test Project")

	out := run(t, bin, "--db", db, "project", "annotate", "testproj", "first", "note")
	if !strings.Contains(out, "Annotated project") {
		t.Fatalf("annotate output = %q", out)
	}

	out = run(t, bin, "--db", db, "project", "annotations", "testproj")
	if !strings.Contains(out, "first note") {
		t.Fatalf("annotations output = %q", out)
	}

	out = run(t, bin, "--db", db, "--json", "project", "annotations", "testproj")
	var annotations []map[string]any
	if err := json.Unmarshal([]byte(out), &annotations); err != nil {
		t.Fatalf("JSON parse error = %v, output = %q", err, out)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations count = %d, want 1", len(annotations))
	}
	annotationID, _ := annotations[0]["ID"].(string)

	out = run(t, bin, "--db", db, "project", "denotate", "testproj", annotationID)
	if !strings.Contains(out, "Removed annotation") || !strings.Contains(out, "project") {
		t.Fatalf("denotate output = %q", out)
	}

	out = run(t, bin, "--db", db, "project", "annotations", "testproj")
	if !strings.Contains(out, "No annotations") {
		t.Fatalf("expected no annotations, got = %q", out)
	}
}

func TestCLIProjectTimeline(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "myproject", "name:My Project")

	run(t, bin, "--db", db, "project", "annotate", "myproject", "project decision")

	run(t, bin, "--db", db, "add", "task one", "project:myproject")
	run(t, bin, "--db", db, "1", "annotate", "task observation")

	out := run(t, bin, "--db", db, "project", "timeline", "myproject")
	if !strings.Contains(out, "project decision") {
		t.Fatalf("timeline missing project annotation = %q", out)
	}
	if !strings.Contains(out, "task observation") {
		t.Fatalf("timeline missing task annotation = %q", out)
	}
}

func TestCLIProjectAnnotateTargetStyle(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "webapp", "name:Web App")

	out := run(t, bin, "--db", db, "webapp", "annotate", "target style note")
	if !strings.Contains(out, "Annotated project") {
		t.Fatalf("target-style annotate output = %q", out)
	}

	out = run(t, bin, "--db", db, "webapp", "annotations")
	if !strings.Contains(out, "target style note") {
		t.Fatalf("annotations output = %q", out)
	}
}

func TestCLIProjectAnnotateRejectsArchived(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "project", "add", "oldproj", "name:Old Project")
	run(t, bin, "--db", db, "project", "archive", "oldproj")

	_, err := runErr(t, bin, "--db", db, "project", "annotate", "oldproj", "should fail")
	if err == nil {
		t.Fatal("expected annotate on archived project to fail")
	}
}

func TestCLIProjectSlugRejectsDigitStart(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	_, err := runErr(t, bin, "--db", db, "project", "add", "123project", "name:Bad Slug")
	if err == nil {
		t.Fatal("expected digit-starting slug to be rejected")
	}
}

func buildXuanchu(t *testing.T) string {
	t.Helper()
	buildXuanchuOnce.Do(func() {
		dir, err := os.MkdirTemp("", "xuanchu-integration-")
		if err != nil {
			buildXuanchuErr = err
			return
		}
		buildXuanchuPath = filepath.Join(dir, "xuanchu")
		cmd := exec.Command("go", "build", "-o", buildXuanchuPath, "./cmd/xuanchu")
		cmd.Dir = projectRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildXuanchuErr = fmt.Errorf("go build error: %w\n%s", err, out)
		}
	})
	if buildXuanchuErr != nil {
		t.Fatal(buildXuanchuErr)
	}
	return buildXuanchuPath
}

func seedM4DatabaseWithTasksForCLI(t *testing.T, dbPath string, tasks []seedTask) {
	t.Helper()
	db, err := gorm.Open(gsqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open(%s) error = %v", dbPath, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	defer sqlDB.Close()

	mustExecSQL(t, db, `CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	mustExecSQL(t, db, `CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT, default_workspace_id TEXT, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL)`)
	mustExecSQL(t, db, `CREATE UNIQUE INDEX idx_users_name ON users(name)`)
	mustExecSQL(t, db, `CREATE UNIQUE INDEX idx_users_email ON users(email)`)
	mustExecSQL(t, db, `CREATE TABLE workspaces (id TEXT PRIMARY KEY, slug TEXT NOT NULL, name TEXT NOT NULL DEFAULT 'Local', created_by_user_id TEXT, description TEXT, visibility TEXT NOT NULL DEFAULT 'private', settings_json TEXT NOT NULL DEFAULT '{}', archived_at INTEGER, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL DEFAULT 0)`)
	mustExecSQL(t, db, `CREATE UNIQUE INDEX idx_workspaces_slug ON workspaces(slug)`)
	mustExecSQL(t, db, `CREATE TABLE memberships (user_id TEXT NOT NULL, workspace_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (user_id, workspace_id))`)
	mustExecSQL(t, db, `CREATE INDEX idx_memberships_workspace_id ON memberships(workspace_id)`)
	mustExecSQL(t, db, `CREATE INDEX idx_memberships_role ON memberships(role)`)
	mustExecSQL(t, db, `CREATE TABLE audit_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, actor_user_id TEXT, workspace_id TEXT, action TEXT NOT NULL, target_type TEXT, target_id TEXT, payload_json TEXT, created_at INTEGER NOT NULL)`)
	mustExecSQL(t, db, `CREATE TABLE contexts (workspace_id TEXT NOT NULL, name TEXT NOT NULL, filter_source TEXT NOT NULL, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (workspace_id, name))`)
	mustExecSQL(t, db, `CREATE TABLE uda_definitions (workspace_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL, label TEXT, values_json TEXT, default_value TEXT, created_at INTEGER NOT NULL, modified_at INTEGER NOT NULL, PRIMARY KEY (workspace_id, name))`)
	mustExecSQL(t, db, `CREATE TABLE tasks (
uuid TEXT PRIMARY KEY,
workspace_id TEXT NOT NULL,
title TEXT NOT NULL,
description TEXT,
status TEXT NOT NULL,
entry INTEGER NOT NULL,
modified INTEGER NOT NULL,
end_ts INTEGER,
due INTEGER,
project TEXT,
priority TEXT,
start INTEGER,
wait INTEGER,
scheduled INTEGER,
until INTEGER,
recur TEXT,
parent TEXT,
mask TEXT,
i_mask INTEGER
)`)
	mustExecSQL(t, db, `CREATE INDEX idx_tasks_workspace_id ON tasks(workspace_id)`)
	mustExecSQL(t, db, `CREATE INDEX idx_tasks_project ON tasks(project)`)
	mustExecSQL(t, db, `CREATE TABLE task_tags (task_uuid TEXT NOT NULL, tag TEXT NOT NULL, PRIMARY KEY (task_uuid, tag))`)
	mustExecSQL(t, db, `CREATE TABLE task_annotations (task_uuid TEXT NOT NULL, entry INTEGER NOT NULL, description TEXT NOT NULL, PRIMARY KEY (task_uuid, entry, description))`)
	mustExecSQL(t, db, `CREATE TABLE task_dependencies (task_uuid TEXT NOT NULL, depends_on TEXT NOT NULL, PRIMARY KEY (task_uuid, depends_on))`)
	mustExecSQL(t, db, `CREATE INDEX idx_task_dependencies_depends_on ON task_dependencies(depends_on)`)
	mustExecSQL(t, db, `CREATE TABLE task_uda_values (workspace_id TEXT NOT NULL, task_uuid TEXT NOT NULL, name TEXT NOT NULL, value TEXT NOT NULL, value_type TEXT, orphan NUMERIC NOT NULL DEFAULT false, PRIMARY KEY (task_uuid, name))`)
	mustExecSQL(t, db, `CREATE INDEX idx_task_uda_values_workspace_id ON task_uda_values(workspace_id)`)
	mustExecSQL(t, db, `CREATE INDEX idx_task_uda_values_task_uuid ON task_uda_values(task_uuid)`)

	workspaces := map[string]string{}
	for _, task := range tasks {
		slug := task.WorkspaceSlug
		if slug == "" {
			slug = "local"
		}
		if _, ok := workspaces[slug]; !ok {
			workspaces[slug] = "ws-" + slug
			mustExecSQL(t, db, `INSERT INTO workspaces(id, slug, name, visibility, settings_json, created_at, modified_at) VALUES(?, ?, ?, 'private', '{}', 1, 1)`, workspaces[slug], slug, "Workspace "+slug)
		}
	}
	for _, task := range tasks {
		slug := task.WorkspaceSlug
		if slug == "" {
			slug = "local"
		}
		mustExecSQL(t, db, `INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project) VALUES(?, ?, ?, 'pending', ?, ?, ?)`,
			task.UUID, workspaces[slug], "task "+task.UUID, task.Entry, task.Entry, task.Project)
	}
}

type seedTask struct {
	WorkspaceSlug string
	UUID          string
	Project       *string
	Entry         int64
}

func mustExecSQL(t *testing.T, db *gorm.DB, query string, args ...any) {
	t.Helper()
	if err := db.Exec(query, args...).Error; err != nil {
		t.Fatalf("Exec(%s) error = %v", query, err)
	}
}

func strptr(v string) *string { return &v }

func projectRoot(t *testing.T) string {
	t.Helper()
	// tests/integration -> project root is two directories up
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func run(t *testing.T, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v error = %v\n%s", bin, args, err, out)
	}
	return string(out)
}

func runExpectError(t *testing.T, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("%s %v succeeded unexpectedly\n%s", bin, args, out)
	}
	return string(out)
}

func runWithEnv(t *testing.T, env map[string]string, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v error = %v\n%s", bin, args, err, out)
	}
	return string(out)
}

func startXuanchuServer(t *testing.T, bin string, args ...string) (*exec.Cmd, string) {
	t.Helper()
	listen := pickFreeAddr(t)
	serverArgs := append([]string{"server", "--listen", listen}, args...)
	cmd := exec.Command(bin, serverArgs...)
	cmd.Env = os.Environ()
	output, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	baseURL := "http://" + listen
	waitForHTTPServer(t, baseURL+"/healthz", output, cmd)
	return cmd, baseURL
}

func stopXuanchuServer(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
	}
	_, _ = cmd.Process.Wait()
}

func pickFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitForHTTPServer(t *testing.T, url string, stderr io.Reader, cmd *exec.Cmd) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			data, _ := io.ReadAll(stderr)
			t.Fatalf("server exited before ready: %s", string(data))
		}
		time.Sleep(50 * time.Millisecond)
	}
	data, _ := io.ReadAll(stderr)
	t.Fatalf("server did not become ready: %s", string(data))
}

func runErr(t *testing.T, bin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func buildEditorHelper(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "editor")
	cmd := exec.Command("go", "build", "-o", bin, mainPath)
	cmd.Dir = projectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build editor helper error = %v\n%s", err, out)
	}
	return bin
}

func TestCLIMCPStdioOutputsJSONRPC(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	// 初始化 DB（创建 local user/workspace）
	run(t, bin, "--db", db, "list")

	// 发送 MCP initialize 请求到 stdin，读取 stdout
	initRequest := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}` + "\n"

	cmd := exec.Command(bin, "--db", db, "mcp", "stdio")
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// 发送 initialize
	if _, err := io.WriteString(stdinPipe, initRequest); err != nil {
		t.Fatal(err)
	}
	// 发送 shutdown
	time.Sleep(200 * time.Millisecond)
	shutdownRequest := `{"jsonrpc":"2.0","id":2,"method":"shutdown","params":{}}` + "\n"
	_, _ = io.WriteString(stdinPipe, shutdownRequest)
	time.Sleep(200 * time.Millisecond)
	stdinPipe.Close()

	_ = cmd.Wait()

	// stderr 不应有 migration warning
	stderrStr := stderr.String()
	if strings.Contains(stderrStr, "warning") || strings.Contains(stderrStr, "migration") {
		t.Fatalf("stderr should not contain migration warning: %q", stderrStr)
	}

	// stdout 应该只有 JSON-RPC 响应
	stdoutStr := strings.TrimSpace(stdout.String())
	if stdoutStr == "" {
		t.Fatal("stdout is empty, expected JSON-RPC response")
	}
	lines := strings.Split(stdoutStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			t.Fatalf("stdout line is not JSON: %q", line)
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("stdout line is not valid JSON: %q\nparse error: %v", line, err)
		}
		if _, ok := msg["jsonrpc"]; !ok {
			t.Fatalf("stdout JSON missing jsonrpc field: %q", line)
		}
	}
}

func TestCLIServerMCPRejectsMissingToken(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	// POST /mcp 无 token -> 401
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /mcp without token status = %d, want 401 body=%s", resp.StatusCode, string(body))
	}

	// GET /mcp 无 token -> 401
	req, err = http.NewRequest(http.MethodGet, baseURL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /mcp without token status = %d, want 401 body=%s", resp.StatusCode, string(body))
	}
}

func TestConfigFlagNonexistent(t *testing.T) {
	bin := buildXuanchu(t)
	runExpectError(t, bin, "--config", "/nonexistent/path.toml", "list")
}

func TestConfigFlagValid(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "xuanchu.db")
	tomlPath := filepath.Join(dir, "xuanchu.toml")
	if err := os.WriteFile(tomlPath, []byte("[database]\npath = \""+dbPath+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, bin, "--config", tomlPath, "list")
}

func TestVersionOutput(t *testing.T) {
	bin := buildXuanchu(t)
	out := run(t, bin, "--version")
	if !strings.HasPrefix(strings.TrimSpace(out), "xuanchu ") {
		t.Fatalf("--version output = %q", out)
	}
}

func TestCLIServerMCPRejectsBodyOverLimit(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	// 创建 token
	out := run(t, bin, "--db", db, "--json", "--workspace", "local", "token", "create", "mcp-http", "--scope", "task:read", "--expires-in", "720h")
	var created map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	token, _ := created["token"].(string)

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	// 发送超大 body -> 413
	largeBody := make([]byte, 11*1024*1024) // 11MB，超过默认 10MB 限制
	for i := range largeBody {
		largeBody[i] = 'a'
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewReader(largeBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /mcp oversized body status = %d, want 413 body=%s", resp.StatusCode, string(body))
	}
}
