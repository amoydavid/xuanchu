package render

import (
	"bytes"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestTaskListShowsAssignees(t *testing.T) {
	var buf bytes.Buffer
	project := "api"
	projectSeq := int64(3)
	TaskList(&buf, []task.Task{{
		UUID:        "12345678-1234-1234-1234-123456789abc",
		Description: "assigned task",
		Project:     &project,
		ProjectSeq:  &projectSeq,
		Assignees: []task.AssigneeInfo{{
			UserID: "user-local",
			Name:   "local",
		}},
	}})

	out := buf.String()
	for _, want := range []string{"SLUG", "api-3", "ASSIGNEES", "@local"} {
		if !strings.Contains(out, want) {
			t.Fatalf("TaskList output = %q, want %q", out, want)
		}
	}
}

func TestTaskInfoFormatsEmailAssigneeWithAtPrefix(t *testing.T) {
	var buf bytes.Buffer
	email := "alice@example.com"
	project := "api"
	projectSeq := int64(3)
	TaskInfo(&buf, task.Task{
		UUID:        "12345678-1234-1234-1234-123456789abc",
		Description: "assigned task",
		Project:     &project,
		ProjectSeq:  &projectSeq,
		Assignees: []task.AssigneeInfo{{
			UserID: "user-alice",
			Email:  &email,
		}},
	})

	out := buf.String()
	for _, want := range []string{"Task slug:", "api-3", "@alice@example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("TaskInfo output = %q, want %q", out, want)
		}
	}
}
