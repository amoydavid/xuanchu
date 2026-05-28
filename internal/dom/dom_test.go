package dom

import (
	"strings"
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

func TestResolveM2Fields(t *testing.T) {
	start := int64(10)
	tsk := task.Task{
		UUID: "u1", Description: "task", Status: task.StatusPending, Start: &start,
		Depends:     []string{"dep1", "dep2"},
		Annotations: []task.Annotation{{Entry: 1, Description: "note"}},
	}
	if got, _ := Resolve(tsk, "start", 0); got != "10" {
		t.Fatalf("start = %q", got)
	}
	if got, _ := Resolve(tsk, "depends", 0); got != "dep1,dep2" {
		t.Fatalf("depends = %q", got)
	}
	if got, _ := Resolve(tsk, "annotations", 0); !strings.Contains(got, "note") {
		t.Fatalf("annotations = %q", got)
	}
}

func TestResolveUDAFields(t *testing.T) {
	tsk := task.Task{UDAs: map[string]task.UDAValue{
		"estimate": {Name: "estimate", Raw: "3", Type: "numeric"},
	}}
	for _, field := range []string{"estimate", "uda.estimate"} {
		got, err := Resolve(tsk, field, 0)
		if err != nil {
			t.Fatalf("Resolve(%s) error = %v", field, err)
		}
		if got != "3" {
			t.Fatalf("Resolve(%s) = %q, want 3", field, got)
		}
	}
}
