# 任务详情循环提示栏与描述编辑开发态修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让循环实例提示栏占满任务详情标题区，并消除 Vite 开发态下 Markdown 描述编辑器的 React Hook 错误。

**Architecture:** 页面仅调整 `RecurrenceContextAlert` 的父级位置，令它脱离桌面端标题/动作 flex 行；不改变 alert 组件或循环业务数据。Vite 在模块解析阶段去重 React 与 React DOM，使 Tiptap 和应用入口在优化依赖后共享同一运行时。

**Tech Stack:** React 19、TypeScript、Vite 8、Vitest、Testing Library、Tailwind。

## Global Constraints

- 循环任务继续使用既有 `series` / `occurrence` 模型，不修改 API 或数据语义。
- 任务描述继续持久化为 Markdown 字符串，Tiptap 仅为编辑器运行时。
- 不删除任务数据、`node_modules` 或用户未跟踪的 `docs/business/`。
- 每项改动先写失败测试，再做最小实现；开发态验证使用 `pnpm dev -- --force` 重建 Vite 优化依赖缓存。

---

### Task 1: 锁定循环提示栏的全宽结构

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`

**Interfaces:**
- Consumes: `TaskDetailPage` 的既有 `ProjectTask.recurrence_info` 和 `RecurrenceContextAlert`。
- Produces: `data-testid="occurrence-banner"` 作为 header section 内、标题/动作 flex 行之后的直接后代。

- [x] **Step 1: 写失败测试**

```tsx
const banner = await screen.findByTestId("occurrence-banner")
expect(banner.parentElement?.className).toContain("w-full")
expect(banner.parentElement?.previousElementSibling?.className).toContain("md:flex-row")
```

- [x] **Step 2: 运行测试并确认失败**

Run: `pnpm --dir web test -- task-detail-page.test.tsx`

Expected: FAIL，因为 banner 的父级仍是标题左侧 `min-w-0 flex-1` 容器。

- [x] **Step 3: 实现最小布局调整**

```tsx
        <div className="mt-3 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          {/* 标题、徽标和操作区 */}
        </div>
        <div className="mt-4 w-full">
          <RecurrenceContextAlert ... />
        </div>
```

- [x] **Step 4: 运行测试并确认通过**

Run: `pnpm --dir web test -- task-detail-page.test.tsx`

Expected: PASS。

### Task 2: 固定 Vite 的 React 单例解析

**Files:**
- Modify: `web/vite.config.test.ts`
- Modify: `web/vite.config.ts`

**Interfaces:**
- Consumes: Vite `resolve` 配置与 `@tiptap/react` 的 React peer dependency。
- Produces: `resolve.dedupe` 同时包含 `react` 与 `react-dom`。

- [x] **Step 1: 写失败测试**

```ts
expect(config.resolve?.dedupe).toEqual(["react", "react-dom"])
```

- [x] **Step 2: 运行测试并确认失败**

Run: `pnpm --dir web test -- vite.config.test.ts`

Expected: FAIL，因为当前 Vite 配置没有 `resolve.dedupe`。

- [x] **Step 3: 实现最小配置**

```ts
resolve: {
  alias: { "@": path.resolve(__dirname, "./src") },
  dedupe: ["react", "react-dom"],
},
```

- [x] **Step 4: 运行测试并确认通过**

Run: `pnpm --dir web test -- vite.config.test.ts`

Expected: PASS。

### Task 3: 开发态与回归验证

**Files:**
- Verify only: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Verify only: `web/vite.config.ts`

- [x] **Step 1: 强制重建 Vite 优化依赖缓存**

Run: `pnpm --dir web dev -- --force`

Expected: 9095 服务启动且输出 `Forced re-optimization of dependencies` 或等价重建提示。

- [x] **Step 2: 在任务页复测**

Open: `http://localhost:9095/workspaces/local/projects/ops/tasks/ops-12`

Expected: 循环提示栏与标题区同宽；点击「编辑描述」出现可编辑的对话框，无 Error Boundary 或 `Invalid hook call`。

- [x] **Step 3: 运行相关质量检查**

Run: `pnpm --dir web typecheck && pnpm --dir web lint && pnpm --dir web test && git diff --check`

Expected: 类型检查、lint、测试和空白检查通过。若 `build` 仍仅因既有 `tokens/scopes.test.ts` 的 Node 类型配置失败，记录为基线问题，不归因于本修复。
