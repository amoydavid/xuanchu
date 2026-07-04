# Task Change History Audit Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在任务详情页展示来自 `audit_logs` 的字段级 task 变更历史，覆盖 assignees、due、priority、project、tags、title、description。

**Architecture:** 后端继续以 `audit_logs` 为唯一持久化来源，在 `task.modify` 的 `payload_json` 里追加机器语义 `changes` 数组；通用 audit 列表仍走 `audit:read`，任务详情历史新增 task-read 专用服务入口和 HTTP route。App/HTTP view 把 payload 解析为可渲染 change view（kind、label_key、raw、text），前端用 i18n 模板组合人类文字，mutation 成功后刷新 task 和 audit query。

**Tech Stack:** Go 1.25, GORM, glebarez/sqlite, PostgreSQL driver, chi/Huma route registry, React 19, TanStack Query, TypeScript, Vitest, Testing Library, pnpm.

---

## Chunk 1: 后端 Audit Payload 与查询基础

### Task 1: 给 task diff 增加 title / description，并生成 audit changes

**Files:**
- Modify: `internal/app/task_change_events.go`
- Create: `internal/app/task_audit_payload.go`
- Test: `internal/app/task_audit_payload_test.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 写 payload helper 单元测试**

在 `internal/app/task_audit_payload_test.go` 写表驱动测试：

```go
func TestTaskModifyAuditPayloadIncludesStableChanges(t *testing.T) {
    beforeProject := "ops"
    afterProject := "agentapi"
    beforeProjectID := "project-before"
    afterProjectID := "project-after"
    previousDue := int64(1783036800)
    currentPriority := "H"
    previousDescription := "旧描述"
    currentDescription := "新描述"

    payload := taskModifyAuditPayload(projectChange{
        Before: projectBinding{ID: &beforeProjectID, Slug: &beforeProject},
        After:  projectBinding{ID: &afterProjectID, Slug: &afterProject},
    }, TaskChangeDiff{
        AssigneesChanged: true,
        AddedAssignees: []task.UserInfo{{ID: "u2", Name: "lisi", DisplayName: "李四"}},
        RemovedAssignees: []task.UserInfo{{ID: "u1", Name: "zhangsan", DisplayName: "张三"}},
        DueChanged: true,
        PreviousDue: &previousDue,
        CurrentDue: nil,
        PriorityChanged: true,
        PreviousPriority: nil,
        CurrentPriority: &currentPriority,
        ProjectChanged: true,
        PreviousProject: &beforeProject,
        CurrentProject: &afterProject,
        TagsChanged: true,
        AddedTags: []string{"dashboard"},
        RemovedTags: []string{"ads"},
        TitleChanged: true,
        PreviousTitle: "旧标题",
        CurrentTitle: "新标题",
        DescriptionChanged: true,
        PreviousDescription: &previousDescription,
        CurrentDescription: &currentDescription,
    })

    changes, ok := payload["changes"].([]map[string]any)
    if !ok {
        t.Fatalf("changes type = %T", payload["changes"])
    }
    gotFields := make([]string, 0, len(changes))
    for _, change := range changes {
        gotFields = append(gotFields, change["field"].(string))
    }
    wantFields := []string{"assignees", "due", "priority", "project", "tags", "title", "description"}
    if !reflect.DeepEqual(gotFields, wantFields) {
        t.Fatalf("fields = %#v, want %#v", gotFields, wantFields)
    }
    added := changes[0]["added"].([]any)[0].(map[string]any)
    if added["name"] != "lisi" || added["display_name"] != "李四" {
        t.Fatalf("assignee payload = %#v", added)
    }
    if _, ok := changes[1]["current"]; !ok {
        t.Fatal("due current key missing; explicit null must be preserved")
    }
}
```

测试文件需要导入 `reflect`，或者改用已有断言 helper；关键是断言完整字段顺序，而不是只看第一项。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/app -run 'TestTaskModifyAuditPayload|TestModify.*Audit' -count=1
```

Expected: FAIL，`taskModifyAuditPayload` 和 title/description diff 尚不存在。

- [ ] **Step 3: 扩展 `TaskChangeDiff`**

在 `internal/app/task_change_events.go` 增加：

```go
TitleChanged bool
PreviousTitle string
CurrentTitle string

DescriptionChanged bool
PreviousDescription *string
CurrentDescription *string
```

在 `diffTaskChanges` 里比较：

```go
if before.Title != after.Title {
    diff.TitleChanged = true
    diff.PreviousTitle = before.Title
    diff.CurrentTitle = after.Title
}
if ptrStringDiff(before.Description, after.Description) {
    diff.DescriptionChanged = true
    diff.PreviousDescription = before.Description
    diff.CurrentDescription = after.Description
}
```

不要修改 `buildFineGrainedEvents` 的事件类型。

- [ ] **Step 4: 实现 audit payload helper**

创建 `internal/app/task_audit_payload.go`：

```go
package app

func taskModifyAuditPayload(change projectChange, diff TaskChangeDiff) map[string]any {
    payload := projectChangePayload(change)
    payload["changes"] = taskFieldChanges(change, diff)
    return payload
}
```

实现 `taskFieldChanges`，按 spec 固定顺序 append。UserInfo 序列化必须包含 `id`、`name`、`display_name`、`email`、`external_ids`；可以复用并修正 `userInfosToJSONList` / `userInfoToEventPayload`，但不能继续沿用当前缺少 `display_name` 的输出形状。

- [ ] **Step 5: 串入 `task.modify` 专用 helper 和 `Service.Modify`**

不要修改现有 `taskAuditEntry(action, targetID, change)`，避免影响 `task.add`、`task.done`、`task.annotate` 等非本期 action。在 `internal/app/task_audit_payload.go` 新增 `taskModifyAuditEntry`：

```go
func taskModifyAuditEntry(targetID string, change projectChange, diff TaskChangeDiff) AuditEntry {
    return AuditEntry{
        Action: "task.modify",
        ProjectID: auditProjectIDForChange(change),
        TargetType: "task",
        TargetID: targetID,
        Payload: taskModifyAuditPayload(change, diff),
    }
}
```

修改 `internal/app/service.go` 的 `Modify`：

```go
entry := taskModifyAuditEntry(modified.UUID, change, diff)
```

不要改其他 `taskAuditEntry` 调用点。

- [ ] **Step 6: 写 service 集成测试**

在 `internal/app/service_test.go` 增加测试：

- 修改 title/description 后，最新 `task.modify` payload 里有 `changes`。
- 只修改未覆盖字段（例如 wait）时，新 payload 里 `changes` 是空数组。
- 修改 assignees 后，added/removed 是完整 UserInfo，而不是裸 UUID。
- 清空 due/description 时，payload 里保留 `current: nil` 语义，后续 HTTP view 不允许把该 key 省略。
- UserInfo payload 同时保留稳定 `name` 和展示用 `display_name`，不要把 display name 写进 `name`。

- [ ] **Step 7: 运行后端 app 测试**

Run:

```bash
go test ./internal/app -count=1
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add internal/app/task_change_events.go internal/app/task_audit_payload.go internal/app/task_audit_payload_test.go internal/app/service_test.go
git commit -m "feat: 记录任务字段级审计变更"
```

### Task 2: 支持 target/action 过滤和结构化 changes view

**Files:**
- Modify: `internal/storage/audit_repo.go`
- Modify: `internal/storage/models.go`
- Create: `internal/storage/audit_repo_test.go`
- Modify: `internal/storage/db_test.go`
- Modify: `internal/app/audit.go`
- Test: `internal/app/audit_test.go`

- [ ] **Step 1: 写 storage 过滤测试**

创建 `internal/storage/audit_repo_test.go`，构造同 workspace 下两条 `target_id` 不同的 audit，断言：

```go
rows, err := repo.List(AuditListOptions{
    WorkspaceID: &workspaceID,
    TargetType: ptr("task"),
    TargetID: ptr("task-1"),
    Action: ptr("task.modify"),
})
```

只返回 `task-1` 的 `task.modify`。

- [ ] **Step 2: 写索引迁移测试**

在 `internal/storage/db_test.go` 增加：

```go
if !store.DB().Migrator().HasIndex(&AuditLog{}, "idx_audit_target_time") {
    t.Fatal("audit target index missing")
}
```

- [ ] **Step 3: 写 app view 解析测试**

在 `internal/app/audit_test.go` 增加：

- `ListAudit` 解析 payload 中的 `changes` 到 `AuditLogView.Changes`。
- payload 是历史空字符串或坏 JSON 时不报错，`Changes` 为空。
- `ListTaskAudit` 用只有 `task:read` 的 token 成功；同 token 调 `ListAudit` 仍失败。

- [ ] **Step 4: 运行失败测试**

Run:

```bash
go test ./internal/storage ./internal/app -run 'Test.*Audit|Test.*audit' -count=1
```

Expected: FAIL，因为 target/action 过滤、索引和 `ListTaskAudit` 尚未实现。

- [ ] **Step 5: 实现 storage options**

`internal/storage/audit_repo.go`：

```go
type AuditListOptions struct {
    WorkspaceID *string
    ProjectID *string
    TargetType *string
    TargetID *string
    Action *string
    Limit int
    Offset int
}
```

`List` 中用参数绑定追加：

```go
if opts.TargetType != nil { query = query.Where("target_type = ?", *opts.TargetType) }
if opts.TargetID != nil { query = query.Where("target_id = ?", *opts.TargetID) }
if opts.Action != nil { query = query.Where("action = ?", *opts.Action) }
```

- [ ] **Step 6: 添加 GORM 复合索引**

在 `internal/storage/models.go` 的 `AuditLog` tags 中加入 `idx_audit_target_time`，保持 SQLite/Postgres 都走 AutoMigrate。

- [ ] **Step 7: 重构 app audit view 构造**

在 `internal/app/audit.go`：

- `AuditListInput` 增加 target/action 字段。
- 新增 `TaskAuditInput` 和 `ListTaskAudit`。
- 把 `ListAudit` 中 rows -> views 的逻辑抽成 `auditLogViewsFromRows`。
- 新增 `TaskChangeDisplayValue`、`TaskFieldChange` 与 `parseTaskFieldChanges(payload string) []TaskFieldChange`。
- `TaskFieldChange` 必须包含 `Field`、`Kind`、`LabelKey` 和可渲染 value；标量的 previous/current 需要能表达 raw 为 nil 的值，不要依赖 `omitempty` 判断字段是否存在。
- response DTO 使用 snake_case JSON tag：`label_key`、`previous`、`current`、`added`、`removed`、`raw`、`text`。

`ListTaskAudit` 必须：

```go
if err := s.Require(PermissionTaskRead); err != nil { return nil, err }
resolved, err := s.ResolveProtocolTarget(taskRef)
...
targetType := "task"
targetID := resolved.UUID
action := "task.modify"
rows, err := s.auditRepo.List(storage.AuditListOptions{...})
```

`TaskFieldChange` 建议形状：

```go
type TaskChangeDisplayValue struct {
    Raw  any
    Text string
}

type TaskFieldChange struct {
    Field    string
    Kind     string
    LabelKey string
    Previous *TaskChangeDisplayValue
    Current  *TaskChangeDisplayValue
    Added    []TaskChangeDisplayValue
    Removed  []TaskChangeDisplayValue
}
```

display text / formatter 规则：

- 这是 app view 概念形状；HTTP response 层不要给 scalar 的 `previous/current` 加 `omitempty`。如果 Go 指针用于表达 presence，scalar change 中必须保证 `Previous` / `Current` 非 nil，内部 `Raw` 可以为 nil。
- `Text` 是兜底显示值，不是最终 UI 文案；前端 formatter 应优先使用 `field` + `raw` + i18n locale。
- nil raw -> 前端用 `projectWorkbench.taskHistory.unset`，不要直接显示 `nil`、`null` 或后端兜底中文。
- UserInfo raw -> display_name 优先，其次 name，其次 id。
- due raw -> 前端按 locale 格式化日期；服务端 text 可用 `YYYY-MM-DD` 兜底。
- description raw -> 前端去除 Markdown 标记或至少压缩换行后截断，避免 timeline 行撑开。
- 不在后端生成整句 summary。

- [ ] **Step 8: 运行 focused 测试**

Run:

```bash
go test ./internal/storage ./internal/app -count=1
```

Expected: PASS。

- [ ] **Step 9: Commit**

```bash
git add internal/storage/audit_repo.go internal/storage/models.go internal/storage/audit_repo_test.go internal/storage/db_test.go internal/app/audit.go internal/app/audit_test.go
git commit -m "feat: 支持按任务读取审计历史"
```

## Chunk 2: HTTP API

### Task 3: 暴露 `/api/v1/tasks/{taskRef}/audit`

**Files:**
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/tasks_test.go`
- Modify: `internal/httpapi/server_test.go`

- [ ] **Step 1: 写 HTTP 测试**

在 `internal/httpapi/tasks_test.go` 增加：

```go
func TestTaskAuditUsesTaskReadScope(t *testing.T) {
    writer := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
    // 用 writer 的 store 创建项目和 task，并修改 title 生成 audit。
    // 再在同一个 store 里创建一个新的只含 task:read 的 token，请求 GET /api/v1/tasks/{slug}/audit。
    // 请求不能复用 writer token，否则无法证明 task audit 不要求 task:write。
    // 断言 200，body 包含 "changes"、"payload"、"title"。
}
```

再加两个边界测试：

```go
GET /api/v1/tasks/1/audit -> 400 task_ref_invalid
只有 audit:read、没有 task:read 的 token -> 403 permission_denied
清空 due 后响应 JSON 仍包含 `"current":{"raw":null,...}`，不能省略 `current` 或 `raw`
```

- [ ] **Step 2: 更新 route smoke**

如果 `internal/httpapi/server_test.go` 有固定 route 清单，把 `/api/v1/tasks/{taskRef}/audit` 加进去。

- [ ] **Step 3: 运行失败测试**

Run:

```bash
go test ./internal/httpapi -run 'TestTaskAudit|TestRoutes' -count=1
```

Expected: FAIL，route/handler 未实现。

- [ ] **Step 4: 提取 audit response helper**

在 `internal/httpapi/import_audit.go`：

- 给 `auditResponse` 新增：

```go
Changes []taskFieldChangeResponse `json:"changes,omitempty"`
```

- 新增 `auditRowsToResponse(rows []app.AuditLogView) []auditResponse`，让 `handleAuditList` 和 task audit handler 共用。
- `Payload` 仍保留 `json.RawMessage`。
- `taskFieldChangeResponse` 的 scalar `previous/current` 不要因为 raw 为 nil 被 omitempty 掉；集合 `added/removed` 返回数组。必要时为 response 层单独定义 struct，不要直接暴露 app view 指针字段导致 JSON presence 不稳定。

- [ ] **Step 5: 实现 task audit handler**

在 `internal/httpapi/tasks.go` 增加：

```go
func (s *Server) handleTaskAudit(w http.ResponseWriter, r *http.Request) {
    taskRef, ok := requireTaskRef(w, r)
    if !ok { return }
    limit, offset, ok := parseLimitOffset(w, r, 50, auditMaxLimit)
    if !ok { return }
    scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
    if err != nil { writeAppError(w, err); return }
    rows, err := scoped.ListTaskAudit(taskRef, app.TaskAuditInput{Limit: limit, Offset: offset})
    if err != nil { writeAppError(w, err); return }
    writeSuccess(w, http.StatusOK, auditRowsToResponse(rows), nil)
}
```

当前 `handleAuditList` 只有内联 limit 解析，没有通用 `parseLimitOffset`。实现本步骤时创建这个小 helper，复用现有 audit/annotation 的解析规则，至少让 task audit 支持 `limit` 和 `offset`；通用 `/api/v1/audit` 可以继续保持现有无 offset 行为，除非顺手复用 helper 不改变兼容性。

- [ ] **Step 6: 注册路由**

`internal/httpapi/huma_routes.go` 的 task route 组加入：

```go
{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/audit", Tag: "Tasks", Summary: "List task audit history.", Handler: s.handleTaskAudit},
```

放在 `/api/v1/tasks/{taskRef}` 之后、annotations/links 之前或之后都可以；chi/Huma route registry 应能处理具体后缀。

- [ ] **Step 7: 运行 HTTP 测试**

Run:

```bash
go test ./internal/httpapi -count=1
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add internal/httpapi/import_audit.go internal/httpapi/tasks.go internal/httpapi/huma_routes.go internal/httpapi/tasks_test.go internal/httpapi/server_test.go
git commit -m "feat: 暴露任务审计历史接口"
```

## Chunk 3: Web Console Task History

### Task 4: 增加前端 API 和 query hook

**Files:**
- Modify: `web/src/features/workspace/project-workbench/api/task-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/task-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.test.tsx`

- [ ] **Step 1: 写 path builder 测试**

在 `task-api.test.ts` 增加：

```ts
expect(taskAuditPath("workspace 1", "ads/1")).toBe(
  "/api/v1/tasks/ads%2F1/audit?workspace=workspace%201"
)
```

- [ ] **Step 2: 写 mutation invalidation 测试**

在 `use-task-mutations.test.tsx` 增加断言：title/description/assignee/tag mutation 成功后 invalidate `taskQueryKeys.audit(workspaceSlug, taskRef)`。

- [ ] **Step 3: 运行失败测试**

Run:

```bash
pnpm --dir web test -- project-workbench/api project-workbench/hooks/use-task-mutations
```

Expected: FAIL，API 和 query key 尚不存在。

- [ ] **Step 4: 实现 API 类型和请求**

`task-api.ts` 增加：

```ts
export type TaskChangeDisplayValue = {
  raw: unknown
  text: string
}

export type TaskChangeField =
  | "assignees"
  | "tags"
  | "due"
  | "priority"
  | "project"
  | "title"
  | "description"

export type TaskScalarFieldChange = {
  field: Exclude<TaskChangeField, "assignees" | "tags">
  kind: "scalar"
  label_key: string
  previous: TaskChangeDisplayValue
  current: TaskChangeDisplayValue
}

export type TaskSetFieldChange = {
  field: "assignees" | "tags"
  kind: "set"
  label_key: string
  added: TaskChangeDisplayValue[]
  removed: TaskChangeDisplayValue[]
}

export type TaskFieldChange = TaskScalarFieldChange | TaskSetFieldChange

export type TaskAuditEntry = {
  id: number
  actor?: UserInfo | null
  actor_token?: { id: string; name: string; prefix: string } | null
  action: string
  target_type: string
  target_id: string
  payload?: unknown
  changes?: TaskFieldChange[]
  created_at: number
}
```

从 `project-api.ts` 复用 `UserInfo` 类型。

- [ ] **Step 5: 实现 query hook 和 invalidation**

`use-task-detail-data.ts`：

```ts
audit: (workspaceSlug: string, taskRef: string) =>
  ["task", workspaceSlug, taskRef, "audit"] as const
```

新增 `useTaskAuditQuery`。

`use-task-mutations.ts` 的成功回调同时 invalidate `taskQueryKeys.task(...)` 和 `taskQueryKeys.audit(...)`。

- [ ] **Step 6: 运行 focused 前端测试**

Run:

```bash
pnpm --dir web test -- project-workbench/api project-workbench/hooks/use-task-mutations
```

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add web/src/features/workspace/project-workbench/api/task-api.ts web/src/features/workspace/project-workbench/api/task-api.test.ts web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts web/src/features/workspace/project-workbench/hooks/use-task-mutations.test.tsx
git commit -m "feat: 增加任务历史前端客户端"
```

### Task 5: 在任务详情页渲染变更历史

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Create: `web/src/features/workspace/project-workbench/task-detail/task-change-history.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写详情页测试**

在 `task-detail-page.test.tsx` mock `getTaskAudit` 或通过 hook 依赖的 API 返回：

```ts
vi.mocked(getTaskAudit).mockResolvedValue([
  {
    id: 1,
    actor: { id: "u1", name: "alice", display_name: "Alice" },
    action: "task.modify",
    target_type: "task",
    target_id: "task-1",
    created_at: 1783036800,
    changes: [{
      field: "title",
      kind: "scalar",
      label_key: "projectWorkbench.taskHistory.field.title",
      previous: { raw: "旧标题", text: "旧标题" },
      current: { raw: "新标题", text: "新标题" },
    }],
  },
])
```

断言页面显示自然语言句子，例如「Alice 将标题从 旧标题 改为 新标题」，而不是直接展示 raw JSON。

再加这些测试：

- 空历史显示占位。
- 请求失败不阻断主体。
- 清空 due / description 时显示「未设置」，不显示 `null`。
- 集合变化显示「新增」/「移除」和人名/tag 文本，不展示 UUID。
- UserInfo 同时有 `display_name` 和 `name` 时优先显示 `display_name`，但不要用 `display_name` 当 identity key。

- [ ] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- project-workbench/task-detail/task-detail-page
```

Expected: FAIL，UI 尚未实现。

- [ ] **Step 3: 实现 `TaskChangeHistory`**

新建 `task-change-history.tsx`：

- 接收 `workspaceSlug` / `taskRef`。
- 内部调用 `useTaskAuditQuery`。
- 过滤掉 `changes` 为空的 rows。
- 按 `kind` 渲染标量和集合两类 change，不靠字段 presence 猜类型。
- 使用 `label_key` 翻译字段名；通过 `field` + `raw` + i18n formatter 生成人类可读值，未知类型才回退到 `*.text`；不要直接把 `raw` 展示给普通用户。
- description 行默认截断；可用 `<details>` 展开 before/after。
- 错误态用紧凑提示，不 throw。

- [ ] **Step 4: 接入详情页**

在 `task-detail-page.tsx` 中，把 `<TaskChangeHistory>` 放在 description/annotations 下方、links 上方：

```tsx
<TaskChangeHistory workspaceSlug={workspaceSlug} taskRef={taskRef} />
```

移动端继续放在 annotations tab 内。

- [ ] **Step 5: 增加 i18n 文案**

`zh-CN.ts` / `en-US.ts` 在 `projectWorkbench.taskHistory` 下新增 spec 中列出的 key，包括 `scalarChange`、`setChange`、`added`、`removed`、`unset`、`changedDescription`、`expandValue` 和 `field.*`。

- [ ] **Step 6: 运行前端 focused 测试**

Run:

```bash
pnpm --dir web test -- project-workbench/task-detail/task-detail-page
```

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx web/src/features/workspace/project-workbench/task-detail/task-change-history.tsx web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 在任务详情页展示变更历史"
```

## Chunk 4: 文档与全量验证

### Task 6: 更新 README 并跑完整验证

**Files:**
- Modify: `README.md`
- Optional Modify: `docs/superpowers/specs/2026-07-04-task-change-history-audit-design.md`

- [ ] **Step 1: 更新 README**

在 Web Console 任务详情页段落补一句：任务详情页会显示字段级变更历史，来源于 `audit_logs`，记录 actor、时间和字段 before/after。

- [ ] **Step 2: 跑完整后端验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS。

- [ ] **Step 3: 跑完整前端验证**

Run:

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web build
```

Expected: PASS。

- [ ] **Step 4: 跑 diff 检查**

Run:

```bash
git diff --check
git status --short
```

Expected: `git diff --check` 无输出；`git status --short` 只包含本任务相关文件和预期构建产物。如果 `pnpm --dir web build` 刷新 `internal/webconsole/dist`，按仓库忽略规则确认不要误提交构建产物。

- [ ] **Step 5: Commit**

```bash
git add README.md docs/superpowers/specs/2026-07-04-task-change-history-audit-design.md docs/superpowers/plans/2026-07-04-task-change-history-audit-implementation.md
git commit -m "docs: 补充任务变更历史实现计划"
```

## Final Acceptance

- `task.modify` 新写入 audit payload 包含稳定 `changes` 数组。
- title / description 修改进入 audit history，但不新增 HookEvent 类型。
- `/api/v1/tasks/{taskRef}/audit` 只要求 task read，且只返回该 task 的 `task.modify` 变更。
- `/api/v1/audit` 仍要求 audit read。
- 任务详情页展示字段级历史；空历史和请求失败都有可控 UI。
- 全量验证命令全部通过：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web build
git diff --check
```
