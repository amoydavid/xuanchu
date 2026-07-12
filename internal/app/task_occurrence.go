package app

import (
	"fmt"
	"strconv"
	"strings"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
)

// OccurrenceMode 控制任务查询如何处理循环 occurrence（spec §13.3、§17.1）。
type OccurrenceMode string

const (
	// OccurrenceModeAuto 同时存在 due_after/due_before 时 expand，否则 materialized。
	OccurrenceModeAuto OccurrenceMode = "auto"
	// OccurrenceModeMaterialized 只返回普通任务和已物化 occurrence。
	OccurrenceModeMaterialized OccurrenceMode = "materialized"
	// OccurrenceModeExpand 合并 ordinary/projected/materialized/tombstone，要求完整范围。
	OccurrenceModeExpand OccurrenceMode = "expand"
)

// TaskViewRange 是有界日期范围，左闭右开 [start, end)（spec §13.3）。
type TaskViewRange struct {
	Start int64
	End   int64
}

// TaskViewMetadata 是 TaskViewPage 的执行元信息。
type TaskViewMetadata struct {
	OccurrenceMode OccurrenceMode
	Range          *TaskViewRange
}

// RecurrenceInfo 描述 occurrence 的循环归属（spec §7.8）。
// 普通任务的 RecurrenceInfo 为 nil。
type RecurrenceInfo struct {
	Role            string // 固定 "occurrence"
	SeriesID        string
	SeriesStatus    string // active|ended|stopped
	Rule            string
	RecurrenceAt    int64
	Materialization string // projected|materialized
	Overrides       []string
	Until           *int64
}

// TaskOccurrenceView 是 App 层统一的任务视图（spec §7.8）。
// 普通任务、projected occurrence、materialized occurrence 都映射到此结构。
// HTTP/MCP/CLI/Remote 只消费此 view，不自己展开规则或拼 exception。
type TaskOccurrenceView struct {
	// ID 是公开稳定 id：普通任务=UUID；occurrence=occurrence_ref（投影/物化前后不变）。
	ID         string
	UUID       *string  // projected 时为 nil
	TaskSlug   *string  // projected 时为 nil
	ProjectSeq *int64   // projected 时为 nil
	WorkspaceID string
	ProjectID   *string
	Project     *string
	Title       string
	Description *string
	Status      string // projected 固定为 pending
	Entry       *int64 // projected 时为 nil
	Modified    *int64 // projected 时为 nil
	Start       *int64
	End         *int64
	Due         *int64
	Wait        *int64
	Scheduled   *int64
	Parent      *string // 仅手工父任务；occurrence 首版为空
	Priority    *string
	Tags        []string
	Assignees   []domain.UserInfo
	Depends     []string
	UDAs        map[string]domain.UDAValue
	RecurrenceInfo *RecurrenceInfo
}

// TaskViewPage 是 TaskOccurrenceView 的分页结果（spec §13.3）。
type TaskViewPage struct {
	Items          []TaskOccurrenceView
	Total          int
	Limit          int
	Offset         int
	OccurrenceMode OccurrenceMode
	Range          *TaskViewRange
}

// OccurrenceRef 构造 occurrence 的稳定公开引用（spec §7.4）。
// 格式：occ:<series_uuid>:<recurrence_at_unix>
func OccurrenceRef(seriesID string, recurrenceAt int64) string {
	return fmt.Sprintf("occ:%s:%d", seriesID, recurrenceAt)
}

// occurrenceRefPrefix 是 occurrence_ref 的前缀。
const occurrenceRefPrefix = "occ:"

// IsOccurrenceRef 判断给定 ref 是否为 occurrence_ref。
func IsOccurrenceRef(ref string) bool {
	return strings.HasPrefix(ref, occurrenceRefPrefix)
}

// ParseOccurrenceRef 解析 occurrence_ref，返回 (seriesID, recurrenceAt)。
func ParseOccurrenceRef(ref string) (seriesID string, recurrenceAt int64, err error) {
	if !strings.HasPrefix(ref, occurrenceRefPrefix) {
		return "", 0, fmt.Errorf("not an occurrence ref: %q", ref)
	}
	rest := ref[len(occurrenceRefPrefix):]
	// 格式 occ:<series_uuid>:<unix>。series_uuid 含两个 '-' 分隔的冒号会干扰，
	// 但 UUID 不含冒号，所以从右找最后一个冒号分隔 recurrence_at。
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		return "", 0, fmt.Errorf("malformed occurrence ref: %q", ref)
	}
	seriesID = rest[:idx]
	slotStr := rest[idx+1:]
	slot, err := strconv.ParseInt(slotStr, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("malformed occurrence ref slot: %q", ref)
	}
	if seriesID == "" {
		return "", 0, fmt.Errorf("malformed occurrence ref: empty series id")
	}
	return seriesID, slot, nil
}

// --- View 构造辅助 ---

// taskToView 把普通或已物化 task.Task 映射为 TaskOccurrenceView。
// occurrence 的 ID 固定为 occurrence_ref（即使已物化）。
func taskToView(tsk domain.Task, assignees []domain.UserInfo) TaskOccurrenceView {
	view := TaskOccurrenceView{
		ID:          tsk.UUID,
		UUID:        &tsk.UUID,
		WorkspaceID: tsk.WorkspaceID,
		ProjectID:   tsk.ProjectID,
		Project:     tsk.Project,
		Title:       tsk.Title,
		Description: tsk.Description,
		Status:      tsk.Status,
		Tags:        tsk.Tags,
		Start:       tsk.Start,
		End:         tsk.End,
		Due:         tsk.Due,
		Wait:        tsk.Wait,
		Scheduled:   tsk.Scheduled,
		Parent:      tsk.Parent,
		Priority:    tsk.Priority,
		Depends:     tsk.Depends,
		UDAs:        tsk.UDAs,
		Assignees:   assignees,
	}
	entry := tsk.Entry
	view.Entry = &entry
	modified := tsk.Modified
	view.Modified = &modified
	if tsk.ProjectSeq != nil {
		seqCopy := *tsk.ProjectSeq
		view.ProjectSeq = &seqCopy
	}
	if tsk.Project != nil && tsk.ProjectSeq != nil {
		slug := fmt.Sprintf("%s-%d", *tsk.Project, *tsk.ProjectSeq)
		view.TaskSlug = &slug
	}
	// occurrence：ID 用 occurrence_ref，附加 RecurrenceInfo。
	if tsk.SeriesID != nil && tsk.RecurrenceAt != nil && tsk.RecurrenceRuleSnapshot != nil {
		view.ID = OccurrenceRef(*tsk.SeriesID, *tsk.RecurrenceAt)
		view.RecurrenceInfo = &RecurrenceInfo{
			Role:            "occurrence",
			SeriesID:        *tsk.SeriesID,
			Rule:            *tsk.RecurrenceRuleSnapshot,
			RecurrenceAt:    *tsk.RecurrenceAt,
			Materialization: "materialized",
			Overrides:       tsk.RecurrenceOverrides,
		}
	}
	return view
}

// projectedOccurrenceView 从 series 共享字段构造 projected occurrence view（spec §7.8）。
// 不写库、不分配 UUID/project_seq。
func projectedOccurrenceView(series taskseries.Series, slot taskseries.Slot, assignees []domain.UserInfo) TaskOccurrenceView {
	ref := OccurrenceRef(series.ID, slot.RecurrenceAt)
	view := TaskOccurrenceView{
		ID:          ref,
		WorkspaceID: series.WorkspaceID,
		ProjectID:   &series.ProjectID,
		Title:       series.Title,
		Description: series.Description,
		Status:      domain.StatusPending,
		Due:         &slot.RecurrenceAt, // projected 的 due 初始等于 recurrence_at
		Priority:    series.Priority,
		Tags:        series.Tags,
		Assignees:   assignees,
		RecurrenceInfo: &RecurrenceInfo{
			Role:            "occurrence",
			SeriesID:        series.ID,
			SeriesStatus:    series.Status,
			Rule:            slot.Rule,
			RecurrenceAt:    slot.RecurrenceAt,
			Materialization: "projected",
			Overrides:       []string{},
			Until:           series.Until,
		},
	}
	return view
}
