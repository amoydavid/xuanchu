# Web Console Outbound Integration Console Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Web Console 的 `/hooks` 从 Hook 运维表升级为完整出站集成控制台，让用户能在浏览器内配置 sink、Hook、通知规则、定时规则、查看投递并发送测试投递。

**Architecture:** 第一阶段只复用现有 `/api/v1/*`，在前端新增 outbound feature，把 `/hooks`、`/notifications`、`/integrations` 都导向同一个控制台并用不同默认 tab。第二阶段在后端新增 `notification-sinks/{sinkID}/test`，复用现有 sink 渲染、SSRF 防护、HTTP 投递与 audit 边界，但不写正式 delivery 表。后续阶段再把 notification rule、reminder rule 和统一 delivery 排障中心并入同一控制台。

**Tech Stack:** Go 1.25、GORM、`github.com/glebarez/sqlite`、React 19、TanStack Router、TanStack Query、shadcn/ui、lucide-react、Vitest、Testing Library。

---

## Scope Check

本计划覆盖一个产品闭环，但分为可独立发布的 chunk：

- Chunk 1-4：不新增后端能力，前端补齐 sink/hook/delivery 配置闭环。
- Chunk 5-6：新增 sink test API 与 UI。
- Chunk 7：把 notification rule / reminder rule 接入同一控制台。
- Chunk 8：文档、验证和收尾。

不要在本计划中新增事件类型、重命名数据库表、引入业务 adapter、引入外部队列或实现统一 `outbound-deliveries` 聚合 endpoint。统一 delivery 聚合属于后续 Phase 4。

执行前注意当前工作区可能有其他未提交改动。只修改本计划列出的文件，不要回滚无关改动。

## File Structure

新增前端出站集成模块：

- Create: `web/src/features/workspace/outbound/outbound-api.ts`
  统一封装 sink、hook、notification rule、reminder rule、delivery、test API path 和 DTO。
- Create: `web/src/features/workspace/outbound/outbound-permissions.ts`
  基于 `effective_role`、`token.scopes`、`actor_type` 计算只读/可写能力。
- Create: `web/src/features/workspace/outbound/outbound-event-types.ts`
  前端事件白名单与分组。第一阶段硬编码，必须和 `internal/app/hook.go` 对齐。
- Create: `web/src/features/workspace/outbound/outbound-console.tsx`
  控制台总入口、tabs、概览和 query 编排。
- Create: `web/src/features/workspace/outbound/sinks/sink-list.tsx`
  sink 列表、行操作和详情入口。
- Create: `web/src/features/workspace/outbound/sinks/sink-form-dialog.tsx`
  创建/编辑 sink。
- Create: `web/src/features/workspace/outbound/sinks/sink-test-dialog.tsx`
  发送测试投递，Chunk 6 接入。
- Create: `web/src/features/workspace/outbound/hooks/hook-list.tsx`
  Hook 列表、行展开、行操作。
- Create: `web/src/features/workspace/outbound/hooks/hook-form-dialog.tsx`
  创建/编辑 Hook。
- Create: `web/src/features/workspace/outbound/deliveries/hook-delivery-table.tsx`
  Hook delivery 表格。
- Create: `web/src/features/workspace/outbound/deliveries/delivery-detail-dialog.tsx`
  Hook / notification delivery 详情 JSON 展示。
- Create: `web/src/features/workspace/outbound/rules/notification-rule-list.tsx`
  事件通知规则列表与基础 CRUD。
- Create: `web/src/features/workspace/outbound/rules/reminder-rule-list.tsx`
  定时规则列表与基础 CRUD。

修改现有前端入口：

- Modify: `web/src/routes/router.tsx`
  新增 `/integrations` alias；保留 `/hooks` 和 `/notifications`。
- Modify: `web/src/features/workspace/resources/resource-dispatch.tsx`
  `/hooks`、`/notifications` 都渲染 `OutboundConsole`，默认 tab 不同。
- Modify: `web/src/features/workspace/hooks/hooks-api.ts`
  可保留旧导出，但逐步改为 re-export outbound API，避免重复 path 逻辑。
- Modify: `web/src/features/workspace/hooks/hook-console.tsx`
  可删除或改成薄 wrapper，最终由 outbound console 承载。
- Modify: `web/src/features/workspace/notifications/notification-console.tsx`
  改成薄 wrapper 或不再由 dispatch 使用。
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

新增后端测试投递能力：

- Modify: `internal/httpapi/notifications.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/app/notification.go`
- Modify: `internal/app/audit.go` 如 audit action 常量或 helper 需要集中补充。
- Test: `internal/app/notification_test.go`
- Test: `internal/httpapi/notifications_test.go`

文档同步：

- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/hooks.md`
- Modify: `docs/manual/notifications.md`
- Modify: `docs/skills/wire-up-automation/references/hook-tools.md`
- Modify: `docs/skills/wire-up-automation/references/notification-tools.md`

---

## Chunk 1: 路由、权限和 API 基础

### Task 1: 新增 outbound API path 与 DTO

**Files:**
- Create: `web/src/features/workspace/outbound/outbound-api.ts`
- Create: `web/src/features/workspace/outbound/outbound-api.test.ts`
- Modify: `web/src/features/workspace/hooks/hooks-api.ts`

- [ ] **Step 1: Write failing API path tests**

Add tests for:

```ts
expect(notificationSinkPath()).toBe("/api/v1/notification-sinks")
expect(notificationSinkPath("s1")).toBe("/api/v1/notification-sinks/s1")
expect(notificationSinkEnablePath("s1")).toBe("/api/v1/notification-sinks/s1/enable")
expect(notificationSinkDisablePath("s1")).toBe("/api/v1/notification-sinks/s1/disable")
expect(notificationSinkTestPath("s1")).toBe("/api/v1/notification-sinks/s1/test")
expect(hookPath()).toBe("/api/v1/hooks")
expect(hookDeliveriesPath("h1")).toBe("/api/v1/hooks/h1/deliveries")
expect(notificationDeliveriesPath({ sink: "s1", status: "dead_lettered", limit: 20 }))
  .toBe("/api/v1/notification-deliveries?sink=s1&status=dead_lettered&limit=20")
```

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-api.test.ts`

Expected: FAIL because files do not exist.

- [ ] **Step 3: Implement outbound API helpers**

Implement types:

```ts
export type NotificationSink = {
  id: string
  workspace_id: string
  name: string
  type: "webhook" | "http_template" | string
  endpoint_mode: "static_url" | "template" | "config_value" | string
  url?: string
  url_template?: string
  config_key?: string
  allowed_hosts?: string[]
  http_method?: string
  header_templates?: Array<{ name: string; value: string }>
  body_template?: string
  body_content_type?: string
  secret_refs?: Array<{ alias: string; config_key: string }>
  enabled: boolean
  timeout_seconds: number
  max_attempts: number
  max_concurrency: number
  created_at: number
  modified_at: number
}
```

Move or re-export existing `Hook`, `HookDelivery`, `createHook`, `modifyHook`, `deleteHook`, `enableHook`, `disableHook`, `listHookDeliveries`, `replayHookDelivery` from the old hooks API to avoid two sources of truth.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-api.test.ts web/src/features/workspace/hooks/hooks-api.test.ts`

Expected: PASS.

### Task 2: 新增权限 helper

**Files:**
- Create: `web/src/features/workspace/outbound/outbound-permissions.ts`
- Create: `web/src/features/workspace/outbound/outbound-permissions.test.ts`

- [ ] **Step 1: Write failing permission tests**

Cover:

- owner + `hook:write` can write hooks.
- admin + `notification:write` can write sinks.
- wildcard `*` grants all scopes.
- member with write scope still cannot write hooks/sinks if role is not owner/admin and actor is not tenant token.
- tenant access token with matching write scope can write.
- missing scope is read-only.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-permissions.test.ts`

Expected: FAIL.

- [ ] **Step 3: Implement helpers**

Implement:

```ts
export type OutboundPermissionInput = {
  role?: string
  actorType?: string
  scopes?: string[] | null
}

export function canWriteHooks(input: OutboundPermissionInput): boolean
export function canWriteSinks(input: OutboundPermissionInput): boolean
export function canWriteReminderRules(input: OutboundPermissionInput): boolean
export function hasScope(scopes: string[] | null | undefined, scope: string): boolean
```

Keep front-end role checks conservative. Server remains final authority.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-permissions.test.ts`

Expected: PASS.

### Task 3: Route `/hooks`, `/notifications`, `/integrations` to one console

**Files:**
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/workspace/resources/resource-dispatch.tsx`
- Create: `web/src/features/workspace/outbound/outbound-console.tsx`
- Create: `web/src/features/workspace/outbound/outbound-console.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: Write failing console route tests**

Test `OutboundConsole` renders title and default tab:

- `initialTab="hooks"` shows Hooks tab selected.
- `initialTab="notification-rules"` or `"sinks"` works for `/notifications`.
- Read-only banner appears when no write permissions.

Router-level smoke can stay light; do not over-test TanStack internals.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-console.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement route dispatch**

In `ResourceDispatch`:

```tsx
case "hooks":
  return <OutboundConsole initialTab="hooks" workspaceSlug={workspaceSlug} />
case "notifications":
  return <OutboundConsole initialTab="notification-rules" workspaceSlug={workspaceSlug} />
```

In router, add `/integrations` as an alias route under `workspaceRootRoute` that renders the same resource dispatch or a thin `IntegrationsRoute`.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-console.test.tsx`
Run: `pnpm --dir web typecheck`

Expected: PASS.

---

## Chunk 2: Sink 管理闭环

### Task 4: Sink list 和行操作

**Files:**
- Create: `web/src/features/workspace/outbound/sinks/sink-list.tsx`
- Create: `web/src/features/workspace/outbound/sinks/sink-list.test.tsx`
- Modify: `web/src/features/workspace/outbound/outbound-console.tsx`

- [ ] **Step 1: Write failing list tests**

Mock `/api/v1/notification-sinks` and assert:

- Name/type/endpoint/status render.
- Disabled sink uses secondary badge.
- Write users see Edit / Enable / Disable / Test / Delete actions.
- Read-only users do not see write actions.
- Empty state explains that Hook needs a Sink first.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/sinks/sink-list.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement `SinkList`**

Use TanStack Query:

```ts
useQuery({ queryKey: ["outbound", "sinks"], queryFn: listNotificationSinks })
```

Actions:

- `enableNotificationSink(id)`
- `disableNotificationSink(id)`
- `deleteNotificationSink(id)` with existing dialog pattern or `AlertDialog`, not `window.confirm`.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/sinks/sink-list.test.tsx`

Expected: PASS.

### Task 5: Sink create/edit form

**Files:**
- Create: `web/src/features/workspace/outbound/sinks/sink-form-dialog.tsx`
- Create: `web/src/features/workspace/outbound/sinks/sink-form-dialog.test.tsx`
- Modify: `web/src/features/workspace/outbound/sinks/sink-list.tsx`

- [ ] **Step 1: Write failing form tests**

Cover:

- Default create form is `webhook + static_url`.
- `config_value` shows config key and allowed hosts.
- `http_template` shows header templates, body template, content type and secret refs.
- Editing a sink does not prefill secret plaintext.
- Submit sends expected JSON payload.
- Validation prevents missing name and missing endpoint.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/sinks/sink-form-dialog.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement form**

Use existing shadcn components. Keep layout dense and operational:

- `Select` for type and endpoint mode.
- `Input` for name/url/config key/allowed hosts.
- `Textarea` for body template.
- Repeatable rows for headers and secret refs.

Normalize comma-separated allowed hosts into `string[]`.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/sinks/sink-form-dialog.test.tsx web/src/features/workspace/outbound/sinks/sink-list.test.tsx`

Expected: PASS.

---

## Chunk 3: Hook 管理闭环

### Task 6: Event type constants and tests

**Files:**
- Create: `web/src/features/workspace/outbound/outbound-event-types.ts`
- Create: `web/src/features/workspace/outbound/outbound-event-types.test.ts`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: Write failing tests**

Assert event list equals current backend whitelist:

```ts
[
  "task.created",
  "task.modified",
  "task.completed",
  "task.deleted",
  "task.started",
  "task.stopped",
  "task.assigned",
  "task.unassigned",
  "task.blocked",
  "task.due_changed",
  "task.priority_changed",
  "task.project_changed",
  "task.tags_changed",
  "task.unblocked",
  "project.archived",
  "project.transitioned",
  "project.annotated",
  "project.denotated",
]
```

Also assert no label/placeholder contains `task.done`.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-event-types.test.ts`

Expected: FAIL.

- [ ] **Step 3: Implement constants**

Group as:

- `task.basic`
- `task.status`
- `task.fields`
- `project`

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/outbound-event-types.test.ts`

Expected: PASS.

### Task 7: Hook form with sink dropdown and project selector

**Files:**
- Create: `web/src/features/workspace/outbound/hooks/hook-form-dialog.tsx`
- Create: `web/src/features/workspace/outbound/hooks/hook-form-dialog.test.tsx`
- Modify: `web/src/features/workspace/outbound/hooks/hook-list.tsx`

- [ ] **Step 1: Write failing tests**

Mock sinks and projects. Assert:

- Sink is selected from list, not typed freeform.
- Event checkboxes submit `event_types`.
- `scope_type=project` requires `project_ref`.
- Editing existing hook preselects events and sink.
- `timeout_seconds` and `max_attempts` submit when changed.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/hooks/hook-form-dialog.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement form**

Fetch:

- Sinks via `listNotificationSinks({ includeDisabled: true })`.
- Projects via existing `listProjects` helper if available; otherwise add minimal API helper in outbound-api using `/api/v1/projects?workspace=<slug>&status=all`.

Payload:

```ts
{
  name,
  scope_type,
  project_ref: scopeType === "project" ? projectRef : undefined,
  sink: sinkId,
  event_types: selectedEvents,
  timeout_seconds,
  max_attempts,
}
```

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/hooks/hook-form-dialog.test.tsx`

Expected: PASS.

### Task 8: Hook list, edit, enable/disable/delete

**Files:**
- Create: `web/src/features/workspace/outbound/hooks/hook-list.tsx`
- Create: `web/src/features/workspace/outbound/hooks/hook-list.test.tsx`
- Modify: `web/src/features/workspace/outbound/outbound-console.tsx`
- Modify: `web/src/features/workspace/hooks/hook-console.tsx`

- [ ] **Step 1: Write failing tests**

Assert:

- Hook list renders name, scope, event count, sink name, enabled badge.
- Row expands and loads deliveries.
- Edit opens form.
- Disable/enable call correct endpoint.
- Delete uses confirmation dialog.
- Read-only hides write actions.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/hooks/hook-list.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement list**

Move useful logic from `web/src/features/workspace/hooks/hook-console.tsx` into outbound components. Leave old `HookConsole` as:

```tsx
export function HookConsole({ canWrite }: { canWrite: boolean }) {
  return <OutboundConsole initialTab="hooks" forceCanWrite={canWrite} />
}
```

If `forceCanWrite` feels awkward, remove `HookConsole` from dispatch and keep only for tests until deleted.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/hooks/hook-list.test.tsx web/src/features/workspace/hooks/hook-console.test.tsx`

Expected: PASS or update/delete old hook-console tests if the wrapper makes them obsolete.

---

## Chunk 4: Delivery 详情与 replay

### Task 9: Hook delivery table and detail dialog

**Files:**
- Create: `web/src/features/workspace/outbound/deliveries/hook-delivery-table.tsx`
- Create: `web/src/features/workspace/outbound/deliveries/delivery-detail-dialog.tsx`
- Create: `web/src/features/workspace/outbound/deliveries/hook-delivery-table.test.tsx`
- Modify: `web/src/features/workspace/outbound/hooks/hook-list.tsx`

- [ ] **Step 1: Write failing tests**

Assert:

- Status badge variants for `succeeded`, `retry_wait`, `dead_lettered`, `disabled_skipped`.
- Detail button fetches `/api/v1/hook-deliveries/{id}`.
- Payload and headers render as formatted JSON.
- Replay button only appears for failed/dead-letter statuses and write users.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/deliveries/hook-delivery-table.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement delivery components**

Use `<pre>` or existing JSON rendering pattern. Keep payload readable and copyable. Do not invent new syntax highlighting dependency.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/deliveries/hook-delivery-table.test.tsx`

Expected: PASS.

### Task 10: Notification delivery detail support

**Files:**
- Modify: `web/src/features/workspace/outbound/outbound-api.ts`
- Modify: `web/src/features/workspace/outbound/deliveries/delivery-detail-dialog.tsx`
- Create: `web/src/features/workspace/outbound/deliveries/notification-delivery-table.tsx`
- Create: `web/src/features/workspace/outbound/deliveries/notification-delivery-table.test.tsx`

- [ ] **Step 1: Write failing tests**

Assert notification delivery details include:

- recipient.
- resolved endpoint fingerprint.
- rendered method / headers / body / content type.
- event/object fields.
- replay button for failed statuses.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/deliveries/notification-delivery-table.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement notification delivery helpers**

Use:

- `GET /api/v1/notification-deliveries?sink=&status=&limit=`
- `GET /api/v1/notification-deliveries/{deliveryID}`
- `POST /api/v1/notification-deliveries/{deliveryID}/replay`

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/deliveries/notification-delivery-table.test.tsx`

Expected: PASS.

---

## Chunk 5: 后端 sink test API

### Task 11: App 层测试投递契约

**Files:**
- Modify: `internal/app/notification.go`
- Test: `internal/app/notification_test.go`

- [ ] **Step 1: Write failing app tests**

Add tests:

- owner with `PermissionNotificationWrite` can test a static webhook sink against `httptest.Server`.
- target returning 500 yields `Status: "failed"` and `StatusCode: 500`, not an app error.
- unknown sink returns `notification_sink_not_found`.
- foreign workspace sink returns `notification_sink_not_found`.
- loopback/private host is blocked by the same network guard used by real dispatch. For `httptest.Server`, follow existing test bypass pattern if current code has one; do not weaken production guard.
- response and audit do not contain raw secret.
- `project_ref` is accepted and used for config_value endpoint resolution.

- [ ] **Step 2: Run red tests**

Run: `go test ./internal/app -run 'TestNotificationSinkTest' -count=1`

Expected: FAIL.

- [ ] **Step 3: Add app types**

Add:

```go
type NotificationSinkTestInput struct {
    Kind       string
    EventType  string
    Sample     string
    ProjectRef string
}

type NotificationSinkTestView struct {
    Status                      string
    StatusCode                  *int
    DurationMS                  int64
    ResolvedEndpointSource      string
    ResolvedEndpointFingerprint string
    RenderedMethod              string
    RenderedHeaders             map[string][]string
    RenderedBodyPreview         string
    Error                       string
}
```

- [ ] **Step 4: Implement `TestNotificationSink`**

Add method:

```go
func (s *Service) TestNotificationSink(sinkID string, input NotificationSinkTestInput) (NotificationSinkTestView, error)
```

Rules:

- Require `PermissionNotificationWrite`.
- Resolve sink in current workspace.
- Validate `kind` is `hook` or `notification`; default `hook`.
- Validate `event_type` against existing hook event whitelist for `kind=hook`.
- Build a sample payload; keep it deterministic.
- Reuse existing sink render / HTTP send helpers. If helpers are dispatcher-private, extract the smallest shared renderer/sender into app or runtime package without changing production behavior.
- Do not insert into delivery tables.
- Write audit action `notification.sink.test`.
- Add `X-Xuanchu-Test: true`.

- [ ] **Step 5: Run app tests**

Run: `go test ./internal/app -run 'TestNotificationSinkTest|TestNotificationSink' -count=1`

Expected: PASS.

### Task 12: HTTP route for sink test

**Files:**
- Modify: `internal/httpapi/notifications.go`
- Modify: `internal/httpapi/huma_routes.go`
- Test: `internal/httpapi/notifications_test.go`

- [ ] **Step 1: Write failing HTTP tests**

Add tests:

- `POST /api/v1/notification-sinks/{sinkID}/test` returns test result.
- Requires `notification:write`.
- Bad JSON returns `api_bad_json`.
- Cross-workspace sink is not found.
- Response has no secret fields.

- [ ] **Step 2: Run red tests**

Run: `go test ./internal/httpapi -run 'TestNotificationSinkTest' -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement request/response and route**

In `notifications.go` add:

```go
type notificationSinkTestRequest struct {
    Kind       string `json:"kind"`
    EventType  string `json:"event_type"`
    Sample     string `json:"sample"`
    ProjectRef string `json:"project_ref"`
}
```

Handler:

```go
func (s *Server) handleNotificationSinkTest(w http.ResponseWriter, r *http.Request)
```

Register in `huma_routes.go`:

```go
{Method: http.MethodPost, Path: "/api/v1/notification-sinks/{sinkID}/test", Tag: "Notification Sinks", Summary: "Test a notification sink.", Handler: s.handleNotificationSinkTest},
```

- [ ] **Step 4: Run HTTP tests**

Run: `go test ./internal/httpapi -run 'TestNotificationSinkTest|TestNotificationSink' -count=1`

Expected: PASS.

---

## Chunk 6: Sink test UI

### Task 13: Sink test dialog

**Files:**
- Create: `web/src/features/workspace/outbound/sinks/sink-test-dialog.tsx`
- Create: `web/src/features/workspace/outbound/sinks/sink-test-dialog.test.tsx`
- Modify: `web/src/features/workspace/outbound/sinks/sink-list.tsx`
- Modify: `web/src/features/workspace/outbound/outbound-api.ts`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: Write failing tests**

Assert:

- Test dialog defaults to `kind=hook`, `event_type=task.completed`.
- Optional project selector is visible for config_value sinks.
- Submit calls `/api/v1/notification-sinks/{id}/test`.
- Success renders status code, duration, endpoint fingerprint.
- Failed target renders error/status without closing dialog.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/sinks/sink-test-dialog.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement dialog**

Use mutation:

```ts
useMutation({
  mutationFn: (input: NotificationSinkTestInput) => testNotificationSink(sink.id, input),
})
```

Do not store test history locally except current dialog result.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/sinks/sink-test-dialog.test.tsx web/src/features/workspace/outbound/sinks/sink-list.test.tsx`

Expected: PASS.

---

## Chunk 7: Rules tabs

### Task 14: Notification rule list and form

**Files:**
- Create: `web/src/features/workspace/outbound/rules/notification-rule-list.tsx`
- Create: `web/src/features/workspace/outbound/rules/notification-rule-list.test.tsx`
- Modify: `web/src/features/workspace/outbound/outbound-api.ts`
- Modify: `web/src/features/workspace/outbound/outbound-console.tsx`

- [ ] **Step 1: Write failing tests**

Assert:

- Lists rules from `/api/v1/notification-rules`.
- Create form supports name, project_ref, event_type, filter_source, audience_type, recipients, sink, template subject/body.
- Enable/disable/delete work.
- Read-only hides write actions.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/rules/notification-rule-list.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement notification rules**

Keep form simple and text-first. Recipient picker can initially be comma-separated user refs if no reusable member selector exists; document this as a Phase 3 UI simplification in code comments only if needed.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/rules/notification-rule-list.test.tsx`

Expected: PASS.

### Task 15: Reminder rule list and form

**Files:**
- Create: `web/src/features/workspace/outbound/rules/reminder-rule-list.tsx`
- Create: `web/src/features/workspace/outbound/rules/reminder-rule-list.test.tsx`
- Modify: `web/src/features/workspace/outbound/outbound-api.ts`
- Modify: `web/src/features/workspace/outbound/outbound-console.tsx`

- [ ] **Step 1: Write failing tests**

Assert:

- Lists rules from `/api/v1/reminder-rules`.
- Create form prioritizes schedule model: schedule_type, schedule_value, filter_source.
- Supports audience_type, recipients, sink_ref.
- Enable/disable/delete work.

- [ ] **Step 2: Run red tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/rules/reminder-rule-list.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement reminder rules**

Default create form:

- `schedule_type=daily_at`
- `schedule_value=08:50`
- `filter_source=status:pending`
- `audience_type=assignees`

Do not expose unsupported audiences as selectable.

- [ ] **Step 4: Run green tests**

Run: `pnpm --dir web test web/src/features/workspace/outbound/rules/reminder-rule-list.test.tsx`

Expected: PASS.

---

## Chunk 8: Docs, regression and release readiness

### Task 16: Documentation sync

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/hooks.md`
- Modify: `docs/manual/notifications.md`
- Modify: `docs/skills/wire-up-automation/references/hook-tools.md`
- Modify: `docs/skills/wire-up-automation/references/notification-tools.md`
- Modify: `docs/superpowers/specs/2026-07-05-web-console-outbound-integration-console-design.md` if implementation decisions changed.

- [ ] **Step 1: Update docs**

Document:

- `/hooks` is the出站集成控制台.
- `/notifications` reuses the same console and opens notification/rule context.
- `/integrations` alias exists.
- Sink test behavior: writes audit, not delivery table.
- `task.completed` is the correct event name.

- [ ] **Step 2: Check stale terms**

Run:

```bash
rg -n "task\\.done|只读 sink|只读 DataTable|Hook 控制台.*只读" README.md ROADMAP.md docs web/src
```

Expected: No stale user-facing guidance except tests intentionally asserting absence.

### Task 17: Full verification

**Files:** no planned edits.

- [ ] **Step 1: Frontend focused tests**

Run:

```bash
pnpm --dir web test web/src/features/workspace/outbound
```

Expected: PASS.

- [ ] **Step 2: Frontend full checks**

Run:

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web build
```

Expected: PASS.

- [ ] **Step 3: Backend checks**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS.

- [ ] **Step 4: Diff hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only files related to this plan are changed.

### Task 18: Commit sequence

Use small Chinese commits. Suggested order:

```bash
git add web/src/features/workspace/outbound web/src/features/workspace/resources/resource-dispatch.tsx web/src/routes/router.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 新增出站集成控制台"

git add internal/app/notification.go internal/app/notification_test.go internal/httpapi/notifications.go internal/httpapi/notifications_test.go internal/httpapi/huma_routes.go
git commit -m "feat: 支持 sink 测试投递"

git add README.md ROADMAP.md docs/manual/hooks.md docs/manual/notifications.md docs/skills/wire-up-automation/references/hook-tools.md docs/skills/wire-up-automation/references/notification-tools.md docs/superpowers/specs/2026-07-05-web-console-outbound-integration-console-design.md
git commit -m "docs: 同步出站集成控制台说明"
```

Before committing, re-check unrelated dirty files and do not stage them.
