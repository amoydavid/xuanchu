package edit

import (
	"encoding/json"
	"fmt"

	"github.com/dajee/taskg/internal/task"
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
	out.Description = edited.Description
	out.Status = edited.Status
	out.Project = edited.Project
	out.Priority = edited.Priority
	out.Due = parseTimePtr(edited.Due)
	out.Wait = parseTimePtr(edited.Wait)
	out.Scheduled = parseTimePtr(edited.Scheduled)
	out.Until = parseTimePtr(edited.Until)
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
			out.Annotations[i] = task.Annotation{
				Entry:       parseTime(annotation.Entry),
				Description: annotation.Description,
			}
		}
	}
	return out, out.Validate()
}

func parseTimePtr(value *string) *int64 {
	if value == nil {
		return nil
	}
	parsed := parseTime(*value)
	return &parsed
}

func parseTime(value string) int64 {
	return task.FromJSON(task.JSONTask{Entry: value}).Entry
}
