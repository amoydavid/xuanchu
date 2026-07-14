package task

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
	StatusWaiting   = "waiting"
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
	UserType   string
	ExternalID string
}

type UserInfo struct {
	ID          string
	Name        string
	DisplayName string
	Email       *string
	ExternalIDs []ExternalIDInfo
}

type TokenActorInfo struct {
	ID     string
	Name   string
	Prefix string
}

type ActorInfo struct {
	Type  string
	ID    string
	Name  string
	User  *UserInfo
	Token *TokenActorInfo
}

type AssigneeInfo struct {
	UserID      string
	Name        string
	DisplayName string
	Email       *string
	ExternalIDs []ExternalIDInfo
}

type TaskLinkInfo struct {
	ID        string
	Type      string
	URL       string
	Title     string
	CreatedAt int64
	CreatedBy ActorInfo
}

type Task struct {
	UUID        string
	WorkspaceID string
	Title       string
	Description *string
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
	Parent      *string
	Assignees   []AssigneeInfo
	Links       []TaskLinkInfo
	UDAs        map[string]UDAValue
	// 以下为循环实例（occurrence）持久字段（spec §7.2）。
	// 普通任务三者全 nil；已物化 occurrence 三者全非 nil。
	SeriesID                *string
	RecurrenceAt            *int64
	RecurrenceRuleSnapshot  *string
	RecurrenceOverrides     []string
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("title is required")
	}
	switch t.Status {
	case StatusPending, StatusCompleted, StatusDeleted, StatusWaiting:
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
	if err := validateOccurrenceInvariant(&t); err != nil {
		return err
	}
	return nil
}

// validateOccurrenceInvariant 校验 occurrence 持久字段的三元一致性（spec §7.2）。
// series_id / recurrence_at / recurrence_rule_snapshot 要么全 nil，要么全非 nil。
func validateOccurrenceInvariant(t *Task) error {
	set := 0
	if t.SeriesID != nil {
		set++
	}
	if t.RecurrenceAt != nil {
		set++
	}
	if t.RecurrenceRuleSnapshot != nil {
		set++
	}
	if set != 0 && set != 3 {
		return errors.New("occurrence fields series_id/recurrence_at/recurrence_rule_snapshot must be all nil or all set")
	}
	if err := ValidateRecurrenceOverrides(t.RecurrenceOverrides); err != nil {
		return err
	}
	return nil
}

// allowedRecurrenceOverrideFields 是 occurrence 单次字段覆盖允许的字段集合（spec §7.5）。
var allowedRecurrenceOverrideFields = map[string]struct{}{
	"title":       {},
	"description": {},
	"priority":    {},
	"due":         {},
	"assignees":   {},
	"tags":        {},
	"udas":        {},
	"wait":        {},
	"scheduled":   {},
	"depends":     {},
}

// NormalizeRecurrenceOverrides 返回去重、升序排序后的 override 字段列表。
// nil/空输入返回空 slice（不返回 nil，便于持久化层稳定序列化）。
func NormalizeRecurrenceOverrides(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, f := range in {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// ValidateRecurrenceOverrides 校验 override 字段列表是否都在允许集合内。
// 注意：它不要求排序，排序由 NormalizeRecurrenceOverrides 完成。
func ValidateRecurrenceOverrides(in []string) error {
	for _, f := range in {
		if _, ok := allowedRecurrenceOverrideFields[f]; !ok {
			return fmt.Errorf("recurrence override field %q is not allowed", f)
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

// Reopen 把已完成任务恢复为 pending。
// 作为 Complete 的逆操作：清掉 End（完成时间）与 Start（计时锚点），
// 回到普通待处理状态，用户可重新 start。
func (t *Task) Reopen(now int64) {
	t.Status = StatusPending
	t.End = nil
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
