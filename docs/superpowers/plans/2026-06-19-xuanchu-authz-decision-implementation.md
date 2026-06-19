# Xuanchu 授权决策层重构 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成授权决策层重构的 Phase 1 和 Phase 2：先落地统一 Authorization Decision 并保持行为等价，再集中整理授权错误语义、HTTP status 映射和文档。

**Architecture:** 新增 `internal/authz` 作为纯授权概念与规则包，避免 HTTP/MCP/Cobra/GORM 细节进入该层。`internal/app` 继续负责 repository 查询和 runtime 编排，但输出 `authz.Decision`；HTTP API 与 HTTP MCP 通过同一 app 授权入口获得 scoped service。Phase 2 将授权错误码集中为 `authz` 常量，并让 HTTP status 映射集中维护。

**Tech Stack:** Go 1.25、GORM、github.com/glebarez/sqlite、PostgreSQL driver、chi、MCP Go SDK；测试使用 Go test 和现有集成测试。

**Spec:** `docs/superpowers/specs/2026-06-19-xuanchu-authz-decision-design.md`

**Delivery Boundary:** 本计划必须完整交付 Phase 1 和 Phase 2。Phase 1 只做行为等价的内部结构整理；Phase 2 必须继续完成授权错误语义、HTTP status 映射和文档同步。Phase 3 的浏览器 SSO/OIDC、cookie session、企业目录同步只保留架构预留，不进入本计划。

---

## File Structure

新增文件：

- `internal/authz/model.go`：定义 `Role`、`Permission`、`Credential`、`Principal`、`Delegator`、`TenantScope`、`RequestScope`、`Decision`、`Requirement`。
- `internal/authz/policy.go`：集中 role permission matrix 和 `AllowedForRole`。
- `internal/authz/scope.go`：集中 request scope 的 capability、workspace allowlist、project allowlist 判断。
- `internal/authz/errors.go`：Phase 2 集中授权错误码、`Error`、`PermissionError`。
- `internal/authz/policy_test.go`：覆盖 role permission matrix。
- `internal/authz/scope_test.go`：覆盖 capability、workspace/project allowlist。
- `internal/authz/errors_test.go`：覆盖错误码和错误类型。
- `internal/httpapi/error_status.go`：集中 HTTP status 映射。
- `internal/httpapi/error_status_test.go`：覆盖授权错误码到 HTTP status 的映射。

修改文件：

- `internal/app/runtime.go`：将 `Role`、`RuntimeError` 迁移为 `authz` 类型别名，保留现有 app API。
- `internal/app/permission.go`：将 `Permission`、`PermissionError` 迁移为 `authz` 类型别名，保留现有 app API。
- `internal/app/workspace.go`：删除本地 role matrix，`requireRolePermission` 改为调用 `authz.AllowedForRole`。
- `internal/app/request_scope.go`：让 `RequestScope` 使用 `authz.RequestScope`，`AuthorizeTokenRequest` 构造并返回 `authz.Decision`。
- `internal/app/request_scope_test.go`：补 Decision 断言和错误语义回归。
- `internal/httpapi/app_service.go`：用 `AuthorizedRequest.Decision` 构造 scoped service 和 access log state。
- `internal/httpapi/envelope.go`：`writeAppError` 改为使用集中 status 映射。
- `internal/httpapi/auth_test.go`、`internal/httpapi/impersonation_test.go`、`internal/httpapi/mcp_test.go`：补 HTTP 错误语义和 impersonation 回归。
- `internal/mcpserver/auth.go`：HTTP MCP 使用同一 Decision 输出构造 service。
- `internal/mcpserver/auth_test.go`、`internal/mcpserver/integration_test.go`：补 MCP 授权一致性回归。
- `README.md`、`docs/manual/team-workspaces-projects.md`、`docs/manual/web-console.md`、`docs/manual/mcp.md`：同步说明 role、token scope、workspace/project allowlist、impersonation、server admin token、错误边界。

---

## Chunk 1: Phase 1 authz 模型和权限矩阵

### Task 1: 新增 authz 基础模型

**Files:**
- Create: `internal/authz/model.go`
- Create: `internal/authz/scope.go`
- Create: `internal/authz/scope_test.go`

- [ ] **Step 1: 写 RequestScope 失败测试**

在 `internal/authz/scope_test.go` 新增：

```go
package authz

import "testing"

func TestRequestScopeAllowsCapabilityWorkspaceAndProject(t *testing.T) {
	scope := RequestScope{
		TokenID:      "tok-1",
		TokenType:    "agent",
		WorkspaceIDs: []string{"ws-1"},
		ProjectIDs:   []string{"p-1"},
		Capabilities: []string{"task:read", "project:read"},
	}
	if !scope.HasCapability("task:read") {
		t.Fatal("HasCapability(task:read) = false")
	}
	if scope.HasCapability("task:write") {
		t.Fatal("HasCapability(task:write) = true")
	}
	if !scope.AllowsWorkspace("ws-1") || scope.AllowsWorkspace("ws-2") {
		t.Fatalf("workspace allowlist mismatch")
	}
	if !scope.AllowsProject("p-1") || scope.AllowsProject("p-2") {
		t.Fatalf("project allowlist mismatch")
	}
}

func TestRequestScopeEmptyAllowlistsAreUnrestricted(t *testing.T) {
	scope := RequestScope{}
	if !scope.HasCapability("") {
		t.Fatal("empty capability should be allowed")
	}
	if !scope.AllowsWorkspace("any-ws") {
		t.Fatal("empty workspace allowlist should be unrestricted")
	}
	if !scope.AllowsProject("any-project") {
		t.Fatal("empty project allowlist should be unrestricted")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/authz -run TestRequestScope -count=1
```

Expected: FAIL，原因是 `internal/authz` 包还不存在。

- [ ] **Step 3: 实现 `internal/authz/model.go`**

```go
package authz

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

type Permission string

const (
	PermissionTaskWrite          Permission = "task.write"
	PermissionTaskRead           Permission = "task.read"
	PermissionProjectRead        Permission = "project.read"
	PermissionProjectManage      Permission = "project.manage"
	PermissionProjectConfigRead  Permission = "project.config.read"
	PermissionProjectConfigWrite Permission = "project.config.write"
	PermissionConfigSchemaRead   Permission = "config.schema.read"
	PermissionConfigSchemaWrite  Permission = "config.schema.write"
	PermissionContextUse         Permission = "context.use"
	PermissionContextManage      Permission = "context.manage"
	PermissionUDAManage          Permission = "uda.manage"
	PermissionWorkspaceRead      Permission = "workspace.read"
	PermissionWorkspaceModify    Permission = "workspace.modify"
	PermissionWorkspaceArchive   Permission = "workspace.archive"
	PermissionMemberManage       Permission = "member.manage"
	PermissionMemberManageOwner  Permission = "member.manage.owner"
	PermissionAuditRead          Permission = "audit.read"
	PermissionTokenRead          Permission = "token.read"
	PermissionTokenWrite         Permission = "token.write"
	PermissionHookRead           Permission = "hook.read"
	PermissionHookWrite          Permission = "hook.write"
	PermissionNotificationRead   Permission = "notification.read"
	PermissionNotificationWrite  Permission = "notification.write"
	PermissionReminderRead       Permission = "reminder.read"
	PermissionReminderWrite      Permission = "reminder.write"
)

type CredentialKind string

const (
	CredentialPAT            CredentialKind = "pat"
	CredentialAgent          CredentialKind = "agent"
	CredentialServerAdmin    CredentialKind = "server_admin"
	CredentialBrowserSession CredentialKind = "browser_session"
)

type Credential struct {
	Kind         CredentialKind
	TokenID      string
	TokenUserID  string
	Capabilities []string
	WorkspaceIDs []string
	ProjectIDs   []string
}

type Principal struct {
	UserID   string
	UserName string
}

type Delegator struct {
	UserID  string
	TokenID string
}

type TenantScope struct {
	WorkspaceID   string
	WorkspaceSlug string
	ProjectID     *string
}

type RequestScope struct {
	TokenID      string
	TokenType    string
	WorkspaceIDs []string
	ProjectIDs   []string
	Capabilities []string
}

type Requirement struct {
	Capability string
	Permission Permission
}

type Decision struct {
	Principal   Principal
	Delegator   *Delegator
	Credential  Credential
	Tenant      TenantScope
	Role        Role
	RequestScope RequestScope
}
```

- [ ] **Step 4: 实现 `internal/authz/scope.go`**

```go
package authz

import (
	"slices"
	"strings"
)

func (s RequestScope) HasCapability(capability string) bool {
	if strings.TrimSpace(capability) == "" {
		return true
	}
	return slices.Contains(s.Capabilities, capability)
}

func (s RequestScope) RestrictsWorkspaces() bool {
	return len(s.WorkspaceIDs) > 0
}

func (s RequestScope) RestrictsProjects() bool {
	return len(s.ProjectIDs) > 0
}

func (s RequestScope) AllowsWorkspace(id string) bool {
	if !s.RestrictsWorkspaces() {
		return true
	}
	return slices.Contains(s.WorkspaceIDs, id)
}

func (s RequestScope) AllowsProject(id string) bool {
	if !s.RestrictsProjects() {
		return true
	}
	return slices.Contains(s.ProjectIDs, id)
}
```

- [ ] **Step 5: 运行测试确认通过**

Run:

```bash
go test ./internal/authz -run TestRequestScope -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/authz/model.go internal/authz/scope.go internal/authz/scope_test.go
git commit -m "feat: 新增授权决策基础模型"
```

### Task 2: 迁移 role permission matrix

**Files:**
- Create: `internal/authz/policy.go`
- Create: `internal/authz/policy_test.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/permission.go`
- Modify: `internal/app/workspace.go`

- [ ] **Step 1: 写 role matrix 失败测试**

在 `internal/authz/policy_test.go` 新增表格测试：

```go
package authz

import "testing"

func TestAllowedForRole(t *testing.T) {
	tests := []struct {
		name string
		role Role
		perm Permission
		want bool
	}{
		{name: "owner can archive workspace", role: RoleOwner, perm: PermissionWorkspaceArchive, want: true},
		{name: "admin can manage members", role: RoleAdmin, perm: PermissionMemberManage, want: true},
		{name: "admin cannot manage owner", role: RoleAdmin, perm: PermissionMemberManageOwner, want: false},
		{name: "member can write task", role: RoleMember, perm: PermissionTaskWrite, want: true},
		{name: "member cannot manage project", role: RoleMember, perm: PermissionProjectManage, want: false},
		{name: "viewer can read task", role: RoleViewer, perm: PermissionTaskRead, want: true},
		{name: "viewer cannot write task", role: RoleViewer, perm: PermissionTaskWrite, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowedForRole(tt.role, tt.perm); got != tt.want {
				t.Fatalf("AllowedForRole(%s,%s) = %v, want %v", tt.role, tt.perm, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/authz -run TestAllowedForRole -count=1
```

Expected: FAIL，`AllowedForRole` 未定义。

- [ ] **Step 3: 实现 `internal/authz/policy.go`**

把 `internal/app/workspace.go` 现有 `allowedForRole` 矩阵原样迁入：

```go
package authz

func AllowedForRole(role Role, p Permission) bool {
	switch role {
	case RoleOwner:
		return true
	case RoleAdmin:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectManage, PermissionProjectConfigRead, PermissionProjectConfigWrite,
			PermissionConfigSchemaRead, PermissionConfigSchemaWrite,
			PermissionContextUse, PermissionContextManage, PermissionUDAManage, PermissionWorkspaceRead, PermissionWorkspaceModify, PermissionMemberManage, PermissionAuditRead,
			PermissionTokenRead, PermissionTokenWrite,
			PermissionHookRead, PermissionHookWrite,
			PermissionNotificationRead, PermissionNotificationWrite, PermissionReminderRead, PermissionReminderWrite:
			return true
		}
	case RoleMember:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectConfigRead, PermissionConfigSchemaRead,
			PermissionContextUse, PermissionContextManage, PermissionWorkspaceRead,
			PermissionReminderRead:
			return true
		}
	case RoleViewer:
		switch p {
		case PermissionTaskRead, PermissionProjectRead, PermissionProjectConfigRead, PermissionConfigSchemaRead, PermissionContextUse, PermissionWorkspaceRead:
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 在 app 层保留类型兼容别名**

Modify `internal/app/runtime.go`:

```go
import "git.dajee.net/dajee/xuanchu/internal/authz"

type Role = authz.Role

const (
	RoleOwner  = authz.RoleOwner
	RoleAdmin  = authz.RoleAdmin
	RoleMember = authz.RoleMember
	RoleViewer = authz.RoleViewer
)
```

Modify `internal/app/permission.go`:

```go
import "git.dajee.net/dajee/xuanchu/internal/authz"

type Permission = authz.Permission

const (
	PermissionTaskWrite          = authz.PermissionTaskWrite
	PermissionTaskRead           = authz.PermissionTaskRead
	PermissionProjectRead        = authz.PermissionProjectRead
	PermissionProjectManage      = authz.PermissionProjectManage
	PermissionProjectConfigRead  = authz.PermissionProjectConfigRead
	PermissionProjectConfigWrite = authz.PermissionProjectConfigWrite
	PermissionConfigSchemaRead   = authz.PermissionConfigSchemaRead
	PermissionConfigSchemaWrite  = authz.PermissionConfigSchemaWrite
	PermissionContextUse         = authz.PermissionContextUse
	PermissionContextManage      = authz.PermissionContextManage
	PermissionUDAManage          = authz.PermissionUDAManage
	PermissionWorkspaceRead      = authz.PermissionWorkspaceRead
	PermissionWorkspaceModify    = authz.PermissionWorkspaceModify
	PermissionWorkspaceArchive   = authz.PermissionWorkspaceArchive
	PermissionMemberManage       = authz.PermissionMemberManage
	PermissionMemberManageOwner  = authz.PermissionMemberManageOwner
	PermissionAuditRead          = authz.PermissionAuditRead
	PermissionTokenRead          = authz.PermissionTokenRead
	PermissionTokenWrite         = authz.PermissionTokenWrite
	PermissionHookRead           = authz.PermissionHookRead
	PermissionHookWrite          = authz.PermissionHookWrite
	PermissionNotificationRead   = authz.PermissionNotificationRead
	PermissionNotificationWrite  = authz.PermissionNotificationWrite
	PermissionReminderRead       = authz.PermissionReminderRead
	PermissionReminderWrite      = authz.PermissionReminderWrite
)
```

注意：此步骤只迁移类型和常量来源，不改变 `RuntimeError` / `PermissionError`，这两个留到 Phase 2。

- [ ] **Step 5: `requireRolePermission` 改用 authz policy**

Modify `internal/app/workspace.go`:

```go
func requireRolePermission(role Role, permission Permission) error {
	if authz.AllowedForRole(role, permission) {
		return nil
	}
	return PermissionError{Code: "permission_denied", Message: "permission denied"}
}
```

删除同文件中的 `allowedForRole` 函数。

- [ ] **Step 6: 运行 authz 和 app 权限测试**

Run:

```bash
go test ./internal/authz ./internal/app -run 'TestAllowedForRole|TestProjectPermissionsByRole|TestWorkspaceRolePermissions|TestReadMethodsRequireTaskReadPermission' -count=1
```

Expected: PASS。若测试名不完全匹配，以实际 `go test` 输出为准，但必须覆盖 `internal/app` 现有权限测试。

- [ ] **Step 7: 提交**

```bash
git add internal/authz/policy.go internal/authz/policy_test.go internal/app/runtime.go internal/app/permission.go internal/app/workspace.go
git commit -m "refactor: 集中 workspace 角色权限矩阵"
```

---

## Chunk 2: Phase 1 App 授权决策输出

### Task 3: 将 app RequestScope 接到 authz

**Files:**
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/request_scope_test.go`

- [ ] **Step 1: 写兼容测试**

在 `internal/app/request_scope_test.go` 增加测试，确保 `NewRequestScope` 仍返回同样数据，且 `projectScopeExpr` 仍过滤项目：

```go
func TestNewRequestScopeUsesAuthzScopeSemantics(t *testing.T) {
	scope := NewRequestScope(TokenView{
		ID:           "tok-1",
		Type:         "agent",
		WorkspaceIDs: []string{"ws-1"},
		ProjectIDs:   []string{"p-1"},
		Scopes:       []string{"task:read"},
	})
	if scope.TokenID != "tok-1" || scope.TokenType != "agent" {
		t.Fatalf("scope identity = %#v", scope)
	}
	if !scope.HasCapability("task:read") || scope.HasCapability("task:write") {
		t.Fatalf("capability behavior changed: %#v", scope)
	}
	if !scope.AllowsWorkspace("ws-1") || scope.AllowsWorkspace("ws-2") {
		t.Fatalf("workspace allowlist behavior changed: %#v", scope)
	}
	if !scope.AllowsProject("p-1") || scope.AllowsProject("p-2") {
		t.Fatalf("project allowlist behavior changed: %#v", scope)
	}
}
```

- [ ] **Step 2: Run test**

Run:

```bash
go test ./internal/app -run TestNewRequestScopeUsesAuthzScopeSemantics -count=1
```

Expected: PASS before and after refactor. This is a behavior pin.

- [ ] **Step 3: Alias app RequestScope to authz.RequestScope**

Modify `internal/app/request_scope.go`:

```go
type RequestScope = authz.RequestScope
```

Keep `NewRequestScope(token TokenView) RequestScope` in app because it depends on `TokenView`.

- [ ] **Step 4: Replace method-style `projectFilterExpr`**

Because methods cannot be added to an imported alias type, replace:

```go
return s.requestScope.projectFilterExpr()
```

with:

```go
return requestScopeProjectFilterExpr(s.requestScope)
```

Add helper in `internal/app/request_scope.go`:

```go
func requestScopeProjectFilterExpr(scope *RequestScope) query.Expr {
	if scope == nil || !scope.RestrictsProjects() {
		return nil
	}
	var expr query.Expr
	for _, projectID := range scope.ProjectIDs {
		predicate := query.Predicate{
			Attribute: query.AttrProjectID,
			Operator:  query.OpEqual,
			Value:     query.StringValue(projectID),
		}
		if expr == nil {
			expr = predicate
			continue
		}
		expr = query.Or(expr, predicate)
	}
	return expr
}
```

Remove the old `func (s RequestScope) projectFilterExpr() query.Expr`.

- [ ] **Step 5: Run focused app request scope tests**

Run:

```bash
go test ./internal/app -run 'TestNewRequestScope|TestProjectScopedService|TestAuthorizeTokenRequest' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/app/request_scope.go internal/app/request_scope_test.go
git commit -m "refactor: 复用 authz 请求范围模型"
```

### Task 4: `AuthorizeTokenRequest` 输出 Decision

**Files:**
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/request_scope_test.go`

- [ ] **Step 1: 写 Decision 失败测试**

在 `internal/app/request_scope_test.go` 对已有成功路径补 Decision 断言，或新增：

```go
func TestAuthorizeTokenRequestReturnsDecision(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	if authorized.Decision.Principal.UserID != authn.User.ID {
		t.Fatalf("principal = %#v, want token user %s", authorized.Decision.Principal, authn.User.ID)
	}
	if authorized.Decision.Delegator != nil {
		t.Fatalf("delegator = %#v, want nil", authorized.Decision.Delegator)
	}
	if authorized.Decision.Tenant.WorkspaceID != authorized.Workspace.ID {
		t.Fatalf("tenant = %#v, workspace = %#v", authorized.Decision.Tenant, authorized.Workspace)
	}
	if authorized.Decision.Credential.TokenID != created.View.ID {
		t.Fatalf("credential token id = %q, want %q", authorized.Decision.Credential.TokenID, created.View.ID)
	}
}
```

- [ ] **Step 2: 写 impersonation Decision 失败测试**

在现有 `TestAuthorizeTokenRequestImpersonationUsesSubjectMembership` 中补断言，或新增：

```go
if authorized.Decision.Delegator == nil {
	t.Fatal("delegator = nil, want delegator for impersonation")
}
if authorized.Decision.Delegator.TokenID != created.View.ID {
	t.Fatalf("delegator token = %q, want %q", authorized.Decision.Delegator.TokenID, created.View.ID)
}
if authorized.Decision.Principal.UserID != memberUser.ID {
	t.Fatalf("principal = %q, want subject %q", authorized.Decision.Principal.UserID, memberUser.ID)
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
go test ./internal/app -run 'TestAuthorizeTokenRequestReturnsDecision|TestAuthorizeTokenRequestImpersonationUsesSubjectMembership' -count=1
```

Expected: FAIL，`AuthorizedRequest.Decision` 不存在。

- [ ] **Step 4: Add Decision to `AuthorizedRequest`**

Modify `internal/app/request_scope.go`:

```go
type AuthorizedRequest struct {
	Runtime   RuntimeContext
	Scope     RequestScope
	Workspace storage.Workspace
	Project   *storage.Project
	Decision  authz.Decision
}
```

- [ ] **Step 5: Build Decision in `AuthorizeTokenRequest`**

After `runtime` and `effectiveScope` are known, create:

```go
decision := authz.Decision{
	Principal: authz.Principal{
		UserID:   subjectUser.ID,
		UserName: subjectUser.Name,
	},
	Credential: authz.Credential{
		Kind:         credentialKindFromTokenType(input.Token.Token.Type),
		TokenID:      input.Token.Token.ID,
		TokenUserID:  tokenUser.ID,
		Capabilities: append([]string(nil), scope.Capabilities...),
		WorkspaceIDs: append([]string(nil), scope.WorkspaceIDs...),
		ProjectIDs:   append([]string(nil), scope.ProjectIDs...),
	},
	Tenant: authz.TenantScope{
		WorkspaceID:   workspace.ID,
		WorkspaceSlug: workspace.Slug,
	},
	Role:         Role(member.Role),
	RequestScope: effectiveScope,
}
if project != nil {
	decision.Tenant.ProjectID = &project.ID
}
if delegatorTokenID != "" {
	decision.Delegator = &authz.Delegator{UserID: delegatorUserID, TokenID: delegatorTokenID}
}
```

Add helper:

```go
func credentialKindFromTokenType(tokenType string) authz.CredentialKind {
	switch tokenType {
	case auth.TokenTypeAgent:
		return authz.CredentialAgent
	case auth.TokenTypePAT:
		return authz.CredentialPAT
	default:
		return authz.CredentialKind(tokenType)
	}
}
```

- [ ] **Step 6: Return Decision without changing existing fields**

```go
return AuthorizedRequest{
	Runtime:   runtime,
	Scope:     effectiveScope,
	Workspace: workspace,
	Project:   project,
	Decision:  decision,
}, nil
```

- [ ] **Step 7: Run focused tests**

Run:

```bash
go test ./internal/app -run 'TestAuthorizeTokenRequest|TestProjectScopedService' -count=1
```

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add internal/app/request_scope.go internal/app/request_scope_test.go
git commit -m "refactor: 输出授权决策对象"
```

### Task 5: 用 Decision 构造 RuntimeContext

**Files:**
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/request_scope_test.go`

- [ ] **Step 1: 写 runtime conversion 测试**

新增测试：

```go
func TestRuntimeContextFromDecision(t *testing.T) {
	delegator := &authz.Delegator{UserID: "service-user", TokenID: "tok-1"}
	decision := authz.Decision{
		Principal: authz.Principal{UserID: "alice-id", UserName: "alice"},
		Delegator: delegator,
		Tenant: authz.TenantScope{WorkspaceID: "ws-1", WorkspaceSlug: "team"},
		Role: RoleMember,
	}
	rt := runtimeContextFromDecision(decision)
	if rt.ActorUserID != "alice-id" || rt.ActorName != "alice" || rt.WorkspaceID != "ws-1" || rt.WorkspaceSlug != "team" || rt.Role != RoleMember {
		t.Fatalf("runtime = %#v", rt)
	}
	if rt.DelegatorUserID != "service-user" || rt.DelegatorTokenID != "tok-1" {
		t.Fatalf("delegator runtime fields = %#v", rt)
	}
}
```

Add required import:

```go
import "git.dajee.net/dajee/xuanchu/internal/authz"
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./internal/app -run TestRuntimeContextFromDecision -count=1
```

Expected: FAIL，helper 不存在。

- [ ] **Step 3: Implement helper**

In `internal/app/request_scope.go`:

```go
func runtimeContextFromDecision(decision authz.Decision) RuntimeContext {
	rt := RuntimeContext{
		ActorUserID:   decision.Principal.UserID,
		ActorName:     decision.Principal.UserName,
		WorkspaceID:   decision.Tenant.WorkspaceID,
		WorkspaceSlug: decision.Tenant.WorkspaceSlug,
		Role:          decision.Role,
	}
	if decision.Delegator != nil {
		rt.DelegatorUserID = decision.Delegator.UserID
		rt.DelegatorTokenID = decision.Delegator.TokenID
	}
	return rt
}
```

- [ ] **Step 4: Use helper in `AuthorizeTokenRequest`**

After building `decision`, replace direct `runtime := RuntimeContext{...}` construction with:

```go
runtime := runtimeContextFromDecision(decision)
```

Keep the role permission check:

```go
if err := requireRolePermission(runtime.Role, input.RequiredPermission); err != nil {
	return AuthorizedRequest{}, err
}
```

- [ ] **Step 5: Run app authorization tests**

Run:

```bash
go test ./internal/app -run 'TestRuntimeContextFromDecision|TestAuthorizeTokenRequest|TestProjectScopedService' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/app/request_scope.go internal/app/request_scope_test.go
git commit -m "refactor: 由授权决策生成运行时上下文"
```

---

## Chunk 3: Phase 1 HTTP API 和 HTTP MCP 复用 Decision

### Task 6: HTTP scoped service 使用 Decision

**Files:**
- Modify: `internal/httpapi/app_service.go`
- Modify: `internal/httpapi/auth_test.go`
- Modify: `internal/httpapi/impersonation_test.go`

- [ ] **Step 1: 补 HTTP access log Decision 回归测试**

在 `internal/httpapi/impersonation_test.go` 或已有 access log 测试附近新增测试，使用 `httptest` server 的 stderr/logger 捕获。目标断言：impersonation 成功后 access log state 中仍包含 `delegator_user_id` 和 `delegator_token_id`。

如果已有日志测试设施不足，先在 `internal/httpapi/app_service.go` 中不改行为，仅补成功路径 HTTP API 响应断言：

```go
func TestHTTPImpersonationUsesDecisionRuntime(t *testing.T) {
	// 复用现有 impersonation fixture。
	// 请求 GET /api/v1/me 不使用 scopedService，不适合本测试。
	// 请求 GET /api/v1/tasks?workspace=local，并带 X-Xuanchu-As: alice。
	// 断言返回 200，且后续 audit/task 行为中的 actor 为 alice。
}
```

Expected: 先写一个会因缺少 Decision 消费断言而失败的测试；如果现有 behavior 已经通过，保留为回归 pin。

- [ ] **Step 2: Add helper to build scoped service from AuthorizedRequest**

In `internal/httpapi/app_service.go`, replace manual runtime fields with Decision-derived fields:

```go
scoped, err := app.NewService(app.ServiceOptions{
	Store:        s.store,
	Clock:        s.effectiveClock(),
	Runtime:      &authorized.Runtime,
	RequestScope: &authorized.Decision.RequestScope,
})
```

Then set:

```go
authn.EffectiveWorkspace = authorized.Workspace
if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok {
	state.actorID = authorized.Decision.Principal.UserID
	state.workspaceID = authorized.Decision.Tenant.WorkspaceID
	state.workspaceRef = authorized.Decision.Tenant.WorkspaceSlug
	if authorized.Decision.Delegator != nil {
		state.delegatorUserID = authorized.Decision.Delegator.UserID
		state.delegatorTokenID = authorized.Decision.Delegator.TokenID
	}
}
```

Preserve existing `impersonateAttempt` behavior on errors.

- [ ] **Step 3: Run HTTP auth tests**

Run:

```bash
go test ./internal/httpapi -run 'TestAuth|TestHTTPImpersonation|TestTaskListProjectIDSelectsOwningWorkspace|TestWorkspaceListRespectsTokenWorkspaceScope' -count=1
```

Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add internal/httpapi/app_service.go internal/httpapi/auth_test.go internal/httpapi/impersonation_test.go
git commit -m "refactor: HTTP API 使用授权决策构造服务"
```

### Task 7: HTTP MCP 使用同一 Decision 输出

**Files:**
- Modify: `internal/mcpserver/auth.go`
- Modify: `internal/mcpserver/auth_test.go`
- Modify: `internal/mcpserver/integration_test.go`

- [ ] **Step 1: 补 HTTP MCP impersonation decision 回归**

在 `internal/mcpserver/auth_test.go` 新增或扩展测试：

```go
func TestServiceForHTTPUsesImpersonationDecision(t *testing.T) {
	// 建 local owner、alice member、带 impersonate 的 agent token。
	// 构造 HTTP request: Authorization Bearer + X-Xuanchu-As: alice + workspace=local。
	// 调 RuntimeFactory.ServiceForHTTP(..., "task:read", app.PermissionTaskRead)。
	// 断言 svc.Runtime().ActorName == "alice"。
	// 断言 svc.Runtime().DelegatorTokenID != ""。
}
```

- [ ] **Step 2: Run test to verify current behavior**

Run:

```bash
go test ./internal/mcpserver -run TestServiceForHTTPUsesImpersonationDecision -count=1
```

Expected: PASS 或 FAIL。如果当前行为已 PASS，保留为重构回归；如果 FAIL，先修实现。

- [ ] **Step 3: Update `ServiceForHTTP` to consume Decision scope**

In `internal/mcpserver/auth.go`:

```go
scoped, err := app.NewService(app.ServiceOptions{
	Store:        f.Store,
	Clock:        f.Clock,
	Runtime:      &authorized.Runtime,
	RequestScope: &authorized.Decision.RequestScope,
})
```

Ensure no code rebuilds scope from token after authorization.

- [ ] **Step 4: Keep project ref matching**

Leave `ensureProjectRefsMatch(scoped, input.Project, input.ProjectID)` after scoped service creation. This check is not authorization; it validates tool input consistency.

- [ ] **Step 5: Run MCP focused tests**

Run:

```bash
go test ./internal/mcpserver -run 'TestServiceForHTTP|TestServiceForStdio|TestHTTPMCP' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/mcpserver/auth.go internal/mcpserver/auth_test.go internal/mcpserver/integration_test.go
git commit -m "refactor: HTTP MCP 复用授权决策"
```

### Task 8: Phase 1 回归验证

**Files:**
- No source changes unless tests reveal drift.

- [ ] **Step 1: Run focused authz/app/http/mcp tests**

Run:

```bash
go test ./internal/authz ./internal/app ./internal/httpapi ./internal/mcpserver -count=1
```

Expected: PASS。

- [ ] **Step 2: Run full Go tests**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 3: Run zero-CGO tests**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [ ] **Step 4: Run zero-CGO build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS。If this creates a local `xuanchu` binary, leave it untracked and do not commit it.

- [ ] **Step 5: Commit verification-only fixes if needed**

If any behavior drift was fixed:

```bash
git add <changed files>
git commit -m "fix: 保持授权决策重构行为等价"
```

If no fixes are needed, do not create an empty commit.

---

## Chunk 4: Phase 2 授权错误语义集中化

Phase 2 的目标是集中授权边界错误，不是把所有业务错误都迁到 `internal/authz`。`hook_*`、`notification_*`、`config_*`、`task_ref_invalid`、`project_mismatch` 等领域或接口输入错误继续留在原调用层；本阶段只集中认证、token scope、workspace/project allowlist、membership、role permission 相关错误。

### Task 9: 集中 authz 错误类型和错误码

**Files:**
- Create: `internal/authz/errors.go`
- Create: `internal/authz/errors_test.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/permission.go`

- [ ] **Step 1: 写 authz error 测试**

In `internal/authz/errors_test.go`:

```go
package authz

import "testing"

func TestAuthzErrorTypes(t *testing.T) {
	err := Error{Code: CodeTokenScopeDenied, Message: "token scope denied"}
	if err.Error() != "token scope denied" {
		t.Fatalf("Error() = %q", err.Error())
	}
	permErr := PermissionError{Code: CodePermissionDenied, Message: "permission denied"}
	if permErr.Error() != "permission denied" {
		t.Fatalf("PermissionError() = %q", permErr.Error())
	}
}

func TestAuthzErrorCodeConstants(t *testing.T) {
	tests := map[string]string{
		CodeAuthMissingToken:    "auth_missing_token",
		CodeAuthInvalidToken:    "auth_invalid_token",
		CodeAuthTokenRevoked:    "auth_token_revoked",
		CodeAuthTokenExpired:    "auth_token_expired",
		CodeTokenScopeDenied:    "token_scope_denied",
		CodeWorkspaceScopeDenied:"workspace_scope_denied",
		CodeProjectScopeDenied:  "project_scope_denied",
		CodeMembershipNotFound:  "membership_not_found",
		CodePermissionDenied:    "permission_denied",
		CodeWorkspaceRequired:   "workspace_required",
		CodeWorkspaceArchived:   "workspace_archived",
		CodeProjectNotFound:     "project_not_found",
	}
	for got, want := range tests {
		if got != want {
			t.Fatalf("code constant = %q, want %q", got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./internal/authz -run TestAuthzError -count=1
```

Expected: FAIL，error 类型和常量不存在。

- [ ] **Step 3: Implement `internal/authz/errors.go`**

```go
package authz

const (
	CodeAuthMissingToken      = "auth_missing_token"
	CodeAuthInvalidToken      = "auth_invalid_token"
	CodeAuthTokenRevoked      = "auth_token_revoked"
	CodeAuthTokenExpired      = "auth_token_expired"
	CodeTokenScopeDenied      = "token_scope_denied"
	CodeWorkspaceScopeDenied  = "workspace_scope_denied"
	CodeProjectScopeDenied    = "project_scope_denied"
	CodeMembershipNotFound    = "membership_not_found"
	CodePermissionDenied      = "permission_denied"
	CodeWorkspaceRequired     = "workspace_required"
	CodeWorkspaceArchived     = "workspace_archived"
	CodeWorkspaceNotFound     = "workspace_not_found"
	CodeProjectNotFound       = "project_not_found"
	CodeProjectWorkspaceMismatch = "project_workspace_mismatch"
	CodeTaskNotFound          = "task_not_found"
)

type Error struct {
	Code    string
	Message string
}

func (e Error) Error() string { return e.Message }

type PermissionError struct {
	Code    string
	Message string
}

func (e PermissionError) Error() string { return e.Message }
```

Run `gofmt` after implementation.

- [ ] **Step 4: Alias app error types to authz**

Modify `internal/app/runtime.go`:

```go
type RuntimeError = authz.Error
```

Remove the old `RuntimeError` struct and `Error()` method.

Modify `internal/app/permission.go`:

```go
type PermissionError = authz.PermissionError
```

Remove the old `PermissionError` struct and `Error()` method.

- [ ] **Step 5: Run app/http error tests**

Run:

```bash
go test ./internal/authz ./internal/app ./internal/httpapi -run 'TestAuthzError|TestAuthRequiresBearerHeader|TestWorkspaceScopeDeniedIsForbidden|TestAuthorizeTokenRequestRejectsMissingCapability|TestHTTPImpersonation' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/authz/errors.go internal/authz/errors_test.go internal/app/runtime.go internal/app/permission.go
git commit -m "refactor: 集中授权错误类型"
```

### Task 10: Replace authorization string literals with constants

**Files:**
- Modify: `internal/app/request_scope.go`
- Modify: `internal/app/token.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/workspace.go`
- Modify: `internal/httpapi/middleware.go`
- Modify: `internal/mcpserver/auth.go`

- [ ] **Step 1: Search current literals**

Run:

```bash
rg -n '"auth_missing_token"|"auth_invalid_token"|"auth_token_revoked"|"auth_token_expired"|"token_scope_denied"|"workspace_scope_denied"|"project_scope_denied"|"membership_not_found"|"permission_denied"|"workspace_required"|"workspace_archived"|"workspace_not_found"|"project_not_found"|"task_not_found"' internal
```

Expected: list of current string literal call sites.

- [ ] **Step 2: Replace app-layer authz literals**

Examples:

```go
return AuthorizedRequest{}, RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "token scope denied"}
return AuthorizedRequest{}, RuntimeError{Code: authz.CodeMembershipNotFound, Message: "impersonation target user not found"}
return PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
```

Do not replace unrelated domain errors unless they are part of the authz boundary table.

- [ ] **Step 3: Replace HTTP/MCP authz literals**

Examples:

```go
writeError(w, http.StatusUnauthorized, authz.CodeAuthMissingToken, "missing bearer token", nil)
return nil, app.RuntimeError{Code: authz.CodeAuthMissingToken, Message: "missing bearer token"}
```

- [ ] **Step 4: Run focused tests**

Run:

```bash
go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'TestAuthorizeTokenRequest|TestAuth|TestHTTPImpersonation|TestServiceForHTTP' -count=1
```

Expected: PASS。

- [ ] **Step 5: Confirm fewer literals remain**

Run:

```bash
rg -n '"token_scope_denied"|"workspace_scope_denied"|"project_scope_denied"|"membership_not_found"|"permission_denied"|"workspace_required"' internal/app internal/httpapi internal/mcpserver
```

Expected: remaining matches should be tests or explicit JSON/golden expectations. Production code should use constants.

- [ ] **Step 6: 提交**

```bash
git add internal/app internal/httpapi internal/mcpserver
git commit -m "refactor: 使用授权错误码常量"
```

### Task 11: Centralize HTTP status mapping

**Files:**
- Create: `internal/httpapi/error_status.go`
- Create: `internal/httpapi/error_status_test.go`
- Modify: `internal/httpapi/envelope.go`

- [ ] **Step 1: Write status mapping tests**

In `internal/httpapi/error_status_test.go`:

```go
package httpapi

import (
	"net/http"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/authz"
)

func TestStatusForAppErrorCodeAuthorizationBoundary(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{authz.CodeAuthMissingToken, http.StatusUnauthorized},
		{authz.CodeAuthInvalidToken, http.StatusUnauthorized},
		{authz.CodeAuthTokenExpired, http.StatusUnauthorized},
		{authz.CodeAuthTokenRevoked, http.StatusUnauthorized},
		{authz.CodeTokenScopeDenied, http.StatusForbidden},
		{authz.CodeWorkspaceScopeDenied, http.StatusForbidden},
		{authz.CodeProjectScopeDenied, http.StatusForbidden},
		{authz.CodeMembershipNotFound, http.StatusForbidden},
		{authz.CodePermissionDenied, http.StatusForbidden},
		{authz.CodeWorkspaceRequired, http.StatusBadRequest},
		{authz.CodeWorkspaceNotFound, http.StatusNotFound},
		{authz.CodeProjectNotFound, http.StatusNotFound},
		{authz.CodeTaskNotFound, http.StatusNotFound},
	}
	for _, tt := range tests {
		if got := statusForAppErrorCode(tt.code); got != tt.want {
			t.Fatalf("statusForAppErrorCode(%q) = %d, want %d", tt.code, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./internal/httpapi -run TestStatusForAppErrorCodeAuthorizationBoundary -count=1
```

Expected: FAIL，helper 不存在。

- [ ] **Step 3: Implement `internal/httpapi/error_status.go`**

Move the switch from `writeAppError` into:

```go
func statusForAppErrorCode(code string) int {
	switch code {
	case authz.CodeAuthMissingToken, authz.CodeAuthInvalidToken, authz.CodeAuthTokenExpired, authz.CodeAuthTokenRevoked:
		return http.StatusUnauthorized
	case authz.CodeTokenScopeDenied, authz.CodeWorkspaceScopeDenied, authz.CodeProjectScopeDenied, authz.CodeMembershipNotFound, authz.CodePermissionDenied:
		return http.StatusForbidden
	case authz.CodeWorkspaceRequired:
		return http.StatusBadRequest
	// Keep existing non-auth domain mappings here too.
	default:
		return http.StatusBadRequest
	}
}
```

Important: preserve all current non-auth mappings from `envelope.go`, including hook/notification/token domain errors.

- [ ] **Step 4: Simplify `writeAppError`**

In `internal/httpapi/envelope.go`:

```go
if errors.As(err, &runtimeErr) {
	writeError(w, statusForAppErrorCode(runtimeErr.Code), runtimeErr.Code, runtimeErr.Message, nil)
	return
}
var permissionErr app.PermissionError
if errors.As(err, &permissionErr) {
	writeError(w, statusForAppErrorCode(permissionErr.Code), permissionErr.Code, permissionErr.Message, nil)
	return
}
```

- [ ] **Step 5: Run HTTP error tests**

Run:

```bash
go test ./internal/httpapi -run 'TestStatusForAppErrorCode|TestAuthRequiresBearerHeader|TestWorkspaceScopeDeniedIsForbidden|TestAdminTokenCannotAccessMe|TestNormalTokenCannotAccessAdminAPI|TestHTTPImpersonation' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/httpapi/error_status.go internal/httpapi/error_status_test.go internal/httpapi/envelope.go
git commit -m "refactor: 集中 HTTP 错误状态映射"
```

### Task 12: Document error semantics in tests

**Files:**
- Modify: `internal/app/request_scope_test.go`
- Modify: `internal/httpapi/auth_test.go`
- Modify: `internal/httpapi/impersonation_test.go`
- Modify: `internal/mcpserver/auth_test.go`

- [ ] **Step 1: Add app error boundary table test**

In `internal/app/request_scope_test.go`, add a table-driven test or extend existing tests to cover:

- missing capability -> `token_scope_denied`
- PAT + `X-Xuanchu-As` -> `token_scope_denied`
- Agent without `impersonate` -> `token_scope_denied`
- impersonation target missing -> `membership_not_found`
- impersonation target non-member -> `membership_not_found`
- multiple workspace ambiguity -> `workspace_required`
- project outside allowlist -> `project_scope_denied`
- role too weak -> `permission_denied`

- [ ] **Step 2: Add HTTP status boundary assertions**

In HTTP tests, assert code + status pairs:

- `token_scope_denied` -> 403
- `workspace_scope_denied` -> 403
- `project_scope_denied` -> 403
- `membership_not_found` -> 403
- `permission_denied` -> 403
- `workspace_required` -> 400
- `auth_missing_token` -> 401

Use existing `assertHTTPErrorCode` helper.

- [ ] **Step 3: Add MCP parity assertions**

In `internal/mcpserver/auth_test.go`, ensure HTTP MCP returns the same app error codes for:

- missing token
- project slug ambiguity
- impersonation without scope
- project outside allowlist

- [ ] **Step 4: Run focused boundary tests**

Run:

```bash
go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'ErrorBoundary|AuthorizeTokenRequest|StatusForAppErrorCode|ServiceForHTTP|Impersonation|WorkspaceScopeDenied' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/app/request_scope_test.go internal/httpapi/auth_test.go internal/httpapi/impersonation_test.go internal/mcpserver/auth_test.go
git commit -m "test: 覆盖授权错误边界"
```

---

## Chunk 5: Phase 2 文档同步和最终验证

### Task 13: Update docs for authorization model and errors

**Files:**
- Modify: `README.md`
- Modify: `docs/manual/team-workspaces-projects.md`
- Modify: `docs/manual/web-console.md`
- Modify: `docs/manual/mcp.md`

- [ ] **Step 1: Update README authorization section**

In `README.md` around the existing token scope section, add a concise table:

```markdown
授权判断分两层：

| 层 | 说明 |
|---|---|
| Principal | 本次业务 actor。普通 token 为 token 绑定用户；impersonation 为 `X-Xuanchu-As` 目标用户。 |
| Credential | 请求凭证。PAT / Agent token 只提供 capability 和 allowlist，不能放大 membership role。 |
| TenantScope | effective workspace 和可选 project。workspace 是隔离边界，project 是收窄边界。 |
| Decision | `principal + credential + tenant + role + request scope` 的最终授权结果。 |
```

Also add a short error table for:

- `auth_missing_token`
- `auth_invalid_token`
- `token_scope_denied`
- `workspace_scope_denied`
- `project_scope_denied`
- `membership_not_found`
- `permission_denied`
- `workspace_required`

- [ ] **Step 2: Update `docs/manual/team-workspaces-projects.md`**

Add a section after roles:

```markdown
## 授权决策

本地 CLI 的 actor 来自 active user；远程 HTTP/MCP 的 actor 来自 token，或在 Agent token impersonation 下来自 `X-Xuanchu-As`。最终权限仍然是 membership role、token capability、workspace allowlist、project allowlist 的交集。
```

- [ ] **Step 3: Update `docs/manual/web-console.md`**

Clarify browser token login:

```markdown
普通 Console 的 token 登录不是浏览器 SSO。它只是把 PAT / Agent token 放入当前 tab 的 sessionStorage；服务端仍按 Bearer token 做同一套 Authorization Decision。
```

Reinforce server admin separation.

- [ ] **Step 4: Update `docs/manual/mcp.md`**

Document HTTP MCP parity:

```markdown
HTTP MCP 与 HTTP API 共享同一授权决策：Bearer token、workspace/project 参数、`X-Xuanchu-As` impersonation、token scope 和 membership role 的结果一致。
```

- [ ] **Step 5: Run markdown grep sanity**

Run:

```bash
rg -n "Authorization Decision|授权决策|token_scope_denied|workspace_scope_denied|project_scope_denied|membership_not_found|permission_denied|workspace_required" README.md docs/manual
```

Expected: new documentation appears in README and manuals.

- [ ] **Step 6: 提交**

```bash
git add README.md docs/manual/team-workspaces-projects.md docs/manual/web-console.md docs/manual/mcp.md
git commit -m "docs: 说明授权决策和错误边界"
```

### Task 14: Full verification for P1/P2 completion

**Files:**
- No source changes unless verification finds issues.

- [ ] **Step 1: Run `go test ./...`**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 2: Run zero-CGO tests**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [ ] **Step 3: Run zero-CGO build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS。Do not commit the local `xuanchu` binary if generated.

- [ ] **Step 4: Run vet**

Run:

```bash
go vet ./...
```

Expected: PASS。

- [ ] **Step 5: Run diff hygiene**

Run:

```bash
git diff --check
```

Expected: no output, exit 0.

- [ ] **Step 6: Confirm spec P1/P2 acceptance checklist**

Manually verify:

- `internal/authz` exists and contains model, scope, policy, errors.
- HTTP API and HTTP MCP both consume `AuthorizeTokenRequest` decision output.
- role permission matrix is no longer duplicated in app workspace logic.
- authorization error code constants exist and production code uses them.
- HTTP status mapping is centralized.
- README/manual docs describe decision model and error boundary.

- [ ] **Step 7: Final commit for verification fixes if needed**

If verification required fixes:

```bash
git add <changed files>
git commit -m "fix: 完成授权重构验证修正"
```

If no fixes were needed, do not create an empty commit.

---

## Execution Notes

- Do not start Phase 2 until Phase 1 focused tests and at least `go test ./internal/authz ./internal/app ./internal/httpapi ./internal/mcpserver -count=1` pass.
- Do not introduce database migrations, OIDC, OAuth, browser cookie sessions, organization/group/SCIM, or policy-engine dependencies.
- Keep existing external APIs, MCP tool names, token prefixes, and database schema stable.
- Preserve user-facing JSON identity fields through `task.UserInfo` where applicable.
- If an implementation step reveals a real behavior bug, add a focused regression test and keep the fix inside the nearest task; do not widen scope into SSO or member provisioning.
