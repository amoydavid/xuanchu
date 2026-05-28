package task

import "time"

type JSONTask struct {
	UUID        string   `json:"uuid"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Entry       string   `json:"entry"`
	Modified    string   `json:"modified"`
	End         *string  `json:"end,omitempty"`
	Due         *string  `json:"due,omitempty"`
	Project     *string  `json:"project,omitempty"`
	Priority    *string  `json:"priority,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func ToJSON(tsk Task) JSONTask {
	return JSONTask{
		UUID: tsk.UUID, Description: tsk.Description, Status: tsk.Status,
		Entry: formatUnix(tsk.Entry), Modified: formatUnix(tsk.Modified),
		End: formatUnixPtr(tsk.End), Due: formatUnixPtr(tsk.Due),
		Project: tsk.Project, Priority: tsk.Priority, Tags: tsk.Tags,
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
