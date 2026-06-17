# Web Console 项目-任务浏览体验重构 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 project-readonly 视图扶正为主体验，补全任务详情（注解懒加载 + links + UDAs），加 shadcn 过滤工具栏（URL 同步），打通从侧边栏到项目→任务→详情的完整浏览路径；后端补 annotations 分页端点、tasks restful filter、projects 聚合统计。

**Architecture:** 后端先行（3 个独立增强，各自可单独测试、可被现有 CLI/MCP 复用），全部挂在 `/api/v1/` 下、复用 `internal/app`、零 CGO。前端按现有分层（routes 极薄 → features page → features api → lib/api）新增/重构三个页面，引入 TanStack Router search schema 做 URL 同步。

**Tech Stack:** Go 1.25 / chi / gorm（glebarez/sqlite，零 CGO）；React 19 / TanStack Router+Query / Tailwind v4 / shadcn(radix-lyra) / vitest。

**Spec:** `docs/superpowers/specs/2026-06-17-web-console-project-task-browsing-design.md`

---

## 关键约束（来自 AGENTS.md / spec）

- 所有新接口在 `/api/v1/` 下，不经 `/api/v1/admin/`。
- 用户身份字段用 `task.UserInfo` / `task.JSONUserInfo`，不输出裸 UUID。
- 改动后必须满足 `CGO_ENABLED=0 go test ./...` 和 `CGO_ENABLED=0 go build ./cmd/xuanchu`。
- 后端 task status 只有 `pending`/`completed`/`waiting`/`recurring`/`deleted`，**无 active/started**。
- task JSON 字段名注意：循环是 `recur`（非 recurrence）；UDAs 平铺为 JSONTask 顶层字段；无 owner，用 assignees。

## 前端分层约定（本计划遵循）

```
routes/workspace/*.tsx          极薄：useParams/useSearch → 透传给 page
features/workspace/projects/    新 domain：projects-list 页（项目表格）
  ├─ projects-api.ts            类型 + path 函数 + workspaceApiGet 包装
  ├─ projects-list-page.tsx     useQuery + 状态分支 + 装配子组件
  └─ projects-list-page.test.tsx
features/workspace/project-readonly/  既有 domain（增强）
  ├─ project-readonly-api.ts    扩展类型 + 新增注解分页/links 类型
  ├─ project-page.tsx           加过滤工具栏 + URL search 同步
  ├─ project-task-detail-page.tsx  补 links/UDAs/注解懒加载
  ├─ project-filter-toolbar.tsx 新：过滤工具栏组件
  └─ project-task-detail-*.tsx  新拆：annotations/links/属性子组件
components/ui/                  复用现有 shadcn；按需 add select/calendar
```

每层职责单一：api 层只拼 path + 类型；page 层只取数 + 状态分支 + 装配；子组件只渲染 props。纯函数（如 filter 编码）单独文件。

---

## Task 1: 后端 — tasks filter restful 参数增强

**Files:**
- Modify: `internal/httpapi/tasks.go:88-152`（`handleTaskList`）
- Test: `internal/httpapi/tasks_test.go`（已有，追加用例）

把 restful query 参数翻译成现有 query DSL，与 `query=`/`filter=` 用 `query.And` 合并。

- [ ] **Step 1: 写失败测试 — status 参数**

在 `internal/httpapi/tasks_test.go` 末尾追加。该文件已有通过 `newTestServer` + 发请求验证返回的模式，照抄同文件内现有 `TestHandleTaskList` 的装配方式（若函数名不同，沿用文件里现存的列表测试函数模式）。

```go
func TestHandleTaskList_RestfulStatusFilter(t *testing.T) {
	srv := newTestServer(t)
	ws := srv.seedWorkspace(t)
	proj := srv.seedProject(t, ws.ID, "proj-a")
	srv.seedTask(t, ws.ID, proj.ID, "pending task", func(tk *task.Task) { tk.Status = task.StatusPending })
	srv.seedTask(t, ws.ID, proj.ID, "completed task", func(tk *task.Task) { tk.Status = task.StatusCompleted; tk.End = ptrInt64(1) })

	code, body := srv.jsonRequest(t, srv.tokenFor(ws.ID), http.MethodGet,
		"/api/v1/tasks?project=proj-a&status=pending", nil)

	if code != http.StatusOK {
		t.Fatalf("status %d, body %s", code, body)
	}
	var resp struct {
		Data []task.JSONTask `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Description != "pending task" {
		t.Fatalf("expected only pending task, got %+v", resp.Data)
	}
}
```

（`ptrInt64` 等辅助若测试文件已有则复用，否则参照文件内现有 seedTask 用法调整。）

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi/ -run TestHandleTaskList_RestfulStatusFilter -v`
Expected: FAIL（status 参数当前被忽略，会返回 2 条任务）。

- [ ] **Step 3: 实现 restful → DSL 翻译辅助函数**

在 `internal/httpapi/tasks.go` 的 `handleTaskList` 之前（`handleTaskList` 函数定义前）新增：

```go
// restfulTaskFilters 把 restful 风格的 query 参数翻译成 query DSL，
// 与原有 query=/filter= 表达式合并。参数校验失败返回 error。
func restfulTaskFilters(q url.Values) (query.Expr, error) {
	var expr query.Expr
	add := func(e query.Expr) { expr = query.And(expr, e) }

	if v := strings.TrimSpace(q.Get("status")); v != "" {
		add(query.Predicate{Attribute: query.AttrStatus, Operator: query.OpEqual, Value: query.StringValue(v)})
	}
	if v := strings.TrimSpace(q.Get("priority")); v != "" {
		add(query.Predicate{Attribute: query.AttrPriority, Operator: query.OpEqual, Value: query.StringValue(v)})
	}
	if v := strings.TrimSpace(q.Get("assignee")); v != "" {
		add(query.Predicate{Attribute: query.AttrAssignee, Operator: query.OpEqual, Value: query.StringValue(v)})
	}
	if v := strings.TrimSpace(q.Get("due_after")); v != "" {
		pred, err := datePredicate(query.AttrDue, query.OpAfter, v)
		if err != nil {
			return nil, err
		}
		add(pred)
	}
	if v := strings.TrimSpace(q.Get("due_before")); v != "" {
		pred, err := datePredicate(query.AttrDue, query.OpBefore, v)
		if err != nil {
			return nil, err
		}
		add(pred)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		add(query.Predicate{Attribute: query.AttrBare, Operator: query.OpContains, Value: query.BareValue(v)})
	}
	if v := strings.TrimSpace(q.Get("tags")); v != "" {
		for _, tag := range strings.Split(v, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			add(query.Predicate{Attribute: query.AttrTag, Operator: query.OpHasTag, Value: query.StringValue(tag)})
		}
	}
	return expr, nil
}

func datePredicate(attr query.Attribute, op query.Operator, raw string) (query.Expr, error) {
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return nil, fmt.Errorf("invalid date %q (expected YYYY-MM-DD)", raw)
	}
	return query.Predicate{Attribute: attr, Operator: op, Value: query.DateValue(raw)}, nil
}
```

确认导入：增加 `"net/url"`（若未引入）。

- [ ] **Step 4: 在 handleTaskList 中接入**

修改 `internal/httpapi/tasks.go:120-131`，在现有 `filters := r.URL.Query()["query"]` 块**之后、`projectRef` 块之前**插入：

```go
	restful, err := restfulTaskFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_filter", err.Error(), nil)
		return
	}
	input.Query = query.And(input.Query, restful)
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/httpapi/ -run TestHandleTaskList_RestfulStatusFilter -v`
Expected: PASS。

- [ ] **Step 6: 补齐其余参数的单测**

在 `internal/httpapi/tasks_test.go` 追加覆盖 `priority`、`due_before`、`due_after`、`q`、`tags`、`assignee` 各一个最小用例（参考 Step 1 模式，改参数和 seed）。再追加一个"无效日期返回 400"用例：

```go
func TestHandleTaskList_RestfulBadDate(t *testing.T) {
	srv := newTestServer(t)
	ws := srv.seedWorkspace(t)
	code, _ := srv.jsonRequest(t, srv.tokenFor(ws.ID), http.MethodGet,
		"/api/v1/tasks?due_after=not-a-date", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}
```

- [ ] **Step 7: 运行全部 httpapi 测试**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add internal/httpapi/tasks.go internal/httpapi/tasks_test.go
git commit -m "feat(httpapi): tasks 列表支持 restful 风格过滤参数（内部转 query DSL）"
```

---

## Task 2: 后端 — annotations 分页列表端点

**Files:**
- Modify: `internal/storage/task_repo.go`（新增 `ListAnnotations`）
- Modify: `internal/app/service.go`（新增 `ListAnnotations`）
- Modify: `internal/httpapi/router.go`（注册路由）
- Modify: `internal/httpapi/tasks.go`（新增 handler）
- Test: `internal/storage/task_repo_test.go`、`internal/app/service_test.go`、`internal/httpapi/tasks_test.go`

- [ ] **Step 1: 写失败测试 — storage 层**

在 `internal/storage/task_repo_test.go` 追加（参照文件内现有 repo 测试的 db 打开 + seed 模式）：

```go
func TestTaskRepository_ListAnnotations_Pagination(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewTaskRepository(db)
	wsID := "ws-1"
	taskUUID := seedTaskWithAnnotations(t, repo, wsID, "proj-x", 7) // 写入 7 条注解

	got, total, err := repo.ListAnnotations(wsID, taskUUID, 3, 2) // offset=3 limit=2
	if err != nil {
		t.Fatal(err)
	}
	if total != 7 {
		t.Fatalf("total = %d, want 7", total)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}
```

`seedTaskWithAnnotations` 若无则新建辅助：创建 1 个 task + N 条 annotation（entry 递增）。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/storage/ -run TestTaskRepository_ListAnnotations_Pagination -v`
Expected: FAIL（`ListAnnotations` 未定义）。

- [ ] **Step 3: 实现 storage 层 ListAnnotations**

在 `internal/storage/task_repo.go` 的 `DeleteAnnotation` 之后新增：

```go
// ListAnnotations 按 entry 倒序分页返回任务注解，total 为该任务注解总数。
func (r *TaskRepository) ListAnnotations(workspaceID, taskUUID string, offset, limit int) ([]domain.Annotation, int, error) {
	var total int64
	if err := r.db.Model(&TaskAnnotation{}).
		Where("task_uuid = ? AND EXISTS (SELECT 1 FROM tasks WHERE tasks.uuid = task_annotations.task_uuid AND tasks.workspace_id = ?)", taskUUID, workspaceID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []TaskAnnotation
	if err := r.db.
		Where("task_uuid = ? AND EXISTS (SELECT 1 FROM tasks WHERE tasks.uuid = task_annotations.task_uuid AND tasks.workspace_id = ?)", taskUUID, workspaceID).
		Order("entry DESC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Annotation, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Annotation{ID: row.ID, Entry: row.Entry, Description: row.Description})
	}
	return out, int(total), nil
}
```

确认 `domain.Annotation` 字段名（ID/Entry/Description）与 `task.Annotation` 一致——storage 层用的 `domain` 别名即 `internal/task`。

- [ ] **Step 4: 运行 storage 测试通过**

Run: `go test ./internal/storage/ -run TestTaskRepository_ListAnnotations_Pagination -v`
Expected: PASS。

- [ ] **Step 5: 写失败测试 — app 层**

在 `internal/app/service_test.go` 追加（参照文件内现有 service 测试装配）：

```go
func TestService_ListAnnotations(t *testing.T) {
	svc := newTestService(t) // 沿用文件内现有构造
	tasks, err := svc.Add(task.Task{Description: "t", Project: "proj-x"})
	if err != nil {
		t.Fatal(err)
	}
	uuid := tasks[0].UUID
	for i := 0; i < 5; i++ {
		if err := svc.Annotate(uuid, fmt.Sprintf("note-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	got, total, err := svc.ListAnnotations(uuid, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(got) != 3 {
		t.Fatalf("total=%d len=%d", total, len(got))
	}
}
```

（`newTestService`、`svc.Add` 名称按文件内现有实际调整。）

- [ ] **Step 6: 运行确认失败**

Run: `go test ./internal/app/ -run TestService_ListAnnotations -v`
Expected: FAIL。

- [ ] **Step 7: 实现 app 层 ListAnnotations**

在 `internal/app/service.go` 的 `Annotate` 附近新增。注意这是**读操作**，只需 `PermissionTaskRead`：

```go
// ListAnnotations 按 entry 倒序分页返回任务注解。
func (s *Service) ListAnnotations(target string, offset, limit int) ([]task.Annotation, int, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return nil, 0, err
	}
	tsk, err := s.resolveTargetForRead(target)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListAnnotations(s.workspaceID, tsk.UUID, offset, limit)
}
```

确认 `resolveTargetForRead` 方法名（在 service.go 内 grep）；若读路径用别的名（如 `resolveTarget`），用实际的。

- [ ] **Step 8: 运行 app 测试通过**

Run: `go test ./internal/app/ -run TestService_ListAnnotations -v`
Expected: PASS。

- [ ] **Step 9: 注册路由 + 写 handler**

在 `internal/httpapi/router.go` 找到现有 tasks 路由块（`POST .../annotations`、`DELETE .../annotations/{annotationID}` 旁），新增：

```go
r.Get("/{taskRef}/annotations", s.handleTaskAnnotationList)
```

在 `internal/httpapi/tasks.go` 的 `handleTaskDenotate` 附近新增：

```go
func (s *Server) handleTaskAnnotationList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	taskRef := chi.URLParam(r, "taskRef")
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "api_bad_offset", "invalid offset", nil)
			return
		}
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > 100 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "limit must be 1..100", nil)
			return
		}
	}
	annotations, total, err := scoped.ListAnnotations(taskRef, offset, limit)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{
		"annotations": task.AnnotationsToJSON(annotations),
		"total":       total,
		"offset":      offset,
		"limit":       limit,
	}, nil)
}
```

确认 `task.AnnotationsToJSON` 是否存在；若没有，在 `internal/task/json.go` 新增（参照现有 json.go:254-261 的注解序列化逻辑抽出）：

```go
func AnnotationsToJSON(in []Annotation) []JSONAnnotation {
	out := make([]JSONAnnotation, 0, len(in))
	for _, a := range in {
		out = append(out, JSONAnnotation{ID: a.ID, Entry: formatUnix(a.Entry), Description: a.Description})
	}
	return out
}
```

（`formatUnix` 已存在于 json.go:491；`JSONAnnotation.ID` 是 `string` 类型。）

- [ ] **Step 10: 写 handler 单测**

`internal/httpapi/tasks_test.go` 追加：seed 5 条注解，请求 `/api/v1/tasks/{ref}/annotations?offset=2&limit=2`，断言 `total=5`、`len(annotations)=2`、body 含 `total/offset/limit` 字段。

- [ ] **Step 11: 运行全部相关测试**

Run: `go test ./internal/... -v`
Expected: PASS。

- [ ] **Step 12: 提交**

```bash
git add internal/storage/task_repo.go internal/storage/task_repo_test.go internal/app/service.go internal/app/service_test.go internal/httpapi/router.go internal/httpapi/tasks.go internal/httpapi/tasks_test.go internal/task/json.go
git commit -m "feat(httpapi): 新增 GET /tasks/{ref}/annotations 分页列表端点"
```

---

## Task 3: 后端 — projects 列表聚合统计

**Files:**
- Modify: `internal/storage/project_repo.go`（新增 `TaskStatusCounts`，替换调用点）
- Modify: `internal/app/project.go`（`ProjectView` 加字段 + 装配）
- Modify: `internal/httpapi/projects.go`（`projectResponse` 加字段）
- Test: 三个对应 `_test.go`

- [ ] **Step 1: 写失败测试 — storage 层**

`internal/storage/project_repo_test.go` 追加：

```go
func TestProjectRepository_TaskStatusCounts(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewProjectRepository(db)
	wsID := "ws-1"
	proj := seedProjectRow(t, db, wsID, "proj-a")
	seedTaskRow(t, db, wsID, proj.ID, task.StatusPending)
	seedTaskRow(t, db, wsID, proj.ID, task.StatusPending)
	seedTaskRow(t, db, wsID, proj.ID, task.StatusCompleted)
	seedTaskRow(t, db, wsID, proj.ID, task.StatusDeleted) // 不计入

	counts, err := repo.TaskStatusCounts(wsID, []string{proj.ID})
	if err != nil {
		t.Fatal(err)
	}
	c := counts[proj.ID]
	if c.Total != 3 || c.Pending != 2 || c.Completed != 1 {
		t.Fatalf("got %+v, want total=3 pending=2 completed=1", c)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/storage/ -run TestProjectRepository_TaskStatusCounts -v`
Expected: FAIL。

- [ ] **Step 3: 实现 TaskStatusCounts**

在 `internal/storage/project_repo.go` 的 `TaskCounts` 之后新增（保留旧 `TaskCounts` 供 `ProjectInfo` 单项目路径继续用，或也让它内部转调新方法）：

```go
type ProjectTaskCounts struct {
	Total     int
	Pending   int
	Completed int
}

// TaskStatusCounts 按 project 分组返回任务状态分项计数（不含 deleted）。
func (r *ProjectRepository) TaskStatusCounts(workspaceID string, projectIDs []string) (map[string]ProjectTaskCounts, error) {
	out := make(map[string]ProjectTaskCounts, len(projectIDs))
	if len(projectIDs) == 0 {
		return out, nil
	}
	type row struct {
		ProjectID string
		Status    string
		Count     int
	}
	var rows []row
	if err := r.db.Model(&Task{}).
		Select("project_id, status, COUNT(*) AS count").
		Where("workspace_id = ? AND project_id IN ? AND status <> ?", workspaceID, projectIDs, domain.StatusDeleted).
		Group("project_id, status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		c := out[r.ProjectID]
		c.Total += r.Count
		switch r.Status {
		case domain.StatusPending:
			c.Pending += r.Count
		case domain.StatusCompleted:
			c.Completed += r.Count
		}
		out[r.ProjectID] = c
	}
	return out, nil
}
```

- [ ] **Step 4: 运行 storage 测试通过**

Run: `go test ./internal/storage/ -run TestProjectRepository_TaskStatusCounts -v`
Expected: PASS。

- [ ] **Step 5: 扩展 ProjectView 与装配**

`internal/app/project.go`：
- `ProjectView` 结构体（L37-49）在 `TaskCount` 后加 `PendingCount int` / `CompletedCount int`。
- `ListProjectsByStatus`（L91-118）：把 `counts, err := s.projectRepo.TaskCounts(...)` 换成 `counts, err := s.projectRepo.TaskStatusCounts(...)`，并改 `projectViewFromRow` 签名为接收 `storage.ProjectTaskCounts`：

```go
func projectViewFromRow(project storage.Project, counts storage.ProjectTaskCounts) ProjectView {
	return ProjectView{
		// ...原有字段...
		TaskCount:      counts.Total,
		PendingCount:   counts.Pending,
		CompletedCount: counts.Completed,
	}
}
```

- **改签名后必须更新全部 6 个调用点**（grep `projectViewFromRow` 确认）：
  - `internal/app/project.go:80`（AddProject，counts 为零值 `storage.ProjectTaskCounts{}`）
  - `internal/app/project.go:115`（ListProjectsByStatus，用新分项 map）
  - `internal/app/project.go:303`（ProjectInfo，单项目改用 `TaskStatusCounts(wsID, []string{id})`）
  - `internal/app/project.go:415`、`internal/app/project.go:503`（归档/转换等路径，counts 为零值）
  - 旧 `TaskCounts` 方法：可保留（其它处可能用）或删除（若 grep 确认仅此处用）。**改前先 grep 全仓 `\.TaskCounts(` 确认无其它调用方再删。**

- [ ] **Step 6: 扩展 projectResponse**

`internal/httpapi/projects.go` 的 `projectResponse`（L41-53）在 `TaskCount` 后加：

```go
	PendingCount   int `json:"pending_count"`
	CompletedCount int `json:"completed_count"`
```

`projectResponseFromView`（L253 附近）补：

```go
	PendingCount:   view.PendingCount,
	CompletedCount: view.CompletedCount,
```

- [ ] **Step 7: 写 app + httpapi 测试**

- `internal/app/project_test.go`（或 service_test.go，按文件现状）：建项目 + seed 不同 status 任务，断言 `ListProjects` 返回的 view 分项正确。
- `internal/httpapi/projects_test.go`：请求 `/api/v1/projects`，断言 body 每项含 `task_count`/`pending_count`/`completed_count`。

- [ ] **Step 8: 运行全部后端测试 + 零 CGO 构建**

Run:
```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全 PASS、构建成功。

- [ ] **Step 9: 提交**

```bash
git add internal/storage/project_repo.go internal/storage/project_repo_test.go internal/app/project.go internal/app/project_test.go internal/httpapi/projects.go internal/httpapi/projects_test.go
git commit -m "feat(httpapi): projects 列表返回 pending/completed 任务分项计数"
```

---

## Task 4: 前端 — projects-list domain 分层（api + page）

新建项目列表页的 domain，复用现有分层模式。引入行点击（扩展 DataTable）。

**Files:**
- Create: `web/src/features/workspace/projects/projects-api.ts`
- Create: `web/src/features/workspace/projects/projects-list-page.tsx`
- Create: `web/src/features/workspace/projects/projects-list-page.test.tsx`
- Modify: `web/src/components/DataTable.tsx`（加 `onRowClick` 可选 prop）

- [ ] **Step 1: 写 projects-api.ts**

```typescript
import { workspaceApiGet } from "@/features/workspace/session/workspace-api";

export interface ProjectSummary {
  id: string;
  slug: string;
  name: string;
  status: string;
  task_count: number;
  pending_count: number;
  completed_count: number;
}

export function projectsListPath(): string {
  return "/api/v1/projects";
}

export function getProjects(): Promise<ProjectSummary[]> {
  return workspaceApiGet<ProjectSummary[]>(projectsListPath());
}
```

- [ ] **Step 2: 扩展 DataTable 支持 onRowClick**

`web/src/components/DataTable.tsx`：props 加 `onRowClick?: (row: T) => void`；在 `<tr>` 上加 `onClick={() => onRowClick?.(row)}`，并加 `cursor-pointer` class（当 onRowClick 存在时）。同步更新该文件的 props 类型导出。

- [ ] **Step 3: 写 projects-list-page.tsx（失败状态先骨架）**

```typescript
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { DataTable, type Column } from "@/components/DataTable";
import { Badge } from "@/components/ui/badge";
import { getProjects, type ProjectSummary } from "./projects-api";

export function ProjectsListPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: projects, isPending, isError } = useQuery({
    queryKey: ["projects", workspaceSlug],
    queryFn: getProjects,
  });

  if (isPending) return <div className="p-6 text-muted-foreground">{t("common.loading")}</div>;
  if (isError) return <div className="p-6 text-destructive">{t("common.error")}</div>;

  const columns: Column<ProjectSummary>[] = [
    { key: "name", header: t("projects.colName"), render: (p) => p.name || p.slug },
    {
      key: "status",
      header: t("projects.colStatus"),
      render: (p) => <Badge variant="outline">{p.status}</Badge>,
    },
    {
      key: "progress",
      header: t("projects.colProgress"),
      render: (p) => {
        const pct = p.task_count > 0 ? Math.round((p.completed_count / p.task_count) * 100) : 0;
        return (
          <div className="flex items-center gap-2">
            <div className="h-1.5 w-16 rounded-full bg-muted overflow-hidden">
              <div className="h-full bg-primary" style={{ width: `${pct}%` }} />
            </div>
            <span className="text-xs text-muted-foreground">{pct}%</span>
          </div>
        );
      },
    },
    { key: "task_count", header: t("projects.colTaskCount"), render: (p) => `${p.task_count}` },
  ];

  return (
    <div className="p-6 space-y-4">
      <h1 className="text-xl font-semibold">{t("projects.title")}</h1>
      <DataTable
        rows={projects}
        columns={columns}
        empty={t("projects.empty")}
        onRowClick={(p) =>
          navigate({ to: "/workspaces/$workspaceSlug/projects/$projectSlug", params: { workspaceSlug, projectSlug: p.slug } })
        }
      />
    </div>
  );
}
```

- [ ] **Step 4: 补 i18n key**

`web/src/locales/zh-CN.json`（及 en-US.json）的对应 namespace 加：`projects.title`、`projects.colName`、`projects.colStatus`、`projects.colProgress`、`projects.colTaskCount`、`projects.empty`。（参照文件内现有 key 结构。）

- [ ] **Step 5: 写组件测试**

`web/src/features/workspace/projects/projects-list-page.test.tsx`，照搬 `project-page.test.tsx` 的 wrapper（QueryClientProvider + ThemeProvider + renderWithRouter）+ fetch mock 返回 `{data:[{...}]}`。断言：项目名渲染、行点击触发导航（mock `useNavigate` 或断言 `<tr>` 存在）。

- [ ] **Step 6: 运行测试**

Run（在 web/ 下）: `pnpm vitest run src/features/workspace/projects`
Expected: PASS（先确认 fail→pass 循环：先跑见失败，再补实现）。

- [ ] **Step 7: 提交**

```bash
git add web/src/features/workspace/projects web/src/components/DataTable.tsx web/src/locales
git commit -m "feat(web): 新增 projects-list 页面 domain（api + 表格 + 行点击）"
```

---

## Task 5: 前端 — 路由与导航打通

把新 projects-list 接入路由，移除 `/tasks` 入口，projects 列表行点击进项目详情。

**Files:**
- Create: `web/src/routes/workspace/ProjectsListRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/components/AppShell.tsx`

- [ ] **Step 1: 写 ProjectsListRoute（极薄）**

```typescript
import { useParams } from "@tanstack/react-router";
import { ProjectsListPage } from "@/features/workspace/projects/projects-list-page";

export function ProjectsListRoute() {
  const { workspaceSlug } = useParams({ strict: false });
  return <ProjectsListPage workspaceSlug={workspaceSlug as string} />;
}
```

- [ ] **Step 2: 改 router.tsx**

- 把 `/projects` 路由从 `createResourceRoute("projects", ...)` 换成新的 `createRoute`，component `lazyRoute(ProjectsListRoute)`。
- 移除 `/tasks` 路由（或改为重定向到 `/projects`：加一个 beforeLoad `throw redirect({ to: "/projects" })`）。
- 添加 `ProjectsListRoute` 到 lazy import 列表。

- [ ] **Step 3: 改 AppShell.tsx**

- `navItems`（L38-53）移除 `tasks` 项。
- `PageKey` 类型（L26-36）移除 `"tasks"`。
- 保留 `projects`，`to` 指向 `/projects`（已是）。

- [ ] **Step 4: 更新 AppShell 测试**

`web/src/components/AppShell.test.tsx`：移除针对 tasks 导航项的断言，确认 projects 项仍在。

- [ ] **Step 5: 运行前端测试 + 构建**

Run（在 web/ 下）:
```bash
pnpm vitest run
pnpm build
```
Expected: 测试 PASS、构建产物输出（`dist/`）。构建产物会被 go:embed。

- [ ] **Step 6: 提交**

```bash
git add web/src/routes web/src/components/AppShell.tsx
git commit -m "feat(web): projects 列表接入主路由，移除 /tasks 侧边栏入口"
```

---

## Task 6: 前端 — 任务详情页增强（links / UDAs / 注解懒加载）

**Files:**
- Modify: `web/src/features/workspace/project-readonly/project-readonly-api.ts`（扩展类型 + 注解分页函数）
- Create: `web/src/features/workspace/project-readonly/task-detail/TaskLinks.tsx`
- Create: `web/src/features/workspace/project-readonly/task-detail/TaskSidePanel.tsx`
- Create: `web/src/features/workspace/project-readonly/task-detail/TaskAnnotationsLazy.tsx`
- Create: `web/src/features/workspace/project-readonly/task-detail/uda.ts`（纯函数：识别 UDA 字段）
- Modify: `web/src/features/workspace/project-readonly/project-task-detail-page.tsx`

- [ ] **Step 1: 扩展 api 类型与注解分页函数**

`project-readonly-api.ts`：
- `ProjectReadonlyTask` 补字段：`entry?: string`、`modified?: string`、`recur?: string`、`links?: TaskLink[]`。
- 新增类型：

```typescript
export interface TaskLink {
  id: string;
  type: string;
  url: string;
  title?: string;
  created_at: string;
  created_by: { id: string; name: string; email?: string };
}
export interface AnnotationPage {
  annotations: Array<{ id?: string; entry: string; description: string }>;
  total: number;
  offset: number;
  limit: number;
}
export function projectReadonlyAnnotationsPath(workspaceSlug: string, projectSlug: string, taskRef: string, offset: number, limit: number): string {
  return `/api/v1/tasks/${encodeURIComponent(taskRef)}/annotations?workspace=${encodeURIComponent(workspaceSlug)}&offset=${offset}&limit=${limit}`;
}
export function getProjectReadonlyAnnotations(workspaceSlug: string, projectSlug: string, taskRef: string, offset: number, limit: number): Promise<AnnotationPage> {
  return workspaceApiGet<AnnotationPage>(projectReadonlyAnnotationsPath(workspaceSlug, projectSlug, taskRef, offset, limit));
}
```

- [ ] **Step 2: 写 uda.ts 纯函数 + 单测**

`uda.ts`：识别 JSONTask 顶层非保留字段为 UDA。

```typescript
const RESERVED = new Set([
  "uuid","description","status","entry","modified","end","due","project","task_slug",
  "priority","tags","start","wait","scheduled","until","annotations","depends",
  "recur","parent","mask","imask","assignees","links","project_id","project_seq",
]);
export function extractUDAs(task: Record<string, unknown>): Array<[string, unknown]> {
  return Object.entries(task).filter(([k, v]) => !RESERVED.has(k) && v !== undefined && v !== null && v !== "");
}
```

`uda.test.ts`：输入含 `estimate`/`sprint`/标准字段的对象，断言只返回 estimate/sprint。

- [ ] **Step 3: 写 TaskLinks 组件**

```typescript
export function TaskLinks({ links }: { links: TaskLink[] | undefined }) {
  if (!links || links.length === 0) return null;
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-medium text-muted-foreground">{t("task.links")}</h3>
      <ul className="space-y-1">
        {links.map((l) => (
          <li key={l.id} className="text-sm">
            <a href={l.url} target="_blank" rel="noreferrer" className="text-primary hover:underline">
              {l.title || l.url}
            </a>
            <span className="text-muted-foreground"> · {l.type} · {l.created_by.name}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
```

- [ ] **Step 4: 写 TaskSidePanel 组件**（右侧属性栏 + UDAs）

用 `extractUDAs` 渲染常用字段 + UDA 列表。props 接整个 task 对象。

- [ ] **Step 5: 写 TaskAnnotationsLazy 组件**

```typescript
export function TaskAnnotationsLazy({ task, workspaceSlug, projectSlug, taskRef }: Props) {
  const { t } = useTranslation();
  const initial = (task.annotations ?? []).slice(0, 3);
  const total = task.annotations?.length ?? 0;
  const [shown, setShown] = useState(initial);
  const [offset, setOffset] = useState(initial.length);
  const [loading, setLoading] = useState(false);

  const loadMore = async () => {
    setLoading(true);
    const page = await getProjectReadonlyAnnotations(workspaceSlug, projectSlug, taskRef, offset, 10);
    setShown((s) => [...s, ...page.annotations]);
    setOffset((o) => o + page.annotations.length);
    setLoading(false);
  };
  const hasMore = offset < total;

  return (
    <section className="space-y-2">
      <h3 className="text-sm font-medium text-muted-foreground">
        {t("task.annotations")} ({shown.length} / {total})
      </h3>
      <ul className="space-y-1">
        {shown.map((a, i) => (
          <li key={a.id ?? i} className="rounded bg-muted/40 p-2 text-sm">
            <div>{a.description}</div>
            <div className="text-xs text-muted-foreground">{a.entry}</div>
          </li>
        ))}
      </ul>
      {hasMore && (
        <button onClick={loadMore} disabled={loading} className="text-sm text-primary hover:underline">
          {loading ? t("common.loading") : t("task.loadMore", { n: total - shown.length })}
        </button>
      )}
    </section>
  );
}
```

- [ ] **Step 6: 重构 project-task-detail-page.tsx**

改为左右布局：左侧 `<TaskAnnotationsLazy>` + `<TaskLinks>` + 描述；右侧 `<TaskSidePanel>`。保留面包屑、标题、徽章、taskBelongsToProject 校验。删除旧的 TaskFields/TaskAnnotations 内联渲染（或迁移到子组件）。

- [ ] **Step 7: 写/更新测试**

`project-task-detail-page.test.tsx`：mock 任务含 links + 注解 5 条，断言显示 3 条初始 + "查看更多 2 条" + 点击后 fetch `/annotations?offset=3&limit=10` 被调用、注解增加。

- [ ] **Step 8: 运行测试**

Run（在 web/ 下）: `pnpm vitest run src/features/workspace/project-readonly`
Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add web/src/features/workspace/project-readonly
git commit -m "feat(web): 任务详情页增强——links/UDAs/注解懒加载，左右布局"
```

---

## Task 7: 前端 — 过滤工具栏 + URL 同步

引入 TanStack Router search schema，在项目详情页任务表格上方加过滤工具栏，URL 双向同步。

**Files:**
- Modify: `web/src/routes/router.tsx`（给 project detail route 加 `validateSearch`）
- Create: `web/src/features/workspace/project-readonly/project-filter.ts`（纯函数：filter ↔ query string 编解码）
- Create: `web/src/features/workspace/project-readonly/project-filter.test.ts`
- Create: `web/src/features/workspace/project-readonly/project-filter-toolbar.tsx`
- Modify: `web/src/features/workspace/project-readonly/project-task-list.tsx`（接 filter 参数）

- [ ] **Step 1: 写 project-filter.ts 纯函数 + 单测**

```typescript
export interface TaskFilter {
  status?: string;
  priority?: string;
  assignee?: string;
  due_after?: string;
  due_before?: string;
  tags?: string;
  q?: string;
}
export function filterToQuery(f: TaskFilter): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) p.set(k, v);
  const s = p.toString();
  return s ? `&${s}` : "";
}
export function emptyFilter(f: TaskFilter): boolean {
  return Object.values(f).every((v) => !v);
}
```

测试：`filterToQuery({status:"pending", q:"x"})` → 包含 `status=pending&q=x`；`emptyFilter({})` true。

- [ ] **Step 2: 路由加 search schema**

router.tsx 的 `projectReadonlyRoute` 加：

```typescript
validateSearch: (search: Record<string, unknown>): TaskFilter => ({
  status: typeof search.status === "string" ? search.status : undefined,
  priority: typeof search.priority === "string" ? search.priority : undefined,
  assignee: typeof search.assignee === "string" ? search.assignee : undefined,
  due_after: typeof search.due_after === "string" ? search.due_after : undefined,
  due_before: typeof search.due_before === "string" ? search.due_before : undefined,
  tags: typeof search.tags === "string" ? search.tags : undefined,
  q: typeof search.q === "string" ? search.q : undefined,
}),
```

- [ ] **Step 3: ProjectReadonlyPage 接 useSearch**

`project-page.tsx`：`const filter = useSearch({ strict: false }) as TaskFilter;` 把 `filterToQuery(filter)` 拼进 `getProjectReadonlyTasks` 的 path（path 已含 `project=`，追加 `&status=...`）。

- [ ] **Step 4: 写 project-filter-toolbar.tsx**

下拉用 shadcn `<Select>`，日期用原生 input[type=date]（或 shadcn calendar，先原生降复杂度），搜索用 `<Input>`。每个控件 onChange 调 `navigate({ to, search: (prev)=>({...prev, status: v}) })` 更新 URL。活跃 chips 从 filter 派生，✕ 调 navigate 清单个字段；"清除全部" navigate 到空 filter。

- [ ] **Step 5: 写组件测试**

mock useSearch/useNavigate，断言：选 status=pending 后 navigate 被调且 search 含 status；点 chip ✕ 后对应字段被清空。

- [ ] **Step 6: 运行测试 + 构建**

Run（在 web/ 下）: `pnpm vitest run && pnpm build`
Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add web/src/routes/router.tsx web/src/features/workspace/project-readonly
git commit -m "feat(web): 项目详情页任务过滤工具栏 + URL 双向同步"
```

---

## Task 8: 端到端验证与文档同步

- [ ] **Step 1: 全量后端验证**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 2: 全量前端验证**

```bash
cd web && pnpm vitest run && pnpm build
```

- [ ] **Step 3: 手动冒烟（集成测试已有 CLI 黑盒，这里补 web 路径）**

构建后启动 server，浏览器走：`/projects` → 点项目 → 看任务表格 + 设过滤（刷新保留）→ 点任务 → 看详情（links/UDAs/注解查看更多）。

- [ ] **Step 4: 更新文档**

- `README.md`：若 web console 用户可见行为变化（导航移除 Tasks），补一句说明。
- `ROADMAP.md`：标记本 milestone 进度。

- [ ] **Step 5: 提交**

```bash
git add README.md ROADMAP.md
git commit -m "docs: 同步 web console 项目-任务浏览体验重构"
```
