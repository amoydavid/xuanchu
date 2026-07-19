# 资源绝对 URL 输出 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 通过 `server.public_base_url` 为项目、任务、循环实例和循环系列生成跨 HTTP、MCP、CLI、Remote 一致的 Web Console 绝对 URL；配置为空时稳定返回空字符串。

**Architecture:** `internal/config` 校验并规范化部署级 public origin，再与 `server.console.base_path` 合成唯一 `ResourceBaseURL()`。该值沿 CLI、HTTP Server、MCP Server 注入 `app.ServiceOptions.ResourceBaseURL`；App 继续拥有资源 path segment 编码和 view 装配，所有输出层只透传 `URL`。

**Tech Stack:** Go 1.25、Cobra、Huma/OpenAPI、MCP Go SDK、React/TypeScript、Vitest、Playwright、GORM、纯 Go SQLite、PostgreSQL/pgx、deployer、systemd、nginx。

## Global Constraints

- `url` 只能是空字符串或包含 scheme 与 host 的绝对 HTTP(S) URL，禁止退化为相对路径。
- `server.public_base_url` 为空是合法配置，资源读取成功且返回 `url: ""`。
- `server.public_base_url` 只允许 HTTP(S) origin；禁止 userinfo、query、fragment 和除 `/` 外的 path。
- `Config.ResourceBaseURL()` 负责组合 `server.public_base_url` 与 `server.console.base_path`。
- 不从请求 Host、转发头、监听地址、`remote.server` 或 `sso.external_base_url` 推导 canonical origin。
- App 层继续独占资源路径和 segment 编码；HTTP、MCP、CLI、Remote 不自行拼 URL。
- projected occurrence 使用编码后的 `occurrence_ref`，materialized occurrence 使用 `task_slug`；series 继续使用 `/series/{seriesSlug}`。
- 文档和注释以中文为主；SQLite 保持零 CGO。
- 功能分支验证后必须先合并回 `main`，再修改线上共享配置并部署。

---

### Task 1: 配置层建立 public base URL 契约

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/runtime.go`
- Modify: `config.example.toml`

**Interfaces:**
- Produces: `Config.PublicBaseURL string`、`Config.ResourceBaseURL() string`、`ValidatePublicBaseURL(string) (string, error)`。
- Consumes: TOML `server.public_base_url`、环境变量 `XUANCHU_PUBLIC_BASE_URL`、`ConsoleConfig.BasePath`。

- [ ] **Step 1: 写配置失败测试**

在 `internal/config/config_test.go` 增加表驱动测试，断言 TOML 能读取 `https://xuanchu.example.com/` 并规范化为无尾 `/`；环境变量覆盖 TOML；空值合法；`ftp://`、相对 URL、带 userinfo/query/fragment/path 的值返回包含 `server.public_base_url` 的错误；`ResourceBaseURL()` 分别得到 `""`、`https://xuanchu.example.com`、`https://xuanchu.example.com/admin`。

- [ ] **Step 2: 运行测试并确认 RED**

Run: `go test ./internal/config -run 'TestResolvePublicBaseURL|TestConfigResourceBaseURL' -count=1`

Expected: FAIL，编译错误包含 `PublicBaseURL undefined` 或 `ResourceBaseURL undefined`。

- [ ] **Step 3: 最小实现配置解析与合成**

在 `Config` 增加：

```go
PublicBaseURL string

func (c Config) ResourceBaseURL() string {
    if c.PublicBaseURL == "" {
        return ""
    }
    if c.Console.BasePath == "/" {
        return c.PublicBaseURL
    }
    return c.PublicBaseURL + c.Console.BasePath
}
```

`Resolve` 按 `XUANCHU_PUBLIC_BASE_URL` > TOML `server.public_base_url` 解析并调用验证器；`runtimeEnvValues` 映射该环境变量。验证器用 `net/url` 保证 scheme/host 与 origin-only 约束，并删除尾部 `/`。

- [ ] **Step 4: 运行配置测试并确认 GREEN**

Run: `go test ./internal/config -run 'TestResolvePublicBaseURL|TestConfigResourceBaseURL' -count=1`

Expected: PASS。

---

### Task 2: App 层生成绝对 URL 或空字符串

**Files:**
- Modify: `internal/app/resource_url.go`
- Modify: `internal/app/resource_url_test.go`
- Modify: `internal/app/runtime.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/project.go`
- Modify: `internal/app/task_occurrence.go`
- Modify: `internal/app/task_occurrence_test.go`
- Modify: `internal/app/task_reference.go`
- Modify: `internal/app/task_series.go`
- Modify: `internal/app/task_series_test.go`

**Interfaces:**
- Consumes: `ServiceOptions.ResourceBaseURL`。
- Produces: `ProjectURL(resourceBaseURL, workspaceSlug, projectSlug string)`、`ProjectTaskURL(resourceBaseURL, workspaceSlug, projectSlug, taskRef string)`、`StandaloneTaskURL(resourceBaseURL, taskRef string)`、`TaskSeriesURL(resourceBaseURL, workspaceSlug, projectSlug, seriesSlug string)`。

- [ ] **Step 1: 把 URL 单测改为期望绝对 URL，并增加空配置用例**

示例断言：

```go
base := "https://xuanchu.example.com/admin"
if got := ProjectTaskURL(base, "dajee", "agentapi", "occ:series:1"); got != "https://xuanchu.example.com/admin/workspaces/dajee/projects/agentapi/tasks/occ%3Aseries%3A1" {
    t.Fatalf("URL = %q", got)
}
if got := ProjectURL("", "dajee", "agentapi"); got != "" {
    t.Fatalf("URL = %q, want empty", got)
}
```

真实 App view 测试分别用 `ServiceOptions{ResourceBaseURL: base}` 断言绝对 URL，并用默认空配置断言 `URL == ""`。

- [ ] **Step 2: 运行测试并确认 RED**

Run: `go test ./internal/app -run 'TestResourceURLs|TestAppViewsExposeCanonicalURLs|TestListTaskSeriesReturnsCreated' -count=1`

Expected: FAIL，现有函数签名不接收 base URL，或返回相对路径。

- [ ] **Step 3: 最小实现 App 注入和 URL 构造**

`ServiceOptions` 与 `Service` 增加 `ResourceBaseURL string`。四个 URL 函数先判断 base 为空，再在 base 后追加原 canonical path；更新 project/task/series 所有 mapper 调用点传入 `s.resourceBaseURL`。事务克隆沿用 `clone := *s` 自动保留该字段。

- [ ] **Step 4: 运行 App 测试并确认 GREEN**

Run: `go test ./internal/app -run 'TestResourceURLs|TestAppViewsExposeCanonicalURLs|TestListTaskSeriesReturnsCreated|TestProjectedOccurrenceViewDoesNotAllocateIdentity' -count=1`

Expected: PASS。

---

### Task 3: 把 ResourceBaseURL 注入 CLI、HTTP 与 MCP

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/mcp.go`
- Modify: `internal/cli/server.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/app_service.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/mcpserver/options.go`
- Modify: `internal/mcpserver/auth.go`
- Modify: `internal/mcpserver/auth_test.go`
- Modify: `internal/httpapi/projects_test.go`
- Modify: `internal/httpapi/task_series_test.go`
- Modify: `internal/mcpserver/tools_task_series_test.go`
- Modify: `internal/remote/project_test.go`
- Modify: `internal/remote/task_test.go`

**Interfaces:**
- `httpapi.Options.ResourceBaseURL string` -> `Server.resourceBaseURL` -> `app.ServiceOptions.ResourceBaseURL` 与 HTTP MCP options。
- `mcpserver.Options.ResourceBaseURL string` -> `RuntimeFactory.ResourceBaseURL` -> stdio/HTTP scoped App Service。
- 本地 CLI 与 stdio MCP 从 `cfg.ResourceBaseURL()` 注入；Remote 只透传服务端响应。

- [ ] **Step 1: 写协议注入失败测试**

HTTP/MCP fixture 显式设置 `ResourceBaseURL: "https://xuanchu.example.com"`，断言 project/task/series 的 `url` 为绝对 URL；另建默认 fixture 断言 `url == ""`。Remote DTO 测试改用绝对 URL并证明 round-trip 不变。

- [ ] **Step 2: 运行协议测试并确认 RED**

Run: `go test ./internal/httpapi ./internal/mcpserver ./internal/remote -run 'URL|Canonical' -count=1`

Expected: FAIL，Options 尚无字段或响应仍为相对 URL。

- [ ] **Step 3: 完成所有生产构造链路注入**

CLI server 创建 `httpapi.NewServer` 时传 `cfg.ResourceBaseURL()`；stdio MCP 创建 `mcpserver.NewServer` 时传同一值；本地 CLI `buildServiceFromOpts` 注入同一值。HTTP `scopedServiceFor` 与 MCP `RuntimeFactory` 创建真实 scoped service 时继续透传，禁止在输出层或 Remote client 拼接。

- [ ] **Step 4: 运行协议测试并确认 GREEN**

Run: `go test ./internal/httpapi ./internal/mcpserver ./internal/remote -run 'URL|Canonical' -count=1`

Expected: PASS。

---

### Task 4: 文档、跨协议 E2E 与本地配置

**Files:**
- Modify: `README.md`
- Modify: `config.example.toml`
- Modify: `tests/integration/cli_test.go`
- Modify: `tests/integration/e2e_mcp_test.go`
- Local-only modify: `/Users/mac/code/projects/dajee/task/config.toml`

**Interfaces:**
- 配置示例：`[server] public_base_url = "https://xuanchu.example.com"`。
- 本地配置：`[server] public_base_url = "https://xuanchu.example.com"`。

- [ ] **Step 1: 写跨协议失败断言**

让 E2E 生成含 `server.public_base_url = "https://console.example.test"` 的临时配置，断言 HTTP、HTTP MCP、stdio MCP、本地 CLI、Remote CLI 对同一项目/任务/series 返回相同绝对 URL；另覆盖无配置时 `url == ""`。

- [ ] **Step 2: 运行 E2E 并确认 RED**

Run: `go test ./tests/integration -run 'ResourceURL|URLContract' -count=1`

Expected: FAIL，至少一个协议仍返回相对 URL或没有读取 public base URL。

- [ ] **Step 3: 更新文档和配置**

README 写明配置、空值行为、Console base path 合成规则；示例和本地配置增加 `[server] public_base_url`。本地 `config.toml` 不纳入 Git。

- [ ] **Step 4: 运行定向 E2E 并确认 GREEN**

Run: `go test ./tests/integration -run 'ResourceURL|URLContract' -count=1`

Expected: PASS。

---

### Task 5: 全量验证、合并与生产部署

**Files:**
- Runtime modify: `/opt/xuanchu/config.toml` on production host。
- Runtime release: `/opt/xuanchu/releases/<timestamp>`。

**Interfaces:**
- Production origin: `https://xuanchu.example.com`。
- Deploy command: `deployer --config deploy.yaml --stage production`。

- [ ] **Step 1: 功能分支全量验证**

Run: `go test ./...`

Run: `CGO_ENABLED=0 go test ./...`

Run: `CGO_ENABLED=0 go build ./cmd/xuanchu`

Run: `go vet ./...`

Run: `pnpm --dir web typecheck && pnpm --dir web test && pnpm --dir web lint && pnpm --dir web build`

Run: `pnpm --dir web run smoke:editing`

Run: `git diff --check`

Expected: 全部 exit 0。

- [ ] **Step 2: 提交并合并回 main**

功能分支提交后，在主仓库执行普通 merge；合并成功后至少重新运行 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`、`go vet ./...`、`git diff --check`，然后安全移除 worktree 和已合并分支。

- [ ] **Step 3: 修改线上共享配置并部署**

先通过 SSH 备份 `/opt/xuanchu/config.toml`，再幂等写入：

```toml
[server]
public_base_url = "https://xuanchu.example.com"
```

执行 `deployer --config deploy.yaml --stage production`，记录新 release 路径。

- [ ] **Step 4: 线上验收**

验证 `systemctl is-active xuanchu`、本机 `http://127.0.0.1:8080/healthz`、公网 `https://xuanchu.example.com/healthz`；再用有效 token 请求真实 project/task/series API，证明 `url` 以 `https://xuanchu.example.com/` 开头且对应路由可访问。
