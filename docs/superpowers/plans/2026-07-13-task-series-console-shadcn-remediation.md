# 循环任务 Web Console shadcn 收口 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 My Tasks 缺失翻译和循环任务原型 UI，将创建、管理、详情、停止流程统一到现有 shadcn 设计系统，并完成中英文、可访问性与浏览器视觉验收。

**Architecture:** 把 `TaskSeriesDialog` 的业务状态和表单内容下沉为无 modal 语义的 `TaskSeriesForm`，由统一 `TaskCreateDialog` 和独立 `TaskSeriesEditorDialog` 提供 Dialog 容器；列表与详情继续由 URL 驱动的 `TaskSeriesPanelShell` 承载。编辑规则所需的默认切换槽位由 App 层统一计算，并通过 HTTP、MCP、CLI、Remote DTO 和 Web adapter 传递，前端不从 `next_recurrence_at` 猜测。

**Tech Stack:** React 19、TypeScript、TanStack Router/Query、react-i18next、shadcn/Radix、Vitest、Testing Library、Playwright。

## Global Constraints

- 文档、用户可见中文和代码注释以中文为主；所有用户文案必须同时提供 `zh-CN` / `en-US` key。
- 复用 `web/src/components/ui/*` 和现有 shared editors，不新增第二套基础组件或全局视觉 token。
- 同一创建流程只能渲染一个 `role=dialog`；`TaskSeriesForm` 不得渲染 Dialog/Sheet。
- 右侧循环管理栏桌面使用 `w-full lg:w-80 lg:shrink-0`，与项目上下文栏一致。
- HTTP、MCP、CLI、Remote 与 Web 必须共享 Task Series 字段和规则修改约束，不得在传输层自行推算槽位。

---

### Task 1: 修复 My Tasks i18n 与预设 Tabs

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/pages/my-tasks-page.tsx`
- Test: `web/src/features/workspace/my-tasks/my-task-tabs.test.ts`
- Test: `web/src/pages/my-tasks-page.test.tsx`

**Interfaces:**
- Consumes: `MY_TASK_TABS` 的 `incomplete|today|overdue|noDue|completed`。
- Produces: `myTasks.tab.*` 完整翻译和 shadcn Tabs UI。

- [x] **Step 1: 写失败测试**

新增 My Tasks 页面测试，真实切换 `zh-CN/en-US`，断言五个 tab 的可见名称、`tablist/tab` role，并断言 DOM 不包含 `myTasks.tab.`。断言页面不存在与 preset 冲突的 status combobox。

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test src/pages/my-tasks-page.test.tsx src/features/workspace/my-tasks/my-task-tabs.test.ts`

Expected: FAIL，旧页面显示原始 `myTasks.tab.incomplete/completed` 且仍有 status selector。

- [x] **Step 3: 最小实现**

为两个语言包增加 `incomplete/completed` 并删除未使用 `all`；页面改用 `Tabs/TabsList/TabsTrigger`，移除 status state 与 status Select，让 `tabFilter()` 成为唯一状态范围来源；保留搜索、优先级、排序。

- [x] **Step 4: 定向验证**

Run: `pnpm --dir web test src/pages/my-tasks-page.test.tsx src/features/workspace/my-tasks/my-task-tabs.test.ts`

Expected: PASS。

### Task 2: 建立循环任务 i18n 与共享格式化契约

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/features/workspace/project-workbench/task-series/recurrence-preview.ts`
- Test: `web/src/i18n.test.ts`
- Test: `web/src/features/workspace/project-workbench/task-series/recurrence-preview.test.ts`

**Interfaces:**
- Produces: `taskSeries.*` 文案树；`recurrenceRuleLabel(rule, t)`、`taskSeriesStatusLabel(status, t)`、本地日期格式化入口。

- [x] **Step 1: 写失败测试**

断言中英文存在 create/edit/list/detail/stop/status/rule/aria/error key；断言 canonical rule/status 在两种语言下映射为用户文案，未知值安全回退。

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test src/i18n.test.ts src/features/workspace/project-workbench/task-series/recurrence-preview.test.ts`

Expected: FAIL，`taskSeries.*` 尚不存在。

- [x] **Step 3: 最小实现并验证**

增加完整语言包和共享 label/date helper，禁止组件直接渲染 `active|ended|stopped|daily` 与 Unix 秒。

Run: 同 Step 2，Expected: PASS。

### Task 3: 拆分共享 TaskSeriesForm，消除嵌套 Dialog

**Files:**
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-form.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-editor-dialog.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-dialog.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.tsx`
- Test: `web/src/features/workspace/project-workbench/task-series/task-series-dialog.test.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx`

**Interfaces:**
- `TaskSeriesForm({mode, workspaceSlug, projectSlug, series, onCancel, onCreated, onSaved})` 渲染字段和 footer，不渲染 modal。
- `TaskSeriesEditorDialog` 提供 edit modal。
- `TaskSeriesDialog` 保留兼容包装，仅供独立 create/edit 调用；统一创建流程直接使用 `TaskSeriesForm`。

- [x] **Step 1: 写失败测试**

断言循环模式只有一个 dialog；普通/循环切换使用 tab roles；rule/priority 为 combobox；日期按钮来自共享日期选择器；创建/编辑提交 payload 与现有 API 一致。

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx src/features/workspace/project-workbench/task-series/task-series-dialog.test.tsx`

Expected: FAIL，旧实现存在嵌套 dialog 和原生 select/input。

- [x] **Step 3: 最小实现**

使用 `Label/Input/Select/InlineDatePicker/Alert/Button/DialogFooter/Tabs` 重建表单；普通与循环模式各自保留 state；提交 pending 时禁用切换和关闭；创建和编辑继续复用现有 API。

- [x] **Step 4: 定向验证**

Run: 同 Step 2，Expected: PASS。

### Task 4: 收口循环管理栏、列表、详情和停止确认

**Files:**
- Modify: `web/src/features/workspace/project-workbench/project/project-layout.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-panel-shell.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-list.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-detail.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-stop-dialog.tsx`
- Test: `web/src/features/workspace/project-workbench/task-series/task-series-components.test.tsx`
- Test: `web/src/features/workspace/project-workbench/task-series/task-series-panel-shell.test.tsx`

**Interfaces:**
- Panel shell 使用 `TaskCreateDialog(initialMode="recurring")` 和 `TaskSeriesEditorDialog`。
- Stop dialog 保持 `onConfirm(deleteOpen)` 回调契约。

- [x] **Step 1: 写失败测试**

覆盖管理栏宽度、过滤 combobox、状态 Badge、Skeleton/Alert/空状态、格式化日期、编辑/停止按钮、AlertDialog Checkbox 与分页按钮。

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test src/features/workspace/project-workbench/task-series/task-series-components.test.tsx src/features/workspace/project-workbench/task-series/task-series-panel-shell.test.tsx`

Expected: FAIL，旧组件仍是原生控件和硬编码中文。

- [x] **Step 3: 最小实现并验证**

替换为 `Input/Select/Button/Badge/Separator/Skeleton/Alert/AlertDialog/Checkbox`，复用共享 label/date helper，并保持 URL 驱动列表/详情切换。

Run: 同 Step 2，Expected: PASS。

### Task 5: 清理 occurrence 展示和动作中的循环硬编码

**Files:**
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-action-bar.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
- Test: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`

- [x] **Step 1: 写失败测试**

切换英文语言，断言 recurrence badge、occurrence banner、完成本次/重新打开本次/跳过本次与确认文案全部翻译，不显示 canonical 状态或中文残留。

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test src/features/workspace/project-workbench/tasks/task-table.test.tsx src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`

Expected: FAIL，旧 JSX 硬编码中文。

- [x] **Step 3: 最小实现并验证**

统一调用 `taskSeries.*` key 和共享 rule/status formatter，保留现有 shadcn Badge/Button/DestructiveConfirmDialog。

Run: 同 Step 2，Expected: PASS。

### Task 6: 全量、构建与浏览器视觉验收

**Files:**
- Modify: `docs/superpowers/plans/2026-07-13-task-series-console-shadcn-remediation.md`

- [x] **Step 1: 前端质量门**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

- [x] **Step 2: 后端回归门**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
```

- [x] **Step 3: 浏览器视觉验收**

启动本地 server 和 Vite，验证 My Tasks 五个 Tabs、新建普通/循环切换、循环表单、右侧栏列表/详情、停止确认；桌面和窄屏各截图一次。确认无嵌套 dialog、原始 i18n key、裸 Unix 秒和未样式化原生控件。

- [x] **Step 4: 完成审计**

逐项对照 spec §15.21 和本计划，更新 checklist，确认没有遗留硬编码或未验证要求。

### Task 7: 补齐规则修改建议槽位的跨层契约

**Files:**
- Modify: `internal/taskseries/recurrence.go`
- Modify: `internal/app/task_series.go`
- Modify: `internal/httpapi/task_series.go`
- Modify: `internal/mcpserver/tools_task_series.go`
- Modify: `internal/remote/task_series.go`
- Modify: `internal/cli/series.go`
- Modify: `web/src/features/workspace/project-workbench/api/task-series-api.ts`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-form.tsx`
- Test: 对应 App、HTTP、MCP、CLI 和 Web 测试

- [x] **Step 1: App 与规则算法失败测试**

覆盖未来未物化、未来已物化、rule-version 分段、`until` 无后续槽位、终态，以及 anchor 距今超过 10000 个日槽位的快进场景。

- [x] **Step 2: App 最小实现**

为 `TaskSeriesView` 增加 `SuggestedRuleEffectiveFrom`；App 选择严格晚于 now、最大已物化槽位和最后一个 rule-version 起点的下一合法槽位。`buildSeriesView` 改为返回 error，不吞掉计数、用户解析和 occurrence 查询错误。

- [x] **Step 3: 传输层契约测试与实现**

HTTP `suggested_rule_effective_from`、MCP `task_series_get`、CLI JSON、Remote `TaskSeriesDTO` 使用同名可选 Unix 秒字段；停止或无后续合法槽位时省略。

- [x] **Step 4: Web 表单失败测试与实现**

构造 `next_recurrence_at` 与服务端建议值不同的 Series，切换规则后断言日期控件、未来三次预览和 PATCH payload 均使用建议值。不得 fallback 到 `next_recurrence_at`。

- [x] **Step 5: 唯一约束根因修正**

建议值必须晚于最后一个 rule-version 起点；手工提交重复或倒退起点时在写库前返回 `task_series_invalid_effective_from`，不泄漏数据库唯一约束错误。

### 最终评审修正记录

- 统一新建弹窗的普通任务分支也补齐中英文文案，避免切换语言后同一 Dialog 中混用中文。
- 循环任务创建成功后把 Series 返回给调用方；任务页和管理栏都会进入新 Series 详情，而不是只刷新列表。
- “未来三次”从首次截止当天开始；编辑态在规则不变时从 `next_recurrence_at` 开始，修改规则后从用户明确选择的 `effective_from` 开始。
- projected occurrence 仍可编辑截止日期和创建子任务，由服务端在首次合法写操作时物化；只禁止其作为未物化的依赖目标。
- 管理栏 list/detail 更新不再覆盖用户打开前的右栏折叠状态；详情切换加载时不残留上一条 Series。
- 独立编辑弹窗保存期间禁止 Escape/关闭，循环列表行收口为 shadcn `Button`。
- HTTP/MCP 默认查询继续只排除 deleted；Remote CLI 的 `list/next` 显式注入 pending/waiting，恢复与本地 CLI 一致的未完成任务语义。
- SeriesView 新增 `suggested_rule_effective_from`：App 层选择严格晚于 now、最大已物化槽位及最后一个 rule-version 起点的下一合法槽位；HTTP、MCP、CLI JSON、Remote DTO 与 Web 类型统一透传。
- Web 编辑规则时默认使用服务端建议值，并据此刷新未来三次预览和提交 `effective_from`；字段缺失时保持未选择状态，不回退到 `next_recurrence_at`。
- 服务端在写库前拒绝与现有 rule-version 起点重复或倒退的 `effective_from`，返回 `task_series_invalid_effective_from`，避免唯一约束退化为 500。
- 最终浏览器验收使用真实 Series“每日广告汇报”：只读地把规则从“每天”切到“每周”，表单自动带出服务端建议日期 `2026-07-14`，预览同步为 `2026-07-14 · 2026-07-21 · 2026-07-28`；随后取消，未提交修改。
- 浏览器只做只读验收：验证 My Tasks 五个 Tabs、统一创建弹窗、首次截止预览、桌面 320px 管理栏、Series 详情和停止确认；未执行创建或停止写操作。控制台错误仅来自 Chrome 第三方用户脚本扩展，不来自应用页面。
