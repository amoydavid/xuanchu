package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// setupHookTestDB 创建临时数据库并返回数据库路径。
func setupHookTestDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "xuanchu.db")
}

// setupHookTestOpts 创建带标准输出的测试 Options。
func setupHookTestOpts(stdout, stderr *bytes.Buffer) Options {
	return Options{Stdout: stdout, Stderr: stderr}
}

func TestHookCommandRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(setupHookTestOpts(&stdout, &stderr))
	hookCmd, _, err := cmd.Find([]string{"hook"})
	if err != nil {
		t.Fatalf("Find(hook) error = %v", err)
	}
	if hookCmd == nil || hookCmd.Name() != "hook" {
		t.Fatalf("Find(hook) = %#v, want hook command", hookCmd)
	}
	// 检查所有子命令
	subcommands := []string{"list", "add", "info", "modify", "enable", "disable", "delete", "deliveries", "replay"}
	for _, name := range subcommands {
		sub, _, err := hookCmd.Find([]string{name})
		if err != nil {
			t.Errorf("Find(hook %s) error = %v", name, err)
		} else if sub == nil || sub.Name() != name {
			t.Errorf("Find(hook %s) = %#v, want %s command", name, sub, name)
		}
	}
}

func TestHookAddLocal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 先初始化 workspace
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 创建 hook
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "hook", "add", "test-hook",
		"--event", "task.created", "--url", "https://example.com/webhook"})
	if err != nil {
		t.Fatalf("hook add error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Created hook test-hook") {
		t.Fatalf("hook add stdout = %q, want created message", stdout.String())
	}
	if stderr.Len() > 0 {
		t.Fatalf("hook add stderr = %q, want empty", stderr.String())
	}
}

func TestHookAddAndListLocal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 添加 hook
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "add", "my-hook",
		"--event", "task.created", "--event", "task.completed", "--url", "https://example.com/hook"}); err != nil {
		t.Fatalf("hook add error = %v", err)
	}

	// 列出 hooks
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "list"}); err != nil {
		t.Fatalf("hook list error = %v", err)
	}
	if !strings.Contains(stdout.String(), "my-hook") {
		t.Fatalf("hook list stdout = %q, want hook name", stdout.String())
	}
}

func TestHookAddAndInfoLocal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 添加 hook 并捕获 ID
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "hook", "add", "info-hook",
		"--event", "task.modified", "--url", "https://example.com/hook2"}); err != nil {
		t.Fatalf("hook add --json error = %v", err)
	}
	var addResult map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &addResult); err != nil {
		t.Fatalf("hook add JSON parse error = %v", err)
	}
	hookID, _ := addResult["id"].(string)
	if hookID == "" {
		t.Fatal("hook add JSON missing id")
	}

	// 查询 hook 信息
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "info", hookID}); err != nil {
		t.Fatalf("hook info error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "info-hook") || !strings.Contains(output, "task.modified") {
		t.Fatalf("hook info stdout = %q, want hook name and event type", output)
	}
}

func TestHookDisableEnableLocal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 添加 hook 并获取 ID
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "hook", "add", "toggle-hook",
		"--event", "task.deleted", "--url", "https://example.com/hook3"}); err != nil {
		t.Fatalf("hook add error = %v", err)
	}
	var addResult map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &addResult); err != nil {
		t.Fatalf("hook add JSON parse error = %v", err)
	}
	hookID, _ := addResult["id"].(string)

	// 禁用 hook
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "disable", hookID}); err != nil {
		t.Fatalf("hook disable error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Disabled hook") {
		t.Fatalf("hook disable stdout = %q", stdout.String())
	}

	// 启用 hook
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "enable", hookID}); err != nil {
		t.Fatalf("hook enable error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Enabled hook") {
		t.Fatalf("hook enable stdout = %q", stdout.String())
	}
}

func TestHookDeleteLocal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 添加 hook 并获取 ID
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "hook", "add", "del-hook",
		"--event", "task.created", "--url", "https://example.com/hook4"}); err != nil {
		t.Fatalf("hook add error = %v", err)
	}
	var addResult map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &addResult); err != nil {
		t.Fatalf("hook add JSON parse error = %v", err)
	}
	hookID, _ := addResult["id"].(string)

	// 删除 hook
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "delete", hookID}); err != nil {
		t.Fatalf("hook delete error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Deleted hook") {
		t.Fatalf("hook delete stdout = %q", stdout.String())
	}

	// 验证删除后无法查询
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "hook", "info", hookID})
	if err == nil {
		t.Fatal("hook info after delete should fail")
	}
}

func TestHookAddJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// JSON 输出
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "--json", "hook", "add", "json-hook",
		"--event", "task.created", "--url", "https://example.com/webhook"})
	if err != nil {
		t.Fatalf("hook add --json error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("hook add JSON parse error = %v", err)
	}
	if result["name"] != "json-hook" {
		t.Fatalf("hook add JSON name = %v, want json-hook", result["name"])
	}
	if result["endpoint_url"] != "https://example.com/webhook" {
		t.Fatalf("hook add JSON endpoint_url = %v", result["endpoint_url"])
	}
	// secret 不应出现在输出中
	if _, ok := result["secret"]; ok {
		t.Fatal("hook add JSON should not contain secret")
	}
}

func TestHookListJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 添加两个 hooks
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "add", "hook-a",
		"--event", "task.created", "--url", "https://example.com/a"}); err != nil {
		t.Fatalf("hook add a error = %v", err)
	}
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "add", "hook-b",
		"--event", "task.completed", "--url", "https://example.com/b"}); err != nil {
		t.Fatalf("hook add b error = %v", err)
	}

	// JSON 列表
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "hook", "list"}); err != nil {
		t.Fatalf("hook list --json error = %v", err)
	}
	var listResult []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &listResult); err != nil {
		t.Fatalf("hook list JSON parse error = %v", err)
	}
	if len(listResult) != 2 {
		t.Fatalf("hook list JSON count = %d, want 2", len(listResult))
	}
}

func TestHookAddProjectScope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 创建项目
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "project", "add", "myproj", "name:My Project"}); err != nil {
		t.Fatalf("project add error = %v", err)
	}

	// 添加 project-scoped hook
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "hook", "add", "proj-hook",
		"--scope", "project", "--project", "myproj",
		"--event", "task.created", "--url", "https://example.com/proj-hook"})
	if err != nil {
		t.Fatalf("hook add project-scoped error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Created hook proj-hook") {
		t.Fatalf("hook add stdout = %q", stdout.String())
	}
}

func TestHookModifyLocal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 添加 hook 并获取 ID
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "hook", "add", "mod-hook",
		"--event", "task.created", "--url", "https://example.com/old"}); err != nil {
		t.Fatalf("hook add error = %v", err)
	}
	var addResult map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &addResult); err != nil {
		t.Fatalf("hook add JSON parse error = %v", err)
	}
	hookID, _ := addResult["id"].(string)

	// 修改 hook
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "hook", "modify", hookID,
		"--name", "renamed-hook", "--url", "https://example.com/new"})
	if err != nil {
		t.Fatalf("hook modify error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Modified hook") {
		t.Fatalf("hook modify stdout = %q", stdout.String())
	}

	// 验证修改结果
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "hook", "info", hookID}); err != nil {
		t.Fatalf("hook info error = %v", err)
	}
	if !strings.Contains(stdout.String(), "renamed-hook") {
		t.Fatalf("hook info after modify = %q, want renamed-hook", stdout.String())
	}
	if !strings.Contains(stdout.String(), "https://example.com/new") {
		t.Fatalf("hook info after modify = %q, want new URL", stdout.String())
	}
}

func TestHookAddRequiresEventAndURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 缺少 --url
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "hook", "add", "bad-hook", "--event", "task.created"})
	if err == nil {
		t.Fatal("hook add without --url should fail")
	}

	// 缺少 --event
	stdout.Reset()
	stderr.Reset()
	cmd = NewRootCommand(opts)
	err = Execute(cmd, opts, []string{"--db", db, "hook", "add", "bad-hook", "--url", "https://example.com"})
	if err == nil {
		t.Fatal("hook add without --event should fail")
	}
}

func TestHookInfoNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	// 初始化
	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}

	// 查询不存在的 hook
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "hook", "info", "nonexistent-id"})
	if err == nil {
		t.Fatal("hook info nonexistent should fail")
	}
}

func TestResolveSecretMutualExclusion(t *testing.T) {
	_, err := resolveSecret("val", true, "", nil)
	if err == nil {
		t.Fatal("resolveSecret with both --secret and --secret-stdin should fail")
	}

	_, err = resolveSecret("val", false, "file.txt", nil)
	if err == nil {
		t.Fatal("resolveSecret with both --secret and --secret-file should fail")
	}

	_, err = resolveSecret("", true, "file.txt", nil)
	if err == nil {
		t.Fatal("resolveSecret with both --secret-stdin and --secret-file should fail")
	}
}

func TestResolveSecretFromValue(t *testing.T) {
	val, err := resolveSecret("mysecret", false, "", nil)
	if err != nil {
		t.Fatalf("resolveSecret error = %v", err)
	}
	if val != "mysecret" {
		t.Fatalf("resolveSecret = %q, want mysecret", val)
	}
}

func TestResolveSecretEmpty(t *testing.T) {
	val, err := resolveSecret("", false, "", nil)
	if err != nil {
		t.Fatalf("resolveSecret error = %v", err)
	}
	if val != "" {
		t.Fatalf("resolveSecret = %q, want empty", val)
	}
}
