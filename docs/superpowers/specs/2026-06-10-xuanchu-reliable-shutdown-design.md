# Xuanchu 可靠停机设计

## 背景

`xuanchu server` 现在已经具备 notification / hook dispatcher 的可靠投递队列：delivery 先落库，dispatcher claim 后再投递，进程异常退出时可通过 stale recovery 恢复 `delivering`。这能保证“不永久丢消息”，但还不能保证“服务关闭时不主动打断已经开始的工作”。

生产环境中，systemd、容器平台或部署脚本会通过 SIGTERM 关闭进程。如果进程收到信号后立刻取消 root context，正在进行的 HTTP 请求、MCP tool call、外部投递请求都可能被中断。对投递服务来说，这会增加重复通知、半成功状态和重启后的恢复等待；对 HTTP / MCP 来说，会让调用方看到不必要的连接中断。

因此需要把当前“可恢复停机”升级为“可靠停机”：一般情况下先停止接收新工作，再等待已开始工作完成；只有超过停机预算时才强制取消，交给持久化队列和事务语义兜底。

## 目标

- HTTP API、HTTP MCP、stdio MCP、notification dispatcher、hook dispatcher 都支持可靠停机。
- 收到 SIGTERM / SIGINT 后，不再开始新工作。
- 已经开始的 HTTP handler、MCP tool call、delivery 投递请求默认允许继续执行。
- 停机等待受超时控制，不能无限阻塞进程退出。
- 超时后仍允许强制取消；delivery 依赖 DB 队列恢复，HTTP / MCP 返回取消或连接关闭。
- 关闭过程有清晰日志，便于 systemd / 容器平台排查。
- 保持当前零外部队列架构，不引入 Redis、NATS、Kafka。

## 非目标

- 不实现跨进程 / 跨实例全局 drain 协调。
- 不保证 exactly-once。投递仍是 at-least-once，接收方应按 `delivery_id` 幂等。
- 不要求进程在强制 kill、机器断电、OOM kill 下优雅完成；这些情况仍由 DB recovery 兜底。
- 不在本规格中实现热重载或无损滚动发布协议。

## 术语

- **入口关闭**：停止接收新工作。例如 HTTP server 不再 accept 新连接，dispatcher 不再 claim 新 delivery。
- **in-flight 工作**：已经进入业务处理的工作。例如一个 HTTP handler、一个 MCP tool call、一个已开始发送的 webhook 请求。
- **drain**：入口关闭后，等待 in-flight 工作自然结束。
- **强制取消**：drain 超时后取消剩余工作。
- **可靠恢复**：强制取消或进程异常退出后，依靠 DB 状态恢复未完成 delivery。

## 当前问题

### HTTP server

当前 `http.Server.Shutdown(ctx)` 本身具备 graceful shutdown 能力：它会停止 accept 新连接，并等待活跃请求结束，直到 `shutdown-timeout` 超时。

问题在于 server 的 root context 也会传给后台 dispatcher。如果 root context 在收到信号后立即取消，dispatcher 里的投递请求会被主动取消，和 HTTP server 的 graceful shutdown 语义不一致。

### MCP

stdio MCP 当前使用 signal context 直接运行：

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
return srv.Run(ctx, &mcp.StdioTransport{})
```

这可以让进程退出，但不区分“停止接收新的 MCP 请求”和“等待已有 tool call 完成”。如果 SDK 在 context cancel 时直接终止 active session，正在执行的 tool call 可能被中断。

HTTP MCP 挂在 HTTP server 上，入口层可以继承 `http.Server.Shutdown`，但 tool call 内部还需要统一 in-flight 追踪，否则无法明确知道是否 drain 完成。

### dispatcher

notification / hook dispatcher 已经有有界并发和 DB 恢复，但还缺少两阶段停机：

- 停机时应该先停止新一轮 claim。
- 已经开始的 delivery 投递不应立即被 root context 取消。
- 已经 claim 但尚未开始执行的 delivery 应尽量立即 requeue，减少等待 `claim_ttl`。
- 超时后才取消仍未完成的投递，让 stale recovery 兜底。

## 设计原则

### 1. 入口 context 与执行 context 分离

每类运行时都需要区分两个信号：

- `stopAccepting`：不再接收新工作。
- `forceCancel`：drain 超时后取消剩余工作。

收到 SIGTERM / SIGINT 后，先触发 `stopAccepting`。只有超过 `shutdown_timeout` 后，才触发 `forceCancel`。

### 2. 新工作必须被明确拒绝或不再领取

- HTTP：`http.Server.Shutdown` 停止 accept 新连接。
- HTTP MCP：随 HTTP server 停止接收新请求。
- stdio MCP：停止读取或停止处理新的 JSON-RPC 请求，具体取决于 MCP SDK 暴露能力。
- dispatcher：停止进入下一轮 `RunOnce`，也不再 claim 新 delivery。

### 3. 已开始工作默认继续

已开始的投递请求、HTTP handler、MCP tool call 默认不因 SIGTERM 立即取消。它们可以继续使用原本的 request timeout、sink timeout、tool 自身逻辑完成。

### 4. 超时后允许强制取消

如果超过停机预算，进程应取消剩余工作并退出。对 delivery 来说，这是可靠恢复路径，不是数据丢失路径。

### 5. 持久化队列优先于内存队列

dispatcher 停机过程中不能把大量 delivery 保留在内存中等待。已 claim 但尚未开始的 delivery 应回写为 `queued`，避免只能等 `claim_ttl`。

## 配置设计

首版推荐使用一个全局配置，避免配置过多：

```toml
[server.shutdown]
timeout = "30s"
force_timeout = "5s"
```

含义：

- `timeout`：收到 signal 后最多等待 in-flight 工作完成的时间。
- `force_timeout`：发出强制取消后，再等待 runtime 清理资源的时间。超过后返回错误或退出。

CLI flag：

```bash
xuanchu server --shutdown-timeout 30s --shutdown-force-timeout 5s
xuanchu mcp stdio --shutdown-timeout 30s --shutdown-force-timeout 5s
```

兼容说明：

- 现有 `xuanchu server --shutdown-timeout` 保留，语义从“HTTP server shutdown timeout”升级为“server 整体 drain timeout”。
- HTTP server 内部可以继续使用同一个 timeout。
- 后续如有需要，再拆为：
  - `server.http.shutdown_timeout`
  - `mcp.shutdown.timeout`
  - `notifications.dispatcher.shutdown_timeout`
  - `hooks.dispatcher.shutdown_timeout`

## 运行时模型

### ShutdownCoordinator

新增一个小型运行时协调器，负责统一追踪 in-flight 工作：

```go
type ShutdownCoordinator struct {
    // 状态：running -> draining -> forced -> stopped
}

func (c *ShutdownCoordinator) Begin() (done func(), ok bool)
func (c *ShutdownCoordinator) Drain(ctx context.Context) error
func (c *ShutdownCoordinator) StopAccepting()
func (c *ShutdownCoordinator) ForceCancel()
func (c *ShutdownCoordinator) Context() context.Context
```

语义：

- `Begin()` 在开始一个新工作前调用。
- 如果处于 draining，`Begin()` 返回 `ok=false`，调用方不得开始新工作。
- 已经返回 `ok=true` 的工作必须 defer `done()`。
- `Drain(ctx)` 等待所有已开始工作完成。
- `ForceCancel()` 取消 coordinator context，供强制超时使用。

实现可以放在 `internal/runtimeutil/shutdown.go`，保持和 dispatcher 并发工具同层。

### HTTP API / HTTP MCP

HTTP server 停机流程：

1. 收到 signal。
2. 调用 `coordinator.StopAccepting()`。
3. 调用 `httpServer.Shutdown(shutdownCtx)`，停止 accept 并等待 active HTTP request。
4. 并行等待后台 dispatcher drain。
5. 如果 `shutdownCtx` 超时，调用 `coordinator.ForceCancel()`。
6. 等待 `force_timeout`，然后返回。

HTTP handler 层是否需要显式 `Begin()` 有两种选择：

- 方案 A：只依赖 `http.Server.Shutdown` 追踪 HTTP request。
- 方案 B：中间件对每个 HTTP request 调用 `Begin()`，让 HTTP、MCP、dispatcher 使用同一套 drain 计数。

推荐方案 B，因为 HTTP MCP tool call 可能跨 SDK handler 内部异步执行，统一 in-flight 计数更直观。实现时应保证健康检查、静态错误响应等极短请求也能正确释放 `done()`。

### stdio MCP

stdio MCP 没有 HTTP server 的 accept/drain 语义，需要单独处理。

目标行为：

- 收到 SIGTERM 后，不再开始新的 tool call。
- 已经开始的 tool call 继续运行，直到完成或 drain timeout。
- 超时后 cancel tool call context。
- stdout 不输出日志，所有 shutdown 日志走 stderr。

实现取决于 MCP Go SDK 能力：

- 如果 SDK 支持 server/tool middleware，在 tool handler 外层调用 `Begin()`。
- 如果 SDK 不支持全局 middleware，则在 `serviceForTool` 或统一 tool wrapper 中调用 `Begin()`。
- 如果 SDK 无法停止读取新请求但可以让新 tool call 返回错误，则 draining 后新的 tool call 返回稳定 MCP business error：
  - code: `server_draining`
  - message: `server is shutting down`

### dispatcher

dispatcher 需要显式支持 `StopAccepting` 和 `Drain`：

```go
type Dispatcher struct {
    // ...
}

func (d *Dispatcher) Run(ctx context.Context) error
func (d *Dispatcher) StopAccepting()
func (d *Dispatcher) Drain(ctx context.Context) error
func (d *Dispatcher) ForceCancel()
```

也可以不暴露完整接口，而是在 `DispatcherOptions` 中传入 `ShutdownCoordinator`。

推荐方式：传入 coordinator，减少 runtime 各自发明状态机。

dispatcher 行为：

1. `Run` 循环每轮开始前检查 coordinator 是否 accepting。
2. draining 后不再调用 `ClaimDue`。
3. `RunOnce` claim 后，对每个 delivery 启动 worker 前调用 `Begin()`。
4. 如果 `Begin()` 返回 false，说明已经 draining：
   - 该 delivery 尚未开始投递，应立即 `Requeue`。
   - 不再继续启动后续 delivery。
5. 已经开始投递的 delivery 使用非 signal-root context，允许自然完成。
6. `ForceCancel()` 后，投递 request context 被取消。

注意：

- `SinkLimiter` token 必须在 worker 结束时释放，即使 force cancel。
- 已 claim 但尚未开始投递的 delivery 使用 `ReleaseClaim` 释放，撤销 claim 时预增的 `attempt_count`，避免停机消耗真实投递次数。
- 如果进程在 requeue 前崩溃，stale recovery 仍可恢复。

## 状态流

```text
running
  └─ receive SIGTERM/SIGINT
draining
  ├─ stop HTTP accept
  ├─ stop MCP new tool calls
  ├─ stop dispatcher new claim
  ├─ wait in-flight
  └─ all done -> stopped
forced
  ├─ timeout reached
  ├─ cancel remaining work
  ├─ rely on DB recovery for delivery
  └─ stopped / exit with error if cleanup also times out
```

## 错误与日志

server stderr / log 应至少包含：

- `xuanchu: shutdown: signal received`
- `xuanchu: shutdown: draining started timeout=30s`
- `xuanchu: shutdown: http server drained`
- `xuanchu: shutdown: notification dispatcher drained`
- `xuanchu: shutdown: hook dispatcher drained`
- `xuanchu: shutdown: mcp drained`
- `xuanchu: shutdown: force cancel timeout exceeded`

HTTP 新请求在 draining 后通常不会进入，因为 listener 已关闭。若进入中间件，可返回：

```json
{
  "error": {
    "code": "server_draining",
    "message": "server is shutting down"
  }
}
```

MCP 新 tool call 在 draining 后返回同样业务错误码 `server_draining`。

## systemd 建议

文档应建议：

```ini
[Service]
ExecStart=/usr/local/bin/xuanchu --config /etc/xuanchu/config.toml server --listen :8080
TimeoutStopSec=45s
KillSignal=SIGTERM
```

`TimeoutStopSec` 应大于 `server.shutdown.timeout + server.shutdown.force_timeout`，否则 systemd 可能在 xuanchu 自己 drain 完成前发送 SIGKILL。

## 测试要求

### runtimeutil

- `Begin()` 在 running 时成功，在 draining 后失败。
- `Drain()` 等待已开始工作。
- `ForceCancel()` 取消 context。
- `Drain()` 超时返回 context error。

### dispatcher

notification 和 hook 都需要覆盖：

- draining 后不再 claim 新 delivery。
- 已经开始的投递不会因 stop accepting 立即取消。
- force cancel 后正在投递的 request 被取消。
- 已 claim 但未开始的 delivery 会 requeue。
- shutdown 时不会泄漏 worker，也不会泄漏 sink limiter token。

### HTTP server

- 收到 signal 后不再 accept 新请求。
- 已经进入 handler 的请求在 `shutdown_timeout` 内允许完成。
- 超时后 request context 被取消。
- server 返回前等待 dispatcher drain。

### MCP

- stdio MCP 收到 signal 后，已开始 tool call 允许完成。
- draining 后新的 tool call 返回 `server_draining`。
- 超时后 tool context 被取消。
- stdout 不出现 shutdown 日志。

### 集成测试

- 启动 `xuanchu server`，构造一个阻塞的 hook / notification delivery 投递。
- 发送 SIGTERM。
- 验证进程不会立即退出。
- 放行外部 webhook 后，验证进程正常退出且 delivery succeeded。
- 再测超时路径：不放行 webhook，验证进程在 timeout 后退出，delivery 后续可恢复。

## 验收标准

- `go test ./...` 通过。
- `CGO_ENABLED=0 go test ./...` 通过。
- `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。
- systemd / 容器 SIGTERM 默认不会立即中断已经开始的投递。
- 强制超时后没有 delivery 永久卡在 `delivering`。
- MCP stdio 不污染 stdout。

## 落地状态

当前实现按 implementation plan 落地到 `xuanchu server`、HTTP API、HTTP MCP、stdio MCP、notification dispatcher 和 hook dispatcher。HTTP / MCP 新工作在 draining 后返回 `server_draining`，dispatcher draining 后不再 claim，已领取但尚未开始的 delivery 使用 `ReleaseClaim` 回到队列，并撤销本次 claim 预增的 `attempt_count`。

实际差异：

- 端到端进程级停机行为主要由运行时单元测试和现有 integration 套件覆盖，本轮没有新增耗时的外部进程级阻塞 webhook 集成测试。

## 后续扩展

- 按 runtime 拆分 shutdown timeout。
- 暴露 `/api/v1/admin/shutdown-status` 查看 draining 状态。
- 为多实例部署增加 coordinator lease，但这不属于当前版本范围。
- 后续如需要更强的“停止接收新工作”定义，可把 dispatcher 的 claim 与 worker 启动边界继续细化为显式 pending 状态；当前实现把 `Begin()` 成功后的 delivery 视为 in-flight，drain 会等待或释放。
