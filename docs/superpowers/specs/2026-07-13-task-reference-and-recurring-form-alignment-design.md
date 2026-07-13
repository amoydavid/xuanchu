# 任务短引用与循环任务字段对齐设计

**日期：** 2026-07-13

**状态：** 已实现并完成跨协议、双数据库与 Web E2E 验证

**适用范围：** Web Console、HTTP API、MCP、Remote/CLI、App resolver、循环实例物化与现有数据回填

## 1. 文档关系

本文是《[任务系列与日历式循环设计](./2026-07-11-task-series-calendar-recurrence-design.md)》的定向修订，覆盖其中以下旧决定：

- §7.4 中“Web permalink 始终优先 occurrence_ref”和“物化后 URL 不切换到 task_slug”；
- §13.4、§13.7、§14 中根据输入是否为 occurrence_ref 决定返回形态或操作语义的表述；
- §15.2、§15.5、§15.9、§15.12、§15.15 中物化实例仍以 occurrence_ref 作为首选用户链接的表述；
- §16 中物化后仍保留 occurrence_ref Web URL 的数据流；
- 验收项中“物化后 URL 不变”的要求。

未被本文明确修改的 Series、投影、物化、规则版本、查询合并、状态与权限设计继续有效。

## 2. 背景与问题

当前实现暴露出三个相互关联的问题。

### 2.1 已物化实例没有短引用

循环实例物化时只写入 `project_id` 和 `project_seq`，没有写入冗余的项目 slug `project`。`task_slug` 由 `project + project_seq` 计算，因此列表只能退化显示 UUID 前缀。

实际错误数据示例：

```text
uuid          = 6ed61780-802f-4fec-8bf0-2e3fae977e2d
project_slug  = ops
project_seq   = 7
tasks.project = NULL

期望 task_slug = ops-7
```

这不只是展示缺陷，也违反现有 Task 项目绑定约束：

```text
无项目：project = project_id = project_seq = NULL
有项目：project、project_id、project_seq 必须同时存在且互相匹配
```

### 2.2 occurrence_ref 被编码后无法通过 HTTP 路由读取

Web 对动态 path segment 使用 `encodeURIComponent`，`occ:<series_id>:<slot>` 会变成 `occ%3A...%3A...`。Chi 使用 `RawPath` 匹配并把转义后的值保留在 `URLParam` 中。HTTP handler 未执行一次 path unescape，导致 `IsOccurrenceRef` 失败，随后错误降级为普通 UUID/task_slug 查询并返回 404。

### 2.3 用户展示、链接和操作使用不同引用

用户希望在列表看到 `ops-7`，复制和打开的地址也是 `/tasks/ops-7`，进入详情后仍能识别它是循环任务中的一次，并执行“完成本次”“跳过本次”等实例操作。

原 spec 把 occurrence_ref 同时作为机器稳定 ID 和首选 Web permalink。该设计有利于保持投影与物化前后的引用不变，但牺牲了已经拥有 task_slug 的实例的可读性，也使普通任务和已物化循环实例产生不必要的导航差异。

### 2.4 Web 循环任务表单缺少后端已支持字段

Series 的 App、HTTP、MCP、Remote/CLI 已经支持 `description`、`priority`、`assignees`、`tags` 和 `udas`。当前 Web `TaskSeriesForm` 只提交标题、规则、首次日期、结束日期、优先级和标签，遗漏了 description、assignees 和 UDAs，创建与编辑能力都不完整。

## 3. 目标与非目标

### 3.1 目标

1. 普通任务和已物化循环实例统一使用 `{projectSlug}-{taskSeq}` 作为用户可见短引用和首选 Web URL。
2. occurrence_ref 继续作为循环实例投影、物化前后不变的机器稳定 ID，并作为 projected 实例的必要引用。
3. UUID、task_slug、occurrence_ref 任一种合法引用都解析为同一个资源和同一套业务语义。
4. 详情响应和操作语义由解析后的资源类型决定，不由输入字符串形式决定。
5. 循环任务创建/编辑与普通任务共享适用的任务字段和组件。
6. 修复现有不完整项目绑定，并建立防止再次产生错误数据的约束和测试。
7. HTTP、MCP、Remote/CLI 行为保持一致。

### 3.2 非目标

- 不为尚未物化的未来实例提前分配 project sequence 或预留 task_slug。
- 不把 task_series 合并回 tasks 表。
- 不把 `wait`、`scheduled`、`depends` 或 `parent` 变成 Series 共享字段。
- 不删除 occurrence_ref，也不改变其编码格式。
- 不恢复 Taskwarrior JSON 兼容。

## 4. 方案评估与决定

### 4.1 方案 A：稳定 ID 与用户短链接分层（采用）

- occurrence_ref 是机器稳定 ID；
- task_slug 是物化任务的用户短引用；
- 已物化实例首选 `/tasks/{task_slug}`；
- projected 实例没有 task_slug，继续使用 `/tasks/{encoded occurrence_ref}`；
- projected 首次成功写入后获得 task_slug，Web 使用 history replace 把当前地址规范化为短链接；
- UUID 和 occurrence_ref 永久作为可解析别名，不使旧链接失效。

优点：不破坏投影模型，用户链接可读，普通任务与物化实例体验一致。代价是系统需要明确区分“稳定 ID”和“首选用户 URL”。

### 4.2 方案 B：所有 occurrence 永远使用 occurrence_ref（不采用）

实现最简单且与旧 spec 一致，但物化后仍暴露长随机引用，不能满足用户对统一短链接的要求。

### 4.3 方案 C：projected 阶段提前预留 task_slug（不采用）

这会让范围查询产生持久号段占用。规则修改、停止、时区变化或范围浏览会造成大量无实体编号，实质上破坏虚拟投影和只读查询不写库的约束。

## 5. 引用与 URL 契约

### 5.1 三种引用的职责

| 引用 | 示例 | 生命周期 | 用户用途 | 系统用途 |
|---|---|---|---|---|
| UUID | `6ed61780-...` | 物化后存在 | 兼容直接访问，不作为首选显示 | 数据库存储键 |
| task_slug | `ops-7` | 物化后存在 | 首选显示、复制、Web URL、CLI target | 项目内可读别名 |
| occurrence_ref | `occ:290d...:1783958399` | 投影时即可计算，永久稳定 | projected 实例链接；兼容旧链接 | occurrence 稳定 ID、去重键和系列槽位定位 |

结构化输出继续保持：

```json
{
  "id": "occ:290d56bd-8b9d-44e9-8b76-babbf55c62b1:1783958399",
  "uuid": "6ed61780-802f-4fec-8bf0-2e3fae977e2d",
  "task_slug": "ops-7",
  "recurrence_info": {
    "role": "occurrence",
    "series_id": "290d56bd-8b9d-44e9-8b76-babbf55c62b1",
    "series_title": "每日检查投放消耗",
    "recurrence_at": 1783958399,
    "materialization": "materialized"
  }
}
```

`id` 不因物化而变化；Web URL 可以在物化后规范化为 task_slug。文档不得再用同一个“canonical”同时描述二者：

- **稳定 ID：** occurrence_ref；
- **首选用户 permalink：** materialized 使用 task_slug，projected 使用 occurrence_ref。

### 5.2 Web 路由矩阵

| 资源 | 列表标识 | 首选地址 | 读取后规范化 |
|---|---|---|---|
| 普通项目任务 | `ops-6` | `/tasks/ops-6` | UUID 地址 replace 为 task_slug |
| 已物化循环实例 | `ops-7` | `/tasks/ops-7` | UUID/occurrence_ref 地址 replace 为 task_slug |
| projected 循环实例 | `↻07-19` | `/tasks/occ%3A...` | 保持 occurrence_ref，直到成功物化 |
| projected 首次写入后 | `ops-N` | `/tasks/ops-N` | 成功响应后 replace，不新增浏览器历史项 |

replace 只改变展示地址，不改变 React Query 中以稳定 `id` 建立的实体身份。列表选中、返回来源、滚动恢复和 Series 面板 return state 继续保存稳定 ID，不保存任意 href。

### 5.3 路径参数解码

HTTP 动态 task ref 统一经过一个 helper：

```text
Chi URLParam
  -> strings.TrimSpace
  -> url.PathUnescape，恰好一次
  -> ValidateProtocolTaskRef
  -> App resolver
```

约束：

- `%3A` 正确还原为 `:`；
- 非法百分号转义返回 `400 task_ref_invalid`；
- 禁止双重解码，`%253A` 解码一次后仍是 `%3A`，不能再次成为 occurrence_ref；
- 解码后包含 `/`、NUL、控制字符或不满足 UUID/task_slug/occurrence_ref grammar 的值返回 400；
- workspace/project scope 校验发生在解析引用之后、返回资源之前；
- 不在前端取消 `encodeURIComponent`，也不依赖代理或浏览器保留未编码冒号。

## 6. App 统一引用解析

### 6.1 原则

协议层不能再使用以下模式决定业务语义：

```text
if IsOccurrenceRef(input) { occurrence path } else { normal task path }
```

同一个已物化 occurrence 可以通过 task_slug、UUID 或 occurrence_ref 访问。根据输入语法分支会让同一对象得到不同响应、审计和操作语义。

### 6.2 解析结果

App 层提供统一、无副作用的解析结果，概念结构如下：

```text
TaskRefResolution
├─ Kind: normal | occurrence
├─ StableID: UUID | occurrence_ref
├─ UUID: null | persisted UUID
├─ TaskSlug: null | projectSlug-seq
├─ OccurrenceRef: null | occ:series:slot
├─ Materialization: normal | projected | materialized
├─ Task: null | persisted task row
└─ View: TaskOccurrenceView
```

解析规则：

1. occurrence_ref：先按 workspace + series_id + recurrence_at 查已物化行；未命中再验证 projected 槽位。
2. task_slug：按 workspace + project + seq 查 task；若 task.series_id 非空，Kind 必须是 occurrence。
3. UUID：按 workspace 查 task；若 task.series_id 非空，Kind 必须是 occurrence。
4. projected 解析只构造 view，不写任务、project sequence、audit 或 event。
5. 所有别名解析到同一 materialized occurrence 后，输出相同 recurrence_info。

### 6.3 读写分离

- `ResolveTaskRefForRead` 无副作用，可返回 projected view。
- `ResolveTaskRefForWrite` 在权限、scope、参数和动作前置条件通过后，按需在同一事务内物化 projected occurrence。
- 已物化 occurrence 无论通过哪种引用访问，都直接操作同一 task row。
- 写响应返回统一 TaskOccurrenceView；materialized occurrence 同时包含 occurrence_ref、UUID 和 task_slug。

## 7. 操作语义

### 7.1 资源类型决定动作

| 动作 | 普通任务 | occurrence（任意合法引用） |
|---|---|---|
| get | 普通任务详情 | 带 recurrence_info 的实例详情 |
| modify | 修改任务 | 只修改本次并记录字段 override |
| start/done | 开始/完成任务 | 开始本次/完成本次 |
| stop/reopen | 普通状态约束 | 停止本次/重新打开本次，遵守实例状态约束 |
| delete | 删除任务 | 跳过本次，写 recurrence skip 审计 |
| annotation/link/dependency/child | 操作任务子资源 | 只操作本次；projected 按原 spec 决定是否物化 |
| urgency/audit | 普通任务结果 | 基于实例 view；projected 读取不物化 |

### 7.2 单次 override

通过 task_slug 修改已物化 occurrence 不能绕过 override 机制。以下字段发生显式修改或清空时加入 `recurrence_overrides`：

```text
title description priority due assignees tags uda.<name>
```

后续 Series 修改共享字段时：

- projected 自动使用 Series 新值；
- open materialized occurrence 只同步未 override 的字段；
- completed/deleted occurrence 不回写；
- 通过 UUID、task_slug、occurrence_ref 修改同一实例必须得到完全相同的 override 结果。

### 7.3 删除与跳过

`DELETE /tasks/ops-7` 解析出 occurrence 后必须等价于“跳过本次”，不能只因为输入不是 occurrence_ref 就走普通 `task.delete`：

- status 变为 deleted；
- 不影响 Series 和其它槽位；
- audit action 使用 `task.recurrence.skipped`；
- payload 包含 series_id 和 recurrence_at；
- Web 文案和确认框显示“跳过本次”。

## 8. 跨协议契约

### 8.1 HTTP

以下 task ref 均合法：

```text
GET/PATCH/DELETE /api/v1/tasks/{uuid|task_slug|encoded_occurrence_ref}
POST /api/v1/tasks/{ref}/start|stop|done|reopen
GET/POST/DELETE /api/v1/tasks/{ref}/annotations|links|children|audit|urgency
```

要求：

- 三种引用解析到同一已物化 occurrence 时，响应的 task 数据、recurrence_info、权限与错误码一致；
- 响应形态由解析后的 Kind 决定，不由原始 ref 前缀决定；
- projected 仍只能通过 occurrence_ref 访问；
- 任何失败写入不得留下物化行或 project sequence；
- OpenAPI 描述明确 task_slug 是 materialized task/occurrence 的推荐短引用，occurrence_ref 是 projected 和稳定 occurrence ID。

### 8.2 MCP

现有 tool name 不变。所有 task tool 的 `id`/`task` 参数接受 UUID、task_slug 或 occurrence_ref：

```text
task_get
task_modify
task_start / task_stop / task_done / task_reopen / task_delete
task_annotate / task_denotate / task_depends
task_link_add / task_link_remove / task_link_list
urgency_explain
```

行为：

- `task_get(id="ops-7")` 必须返回 recurrence_info，而不是退化成普通 Task；
- materialized occurrence 的 human rendered text 优先显示 `ops-7`；
- structured data 保留稳定 `id=occurrence_ref`、`uuid` 和 `task_slug`；
- Agent 可优先使用 task_slug 操作 materialized occurrence；projected 必须使用 occurrence_ref；
- 同一对象通过不同别名操作产生同一审计和 ToolEnvelope data。

### 8.3 Remote/CLI

- `xuanchu ops-7 info|modify|done|delete|start|stop|reopen` 必须保留 occurrence 语义；
- human 列表的 SLUG 列显示 `ops-7`；
- projected 行显示 `↻MM-DD`，操作提示给出 occurrence_ref；
- Remote `GetTaskView("ops-7")` 解析为 occurrence DTO，不能按 JSONTask 形状丢失 recurrence_info；
- CLI JSON 与 HTTP/MCP 的稳定 id、uuid、task_slug、recurrence_info 一致；
- working set 仍只包含持久任务；projected 不获得数字 working-set ID。

## 9. Web Console 信息设计

### 9.1 列表展示与导航

```text
+--------------------------------------------------------------------------------+
| 标识      | 标题                              | 状态     | 截止       | 负责人 |
+--------------------------------------------------------------------------------+
| OPS-6     | 完成季度复盘                      | 待处理   | 07-18      | 李四   |
| OPS-7     | 每日检查投放消耗  [↻ 每天]        | 待处理   | 07-18      | 张三   |
| ↻07-19    | 每日检查投放消耗  [↻ 每天]        | 计划实例 | 07-19      | 张三   |
+--------------------------------------------------------------------------------+

OPS-6  -> /tasks/ops-6
OPS-7  -> /tasks/ops-7
↻07-19 -> /tasks/occ%3A...
```

显示和路由必须分别计算：

```text
displayRef(task): task_slug ?? projectedDateLabel ?? shortUUID
routeRef(task):
  materialized + task_slug -> task_slug
  projected occurrence     -> occurrence_ref
  ordinary fallback        -> UUID
```

禁止用一个 `task_slug || id || uuid` helper 同时承担显示和路由，否则无法正确处理 projected 与物化后的地址规范化。

### 9.2 详情页

无论入口是 `/tasks/ops-7`、UUID 还是 occurrence_ref，只要解析结果是 occurrence，详情页都使用“任务详情 + 循环上下文”的统一页面。循环实例不是另一套详情页；它复用普通任务详情的标题、描述、属性、子任务和活动结构，只增加实例身份、所属循环任务和本次操作语义。

信息优先级固定为：

```text
本次要做什么
  -> 当前状态和主操作
  -> 为什么会出现本次任务（循环上下文）
  -> 任务正文和关联资源
  -> 本次属性
  -> 子任务与活动历史
```

#### 9.2.1 已物化实例桌面原型

访问地址：

```text
/workspaces/local/projects/ops/tasks/ops-7
```

页面标题：

```text
OPS-7 · 每日检查投放消耗
```

桌面宽度下使用现有任务详情的主栏 + 280px 属性栏，不打开独立 Series 页面，也不把循环信息做成第三栏：

```text
+------------------------------------------------------------------------------------------------+
| 项目 / OPS / 任务 / OPS-7                                                        [返回项目]   |
+------------------------------------------------------------------------------------------------+
|                                                                                                |
| 每日检查投放消耗                                    [开始] [完成本次] [···]                    |
| [待处理] [中优先级] [OPS-7] [↻ 每天]                                                        |
|                                                                                                |
| +--------------------------------------------------------------------------------------------+ |
| | ↻ 循环任务 · 每天                                                                          | |
| | 本次日期：2026-07-14                                                                       | |
| | 所属循环任务：每日检查投放消耗                                      [查看循环任务 →]      | |
| | 修改当前页面只影响 2026-07-14 这一次。                                                      | |
| +--------------------------------------------------------------------------------------------+ |
|                                                                                                |
| +--------------------------------------------------------------+ +---------------------------+ |
| | 详细描述                                                     | | 本次属性                  | |
| |                                                              | |                           | |
| | 检查昨日投放消耗、异常账户和预算余额。                       | | 状态       待处理         | |
| |                                                              | | 负责人     张三           | |
| | [编辑描述]                                                   | | 优先级     中             | |
| |                                                              | | 截止日期   2026-07-14     | |
| | 关联链接                                                     | | 原循环日期 2026-07-14 只读| |
| | · 投放日报                                                   | | 标签       日报  投放     | |
| +--------------------------------------------------------------+ |                           | |
|                                                                  | 循环频率   每天     只读 | |
| +--------------------------------------------------------------+ | 所属项目   OPS            | |
| | 子任务                                                       | +---------------------------+ |
| | □ 检查异常账户                                               |                               |
| | + 新建子任务                                                 |                               |
| +--------------------------------------------------------------+                               |
|                                                                                                |
| +--------------------------------------------------------------+                               |
| | 活动                                                         |                               |
| | 张三 创建了本次任务                           07-14 00:00    |                               |
| +--------------------------------------------------------------+                               |
+------------------------------------------------------------------------------------------------+
```

布局和交互要求：

- 面包屑最后一级、slug Badge 和浏览器 URL 都显示 `OPS-7`，不显示 UUID 或 occurrence_ref；
- 标题保持现有 inline edit；实例可写时修改标题只影响本次并记录 `title` override；
- 状态、优先级、slug 和循环频率使用现有 shadcn `Badge`；slug Badge 不做第二个跳转入口；
- 循环上下文使用 shadcn `Alert`，放在标题区下方、正文上方，不放进右侧属性栏；
- Alert 固定显示循环频率、本次日期、所属循环任务和作用域提示；Series ended/stopped 时追加“所属循环任务已结束/已停止”，但不遮盖本次任务状态；
- “查看循环任务”打开任务页内 Series 管理面板，关闭后回到 `/tasks/ops-7`，保留详情滚动和焦点；
- 主操作保持在标题右侧：未完成实例显示“开始/停止”“完成本次”；更多菜单包含“跳过本次”；不得显示“完成循环任务”或“删除任务”；
- description 使用当前 MarkdownView/MarkdownEditor；链接靠近正文，子任务和活动保持普通任务详情位置；
- 右侧属性栏编辑的是本次实例。`recurrence_at` 显示为“原循环日期”且只读；due 可编辑，改期后同时显示“截止日期”和“原循环日期”；
- recurrence rule 和 Series until 只读，不在任务详情提供修改入口；需要修改未来节奏时进入“查看循环任务”；
- 页面不展示内部字段名 `series_id`、`recurrence_at`、`materialization`、UUID 或 occurrence_ref。复制调试信息属于未来开发者工具，不进入首版产品界面。

#### 9.2.2 普通任务与循环实例的差异

| 区域 | 普通任务 `/tasks/ops-6` | 循环实例 `/tasks/ops-7` |
|---|---|---|
| 面包屑/URL | OPS-6 | OPS-7 |
| 标题与正文 | 普通任务字段 | 同一套组件 |
| 顶部上下文 | 无循环 Alert | 显示本次日期、频率和所属循环任务 |
| 主动作 | 完成任务 / 重新打开任务 | 完成本次 / 重新打开本次 |
| 删除动作 | 删除任务 | 跳过本次 |
| 属性编辑 | 修改任务 | 只修改本次并记录 override |
| due | 截止日期 | 截止日期；改期后另显原循环日期 |
| 循环规则 | 不存在 | 只读摘要，通过“查看循环任务”修改未来规则 |

用户不需要先理解 Series/occurrence 数据模型。页面通过“本次日期”“只影响这一次”和动作文案说明作用域。

#### 9.2.3 projected 实例详情

尚未物化的未来实例只能通过 occurrence_ref 打开，浏览器地址暂时为：

```text
/workspaces/local/projects/ops/tasks/occ%3A290d...%3A1784390399
```

页面仍使用同一个任务详情骨架，但不伪造 slug、UUID、创建时间或活动记录：

```text
+------------------------------------------------------------------------------------------------+
| 项目 / OPS / 任务 / ↻07-19                                                       [返回项目]   |
+------------------------------------------------------------------------------------------------+
|                                                                                                |
| 每日检查投放消耗                                    [开始本次] [完成本次] [···]                |
| [计划实例] [↻ 每天]                                                                         |
|                                                                                                |
| +--------------------------------------------------------------------------------------------+ |
| | ↻ 计划于 2026-07-19                                                                        | |
| | 这是循环任务“每日检查投放消耗”的一次计划。首次编辑或执行操作后会创建本次任务。              | |
| |                                                                     [查看循环任务 →]       | |
| +--------------------------------------------------------------------------------------------+ |
|                                                                                                |
| +--------------------------------------------------------------+ +---------------------------+ |
| | 详细描述（继承自循环任务）                                   | | 本次属性                  | |
| | 检查昨日投放消耗、异常账户和预算余额。                       | | 状态       计划实例       | |
| |                                                              | | 负责人     张三（继承）   | |
| | 子任务：暂无                                                 | | 优先级     中（继承）     | |
| | 活动：本次尚无活动                                           | | 截止日期   2026-07-19     | |
| +--------------------------------------------------------------+ | 原循环日期 2026-07-19     | |
|                                                                  +---------------------------+ |
+------------------------------------------------------------------------------------------------+
```

projected 页面规则：

- 产品文案使用“计划实例”，不显示“虚拟实体”“projected”或“尚无数据库记录”；
- breadcrumb 使用 `↻MM-DD`，不截断显示 occurrence_ref；
- 继承字段可以展示和编辑。有效编辑、开始、完成、跳过、新增 annotation/link/dependency/child 时按原 spec 在同一事务内物化；
- 纯读取、取消编辑、编辑无变化、失败动作、打开/关闭面板均不物化；
- projected 没有 slug Badge、创建时间、变更历史或可删除的既有子资源；相应区域显示简短空状态，不伪造数据；
- `stop/reopen` 等不满足状态前置条件的动作不显示；
- 首次成功物化响应包含 task_slug 后，Router 使用 replace 切换为 `/tasks/ops-N`，页面内容和滚动位置不闪烁、不重挂载为另一类详情页；
- materialization 后“计划实例”Badge 变为真实状态 Badge，Alert 变为 9.2.1 的循环上下文样式。

#### 9.2.4 完成、重新打开与跳过后的页面

完成本次后停留在同一 `/tasks/ops-7`：

```text
状态 Badge：已完成
主操作：[重新打开本次]
Toast：已完成 2026年7月14日这一次
属性：只读
循环 Alert：保留
```

重新打开成功后恢复 pending 状态和可编辑属性，不改变 Series 状态或其它 occurrence。

跳过本次需要 destructive confirm dialog：

```text
标题：跳过 2026年7月14日这一次？
说明：本次任务会从待办中移除，不影响循环任务和之后的安排。
按钮：[取消] [跳过本次]
```

成功后默认返回来源任务列表，并 Toast“已跳过 2026年7月14日这一次”。如果用户通过复制的深链再次访问 deleted occurrence，则显示只读详情、`已跳过` Badge 和“查看循环任务/返回项目”，不显示执行和编辑动作。

Series 已 ended/stopped 不等于 occurrence 已完成或已跳过。既有 pending/completed occurrence 仍按自身状态提供完成或重新打开动作；Alert 只增加所属 Series 状态说明。

#### 9.2.5 URL 别名与加载状态

详情页加载流程：

```text
/tasks/ops-7
  -> GET /api/v1/tasks/ops-7
  -> recurrence_info != null
  -> 直接渲染循环实例详情，不改 URL

/tasks/{occurrence_ref 或 UUID}
  -> GET 成功且 recurrence_info != null、task_slug=ops-7
  -> 先用响应渲染同一详情
  -> Router replace /tasks/ops-7

/tasks/{projected occurrence_ref}
  -> GET 成功且 task_slug=null
  -> 保持 occurrence_ref URL
```

- 加载期间使用与普通任务详情同尺寸的 Skeleton，不能先渲染普通任务操作再切换成“本次”操作；
- 404 显示“任务不存在或你无权访问”，不泄漏 Series、项目或实例是否存在；
- replace 只在 workspace/project 与响应归属匹配、task_slug 通过 grammar 校验后执行；
- `<title>`、面包屑、复制链接和刷新后的地址都使用 replace 后的 task_slug；
- React Query cache 应以响应稳定 `id` 归并别名，避免 occurrence_ref、UUID 和 task_slug 产生三份互相过期的详情缓存。

#### 9.2.6 移动端原型

窄屏不把右侧属性栏直接堆到长页面底部。循环 Alert 始终显示在标题和操作之后，正文以下复用当前移动 Tabs：

```text
+--------------------------------------+
| OPS / 任务 / OPS-7                   |
|                                      |
| 每日检查投放消耗                     |
| [待处理] [OPS-7] [↻ 每天]           |
| [开始] [完成本次] [···]             |
|                                      |
| +----------------------------------+ |
| | ↻ 每天                           | |
| | 本次日期：2026-07-14             | |
| | 只影响这一次                     | |
| | [查看循环任务 →]                 | |
| +----------------------------------+ |
|                                      |
| [描述] [子任务] [属性] [活动]       |
| +----------------------------------+ |
| | 当前 Tab 内容                    | |
| +----------------------------------+ |
+--------------------------------------+
```

- 操作按钮可换行但不能横向溢出；“完成本次”保留完整文案，不缩成含义不明的“完成”；
- 更多操作使用 shadcn `DropdownMenu`，跳过确认使用 `Dialog`；
- Series 详情继续使用任务页内响应式 Sheet，关闭后恢复当前详情 Tab、滚动和焦点；
- recurrence Alert、Tabs、按钮和属性编辑满足键盘操作、可见焦点和 aria-label 要求。

#### 9.2.7 组件契约

详情页优先复用现有组件并收敛输入语义：

```text
TaskDetailPage
├─ TaskDetailHeader
│  ├─ Breadcrumb
│  ├─ InlineTextEditor
│  ├─ Badge group
│  └─ TaskActionBar         // 读取 resolved resource kind
├─ RecurrenceContextAlert  // recurrence_info 非空时显示
├─ TaskDescriptionBlock
├─ TaskLinksEditor
├─ SubTaskList
├─ ActivitySection
└─ TaskPropertyPanel       // occurrence 增加只读 recurrence_at
```

`TaskActionBar`、`TaskPropertyPanel` 和 URL 规范化逻辑只能读取服务端返回的 resource kind/recurrence_info，不得再次通过 `taskRef.startsWith("occ:")` 或 URL 形态猜测资源类型。

#### 9.2.8 可访问性与文案

- 主标题为页面唯一 `h1`；Alert 使用可读标题，不把所有循环信息塞进 Badge；
- 图标按钮必须有包含对象和日期的 aria-label，例如“跳过本次：2026年7月14日”；
- 状态不能只靠颜色表达；`计划实例/待处理/已完成/已跳过` 都有文字；
- 中文使用“本次”，英文统一使用 “this occurrence”，不混用 instance/run/event；
- occurrence 的 description、负责人等继承关系只在 projected 状态标注“继承”，物化后显示当前实例真实值；
- 所有文案进入 i18n，不在 action bar 或确认弹窗中硬编码中文。

核心行为不变：

- 详情页不能从 URL 形态猜测是否为 occurrence，只读取 recurrence_info；
- 所有属性编辑默认只改本次；
- occurrence_ref/UUID 深链读取到 task_slug 后使用 Router replace 规范化地址；
- projected 首次成功物化后使用响应中的 task_slug replace；
- 失败、取消或 no-op 不物化，也不改变 URL；
- Series banner、操作文案、Toast 和确认框不因引用别名变化。

#### 9.2.9 从“我的任务”进入时的详情与返回

从“我的任务”点击 materialized 或 projected occurrence 后，仍进入 §9.2 定义的同一项目级任务详情页，不创建全局循环详情路由。详情页右上返回入口显示“返回我的任务”，并保留进入前的预设、搜索、优先级和排序；打开 Series 面板、关闭面板以及首次物化后的短链接 replace 都不能丢失该来源。

```text
/my-tasks?tab=overdue&priority=H&q=review&sort=priority
  -> 点击 OPS-7 / ↻07-19
  -> /workspaces/local/projects/ops/tasks/{task_slug|occurrence_ref}
       ?from=my-tasks
       &my_tasks_search=tab%3Doverdue%26priority%3DH%26q%3Dreview%26sort%3Dpriority
  -> 渲染同一循环实例详情
       [返回我的任务]
       [查看循环任务]
       [完成本次] [···]
  -> 关闭 Series 面板：回到当前实例详情，来源参数仍在
  -> projected 首次写入：replace 到 /tasks/ops-N，来源参数仍在
  -> 返回或跳过本次：恢复 /my-tasks?tab=overdue&priority=H&q=review&sort=priority
```

来源参数属于浏览会话状态，不属于 permalink：

- 复制任务链接只复制 `/workspaces/{workspace}/projects/{project}/tasks/{task_slug|occurrence_ref}`，剥离 `from`、`my_tasks_search` 和 `panel_return_*`；
- task_slug 规范化使用 `replace` 时原样携带 `from=my-tasks` 与经过白名单校验的 `my_tasks_search`；
- “返回我的任务”和“跳过本次”只恢复 `tab/priority/q/sort` 白名单字段，未知字段不得重新注入 URL；
- Series 面板的 return state 只保存当前任务引用和 My Tasks 白名单 search，不接受任意回跳 URL；
- 直接打开复制链接时没有“返回我的任务”入口，按普通项目详情显示“返回项目”。

## 10. 统一创建与编辑表单

### 10.1 产品原则

循环任务是任务的一种创建方式。创建弹窗使用同一个 Dialog、同一套任务公共字段组件和一致的字段顺序；类型切换只改变时间/规则专属部分，不切换成风格不同的第二套表单。

### 10.2 字段归属

| 字段 | 普通任务 | 循环任务 Series | 说明 |
|---|---|---|---|
| title | 是 | 是 | 共享组件和校验 |
| description | 是 | 是 | Markdown 字符串，统一 MarkdownEditor |
| project | 是 | 是 | 项目内创建隐式提供 |
| priority | 是 | 是 | 共享组件 |
| assignees | 是 | 是 | 共享 Workspace 成员选择器 |
| tags | 是 | 是 | 共享组件 |
| UDAs | 是 | 是 | 根据 workspace schema 渲染同一高级属性区 |
| due | 是 | 否 | 普通任务截止日期 |
| recurrence_rule | 否 | 是 | 循环频率 |
| first_due | 否 | 是 | 第一次截止日期，语义对应首次 occurrence due |
| until | 失效日期 | 循环结束日期 | wire field 同名但产品语义和文案不同 |
| wait/scheduled | 是 | 否 | 单次执行控制，不属于 Series |
| depends/parent | 可用于普通任务 | 否 | occurrence 物化后才可按本次设置 |

### 10.3 创建弹窗原型

```text
+------------------------------------------------------------------+
| 新建任务                                                     [×] |
| [普通任务] [循环任务]                                            |
|                                                                  |
| 标题                                                             |
| [______________________________________________________________] |
|                                                                  |
| 详细描述                                                         |
| +--------------------------------------------------------------+ |
| | MarkdownEditor                                               | |
| |                                                              | |
| +--------------------------------------------------------------+ |
|                                                                  |
| [负责人________________] [优先级________________]                 |
| [标签__________________________________________________________] |
|                                                                  |
| 循环设置                                                         |
| [频率：每天____________] [首次截止：2026-07-14____]              |
| [循环结束：不结束______]                                         |
|                                                                  |
| 接下来三次：07-14 · 07-15 · 07-16                                |
|                                                                  |
|                                      [取消] [创建循环任务]       |
+------------------------------------------------------------------+
```

### 10.4 状态保留

- title、description、priority、assignees、tags 和 UDAs 使用一份共享 state；切换普通/循环不丢失；
- 普通任务和循环任务各自保留专属日期 state；
- 首次从普通切换到循环且 first_due 为空时，可把普通 due 复制为 first_due；后续切换不互相覆盖；
- 首次从循环切回普通且普通 due 为空时，可把 first_due 复制为 due；
- initialMode 只决定首次显示的类型，不重置另一类型已输入内容；
- 关闭成功后统一 reset；用户取消后按现有 Dialog 生命周期处理，不在类型切换时 reset。

### 10.5 description 语义

- 创建 Series 时 description 写入 task_series；
- projected occurrence 读取 Series 当前 description；
- 物化时把 description 复制到 task row；
- 修改 Series description 时，同步到 open 且 description 未 override 的 materialized occurrence；
- 在实例详情修改 description 只改本次并记录 `description` override；
- completed/deleted occurrence 不因 Series description 修改而变化；
- Web 创建和编辑 Series 都必须能填写、修改和清空 description。

### 10.6 组件复用

Web 不复制普通任务表单的组件实现。应提取或复用：

```text
TaskCommonFields
├─ TitleInput
├─ MarkdownEditor
├─ AssigneeSelector
├─ PrioritySelect
├─ TagsInput
└─ UDAFields

NormalTaskScheduleFields
└─ due / scheduled / wait / task until

RecurringTaskScheduleFields
└─ recurrence rule / first due / series until / preview
```

所有交互组件继续使用项目现有 shadcn/ui、MarkdownEditor、InlineDatePicker 和成员选择器；文案全部进入 i18n。

## 11. 数据完整性与迁移

### 11.1 新写入

以下 occurrence 创建入口都必须写完整项目绑定：

1. 创建 Series 时物化已经进入执行期的 first occurrence；
2. scheduler 或首次合法写操作物化 projected occurrence；
3. 停止 Series 并为已进入执行期但未物化槽位创建 tombstone；
4. native bundle 导入 materialized occurrence。

推荐集中为 App helper：

```text
bindOccurrenceProject(series)
  -> resolve project by workspace + project_id
  -> allocate project_seq
  -> set project/project_id/project_seq atomically
  -> validate invariant before repository Create
```

幂等冲突返回已有 occurrence 时也要验证已有行满足 invariant，不能静默继续传播坏数据。

### 11.2 现有数据回填

迁移逻辑按 `tasks.workspace_id + tasks.project_id = projects.workspace_id + projects.id` 回填：

```text
tasks.project IS NULL
AND tasks.project_id IS NOT NULL
AND tasks.project_seq IS NOT NULL
```

要求：

- SQLite 和 PostgreSQL 分别使用参数化、方言适配 SQL；
- 只从同 workspace 的 project 读取 slug；
- 找不到项目、跨 workspace 或只有部分绑定的行使启动迁移失败，并报告数量，不猜测修复；
- 回填后扫描所有 task，确保项目绑定要么全空、要么三者完整；
- 不重新分配已有 project_seq，不改变 UUID、series_id、recurrence_at 或审计历史；
- 回填属于 schema/data migration，不要求用户手工删除循环任务。

## 12. 错误、安全与兼容

### 12.1 错误码

| 场景 | HTTP | 业务码 |
|---|---:|---|
| 空引用、数字协议引用、非法转义、非法 grammar | 400 | `task_ref_invalid` |
| 合法格式但 task/occurrence 不存在 | 404 | `task_not_found` / `task_occurrence_not_found` |
| 引用存在但 scope 不允许 | 遵守现有隐藏策略 | 不泄漏资源存在性 |
| projected 动作前置条件失败 | 400/409，沿用动作错误 | 不物化 |
| 项目绑定 invariant 破坏 | 500，并记录可诊断日志 | `project_invariant_violation` |

### 12.2 安全边界

- path 参数只解码一次，避免双重编码绕过校验；
- task_slug 解析必须先限定 workspace，再解析 project 和 sequence；
- occurrence_ref 解析必须同时校验 workspace、Series project scope、规则段和有效区间；
- UUID/task_slug/occurrence_ref 别名不能绕过 project allowlist；
- 错误响应不返回其它 workspace 的 project slug、Series ID 或 task UUID；
- migration 连接项目时同时使用 workspace_id，避免错误回填跨租户 slug。

### 12.3 向后兼容

- 已复制的 occurrence_ref 和 UUID 链接继续可用；
- HTTP/MCP/CLI 继续接受 occurrence_ref；
- 结构化 `id` 继续是稳定 occurrence_ref，不改成 task_slug；
- 新 Web 链接和 human 输出优先 task_slug；
- 对普通任务现有 task_slug 行为不变；
- 不引入旧 Taskwarrior recur 字段或 JSON 兼容。

## 13. 数据流

### 13.1 已物化实例通过短链接读取

```text
用户点击 OPS-7
  -> Web /tasks/ops-7
  -> GET /api/v1/tasks/ops-7
  -> decode + validate ref
  -> App resolve task_slug
  -> task row.series_id != null
  -> Kind = occurrence
  -> 返回 TaskOccurrenceView + recurrence_info
  -> Web 显示“完成本次 / 跳过本次”
```

### 13.2 projected 实例首次完成

```text
用户打开 ↻07-19
  -> /tasks/occ%3Aseries%3Aslot
  -> GET 解码 occurrence_ref
  -> 返回 projected view，不写库
  -> 用户点击“完成本次”
  -> POST /tasks/{encoded occurrence_ref}/done
  -> 校验权限、scope、槽位和 done 前置条件
  -> 同一事务内：
       分配 project_seq
       写完整 project/project_id/project_seq
       创建 occurrence task
       完成本次
       写 audit/event
  -> 响应 id=occurrence_ref, task_slug=ops-N
  -> Web Router replace 为 /tasks/ops-N
```

### 13.3 同一实例通过不同引用操作

```text
ops-7 -------------------+
UUID --------------------+-> TaskRefResolution -> occurrence task row
occ:series:slot ----------+                         |
                                                    +-> 同一动作语义
                                                    +-> 同一 override
                                                    +-> 同一 audit/event
                                                    +-> 同一响应 view
```

## 14. 实现切片与顺序

本节只定义依赖顺序，详细文件级任务由后续 implementation plan 拆解。

1. **App resolver 与项目绑定 invariant**：先建立统一资源识别和正确物化数据。
2. **数据回填 migration**：修复已有 occurrence，保证新旧数据都可生成 task_slug。
3. **HTTP path decode 与 alias-consistent response**：解决 occurrence_ref 404 和同对象多形态问题。
4. **HTTP/MCP/CLI 动作统一**：按解析后的 Kind 分发 skip、override 和实例动作。
5. **Web displayRef/routeRef 与 URL replace**：materialized 使用短链接，projected 保留稳定引用。
6. **统一表单公共字段**：接入 description、assignees、UDAs 和共享 state。
7. **文档/OpenAPI/MCP schema/Skill 同步**：更新调用说明和示例。
8. **跨入口 E2E**：验证普通任务、projected、materialized 和别名等价性。

## 15. 测试与验收

### 15.1 App 与 Storage

- 三个运行时 occurrence 创建入口都生成完整 project/project_id/project_seq；
- materialized view 生成 `task_slug=ops-N`；
- project invariant 对部分绑定返回错误；
- UUID、task_slug、occurrence_ref 解析同一 materialized occurrence 得到相同 Kind、StableID 和 view；
- projected read 不写库；首次成功写入只分配一次 sequence；失败/no-op 不分配 sequence；
- 通过三种别名修改共享字段都记录同一 override；
- 通过 task_slug 删除 occurrence 写 recurrence skip audit；
- SQLite/PostgreSQL migration 回填正确，跨 workspace 和孤儿绑定失败。

### 15.2 HTTP

- `GET /tasks/occ%3A...` 对 projected/materialized 均返回 200；
- malformed `%`、双重编码和 encoded slash 返回 400；
- `GET /tasks/ops-7` 返回 recurrence_info；
- UUID/task_slug/occurrence_ref 三种 GET 的核心 occurrence 字段一致；
- modify/start/done/stop/reopen/delete/annotation/link/dependency/children/audit/urgency 使用 task_slug 时保留 occurrence 语义；
- projected 首次写入返回 task_slug；
- 所有失败写入无 task/audit/event/project_seq 副作用；
- project/workspace allowlist 不能通过别名绕过。

### 15.3 MCP 与 Remote/CLI

- `task_get(id=ops-7)` 返回 recurrence_info；
- task action 使用 ops-7 与 occurrence_ref 结果一致；
- `task_query` materialized occurrence 同时返回稳定 id、uuid、task_slug；
- human renderer 显示 OPS-7，projected 显示 `↻MM-DD`；
- Remote 不按 entry 时间形态或输入前缀错误降级 occurrence；
- CLI `xuanchu ops-7 done/delete/info` 使用实例文案和实例审计。

### 15.4 Web

- materialized occurrence 列表显示 OPS-7，链接 `/tasks/ops-7`；
- projected 显示 `↻MM-DD`，链接 encoded occurrence_ref；
- occurrence_ref/UUID 深链读取 materialized occurrence 后 replace 为 task_slug；
- projected 首次成功物化后 replace 为 task_slug，失败/no-op 不改变 URL；
- slug 详情按 9.2.1 原型显示 recurrence Alert、“完成本次”“跳过本次”和“查看循环任务”；
- 已改期实例在属性栏同时显示可编辑 due 和只读“原循环日期”；
- 普通任务与循环实例复用详情骨架，但动作、删除语义和作用域提示符合 9.2.2 对照表；
- projected 详情显示“计划实例”和 `↻MM-DD`，不伪造 slug、UUID、创建时间或活动记录；
- completed occurrence 停留在短链接详情并显示“重新打开本次”；deleted 深链只读且不显示执行操作；
- Series ended/stopped 时 Alert 显示归属状态，既有 occurrence 仍按自身状态操作；
- alias 加载期间不短暂渲染普通任务动作，React Query 不产生三份互相过期的实体缓存；
- 移动端循环 Alert 保持可见，描述/子任务/属性/活动 Tabs 和操作按钮无横向溢出；
- 创建/编辑循环任务存在 Markdown description 和负责人选择；
- description、负责人、优先级、标签在普通/循环切换时保留；
- first_due/due 只在首次空值切换时映射，不持续互相覆盖；
- 创建 payload 包含 description/assignees/tags/priority/UDAs；
- Series 编辑可修改和清空 description/assignees/UDAs；
- 中英文、键盘操作、移动端 Dialog/Sheet 和 shadcn 组件契约通过测试。

### 15.5 全量验证

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check

pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

### 15.6 最终验收标准

1. 项目任务列表中的已物化循环实例显示 `ops-N`，点击后地址也是 `/tasks/ops-N`。
2. 通过该地址读取详情时明确识别为循环实例，并提供“完成本次”“跳过本次”等操作。
3. UUID、task_slug、occurrence_ref 访问和操作同一已物化实例时，业务结果、权限、审计与响应语义一致。
4. projected occurrence 不提前分配 task_slug；首次成功物化后 Web 地址无刷新 replace 为短链接。
5. encoded occurrence_ref 不再返回错误 404，非法或双重编码被安全拒绝。
6. 新旧 occurrence 均满足完整项目绑定，能够稳定生成 task_slug。
7. Web 创建和编辑循环任务支持 description、负责人、优先级、标签和适用的高级属性，并与普通任务共用组件。
8. Series 共享字段更新与 occurrence 单次 override 行为正确，不因使用 task_slug 而退化。
9. HTTP、MCP、Remote/CLI 和 Web 对任务引用与循环实例语义保持一致。
10. spec、OpenAPI、MCP schema、用户文档和相关 Skill 不再描述“物化后 Web URL 必须保持 occurrence_ref”。
