package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
