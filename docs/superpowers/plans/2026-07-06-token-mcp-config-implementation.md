# Token MCP Config Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Web Console `/tokens` 的每一行提供 MCP 配置按钮，按需 reveal 加密保存的 raw token，并生成可复制的 HTTP MCP 配置。

**Architecture:** 后端继续用 `token_hash` 做认证，在 `api_tokens` 增加加密后的 raw token envelope，仅供受控 reveal 使用。HTTP/Web Console 创建 token 时要求 `[security].config_secret_key` 可用；列表接口仍只返回 prefix，点击行内按钮时调用专用 `mcp-config` endpoint。前端新增一个 focused dialog，按 token 类型展示 endpoint、Bearer token、配置 JSON 和边界说明。

**Tech Stack:** Go 1.25、GORM、SQLite/PostgreSQL、chi/Huma HTTP、AES-256-GCM envelope；React 19、TanStack Query、shadcn/ui、lucide-react、Vitest、i18next。

---

## Source Spec

- `docs/superpowers/specs/2026-07-06-token-mcp-config-design.md`

## File Map

Backend storage:

- Modify: `internal/storage/models.go` - `ApiToken` 新增 `TokenSecretCiphertext`。
- Modify: `internal/storage/token_repo.go` - `ApiTokenEntry`、model/entry 映射、可选 update 字段同步新增。
- Modify: `internal/storage/token_repo_test.go` - 覆盖 ciphertext 持久化。
- Modify: `internal/storage/postgres_test.go` - 确认 PostgreSQL AutoMigrate 后字段存在。

Backend app:

- Modify: `internal/app/runtime.go` - `ServiceOptions` 增加 `TokenSecretKey []byte` 和 `RequireTokenSecret bool`。
- Modify: `internal/app/service.go` - `Service` 持有 token secret key / strict flag。
- Modify: `internal/app/token.go` - create 写 ciphertext，新增 reveal view 与 reveal 方法。
- Modify: `internal/app/token_test.go` - 覆盖 create / reveal / 缺 key / 无 ciphertext / 越权。
- Modify: `internal/app/admin_workspace.go` - admin 创建 tenant switch token 时按非 reveal 路径处理，避免短期 switch token 被普通 token UI reveal。
- Modify: `internal/app/admin_bootstrap.go` - admin 创建 workspace agent token 时是否保存 ciphertext 需显式决定，推荐传入 key 后保存。

HTTP API:

- Modify: `internal/httpapi/app_service.go` - scoped service 注入 `s.secretKey` 和严格 flag。
- Modify: `internal/httpapi/tokens.go` - 新增普通 token mcp-config response / handler。
- Modify: `internal/httpapi/tenant_tokens.go` - 新增 tenant token mcp-config response / handler。
- Modify: `internal/httpapi/router.go` - 注册两个新 route。
- Modify: `internal/httpapi/huma_routes.go` - 注册 OpenAPI route。
- Modify: `internal/httpapi/error_status.go` - 新错误码 status。
- Modify: `internal/httpapi/tokens_test.go` - HTTP reveal 覆盖。
- Modify: `internal/httpapi/admin_workspace_test.go` - 若 admin workspace agent token 也保存 ciphertext，补回归。

Frontend:

- Modify: `web/src/features/workspace/tokens/token-api.ts` - 新增 `TokenMcpConfig` 类型和 fetch helpers。
- Modify: `web/src/features/workspace/tokens/tokens-page.tsx` - 每行 prefix 后增加 MCP 配置按钮和 dialog state。
- Create: `web/src/features/workspace/tokens/token-mcp-config-dialog.tsx` - MCP 配置弹窗。
- Create: `web/src/features/workspace/tokens/token-mcp-config-dialog.test.tsx` - 弹窗单测。
- Modify: `web/src/features/workspace/tokens/tokens-page.test.tsx` - 行按钮和错误状态测试。
- Modify: `web/src/locales/zh-CN.ts` - 中文文案。
- Modify: `web/src/locales/en-US.ts` - 英文文案。

Docs:

- Modify: `README.md` - 更新 token 明文展示、MCP 配置按钮、`config_secret_key` 要求。
- Optional Modify: `docs/manual/mcp.md` - 若已有 MCP 手册，追加 Web Console reveal 说明。
- Optional Modify: `docs/manual/web-console.md` - 若已有 Web Console 手册，追加 `/tokens` 行内配置说明。

---

## Chunk 1: Storage 和 ServiceOptions 基础

### Task 1: 让 `api_tokens` 保存可恢复 token envelope

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/token_repo.go`
- Modify: `internal/storage/token_repo_test.go`
- Modify: `internal/storage/postgres_test.go`

- [ ] **Step 1: 写 storage 失败测试**

在 `internal/storage/token_repo_test.go` 新增：

```go
func TestTokenRepositoryPersistsTokenSecretCiphertext(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	entry := ApiTokenEntry{
		ID:                    "tok-secret",
		UserID:                ptrString("user-1"),
		Name:                  "agent",
		Type:                  "agent",
		TokenPrefix:           "xuanchu_agent_secret",
		TokenHash:             strings.Repeat("a", 64),
		TokenSecretCiphertext: "enc:v1:ciphertext",
		ScopesJSON:            `["task:read"]`,
		WorkspaceIDsJSON:      `["ws-1"]`,
		ProjectIDsJSON:        `[]`,
		CreatedAt:             100,
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenSecretCiphertext != entry.TokenSecretCiphertext {
		t.Fatalf("ciphertext = %q, want %q", got.TokenSecretCiphertext, entry.TokenSecretCiphertext)
	}
}
```

If `ptrString` already exists in this test file, reuse it. Do not add duplicate helpers.

- [ ] **Step 2: Run failing storage test**

Run:

```bash
go test ./internal/storage -run TestTokenRepositoryPersistsTokenSecretCiphertext -count=1
```

Expected: FAIL with `unknown field TokenSecretCiphertext` or empty persisted value.

- [ ] **Step 3: Add model field**

In `internal/storage/models.go`, update `ApiToken`:

```go
TokenSecretCiphertext string `gorm:"not null;default:''"`
```

Place it next to `TokenHash` so token secret material stays visually grouped:

```go
TokenPrefix           string `gorm:"not null;uniqueIndex:idx_api_tokens_prefix"`
TokenHash             string `gorm:"not null"`
TokenSecretCiphertext string `gorm:"not null;default:''"`
```

- [ ] **Step 4: Add repository entry field**

In `internal/storage/token_repo.go`, update `ApiTokenEntry`:

```go
TokenSecretCiphertext string
```

Update `apiTokenModel(entry)` and `apiTokenEntry(row)` to copy the field both ways.

- [ ] **Step 5: Check update support**

If `TokenUpdates` exists and is used by token modify/admin modify, add an optional field only if needed:

```go
TokenSecretCiphertext *string
```

Do not expose mutation paths that rotate the secret in this feature. The field is mainly written on create.

- [ ] **Step 6: Update PostgreSQL schema smoke expectation**

In `internal/storage/postgres_test.go`, if table-column assertions exist for `api_tokens`, add `token_secret_ciphertext`. If the test only checks table existence, no change is required.

- [ ] **Step 7: Run storage tests**

Run:

```bash
go test ./internal/storage -run 'TestTokenRepository|TestOpenCreatesAPITokenSchema|TestPostgres' -count=1
```

Expected: PASS.

### Task 2: Add token secret key plumbing to app service

**Files:**
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/token_test.go`

- [ ] **Step 1: Add failing app test for strict missing key**

In `internal/app/token_test.go`, add:

```go
func TestCreateTokenRequiresSecretKeyWhenStrict(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              testClock{now: 100},
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	assertRuntimeCode(t, err, "config_secret_key_missing")
}
```

Use existing store/test helpers if names differ; keep the behavior exact.

- [ ] **Step 2: Run failing app test**

Run:

```bash
go test ./internal/app -run TestCreateTokenRequiresSecretKeyWhenStrict -count=1
```

Expected: FAIL because `ServiceOptions.RequireTokenSecret` does not exist.

- [ ] **Step 3: Extend `ServiceOptions`**

In `internal/app/runtime.go`:

```go
type ServiceOptions struct {
	// existing fields...
	TokenSecretKey     []byte
	RequireTokenSecret bool
}
```

Comment:

```go
// TokenSecretKey encrypts recoverable API token secrets for Web Console reveal.
// RequireTokenSecret makes token creation fail when the key is missing.
```

- [ ] **Step 4: Extend `Service`**

In `internal/app/service.go`, add fields:

```go
tokenSecretKey     []byte
requireTokenSecret bool
```

In `NewService`, copy the key defensively:

```go
tokenSecretKey: append([]byte(nil), opts.TokenSecretKey...),
requireTokenSecret: opts.RequireTokenSecret,
```

- [ ] **Step 5: Add helper for encryption**

In `internal/app/token.go`, add private helper:

```go
func (s *Service) encryptRecoverableToken(raw string) (string, error) {
	if len(s.tokenSecretKey) != 32 {
		if s.requireTokenSecret {
			return "", RuntimeError{Code: "config_secret_key_missing", Message: "config secret key is required to create recoverable tokens"}
		}
		return "", nil
	}
	encrypted, err := EncryptConfigSecret(s.tokenSecretKey, raw)
	if err != nil {
		if errors.Is(err, ErrConfigSecretKeyMissing) || errors.Is(err, ErrConfigSecretKeyInvalid) {
			return "", RuntimeError{Code: "config_secret_key_missing", Message: "config secret key is required to create recoverable tokens"}
		}
		return "", err
	}
	return encrypted, nil
}
```

Add `errors` import if not already present.

- [ ] **Step 6: Run focused test**

Run:

```bash
go test ./internal/app -run TestCreateTokenRequiresSecretKeyWhenStrict -count=1
```

Expected: PASS after create path is wired in Chunk 2. If it still fails because create does not call the helper yet, continue to Chunk 2 before expecting PASS.

---

## Chunk 2: App token create and reveal

### Task 3: Write ciphertext on PAT / Agent token creation

**Files:**
- Modify: `internal/app/token.go`
- Modify: `internal/app/token_test.go`

- [ ] **Step 1: Add failing test for encrypted normal token**

In `internal/app/token_test.go`:

```go
func TestCreateTokenStoresRecoverableCiphertext(t *testing.T) {
	store := newTestStore(t)
	key := testSecretKey(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              testClock{now: 100},
		TokenSecretKey:     key,
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Stored.TokenSecretCiphertext == "" {
		t.Fatal("TokenSecretCiphertext is empty")
	}
	if strings.Contains(created.Stored.TokenSecretCiphertext, created.RawToken) {
		t.Fatal("ciphertext leaks raw token")
	}
	plain, err := DecryptConfigSecret(key, created.Stored.TokenSecretCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plain != created.RawToken {
		t.Fatalf("decrypted token = %q, want raw token", plain)
	}
}
```

If `testSecretKey(t)` does not exist in this file, add a small helper returning 32 bytes:

```go
func testSecretKey(t *testing.T) []byte {
	t.Helper()
	return []byte("01234567890123456789012345678901")
}
```

Only add this helper if absent.

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./internal/app -run 'TestCreateTokenStoresRecoverableCiphertext|TestCreateTokenRequiresSecretKeyWhenStrict' -count=1
```

Expected: FAIL because create does not write ciphertext.

- [ ] **Step 3: Extend create stored input**

In `internal/app/token.go`, add to `createTokenStoredInput`:

```go
TokenSecretCiphertext string
```

In `CreateToken`, after `auth.GenerateToken` and before constructing `storage.ApiTokenEntry`, call:

```go
ciphertext, err := s.encryptRecoverableToken(raw)
if err != nil {
	return CreatedToken{}, err
}
```

Thread it into `createTokenStoredInput`.

- [ ] **Step 4: Store ciphertext**

In `createTokenStored`, set:

```go
TokenSecretCiphertext: input.TokenSecretCiphertext,
```

Do not add raw token to audit payload.

- [ ] **Step 5: Run focused tests**

Run:

```bash
go test ./internal/app -run 'TestCreateTokenStoresRecoverableCiphertext|TestCreateTokenRequiresSecretKeyWhenStrict' -count=1
```

Expected: PASS.

### Task 4: Write ciphertext on tenant token creation

**Files:**
- Modify: `internal/app/token.go`
- Modify: `internal/app/token_test.go`

- [ ] **Step 1: Add failing test for tenant token ciphertext**

In `internal/app/token_test.go`:

```go
func TestCreateTenantAccessTokenStoresRecoverableCiphertext(t *testing.T) {
	store := newTestStore(t)
	key := testSecretKey(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              testClock{now: 100},
		TokenSecretKey:     key,
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Stored.TokenSecretCiphertext == "" {
		t.Fatal("tenant ciphertext is empty")
	}
	plain, err := DecryptConfigSecret(key, created.Stored.TokenSecretCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plain != created.RawToken {
		t.Fatalf("decrypted token = %q, want raw tenant token", plain)
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./internal/app -run TestCreateTenantAccessTokenStoresRecoverableCiphertext -count=1
```

Expected: FAIL because tenant create does not write ciphertext.

- [ ] **Step 3: Encrypt tenant raw token**

In `CreateTenantAccessToken`, after `auth.GenerateToken(auth.TokenTypeTenantAccess)`:

```go
ciphertext, err := tx.encryptRecoverableToken(raw)
if err != nil {
	return AuditEntry{}, err
}
```

Then set:

```go
TokenSecretCiphertext: ciphertext,
```

- [ ] **Step 4: Run tenant focused test**

Run:

```bash
go test ./internal/app -run TestCreateTenantAccessTokenStoresRecoverableCiphertext -count=1
```

Expected: PASS.

### Task 5: Implement app reveal methods

**Files:**
- Modify: `internal/app/token.go`
- Modify: `internal/app/token_test.go`

- [ ] **Step 1: Add reveal view type**

In `internal/app/token.go`, near token view types:

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
	ExpiresAt    *int64
	RevokedAt    *int64
}
```

- [ ] **Step 2: Add failing reveal success test**

In `internal/app/token_test.go`:

```go
func TestRevealTokenMCPConfigReturnsRawToken(t *testing.T) {
	store := newTestStore(t)
	key := testSecretKey(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              testClock{now: 100},
		TokenSecretKey:     key,
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.RevealTokenMCPConfig(created.View.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.RawToken != created.RawToken || view.EndpointPath != "/mcp" || view.TokenType != auth.TokenTypeAgent {
		t.Fatalf("view = %#v", view)
	}
}
```

- [ ] **Step 3: Add failure tests**

Add tests:

```go
func TestRevealTokenMCPConfigMissingCiphertext(t *testing.T) {}
func TestRevealTokenMCPConfigRejectsTenantOnNormalEndpoint(t *testing.T) {}
func TestRevealTenantTokenMCPConfigReturnsRawToken(t *testing.T) {}
func TestRevealTenantTokenMCPConfigRejectsNormalToken(t *testing.T) {}
func TestRevealTokenMCPConfigRequiresRequestScope(t *testing.T) {}
```

Expected codes:

- missing ciphertext: `token_secret_unavailable`
- wrong type / hidden target: `token_not_found`
- request scope denial: `token_scope_denied`, `workspace_scope_denied`, or `project_scope_denied` according to existing helper behavior.

- [ ] **Step 4: Run failing reveal tests**

Run:

```bash
go test ./internal/app -run 'TestReveal.*MCPConfig' -count=1
```

Expected: FAIL because methods do not exist.

- [ ] **Step 5: Implement shared decrypt helper**

In `internal/app/token.go`:

```go
func (s *Service) decryptRecoverableToken(row storage.ApiTokenEntry) (string, error) {
	if strings.TrimSpace(row.TokenSecretCiphertext) == "" {
		return "", RuntimeError{Code: "token_secret_unavailable", Message: "token secret is unavailable"}
	}
	raw, err := DecryptConfigSecret(s.tokenSecretKey, row.TokenSecretCiphertext)
	if err != nil {
		if errors.Is(err, ErrConfigSecretKeyMissing) || errors.Is(err, ErrConfigSecretKeyInvalid) {
			return "", RuntimeError{Code: "config_secret_key_missing", Message: "config secret key is required to reveal token"}
		}
		return "", RuntimeError{Code: "token_secret_unavailable", Message: "token secret is unavailable"}
	}
	return raw, nil
}
```

- [ ] **Step 6: Implement lookup and scope checks**

Implement:

```go
func (s *Service) RevealTokenMCPConfig(tokenRef string) (TokenMCPConfigView, error)
func (s *Service) RevealTenantTokenMCPConfig(tokenRef string) (TokenMCPConfigView, error)
```

Rules:

1. Use existing token lookup by id/prefix.
2. Normal method accepts only `auth.TokenTypePAT` and `auth.TokenTypeAgent`.
3. Tenant method accepts only `auth.TokenTypeTenantAccess`.
4. Reuse existing target token scope enforcement helpers. For normal tokens, use the same limits as token read/list management; for tenant tokens, use `enforceTenantTokenWriteLimit`-style subset logic but read-only. If no exact helper exists, extract a small helper instead of duplicating many checks.
5. Do not reject revoked/expired target tokens; include their timestamps in the view.
6. Always set `EndpointPath: "/mcp"`.

- [ ] **Step 7: Add audit for reveal**

Wrap reveal with `s.withAudit(...)`:

- normal token action: `token.mcp_config_reveal`
- tenant token action: `tenant_token.mcp_config_reveal`

Payload must include only id/name/type/prefix. It must not include `RawToken`, `TokenSecretCiphertext`, or generated config JSON.

- [ ] **Step 8: Run app reveal tests**

Run:

```bash
go test ./internal/app -run 'TestCreate.*Recoverable|TestReveal.*MCPConfig|TestAudit.*MCPConfig' -count=1
```

Expected: PASS.

---

## Chunk 3: HTTP endpoints

### Task 6: Inject token secret key into HTTP-scoped services

**Files:**
- Modify: `internal/httpapi/app_service.go`
- Modify: `internal/httpapi/tokens.go`
- Modify: `internal/httpapi/tenant_tokens.go`

- [ ] **Step 1: Update HTTP app service construction**

In `internal/httpapi/app_service.go`, wherever `app.NewService` is used for normal HTTP request handling, pass:

```go
TokenSecretKey:     s.secretKey,
RequireTokenSecret: true,
```

This makes Web Console / HTTP token creation fail fast when `[security].config_secret_key` is missing.

Important: `RequireTokenSecret` must only affect methods that create or reveal recoverable token secrets. Ordinary read/write HTTP handlers that do not touch token secret material must continue to work when the server has no `[security].config_secret_key`.

- [ ] **Step 2: Update direct token create handlers if needed**

`handleTokenCreate` and `handleTenantTokenCreate` use `scopedService`, so Step 1 should cover them. Confirm no direct `app.NewService` path is used for token creation without the key.

- [ ] **Step 3: Do not force MCP tool services to require token secret**

`internal/mcpserver/auth.go` constructs app services for tool execution. Do not change it for this feature. If `token_create` MCP should also create revealable tokens later, thread a key into MCP Options in a separate follow-up. For this feature, Web Console is the required surface.

- [ ] **Step 4: Run compile**

Run:

```bash
go test ./internal/httpapi ./internal/mcpserver -run TestNonExistent -count=1
```

Expected: PASS compile-only.

### Task 7: Add HTTP mcp-config response and handlers

**Files:**
- Modify: `internal/httpapi/tokens.go`
- Modify: `internal/httpapi/tenant_tokens.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/error_status.go`

- [ ] **Step 1: Add response type for normal tokens**

In `internal/httpapi/tokens.go`:

```go
type tokenMCPConfigResponse struct {
	Token       string   `json:"token"`
	TokenID     string   `json:"token_id"`
	TokenName   string   `json:"token_name"`
	TokenType   string   `json:"token_type"`
	Prefix      string   `json:"prefix"`
	EndpointPath string  `json:"endpoint_path"`
	Scopes      []string `json:"scopes"`
	WorkspaceIDs []string `json:"workspace_ids,omitempty"`
	ProjectIDs   []string `json:"project_ids,omitempty"`
	ExpiresAt   *int64   `json:"expires_at,omitempty"`
	RevokedAt   *int64   `json:"revoked_at,omitempty"`
}
```

Use consistent gofmt alignment; the exact spacing does not matter.

- [ ] **Step 2: Add converter helper**

```go
func tokenMCPConfigResponseFromView(view app.TokenMCPConfigView) tokenMCPConfigResponse {
	return tokenMCPConfigResponse{
		Token:        view.RawToken,
		TokenID:      view.TokenID,
		TokenName:    view.TokenName,
		TokenType:    view.TokenType,
		Prefix:       view.Prefix,
		EndpointPath: view.EndpointPath,
		Scopes:       append([]string(nil), view.Scopes...),
		WorkspaceIDs: append([]string(nil), view.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), view.ProjectIDs...),
		ExpiresAt:    view.ExpiresAt,
		RevokedAt:    view.RevokedAt,
	}
}
```

- [ ] **Step 3: Add normal token handler**

```go
func (s *Server) handleTokenMCPConfig(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTokenRead, app.PermissionTokenRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.RevealTokenMCPConfig(chi.URLParam(r, "tokenRef"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tokenMCPConfigResponseFromView(view), nil)
}
```

- [ ] **Step 4: Add tenant token handler**

In `internal/httpapi/tenant_tokens.go`:

```go
func (s *Server) handleTenantTokenMCPConfig(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTokenRead, app.PermissionTokenRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.RevealTenantTokenMCPConfig(chi.URLParam(r, "tokenRef"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tokenMCPConfigResponseFromView(view), nil)
}
```

Keep the response shape identical across normal and tenant endpoints.

- [ ] **Step 5: Register chi routes**

In `internal/httpapi/router.go`, add:

```go
r.Get("/api/v1/tokens/{tokenRef}/mcp-config", s.handleTokenMCPConfig)
r.Get("/api/v1/tenant-access-tokens/{tokenRef}/mcp-config", s.handleTenantTokenMCPConfig)
```

Place them before broader `{tokenRef}` routes if route order matters.

- [ ] **Step 6: Register Huma route metadata**

In `internal/httpapi/huma_routes.go`, add matching GET routes under the existing token route groups:

```go
{Method: http.MethodGet, Path: "/api/v1/tokens/{tokenRef}/mcp-config", Tag: "Tokens", Summary: "Reveal MCP configuration for a token.", Handler: s.handleTokenMCPConfig},
{Method: http.MethodGet, Path: "/api/v1/tenant-access-tokens/{tokenRef}/mcp-config", Tag: "Tenant Access Tokens", Summary: "Reveal MCP configuration for a tenant access token.", Handler: s.handleTenantTokenMCPConfig},
```

- [ ] **Step 7: Add error status**

In `internal/httpapi/error_status.go`, map:

```go
"token_secret_unavailable": http.StatusConflict,
"config_secret_key_missing": http.StatusBadRequest,
```

If `config_secret_key_missing` already maps elsewhere for SSO, reuse the existing mapping.

- [ ] **Step 8: Run compile**

Run:

```bash
go test ./internal/httpapi -run TestNonExistent -count=1
```

Expected: PASS compile-only.

### Task 8: HTTP tests for mcp-config

**Files:**
- Modify: `internal/httpapi/tokens_test.go`

- [ ] **Step 1: Add test fixture with config secret key**

Extend the existing token HTTP fixture helper or create a new one:

```go
func newHTTPServerWithAgentTokenFixtureAndSecret(t *testing.T, scopes ...string) (httpTokenFixture, string) {
	fixture, raw := newHTTPServerWithAgentTokenFixture(t, scopes...)
	// If helper cannot inject Options.ConfigSecretKey, create a new server fixture that passes:
	// ConfigSecretKey: base64.StdEncoding.EncodeToString(testSecretKey(t))
	return fixture, raw
}
```

Prefer adjusting the existing fixture to accept functional options if it keeps tests smaller.

- [ ] **Step 2: Test normal token reveal**

Add:

```go
func TestTokenMCPConfigRevealsRecoverableToken(t *testing.T) {}
```

Flow:

1. Create server with `ConfigSecretKey`.
2. POST `/api/v1/tokens` with agent token body.
3. Capture created `token`.
4. GET `/api/v1/tokens/{createdID}/mcp-config`.
5. Assert response `data.token == created.token`, `endpoint_path == "/mcp"`, `token_type == "agent"`.
6. Assert raw token does not appear in audit payload if audit endpoint/helper is available.

- [ ] **Step 3: Test tenant token reveal**

Add:

```go
func TestTenantTokenMCPConfigRevealsRecoverableToken(t *testing.T) {}
```

Flow:

1. Create tenant token through `/api/v1/tenant-access-tokens`.
2. GET `/api/v1/tenant-access-tokens/{id}/mcp-config`.
3. Assert full token and endpoint path.

- [ ] **Step 4: Test missing key on HTTP create**

Add:

```go
func TestTokenCreateRequiresConfigSecretKeyForHTTP(t *testing.T) {}
func TestTenantTokenCreateRequiresConfigSecretKeyForHTTP(t *testing.T) {}
```

Expected HTTP status should match `config_secret_key_missing`.

- [ ] **Step 5: Test unavailable ciphertext**

Manually insert or update a token row with `token_secret_ciphertext=''`, then call mcp-config. Expect `409 token_secret_unavailable`.

- [ ] **Step 6: Test permission denial**

Use a token with only `task:read`, call normal token mcp-config. Expect `403 token_scope_denied`.

- [ ] **Step 7: Run HTTP tests**

Run:

```bash
go test ./internal/httpapi -run 'Test.*MCPConfig|Test.*RequiresConfigSecretKey' -count=1
```

Expected: PASS.

---

## Chunk 4: Frontend API and dialog

### Task 9: Add frontend mcp-config API helpers

**Files:**
- Modify: `web/src/features/workspace/tokens/token-api.ts`

- [ ] **Step 1: Add type**

Add:

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

- [ ] **Step 2: Add endpoint helpers**

Import `workspaceApiGet` from `@/features/workspace/session/workspace-api` if not already imported. Add:

```ts
export function getTokenMcpConfig(ref: string): Promise<TokenMcpConfig> {
  return workspaceApiGet<TokenMcpConfig>(`/api/v1/tokens/${encodeURIComponent(ref)}/mcp-config`)
}

export function getTenantTokenMcpConfig(ref: string): Promise<TokenMcpConfig> {
  return workspaceApiGet<TokenMcpConfig>(`/api/v1/tenant-access-tokens/${encodeURIComponent(ref)}/mcp-config`)
}
```

- [ ] **Step 3: Add query key helper**

```ts
export function tokenMcpConfigQueryKey(type: string, id: string) {
  return ["token", "mcp-config", type, id] as const
}
```

- [ ] **Step 4: Run frontend typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS or fail only because dialog is not implemented yet if helpers are referenced. If standalone helpers compile, continue.

### Task 10: Implement `TokenMcpConfigDialog`

**Files:**
- Create: `web/src/features/workspace/tokens/token-mcp-config-dialog.tsx`
- Create: `web/src/features/workspace/tokens/token-mcp-config-dialog.test.tsx`

- [ ] **Step 1: Write failing dialog tests**

Create `token-mcp-config-dialog.test.tsx` with Testing Library. Cover:

1. Renders endpoint, token, JSON config.
2. Agent token with `impersonate` shows optional `X-Xuanchu-As` hint.
3. Tenant token shows system-identity warning.
4. `token_secret_unavailable` error shows reissue message.
5. Copy buttons call `navigator.clipboard.writeText`.

Use `vi.spyOn(globalThis, "fetch")` or mock `getTokenMcpConfig` if the project pattern favors API mock. Prefer fetch-level mock to match existing tokens page tests.

- [ ] **Step 2: Run failing test**

Run:

```bash
pnpm --dir web test -- token-mcp-config-dialog
```

Expected: FAIL because component does not exist.

- [ ] **Step 3: Create component skeleton**

In `token-mcp-config-dialog.tsx`, import:

```ts
import { useQuery } from "@tanstack/react-query"
import { CheckIcon, CopyIcon, PlugIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
```

Use existing shadcn components:

```ts
Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription
Button
Badge
Skeleton
```

Props:

```ts
type TokenMcpConfigDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: TokenRow | TenantAccessTokenRow
  mode: "api" | "tenant"
}
```

- [ ] **Step 4: Fetch config only when open**

Use:

```ts
const query = useQuery({
  queryKey: tokenMcpConfigQueryKey(token.type, token.id),
  queryFn: () => mode === "tenant" ? getTenantTokenMcpConfig(token.id) : getTokenMcpConfig(token.id),
  enabled: open,
  staleTime: 0,
  gcTime: 0,
})
```

- [ ] **Step 5: Compute endpoint and JSON**

```ts
const endpoint = new URL(query.data?.endpoint_path ?? "/mcp", window.location.origin).toString()
const config = {
  url: endpoint,
  headers: {
    Authorization: `Bearer ${query.data.token}`,
  },
}
```

If `token_type === "agent"` and scopes include `impersonate`, show a separate optional header snippet, not merged into default config.

- [ ] **Step 6: Implement copy helper**

Use local copied key state:

```ts
const [copied, setCopied] = useState<string | null>(null)
async function copy(label: string, value: string) {
  await navigator.clipboard?.writeText(value)
  setCopied(label)
}
```

Do not write anything to sessionStorage/localStorage.

- [ ] **Step 7: Render error states**

If API error code is `token_secret_unavailable`, show `t("token.secretUnavailable")`.

If code is `config_secret_key_missing`, show a specific message like `token.secretKeyMissing`.

Fallback to `token.errors.unknown`.

- [ ] **Step 8: Render status**

Use existing `deriveTokenStatus(token)` or `deriveTokenStatus(query.data ?? token)` and show a badge. Revoked/expired should not hide config, but should show warning text.

- [ ] **Step 9: Run dialog tests**

Run:

```bash
pnpm --dir web test -- token-mcp-config-dialog
```

Expected: PASS.

---

## Chunk 5: Frontend tokens page integration

### Task 11: Add row-level MCP config button

**Files:**
- Modify: `web/src/features/workspace/tokens/tokens-page.tsx`
- Modify: `web/src/features/workspace/tokens/tokens-page.test.tsx`

- [ ] **Step 1: Add failing tokens page tests**

In `tokens-page.test.tsx`, add:

```ts
it("opens MCP config dialog for API token rows", async () => {})
it("opens MCP config dialog for tenant token rows", async () => {})
it("shows secret unavailable message from MCP config dialog", async () => {})
```

Mock:

- `/api/v1/credentials/current`
- `/api/v1/tokens`
- `/api/v1/tokens/tok-1/mcp-config`
- `/api/v1/tenant-access-tokens`
- `/api/v1/tenant-access-tokens/tenant-1/mcp-config`

Assert the button role by aria label:

```ts
screen.getByRole("button", { name: "MCP 配置：ci-deploy" })
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
pnpm --dir web test -- tokens-page
```

Expected: FAIL because row button/dialog is not wired.

- [ ] **Step 3: Add dialog state**

In `tokens-page.tsx`:

```ts
const [mcpTarget, setMcpTarget] = useState<{
  token: TokenRow | TenantAccessTokenRow
  mode: TokenTab
} | null>(null)
```

- [ ] **Step 4: Add button next to prefix**

Change prefix cell from plain text to flex row:

```tsx
<TableCell className="font-mono text-xs text-muted-foreground">
  <div className="flex items-center gap-2">
    <span>{row.prefix}</span>
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          aria-label={t("token.mcpConfigFor", { name: row.name })}
          onClick={() => setMcpTarget({ token: row, mode: activeTab })}
          size="icon-sm"
          type="button"
          variant="ghost"
        >
          <PlugIcon className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("token.mcpConfig")}</TooltipContent>
    </Tooltip>
  </div>
</TableCell>
```

Import `PlugIcon` and Tooltip components.

- [ ] **Step 5: Mount dialog**

Near other dialogs:

```tsx
{mcpTarget ? (
  <TokenMcpConfigDialog
    mode={mcpTarget.mode}
    onOpenChange={(open) => setMcpTarget(open ? mcpTarget : null)}
    open={mcpTarget !== null}
    token={mcpTarget.token}
  />
) : null}
```

- [ ] **Step 6: Ensure colSpan remains correct**

No new table column is added, so empty-state `colSpan={7}` remains correct.

- [ ] **Step 7: Run tokens page tests**

Run:

```bash
pnpm --dir web test -- tokens-page
```

Expected: PASS.

### Task 12: Add i18n strings

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: Add Chinese strings under `token`**

Add:

```ts
mcpConfig: "MCP 配置",
mcpConfigFor: "MCP 配置：{{name}}",
mcpEndpoint: "MCP Endpoint",
bearerToken: "Bearer Token",
copyConfig: "复制配置",
copyEndpoint: "复制 Endpoint",
secretUnavailable: "该令牌没有保存可恢复密文，请重新签发后再复制完整 token。",
secretKeyMissing: "服务端未配置 config secret key，无法读取完整 token。",
mcp: {
  configJson: "MCP 客户端配置",
  optionalHeaders: "可选 Header",
  revokedWarning: "该令牌已吊销，配置仅供排查，不能继续使用。",
  expiredWarning: "该令牌已过期，配置仅供排查，不能继续使用。",
  agentHint: "Agent token 可用于 HTTP MCP。若要代表成员执行，请配置 X-Xuanchu-As，并确保 token 包含 impersonate scope。",
  patHint: "PAT 可用于 HTTP MCP，但不能使用 X-Xuanchu-As impersonation。",
  tenantHint: "Tenant token 以系统身份调用 HTTP MCP，不绑定自然人用户，不支持 impersonation、me_get、assignee:me 或个人 active context。",
},
```

- [ ] **Step 2: Add English strings under `token`**

Use equivalent concise English:

```ts
mcpConfig: "MCP config",
mcpConfigFor: "MCP config: {{name}}",
mcpEndpoint: "MCP endpoint",
bearerToken: "Bearer token",
copyConfig: "Copy config",
copyEndpoint: "Copy endpoint",
secretUnavailable: "This token has no recoverable secret. Reissue it before copying the full token.",
secretKeyMissing: "The server has no config secret key, so the full token cannot be revealed.",
mcp: {
  configJson: "MCP client config",
  optionalHeaders: "Optional headers",
  revokedWarning: "This token is revoked. The config is shown only for troubleshooting.",
  expiredWarning: "This token is expired. The config is shown only for troubleshooting.",
  agentHint: "Agent tokens can call HTTP MCP. To act as a member, set X-Xuanchu-As and ensure the token has the impersonate scope.",
  patHint: "PATs can call HTTP MCP, but cannot use X-Xuanchu-As impersonation.",
  tenantHint: "Tenant tokens call HTTP MCP as a system identity. They do not bind to a user and cannot use impersonation, me_get, assignee:me, or personal active context.",
},
```

- [ ] **Step 3: Run frontend checks**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test -- token-mcp-config-dialog tokens-page
```

Expected: PASS.

---

## Chunk 6: Docs and final verification

### Task 13: Update docs

**Files:**
- Modify: `README.md`
- Optional Modify: `docs/manual/mcp.md`
- Optional Modify: `docs/manual/web-console.md`

- [ ] **Step 1: Update `/tokens` Web Console description**

In `README.md`, update the paragraph that currently says created token plaintext is shown only once. New meaning:

- raw token is still not returned in list rows.
- Web Console can reveal full token from the row-level MCP config button when the token was created after recoverable secret storage is enabled.
- reveal requires `token:read` and follows workspace/project allowlist.

- [ ] **Step 2: Update token storage description**

In the token section, change “数据库只保存 hash 和短 prefix” to:

```text
认证仍只使用 hash 和短 prefix；server 配置了 [security].config_secret_key 时，HTTP/Web Console 创建的 token 还会保存加密后的 raw token envelope，用于后续在 /tokens 页面按需展示 MCP 配置。
```

- [ ] **Step 3: Update MCP section**

In the HTTP MCP section, add a short note:

```text
Web Console 的 /tokens 页面可从每行「MCP 配置」按钮复制 endpoint、Bearer token 和客户端配置片段；admin token 和 acting token 仍不能用于 /mcp。
```

- [ ] **Step 4: Update manual files if present**

Run:

```bash
test -f docs/manual/mcp.md && rg -n "HTTP MCP|token|/mcp" docs/manual/mcp.md
test -f docs/manual/web-console.md && rg -n "tokens|Token|MCP" docs/manual/web-console.md
```

If these files exist and contain relevant sections, add the same concise notes. If absent, skip.

### Task 14: Backend full verification

- [ ] **Step 1: gofmt**

Run:

```bash
gofmt -w internal/storage/models.go internal/storage/token_repo.go internal/app/runtime.go internal/app/service.go internal/app/token.go internal/httpapi/app_service.go internal/httpapi/tokens.go internal/httpapi/tenant_tokens.go internal/httpapi/router.go internal/httpapi/huma_routes.go internal/httpapi/error_status.go internal/storage/token_repo_test.go internal/app/token_test.go internal/httpapi/tokens_test.go
```

- [ ] **Step 2: Storage/app/http focused tests**

Run:

```bash
go test ./internal/storage ./internal/app ./internal/httpapi -run 'Test.*Token.*Secret|Test.*MCPConfig|Test.*RequiresConfigSecretKey|TestTokenRepositoryPersistsTokenSecretCiphertext' -count=1
```

Expected: PASS.

- [ ] **Step 3: Full Go tests**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS.

### Task 15: Frontend full verification

- [ ] **Step 1: Typecheck**

Run:

```bash
pnpm --dir web typecheck
```

Expected: PASS.

- [ ] **Step 2: Tests**

Run:

```bash
pnpm --dir web test -- token-mcp-config-dialog tokens-page
pnpm --dir web test
```

Expected: PASS.

- [ ] **Step 3: Lint and build**

Run:

```bash
pnpm --dir web lint
pnpm --dir web build
```

Expected: PASS.

### Task 16: Manual smoke check

- [ ] **Step 1: Start server with secret key**

Generate a dev key if needed:

```bash
python3 - <<'PY'
import base64, os
print(base64.b64encode(os.urandom(32)).decode())
PY
```

Use a temp config or env path already supported by the repo; start:

```bash
go run ./cmd/xuanchu server --listen :8080
```

Expected: server starts with `[security].config_secret_key` configured.

- [ ] **Step 2: Open Web Console**

Navigate to:

```text
http://127.0.0.1:8080/tokens
```

Create an Agent token through the UI, close the created-token result, click the row MCP button, and verify:

- endpoint is `http://127.0.0.1:8080/mcp`
- Bearer token is copyable
- config JSON contains `Authorization: Bearer ...`
- token list network response still does not include full token

- [ ] **Step 3: Tenant tab smoke**

Switch to tenant token tab, create a tenant token, open MCP config, verify tenant-specific warning is shown.

- [ ] **Step 4: Stop server**

Stop any local server started for smoke. Confirm no stale process remains on the chosen port if the user asks for cleanup.

### Task 17: Final hygiene

- [ ] **Step 1: Diff check**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; changed files match this plan.

- [ ] **Step 2: Update implementation status**

If implementation completes, update this plan checkboxes or add a short completion note with exact commands run and results. Do not mark complete unless all required verification passed or failures are explicitly documented.

- [ ] **Step 3: Suggested commit**

```bash
git add internal/storage internal/app internal/httpapi web/src/features/workspace/tokens web/src/locales README.md docs/manual docs/superpowers/plans/2026-07-06-token-mcp-config-implementation.md
git commit -m "feat: 添加 token MCP 配置弹窗"
```

Only include files actually changed. Do not stage unrelated user edits.
