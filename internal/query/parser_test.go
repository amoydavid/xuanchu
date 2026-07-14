package query

import (
	"testing"
	"time"
)

func TestParseAddArgsSeparatesTitleAndMods(t *testing.T) {
	parsed, err := ParseAddArgs([]string{"write", "spec", "project:xuanchu", "+planning", "priority:H"})
	if err != nil {
		t.Fatalf("ParseAddArgs() error = %v", err)
	}
	if parsed.Title != "write spec" {
		t.Fatalf("Title = %q", parsed.Title)
	}
	if parsed.Mod.Project == nil || *parsed.Mod.Project != "xuanchu" {
		t.Fatalf("Project = %#v", parsed.Mod.Project)
	}
	if parsed.Mod.Priority == nil || *parsed.Mod.Priority != "H" {
		t.Fatalf("Priority = %#v", parsed.Mod.Priority)
	}
	if len(parsed.Mod.AddTags) != 1 || parsed.Mod.AddTags[0] != "planning" {
		t.Fatalf("AddTags = %#v", parsed.Mod.AddTags)
	}
}

func TestParseAddArgsRecognizesAssignees(t *testing.T) {
	parsed, err := ParseAddArgs([]string{"write", "spec", "@alice", "@bob@example.com"})
	if err != nil {
		t.Fatalf("ParseAddArgs() error = %v", err)
	}
	if parsed.Title != "write spec" {
		t.Fatalf("Title = %q", parsed.Title)
	}
	if got := parsed.Mod.AddAssignees; len(got) != 2 || got[0] != "alice" || got[1] != "bob@example.com" {
		t.Fatalf("AddAssignees = %#v", got)
	}
}

func TestParseModifyArgsRequiresModification(t *testing.T) {
	if _, err := ParseModifyArgs([]string{}); err == nil {
		t.Fatal("ParseModifyArgs() error = nil, want error")
	}
}

func TestParseAddArgsDueStoresEndOfDay(t *testing.T) {
	parsed, err := ParseAddArgs([]string{"task", "due:2030-01-01"})
	if err != nil {
		t.Fatalf("ParseAddArgs() error = %v", err)
	}
	if parsed.Mod.Due == nil {
		t.Fatal("Mod.Due is nil")
	}
	got := time.Unix(*parsed.Mod.Due, 0).In(time.Local)
	if got.Hour() != 23 || got.Minute() != 59 || got.Second() != 59 {
		t.Fatalf("Due time = %v, want 23:59:59 local", got)
	}
	if got.Year() != 2030 || got.Month() != time.January || got.Day() != 1 {
		t.Fatalf("Due date = %v, want 2030-01-01", got)
	}
}

func TestParseModifyArgsDueStoresEndOfDay(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"due:2030-06-15"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if mod.Due == nil {
		t.Fatal("Mod.Due is nil")
	}
	got := time.Unix(*mod.Due, 0).In(time.Local)
	if got.Hour() != 23 || got.Minute() != 59 || got.Second() != 59 {
		t.Fatalf("Due time = %v, want 23:59:59 local", got)
	}
}

func TestParseModifyArgsDateFieldBoundaries(t *testing.T) {
	mod, err := ParseModifyArgs([]string{
		"due:2030-06-15",
		"until:2030-06-16",
		"wait:2030-06-17",
		"scheduled:2030-06-18",
	})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	cases := []struct {
		name       string
		value      *int64
		wantHour   int
		wantMinute int
		wantSecond int
	}{
		{name: "due", value: mod.Due, wantHour: 23, wantMinute: 59, wantSecond: 59},
		{name: "until", value: mod.Until, wantHour: 23, wantMinute: 59, wantSecond: 59},
		{name: "wait", value: mod.Wait, wantHour: 0, wantMinute: 0, wantSecond: 0},
		{name: "scheduled", value: mod.Scheduled, wantHour: 0, wantMinute: 0, wantSecond: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == nil {
				t.Fatal("value is nil")
			}
			got := time.Unix(*tc.value, 0).In(time.Local)
			if got.Hour() != tc.wantHour || got.Minute() != tc.wantMinute || got.Second() != tc.wantSecond {
				t.Fatalf("%s time = %v, want %02d:%02d:%02d local", tc.name, got, tc.wantHour, tc.wantMinute, tc.wantSecond)
			}
		})
	}
}

func TestParseModifyArgsM2Fields(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"wait:tomorrow", "scheduled:eow", "until:2030-01-01", "depends:abc", "depends:"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if mod.Wait == nil || mod.Scheduled == nil || mod.Until == nil {
		t.Fatalf("date fields not parsed: %#v", mod)
	}
	if !mod.ClearDepends || len(mod.AddDepends) != 1 || mod.AddDepends[0] != "abc" {
		t.Fatalf("depends not parsed: %#v", mod)
	}
}

func TestParseModifyArgsRejectsLegacyRecur(t *testing.T) {
	// spec 2026-07-11：recur/mask/imask token 不再支持，返回错误而非静默忽略。
	for _, token := range []string{"recur:weekly", "recur:", "mask:abc", "imask:1"} {
		if _, err := ParseModifyArgs([]string{token}); err == nil {
			t.Fatalf("ParseModifyArgs(%q) 应失败", token)
		}
	}
}

func TestParseModifyArgsRecognizesAssigneeMutations(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"+@alice", "-@bob@example.com"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if got := mod.AddAssignees; len(got) != 1 || got[0] != "alice" {
		t.Fatalf("AddAssignees = %#v", got)
	}
	if got := mod.RemoveAssignees; len(got) != 1 || got[0] != "bob@example.com" {
		t.Fatalf("RemoveAssignees = %#v", got)
	}
}

func TestParseModifyArgsClearsM2DateFields(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"wait:", "scheduled:", "until:"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if !mod.ClearWait || !mod.ClearScheduled || !mod.ClearUntil {
		t.Fatalf("clear flags not set: %#v", mod)
	}
}

func TestParseModifyArgsAllowsUDAFields(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"estimate:3", "reviewed:"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if mod.UDAs["estimate"] != "3" {
		t.Fatalf("UDAs = %#v", mod.UDAs)
	}
	if len(mod.ClearUDAs) != 1 || mod.ClearUDAs[0] != "reviewed" {
		t.Fatalf("ClearUDAs = %#v", mod.ClearUDAs)
	}
}

func TestParseModifyArgsDoesNotTreatReservedFieldsAsUDA(t *testing.T) {
	for _, arg := range []string{"mask:abc", "imask:1", "task_slug:api-1", "project_seq:1"} {
		if _, err := ParseModifyArgs([]string{arg}); err == nil {
			t.Fatalf("ParseModifyArgs(%q) error = nil, want reserved field rejected", arg)
		}
	}
}
