# 自动化定时配置增强：cron 表达式与可视化配置设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-23
**状态：** 设计中
**范围：** 为项目自动化和提醒规则引入标准 cron 表达式定时能力，并提供可视化配置（预设按钮 + 中文解读）；收敛两处重复的 daily_at 逻辑到公共调度包；修复提醒规则 UI 列出但后端不支持的假 cron 选项。

## 0. 产品结论

璇础目前有三套相互独立的调度子系统，但定时表达都很受限，且**全仓库零 cron 能力**：

| 子系统 | 当前定时方式 | 主要局限 |
|---|---|---|
| 项目自动化（巡检/拉群） | `daily_at` + `HH:MM` 文本框 | 不能设"每周一""每工作日""每 N 分钟" |
| 提醒规则（任务到期提醒） | UI 列了 4 选项，后端只认 `daily_at` | 选 `cron`/`hourly`/`weekly_at` 会保存失败（**bug**） |
| 循环任务系列（定期生成任务） | 6 个固定频率预设 | 无"星期几/几号"维度，但语义与 cron 不同，本期不动 |

本规格引入标准 cron 表达式，但**不要求用户手写 cron**——核心是可视化：常用场景一键预设，配以中文解读（如"每工作日 09:00 触发"），让普通用户也能配出"每周一回顾""每 15 分钟巡检""每月 1 号结算"这类规则。手写 cron 作为高级选项保留。

**关键边界判断：循环任务系列不上 cron。** 它和前两者的语义不同（date-only 槽位 + backlog 补齐 + 月末滚动 + anchor 版本历史），而 cron 是"无 anchor 的时刻触发"。强行套 cron 会破坏已解决的不变量。循环任务的定时增强单独立项。本规格只修它的一个相关 bug（MCP 工具描述错误）。

## 1. 背景

当前三处"判断定时规则是否该触发"的逻辑高度重复，且都硬编码 `daily_at`：

- 项目自动化：`internal/app/project_automation_scheduler.go` 的 `automationScheduleDue`（`time.Parse("15:04", scheduleValue)` + 日历比较 + 1 小时窗口）。
- 提醒规则：`internal/app/notification_scheduler.go` 的 `scheduledRuleDueForRun`（同款 `time.Parse` + 日历比较，无窗口）。
- 两处的 dedupe key 都按"天"（`2006-01-02`）。

`go.mod` 没有任何 cron 依赖。提醒规则前端 `SCHEDULE_TYPES = ["daily_at", "hourly", "weekly_at", "cron"]` 里后三个是 UI 承诺但后端拒绝的假选项，用户选了会拿到 `unsupported schedule` 错误。

## 2. 目标

1. 引入标准 5 段 cron 表达式（`分 时 日 月 周`），支持"每周一""每工作日""每 N 分钟""每月几号"等场景。
2. 项目自动化规则支持 cron 触发，保留原有 daily_at 体验。
3. 提醒规则支持 cron 触发，修复 UI 假选项 bug。
4. 提供**可视化配置**：常用预设按钮（一键回填）+ 实时中文解读（让用户确认配置语义）+ 时区选择。
5. 把 daily_at / cron 的"校验 + 到期判断 + dedupe key"逻辑收敛到公共包 `internal/schedule`，消除两处重复。
6. 保证高频 cron（如每 15 分钟）不会因为 dedupe 粒度过粗而被吞掉。
7. 保持 `CGO_ENABLED=0`、零 CGO、SQLite/PostgreSQL 双库兼容。
8. 不改变循环任务系列的调度语义。

## 3. 非目标

- 不给循环任务系列上 cron（语义错位，单独立项）。
- 不实现 cron 的"秒级"精度（标准 5 段，不引入 6/7 段扩展）。
- 不实现分布式 cron 锁（现有调度器已是单实例 + claim/lease 模式，无需新增）。
- 不引入 cron 库的 daemon/调度器（只用其 Parser + Next 计算，调度循环复用现有 ticker）。
- 不做"自然语言转 cron"（如"每天早上九点"自动转表达式）——可视化预设已覆盖大部分需求。
- 不支持 `@reboot`、`@yearly` 等 macro（标准 5 段表达式已够用）。

## 4. 核心决策

| 主题 | 决策 |
|---|---|
| cron 库 | `github.com/robfig/cron/v3`（纯 Go 零 CGO，事实标准），只用 `ParseStandard` + `Schedule.Next()` |
| cron 格式 | 标准 5 段 `分 时 日 月 周`，时区独立配置 |
| 表达范围 | 支持 `*`、`,`、`-`、`/`，标准 POSIX cron 语义 |
| 调度类型 | `daily_at`（保留）+ `cron`（新增），二选一 |
| 公共包 | 新增 `internal/schedule`，三处调用点（校验、到期判断、dedupe）共用 |
| 到期判断 | `schedule.Spec.LastFireAt(now)` 求 `<= now` 的最近触发点，各子系统自行决定窗口 |
| dedupe 粒度 | daily_at → 按天 `2006-01-02`；cron → 按分钟 `2006-01-02T15:04`（**关键**） |
| 时区 | 项目自动化已有 `Timezone`；提醒规则**新增 `Timezone` 字段**（缺省 Asia/Shanghai） |
| 窗口语义 | 项目自动化保留 1h 窗口（防漏触发）；提醒规则无窗口（靠 dedupe 去重）。各自外层保留，`schedule.Spec` 只算触发点 |
| 可视化 | 预设按钮 + 中文解读 + 时区选择；手写 cron 作为高级选项 |
| 循环任务 | 本期只修 MCP 描述 bug，定时文法增强单独立项 |

## 5. `internal/schedule` 包设计

### 5.1 类型与接口

```go
package schedule

import "time"

const (
    TypeDailyAt = "daily_at"
    TypeCron    = "cron"
)

// Spec 描述一个调度规格。
type Spec struct {
    Type     string // "daily_at" | "cron"
    Value    string // daily_at: "09:30"；cron: "0 9 * * 1-5"
    Timezone string // 缺省 Asia/Shanghai；空串按 Asia/Shanghai 处理
}

// Validate 校验 Type + Value 组合。
// daily_at: Value 必须是合法 HH:MM。
// cron: Value 必须是合法标准 5 段 cron 表达式。
func (s Spec) Validate() error

// LastFireAt 返回 <= now 的最近一次触发时刻。
// 若 now 本身就早于任何可能的触发点，返回 (零值, false, nil)。
// daily_at: 返回今天的计划时刻（若 now 已过该时刻）。
// cron:     用 cron.Next 反推最近触发点。
func (s Spec) LastFireAt(now time.Time) (fire time.Time, ok bool, err error)

// DedupeKey 返回本次触发点的去重标识。
// daily_at: 规则时区日期 "2006-01-02"（按天去重，与旧行为一致）。
// cron:     触发时刻 "2006-01-02T15:04"（按分钟去重，支持高频）。
func (s Spec) DedupeKey(now time.Time) (string, error)

// Describe 返回中文人类可读描述，用于 UI 展示。
// 覆盖常见模式：每工作日、每周X、每N分钟、每小时、每月几号等。
// 未匹配的非常规表达式回退为表达式本身。
func (s Spec) Describe() string
```

### 5.2 实现要点

- **daily_at 分支**：复用现有 `time.Parse("15:04", Value)` 逻辑，行为与 `automationScheduleDue` 完全一致（保证不回归）。
- **cron 分支**：`cron.ParseStandard(Value)` 解析得到 `cron.Schedule`，用 `schedule.Next(prev)` 逐步反推 `<= now` 的最近触发点。反推策略：从 `now` 往前按分钟回退，调 `Next(candidate)` 直到结果 `<= now`（上限回退 1440 分钟防死循环）。
- **时区处理**：`time.LoadLocation(s.Timezone)`，失败回退 `time.Local`（与现有 `automationScheduleDue` 一致）。
- **Describe 实现**：维护一张"模式 → 中文"映射表，按优先级匹配。例如：
  - `0 9 * * 1-5` → "每工作日 09:00"
  - `0 9 * * 1` → "每周一 09:00"
  - `*/15 * * * *` → "每 15 分钟"
  - `0 * * * *` → "每小时整点"
  - `0 9 1 * *` → "每月 1 日 09:00"
  - 其它 → 返回原始表达式

### 5.3 为什么不收敛到一处窗口

项目自动化和提醒规则的"到期窗口"语义不同：
- 项目自动化：`local.Sub(today) <= time.Hour`——计划时间后 1 小时内才算到期，防调度间隙漏触发。
- 提醒规则：`!current.Before(scheduled)`——一过点就到期，靠 dedupe 保证每天只发一次。

`schedule.Spec` 只负责"算触发点"，窗口逻辑由各子系统外层保留。这样既消除了重复，又不强行统一两种语义。

## 6. 三套子系统的语义差异（为何循环任务不上 cron）

```text
子系统          心智模型          核心不变量                cron 契合度
─────────────────────────────────────────────────────────────────────
项目自动化      时刻触发          "现在过了今天 09:30 吗"   ★★★ 天然契合
提醒规则        时刻触发          "现在过了今天 08:50 吗"   ★★★ 天然契合
循环任务系列    日历槽位补齐      见下                       ✗ 语义错位
```

**循环任务系列的四个不变量（与 cron 冲突）：**

1. **date-only 槽位**：`Slot.RecurrenceAt` 存当天 23:59:59，执行期折算为当天 00:00。cron 是精确到分钟的触发时刻，套上去要么丢弃时分（浪费表达力），要么破坏 date-only invariant。
2. **backlog 补齐**：调度器每 60s 扫一次，把"已进入执行期但未物化"的槽位补上（停机错过的会自动补）。cron 的心智是"到点触发一次，错过即丢"。
3. **月末滚动**：`time.AddDate(0,1,0)` 让 1/31 → 3/3。cron 的月底（`L`）语义和 Go AddDate 不一致。
4. **anchor + 版本历史**：`RuleVersion.EffectiveFrom` 作为展开 anchor，规则切换后新段重新展开。cron 是无 anchor 的绝对日历匹配。

→ 结论：循环任务走"扩展 canonical 文法"（如 `weekly:MO,WE`、`monthly:15`）更契合，单独立项。本期只修它的 MCP 描述 bug。

## 7. UI 原型

### 7.1 原型一：项目自动化 — 触发配置区（定时分支）

现状只有一个 `HH:MM` 文本框，改为「定时方式切换 + cron 输入 + 常用预设 + 实时中文解读」。

```text
┌─ 编辑自动化规则 ──────────────────────────────────────┐
│                                                       │
│  名称      [ 每周项目回顾                            ]│
│  触发类型  [ 定时触发                          ▾ ]    │
│            ├─ 定时触发                              │
│            └─ 事件触发                              │
│                                                       │
│  ┌─ 定时方式 ──────────────────────────────────────┐ │
│  │                                                  │ │
│  │   类型   ( ) 每天定时    (•) 自定义 cron        │ │
│  │                                                  │ │
│  │   ┌── 选中「自定义 cron」时展开 ──────────────┐ │ │
│  │   │                                            │ │ │
│  │   │   表达式  [ 0 9 * * 1              ] ⓘ  ✓ │ │ │
│  │   │                                            │ │ │
│  │   │   常用预设：                               │ │ │
│  │   │   ┌──────────┐ ┌──────────┐ ┌──────────┐  │ │ │
│  │   │   │ 每天 9点 │ │每工作日9点│ │ 每周一9点│• │ │ │
│  │   │   └──────────┘ └──────────┘ └──────────┘  │ │ │
│  │   │   ┌──────────┐ ┌──────────┐ ┌──────────┐  │ │ │
│  │   │   │  每小时  │ │ 每15分钟 │ │  每月1号 │  │ │ │
│  │   │   └──────────┘ └──────────┘ └──────────┘  │ │ │
│  │   │                                            │ │ │
│  │   │   时区    [ Asia/Shanghai            ▾ ]  │ │ │
│  │   │                                            │ │ │
│  │   │   ┌──────────────────────────────────────┐│ │ │
│  │   │   │ 💡 每周一 09:00 触发                  ││ │ │
│  │   │   │    下次：2026-07-27 (周一) 09:00      ││ │ │
│  │   │   └──────────────────────────────────────┘│ │ │
│  │   └────────────────────────────────────────────┘ │ │
│  │                                                  │ │
│  │   ↑ 选中「每天定时」时折叠为：                   │ │
│  │   ┌──────────────────────────────────────────┐  │ │
│  │   │ 时间  [ 09:30 ]      时区 [Asia/Shanghai▾]│  │ │
│  │   └──────────────────────────────────────────┘  │ │
│  └──────────────────────────────────────────────────┘ │
│                                                       │
│                          [ 取消 ]   [ 保存 ]          │
└───────────────────────────────────────────────────────┘
```

**交互要点：**

- 单选切换「每天定时 / 自定义 cron」。daily_at 保持原体验（单时间框 + 时区）。
- cron 输入框右侧实时校验图标（`✓` 有效 / `✗` 无效）。
- 6 个预设按钮覆盖 90% 场景，点击回填表达式并更新解读。
- 「💡 解读」实时显示 `schedule.Spec.Describe()` 的中文 + 下次触发时间，让用户确认配置无误。

### 7.2 原型二：提醒规则 — 定时配置区（修假 cron bug）

现状 `SCHEDULE_TYPES = ["daily_at", "hourly", "weekly_at", "cron"]`，后两项是假的。改为 `["daily_at", "cron"]`（hourly/weekly 用 cron 表达式覆盖），并补上时区字段。

```text
┌─ 新建提醒规则 ───────────────────────────────────────┐
│                                                       │
│  名称        [ 未完成任务每日提醒                    ]│
│  触发类型    [ 定时过滤                        ▾ ]    │
│                                                       │
│  ┌─ 定时方式 ──────────────────────────────────────┐ │
│  │                                                  │ │
│  │   类型    [ cron 表达式                    ▾ ]   │ │  ← 移除 hourly/weekly_at
│  │            ├─ 每天定时 (daily_at)               │ │     用 cron 表达式覆盖
│  │            └─ cron 表达式            ← 选中      │ │
│  │                                                  │ │
│  │   表达式   [ 50 8 * * 1-5              ] ✓      │ │
│  │                                                  │ │
│  │   常用预设：                                     │ │
│  │   [ 每天 8:50 ] [ 每工作日 8:50 ] [ 每周一 8:50 ] │ │
│  │   [ 每小时    ] [ 每 30 分钟    ]                │ │
│  │                                                  │ │
│  │   时区     [ Asia/Shanghai                ▾ ]    │ │  ← 新增字段
│  │                                                  │ │
│  │   ┌────────────────────────────────────────────┐ │ │
│  │   │ 💡 每工作日 08:50 触发                       │ │ │
│  │   └────────────────────────────────────────────┘ │ │
│  └──────────────────────────────────────────────────┘ │
│                                                       │
│  过滤条件    [ status:pending and due:today         ]│
│  ...                                                  │
│                          [ 取消 ]   [ 保存 ]          │
└───────────────────────────────────────────────────────┘
```

**与原型一的差异：** 提醒规则只有一个"类型下拉"（不展开单选区），cron 输入区样式一致；强制要求时区（模型新增 `Timezone` 字段，缺省 Asia/Shanghai）。

### 7.3 原型三：cron 预设按钮的三种交互状态

同一组预设按钮在不同输入状态下的反馈，保证用户填错时有清晰提示：

```text
状态 A — 点击预设「每工作日9点」
────────────────────────────────────────
  表达式  [ 0 9 * * 1-5         ] ✓ 有效
  常用预设： [每天9点] [每工作日9点●] [每周一] [每小时]
                              ↑ 选中高亮
  💡 每工作日 09:00 触发
     下次：2026-07-23 (周四) 09:00

状态 B — 手动输入非法表达式
────────────────────────────────────────
  表达式  [ 0 9 * *             ] ✗ 无效
  常用预设： [每天9点] [每工作日9点] [每周一] [每小时]
  💡 ⚠ 表达式不合法：缺少「星期」字段

状态 C — 手动输入合法但非常规
────────────────────────────────────────
  表达式  [ */20 8-18 * * 1-5   ] ✓ 有效
  常用预设： [每天9点] [每工作日9点] [每周一] [每小时]
  💡 工作日 08:00–18:00 之间每 20 分钟触发
     下次：2026-07-23 (周四) 09:00
```

### 7.4 原型四：调度判定与去重数据流（后端，解释 dedupe 细化）

说明 cron 高频规则为何要把 dedupe key 细化到分钟级：

```text
  规则: cron "*/15 * * * *"   每 15 分钟

  scheduler.RunOnce (每 60s 轮询)
        │
        ▼
  ┌──────────────────────────────────────────┐
  │ schedule.Spec.LastFireAt(now)            │
  │   now = 09:23                            │
  │   cron.Next(09:08) = 09:23               │
  │   → lastFire = 09:23,  命中              │
  └──────────────────────────────────────────┘
        │
        ▼
  ┌──────────────────────────────────────────┐
  │ dedupeKey = Spec.DedupeKey(now)          │
  │   daily_at  → "2026-07-23"      按天      │
  │   cron      → "2026-07-23T09:23" 按分钟  │ ← 细化
  └──────────────────────────────────────────┘
        │
        ▼
  ExistsByDedupeKey ?  ── 是 ──→ skip (本分钟已触发)
        │ 否
        ▼
  Enqueue delivery (dedupe_key 唯一索引兜底)

  对比：若 cron 仍用按天 key "2026-07-23"
        → 09:00 触发后，09:15/09:30... 全部被吞 ❌
```

## 8. 数据模型

### 8.1 项目自动化（无 schema 变更）

`project_automation_rules.trigger_config_json` 是 JSON 文本列，`ProjectAutomationTriggerConfig` 的 `ScheduleType` 含义从 `"daily_at"` 扩展为 `"daily_at" | "cron"`，无需建表/迁移：

```json
// daily_at（旧行为不变）
{"schedule_type": "daily_at", "schedule_value": "09:30", "timezone": "Asia/Shanghai"}

// cron（新增）
{"schedule_type": "cron", "schedule_value": "0 9 * * 1-5", "timezone": "Asia/Shanghai"}
```

### 8.2 提醒规则（新增 Timezone 字段）

`reminder_rules` 表新增一列：

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `timezone` | string | `Asia/Shanghai` | cron 触发时区 |

通过 GORM AutoMigrate 自动加列，老数据取默认值，无破坏性。`ScheduleType` 取值从 `daily_at` 扩展为 `daily_at | cron`；`hourly`/`weekly_at`/旧 `cron`（若有脏数据）在 normalize 时拒绝并提示用户重新配置。

### 8.3 循环任务系列（无 schema 变更）

本期只修 MCP 工具描述，不涉及数据模型。

## 9. 后端改造点（精确定位）

### 9.1 项目自动化

| 文件 | 行号 | 改动 |
|---|---|---|
| `internal/app/project_automation.go` | L22-27 | `ProjectAutomationTriggerConfig.ScheduleType` 含义扩展（字段类型不变） |
| `internal/app/project_automation.go` | L338-345 | 校验改用 `schedule.Spec{...}.Validate()`，支持 daily_at + cron |
| `internal/app/project_automation.go` | L378-384 | `validHHMM` 删除（被 schedule 包取代） |
| `internal/app/project_automation_scheduler.go` | L98 | `cfg.ScheduleType != "daily_at"` 改为调 `schedule.Spec` 判断 |
| `internal/app/project_automation_scheduler.go` | L149-163 | `automationScheduleDue` 替换为 `schedule.Spec.LastFireAt` + 保留 1h 窗口 |
| `internal/app/project_automation_scheduler.go` | L101 | dedupe key 末段改用 `schedule.Spec.DedupeKey(now)` |
| `internal/app/project_automation_scheduler.go` | L166-172 | `automationLocalDate` 被 `DedupeKey` 取代（daily_at 行为不变） |
| `internal/httpapi/project_automations.go` | - | 纯透传，无需改 DTO |

### 9.2 提醒规则

| 文件 | 行号 | 改动 |
|---|---|---|
| `internal/app/notification.go` | L99-149 | `ReminderRuleAddInput`/`View` 新增 `Timezone` 字段 |
| `internal/app/notification.go` | L1053 | `if scheduleType != "daily_at"` 改为支持 cron |
| `internal/app/notification_scheduler.go` | L221-241 | `scheduledRuleDueForRun` 改用 `schedule.Spec.LastFireAt`（无窗口） |
| `internal/app/notification_scheduler.go` | L511-517 | `scheduleDateKey` 加 cron 分支（分钟级 dedupe） |
| `internal/storage/models.go` | L377-401 | `ReminderRule` 新增 `Timezone` 字段 |
| `internal/httpapi/notifications.go` | - | request/response DTO 新增 `timezone` |
| `internal/cli/notification.go` / `internal/mcpserver/tools_notification.go` / `internal/remote/notification.go` | - | 各入口 input/view 同步加 timezone |

### 9.3 循环任务系列（仅修 bug）

| 文件 | 行号 | 改动 |
|---|---|---|
| `internal/mcpserver/tools_task_series.go` | L23 | jsonschema 描述 `FREQ=WEEKLY;BYDAY=MO`（后端不支持）改为 `"daily, weekly, monthly, <N>days, <N>weeks, <N>months (e.g. 2weeks, 3months)"`，与 CLI 对齐 |

## 10. 前端改造点

### 10.1 公共组件（建议抽取）

新增 `web/src/features/workspace/schedule/` 放 cron 输入器（预设按钮 + 解读 + 时区），项目自动化和提醒规则复用，避免两处重复。

### 10.2 项目自动化 `web/src/features/workspace/project-workbench/automations/`

| 文件 | 行号 | 改动 |
|---|---|---|
| `project-automations-api.ts` | L20 | `schedule_type?: "daily_at"` → `"daily_at" \| "cron"` |
| `automation-rule-dialog.tsx` | L166-182 | 实现原型一（单选切换 + cron 输入 + 预设 + 解读） |
| `automation-rule-form.tsx` | - | 新增「每周项目回顾」模板（cron `0 9 * * 1`） |

### 10.3 提醒规则 `web/src/features/workspace/outbound/rules/reminder-rule-list.tsx`

| 文件 | 行号 | 改动 |
|---|---|---|
| - | L51 | `SCHEDULE_TYPES` 移除 hourly/weekly_at，改为 `["daily_at", "cron"]` |
| - | L322-327 | 实现原型二（type 下拉 + cron 输入 + 时区 + 解读） |

## 11. cron 表达式范围与校验

### 11.1 支持的表达式

标准 5 段 cron，字段顺序 `分 时 日 月 周`，值范围：

| 字段 | 范围 | 特殊字符 |
|---|---|---|
| 分 | 0-59 | `*` `,` `-` `/` |
| 时 | 0-23 | 同上 |
| 日 | 1-31 | 同上 |
| 月 | 1-12 | 同上 |
| 周 | 0-6（0=周日） | 同上 |

支持别名（由 robfig/cron 内置）：月份 `JAN-DEC`、星期 `SUN-SAT`。

### 11.2 预设映射表

```text
预设                cron 表达式         Describe 输出
─────────────────────────────────────────────────────
每天 9 点           0 9 * * *           "每天 09:00 触发"
每工作日 9 点       0 9 * * 1-5         "每工作日 09:00 触发"
每周一 9 点         0 9 * * 1           "每周一 09:00 触发"
每小时整点          0 * * * *           "每小时整点触发"
每 15 分钟          */15 * * * *        "每 15 分钟触发"
每月 1 号 9 点      0 9 1 * *           "每月 1 日 09:00 触发"
```

### 11.3 校验时机

- **前端**：输入时轻量校验（5 段、字符合法），失焦/提交时调后端或本地 robfig 等价校验。
- **后端**：`normalizeProjectAutomationAddInput` / `normalizeReminderScheduleFilter` 调 `schedule.Spec.Validate()`，非法表达式返回 `automation_rule_invalid` / `reminder_rule_invalid`。

## 12. 测试要求

### 12.1 后端

- `internal/schedule`：daily_at 各边界、cron 校验（合法/非法用例）、`LastFireAt` 边界（窗口内/窗口外/跨日）、`DedupeKey` 细粒度、`Describe` 中文映射覆盖 + 回退。
- 项目自动化：新增 `TestProjectAutomationSchedulerEnqueuesCronRuleOnce`（cron 规则命中时入队 1 次，再跑去重返回 0）；保留 daily_at 测试不回归；高频 cron（`*/15`）不会被按天吞掉。
- 提醒规则：cron 规则创建/校验/到期判断；时区字段读写；旧 `hourly`/`weekly_at` 值被明确拒绝。
- `CGO_ENABLED=0 go test ./...` 和 `CGO_ENABLED=0 go build ./cmd/xuanchu`。

### 12.2 前端

- 项目自动化：cron 输入、预设回填、解读展示、保存后回显。
- 提醒规则：SCHEDULE_TYPES 不再含 hourly/weekly_at；cron 输入 + 时区 + 解读。
- 公共 cron 输入器组件单测。
- `cd web && npm run build && npm run lint`。

### 12.3 e2e

- 通过 HTTP API 创建 cron 规则，校验调度器在命中时刻入队、去重生效。
- 提醒规则 cron 全链路（创建 → 校验 → 到期 → 去重）。

## 13. 错误处理

| 错误 | 处理 |
|---|---|
| cron 表达式非法 | 后端 `automation_rule_invalid` / `reminder_rule_invalid`；前端输入框标红 + `Describe` 显示 `⚠ 表达式不合法` |
| 时区非法（如 `Foo/Bar`） | 回退到 `Asia/Shanghai` 并在解读中注明（与现有 `automationScheduleDue` 的 `time.Local` 回退策略一致） |
| 高频 cron 资源担忧 | cron 规则最小粒度为分钟（调度器 60s 轮询），无额外限流；dedupe 保证每分钟最多触发一次 |
| 老脏数据（reminder 存了 `hourly`） | normalize 时拒绝，提示用户改为 cron 表达式（`0 * * * *`） |

## 14. 文档同步

实现完成后同步：

- `README.md`：自动化、提醒规则新增 cron 定时能力说明，给出配置示例。
- `ROADMAP.md`：记录本能力进入对应 milestone。
- 本 spec 对应的 implementation plan。

## 15. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 高频 cron dedupe 粒度 | 已设计分钟级 key（原型四），避免每分钟触发被按天吞掉 |
| 提醒规则时区字段迁移 | AutoMigrate 加列，缺省 Asia/Shanghai，老数据无影响；需同步所有入口的 input/view |
| 窗口语义差异 | `schedule.Spec` 只算触发点，窗口逻辑各自外层保留，不强行统一 |
| robfig/cron 依赖 | 纯 Go 零 CGO，`CGO_ENABLED=0` 通过；只用 Parser + Next，不引入 daemon，避免与现有 ticker 冲突 |
| 循环任务用户期待 | spec 明确记录"不上 cron"的决策与原因，避免后续代理误改 |
