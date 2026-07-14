package integration

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ESQLiteMigrationKeepsJSONStdoutClean(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "legacy.db")
	seedM4DatabaseWithTasksForCLI(t, db, []seedTask{
		{WorkspaceSlug: "local", UUID: "legacy-task-1", Project: strptr("Legacy"), Entry: 10},
	})

	cmd := exec.Command(bin, "--db", db, "--json", "list")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("json list after migration error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "warning:") || strings.Contains(stdout.String(), "migration") {
		t.Fatalf("migration warning polluted stdout: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "warning:") || strings.Contains(stderr.String(), "migration") {
		t.Fatalf("unexpected migration warning on clean legacy fixture: %q", stderr.String())
	}
	var page map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &page); err != nil {
		t.Fatalf("json list stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	rows, ok := page["items"].([]any)
	if !ok || !jsonArrayContainsString(rows, "uuid", "legacy-task-1") {
		t.Fatalf("migrated list page = %#v, want legacy task", page)
	}

	add := run(t, bin, "--db", db, "--json", "add", "post migration task", "project:legacy")
	created := parseJSONMap(t, add)
	if created["title"] != "post migration task" {
		t.Fatalf("post migration add output = %#v", created)
	}
	info := run(t, bin, "--db", db, "--json", "info", "2")
	if parseJSONMap(t, info)["title"] != "post migration task" {
		t.Fatalf("post migration info output = %s", info)
	}
}
