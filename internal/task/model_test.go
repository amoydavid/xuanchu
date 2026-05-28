package task

import "testing"

func TestValidateRequiresDescription(t *testing.T) {
	tsk := Task{Description: "   ", Status: StatusPending}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateRejectsInvalidPriority(t *testing.T) {
	priority := "X"
	tsk := Task{Description: "hello", Status: StatusPending, Priority: &priority}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestCompleteSetsStatusAndEnd(t *testing.T) {
	now := int64(100)
	tsk := Task{Description: "hello", Status: StatusPending}
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
