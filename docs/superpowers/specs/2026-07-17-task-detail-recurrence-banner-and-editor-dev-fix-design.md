# 任务详情循环提示栏与描述编辑开发态修复设计

**日期：** 2026-07-17
**状态：** 已实现
**范围：** 仅修复 Workspace Web Console 任务详情页的循环实例提示栏布局，以及 Vite 开发服务器中 Markdown 描述编辑器的 React Hook 错误。

## 1. 问题与目标

循环实例的 `RecurrenceContextAlert` 目前位于标题左侧列，右侧操作区会使它无法与详情内容同宽。与此同时，在常驻 `pnpm dev` 服务中首次打开 Markdown 描述编辑器会触发 `Invalid hook call`，堆栈落在 `@tiptap/react` 的 `useEditor/useRef`。

本次目标是让循环实例提示栏占满标题区内容宽度，并保证 Vite 开发态下主应用和 Tiptap 始终使用同一个 React/React DOM 模块实例。

## 2. 设计决策

### 2.1 提示栏布局

- 标题、状态徽标与任务动作仍保留在既有桌面端 flex 行。
- `RecurrenceContextAlert` 移出该 flex 行，放在同一个 header section 中的下一行，并使用 `mt-4 w-full`。
- 不修改循环实例、系列链接、日期格式或移动端逻辑。

### 2.2 React 运行时一致性

- 在 `web/vite.config.ts` 的 `resolve` 中加入 `dedupe: ["react", "react-dom"]`。
- 该配置要求 Vite 对预构建依赖（包括 `@tiptap/react`）复用应用的 React 单例，避免 Hook dispatcher 为 `null`。
- 变更后使用 `pnpm dev -- --force` 重建 Vite 优化依赖缓存；不删除任务数据或仓库依赖。

## 3. 验收与非目标

- 循环任务详情的 `occurrence-banner` 不在标题左列内，横向覆盖标题和操作区共同的内容宽度。
- 描述编辑对话框在常驻 Vite 开发服务中可正常打开，且不出现 `Invalid hook call` 或 `useRef` 错误。
- 保持描述 Markdown 持久化契约；Tiptap 仅作为编辑器运行时。
- 用 Vitest 锁定标题区 DOM 结构和 Vite 的 React 去重配置。
- 不改后端、循环任务 series/occurrence 语义、国际化文案或描述编辑器功能。
