# taskg M10 Impersonation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `taskg` 增加基于 `agent` token + `X-Taskg-As` 的 impersonation 能力，并让 HTTP API、remote CLI、HTTP MCP、audit/access log 在同一套 actor/runtime 语义下工作。

**Architecture:** M10 不新增第三种 token type，而是在现有 `pat` / `agent` 模型上为 `agent` token 新增 `impersonate` capability，并在请求授权阶段拆分出“delegator token 身份”和“subject actor 身份”。实现按四层推进：先收敛 token/scope/runtime 数据结构，再把 impersonation 接进 HTTP 鉴权与 audit，再让 remote CLI 和 HTTP MCP 透传 `X-Taskg-As`，最后补齐文档与全量验证。

**Tech Stack:** Go 1.25、Cobra、GORM、`github.com/glebarez/sqlite`、`net/http`、`github.com/go-chi/chi/v5`、官方 MCP Go SDK、CLI integration tests、`httptest`、MCP integration/schema tests。

---

## 范围锁定

严格按 [M10 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-06-03-taskg-m10-impersonation-design.md) 实现。

必须进入 M10：

- `agent` token 新增 `impersonate` capability；`pat` 不允许持有该 scope。
- 请求头 `X-Taskg-As: <user-name|user-email|user-uuid>`。
- request authorization 生成双重 actor：
  - subject：驱动 `ActorUserID`、`ActorName`、membership role、`assignee:me`、active context。
  - delegator：驱动 token id / token user id 的追责字段。
- workspace 解析保持 M6/M7 现有顺序；若 token 可见多个 workspace 且 impersonation 请求未显式指定 `workspace` 或 `project_id`，返回 `workspace_required`。
- workspace 已确定后，目标 user 不存在或不是该 workspace 成员时统一返回 `membership_not_found`。
- `audit_logs` 增加 `delegator_token_id` / `delegator_user_id`，并贯通 `audit list --json`、HTTP audit response、access log。
- remote CLI 增加 `--as`，作为 client 级配置透传到每个 HTTP 子请求。
- HTTP MCP 支持每个 tool-call request 透传 `X-Taskg-As`；stdio MCP 不支持 impersonation。
- 本地/远程 token create 都要执行新的 M10 约束：只有 admin/owner 才能创建带 `impersonate` 的 token，且远程/API 创建时新 token 仍必须是当前 bearer token 的子集。
- README / manual / OpenAPI / roadmap / requirements 同步。

明确不进入 M10：

- 新的 token type、raw prefix、bootstrap 流程。
- OAuth / OIDC / PKCE。
- session 级 impersonation 状态。
- stdio MCP impersonation。
- employee self-authorization Web UI。
- 新的 Hook event。

## 执行注意事项

- 不要在 `internal/httpapi` 或 `internal/mcpserver` 复制权限规则；缺能力就补 `internal/app`。
- `X-Taskg-As` 只替换 actor，不参与 workspace 选择。
- 所有 impersonation 行为必须先经过统一的 request authorization，再构造 scoped service。
- 任何行为差异都要优先用测试锁定：token 创建、HTTP 鉴权、audit 输出、remote CLI 透传、MCP request-scoped header。
- `audit` 与 `access log` 的“双重 actor”必须同时落地；不要只改 DB 表不改上层 view。
- 保持 `CGO_ENABLED=0` 验收要求不变。

## 文件结构

新增文件：

- `internal/httpapi/impersonation_test.go`
  HTTP impersonation 行为测试，避免把 `tasks_test.go` 撑得过大。
- `internal/remote/client_test.go`
  remote client header 透传与 `--as` 配置单测。

修改文件：

- `internal/auth/scope.go`
  新增 `impersonate` scope，并约束仅允许解析为已知 capability。
- `internal/auth/token.go`
  如需对 `agent` / `pat` + `impersonate` 做创建期校验，在此补充 token type 规则。
- `internal/app/token.go`
  扩展 token create 校验、authenticated token 结构、子集约束逻辑。
- `internal/app/request_scope.go`
  扩展 request authorization 输入/输出，支持 `subject` / `delegator` 双 actor。
- `internal/app/runtime.go`
  视需要扩展 `RuntimeContext` 字段，承载 delegator 元信息。
- `internal/app/audit.go`
  audit row 写入 delegator 字段，`AuditLogView` 暴露 delegator 字段。
- `internal/app/workspace.go`
  复用 `resolveUser`，确保 name / email / uuid 都可解析 impersonation subject。
- `internal/app/request_scope_test.go`
  token scope、workspace_required、subject membership、project/workspace allowlist 交集测试。
- `internal/app/token_test.go`
  `impersonate` scope 的 token create / reject PAT / subset 约束测试。
- `internal/storage/models.go`
  `AuditLog` 增加 `DelegatorTokenID` / `DelegatorUserID` 字段。
- `internal/storage/db.go`
  AutoMigrate 覆盖新 audit 字段。
- `internal/storage/audit_repo.go`
  audit repository entry / list DTO 增加 delegator 字段。
- `internal/storage/db_test.go`
  migration 验证新 audit 列存在。
- `internal/httpapi/middleware.go`
  解析 `X-Taskg-As`，在 access log 中输出 subject/delegator。
- `internal/httpapi/app_service.go`
  request-scoped service 构造接入 impersonation authorization。
- `internal/httpapi/envelope.go`
  `workspace_required` 的 HTTP status 映射。
- `internal/httpapi/tokens.go`
  token create 路径触发新的 `impersonate` 规则。
- `internal/httpapi/tasks_test.go`
  HTTP API task/query + impersonation 测试；如过大则只保留基础回归。
- `internal/httpapi/auth_test.go`
  HTTP 鉴权错误码与 access control 测试。
- `internal/httpapi/mcp_test.go`
  `/mcp` Bearer + header 透传基础测试。
- `internal/httpapi/import_audit.go`
  audit JSON 输出结构如复用 view，需要同步 delegator 字段。
- `internal/mcpserver/auth.go`
  HTTP MCP request authorization 接入 impersonation。
- `internal/mcpserver/tools_common.go`
  确保 tool request 的 header clone 能传入 `X-Taskg-As`。
- `internal/mcpserver/auth_test.go`
  HTTP MCP `workspace_required` / `membership_not_found` / `token_scope_denied` 测试。
- `internal/mcpserver/integration_test.go`
  tool call request-scoped impersonation 行为测试。
- `internal/remote/client.go`
  client options 增加 `AsUser` / extra headers；所有请求统一透传。
- `internal/remote/task.go`
  如有 helper 直接构造 request，确保都复用统一 header 路径。
- `internal/remote/config.go`
  `audit list` remote DTO 增加 delegator 字段。
- `internal/cli/root.go`
  新增 persistent flag `--as`，并写入 effective options。
- `internal/cli/dispatch.go`
  `buildRemoteClient()` 传递 `--as`。
- `internal/cli/token.go`
  token create help / type help 更新为 `impersonate` 只对 `agent` 生效。
- `tests/integration/cli_test.go`
  remote CLI `--as` 黑盒测试。
- `docs/openapi/taskg-v1.yaml`
  token scope enum、新 audit 字段、`X-Taskg-As`、`workspace_required` 等说明。
- `docs/manual/remote-cli-and-api.md`
  `--as` / `X-Taskg-As` 文档。
- `docs/manual/mcp.md`
  HTTP MCP request-scoped impersonation 说明。
- `docs/manual/reference/errors.md`
  `workspace_required` 的 remote/HTTP/MCP 语义说明。
- `README.md`
  M10 用户可见能力概述。
- `ROADMAP.md`
  计划状态与实现链接。
- `docs/requirements.md`
  token / audit / actor 语义同步。

---

## Chunk 1: Token、Runtime 与授权边界

### Task 1: 为 token scope 和创建规则补上 `impersonate`

**Files:**
- Modify: `internal/auth/scope.go`
- Modify: `internal/auth/token.go`
- Modify: `internal/app/token.go`
- Test: `internal/auth/token_test.go`
- Test: `internal/app/token_test.go`

- [ ] **Step 1: 写失败测试，锁定 token 规则**

在 `internal/app/token_test.go` 增加至少这些测试：

```go
func TestCreateTokenRejectsPATWithImpersonateScope(t *testing.T) {}
func TestCreateTokenRejectsMemberCreatingImpersonateScope(t *testing.T) {}
func TestCreateTokenRejectsImpersonateScopeOutsideParentToken(t *testing.T) {}
func TestCreateTokenAllowsAgentImpersonateScopeForOwner(t *testing.T) {}
```

必要时在 `internal/auth/token_test.go` 增加 scope parse / type validate 级别测试。

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/auth ./internal/app -run 'TestCreateToken.*Impersonate|TestValidateTokenCreate'
```

Expected: FAIL，提示 `impersonate` 未注册、PAT 未被拒绝或 subset 规则未覆盖。

- [ ] **Step 3: 扩展 allowed scopes**

在 `internal/auth/scope.go` 增加：

```go
"impersonate": {},
```

保持 parse 行为与现有 scope 一致，不新增通配语法。

- [ ] **Step 4: 把 token type 约束收进创建路径**

在 `internal/auth/token.go` / `internal/app/token.go` 明确：

```go
if tokenType == auth.TokenTypePAT && scopes.Has("impersonate") {
    return RuntimeError{Code: "token_scope_invalid", Message: "impersonate scope requires agent token"}
}
```

并在 app 层补充：

- admin/owner 才能创建带 `impersonate` 的 token
- 远程/API 场景下，`ParentToken` 不包含 `impersonate` 时，不能派生出带 `impersonate` 的新 token

- [ ] **Step 5: 运行绿测**

Run:

```bash
go test ./internal/auth ./internal/app -run 'TestCreateToken.*Impersonate|TestValidateTokenCreate'
```

Expected: PASS。

### Task 2: 在 request authorization 中引入 subject / delegator 双 actor

**Files:**
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/workspace.go`
- Test: `internal/app/request_scope_test.go`

- [ ] **Step 1: 写失败测试，锁定 impersonation 授权行为**

在 `internal/app/request_scope_test.go` 增加至少这些测试：

```go
func TestAuthorizeTokenRequestImpersonationUsesSubjectMembership(t *testing.T) {}
func TestAuthorizeTokenRequestImpersonationRequiresExplicitWorkspaceWhenMultipleVisible(t *testing.T) {}
func TestAuthorizeTokenRequestImpersonationReturnsMembershipNotFoundForUnknownUser(t *testing.T) {}
func TestAuthorizeTokenRequestImpersonationKeepsTokenWorkspaceProjectUpperBound(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/app -run 'TestAuthorizeTokenRequest.*Impersonation'
```

Expected: FAIL，当前 authorization 只有单 actor，且不识别 `X-Taskg-As` 语义。

- [ ] **Step 3: 扩展 authorization 输入与输出**

在 `internal/app/request_scope.go` 的输入结构中增加 subject ref 字段，例如：

```go
type RequestAuthorizationInput struct {
    Token              AuthenticatedToken
    SubjectUserRef     string
    RequiredCapability string
    ...
}
```

在 `AuthorizedRequest` / `RuntimeContext` / `RequestScope` 中补充 delegator 元信息，例如：

```go
DelegatorTokenID string
DelegatorUserID  string
```

- [ ] **Step 4: 实现 impersonation 授权顺序**

按 spec 固定顺序实现：

1. 先检查 token capability。
2. 先解析 workspace / project scope。
3. 如果 `SubjectUserRef` 非空：
   - token 必须是 `agent`
   - token 必须带 `impersonate`
   - 多 workspace 且无显式 `workspace` / `project_id` 时返回 `workspace_required`
   - 解析 user ref（uuid/name/email）
   - user 不存在或不在 workspace 内，统一返回 `membership_not_found`
4. 用 subject membership role 做 `RequiredPermission` 判定。
5. Runtime 中 `ActorUserID` / `ActorName` 写 subject；delegator 单独保留。

- [ ] **Step 5: 运行绿测**

Run:

```bash
go test ./internal/app -run 'TestAuthorizeTokenRequest.*Impersonation'
```

Expected: PASS。

---

## Chunk 2: Audit、SQLite 与 HTTP 鉴权链

### Task 3: 扩展 audit schema 与 app audit view

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/db.go`
- Modify: `internal/storage/audit_repo.go`
- Modify: `internal/app/audit.go`
- Test: `internal/storage/db_test.go`
- Test: `internal/app/token_test.go`

- [ ] **Step 1: 写失败测试，锁定新 audit 字段**

增加至少这些测试：

```go
func TestDBMigratesAuditDelegatorColumns(t *testing.T) {}
func TestAuditListIncludesDelegatorFields(t *testing.T) {}
```

`TestAuditListIncludesDelegatorFields` 可以落在 `internal/app` 层，从 impersonation 写路径读取 `AuditLogView`。

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/storage ./internal/app -run 'TestDBMigratesAuditDelegatorColumns|TestAuditListIncludesDelegatorFields'
```

Expected: FAIL，当前 schema / repo / view 都没有 delegator 字段。

- [ ] **Step 3: 修改 SQLite model 与 repo DTO**

在 `internal/storage/models.go` 的 `AuditLog` 增加：

```go
DelegatorTokenID *string `gorm:"index"`
DelegatorUserID  *string `gorm:"index"`
```

同步扩展 `AuditLogEntry`、`AuditLogView`、repo append/list DTO。

- [ ] **Step 4: 让 audit 写入 delegator**

在 `internal/app/audit.go` 写 row 时，从 runtime 或 request scope 读取 delegator：

```go
row.DelegatorTokenID = stringPtr(txSvc.runtime.DelegatorTokenID)
row.DelegatorUserID = stringPtr(txSvc.runtime.DelegatorUserID)
```

并让 `ListAudit()` 返回对应字段。

- [ ] **Step 5: 运行绿测**

Run:

```bash
go test ./internal/storage ./internal/app -run 'TestDBMigratesAuditDelegatorColumns|TestAuditListIncludesDelegatorFields'
```

Expected: PASS。

### Task 4: 把 impersonation 接进 HTTP middleware 与 error mapping

**Files:**
- Modify: `internal/httpapi/middleware.go`
- Modify: `internal/httpapi/app_service.go`
- Modify: `internal/httpapi/envelope.go`
- Modify: `internal/httpapi/auth_test.go`
- Create: `internal/httpapi/impersonation_test.go`

- [ ] **Step 1: 写失败测试，锁定 HTTP 行为**

在 `internal/httpapi/impersonation_test.go` 增加至少这些测试：

```go
func TestImpersonationRequiresExplicitWorkspaceWhenMultipleVisible(t *testing.T) {}
func TestImpersonationRejectsTokenWithoutScope(t *testing.T) {}
func TestImpersonationReturnsMembershipNotFoundForUnknownUser(t *testing.T) {}
func TestImpersonationTaskActionUsesSubjectIdentity(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/httpapi -run 'TestImpersonation'
```

Expected: FAIL，middleware 还没有读取 `X-Taskg-As`，`workspace_required` 也未映射。

- [ ] **Step 3: 在 HTTP 鉴权链中透传 subject**

在 `internal/httpapi/middleware.go` / `app_service.go`：

- 从 header 读取 `X-Taskg-As`
- 传入 `AuthorizeTokenRequest`
- access log 改成同时输出 subject/delegator，例如：

```text
actor_user_id=<subject> delegator_user_id=<delegator> token_id=<token>
```

- [ ] **Step 4: 补 HTTP status 映射**

在 `internal/httpapi/envelope.go` 增加：

```go
case "workspace_required":
    status = http.StatusBadRequest
```

保持 `membership_not_found` 仍为 403，符合现有 auth 语义。

- [ ] **Step 5: 运行绿测**

Run:

```bash
go test ./internal/httpapi -run 'TestImpersonation|TestAuth'
```

Expected: PASS。

---

## Chunk 3: remote CLI 与 HTTP MCP 透传

### Task 5: 给 remote client 和 CLI 增加 `--as`

**Files:**
- Modify: `internal/remote/client.go`
- Modify: `internal/remote/config.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/dispatch.go`
- Modify: `internal/cli/token.go`
- Test: `internal/cli/root_test.go`
- Create: `internal/remote/client_test.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: 写失败测试，锁定 CLI / client 透传**

增加至少这些测试：

```go
func TestRemoteClientSetsAsHeader(t *testing.T) {}
func TestCLIRemoteAsFlagPropagatesToAllRequests(t *testing.T) {}
```

集成测试可采用“remote task + audit list”两跳或 “info 1” working-set ID 两跳，验证 header 始终存在。

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/remote ./internal/cli -run 'TestRemoteClientSetsAsHeader|Test.*AsFlag'
go test ./tests/integration -run 'TestCLI.*As'
```

Expected: FAIL，当前 `Options` / persistent flags / remote client 都没有 `--as`。

- [ ] **Step 3: 扩展 remote client options**

在 `internal/remote/client.go`：

```go
type Options struct {
    BaseURL string
    Token   string
    AsUser  string
    ...
}
```

在统一 `do(req, out)` 路径增加：

```go
if strings.TrimSpace(c.asUser) != "" {
    req.Header.Set("X-Taskg-As", c.asUser)
}
```

不要在各 endpoint 单独塞 header，避免漏掉子请求。

- [ ] **Step 4: 扩展 CLI persistent flag**

在 `internal/cli/root.go` / `dispatch.go`：

- `Options` 增加 `As string`
- persistent flag 新增 `--as`
- `buildRemoteClient()` 把 `As` 传给 remote client
- 本地模式不把 `--as` 注入 app runtime；如需拦截，统一在 remote mode 语义里说明

- [ ] **Step 5: 运行绿测**

Run:

```bash
go test ./internal/remote ./internal/cli -run 'TestRemoteClientSetsAsHeader|Test.*AsFlag'
go test ./tests/integration -run 'TestCLI.*As'
```

Expected: PASS。

### Task 6: 让 HTTP MCP 支持 request-scoped impersonation

**Files:**
- Modify: `internal/mcpserver/auth.go`
- Modify: `internal/mcpserver/tools_common.go`
- Modify: `internal/mcpserver/auth_test.go`
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `internal/httpapi/mcp_test.go`

- [ ] **Step 1: 写失败测试，锁定 MCP 透传语义**

增加至少这些测试：

```go
func TestMCPHTTPImpersonationRequiresWorkspaceWhenMultipleVisible(t *testing.T) {}
func TestMCPHTTPImpersonationUsesSubjectForTaskQuery(t *testing.T) {}
```

HTTP 层只需校验 header 能到 tool request；integration test 再验证真实行为。

- [ ] **Step 2: 运行红测**

Run:

```bash
go test ./internal/mcpserver ./internal/httpapi -run 'TestMCP.*Impersonation'
```

Expected: FAIL，HTTP MCP 还不会把 `X-Taskg-As` 解释为 subject。

- [ ] **Step 3: 复用 HTTP request authorization**

在 `internal/mcpserver/auth.go` 中，`ServiceForHTTP()` 使用与 HTTP API 相同的 `SubjectUserRef` / `workspace_required` / `membership_not_found` 逻辑。  
不要做连接级缓存；每次 tool call 都从当次 HTTP request / `req.Extra.Header` 读取 header。

- [ ] **Step 4: 保持 stdio MCP 不支持 impersonation**

不要在 `ServiceForStdio()` 增加任何 `X-Taskg-As` 或本地 actor 覆盖逻辑。  
如果测试需要，显式证明 stdio 仍只使用本地 active user/workspace。

- [ ] **Step 5: 运行绿测**

Run:

```bash
go test ./internal/mcpserver ./internal/httpapi -run 'TestMCP.*Impersonation'
```

Expected: PASS。

---

## Chunk 4: Audit 输出、文档与全量验证

### Task 7: 同步 audit 输出 DTO、manual、OpenAPI 与错误说明

**Files:**
- Modify: `internal/remote/config.go`
- Modify: `docs/openapi/taskg-v1.yaml`
- Modify: `docs/manual/remote-cli-and-api.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/manual/reference/errors.md`
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/requirements.md`

- [ ] **Step 1: 更新对外 contract**

确保下列内容同步：

- token scope enum 增加 `impersonate`
- audit JSON response 增加 `delegator_token_id` / `delegator_user_id`
- remote CLI / HTTP API 文档增加 `--as` / `X-Taskg-As`
- manual 里明确“HTTP MCP 是 request-scoped impersonation，stdio MCP 不支持”
- `workspace_required` 在 remote/HTTP/MCP 场景下的触发条件写清楚

- [ ] **Step 2: 运行文档/契约相关测试**

Run:

```bash
go test ./internal/mcpserver -run 'TestSchema|TestListTools'
go test ./internal/httpapi -run 'TestMCP|TestAuth'
```

Expected: PASS，schema golden / route tests 不再引用旧 contract。

### Task 8: 跑完整验收并收尾

**Files:**
- Verify only; no new code unless前面发现缺口

- [ ] **Step 1: 运行定向验证**

Run:

```bash
go test ./internal/auth ./internal/app ./internal/storage ./internal/httpapi ./internal/mcpserver ./internal/remote ./internal/cli
go test ./tests/integration
```

Expected: PASS。

- [ ] **Step 2: 运行全量验收**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS。

如果 `go build` 生成本地二进制，执行：

```bash
rm -f taskg
```

- [ ] **Step 3: 检查格式与脏 diff**

Run:

```bash
gofmt -w internal/auth/*.go internal/app/*.go internal/storage/*.go internal/httpapi/*.go internal/mcpserver/*.go internal/remote/*.go internal/cli/*.go
git diff --check
```

Expected: no output from `git diff --check`。

- [ ] **Step 4: Commit**

```bash
git add internal/auth internal/app internal/storage internal/httpapi internal/mcpserver internal/remote internal/cli tests/integration docs/openapi docs/manual README.md ROADMAP.md docs/requirements.md
git commit -m "feat: 实现 token impersonation"
```

---

## 交付检查表

- [ ] `agent` token 支持 `impersonate`，`pat` 被拒绝
- [ ] impersonation 只替换 actor，不改变 workspace 选择逻辑
- [ ] 多 workspace 且未显式指定 scope 时返回 `workspace_required`
- [ ] 未知 user 与非成员统一返回 `membership_not_found`
- [ ] audit / access log 都有 subject + delegator
- [ ] remote CLI `--as` 透传到所有 HTTP 子请求
- [ ] HTTP MCP 每个 tool-call request 都支持 `X-Taskg-As`
- [ ] stdio MCP 不支持 impersonation
- [ ] OpenAPI / manual / roadmap / requirements 已同步
- [ ] `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 通过

Plan complete and saved to `docs/superpowers/plans/2026-06-03-taskg-m10-impersonation-implementation.md`. Ready to execute?
