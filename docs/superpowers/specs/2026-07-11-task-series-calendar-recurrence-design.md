# 璇础循环任务系列、范围投影与按需物化设计规格

**日期：** 2026-07-11

**状态：** 待实现

**修订：** 2026-07-12。在主流日历 recurrence 模型调研及 Web Console、HTTP、MCP 场景复盘后，采用“独立 `task_series` 聚合 + 有界范围投影 + 例外覆盖 + 执行期/写操作物化”的最终模型。Web 信息架构不照搬后端资源边界：普通任务与循环 occurrence 统一进入“任务”执行视图，Series CRUD 收进任务页内可深链的管理面板，不新增项目一级 Tab。璇础从本版本起不再承诺 Taskwarrior JSON、recurring parent 存储形态或循环命令兼容，只参考其规则表达和任务管理思路。

**范围：** 任务循环系列的领域语义、日历调度、CLI/HTTP/Remote/MCP 契约与 Web Console 完整 CRUD 体验

**承接：**

- `2026-05-28-taskg-m2-design.md`
- `2026-06-17-web-console-project-task-browsing-design.md`
- `2026-07-07-web-console-task-detail-redesign-design.md`

> 给 agentic workers 的要求：编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

## 1. 背景与问题

璇础当前代码已有一套待替换的基础 recurring 模型：一条 `status=recurring` 的隐藏任务充当模板，实例任务通过 `parent=<模板 UUID>` 与它关联。当前实现可以在创建时生成第一个实例，并在实例完成后生成下一条。这是现状描述，不是新设计需要兼容的持久化契约。

这套模型尚未形成一致的产品能力：

1. 当前生成是**完成驱动**。当天实例未完成时，第二天不会生成新的 daily 实例，不符合“每天都是一次独立执行”的项目管理语义。
2. 普通任务后补 `recur` 只会写字段，不会转换为循环模板，也不会生成实例，形成“看起来循环、实际不循环”的无效状态。
3. HTTP 与 Remote 创建请求支持 `recur`，MCP `task_add` 不支持；MCP `task_modify` 甚至只能清除 `recur`、不能设置，入口能力不一致。
4. Web Console 创建弹窗没有循环任务入口；任务详情却允许直接修改 `recur`，会触发上述无效状态。
5. Web Console 的循环选项包含 `biweekly`、`quarterly`、`annual`，但 Go recurrence parser 只接受 `2weeks`、`3months`、`12months`，前后端契约不一致。
6. 项目任务查询在带 project filter 时可能同时返回隐藏模板和实例；模板还会进入项目任务统计，导致列表重复、编号空洞和进度失真。
7. `parent` 同时表达“手工父子任务”和“循环模板—实例”，现有 Web 详情会把循环模板误展示为普通父任务。
8. 完成生成的下一实例没有独立 `task.created` 事实，自动化和 Hook 无法稳定观察真实的新任务。
9. 服务端已经有 reminder、project automation 等后台 scheduler 基础设施，但循环任务没有复用这条运行时能力。
10. 旧设计只能查询已经落库的实例，无法像日历一样查看任意未来日期，也把 scheduler 变成了任务是否可见的单点依赖。
11. Web、HTTP 与 MCP 尚无统一的“虚拟 occurrence 稳定引用、读取不落库、写前物化”契约。
12. 当前“我的任务”的 `all/today/overdue/noDue` 预设都会覆盖状态为 pending，状态下拉中的 completed 实际无法形成稳定的“我的已完成”视图；`today` 只设置 due 上界，也会混入逾期任务。
13. `status=recurring`、`parent` 双重语义和 Taskwarrior JSON 兼容承诺会把系列资源永久耦合到 Task 领域；在没有生产历史数据的当前阶段继续保留，长期收益为负。

本规格不是给现有 UI 增加一个下拉框，而是把循环任务收敛成一个完整、可解释、跨入口一致的“循环系列”能力。

### 1.1 当前代码锚点

| 层 | 当前锚点 | 已确认行为 |
|---|---|---|
| Domain | `internal/task/model.go` | `status=recurring` 只校验 `recur` 和 `due`；普通 task 带 `recur` 仍能通过 |
| Recurrence | `internal/recurrence/recurrence.go` | 仅支持 daily/weekly/monthly 与 N days/weeks/months；`Next` 以传入时间为基准 |
| App | `internal/app/service.go` | `AddInput.Recur` 进入 `createRecurringParent`；`ModifyInput.Recur` 只写字段；`doneLocked` 完成 child 后调用 `createNextRecurringChild` |
| Storage | `internal/storage/task_repo.go` | `CreateRecurringChild` 只按 open child 的 parent+due 去重；completed/deleted 槽位不能提供永久幂等 |
| HTTP | `internal/httpapi/tasks.go` | add/modify request 都暴露 `recur`，但没有 series 资源和 derived view |
| Remote | `internal/remote/task.go` | Add/Modify DTO 镜像 HTTP 的 `recur` / `clear_recur` |
| MCP | `internal/mcpserver/tools_task.go` | `TaskAddInput` / `TaskModifyInput` 没有可设置 `recur`；clear 列表却接受 `recur` |
| Web create | `web/src/features/workspace/project-workbench/tasks/task-create-dialog.tsx` | 有 due/wait/scheduled/until，没有 recurrence 控件，也不会提交 `recur` |
| Web detail | `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx` | 对任何可写 task 都可 inline 修改 `recur`，会产生无效普通任务状态 |
| Web options | `web/src/features/workspace/shared/task-labels.ts` | UI 值 biweekly/quarterly/annual 与 Go parser 不一致 |
| Web list | `web/src/features/workspace/project-workbench/api/project-api.ts`、`internal/app/service.go` | project query 会使默认 pending fallback 失效，series template 可能进入任务表 |
| Web global nav | `web/src/components/AppShell.tsx` | 个人组已有概览/我的任务/项目；项目子路由统一高亮“项目”，无需新增全局 series 入口 |
| Web project nav | `project/project-tabs.tsx`、`project/project-layout.tsx` | 现有概览/任务/活动/自动化 Tabs，Tabs 行右侧已有 `setTabActions` 与 context rail toggle |
| Web routes | `web/src/routes/router.tsx` | 已有 project tasks/detail 与 my-tasks；Series 管理使用 tasks 下的可深链面板路由，不新增 ProjectTabs 项 |
| Scheduler | `internal/app/project_automation_scheduler.go`、`internal/cli/server.go` | 已有可复用的 RunOnce/Run、后台 ServiceFactory、60 秒 tick 和 server 生命周期 |

### 1.2 产品兼容边界调整

从本规格开始：

- 不以 Taskwarrior JSON 作为 import/export、HTTP、MCP 或数据库契约。
- 不保留 `status=recurring` hidden task、`parent=<series>`、`mask/imask` 等循环存储形态。
- 不保证 `xuanchu add ... recur:*`、`modify recur:*` 等循环命令兼容；循环能力使用专用 series 命令/API/tool。
- 可以继续借鉴 Taskwarrior 的命令命名、urgency、查询和 recurrence 思路，但每项能力以璇础自己的领域边界和公开契约为准。
- 现有开发数据库和测试 fixture 不构成迁移包袱；实现可以重建 recurring 相关 schema 和测试数据。

### 1.3 主流日历模型调研

本规格不是凭产品类比引入“虚拟任务”，而是参考了公开标准和主流日历 API 的共同逻辑模型：

| 来源 | 已确认的公开语义 | 对璇础的启示 |
|---|---|---|
| [RFC 5545](https://www.rfc-editor.org/rfc/rfc5545.html) | recurrence set 由 `DTSTART + RRULE + RDATE - EXDATE` 得出；`RECURRENCE-ID` 以原始槽位稳定标识某一次，即使该次已改期 | 系列规则生成槽位；单次改期不能改变槽位身份 |
| [CalDAV RFC 4791 §3.2](https://www.rfc-editor.org/rfc/rfc4791.html#section-3.2) | 循环组件和例外建模为一个资源，避免预存无限实例；客户端可按时间范围取展开结果 | 不预生成无限未来；有界查询时展开 |
| [CalDAV `expand`](https://www.rfc-editor.org/rfc/rfc4791.html#section-9.6.5) | 服务端把循环集合展开为指定时间窗内的单次实例 | 展开必须由 App 层统一完成，不能由 Web/MCP 各算一遍 |
| [Google Calendar recurring events](https://developers.google.com/workspace/calendar/api/guides/recurringevents) | 默认 list 返回单次事件、系列主记录和 exception；`singleEvents=true` 才展开普通实例；实例用 `recurringEventId + originalStartTime` 标识 | 普通列表和展开列表是两种查询模式；已修改实例覆盖计算结果 |
| [Microsoft Graph event/calendarView](https://learn.microsoft.com/en-us/graph/api/calendar-list-calendarview?view=graph-rest-1.0) | 区分 `singleInstance/seriesMaster/occurrence/exception`；`calendarView(start,end)` 返回范围内的 occurrence、exception 和单次事件 | HTTP/MCP 应提供同一有界 range view |
| [Apple EventKit](https://developer.apple.com/documentation/eventkit/ekrecurrencerule) | recurrence rule 附着在日历项上，按 start/end predicate 查询时间范围 | UI 选择日期范围时才展开，不能对无界列表生成无限结果 |

公开 API 只能证明逻辑模型，不能证明闭源产品内部绝不缓存任何 occurrence。璇础采用相同的逻辑边界，但根据任务系统需要物化进入执行期或已发生操作的 occurrence。

### 1.4 日历事件与任务的关键差异

日历 occurrence 主要承担展示和时间占用；任务 occurrence 还可能拥有完成、reopen、负责人、评论、附件、依赖、项目编号、审计、提醒、Hook 和 MCP 稳定引用。因此纯虚拟模型不足以承载璇础的执行语义。

最终采用：

```text
系列主记录
  -> 在有界时间窗内计算 projected occurrence
  -> 已物化 occurrence / exception 覆盖同槽位投影
  -> skipped/deleted tombstone 排除同槽位投影
  = Web / HTTP / MCP 看到的一致 TaskOccurrenceView
```

## 2. 产品结论

本规格锁定以下决策：

| 主题 | 决策 |
|---|---|
| 核心语义 | `daily` 是日历驱动。今天未完成，明天仍生成新的独立实例 |
| 停机补偿 | 服务恢复后补齐停机期间所有应生成实例，不吞掉历史日期 |
| 多实例 | 同一系列允许同时存在多条 pending/waiting 实例，旧实例继续逾期 |
| 查询模型 | 有界日期查询合并普通任务、范围内 projected occurrence、已物化 exception，并按槽位去重；无界查询只返回普通任务和已物化 occurrence |
| 物化模型 | 槽位进入执行期、首次写操作、提醒/Hook/自动化需要稳定实体时物化；读取未来投影不写数据库 |
| 可见性兜底 | scheduler 延迟时，范围查询仍返回 projected occurrence；scheduler 不是可见性的唯一来源 |
| 稳定引用 | 所有 occurrence 使用可解析的 `occurrence_ref=(series_id, recurrence_at)`；投影与物化前后引用不变 |
| 单次例外 | occurrence 的字段修改只影响本次，并记录 override；系列共享字段更新不得覆盖已显式修改的字段 |
| 普通任务转循环 | 不支持。创建时必须明确选择普通任务或循环任务 |
| 循环任务转普通 | 不支持。停止循环后，已有未完成实例可继续作为该系列的最后实例处理 |
| 存储路线 | 新建独立 `task_series` 聚合；Task 表只保存普通任务和已物化 occurrence |
| 领域边界 | `Task.status` 不再有 recurring；`Task.parent` 只表示手工父子任务；occurrence 使用 `series_id` |
| 产品资源 | series 是一等领域资源，不是 Task 的隐藏变体，也不可执行 |
| 默认任务列表 | 显示普通任务和循环实例；series 定义通过任务页工具栏中的“循环规则”管理面板治理 |
| 系列管理 | 提供专用 Series CRUD，不再通过普通 `task_modify recur:*` 管理 |
| 截止日期 | `due` 是实例截止时间；系列创建时的 `first_due` 是第一个日历槽位 |
| 循环结束 | `until` 是包含式上界；日期值按本地当天 `23:59:59` 处理 |
| 项目关闭 | archive/cancel 项目时自动停止活跃系列，不在关闭项目中继续生成任务 |
| 系列恢复 | 首版不支持恢复 stopped/ended 系列；需要继续时新建系列 |
| 项目进度 | series 和 occurrence 不进入一次性任务进度分母；列表结果数仍按每条 occurrence 计数，循环执行另给完成率与积压指标 |
| Taskwarrior 边界 | 不兼容 Taskwarrior JSON、recurring parent 或循环命令；仅参考设计思路 |

## 3. 目标

1. 把循环任务从“完成后复制下一条”升级为“按日历槽位生成独立实例”。
2. 建立独立 TaskSeries 领域与仓储，从 Task 领域移除 recurring 状态和循环用途的 parent。
3. 明确普通任务、循环系列、循环实例三种角色及各自允许的操作。
4. 为 Web Console 提供循环创建、当前任务列表、系列列表、系列详情、修改、停止、跳过与实例历史闭环。
5. 为 MCP 提供明确的 series tools 和结构化输出，使 Agent 不可能把 series 当成可完成任务。
6. 修复 HTTP、Remote、CLI、MCP、Web Console 对 recurrence 表达式和字段行为的不一致。
7. 保证 SQLite/PostgreSQL、并发调度、多 workspace、审计、Hook 和 `CGO_ENABLED=0` 边界继续成立。

## 4. 非目标

- 不支持 RRULE、cron、工作日、节假日排除、每月第 N 个工作日等高级规则。
- 不支持普通任务与循环系列互转。
- 不支持暂停后恢复；首版只有 active、ended、stopped。
- 不支持给单个实例修改其系列归属。
- series 不能参与手工任务父子层级；occurrence 首版不能作为另一个任务的 child，但可在物化后作为手工子任务的 parent。
- 不支持 Taskwarrior JSON recurring template/child 的兼容导入导出。
- 不支持 `status=recurring`、task `recur` 字段或 `parent` 指向 series 的兼容读写。
- 不支持预生成未来多个月的实例；只生成当前日期已经到达的槽位。
- 不新增独立权限 scope；继续使用 `task:read` / `task:write` 与现有 workspace/project 角色。
- 不改变任务 description/annotation 的 Markdown 字符串持久化契约。
- 不在本规格中重做所有项目统计；只修复循环数据对现有进度的污染，并补充必要指标。

## 5. 方案比较与最终选型

存储聚合与 occurrence 生成是两个正交决策，不能混成一个方案编号。

### 5.1 存储聚合

| 方案 | 优点 | 长期问题 | 决策 |
|---|---|---|---|
| 隐藏 recurring Task | 初始改动较少 | Task status 和 series status 混用；parent 双重语义；所有 task CRUD/query/statistics 都要防模板泄漏 | 淘汰 |
| 独立 `task_series` | 领域、存储、API、MCP、Web 资源一致；parent 回归单一语义；未来扩展规则无需迁移 Task | 当前要新增 repo、关联表并重写 recurring 测试 | **采用** |

没有生产历史数据时，独立聚合的迁移成本最低。继续隐藏模板只会用短期少量代码换取永久分支，不值得。

### 5.2 Occurrence 策略

| 方案 | 优点 | 问题 | 决策 |
|---|---|---|---|
| 完成驱动 | 最小改动 | 上一次未完成会阻止下一次 | 淘汰 |
| 全量预物化 | task 能力直接 | 无法支持无限未来，规则修改成本高 | 淘汰；仅保留“进入执行期物化” |
| 纯虚拟 | 最接近日历 | Hook、提醒、评论、依赖、完成历史都要重做 | 淘汰 |
| 范围投影 + 选择性物化 | 任意有限日期可见；执行后复用 Task 能力 | 需要 merge、stable ref 和写前物化 | **采用** |

### 5.3 最终组合

```text
独立 task_series 聚合
        +
有界范围 projected occurrence
        +
执行期 / 首次写操作 materialization
        +
exception override / tombstone
```

这是一套最终边界，不是为以后保留 hidden Task 的过渡方案。

## 6. 术语与角色

| 术语 | 存储形态 | 是否出现在默认任务列表 | 是否可执行 |
|---|---|---:|---:|
| 普通任务 | `tasks` row，`series_id` 为空；`parent` 仅表达手工父子任务 | 是 | 是 |
| 循环系列 / series | 独立 `task_series` row，保存规则和共享字段 | 否，通过任务页内“循环规则”管理面板治理 | 否 |
| 投影实例 / projected occurrence | 由系列在有界时间窗内计算，尚无 task row | 仅在范围视图中 | 是；首次写操作先物化 |
| 已物化实例 / materialized occurrence | 普通 task row，关联 series，拥有不可变 `recurrence_at` | 是 | 是 |
| 例外 / exception | 已物化且至少一个字段显式偏离系列共享字段的 occurrence | 是 | 是 |
| tombstone | status=deleted 的已物化 occurrence，用于阻止被规则重新投影 | 按 deleted 筛选 | 否 |
| 日历槽位 / slot | 一次应执行日期，由 `recurrence_at` 唯一标识 | 不单独展示 | 否 |
| 当前实例 | 已进入执行期且未 completed/deleted 的 occurrence；可以有多条 | 是 | 是 |
| 历史实例 | completed/deleted occurrence | 按筛选展示 | 只允许查看；completed 可 reopen |

“父任务”在领域、协议和产品 UI 中只指手工任务层级。循环实例通过 `series_id` 关联系列，在界面上展示为“所属循环系列”，不得写入或猜测 `parent`。

## 7. 领域与数据模型

### 7.1 独立 `task_series` 聚合

新增 `task_series`：

```text
id                 UUID PK
workspace_id       UUID NOT NULL
project_id         UUID NOT NULL
title              TEXT NOT NULL
description        TEXT NULL
status             TEXT NOT NULL  // active|ended|stopped
recurrence_rule    TEXT NOT NULL
first_due          BIGINT NOT NULL
until              BIGINT NULL
effective_end_at   BIGINT NULL
stop_reason        TEXT NULL
priority           TEXT NULL
created_by         UUID NOT NULL
created_at         BIGINT NOT NULL
modified_at        BIGINT NOT NULL
```

共享多值字段使用独立关联表，不塞入 JSON：

```text
task_series_assignees(series_id, user_id)
task_series_tags(series_id, tag)
task_series_uda_values(series_id, uda_definition_id, value...)
task_series_rule_versions(
  id, series_id, effective_from, recurrence_rule,
  created_by, created_at
)
```

`task_series.recurrence_rule` 保存当前规则，便于列表和治理；`task_series_rule_versions` 保存可重建历史投影的规则段。创建系列时写入第一段，`effective_from=first_due`。修改规则时追加新段，不能覆盖旧段：

```text
segment[i] 的槽位：
  从 effective_from[i] 作为新 anchor 按 recurrence_rule[i] 展开
  且 slot < effective_from[i+1]（如存在下一段）
  且 slot <= series.until（如有）
```

这避免系列从 weekly 改为 daily 后，过去未物化的 weekly occurrence 被新规则重算或消失。

约束：

- series 必须属于一个 project；首版不支持 workspace 级无项目系列。
- `status` 只允许 `active|ended|stopped`，且 `effective_end_at` 在 `ended/stopped` 时必填。
- `until` 是包含式槽位上界，存在时必须 `until >= first_due`。
- `recurrence_rule` 必须是 §12 定义的 canonical 表达式。
- rule version 的 `effective_from` 在同一 series 内唯一且严格递增。
- series 不分配 `project_seq`，不能 start/done/reopen，也不出现在 task query。
- series 首版不硬删除；产品“删除循环规则”统一实现为 stop，保留历史引用、审计和 occurrence 可重建性。
- `tasks.series_id` 对 `task_series.id` 使用受限外键；不得级联删除 series 或历史 occurrence。

### 7.2 `tasks` 的 occurrence 字段

`tasks` 移除循环用途的 `recur/mask/imask`；`Task.status` 移除 `recurring`。新增：

```text
series_id                  UUID NULL FK task_series.id
recurrence_at              BIGINT NULL
recurrence_rule_snapshot   TEXT NULL
recurrence_overrides_json  TEXT NOT NULL DEFAULT '[]'
```

两类合法形态：

```text
普通任务：
  series_id IS NULL
  recurrence_at IS NULL
  recurrence_rule_snapshot IS NULL

已物化 occurrence：
  series_id IS NOT NULL
  recurrence_at IS NOT NULL
  recurrence_rule_snapshot IS NOT NULL
```

`parent` 与上述字段正交，只表示手工父子任务。series 不是 task，不能参与父子层级；occurrence 可以作为手工子任务的父任务，创建第一个 child 前先物化 projected occurrence。首版不允许把 occurrence 本身挂到另一个手工父任务下，系列创建/修改接口也不接受 `parent`。

`recurrence_at`：

- 表示该实例对应的不可变日历槽位。
- 创建实例时，`due` 初始等于 `recurrence_at`。
- 用户后续修改实例 `due` 只改变该实例，不改变 `recurrence_at`，也不移动后续系列节奏。
- 后续槽位从覆盖该日期的 rule version anchor 计算，不能从可编辑的 occurrence `due` 计算。
- `recurrence_rule_snapshot` 保存该 occurrence 物化时的 canonical rule；系列改规则后，历史实例仍能解释其生成语义。

这样可以同时支持：

```text
系列槽位：07-11 -> 07-12 -> 07-13
实例 due：07-11 -> 用户改为 07-15
后续生成：仍然生成 07-12、07-13，不被单次延期带偏
```

### 7.3 唯一性与外键

新增跨 SQLite/PostgreSQL 等价的唯一约束：

```text
UNIQUE(workspace_id, series_id, recurrence_at)
WHERE series_id IS NOT NULL AND recurrence_at IS NOT NULL
```

该约束保证同一系列同一日历槽位最多一个实例，且不会限制普通手工父任务下多个同截止日期的子任务。

### 7.4 稳定 occurrence_ref 与公开 id

虚拟 occurrence 不能依赖尚不存在的 task UUID，也不能只返回一个不可反解的 UUIDv5，否则 HTTP/MCP 收到 ID 后无法定位 series 和槽位。定义可解析、URL-safe 的公开引用：

```text
occurrence_ref = occ:<series_uuid>:<recurrence_at_unix>
```

规则：

- `occurrence_ref` 是 occurrence 在投影、物化、完成、改期、跳过前后的稳定公开 `id`。
- materialized occurrence 另有真实 `uuid` 和可选 `task_slug`；projected occurrence 的 `uuid/task_slug` 为 null。
- 普通任务的公开 `id` 继续等于 UUID，同时保留当前 `uuid/task_slug` 字段。
- HTTP 路径、MCP `id`、Web permalink 优先使用公开 `id`；UUID/task_slug 仍可解析已物化任务。
- `ResolveProtocolTarget` 负责解析普通 ref 或 occurrence_ref；先按 `(series_id,recurrence_at)` 查 materialized row。未命中时再校验 workspace/project scope、series 有效区间、槽位属于对应 rule version 且未被 tombstone 排除。规则修改后，旧规则段中的 projected/materialized occurrence 仍可通过 occurrence_ref 读取。
- 物化后 URL 和 MCP 引用不变化；不要把刚分配的 task_slug 替换成 canonical permalink。

### 7.5 单次字段覆盖

`tasks` 表新增 `recurrence_overrides_json TEXT NOT NULL DEFAULT '[]'`，Domain 映射为去重排序后的 `[]string`。使用文本 JSON 保持 SQLite/PostgreSQL 一致，不依赖数据库专属 JSON 运算。首版允许：

```text
title description priority due assignees tags udas wait scheduled depends
```

- projected occurrence 的集合为空。
- 通过普通 task modify 修改 occurrence 时，对应字段加入集合；clear 也算显式覆盖。
- series shared-field modify 只同步未完成且该字段未被 override 的已物化 occurrence。
- completed/deleted occurrence 永不随 series 修改。
- `due` 的 override 永远不改变 `recurrence_at`。
- series rule/until 只影响尚未物化的未来槽位，不写入 override。

### 7.6 系列状态

`task_series.status` 是原生状态：

| status | 含义 | `effective_end_at` |
|---|---|---|
| `active` | 可以继续投影和物化合法槽位 | null |
| `ended` | 已超过包含式 `until`，自然结束 | 最后合法槽位的执行期结束边界 |
| `stopped` | 用户停止或项目关闭，不再产生停止边界之后的槽位 | 停止操作的时间 |

范围展开可读取三种状态，但必须按有效区间裁剪：

```text
first_due <= slot
AND slot <= until（如有）
AND available_at(slot) < effective_end_at（stopped 时）
```

停止不会删除 series；停止前的历史范围仍可重建。`ended/stopped` 首版均为终态，不支持恢复。`stop_reason` 取 `user_stopped|project_archived|project_cancelled`。

### 7.7 SeriesView

App 层新增专用 view，不让 HTTP/MCP 自己拼模板和实例：

```go
type TaskSeriesView struct {
    ID                     string
    WorkspaceID            string
    ProjectID              string
    Project                string
    Title                  string
    Description            *string
    Status                 string // active|ended|stopped
    RecurrenceRule         string
    FirstDue               int64
    Until                  *int64
    Priority               *string
    Tags                   []string
    Assignees              []task.UserInfo
    UDAs                   map[string]task.UDAValue
    OpenOccurrenceCount    int
    CompletedCount         int
    SkippedCount           int
    OverdueCount           int
    LatestOccurrence       *TaskOccurrenceView
    NextRecurrenceAt       *int64
    SuggestedRuleEffectiveFrom *int64
    BacklogRemaining       int
    EffectiveEndAt         *int64 // ended/stopped 的有效边界
    StopReason             *string
    CreatedBy              task.UserInfo
    CreatedAt              int64
    ModifiedAt             int64
}
```

用户信息继续遵守 `task.UserInfo` / `task.JSONUserInfo` 统一规范；若 Series get 暴露 rule-version 摘要，其中的 `created_by` 也必须是完整 UserInfo，而不是裸 UUID。

SeriesView 的 occurrence counts 只统计 materialized rows；未来 projected occurrence 不计入 open count。`NextRecurrenceAt` 是纯日历概念，指严格晚于注入 now 的下一合法槽位，不因该未来 occurrence 是否被提前物化而跳过；它用于展示和 `sort=next`，不能解释为“下一待生成任务”。`SuggestedRuleEffectiveFrom` 才是满足规则修改约束的默认切换槽位，会避开已进入执行期或已物化槽位；`BacklogRemaining` 表示已经进入执行期但尚待物化的槽位数。

### 7.8 TaskOccurrenceView 与 recurrence_info

HTTP、MCP、CLI JSON 和 Web adapter 只能消费 App 层统一的 `TaskOccurrenceView`，不得各自展开规则或拼 exception：

```go
type TaskOccurrenceView struct {
    ID             string              // 普通任务 UUID 或稳定 occurrence_ref
    UUID           *string             // projected 时为空
    TaskSlug       *string             // projected 时为空
    ProjectSeq     *int64              // projected 时为空
    WorkspaceID    string
    ProjectID      *string
    Project        *string
    Title          string
    Description    *string
    Status         string              // projected 固定为 pending
    Entry          *int64              // projected 时为空
    Modified       *int64              // projected 时为空
    Start          *int64
    End            *int64
    Due            *int64
    Wait           *int64
    Scheduled      *int64
    Parent         *string             // 仅手工父任务；occurrence 首版为空
    Priority       *string
    Tags           []string
    Assignees      []task.UserInfo
    Depends        []string
    UDAs           map[string]task.UDAValue
    RecurrenceInfo *RecurrenceInfo
}
```

示例省略 annotations、links 等现有扩展字段；最终 view 必须覆盖当前 task 读取场景所需的完整字段，不能因为引入投影而缩减普通任务响应。不得为 projected occurrence 构造一个不满足 UUID invariant 的 `task.Task` 伪实体。App view 显式承载公共字段；materialized row 和普通 task 再映射进 view。按 `entry` 排序时，projected 使用 `available_at` 作为内部 sort key，但输出 `entry=null`。

HTTP/MCP 的 occurrence view增加派生字段，不再输出循环用途的 `recur/parent/mask/imask`：

```json
{
  "recurrence_info": {
    "role": "occurrence",
    "series_id": "series-uuid",
    "series_status": "active",
    "rule": "daily",
    "recurrence_at": "2026-07-12T23:59:59+08:00",
    "materialization": "projected",
    "overrides": [],
    "until": "2026-07-31T23:59:59+08:00"
  }
}
```

`materialization` 只允许 `projected|materialized`。手工子任务没有 `recurrence_info`。前端只能依据该对象区分“手工 parent”和“循环系列”，不能只猜 `parent`。

### 7.9 Merge 与覆盖算法

对范围 `[start,end)`：

```text
ordinary       = 查询实际 due 落入范围的普通任务
projected      = 对 active/ended/stopped series 在各自有效区间内展开 recurrence_at 落入范围的槽位
overrides      = recurrence_at 落入范围 OR 实际 due 落入范围的 materialized occurrence
tombstones     = overrides 中 status=deleted 的槽位

result = ordinary
       + (projected 按 occurrence_ref 去重)
       + overrides 覆盖同 occurrence_ref 的 projected
       - tombstones（除非显式查询 deleted）
```

例外同时按原槽位和当前 due 读取是必要的：把 07-12 改期到 07-15 后，07-12 视图必须用 exception 抑制原投影，07-15 视图必须显示改期后的 occurrence。

## 8. 日历生成语义

### 8.1 何时投影、何时物化

创建系列时总是可以立即计算第一个 projected occurrence；仅当 `first_due` 已进入执行期时，创建事务才同时物化它。未来 first_due 不提前写 task row，创建响应仍返回带 occurrence_ref 的 projected first occurrence。

后续实例在其 `recurrence_at` 所在本地日期到达时进入执行期并由 scheduler 物化。由于 date-only `due` 存储为 `23:59:59`，生成门槛取槽位所在日期的本地 `00:00:00`：

```text
recurrence_at = 2026-07-12 23:59:59 +08:00
available_at  = 2026-07-12 00:00:00 +08:00
```

当前实例是否完成不参与生成判断。

以下情况也必须物化：

1. 对 projected occurrence 执行 modify/start/done/skip。
2. 增加评论、附件、链接、依赖或子任务。
3. reminder、Hook、project automation 需要以 task 实体触发。
4. 服务恢复后补齐已经进入执行期但尚未物化的槽位。

`stop` 要求 occurrence 已 materialized 且 start 非空；`reopen` 要求已 materialized 且 completed。对 projected ref 调用这两个动作直接返回现有状态错误，不为失败动作制造实体。

纯读取 `task_query/task_get/GET /tasks` 不物化；读路径不得产生 project_seq、audit 或 Hook。

### 8.2 daily 示例

```text
系列：daily，first_due=07-11，until=07-14

查询任意范围  可投影 A/B/C/D
07-11 00:00  物化实例 A，due=07-11 23:59:59
07-12 00:00  无论 A 是否完成，都物化实例 B
07-13 00:00  无论 A/B 是否完成，都物化实例 C
07-14 00:00  物化实例 D
07-15 00:00  不再物化，系列进入 ended
```

### 8.3 停机补偿

服务停机或本地 CLI 多日未运行后，下一次 reconcile 必须补齐所有已到日期的槽位：

```text
最后已有槽位：07-11
当前日期：    07-16
reconcile：   物化 07-12、07-13、07-14、07-15、07-16 五条实例
```

不允许只创建最新一条，因为缺失日期会破坏每日执行记录、审计和统计。

在补偿完成前，范围查询仍应返回缺失槽位的 projected occurrence，因此用户可见性不受补偿批次影响；`backlog_remaining` 只表示尚未完成实体化和事件分发。

### 8.4 批量上限

- 每个 series 每个数据库事务最多创建 100 条实例。
- 单次 scheduler `RunOnce` 全局最多创建 1000 条。
- 超出后不丢弃槽位；返回 `backlog_remaining`，记录结构化 warning，下一轮继续补齐。
- Web series 列表显示“正在补齐 N 条历史实例”，避免用户误以为生成完成。
- 唯一索引保证多个进程或重复 tick 不会生成重复实例。

### 8.5 时区

首版沿用当前 Service clock 的 `Location()` 和现有 date boundary 规则，不新建 workspace timezone 模型：

- date-only `due` / `until`：本地 `23:59:59`
- `recurrence_at` 的日期判断：同一 Location 的本地 `00:00:00`
- monthly 继续使用 Go `time.AddDate(0, n, 0)` 月末滚动语义
- range 使用左闭右开 `[start,end)`；HTTP 日期上界 `due_before=YYYY-MM-DD` 转换为下一日 00:00 的排他上界

后续若引入 workspace timezone，必须整体迁移日期解析、scheduler 和 recurrence，不允许只在 Web 端补时区。

## 9. 调度架构与生成流程

新增 `TaskSeriesScheduler`，复用现有 `ProjectAutomationScheduler` 的 `Clock`、`ServiceFactory`、`RunOnce`、`Run` 和 server goroutine 生命周期模式。

### 9.1 运行入口

| 运行形态 | 触发方式 |
|---|---|
| `xuanchu server` | 启动时先 `RunOnce`，之后每 60 秒扫描一次 |
| 本地 CLI | 每次 task/project 读写命令前，对当前 workspace 执行一次 reconcile |
| HTTP/MCP | server scheduler 负责执行期物化；范围读取可独立投影；对 projected occurrence 的合法写操作在事务中先物化 |
| import | 导入事务完成后对受影响 workspace 执行 reconcile |
| 项目关闭 | transition 事务中停止该项目全部 active series |

### 9.2 Scheduler 流程图

```text
+-------------------------------+
| RunOnce(now)                  |
+---------------+---------------+
                |
                v
+-------------------------------+
| 批量读取 task_series          |
| WHERE status=active           |
+---------------+---------------+
                |
                v
+-------------------------------+
| 项目是否 active/planning?     |
+---------+---------------------+
          | 否                         | 是
          v                            v
+--------------------+       +-------------------------------+
| 停止 series        |       | 找最新 recurrence_at          |
| reason=project_closed|      | 无实例则从 first_due 开始     |
+--------------------+       +---------------+---------------+
                                            |
                                            v
                              +-------------------------------+
                              | 计算下一日历槽位              |
                              +---------------+---------------+
                                              |
                               +--------------+--------------+
                               | 是否超过 until？            |
                               +------+----------------------+
                                      | 是             | 否
                                      v                v
                             +----------------+  +-------------------------+
                             | series -> ended|  | available_at <= now？   |
                             +----------------+  +------+------------------+
                                                       | 否          | 是
                                                       v             v
                                                +-------------+ +------------------+
                                                | 本轮结束    | | 物化 occurrence  |
                                                +-------------+ | 写 audit/event    |
                                                                +--------+---------+
                                                                         |
                                                                         +--> 继续下个槽位
```

### 9.3 物化实例事务

每个槽位的写入必须在 app 层事务中完成：

```text
解析/计算 slot 与 occurrence_ref
  -> 再次校验 series 状态、规则、until 与项目状态
  -> INSERT occurrence（唯一索引幂等；UUID 在此生成）
  -> 分配 occurrence 的 project_seq
  -> 写 task.recurrence.generated audit
  -> 生成 task.created HookEvent
  -> 提交事务
  -> 提交后进入现有 Hook / project automation 分发链路
```

series 本身不发 `task.created`，因为它不是可执行任务；创建 series 时发 `task.series.created`；first occurrence 仅在同时物化时单独发 `task.created`。

写前物化与随后动作必须在同一 app 事务中。例如 `task_done(projected)` 不能先提交 pending occurrence 再完成，否则中间状态会被其它 worker 观察。

### 9.4 范围查询流程

```text
TaskQuery(range, occurrence_mode=expand)
  -> 读取普通任务
  -> 读取可能命中窗口的 series
  -> recurrence.ExpandRange(series rule versions, start, end)
  -> 读取 recurrence_at 或实际 due 命中窗口的 materialized occurrence
  -> materialized/exception 覆盖 projected
  -> tombstone 排除 projected
  -> 对合并结果应用 status/assignee/project/priority/tag/q 筛选
  -> 稳定排序和分页
  -> 返回 TaskOccurrenceView；不写数据库
```

## 10. 生命周期与状态图

### 10.1 Series 状态图

```text
                         到达 until，最后槽位已生成
                    +--------------------------------+
                    |                                v
+---------+  创建  +---------+                 +-----------+
| 不存在  | -----> | active  |                 | ended     |
+---------+        +----+----+                 +-----------+
                        |
                        | 用户停止 / 项目关闭
                        v
                   +-----------+
                   | stopped   |
                   +-----------+

ended / stopped 首版均为终态，不支持恢复。
```

### 10.2 Occurrence 状态图

```text
                                  首次写操作
+-----------+  到达执行期/调度   +---------+
| projected | -----------------> | pending |
+-----+-----+                     +----+----+
      | 跳过/提前完成                 |
      +----> 原子物化为 deleted/completed

                     wait 在未来
                   +------------+
                   |            v
+---------+       +---------+  +---------+
| created | ----> | pending |  | waiting |
+---------+       +----+----+  +----+----+
                      |            |
                 start|stop        | wait 到期
                      |            +-------> pending
                      v
               pending + start!=nil
                      |
          +-----------+-----------+
          |                       |
          | 完成                  | 跳过/删除
          v                       v
    +-----------+           +-----------+
    | completed |           | deleted   |
    +-----+-----+           +-----------+
          |
          | reopen
          v
       pending
```

实例完成、重新打开或跳过都不改变其它实例，也不负责创建下一实例；创建完全由日历 reconcile 决定。

## 11. CRUD 与操作边界

### 11.1 普通任务与循环系列不互转

普通任务与循环系列是不同资源，不提供转换操作。generic Task 的创建/修改 DTO 不包含 recurrence 字段；停止系列后，已有 occurrence 仍保持系列归属，不转换成普通任务。

具体约束：

- `AddInput`、`ModifyInput`、HTTP task request、MCP `task_add/task_modify` 均移除 `recur/clear_recur`。
- 旧客户端向 generic Task JSON 发送 `recur` 时，严格 schema 返回 `task_series_endpoint_required`；这只是纠错提示，不代表兼容该字段。
- Web 普通任务与 occurrence 详情不显示可编辑 recurrence rule。
- CLI 不再提供 `add ... recur:*` 或 `modify recur:*` 的循环能力；这些 token 按未知/不支持字段失败。

### 11.2 创建系列

必填：

- title
- project（Web 项目内创建时自动提供）
- recurrence rule
- first due

可选：

- description
- until
- priority
- assignees
- tags
- UDAs

不接受：

- wait
- scheduled
- depends
- manual parent
- start/end/status

校验：

- `until >= first_due`
- recurrence 只接受 canonical 表达式
- project 必须可写且未关闭
- assignees 必须是当前 workspace 成员

创建 `task_series`、关联字段和 `task.series.created` 必须原子完成。若 first_due 已进入执行期，同时物化 first occurrence 并产生 `task.created`；若 first_due 在未来，只返回 projected first occurrence，不分配 UUID/project_seq、不发 `task.created`。

### 11.3 读取系列

Series list 默认返回 active，可显式筛选 `active|ended|stopped|all`。Series get 返回：

- SeriesView
- 所有未完成 occurrences（分页上限 200）
- 最近 completed/skipped occurrences（默认各 10 条）
- 统计与 backlog 状态

Occurrence 历史另用分页端点读取，避免 series get 无限增长。

Series list 在 HTTP、Remote、MCP、CLI 和 Web 管理面板统一支持：

```text
status=active|ended|stopped|all
q=<title/description substring>
assignee=<user reference>
sort=next|title|modified
limit/offset
```

- `q` trim 后按 title/description 做大小写不敏感包含匹配；空值等于不筛选。
- `assignee` 在 App 层按现有稳定用户引用解析为 workspace user ID，再查询 series association；未知或越权引用按现有 assignee 错误处理。
- `next_recurrence_at` 是 App 派生值，不新增数据库列：使用注入时钟和 workspace 时区，按完整 rule-version/effective_end/until 计算严格晚于 now 的最早合法槽位；active 但没有未来合法槽位以及 ended/stopped 返回 null。它不因 occurrence 的 due override 改变。
- 默认 `sort=next`：Storage 先完成 status/q/assignee 候选过滤但不分页，App 为全部候选批量加载规则版本、计算 next_recurrence_at，再将非空值升序、null 置后，最后用 series ID 稳定排序并执行 offset/limit。
- `sort=title` 使用标准化 title 升序 + ID；`sort=modified` 使用 modified_at 降序 + ID。
- `total` 是 Storage 过滤后的候选总数；三种 sort 和分页都在 App 的完整候选集合上执行。Web 禁止只过滤当前页，Storage 也不得在 App 计算 next 之前提前 limit/offset。

### 11.4 修改系列

Series modify 支持两组字段：

| 字段 | 影响范围 |
|---|---|
| title/description/priority/assignees/tags/UDAs | 更新 series；同步到未完成且对应字段未被单次 override 的 materialized occurrence；projected occurrence 自动继承新值 |
| recurrence_rule/until | 只改变尚未生成的未来槽位；已生成 occurrence 不改 |

`first_due` 创建后不可修改。实例 `due` 可单独修改，但不会改变 `recurrence_at` 或未来节奏。

修改 rule 必须带 `effective_from`（date-only 截止槽位），默认是当前规则的下一个尚未进入执行期的槽位。它必须晚于今天及最大已物化 `recurrence_at`，不早于当前规则段的起点，且不超过同请求提交的 `until`。服务端追加 rule version 并更新 series 当前规则；已进入执行期的槽位、已物化槽位和 tombstone 不回写、不重排。响应返回最终 `effective_from` 和未来三次预览。

在修改任何 series 字段前，App 必须先 reconcile 已进入执行期的 backlog。若受单轮上限影响仍有 backlog，返回 `task_recurrence_backlog`，本次修改不提交；这样共享字段与规则修改不会改变本应按旧值生成的历史任务。

将 `until` 改到早于已生成实例不删除实例；series 立即 ended，已有实例继续保留。UI 必须显示影响摘要。

### 11.5 跳过实例

对循环 occurrence 的“删除”在产品中显示为“跳过本次”：

- projected occurrence 直接物化为 deleted tombstone；materialized occurrence status -> deleted
- audit action：`task.recurrence.skipped`
- 不影响 series 和其它实例
- 不立即创建额外实例；scheduler 只按日历槽位生成

普通任务仍显示“删除任务”。

### 11.6 停止系列

停止 series 是产品层的 Delete，但底层不硬删除：

- `task_series.status -> stopped`，写入 `effective_end_at` 与 `stop_reason`
- 不再生成新实例
- 默认保留所有现有未完成实例，允许继续完成、重新打开或跳过
- 可选 `delete_open_occurrences=true`，在同一事务中把所有已物化 open occurrence 标记 deleted，并把停止边界前已进入执行期但尚未物化的 open 槽位物化为 tombstone
- 历史 completed/deleted 实例永远保留
- audit action：`task.series.stopped`

删除 open occurrence 时单次最多处理 1000 个槽位；超过时返回 `task_recurrence_backlog` 且整个 stop 不提交，Web 提示用户先不勾选该选项完成停止，或等待/触发补齐后重试。停止操作不可撤销，Web 必须二次确认。

### 11.7 普通任务操作作用域

在普通任务列表和详情中，所有 occurrence 操作默认只作用于本次，不弹“仅本次/整个系列”：

| 操作 | 作用域 |
|---|---|
| start/done | 仅本次；projected 先物化 |
| stop/reopen | 仅本次；必须分别已开始/已完成，projected 不物化并返回状态错误 |
| 修改 title/description/priority/due/assignees/tags/UDAs | 仅本次并记录 override |
| annotate/link/dependency/sub-task | 仅本次；projected 先物化 |
| delete | 产品文案“跳过本次”，写 tombstone |
| 修改 rule/未来共享字段/停止 | 只能进入 series API/tool/页面 |

series ID 不属于 task target 命名空间；传给普通 task API/tool 时按 task not found 处理。不得让 generic task delete 暗中解释为停止整个系列。

## 12. recurrence 表达式

后端 canonical 表达式保持：

```text
daily
weekly
monthly
<N>days
<N>weeks
<N>months
```

Web 选项与传值：

| UI | 实际值 |
|---|---|
| 每天 | `daily` |
| 每周 | `weekly` |
| 每两周 | `2weeks` |
| 每月 | `monthly` |
| 每季度 | `3months` |
| 每年 | `12months` |
| 自定义每 N 天/周/月 | `<N>days|weeks|months` |

禁止向后端发送 `biweekly`、`quarterly`、`annual`、`yearly`。所有新写入一律拒绝非 canonical 表达式；本版本没有旧 recurrence 数据迁移路径。

## 13. HTTP 与 Remote 契约

### 13.1 统一原则

HTTP、Remote、MCP、CLI JSON 和 Web adapter 必须调用同一组 App 用例并返回同一语义视图：

```text
QueryTaskViews
GetTaskView
MaterializeOccurrenceForWrite
Add/Modify/StopTaskSeries
ListTaskSeriesOccurrences
SkipTaskSeriesOccurrence
```

传输层只负责鉴权、scope、日期输入解析和 envelope，不得自己调用 recurrence parser、merge 投影或猜测 parent 类型。

### 13.2 专用 HTTP API

```text
POST   /api/v1/task-series
GET    /api/v1/task-series
GET    /api/v1/task-series/{seriesRef}
PATCH  /api/v1/task-series/{seriesRef}
DELETE /api/v1/task-series/{seriesRef}?delete_open_occurrences=false
GET    /api/v1/task-series/{seriesRef}/occurrences
POST   /api/v1/task-series/{seriesRef}/occurrences/{occurrenceRef}/skip
```

所有接口继续接受现有 `workspace`、`project` / `project_id` scope 参数，权限复用 `task:read` / `task:write`。

公开 JSON 字段统一使用 `recurrence_rule`、`first_due`、`effective_from`、`until`。`PATCH /task-series/{seriesRef}` 修改 `recurrence_rule` 时必须同时提交 `effective_from`；Series get 返回 `suggested_rule_effective_from` 和当前规则段摘要，供 Web、CLI、Remote 与 MCP 复用。传输层不得把 `recur` 别名写入 App DTO。

`GET /task-series` 的公开查询参数为 `status/q/assignee/sort/limit/offset`，语义使用 §11.3；OpenAPI、Remote `TaskSeriesListInput` 与 MCP `task_series_list` 使用相同枚举和字段名。

`GET /task-series/{seriesRef}/occurrences` 的 `status` 在 HTTP、Remote、MCP、CLI 中统一为 `pending|waiting|completed|deleted|all`，直接复用移除 recurring 后的 canonical Task status enum；`all` 表示不追加状态 predicate，不是持久状态。Series get 的 open occurrence 包含 pending 和 waiting。

### 13.3 Task 查询与范围展开

`GET /api/v1/tasks` 新增并统一以下参数：

```text
due_after=YYYY-MM-DD
due_before=YYYY-MM-DD
occurrence_mode=auto|materialized|expand
```

语义：

| mode | 行为 |
|---|---|
| `auto`（默认） | 同时存在 due_after/due_before 时 expand，否则 materialized |
| `materialized` | 只返回普通任务和已物化 occurrence |
| `expand` | 必须同时提供范围；合并 ordinary/projected/materialized/tombstone |

- 日期范围最大 366 天，超出返回 `task_occurrence_range_too_large`。
- `due_after` 为当天 00:00 inclusive；`due_before` 为当天结束 inclusive，内部转成次日 00:00 exclusive。
- merge 后再应用 status、assignee、project、priority、tag、q、sort、offset、limit。
- `status=pending` 可命中 projected occurrence；completed/deleted 只可能来自 materialized 数据。
- 稳定排序必须包含 `id` 作为最后 tie-breaker，分页不得因投影与实体合并产生重复或漏项。
- 无界 `q` 搜索不展开无限系列，只搜索普通任务和已物化 occurrence。

列表成功响应统一为：

```json
{
  "data": {
    "items": [],
    "total": 0,
    "limit": 200,
    "offset": 0,
    "occurrence_mode": "expand",
    "range": {"start": "2026-07-01", "end": "2026-08-01"}
  }
}
```

HTTP、Remote `TaskViewPageDTO`、MCP `structuredContent.data` 使用同一 page shape；只有外层 transport envelope 不同。`GET /api/v1/reports/{name}` 与 `GET /api/v1/tasks?report={name}` 也返回该 page shape，并接受同一 due range、occurrence_mode、task_type、query、sort、limit、offset 参数。Web 不得再直接依赖裸 `task.JSONTask[]` 判断 recurrence。该响应变化需要同步 OpenAPI、Remote client 和全部调用方，不做前后端双重猜测。

### 13.4 现有 task API

- `POST /api/v1/tasks` 只创建普通任务；出现旧 `recur` 字段返回 `task_series_endpoint_required`，并提示使用 `/task-series`。
- `GET /api/v1/tasks` 只查询 Task 和 occurrence；series 使用 `/task-series`。
- `GET /api/v1/tasks/{id}` 接受 UUID、task_slug 或 occurrence_ref；projected 只返回投影，不物化。
- `PATCH /api/v1/tasks/{id}` 对 projected occurrence 先物化再修改；Task patch schema 不包含 recurrence rule。
- `POST /tasks/{id}/start|done` 对 projected occurrence 原子物化并执行本次动作；`stop/reopen` 对 projected 返回状态错误，不物化。
- `DELETE /api/v1/tasks/{id}` 对 occurrence 执行“跳过本次”；series 不属于该路由。
- annotations、links、dependencies、children 等 task 子资源写入口共享同一个 resolver，但必须先校验请求是否可能成功：新增 annotation/link/dependency/child 时 projected occurrence 先物化；update/delete/remove 已有子资源时，若 projected occurrence 尚未物化，则该子资源必然不存在，直接返回 404 且不物化。
- projected occurrence 的 annotations、links、children、audit 读接口返回与普通空任务相同的空 list/page，不物化；urgency 使用投影合并后的字段即时计算并返回说明，不物化。projected 的 `entry=null`，因此 urgency 不计算 age 项；due、priority、tags、project、UDA 等按继承值计算，blocked/blocking 固定为 false。materialized occurrence 按实体数据读取。
- 空 patch、清空本就为空的集合等无有效变化写入在物化前完成 diff，直接返回当前 projected view，不创建 Task、不写 audit/event。任何参数错误、权限错误、状态错误或子资源不存在错误都不得留下物化实体。
- 所有 occurrence 写响应返回 materialized view，`id` 仍为原 occurrence_ref，同时补充 `uuid/task_slug`。

Urgency 公开响应改为 `UrgencyView={id, uuid:null|string, total, items}`。计算层只接收无 identity invariant 的 urgency value，不接收或伪造 `task.Task`；App 用 TaskOccurrenceView 的稳定 id 和 nullable uuid 包装计算结果。普通任务/materialized occurrence 的公式与现有结果完全一致。

### 13.5 创建响应

专用创建接口返回。以下示例假定创建发生在 first_due 之前，因此 first occurrence 仍是 projected：

```json
{
  "series": {
    "id": "series-uuid",
    "status": "active",
    "recurrence_rule": "daily",
    "first_due": 1783785599,
    "until": 1785513599,
    "open_occurrence_count": 0,
    "next_recurrence_at": 1783785599
  },
  "first_occurrence": {
    "id": "occ:series-uuid:1783785599",
    "uuid": null,
    "task_slug": null,
    "status": "pending",
    "due": "2026-07-11T23:59:59+08:00",
    "recurrence_info": {
      "role": "occurrence",
      "series_id": "series-uuid",
      "series_status": "active",
      "rule": "daily",
      "recurrence_at": "2026-07-11T23:59:59+08:00",
      "materialization": "projected",
      "overrides": []
    }
  }
}
```

若 first_due 已进入执行期，示例中的 materialization 为 `materialized` 且 uuid/task_slug 非空。

Remote client 新增对应 `AddTaskSeries` / `ListTaskSeries` / `GetTaskSeries` / `ModifyTaskSeries` / `StopTaskSeries` / `ListTaskSeriesOccurrences` / `SkipTaskSeriesOccurrence` 方法。旧 `ListTasks` 直接替换为 `QueryTasks(ctx, TaskQueryInput) (TaskViewPageDTO, error)`，旧 `GetTask` 替换为 `GetTaskView(ctx, workspace, taskRef) (TaskOccurrenceDTO, error)`；`AddTask`、`ModifyTask`、`DoneTask`、`DeleteTask`、`StartTask`、`StopTask`、`ReopenTask`、`AnnotateTask`、`DenotateTask`、dependency/link/child 写方法全部接受未预解析的 `taskRef`（AddTask 除外）并返回对应 TaskOccurrenceDTO 或子资源 DTO。`AddTaskInput`/`ModifyTaskInput` 移除 recurrence 字段，不保留旧签名或兼容 wrapper。

Remote DTO 与 HTTP `data` 内层 JSON 同构：`TaskOccurrenceDTO` 使用稳定公开 `id`、nullable `uuid/task_slug/project_seq` 和 `recurrence_info`；`TaskViewPageDTO` 使用 `items/total/limit/offset/occurrence_mode/range`。Remote 不把 projected occurrence 降级成 `task.Task`，也不从 occurrence_ref 猜测 UUID。

现有 Remote `ExplainUrgency` 改为接受原始 taskRef/occurrence_ref 并调用 projected-aware App 用例，不先解析 UUID；projected 返回即时计算结果且不物化。现有 link list/add/remove 同样保留原始 taskRef，遵守 HTTP 的空读、合法新增物化和不存在子资源删除不物化语义。

### 13.6 CLI 契约

使用显式 series 命令承载完整 CRUD：

```bash
xuanchu series add "每日检查投放消耗" --project ops --recur daily --first-due 2026-07-11 --until 2026-07-31
xuanchu series list --project ops
xuanchu series info <series-ref>
xuanchu series modify <series-ref> --recur weekly --effective-from 2026-08-01 --until 2026-12-31
xuanchu series occurrences <series-ref> [--status pending|waiting|completed|deleted|all] [--due-after YYYY-MM-DD] [--due-before YYYY-MM-DD] [--limit N] [--offset N]
xuanchu series stop <series-ref> [--delete-open]
xuanchu series skip <series-ref> <occurrence-ref>
```

`series list` 另接受 `--status`、`--query`、`--assignee`、`--sort=next|title|modified`、`--limit`、`--offset`，与 HTTP/MCP/Remote 的 Series list 完全等价。

`xuanchu add ... recur:*`、`xuanchu <task-ref> modify recur:*` 和 `recur:` 不再是受支持命令。CLI human 输出使用“循环系列/实例/槽位”术语；`--json` 使用与 HTTP/MCP 相同的 SeriesView 和 recurrence_info 形状。

现有任务命令统一扩展：

- `list`、report aliases 和默认 report 接受 `--due-after`、`--due-before`、`--occurrence-mode=auto|materialized|expand`；`expand` 缺完整范围时与 HTTP 一样失败。completed/deleted/overdue/ready 等 report 仍应用各自过滤条件，但是否展开只由这三个参数决定。
- `info/modify/delete/start/stop/reopen/annotate/denotate/append/prepend/edit/link/annotations/urgency/_urgency` 接受 occurrence_ref。写命令必须把原始 ref 交给 App resolver；不得先转换为 UUID。`edit` 先从 TaskOccurrenceView 生成包含稳定 `id`、nullable `uuid` 的临时文档，不物化；只有编辑器成功退出且 diff 非空时才调用 modify，在同一事务中物化并记录 override。取消、编辑器失败或内容未变均不写库。append/prepend 按 modify 语义物化并记录 description override。
- projected occurrence 不进入持久化 working set，不分配数字 ID；含 projected 行的 human 列表在 `ID` 列显示 `-`。用户对 projected 行操作必须使用 occurrence_ref，物化后的 occurrence 才能进入后续 working set。
- `_get <occurrence-ref>.uuid|task_slug|project_seq` 对 projected 返回 JSON `null`，human 输出为空；读取其它字段使用合并后的 TaskOccurrenceView。`_ids` 和 `_uuids` 只输出具有持久化 working-set ID/UUID 的普通任务或 materialized occurrence，不为 projected 合成标识。
- `_projects`、`_tags`、`_unique` 是无界元数据 helper，固定使用 materialized 模式，不枚举未来投影；`_udas`、`_show`、`_version` 不读取任务集合，行为不变。
- human renderer 直接渲染 `TaskOccurrenceView`，projected 的 UUID/SLUG 显示 `-` 并显示“计划实例”标识；CLI JSON 不再经过 `task.Task` 或 Taskwarrior DTO。
- `series info` 只保留摘要、open occurrences 与最近历史；完整历史必须使用可分页的 `series occurrences`，其 page shape 与 HTTP/MCP/Remote 一致。

### 13.7 跨入口行为对照

| 用户意图 | HTTP | MCP | Remote/CLI | 统一 App 行为 |
|---|---|---|---|---|
| 创建普通任务 | `POST /tasks` | `task_add` | `AddTask` / `add` | schema 不含 recurrence |
| 创建循环系列 | `POST /task-series` | `task_series_add` | `AddTaskSeries` / `series add` | 返回 series + first occurrence view |
| 查询任务/日期范围 | `GET /tasks` | `task_query` | `QueryTasks` / list | 同一 occurrence_mode、range、page 和 merge |
| 读取普通任务/occurrence | `GET /tasks/{id}` | `task_get` | `GetTaskView` / info | projected 只读不物化 |
| 修改 occurrence | `PATCH /tasks/{id}` | `task_modify` | `ModifyTask` / modify | projected 先物化；仅本次并记录 override |
| 完成 occurrence | `POST /tasks/{id}/done` | `task_done` | `DoneTask` / done | projected 原子物化为 completed |
| 跳过 occurrence | `DELETE /tasks/{id}` 或 series skip 子资源 | `task_delete` 或 `task_series_occurrence_skip` | `SkipTaskSeriesOccurrence` / series skip | materialize tombstone；不影响系列 |
| 修改系列 | `PATCH /task-series/{id}` | `task_series_modify` | `ModifyTaskSeries` / series modify | 只改未来和未 override 的 open 字段 |
| 停止系列 | `DELETE /task-series/{id}` | `task_series_stop` | `StopTaskSeries` / series stop | 写 stopped + effective_end_at；不再产生边界后的槽位 |
| 列系列实例 | `GET /task-series/{id}/occurrences` | `task_series_list_occurrences` | `ListTaskSeriesOccurrences` / `series occurrences` | page shape 与 TaskOccurrenceView 一致 |

同一对象、同一 actor、同一 scope 在各入口必须得到同一业务结果、错误码、审计和事件。允许不同的只有 HTTP/MCP/CLI 外层 envelope 与 human rendered 文案。

## 14. MCP 设计

### 14.1 与 HTTP 等价的工具边界

新增：

```text
task_series_add
task_series_list
task_series_get
task_series_modify
task_series_stop
task_series_list_occurrences
task_series_occurrence_skip
```

命名遵守 `{资源}_{动作}` 与下划线规范。

`task_add` 只创建普通任务，不增加模糊的 `recur` 字段。Agent 要创建循环任务必须调用 `task_series_add`。`task_delete` 对 occurrence 等价于 HTTP skip；series 只能使用 `task_series_stop`。

### 14.2 输入摘要

```text
task_series_add:
  workspace/project/project_id
  title/description
  recurrence_rule
  first_due | first_due_date
  until | until_date
  priority/assignees/tags/udas

task_series_list:
  workspace/project/project_id
  status: active|ended|stopped|all
  q/assignee
  sort: next|title|modified
  limit/offset

task_series_modify:
  id
  title/description/priority/assignees/tags/udas
  recurrence_rule/until
  effective_from  // 修改 recurrence_rule 时必填；客户端可先使用 get 返回的建议值
  clear: [description, priority, assignees, tags, until, uda.<name>]

task_series_stop:
  id
  delete_open_occurrences: boolean

task_series_list_occurrences:
  id
  status  // pending|waiting|completed|deleted|all，与 Task status canonical enum 同源
  due_after/due_before
  limit/offset

task_series_occurrence_skip:
  series_id
  occurrence_id
```

`task_query` 新增与 HTTP 同名字段：

```text
due_after: YYYY-MM-DD
due_before: YYYY-MM-DD
occurrence_mode: auto|materialized|expand
task_type: all|normal|occurrence
```

约束、366 天上限、过滤顺序、稳定排序、分页和错误码与 HTTP 完全一致。Agent 不需要知道如何展开 recurrence。

### 14.3 输出

- `structuredContent.data` 返回与 HTTP `data` 等价的 SeriesView / TaskOccurrenceView，不构造 series 对应的 Task。
- text content 必须明确写“循环系列”“已物化实例/计划 occurrence”“下一槽位”，不能把 projected 写成已创建任务。
- `task_query` / `task_get` 中 occurrence 通过公开 `id` 与 `recurrence_info` 表达循环语义；projected 的 uuid/task_slug 为空。
- `task_series_get` 返回 series、open occurrences、recent history 和 counts。
- `task_get` 不读取 series；输入 series ID 按 task not found 处理，Agent 应使用 `task_series_get`。
- MCP `Content[0].text` 继续是 ToolEnvelope JSON，与 `structuredContent` 语义一致；不得出现一个返回投影、另一个只返回实体的分叉。
- `task_modify/task_done/task_delete/task_start/task_stop/task_reopen/task_annotate/task_denotate/task_depends/task_link_add/task_link_remove` 均接收原始 occurrence_ref；合法首次写共享 occurrence write resolver。`task_denotate/task_link_remove` 针对 projected occurrence 时因目标子资源不存在直接返回 not found，不物化。
- `task_link_list` 对 projected 返回空集合；`task_get` 中内嵌的 annotations/depends 使用投影视图值；`urgency_explain` 接受 occurrence_ref 并从投影视图计算。MCP 当前没有独立 children/audit tool，不为循环能力新增重复工具；所有现有读 tool 均不物化。
- `task_export/task_import` 只接受 `xuanchu.task-bundle/v1`，与 HTTP、Remote、CLI 使用同一 bundle DTO 和 App 用例，不保留 Taskwarrior array payload。

### 14.4 MCP 行为矩阵

| 输入对象 | 读 tool | 写 tool |
|---|---|---|
| 普通任务 | 返回普通 TaskOccurrenceView | 现有行为 |
| materialized occurrence | 返回 materialized + overrides | 只改本次 |
| projected occurrence_ref | 返回 projected，不写库 | modify/start/done/skip/合法子资源新增先物化；denotate/link-remove 等既有子资源修改返回 not found；stop/reopen 返回状态错误 |
| series ID | 不属于 task tool；使用 task_series_get | 不属于 task tool；使用对应 task_series_* tool |
| 无效 occurrence_ref | task_occurrence_not_found | 同左，不得创建任意槽位 |

### 14.5 MCP 展示示例

```text
循环系列：每日巡检
状态：active
规则：每天
首次截止：2026-07-11
有效至：2026-07-31
未完成实例：3（逾期 2）
下一槽位：2026-07-14

当前实例：
- ops-17 · 2026-07-11 · overdue
- ops-18 · 2026-07-12 · overdue
- ops-19 · 2026-07-13 · pending
```

## 15. Web Console 信息架构

### 15.1 项目任务页

全局侧栏保持现状，不新增“循环任务”一级导航：

```text
个人
  概览
  我的任务
  项目

管理
  成员 / 令牌 / Hook / 通知 / SSO

系统
  工作区 / 审计 / 设置
```

理由：occurrence 属于“我的任务”和项目执行；series 是项目级治理资源。增加全局入口会重复“项目”维度，也会让用户误以为 series 是跨项目任务列表。

项目 Tabs 保持现状，不新增“循环规则”：

```text
[概览] [任务] [活动] [自动化]
```

后端的 `task_series` 是独立聚合，但用户心智中的“循环任务”仍是任务的一种生成方式。项目导航必须围绕用户工作，而不是围绕数据库资源建模。所有要执行的工作只在“任务”中查看；Series 定义的 CRUD 是任务页内的治理能力，不与“任务”并列。

```text
+--------------------------------------------------------------------------------+
| 项目标题 / 状态 / 项目级操作                                                   |
+--------------------------------------------------------------------------------+
| [概览] [任务] [活动] [自动化]                          [导入] [收起右栏]      |
+--------------------------------------------------------------------------------+
| 任务                                                      [循环规则 3] [新建⌄]|
| [搜索] [状态] [优先级] [负责人] [日期] [任务类型] [更多筛选]                  |
+--------------------------------------------------------------------------------+
| 普通任务 + projected occurrence + materialized occurrence | 项目上下文栏       |
+--------------------------------------------------------------------------------+
```

信息架构原则：

- “任务”是唯一执行入口，默认合并普通任务与 occurrence；用户不需要先判断任务来源。
- Series 定义不是可完成的任务行，不能与 occurrence 同时混入任务表，否则会形成“规则一行 + 今天实例一行”的重复表达。
- 工具栏“循环规则 3”显示 active series 数量，点击后在任务页右侧打开管理面板；0 时显示“循环规则”，不显示 0。
- active count 使用 Series list 的分页 `total`（`status=active&limit=1`），不为一个 badge 拉取全部规则。count 加载中仍显示“循环规则”；请求失败不显示错误 badge、不阻断任务列表，打开面板后再呈现可重试错误。
- “新建”使用菜单承载“新建普通任务 / 新建循环任务”；普通创建仍是默认动作，键盘快捷键保持创建普通任务。
- 面板使用 URL 驱动并支持浏览器前进/后退、复制深链和刷新恢复，但打开面板时 ProjectTabs 始终高亮“任务”，任务列表的查询、选择、滚动位置不丢失。
- 桌面端面板与任务表并存；移动端升级为全屏 Sheet。面板关闭后回到原任务上下文，而不是跳去另一个页面。

### 15.2 当前任务列表原型

```text
+--------------------------------------------------------------------------------+
| 标识     | 标题                         | 状态      | 截止       | 负责人 | 操作 |
+--------------------------------------------------------------------------------+
| OPS-17   | 每日检查投放消耗  [↻ 每天]   | 待处理    | 07-11 逾期 | 张三   | ...  |
| OPS-18   | 每日检查投放消耗  [↻ 每天]   | 待处理    | 07-12 逾期 | 张三   | ...  |
| ↻07-19   | 每日检查投放消耗  [↻ 每天]   | 待处理    | 07-19      | 张三   | ...  |
| OPS-20   | 完成季度复盘                  | 待处理    | 07-31      | 李四   | ...  |
+--------------------------------------------------------------------------------+

行操作：
- 普通任务：[开始] [完成] [... 删除]
- 循环实例：[开始] [完成] [... 跳过本次] [... 管理循环]
```

已物化 occurrence 显示 task_slug；尚未物化的未来投影没有 project_seq/task_slug，标识列显示简短的 `↻MM-DD`，链接使用稳定 occurrence_ref。首次写操作后可补充 task_slug，但 canonical URL 不变化。产品界面不显示“virtual/projected”等技术术语。

筛选仍以实例为单位：status、assignee、due、priority、tag 均过滤 occurrence 自身。增加“任务类型：全部/普通/循环”筛选，默认全部。普通状态筛选中移除 `recurring`；series 的 active/ended/stopped 状态只出现在任务页的循环规则管理面板，不能进入任务状态筛选。

无界项目任务页只显示普通任务和已进入执行期/已物化 occurrence；只有 due_after + due_before 构成有限窗口时才请求 `occurrence_mode=expand`。一个系列积压多次时每次各占一行，首版不隐藏工作量；后续允许纯展示层按系列折叠，但折叠汇总行不得拥有完成动作。

### 15.3 循环规则管理面板原型

```text
┌──────────────────────────────────────────────────────────┐
│ 循环规则 3                                   [新建] [×] │
│ 重复执行的任务规则；每一次仍在左侧任务列表完成。          │
├──────────────────────────────────────────────────────────┤
│ [搜索规则____________] [状态：运行中⌄] [负责人⌄]        │
├──────────────────────────────────────────────────────────┤
│ 每日检查投放消耗                              ● 运行中   │
│ 每天 · 下次今天 · 未完成 3 · 逾期 2                 [›] │
│                                                          │
│ 每周提交项目周报                              ● 运行中   │
│ 每周 · 下次周五 · 未完成 1                         [›] │
│                                                          │
│ 月度账单归档                                  已结束      │
│ 每月 · 有效至 06-30                                  [›] │
└──────────────────────────────────────────────────────────┘
```

面板规则：

- 默认只列 active，状态筛选可查看 ended/stopped/all；列表、错误、空状态只占面板，不替换左侧任务执行视图。
- 行点击在同一面板内进入 Series 详情；浏览器 URL 从 `/tasks/series` 变为 `/tasks/series/:seriesRef`。
- 面板 Header 提供“新建”，直接打开统一创建弹窗的 recurring 模式；项目关闭或只读时隐藏。
- 面板不是二级 Tab，不提供“任务 / 循环规则”切换器，也不把规则列表塞进任务表。
- 桌面宽度建议 440–520px；窗口过窄时使用覆盖式 Sheet，保证主列表最小可用宽度。

### 15.4 创建弹窗原型

```text
+--------------------------------------------------------------------------+
| 新建任务                                                                  |
| [普通任务] [循环任务]                                                     |
|                                                                          |
| 标题        [每日检查投放消耗____________________________________]       |
| 描述        [Markdown 编辑器_____________________________________]       |
| 负责人      [张三 ×]                                                     |
| 优先级      [M v]                    标签 [daily, ads____________]       |
|                                                                          |
| 循环规则    [每天 v]                                                      |
| 首次截止 *  [2026-07-11]             循环结束 [2026-07-31]               |
|                                                                          |
| 预览：从 2026-07-11 开始每天生成一条，最后一次为 2026-07-31。             |
|      即使上一条未完成，下一日期仍会生成。                                 |
|                                                                          |
|                                                  [取消] [创建循环任务]    |
+--------------------------------------------------------------------------+
```

循环模式隐藏 `wait`、`scheduled`、`depends`、manual parent。普通模式不显示循环规则。

### 15.5 循环实例详情原型

```text
+--------------------------------------------------------------------------------+
| OPS-18 每日检查投放消耗                                  [开始] [完成] [...]    |
| [待处理] [↻ 每天 · 2026-07-12]                                               |
+--------------------------------------------------------------------------------+
| 此任务属于循环系列“每日检查投放消耗”。                                       |
| 系列每天独立生成；修改本次不会改动其它日期。             [查看循环规则 →]      |
+----------------------------------------------------------+---------------------+
| 正文 / 子任务 / 活动                                     | 属性                |
|                                                          | 截止：07-12 [可改] |
|                                                          | 循环：每天 [只读]  |
|                                                          | 槽位：07-12 [只读] |
|                                                          | 有效至：07-31      |
+----------------------------------------------------------+---------------------+

更多操作：
- 跳过本次
- 复制链接
- 查看循环规则
```

实例详情不把 series 显示为“父任务”。`recurrence_info.rule` / `until` 只读；“查看循环规则”打开任务页管理面板中的 Series 详情，不离开任务执行上下文。

### 15.6 面板内系列详情原型

```text
+--------------------------------------------------------------------------------+
| 循环规则 / 每日检查投放消耗                    [编辑规则] [停止循环] [×]      |
| [运行中] [每天] [首次 07-11] [有效至 07-31]                                  |
+--------------------------------------------------------------------------------+
| 摘要：未完成 3 · 逾期 2 · 已完成 8 · 已跳过 1                               |
| 下一槽位：07-14 23:59:59                                                     |
|                                                                                |
| 未完成实例                                                                     |
| ○ OPS-17  07-11  逾期  张三                                                   |
| ○ OPS-18  07-12  逾期  张三                                                   |
| ◐ OPS-19  07-13        张三                                                   |
|                                                                                |
| 历史实例                                              [查看全部 →]             |
| ✓ OPS-16  07-10  已完成                                                      |
| ⊘ OPS-15  07-09  已跳过                                                      |
+--------------------------------------------------------------------------------+
```

编辑系列对共享字段显示明确提示：“将更新未来实例；未完成实例中未被单独修改的字段也会更新；已完成、已跳过和已单独覆盖的字段不会改变。”

详情仍位于任务页管理面板中。点击 occurrence 打开任务详情时关闭或暂时隐藏规则面板；浏览器返回恢复 Series 详情。历史“查看全部”在面板内分页，不跳转到新的项目 Tab。

### 15.7 停止确认

```text
+--------------------------------------------------------------+
| 停止循环                                                     |
| 停止后不会再生成新任务，历史记录会保留。                     |
|                                                              |
| [ ] 同时跳过当前 3 条未完成实例                              |
|     包含已进入执行期但尚未物化的次数                          |
|                                                              |
|                                  [取消] [确认停止循环]        |
+--------------------------------------------------------------+
```

勾选数量由服务端按停止边界计算，包含 materialized 与已进入执行期的 projected occurrence；超过单次 1000 条时禁用该选项并说明可先仅停止系列。

### 15.8 我的任务信息架构

当前页面的“全部”实际固定为 pending，且 tab filter 会覆盖状态下拉。修订为互斥、可解释的一级预设：

```text
我的任务
[未完成] [今天] [逾期] [无截止日期] [已完成]
```

| 视图 | 精确定义 | occurrence 行为 |
|---|---|---|
| 未完成 | assignee=me 且 status 为 pending 或 waiting | 普通任务 + 已进入执行期或已物化 occurrence；不展开无限未来 |
| 今天 | open（pending 或 waiting）且 due 在本地今天 `[00:00,次日00:00)` | 自动 expand 今天窗口；不混入逾期/已完成 |
| 逾期 | open（pending 或 waiting）且 due < 今天 00:00 | 多个历史 occurrence 各占一行，waiting 也不能遗漏 |
| 无截止日期 | open（pending 或 waiting）且 due is null | projected occurrence 不会出现；允许清除 due 的 materialized occurrence 会出现，仍保留只读 recurrence_at 作为原槽位 |
| 已完成 | assignee=me 且 status=completed | 每次 completed occurrence 独立显示；series ended/stopped 不算任务完成 |

```text
我的任务 / 今天

□ 每日巡检             ↻ 每天   OPS-23   今天 23:59
□ 修复表单回调                   OPS-19   今天 18:00

我的任务 / 逾期

□ 每日巡检             ↻ 每天   OPS-22   昨天
□ 每日巡检             ↻ 每天   OPS-21   7月10日
```

同标题 occurrence 不去重；它们代表不同日期的独立执行责任。`active` 不是 Task status：界面“进行中”由 `status=pending && start!=null` 派生；start/stop 只设置/清除 start。列表行增加直接 start/stop/done/reopen，避免用户必须进入详情。completed occurrence 的 reopen 只作用于本次，不恢复 stopped/ended series。

My Tasks 的 preset、搜索、项目、优先级、任务类型和排序全部迁入 route search params，不再只保存在组件 `useState`。从 My Tasks 打开 Series 面板时，`panelReturnTo` 另外保存 `{scrollTop, focusId, selectedIds}`；返回后先恢复查询，再在数据加载完成后恢复选择、滚动锚点和焦点。无效/已消失的行 ID 静默忽略并聚焦列表容器。

### 15.9 任务详情与编辑作用域

Occurrence 详情顶部固定显示：

```text
↻ 循环任务 · 每天
本次日期：2026-07-12
所属系列：每日检查投放消耗                    [查看循环规则]
```

- 所有普通属性编辑默认“仅本次”并记录 override，不反复弹范围选择。
- `recurrence_at` 和 rule 只读；due 可改并显示“原循环日期”。
- parent UI 只显示真实手工父任务；所属 series 使用独立属性。
- 删除按钮文案为“跳过本次”；确认文案明确不影响后续。
- 修改系列、停止循环只能通过“查看循环规则”打开任务页管理面板完成。
- projected occurrence 的 GET 在存储层保持只读；界面仍可提供编辑。第一次编辑、评论、依赖、链接、子任务或合法生命周期动作后，响应切换为 materialized。

### 15.10 批量操作

批量完成、重新分配、优先级、due 都逐条作用于选中 occurrence；projected 项逐条原子物化。混合删除确认必须写明：“将删除 N 个普通任务，并跳过 M 次循环任务”。普通任务页不提供批量停止所属系列。

### 15.11 项目统计与列表计数

- 当前筛选结果中每条 occurrence 计一行、计一个结果数。
- 一次性项目完成进度排除 series 和 occurrence，避免永久 daily 系列使项目永不完成。
- 另显“循环执行”：活跃系列数、今日完成、最近 7 天完成率、未完成 occurrence、逾期 occurrence。
- 系列自然 ended 或 stopped 不进入“已完成任务”数量。

### 15.12 路由、面包屑与导航高亮

Series 管理作为任务页的 URL 驱动面板，新增静态子路由：

```text
/workspaces/:workspaceSlug/projects/:projectSlug/tasks/series
/workspaces/:workspaceSlug/projects/:projectSlug/tasks/series/:seriesRef
```

保留：

```text
/workspaces/:workspaceSlug/projects/:projectSlug/tasks
/workspaces/:workspaceSlug/projects/:projectSlug/tasks/:taskRef
/my-tasks
```

导航规则：

- series list/detail panel route 都渲染同一个项目任务页，高亮全局侧栏“项目”和项目 Tab“任务”。不得新增 `ProjectTabKey=task-series`。
- Router 将 `/tasks` 建成持久父路由，`/series` 与 `/series/:seriesRef` 是只渲染管理面板的子路由 Outlet；list/detail 切换不能重挂载 ProjectTasksPage。静态 Series 子路由必须优先于动态 `/tasks/:taskRef`，防止把 `series` 解释为 taskRef。
- 任务筛选、排序、分页继续保存在 search params；打开/关闭/切换 Series 面板时原样保留。直接访问深链时使用任务页默认查询并打开面板。
- 从任务行、任务详情或“我的任务”打开面板时，通过 Router location state 记录结构化 `panelReturnTo`：只允许 `{kind:tasks, search, restore}`、`{kind:task, taskRef}`、`{kind:my-tasks, search, restore}`，其中 restore 仅含 scrollTop/focusId/selectedIds；目标 URL 从当前 workspace/project 参数重新生成，不接受任意 href。关闭优先返回来源，直接深链没有来源时回到项目 `/tasks`。复制链接只复制 canonical panel URL，不携带 return state。
- 桌面面板内部 breadcrumb 为“循环规则 > {系列标题}”；项目级 breadcrumb 仍为“项目 > 任务”，避免让治理面板伪装成项目一级页面。
- occurrence detail 使用项目级 canonical route，因此高亮“项目”。从“我的任务”进入时附加非 canonical 的 `from=my-tasks`，页面首个面包屑显示“返回我的任务”；复制链接时去掉该参数。
- 不新增 `/task-series` 全局路由，不新增跨项目 series 总表。
- “循环规则 N”数量只在任务页工具栏按钮显示 active series 数；ProjectTabs 不显示该数量。

信息架构树：

```text
Web Console
├─ 我的任务
│  ├─ 未完成
│  ├─ 今天
│  ├─ 逾期
│  ├─ 无截止日期
│  └─ 已完成
└─ 项目
   └─ {项目}
      ├─ 概览
      ├─ 任务
      │  ├─ 普通任务
      │  ├─ 循环 occurrence
      │  ├─ 任务详情
      │  └─ 循环规则管理面板
      │     ├─ Series 列表
      │     └─ Series 详情 / occurrence 历史
      ├─ 活动
      └─ 自动化
```

### 15.13 项目任务页完整桌面原型

```text
┌──────────────┬──────────────────────────────────────────────────────────────────────────────┐
│ 璇础         │ Ops 投放项目                                             [进行中] [···]    │
│              ├──────────────────────────────────────────────────────────────────────────────┤
│ 个人         │ [概览] [任务] [活动] [自动化]                       [导入] [收起右栏]    │
│  概览        ├──────────────────────────────────────────────────────────────┬───────────────┤
│  我的任务    │ 任务                                 [循环规则 3] [新建任务⌄]│ 项目上下文    │
│  项目 ●      │ [搜索任务______] [状态⌄] [优先级⌄] [负责人⌄] [任务类型⌄]   │               │
│              │                                                              │ 一次性进度    │
│ 管理         │ 筛选：负责人=张三 ×  类型=全部 ×                            │ 18 / 24       │
│  成员        ├───────┬────────────────────────┬────────┬──────────┬──────────┤               │
│  令牌        │ 标识  │ 标题                   │ 状态   │ 截止     │ 负责人   │ 循环执行      │
│  Hook        ├───────┼────────────────────────┼────────┼──────────┼──────────┤ 今日 2 / 3    │
│  通知        │OPS-17 │每日检查投放消耗 ↻每天  │待处理  │07-11逾期 │张三   ···│ 积压 4        │
│              │OPS-20 │完成季度复盘            │进行中  │07-31     │李四   ···│               │
│ 系统         │↻07-19 │每日检查投放消耗 ↻每天  │待处理  │07-19     │张三   ···│ 近期活动      │
│  工作区      └───────┴────────────────────────┴────────┴──────────┴──────────┤ ...           │
│  审计        │                                              1–50 / 128  < > │               │
│  设置        │                                                              │               │
└──────────────┴──────────────────────────────────────────────────────────────┴───────────────┘
```

现有位置调整：

- `ProjectTabs` 保持 `[概览][任务][活动][自动化]`，不增加 Series 资源项。
- “导入任务”继续使用现有 `setTabActions` 放在 Tabs 行右侧，只在“任务”Tab 出现。
- “收起右栏”保持 Tabs 行最右。
- 任务页内容 Header 右侧依次放“循环规则 N”和“新建任务⌄”；前者是次动作，后者是主动作菜单。
- “新建任务⌄”菜单项为“新建普通任务”和“新建循环任务”；按钮主区域/快捷键默认普通任务，菜单可直接进入循环模式。
- 新增“任务类型：全部/普通/循环”筛选；状态选项删除 `recurring`。
- 不在每行增加永久“管理系列”按钮，避免表格变宽；occurrence 的 `···` 中提供“查看循环规则”。
- 打开循环规则时复用右侧 workspace panel slot，临时替换项目上下文栏；关闭后恢复用户原来的右栏展开状态。宽度不足时使用覆盖式 Sheet，不把主表压到不可读。
- 自定义面板打开时，Tabs 行原“收起右栏”按钮的 aria-label/title 改为“关闭循环规则”，点击行为与面板 `[×]` 一致；不能显示错误的“收起项目上下文”文案。

### 15.14 任务页打开循环规则面板的完整桌面原型

```text
┌──────────────┬──────────────────────────────────────────────────────────────────────────────┐
│ 全局侧栏     │ Ops 投放项目                                             [进行中] [···]    │
│ 项目 ●       ├──────────────────────────────────────────────────────────────────────────────┤
│              │ [概览] [任务] [活动] [自动化]                    [导入] [关闭规则] │
│              ├──────────────────────────────────────────────────────────────┬───────────────┤
│              │ 任务                                [循环规则 3] [新建任务⌄]│ 循环规则 3    │
│              │ [搜索] [状态⌄] [负责人⌄] [任务类型：全部⌄]                  │ [搜索规则___] │
│              ├───────┬────────────────────────┬────────┬──────────┤          │ [运行中⌄]     │
│              │OPS-17 │每日检查投放消耗 ↻每天  │待处理  │今天      │          │ 每日检查投放  │
│              │OPS-20 │完成季度复盘            │进行中  │07-31     │          │ 每天·下次今天›│
│              │↻07-19 │每日检查投放消耗 ↻每天  │待处理  │07-19     │          │                │
│              │       │                        │        │          │          │ 每周提交周报  │
│              │       │                        │        │          │          │ 每周·下次周五›│
│              └───────┴────────────────────────┴────────┴──────────┤          │ [新建]    [×] │
└──────────────┴──────────────────────────────────────────────────────────────┴───────────────┘
```

面板规则：

- 左侧任务列表保持可见，明确“执行工作融合、规则管理内聚”的关系。
- 默认 `status=active`；可切换运行中/已结束/已停止/全部。
- 主动作是“新建循环任务”，打开统一创建弹窗并默认循环模式。
- 行点击在面板内进入 series detail；返回只切回面板列表。
- ended/stopped 行不显示编辑/停止，只显示详情和历史。
- 项目关闭时隐藏主动作和写菜单；任务页已有 `ProjectClosedBanner`，面板不重复显示第二个 Banner。

### 15.15 统一创建弹窗

复用当前 `TaskCreateDialog`，不新建第二套完全独立表单。任务页主按钮/快捷键传 `initialMode=normal`；“新建任务⌄ > 新建循环任务”和循环规则面板 Header 的“新建”传 `initialMode=recurring`。

普通模式：

```text
┌──────────────────────────────────────────────────────────────────────┐
│ 新建任务                                                        [×] │
│ 在当前项目创建一项可执行工作。                                      │
│                                                                      │
│ [● 普通任务] [○ 循环任务]                                           │
│                                                                      │
│ 标题 *       [____________________________________________________] │
│ 内容         [ Markdown 编辑器                                    ] │
│              [                                                    ] │
│                                                                      │
│ 优先级 [M⌄]          负责人 [张三 × 李四 ×__________________⌄]     │
│ 截止日期 [选择日期]   计划开始 [选择日期]                            │
│ 暂缓到   [选择日期]   有效至   [选择日期]                            │
│ 标签     [____________________________________________________]     │
│                                                                      │
│ 错误或字段提示                                                       │
│                                                  [取消] [创建任务]  │
└──────────────────────────────────────────────────────────────────────┘
```

循环模式：

```text
┌──────────────────────────────────────────────────────────────────────┐
│ 新建循环任务                                                    [×] │
│ 创建规则后，每个日期都是可以独立完成的一次任务。                      │
│                                                                      │
│ [○ 普通任务] [● 循环任务]                                           │
│                                                                      │
│ 标题 *       [每日检查投放消耗__________________________________]   │
│ 内容         [ Markdown 编辑器                                    ] │
│                                                                      │
│ 优先级 [M⌄]          负责人 [张三 ×__________________________⌄]     │
│ 循环规则 * [每天⌄]   自定义 [每 1 天⌄]                              │
│ 首次截止 * [2026-07-15]       循环结束 [2026-12-31 / 永不结束]       │
│ 标签       [daily, ads________________________________________]     │
│                                                                      │
│ 未来三次：07-15 · 07-16 · 07-17                                    │
│ 提示：上一日未完成，不影响下一日生成新的任务。                        │
│                                                                      │
│ 错误或字段提示                                  [取消] [创建循环任务]│
└──────────────────────────────────────────────────────────────────────┘
```

交互要求：

- mode selector 放在标题说明之后、所有字段之前，不能埋在“更多字段”。
- 两种模式维护各自日期 state；切换模式时隐藏字段不提交，但切回后恢复，避免静默丢值。
- recurring 隐藏 scheduled、wait、depends、manual parent；`due` 改名“首次截止”，`until` 改名“循环结束”。
- rule 选择支持每天、每周、每两周、每月、每季度、每年、自定义 N 天/周/月，只提交 canonical 值。
- first_due 或 rule 变化后即时显示未来三次预览；until 早于 first_due 时在字段下报错。
- `Cmd/Ctrl+Enter` 提交当前模式；pending 时禁用关闭和重复提交。
- 创建普通任务后关闭弹窗并聚焦新任务行；创建 series 后关闭弹窗、刷新融合任务列表与规则缓存，并在任务页右侧打开新 Series 的详情面板，Toast“循环任务已创建”。若 first occurrence 不在当前过滤/日期窗口中，不强行改变筛选，只在 Toast 提供“查看循环规则”。

### 15.16 编辑系列弹窗

```text
┌──────────────────────────────────────────────────────────────────────┐
│ 编辑循环规则：每日检查投放消耗                                  [×] │
│                                                                      │
│ 基本信息                                                             │
│ 标题       [每日检查投放消耗__________________________________]     │
│ 内容       [Markdown__________________________________________]     │
│ 优先级[M⌄] 负责人[张三⌄] 标签[daily, ads____________________]      │
│                                                                      │
│ 规则                                                                 │
│ 循环规则 [每周⌄]     首次截止 [2026-07-01 只读]                      │
│ 循环结束 [2026-12-31 / 永不结束]                                    │
│ 新规则生效 * [2026-07-15⌄]  默认下一次；不得选择今天或过去            │
│ 未来三次：07-15 · 07-22 · 07-29                                    │
│                                                                      │
│ 影响范围                                                             │
│ • 更新未来 occurrence                                                │
│ • 更新 3 条未完成任务中未被单独修改的字段                            │
│ • 不修改 18 条已完成/已跳过记录                                      │
│                                                                      │
│                                              [取消] [保存循环规则]   │
└──────────────────────────────────────────────────────────────────────┘
```

- first_due 永远只读。
- 修改 rule 时必须显示并提交 effective_from，默认下一次尚未进入执行期的槽位；rule/effective_from/until 变化都实时刷新未来三次预览。
- 只改标题等共享字段时仍显示影响摘要，但不显示无关警告。
- 保存成功刷新 series detail、任务列表、我的任务、项目统计和活动。

### 15.17 行菜单与确认弹窗

普通任务行：

```text
···
├─ 打开详情
├─ 复制链接
└─ 删除任务
```

循环 occurrence 行：

```text
···
├─ 打开本次详情
├─ 查看循环规则
├─ 复制本次链接
└─ 跳过本次
```

管理面板内运行中 series 行：

```text
···
├─ 查看详情
├─ 编辑循环规则
└─ 停止循环
```

跳过本次确认：

```text
┌────────────────────────────────────────────────────────────┐
│ 跳过 2026年7月12日这一次？                                 │
│ “每日检查投放消耗”将从未完成任务中移除。                    │
│ 后续循环任务不会受到影响。                                  │
│                                      [取消] [跳过本次]      │
└────────────────────────────────────────────────────────────┘
```

停止循环确认沿用 §15.7。两个危险动作必须使用不同动词和说明；不得复用当前通用“删除任务”确认文案。

### 15.18 我的任务完整原型

```text
┌──────────────┬──────────────────────────────────────────────────────────────────────────────┐
│ 全局侧栏     │ 我的任务                                                                     │
│ 我的任务 ●  │ 跨项目查看分配给我的任务                                                     │
│              ├──────────────────────────────────────────────────────────────────────────────┤
│              │ [未完成 12] [今天 3] [逾期 4] [无截止日期 2] [已完成]                       │
│              │                                                                              │
│              │ [搜索____________] [项目⌄] [优先级⌄] [任务类型⌄] [排序：截止日期⌄]          │
│              ├───────┬──────────────────────────┬────────┬──────────┬──────────┬───────────┤
│              │ 标识  │ 标题                     │ 状态   │ 截止     │ 项目     │ 操作      │
│              ├───────┼──────────────────────────┼────────┼──────────┼──────────┼───────────┤
│              │OPS-17 │每日检查投放消耗 ↻每天    │待处理  │今天      │投放项目  │[✓] [···] │
│              │CRM-31 │修复客户导入              │进行中  │明天      │CRM       │[■] [✓]   │
│              └───────┴──────────────────────────┴────────┴──────────┴──────────┴───────────┘
│              │ 逾期 4 · 今日 3 · 进行中 1                                      1–12 / 12 │
└──────────────┴──────────────────────────────────────────────────────────────────────────────┘
```

- 一级预设已经定义状态范围，因此移除当前重复的“状态”下拉，避免 tab 与 status 相互覆盖。
- 保留搜索、项目、优先级、任务类型和排序；“今天/逾期”不再让用户手工拼日期。
- pending 且 start 为空的行直接显示开始/完成；pending 且 start 非空（界面“进行中”）显示停止和完成；waiting 行显示完成及等待提示；completed 行显示 reopen。
- occurrence 的 `···` 与项目任务页一致，并提供“查看循环规则”。
- 从我的任务打开 occurrence 使用项目级 canonical URL；项目不存在或不可见时显示只读引用而非错误链接。

### 15.19 移动端原型

全局导航继续使用现有 Sheet；项目 Tabs 不增加“循环规则”。任务页 Header 保留明确的规则管理按钮，点击后打开全屏 Sheet：

```text
┌──────────────────────────────┐
│ ☰  Ops 投放项目          ··· │
├──────────────────────────────┤
│ 概览  任务  活动  自动化 →   │
├──────────────────────────────┤
│ 任务       [循环规则 3] [＋⌄]│
│ [搜索____________________]   │
│ [状态⌄] [负责人⌄] [筛选⌄]  │
│                              │
│ ┌──────────────────────────┐ │
│ │ OPS-17      ↻ 每天      │ │
│ │ 每日检查投放消耗         │ │
│ │ 今天 · 张三 · 待处理     │ │
│ │             [完成] [···]│ │
│ └──────────────────────────┘ │
│                              │
│                     [+ 新建] │
└──────────────────────────────┘
```

- 表格降级为 card，recurrence badge 紧邻标识或标题。
- 主动作使用底部右侧 sticky button；页面滚动时不遮挡最后一张卡。
- `···` 打开 bottom sheet，菜单内容与桌面一致。
- 创建/编辑 series 使用全屏 Dialog/Sheet，mode selector 和 Footer 固定，正文区域滚动。
- “循环规则 3”打开全屏 Sheet；Sheet 内列表、详情、历史纵向排列，Header 返回键在详情与列表间切换，关闭键回到原任务滚动位置。
- 移动端深链仍使用 `/tasks/series/:seriesRef`；系统返回先关闭/后退面板层级，不退出整个项目。

### 15.20 空状态、权限与关闭项目

| 场景 | 页面表现 | 主动作 |
|---|---|---|
| 项目没有任务 | “还没有任务” + 说明普通任务/循环 occurrence 都会出现在这里 | 新建任务 |
| 项目没有 series | 任务页保持正常；打开管理面板后显示“还没有循环规则；适合每日巡检、周报等重复执行工作” | 面板内新建循环任务 |
| 我的今天为空 | “今天没有分配给你的任务” | 无，不诱导创建项目任务 |
| series 无历史 | 显示下一槽位和“尚无执行记录” | 编辑/停止 |
| 只读权限 | 隐藏创建、编辑、完成、跳过、停止；保留查看和复制链接 | 无 |
| 项目 archived/cancelled | 显示现有关闭 Banner；任务和 series 全部只读 | 有权限者仅能从项目状态菜单恢复项目，不自动恢复 series |
| scheduler backlog | 管理面板列表/详情显示“正在补齐 N 条历史任务”；任务日期视图仍显示投影结果 | 无需用户重试 |

面板的加载、错误、空状态必须分别呈现；规则 API 失败只影响面板，不得清空或阻断已加载的任务列表，也不能显示成“没有循环规则”。所有按钮、菜单、Sheet 和 Dialog 均提供 i18n key、键盘焦点和明确 aria-label；打开面板后焦点进入 Header，关闭后回到触发按钮。

## 16. Web 数据流

### 16.1 创建流程

```text
TaskCreateDialog 选择“循环任务”
  -> POST /api/v1/task-series
  -> app.AddTaskSeries
  -> transaction(task_series + associations + series audit/event)
  -> first_due 已进入执行期？
       是：同事务物化 first occurrence + task.created
       否：只计算 projected first occurrence
  -> 返回 SeriesView + first_occurrence
  -> invalidate:
       project tasks
       task series panel list/detail
       project summary
       project timeline
  -> 关闭弹窗，保持任务筛选并打开 /tasks/series/{newSeriesRef} 面板
  -> first_occurrence 当前可见时高亮对应行；不可见时不改变筛选
```

### 16.2 实例与系列修改流程

```text
实例详情修改 title/due/...       任务页规则面板修改 rule/shared fields
          |                                   |
          v                                   v
PATCH /tasks/{occurrence}       PATCH /task-series/{series}
          |                                   |
          v                                   v
projected 则先物化              改 task_series
只改本次并记录 override         同步未 override 的 open 实例
                                              |
                                              v
                                未来 scheduler 使用新 rule
```

### 16.3 范围读取与写前物化

```text
Web 日期窗口
  -> GET /tasks?due_after=...&due_before=...&occurrence_mode=expand
  -> 展示 ordinary + projected + materialized merge
  -> 用户点击完成 occurrence_ref
  -> POST /tasks/{occurrence_ref}/done
  -> app 同事务 materialize + done + audit/events
  -> 返回同 id、补充 uuid/task_slug、materialization=materialized
  -> Web 替换当前行并刷新相关统计
```

### 16.4 停止流程

```text
停止循环确认
  -> DELETE /task-series/{id}?delete_open_occurrences=<bool>
  -> task_series -> stopped + effective_end_at
  -> 可选 open occurrences -> deleted
  -> 写 audit/events
  -> 刷新规则面板、任务列表、项目统计和详情
```

### 16.5 Series 管理面板导航流

```text
任务页点击“循环规则 3”
  -> navigate /tasks/series + 保留全部 task search params
  -> 任务列表保持 mounted；右侧项目上下文暂存并切换为规则面板
  -> 点击 Series
       -> navigate /tasks/series/{seriesRef}
       -> 面板内 list -> detail，不替换任务页
  -> 关闭面板
       -> 有 panelReturnTo：返回任务列表/任务详情/我的任务来源
       -> 无 return state：navigate /tasks
       -> 原样保留 search params、选择和滚动位置
       -> 恢复打开前的项目上下文栏状态与触发按钮焦点

直接访问 /tasks/series/{seriesRef}
  -> 加载任务页默认查询 + 指定 Series 详情
  -> 无权限或 Series 不存在：面板显示 404/无权限，任务列表仍可使用
```

## 17. 查询、列表与项目统计

### 17.1 默认任务查询

`app.List` 新增明确的资源边界：

- 普通 task query 只读取 `tasks`；series 只走专用 service/repository。
- `OccurrenceMode=materialized` 返回普通任务和已物化 occurrence。
- `OccurrenceMode=expand` 要求有限 Range，返回 App 层 merge view。
- `OccurrenceMode=auto` 按是否存在完整 Range 选择上述模式。
- occurrences 按本次字段参与 task query/report；projected 从 series 继承字段后再过滤。
- `series_id` 只作为原生结构化过滤字段；`parent:<uuid>` 只查询手工父子任务。
- report 默认 materialized；只有显式提供有限日期窗口和 expand 才包含 future projected occurrence。

这修复当前“有 query 时默认 pending 过滤失效，模板混入 Web 列表”的问题。

### 17.2 查询 DSL 与所有消费者迁移

查询 AST 是 CLI filter/report、HTTP `query=`、MCP `task_query.filters`、Remote `TaskQueryInput.Filters` 以及复用任务查询的通知/自动化规则的共同契约，不能只修改 HTTP 参数：

- 从 AST、parser、SQL compiler、帮助和 schema 中删除 `recur`；`mask/imask` 同样没有查询属性。parser 保留一份仅用于拒绝的 retired-name guard，输入这些旧名称统一返回现有 `unknown attribute` 查询错误，不解释、不迁移为 UDA。
- 新增原生属性 `series_id`、`recurrence_at`、`task_type`。DSL 写法为 `task_type:normal|occurrence`；HTTP/MCP/Remote 结构化参数写作 `task_type=all|normal|occurrence`，其中 `all` 不生成 predicate。`series_id` 只匹配 occurrence；`recurrence_at` 使用现有日期比较操作符和本地日期边界规则。
- `recurrence_rule` 不进入通用 Task DSL。治理和按规则筛选属于 Series list/API，避免 Task 查询重新暴露模板字段。
- `parent` 只匹配手工父子任务；series ID 不能作为 parent 值使用。
- materialized 模式可在 SQL 层执行过滤；expand 模式必须先生成并合并 TaskOccurrenceView，再对普通任务与 projected/materialized occurrence 使用同一 AST evaluator，最后排序、分页。evaluator 必须覆盖全部保留 AST 属性和操作符，包括 uuid（与公开 id 分离）、title/description/status、全部日期字段、project/project_id、priority、depends、annotations、parent、assignee、tag、bare text 和 UDA；不能只实现 recurrence 新字段。projected 的 uuid/entry/modified/start/end 为 null，必须正确命中 `isnull/notnull`，不得用 occurrence_ref、available_at 或 series modified_at 冒充字段值；`available_at` 只可作为内部默认排序键。
- `task_type:normal` 排除所有 occurrence；`task_type:occurrence` 排除普通任务。HTTP/Web 的同名结构化参数编译到同一 AST predicate，禁止维护第二套判断。
- 通知/自动化保存的过滤表达式在写入时使用同一 parser 校验。由于没有历史数据负担，不迁移含旧 recurrence 属性的规则；启动或读取到此类开发数据时明确报错，不能静默忽略条件。

日期范围参数 `due_after/due_before` 与 DSL 的 `due` predicate 可以同时存在，按 AND 组合。`occurrence_mode` 是查询执行选项，不是 DSL 属性。

### 17.3 Report 查询迁移

保留 `GET /api/v1/reports/{name}`、`GET /api/v1/tasks?report={name}`、MCP `report_run` 和 CLI report aliases，但全部收敛到 App `RunTaskViewReport(ReportViewInput) (TaskViewPage, error)`；删除返回 `[]task.Task` 的 `RunReport/ListReport/ReportResult` 旧签名。Remote 在 `TaskQueryInput.Report` 传 report name；`task_query` 仍是无 report definition 的通用查询，避免与 `report_run` 重复。

执行顺序固定为：

1. 组合 project scope、active context、report definition filter 与用户 query AST。
2. 按 occurrence_mode/range 读取 materialized 或生成并 merge TaskOccurrenceView。
3. 用完整 AST evaluator 过滤 merge 结果。
4. 从当前 workspace 的 materialized task 图构建 dependency state；projected occurrence 首版没有 depends，也不能成为未物化的 dependency target，因此 `blocked=false`、`blocking=false`。materialized occurrence 与普通任务按真实依赖图计算。
5. 应用 ready/blocked/blocking 等 report scope；waiting 与 until 使用合并后的 view 字段。
6. 确定 effective sort：请求显式 `sort` 优先，否则使用 report definition sort；effective sort 为 urgency 时从 view 计算。任何排序都使用公开 `id` 作为最终 tie-breaker。
7. 最后执行 offset/limit，返回 TaskViewPage。实现必须先通过一个不接受/不执行 offset、limit 的内部 candidate/merge 用例收集全集；`QueryTaskViews` 和 `RunTaskViewReport` 分别在各自排序/作用域完成后分页，report 不得直接调用已经分页的 `QueryTaskViews`。

`/reports/{name}` 和 `/tasks?report=` 接受与普通 Task 查询相同的 `due_after/due_before/occurrence_mode/task_type/query/sort/limit/offset`，返回同一 page shape、错误码和 OpenAPI schema。无完整范围时默认 materialized，避免 report 展开无限 future occurrence。

### 17.4 项目统计

普通项目进度不得被每日实例数量扭曲：

- 一次性进度的 `task_count` / `pending_count` / `completed_count` 排除所有 occurrences；series 本来不在 task 表中。
- task query 的 `total` 和列表计数仍包含返回的 occurrence，不能把可见行从分页总数中删除。
- 新增：
  - `recurring_series_count`
  - `active_recurring_series_count`
  - `open_recurring_occurrence_count`
  - `overdue_recurring_occurrence_count`
- Web 项目摘要显示“普通任务进度”和“循环任务运行情况”两个口径。

例如：

```text
普通任务：8 / 12 已完成（67%）
循环任务：3 个运行中系列，5 条未完成实例，其中 2 条逾期
```

## 18. 审计、事件与自动化

新增 audit action：

```text
task.series.created
task.series.modified
task.series.ended
task.series.stopped
task.recurrence.generated
task.recurrence.skipped
```

事件规则：

- Series 的创建/修改/结束/停止使用 `task.series.*` 事件。
- 每个 occurrence 创建都发正常 `task.created`，payload 增加 `recurrence_info`。
- projected occurrence 的纯读取不发 audit/event；只有物化时发一次 `task.created`。
- 写前物化和随后的 done/skip/modify 等在同一事务中可产生 `task.created` 与对应动作事件，顺序固定为 created 在前、动作在后。
- occurrence 完成继续发 `task.completed`；重新打开发 `task.reopened`；跳过发 `task.deleted`，同时有 series audit。
- catch-up 创建五条实例就产生五个独立 `task.created` 事实，自动化按现有去重键逐条处理。
- system scheduler actor 使用现有 system actor 机制；对外用户字段继续是完整 `task.UserInfo` 对象。

## 19. 项目关闭语义

项目 transition 到 `archived` 或 `cancelled` 时，在同一 app 事务边界内停止该项目所有 active series：

- `task_series.status -> stopped`，写 `effective_end_at` 和关闭原因
- reason 写入 audit payload：`project_closed`
- 默认保留 open occurrences，但关闭项目的现有写保护使其只读
- 恢复项目状态不会自动恢复 series

不得继续沿用当前“在关闭项目中照常生成 child，只写 warning”的行为。关闭项目代表该执行上下文不再接收新任务。

## 20. 璇础原生数据契约与 Schema 切换

### 20.1 原生 import/export bundle

Taskwarrior JSON 不再是 import、export、CLI `--json`、HTTP 或 MCP 的兼容目标。需要跨环境迁移时使用带版本的璇础原生 bundle：

```json
{
  "schema": "xuanchu.task-bundle/v1",
  "exported_at": "2026-07-12T10:00:00+08:00",
  "task_series": [],
  "tasks": []
}
```

规则：

- `task_series[]` 独立保存 series、完整 rule-version history 及 assignees/tags/UDAs，不伪装成 Task。
- `tasks[]` 保存普通任务和已物化 occurrence；occurrence 带 `series_id`、`recurrence_at`、`recurrence_rule_snapshot`、`recurrence_overrides`。
- projected occurrence 是范围查询视图，不导出，避免导出窗口改变持久数据集合。
- 不导入或导出 hidden recurring task、循环用途的 `parent`、`mask/imask`。
- 导入 occurrence 时，同 bundle 或目标 workspace 中必须存在同 ID series，且 workspace/project 一致；槽位唯一约束必须成立。
- import 与 export 必须校验 schema 版本。未知 major version 拒绝，不做猜测性宽松读取。
- Web JSON/XLSX import 首版只支持普通任务；series bundle 通过专用原生 JSON import/export 完成，避免表格格式丢失聚合和引用关系。
- CLI `export/import`、HTTP `/export|/import`、MCP `task_export/task_import` 与 Remote `ExportTaskBundle/ImportTaskBundle` 使用完全相同的 bundle object。Remote 不再提供返回 `[]task.Task` 或接收 `[]task.JSONTask` 的旧方法。

CLI 单条命令的 `--json` 直接输出当前原生 view：task 使用 TaskOccurrenceView，series 使用 TaskSeriesView；不再补 `recur/mask/imask` 等 Taskwarrior 字段。

### 20.2 无历史负担下的一次性 Schema 切换

当前没有生产历史数据负担，不实现 hidden recurring 数据自动迁移，也不保留双读双写。实现阶段一次完成：

1. 新建 `task_series`、`task_series_rule_versions`、`task_series_assignees`、`task_series_tags`、`task_series_uda_values`。
2. `tasks` 增加 `series_id`、`recurrence_at`、`recurrence_rule_snapshot`、`recurrence_overrides_json` 和 partial unique index。
3. 从 Task domain、storage model、DTO、query/status filter 中移除 `StatusRecurring` 与 `Recur/Mask/IMask`；`parent` 回归手工层级单一语义。
4. 删除完成 occurrence 后复制下一条的旧路径，改由 TaskSeriesScheduler 负责日历物化。
5. 重写 recurring fixtures、测试数据库和 seed；SQLite 如需 rebuild `tasks` table 可以接受。
6. 开发环境若检测到旧 `status=recurring` 数据，启动失败并明确提示重建开发数据库；不静默转换、不删除用户数据，也不提供生产兼容迁移。

普通任务、用户、workspace、project 等非 recurring 数据迁移不在本规格的破坏范围内。正式实现计划必须明确此变更进入破坏性 schema 版本，并在 release notes 写出开发数据库重建要求。

## 21. 错误码

| Code | 场景 |
|---|---|
| `task_series_not_found` | series 不存在或不在有效 scope |
| `task_series_inactive` | 对 ended/stopped series 修改或请求继续生成；已有 occurrence 仍可完成、reopen 或 skip |
| `task_series_due_required` | 缺 first due |
| `task_series_invalid_until` | until 早于 first due |
| `task_series_invalid_rule` | 非 canonical recurrence rule |
| `task_series_invalid_effective_from` | rule 修改缺少有效生效槽位，或生效槽位已进入执行期/超过 until |
| `task_series_unsupported_field` | series 输入 wait/scheduled/depends/parent 等 |
| `task_series_project_closed` | 在关闭项目创建/修改 series |
| `task_series_occurrence_not_found` | task 不属于指定 series |
| `task_recurrence_backlog` | 补偿超过单轮上限；返回可重试元数据，不作为 5xx |
| `task_series_endpoint_required` | generic task add 收到旧 `recur` 字段；提示使用 series API/tool |
| `task_occurrence_not_found` | occurrence_ref 非法、槽位不属于任何 rule version 或超出有效区间 |
| `task_occurrence_range_required` | occurrence_mode=expand 但缺少完整范围 |
| `task_occurrence_range_too_large` | 展开范围超过 366 天 |

HTTP status 与现有错误映射保持一致：输入错误 400、权限 403、scope 隐藏 404、并发冲突 409、内部错误 500。

## 22. 并发与事务约束

- SQLite 和 PostgreSQL 都必须依靠唯一索引做最终 exactly-once 保护，不能只依赖进程内锁。
- `RunOnce` 可被多个 server 实例并发调用；重复 slot insert 应读取已有 occurrence 并视为幂等成功。
- series modify、stop 与 scheduler 生成竞争时，以数据库事务和 `task_series.status` 当前值为准。
- 修改 recurrence rule 时，追加 rule version、更新 `task_series.recurrence_rule` 和同步 open occurrence 必须在同一事务；同一 effective_from 的并发写由唯一约束收敛为 409。
- stop 提交后不得再创建新 occurrence。
- series shared-field update 与 open occurrences 同步必须同事务，避免 series 已更新但实例只更新一半。
- projected occurrence 的并发首次写由 `(workspace_id,series_id,recurrence_at)` 唯一索引收敛；冲突方读取同一 materialized row 后继续动作。
- 物化与本次动作必须同事务；不得暴露只有 task.created、没有请求动作结果的中间状态。
- query merge 本身只读；不得为了稳定分页写 projection cache。
- Hook/audit 写入继续使用现有事务后分发机制，stdout/stderr 契约不变。

## 23. 测试策略

### 23.1 recurrence/domain

- canonical parser 与 Web option 映射一致。
- daily/weekly/monthly/N-unit 计算。
- 多 rule-version 分段展开，切换边界前后各使用正确 anchor/rule，历史投影不被新规则重算。
- `ExpandRange` 左闭右开、最大范围、月末与时区行为。
- `recurrence_at` 不随 occurrence due 修改。
- occurrence_ref 可解析且投影/物化前后稳定。
- RRULE-like merge：exception 覆盖、tombstone 排除、改期同时抑制原槽位并命中新 due。
- until 包含最后槽位。
- series/occurrence invariant，`Task.status` 不含 recurring，`parent` 不承载 series。
- 普通 Task DTO/schema 不包含 recurrence rule。

### 23.2 app/service

- 创建未来 series 只生成 `task_series` + projected first occurrence；first_due 已到时原子物化 first occurrence。
- 上一实例未完成，次日仍生成新实例。
- 停机五日恢复补齐五条。
- batch 100 / global 1000 与 backlog continuation。
- 并发 reconcile 同一 slot 只生成一条。
- 修改 series 同步所有 open occurrences，不改历史。
- 修改 recurrence rule 追加版本段并遵守 effective_from；有 backlog 时拒绝修改且不产生部分写入。
- 修改 occurrence due 不改变未来 slot。
- skip、stop、ended、project close 状态转换。
- stop 的 delete_open 同时处理 materialized open 与已进入执行期的 projected 槽位；超过 1000 时原子拒绝。
- completed recurrence occurrence 可 reopen，且不产生重复 slot。
- series 不发普通 task.created；occurrence 会发。
- projected 纯读不写库；modify/done/skip/annotate/link/dependency 首次写均原子物化。
- series shared modify 不覆盖 recurrence_overrides。
- projected 的 annotation/link/children/audit 读取为空、urgency 可计算且不物化；对不存在子资源的 update/delete/remove 失败且不物化。
- no-op modify 和所有失败写入均不物化、不写 audit/event。
- external edit 的取消、失败和无 diff 不物化；有 diff 时物化与 modify 原子提交。

### 23.3 storage

- SQLite/PostgreSQL partial unique index 等价。
- `task_series`、rule versions、关联表、外键和状态约束。
- recurrence_overrides round-trip 与唯一 occurrence slot。
- series list/count/occurrence pagination 无 N+1。
- range 查询读取原槽位或新 due 命中的 exception。
- 项目统计排除 occurrence，新增循环指标正确。
- workspace/project 行级隔离。

### 23.4 HTTP/Remote

- task-series 全部 CRUD 路由、权限、scope、日期解析。
- task list 只返回 Task/occurrence；auto/materialized/expand 三种模式。
- expand 缺范围、超 366 天、日期边界、过滤后分页。
- `/reports/{name}` 与 `/tasks?report=` 使用同一 TaskViewPage；ready/blocked/blocking/waiting/urgency 在 merge 后执行且最后分页。
- task get 通过 occurrence_ref 返回 projected 且不物化。
- 合法首次 task 子资源写入口共享写前物化；projected 上针对既有子资源的 update/delete/remove 返回 404 且不物化。
- projected 子资源读返回空集合/计算型 urgency，且不物化。
- Remote `QueryTasks/GetTaskView` 和所有 action 使用 TaskOccurrenceDTO/TaskViewPageDTO，不返回 `task.Task`；TaskSeries DTO 与 HTTP schema 对齐。
- Remote native bundle 方法与 CLI/HTTP/MCP 同构，删除旧 task array 方法。
- generic task add 收到未知 `recur` 返回帮助性错误，PATCH schema 无 recur/clear_recur。

### 23.5 MCP

- 七个 `task_series_*` tools schema golden files。
- `task_series_add` 返回 series + first occurrence。
- `task_query` 的 due_after/due_before/occurrence_mode 与 HTTP 语义等价。
- `task_query` / `task_get` 返回 projected/materialized recurrence_info 与稳定 id。
- modify/start/done/skip、annotate、depends、link 对 projected occurrence 原子物化；stop/reopen 不满足前置状态时不物化。
- series ID 不属于普通 task tools，专用 `task_series_*` tools 可完成治理操作。
- structuredContent 与 text content 语义一致。
- MCP tool name 全部使用下划线。
- project/workspace/token allowlist 与 HTTP 权限等价。
- `task_denotate/task_link_remove` 在 projected 上不物化并返回 not found；`task_link_list` 返回空集合且不物化，`task_get` 返回投影视图，`urgency_explain` 从投影视图计算。
- `report_run` 接受 due range、occurrence_mode、task_type 并返回与 HTTP TaskViewPage 等价的 structuredContent/text envelope。
- `task_export/task_import` 只接受 native bundle。

### 23.6 CLI 与查询 DSL

- `series occurrences` 可按状态、due 范围分页读取完整历史；`series info` 不冒充完整历史接口。
- occurrences status 在四端统一为 pending/waiting/completed/deleted/all。
- list/report 的 due range 与 occurrence_mode 和 HTTP/MCP/Remote 等价。
- 所有现有 task 写命令接受 occurrence_ref；projected working-set ID、`_get/_ids/_uuids` 和 human renderer 遵守 13.6。
- query AST 删除 recur/mask/imask，新增 series_id/recurrence_at/task_type，SQL 与 merge evaluator 结果一致。

### 23.7 Web Console

- 全局侧栏和 ProjectTabs 都不新增 series；`/tasks/series[/seriesRef]` 渲染任务页管理面板并始终高亮“任务”。
- 任务页 Header 提供“循环规则 N”和“新建任务⌄”；active count 不进入 ProjectTabs，import action 和右栏 toggle 位置不回归。
- 创建弹窗普通/循环切换与必填校验。
- 创建弹窗从任务主动作/循环规则面板使用正确 initialMode，切换模式保留各自 state，预览未来三次；创建 series 后打开详情面板且不破坏任务筛选。
- Web option 只提交 canonical recurrence。
- 当前任务列表显示 Task/occurrence 和 recurrence badge，不查询 series。
- 项目普通状态筛选移除 recurring，增加任务类型筛选；Series status 只存在于规则管理面板。
- 我的任务具有未完成/今天/逾期/无截止日期/已完成互斥预设；今天不包含逾期。
- 同系列多条积压不去重；completed occurrence 独立显示和 reopen。
- projected 行使用 occurrence_ref permalink，物化后 URL 不变。
- 管理面板内完成 series list/status/filter/detail/history，并支持 URL 深链、前进后退、焦点与任务列表状态恢复。
- occurrence 详情显示 series banner，不显示普通 parent。
- instance edit、skip、reopen。
- series edit/stop/delete-open 二次确认。
- 普通 delete、occurrence skip、series stop 三套菜单和确认文案不混用。
- My Tasks 移除冲突 status selector，直接动作和 from=my-tasks 返回入口正确。
- 项目关闭隐藏写入口。
- project statistics 的普通/循环口径。
- 移动端 ProjectTabs 不含循环规则；任务 card、规则全屏 Sheet、全屏表单与 sticky 主动作符合原型。
- loading/error/empty/readonly/backlog 状态，i18n、aria-label 与键盘焦点。

### 23.8 必跑验证

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
git diff --check
```

## 24. 验收标准

1. daily 系列的昨日实例未完成时，今天仍有一条新的独立 occurrence。
2. 查询未来有限日期范围可看到计算出的 occurrence，不预生成无限未来，也不产生 audit/Hook。
3. 停机五天恢复期间，即使实体补齐尚未完成，日期查询也能看到五个槽位；补齐后重复 reconcile 不产生重复任务。
4. 普通任务和循环系列不能直接互转；Web、HTTP、MCP、Remote、CLI 都不提供转换入口，generic Task schema 不接受 recurrence。
5. Web Console 可以创建、查看、修改、停止 series，可以完成、重新打开和跳过 occurrence。
6. 项目任务、我的未完成、今天、逾期、我的已完成都展示普通任务和符合条件的 occurrence，不混入 series，且各视图日期/状态边界准确。
7. 全局侧栏和 ProjectTabs 不增加循环规则入口；项目任务页融合普通任务与 occurrence，Series list/detail 通过 `/tasks/series[/seriesRef]` 管理面板深链，桌面/移动端的高亮、返回、焦点、筛选和滚动恢复符合 §15。
8. 创建、编辑、跳过、停止弹窗的字段、影响摘要、默认模式和危险操作文案与 ASCII 原型一致。
9. HTTP 与 MCP 对同一 query 返回相同集合和 recurrence_info；projected occurrence 首次写原子物化，公开 id 不变。
10. MCP 可以通过专用 tools 完成 series CRUD，且 series 不会被建模成可 done/start 的 Task。
11. `due` 单次修改不改变日历节奏；`recurrence_at` 保持不可变；series 更新不覆盖本次 override。
12. 项目 archive/cancel 后不再投影或物化新的 occurrence。
13. 项目普通进度不被循环实例污染，循环运行情况有独立指标。
14. SQLite/PostgreSQL 与 `CGO_ENABLED=0` 全部验证通过。
15. README、ROADMAP、manual commands、HTTP/OpenAPI、MCP schema 与 Web 帮助文案同步更新。
16. CLI `series occurrences` 可分页读取完整实例历史；list/report、working set、helper commands 与 human/JSON renderer 对 projected occurrence 的行为符合 §13.6。
17. 查询 DSL 在所有消费者中删除 recur/mask/imask，新增 series_id/recurrence_at/task_type，SQL 与 expand merge evaluator 返回相同集合。
18. projected 子资源读取无副作用；针对不存在子资源的修改/删除、no-op 和所有失败写入均不物化。
19. Remote task 方法返回 TaskOccurrenceDTO/TaskViewPageDTO，native bundle 在 CLI/HTTP/MCP/Remote 间可无损 round-trip，不存在旧 task array wrapper。
20. `/reports/{name}`、`/tasks?report=`、CLI report 与 MCP/Remote 在同一范围和过滤条件下返回相同 TaskViewPage，ready/blocked/blocking/urgency 语义及分页顺序一致。
21. external edit 取消、失败或无变化不物化 projected occurrence；有效 diff 才原子物化并修改。
22. series occurrences 的 pending/waiting/completed/deleted/all 枚举在 HTTP/MCP/Remote/CLI schema 与测试中一致。
23. Series list 的 status/q/assignee/sort/pagination 在 HTTP/MCP/Remote/CLI/Web 返回相同 items 和 filtered total；Web 不过滤当前页伪造结果。
24. My Tasks open presets 同时包含 pending/waiting，不存在 active status；从 My Tasks 打开/关闭规则面板后恢复 preset、筛选、排序、选择、滚动和焦点。

## 25. 实施边界与建议拆分

Implementation plan 应至少拆为以下可独立验证的阶段：

1. `task_series` 聚合、rule versions、Task/occurrence invariant、`recurrence_at`、规则快照、overrides、occurrence_ref 与唯一索引。
2. recurrence ExpandRange、App TaskOccurrenceView、merge/dedupe 与 write resolver。
3. app TaskSeries service、日历 reconcile、审计与事件。
4. scheduler/server/CLI 生命周期接入。
5. HTTP/Remote/OpenAPI task-series、range query 与 projected write 契约。
6. MCP series tools、range query 与 structured output。
7. Web routes、ProjectTabs、统一创建弹窗、项目任务、我的任务各预设与 occurrence 详情。
8. Web series 列表、详情、编辑、停止、历史、移动端与状态页。
9. 项目统计、关闭项目联动、import/export/docs 收口。

不得把前端 recurrence 下拉框作为第一阶段单独上线；在 app invariant、日历 scheduler 和专用 API 未完成前继续隐藏该入口，避免再次制造无效 `recur` 数据。
