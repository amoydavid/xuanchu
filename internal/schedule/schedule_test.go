package schedule

import (
	"strings"
	"testing"
	"time"
)

// mustParseTime 解析 RFC3339 时间字符串，测试辅助函数。
func mustParseTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	tm, err := time.Parse(layout, value)
	if err != nil {
		t.Fatalf("解析时间 %q 失败: %v", value, err)
	}
	return tm
}

func TestSpec_Validate(t *testing.T) {
	cases := []struct {
		name    string
		spec    Spec
		wantErr bool
	}{
		{"daily_at 合法", Spec{Type: TypeDailyAt, Value: "09:30"}, false},
		{"daily_at 合法零点", Spec{Type: TypeDailyAt, Value: "00:00"}, false},
		{"daily_at 合法末点", Spec{Type: TypeDailyAt, Value: "23:59"}, false},
		{"daily_at 缺冒号", Spec{Type: TypeDailyAt, Value: "0930"}, true},
		{"daily_at 长度不对", Spec{Type: TypeDailyAt, Value: "9:30"}, true},
		{"daily_at 越界小时", Spec{Type: TypeDailyAt, Value: "24:00"}, true},
		{"daily_at 越界分钟", Spec{Type: TypeDailyAt, Value: "09:60"}, true},
		{"daily_at 空值", Spec{Type: TypeDailyAt, Value: ""}, true},

		{"cron 每工作日", Spec{Type: TypeCron, Value: "0 9 * * 1-5"}, false},
		{"cron 每15分钟", Spec{Type: TypeCron, Value: "*/15 * * * *"}, false},
		{"cron 每周一", Spec{Type: TypeCron, Value: "0 9 * * 1"}, false},
		{"cron 每月1号", Spec{Type: TypeCron, Value: "0 9 1 * *"}, false},
		{"cron 月周别名不支持", Spec{Type: TypeCron, Value: "0 9 * JAN FEB"}, true},
		{"cron 字段过多", Spec{Type: TypeCron, Value: "0 9 * * * *"}, true},
		{"cron 字段过少", Spec{Type: TypeCron, Value: "0 9 *"}, true},
		{"cron 非法字符", Spec{Type: TypeCron, Value: "0 9 # * *"}, true},
		{"cron 空值", Spec{Type: TypeCron, Value: ""}, true},

		{"未知类型", Spec{Type: "hourly", Value: "anything"}, true},
		{"空类型", Spec{Type: "", Value: ""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestSpec_LastFireAt_DailyAt(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 2026-07-23 是周四
	// now = 09:31，已过 09:30 → fire = 09:30, ok
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:31:00+08:00")
	spec := Spec{Type: TypeDailyAt, Value: "09:30", Timezone: "Asia/Shanghai"}
	fire, ok, err := spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("LastFireAt 意外错误: %v", err)
	}
	if !ok {
		t.Fatal("期望 ok=true（已过计划点）")
	}
	wantFire := time.Date(2026, 7, 23, 9, 30, 0, 0, loc)
	if !fire.Equal(wantFire) {
		t.Fatalf("fire = %v, want %v", fire, wantFire)
	}

	// now = 09:29，未过 09:30 → ok=false
	now = mustParseTime(t, time.RFC3339, "2026-07-23T09:29:00+08:00")
	_, ok, err = spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("LastFireAt 意外错误: %v", err)
	}
	if ok {
		t.Fatal("期望 ok=false（未到计划点）")
	}
}

func TestSpec_LastFireAt_DailyAt_DefaultTimezone(t *testing.T) {
	// Timezone 为空时按 Asia/Shanghai 处理
	spec := Spec{Type: TypeDailyAt, Value: "09:30"}
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:31:00+08:00")
	_, ok, err := spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("LastFireAt 意外错误: %v", err)
	}
	if !ok {
		t.Fatal("空时区应回退到 Asia/Shanghai，期望 ok=true")
	}
}

func TestSpec_LastFireAt_DailyAt_InvalidTimezone(t *testing.T) {
	// 时区非法时回退到 time.Local，不应报错
	spec := Spec{Type: TypeDailyAt, Value: "09:30", Timezone: "Foo/Bar"}
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:31:00+08:00")
	_, _, err := spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("非法时区应回退而非报错，得到: %v", err)
	}
}

func TestSpec_LastFireAt_Cron(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// cron: 0 9 * * 1-5（每工作日 9 点）
	// now = 周四 09:01 → 本轮触发点 = 周四 09:00
	spec := Spec{Type: TypeCron, Value: "0 9 * * 1-5", Timezone: "Asia/Shanghai"}
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:01:00+08:00") // 周四
	fire, ok, err := spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("LastFireAt 意外错误: %v", err)
	}
	if !ok {
		t.Fatal("期望 ok=true（工作日 9 点已过）")
	}
	wantFire := time.Date(2026, 7, 23, 9, 0, 0, 0, loc)
	if !fire.Equal(wantFire) {
		t.Fatalf("fire = %v, want %v", fire, wantFire)
	}

	// now = 周四 08:59 → 最近触发点为周三 09:00（<= now 存在）。
	// 注意：LastFireAt 只返回最近触发点，不判断窗口；
	// "今天是否到点"由调用方结合窗口/dedupe 判断（周三 09:00 在 1h 窗口外，不会被触发）。
	now = mustParseTime(t, time.RFC3339, "2026-07-23T08:59:00+08:00")
	fire, ok, err = spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("LastFireAt 意外错误: %v", err)
	}
	if !ok {
		t.Fatal("期望 ok=true（存在 <= now 的触发点周三 09:00）")
	}
	wantPrevFire := time.Date(2026, 7, 22, 9, 0, 0, 0, loc) // 周三
	if !fire.Equal(wantPrevFire) {
		t.Fatalf("fire = %v, want %v（周三 09:00）", fire, wantPrevFire)
	}
}

func TestSpec_LastFireAt_Cron_HighFreq(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// cron: */15 * * * *（每 15 分钟）
	// now = 09:23 → 最近触发点是 09:15
	spec := Spec{Type: TypeCron, Value: "*/15 * * * *", Timezone: "Asia/Shanghai"}
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:23:00+08:00")
	fire, ok, err := spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("LastFireAt 意外错误: %v", err)
	}
	if !ok {
		t.Fatal("期望 ok=true")
	}
	wantFire := time.Date(2026, 7, 23, 9, 15, 0, 0, loc)
	if !fire.Equal(wantFire) {
		t.Fatalf("fire = %v, want %v", fire, wantFire)
	}
}

func TestSpec_LastFireAt_Cron_WeekendBeforeWeekday(t *testing.T) {
	// cron: 0 9 * * 1-5（每工作日）
	// now = 周六 10:00 → 最近触发点是周五 09:00
	spec := Spec{Type: TypeCron, Value: "0 9 * * 1-5", Timezone: "Asia/Shanghai"}
	now := mustParseTime(t, time.RFC3339, "2026-07-25T10:00:00+08:00") // 2026-07-25 是周六
	_, ok, err := spec.LastFireAt(now)
	if err != nil {
		t.Fatalf("LastFireAt 意外错误: %v", err)
	}
	// 周六 10:00 时，最近的工作日触发点（周五 09:00）在 1 小时窗口外。
	// 但 LastFireAt 不带窗口判断，只要存在 <= now 的触发点就返回 ok=true。
	// 窗口判断由调用方外层负责。
	if !ok {
		t.Fatal("周末应有 <= now 的最近触发点（周五），期望 ok=true")
	}
}

func TestSpec_DedupeKey_DailyAt(t *testing.T) {
	spec := Spec{Type: TypeDailyAt, Value: "09:30", Timezone: "Asia/Shanghai"}
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:31:00+08:00")
	key, err := spec.DedupeKey(now)
	if err != nil {
		t.Fatalf("DedupeKey 意外错误: %v", err)
	}
	if key != "2026-07-23" {
		t.Fatalf("key = %q, want %q", key, "2026-07-23")
	}
}

func TestSpec_DedupeKey_Cron(t *testing.T) {
	spec := Spec{Type: TypeCron, Value: "*/15 * * * *", Timezone: "Asia/Shanghai"}
	// now = 09:23 → 最近触发点 09:15 → key 含分钟
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:23:00+08:00")
	key, err := spec.DedupeKey(now)
	if err != nil {
		t.Fatalf("DedupeKey 意外错误: %v", err)
	}
	if key != "2026-07-23T09:15" {
		t.Fatalf("key = %q, want %q（按分钟去重）", key, "2026-07-23T09:15")
	}

	// 验证高频 cron 两次不同分钟产生不同 key（不被按天吞掉）
	now2 := mustParseTime(t, time.RFC3339, "2026-07-23T09:31:00+08:00")
	key2, _ := spec.DedupeKey(now2)
	if key == key2 {
		t.Fatalf("两个不同触发点应产生不同 dedupe key: key=%q key2=%q", key, key2)
	}
}

func TestSpec_DedupeKey_Cron_HighFreqSameMinuteIdempotent(t *testing.T) {
	spec := Spec{Type: TypeCron, Value: "*/15 * * * *", Timezone: "Asia/Shanghai"}
	// 同一分钟内多次调用应返回相同 key（幂等去重）
	now := mustParseTime(t, time.RFC3339, "2026-07-23T09:16:00+08:00")
	now2 := mustParseTime(t, time.RFC3339, "2026-07-23T09:16:59+08:00")
	k1, _ := spec.DedupeKey(now)
	k2, _ := spec.DedupeKey(now2)
	if k1 != k2 {
		t.Fatalf("同一触发点分钟内应幂等: k1=%q k2=%q", k1, k2)
	}
}

func TestSpec_Describe(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{"0 9 * * *", "每天 09:00 触发"},
		{"0 9 * * 1-5", "每工作日 09:00 触发"},
		{"0 9 * * 1", "每周一 09:00 触发"},
		{"0 9 * * 5", "每周五 09:00 触发"},
		{"0 * * * *", "每小时整点触发"},
		{"*/15 * * * *", "每 15 分钟触发"},
		{"0 9 1 * *", "每月 1 日 09:00 触发"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			spec := Spec{Type: TypeCron, Value: tc.expr, Timezone: "Asia/Shanghai"}
			got := spec.Describe()
			if got != tc.want {
				t.Fatalf("Describe() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSpec_Describe_NonStandardFallback(t *testing.T) {
	// 非常规表达式（小时为范围、分钟为步进的组合）回退为原始表达式
	spec := Spec{Type: TypeCron, Value: "*/20 8-18 * * 1-5", Timezone: "Asia/Shanghai"}
	got := spec.Describe()
	if got != spec.Value {
		t.Fatalf("非常规表达式应回退为原始表达式 %q，得到 %q", spec.Value, got)
	}
}

func TestSpec_Describe_DailyAt(t *testing.T) {
	spec := Spec{Type: TypeDailyAt, Value: "09:30", Timezone: "Asia/Shanghai"}
	got := spec.Describe()
	if !strings.Contains(got, "09:30") {
		t.Fatalf("daily_at 解读 %q 应包含时间 09:30", got)
	}
}
