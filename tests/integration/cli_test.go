package integration

import (
	"encoding/json"
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

func TestCLINextCommandSortsByDuePriorityAndEntry(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")

	run(t, bin, "--db", db, "add", "later", "task", "due:2030-01-02")
	run(t, bin, "--db", db, "add", "urgent", "task", "due:2030-01-01", "priority:H")
	run(t, bin, "--db", db, "add", "undated", "task")

	out := run(t, bin, "--db", db, "next")
	urgent := strings.Index(out, "urgent task")
	later := strings.Index(out, "later task")
	undated := strings.Index(out, "undated task")
	if urgent < 0 || later < 0 || undated < 0 || !(urgent < later && later < undated) {
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

	run(t, bin, "--db", db, "add", "write", "spec", "project:taskg", "priority:H", "due:2030-01-01", "+planning")
	out := run(t, bin, "--db", db, "info", "1")
	for _, want := range []string{"UUID:", "Status:", "Description:", "Entry:", "Modified:", "Due:", "Project:", "Priority:", "Tags:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("info output missing %q: %q", want, out)
		}
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
