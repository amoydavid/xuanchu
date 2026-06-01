# taskg M7 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 让企业 Agent 能通过 MCP 以结构化方式使用 `taskg`，并把 M6 留下的远程管理命令补齐。M7 必须继续保持一个二进制、同一套 app service、同一套 actor/workspace/project 权限边界；MCP 不应成为第二套业务实现。

**范围策略：** M7 聚焦 MCP Server、MCP tool schema、Agent 可读上下文、远程管理命令收口，以及支撑这些能力所需的 Go / SDK 技术选型升级。不做外部系统 adapter、不做 op-log 同步、不做 Hook / trigger 引擎、不做复杂 Agent 编排平台。

**需求来源：** 本规格从 [README.md](/Users/mac/code/projects/dajee/task/README.md)、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)、[M6 设计规格](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-31-taskg-m6-design.md)、M6 当前实现，以及本轮关于“HTTP 和未来 MCP server 选型，避免重复造轮子”的讨论中收束。

**外部技术依据：**

- 官方 MCP Go SDK：`github.com/modelcontextprotocol/go-sdk`，用于 MCP server/client、stdio transport、Streamable HTTP handler、tool schema 和 JSON-RPC 协议层。
- 官方 MCP Go SDK README：<https://github.com/modelcontextprotocol/go-sdk>
- 官方 MCP 规范：<https://modelcontextprotocol.io/specification/>

---

## 1. 当前基础

M6 已完成并合并。当前项目已有：

- 单一 `taskg` 二进制。
- Cobra 本地 CLI 和远程 CLI。
- HTTP/JSON API 服务端，基于 `net/http` + `github.com/go-chi/chi/v5`。
- OpenAPI 3 文档。
- PAT / Agent token。
- token capability、workspace scope、project scope。
- 多 user、多 workspace、membership、权限和 audit。
- `workspace` 作为企业 / 租户级隔离边界。
- `project` 是 workspace 内一等实体，权限和 token scope 以稳定 `project_id` 为准。
- HTTP handler 已通过 `internal/app` 做请求级 scope 授权。
- 远程 CLI 已覆盖核心 task/report/project/context/config/helper/token/import/export/audit 命令。

M7 不重写 M6 HTTP API，不重做 token 模型，不改变 SQLite driver。M7 的工作是：

- 先把 M6 遗留的远程管理命令接到已有 HTTP endpoint。
- 再把 MCP 作为新的协议入口接入同一套 app service。
- 把 Go / MCP SDK 选型写清楚，避免手写 MCP 协议层。

## 2. 核心产品决策

### 2.1 M7 接受升级 Go 版本

M7 将 Go 版本从 `1.22` 升级到 `1.25`。

理由：

- 官方 MCP Go SDK 已发布版本要求更高 Go 版本；继续停留在 Go 1.22 会迫使项目手写 MCP JSON-RPC / Streamable HTTP / schema 细节。
- MCP 是协议型能力，重复造轮子的风险高于升级 Go 的成本。
- 当前开发环境已经具备 Go 1.24+，升级链路可验证。

规则：

- M7 plan 的 Phase 0 必须先升级 `go.mod`、文档和 CI/验收说明中的 Go 版本。
- 升级 Go 不改变 SQLite driver。仍使用 `GORM + github.com/glebarez/sqlite`，继续保持 `CGO_ENABLED=0` 验收。
- 不引入 `gorm.io/driver/sqlite`。
- 不引入 `github.com/mattn/go-sqlite3`。
- Go 升级必须单独验证，不能和 MCP 大量代码混在同一个不可拆分提交里。

### 2.2 HTTP REST 层继续使用 `net/http` + `chi`

M7 不更换 HTTP 框架。

理由：

- M6 已经使用 `chi` 跑通 REST API、middleware、错误结构、OpenAPI 和集成测试。
- REST API 的问题不是框架能力不足，而是后续协议入口要复用 app service。
- 换 Gin/Echo/Fiber 只会制造迁移成本，不会降低 MCP 协议复杂度。

规则：

- `/api/v1/*` 继续由现有 `internal/httpapi` 负责。
- `/healthz` 继续保持匿名。
- HTTP server 生命周期、timeout、access log、body limit 继续沿用 M6 语义。
- 如需新增 MCP HTTP endpoint，应挂在同一个 `http.Server` 上，而不是启动第二个端口。

### 2.3 MCP 协议层使用官方 MCP Go SDK

M7 引入：

```text
github.com/modelcontextprotocol/go-sdk/mcp
```

职责边界：

- SDK 负责 MCP JSON-RPC、stdio transport、Streamable HTTP transport、tool 注册、schema 推导、会话协议。
- `taskg` 负责 actor/workspace/project 授权、业务调用、错误码映射、审计和 rendered/data 输出。

M7 不手写：

- MCP JSON-RPC envelope。
- MCP initialize/listTools/callTool 协议。
- Streamable HTTP session 协议。
- tool input schema generator。

M7 可以手写：

- `taskg` 自己的 tool 输入 DTO。
- `taskg` 自己的 tool result DTO。
- `taskg` app service adapter。
- `taskg` 错误码到 MCP tool error 的映射。

### 2.4 MCP 是第三个入口，不是第三套业务逻辑

M7 后 `taskg` 有三类入口：

- 本地 CLI。
- HTTP/JSON API + 远程 CLI。
- MCP stdio / MCP HTTP。

三类入口必须复用：

- `internal/app` 权限和业务规则。
- `internal/storage/sqlite` repository。
- `internal/render` 的 human 输出能力。
- M6 token / request scope 语义。
- M4/M5/M6 audit 语义。

MCP handler 不允许：

- 直接拼 SQL。
- 直接绕过 app service 写任务。
- 自己实现 membership 权限。
- 自己解释 Taskwarrior 查询语法。
- 为了方便 Agent 而放宽 project scope。

### 2.5 M7 包含 M6 遗留远程管理命令收口

M6 明确把下列远程 CLI 管理命令留给 M7：

- `taskg --server ... workspace add|list|info|modify|use|archive`
- `taskg --server ... user add|list|info|use`
- `taskg --server ... member list|add|role`
- `taskg --server ... show`

M7 必须把这些命令的远程行为作为 Phase 0 收口：能安全远程化的命令必须支持；本质依赖本机身份切换的命令必须继续明确返回 `remote_unsupported_command`，并用测试证明不会触碰本地 DB。

规则：

- 已有 HTTP endpoint 能覆盖的命令直接接线。
- 当前 HTTP endpoint 不足以覆盖的命令，M7 Phase 0 必须补最小 REST endpoint，并同步 OpenAPI；不能让远程 CLI 回退到本地 SQLite。
- `workspace use` 远程模式写服务端 `(actor, workspace)` active workspace，不修改操作者本机业务状态。
- `user use` 远程模式继续不支持。HTTP token 的 actor 已由 token 决定，远程命令不应让客户端切换成另一个 user。
- 原有 `remote_unsupported_command` 集成测试要拆成两类：
  - 命令已经远程成功。
  - 仍不支持的本机语义命令不会触碰本地 DB。

### 2.6 M7 不做 OAuth/JWT 登录

M7 HTTP MCP 继续使用 M6 PAT / Agent token。

规则：

- HTTP MCP 必须要求 `Authorization: Bearer <token>`。
- token 校验复用 M6 `AuthenticateBearerToken`。
- actor、workspace scope、project scope 复用 M6 request scope。
- OAuth Protected Resource Metadata、JWT 登录、refresh token、浏览器登录都不进入 M7。
- 若官方 SDK 提供 auth middleware，可以作为参考，但不替代 taskg 当前 token 模型。

## 3. 技术选型

### 3.1 可选方案

#### 方案 A：手写 MCP 协议层

做法：

- 继续 Go 1.22。
- 自己实现 JSON-RPC。
- 自己实现 stdio line protocol。
- 自己实现 Streamable HTTP。
- 自己维护 tool schema。

优点：

- 不升级 Go。
- 依赖少。

缺点：

- 协议细节多，容易和 MCP 客户端不兼容。
- Streamable HTTP 和 session 语义容易实现偏。
- tool schema 漂移风险高。
- 后续 MCP spec 演进时维护成本高。

结论：不采用。

#### 方案 B：第三方 MCP Go SDK

做法：

- 引入社区 SDK，例如 `mcp-go`。

优点：

- 比手写更快。
- 可能支持较旧 Go 版本。

缺点：

- 官方 SDK 已可用时，社区 SDK 不应作为首选。
- 长期 spec 跟进、HTTP transport 和 schema 行为可能与官方存在差异。

结论：仅作为官方 SDK 阻塞时的备选，不作为 M7 主路线。

#### 方案 C：官方 MCP Go SDK + 升级 Go

做法：

- 升级 Go。
- 引入 `github.com/modelcontextprotocol/go-sdk/mcp`。
- SDK 承担 MCP 协议层。
- `taskg` 只写业务 adapter。

优点：

- 最大程度避免重复造轮子。
- stdio / Streamable HTTP 有官方实现。
- tool schema 和 MCP spec 演进由 SDK 承担。
- 更容易用 SDK client 做集成测试。

缺点：

- 需要升级 Go。
- 需要在 M7 初期处理依赖和文档同步。

结论：采用。

### 3.2 最终选型

M7 最终选型：

- Go：`1.25`。
- REST HTTP：继续 `net/http` + `github.com/go-chi/chi/v5`。
- MCP：`github.com/modelcontextprotocol/go-sdk/mcp`。
- SQLite：继续 `GORM + github.com/glebarez/sqlite`。
- 验收：继续包含 `CGO_ENABLED=0 go test ./...` 和 `CGO_ENABLED=0 go build ./cmd/taskg`。

### 3.3 依赖边界

允许新增：

- `github.com/modelcontextprotocol/go-sdk`。
- SDK 直接需要的纯 Go 依赖。

不允许新增：

- 需要 CGO 的 SQLite driver。
- 第二个 HTTP web framework。
- 重型 dependency injection 框架。
- 外部 Agent 编排框架。
- OAuth/JWT 登录库，除非只是 SDK 的间接依赖且 M7 不使用该能力。

## 4. 命令与运行模式

### 4.1 HTTP server 同时挂 REST 和 MCP

M7 后：

```bash
taskg server --listen :8080
```

提供：

- `GET /healthz`
- `/api/v1/*`
- `/mcp`

规则：

- `/api/v1/*` 使用现有 HTTP JSON envelope。
- `/mcp` 使用 MCP SDK 的 Streamable HTTP handler。
- `/mcp` 与 `/api/v1/*` 共享同一个 SQLite store、clock、stderr、access log 基础设施。
- `/mcp` 不进入 OpenAPI；OpenAPI 只描述 REST API。
- `/mcp` 的 body limit、panic recovery、request id、access log 应尽量复用 HTTP middleware。

### 4.2 本地 MCP stdio

新增：

```bash
taskg mcp stdio
taskg mcp stdio --db ./taskg.db
taskg mcp stdio --workspace dajee
taskg mcp stdio --project-id <uuid>
```

规则：

- stdio MCP 使用 stdin/stdout 承载 MCP JSON-RPC。
- stderr 只输出日志和错误诊断。
- stdout 不得打印 banner、warning 或启动提示，避免破坏 MCP 协议。
- 本地 migration warning 必须在 `mcp stdio` 下禁用或改到 stderr 且不得干扰 stdout。
- stdio MCP 的 actor 默认来自本地 active user。
- workspace 默认来自本地 active workspace。
- 可用 `--workspace` 覆盖 effective workspace。
- 可用 `--project-id` 或 `--project` 收窄 project scope。
- 如果本地没有 active user/workspace，返回稳定 MCP error，不自动创建匿名 actor。

### 4.3 MCP HTTP

HTTP MCP endpoint：

```text
POST /mcp
GET  /mcp  // 由 SDK transport 需要时支持
```

具体方法由官方 SDK handler 决定，M7 不手写 transport 行为。

规则：

- 必须带 Bearer token。
- 鉴权失败返回 HTTP 401。
- token 缺 capability 或 membership role 不足时，tool call 返回权限错误。
- 每个 tool call 都重新构造 app request scope。
- token 的 workspace/project allowlist 必须对 MCP 生效。
- 不允许 MCP 客户端通过 tool 参数突破 HTTP request 的 token scope。

### 4.4 MCP endpoint 路径

M7 固定使用：

```text
/mcp
```

不使用：

- `/api/v1/mcp`
- `/mcp/v1`
- `/sse`

理由：

- MCP 是协议入口，不是 REST API 子资源。
- 官方 SDK 的 Streamable HTTP client/server 示例使用独立 endpoint。
- 未来如需版本化，应优先通过 MCP protocol version 和 SDK 能力协商处理。

## 5. MCP 架构

### 5.1 包结构

新增建议：

```text
internal/mcpserver
  server.go          // 构造 MCP server，注册 tools/resources
  auth.go            // stdio/http actor 与 request scope 解析
  result.go          // data/rendered/error 映射
  tools_task.go
  tools_project.go
  tools_workspace.go
  tools_context.go
  tools_config.go
  tools_report.go
  resources.go
  schema_test.go
```

`internal/httpapi` 只负责把 `/mcp` 挂进现有 HTTP server，不承载 MCP tool 业务。

`internal/cli` 只负责新增 `mcp` 命令，打开 DB 并运行 stdio transport。

### 5.2 核心对象

建议抽象：

```go
type RuntimeFactory interface {
    ServiceForMCP(ctx context.Context, req MCPRequest, capability string, permission app.Permission, projectRef string) (*app.Service, error)
}
```

这不是必须按字面实现，但必须有等价边界：

- 输入：transport 类型、actor/token、本次 tool 的 workspace/project 参数、所需 capability / permission。
- 输出：已经授权且绑定 request scope 的 app service。

MCP tool handler 不应知道：

- token hash 如何校验。
- membership role 如何映射 permission。
- workspace/project allowlist 如何相交。
- SQLite repository 如何查询。

### 5.3 MCP server 构造

建议提供：

```go
func NewServer(opts Options) *mcp.Server
```

`Options` 至少包含：

- store。
- clock。
- version。
- stderr/logger。
- mode：`stdio` 或 `http`。
- local runtime context resolver。
- HTTP token auth resolver。

规则：

- 所有 tools 在一个注册函数中集中注册，便于 schema 测试。
- tool 名称必须稳定，不能随 CLI 命令名重构而变化。
- tool input struct 必须使用 snake_case JSON 字段。
- tool 描述应面向 Agent，说明权限、scope 和副作用。

### 5.4 MCP result 格式

所有 tool 成功结果统一为：

```json
{
  "data": {},
  "rendered": "human readable result"
}
```

规则：

- `data` 是机器可读 JSON。
- `rendered` 是人类可读文本，适合 Agent 直接展示或总结。
- MCP `TextContent` 中应包含 `rendered`。
- SDK 支持 structured output 时，`data` / `rendered` 应作为 structured output 返回。
- 对于只读列表，`data` 应包含完整数组和必要 meta；`rendered` 可是表格或摘要。
- 对于写操作，`data` 应返回变更后的实体 view；`rendered` 应简短说明结果。

### 5.5 MCP error 格式

业务错误必须保留 taskg 稳定 code：

```json
{
  "code": "project_scope_denied",
  "message": "project scope denied",
  "details": {}
}
```

规则：

- 参数解析错误：`api_bad_request` 或更具体的 app runtime code。
- 认证失败：HTTP MCP 在 handler 层返回 HTTP 401；stdio MCP 返回 MCP error。
- 权限不足：保留 `permission_denied`、`token_scope_denied`、`workspace_scope_denied`、`project_scope_denied`。
- 资源不存在：保留 `task_not_found`、`project_not_found`、`workspace_not_found`、`context_not_found`。
- tool 内部错误：返回 `api_internal` 或 `mcp_internal`，不得泄露 stack trace。
- MCP protocol error 与业务 error 分层；不要把所有 app error 都压成 JSON-RPC `InternalError`。

### 5.6 审计

MCP 写操作必须沿用 app service 现有 audit。

规则：

- `task.add`、`task.modify`、`task.done`、`task.delete`、`task.annotate`、`task.depends`、`task.start`、`task.stop` 等写操作必须写 audit。
- `context.set`、`config.set`、workspace/project 管理类写操作按现有规则写 audit。
- HTTP MCP 鉴权失败不写 audit，与 M6 鉴权失败边界一致。
- stdio MCP actor 解析失败不写 audit。
- 如果未来要记录 `mcp.tool.called` 这类 access audit，不进入 M7；M7 只要求业务写 audit。

## 6. Scope 与鉴权

### 6.1 stdio MCP scope

stdio MCP 是本地入口，默认行为接近本地 CLI：

```text
local active user + local active workspace + optional project scope
```

规则：

- actor 来自服务端 SQLite 中的 active user，而不是操作系统用户名。
- workspace 来自本地 active workspace，或命令行 `--workspace`。
- project scope 可由 `--project` 或 `--project-id` 设置。
- `--project` 必须在 effective workspace 内解析。
- 同时设置 `--project` 和 `--project-id` 时必须解析到同一个 project。
- stdio MCP 不读取远程 token。
- stdio MCP 不使用 `taskg.toml` 中的 `remote.token` 做 actor。

### 6.2 HTTP MCP scope

HTTP MCP 是远程入口，默认行为接近 HTTP API：

```text
Bearer token -> actor user -> visible workspaces -> effective workspace -> optional project scope
```

规则：

- Bearer token 必须存在。
- token 无效、过期、撤销返回 401。
- visible workspaces = membership 可见 workspace ∩ token workspace allowlist。
- project scope = tool 参数 / token project allowlist / request default 的交集。
- token project allowlist 为空表示不限制 project；非空表示 fail-closed。
- workspace allowlist 为空表示不限制 workspace，但仍受 membership 限制。
- 如果 token 可见多个 workspace 且 tool 参数中使用 project slug，必须提供 workspace 或使用 project_id。

### 6.3 tool 参数中的 scope

大多数 tool 都可以接受：

```json
{
  "workspace": "slug-or-uuid",
  "project": "slug",
  "project_id": "uuid"
}
```

规则：

- `workspace` 接受 slug 或 UUID。
- `project` 是当前 effective workspace 内的 slug。
- `project_id` 是稳定 UUID。
- `workspace` 与 `project_id` 同时出现时，project 必须属于该 workspace。
- `project` 与 `project_id` 同时出现时，必须解析到同一 project。
- project-scoped token 访问 allowlist 外任务时，单任务读取返回 not found 语义，避免泄露资源存在性。

### 6.4 capability 映射

MCP tool 到 token capability / app permission 的映射：

| Tool | Capability | App permission |
|---|---|---|
| `task.add` | `task:write` | `PermissionTaskWrite` |
| `task.modify` | `task:write` | `PermissionTaskWrite` |
| `task.done` | `task:write` | `PermissionTaskWrite` |
| `task.delete` | `task:write` | `PermissionTaskWrite` |
| `task.annotate` | `task:write` | `PermissionTaskWrite` |
| `task.depends` | `task:write` | `PermissionTaskWrite` |
| `task.start` | `task:write` | `PermissionTaskWrite` |
| `task.stop` | `task:write` | `PermissionTaskWrite` |
| `task.query` | `task:read` | `PermissionTaskRead` |
| `task.get` | `task:read` | `PermissionTaskRead` |
| `report.run` | `task:read` | `PermissionTaskRead` |
| `urgency.explain` | `task:read` | `PermissionTaskRead` |
| `workspace.list` | `workspace:read` | `PermissionWorkspaceRead` |
| `workspace.current` | `workspace:read` | `PermissionWorkspaceRead` |
| `project.list` | `project:read` | `PermissionProjectRead` |
| `project.get` | `project:read` | `PermissionProjectRead` |
| `project.current` | `project:read` | `PermissionProjectRead` |
| `context.set` | `context:write` | `PermissionContextUse` |
| `context.show` | `context:read` | `PermissionContextUse` |
| `config.get` | `config:read` | config key 对应 read permission |
| `config.set` | `config:write` | config key 对应 write permission |

如果 M6 token capability 表中缺少 `context:write`、`config:read`、`config:write` 等细分能力，M7 plan 必须先补齐 capability 定义或明确复用已有 capability。不能让 MCP tool 绕过 capability 检查。

## 7. MCP Tools

### 7.1 命名规则

规则：

- tool 名称使用点分层：`task.add`。
- tool 参数使用 snake_case。
- tool 不暴露 CLI flag 名称，例如不使用 `--project-id`，而是 `project_id`。
- tool 描述必须说明副作用：只读、写任务、写 context、写 config。
- tool schema 必须测试，避免字段漂移。

### 7.2 Task tools

#### `task.add`

用途：创建任务。

输入：

- `description` 必填。
- `project` / `project_id` 可选。
- `tags` 可选。
- `priority` 可选。
- `due` / `wait` / `scheduled` / `until` 可选。
- `annotations` 可选。
- `workspace` 可选。

输出：

- `data.task`：任务 view。
- `rendered`：类似 CLI add 成功信息。

规则：

- project 必须是已存在、未归档 project。
- project-scoped token 创建任务时，如果指定 project 不在 allowlist，返回 `project_scope_denied`。
- 没有指定 project 但 token 只有一个 project allowlist 时，是否自动填充 project 由 plan 阶段决定；如果自动填充，必须在 spec 实施计划中写测试。默认建议不自动填充，要求显式 project，避免 Agent 写错项目。

#### `task.query`

用途：查询任务列表。

输入：

- `query` 可选，Taskwarrior 风格过滤表达式。
- `status` 可选。
- `project` / `project_id` 可选。
- `workspace` 可选。
- `limit` 可选。
- `include_completed` 可选。
- `include_deleted` 可选。

输出：

- `data.tasks`。
- `data.count`。
- `rendered`：表格或摘要。

规则：

- 默认不返回 deleted。
- limit 必须有上限，建议继承 HTTP list 上限；如果当前 HTTP 没有 list 上限，M7 应为 MCP 单独设默认上限，避免 Agent 一次拉全库。
- token project scope 必须叠加到 query。

#### `task.get`

用途：读取单任务。

输入：

- `id`：UUID 或工作集 ID。
- `workspace` 可选。
- `project` / `project_id` 可选。

输出：

- `data.task`。
- `rendered`：类似 info 输出。

规则：

- stdio 可支持工作集 ID。
- HTTP MCP 默认不应依赖客户端本机 working set；远程场景推荐 UUID。
- 如果 HTTP MCP 支持工作集 ID，必须使用服务端 actor/workspace 维度 working set，不得读取客户端本机状态。
- 任务存在但不在 project allowlist 时返回 `task_not_found`，避免泄露存在性。

#### `task.modify`

用途：修改任务字段。

输入：

- `id` 必填。
- `description`、`project`、`project_id`、`priority`、`due`、`wait`、`scheduled`、`until`、`tags`、`remove_tags`、`udas` 等可选。
- `clear` 可选，表示清空指定字段。

输出：

- `data.task`。
- `rendered`：修改摘要。

规则：

- 必须复用 app service 的 replace/modify 语义。
- project 修改必须遵守严格 project 注册。
- project-scoped token 不能把任务移出 allowlist。

#### `task.done`

用途：完成任务。

输入：

- `id` 必填。
- `workspace` 可选。

输出：

- `data.task`。
- `rendered`：完成摘要。

规则：

- 递归任务生成语义沿用 M2。
- project scope 与单任务读取一致。

#### `task.delete`

用途：删除任务。

输入：

- `id` 必填。
- `workspace` 可选。

输出：

- `data.task`。
- `rendered`：删除摘要。

规则：

- 不做硬删除，沿用任务状态删除语义。

#### `task.annotate`

用途：添加 annotation。

输入：

- `id` 必填。
- `annotation` 必填。

输出：

- `data.task`。
- `rendered`：注释摘要。

M7 不新增 `task.denotate`，除非 plan 阶段确认 app service 已有稳定能力且测试成本很低。ROADMAP M7 列表只要求 annotate。

#### `task.depends`

用途：设置或调整依赖。

输入：

- `id` 必填。
- `depends`：依赖任务 UUID 列表。
- `append` 可选，默认 false 表示替换。

输出：

- `data.task`。
- `rendered`：依赖摘要。

规则：

- 依赖目标必须在同 workspace 且对 actor 可见。
- project-scoped token 不能通过 depends 泄露 allowlist 外任务。

#### `task.start` / `task.stop`

用途：开始 / 停止任务。

输入：

- `id` 必填。

输出：

- `data.task`。
- `rendered`。

规则：

- 复用 CLI start/stop 语义。

### 7.3 Report / urgency tools

#### `report.run`

用途：运行报表。

输入：

- `name` 必填，例如 `next`、`completed`、`overdue`。
- `query` 可选。
- `workspace` / `project` / `project_id` 可选。
- `limit` 可选。

输出：

- `data.report`。
- `data.tasks`。
- `rendered`。

规则：

- 只读。
- project token scope 必须叠加。

#### `urgency.explain`

用途：解释任务 urgency。

输入：

- `id` 必填。

输出：

- `data.urgency`。
- `data.factors`。
- `rendered`。

规则：

- 只读。
- 不重新定义 urgency 公式。
- 使用当前 workspace/project config。

### 7.4 Workspace tools

#### `workspace.list`

用途：列出 actor 可见 workspaces。

输入：

- `include_archived` 可选，默认 false。

输出：

- `data.workspaces`。
- `rendered`。

规则：

- HTTP MCP 必须受 token workspace allowlist 限制。

#### `workspace.current`

用途：返回当前 effective workspace。

输入：

- `workspace` 可选，用于解析并展示某个 workspace。

输出：

- `data.workspace`。
- `rendered`。

规则：

- M7 不要求 MCP tool 修改当前 workspace；`workspace.current` 是只读。
- 如果后续需要切换 active workspace，应另行设计 `workspace.set_current`，不要把写语义藏进 current。

### 7.5 Project tools

#### `project.list`

用途：列出当前 workspace 下项目。

输入：

- `workspace` 可选。
- `include_archived` 可选。

输出：

- `data.projects`。
- `rendered`。

规则：

- project-scoped token 只能看到 allowlist 内 project。

#### `project.get`

用途：读取项目详情。

输入：

- `project` 或 `project_id` 必填。
- `workspace` 可选。

输出：

- `data.project`。
- `data.config_summary` 可选。
- `rendered`。

规则：

- 如果 token project allowlist 不包含该项目，返回 `project_not_found` 或 `project_scope_denied` 的选择必须和 HTTP project endpoint 保持一致；单任务存在性隐藏规则不自动推广到 project 详情。

#### `project.current`

用途：返回当前 effective project scope。

输入：

- `workspace` 可选。
- `project` / `project_id` 可选。

输出：

- `data.project` 或 `null`。
- `rendered`。

规则：

- 只读。
- 如果没有 effective project，返回 `data.project = null`，不要报错。

### 7.6 Context tools

#### `context.show`

用途：展示当前 active context 或指定 context。

输入：

- `name` 可选。
- `workspace` 可选。

输出：

- `data.context`。
- `rendered`。

规则：

- 只读。
- 使用服务端 SQLite active context；HTTP MCP 不读取客户端 TOML。

#### `context.set`

用途：设置 active context。

输入：

- `name` 必填；特殊值 `none` 可清空 context。
- `workspace` 可选。

输出：

- `data.context`。
- `rendered`。

规则：

- 写服务端 SQLite 的 `(actor, workspace)` active context。
- HTTP MCP 不能修改客户端本机 context。

### 7.7 Config tools

#### `config.get`

用途：读取配置。

输入：

- `key` 必填。
- `scope` 可选：`local`、`workspace`、`project`。
- `workspace` / `project` / `project_id` 可选。

输出：

- `data.key`。
- `data.value`。
- `data.scope`。
- `rendered`。

规则：

- HTTP MCP 不支持读取操作者本机 local config。
- stdio MCP 可读 local config，但业务配置优先服务端 DB。
- project config 必须要求 project config read permission。

#### `config.set`

用途：写配置。

输入：

- `key` 必填。
- `value` 必填。
- `scope` 必填：`workspace` 或 `project`。
- `workspace` / `project` / `project_id` 可选。

输出：

- `data.key`。
- `data.value`。
- `data.scope`。
- `rendered`。

规则：

- HTTP MCP 不支持写操作者本机 local config。
- `color`、`json` 这类个人渲染配置不应通过 HTTP MCP 写 workspace/project 配置。
- config key 权限继续复用 M6/M5 分类。

## 8. MCP Resources

M7 应提供最小 resources，让 Agent 获取背景而不是把所有上下文塞进 tool 参数。

### 8.1 Resource URI

建议 URI：

```text
taskg://workspace/current
taskg://workspace/{workspace_id}
taskg://project/{project_id}
taskg://context/current
```

### 8.2 workspace resource

内容：

- workspace id。
- slug。
- name。
- actor role。
- active context summary。
- 可见 project 摘要。

规则：

- HTTP MCP 受 token workspace scope 限制。
- 不返回其它用户 token、audit 明细或成员隐私信息。

### 8.3 project resource

内容：

- project id。
- slug。
- name。
- description。
- status / archived。
- project config 中 Agent 可读背景，例如 `agent.background`、`agent.constraints`、`agent.default_context`。

规则：

- 只读取 allowlist 内 project。
- 如果项目不存在或不可见，返回 not found。
- 不把任意 config 全量暴露给 Agent；只暴露被 spec 允许的 Agent 上下文 key。

### 8.4 context resource

内容：

- 当前 context name。
- context filter。
- effective workspace。
- effective project scope。

规则：

- HTTP MCP 使用服务端 SQLite active context。
- stdio MCP 使用本地 DB active context。

## 9. Agent 上下文配置

M5 已有 project config。M7 定义 Agent 可读 key：

| Key | Scope | 含义 |
|---|---|---|
| `agent.background` | project | 项目背景 |
| `agent.constraints` | project | Agent 执行约束 |
| `agent.default_context` | project | Agent 默认 context 摘要 |
| `agent.handoff` | project | 给 Agent 的交接说明 |

规则：

- 这些 key 是普通 project config 的受控子集。
- `project.get` 和 project resource 可以读取这些 key。
- `config.get` 仍可按权限读取其它允许 key。
- M7 不做向量记忆、长短期记忆、自动总结和外部知识库。

## 10. 远程 CLI 管理命令收口

### 10.1 workspace

远程支持：

```bash
taskg --server ... workspace list
taskg --server ... workspace info <slug|uuid>
taskg --server ... workspace add <slug> name:<name>
taskg --server ... workspace modify <slug|uuid> name:<name>
taskg --server ... workspace use <slug|uuid>
taskg --server ... workspace archive <slug|uuid>
```

规则：

- 接 M6 HTTP workspace endpoint。
- 输出与本地 human / JSON 尽量一致。
- `workspace use` 需要新增 `POST /api/v1/workspaces/{workspace}/use` 或等价 endpoint。
- `workspace use` 写服务端 SQLite 中 actor 的 active workspace，后续远程命令没有显式 `--workspace` 时可使用该状态。
- `workspace use` 不能修改客户端本机 TOML 或本地 DB。

### 10.2 user

远程支持：

```bash
taskg --server ... user list
taskg --server ... user info <user>
taskg --server ... user add <name>
```

规则：

- M7 需要新增最小 user management REST endpoint：
  - `GET /api/v1/users`
  - `POST /api/v1/users`
  - `GET /api/v1/users/{user}`
- 这些 endpoint 必须复用 app service 权限，不得在 handler 中直接创建 user。
- 新增 endpoint 必须更新 OpenAPI。
- `user use` 远程继续返回 `remote_unsupported_command`，因为 HTTP token 的 actor 已由 token 决定，不能由远程 CLI 切换。

### 10.3 member

远程支持：

```bash
taskg --server ... member list --workspace <workspace>
taskg --server ... member add <user> --workspace <workspace> --role member
taskg --server ... member role <user> --workspace <workspace> --role admin
```

规则：

- 接 M6 workspace member endpoint。
- role enum 与 HTTP/OpenAPI 一致：`owner`、`admin`、`member`、`viewer`。
- owner 降级、最后 owner 等边界沿用 app service。

### 10.4 show

远程支持：

```bash
taskg --server ... show
taskg --server ... show <key>
```

规则：

- 读取服务端 config，不读取本机任务 DB。
- 明确区分 local config、workspace config、project config。
- HTTP MCP / 远程 CLI 下不暴露 remote token 原文。

## 11. HTTP 与 MCP 的复用边界

### 11.1 可以共享的层

必须共享：

- store 打开与生命周期。
- clock。
- app service 构造。
- request scope 授权。
- stable error code。
- render。
- audit。

建议共享：

- access log。
- request id。
- panic recover。
- body limit。

### 11.2 不应共享的层

不应共享：

- REST JSON envelope 与 MCP JSON-RPC envelope。
- OpenAPI schema 与 MCP tool schema。
- REST endpoint handler 与 MCP tool handler。

解释：

- HTTP REST 是资源 API。
- MCP 是 tool/resource 协议。
- 二者应共享 app service，不应互相伪装。

### 11.3 MCP 是否调用 HTTP API

MCP tool 不应通过本机 HTTP client 调用 `/api/v1`。

理由：

- 会增加序列化开销。
- 会让错误和权限走两遍。
- 会把进程内 app service 复用变成绕路 HTTP。

正确路径：

```text
MCP tool -> app service -> repository
HTTP handler -> app service -> repository
CLI command -> app service -> repository
```

## 12. OpenAPI 与 MCP schema

### 12.1 OpenAPI

M7 只在 REST endpoint 变化时更新 OpenAPI。

规则：

- `/mcp` 不进入 OpenAPI。
- Phase 0 需要为远程 `user list/info/add` 和 `workspace use` 补最小 REST endpoint，必须更新 `docs/openapi/taskg-v1.yaml`。
- 如果只是把远程 CLI 接到已有 endpoint，不需要新增 OpenAPI path。

### 12.2 MCP tool schema

MCP tool schema 是 M7 的验收重点。

必须测试：

- 所有工具都出现在 `tools/list`。
- 每个工具的 input schema 包含必填字段。
- snake_case 字段稳定。
- 不出现 Go 内部字段名。
- 不出现 CLI flag 名称如 `project-id`。
- `workspace`、`project`、`project_id` 三类 scope 参数一致。

## 13. 测试策略

### 13.1 Phase 0 验证

必须验证：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' && exit 1 || true
```

M7 plan 应在 Go 升级后先跑上述命令，再继续 MCP 实现。

### 13.2 MCP unit tests

覆盖：

- tool 注册。
- schema 稳定性。
- result envelope。
- app error 到 MCP error 映射。
- stdio scope resolver。
- HTTP scope resolver。

### 13.3 MCP integration tests

使用官方 SDK client 做集成测试：

- in-memory transport 或 stdio transport 调用本地 MCP server。
- Streamable HTTP transport 调用 `/mcp`。
- `tools/list` 能列出 M7 要求 tools。
- `task.add` 创建项目任务。
- `task.query` 只能看到授权 project。
- `task.get` 对 allowlist 外任务返回 not found。
- `urgency.explain` 返回 factors。
- `report.run` 返回结构化 data 和 rendered。
- HTTP MCP 无 token 返回 401。
- HTTP MCP project-scoped token 不能越界读写。
- stdio MCP stdout 不出现非 MCP JSON-RPC 内容。

### 13.4 远程 CLI 管理命令测试

覆盖：

- 远程 `workspace list/info/add/modify/archive`。
- 远程 `workspace use`。
- 远程 `user list/info/add`。
- 远程 `member list/add/role`。
- 远程 `show`。
- 仍 unsupported 的本机语义命令返回 `remote_unsupported_command`。
- 远程管理命令不触碰本地 DB。

### 13.5 回归测试

继续跑：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go vet ./...
```

影响 HTTP 时额外跑：

```bash
go test -race ./internal/httpapi ./internal/app
```

影响 MCP 时额外跑：

```bash
go test -race ./internal/mcpserver ./internal/app
```

## 14. 分阶段交付

### Phase 0：平台升级与 M6 遗留收口

目标：

- 升级 Go。
- 引入官方 MCP Go SDK。
- 补齐远程管理命令。

交付：

- `go.mod` Go 版本更新。
- README / ROADMAP / AGENTS 技术栈说明更新。
- M6 遗留远程管理命令接线。
- 相关集成测试。

验收：

- 全量测试通过。
- CGO-free 测试和构建通过。
- 不引入 CGO SQLite driver。

### Phase 1：MCP server 基础设施

目标：

- 建立 `internal/mcpserver`。
- 实现 stdio MCP。
- 实现 HTTP `/mcp`。
- 建立 result/error/scope adapter。

交付：

- `taskg mcp stdio`。
- `taskg server` 挂载 `/mcp`。
- HTTP MCP Bearer token 鉴权。
- stdio MCP 本地 actor/workspace 解析。
- schema/listTools 基础测试。

验收：

- SDK client 可调用 stdio MCP。
- SDK client 可调用 HTTP MCP。
- 未授权 HTTP MCP 返回 401。
- stdout/stderr 不破坏 stdio MCP。

### Phase 2：核心任务 tools

目标：

- 实现 task/report/urgency 工具。

交付：

- `task.add`
- `task.modify`
- `task.done`
- `task.delete`
- `task.query`
- `task.get`
- `task.annotate`
- `task.depends`
- `task.start`
- `task.stop`
- `report.run`
- `urgency.explain`

验收：

- Agent 可以完成“查询项目待办、添加项目任务、解释 urgency、完成任务”的闭环。
- project-scoped token 不能越界。
- 所有写操作写 audit。

### Phase 3：企业上下文 tools/resources

目标：

- 实现 workspace/project/context/config tools。
- 实现 Agent 可读 resources。

交付：

- `workspace.list`
- `workspace.current`
- `project.list`
- `project.get`
- `project.current`
- `context.set`
- `context.show`
- `config.get`
- `config.set`
- workspace/project/context resources。

验收：

- Agent 能读取项目背景和约束。
- HTTP MCP 不读取客户端本机 TOML。
- project resource 受 token allowlist 限制。

## 15. 不进入 M7

M7 不做：

- 外部系统 adapter。
- 飞书 / Slack / GitHub issue 直接集成。
- op-log 同步。
- Hook / trigger 引擎。
- JWT/password login/refresh token。
- OAuth 登录。
- 多节点服务端部署。
- 复杂 Agent 编排平台。
- 向量记忆。
- 任务自动分解 Agent。
- Web UI。

## 16. ROADMAP 对齐

ROADMAP M7 要求：

- 本地 MCP stdio 能被 MCP 客户端调用。
- HTTP MCP 能鉴权并限制 workspace。
- 带 project scope 的 Agent 只能查询和修改授权 project 中的任务。
- MCP tool 与 CLI/API 复用同一 app service。
- 每个 tool 都有 schema 和集成测试。
- Agent 可以完成常见任务管理流程。

本规格完全覆盖以上要求，并额外明确：

- Go 升级是 M7 Phase 0。
- MCP 采用官方 Go SDK，避免重复造轮子。
- REST HTTP 不换框架。
- M6 遗留远程管理命令作为 M7 Phase 0 收口。
- `/mcp` 不进入 OpenAPI。

## 17. 验收标准

M7 完成时必须满足：

- `go.mod` 和文档已统一到新的 Go 版本。
- 官方 MCP Go SDK 已接入。
- `taskg mcp stdio` 可被 MCP 客户端调用。
- `taskg server` 暴露 `/api/v1/*` 和 `/mcp`。
- HTTP MCP 使用 M6 Bearer token 鉴权。
- stdio MCP 使用本地 actor/workspace。
- project-scoped token 只能查询和修改授权 project。
- MCP tools 复用 app service，不复制业务规则。
- MCP resources 能返回 workspace/project/context 背景。
- tool schema 有测试。
- stdio / HTTP MCP 有集成测试。
- M6 遗留远程管理命令已补齐或明确保留 unsupported 的原因。
- README、ROADMAP、OpenAPI、spec/plan 已按实际实现同步。
- 验证命令通过：

```bash
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' && exit 1 || true
```

## 18. implementation plan 默认决策

M7 plan 按下列默认决策拆解，不再把这些问题留到实现中临场判断：

1. `workspace use` 远程模式支持，新增最小服务端 active workspace endpoint，并更新 OpenAPI。
2. `user add/list/info` 远程模式支持，新增最小 user management REST endpoint，并更新 OpenAPI。
3. `user use` 远程模式继续 unsupported，因为 actor 由 token 决定。
4. HTTP MCP `task.get` 只承诺 UUID；stdio MCP 可以支持工作集 ID。
5. `task.add` 不自动填充 project；即使 token 只有一个 project allowlist，Agent 也必须显式指定 project 或 project_id。
6. M7 Phase 0 补齐 `context:read`、`context:write`、`config:read`、`config:write` capability，避免 MCP 放宽权限。
