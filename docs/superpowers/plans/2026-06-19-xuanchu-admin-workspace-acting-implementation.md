# Xuanchu 超管 Workspace Acting 实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 server admin 在 `/admin/*` 管理 workspace，并能通过短期 acting session 切换到任意未归档 workspace 的管理员视角，再安全退回超管界面。

**Architecture:** 保持 server admin token 与普通业务 token 隔离：`xuanchu_admin_...` 只访问 `/api/v1/admin/*`，普通 `/api/v1/*` 只接受 PAT、Agent token 或服务端签发的短期 `xuanchu_act_...` acting token。后端新增 admin workspace 查询、acting session 存储和鉴权分支；前端新增 `/admin/workspaces` 控制面，并让 Workspace Console 在 acting mode 下优先使用 acting token 和显示返回超管入口。

**Tech Stack:** Go 1.25, GORM, `github.com/glebarez/sqlite`, `gorm.io/driver/postgres`, chi HTTP API, React 19, TanStack Router/Query, shadcn/ui, i18next, Vitest.

---

## 范围说明

- 设计依据：[docs/superpowers/specs/2026-06-12-xuanchu-v0.4.1-server-admin-console-design.md](../specs/2026-06-12-xuanchu-v0.4.1-server-admin-console-design.md)
- 不实现任务/项目/通知/hook 的 server-admin 聚合 dashboard。
- 不让 admin token 直接调用普通 `/api/v1/*`。
- acting token 只用于浏览器 Workspace Console 的普通 HTTP API，不开放给 MCP 或长期自动化；remote CLI client 必须拒绝 `xuanchu_act_` 前缀。
- acting token 的 scope 是能力上限，实际访问仍由绑定 workspace、绑定 actor 的当前 workspace role 和 project allowlist 共同收窄。
- 本计划只描述实现，不要求一次性提交；每个 chunk 应独立测试，并在人工确认后用中文提交信息提交。

## 文件地图

后端基础：

- Modify: `internal/auth/token.go` - 增加 acting token 前缀、生成和校验 helper。
- Modify: `internal/storage/models.go` - 增加 `AdminActingSession` 和 audit 追踪列。
- Modify: `internal/storage/migrate_sqlite.go` - SQLite AutoMigrate 纳入 acting session 和新增 audit 列。
- Modify: `internal/storage/migrate_postgres.go` - PostgreSQL AutoMigrate 纳入 acting session 和新增 audit 列。
- Create: `internal/storage/admin_acting_session_repo.go` - acting session CRUD、prefix 查询、last_used/revoke。
- Test: `internal/storage/admin_acting_session_repo_test.go`

后端 app / HTTP：

- Modify: `internal/app/runtime.go` - runtime 记录 admin acting session 和 delegator admin token 来源。
- Modify: `internal/app/audit.go` - audit 写入和读取 admin acting 追踪字段。
- Modify: `internal/app/token.go` - bearer token 鉴权识别 `xuanchu_act_`，返回 acting authn。
- Create: `internal/app/admin_workspace.go` - admin workspace list/detail 和 acting session 创建。
- Modify: `internal/httpapi/admin_auth.go` - admin auth context 增加 token id/name。
- Modify: `internal/httpapi/admin.go` - 新增 admin workspace 和 acting session handlers。
- Modify: `internal/httpapi/router.go` - 注册 `/api/v1/admin/workspaces` GET、detail GET、acting session POST/DELETE。
- Modify: `internal/httpapi/middleware.go` / `internal/httpapi/app_service.go` - 禁止 acting token 访问 `/mcp`，日志记录 acting 来源。
- Modify: `internal/remote/client.go` - remote CLI client 拒绝 acting token 前缀。
- Test: `internal/app/admin_workspace_test.go`
- Test: `internal/httpapi/admin_workspace_test.go`
- Test: `internal/httpapi/impersonation_test.go`
- Test: `internal/remote/client_test.go`

前端：

- Modify: `web/src/features/workspace/session/workspace-token.ts` - 增加 acting token/context helpers。
- Modify: `web/src/features/workspace/session/workspace-api.ts` - acting mode 优先使用 acting token，401 时清 acting token。
- Modify: `web/src/features/workspace/session/useMe.ts` - 暴露 acting context 或保持 me query key 隔离。
- Modify: `web/src/components/AppShell.tsx` - acting banner 和返回超管操作。
- Modify: `web/src/routes/router.tsx` - 增加 `/admin/workspaces` 和 `/admin/workspaces/$workspaceSlug`。
- Modify: `web/src/features/admin/components/AdminShell.tsx` - 增加 Workspace 管理导航。
- Create: `web/src/features/admin/workspaces/admin-workspace-api.ts`
- Create: `web/src/features/admin/workspaces/admin-workspaces-page.tsx`
- Create: `web/src/features/admin/workspaces/admin-workspace-detail-page.tsx`
- Create: `web/src/features/admin/workspaces/admin-acting-session-dialog.tsx`
- Create: `web/src/routes/admin/AdminWorkspacesRoute.tsx`
- Create: `web/src/routes/admin/AdminWorkspaceDetailRoute.tsx`
- Modify: `web/src/locales/zh-CN.ts`, `web/src/locales/en-US.ts`
- Test: `web/src/features/admin/workspaces/*.test.tsx`
- Test: `web/src/features/workspace/session/workspace-api.test.ts`
- Test: `web/src/components/AppShell.test.tsx`

文档 / OpenAPI：

- Modify: `docs/openapi/xuanchu-v1.yaml`
- Modify: `docs/manual/web-console.md`
- Modify: `docs/manual/deployment.md` if admin setup docs mention bootstrap-only behavior.

---

## Chunk 1: 存储、Token 与审计基础

### Task 1: 增加 acting token helper

**Files:**
- Modify: `internal/auth/token.go`
- Test: `internal/auth/token_test.go`

- [ ] **Step 1: Write failing auth tests**

Add tests:

```go
func TestGenerateActingToken(t *testing.T) {
	raw, prefix, hash, err := GenerateActingToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, ActingTokenPrefix) {
		t.Fatalf("raw prefix = %q", raw)
	}
	if !strings.HasPrefix(prefix, ActingTokenPrefix) || len(prefix) > 16 {
		t.Fatalf("display prefix = %q", prefix)
	}
	if !VerifyActingToken(raw, hash) {
		t.Fatal("VerifyActingToken() = false")
	}
	if VerifyActingToken(raw+"x", hash) {
		t.Fatal("VerifyActingToken(invalid) = true")
	}
}
```

- [ ] **Step 2: Run failing test**

Run: `go test ./internal/auth -run TestGenerateActingToken -count=1`

Expected: FAIL because helpers do not exist.

- [ ] **Step 3: Implement helpers**

In `internal/auth/token.go`, add:

```go
const ActingTokenPrefix = "xuanchu_act_"

func GenerateActingToken() (raw string, prefix string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate acting token entropy: %w", err)
	}
	raw = ActingTokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	prefix = raw
	if len(prefix) > 16 {
		prefix = raw[:16]
	}
	sum := sha256.Sum256([]byte(raw))
	return raw, prefix, "sha256:" + hex.EncodeToString(sum[:]), nil
}

func VerifyActingToken(raw, verifier string) bool {
	return VerifyAdminToken(raw, verifier)
}
```

Keep `GenerateToken` unchanged for PAT/Agent tokens.

- [ ] **Step 4: Run test**

Run: `go test ./internal/auth -run TestGenerateActingToken -count=1`

Expected: PASS.

### Task 2: 增加 acting session 模型、仓储和迁移

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`
- Create: `internal/storage/admin_acting_session_repo.go`
- Test: `internal/storage/admin_acting_session_repo_test.go`

- [ ] **Step 1: Write failing storage tests**

Cover:

- create + get by prefix
- valid list excludes revoked/expired
- touch last used
- revoke
- prefix not found returns `storage.ErrNotFound`

Use `newTestStore(t)` pattern from existing storage tests.

- [ ] **Step 2: Run failing test**

Run: `go test ./internal/storage -run TestAdminActingSession -count=1`

Expected: FAIL because model/repo do not exist.

- [ ] **Step 3: Add model**

Add to `internal/storage/models.go`:

```go
type AdminActingSession struct {
	ID             string `gorm:"primaryKey"`
	TokenPrefix    string `gorm:"not null;uniqueIndex:idx_admin_acting_sessions_prefix"`
	TokenHash      string `gorm:"not null"`
	AdminTokenID   *string `gorm:"index"`
	AdminTokenName string `gorm:"not null"`
	WorkspaceID    string `gorm:"not null;index"`
	ActorUserID    string `gorm:"not null;index"`
	Role           string `gorm:"not null"`
	CreatedAt      int64  `gorm:"not null"`
	ExpiresAt      int64  `gorm:"not null;index"`
	RevokedAt      *int64
	LastUsedAt     *int64
}
```

- [ ] **Step 4: Add repository**

Create `internal/storage/admin_acting_session_repo.go` with:

- `AdminActingSessionEntry`
- `AdminActingSessionRepository`
- `Create(entry AdminActingSessionEntry) error`
- `GetByPrefix(prefix string) (AdminActingSessionEntry, error)`
- `GetByID(id string) (AdminActingSessionEntry, error)`
- `TouchLastUsed(id string, ts int64) error`
- `Revoke(id string, ts int64) error`

Follow `server_admin_token_repo.go` style.

- [ ] **Step 5: Wire migrations**

Add `&AdminActingSession{}` to both SQLite and PostgreSQL AutoMigrate lists.

- [ ] **Step 6: Run storage tests**

Run: `go test ./internal/storage -run 'AdminActingSession|DB|Postgres' -count=1`

Expected: PASS for normal tests; PostgreSQL opt-in tests should skip unless configured.

### Task 3: 增加 admin acting 可追溯审计字段

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/audit.go`
- Modify: `internal/httpapi/import_audit.go`
- Test: `internal/app/request_scope_test.go`
- Test: `internal/httpapi/impersonation_test.go`

- [ ] **Step 1: Write failing audit test**

Add an app-level test that constructs a `RuntimeContext` with:

```go
AdminActingSessionID: "act-1",
DelegatorAdminTokenID: "admin-token-1",
DelegatorAdminTokenName: "ops-primary",
```

Perform a simple audited operation such as `Add(AddInput{Description: "via acting"})`, then assert the audit row contains all three fields.

- [ ] **Step 2: Run failing test**

Run: `go test ./internal/app -run TestAdminActingAuditTrace -count=1`

Expected: FAIL because fields do not exist.

- [ ] **Step 3: Extend audit storage and runtime**

Add to `storage.AuditLog` and `storage.AuditLogEntry`:

- `AdminActingSessionID *string`
- `DelegatorAdminTokenID *string`
- `DelegatorAdminTokenName *string`

Add matching fields to `app.RuntimeContext` and `app.AuditLogView`.

- [ ] **Step 4: Write fields in audit append**

In `app.withAuditEntriesAndEvents`, populate the new fields from runtime context.

Keep existing user-token `DelegatorTokenID` / `DelegatorUserID` unchanged for normal `impersonate` behavior.

- [ ] **Step 5: Expose fields in HTTP audit output**

Update `internal/httpapi/import_audit.go` response structs so `/api/v1/audit` can show admin acting trace fields.

- [ ] **Step 6: Run audit tests**

Run:

```bash
go test ./internal/app -run 'AdminActingAuditTrace|ImpersonatedTaskActionRecordsDelegator' -count=1
go test ./internal/httpapi -run 'Impersonation|Audit' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit chunk 1**

```bash
git add internal/auth/token.go internal/auth/token_test.go \
  internal/storage/models.go internal/storage/migrate_sqlite.go internal/storage/migrate_postgres.go \
  internal/storage/admin_acting_session_repo.go internal/storage/admin_acting_session_repo_test.go \
  internal/app/runtime.go internal/app/audit.go internal/httpapi/import_audit.go \
  internal/app/request_scope_test.go internal/httpapi/impersonation_test.go
git commit -m "feat: 增加超管 acting session 存储基础"
```

---

## Chunk 2: App Service 与 HTTP Admin API

### Task 4: 实现 admin workspace 列表和详情 service

**Files:**
- Modify: `internal/storage/workspace_repo.go`
- Create: `internal/app/admin_workspace.go`
- Test: `internal/app/admin_workspace_test.go`

- [ ] **Step 1: Write failing app tests**

Test:

- `AdminListWorkspaces(false)` returns all active workspaces, not just actor membership.
- `AdminListWorkspaces(true)` includes archived workspaces.
- `AdminWorkspaceInfo("slug")` returns members and `acting_candidates`.
- returned users use full `task.UserInfo`, with fallback `{ID:id, Name:id}` if missing.
- token counts count active/revoked/expired tokens per workspace by parsing `WorkspaceIDsJSON`.

- [ ] **Step 2: Run failing test**

Run: `go test ./internal/app -run 'AdminListWorkspaces|AdminWorkspaceInfo' -count=1`

Expected: FAIL because services do not exist.

- [ ] **Step 3: Add repository helpers**

In `internal/storage/workspace_repo.go`, add:

- `ListAll(includeArchived bool) ([]Workspace, error)`

Keep existing `ListVisibleForUser` unchanged.

- [ ] **Step 4: Add app views and methods**

Create `internal/app/admin_workspace.go` with:

- `AdminWorkspaceSummaryView`
- `AdminWorkspaceDetailView`
- `AdminWorkspaceMemberView`
- `AdminWorkspaceTokenCounts`
- `AdminActingCandidateView`
- `AdminListWorkspaces(includeArchived bool) ([]AdminWorkspaceSummaryView, error)`
- `AdminWorkspaceInfo(workspaceRef string) (AdminWorkspaceDetailView, error)`

Reuse:

- `lookupWorkspace`
- `memberRepo.List`
- `tokenRepo.ListAll(true)`
- `resolveUserInfos`
- `workspaceViewFromRow`

- [ ] **Step 5: Run app tests**

Run: `go test ./internal/app -run 'AdminListWorkspaces|AdminWorkspaceInfo' -count=1`

Expected: PASS.

### Task 5: 实现 acting session service 和鉴权

**Files:**
- Modify: `internal/app/token.go`
- Modify: `internal/app/service.go` or `internal/app/service` initialization as needed
- Create/Modify: `internal/app/admin_workspace.go`
- Test: `internal/app/admin_workspace_test.go`
- Test: `internal/app/token_test.go`

- [ ] **Step 1: Write failing acting app tests**

Cover:

- `AdminCreateActingSession` defaults to first owner, then first admin.
- rejects archived workspace.
- rejects target that is not owner/admin.
- raw acting token is returned once and stored only as hash.
- `AuthenticateBearerToken` accepts acting token and returns actor user, workspace IDs limited to one workspace, and HTTP console scope as an upper bound.
- expired/revoked acting token is rejected.

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/app -run 'AdminCreateActingSession|AuthenticateBearerTokenActing' -count=1`

Expected: FAIL.

- [ ] **Step 3: Add service method**

In `internal/app/admin_workspace.go`, add:

```go
type AdminCreateActingSessionInput struct {
	AdminTokenID   *string
	AdminTokenName string
	WorkspaceRef   string
	UserRef        string
	ExpiresIn      *time.Duration
}
```

Return view containing raw token, expiry, workspace, actor `task.UserInfo`, role, admin token name.

Default TTL: 2 hours. Reject `ExpiresIn <= 0`.

- [ ] **Step 4: Authenticate acting tokens**

In `AuthenticateBearerToken`, before `tokenRepo.GetByPrefix`, detect `strings.HasPrefix(raw, auth.ActingTokenPrefix)` and resolve via `adminActingSessionRepo`.

For acting token:

- verify hash
- reject revoked/expired
- load actor user and workspace
- touch last used
- return `AuthenticatedToken` with `Token.Type = auth.TokenTypeAdminActing`, `WorkspaceIDs = []string{workspace.ID}`, `ProjectIDs = nil`, and `Scopes` equivalent to `auth.ParseScopes([]string{"*"})` after deleting `auth.ScopeImpersonate`.

Add `const TokenTypeAdminActing = "admin_acting"` in `internal/auth/token.go`, but do not make it valid for `GenerateToken` or `ValidateTokenCreate`; it is only an in-memory authenticated credential type derived from `admin_acting_sessions`.

Do not trust `AdminActingSession.Role` for ongoing authorization. Treat it as an audit/display snapshot from creation time; `AuthorizeTokenRequest` must continue to load the actor's current workspace membership and use that role for permission checks.

Do not write acting sessions to `api_tokens`.

- [ ] **Step 5: Carry admin trace through auth and authorization**

Extend `app.AuthenticatedToken` with an optional acting trace struct:

```go
type AdminActingTrace struct {
	SessionID               string
	DelegatorAdminTokenID   *string
	DelegatorAdminTokenName string
}
```

For acting tokens, populate this struct in `AuthenticateBearerToken`. In `AuthorizeTokenRequest` / `runtimeContextFromDecision`, copy it into runtime so ordinary audited operations carry:

- `AdminActingSessionID`
- `DelegatorAdminTokenID`
- `DelegatorAdminTokenName`

The normal user-agent impersonation fields must remain independent.

- [ ] **Step 6: Ensure acting authorization cannot widen scope**

Add assertions in tests:

- `AuthorizeTokenRequest` with acting token and a different workspace returns `workspace_scope_denied`.
- `AuthorizeTokenRequest` with acting token and an allowed workspace uses the bound actor's current workspace role, not server admin privileges or the stored role snapshot.
- `X-Xuanchu-As` does not combine with acting token to impersonate another user.
- If the actor is demoted to member/viewer after acting session creation, the token still authenticates but fails admin-only operations through the existing role permission check.

- [ ] **Step 7: Run app tests**

Run:

```bash
go test ./internal/app -run 'AdminCreateActingSession|AuthenticateBearerToken|AuthorizeTokenRequest|AdminActingAuditTrace' -count=1
```

Expected: PASS.

### Task 6: 增加 admin HTTP 路由和响应

**Files:**
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/admin_auth.go`
- Modify: `internal/httpapi/admin.go`
- Test: `internal/httpapi/admin_workspace_test.go`
- Test: `internal/httpapi/admin_test.go`

- [ ] **Step 1: Write failing HTTP tests**

Cover:

- `GET /api/v1/admin/workspaces`
- `GET /api/v1/admin/workspaces/{workspace}`
- `POST /api/v1/admin/workspaces/{workspace}/acting-sessions`
- acting session rejects archived workspace.
- normal PAT/Agent cannot access admin workspace endpoints.
- admin session capabilities include `workspace:list`, `workspace:read`, `acting_session:create`.
- raw acting token appears only in create response, not audit payload.

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/httpapi -run 'AdminWorkspace|AdminActing|AdminSession' -count=1`

Expected: FAIL.

- [ ] **Step 3: Extend admin auth context**

In `admin_auth.go`, add `TokenID *string` to `adminAuthInfo`. For DB-backed server admin tokens, populate ID; for configured hash tokens, leave nil and keep `TokenName`.

- [ ] **Step 4: Register routes**

In `router.go`:

```go
api.With(s.adminAuthMiddleware).Get("/api/v1/admin/workspaces", s.handleAdminWorkspaceList)
api.With(s.adminAuthMiddleware).Get("/api/v1/admin/workspaces/{workspace}", s.handleAdminWorkspaceInfo)
api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces/{workspace}/acting-sessions", s.handleAdminActingSessionCreate)
api.With(s.adminAuthMiddleware).Delete("/api/v1/admin/acting-sessions/{sessionID}", s.handleAdminActingSessionRevoke)
```

Place GET routes before conflicting POST routes where chi matching needs clarity.

- [ ] **Step 5: Add handlers and response mappers**

In `admin.go`, add request/response structs and handlers. Use existing `writeSuccess` / `writeAppError` patterns. Use `task.UserInfoToJSON` for all user references.

- [ ] **Step 6: Run HTTP tests**

Run: `go test ./internal/httpapi -run 'AdminWorkspace|AdminActing|AdminSession|AdminTokenCannotAccessMe|NormalTokenCannotAccessAdmin' -count=1`

Expected: PASS.

### Task 7: 确保 acting token 不能访问 MCP 或 remote CLI

**Files:**
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/middleware.go` or `internal/httpapi/app_service.go`
- Modify: `internal/remote/client.go`
- Test: `internal/httpapi/mcp_test.go`
- Test: `internal/remote/client_test.go`

- [ ] **Step 1: Write failing MCP and remote client tests**

Create an acting session, call `/mcp` with `Authorization: Bearer xuanchu_act_...`, assert unauthorized with a stable error code such as `auth_invalid_token` or `admin_acting_not_allowed`.

Add a remote client test:

```go
_, err := NewClient(Options{
	BaseURL: "http://127.0.0.1:8080",
	Token:   "xuanchu_act_test",
})
if err == nil || !strings.Contains(err.Error(), "acting") {
	t.Fatalf("NewClient(acting token) error = %v", err)
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./internal/httpapi -run 'MCP.*Acting|Acting.*MCP' -count=1
go test ./internal/remote -run 'Acting|Token' -count=1
```

Expected: FAIL if acting tokens are accepted by `/mcp` or if `remote.NewClient` accepts an acting token.

- [ ] **Step 3: Add guard**

Prefer a small middleware or check in `handleMCP` path that rejects `authn.Authn.Token.Type == auth.TokenTypeAdminActing` before constructing MCP server.

In `internal/remote/client.go`, reject `strings.HasPrefix(token, auth.ActingTokenPrefix)` in `NewClient` with a stable error code or message such as `remote_acting_token_not_allowed`. This is the enforceable remote CLI boundary; a caller that bypasses the CLI and handcrafts ordinary HTTP requests is still governed by the ordinary HTTP acting-token rules.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/httpapi -run 'MCP|Acting' -count=1
go test ./internal/remote -run 'Acting|Token' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit chunk 2**

```bash
git add internal/storage/workspace_repo.go \
  internal/app/admin_workspace.go internal/app/admin_workspace_test.go internal/app/token.go \
  internal/httpapi/router.go internal/httpapi/admin_auth.go internal/httpapi/admin.go \
  internal/httpapi/admin_workspace_test.go internal/httpapi/admin_test.go \
  internal/httpapi/middleware.go internal/httpapi/app_service.go internal/httpapi/mcp_test.go \
  internal/remote/client.go internal/remote/client_test.go
git commit -m "feat: 增加超管 workspace 管理接口"
```

---

## Chunk 3: 前端 Admin Workspace 与 Acting Mode

### Task 8: 增加 acting token 存储和 workspace API 切换

**Files:**
- Modify: `web/src/features/workspace/session/workspace-token.ts`
- Modify: `web/src/features/workspace/session/workspace-api.ts`
- Modify: `web/src/features/workspace/session/useMe.ts`
- Test: `web/src/features/workspace/session/workspace-api.test.ts`

- [ ] **Step 1: Write failing frontend token tests**

Cover:

- normal workspace requests use `xuanchu.console.token`.
- acting mode requests use `xuanchu.console.admin_acting_token`.
- unauthorized acting request clears acting token/context but not normal workspace token or admin token.
- `workspaceApi` still rejects `/api/v1/admin/*`.

- [ ] **Step 2: Run failing tests**

Run: `pnpm --dir web test -- workspace-api.test.ts`

Expected: FAIL.

- [ ] **Step 3: Implement helpers**

In `workspace-token.ts`, add:

- `getAdminActingToken`
- `setAdminActingToken`
- `clearAdminActingToken`
- `getAdminActingContext`
- `setAdminActingContext`
- `clearAdminActingContext`
- `clearAdminActingSession`

Keep existing workspace token helpers unchanged.

- [ ] **Step 4: Switch token source**

In `workspace-api.ts`, make `getToken` return acting token first, then normal workspace token. `onUnauthorized` should clear acting session if acting token was used; otherwise clear normal workspace token.

- [ ] **Step 5: Run tests**

Run: `pnpm --dir web test -- workspace-api.test.ts`

Expected: PASS.

### Task 9: 增加 admin workspace API client 和页面

**Files:**
- Create: `web/src/features/admin/workspaces/admin-workspace-api.ts`
- Create: `web/src/features/admin/workspaces/admin-workspaces-page.tsx`
- Create: `web/src/features/admin/workspaces/admin-workspace-detail-page.tsx`
- Create: `web/src/features/admin/workspaces/admin-acting-session-dialog.tsx`
- Test: `web/src/features/admin/workspaces/admin-workspaces-page.test.tsx`
- Test: `web/src/features/admin/workspaces/admin-workspace-detail-page.test.tsx`

- [ ] **Step 1: Write failing page tests**

Use mocked `fetch` like existing admin token tests.

Cover:

- list page renders workspace rows, counts, archived toggle.
- detail page renders owner/admin members and token counts.
- acting dialog calls `POST /api/v1/admin/workspaces/{workspace}/acting-sessions`.
- no candidate path shows “创建管理员并进入” flow, not a dead disabled state.

- [ ] **Step 2: Run failing tests**

Run: `pnpm --dir web test -- admin-workspaces-page.test.tsx admin-workspace-detail-page.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement API types**

Define types matching spec:

- `AdminWorkspaceSummary`
- `AdminWorkspaceDetail`
- `AdminWorkspaceMember`
- `AdminActingCandidate`
- `CreatedAdminActingSession`

- [ ] **Step 4: Implement pages**

Use existing admin token page style:

- border tables, not nested cards
- `useQuery` keys under `["admin", "workspaces"]`
- clear error display using `ApiError.code`
- action buttons with lucide icons

- [ ] **Step 5: Implement acting dialog**

On success:

- save acting token and context to sessionStorage
- navigate to `/workspaces/$workspaceSlug/projects` if possible, otherwise `/`

Do not overwrite normal workspace token.

- [ ] **Step 6: Run tests**

Run: `pnpm --dir web test -- admin-workspaces-page.test.tsx admin-workspace-detail-page.test.tsx`

Expected: PASS.

### Task 10: 接入 admin 路由、导航和 acting banner

**Files:**
- Modify: `web/src/routes/router.tsx`
- Create: `web/src/routes/admin/AdminWorkspacesRoute.tsx`
- Create: `web/src/routes/admin/AdminWorkspaceDetailRoute.tsx`
- Modify: `web/src/features/admin/components/AdminShell.tsx`
- Modify: `web/src/components/AppShell.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `internal/webconsole/console.go`
- Test: `internal/webconsole/console_test.go`
- Test: `web/src/components/AppShell.test.tsx`
- Test: `web/src/features/admin/bootstrap/bootstrap-wizard.test.tsx`

- [ ] **Step 1: Write failing route/shell tests**

Cover:

- AdminShell nav includes “Workspace 管理”.
- direct browser refresh on `/admin/workspaces` and `/admin/workspaces/dajee` returns `index.html`, not 404.
- acting context renders banner in Workspace Console.
- “返回超管界面” clears acting session and navigates to `/admin/workspaces/:workspace`.
- regular workspace console without acting context is unchanged.

- [ ] **Step 2: Run failing tests**

Run: `pnpm --dir web test -- AppShell.test.tsx bootstrap-wizard.test.tsx`

Expected: FAIL for new assertions.

- [ ] **Step 3: Add routes**

Register:

- `/admin/workspaces`
- `/admin/workspaces/$workspaceSlug`

under `adminGuardRoute`.

Also update `internal/webconsole/console.go` SPA fallback:

- add `admin/workspaces` to `spaRoutes`
- add `admin/workspaces/` to `spaPrefixes`

Keep `internal/webconsole/dist` as the existing checked-in placeholder only; do not commit Vite build output.

- [ ] **Step 4: Add nav**

Add a Workspace icon from lucide, e.g. `Building2`, to `AdminShell` nav.

- [ ] **Step 5: Add acting banner**

In `AppShell`, read acting context from session storage or a small hook. Banner should be fixed in the normal content flow, not floating over content.

Text:

```text
正在以 {{workspace}} 的 {{actor}}({{role}}) 身份操作 · 由 server admin {{adminTokenName}} 委托
```

Button: `返回超管界面`.

- [ ] **Step 6: Run route and frontend tests**

Run:

```bash
go test ./internal/webconsole -run 'Console|SPA|Admin' -count=1
pnpm --dir web test -- AppShell.test.tsx bootstrap-wizard.test.tsx admin-workspaces-page.test.tsx admin-workspace-detail-page.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit chunk 3**

```bash
git add web/src/features/workspace/session/workspace-token.ts \
  web/src/features/workspace/session/workspace-api.ts web/src/features/workspace/session/useMe.ts \
  web/src/features/admin/workspaces web/src/routes/admin/AdminWorkspacesRoute.tsx \
  web/src/routes/admin/AdminWorkspaceDetailRoute.tsx web/src/routes/router.tsx \
  web/src/features/admin/components/AdminShell.tsx web/src/components/AppShell.tsx \
  web/src/locales/zh-CN.ts web/src/locales/en-US.ts \
  internal/webconsole/console.go internal/webconsole/console_test.go \
  <本 chunk 新增或修改的具体 test 文件>
git commit -m "feat: 增加超管 workspace 页面和 acting 模式"
```

---

## Chunk 4: OpenAPI、文档与跨层验证

### Task 11: 更新 OpenAPI 和用户文档

**Files:**
- Modify: `docs/openapi/xuanchu-v1.yaml`
- Modify: `docs/manual/web-console.md`
- Modify: `docs/manual/deployment.md`
- Possibly Modify: `README.md` if it mentions admin console scope.

- [ ] **Step 1: Update OpenAPI**

Add schemas and paths:

- `GET /api/v1/admin/workspaces`
- `GET /api/v1/admin/workspaces/{workspace}`
- `POST /api/v1/admin/workspaces/{workspace}/acting-sessions`
- optional `DELETE /api/v1/admin/acting-sessions/{sessionID}`

- [ ] **Step 2: Update docs**

Document:

- admin token remains `/api/v1/admin/*` only
- acting session is short-lived browser-only
- returning to admin clears acting session but not admin token
- HTTP MCP 和 stdio MCP 不接受 acting token
- remote CLI client 拒绝 `xuanchu_act_` 前缀

- [ ] **Step 3: Run doc sanity checks**

Run:

```bash
rg -n "admin_acting|acting session|/api/v1/admin/workspaces" docs README.md
git diff --check
```

Expected: new endpoints and boundaries are discoverable, no whitespace errors.

### Task 12: 全量验证

**Files:** no code changes, verification only.

- [ ] **Step 1: Backend verification**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
```

Expected: PASS.

- [ ] **Step 2: Frontend verification**

Run:

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web build
```

Expected: PASS.

- [ ] **Step 3: Browser verification**

Start the server using the repo's existing command from `Makefile` or manual `go run ./cmd/xuanchu server ...` with admin enabled.

Verify in browser:

- `/admin/login` logs in with server admin token.
- `/admin/workspaces` lists all workspaces.
- workspace detail can create acting session.
- acting banner appears in Workspace Console.
- acting token cannot call `/mcp`.
- `xuanchu --server ... --token xuanchu_act_...` fails before sending an HTTP request.
- “返回超管界面” returns to `/admin/workspaces/:workspace`.

Stop any dev servers after verification.

- [ ] **Step 4: Final diff check**

Run:

```bash
git status --short
git diff --check
```

Expected: only intended files changed; pre-existing unrelated dirty files such as `Makefile` or `web/vite.config.ts` must remain unstaged; no whitespace errors.

- [ ] **Step 5: Commit docs and OpenAPI updates**

```bash
git add docs/openapi/xuanchu-v1.yaml docs/manual/web-console.md docs/manual/deployment.md README.md
git commit -m "docs: 说明超管 workspace 管理"
```

---

## Review Checklist For Implementers

- [ ] `xuanchu_admin_...` is never sent to ordinary `/api/v1/*`.
- [ ] `xuanchu_act_...` is never accepted by `/mcp` or stdio MCP; remote CLI rejects the prefix before sending requests.
- [ ] raw acting token is only returned in create response.
- [ ] all user identity outputs use `task.UserInfo` / `task.UserInfoToJSON`.
- [ ] admin and acting session audit payloads do not contain raw tokens.
- [ ] acting mode has a persistent visible banner.
- [ ] returning to admin clears acting token/context but preserves admin token.
- [ ] `CGO_ENABLED=0` tests and build pass.
