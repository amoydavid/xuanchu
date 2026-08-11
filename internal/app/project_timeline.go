package app

import (
	"sort"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// projectTimelineTaskActions 是项目时间线纳入的任务审计动作白名单。
// 与 task 详情 activity（taskActivityAuditActions）保持一致，确保任务生命周期、
// 字段修改、关联变更、周期实例化事件都能在项目时间线里体现。
var projectTimelineTaskActions = []string{
	"task.add", "task.recurrence.generated", "task.start", "task.stop",
	"task.done", "task.reopen", "task.delete", "task.modify",
	"task.link.add", "task.link.update", "task.link.remove",
}

// taskTimelineEntryFromAudit 把一条任务审计行解析成 TimelineEntry。
// 复用 task activity 的 action 映射、字段变更解析（parseTaskFieldChanges）
// 与链接解析（taskActivityLinkFromPayload），保证两处语义一致。
// taskByUUID 用于补全任务标题（source_label）；找不到时回退到 UUID 前 8 位。
// 第二个返回值 false 表示该审计行不应出现在时间线（例如 task.modify 无 changes）。
func taskTimelineEntryFromAudit(row storage.AuditLogEntry, taskByUUID map[string]task.Task) (TimelineEntry, bool) {
	kind, action := "", ""
	var changes []TaskFieldChange
	var link *TaskActivityLink
	switch row.Action {
	case "task.add":
		kind, action = "lifecycle", "created"
	case "task.recurrence.generated":
		kind, action = "lifecycle", "generated"
	case "task.start":
		kind, action = "lifecycle", "started"
	case "task.stop":
		kind, action = "lifecycle", "stopped"
	case "task.done":
		kind, action = "lifecycle", "completed"
	case "task.reopen":
		kind, action = "lifecycle", "reopened"
	case "task.delete":
		kind, action = "lifecycle", "deleted"
	case "task.modify":
		changes = parseTaskFieldChanges(row.PayloadJSON)
		if len(changes) == 0 {
			return TimelineEntry{}, false
		}
		kind, action = "change", "fields_changed"
	case "task.link.add", "task.link.update", "task.link.remove":
		actionByAudit := map[string]string{
			"task.link.add":    "link_added",
			"task.link.update": "link_updated",
			"task.link.remove": "link_removed",
		}
		kind = "relation"
		action = actionByAudit[row.Action]
		link = taskActivityLinkFromPayload(row.PayloadJSON)
	default:
		return TimelineEntry{}, false
	}
	title := taskTimelineTitle(row.TargetID, taskByUUID)
	return TimelineEntry{
		SourceType:  "task",
		SourceID:    row.TargetID,
		SourceLabel: title,
		Entry:       row.CreatedAt,
		CreatedBy:   task.ActorInfo{},
		Action:      action,
		Kind:        kind,
		Changes:     changes,
		Link:        link,
		CreatedAt:   row.CreatedAt,
	}, true
}

// taskTimelineTitle 从批量任务表里查标题，找不到时回退到 UUID 前 8 位。
func taskTimelineTitle(taskUUID string, taskByUUID map[string]task.Task) string {
	if t, ok := taskByUUID[taskUUID]; ok && t.Title != "" {
		return t.Title
	}
	if len(taskUUID) > 8 {
		return taskUUID[:8]
	}
	return taskUUID
}

// timelineAuditActorColumns 从审计行抽出 actor 列，供批量解析复用。
func timelineAuditActorColumns(row storage.AuditLogEntry) actorColumns {
	return actorColumns{
		Type:        auditActorType(row),
		UserID:      row.ActorUserID,
		TokenID:     row.ActorTokenID,
		TokenName:   row.ActorTokenName,
		TokenPrefix: row.ActorTokenPrefix,
	}
}

// sortTimelineEntries 把合并后的条目按时间戳倒序排列。
// annotation 用 entry（单调计数器），audit 用 created_at（unix 秒），
// 两者数量级接近时排序近似正确；同时间戳下保持稳定顺序。
func sortTimelineEntries(entries []TimelineEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Entry > entries[j].Entry
	})
}
