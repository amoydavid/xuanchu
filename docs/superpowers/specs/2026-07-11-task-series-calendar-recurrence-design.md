# 璇础循环任务系列与日历驱动实例设计规格

**日期：** 2026-07-11

**状态：** 待实现

**范围：** 任务循环系列的领域语义、日历调度、CLI/HTTP/Remote/MCP 契约与 Web Console 完整 CRUD 体验

**承接：**

- `2026-05-28-taskg-m2-design.md`
- `2026-06-17-web-console-project-task-browsing-design.md`
- `2026-07-07-web-console-task-detail-redesign-design.md`

> 给 agentic workers 的要求：编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

## 1. 背景与问题

璇础已经有基础 recurring 模型：一条 `status=recurring` 的隐藏任务充当模板，实例任务通过 `parent=<模板 UUID>` 与它关联。当前实现可以在创建时生成第一个实例，并在实例完成后生成下一条。

这套模型只完成了 CLI 时代的基础兼容，尚未形成一致的产品能力：

1. 当前生成是**完成驱动**。当天实例未完成时，第二天不会生成新的 daily 实例，不符合“每天都是一次独立执行”的项目管理语义。
2. 普通任务后补 `recur` 只会写字段，不会转换为循环模板，也不会生成实例，形成“看起来循环、实际不循环”的无效状态。
3. HTTP 与 Remote 创建请求支持 `recur`，MCP `task_add` 不支持；MCP `task_modify` 甚至只能清除 `recur`、不能设置，入口能力不一致。
4. Web Console 创建弹窗没有循环任务入口；任务详情却允许直接修改 `recur`，会触发上述无效状态。
5. Web Console 的循环选项包含 `biweekly`、`quarterly`、`annual`，但 Go recurrence parser 只接受 `2weeks`、`3months`、`12months`，前后端契约不一致。
6. 项目任务查询在带 project filter 时可能同时返回隐藏模板和实例；模板还会进入项目任务统计，导致列表重复、编号空洞和进度失真。
7. `parent` 同时表达“手工父子任务”和“循环模板—实例”，现有 Web 详情会把循环模板误展示为普通父任务。
8. 完成生成的下一实例没有独立 `task.created` 事实，自动化和 Hook 无法稳定观察真实的新任务。
9. 服务端已经有 reminder、project automation 等后台 scheduler 基础设施，但循环任务没有复用这条运行时能力。

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
| Scheduler | `internal/app/project_automation_scheduler.go`、`internal/cli/server.go` | 已有可复用的 RunOnce/Run、后台 ServiceFactory、60 秒 tick 和 server 生命周期 |

## 2. 产品结论

本规格锁定以下决策：

| 主题 | 决策 |
|---|---|
| 核心语义 | `daily` 是日历驱动。今天未完成，明天仍生成新的独立实例 |
| 停机补偿 | 服务恢复后补齐停机期间所有应生成实例，不吞掉历史日期 |
| 多实例 | 同一系列允许同时存在多条 pending/waiting 实例，旧实例继续逾期 |
| 普通任务转循环 | 不支持。创建时必须明确选择普通任务或循环任务 |
| 循环任务转普通 | 不支持。停止循环后，已有未完成实例可继续作为该系列的最后实例处理 |
| 存储路线 | 延续现有隐藏 `status=recurring` 模板，不新建平行 `task_series` 主表 |
| 产品资源 | 对外把隐藏模板称为“循环系列”；模板不是可执行任务 |
| 默认任务列表 | 隐藏循环模板，显示普通任务和循环实例 |
| 系列管理 | 提供专用 Series CRUD，不再通过普通 `task_modify recur:*` 管理 |
| 截止日期 | `due` 是实例截止时间；系列创建时的 `first_due` 是第一个日历槽位 |
| 循环结束 | `until` 是包含式上界；日期值按本地当天 `23:59:59` 处理 |
| 项目关闭 | archive/cancel 项目时自动停止活跃系列，不在关闭项目中继续生成任务 |
| 系列恢复 | 首版不支持恢复 stopped/ended 系列；需要继续时新建系列 |
| 项目进度 | 循环模板和循环实例不进入普通任务进度分母，另给循环系列指标 |

## 3. 目标

1. 把循环任务从“完成后复制下一条”升级为“按日历槽位生成独立实例”。
2. 保留 Taskwarrior 风格的 `recur`、`parent`、`status=recurring` 基础兼容，不重写整个任务模型。
3. 明确普通任务、循环系列、循环实例三种角色及各自允许的操作。
4. 为 Web Console 提供循环创建、当前任务列表、系列列表、系列详情、修改、停止、跳过与实例历史闭环。
5. 为 MCP 提供明确的 series tools 和结构化输出，避免 Agent 把隐藏模板当成可完成任务。
6. 修复 HTTP、Remote、CLI、MCP、Web Console 对 recurrence 表达式和字段行为的不一致。
7. 保证 SQLite/PostgreSQL、并发调度、多 workspace、审计、Hook 和 `CGO_ENABLED=0` 边界继续成立。

## 4. 非目标

- 不支持 RRULE、cron、工作日、节假日排除、每月第 N 个工作日等高级规则。
- 不支持普通任务与循环系列互转。
- 不支持暂停后恢复；首版只有 active、ended、stopped。
- 不支持给单个实例修改其系列归属。
- 不支持把循环系列作为手工子任务，也不支持给循环模板添加手工子任务。
- 不支持预生成未来多个月的实例；只生成当前日期已经到达的槽位。
- 不新增独立权限 scope；继续使用 `task:read` / `task:write` 与现有 workspace/project 角色。
- 不改变任务 description/annotation 的 Markdown 字符串持久化契约。
- 不在本规格中重做所有项目统计；只修复循环数据对现有进度的污染，并补充必要指标。

## 5. 方案比较

### 5.1 方案 A：只修 Web 表单，保留完成驱动

做法是给 Web/MCP 增加 `recur` 输入，继续在完成当前实例后创建下一实例。

优点是改动最小；缺点是上一实例未完成时第二天仍没有任务，直接违背本次确认的 daily 语义。该方案淘汰。

### 5.2 方案 B：保留隐藏模板，增加日历驱动 Series 层

继续复用当前 `status=recurring` 模板和实例任务，只补充不可变日历槽位、后台 scheduler、series app service、专用 API/MCP/Web 体验。

优点：

- 复用现有 `internal/recurrence`、Task DTO、项目绑定、权限、审计和多数据库实现。
- 保持 Taskwarrior JSON 字段兼容。
- 可以分阶段迁移现有数据，不需要整体重建任务表。

缺点：

- `parent` 在存储层仍有双重语义，所有对外 view 必须通过 `recurrence_info` 消除歧义。
- 需要新增日历槽位字段和专用查询，不能只改 handler。

**本规格采用方案 B。**

### 5.3 方案 C：新建 `task_series` 主表

把系列模板彻底移出 tasks 表，实例以 `series_id` 外键关联。

这是长期最纯粹的模型，但会同时改写 import/export、Taskwarrior compatibility、查询 DSL、项目编号和大量现有测试，当前收益不足以覆盖迁移风险。只有未来需要 RRULE、暂停窗口、复杂例外日时再重新评估。

## 6. 术语与角色

| 术语 | 存储形态 | 是否出现在默认任务列表 | 是否可执行 |
|---|---|---:|---:|
| 普通任务 | `parent` 不指向循环模板，且不是 series template | 是 | 是 |
| 循环系列 / series | 隐藏模板；active 时 `status=recurring` | 否，进入专用系列列表 | 否 |
| 循环实例 / occurrence | 普通 task row，关联 series，拥有不可变 `recurrence_at` | 是 | 是 |
| 日历槽位 / slot | 一次应执行日期，由 `recurrence_at` 唯一标识 | 不单独展示 | 否 |
| 当前实例 | 已生成且未 completed/deleted 的 occurrence；可以有多条 | 是 | 是 |
| 历史实例 | completed/deleted occurrence | 按筛选展示 | 只允许查看；completed 可 reopen |

“父任务”在产品 UI 中只指手工任务层级。循环实例不得把 series template 展示为普通父任务；应展示为“所属循环系列”。

## 7. 领域与数据模型

### 7.1 保留现有字段

Series template 继续使用现有 Task 字段：

```text
status      = recurring | completed | deleted
recur       = daily | weekly | monthly | <N>days | <N>weeks | <N>months
due         = first_due，系列第一个槽位
until       = 可选，系列生成上界
parent      = null
```

Occurrence 继续是 task：

```text
status      = pending | waiting | completed | deleted
parent      = series template UUID（兼容字段）
recur       = series rule 的创建时快照
until       = series until 的创建时快照
due         = 实例当前截止时间，可做单次调整
```

### 7.2 新增 `recurrence_at`

`tasks` 表新增 nullable `recurrence_at BIGINT`，仅循环实例填写：

- 表示该实例对应的不可变日历槽位。
- 创建实例时，`due` 初始等于 `recurrence_at`。
- 用户后续修改实例 `due` 只改变该实例，不改变 `recurrence_at`，也不移动后续系列节奏。
- 后续槽位必须从上一条 `recurrence_at` 计算，不能从可编辑的 `due` 计算。

这样可以同时支持：

```text
系列槽位：07-11 -> 07-12 -> 07-13
实例 due：07-11 -> 用户改为 07-15
后续生成：仍然生成 07-12、07-13，不被单次延期带偏
```

### 7.3 唯一性

新增跨 SQLite/PostgreSQL 等价的唯一约束：

```text
UNIQUE(workspace_id, parent, recurrence_at)
WHERE parent IS NOT NULL AND recurrence_at IS NOT NULL
```

该约束保证同一系列同一日历槽位最多一个实例，且不会限制普通手工父任务下多个同截止日期的子任务。

### 7.4 系列状态

不新增平行 status 枚举，复用模板任务状态并映射为产品状态：

| 模板 task status | SeriesView.status | 含义 |
|---|---|---|
| `recurring` | `active` | 继续生成 |
| `completed` | `ended` | 已达到 `until`，自然结束 |
| `deleted` | `stopped` | 用户停止或项目关闭导致停止 |

Series template 必须始终满足：

- `recur != nil`
- `due != nil`
- `parent == nil`
- 不接受 `wait`、`scheduled`、`depends`、`start`

新建 series template 不再分配 `project_seq`，因为模板不是项目可执行任务。已有模板的 `project_seq` 保留，不重写历史标识。

### 7.5 SeriesView

App 层新增专用 view，不让 HTTP/MCP 自己拼模板和实例：

```go
type TaskSeriesView struct {
    ID                     string
    WorkspaceID            string
    ProjectID              *string
    Project                *string
    Title                  string
    Description            *string
    Status                 string // active|ended|stopped
    Recur                  string
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
    LatestOccurrence       *task.Task
    NextRecurrenceAt       *int64
    BacklogRemaining       int
    CreatedAt              int64
    ModifiedAt             int64
}
```

用户信息继续遵守 `task.UserInfo` / `task.JSONUserInfo` 统一规范。

### 7.6 Task recurrence_info

HTTP/MCP 的 occurrence view 增加派生字段，旧 `recur` / `parent` 保留：

```json
{
  "recurrence_info": {
    "role": "occurrence",
    "series_id": "series-uuid",
    "series_status": "active",
    "rule": "daily",
    "recurrence_at": "2026-07-12T23:59:59+08:00",
    "until": "2026-07-31T23:59:59+08:00"
  }
}
```

手工子任务没有 `recurrence_info`。前端只能依据该对象区分“手工 parent”和“循环系列”，不能只猜 `parent`。

## 8. 日历生成语义

### 8.1 何时生成

创建系列时立即创建第一个实例，即使 `first_due` 在未来；它代表用户已经安排好的第一次执行。

后续实例在其 `recurrence_at` 所在本地日期到达时生成。由于 date-only `due` 存储为 `23:59:59`，生成门槛取槽位所在日期的本地 `00:00:00`：

```text
recurrence_at = 2026-07-12 23:59:59 +08:00
available_at  = 2026-07-12 00:00:00 +08:00
```

当前实例是否完成不参与生成判断。

### 8.2 daily 示例

```text
系列：daily，first_due=07-11，until=07-14

07-11 00:00  已有实例 A，due=07-11 23:59:59
07-12 00:00  无论 A 是否完成，都生成实例 B
07-13 00:00  无论 A/B 是否完成，都生成实例 C
07-14 00:00  生成实例 D
07-15 00:00  不生成，系列进入 ended
```

### 8.3 停机补偿

服务停机或本地 CLI 多日未运行后，下一次 reconcile 必须补齐所有已到日期的槽位：

```text
最后已有槽位：07-11
当前日期：    07-16
reconcile：   创建 07-12、07-13、07-14、07-15、07-16 五条实例
```

不允许只创建最新一条，因为缺失日期会破坏每日执行记录、审计和统计。

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

后续若引入 workspace timezone，必须整体迁移日期解析、scheduler 和 recurrence，不允许只在 Web 端补时区。

## 9. 调度架构与生成流程

新增 `TaskSeriesScheduler`，复用现有 `ProjectAutomationScheduler` 的 `Clock`、`ServiceFactory`、`RunOnce`、`Run` 和 server goroutine 生命周期模式。

### 9.1 运行入口

| 运行形态 | 触发方式 |
|---|---|
| `xuanchu server` | 启动时先 `RunOnce`，之后每 60 秒扫描一次 |
| 本地 CLI | 每次 task/project 读写命令前，对当前 workspace 执行一次 reconcile |
| HTTP/MCP | 依赖 server scheduler；series create/modify/skip/stop 自身仍在事务中立即收敛 |
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
| 批量读取 active series        |
| status=recurring              |
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
                                                | 本轮结束    | | 创建 occurrence  |
                                                +-------------+ | 写 audit/event    |
                                                                +--------+---------+
                                                                         |
                                                                         +--> 继续下个槽位
```

### 9.3 创建实例事务

每个槽位的写入必须在 app 层事务中完成：

```text
计算 slot
  -> INSERT occurrence（唯一索引幂等）
  -> 分配 occurrence 的 project_seq
  -> 写 task.recurrence.generated audit
  -> 生成 task.created HookEvent
  -> 提交事务
  -> 提交后进入现有 Hook / project automation 分发链路
```

模板本身不再发 `task.created`，因为它不是可执行任务；创建 series 时发 `task.series.created`，第一个 occurrence 单独发 `task.created`。

## 10. 生命周期与状态图

### 10.1 Series 状态图

```text
                         到达 until，最后槽位已生成
                    +--------------------------------+
                    |                                v
+---------+  创建  +---------+                 +-----------+
| 不存在  | -----> | active  |                 | ended     |
+---------+        | recurring|                 | completed |
                   +----+----+                 +-----------+
                        |
                        | 用户停止 / 项目关闭
                        v
                   +-----------+
                   | stopped   |
                   | deleted   |
                   +-----------+

ended / stopped 首版均为终态，不支持恢复。
```

### 10.2 Occurrence 状态图

```text
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

普通任务不能通过 `modify recur:daily` 变成循环任务；循环实例或 series template 也不能通过 `clear_recur` 变成普通任务。

所有入口统一返回：

```text
code: task_recurrence_transition_not_supported
message: 普通任务与循环任务不能直接切换；请新建循环任务或停止现有循环系列
```

具体约束：

- `app.Modify` 对普通任务设置 `Recur`：拒绝。
- `app.Modify` 对 occurrence/template 清除 `Recur`：拒绝。
- `app.Modify` 直接修改 template：拒绝并提示使用 series service。
- Web 普通任务详情不显示可编辑 recurrence 下拉框。
- CLI 旧 `modify recur:*` / `modify recur:` 进入同一错误，不保留假兼容。

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

创建 template、第一个 occurrence、两类 audit/event 必须原子完成。

### 11.3 读取系列

Series list 默认返回 active，可显式筛选 `active|ended|stopped|all`。Series get 返回：

- SeriesView
- 所有未完成 occurrences（分页上限 200）
- 最近 completed/skipped occurrences（默认各 10 条）
- 统计与 backlog 状态

Occurrence 历史另用分页端点读取，避免 series get 无限增长。

### 11.4 修改系列

Series modify 支持两组字段：

| 字段 | 影响范围 |
|---|---|
| title/description/priority/assignees/tags/UDAs | 更新 template，并同步到所有未完成 occurrences；历史 completed/deleted 不改 |
| recur/until | 只改变尚未生成的未来槽位；已生成 occurrence 不改 |

`first_due` 创建后不可修改。实例 `due` 可单独修改，但不会改变 `recurrence_at` 或未来节奏。

修改 rule 时，新规则从当前最大 `recurrence_at` 向后计算；不回写或合并已经生成的槽位。

将 `until` 改到早于已生成实例不删除实例；series 立即 ended，已有实例继续保留。UI 必须显示影响摘要。

### 11.5 跳过实例

对循环 occurrence 的“删除”在产品中显示为“跳过本次”：

- occurrence status -> deleted
- audit action：`task.recurrence.skipped`
- 不影响 series 和其它实例
- 不立即创建额外实例；scheduler 只按日历槽位生成

普通任务仍显示“删除任务”。

### 11.6 停止系列

停止 series 是产品层的 Delete：

- template status -> deleted
- 不再生成新实例
- 默认保留所有现有未完成实例，允许继续完成、重新打开或跳过
- 可选 `delete_open_occurrences=true`，在同一事务中把所有未完成实例标记 deleted
- 历史 completed/deleted 实例永远保留
- audit action：`task.series.stopped`

停止操作不可撤销，Web 必须二次确认。

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

禁止再向后端发送 `biweekly`、`quarterly`、`annual`、`yearly`。这些旧值若来自历史导入，只在读取时映射为 canonical 值并写 system audit；新写入一律拒绝非 canonical 表达式。

## 13. HTTP 与 Remote 契约

### 13.1 专用 HTTP API

```text
POST   /api/v1/task-series
GET    /api/v1/task-series
GET    /api/v1/task-series/{seriesRef}
PATCH  /api/v1/task-series/{seriesRef}
DELETE /api/v1/task-series/{seriesRef}?delete_open_occurrences=false
GET    /api/v1/task-series/{seriesRef}/occurrences
POST   /api/v1/task-series/{seriesRef}/occurrences/{taskRef}/skip
```

所有接口继续接受现有 `workspace`、`project` / `project_id` scope 参数，权限复用 `task:read` / `task:write`。

### 13.2 现有 task API

- `GET /api/v1/tasks` 默认排除 series templates；只有显式 `status=recurring` 的兼容查询可读取 active template。
- `GET /api/v1/tasks/{ref}` 对 occurrence 增加 `recurrence_info`。
- `PATCH /api/v1/tasks/{ref}` 不再接受 recurrence 转换；instance 普通字段仍可修改。
- `DELETE /api/v1/tasks/{ref}` 对 recurrence occurrence 执行 skip 语义，对 series template 执行 stop 语义；新客户端应使用专用 API 获取完整返回。
- `POST /api/v1/tasks` 的 `recur` 为 CLI/Remote 向后兼容入口，内部转调 `AddTaskSeries`；Web 和 MCP 不使用该兼容入口。

### 13.3 创建响应

专用创建接口返回：

```json
{
  "series": {
    "id": "series-uuid",
    "status": "active",
    "recur": "daily",
    "first_due": 1783785599,
    "until": 1785513599,
    "open_occurrence_count": 1,
    "next_recurrence_at": 1783871999
  },
  "first_occurrence": {
    "uuid": "task-uuid",
    "task_slug": "ops-17",
    "status": "pending",
    "due": "2026-07-11T23:59:59+08:00",
    "recurrence_info": {
      "role": "occurrence",
      "series_id": "series-uuid",
      "series_status": "active",
      "rule": "daily",
      "recurrence_at": "2026-07-11T23:59:59+08:00"
    }
  }
}
```

Remote client 新增对应 `AddTaskSeries` / `ListTaskSeries` / `GetTaskSeries` / `ModifyTaskSeries` / `StopTaskSeries` / `SkipTaskSeriesOccurrence` 方法；旧 `AddTask(Recur)` 继续可用但标记 compatibility。

### 13.4 CLI 契约

保留现有兼容创建：

```bash
xuanchu add "每日检查投放消耗" project:ops recur:daily due:2026-07-11 until:2026-07-31
```

新增显式 series 命令承载完整 CRUD：

```bash
xuanchu series add "每日检查投放消耗" project:ops recur:daily due:2026-07-11 until:2026-07-31
xuanchu series list project:ops
xuanchu series info <series-ref>
xuanchu series modify <series-ref> recur:weekly until:2026-12-31
xuanchu series stop <series-ref> [--delete-open]
xuanchu series skip <series-ref> <task-ref>
```

普通 `xuanchu <task-ref> modify recur:*` 和 `recur:` 清空统一拒绝。CLI human 输出使用“循环系列/实例/槽位”术语；`--json` 使用与 HTTP/MCP 相同的 SeriesView 和 recurrence_info 形状。

## 14. MCP 设计

### 14.1 Tools

新增：

```text
task_series_add
task_series_list
task_series_get
task_series_modify
task_series_stop
task_series_skip
```

命名遵守 `{资源}_{动作}` 与下划线规范。

`task_add` 继续只创建普通任务，不增加模糊的 `recur` 字段。Agent 要创建循环任务必须调用 `task_series_add`。

### 14.2 输入摘要

```text
task_series_add:
  workspace/project/project_id
  title/description
  recur
  first_due | first_due_date
  until | until_date
  priority/assignees/tags/udas

task_series_modify:
  id
  title/description/priority/assignees/tags/udas
  recur/until
  clear: [description, priority, assignees, tags, until, uda.<name>]

task_series_stop:
  id
  delete_open_occurrences: boolean

task_series_skip:
  series_id
  task_id
```

### 14.3 输出

- `structuredContent` 返回 SeriesView / occurrence view，不返回裸模板 Task 作为主要对象。
- text content 必须明确写“循环系列”“已生成实例”“下一槽位”，不能写成“task created <template uuid>”。
- `task_query` / `task_get` 中 occurrence 保留 `recur`、`parent`，并新增 `recurrence_info`。
- `task_series_get` 返回 series、open occurrences、recent history 和 counts。
- `task_get` 若读取 template，文本提示使用 `task_series_get`；结构化数据仍保持兼容可读。

### 14.4 MCP 展示示例

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

项目任务页保留“任务”主入口，在页面内增加二级视图：

```text
+--------------------------------------------------------------------------------+
| 项目 / 任务                                                                    |
| [当前任务] [循环系列 3]                         [筛选] [导入] [+ 新建任务]      |
+--------------------------------------------------------------------------------+
```

“当前任务”显示普通任务和 occurrences；series template 永远不作为行出现。“循环系列”显示一行一个 series，承载管理入口。

### 15.2 当前任务列表原型

```text
+--------------------------------------------------------------------------------+
| 标识     | 标题                         | 状态      | 截止       | 负责人 | 操作 |
+--------------------------------------------------------------------------------+
| OPS-17   | 每日检查投放消耗  [↻ 每天]   | 待处理    | 07-11 逾期 | 张三   | ...  |
| OPS-18   | 每日检查投放消耗  [↻ 每天]   | 待处理    | 07-12 逾期 | 张三   | ...  |
| OPS-19   | 每日检查投放消耗  [↻ 每天]   | 进行中    | 07-13      | 张三   | ...  |
| OPS-20   | 完成季度复盘                  | 待处理    | 07-31      | 李四   | ...  |
+--------------------------------------------------------------------------------+

行操作：
- 普通任务：[开始] [完成] [... 删除]
- 循环实例：[开始] [完成] [... 跳过本次] [... 管理循环]
```

筛选仍以实例为单位：status、assignee、due、priority、tag 均过滤 occurrence 自身。增加 `recurring=only|exclude|include`，默认 include。

### 15.3 循环系列列表原型

```text
+--------------------------------------------------------------------------------+
| 循环系列                 | 规则    | 状态   | 未完成/逾期 | 下次槽位 | 有效至  |
+--------------------------------------------------------------------------------+
| 每日检查投放消耗         | 每天    | 运行中 | 3 / 2       | 07-14    | 07-31   |
| 每周提交项目周报         | 每周    | 运行中 | 1 / 0       | 07-18    | -       |
| 月度账单归档             | 每月    | 已结束 | 0 / 0       | -        | 06-30   |
+--------------------------------------------------------------------------------+
| 点击进入系列详情                                      [... 编辑] [... 停止]     |
+--------------------------------------------------------------------------------+
```

默认只列 active，状态筛选可查看 ended/stopped。

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
| 系列每天独立生成；修改本次不会改动其它日期。             [管理循环系列 →]      |
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
- 管理循环系列
```

实例详情不再把 template 显示为“父任务”。`recur` / `until` 只读，series 级修改必须进入 series 详情。

### 15.6 系列详情原型

```text
+--------------------------------------------------------------------------------+
| 循环系列 / 每日检查投放消耗                       [编辑系列] [停止循环]         |
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

编辑系列对共享字段显示明确提示：“将更新所有未完成实例和未来实例；已完成/已跳过历史不会改变。”

### 15.7 停止确认

```text
+--------------------------------------------------------------+
| 停止循环                                                     |
| 停止后不会再生成新任务，历史记录会保留。                     |
|                                                              |
| [ ] 同时跳过当前 3 条未完成实例                              |
|                                                              |
|                                  [取消] [确认停止循环]        |
+--------------------------------------------------------------+
```

## 16. Web 数据流

### 16.1 创建流程

```text
TaskCreateDialog 选择“循环任务”
  -> POST /api/v1/task-series
  -> app.AddTaskSeries
  -> transaction(template + first occurrence + audit/events)
  -> 返回 SeriesView + first_occurrence
  -> invalidate:
       project tasks
       task series list
       project summary
       project timeline
  -> 关闭弹窗并聚焦 first_occurrence 行
```

### 16.2 实例与系列修改流程

```text
实例详情修改 title/due/...       系列详情修改 rule/shared fields
          |                                   |
          v                                   v
PATCH /tasks/{occurrence}       PATCH /task-series/{series}
          |                                   |
          v                                   v
只改本次实例                    改 template + 所有未完成实例
                                              |
                                              v
                                未来 scheduler 使用新 rule
```

### 16.3 停止流程

```text
停止循环确认
  -> DELETE /task-series/{id}?delete_open_occurrences=<bool>
  -> template -> deleted
  -> 可选 open occurrences -> deleted
  -> 写 audit/events
  -> 刷新系列、任务列表、项目统计和详情
```

## 17. 查询、列表与项目统计

### 17.1 默认任务查询

`app.List` 新增明确的 template visibility 规则：

- 默认排除 series templates，无论是否附带 project/query filter。
- 显式 `status=recurring` 或内部 `IncludeSeriesTemplates=true` 才返回 active templates。
- occurrences 按普通任务参与 task query/report。
- `parent:<uuid>` 兼容查询仍可找到 occurrence，但 Web 不靠它识别 series。

这修复当前“有 query 时默认 pending 过滤失效，模板混入 Web 列表”的问题。

### 17.2 项目统计

普通项目进度不得被每日实例数量扭曲：

- `task_count` / `pending_count` / `completed_count` 排除 series template 和所有 occurrences。
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

- Series template 的创建/修改/结束/停止使用 `task.series.*` 事件。
- 每个 occurrence 创建都发正常 `task.created`，payload 增加 `recurrence_info`。
- occurrence 完成继续发 `task.completed`；重新打开发 `task.reopened`；跳过发 `task.deleted`，同时有 series audit。
- catch-up 创建五条实例就产生五个独立 `task.created` 事实，自动化按现有去重键逐条处理。
- system scheduler actor 使用现有 system actor 机制；对外用户字段继续是完整 `task.UserInfo` 对象。

## 19. 项目关闭语义

项目 transition 到 `archived` 或 `cancelled` 时，在同一 app 事务边界内停止该项目所有 active series：

- template -> deleted
- reason 写入 audit payload：`project_closed`
- 默认保留 open occurrences，但关闭项目的现有写保护使其只读
- 恢复项目状态不会自动恢复 series

不得继续沿用当前“在关闭项目中照常生成 child，只写 warning”的行为。关闭项目代表该执行上下文不再接收新任务。

## 20. Import / Export 与迁移

### 20.1 JSON 兼容

- CLI `--json` 与 export 继续保留 `recur`、`parent`、`mask`、`imask`。
- occurrence 可新增 `recurrence_at` 输出；旧消费者忽略未知字段。
- Web JSON/XLSX import 的 recurrence 文案改为 canonical 表达式，不再声称任意字符串原样保存即可形成循环。
- 导入 series template 必须满足完整 series invariant；导入 occurrence 必须能解析到同 workspace 的 series template。
- 普通任务带 `recur` 且没有合法 series parent：拒绝导入。

### 20.2 数据迁移

迁移步骤：

1. 新增 nullable `recurrence_at`。
2. 对 parent 指向现有 `status=recurring` 模板、且 due 非空的 children，回填 `recurrence_at=due`。
3. 建立 partial unique index。
4. 将历史 `biweekly` / `quarterly` / `annual` / `yearly` canonicalize 为 `2weeks` / `3months` / `12months`，写 system audit。
5. 对 `parent IS NULL AND status NOT IN (recurring) AND recur IS NOT NULL` 的孤立无效数据：保留普通任务行为，清除无效 `recur`，写 `task.recurrence.legacy_normalized` system audit；不得自动转换为 series。
6. 首次 scheduler 启动后按日历补齐缺失槽位，受批量上限保护。

迁移不得删除已有任务、修改 occurrence UUID、重排已有 `project_seq`。

## 21. 错误码

| Code | 场景 |
|---|---|
| `task_series_not_found` | series 不存在或不在有效 scope |
| `task_series_inactive` | 对 ended/stopped series 修改模板或请求继续生成；已有 occurrence 仍可完成、reopen 或 skip |
| `task_series_due_required` | 缺 first due |
| `task_series_invalid_until` | until 早于 first due |
| `task_series_invalid_recur` | 非 canonical recurrence |
| `task_series_unsupported_field` | series 输入 wait/scheduled/depends/parent 等 |
| `task_series_project_closed` | 在关闭项目创建/修改 series |
| `task_series_occurrence_not_found` | task 不属于指定 series |
| `task_recurrence_transition_not_supported` | 普通任务与循环系列互转 |
| `task_recurrence_backlog` | 补偿超过单轮上限；返回可重试元数据，不作为 5xx |

HTTP status 与现有错误映射保持一致：输入错误 400、权限 403、scope 隐藏 404、并发冲突 409、内部错误 500。

## 22. 并发与事务约束

- SQLite 和 PostgreSQL 都必须依靠唯一索引做最终 exactly-once 保护，不能只依赖进程内锁。
- `RunOnce` 可被多个 server 实例并发调用；重复 slot insert 应读取已有 occurrence 并视为幂等成功。
- series modify、stop 与 scheduler 生成竞争时，以数据库事务和模板当前 status 为准。
- stop 提交后不得再创建新 occurrence。
- series shared-field update 与 open occurrences 同步必须同事务，避免模板已更新但实例只更新一半。
- Hook/audit 写入继续使用现有事务后分发机制，stdout/stderr 契约不变。

## 23. 测试策略

### 23.1 recurrence/domain

- canonical parser 与 Web option 映射一致。
- daily/weekly/monthly/N-unit 计算。
- `recurrence_at` 不随 occurrence due 修改。
- until 包含最后槽位。
- series/occurrence invariant。
- 普通任务设置/清除 recur 被拒绝。

### 23.2 app/service

- 创建 series 原子生成 template 与 first occurrence。
- 上一实例未完成，次日仍生成新实例。
- 停机五日恢复补齐五条。
- batch 100 / global 1000 与 backlog continuation。
- 并发 reconcile 同一 slot 只生成一条。
- 修改 series 同步所有 open occurrences，不改历史。
- 修改 occurrence due 不改变未来 slot。
- skip、stop、ended、project close 状态转换。
- completed recurrence occurrence 可 reopen，且不产生重复 slot。
- series template 不发普通 task.created；occurrence 会发。

### 23.3 storage

- SQLite/PostgreSQL partial unique index 等价。
- recurrence_at backfill。
- series list/count/occurrence pagination 无 N+1。
- 项目统计排除 template/occurrence，新增循环指标正确。
- workspace/project 行级隔离。

### 23.4 HTTP/Remote

- task-series 全部 CRUD 路由、权限、scope、日期解析。
- task list 默认不返回 template。
- task get 返回 recurrence_info。
- 旧 Remote `AddTask(Recur)` 继续工作。
- 普通 PATCH recur/clear_recur 返回稳定错误码。

### 23.5 MCP

- 六个 `task_series_*` tools schema golden files。
- `task_series_add` 返回 series + first occurrence。
- `task_query` / `task_get` recurrence_info。
- structuredContent 与 text content 语义一致。
- MCP tool name 全部使用下划线。
- project/workspace/token allowlist 与 HTTP 权限等价。

### 23.6 Web Console

- 创建弹窗普通/循环切换与必填校验。
- Web option 只提交 canonical recurrence。
- 当前任务列表隐藏 template、显示 recurrence badge。
- series list/status/filter/detail/history。
- occurrence 详情显示 series banner，不显示普通 parent。
- instance edit、skip、reopen。
- series edit/stop/delete-open 二次确认。
- 项目关闭隐藏写入口。
- project statistics 的普通/循环口径。
- 移动端布局与键盘操作。

### 23.7 必跑验证

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

1. daily 系列的昨日实例未完成时，今天仍有一条新的独立实例。
2. 停机五天恢复后，五个日期的实例全部补齐，且重复 reconcile 不产生重复任务。
3. 普通任务和循环系列不能直接互转，所有入口返回一致错误。
4. Web Console 可以创建、查看、修改、停止 series，可以完成、重新打开和跳过 occurrence。
5. 默认项目任务列表只显示普通任务与 occurrence，不显示 template。
6. MCP 可以通过专用 tools 完成 series CRUD，且输出不会诱导 Agent 对 template 执行 done/start。
7. `due` 单次修改不改变日历节奏；`recurrence_at` 保持不可变。
8. 项目 archive/cancel 后不再生成新 occurrence。
9. 项目普通进度不被循环模板或每日历史实例污染，循环运行情况有独立指标。
10. SQLite/PostgreSQL 与 `CGO_ENABLED=0` 全部验证通过。
11. README、ROADMAP、manual commands、HTTP/OpenAPI、MCP schema 与 Web 帮助文案同步更新。

## 25. 实施边界与建议拆分

Implementation plan 应至少拆为以下可独立验证的阶段：

1. 数据 invariant、`recurrence_at` 迁移、唯一索引与 legacy normalization。
2. app TaskSeries service、日历 reconcile、审计与事件。
3. scheduler/server/CLI 生命周期接入。
4. HTTP/Remote/OpenAPI task-series 契约。
5. MCP series tools 与 structured output。
6. Web 创建、当前任务列表与 occurrence 详情。
7. Web series 列表、详情、编辑、停止与历史。
8. 项目统计、关闭项目联动、import/export/docs 收口。

不得把前端 recurrence 下拉框作为第一阶段单独上线；在 app invariant、日历 scheduler 和专用 API 未完成前继续隐藏该入口，避免再次制造无效 `recur` 数据。
