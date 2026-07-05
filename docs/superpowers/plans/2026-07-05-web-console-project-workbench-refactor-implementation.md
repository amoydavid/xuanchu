# Web Console 项目工作台首页重构实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 重构项目工作台首页，让筛选、排序、新建任务和负责人摘要成为清晰、可组合、可验证的工作流。

**Architecture:** 页面继续 project-first：`ProjectWorkbenchPage` 负责数据和权限编排，新增 toolbar、create dialog、workload summary 等小组件承载交互。过滤和排序统一同步到 URL，再转换为 `/api/v1/tasks` 支持的 query 参数。

**Tech Stack:** React 19、TanStack Router、TanStack Query、shadcn/ui、lucide-react、Vitest、Testing Library。

---

## Chunk 1: 数据模型与测试

### Task 1: 扩展项目过滤模型

**Files:**
- Modify: `web/src/features/workspace/project-readonly/project-filter.ts`
- Modify: `web/src/routes/router.tsx`
- Test: `web/src/features/workspace/project-readonly/project-filter.test.ts`
- Test: `web/src/features/workspace/project-workbench/api/project-api.test.ts`

- [x] **Step 1: Write failing tests**
  - `filterToTaskQuery` 应保留 `sort`。
  - `filterToTaskQuery` 应把 `query`、`due_empty`、`assignee_empty`、`wait_before`、`scheduled_before`、`until_before` 编译进后端 `query` 表达式。
  - route `validateSearch` 应允许新增 search keys。
- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-readonly/project-filter.test.ts web/src/features/workspace/project-workbench/api/project-api.test.ts`
- [x] **Step 3: Implement filter model**
  - 扩展 `TaskFilter` 和 `FILTER_KEYS`。
  - `filterToTaskQuery` 继续输出 `status/priority/assignee/tags/q/due_after/due_before/sort`，并把低频条件合并为一个或多个 `query` 参数。
- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-readonly/project-filter.test.ts web/src/features/workspace/project-workbench/api/project-api.test.ts`

## Chunk 2: Toolbar 与排序

### Task 2: 新增 ProjectTaskToolbar

**Files:**
- Create: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx`

- [x] **Step 1: Write failing tests**
  - 渲染关键词、状态、优先级、负责人、标签、日期范围、排序和新建任务按钮。
  - 修改排序应更新 URL search 中的 `sort`。
  - 点击清除应移除对应筛选。
- [x] **Step 2: Run red test**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx`
- [x] **Step 3: Implement toolbar**
  - 使用 shadcn `Input`、`Select`、`Button`、`Badge`、`Popover`。
  - 移除旧 `ProjectFilterToolbar` 在工作台首页的使用。
- [x] **Step 4: Run green test**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx`

## Chunk 3: 新建任务弹窗

### Task 3: 替换行内创建为 TaskCreateDialog

**Files:**
- Create: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`
- Delete or leave unused: `web/src/features/workspace/project-workbench/tasks/task-quick-create.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx`
- Test: update `web/src/features/workspace/project-workbench/project/project-workbench-page.test.tsx`

- [x] **Step 1: Write failing tests**
  - 点击新建按钮打开弹窗。
  - 填写标题、描述、负责人、优先级、截止日期后提交完整 payload。
  - 空标题显示错误且不提交。
  - closed project 不显示新建任务入口。
- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx web/src/features/workspace/project-workbench/project/project-workbench-page.test.tsx`
- [x] **Step 3: Implement dialog**
  - 复用 `useCreateTaskMutation`、`getWorkspaceMembers`、`InlineDatePicker` 或同等 date picker。
  - 描述输入复用 `@/components/markdown` 的 `MarkdownEditor`，与任务详情页保持一致。
  - 保存失败保留弹窗。
- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx web/src/features/workspace/project-workbench/project/project-workbench-page.test.tsx`

## Chunk 4: 任务表格排序与负责人摘要

### Task 4: 表格和摘要体验

**Files:**
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.tsx`
- Create: `web/src/features/workspace/project-workbench/project/assignee-workload-summary.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
- Test: `web/src/features/workspace/project-workbench/project/assignee-workload-summary.test.tsx`

- [x] **Step 1: Write failing tests**
  - 表头能显示排序状态并触发排序。
  - 负责人摘要展示未完成、逾期、高优先级、即将到期。
  - 点击负责人写入 `assignee` 筛选，点击未分配写入 `assignee_empty`。
- [x] **Step 2: Run red tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/task-table.test.tsx web/src/features/workspace/project-workbench/project/assignee-workload-summary.test.tsx`
- [x] **Step 3: Implement components**
  - `TaskTable` 接收当前排序和排序回调。
  - `AssigneeWorkloadSummary` 使用无边框 list + 进度条。
- [x] **Step 4: Run green tests**
  - Run: `pnpm --dir web test web/src/features/workspace/project-workbench/tasks/task-table.test.tsx web/src/features/workspace/project-workbench/project/assignee-workload-summary.test.tsx`

## Chunk 5: 验证与收口

### Task 5: Full verification

**Files:**
- Modify: localization files if new text remains user-visible.
- Modify: remove stale imports/usages after replacing quick create/filter toolbar.

- [x] **Step 1: Run focused frontend tests**
  - Run: `pnpm --dir web test`
- [x] **Step 2: Run typecheck**
  - Run: `pnpm --dir web typecheck`
- [x] **Step 3: Run production build**
  - Run: `pnpm --dir web build`
- [x] **Step 4: Run diff check**
  - Run: `git diff --check`
- [ ] **Step 5: If Go API semantics changed, run Go verification**
  - 本次未修改 Go API 语义，未运行 Go 验证。
  - Run: `go test ./...`
  - Run: `CGO_ENABLED=0 go test ./...`
  - Run: `CGO_ENABLED=0 go build ./cmd/xuanchu`
