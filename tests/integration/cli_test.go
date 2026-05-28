package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	run(t, bin, "--db", db, "add", "work", "task", "project:work", "+next")
	run(t, bin, "--db", db, "add", "home", "task", "project:home", "+later")

	got := run(t, bin, "--db", db, "_get", "1.description", "1.tag.next", "1.tag.missing", "1.urgency")
	if !strings.Contains(got, "work task") || !strings.Contains(got, "next") {
		t.Fatalf("_get output = %q", got)
	}
	ids := run(t, bin, "--db", db, "_ids", "+next")
	if strings.TrimSpace(ids) != "1" {
		t.Fatalf("_ids output = %q", ids)
	}
	projects := run(t, bin, "--db", db, "_projects")
	if !strings.Contains(projects, "home") || !strings.Contains(projects, "work") {
		t.Fatalf("_projects output = %q", projects)
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
