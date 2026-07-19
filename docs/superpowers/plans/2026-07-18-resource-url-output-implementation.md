# 资源 URL 统一输出 Implementation Plan

> **2026-07-19 更新：** 本计划记录最初相对路径实现；当前绝对 URL 与空配置契约由 `2026-07-19-absolute-resource-url-output-implementation.md` 接续并取代 URL 形态相关步骤。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让项目、任务、projected/materialized occurrence 和循环系列经 HTTP、MCP、CLI local、CLI remote 返回一致、可打开的 Web Console 相对 URL。

**Architecture:** `internal/app` 新增唯一的资源 URL 构造器，并在 `ProjectView`、`TaskOccurrenceView`、`TaskSeriesView` 装配时填入非空 `URL`。HTTP、MCP、CLI 和 Remote DTO 只透传该字段；Huma/OpenAPI 与 Web TypeScript 类型同步声明必填 `url`，跨协议 E2E 负责证明同一资源的路径完全一致。

**Tech Stack:** Go 1.25、Cobra、Huma/OpenAPI、MCP Go SDK、React/TypeScript、Vitest、Playwright、GORM、`github.com/glebarez/sqlite`、PostgreSQL/pgx。

## Global Constraints

- URL 是以 `/` 开头的相对路径，不包含 scheme、host、端口、部署前缀，也不新增 `web_base_url` 配置。
- 项目 URL 固定为 `/workspaces/{workspaceSlug}/projects/{projectSlug}`。
- 项目任务 URL 固定为 `/workspaces/{workspaceSlug}/projects/{projectSlug}/tasks/{taskRef}`；普通任务和 materialized occurrence 使用 `task_slug`，projected occurrence 使用编码后的 `occurrence_ref`。
- 无项目任务 URL 固定为 `/tasks/{uuid}`。
- 循环系列 URL 固定为 `/workspaces/{workspaceSlug}/projects/{projectSlug}/series/{seriesSlug}`；不得新增 `/tasks/{taskSlug}/series/{seriesSlug}`。
- URL 只在读取时派生，不写数据库，不进入 `task.JSONTask` import/export，也不得为了生成 URL 物化 projected occurrence。
- 动态 path segment 使用 RFC 3986 unreserved 规则编码；至少保证 occurrence ref 的 `:` 输出为 `%3A`。
- `internal/cli` 不实现 URL 业务规则，各输出层不得自行拼路径。
- 文档和代码注释以中文为主。
- SQLite 继续使用 `github.com/glebarez/sqlite`，不得引入 CGO SQLite；必须通过 `CGO_ENABLED=0` 测试与构建。
- 保留当前未跟踪的 `docs/business/`，不得纳入任何提交。

---

## 文件结构

- 新建 `internal/app/resource_url.go`：唯一的 path segment 编码与四类 Web Console URL 纯函数。
- 新建 `internal/app/resource_url_test.go`：锁定 URL 字符串、特殊字符编码和 projected occurrence 路径。
- 修改 `internal/app/project.go`：`ProjectView.URL` 与项目 view 装配。
- 修改 `internal/app/task_occurrence.go`、`internal/app/task_reference.go`、`internal/app/task_series.go`：`TaskOccurrenceView.URL`、`TaskSeriesView.URL` 及全部构造调用点。
- 修改 `internal/httpapi/projects.go`、`internal/httpapi/task_series.go`、`internal/httpapi/huma_routes.go`：HTTP DTO 与 OpenAPI 透传。
- 修改 `internal/remote/project.go`、`internal/remote/task_series.go`：Remote DTO 透传。
- 修改 `internal/cli/project.go`、`internal/cli/add.go`、`internal/cli/info.go`、`internal/cli/list.go`、`internal/cli/series.go`：JSON 与人类可读输出透传。
- 修改 `internal/mcpserver/tools_views.go`、`internal/mcpserver/tools_task_series.go`：MCP structured data 透传；`content[0].text` 继续承载同一个 JSON envelope。
- 修改 `web/src/features/workspace/projects/projects-api.ts`、`web/src/features/workspace/project-workbench/api/project-api.ts`、`web/src/features/workspace/project-workbench/api/task-series-api.ts`：前端 view 类型声明必填 `url`。
- 修改 `tests/integration/e2e_mcp_test.go`、`tests/integration/cli_test.go`：跨协议和 CLI 端到端验收。
- 修改 `README.md`：记录资源响应的 `url` 契约；`ROADMAP.md` 不变，因为本次不调整 milestone 或产品边界。

---

### Task 1: 在 App 层建立唯一 URL 契约并填满三个 view

**Files:**
- Create: `internal/app/resource_url.go`
- Create: `internal/app/resource_url_test.go`
- Modify: `internal/app/project.go:39-53,113-165,348-384`
- Modify: `internal/app/task_occurrence.go:59-91,152-275,478-531,581-626,1090-1165`
- Modify: `internal/app/task_reference.go:107-132`
- Modify: `internal/app/task_series.go:45-56,258-329,724-765,1248-1350`
- Test: `internal/app/task_occurrence_test.go:33-64,334-390,618-650,685-875`

**Interfaces:**
- Consumes: `RuntimeContext.WorkspaceSlug`、`task.Task.Project`、`task.Task.ProjectSeq`、`taskseries.Series.ProjectSlug`、`SeriesSlugOf`、`OccurrenceRef`。
- Produces: `ProjectURL(workspaceSlug, projectSlug string) string`、`ProjectTaskURL(workspaceSlug, projectSlug, taskRef string) string`、`StandaloneTaskURL(taskRef string) string`、`TaskSeriesURL(workspaceSlug, projectSlug, seriesSlug string) string`，以及始终由真实 Service 填充的 `ProjectView.URL`、`TaskOccurrenceView.URL`、`TaskSeriesView.URL`。

- [ ] **Step 1: 写 URL 纯函数的失败测试**

创建 `internal/app/resource_url_test.go`：

```go
package app

import "testing"

func TestResourceURLs(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"project", ProjectURL("dajee", "agentapi"), "/workspaces/dajee/projects/agentapi"},
		{"project task", ProjectTaskURL("dajee", "agentapi", "agentapi-17"), "/workspaces/dajee/projects/agentapi/tasks/agentapi-17"},
		{"projected occurrence", ProjectTaskURL("dajee", "agentapi", "occ:series-1:1784303999"), "/workspaces/dajee/projects/agentapi/tasks/occ%3Aseries-1%3A1784303999"},
		{"standalone task", StandaloneTaskURL("7b4d901e-5e4f-4cfa-b42c-38fc7089880a"), "/tasks/7b4d901e-5e4f-4cfa-b42c-38fc7089880a"},
		{"series", TaskSeriesURL("dajee", "agentapi", "agentapi-s-3"), "/workspaces/dajee/projects/agentapi/series/agentapi-s-3"},
		{"encoded segments", ProjectTaskURL("workspace/name", "project?name", "task#1"), "/workspaces/workspace%2Fname/projects/project%3Fname/tasks/task%231"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("URL = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试，确认因构造函数不存在而失败**

Run: `go test ./internal/app -run TestResourceURLs -count=1`

Expected: FAIL，编译错误包含 `undefined: ProjectURL`。

- [ ] **Step 3: 实现唯一 URL 构造器**

创建 `internal/app/resource_url.go`：

```go
package app

import (
	"net/url"
	"strings"
)

func webPathSegment(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func ProjectURL(workspaceSlug, projectSlug string) string {
	return "/workspaces/" + webPathSegment(workspaceSlug) + "/projects/" + webPathSegment(projectSlug)
}

func ProjectTaskURL(workspaceSlug, projectSlug, taskRef string) string {
	return ProjectURL(workspaceSlug, projectSlug) + "/tasks/" + webPathSegment(taskRef)
}

func StandaloneTaskURL(taskRef string) string {
	return "/tasks/" + webPathSegment(taskRef)
}

func TaskSeriesURL(workspaceSlug, projectSlug, seriesSlug string) string {
	return ProjectURL(workspaceSlug, projectSlug) + "/series/" + webPathSegment(seriesSlug)
}
```

- [ ] **Step 4: 运行纯函数测试，确认通过**

Run: `go test ./internal/app -run TestResourceURLs -count=1`

Expected: PASS。

- [ ] **Step 5: 写真实 App view 的失败测试**

在 `internal/app/task_occurrence_test.go` 增加以下断言，并在现有 `TestAddTaskViewReturnsUnifiedNormalTaskView`、`TestProjectedOccurrenceViewDoesNotAllocateIdentity`、`TestGetTaskViewProjectedDoesNotWrite`、`TestListTaskSeriesReturnsCreated` 中补相同语义：

```go
func TestAppViewsExposeCanonicalURLs(t *testing.T) {
	svc, closeFn := newTestService(t, 1_750_000_000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if project.URL != "/workspaces/local/projects/ops" {
		t.Fatalf("project URL = %q", project.URL)
	}

	projectRef := project.Slug
	created, err := svc.AddTaskView(AddInput{Title: "project task", Project: &projectRef})
	if err != nil {
		t.Fatal(err)
	}
	if created.URL != "/workspaces/local/projects/ops/tasks/ops-1" {
		t.Fatalf("task URL = %q", created.URL)
	}

	standalone, err := svc.AddTaskView(AddInput{Title: "standalone"})
	if err != nil {
		t.Fatal(err)
	}
	if standalone.URL != StandaloneTaskURL(*standalone.UUID) {
		t.Fatalf("standalone URL = %q", standalone.URL)
	}
}
```

Projected occurrence 的现有测试必须增加：

```go
if view.Project == nil || *view.Project != "ops" {
	t.Fatalf("projected project = %#v", view.Project)
}
wantURL := ProjectTaskURL("local", "ops", view.ID)
if view.URL != wantURL {
	t.Fatalf("projected URL = %q, want %q", view.URL, wantURL)
}
```

Series 列表测试必须增加：

```go
if got.Items[0].URL != "/workspaces/local/projects/ops/series/ops-s-1" {
	t.Fatalf("series URL = %q", got.Items[0].URL)
}
```

- [ ] **Step 6: 运行 App 测试，确认新 `URL` 字段不存在或为空而失败**

Run: `go test ./internal/app -run 'Test(AppViewsExposeCanonicalURLs|ProjectedOccurrenceViewDoesNotAllocateIdentity|GetTaskViewProjectedDoesNotWrite|ListTaskSeriesReturnsCreated)' -count=1`

Expected: FAIL，编译错误包含 `URL undefined`，或断言显示 `URL = ""`。

- [ ] **Step 7: 给 view 和全部 App 构造路径填充 URL**

修改三个 view：

```go
type ProjectView struct {
	URL string
	// existing fields stay unchanged
}

type TaskOccurrenceView struct {
	URL string
	// existing fields stay unchanged
}

type TaskSeriesView struct {
	taskseries.Series
	URL string
	// existing fields stay unchanged
}
```

将项目 mapper 改为显式接收 workspace slug，并更新全部调用点：

```go
func projectViewFromRow(workspaceSlug string, project storage.Project, counts storage.ProjectTaskCounts) ProjectView {
	return ProjectView{
		URL:         ProjectURL(workspaceSlug, project.Slug),
		ID:          project.ID,
		WorkspaceID: project.WorkspaceID,
		Slug:        project.Slug,
		// copy every existing field unchanged
	}
}
```

将 task mapper 改为接收 workspace slug；在既有 `TaskSlug` 派生完成后设置 URL：

```go
func taskToView(workspaceSlug string, tsk domain.Task, assignees []domain.UserInfo) TaskOccurrenceView {
	view := TaskOccurrenceView{/* existing field copies */}
	// keep the existing TaskSlug and RecurrenceInfo assembly
	if view.Project != nil && view.TaskSlug != nil {
		view.URL = ProjectTaskURL(workspaceSlug, *view.Project, *view.TaskSlug)
	} else {
		view.URL = StandaloneTaskURL(tsk.UUID)
	}
	return view
}
```

将 projected mapper 改为接收 workspace slug、回填项目 slug，并用 occurrence ref：

```go
func projectedOccurrenceView(workspaceSlug string, series taskseries.Series, slot taskseries.Slot, assignees []domain.UserInfo) TaskOccurrenceView {
	ref := OccurrenceRef(series.ID, slot.RecurrenceAt)
	projectSlug := series.ProjectSlug
	return TaskOccurrenceView{
		URL:       ProjectTaskURL(workspaceSlug, projectSlug, ref),
		ID:        ref,
		Project:   &projectSlug,
		ProjectID: &series.ProjectID,
		// copy every existing projected field unchanged
	}
}
```

在 `buildSeriesViewFromSummary` 中先验证派生 slug，再填 URL：

```go
seriesSlug := SeriesSlugOf(series)
if strings.TrimSpace(series.ProjectSlug) == "" || strings.TrimSpace(seriesSlug) == "" {
	return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid", Message: "series project slug is unavailable"}
}
return TaskSeriesView{
	Series: series,
	URL:    TaskSeriesURL(s.runtime.WorkspaceSlug, series.ProjectSlug, seriesSlug),
	// copy every existing summary field unchanged
}, nil
```

更新 `task_occurrence.go`、`task_reference.go`、`task_series.go` 中所有 `taskToView` / `projectedOccurrenceView` 调用点，统一传 `s.runtime.WorkspaceSlug`；更新两个直接调用 mapper 的单元测试。

- [ ] **Step 8: 运行 App 包测试与零 CGO 测试**

Run: `gofmt -w internal/app/resource_url.go internal/app/resource_url_test.go internal/app/project.go internal/app/task_occurrence.go internal/app/task_reference.go internal/app/task_series.go internal/app/task_occurrence_test.go`

Run: `go test ./internal/app -count=1`

Run: `CGO_ENABLED=0 go test ./internal/app -count=1`

Expected: 全部 PASS。

- [ ] **Step 9: 提交 App 契约**

```bash
git add internal/app/resource_url.go internal/app/resource_url_test.go internal/app/project.go internal/app/task_occurrence.go internal/app/task_reference.go internal/app/task_series.go internal/app/task_occurrence_test.go
git commit -m "feat: 统一生成资源 URL"
```

---

### Task 2: 让 HTTP、OpenAPI 与 Web 类型透传 URL

**Files:**
- Modify: `internal/httpapi/projects.go:76-96,282-310`
- Modify: `internal/httpapi/projects_test.go:11-58`
- Modify: `internal/httpapi/task_series.go:38-139,154-245`
- Modify: `internal/httpapi/task_series_test.go:36-205,381-418,459-560`
- Modify: `internal/httpapi/huma_routes.go:300-550`
- Modify: `internal/httpapi/server_test.go:129-240`
- Modify: `internal/httpapi/home_test.go:132-150`
- Modify: `web/src/features/workspace/projects/projects-api.ts:4-13`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts:25-105`
- Modify: `web/src/features/workspace/project-workbench/api/task-series-api.ts:24-115`

**Interfaces:**
- Consumes: Task 1 的 `ProjectView.URL`、`TaskOccurrenceView.URL`、`TaskSeriesView.URL`。
- Produces: HTTP JSON 中必填 `url`，以及 OpenAPI/Web 类型中的 `url: string`。

- [ ] **Step 1: 写 HTTP DTO 的失败测试**

扩展 `projectListItem` 并在 `TestHandleProjectListReturnsStatusBreakdown` 断言：

```go
type projectListItem struct {
	Slug           string `json:"slug"`
	URL            string `json:"url"`
	TaskCount      int    `json:"task_count"`
	PendingCount   int    `json:"pending_count"`
	CompletedCount int    `json:"completed_count"`
}

if p.Slug == "api" && p.URL != "/workspaces/local/projects/api" {
	t.Fatalf("project URL = %q", p.URL)
}
```

在 `TestTaskHTTPGetsProjectedOccurrenceByEncodedReferenceWithoutMaterializing` 增加：

```go
wantURL := "/workspaces/local/projects/ops/tasks/" + url.QueryEscape(payload.Data.ID)
if payload.Data.URL != wantURL {
	t.Fatalf("projected URL = %q, want %q", payload.Data.URL, wantURL)
}
```

在 series create/get/list 测试分别断言：

```go
if payload.Data.Series.URL != "/workspaces/local/projects/ops/series/ops-s-1" {
	t.Fatalf("series URL = %q", payload.Data.Series.URL)
}
if payload.Data.FirstOccurrence == nil || payload.Data.FirstOccurrence.URL == "" {
	t.Fatalf("first occurrence URL missing: %#v", payload.Data.FirstOccurrence)
}
```

- [ ] **Step 2: 运行 HTTP 测试，确认 DTO 没有 `URL` 而失败**

Run: `go test ./internal/httpapi -run 'Test(HandleProjectListReturnsStatusBreakdown|TaskHTTPGetsProjectedOccurrenceByEncodedReferenceWithoutMaterializing|TaskSeriesHTTPCreateAndGet|TaskSeriesHTTPList)' -count=1`

Expected: FAIL，编译错误显示 `payload.Data.URL undefined` 或断言为空。

- [ ] **Step 3: 在 HTTP DTO 与 mapper 中透传 URL**

给三个 DTO 增加字段：

```go
type projectResponse struct {
	URL string `json:"url"`
	// existing fields
}

type taskOccurrenceJSON struct {
	URL string `json:"url"`
	// existing fields
}

type taskSeriesJSON struct {
	URL string `json:"url"`
	// existing fields
}
```

在 mapper 中逐一复制，不拼路径：

```go
URL: view.URL,
```

- [ ] **Step 4: 写 OpenAPI 必填 `url` 的失败断言**

在 `TestOpenAPIDocumentsTaskSeriesAndOccurrenceContracts` 增加 response schema helper，并断言 task/series：

```go
assertRequiredProperty := func(path, method, property string) {
	operation := operation(path, method)
	response := operation["responses"].(map[string]any)["200"].(map[string]any)
	content := response["content"].(map[string]any)["application/json"].(map[string]any)
	encoded, _ := json.Marshal(content)
	if !strings.Contains(string(encoded), `"`+property+`"`) {
		t.Fatalf("%s %s response missing %s", method, path, property)
	}
}
assertRequiredProperty("/api/v1/tasks/{taskRef}", "get", "url")
assertRequiredProperty("/api/v1/task-series/{seriesRef}", "get", "url")
assertRequiredProperty("/api/v1/projects/{projectRef}", "get", "url")
```

- [ ] **Step 5: 扩展 OpenAPI 项目、任务、系列 schema**

在 `taskOccurrenceOpenAPISchema` 和 `taskSeriesOpenAPISchema` 的 properties/required 增加：

```go
"url": {Type: "string", Description: "Web Console relative URL."},
```

新增并复用项目 schema：

```go
func projectOpenAPISchema() *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{
		"id": {Type: "string"}, "workspace_id": {Type: "string"},
		"slug": {Type: "string"}, "name": {Type: "string"},
		"url": {Type: "string", Description: "Web Console relative URL."},
		"description": {Type: "string"}, "status": {Type: "string"},
		"task_count": {Type: "integer", Format: "int32"},
		"pending_count": {Type: "integer", Format: "int32"},
		"completed_count": {Type: "integer", Format: "int32"},
		"created_at": {Type: "integer", Format: "int64"},
		"modified_at": {Type: "integer", Format: "int64"},
	}, Required: []string{"id", "workspace_id", "slug", "name", "url", "status", "task_count", "pending_count", "completed_count", "created_at", "modified_at"}}
}
```

`contractSuccessResponse` 对 `/api/v1/projects` 的 list/create 和返回 project 的 detail/write routes 使用 `projectOpenAPISchema()`；`homeOpenAPISchema` 删除内联 project 重复定义，直接复用该函数。

- [ ] **Step 6: 同步 Web TypeScript view**

给以下类型增加必填字段：

```ts
url: string
```

- `ProjectSummary`
- `ProjectWorkbenchProject`
- `ProjectWorkbenchTask`
- `TaskOccurrenceView`
- `TaskSeriesView`

测试 fixture 因缺少 `url` 导致类型错误时，使用与 fixture slug/ref 对应的真实路径补齐，不能使用空字符串或类型断言绕过。

- [ ] **Step 7: 运行 HTTP 与 Web 类型验证**

Run: `gofmt -w internal/httpapi/projects.go internal/httpapi/projects_test.go internal/httpapi/task_series.go internal/httpapi/task_series_test.go internal/httpapi/huma_routes.go internal/httpapi/server_test.go internal/httpapi/home_test.go`

Run: `go test ./internal/httpapi -count=1`

Run: `pnpm --dir web typecheck`

Run: `pnpm --dir web test`

Expected: 全部 PASS；OpenAPI 中 project/task/series 的 `url` 都在 required 中。

- [ ] **Step 8: 提交 HTTP/OpenAPI/Web 契约**

```bash
git add internal/httpapi/projects.go internal/httpapi/projects_test.go internal/httpapi/task_series.go internal/httpapi/task_series_test.go internal/httpapi/huma_routes.go internal/httpapi/server_test.go internal/httpapi/home_test.go web/src/features/workspace/projects/projects-api.ts web/src/features/workspace/project-workbench/api/project-api.ts web/src/features/workspace/project-workbench/api/task-series-api.ts
git commit -m "feat: 在 HTTP 响应返回资源 URL"
```

---

### Task 3: 让 Remote 与 CLI local/remote 保留并展示 URL

**Files:**
- Modify: `internal/remote/project.go:12-25,138-153`
- Create: `internal/remote/project_test.go`
- Modify: `internal/remote/task_series.go:20-120,377-405`
- Modify: `internal/remote/task_test.go:9-30`
- Modify: `internal/remote/task_series_test.go:212-245`
- Modify: `internal/cli/project.go:37-92,99-350,528-560`
- Modify: `internal/cli/add.go:17-86`
- Modify: `internal/cli/info.go:59-105`
- Modify: `internal/cli/list.go:177-205`
- Modify: `internal/cli/series.go:640-990`
- Modify: `internal/cli/series_test.go:93-235`
- Test: `tests/integration/cli_test.go:45-75,360-395,1385-1410,1920-1950,2180-2235,3015-3090`

**Interfaces:**
- Consumes: HTTP JSON 的 `url` 与 Task 1 的 App view `URL`。
- Produces: `projectDTO.URL`、`TaskOccurrenceDTO.URL`、`TaskSeriesDTO.URL`，CLI JSON 中的 `url`，以及人类可读输出中的 URL。

- [ ] **Step 1: 写 Remote round-trip 的失败测试**

创建 `internal/remote/project_test.go`：

```go
package remote

import "testing"

func TestProjectDTOToViewKeepsURL(t *testing.T) {
	view := projectDTOToView(projectDTO{ID: "p1", Slug: "ops", URL: "/workspaces/local/projects/ops"})
	if view.URL != "/workspaces/local/projects/ops" {
		t.Fatalf("URL = %q", view.URL)
	}
}
```

在 `TestRemoteTaskResponseDecodesTaskSlug` 的 JSON 加入：

```json
"url":"/workspaces/local/projects/api/tasks/api-12"
```

并断言 `dto.URL`；在 `TestParseTaskOccurrenceDTOMaterializedShape` 和现有 series remote 测试同样断言 URL。

- [ ] **Step 2: 运行 Remote 测试，确认 DTO 字段不存在而失败**

Run: `go test ./internal/remote -run 'Test(ProjectDTOToViewKeepsURL|RemoteTaskResponseDecodesTaskSlug|ParseTaskOccurrenceDTOMaterializedShape)' -count=1`

Expected: FAIL，编译错误包含 `URL undefined`。

- [ ] **Step 3: 给 Remote DTO 与 CLI DTO→view mapper 增加 URL**

三个 DTO 增加：

```go
URL string `json:"url"`
```

`projectDTOToView`、`remoteOccurrenceDTOToView`、`remoteSeriesDTOToView` 分别增加：

```go
URL: dto.URL,
```

`parseTaskOccurrenceDTO` 在 `id` 校验后增加：

```go
if strings.TrimSpace(dto.URL) == "" {
	return TaskOccurrenceDTO{}, fmt.Errorf("task occurrence response is missing url")
}
```

这会让 remote CLI 对不满足新服务端契约的响应显式失败，不在客户端重算 URL。

- [ ] **Step 4: 写 CLI JSON 与人类输出的失败测试**

在 `internal/cli/series_test.go` 增加：

```go
func TestResourceJSONAndHumanOutputKeepURL(t *testing.T) {
	view := app.TaskOccurrenceView{ID: "task-1", URL: "/tasks/task-1", Title: "任务", Status: task.StatusPending}
	payload := occurrenceViewJSON(view)
	if payload["url"] != view.URL {
		t.Fatalf("json url = %#v", payload["url"])
	}
	var out bytes.Buffer
	renderTaskOccurrenceInfo(&out, false, view)
	if !strings.Contains(out.String(), "URL: /tasks/task-1") {
		t.Fatalf("human output = %q", out.String())
	}
}
```

扩展 integration CLI 测试，至少断言：

```go
if got["url"] != "/workspaces/local/projects/api/tasks/api-1" {
	t.Fatalf("task url = %#v", got["url"])
}
if !strings.Contains(info, "URL: /workspaces/local/projects/api/tasks/api-1") {
	t.Fatalf("task info = %q", info)
}
```

project info/list/add/modify/archive/transition 与 series list/info/create 的 local/remote JSON 和人类输出使用对应路径做同类断言。

- [ ] **Step 5: 在 CLI 所有资源输出中透传 URL**

JSON mapper 增加：

```go
"url": project.URL,
"url": v.URL,
```

分别放入 `projectViewForJSON`、`occurrenceViewJSON`、`seriesViewJSON`。

人类详情增加：

```go
fmt.Fprintf(w, "URL: %s\n", view.URL)
```

项目 list 每行、任务 list 的现有表格之后、series list 每行都以 ref + URL 明确关联：

```go
fmt.Fprintf(out, "%s  %s\n", project.Slug, project.URL)
fmt.Fprintf(out, "%s  %s\n", occurrenceHumanRef(view), view.URL)
fmt.Fprintf(out, "%s  %s\n", app.SeriesSlugOf(series.Series), series.URL)
```

project/task/series 的 add/modify/archive/transition/stop 等已经拿到 view 的人类输出，在现有结果行后追加 `URL: <path>`；不为只返回 `error`、annotation、config 或 timeline 的命令伪造资源 URL。

- [ ] **Step 6: 运行 Remote、CLI 与 CLI integration 测试**

Run: `gofmt -w internal/remote/project.go internal/remote/project_test.go internal/remote/task_series.go internal/remote/task_test.go internal/remote/task_series_test.go internal/cli/project.go internal/cli/add.go internal/cli/info.go internal/cli/list.go internal/cli/series.go internal/cli/series_test.go tests/integration/cli_test.go`

Run: `go test ./internal/remote ./internal/cli -count=1`

Run: `go test ./tests/integration -run 'TestCLI|TestTaskSeries|TestProject' -count=1`

Expected: 全部 PASS；local/remote CLI 的 `url` 相同。

- [ ] **Step 7: 提交 Remote/CLI 输出**

```bash
git add internal/remote/project.go internal/remote/project_test.go internal/remote/task_series.go internal/remote/task_test.go internal/remote/task_series_test.go internal/cli/project.go internal/cli/add.go internal/cli/info.go internal/cli/list.go internal/cli/series.go internal/cli/series_test.go tests/integration/cli_test.go
git commit -m "feat: 在 CLI 返回资源 URL"
```

---

### Task 4: 让 MCP project/task/series structured data 透传 URL

**Files:**
- Modify: `internal/mcpserver/tools_views.go:22-40,118-146`
- Modify: `internal/mcpserver/tools_task_series.go:350-520`
- Modify: `internal/mcpserver/integration_test.go:40-90,400-450,1560-1605`
- Modify: `internal/mcpserver/tools_task_series_test.go:40-290`

**Interfaces:**
- Consumes: Task 1 的 App view `URL`。
- Produces: MCP `ToolEnvelope.Data` 中 project/task/series 的 `url`；`successResult` 会把同一 envelope JSON 放进 `content[0].text`，不新增客户端侧重塑逻辑。

- [ ] **Step 1: 写 MCP project/task/series URL 的失败测试**

在 `internal/mcpserver/integration_test.go` 的 task add/get 和 project get 测试增加：

```go
if taskObj["url"] != "/tasks/"+taskObj["uuid"].(string) {
	t.Fatalf("task url = %#v", taskObj["url"])
}
if projectData["url"] != "/workspaces/local/projects/agent" {
	t.Fatalf("project url = %#v", projectData["url"])
}
```

在 `tools_task_series_test.go` 的 add/get/list/occurrence 测试增加：

```go
if series["url"] != "/workspaces/local/projects/ops/series/ops-s-1" {
	t.Fatalf("series url = %#v", series["url"])
}
if occurrence["url"] == "" {
	t.Fatalf("occurrence url missing: %#v", occurrence)
}
```

同时将 `renderedText(result)` 解析为 `ToolEnvelope`，断言其 JSON `data` 中含同一个 URL，证明 text 与 structured content 未分叉。

- [ ] **Step 2: 运行 MCP 测试，确认 `url` 缺失而失败**

Run: `go test ./internal/mcpserver -run 'Test(MCPTaskSeriesAddAndGet|MCPTaskSeriesList|TaskAdd|Project)' -count=1`

Expected: FAIL，断言显示 `url = <nil>`。

- [ ] **Step 3: 只在 MCP mapper 透传 App URL**

`projectView` 增加：

```go
URL string `json:"url"`
```

`projectViewFromApp` 增加：

```go
URL: row.URL,
```

`seriesViewToMCPJSON` 与 `occurrenceViewToMCPJSON` 的基础 map 增加：

```go
"url": v.URL,
```

不要修改 `successResult`，不要在 Yaoguang或其他 MCP client 侧合并 `structuredContent`；`content[0].text` 已经是同一个 `ToolEnvelope` 的 JSON。

- [ ] **Step 4: 运行 MCP 包测试**

Run: `gofmt -w internal/mcpserver/tools_views.go internal/mcpserver/tools_task_series.go internal/mcpserver/integration_test.go internal/mcpserver/tools_task_series_test.go`

Run: `go test ./internal/mcpserver -count=1`

Expected: 全部 PASS。

- [ ] **Step 5: 提交 MCP 输出**

```bash
git add internal/mcpserver/tools_views.go internal/mcpserver/tools_task_series.go internal/mcpserver/integration_test.go internal/mcpserver/tools_task_series_test.go
git commit -m "feat: 在 MCP 返回资源 URL"
```

---

### Task 5: 锁定跨协议等价性、同步 README 并完成全量验证

**Files:**
- Modify: `tests/integration/e2e_mcp_test.go:1-420`
- Modify: `README.md:350-390`
- Verify: `docs/superpowers/specs/2026-07-18-resource-url-output-design.md`
- Verify: `docs/superpowers/plans/2026-07-18-resource-url-output-implementation.md`

**Interfaces:**
- Consumes: Tasks 1-4 的 App、HTTP、MCP、CLI、Remote `url`。
- Produces: HTTP/MCP/remote CLI 等价性证据、用户文档和全量测试证据。

- [ ] **Step 1: 先扩展跨协议 contract，确认任一缺失 URL 都会失败**

在 `taskViewPageContract` 和 `seriesPageContract` 增加：

```go
URL string `json:"url"`
```

在 `assertTaskViewPagesEquivalent` 对每条 item 增加：

```go
rawURL, ok := item["url"].(string)
if !ok || rawURL == "" {
	t.Fatalf("%s page %d item missing url: %#v", label, index, item)
}
```

在 `assertSeriesPagesEquivalent` 对每条 series 做相同断言。新增项目跨协议检查：通过 HTTP `GET /api/v1/projects/ops`、MCP `project_get`、remote CLI `project info ops --json` 读取同一项目，并用仅包含 `ID/Slug/URL` 的 struct `reflect.DeepEqual`。

- [ ] **Step 2: 运行跨协议 E2E，确认测试能捕获任何未透传路径**

Run: `go test ./tests/integration -run TestE2ERecurringQueriesReportsAndSeriesListsAreProtocolEquivalent -count=1`

Expected: 如果 Tasks 1-4 有任何漏点则 FAIL 并指出具体协议缺失 `url`；完整实现后 PASS。

- [ ] **Step 3: 同步 README 的当前行为**

在 README 的统一 TaskView/Remote 契约段落后加入：

```markdown
- HTTP、MCP、CLI `--json` 与 Remote client 返回项目、任务或循环系列对象时，统一包含 Web Console 相对路径 `url`。项目路径为 `/workspaces/{workspaceSlug}/projects/{projectSlug}`；项目任务路径为 `/workspaces/{workspaceSlug}/projects/{projectSlug}/tasks/{taskRef}`；循环系列路径为 `/workspaces/{workspaceSlug}/projects/{projectSlug}/series/{seriesSlug}`。普通任务和已物化循环实例优先使用 `task_slug`，计划实例使用编码后的 `occurrence_ref`，无项目任务使用 `/tasks/{uuid}`。
```

不修改 `ROADMAP.md`：该变更没有扩大 milestone 或改变产品边界。

- [ ] **Step 4: 运行格式、静态检查和完整 Go 验证**

Run: `gofmt -w $(git diff --name-only --diff-filter=ACM -- '*.go')`

Run: `git diff --check`

Run: `go test ./...`

Run: `CGO_ENABLED=0 go test ./...`

Run: `CGO_ENABLED=0 go build ./cmd/xuanchu`

Run: `go vet ./...`

Expected: 全部 exit 0。

- [ ] **Step 5: 运行完整 Web 验证**

Run: `pnpm --dir web typecheck`

Run: `pnpm --dir web test`

Run: `pnpm --dir web lint`

Run: `pnpm --dir web build`

Run: `pnpm --dir web run smoke:editing`

Expected: 全部 exit 0；smoke 结束后不留下 dev server。

- [ ] **Step 6: 做完成审计**

Run: `rg -n 'json:"url"|"url": .*\.URL|URL: .*\.URL|url: string' internal/app internal/httpapi internal/remote internal/cli internal/mcpserver web/src`

Expected: project/task/series 的 App view、HTTP DTO、Remote DTO、CLI mapper、MCP mapper、OpenAPI/Web 类型都有明确命中。

Run: `rg -n '/tasks/.*/series/|tasks/\{taskSlug\}/series' internal web/src README.md docs/superpowers/specs/2026-07-18-resource-url-output-design.md`

Expected: 仅设计 spec 中“不得新增”的说明可以命中；代码和 README 不返回该旧提议路径。

检查 `git status --short`，确认只有本计划范围文件；`docs/business/` 仍为用户未跟踪内容且未暂存。

- [ ] **Step 7: 提交 E2E 与文档收口**

```bash
git add tests/integration/e2e_mcp_test.go README.md
git commit -m "test: 验证资源 URL 跨协议一致"
```

---

## 最终验收矩阵

| 资源 | App | HTTP | MCP | CLI local JSON/human | CLI remote JSON/human | OpenAPI/Web |
|---|---|---|---|---|---|---|
| Project | `ProjectView.URL` | project list/detail/write + home | project tools | list/info/write | list/info/write | required `url` |
| Project task | `TaskOccurrenceView.URL` | task list/detail/write + home | task tools | list/info/write | list/info/write | required `url` |
| Standalone task | `TaskOccurrenceView.URL` | task list/detail/write | task tools | list/info/write | list/info/write | required `url` |
| Projected occurrence | `TaskOccurrenceView.URL`，无写入 | task expand/detail | task/series occurrence tools | list/info | list/info | required `url` |
| Materialized occurrence | `TaskOccurrenceView.URL`，使用 task slug | task/series responses | task/series tools | list/info/write | list/info/write | required `url` |
| Task series | `TaskSeriesView.URL` | series list/detail/write | task-series tools | list/info/write | list/info/write | required `url` |

计划完成的唯一成功标准是：上述矩阵所有单元都有直接测试或跨协议 E2E 证据，且完整 Go、零 CGO、Web、Playwright 验证全部通过。
