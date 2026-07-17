# Web Console 用户首页重构设计

**日期：** 2026-07-17

**状态：** 已实施

**范围：** Web Console 登录后的 workspace 首页 `/`、首页摘要 API、首页相关导航文案

**目标用户：** 使用浏览器处理日常任务的 workspace 成员；owner/admin 仍可从管理分组进入治理页面

**文档关系：** 本设计延续 `2026-07-05-web-console-capability-bridge-design.md` 的“任务协作入口 + 治理控制台”边界，并复用 `2026-07-07-web-console-project-subpages-design.md` 的项目摘要口径。它替换 `2026-07-07-web-console-config-definition-management-design.md` 第 11 节关于首页位置和空状态的表达，但不改变 `show_on_console_home`、effective value、scope 和 secret 脱敏规则。

## 0. 结论

将 Web Console 登录后的默认页面从“运维概览”改为“个人工作首页”。首页首先回答三个问题：

1. 我现在应该处理什么任务？
2. 哪些项目需要我关注？
3. 这个 workspace 有哪些被管理员明确放到首页的业务信息？

首页不再展示当前操作者、Token 类型、Scope、失败投递表和审计表。身份与凭证已经在左侧栏底部表达；投递与审计分别回到通知、Hook 和审计页面。首页只在用户能够采取行动时展示异常或风险，不把控制面数据当成用户工作。

推荐形态是“个人工作台 + 项目关注 + 可选工作区信息”，而不是把 `/` 直接重定向到 `/my-tasks`。这样既保留个人执行主线，又能在不复制完整项目页的前提下提供跨项目判断。

## 1. 背景与现状

### 1.1 重构前首页

当前 `web/src/pages/OverviewPage.tsx` 同时请求：

- `GET /api/v1/tasks?limit=200`
- `GET /api/v1/projects`
- 两次 notification delivery 查询
- `GET /api/v1/audit?limit=5`
- workspace console-home effective config

页面依次展示：

- 当前操作者、Token 类型、Scope
- 任务总数、逾期、七天内到期、项目数、失败投递数
- 配置概览
- 最近失败投递
- 最近审计

这个结构有四个问题：

1. **主语是系统，不是用户。** 首屏解释“我以什么凭证连接了服务”，没有回答“我接下来做什么”。
2. **身份信息重复。** `AppShell` 左侧栏底部已经展示身份、角色和凭证类型，首页再次展示没有新的决策价值。
3. **统计口径不可靠。** 首页用 `limit=200` 的任务结果在浏览器内计算总数和日期指标；任务超过 200 条时，页面仍会把分页子集表现成全量统计。
4. **运维信息抢占业务信息。** failed delivery 和 audit 对 owner/admin 有价值，但不是普通成员每天登录后的首要工作。

### 1.2 已有能力

仓库已经具备构成用户首页的主要能力：

- `/my-tasks` 已支持未完成、今日到期、逾期、无截止和已完成视图，并支持任务行操作与批量操作。
- `/projects` 已返回项目状态、任务数、待办数和完成数。
- 项目页已有权威 `ProjectTaskSummary`，包含逾期、高优未完成、等待已到期、未分配和负责人负载。
- 项目详情已拆成概览、任务、活动三个子页面。
- `ConfigDefinition.show_on_console_home` 已定义管理员主动放到首页的 workspace 信息。
- `AppShell` 已把导航分为个人、管理、系统三组，并在底部展示当前身份。

本次设计复用这些概念，不新增看板、甘特、PMO 报表或独立通知中心。

## 2. 外部产品调研

调研时间为 2026-07-17，优先采用产品官方文档。不同产品的字段和视觉不同，但登录后入口的主线高度一致。

| 产品 | 官方入口的做法 | 可复用原则 |
|---|---|---|
| Asana | Home 是可定制 landing page，常用 widget 包括 My Tasks 和 Recent Projects；My Tasks 自动聚合跨项目分配给当前用户的任务 | 首页先聚合“我的工作”，项目上下文作为第二层 |
| Jira | `For you` 用于恢复工作，展示最近访问或处理的 work items、boards、spaces、分配给我的工作和 starred items | 首页应帮助用户继续工作，而不是展示系统配置 |
| Linear | `My issues` 聚合 assigned、created、subscribed、recent activity，并用 urgent、SLA、blocker、cycle、active 等顺序组织重点 | 用户首页需要后端可解释的优先顺序，不应只给总数 |
| ClickUp | Home Sidebar 是工作汇合点；`My Tasks` 提供 Assigned to me、Today & Overdue、Personal List，并把 Inbox 与 favorites 放在相邻入口 | 首页要把今日与逾期放在首层，完整列表留在专门页面 |

官方资料：

- Asana：[Set up your ideal workspace](https://help.asana.com/s/article/set-up-your-ideal-workspace)、[My Tasks](https://help.asana.com/s/article/my-tasks)
- Jira：[Navigate to your work](https://support.atlassian.com/jira-software-cloud/docs/navigate-to-your-work/)
- Linear：[My issues](https://linear.app/docs/my-issues)
- ClickUp：[What is the Home Sidebar?](https://help.clickup.com/hc/en-us/articles/32057009861271-What-is-the-Home-Sidebar)

### 2.1 对璇础的启示

璇础不需要复制这些产品的全部模块。它当前最可靠、最有用户价值的首页信息是：

- 当前用户的 open tasks；
- 逾期、今日到期、已经开始和高优任务；
- workspace 内需要关注的项目风险；
- 管理员明确标记为 `show_on_console_home` 的业务信息。

璇础当前没有“收藏项目”“个人最近访问”“统一业务 Inbox”的服务端模型。本次不使用审计日志假装用户动态，也不引入只存在于浏览器本机的伪个性化数据。

## 3. 方案比较

### 3.1 方案 A：`/` 直接进入“我的任务”

优点：实现最短，用户登录后立即看到任务。

缺点：失去跨项目判断；`/` 与 `/my-tasks` 完全重复；系统身份没有合理落点。

### 3.2 方案 B：保留 workspace 仪表盘，只替换几张卡片

优点：改动小，沿用当前页面结构。

缺点：主语仍然是 workspace；数字多但缺少可执行对象，不能解决“登录后该做什么”。

### 3.3 方案 C：个人工作台 + 项目关注 + 工作区信息

优点：个人任务是首要信息，同时保留项目态势与管理员定义的业务上下文；与现有 `/my-tasks`、项目概览和配置定义边界一致。

缺点：需要新增一个权限感知、权威聚合的首页摘要 API。

**采用方案 C。** 首页负责“判断与进入”，`/my-tasks` 和项目子页面负责“完整处理”。

## 4. 目标与非目标

### 4.1 目标

1. 普通用户在登录后 10 秒内知道最需要处理的任务。
2. 用户无需逐个打开项目即可知道哪些项目存在明显风险。
3. 首页所有数字都来自全量、权限内、服务端统一口径，不从分页列表推断。
4. 首页只展示能够解释、能够点击进入下一步的信息。
5. owner/admin 明确配置的 workspace 信息仍能出现在首页，但不抢占个人工作。
6. 系统身份进入 `/` 时有明确的降级页面，不伪造个人任务。

### 4.2 非目标

- 不替代 `/my-tasks` 的完整筛选、批量操作和历史任务能力。
- 不替代项目 Overview、Tasks、Activity 子页面。
- 不新增看板、甘特、里程碑、项目成员或项目负责人模型。
- 不新增收藏、最近访问、统一 Inbox 或消息已读模型。
- 不把 audit、Hook delivery、notification delivery 解释为用户活动。
- 不开放首页 widget 自由拖拽或自定义布局。
- 不改变 CLI、MCP、Remote 和现有 HTTP 资源的输出契约。

## 5. 核心决策

| 主题 | 决策 |
|---|---|
| 默认路由 | 保持 `/`，页面名称从“概览”改为“首页” |
| 首页主语 | 当前 user actor 的个人工作 |
| 首要区块 | “我的今日”：已经开始、逾期、今日到期、高优未完成任务 |
| 完整任务入口 | `/my-tasks`，新增“已开始”预设；首页不复制完整表格和全部筛选器 |
| 项目区块 | “项目关注”，按权限内项目的风险和最近更新时间排序；不称“我的项目” |
| workspace 配置 | 保留 `show_on_console_home=true`，放在页面末尾；没有数据时整块隐藏 |
| 运维数据 | 从首页移除，回到 Hook、通知、审计等专页 |
| 数据策略 | 新增 `GET /api/v1/home`，由 app/storage 权威聚合；禁止前端从 `limit=200` 列表计算全量指标 |
| 系统身份 | 不展示个人任务；展示系统身份说明、项目关注和管理快捷入口 |
| 个性化边界 | 第一版只按当前用户 assignee 个性化，不引入收藏与最近访问模型 |

## 6. 信息架构

### 6.1 全局结构

```text
Web Console
├── 个人
│   ├── 首页                 /
│   ├── 我的任务             /my-tasks
│   └── 项目                 /projects
├── 管理
│   ├── 成员                 /members
│   ├── 令牌                 /tokens
│   ├── Hook                 /hooks
│   ├── 通知                 /notifications
│   └── 单点登录             /sso
└── 系统
    ├── 工作区               /workspaces
    ├── 审计                 /audit
    └── 设置                 /settings
```

导航只把 `nav.overview` 的显示文案改为“首页”；`PageKey` 可继续使用 `overview`，避免无价值的内部重命名。

管理和系统分组不再影响首页内容。后续如果要收起低频导航，应单独评估，不能把本次首页重构扩大成 Shell 权限重写。

### 6.2 首页内容层级

```text
首页
├── 页面标题与主动作
│   ├── 你好，<display_name || name>
│   ├── 当前 workspace
│   └── 新建任务
├── 我的今日
│   ├── 已开始
│   ├── 逾期
│   ├── 今日到期
│   ├── 高优未完成
│   └── 最多 8 条重点任务
├── 项目关注
│   └── 最多 5 个风险项目或最近更新项目
└── 工作区信息
    └── show_on_console_home=true 的 effective config
```

这个顺序表达明确优先级：先执行个人任务，再判断项目，最后读取 workspace 上下文。

## 7. 桌面端 ASCII 原型

```text
+----------------------------------------------------------------------------------+
| 首页                                                     [新建任务] [刷新]       |
+----------------------------------------------------------------------------------+
| 你好，张三                                                                       |
| dajee workspace · 今天是 7 月 17 日                                              |
+----------------------------------------------------------------------------------+
| 我的今日                                                     [查看全部任务 →]    |
|                                                                                  |
| [已开始 1]    [逾期 3]    [今日到期 2]    [高优未完成 4]                        |
|                                                                                  |
| ● DEM-18  完成首页信息架构 spec                         已开始 · H · 今天       |
|   营销自动化改造                                               [停止] [完成]     |
| ! DEM-07  修复回调签名校验                                 逾期 2 天 · H       |
|   Webhook 稳定性                                               [开始] [完成]     |
| ! DEM-12  确认上线验收人                                   今天 18:00 · M       |
|   远程 CLI 发布                                                [开始] [完成]     |
|                                                                                  |
| 还有 5 项重点任务                                               [进入我的任务]   |
+---------------------------------------------------+------------------------------+
| 项目关注                            [查看项目 →]   | 工作区信息                   |
|                                                   |                              |
| Webhook 稳定性                                    | 默认通知渠道                 |
| 2 逾期 · 1 高优 · 63% 完成                        | 飞书项目群                   |
| 最新更新：回调域名已经确认         [打开项目]     | notifications.default_sink  |
|                                                   |                              |
| 远程 CLI 发布                                     | Agent 工作模式               |
| 1 今日到期 · 4 未分配 · 78% 完成                  | supervised                   |
| 最新更新：等待安全复核             [打开项目]     | agent.mode                   |
|                                                   |                              |
| 数据迁移                                          |                              |
| 无风险 · 91% 完成                  [打开项目]     |                              |
+---------------------------------------------------+------------------------------+
```

当没有 `show_on_console_home` 配置时，“项目关注”占满整行，工作区信息区不渲染。

## 8. 窄屏 ASCII 原型

```text
+--------------------------------------+
| 首页                    [+] [刷新]   |
+--------------------------------------+
| 你好，张三                           |
| dajee · 7 月 17 日                   |
+--------------------------------------+
| 我的今日                 [查看全部]  |
| [已开始 1] [逾期 3]                  |
| [今日 2]   [高优 4]                  |
|                                      |
| DEM-18 完成首页信息架构 spec         |
| 已开始 · H · 今天       [完成]       |
|                                      |
| DEM-07 修复回调签名校验              |
| 逾期 2 天 · H           [完成]       |
+--------------------------------------+
| 项目关注                 [查看项目]  |
| Webhook 稳定性                       |
| 2 逾期 · 1 高优 · 63%               |
| 最新更新：回调域名已经确认           |
+--------------------------------------+
| 工作区信息                           |
| 默认通知渠道                         |
| 飞书项目群                           |
+--------------------------------------+
```

窄屏按区块纵向排列。任务行只保留一个主要快捷动作“完成”；开始/停止等次要动作放进任务详情，避免一行出现多个小按钮。

## 9. 区块与交互

### 9.1 页面标题与问候

展示：

- 标题固定为“首页”。
- 问候使用 `actor.display_name || actor.name`，`name` 仍是稳定查找键，`display_name` 只用于展示。
- 显示 effective workspace 的 name；没有 name 时回退 slug。
- 日期按 Web Console 当前 locale 和 workspace 的服务端日期口径展示，不用于浏览器端重新判断逾期。

“新建任务”交互：

1. 仅在当前身份拥有 task write 能力时显示。
2. 点击后先选择一个 active/planning project。
3. 选定项目后复用现有 `TaskCreateDialog`，不创建 workspace 级无项目任务。
4. 创建成功后刷新首页摘要和 `/my-tasks` query，并提供“打开任务”操作。
5. 没有可写项目时隐藏按钮，不展示一个必然失败的入口。

### 9.2 “我的今日”摘要

四个计数彼此可以重叠，页面不把它们相加为“共 N 项”：

| 计数 | 口径 |
|---|---|
| 已开始 | assignee 为当前用户、open、`start != nil` |
| 逾期 | assignee 为当前用户、open、due 早于服务端今天开始 |
| 今日到期 | assignee 为当前用户、open、due 落在服务端今天；不含逾期 |
| 高优未完成 | assignee 为当前用户、open、priority=H |

open 指 `pending` 或 `waiting`。日期边界沿用璇础既有规则：date-only `due/until` 按本地日末，`wait/scheduled` 按本地日初。首页和 `/my-tasks` 必须调用同一 app/query 逻辑，不能分别在 Go 和浏览器里实现一套近似规则。

点击计数：

- 已开始：进入 `/my-tasks?tab=started`。本次同步为 `MyTaskTabKey`、路由校验和 `tabFilter` 增加 `started`；其查询固定为 `(status:pending or status:waiting) and start.notnull`。
- 逾期：进入 `/my-tasks?tab=overdue`。
- 今日到期：进入 `/my-tasks?tab=today`。
- 高优未完成：进入 `/my-tasks?tab=incomplete&priority=H`。

`start.notnull` 已由现有 query AST 支持。首页不把原始 query 暴露在 URL 中，统一通过 `started` 预设进入，保持与 today/overdue 相同的稳定入口。

### 9.3 重点任务列表

最多展示 8 条，由服务端返回已经排序的结果。每条任务可以带多个理由 badge，但只按以下顺序决定主排序：

1. 已开始；
2. 逾期；
3. 今日到期；
4. 高优未完成；
5. 其余 open task 按 urgency 降序补足。

同一层内按 urgency 降序、due 升序、entry 升序稳定排序。服务端返回 `reasons`，前端不根据字段再次推测理由。

任务行展示：

- task slug；没有 slug 时使用现有 task reference fallback；
- title；
- project 名称或 slug；
- 主理由、priority、due；
- 桌面端快捷动作；
- projected/materialized occurrence 沿用现有任务引用和 mutation 语义。

任务行交互：

- 点击任务主体进入 project-scoped task detail route，并记录 return state，使返回后仍停在首页原滚动位置。
- “开始/停止”复用现有 task mutation；不在首页重写生命周期规则。
- “完成”需要 optimistic pending 状态，成功后从列表移除并刷新四个计数；失败时恢复原行并显示后端错误。
- 首页不提供删除、跳过、改负责人、改日期和批量操作；这些操作进入任务详情或 `/my-tasks`。
- 所有快捷按钮必须阻止任务行点击冒泡，并有独立 `aria-label`。

空状态：

```text
今天没有需要优先处理的任务
可以查看全部任务，或在一个项目中创建新任务。
[查看我的任务] [新建任务]
```

如果用户仍有 open task，但没有命中前四类，服务端用 urgency 最高的任务补足，因此空状态只在确实没有 open task 时出现。

### 9.4 “项目关注”

这个区块不叫“我的项目”，因为当前没有 project membership 或 favorite 模型。

服务端从当前身份有权限读取的 active/planning 项目中选择最多 5 个：

1. 有逾期普通任务或逾期循环实例的项目；
2. 有高优未完成任务的项目；
3. 有等待已到期或未分配任务的项目；
4. 同风险等级按 `modified_at` 倒序；
5. 如果没有风险项目，展示最近更新的 3 个 active/planning 项目。

项目卡展示：

- name，空时回退 slug；
- 完成比例；
- 逾期、高优、等待已到期、未分配中非零的指标；
- 最近一条 project annotation 摘要；没有 annotation 时显示最近更新时间，不编造“进展正常”；
- archived/cancelled 项目不进入首页关注区，但仍可在项目列表查看。

风险计数复用现有 `ProjectTaskSummary` 语义：`overdue_count`、`high_priority_open_count`、`wait_ready_count`、`unassigned_open_count` 已统计普通任务和已物化循环实例；`series_metrics` 只提供循环任务子集和系列运行上下文。排序直接使用 `overdue_count`，不能再加 `overdue_recurring_occurrence_count` 造成重复计数。卡片有逾期循环实例时单独显示“其中循环实例逾期 N”，首页不得新定义第三套项目统计口径。

交互：

- 点击卡片主体进入项目 Overview。
- 点击逾期、高优、等待、未分配指标进入项目 Tasks 并带相应筛选条件。
- 指标链接只表达筛选条件，不承诺列表当前页条数与摘要计数完全相等；两者必须使用同一业务口径后才可显示“共 N 条完全一致”。
- 首页不在项目卡内提供状态流转、项目编辑或新建任务，避免卡片变成缩小版项目工作台。

### 9.5 “工作区信息”

沿用 `ConfigDefinition.show_on_console_home` 设计：

- 只展示 workspace 可解析的 effective value；project-only key 不展示。
- 主文本使用 `definition.label || key`，raw key 是次要文本。
- secret 只展示后端脱敏值或“已设置”，不提供 reveal。
- 没有任何可展示配置时整块隐藏，不显示“没有标记为首页展示的配置”。
- 有 config write 能力的用户看到“管理工作区信息”链接并进入 `/settings`；其他用户只读。
- 加载失败不阻断“我的今日”和“项目关注”，只在本区显示弱错误与重试。

`show_on_console_home` 的含义从“显示在运维概览”明确为“显示在用户首页的工作区信息”。管理员应只开启对普通成员有阅读价值的字段。

### 9.6 从首页移除的内容

| 当前内容 | 新位置 | 理由 |
|---|---|---|
| 当前操作者、Token 类型、Scope | AppShell 左侧身份块、令牌详情页 | 已有稳定位置，首页重复 |
| workspace 全部任务数 | `/my-tasks`、项目列表 | 全局总数不能指导个人下一步 |
| workspace 项目总数 | `/projects` | 列表入口比孤立数字更有价值 |
| 失败投递数与表格 | `/notifications`、`/hooks` | 属于运维闭环 |
| 最近审计 | `/audit` | audit 不是业务动态 |
| 配置空状态和“去配置定义”强提示 | `/settings` | 普通用户不需要被首页催促维护控制面 |

## 10. 系统身份首页

`actor_type=tenant_access_token` 没有个人 assignee 语义，不能显示空的“我的今日”后让用户误以为 workspace 没有任务。

系统身份访问 `/` 时展示：

```text
+--------------------------------------------------------------------+
| 首页                                                               |
+--------------------------------------------------------------------+
| 当前为系统身份                                                     |
| 系统身份没有个人任务。若要处理分配给成员的任务，请切换为成员身份。 |
|                                                                    |
| [查看项目] [成员] [Hook] [通知] [审计] [设置]                     |
+--------------------------------------------------------------------+
| 项目关注                                                           |
| ...与普通首页相同的权限内项目态势...                               |
+--------------------------------------------------------------------+
| 工作区信息                                                         |
| ...show_on_console_home effective config...                        |
+--------------------------------------------------------------------+
```

快捷入口按现有权限显示。系统身份页可以服务管理动作，但不恢复旧首页的 audit 和 delivery 表格。

## 11. 首页摘要 API

### 11.1 路由

```http
GET /api/v1/home
```

使用当前认证上下文解析 workspace 和 actor，不接受另一个 user id，避免越权查询他人的“我的今日”。workspace 切换仍使用现有有效 workspace 机制。

### 11.2 响应示例

```json
{
  "generated_at": 1784246400,
  "today": "2026-07-17",
  "actor_type": "user",
  "my_work": {
    "open_count": 17,
    "started_count": 1,
    "overdue_count": 3,
    "due_today_count": 2,
    "high_priority_open_count": 4,
    "items": [
      {
        "task": {
          "id": "8d53...",
          "uuid": "8d53...",
          "task_slug": "DEM-18",
          "title": "完成首页信息架构 spec",
          "status": "pending",
          "project": "marketing-automation",
          "priority": "H",
          "due": 1784303999,
          "start": 1784240000,
          "assignees": [
            {
              "id": "user-uuid",
              "name": "zhangsan",
              "display_name": "张三",
              "email": null,
              "external_ids": []
            }
          ]
        },
        "reasons": ["started", "due_today", "high_priority"]
      }
    ]
  },
  "project_attention": [
    {
      "project": {
        "id": "project-uuid",
        "slug": "webhook-stability",
        "name": "Webhook 稳定性",
        "status": "active",
        "task_count": 19,
        "pending_count": 7,
        "completed_count": 12,
        "modified_at": 1784240000
      },
      "overdue_count": 2,
      "high_priority_open_count": 1,
      "wait_ready_count": 0,
      "unassigned_open_count": 0,
      "series_metrics": {
        "recurring_series_count": 2,
        "active_recurring_series_count": 1,
        "open_recurring_occurrence_count": 3,
        "overdue_recurring_occurrence_count": 1
      },
      "latest_update": {
        "id": "annotation-uuid",
        "content": "回调域名已经确认",
        "created_by": {
          "type": "user",
          "user": {
            "id": "user-uuid",
            "name": "lisi",
            "display_name": "李四",
            "email": null,
            "external_ids": []
          }
        },
        "created_at": 1784230000
      }
    }
  ]
}
```

workspace information 继续使用现有：

```http
GET /api/v1/config/effective?console_home=true
```

不把配置塞进 `/api/v1/home`，原因是它已经有独立权限、secret 脱敏、缓存和失败边界；首页应允许个人任务成功而配置区单独失败。

`today` 由服务端按任务查询使用的同一 location 生成，格式固定为 `YYYY-MM-DD`。前端只把它本地化为标题日期，不用浏览器日期覆盖该值，也不使用它重新计算任务分类。

### 11.3 用户身份输出

首页新增的所有用户引用都遵守仓库统一规范：

- task `assignees[]` 使用 `task.JSONUserInfo`；
- project annotation `created_by` 使用 `task.JSONActorInfo`，其 `user` 为 `task.JSONUserInfo`；
- 字段使用 `id`，不使用 `user_id`，不输出裸 UUID；
- App 层通过 `resolveUserInfos` 批量解析，Storage 层只返回原始 UUID；
- 未找到用户时 fallback `{ID: id, Name: id}`。

### 11.4 聚合边界

- `internal/storage` 提供 workspace 范围的集合查询，不允许 HTTP handler 循环每个项目调用 `ProjectTaskSummary` 造成 N+1。
- `internal/app` 负责当前用户解析、权限、日期边界、urgency、项目排序和 `UserInfo` 补全。
- `internal/httpapi` 只做参数、响应 DTO 和错误映射，不复制首页排序规则。
- Web 只渲染服务端返回的计数、顺序和 reasons，不重新计算业务口径。
- SQLite 与 PostgreSQL 必须返回一致结果；所有查询使用参数绑定并保持零 CGO。

## 12. 权限与可见性

`GET /api/v1/home` 按区块降级，而不是因为一个低权限区块让整个首页 403：

| 能力 | 表现 |
|---|---|
| task read + user actor | 返回 `my_work` |
| 无 task read | `my_work=null`，前端不渲染个人任务区 |
| project read | 返回 `project_attention` |
| 无 project read | `project_attention=[]`，前端不渲染项目区 |
| config read | 前端请求 workspace information |
| 无 config read | 不请求或将 403 解释为区块不可见，不显示错误 |
| task write | 显示新建、开始/停止、完成 |
| 只有 task read | 任务行可打开，所有写动作隐藏 |

首页不能通过计数、task reference、项目 annotation 或 config value 泄露当前 token scope / project scope 之外的数据。

## 13. 加载、错误与刷新

### 13.1 首次加载

- 标题和问候立即渲染。
- “我的今日”和“项目关注”使用与最终布局等高的 skeleton，避免页面跳动。
- 工作区信息单独加载，不阻塞上方内容。

### 13.2 部分失败

- `/api/v1/home` 失败：保留页面标题，显示“首页暂时无法加载”与重试按钮，不回退展示旧运维首页。
- config effective 失败：只影响工作区信息区。
- 写操作失败：恢复对应任务行，展示后端错误；不把整页切换成 error state。

### 13.3 刷新与缓存

- 首页 query key 使用 `['home', workspaceSlug, actor.id]`。
- workspace、actor 或 acting/tenant-switch 身份变化时不能复用旧首页缓存。
- 全局刷新按钮失效首页和 config effective query。
- 任务开始、停止、完成、创建成功后，同时失效首页、`my-tasks` 和对应 project summary。
- 项目更新、状态变化或 annotation 写入成功后，同时失效首页和对应 project query。
- 首页 query 使用 `staleTime=30s` 和 `refetchOnWindowFocus=true`；不做实时轮询。

## 14. 响应式与可访问性

- 桌面端主内容最大宽度为 `1280px` 并居中，任务区占整行，项目关注与工作区信息按 `2:1` 分栏。
- 小于 `lg` 时改为单列；计数卡允许两列换行。
- 任务标题、project 名称和 annotation 摘要必须允许换行或截断，不能挤压动作按钮。
- 风险不能只依赖红、黄颜色；必须有“逾期”“高优”等文字。
- 所有卡片可点击区域、快捷按钮、计数链接必须可通过键盘聚焦。
- 行内动作有明确 `aria-label`，焦点态不能依赖 hover。
- skeleton 使用 `aria-busy`；错误与 mutation 结果使用现有可访问 toast/alert 机制。
- 日期和数字使用当前 locale；不在 UI 暴露 Unix 时间戳。

## 15. 文案

使用面向任务的中文，不使用运维术语：

| 位置 | 文案 |
|---|---|
| 导航 | 首页 |
| 主区块 | 我的今日 |
| 任务入口 | 查看全部任务 |
| 项目区块 | 项目关注 |
| 配置区块 | 工作区信息 |
| 无任务 | 今天没有需要优先处理的任务 |
| 系统身份 | 当前为系统身份；系统身份没有个人任务 |

不使用“控制面状态”“资源总览”“运行配置”“租户态势”等只对实现者或运维人员有意义的标题。

## 16. 测试与验收

### 16.1 App / Storage

- user actor 只聚合分配给自己的 open tasks。
- started、overdue、due today、high priority 计数允许重叠且口径正确。
- date-only due 使用服务端本地日末，跨时区测试不由浏览器重算。
- waiting 与 pending 都属于 open；completed/deleted 不进入首页。
- materialized/projected occurrence 保持现有可见性和引用语义。
- 重点任务排序符合 started、overdue、today、high priority、urgency 顺序且稳定。
- 项目关注只包含权限内 active/planning 项目，风险排序与无风险 fallback 正确。
- 项目聚合使用集合查询，SQLite/PostgreSQL 结果一致。
- 用户引用解析为完整 `task.UserInfo` / `task.ActorInfo`，不输出裸 UUID。

### 16.2 HTTP

- `/api/v1/home` 在 user、tenant access token、project-scoped token 下返回正确裁剪结果。
- 无 task read 时 `my_work=null`；无 project read 时 `project_attention=[]`。
- 不允许通过 query 指定另一个用户。
- OpenAPI schema 包含 `reasons` 枚举和完整 user 对象。
- 首页响应不包含 secret config；配置仍由 effective endpoint 脱敏。

### 16.3 Web

- 首页不再请求 audit 和 notification delivery。
- 首页不再显示操作者、Token 类型、Scope、失败投递、最近审计。
- 普通用户看到问候、我的今日、项目关注和有数据时的工作区信息。
- 点击计数进入正确 `/my-tasks` 预设。
- 点击任务进入 project-scoped detail，返回后恢复首页位置。
- 完成任务成功后从列表移除并刷新计数；失败后恢复。
- 没有 `show_on_console_home` 配置时整块隐藏。
- tenant access token 看到系统身份降级页，不看到假的个人任务空状态。
- read-only 身份看不到新建和任务写操作。
- 窄屏单列可用，按钮可键盘访问。

### 16.4 完成前验证

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

## 17. 验收标准

1. user actor 登录后 `/` 首屏首先展示个人任务，而不是身份、投递或审计。
2. 首页重点任务能直接打开、开始/停止和完成，完整编辑仍进入任务详情或 `/my-tasks`。
3. 所有首页计数来自权限内全量数据，不受 `limit=200` 影响。
4. 项目关注使用权威 workspace 聚合，不产生按项目 N+1 请求。
5. `show_on_console_home` 数据仍按 label/value 展示，secret 不泄露，无数据时不出现运维式空提示。
6. 系统身份有明确降级首页，不伪造个人任务，也不恢复旧运维表格。
7. 首页新增用户字段全部符合 `task.UserInfo` / `task.JSONActorInfo` 输出规范。
8. SQLite、PostgreSQL 与 `CGO_ENABLED=0` 测试和构建全部通过。

## 18. 后续但不进入本次

只有真实产品需求出现后，才单独设计以下能力：

- 个人收藏和最近访问项目；
- 跨项目业务 Inbox、@mention 和已读状态；
- 用户自定义首页 widget；
- 项目负责人、项目成员或关注关系；
- 按角色或权限折叠全局导航；
- workspace 级管理态势页。

这些能力需要独立数据模型或信息架构，不能用 localStorage、audit 或现有投递记录临时模拟。
