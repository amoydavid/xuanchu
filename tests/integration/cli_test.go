package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCLIAddListInfo(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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

func TestCLIShowAndConfig(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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

func TestCLITomlRuntimeAffectsServiceBehavior(t *testing.T) {
	bin := buildTaskg(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "taskg.db")
	configDir := filepath.Join(dir, "config", "taskg")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
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
	bin := buildTaskg(t)
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config", "taskg")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlDB := filepath.Join(dir, "toml.db")
	flagDB := filepath.Join(dir, "flag.db")
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte("[database]\npath = \""+tomlDB+"\"\n"), 0o644); err != nil {
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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

func TestCLIUDAConfigAndImport(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	if err := os.WriteFile(path, []byte(`[{"uuid":"u1","description":"legacy task","status":"pending","entry":"1970-01-01T00:01:40Z","modified":"1970-01-01T00:01:40Z","legacy_field":"old"}]`), 0o644); err != nil {
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	path := filepath.Join(t.TempDir(), ".taskrc")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"color=off",
		"dateformat=epoch",
		"context.work=project:work",
		"uda.estimate.type=numeric",
		"uda.estimate.values=1,2,3",
		"urgency.uda.estimate.coefficient=2",
		"report.next.columns=id,description",
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "config", "set", "date.format", "epoch")
	show := run(t, bin, "--db", db, "_show", "date.format", "database.path")
	lines := strings.Split(strings.TrimSpace(show), "\n")
	if len(lines) != 2 || lines[0] != "epoch" || lines[1] != db {
		t.Fatalf("_show output = %q", show)
	}
	version := strings.TrimSpace(run(t, bin, "_version"))
	if version != "taskg dev" {
		t.Fatalf("_version output = %q", version)
	}
	badDB := filepath.Join(t.TempDir(), "missing-parent", "taskg.db")
	completion := run(t, bin, "--db", badDB, "completion", "bash")
	if !strings.Contains(completion, "complete") || !strings.Contains(completion, "taskg") {
		t.Fatalf("completion output = %q", completion)
	}
}

func TestCLICompletionDoesNotOpenDatabase(t *testing.T) {
	bin := buildTaskg(t)
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			badDB := filepath.Join(t.TempDir(), "missing-parent", "taskg.db")
			out := run(t, bin, "--db", badDB, "completion", shell)
			if !strings.Contains(out, "taskg") {
				t.Fatalf("completion %s output = %q", shell, out)
			}
		})
	}
}

func TestCLICompletionRejectsUnsupportedShell(t *testing.T) {
	bin := buildTaskg(t)
	badDB := filepath.Join(t.TempDir(), "missing-parent", "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

	run(t, bin, "--db", db, "add", "one")
	run(t, bin, "--db", db, "context", "define", "work", "description:one")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
		{name: "get", args: []string{"_get", "1.description"}, wantWork: "work task", wantLocal: "local task"},
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

	addOut := run(t, bin, "--db", db, "--json", "add", "write", "spec")
	var created map[string]any
	if err := json.Unmarshal([]byte(addOut), &created); err != nil {
		t.Fatalf("add --json output is not JSON: %v\n%s", err, addOut)
	}
	if created["description"] != "write spec" {
		t.Fatalf("created description = %#v", created["description"])
	}

	listOut := run(t, bin, "--db", db, "--json", "list")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list --json output is not JSON: %v\n%s", err, listOut)
	}
	if len(listed) != 1 || listed[0]["description"] != "write spec" {
		t.Fatalf("listed = %#v", listed)
	}
}

func TestCLIShowUsesParsedDBFlag(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "custom.db")

	out := run(t, bin, "--db", db, "show")
	if !strings.Contains(out, "database.path="+db) {
		t.Fatalf("show output = %q, want database.path=%s", out, db)
	}
}

func TestCLIPrefixFiltersAndTargetFilters(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

	run(t, bin, "--db", db, "project", "add", "taskg", "name:Taskg")
	run(t, bin, "--db", db, "add", "write", "spec", "project:taskg", "priority:H", "due:2030-01-01", "+planning")
	out := run(t, bin, "--db", db, "info", "1")
	for _, want := range []string{"UUID:", "Status:", "Description:", "Entry:", "Modified:", "Due:", "Project:", "Priority:", "Tags:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("info output missing %q: %q", want, out)
		}
	}
}

func TestCLIReportsAndUrgency(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "project", "add", "work", "name:Work")
	run(t, bin, "--db", db, "project", "add", "home", "name:Home")
	run(t, bin, "--db", db, "add", "work", "task", "project:work", "+next")
	run(t, bin, "--db", db, "add", "home", "task", "project:home", "+later")

	nextID := strings.TrimSpace(run(t, bin, "--db", db, "_ids", "+next"))
	if nextID == "" {
		t.Fatal("_ids +next returned empty result")
	}
	got := run(t, bin, "--db", db, "_get", nextID+".description", nextID+".tag.next", nextID+".tag.missing", nextID+".urgency")
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
	bin := buildTaskg(t)
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "alpha", "+work")
	run(t, bin, "--db", db, "add", "beta", "+work")
	run(t, bin, "--db", db, "1", "done")

	out := run(t, bin, "--db", db, "+work", "list")
	if strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("+work list output = %q", out)
	}
}

func TestCLIIDsReturnDefaultWorkingSetIDs(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "first")
	run(t, bin, "--db", db, "add", "second", "+next")

	ids := run(t, bin, "--db", db, "_ids", "+next")
	if strings.TrimSpace(ids) != "2" {
		t.Fatalf("_ids output = %q, want 2", ids)
	}
	get := run(t, bin, "--db", db, "_get", strings.TrimSpace(ids)+".description")
	if strings.TrimSpace(get) != "second" {
		t.Fatalf("_get for _ids output = %q, want second", get)
	}
}

func TestCLIIDsOnlyReturnDefaultWorkingSetIDsForCompletedQueries(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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

func TestCLIDescriptionAttributeUsesSubstring(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "write", "spec")
	run(t, bin, "--db", db, "add", "other", "task")

	out := run(t, bin, "--db", db, "description:spec", "list")
	if !strings.Contains(out, "write spec") {
		t.Fatalf("description:spec list missing write spec: %q", out)
	}
	if strings.Contains(out, "other task") {
		t.Fatalf("description:spec list should not match other task: %q", out)
	}
}

func TestCLIM1QueryOverdueReport(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "annotated task")
	run(t, bin, "--db", db, "1", "annotate", "first note")
	got := run(t, bin, "--db", db, "_get", "1.annotations")
	if !strings.Contains(got, "first note") {
		t.Fatalf("annotations output = %q", got)
	}
	run(t, bin, "--db", db, "1", "denotate", "1")
	got = run(t, bin, "--db", db, "_get", "1.annotations")
	if strings.Contains(got, "first note") {
		t.Fatalf("annotation not removed: %q", got)
	}
}

func TestCLIM2Reports(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "hidden waiting", "wait:tomorrow")
	run(t, bin, "--db", db, "add", "visible task")
	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "2") || !strings.Contains(list, "visible task") {
		t.Fatalf("list output = %q, want visible task with working-set ID 2", list)
	}
	got := run(t, bin, "--db", db, "_get", "2.description")
	if strings.TrimSpace(got) != "visible task" {
		t.Fatalf("_get 2.description = %q, want visible task", got)
	}
}

func TestCLINextRowIDMatchesWorkingSetIDUnderUrgencySort(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "middle")
	run(t, bin, "--db", db, "1", "append", "end")
	run(t, bin, "--db", db, "1", "prepend", "start")
	got := run(t, bin, "--db", db, "_get", "1.description")
	if !strings.Contains(got, "start middle end") {
		t.Fatalf("description = %q", got)
	}
}

func TestCLIAppendPrependCommands(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "middle")
	run(t, bin, "--db", db, "append", "1", "tail", "text")
	run(t, bin, "--db", db, "prepend", "1", "head", "text")
	got := run(t, bin, "--db", db, "_get", "1.description")
	if !strings.Contains(got, "head text middle tail text") {
		t.Fatalf("description = %q", got)
	}
}

func TestCLIEditRejectsInvalidDate(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	doc["description"] = "edited task"
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
	got := run(t, bin, "--db", db, "_get", "1.description")
	if !strings.Contains(got, "edited task") {
		t.Fatalf("description = %q", got)
	}
}

func TestCLIRecurringDaily(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
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
	bin := buildTaskg(t)
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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

	addOut := run(t, bin, "--db", db, "project", "add", "ai-agent-platform", "name:AI Agent Platform")
	if !strings.Contains(addOut, "Created project ai-agent-platform") {
		t.Fatalf("project add output = %q", addOut)
	}
	listOut := run(t, bin, "--db", db, "project", "list")
	if !strings.Contains(listOut, "ai-agent-platform") {
		t.Fatalf("project list output = %q", listOut)
	}
	infoJSON := run(t, bin, "--db", db, "--json", "project", "info", "ai-agent-platform")
	var info map[string]any
	if err := json.Unmarshal([]byte(infoJSON), &info); err != nil {
		t.Fatalf("json.Unmarshal(project info) error = %v", err)
	}
	if info["slug"] != "ai-agent-platform" || info["name"] != "AI Agent Platform" {
		t.Fatalf("project info --json output = %#v", info)
	}
	modOut := run(t, bin, "--db", db, "project", "modify", "ai-agent-platform", "description:Agent MCP platform")
	if !strings.Contains(modOut, "Modified project ai-agent-platform") {
		t.Fatalf("project modify output = %q", modOut)
	}
	run(t, bin, "--db", db, "add", "Design schema", "project:ai-agent-platform")
	archiveOut := run(t, bin, "--db", db, "project", "archive", "ai-agent-platform")
	if !strings.Contains(archiveOut, "Archived project ai-agent-platform") || !strings.Contains(archiveOut, "warning: archived project ai-agent-platform still has 1 non-deleted task") {
		t.Fatalf("project archive output = %q", archiveOut)
	}
	archiveJSON := run(t, bin, "--db", db, "--json", "project", "info", "ai-agent-platform")
	var archivedInfo map[string]any
	if err := json.Unmarshal([]byte(archiveJSON), &archivedInfo); err != nil {
		t.Fatalf("json.Unmarshal(archived project info) error = %v", err)
	}
	if archivedInfo["task_count"] != float64(1) {
		t.Fatalf("archived project task_count = %#v, want 1", archivedInfo["task_count"])
	}
	run(t, bin, "--db", db, "project", "add", "json-archive", "name:JSON Archive")
	run(t, bin, "--db", db, "add", "JSON archive task", "project:json-archive")
	archiveJSONOut := run(t, bin, "--db", db, "--json", "project", "archive", "json-archive")
	var archivedArchive map[string]any
	if err := json.Unmarshal([]byte(archiveJSONOut), &archivedArchive); err != nil {
		t.Fatalf("json.Unmarshal(project archive --json) error = %v; output = %q", err, archiveJSONOut)
	}
	if archivedArchive["slug"] != "json-archive" || archivedArchive["task_count"] != float64(1) || archivedArchive["status"] != "archived" {
		t.Fatalf("project archive --json output = %#v", archivedArchive)
	}
	if _, err := runErr(t, bin, "--db", db, "add", "Should fail", "project:ai-agent-platform"); err == nil {
		t.Fatal("add with archived project error = nil, want failure")
	}
	if _, err := runErr(t, bin, "--db", db, "project", "archive", "ai-agent-platform"); err == nil {
		t.Fatal("project archive twice error = nil, want failure")
	}
}

func TestCLIProjectWorkspaceIsolation(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

	run(t, bin, "--db", db, "project", "add", "ai-agent-platform", "name:AI Agent Platform")
	run(t, bin, "--db", db, "project", "config", "set", "ai-agent-platform", "agent.background", "Project background")
	got := strings.TrimSpace(run(t, bin, "--db", db, "project", "config", "get", "ai-agent-platform", "agent.background"))
	if got != "Project background" {
		t.Fatalf("project config get output = %q", got)
	}
	listOut := run(t, bin, "--db", db, "project", "config", "list", "ai-agent-platform")
	if !strings.Contains(listOut, "agent.background=Project background") {
		t.Fatalf("project config list output = %q", listOut)
	}
	run(t, bin, "--db", db, "project", "config", "unset", "ai-agent-platform", "agent.background")
	if _, err := runErr(t, bin, "--db", db, "project", "config", "get", "ai-agent-platform", "agent.background"); err == nil {
		t.Fatal("project config get after unset error = nil, want failure")
	}
}

func TestCLIConfigRejectsProjectScopedKeysWithoutScope(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

	run(t, bin, "--db", db, "project", "add", "ai-agent-platform", "name:AI Agent Platform")
	for _, args := range [][]string{
		{"--db", db, "config", "set", "agent.background", "Background"},
		{"--db", db, "config", "get", "agent.background"},
		{"--db", db, "config", "unset", "agent.background"},
	} {
		out, err := runErr(t, bin, args...)
		if err == nil {
			t.Fatalf("%v error = nil, want project_config_scope_required", args)
		}
		if !strings.Contains(out, "project config requires project scope") {
			t.Fatalf("%v output = %q", args, out)
		}
	}

	configList := run(t, bin, "--db", db, "config", "list")
	if strings.Contains(configList, "agent.background=") {
		t.Fatalf("config list should not include project config values: %q", configList)
	}
}

func buildTaskg(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "taskg")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/taskg")
	cmd.Dir = projectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v\n%s", err, out)
	}
	return bin
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
description TEXT NOT NULL,
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
		mustExecSQL(t, db, `INSERT INTO tasks(uuid, workspace_id, description, status, entry, modified, project) VALUES(?, ?, ?, 'pending', ?, ?, ?)`,
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
