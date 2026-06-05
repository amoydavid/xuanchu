# xuanchu M6 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 让同一个 `xuanchu` 二进制可以作为 HTTP/JSON API 服务端运行，并让远程 CLI 与 Agent token 通过同一套 actor、workspace、project scope 访问已有 app service。

**范围策略：** M6 只做服务化入口、令牌鉴权、请求级 scope 和远程 CLI 基础，不做 MCP、不做 op-log 同步、不做外部系统 adapter。M6 的关键决策是：HTTP/API/远程 CLI 必须复用 `internal/app`，权限边界来自 token 与 membership，不能在 handler 或远程 CLI 中复制业务规则。

**需求来源：** 本规格从 [README.md](/Users/mac/code/projects/dajee/task/README.md)、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)、[M5 设计规格](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-30-xuanchu-m5-design.md)，以及当前 M5 已完成实现边界中收束。

---

## 1. 当前基础

M5 已完成并合并。当前项目已有：

- Go 1.22 单二进制入口。
- Cobra 本地 CLI。
- GORM + `github.com/glebarez/sqlite`，保持零 CGO。
- 多 user、多 workspace、membership、权限和 audit。
- `workspace` 作为企业 / 租户级隔离边界。
- `project` 已经是 workspace 内一等实体。
- 任务写入、查询、context、helper、audit 已基于稳定 `project_id`。
- `project config` 已建立 project 级业务配置边界。
- app service 已集中处理运行时 actor、workspace、权限、业务规则。

M6 不重做任务模型和 project 模型。M6 的工作是把已有 app service 暴露到 HTTP/JSON，并让 CLI 在本地模式和远程模式下尽量保持相同行为。

## 2. 核心产品决策

### 2.1 服务端是同一个二进制的运行模式

M6 新增：

```bash
xuanchu server --listen :8080
xuanchu server --listen 127.0.0.1:8080 --db ./xuanchu.db
xuanchu server --listen :8080 --data-dir ./data
```

规则：

- `server` 是 `xuanchu` 的子命令，不新增第二个二进制。
- 服务端数据库路径继续遵守现有 `--db`、`--data-dir`、TOML、XDG data、home fallback 规则。
- 服务端启动后使用同一个 SQLite 文件，通过表内 `workspace_id` 做行级隔离。
- 服务端模式不读取请求方本机 TOML，也不依赖请求方本机 active context。服务端持有的 `(actor, workspace)` 维度 active context 仍存于服务端 SQLite，按 M4/M5 既有语义工作。
- 服务端进程自己的 TOML 只描述启动和显示配置，例如数据库路径、默认 listen 地址。
- 服务端必须支持 graceful shutdown。

### 2.2 HTTP 是协议层，不是第二套业务实现

HTTP handler 只负责：

- 解析请求。
- 鉴权 token。
- 解析请求级 workspace/project scope。
- 调用 `internal/app`。
- 编码 JSON 响应和稳定错误结构。

HTTP handler 不负责：

- 直接拼 SQL。
- 自己实现 task/project/context/config 权限。
- 自己解释 Taskwarrior 查询语义。
- 绕过 app service 写 audit。

如果现有 app service 缺少某个可复用方法，应补 app service，而不是把逻辑写在 `internal/httpapi`。

### 2.3 Token 是权限收窄，不是权限放大

M6 新增 PAT / Agent token。token 绑定一个 `user_id`，并可进一步限制：

- 可访问 workspace allowlist。
- 可访问 project allowlist。
- 能力 scope，例如 `task:read`、`task:write`、`project:read`。

token 不能让用户越过 membership role。最终权限是：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

例子：

- 一个 `viewer` 用户的 token 即使带 `task:write`，仍不能写任务。
- 一个 `member` 用户的 token 如果只允许 project A，则不能读取 project B 的任务。
- 一个 token 如果只允许 workspace `dajee`，请求 `--workspace partner` 必须失败。

### 2.4 project scope 以 `project_id` 为准

M6 延续 M5 决策：

- API、token、远程 CLI、后续 MCP 优先使用 `project_id`。
- project slug 只作为 human input，必须在明确 effective workspace 内解析。
- `project_id` 与 `workspace` 同时出现时，project 必须属于该 workspace。
- `project` slug 与 `project_id` 同时出现时，slug 必须解析到同一个 project。
- token 的 project allowlist 存 `project_id`，不存裸 slug。

### 2.5 远程 CLI 是 API client，不是 SSH shell

M6 支持：

```bash
xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" list
xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" --workspace dajee list project:ai-agent-platform
xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" add "Review API" --project-id <uuid>
```

规则：

- `--server` 开启远程模式。
- 远程模式下 CLI 调用 HTTP API，再使用现有 render 逻辑输出 human / JSON。
- 远程模式 stdout/stderr 分离要求不变。
- 远程模式不直接打开本地任务数据库。
- 远程模式可以读取本机 TOML / env 获取 server 和 token，但业务配置仍来自服务端 DB。
- M6 不要求所有本地命令一次性远程化；必须覆盖核心任务、报表、project、context/config、token 自举所需命令。

## 3. M6 产品体验

### 3.1 启动服务端

```bash
xuanchu server --listen :8080 --data-dir ./data
```

启动行为：

- 初始化或打开 SQLite。
- 自动完成既有迁移。
- 输出启动信息到 stderr，不污染 stdout。
- 收到 SIGINT/SIGTERM 后停止接收新请求，默认等待进行中请求 30 秒后超时退出；`--shutdown-timeout 30s` 可覆盖。
- 服务端启动时执行的 schema migration、默认身份初始化等内部初始化不写 audit。运行时所有写操作必须有明确 actor，actor 来自 token。
- M6 不使用 `actor_user_id = NULL` 的 audit 路径。后续如需系统任务 audit，必须单独设计 system actor。
- M6 服务端默认每个请求输出一行简短访问日志到 stderr，包含 method、path、status、actor_user_id、token_id、duration。格式先使用 text；JSON 日志、日志级别配置和采样留给后续版本。
- 鉴权失败或 `/healthz` 这类匿名请求没有 actor/token 时，访问日志中的 actor_user_id 和 token_id 写为 `-`。
- M6 服务端不内置 TLS，只提供 HTTP。生产部署必须放在可信网络内，或由 Nginx/Caddy 等反向代理做 TLS termination；不要把裸 HTTP token 服务直接暴露到公网。
- 服务端运行期间，本地 CLI 仍可直接访问同一个 SQLite 文件。SQLite 支持多进程读，写入按文件锁排队；但生产部署建议同一时间只有一个写入口。并发写如果遇到 `SQLITE_BUSY`，M6 只要求返回清晰错误，不承诺高并发写体验。
- `github.com/glebarez/sqlite` 保持纯 Go、零 CGO，但并发写吞吐不是 M6 目标。M6 验收只要求功能正确和锁错误可解释，不做高并发优化、不切换 SQLite driver、不引入 CGO。

### 3.2 创建 token

本地管理员可以创建 token：

```bash
xuanchu token create cli-dajee \
  --user alice \
  --workspace dajee \
  --scope task:read,task:write,project:read,context:read \
  --expires-in 720h

xuanchu token create agent-api-docs \
  --user agent-api \
  --workspace dajee \
  --project-id <project-id> \
  --scope task:read,task:write,project:read
```

输出规则：

- raw token 只在创建时输出一次。
- `--json` 输出包含 `token`、`id`、`name`、`user_id`、`workspace_ids`、`project_ids`、`scopes`、`expires_at`。
- human 输出必须明确提示 token 只显示一次。
- 数据库只保存 token 哈希和短前缀，不保存 raw token。

M6 还需要：

```bash
xuanchu token list
xuanchu token revoke <token-id|token-prefix>
```

`xuanchu token list` 输出规则：

- human columns 固定为 `ID PREFIX NAME TYPE WORKSPACES SCOPES EXPIRES_AT LAST_USED_AT`。
- `--json` 输出 token view 字段：`id`、`prefix`、`name`、`type`、`user_id`、`workspace_ids`、`project_ids`、`scopes`、`created_at`、`expires_at`、`revoked_at`、`last_used_at`，但不得包含 raw token 或 token hash。
- `expires_at` 对外使用 RFC3339 字符串，`null` 表示永不过期；不要在同一 endpoint 中混用 epoch number 和字符串时间。

token 创建有两条路径：

- 本地 CLI：直接打开 SQLite 创建 token，使用本地 active user 作为 actor。这是 fresh server 的 bootstrap 路径。
- HTTP API：通过已有 token 创建或 revoke token，受 token capability、workspace/project scope 和 membership role 共同限制。

建议在 `xuanchu server` 启动前，先用本地 CLI 创建至少一个 admin token 作为远程 bootstrap 凭证。M6 不在 `xuanchu server` 启动时输出 bootstrap token，也不提供匿名 token 创建 endpoint。server 已运行后，后续 token 应优先通过 HTTP API 创建，避免长期绕开服务端写入口。

本地 `xuanchu token create` 规则：

- 省略 `--user` 时，token 归属当前 active user。
- 指定 `--user other` 时，当前 actor 必须在目标 workspace 中是 admin/owner。
- `--scope` 支持逗号分隔和重复 flag 两种形式，例如 `--scope task:read,task:write` 与 `--scope task:read --scope task:write` 等价。
- `--expires-in` 使用 Go `time.ParseDuration` 兼容格式，例如 `720h`。M6 不支持 `30d`、`1y` 这类日期单位；需要 30 天时写 `720h`。
- 省略 `--expires-in` 时 `expires_at = NULL`，表示永不过期。文档示例应优先给出过期时间，避免默认创建长期 token。

M6 不做用户名密码登录和短期 JWT。JWT 可在 M6.5 或后续里程碑单独设计。

### 3.3 使用远程 CLI

连接配置可以来自 flag、环境变量或本机 TOML：

```bash
XUANCHU_SERVER=http://127.0.0.1:8080
XUANCHU_TOKEN=xuanchu_pat_xxx

xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" list
xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" --workspace dajee next
xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" --project-id <project-id> add "Draft OpenAPI docs"
```

优先级：

```text
CLI flag > 环境变量 > xuanchu.toml > 空值
```

建议 TOML：

```toml
[remote]
server = "http://127.0.0.1:8080"
token = "xuanchu_pat_xxx"
```

说明：

- `remote.server` 和 `remote.token` 是本机连接配置，不是 workspace 业务配置。
- token 写入 TOML 是本地便利功能，不进入服务端 DB 业务配置。
- 推荐优先用环境变量 `XUANCHU_TOKEN` 或系统 secret manager 注入 token。把 `remote.token` 写入 TOML 时，用户必须自行保护文件权限，建议 `0600`；不要把含 token 的 `xuanchu.toml` 提交到公共 repo。
- M6 不提供 token 加密存储，也不做系统钥匙串集成。
- 如果 CLI 发现 `xuanchu.toml` 包含 `remote.token` 且文件权限比 `0600` 更宽，应向 stderr 输出 warning，但不阻止执行。

### 3.4 Agent token 的使用边界

Agent token 与 PAT 使用同一个 HTTP bearer 机制：

```http
Authorization: Bearer xuanchu_agent_xxx
```

Agent token 的区别在于：

- token type 为 `agent`。
- 创建 Agent token 时必须显式提供至少一个 `--workspace` 或 `--workspace-id`。
- 创建 Agent token 时必须显式提供 `--scope`，不接受默认 scope。
- Agent token 不能使用通配或隐式全权限 scope。
- 推荐显式设置 `--project-id`，除非用户明确创建 workspace 级 Agent token。

M6 不实现 MCP，但必须让 M7 可以直接复用 token 校验和 request scope 解析。

## 4. 架构与目录

建议新增目录：

- `internal/httpapi`
  - HTTP router、handler、middleware、request/response DTO、OpenAPI 文档静态输出。
- `internal/auth`
  - token 生成、hash、常量时间比较、scope 解析。
- `internal/remote`
  - 远程 CLI client、请求构造、错误解码。

建议新增或扩展 app 层：

- `internal/app/token.go`
  - `CreateToken/ListTokens/RevokeToken/AuthenticateToken`。
- `internal/app/request_scope.go`
  - HTTP/远程 CLI 请求级 workspace/project scope 解析。
- `internal/app/service.go`
  - 支持从已解析的 `RuntimeContext` 和 token scope 构造 service。

建议新增 storage：

- `internal/storage/token_repo.go`
  - token CRUD、按 prefix 查询、revoke、过期过滤。

建议新增 CLI：

- `internal/cli/server.go`
- `internal/cli/token.go`
- 现有核心命令接入远程模式分支。

### 4.1 app service 构造方式

当前 `NewService(ServiceOptions)` 会从本地 active user / active workspace 解析 runtime。M6 需要新增请求级构造能力：

```go
type RequestScope struct {
    ActorUserID      string
    ActorName        string
    WorkspaceID      string
    WorkspaceSlug    string
    Role             Role
    TokenID          *string
    TokenType        string
    TokenScopes      []string
    AllowedWorkspaceIDs []string
    AllowedProjectIDs   []string
}
```

HTTP handler 应先完成鉴权和 scope 解析，再创建 app service。不要让 app service 在 HTTP 请求中读取 `active_workspace.<user>` meta 作为默认值，除非请求和 token scope 明确允许这种 fallback。

M6 推荐新增：

```go
NewServiceForRuntime(store, runtime, scope, opts)
```

或等价构造器。目标是让 CLI 本地模式、HTTP 模式、后续 MCP 模式都能复用同一业务 service。

### 4.2 请求级 effective workspace

HTTP 请求解析 workspace 的顺序：

```text
请求参数 workspace / X-Xuanchu-Workspace > token 单 workspace scope > actor default workspace > 错误
```

规则：

- 如果 token 只允许一个 workspace，请求可以省略 workspace。
- 如果 token 允许多个 workspace，请求必须显式提供 workspace，除非 actor default workspace 在 token scope 内。
- 如果 actor default workspace 不在 token workspace scope 内，不允许 fallback；请求必须显式提供 workspace，或依赖 token 单 workspace scope。
- 如果请求 workspace 不在 token scope 内，返回 `workspace_scope_denied`。
- 如果 actor 不是该 workspace member，返回 `membership_not_found` 或 `permission_denied`。
- 不允许因 project_id 推断 workspace 后绕过 token workspace scope。

### 4.3 请求级 project scope

HTTP 请求可以通过三种方式指定 project：

- `project_id=<uuid>`
- `project=<slug>`
- 查询表达式里的 `project:<slug>`

规则：

- `project_id` 优先。
- `project` slug 必须在 effective workspace 内解析。
- 同时给出 `project` 和 `project_id` 时必须指向同一个 project。
- 如果 token 带 project allowlist，所有任务读写、report、import/export、audit project 查询都必须叠加 project allowlist。
- 如果请求明确指定的 project 不在 token allowlist 内，返回 `project_scope_denied`。
- 如果 token 有 project allowlist，但请求没有指定 project，列表和报表返回 allowlist 内所有 project 的数据。
- 无 project 的任务在 project-scoped token 下默认不可见。`project:` 空值查询是合法查询，但在 project-scoped token 下永远返回空集，因为 allowlist 不包含“无 project”。M6 不提供 `project:none` 特殊 scope。

## 5. 数据模型

### 5.1 `api_tokens`

新增表：

```text
api_tokens
- id TEXT PRIMARY KEY
- user_id TEXT NOT NULL
- name TEXT NOT NULL
- type TEXT NOT NULL            -- pat | agent
- token_prefix TEXT NOT NULL UNIQUE
- token_hash TEXT NOT NULL
- scopes_json TEXT NOT NULL     -- JSON array
- workspace_ids_json TEXT NOT NULL DEFAULT '[]'
- project_ids_json TEXT NOT NULL DEFAULT '[]'
- created_at INTEGER NOT NULL
- expires_at INTEGER NULL
- revoked_at INTEGER NULL
- last_used_at INTEGER NULL
```

必需索引：

- `idx_api_tokens_user(user_id)`
- `idx_api_tokens_prefix(token_prefix)`，由 `UNIQUE` 约束提供。

M6 不要求 `idx_api_tokens_active` partial index。每个 user 的 token 量级预计很小，`ListByUser` 直接按 `user_id` 过滤即可；`WHERE revoked_at IS NULL` 的 partial index 属于后续性能优化，不影响 M6 语义。

规则：

- raw token 使用至少 32 bytes 随机数生成，并带可读前缀：
  - `xuanchu_pat_...`
  - `xuanchu_agent_...`
- `token_prefix` 保存前 12 到 16 个可用于列表和快速定位的字符。
- `token_hash` 保存 raw token 的 SHA-256 hex 或等价安全哈希。
- 校验时先按 prefix 找候选，再用常量时间比较 hash。
- `expires_at IS NULL` 表示永不过期。校验时如果 `expires_at IS NOT NULL && now > expires_at`，视为过期。
- `revoked_at IS NOT NULL` 表示已撤销，撤销后立即不可用。
- M6 不缓存 token validation；每次请求都查 DB。内存缓存和 revoke broadcast 留给后续版本。
- `last_used_at` 更新不进入 audit。实现可以每 N 秒合并更新一次，避免每个请求都争抢 SQLite 写锁；M6 测试只要求成功鉴权后 `last_used_at` 在合理时间内更新，不要求精确每请求更新。

M6 不引入 password_hash 登录流，不新增 session 表。

### 5.2 audit

新增 audit action：

- `token.create`
- `token.revoke`
- `server.request.denied` 可选，默认不记录每次失败请求，避免 audit 噪音和 DoS 放大。

token audit payload 至少包含：

- `token_id`
- `token_name`
- `token_type`
- `workspace_ids`
- `project_ids`
- `scopes`
- `expires_at`

不要把 raw token 或 token hash 写入 audit。

## 6. HTTP API

### 6.1 协议基础

- base path：`/api/v1`
- 鉴权：`Authorization: Bearer <token>`
- M6 仅接受 Authorization Bearer header。不支持 query string token、cookie、HTTP Basic Auth。`Bearer` scheme 大小写不敏感，`bearer <token>` 可接受；其它 scheme 一律按缺少 bearer token 处理。
- 请求/响应：JSON，`Content-Type: application/json`
- 日期：沿用当前 task JSON 语义，内部 int64 epoch，API DTO 对外使用 RFC3339 字符串或既有 JSON 字段语义；同一 endpoint 内不得混用。
- 错误：稳定 JSON envelope。
- 查询表达式作为 URL query parameter 时必须 URL-encoded。例如 `+next` 需要传为 `%2Bnext`。

### 6.2 成功响应结构

统一响应：

```json
{
  "data": {},
  "meta": {
    "workspace_id": "uuid",
    "workspace_slug": "dajee"
  }
}
```

列表响应：

```json
{
  "data": [],
  "meta": {
    "workspace_id": "uuid",
    "workspace_slug": "dajee",
    "returned": 12
  }
}
```

M6 不做 cursor pagination。`returned` 表示本次响应返回的数量，不表示全量总数。列表 endpoint 可以先支持 `limit`，默认上限 500，超出返回 `api_limit_too_large`。

### 6.3 错误响应结构

```json
{
  "error": {
    "code": "project_scope_denied",
    "message": "token cannot access project \"...\"",
    "details": {
      "project_id": "uuid"
    }
  }
}
```

`details` 是开放对象，OpenAPI 中用 `additionalProperties: true` 描述。客户端不能依赖 `details` 中的具体字段；稳定判断必须使用 `error.code`。

HTTP status 映射：

| Status | 场景 |
|---|---|
| 400 | 参数错误、查询语法错误、workspace/project 不一致 |
| 401 | 未提供 token、token 不存在、token 过期、token revoked |
| 403 | membership 权限不足、token scope 不允许 |
| 404 | workspace/project/task/context 不存在 |
| 409 | 唯一冲突、状态冲突、归档资源写入 |
| 422 | 业务校验失败，例如非法 priority、非法 UDA |
| 500 | 未分类内部错误 |

错误 code 必须和 CLI/app 层已有 code 尽量一致；HTTP 不要为同一业务错误发明第二套名字。

### 6.4 必须实现的 endpoint

健康检查：

```http
GET /healthz
```

返回：

```json
{"ok": true}
```

`/healthz` 不需要 token，只用于 load balancer / readiness probe。它不得暴露版本号、数据库路径、schema version 或 token 状态。

当前身份：

```http
GET /api/v1/me
```

最小返回结构：

```json
{
  "data": {
    "actor": {"id": "uuid", "name": "alice"},
    "token": {"id": "uuid", "name": "cli-dajee", "type": "pat", "scopes": ["task:read"]},
    "visible_workspaces": [{"id": "uuid", "slug": "dajee"}],
    "effective_workspace": {"id": "uuid", "slug": "dajee"}
  }
}
```

workspace：

```http
GET /api/v1/workspaces
GET /api/v1/workspaces/{workspace}
POST /api/v1/workspaces
PATCH /api/v1/workspaces/{workspace}
```

`{workspace}` 接受 slug 或 UUID，与 CLI `--workspace <slug|uuid>` 一致。如果请求同时提供 `{workspace}` 和 `workspace_id` query/body 字段，两者必须解析到同一个 workspace；不一致返回 400。

M6 不要求 HTTP 实现 workspace archive；可以进入 M6.5，除非 implementation plan 评估成本很低。

member：

```http
GET /api/v1/workspaces/{workspace}/members
POST /api/v1/workspaces/{workspace}/members
PATCH /api/v1/workspaces/{workspace}/members/{user}
```

project：

```http
GET /api/v1/projects?workspace=dajee&include_archived=false
POST /api/v1/projects
GET /api/v1/projects/{project}
PATCH /api/v1/projects/{project}
POST /api/v1/projects/{project}/archive
GET /api/v1/projects/{project}/config
GET /api/v1/projects/{project}/config/{key}
PUT /api/v1/projects/{project}/config/{key}
DELETE /api/v1/projects/{project}/config/{key}
```

`{project}` 接受当前 effective workspace 内的 project slug 或全局稳定 `project_id`。如果请求同时提供 `{project}` 和 `project_id` query/body 字段，两者必须解析到同一个 project；不一致返回 400。

task：

```http
GET /api/v1/tasks?workspace=dajee&query=+next&report=list
POST /api/v1/tasks
GET /api/v1/tasks/{uuid}
PATCH /api/v1/tasks/{uuid}
POST /api/v1/tasks/{uuid}/done
DELETE /api/v1/tasks/{uuid}
POST /api/v1/tasks/{uuid}/start
POST /api/v1/tasks/{uuid}/stop
POST /api/v1/tasks/{uuid}/annotations
DELETE /api/v1/tasks/{uuid}/annotations/{index}
```

报表：

```http
GET /api/v1/reports/{name}?workspace=dajee&query=project%3Aai-agent-platform
GET /api/v1/tasks/{uuid}/urgency
```

context：

```http
GET /api/v1/contexts
POST /api/v1/contexts
GET /api/v1/contexts/{name}
DELETE /api/v1/contexts/{name}
POST /api/v1/contexts/{name}/use
POST /api/v1/contexts/none
```

`contexts/none` 清除 effective workspace 内 actor 的 active context，与本地 CLI 的 `(actor, workspace)` 作用域一致。

config：

```http
GET /api/v1/config
GET /api/v1/config/{key}
PUT /api/v1/config/{key}
DELETE /api/v1/config/{key}
```

说明：

- 这些 config endpoint 只处理 workspace 业务配置。
- 本机 config 不通过 HTTP 修改。
- project config 只走 project config endpoint。

import/export：

```http
GET /api/v1/export?workspace=dajee
POST /api/v1/import
```

`POST /api/v1/import` 只接受 JSON body 中的任务数组或等价 JSON envelope。M6 不支持 multipart 文件上传。请求体大小默认上限 10 MB，超出返回 413 `api_payload_too_large`。import 必须保持整批事务语义；失败时整批回滚。audit 只写一条 `task.import`，payload 记录 count 和 project scope，不为每个任务单独写 audit。

audit：

```http
GET /api/v1/audit?workspace=dajee&project=ai-agent-platform&limit=50
```

需要 `audit:read` capability，并要求 actor 在 effective workspace 中具备 admin/owner role。audit list 必须叠加 token workspace/project scope。project-scoped token 不能读取 allowlist 之外的 audit。

M6 audit endpoint 只支持 `workspace`、`project`、`limit` 三个 query parameter。`actor`、`action`、`since` 等更细过滤留给后续 milestone，避免扩大 M6 OpenAPI 表面积。

token：

```http
GET /api/v1/tokens
POST /api/v1/tokens
DELETE /api/v1/tokens/{token}
```

token endpoint 的权限必须非常保守：

- owner/admin 可创建 token。
- 用户可以列出和 revoke 自己的 token。
- 创建绑定其它 user 的 token 需要 admin/owner。
- 创建 workspace/project scoped token 时，创建者必须有对应 workspace/project 的管理权限。

### 6.5 暂不实现的 endpoint

M6 不做：

- MCP endpoint。
- WebSocket / SSE。
- op-log sync。
- webhook trigger。
- password login。
- refresh token。
- batch mutation。
- 文件上传。
- project restore / hard delete。

## 7. 远程 CLI 范围

### 7.1 必须支持的远程命令

核心 task：

- `add`
- `list`
- `next`
- `all`
- `completed`
- `deleted`
- `waiting`
- `active`
- `ready`
- `overdue`
- `blocked`
- `blocking`
- `info`
- `<target> modify`
- `<target> done`
- `<target> delete`
- `<target> start`
- `<target> stop`
- `<target> annotate`
- `<target> denotate`
- `<target> append`
- `<target> prepend`

project：

- `project list/add/info/modify/archive`
- `project config get/set/unset/list`

context/config/helper：

- `context list/define/show/delete/use/none`
- `config get/set/unset/list`
- `_ids`
- `_uuids`
- `_projects`
- `_tags`
- `_udas`
- `_unique`
- `_urgency`
- `_show`
- `_version`

token/server：

- `token create/list/revoke`
- `server`

远程 helper 实现原则：

- `_ids`、`_uuids`、`_projects`、`_tags`、`_udas`、`_unique`、`_urgency`、`_show`、`_version` 在远程模式下优先由客户端调用已有 task/project/config/me endpoint 后本地后处理输出。
- M6 不新增 `/api/v1/_ids`、`/api/v1/_projects` 这类 helper endpoint，避免扩大 OpenAPI 表面积。
- 如果某个 helper 需要的信息没有现有 endpoint 支撑，应优先补通用 endpoint，而不是新增下划线 API。

### 7.2 M6 不要求远程支持的命令

- `edit`：依赖本地 `$EDITOR`，M6 暂不远程化。
- `completion`：本地生成即可。
- `calc`：可本地执行，不需要远程。
- `.taskrc import`：涉及本地文件读取，M6 暂不远程化；后续可以设计上传式 import。

如果用户在远程模式调用暂不支持命令，返回：

```text
remote_unsupported_command: command "edit" is not supported in remote mode
```

这是 CLI 错误，必须写 stderr，exit code 非 0；stdout 不输出错误文本，保持脚本友好。

### 7.3 target 与 working-set ID

远程 CLI 仍要支持数字 working-set ID：

```bash
xuanchu --server ... list
xuanchu --server ... 1 done
```

规则：

- HTTP path parameter `{uuid}` 只接受真实 UUID，不接受数字 working-set ID。
- 数字 ID 由远程 CLI 通过两跳实现：先调用任务列表 / report endpoint，在服务端 effective workspace、context、token scope 下拿到 numbered working set；再把数字 ID 解析成 UUID，调用对应 mutation endpoint。
- 远程 CLI target 识别规则必须与 HTTP path 规则分开：纯数字按 working-set ID 两跳解析；完整 UUID 可直接调用 task endpoint；UUID prefix 或其它本地支持的非数字 target 必须先由客户端解析成完整 UUID，再调用 task endpoint。HTTP path 永远只接收完整 UUID。
- 远程 CLI 不持久化缓存 working set，不跨命令复用上一次 `list` 结果。
- 同一个 token scope 下，`list` 与后续 `1 done` 的语义应与本地 CLI 一致。
- 如果 token scope 改变或 active context 改变，数字 ID 变化是允许的，行为与本地 CLI 一致。
- M6 不保证 working-set ID 在并发场景下稳定。如果两跳解析之间另一个 client 创建、完成或删除任务，远程 CLI 不重试、不重新解析，直接展示 mutation 的实际错误；这与本地 CLI 的边界一致。
- M6 不新增 `/api/v1/working-set/{n}/...` endpoint。后续如果远程数字 ID 成为性能瓶颈，再单独设计服务端解析 endpoint。

## 8. 配置边界

配置继续分三类：

1. 本机配置：
   - `database.path`
   - `color`
   - `json`
   - `date.format`
   - `remote.server`
   - `remote.token`
2. workspace 业务配置：
   - UDA schema
   - urgency UDA 系数
   - context
   - report 默认配置
3. project 业务配置：
   - `agent.background`
   - `agent.constraints`
   - `context.default`

HTTP 与远程 CLI 规则：

- HTTP 不修改服务端本机 TOML。
- 远程 CLI 的 `config get/set/list` 作用于服务端 workspace 业务配置。
- 远程 CLI 要查看本机连接配置，后续可以加 `config local get remote.server`；M6 不要求实现。
- project 配置仍只能通过 `project config`。
- `remote.server`、`remote.token` 不得通过 HTTP config endpoint 写入服务端 DB。
- 远程 `_show database.path` 不支持，返回 `remote_unsupported_command`，避免向普通 token 暴露服务端文件路径。
- 远程 `_show active.user`、`active.workspace`、`active.context` 可以通过 `/api/v1/me` 和 context endpoint 拼出，不需要新增 `_show` 专用 endpoint。

## 9. 权限与 scope

### 9.0 鉴权与授权顺序

M6 固定每个 HTTP 请求的鉴权与授权流水线：

1. 校验 token 存在、格式正确、未过期、未 revoke。失败返回 401。
2. 校验 token capability scope 覆盖请求所需最低 capability。失败返回 403 `token_scope_denied`。
3. 解析 effective workspace，并校验 workspace 在 token workspace allowlist 内。失败返回 403 `workspace_scope_denied`。
4. 解析 explicit project scope，并校验 project 在 token project allowlist 内。失败返回 403 `project_scope_denied`。单任务读取如果任务存在但 project 不在 allowlist 内，返回 404 `task_not_found`，避免泄露资源存在性。
5. 校验 actor 在 effective workspace 的 membership role 是否允许该 app permission。失败返回 403 `permission_denied`。
6. 调用 app service 执行业务逻辑。

第 1-4 步任何失败都不写 audit，避免 DoS 放大；token 失败时也没有可靠 actor 信息。第 5 步 membership role 失败按 M5 既有行为不写 audit。

空数组语义必须 fail-closed / limit-open 分开处理：

- `scopes_json = []` 表示没有任何 capability，拒绝所有需要鉴权的 API。不存在“空 scope = 全权限”。
- `workspace_ids_json = []` 表示不额外限制 workspace，最终仍受 membership 限制。
- `project_ids_json = []` 表示不额外限制 project，最终仍受 workspace/project 业务规则限制。

这种非对称是刻意设计：capability 是权限，默认拒绝；workspace/project allowlist 是收窄条件，空数组表示不额外收窄。

### 9.1 capability scope

M6 定义这些 token scope：

| Scope | 允许能力 |
|---|---|
| `task:read` | task list/info/report/helper/export/urgency |
| `task:write` | add/modify/done/delete/start/stop/annotate/import |
| `project:read` | project list/info/config read |
| `project:write` | project add/modify/archive/config write |
| `context:read` | context list/show |
| `context:write` | context define/use/delete/none |
| `config:read` | workspace config read |
| `config:write` | workspace config write |
| `workspace:read` | workspace info/list/member list |
| `workspace:write` | workspace/member management |
| `audit:read` | audit list |
| `token:read` | token list |
| `token:write` | token create/revoke |

默认建议：

- PAT 可以由用户显式选择 scope。
- Agent token 默认不包含 `workspace:write`、`token:write`。
- M6 不提供 `admin:*` 通配 scope。需要什么能力就显式列出什么能力，避免后续新增 scope 时旧 token 自动获得新权限。

### 9.2 role 与 capability 的映射

app 层已有 role permission。M6 要在 service 执行前叠加 token capability：

- 读任务需要 app permission + `task:read`。
- 写任务需要 app permission + `task:write`。
- project config 写需要 app permission + `project:write`。
- token 管理需要 app permission + `token:write`。

如果 membership 允许但 token scope 不允许，返回 `token_scope_denied`。
如果 token scope 允许但 membership 不允许，返回现有 `permission_denied`。

### 9.3 project-scoped token 的全局过滤

带 project allowlist 的 token 必须影响：

- `GET /tasks`
- `GET /reports/*`
- `_ids` / `_uuids`
- `_unique`
- `export`
- `import`
- task detail
- task mutation
- urgency explain
- audit list 的 project 查询

规则：

- 读取列表时自动叠加 `project_id IN (...)`。
- 读取单任务时任务 project 不在 allowlist，固定返回 404 `task_not_found`，避免泄露资源存在性。
- 写任务时 project 不在 allowlist，返回 403 `project_scope_denied`。
- 无 project 任务对 project-scoped token 不可见。

## 10. OpenAPI

M6 必须提供 OpenAPI 3 文档。

允许两种实现：

1. 手写 `docs/openapi/xuanchu-v1.yaml`。
2. 用 Go 注释或结构生成，但生成产物仍提交到 `docs/openapi/xuanchu-v1.yaml`。

推荐 M6 先手写，因为 endpoint 范围可控，避免引入过重生成链。

OpenAPI 必须覆盖：

- auth bearer security scheme。
- error envelope。
- task/project/workspace/context/config/token DTO。
- M6 已实现 endpoint。

OpenAPI 不需要覆盖 M7 MCP。

## 11. 错误 code

M6 新增错误 code：

| Code | Message |
|---|---|
| `server_listen_required` | `server listen address is required` |
| `route_not_found` | `route not found` |
| `method_not_allowed` | `method not allowed` |
| `api_internal` | `internal server error` |
| `auth_missing_token` | `authorization bearer token is required` |
| `auth_invalid_token` | `invalid token` |
| `auth_token_expired` | `token expired` |
| `auth_token_revoked` | `token revoked` |
| `token_name_required` | `token name is required` |
| `token_scope_invalid` | `invalid token scope` |
| `token_workspace_scope_invalid` | `invalid token workspace scope` |
| `token_project_scope_invalid` | `invalid token project scope` |
| `token_scope_denied` | `token scope denied` |
| `workspace_scope_denied` | `token cannot access workspace` |
| `project_scope_denied` | `token cannot access project` |
| `workspace_mismatch` | `workspace references do not match` |
| `project_mismatch` | `project references do not match` |
| `task_not_found` | `task not found` |
| `task_uuid_invalid` | `invalid task UUID` |
| `remote_server_required` | `remote server URL is required` |
| `remote_server_invalid` | `remote server URL is invalid` |
| `remote_token_required` | `remote token is required` |
| `remote_unsupported_command` | `command is not supported in remote mode` |
| `api_bad_json` | `invalid JSON request body` |
| `api_limit_too_large` | `limit is too large` |
| `api_payload_too_large` | `request body is too large` |

已有 code 继续复用：

- `workspace_not_found`
- `workspace_archived`
- `membership_not_found`
- `permission_denied`
- `project_not_found`
- `project_archived`
- `project_workspace_mismatch`
- `project_invariant_violation`

## 12. 不进入 M6

- MCP Server。
- MCP tool schema。
- Streamable HTTP / SSE MCP transport。
- 用户密码登录。
- JWT / refresh token。
- OAuth。
- system keychain。
- op-log sync。
- conflict resolution。
- webhook trigger。
- 外部系统 adapter。
- project 成员表。
- project RBAC override。
- SaaS organization/tenant 层。
- cursor pagination。
- bulk mutation。
- 复杂 rate limit。

## 13. 测试策略

### 13.1 单元测试

必须覆盖：

- token 生成格式。
- token hash 不保存 raw token。
- token verify 成功、失败、过期、revoked。
- token scope parser。
- workspace allowlist。
- project allowlist。
- project slug + project_id 一致性校验。
- token audit 不包含 raw token/hash。
- app service 在 token capability scope 下拒绝越权。

### 13.2 HTTP handler 测试

必须覆盖：

- 无 token 返回 401。
- invalid token 返回 401。
- 过期 token 返回 401。
- scope 不足返回 403。
- workspace scope 越界返回 403。
- project scope 越界返回 403 或单资源读取 404。
- task CRUD 通过 API 写入 audit。
- report endpoint 叠加 context/query/project scope。
- project config endpoint 与 workspace config endpoint 不混淆。
- error envelope 稳定。

### 13.3 CLI 集成测试

必须覆盖：

- `xuanchu server` 可启动并响应 `/healthz`。
- `token create/list/revoke`。
- 远程 `list/add/info/modify/done/delete`。
- 远程 `project list/add/info/modify/archive`。
- 远程 `project config get/set/list/unset`。
- 远程 `--workspace` 不能突破 token workspace scope。
- 远程 `--project-id` 不能突破 token project scope。
- 远程 `info <uuid>` 在任务存在但 project 不在 token allowlist 时返回 404 `task_not_found`，不能返回 403，避免泄露资源存在性。
- 远程数字 working-set ID。
- 远程 stdout/stderr 分离。
- `--json` 输出稳定。

### 13.4 必跑验证

每次声称 M6 完成前必须运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

如果引入 HTTP router 依赖，必须确认它不引入 CGO。

## 14. 文档同步

M6 完成后必须更新：

- [README.md](/Users/mac/code/projects/dajee/task/README.md)
  - 增加 server、token、远程 CLI 用法。
  - 明确本机 config 与远程连接 config 区别。
- [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)
  - 标记 M6 已完成。
  - 收紧 M7 为 MCP Server 和 tool schema。
- [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)
  - 更新 M6 实际 endpoint、token scope、远程 CLI 范围。
- [docs/openapi/xuanchu-v1.yaml](/Users/mac/code/projects/dajee/task/docs/openapi/xuanchu-v1.yaml)
  - 覆盖已实现 HTTP API。

## 15. 进入 implementation plan 前的检查

进入 M6 implementation plan 前，应确认：

- M6 没有重新打开 M5 的 project 身份决策。
- API、token、远程 CLI 都以 `project_id` 作为稳定 scope。
- handler 没有复制 app service 业务规则。
- 远程 CLI 暂不支持的命令已经明确列出。
- JWT、MCP、op-log、webhook 没有混进 M6。
- `CGO_ENABLED=0` 仍是硬约束。

## 16. 推荐实施切分

implementation plan 可以按这些 chunk 拆：

1. Token schema、auth service、CLI token 管理。
2. HTTP server skeleton、healthz、auth middleware、error envelope。
3. Request scope 解析与 app service 构造改造。
4. task/report/project/context/config API endpoint。
5. 远程 CLI client 与核心命令远程化。
6. project-scoped token 的端到端越权测试。
7. OpenAPI 与 README/ROADMAP/requirements 同步。

每个 chunk 都应先写测试，再写实现，并保持小提交。OpenAPI 应在每个 endpoint chunk 内同步更新，不要堆到最后一次性补。
