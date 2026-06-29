# Xuanchu Tenant Access Token Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 workspace 级 `tenant_access_token`，让 HTTP API 和 HTTP MCP 可以使用类似 OpenAI API key 的租户 token，同时保持 PAT / Agent / Admin acting token 行为不回归。

**Architecture:** 复用现有 `api_tokens` 表作为统一 token 表，用 `type=tenant_access_token`、`user_id=NULL`、单 workspace allowlist 表达租户凭证。认证后生成无 user 的 tenant credential 分支，授权只检查 scope、workspace binding、project allowlist 和 tool / endpoint 禁止清单，不读取 membership role。Web Console 在现有 token 页面加 tab，管理 API 只允许普通 PAT / Agent 的 workspace owner/admin 或 server admin admin API 管理 tenant token。

**Tech Stack:** Go 1.25、GORM、SQLite/PostgreSQL、chi/Huma HTTP、MCP Go SDK、React/Vite、TanStack Query、Vitest。

---

## Source Spec

- `docs/superpowers/specs/2026-06-29-xuanchu-tenant-access-token-design.md`

## File Map

Backend storage/auth:

- Modify: `internal/auth/token.go` - 新增 tenant token 类型和前缀生成。
- Modify: `internal/auth/scope.go` - 增加 tenant scope whitelist 和 wildcard 过滤。
- Modify: `internal/auth/token_test.go` - 覆盖 tenant token 生成和 scope 过滤。
- Modify: `internal/storage/models.go` - `ApiToken.UserID` 改成 nullable，`AuditLog` 增加 machine actor 字段。
- Modify: `internal/storage/token_repo.go` - `ApiTokenEntry.UserID` 改成 `*string`，新增 tenant list/type list 查询。
- Modify: `internal/storage/token_repo_test.go` - 覆盖 nullable user、tenant token 查询、普通 token 查询隔离。
- Modify: `internal/storage/audit_repo.go` - 存取 actor type / token 信息。
- Modify: `internal/storage/db.go`、`internal/storage/db_test.go` - SQLite/PostgreSQL migration 和回归测试。
- Modify: `internal/storage/postgres_test.go` - 确认表仍包含 `api_tokens` 和新增 audit 字段。

Backend app/authz:

- Modify: `internal/app/token.go` - 增加 tenant token service，保持 PAT / Agent service 隔离。
- Modify: `internal/app/request_scope.go` - `AuthorizeTokenRequest` 增加 tenant credential 分支。
- Modify: `internal/app/runtime.go` - `RuntimeContext` 增加 machine actor 字段。
- Modify: `internal/authz/decision.go` - `Decision` 增加 `Actor`，保留 `Principal` 兼容用户路径。
- Modify: `internal/app/audit.go` - 审计写入和读取 machine actor 字段。
- Modify: `internal/app/context.go` - tenant actor 禁止 active context use/none。
- Modify: `internal/app/service.go` - 无 user credential 的 service runtime 支持。
- Test: `internal/app/token_test.go`、`internal/app/request_scope_test.go`、`internal/app/audit_test.go`、`internal/app/context_test.go`

HTTP API:

- Modify: `internal/httpapi/tokens.go` - 保持普通 token API 排除 tenant token。
- Create: `internal/httpapi/tenant_tokens.go` - workspace tenant token 管理 API。
- Create: `internal/httpapi/admin_tenant_tokens.go` - admin tenant token list/edit/revoke API。
- Modify: `internal/httpapi/huma_routes.go` - 注册 tenant token routes。
- Modify: `internal/httpapi/middleware.go` - Bearer 认证识别 tenant token，request auth 支持无 user。
- Modify: `internal/httpapi/error_status.go` - 新错误码 HTTP status。
- Modify: `internal/httpapi/import_audit.go` - audit JSON 输出 machine actor。
- Test: `internal/httpapi/auth_test.go`、`internal/httpapi/tokens_test.go`、`internal/httpapi/admin_test.go`、`internal/httpapi/context_config_test.go`。

MCP:

- Modify: `internal/mcpserver/auth.go` - tenant credential service 构造。
- Modify: `internal/mcpserver/tools_misc.go` - `me_get` tenant actor 错误。
- Modify: `internal/mcpserver/tools_context.go` - active context tools 禁止 tenant token。
- Modify: `internal/mcpserver/tools_token.go`、`tools_user.go`、`tools_member.go`、`tools_workspace.go` - 禁止 tenant token 管理 user/member/token/workspace 写操作和 workspace list。
- Modify: `internal/mcpserver/resources.go` - 当前 context resource 对 tenant token 返回 `tenant_actor_not_user`。
- Test: `internal/mcpserver/auth_test.go`、`internal/mcpserver/integration_test.go`、`internal/httpapi/mcp_test.go`。

Web Console:

- Modify: `web/src/features/workspace/tokens/token-api.ts` - tenant token 类型和 API 契约。
- Modify: `web/src/features/workspace/tokens/scopes.ts` - tenant scope whitelist。
- Modify: `web/src/features/workspace/tokens/scope-editor.tsx` - 支持禁用 token/impersonate/workspace write scopes。
- Modify: `web/src/features/workspace/tokens/tokens-page.tsx` - 增加 tab。
- Create: `web/src/features/workspace/tokens/tenant-token-api.ts`
- Create: `web/src/features/workspace/tokens/tenant-token-form.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-create-dialog.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-edit-dialog.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-revoke-dialog.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-created-result.tsx`
- Modify: `web/src/features/workspace/tokens/tokens-page.test.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-form.test.tsx`
- Modify: `web/src/features/admin/tokens/admin-token-api.ts`
- Modify: `web/src/features/admin/tokens/admin-tokens-page.tsx`
- Create: `web/src/features/admin/tokens/admin-tenant-token-api.ts`
- Create: `web/src/features/admin/tokens/admin-tenant-token-edit-dialog.tsx`
- Create: `web/src/features/admin/tokens/admin-tenant-token-revoke-dialog.tsx`
- Modify: `web/src/features/admin/tokens/admin-tokens-page.test.tsx`

Docs:

- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/manual/web-console.md`
- Modify: `docs/manual/team-workspaces-projects.md`

---

## Chunk 1: Storage, Auth Constants, Scope Rules

### Task 1: 让 `api_tokens` 支持 `tenant_access_token`

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/token_repo.go`
- Modify: `internal/storage/token_repo_test.go`
- Modify: `internal/storage/db.go`
- Modify: `internal/storage/db_test.go`

- [ ] **Step 1: 写失败测试，证明 `api_tokens.user_id` 可空但普通 token 查询不混入 tenant token**

在 `internal/storage/token_repo_test.go` 新增测试：

```go
func TestTokenRepositoryTenantTokenUsesNullableUserID(t *testing.T) {
    store := newTokenRepoTestStore(t)
    repo := NewTokenRepository(store.DB())
    wsID := "ws-1"
    userID := "user-1"

    userToken := ApiTokenEntry{
        ID: "pat-1", UserID: strptr(userID), Name: "cli", Type: auth.TokenTypePAT,
        TokenPrefix: "xuanchu_pat_a", TokenHash: strings.Repeat("a", 64),
        ScopesJSON: `["task:read"]`, WorkspaceIDsJSON: `[]`, ProjectIDsJSON: `[]`, CreatedAt: 100,
    }
    tenantToken := ApiTokenEntry{
        ID: "tenant-1", UserID: nil, Name: "runtime", Type: auth.TokenTypeTenantAccess,
        TokenPrefix: "xuanchu_tenant_a", TokenHash: strings.Repeat("b", 64),
        ScopesJSON: `["task:read"]`, WorkspaceIDsJSON: `["` + wsID + `"]`, ProjectIDsJSON: `[]`, CreatedAt: 101,
    }
    if err := repo.Create(userToken); err != nil {
        t.Fatal(err)
    }
    if err := repo.Create(tenantToken); err != nil {
        t.Fatal(err)
    }

    byUser, err := repo.ListByUser(userID, true)
    if err != nil {
        t.Fatal(err)
    }
    if len(byUser) != 1 || byUser[0].ID != "pat-1" {
        t.Fatalf("ListByUser() = %#v", byUser)
    }

    tenants, err := repo.ListTenantByWorkspace(wsID, true)
    if err != nil {
        t.Fatal(err)
    }
    if len(tenants) != 1 || tenants[0].UserID != nil {
        t.Fatalf("ListTenantByWorkspace() = %#v", tenants)
    }
}
```

在 `internal/storage/token_repo_test.go` 增加本文件私有 helper：

```go
func strptr(value string) *string { return &value }
```

如果同名 helper 已存在，复用已有 helper，不要重复定义。

- [ ] **Step 2: 运行 storage 聚焦测试，确认失败**

Run:

```bash
go test ./internal/storage -run 'TestTokenRepositoryTenantTokenUsesNullableUserID|TestOpenCreatesAPITokenSchema' -count=1
```

Expected: FAIL，原因是 `ApiTokenEntry.UserID` 还是 `string` 或 `api_tokens.user_id` 仍为 NOT NULL，且缺少 `ListTenantByWorkspace`。

- [ ] **Step 3: 修改 model 和 entry**

在 `internal/storage/models.go`：

```go
type ApiToken struct {
    ID               string  `gorm:"primaryKey"`
    UserID           *string `gorm:"index:idx_api_tokens_user"`
    Name             string  `gorm:"not null"`
    Type             string  `gorm:"not null"`
    TokenPrefix      string  `gorm:"not null;uniqueIndex:idx_api_tokens_prefix"`
    TokenHash        string  `gorm:"not null"`
    ScopesJSON       string  `gorm:"not null;default:'[]'"`
    WorkspaceIDsJSON string  `gorm:"not null;default:'[]'"`
    ProjectIDsJSON   string  `gorm:"not null;default:'[]'"`
    CreatedAt        int64   `gorm:"not null"`
    ExpiresAt        *int64
    RevokedAt        *int64
    LastUsedAt       *int64
}
```

在 `internal/storage/token_repo.go`：

```go
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

更新 `apiTokenModel` / `apiTokenEntry` 直接传递 `*string`。

- [ ] **Step 4: 增加 tenant 查询 API**

在 `internal/storage/token_repo.go` 增加：

```go
func (r *TokenRepository) ListAllByType(tokenType string, includeRevoked bool) ([]ApiTokenEntry, error) {
    var rows []ApiToken
    query := r.db.Where("type = ?", tokenType)
    if !includeRevoked {
        query = query.Where("revoked_at IS NULL")
    }
    if err := query.Order("created_at DESC").Order("id DESC").Find(&rows).Error; err != nil {
        return nil, err
    }
    out := make([]ApiTokenEntry, 0, len(rows))
    for _, row := range rows {
        out = append(out, apiTokenEntry(row))
    }
    return out, nil
}

func (r *TokenRepository) ListTenantByWorkspace(workspaceID string, includeRevoked bool) ([]ApiTokenEntry, error) {
    rows, err := r.ListAllByType("tenant_access_token", includeRevoked)
    if err != nil {
        return nil, err
    }
    out := make([]ApiTokenEntry, 0, len(rows))
    for _, row := range rows {
        if jsonStringArrayContains(row.WorkspaceIDsJSON, workspaceID) {
            out = append(out, row)
        }
    }
    return out, nil
}
```

Prefer a small helper using `encoding/json`; do not string-match JSON.

- [ ] **Step 5: 保持普通列表隔离**

修改 `ListByUser`：

```go
query := r.db.Where("user_id = ? AND type IN ?", userID, []string{"pat", "agent"})
```

修改 `ListAll` 用于普通 admin token 列表时如果当前语义是“全部 PAT/Agent”，则过滤 `type IN ('pat','agent')`。如果有调用点需要真正全部，新增 `ListAllTokens`，不要改变 admin token 页面语义。

- [ ] **Step 6: 增加 SQLite 迁移测试**

在 `internal/storage/db_test.go` 增加从旧 schema 迁移的测试，旧表使用：

```sql
CREATE TABLE api_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  token_prefix TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  scopes_json TEXT NOT NULL DEFAULT '[]',
  workspace_ids_json TEXT NOT NULL DEFAULT '[]',
  project_ids_json TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL,
  expires_at INTEGER,
  revoked_at INTEGER,
  last_used_at INTEGER
)
```

Assert after `Open`:

```go
assertColumnNullable(t, store, "api_tokens", "user_id", true)
```

If no helper exists, add one using `PRAGMA table_info(api_tokens)` for SQLite.

- [ ] **Step 7: 实现 SQLite migration**

In `internal/storage/db.go`, add an idempotent migration before/around `AutoMigrate` that rebuilds `api_tokens` when SQLite reports `user_id` as NOT NULL:

1. Create `api_tokens_new` with nullable `user_id`.
2. Copy all columns.
3. Drop old table.
4. Rename.
5. Recreate indexes `idx_api_tokens_user` and `idx_api_tokens_prefix`.

Keep PostgreSQL path simple: `ALTER TABLE api_tokens ALTER COLUMN user_id DROP NOT NULL` when dialect is postgres.

- [ ] **Step 8: 跑 storage 测试**

Run:

```bash
go test ./internal/storage -run 'TokenRepository|APIToken|api_tokens|Audit' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/storage/models.go internal/storage/token_repo.go internal/storage/token_repo_test.go internal/storage/db.go internal/storage/db_test.go
git commit -m "feat: 让 api_tokens 支持租户 token"
```

### Task 2: 增加 tenant token 类型和 scope 过滤

**Files:**
- Modify: `internal/auth/token.go`
- Modify: `internal/auth/scope.go`
- Modify: `internal/auth/token_test.go`

- [ ] **Step 1: 写失败测试**

在 `internal/auth/token_test.go` 新增：

```go
func TestGenerateTenantAccessToken(t *testing.T) {
    raw, prefix, hash, err := GenerateToken(TokenTypeTenantAccess)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.HasPrefix(raw, "xuanchu_tenant_") {
        t.Fatalf("raw prefix = %q", raw)
    }
    if !strings.HasPrefix(prefix, "xuanchu_tenant_") || len(prefix) > 24 {
        t.Fatalf("prefix = %q", prefix)
    }
    if hash == "" || strings.Contains(hash, raw) {
        t.Fatalf("hash leaks raw token")
    }
}

func TestValidateTenantScopesFiltersWildcards(t *testing.T) {
    scopes, err := ValidateTenantTokenScopes([]string{"*", "*:read", "workspace:*"})
    if err != nil {
        t.Fatal(err)
    }
    if scopes.Has(ScopeTokenRead) || scopes.Has(ScopeTokenWrite) || scopes.Has(ScopeImpersonate) || scopes.Has(ScopeWorkspaceWrite) {
        t.Fatalf("tenant scopes include forbidden scope: %#v", scopes.Values())
    }
    if !scopes.Has(ScopeWorkspaceRead) || !scopes.Has(ScopeTaskRead) {
        t.Fatalf("tenant scopes missing allowed scopes: %#v", scopes.Values())
    }
}

func TestValidateTenantScopesRejectsForbidden(t *testing.T) {
    for _, value := range []string{"token:read", "token:*", "impersonate", "workspace:write", "user:*", "member:*"} {
        if _, err := ValidateTenantTokenScopes([]string{value}); err == nil {
            t.Fatalf("ValidateTenantTokenScopes(%q) expected error", value)
        }
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/auth -run 'Tenant|Token' -count=1
```

Expected: FAIL，缺少 `TokenTypeTenantAccess` / tenant scope validator。

- [ ] **Step 3: 实现 token 类型和前缀**

In `internal/auth/token.go`:

```go
const (
    TokenTypePAT          = "pat"
    TokenTypeAgent        = "agent"
    TokenTypeTenantAccess = "tenant_access_token"
    TokenTypeAdminActing  = "admin_acting"
)
```

Add in prefix switch:

```go
case TokenTypeTenantAccess:
    return "xuanchu_tenant_", nil
```

Do not make `ValidateTokenCreate` accept tenant token as a normal user token. Tenant token creation uses `ValidateTenantTokenScopes`.

- [ ] **Step 4: 实现 tenant scope validator**

In `internal/auth/scope.go`:

```go
var tenantAllowedScopes = map[string]struct{}{
    ScopeTaskRead: {}, ScopeTaskWrite: {},
    ScopeProjectRead: {}, ScopeProjectWrite: {},
    ScopeContextRead: {}, ScopeContextWrite: {},
    ScopeConfigRead: {}, ScopeConfigWrite: {},
    ScopeWorkspaceRead: {},
    ScopeAuditRead: {},
    ScopeHookRead: {}, ScopeHookWrite: {},
    ScopeNotificationRead: {}, ScopeNotificationWrite: {},
    ScopeReminderRead: {}, ScopeReminderWrite: {},
}

func ValidateTenantTokenScopes(values []string) (ScopeSet, error) {
    expanded, err := expandTenantWildcardScopes(values)
    if err != nil {
        return nil, err
    }
    out := ScopeSet{}
    for _, scope := range expanded {
        if _, ok := tenantAllowedScopes[scope]; !ok {
            return nil, fmt.Errorf("invalid tenant token scope %q", scope)
        }
        out[scope] = struct{}{}
    }
    return out, nil
}
```

`expandTenantWildcardScopes` 必须只从 `tenantAllowedScopes` 展开：

- `*` 展开为全部 tenant 白名单 scope。
- `workspace:*` 只展开为 `workspace:read`。
- `*:read` / `*:write` 只展开为 tenant 白名单里对应 action 的 scope。
- `token:*`、`user:*`、`member:*`、`impersonate`、`workspace:write` 必须直接拒绝。

不要先调用普通 `expandWildcardScopes` 后再 reject，否则 `*:read` 会因为展开出禁止 scope 而不符合 spec。

- [ ] **Step 5: 跑 auth 测试**

```bash
go test ./internal/auth -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/token.go internal/auth/scope.go internal/auth/token_test.go
git commit -m "feat: 增加租户 token 类型和 scope 规则"
```

---

## Chunk 2: App Layer, Authorization, Audit

### Task 3: 增加 tenant token app service

**Files:**
- Modify: `internal/app/token.go`
- Test: `internal/app/token_test.go`

- [ ] **Step 1: 写创建/list/modify/revoke 失败测试**

Add tests in `internal/app/token_test.go`:

```go
func TestCreateTenantAccessTokenStoresAPIKeyWithoutUser(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
        Name: "runtime-prod",
        Scopes: []string{"task:read", "task:write"},
        WorkspaceRef: svc.Runtime().WorkspaceSlug,
    })
    if err != nil {
        t.Fatal(err)
    }
    if !strings.HasPrefix(created.RawToken, "xuanchu_tenant_") {
        t.Fatalf("raw token = %q", created.RawToken)
    }
    if created.Stored.UserID != nil {
        t.Fatalf("tenant token user_id = %#v, want nil", created.Stored.UserID)
    }
    if created.View.WorkspaceID != svc.Runtime().WorkspaceID {
        t.Fatalf("workspace id mismatch")
    }
}

func TestTenantTokenModifyRejectsPATRef(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    pat, err := svc.CreateToken(CreateTokenInput{Name: "cli", Scopes: []string{"task:read"}})
    if err != nil {
        t.Fatal(err)
    }
    _, err = svc.ModifyTenantAccessToken(ModifyTenantAccessTokenInput{TokenRef: pat.View.ID, Name: strptr("bad")})
    assertRuntimeCode(t, err, "tenant_token_not_found")
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/app -run 'TenantAccessToken|Token' -count=1
```

Expected: FAIL，缺少 service types/methods。

- [ ] **Step 3: 增加 app 类型**

In `internal/app/token.go`:

```go
type CreateTenantAccessTokenInput struct {
    Name string
    Scopes []string
    WorkspaceRef string
    ProjectRefs []string
    ExpiresIn *time.Duration
}

type ModifyTenantAccessTokenInput struct {
    TokenRef string
    Name *string
    Scopes *[]string
    ProjectRefs *[]string
    ExpiresIn *time.Duration
}

type ListTenantAccessTokensInput struct {
    WorkspaceRef string
    IncludeRevoked bool
}

type TenantAccessTokenView struct {
    ID string
    Prefix string
    Name string
    Type string
    WorkspaceID string
    ProjectIDs []string
    Scopes []string
    CreatedAt int64
    ExpiresAt *int64
    RevokedAt *int64
    LastUsedAt *int64
}
```

- [ ] **Step 4: 实现 create**

Implement:

```go
func (s *Service) CreateTenantAccessToken(input CreateTenantAccessTokenInput) (CreatedTenantAccessToken, error)
```

Rules:

- `s.runtime.Role` must be owner/admin using existing `tokenManageAllowed`.
- `s.runtime.AdminActingSessionID != ""` returns `admin_acting_not_allowed`.
- Resolve workspace from `input.WorkspaceRef`; if empty use `s.runtime.WorkspaceID`.
- Workspace must equal current runtime workspace for workspace API.
- Resolve projects inside workspace.
- Use `auth.ValidateTenantTokenScopes`.
- Store in `api_tokens` with `Type=auth.TokenTypeTenantAccess`, `UserID=nil`, `WorkspaceIDsJSON` as one id.
- Audit action `tenant_token.create`.

- [ ] **Step 5: 实现 list/modify/revoke**

Rules:

- Always fetch through `TokenRepository`, then assert `entry.Type == auth.TokenTypeTenantAccess`.
- For workspace API, token's single workspace must equal runtime workspace.
- `ModifyTenantAccessToken` refuses revoked token with `tenant_token_revoked`.
- `ModifyTenantAccessToken` refuses expired token extension with `tenant_token_expired`.
- `RevokeTenantAccessToken` sets `revoked_at`.
- `ListTenantAccessTokens` uses `ListTenantByWorkspace`.

- [ ] **Step 6: 确保普通 token service 不处理 tenant token**

Update `ModifyToken`, `RevokeToken`, `tokenEntryToView`, `fillTokenViews` as needed:

- `ModifyToken` / `RevokeToken` reject `entry.Type == tenant_access_token` with `token_not_found` or `token_scope_denied`.
- `TokenView.User` remains required for PAT/Agent only.
- `ListTokens` never returns tenant token.

- [ ] **Step 7: 跑 app token 测试**

```bash
go test ./internal/app -run 'TenantAccessToken|CreateToken|ModifyToken|RevokeToken|ListTokens' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/app/token.go internal/app/token_test.go
git commit -m "feat: 增加租户 token 应用服务"
```

### Task 4: tenant credential 授权分支

**Files:**
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/authz/decision.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/request_scope_test.go`

- [ ] **Step 1: 写失败测试**

Add in `internal/app/request_scope_test.go`:

```go
func TestAuthorizeTenantTokenDoesNotRequireMembership(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
        Name: "runtime",
        Scopes: []string{"task:read"},
        WorkspaceRef: svc.Runtime().WorkspaceSlug,
    })
    if err != nil { t.Fatal(err) }

    authn := AuthenticatedToken{
        Kind: AuthenticatedCredentialTenantToken,
        Token: TokenView{
            ID: created.View.ID, Prefix: created.View.Prefix, Name: created.View.Name,
            Type: created.View.Type, WorkspaceIDs: []string{created.View.WorkspaceID},
            ProjectIDs: created.View.ProjectIDs, Scopes: created.View.Scopes,
        },
        TenantAccessToken: &created.View,
    }
    authorized, err := svc.AuthorizeTokenRequest(RequestAuthorizationInput{
        Token: authn,
        RequiredCapability: "task:read",
        RequiredPermission: PermissionTaskRead,
        WorkspaceRef: svc.Runtime().WorkspaceSlug,
    })
    if err != nil { t.Fatal(err) }
    if authorized.Runtime.ActorType != "tenant_access_token" || authorized.Runtime.ActorUserID != "" {
        t.Fatalf("runtime = %#v", authorized.Runtime)
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/app -run 'AuthorizeTenant|RuntimeContextFromDecision' -count=1
```

Expected: FAIL，当前 path 需要 user membership。

- [ ] **Step 3: 增加 actor model**

In `internal/authz/decision.go`:

```go
type ActorType string
const (
    ActorUser ActorType = "user"
    ActorTenantAccessToken ActorType = "tenant_access_token"
)
type Actor struct {
    Type ActorType
    UserID *string
    UserName string
    TokenID *string
    TokenName string
    TokenPrefix string
}
```

Add `Actor Actor` to `Decision`.

- [ ] **Step 4: 扩展 RuntimeContext**

In `internal/app/runtime.go`:

```go
ActorType string
ActorTokenID string
ActorTokenName string
ActorTokenPrefix string
```

User runtime should set `ActorType="user"` for normal local/PAT/Agent paths.

- [ ] **Step 5: 扩展 AuthenticatedToken**

In `internal/app/token.go`:

```go
type AuthenticatedCredentialKind string
const (
    AuthenticatedCredentialUserToken AuthenticatedCredentialKind = "user_token"
    AuthenticatedCredentialTenantToken AuthenticatedCredentialKind = "tenant_access_token"
    AuthenticatedCredentialAdminActing AuthenticatedCredentialKind = "admin_acting"
)
type AuthenticatedToken struct {
    Kind AuthenticatedCredentialKind
    Token TokenView
    User storage.User
    TenantAccessToken *TenantAccessTokenView
    AdminActingTrace *AdminActingTrace
}
```

Populate `Kind` for existing PAT/Agent/acting authentication.

- [ ] **Step 6: 实现 tenant authorization branch**

In `AuthorizeTokenRequest`:

```go
if input.Token.Kind == AuthenticatedCredentialTenantToken {
    return s.authorizeTenantTokenRequest(input)
}
```

`authorizeTenantTokenRequest`:

- reject `SubjectUserRef` with `token_scope_denied`.
- check capability in token scopes.
- resolve token workspace from one `WorkspaceIDs`.
- reject mismatched workspace.
- resolve project and check allowlist.
- set `Decision.Actor.Type=tenant_access_token`.
- set `RuntimeContext.ActorType`, `ActorTokenID`, `ActorTokenName`, `ActorTokenPrefix`.
- do not call `memberRepo.Get`.
- do not call `requireRolePermission`; instead map required permission to capability via existing handler's `RequiredCapability`.

- [ ] **Step 7: 跑授权测试**

```bash
go test ./internal/app -run 'AuthorizeTokenRequest|AuthorizeTenant|RuntimeContextFromDecision' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/authz/decision.go internal/app/request_scope.go internal/app/runtime.go internal/app/service.go internal/app/token.go internal/app/request_scope_test.go
git commit -m "feat: 增加租户 token 授权分支"
```

### Task 5: machine actor 审计

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/audit_repo.go`
- Modify: `internal/app/audit.go`
- Modify: `internal/httpapi/import_audit.go`
- Test: `internal/app/audit_test.go`
- Test: `internal/storage/db_test.go`

- [ ] **Step 1: 写失败测试**

In `internal/app/audit_test.go`:

```go
func TestTenantTokenAuditStoresMachineActor(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{Name: "runtime", Scopes: []string{"task:write"}})
    if err != nil { t.Fatal(err) }
    tenantSvc := newServiceWithTenantRuntimeForTest(t, svc, created.View)
    _, err = tenantSvc.Add(AddInput{Title: "from tenant"})
    if err != nil { t.Fatal(err) }
    rows, err := svc.ListAudit(AuditListInput{Limit: 10})
    if err != nil { t.Fatal(err) }
    if rows[0].ActorType != "tenant_access_token" || rows[0].ActorToken == nil {
        t.Fatalf("audit row = %#v", rows[0])
    }
}
```

同时在 `internal/app/audit_test.go` 增加测试 helper：

```go
func newServiceWithTenantRuntimeForTest(t *testing.T, parent *Service, view TenantAccessTokenView) *Service {
    t.Helper()
    svc := *parent
    svc.runtime.ActorType = "tenant_access_token"
    svc.runtime.ActorUserID = ""
    svc.runtime.ActorName = view.Name
    svc.runtime.ActorTokenID = view.ID
    svc.runtime.ActorTokenName = view.Name
    svc.runtime.ActorTokenPrefix = view.Prefix
    svc.runtime.WorkspaceID = view.WorkspaceID
    return &svc
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/app ./internal/storage -run 'TenantTokenAudit|AuditLog' -count=1
```

Expected: FAIL，缺少 columns/view fields。

- [ ] **Step 3: 扩展 storage model/repo**

Add to `AuditLog` and `AuditLogEntry`:

```go
ActorType string
ActorTokenID *string
ActorTokenName *string
ActorTokenPrefix *string
```

Write migration for these columns in SQLite/PostgreSQL.

- [ ] **Step 4: 扩展 app AuditEntry/AuditLogView**

Add fields:

```go
ActorType string
ActorTokenID *string
ActorTokenName *string
ActorTokenPrefix *string
ActorToken *TokenActorInfo
```

During `withAudit`, derive:

- user runtime: `actor_type=user`, `actor_user_id=&ActorUserID`
- tenant runtime: `actor_type=tenant_access_token`, `actor_user_id=nil`, token fields set

- [ ] **Step 5: 扩展 HTTP/MCP audit output**

In `internal/httpapi/import_audit.go` and MCP audit view, output:

```json
"actor_type": "tenant_access_token",
"actor_user": null,
"actor_token": {"id":"...", "name":"...", "prefix":"..."}
```

Keep existing `actor` / `delegator_user` behavior for user actor.

- [ ] **Step 6: 跑 audit tests**

```bash
go test ./internal/storage ./internal/app ./internal/httpapi ./internal/mcpserver -run 'Audit|TenantTokenAudit' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/storage/models.go internal/storage/audit_repo.go internal/storage/db.go internal/storage/db_test.go internal/app/audit.go internal/app/audit_test.go internal/httpapi/import_audit.go internal/mcpserver/tools_misc.go
git commit -m "feat: 记录租户 token 审计 actor"
```

---

## Chunk 3: HTTP API

### Task 6: Bearer 认证支持 tenant token

**Files:**
- Modify: `internal/app/token.go`
- Modify: `internal/httpapi/middleware.go`
- Modify: `internal/httpapi/auth_test.go`

- [ ] **Step 1: 写失败测试**

In `internal/httpapi/auth_test.go`, add:

```go
func TestHTTPAuthenticateTenantAccessToken(t *testing.T) {
    fixture := newHTTPServerWithTokenFixture(t, "task:read")
    created := createTenantTokenForHTTP(t, fixture.svc, []string{"task:read"})
    rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks?no_context=true", map[string]string{
        "Authorization": "Bearer " + created.RawToken,
    })
    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
}
```

Before adding the test, extend the local `httpTokenFixture` in `internal/httpapi/auth_test.go` with the existing service so helpers can create tenant tokens against the same store:

```go
type httpTokenFixture struct {
    server *Server
    token  string
    id     string
    svc    *app.Service
}
```

and return `svc: svc` from `newHTTPServerWithTokenFixture`.

Add local helper:

```go
func createTenantTokenForHTTP(t *testing.T, svc *app.Service, scopes []string) app.CreatedTenantAccessToken {
    t.Helper()
    created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
        Name: "http-tenant",
        Scopes: scopes,
        WorkspaceRef: svc.Runtime().WorkspaceSlug,
    })
    if err != nil {
        t.Fatal(err)
    }
    return created
}
```

Call it as `createTenantTokenForHTTP(t, fixture.svc, []string{"task:read"})`.

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/httpapi -run 'TenantAccessToken|Authenticate' -count=1
```

Expected: FAIL，auth middleware 不识别 tenant prefix。

- [ ] **Step 3: 修改 `AuthenticateBearerToken`**

In `internal/app/token.go`:

- detect `auth.TenantAccessTokenPrefix`
- query `api_tokens`
- require `Type == tenant_access_token`
- verify hash
- reject revoked/expired
- touch last used
- build the embedded `TokenView` directly from `TenantAccessTokenView`; do not add a fake `User`.

- [ ] **Step 4: 修改 middleware request auth**

In `internal/httpapi/middleware.go`, remove assumptions that authenticated token always has `User`. Store `requestAuth` with `Authn` only; handlers must rely on scoped service.

- [ ] **Step 5: 跑认证测试**

```bash
go test ./internal/httpapi -run 'Auth|TenantAccessToken' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/token.go internal/httpapi/middleware.go internal/httpapi/auth_test.go
git commit -m "feat: HTTP 认证支持租户 token"
```

### Task 7: Workspace tenant token 管理 API

**Files:**
- Create: `internal/httpapi/tenant_tokens.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/error_status.go`
- Test: `internal/httpapi/tokens_test.go`

- [ ] **Step 1: 写 HTTP API 失败测试**

In `internal/httpapi/tokens_test.go`:

```go
func TestTenantAccessTokenManagementAPI(t *testing.T) {
    fixture := newHTTPServerWithTokenFixture(t, "token:read", "token:write")
    body := `{"name":"runtime","scopes":["task:read"],"expires_in_seconds":3600}`
    authHeader := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}
    rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tenant-access-tokens", body, authHeader)
    if rr.Code != http.StatusCreated { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
    if !strings.Contains(rr.Body.String(), `"token":"xuanchu_tenant_`) { t.Fatalf("body=%s", rr.Body.String()) }

    rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tenant-access-tokens", authHeader)
    if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), `"token":"xuanchu_tenant_`) {
        t.Fatalf("list leaked raw token or failed: %d %s", rr.Code, rr.Body.String())
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/httpapi -run 'TenantAccessTokenManagementAPI' -count=1
```

Expected: FAIL，routes missing。

- [ ] **Step 3: 实现 response/request structs**

In `tenant_tokens.go`:

```go
type tenantTokenResponse struct {
    ID string `json:"id"`
    Prefix string `json:"prefix"`
    Name string `json:"name"`
    Type string `json:"type"`
    WorkspaceID string `json:"workspace_id"`
    ProjectIDs []string `json:"project_ids"`
    Scopes []string `json:"scopes"`
    CreatedAt int64 `json:"created_at"`
    ExpiresAt *int64 `json:"expires_at,omitempty"`
    RevokedAt *int64 `json:"revoked_at,omitempty"`
    LastUsedAt *int64 `json:"last_used_at,omitempty"`
}
```

Create response embeds `Token string`.

- [ ] **Step 4: 实现 handlers**

Handlers:

- `handleTenantTokenList`
- `handleTenantTokenCreate`
- `handleTenantTokenModify`
- `handleTenantTokenRevoke`

Use `s.scopedService(r, auth.ScopeTokenRead/Write, app.PermissionTokenRead/Write, "")`, then reject `authn.Authn.Token.Type == auth.TokenTypeAdminActing` with `admin_acting_not_allowed`.

- [ ] **Step 5: 注册 routes**

In `internal/httpapi/huma_routes.go`:

```go
GET    /api/v1/tenant-access-tokens
POST   /api/v1/tenant-access-tokens
PATCH  /api/v1/tenant-access-tokens/{tokenRef}
DELETE /api/v1/tenant-access-tokens/{tokenRef}
```

- [ ] **Step 6: 错误码映射**

In `internal/httpapi/error_status.go`:

```go
case "tenant_token_not_found":
    return http.StatusNotFound
case "tenant_token_name_required", "tenant_token_scope_invalid", "tenant_token_project_scope_invalid", "tenant_actor_not_user":
    return http.StatusBadRequest
case "tenant_token_revoked", "tenant_token_expired":
    return http.StatusConflict
case "admin_acting_not_allowed":
    return http.StatusUnauthorized
```

- [ ] **Step 7: 跑 HTTP token tests**

```bash
go test ./internal/httpapi -run 'TenantAccessToken|Token' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/httpapi/tenant_tokens.go internal/httpapi/huma_routes.go internal/httpapi/error_status.go internal/httpapi/tokens_test.go
git commit -m "feat: 增加租户 token HTTP 管理接口"
```

### Task 8: Admin tenant token API

**Files:**
- Create: `internal/httpapi/admin_tenant_tokens.go`
- Modify: `internal/httpapi/huma_routes.go`
- Test: `internal/httpapi/admin_test.go`

- [ ] **Step 1: 写失败测试**

In `internal/httpapi/admin_test.go`:

```go
func TestAdminCanListAndRevokeTenantAccessTokens(t *testing.T) {
    fixture := newAdminHTTPFixture(t)
    tenant := createTenantTokenInWorkspace(t, fixture)
    rr := adminRequest(t, fixture.server, http.MethodGet, "/api/v1/admin/tenant-access-tokens?all=true", fixture.adminToken, nil)
    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
    if !strings.Contains(rr.Body.String(), tenant.View.ID) { t.Fatalf("body=%s", rr.Body.String()) }

    rr = adminRequest(t, fixture.server, http.MethodDelete, "/api/v1/admin/tenant-access-tokens/"+tenant.View.ID, fixture.adminToken, nil)
    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/httpapi -run 'Admin.*TenantAccessToken' -count=1
```

Expected: FAIL, route missing.

- [ ] **Step 3: 实现 admin handlers**

Handlers:

- `handleAdminTenantTokenList`
- `handleAdminTenantTokenModify`
- `handleAdminTenantTokenRevoke`

Use server admin auth only. P1 no create. Modification must write audit payload with admin token id/name.

- [ ] **Step 4: 注册 admin routes**

```go
GET    /api/v1/admin/tenant-access-tokens
PATCH  /api/v1/admin/tenant-access-tokens/{tokenRef}
DELETE /api/v1/admin/tenant-access-tokens/{tokenRef}
```

- [ ] **Step 5: 跑 admin tests**

```bash
go test ./internal/httpapi -run 'Admin.*Tenant|TenantAccessToken' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/admin_tenant_tokens.go internal/httpapi/huma_routes.go internal/httpapi/admin_test.go
git commit -m "feat: 增加超管租户 token 接口"
```

### Task 9: HTTP forbidden paths and context semantics

**Files:**
- Modify: `internal/httpapi/context_config.go`
- Modify: `internal/httpapi/tasks.go`
- Test: `internal/httpapi/context_config_test.go`
- Test: `internal/httpapi/tasks_test.go`

- [ ] **Step 1: 写失败测试**

Cover:

- tenant token `/api/v1/me` => `tenant_actor_not_user`
- tenant token `X-Xuanchu-As` => `token_scope_denied`
- tenant token `/api/v1/contexts/{name}/use` => `tenant_actor_not_user`
- tenant token `/api/v1/contexts/none` => `tenant_actor_not_user`
- tenant token `assignee=me` => `tenant_actor_not_user`
- tenant token can `POST /api/v1/contexts` and `GET /api/v1/contexts/{name}`

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/httpapi -run 'Tenant.*Context|Tenant.*Me|Tenant.*Assignee|Impersonation' -count=1
```

- [ ] **Step 3: 实现 endpoint checks**

Use helper:

```go
func isTenantActor(svc *app.Service) bool {
    return svc.Runtime().ActorType == string(authz.ActorTenantAccessToken)
}
```

Return `tenant_actor_not_user` before calling app methods that read/write active context or expand `assignee:me`.

- [ ] **Step 4: 跑 HTTP forbidden tests**

```bash
go test ./internal/httpapi -run 'Tenant.*Context|Tenant.*Me|Tenant.*Assignee|Impersonation' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/context_config.go internal/httpapi/tasks.go internal/httpapi/context_config_test.go internal/httpapi/tasks_test.go
git commit -m "fix: 收紧租户 token 的个人语义接口"
```

---

## Chunk 4: MCP

### Task 10: MCP tenant auth and tool denylist

**Files:**
- Modify: `internal/mcpserver/auth.go`
- Modify: `internal/mcpserver/tools_misc.go`
- Modify: `internal/mcpserver/tools_context.go`
- Modify: `internal/mcpserver/tools_token.go`
- Modify: `internal/mcpserver/tools_user.go`
- Modify: `internal/mcpserver/tools_member.go`
- Modify: `internal/mcpserver/tools_workspace.go`
- Modify: `internal/mcpserver/resources.go`
- Test: `internal/mcpserver/auth_test.go`
- Test: `internal/mcpserver/integration_test.go`

- [ ] **Step 1: 写 MCP 失败测试**

In `internal/mcpserver/integration_test.go`, add table tests:

```go
func TestHTTPMCPWithTenantTokenAllowedAndDeniedTools(t *testing.T) {
    fixture := newHTTPMCPFixture(t)
    tenant := fixture.CreateTenantToken([]string{"*", "context:write"})
    assertMCPToolOK(t, fixture, tenant.RawToken, "task_query", map[string]any{"no_context": true})
    assertMCPToolErrorCode(t, fixture, tenant.RawToken, "me_get", nil, "tenant_actor_not_user")
    assertMCPToolErrorCode(t, fixture, tenant.RawToken, "token_list", nil, "token_scope_denied")
    assertMCPToolErrorCode(t, fixture, tenant.RawToken, "context_set", map[string]any{"name":"sprint"}, "tenant_actor_not_user")
}
```

If the named helpers do not exist, add them in the same test file:

- `newHTTPMCPFixture(t)` should create a store, app service, HTTP server, and MCP client request helper.
- `CreateTenantToken(scopes []string)` should call `app.CreateTenantAccessToken` on that service and return the raw token plus view.
- `assertMCPToolOK` should call the tool and fail on any business error.
- `assertMCPToolErrorCode` should call the tool and assert the returned tool envelope/app error code.

Keep these helpers test-local. Do not add production abstraction just for tests.

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/mcpserver ./internal/httpapi -run 'Tenant|HTTPMCP|ServiceForHTTP' -count=1
```

Expected: FAIL.

- [ ] **Step 3: tenant service construction**

In `internal/mcpserver/auth.go`, ensure `serviceForTool` / `serviceForHTTP` pass tenant `AuthenticatedToken` to `AuthorizeTokenRequest` without requiring user.

- [ ] **Step 4: central deny helper**

Add helper:

```go
func rejectTenantTool(svc *app.Service, toolName string) error {
    if svc.Runtime().ActorType != "tenant_access_token" {
        return nil
    }
    switch toolName {
    case "me_get", "context_get", "context_set", "context_none":
        return app.RuntimeError{Code: "tenant_actor_not_user", Message: "tenant token has no user actor"}
    case "user_list", "user_get", "user_bind", "user_unbind", "user_add", "user_use", "user_list_external_ids",
        "member_list", "member_add", "member_role",
        "token_list", "token_create", "token_modify", "token_revoke",
        "workspace_list", "workspace_add", "workspace_modify", "workspace_archive", "workspace_use":
        return app.RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "tenant token cannot call this tool"}
    }
    return nil
}
```

Call after service construction and before business logic. Keep `workspace_get_current` and `workspace_info` allowed.

`context_get` 只有在未指定 `name`、需要读取当前 active context 时返回 `tenant_actor_not_user`；指定 `name` 的显式 context 读取继续按 `context:read` 和 workspace/project scope 授权。

- [ ] **Step 5: context resource behavior**

In `resources.go`, current context resource should return `tenant_actor_not_user` when actor type is tenant. Named context reads remain through tools/API, not current context resource.

- [ ] **Step 6: 跑 MCP tests**

```bash
go test ./internal/mcpserver ./internal/httpapi -run 'Tenant|HTTPMCP|Context|Tool' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/mcpserver/auth.go internal/mcpserver/tools_misc.go internal/mcpserver/tools_context.go internal/mcpserver/tools_token.go internal/mcpserver/tools_user.go internal/mcpserver/tools_member.go internal/mcpserver/tools_workspace.go internal/mcpserver/resources.go internal/mcpserver/auth_test.go internal/mcpserver/integration_test.go
git commit -m "feat: MCP 支持租户 token"
```

---

## Chunk 5: Web Console

### Task 11: Workspace Console tenant token tab

**Files:**
- Modify: `web/src/features/workspace/tokens/token-api.ts`
- Modify: `web/src/features/workspace/tokens/scopes.ts`
- Modify: `web/src/features/workspace/tokens/scope-editor.tsx`
- Modify: `web/src/features/workspace/tokens/tokens-page.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-api.ts`
- Create: `web/src/features/workspace/tokens/tenant-token-form.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-create-dialog.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-edit-dialog.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-revoke-dialog.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-created-result.tsx`
- Modify: `web/src/features/workspace/tokens/tokens-page.test.tsx`
- Create: `web/src/features/workspace/tokens/tenant-token-form.test.tsx`

- [ ] **Step 1: 写失败测试**

In `tokens-page.test.tsx`:

```tsx
it("renders tenant access token tab and does not mix user tokens", async () => {
  setWorkspaceToken("xuanchu_pat_test")
  mockFetch([
    ["/api/v1/tokens", okResponse([makeToken({ name: "cli" })])],
    ["/api/v1/tenant-access-tokens", okResponse([makeTenantToken({ name: "runtime-prod" })])],
  ])
  renderWithRouter(<TokensPage />, { route: "/tokens?tab=tenant" })
  expect(await screen.findByText("Tenant Access Tokens")).toBeTruthy()
  expect(await screen.findByText("runtime-prod")).toBeTruthy()
  expect(screen.queryByText("cli")).toBeNull()
})
```

Add `mockFetch` and `makeTenantToken` as local helpers in `tokens-page.test.tsx`, following the file's existing `okResponse` and `makeToken` style. `mockFetch` should route by URL substring and return `okResponse([])` for unrelated known endpoints such as `/api/v1/me`.

In `tenant-token-form.test.tsx`, assert disabled/hidden forbidden scopes.

- [ ] **Step 2: 跑 web tests 确认失败**

```bash
pnpm --dir web test -- --run tokens-page tenant-token-form
```

Expected: FAIL.

- [ ] **Step 3: API types**

In `tenant-token-api.ts`:

```ts
export type TenantTokenRow = {
  id: string
  prefix: string
  name: string
  type: "tenant_access_token"
  workspace_id: string
  project_ids: string[]
  scopes: string[]
  created_at: number
  expires_at?: number
  revoked_at?: number
  last_used_at?: number
}

export type CreatedTenantTokenRow = TenantTokenRow & { token: string }
```

Add create/modify input types.

- [ ] **Step 4: Form and dialogs**

Implement form with:

- name
- project allowlist
- tenant scope whitelist
- expires preset

Reuse `ScopeEditor`, but pass disabled scopes or only tenant groups. Do not show token/user/member/impersonate/workspace write.

- [ ] **Step 5: Page tab**

In `tokens-page.tsx`:

- Parse `tab` from query string.
- `tab=user` keeps current behavior.
- `tab=tenant` calls `/api/v1/tenant-access-tokens?all=...`.
- `Show revoked` default false.
- Hide tenant tab in admin acting mode by checking workspace session acting context.
- Render empty/error/retry states.

- [ ] **Step 6: 跑 workspace web tests**

```bash
pnpm --dir web test -- --run tokens-page token-form scope-editor tenant-token-form
pnpm --dir web typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/workspace/tokens
git commit -m "feat: 工作台增加租户 token 页面"
```

### Task 12: Admin Console tenant token tab

**Files:**
- Modify: `web/src/features/admin/tokens/admin-token-api.ts`
- Modify: `web/src/features/admin/tokens/admin-tokens-page.tsx`
- Create: `web/src/features/admin/tokens/admin-tenant-token-api.ts`
- Create: `web/src/features/admin/tokens/admin-tenant-token-edit-dialog.tsx`
- Create: `web/src/features/admin/tokens/admin-tenant-token-revoke-dialog.tsx`
- Modify: `web/src/features/admin/tokens/admin-tokens-page.test.tsx`

- [ ] **Step 1: 写失败测试**

In `admin-tokens-page.test.tsx`:

```tsx
it("renders admin tenant token tab without create action", async () => {
  setAdminToken("xuanchu_admin_test")
  mockFetch([
    ["/api/v1/admin/tenant-access-tokens?all=false", okResponse([makeAdminTenantToken({ name: "runtime-prod" })])],
  ])
  renderWithRouter(<AdminTokensPage />, { route: "/admin/tokens?tab=tenant" })
  expect(await screen.findByText("runtime-prod")).toBeTruthy()
  expect(screen.queryByText("创建 Token")).toBeNull()
})
```

Add `mockFetch` and `makeAdminTenantToken` as local helpers in `admin-tokens-page.test.tsx`, following the file's existing `okResponse` and `makeRow` style.

- [ ] **Step 2: 跑测试确认失败**

```bash
pnpm --dir web test -- --run admin-tokens-page
```

Expected: FAIL.

- [ ] **Step 3: API and dialogs**

Implement:

- `GET /api/v1/admin/tenant-access-tokens?all=...`
- `PATCH /api/v1/admin/tenant-access-tokens/{ref}`
- `DELETE /api/v1/admin/tenant-access-tokens/{ref}`

Admin edit supports name/scopes/projects/expires. No create button.

- [ ] **Step 4: Admin page tab**

In `admin-tokens-page.tsx`:

- `tab=user` existing table.
- `tab=tenant` tenant table with workspace/name/prefix/projects/scope/status/actions.
- `All revoked` default false.

- [ ] **Step 5: 跑 admin web tests**

```bash
pnpm --dir web test -- --run admin-tokens-page
pnpm --dir web typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/admin/tokens
git commit -m "feat: 超管后台增加租户 token 页面"
```

---

## Chunk 6: Docs, Full Verification, Release Readiness

### Task 13: 文档同步

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/manual/web-console.md`
- Modify: `docs/manual/team-workspaces-projects.md`
- Modify: `docs/superpowers/specs/2026-06-29-xuanchu-tenant-access-token-design.md` if implementation decisions drift.

- [ ] **Step 1: 更新 README**

Add:

- `tenant_access_token` 是 workspace API key。
- raw prefix `xuanchu_tenant_`。
- HTTP API / MCP usage:

```bash
curl -H "Authorization: Bearer xuanchu_tenant_xxx" http://127.0.0.1:8080/api/v1/tasks
```

- It cannot call `/me`, user/member/token management, impersonation, active context use/none.

- [ ] **Step 2: 更新 MCP manual**

Document:

- HTTP MCP accepts tenant token.
- allowed/forbidden tool categories.
- `context_get` with name allowed; current context resource / `context_set` / `context_none` forbidden.

- [ ] **Step 3: 更新 Web Console manual**

Document:

- Workspace `/tokens?tab=tenant`
- Admin `/admin/tokens?tab=tenant`
- raw token only shown once
- acting mode hides tenant tab

- [ ] **Step 4: 更新 ROADMAP**

Mark P1 tenant access token as implemented when code is complete. Keep OIDC/user_access_token/client_credentials in P2/future.

- [ ] **Step 5: 文档 grep 校验**

```bash
rg -n "tenant_access_token|xuanchu_tenant_|Tenant Access Tokens|user_access_token|OIDC" README.md ROADMAP.md docs/manual docs/superpowers/specs/2026-06-29-xuanchu-tenant-access-token-design.md
```

Expected: docs consistently describe tenant token as API key, not OAuth/OIDC token.

- [ ] **Step 6: Commit**

```bash
git add README.md ROADMAP.md docs/manual/mcp.md docs/manual/web-console.md docs/manual/team-workspaces-projects.md docs/superpowers/specs/2026-06-29-xuanchu-tenant-access-token-design.md
git commit -m "docs: 补充租户 token 使用说明"
```

### Task 14: 全量验证

**Files:**
- No source changes unless failures reveal bugs.

- [ ] **Step 1: 后端聚焦验证**

```bash
go test ./internal/auth ./internal/authz ./internal/storage ./internal/app ./internal/httpapi ./internal/mcpserver -run 'Tenant|Token|Auth|MCP|Audit|Context' -count=1
```

Expected: PASS.

- [ ] **Step 2: 后端全量验证**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
```

Expected: PASS.

- [ ] **Step 3: 前端验证**

```bash
pnpm --dir web test -- --run
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
```

Expected: PASS.

- [ ] **Step 4: 格式和状态检查**

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only intentional changes.

- [ ] **Step 5: 修复发现的问题**

If any command fails:

1. Read the first failure.
2. Fix root cause, not just assertion text.
3. Re-run the smallest failing command.
4. Re-run the full command group for the affected layer.

- [ ] **Step 6: Final commit if fixes were needed**

```bash
git add <fixed files>
git commit -m "fix: 完成租户 token 验证修正"
```

## Final Acceptance Checklist

- [ ] `api_tokens.user_id` nullable migration works on SQLite and PostgreSQL.
- [ ] PAT / Agent token still require user and keep current behavior.
- [ ] tenant token stored in `api_tokens` with `type=tenant_access_token` and `user_id=NULL`.
- [ ] tenant token creation returns raw token once.
- [ ] list/modify/revoke never returns raw token.
- [ ] tenant token cannot manage PAT / Agent / tenant token APIs.
- [ ] tenant token cannot use `X-Xuanchu-As`.
- [ ] tenant token cannot use `assignee:me`.
- [ ] tenant token cannot call `/api/v1/me` or MCP `me_get`.
- [ ] tenant token cannot set/clear active context.
- [ ] tenant token can manage workspace context definitions.
- [ ] HTTP API accepts tenant token for allowed task/project/config/audit operations and hook/notification/reminder operations that do not require a user-shaped `created_by` / `actor`; creation paths that still require user actor return `tenant_actor_not_user`.
- [ ] HTTP MCP accepts tenant token for allowed tools.
- [ ] Audit and access log show `actor_type=tenant_access_token` with token id/name/prefix and no raw secret.
- [ ] Web Console workspace tab works and hides in acting mode.
- [ ] Admin Console tenant token tab works without create action.
- [ ] README/ROADMAP/manual docs match implementation.
- [ ] `go test ./...`, `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go build ./cmd/xuanchu`, `go vet ./...`, `pnpm --dir web test -- --run`, `pnpm --dir web typecheck`, `pnpm --dir web lint`, and `pnpm --dir web build` pass.
