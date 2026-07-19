package app

import (
	"context"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

// buildUserMentionedEventIfNeeded 比较前后 description 中的 user 引用集合，
// 仅当存在新增 user mention 时返回一个 task.user_mentioned 事件。
//
// spec §16.1：label/移动/同次删除再插入都不触发；先删保存、后再次添加保存会再次触发。
func (s *Service) buildUserMentionedEventIfNeeded(before, after domain.Task, now int64) (HookEvent, bool) {
	beforeRefs, err := domain.ParseContentReferences(optionalTextValue(before.Description))
	if err != nil {
		return HookEvent{}, false
	}
	afterRefs, err := domain.ParseContentReferences(optionalTextValue(after.Description))
	if err != nil {
		return HookEvent{}, false
	}
	added, _ := domain.DiffContentReferenceKeys(beforeRefs, afterRefs)
	addedUserIDs := uniqueUserIDs(added)
	if len(addedUserIDs) == 0 {
		return HookEvent{}, false
	}

	// 解析为完整 UserInfo。未找到的 fallback 为 {ID, Name: id}，不出错。
	addedInfos, _ := s.resolveUserInfos(addedUserIDs)
	currentIDs, _ := domain.MentionedUserIDs(optionalTextValue(after.Description))
	currentInfos, _ := s.resolveUserInfos(currentIDs)

	addedList := orderedUserInfoList(addedUserIDs, addedInfos)
	currentList := orderedUserInfoList(currentIDs, currentInfos)

	event := buildTaskHookEvent("task.user_mentioned", after, s.runtime, now)
	event.Data["source_field"] = "description"
	event.Data["mentioned_users"] = domain.UserInfoListToJSON(addedList)
	event.Data["current_mentioned_users"] = domain.UserInfoListToJSON(currentList)
	return event, true
}

func uniqueUserIDs(keys []domain.ContentReferenceKey) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, k := range keys {
		if k.Kind != domain.ContentReferenceUser {
			continue
		}
		if _, ok := seen[k.ID]; ok {
			continue
		}
		seen[k.ID] = struct{}{}
		ids = append(ids, k.ID)
	}
	return ids
}

func orderedUserInfoList(ids []string, infos map[string]domain.UserInfo) []domain.UserInfo {
	out := make([]domain.UserInfo, 0, len(ids))
	for _, id := range ids {
		info, ok := infos[id]
		if !ok {
			info = domain.UserInfo{ID: id, Name: id}
		}
		out = append(out, info)
	}
	return out
}

// SuggestContentReferences 是计划 4 Task 3 的入口；当前实现返回最小占位，
// 完整查询由后续迭代补齐。返回值结构对齐 spec §13.2。
func (s *Service) SuggestContentReferences(ctx context.Context, refType, query string) ([]map[string]any, error) {
	// 当前返回空 slice，避免误用未完成查询；上层 HTTP handler 在 refType 不支持时返回错误。
	return []map[string]any{}, nil
}
