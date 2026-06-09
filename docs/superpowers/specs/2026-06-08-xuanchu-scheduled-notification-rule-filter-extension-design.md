# Xuanchu 定时通知规则过滤扩展设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 在现有定时通知能力上，补齐“按每天固定时刻执行”和“按任务过滤条件组合执行”的规则表达能力，让 workspace 内可以稳定配置如下两类通知：

1. 每天早上 8:50，给“未开始、未结束、24 小时内即将到期”的任务发送预警。
2. 每天早上 9:00，给“未开始、未结束、已逾期”的任务发送逾期通知，并且通知内容里能体现这是第几次逾期通知。

**范围策略：** 本规格只设计 Xuanchu 侧的 reminder rule 表达升级、schedule 语义、任务过滤条件、重复计数和模板变量扩展。它建立在现有 M16 定时通知、query AST、scheduler、notification sink 和 delivery outbox 之上，不重新发明 sink/outbox/dispatcher。

**需求来源：**

- 当前 M16 已实现 `notification sink`、`reminder rule`、`notification delivery`、scheduler、dispatcher、动态 endpoint、`http_template` sink。
- 当前 query 层已有 `status`、`due`、`start`、`end`、`project`、`tag`、`assignee` 等 AST 和 SQL 编译能力。
- 当前 reminder rule 只有 `due_before` / `overdue` 这类时间触发语义，无法直接表达“每天 8:50 / 9:00”。
- 当前 scheduler 只做周期轮询，没有规则级的墙上时间调度。

---

## 1. 当前基础

Xuanchu 现在已经具备：

- workspace / project / task 的隔离。
- 任务字段 `status`、`due`、`start`、`end`、`wait`、`scheduled`、`until`。
- Taskwarrior 风格 query AST 和 SQL 编译。
- 定时通知基础设施：sink、rule、delivery、scheduler、dispatcher。
- `http_template` sink 的数据库模板保存与 delivery 冻结。
- delivery 重试、dead-letter、手动 replay。
- MCP / HTTP / CLI 对 reminder rule 和 notification sink 的管理入口。

当前缺口：

- 不能表达固定的每天 8:50 / 9:00 调度时刻。
- 不能把 reminder rule 直接写成“schedule + task filter”的组合。
- `overdue` 目前更像一种固定 trigger，而不是可组合条件。
- 通知模板里还没有“这是第几次提醒/第几次逾期通知”的稳定变量。

---

## 2. 设计目标

首版完成后，应支持这些规则：

```bash
# 每天 8:50 发送 24 小时内即将到期的预警
xuanchu reminder rule add due-soon-24h \
  --schedule daily@08:50 \
  --filter 'end.isnull and start.isnull and due.after:now and due.before:now+24h' \
  --audience assignees \
  --sink openclaw

# 每天 9:00 发送逾期通知，并在内容里能显示第几次逾期通知
xuanchu reminder rule add overdue-daily \
  --schedule daily@09:00 \
  --filter 'end.isnull and (start.isnull or start.notnull) and due.before:now' \
  --repeat every:24h \
  --audience assignees \
  --sink openclaw
```

核心目标：

- 支持“每天固定时刻”而不是只靠轮询间隔。
- 支持把任务筛选条件显式写成 filter，而不是继续堆 trigger 枚举。
- 规则表达能覆盖“未开始 / 进行中 / 未结束 / 已到期 / 即将到期”这类场景。
- 通知模板可引用“第 N 次提醒”或“第 N 次逾期提醒”。
- 保持 workspace 隔离、幂等、审计、delivery 冻结、重试与 replay 语义不变。

---

## 3. 非目标

本规格不做：

- 不做复杂工作流引擎、分支、审批。
- 不做自动修改任务状态。
- 不做跨 workspace 汇总提醒。
- 不把 reminder rule 混进 Hook definition。
- 不内置第三方 adapter。
- 不做任意脚本执行。
- 不要求用户必须写完整 query AST；可以先用受控的 filter 子集或结构化参数。

---

## 4. 核心产品决策

### 4.1 reminder rule 需要分成 schedule 和 filter

`overdue` 不是唯一条件，它只是一个预置的任务过滤语义。真正的规则应由两部分组成：

- `schedule`：什么时候执行，例如 `daily@08:50`、`daily@09:00`、后续可扩展 `cron(...)`
- `filter`：哪些任务命中，例如 `end.isnull and due.before:now`

这样可以直接表达用户场景，也能继续复用 query AST。

### 4.2 `status` 在提醒规则里应是组合条件，而非单值

在通知语义里，任务是否“该提醒”通常不是单个状态字段，而是多个字段组合。某些 workspace 甚至会把“未开始”和“进行中但未结束”视为同一提醒前提，所以 filter 必须允许在 `status` 分支上做 OR 组合：

- 未开始：`start is null`
- 进行中：`start is not null`
- 未结束：`end is null`
- 已结束：`end is not null`
- 未来 24 小时内到期：`due` 落在 `[now, now+24h)`
- 已逾期：`due < now`

因此 reminder rule 的 filter 必须支持 OR/AND 组合，而不是只靠 `overdue` 一种 trigger。

### 4.3 每日固定时刻应由 scheduler 记录“今日窗口”

`daily@08:50` / `daily@09:00` 不是简单地每 60 秒轮询一下就算完，而是要在 scheduler 内确定：

- 当前规则对应的执行日
- 当前执行窗是否已经跑过
- 同一 rule + task + recipient 在同一日是否已经发过

否则时区切换、服务重启、延迟运行都会让重复计数和幂等变得不稳定。

### 4.4 第几次通知应该是规则级的稳定派生值

“第几次逾期通知”不应靠前端/手工填数，而应由 delivery 历史或 dedupe 窗口计算出稳定的 `notification_index`。

建议将其作为 delivery payload 和模板变量的一部分，例如：

- `reminder.sequence`
- `reminder.daily_sequence`
- `reminder.overdue_sequence`

---

## 5. 规则模型

建议把 reminder rule 扩展为以下字段组：

- `schedule_type`
  - `daily_at`
  - 未来可扩 `cron`
- `schedule_value`
  - `08:50` 这类本地时间
- `filter_source`
  - 受控的 task filter 表达式
- `repeat_policy`
  - `once`
  - `every:<duration>`
- `audience_type`
  - 继续沿用现有受众模型
- `sink_ref`
  - 继续沿用现有 sink 引用

现有 `trigger_type/offset/after` 可以保留为兼容层，但新规则应优先走 `schedule + filter`。

### 5.1 filter 语义

filter 应至少支持：

- `and` / `or` / `not`
- 括号
- `status`
- `start`
- `end`
- `due`
- `project`
- `assignee`
- `tag`

建议的规则过滤语义：

- `end.isnull` 表示未结束
- `start.isnull` 表示未开始
- `start.notnull` 表示已开始
- `due.before:now` 表示已逾期
- `due.after:now` 表示未到期
- `due.before:now+24h` 表示 24 小时内到期；这里的相对时间值统一按 `now +/- duration` 理解，其中 `duration` 直接使用 Go `time.ParseDuration` 语法，例如 `24h`、`90m`、`2h30m`。

如果现有 query 语法不直接支持 `now+24h` / `now-2h`，实现计划必须扩 query/date 语法，支持 `now +/- duration` 相对时间表达式；不能在 reminder rule 层再引入一套独立的时间窗口字段作为替代方案。

duration 语法要求：

- 在 query/filter 中的规范写法不包含空格，例如 `due.before:now+24h`、`due.after:now-2h`；不要写成 `due.before:now + 24h`。
- duration 由 Go `time.ParseDuration` 解析；支持 `ns`、`us`、`µs`、`μs`、`ms`、`s`、`m`、`h`。
- 支持复合表达式，例如 `2h30m`、`1h15m30s`。
- 不支持 `d`、`w`、`min`、`hour` 这类 Go 原生不支持的单位；`1d` 应写成 `24h`，`90min` 应写成 `90m`。
- 不接受裸数字；正负方向只能由 `now+` 或 `now-` 表示，filter 中不要写负 duration。

---

## 6. 调度语义

### 6.1 daily_at

`daily_at=08:50` 表示：

- 每个 workspace 的 reminder scheduler 在本地时区每天 08:50 计算一次该规则
- 规则命中后，为每个 recipient 生成 delivery
- 同一日同一规则同一 recipient 只生成一次，除非显式 repeat policy 允许重复

### 6.2 overdue 序列

逾期通知默认可通过日窗口重复发送。

对于 `daily@09:00` 的逾期通知，规则应支持：

- 第 1 次：任务首次进入逾期范围后，第一次 09:00 扫描命中
- 第 2 次：次日 09:00 仍未完成，则再发一次
- 以此类推

这个“第几次”应写入 delivery，并暴露给模板。

### 6.3 幂等 key

除现有 rule/task/recipient 窗口外，还需要把 schedule day 纳入幂等语义，避免同一日重复发送。

建议 dedupe key 包含：

- workspace_id
- rule_id
- task_uuid
- recipient_user_id
- schedule_date 或 schedule_window
- sequence

---

## 7. 模板变量扩展

现有模板已经支持：

- workspace
- project
- rule
- task
- recipient
- secret

本次需要补充：

- `reminder.sequence`
- `reminder.overdue_sequence`
- `reminder.window_start`
- `reminder.window_end`

其中：

- `sequence` 是该 rule + task + recipient 的累计通知次数
- `overdue_sequence` 是逾期通知专用计数
- `window_start/window_end` 用于调试和审计

---

## 8. 对现有能力的复用

本规格应尽量复用已有能力：

- 复用 query AST 和 SQL 编译，不再另起一套过滤语言。
- 复用 notification sink / delivery / dispatcher。
- 复用 HTTP request template 的数据库存储和 delivery 冻结。
- 复用 workspace / project / user / assignee / external id 解析。

不复用的部分：

- 不继续扩 `overdue` 为唯一 trigger。
- 不把 schedule 强行塞进 Hook。

---

## 9. 错误与边界

需要明确的错误：

- schedule 格式非法
- filter 语法非法
- filter 引用不支持的字段
- 模板变量非法
- 时间窗口解析失败
- 同一 rule 的重复次数计算失败

失败策略：

- 规则配置错误时，创建/修改 rule 直接失败。
- 运行时单个 task/recipient 解析失败时，不影响其他 recipient。
- 运行时失败 delivery 进入 dead-letter 或 skipped，并保留原因。

---

## 10. 迁移与兼容

现有 `due_before` / `overdue` 规则需要继续可读可用。

兼容策略：

- 老规则继续按现有逻辑跑。
- 新规则优先使用 `schedule + filter`。
- CLI / HTTP / MCP 可以逐步补充新字段，同时保留旧字段。

---

## 11. 验收标准

应至少满足：

1. 可以创建“每天 8:50 预警”的规则。
2. 可以创建“每天 9:00 逾期通知”的规则。
3. 规则能用 AND/OR 组合表达 `start is null`、`end is null`、`due before/after` 这类条件。
4. delivery 中能看到第几次提醒。
5. 既有 sink / delivery / replay / dead-letter 能力不退化。
6. 现有 `due_before` / `overdue` 规则仍然兼容。

---

## 12. 相关接口草案

```bash
xuanchu reminder rule add overdue-daily \
  --schedule daily@09:00 \
  --filter 'end.isnull and due.before:now' \
  --repeat every:24h \
  --audience assignees \
  --sink openclaw
```

```bash
xuanchu reminder rule add due-soon-24h \
  --schedule daily@08:50 \
  --filter 'end.isnull and start.isnull and due.after:now and due.before:now+24h' \
  --audience assignees \
  --sink openclaw
```
