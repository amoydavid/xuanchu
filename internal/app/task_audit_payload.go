package app

import (
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// taskModifyAuditPayload 构造 task.modify 专用的 audit payload。
// 它在既有 project before/after 字段基础上追加 changes 数组，
// 记录字段级 before/after diff，供任务详情页变更历史展示。
//
// changes 只保存机器语义（字段名、旧值、新值、added/removed），
// 不写入任何人类文案；展示层负责生成自然语言。
func taskModifyAuditPayload(change projectChange, diff TaskChangeDiff) map[string]any {
	payload := projectChangePayload(change)
	payload["changes"] = taskFieldChanges(change, diff)
	return payload
}

// taskFieldChanges 按 spec 固定顺序输出字段级 change 列表：
// assignees -> due -> priority -> project -> tags -> title -> description。
// 即使列表为空也返回空切片，让新写入的 payload 与历史缺字段区分开。
func taskFieldChanges(change projectChange, diff TaskChangeDiff) []map[string]any {
	out := make([]map[string]any, 0, 7)

	if diff.AssigneesChanged && (len(diff.AddedAssignees) > 0 || len(diff.RemovedAssignees) > 0) {
		out = append(out, map[string]any{
			"field":   "assignees",
			"added":   userInfoListToPayload(diff.AddedAssignees),
			"removed": userInfoListToPayload(diff.RemovedAssignees),
		})
	}

	if diff.DueChanged {
		out = append(out, map[string]any{
			"field":    "due",
			"previous": int64PtrToAny(diff.PreviousDue),
			"current":  int64PtrToAny(diff.CurrentDue),
		})
	}

	if diff.PriorityChanged {
		out = append(out, map[string]any{
			"field":    "priority",
			"previous": stringPtrToAny(diff.PreviousPriority),
			"current":  stringPtrToAny(diff.CurrentPriority),
		})
	}

	if diff.ProjectChanged {
		out = append(out, map[string]any{
			"field":    "project",
			"previous": stringPtrToAny(diff.PreviousProject),
			"current":  stringPtrToAny(diff.CurrentProject),
		})
	}

	if diff.TagsChanged && (len(diff.AddedTags) > 0 || len(diff.RemovedTags) > 0) {
		out = append(out, map[string]any{
			"field":   "tags",
			"added":   append([]string(nil), diff.AddedTags...),
			"removed": append([]string(nil), diff.RemovedTags...),
		})
	}

	if diff.TitleChanged {
		out = append(out, map[string]any{
			"field":    "title",
			"previous": diff.PreviousTitle,
			"current":  diff.CurrentTitle,
		})
	}

	if diff.DescriptionChanged {
		out = append(out, map[string]any{
			"field":    "description",
			"previous": stringPtrToAny(diff.PreviousDescription),
			"current":  stringPtrToAny(diff.CurrentDescription),
		})
	}

	// 低频字段（spec 第二期纳入）。
	if diff.WaitChanged {
		out = append(out, map[string]any{
			"field":    "wait",
			"previous": int64PtrToAny(diff.PreviousWait),
			"current":  int64PtrToAny(diff.CurrentWait),
		})
	}
	if diff.ScheduledChanged {
		out = append(out, map[string]any{
			"field":    "scheduled",
			"previous": int64PtrToAny(diff.PreviousScheduled),
			"current":  int64PtrToAny(diff.CurrentScheduled),
		})
	}
	if diff.UntilChanged {
		out = append(out, map[string]any{
			"field":    "until",
			"previous": int64PtrToAny(diff.PreviousUntil),
			"current":  int64PtrToAny(diff.CurrentUntil),
		})
	}
	if diff.DependsChanged && (len(diff.AddedDepends) > 0 || len(diff.RemovedDepends) > 0) {
		out = append(out, map[string]any{
			"field":   "depends",
			"added":   append([]string(nil), diff.AddedDepends...),
			"removed": append([]string(nil), diff.RemovedDepends...),
		})
	}
	if diff.UDAsChanged {
		out = append(out, udaChangesToPayload(diff))
	}

	return out
}

// udaChangesToPayload 把 UDA 变化输出为单个 uda change 条目，
// 用 entries 数组承载每个 UDA 的 before/after，避免多个 uda field 行。
func udaChangesToPayload(diff TaskChangeDiff) map[string]any {
	entries := make([]map[string]any, 0, len(diff.ChangedUDAs))
	for _, c := range diff.ChangedUDAs {
		entry := map[string]any{"name": c.Name}
		if c.Before != nil {
			entry["previous"] = c.Before.Raw
		} else {
			entry["previous"] = nil
		}
		if c.After != nil {
			entry["current"] = c.After.Raw
		} else {
			entry["current"] = nil
		}
		entries = append(entries, entry)
	}
	return map[string]any{
		"field":   "udas",
		"entries": entries,
	}
}

// stringPtrToAny 把 *string 解为 any；nil 指针返回 nil，避免 JSON 里出现
// `(*string)(nil)` 导致 key 被省略或序列化异常。
func stringPtrToAny(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// int64PtrToAny 同 stringPtrToAny，处理 *int64。
func int64PtrToAny(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// taskModifyAuditEntry 是 task.modify 专用的 AuditEntry 构造器。
// 不复用通用 taskAuditEntry，避免给 task.add / task.done / task.annotate
// 等非本期 action 追加无意义的 changes 字段。
func taskModifyAuditEntry(targetID string, change projectChange, diff TaskChangeDiff) AuditEntry {
	return AuditEntry{
		Action:     "task.modify",
		ProjectID:  auditProjectIDForChange(change),
		TargetType: "task",
		TargetID:   targetID,
		Payload:    taskModifyAuditPayload(change, diff),
	}
}

// userInfoListToPayload 把 UserInfo 列表序列化为完整 JSON 形状，
// 包含 id / name / display_name / email / external_ids，
// 遵循项目用户信息输出规范，不允许裸 UUID。
func userInfoListToPayload(infos []task.UserInfo) []any {
	out := make([]any, 0, len(infos))
	for _, info := range infos {
		out = append(out, userInfoToAuditPayload(info))
	}
	return out
}

// userInfoToAuditPayload 输出含 display_name 的 UserInfo payload。
// 不复用 userInfoToEventPayload（webhook 专用，缺 display_name），
// 这里使用与 task.UserInfoToJSON 一致的形状，保证历史数据人类可读。
func userInfoToAuditPayload(info task.UserInfo) map[string]any {
	externalIDs := make([]any, 0, len(info.ExternalIDs))
	for _, externalID := range info.ExternalIDs {
		externalIDs = append(externalIDs, map[string]any{
			"provider":    externalID.Provider,
			"external_id": externalID.ExternalID,
		})
	}
	return map[string]any{
		"id":           info.ID,
		"name":         info.Name,
		"display_name": info.DisplayName,
		"email":        info.Email,
		"external_ids": externalIDs,
	}
}
