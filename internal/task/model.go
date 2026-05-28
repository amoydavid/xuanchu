package task

import (
	"errors"
	"strings"

	"github.com/dajee/taskg/internal/recurrence"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
	StatusWaiting   = "waiting"
	StatusRecurring = "recurring"
)

type Annotation struct {
	Entry       int64
	Description string
}

type Task struct {
	UUID        string
	WorkspaceID string
	Description string
	Status      string
	Entry       int64
	Modified    int64
	End         *int64
	Due         *int64
	Project     *string
	Priority    *string
	Tags        []string
	Start       *int64
	Wait        *int64
	Scheduled   *int64
	Until       *int64
	Annotations []Annotation
	Depends     []string
	Recur       *string
	Parent      *string
	Mask        *string
	IMask       *int
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Description) == "" {
		return errors.New("description is required")
	}
	switch t.Status {
	case StatusPending, StatusCompleted, StatusDeleted, StatusWaiting, StatusRecurring:
	default:
		return errors.New("invalid status")
	}
	if t.Priority != nil {
		switch *t.Priority {
		case "H", "M", "L":
		default:
			return errors.New("invalid priority")
		}
	}
	for i, a := range t.Annotations {
		trimmed := strings.TrimSpace(a.Description)
		if trimmed == "" {
			return errors.New("annotation description is required")
		}
		if strings.ContainsAny(a.Description, "\n\r") {
			return errors.New("annotation description must not contain newlines")
		}
		_ = i
	}
	for _, d := range t.Depends {
		if strings.TrimSpace(d) == "" {
			return errors.New("dependency must not be empty")
		}
	}
	if t.Recur != nil {
		if err := recurrence.Validate(*t.Recur); err != nil {
			return err
		}
	}
	if t.Status == StatusRecurring {
		if t.Recur == nil {
			return errors.New("recurring task requires recur")
		}
		if t.Due == nil {
			return errors.New("recurring task requires due")
		}
	}
	return nil
}

func (t *Task) Complete(now int64) {
	t.Status = StatusCompleted
	t.End = &now
	t.Start = nil
	t.Modified = now
}

func (t *Task) Delete(now int64) {
	t.Status = StatusDeleted
	t.End = &now
	t.Start = nil
	t.Modified = now
}

func (t *Task) StartTask(now int64) {
	t.Status = StatusPending
	t.Start = &now
	t.Wait = nil
	t.Modified = now
}

func (t *Task) StopTask(now int64) {
	t.Start = nil
	t.Modified = now
}
