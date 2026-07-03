# Workspace OIDC 接入设计（yaoguang IdP）

- 日期：2026-07-03
- 状态：草案
- 里程碑：v0.5.0
- 关联文档：[ROADMAP.md](../../../ROADMAP.md)、[README.md](../../../README.md)、`docs/superpowers/specs/2026-06-19-xuanchu-authz-decision-design.md`

## 1. 背景与目标

### 1.1 目标

让每个 workspace 可选择接入一个 OIDC 提供方（本期固定对接 [yaoguang](../../../../../yaoguang)），实现：

1. **浏览器 SSO 登录**：用户通过 yaoguang 完成 OIDC 登录后，免输 xuanchu token 直接访问 Web Console，并按璇础本地 membership role 执行读写授权。
2. **通讯录同步开通成员**：自然人 workspace owner 或具备 `workspace:write` 的 tenant actor 触发同步，从 yaoguang 拉取组织成员，建立本地 user、身份映射与 membership，免去手动开通。

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
- 不把 browser session 用作 CLI、Remote Client、MCP 或外部自动化凭证；这些入口继续使用 PAT / Agent token / `tenant_access_token`。
- 不把 OIDC 登录后的用户 token 透传为璇础 API token；浏览器态只使用服务端可撤销的 opaque session cookie。

## 2. 现状与扩展点

| 既有扩展点 | 位置 | 复用方式 |
|---|---|---|
| `CredentialBrowserSession` 凭证类型（当前仅预留枚举位，代码注释仍写“本次不实现”） | `internal/authz/model.go` | 本轮改为 OIDC 登录产生的 browser session 凭证类型，并接入现有授权决策 |
| `UserExternalID (Provider, ExternalID)` 身份映射表 + repo | `internal/storage/models.go`、`external_id_repo.go` | OIDC sub 与 IM external identity 的映射落点 |
| `ConfigRepository` workspace 级 KV | `internal/storage/config_repo.go`、`internal/app/scoped_config.go` | 存储 workspace 级 OIDC/directory 配置；当前只是明文 KV，本轮必须在 app 层补 secret envelope 加密与脱敏 |
| `/sso/{provider}?redirect=` redirect 语义（v0.4.2 预留） | ROADMAP | 落地为 `/sso/oidc/*` 路由 |
| 现有 hook/notification dispatcher（DB 轮询 + claim/lease） | `internal/hookruntime`、`internal/notificationruntime` | directory sync job 的持久化执行范式 |
| HTTP server graceful shutdown / runtime goroutine 管理 | `internal/cli/server.go` | 挂入新的 sync scheduler/dispatcher goroutine |

**核心新增基础设施**：当前服务端只有 Bearer 鉴权、无 cookie/session 表。需新增 `BrowserSession` / `BrowserAuthFlow` 持久化，并改造 `authMiddleware` 支持「Bearer 优先、Cookie 兜底」双通道。

## 3. 整体架构

两条独立但共享配置与身份映射的链路：

```
┌─────────────────────────────────────────────────────────────┐
│  链路 A：owner/tenant actor 通讯录同步（建立身份与成员关系）  │
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
│  - 后续 /api/v1/* 仍走本地 membership/role/scope 授权         │
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
| `sso.client_secret` | string(secret) | 是 | OIDC client_secret。加密存储。通讯录同步也复用此凭证通过 `client_credentials` grant 自动换取 directory 访问 token |
| `sso.scopes` | string | 否 | OIDC 请求的 scope，空则默认 `openid profile email` |
| `sso.redirect_path` | string | 否 | 回调路径，默认 `/sso/oidc/callback` |
| `sso.external_base_url` | string | 否 | xuanchu 自身外部可达 URL，用于拼 `redirect_uri`。缺省时回退到 Host header |
| `sso.session_ttl` | duration | 否 | browser session 有效期，默认 `168h`（7 天） |
| `sso.sync_interval` | duration | 否 | 定时同步周期，默认 `1h`，`0` 表示禁用定时（仅手动） |
| `sso.insecure_cookie` | bool | 否 | 本地 dev 用，关闭 cookie 的 `Secure` 属性，默认 `false` |

### 4.2 安全处理

- `ConfigRepository` 当前没有加密能力，只有 `configs.value` 明文列。本轮不新增独立 secret 表，仍使用同一张 `configs` 表，但必须在 `OIDCConfigService` 写入 secret key 时做 envelope 加密。
- secret 存储格式：`enc:v1:<base64(nonce+ciphertext)>`。加密算法使用 AES-256-GCM，密钥从 TOML 配置 `[security].config_secret_key` 读取，格式固定为 32 字节随机值的 base64 编码。缺少密钥时，`PUT /sso/config` 不允许保存 `client_secret`，返回 `config_secret_key_missing`。
- 读取时只在 app 层短暂解密，绝不进入日志/响应/audit 明文。API 读取只返回脱敏视图（只露前后 2 位），写时接收完整值；secret 字段留空表示不覆盖原值。

### 4.3 配置入口（仅 Web Console）

OIDC 配置是 **workspace 级管理功能**，出现在用户登录 workspace 后的 Web Console 内。配置管理与读取仅走 HTTP API（`PUT/GET /api/v1/workspaces/{id}/sso/config`），不做 CLI / MCP 入口。

- 写入：`PUT /api/v1/workspaces/{id}/sso/config`（接收完整值），仅自然人 workspace owner 或具备 `workspace:write` 的 `tenant_access_token` 可写。
- 读取（脱敏）：`GET /api/v1/workspaces/{id}/sso/config`。
- 启用判断：`OIDCConfigService.Get(workspaceID)` 返回 `(Config, enabled bool)`，`enabled=false` 时 SSO 路由对内返回 404、directory sync 拒绝执行。

### 4.4 Web Console 配置页设计

#### 4.4.1 导航落点

在现有侧边栏新增独立 nav 项 **「单点登录」**（`/sso`，`KeyRound` 图标），放在 `tokens` 之后、与 tokens/members 平级，都是 workspace 级管理功能。

需改动的前端骨架点（基于现有代码模式）：

- `web/src/components/AppShell.tsx`：`PageKey` 联合类型加 `sso`；`navItems`（47-61 行）加一项 `{ key: "sso", icon: KeyRound, to: "/sso" }`。
- `web/src/routes/router.tsx`：`workspaceRootRoute.addChildren`（207-221 行）加 `ssoRoute`，路径 `/sso`，仿 `tokensRoute`（201-205 行）薄包装。
- 新增 `web/src/routes/workspace/SsoRoute.tsx`（路由包装）+ `web/src/features/workspace/sso/sso-config-page.tsx`（页面实现）+ `web/src/features/workspace/sso/use-sso.ts`（数据层）。
- `web/src/locales/zh-CN.ts` / `en-US.ts`：新增 `nav.sso` + `sso.*` 文案组。

#### 4.4.2 页面 ASCII 原型

侧边栏整体布局（OIDC 项高亮位置）：

```
┌─────────────────────────────────────────────────────────────────┐
│ ◆璇础            my-workspace                                   │
├──────────────┬──────────────────────────────────────────────────┤
│              │                                                  │
│  ◉ 概览       │                                                  │
│    项目       │                                                  │
│    成员       │                                                  │
│    Token     │                                                  │
│  ◉ 单点登录   │   ← 新增 nav 项                                   │
│    Hook      │                                                  │
│    通知       │                                                  │
│    审计       │                                                  │
│    设置       │                                                  │
│              │                                                  │
└──────────────┴──────────────────────────────────────────────────┘
```

「单点登录」配置页内容区（仿 `token-form.tsx` 受控表单模式）：

```
┌──────────────────────────────────────────────────────────────────┐
│ 单点登录（OIDC）                                                  │
│ 通过 yaoguang IdP 让 workspace 成员使用浏览器 SSO 登录。            │
│                                                                  │
│ ── IdP 连接 ─────────────────────────────────────────────────── │
│                                                                  │
│  Issuer 根地址 *                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ https://yaoguang.example.com                             │   │
│  └──────────────────────────────────────────────────────────┘   │
│  yaoguang 服务根 URL；OIDC discovery 与通讯录接口都基于此地址。     │
│                                                                  │
│  组织 ID *                                          │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ org_a1b2c3...                                            │   │
│  └──────────────────────────────────────────────────────────┘   │
│  yaoguang 的 organization id，用于通讯录接口路径。                  │
│                                                                  │
│  Client ID *                                                     │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ xuanchu_rp_client_01                                     │   │
│  └──────────────────────────────────────────────────────────┘   │
│  xuanchu 在 yaoguang 注册的 internal app client_id。              │
│                                                                  │
│  Client Secret *                              [已设置 ••••78ab] │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ •••••••••••••••••••••••••••••••••••                       │   │
│  └──────────────────────────────────────────────────────────┘   │
│  OIDC client_secret，加密存储。留空保存表示不修改。                  │
│                                                                  │
│ ── 通讯录同步 ───────────────────────────────────────────────── │
│                                                                  │
│  通讯录访问令牌 *                          [已设置 ••••34ef]     │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ •••••••••••••••••••••••••••••••••••                       │   │
│  └──────────────────────────────────────────────────────────┘   │
│  调用 yaoguang 通讯录接口的 tenant_access_token，需覆盖             │
│  org.members.read scope 且 directory_access=org_read。            │
│                                                                  │
│  同步周期                                  [ 1 小时 ▾]            │
│  定时拉取通讯录的间隔；选「禁用」则仅手动触发。                       │
│                                                                  │
│ ── 高级（可选）────────────────────────────────────────────── │
│                                                                  │
│  外部可达地址                                                     │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ https://xuanchu.example.com                              │   │
│  └──────────────────────────────────────────────────────────┘   │
│  用于拼接 OIDC redirect_uri，留空则用请求 Host。                   │
│                                                                  │
│  Session 有效期                              [ 7 天 ▾]           │
│  Browser session 有效期。                                         │
│                                                                  │
│                          [取消]  [保存配置]                       │
│                                                                  │
│ ── 成员同步 ─────────────────────────────────────────────────── │
│                                                                  │
│  上次同步：2026-07-03 14:20  +12  -1  ~3                         │
│                                                                  │
│                                  [立即同步成员]                   │
└──────────────────────────────────────────────────────────────────┘
```

#### 4.4.3 页面交互细节

- **首次进入**：所有字段为空，显示「未配置」状态，提示 owner 或 tenant actor 填写 IdP 信息。secret 类字段显示占位 `[未设置]`。
- **编辑已有配置**：`GET` 返回脱敏值（secret 露前后 2 位，如 `••••78ab`）。secret 字段渲染为 password 输入框，**留空保存表示不修改原值**；填了新值则覆盖。
- **保存**：本地校验必填项（issuer_base_url / org_id / client_id / client_secret / directory_access_token）→ `PUT` 提交 → 成功后 toast 提示 + invalidate query。
- **立即同步成员**：`POST /api/v1/workspaces/{id}/sso/sync` → 成功后轮询最近 job 状态，展示 `+N -M ~K` 结果。
- **权限**：SSO 配置菜单与页面仅对两类身份可见/可写：**(1) 自然人 workspace owner**（`effective_role === "owner"`）；**(2) workspace 系统身份**（`tenant_access_token` 登录 Console，具备 `workspace:write`）。member/admin/viewer 不可见。server admin 进入 workspace 时应通过现有 tenant-switch/acting 链路获得对应 workspace 系统身份或 owner 视角，不再单独给 `admin acting` 开 SSO 特权。
  - 前端 nav 显隐（无现成先例，需新增 filter）：`navItems` 改为带 `requireSsoVisible?: boolean` 标记的结构，渲染时按 `showSso = (me.data?.effective_role === "owner") || tokenType === "tenant_access_token" || getTenantSwitchContext() !== null` 过滤。直接访问 `/sso` 路径时若无权限则隐藏内容并提示「无权限」。
  - 后端权限校验（照 `PermissionWorkspaceArchive` owner-only 范式）：新增 `PermissionSsoConfigRead` / `PermissionSsoConfigWrite`，自然人仅 `RoleOwner` 放行（admin/member/viewer 拒绝）；tenant actor 路径在 `tenantCapabilityForPermission` 补 capability 映射：读映射 `workspace:read`，写映射 `workspace:write`。普通 PAT/agent token 只有 owner 用户能通过。

#### 4.4.4 字段与配置 key 对照

| 页面字段 | 配置 key | 必填 | 输入类型 |
|---|---|---|---|
| Issuer 根地址 | `sso.issuer_base_url` | 是 | text |
| 组织 ID | `sso.org_id` | 是 | text |
| Client ID | `sso.client_id` | 是 | text |
| Client Secret | `sso.client_secret` | 是 | password（脱敏读，留空不改） |
| 通讯录访问令牌 | `sso.directory_access_token` | 是 | password（脱敏读，留空不改） |
| 同步周期 | `sso.sync_interval` | 否 | select（禁用 / 30m / 1h / 6h / 24h） |
| 外部可达地址 | `sso.external_base_url` | 否 | text |
| Session 有效期 | `sso.session_ttl` | 否 | select（1d / 7d / 30d） |

> `sso.provider`（固定 `yaoguang`）、`sso.redirect_path`（默认 `/sso/oidc/callback`）、`sso.scopes`、`sso.insecure_cookie` 不在表单暴露，由后端默认值处理。

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

- 手动：`POST /api/v1/workspaces/{id}/sso/sync`（同 SSO 配置写权限：自然人 owner 或具备 `workspace:write` 的 tenant actor）→ 插入 `pending` job → 立即返回 job id；若该 workspace 已有 `pending`/`running` → 409 `sync_in_progress`。Web Console 轮询 `GET /api/v1/workspaces/{id}/sso/sync/jobs/{id}` 查状态。
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
     - id (crypto/rand token), user_id, workspace_id, csrf_hash
     - expires_at = now + session_ttl (默认 7d)
  7. 删除 BrowserAuthFlow(state)
  8. 返回 session token + csrf token → HTTP 层设置 session/csrf cookie + 302 redirect 到 web console

Logout(sessionID):
  1. 删除 BrowserSession
  2. 清 cookie, redirect 到登录页
```

### 6.3 持久化（`internal/storage`）

新增 model + repo：

| Model | 字段 |
|---|---|
| `BrowserSession` | `ID / UserID / WorkspaceID / CSRFHash / ExpiresAt / CreatedAt / LastSeenAt` |
| `BrowserAuthFlow` | `State / WorkspaceID / PKCEVerifier / CreatedAt / ExpiresAt` |

- Session ID 用 `crypto/rand` 生成，**存哈希**（类比现有 `ApiToken` 的 hash 存储），cookie 里放明文 token，服务端 hash 后比对。
- CSRF token 同样用 `crypto/rand` 生成，服务端只存哈希；浏览器拿到非 HttpOnly 的 CSRF cookie，后续写请求必须带 `X-Xuanchu-CSRF`。
- 清理：定时清理过期 session/flow（并入 runtime goroutine ticker）。

### 6.4 HTTP 路由（`internal/httpapi`）

新增**不挂 authMiddleware** 的路由：

- `GET /sso/oidc/start?workspace=<slug|id>` → Start，返回 302 到 IdP
- `GET /sso/oidc/callback?state=...&code=...` → Callback，成功 set cookie + 302 回 console；失败 302 到登录页带 `?error=...`
- `POST /auth/logout` → Logout（从 cookie 取 sessionID）

## 7. 安全模型

### 7.1 Cookie / CSRF 边界（关键）

两条通道的使用场景明确区分：

| 通道 | 鉴权来源 | 允许的操作 |
|---|---|---|
| **Bearer**（PAT/agent/tenant/admin） | `Authorization` header | 全部 `/api/v1/*` 读写 |
| **Cookie**（browser_session） | `HttpOnly` session cookie + CSRF token | Web Console 的 `/api/v1/*` 读写；写操作必须通过 CSRF 校验；不允许访问 `/mcp`、Remote Client 或 server admin API |

`authMiddleware` 判定逻辑：

1. Bearer 优先：有 `Authorization: Bearer` 时走现有 token 认证，不读取 cookie。
2. 无 Bearer 时，仅普通 `/api/v1/*` 允许尝试 browser session cookie；`/mcp`、`/api/v1/admin/*`、Remote Client 相关入口必须继续要求 Bearer。
3. 对 `POST/PUT/PATCH/DELETE` 等写请求，cookie 模式必须同时满足：
   - `xuanchu_session` HttpOnly cookie 有效；
   - `xuanchu_csrf` cookie 存在；
   - 请求头 `X-Xuanchu-CSRF` 与 `xuanchu_csrf` cookie 一致；
   - 服务端哈希后等于 `BrowserSession.CSRFHash`。

这样 OIDC 登录后的用户可以正常使用 Web Console，而 CLI/MCP/Agent 的机器凭证边界仍保持不变。

### 7.2 Cookie 属性

- `Name`：`xuanchu_session`
- `HttpOnly`（JS 不可读）+ `Secure`（仅 HTTPS，本地 dev 用 `sso.insecure_cookie` 关闭）+ `SameSite=Lax`
- `Path=/`，`Max-Age=session_ttl`
- Session ID 存哈希，cookie 里放明文 token，服务端 hash 后比对
- 另设 `xuanchu_csrf`：非 HttpOnly、`Secure`、`SameSite=Lax`、`Path=/`。该 cookie 只存 CSRF 明文，不能当作认证凭证。

### 7.3 OIDC 协议安全

- **state**：`crypto/rand` 32 字节，存 `BrowserAuthFlow` 表（短 TTL 10min），callback 校验一次性 + 过期失效。
- **PKCE**：每个 flow 独立 `code_verifier`（`crypto/rand`），`S256` challenge，不把 client_secret 暴露给浏览器。
- **id_token 校验**：discovery 拉 JWKS，验签名 + `iss` = `{issuer_base_url}` + `aud` = `client_id` + `exp` 未过。
- **redirect_uri 校验**：必须等于配置的 `{external_base_url}/sso/oidc/callback`，防开放重定向。

### 7.4 配置安全

- `client_secret` / `directory_access_token`：ConfigRepository 加密存储，脱敏读取，永不进日志/audit/响应明文。
- 配置写入接口限自然人 workspace owner 或具备 `workspace:write` 的 tenant actor。

### 7.5 错误码与用户可读信息

| 场景 | HTTP | code | 用户提示 |
|---|---|---|---|
| workspace 未启用 OIDC | 404 | `sso_not_enabled` | 该工作区未启用 SSO |
| state 无效/过期 | 400 | `invalid_state` | 登录请求已过期，请重试 |
| id_token 校验失败 | 400 | `id_token_invalid` | IdP 认证失败，请重试 |
| sub 未命中映射 | 403 | `identity_not_found` | 未在成员中找到该身份，请联系管理员同步通讯录 |
| membership 不存在/禁用 | 403 | `membership_inactive` | 您不是该工作区的成员 |
| CSRF 缺失或不匹配 | 403 | `csrf_invalid` | 页面会话已过期，请刷新后重试 |
| directory 同步并发 | 409 | `sync_in_progress` | 已有同步任务进行中 |

## 8. 测试策略

- `internal/auth/oidc`：用 `httptest.Server` mock IdP（discovery + token + JWKS），覆盖 code exchange、id_token 验签失败、PKCE 不匹配、过期。
- `internal/auth/directory`：mock yaoguang directory API，覆盖全量拉取、空列表、401/403、字段缺失。
- `internal/app`：`DirectorySyncService`（新建/upsert/移除 disabled/Name 冲突）、`OIDCAuthService`（命中/未命中/过期 flow）。
- `internal/storage`：`BrowserSession` / `BrowserAuthFlow` / `DirectorySyncJob` repo CRUD + 过期清理。
- `internal/httpapi`：SSO 路由端到端（mock auth service）、cookie 读写、CSRF 缺失拒绝、authMiddleware 双通道、`/mcp` cookie 拒绝。
- 集成测试：完整流程（配置 → 同步 → 登录 → cookie 读 → CSRF 写 → logout）。
- 全程满足 `CGO_ENABLED=0`：
  ```bash
  go test ./...
  CGO_ENABLED=0 go test ./...
  CGO_ENABLED=0 go build ./cmd/xuanchu
  ```

## 9. 文档同步清单

milestone 完成后需更新：

- `README.md`：新增 SSO 登录说明、browser session 与机器 token 边界说明、CSRF 约束说明。
- `ROADMAP.md`：标记 v0.5.0 OIDC 项完成，更新「企业 SSO」条目状态。
- `docs/manual/web-console.md`：补充 OIDC 登录入口、通讯录同步操作说明、browser session 与 CSRF 行为。
- 本 spec 对应的 implementation plan。
