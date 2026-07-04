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

	return diff
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
