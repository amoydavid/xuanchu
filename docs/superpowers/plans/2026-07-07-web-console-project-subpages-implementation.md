# Web Console 项目子页面实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把项目详情从单一工作台拆成「概览 / 任务 / 活动」三个项目子页面，并保留一致的项目 Header 与可开合右侧项目信息栏。

**Architecture:** 后端新增 ProjectSummary 作为 Overview 与右栏全量任务统计的权威来源；前端先抽共享 ProjectLayout，再把当前工作台任务能力迁移到 Tasks 页，并新增 Overview 与 Activity 页。项目附属信息继续复用 effective config API，只显示 label/value，不把它包装成“运行配置”。

**Tech Stack:** Go 1.25、GORM、`github.com/glebarez/sqlite`、PostgreSQL driver、React 19、TanStack Router、TanStack Query、shadcn/radix UI、lucide-react、Vitest、Testing Library。

## Global Constraints

- 文档、注释、界面文案以中文为主；中文里称呼 Xuanchu 时使用「璇础」。
- 不展示当前 schema 没有的数据：项目负责人、项目成员、里程碑、Slack/IM 频道、客户请求都不在本次范围内。
- 页面上每个数字、姓名、状态、列表项必须追溯到明确接口字段或前端派生规则；没有数据来源的内容不展示。
- Overview 的「当前重点」必须展示项目整体情况，不能用当前任务列表、筛选结果、分页结果冒充全量统计。
- ProjectSummary 同时要求 `project:read` 与 `task:read`，并遵守 workspace/project scope；无任务读取权限时不能泄露任务数量或任务标识。
- ProjectSummary 的时间判断复用璇础任务日期边界：date-only `due/until` 是本地日末，date-only `wait/scheduled` 是本地日初；前端不得用浏览器时间重新计算 Overview 风险数。
- Tasks 页第一阶段只显示简单任务列表，不做分组、父子树、看板或泳道。
- 项目右侧栏默认打开，允许收起；展开/收起使用图标按钮，例如 `PanelRightCloseIcon` / `PanelRightOpenIcon`，按钮必须有 `aria-label` 和 `title`。
- 共享 Header 只放复制、状态、设置等项目级动作；新建任务和导入任务只放在 Tasks 页主动作区。
- 项目附属信息直接显示 `definition.label || key` 与 effective `value`；`source` 只作为次要提示，不把区块命名为「运行配置」；secret 值只显示后端脱敏值或“已设置”。
- 无 `audit:read` 时 Activity 不展示审计筛选项，也不请求 audit 端点。
- Activity 列表以 project timeline 为主；不能把 annotations 列表再拼到 timeline 上造成项目更新重复。
- 窄屏下右侧栏默认收起或变成不挤压主内容的展示形态，任务表格文字不能重叠。
- archived/cancelled 项目允许浏览 Overview、Tasks、Activity，但隐藏任务创建、导入、项目更新写入入口。
- Go 改动继续满足零 CGO：不能引入 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`。

---

**规格来源：** `docs/superpowers/specs/2026-07-07-web-console-project-subpages-design.md`

**口径校准：**

- 规格第 4 节有「第一阶段前端组合现有端点」的旧表述，但第 6、7、11、12 节和用户修正都要求 Overview/右栏使用项目全量摘要。本实现计划以新增 `GET /api/v1/projects/{projectRef}/task-summary` 为准。
- `settings/notes` 不再作为主入口；实现中保留兼容路由并重定向到项目 Activity。

## File Structure

后端文件：

- Modify: `internal/storage/project_repo.go` - 增加项目任务摘要聚合查询，所有计数基于项目全量任务。
- Modify: `internal/storage/project_repo_test.go` - 覆盖逾期、高优、等待已到期、未分配任务、负责人负载聚合。
- Modify: `internal/app/project.go` - 增加 `ProjectTaskSummaryView`、`ProjectTaskSummary()`，负责权限、项目解析、用户信息补全。
- Modify: `internal/app/service_test.go` - 覆盖 app 层权限、project scope、显示名 fallback。
- Modify: `internal/httpapi/projects.go` - 增加 task-summary response DTO 与 handler。
- Modify: `internal/httpapi/projects_test.go` - 覆盖 HTTP 契约、workspace query、权限错误。
- Modify: `internal/httpapi/huma_routes.go` - 注册 `/api/v1/projects/{projectRef}/task-summary`。
- Modify: `internal/storage/project_annotation_repo.go` - 将 project timeline 中 project entry 的 `source_id` 改为 annotation id。
- Modify: `internal/storage/project_annotation_repo_test.go` - 覆盖 timeline project entry 可用于 Activity 去重和删除。

前端文件：

- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts` - 增加 `ProjectTaskSummary` 类型、path builder、fetcher。
- Modify: `web/src/features/workspace/project-workbench/api/project-api.test.ts` - 覆盖 task-summary path 和类型约定。
- Modify: `web/src/features/workspace/project-workbench/hooks/use-project-data.ts` - 增加 summary query key 与 hook。
- Modify: `web/src/features/workspace/project-workbench/permissions/permissions.ts` - 增加 `canTaskRead()`、`canAuditRead()`。
- Create: `web/src/features/workspace/project-workbench/project/project-layout.tsx` - 共享项目布局、Header、tabs、右栏开合状态。
- Create: `web/src/features/workspace/project-workbench/project/project-tabs.tsx` - 由 pathname 显示「概览 / 任务 / 活动」。
- Create: `web/src/features/workspace/project-workbench/project/project-context-rail.tsx` - 右侧项目信息栏，使用图标按钮展开/收起。
- Create: `web/src/features/workspace/project-workbench/project/project-overview-page.tsx` - Overview 主体。
- Create: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.tsx` - Tasks 主体，承接当前任务工具栏与表格。
- Create: `web/src/features/workspace/project-workbench/activity/project-activity-page.tsx` - Activity 主体。
- Create: `web/src/features/workspace/project-workbench/activity/project-activity-timeline.tsx` - 统一渲染项目更新、任务注解、审计。
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx` - 改成 Overview route wrapper，保留 `ProjectWorkbenchPage` 导出名。
- Keep: `web/src/pages/project-notes-tab.tsx` - 兼容入口重定向后该页面不再作为主入口，本计划不修改它。
- Modify: `web/src/routes/router.tsx` - 新增 `/tasks`、`/activity` 子路由，保留 task detail 路由。
- Modify: `web/src/routes/workspace/ProjectWorkbenchRoute.tsx` - 渲染 Overview。
- Create: `web/src/routes/workspace/ProjectTasksRoute.tsx` - 渲染 Tasks。
- Create: `web/src/routes/workspace/ProjectActivityRoute.tsx` - 渲染 Activity。
- Modify: `web/src/routes/workspace/ProjectSettingsNotesRoute.tsx` - 重定向到 Activity。
- Modify: `web/src/locales/zh-CN.ts`、`web/src/locales/en-US.ts` - 增加项目子页面文案。

测试文件：

- Create: `web/src/features/workspace/project-workbench/project/project-layout.test.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx`
- Create: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx`
- Create: `web/src/features/workspace/project-workbench/activity/project-activity-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.test.tsx`
- Modify: `web/src/routes/workspace/WorkspaceRootRoute.test.tsx` - 覆盖 notes 重定向。

## Shared Interfaces

后端 storage 输出：

```go
type ProjectTaskRefSummary struct {
	UUID     string
	TaskSlug string
	Title    string
}

type ProjectAssigneeWorkloadRow struct {
	UserID    string
	Name      string
	DisplayName string
	Email     *string
	OpenCount int
	OverdueCount int
	HighPriorityCount int
}

type ProjectTaskSummary struct {
	OverdueCount          int
	OverdueRefs           []ProjectTaskRefSummary
	HighPriorityOpenCount int
	HighPriorityOpenRefs  []ProjectTaskRefSummary
	WaitReadyCount        int
	WaitReadyRefs         []ProjectTaskRefSummary
	UnassignedOpenCount   int
	UnassignedOpenRefs    []ProjectTaskRefSummary
	Workload              []ProjectAssigneeWorkloadRow
}
```

HTTP JSON 输出：

```json
{
  "overdue_count": 2,
  "overdue_refs": [{"uuid": "...", "task_slug": "DEM-7", "title": "..." }],
  "high_priority_open_count": 3,
  "high_priority_open_refs": [],
  "wait_ready_count": 5,
  "wait_ready_refs": [],
  "unassigned_open_count": 4,
  "unassigned_open_refs": [],
  "workload": [
    {
      "user": {"id": "u1", "name": "zhangsan", "email": null, "external_ids": []},
      "label": "张三",
      "open_count": 8,
      "overdue_count": 1,
      "high_priority_count": 1
    }
  ]
}
```

前端类型：

```ts
export type ProjectSummaryTaskRef = {
  uuid: string
  task_slug?: string
  title: string
}

export type ProjectSummaryWorkloadRow = {
  user?: UserInfo | null
  label: string
  open_count: number
  overdue_count: number
  high_priority_count: number
}

export type ProjectTaskSummary = {
  overdue_count: number
  overdue_refs: ProjectSummaryTaskRef[]
  high_priority_open_count: number
  high_priority_open_refs: ProjectSummaryTaskRef[]
  wait_ready_count: number
  wait_ready_refs: ProjectSummaryTaskRef[]
  unassigned_open_count: number
  unassigned_open_refs: ProjectSummaryTaskRef[]
  workload: ProjectSummaryWorkloadRow[]
}
```

UI 命名约定：`wait_ready_*` 在中文里显示为「等待已到期」；`unassigned_open_*` 显示为「未分配任务」。不要在 Overview 里使用「等待解除」或「缺负责人」。

## ASCII 实现骨架

右栏打开：

```text
+------------------------------------------------------------------------------+
| workspace / project                                [复制] [状态] [设置图标]   |
+------------------------------------------------------------------------------+
| [概览] [任务] [活动]                                      [收起右栏图标]      |
+-----------------------------------------------------+------------------------+
| 子页面主体                                            | 项目信息               |
|                                                     | 状态 / 任务 / 创建更新 |
|                                                     | 附属信息 label/value   |
|                                                     | 负责人负载             |
|                                                     | 最近活动               |
+-----------------------------------------------------+------------------------+
```

右栏收起：

```text
+------------------------------------------------------------------------------+
| workspace / project                                [复制] [状态] [设置图标]   |
+------------------------------------------------------------------------------+
| [概览] [任务] [活动]                                      [展开右栏图标]      |
+------------------------------------------------------------------------------+
| 子页面主体，占满主要宽度                                                       |
+------------------------------------------------------------------------------+
```

图标按钮要求：

```tsx
<Button
  aria-label={railOpen ? t("projectSubpages.railCollapse") : t("projectSubpages.railExpand")}
  title={railOpen ? t("projectSubpages.railCollapse") : t("projectSubpages.railExpand")}
  size="icon"
  type="button"
  variant="ghost"
  onClick={() => setRailOpen((value) => !value)}
>
  {railOpen ? <PanelRightCloseIcon className="h-4 w-4" /> : <PanelRightOpenIcon className="h-4 w-4" />}
</Button>
```

## Chunk 1: 后端 ProjectSummary API

### Task 1: Storage 聚合项目全量任务摘要

**Files:**

- Modify: `internal/storage/project_repo.go`
- Modify: `internal/storage/project_repo_test.go`

**Interfaces:**

- Produces: `func (r *ProjectRepository) TaskSummary(workspaceID, projectID string, now int64) (ProjectTaskSummary, error)`
- Consumes: `storage.Task`、`storage.TaskAssignee`、`storage.User`

- [ ] **Step 1: 写 storage 失败测试**

在 `internal/storage/project_repo_test.go` 新增 `TestProjectRepositoryTaskSummary`。测试数据必须包含：

```go
now := int64(1_800_000_000)
projectID := "project-summary"
otherProjectID := "project-other"
duePast := now - 3600
dueFuture := now + 3600
waitReady := now

tasks := []Task{
	{UUID: "overdue-h", WorkspaceID: ws, ProjectID: &projectID, Status: "pending", Title: "逾期高优", Due: &duePast, Priority: ptr("H"), Entry: now - 5, Modified: now - 5},
	{UUID: "wait-ready", WorkspaceID: ws, ProjectID: &projectID, Status: "waiting", Title: "等待已到期", Wait: &waitReady, Priority: ptr("M"), Entry: now - 4, Modified: now - 4},
	{UUID: "future", WorkspaceID: ws, ProjectID: &projectID, Status: "pending", Title: "未来任务", Due: &dueFuture, Entry: now - 3, Modified: now - 3},
	{UUID: "done-overdue", WorkspaceID: ws, ProjectID: &projectID, Status: "completed", Title: "已完成逾期", Due: &duePast, Priority: ptr("H"), Entry: now - 2, Modified: now - 2},
	{UUID: "deleted-overdue", WorkspaceID: ws, ProjectID: &projectID, Status: "deleted", Title: "已删除逾期", Due: &duePast, Entry: now - 1, Modified: now - 1},
	{UUID: "other-overdue", WorkspaceID: ws, ProjectID: &otherProjectID, Status: "pending", Title: "其他项目", Due: &duePast, Entry: now, Modified: now},
}
```

断言：

```go
summary, err := repo.TaskSummary(ws, projectID, now)
if err != nil {
	t.Fatalf("TaskSummary() error = %v", err)
}
if summary.OverdueCount != 1 {
	t.Fatalf("OverdueCount = %d, want 1", summary.OverdueCount)
}
if summary.HighPriorityOpenCount != 1 {
	t.Fatalf("HighPriorityOpenCount = %d, want 1", summary.HighPriorityOpenCount)
}
if summary.WaitReadyCount != 1 {
	t.Fatalf("WaitReadyCount = %d, want 1", summary.WaitReadyCount)
}
if summary.UnassignedOpenCount != 2 {
	t.Fatalf("UnassignedOpenCount = %d, want 2", summary.UnassignedOpenCount)
}
```

负责人负载断言：

```go
if got := workloadByLabel(summary.Workload, "张三").OpenCount; got != 2 {
	t.Fatalf("张三 OpenCount = %d, want 2", got)
}
if got := workloadByLabel(summary.Workload, "未分配任务").OpenCount; got != 2 {
	t.Fatalf("未分配任务 OpenCount = %d, want 2", got)
}
```

补充边界断言：`wait == now` 必须计入 `WaitReadyCount`，`due == now` 不计入 `OverdueCount`。这些断言保证 ProjectSummary 使用落库后的 Unix 时间和注入时钟，不在前端按浏览器时区二次判断。

Run:

```bash
go test ./internal/storage -run TestProjectRepositoryTaskSummary -count=1
```

Expected: FAIL，`TaskSummary` 未定义。

- [ ] **Step 2: 增加 storage 类型**

在 `internal/storage/project_repo.go` 增加本计划 `Shared Interfaces` 中的 `ProjectTaskRefSummary`、`ProjectAssigneeWorkloadRow`、`ProjectTaskSummary`。

- [ ] **Step 3: 实现任务引用查询 helper**

在 `internal/storage/project_repo.go` 增加私有 helper：

```go
func (r *ProjectRepository) taskSummaryRefs(workspaceID, projectID string, now int64, kind string) ([]ProjectTaskRefSummary, int, error)
```

`kind` 只允许四个内部常量：`overdue`、`high_priority_open`、`wait_ready`、`unassigned_open`。每个查询都必须包含：

```go
Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
Where("status NOT IN ?", []string{domain.StatusCompleted, domain.StatusDeleted})
```

四类条件：

```go
// overdue
Where("due IS NOT NULL AND due < ?", now)

// high_priority_open
Where("priority = ?", "H")

// wait_ready
Where("wait IS NOT NULL AND wait <= ?", now)

// unassigned_open
Where("NOT EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid)")
```

计数查询返回全量 count；refs 查询 `Order("due ASC, entry ASC")` 或 `Order("entry DESC")`，只取 `Limit(3)`。

- [ ] **Step 4: 实现负责人负载聚合**

在 `TaskSummary` 内用两段查询：

1. assignee 负载：`tasks` join `task_assignees` left join `users`，按 `user_id` 聚合 open/overdue/high。
2. 未分配负载：`NOT EXISTS task_assignees` 聚合成 `UserID == ""`、`DisplayName == "未分配任务"`。

显示名规则在 storage 保留原始字段，最终 label 由 app 层生成：`display_name -> name -> email -> user_id`。

- [ ] **Step 5: 跑 storage 测试**

Run:

```bash
go test ./internal/storage -run TestProjectRepositoryTaskSummary -count=1
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/storage/project_repo.go internal/storage/project_repo_test.go
git commit -m "feat: 增加项目任务摘要聚合"
```

### Task 2: App 层 ProjectTaskSummaryView

**Files:**

- Modify: `internal/app/project.go`
- Modify: `internal/app/service_test.go`

**Interfaces:**

- Consumes: `storage.ProjectRepository.TaskSummary(workspaceID, projectID, now)`
- Produces: `func (s *Service) ProjectTaskSummary(ref string) (ProjectTaskSummaryView, error)`

- [ ] **Step 1: 写 app 失败测试**

在 `internal/app/service_test.go` 新增 `TestServiceProjectTaskSummaryRequiresProjectAndTaskReadAndUsesWholeProject`：

```go
view, err := svc.ProjectTaskSummary("ops")
if err != nil {
	t.Fatalf("ProjectTaskSummary() error = %v", err)
}
if view.OverdueCount != 1 || view.HighPriorityOpenCount != 1 || view.WaitReadyCount != 1 {
	t.Fatalf("summary counts = overdue %d high %d wait %d", view.OverdueCount, view.HighPriorityOpenCount, view.WaitReadyCount)
}
if view.OverdueRefs[0].Label != "OPS-1" {
	t.Fatalf("OverdueRefs[0].Label = %q, want OPS-1", view.OverdueRefs[0].Label)
}
```

新增权限断言：

```go
projectOnly := serviceWithScopes(t, []string{"project:read"})
_, err := projectOnly.ProjectTaskSummary("ops")
if err == nil {
	t.Fatal("ProjectTaskSummary without task read scope succeeded")
}

taskOnly := serviceWithScopes(t, []string{"task:read"})
_, err = taskOnly.ProjectTaskSummary("ops")
if err == nil {
	t.Fatal("ProjectTaskSummary without project read scope succeeded")
}
```

Run:

```bash
go test ./internal/app -run TestServiceProjectTaskSummary -count=1
```

Expected: FAIL，`ProjectTaskSummary` 未定义。

- [ ] **Step 2: 增加 app view 类型**

在 `internal/app/project.go` 增加：

```go
type ProjectSummaryTaskRefView struct {
	UUID     string
	TaskSlug string
	Title    string
	Label    string
}

type ProjectSummaryWorkloadView struct {
	User              *task.UserInfo
	Label             string
	OpenCount         int
	OverdueCount      int
	HighPriorityCount int
}

type ProjectTaskSummaryView struct {
	OverdueCount          int
	OverdueRefs           []ProjectSummaryTaskRefView
	HighPriorityOpenCount int
	HighPriorityOpenRefs  []ProjectSummaryTaskRefView
	WaitReadyCount        int
	WaitReadyRefs         []ProjectSummaryTaskRefView
	UnassignedOpenCount   int
	UnassignedOpenRefs    []ProjectSummaryTaskRefView
	Workload              []ProjectSummaryWorkloadView
}
```

- [ ] **Step 3: 实现 app 方法**

在 `internal/app/project.go` 增加：

```go
func (s *Service) ProjectTaskSummary(ref string) (ProjectTaskSummaryView, error) {
	if err := s.Require(PermissionProjectRead); err != nil {
		return ProjectTaskSummaryView{}, err
	}
	if err := s.Require(PermissionTaskRead); err != nil {
		return ProjectTaskSummaryView{}, err
	}
	project, err := s.ResolveProject(ref)
	if err != nil {
		return ProjectTaskSummaryView{}, err
	}
	summary, err := s.projectRepo.TaskSummary(s.workspaceID, project.ID, s.clock.Unix())
	if err != nil {
		return ProjectTaskSummaryView{}, err
	}
	return projectTaskSummaryViewFromStorage(summary), nil
}
```

`projectTaskRefLabel` 规则：

```go
func projectTaskRefLabel(ref storage.ProjectTaskRefSummary) string {
	if strings.TrimSpace(ref.TaskSlug) != "" {
		return ref.TaskSlug
	}
	if len(ref.UUID) >= 8 {
		return ref.UUID[:8]
	}
	return ref.UUID
}
```

负责人 label 规则：

```go
func projectWorkloadLabel(row storage.ProjectAssigneeWorkloadRow) string {
	for _, candidate := range []string{row.DisplayName, row.Name, stringValue(row.Email), row.UserID} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return "未分配任务"
}
```

- [ ] **Step 4: 跑 app 测试**

Run:

```bash
go test ./internal/app -run TestServiceProjectTaskSummary -count=1
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/app/project.go internal/app/service_test.go
git commit -m "feat: 暴露项目任务摘要服务"
```

### Task 3: HTTP task-summary 端点

**Files:**

- Modify: `internal/httpapi/projects.go`
- Modify: `internal/httpapi/projects_test.go`
- Modify: `internal/httpapi/huma_routes.go`

**Interfaces:**

- Consumes: `Service.ProjectTaskSummary(ref string)`
- Produces: `GET /api/v1/projects/{projectRef}/task-summary?workspace={workspaceSlug}`

- [ ] **Step 1: 写 HTTP 失败测试**

在 `internal/httpapi/projects_test.go` 新增 `TestHTTPProjectTaskSummary`，请求：

```go
req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/ops/task-summary?workspace=local", nil)
req.Header.Set("Authorization", "Bearer "+token)
rr := httptest.NewRecorder()
server.ServeHTTP(rr, req)
if rr.Code != http.StatusOK {
	t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
}
var envelope struct {
	Data struct {
		OverdueCount int `json:"overdue_count"`
		Workload []struct {
			Label string `json:"label"`
			User *task.JSONUserInfo `json:"user"`
		} `json:"workload"`
	} `json:"data"`
}
if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
	t.Fatal(err)
}
if envelope.Data.OverdueCount != 1 {
	t.Fatalf("overdue_count = %d, want 1", envelope.Data.OverdueCount)
}
```

Run:

```bash
go test ./internal/httpapi -run TestHTTPProjectTaskSummary -count=1
```

Expected: FAIL，路由 404 或 handler 未定义。

- [ ] **Step 2: 增加 response DTO**

在 `internal/httpapi/projects.go` 增加：

```go
type projectSummaryTaskRefResponse struct {
	UUID     string `json:"uuid"`
	TaskSlug string `json:"task_slug,omitempty"`
	Title    string `json:"title"`
	Label    string `json:"label"`
}

type projectSummaryWorkloadResponse struct {
	User              *task.JSONUserInfo `json:"user,omitempty"`
	Label             string             `json:"label"`
	OpenCount         int                `json:"open_count"`
	OverdueCount      int                `json:"overdue_count"`
	HighPriorityCount int                `json:"high_priority_count"`
}

type projectTaskSummaryResponse struct {
	OverdueCount          int                             `json:"overdue_count"`
	OverdueRefs           []projectSummaryTaskRefResponse `json:"overdue_refs"`
	HighPriorityOpenCount int                             `json:"high_priority_open_count"`
	HighPriorityOpenRefs  []projectSummaryTaskRefResponse `json:"high_priority_open_refs"`
	WaitReadyCount        int                             `json:"wait_ready_count"`
	WaitReadyRefs         []projectSummaryTaskRefResponse `json:"wait_ready_refs"`
	UnassignedOpenCount   int                             `json:"unassigned_open_count"`
	UnassignedOpenRefs    []projectSummaryTaskRefResponse `json:"unassigned_open_refs"`
	Workload              []projectSummaryWorkloadResponse `json:"workload"`
}
```

- [ ] **Step 3: 增加 handler**

在 `internal/httpapi/projects.go` 增加：

```go
func (s *Server) handleProjectTaskSummary(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	summary, err := scoped.ProjectTaskSummary(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectTaskSummaryToJSON(summary), nil)
}
```

- [ ] **Step 4: 注册 Huma 路由**

在 `internal/httpapi/huma_routes.go` 的 project routes 区域加入，位置放在 project info 后、config 前：

```go
{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/task-summary", Tag: "Projects", Summary: "Get project task summary.", Handler: s.handleProjectTaskSummary},
```

- [ ] **Step 5: 跑 HTTP 测试**

Run:

```bash
go test ./internal/httpapi -run TestHTTPProjectTaskSummary -count=1
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/projects.go internal/httpapi/projects_test.go internal/httpapi/huma_routes.go
git commit -m "feat: 增加项目任务摘要接口"
```

## Chunk 2: 前端 API、权限与共享数据 hooks

### Task 4: 增加 task-summary API 与 query hook

**Files:**

- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-project-data.ts`
- Modify: `web/src/features/workspace/project-workbench/permissions/permissions.ts`
- Test: `web/src/features/workspace/project-workbench/permissions/permissions.test.ts`

**Interfaces:**

- Produces: `getProjectTaskSummary(workspaceSlug, projectRef)`
- Produces: `useProjectTaskSummaryQuery(workspaceSlug, projectSlug, enabled?)`
- Produces: `canTaskRead({ role, scopes })`
- Produces: `canAuditRead({ role, scopes })`

- [ ] **Step 1: 写 API/path 失败测试**

在 `project-api.test.ts` 新增：

```ts
it("builds project task summary path", () => {
  expect(projectTaskSummaryPath("local", "ops/demo")).toBe(
    "/api/v1/projects/ops%2Fdemo/task-summary?workspace=local"
  )
})
```

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/api/project-api.test.ts
```

Expected: FAIL，`projectTaskSummaryPath` 未导出。

- [ ] **Step 2: 实现 API 类型与 fetcher**

在 `project-api.ts` 加入本计划 `Shared Interfaces` 中的前端类型，并新增：

```ts
export function projectTaskSummaryPath(
  workspaceSlug: string,
  projectRef: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/task-summary?${workspaceQuery(workspaceSlug)}`
}

export function getProjectTaskSummary(
  workspaceSlug: string,
  projectRef: string
): Promise<ProjectTaskSummary> {
  return workspaceApiGet<ProjectTaskSummary>(
    projectTaskSummaryPath(workspaceSlug, projectRef)
  )
}
```

- [ ] **Step 3: 增加 query hook**

在 `use-project-data.ts` 加入：

```ts
projectTaskSummary: (workspaceSlug: string, projectSlug: string) =>
  ["project", workspaceSlug, projectSlug, "task-summary"] as const,
```

并新增：

```ts
export function useProjectTaskSummaryQuery(
  workspaceSlug: string,
  projectSlug: string,
  enabled = true
) {
  return useQuery({
    queryKey: projectQueryKeys.projectTaskSummary(workspaceSlug, projectSlug),
    queryFn: () => getProjectTaskSummary(workspaceSlug, projectSlug),
    enabled: enabled && workspaceSlug.length > 0 && projectSlug.length > 0,
  })
}
```

- [ ] **Step 4: 增加 task/audit 权限 helper**

在 `permissions.ts` 增加：

```ts
export function canTaskRead(input: WorkbenchPermissionInput): boolean {
  return hasScope(input.scopes, "task:read") && ["owner", "admin", "member", "viewer"].includes(input.role ?? "")
}

export function canAuditRead(input: WorkbenchPermissionInput): boolean {
  return hasScope(input.scopes, "audit:read") && ["owner", "admin"].includes(input.role ?? "")
}
```

在 `permissions.test.ts` 增加：

- owner/admin/member/viewer + `task:read` 时 `canTaskRead` 为 true。
- 缺 `task:read` 时 `canTaskRead` 为 false。
- owner/admin + `audit:read` 时 `canAuditRead` 为 true。
- member 或缺 scope 时 `canAuditRead` 为 false。

- [ ] **Step 5: 跑前端 API/权限测试**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/api/project-api.test.ts web/src/features/workspace/project-workbench/permissions/permissions.test.ts
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/api/project-api.ts web/src/features/workspace/project-workbench/api/project-api.test.ts web/src/features/workspace/project-workbench/hooks/use-project-data.ts web/src/features/workspace/project-workbench/permissions/permissions.ts web/src/features/workspace/project-workbench/permissions/permissions.test.ts
git commit -m "feat: 增加项目摘要前端接口"
```

## Chunk 3: 项目 Layout、Tabs 与可开合右栏

### Task 5: 抽 ProjectLayout 与 ProjectTabs

**Files:**

- Create: `web/src/features/workspace/project-workbench/project/project-layout.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-tabs.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-layout.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**

- Produces: `ProjectLayout({ workspaceSlug, projectSlug, activeTab, children, actions })`
- Produces: `ProjectTabs({ workspaceSlug, projectSlug, activeTab })`

- [ ] **Step 1: 写 layout 失败测试**

`project-layout.test.tsx` 覆盖：

```ts
it("renders header, tabs and child content", async () => {
  render(<ProjectLayout workspaceSlug="local" projectSlug="ops" activeTab="overview"><div>概览主体</div></ProjectLayout>)
  expect(await screen.findByText("概览主体")).toBeInTheDocument()
  expect(screen.getByRole("link", { name: "概览" })).toHaveAttribute("href", "/workspaces/local/projects/ops")
  expect(screen.getByRole("link", { name: "任务" })).toHaveAttribute("href", "/workspaces/local/projects/ops/tasks")
  expect(screen.getByRole("link", { name: "活动" })).toHaveAttribute("href", "/workspaces/local/projects/ops/activity")
})
```

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/project/project-layout.test.tsx
```

Expected: FAIL，组件未创建。

- [ ] **Step 2: 实现 ProjectTabs**

`project-tabs.tsx`：

```tsx
export type ProjectTabKey = "overview" | "tasks" | "activity"

export function ProjectTabs({ activeTab, projectSlug, workspaceSlug }: ProjectTabsProps) {
  const items = [
    { key: "overview", label: "概览", to: "/workspaces/$workspaceSlug/projects/$projectSlug" },
    { key: "tasks", label: "任务", to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks" },
    { key: "activity", label: "活动", to: "/workspaces/$workspaceSlug/projects/$projectSlug/activity" },
  ] as const
  return (
    <nav aria-label="项目子页面" className="flex h-10 items-center gap-1 border-b">
      {items.map((item) => (
        <Link
          className={cn("px-3 py-2 text-sm", activeTab === item.key ? "font-medium text-foreground" : "text-muted-foreground")}
          key={item.key}
          params={{ workspaceSlug, projectSlug }}
          to={item.to}
        >
          {item.label}
        </Link>
      ))}
    </nav>
  )
}
```

- [ ] **Step 3: 实现 ProjectLayout**

`project-layout.tsx` 负责：

- 调 `useMe()`、`useProjectQuery()`。
- 计算 `canManage`、`canCreateTask`、`canReadTasks`、`closed`。
- 渲染 `ProjectHeaderEditor`、`ProjectTabs`、`ProjectContextRail` 与 `children`。
- 提供 `ProjectLayoutContext` 给子页读取 `project`、`me`、权限；不在共享 Header 中提供导入任务动作。

Context 类型：

```ts
export type ProjectLayoutContextValue = {
  workspaceSlug: string
  projectSlug: string
  project: ProjectWorkbenchProject
  canManage: boolean
  canCreateTask: boolean
  canReadTasks: boolean
  closed: boolean
}
```

- [ ] **Step 4: 跑 layout 测试**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/project/project-layout.test.tsx
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/project-workbench/project/project-layout.tsx web/src/features/workspace/project-workbench/project/project-tabs.tsx web/src/features/workspace/project-workbench/project/project-layout.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 抽项目子页面布局"
```

### Task 6: 实现 ProjectContextRail 开合与数据展示

**Files:**

- Create: `web/src/features/workspace/project-workbench/project/project-context-rail.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-layout.tsx`

**Interfaces:**

- Consumes: `ProjectWorkbenchProject`
- Consumes: `ProjectTaskSummary`
- Consumes: `ConfigEffectiveValue[]`
- Consumes: `ProjectTimelineEntry[]`

- [ ] **Step 1: 写右栏失败测试**

测试必须覆盖：

```ts
it("collapses and expands with icon buttons", async () => {
  const user = userEvent.setup()
  render(<ProjectContextRail project={project} summary={summary} configRows={rows} timeline={timeline} />)
  expect(screen.getByText("项目信息")).toBeInTheDocument()
  await user.click(screen.getByRole("button", { name: "收起右栏" }))
  expect(screen.queryByText("项目信息")).not.toBeInTheDocument()
  await user.click(screen.getByRole("button", { name: "展开右栏" }))
  expect(screen.getByText("项目信息")).toBeInTheDocument()
})
```

附属信息测试：

```ts
expect(screen.getByText("模型供应商")).toBeInTheDocument()
expect(screen.getByText("openai")).toBeInTheDocument()
expect(screen.queryByText("运行配置")).not.toBeInTheDocument()
expect(screen.getByText("回调密钥")).toBeInTheDocument()
expect(screen.queryByText("raw-secret")).not.toBeInTheDocument()
```

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx
```

Expected: FAIL，组件未创建。

- [ ] **Step 2: 实现图标按钮**

使用：

```tsx
import { PanelRightCloseIcon, PanelRightOpenIcon } from "lucide-react"
```

按钮必须是 icon-only：

```tsx
<Button aria-label="收起右栏" title="收起右栏" size="icon" variant="ghost" type="button">
  <PanelRightCloseIcon className="h-4 w-4" />
</Button>
```

收起后只保留展开按钮，不渲染右栏区块内容。

- [ ] **Step 3: 实现右栏数据展示**

右栏展示规则：

- 状态、任务数、创建/更新：来自 `ProjectWorkbenchProject`。
- 完成进度：`completed_count / task_count`，`task_count === 0` 时显示 `0%`。
- 逾期、高优未完成、等待已到期、未分配任务：来自 `ProjectTaskSummary`。
- 附属信息：`ConfigEffectiveValue[]`，label 为 `row.definition.label || row.key`；`missing_required` 显示「缺少必填」；`row.definition.secret === true` 时只展示后端返回的脱敏值或「已设置」，不调用普通 formatter 展示原文。
- 负责人负载：来自 `summary.workload`，优先显示 open_count 最大的前 5 条。
- 最近活动：来自 `ProjectTimelineEntry[]` 前 5 条。
- 窄屏：默认收起右栏或改为不挤压主内容的抽屉/下方区块；测试中至少用 class/状态断言确认任务表格容器不会被固定右栏挤压。

- [ ] **Step 4: 接入 ProjectLayout**

`ProjectLayout` 发起：

```ts
const summary = useProjectTaskSummaryQuery(workspaceSlug, projectSlug, canReadTasks)
const timeline = useProjectTimelineQuery(workspaceSlug, projectSlug)
const homeConfig = useQuery({
  queryKey: ["project", workspaceSlug, projectSlug, "config-effective", "console-home"],
  queryFn: () => listProjectEffectiveConfig(projectSlug, { consoleHome: true }),
})
```

失败处理：

- summary 失败：右栏隐藏「进度风险」和「负责人负载」，显示轻量错误「项目摘要暂不可用」。
- config 失败：隐藏附属信息或显示轻量错误，不阻塞页面。
- timeline 失败：最近活动显示轻量错误。

- [ ] **Step 5: 跑右栏测试**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx web/src/features/workspace/project-workbench/project/project-layout.test.tsx
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/project/project-context-rail.tsx web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx web/src/features/workspace/project-workbench/project/project-layout.tsx
git commit -m "feat: 增加项目右侧信息栏"
```

## Chunk 4: Overview 子页面

### Task 7: 实现 ProjectOverviewPage

**Files:**

- Create: `web/src/features/workspace/project-workbench/project/project-overview-page.tsx`
- Create: `web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`
- Modify: `web/src/routes/workspace/ProjectWorkbenchRoute.tsx`

**Interfaces:**

- Consumes: `ProjectLayoutContext`
- Consumes: `useProjectTaskSummaryQuery`
- Consumes: `listProjectEffectiveConfig(projectSlug, { consoleHome: true })`
- Consumes: `useProjectTimelineQuery`

- [ ] **Step 1: 写 Overview 失败测试**

测试必须覆盖：

```ts
it("renders whole-project focus from task summary", async () => {
  render(<ProjectOverviewPage workspaceSlug="local" projectSlug="ops" />)
expect(await screen.findByText("当前重点")).toBeInTheDocument()
expect(screen.getByText("逾期任务")).toBeInTheDocument()
expect(screen.getByText("等待已到期")).toBeInTheDocument()
expect(screen.getByText("未分配任务")).toBeInTheDocument()
expect(screen.getByText("DEM-7、DEM-9")).toBeInTheDocument()
expect(screen.queryByText("当前加载范围内")).not.toBeInTheDocument()
})
```

附属信息测试：

```ts
expect(screen.getByText("模型供应商")).toBeInTheDocument()
expect(screen.getByText("openai")).toBeInTheDocument()
expect(screen.queryByText("运行配置")).not.toBeInTheDocument()
expect(screen.queryByText("raw-secret")).not.toBeInTheDocument()
```

无任务读取权限测试：

```ts
it("does not request project task summary without task read", async () => {
  render(<ProjectOverviewPage workspaceSlug="local" projectSlug="ops" />)
  await screen.findByText("暂无项目更新")
  expect(fetchSpy).not.toHaveBeenCalledWith(expect.stringContaining("/task-summary"), expect.anything())
})
```

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx
```

Expected: FAIL，页面未创建。

- [ ] **Step 2: 实现 Overview 区块**

`ProjectOverviewPage` 渲染：

- 最新项目更新：`project.recent_annotations?.[0]`，为空显示「暂无项目更新」。
- 当前重点：四行来自 `summary`，每行最多显示 3 个 `ref.label`，中文标签使用「逾期任务 / 高优未完成 / 等待已到期 / 未分配任务」；`查看` 跳转到 Tasks 页并带筛选。
- 项目附属信息：console-home effective config 的 label/value，不显示区块标题「运行配置」；secret 值只展示脱敏值或「已设置」。
- 负责人负载：`summary.workload`。
- 最近活动：`timeline.data?.slice(0, 5)`。

当前重点跳转规则：

```ts
const focusLinks = {
  overdue: { due_before: todayISO, status: "open" },
  high: { priority: "H", status: "open" },
  waitReady: { wait_before: todayISO, status: "open" },
  unassigned: { assignee_empty: "true", status: "open" },
}
```

`todayISO` 只用于跳转筛选；Overview 显示的计数仍然来自后端 summary。
如果现有任务查询表达式无法精确复现 ProjectSummary 的后端规则，Tasks 页只显示筛选条件，不显示“与 Overview 计数完全一致”的文案。

- [ ] **Step 3: 路由 wrapper 使用 Overview**

`ProjectWorkbenchRoute` 继续匹配根项目页，但渲染：

```tsx
<ProjectLayout activeTab="overview" projectSlug={params.projectSlug} workspaceSlug={params.workspaceSlug}>
  <ProjectOverviewPage projectSlug={params.projectSlug} workspaceSlug={params.workspaceSlug} />
</ProjectLayout>
```

保留 `ProjectWorkbenchPage` 时，将它改成上述 wrapper，避免旧任务表继续出现在根项目页。

- [ ] **Step 4: 跑 Overview 测试**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx web/src/features/workspace/project-workbench/project/project-workbench-page.test.tsx
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/project-workbench/project/project-overview-page.tsx web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx web/src/features/workspace/project-workbench/project/project-workbench-page.tsx web/src/routes/workspace/ProjectWorkbenchRoute.tsx
git commit -m "feat: 增加项目概览页"
```

## Chunk 5: Tasks 子页面

### Task 8: 迁移当前任务能力到 ProjectTasksPage

**Files:**

- Create: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.tsx`
- Create: `web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx`
- Create: `web/src/routes/workspace/ProjectTasksRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.tsx`

**Interfaces:**

- Consumes: `ProjectTaskToolbar`
- Consumes: `TaskTable`
- Consumes: `TaskCreateDialog`
- Consumes: `TaskImportDialog`
- Consumes: `useProjectTasksQuery`

- [ ] **Step 1: 写 Tasks 页失败测试**

测试必须确认是简单列表，不是分组：

```ts
it("renders task toolbar and simple table without grouping headings", async () => {
  render(<ProjectTasksPage workspaceSlug="local" projectSlug="ops" />)
  expect(await screen.findByRole("table")).toBeInTheDocument()
  expect(screen.getByText("DEM-1")).toBeInTheDocument()
  expect(screen.queryByText("按状态")).not.toBeInTheDocument()
  expect(screen.queryByText("按负责人")).not.toBeInTheDocument()
})
```

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx
```

Expected: FAIL，页面未创建。

- [ ] **Step 2: 从旧 ProjectWorkbenchPage 移动任务逻辑**

`ProjectTasksPage` 包含当前旧页面中的这些逻辑：

- URL search -> `TaskFilter`
- `filterToTaskQuery(filter)`
- `useProjectTasksQuery(workspaceSlug, projectSlug, filterQuery)`
- `getWorkspaceMembers(workspaceSlug)`
- `ProjectTaskToolbar`
- `TaskTable`
- `TaskCreateDialog`
- `TaskImportDialog`

Tasks 页动作规则：

- 新建任务使用 `PlusIcon` 图标按钮，`aria-label="新建任务"`，打开 `TaskCreateDialog`。
- 导入任务使用 `UploadIcon` 图标按钮，`aria-label="导入任务"`，打开 `TaskImportDialog`。
- 这两个动作不出现在共享项目 Header 中。

导航更新目标必须改为：

```ts
to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks"
```

- [ ] **Step 3: 创建 ProjectTasksRoute**

`web/src/routes/workspace/ProjectTasksRoute.tsx`：

```tsx
export function ProjectTasksRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string; workspaceSlug: string }
  return (
    <ProjectLayout activeTab="tasks" projectSlug={params.projectSlug} workspaceSlug={params.workspaceSlug}>
      <ProjectTasksPage projectSlug={params.projectSlug} workspaceSlug={params.workspaceSlug} />
    </ProjectLayout>
  )
}
```

- [ ] **Step 4: 更新 router search validation**

在 `router.tsx` 新增 project tasks route，复用当前 project route 的 task filter `validateSearch`。项目根 route 不再接收任务筛选 search；为了旧链接兼容，可以接受但 Overview 不使用这些 search。

路径：

```ts
path: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks"
```

保留任务详情：

```ts
path: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
```

- [ ] **Step 5: 跑 Tasks 测试**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx web/src/features/workspace/project-workbench/tasks/project-task-toolbar.test.tsx web/src/features/workspace/project-workbench/tasks/task-table.test.tsx
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/tasks/project-tasks-page.tsx web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx web/src/routes/workspace/ProjectTasksRoute.tsx web/src/routes/router.tsx web/src/features/workspace/project-workbench/project/project-workbench-page.tsx
git commit -m "feat: 增加项目任务子页面"
```

## Chunk 6: Activity 子页面与 notes 迁移

### Task 9: 校准 ProjectTimeline 的 project entry 标识

**Files:**

- Modify: `internal/storage/project_annotation_repo.go`
- Modify: `internal/storage/project_annotation_repo_test.go`
- Modify: `internal/httpapi/project_annotation_test.go`

**Interfaces:**

- Produces: project timeline 中 `source_type="project"` 的 entry，`source_id` 为 project annotation id，`source_label` 为 project slug。

- [ ] **Step 1: 写 timeline source_id 失败测试**

在 `internal/storage/project_annotation_repo_test.go` 增加断言：

```go
rows, err := repo.TimelineByProjectID(projectID, 10, 0)
if err != nil {
	t.Fatalf("TimelineByProjectID() error = %v", err)
}
projectRow := firstTimelineRowByType(rows, "project")
if projectRow.SourceID != annotationID {
	t.Fatalf("project timeline source_id = %q, want annotation id %q", projectRow.SourceID, annotationID)
}
if projectRow.SourceLabel != "ops" {
	t.Fatalf("project timeline source_label = %q, want ops", projectRow.SourceLabel)
}
```

Run:

```bash
go test ./internal/storage -run TestProjectAnnotationTimeline -count=1
```

Expected: FAIL，当前 project entry 的 `source_id` 是 project id。

- [ ] **Step 2: 修改 SQL**

在 `TimelineByProjectID` 的 project annotation 分支中，把：

```sql
SELECT 'project' AS source_type, ? AS source_id, ? AS source_label,
```

改成：

```sql
SELECT 'project' AS source_type, id AS source_id, ? AS source_label,
```

并移除多余的 projectID SQL 参数。task annotation 分支保持 `source_id = task uuid`。

- [ ] **Step 3: 补 HTTP 契约测试**

在 `internal/httpapi/project_annotation_test.go` 或现有 timeline 测试中断言：

```go
if got := envelope.Data[0].SourceID; got != annotationID {
	t.Fatalf("timeline project source_id = %q, want annotation id", got)
}
```

Run:

```bash
go test ./internal/storage ./internal/httpapi -run 'ProjectAnnotationTimeline|ProjectTimeline' -count=1
```

Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add internal/storage/project_annotation_repo.go internal/storage/project_annotation_repo_test.go internal/httpapi/project_annotation_test.go
git commit -m "fix: 校准项目时间线记录标识"
```

### Task 10: 实现 ProjectActivityPage

**Files:**

- Create: `web/src/features/workspace/project-workbench/activity/project-activity-page.tsx`
- Create: `web/src/features/workspace/project-workbench/activity/project-activity-timeline.tsx`
- Create: `web/src/features/workspace/project-workbench/activity/project-activity-page.test.tsx`
- Create: `web/src/routes/workspace/ProjectActivityRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Keep: `web/src/pages/project-notes-tab.tsx`

**Interfaces:**

- Consumes: `addProjectAnnotation`、`deleteProjectAnnotation`
- Consumes: `useProjectTimelineQuery`
- Consumes: audit API `auditPath({ project: projectSlug, limit: 100 })`
- Consumes: `canAuditRead()`

- [ ] **Step 1: 写 Activity 失败测试**

覆盖发布入口与无 audit 权限：

```ts
it("does not request audit and hides audit filter without audit read", async () => {
  render(<ProjectActivityPage workspaceSlug="local" projectSlug="ops" />)
  expect(await screen.findByText("项目更新")).toBeInTheDocument()
  expect(screen.queryByRole("button", { name: "审计" })).not.toBeInTheDocument()
  expect(fetchSpy).not.toHaveBeenCalledWith(expect.stringContaining("/api/v1/audit"), expect.anything())
})
```

覆盖 project annotation 不重复：

```ts
it("uses timeline as the only list source for project updates", async () => {
  render(<ProjectActivityPage workspaceSlug="local" projectSlug="ops" />)
  expect(await screen.findAllByText("完成 token mcp-config 验证")).toHaveLength(1)
  expect(fetchSpy).not.toHaveBeenCalledWith(expect.stringContaining("/annotations"), expect.objectContaining({ method: "GET" }))
})
```

覆盖有 audit 权限：

```ts
it("shows audit filter for owner with audit:read", async () => {
  render(<ProjectActivityPage workspaceSlug="local" projectSlug="ops" />)
  expect(await screen.findByRole("button", { name: "审计" })).toBeInTheDocument()
})
```

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/activity/project-activity-page.test.tsx
```

Expected: FAIL，页面未创建。

- [ ] **Step 2: 实现项目更新表单**

复用 `ProjectNotesTab` 的 mutation 逻辑，写入仍走：

```ts
addProjectAnnotation(workspaceSlug, projectSlug, text)
```

显示规则：

- `canManage && !closed` 时显示输入框和「发布更新」按钮。
- closed 项目显示只读提示。
- 只有 `project:read` 时展示时间线但不展示输入框。
- 列表只使用 `useProjectTimelineQuery`。发布成功后 invalidate timeline、project query，不额外把 annotations 列表拼到 timeline。

- [ ] **Step 3: 实现 ActivityTimeline**

统一 entry 类型：

```ts
type ActivityItem =
  | { kind: "project"; id: string; entry: number; content: string; actorLabel: string }
  | { kind: "task"; id: string; entry: number; content: string; sourceLabel: string; actorLabel: string }
  | { kind: "audit"; id: string; entry: number; action: string; actorLabel: string; summary: string }
```

数据点约束：

- project annotation 只用 `content`、`created_by`、`created_at`、`entry`、`id`。
- project annotation 的 `id` 来自 timeline entry 的 `source_id`。
- task annotation 只用 timeline 的 `source_label`、`content`、`entry`、`source_id`。
- audit 只用 audit row 的 `action`、`actor`、`created_at`、`changes` 或 `payload` 里已有字段。
去重 key 为 `${kind}:${id}:${entry}`；不能把 audit 与 annotation 合并成没有来源的自然语言摘要。

- [ ] **Step 4: 创建 ProjectActivityRoute 并注册路由**

`ProjectActivityRoute` 与 `ProjectTasksRoute` 同形，`activeTab="activity"`。

在 `router.tsx` 增加：

```ts
path: "/workspaces/$workspaceSlug/projects/$projectSlug/activity"
```

- [ ] **Step 5: 跑 Activity 测试**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/activity/project-activity-page.test.tsx web/src/features/workspace/project-workbench/project/project-notes-timeline.test.tsx
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/project-workbench/activity/project-activity-page.tsx web/src/features/workspace/project-workbench/activity/project-activity-timeline.tsx web/src/features/workspace/project-workbench/activity/project-activity-page.test.tsx web/src/routes/workspace/ProjectActivityRoute.tsx web/src/routes/router.tsx
git commit -m "feat: 增加项目活动子页面"
```

### Task 11: settings/notes 兼容重定向

**Files:**

- Modify: `web/src/routes/workspace/ProjectSettingsNotesRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Test: `web/src/routes/workspace/WorkspaceRootRoute.test.tsx`

**Interfaces:**

- Produces: `/projects/:projectSlug/settings/notes` -> `/workspaces/:workspaceSlug/projects/:projectSlug/activity`

- [ ] **Step 1: 写重定向失败测试**

测试从 `/projects/ops/settings/notes` 进入时重定向到 Activity：

```ts
expect(router.state.location.pathname).toBe("/workspaces/local/projects/ops/activity")
```

Run:

```bash
pnpm --dir web test web/src/routes/workspace/WorkspaceRootRoute.test.tsx
```

Expected: FAIL，当前仍渲染 notes tab。

- [ ] **Step 2: 实现重定向**

`ProjectSettingsNotesRoute.tsx` 改为：

```tsx
export function ProjectSettingsNotesRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug
  if (!workspaceSlug) {
    return <section className="border bg-card p-6 text-sm text-muted-foreground">Loading...</section>
  }
  return <Navigate to="/workspaces/$workspaceSlug/projects/$projectSlug/activity" params={{ workspaceSlug, projectSlug: params.projectSlug }} replace />
}
```

- [ ] **Step 3: 跑重定向测试**

Run:

```bash
pnpm --dir web test web/src/routes/workspace/WorkspaceRootRoute.test.tsx
```

Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add web/src/routes/workspace/ProjectSettingsNotesRoute.tsx web/src/routes/workspace/WorkspaceRootRoute.test.tsx
git commit -m "feat: 将项目记录入口迁到活动页"
```

## Chunk 7: 文案、回归测试与视觉检查

### Task 12: 中文文案与路由回归

**Files:**

- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/features/workspace/project-workbench/project/project-workbench-page.test.tsx`
- Modify: `web/src/routes/workspace/WorkspaceRootRoute.test.tsx`

- [ ] **Step 1: 补齐文案 key**

中文 key 建议：

```ts
projectSubpages: {
  overview: "概览",
  tasks: "任务",
  activity: "活动",
  railExpand: "展开右栏",
  railCollapse: "收起右栏",
  focusTitle: "当前重点",
  overdueTasks: "逾期任务",
  highPriorityOpen: "高优未完成",
  waitReady: "等待已到期",
  unassignedOpen: "未分配任务",
  secretConfigured: "已设置",
  projectFacts: "项目附属信息",
  manageConfig: "管理配置",
  latestUpdate: "最新项目更新",
  noProjectUpdate: "暂无项目更新",
}
```

避免出现：

```text
运行配置
issue
milestone
lead
当前加载范围内
等待解除
缺负责人
```

- [ ] **Step 2: 跑静态文案搜索**

Run:

```bash
rg -n "运行配置|issue|milestone|lead|当前加载范围内|等待解除|缺负责人" web/src/features/workspace/project-workbench web/src/locales
```

Expected: 不应在项目子页面新增代码中命中；若历史 locale 仍有无关命中，确认不被新页面引用并在计划执行记录中说明。

- [ ] **Step 3: 跑 route 与页面 focused tests**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/project/project-layout.test.tsx web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx web/src/features/workspace/project-workbench/activity/project-activity-page.test.tsx
```

Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/zh-CN.ts web/src/locales/en-US.ts web/src/features/workspace/project-workbench/project/project-workbench-page.test.tsx web/src/routes
git commit -m "test: 补项目子页面回归覆盖"
```

### Task 13: 最终验证

**Files:**

- No source changes unless verification exposes failures.

- [ ] **Step 1: Go focused tests**

Run:

```bash
go test ./internal/storage -run TestProjectRepositoryTaskSummary -count=1
go test ./internal/storage -run TestProjectAnnotationTimeline -count=1
go test ./internal/app -run TestServiceProjectTaskSummary -count=1
go test ./internal/httpapi -run 'TestHTTPProjectTaskSummary|TestHTTPProjectTimeline' -count=1
```

Expected: PASS。

- [ ] **Step 2: Go full verification**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS。

- [ ] **Step 3: Frontend focused tests**

Run:

```bash
pnpm --dir web test web/src/features/workspace/project-workbench/api/project-api.test.ts web/src/features/workspace/project-workbench/permissions/permissions.test.ts web/src/features/workspace/project-workbench/project/project-layout.test.tsx web/src/features/workspace/project-workbench/project/project-context-rail.test.tsx web/src/features/workspace/project-workbench/project/project-overview-page.test.tsx web/src/features/workspace/project-workbench/tasks/project-tasks-page.test.tsx web/src/features/workspace/project-workbench/activity/project-activity-page.test.tsx
```

Expected: PASS。

- [ ] **Step 4: Frontend full verification**

Run:

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

Expected: PASS。

- [ ] **Step 5: Diff hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected:

- `git diff --check` 无输出。
- `git status --short` 只包含本功能相关文件。

- [ ] **Step 6: 浏览器手动验收**

启动本地服务后检查：

```text
/workspaces/local/projects/ops
  显示概览，不显示完整任务表。

/workspaces/local/projects/ops/tasks
  显示筛选工具栏与简单任务列表，不出现分组标题。

/workspaces/local/projects/ops/activity
  显示项目更新与时间线；无 audit:read 时不出现审计筛选。

右栏
  默认打开；点击「收起右栏」图标后主体变宽；点击「展开右栏」图标后恢复。
```

- [ ] **Step 7: Final commit**

前面按 task 小步提交后，本步只提交验证阶段产生的修复；仍有未提交相关文件时执行：

```bash
git add docs/superpowers/plans/2026-07-07-web-console-project-subpages-implementation.md internal web
git commit -m "feat: 落地项目子页面"
```

## Self-Review Checklist

- [ ] Overview 当前重点全部来自 `ProjectTaskSummary`，没有使用当前任务列表或筛选结果派生全量数字。
- [ ] ProjectSummary 要求 `task:read`，且 date-only due/wait 边界与既有任务日期规则一致。
- [ ] Tasks 页仍是简单列表，没有状态分组、负责人分组、父子树、看板、泳道。
- [ ] 右侧栏可打开/收起，开合按钮是 lucide 图标按钮，带 `aria-label` 和 `title`。
- [ ] 窄屏下右侧栏不挤压任务表格导致文字重叠。
- [ ] 项目附属信息显示 label/value，不出现「运行配置」标题，secret 不显示原文。
- [ ] Activity 无 `audit:read` 时不请求 audit，不展示审计筛选，project annotation 不重复展示。
- [ ] project timeline 中 `source_type=project` 的 `source_id` 是 annotation id。
- [ ] archived/cancelled 项目隐藏写入动作，但 Overview、Tasks、Activity 可读。
- [ ] Header、Tabs、右栏在三个项目子页面一致。
- [ ] 任务详情 `/workspaces/:workspaceSlug/projects/:projectSlug/tasks/:taskRef` 保持可用。
- [ ] 所有新增用户身份输出仍使用 `task.UserInfo` / `task.JSONUserInfo`，不输出裸 UUID。
- [ ] 全量验证命令已跑完并记录结果。
