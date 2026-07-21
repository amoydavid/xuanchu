package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommandVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{
		Stdout:  &stdout,
		Stderr:  &stderr,
		Version: "test",
	})
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stdout.String(); got != "xuanchu test\n" {
		t.Fatalf("stdout = %q, want %q", got, "xuanchu test\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRootReorderRecognizesM1QueryTokens(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantCmd string
	}{
		{name: "parentheses", args: []string{"(project:work and +urgent) or priority:H", "list"}, wantCmd: "list"},
		{name: "slash text", args: []string{"/spec/", "all"}, wantCmd: "all"},
		{name: "multi token bool", args: []string{"+next", "or", "due.before:tomorrow", "list"}, wantCmd: "list"},
		{name: "negative tag", args: []string{"-later", "completed"}, wantCmd: "completed"},
		{name: "attr value", args: []string{"project:work", "list"}, wantCmd: "list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, positional := splitFlagsAndPositional(tt.args)
			knownSubs := map[string]bool{
				"add": true, "list": true, "next": true, "info": true,
				"export": true, "import": true, "show": true, "config": true,
				"project": true,
				"help":    true, "version": true, "completion": true,
				"all": true, "completed": true, "deleted": true, "overdue": true,
				"urgency": true, "_urgency": true,
				"calc": true, "_get": true, "_ids": true, "_uuids": true, "_projects": true, "_tags": true, "_udas": true, "_unique": true, "_show": true, "_version": true,
			}
			if idx := commandIndex(positional, knownSubs); idx > 0 {
				reordered := append([]string{positional[idx]}, positional[:idx]...)
				reordered = append(reordered, positional[idx+1:]...)
				positional = reordered
			}
			_ = flags
			if len(positional) == 0 || positional[0] != tt.wantCmd {
				t.Fatalf("args = %v, positional = %v, want first = %q", tt.args, positional, tt.wantCmd)
			}
		})
	}
}

func TestRootParsesRcOverridesAndNoContext(t *testing.T) {
	flags, positional, rc := splitFlagsRcAndPositional([]string{"rc.date.format=epoch", "--no-context", "+next", "list"})
	if len(flags) != 1 || flags[0] != "--no-context" {
		t.Fatalf("flags = %#v", flags)
	}
	if len(positional) != 2 || positional[0] != "+next" || positional[1] != "list" {
		t.Fatalf("positional = %#v", positional)
	}
	if got := rcValue(rc, "date.format"); got != "epoch" {
		t.Fatalf("date.format rc = %q", got)
	}
}

func TestRootParsesWorkspaceFlag(t *testing.T) {
	flags, positional, _ := splitFlagsRcAndPositional([]string{"--workspace", "work", "+next", "list"})
	if len(flags) != 2 || flags[0] != "--workspace" || flags[1] != "work" {
		t.Fatalf("flags = %#v", flags)
	}
	if len(positional) != 2 || positional[0] != "+next" || positional[1] != "list" {
		t.Fatalf("positional = %#v", positional)
	}
}

func TestRootParsesWorkspaceEqualsFlag(t *testing.T) {
	flags, positional, _ := splitFlagsRcAndPositional([]string{"--workspace=work", "+next", "list"})
	if len(flags) != 1 || flags[0] != "--workspace=work" {
		t.Fatalf("flags = %#v", flags)
	}
	if len(positional) != 2 || positional[0] != "+next" || positional[1] != "list" {
		t.Fatalf("positional = %#v", positional)
	}
}

func TestRcOverrideEmptyClearsKey(t *testing.T) {
	tests := [][]string{
		{"rc.context="},
		{"rc.context:"},
		{"rc.context=none"},
	}
	for _, args := range tests {
		_, positional, rc := splitFlagsRcAndPositional(args)
		if len(positional) != 0 {
			t.Fatalf("%v positional = %#v", args, positional)
		}
		if got := rcValue(rc, "context.active"); got != "" {
			t.Fatalf("%v context.active = %q, want empty", args, got)
		}
	}
}

func TestExecutePassesRcOverridesToSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	if err := Execute(cmd, opts, []string{"--db", db, "rc.date.format=epoch", "show"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "date.format=epoch") {
		t.Fatalf("stdout = %q, want rc override in show output", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRootIncludesMCPStdioCommand(t *testing.T) {
	cmd := NewRootCommand(Options{})
	mcpCmd, _, err := cmd.Find([]string{"mcp", "stdio"})
	if err != nil {
		t.Fatalf("Find(mcp stdio) error = %v", err)
	}
	if mcpCmd == nil || mcpCmd.Name() != "stdio" {
		t.Fatalf("Find(mcp stdio) = %#v, want stdio command", mcpCmd)
	}
}

func TestExecuteShowUsesScopedActiveKeys(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	if err := Execute(cmd, opts, []string{"--db", db, "show"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	out := stdout.String()
	for _, key := range []string{"active.user=local", "active.workspace=local", "active.context="} {
		if !strings.Contains(out, key) {
			t.Fatalf("show output = %q, missing %q", out, key)
		}
	}
	if strings.Contains(out, "context.active=") {
		t.Fatalf("show output = %q, should not expose legacy context.active", out)
	}
}

func TestExecuteRejectsLegacyContextActiveGet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	if err := Execute(cmd, opts, []string{"--db", db, "config", "get", "context.active"}); err == nil {
		t.Fatal("Execute() error = nil, want unsupported legacy key")
	}
}

func TestExecuteRejectsLegacyContextActiveShow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	if err := Execute(cmd, opts, []string{"--db", db, "_show", "context.active"}); err == nil {
		t.Fatal("Execute() error = nil, want unsupported legacy key")
	}
}

func TestExecuteRejectsInternalScopedShowKeys(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	for _, key := range []string{"active_user_id", "active_workspace.local", "active_context.local.local"} {
		cmd := NewRootCommand(opts)
		err := Execute(cmd, opts, []string{"--db", db, "_show", key})
		if err == nil {
			t.Fatalf("_show %s error = nil, want unsupported internal key", key)
		}
	}
}

func TestExecuteConfigListHidesInternalScopedKeys(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	if err := Execute(cmd, opts, []string{"--db", db, "context", "define", "work", "description:one"}); err != nil {
		t.Fatalf("context define error = %v", err)
	}
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "context", "use", "work"}); err != nil {
		t.Fatalf("context use error = %v", err)
	}
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "config", "list"}); err != nil {
		t.Fatalf("config list error = %v", err)
	}
	out := stdout.String()
	for _, forbidden := range []string{"context.active=", "active_user_id=", "active_context."} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("config list output = %q, should not contain %q", out, forbidden)
		}
	}
}

func TestExecuteAuditListReturnsServiceError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	err := Execute(cmd, opts, []string{"--db", db, "audit", "list", "--project", "missing"})
	if err == nil {
		t.Fatal("Execute(audit list --project missing) error = nil, want service error")
	}
}

func TestExecuteProjectConfigListReturnsServiceError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	err := Execute(cmd, opts, []string{"--db", db, "project", "config", "list", "missing"})
	if err == nil {
		t.Fatal("Execute(project config list missing) error = nil, want service error")
	}
}

func TestUserUseIgnoresWorkspaceOverride(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--workspace", "missing", "user", "use", "alice"}); err != nil {
		t.Fatalf("user use with missing workspace override error = %v", err)
	}
}

func TestJSONViewsUseSnakeCaseFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "user", "list"}); err != nil {
		t.Fatalf("user list --json error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("user list --json invalid JSON: %v\n%s", err, stdout.String())
	}
	if len(rows) == 0 || rows[0]["id"] == nil || rows[0]["default_workspace_id"] == nil {
		t.Fatalf("user JSON rows = %#v", rows)
	}
	if _, ok := rows[0]["ID"]; ok {
		t.Fatalf("user JSON uses Go field names: %#v", rows[0])
	}
}

func TestProjectCommandRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	if got, _, err := cmd.Find([]string{"project"}); err != nil {
		t.Fatalf("Find(project) error = %v", err)
	} else if got == nil || got.Name() != "project" {
		t.Fatalf("Find(project) = %#v, want project command", got)
	}
}

func TestExecuteKeepsHelpOnNestedProjectTemplateCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)

	if err := Execute(cmd, opts, []string{"project", "template", "--help"}); err != nil {
		t.Fatalf("Execute(project template --help) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "instantiate") || !strings.Contains(stdout.String(), "list") {
		t.Fatalf("nested help output = %q", stdout.String())
	}
}

func TestExecuteRejectsUnknownRCKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	if err := Execute(cmd, opts, []string{"--db", db, "rc.notreal=value", "show"}); err == nil {
		t.Fatal("Execute() error = nil, want unknown rc key error")
	}
}

func rcValue(values map[string]*string, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return *value
}
