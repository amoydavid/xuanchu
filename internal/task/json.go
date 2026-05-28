package task

import (
	"encoding/json"
	"io"
	"time"
)

type JSONAnnotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

type JSONTask struct {
	UUID        string           `json:"uuid"`
	Description string           `json:"description"`
	Status      string           `json:"status"`
	Entry       string           `json:"entry"`
	Modified    string           `json:"modified"`
	End         *string          `json:"end,omitempty"`
	Due         *string          `json:"due,omitempty"`
	Project     *string          `json:"project,omitempty"`
	Priority    *string          `json:"priority,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Start       *string          `json:"start,omitempty"`
	Wait        *string          `json:"wait,omitempty"`
	Scheduled   *string          `json:"scheduled,omitempty"`
	Until       *string          `json:"until,omitempty"`
	Annotations []JSONAnnotation `json:"annotations,omitempty"`
	Depends     []string         `json:"depends,omitempty"`
	Recur       *string          `json:"recur,omitempty"`
	Parent      *string          `json:"parent,omitempty"`
	Mask        *string          `json:"mask,omitempty"`
	IMask       *int             `json:"imask,omitempty"`
}

func (t JSONTask) MarshalJSON() ([]byte, error) {
	type alias JSONTask
	wire := map[string]any{
		"uuid":        t.UUID,
		"description": t.Description,
		"status":      t.Status,
		"entry":       t.Entry,
		"modified":    t.Modified,
	}
	if t.End != nil {
		wire["end"] = t.End
	}
	if t.Due != nil {
		wire["due"] = t.Due
	}
	if t.Project != nil {
		wire["project"] = t.Project
	}
	if t.Priority != nil {
		wire["priority"] = t.Priority
	}
	if t.Tags != nil {
		wire["tags"] = t.Tags
	}
	if t.Start != nil {
		wire["start"] = t.Start
	}
	if t.Wait != nil {
		wire["wait"] = t.Wait
	}
	if t.Scheduled != nil {
		wire["scheduled"] = t.Scheduled
	}
	if t.Until != nil {
		wire["until"] = t.Until
	}
	if t.Annotations != nil {
		wire["annotations"] = t.Annotations
	}
	if t.Depends != nil {
		wire["depends"] = t.Depends
	}
	if t.Recur != nil {
		wire["recur"] = t.Recur
	}
	if t.Parent != nil {
		wire["parent"] = t.Parent
	}
	if t.Mask != nil {
		wire["mask"] = t.Mask
	}
	if t.IMask != nil {
		wire["imask"] = t.IMask
	}
	return json.Marshal(wire)
}

func ToJSON(tsk Task) JSONTask {
	return JSONTask{
		UUID: tsk.UUID, Description: tsk.Description, Status: tsk.Status,
		Entry: formatUnix(tsk.Entry), Modified: formatUnix(tsk.Modified),
		End: formatUnixPtr(tsk.End), Due: formatUnixPtr(tsk.Due),
		Project: tsk.Project, Priority: tsk.Priority, Tags: tsk.Tags,
		Start:     formatUnixPtr(tsk.Start),
		Wait:      formatUnixPtr(tsk.Wait),
		Scheduled: formatUnixPtr(tsk.Scheduled),
		Until:     formatUnixPtr(tsk.Until),
		Annotations: func() []JSONAnnotation {
			if tsk.Annotations == nil {
				return nil
			}
			out := make([]JSONAnnotation, len(tsk.Annotations))
			for i, a := range tsk.Annotations {
				out[i] = JSONAnnotation{Entry: formatUnix(a.Entry), Description: a.Description}
			}
			return out
		}(),
		Depends: tsk.Depends,
		Recur:   tsk.Recur,
		Parent:  tsk.Parent,
		Mask:    tsk.Mask,
		IMask:   tsk.IMask,
	}
}

func FromJSON(dto JSONTask) Task {
	return Task{
		UUID:        dto.UUID,
		Description: dto.Description,
		Status:      dto.Status,
		Entry:       parseUnixString(dto.Entry),
		Modified:    parseUnixString(dto.Modified),
		End:         parseUnixStringPtr(dto.End),
		Due:         parseUnixStringPtr(dto.Due),
		Project:     dto.Project,
		Priority:    dto.Priority,
		Tags:        dto.Tags,
		Start:       parseUnixStringPtr(dto.Start),
		Wait:        parseUnixStringPtr(dto.Wait),
		Scheduled:   parseUnixStringPtr(dto.Scheduled),
		Until:       parseUnixStringPtr(dto.Until),
		Annotations: func() []Annotation {
			if dto.Annotations == nil {
				return nil
			}
			out := make([]Annotation, len(dto.Annotations))
			for i, a := range dto.Annotations {
				out[i] = Annotation{Entry: parseUnixString(a.Entry), Description: a.Description}
			}
			return out
		}(),
		Depends: dto.Depends,
		Recur:   dto.Recur,
		Parent:  dto.Parent,
		Mask:    dto.Mask,
		IMask:   dto.IMask,
	}
}

func formatUnix(sec int64) string {
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func formatUnixPtr(sec *int64) *string {
	if sec == nil {
		return nil
	}
	value := formatUnix(*sec)
	return &value
}

func parseUnixString(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func parseUnixStringPtr(s *string) *int64 {
	if s == nil {
		return nil
	}
	v := parseUnixString(*s)
	return &v
}

func MarshalJSONTasks(tasks []JSONTask) ([]byte, error) {
	return json.Marshal(tasks)
}

func UnmarshalJSONTasks(r io.Reader, tasks *[]JSONTask) error {
	return json.NewDecoder(r).Decode(tasks)
}
