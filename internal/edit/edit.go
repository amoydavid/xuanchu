package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

type EditableTask struct {
	ID          string                `json:"id"`
	UUID        *string               `json:"uuid"`
	Entry       *string               `json:"entry"`
	Title       string                `json:"title"`
	Description *string               `json:"description,omitempty"`
	Status      string                `json:"status"`
	Project     *string               `json:"project,omitempty"`
	Priority    *string               `json:"priority,omitempty"`
	Due         *string               `json:"due,omitempty"`
	Wait        *string               `json:"wait,omitempty"`
	Scheduled   *string               `json:"scheduled,omitempty"`
	Until       *string               `json:"until,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Annotations []task.JSONAnnotation `json:"annotations,omitempty"`
	Depends     []string              `json:"depends,omitempty"`
	Parent      *string               `json:"parent,omitempty"`
}

func FromTask(tsk task.Task) EditableTask {
	dto := task.ToJSON(tsk)
	return EditableTask{
		ID:          dto.UUID,
		UUID:        stringPtr(dto.UUID),
		Entry:       stringPtr(dto.Entry),
		Title:       dto.Title,
		Description: dto.Description,
		Status:      dto.Status,
		Project:     dto.Project,
		Priority:    dto.Priority,
		Due:         dto.Due,
		Wait:        dto.Wait,
		Scheduled:   dto.Scheduled,
		Until:       dto.Until,
		Tags:        dto.Tags,
		Annotations: dto.Annotations,
		Depends:     dto.Depends,
		Parent:      dto.Parent,
	}
}

func Parse(data []byte, original EditableTask) (EditableTask, error) {
	var edited EditableTask
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&edited); err != nil {
		return EditableTask{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return EditableTask{}, fmt.Errorf("multiple JSON values are not allowed")
		}
		return EditableTask{}, err
	}
	if edited.ID != original.ID {
		return EditableTask{}, fmt.Errorf("id is immutable")
	}
	if !equalStringPtr(edited.UUID, original.UUID) {
		return EditableTask{}, fmt.Errorf("uuid is immutable")
	}
	if !equalStringPtr(edited.Entry, original.Entry) {
		return EditableTask{}, fmt.Errorf("entry is immutable")
	}
	return edited, nil
}

func Apply(edited EditableTask) (task.EditableFields, error) {
	due, err := parseTimePtr("due", edited.Due)
	if err != nil {
		return task.EditableFields{}, err
	}
	wait, err := parseTimePtr("wait", edited.Wait)
	if err != nil {
		return task.EditableFields{}, err
	}
	scheduled, err := parseTimePtr("scheduled", edited.Scheduled)
	if err != nil {
		return task.EditableFields{}, err
	}
	until, err := parseTimePtr("until", edited.Until)
	if err != nil {
		return task.EditableFields{}, err
	}
	out := task.EditableFields{
		Title: edited.Title, Description: edited.Description, Status: edited.Status,
		Project: edited.Project, Priority: edited.Priority,
		Due: due, Wait: wait, Scheduled: scheduled, Until: until,
		Tags: edited.Tags, Depends: edited.Depends, Parent: edited.Parent,
	}
	if edited.Annotations == nil {
		out.Annotations = nil
	} else {
		out.Annotations = make([]task.Annotation, len(edited.Annotations))
		for i, annotation := range edited.Annotations {
			entry, err := parseTime(fmt.Sprintf("annotations[%d].entry", i), annotation.Entry)
			if err != nil {
				return task.EditableFields{}, err
			}
			out.Annotations[i] = task.Annotation{
				ID:          annotation.ID,
				Entry:       entry,
				Description: annotation.Description,
			}
		}
	}
	return out, out.Validate()
}

func parseTimePtr(field string, value *string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := parseTime(field, *value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseTime(field, value string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}
	return parsed.Unix(), nil
}

func stringPtr(value string) *string {
	copy := value
	return &copy
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
