package render

import (
	"bytes"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestTaskListShowsAssignees(t *testing.T) {
	var buf bytes.Buffer
	TaskList(&buf, []task.Task{{
		UUID:        "12345678-1234-1234-1234-123456789abc",
		Description: "assigned task",
		Assignees: []task.AssigneeInfo{{
			UserID: "user-local",
			Name:   "local",
		}},
	}})

	out := buf.String()
	if !strings.Contains(out, "ASSIGNEES") || !strings.Contains(out, "@local") {
		t.Fatalf("TaskList output = %q, want assignees column with @local", out)
	}
}

func TestTaskInfoFormatsEmailAssigneeWithAtPrefix(t *testing.T) {
	var buf bytes.Buffer
	email := "alice@example.com"
	TaskInfo(&buf, task.Task{
		UUID:        "12345678-1234-1234-1234-123456789abc",
		Description: "assigned task",
		Assignees: []task.AssigneeInfo{{
			UserID: "user-alice",
			Email:  &email,
		}},
	})

	out := buf.String()
	if !strings.Contains(out, "@alice@example.com") {
		t.Fatalf("TaskInfo output = %q, want email assignee with @ prefix", out)
	}
}
