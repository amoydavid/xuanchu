# Workspace OIDC 接入设计（yaoguang IdP）

- 日期：2026-07-03
- 状态：草案
- 里程碑：v0.5.0
- 关联文档：[ROADMAP.md](../../../ROADMAP.md)、[README.md](../../../README.md)、`docs/superpowers/specs/2026-06-19-xuanchu-authz-decision-design.md`

## 1. 背景与目标

### 1.1 目标

让每个 workspace 可选择接入一个 OIDC 提供方（本期固定对接 [yaoguang](../../../../../yaoguang)），实现：

1. **浏览器 SSO 登录**：用户通过 yaoguang 完成 OIDC 登录后，免输 xuanchu token 直接访问 Web Console（只读视角）。
2. **通讯录同步开通成员**：workspace 管理员触发同步，从 yaoguang 拉取组织成员，建立本地 user、身份映射与 membership，免去手动开通。

### 1.2 既有约束（沿用 ROADMAP / authz spec 原则）

- **认证只负责外部身份映射；授权仍由 membership / role / scope / allowlist 决定。** OIDC 不复用现有 token scope 体系，仅产出 Principal。
- yaoguang 既是 OIDC provider，又是 directory 通讯录来源，两者同源、共享 base URL 与 org 配置。
- 配置管理仅走 Web Console（HTTP API），不做 CLI / MCP 配置入口。
- 不做 JIT 自动开通：用户必须在通讯录同步阶段已建立身份映射，OIDC 登录才放行。

### 1.3 非目标

- 不实现 OAuth / OIDC token server（xuanchu 不做 IdP）。
- 不引入 redis / asynq，同步任务持久化复用现有 DB 轮询 dispatcher 范式。
- 不实现多 provider 并存（单 workspace 单 provider，固定 yaoguang，结构上为未来多 provider 留位）。
- 不实现 SCIM。
- 不为 cookie session 开放写操作 API（仅读 + SSO 自身流程）。

## 2. 现状与扩展点

| 既有扩展点 | 位置 | 复用方式 |
|---|---|---|
| `CredentialBrowserSession` 凭证类型（预留枚举位） | `internal/authz/model.go` | 作为 OIDC 登录产生的 browser session 凭证类型 |
| `UserExternalID (Provider, ExternalID)` 身份映射表 + repo | `internal/storage/models.go`、`external_id_repo.go` | OIDC sub 与 IM external identity 的映射落点 |
| `ConfigRepository` 结构化 workspace 级配置（带 schema、加密、脱敏） | `internal/storage/config_repo.go`、`internal/app/scoped_config.go` | 存储 workspace 级 OIDC/directory 配置 |
| `/sso/{provider}?redirect=` redirect 语义（v0.4.2 预留） | ROADMAP | 落地为 `/sso/oidc/*` 路由 |
| 现有 hook/notification dispatcher（DB 轮询 + claim/lease） | `internal/hookruntime`、`internal/notificationruntime` | directory sync job 的持久化执行范式 |
| HTTP server graceful shutdown / runtime goroutine 管理 | `internal/cli/server.go` | 挂入新的 sync scheduler/dispatcher goroutine |

**核心新增基础设施**：当前服务端只有 Bearer 鉴权、无 cookie/session 表。需新增 `BrowserSession` / `BrowserAuthFlow` 持久化，并改造 `authMiddleware` 支持「Bearer 优先、Cookie 兜底」双通道。

## 3. 整体架构

两条独立但共享配置与身份映射的链路：

```
┌─────────────────────────────────────────────────────────────┐
│  链路 A：管理员通讯录同步（建立身份与成员关系）               │
│  HTTP: POST /api/v1/workspaces/{id}/sso/sync                 │
│  持久化: DirectorySyncJob 表（DB 轮询 dispatcher）           │
│  app: DirectorySyncService                                  │
│  - 读 workspace sso.* 配置                                  │
│  - HTTP GET yaoguang /api/orgs/{org}/directory/members       │
│  - upsert User + UserExternalID(sub + external_identities)   │
│  - upsert Membership (role 原样映射, disabled 移除)          │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│  链路 B：用户 OIDC 浏览器登录（映射 + 建立 session）          │
│  HTTP: /sso/oidc/start, /sso/oidc/callback, /auth/logout     │
│  app: OIDCAuthService                                       │
│  - Auth Code Flow + PKCE (go-oidc + oauth2)                  │
│  - id_token 验签（discovery 拉 JWKS）                        │
│  - sub 查 UserExternalID 命中→建 browser_session+发 cookie   │
│  - 未命中→拒绝（依赖链路 A 已建好成员）                       │
└─────────────────────────────────────────────────────────────┘
```

### 3.1 分层约束

| 层 | 职责 | 约束 |
|---|---|---|
| `internal/auth/oidc`（新） | OIDC RP 逻辑：discovery、code exchange、id_token 校验 | 纯 Go，不依赖 GORM/Cobra/HTTP server |
| `internal/auth/directory`（新） | yaoguang directory HTTP 客户端 + JSON 解析 | 纯客户端，不做持久化，返回结构化 `DirectoryMember` |
| `internal/app`（扩） | `OIDCAuthService`、`DirectorySyncService`、`OIDCConfigService`、`DirectorySyncScheduler`、`DirectorySyncDispatcher` | 编排 repo 与 auth 包，管理 session/sync job 生命周期 |
| `internal/storage`（扩） | 新增 `BrowserSession`、`BrowserAuthFlow`、`DirectorySyncJob` model + repo；扩 `UserExternalID` 使用 | 不做参数解释 |
| `internal/httpapi`（扩） | SSO/logout 路由（不挂 authMiddleware）；`authMiddleware` 改双通道；sso config 路由 | 不写业务规则 |
| `internal/cli`（扩） | server 启动挂入新 runtime goroutine | 不新增 CLI 命令（配置仅走 Web Console） |

### 3.2 新增依赖（纯 Go，零 CGO）

- `github.com/coreos/go-oidc/v3/oidc` —— OIDC discovery + id_token 验签
- `golang.org/x/oauth2` —— Auth Code Flow + PKCE

## 4. Workspace OIDC / Directory 配置模型

每个 workspace 在 `ConfigRepository` 里以 `scope=workspace` 存一组带类型的 key（约定前缀 `sso.`）。未配置 = 该 workspace 未启用 OIDC。

### 4.1 配置 schema

| Key | 类型 | 必填 | 含义 |
|---|---|---|---|
| `sso.provider` | string | 是 | 固定 `yaoguang`（本期唯一 provider，为未来多 provider 留扩展位） |
| `sso.issuer_base_url` | string | 是 | yaoguang 根 URL。OIDC discovery 走 `{base}/.well-known/openid-configuration`，directory API 走 `{base}/api/orgs/{org_id}/directory/members`，OAuth 端点由 discovery 给出 |
| `sso.org_id` | string | 是 | yaoguang 的 organization id（directory API 路径 + 校验 token 所属 org） |
| `sso.client_id` | string | 是 | OIDC client_id（xuanchu 作为 RP 在 yaoguang 注册的 internal app client） |
| `sso.client_secret` | string(secret) | 是 | OIDC client_secret。加密存储 |
| `sso.directory_access_token` | string(secret) | 是 | 调用 directory API 的 Bearer（tenant_access_token，须覆盖 `org.members.read` + `directory_access=org_read`）。加密存储 |
| `sso.scopes` | string | 否 | OIDC 请求的 scope，空则默认 `openid profile email` |
| `sso.redirect_path` | string | 否 | 回调路径，默认 `/sso/oidc/callback` |
| `sso.external_base_url` | string | 否 | xuanchu 自身外部可达 URL，用于拼 `redirect_uri`。缺省时回退到 Host header |
| `sso.session_ttl` | duration | 否 | browser session 有效期，默认 `168h`（7 天） |
| `sso.sync_interval` | duration | 否 | 定时同步周期，默认 `1h`，`0` 表示禁用定时（仅手动） |
| `sso.insecure_cookie` | bool | 否 | 本地 dev 用，关闭 cookie 的 `Secure` 属性，默认 `false` |

### 4.2 安全处理

- `client_secret` / `directory_access_token` 走 ConfigRepository 的 secret 通道（落库前加密，读取后只在 app 层短暂存活，绝不进入日志/响应/audit 明文）。
- 配置读取提供「脱敏视图」给 API/CLI 展示（只露前后 2 位），写时接收完整值。

### 4.3 配置入口（仅 Web Console）

- 写入：`PUT /api/v1/workspaces/{id}/sso/config`（接收完整值），仅 workspace owner/admin 可写。
- 读取（脱敏）：`GET /api/v1/workspaces/{id}/sso/config`。
- 启用判断：`OIDCConfigService.Get(workspaceID)` 返回 `(Config, enabled bool)`，`enabled=false` 时 SSO 路由对内返回 404、directory sync 拒绝执行。

## 5. 链路 A：通讯录同步

### 5.1 数据源：yaoguang directory API

调用 `GET {issuer_base_url}/api/orgs/{org_id}/directory/members`，请求头 `Authorization: Bearer <directory_access_token>`。

- 认证要求：token 须覆盖 scope `org.members.read`，且对应 internal app 的 `directory_access=org_read`。
- **无分页**，全量返回。
- 响应结构：

```json
{
  "ok": true,
  "data": {
    "members": [{
      "id": "<member uuid>",
      "sub": "yaoguang_member:<id>",
      "display_name": "张三",
      "role": "owner|admin|member",
      "status": "active|disabled",
      "external_identities": [
        {"provider":"feishu|wecom|dingtalk","user_type":"user_id","value":"<im user_id>"}
      ],
      "created_at": "...", "updated_at": "...",
      "department_ids": ["..."],
      "email_hint": "z***@x.com",
      "mobile_hint": "138****1234"
    }],
    "source": "organization_members",
    "stale": false
  }
}
```

- 字段约束：`external_identities` 仅暴露 feishu/wecom/dingtalk 的 `user_id` 类型；`email_hint`/`mobile_hint` 为脱敏且需 client 配置才返回。

### 5.2 字段映射（yaoguang → xuanchu）

| yaoguang member 字段 | xuanchu 落点 |
|---|---|
| `sub` (`yaoguang_member:{id}`) | `UserExternalID(provider="yaoguang", external_id=sub)` —— **OIDC 登录映射键** |
| `external_identities[].{provider,value}` | `UserExternalID(provider=<feishu/wecom/dingtalk>, external_id=value)` —— 多条，指向同一 user |
| `display_name` | `User.DisplayName`（新建 user 时 `Name` 也用此值） |
| `email_hint` | **不映射**（脱敏不可用，`User.Email` 留空） |
| `role` (`owner/admin/member`) | `Membership.Role` 原样映射 |
| `status` (`disabled`) | 跳过：不写入；已存在则移除其在该 workspace 的 Membership（不删 User） |
| `created_at`/`updated_at`/`id`/`department_ids`/`mobile_hint` | 不映射 |

### 5.3 DirectorySyncService 流程（`internal/app`）

```
1. OIDCConfigService.Get(workspaceID) → 未启用则报错中止
2. directoryClient.ListMembers(baseURL, orgID, accessToken)
   → GET {base}/api/orgs/{org}/directory/members, Bearer token
   → 解析 data.members[]（无分页，全量）
3. 对每个 status != disabled 的 member:
   a. 按 sub 查 UserExternalID(provider=yaoguang, external_id=sub)
   b. 命中 → 复用 UserID；未命中 → 新建 User(Name=display_name, Email="")
      - User.Name 唯一冲突：追加后缀 " (yaoguang:{id})" 保证唯一
   c. 同步 UserExternalID：
      - 主映射 (yaoguang, sub) 必写
      - external_identities 每条 (provider, value) 写入/更新
      - yaoguang 已删的身份 → 删除本地对应 UserExternalID
   d. upsert Membership(workspace, user, role=映射后)
4. 移除 disabled / 远端已不存在的成员：
   - 本地有 (yaoguang, sub) 映射但本次远端 status=disabled 或已不存在
   - 删除其在该 workspace 的 Membership（不删 User，保留痕迹）
5. 写审计日志（actor=system/directory-sync，记录 +N/-M/~K）
```

### 5.4 持久化与触发（DB 轮询 dispatcher）

复用现有 hook/notification dispatcher 范式（claim/lease，零外部依赖，重启不丢）。

**新增表 `DirectorySyncJob`**：

| 字段 | 类型 | 含义 |
|---|---|---|
| `ID` | string (uuid) | 主键 |
| `WorkspaceID` | string | 所属 workspace |
| `Status` | string | `pending` / `running` / `succeeded` / `failed` |
| `ClaimedAt` | *time | worker 认领时间 |
| `ClaimExpiresAt` | *time | claim 租约过期（防 worker 崩溃卡死） |
| `ErrorMessage` | string | 失败原因 |
| `StatsJSON` | string | 本次同步增/删/改计数（JSON） |
| `CreatedAt` | time | 创建时间 |
| `FinishedAt` | *time | 完成时间 |

**触发方式**：

- 手动：`POST /api/v1/workspaces/{id}/sso/sync`（owner/admin）→ 插入 `pending` job → 立即返回 job id；若该 workspace 已有 `pending`/`running` → 409 `sync_in_progress`。Web Console 轮询 `GET /api/v1/workspaces/{id}/sso/sync/jobs/{id}` 查状态。
- 定时：`DirectorySyncScheduler.Run(runCtx)`，用 `time.Ticker` 周期扫所有启用 OIDC 的 workspace，按各自 `sso.sync_interval`（默认 `1h`，`0`=禁用）决定是否插入 job。

**执行 worker**：`DirectorySyncDispatcher.Run(runCtx)`，与现有 dispatcher 同构——轮询 `pending` job，claim，执行 5.3 流程，写回 `succeeded`/`failed`。

**新增 runtime goroutine**：在 `internal/cli/server.go` 的 runtime 编排里挂入 scheduler + dispatcher（随 `runCtx` 取消，纳入现有 drain 流程）。

### 5.5 增量与幂等

- 整个同步过程幂等：重复执行结果一致。
- 不做 delta 同步（yaoguang 无分页无游标，全量比对即可）。

### 5.6 错误处理

- yaoguang 不可达 / 401 / 403 → 同步失败，job 标记 `failed` + ErrorMessage，不影响现有成员。
- 单个 member 解析失败 → 跳过该成员，继续其余，最后汇总进 StatsJSON。

## 6. 链路 B：OIDC 浏览器登录

### 6.1 OIDC RP 实现（`internal/auth/oidc`，纯 Go 包）

依赖：`github.com/coreos/go-oidc/v3/oidc` + `golang.org/x/oauth2`。

职责（不依赖 GORM/Cobra/HTTP server）：

- `Provider`：按 `issuer_base_url` 做 discovery（`{base}/.well-known/openid-configuration`），缓存 JWKS，验证 id_token 签名 + iss + aud + exp。
- `AuthCodeURL(state, pkceVerifier, redirectURI)`：生成 IdP 授权跳转 URL。
- `Exchange(code, pkceVerifier, redirectURI)`：code → id_token，校验 aud/iss/exp，返回 `*oidc.IDToken`（含 `sub`）。

### 6.2 OIDCAuthService 流程（`internal/app`）

```
Start(workspaceID):
  1. OIDCConfigService.Get(workspaceID) → 未启用则 404
  2. 生成 state(crypto/rand 32B) + PKCE verifier(crypto/rand)，存 BrowserAuthFlow
     (state → workspaceID + verifier + created_at + expires_at(10min))
  3. 返回 oidc.AuthCodeURL(state, verifier, redirectURI)
     redirectURI = {external_base_url}/sso/oidc/callback

Callback(state, code):
  1. 查 BrowserAuthFlow by state → 取 workspaceID + verifier
     - 未命中/过期 → 400 invalid_state
  2. oidc.Exchange(code, verifier, redirectURI) → id_token
     - 失败 → 400 id_token_invalid
  3. sub = id_token.Subject
  4. UserExternalIDRepo.Find(provider=yaoguang, external_id=sub)
     - 未命中 → 403 identity_not_found（不做 JIT 自动开通）
  5. 校验 Membership(workspaceID, userID) 存在且未禁用
     - 不存在/禁用 → 403 membership_inactive
  6. 创建 BrowserSession:
     - id (crypto/rand token), user_id, workspace_id
     - expires_at = now + session_ttl (默认 7d)
  7. 删除 BrowserAuthFlow(state)
  8. 返回 session → HTTP 层 set cookie + 302 redirect 到 web console

Logout(sessionID):
  1. 删除 BrowserSession
  2. 清 cookie, redirect 到登录页
```

### 6.3 持久化（`internal/storage`）

新增 model + repo：

| Model | 字段 |
|---|---|
| `BrowserSession` | `ID / UserID / WorkspaceID / ExpiresAt / CreatedAt / LastSeenAt` |
| `BrowserAuthFlow` | `State / WorkspaceID / PKCEVerifier / CreatedAt / ExpiresAt` |

- Session ID 用 `crypto/rand` 生成，**存哈希**（类比现有 `ApiToken` 的 hash 存储），cookie 里放明文 token，服务端 hash 后比对。
- 清理：定时清理过期 session/flow（并入 runtime goroutine ticker）。

### 6.4 HTTP 路由（`internal/httpapi`）

新增**不挂 authMiddleware** 的路由：

- `GET /sso/oidc/start?workspace=<slug|id>` → Start，返回 302 到 IdP
- `GET /sso/oidc/callback?state=...&code=...` → Callback，成功 set cookie + 302 回 console；失败 302 到登录页带 `?error=...`
- `POST /auth/logout` → Logout（从 cookie 取 sessionID）

## 7. 安全模型

### 7.1 Cookie / CSRF 边界（关键）

两条通道权限范围明确区分：

| 通道 | 鉴权来源 | 允许的操作 |
|---|---|---|
| **Bearer**（PAT/agent/tenant/admin） | `Authorization` header | 全部 `/api/v1/*` 读写 |
| **Cookie**（browser_session） | `HttpOnly` cookie | 仅 `GET /api/v1/*`（读）+ `GET /sso/*` + `POST /auth/logout` |

`authMiddleware` 判定逻辑：

1. 解析 method：`POST/PUT/PATCH/DELETE` 且 path 以 `/api/v1/` 开头 → **必须有 Bearer**；只有 cookie → 403 `cookie_write_forbidden`。
2. 其余（`GET /api/v1/*`、`/sso/*`、`/auth/logout`）→ Bearer 优先，无 Bearer 则试 cookie。

> Web Console 写操作（如建任务）需用 PAT/agent token（现状不变），cookie 只解决「免输 token 看一眼」。此约束让 browser session 不会被 CSRF 利用做恶意写入。

### 7.2 Cookie 属性

- `Name`：`xuanchu_session`
- `HttpOnly`（JS 不可读）+ `Secure`（仅 HTTPS，本地 dev 用 `sso.insecure_cookie` 关闭）+ `SameSite=Lax`
- `Path=/`，`Max-Age=session_ttl`
- Session ID 存哈希，cookie 里放明文 token，服务端 hash 后比对

### 7.3 OIDC 协议安全

- **state**：`crypto/rand` 32 字节，存 `BrowserAuthFlow` 表（短 TTL 10min），callback 校验一次性 + 过期失效。
- **PKCE**：每个 flow 独立 `code_verifier`（`crypto/rand`），`S256` challenge，不把 client_secret 暴露给浏览器。
- **id_token 校验**：discovery 拉 JWKS，验签名 + `iss` = `{issuer_base_url}` + `aud` = `client_id` + `exp` 未过。
- **redirect_uri 校验**：必须等于配置的 `{external_base_url}/sso/oidc/callback`，防开放重定向。

### 7.4 配置安全

- `client_secret` / `directory_access_token`：ConfigRepository 加密存储，脱敏读取，永不进日志/audit/响应明文。
- 配置写入接口限 workspace owner/admin。

### 7.5 错误码与用户可读信息

| 场景 | HTTP | code | 用户提示 |
|---|---|---|---|
| workspace 未启用 OIDC | 404 | `sso_not_enabled` | 该工作区未启用 SSO |
| state 无效/过期 | 400 | `invalid_state` | 登录请求已过期，请重试 |
| id_token 校验失败 | 400 | `id_token_invalid` | IdP 认证失败，请重试 |
| sub 未命中映射 | 403 | `identity_not_found` | 未在成员中找到该身份，请联系管理员同步通讯录 |
| membership 不存在/禁用 | 403 | `membership_inactive` | 您不是该工作区的成员 |
| cookie 写操作 | 403 | `cookie_write_forbidden` | 此操作需要 access token |
| directory 同步并发 | 409 | `sync_in_progress` | 已有同步任务进行中 |

## 8. 测试策略

- `internal/auth/oidc`：用 `httptest.Server` mock IdP（discovery + token + JWKS），覆盖 code exchange、id_token 验签失败、PKCE 不匹配、过期。
- `internal/auth/directory`：mock yaoguang directory API，覆盖全量拉取、空列表、401/403、字段缺失。
- `internal/app`：`DirectorySyncService`（新建/upsert/移除 disabled/Name 冲突）、`OIDCAuthService`（命中/未命中/过期 flow）。
- `internal/storage`：`BrowserSession` / `BrowserAuthFlow` / `DirectorySyncJob` repo CRUD + 过期清理。
- `internal/httpapi`：SSO 路由端到端（mock auth service）、cookie 写操作拒绝、authMiddleware 双通道。
- 集成测试：完整流程（配置 → 同步 → 登录 → cookie 读 → logout）。
- 全程满足 `CGO_ENABLED=0`：
  ```bash
  go test ./...
  CGO_ENABLED=0 go test ./...
  CGO_ENABLED=0 go build ./cmd/xuanchu
  ```

## 9. 文档同步清单

milestone 完成后需更新：

- `README.md`：新增 SSO 登录说明、cookie 限制说明。
- `ROADMAP.md`：标记 v0.5.0 OIDC 项完成，更新「企业 SSO」条目状态。
- `docs/manual/web-console.md`：补充 OIDC 登录入口、通讯录同步操作说明、cookie 只读约束。
- 本 spec 对应的 implementation plan。
