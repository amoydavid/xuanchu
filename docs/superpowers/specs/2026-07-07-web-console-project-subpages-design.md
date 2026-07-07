# Web Console 项目子页面信息架构设计

**日期：** 2026-07-07
**状态：** 待评审
**范围：** `/workspaces/:workspaceSlug/projects/:projectSlug` 项目详情与项目级子页面

## 1. 背景

当前项目详情页已经具备项目编辑、状态切换、配置概览、任务筛选、任务表格、负责人负载和最近动态等能力，但这些信息都挤在同一个页面里。用户进入 `/workspaces/local/projects/ops` 时，看到的是一个混合工作台，而不是清晰回答不同问题的项目空间。

Linear 的启发不是照搬 `Overview / Activity / Issues` 这几个词，而是它的信息架构：同一个项目上下文在右侧保持稳定，左侧根据用户正在查看的信息主体切换。对璇础来说，左侧主体应该围绕项目态势、任务执行、项目事实流三类问题展开。

璇础当前数据结构也决定了不能盲目复制 Linear：

- `Project` 当前没有负责人、项目成员、里程碑模型。
- 任务是核心执行对象，当前产品语义是 `task`，不是 `issue`。
- 项目级 annotation 已经存在，且 `ProjectTimeline` 已能合并 project annotation 与 task annotation。
- 项目配置 effective value 是璇础区别于普通项目管理工具的重要信息，应该作为项目运行上下文展示。

## 2. 目标

1. 将当前项目单页拆成项目级子页面：`Overview`、`Tasks`、`Activity`。
2. 保持项目头部和右侧项目信息栏在子页面之间一致，减少上下文切换成本。
3. 让 `Overview` 回答“项目现在怎么样，下一步该看哪里”，而不是继续展示完整任务表。
4. 让 `Tasks` 成为任务执行和筛选主页面，承接当前任务工具栏、任务表格、导入、新建能力。
5. 让 `Activity` 成为项目更新、任务注解、项目相关审计的时间线页面。
6. 第一阶段优先复用现有 API 和数据结构，不新增 project lead、project members、milestones 等模型。

## 3. 非目标

- 不把 `task` 改名为 `issue`。
- 不新增项目负责人、项目成员、里程碑数据表。
- 不做看板、拖拽排序、批量编辑。
- 不在没有 `audit:read` 权限时展示审计记录。
- 不把项目配置编辑表单放进 Overview；Overview 只展示运行上下文摘要，编辑仍进入设置页。
- 不改变 CLI、MCP、HTTP API 的身份输出规范。

## 4. 核心决策

| 主题 | 决策 |
|---|---|
| 默认项目页 | `/workspaces/:workspaceSlug/projects/:projectSlug` 显示 Overview |
| 任务执行页 | 新增 `/workspaces/:workspaceSlug/projects/:projectSlug/tasks` |
| 活动页 | 新增 `/workspaces/:workspaceSlug/projects/:projectSlug/activity` |
| 设置页 | 继续使用现有 `/projects/:projectSlug/settings/*`，不放入主 tab |
| 右侧栏 | 新增共享 `ProjectContextRail`，所有项目子页面复用 |
| Notes 归属 | 项目 notes/annotations 从 settings 迁到 Activity |
| 第一阶段数据策略 | 前端组合现有端点，不新增后端聚合端点 |
| 任务层级 | Tasks 页保留 Linear 式父子任务树方向，第一阶段可先在当前结果内聚合，后续补 `child_count` |

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
| 工作区 / 项目代号                                      [复制] [状态] [设置] |
| 项目名称                                                                     |
| 项目描述                                                                     |
+------------------------------------------------------------------------------+
| [概览] [任务] [活动]                                                         |
+-----------------------------------------------+------------------------------+
| 左侧：当前子页面主体                          | 右侧：项目信息栏             |
|                                               |                              |
| 概览页：项目态势、重点风险、运行配置、摘要     | 属性                         |
| 任务页：筛选、任务树、任务操作                | 进度                         |
| 活动页：更新输入框、时间线、审计              | 运行配置                     |
|                                               | 负责人负载                   |
|                                               | 最近活动                     |
+-----------------------------------------------+------------------------------+
```

## 6. 共享区域

### 6.1 Project Header

```text
工作区 / 项目代号                                      [导入任务] [复制] [状态] [设置]
项目名称
项目描述
```

展示内容：

| 区块 | 数据来源 | 展示原因 |
|---|---|---|
| 工作区、项目代号 | route params + `GET /api/v1/projects/{ref}` | 明确当前上下文，避免跨 workspace/project 操作误判 |
| 项目名称、描述 | `ProjectView.name`、`ProjectView.description` | 项目主身份，支持 inline 编辑 |
| 状态菜单 | `ProjectView.status` + transition API | 状态是项目级决策，应在所有子页可见 |
| 导入任务 | 当前任务导入能力 | 属于任务执行动作，只在有写权限且项目未关闭时显示，可在 Tasks 页更突出 |
| 复制链接 | 当前浏览器 URL | 项目页是协作入口，需要稳定分享 |
| 设置入口 | 现有 project settings route | 配置编辑属于管理面，保持齿轮入口 |

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

右侧栏在所有项目子页面保持一致。它不是第二个详情页，而是“当前项目的固定上下文”。

```text
+-- 项目信息 --------------------+
| 状态          active           |
| 任务          38 总数 / 22 未完 |
| 创建          2026-07-01       |
| 更新          2026-07-07       |
+-- 进度 ------------------------+
| 已完成        #####----- 42%    |
| 逾期          2                |
| 高优未完成    3                |
+-- 运行配置 --------------------+
| 缺必填        1                |
| 项目覆盖      2                |
| 继承工作区    4                |
+-- 负责人负载 ------------------+
| 未分配        5 未完 / 2 逾期  |
| 张三          8 未完 / 1 高优  |
| 李四          4 未完           |
+-- 最近活动 --------------------+
| 王五发布项目更新               |
| DEM-12 修改截止日期            |
| DEM-8 已完成                   |
+--------------------------------+
```

数据来源与展示原因：

| 区块 | 数据来源 | 为什么在右栏展示 |
|---|---|---|
| 状态、任务总数、创建、更新 | `GET /api/v1/projects/{ref}` | 项目元信息跨子页都需要可见 |
| 完成进度 | `completed_count / task_count` | 快速判断项目推进状态 |
| 逾期、高优未完成 | `GET /api/v1/tasks?project=...` 前端派生 | 比完整任务表更适合作为固定风险提示 |
| 运行配置 | `GET /api/v1/projects/{ref}/config/effective?console_home=true` | Agent/runtime 项目的关键上下文，不应藏在设置里 |
| 负责人负载 | 项目任务 assignees 聚合 | 当前没有项目成员模型，展示任务负载比展示“成员”更真实 |
| 最近活动 | `GET /api/v1/projects/{ref}/timeline?limit=5`，有权限时合并 audit 预览 | 给用户持续的项目活态信号 |

## 7. Overview 子页面

### 7.1 目标

Overview 是项目判断页，不是完整任务表。它应该让用户在 30 秒内知道：

- 最近一次项目级判断是什么。
- 当前最需要处理的风险是什么。
- 运行配置是否阻塞 Agent 或集成。
- 任务负载是否集中或有大量未分配。
- 最近有没有关键变化。

### 7.2 ASCII 原型

```text
+-- 概览 ------------------------------------------------------------+
| 最新项目更新                                                       |
| 进展正常 · 王五 · 2 小时前                                         |
| 本周目标：完成远程 token 配置和 MCP 示例验证。                     |
|                                                        [写项目更新] |
+--------------------------------------------------------------------+
| 当前重点                                                           |
| 逾期任务        2      DEM-7、DEM-9                         [查看] |
| 高优未完成      3      DEM-2、DEM-4、DEM-8                  [查看] |
| 等待解除        5      wait 小于等于今天                    [查看] |
| 缺负责人        4                                       [查看]    |
+--------------------------------------------------------------------+
| 运行配置                                                           |
| llm.provider        项目覆盖          openai                       |
| webhook.secret      缺少必填          -                            |
| agent.mode          继承工作区        supervised                   |
|                                                        [管理配置]   |
+--------------------------------------------------------------------+
| 负责人负载                                                         |
| 未分配      ####----  5 未完 / 2 逾期                              |
| 张三        ########  8 未完 / 1 高优                              |
| 李四        ###-----  3 未完                                       |
+--------------------------------------------------------------------+
| 最近活动                                                           |
| 王五发布项目更新 · 2 小时前                                        |
| DEM-12 修改截止日期 · 今天                                         |
| DEM-8 已完成 · 昨天                                                |
|                                                        [查看全部]   |
+--------------------------------------------------------------------+
```

### 7.3 区块说明

| 区块 | 展示内容 | 数据来源 | 为什么放在这里 |
|---|---|---|---|
| 最新项目更新 | 最近一条 project annotation 的内容、作者、时间 | `ProjectView.recent_annotations` 或 `GET /api/v1/projects/{ref}/annotations` | 项目首页需要人类可读判断，单纯统计不足以表达项目状态 |
| 当前重点 | 逾期、高优未完成、等待解除、缺负责人 | `GET /api/v1/tasks?project=...` 派生 | 把任务表转成行动入口，帮助用户先处理风险 |
| 运行配置 | console-home effective config、缺必填、来源 | `listProjectEffectiveConfig(projectRef, { consoleHome: true })` | 璇础项目经常驱动 Agent/runtime，配置缺失会直接影响执行 |
| 负责人负载 | 未分配、每个负责人未完成/逾期/高优 | 项目任务 assignees 聚合 | 当前没有项目成员模型，任务负载是更真实的协作信号 |
| 最近活动 | timeline 最近 3 到 5 条 | `GET /api/v1/projects/{ref}/timeline?limit=5` | 给 Overview 提供项目活态，但不替代 Activity 页 |

## 8. Tasks 子页面

### 8.1 目标

Tasks 是项目执行页，承接当前项目工作台的大部分操作能力。它应该专注于任务列表、筛选、排序、创建、导入和轻量编辑。

### 8.2 ASCII 原型

```text
+-- 任务 ------------------------------------------------------------+
| [搜索标题或内容        ] [状态] [优先级] [负责人] [标签] [日期] [+] |
| 当前筛选：状态=未完成  负责人=我                         [清除全部] |
+--------------------------------------------------------------------+
| v 未完成  12                                                       |
|   > DEM-1   父任务标题                         高优   张三   周五 |
|     `- DEM-3 子任务标题                         中优   李四   周五 |
|     `- DEM-4 子任务标题                         -      未分配 下周 |
|     DEM-5   独立任务标题                       -      王五   -    |
+--------------------------------------------------------------------+
| v 等待中  4                                                        |
|     DEM-8   等外部回调确认                    低优   张三   -    |
+--------------------------------------------------------------------+
| v 已完成  22                                                       |
|     DEM-2   已完成任务                         -      李四   昨天 |
+--------------------------------------------------------------------+
```

### 8.3 区块说明

| 区块 | 展示内容 | 数据来源 | 为什么放在这里 |
|---|---|---|---|
| 筛选工具栏 | 搜索、状态、优先级、负责人、标签、日期、更多筛选 | URL search + `ProjectTaskToolbar` | 任务页的首要工作是缩小任务集合 |
| 新建任务 | 打开 `TaskCreateDialog` | 现有 task create API | 用户在查看任务时最可能创建同项目任务 |
| 导入任务 | 打开 `TaskImportDialog` | 现有导入能力 | 批量导入属于任务页主动作 |
| 任务树 | 父任务、子任务、独立任务、状态分组 | `GET /api/v1/tasks?project=...`，使用 `parent` / `parent_info` | 任务有父子关系时，默认平铺会失去结构 |
| 行内编辑 | 标题、优先级、截止日期、状态动作 | 现有 task mutation hooks | 当前项目工作台已证明编辑面应在 workbench 内 |
| 负责人负载 | 可留在右栏或页面下方补充 | 当前任务集合派生 | 任务页需要快速判断任务是否堆在人或未分配上 |

### 8.4 父子任务规则

第一阶段不新增后端字段时：

- 前端在当前加载的任务集合内按 `parent` 聚合子任务。
- 默认显示父任务和独立任务，子任务跟随父任务展开。
- 如果筛选命中子任务但父任务不在当前结果内，应显示该子任务，并用 `parent_info` 标记父任务。
- recurring 父任务继续保持现有隐藏语义，不为了树结构强制显示 recurring parent。

后续增强时：

- 后端任务列表可返回 `child_count`。
- 展开父任务时可按父 UUID 懒加载子任务，避免首屏拉取过多。

## 9. Activity 子页面

### 9.1 目标

Activity 是项目事实流。它应该合并项目更新、任务注解和有权限可见的项目审计，让用户看到“项目发生了什么”。

### 9.2 ASCII 原型

```text
+-- 活动 ------------------------------------------------------------+
| [项目更新] [评论]                                                  |
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
| * 项目状态变更 · 系统 · 18:30                                      |
|   planning -> active                                               |
|                                                        [加载更多]   |
+--------------------------------------------------------------------+
```

### 9.3 区块说明

| 区块 | 展示内容 | 数据来源 | 为什么放在这里 |
|---|---|---|---|
| 发布项目更新 | 多行文本输入，保存为 project annotation | `POST /api/v1/projects/{ref}/annotations` | 项目级 annotation 原本就是项目记录，不应藏在 settings |
| 项目更新时间线 | project annotation | `GET /api/v1/projects/{ref}/annotations` 或 timeline 中 `source_type=project` | 展示人类写下的项目判断、风险、决策 |
| 任务注解 | task annotation | `GET /api/v1/projects/{ref}/timeline` 中 `source_type=task` | 任务执行过程中的重要事实也属于项目上下文 |
| 审计记录 | project-scoped audit | `GET /api/v1/audit?project=<ref>&limit=...`，仅有 `audit:read` 时请求 | 字段变更是事实流的一部分，但权限高于普通项目读取 |
| 时间分组 | 按本地日期分组 | 前端格式化 `entry` / `created_at` | 时间线需要按天扫描，减少长列表噪音 |

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

### 11.1 第一阶段复用现有端点

| 用途 | 端点或函数 | 说明 |
|---|---|---|
| 项目基础信息 | `GET /api/v1/projects/{projectRef}` | 返回 `ProjectView`，含任务计数和最近 annotations |
| 项目任务 | `GET /api/v1/tasks?project={projectRef}&limit=200` | 支持现有筛选和排序参数 |
| 项目时间线 | `GET /api/v1/projects/{projectRef}/timeline?limit=&offset=` | 合并 project annotation 与 task annotation |
| 项目 annotations | `GET/POST/DELETE /api/v1/projects/{projectRef}/annotations` | Activity 发布与列表 |
| 项目配置 | `GET /api/v1/projects/{projectRef}/config/effective` | Overview 和右栏显示运行配置 |
| 工作区成员 | `GET /api/v1/members` 或现有 members helper | 任务负责人筛选选项 |
| 项目审计 | `GET /api/v1/audit?project={projectRef}` | 仅在有 `audit:read` 权限时使用 |

### 11.2 后续可选增强

| 增强 | 目的 | 触发条件 |
|---|---|---|
| `timeline` 支持 `source_type` 筛选 | Activity 过滤更轻 | Activity 首屏性能或分页复杂度变高 |
| 任务列表返回 `child_count` | 父子任务树首屏更准确 | 项目任务量大，前端当前页聚合不够 |
| 项目 activity 聚合端点 | 后端统一裁剪 timeline + audit | 前端组合端点导致重复分页或权限分支复杂 |
| 任务统计聚合端点 | Overview 不再依赖拉取完整任务列表 | 项目任务量超过当前 `limit=200` 假设 |

第一阶段不做这些增强，避免为了页面拆分扩大后端范围。

## 12. 前端组件边界

建议新增或重组以下组件：

| 组件 | 责任 |
|---|---|
| `ProjectLayout` | 加载项目基础信息，渲染 header、tabs、左右两栏 |
| `ProjectHeaderEditor` | 继续负责项目名、描述、状态、复制、设置入口 |
| `ProjectTabs` | 根据当前路径显示概览、任务、活动 |
| `ProjectContextRail` | 右侧项目信息栏，聚合项目、任务、配置、timeline 预览 |
| `ProjectOverviewPage` | 渲染最新更新、当前重点、运行配置、负载摘要、最近活动 |
| `ProjectTasksPage` | 渲染任务工具栏、任务树、任务创建、导入、行内编辑 |
| `ProjectActivityPage` | 渲染项目更新输入框、时间线、审计合并、筛选 |
| `ProjectActivityTimeline` | 负责统一渲染 project annotation、task annotation、audit rows |

页面编排原则：

- `ProjectLayout` 只处理项目级数据和布局，不承载任务表内部逻辑。
- `ProjectTasksPage` 继续复用现有 `ProjectTaskToolbar`、`TaskTable`、`TaskCreateDialog`、`TaskImportDialog`。
- `ProjectOverviewPage` 和 `ProjectContextRail` 可以共享统计派生函数，避免重复计算。
- `ProjectActivityPage` 只在权限允许时请求 audit，避免把 403 当页面错误。

## 13. 状态、权限与错误处理

| 场景 | 行为 |
|---|---|
| 无 `project:read` | 显示项目读取权限错误，提供返回项目列表入口 |
| 有 `project:read` 无 `task:read` | Overview 只显示项目基础信息和项目更新；Tasks 页显示任务读取权限错误 |
| 有 `project:read` 无 `audit:read` | Activity 不显示审计筛选项，也不请求审计 |
| archived/cancelled 项目 | 允许读取 Overview、Tasks、Activity；隐藏任务创建、导入、项目更新写入入口 |
| 配置 effective 请求失败 | Overview 和右栏隐藏配置摘要或显示轻量错误，不阻塞任务与项目信息 |
| timeline 请求失败 | 最近活动区显示轻量错误；Activity 页允许重试 |

## 14. 验收标准

1. `/workspaces/:workspaceSlug/projects/:projectSlug` 显示概览页，不再直接铺完整任务表。
2. `/workspaces/:workspaceSlug/projects/:projectSlug/tasks` 显示任务筛选、任务列表、新建、导入和行内编辑能力。
3. `/workspaces/:workspaceSlug/projects/:projectSlug/activity` 显示项目更新输入框和项目时间线。
4. 项目 header 与右侧项目信息栏在三个子页面保持一致。
5. `settings/notes` 不再作为主要 notes 入口；项目更新归入 Activity。
6. 没有 `audit:read` 权限的用户不会看到审计筛选项，也不会因 audit 403 影响 Activity。
7. archived/cancelled 项目仍可浏览，但不显示写入动作。
8. 当前任务详情页路径保持可用。
9. 中文界面文案清楚表达真实行为，不使用“issue”“milestone”“lead”等当前模型不存在的概念。

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
- `ProjectContextRail` 在三个子页面都展示。
- Overview 能显示最新项目更新、风险摘要、配置摘要和最近活动。
- Tasks 页保留当前筛选 URL 同步、任务创建、任务导入、行内编辑能力。
- Activity 页在有/无 `audit:read` 权限时分别展示正确内容。
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
2. 将当前 `ProjectWorkbenchPage` 的任务工具栏和表格迁入 Tasks 子页面。
3. 新建 Overview 子页面，组合项目更新、风险摘要、配置摘要、负载摘要、最近活动。
4. 新建 Activity 子页面，迁移项目 notes 能力，合并 timeline 与可选 audit。
5. 调整 settings notes 路由为跳转或兼容入口。
6. 补测试、i18n 文案和 smoke 验证。
