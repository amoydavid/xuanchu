# 任务短引用与循环任务字段对齐 Implementation Plan

> **实施进度（2026-07-13）✅：全部完成。** UUID、task_slug、occurrence_ref 三别名已统一到 App resolver；已物化实例首选 `{projectSlug}-{projectSeq}`；Series 表单字段与普通任务对齐；循环实例详情、Series 面板短链接、来源返回、`series_title` 与 shadcn 操作层级已落地。验证覆盖 SQLite、PostgreSQL、HTTP、MCP、Remote/CLI、桌面/移动 Web E2E、零 CGO、Web 593 项测试、production build 与编辑 smoke。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成短 task_slug 链接、循环实例统一引用语义、项目绑定回填、循环详情页和普通/循环共享创建字段，使 Web、HTTP、MCP、Remote/CLI 对同一实例行为一致。

**Architecture:** App 层成为唯一 task reference resolver：UUID、task_slug、occurrence_ref 都解析为统一 `TaskRefResolution`，协议层不再按字符串前缀猜资源类型。物化 occurrence 保持稳定 `id=occurrence_ref`，同时获得可读 task_slug；Web 对 materialized task 使用 slug permalink，projected 使用 occurrence_ref 并在物化后 replace。Series 与普通任务共享表单组件，但保留各自的时间字段。

**Tech Stack:** Go 1.25、GORM、SQLite/PostgreSQL、Chi、Huma/OpenAPI、MCP Go SDK、Cobra、React 19、TypeScript、TanStack Router/Query、react-i18next、shadcn/Radix、Vitest、Playwright。

## Global Constraints

- 文档、注释和用户可见中文以中文为主。
- SQLite 必须继续使用 `github.com/glebarez/sqlite`，不得引入 CGO；PostgreSQL 使用现有 pgx/GORM driver。
- occurrence_ref 继续是投影和物化前后不变的结构化 `id`；不得为 projected occurrence 提前分配 UUID/project_seq/task_slug。
- materialized occurrence 的用户首选 permalink 是 task_slug；UUID 和 occurrence_ref 永久可解析。
- `project`、`project_id`、`project_seq` 要么全空，要么完整且属于同一 workspace/project。
- 协议层不得通过 `strings.HasPrefix(ref, "occ:")` 决定响应或操作语义。
- 所有用户对象继续使用 `task.UserInfo` / `task.JSONUserInfo`。
- Web 必须复用现有 shadcn/ui、MarkdownEditor、InlineDatePicker 和成员选择器；所有文案进入 zh-CN/en-US i18n。
- 修改 occurrence 共享字段只影响本次并记录 override；删除 occurrence 等价于跳过本次。
- 保留当前工作区已有 shadcn/i18n/规则建议槽位改动，不覆盖、不重置。

---

### Task 1: 建立完整 occurrence 项目绑定和数据回填

**Files:**
- Modify: `internal/app/task_series.go`
- Modify: `internal/app/task_occurrence.go`
- Modify: `internal/app/task_bundle.go`
- Modify: `internal/storage/migration_task_series.go`
- Test: `internal/app/task_occurrence_test.go`
- Test: `internal/app/task_bundle_test.go`
- Test: `internal/storage/db_test.go`
- Test: `internal/storage/postgres_test.go`

**Interfaces:**
- Produces: `func (s *Service) bindOccurrenceProject(tsk *task.Task, workspaceID, projectID string) error`。
- Produces: `func backfillOccurrenceProjectBindings(db *gorm.DB) error`，在 SQLite/PostgreSQL 上幂等运行。
- Invariant: materialized occurrence 和 tombstone 的 `Project/ProjectID/ProjectSeq` 完整。

- [x] **Step 1: 写 App 失败测试**

在 projected 写前物化、Series 首次物化和 stop tombstone 测试中断言：

```go
if tsk.Project == nil || *tsk.Project != "ops" {
    t.Fatalf("Project = %#v, want ops", tsk.Project)
}
if tsk.ProjectID == nil || *tsk.ProjectID != project.ID || tsk.ProjectSeq == nil {
    t.Fatalf("project binding incomplete: %#v", tsk)
}
view, err := svc.GetTaskView(app.OccurrenceRef(series.ID, slot))
if err != nil || view.TaskSlug == nil || *view.TaskSlug != "ops-7" {
    t.Fatalf("view task_slug = %#v err=%v", view.TaskSlug, err)
}
```

- [x] **Step 2: 运行 App 测试确认失败**

Run:

```bash
go test ./internal/app -run 'TestMaterializeOccurrenceForWrite|TestAddTaskSeries|TestStopTaskSeries' -count=1
```

Expected: FAIL，Project 为 nil 或 task_slug 缺失。

- [x] **Step 3: 实现统一绑定 helper**

helper 必须在已分配 project_seq 的同一事务中解析项目并写入：

```go
func (s *Service) bindOccurrenceProject(tsk *domain.Task, workspaceID, projectID string) error {
    project, err := s.projectRepo.GetByID(projectID)
    if err != nil || project.WorkspaceID != workspaceID {
        return RuntimeError{Code: "project_invariant_violation", Message: "occurrence project not found"}
    }
    tsk.Project = cloneStringPtr(&project.Slug)
    tsk.ProjectID = cloneStringPtr(&project.ID)
    if tsk.ProjectSeq == nil {
        seq, err := s.projectRepo.AllocateProjectTaskSeqLocked(workspaceID, projectID)
        if err != nil { return err }
        tsk.ProjectSeq = &seq
    }
    return s.validateTaskProjectInvariant(*tsk)
}
```

所有首个实例、写前物化、tombstone 和 bundle 导入路径复用该 helper，不再手写部分绑定。

- [x] **Step 4: 写 migration 失败测试**

覆盖：`project=NULL + valid project_id/project_seq` 回填 slug、完整绑定幂等、孤儿 project_id 失败、跨 workspace project_id 失败。

- [x] **Step 5: 实现方言无关回填**

在 `prepareTaskSeriesSchema` 建索引前运行事务化回填。PostgreSQL 使用 `UPDATE ... FROM`，SQLite 使用 correlated subquery；join 同时包含 `workspace_id`。回填后扫描部分绑定和 slug 不匹配行，非零即返回明确错误。

- [x] **Step 6: 验证并提交**

```bash
go test ./internal/storage ./internal/app -count=1
CGO_ENABLED=0 go test ./internal/storage ./internal/app -count=1
git add internal/app/task_series.go internal/app/task_occurrence.go internal/app/task_bundle.go internal/app/task_occurrence_test.go internal/app/task_bundle_test.go internal/storage/migration_task_series.go internal/storage/db_test.go internal/storage/postgres_test.go
git commit -m "fix: 完整绑定循环实例项目"
```

### Task 2: 建立统一 task reference resolver 和实例 override 语义

**Files:**
- Create: `internal/app/task_reference.go`
- Create: `internal/app/task_reference_test.go`
- Modify: `internal/app/task_occurrence.go`
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/workspace.go`
- Modify: `internal/app/audit.go`
- Test: `internal/app/task_occurrence_test.go`
- Test: `internal/app/service_test.go`
- Test: `internal/app/audit_test.go`

**Interfaces:**

```go
type TaskResourceKind string
const TaskResourceNormal TaskResourceKind = "normal"
const TaskResourceOccurrence TaskResourceKind = "occurrence"

type TaskRefResolution struct {
    Kind            TaskResourceKind
    StableID        string
    UUID            *string
    TaskSlug        *string
    OccurrenceRef   *string
    Materialization string
    Task            *task.Task
    View            TaskOccurrenceView
}

func (s *Service) ResolveTaskReferenceForRead(ref string) (TaskRefResolution, error)
func (s *Service) ResolveExistingTaskReference(ref string, write bool) (TaskRefResolution, error)
```

- [x] **Step 1: 写三别名等价失败测试**

创建 `ops-7` occurrence 后，对 UUID、task_slug、occurrence_ref 逐一断言：

```go
for _, ref := range []string{occ.UUID, "ops-7", OccurrenceRef(series.ID, slot)} {
    got, err := svc.ResolveTaskReferenceForRead(ref)
    if err != nil { t.Fatal(err) }
    if got.Kind != TaskResourceOccurrence || got.StableID != OccurrenceRef(series.ID, slot) {
        t.Fatalf("resolution(%q) = %#v", ref, got)
    }
    if got.View.RecurrenceInfo == nil || got.View.TaskSlug == nil { t.Fatal("missing recurrence view") }
}
```

- [x] **Step 2: 写 override 和 delete 失败测试**

通过三个别名分别修改 title/description/priority/due/assignees/tags/UDA，断言 `RecurrenceOverrides` 包含实际变化字段；通过 `ops-7` 删除时 audit action 为 `task.recurrence.skipped`，payload 含 series_id/recurrence_at。

- [x] **Step 3: 实现 resolver**

解析顺序：严格 occurrence_ref -> UUID -> task_slug。UUID/task_slug 命中 task 后根据循环持久字段派生 Kind 和稳定 occurrence_ref。projected 只允许 occurrence_ref 且只构造 View。

- [x] **Step 4: 实现 override 记录**

```go
if tsk.SeriesID != nil {
    tsk.RecurrenceOverrides = domain.NormalizeRecurrenceOverrides(
        append(tsk.RecurrenceOverrides, changedOccurrenceFields(before, tsk, input)...),
    )
}
```

UDA 使用现有 canonical key `udas`；no-op 不新增 override。

- [x] **Step 5: 统一删除和子资源语义**

- occurrence `Delete` 写 `task.recurrence.skipped`；普通任务仍写 `task.delete`。
- projected audit/children/links/annotations 返回空且不物化；urgency 从 TaskOccurrenceView 即时计算。
- denotate/link update/remove 等已有子资源写先拒绝 projected，不能先物化。
- 新增 annotation/link/dependency/child 在完整校验后按需物化。

- [x] **Step 6: 验证并提交**

```bash
go test ./internal/app -run 'TaskRef|Occurrence|Override|Audit|Children|Urgency|Link|Annotation' -count=1
git add internal/app
git commit -m "feat: 统一任务引用与实例语义"
```

### Task 3: 修复 HTTP path decode 并统一 task view 响应

**Files:**
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/task_series.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/error_status.go`
- Test: `internal/httpapi/tasks_test.go`
- Test: `internal/httpapi/task_series_test.go`

**Interfaces:**
- Produces: `func decodedPathParam(r *http.Request, name string) (string, error)`。
- 所有 task read/write 响应使用 TaskOccurrenceView serializer；Kind 来自 App resolver。

- [x] **Step 1: 写 encoded ref 失败测试**

```go
encoded := url.PathEscape(app.OccurrenceRef(series.ID, slot))
rr := requestHTTP(t, server, http.MethodGet, "/api/v1/tasks/"+encoded, headers)
if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
```

同时覆盖 `%`、`%253A`、`%2F` 返回 400，projected GET 前后 task row count 不变。

- [x] **Step 2: 实现单次解码和严格校验**

```go
func decodedPathParam(r *http.Request, name string) (string, error) {
    raw := strings.TrimSpace(chi.URLParam(r, name))
    value, err := url.PathUnescape(raw)
    if err != nil || strings.ContainsAny(value, "/\x00\r\n") {
        return "", app.RuntimeError{Code: "task_ref_invalid", Message: "invalid path parameter"}
    }
    return value, nil
}
```

不进行第二次 unescape；task、series occurrence 子路由共用。

- [x] **Step 3: 消除按原始 ref 分支**

`handleTaskInfo`、`writeTaskAfterMutation` 始终调用 `GetTaskView` 并输出统一 view。所有 action 通过 App 资源 Kind 工作；task_slug occurrence 返回 recurrence_info。

- [x] **Step 4: 更新 Huma/OpenAPI**

所有 `{taskRef}` 描述改为“UUID、materialized task_slug 或 occurrence_ref；projected 仅 occurrence_ref”，保持数字 ref 拒绝说明。

- [x] **Step 5: 验证并提交**

```bash
go test ./internal/httpapi -run 'TaskHTTP|Occurrence|TaskInfo|TaskLink|TaskActivity|TaskUrgency' -count=1
git add internal/httpapi
git commit -m "fix: 统一 HTTP 循环实例引用"
```

### Task 4: 统一 MCP、Remote 和 CLI 的实例响应

**Files:**
- Modify: `internal/mcpserver/tools_common.go`
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/tools_task_series.go`
- Modify: `internal/mcpserver/schema_test.go`
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `internal/remote/task_series.go`
- Modify: `internal/remote/task.go`
- Modify: `internal/remote/task_test.go`
- Modify: `internal/cli/info.go`
- Modify: `internal/cli/list.go`
- Modify: `internal/cli/series.go`
- Test: `internal/cli/series_test.go`
- Test: `tests/integration/cli_test.go`

**Interfaces:**
- MCP task tools 对 materialized occurrence 的任意别名返回相同 occurrence map。
- Remote `TaskOccurrenceDTO` 是统一响应 DTO。
- CLI human renderer 对 materialized 显示 SLUG，对 projected 显示 `↻MM-DD`。

- [x] **Step 1: 写 MCP 三别名失败测试**

对 `task_get` 和写 tool 循环 UUID/task_slug/occurrence_ref，断言：

```go
if got["id"] != occurrenceRef || got["task_slug"] != "ops-7" || got["recurrence_info"] == nil {
    t.Fatalf("unexpected occurrence payload: %#v", got)
}
```

- [x] **Step 2: MCP 始终使用 App view**

删除 `IsOccurrenceRef(in.ID)` 响应分支；`task_get`、`taskAfterMutation` 调用 `GetTaskView`。更新 tool schema description。

- [x] **Step 3: 调整 Remote DTO**

优先解码 `TaskOccurrenceDTO`；只给尚未统一的 create/import 响应保留 JSONTask fallback，不用输入前缀决定 occurrence Kind。校验 task_slug/project 一致性。

- [x] **Step 4: CLI 显示与操作测试**

覆盖 `xuanchu ops-7 info/done/delete` 和 projected human 行：materialized 使用 SLUG；projected ID/SLUG 为 `-`，显示 `↻MM-DD` 与 occurrence_ref 操作提示。

- [x] **Step 5: 验证并提交**

```bash
go test ./internal/mcpserver ./internal/remote ./internal/cli ./tests/integration -count=1
git add internal/mcpserver internal/remote internal/cli tests/integration/cli_test.go
git commit -m "feat: 统一 MCP 与 CLI 实例引用"
```

### Task 5: Web 列表短链接和详情 URL 规范化

**Files:**
- Create: `web/src/features/workspace/project-workbench/tasks/task-reference.ts`
- Create: `web/src/features/workspace/project-workbench/tasks/task-reference.test.ts`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`
- Modify: `web/src/features/workspace/my-tasks/my-tasks-table.tsx`
- Modify: `web/src/features/workspace/my-tasks/my-tasks-table.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`

**Interfaces:**

```ts
export function taskDisplayRef(task: ProjectWorkbenchTask, locale: string): string
export function taskRouteRef(task: ProjectWorkbenchTask): string
export function canonicalTaskRouteRef(task: ProjectWorkbenchTask): string | null
```

- [x] **Step 1: 写列表失败测试**

materialized occurrence 断言文本和 href 都是 `ops-7`；projected 断言显示 `↻07-19`、href 使用 occurrence_ref，DOM 不显示随机串。

- [x] **Step 2: 拆分 display/route helper**

`taskRouteRef`: materialized + slug -> slug；projected -> stable id；ordinary fallback -> UUID。禁止继续用一个 fallback helper 同时承担显示和路由。

- [x] **Step 3: 写 alias replace 失败测试**

详情 ref 为 occurrence_ref/UUID，mock GET 返回 `task_slug=ops-7`，断言 TanStack Router `replace:true`；projected `task_slug=null` 不导航。

- [x] **Step 4: 实现 cache alias 归并**

获取详情后用稳定 `task.id` 写 canonical cache，并同步原 ref/slug；mutation 成功按响应 `id/task_slug` 失效，避免三份过期缓存。

- [x] **Step 5: 验证并提交**

```bash
pnpm --dir web test src/features/workspace/project-workbench/tasks/task-reference.test.ts src/features/workspace/project-workbench/tasks/task-table.test.tsx src/features/workspace/my-tasks/my-tasks-table.test.tsx src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx
pnpm --dir web typecheck
git add web/src/features/workspace/project-workbench web/src/features/workspace/my-tasks
git commit -m "feat: 使用任务短链接导航实例"
```

### Task 6: 实现循环实例详情页完整状态

**Files:**
- Create: `web/src/features/workspace/project-workbench/task-detail/recurrence-context-alert.tsx`
- Create: `web/src/features/workspace/project-workbench/task-detail/recurrence-context-alert.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-action-bar.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- `RecurrenceContextAlert({task, projectSlug, workspaceSlug})` 渲染 materialized/projected/ended/stopped 上下文。
- `TaskPropertyPanel` 对 occurrence 显示只读 recurrence_at；due override 后两者并存。
- `TaskActionBar` 只读 recurrence_info 决定“本次”文案和 skip 行为。

- [x] **Step 1: 写 9.2 原型失败测试**

覆盖 materialized pending、projected、completed、deleted、Series stopped：

```text
materialized -> OPS-7、循环 Alert、本次日期、完成本次、跳过本次
projected -> 计划实例、↻MM-DD、无 slug/创建时间/伪活动
completed -> 重新打开本次、属性只读
deleted -> 已跳过、无执行动作
stopped series + pending occurrence -> 归属已停止但仍可完成本次
```

- [x] **Step 2: 实现详情信息层级**

保留主栏 + 280px 属性栏和移动四 Tabs。Alert 位于 header 后、正文前；Series 管理链接保留 return state。标题、描述和属性编辑都提交当前 ref。

- [x] **Step 3: 实现动作和确认文案**

materialized slug 调用 `DELETE /tasks/ops-7`；projected 使用 occurrence_ref。确认文案包含本次日期；成功返回来源列表并 Toast。全部进入 i18n。

- [x] **Step 4: 验证并提交**

```bash
pnpm --dir web test src/features/workspace/project-workbench/task-detail
pnpm --dir web typecheck
pnpm --dir web lint
git add web/src/features/workspace/project-workbench/task-detail web/src/locales
git commit -m "feat: 完善循环实例详情页"
```

### Task 7: 统一普通任务与循环任务公共创建字段

**Files:**
- Create: `web/src/features/workspace/project-workbench/tasks/task-common-fields.tsx`
- Create: `web/src/features/workspace/project-workbench/tasks/task-common-fields.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-form.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-series/task-series-dialog.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/api/task-series-api.ts`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**

```ts
export type TaskCommonFieldValue = {
  title: string
  description: string
  priority: string
  assignees: string[]
  tags: string
  udas: Record<string, string>
}
```

- [x] **Step 1: 写公共字段和 payload 失败测试**

普通/循环切换后断言 title、Markdown description、负责人、优先级、标签保留；循环提交包含：

```ts
expect(createTaskSeries).toHaveBeenCalledWith("local", expect.objectContaining({
  title: "每日巡检",
  description: "检查预算",
  assignees: ["user-1"],
  priority: "H",
  tags: ["ops"],
  udas: { effort: "1h" },
}))
```

- [x] **Step 2: 提取共享组件**

复用 `MarkdownEditor`、成员 Popover/Checkbox/Badge、priority Select、tags Input。UDA 区按现有 schema/API 渲染；无定义时不显示空高级区。

- [x] **Step 3: 实现状态映射**

common fields 一份 state；normal due 和 recurring first_due 各自保留。首次切换且目标为空时复制日期，后续不覆盖。普通 until 文案为“失效日期”，Series until 为“循环结束日期”。

- [x] **Step 4: 完善编辑 Series**

初始化 description/assignees/UDAs，支持清空并生成 `clear:["description","assignees","uda.<name>"]`；创建和编辑复用 common fields。

- [x] **Step 5: 验证并提交**

```bash
pnpm --dir web test src/features/workspace/project-workbench/tasks/task-common-fields.test.tsx src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx src/features/workspace/project-workbench/task-series/task-series-dialog.test.tsx
pnpm --dir web typecheck
pnpm --dir web lint
git add web/src/features/workspace/project-workbench/tasks web/src/features/workspace/project-workbench/task-series web/src/locales
git commit -m "feat: 对齐普通与循环任务字段"
```

### Task 8: 同步主 spec、OpenAPI、手册和 Agent Skills

**Files:**
- Modify: `docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md`
- Modify: `docs/superpowers/plans/2026-07-13-task-reference-and-recurring-fields-implementation.md`
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/tasks.md`
- Modify: `docs/manual/remote-cli-and-api.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/manual/web-console.md`
- Modify: `docs/skills/xuanchu-capture-and-track-work/SKILL.md`
- Modify: `docs/skills/xuanchu-mcp-base/SKILL.md`
- Modify: generated MCP/OpenAPI golden files under `internal/mcpserver/testdata/` as required

**Interfaces:**
- 稳定 ID=occurrence_ref；materialized 首选用户 permalink=task_slug；projected 仅 occurrence_ref。
- 循环创建字段与 Web 详情页描述和实际实现一致。

- [x] **Step 1: 搜索冲突旧语义**

```bash
rg -n '物化后 URL 不变|canonical URL|UUID 或 `task_slug`|UUID or task_slug|occurrence_ref|task_series_add' README.md ROADMAP.md docs internal/mcpserver/testdata
```

- [x] **Step 2: 同步文档和 schema**

修正主 spec 被补充 spec 覆盖的条款；更新 HTTP/MCP/CLI 示例；Skill 明确 materialized 优先 task_slug、projected 使用 occurrence_ref，创建 Series 可传 description/assignees/tags/priority/udas。

- [x] **Step 3: 验证文档与 golden**

```bash
go test ./internal/mcpserver -run 'Schema|Golden|Tool' -count=1
git diff --check
! rg -n '物化后 URL 不变|物化后.*occurrence_ref.*permalink' README.md ROADMAP.md docs/manual docs/skills
```

- [x] **Step 4: 提交**

```bash
git add README.md ROADMAP.md docs internal/mcpserver/testdata
git commit -m "docs: 更新循环实例短引用说明"
```

### Task 9: 全量验证、浏览器 E2E 与完成审计

**Files:**
- Modify: `docs/superpowers/plans/2026-07-13-task-reference-and-recurring-fields-implementation.md`
- Test: `internal/app/task_reference_test.go`
- Test: `internal/httpapi/tasks_test.go`
- Test: `internal/mcpserver/integration_test.go`
- Test: `tests/integration/cli_test.go`
- Test: `web/src/features/workspace/project-workbench/tasks/task-reference.test.ts`
- Test: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Test: `web/src/features/workspace/project-workbench/tasks/task-create-dialog.test.tsx`

**Interfaces:**
- Produces: spec §15 每个验收项的当前证据。

- [x] **Step 1: Go 全量质量门**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
```

- [x] **Step 2: Web 全量质量门**

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

- [x] **Step 3: PostgreSQL/SQLite 与 HTTP E2E**

验证：创建含 description/assignee 的 daily Series；列表显示 ops-N；slug 与 encoded occurrence_ref GET 相同；slug 修改产生 override；slug done/reopen/delete 保持本次语义；projected GET 不写库且首次 done 返回 slug；重启迁移回填旧 NULL project occurrence。

- [x] **Step 4: 浏览器桌面与移动验收**

按 spec §9.2 和 §10 验证列表短链接、materialized/projected/completed/deleted 详情、Series stopped banner、URL replace、统一创建弹窗 description/负责人、移动 Tabs 和无横向溢出。

- [x] **Step 5: 完成逐项审计**

逐条检查补充 spec §15.1-15.6。任何缺少直接测试、运行时输出或代码证据的条目保持未完成并继续修复。

- [x] **Step 6: 最终 diff 和提交**

```bash
git diff --check
git status --short
git diff --stat
git add internal web/src tests/integration/cli_test.go README.md ROADMAP.md docs
git commit -m "feat: 完成循环任务短引用与字段对齐"
```
