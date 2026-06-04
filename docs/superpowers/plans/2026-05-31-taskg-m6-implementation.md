# taskg M6 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 M6：同一个 `taskg` 二进制支持 HTTP/JSON API 服务端、PAT/Agent token、request scope、远程 CLI，并保持所有业务规则复用 `internal/app`。

**Architecture:** 新增 `internal/auth` 处理 token 生成、hash 与 scope；新增 `api_tokens` 存储和 app token service；新增 `internal/httpapi` 作为协议层，只做鉴权、request scope、JSON envelope 和 handler 编排。远程 CLI 通过 `internal/remote` 调用 HTTP API，再复用现有 render 输出，不在客户端复制业务逻辑。

**Tech Stack:** Go 1.22、Cobra、GORM、`github.com/glebarez/sqlite`（保持 `CGO_ENABLED=0`）、`net/http`、`github.com/go-chi/chi/v5`、现有 `internal/app` / `internal/storage` / `internal/render` / CLI 集成测试、OpenAPI 3 YAML。

---

## 范围锁定

严格按 [M6 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-31-taskg-m6-design.md) 实现。

必须进入 M6：

- `taskg server --listen ...`。
- `api_tokens` 表、token 生成、hash、verify、revoke、`last_used_at`。
- `token create/list/revoke` 本地 CLI 与 HTTP API。
- Authorization Bearer only，不支持 query token / cookie / Basic Auth。
- HTTP success/error envelope、`/healthz`、`/api/v1/me`。
- request scope：token capability、workspace allowlist、project allowlist、membership role 的 6 步授权流水线。
- HTTP endpoint：workspace/member/project/task/report/context/config/import/export/audit/token 的 M6 范围。
- 远程 CLI：核心 task/report/project/context/config/helper/token 命令。
- project-scoped token 全局过滤，单任务越界读固定返回 404 `task_not_found`。
- OpenAPI：`docs/openapi/taskg-v1.yaml` 随 endpoint chunk 同步维护。
- README、ROADMAP、requirements 在 M6 完成后同步。

不进入 M6：

- MCP。
- JWT / password login / refresh token。
- OAuth。
- system keychain。
- op-log sync。
- webhook trigger。
- 外部 adapter。
- project RBAC override / project 成员表。
- cursor pagination / bulk mutation。
- 高并发 SQLite 优化。

## 执行注意事项

- 不要把业务规则写进 `internal/httpapi` 或 `internal/remote`；缺 app 方法就补 `internal/app`。
- token capability 空数组表示无权限；workspace/project allowlist 空数组表示不额外收窄。
- 所有远程/API 写操作必须有明确 actor，M6 不使用 `actor_user_id = NULL` 写 audit。
- 第 1-5 步鉴权/授权失败不写 audit。
- `taskg server` 启动不输出 bootstrap token；第一个 token 由本地 CLI 预先创建。
- `taskg.toml` 中 `remote.token` 只作为便利功能；权限宽于 `0600` 时 stderr warning。
- HTTP path `{uuid}` 只接受真实 UUID；远程数字 working-set ID 由客户端两跳解析。
- helper 远程化不要新增下划线 API；优先用已有 endpoint 本地后处理。
- 每个 endpoint chunk 同步更新 OpenAPI，不要最后一次性补。
- 每个 chunk 完成后至少跑对应包测试；跨 chunk 后跑 `go test ./...`。

## 文件结构

新增文件：

- `internal/auth/token.go`
  raw token 生成、prefix、SHA-256 hash、常量时间比较、PAT/Agent token 前缀。
- `internal/auth/scope.go`
  capability scope 解析、校验、StringSlice 输入归一化、Agent token 硬约束。
- `internal/auth/token_test.go`
  token 格式、hash、常量时间 verify、scope parser 测试。
- `internal/storage/token_repo.go`
  `api_tokens` CRUD、按 prefix 查候选、revoke、list active、更新 `last_used_at`。
- `internal/storage/token_repo_test.go`
  schema、raw token 不落库、过期/revoked/list/update 测试。
- `internal/app/token.go`
  `CreateToken/ListTokens/RevokeToken/AuthenticateBearerToken`，权限、audit、workspace/project scope 校验。
- `internal/app/request_scope.go`
  HTTP/远程 request scope 解析、6 步授权流水线、project allowlist filter helper。
- `internal/app/token_test.go`
  token app service、bootstrap、本地/远程权限、scope 失败码测试。
- `internal/httpapi/server.go`
  router、server options、graceful shutdown 入口。
- `internal/httpapi/router.go`
  集中注册 chi routes、NotFound/MethodNotAllowed handler 和 middleware 顺序，避免 `server.go` 膨胀。
- `internal/httpapi/envelope.go`
  success/error envelope、status code 映射、`details` 开放对象。
- `internal/httpapi/middleware.go`
  request id、access log、auth middleware、body limit。
- `internal/httpapi/scope.go`
  从 header/query/path/body 解析 workspace/project/request scope。
- `internal/httpapi/handlers_*.go`
  按资源拆 handler：health/me/token/workspace/member/project/task/report/context/config/import_export/audit。
- `internal/httpapi/*_test.go`
  handler 与 scope 的 `httptest` 覆盖。
- `internal/remote/client.go`
  HTTP client、base URL、Bearer、JSON request/response、error 解码。
- `internal/remote/task.go`
  远程 task/report/working-set 两跳 helper。
- `internal/remote/project.go`
  远程 project/project config。
- `internal/remote/config.go`
  远程 context/config/helper。
- `internal/remote/client_test.go`
  URL 编码、Bearer-only、error envelope、remote unsupported 测试。
- `internal/cli/server.go`
  `taskg server` 子命令。
- `internal/cli/token.go`
  `taskg token create/list/revoke`。
- `internal/cli/dispatch.go`
  remote/local dispatch helper，避免每个命令重复 `isRemoteMode` 判断模板。
- `docs/openapi/taskg-v1.yaml`
  M6 HTTP API OpenAPI 3 文档。

修改文件：

- `go.mod` / `go.sum`
  添加 `github.com/go-chi/chi/v5`，确认无 CGO。
- `internal/storage/models.go`
  新增 `ApiToken` model 和索引。
- `internal/storage/db.go`
  AutoMigrate `ApiToken`。M6 不创建 `idx_api_tokens_active` partial index。
- `internal/app/runtime.go`
  增加 request runtime / token scope 表达，支持不读 active workspace 的服务端构造路径。
- `internal/app/service.go`
  新增从已解析 runtime/scope 构造 service 的方法；task list/report/import/export 叠加 project allowlist。
- `internal/app/permission.go`
  增加 token capability 到 app permission 的映射 helper，不改变现有 role map。
- `internal/app/audit.go`
  token create/revoke audit；audit list 叠加 project allowlist。
- `internal/cli/root.go`
  注册 `server` / `token`，新增 `--server`、`--token`、`--project-id`、`--project` 远程相关全局 flag，split flag 逻辑同步。
- `internal/cli/dispatch.go`
  远程/本地分发 helper。
- `internal/cli/*.go`
  核心命令在 remote mode 下走 `internal/remote`，local mode 保持原逻辑。
- `internal/config/config.go` / `internal/config/toml.go`
  支持 `remote.server`、`remote.token`、`TASKG_SERVER`、`TASKG_TOKEN`；含 token TOML 权限 warning。
- `tests/integration/cli_test.go`
  增加 server/token/remote CLI 黑盒测试。
- `README.md`、`ROADMAP.md`、`docs/requirements.md`
  M6 完成后同步用户文档和状态。

---

## Chunk 1：Token Storage、Auth 与本地 token CLI

### Task 1：新增 auth token 生成与 scope parser

**Files:**
- Create: `internal/auth/token.go`
- Create: `internal/auth/scope.go`
- Test: `internal/auth/token_test.go`

- [ ] **Step 1：写失败的 token 单元测试**

在 `internal/auth/token_test.go` 新增：

```go
func TestGenerateRawTokenAndHash(t *testing.T) {
    raw, prefix, hash, err := auth.GenerateToken(auth.TokenTypePAT)
    if err != nil { t.Fatal(err) }
    if !strings.HasPrefix(raw, "taskg_pat_") { t.Fatalf("raw = %q", raw) }
    if len(prefix) < 12 || !strings.HasPrefix(raw, prefix) { t.Fatalf("prefix = %q raw = %q", prefix, raw) }
    if strings.Contains(hash, raw) { t.Fatalf("hash contains raw token") }
    if !auth.VerifyTokenHash(raw, hash) { t.Fatalf("hash did not verify") }
    if auth.VerifyTokenHash(raw+"x", hash) { t.Fatalf("wrong token verified") }
}

func TestParseScopesFailClosed(t *testing.T) {
    scopes, err := auth.ParseScopes(nil)
    if err != nil { t.Fatal(err) }
    if scopes.Has("task:read") { t.Fatalf("empty scope should not grant capability") }
    scopes, err = auth.ParseScopes([]string{"task:read,task:write", "project:read"})
    if err != nil { t.Fatal(err) }
    for _, want := range []string{"task:read", "task:write", "project:read"} {
        if !scopes.Has(want) { t.Fatalf("missing scope %s", want) }
    }
}

func TestAgentTokenRequiresExplicitWorkspaceAndScope(t *testing.T) {
    err := auth.ValidateTokenCreate(auth.CreateTokenOptions{Type: auth.TokenTypeAgent, Scopes: []string{"task:read"}})
    if err == nil || !strings.Contains(err.Error(), "workspace") { t.Fatalf("expected workspace error, got %v", err) }
    err = auth.ValidateTokenCreate(auth.CreateTokenOptions{Type: auth.TokenTypeAgent, WorkspaceIDs: []string{"w1"}})
    if err == nil || !strings.Contains(err.Error(), "scope") { t.Fatalf("expected scope error, got %v", err) }
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/auth -count=1
```

Expected: FAIL，`internal/auth` 不存在。

- [ ] **Step 3：实现最小 auth 包**

实现：

- `TokenTypePAT = "pat"`、`TokenTypeAgent = "agent"`。
- raw token 使用 `crypto/rand` 生成至少 32 bytes，base64url 无 padding。
- raw token 前缀为 `taskg_pat_` 或 `taskg_agent_`。
- `token_prefix` 取 raw token 前 16 字符。
- `token_hash` 用 SHA-256 hex。
- verify 使用 `subtle.ConstantTimeCompare`。
- `ParseScopes` 支持逗号分隔和重复 flag；空 scope 不报错但不授予能力。
- 不支持 `admin:*`。

- [ ] **Step 4：运行 auth 测试通过**

Run:

```bash
go test ./internal/auth -count=1
```

Expected: PASS。

- [ ] **Step 5：提交**

```bash
git add internal/auth
git commit -m "feat: 添加 token 生成与 scope 解析"
```

### Task 2：新增 api_tokens schema 与 repository

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/db.go`
- Create: `internal/storage/token_repo.go`
- Test: `internal/storage/token_repo_test.go`

- [ ] **Step 1：写失败的 schema/repo 测试**

新增测试覆盖：

```go
func TestOpenCreatesAPITokenSchema(t *testing.T) {
    store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
    if err != nil { t.Fatal(err) }
    defer store.Close()
    if !store.DB().Migrator().HasTable(&ApiToken{}) {
        t.Fatalf("missing api_tokens table")
    }
    assertIndexColumns(t, store, "idx_api_tokens_user", []string{"user_id"})
    assertIndexColumns(t, store, "idx_api_tokens_prefix", []string{"token_prefix"})
}

func TestTokenRepositoryDoesNotStoreRawToken(t *testing.T) {
    store := openTestStore(t)
    repo := NewTokenRepository(store.DB())
    row := ApiTokenEntry{ID: "tok1", UserID: "u1", Name: "cli", Type: "pat", TokenPrefix: "taskg_pat_abcd", TokenHash: strings.Repeat("a", 64), ScopesJSON: `["task:read"]`, WorkspaceIDsJSON: `[]`, ProjectIDsJSON: `[]`, CreatedAt: 100}
    if err := repo.Create(row); err != nil { t.Fatal(err) }
    got, err := repo.GetByPrefix("taskg_pat_abcd")
    if err != nil { t.Fatal(err) }
    if got.TokenHash != row.TokenHash { t.Fatalf("hash mismatch") }
    if strings.Contains(got.TokenHash, "taskg_pat_") { t.Fatalf("raw token leaked") }
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/storage -run 'Test(OpenCreatesAPITokenSchema|TokenRepository)' -count=1
```

Expected: FAIL，`ApiToken` / repo 不存在。

- [ ] **Step 3：实现 schema 与 repo**

新增 `ApiToken` model：

```go
type ApiToken struct {
    ID               string `gorm:"primaryKey"`
    UserID           string `gorm:"not null;index:idx_api_tokens_user"`
    Name             string `gorm:"not null"`
    Type             string `gorm:"not null"`
    TokenPrefix      string `gorm:"not null;uniqueIndex:idx_api_tokens_prefix"`
    TokenHash        string `gorm:"not null"`
    ScopesJSON       string `gorm:"not null;default:'[]'"`
    WorkspaceIDsJSON string `gorm:"not null;default:'[]'"`
    ProjectIDsJSON   string `gorm:"not null;default:'[]'"`
    CreatedAt        int64  `gorm:"not null"`
    ExpiresAt        *int64
    RevokedAt        *int64
    LastUsedAt       *int64
}
```

Repo 方法：

- `Create(ApiTokenEntry) error`
- `ListByUser(userID string, includeRevoked bool) ([]ApiTokenEntry, error)`
- `GetByPrefix(prefix string) (ApiTokenEntry, error)`
- `GetByIDOrPrefix(ref string) (ApiTokenEntry, error)`
- `Revoke(ref string, ts int64) error`
- `TouchLastUsed(id string, ts int64) error`

M6 不建 partial index `idx_api_tokens_active`。每个 user 的 token 量级预计很小，`ListByUser` 直接按 `user_id` 过滤即可；partial index 优化留给后续版本。

- [ ] **Step 4：运行 storage 测试**

Run:

```bash
go test ./internal/storage -run 'Test(OpenCreatesAPITokenSchema|TokenRepository)' -count=1
```

Expected: PASS。

- [ ] **Step 5：提交**

```bash
git add internal/storage/models.go internal/storage/db.go internal/storage/token_repo.go internal/storage/token_repo_test.go
git commit -m "feat: 添加 API token 存储"
```

### Task 3：实现 app token service、本地 token CLI 与 audit

**Files:**
- Create: `internal/app/token.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/audit.go`
- Modify: `internal/cli/root.go`
- Create: `internal/cli/token.go`
- Test: `internal/app/token_test.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1：写失败的 app/CLI 测试**

App 测试：

```go
func TestCreateTokenStoresHashAndAudits(t *testing.T) {
    svc := newTestServiceAsOwner(t)
    out, err := svc.CreateToken(app.CreateTokenInput{
        Name: "cli",
        Type: "pat",
        Scopes: []string{"task:read", "task:write"},
        WorkspaceRefs: []string{"local"},
        ExpiresIn: ptrDuration(720 * time.Hour),
    })
    if err != nil { t.Fatal(err) }
    if !strings.HasPrefix(out.RawToken, "taskg_pat_") { t.Fatalf("token = %q", out.RawToken) }
    if strings.Contains(out.Stored.TokenHash, out.RawToken) { t.Fatalf("raw token stored") }
    audits := mustListAudit(t, svc)
    assertAuditAction(t, audits, "token.create")
}

func TestCreateAgentTokenRequiresExplicitWorkspaceAndScope(t *testing.T) {
    svc := newTestServiceAsOwner(t)
    _, err := svc.CreateToken(app.CreateTokenInput{Name: "agent", Type: "agent", Scopes: []string{"task:read"}})
    assertRuntimeCode(t, err, "token_workspace_scope_invalid")
}
```

Integration 测试：

```go
func TestCLITokenCreateListRevoke(t *testing.T) {
    bin := buildTaskg(t)
    db := filepath.Join(t.TempDir(), "taskg.db")
    out := run(t, bin, "--db", db, "--json", "token", "create", "cli", "--scope", "task:read", "--workspace", "local", "--expires-in", "720h")
    var created map[string]any
    if err := json.Unmarshal([]byte(out), &created); err != nil { t.Fatal(err) }
    if created["token"] == "" { t.Fatalf("missing raw token: %s", out) }
    list := run(t, bin, "--db", db, "token", "list")
    if !strings.Contains(list, "cli") || strings.Contains(list, created["token"].(string)) {
        t.Fatalf("token list leaked raw token: %q", list)
    }
    run(t, bin, "--db", db, "token", "revoke", created["id"].(string))
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/app -run TestCreateToken -count=1
go test ./tests/integration -run TestCLITokenCreateListRevoke -count=1
```

Expected: FAIL，app/CLI token 功能不存在。

- [ ] **Step 3：实现 app token service**

实现：

- `CreateTokenInput`、`CreatedToken`、`TokenView`。
- token name 必填：`token_name_required`。
- `--user` 省略时归属当前 actor；指定其它 user 时当前 actor 必须 admin/owner。
- workspace refs 解析为 IDs；Agent token 必须至少一个 workspace。
- project refs/id 必须属于允许 workspace，存 `project_ids_json`。如果 `workspace_ids_json` 非空，所有 `project_ids` 必须属于这些 workspace 中至少一个；否则返回 `token_project_scope_invalid`。
- scopes 必须显式合法；空 scopes 对 PAT 允许创建但无 capability，Agent token 不允许空 scopes。
- `expires_at = nil` 表示永不过期。
- `AuthenticateBearerToken(raw string)` 不缓存，每次查 DB，过期/revoked 返回 `auth_token_expired` / `auth_token_revoked`。
- `token.create` / `token.revoke` audit payload 不包含 raw token/hash。

- [ ] **Step 4：实现 `taskg token` CLI**

CLI 规则：

- `token create <name> --scope <scope> [--scope ...] [--user <user>] [--workspace <slug|id>] [--workspace-id <id>] [--project <slug>] [--project-id <id>] [--type pat|agent] [--expires-in 720h]`
- 默认 type 为 `pat`。
- `--scope` 使用 Cobra `StringSliceVar`，支持重复和逗号分隔。
- human 输出只显示 raw token 一次，并提示只显示一次。
- `token list` human columns 固定为 `ID PREFIX NAME TYPE WORKSPACES SCOPES EXPIRES_AT LAST_USED_AT`。
- `token list --json` 输出 token view 字段：`id`、`prefix`、`name`、`type`、`user_id`、`workspace_ids`、`project_ids`、`scopes`、`created_at`、`expires_at`、`revoked_at`、`last_used_at`，但不包含 `token` raw 值和 `token_hash`。`expires_at` 使用 RFC3339 字符串，`null` 表示永不过期。
- `token revoke <id|prefix>`。

- [ ] **Step 5：运行测试通过**

Run:

```bash
go test ./internal/app -run 'TestCreateToken|TestAuthenticateBearerToken' -count=1
go test ./tests/integration -run TestCLITokenCreateListRevoke -count=1
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal/app/token.go internal/app/service.go internal/app/audit.go internal/cli/root.go internal/cli/token.go internal/app/token_test.go tests/integration/cli_test.go
git commit -m "feat: 添加 token 管理命令"
```

---

## Chunk 2：HTTP Server Skeleton、Envelope 与 Auth Middleware

### Task 4：新增 HTTP router、healthz、server 命令

**Files:**
- Modify: `go.mod`
- Create: `internal/httpapi/server.go`
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/envelope.go`
- Create: `internal/httpapi/middleware.go`
- Create: `internal/httpapi/server_test.go`
- Create: `internal/cli/server.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`
- Create/Modify: `docs/openapi/taskg-v1.yaml`

- [ ] **Step 1：写失败的 HTTP skeleton 测试**

新增：

```go
func TestHealthzIsAnonymous(t *testing.T) {
    srv := httpapi.NewServer(httpapi.Options{Store: openTestStore(t), Clock: fakeClock{Now: 100}})
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    srv.Router().ServeHTTP(rr, req)
    if rr.Code != http.StatusOK { t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String()) }
    if strings.Contains(rr.Body.String(), "schema") || strings.Contains(rr.Body.String(), "database") {
        t.Fatalf("healthz leaked details: %s", rr.Body.String())
    }
}

func TestErrorEnvelopeForUnknownRoute(t *testing.T) {
    srv := httpapi.NewServer(httpapi.Options{Store: openTestStore(t)})
    rr := httptest.NewRecorder()
    srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
    assertErrorCode(t, rr, http.StatusNotFound, "route_not_found")
    rr = httptest.NewRecorder()
    srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/healthz", nil))
    assertErrorCode(t, rr, http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestPanicIsRecoveredAsErrorEnvelope(t *testing.T) {
    srv := httpapi.NewServer(httpapi.Options{Store: openTestStore(t), TestPanicRoute: true})
    rr := httptest.NewRecorder()
    srv.Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/__panic", nil))
    assertErrorCode(t, rr, http.StatusInternalServerError, "api_internal")
}
```

Integration：

```go
func TestCLIServerHealthz(t *testing.T) {
    bin := buildTaskg(t)
    db := filepath.Join(t.TempDir(), "taskg.db")
    cmd, baseURL := startTaskgServer(t, bin, "--db", db)
    defer cmd.Process.Kill()
    resp, err := http.Get(baseURL + "/healthz")
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("status = %d", resp.StatusCode) }
}

func TestServerEnforcesForeignKeys(t *testing.T) {
    bin := buildTaskg(t)
    db := filepath.Join(t.TempDir(), "taskg.db")
    server := startTaskgServerWithDB(t, bin, db)
    token := createAdminToken(t, bin, db)
    // 通过 API 构造跨 workspace project_id 写入，必须返回明确错误，不能静默写入。
    rr := postJSON(t, server.URL+"/api/v1/tasks?workspace=local", bearer(token), `{"description":"bad","project_id":"project-from-other-workspace"}`)
    assertHTTPErrorCode(t, rr, http.StatusBadRequest, "project_workspace_mismatch")
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/httpapi -run TestHealthzIsAnonymous -count=1
go test ./tests/integration -run TestCLIServerHealthz -count=1
```

Expected: FAIL，包/命令不存在。

- [ ] **Step 3：添加 chi 与 HTTP skeleton**

实现：

- `internal/httpapi.NewServer(Options)` 返回含 router 的 server struct。
- middleware 顺序固定：RequestID -> Recoverer -> AccessLog -> BodyLimit -> Auth（只在 `/api/v1/*` 启用）。
- Recoverer 把 panic 转换为 500 `api_internal` error envelope，并把 stack trace 写 stderr。
- `GET /healthz` 匿名返回 `{"ok":true}`。
- `/api/v1` group 预留 auth middleware。
- success envelope：`{"data":..., "meta":{...}}`。
- error envelope：`{"error":{"code":"...","message":"...","details":{...}}}`，`details` 为开放对象。
- access log 默认写 stderr：method/path/status/actor_user_id/token_id/duration；鉴权失败和 `/healthz` 的 actor/token 写 `-`。
- body limit middleware 默认 10 MB，超出返回 413 `api_payload_too_large`。

- [ ] **Step 4：实现 `taskg server`**

实现：

- `taskg server --listen :8080`，默认 listen 可以为空则报 `server_listen_required`。
- 启动时立即 `sqlite.Open(path)`，触发 schema 迁移。迁移失败时写 stderr 并 exit 1，不能等第一个请求才失败。
- 使用 `openStore(opts)` 打开服务端 DB。
- SIGINT/SIGTERM graceful shutdown，默认 30 秒超时；增加 `--shutdown-timeout 30s` flag。超时后进程退出 1。
- 启动日志写 stderr，不污染 stdout。
- 不输出 bootstrap token。
- M6 server 只支持 HTTP，不内置 TLS。README 必须说明生产部署用反向代理做 TLS termination，不要把裸 HTTP token 服务直接暴露公网。

- [ ] **Step 5：同步 OpenAPI skeleton**

创建 `docs/openapi/taskg-v1.yaml`，至少包含：

- OpenAPI 3 header。
- Bearer auth security scheme。
- `/healthz`。
- Error envelope schema。

- [ ] **Step 6：运行测试**

Run:

```bash
go test ./internal/httpapi -run 'TestHealthz|TestErrorEnvelope' -count=1
go test ./tests/integration -run TestCLIServerHealthz -count=1
CGO_ENABLED=0 go test ./internal/httpapi -count=1
go vet ./internal/httpapi ./internal/cli
```

Expected: PASS。

- [ ] **Step 7：提交**

```bash
git add go.mod go.sum internal/httpapi internal/cli/server.go internal/cli/root.go tests/integration/cli_test.go docs/openapi/taskg-v1.yaml
git commit -m "feat: 添加 HTTP 服务端骨架"
```

### Task 5：实现 auth middleware 与 `/api/v1/me`

**Files:**
- Modify: `internal/httpapi/middleware.go`
- Create: `internal/httpapi/me.go`
- Create/Modify: `internal/httpapi/auth_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [ ] **Step 1：写失败的 auth middleware 测试**

覆盖：

```go
func TestAuthRequiresBearerHeader(t *testing.T) {
    srv := newHTTPServerWithTokenFixture(t)
    rr := request(t, srv, "GET", "/api/v1/me", nil, nil)
    assertErrorCode(t, rr, 401, "auth_missing_token")
    rr = request(t, srv, "GET", "/api/v1/me?token=bad", nil, nil)
    assertErrorCode(t, rr, 401, "auth_missing_token")
    rr = requestWithCookie(t, srv, "GET", "/api/v1/me", "token=bad")
    assertErrorCode(t, rr, 401, "auth_missing_token")
    rr = request(t, srv, "GET", "/api/v1/me", nil, map[string]string{"Authorization": "Basic abc"})
    assertErrorCode(t, rr, 401, "auth_missing_token")
    rr = request(t, srv, "GET", "/api/v1/me", nil, map[string]string{"Authorization": "bearer " + validToken(t, srv)})
    assertStatus(t, rr, 200)
}

func TestMeReturnsActorTokenAndWorkspace(t *testing.T) {
    srv, token := newHTTPServerWithTokenFixture(t, "task:read")
    rr := request(t, srv, "GET", "/api/v1/me", nil, bearer(token))
    assertStatus(t, rr, 200)
    assertJSONPath(t, rr.Body.Bytes(), "data.actor.name", "local")
    assertJSONPath(t, rr.Body.Bytes(), "data.token.type", "pat")
    assertJSONPath(t, rr.Body.Bytes(), "data.effective_workspace.slug", "local")
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/httpapi -run 'TestAuth|TestMe' -count=1
```

Expected: FAIL。

- [ ] **Step 3：实现 auth middleware**

实现：

- 仅接受 `Authorization: Bearer <token>`。
- Bearer scheme 大小写不敏感；`bearer <token>` 可接受。Cookie、query string、Basic auth 都按缺少 bearer token 返回 `auth_missing_token`。
- invalid / expired / revoked 分别映射 401 code。
- 鉴权失败不写 audit。
- 成功后把 `AuthenticatedToken` 放入 request context。
- `last_used_at` 可 best-effort 更新，失败不影响主请求。

- [ ] **Step 4：实现 `/api/v1/me`**

返回 spec 最小结构：

```json
{
  "data": {
    "actor": {"id": "...", "name": "..."},
    "token": {"id": "...", "name": "...", "type": "pat", "scopes": ["task:read"]},
    "visible_workspaces": [{"id": "...", "slug": "local"}],
    "effective_workspace": {"id": "...", "slug": "local"}
  }
}
```

测试 fixture `newHTTPServerWithTokenFixture` 默认创建 `local` user、`local` workspace、owner membership，并创建归属 `local` 的 PAT；断言默认 actor/workspace 时只能依赖这个 fixture，不要把它推广成任意 fresh DB 的 API 语义。

- [ ] **Step 5：更新 OpenAPI**

补 `/api/v1/me`、auth security、401 responses。

- [ ] **Step 6：运行测试**

Run:

```bash
go test ./internal/httpapi -run 'TestAuth|TestMe' -count=1
```

Expected: PASS。

- [ ] **Step 7：提交**

```bash
git add internal/httpapi docs/openapi/taskg-v1.yaml
git commit -m "feat: 添加 HTTP token 鉴权"
```

---

## Chunk 3：Request Scope 与 Token Authorization Pipeline

### Task 6：实现 request scope 解析和 app service 构造

**Files:**
- Create: `internal/app/request_scope.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/permission.go`
- Create: `internal/app/request_scope_test.go`
- Modify: `internal/httpapi/scope.go`
- Create/Modify: `internal/httpapi/scope_test.go`

- [ ] **Step 1：写失败的 request scope 测试**

App 测试：

```go
func TestRequestScopeAuthorizationOrder(t *testing.T) {
    fixture := newScopeFixture(t)
    _, err := fixture.Authorize(app.RequestScopeInput{RequiredCapability: "task:write", WorkspaceRef: "local"}, tokenWithScopes("task:read"))
    assertRuntimeCode(t, err, "token_scope_denied")
    _, err = fixture.Authorize(app.RequestScopeInput{RequiredCapability: "task:read", WorkspaceRef: "other"}, tokenWithWorkspaceScope("local", "task:read"))
    assertRuntimeCode(t, err, "workspace_scope_denied")
}

func TestProjectScopedTokenHidesUnscopedTasks(t *testing.T) {
    svc := newServiceWithProjectScopedToken(t, "project-a")
    tasks, err := svc.List(app.ListInput{Query: mustParse("project:")})
    if err != nil { t.Fatal(err) }
    if len(tasks) != 0 { t.Fatalf("project-scoped token saw no-project tasks") }
}
```

HTTP scope 测试：

```go
func TestWorkspaceFallbackDoesNotBypassTokenScope(t *testing.T) {
    srv, token := serverWithTokenScopedTo(t, "dajee")
    rr := request(t, srv, "GET", "/api/v1/tasks?workspace=partner", nil, bearer(token))
    assertErrorCode(t, rr, 403, "workspace_scope_denied")
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/app -run 'TestRequestScope|TestProjectScopedToken' -count=1
go test ./internal/httpapi -run TestWorkspaceFallbackDoesNotBypassTokenScope -count=1
```

Expected: FAIL。

- [ ] **Step 3：实现 app request scope**

实现：

- `RequestScope` 包含 actor、workspace、role、token ID/type/scopes、allowed workspace/project IDs。
- `NewServiceForRuntime(opts)` 或等价构造器，避免 HTTP 请求读取本地 active workspace。
- 6 步授权流水线：
  1. token valid。
  2. capability。
  3. workspace allowlist。
  4. project allowlist。
  5. membership role。
  6. app service。
- 第 4 步 project allowlist 按 endpoint 类型处理：
  - explicit project endpoint（task detail/mutation、report with project、project、audit project filter）必须校验 explicit project 在 allowlist 内；越界按 spec 返回 403 或单任务读取 404。
  - non-project endpoint（me/workspace/member/context/config/token）第 4 步是 noop，不因 token project allowlist 收窄返回内容。
  - list endpoint（task list、report、audit list）即使请求没有 explicit project，也必须默认叠加 `project_id IN allowlist` 过滤。
- 第 1-5 步失败不写 audit。
- scope 空数组语义：capability fail-closed，workspace/project allowlist limit-open。
- `ApplyProjectAllowlist(ListInput)` 或 repo/app 层等价 helper。

- [ ] **Step 4：实现 HTTP scope parser**

实现：

- workspace 来源优先级固定为 path > body > query > header (`X-Taskg-Workspace`) > token 单 workspace > actor default workspace > error。
- 如果多个来源同时存在但解析到不同 workspace，返回 400 `workspace_mismatch`。
- `{workspace}` 接受 slug/UUID；与 body/query `workspace_id` 同时存在时必须一致。
- `{project}` 接受 slug/project_id；与 body/query `project_id` 同时存在但解析不同，返回 400 `project_mismatch`。
- project_id 不能绕过 token workspace scope。

- [ ] **Step 5：运行测试**

Run:

```bash
go test ./internal/app -run 'TestRequestScope|TestProjectScopedToken' -count=1
go test ./internal/httpapi -run 'TestWorkspaceFallback|TestProjectScope' -count=1
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal/app/request_scope.go internal/app/runtime.go internal/app/service.go internal/app/permission.go internal/app/request_scope_test.go internal/httpapi/scope.go internal/httpapi/scope_test.go
git commit -m "feat: 添加请求级权限边界"
```

---

## Chunk 4：HTTP API Endpoints

### Task 7：实现 project、project config、workspace、member API

**Files:**
- Create: `internal/httpapi/project.go`
- Create: `internal/httpapi/workspace.go`
- Create: `internal/httpapi/member.go`
- Test: `internal/httpapi/project_test.go`
- Test: `internal/httpapi/workspace_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [ ] **Step 1：写失败的 endpoint 测试**

覆盖：

- `GET /api/v1/projects?workspace=local` 需要 `project:read`。
- `POST /api/v1/projects` 需要 `project:write` + admin/owner。
- `POST /api/v1/workspaces` 需要 `workspace:write`；缺 scope 返回 403 `token_scope_denied`。
- `{project}` slug/id 解析和 `project_id` 一致性。
- `GET/PUT/DELETE /api/v1/projects/{project}/config/{key}` 不混淆 workspace config。
- member list/manage 需要 `workspace:read` / `workspace:write`。
- `POST /api/v1/projects?workspace=dajee` 写入的 audit 必须使用 request workspace，并填充新建 project id。

示例：

```go
func TestProjectEndpointRequiresProjectRead(t *testing.T) {
    srv, token := serverWithToken(t, "task:read")
    rr := request(t, srv, "GET", "/api/v1/projects?workspace=local", nil, bearer(token))
    assertErrorCode(t, rr, 403, "token_scope_denied")
}

func TestCreateWorkspaceRequiresWorkspaceWrite(t *testing.T) {
    srv, token := serverWithToken(t, "task:read,task:write")
    rr := request(t, srv, "POST", "/api/v1/workspaces", `{"slug":"new","name":"New"}`, bearer(token))
    assertErrorCode(t, rr, 403, "token_scope_denied")
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/httpapi -run 'TestProject|TestWorkspace|TestMember' -count=1
```

Expected: FAIL。

- [ ] **Step 3：实现 handler**

实现 spec endpoint：

- `GET/POST/PATCH /api/v1/workspaces...`
- `GET/POST/PATCH /api/v1/workspaces/{workspace}/members...`
- `GET/POST/PATCH/POST archive /api/v1/projects...`
- project config endpoint。

注意：

- 不实现 workspace archive。
- 所有 handler 只调用 app service。
- JSON DTO 使用 snake_case。
- path parameter 在 handler 内部校验格式；格式错误返回 400，不让错误穿透到 storage 层。
- 可以按 workspace/member、project、project config 子资源分别做小提交；如果已经按子资源提交，Task 7 末尾只跑测试和确认状态，不再做汇总提交。如果一次完成整个 Task 7，则使用 Step 6 的汇总提交。

- [ ] **Step 4：同步 OpenAPI**

补 workspace/member/project/project config schemas 和 responses。

- [ ] **Step 5：运行测试**

Run:

```bash
go test ./internal/httpapi -run 'TestProject|TestWorkspace|TestMember' -count=1
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal/httpapi/project.go internal/httpapi/workspace.go internal/httpapi/member.go internal/httpapi/*_test.go docs/openapi/taskg-v1.yaml
git commit -m "feat: 添加 project 与 workspace API"
```

### Task 8：实现 task、report、urgency API

**Files:**
- Create: `internal/httpapi/task.go`
- Create: `internal/httpapi/report.go`
- Test: `internal/httpapi/task_test.go`
- Test: `internal/httpapi/report_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [ ] **Step 1：写失败的 task/report 测试**

覆盖：

- `GET /api/v1/tasks?query=%2Bnext&report=list` URL 编码。
- `POST /api/v1/tasks` 写入 task audit。
- `GET /api/v1/tasks/{uuid}` project allowlist 越界返回 404 `task_not_found`。
- `GET /api/v1/tasks/not-a-uuid` 返回 400 `task_uuid_invalid`。
- `PATCH /api/v1/tasks/{uuid}` project_id/workspace 不一致返回 400。
- `POST /done`、`/start`、`/stop`。
- annotations add/delete。
- `GET /api/v1/reports/{name}`。
- `GET /api/v1/tasks/{uuid}/urgency`。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/httpapi -run 'TestTask|TestReport|TestUrgency' -count=1
```

Expected: FAIL。

- [ ] **Step 3：实现 handler 和 DTO**

实现：

- `GET /api/v1/tasks`：query/report/limit/project scope。
- `POST /api/v1/tasks`：description、project/project_id、priority、due、tags、UDA。
- `GET/PATCH/DELETE /api/v1/tasks/{uuid}`。
- action endpoint：done/start/stop/annotations。
- report endpoint。
- urgency endpoint。

注意：

- `{uuid}` 只接受真实 UUID。
- `{uuid}` 格式错误在 handler 内部返回 400 `task_uuid_invalid`，不交给 storage 层解析。
- 单任务 project 越界固定 404。
- list meta 使用 `returned`，不是 `count`。
- 可以按 task CRUD/action、report/urgency 子资源分别做小提交；如果已经按子资源提交，Task 8 末尾只跑测试和确认状态，不再做汇总提交。如果一次完成整个 Task 8，则使用 Step 6 的汇总提交。

- [ ] **Step 4：同步 OpenAPI**

补 task/report/urgency schemas 和 encoded query example：`%2Bnext`。

- [ ] **Step 5：运行测试**

Run:

```bash
go test ./internal/httpapi -run 'TestTask|TestReport|TestUrgency' -count=1
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal/httpapi/task.go internal/httpapi/report.go internal/httpapi/*_test.go docs/openapi/taskg-v1.yaml
git commit -m "feat: 添加 task 与 report API"
```

### Task 9：实现 context、config、import/export、audit、token API

**Files:**
- Create: `internal/httpapi/context.go`
- Create: `internal/httpapi/config.go`
- Create: `internal/httpapi/import_export.go`
- Create: `internal/httpapi/audit.go`
- Create: `internal/httpapi/token.go`
- Test: `internal/httpapi/context_config_test.go`
- Test: `internal/httpapi/import_audit_token_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [ ] **Step 1：写失败测试**

覆盖：

- `contexts/none` 清除 effective workspace 内 actor active context。
- config endpoint 只处理 workspace config，不处理 `remote.server` / `remote.token`。
- import JSON body 10 MB 限制，超出 413 `api_payload_too_large`。
- import 整批回滚。
- import 只写一条 `task.import` audit。
- audit endpoint 需要 `audit:read` + admin/owner。
- token endpoint 需要 `token:read` / `token:write`。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/httpapi -run 'TestContext|TestConfig|TestImport|TestAudit|TestToken' -count=1
```

Expected: FAIL。

- [ ] **Step 3：实现 handler**

实现 spec endpoint：

- `GET/POST/DELETE/POST use/none /api/v1/contexts...`
- `GET/PUT/DELETE /api/v1/config...`
- `GET /api/v1/export`
- `POST /api/v1/import`
- `GET /api/v1/audit`
- `GET/POST/DELETE /api/v1/tokens`

注意：

- config endpoint 不读写本机 TOML。
- project config 仍只走 project config endpoint。
- audit list 叠加 token workspace/project scope。
- audit endpoint M6 只支持 `workspace`、`project`、`limit` query parameter；`actor`、`action`、`since` 等过滤不要在 M6 实现。
- token endpoint 不返回 raw token，除 create response。
- 可以按 context/config、import/export、audit/token 子资源分别做小提交；如果已经按子资源提交，Task 9 末尾只跑测试和确认状态，不再做汇总提交。如果一次完成整个 Task 9，则使用 Step 6 的汇总提交。

- [ ] **Step 4：同步 OpenAPI**

补 context/config/import/export/audit/token schemas。

- [ ] **Step 5：运行测试**

Run:

```bash
go test ./internal/httpapi -run 'TestContext|TestConfig|TestImport|TestAudit|TestToken' -count=1
go test ./internal/httpapi -count=1
go vet ./internal/httpapi
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal/httpapi docs/openapi/taskg-v1.yaml
git commit -m "feat: 补齐 M6 HTTP API"
```

---

## Chunk 5：Remote CLI Client 与核心命令远程化

### Task 10：新增 remote client 与 config/env 解析

**Files:**
- Create: `internal/remote/client.go`
- Create: `internal/remote/client_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/toml.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1：写失败测试**

覆盖：

- `TASKG_SERVER` / `TASKG_TOKEN`。
- TOML `[remote] server/token`。
- CLI flag > env > TOML。
- remote server URL 校验：
  - `foo.com` 返回 `remote_server_invalid`。
  - `https://foo.com:abc` 返回 `remote_server_invalid`。
  - `http://foo.com/api/v1` 可接受，client 拼接 endpoint 时不能重复 `/api/v1`。
- TOML 含 `remote.token` 且权限宽于 `0600` 时 stderr warning。
- remote client 只发 Authorization Bearer header。
- error envelope 解码为 CLI error。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/config -run TestRemoteConfig -count=1
go test ./internal/remote -run TestClient -count=1
go test ./internal/cli -run TestRemoteFlags -count=1
```

Expected: FAIL。

- [ ] **Step 3：实现 remote config 和 client**

实现：

- `config.Config` 增加 `RemoteServer`、`RemoteToken`。
- env 读取 `TASKG_SERVER`、`TASKG_TOKEN`。
- TOML 读取 `remote.server`、`remote.token`。
- CLI `Options` 增加 `Server`、`Token`、`Project`、`ProjectID`。
- `splitFlagsRcAndPositional` string flags 增加 `--server`、`--token`、`--project`、`--project-id`。
- `remote.Client` 包装 GET/POST/PATCH/DELETE JSON。
- remote server URL 必须包含 `http` 或 `https` scheme；无 scheme 不自动补 `http`。

- [ ] **Step 4：运行测试**

Run:

```bash
go test ./internal/config -run TestRemoteConfig -count=1
go test ./internal/remote -run TestClient -count=1
go test ./internal/cli -run TestRemoteFlags -count=1
```

Expected: PASS。

- [ ] **Step 5：提交**

```bash
git add internal/remote internal/config internal/cli/root.go
git commit -m "feat: 添加远程 CLI 客户端配置"
```

### Task 11：远程化 task/report/project/context/config/helper 命令

**Files:**
- Create: `internal/remote/task.go`
- Create: `internal/remote/project.go`
- Create: `internal/remote/config.go`
- Modify: `internal/cli/add.go`
- Modify: `internal/cli/list.go`
- Modify: `internal/cli/info.go`
- Modify: `internal/cli/report.go`
- Modify: `internal/cli/activity.go`
- Modify: `internal/cli/annotation.go`
- Modify: `internal/cli/project.go`
- Modify: `internal/cli/context.go`
- Modify: `internal/cli/config.go`
- Modify: `internal/cli/helper.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1：写失败的远程 CLI 集成测试**

覆盖：

```go
func TestRemoteCLIAddListDone(t *testing.T) {
    bin := buildTaskg(t)
    db := filepath.Join(t.TempDir(), "taskg.db")
    run(t, bin, "--db", db, "token", "create", "cli", "--scope", "task:read,task:write,project:read", "--workspace", "local", "--expires-in", "720h")
    server := startTaskgServerWithDB(t, bin, db)
    token := extractToken(...)
    run(t, bin, "--server", server.URL, "--token", token, "project", "add", "api", "name:API")
    run(t, bin, "--server", server.URL, "--token", token, "add", "remote", "task", "project:api")
    out := run(t, bin, "--server", server.URL, "--token", token, "list")
    if !strings.Contains(out, "remote task") { t.Fatalf("list = %q", out) }
    run(t, bin, "--server", server.URL, "--token", token, "1", "done")
}
```

还要覆盖：

- 远程 `project config get/set/list/unset`。
- 远程 `context define/use/none`。
- 远程 `_ids/_uuids/_projects/_unique` 通过已有 endpoint 本地后处理。
- 远程 `_show database.path` 返回 `remote_unsupported_command`。
- 远程 `edit` 返回 `remote_unsupported_command`。
- `remote_unsupported_command` 写 stderr，exit code 非 0，stdout 不输出错误文本。
- 远程 `info <uuid>` project 越界返回 404 `task_not_found`。
- 远程 `info 1` 使用两跳解析；远程 `info <uuid>` 直接调用 task detail endpoint。
- 模拟两跳之间任务被另一个 client 完成或删除，远程 CLI 不重试、不重新解析，直接透传 mutation 返回的实际错误。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./tests/integration -run 'TestRemoteCLI' -count=1
```

Expected: FAIL。

- [ ] **Step 3：实现 remote 分支**

实现原则：

- 每个命令开头检查 `isRemoteMode(currentOpts)`。
- 抽出 `internal/cli/dispatch.go` 中的 `dispatchRemoteOrLocal` / `isRemoteMode` helper，避免每个命令重复分支模板。
- remote mode 调用 `internal/remote`，local mode 原逻辑不变。
- remote render 复用现有 `render.TaskListWithIDs`、`render.JSON`。
- 数字 working-set ID 两跳：先远程 list/report 拿 numbered tasks，再转换 UUID 发 mutation。
- 两跳之间发生并发变化时不重试、不重新解析，直接把 mutation 错误展示给用户。
- 远程 add/modify 的 Taskwarrior 风格参数解析（description、project、tags、UDA 等）仍在客户端复用现有 CLI add/modify 解析 helper，POST 到服务端的是结构化 JSON；`POST /api/v1/tasks` body 不接受 raw Taskwarrior CLI 字符串。
- 远程 target 识别沿用本地规则，但不得把 UUID prefix 直接塞进 HTTP path：纯数字走 working-set 两跳；完整 UUID 直接走 detail/mutation endpoint；UUID prefix 或其它本地支持的非数字 target 先由客户端唯一解析为完整 UUID，再调用 endpoint。
- 不新增 helper API；helper 用 task/project/config/me endpoint 本地后处理。
- 远程 `_unique` 等 helper 在大 workspace 上可能较慢，M6 接受这个权衡并在 README 说明；后续如需要再设计 aggregation endpoint。
- `calc`、`completion` 保持本地；`edit`、`.taskrc import` remote mode 返回 unsupported。

- [ ] **Step 4：运行远程 CLI 测试**

Run:

```bash
go test ./tests/integration -run 'TestRemoteCLI' -count=1
```

Expected: PASS。

- [ ] **Step 5：提交**

```bash
git add internal/remote internal/cli tests/integration/cli_test.go
git commit -m "feat: 支持远程 CLI 核心命令"
```

---

## Chunk 6：Project-scoped Token 安全闭环

### Task 12：project allowlist 安全闭环回归测试

**Files:**
- Modify: `internal/app/request_scope_test.go`
- Modify: `internal/httpapi/task_test.go`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1：写安全闭环回归测试**

覆盖：

- project-scoped token 在不指定 project 的 list/report 请求中，返回结果自动过滤为 allowlist 内 project 的任务。
- `query=project:` 在 project-scoped token 下合法但返回空。
- `GET /api/v1/tasks/{uuid}` 任务存在但 project 不在 allowlist 时 404 `task_not_found`。
- `PATCH /api/v1/tasks/{uuid}` 越界时 403 `project_scope_denied`。
- `POST /api/v1/import` 尝试导入 allowlist 外 project 失败且整批回滚。
- audit list 不能读 allowlist 外 project。
- 远程 CLI `--workspace` 和 `--project-id` 不能突破 token scope。
- token revoke 后立即生效：旧 token revoke 前 `/api/v1/me` 成功，revoke 后下一次 `/api/v1/me` 必须返回 401 `auth_token_revoked`。

- [ ] **Step 2：运行测试确认是否存在缺口**

Run:

```bash
go test ./internal/app -run ProjectScoped -count=1
go test ./internal/httpapi -run ProjectScoped -count=1
go test ./tests/integration -run 'TestRemoteCLIProjectScope|TestProjectScopedToken' -count=1
```

Expected: 如果前面 task 已完整实现，这组回归测试可以直接 PASS；如果 FAIL，失败必须定位到 Chunk 3-5 对应 task 的遗漏。

- [ ] **Step 3：按归属 task 修复遗漏**

Task 12 是安全闭环兜底，不应成为新增业务逻辑的主要实现点。如果 Step 2 有失败，回到对应归属 task 修复：

- app list/report/export/import 统一叠加 project allowlist。
- detail read 越界转 404。
- mutation 越界转 403。
- audit list 叠加 project allowlist。
- 远程 CLI 不把 project slug 当全局唯一。

- [ ] **Step 4：运行安全测试和全 HTTP 测试**

Run:

```bash
go test ./internal/app -run ProjectScoped -count=1
go test ./internal/httpapi -run ProjectScoped -count=1
go test ./internal/httpapi -count=1
go test ./tests/integration -run 'TestRemoteCLIProjectScope|TestProjectScopedToken' -count=1
```

Expected: PASS。

- [ ] **Step 5：提交**

```bash
git add internal/app internal/httpapi internal/remote tests/integration/cli_test.go
git commit -m "fix: 收紧 project scoped token 边界"
```

---

## Chunk 7：OpenAPI、文档同步与总体验证

### Task 13：补齐 OpenAPI 与用户文档

**Files:**
- Modify: `docs/openapi/taskg-v1.yaml`
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/requirements.md`

- [ ] **Step 1：审查 OpenAPI 覆盖面**

检查 `docs/openapi/taskg-v1.yaml` 是否覆盖：

- bearer auth。
- error envelope，`details.additionalProperties: true`。
- `/healthz` 匿名。
- `/api/v1/me`。
- workspace/member/project/project config。
- task/report/urgency。
- context/config/import/export/audit/token。
- 413 `api_payload_too_large`。
- 404 `task_not_found`。
- spec §11 列出的所有 M6 错误 code，并确保每个 code 至少有对应 endpoint response 或 shared error schema 示例。

- [ ] **Step 2：同步 README**

新增：

- `taskg server --listen :8080`。
- `taskg token create/list/revoke`。
- 远程 CLI `--server` / `--token` / `TASKG_SERVER` / `TASKG_TOKEN`。
- project-scoped token 示例。
- TOML `remote.server` / `remote.token` 安全提醒。
- M6 server 不内置 TLS；生产部署应使用 Nginx/Caddy 等反向代理做 TLS termination，不要把裸 HTTP token 服务直接暴露公网。
- 远程 `_unique` 等 helper 在大 workspace 上可能较慢，M6 不提供 aggregation endpoint。
- `edit`、`.taskrc import` 等远程暂不支持说明。

- [ ] **Step 3：同步 ROADMAP**

更新：

- M6 状态为已完成。
- M7 收紧为 MCP Server 与 tool schema，不再包含 token/API 基础。
- 当前下一步指向 M7。

- [ ] **Step 4：同步 requirements**

更新：

- M6 实际 endpoint。
- token scope 表。
- 远程 CLI 范围。
- `project_id` scope 和 `task_not_found` 安全语义。

- [ ] **Step 5：文档 diff 检查**

Run:

```bash
git diff --check
rg -n "\\| `admin:\\*` \\||report:read|meta\\.count|\"count\"\\s*:" docs/superpowers/specs/2026-05-31-taskg-m6-design.md docs/openapi/taskg-v1.yaml README.md ROADMAP.md docs/requirements.md
rg -n "auth_missing_token|auth_invalid_token|auth_token_expired|auth_token_revoked|taskg_pat_|taskg_agent_|task_not_found|workspace_scope_denied|project_scope_denied|route_not_found|method_not_allowed|api_internal|task_uuid_invalid|remote_server_invalid" docs/openapi/taskg-v1.yaml README.md ROADMAP.md docs/requirements.md
```

Expected: `git diff --check` 无输出；第一条 `rg` 不应发现旧的 M6 设计残留；第二条 `rg` 应能在 OpenAPI 或用户文档中命中对应错误码和 token 前缀说明。

- [ ] **Step 6：提交**

```bash
git add docs/openapi/taskg-v1.yaml README.md ROADMAP.md docs/requirements.md
git commit -m "docs: 更新 M6 服务端与远程 CLI 文档"
```

### Task 14：最终验证

**Files:**
- No source changes unless verification reveals issues.

- [ ] **Step 1：检查格式**

Run:

```bash
test -z "$(gofmt -l internal cmd tests)"
```

Expected: PASS，无输出。

- [ ] **Step 2：运行 go vet**

Run:

```bash
go vet ./...
```

Expected: PASS。

- [ ] **Step 3：运行单元测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 4：运行 race 测试**

Run:

```bash
go test -race ./internal/httpapi ./internal/auth ./internal/remote ./internal/app
```

Expected: PASS。

- [ ] **Step 5：运行 CGO-free 测试**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [ ] **Step 6：运行 CGO-free build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS。

- [ ] **Step 7：检查 git diff**

Run:

```bash
git status --short
git diff --check
```

Expected: 只包含 M6 相关文件；`git diff --check` 无输出。

- [ ] **Step 8：提交最终修复（如有）**

如果最终验证暴露小修复：

```bash
git add <fixed-files>
git commit -m "fix: 完成 M6 验证修复"
```

如果没有修复，不需要空提交。

## 执行完成标准

M6 只有在以下条件全部满足时才能标记完成：

- `taskg server` 可启动、graceful shutdown、`/healthz` 匿名可用。
- 本地 CLI 可创建第一个 token；HTTP 可用已有 token 管理后续 token。
- HTTP API 全部使用 Authorization Bearer header，拒绝 query/cookie/basic auth。
- API error envelope 稳定，spec §11 列出的所有 M6 错误 code 都在对应 endpoint 有 OpenAPI 描述和测试覆盖。
- project-scoped token 对 list/detail/mutation/import/export/audit 全部生效。
- 远程 CLI 核心命令输出与本地 CLI 尽量一致，stdout/stderr 分离。
- `_show database.path` 远程模式不泄露服务端路径。
- OpenAPI 与已实现 endpoint 一致。
- README、ROADMAP、requirements 已同步。
- `test -z "$(gofmt -l internal cmd tests)"`、`go vet ./...`、`go test ./...`、`go test -race ./internal/httpapi ./internal/auth ./internal/remote ./internal/app`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 全部通过。
