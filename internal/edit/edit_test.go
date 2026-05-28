package edit

import "testing"

func TestParseEditableTaskRejectsUUIDChange(t *testing.T) {
	original := EditableTask{UUID: "u1", Entry: "1970-01-01T00:00:01Z", Description: "task", Status: "pending"}
	input := `{"uuid":"u2","entry":"1970-01-01T00:00:01Z","description":"task","status":"pending"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want immutable uuid error")
	}
}

func TestParseEditableTaskRejectsEntryChange(t *testing.T) {
	original := EditableTask{UUID: "u1", Entry: "1970-01-01T00:00:01Z", Description: "task", Status: "pending"}
	input := `{"uuid":"u1","entry":"1970-01-01T00:00:02Z","description":"task","status":"pending"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want immutable entry error")
	}
}

func TestParseEditableTaskUpdatesDescription(t *testing.T) {
	original := EditableTask{UUID: "u1", Entry: "1970-01-01T00:00:01Z", Description: "task", Status: "pending"}
	got, err := Parse([]byte(`{"uuid":"u1","entry":"1970-01-01T00:00:01Z","description":"new","status":"pending"}`), original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Description != "new" {
		t.Fatalf("Description = %q", got.Description)
	}
}
