package taskseries

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ValidateRule 校验 canonical recurrence 表达式（spec §12）。
//
// 接受：daily|weekly|monthly|<N>days|<N>weeks|<N>months（N 为正整数）。
// 拒绝：biweekly|quarterly|annual|yearly 等别名，以及 0/负数。
func ValidateRule(expr string) error {
	_, _, err := parseRule(expr)
	return err
}

// Next 根据规则从给定时间戳计算下一槽位时间戳。
// monthly 等使用 time.AddDate 月末滚动语义（spec §8.5）。
func Next(fromUnix int64, rule string, loc *time.Location) (int64, error) {
	if loc == nil {
		return 0, fmt.Errorf("taskseries: location 不能为空")
	}
	n, unit, err := parseRule(rule)
	if err != nil {
		return 0, err
	}
	from := time.Unix(fromUnix, 0).In(loc)
	switch unit {
	case "days":
		return from.AddDate(0, 0, n).Unix(), nil
	case "weeks":
		return from.AddDate(0, 0, 7*n).Unix(), nil
	case "months":
		return from.AddDate(0, n, 0).Unix(), nil
	default:
		return 0, fmt.Errorf("taskseries: 不支持的规则 %q", rule)
	}
}

func parseRule(expr string) (int, string, error) {
	switch expr {
	case "daily":
		return 1, "days", nil
	case "weekly":
		return 1, "weeks", nil
	case "monthly":
		return 1, "months", nil
	}
	for _, unit := range []string{"days", "weeks", "months"} {
		if strings.HasSuffix(expr, unit) {
			nStr := strings.TrimSuffix(expr, unit)
			if nStr == "" {
				return 0, "", fmt.Errorf("taskseries: 非法规则 %q", expr)
			}
			n, err := strconv.Atoi(nStr)
			if err != nil || n <= 0 {
				return 0, "", fmt.Errorf("taskseries: 非法规则 %q", expr)
			}
			return n, unit, nil
		}
	}
	return 0, "", fmt.Errorf("taskseries: 非法规则 %q", expr)
}

// ExpandRange 在有界区间 [start, end) 内按 rule-version 分段展开槽位。
//
// 每段槽位：从 effective_from[i] 作为 anchor 按 recurrence_rule[i] 展开，
// 且 slot < effective_from[i+1]（如下一段存在），且 slot <= until（如有），且 [start, end)。
//
// 返回去重、升序排列的 Slot 列表。start/end/until/rule 都基于传入 loc 的本地时间。
//
// 不校验 366 天产品上限，那属于 App 层 TaskViewQuery 的校验职责。
func ExpandRange(versions []RuleVersion, until *int64, start, end int64, loc *time.Location) ([]Slot, error) {
	if end <= start {
		return nil, fmt.Errorf("taskseries: 展开范围 end(%d) 必须 > start(%d)", end, start)
	}
	if loc == nil {
		return nil, fmt.Errorf("taskseries: location 不能为空")
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("taskseries: 至少需要一个 rule version")
	}
	// 校验规则并按 effective_from 严格递增排序+去重。
	sorted := make([]RuleVersion, len(versions))
	copy(sorted, versions)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].EffectiveFrom < sorted[j].EffectiveFrom
	})
	for i, v := range sorted {
		if err := ValidateRule(v.RecurrenceRule); err != nil {
			return nil, fmt.Errorf("taskseries: rule version %d 规则非法: %w", i, err)
		}
		if i > 0 && sorted[i].EffectiveFrom <= sorted[i-1].EffectiveFrom {
			return nil, fmt.Errorf("taskseries: rule version effective_from 必须严格递增")
		}
	}

	var slots []Slot
	for i, seg := range sorted {
		var segUpper int64
		var hasSegUpper bool
		if i+1 < len(sorted) {
			segUpper = sorted[i+1].EffectiveFrom
			hasSegUpper = true
		}
		// segment 槽位 anchor 从 effective_from[i] 开始，按 rule[i] 递增。
		anchor := seg.EffectiveFrom
		// 跳过完全在 start 之前的 anchor，但仍需从 anchor 递增直到进入 [start, end)。
		// 为避免无限循环，限制单段最多展开 10000 个槽位。
		const maxPerSegment = 10000
		count := 0
		for anchor < end {
			if count > maxPerSegment {
				return nil, fmt.Errorf("taskseries: 单段展开超过 %d 槽位，疑似非法参数", maxPerSegment)
			}
			if hasSegUpper && anchor >= segUpper {
				break
			}
			if until != nil && anchor > *until {
				break
			}
			if anchor >= start {
				slots = append(slots, Slot{RecurrenceAt: anchor, Rule: seg.RecurrenceRule})
			}
			next, err := Next(anchor, seg.RecurrenceRule, loc)
			if err != nil {
				return nil, err
			}
			if next <= anchor {
				// 防御：规则不应产生非递增序列。
				return nil, fmt.Errorf("taskseries: 规则 %q 在 %d 产生非递增槽位 %d", seg.RecurrenceRule, anchor, next)
			}
			anchor = next
			count++
		}
	}

	// 去重（跨段边界理论上不应重叠，但防御性去重）+ 排序。
	sort.SliceStable(slots, func(i, j int) bool {
		return slots[i].RecurrenceAt < slots[j].RecurrenceAt
	})
	out := slots[:0]
	for _, s := range slots {
		if len(out) > 0 && out[len(out)-1].RecurrenceAt == s.RecurrenceAt {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ValidateSeries 校验 series 领域对象的基本 invariant（spec §7.1）。
//
// 不校验 created_by 是否真实存在（那是 App 层职责）、也不校验 rule version 段
// 与 first_due 的一致性（那是 App 层在创建/修改事务中校验）。
func ValidateSeries(s Series) error {
	if s.ID == "" {
		return fmt.Errorf("taskseries: ID 不能为空")
	}
	if s.WorkspaceID == "" {
		return fmt.Errorf("taskseries: workspace_id 不能为空")
	}
	if s.ProjectID == "" {
		return fmt.Errorf("taskseries: project_id 不能为空")
	}
	if strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("taskseries: title 不能为空")
	}
	switch s.Status {
	case StatusActive, StatusEnded, StatusStopped:
	default:
		return fmt.Errorf("taskseries: 非法 status %q", s.Status)
	}
	if err := ValidateRule(s.RecurrenceRule); err != nil {
		return fmt.Errorf("taskseries: %w", err)
	}
	if s.FirstDue <= 0 {
		return fmt.Errorf("taskseries: first_due 必须为正")
	}
	if s.Until != nil && *s.Until < s.FirstDue {
		return fmt.Errorf("taskseries: until(%d) 必须 >= first_due(%d)", *s.Until, s.FirstDue)
	}
	switch s.Status {
	case StatusActive:
		if s.EffectiveEndAt != nil {
			return fmt.Errorf("taskseries: active series 不应设置 effective_end_at")
		}
	case StatusEnded, StatusStopped:
		if s.EffectiveEndAt == nil {
			return fmt.Errorf("taskseries: %s series 必须设置 effective_end_at", s.Status)
		}
		if s.StopReason == nil && s.Status == StatusStopped {
			return fmt.Errorf("taskseries: stopped series 必须设置 stop_reason")
		}
	}
	return nil
}
