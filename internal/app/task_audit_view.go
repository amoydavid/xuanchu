package app

import (
	"encoding/json"
	"fmt"
	"time"
)

// parseTaskFieldChanges 从 audit payload JSON 解析出字段级 change 视图。
// 解析失败或无 changes 时返回空切片（不报错），保证旧数据兼容。
func parseTaskFieldChanges(payloadJSON string) []TaskFieldChange {
	if payloadJSON == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil
	}
	rawChanges, ok := payload["changes"].([]any)
	if !ok {
		return nil
	}
	out := make([]TaskFieldChange, 0, len(rawChanges))
	for _, raw := range rawChanges {
		change, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		field, _ := change["field"].(string)
		if field == "" {
			continue
		}
		view := TaskFieldChange{
			Field:    field,
			LabelKey: taskFieldLabelKey(field),
		}
		switch {
		case field == "udas":
			// UDA 用 entries 承载每个 UDA 的 name + before/after。
			view.Kind = "uda"
			view.Entries = parseUDAEntries(change["entries"])
		case hasKey(change, "added"):
			view.Kind = "set"
			view.Added = parseDisplayValues(change["added"])
			view.Removed = parseDisplayValues(change["removed"])
		default:
			view.Kind = "scalar"
			// 标量 change 的 previous/current 必须保留 presence，
			// 即使 raw 为 nil，所以这里总是构造非 nil 指针。
			prev := change["previous"]
			curr := change["current"]
			view.Previous = &TaskChangeDisplayValue{
				Raw:  prev,
				Text: taskChangeDisplayText(field, prev, true),
			}
			view.Current = &TaskChangeDisplayValue{
				Raw:  curr,
				Text: taskChangeDisplayText(field, curr, false),
			}
		}
		out = append(out, view)
	}
	return out
}

// hasKey 报告 map 是否含某个 key（用于区分 set 和 scalar，
// 因为 change["added"] == nil 可能是 key 存在但值为 nil）。
func hasKey(change map[string]any, key string) bool {
	_, ok := change[key]
	return ok
}

// parseUDAEntries 把 audit payload 里的 uda entries 转成 view 结构。
// 每个 entry 含 name + previous（raw 字符串或 null）+ current（同）。
func parseUDAEntries(raw any) []UDAEntryChange {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]UDAEntryChange, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["name"].(string)
		if name == "" {
			continue
		}
		cec := UDAEntryChange{Name: name}
		// UDA 的 previous/current 用指针：key 缺失（新增/删除）时该侧为 nil，
		// key 存在但值为 nil 时构造非 nil 指针 + Raw:nil。
		if prevRaw, hasPrev := entry["previous"]; hasPrev {
			cec.Before = &TaskChangeDisplayValue{
				Raw:  prevRaw,
				Text: udaValueText(prevRaw),
			}
		}
		if currRaw, hasCurr := entry["current"]; hasCurr {
			cec.After = &TaskChangeDisplayValue{
				Raw:  currRaw,
				Text: udaValueText(currRaw),
			}
		}
		out = append(out, cec)
	}
	return out
}

func udaValueText(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return truncateForTimeline(s)
	}
	return fmt.Sprintf("%v", raw)
}

// parseDisplayValues 把集合类 change 的 added/removed 元素转成 DisplayValue。
// 元素可能是字符串（tags）或对象（assignees 的 UserInfo）。
func parseDisplayValues(raw any) []TaskChangeDisplayValue {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]TaskChangeDisplayValue, 0, len(items))
	for _, item := range items {
		out = append(out, TaskChangeDisplayValue{
			Raw:  item,
			Text: taskChangeSetItemText(item),
		})
	}
	return out
}

// taskFieldLabelKey 返回字段对应的 i18n key。
// 前端用当前语言翻译字段名，后端不固定文案。
func taskFieldLabelKey(field string) string {
	return "projectWorkbench.taskHistory.field." + field
}

// taskChangeDisplayText 是标量字段的兜底显示文案。
// 前端应优先用 field + raw + locale 自行格式化，
// 这里只提供一个稳定的兜底，避免 UI 直接展示 nil / unix 秒。
func taskChangeDisplayText(field string, raw any, previous bool) string {
	if raw == nil {
		// 前端会用 unset key 翻译，这里返回空串避免误导；
		// HTTP 层不会把空串当契约，raw 才是权威。
		return ""
	}
	switch field {
	case "due", "wait", "scheduled", "until":
		// unix 秒按 UTC 给一个可读日期兜底；前端按 locale 重排。
		if secs, ok := toInt64(raw); ok {
			return time.Unix(secs, 0).UTC().Format("2006-01-02")
		}
	case "description":
		// 压缩空白后截断，避免 timeline 行被大段文本撑开。
		if s, ok := raw.(string); ok {
			return truncateForTimeline(s)
		}
	}
	return fmt.Sprintf("%v", raw)
}

// taskChangeSetItemText 是集合元素（assignees UserInfo 或 tag 字符串）的兜底文案。
// assignees 优先 display_name -> name -> id；tag 直接文本化。
func taskChangeSetItemText(item any) string {
	if obj, ok := item.(map[string]any); ok {
		if displayName, _ := obj["display_name"].(string); displayName != "" {
			return displayName
		}
		if name, _ := obj["name"].(string); name != "" {
			return name
		}
		if id, _ := obj["id"].(string); id != "" {
			return id
		}
		return ""
	}
	return fmt.Sprintf("%v", item)
}

// truncateForTimeline 压缩换行/多余空白并截断到 80 字符，避免 timeline 行撑开。
func truncateForTimeline(s string) string {
	const max = 80
	out := make([]rune, 0, len(s))
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		out = append(out, r)
		if len(out) >= max {
			out = append(out, []rune("…")...)
			return string(out)
		}
	}
	return string(out)
}

// toInt64 兼容 JSON 解析后的数字类型（float64 / json.Number）。
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}
