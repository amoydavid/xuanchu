package task

import (
	"errors"
	"sort"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/recurrence"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
	StatusWaiting   = "waiting"
	StatusRecurring = "recurring"
)

type Annotation struct {
	ID          string
	Entry       int64
	Description string
}

type UDAValue struct {
	Name   string
	Raw    string
	Type   string
	Orphan bool
}

type ExternalIDInfo struct {
	Provider   string
	ExternalID string
}

type UserInfo struct {
	ID          string
	Name        string
	Email       *string
	ExternalIDs []ExternalIDInfo
}

type AssigneeInfo struct {
	UserID      string
	Name        string
	Email       *string
	ExternalIDs []ExternalIDInfo
}

type TaskLinkInfo struct {
	ID        string
	Type      string
	URL       string
	Title     string
	CreatedAt int64
	CreatedBy UserInfo
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
	ProjectID   *string
	ProjectSeq  *int64
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
	Assignees   []AssigneeInfo
	Links       []TaskLinkInfo
	UDAs        map[string]UDAValue
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

		_ = i
	}
	for _, a := range t.Assignees {
		if strings.TrimSpace(a.UserID) == "" {
			return errors.New("assignee user_id is required")
		}
	}
	for _, d := range t.Depends {
		if strings.TrimSpace(d) == "" {
			return errors.New("dependency must not be empty")
		}
	}
	for name, value := range t.UDAs {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\n\r") {
			return errors.New("UDA name is invalid")
		}
		if value.Raw != "" && strings.ContainsAny(value.Raw, "\n\r") {
			return errors.New("UDA value must not contain newlines")
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

func SortAssigneeInfos(assignees []AssigneeInfo) {
	sort.Slice(assignees, func(i, j int) bool {
		if assignees[i].Name != assignees[j].Name {
			return assignees[i].Name < assignees[j].Name
		}
		leftEmail := ""
		if assignees[i].Email != nil {
			leftEmail = *assignees[i].Email
		}
		rightEmail := ""
		if assignees[j].Email != nil {
			rightEmail = *assignees[j].Email
		}
		if leftEmail != rightEmail {
			return leftEmail < rightEmail
		}
		return assignees[i].UserID < assignees[j].UserID
	})
}
