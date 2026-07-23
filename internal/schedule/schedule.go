// Package schedule 提供统一的调度规格校验、到期判断和去重标识，
// 供项目自动化和提醒规则两个子系统共用。
//
// 支持两种调度类型：
//   - daily_at: 每天固定 HH:MM（与历史行为一致，按天去重）
//   - cron:     标准 5 段 cron 表达式（分 时 日 月 周，按分钟去重以支持高频）
//
// 循环任务系列（taskseries）不使用本包，它有自己的日历槽位语义。
package schedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	// TypeDailyAt 每天固定时刻触发，Value 为 HH:MM。
	TypeDailyAt = "daily_at"
	// TypeCron 标准 5 段 cron 表达式触发。
	TypeCron = "cron"
	// DefaultTimezone 未指定时区时的默认值。
	DefaultTimezone = "Asia/Shanghai"
)

// Spec 描述一个调度规格。
type Spec struct {
	Type     string // "daily_at" | "cron"
	Value    string // daily_at: "09:30"；cron: "0 9 * * 1-5"
	Timezone string // 缺省 Asia/Shanghai；空串按默认处理
}

// Location 返回 Spec 对应的时区。
// 时区非法时回退到 time.Local（与历史 automationScheduleDue 行为一致）。
// 导出供调用方做窗口判断时复用同一时区。
func (s Spec) Location() *time.Location {
	tz := strings.TrimSpace(s.Timezone)
	if tz == "" {
		tz = DefaultTimezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Local
	}
	return loc
}

// Validate 校验 Type + Value 组合是否合法。
func (s Spec) Validate() error {
	switch s.Type {
	case TypeDailyAt:
		value := strings.TrimSpace(s.Value)
		if len(value) != 5 || value[2] != ':' {
			return fmt.Errorf("schedule: 无效的 daily_at 值 %q", s.Value)
		}
		if _, err := time.Parse("15:04", value); err != nil {
			return fmt.Errorf("schedule: 无效的 daily_at 值 %q", s.Value)
		}
		return nil
	case TypeCron:
		value := strings.TrimSpace(s.Value)
		if value == "" {
			return fmt.Errorf("schedule: cron 表达式不能为空")
		}
		if _, err := cron.ParseStandard(value); err != nil {
			return fmt.Errorf("schedule: 无效的 cron 表达式 %q: %w", s.Value, err)
		}
		return nil
	default:
		return fmt.Errorf("schedule: 不支持的调度类型 %q", s.Type)
	}
}

// LastFireAt 返回 <= now 的最近一次触发时刻。
//
// 返回值：
//   - fire: 触发时刻（在 Spec 时区下）
//   - ok:   是否存在 <= now 的触发点
//   - err:  Spec 非法时返回错误
//
// 注意：本方法不做窗口判断（"是否在触发点后 N 小时内"），窗口语义由调用方外层决定。
// daily_at: 返回今天的计划时刻（若 now 已过该时刻），否则 ok=false。
// cron:     用 cron.Next 反推最近触发点。
func (s Spec) LastFireAt(now time.Time) (fire time.Time, ok bool, err error) {
	loc := s.Location()
	local := now.In(loc)
	switch s.Type {
	case TypeDailyAt:
		value := strings.TrimSpace(s.Value)
		parsed, perr := time.Parse("15:04", value)
		if perr != nil {
			return time.Time{}, false, fmt.Errorf("schedule: 无效的 daily_at 值 %q", s.Value)
		}
		today := time.Date(local.Year(), local.Month(), local.Day(),
			parsed.Hour(), parsed.Minute(), 0, 0, loc)
		if !local.Before(today) {
			return today, true, nil
		}
		return time.Time{}, false, nil
	case TypeCron:
		value := strings.TrimSpace(s.Value)
		sched, perr := cron.ParseStandard(value)
		if perr != nil {
			return time.Time{}, false, fmt.Errorf("schedule: 无效的 cron 表达式 %q: %w", s.Value, perr)
		}
		return lastCronFire(sched, local)
	default:
		return time.Time{}, false, fmt.Errorf("schedule: 不支持的调度类型 %q", s.Type)
	}
}

// lastCronFire 反推 cron 调度的最近触发点（<= now）。
//
// robfig/cron 只提供 Next（求 > t 的下次触发），没有 Prev。
// 这里用二分搜索在 [now-1年, now] 区间内找最大的 candidate，使得
// Next(candidate) <= now。该 Next 返回值即为最近触发点。
//
// 二分区间以 cron 最大年周期（1 年）为下界，足够覆盖所有合法 cron。
func lastCronFire(sched cron.Schedule, now time.Time) (time.Time, bool, error) {
	lo := now.AddDate(-1, 0, 0)
	hi := now
	for !hi.Before(lo) {
		mid := lo.Add(hi.Sub(lo) / 2).Truncate(time.Minute)
		n := sched.Next(mid)
		if n.IsZero() {
			return time.Time{}, false, nil
		}
		if n.After(now) {
			hi = mid.Add(-time.Minute)
		} else {
			lo = mid.Add(time.Minute)
		}
	}
	// 验证收敛点：优先用 lo 的前驱
	if candidate := sched.Next(lo.Add(-time.Minute)); !candidate.IsZero() && !candidate.After(now) {
		return candidate, true, nil
	}
	if candidate := sched.Next(hi.Add(-time.Minute)); !candidate.IsZero() && !candidate.After(now) {
		return candidate, true, nil
	}
	return time.Time{}, false, nil
}

// DedupeKey 返回本次触发点的去重标识。
//
// daily_at: 规则时区日期 "2006-01-02"（按天去重，与历史行为一致）。
// cron:     触发时刻 "2006-01-02T15:04"（按分钟去重，支持高频规则）。
//
// 若当前时刻没有 <= now 的触发点（ok=false），返回错误。
func (s Spec) DedupeKey(now time.Time) (string, error) {
	switch s.Type {
	case TypeDailyAt:
		return now.In(s.Location()).Format("2006-01-02"), nil
	case TypeCron:
		fire, ok, err := s.LastFireAt(now)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("schedule: 当前时刻无 cron 触发点，无法生成 dedupe key")
		}
		return fire.Format("2006-01-02T15:04"), nil
	default:
		return "", fmt.Errorf("schedule: 不支持的调度类型 %q", s.Type)
	}
}

// Describe 返回中文人类可读描述，用于 UI 展示。
//
// 覆盖常见 cron 模式（见 spec §11.2）；未匹配的非常规表达式回退为原始表达式。
// daily_at 返回 "每天 HH:MM 触发"。
func (s Spec) Describe() string {
	switch s.Type {
	case TypeDailyAt:
		value := strings.TrimSpace(s.Value)
		return fmt.Sprintf("每天 %s 触发", value)
	case TypeCron:
		return describeCron(strings.TrimSpace(s.Value))
	default:
		return s.Type
	}
}

// describeCron 把常见 cron 模式映射为中文。
// 周几映射表（0=周日，与标准 cron 一致）。
var cronWeekdays = map[int]string{
	0: "日", 1: "一", 2: "二", 3: "三", 4: "四", 5: "五", 6: "六",
}

func describeCron(expr string) string {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return expr
	}
	minute, hour, dayOfMonth, month, week := fields[0], fields[1], fields[2], fields[3], fields[4]
	// 仅当月字段为通配时才做模式匹配，否则表达式带月维度，回退
	if month != "*" {
		return expr
	}
	// 小时和分钟必须是简单单值（非范围/列表/步进），否则不匹配时间类预设
	timeOK := isSimpleField(hour) && isSimpleField(minute)
	hhmm := func() string {
		return fmt.Sprintf("%s:%s", pad2(hour), pad2(minute))
	}

	// 每N分钟：*/N * * * *
	if strings.HasPrefix(minute, "*/") && hour == "*" && dayOfMonth == "*" && week == "*" {
		n := strings.TrimPrefix(minute, "*/")
		return fmt.Sprintf("每 %s 分钟触发", n)
	}
	// 每小时整点：0 * * * *
	if minute == "0" && hour == "*" && dayOfMonth == "*" && week == "*" {
		return "每小时整点触发"
	}
	// 每天 HH:MM：M H * * *
	if timeOK && dayOfMonth == "*" && week == "*" {
		return fmt.Sprintf("每天 %s 触发", hhmm())
	}
	// 月维度：M H D * *
	if timeOK && dayOfMonth != "*" && week == "*" {
		return fmt.Sprintf("每月 %s 日 %s 触发", dayOfMonth, hhmm())
	}
	// 周维度：M H * * W
	if timeOK && dayOfMonth == "*" && week != "*" {
		if week == "1-5" {
			return fmt.Sprintf("每工作日 %s 触发", hhmm())
		}
		if wd, ok := parseWeekday(week); ok {
			return fmt.Sprintf("每周%s %s 触发", wd, hhmm())
		}
	}
	// 未匹配，回退原始表达式
	return expr
}

// isSimpleField 判断 cron 字段是否为简单单值（纯数字），不含 -/, 。
func isSimpleField(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseWeekday 解析单数字星期（0-6），返回中文。
func parseWeekday(s string) (string, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", false
		}
		n = n*10 + int(r-'0')
	}
	wd, ok := cronWeekdays[n]
	return wd, ok
}

// pad2 把单个数字补成两位（"9" → "09"）。
func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
