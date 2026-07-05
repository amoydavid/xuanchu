# Web Console Capability Bridge Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Web Console 从单项目工作台补齐为「任务协作入口 + 治理控制台」，让普通成员能处理个人任务并查看全量 audit，让 owner/admin 能在浏览器完成常见治理操作。

**Architecture:** 继续复用现有 `/api/v1/*`、TanStack Router、TanStack Query 和 project-workbench 组件边界。Phase 1 先整理 Shell 和个人任务中心；Phase 2 补齐 Audit、Hook、项目设置和成员外部身份；Phase 3 只接入后端已具备的可见性增强，不引入新的业务后端。

**Tech Stack:** Go 1.25、GORM、`github.com/glebarez/sqlite`、React 19、TanStack Router、TanStack Query、shadcn/ui、lucide-react、Vitest、Testing Library、Playwright smoke。

---

## Scope Check

本 spec 跨越 Shell、任务、Audit、Hook、项目设置、成员身份和若干低频资源。为避免大爆炸，本计划按可独立发布的 chunk 拆分。每个 chunk 都必须在合并前独立通过前端验证；触碰后端 authz / HTTP 契约的 chunk 还必须跑 Go 验证。

执行原则：

- 优先复用现有 API 和组件；新增 API 仅限 plan 中明确列出的后端契约对齐。
- 不做 kanban、drag/drop、甘特、完整通知收件箱、task restore、workspace unarchive。
- 普通成员必须能查看 workspace 全量 audit；这需要同步后端 authz、HTTP 测试和前端 capability 文案。
- 用户身份输出继续遵守 `task.UserInfo` / `task.UserInfoToJSON()` 规则，不输出裸 UUID。

## File Structure

计划新增或修改的主要文件：

- `web/src/components/AppShell.tsx`：侧栏分组、侧栏身份块、header slot、`my-tasks` nav。
- `web/src/components/AppShell.test.tsx`：Shell 行为回归。
- `web/src/features/workspace/session/useMe.ts`：补齐 `MeResponse.actor` 类型。
- `web/src/routes/router.tsx`：新增 `/my-tasks`、`/tasks/$taskRef`、`/projects/$projectSlug/settings` 路由。
- `web/src/routes/workspace/MyTasksRoute.tsx`：我的任务路由壳。
- `web/src/routes/workspace/TaskDetailRoute.tsx`：脱离项目上下文的任务详情入口。
- `web/src/routes/workspace/ProjectSettingsRoute.tsx`：项目设置页路由壳。
- `web/src/pages/my-tasks-page.tsx`：我的任务页面编排。
- `web/src/pages/project-settings-page.tsx`：项目设置页面编排。
- `web/src/features/workspace/my-tasks/*`：我的任务 API、tabs、筛选和测试。
- `web/src/features/workspace/project-workbench/tasks/task-table.tsx`：支持全局任务页的可选项目上下文、已删除态展示。
- `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`：项目参数可选，按 task.project 兜底。
- `web/src/features/workspace/project-workbench/task-detail/task-urgency-panel.tsx`：urgency 展示。
- `web/src/features/workspace/project-workbench/api/task-api.ts`：任务 urgency path/API。
- `web/src/features/workspace/project-workbench/api/project-api.ts`：project config / annotation API。
- `web/src/features/workspace/hooks/*`：Hook Console、API、delivery 展开和测试。
- `web/src/features/workspace/audit/*`：Audit Console、API、当前结果筛选、CSV 导出和测试。
- `web/src/features/workspace/resources/resource-dispatch.tsx`：资源页专用控制台分发。
- `web/src/routes/workspace/ResourceRoute.tsx`：从 `ResourcePage` 改为 dispatch。
- `web/src/features/workspace/members/members-api.ts`：external-id bind/unbind/list API。
- `web/src/features/workspace/members/external-id-bind-dialog.tsx`：外部身份绑定弹窗。
- `web/src/features/workspace/members/members-page.tsx`：成员详情页移除只读限制，补 external-id 操作。
- `web/src/features/workspace/project-workbench/projects/project-row-actions.tsx`：项目列表行操作菜单。
- `web/src/locales/zh-CN.ts`、`web/src/locales/en-US.ts`：所有新增可见文案。
- `internal/authz/policy.go`、`internal/authz/policy_test.go`：普通 member / viewer 的 audit read 授权对齐。
- `internal/app/audit_test.go`、`internal/httpapi/auth_test.go`：通用 audit 全量可见行为回归。
- `README.md`、`ROADMAP.md`、本 spec：落地后的文档同步。

---

## Chunk 1: Shell 归位与我的任务入口

### Task 1: 扩展当前凭证类型与 Shell 测试

**Files:**
- Modify: `web/src/features/workspace/session/useMe.ts`
- Modify: `web/src/components/AppShell.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing tests**
  - 在 `AppShell.test.tsx` 增加用例：
    - 侧栏底部展示 `Alice Chen`、`alice`、`admin`、`pat`、workspace slug 和 risk。
    - 普通模式下「退出」在侧栏身份块中，不再依赖顶栏左侧 actor 文案。
    - acting / tenant-switch 模式下侧栏身份块显示 amber 风险状态并提供「返回超管」。
    - SSO 菜单仍只对 owner / tenant actor / tenant switch 可见。
  - 在 `useMe.ts` 类型上补齐测试所需字段：`actor.id`、`actor.display_name`、`actor.email`、`actor.external_ids`。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/components/AppShell.test.tsx`
  - Expected: FAIL，因为 AppShell 还没有身份块和完整 actor 类型。

- [x] **Step 3: Implement minimal type changes**
  - 修改 `MeResponse`：
    ```ts
    actor: {
      id: string
      name: string
      display_name?: string
      email?: string | null
      external_ids?: Array<{ provider: string; external_id: string }>
    }
    ```
  - 不改变 API 请求路径，继续使用 `/api/v1/credentials/current`。

- [x] **Step 4: Run focused typecheck**
  - Run: `pnpm --dir web typecheck`
  - Expected: 现有调用点如果假设 actor 只有 name，应全部显式处理。

### Task 2: 重构 AppShell 为侧栏身份块 + Header Slot

**Files:**
- Modify: `web/src/components/AppShell.tsx`
- Modify: `web/src/routes/workspace/WorkspaceRootRoute.tsx`
- Modify: `web/src/components/AppShell.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing tests**
  - `AppShell` 支持 `headerTitle`、`breadcrumbs`、`headerActions`。
  - 默认 header title 没有传入时显示 `app.title`，不显示 `actorName · tokenType`。
  - 刷新按钮仍可点击并调用 `onRefresh`。
  - 侧栏底部身份块包含退出 / 返回超管 / 主题 / 语言入口。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/components/AppShell.test.tsx`
  - Expected: FAIL with missing header slot behavior.

- [x] **Step 3: Implement AppShell**
  - 将 `navItems` 改为分组数据：
    ```ts
    const navGroups = [
      { labelKey: "nav.group.personal", items: [...] },
      { labelKey: "nav.group.management", items: [...] },
      { labelKey: "nav.group.system", items: [...] },
    ]
    ```
  - 新增 `PageKey`：`"myTasks"`。
  - 新增侧栏身份块组件，读取 `useMe()` 数据和 `tokenType` fallback。
  - 顶栏左侧只渲染 `breadcrumbs ?? headerTitle ?? t("app.title")`。
  - 保留 acting / tenant-switch 的 amber 背景提示，但详细身份在侧栏底部。
  - `WorkspaceRootRoute` 继续负责登录和刷新，只传入 Shell 需要的全局回调。

- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/components/AppShell.test.tsx web/src/routes/workspace/WorkspaceRootRoute.test.tsx`
  - Expected: PASS.

- [x] **Step 5: Commit chunk**
  ```bash
  git add web/src/components/AppShell.tsx web/src/components/AppShell.test.tsx web/src/routes/workspace/WorkspaceRootRoute.tsx web/src/features/workspace/session/useMe.ts web/src/locales/zh-CN.ts web/src/locales/en-US.ts
  git commit -m "feat: 调整 Web Console Shell 身份区"
  ```

### Task 3: 新增我的任务路由骨架

**Files:**
- Modify: `web/src/routes/router.tsx`
- Create: `web/src/routes/workspace/MyTasksRoute.tsx`
- Create: `web/src/pages/my-tasks-page.tsx`
- Modify: `web/src/components/AppShell.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing tests**
  - `AppShell` 在 `/my-tasks` 高亮「我的任务」。
  - router 注册 `/my-tasks`，且 `/tasks` 旧重定向保持不变。
  - `MyTasksPage` 初始只显示标题、空状态和 system actor 说明。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/components/AppShell.test.tsx`
  - Expected: FAIL because `myTasks` nav does not exist.

- [x] **Step 3: Implement route skeleton**
  - `router.tsx` 新增 lazy route：
    ```ts
    const MyTasksRoute = lazy(() =>
      import("@/routes/workspace/MyTasksRoute").then((module) => ({
        default: module.MyTasksRoute,
      }))
    )
    ```
  - 新增 `myTasksRoute` 路由 path `/my-tasks`。
  - `MyTasksRoute` 从 `useMe()` 读取 workspace 和 actor，传给 `MyTasksPage`。
  - `MyTasksPage` 先只做页面壳，使用 AppShell header slot 显示「我的任务」。

- [x] **Step 4: Run tests**
  - Run: `pnpm --dir web test web/src/components/AppShell.test.tsx`
  - Run: `pnpm --dir web typecheck`
  - Expected: PASS.

---

## Chunk 2: 我的任务数据、详情解耦与已删除任务可见性

### Task 4: 实现我的任务 API 与筛选模型

**Files:**
- Create: `web/src/features/workspace/my-tasks/my-tasks-api.ts`
- Create: `web/src/features/workspace/my-tasks/my-tasks-api.test.ts`
- Create: `web/src/features/workspace/my-tasks/my-task-tabs.ts`
- Create: `web/src/features/workspace/my-tasks/my-task-tabs.test.ts`
- Modify: `web/src/pages/my-tasks-page.tsx`

- [x] **Step 1: Write failing API tests**
  - `myTasksPath("dajee", { assignee: "user-1", status: "pending" })` 输出 `/api/v1/tasks?workspace=dajee&assignee=user-1&status=pending&limit=200`。
  - 支持 `project`、`priority`、`due_before`、`due_after`、`due_empty`、`q`、`sort`。
  - 空值不进入 query string。

- [x] **Step 2: Write failing tab tests**
  - `today` tab 输出 `status=pending` + 当天 23:59 的 `due_before`。
  - `overdue` tab 输出 `status=pending` + 当前时间前的 `due_before`。
  - `noDue` tab 输出 `status=pending` + `due_empty=true`。

- [x] **Step 3: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/my-tasks/my-tasks-api.test.ts web/src/features/workspace/my-tasks/my-task-tabs.test.ts`
  - Expected: FAIL because files do not exist.

- [x] **Step 4: Implement API helpers**
  - 复用 `workspaceApiGet<ProjectWorkbenchTask[]>`。
  - 类型复用 `ProjectWorkbenchTask`，不要创建第二套 task DTO。
  - tab 纯函数接收 `now: Date`，避免测试依赖当前时间。

- [x] **Step 5: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/my-tasks/my-tasks-api.test.ts web/src/features/workspace/my-tasks/my-task-tabs.test.ts`
  - Expected: PASS.

### Task 5: 抽出可复用任务表格能力

**Files:**
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
- Modify: `web/src/pages/my-tasks-page.tsx`

- [x] **Step 1: Write failing tests**
  - `TaskTable` 在没有固定 `projectSlug` 时，使用 task 自身 `project` 字段生成 `/tasks/:taskRef` 或项目深链。
  - 全局模式下显示项目列，项目列跳转 `/workspaces/:workspaceSlug/projects/:projectSlug`。
  - `status=deleted` 的任务显示灰显和「已删除」标记，写操作按钮禁用。
  - 项目工作台原有 inline title / priority / due 行为不退化。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
  - Expected: FAIL because table currently requires projectSlug.

- [x] **Step 3: Implement minimal table generalization**
  - 保留现有 `TaskTable` 文件，不大规模搬迁。
  - 将 props 调整为：
    ```ts
    projectSlug?: string
    mode?: "project" | "global"
    ```
  - `useModifyTaskMutation` 仍需要 project context；global 模式行内编辑先只允许状态流转 / 详情入口，或从 task.project 兜底生成 project context。
  - 所有 fallback 必须避免空 project 导致错误链接。

- [x] **Step 4: Wire MyTasksPage**
  - `MyTasksPage` 使用 `useQuery` 调 `getMyTasks`。
  - tenant actor 显示 system identity 空状态，不发起 assignee 查询。
  - 普通用户使用 `me.data.actor.id` 作为 assignee。

- [x] **Step 5: Run tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
  - Run: `pnpm --dir web test web/src/pages/my-tasks-page.test.tsx`
  - Expected: PASS. If `my-tasks-page.test.tsx` does not exist, create it in this task.

### Task 6: 解耦任务详情页的项目上下文

**Files:**
- Create: `web/src/routes/workspace/TaskDetailRoute.tsx`
- Modify: `web/src/routes/workspace/ProjectTaskDetailRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`

- [x] **Step 1: Write failing tests**
  - `/tasks/$taskRef` 路由能渲染 `TaskDetailPage`。
  - `TaskDetailPage` 在 `projectSlug` 未传入时，从 `task.project` 兜底生成返回项目链接。
  - task 没有 project 时，隐藏「返回项目」和需要 project context 的动作。
  - 项目内详情页仍校验 task 属于 URL project。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
  - Expected: FAIL with required `projectSlug`.

- [x] **Step 3: Implement optional project context**
  - `TaskDetailPageProps.projectSlug?: string`。
  - `effectiveProjectSlug = projectSlug ?? taskData.project ?? undefined`。
  - `useModifyTaskMutation` 支持 optional project slug 或在 mutation invalidate 时只 invalidates task detail + global lists。
  - 项目内路由仍传入 `projectSlug`，保持旧行为。

- [x] **Step 4: Register route**
  - `router.tsx` 新增 `/tasks/$taskRef`。
  - 保留 `/tasks` 旧列表重定向到 `/projects`。

- [x] **Step 5: Run tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
  - Run: `pnpm --dir web typecheck`
  - Expected: PASS.

### Task 7: 已删除任务筛选可见性

**Files:**
- Modify: `web/src/features/workspace/project-readonly/project-filter.ts`
- Modify: `web/src/features/workspace/project-readonly/project-filter.test.ts`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.tsx`

- [x] **Step 1: Write failing tests**
  - toolbar 更多筛选能设置 `status=deleted`。
  - deleted 任务行灰显，不显示写入口。
  - 默认筛选不包含 deleted。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-readonly/project-filter.test.ts web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`

- [x] **Step 3: Implement**
  - 复用现有 `status` 参数，不新增后端 query。
  - tooltip 文案明确「恢复能力待后端支持」。

- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-readonly/project-filter.test.ts web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`

- [x] **Step 5: Commit chunk**
  ```bash
  git add web/src/routes/router.tsx web/src/routes/workspace/MyTasksRoute.tsx web/src/routes/workspace/TaskDetailRoute.tsx web/src/pages/my-tasks-page.tsx web/src/features/workspace/my-tasks web/src/features/workspace/project-workbench web/src/locales/zh-CN.ts web/src/locales/en-US.ts
  git commit -m "feat: 增加我的任务入口"
  ```

---

## Chunk 3: Audit 全量可见与 Audit Console

### Task 8: 后端授权对齐为普通成员可看全量 audit

**Files:**
- Modify: `internal/authz/policy.go`
- Modify: `internal/authz/policy_test.go`
- Modify: `internal/app/audit_test.go`
- Modify: `internal/httpapi/auth_test.go`
- Modify: `docs/superpowers/specs/2026-07-05-web-console-capability-bridge-design.md` if behavior wording drifts.

- [x] **Step 1: Write failing authz tests**
  - 在 `internal/authz/policy_test.go` 增加：
    - member can read audit。
    - viewer can read audit（如果「所有可登录成员」包含 viewer，则 viewer 也应通过；若产品只指 member/admin/owner，则先更新 spec 明确不含 viewer）。
  - 在 `internal/app/audit_test.go` 修改旧断言：viewer/member 调 `ListAudit` 应返回 rows，而不是 permission denied。
  - 在 `internal/httpapi/auth_test.go` 增加普通 member token 调 `/api/v1/audit?limit=10` 返回 200 的测试。

- [x] **Step 2: Run red Go tests**
  - Run: `go test ./internal/authz ./internal/app ./internal/httpapi -run 'Audit|AllowedForRole' -count=1`
  - Expected: FAIL because member/viewer currently lack `PermissionAuditRead`.

- [x] **Step 3: Implement authz change**
  - 在 `internal/authz/policy.go` 的 RoleMember / RoleViewer 权限集合加入 `PermissionAuditRead`。
  - 不绕过 token scope：HTTP 仍要求 token 或 browser session capabilities 包含 `audit:read`。
  - 如果 browser session 默认 capabilities 没有 `audit:read`，补齐 session capability 生成处，并加测试。

- [x] **Step 4: Run focused Go tests**
  - Run: `go test ./internal/authz ./internal/app ./internal/httpapi -run 'Audit|AllowedForRole' -count=1`
  - Expected: PASS.

### Task 9: Audit API 与当前结果筛选

**Files:**
- Create: `web/src/features/workspace/audit/audit-api.ts`
- Create: `web/src/features/workspace/audit/audit-api.test.ts`
- Create: `web/src/features/workspace/audit/audit-filter.ts`
- Create: `web/src/features/workspace/audit/audit-filter.test.ts`
- Create: `web/src/features/workspace/audit/audit-console.tsx`
- Create: `web/src/features/workspace/audit/audit-console.test.tsx`
- Modify: `web/src/features/workspace/resources/resource-config.tsx`
- Create: `web/src/features/workspace/resources/resource-dispatch.tsx`
- Modify: `web/src/routes/workspace/ResourceRoute.tsx`

- [x] **Step 1: Write failing API/filter tests**
  - `auditPath({ limit: 50, project: "proj-a" })` 输出 `/api/v1/audit?limit=50&project=proj-a`。
  - `filterAuditRows` 按 actor/action/target/time 过滤当前 rows。
  - CSV 导出函数包含 header，能处理逗号、换行和引号。

- [x] **Step 2: Write failing console tests**
  - 普通 member capabilities 包含 `audit:read` 时可看到 Audit Console。
  - 页面文案显示「搜索当前结果」，不声称全量 actor/action/time 服务端搜索。
  - 点击「导出 CSV」调用下载逻辑或返回可测试 CSV 字符串。

- [x] **Step 3: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/audit/audit-api.test.ts web/src/features/workspace/audit/audit-filter.test.ts web/src/features/workspace/audit/audit-console.test.tsx`

- [x] **Step 4: Implement Audit Console**
  - API 只使用当前后端支持的 `limit`、`project`。
  - actor/action/time range 明确作为当前结果筛选。
  - `ResourceRoute` 改为：
    ```tsx
    return <ResourceDispatch page={page} workspaceSlug={me.data?.effective_workspace.slug} />
    ```
  - `resource-config.tsx` 保留 fallback 列定义，不再负责 `audit` 专用页面。

- [x] **Step 5: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/audit/audit-api.test.ts web/src/features/workspace/audit/audit-filter.test.ts web/src/features/workspace/audit/audit-console.test.tsx`
  - Run: `pnpm --dir web typecheck`

- [x] **Step 6: Commit chunk**
  ```bash
  git add internal/authz internal/app/audit_test.go internal/httpapi/auth_test.go web/src/features/workspace/audit web/src/features/workspace/resources web/src/routes/workspace/ResourceRoute.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
  git commit -m "feat: 开放成员审计查看"
  ```

---

## Chunk 4: Hook Console、项目设置与成员外部身份

### Task 10: Hook Console API 与列表

**Files:**
- Create: `web/src/features/workspace/hooks/hooks-api.ts`
- Create: `web/src/features/workspace/hooks/hooks-api.test.ts`
- Create: `web/src/features/workspace/hooks/hook-console.tsx`
- Create: `web/src/features/workspace/hooks/hook-console.test.tsx`
- Create: `web/src/features/workspace/hooks/hook-deliveries.tsx`
- Modify: `web/src/features/workspace/resources/resource-dispatch.tsx`

- [x] **Step 1: Write failing API tests**
  - paths:
    - `/api/v1/hooks`
    - `/api/v1/hooks/{hookID}/enable`
    - `/api/v1/hooks/{hookID}/disable`
    - `/api/v1/hooks/{hookID}/deliveries`
    - `/api/v1/hook-deliveries/{deliveryID}/replay`
  - CRUD helpers call `workspaceApiGet/Post/Patch/Delete` with typed DTOs.

- [x] **Step 2: Write failing console tests**
  - 列表显示 hook name、scope/event、enabled、最近投递状态。
  - 展开行加载 deliveries。
  - replay 按钮调用 replay API 并刷新 deliveries。
  - 没有 `hook:write` capability 时隐藏新增、编辑、启用/暂停、删除、重放。

- [x] **Step 3: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/hooks/hooks-api.test.ts web/src/features/workspace/hooks/hook-console.test.tsx`

- [x] **Step 4: Implement Hook Console**
  - 第一版行展开内嵌 deliveries，不跳转。
  - 保留 `ResourcePage` fallback，`hooks` 走 `HookConsole`。
  - 失败投递使用 Badge + 文案，不只靠图标。

- [x] **Step 5: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/hooks/hooks-api.test.ts web/src/features/workspace/hooks/hook-console.test.tsx`

### Task 11: Hook 新建/编辑/删除/启停弹窗

**Files:**
- Create: `web/src/features/workspace/hooks/hook-create-dialog.tsx`
- Create: `web/src/features/workspace/hooks/hook-edit-dialog.tsx`
- Modify: `web/src/features/workspace/hooks/hook-console.tsx`
- Modify: `web/src/features/workspace/hooks/hook-console.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing tests**
  - 新建 Hook 弹窗提交 name、event/scope、sink 或后端当前 DTO 必填字段。
  - 编辑保存失败不关闭弹窗。
  - 删除需要二次确认。
  - enable/disable 调正确 endpoint。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/hooks/hook-console.test.tsx`

- [x] **Step 3: Implement dialogs**
  - 参考 `web/src/features/workspace/tokens` 和 `project-create-dialog` 的 shadcn dialog 模式。
  - 不在前端发明后端不存在的「测试投递」能力；如果按钮保留，必须置灰并说明后端缺口。

- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/hooks/hook-console.test.tsx`

### Task 12: 项目设置页与项目行操作

**Files:**
- Create: `web/src/routes/workspace/ProjectSettingsRoute.tsx`
- Create: `web/src/pages/project-settings-page.tsx`
- Create: `web/src/pages/project-settings-page.test.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.test.ts`
- Create: `web/src/features/workspace/project-workbench/project/project-config-editor.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-annotations-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/projects/project-row-actions.tsx`
- Modify: `web/src/features/workspace/project-workbench/projects/projects-list-page.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing API tests**
  - `projectConfigPath(workspace, project, key?)` encodes key。
  - `listProjectAnnotations`、`addProjectAnnotation`、`deleteProjectAnnotation` 使用 POST/GET/DELETE，无 PATCH。
  - `transitionProject` 继续覆盖 archived/cancelled/active。

- [x] **Step 2: Write failing page tests**
  - `/projects/:slug/settings` 显示基本信息、状态、config、annotations。
  - annotation 只显示新增和删除，不显示编辑。
  - config 支持新增、编辑、删除。
  - 项目列表行菜单显示「打开项目」「设置」「归档」「取消」。

- [x] **Step 3: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/api/project-api.test.ts web/src/pages/project-settings-page.test.tsx web/src/features/workspace/project-workbench/projects/projects-list-page.test.tsx`

- [x] **Step 4: Implement route/page**
  - Route path: `/projects/$projectSlug/settings` under workspace root。
  - `ProjectSettingsPage` 使用现有 `getProject` / `modifyProject` / `transitionProject`。
  - config editor 调现有 project config endpoints。
  - annotations editor 不做伪编辑。
  - row action 的归档/取消复用 `project-status-menu` 的确认模式或 `destructive-confirm-dialog`。

- [x] **Step 5: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/api/project-api.test.ts web/src/pages/project-settings-page.test.tsx web/src/features/workspace/project-workbench/projects/projects-list-page.test.tsx`

### Task 13: 成员 external-id 绑定/解绑

**Files:**
- Modify: `web/src/features/workspace/members/members-api.ts`
- Modify: `web/src/features/workspace/members/members-page.tsx`
- Modify: `web/src/features/workspace/members/members-page.test.tsx`
- Create: `web/src/features/workspace/members/external-id-bind-dialog.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing API tests**
  - `bindExternalID(userRef, provider, externalID)` POST `/api/v1/users/{user}/external-ids`。
  - `unbindExternalID(userRef, provider, externalID)` DELETE encoded provider/externalID。
  - `listExternalIDs(userRef)` GET endpoint。

- [x] **Step 2: Write failing UI tests**
  - owner/admin + `member:write` 在成员详情页看到「绑定外部 ID」。
  - member/viewer 不看到绑定/解绑按钮。
  - 解绑需要确认。
  - 绑定成功后刷新 user detail。

- [x] **Step 3: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/members/members-page.test.tsx`

- [x] **Step 4: Implement UI**
  - 详情页使用 `user.external_ids` 展示。
  - provider 输入第一版用普通 select/input，不做 provider 管理后台。
  - 所有错误走现有 `ApiError` 展示模式。

- [x] **Step 5: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/members/members-page.test.tsx`

- [x] **Step 6: Commit chunk**
  ```bash
  git add web/src/features/workspace/hooks web/src/features/workspace/project-workbench web/src/features/workspace/members web/src/pages/project-settings-page.tsx web/src/routes/workspace/ProjectSettingsRoute.tsx web/src/routes/router.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
  git commit -m "feat: 补齐控制台管理闭环"
  ```

---

## Chunk 5: P2 可见性、文档同步与全量验证

### Task 14: Task urgency panel

**Files:**
- Modify: `web/src/features/workspace/project-workbench/api/task-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/task-api.test.ts`
- Create: `web/src/features/workspace/project-workbench/task-detail/task-urgency-panel.tsx`
- Create: `web/src/features/workspace/project-workbench/task-detail/task-urgency-panel.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing tests**
  - API path `/api/v1/tasks/{taskRef}/urgency?workspace=...`。
  - panel 显示 urgency 数字和 contribution 列表。
  - API 失败时不阻断任务详情页，只显示轻量错误。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/api/task-api.test.ts web/src/features/workspace/project-workbench/task-detail/task-urgency-panel.test.tsx`

- [x] **Step 3: Implement**
  - `TaskPropertyPanel` 内挂载 `TaskUrgencyPanel`。
  - 不把 urgency 放入默认列表列。

- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/api/task-api.test.ts web/src/features/workspace/project-workbench/task-detail/task-urgency-panel.test.tsx`

### Task 15: Workspace / Notification Console P2 fallback

**Files:**
- Modify: `web/src/features/workspace/resources/resource-dispatch.tsx`
- Create: `web/src/features/workspace/workspaces/workspace-console.tsx`
- Create: `web/src/features/workspace/workspaces/workspace-console.test.tsx`
- Create: `web/src/features/workspace/notifications/notification-console.tsx`
- Create: `web/src/features/workspace/notifications/notification-console.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] **Step 1: Write failing tests**
  - Workspace Console 显示归档按钮；unarchive 显示置灰说明。
  - Notification Console 使用 sink/rule/delivery 管控文案，不出现「已读 / 删除」收件箱语义。
  - 无对应 capability 时只读。

- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/workspaces/workspace-console.test.tsx web/src/features/workspace/notifications/notification-console.test.tsx`

- [x] **Step 3: Implement minimal consoles**
  - Workspace 第一版可复用 `/api/v1/workspaces` 列表和 `/archive`。
  - Notification 第一版可以先分 tab 展示 sinks、rules、deliveries；写操作只暴露后端已存在能力。

- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/workspaces/workspace-console.test.tsx web/src/features/workspace/notifications/notification-console.test.tsx`

### Task 16: Docs and release state

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-07-05-web-console-capability-bridge-design.md`
- Modify: `docs/superpowers/plans/2026-07-05-web-console-capability-bridge-implementation.md`

- [x] **Step 1: Update README**
  - Web Console 章节补充：
    - 侧栏身份块。
    - `/my-tasks`。
    - `/hooks` Hook Console。
    - `/projects/:slug/settings`。
    - 普通成员可查看 workspace 全量 audit。

- [x] **Step 2: Update ROADMAP**
  - 状态总览增加或更新 Web Console 能力桥接 milestone。
  - 描述清楚这是浏览器控制面补齐，不是 Linear/Jira 替代品。

- [x] **Step 3: Update spec/plan status**
  - spec 状态改为「已实施」。
  - plan 勾选实际完成项，未完成后端缺口保持未勾选或移到后续计划。

### Task 17: Full verification

**Files:**
- No new files.
- Validate all files touched by this plan.

- [x] **Step 1: Run frontend tests**
  - Run: `pnpm --dir web test`
  - Expected: all tests pass.

- [x] **Step 2: Run frontend typecheck**
  - Run: `pnpm --dir web typecheck`
  - Expected: no TypeScript errors.

- [x] **Step 3: Run frontend lint**
  - Run: `pnpm --dir web lint`
  - Expected: no ESLint errors.

- [x] **Step 4: Run frontend build**
  - Run: `pnpm --dir web build`
  - Expected: build succeeds.

- [x] **Step 5: Run editing smoke**
  - Run: `pnpm --dir web run smoke:editing`
  - Expected: smoke passes against the configured local server. If no server is running, start the documented local dev server first and record the URL.

- [x] **Step 6: Run Go tests**
  - Run: `go test ./...`
  - Expected: all tests pass.

- [x] **Step 7: Run CGO-free tests**
  - Run: `CGO_ENABLED=0 go test ./...`
  - Expected: all tests pass with pure-Go SQLite.

- [x] **Step 8: Run CGO-free build**
  - Run: `CGO_ENABLED=0 go build ./cmd/xuanchu`
  - Expected: build succeeds.

- [x] **Step 9: Run diff hygiene**
  - Run: `git diff --check`
  - Expected: no whitespace errors.

- [x] **Step 10: Final commit**
  ```bash
  git add README.md ROADMAP.md docs/superpowers/specs/2026-07-05-web-console-capability-bridge-design.md docs/superpowers/plans/2026-07-05-web-console-capability-bridge-implementation.md web internal
  git commit -m "feat: 完善 Web Console 能力桥接"
  ```

---

## Implementation Notes

- `internal/webconsole/dist` 不应提交真实构建产物；保持 `.gitkeep` 策略。
- 若 `pnpm --dir web build` 刷新了 dist，提交前确认 `.gitignore` 只留下允许的文件。
- 如果 Audit 全量可见的后端改动影响 token scope 默认生成，必须同步 token / browser session 相关测试，不要只改前端 capability 判断。
- 如果 Hook DTO 与计划假设不一致，以 `internal/httpapi/hooks.go` 和 `internal/remote/hook.go` 为准，先修正 API helper 测试，再实现 UI。
- 如果任一 Phase 需要新增后端业务能力（restore、unarchive、annotation PATCH、全量 audit actor/action/time 搜索），停止当前实现并先补 spec。
