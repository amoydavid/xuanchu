# SSO 用户创建的 PAT 不能登录 Web Console

**日期：** 2026-07-06
**状态：** 草案
**背景：** v0.5.3 放开了 `browserSessionScopes` 的 `token:write`，owner 可在 Web Console（SSO browser session）创建 tenant token / PAT / Agent token。但「防止 SSO 用户创建 PAT 绕过 SSO 登录 Console」这条原安全意图需要用更精细的方式守住。

## 1. 背景与现状

- Web Console 登录支持两种方式：token 登录（PAT/Agent/tenant）和 SSO（OIDC browser session）。
- 登录页用 `GET /api/v1/credentials/current` 校验 token，只要 Bearer token 能通过鉴权即登录成功，**后端没有「PAT 能否登录 Console」的专门校验**。
- v0.5.3 之前 `browserSessionScopes` 排除 `token:write`，导致 owner 通过 SSO 登录后无法在 Console 创建 tenant token（报 403）。该限制一刀切也拦住了合法的 tenant token 创建。
- v0.5.3 放开 `token:write` 后，owner 可创建 token，但「SSO workspace 里用 PAT 绕过 SSO 登 Console」的口子重新打开。

## 2. 目标

1. owner/admin 通过 SSO browser session 在 Web Console 创建的 PAT / Agent token，**不能用于 Web Console 登录页登录**。
2. 这些 token 在 HTTP API / HTTP MCP / 远程 CLI / stdio MCP 等其他场景**仍正常可用**。
3. tenant token 不受影响（机器身份，本就不是给人登 Console 用的）。
4. CLI / Bearer token 创建的 PAT 不受影响，可正常登录 Console。
5. 不引入 workspace 级「强制 SSO」开关（独立功能，不在本 spec 范围）。

## 3. 非目标

- 不提供「标记改写」API（标记只在创建时写入，创建后不可改）。
- 不对 tenant token 打标。
- 不强制所有 workspace 走 SSO。
- 不回退 v0.5.3 对 `token:write` 的放开（本方案建立在其之上）。

## 4. 核心产品决策

### 4.1 按创建来源打标，而非按角色或 workspace

「SSO 用户创建的 PAT」界定为：**由 SSO browser session 创建的 PAT/Agent token**（`RuntimeContext.WebLoginDisabled = true` 时创建）。admin/owner 也打标（边界统一：SSO workspace 里所有人工 token 走 SSO 登 Console）。不按 workspace SSO 开关一刀切，避免影响非 SSO 用户。

### 4.2 仅禁 Console 登录入口

标记的 token 只在 Web Console 登录页（`/api/v1/credentials/current` 校验）被拒，其他场景照常使用。理由：PAT 是给自动化用的，不是给人登 Console 用的；登录入口是「人」的动作，禁这个入口即可守住边界，不影响自动化用途。

### 4.3 后端兜底拒绝

拒绝点在 `handleCredentialsCurrent`（后端），而非前端判断。理由：安全边界应在后端，防止前端绕过。

## 5. 后端设计

### 5.1 数据模型

`internal/storage/models.go` 的 `ApiToken` 新增：

```go
WebLoginDisabled bool `gorm:"not null;default:false"`
```

`ApiTokenEntry` 同步新增，`apiTokenModel` / `apiTokenEntry` 双向映射。

### 5.2 来源透传与打标

`RuntimeContext` 新增 `WebLoginDisabled bool`。

- `internal/httpapi/middleware.go`：`handleCookieAuth` 构造的临时 token `Type` 用常量 `browserSessionTokenType`（="browser_session"）。
- `internal/httpapi/app_service.go`：`scopedServiceFor` 在 `AuthorizeTokenRequest` 后，若 `authn.Authn.Token.Type == browserSessionTokenType`，设 `authorized.Runtime.WebLoginDisabled = true`（因为 `runtimeContextFromDecision` 把所有非 tenant actor 归一化成 `ActorUser`，来源信息在 app 层拿不到，需在此重新透传）。
- `internal/app/token.go`：`createTokenStored` 构造 `stored` 时，`WebLoginDisabled: s.runtime.WebLoginDisabled`。`withStore` 克隆会复制 runtime 值，所以 `withAudit` 的 tx 也带标记。
- `CreateTenantAccessToken` **不打标**。

### 5.3 TokenView 透传

`app.TokenView` 新增 `WebLoginDisabled bool`，`tokenViewFromEntry` 填充。`AuthenticateBearerToken` 走 `tokenViewFromEntry`，所以鉴权后 `authn.Authn.Token.WebLoginDisabled` 可用。

### 5.4 登录拒绝

`internal/httpapi/me.go` 的 `handleCredentialsCurrent`：user 分支（非 tenant actor）在 `UserInfo` 调用前检查：

```go
if authn.Authn.Token.WebLoginDisabled {
    writeError(w, http.StatusForbidden, "token_web_login_disabled", "该令牌不能用于 Web Console 登录，请使用 SSO 登录", nil)
    return
}
```

错误码 `token_web_login_disabled` → HTTP 403（`error_status.go` 已映射）。

## 6. 前端设计

### 6.1 LoginPage 错误码映射

`web/src/pages/LoginPage.tsx` 的 catch 块改为按 `ApiError.code` 映射 i18n 文案：

```ts
if (err instanceof ApiError && err.code) {
  const codeKey = `auth.errors.${err.code}`
  const translated = t(codeKey)
  setError(translated === codeKey ? t("auth.failed") : translated)
} else {
  setError(t("auth.failed"))
}
```

`admin_setup_required` 特殊跳转保留。其余未知码回退 `auth.failed`。

### 6.2 i18n 文案

```ts
auth.errors.token_web_login_disabled = "该令牌由 SSO 会话创建，不能用于登录 Web Console，请使用 SSO 登录。"
```

英文同步。

## 7. 安全边界

1. **只禁 Console 登录入口**，不禁 API/MCP/CLI。
2. **后端兜底拒绝**，前端只是消费方。
3. **admin/owner 一视同仁打标**，SSO workspace 里所有人工 token 走 SSO 登 Console。
4. **tenant token 不打标**（机器身份）。
5. **CLI/Bearer 创建的 PAT 不打标**，可正常登录 Console。

## 8. 验收标准

- owner 通过 SSO browser session 创建 PAT/Agent token，token 行 `web_login_disabled=true`。
- 用该 token 在 Console 登录页登录 → 被拒（403），展示「该令牌由 SSO 会话创建，不能用于登录 Web Console」。
- 同一 token 调 `GET /api/v1/tasks`（HTTP API）、HTTP MCP、远程 CLI → 正常工作。
- tenant token、CLI/Bearer 创建的 PAT → 不带标记，可正常登录 Console。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。
- `pnpm typecheck`、`pnpm test`、`pnpm build` 通过。

## 9. 实施顺序

1. Storage：`ApiToken.WebLoginDisabled` + entry 映射 + 测试。
2. App：`RuntimeContext.WebLoginDisabled` + `CreateToken` 打标 + `TokenView` 透传 + 测试。
3. HTTP：`scopedServiceFor` 来源透传 + `handleCredentialsCurrent` 拒绝 + error status + 测试（含端到端）。
4. 前端：LoginPage 错误码映射 + i18n + 测试。
5. 文档：README + web-console manual + spec + ROADMAP。
