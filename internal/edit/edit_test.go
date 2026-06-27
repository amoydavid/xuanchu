package edit

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestParseEditableTaskRejectsUUIDChange(t *testing.T) {
	original := EditableTask{UUID: "u1", Entry: "1970-01-01T00:00:01Z", Title: "task", Status: "pending"}
	input := `{"uuid":"u2","entry":"1970-01-01T00:00:01Z","title":"task","status":"pending"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want immutable uuid error")
	}
}

func TestParseEditableTaskRejectsEntryChange(t *testing.T) {
	original := EditableTask{UUID: "u1", Entry: "1970-01-01T00:00:01Z", Title: "task", Status: "pending"}
	input := `{"uuid":"u1","entry":"1970-01-01T00:00:02Z","title":"task","status":"pending"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want immutable entry error")
	}
}

func TestParseEditableTaskUpdatesTitle(t *testing.T) {
	original := EditableTask{UUID: "u1", Entry: "1970-01-01T00:00:01Z", Title: "task", Status: "pending"}
	got, err := Parse([]byte(`{"uuid":"u1","entry":"1970-01-01T00:00:01Z","title":"new","status":"pending"}`), original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Title != "new" {
		t.Fatalf("Title = %q", got.Title)
	}
}

func TestApplyRejectsInvalidDate(t *testing.T) {
	due := "not-a-date"
	_, err := Apply(taskFixture(), EditableTask{
		UUID:   "u1",
		Entry:  "1970-01-01T00:00:01Z",
		Title:  "task",
		Status: "pending",
		Due:    &due,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want invalid due error")
	}
}

func TestApplyRejectsInvalidAnnotationEntry(t *testing.T) {
	_, err := Apply(taskFixture(), EditableTask{
		UUID:        "u1",
		Entry:       "1970-01-01T00:00:01Z",
		Title:       "task",
		Status:      "pending",
		Annotations: []task.JSONAnnotation{{Entry: "not-a-date", Description: "note"}},
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want invalid annotation entry error")
	}
}

func taskFixture() task.Task {
	return task.Task{
		UUID:        "u1",
		WorkspaceID: "w1",
		Title:       "task",
		Status:      task.StatusPending,
		Entry:       1,
		Modified:    1,
	}
}
