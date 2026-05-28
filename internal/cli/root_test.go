package cli

import (
	"bytes"
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
	if got := stdout.String(); got != "taskg test\n" {
		t.Fatalf("stdout = %q, want %q", got, "taskg test\n")
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
				"help": true, "version": true, "completion": true,
				"all": true, "completed": true, "deleted": true, "overdue": true,
				"urgency": true, "_urgency": true,
				"calc": true, "_get": true, "_ids": true, "_uuids": true, "_projects": true, "_tags": true, "_udas": true, "_unique": true,
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
	db := filepath.Join(t.TempDir(), "taskg.db")

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

func TestExecuteRejectsUnknownRCKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	cmd := NewRootCommand(opts)
	db := filepath.Join(t.TempDir(), "taskg.db")

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
