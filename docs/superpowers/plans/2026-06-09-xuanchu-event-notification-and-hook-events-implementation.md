# Xuanchu 事件通知与 Hook 事件扩展实现计划

> **给 agentic workers 的要求：** 执行本计划时必须使用 `superpowers:subagent-driven-development`（如果当前环境支持子代理）或 `superpowers:executing-plans`。步骤使用 checkbox（`- [ ]`）语法跟踪进度。

**目标：** 实现 sink 化 Hook、补齐项目 annotation / 依赖解除阻塞事件，并新增基于事件的 notification rule。

**架构：** 把事件从 Hook 专属概念收敛为 app 层统一事件原语；Hook 和 event notification rule 都消费同一事件流，但 Hook 投递原始事件 envelope，notification rule 解析 audience 并投递面向人的消息。外部投递统一通过 workspace 级 outbound sink 解析，首版复用现有 `notification_sinks` 表和 endpoint/template 渲染能力，Hook 不再接受直接 URL。

**技术栈：** Go 1.25、Cobra、GORM、SQLite/PostgreSQL、chi HTTP router、MCP Go SDK、现有 notification sink / delivery / dispatcher。

---

## 前置约束

- 文档对应规格：`docs/superpowers/specs/2026-06-09-xuanchu-event-notification-and-hook-events-design.md`
- 当前能力尚未上线，不需要兼容 `hook add --url`。
- 规格中的“下一大版本候选事件”和“任务事件 payload 基线”只作为后续版本补充，不进入本轮实现范围。
- 所有文档和用户可见描述必须使用中文。
- 所有 MCP tool 名必须使用下划线。
- `sink-ref` 是 workspace 级资源引用：
  - name 只在当前 workspace 内解析。
  - UUID 也必须校验 `workspace_id == 当前 workspace`。
  - Hook / Notification Rule 保存 `sink_id`，不保存 sink name。
- 每个实现 chunk 后建议提交一次，提交信息使用中文。

## 文件结构

新增文件：

- `internal/app/event_notification.go`：event notification rule 的 app 层创建、修改、匹配、delivery enqueue。
- `internal/app/event_notification_test.go`：event notification rule、sink workspace 隔离、audience、dedupe 测试。
- `internal/storage/event_notification_rule_repo.go`：event notification rule repository。
- `internal/storage/event_notification_rule_repo_test.go`：rule CRUD、workspace 隔离、唯一索引测试。
- `internal/httpapi/notification_rules.go`：event notification rule HTTP handlers。
- `internal/httpapi/notification_rules_test.go`：HTTP rule 管理与跨 workspace sink 测试。
- `internal/remote/notification_rule.go`：remote client 的 event notification rule DTO 和方法。

修改文件：

- `internal/storage/models.go`：`HookDefinition` 改为 `SinkID`；`HookDelivery` 增加冻结后的 resolved endpoint / method / body / content type；新增 `EventNotificationRule`；`NotificationDelivery.TaskUUID` 调整为可空或新增 object 字段。
- `internal/storage/migrate_sqlite.go` / `internal/storage/migrate_postgres.go`：迁移 hook sink 字段、hook delivery 冻结字段、event notification rule 表、notification delivery task 可空。
- `internal/storage/hook_repo.go` / `internal/storage/hook_repo_test.go`：Hook repository 使用 sink 字段，测试不再依赖 endpoint URL。
- `internal/app/hook.go` / `internal/app/hook_test.go`：HookAddInput / HookModifyInput 改为 `SinkRef`，删除 URL/secret 输入；允许新事件。
- `internal/app/hook_event.go`：project annotation payload 补齐 project/annotation；新增 `task.unblocked` builder；enqueue 时同时处理 Hook 和 event notification rule。
- `internal/app/notification.go` / `internal/app/notification_endpoint.go`：抽出通用 sink 解析和 event 模板上下文；继续保持 reminder rule 行为。
- `internal/app/notification_scheduler.go`：适配 `NotificationDelivery.TaskUUID` 可空或 object 字段变化。
- `internal/app/service.go`：`Done` 生成 `task.completed` 和必要的 `task.unblocked` 事件。
- `internal/hookruntime/dispatcher.go` / `internal/hookruntime/signer.go`：Hook dispatcher 使用 delivery 中冻结的请求快照，不再读取 hook endpoint URL/secret。
- `internal/cli/hook.go` / `internal/cli/hook_test.go`：Hook CLI 使用 `--sink`，移除 `--url` / `--secret`。
- `internal/cli/notification.go` / `internal/cli/notification_test.go`：新增 `notification rule` 子命令。
- `internal/httpapi/hooks.go` / `internal/httpapi/hooks_test.go` / `internal/httpapi/hook_e2e_test.go`：Hook API 使用 `sink` / `sink_id`，拒绝 `url`。
- `internal/mcpserver/tools_hook.go` / `internal/mcpserver/tools_notification.go`：Hook tool 使用 sink；新增 notification rule tools。
- `internal/mcpserver/testdata/*.json` / `internal/mcpserver/integration_test.go`：更新 schema 和集成测试。
- `README.md`、`ROADMAP.md`、`docs/manual/hooks.md`、`docs/manual/notifications.md`、`docs/manual/mcp.md`、`docs/skills/*`：同步用户可见文档。
- `docs/openapi/xuanchu-v1.yaml`：同步 HTTP API。

---

## Chunk 1: Workspace 级 outbound sink 与 Hook sink 化

### Task 1: 存储层让 Hook 引用 sink

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`
- Modify: `internal/storage/hook_repo_test.go`

- [ ] **Step 1: 写失败测试**

在 `internal/storage/hook_repo_test.go` 更新 helper，让 `HookDefinition` 必须包含 `SinkID`，并新增测试：

```go
func TestHookDefinitionStoresSinkID(t *testing.T) {
	store := openTestStore(t)
	wsID := "ws-hook-sink"
	sink := makeNotificationSink(wsID, "audit-stream")
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatalf("Create sink error = %v", err)
	}
	repo := storage.NewHookRepository(store.DB())
	hook := makeHookDef(t, wsID, "audit", `["task.created"]`, func(h *storage.HookDefinition) {
		h.SinkID = sink.ID
	})
	if err := repo.Create(hook); err != nil {
		t.Fatalf("Create hook error = %v", err)
	}
	got, err := repo.GetByID(hook.ID)
	if err != nil {
		t.Fatalf("GetByID error = %v", err)
	}
	if got.SinkID != sink.ID {
		t.Fatalf("SinkID = %q, want %q", got.SinkID, sink.ID)
	}
}
```

同时删除或改写断言 `EndpointURL` / `Secret` 保持不变的测试；新测试 helper 不应再为 Hook 填 URL/secret。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/storage -run 'TestHookDefinitionStoresSinkID|TestHookRepository' -count=1
```

预期：FAIL，`SinkID` 字段不存在或迁移缺列。

- [ ] **Step 3: 最小实现**

在 `storage.HookDefinition` 中：

```go
SinkID string `gorm:"not null;index"`
```

删除或停止使用：

```go
EndpointURL string
Secret string
```

如果为了迁移代码过渡需要保留字段，必须在同一 chunk 结束前从 app/HTTP/CLI/MCP 对外模型中移除，且新建 hook 不再写入它们。

SQLite/PostgreSQL AutoMigrate 应覆盖新增字段。由于未上线，不需要为旧数据回填 URL 到 sink。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/storage -run 'TestHookDefinitionStoresSinkID|TestHookRepository|TestHookDelivery' -count=1
```

预期：PASS。

### Task 2: app 层 Hook 输入改为 `SinkRef`

**Files:**
- Modify: `internal/app/hook.go`
- Modify: `internal/app/hook_test.go`
- Modify: `internal/app/notification.go`

- [ ] **Step 1: 写失败测试**

在 `internal/app/hook_test.go` 添加：

```go
func TestAddHookRequiresSink(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.AddHook(HookAddInput{
		Name: "audit",
		EventTypes: []string{"task.created"},
	})
	assertRuntimeErrorCode(t, err, "hook_sink_required")
}

func TestAddHookRejectsCrossWorkspaceSink(t *testing.T) {
	svcA := newTestServiceInWorkspace(t, "alpha")
	svcB := newTestServiceInWorkspace(t, "beta")
	sinkB, err := svcB.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatalf("AddNotificationSink(B) error = %v", err)
	}
	_, err = svcA.AddHook(HookAddInput{
		Name: "audit",
		EventTypes: []string{"task.created"},
		SinkRef: sinkB.ID,
	})
	assertRuntimeErrorCode(t, err, "notification_sink_not_found")
}
```

并把所有 `defaultHookInput()` 改为先创建 sink，再填 `SinkRef`。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/app -run 'TestAddHook.*Sink|TestHook' -count=1
```

预期：FAIL，`HookAddInput.SinkRef` 未定义或旧 URL 校验仍生效。

- [ ] **Step 3: 最小实现**

修改 app 结构：

```go
type HookAddInput struct {
	Name string
	ScopeType HookScopeType
	ProjectRef string
	EventTypes []string
	SinkRef string
	TimeoutSeconds int
	MaxAttempts int
}

type HookModifyInput struct {
	Name *string
	EventTypes *[]string
	SinkRef *string
	TimeoutSeconds *int
	MaxAttempts *int
}
```

实现规则：

- `AddHook` 必须调用 `resolveNotificationSink(input.SinkRef)`。
- `resolveNotificationSink` 已按 `s.workspaceID` 校验 UUID/name；直接复用。
- sink disabled 不影响创建 Hook，但 dispatcher 发送时应 skipped 或 dead-letter；本计划在 dispatcher chunk 固定。
- `HookView` 增加 `SinkID`、`SinkName`、`SinkType`，移除 `EndpointURL`。
- audit payload 写 `sink_id` / `sink_name`，不写 endpoint URL / secret。
- `validateHookEndpointLength`、`validateHookSecret` 对 Hook 创建/修改不再使用；若无其他调用，可删除。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/app -run 'TestAddHook|TestModifyHook|TestHook|TestProject.*Hook|TestHookDelivery' -count=1
```

预期：PASS。

### Task 3: Hook CLI / HTTP / Remote / MCP 输入改为 sink

**Files:**
- Modify: `internal/cli/hook.go`
- Modify: `internal/cli/hook_test.go`
- Modify: `internal/httpapi/hooks.go`
- Modify: `internal/httpapi/hooks_test.go`
- Modify: `internal/remote/hook.go`
- Modify: `internal/mcpserver/tools_hook.go`
- Modify: `internal/mcpserver/testdata/hook_add.schema.json`
- Modify: `internal/mcpserver/testdata/hook_modify.schema.json`
- Modify: `internal/mcpserver/testdata/list-tools-default.json`

- [ ] **Step 1: 写失败测试**

测试要求：

```bash
xuanchu hook add audit --event task.created --sink audit-stream
```

成功。

```bash
xuanchu hook add audit --event task.created --url https://example.test/hook
```

失败，错误码或错误文本包含 `hook_url_not_supported` / `--sink`。

HTTP:

```json
{"name":"audit","event_types":["task.created"],"sink":"audit-stream"}
```

成功。

```json
{"name":"audit","event_types":["task.created"],"url":"https://example.test/hook"}
```

失败。

MCP `hook_add` schema 不再包含 `url`，包含 `sink`。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/cli ./internal/httpapi ./internal/mcpserver -run 'Test.*Hook' -count=1
```

预期：FAIL，旧 `url` 参数仍存在。

- [ ] **Step 3: 最小实现**

CLI：

- `hook add` 增加 `--sink` required。
- 移除 `--url`、`--secret`、`--secret-stdin`、`--secret-file`。
- `hook modify` 支持 `--sink`，移除 URL/secret 修改。
- human/JSON 输出显示 sink 信息。

HTTP/Remote：

- request DTO 使用 `sink` / `sink_id`，不接受 `url`。
- response DTO 输出 `sink_id`、`sink_name`、`sink_type`。

MCP：

- `HookAddInput` 增加 `Sink string`，删除 `URL`/`Secret`。
- `HookModifyInput` 增加 `Sink *string`，删除 `URL`/`Secret`。
- schema 重新生成或更新 golden。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/cli ./internal/httpapi ./internal/remote ./internal/mcpserver -run 'Test.*Hook|TestListTools' -count=1
```

预期：PASS。

### Task 4: 提交 Chunk 1

- [ ] **Step 1: 检查 diff**

Run:

```bash
git diff -- internal/storage internal/app internal/cli internal/httpapi internal/remote internal/mcpserver
```

预期：Hook 对外入口不再暴露 `url` / `secret`。

- [ ] **Step 2: 提交**

```bash
git add internal/storage internal/app internal/cli internal/httpapi internal/remote internal/mcpserver
git commit -m "feat: 将 Hook 改为 workspace sink 投递"
```

---

## Chunk 2: Hook delivery 冻结 sink 渲染结果

### Task 1: HookDelivery 增加请求快照字段

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/hook_repo_test.go`
- Modify: `internal/app/hook_event.go`
- Modify: `internal/app/hook_test.go`

- [ ] **Step 1: 写失败测试**

在 hook delivery 入队测试中断言字段：

```go
if delivery.ResolvedURL == "" {
	t.Fatal("ResolvedURL empty")
}
if delivery.RenderedMethod != "POST" {
	t.Fatalf("RenderedMethod = %q, want POST", delivery.RenderedMethod)
}
if delivery.RenderedBody == "" {
	t.Fatal("RenderedBody empty")
}
if !strings.Contains(delivery.PayloadJSON, `"event_type":"task.created"`) {
	t.Fatalf("PayloadJSON missing event envelope: %s", delivery.PayloadJSON)
}
```

新增 HTTP template sink 测试：Hook 使用 `http_template` sink 时，delivery 冻结 header/body 模板结果。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/storage ./internal/app -run 'TestHookDelivery|Test.*HTTPTemplate.*Hook' -count=1
```

预期：FAIL，字段不存在或未填。

- [ ] **Step 3: 最小实现**

`storage.HookDelivery` 增加：

```go
SinkID string
ResolvedURL string
ResolvedEndpointSource string
ResolvedEndpointFingerprint string
RenderedMethod string
RenderedHeadersJSON string
RenderedBody string
RenderedContentType string
```

保留 `PayloadJSON` 作为事件 envelope 快照。

`enqueueHookEvents` 对每个匹配 Hook：

1. 加载 hook 的 sink。
2. 解析 workspace/project/actor context。
3. 用通用 sink resolver 渲染请求快照。
4. 写入 HookDelivery。

建议新增 `ResolveHookRequest` 或扩展 `ResolveNotificationRequest`：

- Hook 模板上下文包含 `event.*`、`workspace.*`、`project.*`、`actor.*`。
- `event.json` 返回完整 event envelope JSON。
- Hook 默认 body 是 event envelope JSON。
- Hook 使用的 sink 可以引用 `secret.*`，secret ref 仍由 sink 自己声明；Hook 不再持有独立 secret。
- `http_template` sink 的 `HTTPMethod` 应被尊重；如果为空才默认 POST。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/app ./internal/storage -run 'TestHookDelivery|Test.*Hook.*Template|TestNotificationEndpoint' -count=1
```

预期：PASS。

### Task 2: Hook dispatcher 使用冻结快照

**Files:**
- Modify: `internal/hookruntime/dispatcher.go`
- Modify: `internal/hookruntime/signer.go`
- Modify: `internal/hookruntime/dispatcher_test.go`
- Modify: `internal/httpapi/hook_e2e_test.go`

- [ ] **Step 1: 写失败测试**

更新 dispatcher 测试：

- dispatcher 不读取 `hook.EndpointURL`。
- 请求 URL 来自 `delivery.ResolvedURL`。
- 请求 body 来自 `delivery.RenderedBody`；如果为空则退回 `PayloadJSON`。
- headers 合并 `delivery.RenderedHeadersJSON` 和基础 `X-Xuanchu-*` headers。
- sink disabled 时标记 `disabled_skipped`。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/hookruntime ./internal/httpapi -run 'TestHook.*Dispatch|Test.*Webhook|Test.*Hook.*E2E' -count=1
```

预期：FAIL，dispatcher 仍读取 hook endpoint。

- [ ] **Step 3: 最小实现**

Dispatcher 逻辑：

1. 加载 hook，校验 enabled。
2. 加载 sink，校验 workspace 一致。
3. sink disabled 则 `MarkDisabledSkipped`。
4. SSRF 校验 `delivery.ResolvedURL`。
5. method 使用 `delivery.RenderedMethod`，默认 POST。
6. body 使用 `delivery.RenderedBody`，为空时使用 `PayloadJSON`。
7. headers 使用 `delivery.RenderedHeadersJSON`，再叠加基础 hook headers。
8. 签名密钥使用 sink secret，而不是 hook secret。

`HeadersForDelivery` 参数改为接收 sink 或 secret 字符串。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/hookruntime ./internal/httpapi -run 'TestHook.*Dispatch|Test.*Webhook|Test.*Hook.*E2E|TestHookSecret' -count=1
```

预期：PASS。

### Task 3: 提交 Chunk 2

```bash
git add internal/storage internal/app internal/hookruntime internal/httpapi
git commit -m "feat: 冻结 Hook sink 投递请求"
```

---

## Chunk 3: 补齐项目 annotation 事件与 `task.unblocked`

### Task 1: 允许并完善 project annotation 事件

**Files:**
- Modify: `internal/app/hook.go`
- Modify: `internal/app/hook_event.go`
- Modify: `internal/app/project.go`
- Modify: `internal/app/hook_test.go`
- Modify: `internal/app/project_test.go`

- [ ] **Step 1: 写失败测试**

测试：

- `AddHook` 接受 `project.annotated`。
- `AddHook` 接受 `project.denotated`。
- `ProjectAnnotate` 生成 delivery，payload `data.project` 和 `data.annotation` 存在。
- `ProjectDenotate` 生成 delivery，payload `data.project` 和 `data.annotation.id` 存在。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/app -run 'Test.*Project.*Hook|TestAddHook.*Project|TestProjectAnnotate|TestProjectDenotate' -count=1
```

预期：FAIL，事件白名单或 payload 不完整。

- [ ] **Step 3: 最小实现**

- `allowedHookEventTypes` 增加 `project.annotated`、`project.denotated`、`task.unblocked`。
- `buildProjectAnnotatedHookEvent` payload 改为：
  - `project`: `projectViewToMap(pv)`
  - `annotation`: id、entry、content、content_preview、created_by、created_at
- `buildProjectDenotatedHookEvent` payload 改为：
  - `project`
  - `annotation.id`

如果 annotation `CreatedBy` 还只是 ID，必须在 app 层批量解析为 `task.UserInfo` 后再放入 event payload；不得在对外 payload 中输出裸 UUID。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/app -run 'Test.*Project.*Hook|TestAddHook.*Project|TestProjectAnnotate|TestProjectDenotate' -count=1
```

预期：PASS。

### Task 2: 生成 `task.unblocked`

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/hook_event.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/app/hook_test.go`

- [ ] **Step 1: 写失败测试**

覆盖：

```go
func TestDoneGeneratesTaskUnblockedForLastDependency(t *testing.T) { ... }
func TestDoneDoesNotGenerateTaskUnblockedWhenOtherBlockersRemain(t *testing.T) { ... }
func TestDoneDoesNotGenerateTaskUnblockedForCompletedDependent(t *testing.T) { ... }
```

断言：

- 完成最后一个依赖后生成 `task.unblocked` delivery。
- payload `data.task.uuid` 是被解除阻塞的任务。
- payload `data.dependency.completed_task.uuid` 是刚完成的任务。
- 仍有其他未完成依赖时不生成。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/app -run 'TestDone.*Unblocked|TestHookDeliveryEnqueuedOnTaskCompleted' -count=1
```

预期：FAIL，`task.unblocked` 尚未生成。

- [ ] **Step 3: 最小实现**

实现建议：

- 在 `Done` 的 transaction 内，先读取当前 workspace 任务快照并计算 `beforeBlocked`，再执行 `doneLocked`，然后重新读取任务快照并计算 `afterBlocked`。
- 为避免逻辑分散，可复用现有 `buildDependencyState`；性能优化可以后续再做针对 `depends_on = doneTask.UUID` 的 repository 查询。
- 从更新后快照中找出 `Depends` 包含 `doneTask.UUID` 的候选 dependent task。
- `beforeBlocked[dependent.UUID] == true`、`afterBlocked[dependent.UUID] == false`、状态 pending/waiting、未 until expired，则生成 `buildTaskUnblockedHookEvent(dependent, doneTask, runtime, now)`。
- `Done` 返回事件列表：`task.completed` + 多个 `task.unblocked`。

注意：

- recurring parent 生成下一条任务的逻辑不能被破坏。
- project invariant 校验仍然有效。
- 不要在 CLI/HTTP/MCP 层生成事件。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/app -run 'TestDone.*Unblocked|TestHookDeliveryEnqueuedOnTaskCompleted|TestDependency' -count=1
```

预期：PASS。

### Task 3: 提交 Chunk 3

```bash
git add internal/app
git commit -m "feat: 补齐项目与解除阻塞事件"
```

---

## Chunk 4: 事件通知规则与 notification delivery

### Task 1: 存储层新增 event notification rule

**Files:**
- Modify: `internal/storage/models.go`
- Create: `internal/storage/event_notification_rule_repo.go`
- Create: `internal/storage/event_notification_rule_repo_test.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`

- [ ] **Step 1: 写失败测试**

测试：

- `(workspace_id, name)` 唯一。
- `ListMatching(workspaceID, projectID, eventType)` 只返回 enabled、同 workspace、事件匹配、project 匹配的 rule。
- 跨 workspace 不返回。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/storage -run 'TestEventNotificationRule' -count=1
```

预期：FAIL，repo/model 不存在。

- [ ] **Step 3: 最小实现**

新增模型：

```go
type EventNotificationRule struct {
	ID string `gorm:"primaryKey"`
	WorkspaceID string `gorm:"not null;index;uniqueIndex:idx_event_notification_rules_ws_name,priority:1"`
	ProjectID *string `gorm:"index"`
	Name string `gorm:"not null;uniqueIndex:idx_event_notification_rules_ws_name,priority:2"`
	Enabled *bool `gorm:"not null;default:true;index"`
	EventType string `gorm:"not null;index"`
	FilterSource string `gorm:"not null;default:''"`
	AudienceType string `gorm:"not null"`
	RecipientUserIDsJSON string `gorm:"not null;default:'[]'"`
	SinkID string `gorm:"not null;index"`
	TemplateSubject string `gorm:"not null;default:''"`
	TemplateBody string `gorm:"not null;default:''"`
	CreatedBy string `gorm:"not null;index"`
	CreatedAt int64 `gorm:"not null"`
	ModifiedAt int64 `gorm:"not null"`
}
```

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/storage -run 'TestEventNotificationRule|TestDBMigrates' -count=1
```

预期：PASS。

### Task 2: notification delivery 支持事件对象

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/notification_delivery_repo.go`
- Modify: `internal/storage/notification_delivery_repo_test.go`
- Modify: `internal/app/notification.go`
- Modify: `internal/app/notification_scheduler.go`

- [ ] **Step 1: 写失败测试**

新增测试：可以插入 `task_uuid=""` 但 `object_kind="project"` / `object_id=<projectID>` 的 notification delivery。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/storage ./internal/app -run 'TestNotificationDelivery.*Event|TestNotificationDelivery|TestReminderScheduler' -count=1
```

预期：FAIL，`TaskUUID` not null 或 view 假设 task 必填。

- [ ] **Step 3: 最小实现**

二选一，推荐 B：

- A：把 `TaskUUID` 改为 `*string`。
- B：保留 `TaskUUID` 字段但允许空字符串，并新增：
  - `ObjectKind string`
  - `ObjectID string`

推荐 B，迁移风险更小。Reminder delivery 写 `ObjectKind="task"`、`ObjectID=tsk.UUID`。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/storage ./internal/app -run 'TestNotificationDelivery|TestReminderScheduler' -count=1
```

预期：PASS。

### Task 3: app 层创建和匹配 event notification rule

**Files:**
- Create: `internal/app/event_notification.go`
- Create: `internal/app/event_notification_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/hook_event.go`

- [ ] **Step 1: 写失败测试**

覆盖：

- `AddEventNotificationRule` 接受 `task.unblocked --sink openclaw --audience assignees`。
- `project.annotated` + `assignees` 创建失败，错误 `audience_unsupported_for_event`。
- 跨 workspace sink UUID 创建失败。
- `enqueueEventNotificationDeliveries` 对同一 event/rule/recipient 只生成一条 delivery。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/app -run 'TestEventNotificationRule|TestEventNotificationDelivery' -count=1
```

预期：FAIL，app 方法不存在。

- [ ] **Step 3: 最小实现**

新增 app 输入/视图：

```go
type EventNotificationRuleAddInput struct {
	Name string
	ProjectRef string
	EventType string
	FilterSource string
	AudienceType string
	Recipients []string
	SinkRef string
	TemplateSubject string
	TemplateBody string
}
```

实现：

- `AddEventNotificationRule`
- `ListEventNotificationRules`
- `EventNotificationRuleInfo`
- `ModifyEventNotificationRule`
- `EnableEventNotificationRule`
- `DisableEventNotificationRule`
- `DeleteEventNotificationRule`

匹配逻辑：

- 事件类型必须在 allowed event set。
- rule workspace 必须等于 event workspace。
- project rule 只匹配同 project event。
- filter 首版只对 task event 启用；project event 若提供 filter 则拒绝或忽略。推荐拒绝。
- audience：
  - `assignees` 仅 task primary object。
  - `explicit_users` 任意事件。
  - `assignees_and_explicit_users` 仅 task primary object。
  - `actor` 任意事件，recipient 为 event actor，对外 payload 中 actor 必须是 `task.UserInfo`。

recipient 解析：

- `assignees` 从 primary task 的 `Assignees` 取用户。
- `explicit_users` 使用规则中的 `RecipientUserIDsJSON`。
- `actor` 使用事件 envelope 的 `ActorUserID`，并通过 `resolveUserInfos` 转成完整 `task.UserInfo`。
- 所有 recipient 必须去重，并且必须是当前 workspace member。

delivery dedupe key：

```text
workspace_id:rule_id:event_id:recipient_user_id
```

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/app -run 'TestEventNotificationRule|TestEventNotificationDelivery|TestHookDelivery' -count=1
```

预期：PASS。

### Task 4: 事件入队同时触发 notification rule

**Files:**
- Modify: `internal/app/audit.go`
- Modify: `internal/app/hook_event.go`
- Modify: `internal/app/event_notification.go`
- Modify: `internal/app/event_notification_test.go`

- [ ] **Step 1: 写失败测试**

在实际业务操作中验证：

- `Done` 生成 `task.unblocked` 后，会因 rule 生成 notification delivery。
- `ProjectAnnotate` 生成 `project.annotated` 后，`actor` audience rule 会生成 notification delivery。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/app -run 'Test.*EventNotification.*Business|TestDone.*Unblocked|TestProjectAnnotate' -count=1
```

预期：FAIL，事件还未触发 notification rule。

- [ ] **Step 3: 最小实现**

修改 `withAuditEntriesAndEvents` 末尾：

```go
if err := txSvc.enqueueHookEvents(events); err != nil { return err }
return txSvc.enqueueEventNotificationDeliveries(events)
```

注意：

- 如果后续决定“delivery enqueue 失败不回滚业务事实”，应另起事务/outbox 规格；本轮先延续现有 hook enqueue 失败会返回错误的行为。
- 两个 enqueue 都只写数据库，不调用外部网络。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/app -run 'Test.*EventNotification|TestHookDelivery|TestProjectAnnotate|TestDone' -count=1
```

预期：PASS。

### Task 5: 提交 Chunk 4

```bash
git add internal/storage internal/app
git commit -m "feat: 添加事件通知规则"
```

---

## Chunk 5: CLI / HTTP / MCP 暴露事件通知规则

### Task 1: CLI `notification rule`

**Files:**
- Modify: `internal/cli/notification.go`
- Modify: `internal/cli/notification_test.go`
- Modify: `internal/remote/notification_rule.go`
- Modify: `internal/remote/notification.go`

- [ ] **Step 1: 写失败测试**

CLI：

```bash
xuanchu notification rule add task-unblocked-openclaw \
  --event task.unblocked \
  --audience assignees \
  --sink openclaw
```

成功。

```bash
xuanchu notification rule add bad --event task.unblocked --url https://example.test
```

失败。

list/info/modify/enable/disable/remove 均可用。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/cli ./internal/remote -run 'TestCLINotificationRule|TestRemoteNotificationRule' -count=1
```

预期：FAIL，命令不存在。

- [ ] **Step 3: 最小实现**

- 在 `notification` 下新增 `rule` 子命令，不要放进 `reminder rule`。
- 入参：`--event`、`--audience`、`--sink`、`--project`、`--recipient`、`--filter`、`--template-subject`、`--template-body`。
- JSON 输出稳定。
- remote mode 调用 HTTP `/api/v1/notification-rules`。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/cli ./internal/remote -run 'TestCLINotificationRule|TestRemoteNotificationRule|TestCLINotificationSink|TestCLIReminderRule' -count=1
```

预期：PASS。

### Task 2: HTTP API

**Files:**
- Create: `internal/httpapi/notification_rules.go`
- Create: `internal/httpapi/notification_rules_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `docs/openapi/xuanchu-v1.yaml`

- [ ] **Step 1: 写失败测试**

覆盖：

- POST `/api/v1/notification-rules` 创建 event rule。
- body 包含 `url` 时 400 `notification_rule_url_not_supported`。
- 跨 workspace sink UUID 不能绑定。
- GET/list/info/patch/enable/disable/delete。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/httpapi -run 'TestHTTPNotificationRule' -count=1
```

预期：FAIL，路由不存在。

- [ ] **Step 3: 最小实现**

路由：

- `GET /api/v1/notification-rules`
- `POST /api/v1/notification-rules`
- `GET /api/v1/notification-rules/{ruleID}`
- `PATCH /api/v1/notification-rules/{ruleID}`
- `POST /api/v1/notification-rules/{ruleID}/enable`
- `POST /api/v1/notification-rules/{ruleID}/disable`
- `DELETE /api/v1/notification-rules/{ruleID}`

字段使用 `sink` / `sink_id`。不要接受 `url`。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/httpapi -run 'TestHTTPNotificationRule|TestHTTPNotificationSink|TestHTTPReminderRule' -count=1
```

预期：PASS。

### Task 3: MCP tools

**Files:**
- Modify: `internal/mcpserver/tools_notification.go`
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `internal/mcpserver/testdata/notification_rule_add.schema.json`
- Modify: `internal/mcpserver/testdata/notification_rule_list.schema.json`
- Modify: `internal/mcpserver/testdata/notification_rule_info.schema.json`
- Modify: `internal/mcpserver/testdata/notification_rule_modify.schema.json`
- Modify: `internal/mcpserver/testdata/notification_rule_enable.schema.json`
- Modify: `internal/mcpserver/testdata/notification_rule_disable.schema.json`
- Modify: `internal/mcpserver/testdata/notification_rule_remove.schema.json`
- Modify: `internal/mcpserver/testdata/list-tools-default.json`

- [ ] **Step 1: 写失败测试**

MCP tool list 必须包含：

- `notification_rule_list`
- `notification_rule_add`
- `notification_rule_info`
- `notification_rule_modify`
- `notification_rule_enable`
- `notification_rule_disable`
- `notification_rule_remove`

不得包含点号命名。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCP.*NotificationRule|TestListTools' -count=1
```

预期：FAIL，tools 不存在。

- [ ] **Step 3: 最小实现**

复用 app 方法，MCP input 字段：

```go
Event string `json:"event"`
Audience string `json:"audience"`
Sink string `json:"sink"`
Project string `json:"project,omitempty"`
Recipients []string `json:"recipients,omitempty"`
Filter string `json:"filter,omitempty"`
TemplateBody string `json:"template_body,omitempty"`
```

`sink` 按 request scope workspace 解析。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCP.*NotificationRule|TestListTools|TestMCPNotification|TestMCPHook' -count=1
```

预期：PASS。

### Task 4: 提交 Chunk 5

```bash
git add internal/cli internal/remote internal/httpapi internal/mcpserver docs/openapi/xuanchu-v1.yaml
git commit -m "feat: 暴露事件通知规则接口"
```

---

## Chunk 6: 文档、端到端验证与收尾

### Task 1: 用户文档同步

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/hooks.md`
- Modify: `docs/manual/notifications.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/manual/reference/commands.md`
- Modify: `docs/skills/*` 中涉及 Hook / notification / MCP 的 skill

- [ ] **Step 1: 更新文档**

必须写清：

- Hook 使用 `--sink`，不使用 `--url`。
- sink 是 workspace 级资源。
- `project.annotated`、`project.denotated`、`task.unblocked` 是支持事件。
- OpenClaw 通过 sink 接收通知。
- event notification rule 示例。
- MCP tool 使用下划线命名。
- `ROADMAP.md` 必须保留未实现事件的下一大版本规划，至少列出：
  - Priority 1：`task.assigned`、`task.unassigned`、`task.started`、`task.stopped`、`task.blocked`、`task.due_changed`、`task.priority_changed`、`task.project_changed`、`task.tags_changed`。
  - Priority 2：`task.annotated`、`task.denotated`、`task.link_added`、`task.link_removed`、`project.created`、`project.updated`、`workspace.member_added`、`workspace.member_removed`、`workspace.member_role_changed`。
  - `task.due_soon` / `task.overdue` 属于 scheduler / reminder rule 生成的时间条件事件，不能和 `task.due_changed` 混为一类。

- [ ] **Step 2: 搜索旧表达**

Run:

```bash
rg -n 'hook add .*--url|project\\.annotate|notification\\.rule|task\\.unblocked|project\\.annotated|project\\.denotated|task\\.assigned|task\\.due_changed|workspace\\.member_added' README.md ROADMAP.md docs
```

预期：

- 不再出现推荐 `hook add --url`。
- 不出现点号 MCP tool。
- 新事件在 hooks/notifications/mcp 文档中可查。
- 未实现的 Priority 1 / Priority 2 事件在 `ROADMAP.md` 中可查，且标注为下一大版本规划，不误写成当前已实现能力。

### Task 2: 端到端与全量验证

**Files:**
- No direct edits unless tests expose missing coverage.

- [ ] **Step 1: 跑目标包测试**

```bash
go test ./internal/storage ./internal/app ./internal/cli ./internal/httpapi ./internal/mcpserver ./internal/hookruntime ./internal/notificationruntime -count=1
```

预期：PASS。

- [ ] **Step 2: 跑全量测试**

```bash
go test ./...
```

预期：PASS。

- [ ] **Step 3: 跑 CGO=0 测试**

```bash
CGO_ENABLED=0 go test ./...
```

预期：PASS。

- [ ] **Step 4: 跑 CGO=0 build**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

预期：PASS。

- [ ] **Step 5: diff check**

```bash
git diff --check
```

预期：无输出。

### Task 3: 提交 Chunk 6

```bash
git add README.md ROADMAP.md docs internal
git commit -m "docs: 更新事件通知与 Hook 文档"
```

---

## 实施注意事项

- 如果 `EventNotificationRule` 与现有 `ReminderRule` 出现大量重复，不要先抽象合并；首版保持两个清晰模型，后续再考虑统一 notification rule family。
- `notification_sinks` 表名可以暂不改，避免无收益迁移；对外文档用 outbound sink 解释。
- Hook delivery 继续独立于 notification delivery，避免把机器事件投递和用户通知混在一张 delivery 表中。
- event notification rule 使用 notification delivery，是因为它最终投递的是用户通知消息。
- `project.denotate` 是动作名；事件名统一为 `project.denotated`。
- `task.assigned`、`task.blocked`、`task.due_changed`、`task.annotated`、`workspace.member_added` 等候选事件只在规格中列为下个大版本方向，本轮不要实现。
- 若实现过程中发现 enqueue notification delivery 失败会回滚业务写入，不在本轮扩大事务语义；记录为后续 outbox 可靠性改进。

## 计划评审记录

### Review 1: spec 一致性审阅

结论：发现 3 个问题，已修正。

- 问题：Hook 存储层测试示例仍给 helper 填 `EndpointURL` / `Secret`，容易误导实现者保留旧模型。修正：测试示例只填写 `SinkID`，并明确 helper 不应再填 URL/secret。
- 问题：`task.unblocked` 计划只写“前后计算依赖状态”，没有说明如何保留变更前状态。修正：明确先读取更新前任务快照计算 `beforeBlocked`，执行 `doneLocked` 后再读取快照计算 `afterBlocked`。
- 问题：project annotation 和 actor audience 可能退化为裸 UUID。修正：明确 app 层必须解析为 `task.UserInfo`，recipient 必须是当前 workspace member。

复核结果：

- 没有保留 Hook `--url` 兼容路径；`--url` 只出现在失败测试和文档搜索命令中。
- sink-ref workspace 级隔离已覆盖 app、CLI、HTTP、MCP、测试。
- project annotation 事件 payload 要求包含 project 和 annotation。
- `task.unblocked` 只在最后一个 blocker 完成后生成。
- event notification rule 独立于 reminder rule。

### Review 2: 可执行性审阅

结论：发现 2 个问题，已修正。

- 问题：计划标题和模板字段仍是英文，不符合本仓库中文文档要求。修正：标题、目标、架构、技术栈、预期输出、评审记录改为中文。
- 问题：Hook sink 模板没有说明 secret ref 和 HTTP method。修正：明确 Hook 使用的 sink 可引用 `secret.*`，secret ref 由 sink 声明；`http_template` 的 `HTTPMethod` 应被尊重。

复核结果：

- 每个 chunk 都有失败测试、实现、通过测试和提交点。
- 已列出旧 URL 模型影响到的 storage、app、CLI、HTTP、remote、MCP、testdata、dispatcher、docs。
- 已包含目标包测试、全量测试、`CGO_ENABLED=0` 测试和 build、`git diff --check`。
