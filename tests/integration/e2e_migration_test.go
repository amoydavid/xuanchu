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
	var rows []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("json list stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if !jsonArrayContainsString(mapsToAny(rows), "uuid", "legacy-task-1") {
		t.Fatalf("migrated list rows = %#v, want legacy task", rows)
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

func mapsToAny(rows []map[string]any) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}
