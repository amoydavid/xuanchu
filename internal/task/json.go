package task

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type JSONAnnotation struct {
	ID          string `json:"id,omitempty"`
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

type JSONExternalID struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type JSONUserInfo struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	DisplayName string           `json:"display_name,omitempty"`
	Email       *string          `json:"email,omitempty"`
	ExternalIDs []JSONExternalID `json:"external_ids,omitempty"`
}

type JSONAssignee struct {
	UserID      string           `json:"user_id,omitempty"`
	Name        string           `json:"name,omitempty"`
	DisplayName string           `json:"display_name,omitempty"`
	Email       *string          `json:"email,omitempty"`
	ExternalIDs []JSONExternalID `json:"external_ids,omitempty"`
}

type JSONTaskLink struct {
	ID        string       `json:"id"`
	Type      string       `json:"type"`
	URL       string       `json:"url"`
	Title     string       `json:"title,omitempty"`
	CreatedAt string       `json:"created_at"`
	CreatedBy JSONUserInfo `json:"created_by"`
}

type JSONTask struct {
	UUID        string           `json:"uuid"`
	Title       string           `json:"title"`
	Description *string          `json:"description,omitempty"`
	Status      string           `json:"status"`
	Entry       string           `json:"entry"`
	Modified    string           `json:"modified"`
	End         *string          `json:"end,omitempty"`
	Due         *string          `json:"due,omitempty"`
	Project     *string          `json:"project,omitempty"`
	TaskSlug    *string          `json:"task_slug,omitempty"`
	Priority    *string          `json:"priority,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Start       *string          `json:"start,omitempty"`
	Wait        *string          `json:"wait,omitempty"`
	Scheduled   *string          `json:"scheduled,omitempty"`
	Until       *string          `json:"until,omitempty"`
	Annotations []JSONAnnotation `json:"annotations,omitempty"`
	Depends     []string         `json:"depends,omitempty"`
	DependsInfo []JSONTaskRef    `json:"depends_info,omitempty"`
	Recur       *string          `json:"recur,omitempty"`
	Parent      *string          `json:"parent,omitempty"`
	ParentInfo  *JSONTaskRef     `json:"parent_info,omitempty"`
	// BlockedByInfo 是被当前任务阻塞的任务列表（反向依赖），供 UI 展示「阻塞了」关系。
	BlockedByInfo []JSONTaskRef       `json:"blocked_by_info,omitempty"`
	Mask          *string             `json:"mask,omitempty"`
	IMask         *int                `json:"imask,omitempty"`
	Assignees     []JSONAssignee      `json:"assignees,omitempty"`
	Links         []JSONTaskLink      `json:"links,omitempty"`
	UDAs          map[string]UDAValue `json:"-"`
}

// JSONTaskRef 是任务的轻量引用，用于 depends_info/parent_info，
// 把裸 UUID 展开为人类可读的标题 + 稳定短标识，便于 UI 展示和跳转。
type JSONTaskRef struct {
	UUID     string  `json:"uuid"`
	Title    string  `json:"title"`
	TaskSlug *string `json:"task_slug,omitempty"`
}

func (t JSONTask) MarshalJSON() ([]byte, error) {
	type alias JSONTask
	wire := map[string]any{
		"uuid":     t.UUID,
		"title":    t.Title,
		"status":   t.Status,
		"entry":    t.Entry,
		"modified": t.Modified,
	}
	if t.Description != nil {
		wire["description"] = t.Description
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
	if t.TaskSlug != nil {
		wire["task_slug"] = t.TaskSlug
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
	if t.DependsInfo != nil {
		wire["depends_info"] = t.DependsInfo
	}
	if t.Recur != nil {
		wire["recur"] = t.Recur
	}
	if t.Parent != nil {
		wire["parent"] = t.Parent
	}
	if t.ParentInfo != nil {
		wire["parent_info"] = t.ParentInfo
	}
	if t.BlockedByInfo != nil {
		wire["blocked_by_info"] = t.BlockedByInfo
	}
	if t.Mask != nil {
		wire["mask"] = t.Mask
	}
	if t.IMask != nil {
		wire["imask"] = t.IMask
	}
	if t.Assignees != nil {
		wire["assignees"] = t.Assignees
	}
	if t.Links != nil {
		wire["links"] = t.Links
	}
	reserved := reservedJSONFields()
	for name, value := range t.UDAs {
		if _, ok := reserved[name]; ok {
			continue
		}
		wire[name] = value.Raw
	}
	return json.Marshal(wire)
}

func (t *JSONTask) UnmarshalJSON(data []byte) error {
	var core struct {
		UUID        string           `json:"uuid"`
		Title       string           `json:"title"`
		Description *string          `json:"description,omitempty"`
		Status      string           `json:"status"`
		Entry       string           `json:"entry"`
		Modified    string           `json:"modified"`
		End         *string          `json:"end,omitempty"`
		Due         *string          `json:"due,omitempty"`
		Project     *string          `json:"project,omitempty"`
		TaskSlug    *string          `json:"task_slug,omitempty"`
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
	if err := json.Unmarshal(data, &core); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for field := range reservedJSONFields() {
		if _, ok := raw[field]; ok {
			return fmt.Errorf("%s is reserved; use project:<slug> to modify project", field)
		}
	}
	if assigneesRaw, ok := raw["assignees"]; ok {
		assignees, err := unmarshalJSONAssignees(assigneesRaw)
		if err != nil {
			return err
		}
		t.Assignees = assignees
	} else {
		t.Assignees = nil
	}
	if linksRaw, ok := raw["links"]; ok {
		var links []JSONTaskLink
		if err := json.Unmarshal(linksRaw, &links); err != nil {
			return fmt.Errorf("invalid links: %w", err)
		}
		t.Links = links
	} else {
		t.Links = nil
	}
	for _, key := range coreJSONFields() {
		delete(raw, key)
	}
	if len(raw) > 0 {
		udas := map[string]UDAValue{}
		for key, value := range raw {
			text, err := rawToUDAString(value)
			if err != nil {
				return err
			}
			udas[key] = UDAValue{Name: key, Raw: text, Orphan: true}
		}
		t.UDAs = udas
	} else {
		t.UDAs = nil
	}
	t.UUID = core.UUID
	t.Title = core.Title
	t.Description = core.Description
	t.Status = core.Status
	t.Entry = core.Entry
	t.Modified = core.Modified
	t.End = core.End
	t.Due = core.Due
	t.Project = core.Project
	t.TaskSlug = core.TaskSlug
	t.Priority = core.Priority
	t.Tags = core.Tags
	t.Start = core.Start
	t.Wait = core.Wait
	t.Scheduled = core.Scheduled
	t.Until = core.Until
	t.Annotations = core.Annotations
	t.Depends = core.Depends
	t.Recur = core.Recur
	t.Parent = core.Parent
	t.Mask = core.Mask
	t.IMask = core.IMask
	return nil
}

func ToJSON(tsk Task) JSONTask {
	var taskSlug *string
	if tsk.Project != nil && tsk.ProjectSeq != nil {
		value := fmt.Sprintf("%s-%d", *tsk.Project, *tsk.ProjectSeq)
		taskSlug = &value
	}
	return JSONTask{
		UUID: tsk.UUID, Title: tsk.Title, Description: tsk.Description, Status: tsk.Status,
		Entry: formatUnix(tsk.Entry), Modified: formatUnix(tsk.Modified),
		End: formatUnixPtr(tsk.End), Due: formatUnixPtr(tsk.Due),
		Project: tsk.Project, TaskSlug: taskSlug, Priority: tsk.Priority, Tags: tsk.Tags,
		Start:       formatUnixPtr(tsk.Start),
		Wait:        formatUnixPtr(tsk.Wait),
		Scheduled:   formatUnixPtr(tsk.Scheduled),
		Until:       formatUnixPtr(tsk.Until),
		Annotations: AnnotationsToJSON(tsk.Annotations),
		Depends:     tsk.Depends,
		Recur:       tsk.Recur,
		Parent:      tsk.Parent,
		Mask:        tsk.Mask,
		IMask:       tsk.IMask,
		Assignees: func() []JSONAssignee {
			if tsk.Assignees == nil {
				return nil
			}
			out := make([]JSONAssignee, len(tsk.Assignees))
			for i, assignee := range tsk.Assignees {
				a := JSONAssignee{
					UserID:      assignee.UserID,
					Name:        assignee.Name,
					DisplayName: assignee.DisplayName,
					Email:       assignee.Email,
				}
				if len(assignee.ExternalIDs) > 0 {
					a.ExternalIDs = make([]JSONExternalID, len(assignee.ExternalIDs))
					for j, eid := range assignee.ExternalIDs {
						a.ExternalIDs[j] = JSONExternalID{Provider: eid.Provider, ExternalID: eid.ExternalID}
					}
				}
				out[i] = a
			}
			return out
		}(),
		Links: func() []JSONTaskLink {
			if tsk.Links == nil {
				return nil
			}
			out := make([]JSONTaskLink, len(tsk.Links))
			for i, link := range tsk.Links {
				out[i] = JSONTaskLink{
					ID: link.ID, Type: link.Type, URL: link.URL,
					Title: link.Title, CreatedAt: formatUnix(link.CreatedAt),
					CreatedBy: JSONUserInfo{
						ID: link.CreatedBy.ID, Name: link.CreatedBy.Name, DisplayName: link.CreatedBy.DisplayName,
						Email: link.CreatedBy.Email, ExternalIDs: externalIDsToJSON(link.CreatedBy.ExternalIDs),
					},
				}
			}
			return out
		}(),
		UDAs: tsk.UDAs,
	}
}

func FromJSON(dto JSONTask) Task {
	tsk, _ := FromJSONStrict(dto)
	return tsk
}

func FromJSONStrict(dto JSONTask) (Task, error) {
	if strings.TrimSpace(dto.Title) == "" {
		return Task{}, fmt.Errorf("title is required")
	}
	entry, err := parseUnixString("entry", dto.Entry)
	if err != nil {
		return Task{}, err
	}
	modified, err := parseUnixString("modified", dto.Modified)
	if err != nil {
		return Task{}, err
	}
	end, err := parseUnixStringPtr("end", dto.End)
	if err != nil {
		return Task{}, err
	}
	due, err := parseUnixStringPtr("due", dto.Due)
	if err != nil {
		return Task{}, err
	}
	start, err := parseUnixStringPtr("start", dto.Start)
	if err != nil {
		return Task{}, err
	}
	wait, err := parseUnixStringPtr("wait", dto.Wait)
	if err != nil {
		return Task{}, err
	}
	scheduled, err := parseUnixStringPtr("scheduled", dto.Scheduled)
	if err != nil {
		return Task{}, err
	}
	until, err := parseUnixStringPtr("until", dto.Until)
	if err != nil {
		return Task{}, err
	}
	annotations := func() []Annotation {
		if dto.Annotations == nil {
			return nil
		}
		return make([]Annotation, len(dto.Annotations))
	}()
	for i, a := range dto.Annotations {
		entry, err := parseUnixString(fmt.Sprintf("annotations[%d].entry", i), a.Entry)
		if err != nil {
			return Task{}, err
		}
		annotations[i] = Annotation{ID: a.ID, Entry: entry, Description: a.Description}
	}
	return Task{
		UUID:        dto.UUID,
		Title:       dto.Title,
		Description: dto.Description,
		Status:      dto.Status,
		Entry:       entry,
		Modified:    modified,
		End:         end,
		Due:         due,
		Project:     dto.Project,
		Priority:    dto.Priority,
		Tags:        dto.Tags,
		Start:       start,
		Wait:        wait,
		Scheduled:   scheduled,
		Until:       until,
		Annotations: annotations,
		Depends:     dto.Depends,
		Recur:       dto.Recur,
		Parent:      dto.Parent,
		Mask:        dto.Mask,
		IMask:       dto.IMask,
		Assignees: func() []AssigneeInfo {
			if dto.Assignees == nil {
				return nil
			}
			out := make([]AssigneeInfo, len(dto.Assignees))
			for i, assignee := range dto.Assignees {
				info := AssigneeInfo{
					UserID:      assignee.UserID,
					Name:        assignee.Name,
					DisplayName: assignee.DisplayName,
					Email:       assignee.Email,
				}
				if len(assignee.ExternalIDs) > 0 {
					info.ExternalIDs = make([]ExternalIDInfo, len(assignee.ExternalIDs))
					for j, eid := range assignee.ExternalIDs {
						info.ExternalIDs[j] = ExternalIDInfo{Provider: eid.Provider, ExternalID: eid.ExternalID}
					}
				}
				out[i] = info
			}
			return out
		}(),
		Links: func() []TaskLinkInfo {
			if dto.Links == nil {
				return nil
			}
			out := make([]TaskLinkInfo, len(dto.Links))
			for i, link := range dto.Links {
				createdAt, err := parseUnixString(fmt.Sprintf("links[%d].created_at", i), link.CreatedAt)
				if err != nil {
					return nil
				}
				out[i] = TaskLinkInfo{
					ID: link.ID, Type: link.Type, URL: link.URL,
					Title: link.Title, CreatedAt: createdAt,
					CreatedBy: UserInfo{
						ID: link.CreatedBy.ID, Name: link.CreatedBy.Name,
						Email: link.CreatedBy.Email, ExternalIDs: externalIDsFromJSON(link.CreatedBy.ExternalIDs),
					},
				}
			}
			return out
		}(),
		UDAs: dto.UDAs,
	}, nil
}

func coreJSONFields() []string {
	return []string{"uuid", "title", "description", "status", "entry", "modified", "end", "due", "project", "task_slug", "project_seq", "priority", "tags", "start", "wait", "scheduled", "until", "annotations", "depends", "recur", "parent", "mask", "imask", "assignees", "links"}
}

func reservedJSONFields() map[string]struct{} {
	return map[string]struct{}{
		"project_id":      {},
		"project_seq":     {},
		"depends_info":    {},
		"parent_info":     {},
		"blocked_by_info": {},
	}
}

func rawToUDAString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return fmt.Sprintf("%v", f), nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			return "true", nil
		}
		return "false", nil
	}
	compact, err := json.Marshal(json.RawMessage(raw))
	if err != nil {
		return "", err
	}
	return string(compact), nil
}

func unmarshalJSONAssignees(raw json.RawMessage) ([]JSONAssignee, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	assignees := make([]JSONAssignee, len(items))
	for i, item := range items {
		var stringValue string
		if err := json.Unmarshal(item, &stringValue); err == nil {
			assignees[i] = jsonAssigneeFromStringRef(stringValue)
			continue
		}
		var assignee JSONAssignee
		if err := json.Unmarshal(item, &assignee); err != nil {
			return nil, fmt.Errorf("invalid assignees[%d]: %w", i, err)
		}
		assignees[i] = assignee
	}
	return assignees, nil
}

func jsonAssigneeFromStringRef(ref string) JSONAssignee {
	if strings.Contains(ref, "@") {
		return JSONAssignee{Email: &ref}
	}
	return JSONAssignee{UserID: ref}
}

// AnnotationsToJSON 把注解列表序列化为 JSONAnnotation，供 GET /tasks/{ref}/annotations 等独立端点复用。
// nil 输入返回 nil（保证 omitempty 生效）。
func AnnotationsToJSON(in []Annotation) []JSONAnnotation {
	if in == nil {
		return nil
	}
	out := make([]JSONAnnotation, len(in))
	for i, a := range in {
		out[i] = JSONAnnotation{ID: a.ID, Entry: formatUnix(a.Entry), Description: a.Description}
	}
	return out
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

func parseUnixString(field, s string) (int64, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", field, s, err)
	}
	return t.Unix(), nil
}

func parseUnixStringPtr(field string, s *string) (*int64, error) {
	if s == nil {
		return nil, nil
	}
	v, err := parseUnixString(field, *s)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func MarshalJSONTasks(tasks []JSONTask) ([]byte, error) {
	return json.Marshal(tasks)
}

func UnmarshalJSONTasks(r io.Reader, tasks *[]JSONTask) error {
	return json.NewDecoder(r).Decode(tasks)
}

func externalIDsToJSON(ids []ExternalIDInfo) []JSONExternalID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]JSONExternalID, len(ids))
	for i, eid := range ids {
		out[i] = JSONExternalID{Provider: eid.Provider, ExternalID: eid.ExternalID}
	}
	return out
}

func externalIDsFromJSON(ids []JSONExternalID) []ExternalIDInfo {
	if len(ids) == 0 {
		return nil
	}
	out := make([]ExternalIDInfo, len(ids))
	for i, eid := range ids {
		out[i] = ExternalIDInfo{Provider: eid.Provider, ExternalID: eid.ExternalID}
	}
	return out
}

func UserInfoToJSON(u UserInfo) JSONUserInfo {
	return JSONUserInfo{
		ID: u.ID, Name: u.Name, DisplayName: u.DisplayName, Email: u.Email,
		ExternalIDs: externalIDsToJSON(u.ExternalIDs),
	}
}

func UserInfoFromJSON(j JSONUserInfo) UserInfo {
	return UserInfo{
		ID: j.ID, Name: j.Name, DisplayName: j.DisplayName, Email: j.Email,
		ExternalIDs: externalIDsFromJSON(j.ExternalIDs),
	}
}
