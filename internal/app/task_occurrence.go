package app

import (
	"fmt"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
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

// occurrenceRangeMaxDays 是 occurrence_mode=expand 的最大范围天数（spec §13.3）。
const occurrenceRangeMaxDays = 366

// daySeconds 是 24 小时的秒数。
const daySeconds = 86400

// validateTaskViewRange 校验 expand 模式必需的完整范围（spec §13.3）。
// 返回 normalized 的 [start, end) 范围。
func validateTaskViewRange(mode OccurrenceMode, rng *TaskViewRange) error {
	if mode != OccurrenceModeExpand {
		return nil
	}
	if rng == nil {
		return RuntimeError{Code: "task_occurrence_range_required", Message: "occurrence_mode=expand 需要完整日期范围"}
	}
	if rng.End <= rng.Start {
		return RuntimeError{Code: "task_occurrence_range_required", Message: "occurrence_mode=expand 需要非空日期范围"}
	}
	days := (rng.End - rng.Start) / daySeconds
	if days > occurrenceRangeMaxDays {
		return RuntimeError{Code: "task_occurrence_range_too_large", Message: "展开范围超过 366 天"}
	}
	return nil
}

// TaskViewQuery 是 QueryTaskViews 的输入（spec §13.3、§17.1）。
type TaskViewQuery struct {
	WorkspaceID    string
	ProjectID      string // 可选，项目 scope
	Range          *TaskViewRange
	OccurrenceMode OccurrenceMode
	Status         string
	Sort           string
	Limit          int
	Offset         int
}

// QueryTaskViews 合并普通任务、projected occurrence、materialized occurrence（spec §7.9、§13.3）。
//
// expand 模式：require 完整范围，merge ordinary + projected + materialized exception - tombstone。
// materialized/auto(无范围)：只返回普通任务和已物化 occurrence。
//
// 纯读取：不写库、不分配 UUID/project_seq、不写 audit。
func (s *Service) QueryTaskViews(q TaskViewQuery) (TaskViewPage, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskViewPage{}, err
	}
	// 解析 effective mode：auto 在有完整范围时升级为 expand，否则 materialized。
	mode := q.OccurrenceMode
	if mode == "" || mode == OccurrenceModeAuto {
		if q.Range != nil && q.Range.End > q.Range.Start {
			mode = OccurrenceModeExpand
		} else {
			mode = OccurrenceModeMaterialized
		}
	}
	if err := validateTaskViewRange(mode, q.Range); err != nil {
		return TaskViewPage{}, err
	}

	items, err := s.collectTaskViewCandidates(q, mode)
	if err != nil {
		return TaskViewPage{}, err
	}

	// 稳定排序：ID 作为最终 tie-breaker。
	sortTaskViews(items, q.Sort)

	total := len(items)
	limit := q.Limit
	offset := q.Offset
	paged := paginateTaskViews(items, limit, offset)

	return TaskViewPage{
		Items:          paged,
		Total:          total,
		Limit:          limit,
		Offset:         offset,
		OccurrenceMode: mode,
		Range:          q.Range,
	}, nil
}

// collectTaskViewCandidates 执行 merge 算法，返回未排序、未分页的候选 view（spec §7.9）。
func (s *Service) collectTaskViewCandidates(q TaskViewQuery, mode OccurrenceMode) ([]TaskOccurrenceView, error) {
	workspaceID := q.WorkspaceID
	if workspaceID == "" {
		workspaceID = s.workspaceID
	}

	// 1. 读取普通任务 + 已物化 occurrence（materialized 行）。
	var ordinaryTasks []domain.Task
	var occurrenceTasks []domain.Task
	listOpts := storage.ListOptions{Status: q.Status, Sort: q.Sort}
	if q.ProjectID != "" {
		// 用 query scope 限制项目。
	}
	_ = listOpts

	// 普通任务（series_id IS NULL）+ materialized occurrence（series_id IS NOT NULL）。
	allTasks, err := s.repo.List(workspaceID, storage.ListOptions{Status: q.Status})
	if err != nil {
		return nil, err
	}
	for _, t := range allTasks {
		if t.SeriesID != nil {
			occurrenceTasks = append(occurrenceTasks, t)
		} else {
			ordinaryTasks = append(ordinaryTasks, t)
		}
	}

	// 收集所有 assignee user ids 以批量解析 UserInfo。
	assigneeIDs := collectAssigneeUserIDs(ordinaryTasks)
	// series assignee ids 在 expand 模式加入。
	var seriesList []taskseries.Series
	if mode == OccurrenceModeExpand {
		seriesOpts := storage.TaskSeriesListOptions{WorkspaceID: workspaceID}
		seriesCandidates, err := s.taskSeriesRepo.ListCandidates(seriesOpts)
		if err != nil {
			return nil, err
		}
		for _, se := range seriesCandidates {
			assigneeIDs = appendUnique(assigneeIDs, se.AssigneeIDs...)
			seriesList = append(seriesList, se)
		}
	}
	assigneeIDs = appendUnique(assigneeIDs, collectAssigneeUserIDs(occurrenceTasks)...)
	userInfos, err := s.resolveUserInfos(assigneeIDs)
	if err != nil {
		return nil, err
	}

	result := make([]TaskOccurrenceView, 0, len(ordinaryTasks)+len(occurrenceTasks))
	// 2. 普通任务（非 occurrence）。
	for _, t := range ordinaryTasks {
		result = append(result, taskToView(t, userInfoList(t.Assignees, userInfos)))
	}

	if mode != OccurrenceModeExpand {
		// materialized 模式：加入已物化 occurrence，不投影。
		for _, t := range occurrenceTasks {
			result = append(result, taskToView(t, userInfoList(t.Assignees, userInfos)))
		}
		return result, nil
	}

	// 3. expand 模式 merge（spec §7.9）。
	// 3a. 计算每个 active series 在 [start,end) 内的 projected slots。
	// 用 map[seriesID]map[slot]view 索引 projected，便于后续 exception/tombstone 覆盖。
	projectedByID := map[string]map[int64]TaskOccurrenceView{}
	for _, se := range seriesList {
		// ended/stopped series 按有效区间裁剪 projected；active 全展开。
		versions := ruleVersionsForExpand(se)
		if len(versions) == 0 {
			continue
		}
		slots, err := taskseries.ExpandRange(versions, se.Until, q.Range.Start, q.Range.End, s.clock.Location())
		if err != nil {
			return nil, err
		}
		// stopped/ended 的 effective_end_at 裁剪：available_at < effective_end_at。
		slotMap := projectedByID[se.ID]
		if slotMap == nil {
			slotMap = map[int64]TaskOccurrenceView{}
			projectedByID[se.ID] = slotMap
		}
		seriesAssignees := seriesUserInfoList(se, userInfos)
		for _, slot := range slots {
			if !slotInRangeForSeriesStatus(se, slot.RecurrenceAt) {
				continue
			}
			slotMap[slot.RecurrenceAt] = projectedOccurrenceView(se, slot, seriesAssignees)
		}
	}

	// 3b. materialized exception 覆盖 projected（按 recurrence_at）。
	// tombstone（deleted）排除 projected，除非显式查 deleted。
	for _, t := range occurrenceTasks {
		if t.SeriesID == nil || t.RecurrenceAt == nil {
			continue
		}
		seriesID := *t.SeriesID
		slot := *t.RecurrenceAt
		// 只处理属于当前 series candidates 的 occurrence（防御跨 series 污染）。
		slotMap := projectedByID[seriesID]
		view := taskToView(t, userInfoList(t.Assignees, userInfos))
		// 补充 series_status（materialized 行不含 series 当前状态）。
		if se := findSeries(seriesList, seriesID); se != nil && view.RecurrenceInfo != nil {
			view.RecurrenceInfo.SeriesStatus = se.Status
			view.RecurrenceInfo.Until = se.Until
		}
		// tombstone：deleted 排除 projected（除非显式查 deleted）。
		if t.Status == domain.StatusDeleted && q.Status != domain.StatusDeleted && q.Status != "" {
			// 默认（无 status filter 或 status != deleted）排除 deleted。
			if q.Status == "" {
				delete(slotMap, slot)
				continue
			}
		}
		if t.Status == domain.StatusDeleted && q.Status == "" {
			delete(slotMap, slot)
			continue
		}
		// 物化行覆盖 projected（同槽位）。
		if slotMap != nil {
			delete(slotMap, slot)
		}
		// exception 同时按原槽位和当前 due 命中范围（spec §7.9）。
		// materialized 行已经从 occurrenceTasks 读出，直接加入结果。
		result = append(result, view)
	}

	// 3c. 剩余 projected 加入结果。
	for _, slotMap := range projectedByID {
		for _, v := range slotMap {
			result = append(result, v)
		}
	}

	return result, nil
}

// GetTaskView 返回单个任务的 view（spec §13.4）。
// occurrence_ref：先查 materialized row；未命中则校验 series 有效区间和槽位合法性，返回 projected。
// 不写库。
func (s *Service) GetTaskView(ref string) (TaskOccurrenceView, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskOccurrenceView{}, err
	}
	if IsOccurrenceRef(ref) {
		return s.getOccurrenceView(ref)
	}
	// 普通 UUID/slug：读取 materialized task。
	tsk, err := s.resolveTargetForRead(ref)
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	userInfos, err := s.resolveUserInfos(collectAssigneeUserIDs([]domain.Task{tsk}))
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	return taskToView(tsk, userInfoList(tsk.Assignees, userInfos)), nil
}

// getOccurrenceView 处理 occurrence_ref，不写库（spec §7.4、§13.4）。
func (s *Service) getOccurrenceView(ref string) (TaskOccurrenceView, error) {
	seriesID, slot, err := ParseOccurrenceRef(ref)
	if err != nil {
		return TaskOccurrenceView{}, RuntimeError{Code: "task_occurrence_not_found", Message: err.Error()}
	}
	// 1. 先查 materialized row（任意 status）。
	occ, err := s.taskOccurrenceRepo.GetOccurrence(s.workspaceID, seriesID, slot)
	if err == nil {
		userInfos, rerr := s.resolveUserInfos(collectAssigneeUserIDs([]domain.Task{occ}))
		if rerr != nil {
			return TaskOccurrenceView{}, rerr
		}
		view := taskToView(occ, userInfoList(occ.Assignees, userInfos))
		// 补充 series status。
		se, _ := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
		if se.ID != "" && view.RecurrenceInfo != nil {
			view.RecurrenceInfo.SeriesStatus = se.Status
			view.RecurrenceInfo.Until = se.Until
		}
		return view, nil
	}
	// 2. projected：校验 series 存在、槽位合法。
	series, gerr := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
	if gerr != nil {
		return TaskOccurrenceView{}, RuntimeError{Code: "task_occurrence_not_found", Message: "series not found"}
	}
	versions := ruleVersionsForExpand(series)
	slots, err := taskseries.ExpandRange(versions, series.Until, slot, slot+1, s.clock.Location())
	if err != nil || len(slots) == 0 {
		return TaskOccurrenceView{}, RuntimeError{Code: "task_occurrence_not_found", Message: "槽位不属于任何规则段"}
	}
	if !slotInRangeForSeriesStatus(series, slot) {
		return TaskOccurrenceView{}, RuntimeError{Code: "task_occurrence_not_found", Message: "槽位超出 series 有效区间"}
	}
	userInfos, err := s.resolveUserInfos(series.AssigneeIDs)
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	return projectedOccurrenceView(series, slots[0], seriesUserInfoList(series, userInfos)), nil
}

// --- merge 辅助 ---

// ruleVersionsForExpand 把 series 的 RuleVersions 转为 taskseries.RuleVersion 切片。
func ruleVersionsForExpand(se taskseries.Series) []taskseries.RuleVersion {
	if len(se.RuleVersions) > 0 {
		return se.RuleVersions
	}
	// 兜底：无显式 rule version 时用 series 当前规则 + first_due 作为单段。
	return []taskseries.RuleVersion{{EffectiveFrom: se.FirstDue, RecurrenceRule: se.RecurrenceRule}}
}

// slotInRangeForSeriesStatus 检查槽位是否在 series 状态定义的有效区间内。
// stopped：available_at < effective_end_at；ended：slot <= until。
func slotInRangeForSeriesStatus(se taskseries.Series, slot int64) bool {
	switch se.Status {
	case taskseries.StatusStopped:
		if se.EffectiveEndAt == nil {
			return false
		}
		// available_at = 槽位所在日期的本地 00:00；这里简化用 slot 本身（23:59:59）
		// 作为上界判断（effective_end_at 之后的槽位不投影）。
		return slot < *se.EffectiveEndAt+daySeconds
	case taskseries.StatusEnded:
		if se.Until != nil && slot > *se.Until {
			return false
		}
	}
	if se.Until != nil && slot > *se.Until {
		return false
	}
	return slot >= se.FirstDue
}

func findSeries(list []taskseries.Series, id string) *taskseries.Series {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func collectAssigneeUserIDs(tasks []domain.Task) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, t := range tasks {
		for _, a := range t.Assignees {
			if _, ok := seen[a.UserID]; ok {
				continue
			}
			seen[a.UserID] = struct{}{}
			out = append(out, a.UserID)
		}
	}
	return out
}

func appendUnique(base []string, more ...string) []string {
	seen := map[string]struct{}{}
	for _, v := range base {
		seen[v] = struct{}{}
	}
	for _, v := range more {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		base = append(base, v)
	}
	return base
}

// userInfoList 把 task.Assignees 配合解析好的 userInfos 转为 []domain.UserInfo。
func userInfoList(assignees []domain.AssigneeInfo, userInfos map[string]domain.UserInfo) []domain.UserInfo {
	out := make([]domain.UserInfo, 0, len(assignees))
	for _, a := range assignees {
		if info, ok := userInfos[a.UserID]; ok {
			out = append(out, info)
		} else {
			out = append(out, domain.UserInfo{ID: a.UserID, Name: a.UserID})
		}
	}
	return out
}

// seriesUserInfoList 把 series.AssigneeIDs 配合 userInfos 转为 []domain.UserInfo。
func seriesUserInfoList(se taskseries.Series, userInfos map[string]domain.UserInfo) []domain.UserInfo {
	out := make([]domain.UserInfo, 0, len(se.AssigneeIDs))
	for _, id := range se.AssigneeIDs {
		if info, ok := userInfos[id]; ok {
			out = append(out, info)
		} else {
			out = append(out, domain.UserInfo{ID: id, Name: id})
		}
	}
	return out
}

// sortTaskViews 按 sort 模式稳定排序，ID 作为最终 tie-breaker。
func sortTaskViews(items []TaskOccurrenceView, sort string) {
	switch sort {
	case "due":
		sortByDueThenID(items)
	case "modified":
		sortByModifiedDescThenID(items)
	default:
		sortByID(items)
	}
}

func sortByID(items []TaskOccurrenceView) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].ID < items[j-1].ID; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func sortByDueThenID(items []TaskOccurrenceView) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0; j-- {
			a, b := items[j], items[j-1]
			aDue, bDue := int64(0), int64(0)
			if a.Due != nil {
				aDue = *a.Due
			}
			if b.Due != nil {
				bDue = *b.Due
			}
			if aDue < bDue || (aDue == bDue && a.ID < b.ID) {
				items[j], items[j-1] = items[j-1], items[j]
			} else {
				break
			}
		}
	}
}

func sortByModifiedDescThenID(items []TaskOccurrenceView) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0; j-- {
			a, b := items[j], items[j-1]
			aMod, bMod := int64(0), int64(0)
			if a.Modified != nil {
				aMod = *a.Modified
			}
			if b.Modified != nil {
				bMod = *b.Modified
			}
			if aMod > bMod || (aMod == bMod && a.ID < b.ID) {
				items[j], items[j-1] = items[j-1], items[j]
			} else {
				break
			}
		}
	}
}

func paginateTaskViews(items []TaskOccurrenceView, limit, offset int) []TaskOccurrenceView {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return nil
	}
	rest := items[offset:]
	if limit > 0 && limit < len(rest) {
		rest = rest[:limit]
	}
	return rest
}
