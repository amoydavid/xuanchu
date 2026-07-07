# Web Console 任务详情页结构重构设计

**日期：** 2026-07-07
**状态：** 草案待审
**范围：** Workspace Web Console 的任务详情页与手动 sub-task 创建入口
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
| `blocked_by_info` | 右侧属性 | 只读展示 |
| `parent_info` | 右侧属性 | 只读展示；没有设置/清除父任务交互 |
| `links` | 左侧链接编辑器 | 添加、编辑、删除链接 |
| `annotations` | 左侧注解编辑器 | 添加、编辑、删除注解 |
| task audit | 左侧变更历史 | 字段级历史 |
| UDA | 右侧属性 | 根据类型选择文本/数字/日期/开关 |

### 2.3 当前缺口

1. 页面主区没有 sub-task 列表，也没有“添加 sub-task”的入口。
2. 标题、描述、注解、链接、变更历史之间的层级不够清楚；描述被卡片化后更像属性块，不像任务正文。
3. 右侧属性栏字段太长，所有字段同级展示，扫描成本高。
4. 链接被放在主区独立块和移动端 tab 中，但从用户心智看更接近“任务附件/关联资源”，应靠近正文动作或右侧附属信息。
5. 顶部 action bar 与右侧属性都有状态信息，但页面没有明确区分“执行动作”和“属性编辑”。
6. 后端 `addTaskRequest` / `app.AddInput` / `TaskCreateInput` 当前没有 `parent` 字段；常规 Web 创建任务无法写入手动父子关系。
7. `parent` 字段当前承担 recurring parent/child 关系；手动 sub-task 必须明确和 recurring child 的边界，避免把周期任务规则误当普通任务清单。

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

## 5. 非目标

- 不新增看板、拖拽排序、批量编辑。
- 不实现无限层级树编辑；第一阶段只在当前任务详情页展示直接子任务。
- 不新增 milestone、cycle、team、estimate 等 Linear 数据模型。
- 不把 `task` 改名为 `issue`。
- 不做实时评论或 threaded comment。
- 不改变任务 Markdown 持久化格式；正文仍保存为 Markdown 字符串。
- 不让前端绕过现有 app service 和 HTTP 授权。
- 不把 recurring parent 的自动生成规则改造成普通 checklist。

## 6. 核心决策

| 主题 | 决策 |
|---|---|
| 页面骨架 | 桌面使用左主右栏；移动端使用分段 tab 或纵向折叠，不让属性栏挤压正文 |
| 主区顺序 | 标题/正文、子任务、活动 |
| 活动定义 | 合并展示注解与字段变更历史；链接变更仍通过 audit 体现 |
| 链接位置 | 作为正文附近的“关联资源”小区块，不再和 Activity 同级抢主线 |
| 子任务展示 | 当前任务的直接 children，默认显示 open children，提供显示 completed/deleted 的入口 |
| 子任务创建 | 主区就地 composer；只要求标题，可补描述、负责人、优先级、截止日期、标签 |
| 子任务默认值 | project 继承当前任务 project；parent 写当前任务 UUID；不默认继承 depends/recur/wait/scheduled/until |
| 父子字段来源 | 复用 `parent` 字段，但在 UI 上区分手动 sub-task 与 recurring child |
| recurring 边界 | recurring parent 详情页可以查看自动 children；第一阶段不提供“添加手动子任务”按钮，避免混淆周期规则 |
| 右侧属性分组 | Properties、Schedule、Relations、System、Custom fields |
| 权限 | 前端基于 `/api/v1/me` 和 task/project 状态隐藏或禁用写入口；服务端 403 仍是最终事实 |
| API 策略 | 扩展现有 task create/modify 能力，不新增一套独立 sub-task 业务逻辑 |

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
|   [优先级 -] [截止日期] [负责人] [标签] [关联资源]                       |
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
6. parent 是 completed task 时第一阶段拒绝创建；完成任务下继续拆任务容易造成状态语义混乱。
7. parent 是 recurring parent 时拒绝手动 sub-task 创建，错误说明应直接说明“周期任务规则下不能添加手动子任务”。

### 8.3 子任务读取

直接子任务列表可以先复用现有 task query：

```text
GET /api/v1/tasks?workspace=<workspace>&project=<project>&query=parent:<uuid>&limit=50
```

如果实现中发现 query 解析和 project scope 组合无法稳定满足需求，再新增专用端点：

```text
GET /api/v1/tasks/{taskRef}/children?workspace=<workspace>&limit=50&include_closed=false
```

优先级判断：

1. 如果现有 query 路径能复用，并且测试覆盖 parent 过滤、权限和 project scope，就不新增端点。
2. 如果需要 child count、分页、include_closed 或 recurring/manual 区分，就新增 children endpoint，避免前端拼复杂 query。

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
| title | 是 | 空 | Enter 不直接提交，避免误触；Cmd/Ctrl+Enter 提交 |
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

Activity 应统一承载“人写的注解”和“系统记录的字段变更”。

```text
活动
[写注解...]

王五 添加注解 · 今天 10:30
  需要补素材截图。

Alice 修改优先级 · 昨天
  M -> H
```

当前 `TaskAnnotationsEditor` 与 `TaskChangeHistory` 可以先保持两个组件，但视觉上归入同一个 Activity 区块。后续可再合并数据源。

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
- `Schedule` 默认展开；如果全空，可折叠但保留入口。
- `Relations` 默认展开；如果没有 parent/depends/blocking/links，可用轻量空态。
- `System` 默认折叠。
- `Custom fields` 仅在存在 UDA 或有 UDA 定义时展示。

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
- `CGO_ENABLED=0 go test ./...` 继续通过。

### 11.2 前端测试

至少覆盖：

- `TaskDetailPage` 显示子任务区。
- 点击“添加子任务”展开 composer。
- 只填标题可创建，payload 包含 `project` 和 `parent`。
- 创建成功后 composer 清空并关闭，子任务列表刷新。
- completed/deleted/recurring/no-write 状态下不显示可用创建入口。
- 右侧属性分组渲染关键字段。
- 移动端 tab 能在正文、子任务、属性、活动之间切换。

### 11.3 验证命令

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

### 阶段一：结构重排，不改数据模型

- 重排 TaskDetailPage 主区和右侧栏。
- 将描述、链接、注解、变更历史组织为正文/关联资源/活动。
- 右侧属性按组展示。
- 保持现有 API，不加入子任务创建。

阶段一可以降低 UI 风险，但用户无法立即添加子任务。

### 阶段二：补手动 sub-task 闭环

- 扩展后端 create task parent 能力。
- 前端新增子任务查询和 composer。
- 补测试和 smoke。

这是推荐实施路径：先把布局变清楚，再补交互闭环；两个阶段可以在同一个 plan 中连续执行，但提交最好拆开。

### 阶段三：任务列表父子树

- 将任务列表页升级为父子层级展示。
- 保留 recurring parent 隐藏、recurring child 可见的现有语义。
- 需要 child count 或 lazy loading 时再新增后端聚合。

阶段三不属于本 spec 的第一交付，但本 spec 的 sub-task 数据形态必须为它留出口。

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
|  | David 添加注解：需要素材截图 · 9m ago                     |  | 依赖 DEM-7  | |
|  | David changed priority M -> H · 9m ago                   |  | 阻塞 DEM-40 | |
|  +----------------------------------------------------------+  +-------------+ |
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
- 当前已有信息与目标结构有明确映射。
- ASCII 原型覆盖默认态、添加态、移动端。
- 验证命令包含 Go、CGO=0、Web 和 smoke。
