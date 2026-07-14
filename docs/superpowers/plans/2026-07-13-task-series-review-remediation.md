# 循环任务评审问题完整修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **完成记录（2026-07-13）：** 下列修复已落地。接口残留审计和 native bundle 跨 workspace 重绑定修复后，重新完成普通/零 CGO 全仓测试、零 CGO build、`go vet`、Web 544 tests/typecheck/lint/build/smoke；最终复验记录见文末。

**Goal:** 修复 `b85c64b` 评审确认的调度崩溃、occurrence 越权与非原子写、Report/MCP/HTTP/Bundle 协议分叉、Series 事务及 Web CRUD 缺口，使实现重新满足循环任务 Spec。

**Architecture:** App 层重新成为唯一行为边界：后台 Scheduler 使用 workspace-aware system Service；occurrence 的读、写、子资源写统一经过 scope-aware resolver 和事务包装器；Report、HTTP、MCP、Remote、CLI 共用 TaskViewPage/native bundle；Web 只消费稳定公开 `id`，Series 作为任务页治理面板呈现。

**Tech Stack:** Go 1.25、GORM、SQLite/PostgreSQL、Cobra、HTTP/Huma、MCP Go SDK、React、TypeScript、TanStack Router、Vitest、Playwright。

## Global Constraints

- SQLite 保持 `github.com/glebarez/sqlite` 和 `CGO_ENABLED=0`。
- HTTP、MCP、Remote、CLI 不复制 App 业务规则。
- 所有用户引用继续输出完整 `task.UserInfo`。
- Web 用户可见名称使用“循环任务”；协议类型继续使用 series/occurrence。
- projected occurrence 读取不写库；首次合法写必须与动作同事务。
- 项目 allowlist 对 Task、occurrence、Series、Bundle、Report 行为一致。

---

### Task 1: 修复 Scheduler 与本地 reconcile 生命周期

**Files:**
- Modify: `internal/app/task_series_scheduler.go`
- Modify: `internal/app/task_occurrence_test.go`
- Modify: `internal/cli/server.go`
- Modify: `internal/cli/server_test.go`
- Modify: `internal/cli/root.go`

- [x] 写失败测试：默认 `ServiceFactory` 可运行、缺 Store 返回错误、启动时先 RunOnce。
- [x] 运行定向测试确认 nil factory / nil Store 的原始崩溃。
- [x] 复用后台 system Service factory，补 Clock/Store 校验和首轮 reconcile。
- [x] 本地 CLI 仅在业务命令执行 reconcile；warning 写 stderr，存储错误阻断命令。
- [x] 运行 Scheduler/Reconcile 定向测试并纳入全仓测试。

### Task 2: 收敛 occurrence scope 与原子读写

**Files:**
- Modify: `internal/app/task_occurrence.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/task_series.go`
- Modify: `internal/app/task_occurrence_test.go`
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/task_series_test.go`
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/integration_test.go`

- [x] 写失败测试：allowlist 外 occurrence get/list/skip/write 不可见且不产生 task/audit/project_seq。
- [x] 写失败测试：projected modify/action 失败时物化回滚，成功时公开 id 不变。
- [x] 将 `MaterializeOccurrenceForWrite` 变为 scope-aware；协议 resolver 不再产生写副作用。
- [x] 普通 Task action、modify、annotation/link 子资源入口收到 occurrence_ref 时在 App 事务内物化和执行。
- [x] HTTP/MCP 保留原始 ref 调用 App，并以 `GetTaskView` 返回统一 view；数字 ref 只做无副作用格式校验。
- [x] 运行 App、HTTP、MCP 定向及全量测试。

### Task 3: 修复 Report、Task 查询与完整 DTO

**Files:**
- Modify: `internal/app/task_occurrence.go`
- Modify: `internal/app/task_occurrence_test.go`
- Modify: `internal/httpapi/task_series.go`
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/task_series_test.go`
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/views.go`

- [x] 写失败测试覆盖 ready/blocked/blocking/urgency、auto 完整范围、expand 缺边界、task_type。
- [x] 从完整 materialized dependency graph 计算 blocked/blocking；projected occurrence 固定非阻塞。
- [x] urgency 在过滤后、分页前计算并排序。
- [x] MCP `task_query` 改用 `QueryTaskViews`，消费 due range/occurrence_mode/task_type，并与 HTTP 使用相同包含式日期 AST。
- [x] HTTP auto 在完整范围 expand，显式 expand 缺边界报错，并映射 task_type。
- [x] HTTP/MCP/Remote 输出完整 TaskOccurrenceView 字段；evaluator 补齐 `OpNotEqual`。

### Task 4: 修复 Series modify/stop/detail 一致性

**Files:**
- Modify: `internal/app/task_series.go`
- Modify: `internal/storage/task_occurrence_repo.go`
- Modify: `internal/app/task_series_test.go`
- Modify: `internal/httpapi/task_series.go`

- [x] 写失败测试覆盖 modify 回滚、UDA 继承/同步、超过一年 occurrence、详情分组；既有 stop/UI 测试锁定 1000 上限。
- [x] 在事务前完成输入解析和 invariant 校验；事务内更新 Series、RuleVersion、open occurrence 和 audit。
- [x] Series occurrence 同步查询移除固定年度窗口；`GetOccurrence` 统一 preload 所有关联字段。
- [x] Stop 在同一事务内统计影响数，超过 1000 返回错误并整体回滚。
- [x] GetTaskSeries 填充 open/recent-completed/recent-skipped，Web/HTTP/MCP/Remote 均消费同一分组。

### Task 5: 统一 native bundle 与租户边界

**Files:**
- Modify: `internal/app/task_bundle.go`
- Modify: `internal/app/task_bundle_test.go`
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/remote/config.go`
- Modify: `internal/cli/import_export.go`
- Modify: `internal/mcpserver/tools_task.go`

- [x] 写失败测试：导出遵守 project scope；occurrence/series project 不一致被拒绝；任一失败整体回滚。
- [x] 导入时重绑定当前 workspace，所有 Project 必须属于当前 workspace 和 allowlist。
- [x] Bundle 导入使用单一事务，并完整 round-trip 负责人、注解、依赖、UDA、链接和循环字段。
- [x] HTTP、Remote、CLI、MCP 全部使用 `xuanchu.task-bundle/v1` object。
- [x] 删除旧 task-array Remote transport 和 recur 字段；Web 普通任务导入改为独立版本化 `xuanchu.task-import/v1`。

### Task 6: 完成 Web 融合入口、Series CRUD 与 occurrence 操作

**Files:**
- Modify: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/*`
- Modify: `web/src/routes/workspace/ProjectTaskSeriesPanelRoute.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-action-bar.tsx`
- Modify: `web/src/pages/my-tasks-page.tsx`
- Modify: `web/src/features/workspace/my-tasks/my-tasks-table.tsx`

- [x] 测试覆盖“循环任务 N”入口、close/back、详情分组、编辑清空字段、分页/排序/负责人筛选和 route search 恢复。
- [x] 工具栏增加治理入口和普通/循环创建入口；Panel 增加新建、关闭、分页、排序和负责人筛选。
- [x] 编辑提交规则/effective_from/until/共享字段，空值通过 `clear` 表达。
- [x] occurrence ActionBar 使用“完成本次/重新打开本次/跳过本次”。
- [x] projected 卡片和移动端列表全部使用稳定公开 id。
- [x] 修复移动按钮横向溢出与 Markdown Dialog 关闭脚本，并通过 `smoke:editing`。

### Task 7: 文档同步与全量验证

**Files:**
- Modify: `docs/superpowers/plans/2026-07-12-task-series-calendar-recurrence-implementation.md`
- Modify: `ROADMAP.md`
- Modify: `README.md`

- [x] 删除不真实的旧验证声明，按实际结果更新进度、README 和 ROADMAP。
- [x] 运行 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`、`go vet ./...`。
- [x] 运行 Web typecheck/test/lint/build/smoke。
- [x] 运行 `git diff --check` 并审查工作区差异。

### Task 8: 最终接口残留审计与协议闭环

**Files:**
- Modify: `internal/app/task_series.go`
- Modify: `internal/app/task_bundle.go`
- Modify: `internal/httpapi/task_series.go`
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/mcpserver/tools_task_series.go`
- Modify: `internal/remote/task_series.go`
- Modify: `internal/cli/series.go`
- Modify: `web/src/features/workspace/project-workbench/import/*`
- Modify: `web/src/features/workspace/project-readonly/*`
- Modify: `docs/manual/tasks.md`
- Modify: `docs/manual/mcp.md`

- [x] 删除 Web 普通导入、只读 DTO、状态筛选和详情文案中残留的 `recur/mask/imask/status=recurring` 可执行语义；旧字段上传显式失败。
- [x] 普通任务导入拒绝 occurrence identity，避免绕过 Series/occurrence invariant。
- [x] HTTP/MCP/Remote/CLI 的 Series add/modify 补齐 UDA、`clear`、date-only 和 occurrence range；未知 clear 字段统一报错。
- [x] 修复未来首次截止 Series 在 modify 前 reconcile 的非法展开范围。
- [x] MCP Series/occurrence 输出统一 UserInfo、UDA、links 和详情实例分组。
- [x] Remote CLI DTO 转换保留完整 Series/occurrence 字段，`series info` 展示实例分组，CLI `--json` 使用协议 snake_case 而不是 Go 字段名。
- [x] native bundle 增加 Series project slug，并在跨 workspace 导入时按 slug 重绑定 project ID；同步迁移旧数组集成测试。

## 最终验证记录

- `go test ./...`：通过。
- `CGO_ENABLED=0 go test ./...`：通过。
- `CGO_ENABLED=0 go build -o /tmp/xuanchu-task-series-final ./cmd/xuanchu`：通过。
- `go vet ./...`：通过。
- `pnpm --dir web typecheck`：通过。
- `pnpm --dir web test`：113 个 test files、544 个 tests 通过。
- `pnpm --dir web lint`：通过。
- `pnpm --dir web exec vite build --outDir /tmp/xuanchu-web-final-review --emptyOutDir`：通过。
- `pnpm --dir web run smoke:editing`：通过。
