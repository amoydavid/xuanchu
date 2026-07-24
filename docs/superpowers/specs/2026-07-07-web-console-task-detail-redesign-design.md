# Web Console 任务详情页结构重构设计

**日期：** 2026-07-07
**最近更新：** 2026-07-24
**状态：** 已实现（阶段一至阶段三已完成；阶段四“任务列表父子树”仍不在当前交付范围）
**范围：** Workspace Web Console 的任务详情页、手动 sub-task 创建入口与任务 Activity 语义时间线
**承接：**

- `2026-06-17-web-console-project-task-browsing-design.md`
- `2026-06-22-xuanchu-task-title-description-design.md`
- `2026-06-27-xuanchu-web-console-editing-design.md`
- `2026-07-07-web-console-project-subpages-design.md`

> 给 agentic workers 的要求：编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

## 1. 背景

当前任务详情页已经从只读浏览升级为可编辑页面，具备标题、描述、状态动作、注解、链接、变更历史和属性编辑能力。但页面结构仍然偏“功能块堆叠”：主区域先展示描述、注解、变更历史，链接单独占一个移动 tab，右侧属性栏罗列大量字段。用户可以修改字段，却不容易形成“我正在处理这个任务，它下面还有哪些可执行子项，最近发生了什么”的连续工作流。

用户提供的 Linear 参考图展示了另一种信息架构：

- 顶部是稳定的 issue 上下文：项目、编号、标题、收藏、更多、前后导航。
- 左侧主区域承载任务叙事：标题、摘要/描述、附件/反应、sub-issues 入口、Activity。
- 右侧栏承载结构化属性：Properties、Labels、Project 等可折叠分组。
- 点击 `Add sub-issues` 后，不跳转页面，而是在主叙事流里展开一个轻量 composer；它继承当前 issue 的项目/人/里程碑等上下文，并允许补标题、描述和少量关键属性。

璇础不能直接复制 Linear：

- 当前产品语义是 `task`，不是 `issue`。
- 没有 milestone、team、cycle、estimate 等 Linear 模型。
- 当前右侧属性比 Linear 多：urgency、wait、scheduled、until、recur、depends、blocking、UDA 等都是璇础/Taskwarrior 语义。
- 当前 `parent` 字段主要被 recurring child 使用；Web Console 还没有手动创建 sub-task 的 API 和交互。
- 项目子页面已经将项目 Overview、Tasks、Activity 拆开，任务详情页应沿用项目上下文，而不是再做一个孤立页面。

因此本次设计目标不是“做成 Linear”，而是借 Linear 的信息架构，把璇础已有信息重新排序，并补上手动 sub-task 的最小闭环。

### 1.1 术语与字段对照

为避免实施时中英混用和字段接反，本文统一术语（后续 i18n key、组件命名、错误文案都以此为准）：

| 本文术语 | 对应字段/接口 | 含义 | 注意 |
|---|---|---|---|
| 任务 / task | `task` | 产品统一叫“任务”，不叫 issue | 不改名为 issue |
| 子任务 | 其他任务的 `parent == 当前任务 uuid` | 当前任务的直接下级 | 详情接口**不返回** children，只能反查 |
| 父任务 | `parent` / `parent_info` | 当前任务的上级 | 详情接口已把 `parent` 展开为 `parent_info`（带 title+slug） |
| 依赖 | `depends` / `depends_info` | 当前任务依赖谁（我卡在别人上） | |
| 被阻塞/正在阻塞 | `blocked_by_info` | **反向依赖**：谁依赖当前任务（当前任务卡住了别人） | 命名易接反，UI 文案为“正在阻塞这些任务” |
| 活动 / Activity | 当前注解 + 任务 audit | 人写的注解、任务生命周期、字段和关系变化的统一产品时间线 | 不等于原始审计日志，也不读取 Hook delivery |
| 紧迫度 / urgency | 独立接口，**非详情字段** | 计算值 | 有独立加载态，不随详情同步返回 |

## 2. 现状

### 2.1 当前页面结构

当前实现入口：

- `web/src/routes/workspace/ProjectTaskDetailRoute.tsx`
- `web/src/routes/workspace/TaskDetailRoute.tsx`
- `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx`

当前桌面结构可以概括为：

```text
+----------------------------------------------------------------------------+
| workspace / project / taskRef                                               |
| title inline edit                                      [返回项目] [动作组]   |
| [status] [priority] [task_slug]                                             |
+----------------------------------------------------------------------------+
| 移动端 tabs: [属性] [注解] [链接]                                           |
+------------------------------------------------------+---------------------+
| 左侧主区                                             | 右侧属性栏          |
|                                                      |                     |
| +-- 描述 card ------------------------------------+  | 属性                |
| | markdown view + 编辑描述 dialog                 |  | - status            |
| +-------------------------------------------------+  | - urgency           |
| +-- 注解编辑器 -----------------------------------+  | - priority          |
| +-- 变更历史 -------------------------------------+  | - due               |
| +-- 链接编辑器 -----------------------------------+  | - assignee          |
|                                                      | - tags              |
|                                                      | - wait/scheduled    |
|                                                      | - until/recur       |
|                                                      | - depends           |
|                                                      | - parent/blocking   |
|                                                      | - entry/modified    |
|                                                      | - UDA               |
+------------------------------------------------------+---------------------+
```

### 2.2 当前已有信息

任务详情接口 `GET /api/v1/tasks/{taskRef}` 已返回：

| 信息 | 当前展示位置 | 说明 |
|---|---|---|
| `title` | 顶部主标题 | inline edit |
| `description` | 左侧描述 card | Markdown 展示，dialog 编辑 |
| `status` | 顶部 badge、右侧属性、动作组 | 动作走 start/stop/done/delete |
| `priority` | 顶部 badge、右侧属性 | inline select |
| `task_slug` | 顶部 badge、breadcrumb | 项目内稳定短标识 |
| `project` | breadcrumb、路由 | 详情页可从项目入口或全局任务入口进入 |
| `assignees` | 右侧属性 | picker 替换语义：clear + assignees |
| `tags` | 右侧属性 | tag picker |
| `due/wait/scheduled/until/recur` | 右侧属性 | 日期边界沿用后端规则 |
| `depends_info` | 右侧属性 | dependency picker |
| `blocked_by_info` | 右侧属性 | **反向依赖**：依赖当前任务的任务（当前任务“正在阻塞”它们），只读展示 |
| `parent_info` | 右侧属性 | 后端已把 `parent` 展开为带 title+slug 的可读引用；只读，无设置/清除父任务交互 |
| urgency | 右侧属性（`TaskUrgencyPanel`） | **不在详情接口返回**，由独立请求获取，有独立加载态 |
| `links` | 左侧链接编辑器 | 添加、编辑、删除链接 |
| `annotations` | 左侧注解编辑器 | 添加、编辑、删除注解 |
| 字段历史 | 左侧变更历史 | 阶段二专用读取只返回 `task.modify`，前端又只显示带 `changes` 的行 |
| UDA | 右侧属性 | 根据类型选择文本/数字/日期/开关 |

### 2.3 当前缺口

1. 页面主区没有 sub-task 列表，也没有“添加 sub-task”的入口。
2. 标题、描述、注解、链接、变更历史之间的层级不够清楚；描述被卡片化后更像属性块，不像任务正文。
3. 右侧属性栏字段太长，所有字段同级展示，扫描成本高。
4. 链接被放在主区独立块和移动端 tab 中，但从用户心智看更接近“任务附件/关联资源”，应靠近正文动作或右侧附属信息。
5. 顶部 action bar 与右侧属性都有状态信息，但页面没有明确区分“执行动作”和“属性编辑”。
6. 后端 `addTaskRequest` / `app.AddInput` / `TaskCreateInput` 当前没有 `parent` 字段；常规 Web 创建任务无法写入手动父子关系。
7. `parent` 字段当前承担 recurring parent/child 关系；手动 sub-task 必须明确和 recurring child 的边界，避免把周期任务规则误当普通任务清单。
8. 详情接口已填充 `parent_info`（当前任务的父），但**没有任何接口返回 children**（谁以当前任务为 parent）。子任务列表目前只能靠 `parent:<uuid>` 反查，且没有 child count、没有聚合、没有 include_closed 语义。
9. 页面标题写“活动”，实际只是注解与 `task.modify` 字段变更的上下分区，既未统一排序，也不是完整的任务事实流。
10. 创建、开始、停止、完成、重新打开等动作已经分别写入 `task.add`、`task.start`、`task.stop`、`task.done`、`task.reopen` audit，但阶段二的字段级历史读取只筛选 `task.modify`，所以详情页必然看不到这些生命周期事件。
11. Hook 层同时存在 `task.created`、`task.completed` 等语义事件，但 Hook delivery 取决于 Hook 配置和投递生命周期，不能作为用户可见历史的事实源。
12. `task_annotations` 保存注解 ID、时间和正文，但不保存作者；现有 `task.annotate` audit 又没有 `annotation_id`，无法可靠地把历史注解与 actor 关联。不能靠“同一秒”猜测作者。项目注解和任务链接已经有 `created_by` actor 列，任务注解应复用同一模式。

## 3. Linear 信息架构与交互思路

Linear 的关键不是视觉样式，而是信息分层。

### 3.1 主叙事区

左侧主区从上到下讲一条连续故事：

```text
标题
简短说明/正文
反应 / 附件
+ Add sub-issues
Activity
```

这个顺序表达的是：先理解任务，再拆解子项，再看讨论和事实流。`Add sub-issues` 放在 Activity 上方，说明子项是任务结构的一部分，不是评论里的一个事件。

### 3.2 右侧属性栏

右侧栏把结构化字段分组：

```text
Properties
  status
  priority
  assignee

Labels
  labels

Project
  project
  milestone
```

每组都有标题，可折叠。高频字段靠上，低频上下文字段靠下。右侧栏不讲故事，只让用户快速扫描和修改属性。

### 3.3 就地创建 sub-issue

点击 `Add sub-issues` 后，Linear 在主区当前位置展开 composer：

```text
+ Add sub-issues
+------------------------------------------------------+
| ○ Issue title                                        |
| Add description...                                   |
|                                                      |
| Quick suggestions  [assignee] [milestone]            |
| [team/project] [priority] [assignee] [label] [...]   |
|                                      [Cancel] [Create]|
+------------------------------------------------------+
```

这个交互有三个优点：

1. 不打断阅读上下文。
2. 子项天然继承当前 issue 的项目上下文。
3. 默认只要求标题，其他字段逐步补充。

## 4. 目标

1. 将任务详情页重构为“主叙事区 + 右侧属性栏”的清晰结构。
2. 主叙事区按 `标题/正文 -> 子任务 -> 活动` 的顺序组织。
3. 右侧属性栏按高频属性、计划字段、关系字段、系统字段、自定义字段分组，可扫描、可折叠。
4. 增加手动 sub-task 创建能力：在任务详情页点击“添加子任务”后就地展开 composer。
5. 新建 sub-task 默认继承当前任务的 project，写入当前任务 UUID 作为 `parent`。
6. 子任务创建成功后刷新当前任务详情、子任务列表、项目任务列表和项目摘要。
7. 保持现有权限、workspace/project scope、closed project 禁写、completed/deleted task 禁写规则。
8. 保持中文为主的 UI 文案和注释；英文 locale 同步补齐。
9. 不破坏 recurring task 现有语义：周期规则生成的 children 继续可见，但手动 sub-task 和 recurring child 在 UI 上要有明确提示。
10. 将任务 Activity 收口为真正的语义时间线：至少覆盖创建、开始、停止、完成、重新打开、字段变更、链接变更和当前注解。
11. Activity 由 app 层输出稳定的语义结构；Web 不理解内部 audit action，不拼接原始 payload，不生成无法本地化的后端文案。
12. Activity 使用同一排序和分页契约，桌面与移动端读取同一数据源。

## 5. 非目标

- 不新增看板、拖拽排序、批量编辑。
- 不实现无限层级树编辑；第一阶段只在当前任务详情页展示直接子任务。
- 不新增 milestone、cycle、team、estimate 等 Linear 数据模型。
- 不把 `task` 改名为 `issue`。
- 不做实时评论或 threaded comment。
- 不改变任务 Markdown 持久化格式；正文仍保存为 Markdown 字符串。
- 不让前端绕过现有 app service 和 HTTP 授权。
- 不把 recurring parent 的自动生成规则改造成普通 checklist。
- 不把通用 Audit Console 嵌入任务详情页，也不向普通 task reader 暴露原始 audit payload。
- 不把 Hook definition、Hook delivery、通知投递或自动化执行记录混进任务 Activity；它们属于运维/集成可观测性。
- 不为 Activity 新建第二套 append-only 事件表；已有 `audit_logs` 继续承载系统动作事实，`task_annotations` 继续承载当前可见注解正文。
- 不在本阶段实现 threaded comment、reaction、@mention 通知或注解修订历史。

## 6. 核心决策

| 主题 | 决策 |
|---|---|
| 页面骨架 | 桌面使用左主右栏；移动端使用分段 tab 或纵向折叠，不让属性栏挤压正文 |
| 主区顺序 | 标题/正文、子任务、活动 |
| 活动定义 | 合并当前注解、任务生命周期、字段变更与链接变更；输出产品语义，不直接暴露 audit action |
| 活动事实源 | 生命周期/字段/关系事件来自 `audit_logs`；注解正文来自 `task_annotations`；Hook delivery 不参与 |
| 活动接口 | task-read 专用 `GET /api/v1/tasks/{ref}/activity` 是任务详情历史的唯一读取接口；字段级任务审计端点已在确认无消费者后移除 |
| 活动排序 | `occurred_at DESC`，同秒使用稳定 source key 倒序；使用 opaque cursor，不使用会漂移的 offset 合并分页 |
| 注解去重 | 当前注解只渲染一次；`task.annotate` / `task.annotation.update` / `task.denotate` audit 不再另渲染一条重复事件 |
| 活动视觉 | 单列纵向 timeline；左侧小圆点由细线连接，右侧展示 actor、动作、时间和详情；不用互相割裂的卡片列表 |
| 链接位置 | 作为正文附近的“关联资源”小区块，不再和 Activity 同级抢主线 |
| 子任务展示 | 当前任务的直接 children，默认显示 open children，提供显示 completed/deleted 的入口 |
| 子任务创建 | 主区就地 composer；只要求标题，可补描述、负责人、优先级、截止日期、标签；**Enter 直接提交并保持 composer 打开**，支持连续快速拆多条 |
| 子任务默认值 | project 继承当前任务 project；parent 写当前任务 UUID；不默认继承 depends/recur/wait/scheduled/until |
| 父子字段来源 | 复用 `parent` 字段，但在 UI 上区分手动 sub-task 与 recurring child |
| recurring 边界 | recurring parent 详情页可以查看自动 children；第一阶段不提供“添加手动子任务”按钮，避免混淆周期规则 |
| 右侧属性分组 | Properties、Schedule、Relations、System、Custom fields |
| 权限 | 前端基于 `/api/v1/me` 和 task/project 状态隐藏或禁用写入口；服务端 403 仍是最终事实 |
| 子任务创建 API | 扩展现有 task create 能力补 `parent`，不新增一套独立 sub-task 业务逻辑 |
| 子任务读取 API | 新增专用 `GET /tasks/{ref}/children` 端点（而非前端拼 `parent:<uuid>` query），因带 query 后默认状态过滤失效且需 child count / include_closed |
| 阶段顺序 | 阶段一至三已经完成；后续若继续推进，则进入不属于当前交付的任务列表父子树 |

## 7. 信息架构设计

### 7.1 桌面骨架

```text
+--------------------------------------------------------------------------------+
| 项目 / 任务标识 / 标题                                         [复制] [...] [←] |
+--------------------------------------------------------------------------------+
|                                                                                |
| +-- 主叙事区 ------------------------------------------------+  +-- 右侧栏 ----+ |
| | DEM-23 写投放日报                                        |  | Properties   | |
| | [状态 badge] [优先级 badge] [负责人头像]                  |  | status       | |
| |                                                          |  | priority     | |
| | 正文                                                     |  | assignee     | |
| | -------------------------------------------------------- |  | tags         | |
| | 素材、预算、验收标准...                                  |  +--------------+ |
| |                                      [编辑正文] [链接]    |  | Schedule     | |
| |                                                          |  | due          | |
| | 子任务                                                   |  | wait         | |
| | [ + 添加子任务 ]                                         |  | scheduled    | |
| | ○ DEM-24 拉取素材数据              H   张三   今天        |  | until/recur  | |
| | ✓ DEM-25 对齐投放口径              L   李四   昨天        |  +--------------+ |
| |                                                          |  | Relations    | |
| | 活动                                                     |  | parent       | |
| | [写注解...]                                              |  | depends      | |
| | 王五 添加注解：需要补截图 · 10:30                         |  | blocking     | |
| | Alice changed priority from M to H · 昨天                |  +--------------+ |
| +----------------------------------------------------------+  | System       | |
|                                                               | entry        | |
|                                                               | modified     | |
|                                                               +--------------+ |
+--------------------------------------------------------------------------------+
```

### 7.2 添加子任务展开态

```text
子任务
[ + 添加子任务 ]
+--------------------------------------------------------------------------+
| ○ [ 子任务标题                                                       ]   |
|   [ 补充背景、验收标准或处理说明...                                  ]   |
|                                                                          |
|   建议：继承当前项目 project-x                                           |
|   [优先级 -] [截止日期] [负责人] [标签]                                  |
|                                                                          |
|                                             [取消] [创建子任务]          |
+--------------------------------------------------------------------------+
```

创建成功后：

```text
子任务
[ + 添加子任务 ]
○ DEM-31 子任务标题                         -    未分配    -
○ DEM-24 拉取素材数据                       H    张三      今天
✓ DEM-25 对齐投放口径                       L    李四      昨天
```

### 7.3 移动端骨架

```text
+------------------------------------------+
| 项目 / DEM-23                            |
| 写投放日报                               |
| [开始] [完成] [...]                      |
+------------------------------------------+
| [正文] [子任务] [属性] [活动]             |
+------------------------------------------+
| 当前 tab 内容                             |
|                                          |
| 正文 tab: Markdown + 关联资源             |
| 子任务 tab: 子任务列表 + 添加 composer     |
| 属性 tab: 分组属性                         |
| 活动 tab: 注解 composer + 历史             |
+------------------------------------------+
```

移动端默认打开“正文”。从项目任务列表进入时，用户最需要先理解任务内容；属性可以通过 tab 进入。

## 8. 子任务模型与 API

### 8.1 数据模型

第一阶段复用任务现有 `parent` 字段表示父任务 UUID。

手动 sub-task 满足：

- `parent == 当前任务 uuid`
- `project == 当前任务 project`
- `recur == nil`
- 创建来源是用户显式创建，不是 recurring engine 自动生成

Recurring child 满足：

- `parent == recurring parent uuid`
- 通常继承 parent 的 `recur`
- 由 `createNextRecurringChild` 生成

UI 必须避免把 recurring parent 的“下一次任务”误导为普通手动子任务。因此：

- 非 recurring 普通任务：显示“子任务”，允许添加。
- recurring parent：显示“自动生成任务”，第一阶段不允许添加手动子任务。
- recurring child：如果它自身是普通 pending/waiting 任务，可以显示自己的手动子任务入口；但不自动继承 recurring parent 的规则。

### 8.2 后端能力补齐

当前常规 HTTP create path 不支持 parent：

- `internal/httpapi/tasks.go` 的 `addTaskRequest` 没有 `Parent`。
- `internal/app/service.go` 的 `AddInput` 没有 `Parent`。
- `web/src/features/workspace/project-workbench/api/task-api.ts` 的 `TaskCreateInput` 没有 `parent`。

需要补齐：

```go
type AddInput struct {
    // ...
    Parent *string
}
```

```go
type addTaskRequest struct {
    // ...
    Parent string `json:"parent,omitempty"`
}
```

```ts
export type TaskCreateInput = {
  // ...
  parent?: string
}
```

创建时规则：

1. `parent` 可接受 UUID 或当前 workspace 内可解析 task ref；app 层解析为父任务 UUID。
2. parent 必须属于同一 workspace。
3. 如果子任务指定 project，必须与父任务 project 一致；如果未指定 project，则继承父任务 project。
4. parent 不能是自身，不能形成环。
5. parent 是 deleted task 时拒绝创建。
6. parent 是 completed task 时第一阶段拒绝创建。从工程看，完成任务下继续拆任务容易造成状态语义混乱；但从使用者看，“完成后发现遗漏想补挂收尾子项”是真实场景，直接拒绝会让人困惑。因此错误文案不能只说“不允许”，必须给出面向使用者的替代动作，例如“请先重新打开父任务再添加子任务”。（若后续用户反馈强烈，可在后续阶段重新评估是否允许，因子任务自身是 pending，不直接改变父状态。）
7. parent 是 recurring parent 时拒绝手动 sub-task 创建，错误说明应直接说明“周期任务规则下不能添加手动子任务”。

### 8.3 子任务读取

查询引擎已支持按 `parent` 过滤（`AttrParent`，底层 `compareColumn("parent", ...)`），因此理论上可以：

```text
GET /api/v1/tasks?workspace=<workspace>&project=<project>&query=parent:<uuid>&limit=50
```

但有一个关键陷阱必须写明：**带了 `query` 后，默认 pending 状态过滤不再生效**（见 `app.Service.List`：仅当 `input.Query == nil` 才默认 pending）。因此 `parent:<uuid>` 会返回该 parent 下 **pending/waiting/completed/deleted 全部**子任务；“默认只显示 open children”必须靠前端拿全量后自行分组/过滤，或在查询里显式带 status。这直接影响 §9.3 的“折叠已完成/隐藏已删除”能否靠一次查询实现。

**决策：直接新增专用 children 端点**，不让前端拼复杂 query：

```text
GET /api/v1/tasks/{taskRef}/children?workspace=<workspace>&limit=50&include_closed=false
```

理由（符合 AGENTS.md“选对未来更稳的边界”）：

1. `parent:<uuid>` 的状态语义反直觉（见上），若靠查询字符串表达，会把业务规则（默认只显示 open）散落到前端。
2. 子任务区需要 child count、include_closed、manual/recurring 区分，前端拼 query 几乎注定返工。
3. 阶段四要做父子树，专用端点可以一次把 include_closed、count、manual/recurring 区分的出口留好。

若实现时决定先用 query 快速跑通，必须在测试里固定“返回全状态”这个行为，并在前端明确按状态分组，避免默认就漏显已删除子任务。

## 9. 交互细节

### 9.1 子任务入口

显示条件：

- 用户有 task write 权限。
- 当前项目未 closed。
- 当前任务状态不是 completed/deleted。
- 当前任务不是 recurring parent。

无权限或不可写时：

- 已有子任务仍可读。
- “添加子任务”按钮隐藏或 disabled；若 disabled，hover/tooltip 说明真实原因。

### 9.2 Composer 字段

第一阶段字段：

| 字段 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| title | 是 | 空 | **Enter 直接提交并保持 composer 打开**（支持连续快速拆多个子任务，对齐 Linear 心智）；只有多行 description 编辑器里才用 Cmd/Ctrl+Enter |
| description | 否 | 空 | MarkdownEditor，和任务创建 dialog 保持一致 |
| priority | 否 | 空 | H/M/L |
| due | 否 | 空 | date-only 按本地日末 |
| assignees | 否 | 空 | 复用 AssigneePicker |
| tags | 否 | 空 | 复用 TagPicker 或轻量 tag input |

不在第一阶段放入 composer 的字段：

- depends：子任务依赖需要搜索与环检测，放到创建后右侧属性编辑。
- wait/scheduled/until/recur：低频且语义复杂，创建后编辑。
- UDA：字段多且依赖定义，创建后编辑。
- links：可作为创建后正文附近的关联资源添加；composer 里不放复杂链接表单。

### 9.3 子任务列表行

子任务行用于扫描，不替代完整详情页。

```text
○ DEM-31 子任务标题                         H    张三      今天
```

行内展示：

- status icon：pending/waiting/completed/deleted 映射为清晰状态。
- task_slug 或 UUID 短码。
- title。
- priority。
- assignee label。
- due。

交互：

- 点击标题进入子任务详情页。
- completed 子任务可折叠到“显示已完成”里。
- deleted 子任务默认隐藏，除非用户显式显示 closed。
- 不做行内编辑；详情页和任务列表已经承担编辑。

### 9.4 活动区

原“注解组件在上、字段变更组件在下”的过渡实现已经退役。阶段三交付的 Activity 是一条统一、可分页、按时间交错的产品时间线，不再由两个组件共用标题伪装成统一活动。

```text
活动
[写注解...]

●  李四 完成了任务 · 今天 11:02
│
●  王五 添加注解 · 今天 10:30
│  需要补素材截图。
│
●  Alice 修改了优先级 · 昨天 18:40
│  M -> H
│
●  Alice 创建了任务 · 昨天 18:20
```

#### 9.4.1 产品语义

Activity 回答的是“这个任务发生了什么”，不是“数据库里写了哪些审计 action”。后端必须把内部 action 映射为稳定的 Activity action，前端只根据 `kind/action` 和结构化详情进行 i18n 渲染。

| Activity `kind` | Activity `action` | 事实来源 | 默认展示 |
|---|---|---|---|
| `lifecycle` | `created` | `task.add` | “创建了任务” |
| `lifecycle` | `generated` | `task.recurrence.generated` | “生成了本次任务” |
| `lifecycle` | `started` | `task.start` | “开始了任务” |
| `lifecycle` | `stopped` | `task.stop` | “停止了任务” |
| `lifecycle` | `completed` | `task.done` | “完成了任务” |
| `lifecycle` | `reopened` | `task.reopen` | “重新打开了任务” |
| `lifecycle` | `deleted` | `task.delete` | “删除了任务”；仅在该任务仍可被授权读取时出现 |
| `change` | `fields_changed` | 带非空 `changes` 的 `task.modify` | 一条 event 内按稳定顺序展示本次所有字段变化 |
| `relation` | `link_added` | `task.link.add` | “添加了链接”，详情包含结构化链接摘要 |
| `relation` | `link_updated` | `task.link.update` | “更新了链接” |
| `relation` | `link_removed` | `task.link.remove` | “移除了链接” |
| `annotation` | `commented` | 当前 `task_annotations` 行 | 展示当前注解正文 |

阶段三首版不展示以下记录：

- 空 `changes` 的 `task.modify`，因为没有可解释内容。
- `task.annotate`、`task.annotation.update`、`task.denotate` 的 audit 行，因为当前注解已经作为 `annotation/commented` 条目展示，重复展示会造成“一次操作两条活动”。
- 仅供兼容的 `task.append`、`task.prepend`、`task.edit` 若没有结构化字段 diff，不能用含糊的“更新了任务”占位。后续若要纳入，应先让这些写路径生成同一套 `changes`。
- Hook delivery、通知 delivery、自动化 job 状态和通用审计管理动作。

`task.add`、`task.done` 等是内部 audit action；`created`、`completed` 等是 Activity 公共语义。HTTP 响应和 Web i18n 不得直接依赖 audit action 名称。

#### 9.4.2 App 读模型与 HTTP 契约

新增 app 层读模型，名称可按实现风格微调，但字段语义必须保持：

```go
type TaskActivityEntry struct {
    ID         string         // audit:<id> | annotation:<uuid> | snapshot:created
    Kind       string         // lifecycle | change | relation | annotation
    Action     string         // created | completed | fields_changed | commented ...
    Actor      task.ActorInfo // user/token/system/unknown；HTTP 转 JSONActorInfo
    OccurredAt int64
    Changes    []TaskFieldChange
    Annotation *TaskActivityAnnotation
    Link       *TaskActivityLink
}

type TaskActivityPage struct {
    Entries    []TaskActivityEntry
    NextCursor *string
}
```

HTTP：

```text
GET /api/v1/tasks/{taskRef}/activity?workspace=<workspace>&limit=30&cursor=<opaque>
```

```json
{
  "data": {
    "entries": [
      {
        "id": "audit:42",
        "kind": "lifecycle",
        "action": "completed",
        "actor": {
          "type": "user",
          "user": {
            "id": "user-1",
            "name": "lisi",
            "display_name": "李四",
            "email": "lisi@example.com",
            "external_ids": []
          }
        },
        "occurred_at": "2026-07-24T11:02:00Z"
      }
    ],
    "next_cursor": null
  }
}
```

约束：

- 入口只要求 `task:read` / `PermissionTaskRead`，与任务详情一致；不要求普通成员持有 `audit:read`。
- App 先按当前 workspace 解析 task ref，再以 `workspace_id + target_type=task + target_id` 查询，不能接受前端传任意 target ID 绕过 scope。
- `actor` 统一使用 `task.ActorInfo` / `task.JSONActorInfo`。自然人必须输出完整 `task.UserInfo`，字段为 `id/name/display_name/email/external_ids`，不能输出裸 UUID。
- `tenant_access_token` 等非自然人 actor 使用 `type + token`；内部生成且没有自然人主体的事件使用 `type=system`；历史数据无法确认主体时使用 `type=unknown`，不能拿当前用户或任务负责人代替。
- 后端不返回已拼好的人类句子。时间格式、字段名、状态和空值文案由 Web i18n 渲染。
- `limit` 默认 30、最大 100。cursor 是服务端不透明值，至少编码最后一条的 `occurred_at + source_rank + source_id`；客户端不得解析或自行构造，非法或不兼容 cursor 返回稳定的 `api_bad_cursor`。

`TaskDetailPage` 只请求 `/api/v1/tasks/{ref}/activity`，不再并行读取另一套字段历史并在前端二次拼接。确认 Web Console 和其他仓库入口没有消费者后，阶段三收尾已删除原字段级任务审计 route、App 入口、TypeScript fetch/query 及其 OpenAPI 文档；通用 `/api/v1/audit` 继续保留。

#### 9.4.3 事实源、注解作者与兼容

系统事件继续复用 `audit_logs`。不新增 `task_activity_entries`，也不从 hook delivery 反推历史。

当前注解正文继续以 `task_annotations` 为准：已删除注解不应因为 audit payload 仍存在而重新显示正文。任务注解的作者元数据直接落在注解行，复用 `ProjectAnnotation` / `TaskLink` 已有 actor 列模式：

```text
created_by_actor_type
created_by_user_id
created_by_token_id
created_by_token_name
created_by_token_prefix
created_at
```

约束：

- `created_by_actor_type` 支持现有 user / tenant token 类型，并新增历史兼容用 `unknown`；不能把空 user ID 序列化成伪造的自然人。
- 新增注解时，app 在同一事务里把 `RuntimeContext.actorColumns()` 写入注解行；编辑注解不能改变原作者。
- SQLite 与 PostgreSQL 都需要显式迁移。历史行回填 `created_by_actor_type=unknown`、`created_at=entry`，不能默认成当前用户或 workspace 创建者。
- Activity 读取注解行上的 actor 列并批量 `resolveUserInfos`，不扫描 JSON payload 反查作者。
- Taskwarrior 兼容的 `task.JSONAnnotation` 继续保持 `id/entry/description`，不把 actor 元数据塞进 Taskwarrior JSON。Activity HTTP 使用自己的 `actor` 字段；独立 annotations HTTP 若同步扩展，也必须使用 `task.JSONActorInfo`。
- Task bundle/import 若没有可信作者来源，导入的注解使用 `unknown`；由当前写操作直接创建的注解使用真实 runtime actor。不能把“执行导入的人”冒充为原注解作者。

同时让新写入的 `task.annotate` audit 与项目注解保持一致，补充可追踪的 `annotation_id`：

```json
{
  "annotation_id": "annotation-uuid",
  "before_project_id": "...",
  "after_project_id": "..."
}
```

- `task.annotate` 必须在同一事务里写入注解和包含 `annotation_id` 的 audit；不能提交注解后再补 audit。
- Activity actor 以注解行元数据为准；`annotation_id` 用于审计追踪，不要求 Activity 解析 payload 才能显示作者。
- Activity 不把 `task.annotate` audit 另渲染为第二条活动。
- `task.annotation.update` 与 `task.denotate` 继续写 audit，但阶段三不展示注解编辑/删除历史。
- 历史注解仍要展示，actor 返回 `type=unknown`。禁止按同一秒、相同正文或数组位置猜作者。
- `AddWithAnnotations`、模板实例化或导入路径若创建注解，也必须遵守同一 actor 写入契约。

创建事件的兼容规则：

- 有 `task.add` 时映射为 `created`。
- recurrence occurrence 有 `task.recurrence.generated` 时映射为 `generated`，不再额外伪造 `created`。
- 历史任务既没有 `task.add` 也没有 `task.recurrence.generated` 时，从任务 `entry` 合成 `snapshot:created`，actor 为 `unknown`，保证时间线有真实创建时间但不伪造创建人。
- 不从当前 `status/end` 合成完成或重新打开事件；快照无法还原多次完成/重开历史，猜测会制造假事实。

#### 9.4.4 排序、分页与去重

统一顺序为：

```text
occurred_at DESC, source_rank DESC, source_id DESC
```

- `occurred_at`：audit 使用 `created_at`，annotation 使用新增的 `created_at`（历史 fallback 为 `entry`），snapshot creation 使用 task `entry`。
- `source_rank` 是服务端固定的同秒排序值，只用于结果稳定，不表达业务优先级；取值一旦发布不可随意调整。
- `source_id`：audit 使用自增 ID，annotation 使用 UUID，snapshot 使用固定 key。
- 下一页严格查询“小于 cursor 排序键”的记录，避免新活动插入后出现 offset 重复或漏项。
- App 层可以分别从 audit 与 annotation repository 取 `limit + 1` 条候选后归并；不能把两个已经各自 offset 的页面简单拼接，因为那会导致跨数据源漏项。
- `task.modify` 一次操作即一条 Activity entry，即使包含多个 field changes；不要拆成多条后让一次保存刷屏。
- 当前注解、对应 `task.annotate` audit 只产生一个 `annotation/commented` entry。
- 前端以 `entry.id` 作为稳定 key，不用数组 index、时间戳或正文作为 key。

#### 9.4.5 纵向时间线视觉

`TaskActivityTimeline` 使用单列纵向布局。每个条目分成固定宽度的轨道列和自适应内容列：

```text
轨道列    内容列

  ●       李四 完成了任务 · 今天 11:02
  │
  ●       王五 添加注解 · 今天 10:30
  │       需要补素材截图。
  │
  ●       Alice 修改了优先级 · 昨天 18:40
  │       M -> H
  │
  ●       Alice 创建了任务 · 昨天 18:20
```

视觉契约：

- 使用 `<ol>` / `<li>` 表达有序活动列表，DOM 顺序与接口一致：最新在上、最旧在下。
- 轨道列建议宽 16px；节点是 6–8px 实心圆点，连接线是 1px 细线。具体尺寸可以随现有 design token 微调，但节点必须清楚落在线上，不能像项目符号列表一样与线脱节。
- 连接线位于节点后方并贯穿相邻条目。首条不向节点上方冒线；最后一条在没有下一页时不向节点下方拖尾。
- 当 `next_cursor != nil` 时，最后节点下方保留一小段向下延伸并渐隐的线，表达时间线仍可继续；“加载更多”与内容列左边缘对齐，不把按钮塞进圆点位置。
- 节点和连线是装饰元素，设置 `aria-hidden="true"`；事件含义必须由正文表达，不能只靠圆点颜色或形状区分。
- 首版所有节点使用中性前景/边框 token，避免把 Activity 做成彩色状态灯。若后续需要强调 `completed` 等里程碑，只能作为辅助视觉，文本仍是权威语义。
- 条目正文不套独立大卡片、不加整行边框。actor、动作、时间组成首行，结构化 changes、注解 Markdown 或链接摘要紧随其下；相邻条目靠 16–20px 纵向间距和时间线分隔。
- actor 与动作使用正常正文色；时间使用弱化色。桌面尽量同一行，窄屏允许时间自然换行，但不能挤压或覆盖轨道列。
- 注解的编辑/删除入口在该条目 hover/focus 时出现，预留固定操作空间，避免按钮出现导致正文或圆点横向跳动。
- composer 位于 timeline 上方，不分配圆点，也不接入竖线；草稿尚未成为活动事实。
- 空态不显示孤立圆点或竖线。加载态使用带节点和短线的 timeline skeleton，避免数据回来时布局跳变。
- 桌面和移动端使用同一结构；移动端仅压缩轨道宽度和内容间距，不退化成卡片列表。

#### 9.4.6 UI 行为

- 桌面和移动端共用 `TaskActivityTimeline`，移动端“活动”tab 不实现另一套映射。
- composer 位于时间线顶部。新增、编辑、删除注解成功后使 activity query 失效并刷新；任务 start/stop/done/reopen/modify/link mutation 同样刷新 activity query。
- 初始加载使用骨架；翻页使用“加载更多”，不得清空已展示活动。
- 局部失败显示“活动暂不可用”与重试入口，不阻塞任务标题、正文和属性读取。
- 空态统一为“暂无活动”；不可继续显示“暂无字段级变更记录”，因为 Activity 已不再等于字段变更。
- actor 展示顺序为 `display_name -> name -> id`；token 展示 token name；`system` 显示“系统”，`unknown` 显示“未知主体”。
- 生命周期事件使用直接文案，例如“完成了任务”，不用“状态从 pending 修改为 completed”替代。
- 字段变化继续展示结构化 before/after；长 description 变化沿用可展开查看，不在主时间线上铺满正文。

### 9.5 右侧属性分组

右侧栏建议分组：

```text
Properties
  status
  urgency
  priority
  assignees
  tags

Schedule
  due
  wait
  scheduled
  until
  recur

Relations
  parent
  depends
  blocking
  links

System
  task_slug
  entry
  modified

Custom fields
  UDA...
```

分组规则：

- `Properties` 默认展开。
- `Schedule` 默认展开；**如果全空，整组不渲染标题**（可写状态下仍要有一个轻量入口让用户新增计划字段），不要保留空壳分组制造扫描噪音。
- `Relations` 默认展开；如果没有 parent/depends/blocking/links，用轻量空态或整组隐身。
- `System` 默认折叠。
- `Custom fields` 仅在存在 UDA 或有 UDA 定义时展示。

原则：右侧栏的价值是「一眼看到有值的属性」。空分组应隐身而非折叠占位；仅在用户有写权限、需要引导新增时，才保留一个可添加入口。

### 9.6 空态与边界态

对新用户，空态就是首屏，必须显式定义，不能留白：

| 区域 | 空态 | 加载态 | 失败态 |
|---|---|---|---|
| 子任务区 | 一句引导文案 + 一个明显的「添加子任务」按钮（可写时），而非空白；不可写时显示「暂无子任务」 | 骨架行占位 | 行内错误 + 重试入口，不用全局 toast 作为唯一反馈 |
| 活动区 | 「暂无活动」+ 注解 composer（可写时） | 统一时间线骨架；加载更多不清空已有条目 | 行内“活动暂不可用”+ 重试 |
| 正文 | 「添加描述…」可点击引导（沿用现状） | — | — |
| urgency | 独立加载态（因是独立请求），加载中显示占位而非闪烁 | 独立 skeleton | 失败静默降级，不阻塞其他属性 |

原则：

- 可写用户的空态要「引导下一步动作」，不可写用户的空态只需「说明当前无内容」。
- 子任务/活动的加载失败是局部失败，不应让整页详情不可用。

## 10. 权限与错误

前端粗判断：

```text
canCreateSubTask =
  canTaskWrite
  && !projectClosed
  && currentTask.status not in [completed, deleted]
  && currentTask.status != recurring
```

服务端最终校验：

- workspace scope。
- project scope。
- task write permission。
- 父任务可写。
- 子任务 project 与父任务 project 一致。
- parent 不形成环。
- recurring parent 不允许手动创建普通子任务。

错误展示：

- composer 提交失败时保留草稿。
- 字段级错误放在 composer 下方，不用全局 toast 作为唯一反馈。
- 403 / `token_scope_denied` / `permission_denied` / `project_scope_denied` 使用现有错误码直译，并附一句真实原因。

## 11. 测试与验收

### 11.1 后端测试

至少覆盖：

- `app.Service.Add` 支持 parent，创建后 `Info` 返回 `parent`。
- parent ref 可用 UUID 和 task_slug 解析。
- parent 不存在、跨 workspace、自引用、环、deleted parent、completed parent、recurring parent 均拒绝。
- 子任务默认继承父任务 project。
- 显式 project 与父任务 project 不一致时拒绝。
- HTTP `POST /api/v1/tasks` 可提交 `parent` 并返回 JSON。
- `GET /api/v1/tasks/{ref}/children` 返回直接子任务；`include_closed=false` 时默认隐藏 completed/deleted，`true` 时全量返回。
- children 端点遵守 workspace/project scope 与 task:read 权限。
- （若改用 `parent:<uuid>` query 方案）固定验证带 query 后默认返回全状态子任务的行为。
- `CGO_ENABLED=0 go test ./...` 继续通过。

### 11.2 前端测试

至少覆盖：

- `TaskDetailPage` 显示子任务区。
- 点击“添加子任务”展开 composer。
- 只填标题可创建，payload 包含 `project` 和 `parent`。
- **Enter 提交后 composer 清空但保持打开**（支持连续创建），子任务列表刷新；点取消或失焦才关闭。
- completed/deleted/recurring/no-write 状态下不显示可用创建入口。
- 子任务区零子任务时展示引导空态（可写）或“暂无子任务”（不可写）。
- 子任务列表默认隐藏 deleted、折叠 completed，提供“显示已完成”入口。
- 右侧属性分组渲染关键字段，空分组隐身。
- 移动端 tab 能在正文、子任务、属性、活动之间切换。

### 11.3 Activity 后端测试

至少覆盖：

- 创建任务后 Activity 包含唯一 `created`；完成后在其上方出现 `completed`。
- start/stop/reopen 映射为稳定语义 action，不暴露 `task.start` 等 audit action。
- 一次 `task.modify` 含多个 changes 时只返回一条 `fields_changed`。
- 空 changes 的 modify、annotate/update/denotate audit 不单独出现。
- 当前注解只出现一条；新注解从注解行解析完整 actor UserInfo，`task.annotate` audit 不重复出现。
- 历史注解迁移后仍展示正文，actor 为 unknown、created_at fallback 为 entry；不得猜测当前用户。
- 注解 actor migration 在 SQLite/PostgreSQL 都正确回填；Taskwarrior JSON annotation 形状保持兼容。
- 删除注解后 Activity 不再泄露已删除正文。
- 普通 task reader 可读取 Activity，但无 `audit:read` 时仍不能读取通用 Audit Console。
- workspace/project scope 不能通过 task UUID、cursor 或 annotation ID 绕过。
- 同秒多条 audit/annotation 顺序稳定；翻页无重复、无漏项。
- 历史任务缺少创建 audit 时只合成一条 `snapshot:created`；不从 completed 快照伪造完成事件。
- SQLite 与 PostgreSQL 查询均使用参数绑定，继续满足 `CGO_ENABLED=0`。

### 11.4 Activity 前端测试

至少覆盖：

- 创建、完成、重开、字段变化、链接变化和注解按 `occurred_at` 交错展示。
- 生命周期文案使用 Activity action i18n，不渲染内部 audit action。
- actor 按 display_name/name/id fallback；system、unknown、token 文案正确。
- 一次多字段修改显示为一个条目；description 可展开。
- Activity 使用语义化纵向 timeline：有序列表、装饰性节点/连线不进入无障碍名称，桌面和移动端不退化成独立卡片列表。
- 首条上方无线、末条在无下一页时下方无线；有下一页时显示延伸线，“加载更多”与内容列对齐。
- composer、空态不显示伪造的 timeline 节点；加载态保持轨道与内容列布局稳定。
- 新增/编辑/删除注解和任务动作成功后刷新 activity query。
- “加载更多”追加条目并保留已有列表；next_cursor 为空时隐藏入口。
- Activity 局部失败不影响详情其他区域；空态为“暂无活动”。

### 11.5 验证命令

实现完成前不要声称完成。至少运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
git diff --check
```

## 12. 分阶段建议

> 划分原则（CIO/CPO 视角）：**先给用户能立刻感知价值的能力，再做低价值高回归风险的纯视觉重排**。纯结构重排对用户几乎无感，却是回归风险最高的动作，因此不前置。

### 阶段一：手动 sub-task 最小闭环（先给能力）

- 扩展后端 create task parent 能力（`AddInput` / `addTaskRequest` / `TaskCreateInput` 补 parent）。
- 新增 children 读取端点（见 §8.3）。
- 前端在现有布局中新增子任务列表与就地 composer（Enter 连续创建）。
- 补后端约束测试（自引用/环/跨 workspace/deleted/completed/recurring parent）、前端交互测试与 smoke。

阶段一交付后，用户立刻能拆任务，获得可感知价值；且不动大 UI，回归面小。

### 阶段二：结构重排与 Activity 视觉归并（已完成）

- 重排 TaskDetailPage 主区和右侧栏为「正文/关联资源/子任务/活动」。
- 右侧属性按 Properties/Schedule/Relations/System/Custom fields 分组，空组隐身。
- Activity 只完成“共用标题和区域”的视觉归并，仍是两个数据源上下排列；阶段三负责完成 §9.4 的语义合并契约。
- 纯视觉改动，风险集中、可灰度、可回退。

说明：阶段一、二可在同一个 plan 中连续执行，但提交必须拆开——能力闭环与视觉重排分别可独立回退。

### 阶段三：任务 Activity 语义时间线（已完成）

- 新增 `TaskActivityEntry` app 读模型和 task-read 专用 `/api/v1/tasks/{ref}/activity` HTTP 端点。
- 扩展 audit repository 的安全 action/cursor 查询，复用 actor 批量解析。
- 为任务注解补齐与项目注解/任务链接一致的 actor 持久化字段，并让新注解 audit payload 带 `annotation_id`。
- 将 lifecycle、field change、link relation 和当前 annotation 映射为稳定 Activity action。
- 前端以统一的纵向 `TaskActivityTimeline` 替代 `TaskAnnotationsEditor + TaskChangeHistory` 的上下拼接，左侧圆点以细线连接，右侧承载事件内容。
- 补齐 cursor 分页、权限、跨 workspace、历史 fallback、删除注解不泄露正文和 Web mutation 刷新测试。

阶段三已经按独立 implementation plan 完成：详情页只读取 `/api/v1/tasks/{ref}/activity`，当前注解、生命周期、字段和链接事件按统一游标排序；无人使用的字段级任务审计端点及 Web 兼容代码已删除。Web 使用 `<ol>/<li>` 单列纵向时间线，左侧圆点和细线均为装饰元素，composer、空态与错误态不伪造节点。

实现验证补充：SQLite 旧注解迁移、零 CGO 查询和全量测试已通过；PostgreSQL schema/query 条件测试已加入 `internal/storage/postgres_test.go`，但本次环境未配置 `XUANCHU_TEST_DB_URL`，因此按仓库惯例 skip，不能视为真实 PostgreSQL 实测结果。

### 阶段四：任务列表父子树

- 将任务列表页升级为父子层级展示。
- 保留 recurring parent 隐藏、recurring child 可见的现有语义。
- 需要 child count 或 lazy loading 时再新增后端聚合。

阶段四不属于本 spec 的当前交付，但阶段一的 children 端点与 sub-task 数据形态必须为它留出口。

## 13. ASCII 终态原型

```text
+--------------------------------------------------------------------------------+
| Projects / 广告投放自动化 / DEM-23 写投放日报                  [复制] [...] [→] |
+--------------------------------------------------------------------------------+
|                                                                                |
|  +-- Main ----------------------------------------------------+  +-- Rail -----+ |
|  | 写投放日报                                                |  | Properties  | |
|  | [待处理] [H] [DEM-23]                                     |  | 状态 待处理 | |
|  |                                                          |  | 紧迫度 8.2  | |
|  | 整理素材表现，补齐预算与验收截图。                       |  | 优先级 H    | |
|  |                                           [编辑] [添加链接] |  | 负责人 张三 | |
|  |                                                          |  | 标签 ads    | |
|  | 子任务                                                   |  +-------------+ |
|  | [ + 添加子任务 ]                                         |  | Schedule    | |
|  | ○ DEM-31 拉取素材数据           H   张三   今天           |  | 截止 今天   | |
|  | ○ DEM-32 核对投放口径           -   未分配 -              |  | 暂缓 -      | |
|  | [显示 1 个已完成]                                        |  | 计划 -      | |
|  |                                                          |  +-------------+ |
|  | 活动                                                     |  | Relations   | |
|  | [写注解...]                                              |  | 父任务 -    | |
|  | ● 李四 完成了任务 · 2m ago                                |  | 依赖 DEM-7  | |
|  | │                                                        |  | 阻塞 DEM-40 | |
|  | ● David 添加注解 · 9m ago                                |  +-------------+ |
|  | │  需要素材截图                                          |                  |
|  | │                                                        |                  |
|  | ● David 修改了优先级 · 12m ago                           |                  |
|  | │  M -> H                                                |                  |
|  | │                                                        |                  |
|  | ● Alice 创建了任务 · 1d ago                              |                  |
|  +----------------------------------------------------------+                  |
|                                                                                |
+--------------------------------------------------------------------------------+
```

添加子任务：

```text
| 子任务                                                                       |
| [ + 添加子任务 ]                                                             |
| +------------------------------------------------------------------------+   |
| | ○ [核对落地页转化口径                                             ]    |   |
| |   [补充背景、验收标准或处理说明...                                  ]    |   |
| |                                                                        |   |
| |   继承项目：广告投放自动化                                             |   |
| |   [优先级 -] [截止日期] [负责人] [标签]                                |   |
| |                                             [取消] [创建子任务]        |   |
| +------------------------------------------------------------------------+   |
```

## 14. 自审清单

- 本规格不引入 Linear 独有模型。
- 子任务创建走现有 app/service/API 分层，不在前端伪造关系。
- 手动 sub-task 和 recurring child 的边界已写明。
- 当前已有信息与目标结构有明确映射，且 urgency/blocked_by/parent_info 等字段的真实语义已校对代码。
- 术语对照表统一中英文与字段含义，避免实施时接反。
- children 读取已明确为专用端点，并写明 `parent:<uuid>` query 的状态陷阱。
- 空态/加载态/失败态已定义（§9.6）。
- 分阶段以“先能力后重排”为序，降低回归风险。
- Activity 已明确为产品语义时间线，而不是 `task.modify` 别名或原始审计日志。
- Activity 事实源、action 映射、完整 actor、注解 actor migration、去重、历史 fallback、cursor 排序与权限边界已定义。
- Activity 视觉明确为带圆点和连接线的单列纵向 timeline，并覆盖首尾线段、分页延伸、移动端与无障碍规则。
- 创建和完成等生命周期事件有明确映射；Hook delivery 被排除，不会因 Hook 配置影响用户历史。
- ASCII 原型覆盖默认态、添加态、移动端。
- 验证命令包含 Go、CGO=0、Web 和 smoke。
