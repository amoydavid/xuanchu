package edit

import (
	"encoding/json"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

type EditableTask struct {
	UUID        string                `json:"uuid"`
	Entry       string                `json:"entry"`
	Description string                `json:"description"`
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
	Recur       *string               `json:"recur,omitempty"`
	Parent      *string               `json:"parent,omitempty"`
	Mask        *string               `json:"mask,omitempty"`
	IMask       *int                  `json:"imask,omitempty"`
}

func FromTask(tsk task.Task) EditableTask {
	dto := task.ToJSON(tsk)
	return EditableTask{
		UUID:        dto.UUID,
		Entry:       dto.Entry,
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
		Recur:       dto.Recur,
		Parent:      dto.Parent,
		Mask:        dto.Mask,
		IMask:       dto.IMask,
	}
}

func Parse(data []byte, original EditableTask) (EditableTask, error) {
	var edited EditableTask
	if err := json.Unmarshal(data, &edited); err != nil {
		return EditableTask{}, err
	}
	if edited.UUID != original.UUID {
		return EditableTask{}, fmt.Errorf("uuid is immutable")
	}
	if edited.Entry != original.Entry {
		return EditableTask{}, fmt.Errorf("entry is immutable")
	}
	return edited, nil
}

func Apply(original task.Task, edited EditableTask) (task.Task, error) {
	out := original
	due, err := parseTimePtr("due", edited.Due)
	if err != nil {
		return task.Task{}, err
	}
	wait, err := parseTimePtr("wait", edited.Wait)
	if err != nil {
		return task.Task{}, err
	}
	scheduled, err := parseTimePtr("scheduled", edited.Scheduled)
	if err != nil {
		return task.Task{}, err
	}
	until, err := parseTimePtr("until", edited.Until)
	if err != nil {
		return task.Task{}, err
	}
	out.Description = edited.Description
	out.Status = edited.Status
	out.Project = edited.Project
	out.Priority = edited.Priority
	out.Due = due
	out.Wait = wait
	out.Scheduled = scheduled
	out.Until = until
	out.Tags = edited.Tags
	out.Depends = edited.Depends
	out.Recur = edited.Recur
	out.Parent = edited.Parent
	out.Mask = edited.Mask
	out.IMask = edited.IMask
	if edited.Annotations == nil {
		out.Annotations = nil
	} else {
		out.Annotations = make([]task.Annotation, len(edited.Annotations))
		for i, annotation := range edited.Annotations {
			entry, err := parseTime(fmt.Sprintf("annotations[%d].entry", i), annotation.Entry)
			if err != nil {
				return task.Task{}, err
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
