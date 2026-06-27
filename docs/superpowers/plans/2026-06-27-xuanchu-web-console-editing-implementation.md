# Web Console Project/Task Editing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把当前只读项目/任务 Web Console 升级为项目上下文内的可编辑工作台，支持项目创建/编辑/状态转移、任务创建/inline 编辑、任务详情编辑、注解与链接管理。

**Architecture:** 复用现有 `/api/v1/*` 后端写接口，不新增业务后端能力；前端新增 `project-workbench` feature，按 `api/`、`hooks/`、`permissions/`、`shared/`、`projects/`、`project/`、`tasks/`、`task-detail/` 子目录拆分。React Query 负责缓存和 mutation invalidation，所有权限仍由后端最终裁决，前端只做写控件可见性的粗判断。

**Tech Stack:** Go 1.25, React 19, TanStack Router, TanStack Query, TypeScript, Vitest, Testing Library, shadcn/Radix UI, Tailwind CSS v4, lucide-react.

**执行结果（2026-06-27）：** 已完成实现并验证。前端新增 `project-workbench` feature，支持项目创建/编辑/状态转移、任务快速创建、任务表 inline 编辑、任务详情编辑、注解/链接管理和属性栏编辑。未删除 `project-readonly/`：其中 filter、stats、activity、UDA helper 仍被新工作台复用，纯重命名留到后续清理。首版 UDA 只编辑已有字段；depends/assignees/tags 使用轻量逗号输入，后续可替换成 picker。

**验证结果（2026-06-27）：**

```bash
pnpm --dir web test
pnpm --dir web typecheck
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
git diff --check
```

---

## Chunk 1: 基础 API、权限与共享编辑控件

### Task 1: 建立 `project-workbench` 目录与 API 客户端

**Files:**
- Create: `web/src/features/workspace/project-workbench/api/project-api.ts`
- Create: `web/src/features/workspace/project-workbench/api/task-api.ts`
- Create: `web/src/features/workspace/project-workbench/api/users-api.ts`
- Test: `web/src/features/workspace/project-workbench/api/project-api.test.ts`
- Test: `web/src/features/workspace/project-workbench/api/task-api.test.ts`

- [ ] **Step 1: 写 project API path builder 测试**

```ts
import { describe, expect, it } from "vitest"
import {
  projectPath,
  projectTasksPath,
  projectTransitionPath,
  projectsPath,
} from "./project-api"

describe("project workbench project api paths", () => {
  it("builds workspace-scoped project paths", () => {
    expect(projectsPath("local", "open")).toBe("/api/v1/projects?workspace=local&status=open")
    expect(projectPath("local", "adsops")).toBe("/api/v1/projects/adsops?workspace=local")
    expect(projectTransitionPath("local", "adsops")).toBe(
      "/api/v1/projects/adsops/transition?workspace=local"
    )
  })

  it("keeps task list filters readable", () => {
    expect(projectTasksPath("local", "adsops", "status=pending&q=copy")).toBe(
      "/api/v1/tasks?workspace=local&project=adsops&limit=200&status=pending&q=copy"
    )
  })
})
```

- [ ] **Step 2: 写 task API path builder 测试**

```ts
import { describe, expect, it } from "vitest"
import {
  taskAnnotationPath,
  taskDonePath,
  taskLinkPath,
  taskPath,
  taskStartPath,
} from "./task-api"

describe("project workbench task api paths", () => {
  it("builds workspace-scoped task mutation paths", () => {
    expect(taskPath("local", "ads-1")).toBe("/api/v1/tasks/ads-1?workspace=local")
    expect(taskDonePath("local", "ads-1")).toBe("/api/v1/tasks/ads-1/done?workspace=local")
    expect(taskStartPath("local", "ads-1")).toBe("/api/v1/tasks/ads-1/start?workspace=local")
    expect(taskAnnotationPath("local", "ads-1")).toBe(
      "/api/v1/tasks/ads-1/annotations?workspace=local"
    )
    expect(taskLinkPath("local", "ads-1")).toBe("/api/v1/tasks/ads-1/links?workspace=local")
  })
})
```

- [ ] **Step 3: 运行失败测试**

Run:

```bash
pnpm --dir web test -- project-workbench/api
```

Expected: FAIL，因为文件尚不存在。

- [ ] **Step 4: 实现 project API**

`project-api.ts` 应导出：

- `ProjectWorkbenchProject`
- `ProjectCreateInput`
- `ProjectModifyInput`
- `ProjectStatus`
- `projectsPath(workspaceSlug, status?)`
- `projectPath(workspaceSlug, projectRef)`
- `projectTasksPath(workspaceSlug, projectRef, filterQuery?)`
- `projectTimelinePath(workspaceSlug, projectRef)`
- `projectTransitionPath(workspaceSlug, projectRef)`
- `getProjects`
- `getProject`
- `createProject`
- `modifyProject`
- `transitionProject`

所有请求函数使用 `workspaceApiGet/Post/Patch`。

- [ ] **Step 5: 实现 task API**

`task-api.ts` 应导出：

- `ProjectTask`
- `TaskCreateInput`
- `TaskModifyInput`
- `TaskAnnotationInput`
- `TaskLinkInput`
- `taskPath`
- `taskStartPath`
- `taskStopPath`
- `taskDonePath`
- `taskAnnotationPath`
- `taskAnnotationItemPath`
- `taskLinkPath`
- `taskLinkItemPath`
- `createTask`
- `modifyTask`
- `startTask`
- `stopTask`
- `doneTask`
- `deleteTask`
- `addTaskAnnotation`
- `deleteTaskAnnotation`
- `addTaskLink`
- `deleteTaskLink`

所有 mutation 成功返回后端响应，但页面层仍要 invalidate/refetch。

- [ ] **Step 6: 实现 users API**

`users-api.ts` 使用 `GET /api/v1/workspaces/{workspace}/members` 作为首选 picker 数据源，导出：

- `WorkspaceMemberCandidate`
- `workspaceMembersPath(workspaceSlug)`
- `getWorkspaceMembers(workspaceSlug)`

- [ ] **Step 7: 运行测试**

Run:

```bash
pnpm --dir web test -- project-workbench/api
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/workspace/project-workbench/api
git commit -m "feat: 添加项目工作台 API 客户端"
```

### Task 2: 实现前端权限粗判断

**Files:**
- Create: `web/src/features/workspace/project-workbench/permissions/permissions.ts`
- Create: `web/src/features/workspace/project-workbench/permissions/permissions.test.ts`
- Modify: `web/src/features/workspace/session/useMe.ts`

- [ ] **Step 1: 写权限测试**

```ts
import { describe, expect, it } from "vitest"
import { canProjectManage, canTaskWrite, hasScope } from "./permissions"

describe("project workbench permissions", () => {
  it("allows wildcard scopes", () => {
    expect(hasScope(["*"], "task:write")).toBe(true)
  })

  it("allows task writes for member with task:write", () => {
    expect(canTaskWrite({ role: "member", scopes: ["task:write"] })).toBe(true)
  })

  it("denies task writes for viewer even with task:write", () => {
    expect(canTaskWrite({ role: "viewer", scopes: ["task:write"] })).toBe(false)
  })

  it("allows project manage for owner/admin only", () => {
    expect(canProjectManage({ role: "admin", scopes: ["project:write"] })).toBe(true)
    expect(canProjectManage({ role: "member", scopes: ["project:write"] })).toBe(false)
  })
})
```

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- project-workbench/permissions
```

Expected: FAIL，因为模块尚不存在。

- [ ] **Step 3: 实现权限函数**

`permissions.ts`：

```ts
export type WorkbenchPermissionInput = {
  role?: string | null
  scopes?: string[] | null
}

export function hasScope(scopes: string[] | null | undefined, scope: string): boolean {
  return Array.isArray(scopes) && (scopes.includes("*") || scopes.includes(scope))
}

export function canTaskWrite(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "task:write") &&
    ["owner", "admin", "member"].includes(input.role ?? "")
  )
}

export function canProjectManage(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "project:write") &&
    ["owner", "admin"].includes(input.role ?? "")
  )
}
```

- [ ] **Step 4: 确认 `useMe.ts` 类型够用**

`useMe.ts` 已有 `token.scopes` 与 `effective_role`。仅当测试或类型需要时扩展 `MeResponse` 的 workspace id/name，不要为了本任务新增后端字段。

- [ ] **Step 5: 运行测试**

Run:

```bash
pnpm --dir web test -- project-workbench/permissions
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/permissions web/src/features/workspace/session/useMe.ts
git commit -m "feat: 添加项目工作台权限判断"
```

### Task 3: 建立共享 inline 编辑控件

**Files:**
- Create: `web/src/features/workspace/project-workbench/shared/inline-text-editor.tsx`
- Create: `web/src/features/workspace/project-workbench/shared/inline-text-editor.test.tsx`
- Create: `web/src/features/workspace/project-workbench/shared/inline-select-editor.tsx`
- Create: `web/src/features/workspace/project-workbench/shared/inline-date-editor.tsx`
- Create: `web/src/features/workspace/project-workbench/shared/destructive-confirm-dialog.tsx`

- [ ] **Step 1: 写 inline text editor 测试**

```tsx
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"
import { InlineTextEditor } from "./inline-text-editor"

describe("InlineTextEditor", () => {
  it("saves edited value with Enter", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(<InlineTextEditor value="旧标题" ariaLabel="标题" onSave={onSave} />)

    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.clear(screen.getByLabelText("标题"))
    await userEvent.type(screen.getByLabelText("标题"), "新标题{Enter}")

    expect(onSave).toHaveBeenCalledWith("新标题")
  })

  it("keeps editing state when save fails", async () => {
    const onSave = vi.fn().mockRejectedValue(new Error("scope denied"))
    render(<InlineTextEditor value="旧标题" ariaLabel="标题" onSave={onSave} />)

    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.clear(screen.getByLabelText("标题"))
    await userEvent.type(screen.getByLabelText("标题"), "新标题{Enter}")

    expect(await screen.findByText(/scope denied/)).toBeTruthy()
    expect(screen.getByLabelText("标题")).toHaveValue("新标题")
  })
})
```

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- inline-text-editor
```

Expected: FAIL.

- [ ] **Step 3: 实现 `InlineTextEditor`**

要求：

- display 模式渲染文本。
- click 进入编辑。
- Enter 保存，Esc 取消。
- `multiline` 时用 textarea，Cmd/Ctrl+Enter 保存。
- `validate` 返回字符串时不提交并显示字段错误。
- `disabled` 时不可进入编辑。
- 保存失败时保留输入并显示错误。

- [ ] **Step 4: 实现 select/date/confirm 基础组件**

`inline-select-editor.tsx`：

- props: `value`, `options`, `placeholder`, `disabled`, `onSave`
- 使用现有 `Select` 组件。
- 选中后立即保存。

`inline-date-editor.tsx`：

- props: `value?: number | null`, `disabled`, `onSave`
- 首版使用原生 `<input type="date">`，保存时转 unix 秒。
- 清空提交 `null`，由调用方转换为 `clear_due` 等 payload。

`destructive-confirm-dialog.tsx`：

- props: `open`, `title`, `description`, `confirmLabel`, `pending`, `onConfirm`, `onOpenChange`
- 使用现有 `AlertDialog`。

- [ ] **Step 5: 运行共享控件测试**

Run:

```bash
pnpm --dir web test -- project-workbench/shared
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/shared
git commit -m "feat: 添加项目工作台内联编辑控件"
```

## Chunk 2: 项目列表与项目工作台

### Task 4: 实现项目 mutation hooks

**Files:**
- Create: `web/src/features/workspace/project-workbench/hooks/use-project-mutations.ts`
- Create: `web/src/features/workspace/project-workbench/hooks/use-project-data.ts`
- Test: `web/src/features/workspace/project-workbench/hooks/use-project-mutations.test.tsx`

- [ ] **Step 1: 写 mutation invalidate 测试**

测试目标：`modifyProject` 成功后 invalidates `["project", workspaceSlug, projectSlug]` 与 `["projects", workspaceSlug]`。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- use-project-mutations
```

Expected: FAIL.

- [ ] **Step 3: 实现 hooks**

导出：

- `useProjectsQuery(workspaceSlug, statusFilter)`
- `useProjectQuery(workspaceSlug, projectSlug)`
- `useProjectTimelineQuery(workspaceSlug, projectSlug)`
- `useCreateProjectMutation()`
- `useModifyProjectMutation(workspaceSlug, projectSlug)`
- `useTransitionProjectMutation(workspaceSlug, projectSlug)`

invalidate 规则：

- create: invalidate `["projects", workspaceSlug]`
- modify: invalidate project + projects
- transition: invalidate project + projects + timeline + tasks

- [ ] **Step 4: 运行测试**

Run:

```bash
pnpm --dir web test -- project-workbench/hooks
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/project-workbench/hooks
git commit -m "feat: 添加项目工作台 mutation hooks"
```

### Task 5: 替换项目列表页并支持新建项目

**Files:**
- Create: `web/src/features/workspace/project-workbench/projects/projects-list-page.tsx`
- Create: `web/src/features/workspace/project-workbench/projects/project-create-dialog.tsx`
- Create: `web/src/features/workspace/project-workbench/projects/project-row-actions.tsx`
- Test: `web/src/features/workspace/project-workbench/projects/projects-list-page.test.tsx`
- Modify: `web/src/routes/workspace/ProjectsListRoute.tsx`
- Modify: `web/src/features/workspace/projects/projects-list-page.tsx` or remove imports after route migration
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写项目列表测试**

覆盖：

- 渲染项目行。
- 点击“新建项目”打开 dialog。
- 提交后调用 `POST /api/v1/projects`。
- 成功后 navigate 到 `/workspaces/$workspaceSlug/projects/$projectSlug`。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- projects-list-page
```

Expected: FAIL.

- [ ] **Step 3: 实现 `ProjectCreateDialog`**

字段：

- slug: 必填，前端基础校验 `^[a-z][a-z0-9]{2,9}$`
- name: 必填
- description: 可选 textarea

失败时保留输入，错误显示在 dialog 内。

- [ ] **Step 4: 实现新项目列表页**

使用 `useProjectsQuery(workspaceSlug, "all")` 或保留当前 `all=true` 行为，但新 API helper 统一生成 path。布局沿用现有 `DataTable` 或 shadcn table，行点击进入项目页。

- [ ] **Step 5: 更新路由指向**

`web/src/routes/workspace/ProjectsListRoute.tsx` 改为导入新页面。旧 `web/src/features/workspace/projects/*` 暂不删除，等后续任务确认无引用后清理。

- [ ] **Step 6: 添加 i18n 文案**

至少包含：

- `projectWorkbench.createProject`
- `projectWorkbench.field.slug`
- `projectWorkbench.field.name`
- `projectWorkbench.field.description`
- `projectWorkbench.validation.slug`

- [ ] **Step 7: 运行测试**

Run:

```bash
pnpm --dir web test -- projects-list-page
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/workspace/project-workbench/projects web/src/routes/workspace/ProjectsListRoute.tsx web/src/locales
git commit -m "feat: 项目列表支持新建项目"
```

### Task 6: 实现项目 header inline 编辑与状态菜单

**Files:**
- Create: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-header-editor.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-status-menu.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-settings-dialog.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-closed-banner.tsx`
- Test: `web/src/features/workspace/project-workbench/project/project-header-editor.test.tsx`
- Test: `web/src/features/workspace/project-workbench/project/project-status-menu.test.tsx`
- Modify: `web/src/routes/workspace/ProjectReadonlyRoute.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写 header 编辑测试**

覆盖：

- 点击项目名进入 inline edit。
- Enter 保存并调用 `PATCH /api/v1/projects/{projectRef}` `{name}`。
- description 清空可保存为空字符串。
- `canProjectManage=false` 时显示纯文本。

- [ ] **Step 2: 写状态菜单测试**

覆盖：

- planning -> active 直接提交。
- active -> archived 弹确认。
- archived -> active 显示“恢复后可写”提示并提交。

- [ ] **Step 3: 运行失败测试**

Run:

```bash
pnpm --dir web test -- project-workbench/project
```

Expected: FAIL.

- [ ] **Step 4: 实现项目 header**

`project-header-editor.tsx` props:

- `project`
- `workspaceSlug`
- `canManage`
- `onCopyLink`

内部使用 `InlineTextEditor` 保存 name，`InlineTextEditor multiline` 保存 description。

- [ ] **Step 5: 实现状态菜单和 closed banner**

`project-status-menu.tsx` 使用 `Select` 或 `DropdownMenu`。closed 状态确认用 `DestructiveConfirmDialog`。

`project-closed-banner.tsx` 在 status 为 `archived` / `cancelled` 时展示，并接受 `canManage` 来决定是否提示可恢复。

- [ ] **Step 6: 实现项目设置 dialog**

首版只放 slug 修改。slug 修改成功后：

1. invalidate projects。
2. navigate 到新 slug 路由。

- [ ] **Step 7: 新页面复用旧浏览内容**

`project-workbench-page.tsx` 先复用旧 `ProjectFilterToolbar`、任务列表、timeline 组件；本任务只替换 header 体验。不要在这个任务同时实现任务写操作。

- [ ] **Step 8: 路由迁移**

`ProjectReadonlyRoute.tsx` 改为渲染 `ProjectWorkbenchPage`。文件名可暂不改，避免路由层大改。

- [ ] **Step 9: 运行测试**

Run:

```bash
pnpm --dir web test -- project-workbench/project
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add web/src/features/workspace/project-workbench/project web/src/routes/workspace/ProjectReadonlyRoute.tsx web/src/locales
git commit -m "feat: 项目页支持内联编辑与状态转移"
```

## Chunk 3: 任务创建、任务表 inline 编辑

### Task 7: 实现 task mutation hooks

**Files:**
- Create: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`
- Create: `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`
- Test: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.test.tsx`

- [ ] **Step 1: 写 hooks 测试**

覆盖：

- create task invalidates project tasks + project summary。
- modify task invalidates task detail + project tasks。
- done/delete invalidates project tasks + project timeline。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- use-task-mutations
```

Expected: FAIL.

- [ ] **Step 3: 实现 hooks**

导出：

- `useCreateTaskMutation(workspaceSlug, projectSlug, filterQuery)`
- `useModifyTaskMutation(workspaceSlug, projectSlug, taskRef?)`
- `useTaskActionMutation(workspaceSlug, projectSlug, action)`
- `useTaskAnnotationMutations(workspaceSlug, projectSlug, taskRef)`
- `useTaskLinkMutations(workspaceSlug, projectSlug, taskRef)`

不要在 hook 内吞掉错误；组件需要拿到错误展示字段级状态。

- [ ] **Step 4: 运行测试**

Run:

```bash
pnpm --dir web test -- project-workbench/hooks/use-task-mutations
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/project-workbench/hooks
git commit -m "feat: 添加任务编辑 mutation hooks"
```

### Task 8: 实现任务快速创建 composer

**Files:**
- Create: `web/src/features/workspace/project-workbench/tasks/task-quick-create.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/task-quick-create.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写快速创建测试**

覆盖：

- 输入 title 后 Enter 创建。
- 创建 payload 包含当前 project。
- 创建成功后输入框清空。
- closed project 或 `canTaskWrite=false` 时不显示 composer。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- task-quick-create
```

Expected: FAIL.

- [ ] **Step 3: 实现 `TaskQuickCreate`**

首版字段：

- title 单行输入。
- priority select。
- due date。
- assignees picker 可选；若 picker 复杂度过高，先支持 title/priority/due，assignees 放到任务表与详情。

创建 payload：

```ts
{
  title,
  project: projectSlug,
  priority,
  due,
  assignees
}
```

- [ ] **Step 4: 接入项目页**

放在过滤工具栏和任务表之间。当前过滤条件隐藏新任务时显示提示：

```text
任务已创建，但被当前过滤条件隐藏。
```

- [ ] **Step 5: 运行测试**

Run:

```bash
pnpm --dir web test -- task-quick-create
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/tasks web/src/features/workspace/project-workbench/project web/src/locales
git commit -m "feat: 项目页支持快速创建任务"
```

### Task 9: 实现任务表 inline 编辑和行操作

**Files:**
- Create: `web/src/features/workspace/project-workbench/tasks/task-table.tsx`
- Create: `web/src/features/workspace/project-workbench/tasks/task-row-actions.tsx`
- Create: `web/src/features/workspace/project-workbench/shared/assignee-picker.tsx`
- Create: `web/src/features/workspace/project-workbench/shared/tag-editor.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写任务表测试**

覆盖：

- title inline 保存调用 PATCH `{title}`。
- priority select 保存调用 PATCH `{priority}`。
- due 清空调用 PATCH `{clear_due:true}`。
- row action “完成”调用 POST `/done`。
- delete action 有确认。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- task-table
```

Expected: FAIL.

- [ ] **Step 3: 实现 `TaskTable`**

列：

- ID
- title
- status
- priority
- assignees
- due
- actions

桌面端 inline 编辑，移动端保留卡片 + 行末菜单，复杂编辑进入详情页。

- [ ] **Step 4: 实现 row actions**

菜单项：

- 打开详情
- 复制任务链接
- 添加注解（可先打开 prompt-like small dialog，最终详情页也支持）
- 添加链接
- 删除任务

动作按钮：

- pending 未 start: start / done
- pending 已 start: stop / done
- completed: 不显示 start/stop/done

- [ ] **Step 5: 接入项目页**

用 `TaskTable` 替换旧 `ProjectTaskList`。保留旧组件直到全量迁移完成后清理。

- [ ] **Step 6: 运行测试**

Run:

```bash
pnpm --dir web test -- task-table
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/workspace/project-workbench/tasks web/src/features/workspace/project-workbench/shared web/src/features/workspace/project-workbench/project web/src/locales
git commit -m "feat: 任务表支持内联编辑"
```

## Chunk 4: 任务详情完整编辑

### Task 10: 迁移任务详情页并支持标题/描述/动作编辑

**Files:**
- Create: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Create: `web/src/features/workspace/project-workbench/task-detail/task-action-bar.tsx`
- Test: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/routes/workspace/ProjectTaskDetailRoute.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写详情页编辑测试**

覆盖：

- title inline 保存。
- description 清空提交 `{clear_description:true}`。
- start/stop/done/delete 调用正确 API。
- delete 有确认。
- completed 任务不显示 start/stop/done。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- task-detail-page
```

Expected: FAIL.

- [ ] **Step 3: 实现 task detail page**

从旧 `project-readonly/project-task-detail-page.tsx` 迁移布局和 formatting helper。保留：

- belongs-to-project 保护。
- annotations 懒加载。
- links 展示。
- depends/parent/blocking 链接展示。
- UDA 展示。

新增：

- title inline。
- description inline。
- action bar。

- [ ] **Step 4: 路由迁移**

`ProjectTaskDetailRoute.tsx` 改为渲染新 `TaskDetailPage`。

- [ ] **Step 5: 运行测试**

Run:

```bash
pnpm --dir web test -- task-detail-page
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/task-detail web/src/routes/workspace/ProjectTaskDetailRoute.tsx web/src/locales
git commit -m "feat: 任务详情支持核心字段编辑"
```

### Task 11: 实现注解和链接编辑

**Files:**
- Create: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.tsx`
- Create: `web/src/features/workspace/project-workbench/task-detail/task-links-editor.tsx`
- Test: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.test.tsx`
- Test: `web/src/features/workspace/project-workbench/task-detail/task-links-editor.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写注解编辑测试**

覆盖：

- 添加注解调用 POST `{description}`。
- 空注解不提交。
- 删除注解有确认并调用 DELETE。
- 失败时保留输入。

- [ ] **Step 2: 写链接编辑测试**

覆盖：

- 添加链接 dialog 要求 type/url。
- URL 基础格式错误不提交。
- 删除链接有确认。

- [ ] **Step 3: 运行失败测试**

Run:

```bash
pnpm --dir web test -- task-annotations-editor task-links-editor
```

Expected: FAIL.

- [ ] **Step 4: 实现注解编辑器**

复用现有 lazy loading 行为。新增 composer 在注解列表顶部，删除按钮在每条注解行末。

- [ ] **Step 5: 实现链接编辑器**

字段：

- type
- url
- title

新增成功后 refetch task detail。删除成功后 refetch。

- [ ] **Step 6: 接入详情页**

替换旧 `TaskAnnotationsLazy` 和 `TaskLinks` 的静态版本。

- [ ] **Step 7: 运行测试**

Run:

```bash
pnpm --dir web test -- task-annotations-editor task-links-editor
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/workspace/project-workbench/task-detail web/src/locales
git commit -m "feat: 任务详情支持注解和链接编辑"
```

### Task 12: 实现右侧属性栏编辑

**Files:**
- Create: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx`
- Create: `web/src/features/workspace/project-workbench/task-detail/task-dependency-picker.tsx`
- Test: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写属性栏测试**

覆盖：

- priority 保存。
- due 保存与清空。
- tags 添加/移除。
- assignees 保存与清空。
- UDA 已有字段编辑。
- parent 只读展示，不出现可编辑保存入口。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- task-property-panel
```

Expected: FAIL.

- [ ] **Step 3: 实现属性栏**

可编辑：

- priority
- due
- assignees
- tags
- wait
- scheduled
- until
- recur
- existing UDA values
- depends

不可编辑：

- status 只显示，动作在 action bar。
- parent 只读展示，因为当前 HTTP modify request 没有 parent 字段。
- blocked_by 只读展示。

- [ ] **Step 4: 实现 dependency picker**

首版只列当前项目任务，支持多选 depends。提交：

- 有值：`{depends:[taskRef...]}`
- 清空：`{clear_depends:true}`

- [ ] **Step 5: 接入详情页**

替换旧 `TaskSidePanel`。

- [ ] **Step 6: 运行测试**

Run:

```bash
pnpm --dir web test -- task-property-panel
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/workspace/project-workbench/task-detail web/src/locales
git commit -m "feat: 任务详情支持属性栏编辑"
```

## Chunk 5: 清理、文档与全量验证

### Task 13: 清理旧 readonly 命名和无引用文件

**Files:**
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/routes/workspace/ProjectReadonlyRoute.tsx`
- Modify: `web/src/routes/workspace/ProjectTaskDetailRoute.tsx`
- Potentially delete: `web/src/features/workspace/project-readonly/*` only after confirming no imports remain
- Potentially delete: `web/src/features/workspace/projects/*` only after confirming no imports remain

- [ ] **Step 1: 搜索旧引用**

Run:

```bash
rg -n "project-readonly|ProjectReadonly|features/workspace/projects" web/src
```

Expected: 只剩历史测试或无引用文件。

- [ ] **Step 2: 决定是否改 route 名**

不强制重命名 URL。内部组件名可以改为 Workbench，但路径继续保持：

```text
/workspaces/:workspaceSlug/projects/:projectSlug
/workspaces/:workspaceSlug/projects/:projectSlug/tasks/:taskRef
```

- [ ] **Step 3: 删除无引用旧文件**

只删除确认无 import 的旧文件。不要删除测试覆盖仍依赖的 helper，先迁移测试再删。

- [ ] **Step 4: 运行类型检查**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "chore: 清理项目工作台旧只读实现"
```

### Task 14: 同步 README 和 spec 状态

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md` if this work is tied to a roadmap item
- Modify: `docs/superpowers/specs/2026-06-27-xuanchu-web-console-editing-design.md`
- Modify: `docs/superpowers/plans/2026-06-27-xuanchu-web-console-editing-implementation.md`

- [ ] **Step 1: 更新 README Web Console 段落**

把“只读项目深链”改为说明：

- 项目页支持 project/task 编辑。
- 写操作仍受 token scope、membership、project allowlist、closed project 状态约束。
- closed project 只允许管理者恢复状态。

- [ ] **Step 2: 更新 spec 状态**

实现完成后把 spec 状态从“草案”改为“已完成”，并记录实际取舍：

- 是否支持 UDA 新增/删除。
- depends 是否只支持当前项目。
- 是否做了目录重命名。

- [ ] **Step 3: 更新 plan 勾选状态**

执行过程中逐步勾选，不要到最后一次性全勾。

- [ ] **Step 4: Commit**

```bash
git add README.md ROADMAP.md docs/superpowers/specs/2026-06-27-xuanchu-web-console-editing-design.md docs/superpowers/plans/2026-06-27-xuanchu-web-console-editing-implementation.md
git commit -m "docs: 同步 Web Console 编辑体验文档"
```

### Task 15: 全量验证

**Files:**
- No code changes unless verification reveals failures.

- [ ] **Step 1: 前端 typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS.

- [ ] **Step 2: 前端测试**

Run:

```bash
pnpm --dir web test
```

Expected: PASS.

- [ ] **Step 3: 前端构建**

Run:

```bash
pnpm --dir web build
```

Expected: PASS. 不提交 `internal/webconsole/dist` 构建产物，除非发布流程明确要求。

- [ ] **Step 4: Go 测试**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: 零 CGO 测试**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS.

- [ ] **Step 6: 零 CGO 构建**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS.

- [ ] **Step 7: go vet**

Run:

```bash
go vet ./...
```

Expected: PASS.

- [ ] **Step 8: diff 检查**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 9: 最终提交**

如果前面按任务提交，本步骤只提交验证修复或文档勾选更新：

```bash
git status --short
git add <changed-files>
git commit -m "chore: 完成 Web Console 编辑体验验证"
```
