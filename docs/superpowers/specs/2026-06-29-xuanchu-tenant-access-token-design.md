# Xuanchu Tenant Access Token 设计

> **给编码代理的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-06-29
**状态：** 草案
**背景需求：** Xuanchu 未来会支持 OIDC / OAuth，但第一阶段不需要完整 OAuth token server。当前更急需的是类似 OpenAI API key 的租户级访问凭证：一个 workspace 可以创建多个 `tenant_access_token`，用于外部 Agent、自动化、LLM runtime、HTTP API 和 HTTP MCP。token 可以按 scope / project allowlist 收窄，可以过期和吊销，明文只展示一次。

## 1. 背景与现状

### 1.1 当前 token 类型

当前普通 API token 只有两类：

- `pat`：个人访问 token，raw token 前缀为 `xuanchu_pat_`。
- `agent`：Agent / 自动化 token，raw token 前缀为 `xuanchu_agent_`。

两者都存储在 `api_tokens` 表中，并且都必须绑定 `user_id`。HTTP API、HTTP MCP 和 remote CLI 统一走 Bearer token 鉴权，再由 `AuthorizeTokenRequest` 计算：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

这套模型适合“某个用户签发的个人或 Agent token”，但不适合表达“租户本身签发的 API key”。关键问题不在 `api_tokens` 这张表本身，而在当前表结构把所有 token 都强制绑定 `user_id`。P1 应复用 `api_tokens` 作为统一 token 表，但把 `user_id` 改为 nullable，并用 `type=tenant_access_token` 明确区分机器凭证。不要创建假用户或服务用户，否则后续审计、成员权限、`assignee:me`、impersonation 都会变得含糊。

### 1.2 当前 Web Console 资产

普通 Workspace Console 已有 `/tokens` 页面，支持 PAT / Agent token 创建、编辑、吊销，包含：

- 列表页：name、type、prefix、scope 数量、状态、last_used、操作菜单。
- 创建 / 编辑 Dialog：name、type、workspace、scopes、expires、project allowlist。
- `ScopeEditor`：按资源分组勾选 scope。
- 创建成功后一次性展示 raw token。

Admin Console 已有 `/admin/tokens` 页面，支持跨 workspace 查看和管控普通 PAT / Agent token：

- 列表页：name、type、user、workspace_ids、scope 数量、状态、last_used、操作菜单。
- 编辑：name、scope、expires。
- 吊销：server admin 可跨 workspace revoke。

本设计应复用这些现有 UI 组件和交互，不重新做一套 token 管理体验。

### 1.3 OIDC / OAuth 的关系

`tenant_access_token` 第一版不是 OAuth access token，也不是 OIDC user token。它是 Xuanchu 自己签发和校验的租户 API key。未来可以继续新增：

- `user_access_token`：面向 Xuanchu API 的 OAuth access token，由 OIDC / OAuth 授权流程签发。
- `browser_session`：Web Console 的浏览器登录态。
- `client_credentials`：标准 OAuth client credentials flow。

本规格只解决租户 API key，不引入 OAuth 授权码、refresh token、用户 consent 或 OIDC provider。

## 2. 目标

1. 新增一等 `tenant_access_token`，raw token 前缀为 `xuanchu_tenant_`。
2. `tenant_access_token` 绑定一个 workspace。当前 Xuanchu 以 workspace 作为企业 / 租户隔离边界，因此第一版 tenant = workspace。
3. 一个 workspace 可创建多个 `tenant_access_token`。
4. token 明文只在创建时返回一次，数据库只保存 hash 和短 prefix。
5. token 可设置 name、scope、project allowlist、expires_at，可吊销，可更新 last_used_at。
6. HTTP API 和 HTTP MCP 都接受 `Authorization: Bearer xuanchu_tenant_...`。
7. 授权不依赖普通 user membership role，而是基于 token 自身的 workspace、scope、project allowlist。
8. 审计和访问日志能清晰表达 actor 是 tenant token，而不是普通用户。
9. Web Console 在现有 token 管理页面中增加租户 token 管理入口和 ASCII 原型所示交互。

## 3. 非目标

- 不实现 OIDC / OAuth `user_access_token`。
- 不实现 OAuth client credentials flow。
- 不实现 refresh token。
- 不让 `tenant_access_token` 代用户执行操作。
- 不支持 `X-Xuanchu-As` impersonation。
- 不把 tenant token 绑定到伪造 user。
- 不用 tenant token 登录 Web Console。
- 不让 tenant token 管理普通 PAT / Agent token。
- 不实现 token 轮换 / 双 token 平滑切换；第一版只支持新建 + 吊销。
- 不实现 per-token rate limit；后续 P2 再考虑。

## 4. 核心语义

### 4.1 凭证类型

新增凭证类型：

```text
tenant_access_token
```

raw token：

```text
xuanchu_tenant_<random>
```

认证后得到的授权上下文：

```text
credential.kind = tenant_access_token
credential.token_id = <tenant_token_id>
credential.token_name = <tenant_token_name>
tenant.workspace_id = <bound_workspace_id>
principal.type = tenant_token
principal.user_id = null
```

### 4.2 授权公式

PAT / Agent token 继续使用现有公式：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

tenant token 使用新的公式：

```text
tenant token capability scope ∩ tenant workspace binding ∩ tenant project allowlist
```

也就是说：

- workspace 由 token 绑定值决定。
- token 不能跨 workspace。
- project allowlist 为空表示该 workspace 内所有 project；非空表示只允许这些 project。
- 不读取 `memberships`，也不把 token 映射成 owner/admin/member/viewer。

### 4.3 允许与禁止的能力

第一版允许 tenant token 调用偏机器到机器的能力：

- task read/write
- project read/write
- project config read/write
- context read/write（仅 workspace 级上下文定义，不涉及个人 active context）
- config read/write
- config schema read/write
- audit read
- hook read/write（P1 可读、修改、删除已有资源；创建 hook 仍需要用户 actor）
- notification read/write（P1 可读、修改、删除已有资源；创建 sink / 事件通知规则仍需要用户 actor）
- reminder read/write（P1 可读、修改、删除已有资源；创建 reminder rule 仍需要用户 actor）
- HTTP MCP tool 调用中上述同类能力

第一版禁止 tenant token 调用需要个人身份或成员语义的能力：

- `/api/v1/me` 的个人用户语义；P1 不新增 `/api/v1/credentials/current`，tenant token 调用 `/api/v1/me` 固定返回 `tenant_actor_not_user`。
- user create/list/info/external-id 变更。
- member add/role/list 管理。
- PAT / Agent token 管理。
- tenant token 自管理；管理必须由 workspace owner/admin 或 server admin 完成。
- `X-Xuanchu-As` impersonation。
- `assignee:me` 查询或写入。
- 新建带 `created_by` / `actor` 用户语义的对象：task link、project annotation、hook、notification sink、reminder rule、event notification rule。P1 不把 tenant token 伪装成用户；这些路径返回 `tenant_actor_not_user`。tenant token 可以转移 project 状态，但不会写入自动状态变更 annotation。
- tenant token 触发的 task / project 事件不会生成 Hook delivery 或 event notification delivery，因为当前 delivery actor schema 仍是用户形态。后续如果要支持，应先把这些业务表扩展为 `actor_type + actor_token` 模型。
- 任何依赖当前 active user / active workspace 的本地 CLI 语义。

### 4.4 Tenant scope 白名单

tenant token 不能直接复用普通 token 的 wildcard 展开结果。P1 必须新增 tenant 专用 scope 过滤：

```text
tenant_allowed_scopes =
  task:read task:write
  project:read project:write
  context:read context:write
  config:read config:write
  workspace:read
  audit:read
  hook:read hook:write
  notification:read notification:write
  reminder:read reminder:write
```

规则：

- 创建或修改 tenant token 时，`*` 只展开为 `tenant_allowed_scopes`。
- `workspace:*` 只保留 `workspace:read`，不授予 `workspace:write`。
- `*:read`、`*:write` 按 `tenant_allowed_scopes` 过滤展开；如果展开结果为空则返回 `tenant_token_scope_invalid`。
- `user:*`、`member:*`、`token:*`、`impersonate`、`workspace:write` 在 tenant token 创建 / 修改时直接返回 `tenant_token_scope_invalid`。
- HTTP MCP 在 tool 调用前必须同时检查 scope 和 tool 级禁止清单，不能只依赖普通 scope。

P1 禁止的 MCP tool：

```text
me_get
user_list user_get user_bind user_unbind user_add user_use user_list_external_ids
member_list member_add member_role
token_list token_create token_modify token_revoke
context_set context_none
workspace_add workspace_modify workspace_archive workspace_use
```

P1 允许的 workspace MCP tool 只有：

```text
workspace_get_current
workspace_info
```

`workspace_list` 在 P1 禁止，因为 tenant token 只绑定单一 workspace，列出“可见 workspace”容易把它误读成用户可见性语义。

context 规则：

- tenant token 可以读取 workspace context 定义：HTTP `GET /api/v1/contexts`、`GET /api/v1/contexts/{name}`，MCP `context_list`、`context_get` 指定 `name` 时。
- tenant token 可以管理 workspace context 定义：HTTP `POST /api/v1/contexts`、`DELETE /api/v1/contexts/{name}`，MCP `context_delete`。
- tenant token 不能读写个人 active context：HTTP `POST /api/v1/contexts/{name}/use`、`POST /api/v1/contexts/none`，MCP `context_set`、`context_none`。
- tenant token 调用未指定 `name` 的 MCP `context_get` 或读取当前 context MCP resource 时返回 `tenant_actor_not_user`，因为该语义依赖 `ActorUserID + WorkspaceID`。

### 4.5 审计语义

tenant token 触发的操作必须能和用户操作区分：

```json
{
  "actor_type": "tenant_access_token",
  "actor_user": null,
  "actor_token": {
    "id": "tok_...",
    "name": "runtime-prod",
    "prefix": "xuanchu_tenant_x"
  },
  "workspace_id": "ws_...",
  "project_id": "proj_..."
}
```

现有 `actor_user_id` 字段可以为空或保留兼容输出，但新的对外 JSON 不应只用空 user 表达机器 actor。后续所有审计、HTTP access log、MCP tool log 都应包含 `actor_type`。

## 5. 数据模型

P1 不新增独立 `tenant_access_tokens` 表，复用现有 `api_tokens` 作为统一 token 表：

```text
api_tokens
```

现有字段大部分可以直接承载 tenant token：

```text
id                 string primary key
user_id            string nullable index
name               string not null
type               string not null
token_prefix       string not null unique
token_hash         string not null
scopes_json        string not null default '[]'
workspace_ids_json string not null default '[]'
project_ids_json   string not null default '[]'
created_at         int64 not null
expires_at         int64 nullable
revoked_at         int64 nullable
last_used_at       int64 nullable
```

说明：

- `type = pat | agent | tenant_access_token`。
- `user_id` 对 `pat` / `agent` 必须非空；对 `tenant_access_token` 必须为空。
- tenant token 的租户边界写入 `workspace_ids_json`，P1 必须且只能包含一个 workspace id。
- tenant token 的 project allowlist 继续写入 `project_ids_json`，为空表示该 workspace 下所有 project。
- `token_hash` 继续使用 SHA-256 verifier，明文不可恢复。
- `token_prefix` 用于列表和识别，不泄露完整 secret。
- SQLite / PostgreSQL 都需要 migration：把 `api_tokens.user_id` 改为 nullable，并补充应用层校验。
- 不新增 `created_by_user_id` 字段；P1 用审计 action `tenant_token.create` 记录创建来源。列表页的 `created_by` 不是 P1 字段。

应用层必须维护的约束：

```text
type in (pat, agent)              => user_id not null
type = tenant_access_token        => user_id is null
type = tenant_access_token        => len(workspace_ids_json) = 1
type = tenant_access_token        => token_prefix starts with xuanchu_tenant_
type in (pat, agent)              => token_prefix starts with xuanchu_pat_ / xuanchu_agent_
```

如果后续需要更强数据库级约束，可在 PostgreSQL 使用 CHECK constraint；SQLite P1 先由 migration + repository/service 测试保证。

### 5.1 审计表扩展

P1 必须扩展 `audit_logs`，不能只把机器 actor 写进自由 JSON payload。新增 nullable 字段：

```text
actor_type          string nullable index
actor_token_id      string nullable index
actor_token_name    string nullable
actor_token_prefix  string nullable
```

规则：

- 普通用户路径：`actor_type = user`，`actor_user_id` 继续写当前用户 id，`actor_token_*` 为空。
- tenant token 路径：`actor_type = tenant_access_token`，`actor_user_id = null`，`actor_token_*` 写 tenant token 信息。
- admin acting 路径：保持现有 `admin_acting_session_id`、`delegator_admin_token_id/name` 字段；普通业务 actor 仍是 acting session 绑定用户，不改成 tenant token。
- 旧数据 `actor_type` 为空时，读取层按 `actor_user_id != null` 推断为 `user`，保证兼容。
- `payload_json` 可以继续记录 action 细节，但不能作为 actor 类型和 token id 的唯一来源。

## 6. API 设计

### 6.1 Workspace API

由 workspace owner/admin 使用普通 PAT / Agent token 管理当前 workspace 的 tenant token。

```text
GET    /api/v1/tenant-access-tokens
POST   /api/v1/tenant-access-tokens
PATCH  /api/v1/tenant-access-tokens/{tokenRef}
DELETE /api/v1/tenant-access-tokens/{tokenRef}
```

管理 API 认证规则：

- 只接受普通 `pat` / `agent` token。
- 调用者必须是目标 workspace 的 `owner` 或 `admin`。
- 调用 token 必须具备 `token:write` 才能创建、修改、吊销，具备 `token:read` 才能列表。
- 不接受 `tenant_access_token` 自管理。
- 不接受 server admin token；server admin 走 `/api/v1/admin/*`。
- P1 不接受 `admin_acting` token。acting mode 是浏览器临时委托，不应借此签发长期租户 API key。

创建请求：

```json
{
  "name": "runtime-prod",
  "scopes": ["task:read", "task:write", "project:read"],
  "projects": ["agentapi"],
  "expires_in_seconds": 7776000
}
```

创建响应：

```json
{
  "data": {
    "token": "xuanchu_tenant_...",
    "id": "tok_...",
    "prefix": "xuanchu_tenant_x",
    "name": "runtime-prod",
    "type": "tenant_access_token",
    "workspace_id": "ws_...",
    "project_ids": ["proj_..."],
    "scopes": ["task:read", "task:write", "project:read"],
    "created_at": 1782720000,
    "expires_at": 1790496000
  }
}
```

列表/修改/吊销响应不返回 raw token。

### 6.2 Admin API

server admin 可跨 workspace 管控 tenant token：

```text
GET    /api/v1/admin/tenant-access-tokens?all=true
PATCH  /api/v1/admin/tenant-access-tokens/{tokenRef}
DELETE /api/v1/admin/tenant-access-tokens/{tokenRef}
```

第一版 admin 不创建 tenant token。创建应在 workspace 视角完成，避免跨 workspace 创建时的产品语义不清。后续如果需要，可在 `/admin/workspaces/{workspace}/tenant-access-tokens` 增加创建入口。

Admin API 认证规则：

- 只接受 server admin token。
- P1 只支持 list / edit / revoke。
- 不接受 `admin_acting` token。
- admin 修改和吊销必须写审计 payload，记录 server admin token id/name；不生成长期 acting trace。

### 6.3 Resource API / MCP 使用

调用方式：

```http
Authorization: Bearer xuanchu_tenant_xxx
X-Xuanchu-Workspace: ignored-or-must-match
```

规则：

- 如果请求指定 workspace，则必须等于 token 绑定 workspace，否则返回 `workspace_scope_denied`。
- 如果请求未指定 workspace，则使用 token 绑定 workspace。
- 如果请求指定 project，则 project 必须属于 token workspace 且满足 allowlist。
- `/mcp` 接受 tenant token，但拒绝 acting token 的逻辑保持不变。
- `/api/v1/me` 和 MCP `me_get` 对 tenant token 返回 `tenant_actor_not_user`。

## 7. Web Console 设计

### 7.1 普通 Workspace Console `/tokens`

在现有 `/tokens` 页面增加两个 tab：

```text
User / Agent Tokens
Tenant Access Tokens
```

或者在左侧资源导航下继续同页分段。推荐 tab，因为两类 token 的 actor 语义不同，强行混在一张表里会让 user 列、workspace 列和操作菜单都变复杂。

交互规则：

- tab 状态写入 query string：`/tokens?tab=user`、`/tokens?tab=tenant`。
- `Show revoked` 默认关闭；开启时请求 `?all=true`。
- 列表空态显示“暂无 Tenant Access Token”，并提供创建按钮。
- 加载失败时保留当前 tab，显示错误提示和重试按钮。
- acting mode 下不展示 Tenant Access Tokens tab，也不允许通过前端入口创建、修改、吊销 tenant token。
- 如果用户不是 workspace owner/admin，不展示创建、编辑、吊销操作；可隐藏 tab，或只读显示“需要 workspace owner/admin 权限”。

#### 页面 ASCII 原型

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│  Tokens                                                        [+ Create]     │
├──────────────────────────────────────────────────────────────────────────────┤
│  [ User / Agent Tokens ]  [ Tenant Access Tokens ]                           │
├──────────────────────────────────────────────────────────────────────────────┤
│  Tenant Access Tokens                                      [Show revoked ☐]  │
│                                                                              │
│  ┌──────────────┬────────────────┬──────────────┬───────┬────────┬────────┐ │
│  │ Name         │ Prefix         │ Projects     │Scope  │Status  │Actions │ │
│  ├──────────────┼────────────────┼──────────────┼───────┼────────┼────────┤ │
│  │ runtime-prod │ xuanchu_tenant │ agentapi     │ 8     │Active  │ ⋯      │ │
│  │ report-bot   │ xuanchu_tenant │ all projects │ 3     │Expires │ ⋯      │ │
│  │ old-sync     │ xuanchu_tenant │ agentapi     │ 4     │Revoked │ —      │ │
│  └──────────────┴────────────────┴──────────────┴───────┴────────┴────────┘ │
│                                                                              │
│  ⋯ menu: Edit / Revoke                                                        │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 创建 Dialog

```text
┌────────────────────────────────────────────────────────────┐
│  Create Tenant Access Token                           [×]  │
├────────────────────────────────────────────────────────────┤
│  Name *                                                    │
│  ┌──────────────────────────────────────────────────┐     │
│  │ runtime-prod                                     │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  Workspace                                                │
│  dajee                                                     │
│                                                            │
│  Project allowlist                                         │
│  [ ] all projects                                          │
│  [x] agentapi                                              │
│  [ ] ops                                                   │
│                                                            │
│  Scopes                                                    │
│  ┌────────────────────────────────────────────────────┐   │
│  │ Task          [x] read   [x] write                  │   │
│  │ Project       [x] read   [ ] write                  │   │
│  │ Hook          [ ] read   [ ] write                  │   │
│  │ Notification  [ ] read   [ ] write                  │   │
│  │ Token         [disabled for tenant tokens]          │   │
│  │ Impersonate   [hidden]                              │   │
│  └────────────────────────────────────────────────────┘   │
│                                                            │
│  Expires                                                   │
│  ( ) Never   ( ) 7 days   (●) 90 days   ( ) Custom         │
│                                                            │
│                                      [Cancel] [Create]     │
└────────────────────────────────────────────────────────────┘
```

### 7.3 创建成功 Dialog

```text
┌────────────────────────────────────────────────────────────┐
│  Tenant Access Token Created                         [×]  │
├────────────────────────────────────────────────────────────┤
│  Copy this token now. It will not be shown again.          │
│                                                            │
│  ┌──────────────────────────────────────────────────┐     │
│  │ xuanchu_tenant_xxxxxxxxxxxxxxxxxxxxxxxxxxxxx      │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  Use as:                                                   │
│  Authorization: Bearer xuanchu_tenant_xxx                  │
│                                                            │
│                                      [Copy] [Done]         │
└────────────────────────────────────────────────────────────┘
```

### 7.4 编辑 Dialog

```text
┌────────────────────────────────────────────────────────────┐
│  Edit Tenant Access Token                            [×]  │
├────────────────────────────────────────────────────────────┤
│  Name                                                      │
│  ┌──────────────────────────────────────────────────┐     │
│  │ runtime-prod                                     │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  Prefix                                                    │
│  xuanchu_tenant_x                                          │
│                                                            │
│  Project allowlist                                         │
│  [x] agentapi   [ ] ops   [ ] finance                      │
│                                                            │
│  Scopes                                                    │
│  ┌────────────────────────────────────────────────────┐   │
│  │ Task          [x] read   [x] write                  │   │
│  │ Project       [x] read   [ ] write                  │   │
│  │ Audit         [ ] read                              │   │
│  └────────────────────────────────────────────────────┘   │
│                                                            │
│  Expires                                                   │
│  ( ) Never   ( ) 7 days   (●) 90 days   ( ) Custom         │
│                                                            │
│                                      [Cancel] [Save]       │
└────────────────────────────────────────────────────────────┘
```

### 7.5 Admin Console `/admin/tokens`

Admin token 页面应能显示普通 token 和 tenant token。为避免一张表字段过多，推荐同样加 tab：

```text
┌────────────────────────────────────────────────────────────────────────────────────┐
│  Admin Token Management                                                             │
├────────────────────────────────────────────────────────────────────────────────────┤
│  [ User / Agent Tokens ]  [ Tenant Access Tokens ]                    [All revoked] │
│                                                                                    │
│  ┌──────────────┬────────────┬──────────────┬──────────────┬──────┬─────────┬────┐ │
│  │ Name         │ Workspace  │ Prefix       │ Projects     │Scope │ Status  │ ⋯  │ │
│  ├──────────────┼────────────┼──────────────┼──────────────┼──────┼─────────┼────┤ │
│  │ runtime-prod │ dajee      │ xuanchu_t... │ agentapi     │ 8    │ Active  │ ⋯  │ │
│  │ sync-staging │ partner    │ xuanchu_t... │ all projects │ 4    │ Revoked │ —  │ │
│  └──────────────┴────────────┴──────────────┴──────────────┴──────┴─────────┴────┘ │
└────────────────────────────────────────────────────────────────────────────────────┘
```

Admin 第一版只支持 edit / revoke，不提供 create。

## 8. 状态图

### 8.1 Token 生命周期

```text
                       create
                         │
                         ▼
              ┌──────────────────┐
              │ Active           │
              │ revoked_at = nil │
              │ expires_at > now │
              └────────┬─────────┘
                       │
          ┌────────────┴────────────┐
          │                         │
          ▼                         ▼
┌──────────────────┐       ┌──────────────────┐
│ Expired          │       │ Revoked          │
│ expires_at <= now│       │ revoked_at != nil│
└────────┬─────────┘       └──────────────────┘
         │
         │ revoke expired token
         ▼
┌──────────────────┐
│ Revoked          │
└──────────────────┘
```

规则：

- `Expired` 是时间派生状态，不需要写入数据库。
- `Revoked` 是显式状态，写 `revoked_at`。
- 已吊销 token 不能恢复。
- 已过期 token 可以被吊销，但不能被使用。
- P1 不允许把已过期 token 通过修改 `expires_at` 恢复为 active。需要继续使用时应创建新 token，并吊销旧 token。
- 未过期且未吊销的 token 可以修改 `expires_at`。

### 8.2 认证状态

```text
┌──────────────┐
│ Bearer token │
└──────┬───────┘
       │ prefix = xuanchu_tenant_
       ▼
┌──────────────────────────┐
│ Lookup tenant token row  │
└──────┬───────────────────┘
       │ not found / hash mismatch
       ▼
┌──────────────────────────┐
│ 401 auth_invalid_token   │
└──────────────────────────┘

Happy path:

┌──────────────┐
│ Bearer token │
└──────┬───────┘
       ▼
┌──────────────────────────┐
│ Hash verified            │
└──────┬───────────────────┘
       ▼
┌──────────────────────────┐
│ Check revoked / expired  │
└──────┬───────────────────┘
       ▼
┌──────────────────────────┐
│ Build Tenant Decision    │
└──────┬───────────────────┘
       ▼
┌──────────────────────────┐
│ Handler / MCP tool runs  │
└──────────────────────────┘
```

## 9. 流程图

### 9.1 Web Console 创建流程

```text
Owner/Admin browser
      │
      │ POST /api/v1/tenant-access-tokens
      │ {name, scopes, projects, expires_in_seconds}
      ▼
HTTP handler
      │
      │ require current user token: token:write + PermissionTokenWrite
      ▼
App service
      │
      ├─ validate workspace role owner/admin
      ├─ reject admin_acting token
      ├─ validate scopes (no impersonate, no token management scope in P1)
      ├─ resolve project allowlist in current workspace
      ├─ generate xuanchu_tenant_ raw token
      ├─ store sha256 hash + prefix
      └─ append audit: tenant_token.create
      ▼
Response
      │
      └─ raw token shown once in browser
```

### 9.2 HTTP API 调用流程

```text
External Agent / LLM runtime
      │
      │ Authorization: Bearer xuanchu_tenant_xxx
      ▼
authMiddleware
      │
      ├─ detect prefix xuanchu_tenant_
      ├─ authenticate api_tokens where type=tenant_access_token
      └─ attach credential context
      ▼
scopedServiceFor
      │
      ├─ workspace = token.workspace_id
      ├─ reject mismatched X-Xuanchu-Workspace / ?workspace
      ├─ resolve project ref inside workspace
      ├─ check scope capability
      ├─ check project allowlist
      └─ build RuntimeContext actor_type=tenant_access_token
      ▼
handler
      │
      ├─ executes app service
      └─ writes audit/log with actor_token_id
```

### 9.3 HTTP MCP 调用流程

```text
MCP client
      │
      │ POST /mcp
      │ Authorization: Bearer xuanchu_tenant_xxx
      ▼
HTTP router
      │
      ├─ mcpHostProtectionMiddleware
      ├─ authMiddleware
      └─ rejectActingTokenMiddleware (tenant token allowed)
      ▼
mcpserver.NewServer
      │
      └─ request carries tenant credential context
      ▼
tool wrapper
      │
      ├─ required capability per tool
      ├─ tenant tool denylist check
      ├─ required permission bypasses membership role for tenant token
      ├─ project/workspace allowlist check
      └─ no X-Xuanchu-As support in P1
      ▼
tool result
```

### 9.4 吊销流程

```text
Owner/Admin
      │
      │ DELETE /api/v1/tenant-access-tokens/{ref}
      ▼
App service
      │
      ├─ find by id or prefix
      ├─ verify token belongs to current workspace
      ├─ set revoked_at = now
      └─ append audit: tenant_token.revoke
      ▼
Next API call with same token
      │
      └─ 401 auth_token_revoked
```

## 10. 后端设计

### 10.1 Auth 包

新增：

```go
const TokenTypeTenantAccess = "tenant_access_token"
const TenantAccessTokenPrefix = "xuanchu_tenant_"
```

`GenerateTenantAccessToken()` 可复用现有 `GenerateToken` 逻辑，最终写入统一 `api_tokens` 表。

### 10.2 Storage 层

扩展现有 `ApiToken` / `ApiTokenEntry`：

```go
type ApiToken struct {
    ID               string
    UserID           *string
    Name             string
    Type             string
    TokenPrefix      string
    TokenHash        string
    ScopesJSON       string
    WorkspaceIDsJSON string
    ProjectIDsJSON   string
    CreatedAt        int64
    ExpiresAt        *int64
    RevokedAt        *int64
    LastUsedAt       *int64
}

type ApiTokenEntry struct {
    ID               string
    UserID           *string
    Name             string
    Type             string
    TokenPrefix      string
    TokenHash        string
    ScopesJSON       string
    WorkspaceIDsJSON string
    ProjectIDsJSON   string
    CreatedAt        int64
    ExpiresAt        *int64
    RevokedAt        *int64
    LastUsedAt       *int64
}
```

Repository：

```go
Create(row ApiTokenEntry) error
GetByIDOrPrefix(ref string) (ApiTokenEntry, error)
GetByPrefix(prefix string) (ApiTokenEntry, error)
ListByUser(userID string, includeRevoked bool) ([]ApiTokenEntry, error)
ListTenantByWorkspace(workspaceID string, includeRevoked bool) ([]ApiTokenEntry, error)
ListAllByType(tokenType string, includeRevoked bool) ([]ApiTokenEntry, error)
Update(ref string, updates TokenUpdates) error
Revoke(ref string, now int64) error
TouchLastUsed(id string, now int64) error
```

`ListByUser` 必须继续只返回 `pat` / `agent`，不能把 `user_id IS NULL` 的 tenant token 混入普通用户 token 列表。Admin 普通 token 列表同理默认排除 tenant token，tenant tab 通过 `ListAllByType(tenant_access_token, ...)` 获取。

tenant token 的 modify / revoke service 在 `GetByIDOrPrefix` 后必须显式校验 `entry.Type == tenant_access_token`，避免共用 ref 查询误操作 PAT / Agent token。普通 PAT / Agent token 的 modify / revoke service 也必须拒绝 `tenant_access_token`。

### 10.3 App 层

新增服务方法：

```go
CreateTenantAccessToken(input CreateTenantAccessTokenInput) (CreatedTenantAccessToken, error)
ListTenantAccessTokens(input ListTenantAccessTokensInput) ([]TenantAccessTokenView, error)
ModifyTenantAccessToken(input ModifyTenantAccessTokenInput) (*TenantAccessTokenView, error)
RevokeTenantAccessToken(ref string) error
AuthenticateBearerToken(raw string) // 扩展识别 xuanchu_tenant_
AuthorizeTokenRequest(...)          // 扩展 tenant credential 分支
```

`TenantAccessTokenView` 没有业务 actor user；P1 列表也不返回创建人。后续如果要展示创建来源，应从审计日志或新增显式字段设计，不要把 tenant token 伪装成用户 token：

```go
type TenantAccessTokenView struct {
    ID           string
    Prefix       string
    Name         string
    Type         string // tenant_access_token
    WorkspaceID  string
    ProjectIDs   []string
    Scopes       []string
    CreatedAt    int64
    ExpiresAt    *int64
    RevokedAt    *int64
    LastUsedAt   *int64
}
```

### 10.4 认证链改造边界

当前 HTTP / MCP 鉴权链默认 `AuthenticatedToken` 一定有 `User`，`AuthorizeTokenRequest` 会读取 membership role，再构造 `RuntimeContext.ActorUserID`。tenant token 无 user，因此 P1 必须在这些边界做显式分支：

```go
type AuthenticatedCredentialKind string

const (
    AuthenticatedCredentialUserToken   AuthenticatedCredentialKind = "user_token"
    AuthenticatedCredentialTenantToken AuthenticatedCredentialKind = "tenant_access_token"
    AuthenticatedCredentialAdminActing AuthenticatedCredentialKind = "admin_acting"
)

type AuthenticatedToken struct {
    Kind              AuthenticatedCredentialKind
    Token             TokenView
    User              storage.User
    TenantAccessToken *TenantAccessTokenView
    AdminActingTrace  *AdminActingTrace
}
```

实现要求：

- `authMiddleware` 只负责认证 Bearer，不能要求所有 credential 都有 `User`。
- `AuthorizeTokenRequest` 先按 `Kind` 分支；tenant token 走 `authorizeTenantTokenRequest`，不调用 `resolveRequestWorkspace(user, ...)`，不查询 `memberships`。
- tenant 分支直接以 token 绑定 workspace 作为 effective workspace。
- tenant 分支只做 scope、workspace binding、project allowlist、tool / endpoint 禁止清单校验。
- handler 不应通过空 `ActorUserID` 推断匿名请求；必须读取 `RuntimeContext.ActorType`。

### 10.5 Authz / RuntimeContext

现有 `authz.Decision.Principal` 只有 user 语义。需要扩展，推荐新增 Actor 而不是把 machine actor 塞进 user：

```go
type ActorType string

const (
    ActorUser ActorType = "user"
    ActorTenantAccessToken ActorType = "tenant_access_token"
)

type Actor struct {
    Type      ActorType
    UserID    *string
    UserName  string
    TokenID   *string
    TokenName string
    TokenPrefix string
}
```

`Decision` 增加 `Actor Actor`，旧 `Principal` 保留兼容用户路径。tenant token path：

```text
Decision.Actor.Type = tenant_access_token
Decision.Principal.UserID = ""
Decision.Credential.Kind = tenant_access_token
Decision.Tenant.WorkspaceID = token.workspace_id
```

`RuntimeContext` 增加：

```go
ActorType string
ActorTokenID string
ActorTokenName string
ActorTokenPrefix string
```

业务写审计时优先使用 `ActorType`。

### 10.6 禁止的路径

tenant token 请求以下能力时返回：

```text
token_scope_denied 或 permission_denied
```

具体规则：

- `X-Xuanchu-As` 非空：`token_scope_denied`
- user/member/token 管理 endpoint：`token_scope_denied`
- `assignee:me`：`tenant_actor_not_user`
- active context use/none endpoint 或 tool：`tenant_actor_not_user`
- MCP `me_get`：固定返回 `tenant_actor_not_user`，不要并入普通 tool denylist 的 `token_scope_denied`
- 需要 active user 的 helper：`tenant_actor_not_user`

## 11. 前端设计

### 11.1 Workspace Console

新增目录：

```text
web/src/features/workspace/tenant-tokens/
```

建议文件：

```text
tenant-token-api.ts
tenant-tokens-page.tsx
tenant-token-create-dialog.tsx
tenant-token-edit-dialog.tsx
tenant-token-revoke-dialog.tsx
tenant-token-created-result.tsx
tenant-token-form.tsx
```

复用：

- `ScopeEditor`
- `deriveTokenStatus`
- expires preset helpers
- `workspaceApiGet/Post/Patch/Delete`

页面集成方式：

- 方案 A：在现有 `TokensPage` 内加 tabs。
- 方案 B：新增路由 `/tenant-tokens`。

推荐方案 A。理由：用户心智里都是“访问 token”，但 tab 明确区分 user/agent 与 tenant token。

### 11.2 Admin Console

新增目录：

```text
web/src/features/admin/tenant-tokens/
```

也可复用现有 `admin/tokens` 页并加 tabs。推荐先在同一个 `/admin/tokens` 页面内加 tabs，避免 admin 导航膨胀。

### 11.3 UI 权限

Workspace Console：

- 只有 `effective_role` 为 `owner` 或 `admin` 时展示 Tenant Access Tokens tab 的 create/edit/revoke 操作。
- viewer/member 可不展示该 tab，或展示只读空权限提示。
- acting mode 下不展示 Tenant Access Tokens tab；即使 acting actor 是 owner/admin，也必须由后端拒绝管理 API。

Admin Console：

- server admin 可 list/edit/revoke 所有 workspace 的 tenant token。
- 第一版不创建。
- admin tab 状态写入 query string：`/admin/tokens?tab=user`、`/admin/tokens?tab=tenant`。
- `All revoked` 默认关闭，开启后请求 `?all=true`。

## 12. 错误码

管理 API 新增或复用：

```text
tenant_token_not_found                 404
tenant_token_name_required             400
tenant_token_scope_invalid             400
tenant_token_project_scope_invalid     400
tenant_token_revoked                   409  修改已吊销 token
tenant_token_expired                   409  修改已过期 token 试图复活
admin_acting_not_allowed               401  acting token 调管理 API
```

资源 API / MCP 认证与授权错误：

```text
auth_invalid_token       401  token 不存在或 hash mismatch
auth_token_revoked       401  revoked_at 非空
auth_token_expired       401  expires_at <= now
workspace_scope_denied   403  请求 workspace 与 token 绑定不一致
project_scope_denied     403  请求 project 不在 allowlist
token_scope_denied       403  scope/tool/endpoint 不允许
tenant_actor_not_user    400  请求依赖个人 actor，例如 /me 或 assignee:me
```

资源 API 不应暴露 token 是否存在，仍使用 `auth_invalid_token`。`tenant_token_workspace_mismatch` 不作为 P1 错误码使用，统一复用现有 `workspace_scope_denied`。

## 13. 日志与审计

HTTP access log 新增字段：

```text
actor_type
actor_token_id
actor_token_name
actor_token_prefix
workspace_id
project_id
```

审计 action：

```text
tenant_token.create
tenant_token.modify
tenant_token.revoke
```

tenant token 执行业务操作时，业务审计记录：

```text
actor_type = tenant_access_token
actor_token_id = <id>
actor_token_name = <name>
actor_token_prefix = <prefix>
actor_user_id = null
```

审计列表对外 JSON：

```json
{
  "actor_type": "tenant_access_token",
  "actor_user": null,
  "actor_token": {
    "id": "tok_...",
    "name": "runtime-prod",
    "prefix": "xuanchu_tenant_x"
  }
}
```

普通用户审计继续输出 `actor_type=user` 和 `actor` / `actor_user` 的 `task.UserInfo`，不得退化为裸 UUID。

不要把 raw token 写入日志、审计、错误返回或浏览器持久存储。

## 14. P1 / P2 范围

### P1 必须完成

- 扩展 `api_tokens` 表：`user_id` 改为 nullable，新增 tenant token 类型约束测试，不新增 `tenant_access_tokens` 表。
- 新增 `audit_logs.actor_type`、`actor_token_id/name/prefix` migration。
- 扩展 token repository，并新增 app service、HTTP management API。
- Bearer 认证识别 `xuanchu_tenant_`。
- 鉴权链支持无 user credential，tenant token 不走 membership 查询。
- tenant 专用 scope whitelist 和 wildcard 过滤。
- HTTP API 授权支持 tenant token。
- HTTP MCP 授权支持 tenant token。
- 审计与 access log 记录 `actor_type=tenant_access_token`。
- Workspace Console `/tokens` 增加 Tenant Access Tokens tab。
- Admin Console `/admin/tokens` 增加 Tenant Access Tokens tab。
- 创建后 raw token 一次性展示。
- revoke / expires / last_used 行为完整。
- 禁止 `X-Xuanchu-As`、user/member/token 管理、`assignee:me`。
- 禁止 active context use/none。
- 禁止 acting token 管理 tenant token。
- 已过期 token 不能通过修改 `expires_at` 恢复使用。
- 单元测试、HTTP 测试、MCP 测试、Web Console 组件测试。

### P2 后续考虑

- OAuth client credentials flow，把 `tenant_access_token` 从 API key 形态升级为标准 OAuth access token。
- `user_access_token` / OIDC resource server。
- token rotation：生成新 token、旧 token grace period、自动替换提示。
- per-token rate limit。
- token description / labels。
- admin 在 `/admin/workspaces/{workspace}` 直接创建 tenant token。
- tenant token 受控 impersonation / actor assertion。
- CLI 管理命令：`tenant-token create/list/revoke`。
- 更细的 MCP tool allowlist。

## 15. 测试计划

后端：

```bash
go test ./internal/auth ./internal/authz ./internal/app ./internal/storage ./internal/httpapi ./internal/mcpserver -run 'Tenant|Token|Auth|MCP'
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
```

前端：

```bash
pnpm --dir web test -- --run
pnpm --dir web typecheck
pnpm --dir web lint
```

覆盖点：

- 创建 tenant token 只返回一次 raw token。
- hash 校验正确，错误 token 返回 `auth_invalid_token`。
- revoked / expired token 无法访问 HTTP API 和 MCP。
- workspace mismatch 被拒。
- project allowlist 生效。
- tenant token 可以调用 task/project/config/audit 和不需要用户 `created_by` 的 hook/notification/reminder 类接口；创建 hook、notification sink、reminder rule、event notification rule、task link、project annotation 返回 `tenant_actor_not_user`。
- tenant token 不能调用 user/member/token 管理接口。
- tenant token 使用 `*` scope 也不能获得 user/member/token/impersonate/workspace:write 能力。
- tenant token 不能使用 `X-Xuanchu-As`。
- tenant token 不能使用 `assignee:me`。
- tenant token 调 `/api/v1/me` 和 MCP `me_get` 返回 `tenant_actor_not_user`。
- tenant token 调 active context use/none 路径和 MCP `context_set` / `context_none` 返回 `tenant_actor_not_user`。
- tenant token 可管理 workspace context 定义，但不会写入 `activeContextMetaKey(userID, workspaceID)`。
- acting token 调 tenant token 管理 API 返回 `admin_acting_not_allowed`。
- 已过期 token 不能通过 PATCH `expires_at` 复活。
- 审计列表和 access log 能稳定输出 machine actor 字段。
- HTTP MCP 使用 tenant token 能调用允许的 tool。
- HTTP MCP 使用 tenant token 调禁止 tool 返回 `token_scope_denied` 或 `tenant_actor_not_user`。
- Web Console create/edit/revoke/list 状态正确。
- Web Console acting mode 不展示 Tenant Access Tokens tab。
- Admin Console 可跨 workspace list/edit/revoke tenant token。

## 16. 验收标准

1. workspace owner/admin 可以在 Web Console 创建 `tenant_access_token`，复制 raw token 后刷新页面无法再次看到明文。
2. 外部客户端可用该 token 调用 HTTP API。
3. 外部 MCP client 可用该 token 连接 `/mcp` 并调用授权范围内 tool。
4. revoke 后同一 token 立即失效。
5. 过期 token 返回 401。
6. 审计日志能看出操作来自 tenant token，且包含 token id/name/prefix。
7. 当前 PAT / Agent token 行为不回归。
8. 当前 server admin token / acting token 行为不回归。
9. 文档同步 README、ROADMAP、HTTP/MCP manual、Web Console manual。

## 17. 设计结论

第一版应把 `tenant_access_token` 做成一等租户 API key，而不是完整 OAuth token。它独立于普通 user，不复用 `api_tokens.user_id` 语义，也不伪装成 Agent token；但存储上应复用现有 `api_tokens` 表，用 `type=tenant_access_token` 和 nullable `user_id` 表达机器凭证。这样可以减少重复表和重复仓储，同时为未来 OIDC `user_access_token` 和 OAuth client credentials 留出清晰边界。
