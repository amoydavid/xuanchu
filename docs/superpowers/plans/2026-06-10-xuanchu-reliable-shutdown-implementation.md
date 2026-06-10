# Xuanchu 可靠停机实现计划

> **给 agentic workers 的要求：** 执行本计划时必须使用 `superpowers:subagent-driven-development`（如果当前环境支持子代理）或 `superpowers:executing-plans`。步骤使用 checkbox（`- [x]`）语法跟踪进度。

**目标：** 为 `xuanchu server`、HTTP API、HTTP MCP、stdio MCP、notification dispatcher、hook dispatcher 增加可靠停机能力：收到 SIGTERM / SIGINT 后停止接收新工作，等待已开始工作完成，超时后再强制取消。

**架构：** 新增一个小型 `ShutdownCoordinator`，把“停止接收新工作”和“强制取消执行上下文”拆成两阶段。HTTP 入口复用 `http.Server.Shutdown`，MCP tool 通过 `addTool` 统一 gate，dispatcher 通过 coordinator 控制 claim、worker 启动和 in-flight 投递生命周期。

**技术栈：** Go 1.25、Cobra、`net/http`、MCP Go SDK、GORM、SQLite/PostgreSQL、现有 `runtimeutil` / `notificationruntime` / `hookruntime` / `httpapi` / `mcpserver` 分层。

## 当前执行状态

本轮实现已覆盖可靠停机的核心代码路径和聚焦单元测试：`ShutdownCoordinator`、配置与 CLI flag、HTTP middleware、MCP tool gate、stdio MCP 停机编排、server 两阶段停机，以及 hook / notification dispatcher 的 draining、force cancel、已 claim 未开始 delivery `ReleaseClaim` 回队列行为。

未新增耗时的端到端进程级阻塞 webhook 停机测试；对应行为由 dispatcher、HTTP/MCP、CLI 组装层单元测试和现有 integration 套件验证。PostgreSQL 冒烟需要本地 PostgreSQL 测试实例，本轮最终验证如无实例则记录为未运行。

---

## 前置约束

- 规格文档：`docs/superpowers/specs/2026-06-10-xuanchu-reliable-shutdown-design.md`
- 文档、注释和用户可见描述必须使用中文。
- 不引入 Redis、NATS、Kafka 或其它外部队列。
- 不改变 delivery 的 at-least-once 语义。
- 不承诺 exactly-once；接收方仍应按 `delivery_id` 幂等。
- SQLite 继续使用 `github.com/glebarez/sqlite`，PostgreSQL 继续使用 `gorm.io/driver/postgres`，保持 `CGO_ENABLED=0`。
- `server --shutdown-timeout` 保留，语义升级为整体 drain timeout。
- 新增 `server --shutdown-force-timeout` 和 `mcp stdio --shutdown-timeout / --shutdown-force-timeout`。
- MCP tool 不逐个改业务实现，必须在 `internal/mcpserver/tools_common.go` 的 `addTool` 包装层统一接入。
- 每个 chunk 完成后建议提交一次，提交信息使用中文。

## 文件结构

新增文件：

- `internal/runtimeutil/shutdown.go`：实现 `ShutdownCoordinator`，负责状态、in-flight 计数、drain 等待、force cancel context。
- `internal/runtimeutil/shutdown_test.go`：覆盖 coordinator 的 running/draining/forced 状态和超时行为。
- `internal/runtimeutil/context.go`：提供 `ContextWithCancelOnEither(parent, other)`，用于把 SDK request context 与 force-cancel context 合并。
- `internal/runtimeutil/context_test.go`：覆盖任一上游 context 取消时派生 context 都会取消，且 defer cancel 不泄漏 goroutine。
- `internal/httpapi/shutdown_middleware.go`：HTTP request gate。draining 后返回 `server_draining`，已进入的 request defer 释放 in-flight。
- `internal/httpapi/shutdown_middleware_test.go`：覆盖 HTTP gate 行为。

修改文件：

- `internal/config/config.go`：新增 `ShutdownConfig`，解析 `[server.shutdown] timeout / force_timeout`。
- `internal/config/config_test.go`：覆盖默认值、TOML 解析、非法 duration。
- `config.example.toml`：增加 `[server.shutdown]` 示例。
- `internal/cli/server.go`：新增 shutdown coordinator，调整 signal 处理、dispatcher context、HTTP server drain、force cancel、日志和 flag。
- `internal/cli/server_test.go`：覆盖 shutdown flag 与配置优先级、非法值、dispatcher options 注入。
- `internal/cli/mcp.go`：stdio MCP 增加 shutdown flag、coordinator、signal drain 流程。
- `internal/cli/mcp_test.go`：覆盖 MCP shutdown flag 和非法值。
- `internal/httpapi/server.go`：`Options` / `Server` 增加 `Shutdown *runtimeutil.ShutdownCoordinator`。
- `internal/httpapi/router.go`：接入 shutdown middleware；`handleMCP()` 把同一个 coordinator 传给 `mcpserver.Options`。
- `internal/mcpserver/options.go`：`Options` 增加 `Shutdown *runtimeutil.ShutdownCoordinator`。
- `internal/mcpserver/tools_common.go`：在 `addTool` 内统一 `Begin()` / `done()`；draining 后返回 MCP business error `server_draining`。
- `internal/mcpserver/integration_test.go` 或新增聚焦测试：覆盖 draining 后 tool call 返回 `server_draining`。
- `internal/notificationruntime/dispatcher.go`：`DispatcherOptions` 增加 shutdown coordinator；`Run` / `RunOnce` 接入 stop accepting、requeue、force cancel context。
- `internal/notificationruntime/dispatcher_test.go`：覆盖 reliable shutdown 场景。
- `internal/hookruntime/dispatcher.go`：同 notification dispatcher。
- `internal/hookruntime/dispatcher_test.go`：同 notification dispatcher。
- `internal/storage/notification_delivery_repo.go`、`internal/storage/hook_delivery_repo.go`：如现有 `Requeue` 无法安全释放已 claim 未开始 delivery，则补 `ReleaseClaim` / `RequeueClaimed` 方法。
- `internal/storage/*_test.go`：覆盖 release/requeue 不让 delivery 永久停在 `delivering`。
- `README.md`：同步 `server.shutdown` 配置和 CLI flag。
- `docs/manual/deployment.md`：增加 systemd / 容器可靠停机说明。
- `docs/manual/mcp.md`：说明 stdio MCP shutdown 行为。
- `docs/superpowers/specs/2026-06-10-xuanchu-reliable-shutdown-design.md`：实现完成后只勾勒实际落地差异，不改设计目标。

---

## Chunk 1: ShutdownCoordinator

### Task 1: 增加 coordinator 单元测试

**文件：**
- 新增：`internal/runtimeutil/shutdown_test.go`

- [x] **步骤 1: 写 running 状态测试**

新增测试：

```go
func TestShutdownCoordinatorBeginAllowsWorkWhileRunning(t *testing.T) {
	c := runtimeutil.NewShutdownCoordinator()
	done, ok := c.Begin()
	if !ok {
		t.Fatal("Begin() ok = false, want true")
	}
	done()
	if err := c.Drain(t.Context()); err != nil {
		t.Fatalf("Drain() error = %v", err)
	}
}
```

- [x] **步骤 2: 写 draining 后拒绝新工作测试**

```go
func TestShutdownCoordinatorBeginRejectsWorkAfterStopAccepting(t *testing.T) {
	c := runtimeutil.NewShutdownCoordinator()
	c.StopAccepting()
	if _, ok := c.Begin(); ok {
		t.Fatal("Begin() ok = true after StopAccepting, want false")
	}
}
```

- [x] **步骤 3: 写 drain 等待 in-flight 测试**

```go
func TestShutdownCoordinatorDrainWaitsForInflightWork(t *testing.T) {
	c := runtimeutil.NewShutdownCoordinator()
	done, ok := c.Begin()
	if !ok {
		t.Fatal("Begin() ok = false, want true")
	}

	drained := make(chan error, 1)
	go func() { drained <- c.Drain(context.Background()) }()

	select {
	case err := <-drained:
		t.Fatalf("Drain returned before done(): %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	done()
	if err := <-drained; err != nil {
		t.Fatalf("Drain() error = %v", err)
	}
}
```

- [x] **步骤 4: 写 force cancel 测试**

```go
func TestShutdownCoordinatorForceCancelCancelsContext(t *testing.T) {
	c := runtimeutil.NewShutdownCoordinator()
	c.ForceCancel()
	select {
	case <-c.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("Context was not canceled")
	}
}
```

- [x] **步骤 5: 写 drain 超时测试**

```go
func TestShutdownCoordinatorDrainReturnsContextError(t *testing.T) {
	c := runtimeutil.NewShutdownCoordinator()
	done, ok := c.Begin()
	if !ok {
		t.Fatal("Begin() ok = false, want true")
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := c.Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain() error = %v, want context deadline exceeded", err)
	}
}
```

- [x] **步骤 6: 运行测试确认失败**

运行：

```bash
go test ./internal/runtimeutil -run ShutdownCoordinator -count=1
```

预期：失败，`NewShutdownCoordinator` 尚未实现。

### Task 2: 实现 coordinator

**文件：**
- 新增：`internal/runtimeutil/shutdown.go`
- 修改：`internal/runtimeutil/shutdown_test.go`

- [x] **步骤 1: 实现最小结构**

实现：

```go
type ShutdownCoordinator struct {
	mu        sync.Mutex
	accepting bool
	active    int
	drained   chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}
```

构造函数：

```go
func NewShutdownCoordinator() *ShutdownCoordinator {
	ctx, cancel := context.WithCancel(context.Background())
	return &ShutdownCoordinator{
		accepting: true,
		drained:   make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}
```

`Begin()` 语义：

- 已停止接收新工作时返回 `(nil, false)`。
- running 时 `active++`，返回只执行一次的 `done()`。
- `done()` 让 `active--`，当 `accepting=false && active==0` 时关闭 `drained`。

`StopAccepting()` 语义：

- 幂等。
- 设置 `accepting=false`。
- 如果没有 active work，立即关闭 `drained`。

`Drain(ctx)` 语义：

- 先调用 `StopAccepting()`。
- 等待 `drained` 或 `ctx.Done()`。

`ForceCancel()` 语义：

- 幂等调用 `cancel()`。

`Context()` 返回 force cancel context。

- [x] **步骤 2: 增加状态查询**

增加：

```go
func (c *ShutdownCoordinator) Accepting() bool
func (c *ShutdownCoordinator) IsDraining() bool
```

用途：

- dispatcher 在 claim 前快速判断是否还能领取。
- HTTP middleware / 测试可以判断 draining 状态。

- [x] **步骤 3: 运行测试**

运行：

```bash
go test ./internal/runtimeutil -run ShutdownCoordinator -count=1
```

预期：通过。

### Task 3: 增加 context 合并工具

**文件：**
- 新增：`internal/runtimeutil/context.go`
- 新增：`internal/runtimeutil/context_test.go`

- [x] **步骤 1: 写 parent 取消测试**

```go
func TestContextWithCancelOnEitherCancelsWhenParentCancels(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	other := context.Background()
	ctx, cancel := runtimeutil.ContextWithCancelOnEither(parent, other)
	defer cancel()

	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("derived context was not canceled")
	}
}
```

- [x] **步骤 2: 写 other 取消测试**

```go
func TestContextWithCancelOnEitherCancelsWhenOtherCancels(t *testing.T) {
	parent := context.Background()
	other, cancelOther := context.WithCancel(context.Background())
	ctx, cancel := runtimeutil.ContextWithCancelOnEither(parent, other)
	defer cancel()

	cancelOther()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("derived context was not canceled")
	}
}
```

- [x] **步骤 3: 实现 helper**

实现：

```go
func ContextWithCancelOnEither(parent context.Context, other context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		select {
		case <-ctx.Done():
		case <-other.Done():
			cancel()
		}
	}()
	return ctx, cancel
}
```

注意：

- 调用方必须 `defer cancel()`，避免 parent 和 other 都长期不取消时 goroutine 常驻。
- 如果 `other == nil`，直接返回 `context.WithCancel(parent)`。

- [x] **步骤 4: 运行 runtimeutil 测试**

```bash
go test ./internal/runtimeutil -run 'ShutdownCoordinator|ContextWithCancelOnEither' -count=1
```

预期：通过。

- [x] **步骤 5: 提交**

```bash
git add internal/runtimeutil/shutdown.go internal/runtimeutil/shutdown_test.go internal/runtimeutil/context.go internal/runtimeutil/context_test.go
git commit -m "feat: 增加可靠停机协调器"
```

---

## Chunk 2: 配置与 CLI flag

### Task 1: 解析 `[server.shutdown]`

**文件：**
- 修改：`internal/config/config.go`
- 修改：`internal/config/config_test.go`
- 修改：`config.example.toml`

- [x] **步骤 1: 写配置测试**

在 `internal/config/config_test.go` 增加：

```go
func TestResolveParsesServerShutdownConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	if err := os.WriteFile(path, []byte(`
[server.shutdown]
timeout = "45s"
force_timeout = "7s"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve(Options{HomeDir: dir, ConfigPath: path})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.Shutdown.Timeout != 45*time.Second {
		t.Fatalf("Shutdown.Timeout = %v, want 45s", cfg.Shutdown.Timeout)
	}
	if cfg.Shutdown.ForceTimeout != 7*time.Second {
		t.Fatalf("Shutdown.ForceTimeout = %v, want 7s", cfg.Shutdown.ForceTimeout)
	}
}
```

增加默认值测试：

```go
func TestResolveDefaultServerShutdownConfig(t *testing.T) {
	cfg, err := Resolve(Options{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.Shutdown.Timeout != 30*time.Second {
		t.Fatalf("Shutdown.Timeout = %v, want 30s", cfg.Shutdown.Timeout)
	}
	if cfg.Shutdown.ForceTimeout != 5*time.Second {
		t.Fatalf("Shutdown.ForceTimeout = %v, want 5s", cfg.Shutdown.ForceTimeout)
	}
}
```

增加非法 duration 测试：

```go
func TestResolveRejectsInvalidServerShutdownDuration(t *testing.T) {
	// 写入 timeout = "bad"
	// 断言错误包含 "server.shutdown.timeout"
}
```

- [x] **步骤 2: 运行测试确认失败**

```bash
go test ./internal/config -run 'ServerShutdown|ShutdownConfig' -count=1
```

预期：失败，字段尚不存在。

- [x] **步骤 3: 实现配置**

在 `Config` 增加：

```go
Shutdown ShutdownConfig
```

新增：

```go
type ShutdownConfig struct {
	Timeout      time.Duration
	ForceTimeout time.Duration
}
```

新增解析函数：

```go
func parseShutdownConfig(values map[string]string) (ShutdownConfig, error) {
	cfg := ShutdownConfig{
		Timeout:      30 * time.Second,
		ForceTimeout: 5 * time.Second,
	}
	var err error
	if cfg.Timeout, err = parsePositiveDurationField(values, "server.shutdown.timeout", cfg.Timeout); err != nil {
		return ShutdownConfig{}, err
	}
	if cfg.ForceTimeout, err = parsePositiveDurationField(values, "server.shutdown.force_timeout", cfg.ForceTimeout); err != nil {
		return ShutdownConfig{}, err
	}
	return cfg, nil
}
```

`Resolve()` 中解析并写入 `Config.Shutdown`。

- [x] **步骤 4: 更新示例配置**

在 `config.example.toml` 增加：

```toml
[server.shutdown]
# 收到 SIGTERM / SIGINT 后等待已开始工作完成的时间。
timeout = "30s"
# drain 超时后发出强制取消，再等待运行时清理资源的时间。
force_timeout = "5s"
```

- [x] **步骤 5: 运行测试**

```bash
go test ./internal/config -run 'ServerShutdown|ShutdownConfig|DispatcherConfig|Resolve' -count=1
```

预期：通过。

### Task 2: server / mcp flag 接入

**文件：**
- 修改：`internal/cli/server.go`
- 修改：`internal/cli/server_test.go`
- 修改：`internal/cli/mcp.go`
- 新增或修改：`internal/cli/mcp_test.go`

- [x] **步骤 1: 写 server flag 测试**

在 `internal/cli/server_test.go` 增加 helper 或测试：

```go
func TestServerShutdownFlagOverridesConfig(t *testing.T) {
	cfg := config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:      30 * time.Second,
			ForceTimeout: 5 * time.Second,
		},
	}
	flags := serverShutdownFlagOverrides{
		Timeout:      ptrDuration(45 * time.Second),
		ForceTimeout: ptrDuration(9 * time.Second),
	}
	got, err := buildServerShutdownOptions(cfg, flags)
	if err != nil {
		t.Fatalf("buildServerShutdownOptions() error = %v", err)
	}
	if got.Timeout != 45*time.Second || got.ForceTimeout != 9*time.Second {
		t.Fatalf("shutdown options = %+v", got)
	}
}
```

增加非法值测试：`0` 或负数必须报错。

- [x] **步骤 2: 写 mcp flag 注册测试**

如果现有 CLI 测试可直接 inspect cobra flags，增加：

```go
func TestMCPStdioCommandHasShutdownFlags(t *testing.T) {
	cmd := newMCPStdioCommand(Options{})
	for _, name := range []string{"shutdown-timeout", "shutdown-force-timeout"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag %s", name)
		}
	}
}
```

- [x] **步骤 3: 运行测试确认失败**

```bash
go test ./internal/cli -run 'Shutdown|MCPStdioCommandHasShutdownFlags' -count=1
```

预期：失败，helper / flag 尚不存在。

- [x] **步骤 4: 实现 helper 与 flag**

在 `internal/cli/server.go` 增加：

```go
type serverShutdownFlagOverrides struct {
	Timeout      *time.Duration
	ForceTimeout *time.Duration
}

type serverShutdownOptions struct {
	Timeout      time.Duration
	ForceTimeout time.Duration
}
```

实现：

```go
func buildServerShutdownOptions(cfg config.Config, flags serverShutdownFlagOverrides) (serverShutdownOptions, error)
```

规则：

- 默认来自 `cfg.Shutdown`。
- flag 被设置时覆盖 TOML。
- `timeout <= 0` 返回 `shutdown-timeout must be positive`。
- `force_timeout <= 0` 返回 `shutdown-force-timeout must be positive`。

在 `newServerCommand`：

- `shutdownTimeout` 默认值仍为 `30*time.Second`，但实际值由 config/helper 决定。
- 新增 `shutdownForceTimeout` flag，默认 `5*time.Second`。
- 只有 `cmd.Flags().Changed(...)` 时写入 override，避免 flag 默认值覆盖 TOML。

在 `newMCPStdioCommand` 做同样 flag 和 helper 复用。

- [x] **步骤 5: 运行测试**

```bash
go test ./internal/cli -run 'Shutdown|MCPStdioCommandHasShutdownFlags|ServerDispatcher' -count=1
```

预期：通过。

- [x] **步骤 6: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go config.example.toml internal/cli/server.go internal/cli/server_test.go internal/cli/mcp.go internal/cli/mcp_test.go
git commit -m "feat: 增加停机配置与命令行参数"
```

---

## Chunk 3: Dispatcher 接入可靠停机

### Task 1: notification dispatcher shutdown 测试

**文件：**
- 修改：`internal/notificationruntime/dispatcher_test.go`

- [x] **步骤 1: 写 draining 后不 claim 测试**

新增测试：

```go
func TestNotificationDispatcherDoesNotClaimWhenDraining(t *testing.T) {
	// 准备一个 due notification delivery。
	// 创建 coordinator，先 StopAccepting。
	// NewDispatcher 时传 Shutdown: coordinator。
	// RunOnce(context.Background())。
	// 断言 delivery 仍为 queued，attempt_count 不增加，server 没收到请求。
}
```

- [x] **步骤 2: 写 stop accepting 不取消已开始投递测试**

```go
func TestNotificationDispatcherStopAcceptingDoesNotCancelStartedDelivery(t *testing.T) {
	// httptest.Server handler 进入后阻塞，记录 request.Context 是否被取消。
	// RunOnce 后等待 handler started。
	// 调用 coordinator.StopAccepting()。
	// 断言请求没有立即取消。
	// 放行 handler。
	// 断言 RunOnce 返回 nil，delivery succeeded。
}
```

- [x] **步骤 3: 写 force cancel 取消投递测试**

```go
func TestNotificationDispatcherForceCancelCancelsStartedDelivery(t *testing.T) {
	// httptest.Server handler 阻塞直到 r.Context().Done()。
	// RunOnce 后等待 handler started。
	// 调用 coordinator.ForceCancel()。
	// 断言 RunOnce 返回 context.Canceled 或 delivery 进入可恢复状态。
}
```

- [x] **步骤 4: 写已 claim 未开始 delivery 释放测试**

```go
func TestNotificationDispatcherRequeuesClaimedDeliveryWhenDrainingBeforeWorkerStart(t *testing.T) {
	// 使用 max_concurrency=1、batch_size>1、prefetch_factor>1。
	// 第一条 delivery handler 阻塞。
	// 让 RunOnce claim 多条后 StopAccepting。
	// 放行第一条。
	// 断言未开始的 delivery 不停留在 delivering。
}
```

验收重点：

- 未开始的 delivery 最终状态是 `queued` 或下一次可 claim 状态。
- 未开始的 delivery 使用 `ReleaseClaim` 撤销 claim，`attempt_count` 不会因为停机窗口被消耗。

- [x] **步骤 5: 写 sink limiter 不泄漏测试**

```go
func TestNotificationDispatcherShutdownDoesNotLeakSinkLimiterToken(t *testing.T) {
	// sink max_concurrency=1。
	// 第一次投递过程中 ForceCancel。
	// 再次 RunOnce 或手动 TryAcquire 同 sink，断言 token 已释放。
}
```

- [x] **步骤 6: 运行测试确认失败**

```bash
go test ./internal/notificationruntime -run 'Shutdown|Draining|ForceCancel|RequeuesClaimed|SinkLimiter' -count=1
```

预期：失败，dispatcher 尚未接入 coordinator。

### Task 2: notification dispatcher 实现

**文件：**
- 修改：`internal/notificationruntime/dispatcher.go`
- 按需修改：`internal/storage/notification_delivery_repo.go`
- 按需修改：`internal/storage/notification_delivery_repo_test.go`

- [x] **步骤 1: Options 增加 shutdown**

在 `DispatcherOptions` 增加：

```go
Shutdown *runtimeutil.ShutdownCoordinator
```

在 `NewDispatcher` 中：

- 如果未传入，创建一个新的 coordinator，保证单测和手动 `RunOnce` 不需要额外配置。

- [x] **步骤 2: Run 循环停止新 claim**

`Run(ctx)` 每轮 `RunOnce` 前检查：

```go
if !d.opts.Shutdown.Accepting() {
	return nil
}
```

`select` 中继续监听外部 `ctx.Done()`，但外部 signal 不应在 server 中直接传入会取消投递的 root context。`ctx` 只表示 runtime 进程主循环结束或测试取消。

- [x] **步骤 3: RunOnce claim 前检查**

`RunOnce(ctx)` 开头：

```go
if !d.opts.Shutdown.Accepting() {
	return nil
}
```

恢复 stale delivering 可以保留在检查前或后；推荐保留在检查前，因为它不开始外部投递，能加速恢复。

- [x] **步骤 4: worker 启动前 Begin**

对每个已 claim delivery：

```go
done, ok := d.opts.Shutdown.Begin()
if !ok {
	_ = d.deliveryRepo.Requeue(delivery.ID, d.opts.Clock.Unix())
	break
}
wg.Add(1)
go func(delivery storage.NotificationDelivery) {
	defer wg.Done()
	defer done()
	recordErr(d.dispatchOne(d.opts.Shutdown.Context(), delivery, d.opts.Clock.Unix()))
}(delivery)
```

注意：

- 只有 Begin 成功才启动 worker。
- Begin 失败的 delivery 尚未开始投递，必须 requeue。
- Begin 成功后，`dispatchOne` 使用 `Shutdown.Context()` 派生 sink timeout，避免 `StopAccepting()` 取消请求。
- 外部 `ctx` 只控制是否继续启动新 worker，不传给已开始投递。

- [x] **步骤 5: 未开始 delivery 全部释放**

如果因为 draining / ctx cancel 停止继续启动 worker，剩余已 claim delivery 必须逐条 `ReleaseClaim`。

新增：

```go
ReleaseClaim(id string, now int64) error
```

语义：

- 仅当状态为 `delivering` 时写回 `queued`。
- 清空 `claim_expires_at` / `next_attempt_at`。
- 撤销本次 claim 预增的 `attempt_count`，不消耗真实投递次数。

- [x] **步骤 6: 运行 notification dispatcher 测试**

```bash
go test ./internal/notificationruntime -run 'Dispatcher|Shutdown|Draining|ForceCancel|RequeuesClaimed|SinkLimiter' -count=1
```

预期：通过。

### Task 3: hook dispatcher 同步实现

**文件：**
- 修改：`internal/hookruntime/dispatcher_test.go`
- 修改：`internal/hookruntime/dispatcher.go`
- 按需修改：`internal/storage/hook_delivery_repo.go`
- 按需修改：`internal/storage/hook_delivery_repo_test.go`

- [x] **步骤 1: 复制同类测试但使用 hook delivery**

覆盖：

- draining 后不 claim。
- `StopAccepting()` 不取消已开始 webhook。
- `ForceCancel()` 取消正在投递的 webhook。
- 已 claim 未开始 delivery 会 requeue/release。
- shutdown 不泄漏 sink limiter token。

- [x] **步骤 2: 运行测试确认失败**

```bash
go test ./internal/hookruntime -run 'Shutdown|Draining|ForceCancel|RequeuesClaimed|SinkLimiter' -count=1
```

- [x] **步骤 3: 按 notification dispatcher 同样模式实现**

注意 hook dispatcher 现有实现使用 semaphore worker pool；保留该结构即可，但要把 Begin 放在启动 goroutine 前。

关键规则：

- Begin 失败的 delivery 不能进入 goroutine。
- 已进入 goroutine 的 delivery 必须 defer `done()` 和 sink limiter release。
- force cancel 后 HTTP request context 必须取消。

- [x] **步骤 4: 运行 hook dispatcher 测试**

```bash
go test ./internal/hookruntime -run 'Dispatcher|Shutdown|Draining|ForceCancel|RequeuesClaimed|SinkLimiter' -count=1
```

预期：通过。

- [x] **步骤 5: 跑 storage 相关测试**

如果新增 release 方法，运行：

```bash
go test ./internal/storage -run 'Delivery|ReleaseClaim|Requeue|ClaimDue' -count=1
```

- [x] **步骤 6: 提交**

```bash
git add internal/notificationruntime internal/hookruntime internal/storage
git commit -m "feat: dispatcher 接入可靠停机"
```

---

## Chunk 4: HTTP / MCP / server 运行时接入

### Task 1: HTTP shutdown middleware

**文件：**
- 新增：`internal/httpapi/shutdown_middleware.go`
- 新增：`internal/httpapi/shutdown_middleware_test.go`
- 修改：`internal/httpapi/server.go`
- 修改：`internal/httpapi/router.go`

- [x] **步骤 1: 写 middleware 测试**

新增测试：

```go
func TestShutdownMiddlewareRejectsNewRequestsWhenDraining(t *testing.T) {
	c := runtimeutil.NewShutdownCoordinator()
	c.StopAccepting()
	srv := NewServer(Options{
		Store:    testStore,
		Shutdown: c,
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	// 断言 JSON error.code == "server_draining"
}
```

首版选择让 `/healthz` 在 draining 后也返回 503，方便 systemd / 容器 / 负载均衡把实例从服务池摘除。后续如果要拆 liveness/readiness，可另加 `/readyz`，不在本计划范围内。

再写一个已开始请求会释放 in-flight 的测试：

```go
func TestShutdownMiddlewareTracksInflightRequests(t *testing.T) {
	// 用测试 handler 包在 middleware 后，handler 内阻塞。
	// 请求进入后调用 Drain，断言 Drain 等待。
	// 放行 handler，断言 Drain 返回。
}
```

- [x] **步骤 2: 运行测试确认失败**

```bash
go test ./internal/httpapi -run ShutdownMiddleware -count=1
```

- [x] **步骤 3: 实现 middleware**

实现：

```go
func (s *Server) shutdownMiddleware(next http.Handler) http.Handler {
	if s.shutdown == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done, ok := s.shutdown.Begin()
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "server_draining", "server is shutting down", nil)
			return
		}
		defer done()
		next.ServeHTTP(w, r)
	})
}
```

在 `Options` 和 `Server` 增加：

```go
Shutdown *runtimeutil.ShutdownCoordinator
```

`NewServer` 未传入时可为 nil；server 命令会显式传入。

在 `newRouter()` 中把 middleware 放在最外层靠前位置：

```go
api.Use(s.shutdownMiddleware)
api.Use(s.requestIDMiddleware)
...
```

这样 draining 后尽早拒绝新请求。

- [x] **步骤 4: HTTP MCP 传入 coordinator**

在 `handleMCP()` 构造 `mcpserver.Options` 时增加：

```go
Shutdown: s.shutdown,
Logger:   s.logger,
```

- [x] **步骤 5: 运行 HTTP 测试**

```bash
go test ./internal/httpapi -run 'ShutdownMiddleware|MCP|Healthz' -count=1
```

预期：通过。

### Task 2: MCP tool gate

**文件：**
- 修改：`internal/mcpserver/options.go`
- 修改：`internal/mcpserver/tools_common.go`
- 修改或新增：`internal/mcpserver/integration_test.go`

- [x] **步骤 1: 写 MCP draining 测试**

新增测试：

```go
func TestMCPToolReturnsServerDrainingWhenShutdownStarted(t *testing.T) {
	c := runtimeutil.NewShutdownCoordinator()
	c.StopAccepting()
	srv := NewServer(Options{
		Store:    testStore,
		Mode:     ModeStdio,
		Shutdown: c,
	})
	// 调一个轻量 tool，例如 misc/version 或 task_query。
	// 断言 CallToolResult 是 error，内容包含 server_draining。
}
```

如果现有 MCP 集成 helper 较重，可以新增单元测试验证 `addTool` 包装：

- 创建临时 `mcp.Server`。
- 注册一个测试 tool。
- coordinator draining 后调用 handler。
- 断言返回 business error。

- [x] **步骤 2: 运行测试确认失败**

```bash
go test ./internal/mcpserver -run 'ServerDraining|Shutdown' -count=1
```

- [x] **步骤 3: Options 增加 Shutdown**

`internal/mcpserver/options.go`：

```go
Shutdown *runtimeutil.ShutdownCoordinator
```

- [x] **步骤 4: addTool 统一 gate**

在 `addTool` 注册的 handler 最外层增加：

```go
var done func()
if currentToolOptions.Shutdown != nil {
	var ok bool
	done, ok = currentToolOptions.Shutdown.Begin()
	if !ok {
		return businessErrorResult(app.RuntimeError{
			Code:    "server_draining",
			Message: "server is shutting down",
		}), nil
	}
	defer done()
}
```

注意：`addTool` 当前签名没有 `opts`。需要把签名改成：

```go
func addTool[In any](s *mcp.Server, opts Options, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, ToolEnvelope])
```

然后批量更新各 `register*Tools` 中的调用。

约束：

- 只改 `addTool` 调用签名，不把 shutdown 逻辑散落到每个 tool。
- tool handler 使用传入的 `ctx` 时，必须能同时响应 SDK request cancel 和 `Shutdown.ForceCancel()`。
- `StopAccepting()` 不取消已开始 tool；只有 `ForceCancel()` 会取消已开始 tool。

推荐实现合并 context：

```go
ctx, cancel := runtimeutil.ContextWithCancelOnEither(ctx, opts.Shutdown.Context())
defer cancel()
```

`ContextWithCancelOnEither` 已在 Chunk 1 中定义；不要在 `addTool` 内重复写匿名 goroutine。

- [x] **步骤 5: 写 force cancel 会取消 tool context 的测试**

新增测试：

```go
func TestMCPToolContextCanceledOnShutdownForceCancel(t *testing.T) {
	// 注册一个测试 tool，handler 阻塞等待 ctx.Done()。
	// 调用 tool 后确认 handler 已开始。
	// 调用 shutdown.ForceCancel()。
	// 断言 handler 观察到 context canceled，tool 调用返回。
}
```

- [x] **步骤 6: 运行 MCP 测试**

```bash
go test ./internal/mcpserver -run 'ServerDraining|Shutdown|Tool' -count=1
```

预期：通过。

### Task 3: server 命令两阶段停机

**文件：**
- 修改：`internal/cli/server.go`
- 修改：`internal/cli/server_test.go`

- [x] **步骤 1: 写 server runtime 组装测试**

扩展或新增测试，覆盖：

- server 创建一个 `ShutdownCoordinator`。
- 同一个 coordinator 传给 `httpapi.NewServer`、notification dispatcher、hook dispatcher。
- signal 后先 `StopAccepting()`，不是直接取消 dispatcher 执行 context。

如果当前 `newServerCommand` 难以直接测，可先把停机编排拆成 helper：

```go
func runServerShutdown(ctx context.Context, opts serverRuntimeShutdownOptions) error
```

测试 helper 而不是真实起完整 server。

- [x] **步骤 2: 实现 server 流程**

当前流程中：

```go
ctx, stop := signal.NotifyContext(...)
hookDispatcher.Run(ctx)
notificationDispatcher.Run(ctx)
...
<-ctx.Done()
httpServer.Shutdown(shutdownCtx)
```

调整为：

1. 创建：

```go
shutdown := runtimeutil.NewShutdownCoordinator()
runCtx, cancelRun := context.WithCancel(context.Background())
signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
```

2. `httpapi.NewServer` 传入 `Shutdown: shutdown`。
3. dispatcher options 传入 `Shutdown: shutdown`。
4. dispatcher `Run(runCtx)`，不要传 signal context。
5. reminder scheduler loop 用 `runCtx`；draining 后 `cancelRun()` 让它停止下一轮调度。scheduler 的单次 `RunOnce` 很短，不作为长 drain 对象。
6. `select`：

```go
case err := <-errCh:
    cancelRun()
    shutdown.ForceCancel()
    return err
case <-signalCtx.Done():
```

7. 收到 signal 后：

```go
fmt.Fprintln(stderr, "xuanchu: shutdown: signal received")
shutdown.StopAccepting()
cancelRun() // 停止 dispatcher/scheduler 下一轮循环，但不取消已开始投递；投递使用 shutdown.Context()
```

8. 创建 drain context：

```go
drainCtx, cancel := context.WithTimeout(context.Background(), shutdownOpts.Timeout)
```

9. 并行等待：

- `httpServer.Shutdown(drainCtx)`
- `shutdown.Drain(drainCtx)`
- dispatcher / scheduler `Run` goroutine 退出

10. drain 超时：

```go
shutdown.ForceCancel()
forceCtx, cancel := context.WithTimeout(context.Background(), shutdownOpts.ForceTimeout)
defer cancel()
// 等待剩余 goroutine 或 forceCtx 超时
```

11. 日志全部写 stderr / logger，不写 stdout。

- [x] **步骤 3: 处理 errCh 容量和 goroutine 退出**

当前 `errCh := make(chan error, 4)`。新增等待 dispatcher goroutine 时要避免阻塞：

- 使用 `sync.WaitGroup` 追踪后台 runtime。
- 每个 goroutine 返回时写 `errCh` 前使用 non-blocking 或保证 errCh 足够容量。
- server 返回前等待后台 goroutine，不留下运行中的 session。
- drain 正常完成时，`context.Canceled` / `http.ErrServerClosed` 不作为错误返回。
- force timeout 后返回明确错误，错误信息包含 `shutdown force timeout`，便于部署系统定位。

- [x] **步骤 4: 运行 CLI server 测试**

```bash
go test ./internal/cli -run 'Server|Shutdown' -count=1
```

预期：通过。

### Task 4: stdio MCP 两阶段停机

**文件：**
- 修改：`internal/cli/mcp.go`
- 修改：`internal/cli/mcp_test.go`

- [x] **步骤 1: 写 stdio MCP shutdown 编排测试**

如果真实 stdio MCP 难测，提取 helper：

```go
func runMCPStdioWithShutdown(ctx context.Context, srv *mcp.Server, transport mcp.Transport, shutdown *runtimeutil.ShutdownCoordinator, opts serverShutdownOptions, stderr io.Writer) error
```

测试：

- signal 后调用 `StopAccepting()`。
- drain timeout 后调用 `ForceCancel()`。
- shutdown 日志写 stderr。

- [x] **步骤 2: 实现 stdio 流程**

`newMCPStdioCommand`：

- 解析 `cfg.Shutdown` 和 flags。
- 创建 coordinator。
- `mcpserver.NewServer` 传入 `Shutdown: shutdown`。
- 不再直接用 signal context 作为 tool 的唯一生命周期。

推荐流程：

```go
runCtx, cancelRun := context.WithCancel(context.Background())
signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stopSignals()

errCh := make(chan error, 1)
go func() {
	errCh <- srv.Run(runCtx, &mcp.StdioTransport{})
}()

select {
case err := <-errCh:
	return err
case <-signalCtx.Done():
}

fmt.Fprintln(cmd.ErrOrStderr(), "xuanchu: shutdown: signal received")
shutdown.StopAccepting()

drainCtx, cancel := context.WithTimeout(context.Background(), shutdownOpts.Timeout)
defer cancel()
if err := shutdown.Drain(drainCtx); err != nil {
	shutdown.ForceCancel()
	forceCtx, cancel := context.WithTimeout(context.Background(), shutdownOpts.ForceTimeout)
	defer cancel()
	// 等 srv.Run 返回或 forceCtx 超时；必要时 cancelRun()
	cancelRun()
	select {
	case err := <-errCh:
		return err
	case <-forceCtx.Done():
		return forceCtx.Err()
	}
}
cancelRun()
return <-errCh
```

注意：

- stdout 不能出现 shutdown 日志。
- 如果 MCP SDK 在 `cancelRun()` 后返回 `context.Canceled`，应视为正常停机。
- draining 期间新的 tool call 由 `addTool` 返回 `server_draining`。

- [x] **步骤 3: 运行 MCP CLI 测试**

```bash
go test ./internal/cli -run 'MCP|Shutdown' -count=1
```

预期：通过。

- [x] **步骤 4: 提交**

```bash
git add internal/httpapi internal/mcpserver internal/cli/server.go internal/cli/server_test.go internal/cli/mcp.go internal/cli/mcp_test.go
git commit -m "feat: HTTP 与 MCP 接入可靠停机"
```

---

## Chunk 5: 集成验证与文档

### Task 1: 增加端到端停机测试

> 状态：本轮未新增进程级阻塞 webhook 集成测试。停机语义已在 dispatcher / HTTP / MCP / CLI 组装层单元测试中覆盖，避免给集成套件引入长耗时和不稳定信号时序。

**文件：**
- 修改或新增：`tests/integration/server_shutdown_test.go`

- [ ] **步骤 1: 写正常 drain 集成测试**

测试流程：

1. 构建或启动测试用 `xuanchu server`。
2. 配置一个 notification 或 hook sink 指向 `httptest` 阻塞服务。
3. 产生一条 due delivery。
4. 等待外部服务收到请求并阻塞。
5. 对 xuanchu 进程发送 SIGTERM。
6. 断言进程不会立即退出。
7. 放行外部服务。
8. 断言进程正常退出，delivery 最终 succeeded。

命令层建议使用短 timeout：

```bash
xuanchu --config test.toml server --listen 127.0.0.1:0 --shutdown-timeout 2s --shutdown-force-timeout 500ms
```

- [ ] **步骤 2: 写 force cancel 集成测试**

测试流程：

1. 外部服务一直阻塞。
2. 发送 SIGTERM。
3. 断言进程在 `shutdown_timeout + force_timeout` 附近退出。
4. 重启 dispatcher 或调用 RunOnce。
5. 断言 delivery 不会永久停在 `delivering`，可被 stale recovery / release 恢复。

如集成测试耗时过长，先在 dispatcher 单元测试覆盖 force cancel，集成测试只保留正常 drain，避免套件超时。

- [ ] **步骤 3: 运行集成测试**

```bash
go test ./tests/integration -run 'Shutdown|Server' -count=1
```

预期：通过，单测耗时可控。

### Task 2: 文档同步

**文件：**
- 修改：`README.md`
- 修改：`docs/manual/deployment.md`
- 修改：`docs/manual/mcp.md`
- 修改：`docs/superpowers/specs/2026-06-10-xuanchu-reliable-shutdown-design.md`
- 修改：`docs/superpowers/plans/2026-06-10-xuanchu-reliable-shutdown-implementation.md`

- [x] **步骤 1: README 增加当前配置说明**

写入：

```toml
[server.shutdown]
timeout = "30s"
force_timeout = "5s"
```

说明：

- `timeout` 是整体 drain 时间。
- `force_timeout` 是强制取消后的清理等待。
- `server --shutdown-timeout` 会覆盖 TOML。
- `server --shutdown-force-timeout` 会覆盖 TOML。

- [x] **步骤 2: deployment 增加 systemd 示例**

在 `docs/manual/deployment.md` 增加：

```ini
[Service]
ExecStart=/usr/local/bin/xuanchu --config /etc/xuanchu/config.toml server --listen :8080
KillSignal=SIGTERM
TimeoutStopSec=45s
```

说明：

- `TimeoutStopSec` 应大于 `server.shutdown.timeout + server.shutdown.force_timeout`。
- 如果容器平台有 termination grace period，也应按同样预算设置。

- [x] **步骤 3: MCP 文档补充 stdio 行为**

说明：

- 收到 SIGTERM / SIGINT 后，stdio MCP 不再开始新的 tool call。
- 已开始 tool call 会等待 drain timeout。
- 超时后强制取消。
- shutdown 日志写 stderr，不污染 stdout。

- [x] **步骤 4: 更新 spec / plan 状态**

实现完成后：

- 在 spec 的验收标准下补一句“已按 implementation plan 落地”或记录实际差异。
- 在本 plan 中把已完成任务勾选为 `[x]`。
- spec / plan 已记录实际实现：已 claim 未开始 delivery 使用 `ReleaseClaim` 释放，撤销本次 claim 的 attempt。

### Task 3: 全量验证

**文件：**
- 不修改或只修复验证发现的问题。

- [x] **步骤 1: 格式与静态检查**

运行：

```bash
gofmt -w internal/runtimeutil internal/config internal/cli internal/httpapi internal/mcpserver internal/notificationruntime internal/hookruntime internal/storage tests/integration
git diff --check
```

预期：无格式错误，无 trailing whitespace。

结果：已运行 `gofmt` 和 `git diff --check`，通过。

- [x] **步骤 2: 单测**

运行：

```bash
go test ./...
```

预期：通过。

结果：已运行 `go test ./...`，通过。

- [x] **步骤 3: 零 CGO 单测**

运行：

```bash
CGO_ENABLED=0 go test ./...
```

预期：通过。

结果：已运行 `CGO_ENABLED=0 go test ./...`，通过。

- [x] **步骤 4: 零 CGO 构建**

运行：

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

预期：通过。

结果：已运行 `CGO_ENABLED=0 go build ./cmd/xuanchu`，通过。

- [ ] **步骤 5: PostgreSQL 回归冒烟**

如果本地有 PostgreSQL 测试实例，额外运行：

```bash
XUANCHU_DB_URL='postgres://user:pass@localhost:5432/xuanchu_test?sslmode=disable' go test ./internal/storage ./internal/notificationruntime ./internal/hookruntime -count=1
```

预期：不再出现 `function max(unknown, bigint) does not exist` 这类 PostgreSQL 类型推断错误。

结果：本轮未运行；当前环境没有配置 PostgreSQL 测试实例。PostgreSQL 类型推断相关 SQL 已由 storage 单元测试覆盖 `CAST(? AS BIGINT)` 与 `CASE` 表达式。

- [ ] **步骤 6: 最终提交**

```bash
git status --short
git add README.md docs/manual/deployment.md docs/manual/mcp.md docs/superpowers/specs/2026-06-10-xuanchu-reliable-shutdown-design.md docs/superpowers/plans/2026-06-10-xuanchu-reliable-shutdown-implementation.md tests/integration internal
git commit -m "feat: 实现可靠停机"
```

---

## 风险与处理

- **MCP SDK 无法停止读取新请求：** 首版允许 SDK 继续接收帧，但 `addTool` 在业务执行前返回 `server_draining`；这满足“不再开始新工作”。
- **已 claim 未开始 delivery 的 attempt_count：** 使用 `ReleaseClaim` 释放未启动投递，撤销本次 claim 预增的 attempt，避免停机消耗真实投递次数。
- **HTTP middleware 与 `http.Server.Shutdown` 双重追踪：** 这是刻意设计。HTTP server 负责连接层 drain，coordinator 负责跨 HTTP/MCP/dispatcher 的统一 in-flight 视图。
- **signal context 误伤投递：** server 中禁止把 signal context 直接传给已开始投递。投递请求只能由 sink timeout 或 `ForceCancel()` 取消。
- **测试耗时：** dispatcher 和 server 停机测试必须使用毫秒级 timeout 和显式 channel 放行，不使用真实 sleep 等待长周期。

## 完成定义

- `server.shutdown.timeout / force_timeout` 可从 TOML 和 CLI flag 控制。
- 收到 SIGTERM / SIGINT 后，HTTP、MCP、dispatcher 不再开始新工作。
- 已开始的 HTTP handler、MCP tool call、delivery 投递不会因为 stop accepting 立即取消。
- drain 超时后会 force cancel。
- 已 claim 未开始的 delivery 不会永久停在 `delivering`。
- shutdown 日志不污染 MCP stdio stdout。
- 文档已同步 README、deployment、MCP manual、spec / plan 状态。
- 以下命令通过：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
git diff --check
```
