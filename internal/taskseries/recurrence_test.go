package taskseries

import (
	"testing"
	"time"
)

// loc 是测试用的 Asia/Shanghai 时区，日期边界按本地 00:00 / 23:59:59 处理。
func shanghaiLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	return loc
}

// dayAt 返回给定本地日期 23:59:59 的 Unix 时间戳，模拟 date-only due/until 存储。
func dayAt(t *testing.T, loc *time.Location, date string) int64 {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", date+" 23:59:59", loc)
	if err != nil {
		t.Fatalf("解析日期 %s 失败: %v", date, err)
	}
	return ts.Unix()
}

func TestValidateRuleAcceptsCanonical(t *testing.T) {
	for _, value := range []string{"daily", "weekly", "monthly", "1days", "2days", "1weeks", "2weeks", "1months", "3months", "12months"} {
		if err := ValidateRule(value); err != nil {
			t.Fatalf("ValidateRule(%q) 不应失败: %v", value, err)
		}
	}
}

func TestValidateRuleRejectsAliases(t *testing.T) {
	for _, value := range []string{"", "biweekly", "quarterly", "annual", "yearly", "0days", "0weeks", "0months", "-1days", "abc", "2day", "1month"} {
		if err := ValidateRule(value); err == nil {
			t.Fatalf("ValidateRule(%q) 应失败", value)
		}
	}
}

func TestNextAdvancesByRule(t *testing.T) {
	loc := shanghaiLocation(t)
	cases := []struct {
		name string
		rule string
		from int64
		want int64
	}{
		{"daily", "daily", dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-07-12")},
		{"weekly", "weekly", dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-07-18")},
		{"monthly", "monthly", dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-08-11")},
		{"2days", "2days", dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-07-13")},
		{"2weeks", "2weeks", dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-07-25")},
		{"3months", "3months", dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-10-11")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Next(tc.from, tc.rule, loc)
			if err != nil {
				t.Fatalf("Next 失败: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Next(%q) = %d, want %d", tc.rule, got, tc.want)
			}
		})
	}
}

func TestNextMonthUsesGoAddDateSemantics(t *testing.T) {
	// spec §8.5：monthly 继续使用 Go time.AddDate(0, n, 0) 月末语义。
	// Go 的 AddDate 会规范化日期：1 月 31 日加 1 个月得到 3 月 3 日（2 月无 31 日），
	// 而不是 clamp 到 2 月 28 日。这里锁定 Go 的实际行为，避免后续被"优化"。
	loc := shanghaiLocation(t)
	got, err := Next(dayAt(t, loc, "2026-01-31"), "monthly", loc)
	if err != nil {
		t.Fatalf("Next 失败: %v", err)
	}
	if want := dayAt(t, loc, "2026-03-03"); got != want {
		t.Fatalf("Go AddDate 月末语义 got=%d want=%d (2026-03-03)", got, want)
	}
	// 正常月份不受影响：7 月 11 日 -> 8 月 11 日。
	got2, err := Next(dayAt(t, loc, "2026-07-11"), "monthly", loc)
	if err != nil {
		t.Fatalf("Next 失败: %v", err)
	}
	if want := dayAt(t, loc, "2026-08-11"); got2 != want {
		t.Fatalf("normal month got=%d want=%d", got2, want)
	}
}

func TestExpandRangeDailyLeftClosedRightOpen(t *testing.T) {
	loc := shanghaiLocation(t)
	versions := []RuleVersion{{EffectiveFrom: dayAt(t, loc, "2026-07-11"), RecurrenceRule: "daily"}}
	slots, err := ExpandRange(versions, nil, dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-07-14")+1, loc)
	if err != nil {
		t.Fatalf("ExpandRange 失败: %v", err)
	}
	want := []int64{
		dayAt(t, loc, "2026-07-11"),
		dayAt(t, loc, "2026-07-12"),
		dayAt(t, loc, "2026-07-13"),
		dayAt(t, loc, "2026-07-14"),
	}
	if len(slots) != len(want) {
		t.Fatalf("len=%d want=%d: %#v", len(slots), len(want), slots)
	}
	for i := range want {
		if slots[i].RecurrenceAt != want[i] {
			t.Fatalf("slot[%d]=%d want=%d", i, slots[i].RecurrenceAt, want[i])
		}
		if slots[i].Rule != "daily" {
			t.Fatalf("slot[%d].Rule=%q want=daily", i, slots[i].Rule)
		}
	}
}

func TestExpandRangeUsesRuleVersionAnchors(t *testing.T) {
	loc := shanghaiLocation(t)
	versions := []RuleVersion{
		{EffectiveFrom: dayAt(t, loc, "2026-07-01"), RecurrenceRule: "weekly"},
		{EffectiveFrom: dayAt(t, loc, "2026-07-15"), RecurrenceRule: "daily"},
	}
	// 左闭右开 [07-01, 07-18+1)。
	slots, err := ExpandRange(versions, nil, dayAt(t, loc, "2026-07-01"), dayAt(t, loc, "2026-07-18")+1, loc)
	if err != nil {
		t.Fatalf("ExpandRange 失败: %v", err)
	}
	want := []int64{
		dayAt(t, loc, "2026-07-01"),
		dayAt(t, loc, "2026-07-08"),
		dayAt(t, loc, "2026-07-15"),
		dayAt(t, loc, "2026-07-16"),
		dayAt(t, loc, "2026-07-17"),
		dayAt(t, loc, "2026-07-18"),
	}
	if len(slots) != len(want) {
		t.Fatalf("len=%d want=%d: %#v", len(slots), len(want), slots)
	}
	for i := range want {
		if slots[i].RecurrenceAt != want[i] {
			t.Fatalf("slot[%d]=%d want=%d", i, slots[i].RecurrenceAt, want[i])
		}
	}
	// weekly 段规则保持 weekly，daily 段规则保持 daily。
	if slots[0].Rule != "weekly" {
		t.Fatalf("weekly 段槽位规则 = %q want=weekly", slots[0].Rule)
	}
	if slots[2].Rule != "daily" {
		t.Fatalf("daily 段槽位规则 = %q want=daily", slots[2].Rule)
	}
}

func TestExpandRangeInclusiveUntil(t *testing.T) {
	loc := shanghaiLocation(t)
	versions := []RuleVersion{{EffectiveFrom: dayAt(t, loc, "2026-07-11"), RecurrenceRule: "daily"}}
	until := dayAt(t, loc, "2026-07-14")
	// until 是包含式上界，07-14 应该出现。
	slots, err := ExpandRange(versions, &until, dayAt(t, loc, "2026-07-11"), dayAt(t, loc, "2026-07-20"), loc)
	if err != nil {
		t.Fatalf("ExpandRange 失败: %v", err)
	}
	if len(slots) != 4 {
		t.Fatalf("包含式 until got=%d slots want=4: %#v", len(slots), slots)
	}
	if slots[len(slots)-1].RecurrenceAt != until {
		t.Fatalf("最后槽位=%d want=%d", slots[len(slots)-1].RecurrenceAt, until)
	}
}

func TestExpandRangeRejectsBadInput(t *testing.T) {
	loc := shanghaiLocation(t)
	day := dayAt(t, loc, "2026-07-11")
	versions := []RuleVersion{{EffectiveFrom: day, RecurrenceRule: "daily"}}
	cases := []struct {
		name     string
		versions []RuleVersion
		until    *int64
		start    int64
		end      int64
		loc      *time.Location
	}{
		{"end <= start", versions, nil, day, day, loc},
		{"nil location", versions, nil, day, day + 1, nil},
		{"duplicate effective_from", []RuleVersion{
			{EffectiveFrom: day, RecurrenceRule: "daily"},
			{EffectiveFrom: day, RecurrenceRule: "weekly"},
		}, nil, day, day + 1, loc},
		{"invalid rule", []RuleVersion{{EffectiveFrom: day, RecurrenceRule: "biweekly"}}, nil, day, day + 1, loc},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ExpandRange(tc.versions, tc.until, tc.start, tc.end, tc.loc); err == nil {
				t.Fatal("ExpandRange 应失败")
			}
		})
	}
}

func TestValidateSeriesRejectsInvalid(t *testing.T) {
	base := Series{
		ID: "s1", WorkspaceID: "ws", ProjectID: "proj", Title: "巡检",
		Status: StatusActive, RecurrenceRule: "daily", FirstDue: 100, CreatedBy: "u1", CreatedAt: 100, ModifiedAt: 100,
	}
	cases := []struct {
		name   string
		mutate func(*Series)
	}{
		{"empty title", func(s *Series) { s.Title = "" }},
		{"empty project", func(s *Series) { s.ProjectID = "" }},
		{"empty workspace", func(s *Series) { s.WorkspaceID = "" }},
		{"empty id", func(s *Series) { s.ID = "" }},
		{"invalid status", func(s *Series) { s.Status = "running" }},
		{"active with end", func(s *Series) { x := int64(200); s.EffectiveEndAt = &x }},
		{"ended without end", func(s *Series) { s.Status = StatusEnded }},
		{"stopped without end", func(s *Series) { s.Status = StatusStopped }},
		{"invalid rule", func(s *Series) { s.RecurrenceRule = "yearly" }},
		{"until before first_due", func(s *Series) { u := int64(50); s.Until = &u }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := base
			tc.mutate(&value)
			if err := ValidateSeries(value); err == nil {
				t.Fatal("ValidateSeries 应失败")
			}
		})
	}
}

func TestValidateSeriesAcceptsValid(t *testing.T) {
	end := int64(500)
	cases := []Series{
		{
			ID: "s1", WorkspaceID: "ws", ProjectID: "proj", Title: "巡检",
			Status: StatusActive, RecurrenceRule: "daily", FirstDue: 100, CreatedBy: "u1", CreatedAt: 100, ModifiedAt: 100,
		},
		{
			ID: "s2", WorkspaceID: "ws", ProjectID: "proj", Title: "巡检",
			Status: StatusStopped, RecurrenceRule: "daily", FirstDue: 100, CreatedBy: "u1", CreatedAt: 100, ModifiedAt: 100,
			EffectiveEndAt: &end, StopReason: strPtr("project_archived"),
		},
	}
	for i, s := range cases {
		if err := ValidateSeries(s); err != nil {
			t.Fatalf("case %d ValidateSeries 失败: %v", i, err)
		}
	}
}

func strPtr(s string) *string { return &s }
