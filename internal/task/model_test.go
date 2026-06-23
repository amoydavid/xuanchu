package task

import "testing"

func TestValidateRequiresTitle(t *testing.T) {
	tsk := Task{Title: "   ", Status: StatusPending}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateAllowsEmptyDescription(t *testing.T) {
	tsk := Task{Title: "hello", Status: StatusPending}
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	empty := ""
	tsk.Description = &empty
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate(empty detailed description) error = %v, want nil", err)
	}
}

func TestValidateRejectsInvalidPriority(t *testing.T) {
	priority := "X"
	tsk := Task{Title: "hello", Status: StatusPending, Priority: &priority}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateAllowsM2StatusesAndFields(t *testing.T) {
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Title: "task", Status: StatusWaiting, Entry: 1, Modified: 1}
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate(waiting) error = %v", err)
	}
}

func TestValidateRecurringRequiresRecurAndDue(t *testing.T) {
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Title: "parent", Status: StatusRecurring, Entry: 1, Modified: 1}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want recurring validation error")
	}

	recur := "weekly"
	tsk.Recur = &recur
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want due validation error")
	}

	due := int64(100)
	tsk.Due = &due
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate(recurring with recur/due) error = %v", err)
	}
}

func TestValidateRejectsUnsupportedRecurrence(t *testing.T) {
	recur := "fortnightly"
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Title: "task", Status: StatusPending, Entry: 1, Modified: 1, Recur: &recur}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want recurrence validation error")
	}
}

func TestAnnotationValidateRejectsEmptyDescription(t *testing.T) {
	tsk := Task{
		UUID: "u1", WorkspaceID: "w1", Title: "task", Status: StatusPending, Entry: 1, Modified: 1,
		Annotations: []Annotation{{Entry: 10, Description: "  "}},
	}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want annotation error")
	}
}

func TestStartStopHelpers(t *testing.T) {
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Title: "task", Status: StatusPending, Entry: 1, Modified: 1}
	tsk.StartTask(100)
	if tsk.Start == nil || *tsk.Start != 100 || tsk.Modified != 100 {
		t.Fatalf("StartTask did not set start/modified: %#v", tsk)
	}
	tsk.StopTask(200)
	if tsk.Start != nil || tsk.Modified != 200 {
		t.Fatalf("StopTask did not clear start/update modified: %#v", tsk)
	}
}

func TestCompleteSetsStatusAndEnd(t *testing.T) {
	now := int64(100)
	tsk := Task{Title: "hello", Status: StatusPending}
	tsk.Complete(now)
	if tsk.Status != StatusCompleted {
		t.Fatalf("Status = %q", tsk.Status)
	}
	if tsk.End == nil || *tsk.End != now {
		t.Fatalf("End = %#v, want %d", tsk.End, now)
	}
	if tsk.Modified != now {
		t.Fatalf("Modified = %d, want %d", tsk.Modified, now)
	}
}
