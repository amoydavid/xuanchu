# Xuanchu Server Admin Bootstrap 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 为 Xuanchu Server 增加一个受配置文件控制的 server admin bootstrap 能力，让全新部署或无人值守部署可以通过 REST 创建 workspace，并为 workspace 创建受限 agent token。

**范围策略：** 本规格只设计 server control-plane bootstrap。admin token 不是普通用户 token，不写入 `api_tokens`，不拥有业务 scope，也不能访问普通业务 API。它只能访问 `/api/v1/admin/*` 下显式暴露的少量 bootstrap endpoint。

---

## 1. 背景与问题

Xuanchu 当前已经支持：

- 本地 CLI 创建 user、workspace、membership、token。
- HTTP/JSON API 与 Remote CLI。
- 数据库内 `api_tokens`，包括 PAT 和 Agent token。
- workspace / project / member / token scope 隔离。
- Agent token 和 request-scoped impersonation。

当前缺口：

- 新 server 部署后，如果只开放 REST，不方便远程创建第一个业务 workspace。
- 创建 workspace 级 agent token 目前依赖已有 workspace owner/admin token；全新 workspace 初始化会出现 bootstrap 鸡生蛋问题。
- 如果直接把普通 token 做成“全局超管 token”，会破坏现有 workspace/member/token scope 模型。

因此需要一个更小、更清晰的控制面入口：server admin bootstrap token。

## 2. 设计目标

首版完成后，应支持：

```bash
# 生成一组 admin token 和 hash
xuanchu admin token generate

# 或只对已有 token 生成 hash
xuanchu admin token hash
```

服务端配置：

```toml
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:..."
enabled = true

[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = true
```

远程调用：

```http
POST /api/v1/admin/workspaces
Authorization: Bearer <server-admin-token>
Content-Type: application/json

{
  "slug": "dajee",
  "name": "Dajee",
  "visibility": "team",
  "owner": {
    "name": "alice",
    "email": "alice@example.com"
  }
}
```

```http
POST /api/v1/admin/workspaces/dajee/agent-tokens
Authorization: Bearer <server-admin-token>
Content-Type: application/json

{
  "name": "openclaw-agent",
  "user": "alice@example.com",
  "scopes": ["task:read", "task:write", "project:read"],
  "project_refs": [],
  "expires_in": "2160h"
}
```

核心目标：

- 允许通过 REST 创建新的 workspace。
- 允许通过 REST 为指定 workspace 创建 workspace 级 agent token。
- admin token 的校验材料可持久化，重启后不丢失。
- 不在 Xuanchu 配置文件里长期保存 admin token 明文。
- 支持多个 admin token verifier，方便轮换。
- admin token 只用于 `/api/v1/admin/*`。
- 普通 API token 不能访问 admin endpoint。
- admin token 不能访问普通业务 endpoint。
- 所有 admin 操作可审计。

## 3. 非目标

本规格不做：

- 不创建“超管用户”。
- 不让 admin token 进入 `api_tokens` 表。
- 不给 admin token 增加 `task:*`、`project:*`、`config:*`、`token:*` 等普通 scope。
- 不允许 admin token 直接读写任务、项目、通知、hook、业务配置。
- 不提供 Web UI。
- 不做 OAuth、OIDC、SAML、LDAP 等企业 SSO。
- 不做细粒度 admin RBAC。首版只有配置文件里的 server admin token。
- 不支持 query string token、cookie token、Basic Auth。
- 不默认启用 admin bootstrap。
- 不在 server 启动时自动打印生产可用的 admin token。

## 4. 核心产品决策

### 4.1 admin token 是 control-plane bootstrap token

admin token 只负责创建边界和发放受限凭证：

- 创建 workspace。
- 创建 workspace owner membership。
- 创建 workspace 级 agent token。

它不负责进入 workspace 内操作业务数据。进入 workspace 后，仍然必须使用普通数据库 token，并受 workspace/member/scope/project 限制。

原因：

- Xuanchu 当前权限模型的核心是 workspace/member/token scope。
- 把 admin token 做成万能业务 token 会绕过模型，后续 audit 和权限排查会变得混乱。
- bootstrap 能力应该尽量短路径、窄权限、低频使用。

### 4.2 配置持久化 verifier，不持久化明文

推荐配置保存 token hash，而不是 token 明文：

```toml
[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:..."
enabled = true
```

重启后 hash 仍在，用户继续使用原明文 token 发请求，服务端对请求 token 计算 hash 后常量时间比较。这个模型和密码 hash 一样：重启不影响可用性，配置泄露也不会直接泄露明文 token。

如果明文 token 丢失，不能从 hash 反推。恢复方式是生成新 token/hash，加入配置，重启或 reload 后使用新 token，再删除旧 hash。

### 4.3 表数组优先于 `token_hashes`

不采用：

```toml
token_hashes = ["sha256:..."]
```

原因是它很快无法承载运维信息。推荐使用表数组：

```toml
[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:..."
enabled = true
description = "primary ops bootstrap token"

[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = true
```

好处：

- 可以命名。
- 可以单独启停。
- 可以轮换。
- audit 可以记录 token name。
- 后续可扩展 `expires_at`、`allowed_cidrs`、`description` 等字段。

### 4.4 admin endpoint 和普通 endpoint 必须分离

Admin endpoint 放在：

```text
/api/v1/admin/*
```

普通 `authMiddleware` 不应直接接受 admin token。应新增 admin 专用 middleware：

- `adminAuthMiddleware` 只挂载 `/api/v1/admin/*`。
- 普通 `authMiddleware` 继续只校验数据库 token。
- admin token 请求普通 API 返回 `auth_invalid_token` 或普通未授权错误。
- 普通 API token 请求 admin API 返回 `admin_auth_invalid` 或 `admin_auth_required`。

这能避免 admin token 和业务 token 的边界混淆。

## 5. 配置设计

### 5.1 TOML 配置

新增 server admin 配置：

```toml
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:..."
enabled = true
description = "primary bootstrap token"

[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = true
description = "rotation token hash injected by deploy system"
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `server.admin.enabled` | bool | 否 | 默认 false |
| `server.admin.tokens[].name` | string | 是 | token 名称，用于日志和 audit，不要求保密 |
| `server.admin.tokens[].hash` | string | 否 | token hash，格式见 5.2 |
| `server.admin.tokens[].hash_env` | string | 否 | 从环境变量读取 token hash |
| `server.admin.tokens[].enabled` | bool | 否 | 默认 true |
| `server.admin.tokens[].description` | string | 否 | 运维备注 |

约束：

- `enabled = false` 时忽略所有 admin token。
- 每个 enabled token 必须提供且只能提供 `hash` 或 `hash_env`。
- token `name` 在配置内必须唯一。
- `hash_env` 读取不到值时，server 启动失败，而不是静默跳过。
- `hash` / `hash_env` 中的值不进入日志。

### 5.2 Hash 格式

首版支持：

```text
sha256:<hex-encoded-sha256>
```

生成规则：

```text
sha256(admin token raw bytes)
```

比较规则：

- 解析 `sha256:` 前缀。
- 对请求 token 计算 sha256。
- 使用常量时间比较。

后续如需更强 verifier，可以扩展：

```text
sha256:v1:<salt>:<hash>
argon2id:...
```

首版不引入新密码 KDF 依赖，避免把实现复杂度推高。admin token 本身必须由高熵随机数生成。

### 5.3 明文 token 的处理

生产默认不支持在 TOML 里写：

```toml
token = "plain-admin-token"
```

如果为了开发便利要支持，应满足：

```toml
[server.admin]
enabled = true
allow_plaintext_token = true

[[server.admin.tokens]]
name = "dev"
token = "dev-only-token"
enabled = true
```

约束：

- `allow_plaintext_token` 默认 false。
- 使用明文 token 时 server 启动必须向 stderr/log 打 warning。
- `config.example.toml` 不展示生产明文写法，只在注释里说明 dev-only。

## 6. REST API 设计

### 6.1 创建 workspace

```http
POST /api/v1/admin/workspaces
Authorization: Bearer <server-admin-token>
Content-Type: application/json
```

请求：

```json
{
  "slug": "dajee",
  "name": "Dajee",
  "description": "Dajee workspace",
  "visibility": "team",
  "owner": {
    "name": "alice",
    "email": "alice@example.com"
  }
}
```

字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `slug` | string | 是 | workspace slug，沿用现有 workspace slug 规则 |
| `name` | string | 否 | workspace 名称，默认使用 slug |
| `description` | string | 否 | workspace 描述 |
| `visibility` | string | 否 | 默认 private，可用值沿用现有 workspace 能力 |
| `owner.name` | string | 是 | owner 用户名 |
| `owner.email` | string | 否 | owner email |

行为：

1. 校验 admin token。
2. 校验 workspace slug/name/visibility。
3. 查找或创建 owner user。
4. 创建 workspace。
5. 添加 owner membership。
6. 初始化 workspace 内置 config schema。
7. 写 audit。

响应：

```json
{
  "data": {
    "workspace": {
      "id": "workspace-uuid",
      "slug": "dajee",
      "name": "Dajee",
      "visibility": "team"
    },
    "owner": {
      "id": "user-uuid",
      "name": "alice",
      "email": "alice@example.com"
    }
  }
}
```

幂等策略：

- 首版不做通用 idempotency store。
- 如果 workspace slug 已存在，返回 `admin_workspace_exists`。
- 后续可以支持 `Idempotency-Key`。

### 6.2 创建 workspace 级 agent token

```http
POST /api/v1/admin/workspaces/{workspace}/agent-tokens
Authorization: Bearer <server-admin-token>
Content-Type: application/json
```

请求：

```json
{
  "name": "openclaw-agent",
  "user": "alice@example.com",
  "scopes": ["task:read", "task:write", "project:read", "config:read"],
  "project_refs": [],
  "expires_in": "2160h"
}
```

字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | token 名称 |
| `user` | string | 否 | token 归属 user；省略时使用 workspace owner |
| `scopes` | string[] | 是 | token scopes，必须是现有合法 scope |
| `project_refs` | string[] | 否 | 限定项目；为空表示 workspace 级 |
| `expires_in` | string | 否 | Go `time.ParseDuration` 格式 |

行为：

1. 校验 admin token。
2. 解析目标 workspace。
3. 解析 token 归属 user。
4. 如果省略 `user`，使用 workspace owner；如果 workspace 没有 owner，返回 `admin_owner_required`。
5. 校验 user 是目标 workspace 成员。
6. 调用现有 token 创建逻辑创建 `type = agent` token。
7. 强制 `workspace_ids` 只包含目标 workspace。
8. `project_refs` 必须属于目标 workspace。
9. raw token 只在响应中返回一次。
10. 写 audit。

响应：

```json
{
  "data": {
    "token": "xuanchu_agent_...",
    "token_info": {
      "id": "token-uuid",
      "name": "openclaw-agent",
      "type": "agent",
      "user": {
        "id": "user-uuid",
        "name": "alice",
        "email": "alice@example.com"
      },
      "workspace_ids": ["workspace-uuid"],
      "project_ids": [],
      "scopes": ["task:read", "task:write", "project:read", "config:read"],
      "expires_at": 1780000000
    }
  }
}
```

约束：

- 不允许创建 PAT，只允许创建 `type = agent`。
- 不允许 workspace_ids 跨多个 workspace。
- 不允许 project_refs 指向其它 workspace。
- 不允许创建空 scopes token。
- scopes 仍复用现有 `auth.ValidateTokenCreate`。
- 如果请求 `impersonate` scope，沿用现有 agent token 规则；是否允许由实施计划根据当前权限模型明确。

## 7. Token 归属与审计

### 7.1 为什么 agent token 必须归属 user

现有 token 模型中 `user_id` 是核心字段，影响：

- audit actor。
- membership 检查。
- impersonation 边界。
- token list/revoke。

因此 admin 创建 agent token 时不绕过 user。

推荐规则：

- 请求指定 `user`：token 归属该 user。
- 请求不指定 `user`：token 归属 workspace owner。
- 如果没有可解析 owner，返回 `admin_owner_required`。
- 不新增 system user，除非以后有专门系统身份模型。

### 7.2 Audit 表达

admin 操作必须写 audit，但 audit 当前 actor 以 user 为主。首版有两个可选实现：

**方案 A：扩 audit actor 类型。**

新增字段：

- `actor_type`: `user` / `server_admin`
- `actor_label`: admin token name

优点是语义最清晰。缺点是会动 audit 输出、HTTP、MCP、CLI 多处结构。

**方案 B：沿用现有 actor 字段，payload 标记 admin。**

不扩表，在 audit payload 中写：

```json
{
  "admin": true,
  "admin_token_name": "ops-primary",
  "admin_action": "workspace.create"
}
```

优点是改动小。缺点是 actor 语义不够干净。

本规格推荐方案 A；若实施计划希望先降低改动，可以在首版采用方案 B，但必须在 spec 或 plan 中明确这是过渡实现。

Audit action：

- `admin.workspace.create`
- `admin.agent_token.create`

Audit payload 不得包含 raw token。

## 8. 鉴权与错误处理

### 8.1 Header

只接受：

```http
Authorization: Bearer <server-admin-token>
```

不接受：

- query string token
- cookie token
- Basic Auth
- 自定义 `X-Admin-Token`

原因是保持和现有 API token 入口一致，同时避免多套泄露路径。

### 8.2 错误码

新增错误码：

| Code | HTTP | 说明 |
|---|---:|---|
| `route_not_found` | 404 | admin bootstrap 未启用时隐藏控制面路由 |
| `admin_auth_required` | 401 | 缺少 Bearer token |
| `admin_auth_invalid` | 401 | token 无效 |
| `admin_workspace_exists` | 409 | workspace 已存在 |
| `admin_owner_required` | 400 | 缺少 owner 或无法推断 owner |
| `admin_owner_invalid` | 400 | owner 无法解析或冲突 |
| `token_scope_invalid` | 400 | scope 非法 |
| `token_project_scope_invalid` | 400 | project 不属于目标 workspace |

配置错误不作为 HTTP 错误码暴露：enabled token 缺少 `hash`/`hash_env`、同时配置二者、`hash_env` 为空、hash 格式不合法、token name 重复等情况，应在 server 启动或配置解析阶段失败。

安全策略：

- auth 失败不要暴露是 name 不存在、hash 不匹配还是 disabled。
- 日志只记录 request id、admin token name（匹配成功时）、endpoint、状态码。
- 失败请求不记录 raw token。

## 9. 模块设计

### 9.1 `internal/config`

新增配置字段：

- `ServerAdminEnabled bool`
- `ServerAdminTokens []AdminTokenConfig`

`AdminTokenConfig` 包含：

- `Name`
- `Hash`
- `HashEnv`
- `Enabled`
- `Description`

解析来源：

- TOML `[server.admin]`
- `[[server.admin.tokens]]`
- 环境变量只作为 `hash_env` 指向，不建议新增一堆平铺 env key。

### 9.2 `internal/auth`

新增 admin token helper：

- `GenerateAdminToken()`
- `HashAdminToken(raw string) string`
- `VerifyAdminToken(raw string, verifier AdminTokenVerifier) bool`

要求：

- token 使用高熵随机数。
- hash 使用 `sha256:<hex>`。
- verify 使用常量时间比较。

### 9.3 `internal/httpapi`

新增：

- `adminAuthMiddleware`
- `adminAuthContext`
- `handleAdminWorkspaceCreate`
- `handleAdminAgentTokenCreate`

路由：

```go
api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces", s.handleAdminWorkspaceCreate)
api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces/{workspace}/agent-tokens", s.handleAdminAgentTokenCreate)
```

`Server.Options` 需要接收 admin 配置，或者从已解析 runtime config 注入。

### 9.4 `internal/app`

新增 admin service 方法，复用已有能力：

- `AdminCreateWorkspace(input AdminCreateWorkspaceInput) (AdminWorkspaceCreateResult, error)`
- `AdminCreateWorkspaceAgentToken(input AdminCreateAgentTokenInput) (CreatedToken, error)`

关键点：

- 不复制 workspace/token 创建逻辑。
- 尽量复用 `AddWorkspace`、`AddUser`、membership 和 `CreateToken` 的底层方法。
- 如果现有 service 入口强依赖 runtime user/role，应提取更底层的 locked helper，而不是伪造一个普通超管 user。
- admin 控制面和 HTTP/MCP 内部鉴权使用空 runtime 时，必须禁用普通 workspace scope bootstrap，避免写入 `workspace_id=""` 的配置定义。

### 9.5 `cmd/xuanchu` / `internal/cli`

新增本地辅助命令：

```bash
xuanchu admin token generate
xuanchu admin token hash
```

`generate` 输出：

```text
token: <raw-token>
hash: sha256:<hash>
```

要求：

- raw token 只输出到 stdout。
- 不写数据库。
- 不修改配置文件。

`hash` 可以从 stdin 读 token，避免 shell history：

```bash
printf '%s' "$TOKEN" | xuanchu admin token hash
```

## 10. 配置示例

更新 `config.example.toml`：

```toml
[server.admin]
enabled = false

# 生产建议使用 hash 或 hash_env，不要长期保存明文 token。
# 生成方式：
#   xuanchu admin token generate

[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:replace-with-token-hash"
enabled = false
description = "server bootstrap token"

[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = false
description = "rotation token hash from deployment secret"
```

## 11. OpenAPI 与文档

需要更新：

- `README.md`
- `config.example.toml`
- `docs/openapi/xuanchu-v1.yaml`
- `docs/deployment.md`
- `docs/skills/token-management/SKILL.md` 如涉及 Agent 使用说明

文档必须明确：

- admin token 是 bootstrap/control-plane token。
- admin token 不可用于普通 API。
- 普通 API token 不可用于 admin API。
- 配置保存 hash 后，server 重启不会丢 token。
- 明文丢失无法恢复，只能轮换。

## 12. 测试要求

### 12.1 Config / Auth

- 解析 `[server.admin]`。
- 解析多个 `[[server.admin.tokens]]`。
- `hash` 和 `hash_env` 互斥。
- enabled token 缺 hash/hash_env 报错。
- 重复 name 报错。
- hash_env 缺环境变量报错。
- admin token hash/verify 成功。
- 错误 token verify 失败。

### 12.2 HTTP

- admin disabled 时 `/api/v1/admin/workspaces` 不可用。
- 无 Authorization 返回 `admin_auth_required`。
- 错误 token 返回 `admin_auth_invalid`。
- 正确 admin token 可以创建 workspace。
- 普通 API token 不能访问 admin endpoint。
- admin token 不能访问普通 `/api/v1/tasks`。
- 创建 workspace 后 owner membership 存在。
- 重复 workspace 返回 `admin_workspace_exists`。
- 创建 agent token 后 raw token 只在响应出现。
- 创建出的 agent token 可以访问目标 workspace。
- 创建出的 agent token 不能访问其它 workspace。
- project_refs 跨 workspace 被拒绝。

### 12.3 Audit

- 创建 workspace 写 `admin.workspace.create`。
- 创建 agent token 写 `admin.agent_token.create`。
- audit payload 不包含 raw token。
- audit 可定位 admin token name。

### 12.4 CLI

- `xuanchu admin token generate` 输出 raw token 和 hash。
- `xuanchu admin token hash` 可从 stdin 读取。
- 生成 hash 可被 server admin auth 成功验证。

### 12.5 验证命令

完成前必须运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 13. 安全注意事项

- admin token 等价于创建 workspace 和发放 agent token 的能力，必须视为高危密钥。
- 生产环境必须使用 HTTPS 或可信内网反代。
- 不允许把 admin token 放入 URL。
- 不允许在 access log 中输出 Authorization header。
- `config.example.toml` 必须默认 disabled。
- 文档要建议配置文件权限为 `0600`。
- 如果使用 `hash_env`，部署系统必须保证环境变量不被普通用户读取。
- 不做自动创建生产 admin token，避免日志泄露。

## 14. 未来扩展

后续可以考虑：

- `expires_at`：admin token verifier 过期时间。
- `allowed_cidrs`：限制来源 IP。
- `reload`：无需重启重新加载 admin token 配置。
- `GET /api/v1/admin/status`：只返回 admin bootstrap 状态，不返回 hash。
- `Idempotency-Key`：防止 workspace/token 重复创建。
- `POST /api/v1/admin/workspaces/{workspace}/owners`：独立 owner 管理。
- 企业 SSO / OIDC bootstrap，但不属于本规格首版。

## 15. 验收标准

该能力完成时应满足：

- `config.example.toml` 能表达 disabled admin bootstrap 示例。
- 配置 hash 后，server 重启不会丢失 admin token 校验能力。
- REST 可以用 admin token 创建 workspace。
- REST 可以用 admin token 为指定 workspace 创建 agent token。
- admin token 不能访问普通 API。
- 普通 API token 不能访问 admin API。
- 创建出的 agent token 继续走现有 token/auth/scope/membership 模型。
- 所有 admin 操作有 audit 记录，且不泄露 raw token。
- SQLite 和 PostgreSQL 均可用。
- 完整验证命令通过。
