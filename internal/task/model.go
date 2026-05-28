package task

import (
	"errors"
	"strings"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
)

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
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Description) == "" {
		return errors.New("description is required")
	}
	switch t.Status {
	case StatusPending, StatusCompleted, StatusDeleted:
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
	return nil
}

func (t *Task) Complete(now int64) {
	t.Status = StatusCompleted
	t.End = &now
	t.Modified = now
}

func (t *Task) Delete(now int64) {
	t.Status = StatusDeleted
	t.End = &now
	t.Modified = now
}
