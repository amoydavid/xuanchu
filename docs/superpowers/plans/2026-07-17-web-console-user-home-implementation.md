# Web Console 用户首页 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Web Console 登录后的 `/` 从运维概览改成以“我的今日”为主、项目关注与工作区信息为辅的用户首页，并提供权威、权限感知的首页摘要 API。

**Architecture:** 新增 `storage.HomeRepository` 做 workspace 级项目风险与最新项目更新的集合查询，`app.Service.Home()` 复用 `QueryTaskViews` 生成当前用户全量任务摘要、排序重点任务并组装项目关注。HTTP 层新增 `GET /api/v1/home` 与 OpenAPI 契约；Web 层新增 home feature，保留 `OverviewPage` 导出兼容已有 `PageHeader` 引用，但彻底替换其内容。

**Tech Stack:** Go 1.25、GORM、SQLite/PostgreSQL、Huma/chi、React 19、TanStack Query/Router、Vitest、Testing Library、Tailwind CSS、shadcn/ui。

## Global Constraints

- SQLite 继续使用 `github.com/glebarez/sqlite`；不得引入 CGO SQLite 依赖。
- 所有首页全量计数在 App/Storage 层计算；Web 不从 `limit=200` 列表推断。
- 首页所有用户引用使用 `task.UserInfo` / `task.JSONUserInfo` 或 `task.ActorInfo` / `task.JSONActorInfo`，字段统一为 `id`。
- `name` 是稳定查找键，`display_name` 只用于展示。
- `overdue_count` 已包含普通任务和已物化循环实例；`series_metrics.overdue_recurring_occurrence_count` 是子集，不得重复相加。
- date-only `due/until` 使用服务端本地日末，`wait/scheduled` 使用服务端本地日初；浏览器不得重算任务分类。
- 首页不请求或展示 audit、Hook delivery、notification delivery、Token type 或 Scope。
- 项目关注不得通过逐项目调用 `ProjectTaskSummary` 产生 N+1。
- stdout/stderr、CLI、MCP、Remote 既有契约不变。
- 文档和注释以中文为主，用户文案同步维护 `zh-CN` 与 `en-US`。

---

### Task 1: Storage 集合聚合与 App 首页领域视图

**Files:**
- Create: `internal/storage/home_repo.go`
- Create: `internal/storage/home_repo_test.go`
- Create: `internal/app/home.go`
- Create: `internal/app/home_test.go`
- Modify: `internal/app/service.go`
- Modify: `docs/superpowers/specs/2026-07-17-web-console-user-home-design.md`

**Interfaces:**
- Produces: `storage.NewHomeRepository(db) *HomeRepository`
- Produces: `(*HomeRepository).ProjectMetrics(workspaceID string, projectIDs []string, now int64) (map[string]HomeProjectMetrics, error)`
- Produces: `(*HomeRepository).LatestProjectAnnotations(projectIDs []string) (map[string]ProjectAnnotation, error)`
- Produces: `(*app.Service).Home() (app.HomeView, error)`
- Produces: `app.HomeTaskReason` values `started|overdue|due_today|high_priority`

- [x] **Step 1: 写 Storage 失败测试**

在 `internal/storage/home_repo_test.go` 构造两个 active 项目、普通任务、已物化 occurrence、未分配任务和 project annotation，断言集合查询：

```go
func TestHomeRepositoryProjectMetricsBatch(t *testing.T) {
    // project-a: overdue H occurrence；project-b: wait ready unassigned task
    got, err := NewHomeRepository(store.DB()).ProjectMetrics(ws.ID, []string{a.ID, b.ID}, now)
    if err != nil { t.Fatal(err) }
    if got[a.ID].OverdueCount != 1 || got[a.ID].OverdueRecurringOccurrenceCount != 1 {
        t.Fatalf("project-a metrics = %#v", got[a.ID])
    }
    if got[b.ID].WaitReadyCount != 1 || got[b.ID].UnassignedOpenCount != 1 {
        t.Fatalf("project-b metrics = %#v", got[b.ID])
    }
}

func TestHomeRepositoryLatestProjectAnnotations(t *testing.T) {
    got, err := NewHomeRepository(store.DB()).LatestProjectAnnotations([]string{a.ID, b.ID})
    if err != nil { t.Fatal(err) }
    if got[a.ID].Content != "latest-a" || got[b.ID].Content != "latest-b" {
        t.Fatalf("latest = %#v", got)
    }
}
```

- [x] **Step 2: 运行 Storage 测试并确认 RED**

Run: `go test ./internal/storage -run 'TestHomeRepository' -count=1`

Expected: FAIL，原因是 `NewHomeRepository` / `HomeProjectMetrics` 尚不存在。

- [x] **Step 3: 实现 HomeRepository**

`internal/storage/home_repo.go` 定义：

```go
type HomeRepository struct { db *gorm.DB }

type HomeProjectMetrics struct {
    OverdueCount int
    HighPriorityOpenCount int
    WaitReadyCount int
    UnassignedOpenCount int
    OpenRecurringOccurrenceCount int
    OverdueRecurringOccurrenceCount int
}
```

实现要求：

- 一条 `tasks GROUP BY project_id` 查询计算四类风险，其中 open 只包含 `pending|waiting`。
- `overdue_count` 直接包含普通任务和已物化 occurrence。
- 同一条或第二条集合查询用 `series_id IS NOT NULL` 计算循环实例子集。
- `LatestProjectAnnotations` 一次读取目标项目 annotations，按 `project_id ASC, entry DESC` 排序后在 Go 中保留每项目第一条；不得逐项目查询。
- 空 `projectIDs` 直接返回空 map。

- [x] **Step 4: 运行 Storage 测试并确认 GREEN**

Run: `go test ./internal/storage -run 'TestHomeRepository' -count=1`

Expected: PASS。

- [x] **Step 5: 写 App 失败测试**

在 `internal/app/home_test.go` 使用 `FixedClock` 和真实 Store，覆盖：

```go
func TestHomeBuildsPersonalWorkAndProjectAttention(t *testing.T) {
    view, err := svc.Home()
    if err != nil { t.Fatal(err) }
    if view.Today != "2026-07-17" { t.Fatalf("today = %q", view.Today) }
    if view.MyWork == nil || view.MyWork.StartedCount != 1 || view.MyWork.OverdueCount != 1 {
        t.Fatalf("my_work = %#v", view.MyWork)
    }
    if len(view.MyWork.Items) == 0 || view.MyWork.Items[0].Reasons[0] != HomeTaskReasonStarted {
        t.Fatalf("items = %#v", view.MyWork.Items)
    }
    if len(view.ProjectAttention) == 0 || view.ProjectAttention[0].Project.Slug != "risk" {
        t.Fatalf("project_attention = %#v", view.ProjectAttention)
    }
}

func TestHomeTenantActorOmitsPersonalWork(t *testing.T) {
    view, err := tenantSvc.Home()
    if err != nil { t.Fatal(err) }
    if view.MyWork != nil { t.Fatalf("my_work = %#v, want nil", view.MyWork) }
}

```

另写两个独立测试：`TestHomeRespectsProjectScope` 创建 allowlist 内外各一个项目并断言响应只含 allowlist 项目；`TestHomeUsesCompleteUserInfo` 为 assignee 和 annotation actor 写入 display name、email、external ID，并逐字段断言首页 view 完整返回。

- [x] **Step 6: 运行 App 测试并确认 RED**

Run: `go test ./internal/app -run '^TestHome' -count=1`

Expected: FAIL，原因是 `Service.Home` 和首页 view types 尚不存在。

- [x] **Step 7: 实现 App 首页聚合**

`internal/app/home.go` 定义：

```go
type HomeTaskReason string
const (
    HomeTaskReasonStarted HomeTaskReason = "started"
    HomeTaskReasonOverdue HomeTaskReason = "overdue"
    HomeTaskReasonDueToday HomeTaskReason = "due_today"
    HomeTaskReasonHighPriority HomeTaskReason = "high_priority"
)

type HomeTaskItemView struct {
    Task TaskOccurrenceView
    Reasons []HomeTaskReason
}
type HomeMyWorkView struct {
    OpenCount, StartedCount, OverdueCount, DueTodayCount, HighPriorityOpenCount int
    Items []HomeTaskItemView
}
type HomeProjectAttentionView struct {
    Project ProjectView
    OverdueCount, HighPriorityOpenCount, WaitReadyCount, UnassignedOpenCount int
    SeriesMetrics ProjectSeriesMetricsView
    LatestUpdate *ProjectAnnotationInfo
}
type HomeView struct {
    GeneratedAt int64
    Today string
    ActorType string
    MyWork *HomeMyWorkView
    ProjectAttention []HomeProjectAttentionView
}
```

实现要求：

- `Service` 注入 `homeRepo`，事务 clone 同步重建。
- user actor 通过 `QueryTaskViews` + AST `assignee:<ActorUserID> and (status:pending or status:waiting)` 读取 materialized 全量 open task，`Sort=urgency`，`Limit=0`。
- 按 service location 的 `[todayStart,tomorrowStart)` 计算分类；同一任务可有多个 reason。
- 重点任务按 started、overdue、due_today、high_priority bucket，再按 urgency DESC、due ASC、entry ASC、id ASC；最多 8 条。
- tenant actor 的 `MyWork=nil`。
- 项目列表使用 `ListProjectsByStatus("open")` 并经过现有 project scope 过滤；`homeRepo` 一次聚合风险和 latest annotations。
- 项目排序按 overdue、high priority、wait ready、unassigned、modified DESC；有风险最多 5 个，无风险 fallback 最近 3 个。
- `projectViewFromRow` / `projectAnnotationInfoFromModel` / `resolveUserInfos` 复用现有逻辑。

- [x] **Step 8: 运行 App 测试并确认 GREEN**

Run: `go test ./internal/app -run '^TestHome' -count=1`

Expected: PASS。

- [x] **Step 9: 修正文档口径并提交 Task 1**

确认 spec 明确 `overdue_count` 已包含 materialized occurrence，运行：

```bash
git add internal/storage/home_repo.go internal/storage/home_repo_test.go internal/app/home.go internal/app/home_test.go internal/app/service.go docs/superpowers/specs/2026-07-17-web-console-user-home-design.md
git diff --cached --check
git commit -m "feat: 增加用户首页聚合"
```

---

### Task 2: 首页 HTTP API、权限降级与 OpenAPI

**Files:**
- Create: `internal/httpapi/home.go`
- Create: `internal/httpapi/home_test.go`
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/request_scope_test.go`
- Modify: `internal/httpapi/app_service.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes: `(*app.Service).Home() (app.HomeView, error)`
- Produces: `GET /api/v1/home`
- Produces: context-only authorization when both `RequiredCapability` and `RequiredPermission` are empty; this resolves actor/workspace/project scope but grants no resource operation by itself

- [x] **Step 1: 写 context-only 授权失败测试**

在 `request_scope_test.go` 断言空 required capability/permission 能解析合法 user/tenant token 的 workspace context，但后续 `svc.Require(PermissionTaskRead)` 仍按 role/scope 判定；无效 workspace 和非成员仍失败。

- [x] **Step 2: 运行授权测试并确认 RED**

Run: `go test ./internal/app -run 'TestAuthorizeTokenRequestContextOnly' -count=1`

Expected: FAIL，当前空 capability 会触发 `token_scope_denied`，空 permission 会触发 `permission_denied`。

- [x] **Step 3: 实现 context-only 授权**

只在 required 字段非空时执行检查：

```go
if input.RequiredCapability != "" && !scope.HasCapability(input.RequiredCapability) {
    return AuthorizedRequest{}, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "token scope denied"}
}
if input.RequiredPermission != "" {
    if err := requireRolePermission(runtime.Role, input.RequiredPermission); err != nil {
        return AuthorizedRequest{}, err
    }
}
```

tenant actor 同样只跳过入口检查；`Home()` 内部仍通过 `Require` 决定 task/project 区块。

- [x] **Step 4: 运行授权测试并确认 GREEN**

Run: `go test ./internal/app -run 'TestAuthorizeTokenRequestContextOnly' -count=1`

Expected: PASS。

- [x] **Step 5: 写 HTTP 失败测试**

`internal/httpapi/home_test.go` 覆盖：

- user token 返回 `today`、完整 `my_work.items[].task.assignees[]`、`project_attention`。
- tenant token 只有 `project:read` 时 `my_work=null` 且返回项目区。
- tenant token 只有 `task:read` 时返回 `my_work=null`（系统身份）且项目区为空。
- project-scoped token 只返回 allowlist project。
- 响应 snake_case，不含裸 `user_id`。
- OpenAPI `/api/v1/home` 包含 `reasons` enum 与 `JSONUserInfo`。

- [x] **Step 6: 运行 HTTP 测试并确认 RED**

Run: `go test ./internal/httpapi -run 'TestHTTPHome|TestOpenAPIDocumentsHome' -count=1`

Expected: FAIL，route/handler/schema 尚不存在。

- [x] **Step 7: 实现 handler、DTO 与 Huma schema**

`home.go`：

```go
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
    scoped, _, err := s.scopedService(r, "", "", "")
    if err != nil { writeAppError(w, err); return }
    view, err := scoped.Home()
    if err != nil { writeAppError(w, err); return }
    writeSuccess(w, http.StatusOK, homeResponseFromView(view), nil)
}
```

转换规则：

- task 复用 `taskOccurrenceViewToJSON`；assignees 自动走 `task.JSONUserInfo`。
- annotation actor 复用 `task.ActorInfoToJSON`。
- project 复用 `projectResponseFromView`。
- nil `my_work` 输出 JSON `null`，数组初始化为空数组。

在 `humaRoutes()` 注册 `GET /api/v1/home`；在 `contractSuccessResponse` 加 `homeOpenAPISchema()`，reasons enum 固定四个值。

- [x] **Step 8: 运行 HTTP 测试并确认 GREEN**

Run: `go test ./internal/httpapi -run 'TestHTTPHome|TestOpenAPIDocumentsHome|TestOpenAPIIncludesEveryRegisteredHTTPRoute' -count=1`

Expected: PASS。

- [x] **Step 9: 提交 Task 2**

```bash
git add internal/app/request_scope.go internal/app/request_scope_test.go internal/httpapi/app_service.go internal/httpapi/home.go internal/httpapi/home_test.go internal/httpapi/huma_routes.go internal/httpapi/server_test.go
git diff --cached --check
git commit -m "feat: 提供用户首页 API"
```

---

### Task 3: “已开始”预设与首页返回状态

**Files:**
- Modify: `web/src/features/workspace/my-tasks/my-task-tabs.ts`
- Modify: `web/src/features/workspace/my-tasks/my-task-tabs.test.ts`
- Modify: `web/src/pages/my-tasks-page.tsx`
- Modify: `web/src/routes/router.tsx`
- Create: `web/src/features/workspace/home/home-return-state.ts`
- Create: `web/src/features/workspace/home/home-return-state.test.ts`
- Modify: `web/src/routes/workspace/ProjectTaskDetailRoute.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`

**Interfaces:**
- Produces: `MyTaskTabKey` 新值 `started`
- Produces: `/my-tasks?tab=started`
- Produces: task detail search `from=home`
- Produces: `saveHomeReturnState` / `takeHomeReturnState`，按 workspace + actor 隔离

- [x] **Step 1: 写“已开始”失败测试**

```ts
it("maps started to open tasks with a start timestamp", () => {
  expect(tabFilter("started", now)).toEqual({
    query: "(status:pending or status:waiting) and start.notnull",
  })
})
```

并断言 `MY_TASK_TABS` 顺序为 `incomplete, started, today, overdue, noDue, completed`。

- [x] **Step 2: 运行测试确认 RED**

Run: `pnpm --dir web test -- my-task-tabs.test.ts`

Expected: FAIL，`started` 不属于 `MyTaskTabKey`。

- [x] **Step 3: 实现 started tab 和路由校验**

更新 tab 类型、数组、`tabFilter`、`isMyTaskTabKey` 和中英文文案；`router.tsx` 继续只接受结构化 `tab`，不暴露 raw query。

- [x] **Step 4: 运行测试确认 GREEN**

Run: `pnpm --dir web test -- my-task-tabs.test.ts my-tasks-page.test.tsx`

Expected: PASS。

- [x] **Step 5: 写首页返回状态失败测试**

测试 storage key 包含 workspace+actor，保存 `scrollTop/focusId` 后只能取一次；task detail `from=home` 时返回 `/` 并显示“返回首页”。

- [x] **Step 6: 运行返回状态测试确认 RED**

Run: `pnpm --dir web test -- home-return-state.test.ts task-detail-page.test.tsx`

Expected: FAIL，home return state 和 `from=home` 尚不存在。

- [x] **Step 7: 实现返回状态**

- `home-return-state.ts` 复用 my-tasks return state 的 sessionStorage 思路，key 加 workspaceSlug 与 actorID。
- router 的 task detail validateSearch 接受 `from`；`ProjectTaskDetailRoute` 把 `from=home` 传给 `TaskDetailPage`。
- `TaskDetailPage` 优先返回首页，其次 my-tasks，再回 project；canonicalize occurrence permalink 时保留 `from=home`。

- [x] **Step 8: 运行返回状态测试确认 GREEN**

Run: `pnpm --dir web test -- home-return-state.test.ts task-detail-page.test.tsx`

Expected: PASS。

- [x] **Step 9: 提交 Task 3**

```bash
git add web/src/features/workspace/my-tasks web/src/pages/my-tasks-page.tsx web/src/routes/router.tsx web/src/features/workspace/home/home-return-state.ts web/src/features/workspace/home/home-return-state.test.ts web/src/routes/workspace/ProjectTaskDetailRoute.tsx web/src/features/workspace/project-workbench/task-detail web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git diff --cached --check
git commit -m "feat: 增加已开始任务入口"
```

---

### Task 4: Web 用户首页与任务快捷交互

**Files:**
- Create: `web/src/features/workspace/home/home-api.ts`
- Create: `web/src/features/workspace/home/home-api.test.ts`
- Create: `web/src/features/workspace/home/home-page.tsx`
- Create: `web/src/features/workspace/home/home-page.test.tsx`
- Modify: `web/src/pages/OverviewPage.tsx`
- Modify: `web/src/pages/OverviewPage.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`
- Modify: `web/src/components/AppShell.tsx`
- Modify: `web/src/components/AppShell.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: `GET /api/v1/home`
- Produces: `getHome(): Promise<HomeView>`
- Produces: `HomePage({me})`
- Produces: optional `TaskCreateDialog.onCreated(task)` callback
- Produces: home query key `['home', workspaceSlug, actor.id]`

- [x] **Step 1: 写 API 失败测试**

断言 `getHome()` 请求 `/api/v1/home`，类型覆盖 `my_work=null`、reasons、project metrics、latest update actor。

- [x] **Step 2: 运行 API 测试确认 RED**

Run: `pnpm --dir web test -- home-api.test.ts`

Expected: FAIL，module 尚不存在。

- [x] **Step 3: 实现 API client**

使用 `workspaceApiGet<HomeView>("/api/v1/home")`，复用 `ProjectWorkbenchTask/ProjectWorkbenchProject/ProjectSeriesMetrics/ProjectAnnotationInfo` 类型，不复制用户结构。

- [x] **Step 4: 写首页组件失败测试**

覆盖：

- user actor 显示“你好，display_name”“我的今日”“项目关注”。
- 点击 started/today/overdue/high priority 计数进入对应 `/my-tasks` search。
- 重点任务顺序使用 API 顺序；显示 project、reason、priority、due。
- 快捷 start/stop/done 调用既有 API，成功失效 home/my-tasks/project queries，失败保留行。
- 点击任务进入 project-scoped detail 且 search 为 `from=home`。
- config rows 为空时不显示“工作区信息”；有数据时 label 主、key 次、secret 不 reveal。
- tenant actor 显示系统身份说明和管理快捷入口，不显示个人任务空状态。
- loading/error/readonly/mobile class 与 aria label。
- 页面不请求 audit 或 notification-deliveries。

- [x] **Step 5: 运行组件测试确认 RED**

Run: `pnpm --dir web test -- home-page.test.tsx OverviewPage.test.tsx`

Expected: FAIL，当前仍渲染旧运维概览。

- [x] **Step 6: 实现 HomePage**

组件拆分保持单一职责：

```text
home-page.tsx
  HomeHeader
  MyTodaySection
  HomeTaskRow
  ProjectAttentionSection
  WorkspaceInfoSection (复用 ConfigOverviewSection 的 value formatter，空数组时隐藏)
  SystemActorHome
```

实现要求：

- `OverviewPage` 只作为兼容包装器渲染 `HomePage`，继续导出 `PageHeader`。
- query 使用 `staleTime: 30_000`、`refetchOnWindowFocus: true`。
- mutation 复用 `useTaskActionMutation`，并在成功时新增失效 `['home', workspaceSlug]`。
- desktop 任务区全宽，项目/工作区信息 `lg:grid-cols-[2fr_1fr]`；无 config 时项目区占满。
- task row 主体是 Link；动作按钮 `stopPropagation`，有独立 aria-label。
- 项目风险 chip 进入 project Tasks 的现有结构化 filters。
- 完成比例使用 `completed_count/task_count`；0 task 显示 0%。
- project latest update 只展示后端 content/actor/time，不从 audit 推导。

- [x] **Step 7: 实现首页新建任务**

- 首页按钮先打开 active/planning project selector。
- 选定 project 后复用 `TaskCreateDialog`。
- 为 `TaskCreateDialog` 增加 `onCreated?: (task: ProjectWorkbenchTask) => void`，在普通任务成功创建后调用。
- created callback 失效 home query，并提供打开 project-scoped detail 的入口。
- 无可写项目或 read-only 身份不显示按钮。

- [x] **Step 8: 更新导航和文案**

- `nav.overview` 改为“首页”/“Home”。
- `app.description` 不再描述运维入口，改成面向任务协作的中性文案。
- 首页所有文案进入 `home.*` namespace，不硬编码中文。
- 保留管理/系统导航结构，不扩大为 Shell 重写。

- [x] **Step 9: 运行组件测试确认 GREEN**

Run: `pnpm --dir web test -- home-api.test.ts home-page.test.tsx OverviewPage.test.tsx task-create-dialog.test.tsx AppShell.test.tsx`

Expected: PASS。

- [x] **Step 10: 提交 Task 4**

```bash
git add web/src/features/workspace/home web/src/pages/OverviewPage.tsx web/src/pages/OverviewPage.test.tsx web/src/features/workspace/project-workbench/tasks/task-create-dialog.tsx web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts web/src/components/AppShell.tsx web/src/components/AppShell.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git diff --cached --check
git commit -m "feat: 重构 Web Console 用户首页"
```

---

### Task 5: 文档同步、全量验证与完成审计

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-07-17-web-console-user-home-design.md`
- Modify: `docs/superpowers/plans/2026-07-17-web-console-user-home-implementation.md`

**Interfaces:**
- Consumes: Tasks 1-4 的最终 API 与 UI 行为
- Produces: 与代码一致的用户文档、路线图状态和验证证据

- [x] **Step 1: 更新 README / ROADMAP**

- README Web Console 能力改为：首页以个人任务为主，项目关注与工作区信息为辅；运维信息在专页。
- ROADMAP 新增当前 milestone 条目与 `GET /api/v1/home`、started preset、系统身份降级页的完成状态。
- spec 状态从“待评审”改为“已实施”，并确保 API 示例与最终字段一致。
- plan 勾选实际完成步骤；未执行的步骤不能标完成。

- [x] **Step 2: 运行 focused Go tests**

```bash
go test ./internal/storage -run 'TestHomeRepository' -count=1
go test ./internal/app -run 'TestHome|TestAuthorizeTokenRequestContextOnly' -count=1
go test ./internal/httpapi -run 'TestHTTPHome|TestOpenAPIDocumentsHome|TestOpenAPIIncludesEveryRegisteredHTTPRoute' -count=1
```

Expected: 全部 PASS。

- [x] **Step 3: 运行 focused Web tests**

```bash
pnpm --dir web test -- my-task-tabs.test.ts home-return-state.test.ts task-detail-page.test.tsx home-api.test.ts home-page.test.tsx OverviewPage.test.tsx task-create-dialog.test.tsx AppShell.test.tsx
```

Expected: 全部 PASS。

- [x] **Step 4: 运行仓库完整验证**

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

Expected: 所有命令 exit 0；若 smoke 需要服务进程，按脚本既有方式启动并在验证后停止，证明端口已释放。

- [x] **Step 5: 按 spec 逐条完成审计**

逐项核对 spec §17：

- 首页首屏主语；
- 任务打开/开始/停止/完成；
- 全量计数不受 200 限制；
- 项目集合聚合无 N+1；
- config label/value 与 secret；
- 系统身份降级；
- UserInfo/ActorInfo；
- SQLite/PostgreSQL/CGO=0。

每项记录对应测试、代码或命令输出；证据缺失就继续实现或补测试，不能仅凭搜索结果判定完成。

- [x] **Step 6: 提交文档与验证收口**

```bash
git add README.md ROADMAP.md docs/superpowers/specs/2026-07-17-web-console-user-home-design.md docs/superpowers/plans/2026-07-17-web-console-user-home-implementation.md
git diff --cached --check
git commit -m "docs: 同步用户首页实现"
```

- [x] **Step 7: 最终工作树检查**

Run: `git status --short && git log -5 --oneline`

Expected: 实现 worktree 无未提交文件；提交历史只包含本功能相关变更。
