# Xuanchu Server Admin Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现配置驱动的 server admin bootstrap 能力，让 Xuanchu Server 可以通过受限 REST 控制面创建 workspace，并为 workspace 创建 agent token。

**Architecture:** admin token 是 server control-plane verifier，不进入 `api_tokens`，只挂载 `/api/v1/admin/*`。配置解析产生 `config.AdminConfig`，HTTP server 注入该配置并使用独立 `adminAuthMiddleware`；admin handler 调用 `internal/app` 中的 admin bootstrap service，后者复用现有 user/workspace/membership/token repository 和校验逻辑。

**Tech Stack:** Go 1.25, Cobra, GORM, chi HTTP router, BurntSushi TOML, `crypto/sha256`, `crypto/subtle`

---

## 文件结构

新增文件：

- `internal/auth/admin_token.go`：生成 admin token、hash、verify。
- `internal/auth/admin_token_test.go`：admin token hash/verify 单元测试。
- `internal/config/admin.go`：admin TOML 配置结构、解析、校验。
- `internal/config/admin_test.go`：表数组、hash/hash_env、错误配置测试。
- `internal/app/admin_bootstrap.go`：创建 workspace、创建 workspace agent token 的 app 层方法。
- `internal/app/admin_bootstrap_test.go`：app 层行为和 audit 测试。
- `internal/app/runtime.go` / `internal/app/service.go`：`ServiceOptions` 增加 `DisableScopeBootstrap`，供 admin 控制面和 HTTP/MCP 内部鉴权服务使用，避免空 runtime 写入 `workspace_id=""` 的配置定义。
- `internal/httpapi/admin_auth.go`：admin 专用鉴权 middleware 和 context。
- `internal/httpapi/admin.go`：admin REST handler、请求/响应 DTO。
- `internal/httpapi/admin_test.go`：admin REST 权限边界和端到端测试。
- `internal/cli/admin.go`：`xuanchu admin token generate/hash` 命令。
- `internal/cli/admin_test.go`：CLI token helper 测试。

修改文件：

- `internal/config/config.go`：`Config` 增加 `ServerAdmin config.AdminConfig`，`Resolve` 读取 TOML/env。
- `internal/config/toml.go`：保留原 flatten 用于旧配置；admin 配置使用结构化 decode，不能用 flatten 表达 `[[server.admin.tokens]]`。
- `internal/httpapi/server.go`：`Options`/`Server` 增加 `Admin config.AdminConfig`。
- `internal/httpapi/router.go`：挂载 `/api/v1/admin/workspaces` 和 `/api/v1/admin/workspaces/{workspace}/agent-tokens`。
- `internal/cli/root.go`：注册 `admin` 命令。
- `internal/cli/server.go`：把 `cfg.ServerAdmin` 注入 `httpapi.NewServer`。
- `internal/app/workspace.go`：本计划不修改；admin bootstrap 在 `internal/app/admin_bootstrap.go` 内直接使用 `userRepo`、`workspaceRepo`、`memberRepo` 完成创建流程。
- `internal/app/token.go`：提取 `createTokenStored` 私有 helper，封装 token 生成、scope 校验、JSON 序列化、`tokenRepo.Create` 和 `CreatedToken` 组装；`CreateToken` 与 admin agent token 创建共同调用它。
- `internal/app/audit.go`：新增 `appendAdminAuditInTx` 私有 helper；该 helper 在 payload 写入 `admin: true`、`admin_token_name`，并用 `ActorUserID:nil` 写 audit。
- `docs/openapi/xuanchu-v1.yaml`：新增 admin endpoints。
- `config.example.toml`：新增 disabled admin 配置示例。
- `README.md` / `docs/deployment.md`：新增 bootstrap 使用说明。

首版审计采用 spec 中的方案 B：不扩 audit 表结构，在 payload 标记 `admin: true`、`admin_token_name`。原因是本轮核心目标是 bootstrap REST 能力；扩展 audit actor schema 会放大 HTTP/CLI/MCP 输出改动。后续若需要更干净语义，再单独设计 audit actor 类型迁移。

评审修正：

- admin handler 创建 `app.Service` 时必须设置 `DisableScopeBootstrap: true`；HTTP/MCP 中只用于鉴权或授权的空 runtime base service 也必须设置该选项。
- `admin_workspace_exists` 映射为 HTTP 409；workspace 唯一约束竞争也归一到该错误码。
- owner `name` 和 `email` 同时提供时，如果分别命中不同已有用户，返回 `admin_owner_invalid`。
- admin agent token 的非法 scope / project scope 复用现有 `token_scope_invalid` / `token_project_scope_invalid`。

---

## Chunk 1: Admin Token Hash 与配置解析

### Task 1: admin token hash/verify

**Files:**
- Create: `internal/auth/admin_token.go`
- Create: `internal/auth/admin_token_test.go`

- [ ] **Step 1: 写失败测试**

在 `internal/auth/admin_token_test.go` 写：

```go
package auth

import (
	"strings"
	"testing"
)

func TestAdminTokenHashAndVerify(t *testing.T) {
	raw := "xuanchu_admin_test_token_123"
	hash := HashAdminToken(raw)
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("hash = %q, want sha256 prefix", hash)
	}
	if !VerifyAdminToken(raw, hash) {
		t.Fatal("VerifyAdminToken(valid) = false")
	}
	if VerifyAdminToken(raw+"x", hash) {
		t.Fatal("VerifyAdminToken(invalid) = true")
	}
}

func TestGenerateAdminToken(t *testing.T) {
	raw, hash, err := GenerateAdminToken()
	if err != nil {
		t.Fatalf("GenerateAdminToken() error = %v", err)
	}
	if !strings.HasPrefix(raw, "xuanchu_admin_") {
		t.Fatalf("raw token = %q, want xuanchu_admin_ prefix", raw)
	}
	if !VerifyAdminToken(raw, hash) {
		t.Fatal("generated token does not verify")
	}
}

func TestVerifyAdminTokenRejectsUnsupportedHash(t *testing.T) {
	if VerifyAdminToken("token", "md5:bad") {
		t.Fatal("VerifyAdminToken(unsupported) = true")
	}
	if VerifyAdminToken("token", "sha256:not-hex") {
		t.Fatal("VerifyAdminToken(malformed) = true")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/auth -run TestAdminToken -count=1`

Expected: FAIL，提示 `HashAdminToken` / `GenerateAdminToken` 未定义。

- [ ] **Step 3: 实现 admin token helper**

在 `internal/auth/admin_token.go` 写：

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

const AdminTokenPrefix = "xuanchu_admin_"

func GenerateAdminToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = AdminTokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashAdminToken(raw), nil
}

func HashAdminToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func VerifyAdminToken(raw, verifier string) bool {
	verifier = strings.TrimSpace(verifier)
	if !strings.HasPrefix(verifier, "sha256:") {
		return false
	}
	wantHex := strings.TrimPrefix(verifier, "sha256:")
	want, err := hex.DecodeString(wantHex)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	sum := sha256.Sum256([]byte(raw))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/auth -run TestAdminToken -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/auth/admin_token.go internal/auth/admin_token_test.go
git commit -m "feat: 添加 admin token hash 工具"
```

### Task 2: 结构化解析 `[[server.admin.tokens]]`

**Files:**
- Create: `internal/config/admin.go`
- Create: `internal/config/admin_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/toml.go`

- [ ] **Step 1: 写失败测试**

在 `internal/config/admin_test.go` 写：

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveServerAdminConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
enabled = true
description = "primary"

[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = true
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{
		ConfigPath: path,
		HomeDir:    dir,
		Env: map[string]string{
			"XUANCHU_ADMIN_TOKEN_HASH": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !cfg.ServerAdmin.Enabled {
		t.Fatal("ServerAdmin.Enabled = false")
	}
	if len(cfg.ServerAdmin.Tokens) != 2 {
		t.Fatalf("tokens = %#v, want 2", cfg.ServerAdmin.Tokens)
	}
	if cfg.ServerAdmin.Tokens[1].Hash != "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("hash_env not resolved: %#v", cfg.ServerAdmin.Tokens[1])
	}
}

func TestResolveServerAdminRejectsDuplicateTokenName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops"
hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

[[server.admin.tokens]]
name = "ops"
hash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resolve(Options{ConfigPath: path, HomeDir: dir, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "duplicate admin token name") {
		t.Fatalf("Resolve() err = %v, want duplicate admin token name", err)
	}
}

func TestResolveServerAdminRejectsMissingHashEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops"
hash_env = "MISSING_HASH"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resolve(Options{ConfigPath: path, HomeDir: dir, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "MISSING_HASH") {
		t.Fatalf("Resolve() err = %v, want missing hash env", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config -run TestResolveServerAdmin -count=1`

Expected: FAIL，提示 `Config.ServerAdmin` 未定义。

- [ ] **Step 3: 新增配置结构**

在 `internal/config/admin.go` 写：

```go
package config

import (
	"fmt"
	"strings"
)

type AdminConfig struct {
	Enabled bool
	Tokens  []AdminTokenConfig
}

type AdminTokenConfig struct {
	Name        string
	Hash        string
	HashEnv     string
	Enabled     bool
	Description string
}

type adminTomlRoot struct {
	Server struct {
		Admin struct {
			Enabled bool `toml:"enabled"`
			Tokens  []struct {
				Name        string `toml:"name"`
				Hash        string `toml:"hash"`
				HashEnv     string `toml:"hash_env"`
				Enabled     *bool  `toml:"enabled"`
				Description string `toml:"description"`
			} `toml:"tokens"`
		} `toml:"admin"`
	} `toml:"server"`
}

func normalizeAdminConfig(raw adminTomlRoot, env map[string]string) (AdminConfig, error) {
	cfg := AdminConfig{Enabled: raw.Server.Admin.Enabled}
	if !cfg.Enabled {
		return cfg, nil
	}
	seen := map[string]struct{}{}
	for _, token := range raw.Server.Admin.Tokens {
		name := strings.TrimSpace(token.Name)
		if name == "" {
			return AdminConfig{}, fmt.Errorf("admin token name is required")
		}
		if _, ok := seen[name]; ok {
			return AdminConfig{}, fmt.Errorf("duplicate admin token name %q", name)
		}
		seen[name] = struct{}{}
		enabled := true
		if token.Enabled != nil {
			enabled = *token.Enabled
		}
		hash := strings.TrimSpace(token.Hash)
		hashEnv := strings.TrimSpace(token.HashEnv)
		if enabled {
			switch {
			case hash != "" && hashEnv != "":
				return AdminConfig{}, fmt.Errorf("admin token %q must use either hash or hash_env", name)
			case hash == "" && hashEnv == "":
				return AdminConfig{}, fmt.Errorf("admin token %q requires hash or hash_env", name)
			case hashEnv != "":
				value := strings.TrimSpace(env[hashEnv])
				if value == "" {
					return AdminConfig{}, fmt.Errorf("admin token %q hash_env %s is empty", name, hashEnv)
				}
				hash = value
			}
		}
		cfg.Tokens = append(cfg.Tokens, AdminTokenConfig{
			Name: name, Hash: hash, HashEnv: hashEnv, Enabled: enabled,
			Description: strings.TrimSpace(token.Description),
		})
	}
	return cfg, nil
}
```

- [ ] **Step 4: 修改 TOML 加载以返回结构化 admin 配置**

在 `internal/config/toml.go` 新增：

```go
func loadTomlAdminConfigFile(path string, env map[string]string) (AdminConfig, error) {
	var raw adminTomlRoot
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AdminConfig{}, os.ErrNotExist
		}
		return AdminConfig{}, err
	}
	return normalizeAdminConfig(raw, env)
}
```

在 `internal/config/config.go`：

- `Config` 增加 `ServerAdmin AdminConfig`。
- `Resolve` 记录实际 TOML path，并调用 `loadTomlAdminConfigFile(path, env)`。
- 如果没有 TOML，`ServerAdmin` 为零值 disabled。

实现时必须把当前三段 TOML 查找逻辑整理成下面的形状，`tomlValues` 继续沿用现有 flatten 配置读取结果：

```go
var tomlPath string
var tomlValues map[string]string
if opts.ConfigPath != "" {
	tomlPath = opts.ConfigPath
	values, err := loadTomlConfigFile(tomlPath)
	if err != nil {
		return Config{}, err
	}
	tomlValues = values
} else if configPath := env["XUANCHU_CONFIG"]; configPath != "" {
	tomlPath = configPath
	values, err := loadTomlConfigFile(tomlPath)
	if err != nil {
		return Config{}, err
	}
	tomlValues = values
} else if path, values, err := loadTomlConfigWithPath(configDir(home, env)); err == nil {
	tomlPath = path
	tomlValues = values
} else if !errors.Is(err, os.ErrNotExist) {
	return Config{}, err
}
```

然后：

```go
var adminCfg AdminConfig
if tomlPath != "" {
	adminCfg, err = loadTomlAdminConfigFile(tomlPath, env)
	if err != nil {
		return Config{}, err
	}
}
```

- [ ] **Step 5: 运行配置测试**

Run: `go test ./internal/config -run TestResolveServerAdmin -count=1`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/config/admin.go internal/config/admin_test.go internal/config/config.go internal/config/toml.go
git commit -m "feat: 解析 server admin 配置"
```

---

## Chunk 2: CLI Admin Token Helper

### Task 3: `xuanchu admin token generate/hash`

**Files:**
- Create: `internal/cli/admin.go`
- Create: `internal/cli/admin_test.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: 写失败测试**

在 `internal/cli/admin_test.go` 写：

```go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
)

func TestAdminTokenGenerateCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	cmd.SetArgs([]string{"admin", "token", "generate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "token: xuanchu_admin_") || !strings.Contains(out, "hash: sha256:") {
		t.Fatalf("output = %q", out)
	}
}

func TestAdminTokenHashCommandReadsStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	raw := "xuanchu_admin_test_token"
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(raw)})
	cmd.SetArgs([]string{"admin", "token", "hash"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v stderr=%s", err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != auth.HashAdminToken(raw) {
		t.Fatalf("hash = %q, want %q", got, auth.HashAdminToken(raw))
	}
}
```

本任务必须给 `Options` 新增 `Stdin io.Reader`，`hash` 命令只从 stdin 读取 token。不要增加 `--token` 明文 flag，避免把 admin token 写入 shell history。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cli -run TestAdminToken -count=1`

Expected: FAIL，提示 admin 命令或 `Options.Stdin` 未定义。

- [ ] **Step 3: 实现 CLI 命令**

在 `internal/cli/admin.go` 写：

```go
package cli

import (
	"fmt"
	"io"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"github.com/spf13/cobra"
)

func newAdminCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "管理 server admin bootstrap 工具"}
	tokenCmd := &cobra.Command{Use: "token", Short: "生成或计算 admin token hash"}
	tokenCmd.AddCommand(newAdminTokenGenerateCommand(opts))
	tokenCmd.AddCommand(newAdminTokenHashCommand(opts))
	cmd.AddCommand(tokenCmd)
	return cmd
}

func newAdminTokenGenerateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "generate",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, hash, err := auth.GenerateAdminToken()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "token: %s\nhash: %s\n", raw, hash)
			return nil
		},
	}
}

func newAdminTokenHashCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "hash",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := opts.Stdin
			if in == nil {
				in = cmd.InOrStdin()
			}
			data, err := io.ReadAll(in)
			if err != nil {
				return err
			}
			raw := strings.TrimSpace(string(data))
			if raw == "" {
				return fmt.Errorf("admin token is required on stdin")
			}
			fmt.Fprintln(cmd.OutOrStdout(), auth.HashAdminToken(raw))
			return nil
		},
	}
}
```

在 `internal/cli/root.go`：

- `Options` 增加 `Stdin io.Reader`。
- root command 设置 stdin。
- `cmd.AddCommand(newAdminCommand(opts))`。

- [ ] **Step 4: 运行 CLI 测试**

Run: `go test ./internal/cli -run TestAdminToken -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/cli/admin.go internal/cli/admin_test.go internal/cli/root.go
git commit -m "feat: 添加 admin token CLI 工具"
```

---

## Chunk 3: HTTP Admin Auth 边界

### Task 4: admin auth middleware

**Files:**
- Create: `internal/httpapi/admin_auth.go`
- Create: `internal/httpapi/admin_test.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/cli/server.go`

- [ ] **Step 1: 写失败测试：disabled / missing / invalid**

在 `internal/httpapi/admin_test.go` 写第一组测试：

```go
package httpapi

import (
	"net/http"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/config"
)

func TestAdminEndpointDisabled(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, nil)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "route_not_found")
}

func TestAdminEndpointRequiresAdminToken(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens: []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken("secret"), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, nil)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_required")
}

func TestAdminEndpointRejectsInvalidAdminToken(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens: []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken("secret"), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, map[string]string{
		"Authorization": "Bearer wrong",
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_invalid")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi -run TestAdminEndpoint -count=1`

Expected: FAIL，提示 `Server.admin` 未定义或 route 不存在。

- [ ] **Step 3: 修改 Server options**

在 `internal/httpapi/server.go`：

```go
type Options struct {
	Store          *storage.Store
	Clock          app.Clock
	Stderr         io.Writer
	BodyLimitBytes int64
	TestPanicRoute bool
	Logger         *logging.Logger
	Admin config.AdminConfig
}

type Server struct {
	store          *storage.Store
	clock          app.Clock
	stderr         io.Writer
	bodyLimitBytes int64
	testPanicRoute bool
	logger         *logging.Logger
	router         *http.ServeMux
	admin config.AdminConfig
}
```

`NewServer` 赋值 `admin: opts.Admin`。

在 `internal/cli/server.go` 的 `httpapi.NewServer` 注入：

```go
Admin: cfg.ServerAdmin,
```

- [ ] **Step 4: 实现 admin middleware**

在 `internal/httpapi/admin_auth.go` 写：

```go
package httpapi

import (
	"context"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type adminAuthInfo struct {
	TokenName string
}

type adminAuthContextKey struct{}

func adminAuthFromContext(ctx context.Context) (adminAuthInfo, bool) {
	v, ok := ctx.Value(adminAuthContextKey{}).(adminAuthInfo)
	return v, ok
}

func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.admin.Enabled {
			writeError(w, http.StatusNotFound, "route_not_found", "route not found", nil)
			return
		}
		raw, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "admin_auth_required", "admin token is required", nil)
			return
		}
		for _, token := range s.admin.Tokens {
			if !token.Enabled {
				continue
			}
			if auth.VerifyAdminToken(raw, token.Hash) {
				ctx := context.WithValue(r.Context(), adminAuthContextKey{}, adminAuthInfo{TokenName: token.Name})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		writeError(w, http.StatusUnauthorized, "admin_auth_invalid", "admin token is invalid", nil)
	})
}

```

复用 `internal/httpapi/middleware.go` 已有的 `bearerToken(header string) (string, bool)`，不要在 `admin_auth.go` 里定义同名函数，也不要新增另一个 Bearer parser。

- [ ] **Step 5: 挂载占位路由**

在 `internal/httpapi/router.go`：

```go
api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces", func(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "admin_not_implemented", "admin endpoint is not implemented", nil)
})
```

注意：disabled 时应该仍然表现为 404，避免暴露 admin bootstrap 状态。

- [ ] **Step 6: 运行 admin auth 测试**

Run: `go test ./internal/httpapi -run TestAdminEndpoint -count=1`

Expected: PASS 或仅正确 token 场景返回 `admin_not_implemented`；本任务只覆盖 disabled/missing/invalid。

- [ ] **Step 7: 提交**

```bash
git add internal/httpapi/admin_auth.go internal/httpapi/admin_test.go internal/httpapi/server.go internal/httpapi/router.go internal/cli/server.go
git commit -m "feat: 添加 server admin HTTP 鉴权"
```

---

## Chunk 4: App Admin Bootstrap Service

### Task 5: app 层创建 workspace

**Files:**
- Create: `internal/app/admin_bootstrap.go`
- Create: `internal/app/admin_bootstrap_test.go`
- 不修改：`internal/app/workspace.go`

- [ ] **Step 1: 写失败测试**

在 `internal/app/admin_bootstrap_test.go` 写：

```go
package app

import (
	"encoding/json"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestAdminCreateWorkspaceCreatesOwnerMembership(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Runtime: &RuntimeContext{ActorName: "server-admin"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Name:           "Dajee",
		Visibility:     "team",
		Owner:          AdminOwnerInput{Name: "alice", Email: "alice@example.com"},
	})
	if err != nil {
		t.Fatalf("AdminCreateWorkspace() error = %v", err)
	}
	if result.Workspace.Slug != "dajee" || result.Owner.Name != "alice" {
		t.Fatalf("result = %#v", result)
	}
	member, err := storage.NewMemberRepository(store.DB()).Get(result.Owner.ID, result.Workspace.ID)
	if err != nil {
		t.Fatalf("membership missing: %v", err)
	}
	if member.Role != string(RoleOwner) {
		t.Fatalf("role = %q, want owner", member.Role)
	}
}

func TestAdminCreateWorkspaceWritesAuditWithoutRawToken(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Runtime: &RuntimeContext{ActorName: "server-admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "auditws",
		Owner:          AdminOwnerInput{Name: "alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].Action != "admin.workspace.create" {
		t.Fatalf("audit rows = %#v", rows)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["admin_token_name"] != "ops" || payload["admin"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["token"]; ok {
		t.Fatalf("payload leaked token: %#v", payload)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run TestAdminCreateWorkspace -count=1`

Expected: FAIL，提示 admin 方法未定义。

- [ ] **Step 3: 实现 admin workspace 创建**

在 `internal/app/admin_bootstrap.go` 写：

```go
package app

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type AdminOwnerInput struct {
	Name  string
	Email string
}

type AdminCreateWorkspaceInput struct {
	AdminTokenName string
	Slug           string
	Name           string
	Description    string
	Visibility     string
	Owner          AdminOwnerInput
}

type AdminCreateWorkspaceResult struct {
	Workspace WorkspaceView
	Owner     UserView
}

func (s *Service) AdminCreateWorkspace(input AdminCreateWorkspaceInput) (AdminCreateWorkspaceResult, error) {
	// normalize input, then create user/workspace/membership in one transaction
}
```

实现要点：

- `Owner.Name` 必填。
- 先按 email 查 user；email 空时按 name 查；不存在则创建。
- workspace slug 冲突返回 `RuntimeError{Code:"admin_workspace_exists"}`。
- workspace 创建时 `CreatedByUserID` 设为 owner.ID。
- owner membership role = owner。
- 调用 `ensureBuiltinConfigDefinitions(workspace.ID)`。
- audit 使用 `admin.workspace.create`，业务 payload 只包含业务对象字段；`admin` 与 `admin_token_name` 由 `appendAdminAuditInTx` 统一补入：

```go
map[string]any{
	"owner_user_id": owner.ID,
	"workspace_slug": workspace.Slug,
}
```

新增 `internal/app/audit.go` helper，admin bootstrap 必须使用该 helper，不能调用 `withAudit`，也不能伪造超管 user：

```go
func (s *Service) appendAdminAuditInTx(tx *Service, entry AuditEntry, adminTokenName string) error {
	payload := map[string]any{}
	for key, value := range entry.Payload {
		payload[key] = value
	}
	payload["admin"] = true
	payload["admin_token_name"] = adminTokenName
	raw, err := marshalAuditPayload(payload)
	if err != nil {
		return err
	}
	workspaceID := entry.WorkspaceID
	return tx.auditRepo.Append(storage.AuditLogEntry{
		ActorUserID: nil,
		WorkspaceID: workspaceID,
		ProjectID:   entry.ProjectID,
		Action:      entry.Action,
		TargetType:  entry.TargetType,
		TargetID:    entry.TargetID,
		PayloadJSON: raw,
		CreatedAt:   tx.clock.Unix(),
	})
}
```

`AdminCreateWorkspace` 在 `s.store.Transaction` 中创建 user/workspace/membership/config definitions，然后调用 `txSvc.appendAdminAuditInTx(txSvc, entry, input.AdminTokenName)`。首版允许 `ActorUserID=nil`，payload 是 admin 来源的唯一标记。

- [ ] **Step 4: 运行 app 测试**

Run: `go test ./internal/app -run TestAdminCreateWorkspace -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/app/admin_bootstrap.go internal/app/admin_bootstrap_test.go internal/app/audit.go
git commit -m "feat: 添加 admin 创建 workspace 服务"
```

### Task 6: app 层创建 workspace agent token

**Files:**
- Modify: `internal/app/admin_bootstrap.go`
- Modify: `internal/app/admin_bootstrap_test.go`
- Modify: `internal/app/token.go`

- [ ] **Step 1: 写失败测试**

在 `internal/app/admin_bootstrap_test.go` 追加：

```go
func TestAdminCreateWorkspaceAgentToken(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Runtime: &RuntimeContext{ActorName: "server-admin"}})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "alice", Email: "alice@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AdminCreateWorkspaceAgentToken(AdminCreateAgentTokenInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "openclaw",
		UserRef:        "alice@example.com",
		Scopes:         []string{"task:read", "task:write"},
	})
	if err != nil {
		t.Fatalf("AdminCreateWorkspaceAgentToken() error = %v", err)
	}
	if created.RawToken == "" || created.View.Type != "agent" {
		t.Fatalf("created = %#v", created)
	}
	if len(created.View.WorkspaceIDs) != 1 || created.View.WorkspaceIDs[0] != ws.Workspace.ID {
		t.Fatalf("workspace ids = %#v, want %s", created.View.WorkspaceIDs, ws.Workspace.ID)
	}
}

func TestAdminCreateWorkspaceAgentTokenRejectsNonMember(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Runtime: &RuntimeContext{ActorName: "server-admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddUser(AddUserInput{Name: "bob"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.AdminCreateWorkspaceAgentToken(AdminCreateAgentTokenInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "bad",
		UserRef:        "bob",
		Scopes:         []string{"task:read"},
	})
	if err == nil {
		t.Fatal("AdminCreateWorkspaceAgentToken(non-member) error = nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run TestAdminCreateWorkspaceAgentToken -count=1`

Expected: FAIL，方法未定义。

- [ ] **Step 3: 实现 agent token 创建**

在 `internal/app/admin_bootstrap.go` 添加：

```go
type AdminCreateAgentTokenInput struct {
	AdminTokenName string
	WorkspaceRef   string
	Name           string
	UserRef        string
	Scopes         []string
	ProjectRefs    []string
	ExpiresIn      *time.Duration
}
```

实现要点：

- 解析 workspace，必须未归档。
- `UserRef` 为空时，通过 `memberRepo.List(workspace.ID)` 查 role=`owner` 的成员；由于现有 `MemberRepository.List` 按 `users.name ASC` 排序，若有多个 owner，选择排序后的第一个，保证确定性；没有 owner 返回 `admin_owner_required`。
- user 必须是 workspace member。
- 只创建 `auth.TokenTypeAgent`。
- workspace refs 强制为目标 workspace ID。
- project refs 必须属于目标 workspace。
- 不直接调用 `CreateToken`。从 `CreateToken` 提取 `createTokenStored` helper，admin 方法直接调用该 helper，绕开普通 runtime actor 的 membership/role 判断，但仍复用 `auth.ValidateTokenCreate`、`auth.GenerateToken`、`tokenRepo.Create`、`marshalStringSlice`、`tokenViewFromEntry`。
- audit `admin.agent_token.create` payload 包含 scopes/workspace_id/project_ids/admin token name，不含 raw token。

`internal/app/token.go` 必须提取以下私有 helper：

```go
type createTokenStoredInput struct {
	Name         string
	TokenType    string
	UserID       string
	Scopes       []string
	WorkspaceIDs []string
	ProjectIDs   []string
	ExpiresIn    *time.Duration
}

func (s *Service) createTokenStored(input createTokenStoredInput) (CreatedToken, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CreatedToken{}, RuntimeError{Code: "token_name_required", Message: "token name is required"}
	}
	tokenType := strings.TrimSpace(input.TokenType)
	if tokenType == "" {
		tokenType = auth.TokenTypePAT
	}
	scopes, err := auth.ValidateTokenCreate(auth.CreateTokenOptions{
		Type:         tokenType,
		Scopes:       input.Scopes,
		WorkspaceIDs: input.WorkspaceIDs,
	})
	if err != nil {
		return CreatedToken{}, classifyTokenCreateError(err)
	}
	raw, prefix, hash, err := auth.GenerateToken(tokenType)
	if err != nil {
		return CreatedToken{}, err
	}
	createdAt := s.clock.Unix()
	var expiresAt *int64
	if input.ExpiresIn != nil {
		value := createdAt + int64(input.ExpiresIn.Seconds())
		expiresAt = &value
	}
	scopesJSON, err := marshalStringSlice(scopes.Values())
	if err != nil {
		return CreatedToken{}, err
	}
	workspaceJSON, err := marshalStringSlice(input.WorkspaceIDs)
	if err != nil {
		return CreatedToken{}, err
	}
	projectJSON, err := marshalStringSlice(input.ProjectIDs)
	if err != nil {
		return CreatedToken{}, err
	}
	stored := storage.ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           input.UserID,
		Name:             name,
		Type:             tokenType,
		TokenPrefix:      prefix,
		TokenHash:        hash,
		ScopesJSON:       scopesJSON,
		WorkspaceIDsJSON: workspaceJSON,
		ProjectIDsJSON:   projectJSON,
		CreatedAt:        createdAt,
		ExpiresAt:        expiresAt,
	}
	if err := s.tokenRepo.Create(stored); err != nil {
		return CreatedToken{}, err
	}
	return CreatedToken{
		RawToken: raw,
		View:     tokenViewFromEntry(stored, scopes.Values(), input.WorkspaceIDs, input.ProjectIDs),
		Stored:   stored,
	}, nil
}
```

`CreateToken` 仍负责普通用户路径的 `resolveTokenTargetUser`、`resolveTokenWorkspaces`、`resolveTokenProjects`、`tokenManageAllowed` 和 `enforceTokenCreateLimit`；进入 `withAudit("token.create", ...)` 后调用 `tx.createTokenStored(...)` 并写原有 `token.create` audit。`AdminCreateWorkspaceAgentToken` 在自己的 transaction 中解析 workspace/user/project/member 后调用 `txSvc.createTokenStored(...)`，随后调用 `txSvc.appendAdminAuditInTx(...)` 写 `admin.agent_token.create` audit。

- [ ] **Step 4: 运行 app 测试**

Run: `go test ./internal/app -run TestAdminCreateWorkspaceAgentToken -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/app/admin_bootstrap.go internal/app/admin_bootstrap_test.go internal/app/token.go
git commit -m "feat: 添加 admin 创建 agent token 服务"
```

---

## Chunk 5: Admin REST Endpoints

### Task 7: `POST /api/v1/admin/workspaces`

**Files:**
- Create: `internal/httpapi/admin.go`
- Modify: `internal/httpapi/admin_test.go`
- Modify: `internal/httpapi/router.go`

- [ ] **Step 1: 写失败测试**

在 `internal/httpapi/admin_test.go` 添加：

```go
func TestAdminCreateWorkspaceHTTP(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens: []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	body := `{"slug":"dajee","name":"Dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"slug":"dajee"`) || !strings.Contains(rr.Body.String(), `"name":"alice"`) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestAdminTokenCannotAccessNormalAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write", "task:read")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens: []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_invalid_token")
}

func TestNormalTokenCannotAccessAdminAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens: []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_invalid")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi -run TestAdmin -count=1`

Expected: FAIL，handler 未实现或返回 501。

- [ ] **Step 3: 实现 handler**

在 `internal/httpapi/admin.go` 写：

```go
package httpapi

import (
	"encoding/json"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type adminCreateWorkspaceRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Owner       struct {
		Name  string `json:"name"`
		Email string `json:"email,omitempty"`
	} `json:"owner"`
}

func (s *Server) handleAdminWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := app.NewService(app.ServiceOptions{Store: s.store, Clock: s.effectiveClock(), Runtime: &app.RuntimeContext{ActorName: "server-admin"}})
	if err != nil {
		writeAppError(w, err)
		return
	}
	result, err := svc.AdminCreateWorkspace(app.AdminCreateWorkspaceInput{
		AdminTokenName: admin.TokenName,
		Slug: req.Slug, Name: req.Name, Description: req.Description, Visibility: req.Visibility,
		Owner: app.AdminOwnerInput{Name: req.Owner.Name, Email: req.Owner.Email},
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, map[string]any{
		"workspace": workspaceResponseFromView(result.Workspace),
		"owner": task.UserInfoToJSON(task.UserInfo{ID: result.Owner.ID, Name: result.Owner.Name, Email: result.Owner.Email, ExternalIDs: result.Owner.ExternalIDs}),
	}, nil)
}
```

在 `router.go` 替换占位路由：

```go
api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces", s.handleAdminWorkspaceCreate)
```

- [ ] **Step 4: 运行 HTTP admin 测试**

Run: `go test ./internal/httpapi -run TestAdmin -count=1`

Expected: 当前 workspace 创建相关测试 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/httpapi/admin.go internal/httpapi/admin_test.go internal/httpapi/router.go
git commit -m "feat: 添加 admin workspace REST 接口"
```

### Task 8: `POST /api/v1/admin/workspaces/{workspace}/agent-tokens`

**Files:**
- Modify: `internal/httpapi/admin.go`
- Modify: `internal/httpapi/admin_test.go`
- Modify: `internal/httpapi/router.go`

- [ ] **Step 1: 写失败测试**

在 `internal/httpapi/admin_test.go` 添加：

```go
func TestAdminCreateWorkspaceAgentTokenHTTP(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write", "token:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens: []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	createWS := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := `{"name":"openclaw","user":"alice@example.com","scopes":["task:read","task:write"],"expires_in":"24h"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/agent-tokens", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create token status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"token":"xuanchu_agent_`) || !strings.Contains(rr.Body.String(), `"workspace_ids"`) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi -run TestAdminCreateWorkspaceAgentTokenHTTP -count=1`

Expected: FAIL，route 或 handler 未实现。

- [ ] **Step 3: 实现 handler**

在 `internal/httpapi/admin.go` 添加：

```go
type adminCreateAgentTokenRequest struct {
	Name             string   `json:"name"`
	User             string   `json:"user,omitempty"`
	Scopes           []string `json:"scopes"`
	ProjectRefs      []string `json:"project_refs,omitempty"`
	ProjectIDs       []string `json:"project_ids,omitempty"`
	ExpiresIn        string   `json:"expires_in,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

func (s *Server) handleAdminAgentTokenCreate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateAgentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	var ttl *time.Duration
	if req.ExpiresIn != "" {
		value, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			writeError(w, http.StatusBadRequest, "admin_token_ttl_invalid", "expires_in is invalid", nil)
			return
		}
		ttl = &value
	} else if req.ExpiresInSeconds != nil {
		value := time.Duration(*req.ExpiresInSeconds) * time.Second
		ttl = &value
	}
	admin, _ := adminAuthFromContext(r.Context())
	svc, err := app.NewService(app.ServiceOptions{Store: s.store, Clock: s.effectiveClock(), Runtime: &app.RuntimeContext{ActorName: "server-admin"}})
	if err != nil {
		writeAppError(w, err)
		return
	}
	projectRefs := append([]string(nil), req.ProjectRefs...)
	projectRefs = append(projectRefs, req.ProjectIDs...)
	created, err := svc.AdminCreateWorkspaceAgentToken(app.AdminCreateAgentTokenInput{
		AdminTokenName: admin.TokenName,
		WorkspaceRef: chi.URLParam(r, "workspace"),
		Name: req.Name, UserRef: req.User, Scopes: req.Scopes, ProjectRefs: projectRefs, ExpiresIn: ttl,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, createdTokenResponse{
		Token: created.RawToken,
		tokenResponse: tokenResponseFromView(created.View),
	}, nil)
}
```

在 `router.go` 增加：

```go
api.With(s.adminAuthMiddleware).Post("/api/v1/admin/workspaces/{workspace}/agent-tokens", s.handleAdminAgentTokenCreate)
```

- [ ] **Step 4: 运行 HTTP admin 测试**

Run: `go test ./internal/httpapi -run TestAdmin -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/httpapi/admin.go internal/httpapi/admin_test.go internal/httpapi/router.go
git commit -m "feat: 添加 admin agent token REST 接口"
```

---

## Chunk 6: 文档、OpenAPI、完整验证

### Task 9: 文档与示例配置

**Files:**
- Modify: `config.example.toml`
- Modify: `README.md`
- Modify: `docs/deployment.md`
- Modify: `docs/openapi/xuanchu-v1.yaml`

- [ ] **Step 1: 更新 `config.example.toml`**

追加：

```toml
[server.admin]
enabled = false

# 生产建议使用 hash 或 hash_env，不要长期保存明文 token。
# 生成方式：
#   xuanchu admin token generate

[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:replace-with-token-hash"
enabled = false
description = "server bootstrap token"

[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = false
description = "rotation token hash from deployment secret"
```

- [ ] **Step 2: 更新 README / deployment**

说明：

- admin token 是 bootstrap/control-plane token。
- 配置保存 hash，重启后不会丢。
- 明文丢失只能轮换。
- admin token 不能访问普通 API。
- 普通 API token 不能访问 admin API。

- [ ] **Step 3: 更新 OpenAPI**

新增：

```yaml
/api/v1/admin/workspaces:
  post:
    summary: Create workspace via server admin bootstrap token.
/api/v1/admin/workspaces/{workspace}/agent-tokens:
  post:
    summary: Create workspace-scoped agent token via server admin bootstrap token.
```

安全说明写明使用 `Authorization: Bearer <server-admin-token>`，但不是普通 API token。

- [ ] **Step 4: 文档 grep 检查**

Run:

```bash
rg -n "server.admin|admin token|admin bootstrap|/api/v1/admin" README.md docs config.example.toml
```

Expected: 能看到新增说明；没有把 admin token 描述成普通业务超管 token。

- [ ] **Step 5: 提交**

```bash
git add config.example.toml README.md docs/deployment.md docs/openapi/xuanchu-v1.yaml
git commit -m "docs: 补充 server admin bootstrap 文档"
```

### Task 10: 完整验证

**Files:**
- All changed files

- [ ] **Step 1: 运行 targeted tests**

```bash
go test ./internal/auth -run TestAdminToken -count=1
go test ./internal/config -run TestResolveServerAdmin -count=1
go test ./internal/cli -run TestAdminToken -count=1
go test ./internal/app -run TestAdminCreate -count=1
go test ./internal/httpapi -run TestAdmin -count=1
```

Expected: all PASS。

- [ ] **Step 2: 运行完整测试**

```bash
go test ./...
```

Expected: all PASS。

- [ ] **Step 3: 运行 CGO=0 测试**

```bash
CGO_ENABLED=0 go test ./...
```

Expected: all PASS。

- [ ] **Step 4: 运行 CGO=0 build**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: exit 0。

- [ ] **Step 5: 检查 diff 和状态**

```bash
git status --short
git diff --check
```

Expected: 无 whitespace error；只有本功能相关文件。

- [ ] **Step 6: 最终提交**

如果前面任务没有逐步提交，最后统一提交：

```bash
git add internal/auth internal/config internal/cli internal/app internal/httpapi README.md docs config.example.toml
git commit -m "feat: 添加 server admin bootstrap"
```

---

## 实施注意事项

- 不要把 admin token 写入 `api_tokens`。
- 不要让 admin token 通过普通 `authMiddleware`。
- 不要让普通 token 通过 admin endpoint。
- 不要在 audit、日志、错误响应中输出 raw admin token 或 raw agent token。agent raw token 只能在创建响应里出现一次。
- 如果需要为了 admin 创建 token 提取 app helper，优先提取现有逻辑，不复制 token hash/scopes/workspace/project 解析逻辑。
- 首版 audit 使用 payload 标记 admin 是有意取舍；不要顺手扩 audit schema，除非先更新 spec 和 plan。
- 所有文档和注释以中文为主。
