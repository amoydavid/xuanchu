package query

import (
	"testing"
	"time"
)

func TestParseAddArgsSeparatesDescriptionAndMods(t *testing.T) {
	parsed, err := ParseAddArgs([]string{"write", "spec", "project:taskg", "+planning", "priority:H"})
	if err != nil {
		t.Fatalf("ParseAddArgs() error = %v", err)
	}
	if parsed.Description != "write spec" {
		t.Fatalf("Description = %q", parsed.Description)
	}
	if parsed.Mod.Project == nil || *parsed.Mod.Project != "taskg" {
		t.Fatalf("Project = %#v", parsed.Mod.Project)
	}
	if parsed.Mod.Priority == nil || *parsed.Mod.Priority != "H" {
		t.Fatalf("Priority = %#v", parsed.Mod.Priority)
	}
	if len(parsed.Mod.AddTags) != 1 || parsed.Mod.AddTags[0] != "planning" {
		t.Fatalf("AddTags = %#v", parsed.Mod.AddTags)
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

func TestParseModifyArgsM2Fields(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"wait:tomorrow", "scheduled:eow", "until:2030-01-01", "depends:abc", "depends:", "recur:weekly"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if mod.Wait == nil || mod.Scheduled == nil || mod.Until == nil {
		t.Fatalf("date fields not parsed: %#v", mod)
	}
	if !mod.ClearDepends || len(mod.AddDepends) != 1 || mod.AddDepends[0] != "abc" {
		t.Fatalf("depends not parsed: %#v", mod)
	}
	if mod.Recur == nil || *mod.Recur != "weekly" {
		t.Fatalf("Recur = %#v", mod.Recur)
	}
}

func TestParseModifyArgsClearsM2DateFields(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"wait:", "scheduled:", "until:", "recur:"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if !mod.ClearWait || !mod.ClearScheduled || !mod.ClearUntil || !mod.ClearRecur {
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
	for _, arg := range []string{"mask:abc", "imask:1"} {
		if _, err := ParseModifyArgs([]string{arg}); err == nil {
			t.Fatalf("ParseModifyArgs(%q) error = nil, want reserved field rejected", arg)
		}
	}
}
