package taskrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTaskRCRecognizesConfigContextAndUDAKeys(t *testing.T) {
	dir := t.TempDir()
	included := filepath.Join(dir, "included.taskrc")
	if err := os.WriteFile(included, []byte("uda.reviewed.type=date\nunknown.value=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".taskrc")
	if err := os.WriteFile(path, []byte(`
# comment
data.location = /tmp/task
color = off
dateformat = Y-M-D
context.work = project:work
uda.estimate.type = numeric
uda.estimate.values = 1,2,3,5
report.next.columns = id,title
include `+included+`
`), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if !hasEntry(report.Imported, "date.format") || !hasEntry(report.Imported, "context.work") || !hasEntry(report.Imported, "uda.estimate.values") || !hasEntry(report.Imported, "uda.reviewed.type") {
		t.Fatalf("imported = %#v", report.Imported)
	}
	if !hasEntry(report.Skipped, "database.path") || !hasEntry(report.Skipped, "report.next.columns") {
		t.Fatalf("skipped = %#v", report.Skipped)
	}
	if !hasEntry(report.Unknown, "unknown.value") {
		t.Fatalf("unknown = %#v", report.Unknown)
	}
	if got := report.Values["uda.estimate.values"]; got != "1,2,3,5" {
		t.Fatalf("uda values = %q", got)
	}
}

func TestParseTaskRCPreservesQuotedHashesAndRejectsMalformedUDAKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".taskrc")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		`context.active="work#alpha" # trailing comment`,
		`uda.ticket.label="fix #1234"`,
		`uda..type=numeric`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if got := report.Values["context.active"]; got != "work#alpha" {
		t.Fatalf("context.active = %q, want work#alpha", got)
	}
	if got := report.Values["uda.ticket.label"]; got != "fix #1234" {
		t.Fatalf("uda.ticket.label = %q, want quoted hash preserved", got)
	}
	if hasEntry(report.Imported, "uda..type") {
		t.Fatalf("malformed uda key imported: %#v", report.Imported)
	}
	if !hasEntry(report.Unknown, "uda..type") {
		t.Fatalf("malformed uda key should be unknown: %#v", report.Unknown)
	}
}

func hasEntry(entries []Entry, key string) bool {
	for _, entry := range entries {
		if entry.Key == key || entry.Target == key {
			return true
		}
	}
	return false
}
