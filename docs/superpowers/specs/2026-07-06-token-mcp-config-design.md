# Token MCP 配置弹窗与可恢复密文设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-06  
**状态：** 草案  
**背景需求：** Web Console `/tokens` 页面中，每一行 token 后面需要一个按钮。点击后弹出 HTTP MCP 配置说明，并能复制可直接使用的 token。当前实现只保存 token hash，列表只返回 prefix，创建后明文只展示一次；本次明确调整为新创建 token 可以被服务端加密保存、按需解密展示。

## 1. 背景与现状

当前 `/tokens` 页面支持两类列表：

- 普通 API Tokens：PAT / Agent token，接口为 `/api/v1/tokens`。
- 租户访问令牌：`tenant_access_token`，接口为 `/api/v1/tenant-access-tokens`。

当前数据契约：

- 普通 token 列表响应只含 `id / prefix / name / type / user / workspace_ids / project_ids / scopes / created_at / expires_at / revoked_at / last_used_at`。
- tenant token 列表响应只含 `id / prefix / name / type / workspace_id / project_ids / scopes / created_at / expires_at / revoked_at / last_used_at`。
- 创建响应含一次性 `token` 明文。
- `api_tokens` 表只保存 `token_prefix` 和 `token_hash`，不能从数据库恢复完整 token。

因此，若要在任意列表行弹出可复制 MCP 配置，必须先改变 token secret 的存储模型：继续保留 hash 用于认证，同时额外保存加密后的 raw token，供受控 reveal 场景读取。

## 2. 目标

1. `/tokens` 普通 API Tokens 和租户访问令牌两张列表的每一行都提供「MCP 配置」按钮。
2. 点击按钮打开弹窗，展示 HTTP MCP endpoint、Authorization header、客户端配置片段和当前 token 的关键限制。
3. 弹窗内可以复制完整 token，也可以复制 MCP 配置 JSON。
4. 新创建的 PAT / Agent / tenant token 都保存加密后的 raw token，可在后续 reveal。
5. 认证仍使用 `token_hash`，不改 Bearer token 校验路径。
6. reveal 行为必须走现有 workspace / token scope / project allowlist 授权边界。
7. 没有线上数据、没有测试数据，本次不做旧数据迁移和 backfill。

## 3. 非目标

- 不改变 admin token 和 acting token。它们仍不用于 HTTP MCP。
- 不让 browser OIDC session 直接作为 MCP 凭证；MCP 仍只接受 Bearer token。
- 不把 raw token 放进普通列表接口。
- 不支持从旧 hash 反推 raw token。
- 不实现 token 轮换或双 token 平滑切换。
- 不改变 MCP tool schema、tool name 或权限模型。
- 不新增完整 OAuth / client credentials flow。

## 4. 核心产品决策

### 4.1 选择按需 reveal，不在列表返回明文

列表接口继续只返回 prefix。每行按钮只负责打开弹窗；弹窗打开时单独调用 reveal/config 接口读取完整 token 和配置说明。

原因：

- 避免打开 `/tokens` 时一次性把所有 token 明文放进 React Query cache、浏览器 DevTools 和网络响应。
- reveal 是用户明确动作，便于后续增加审计、二次确认或更细粒度权限。
- 不影响现有列表性能和表格渲染。

### 4.2 继续用 hash 做认证，用 envelope 做可恢复明文

新增字段只用于展示和复制，不参与认证：

```text
api_tokens.token_secret_ciphertext
```

认证继续走：

```text
Authorization raw token -> sha256 -> token_hash 常量时间比较
```

reveal 走：

```text
token_secret_ciphertext -> AES-256-GCM decrypt -> raw token
```

### 4.3 复用 `[security].config_secret_key`

加密使用现有 `internal/app/config_secret.go` 的 AES-256-GCM envelope 能力，密钥继续来自 TOML：

```toml
[security]
config_secret_key = "<base64-encoded-32-byte-key>"
```

服务端创建可恢复 token 时必须能拿到这个 key。若 HTTP / Web Console 创建 token 时缺少 key，返回：

```text
config_secret_key_missing
```

原因：如果允许在无 key 情况下创建 token，就会产生后续无法复制的「半功能」token，和本需求冲突。CLI 本地创建的历史 token 或手工插入 token 若没有 ciphertext，reveal 时返回明确错误。

### 4.4 不考虑迁移

当前没有线上数据，也没有测试数据。本次允许直接调整 GORM model，让新库自动拥有 `token_secret_ciphertext` 字段；不写 SQLite 手工迁移、不做旧行 backfill、不保留旧 token 可恢复兼容逻辑。

对没有 ciphertext 的行，接口返回：

```text
token_secret_unavailable
```

UI 文案：该令牌没有保存可恢复密文，请重新签发。

## 5. 后端设计

### 5.1 数据模型

`internal/storage/models.go` 的 `ApiToken` 新增字段：

```go
TokenSecretCiphertext string `gorm:"not null;default:''"`
```

`internal/storage/token_repo.go` 的 `ApiTokenEntry` 同步新增：

```go
TokenSecretCiphertext string
```

`apiTokenModel` / `apiTokenEntry` 需要双向映射该字段。

### 5.2 App 层输入与视图

普通 token 和 tenant token 创建流程中，生成 raw token 后同时生成 ciphertext：

```go
raw, prefix, hash, err := auth.GenerateToken(tokenType)
ciphertext, err := app.EncryptConfigSecret(secretKey, raw)
```

实现上不建议让 `token.go` 直接读 config。应在 `app.ServiceOptions` 或专用 token secret service 中注入 `SecretKey []byte`，保持 app 层可测试。

新增 app view：

```go
type TokenMCPConfigView struct {
    TokenID      string
    TokenName    string
    TokenType    string
    Prefix       string
    RawToken     string
    EndpointPath string
    Scopes       []string
    WorkspaceIDs []string
    ProjectIDs   []string
    RevokedAt    *int64
    ExpiresAt    *int64
}
```

App 层新增方法：

```go
RevealTokenMCPConfig(tokenRef string) (TokenMCPConfigView, error)
RevealTenantTokenMCPConfig(tokenRef string) (TokenMCPConfigView, error)
```

职责：

1. 按 token ref 查行。
2. 校验 token 类型：普通接口只允许 `pat` / `agent`；tenant 接口只允许 `tenant_access_token`。
3. 复用现有 `enforceTokenRead` / request scope 限制，不能 reveal 当前 token 无权管理的目标 token。
4. revoked / expired token 可以 reveal，但 view 标出状态；前端展示不可用提示。
5. `TokenSecretCiphertext == ""` 返回 `token_secret_unavailable`。
6. 解密失败返回 `token_secret_unavailable` 或 `config_secret_key_missing`，不要把底层解密细节暴露给用户。

### 5.3 HTTP API

新增普通 token MCP 配置接口：

```http
GET /api/v1/tokens/{tokenRef}/mcp-config
Authorization: Bearer <workspace token>
```

新增 tenant token MCP 配置接口：

```http
GET /api/v1/tenant-access-tokens/{tokenRef}/mcp-config
Authorization: Bearer <workspace token or tenant token with token:read>
```

响应：

```json
{
  "token": "xuanchu_agent_xxx",
  "token_id": "tok_...",
  "token_name": "claude-agent",
  "token_type": "agent",
  "prefix": "xuanchu_agent_abcd1234",
  "endpoint_path": "/mcp",
  "scopes": ["task:read", "task:write"],
  "workspace_ids": ["ws_..."],
  "project_ids": [],
  "expires_at": null,
  "revoked_at": null
}
```

HTTP 层不需要拼 `window.location.origin`，只返回 `endpoint_path=/mcp`。前端根据当前 origin 生成完整 URL。这样反向代理域名、localhost、测试环境都由浏览器当前地址决定。

错误码：

| 错误码 | HTTP | 语义 |
|---|---:|---|
| `token_not_found` | 404 | 目标 token 不存在或类型不匹配 |
| `token_secret_unavailable` | 409 | 目标 token 没有可恢复密文或解密失败 |
| `config_secret_key_missing` | 500 / 400 | 服务端未配置解密密钥 |
| `token_scope_denied` | 403 | 当前凭证没有 `token:read` 或目标超出当前 token 范围 |
| `workspace_scope_denied` | 403 | 目标 token workspace 超出当前凭证范围 |
| `project_scope_denied` | 403 | 目标 token project 超出当前凭证范围 |

### 5.4 审计与日志

reveal 完整 token 是高敏动作，应记录审计：

```text
token.mcp_config_reveal
tenant_token.mcp_config_reveal
```

审计 payload 只记录：

```json
{
  "token_id": "tok_...",
  "token_name": "claude-agent",
  "token_type": "agent",
  "token_prefix": "xuanchu_agent_abcd1234"
}
```

严禁记录 raw token、Authorization header、完整 MCP config JSON。

operation log 同样只记录 prefix、token id、actor、workspace、结果，不记录 raw token。

## 6. 前端设计

### 6.1 列表行按钮

在 `web/src/features/workspace/tokens/tokens-page.tsx` 中，表格每行的 prefix 后面增加一个图标按钮：

```text
xuanchu_agent_abcd1234  [MCP]
```

具体交互：

- 图标使用 `lucide-react`，优先 `PlugIcon` 或 `CableIcon`；没有合适图标时用 `CircleHelpIcon`。
- 按钮 `aria-label`：`MCP 配置：{{name}}`。
- tooltip：`查看 MCP 配置`。
- 对 revoked / expired token 不禁用按钮，因为用户可能需要复制配置排查；弹窗里明确标识「已吊销」或「已过期」。
- 对 `token_secret_unavailable`，弹窗显示不可恢复提示和「重新签发」建议，不展示 copy token。

### 6.2 MCP 配置弹窗

新增组件：

```text
web/src/features/workspace/tokens/token-mcp-config-dialog.tsx
```

弹窗结构：

```text
┌──────────────────────────────────────────────┐
│ MCP 配置 · claude-agent                 [×] │
├──────────────────────────────────────────────┤
│ 类型 agent · prefix xuanchu_agent_abcd1234   │
│ 状态 有效                                    │
│                                              │
│ Endpoint                                     │
│ http://127.0.0.1:8080/mcp          [复制]    │
│                                              │
│ Bearer Token                                  │
│ xuanchu_agent_xxxxxxxxxxxxxxxxxxxx  [复制]    │
│                                              │
│ MCP 客户端配置                                │
│ {                                            │
│   "url": "http://127.0.0.1:8080/mcp",        │
│   "headers": {                               │
│     "Authorization": "Bearer ..."            │
│   }                                          │
│ }                                  [复制配置] │
│                                              │
│ 说明：Agent token 可用于 HTTP MCP。若需要     │
│ impersonation，token 必须包含 impersonate。   │
└──────────────────────────────────────────────┘
```

tenant token 的说明：

```text
Tenant token 以系统身份调用 HTTP MCP，不绑定自然人用户，不支持 impersonation、me_get、assignee:me 或个人 active context。
```

PAT 的说明：

```text
PAT 可用于 HTTP MCP，但不能使用 X-Xuanchu-As impersonation。
```

Agent 的说明：

```text
Agent token 可用于 HTTP MCP。若要代表成员执行，请配置 X-Xuanchu-As，并确保 token 包含 impersonate scope。
```

### 6.3 配置 JSON

前端生成通用 Streamable HTTP MCP 配置片段：

```json
{
  "url": "https://example.com/mcp",
  "headers": {
    "Authorization": "Bearer xuanchu_agent_xxx"
  }
}
```

如果当前 token 是 agent 且包含 `impersonate`，弹窗额外展示可选 header：

```json
{
  "X-Xuanchu-As": "<user-name-or-email>"
}
```

这里不默认填用户，避免误导为当前登录用户。

### 6.4 API 封装

`web/src/features/workspace/tokens/token-api.ts` 新增类型：

```ts
export type TokenMcpConfig = {
  token: string
  token_id: string
  token_name: string
  token_type: string
  prefix: string
  endpoint_path: string
  scopes: string[] | null
  workspace_ids?: string[] | null
  project_ids?: string[] | null
  expires_at?: number | null
  revoked_at?: number | null
}
```

新增 fetch helpers：

```ts
getTokenMcpConfig(ref: string): Promise<TokenMcpConfig>
getTenantTokenMcpConfig(ref: string): Promise<TokenMcpConfig>
```

query key 使用：

```ts
["token", "mcp-config", tokenType, tokenId]
```

弹窗关闭时不需要主动清 cache；但 query cache 时间应短，或在组件中用 `gcTime: 0` / `staleTime: 0`，降低 raw token 留在内存中的时间。

## 7. 安全边界

1. **不在列表返回 raw token。**
2. **不在审计和日志记录 raw token。**
3. **不在 URL query 传 token。**
4. **不把 MCP config 写入 localStorage / sessionStorage。**
5. **弹窗复制动作只写剪贴板，不持久化。**
6. **reveal 必须要求 `token:read`，并受当前 token workspace/project 范围限制。**
7. **tenant token reveal tenant token 时，仍只能 reveal 当前 workspace 内、当前 project allowlist 内可管理的 tenant token。**
8. **服务端缺少 config secret key 时，不创建可恢复 token。**
9. **旧行没有 ciphertext 时不猜测、不降级展示 prefix。**

## 8. 文案

新增中文文案：

```ts
token.mcpConfig = "MCP 配置"
token.mcpConfigFor = "MCP 配置：{{name}}"
token.mcpEndpoint = "MCP Endpoint"
token.bearerToken = "Bearer Token"
token.copyConfig = "复制配置"
token.copyEndpoint = "复制 Endpoint"
token.secretUnavailable = "该令牌没有保存可恢复密文，请重新签发后再复制完整 token。"
token.mcp.agentHint = "Agent token 可用于 HTTP MCP。若要代表成员执行，请配置 X-Xuanchu-As，并确保 token 包含 impersonate scope。"
token.mcp.patHint = "PAT 可用于 HTTP MCP，但不能使用 X-Xuanchu-As impersonation。"
token.mcp.tenantHint = "Tenant token 以系统身份调用 HTTP MCP，不绑定自然人用户，不支持 impersonation、me_get、assignee:me 或个人 active context。"
```

英文文案同步加入 `en-US.ts`。

## 9. 测试计划

### 9.1 后端

新增或更新测试：

- `internal/app/token_test.go`
  - 创建普通 token 时写入 `TokenSecretCiphertext`。
  - 创建 tenant token 时写入 `TokenSecretCiphertext`。
  - `RevealTokenMCPConfig` 能解密并返回 raw token。
  - 无 ciphertext 返回 `token_secret_unavailable`。
  - 缺少 secret key 时创建可恢复 token 返回 `config_secret_key_missing`。
  - reveal 不越过 workspace/project/token scope 限制。

- `internal/httpapi/tokens_test.go`
  - `GET /api/v1/tokens/{ref}/mcp-config` 返回 raw token 与 `/mcp` path。
  - `GET /api/v1/tenant-access-tokens/{ref}/mcp-config` 返回 tenant token 配置。
  - 无 `token:read` 返回 `token_scope_denied`。
  - reveal 后审计存在，但 audit payload 不含 raw token。

### 9.2 前端

新增或更新测试：

- `web/src/features/workspace/tokens/tokens-page.test.tsx`
  - 普通 token 行显示 MCP 配置按钮。
  - tenant token 行显示 MCP 配置按钮。
  - 点击按钮打开弹窗并请求正确 endpoint。
  - `token_secret_unavailable` 展示重新签发提示。

- 新增 `token-mcp-config-dialog.test.tsx`
  - 显示 endpoint、token、配置 JSON。
  - agent / pat / tenant 三类 hint 正确。
  - 复制按钮调用 `navigator.clipboard.writeText`。

### 9.3 验证命令

后端：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

前端：

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
```

## 10. 实施顺序建议

1. Storage model / repo 增加 `TokenSecretCiphertext`。
2. App ServiceOptions 注入 config secret key，并让 token create 写 ciphertext。
3. App 层实现普通 token / tenant token reveal。
4. HTTP 层新增两个 mcp-config endpoint。
5. 前端新增 API helper 和 `TokenMcpConfigDialog`。
6. `/tokens` 表格每行增加 MCP 配置按钮。
7. 补 i18n 文案。
8. 补后端、前端测试。
9. 更新 README 中「token 明文只展示一次」相关描述，改为「创建后可在具备权限的 Web Console 中按需 reveal」。

## 11. 验收标准

- 新建 Agent token 后，关闭创建弹窗，再从列表行点击「MCP 配置」，仍能复制完整 token。
- 新建 tenant token 后，同样可以从 tenant token 列表行复制完整 token 和 MCP 配置。
- 列表接口响应中不出现完整 raw token。
- reveal endpoint 的日志和审计不泄露 raw token。
- 没有 `token:read` 的凭证不能 reveal。
- revoked / expired token 的弹窗能打开，但明确标识不可用状态。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。
- `pnpm --dir web typecheck`、`pnpm --dir web test`、`pnpm --dir web lint`、`pnpm --dir web build` 通过。
