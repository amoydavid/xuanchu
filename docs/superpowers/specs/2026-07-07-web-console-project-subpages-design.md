# Web Console 项目子页面信息架构设计

**日期：** 2026-07-07
**状态：** 已完成
**范围：** `/workspaces/:workspaceSlug/projects/:projectSlug` 项目详情与项目级子页面

## 1. 背景

当前项目详情页已经具备项目编辑、状态切换、配置概览、任务筛选、任务表格、负责人负载和最近动态等能力，但这些信息都挤在同一个页面里。用户进入 `/workspaces/local/projects/ops` 时，看到的是一个混合工作台，而不是清晰回答不同问题的项目空间。

Linear 的启发不是照搬 `Overview / Activity / Issues` 这几个词，而是它的信息架构：同一个项目上下文在右侧保持稳定，左侧根据用户正在查看的信息主体切换。对璇础来说，左侧主体应该围绕项目态势、任务执行、项目事实流三类问题展开。

璇础当前数据结构也决定了不能盲目复制 Linear：

- `Project` 当前没有负责人、项目成员、里程碑模型。
- 任务是核心执行对象，当前产品语义是 `task`，不是 `issue`。
- 项目级 annotation 已经存在，且 `ProjectTimeline` 已能合并 project annotation 与 task annotation。
- 项目配置 effective value 是璇础区别于普通项目管理工具的重要信息，在项目页应作为项目附属信息展示，未来也会作为 LLM 上下文使用。

## 2. 目标

1. 将当前项目单页拆成项目级子页面：`Overview`、`Tasks`、`Activity`。
2. 保持项目头部和右侧项目信息栏在子页面之间一致，减少上下文切换成本。
3. 让 `Overview` 回答“项目现在怎么样，下一步该看哪里”，而不是继续展示完整任务表。
4. 让 `Tasks` 成为任务执行和筛选主页面，承接当前任务工具栏、任务表格、导入、新建能力。
5. 让 `Activity` 成为项目更新、任务注解、项目相关审计的时间线页面。
6. 第一阶段优先复用现有 API 和数据结构，不新增 project lead、project members、milestones 等模型。
7. 页面上每个数字、姓名、状态、列表项都必须能追溯到明确接口字段或前端派生规则；没有数据来源的内容不展示。
8. 明确 ProjectSummary 的时间口径、权限口径和 secret 配置展示规则，避免 Overview 泄露或误判全量项目状态。

## 3. 非目标

- 不把 `task` 改名为 `issue`。
- 不新增项目负责人、项目成员、里程碑数据表。
- 不做看板、拖拽排序、批量编辑。
- 不在没有 `audit:read` 权限时展示审计记录。
- 不把项目配置编辑表单放进 Overview；Overview 只展示项目附属信息的 label/value，编辑仍进入设置页。
- 不改变 CLI、MCP、HTTP API 的身份输出规范。
- 不新增 Activity 评论、线程、回复模型；第一阶段只有 project annotation、task annotation 和有权限可见的 audit。

## 4. 核心决策

| 主题 | 决策 |
|---|---|
| 默认项目页 | `/workspaces/:workspaceSlug/projects/:projectSlug` 显示 Overview |
| 任务执行页 | 新增 `/workspaces/:workspaceSlug/projects/:projectSlug/tasks` |
| 活动页 | 新增 `/workspaces/:workspaceSlug/projects/:projectSlug/activity` |
| 设置页 | 继续使用现有 `/projects/:projectSlug/settings/*`，不放入主 tab |
| 右侧栏 | 新增共享 `ProjectContextRail`，所有项目子页面复用 |
| 右侧栏开合 | 右侧栏默认打开，用户可收起；展开/收起使用图标按钮，收起后只保留展开图标按钮 |
| Notes 归属 | 项目 notes/annotations 从 settings 迁到 Activity |
| 第一阶段数据策略 | 前端组合项目、配置、timeline 等现有端点；Overview/右栏的全量任务摘要必须新增后端 ProjectSummary API |
| ProjectSummary 权限 | 同时要求项目可读和任务可读，并遵守 workspace/project scope；不能向无任务读取权限的用户泄露任务数量或任务标识 |
| ProjectSummary 时间口径 | 使用后端统一任务日期边界：date-only `due/until` 按本地日末，`wait/scheduled` 按本地日初；不能由浏览器时区重新解释 |
| 任务列表形态 | 第一阶段 Tasks 页使用简单列表，不做状态分组、父子树、看板或泳道 |
| 后续分组策略 | 任务分组有多种路径，需另起设计：按状态、负责人、优先级、标签、父子层级或自定义查询 |
| Activity 数据源 | 列表以 project timeline 为主；project annotations 端点只用于发布、删除和兼容读取，避免同一条项目更新重复展示 |

## 5. 信息架构

### 5.1 路由结构

```text
/workspaces/:workspaceSlug/projects/:projectSlug
  项目 Overview，保留当前短链接，适合作为分享入口。

/workspaces/:workspaceSlug/projects/:projectSlug/tasks
  项目任务页，承接任务筛选、任务表格、新建、导入、负责人负载。

/workspaces/:workspaceSlug/projects/:projectSlug/activity
  项目活动页，展示项目更新、任务注解、项目相关审计。

/workspaces/:workspaceSlug/projects/:projectSlug/tasks/:taskRef
  现有任务详情页，保持兼容。

/projects/:projectSlug/settings/config
/projects/:projectSlug/settings/definitions
  现有项目配置与配置定义管理页，保持管理入口。
```

### 5.2 页面骨架

```text
+------------------------------------------------------------------------------+
| 工作区 / 项目代号                                 [复制] [状态] [设置图标] |
| 项目名称                                                                     |
| 项目描述                                                                     |
+------------------------------------------------------------------------------+
| [概览] [任务] [活动]                                           [收起右栏图标] |
+-----------------------------------------------+------------------------------+
| 左侧：当前子页面主体                          | 右侧：项目信息栏             |
|                                               |                              |
| 概览页：项目态势、重点风险、附属信息、摘要     | 属性                         |
| 任务页：筛选、简单任务列表、任务操作           | 进度                         |
| 活动页：更新输入框、时间线、审计              | 附属信息                     |
|                                               | 负责人负载                   |
|                                               | 最近活动                     |
+-----------------------------------------------+------------------------------+
```

右侧栏收起后：

```text
+------------------------------------------------------------------------------+
| 工作区 / 项目代号                                 [复制] [状态] [设置图标] |
+------------------------------------------------------------------------------+
| [概览] [任务] [活动]                                           [展开右栏图标] |
+------------------------------------------------------------------------------+
| 左侧：当前子页面主体，占满主要宽度                                           |
|                                                                              |
| 概览 / 任务 / 活动内容                                                       |
+------------------------------------------------------------------------------+
```

## 6. 共享区域

### 6.1 Project Header

```text
工作区 / 项目代号                                      [复制] [状态] [设置图标]
项目名称
项目描述
```

展示内容：

| 区块 | 数据来源 | 展示原因 |
|---|---|---|
| 工作区 | route params 的 `workspaceSlug` | 明确当前 workspace 上下文，避免跨 workspace 操作误判 |
| 项目代号 | route params 的 `projectSlug`，并由 `GET /api/v1/projects/{ref}` 校验存在性 | 明确当前 project 上下文 |
| 项目名称、描述 | `GET /api/v1/projects/{ref}` 返回的 `name`、`description` | 项目主身份，支持 inline 编辑 |
| 状态菜单 | `GET /api/v1/projects/{ref}` 返回的 `status`；写入走 project transition API | 状态是项目级决策，应在所有子页可见 |
| 复制链接 | 当前浏览器 URL | 项目页是协作入口，需要稳定分享 |
| 设置入口 | 当前登录态/令牌的 `project:manage` 能力 + 现有 project settings route | 配置编辑属于管理面，保持齿轮入口 |

导入任务和新建任务是 Tasks 页主动作，不放在所有子页面共享 Header 中。这样 Overview 和 Activity 的主语仍然是项目态势和事实流，不被任务执行动作抢占。

### 6.2 Project Tabs

```text
[概览] [任务] [活动]
```

展示内容：

| Tab | 对应路由 | 回答的问题 |
|---|---|---|
| 概览 | `/workspaces/:workspaceSlug/projects/:projectSlug` | 项目现在怎么样，是否需要介入 |
| 任务 | `/workspaces/:workspaceSlug/projects/:projectSlug/tasks` | 具体任务是什么，如何推进 |
| 活动 | `/workspaces/:workspaceSlug/projects/:projectSlug/activity` | 最近发生了什么，谁记录或修改了什么 |

### 6.3 Project Context Rail

右侧栏在所有项目子页面保持一致。它不是第二个详情页，而是“当前项目的固定上下文”。右侧栏默认打开，用户可以收起。展开/收起使用图标按钮，建议使用 `PanelRightCloseIcon` / `PanelRightOpenIcon`；按钮必须有 `aria-label` 和 `title`，不使用冗长文字按钮。收起后，页面右侧只保留一个窄按钮用于展开，左侧主体自动占满剩余宽度。第一阶段不要求跨浏览器持久化开合状态；若使用 localStorage，key 使用 workspace + project 维度，避免污染其他项目页面。

窄屏规则：在移动端或主内容宽度不足以同时容纳列表和右栏时，右侧栏默认收起；展开时可以作为抽屉或置于主内容下方，但不能挤压任务表格导致列内容重叠。

```text
+-- 项目信息 --------------------+
| 状态          active           |
| 任务          38 总数 / 22 待办 |
| 创建          2026-07-01       |
| 更新          2026-07-07       |
+-- 进度 ------------------------+
| 已完成        #####----- 42%    |
| 逾期          2                |
| 高优未完成    3                |
+-- 附属信息 --------------------+
| 模型供应商    openai           |
| 回调密钥      缺少必填         |
| Agent 模式    supervised       |
+-- 负责人负载 ------------------+
| 未分配        5 待办 / 2 逾期  |
| 张三          8 待办 / 1 高优  |
| 李四          4 待办           |
+-- 最近活动 --------------------+
| 王五发布项目更新               |
| DEM-12 线上回调地址已确认      |
| 项目记录：下周补验收报告       |
+--------------------------------+
```

数据来源与展示原因：

| 数据点 | 数据来源 | 派生规则 | 为什么在右栏展示 |
|---|---|---|---|
| 状态 | `GET /api/v1/projects/{ref}` 的 `status` | 直接展示 | 项目状态跨子页都需要可见 |
| 任务总数 | `GET /api/v1/projects/{ref}` 的 `task_count` | 直接展示 | 给出项目规模 |
| 待办任务数 | `GET /api/v1/projects/{ref}` 的 `pending_count` | 第一阶段按后端字段直接展示为 pending 计数；不把 waiting/recurring 编进待办，除非后端新增明确聚合 | 避免用不完整前端列表编造全量未完成 |
| 创建时间 | `GET /api/v1/projects/{ref}` 的 `created_at` | 前端按 locale 格式化 | 判断项目历史长度 |
| 更新时间 | `GET /api/v1/projects/{ref}` 的 `modified_at` | 前端按 locale 格式化 | 判断项目最近是否活跃 |
| 完成进度 | `task_count`、`completed_count` | `task_count == 0` 时显示 `0%`；否则 `completed_count / task_count` | 快速判断项目推进状态 |
| 逾期任务数 | 新增 `GET /api/v1/projects/{ref}/task-summary` 或等价 ProjectSummary API | 后端按项目全量任务聚合：`due < now` 且状态非 completed/deleted | Overview 和右栏展示的是项目整体情况，不能用当前列表或过滤结果代替 |
| 高优未完成 | 同上 | 后端按项目全量任务聚合：`priority == H` 且状态非 completed/deleted | 提供全局风险提示 |
| 等待已到期 | 同上 | 后端按项目全量任务聚合：`wait <= now` 且状态非 completed/deleted；date-only wait 按本地日初解释 | 提示已经可以开始处理的等待任务 |
| 附属信息 label/value | `GET /api/v1/projects/{ref}/config/effective?console_home=true` | 使用 `definition.label || key` 作为 label，使用 effective `value` 作为 value；secret 值展示后端脱敏结果或“已设置”，缺必填展示“缺少必填”；`source` 仅作为次要提示，不作为主标题 | 这些字段是项目附属信息和未来 LLM 上下文，不应以“运行配置”作为 UI 分类 |
| 负责人负载 | 新增 `GET /api/v1/projects/{ref}/task-summary` 或等价 ProjectSummary API | 后端按项目全量任务 assignees 聚合；空 assignees 归入“未分配任务” | 当前没有项目成员模型，任务负载比成员列表更真实 |
| 最近活动 | `GET /api/v1/projects/{ref}/timeline?limit=5` | 按 `entry` 倒序显示最近 5 条；审计不并入右栏第一版 | 给用户持续的项目活态信号 |

禁止展示的数据点：

- 不展示项目负责人、项目成员、里程碑、Slack/IM 频道、客户请求等字段，除非后续 schema 明确提供。
- 不展示“进展正常 / 有风险”等项目健康状态，除非它来自项目 annotation 内容或后续新增的结构化项目更新字段。
- 不把当前任务列表、当前筛选结果或分页结果派生出的数字表述为全量项目统计。

## 7. Overview 子页面

### 7.1 目标

Overview 是项目判断页，不是完整任务表。它应该让用户在 30 秒内知道：

- 最近一次项目级判断是什么。
- 当前最需要处理的风险是什么。
- 项目附属信息是否缺失，是否会影响后续 Agent 或 LLM 上下文。
- 任务负载是否集中或有大量未分配。
- 最近有没有关键变化。

### 7.2 ASCII 原型

```text
+-- 概览 ------------------------------------------------------------+
| 最新项目更新                                                       |
| 王五 · 2 小时前                                                    |
| 本周目标：完成远程 token 配置和 MCP 示例验证。                     |
|                                                        [写项目更新] |
+--------------------------------------------------------------------+
| 当前重点                                                           |
| 逾期任务        2      DEM-7、DEM-9                         [查看] |
| 高优未完成      3      DEM-2、DEM-4、DEM-8                  [查看] |
| 等待已到期      5      DEM-5、DEM-10、DEM-11                [查看] |
| 未分配任务      4      DEM-6、DEM-13、DEM-15                [查看] |
+--------------------------------------------------------------------+
| 模型供应商        openai                                            |
| 回调密钥          缺少必填                                          |
| Agent 模式        supervised                                        |
|                                                        [管理配置]   |
+--------------------------------------------------------------------+
| 负责人负载                                                         |
| 未分配      ####----  5 待办 / 2 逾期                              |
| 张三        ########  8 待办 / 1 高优                              |
| 李四        ###-----  3 待办                                       |
+--------------------------------------------------------------------+
| 最近活动                                                           |
| 王五发布项目更新 · 2 小时前                                        |
| DEM-12 线上回调地址已确认 · 今天                                   |
| 项目记录：下周补验收报告 · 昨天                                    |
|                                                        [查看全部]   |
+--------------------------------------------------------------------+
```

### 7.3 区块说明

| 区块 | 展示内容 | 数据来源 | 为什么放在这里 |
|---|---|---|---|
| 最新项目更新 | 最近一条 project annotation 的 `content`、`created_by`、`created_at/entry` | 优先使用 `ProjectView.recent_annotations`；为空时不展示该块，或显示“暂无项目更新” | 项目首页需要人类可读判断；不展示结构化健康状态，避免编造 |
| 当前重点 | 项目全量逾期、高优未完成、等待已到期、未分配任务，以及每类最多 3 个任务短标识 | 新增 `GET /api/v1/projects/{ref}/task-summary` 或等价 ProjectSummary API；后端通过 storage 聚合，不能使用当前任务列表派生 | Overview 回答项目整体情况；点击“查看”带筛选条件跳转 Tasks 页 |
| 项目附属信息 | console-home effective config 的 label/value；可用次要样式显示 source 或缺必填；secret 值只显示脱敏或“已设置” | `listProjectEffectiveConfig(projectRef, { consoleHome: true })`，后端已对 `definition.secret` 值脱敏 | 这些字段是项目附属信息和未来 LLM 上下文，UI 不使用“运行配置”标题，也不暴露 secret |
| 负责人负载 | 项目全量任务中未分配、每个负责人待办/逾期/高优 | 新增 `GET /api/v1/projects/{ref}/task-summary` 或等价 ProjectSummary API | 当前没有项目成员模型，任务负载是更真实的协作信号 |
| 最近活动 | timeline 最近 3 到 5 条的 `source_type`、`source_label`、`content`、`entry` | `GET /api/v1/projects/{ref}/timeline?limit=5` | 给 Overview 提供项目活态，但不替代 Activity 页 |

当前重点的“查看”跳转只负责带用户进入 Tasks 页对应筛选视角，不承诺与 ProjectSummary 的全量计数完全一一对应。若现有任务查询表达式无法精确表达 ProjectSummary 的后端规则，Tasks 页应显示筛选条件本身，不显示“共 N 条与 Overview 完全一致”这类未经同源计算的文案。

## 8. Tasks 子页面

### 8.1 目标

Tasks 是项目执行页，承接当前项目工作台的大部分操作能力。第一阶段只做简单任务列表，不做分组、父子树、看板或泳道。分组有多种合理路径，应该在后续单独设计，而不是在本 spec 中提前定死。

### 8.2 ASCII 原型

```text
+-- 任务 ------------------------------------------------------------+
| [搜索标题或内容        ] [状态] [优先级] [负责人] [标签] [日期] [+] [导入图标] |
| 当前筛选：状态=未完成  负责人=我                         [清除全部] |
+--------------------------------------------------------------------+
| 标识      标题                         状态      优先级  负责人  截止 |
| DEM-1     父任务标题                   未完成    高优    张三    周五 |
| DEM-3     子任务标题                   未完成    中优    李四    周五 |
| DEM-4     子任务标题                   未完成    -       未分配  下周 |
| DEM-5     独立任务标题                 未完成    -       王五    -    |
| DEM-8     等外部回调确认               等待中    低优    张三    -    |
| DEM-2     已完成任务                   已完成    -       李四    昨天 |
+--------------------------------------------------------------------+
```

### 8.3 区块说明

| 区块 | 展示内容 | 数据来源 | 为什么放在这里 |
|---|---|---|---|
| 筛选工具栏 | 搜索、状态、优先级、负责人、标签、日期、更多筛选 | URL search + `ProjectTaskToolbar` | 任务页的首要工作是缩小任务集合 |
| 新建任务 | 使用 `PlusIcon` 图标按钮打开 `TaskCreateDialog`，按钮带 `aria-label=新建任务` | 现有 task create API | 用户在查看任务时最可能创建同项目任务 |
| 导入任务 | 使用 `UploadIcon` 图标按钮打开 `TaskImportDialog`，按钮带 `aria-label=导入任务` | 现有导入能力 | 批量导入属于任务页主动作，避免放进全局项目 Header |
| 任务列表 | 简单表格列表：标识、标题、状态、优先级、负责人、截止日期、操作 | `GET /api/v1/tasks?project=...` 返回的任务字段 | 第一阶段先保持清晰、可落地；不提前绑定某一种分组策略 |
| 行内编辑 | 标题、优先级、截止日期、状态动作 | 现有 task mutation hooks | 当前项目工作台已证明编辑面应在 workbench 内 |
| 负责人负载 | 可留在右栏或页面下方补充 | 当前任务集合派生 | 任务页需要快速判断任务是否堆在人或未分配上 |

### 8.4 后续分组与层级规则

第一阶段不做分组。后续如果要做分组或层级，必须先单独更新 spec，因为分组至少有以下路径：

- 按状态分组：适合执行状态扫描。
- 按负责人分组：适合负载均衡。
- 按优先级分组：适合风险处理。
- 按标签分组：适合业务域或模块视角。
- 按父子层级展示：适合拆解结构，但需要处理 child count、筛选命中子任务、recurring parent 隐藏语义。
- 按自定义查询或保存视图分组：适合高级用户，但需要明确配置模型。

如果后续选择父子层级方向，必须遵守：

- 不在当前简单列表实现里偷偷加入分组。
- 后端任务列表应返回 `child_count`，或者提供按父 UUID 懒加载子任务的端点。
- recurring 父任务继续保持现有隐藏语义，不为了树结构强制显示 recurring parent。
- 筛选命中子任务但父任务不在当前结果内时，应明确展示父任务引用来源，而不是伪造父行。

## 9. Activity 子页面

### 9.1 目标

Activity 是项目事实流。它应该合并项目更新、任务注解和有权限可见的项目审计，让用户看到“项目发生了什么”。

### 9.2 ASCII 原型

```text
+-- 活动 ------------------------------------------------------------+
| 写项目更新，记录风险、决策、进展...                                |
|                                                [发布更新]           |
+--------------------------------------------------------------------+
| 筛选：[全部] [项目更新] [任务注解] [审计]                           |
+--------------------------------------------------------------------+
| 2026-07-07                                                         |
| * 项目更新 · 王五 · 10:22                                          |
|   完成 token mcp-config 验证，下一步补远程 CLI 示例。              |
|                                                                    |
| * 任务注解 · DEM-12 · 09:40                                        |
|   线上回调地址已确认。                                             |
|                                                                    |
| * 审计 · task.modify · 张三 · 09:15                                |
|   截止日期：2026-07-07 -> 2026-07-08                               |
+--------------------------------------------------------------------+
| 2026-07-06                                                         |
| * 审计 · project.transition · 系统 · 18:30                          |
|   planning -> active                                               |
|                                                        [加载更多]   |
+--------------------------------------------------------------------+
```

### 9.3 区块说明

| 区块 | 展示内容 | 数据来源 | 为什么放在这里 |
|---|---|---|---|
| 发布项目更新 | 多行文本输入，保存为 project annotation | `POST /api/v1/projects/{ref}/annotations` | 项目级 annotation 原本就是项目记录，不应藏在 settings |
| 项目更新时间线 | project annotation | 以 `GET /api/v1/projects/{ref}/timeline` 中 `source_type=project` 为列表来源；project entry 的 `source_id` 必须是 annotation id，`source_label` 是项目标识；annotations 端点只用于发布、删除和兼容读取 | 展示人类写下的项目判断、风险、决策，并避免 timeline 与 annotations 双源重复 |
| 任务注解 | task annotation | `GET /api/v1/projects/{ref}/timeline` 中 `source_type=task` | 任务执行过程中的重要事实也属于项目上下文 |
| 审计记录 | project-scoped audit | `GET /api/v1/audit?project=<ref>&limit=...`，仅有 `audit:read` 时请求 | 字段变更是事实流的一部分，但权限高于普通项目读取 |
| 时间分组 | 按本地日期分组 | 前端格式化 `entry` / `created_at` | 时间线需要按天扫描，减少长列表噪音 |

Activity 每条记录的数据点约束：

- project annotation 只展示 timeline 返回的 `content`、`created_by`、`entry`、`source_id`、`source_label`；其中 `source_id` 是 annotation id。
- task annotation 只展示 timeline 返回的 `source_label`、`content`、`entry`、`source_id`，不展示不存在的任务状态或负责人。
- audit 只展示 audit row 的 `action`、`actor`、`created_at`、`changes` 或 `payload` 中已有信息；不能把 payload 解释成没有来源的自然语言结论。

Activity 去重规则：

- 首屏列表以 project timeline 为主数据源。
- 发布项目更新成功后，刷新 project timeline 和 `ProjectView.recent_annotations`。
- 如果实现中为了删除能力额外读取 annotations，只能用于删除按钮状态和兼容，不把 annotations 列表再拼到 timeline 上。
- 审计记录与 timeline 合并时以 `kind + id/action + entry` 作为前端 key；不能因为 audit 与 annotation 时间接近就合并成一条没有来源的自然语言总结。

### 9.4 权限规则

- 有 `project:manage` 且项目未关闭时，显示发布项目更新输入框。
- 只有 `project:read` 时，可以读取 project timeline，但不能发布更新。
- 有 `audit:read` 时，Activity 合并项目审计；没有该权限时，不显示审计筛选项，不请求 audit 端点。
- archived/cancelled 项目允许读取 Activity，不允许新增项目更新。

## 10. Settings 调整

现有项目设置页继续承载配置管理：

```text
/projects/:projectSlug/settings/config
/projects/:projectSlug/settings/definitions
```

建议移除或重定向：

```text
/projects/:projectSlug/settings/notes
  -> /workspaces/:workspaceSlug/projects/:projectSlug/activity
```

原因：

- notes/annotations 是项目事实流，不是设置项。
- 配置值和配置定义是管理面，应继续留在 settings。
- 项目更新需要在 Activity 和 Overview 中自然出现，不能藏在管理页。

## 11. 数据与 API 设计

### 11.1 第一阶段端点与新增摘要

| 用途 | 端点或函数 | 说明 |
|---|---|---|
| 项目基础信息 | `GET /api/v1/projects/{projectRef}` | 返回 `ProjectView`，含任务计数和最近 annotations |
| 项目任务 | `GET /api/v1/tasks?project={projectRef}&limit=200` | 支持现有筛选和排序参数 |
| 项目时间线 | `GET /api/v1/projects/{projectRef}/timeline?limit=&offset=` | 合并 project annotation 与 task annotation |
| 项目 annotations | `GET/POST/DELETE /api/v1/projects/{projectRef}/annotations` | Activity 发布、删除和兼容读取；首屏列表以 timeline 为主，避免重复 |
| 项目附属信息 | `GET /api/v1/projects/{projectRef}/config/effective` | Overview 和右栏显示 label/value 形式的项目附属信息 |
| 项目任务摘要 | 新增 `GET /api/v1/projects/{projectRef}/task-summary` 或等价 ProjectSummary API | Overview 和右栏展示全量重点任务、风险计数和负责人负载；这是 Overview 全局统计的权威来源；同时要求 `project:read` 与 `task:read` |
| 工作区成员 | `GET /api/v1/workspaces/{workspaceSlug}/members` | 任务负责人筛选选项 |
| 项目审计 | `GET /api/v1/audit?project={projectRef}` | 仅在有 `audit:read` 权限时使用 |

ProjectSummary API 最小字段：

| 字段 | 数据来源 | 计算规则 |
|---|---|---|
| `overdue_count` | `tasks` 表 | `due < now` 且 `status` 不为 completed/deleted；date-only due 已在写入/解析时按本地日末规则落库，summary 不重新用浏览器时区解释 |
| `overdue_refs` | `tasks.task_slug`、`tasks.uuid`、`tasks.title` | 取最多 3 个任务短标识；优先 `task_slug`，否则 UUID 短码 |
| `high_priority_open_count` | `tasks` 表 | `priority == H` 且 `status` 不为 completed/deleted |
| `high_priority_open_refs` | 同上 | 取最多 3 个任务短标识 |
| `wait_ready_count` | `tasks` 表 | `wait <= now` 且 `status` 不为 completed/deleted；date-only wait 已按本地日初规则落库 |
| `wait_ready_refs` | 同上 | 取最多 3 个任务短标识 |
| `unassigned_open_count` | `tasks` + `task_assignees` | 无 assignee 且 `status` 不为 completed/deleted |
| `unassigned_open_refs` | `tasks.task_slug`、`tasks.uuid`、`tasks.title` | 取最多 3 个任务短标识 |
| `workload` | `task_assignees` + `users` | 按用户聚合未关闭任务数、逾期数、高优数；显示名使用 `display_name -> name -> email -> user_id` |

这些字段必须由后端按项目全量任务聚合。前端不能用当前 Tasks 页列表、当前筛选条件或分页结果计算 Overview 的“当前重点”。摘要聚合必须复用现有 workspace/project scope 解析，不能统计当前凭证不可见的项目或任务。

ProjectSummary 的时间规则：

- `now` 使用服务端注入时钟，测试中必须可控。
- date-only `due` / `until` 沿用璇础既有规则：本地日末。
- date-only `wait` / `scheduled` 沿用璇础既有规则：本地日初。
- 前端只展示后端结果，不用浏览器时区重算 `overdue_count` 或 `wait_ready_count`。

ProjectSummary 命名规则：

- JSON 字段保留 `wait_ready_*` 和 `unassigned_open_*`，表达机器语义。
- 中文 UI 中 `wait_ready_*` 显示为「等待已到期」。
- 中文 UI 中 `unassigned_open_*` 显示为「未分配任务」，避免误解为项目负责人缺失。

### 11.2 页面数据点契约

| 页面/区域 | 数据点 | 权威来源 | 空值或缺权限处理 |
|---|---|---|---|
| Header | 项目名、描述、状态 | `GET /api/v1/projects/{projectRef}` 的 `name`、`description`、`status` | 项目读取失败则整页显示错误 |
| Header | 导入按钮可见性 | 当前用户 `task:write` 权限 + 项目状态非 archived/cancelled | 不满足时隐藏 |
| Header | 设置按钮可见性 | 当前用户 `project:manage` 权限 | 不满足时隐藏 |
| Tabs | 当前激活 tab | 当前 URL pathname | 不从后端读取 |
| 右栏 | 项目任务总数、完成数、待办计数 | `ProjectView.task_count`、`completed_count`、`pending_count` | 缺字段时隐藏对应行，不用任务列表回填全量数字 |
| 右栏/Overview | 逾期、高优、等待已到期、未分配任务 | ProjectSummary API 的项目全量聚合结果 | API 缺失或失败时隐藏该区块或显示轻量错误；不能退回用当前任务列表冒充全量 |
| 右栏/Overview | 项目附属信息 | `ConfigEffectiveValue[]` 的 `definition.label || key` 和 effective `value` | 请求失败时显示轻量错误或隐藏，不阻塞页面；secret 值只显示脱敏值或“已设置” |
| 右栏/Overview | 负责人负载 | ProjectSummary API 的项目全量 assignee 聚合结果 | 无 assignees 归为未分配；API 缺失时不展示 |
| Overview | 最新项目更新 | `ProjectView.recent_annotations[0]` | 没有记录时显示“暂无项目更新”或隐藏 |
| Tasks | 任务行 | `GET /api/v1/tasks?project=...` 返回的任务对象 | 字段为空显示 `-`，不推断不存在字段 |
| Activity | 项目更新 | project annotation | 没有记录时显示空状态 |
| Activity | 任务注解 | project timeline 中 `source_type=task` | 只展示 timeline 返回的信息 |
| Activity | 审计 | audit endpoint | 无 `audit:read` 时不请求、不展示 |

### 11.3 后续可选增强

| 增强 | 目的 | 触发条件 |
|---|---|---|
| `timeline` 支持 `source_type` 筛选 | Activity 过滤更轻 | Activity 首屏性能或分页复杂度变高 |
| 任务列表返回 `child_count` | 父子任务树首屏更准确 | 项目任务量大，前端当前页聚合不够 |
| 项目 activity 聚合端点 | 后端统一裁剪 timeline + audit | 前端组合端点导致重复分页或权限分支复杂 |
| ProjectSummary 形态调整 | 若不单独建 `/task-summary`，可以把同等字段挂到 `GET /api/v1/projects/{ref}` 的 `summary` 下 | 只改变承载位置，不改变“后端全量聚合是权威来源”的要求 |
| 侧栏开合偏好持久化 | 跨页面保持用户偏好 | 已有 UI preference/localStorage 约定或用户明确需要 |

第一阶段不做这些增强，避免为了页面拆分扩大后端范围。

## 12. 前端组件边界

建议新增或重组以下组件：

| 组件 | 责任 |
|---|---|
| `ProjectLayout` | 加载项目基础信息，渲染 header、tabs、左右两栏 |
| `ProjectHeaderEditor` | 继续负责项目名、描述、状态、复制、设置入口 |
| `ProjectTabs` | 根据当前路径显示概览、任务、活动 |
| `ProjectContextRail` | 右侧项目信息栏，聚合项目、任务、配置、timeline 预览，并支持打开/收起 |
| `ProjectOverviewPage` | 渲染最新更新、当前重点、项目附属信息、负载摘要、最近活动 |
| `ProjectTasksPage` | 渲染任务工具栏、简单任务列表、任务创建、导入、行内编辑 |
| `ProjectActivityPage` | 渲染项目更新输入框、时间线、审计合并、筛选 |
| `ProjectActivityTimeline` | 负责统一渲染 project annotation、task annotation、audit rows |

页面编排原则：

- `ProjectLayout` 只处理项目级数据、tabs、右栏开合和整体布局，不承载任务表内部逻辑。
- `ProjectTasksPage` 继续复用现有 `ProjectTaskToolbar`、`TaskTable`、`TaskCreateDialog`、`TaskImportDialog`。
- `ProjectOverviewPage` 和 `ProjectContextRail` 可以共享统计派生函数，避免重复计算。
- `ProjectActivityPage` 只在权限允许时请求 audit，避免把 403 当页面错误。
- `ProjectContextRail` 在桌面宽度内作为右栏展示；窄屏默认收起，展开时不能压缩任务表格列宽到文字重叠。

## 13. 状态、权限与错误处理

| 场景 | 行为 |
|---|---|
| 无 `project:read` | 显示项目读取权限错误，提供返回项目列表入口 |
| 有 `project:read` 无 `task:read` | Overview 只显示项目基础信息、项目更新和可读配置；不请求 ProjectSummary；Tasks 页显示任务读取权限错误 |
| 有 `project:read` 无 `audit:read` | Activity 不显示审计筛选项，也不请求审计 |
| archived/cancelled 项目 | 允许读取 Overview、Tasks、Activity；隐藏任务创建、导入、项目更新写入入口 |
| 配置 effective 请求失败 | Overview 和右栏隐藏项目附属信息或显示轻量错误，不阻塞任务与项目信息 |
| timeline 请求失败 | 最近活动区显示轻量错误；Activity 页允许重试 |

## 14. 验收标准

1. `/workspaces/:workspaceSlug/projects/:projectSlug` 显示概览页，不再直接铺完整任务表。
2. `/workspaces/:workspaceSlug/projects/:projectSlug/tasks` 显示任务筛选、简单任务列表、新建、导入和行内编辑能力；第一阶段不显示任务分组或父子树。
3. `/workspaces/:workspaceSlug/projects/:projectSlug/activity` 显示项目更新输入框和项目时间线。
4. 项目 header 与右侧项目信息栏在三个子页面保持一致。
5. 右侧项目信息栏可以打开和收起；收起时左侧主体获得更多宽度，并保留展开入口。
6. 所有展示数字、状态、人员、活动记录都能追溯到本 spec 的数据点契约。
7. `settings/notes` 不再作为主要 notes 入口；项目更新归入 Activity。
8. 没有 `audit:read` 权限的用户不会看到审计筛选项，也不会因 audit 403 影响 Activity。
9. archived/cancelled 项目仍可浏览，但不显示写入动作。
10. 当前任务详情页路径保持可用。
11. 中文界面文案清楚表达真实行为，不使用“issue”“milestone”“lead”等当前模型不存在的概念。
12. ProjectSummary 的逾期和等待已到期判断与璇础任务日期边界规则一致。
13. 项目附属信息中的 secret 配置不展示原文。
14. Activity 不重复展示同一条 project annotation；项目更新时间线以 project timeline 为主。
15. 窄屏下右侧栏默认收起或变成不挤压主内容的展示形态，任务表格文字不重叠。

## 15. 测试与验证

前端测试重点：

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
git diff --check
```

需要覆盖：

- 项目根路由显示 Overview，Tasks 和 Activity tab 导航正确。
- `ProjectContextRail` 在三个子页面都展示，并可打开/收起。
- `ProjectContextRail` 展开/收起按钮是图标按钮，带 `aria-label` 和 `title`；窄屏不挤压任务表格。
- Overview 能显示最新项目更新、风险摘要、项目附属信息和最近活动。
- Overview 风险摘要使用 ProjectSummary，覆盖 date-only due/wait 边界规则，并在无 `task:read` 时不请求摘要。
- Overview 和右栏展示 secret config 时只显示脱敏值或“已设置”，不显示原文。
- Tasks 页保留当前筛选 URL 同步、任务创建、任务导入、行内编辑能力，并保持简单列表形态。
- Activity 页在有/无 `audit:read` 权限时分别展示正确内容，并且 project annotation 不因同时读取 timeline 与 annotations 重复展示。
- archived/cancelled 项目隐藏写入动作。

如实现过程中新增或调整后端 API，再补跑：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 16. 后续实现计划提示

实现计划应拆成小步：

1. 抽项目级 layout、tabs、右栏，不改变业务行为。
2. 将当前 `ProjectWorkbenchPage` 的任务工具栏和表格迁入 Tasks 子页面，保持简单列表。
3. 新建 Overview 子页面，组合项目更新、风险摘要、项目附属信息、负载摘要、最近活动。
4. 新建 Activity 子页面，迁移项目 notes 能力，合并 timeline 与可选 audit。
5. 调整 settings notes 路由为跳转或兼容入口。
6. 补测试、i18n 文案和 smoke 验证。
