# 实施计划：自动化定时配置增强（cron + 可视化）

> **对应 spec：** [2026-07-23-automation-cron-scheduling-design.md](../specs/2026-07-23-automation-cron-scheduling-design.md)
>
> 本计划按 spec 拆成 9 个可独立提交的任务。每个任务都给出改动文件、验收标准和测试命令。按编号顺序执行；任务 2/3 依赖任务 1，任务 6/7 依赖任务 2/3/5，其余可并行。

## 任务依赖关系

```text
1 (schedule 包) ─┬─→ 2 (项目自动化后端) ──┐
                 └─→ 3 (提醒规则后端)  ──┤
4 (task series bug，独立) ────────────────┤
5 (前端 cron 组件，独立) ─→ 6 (项目自动化前端) ──┤
                     └──→ 7 (提醒规则前端)  ──┤
                                           ├──→ 8 (e2e)
                                           └──→ 9 (文档)
```

---

## 任务 1：引入 cron 依赖 + 建 `internal/schedule` 包

**目标：** 建立公共调度包，收敛 daily_at 逻辑并新增 cron 能力。这是后续所有后端改动的基础。

**改动文件：**
- `go.mod` / `go.sum`：新增 `github.com/robfig/cron/v3`（纯 Go 零 CGO）。
- `internal/schedule/schedule.go`（新建）：`Spec` 结构体 + `Validate()` / `LastFireAt(now)` / `DedupeKey(now)` / `Describe()`。
- `internal/schedule/schedule_test.go`（新建）。

**实现要点：**

```go
package schedule

import (
    "fmt"
    "strings"
    "time"

    "github.com/robfig/cron/v3"
)

const (
    TypeDailyAt = "daily_at"
    TypeCron    = "cron"
    DefaultTimezone = "Asia/Shanghai"
)

type Spec struct {
    Type     string
    Value    string
    Timezone string
}

func (s Spec) location() *time.Location {
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

func (s Spec) Validate() error {
    switch s.Type {
    case TypeDailyAt:
        if len(s.Value) != 5 || s.Value[2] != ':' {
            return fmt.Errorf("schedule: 无效的 daily_at 值 %q", s.Value)
        }
        if _, err := time.Parse("15:04", s.Value); err != nil {
            return fmt.Errorf("schedule: 无效的 daily_at 值 %q", s.Value)
        }
        return nil
    case TypeCron:
        if _, err := cron.ParseStandard(s.Value); err != nil {
            return fmt.Errorf("schedule: 无效的 cron 表达式 %q: %w", s.Value, err)
        }
        return nil
    default:
        return fmt.Errorf("schedule: 不支持的调度类型 %q", s.Type)
    }
}

// LastFireAt 返回 <= now 的最近触发时刻。
func (s Spec) LastFireAt(now time.Time) (fire time.Time, ok bool, err error) {
    loc := s.location()
    local := now.In(loc)
    switch s.Type {
    case TypeDailyAt:
        parsed, err := time.Parse("15:04", s.Value)
        if err != nil {
            return time.Time{}, false, err
        }
        today := time.Date(local.Year(), local.Month(), local.Day(),
            parsed.Hour(), parsed.Minute(), 0, 0, loc)
        if !local.Before(today) {
            return today, true, nil
        }
        return time.Time{}, false, nil
    case TypeCron:
        sched, err := cron.ParseStandard(s.Value)
        if err != nil {
            return time.Time{}, false, err
        }
        // 反推最近触发点：从 now 往前找，Next(prev) <= now 的最大 prev。
        // 策略：从 now - 1 分钟起逐步回退，调 Next 直到命中 <= now。
        // 上限回退 1440 分钟（一天），防死循环。
        candidate := local.Add(-time.Minute)
        for i := 0; i < 1440; i++ {
            next := sched.Next(candidate)
            if !next.After(local) && !next.IsZero() {
                return next, true, nil
            }
            if next.IsZero() {
                break
            }
            candidate = next.Add(-time.Minute)
        }
        return time.Time{}, false, nil
    default:
        return time.Time{}, false, fmt.Errorf("schedule: 不支持的调度类型 %q", s.Type)
    }
}

func (s Spec) DedupeKey(now time.Time) (string, error) {
    loc := s.location()
    local := now.In(loc)
    switch s.Type {
    case TypeDailyAt:
        return local.Format("2006-01-02"), nil
    case TypeCron:
        fire, ok, err := s.LastFireAt(now)
        if err != nil {
            return "", err
        }
        if !ok {
            return "", fmt.Errorf("schedule: 当前时刻无触发点，无法生成 dedupe key")
        }
        return fire.Format("2006-01-02T15:04"), nil
    default:
        return "", fmt.Errorf("schedule: 不支持的调度类型 %q", s.Type)
    }
}
```

`Describe()` 实现一张模式表（见 spec §11.2），未匹配回退原始表达式。

**验收标准：**
- daily_at 行为与原 `automationScheduleDue` 完全一致（不回归）。
- cron 合法表达式能 `Validate` 通过；非法表达式返回错误。
- `LastFireAt` 在窗口内返回触发点，窗口外返回 `(_, false, nil)`。
- `DedupeKey`：daily_at 返回日期串；cron 返回分钟级串。
- `Describe` 覆盖 spec §11.2 全部预设 + 回退。

**测试命令：**
```bash
go test ./internal/schedule/...
CGO_ENABLED=0 go test ./internal/schedule/...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

---

## 任务 2：项目自动化 app 层改造

**依赖：** 任务 1。

**改动文件：**
- `internal/app/project_automation.go`：
  - L338-345 `normalizeProjectAutomationAddInput` 的 schedule 校验分支改为调 `schedule.Spec{...}.Validate()`。
  - L378-384 `validHHMM` 删除。
- `internal/app/project_automation_scheduler.go`：
  - L98 `cfg.ScheduleType != "daily_at" || !automationScheduleDue(cfg, now)` 改为基于 `schedule.Spec.LastFireAt` 判断（daily_at 保留 1h 窗口语义）。
  - L149-163 `automationScheduleDue` 替换/删除。
  - L101 dedupe key 末段改用 `schedule.Spec.DedupeKey(now)`。
  - L166-172 `automationLocalDate` 删除（被 DedupeKey 取代）。

**改造后的调度器核心（保留 1h 窗口）：**
```go
cfg := decodeProjectAutomationTriggerConfig(rule.TriggerConfigJSON)
spec := schedule.Spec{Type: cfg.ScheduleType, Value: cfg.ScheduleValue, Timezone: cfg.Timezone}
fire, ok, err := spec.LastFireAt(s.clock.Now())
if err != nil || !ok {
    continue
}
local := s.clock.Now().In(spec.Location()) // 暴露 location 给外层
// 保留 1 小时窗口：仅在 fire 后 1 小时内入队
if local.Sub(fire) > time.Hour {
    continue
}
key := fmt.Sprintf("%s:%s:%s:%s", rule.WorkspaceID, rule.ProjectID, rule.ID, mustDedupe(spec, now))
```

**新增测试：** `internal/app/project_automation_scheduler_test.go` 新增 `TestProjectAutomationSchedulerEnqueuesCronRuleOnce`——建 cron 规则 `0 9 * * 1-5`，clock 设在周四 09:01（窗口内），断言入队 1 次；再跑返回 0（dedupe）。新增 `TestProjectAutomationSchedulerHighFreqCronNotSwallowed`——建 `*/15 * * * *`，clock 设在 09:16 和 09:31 两次跑，断言两次都入队（验证分钟级 dedupe）。

**验收标准：**
- 现有 daily_at 测试全部通过（不回归）。
- cron 规则能在命中时刻入队。
- 高频 cron 不被按天吞掉。

**测试命令：**
```bash
go test ./internal/app/... -run ProjectAutomation
CGO_ENABLED=0 go test ./...
```

---

## 任务 3：提醒规则 app 层改造

**依赖：** 任务 1。

**改动文件：**
- `internal/storage/models.go` L377-401：`ReminderRule` 新增 `Timezone string` 字段（`gorm:"not null;default:'Asia/Shanghai'"`）。
- `internal/app/notification.go`：
  - L99-149 `ReminderRuleAddInput` / `ReminderRuleModifyInput` / `ReminderRuleView` 新增 `Timezone`。
  - L1041-1070 `normalizeReminderScheduleFilter`：签名加 `timezone` 参数；L1053 改为支持 cron（调 `schedule.Spec.Validate`）；旧 `hourly`/`weekly_at` 显式拒绝并提示。
- `internal/app/notification_scheduler.go`：
  - L221-241 `scheduledRuleDueForRun`：改用 `schedule.Spec.LastFireAt`（**无窗口**，与原行为一致）。
  - L511-517 `scheduleDateKey`：加 cron 分支（分钟级）。
- `internal/httpapi/notifications.go`：request/response DTO 新增 `timezone`。
- `internal/cli/notification.go` / `internal/mcpserver/tools_notification.go` / `internal/remote/notification.go`：各入口 input/view 同步加 `timezone`。

**验收标准：**
- 提醒规则可创建 daily_at（旧行为）和 cron 规则。
- 旧 `hourly`/`weekly_at` 值被明确拒绝（错误信息提示改用 cron）。
- cron 规则到期判断正确，dedupe 按分钟。
- `Timezone` 字段在所有入口读写正确。

**测试命令：**
```bash
go test ./internal/app/... -run Reminder
go test ./internal/storage/...
CGO_ENABLED=0 go test ./...
```

---

## 任务 4：修 task series MCP 描述 bug（独立）

**改动文件：**
- `internal/mcpserver/tools_task_series.go` L23：jsonschema 描述 `FREQ=WEEKLY;BYDAY=MO` 改为 `"daily, weekly, monthly, <N>days, <N>weeks, <N>months (e.g. 2weeks, 3months)"`，与 CLI flag（series.go L104）对齐。

**验收标准：** MCP 工具描述不再误导用户使用后端不支持的 RRULE 语法。

**测试命令：**
```bash
go test ./internal/mcpserver/...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

---

## 任务 5：前端 cron 输入公共组件

**改动文件：**
- `web/src/features/workspace/schedule/cron-input.tsx`（新建）：实现 spec §7.1/§7.2 的 cron 输入器——表达式输入框、校验图标、6 个预设按钮、时区下拉、中文解读区。props 接收 `value`/`timezone`/`onChange`。
- `web/src/features/workspace/schedule/cron-input.ts`（新建）：预设表（`PRESETS`）、轻量 cron 合法性校验（5 段切分）、`describe` 调用后端或本地映射。
- `web/src/features/workspace/schedule/cron-input.test.tsx`（新建）：组件单测。

**预设表（与后端 spec §11.2 一致）：**
```ts
export const CRON_PRESETS = [
  { label: "每天 9 点",     expr: "0 9 * * *" },
  { label: "每工作日 9 点", expr: "0 9 * * 1-5" },
  { label: "每周一 9 点",   expr: "0 9 * * 1" },
  { label: "每小时整点",    expr: "0 * * * *" },
  { label: "每 15 分钟",    expr: "*/15 * * * *" },
  { label: "每月 1 号 9 点",expr: "0 9 1 * *" },
]
```

**验收标准：**
- 组件可独立使用，受控（value/onChange）。
- 预设点击回填表达式并更新解读。
- 非法表达式显示 `✗` + 警告。

**测试命令：**
```bash
cd web && npm run test -- cron-input
cd web && npm run lint
```

---

## 任务 6：项目自动化前端（原型一）

**依赖：** 任务 2（API 契约）、任务 5（cron 组件）。

**改动文件：**
- `web/src/features/workspace/project-workbench/automations/project-automations-api.ts` L20：`schedule_type?: "daily_at"` → `"daily_at" | "cron"`。
- `web/src/features/workspace/project-workbench/automations/automation-rule-dialog.tsx` L166-182：实现 spec §7.1——单选切换「每天定时 / 自定义 cron」，cron 分支用任务 5 的 `<CronInput>`。
- `web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx`：新增「每周项目回顾」模板（cron `0 9 * * 1`）。

**验收标准：**
- 选 cron 时展示 cron 输入器；选 daily_at 时展示原时间框。
- 保存 cron 规则后回显正确。
- 解读实时更新。

**测试命令：**
```bash
cd web && npm run build && npm run lint
```

---

## 任务 7：提醒规则前端（原型二）

**依赖：** 任务 3（API 契约）、任务 5（cron 组件）。

**改动文件：**
- `web/src/features/workspace/outbound/rules/reminder-rule-list.tsx`：
  - L51 `SCHEDULE_TYPES` 移除 `hourly`/`weekly_at`，改为 `["daily_at", "cron"]`。
  - L322-327 实现原型二——type 下拉 + cron 输入器（任务 5）+ 时区字段。

**验收标准：**
- 下拉只剩 daily_at / cron 两项。
- cron 输入器 + 时区 + 解读正常工作。
- 保存后回显。

**测试命令：**
```bash
cd web && npm run build && npm run lint
```

---

## 任务 8：e2e 集成测试

**依赖：** 任务 2、3。

**改动文件：**
- `tests/integration/e2e_project_automation_test.go`：新增 cron 规则全链路——HTTP 创建 cron 规则 → 调度器命中 → delivery 入队 → 去重。
- `tests/integration/` 下提醒规则 e2e（若已有则扩展）：cron 提醒规则全链路。

**验收标准：**
- HTTP API 创建 cron 规则成功。
- 调度器在 cron 命中时刻入队 delivery。
- 同一触发点（同分钟）去重生效。

**测试命令：**
```bash
go test ./tests/integration/... -run Automation
go test ./tests/integration/... -run Reminder
```

---

## 任务 9：文档同步

**改动文件：**
- `README.md`：自动化、提醒规则新增 cron 定时能力说明 + 配置示例。
- `ROADMAP.md`：记录本能力进入对应 milestone。

**验收标准：** 文档与代码行为一致；cron 配置示例可复制使用。

---

## 全局验证（所有任务完成后）

```bash
# 后端
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu

# 前端
cd web && npm run build && npm run lint

# 集成测试
go test ./tests/integration/...
```

全部通过后，提交按任务粒度拆分（每个任务一个提交，提交信息用中文，遵循 `feat:`/`fix:` 前缀）。
