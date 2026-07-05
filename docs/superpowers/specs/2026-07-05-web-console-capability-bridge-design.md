# Web Console 能力桥接：从项目工作台走向日常协作与管理闭环

**日期：** 2026-07-05
**状态：** 已实施
**范围：** `web/` 前端整体（Shell 信息架构、任务中心、资源管理、项目配置、成员身份）
**前置依赖：** 2026-07-05 项目工作台首页重构已完成

## 0. 产品 / CIO 评审结论

方向正确，但原稿的定位需要收窄：Xuanchu Web Console 不应被包装成「轻量 Linear/Jira」。它的核心价值不是替代专业项目管理套件，而是把已经存在的 HTTP / MCP / CLI 能力桥接到浏览器，让普通成员能完成个人任务闭环，让 owner/admin 能完成 workspace 日常治理，不再因为缺 UI 退回 CLI。

本次评审后的关键调整：

1. **定位从「项目管理套件」改为「任务协作入口 + 治理控制台」。** 继续坚持 project-first，不做看板、甘特、复杂报表和组织级 PMO 流程。
2. **Shell 职责重新划分。** 左侧栏负责全局导航、workspace 和当前登录身份；主内容区顶栏左侧只承载当前子应用标题、面包屑、页内 tab 和局部操作，不再放 actor/token 信息。
3. **按业务闭环排优先级。** P0 是「我的任务」和 Hook 运维闭环；P1 是项目设置、成员外部身份和审计可用性；P2 才是 Workspace / Notification 等低频控制面。
4. **严格区分已有后端能力和后端缺口。** 当前已确认有 project config、project annotation add/list/delete、user external-id bind/unbind、hook delivery replay、task urgency；也确认 project annotation 暂无 PATCH、audit 暂无 actor/action/time 服务端筛选、task restore 和 workspace unarchive 暂无后端能力。
5. **保持可运维、可审计、可回滚。** 每个 phase 必须能独立发布，并复用现有 `/api/v1/*`、authz、CSRF、`task.UserInfo` 和 closed-project 规则。

## 背景

项目工作台首页重构已落地：`ProjectTaskToolbar` / `TaskCreateDialog` / `AssigneeWorkloadSummary` 已挂载，`TaskQuickCreate` 已删除。当前项目内的读写闭环完整：任务全字段创建/编辑/状态流转/删除/annotation/link、项目编辑/状态流转/成员管理（workspace 级）。

但站到「普通用户能否在 web 上完成日常工作」「owner/admin 能否在 web 上管理好 workspace」两个高度看，console 仍是断的：

1. **没有跨项目的「我的任务」入口**。`/tasks` 已下线，任务只能从项目进入。后端 `GET /tasks` 支持 assignee 过滤，前端没用。
2. **资源页全是只读 DataTable**。`/hooks` `/audit` `/notifications` `/workspaces` `/settings` 共用同一个 `ResourcePage`——纯只读，顶部「搜索/筛选」按钮是装饰品。Hook 的 CRUD / enable / deliveries / replay 后端全套都有，web 完全管不了。
3. **Project 配置无 UI**。后端 `GET/PUT/DELETE /projects/{ref}/config/{key}` 完整，前端零覆盖；`project-settings-dialog` 只能改 slug。
4. **Project annotation 只读**。后端 `POST /projects/{ref}/annotations` 有，前端只在 activity 区展示 `recent_annotations`，没有创建/删除入口。任务侧有完整 annotation 编辑器，项目侧没有，体验不对称。
5. **成员详情页是只读快照**。`detailReadonlyHint`，external_ids 只展示 Badge，没有「绑定/解绑外部 ID」按钮。SSO 场景下身份映射修不了，得退回 CLI。
6. **看不到已删除任务**。后端 task 软删进 deleted 列表，前端无入口（且后端无 restore，是双层缺）。
7. **urgency 在 web 不可见**。后端 `GET /tasks/{ref}/urgency` 有，前端没用。
8. **项目列表行操作贫乏**。`project-row-actions` 只有「打开」+ slug，无法在列表页归档/取消/改状态。

一句话：**单一对象的读写闭环已经做好，但「跨对象编排」和「管理流」仍断裂**。普通用户缺个人任务入口，owner/admin 的管理动作不完整，被迫退回 CLI。这正是 console 作为浏览器控制面的价值区。

## 目标

把 web console 从「单项目工作台」推进为「任务协作入口 + 治理控制台」：普通用户在 web 上能完成个人任务闭环，owner/admin 在 web 上能完成日常管理而不必退回 CLI。

具体：

1. **Shell 归位**：左侧栏底部展示当前登录身份、角色、凭证类型和退出入口；主内容顶栏左侧改为页面上下文区。
2. **新增「我的任务」全局入口**：跨项目聚合分配给当前用户的任务，支持状态/截止/项目维度筛选，作为 console 首页候选，但不立即替换默认首页。
3. **资源页按闭环可写化**：Hook / Audit / Workspaces / Notifications 等管理页从只读 DataTable 升级为专用控制台；优先让 Hook 完成 CRUD + enable/disable + deliveries + replay。
4. **补齐项目级管理 UI**：项目配置（config key/value）读写、项目 annotation 创建/删除、项目列表行内状态流转。
5. **成员身份管理可写**：成员详情页支持绑定/解绑 external-id，与 SSO 配置和通讯录同步形成闭环。
6. **可见性补齐**：已删除任务视图、任务 urgency 展示。
7. **保持边界**：所有改动优先复用现有 HTTP / app service / MCP 能力；除非明确进入后端 milestone，否则不新增后端业务逻辑，不改变权限判定、`task.UserInfo` 语义、closed-project 禁写规则。

## 非目标

- **不做后端新能力**：批量操作 API、task restore、workspace unarchive 需后端先动，本 spec 不涵盖；仅在「未来后端就绪」处标注对接点。
- **不做完整 Linear/Jira 替代品**：不做组织级 PMO 流程、看板 / 拖拽 / 甘特、复杂报表和工作流自定义；保持表格为主、轻量交互。
- **不重写已稳定的模块**：项目工作台首页、任务详情页 annotation/link 编辑器、token 管理对话框不在重构范围，仅做最小增量。
- **不改变导航骨架的权限模型**：SSO 菜单仍 owner/tenant 可见，新增「我的任务」对所有可登录角色可见。
- **不引入新 UI 库**：继续 shadcn/ui + lucide-react + 现有 `@/components/markdown`。

## 信息架构

### Shell 职责划分

Web Console 的 Shell 分为三层职责：

1. **左侧栏：全局导航 + workspace + 当前登录身份。** 它回答「我现在在哪个 workspace、以什么身份、可以进入哪些控制面」。
2. **主内容顶栏：当前页面上下文。** 它回答「当前子应用是什么、面包屑在哪里、这一页有哪些局部操作」。这里不再放当前用户身份。
3. **主内容区：具体工作流。** 每个页面只处理自己的业务闭环，避免把全局身份、跨资源入口和局部表格动作混在一起。

### 左侧栏调整

当前 `navItems` 顺序：overview / projects / workspaces / members / tokens / sso / hooks / notifications / audit / settings。

调整为按「个人工作流 / 管理 / 系统」分组，新增「我的任务」，并把当前登录身份固定在底部：

```text
┌────────────────────────────────────────┐
│  xuanchu                               │
│  workspace: dajee        [切换/查看]   │
├────────────────────────────────────────┤
│  个人                                  │
│   概览              /                  │
│   我的任务          /my-tasks   [新]   │
│   项目              /projects          │
│                                        │
│  管理                                  │
│   成员              /members           │
│   Token             /tokens            │
│   集成 / Hook       /hooks             │
│   通知              /notifications     │
│   SSO               /sso   [owner]     │
│                                        │
│  系统                                  │
│   Workspace         /workspaces        │
│   审计              /audit             │
│   设置              /settings          │
├────────────────────────────────────────┤
│  Alice Chen                            │
│  alice · admin · PAT                   │
│  风险：normal          [退出] [主题]   │
└────────────────────────────────────────┘
```

- 「我的任务」对所有可登录用户可见，是 console 的个人任务中心。
- 「项目」即原 `/projects`，路由不变。不命名为「我的项目」，避免误导为仅显示我参与的项目；如果后续做 project membership/filter，再单独改名。
- 分组用小标题，仅视觉分组，不引入折叠，避免低频管理入口被藏起来。
- 当前登录身份使用 `/api/v1/credentials/current` 返回的 `actor.id/name/display_name/email`、`effective_role`、`token.type` 和 `effective_workspace`。tenant actor 显示为「系统身份」，acting/tenant-switch 显示 amber 风险状态。
- 退出、返回超管、主题、语言等全局操作优先放在底部身份区；在窄屏抽屉中也保持同样的底部身份块。
- 现有 `RiskBadge` 不再单独占据底部，而是并入身份块。

### 主内容顶栏调整

当前 `AppShell` 顶栏左侧显示 workspace、actorName 和 tokenType，这会占用最适合承载子应用上下文的位置。调整为：

```text
┌────────────────────────────────────────────────────────────┐
│ 项目 / proj-a / 设置                         [刷新] [...] │
└────────────────────────────────────────────────────────────┘
```

- 左侧展示当前页面标题、面包屑、页内 tab 或局部状态（例如「项目 / proj-a / 设置」「集成 / Hook」「我的任务」）。
- 右侧展示刷新、页面级快捷动作和少量全局小按钮；全局身份不在这里重复展示。
- acting / tenant-switch 仍可在顶栏用轻量 banner 或 amber 背景提示，但具体 actor/token 信息在侧栏底部展开。
- `AppShell` 需要增加可选 `headerTitle` / `breadcrumbs` / `headerActions` 之类的插槽，避免每个页面自己绕过 Shell。

### 路由新增

| 路由 | 组件 | 说明 |
|---|---|---|
| `/my-tasks` | `MyTasksPage` | 跨项目「我的任务」聚合页 |
| `/tasks/:taskRef` | 复用 `TaskDetailPage`（脱离项目上下文的任务详情） | 从「我的任务」点击进入 |
| `/projects/:projectSlug/settings` | `ProjectSettingsPage`（独立页，承载 config + annotation 管理） | 从项目 header 「设置」入口进 |

任务详情页 `TaskDetailPage` 已存在（`project-workbench/task-detail/task-detail-page`），需解耦对 project 路由参数的强依赖：当从 `/my-tasks` 进入时，通过 task 自身的 `project` 字段回链项目，而不是从 URL 取 project slug。

## 设计

### 1. 我的任务（My Tasks）

#### 数据策略

后端 `GET /api/v1/tasks?assignee=me&status=pending` 已支持。前端新增 `my-tasks-api.ts`：

```ts
// 复用 workspaceApiGet，路径带 effective workspace
GET /api/v1/tasks?assignee=<myUserId>&status=<status>&due_before=<ts>&project=<slug>
```

- 不引入新端点；assignee 用 `/api/v1/credentials/current` 的 `actor.id`。前端当前 `MeResponse` 类型需要补齐 `actor.id/display_name/email/external_ids`，不要只靠 `actor.name`。
- 排序复用后端 `sort=due|next|priority|entry`。
- 截止维度筛选编译成 `due_before` / `due_after` / `due_empty`。
- tenant actor 没有自然人「我的任务」语义：tenant/system 身份进入时页面显示空状态和解释，不伪造 assignee。

#### 页面原型

```text
我的任务                                              [刷新]
═══════════════════════════════════════════════════════════
[全部 42] [待开始 12] [进行中 8] [今日到期 3] [逾期 2] [无截止 17]
─────────────────────────────────────────────────────────
🔍 搜索...   [状态▾] [优先级▾] [项目▾] [到期▾]    排序: [到期▾]
─────────────────────────────────────────────────────────
☐  TASK-142  修复登录回调死循环          高   🔴  今天    proj-a
☐  TASK-138  导出 xlsx 列顺序错误        中   🟡  明天    proj-a
▸  TASK-121  设计 SSO 身份映射方案       高   ⚪  3天后   proj-b
☐  TASK-119  补 e2e: 三入口字段审计      低   🟢  —       proj-b
☐  TASK-115  文档：成员 external-id       低   ⚫  逾期 2天 proj-c
─────────────────────────────────────────────────────────

   逾期 2 项 · 今日到期 3 项 · 进行中 8 项
```

要点：

- **顶部 tab 条**是「快速视图」，等价于一组预设 filter：点击「今日到期」即设置 `due_before=<今天23:59>` + `status=pending`。tab 上的数字实时反映当前筛选下的计数。
- **项目列可点击**，跳转到该项目工作台；点击任务行进入 `/tasks/:taskRef` 详情。
- **状态/优先级/项目/到期** 都是 toolbar 下拉，与项目工作台的 `ProjectTaskToolbar` 视觉一致，但筛选模型独立（不复用 `project-filter.ts`，因为没有 project 上下文）。
- **页脚摘要**替代项目工作台的 `AssigneeWorkloadSummary`——这里视角是「我」，摘要变成「我的负载概览」。

#### 组件边界

- `pages/my-tasks-page.tsx`（新建于 `web/src/pages/`）：页面壳 + tab + toolbar + table 编排。
- `features/workspace/my-tasks/my-tasks-api.ts`：fetch + filter 编译。
- `features/workspace/my-tasks/my-task-tabs.ts`：tab → filter 映射，纯函数易测。
- 复用 `TaskTable`，但不能把 project-workbench 的项目上下文硬塞进全局页；先抽出可复用的表格内核，再由项目页和我的任务页各自提供列、空状态和 row action。
- `AppShell` 增加 header slot 后，`MyTasksPage` 用它设置标题「我的任务」和刷新动作，不在页面内再造一套顶栏。

#### 任务详情解耦

`task-detail-page.tsx` 当前从 `useSearch()` 取 `projectSlug`。改为：

1. 优先从 URL 取（项目内进入）。
2. 兜底从 task 对象的 `project` 字段取（`/my-tasks` / `/tasks/:ref` 进入）。
3. 都没有时，详情页只展示任务本身，不渲染项目相关动作（如「回到项目」链接隐藏）。

### 2. 资源页可写化（Resource Console）

当前 `ResourcePage` 是「一个 DataTable 吃所有资源」的退化设计。升级方向不是一次性把所有资源都做成复杂工作台，而是按资源的业务闭环分发到专用控制台组件：高频且有明确操作闭环的资源先做，低频资源保留只读 fallback。

#### 架构

```text
ResourceRoute(page)
   └─ switch(page)
       ├─ "hooks"        → HookConsole
       ├─ "audit"        → AuditConsole
       ├─ "workspaces"   → WorkspaceConsole
       ├─ "notifications"→ NotificationConsole
       └─ default        → ResourcePage (旧只读 DataTable)
```

`resource-config.tsx` 退化为只提供列定义给 fallback；专用资源走自己的 feature 目录。

#### 2.1 Hook Console（P0，最高优先）

后端能力已确认：

- `/api/v1/hooks` CRUD
- `POST /api/v1/hooks/{hookID}/enable`
- `POST /api/v1/hooks/{hookID}/disable`
- `GET /api/v1/hooks/{hookID}/deliveries`
- `GET /api/v1/hook-deliveries/{deliveryID}`
- `POST /api/v1/hook-deliveries/{deliveryID}/replay`

```text
集成 / Webhook                                [+ 新建 Hook]
═══════════════════════════════════════════════════════════
名称                  目标事件              状态   最近投递
task.done            https://acme.com/hook  启用   2分钟前 成功
project.archived     https://acme.com/arc   暂停   3小时前 失败 ⚠
─────────────────────────────────────────────────────────
点击行展开 ↓
┌─ task.done · https://acme.com/hook ─────────────────────┐
│  [编辑] [启用/暂停] [测试投递] [删除]                    │
│                                                          │
│  最近投递 (5)                          [刷新]            │
│  ┌──────────────┬────────┬─────────┬───────────┐        │
│  │ 时间          │ 状态   │ 耗时    │ 操作       │        │
│  │ 14:02:11      │ 200    │ 120ms   │ [重放]     │        │
│  │ 13:58:40      │ 500    │ 3000ms  │ [重放]     │        │
│  └──────────────┴────────┴─────────┴───────────┘        │
└──────────────────────────────────────────────────────────┘
```

- 行展开后内嵌投递历史 + 重放按钮，避免跳页。
- 新建/编辑 Hook 弹窗复用现有 dialog 模式（参考 `tokens/token-create-dialog` 风格）。
- 失败投递用 ⚠ 标记，便于排查。

#### 2.2 Audit Console（P1）

后端 `GET /api/v1/audit` 当前只支持 `limit` 与 `project/project_id`。因此本 spec 内 Audit Console 的第一版以「可读、可导出、可按当前页数据筛选」为目标；actor/action/time range 属于后端能力缺口，不在本 spec 悄悄承诺。

```text
审计日志                                    [导出 CSV]
═══════════════════════════════════════════════════════════
🔍 搜索当前结果...  [project▾] [action▾*] [actor▾*] [时间范围▾*]
─────────────────────────────────────────────────────────
2026-07-05 14:02  alice  task.modify    TASK-142  改 due
2026-07-05 13:58  bob    task.done      TASK-138
2026-07-05 13:30  alice  project.transition  proj-a  active→archived
2026-07-05 11:12  carol  member.role    user-x   member→admin
─────────────────────────────────────────────────────────
[加载更多]
```

- `project` 和 `limit` 走服务端参数。
- `actor/action/time range` 在后端未支持前，只能筛选当前已加载结果，UI 必须标明「筛选当前结果」，不能误导为全量审计搜索。
- 支持导出当前已加载 / 当前筛选结果为 CSV（前端拼装）。
- 如果要做全量 actor/action/range 审计搜索，必须先开后端 spec，扩展 `AuditListInput` 与存储查询。

#### 2.3 Workspace Console（P2）

后端 `/api/v1/workspaces` CRUD + `/archive`。当前只读列表。

```text
Workspace                                 [+ 新建 Workspace]
═══════════════════════════════════════════════════════════
slug      名称         成员  项目  状态    操作
acme      ACME 主区    12    8     活跃    [设置] [归档]
sandbox   沙盒         3     2     归档    [恢复*] [设置]
─────────────────────────────────────────────────────────
* 恢复依赖后端 unarchive（当前未实现，按钮置灰 + tooltip 说明）
```

- workspace 级管理对 owner/tenant 才可见。
- 归档/恢复按后端实际能力开放；未实现的（unarchive）按钮置灰并带说明，不静默失败。

#### 2.4 Notification Console（P2）

当前没有 `/api/v1/notifications` 这种单一通知收件箱。现有能力分为四类：

- `/api/v1/notification-sinks`
- `/api/v1/reminder-rules`
- `/api/v1/notification-rules`
- `/api/v1/notification-deliveries`

因此 Notification Console 应定位为「通知管控台」，不是用户消息中心。第一版可以把现有只读 `notification-sinks` 表升级为 sink/rule/delivery 三段式管理；如果要做「已读 / 删除」类收件箱语义，需要另开后端设计。

### 3. 项目级管理补齐

#### 3.1 项目设置页（独立页）

当前 `project-settings-dialog.tsx` 只能改 slug。把项目级管理动作集中到独立页 `/projects/:slug/settings`，header 提供「设置」入口：

```text
‹ proj-a  设置
═══════════════════════════════════════════════════════════
基本信息
  名称:     [项目管理重构_____________]
  Slug:     [proj-a____________] [改名并跳转]
  描述:     [多行 markdown 编辑器_________]
            [保存]

状态
  当前: active   [规划] [活跃●] [归档] [取消]
                 （含二次确认弹窗）

危险操作
  归档项目 →  项目将变为只读，任务不可再编辑
  [归档]
═══════════════════════════════════════════════════════════
配置项 (Config Keys)                        [+ 新增配置]
  key                  value                操作
  review.required      true                 [编辑] [删除]
  sprint.length        14                   [编辑] [删除]
  notify.channel       #proj-a              [编辑] [删除]
─────────────────────────────────────────────────────────
项目备注 (Annotations)                      [+ 新增备注]
┌─ alice · 2026-07-05 14:00 ───────────────────────────┐
│  本项目审计需求见 SPEC-001，所有 due 修改必须留痕。   │
│                                                [删除]│
└────────────────────────────────────────────────────────┘
┌─ bob · 2026-07-04 ────────────────────────────────────┐
│  导入模板已更新到 v2。                                │
│                                                [删除]│
└────────────────────────────────────────────────────────┘
```

- 配置项对应后端 `/projects/{ref}/config/{key}` 的 GET/PUT/DELETE。
- 项目备注当前对应 `/projects/{ref}/annotations` 的 POST / GET / DELETE；后端暂不支持 PATCH，第一版只做新增和删除。
- 不做「删除 + 重建」伪编辑：那会改变作者、创建时间和审计语义。若需要编辑项目备注，必须先补后端 PATCH 能力。
- 「状态」区把 `project-status-menu` 的能力搬到设置页，header 仍保留快捷状态菜单不变。

#### 3.2 项目列表行内状态操作

`project-row-actions.tsx` 升级为下拉菜单：

```text
proj-a   活跃   8 任务   3 逾期        [⋯ ▾]
                                      ├─ 打开项目
                                      ├─ 设置
                                      ├─ 归档
                                      └─ 取消
```

- 归档/取消带二次确认。
- closed project 行显示「恢复到 active」（后端 transition 支持 archived→active，是状态字段变更，不是 unarchive）。

### 4. 成员身份管理补齐

成员详情页当前是只读快照。补齐为可写管理页：

```text
‹ 成员  alice
═══════════════════════════════════════════════════════════
身份
  姓名:    Alice Chen        [编辑]
  邮箱:    alice@acme.com
  角色:    admin             [改角色]   ← owner 才能改到 owner

外部身份 (SSO 映射)                         [+ 绑定外部 ID]
  feishu : ou_alice_001        [解绑]
  github : alice-c            [解绑]
  ── 无更多 ──

Token (3 活跃 / 1 已撤销)        [管理 Token →]

最近活动 (审计)
  2026-07-05  task.modify  TASK-142
  2026-07-04  task.done    TASK-130
  ...
```

- 「绑定外部 ID」弹窗：选 provider（feishu/github/...）+ 填 external_id，调 `POST /users/{ref}/external-ids`。
- 「解绑」调 `DELETE /users/{ref}/external-ids`。
- 这与 `/sso` 配置页形成闭环：SSO 配好同步后，映射不上的用户在此手工绑定。

权限：external-id 绑定需 `member:write`（owner/admin）。

### 5. 可见性补齐

#### 5.1 已删除任务视图

后端 task 软删后进 deleted 列表，但无 restore。前端在项目工作台 toolbar 的「更多筛选」加 `status=deleted` 快捷开关，并在表格中用灰显样式区分：

```text
☐ TASK-142  修复登录回调  (已删除 2026-07-04)   灰显
```

- 后端 restore 未实现时，前端只展示不可恢复，附 tooltip「恢复能力待后端支持」。
- 默认不展示已删除，需主动切换。

#### 5.2 Urgency 展示

在任务详情页属性面板新增「紧迫度」字段，调 `GET /tasks/{ref}/urgency`：

```text
┌─ 属性 ─────────────────────────────────────┐
│  状态      进行中                           │
│  优先级    高                               │
│  紧迫度    8.4   ▓▓▓▓▓▓▓▓░░                │
│            (优先级 +2.0 / 截止 +3.5 / ...)  │
│  到期      今天 23:59                       │
└────────────────────────────────────────────────┘
```

- 紧迫度数字 + 进度条可视化；展开显示各分项贡献（来自 urgency 解释接口）。
- 不在列表默认列展示（避免噪音），仅详情页可见；后续可在 toolbar 「列设置」中开启。

## 权限矩阵（汇总）

| 动作 | 普通成员 | admin | owner | 备注 |
|---|---|---|---|---|
| 我的任务（查看自己） | ✅ | ✅ | ✅ | |
| 任务读写（项目内） | ✅ | ✅ | ✅ | 受 closed project 限制 |
| 项目设置（config/annotation） | ❌ | ✅ | ✅ | `canProjectManage` |
| 项目状态流转 | ❌ | ✅ | ✅ | |
| Hook 管理 | ❌ | ✅ | ✅ | workspace 级，按 capability |
| Workspace 管理 | ❌ | ❌ | ✅（+ tenant） | |
| 成员管理（角色/external-id） | ❌ | ✅ | ✅ | owner 角色仅 owner 可改 |
| Audit 查看 | ✅（全） | ✅（全） | ✅（全） | 所有可登录成员可查看 workspace 全量 audit |
| SSO 配置 | ❌ | ❌ | ✅（+ tenant） | |

权限判定全部复用现有 `permissions.ts` 的 capability 检查，不新增判定逻辑。

## 组件落地清单（新增）

```text
web/src/
├── pages/
│   ├── my-tasks-page.tsx              [新]
│   └── project-settings-page.tsx      [新]
├── components/
│   ├── AppShell.tsx                   [改：nav 分组 + header slot + 侧栏身份块]
│   └── AppShell.test.tsx              [改：身份块、header slot、acting/tenant-switch]
├── features/workspace/
│   ├── my-tasks/
│   │   ├── my-tasks-api.ts            [新]
│   │   └── my-task-tabs.ts            [新]
│   ├── hooks/                          [新目录]
│   │   ├── hooks-api.ts
│   │   ├── hook-console.tsx
│   │   ├── hook-create-dialog.tsx
│   │   ├── hook-edit-dialog.tsx
│   │   └── hook-deliveries.tsx
│   ├── audit/                          [新目录]
│   │   ├── audit-api.ts
│   │   ├── audit-console.tsx
│   │   └── audit-filter.ts
│   ├── resources/
│   │   ├── resource-config.tsx        [改：退化为 fallback]
│   │   └── resource-dispatch.tsx      [新]
│   ├── project-workbench/
│   │   ├── project/
│   │   │   ├── project-config-editor.tsx      [新]
│   │   │   ├── project-annotations-editor.tsx [新]
│   │   │   └── project-row-actions.tsx        [改：加菜单]
│   │   └── task-detail/
│   │       └── task-urgency-panel.tsx         [新]
│   └── members/
│       ├── external-id-bind-dialog.tsx        [新]
│       └── members-page.tsx                   [改：去 readonly]
```

## 分阶段范围

为避免大爆炸式落地，分三阶段，每阶段独立可发布、可验证、可回退。

### Phase 1：Shell 归位 + 个人任务中心（解决最大体验断点）
- AppShell nav 分组 + 我的任务入口
- 侧栏底部当前登录身份块：actor、role、token type、workspace、风险状态、退出/返回超管
- 主内容顶栏改为页面上下文 slot，不再显示 actor/token 信息
- 「我的任务」页 + tab + toolbar
- 任务详情页解耦（支持 `/tasks/:ref` 入口）
- 已删除任务可见性（toolbar 开关）
- **验证：** 普通用户能在 web 上确认当前身份、看到自己跨项目的全部任务并完成状态流转；主栏顶栏左侧用于当前页面标题/面包屑。

### Phase 2：管理流可写化（owner/admin 价值区）
- Hook Console（CRUD + deliveries + replay）
- Audit Console（project/limit 服务端查询 + 当前结果筛选 + 导出）
- 项目设置页（config + annotation + 状态集中）
- 项目列表行内操作菜单
- 成员 external-id 绑定/解绑
- **验证：** owner/admin 在 web 上完成 webhook 配置、审计排查、身份映射修复，无需退回 CLI。

### Phase 3：体验增强
- Workspace Console（受后端 unarchive 能力限制，恢复按钮置灰）
- Notification Console（sink/rule/delivery 管控，不做消息收件箱）
- Urgency 展示（详情页）
- **验证：** 信息可见性补齐，console 覆盖当前后端已经具备的主要日常管理闭环。

## 测试策略

延续 spec/plan 驱动 + 红绿验证：

- 每个 Phase 单独出 implementation plan，含可执行小任务清单。
- 前端：组件单测（vitest）+ 页面级集成测试（MSW mock HTTP）。
- Shell：`AppShell.test.tsx` 必须覆盖身份块、退出/返回超管、SSO 菜单可见性、`/my-tasks` 与项目详情路由高亮。
- 后端：本 spec 不改后端，但新增前端调用必须对齐现有 `--json` / HTTP 契约测试。
- 验证命令：`pnpm --dir web test && pnpm --dir web typecheck && pnpm --dir web lint && pnpm --dir web build`（每阶段）。
- CLI/Go 未改时可不跑完整 Go 验证；若触碰 HTTP 契约、authz 或 storage，补跑 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`。
- E2E：在 Phase 1 完成后补一条「登录 → 确认侧栏身份 → 看到我的任务 → 改状态 → 验证列表更新」的 happy path，并保留现有 `pnpm --dir web run smoke:editing` 作为编辑流回归。

## 后端契约与缺口

当前仓库已确认的可复用契约：

- Project config：`GET/PUT/DELETE /api/v1/projects/{projectRef}/config/{key}`，以及 `GET /api/v1/projects/{projectRef}/config`。
- Project annotation：`POST/GET/DELETE /api/v1/projects/{projectRef}/annotations`，无 PATCH。
- User external IDs：`POST /api/v1/users/{user}/external-ids`，`GET /api/v1/users/{user}/external-ids`，`DELETE /api/v1/users/{user}/external-ids/{provider}/{externalID}`。
- Hook deliveries：`GET /api/v1/hooks/{hookID}/deliveries`，`GET /api/v1/hook-deliveries/{deliveryID}`，`POST /api/v1/hook-deliveries/{deliveryID}/replay`。
- Task urgency：`GET /api/v1/tasks/{taskRef}/urgency`。
- Audit：`GET /api/v1/audit` 支持 `limit` 与 `project/project_id`。

明确不在本 spec 内补的后端缺口：

- Project annotation PATCH。没有 PATCH 前不做伪编辑。
- Task restore。前端只做已删除可见性和恢复占位提示。
- Workspace unarchive。前端 Phase 3 按置灰处理。
- Audit actor/action/time range 服务端筛选。需要独立后端 spec。
- 用户消息收件箱式 notification read/delete。当前 notification 是 sink/rule/delivery 管控模型。

## 文档同步

落地后需更新：
- `README.md`：web console 章节补充「我的任务」「集成管理」「项目设置」入口说明。
- `ROADMAP.md`：记录 console 能力补齐 milestone。
- 本 spec 实施完成后状态改为「已实施」。

## 不做的事（再次强调）

- 不引入新 UI 库 / 新状态管理库。
- 不重写已稳定的 token 管理 / 任务详情 annotation 编辑器。
- 不改变权限模型与身份语义。
- 不在本 spec 内推动后端批量操作 / restore / unarchive。
