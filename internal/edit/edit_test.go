package edit

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestParseEditableTaskRejectsUUIDChange(t *testing.T) {
	original := EditableTask{ID: "u1", UUID: stringPointer("u1"), Entry: stringPointer("1970-01-01T00:00:01Z"), Title: "task", Status: "pending"}
	input := `{"id":"u1","uuid":"u2","entry":"1970-01-01T00:00:01Z","title":"task","status":"pending"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want immutable uuid error")
	}
}

func TestParseEditableTaskRejectsEntryChange(t *testing.T) {
	original := EditableTask{ID: "u1", UUID: stringPointer("u1"), Entry: stringPointer("1970-01-01T00:00:01Z"), Title: "task", Status: "pending"}
	input := `{"id":"u1","uuid":"u1","entry":"1970-01-01T00:00:02Z","title":"task","status":"pending"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want immutable entry error")
	}
}

func TestParseEditableTaskUpdatesTitle(t *testing.T) {
	original := EditableTask{ID: "u1", UUID: stringPointer("u1"), Entry: stringPointer("1970-01-01T00:00:01Z"), Title: "task", Status: "pending"}
	got, err := Parse([]byte(`{"id":"u1","uuid":"u1","entry":"1970-01-01T00:00:01Z","title":"new","status":"pending"}`), original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Title != "new" {
		t.Fatalf("Title = %q", got.Title)
	}
}

func TestParseEditableTaskRejectsRecurrenceIdentityFields(t *testing.T) {
	original := EditableTask{ID: "u1", UUID: stringPointer("u1"), Entry: stringPointer("1970-01-01T00:00:01Z"), Title: "task", Status: "pending"}
	input := `{"id":"u1","uuid":"u1","entry":"1970-01-01T00:00:01Z","title":"task","status":"pending","series_id":"forged"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want unknown recurrence identity field error")
	}
}

func TestParseProjectedEditableTaskKeepsNullableIdentityImmutable(t *testing.T) {
	original := EditableTask{ID: "occ:series-1:100", UUID: nil, Entry: nil, Title: "巡检", Status: "pending"}
	input := `{"id":"occ:series-1:100","uuid":null,"entry":null,"title":"巡检","status":"pending"}`
	if _, err := Parse([]byte(input), original); err != nil {
		t.Fatalf("Parse(projected) error = %v", err)
	}
	forged := `{"id":"occ:series-1:100","uuid":"forged","entry":null,"title":"巡检","status":"pending"}`
	if _, err := Parse([]byte(forged), original); err == nil {
		t.Fatal("Parse(projected forged uuid) error = nil")
	}
}

func TestApplyRejectsInvalidDate(t *testing.T) {
	due := "not-a-date"
	_, err := Apply(EditableTask{
		ID:     "u1",
		UUID:   stringPointer("u1"),
		Entry:  stringPointer("1970-01-01T00:00:01Z"),
		Title:  "task",
		Status: "pending",
		Due:    &due,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want invalid due error")
	}
}

func TestApplyRejectsInvalidAnnotationEntry(t *testing.T) {
	_, err := Apply(EditableTask{
		ID:          "u1",
		UUID:        stringPointer("u1"),
		Entry:       stringPointer("1970-01-01T00:00:01Z"),
		Title:       "task",
		Status:      "pending",
		Annotations: []task.JSONAnnotation{{Entry: "not-a-date", Description: "note"}},
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want invalid annotation entry error")
	}
}

func TestApplyProjectedEditableTaskReturnsValidatedFields(t *testing.T) {
	due := "2030-01-01T00:00:00Z"
	fields, err := Apply(EditableTask{
		ID: "occ:series-1:100", Title: "本次巡检", Status: task.StatusPending,
		Due: &due, Tags: []string{"ops"},
	})
	if err != nil {
		t.Fatalf("Apply(projected) error = %v", err)
	}
	if fields.Title != "本次巡检" || fields.Due == nil || *fields.Due != 1893456000 {
		t.Fatalf("fields = %#v", fields)
	}
}

func stringPointer(value string) *string {
	return &value
}
