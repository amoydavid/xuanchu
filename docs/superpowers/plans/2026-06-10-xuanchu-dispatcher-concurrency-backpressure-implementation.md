# Xuanchu Dispatcher 并发、背压与可靠投递实现计划

> **给 agentic workers 的要求：** 执行本计划时必须使用 `superpowers:subagent-driven-development`（如果当前环境支持子代理）或 `superpowers:executing-plans`。步骤使用 checkbox（`- [ ]`）语法跟踪进度。

**目标：** 为 notification dispatcher 和 hook dispatcher 增加可配置的进程级并发、workspace 内 sink 级并发、受容量约束的 claim 策略，以及不会因进程重启丢失 delivery 的可靠投递运行时。

**架构：** 数据库中的 `notification_deliveries` / `hook_deliveries` 继续作为唯一可靠队列；dispatcher 只在有执行容量时 claim delivery，并用有界 worker pool 执行 HTTP 请求。进程级配置来自 server 运行时配置 / CLI flag，dispatcher 默认 `max_concurrency=1` 保持当前顺序投递行为；sink 级并发保存在 `notification_sinks` 表中，`0` 表示继承 dispatcher，不是 sink 自己默认并发 1。

**技术栈：** Go 1.25、Cobra、GORM、SQLite/PostgreSQL、`net/http`、现有 `notificationruntime` / `hookruntime` / `storage` / `app` / `config` 分层。

---

## 前置约束

- 规格文档：`docs/superpowers/specs/2026-06-10-xuanchu-dispatcher-concurrency-backpressure-design.md`
- 文档和用户可见描述必须使用中文。
- 不引入 Redis、Kafka、NATS 或其它外部队列。
- 不改变 delivery 冻结请求快照规则。
- 不承诺 exactly-once；保持 at-least-once 语义。
- SQLite 继续使用 `github.com/glebarez/sqlite`，PostgreSQL 继续使用 `gorm.io/driver/postgres`。
- 每个 chunk 完成后建议提交一次，提交信息使用中文。

## 文件结构

新增文件：

- `internal/runtimeutil/concurrency.go`：小型并发工具，提供可测试的 `EffectiveConcurrency` / `EffectivePrefetchFactor` / `ClaimLimit` 计算，避免两个 dispatcher 复制细节。
- `internal/runtimeutil/concurrency_test.go`：并发默认值和 claim 上限单元测试。
- `internal/runtimeutil/sink_limiter.go`：进程内共享 sink 并发 limiter。server 启动时创建一个实例，同时传给 notification dispatcher 和 hook dispatcher，保证同一进程内同一 sink 的并发上限不会被两个 runtime 各自放大。`max_concurrency=0` 的 sink 使用 dispatcher options 中已经归一化的 `DefaultSinkConcurrency`，不由 limiter 自行推断。
- `internal/runtimeutil/sink_limiter_test.go`：sink limiter 的 acquire/release、继承默认 sink 并发、并发竞争测试。

修改文件：

- `internal/storage/models.go`：为 `NotificationSink` 增加 `MaxConcurrency int`。
- `internal/storage/db_test.go`：验证 `notification_sinks.max_concurrency` 迁移存在，默认值为 0。
- `internal/app/notification.go`：`NotificationSinkAddInput` / `ModifyInput` / `View` 增加 `MaxConcurrency`，校验非负，创建/修改/展示都保留该字段。
- `internal/app/notification_test.go`：sink max concurrency 创建、修改、非法负数测试。
- `internal/httpapi/notifications.go` / `internal/httpapi/notifications_test.go`：HTTP sink request/response 增加 `max_concurrency`。
- `internal/remote/notification.go`：remote DTO 增加 `max_concurrency`。
- `internal/cli/notification.go` / `internal/cli/notification_test.go`：`notification sink add/modify/info/list --json` 支持 `--max-concurrency` 和 JSON 字段。
- `internal/mcpserver/tools_notification.go` / `internal/mcpserver/integration_test.go`：MCP `notification_sink_add/modify/list/info` 支持 `max_concurrency`。
- `docs/openapi/xuanchu-v1.yaml`：notification sink schema 增加 `max_concurrency`。
- `internal/config/config.go` / `internal/config/config_test.go`：解析 `notifications.dispatcher.*` 与 `hooks.dispatcher.*` runtime 配置。
- `internal/cli/server.go`：server flag 和 TOML 配置接入 dispatcher options。
- `config.example.toml`：增加 dispatcher 配置示例。
- `internal/app/notification_endpoint.go` / `internal/app/notification_endpoint_test.go`：notification payload 和 `http_template` 上下文提供稳定 delivery 信息。
- `internal/app/notification_scheduler.go` / `internal/app/notification_scheduler_test.go`：生成 notification delivery 前先生成 delivery ID，并传给 request resolver。
- `internal/app/hook_event.go` / `internal/app/hook_test.go`：Hook delivery envelope / payload 在冻结时包含 delivery 信息。
- `internal/notificationruntime/dispatcher.go` / `internal/notificationruntime/dispatcher_test.go`：notification dispatcher 支持并发、prefetch、共享 sink token、受容量约束 claim、stale recovery 验收。
- `internal/hookruntime/dispatcher.go` / `internal/hookruntime/dispatcher_test.go`：hook dispatcher 支持同样并发模型和 stale recovery 验收。
- `internal/cli/server.go` / `internal/cli/server_test.go`：server 运行时配置解析为 dispatcher options，并把同一个 sink limiter 传给两个 dispatcher。
- `README.md`、`docs/manual/notifications.md`、`docs/manual/hooks.md`、`docs/manual/mcp.md`、`docs/skills/*/SKILL.md`：同步用户可见说明。

---

## Chunk 1: Sink 级并发字段与配置入口

### Task 1: 为 notification sink 增加 `max_concurrency`

**文件：**
- 修改：`internal/storage/models.go`
- 修改：`internal/storage/db_test.go`
- 修改：`internal/app/notification.go`
- 修改：`internal/app/notification_test.go`

- [x] **步骤 1: 写失败测试**

在 `internal/app/notification_test.go` 增加测试：

```go
func TestNotificationSinkMaxConcurrencyCreateModifyView(t *testing.T) {
	svc := newTestService(t)
	created, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:           "limited",
		Type:           NotificationSinkTypeWebhook,
		EndpointMode:   NotificationEndpointStaticURL,
		URL:            "https://example.com/hook",
		MaxConcurrency: 2,
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	if created.MaxConcurrency != 2 {
		t.Fatalf("created.MaxConcurrency = %d, want 2", created.MaxConcurrency)
	}

	next := 3
	modified, err := svc.ModifyNotificationSink(created.ID, NotificationSinkModifyInput{MaxConcurrency: &next})
	if err != nil {
		t.Fatalf("ModifyNotificationSink() error = %v", err)
	}
	if modified.MaxConcurrency != 3 {
		t.Fatalf("modified.MaxConcurrency = %d, want 3", modified.MaxConcurrency)
	}
}

func TestNotificationSinkRejectsNegativeMaxConcurrency(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:           "bad",
		Type:           NotificationSinkTypeWebhook,
		EndpointMode:   NotificationEndpointStaticURL,
		URL:            "https://example.com/hook",
		MaxConcurrency: -1,
	})
	assertRuntimeCode(t, err, "notification_sink_invalid")
}
```

在 `internal/storage/db_test.go` 增加迁移测试，使用现有 helper 检查 `notification_sinks` 表有 `max_concurrency` 列，默认值允许为 `0`。

在 storage 层增加 claim SQL 兼容测试，先让实现者在早期任务看到 PostgreSQL 类型推断问题：

```go
func TestNotificationDeliveryClaimDueSQLAvoidsPostgresUntypedMax(t *testing.T) {
	// 断言 notification claim SQL 不包含 PostgreSQL 无法推断类型的 MAX(?...) / max(unknown, bigint) 形态。
	// 应使用 CASE 或显式 CAST(? AS BIGINT)。
}

func TestHookDeliveryClaimDueSQLAvoidsPostgresUntypedMax(t *testing.T) {
	// 断言 hook claim SQL 使用同样的 PostgreSQL 安全写法。
}
```

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/app -run 'TestNotificationSink.*MaxConcurrency' -count=1
go test ./internal/storage -run 'Test.*NotificationSink.*MaxConcurrency|TestAutoMigrate|NotificationDeliveryClaimDueSQLAvoidsPostgresUntypedMax|HookDeliveryClaimDueSQLAvoidsPostgresUntypedMax' -count=1
```

预期：失败，字段尚不存在。

- [x] **步骤 3: 最小实现**

在 `storage.NotificationSink` 增加：

```go
MaxConcurrency int `gorm:"not null;default:0"`
```

在 app 输入/输出结构增加：

```go
MaxConcurrency int
```

和：

```go
MaxConcurrency *int
```

校验规则：

- `MaxConcurrency < 0` 返回 `notification_sink_invalid`。
- `0` 表示不单独限制，继承 dispatcher 级并发。
- `>0` 表示该 sink 在当前进程内最多并发投递数。

`AddNotificationSink` 创建 row 时写入 `MaxConcurrency`。

`ModifyNotificationSink` 仅在传入指针非 nil 时更新。

`notificationSinkViewFromRow` 输出 `MaxConcurrency`。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/app -run 'TestNotificationSink.*MaxConcurrency|TestNotificationSink' -count=1
go test ./internal/storage -run 'Test.*NotificationSink|TestAutoMigrate' -count=1
```

预期：通过。

### Task 2: HTTP / Remote / CLI / MCP 暴露 `max_concurrency`

**文件：**
- 修改：`internal/httpapi/notifications.go`
- 修改：`internal/httpapi/notifications_test.go`
- 修改：`internal/remote/notification.go`
- 修改：`internal/cli/notification.go`
- 修改：`internal/cli/notification_test.go`
- 修改：`internal/mcpserver/tools_notification.go`
- 修改：`internal/mcpserver/integration_test.go`

- [x] **步骤 1: 写失败测试**

HTTP 测试：在 `internal/httpapi/notifications_test.go` 的 notification sink create/modify 测试中加入：

```json
{"max_concurrency":2}
```

并断言响应：

```go
if got := int(resp.Data["max_concurrency"].(float64)); got != 2 {
	t.Fatalf("max_concurrency = %d, want 2", got)
}
```

CLI 测试：在 `internal/cli/notification_test.go` 添加：

```go
func TestNotificationSinkAddJSONIncludesMaxConcurrency(t *testing.T) {
	// 使用现有 CLI test helper 执行：
	// notification sink add limited --url https://example.com/hook --max-concurrency 2 --json
	// 断言 JSON 中 max_concurrency == 2。
}
```

MCP 集成测试：在 `notification_sink_add` 输入中传 `max_concurrency: 2`，断言 tool 输出包含 `max_concurrency`。

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/httpapi -run 'Test.*NotificationSink' -count=1
go test ./internal/cli -run 'TestNotificationSink.*MaxConcurrency|TestNotificationSink' -count=1
go test ./internal/mcpserver -run 'Test.*Notification.*Sink' -count=1
```

预期：失败，JSON/CLI/MCP 尚未暴露字段。

- [x] **步骤 3: 最小实现**

HTTP:

```go
MaxConcurrency int  `json:"max_concurrency,omitempty"`
MaxConcurrency *int `json:"max_concurrency,omitempty"`
```

在 create/modify request 到 app input 时传递字段；`notificationSinkResponse` 增加：

```go
"max_concurrency": row.MaxConcurrency,
```

Remote DTO 增加同名字段。

CLI:

- `notificationSinkCLIInput` 增加 `maxConcurrency int`。
- `bindNotificationSinkFlags` 增加：

```go
cmd.Flags().IntVar(&input.maxConcurrency, "max-concurrency", 0, "该 sink 在当前进程内的最大并发投递数（0 表示继承 dispatcher）")
```

- add/modify 传递字段；modify 仅在 `cmd.Flags().Changed("max-concurrency")` 时设置指针。
- `notificationSinkViewForJSON` 和 human info/list 输出增加字段。human list 如果已有紧凑表格，可以只在 info 输出显示；JSON 必须稳定包含。

MCP:

- `NotificationSinkAddInput` / modify input 增加 `MaxConcurrency`。
- tool 输出 map 增加 `max_concurrency`。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/httpapi -run 'Test.*NotificationSink' -count=1
go test ./internal/cli -run 'TestNotificationSink' -count=1
go test ./internal/mcpserver -run 'Test.*Notification.*Sink' -count=1
```

预期：通过。

### Task 3: 解析 dispatcher runtime 配置

**文件：**
- 修改：`internal/config/config.go`
- 修改：`internal/config/config_test.go`
- 修改：`internal/cli/server.go`
- 修改：`config.example.toml`

- [x] **步骤 1: 写失败测试**

在 `internal/config/config_test.go` 增加：

```go
func TestResolveDispatcherConfigFromToml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[notifications.dispatcher]
max_concurrency = 4
batch_size = 25
prefetch_factor = 1
poll_interval = "7s"
claim_ttl = "6m"

[hooks.dispatcher]
max_concurrency = 3
batch_size = 20
prefetch_factor = 1
poll_interval = "8s"
claim_ttl = "7m"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{ConfigPath: path, HomeDir: dir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.NotificationDispatcher.MaxConcurrency != 4 {
		t.Fatalf("notification max concurrency = %d, want 4", cfg.NotificationDispatcher.MaxConcurrency)
	}
	if cfg.HookDispatcher.PollInterval != 8*time.Second {
		t.Fatalf("hook poll interval = %v, want 8s", cfg.HookDispatcher.PollInterval)
	}
}
```

再加一个非法值测试：负数、非法 duration 返回错误。

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/config -run 'TestResolveDispatcherConfig' -count=1
```

预期：失败，配置结构不存在。

- [x] **步骤 3: 最小实现**

在 `internal/config/config.go` 增加：

```go
type DispatcherConfig struct {
	MaxConcurrency int
	BatchSize      int
	PrefetchFactor int
	PollInterval   time.Duration
	ClaimTTL       time.Duration
}
```

在 `Config` 增加：

```go
NotificationDispatcher DispatcherConfig
HookDispatcher         DispatcherConfig
```

新增解析函数：

```go
func parseDispatcherConfig(values map[string]string, prefix string) (DispatcherConfig, error)
```

规则：

- `max_concurrency` 默认 `1`，必须 `>= 1`。
- `batch_size` 默认 `50`，必须 `>= 1`。
- `prefetch_factor` 默认 `1`，必须 `>= 1`。
- `poll_interval` 默认 `5s`，用 `time.ParseDuration`。
- `claim_ttl` 默认 `5m`，用 `time.ParseDuration`。

`Resolve` 中解析：

```go
notificationDispatcher, err := parseDispatcherConfig(tomlValues, "notifications.dispatcher")
hookDispatcher, err := parseDispatcherConfig(tomlValues, "hooks.dispatcher")
```

注意：`tomlValues == nil` 时也要返回默认值。

`internal/cli/server.go` 增加 flag：

- `--notification-dispatcher-max-concurrency`
- `--notification-dispatcher-batch-size`
- `--notification-dispatcher-prefetch-factor`
- `--notification-dispatcher-claim-ttl`
- `--hook-dispatcher-interval`
- `--hook-dispatcher-max-concurrency`
- `--hook-dispatcher-batch-size`
- `--hook-dispatcher-prefetch-factor`
- `--hook-dispatcher-claim-ttl`

flag 默认值从 `cfg.NotificationDispatcher` / `cfg.HookDispatcher` 读取会比较麻烦，因为 Cobra flag 在 `Resolve` 前绑定。首版采用“零值 flag + 显式 Changed 覆盖”的方式：

- 保留现有 `--notification-dispatcher-interval`，它覆盖 notification dispatcher；为了不改变现有启动参数含义，如果用户没有显式传 `--hook-dispatcher-interval`，旧 flag 也作为 hook interval 的兼容别名。
- 新增 flag 默认零值，只有 `cmd.Flags().Changed(...)` 时覆盖 `cfg.*`。
- `--hook-dispatcher-interval` 显式传入时优先级最高，只覆盖 hook dispatcher。
- TOML 中的 `notifications.dispatcher.poll_interval` 和 `hooks.dispatcher.poll_interval` 分别作为两个 dispatcher 的默认值。

把 `config.example.toml` 增加：

```toml
[notifications.dispatcher]
max_concurrency = 1
batch_size = 50
prefetch_factor = 1
poll_interval = "5s"
claim_ttl = "5m"

[hooks.dispatcher]
max_concurrency = 1
batch_size = 50
prefetch_factor = 1
poll_interval = "5s"
claim_ttl = "5m"
```

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/config -run 'TestResolveDispatcherConfig|TestResolveConfigPathLoadsToml' -count=1
go test ./internal/cli -run 'TestServer|TestRoot' -count=1
```

预期：通过。

---

## Chunk 2: 并发工具、幂等 payload 与 Notification dispatcher

### Task 4: 添加并发计算工具和共享 sink limiter

**文件：**
- 新增：`internal/runtimeutil/concurrency.go`
- 新增：`internal/runtimeutil/concurrency_test.go`
- 新增：`internal/runtimeutil/sink_limiter.go`
- 新增：`internal/runtimeutil/sink_limiter_test.go`

- [x] **步骤 1: 写失败测试**

创建 `internal/runtimeutil/concurrency_test.go`：

```go
func TestClaimLimit(t *testing.T) {
	tests := []struct {
		name string
		batch int
		available int
		prefetch int
		want int
	}{
		{"no available", 50, 0, 1, 0},
		{"less than batch", 50, 3, 1, 3},
		{"prefetch", 50, 3, 2, 6},
		{"capped by batch", 5, 3, 2, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClaimLimit(tt.batch, tt.available, tt.prefetch); got != tt.want {
				t.Fatalf("ClaimLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}
```

创建 `internal/runtimeutil/sink_limiter_test.go`：

```go
func TestSinkLimiterAcquireRelease(t *testing.T) {
	limiter := NewSinkLimiter()
	if !limiter.TryAcquire("sink-a", 1) {
		t.Fatal("first acquire failed")
	}
	if limiter.TryAcquire("sink-a", 1) {
		t.Fatal("second acquire succeeded, want limited")
	}
	limiter.Release("sink-a")
	if !limiter.TryAcquire("sink-a", 1) {
		t.Fatal("acquire after release failed")
	}
}

func TestEffectiveSinkConcurrencyUsesDefaultWhenZero(t *testing.T) {
	if got := EffectiveSinkConcurrency(0, 4); got != 4 {
		t.Fatalf("EffectiveSinkConcurrency(0,4) = %d, want 4", got)
	}
	if got := EffectiveSinkConcurrency(2, 4); got != 2 {
		t.Fatalf("EffectiveSinkConcurrency(2,4) = %d, want 2", got)
	}
}
```

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/runtimeutil -count=1
```

预期：失败，包或方法不存在。

- [x] **步骤 3: 最小实现**

实现：

```go
package runtimeutil

func EffectiveConcurrency(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func EffectivePrefetchFactor(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func ClaimLimit(batchSize, availableSlots, prefetchFactor int) int {
	if batchSize <= 0 || availableSlots <= 0 {
		return 0
	}
	prefetch := EffectivePrefetchFactor(prefetchFactor)
	limit := availableSlots * prefetch
	if limit > batchSize {
		return batchSize
	}
	return limit
}
```

实现共享 sink limiter：

```go
type SinkLimiter struct {
	mu       sync.Mutex
	inflight map[string]int
}

func NewSinkLimiter() *SinkLimiter {
	return &SinkLimiter{inflight: map[string]int{}}
}

func EffectiveSinkConcurrency(sinkLimit int, defaultSinkConcurrency int) int {
	if sinkLimit > 0 {
		return sinkLimit
	}
	return EffectiveConcurrency(defaultSinkConcurrency)
}

func (l *SinkLimiter) TryAcquire(sinkID string, limit int) bool {
	limit = EffectiveConcurrency(limit)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[sinkID] >= limit {
		return false
	}
	l.inflight[sinkID]++
	return true
}

func (l *SinkLimiter) Release(sinkID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[sinkID] <= 1 {
		delete(l.inflight, sinkID)
		return
	}
	l.inflight[sinkID]--
}
```

设计要求：

- `SinkLimiter` 是进程内共享对象，不属于某一个 dispatcher。
- `xuanchu server` 启动时创建一个 `SinkLimiter`，同时传给 notification dispatcher 和 hook dispatcher。
- 单元测试只验证 limiter 自身；跨 dispatcher 共享行为在 Task 12 的 server helper 测试中验证两者收到同一个 limiter 指针。
- `EffectiveSinkConcurrency(sinkLimit, defaultSinkConcurrency)` 的第二个参数语义是“这个 sink 没有显式限制时使用的默认 sink 上限”，不是某个 dispatcher 的即时 worker 空闲数。
- 非 server 的单 dispatcher 使用者可以让 `DefaultSinkConcurrency=MaxConcurrency`；server 模式必须由 Task 12 统一计算后传入 notification 和 hook 两个 dispatcher，避免同一 sink 因两个 runtime 的默认值不同而上限漂移。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/runtimeutil -count=1
```

预期：通过。

### Task 5: delivery payload 和模板上下文携带幂等字段

**文件：**
- 修改：`internal/app/notification_endpoint.go`
- 修改：`internal/app/notification_endpoint_test.go`
- 修改：`internal/app/notification_scheduler.go`
- 修改：`internal/app/notification_scheduler_test.go`
- 修改：`internal/app/event_notification.go`
- 修改：`internal/app/event_notification_test.go`
- 修改：`internal/app/hook_event.go`
- 修改：`internal/app/hook_test.go`

- [x] **步骤 1: 写失败测试**

在 `internal/app/notification_endpoint_test.go` 增加：

```go
func TestResolveNotificationRequestProvidesDeliveryTemplateContext(t *testing.T) {
	input := sampleNotificationRequestInput(t)
	input.Delivery = NotificationDeliveryContext{
		ID:          "delivery-1",
		Attempt:     1,
		WorkspaceID: input.Workspace.ID,
		SinkID:      input.Sink.ID,
	}
	input.Sink.Type = NotificationSinkTypeHTTPTemplate
	input.Sink.BodyTemplate = `{"delivery_id":"{{delivery.id}}","attempt":{{delivery.attempt}},"workspace_id":"{{delivery.workspace_id}}","sink_id":"{{delivery.sink_id}}","object_kind":"{{object.kind}}","object_id":"{{object.id}}"}`
	req, err := ResolveNotificationRequest(input)
	if err != nil {
		t.Fatalf("ResolveNotificationRequest() error = %v", err)
	}
	if !strings.Contains(req.RenderedBody, `"delivery_id":"delivery-1"`) {
		t.Fatalf("RenderedBody missing delivery context: %s", req.RenderedBody)
	}
}
```

在 reminder scheduler 测试中断言 `PayloadJSON` 包含：

```json
"delivery_id":"..."
"attempt":1
"workspace_id":"..."
"sink_id":"..."
"delivery":{"id":"...","attempt":1,"workspace_id":"...","sink_id":"..."}
```

在 event notification 测试中断言 payload 也包含 delivery / object / event 信息。

在 hook 测试中断言 hook delivery 的 `PayloadJSON` 或 `RenderedBody` 包含顶层 `delivery_id`、`sink_id`、`attempt`。如果 Hook 标准 envelope 当前已经包含部分字段，测试只补缺失字段。

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/app -run 'TestResolveNotificationRequestProvidesDeliveryTemplateContext|TestReminder.*Payload|TestEventNotification.*Payload|TestHook.*Payload' -count=1
```

预期：失败，delivery context 字段未提供。

- [x] **步骤 3: 最小实现**

在 `internal/app/notification_endpoint.go` 增加：

```go
type NotificationDeliveryContext struct {
	ID          string
	Attempt     int
	WorkspaceID string
	SinkID      string
}
```

在 `NotificationRequestResolveInput` 增加：

```go
Delivery NotificationDeliveryContext
```

模板变量必须支持：

```text
delivery.id
delivery.attempt
delivery.workspace_id
delivery.sink_id
event.id
event.type
object.kind
object.id
```

数字变量判断中加入：

```go
"delivery.attempt"
```

`buildNotificationPayloadJSON` 默认 envelope 增加：

```go
"delivery_id": input.Delivery.ID,
"attempt": input.Delivery.Attempt,
"workspace_id": input.Delivery.WorkspaceID,
"sink_id": input.Delivery.SinkID,
"delivery": map[string]any{
	"id": input.Delivery.ID,
	"attempt": input.Delivery.Attempt,
	"workspace_id": input.Delivery.WorkspaceID,
	"sink_id": input.Delivery.SinkID,
},
"object": map[string]any{
	"kind": objectKind,
	"id": objectID,
},
"event": map[string]any{
	"id": input.Event.ID,
	"type": eventType,
},
```

要求同时保留顶层字段和嵌套对象：

- 顶层 `delivery_id` / `attempt` / `workspace_id` / `sink_id` 满足外部消费者按规格直接读取。
- 嵌套 `delivery.*` 满足模板上下文和后续结构化扩展。

生成 delivery 时必须先生成 delivery ID，再传给 request resolver：

```go
deliveryID := uuid.NewString()
req, err := ResolveNotificationRequest(input.WithDelivery(deliveryID, 1, workspaceID, sinkID))
row := storage.NotificationDelivery{ID: deliveryID, ...}
```

说明：

- 冻结 body / payload 中的 `attempt` 在 enqueue 阶段为 `1`，因为 retry/replay 使用冻结请求快照，不重新渲染 body。
- 真实本次投递尝试次数由 Task 6 / Task 9 在 dispatcher 运行时 header 中表达：notification 和 hook 都使用 `X-Xuanchu-Attempt`，值来自 claim 后的 `delivery.AttemptCount`。
- 文档中必须说明：body 里的 `attempt` 是初始投递上下文；`X-Xuanchu-Attempt` 是本次 HTTP 请求的真实尝试次数。
- Hook delivery 同样在 enqueue 前生成 ID，并把 `delivery_id` 写入 envelope；Hook 签名 header 继续使用现有 `delivery.ID`。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/app -run 'NotificationRequest|Reminder.*Payload|EventNotification.*Payload|Hook.*Payload' -count=1
```

预期：通过。

### Task 6: notification dispatcher 支持进程级并发

**文件：**
- 修改：`internal/notificationruntime/dispatcher.go`
- 修改：`internal/notificationruntime/dispatcher_test.go`

- [x] **步骤 1: 写失败测试**

在 `internal/notificationruntime/dispatcher_test.go` 增加测试：

```go
func TestDispatcherClaimLimitUsesMaxConcurrency(t *testing.T) {
	store := newNotificationDispatcherStore(t)
	sink := createNotificationSink(t, store, "limited")
	for i := 0; i < 10; i++ {
		enqueueNotificationDelivery(t, store, sink.ID, fmt.Sprintf("d-%02d", i))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	updateDeliveriesResolvedURL(t, store, server.URL)

	dispatcher := NewDispatcher(DispatcherOptions{
		Store: store,
		Clock: testClock{now: 1000},
		Client: server.Client(),
		Resolver: app.DefaultHookResolver(),
		BatchSize: 50,
		MaxConcurrency: 3,
		PrefetchFactor: 1,
	})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	rows := listNotificationDeliveries(t, store)
	if succeeded := countStatus(rows, storage.DeliveryStatusSucceeded); succeeded != 3 {
		t.Fatalf("succeeded = %d, want 3", succeeded)
	}
	if queued := countStatus(rows, storage.DeliveryStatusQueued); queued != 7 {
		t.Fatalf("queued = %d, want 7", queued)
	}
}
```

再加并发峰值测试：

```go
func TestDispatcherMaxConcurrencyBoundsInflightRequests(t *testing.T) {
	// 创建 6 条 delivery，MaxConcurrency=2。
	// httptest handler 用 atomic 统计 inflight 和 maxInflight，并 sleep 50ms。
	// RunOnce 后断言 maxInflight <= 2。
}
```

增加或扩展 notification runtime header 测试，断言真实本次投递尝试次数通过运行时 header 输出：

```go
func TestDispatcherSendsAttemptHeaderFromClaimedDelivery(t *testing.T) {
	// 插入一条已有 attempt_count=1 的 delivery。
	// ClaimDue 后本次请求应看到 X-Xuanchu-Attempt: 2。
	// 注意 body/payload 中冻结的 attempt 可以仍是 1。
	if got := captured.Header.Get("X-Xuanchu-Attempt"); got != "2" {
		t.Fatalf("X-Xuanchu-Attempt = %q, want 2", got)
	}
}
```

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/notificationruntime -run 'TestDispatcher.*Concurrency|TestDispatcherClaimLimit|TestDispatcher.*AttemptHeader' -count=1
```

预期：失败，options 字段不存在或仍一次处理 10 条。

- [x] **步骤 3: 最小实现**

在 `notificationruntime.DispatcherOptions` 增加：

```go
MaxConcurrency int
PrefetchFactor int
DefaultSinkConcurrency int
```

`NewDispatcher` 默认：

```go
opts.MaxConcurrency = runtimeutil.EffectiveConcurrency(opts.MaxConcurrency)
opts.PrefetchFactor = runtimeutil.EffectivePrefetchFactor(opts.PrefetchFactor)
if opts.DefaultSinkConcurrency <= 0 {
	opts.DefaultSinkConcurrency = opts.MaxConcurrency
}
```

`RunOnce` 改为：

```go
available := d.opts.MaxConcurrency
claimLimit := runtimeutil.ClaimLimit(d.opts.BatchSize, available, d.opts.PrefetchFactor)
deliveries, err := d.deliveryRepo.ClaimDue(now, claimExpiresAt, claimLimit)
```

然后用有界 worker pool 并行执行：

```go
sem := make(chan struct{}, d.opts.MaxConcurrency)
errCh := make(chan error, len(deliveries))
var wg sync.WaitGroup
for _, delivery := range deliveries {
	if ctx.Err() != nil { return ctx.Err() }
	sem <- struct{}{}
	wg.Add(1)
	go func(delivery storage.NotificationDelivery) {
		defer wg.Done()
		defer func(){ <-sem }()
		errCh <- d.dispatchOne(ctx, delivery, d.opts.Clock.Unix())
	}(delivery)
}
wg.Wait()
close(errCh)
for err := range errCh {
	if err != nil {
		return err
	}
}
```

注意：

- 传给 `dispatchOne` 的 `now` 建议在 worker 内重新取，避免长请求后更新状态时间过旧。
- `RunOnce` 不要在 worker 未结束时返回。
- 默认 `MaxConcurrency=1` 时行为仍然顺序等价。
- 当 `prefetch_factor=1` 时，`RunOnce` 每轮最多 claim 并处理 `max_concurrency` 条 delivery，这是刻意的背压行为；持续运行时由下一次 poll 继续处理队列。
- 实际 HTTP 请求必须设置 `X-Xuanchu-Attempt`，值来自本次 claim 后的 `delivery.AttemptCount`；不要从冻结 body/payload 中的 `attempt` 反推。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/notificationruntime -run 'TestDispatcher.*Concurrency|TestDispatcherClaimLimit|TestDispatcher.*AttemptHeader|TestDispatcher' -count=1
```

预期：通过。

### Task 7: notification dispatcher 支持共享 sink 级并发

**文件：**
- 修改：`internal/notificationruntime/dispatcher.go`
- 修改：`internal/notificationruntime/dispatcher_test.go`

- [x] **步骤 1: 写失败测试**

新增测试：

```go
func TestDispatcherSinkMaxConcurrencyBoundsInflightPerSink(t *testing.T) {
	// sink A MaxConcurrency=1，sink B MaxConcurrency=0。
	// 全局 MaxConcurrency=4。
	// 各插入 4 条 delivery。
	// handler 根据 sink 或 URL path 统计每个 sink 的 max inflight。
	// 断言 sink A maxInflight <= 1，整体 maxInflight <= 4，sink B 可以并行。
}
```

新增测试：

```go
func TestDispatcherDoesNotLeaveSinkLimitedDeliveriesDelivering(t *testing.T) {
	// sink MaxConcurrency=1，全局 MaxConcurrency=4，插入 4 条。
	// RunOnce 后只允许 1 条 succeeded，其余应保持 queued 或 retry_wait，
	// 不能是 delivering。
}
```

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/notificationruntime -run 'TestDispatcherSink.*Concurrency|TestDispatcherDoesNotLeaveSinkLimited' -count=1
```

预期：失败，sink max concurrency 未实现。

- [x] **步骤 3: 最小实现**

推荐首版实现简单可靠方案：

1. `ClaimDue` 仍按全局 claim 上限 claim 少量 delivery。
2. `DispatcherOptions` 增加共享 limiter：

```go
SinkLimiter *runtimeutil.SinkLimiter
```

3. `NewDispatcher` 如果没有传入 limiter，则创建独立 limiter，保证测试和非 server 使用者仍可工作；server 模式必须传入共享 limiter。
4. `NewDispatcher` 如果 `DefaultSinkConcurrency <= 0`，设置为 `MaxConcurrency`。这只适用于非 server 或单 dispatcher 使用者；server 模式由 Task 12 显式传入统一值。
5. 处理每条 delivery 前读取 sink，计算有效 sink limit：

```go
limit := runtimeutil.EffectiveSinkConcurrency(sink.MaxConcurrency, d.opts.DefaultSinkConcurrency)
```

6. 如果 sink token 满了，不启动 HTTP 请求，立即 requeue：

```go
_ = d.deliveryRepo.Requeue(delivery.ID, now)
```

7. 如果拿到 token，则启动 worker，结束后必须 release：

```go
if !d.opts.SinkLimiter.TryAcquire(delivery.SinkID, limit) {
	return d.deliveryRepo.Requeue(delivery.ID, now)
}
defer d.opts.SinkLimiter.Release(delivery.SinkID)
```

这不是最终最高吞吐模型，但符合首版可靠性要求：不会把 sink 受限的 delivery 长期留在 `delivering`，也不会引入可靠性依赖的内存队列。

注意：

- 对 requeue 的 delivery 不应增加额外错误。
- `NotificationDeliveryRepository.Requeue` 现已存在，dispatcher 直接复用；如果实现过程中调整签名，必须同步更新 notification runtime 测试。
- 因为 `ClaimDue` 已经增加 `attempt_count`，requeue 会导致 attempt 被预增。实现计划允许首版接受该现象，但应在代码注释中说明；如果要避免，应新增 storage 方法支持“peek sink capacity 后再 claim”，复杂度放后续。
- 测试应断言没有 delivering，而不是 attempt_count 不变。
- sink `max_concurrency>0` 优先；sink `max_concurrency=0` 使用 `DefaultSinkConcurrency`；server 模式下 `DefaultSinkConcurrency` 由 Task 12 统一计算，避免同一 sink 在 notification/hook 两个 runtime 中因继承不同 dispatcher 并发而出现不稳定上限。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/notificationruntime -run 'TestDispatcher.*Concurrency|TestDispatcherSink|TestDispatcherDoesNotLeaveSinkLimited' -count=1
```

预期：通过。

### Task 8: notification dispatcher 补齐重启恢复验收

**文件：**
- 修改：`internal/notificationruntime/dispatcher_test.go`

- [x] **步骤 1: 写失败测试**

新增：

```go
func TestDispatcherRecoversStaleDeliveringOnRunOnce(t *testing.T) {
	// 插入 status=delivering、claim_expires_at=999 的 delivery，clock.now=1000。
	// RunOnce 应先 RecoverStaleDelivering，再重新 claim 并投递成功。
	// 断言最终 status=succeeded，attempt_count 在原值基础上增加。
}

func TestDispatcherKeepsFutureDeliveringInvisible(t *testing.T) {
	// 插入 status=delivering、claim_expires_at=1200 的 delivery，clock.now=1000。
	// RunOnce 不应投递它，状态仍为 delivering。
}

func TestDispatcherRetryWaitSurvivesUntilNextAttempt(t *testing.T) {
	// 插入 status=retry_wait、next_attempt_at=1200，clock.now=1000。
	// RunOnce 不应投递它，状态仍为 retry_wait。
}
```

- [x] **步骤 2: 跑测试确认失败或确认现有行为**

运行：

```bash
go test ./internal/notificationruntime -run 'TestDispatcher.*Stale|TestDispatcher.*RetryWait|TestDispatcher.*FutureDelivering' -count=1
```

预期：如果现有代码已经满足，测试通过；如果没有覆盖或行为不完整，失败后按下一步修复。

- [x] **步骤 3: 最小实现**

保持或补齐：

- `RunOnce` 最开始调用 `RecoverStaleDelivering(now)`。
- claim 查询只领取 `queued` 和到期 `retry_wait`。
- 未过期 `delivering` 不被 claim。
- `claim_expires_at` 使用现有 “base TTL 与 sink timeout + 60 秒取较大” 逻辑，不退化成固定短 TTL。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/notificationruntime -run 'TestDispatcher.*Stale|TestDispatcher.*RetryWait|TestDispatcher.*FutureDelivering|TestDispatcher' -count=1
```

预期：通过。

---

## Chunk 3: Hook dispatcher 并发与背压

### Task 9: hook dispatcher 支持进程级并发

**文件：**
- 修改：`internal/hookruntime/dispatcher.go`
- 新增或修改：`internal/hookruntime/dispatcher_test.go`

- [x] **步骤 1: 写失败测试**

测试内容：

```go
func TestHookDispatcherClaimLimitUsesMaxConcurrency(t *testing.T) {
	// 创建 10 条 hook_deliveries，BatchSize=50，MaxConcurrency=3。
	// RunOnce 后 succeeded=3，queued=7。
}

func TestHookDispatcherMaxConcurrencyBoundsInflightRequests(t *testing.T) {
	// 创建 6 条 hook_deliveries，MaxConcurrency=2。
	// httptest handler 统计 maxInflight，断言 <=2。
}

func TestHookDispatcherSendsAttemptHeaderFromClaimedDelivery(t *testing.T) {
	// 插入一条已有 attempt_count=1 的 hook delivery。
	// ClaimDue 后本次请求应看到 X-Xuanchu-Attempt: 2。
	// 注意 body/payload 中冻结的 attempt 可以仍是 1。
	if got := captured.Header.Get("X-Xuanchu-Attempt"); got != "2" {
		t.Fatalf("X-Xuanchu-Attempt = %q, want 2", got)
	}
}
```

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/hookruntime ./internal/httpapi -run 'TestHookDispatcher.*Concurrency|TestHookDispatcher.*AttemptHeader' -count=1
```

预期：失败，字段或行为不存在。

- [x] **步骤 3: 最小实现**

在 `hookruntime.DispatcherOptions` 增加：

```go
MaxConcurrency int
PrefetchFactor int
DefaultSinkConcurrency int
```

默认值和 `notificationruntime` 一致。

`RunOnce` 使用：

```go
claimLimit := runtimeutil.ClaimLimit(d.opts.BatchSize, d.opts.MaxConcurrency, d.opts.PrefetchFactor)
```

并用有界 worker pool 执行 `dispatchOne`。

保持：

- `RecoverStaleDelivering(now)` 每轮先执行。
- `ClaimDue` 仍然原子 claim。
- 默认 `MaxConcurrency=1` 行为与当前顺序投递一致。
- 当 `prefetch_factor=1` 时，`RunOnce` 每轮最多 claim 并处理 `max_concurrency` 条 delivery，这是刻意的背压行为；持续运行时由下一次 poll 继续处理队列。
- 实际 HTTP 请求必须设置 `X-Xuanchu-Attempt`，值来自本次 claim 后的 `delivery.AttemptCount`；不要从冻结 body/payload 中的 `attempt` 反推。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/hookruntime ./internal/httpapi -run 'TestHookDispatcher.*Concurrency|TestHookDispatcher.*AttemptHeader|TestHook.*Delivery|TestHook.*Dispatcher' -count=1
```

预期：通过。

### Task 10: hook dispatcher 支持共享 sink 级并发

**文件：**
- 修改：`internal/hookruntime/dispatcher.go`
- 修改：`internal/hookruntime/dispatcher_test.go`

- [x] **步骤 1: 写失败测试**

新增：

```go
func TestHookDispatcherSinkMaxConcurrencyBoundsInflightPerSink(t *testing.T) {
	// sink A MaxConcurrency=1，sink B MaxConcurrency=0。
	// 全局 MaxConcurrency=4。
	// 各插入 4 条 hook delivery。
	// 断言 sink A max inflight <=1，整体 <=4。
}
```

新增：

```go
func TestHookDispatcherDoesNotLeaveSinkLimitedDeliveriesDelivering(t *testing.T) {
	// sink MaxConcurrency=1，全局 MaxConcurrency=4。
	// RunOnce 后受限未执行的 delivery 不得保持 delivering。
}
```

- [x] **步骤 2: 跑测试确认失败**

运行：

```bash
go test ./internal/hookruntime ./internal/httpapi -run 'TestHookDispatcherSink|TestHookDispatcherDoesNotLeaveSinkLimited' -count=1
```

预期：失败。

- [x] **步骤 3: 最小实现**

hook dispatcher 读取 delivery 的 `SinkID`，加载 sink 后：

- sink missing / workspace mismatch：沿用当前 dead-letter。
- sink disabled：沿用 disabled-skipped。
- sink token 满：立即 `Requeue(delivery.ID, now)`，不执行 HTTP 请求。
- 使用 `DispatcherOptions.SinkLimiter *runtimeutil.SinkLimiter`。如果没有传入 limiter，则 `NewDispatcher` 创建独立 limiter；server 模式必须传入共享 limiter。
- sink limit 使用 `runtimeutil.EffectiveSinkConcurrency(sink.MaxConcurrency, d.opts.DefaultSinkConcurrency)`。
- sink `max_concurrency>0` 优先；sink `max_concurrency=0` 使用 `DefaultSinkConcurrency`。
- `NewDispatcher` 默认 `DefaultSinkConcurrency = MaxConcurrency`，server 模式由 Task 12 显式传入共享默认值。
- `HookDeliveryRepository.Requeue` 现已存在，dispatcher 直接复用；如果实现过程中调整签名，必须同步更新 hook runtime 测试。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/hookruntime ./internal/httpapi -run 'TestHookDispatcher.*Concurrency|TestHookDispatcherSink|TestHookDispatcherDoesNotLeaveSinkLimited' -count=1
```

预期：通过。

### Task 11: hook dispatcher 补齐重启恢复验收

**文件：**
- 修改：`internal/hookruntime/dispatcher_test.go`

- [x] **步骤 1: 写失败测试**

新增：

```go
func TestHookDispatcherRecoversStaleDeliveringOnRunOnce(t *testing.T) {
	// 插入 status=delivering、claim_expires_at=999 的 hook delivery，clock.now=1000。
	// RunOnce 应恢复并重新投递。
}

func TestHookDispatcherKeepsFutureDeliveringInvisible(t *testing.T) {
	// 插入 status=delivering、claim_expires_at=1200，clock.now=1000。
	// RunOnce 不应投递它。
}

func TestHookDispatcherRetryWaitSurvivesUntilNextAttempt(t *testing.T) {
	// 插入 status=retry_wait、next_attempt_at=1200，clock.now=1000。
	// RunOnce 不应投递它。
}
```

- [x] **步骤 2: 跑测试确认失败或确认现有行为**

运行：

```bash
go test ./internal/hookruntime -run 'TestHookDispatcher.*Stale|TestHookDispatcher.*RetryWait|TestHookDispatcher.*FutureDelivering' -count=1
```

预期：如果现有代码已经满足，测试通过；如果没有覆盖或行为不完整，失败后按下一步修复。

- [x] **步骤 3: 最小实现**

保持或补齐：

- `RunOnce` 最开始调用 `RecoverStaleDelivering(now)`。
- claim 查询只领取 `queued` 和到期 `retry_wait`。
- 未过期 `delivering` 不被 claim。
- `claim_expires_at` 晚于 hook/sink timeout。

- [x] **步骤 4: 跑测试确认通过**

运行：

```bash
go test ./internal/hookruntime -run 'TestHookDispatcher.*Stale|TestHookDispatcher.*RetryWait|TestHookDispatcher.*FutureDelivering|TestHookDispatcher' -count=1
```

预期：通过。

---

## Chunk 4: Server 接入、文档与全量验收

### Task 12: server 命令接入 dispatcher 配置和共享 sink limiter

**文件：**
- 修改：`internal/cli/server.go`
- 新增：`internal/cli/server_test.go`

- [x] **步骤 1: 写失败测试**

新增 `internal/cli/server_test.go`，测试一个纯函数，不启动 HTTP server：

```go
func TestServerDispatcherRuntimeOptionsUseConfigAndFlags(t *testing.T) {
	cfg := config.Config{
		NotificationDispatcher: config.DispatcherConfig{MaxConcurrency: 1, BatchSize: 50, PrefetchFactor: 1, PollInterval: 5 * time.Second, ClaimTTL: 5 * time.Minute},
		HookDispatcher: config.DispatcherConfig{MaxConcurrency: 1, BatchSize: 40, PrefetchFactor: 1, PollInterval: 6 * time.Second, ClaimTTL: 6 * time.Minute},
	}
	flags := serverDispatcherFlagOverrides{
		NotificationMaxConcurrency: ptrInt(4),
		LegacyNotificationInterval: ptrDuration(7 * time.Second),
	}
	opts := buildServerDispatcherRuntimeOptions(cfg, flags)
	if opts.Notification.MaxConcurrency != 4 {
		t.Fatalf("notification max = %d, want 4", opts.Notification.MaxConcurrency)
	}
	if opts.Notification.PollInterval != 7*time.Second {
		t.Fatalf("notification interval = %v, want 7s", opts.Notification.PollInterval)
	}
	if opts.Hook.PollInterval != 7*time.Second {
		t.Fatalf("hook interval = %v, want legacy 7s when hook flag absent", opts.Hook.PollInterval)
	}
	if opts.SinkLimiter == nil || opts.Notification.SinkLimiter != opts.Hook.SinkLimiter {
		t.Fatal("notification and hook dispatchers must share one sink limiter")
	}
	if opts.Notification.DefaultSinkConcurrency != opts.Hook.DefaultSinkConcurrency {
		t.Fatalf("default sink concurrency mismatch: notification=%d hook=%d", opts.Notification.DefaultSinkConcurrency, opts.Hook.DefaultSinkConcurrency)
	}
	if opts.Notification.DefaultSinkConcurrency != 1 {
		t.Fatalf("default sink concurrency = %d, want min(notification, hook)=1", opts.Notification.DefaultSinkConcurrency)
	}
}

func TestServerHookDispatcherIntervalFlagWinsOverLegacyInterval(t *testing.T) {
	cfg := config.Config{
		NotificationDispatcher: config.DispatcherConfig{MaxConcurrency: 1, BatchSize: 50, PrefetchFactor: 1, PollInterval: 5 * time.Second, ClaimTTL: 5 * time.Minute},
		HookDispatcher: config.DispatcherConfig{MaxConcurrency: 1, BatchSize: 50, PrefetchFactor: 1, PollInterval: 6 * time.Second, ClaimTTL: 5 * time.Minute},
	}
	flags := serverDispatcherFlagOverrides{
		LegacyNotificationInterval: ptrDuration(7 * time.Second),
		HookInterval: ptrDuration(8 * time.Second),
	}
	opts := buildServerDispatcherRuntimeOptions(cfg, flags)
	if opts.Hook.PollInterval != 8*time.Second {
		t.Fatalf("hook interval = %v, want hook flag 8s", opts.Hook.PollInterval)
	}
}
```

- [x] **步骤 2: 最小实现**

在 `internal/cli/server.go`：

- 分离 notification 和 hook dispatcher interval。
- 从 `cfg.NotificationDispatcher` / `cfg.HookDispatcher` 初始化 options。
- 只有 flag 显式传入时覆盖。
- 新增内部纯函数 `buildServerDispatcherRuntimeOptions(cfg config.Config, flags serverDispatcherFlagOverrides) serverDispatcherRuntimeOptions`，供 server command 和测试共同使用。
- `serverDispatcherRuntimeOptions` 内部包含一个共享 `*runtimeutil.SinkLimiter`，并把同一个指针赋给 notification / hook dispatcher options。
- `defaultSinkConcurrency` 由 server helper 统一计算，而不是让两个 dispatcher 各自用自己的 `MaxConcurrency` 推导：

```go
defaultSinkConcurrency := min(
	runtimeutil.EffectiveConcurrency(notificationCfg.MaxConcurrency),
	runtimeutil.EffectiveConcurrency(hookCfg.MaxConcurrency),
)
if defaultSinkConcurrency < 1 {
	defaultSinkConcurrency = 1
}
```

- notification 和 hook dispatcher options 都必须写入同一个 `DefaultSinkConcurrency`。这样 sink `max_concurrency=0` 的继承语义在同一 server 进程内稳定，不会因为 notification dispatcher 配了 4、hook dispatcher 配了 2 而让同一个 sink 在不同 runtime 下出现不同上限。

构造 options：

```go
hookDispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
	Store: store,
	Clock: app.RealClock{},
	Version: "dev",
	BatchSize: hookCfg.BatchSize,
	PollInterval: hookCfg.PollInterval,
	ClaimTTL: hookCfg.ClaimTTL,
	MaxConcurrency: hookCfg.MaxConcurrency,
	PrefetchFactor: hookCfg.PrefetchFactor,
	SinkLimiter: sharedSinkLimiter,
	DefaultSinkConcurrency: defaultSinkConcurrency,
})
```

notification 同理。

保留旧 flag `--notification-dispatcher-interval`，并新增 `--hook-dispatcher-interval`。兼容规则必须和 Task 3 一致：如果用户没有显式传 `--hook-dispatcher-interval`，旧 flag 也作为 hook interval 的兼容别名；如果显式传了 hook 专用 flag，则 hook 专用 flag 优先。

- [x] **步骤 3: 跑测试确认通过**

运行：

```bash
go test ./internal/cli -run 'TestServer|TestRoot|TestNotification' -count=1
go test ./internal/config -run 'TestResolveDispatcherConfig' -count=1
```

预期：通过。

### Task 13: 更新文档与示例配置

**文件：**
- 修改：`config.example.toml`
- 修改：`README.md`
- 修改：`docs/manual/notifications.md`
- 修改：`docs/manual/hooks.md`
- 修改：`docs/manual/mcp.md`
- 修改：`docs/openapi/xuanchu-v1.yaml`
- 修改：`docs/skills/*/SKILL.md` 中涉及 notification sink 的说明

- [x] **步骤 1: 更新 `config.example.toml`**

加入：

```toml
[notifications.dispatcher]
max_concurrency = 1
batch_size = 50
prefetch_factor = 1
poll_interval = "5s"
claim_ttl = "5m"

[hooks.dispatcher]
max_concurrency = 1
batch_size = 50
prefetch_factor = 1
poll_interval = "5s"
claim_ttl = "5m"
```

- [x] **步骤 2: 更新 README / manual**

必须说明：

- DB delivery 表是唯一可靠队列。
- `batch_size` 不是并发。
- 默认 `max_concurrency=1`。
- sink `max_concurrency=0` 表示继承默认 sink 并发：单 dispatcher 使用时继承该 dispatcher 的 `max_concurrency`；`xuanchu server` 同时运行 notification / hook dispatcher 时继承 server 统一计算出的 `DefaultSinkConcurrency`，当前为两个 dispatcher `max_concurrency` 的较小值。
- server 进程内 notification dispatcher 和 hook dispatcher 共享 sink limiter；同一 sink 的并发不会因为两个 runtime 同时工作而在单进程内翻倍。
- 多实例部署整体并发约等于单实例乘以副本数。
- 进程重启后 queued / retry_wait / delivering 不丢，delivering 靠 `claim_expires_at` 恢复。
- payload / `http_template` context 提供 `delivery.id`、`delivery.attempt`、`delivery.workspace_id`、`delivery.sink_id` 等幂等字段。

- [x] **步骤 3: 更新 OpenAPI / MCP / skills**

所有 notification sink 的 request/response schema 增加：

```yaml
max_concurrency:
  type: integer
  minimum: 0
  description: 当前进程内该 sink 的最大并发投递数，0 表示继承默认 sink 并发；server 模式下继承 notification/hook dispatcher 统一计算出的默认值。
```

MCP skill 文档同步 `notification_sink_add` / `notification_sink_modify` 字段。

- [x] **步骤 4: 跑文档和编译检查**

运行：

```bash
git diff --check
go test ./internal/config ./internal/app ./internal/storage ./internal/notificationruntime ./internal/hookruntime ./internal/httpapi ./internal/cli ./internal/mcpserver -count=1
```

预期：通过。

### Task 14: 全量验收

**文件：**
- 无额外文件。

- [x] **步骤 1: 跑全量测试**

运行：

```bash
go test ./...
```

预期：通过。

- [x] **步骤 2: 跑 CGO disabled 测试**

运行：

```bash
CGO_ENABLED=0 go test ./...
```

预期：通过。

- [x] **步骤 3: 跑 CGO disabled build**

运行：

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

预期：通过。

- [x] **步骤 4: 检查工作区**

运行 PostgreSQL 测试口径：

```bash
go test ./internal/storage -run 'Postgres|NotificationDeliveryClaimDueSQLAvoidsPostgresUntypedMax|HookDeliveryClaimDueSQLAvoidsPostgresUntypedMax' -count=1
```

预期：

- 未设置 `XUANCHU_TEST_DB_URL` 时，PostgreSQL 集成测试应被 skip，SQL 生成/兼容测试仍通过。
- 如果本地提供 `XUANCHU_TEST_DB_URL='postgres://...'`，上述命令必须实际连接 PostgreSQL 并通过。
- storage 层必须同时覆盖 notification 和 hook 的 claim SQL 兼容性，至少包含：
  - `TestNotificationDeliveryClaimDueSQLAvoidsPostgresUntypedMax`
  - `TestHookDeliveryClaimDueSQLAvoidsPostgresUntypedMax`
- 两个测试都要验证 claim SQL 不使用 PostgreSQL 无法推断类型的 `MAX(?...)` / `max(unknown, bigint)` 形态；应使用 `CASE` 或显式 `CAST(? AS BIGINT)`。如果当前 hook claim SQL 还没有可单测 helper，本计划要求先抽出等价 helper 或添加字符串级 SQL 生成测试。

- [x] **步骤 5: 检查工作区**

运行：

```bash
git status --short
git diff --check
```

预期：只包含本计划相关文件，无 whitespace error。

- [ ] **步骤 6: 提交**

```bash
git add internal docs README.md config.example.toml
git commit -m "feat: 增加 dispatcher 并发与背压控制"
```

---

## 风险与注意事项

- sink token 满时 requeue 会让 `attempt_count` 因 claim 预增。首版可以接受，但需要测试确保不会保持 `delivering`；后续如果要精确 attempt，可改为 claim 前按 sink capacity 过滤。
- 默认 `max_concurrency=1` 是兼容当前行为的关键，不能在无明确配置时突然改成并发投递。
- `RunOnce` 在测试中应等待本轮 worker 全部结束后返回，避免测试不稳定。
- context canceled 时不要把未确认结果的 delivery 直接标记失败；让 stale recovery 接手更稳。
- 多实例严格全局并发不在本计划范围内，不要引入分布式锁。
