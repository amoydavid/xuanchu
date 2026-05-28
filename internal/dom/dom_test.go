package dom

import (
	"testing"

	"github.com/dajee/taskg/internal/task"
)

func TestResolveTaskField(t *testing.T) {
	project := "work"
	tsk := task.Task{UUID: "u1", Description: "write spec", Status: task.StatusPending, Project: &project, Tags: []string{"next"}}
	got, err := Resolve(tsk, "description", 12.5)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "write spec" {
		t.Fatalf("got %q", got)
	}
	tag, err := Resolve(tsk, "tag.next", 12.5)
	if err != nil {
		t.Fatalf("Resolve(tag.next) error = %v", err)
	}
	if tag != "next" {
		t.Fatalf("tag = %q", tag)
	}
}

func TestResolveUnknownField(t *testing.T) {
	if _, err := Resolve(task.Task{}, "foo", 0); err == nil {
		t.Fatal("Resolve() error = nil, want error")
	}
}

func TestResolveMissingVirtualTagReturnsEmpty(t *testing.T) {
	got, err := Resolve(task.Task{Tags: []string{"next"}}, "tag.urgent", 0)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}
