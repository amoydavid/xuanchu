package query

import "testing"

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

func TestParseFilters(t *testing.T) {
	filter, err := ParseFilters([]string{"+work", "project:taskg", "status:completed", "/spec/"})
	if err != nil {
		t.Fatalf("ParseFilters() error = %v", err)
	}
	if len(filter.Tags) != 1 || filter.Tags[0] != "work" {
		t.Fatalf("Tags = %#v", filter.Tags)
	}
	if filter.Project == nil || *filter.Project != "taskg" {
		t.Fatalf("Project = %#v", filter.Project)
	}
	if filter.Status == nil || *filter.Status != "completed" {
		t.Fatalf("Status = %#v", filter.Status)
	}
	if filter.Text == nil || *filter.Text != "spec" {
		t.Fatalf("Text = %#v", filter.Text)
	}
}

func TestParseModifyArgsRequiresModification(t *testing.T) {
	if _, err := ParseModifyArgs([]string{}); err == nil {
		t.Fatal("ParseModifyArgs() error = nil, want error")
	}
}
