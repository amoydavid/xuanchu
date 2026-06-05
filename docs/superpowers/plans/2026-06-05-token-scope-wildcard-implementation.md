# Token Scope 通配符与 Token 修改 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `--scope` 支持通配符、新增 `scope list` 命令、新增 `token modify` 命令和 HTTP API。

**Architecture:** 改动集中在 `internal/auth/scope.go`（通配符展开+有序注册）、`internal/app/token.go`（modify 逻辑）、`internal/storage/token_repo.go`（UpdateToken）、`internal/cli/`（scope + token modify 命令）、`internal/httpapi/tokens.go`（PATCH endpoint）。运行时 scope 检查不变。

**Tech Stack:** Go, Cobra, GORM

**Spec:** `docs/superpowers/specs/2026-06-05-xuanchu-scope-wildcard-design.md`

---

## Chunk 1: Scope 注册与通配符展开

### Task 1: 有序注册 + 通配符展开

**Files:**
- Modify: `internal/auth/scope.go`
- Modify: `internal/auth/token.go`
- Modify: `internal/auth/token_test.go`

- [ ] **Step 1: 写通配符展开失败的测试**

在 `internal/auth/scope_test.go`（与 `token_test.go` 同包，直接写 scope_test.go）中新增：

```go
package auth

import (
	"testing"
)

func TestExpandScopes_ResourceWildcard(t *testing.T) {
	set, err := ParseScopes([]string{"task:*"})
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("task:read") || !set.Has("task:write") {
		t.Fatalf("expected task:read and task:write, got %v", set.Values())
	}
	if set.Has("project:read") {
		t.Fatalf("should not contain project:read")
	}
}

func TestExpandScopes_ActionWildcard(t *testing.T) {
	set, err := ParseScopes([]string{"*:read"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"task:read", "project:read", "context:read", "config:read", "workspace:read", "audit:read", "token:read", "hook:read"} {
		if !set.Has(s) {
			t.Fatalf("missing %s", s)
		}
	}
	if set.Has("task:write") {
		t.Fatalf("should not contain task:write")
	}
}

func TestExpandScopes_StarWildcard(t *testing.T) {
	set, err := ParseScopes([]string{"*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != len(scopeRegistry) {
		t.Fatalf("expected %d scopes, got %d", len(scopeRegistry), len(set))
	}
}

func TestExpandScopes_Mixed(t *testing.T) {
	set, err := ParseScopes([]string{"task:*", "hook:read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 3 {
		t.Fatalf("expected 3 scopes, got %d: %v", len(set), set.Values())
	}
}

func TestExpandScopes_InvalidResource(t *testing.T) {
	_, err := ParseScopes([]string{"foo:*"})
	if err == nil {
		t.Fatal("expected error for foo:*")
	}
}

func TestExpandScopes_InvalidAction(t *testing.T) {
	_, err := ParseScopes([]string{"*:execute"})
	if err == nil {
		t.Fatal("expected error for *:execute")
	}
}

func TestExpandScopes_PlainScopeStillWorks(t *testing.T) {
	set, err := ParseScopes([]string{"task:read"})
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("task:read") || len(set) != 1 {
		t.Fatalf("expected only task:read, got %v", set.Values())
	}
}

func TestScopeRegistryValues(t *testing.T) {
	values := ScopeRegistryValues()
	if len(values) != 16 {
		t.Fatalf("expected 16 scopes, got %d", len(values))
	}
	if values[0] != "task:read" {
		t.Fatalf("expected first scope task:read, got %s", values[0])
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/auth/ -run "TestExpandScopes|TestScopeRegistryValues" -v
```

预期：编译失败或测试失败（`scopeRegistry`、`ScopeRegistryValues` 不存在）。

- [ ] **Step 3: 实现 scope 注册和通配符展开**

修改 `internal/auth/scope.go`：

```go
package auth

import (
	"fmt"
	"sort"
	"strings"
)

var scopeRegistry = []string{
	"task:read", "task:write",
	"project:read", "project:write",
	"context:read", "context:write",
	"config:read", "config:write",
	"workspace:read", "workspace:write",
	"audit:read",
	"token:read", "token:write",
	"hook:read", "hook:write",
	"impersonate",
}

var scopeLookup map[string]struct{}

func init() {
	scopeLookup = make(map[string]struct{}, len(scopeRegistry))
	for _, s := range scopeRegistry {
		scopeLookup[s] = struct{}{}
	}
}

func ScopeRegistryValues() []string {
	out := make([]string, len(scopeRegistry))
	copy(out, scopeRegistry)
	return out
}

type ScopeSet map[string]struct{}

func ParseScopes(values []string) (ScopeSet, error) {
	expanded, err := expandWildcardScopes(values)
	if err != nil {
		return nil, err
	}
	out := ScopeSet{}
	for _, scope := range expanded {
		if _, ok := scopeLookup[scope]; !ok {
			return nil, fmt.Errorf("invalid token scope %q", scope)
		}
		out[scope] = struct{}{}
	}
	return out, nil
}

func expandWildcardScopes(values []string) ([]string, error) {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			scope := strings.TrimSpace(part)
			if scope == "" {
				continue
			}
			if scope == "*" {
				out = append(out, scopeRegistry...)
				continue
			}
			if strings.HasSuffix(scope, ":*") {
				resource := strings.TrimSuffix(scope, ":*")
				matched := expandResourceWildcard(resource)
				if len(matched) == 0 {
					return nil, fmt.Errorf("invalid token scope %q", scope)
				}
				out = append(out, matched...)
				continue
			}
			if strings.HasPrefix(scope, "*:") {
				action := strings.TrimPrefix(scope, "*:")
				matched := expandActionWildcard(action)
				if len(matched) == 0 {
					return nil, fmt.Errorf("invalid token scope %q", scope)
				}
				out = append(out, matched...)
				continue
			}
			out = append(out, scope)
		}
	}
	return out, nil
}

func expandResourceWildcard(resource string) []string {
	prefix := resource + ":"
	var out []string
	for _, s := range scopeRegistry {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func expandActionWildcard(action string) []string {
	suffix := ":" + action
	var out []string
	for _, s := range scopeRegistry {
		if strings.HasSuffix(s, suffix) {
			out = append(out, s)
		}
	}
	return out
}

func (s ScopeSet) Has(scope string) bool {
	_, ok := s[scope]
	return ok
}

func (s ScopeSet) Values() []string {
	out := make([]string, 0, len(s))
	for scope := range s {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: 修改 `ValidateTokenCreate` 使 PAT + `*` 不报错**

修改 `internal/auth/token.go`，将 `ParseScopes` 结果中 PAT 的 `impersonate` 剔除改为静默剔除：

```go
func ValidateTokenCreate(opts CreateTokenOptions) (ScopeSet, error) {
	if opts.Type == "" {
		opts.Type = TokenTypePAT
	}
	if _, err := tokenPrefixForType(opts.Type); err != nil {
		return nil, err
	}
	if opts.Type == TokenTypeAgent {
		if len(opts.WorkspaceIDs) == 0 {
			return nil, fmt.Errorf("agent token requires at least one workspace")
		}
		if len(opts.Scopes) == 0 {
			return nil, fmt.Errorf("agent token requires explicit scopes")
		}
	}
	scopes, err := ParseScopes(opts.Scopes)
	if err != nil {
		return nil, err
	}
	if opts.Type == TokenTypePAT {
		delete(scopes, "impersonate")
	}
	return scopes, nil
}
```

注意：返回值从 `error` 变为 `(ScopeSet, error)`。所有调用方需要适配。

- [ ] **Step 5: 适配 `ValidateTokenCreate` 的调用方**

`ValidateTokenCreate` 唯一调用方是 `internal/app/token.go` 的 `CreateToken` 方法（约第 87-93 行）。当前 `CreateToken` 内部先调用 `ValidateTokenCreate` 做结构校验，然后又调用 `ParseScopes` 做 scope 展开。改为：

1. `scopes, err := auth.ValidateTokenCreate(...)` 获取展开后的 ScopeSet。
2. 删除 `CreateToken` 中重复的 `auth.ParseScopes(opts.Scopes)` 调用。
3. 后续直接使用 `scopes.Values()` 作为存储用的 scope 列表。

注意：`ValidateTokenCreate` 对 PAT 已静默剔除 `impersonate`，所以后续不再需要单独检查 PAT + impersonate。

- [ ] **Step 6: 运行测试确认通过**

```bash
go test ./internal/auth/ -v
go test ./internal/app/ -run "TestToken|TestCreateToken" -v
```

- [ ] **Step 7: 更新 `token_test.go` 中的旧测试**

`TestParseScopesRejectsInvalidAndWildcardScopes` 测试名不再准确（现在 `admin:*` 会走通配符展开，匹配不到任何 scope 而报错）。确认该测试仍通过。如果 `admin:*` 现在走 `expandResourceWildcard("admin")` 返回空而报错，测试仍 PASS。

- [ ] **Step 8: 运行全量测试**

```bash
go test ./...
```

- [ ] **Step 9: 提交**

```bash
git add internal/auth/
git commit -m "feat: scope 有序注册与通配符展开"
```

---

### Task 2: `scope list` CLI 命令

**Files:**
- Create: `internal/cli/scope.go`
- Modify: `internal/cli/root.go`（注册 `scope` 子命令）

- [ ] **Step 1: 创建 `internal/cli/scope.go`**

```go
package cli

import (
	"fmt"
	"strings"

	"github.com/dajee/xuanchu/internal/auth"
	"github.com/dajee/xuanchu/internal/render"
	"github.com/spf13/cobra"
)

func newScopeCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "scope",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newScopeListCommand(opts))
	return cmd
}

func newScopeListCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			scopes := auth.ScopeRegistryValues()
			if currentOpts.JSON {
				out := make([]map[string]string, 0, len(scopes))
				for _, s := range scopes {
					resource, action := splitScope(s)
					out = append(out, map[string]string{
						"scope":    s,
						"resource": resource,
						"action":   action,
					})
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			groups := make(map[string][]string)
			var standalone []string
			for _, s := range scopes {
				resource, action := splitScope(s)
				if resource == "" {
					standalone = append(standalone, action)
				} else {
					groups[resource] = append(groups[resource], action)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "RESOURCE\tACTIONS")
			for _, s := range scopes {
				resource, _ := splitScope(s)
				if resource == "" {
					continue
				}
				if actions, ok := groups[resource]; ok {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", resource, strings.Join(actions, ", "))
					delete(groups, resource)
				}
			}
			if len(standalone) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "STANDALONE")
				for _, s := range standalone {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\n", s)
				}
			}
			return nil
		},
	}
	return cmd
}

func splitScope(s string) (resource, action string) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return "", s
	}
	return s[:idx], s[idx+1:]
}
```

- [ ] **Step 2: 注册 `scope` 子命令**

在 `internal/cli/root.go` 中找到注册子命令的位置（类似 `cmd.AddCommand(newTokenCommand(opts))`），添加：

```go
cmd.AddCommand(newScopeCommand(opts))
```

- [ ] **Step 3: 运行构建和手动验证**

```bash
go build ./cmd/xuanchu
./xuanchu scope list
./xuanchu scope list --json
```

- [ ] **Step 4: 提交**

```bash
git add internal/cli/scope.go internal/cli/root.go
git commit -m "feat: scope list 命令"
```

---

## Chunk 2: Token Modify

### Task 3: Storage 层 UpdateToken

**Files:**
- Modify: `internal/storage/token_repo.go`
- Modify: `internal/storage/token_repo_test.go`

- [ ] **Step 1: 写 UpdateToken 测试**

在 `internal/storage/token_repo_test.go` 中新增：

```go
func TestTokenRepository_Update(t *testing.T) {
	db := testDB(t)
	repo := storage.NewTokenRepository(db)
	entry := storage.ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           "user1",
		Name:             "test-token",
		Type:             "pat",
		TokenPrefix:      "xuanchu_pat_abc",
		TokenHash:        "hash",
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        time.Now().Unix(),
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}
	newName := "renamed"
	newScopes := `["task:read","task:write"]`
	err := repo.Update(entry.ID, storage.TokenUpdates{
		Name:       &newName,
		ScopesJSON: &newScopes,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "renamed" {
		t.Fatalf("name = %q", updated.Name)
	}
	if updated.ScopesJSON != newScopes {
		t.Fatalf("scopes = %q", updated.ScopesJSON)
	}
}

func TestTokenRepository_Update_ClearExpiresAt(t *testing.T) {
	db := testDB(t)
	repo := storage.NewTokenRepository(db)
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	entry := storage.ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           "user1",
		Name:             "test-token",
		Type:             "pat",
		TokenPrefix:      "xuanchu_pat_abc",
		TokenHash:        "hash",
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        time.Now().Unix(),
		ExpiresAt:        &expiresAt,
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(entry.ID, storage.TokenUpdates{ClearExpiresAt: true}); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ExpiresAt != nil {
		t.Fatalf("expected nil ExpiresAt, got %d", *updated.ExpiresAt)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/storage/ -run TestTokenRepository_Update -v
```

- [ ] **Step 3: 实现 UpdateToken**

在 `internal/storage/token_repo.go` 中新增：

```go
type TokenUpdates struct {
	Name             *string
	ScopesJSON       *string
	WorkspaceIDsJSON *string
	ProjectIDsJSON   *string
	ExpiresAt        *int64
	ClearExpiresAt   bool
}

func (r *TokenRepository) Update(id string, updates TokenUpdates) error {
	attrs := map[string]any{}
	if updates.Name != nil {
		attrs["name"] = *updates.Name
	}
	if updates.ScopesJSON != nil {
		attrs["scopes_json"] = *updates.ScopesJSON
	}
	if updates.WorkspaceIDsJSON != nil {
		attrs["workspace_ids_json"] = *updates.WorkspaceIDsJSON
	}
	if updates.ProjectIDsJSON != nil {
		attrs["project_ids_json"] = *updates.ProjectIDsJSON
	}
	if updates.ClearExpiresAt {
		attrs["expires_at"] = nil
	} else if updates.ExpiresAt != nil {
		attrs["expires_at"] = *updates.ExpiresAt
	}
	if len(attrs) == 0 {
		return nil
	}
	result := r.db.Model(&ApiToken{}).Where("id = ?", id).Updates(attrs)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/storage/ -run TestTokenRepository_Update -v
```

- [ ] **Step 5: 提交**

```bash
git add internal/storage/token_repo.go internal/storage/token_repo_test.go
git commit -m "feat: storage 层 TokenUpdates"
```

---

### Task 4: App 层 ModifyToken

**Files:**
- Modify: `internal/app/token.go`
- Modify: `internal/app/token_test.go`

- [ ] **Step 1: 写 ModifyToken 测试**

在 `internal/app/token_test.go` 中新增测试，覆盖：
- 修改 scope
- 修改 name
- 修改 expires_in
- 已撤销 token 报错
- 已过期 token 报错
- PAT 不允许加 impersonate（通过 `ValidateTokenScopes` 剔除）
- 远程模式下 scope 子集约束

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现 ModifyToken**

在 `internal/app/token.go` 中新增：

```go
type ModifyTokenInput struct {
	TokenRef    string
	Scopes      []string
	Name        *string
	ExpiresIn   *time.Duration
	WorkspaceIDs []string
	ProjectIDs  []string
	ParentToken *TokenView
}
```

实现 `Service.ModifyToken`：

1. 通过 `store.TokenRepo().GetByIDOrPrefix(input.TokenRef)` 查找 token。
2. 检查 `RevokedAt` 不为 nil 则返回 `RuntimeError{Code: "token_revoked"}`。
3. 检查 `ExpiresAt` 不为 nil 且已过期则返回 `RuntimeError{Code: "token_expired"}`。
4. 如果提供了 `Scopes`：调用 `ParseScopes()` 展开，然后按 token type 剔除 `impersonate`（PAT）。
5. 如果远程模式且有 `ParentToken`：复用 `enforceTokenCreateLimit` 逻辑检查 scope/workspace/project 子集。
6. 如果提供了 `ExpiresIn`：
   - `*time.Duration` 值为 `0`：设置 `TokenUpdates.ClearExpiresAt = true`（永不过期）。
   - 正值：设置 `TokenUpdates.ExpiresAt` 为 `now + duration` 的 unix 秒。
   - 负值：返回 `RuntimeError{Code: "token_scope_invalid", Message: "expires-in must be non-negative"}`。
7. 合并修改项，构造 `storage.TokenUpdates`，调用 `store.TokenRepo().Update()`。
8. 写审计（action=`token.modified`，payload 含 before/after diff）。
9. 如果没有任何修改项（所有字段为 nil/空），直接查找并返回当前 `TokenView`，不写数据库和审计。

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/app/ -run "TestModifyToken" -v
```

- [ ] **Step 5: 运行全量测试**

```bash
go test ./...
```

- [ ] **Step 6: 提交**

```bash
git add internal/app/token.go internal/app/token_test.go
git commit -m "feat: app 层 ModifyToken"
```

---

## Chunk 3: CLI + HTTP API + 远程 CLI

### Task 5: `token modify` CLI 命令

**Files:**
- Modify: `internal/cli/token.go`

- [ ] **Step 1: 新增 `newTokenModifyCommand`**

在 `internal/cli/token.go` 中新增 `newTokenModifyCommand`，接受 flags：`--scope`、`--name`、`--expires-in`、`--workspace-id`、`--project`、`--project-id`。

支持远程模式：检测到 `--server` 时走 `client.ModifyToken()`（Task 6 会加这个方法）。

本地模式：调用 `svc.ModifyToken()`。

- [ ] **Step 2: 注册到 token 父命令**

在 `newTokenCommand` 中添加 `cmd.AddCommand(newTokenModifyCommand(opts))`。

- [ ] **Step 3: 构建验证**

```bash
go build ./cmd/xuanchu
./xuanchu token modify --help
```

- [ ] **Step 4: 提交**

```bash
git add internal/cli/token.go
git commit -m "feat: token modify CLI 命令"
```

---

### Task 6: 远程客户端 ModifyToken

**Files:**
- Modify: `internal/remote/client.go`

- [ ] **Step 1: 新增 `ModifyTokenInput` 和 `ModifyToken` 方法**

参考现有 `CreateToken`/`RevokeToken` 的模式。在 `internal/remote/client.go` 中新增：

```go
type ModifyTokenInput struct {
	Scopes       []string
	Name         *string
	ExpiresIn    *string
	WorkspaceIDs []string
	ProjectIDs   []string
}
```

`ModifyToken` 发送 `PATCH /api/v1/tokens/{id}`，JSON body 为 `ModifyTokenInput` 的非 nil 字段。返回 `TokenView`。

- [ ] **Step 2: 提交**

```bash
git add internal/remote/client.go
git commit -m "feat: 远程客户端 ModifyToken"
```

---

### Task 7: HTTP API `PATCH /api/v1/tokens/{id}`

**Files:**
- Modify: `internal/httpapi/tokens.go`
- Modify: `internal/httpapi/tokens_test.go`（如果存在）
- Modify: `docs/openapi/xuanchu-v1.yaml`

- [ ] **Step 1: 新增 PATCH handler**

在 `internal/httpapi/tokens.go` 中新增 `PATCH /api/v1/tokens/{id}` endpoint，解析 JSON body，调用 `svc.ModifyToken()`。

- [ ] **Step 2: 写集成测试**

测试覆盖：
- 成功修改 scope
- 成功修改 name
- 修改已撤销 token 返回 400 `token_revoked`
- 修改已过期 token 返回 400 `token_expired`
- 无 `token:write` capability 返回 403
- scope 扩张超出请求者自身 scope 返回 403

- [ ] **Step 3: 更新 OpenAPI**

在 `docs/openapi/xuanchu-v1.yaml` 中新增 `PATCH /api/v1/tokens/{id}` 的 schema 和 paths，参考现有 `POST /api/v1/tokens` 的结构。

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/httpapi/ -run "TestToken" -v
```

- [ ] **Step 5: 提交**

```bash
git add internal/httpapi/ docs/openapi/
git commit -m "feat: HTTP API PATCH /api/v1/tokens/{id}"
```

---

## Chunk 4: 文档与最终验证

### Task 8: 文档更新

**Files:**
- Modify: `docs/manual/reference/commands.md`
- Modify: `docs/manual/reference/errors.md`
- Modify: `docs/manual/remote-cli-and-api.md`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1: 更新命令速查**

在 `docs/manual/reference/commands.md` 中：

全局参数后补充 scope 通配符说明。

Server / Token / MCP 区域补充 `scope list` 和 `token modify`。

- [ ] **Step 2: 更新错误码**

在 `docs/manual/reference/errors.md` 中新增：

| 错误码 | 含义 |
|---|---|
| `token_revoked` | token 已撤销，不可修改 |
| `token_expired` | token 已过期，不可修改 |

- [ ] **Step 3: 更新远程 CLI 文档**

在 `docs/manual/remote-cli-and-api.md` 中补充 token modify API 说明。

- [ ] **Step 4: 新增 CLI 集成测试**

在 `tests/integration/cli_test.go` 中新增：

- `TestCLIScopeList` — 验证 `scope list` 输出包含 `task:read`。
- `TestCLIScopeListJSON` — 验证 `scope list --json` 输出合法 JSON。
- `TestCLITokenCreateWithWildcard` — 验证 `token create --scope '*'` 创建成功。
- `TestCLITokenModifyScope` — 验证 `token modify <id> --scope 'task:read'` 修改成功。
- `TestCLITokenModifyName` — 验证 `token modify <id> --name "new"` 修改成功。
- `TestCLITokenModifyExpiresIn` — 验证 `token modify <id> --expires-in 720h` 修改成功。

- [ ] **Step 5: 提交**

```bash
git add docs/manual/
git commit -m "docs: scope 通配符与 token modify 文档更新"
```

---

### Task 9: 最终验证

- [ ] **Step 1: 全量测试**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 2: 手动验收**

```bash
./xuanchu scope list
./xuanchu scope list --json
./xuanchu token create test-scope-wildcard --scope '*' --type pat --expires-in 1h
./xuanchu token create test-scope-wildcard-agent --scope '*' --type agent --expires-in 1h
./xuanchu token list
./xuanchu token modify <id> --scope '*:read'
./xuanchu token modify <id> --name "renamed"
./xuanchu token modify <id> --expires-in 720h
```

- [ ] **Step 3: 提交**

```bash
git add -A
git commit -m "chore: scope 通配符与 token modify 最终验证"
```
