# Web Console 任务详情页低噪声默认态 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将普通任务详情页收敛为低噪声默认态，并统一执行、创建和局部编辑动作的层级。

**Architecture:** 保留既有 API 和编辑组件，只给 `TaskLinksEditor`、`SubTaskList` 增加可控展开状态；页面把状态提升后交给顶部更多菜单触发。活动与右栏只调整展示条件和样式，不增加服务端数据流。

**Tech Stack:** React 19、TypeScript、TanStack Query、Vitest、Testing Library、Tailwind/shadcn-ui。

## Global Constraints

- 所有用户可见文案以中文为主，英文 locale 同步。
- 不修改任务、子任务、链接、审计的 HTTP/API 契约。
- completed/deleted/关闭项目/无 `task:write` 时保持既有写权限限制。
- 先写失败测试，再写最小实现；最终运行全量 Web 验证。

---

### Task 1: 用例锁定与默认态测试

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.test.tsx`

- [x] 写失败测试：空 links/children 不显示区块；更多菜单可打开链接弹窗和子任务 composer；完成任务使用 `data-variant=default`；空计划折叠；注解编辑器按点击展开。
- [x] 运行：`pnpm --dir web test -- task-detail-page task-property-panel task-annotations-editor`，确认测试因缺少新行为失败。

### Task 2: 主区渐进披露与顶部创建入口

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-action-bar.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-links-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/sub-task-list.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [x] 给链接弹窗和子任务 composer 增加受控 `open`/`onOpenChange`（或等价）接口；空且未打开时返回 `null`。
- [x] `TaskDetailPage` 持有两类展开状态，传给组件与 `TaskActionBar`；移除标题区重复返回按钮。
- [x] 在更多菜单新增两个创建项；完成按钮改 `default`，更多按钮改 `ghost`，描述动作改 `ghost`。
- [x] 运行 Task 1 定向测试至通过。

### Task 3: 收紧活动和右栏

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-change-history.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx`

- [x] 增加活动 composer 的按需展开状态，提交成功后收起；移除空注解提示及注解/历史独立卡片外观。
- [x] 空计划分组默认折叠；从 `hasRelations` 中移除 `links`。
- [x] 运行 Task 1 定向测试及相邻组件测试至通过。

### Task 4: 完整验证与文档收尾

**Files:**
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-07-17-web-console-task-detail-default-density-design.md`
- Modify: `docs/superpowers/plans/2026-07-17-web-console-task-detail-default-density-implementation.md`

- [x] 将 v0.5.9 记录为此页面重构的进行中/已完成 milestone，并引用规格与计划。
- [x] 运行：`pnpm --dir web typecheck`、`pnpm --dir web test`、`pnpm --dir web lint`、`pnpm --dir web build`、`pnpm --dir web run smoke:editing`、`git diff --check`。
- [x] 审阅差异，确认只有本计划文件和页面重构文件被修改。
