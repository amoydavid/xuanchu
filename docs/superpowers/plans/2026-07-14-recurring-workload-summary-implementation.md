# 循环实例成员待办与项目摘要 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 已物化且未完成的循环实例应计入负责人当前待办；项目右栏明确分开显示普通任务进度和循环任务运行情况。

**Architecture:** storage 的负责人负载查询改为统计所有真实 `tasks` 行，故会纳入 materialized occurrence，同时仍排除已完成和已删除状态；普通风险和进度统计保留 `series_id IS NULL`。HTTP 已返回 `series_metrics`，前端补齐其类型并在共享右栏渲染，不另行派生统计。

**Tech Stack:** Go 1.25、GORM、React、TypeScript、Vitest、pnpm。

## Global Constraints

- SQLite 继续使用 `github.com/glebarez/sqlite`，不得引入 CGO 依赖。
- 所有统计均在 storage/app 层聚合；前端不从当前列表派生。
- projected future occurrence 不进入成员待办；materialized pending/waiting occurrence 进入。
- 用户可见文档与注释使用中文。

---

### Task 1: 固化成员待办的循环实例口径

**Files:**
- Modify: `internal/storage/project_repo_test.go`
- Modify: `internal/app/task_occurrence_test.go`
- Modify: `internal/app/task_series.go:289-329`
- Modify: `internal/storage/project_repo.go:278-622`

**Interfaces:**
- Consumes: `ProjectRepository.TaskSummary(workspaceID, projectID, now)`。
- Produces: `ProjectTaskSummary.Workload`，按负责人返回普通未关闭任务和 materialized occurrence 的合并计数；立即物化的首个实例也持久化负责人关联。

- [x] **Step 1: 写失败测试**

在 repository fixture 中插入一个分配给张三、`series_id != NULL`、`pending` 的 occurrence，并断言张三 `OpenCount` 增加；在 app occurrence 测试中断言该实例仍不改变普通风险计数，但会进入负责人负载。

- [x] **Step 2: 运行测试确认失败**

Run: `CGO_ENABLED=0 go test ./internal/storage ./internal/app -run 'TestProjectRepositoryTaskSummary|TestProjectTaskSummaryExcludesOccurrencesAndReportsSeriesMetrics' -count=1`

Expected: FAIL，因为现有 `taskSummaryWorkload` 使用 `tasks.series_id IS NULL` 排除了循环实例。

- [x] **Step 3: 写最小实现**

从 `taskSummaryWorkload` 的负责人查询移除 `tasks.series_id IS NULL`，未分配行仍仅统计普通任务；在 `materializeFirstOccurrence` 创建前复制 series assignee，确保落入 `task_assignees`；项目状态计数和普通风险仍保留 `series_id IS NULL`，不改 `SeriesMetrics` 查询。

- [x] **Step 4: 运行定向测试确认通过**

Run: `CGO_ENABLED=0 go test ./internal/storage ./internal/app -run 'TestProjectRepositoryTaskSummary|TestProjectTaskSummaryExcludesOccurrencesAndReportsSeriesMetrics' -count=1`

Expected: PASS。

### Task 2: 在右栏呈现循环运行情况

**Files:**
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts:151-161`
- Modify: `web/src/features/workspace/project-workbench/project/project-context-rail.tsx:67-140`
- Modify: `web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: API `series_metrics`，字段为 `recurring_series_count`、`active_recurring_series_count`、`open_recurring_occurrence_count`、`overdue_recurring_occurrence_count`。
- Produces: 右栏的“循环任务运行情况”，并保持「成员待办」使用合并后的 `workload`。

- [x] **Step 1: 写失败测试**

为右栏提供 `series_metrics`，断言出现循环运行情况标题、运行中系列数、未完成实例数和逾期实例数；现有成员待办断言保留。

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx`

Expected: FAIL，因为 `ProjectTaskSummary` 尚无 `series_metrics` 类型，右栏尚无该区块。

- [x] **Step 3: 写最小实现**

新增 `ProjectSeriesMetrics` 类型和 `ProjectTaskSummary.series_metrics`；右栏在存在活跃系列或未完成实例时渲染单独区块，使用 i18n 文案，不改普通任务进度百分比。

- [x] **Step 4: 运行定向前端测试确认通过**

Run: `pnpm --dir web test web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx`

Expected: PASS。

### Task 3: 文档与回归验证

**Files:**
- Modify: `docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 1、Task 2 的最终统计语义。
- Produces: 与 API、右栏行为一致的循环摘要说明。

- [x] **Step 1: 更新面向用户的说明**

在 README 的循环能力条目中明确普通进度与成员待办/循环运行情况的口径。

- [x] **Step 2: 运行完整验证**

Run: `go test ./... && CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build ./cmd/xuanchu && pnpm --dir web typecheck && pnpm --dir web test && pnpm --dir web lint && pnpm --dir web build && pnpm --dir web run smoke:editing && git diff --check`

实际结果：`go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`、`pnpm --dir web typecheck`、`pnpm --dir web lint`、`pnpm --dir web run smoke:editing` 与 `git diff --check` 通过。完整 Web Vitest 与 `pnpm --dir web build` 分别被既有任务详情/任务表测试失败和 `task-detail-page.tsx:235` 路由类型错误阻断；二者不在本计划修改文件内。

### Task 4: 将系列负责人限定为未来默认值

**Files:**
- Modify: `internal/app/task_series.go:653-717`
- Modify: `internal/app/task_occurrence_test.go`
- Modify: `docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md`

**Interfaces:**
- Consumes: `ModifyTaskSeriesInput.Assignees` 与 `ClearAssignees`。
- Produces: 更新后的 `task_series_assignees` 只用于 projected 和后续物化实例；任何已物化 occurrence 的 `task_assignees` 保持原值。

- [x] **Step 1: 写失败测试并确认失败**

创建带 local 负责人的已物化实例，修改 series 为另一位 workspace member；断言 series 返回新负责人，重新读取实例后仍为 local。运行：`CGO_ENABLED=0 go test ./internal/app -run TestModifyTaskSeriesAssigneesDoNotRewriteMaterializedOccurrences -count=1`。

- [x] **Step 2: 实现最小修复并确认通过**

从 `syncSharedFieldsToOpenOccurrences` 删除 assignees 同步分支，只保留 series 自身更新；运行同一测试及 `CGO_ENABLED=0 go test ./internal/app -run TestModifyTaskSeries -count=1`。
