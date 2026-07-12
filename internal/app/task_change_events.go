package app

import (
	"sort"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

type TaskChangeDiff struct {
	AssigneesChanged bool
	AddedAssignees   []task.UserInfo
	RemovedAssignees []task.UserInfo
	CurrentAssignees []task.UserInfo

	DueChanged  bool
	PreviousDue *int64
	CurrentDue  *int64

	PriorityChanged  bool
	PreviousPriority *string
	CurrentPriority  *string

	ProjectChanged  bool
	PreviousProject *string
	CurrentProject  *string

	TagsChanged bool
	AddedTags   []string
	RemovedTags []string

	TitleChanged       bool
	PreviousTitle      string
	CurrentTitle       string

	DescriptionChanged  bool
	PreviousDescription *string
	CurrentDescription  *string

	// 低频字段（spec 第二期纳入）：
	WaitChanged  bool
	PreviousWait *int64
	CurrentWait  *int64

	ScheduledChanged  bool
	PreviousScheduled *int64
	CurrentScheduled  *int64

	UntilChanged  bool
	PreviousUntil *int64
	CurrentUntil  *int64

	DependsChanged bool
	AddedDepends   []string
	RemovedDepends []string

	// UDA 用 (name, before, after) 三元组表达变化：
	// - before 为空表示新增的 UDA；
	// - after 为空表示删除的 UDA；
	// - 都非空表示值变化。
	UDAsChanged    bool
	AddedUDAKeys   []string
	RemovedUDAKeys []string
	ChangedUDAs    []UDAValueChange
}

// UDAValueChange 描述单个 UDA 的 before/after。
type UDAValueChange struct {
	Name   string
	Before *task.UDAValue
	After  *task.UDAValue
}

func diffTaskChanges(before, after task.Task) TaskChangeDiff {
	var diff TaskChangeDiff

	if ptrStringDiff(before.Priority, after.Priority) {
		diff.PriorityChanged = true
		diff.PreviousPriority = before.Priority
		diff.CurrentPriority = after.Priority
	}

	if ptrInt64Diff(before.Due, after.Due) {
		diff.DueChanged = true
		diff.PreviousDue = before.Due
		diff.CurrentDue = after.Due
	}

	if ptrStringDiff(before.Project, after.Project) {
		diff.ProjectChanged = true
		diff.PreviousProject = before.Project
		diff.CurrentProject = after.Project
	}

	beforeTags := stringSet(before.Tags)
	afterTags := stringSet(after.Tags)
	if !stringSetEqual(beforeTags, afterTags) {
		diff.TagsChanged = true
		diff.AddedTags = stringSetDifference(afterTags, beforeTags)
		diff.RemovedTags = stringSetDifference(beforeTags, afterTags)
	}

	beforeAssignees := assigneeIDSet(before.Assignees)
	afterAssignees := assigneeIDSet(after.Assignees)
	if !stringSetEqual(beforeAssignees, afterAssignees) {
		diff.AssigneesChanged = true
	}

	// title / description 只进 audit payload，不进入 HookEvent。
	if before.Title != after.Title {
		diff.TitleChanged = true
		diff.PreviousTitle = before.Title
		diff.CurrentTitle = after.Title
	}
	if ptrStringDiff(before.Description, after.Description) {
		diff.DescriptionChanged = true
		diff.PreviousDescription = before.Description
		diff.CurrentDescription = after.Description
	}

	// 低频字段 diff（只进 audit payload）。
	diffWaitScheduledUntil(&diff, before, after)
	diffDepends(&diff, before, after)
	diffUDAs(&diff, before, after)

	return diff
}

// diffWaitScheduledUntil 计算 wait/scheduled/until 的标量 diff。
func diffWaitScheduledUntil(diff *TaskChangeDiff, before, after task.Task) {
	if ptrInt64Diff(before.Wait, after.Wait) {
		diff.WaitChanged = true
		diff.PreviousWait = before.Wait
		diff.CurrentWait = after.Wait
	}
	if ptrInt64Diff(before.Scheduled, after.Scheduled) {
		diff.ScheduledChanged = true
		diff.PreviousScheduled = before.Scheduled
		diff.CurrentScheduled = after.Scheduled
	}
	if ptrInt64Diff(before.Until, after.Until) {
		diff.UntilChanged = true
		diff.PreviousUntil = before.Until
		diff.CurrentUntil = after.Until
	}
}

// diffDepends 计算依赖（任务 UUID 列表）的集合 diff。
func diffDepends(diff *TaskChangeDiff, before, after task.Task) {
	beforeSet := stringSet(before.Depends)
	afterSet := stringSet(after.Depends)
	if stringSetEqual(beforeSet, afterSet) {
		return
	}
	diff.DependsChanged = true
	diff.AddedDepends = stringSetDifference(afterSet, beforeSet)
	diff.RemovedDepends = stringSetDifference(beforeSet, afterSet)
}

// diffUDAs 计算 UDA 的变化：新增的 key、删除的 key、值变化的 key。
// 比较基于 UDAValue.Raw（归一化后的字符串值）。
func diffUDAs(diff *TaskChangeDiff, before, after task.Task) {
	beforeUDAs := before.UDAs
	afterUDAs := after.UDAs
	visited := make(map[string]bool, len(beforeUDAs)+len(afterUDAs))
	for name := range beforeUDAs {
		visited[name] = true
	}
	for name := range afterUDAs {
		visited[name] = true
	}
	if len(visited) == 0 {
		return
	}
	var added, removed, changed []string
	var changes []UDAValueChange
	for name := range visited {
		beforeVal, hasBefore := beforeUDAs[name]
		afterVal, hasAfter := afterUDAs[name]
		switch {
		case hasBefore && !hasAfter:
			removed = append(removed, name)
			bv := beforeVal
			changes = append(changes, UDAValueChange{Name: name, Before: &bv, After: nil})
		case !hasBefore && hasAfter:
			added = append(added, name)
			av := afterVal
			changes = append(changes, UDAValueChange{Name: name, Before: nil, After: &av})
		case beforeVal.Raw != afterVal.Raw:
			changed = append(changed, name)
			bv := beforeVal
			av := afterVal
			changes = append(changes, UDAValueChange{Name: name, Before: &bv, After: &av})
		}
	}
	if len(added) == 0 && len(removed) == 0 && len(changed) == 0 {
		return
	}
	diff.UDAsChanged = true
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)
	diff.AddedUDAKeys = added
	diff.RemovedUDAKeys = removed
	diff.ChangedUDAs = filterUDAChanges(changes, added, removed, changed)
}

// filterUDAChanges 按 added/removed/changed 顺序整理 changes 输出，
// 让 payload 顺序稳定可断言。
func filterUDAChanges(changes []UDAValueChange, added, removed, changed []string) []UDAValueChange {
	byName := make(map[string]UDAValueChange, len(changes))
	for _, c := range changes {
		byName[c.Name] = c
	}
	out := make([]UDAValueChange, 0, len(added)+len(removed)+len(changed))
	for _, n := range added {
		out = append(out, byName[n])
	}
	for _, n := range removed {
		out = append(out, byName[n])
	}
	for _, n := range changed {
		out = append(out, byName[n])
	}
	return out
}

func buildFineGrainedEvents(diff TaskChangeDiff, after task.Task, runtime RuntimeContext, now int64) []HookEvent {
	var events []HookEvent

	if diff.PriorityChanged {
		e := buildTaskHookEvent("task.priority_changed", after, runtime, now)
		if diff.PreviousPriority != nil {
			e.Data["previous_priority"] = *diff.PreviousPriority
		} else {
			e.Data["previous_priority"] = nil
		}
		if diff.CurrentPriority != nil {
			e.Data["current_priority"] = *diff.CurrentPriority
		} else {
			e.Data["current_priority"] = nil
		}
		events = append(events, e)
	}

	if diff.DueChanged {
		e := buildTaskHookEvent("task.due_changed", after, runtime, now)
		e.Data["previous_due"] = diff.PreviousDue
		e.Data["current_due"] = diff.CurrentDue
		events = append(events, e)
	}

	if diff.ProjectChanged {
		e := buildTaskHookEvent("task.project_changed", after, runtime, now)
		if diff.PreviousProject != nil {
			e.Data["previous_project"] = *diff.PreviousProject
		} else {
			e.Data["previous_project"] = nil
		}
		if diff.CurrentProject != nil {
			e.Data["current_project"] = *diff.CurrentProject
		} else {
			e.Data["current_project"] = nil
		}
		events = append(events, e)
	}

	if diff.TagsChanged {
		e := buildTaskHookEvent("task.tags_changed", after, runtime, now)
		e.Data["added_tags"] = diff.AddedTags
		e.Data["removed_tags"] = diff.RemovedTags
		e.Data["current_tags"] = sortedStringSlice(after.Tags)
		events = append(events, e)
	}

	if diff.AssigneesChanged {
		if len(diff.AddedAssignees) > 0 {
			e := buildTaskHookEvent("task.assigned", after, runtime, now)
			e.Data["added_assignees"] = userInfosToJSONList(diff.AddedAssignees)
			e.Data["current_assignees"] = userInfosToJSONList(diff.CurrentAssignees)
			events = append(events, e)
		}
		if len(diff.RemovedAssignees) > 0 {
			e := buildTaskHookEvent("task.unassigned", after, runtime, now)
			e.Data["removed_assignees"] = userInfosToJSONList(diff.RemovedAssignees)
			e.Data["current_assignees"] = userInfosToJSONList(diff.CurrentAssignees)
			events = append(events, e)
		}
	}

	return events
}

func (s *Service) hydrateAssigneeDiff(diff TaskChangeDiff, before, after []task.AssigneeInfo) TaskChangeDiff {
	if !diff.AssigneesChanged {
		return diff
	}
	beforeIDs := assigneeIDSet(before)
	afterIDs := assigneeIDSet(after)
	var addedIDs, removedIDs []string
	for id := range afterIDs {
		if !beforeIDs[id] {
			addedIDs = append(addedIDs, id)
		}
	}
	for id := range beforeIDs {
		if !afterIDs[id] {
			removedIDs = append(removedIDs, id)
		}
	}
	sort.Strings(addedIDs)
	sort.Strings(removedIDs)
	currentIDs := make([]string, 0, len(after))
	for _, assignee := range after {
		if assignee.UserID != "" {
			currentIDs = append(currentIDs, assignee.UserID)
		}
	}
	sort.Strings(currentIDs)
	ids := make([]string, 0, len(addedIDs)+len(removedIDs)+len(currentIDs))
	ids = append(ids, addedIDs...)
	ids = append(ids, removedIDs...)
	ids = append(ids, currentIDs...)
	infos, err := s.resolveUserInfos(ids)
	if err != nil {
		return diff
	}
	for _, id := range addedIDs {
		diff.AddedAssignees = append(diff.AddedAssignees, infos[id])
	}
	for _, id := range removedIDs {
		diff.RemovedAssignees = append(diff.RemovedAssignees, infos[id])
	}
	for _, id := range currentIDs {
		diff.CurrentAssignees = append(diff.CurrentAssignees, infos[id])
	}
	return diff
}

func (s *Service) detectBlockedEventsAfterModify(before, after task.Task) []HookEvent {
	now := s.clock.Unix()
	if !isDependencyEligible(before, now) || !isDependencyEligible(after, now) {
		return nil
	}
	beforeDepSet := map[string]bool{}
	for _, d := range before.Depends {
		beforeDepSet[d] = true
	}
	if len(beforeDepSet) > 0 {
		for depUUID := range beforeDepSet {
			dep, err := s.repo.GetByUUID(s.workspaceID, depUUID)
			if err != nil {
				continue
			}
			if isDependencyEligible(dep, now) {
				return nil
			}
		}
	}
	var newDeps []string
	for _, d := range after.Depends {
		if !beforeDepSet[d] {
			newDeps = append(newDeps, d)
		}
	}
	if len(newDeps) == 0 {
		return nil
	}
	var blockingDeps []string
	for _, depUUID := range newDeps {
		dep, err := s.repo.GetByUUID(s.workspaceID, depUUID)
		if err != nil {
			continue
		}
		if isDependencyEligible(dep, now) {
			blockingDeps = append(blockingDeps, depUUID)
		}
	}
	if len(blockingDeps) == 0 {
		return nil
	}
	sort.Strings(blockingDeps)
	return []HookEvent{buildTaskBlockedHookEvent(after, blockingDeps, s.runtime, now)}
}

func (s *Service) detectBlockedEventsAfterAdd(created task.Task) []HookEvent {
	now := s.clock.Unix()
	if !isDependencyEligible(created, now) {
		return nil
	}
	if len(created.Depends) == 0 {
		return nil
	}
	var blockingDeps []string
	for _, depUUID := range created.Depends {
		dep, err := s.repo.GetByUUID(s.workspaceID, depUUID)
		if err != nil {
			continue
		}
		if isDependencyEligible(dep, now) {
			blockingDeps = append(blockingDeps, depUUID)
		}
	}
	if len(blockingDeps) == 0 {
		return nil
	}
	sort.Strings(blockingDeps)
	return []HookEvent{buildTaskBlockedHookEvent(created, blockingDeps, s.runtime, now)}
}

func userInfosToJSONList(infos []task.UserInfo) []any {
	out := make([]any, len(infos))
	for i, info := range infos {
		out[i] = userInfoToEventPayload(info)
	}
	return out
}

func userInfoToEventPayload(info task.UserInfo) map[string]any {
	externalIDs := make([]any, len(info.ExternalIDs))
	for i, externalID := range info.ExternalIDs {
		externalIDs[i] = map[string]any{
			"provider":    externalID.Provider,
			"external_id": externalID.ExternalID,
		}
	}
	return map[string]any{
		"id":           info.ID,
		"name":         info.Name,
		"email":        info.Email,
		"external_ids": externalIDs,
	}
}

func ptrStringDiff(a, b *string) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}

func ptrInt64Diff(a, b *int64) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}

func stringSet(tags []string) map[string]bool {
	m := make(map[string]bool, len(tags))
	for _, t := range tags {
		m[t] = true
	}
	return m
}

func assigneeIDSet(assignees []task.AssigneeInfo) map[string]bool {
	m := make(map[string]bool, len(assignees))
	for _, a := range assignees {
		m[a.UserID] = true
	}
	return m
}

func stringSetEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func stringSetDifference(a, b map[string]bool) []string {
	var diff []string
	for k := range a {
		if !b[k] {
			diff = append(diff, k)
		}
	}
	sort.Strings(diff)
	return diff
}

func sortedStringSlice(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
