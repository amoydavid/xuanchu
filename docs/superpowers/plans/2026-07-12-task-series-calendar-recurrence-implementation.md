# 璇础循环任务系列、范围投影与按需物化 Implementation Plan

> **实施进度（2026-07-13，feat/task-series-recurrence 分支）：全部 14 个 Task + spec 收敛补充完成。**
>
> - **Task 1-7 ✅**：后端核心闭环
> - **Task 8 ✅**：HTTP Series CRUD + Task 范围查询 + Remote client
> - **Task 9 ✅**：专用 CLI `xuanchu series ...` 命令树
> - **Task 10 ✅**：七个 task_series_* MCP tools + golden files
> - **Task 11 ✅**：Web 原生类型 + task-series-api + 任务页面板路由
> - **Task 12 ✅**：面板 list/detail/stop-dialog + recurrence-preview + 创建弹窗 initialMode
> - **Task 13 ✅**：occurrence badge/banner + My Tasks 5 预设 + 类型扩展
> - **Task 14 ✅**：xuanchu.task-bundle/v1 原生 bundle round-trip + 删除 internal/recurrence + README/ROADMAP 更新 + 全量验证（go test/CGO build/vet/web typecheck/lint/build 通过）
> - **Spec 收敛补充 ✅**（2026-07-12 追加）：
>   - §17.4 项目统计分离：ProjectTaskSummary 普通计数排除 occurrence + 新增 series_metrics（recurring/active series count、open/overdue occurrence count）
>   - §17.3 HTTP `GET /tasks` 统一返回 TaskViewPage（不再返回裸任务数组），`/reports/{name}` 共用 handleTaskListReport
>   - §13.5 Remote 删除旧 `ListTasks`/`GetTask` 签名，CLI remote 14 处全量迁移到 `QueryTasks`/`GetTaskView`
>   - 前端 3 处列表调用点（OverviewPage/my-tasks/getProjectTasks）适配 TaskViewPage 分页结构

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用独立 `task_series` 聚合、有界 occurrence 投影、执行期/首次写物化和 exception/tombstone，替换现有 hidden recurring Task，并让 CLI、HTTP、Remote、MCP 与 Web Console 使用同一行为契约。

**Architecture:** `internal/task` 只保留普通任务与已物化 occurrence；新的 `internal/taskseries` 负责 series、rule version 和槽位计算。`internal/app` 提供唯一的 Series CRUD、TaskOccurrenceView、范围 merge、稳定 occurrence resolver 与原子写前物化；所有传输层只做输入解析和 envelope。Scheduler 负责执行期补齐，Web/HTTP/MCP 的有限日期读取即使 scheduler 延迟也能计算 projected occurrence。

**Tech Stack:** Go 1.25 / GORM / SQLite `github.com/glebarez/sqlite` / PostgreSQL `gorm.io/driver/postgres` / Cobra / chi + Huma / MCP Go SDK / React + TypeScript + TanStack Router + React Query + shadcn/ui / Vitest + Testing Library / pnpm

**Spec:** `docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md`

## Global Constraints

- 文档、代码注释、用户可见文案和提交信息以中文为主。
- SQLite 继续使用 `github.com/glebarez/sqlite`，禁止引入 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`。
- PostgreSQL 继续使用 `gorm.io/driver/postgres`；所有 schema、索引和 repository 行为必须同时覆盖 SQLite/PostgreSQL。
- `Task.status` 移除 `recurring`；`Task.parent` 只表示手工父子任务；occurrence 只通过 `series_id` 关联 series。
- 不兼容 Taskwarrior JSON、hidden recurring parent、`mask/imask` 或 `add ... recur:*` 循环命令；只保留帮助性 `task_series_endpoint_required` 错误。
- 普通任务和循环系列不提供互转入口。
- date-only `due` / `until` 按本地 `23:59:59`；`wait` / `scheduled` 按本地 `00:00:00`；range 一律左闭右开。
- projected occurrence 的读取禁止写库、分配 UUID/project_seq、写 audit 或发 Hook。
- occurrence 的公开 `id` 在投影/物化前后固定为 `occ:<series_uuid>:<recurrence_at_unix>`。
- 所有用户引用使用 `task.UserInfo` / `task.JSONUserInfo`，不输出裸用户 UUID。
- MCP tool name 使用下划线，`Content[0].text` 的 ToolEnvelope 与 `structuredContent` 语义一致。
- Web 全局侧栏和 ProjectTabs 都不新增循环任务；普通任务与 occurrence 融合在“任务”，series 只进入任务页内可深链的管理面板。
- Web 用户可见的 Series 一级名称统一为“循环任务”；“循环规则”只作为每天/每周等频率字段名。路由、API、MCP、Go/TS 类型和 query key 继续使用 series/recurrence，不做协议重命名。
- 不提交 `internal/webconsole/dist`、本地二进制、数据库、token、缓存或临时文件。
- 每个后端任务至少运行其定向测试；最终必须运行 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`、`go vet ./...`。
- Web 任务最终必须运行 `pnpm --dir web typecheck`、`pnpm --dir web test`、`pnpm --dir web lint`、`pnpm --dir web build`、`pnpm --dir web run smoke:editing`。

---

## 范围与依赖顺序

本 Spec 横跨多个层，但不是可独立发布的多个产品：协议和 Web 都依赖同一 App 投影/物化语义，拆成多份计划会产生不可运行的中间契约。因此保留一个实施计划，并以任务 1–14 的依赖顺序推进。任务 1–7 完成后后端领域闭环成立；任务 8–10 接入协议；任务 11–13 完成 Web；任务 14 做原生 bundle、文档和全量验收。

## Spec 覆盖映射

| Spec 范围 | 实施任务 |
|---|---|
| §1–5 背景、兼容边界、方案选型 | Global Constraints；Task 2–3、9、14 删除旧契约 |
| §6–7 术语、Series/Task 数据模型、rule versions、UserInfo | Task 1–5 |
| §8–10 日历语义、scheduler、生命周期 | Task 4、6、7 |
| §11–12 CRUD、canonical recurrence | Task 1、5、6 |
| §13 HTTP/Remote/CLI | Task 8–9 |
| §14 MCP | Task 10 |
| §15–16 Web IA、ASCII 原型、数据流 | Task 11–13 |
| §17 查询、列表、项目统计 | Task 4、7、8、13 |
| §18 审计、事件与自动化 | Task 5–7 |
| §19 项目关闭 | Task 7 |
| §20 原生 bundle 与 schema 切换 | Task 2–3、14 |
| §21–22 错误码、并发、事务 | Task 3–8 |
| §23–25 测试、验收、交付拆分 | 每个任务的 TDD 步骤；Task 14 全量验收 |

## 文件结构

**新建领域与 App 文件：**

- `internal/taskseries/model.go` — Series、RuleVersion、状态、输入 invariant。
- `internal/taskseries/recurrence.go` — canonical parser、Next、ExpandRange、rule-version 分段。
- `internal/taskseries/recurrence_test.go` — 日期、月末、时区、分段和范围测试。
- `internal/app/task_occurrence.go` — occurrence_ref、TaskOccurrenceView、range merge、读 resolver。
- `internal/app/task_series.go` — Series CRUD、view、共享字段同步、stop/skip。
- `internal/app/task_series_scheduler.go` — 执行期 reconcile 与后台循环。
- `internal/app/task_series_test.go` — Series 用例与事务测试。
- `internal/app/task_occurrence_test.go` — projection/materialization/merge/write resolver 测试。
- `internal/app/task_series_scheduler_test.go` — daily、停机补齐、并发和上限测试。
- `internal/query/evaluator.go`、`internal/query/evaluator_test.go` — 对 merge 后 Task view 执行与 SQL 等价的查询 AST。

**新建存储文件：**

- `internal/storage/task_series_repo.go` — series/rule versions/关联字段 CRUD 与扫描。
- `internal/storage/task_series_repo_test.go` — SQLite repository 契约。
- `internal/storage/task_occurrence_repo.go` — 槽位唯一写入、range exception 与 counts。
- `internal/storage/task_occurrence_repo_test.go` — occurrence 幂等、range、overrides 测试。
- `internal/storage/migration_task_series.go` — 旧 recurring 检测、普通 Task schema 切换、partial index。

**新建协议文件：**

- `internal/httpapi/task_series.go`、`internal/httpapi/task_series_test.go` — Series HTTP CRUD。
- `internal/remote/task_series.go`、`internal/remote/task_series_test.go` — Remote client。
- `internal/cli/series.go`、`internal/cli/series_test.go` — 专用 CLI 命令。
- `internal/mcpserver/tools_task_series.go`、`internal/mcpserver/tools_task_series_test.go` — 七个 series tools。
- `internal/mcpserver/testdata/task_series_*.schema.json` — MCP schema golden files。

**新建 Web 文件：**

- `web/src/features/workspace/project-workbench/api/task-series-api.ts` — TS 契约和 API client。
- `web/src/features/workspace/project-workbench/api/task-series-api.test.ts` — URL/body 契约。
- `web/src/features/workspace/project-workbench/task-series/task-series-panel.tsx` — 任务页内 URL 驱动的 Series 管理面板壳层。
- `web/src/features/workspace/project-workbench/task-series/task-series-panel-shell.tsx` — 桌面右栏/移动全屏 Sheet、焦点和关闭行为。
- `web/src/features/workspace/project-workbench/task-series/task-series-list.tsx` — 面板内 Series 列表。
- `web/src/features/workspace/project-workbench/task-series/task-series-detail.tsx` — 面板内 Series 详情与历史。
- `web/src/features/workspace/project-workbench/task-series/task-series-dialog.tsx` — 编辑规则和 effective_from。
- `web/src/features/workspace/project-workbench/task-series/task-series-stop-dialog.tsx` — 停止确认。
- `web/src/features/workspace/project-workbench/task-series/*.test.tsx` — 页面/弹窗测试。
- `web/src/routes/workspace/ProjectTasksRoute.tsx` — 持久任务父路由，任务页和 Series 子路由 Outlet 共存。
- `web/src/routes/workspace/ProjectTaskSeriesPanelRoute.tsx` — list/detail 子路由只注册面板，不重挂载任务页。

**重点修改文件：**

- `internal/task/model.go`、`internal/task/json.go`、`internal/task/modification.go` — 移除旧 recurrence 字段，增加 occurrence 持久字段。
- `internal/storage/models.go`、`migrate_sqlite.go`、`migrate_postgres.go`、`task_repo.go`、`project_repo.go` — schema、repo、统计。
- `internal/app/service.go`、`project.go`、`hook_event.go`、`task_audit_payload.go` — repository 注入、旧逻辑删除、项目关闭和事件。
- `internal/httpapi/tasks.go`、`huma_routes.go`、`error_status.go` — TaskOccurrenceView、range query、Series routes。
- `internal/query/ast.go`、`parser*.go`、`internal/storage/query_scope.go` — 删除旧 recurrence 属性，增加 series_id/recurrence_at/task_type。
- `internal/remote/task.go`、`config.go` — TaskOccurrenceDTO/TaskViewPageDTO、Series 与 native bundle client。
- `internal/cli/add.go`、`list.go`、`report.go`、`helper.go`、`edit.go`、`root.go`、`server.go`、`internal/edit/edit.go`、`internal/render/table.go` — occurrence_ref、working set、原生 view renderer 和 scheduler wiring。
- `internal/mcpserver/tools_task.go`、`tools_views.go`、`schema_test.go` — occurrence view 与 schema。
- `web/src/features/workspace/project-workbench/api/task-api.ts`、`project-api.ts`、任务列表/详情/创建、My Tasks、project tabs/layout、router、locales — Web 完整闭环。

---

### Task 1: 建立 TaskSeries 领域与规则版本展开

**目标：** 建立不依赖 GORM/CLI 的 Series 领域，定义 canonical rule、状态、rule-version 段和有界槽位展开。

**Files:**

- Create: `internal/taskseries/model.go`
- Create: `internal/taskseries/recurrence.go`
- Create: `internal/taskseries/recurrence_test.go`
- Modify: `internal/recurrence/recurrence.go`（完成调用方迁移后删除该包；本任务先保留转发，避免中间提交无法编译）

**Interfaces:**

- Produces:
  - `type Series struct`
  - `type RuleVersion struct`
  - `type Slot struct { RecurrenceAt int64; Rule string }`
  - `func ValidateSeries(Series) error`
  - `func ValidateRule(string) error`
  - `func Next(int64, string, *time.Location) (int64, error)`
  - `func ExpandRange([]RuleVersion, until *int64, start, end int64, loc *time.Location) ([]Slot, error)`
- Consumes: only Go stdlib.

- [ ] **Step 1: 写失败测试覆盖 canonical rule 与 rule-version 切换**

Create `internal/taskseries/recurrence_test.go`:

```go
package taskseries

import (
	"testing"
	"time"
)

func TestExpandRangeUsesRuleVersionAnchors(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil { t.Fatal(err) }
	day := func(value string) int64 {
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", value+" 23:59:59", loc)
		if err != nil { t.Fatal(err) }
		return ts.Unix()
	}
	versions := []RuleVersion{
		{EffectiveFrom: day("2026-07-01"), RecurrenceRule: "weekly"},
		{EffectiveFrom: day("2026-07-15"), RecurrenceRule: "daily"},
	}
	slots, err := ExpandRange(versions, nil, day("2026-07-01"), day("2026-07-18")+1, loc)
	if err != nil { t.Fatal(err) }
	want := []int64{day("2026-07-01"), day("2026-07-08"), day("2026-07-15"), day("2026-07-16"), day("2026-07-17"), day("2026-07-18")}
	if len(slots) != len(want) { t.Fatalf("len=%d want=%d: %#v", len(slots), len(want), slots) }
	for i := range want {
		if slots[i].RecurrenceAt != want[i] { t.Fatalf("slot[%d]=%d want=%d", i, slots[i].RecurrenceAt, want[i]) }
	}
}

func TestValidateRuleRejectsAliases(t *testing.T) {
	for _, value := range []string{"biweekly", "quarterly", "annual", "yearly", "0days"} {
		if err := ValidateRule(value); err == nil { t.Fatalf("ValidateRule(%q) succeeded", value) }
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/taskseries -run 'TestExpandRangeUsesRuleVersionAnchors|TestValidateRuleRejectsAliases' -count=1`

Expected: FAIL，提示 `internal/taskseries` 或 `RuleVersion`/`ExpandRange` 尚不存在。

- [ ] **Step 3: 实现领域类型与展开算法**

Create `internal/taskseries/model.go` with the exact persistent-domain fields:

```go
package taskseries

type Series struct {
	ID, WorkspaceID, ProjectID, Title string
	Description *string
	Status string
	RecurrenceRule string
	FirstDue int64
	Until, EffectiveEndAt *int64
	StopReason *string
	Priority *string
	AssigneeIDs, Tags []string
	UDAs map[string]string
	CreatedBy string
	CreatedAt, ModifiedAt int64
	RuleVersions []RuleVersion
}

type RuleVersion struct {
	ID, SeriesID string
	EffectiveFrom int64
	RecurrenceRule, CreatedBy string
	CreatedAt int64
}

const (
	StatusActive = "active"
	StatusEnded = "ended"
	StatusStopped = "stopped"
)
```

Create `internal/taskseries/recurrence.go`; implement `daily|weekly|monthly|<N>days|<N>weeks|<N>months`, sort and validate versions, expand each `[effective_from[i], effective_from[i+1])` segment, clip to `[start,end)` and inclusive `until`, and return sorted unique `Slot` values. Keep `time.AddDate` month-end behavior. `ExpandRange` rejects `end <= start`, nil location and duplicate/non-increasing `effective_from`; the 366-day product limit belongs to App `TaskViewQuery` validation.

- [ ] **Step 4: 补齐日期与 invariant 测试**

Append tests for daily/weekly/monthly/N-unit, inclusive until, left-closed/right-open range, Asia/Shanghai date boundary, duplicate rule versions, empty title/project, invalid series status, and `ended/stopped` missing `effective_end_at`.

- [ ] **Step 5: 运行领域测试**

Run: `go test ./internal/taskseries -count=1`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/taskseries internal/recurrence
git commit -m "feat: 建立循环系列领域与规则展开"
```

### Task 2: 在 Task 领域引入 occurrence invariant

**目标：** 先增加已物化 occurrence 的持久字段与 invariant，让后续 schema 切换有可编译的目标类型；旧 hidden recurring 字段在 Task 3 与存储引用一起原子删除。

**Files:**

- Modify: `internal/task/model.go`
- Modify: `internal/task/model_test.go`
- Modify: `internal/task/json.go`
- Modify: `internal/task/json_test.go`
- Modify: `internal/task/modification.go`
- Modify: `internal/task/modification_test.go`

**Interfaces:**

- Produces on `task.Task`:
  - `SeriesID *string`
  - `RecurrenceAt *int64`
  - `RecurrenceRuleSnapshot *string`
  - `RecurrenceOverrides []string`
- Defers to Task 3: `StatusRecurring`, `Recur`, `Mask`, `IMask` removal. `Until` remains an ordinary Task field but is no longer a series snapshot.

- [ ] **Step 1: 写失败测试定义新的 Task invariant**

Add to `internal/task/model_test.go`:

```go
func TestTaskValidateOccurrenceInvariant(t *testing.T) {
	seriesID := "series-1"
	slot := int64(100)
	rule := "daily"
	base := Task{UUID: "task-1", WorkspaceID: "ws-1", Title: "巡检", Status: StatusPending}
	for name, mutate := range map[string]func(*Task){
		"series without slot": func(v *Task) { v.SeriesID = &seriesID },
		"slot without series": func(v *Task) { v.RecurrenceAt = &slot },
		"series without snapshot": func(v *Task) { v.SeriesID = &seriesID; v.RecurrenceAt = &slot },
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			if err := value.Validate(); err == nil { t.Fatal("Validate succeeded") }
		})
	}
	base.SeriesID, base.RecurrenceAt, base.RecurrenceRuleSnapshot = &seriesID, &slot, &rule
	if err := base.Validate(); err != nil { t.Fatalf("Validate: %v", err) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task -run TestTaskValidateOccurrenceInvariant -count=1`

Expected: FAIL，提示新字段不存在。

- [ ] **Step 3: 增加 Task occurrence 字段和 JSON**

Add the four occurrence fields. `Validate` must enforce all three pointer fields are either all nil or all non-nil; `RecurrenceOverrides` must be sorted/unique and restricted to `title description priority due assignees tags udas wait scheduled depends`。普通 `Parent` 保留；Task JSON adds `series_id`、`recurrence_at`、`recurrence_rule_snapshot`、`recurrence_overrides`。为保证本提交全仓可编译，旧字段和 `StatusRecurring` 暂时保留且不得被新 occurrence 路径使用；Task 3 在迁移 storage model/repository 的同一提交中删除它们。

- [ ] **Step 4: 修改 modification diff 与审计字段集合**

Add occurrence fields to JSON/model round-trip but do not expose setters in ordinary `Modification`. Ensure occurrence rule/snapshot/series ID cannot be changed through ordinary `ModifyInput`; `due` remains modifiable and is the only date field that can diverge from `recurrence_at`.

- [ ] **Step 5: 更新单元测试并运行**

Run: `go test ./internal/task -count=1`

Expected: PASS；JSON round-trip includes occurrence fields；旧字段删除断言由 Task 3 完成。

- [ ] **Step 6: 提交**

```bash
git add internal/task
git commit -m "refactor: 为任务引入循环实例关联"
```

### Task 3: 新增 Series/Occurrence schema、迁移与 repositories

**目标：** 建立 SQLite/PostgreSQL 等价的独立表、关联表、规则版本和 occurrence 唯一槽位，并明确拒绝旧 recurring 数据。

**Files:**

- Modify: `internal/task/model.go`
- Modify: `internal/task/json.go`
- Modify: `internal/task/modification.go`
- Modify: `internal/task/model_test.go`
- Modify: `internal/task/json_test.go`
- Modify: `internal/task/modification_test.go`
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`
- Create: `internal/storage/migration_task_series.go`
- Create: `internal/storage/task_series_repo.go`
- Create: `internal/storage/task_series_repo_test.go`
- Create: `internal/storage/task_occurrence_repo.go`
- Create: `internal/storage/task_occurrence_repo_test.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/storage/task_repo_test.go`
- Modify: `internal/storage/db_test.go`
- Modify: `internal/storage/postgres_test.go`

**Interfaces:**

- Produces:
  - `NewTaskSeriesRepository(*gorm.DB) *TaskSeriesRepository`
  - `type TaskSeriesListOptions struct { WorkspaceID, ProjectID, Status, Q, AssigneeUserID string }`
  - `Create(series taskseries.Series) (taskseries.Series, error)`
  - `Get(workspaceID, seriesID string) (taskseries.Series, error)`
  - `ListCandidates(TaskSeriesListOptions) ([]taskseries.Series, error)`; filters but never sorts/pages
  - `Update(series taskseries.Series) error`
  - `ListActive(limit, offset int) ([]taskseries.Series, error)`
  - `StopProjectSeries(workspaceID, projectID string, at int64, reason string) ([]taskseries.Series, error)`
  - `NewTaskOccurrenceRepository(*gorm.DB) *TaskOccurrenceRepository`
  - `CreateOccurrence(task.Task) (task.Task, bool, error)`
  - `GetOccurrence(workspaceID, seriesID string, recurrenceAt int64) (task.Task, error)`
  - `ListOccurrenceExceptions(OccurrenceRangeOptions) ([]task.Task, error)`
  - `CountSeriesOccurrences(workspaceID, seriesID string, now int64) (OccurrenceCounts, error)`

- [ ] **Step 1: 写 repository 失败测试**

Create `internal/storage/task_series_repo_test.go` with a lifecycle test that creates workspace/project/user, creates an active daily series with one rule version and assignee/tag/UDA, reloads it, appends a rule version, lists by project/status, and stops it. Assert all associations and timestamps round-trip.

Add table tests for `TaskSeriesListOptions{Status,Q,AssigneeUserID}`: q matches title/description case-insensitively; assignee filters through the association table; all matching candidates and rule-version associations are returned in stable ID order without limit/offset on SQLite and PostgreSQL. App owns public sorting/pagination.

Create `internal/storage/task_occurrence_repo_test.go` with:

```go
func TestCreateOccurrenceIsUniqueBySeriesSlotForever(t *testing.T) {
	store, ws, project, series := newTaskSeriesRepoFixture(t)
	repo := NewTaskOccurrenceRepository(store.DB())
	slot, rule := int64(100), "daily"
	seriesID := series.ID
	row := domain.Task{UUID: "occ-1", WorkspaceID: ws.ID, Title: "巡检", Status: domain.StatusPending, ProjectID: &project.ID, SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule}
	first, existed, err := repo.CreateOccurrence(row)
	if err != nil || existed { t.Fatalf("first=%#v existed=%v err=%v", first, existed, err) }
	row.UUID = "occ-2"
	second, existed, err := repo.CreateOccurrence(row)
	if err != nil || !existed || second.UUID != first.UUID { t.Fatalf("second=%#v existed=%v err=%v", second, existed, err) }
	first.Status = domain.StatusCompleted
	if err := NewTaskRepository(store.DB()).Update(first); err != nil { t.Fatal(err) }
	row.UUID = "occ-3"
	third, existed, err := repo.CreateOccurrence(row)
	if err != nil || !existed || third.UUID != first.UUID { t.Fatalf("third=%#v existed=%v err=%v", third, existed, err) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/storage -run 'TestTaskSeries|TestCreateOccurrence' -count=1`

Expected: FAIL，提示 repositories/models 不存在。

- [ ] **Step 3: 新增 GORM models 与索引**

Add `TaskSeries`、`TaskSeriesRuleVersion`、`TaskSeriesAssignee`、`TaskSeriesTag`、`TaskSeriesUDAValue`, explicitly mapping to `task_series`、`task_series_rule_versions`、`task_series_assignees`、`task_series_tags`、`task_series_uda_values`; add occurrence columns to storage `Task`. Create partial unique index with dialect-specific SQL:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_ws_series_slot
ON tasks(workspace_id, series_id, recurrence_at)
WHERE series_id IS NOT NULL AND recurrence_at IS NOT NULL;
```

PostgreSQL and SQLite must both install the same logical constraint. Foreign keys from occurrence and association rows use RESTRICT for series history, while association rows may cascade when a development database row is explicitly removed.

Also create `UNIQUE(series_id,effective_from)` on `task_series_rule_versions`, `(workspace_id,project_id,status)` on `task_series`, and lookup indexes for series assignees, recurrence_at and due. Repository tests must assert duplicate rule cutover fails and cross-workspace/project associations are rejected.

- [ ] **Step 4: 实现破坏性 recurring schema 检测**

Before dropping/rebuilding old columns, use `Migrator().HasTable/HasColumn` so a fresh database never queries nonexistent legacy columns. If old columns exist, query `tasks WHERE status='recurring' OR recur IS NOT NULL OR mask IS NOT NULL OR i_mask IS NOT NULL LIMIT 1`; if found, return an error containing `旧循环任务数据不支持自动迁移，请备份后重建开发数据库`. Preserve normal task rows, UUID, project_seq and associations during SQLite table rebuild. PostgreSQL drops old recurrence columns only after the same guard passes.

- [ ] **Step 5: 实现 repositories 和 model mapping**

Use one transaction for series row + rule versions + assignees/tags/UDAs. `CreateOccurrence` catches unique constraint, reads by `(workspace_id,series_id,recurrence_at)`, and returns `(existing,true,nil)` regardless of existing task status. `ListOccurrenceExceptions` must query `(recurrence_at >= start AND recurrence_at < end) OR (due >= start AND due < end)` so rescheduled occurrences suppress the original slot and appear on the new date. Series `ListCandidates` applies status/q/assignee, preloads rule versions and never paginates; scheduler-only `ListActive(limit,offset)` remains separate and cannot back the public Series list.

- [ ] **Step 6: 更新 TaskRepository**

Remove `CreateRecurringChild` and `RecurringParents`; map new occurrence columns and override JSON in `Create/Update/fromModel/toModel`. In the same code change delete `StatusRecurring`、`Task.Recur/Mask/IMask`、JSON fields、modification setters and old audit labels from `internal/task`; keep ordinary Task `Until`. `recur/mask/imask` remain explicitly reserved-and-rejected during JSON decode so they cannot silently become UDA fields. `List` no longer needs hidden-template filtering because series is not in `tasks`.

- [ ] **Step 7: 运行 SQLite/PostgreSQL 存储测试**

Run: `go test ./internal/storage -count=1`

Run when PostgreSQL test DSN is configured: `go test ./internal/storage -run Postgres -count=1`

Expected: PASS；未配置 PostgreSQL 时现有测试按项目约定 SKIP，不得伪报通过。

- [ ] **Step 8: 提交**

```bash
git add internal/task internal/storage
git commit -m "feat: 持久化循环系列与唯一实例槽位"
```

### Task 4: 建立 App 统一 View、稳定引用与范围 merge

**目标：** 让普通 Task、projected occurrence 和 materialized occurrence 映射到同一 App view，并实现无副作用的有限范围合并。

**Files:**

- Create: `internal/app/task_occurrence.go`
- Create: `internal/app/task_occurrence_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/app/user_info.go`
- Modify: `internal/query/ast.go`
- Modify: `internal/query/parser_ast.go`
- Modify: `internal/query/parser_ast_test.go`
- Modify: `internal/query/parser.go`
- Modify: `internal/query/parser_test.go`
- Create: `internal/query/evaluator.go`
- Create: `internal/query/evaluator_test.go`
- Modify: `internal/storage/query_scope.go`
- Modify: `internal/storage/query_scope_test.go`
- Modify: `internal/urgency/explain.go`
- Modify: `internal/urgency/urgency.go`
- Modify: `internal/urgency/urgency_test.go`

**Interfaces:**

- Produces:
  - `type OccurrenceMode string` with `auto|materialized|expand`
  - `type TaskOccurrenceView struct`
  - `type RecurrenceInfo struct`
  - `type TaskViewMetadata struct { OccurrenceMode OccurrenceMode; Range *TaskViewRange }`
  - `type TaskViewPage struct { Items []TaskOccurrenceView; Total, Limit, Offset int; OccurrenceMode OccurrenceMode; Range *TaskViewRange }`
  - `func OccurrenceRef(seriesID string, recurrenceAt int64) string`
  - `func ParseOccurrenceRef(string) (seriesID string, recurrenceAt int64, err error)`
  - `func (s *Service) QueryTaskViews(TaskViewQuery) (TaskViewPage, error)`
  - `func (s *Service) collectTaskViewCandidates(TaskViewQuery) ([]TaskOccurrenceView, TaskViewMetadata, error)`; ignores/rejects offset and limit and never paginates
  - `func (s *Service) GetTaskView(ref string) (TaskOccurrenceView, error)`
  - query DSL attributes `series_id`、`recurrence_at`、`task_type:normal|occurrence`; no `recur/mask/imask`
  - `type query.TaskValue struct { ID string; UUID *string; Title string; Description *string; Status string; Entry, Modified, End, Due, Start, Wait, Scheduled, Until *int64; Project, ProjectID, Priority, Parent, SeriesID, TaskType *string; RecurrenceAt *int64; Depends, AnnotationTexts, AssigneeIDs, Tags []string; UDAs map[string]string }`
  - `func query.MatchTaskValue(query.Expr, query.TaskValue, *time.Location) (bool, error)` for post-merge evaluation; App owns the TaskOccurrenceView→TaskValue mapper
  - `type urgency.TaskValue struct { Status string; Entry, Start, Wait, Due *int64; Priority, Project *string; Tags []string; AnnotationCount int; UDAs map[string]string }`
  - `type urgency.Result struct { Total float64; Items []ExplainItem }`
  - `func urgency.ExplainValue(urgency.TaskValue, urgency.Options) urgency.Result`; replaces UUID-coupled `Explain(task.Task, ...)`
  - `type ReportViewInput struct { Name string; Query TaskViewQuery }`
  - `func (s *Service) RunTaskViewReport(ReportViewInput) (TaskViewPage, error)`; replaces `RunReport/ListReport/ReportResult`
- Consumes: Task 1 domain expansion and Task 3 repositories.

- [ ] **Step 1: 写 occurrence_ref 与 merge 失败测试**

Create `internal/app/task_occurrence_test.go`:

```go
func TestOccurrenceRefRoundTrip(t *testing.T) {
	ref := OccurrenceRef("11111111-1111-1111-1111-111111111111", 1783785599)
	seriesID, slot, err := ParseOccurrenceRef(ref)
	if err != nil { t.Fatal(err) }
	if seriesID != "11111111-1111-1111-1111-111111111111" || slot != 1783785599 { t.Fatalf("%s %d", seriesID, slot) }
}

func TestQueryTaskViewsMergesProjectedExceptionAndTombstoneWithoutWrites(t *testing.T) {
	svc, fixture := newOccurrenceFixture(t)
	before := fixture.taskRowCount(t)
	page, err := svc.QueryTaskViews(TaskViewQuery{OccurrenceMode: OccurrenceModeExpand, Range: &TaskViewRange{Start: fixture.day("2026-07-11"), End: fixture.day("2026-07-15")}})
	if err != nil { t.Fatal(err) }
	if got := fixture.ids(page.Items); !reflect.DeepEqual(got, fixture.expectedMergedIDs()) { t.Fatalf("ids=%#v", got) }
	if after := fixture.taskRowCount(t); after != before { t.Fatalf("read materialized rows: before=%d after=%d", before, after) }
	if fixture.auditCount(t) != 0 { t.Fatal("read wrote audit") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'TestOccurrenceRef|TestQueryTaskViewsMerges' -count=1`

Expected: FAIL，提示 view/query 不存在。

- [ ] **Step 3: 实现 View 类型与稳定引用**

Add `taskSeriesRepo` and `taskOccurrenceRepo` to `Service`, initialize them in `NewService`, and rebind both in `withStore` so transactions never retain a non-transactional DB handle. `TaskOccurrenceView` must contain all fields required by current Task reads, plus `ID`、nullable `UUID/TaskSlug/ProjectSeq` and `RecurrenceInfo`. `RecurrenceInfo` fields are `Role`、`SeriesID`、`SeriesStatus`、`Rule`、`RecurrenceAt`、`Materialization`、`Overrides`、`Until`。普通任务 `ID=UUID`、`RecurrenceInfo=nil`; occurrence `ID=occurrence_ref` even after materialization.

- [ ] **Step 4: 实现 merge 算法**

`OccurrenceModeExpand` requires a complete range no longer than 366 days. Load ordinary tasks, candidate series+rule versions, projected slots, and materialized exceptions. Deduplicate by public ID; materialized rows override projected rows; deleted rows suppress projection unless deleted is explicitly requested. Include an exception when either original `recurrence_at` or current `due` intersects the range. `collectTaskViewCandidates` performs range/merge/AST filtering without sort pagination and refuses nonzero Limit/Offset; `QueryTaskViews` calls it, then stable-sorts with `ID` final tie-breaker and paginates.

- [ ] **Step 5: 迁移查询 DSL 并建立内存 evaluator**

Delete `AttrRecur` and every parser/compiler branch for `recur`; `mask/imask` remain only in an explicit retired-name guard, never as AST attributes. Add typed `AttrSeriesID`、`AttrRecurrenceAt`、`AttrTaskType`. `task_type` accepts only `normal|occurrence`; `recurrence_at` uses the same date parser and local day boundaries as other date attributes. Inputs using `recur/mask/imask` must fail with the existing `unknown attribute %q` error and must not fall through to UDA parsing.

Implement `query.MatchTaskValue` against `query.TaskValue` so `internal/query` does not import `internal/app`; the App mapper fills public ID separately from nullable persisted UUID, complete retained task fields, `TaskType=normal|occurrence` and nullable series fields. Projected UUID、Entry、Modified、Start、End remain nil; `available_at` is not copied into query fields. Cover every retained AST attribute/operator: uuid、title、description、status、entry/modified/end/due/start/wait/scheduled/until、project/project_id、priority、depends、annotations、parent、assignee、tag、bare text、UDA、series_id、recurrence_at、task_type, including isnull/notnull parity. Storage SQL and the evaluator must share operator/date helpers where practical and have table-driven parity tests using the same cases. HTTP/MCP/Remote structured `task_type=all|normal|occurrence` maps `all` to no predicate and the other values to DSL `task_type:normal|occurrence`. Notification/automation filter validation continues to call this parser, so old recurrence attributes fail consistently.

Refactor urgency in the same task: remove identity from the calculation input/result, map ordinary/materialized/projected TaskOccurrenceView to `urgency.TaskValue`, and let App add public ID/nullable UUID later. A nil Entry contributes no age; projected blocked/blocking are false. Preserve exact totals/items for ordinary and materialized tasks with parity tests so this is a boundary extraction, not an urgency formula change.

Replace `RunReport/ListReport/ReportResult` with `RunTaskViewReport`. It calls `collectTaskViewCandidates` with pagination fields cleared and never calls `QueryTaskViews`. Merge project scope, context, report definition and user AST before candidate collection; materialized mode may use SQL but expand mode evaluates the complete expression after merge. Build blocked/blocking from the complete materialized workspace dependency graph, treat projected occurrences as neither blocked nor blocking, then apply report scope. Compute waiting/until from the merged view. Explicit input sort overrides the report definition sort; otherwise use the definition. Compute view urgency only when it is the effective sort, always add public ID tie-breaker, and only then apply the original offset/limit. Both `/reports/{name}` and `/tasks?report=` plus CLI aliases、Remote `TaskQueryInput.Report`、MCP `report_run` call this method and return TaskViewPage.

- [ ] **Step 6: 实现 GetTaskView**

For an occurrence ref, first read materialized row; otherwise load scoped series and validate the slot against its rule-version segment, until and effective end. Return `task_occurrence_not_found` for arbitrary/non-member slots. The method must not write. UUID/task_slug lookup continues to return materialized tasks.

- [ ] **Step 7: 补测试并运行**

Add tests for rescheduled exception appearing only at new due while suppressing old projection, deleted tombstone, stopped historical range, rule-version history, 366-day limit, pagination tie-breaker, UserInfo assignees, and `auto` choosing materialized without a full range. Add SQL/evaluator parity cases for every retained attribute/operator, including nullable UUID versus public ID, nullable entry/modified isnull/notnull, project_id、UDA and all date operators; separately cover `series_id`、`recurrence_at`、`task_type`. Add failure tests for `recur/mask/imask` through CLI-style filter, HTTP query and notification-rule validation entrypoints. Add urgency parity tests for ordinary/materialized legacy totals and projected nil-entry/no-age behavior. Add report tests for ready/blocked/blocking/waiting/urgency, projected inclusion, context/project scope, sort tie-breaker, proof that candidate collection receives no pagination, and pagination-after-scope.

Run: `go test ./internal/query ./internal/storage ./internal/urgency ./internal/app -run 'Occurrence|TaskView|QueryParity|RecurrenceQueryAttribute|TaskViewReport|Urgency' -count=1`

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add internal/app/task_occurrence.go internal/app/task_occurrence_test.go internal/app/service.go internal/app/service_test.go internal/app/user_info.go internal/query internal/storage/query_scope.go internal/storage/query_scope_test.go internal/urgency
git commit -m "feat: 合并任务与循环实例视图"
```

### Task 5: 实现 Series CRUD、共享字段和 rule-version 修改

**目标：** 在 App 层提供唯一 Series CRUD，原子创建、读取、修改、停止和跳过，并用完整 UserInfo 输出。

**Files:**

- Create: `internal/app/task_series.go`
- Create: `internal/app/task_series_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/permission.go`
- Modify: `internal/app/task_audit_payload.go`
- Modify: `internal/app/task_audit_payload_test.go`
- Modify: `internal/app/hook_event.go`
- Modify: `internal/app/hook_test.go`

**Interfaces:**

- Produces:
  - `type AddTaskSeriesInput struct`
  - `type ModifyTaskSeriesInput struct`
  - `type StopTaskSeriesInput struct`
  - `type TaskSeriesListInput struct { Project, ProjectID, Status, Q, Assignee, Sort string; Limit, Offset int }`
  - `type TaskSeriesOccurrenceListInput struct { Status string; DueAfter, DueBefore *int64; Limit, Offset int }`; status is `pending|waiting|completed|deleted|all`
  - `type TaskSeriesView struct`
  - `func (s *Service) AddTaskSeries(AddTaskSeriesInput) (TaskSeriesCreateResult, error)`
  - `func (s *Service) ListTaskSeries(TaskSeriesListInput) (TaskSeriesPage, error)`
  - `func (s *Service) GetTaskSeries(id string) (TaskSeriesDetailView, error)`
  - `func (s *Service) ModifyTaskSeries(id string, ModifyTaskSeriesInput) (TaskSeriesView, error)`
  - `func (s *Service) StopTaskSeries(id string, StopTaskSeriesInput) (TaskSeriesView, error)`
  - `func (s *Service) ListTaskSeriesOccurrences(id string, TaskSeriesOccurrenceListInput) (TaskViewPage, error)`
  - `func (s *Service) SkipTaskSeriesOccurrence(seriesID, occurrenceRef string) (TaskOccurrenceView, error)`
  - `func (s *Service) materializeOccurrenceLocked(series taskseries.Series, slot taskseries.Slot, status string) (task.Task, bool, error)`
  - `func (s *Service) ReconcileTaskSeries(seriesID string, now int64, limit int) (TaskSeriesReconcileResult, error)`
- Consumes: Tasks 1–4.

- [ ] **Step 1: 写创建与修改失败测试**

Create `internal/app/task_series_test.go` with tests that:

1. create a future daily series and assert one series row, one initial rule version, no task row, projected first occurrence and `task.series.created` audit;
2. create a due-today series and assert first occurrence is materialized with project_seq and `task.created` follows `task.series.created`;
3. modify weekly→daily with explicit future `effective_from`, assert old rule row remains and future preview uses the new anchor;
4. reject rule modify without `effective_from`, with effective date today/past, beyond until, or while backlog remains;
5. reject wait/scheduled/depends/parent and closed project.
6. list occurrences with each canonical status plus all, due range and pagination; pending/waiting are both open and invalid status fails before repository access.
7. list Series with status/q/assignee/sort and pagination, resolving assignee in App; freeze clock/location and assert next across rule-version cutover/until/stopped/null, prove an early-materialized future occurrence does not change the next calendar slot, then assert filtered total, pagination after sort and stable ID tie-breaker.

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'Test(Add|Modify)TaskSeries' -count=1`

Expected: FAIL，提示 use cases/types 不存在。

- [ ] **Step 3: 实现输入、View 和创建事务**

Use the Task 4 repositories already wired into `Service`. `AddTaskSeriesInput` fields: Title, Description, Project/ProjectID, RecurrenceRule, FirstDue, Until, Priority, Assignees, Tags, UDAs. Resolve project and members using existing App helpers. In one `Store.Transaction`: create series+associations+initial RuleVersion, append `task.series.created`, and call `materializeOccurrenceLocked` only when first slot's local `available_at <= now`. Implement the workspace-scoped `ReconcileTaskSeries` core here; Task 7's scheduler only finds active series, applies global limits and invokes this method.

`TaskSeriesView.CreatedBy` and all assignees must use `task.UserInfo`; unresolved IDs use the repository-wide fallback `{ID:id,Name:id}`. Register `task.series.created|modified|ended|stopped` and `task.recurrence.generated|skipped` in the event allowlist. Pure projection emits nothing; materialization emits one `task.created` with recurrence_info.

`ListTaskSeriesOccurrences` validates status against one shared enum derived from `task.Status` after recurring is removed; `all` adds no predicate. Use the same input/view in HTTP、Remote、MCP、CLI so waiting cannot disappear at one transport. Series detail open occurrences include pending and waiting but remain capped as specified; complete history always uses this paged method.

`ListTaskSeries` trims Q, validates `status=active|ended|stopped|all` and `sort=next|title|modified`, resolves Assignee through the existing workspace user-reference resolver, and passes only user ID to storage. It receives the complete filtered candidate set, computes each active Series' earliest legal recurrence_at strictly after injected now using workspace location and full rule-version/effective_end/until semantics, without querying/skipping early-materialized future occurrences, then applies next/title/modified stable sort and offset/limit. `TaskSeriesPage.Total=len(candidates)` before pagination. Invalid assignee/sort fails before repository access; all transports consume the same page total。

- [ ] **Step 4: 实现修改事务**

Before any modify, run reconcile for entered backlog; if remaining is nonzero, return `task_recurrence_backlog` without changes. Shared field updates sync only open materialized occurrences whose `RecurrenceOverrides` lacks that field. Rule updates require `EffectiveFrom`, append one RuleVersion, update current `RecurrenceRule`, and return three future slots. `FirstDue` and project are immutable. Shortening until never deletes existing occurrences; when the new inclusive until is already behind the execution boundary, atomically mark the series ended and retain history.

- [ ] **Step 5: 实现 stop 与 skip**

Stop writes `status=stopped`、`effective_end_at=now`、reason and audit. Default preserves open occurrences. With `DeleteOpenOccurrences`, delete materialized open rows and create tombstones for entered projected slots in the same transaction; reject more than 1000 slots atomically. Skip validates series membership and creates/updates one deleted occurrence with `task.recurrence.skipped` audit.

- [ ] **Step 6: 运行 CRUD 测试**

Run: `go test ./internal/app -run 'TaskSeries|SeriesOccurrence' -count=1`

Expected: PASS；事务失败测试证明 series/audit/occurrence 不会部分提交。

- [ ] **Step 7: 提交**

```bash
git add internal/app/task_series.go internal/app/task_series_test.go internal/app/service.go internal/app/permission.go internal/app/task_audit_payload.go internal/app/task_audit_payload_test.go internal/app/hook_event.go internal/app/hook_test.go
git commit -m "feat: 实现循环系列完整用例"
```

### Task 6: 实现写前物化与 occurrence 生命周期

**目标：** 所有 Task 写入口都能解析 occurrence_ref；仅合法首次写在一个事务中物化并执行本次动作，读取、no-op 和失败动作不制造实体。

**Files:**

- Modify: `internal/app/task_occurrence.go`
- Modify: `internal/app/task_occurrence_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/app/workspace.go`
- Modify: `internal/app/task_change_events.go`
- Modify: `internal/app/task_change_events_test.go`

**Interfaces:**

- Produces:
  - `func (s *Service) ResolveTaskForRead(ref string) (TaskOccurrenceView, error)`
  - `func (s *Service) MaterializeOccurrenceForWrite(ref string) (task.Task, bool, error)`
  - `func (s *Service) WithTaskForWrite(ref string, action func(*Service, task.Task) error) (TaskOccurrenceView, error)`
  - `func (s *Service) WithExistingTaskForSubresourceWrite(ref string, action func(*Service, task.Task) error) (TaskOccurrenceView, error)`; projected returns resource not found without materializing
  - `type UrgencyView struct { ID string; UUID *string; Total float64; Items []urgency.ExplainItem }`
  - `func (s *Service) ExplainTaskViewUrgency(ref string) (UrgencyView, error)`; projected uses merged fields and never materializes
- Consumed by HTTP, MCP, CLI and task subresource handlers.

- [ ] **Step 1: 写动作矩阵失败测试**

Add table tests for projected `modify/start/done/delete/annotate/link-add/dependency-add/child-add` and projected invalid `stop/reopen/denotate/link-update/link-remove`. Assert valid writes create exactly one task and preserve public ID; invalid actions return the existing status/not-found errors and leave task/audit/event row counts unchanged. Add no-op modify/clear-empty tests with the same no-write assertion. Run each valid action twice concurrently and assert one physical row.

Add read tests proving projected annotations、links、children、audit return empty list/page, urgency is calculated from merged fields, and all reads leave task/audit/event row counts unchanged.

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'ProjectedOccurrence|MaterializeOccurrence' -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现 resolver 与原子事务包装**

Parse occurrence_ref, validate scope/series/rule version, create Task with snapshot/current shared fields, allocate project_seq and append `task.created`; then execute action before the same transaction commits. If unique insert loses a race, reload the winner and continue the action. Return `TaskOccurrenceView` whose ID remains occurrence_ref. Action handlers must finish permission/input/status/resource-existence validation and projected no-op diff before calling the materializer; a transaction rollback must remove both the row and audit/event outbox writes on any later error.

- [ ] **Step 4: 记录字段 override**

Ordinary task modifications on occurrence add sorted unique override names for title/description/priority/due/assignees/tags/udas/wait/scheduled/depends, including clear operations. Completed/deleted occurrences never receive shared series updates. Due modification leaves recurrence_at unchanged.

- [ ] **Step 5: 接入全部 task 子资源**

Replace direct `Info→UUID→write` sequences for annotation/link/dependency/child **add** with `WithTaskForWrite`. Annotation update/delete and link update/remove use `WithExistingTaskForSubresourceWrite`: ordinary/materialized tasks keep current behavior; projected returns the same 404 as a missing child resource and never materializes. A projected occurrence may become parent of a manual child after materialization; attempts to set an occurrence's own parent remain rejected. Read methods short-circuit projected annotations/links/children/audit to empty values and calculate urgency from TaskOccurrenceView without requiring UUID.

- [ ] **Step 6: 验证事件顺序和并发**

Run: `go test ./internal/app -run 'ProjectedOccurrence|OccurrenceOverride|TaskChangeEvent' -count=1`

Expected: PASS；audit/Hook order is `task.created` then requested action, and duplicate concurrent calls do not duplicate `task.created`.

- [ ] **Step 7: 提交**

```bash
git add internal/app
git commit -m "feat: 原子物化并操作循环实例"
```

### Task 7: 日历 reconcile、Scheduler、项目关闭与统计

**目标：** 每个已到日历槽位都能独立物化，停机后分批补齐；项目关闭停止 series；普通进度与循环运行指标分离。

**Files:**

- Create: `internal/app/task_series_scheduler.go`
- Create: `internal/app/task_series_scheduler_test.go`
- Modify: `internal/cli/server.go`
- Modify: `internal/cli/server_test.go`
- Modify: `internal/app/project.go`
- Create: `internal/app/project_test.go`
- Modify: `internal/storage/project_repo.go`
- Modify: `internal/storage/project_repo_test.go`
- Modify: `internal/httpapi/projects.go`
- Modify: `internal/httpapi/projects_test.go`

**Interfaces:**

- Produces:
  - `type TaskSeriesReconcileResult struct { Created int; BacklogRemaining int; Ended bool }`
  - `type TaskSeriesSchedulerOptions struct { Store *storage.Store; Clock Clock; ServiceFactory func(string) *Service; PerSeriesLimit, GlobalLimit int }`
  - `func NewTaskSeriesScheduler(TaskSeriesSchedulerOptions) *TaskSeriesScheduler`
  - `func (s *TaskSeriesScheduler) RunOnce(context.Context) (TaskSeriesReconcileResult, error)`
  - `func (s *TaskSeriesScheduler) Run(context.Context, time.Duration) error`
  - project summary fields `RecurringSeriesCount`、`ActiveRecurringSeriesCount`、`OpenRecurringOccurrenceCount`、`OverdueRecurringOccurrenceCount`

- [ ] **Step 1: 写 scheduler 失败测试**

Create tests for: daily yesterday still open but today materializes; five-day downtime creates five rows; repeated/concurrent RunOnce is idempotent; 100-per-series and 1000-global limits return backlog; until transitions active→ended; closed project stops series and creates nothing after close.

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'TaskSeriesScheduler|DailySeries|SeriesBacklog' -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现 scheduler**

Follow `ProjectAutomationScheduler` wiring: default ServiceFactory, workspace-aware services, `RunOnce`, context cancellation and 60-second `Run`. Scan only active series. For each rule-version slot with local `available_at <= now`, call the same occurrence materializer used by writes. Continue from stored max slot/rule-version anchors, never from editable due. When inclusive until has passed and every legal slot is reconciled, transition to ended once and emit `task.series.ended`.

- [ ] **Step 4: 接入 server 生命周期**

Start one TaskSeriesScheduler in `xuanchu server`: call RunOnce before accepting steady-state ticks, then run every 60 seconds under the existing shutdown coordinator. Local CLI pre-command reconcile is implemented in Task 9 after command exclusions are known.

- [ ] **Step 5: 接入项目 transition**

In the same `TransitionProject` transaction, when target status is archived/cancelled, stop all active project series with reason `project_archived|project_cancelled` and enqueue one `task.series.stopped` event per changed series. Restoring project state does not restore series.

- [ ] **Step 6: 修改项目统计**

Ordinary task_count/pending_count/completed_count must add `series_id IS NULL`. Add the four recurring metrics from series and materialized occurrence repositories. HTTP response and App view use explicit fields; no field is inferred in Web.

- [ ] **Step 7: 运行 scheduler/project 测试**

Run: `go test ./internal/app ./internal/storage ./internal/httpapi ./internal/cli -run 'TaskSeries|ProjectTaskSummary|TransitionProject|Server' -count=1`

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add internal/app internal/storage/project_repo.go internal/storage/project_repo_test.go internal/httpapi/projects.go internal/httpapi/projects_test.go internal/cli/server.go internal/cli/server_test.go
git commit -m "feat: 按日历补齐循环实例"
```

### Task 8: 统一 HTTP、OpenAPI 与 Remote 契约

**目标：** 暴露 Series CRUD、范围 TaskView 查询和 occurrence 写操作，并让 Remote client 完全复用 HTTP 形状。

**Files:**

- Create: `internal/httpapi/task_series.go`
- Create: `internal/httpapi/task_series_test.go`
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/tasks_test.go`
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/error_status.go`
- Modify: `internal/httpapi/error_status_test.go`
- Create: `internal/remote/task_series.go`
- Create: `internal/remote/task_series_test.go`
- Modify: `internal/remote/task.go`
- Modify: `internal/remote/task_test.go`
- Modify: `internal/remote/config.go`
- Modify: `internal/remote/config_test.go`
- Modify: `internal/remote/client.go`
- Modify: `web/src/features/workspace/project-workbench/api/task-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/task-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-project-data.ts`

**Interfaces:**

- HTTP routes:
  - `POST /api/v1/task-series`
  - `GET /api/v1/task-series`
  - `GET/PATCH/DELETE /api/v1/task-series/{seriesRef}`
  - `GET /api/v1/task-series/{seriesRef}/occurrences`
  - `POST /api/v1/task-series/{seriesRef}/occurrences/{occurrenceRef}/skip`
  - `GET /api/v1/tasks?due_after=&due_before=&occurrence_mode=`
  - `GET /api/v1/reports/{name}?due_after=&due_before=&occurrence_mode=&task_type=`; same TaskViewPage as `/tasks?report={name}`
- Task list query also accepts `task_type=all|normal|occurrence`; it filters merged App views and is not a task status.
- `remote.TaskQueryInput` exposes `TaskType string` with the same enum and sends it as the structured `task_type` parameter.
- Series occurrence list status is exactly `pending|waiting|completed|deleted|all` in HTTP/Remote/MCP/CLI; `all` adds no predicate.
- Series list accepts `status/q/assignee/sort=next|title|modified/limit/offset`; Remote `TaskSeriesListInput` mirrors those fields.
- Remote produces `AddTaskSeries`、`ListTaskSeries`、`GetTaskSeries`、`ModifyTaskSeries`、`StopTaskSeries`、`ListTaskSeriesOccurrences`、`SkipTaskSeriesOccurrence`.
- Remote replaces `ListTasks/GetTask` with `QueryTasks/GetTaskView`; all task reads/actions use `TaskOccurrenceDTO` or `TaskViewPageDTO`, never `task.Task`.

HTTP error mapping must cover `task_series_not_found`(404)、`task_series_inactive`(409)、`task_series_due_required`(400)、`task_series_invalid_until`(400)、`task_series_invalid_rule`(400)、`task_series_invalid_effective_from`(400)、`task_series_unsupported_field`(400)、`task_series_project_closed`(409)、`task_series_occurrence_not_found`(404)、`task_recurrence_backlog`(409)、`task_series_endpoint_required`(400)、`task_occurrence_not_found`(404)、`task_occurrence_range_required`(400)、`task_occurrence_range_too_large`(400). Permission remains 403 and hidden scope remains 404.

- [ ] **Step 1: 写 HTTP 合约失败测试**

Add tests for full Series lifecycle, required project/write permission, scope hiding, closed project, recurrence_rule/effective_from validation, UserInfo output, stop/delete_open, and all documented errors/status mappings. Series list contract tests cover status/q/assignee/all sort modes, invalid values, filtered total, pagination and HTTP/Remote parity. Add task list tests for auto/materialized/expand, missing range, >366 days, inclusive date input, projected GET without writes and stable occurrence ID after done. Test `/reports/{name}` and `/tasks?report=` with identical range/mode/task_type/query, asserting identical TaskViewPage for ready/blocked/blocking/waiting/urgency and pagination after scope/sort. Exercise every existing task/subresource route with an occurrence_ref: projected annotation/link add materializes; annotation/link update/delete returns 404 without materialization; annotations/links/children/audit reads are empty; urgency is computed; no-op/failed writes leave task/audit/event counts unchanged.

Use this response assertion shape:

```go
var page struct {
	Data struct {
		Items []struct {
			ID string `json:"id"`
			UUID *string `json:"uuid"`
			RecurrenceInfo *struct {
				SeriesID string `json:"series_id"`
				Materialization string `json:"materialization"`
			} `json:"recurrence_info"`
		} `json:"items"`
		Total int `json:"total"`
		OccurrenceMode string `json:"occurrence_mode"`
	} `json:"data"`
}
```

- [ ] **Step 2: 运行 HTTP 测试确认失败**

Run: `go test ./internal/httpapi -run 'TaskSeries|TaskOccurrence|TaskRange|Report|Urgency' -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现 DTO/handlers/routes**

Use public JSON field `recurrence_rule`, never `recur`. Task list and both report routes return `TaskViewPage` data instead of a bare array and call `RunTaskViewReport` when a report name is present. Generic POST `/tasks` uses strict decoding; if raw JSON contains `recur`, return `task_series_endpoint_required`, otherwise unknown fields follow current strict-input policy. PATCH task schema has no recurrence field. Every task and subresource path passes the original taskRef/occurrence_ref to App; add routes use the materializing resolver, existing-subresource mutation routes use the non-materializing existing-task resolver, and read routes follow the projected empty/calculated semantics from Task 6.

- [ ] **Step 4: 注册 Huma 并验证 OpenAPI**

Register all seven routes in `huma_routes.go`, use request/response structs so `/openapi.json` includes enums, required fields and nullable projected UUID. Add a server test that fetches `/openapi.json` and asserts `task-series`, Series list q/assignee/next|title|modified, `occurrence_mode`, `recurrence_rule`, occurrence status `pending|waiting|completed|deleted|all`, report TaskViewPage, and no task `recur` property.

- [ ] **Step 5: 实现 Remote client**

Mirror HTTP DTOs in `remote/task_series.go`; define `TaskOccurrenceDTO` and `TaskViewPageDTO` in `remote/task.go`. Delete `ListTasks` and add `QueryTasks(ctx, TaskQueryInput) (TaskViewPageDTO, error)`; `TaskQueryInput.Report` selects the same App report path. Delete `GetTask` and add `GetTaskView(ctx, workspace, taskRef) (TaskOccurrenceDTO, error)`. Change Add/Modify/Done/Delete/Start/Stop/Reopen/Annotate/Denotate and dependency/link/child action methods to return TaskOccurrenceDTO or the HTTP-equivalent subresource DTO; all target-taking methods accept the original taskRef. Change `ExplainUrgency` in `remote/config.go` to accept/pass taskRef and return projected urgency without resolving UUID. Remove `Recur/ClearRecur`, and do not add compatibility wrappers. Update all Remote tests and callers in the same branch; verify occurrence_ref is URL-encoded exactly once.

- [ ] **Step 6: 运行协议测试**

Run: `go test ./internal/httpapi ./internal/remote -count=1`

Update the existing Web task adapter in the same task to read `data.items`/page metadata and keep current project task screens functional before the Series routes land. Run: `pnpm --dir web test -- task-api.test.ts project-api.test.ts`

Run: `pnpm --dir web typecheck`

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add internal/httpapi internal/remote web/src/features/workspace/project-workbench/api web/src/features/workspace/project-workbench/hooks/use-project-data.ts
git commit -m "feat: 提供循环系列 HTTP 与远程契约"
```

### Task 9: 新增专用 CLI 并移除旧循环命令

**目标：** CLI 用 `xuanchu series ...` 管理系列；普通 add/modify 不再解释循环字段；本地任务命令按约定触发 reconcile。

**Files:**

- Create: `internal/cli/series.go`
- Create: `internal/cli/series_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/add.go`
- Modify: `internal/cli/activity.go`
- Modify: `internal/cli/annotation.go`
- Modify: `internal/cli/edit.go`
- Modify: `internal/cli/helper.go`
- Modify: `internal/cli/list.go`
- Modify: `internal/cli/info.go`
- Modify: `internal/cli/link.go`
- Modify: `internal/cli/report.go`
- Modify: `internal/cli/urgency.go`
- Modify: `internal/cli/import_export.go`
- Modify: `internal/cli/root_test.go`
- Modify: `internal/edit/edit.go`
- Modify: `internal/edit/edit_test.go`
- Modify: `internal/render/table.go`
- Modify: `internal/render/table_test.go`
- Modify: `tests/integration/cli_test.go`

**Interfaces:**

- Commands:
  - `xuanchu series add <title> --project --recur --first-due [--until]`
  - `xuanchu series list --project [--status --query --assignee --sort=next|title|modified --limit --offset]`
  - `xuanchu series info <series-ref>`
  - `xuanchu series modify <series-ref> [--recur --effective-from --until ...]`
  - `xuanchu series occurrences <series-ref> [--status pending|waiting|completed|deleted|all] [--due-after --due-before --limit --offset]`
  - `xuanchu series stop <series-ref> [--delete-open]`
  - `xuanchu series skip <series-ref> <occurrence-ref>`
  - task list/report flags `--due-after --due-before --occurrence-mode=auto|materialized|expand`

- [ ] **Step 1: 写 CLI 失败测试**

Add integration tests that create daily series, list/info/modify/occurrences/skip/stop in human and JSON modes, assert stdout contains only result JSON under `--json`, errors go to stderr, and series ID/occurrence_ref are accepted. Series list tests cover query/assignee/sort/pagination parity with HTTP and filtered total in JSON. Modify-rule tests must pass `--effective-from`; missing it fails without writes. `series occurrences` tests page complete history independently of the `series info` recent-history limit.

Add a table covering occurrence_ref through `info/modify/delete/start/stop/reopen/annotate/denotate/append/prepend/edit/link add/list/remove/annotations/urgency/_urgency`; assert legal projected writes materialize and stop/reopen/nonexistent denotate/link-remove do not. External edit must open from a projected view without writes; cancellation, editor failure and unchanged content leave task/audit/event counts unchanged, while a non-empty diff atomically materializes and modifies once. Add list/report tests for all occurrence modes, ready/blocked/blocking/waiting/urgency semantics and date-range errors. Assert projected rows render ID/UUID/SLUG as `-`, are absent from `_ids/_uuids`, `_get ...uuid` is null, and become working-set-addressable only after materialization. Assert `_projects/_tags/_unique` stay materialized-only and `_udas/_show/_version` are unchanged. Add tests proving `xuanchu add x recur:daily`、`xuanchu 1 modify recur:weekly` and query attributes `recur/mask/imask` fail with nonzero exit and do not create/modify data.

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cli ./tests/integration -run 'Series|RecurringCommand' -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现 Cobra command tree**

Create a `newSeriesCommand` with add/list/info/modify/occurrences/stop/skip children. Use existing workspace/project scope resolution and Renderer patterns. `--effective-from` is required when `--recur` changes an existing series. `occurrences` passes status/range/pagination to `ListTaskSeriesOccurrences` and renders the returned page without truncating to the info summary. Human output uses “循环系列/实例/槽位”; JSON serializes App view directly.

- [ ] **Step 4: 删除旧字段解析**

Remove `recur`, `mask`, `imask` from add/modify/edit/import field handling and help. Do not reinterpret old syntax. Keep `parent` only for manual sub-task creation. Remove old recurring status from reports and completion candidates.

- [ ] **Step 5: 迁移现有 task 命令、working set 与 renderer**

Wire `--due-after/--due-before/--occurrence-mode` into local `TaskViewQuery` and Remote `TaskQueryInput` for list and every report alias; aliases call `RunTaskViewReport` so report scope and urgency run before pagination. Pass occurrence_ref unchanged through every read/write command. Replace task-only human/JSON rendering paths with TaskOccurrenceView rendering; projected rows use `-` for numeric ID/UUID/SLUG and a “计划实例” marker. Keep numeric working-set IDs and `_ids/_uuids` limited to persisted tasks; `_get` reads the view and emits null for projected-only identity fields. Generate external edit JSON from TaskOccurrenceView with immutable public ID and nullable UUID, without materializing; remove Recur/Mask/IMask and reject injected recurrence identity/snapshot fields. Only after successful editor exit and non-empty diff call the normal modify path, which atomically materializes if necessary. Append/prepend call the same modify path and record description override.

- [ ] **Step 6: 接入本地 reconcile**

Before local task/project read/write commands, call workspace-scoped `ReconcileTaskSeries` once. Exclude `server`, `mcp`, `completion`, `help`, `version`, config-only commands and remote client mode. Reconcile warnings go to stderr and never contaminate `--json` stdout; hard storage errors fail the command.

- [ ] **Step 7: 运行 CLI 测试**

Run: `go test ./internal/cli ./tests/integration -count=1`

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add internal/cli internal/edit internal/render tests/integration/cli_test.go
git commit -m "feat: 增加循环系列命令"
```

### Task 10: 统一 MCP tools、schema 与结构化输出

**目标：** Agent 通过七个专用 tools 管理 Series，task tools 同 HTTP 一样读写 occurrence_ref，输出不再暴露 hidden Task 语义。

**Files:**

- Create: `internal/mcpserver/tools_task_series.go`
- Create: `internal/mcpserver/tools_task_series_test.go`
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/tools_report.go`
- Modify: `internal/mcpserver/tools_views.go`
- Modify: `internal/mcpserver/schema_test.go`
- Modify: `internal/mcpserver/integration_test.go`
- Create: `internal/mcpserver/testdata/task_series_add.schema.json`
- Create: `internal/mcpserver/testdata/task_series_list.schema.json`
- Create: `internal/mcpserver/testdata/task_series_get.schema.json`
- Create: `internal/mcpserver/testdata/task_series_modify.schema.json`
- Create: `internal/mcpserver/testdata/task_series_stop.schema.json`
- Create: `internal/mcpserver/testdata/task_series_list_occurrences.schema.json`
- Create: `internal/mcpserver/testdata/task_series_occurrence_skip.schema.json`
- Modify: `internal/mcpserver/testdata/task_add.schema.json`
- Modify: `internal/mcpserver/testdata/task_modify.schema.json`
- Modify: `internal/mcpserver/testdata/task_query.schema.json`
- Modify: `internal/mcpserver/testdata/report_run.schema.json`
- Modify: `internal/mcpserver/testdata/urgency_explain.schema.json`
- Modify: `internal/mcpserver/testdata/list-tools-default.json`

**Interfaces:**

- Tools: `task_series_add`、`task_series_list`、`task_series_get`、`task_series_modify`、`task_series_stop`、`task_series_list_occurrences`、`task_series_occurrence_skip`.
- `task_series_list` adds `status/q/assignee/sort=next|title|modified/limit/offset` with HTTP-equivalent filtered total.
- Task query adds `due_after`、`due_before`、`occurrence_mode`、`task_type=all|normal|occurrence`.
- `report_run` adds `due_after`、`due_before`、`occurrence_mode`、`task_type` and returns TaskViewPage; `urgency_explain` accepts occurrence_ref.

- [ ] **Step 1: 写 schema 与行为失败测试**

Add tests that list all seven tool names, compare generated schemas to golden files, and execute add→series list filters/sorts→query expand→get projected→done→series get→list occurrences→skip→stop. Assert project/workspace/token allowlist, `task:read/write` permissions, complete UserInfo, filtered Series total, stable occurrence ID and `materialization` transition.

Add a table over every existing task tool affected by occurrence_ref: modify/start/done/delete/annotate/depends/link-add materialize projected occurrences; stop/reopen return state errors; denotate/link-remove return not found; link-list returns an empty list and task-get returns the projected view; `urgency_explain` calculates projected urgency. All failure/read cases leave task/audit/event counts unchanged. MCP has no independent children/audit tool, so do not invent recurrence-only duplicates. Add `report_run` parity tests against HTTP for ready/blocked/blocking/waiting/urgency, range/mode/task_type and pagination. Add `task_export/task_import` native-bundle schema/round-trip tests in Task 14 and remove old array fixtures there.

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/mcpserver -run 'TaskSeries|Occurrence|Schema|Report|Urgency' -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现 series tool inputs/handlers**

Use `recurrence_rule`, `first_due|first_due_date`, `until|until_date`, and `effective_from|effective_from_date`. `task_series_modify` requires effective_from only when rule changes. Every handler calls the App methods from Task 5 and returns `successWithEnvelope`.

- [ ] **Step 4: 迁移 task tools**

`task_add/task_modify` schemas contain no recur. `task_query` exposes due range、occurrence_mode、task_type, builds `TaskViewQuery` and returns the same page shape as HTTP; `task_type=all` adds no predicate and normal/occurrence compile to the shared AST. Legal first-write tools pass occurrence_ref into App `WithTaskForWrite`; projected stop/reopen return status errors, while denotate/link-remove use the existing-task resolver and return not found without materialization. `task_link_list` returns the projected empty list and `task_get` returns the projected view without writes. `task_export/import` moves to Task 14's native bundle; until then keep tools compiling but mark their old payload tests for replacement in the same branch, never ship an intermediate release.

Migrate `report_run` from `ListReport`/task arrays to `RunTaskViewReport`/TaskViewPage and add the range/mode/task_type inputs. Change `urgency_explain` to pass the original taskRef to the App projected-aware urgency method instead of resolving UUID first. Both text and structuredContent use the same view/page envelope.

- [ ] **Step 5: 统一 text 与 structuredContent**

`structuredContent.data` contains the HTTP-equivalent view. `Content[0].text` remains serialized ToolEnvelope, while human `rendered` says “计划 occurrence” for projected and “已物化实例” for materialized; never claim a projected task was created.

- [ ] **Step 6: 更新 golden files 并运行测试**

Run: `go test ./internal/mcpserver -run 'Schema|ListTools' -count=1 -update`

Inspect every golden diff, then run: `go test ./internal/mcpserver -count=1`

Expected: PASS and no dot-separated tool names.

- [ ] **Step 7: 提交**

```bash
git add internal/mcpserver
git commit -m "feat: 为 MCP 提供循环系列工具"
```

### Task 11: 建立 Web 原生类型、查询缓存与任务页面板路由

**目标：** 前端只消费 TaskView/SeriesView；Series list/detail 使用任务页内可深链面板，不增加全局导航或 ProjectTabs 项。

**Files:**

- Create: `web/src/features/workspace/project-workbench/api/task-series-api.ts`
- Create: `web/src/features/workspace/project-workbench/api/task-series-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/api/task-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/task-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-project-data.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-panel-shell.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-panel-shell.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-layout.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-layout.test.tsx`
- Modify: `web/src/routes/workspace/ProjectTasksRoute.tsx`
- Create: `web/src/routes/workspace/ProjectTasksRoute.test.tsx`
- Create: `web/src/routes/workspace/ProjectTaskSeriesPanelRoute.tsx`
- Create: `web/src/routes/workspace/ProjectTaskSeriesPanelRoute.test.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/i18n.test.ts`

**Interfaces:**

- Produces TS types `TaskOccurrenceView`、`RecurrenceInfo`、`TaskViewPage`、`TaskSeriesView`、`TaskSeriesDetailView` and API/query key helpers.
- Routes:
  - `/workspaces/$workspaceSlug/projects/$projectSlug/tasks/series`
  - `/workspaces/$workspaceSlug/projects/$projectSlug/tasks/series/$seriesRef`
- `ProjectLayoutContextValue.setContextPanel(panel: { node: ReactNode; onClose: () => void } | null)` replaces the project context rail without mutating the user's saved `railOpen` state.
- `type ListRestoreState = { scrollTop: number; focusId?: string; selectedIds: string[] }`
- `type PanelReturnTo = { kind: 'tasks'; search: Record<string,string>; restore?: ListRestoreState } | { kind: 'task'; taskRef: string } | { kind: 'my-tasks'; search: Record<string,string>; restore?: ListRestoreState }`; search snapshots pass their route validators before use, targets are rebuilt from current scoped params, and arbitrary href is impossible.

- [ ] **Step 1: 写 API URL/body 失败测试**

Create tests asserting:

```ts
expect(taskSeriesPath("acme", { project: "ops", status: "active" })).toBe(
  "/api/v1/task-series?workspace=acme&project=ops&status=active"
)
expect(taskSeriesPath("acme", { project: "ops", status: "active", q: "巡检", assignee: "zhangsan", sort: "next", limit: 20, offset: 20 })).toContain(
  "q=%E5%B7%A1%E6%A3%80"
)
expect(taskSeriesItemPath("acme", "series:1")).toBe(
  "/api/v1/task-series/series%3A1?workspace=acme"
)
expect(tasksPath("acme", { due_after: "2026-07-12", due_before: "2026-07-12", occurrence_mode: "expand" })).toContain(
  "occurrence_mode=expand"
)
```

Update TaskCreateInput/TaskModifyInput compile-time tests so `recur/clear_recur/mask/imask` are absent and occurrence fields are read-only.

Add an active-count contract test using `GET /task-series?status=active&limit=1`; the toolbar reads `page.total` and never fetches the full collection for its badge.

- [ ] **Step 2: 运行 API 测试确认失败**

Run: `pnpm --dir web test -- task-series-api.test.ts task-api.test.ts`

Expected: FAIL。

- [ ] **Step 3: 实现 TS contracts 和 API client**

`TaskOccurrenceView.id` is always present; `uuid/task_slug/project_seq` are nullable. `recurrence_info` is nullable and uses `projected|materialized`. API functions cover add/list/get/modify/stop/list occurrences/skip and task range query. Series list types expose status/q/assignee/sort/limit/offset and the response total; no component filters a returned page locally. All paths use `encodeURIComponent` once.

- [ ] **Step 4: 增加 query keys、静态子路由与面板槽位**

Add stable keys rooted at `['project', workspaceSlug, projectSlug, 'task-series']`. Make `/tasks` a persistent parent route whose `ProjectTasksRoute` renders ProjectLayout、ProjectTasksPage and an Outlet. Register `series` and `series/$seriesRef` as static children using `ProjectTaskSeriesPanelRoute`; the child only registers/unregisters the panel, so list→detail→closed never remounts ProjectTasksPage. Keep `/tasks/$taskRef` as the task-detail route and ensure static Series matching wins. The parent owns one search validator including `task_type`; children inherit/preserve every search param. Same-session navigation may carry the structured `PanelReturnTo` union for task list/detail/My Tasks; close validates the kind/ref/search and reconstructs a scoped route, while copied canonical links never serialize it.

- [ ] **Step 5: 扩展 ProjectLayout 的上下文面板槽位**

Add `setContextPanel` to the layout context and a data-agnostic `TaskSeriesPanelShell`. A registered shell replaces `ProjectContextRail`; the existing rail collapse action changes aria-label/title to “关闭循环任务” and calls the custom `onClose`, while closing/unregistering restores the prior context-rail open/collapsed state. Tests assert `ProjectTabKey` remains exactly `overview|tasks|activity|automations`, Series children highlight tasks, the project breadcrumb remains “项目 > 任务” while only the panel owns “循环任务 > 标题”, static `series` is never parsed as taskRef, list/detail child navigation preserves the same ProjectTasksPage instance, and direct deep links load parent tasks plus the optional panel target. Shell tests lock desktop/mobile rendering, close focus and aria labels without depending on Series API data.

- [ ] **Step 6: 补 i18n 与运行测试**

Add panel/status/empty/error/action keys in zh-CN and en-US; do not add a project-tab translation key. User-facing panel、breadcrumb、toolbar、close、empty and view actions use “循环任务/Recurring tasks”; only the frequency field uses “循环规则/Recurrence rule”. Tests assert locale key parity and reject the old panel/action copy.

Run: `pnpm --dir web test -- task-series-api.test.ts task-series-panel-shell.test.tsx project-layout.test.tsx ProjectTasksRoute.test.tsx ProjectTaskSeriesPanelRoute.test.tsx i18n.test.ts`

Run: `pnpm --dir web typecheck`

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add web/src/features/workspace/project-workbench/api web/src/features/workspace/project-workbench/hooks web/src/features/workspace/project-workbench/project web/src/features/workspace/project-workbench/task-series/task-series-panel-shell.tsx web/src/features/workspace/project-workbench/task-series/task-series-panel-shell.test.tsx web/src/routes web/src/locales web/src/i18n.test.ts
git commit -m "feat: 建立任务页循环任务面板路由"
```

### Task 12: 实现统一创建弹窗与任务页 Series 管理面板

**目标：** 在融合任务页内完成 Series 列表、详情、创建、编辑和停止；任务列表始终是唯一执行视图，严格对应 Spec ASCII 原型。

**Files:**

- Create: `web/src/features/workspace/project-workbench/task-series/task-series-panel.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-panel.test.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-list.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-list.test.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-detail.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-detail.test.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-dialog.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-dialog.test.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-stop-dialog.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/task-series-stop-dialog.test.tsx`
- Create: `web/src/features/workspace/project-workbench/task-series/recurrence-preview.ts`
- Create: `web/src/features/workspace/project-workbench/task-series/recurrence-preview.test.ts`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.test.tsx`
- Modify: `web/src/routes/workspace/ProjectTaskSeriesPanelRoute.tsx`
- Modify: `web/src/routes/workspace/ProjectTaskSeriesPanelRoute.test.tsx`

**Interfaces:**

- `TaskCreateDialog` adds `initialMode?: 'normal'|'recurring'`, keeps independent form state per mode, and reports a discriminated task/series create result.
- `TaskSeriesPanel` consumes `{ workspaceSlug, projectSlug, seriesRef?: string, onClose }`; list/detail navigation preserves task search params.
- Series mutations invalidate series list/detail, project tasks, project summary and timeline.

- [ ] **Step 1: 写统一创建弹窗失败测试**

Add tests that open in each initial mode, switch without losing state, hide wait/scheduled/depends/parent in recurring mode, validate title/rule/first_due/until, submit canonical values (`2weeks|3months|12months`), show three-date preview, and use Cmd/Ctrl+Enter. Assert normal mode calls `/tasks` and recurring mode calls `/task-series`. Project task tests assert the primary button/shortcut defaults to normal, dropdown and panel “新建” open recurring, the toolbar/panel title is “循环任务 N”, the frequency field remains “循环规则”, and no `ProjectTabs` Series entry exists.

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- task-create-dialog.test.tsx`

Expected: FAIL。

- [ ] **Step 3: 重构统一创建弹窗**

Keep shared title/description/priority/assignees/tags components but separate normal date state from recurring rule/firstDue/until state. Implement `nextRecurrenceDate(from: Date, rule: CanonicalRecurrenceRule): Date` in `recurrence-preview.ts` with local calendar `setDate/setMonth`, never fixed day milliseconds; cross-language fixture tests must match Task 1 for daily/weekly/monthly/N-unit and month-end behavior. After Series create, preserve all task search params, navigate to `/tasks/series/$seriesRef`, open the detail panel and announce “循环任务已创建”. Highlight first occurrence only when it belongs to the current result page; never clear filters to force visibility.

- [ ] **Step 4: 写管理面板与任务上下文失败测试**

List tests cover active default, status/assignee/search/sort, toolbar active-count badge from `total`, zero/loading/error count fallbacks, ended/stopped read-only rows, loading/error/empty/backlog and row menus. Detail tests cover summary, open occurrences, paginated history and management actions. Copy tests require “循环任务”“查看循环任务”“编辑循环设置”“关闭循环任务”“还没有循环任务”, while “循环规则” appears only beside the recurrence selector. Integration tests assert: task rows remain mounted while panel opens; panel replaces and later restores `ProjectContextRail`; toolbar list→detail history returns detail→list→closed, while direct Series detail from a task returns to that source; both close controls use correct label and behavior; close preserves project-task filters/selection/scroll and restores trigger focus; direct deep links fall back to project tasks; copied links omit return state; Series API failure is isolated to panel; direct detail deep link opens tasks + panel; desktop uses side panel and mobile uses full-screen Sheet. My Tasks return-state wiring lands in Task 13 after its local state is migrated to route search.

- [ ] **Step 5: 实现面板壳层、列表与详情**

Register `TaskSeriesPanel` through `ProjectLayout.setContextPanel`. Desktop renders a 440–520px list/detail panel in place of project context; narrow desktop/mobile uses an overlay/full-screen Sheet. Use the Spec information hierarchy; Series rows never expose done/start. Occurrence links use project task detail route and occurrence_ref. Panel list/detail errors and loading never replace the task query result. Closing unregisters the panel, uses validated `panelReturnTo` when present, otherwise navigates to `/tasks`, and preserves existing search params.

- [ ] **Step 6: 实现编辑/停止弹窗**

Edit shows immutable first_due, effective_from selector only when rule changes, three future previews and impact counts. Stop uses distinct copy from task delete/occurrence skip; delete-open count equals open materialized count + backlog and disables when >1000. Ended/stopped series show no edit/stop actions.

- [ ] **Step 7: 运行 Web 定向测试**

Run: `pnpm --dir web test -- task-create-dialog.test.tsx recurrence-preview.test.ts task-series-panel.test.tsx task-series-list.test.tsx task-series-detail.test.tsx task-series-dialog.test.tsx task-series-stop-dialog.test.tsx project-task-toolbar.test.tsx project-tasks-page.test.tsx use-task-mutations.test.tsx`

Run: `pnpm --dir web typecheck`

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add web/src/features/workspace/project-workbench/task-series web/src/features/workspace/project-workbench/tasks web/src/features/workspace/project-workbench/hooks web/src/routes/workspace
git commit -m "feat: 完成任务页循环任务管理面板"
```

### Task 13: 完成 occurrence 列表、详情、我的任务与项目统计体验

**目标：** 普通任务和 occurrence 在所有常用任务场景中一致显示/操作，Series 只通过治理入口出现。

**Files:**

- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-row-actions.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-action-bar.tsx`
- Modify: `web/src/features/workspace/my-tasks/my-task-tabs.ts`
- Modify: `web/src/features/workspace/my-tasks/my-task-tabs.test.ts`
- Modify: `web/src/features/workspace/my-tasks/my-tasks-api.ts`
- Modify: `web/src/features/workspace/my-tasks/my-tasks-api.test.ts`
- Modify: `web/src/features/workspace/my-tasks/my-tasks-table.tsx`
- Modify: `web/src/features/workspace/my-tasks/my-tasks-table.test.tsx`
- Modify: `web/src/pages/my-tasks-page.tsx`
- Create: `web/src/pages/my-tasks-page.test.tsx`
- Modify: `web/src/routes/workspace/MyTasksRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-overview-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/i18n.test.ts`

**Interfaces:**

- Project/My Tasks filters add `task_type=all|normal|occurrence` and finite range sends `occurrence_mode=expand`.
- My Task tabs become `incomplete|today|overdue|noDue|completed`.
- My Tasks preset/filter/sort state moves from component `useState` into validated route search; `PanelReturnTo.restore` restores selected IDs、scroll anchor and focus after data reload.

- [ ] **Step 1: 写列表与详情失败测试**

Test materialized occurrence shows task_slug, projected shows `↻MM-DD`, both show recurrence badge and link by occurrence_ref. Row actions are normal delete vs occurrence skip/“查看循环任务”; the latter navigates to `/tasks/series/$seriesRef` with current task search params and no ProjectTab change. Detail shows the “所属循环任务” banner, read-only rule/slot, editable due, original slot after reschedule, and never displays series as parent. Assert ordinary detail labels are “完成任务/重新打开任务”, occurrence detail labels are “完成本次/重新打开本次”, compact occurrence completion has an aria-label containing the slot date, and no “本次/整个系列” scope dialog appears. Clicking “完成本次” must submit the stable occurrence_ref, keep the canonical URL after projected materialization, render completed state plus “重新打开本次”, and announce a date-specific success Toast. Assert Series-derived count/history queries are invalidated and refetched, but the Series rule and sibling occurrence states do not change. My Tasks tests open a Series detail with `panelReturnTo` and verify close/back restores route search、selected IDs、scroll and focus after data reload.

- [ ] **Step 2: 写 My Tasks 预设失败测试**

Replace tab tests with exact filters:

```ts
expect(tabFilter("today", now)).toEqual({
  query: "(status:pending or status:waiting)",
  due_after: "2026-07-12",
  due_before: "2026-07-12",
  occurrence_mode: "expand",
})
expect(tabFilter("completed", now)).toEqual({ status: "completed", occurrence_mode: "materialized" })
expect(tabFilter("overdue", now)).toEqual({
  query: "(status:pending or status:waiting)",
  due_before: "2026-07-11",
  occurrence_mode: "materialized",
})
```

Use the backend's inclusive date inputs: overdue must use yesterday as `due_before` and materialized mode because unbounded historical expansion is forbidden; today must include both bounds and expand so it cannot include overdue tasks. Incomplete/noDue use the same pending-or-waiting query. noDue tests include a materialized occurrence whose due was explicitly cleared, retain its recurrence_at badge, and exclude projected occurrences. Tests assert `active` is never sent as status; “进行中” is derived from pending + non-null start.

- [ ] **Step 3: 运行测试确认失败**

Run: `pnpm --dir web test -- task-table.test.tsx project-task-toolbar.test.tsx project-tasks-page.test.tsx task-detail-page.test.tsx task-property-panel.test.tsx my-task-tabs.test.ts my-tasks-api.test.ts my-tasks-table.test.tsx my-tasks-page.test.tsx project-overview-page.test.tsx i18n.test.ts`

Expected: FAIL。

- [ ] **Step 4: 实现项目任务体验**

Remove recurring status from filters; add task type. Unbounded project page uses materialized mode; a complete due range uses expand. Keep each occurrence as a separate row; no title-based dedupe and never insert Series definition rows into the task table. Batch delete copy reports normal delete count and occurrence skip count separately. The task Header keeps “循环任务 N” as a secondary management action and the create dropdown as primary; filtering never hides or changes the Series-management entry.

- [ ] **Step 5: 实现 occurrence 详情与写操作**

Use `recurrence_info`, never parent inference. All normal field edits say “仅本次”; remove editable recurrence select. In `task-action-bar.tsx`, ordinary tasks use “完成任务/重新打开任务” while occurrences use “完成本次/重新打开本次”; compact controls retain the full date-specific aria-label/title. Completion calls task done with the current occurrence_ref, never Series APIs and never a scope picker. Keep the detail route mounted, replace the projected cache entry with the returned materialized view under the same public ID, show the completed state/date-specific Toast, and invalidate occurrence、task list、project statistics plus Series-derived count/history queries; do not optimistically mutate the Series rule or sibling occurrence states. “查看循环任务” opens the task-page panel rather than navigating to a sibling page. Projected comments/links/dependencies/subtask/action responses replace cached projected view with materialized view but preserve route. `from=my-tasks` changes back navigation only and is removed from copied canonical URL.

- [ ] **Step 6: 实现 My Tasks 与统计**

Remove duplicate status selector because tabs own status. Add incomplete/today/overdue/no-due/completed; all open presets use pending OR waiting, today uses exact finite range, completed includes occurrences and supports reopen. Move tab、search、project、priority、task_type、sort into `/my-tasks` search validation instead of local-only state. Before opening a Series panel capture selected IDs/scrollTop/focusId; on return restore after query success, ignore missing IDs and focus the table container as fallback. Render ordinary progress and recurring metrics in separate project overview blocks.

- [ ] **Step 7: 验证桌面、移动端与编辑烟测**

Run: `pnpm --dir web test -- task-table.test.tsx project-task-toolbar.test.tsx project-tasks-page.test.tsx task-detail-page.test.tsx task-property-panel.test.tsx my-task-tabs.test.ts my-tasks-api.test.ts my-tasks-table.test.tsx my-tasks-page.test.tsx project-overview-page.test.tsx i18n.test.ts`

Run: `pnpm --dir web run smoke:editing`

Run: `pnpm --dir web typecheck`

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add web/src/features/workspace/project-workbench web/src/features/workspace/my-tasks web/src/pages/my-tasks-page.tsx web/src/pages/my-tasks-page.test.tsx web/src/routes/workspace/MyTasksRoute.tsx web/src/routes/router.tsx web/src/locales web/src/i18n.test.ts
git commit -m "feat: 统一循环实例任务体验"
```

### Task 14: 原生 bundle、旧模型清理、文档与全量验收

**目标：** 完成 `xuanchu.task-bundle/v1` import/export，删除全部旧 recurrence 路径，更新用户文档并执行全量验证。

**Files:**

- Delete: `internal/recurrence/recurrence.go`
- Delete: `internal/recurrence/recurrence_test.go`
- Create: `internal/app/task_bundle.go`
- Create: `internal/app/task_bundle_test.go`
- Modify: `internal/cli/import_export.go`
- Create: `internal/cli/import_export_test.go`
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/httpapi/tasks_test.go`
- Modify: `internal/remote/config.go`
- Modify: `internal/remote/config_test.go`
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `tests/integration/e2e_task_audit_test.go`
- Modify: `web/src/features/workspace/project-workbench/import/task-import.ts`
- Modify: `web/src/features/workspace/project-workbench/import/task-import.test.ts`
- Modify: `web/src/features/workspace/project-workbench/import/task-import-schema.ts`
- Modify: `web/src/features/workspace/project-workbench/import/task-import-schema.test.ts`
- Modify: `web/src/features/workspace/project-workbench/import/task-import-xlsx.ts`
- Modify: `web/src/features/workspace/project-workbench/import/task-import-xlsx.test.ts`
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md` only if implementation reveals a factual correction

**Interfaces:**

- `type TaskBundleV1 struct { Schema string; ExportedAt string; TaskSeries []TaskSeriesBundle; Tasks []TaskBundle }`
- `func (s *Service) ExportTaskBundle() (TaskBundleV1, error)`
- `func (s *Service) ImportTaskBundle(TaskBundleV1) (TaskBundleImportResult, error)`
- `func (c *remote.Client) ExportTaskBundle(ctx context.Context, workspace, project, projectID string) (TaskBundleV1DTO, error)`
- `func (c *remote.Client) ImportTaskBundle(ctx context.Context, workspace string, bundle TaskBundleV1DTO) (TaskBundleImportResultDTO, error)`

- [ ] **Step 1: 写 bundle 失败测试**

Create round-trip tests with one series, two rule versions, one ordinary task, one materialized occurrence, one tombstone, overrides and UserInfo-linked assignees. Assert projected occurrences are absent, IDs and recurrence_at survive, unknown major schema is rejected, and missing/cross-workspace series references roll back the whole import.

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run TaskBundle -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现版本化原生 bundle**

Export `schema="xuanchu.task-bundle/v1"`, current series including complete rule-version history, and ordinary/materialized task rows. Import in one transaction in dependency order: series→rule versions/associations→tasks/occurrences. Never export projected views. Reject Taskwarrior arrays and old recurrence fields rather than guessing.

- [ ] **Step 4: 接入 CLI/HTTP/MCP 并限制 Web 表格导入**

CLI、HTTP、MCP and Remote import/export use the bundle object and the same schema/version errors. Delete Remote `ExportTasks() ([]task.Task, error)` and `ImportTasks([]task.JSONTask)` methods and update CLI remote-mode callers to the new methods without compatibility wrappers. Web JSON/XLSX task import supports ordinary tasks only and removes recurrence help/columns; series migration uses native JSON bundle, not flattened spreadsheets.

- [ ] **Step 5: 扫描并删除旧实现**

Run:

```bash
rg -n 'StatusRecurring|CreateRecurringChild|RecurringParents|createRecurringParent|createNextRecurringChild|ensureRecurringChildren|clear_recur|\bRecur\b|\bMask\b|\bIMask\b|status.?recurring' internal tests web/src --glob '!internal/webconsole/dist/**'
```

Expected: only migration guard text, historical release documentation, deliberate unknown-field/error tests, and human “recurring” UI mode names remain. Delete or rewrite every executable old-path hit. Also run `rg -n 'ListTasks\(|GetTask\(|ExportTasks\(|ImportTasks\(' internal/cli internal/remote tests` and require zero old Remote method declarations/callers; scan `internal/query` and `internal/storage/query_scope.go` to ensure no executable recur/mask/imask attribute remains.

- [ ] **Step 6: 更新 README/ROADMAP/帮助文案**

README stops documenting old `add ... recur:*` and describes `series` commands, native bundle, Web routes and MCP tools. ROADMAP marks v0.5.7 complete only after all validation passes. Verify help/OpenAPI/MCP schema use the same field names.

- [ ] **Step 7: 运行后端全量验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
```

Expected: all commands exit 0; tests report no failures.

- [ ] **Step 8: 运行 Web 全量验证**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

Expected: all commands exit 0. Confirm `internal/webconsole/dist` did not change or remain staged.

- [ ] **Step 9: 运行最终静态检查**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; status contains only the Task 14 source/docs changes intended for the final commit.

- [ ] **Step 10: 提交**

```bash
git add internal/recurrence internal/app/task_bundle.go internal/app/task_bundle_test.go internal/cli/import_export.go internal/cli/import_export_test.go internal/httpapi/import_audit.go internal/httpapi/tasks_test.go internal/remote/config.go internal/remote/config_test.go internal/mcpserver/tools_task.go internal/mcpserver/integration_test.go tests/integration/e2e_task_audit_test.go web/src/features/workspace/project-workbench/import README.md ROADMAP.md docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md
git commit -m "feat: 完成原生循环系列交付"
```

## 最终验收清单

- daily 昨日实例未完成，今天仍独立生成。
- 停机五日后投影立即可见、scheduler 分批补齐且重复运行不重复。
- future range query 是 ordinary + projected + materialized exception - tombstone。
- projected 纯读取不写库；所有合法首次写原子物化；失败 stop/reopen 不物化。
- projected 子资源读返回空集合/计算型 urgency；不存在子资源的 update/delete/remove、no-op 和任何失败写不物化。
- 公开 occurrence ID 在物化、完成、改期、跳过后不变。
- rule 修改有持久化版本段和 effective_from，历史 projected occurrence 不被新规则重算。
- Task.status 无 recurring，Task.parent 不承载 series，普通 Task schema 无 recurrence。
- HTTP、Remote、CLI、MCP 对同一输入返回相同集合、错误码、UserInfo 与 recurrence_info。
- CLI `series occurrences` 提供完整历史分页；list/report、working set、helper 与 renderer 对 projected 的语义稳定。
- external edit 取消、失败或无 diff 不物化；有效 diff 才原子物化并修改。
- series occurrences 的 pending/waiting/completed/deleted/all 在四端 schema 一致。
- Series list 的 status/q/assignee/sort/pagination 在 HTTP/MCP/Remote/CLI/Web items 与 filtered total 一致。
- 查询 DSL 删除 recur/mask/imask 并新增 series_id/recurrence_at/task_type；SQL 和 expand evaluator 等价。
- `/reports/{name}`、`/tasks?report=`、CLI aliases、Remote 与 MCP `report_run` 在 merge 后应用 scope/urgency 并返回同一 TaskViewPage。
- 项目任务、我的未完成/今天/逾期/无截止日期/已完成显示正确，不混入 Series。
- Web 全局侧栏和 ProjectTabs 不增加独立循环任务入口；任务表融合普通任务与 occurrence，Series 定义不作为任务行。
- `/tasks/series[/seriesRef]` 面板深链、右栏替换、任务筛选/滚动/焦点恢复及移动端全屏 Sheet 与 Spec ASCII 原型一致。
- My Tasks open presets 覆盖 pending/waiting，不产生 active status；面板往返恢复 route search、选择、滚动和焦点。
- Web 列表、详情、创建/编辑/停止/跳过弹窗与 Spec ASCII 原型一致。
- 普通任务详情与 occurrence 详情分别使用“完成任务/重新打开任务”和“完成本次/重新打开本次”；完成只提交当前 occurrence_ref，保留 URL，并仅通过查询刷新反映 Series 派生计数。
- 项目关闭停止 active series，恢复项目不恢复 series。
- 普通任务进度与循环运行指标分离。
- Native bundle 能完整 round-trip series/rule versions/materialized occurrence，且不导出 projected occurrence。
- Remote 只暴露 TaskOccurrenceDTO/TaskViewPageDTO 与 native bundle 方法，不残留返回 `task.Task` 的旧 task read/action/import/export wrapper。
- SQLite/PostgreSQL、零 CGO、OpenAPI、MCP schemas、Web 全套验证通过。
