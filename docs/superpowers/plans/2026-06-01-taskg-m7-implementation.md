# taskg M7 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 实现 M7：升级 Go 与官方 MCP Go SDK，补齐 M6 遗留远程管理命令，并提供 stdio / Streamable HTTP MCP Server，让 Agent 以受权限约束的 tools/resources 操作 taskg。

**Architecture:** M7 分三层推进：先做 Go 版本与 REST/远程 CLI 收口，再引入官方 MCP SDK 骨架，最后在 `internal/mcpserver` 中实现 transport/auth/scope/result/tools/resources。MCP tool 不调用 HTTP API，不直接访问 storage，只通过 `internal/app` 构造已授权 service；REST、CLI、MCP 共享 app service、request scope、render、audit 和稳定错误码。

**Tech Stack:** Go 1.25、Cobra、GORM、`github.com/glebarez/sqlite`（保持 `CGO_ENABLED=0`）、`net/http`、`github.com/go-chi/chi/v5`、`github.com/modelcontextprotocol/go-sdk/mcp`、OpenAPI 3 YAML、Go test / httptest / MCP SDK client integration tests。

---

## 范围锁定

严格按 [M7 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-06-01-taskg-m7-design.md) 实现。

必须进入 M7：

- Go 版本升级到 `1.25`，且独立验证。
- 远程管理命令收口：`workspace list/info/add/modify/use/archive`、`user list/info/add`、`member list/add/role`、`show`。
- REST 小扩展：`PUT /api/v1/me/active_workspace`、`GET /api/v1/users`、`POST /api/v1/users`、`GET /api/v1/users/{user}`。
- `/api/v1/tasks` 与 MCP `task.query` / `report.run` 统一 limit：默认 200，最大 1000，非法返回 `api_bad_limit`。
- 本地 `show <key>` 与远程 `show <key>` 对称。
- 官方 MCP Go SDK 引入，先建空 server 骨架。
- `taskg mcp stdio`。
- `taskg server` 挂载 `/mcp`。
- HTTP MCP Bearer token 鉴权，stdio MCP 本地 actor/workspace 解析。
- MCP result/error/scope adapter。
- MCP tools：`task.*`、`report.run`、`urgency.explain`、workspace/project/context/config tools。
- MCP resources：`taskg://workspace/current`、`taskg://workspace/{workspace_id}`、`taskg://project/{project_id}`、`taskg://context/current`。
- MCP tool schema golden tests。
- stdio / HTTP MCP integration tests。
- README、ROADMAP、AGENTS、OpenAPI 同步。

不进入 M7：

- OAuth/JWT/password login/refresh token。
- 外部系统 adapter。
- op-log sync。
- Hook / trigger engine。
- Web UI。
- 列表型 MCP resources。
- `task.denotate`。
- `ReplaceDepends` 新语义。
- `user use` 远程模式。
- 第二个 HTTP framework。
- CGO SQLite driver。

## 执行注意事项

- 每个 Phase 0a/0b/0c 必须独立提交并独立跑完整验证。
- 不要让 `internal/httpapi` 或 `internal/mcpserver` 复制 app 业务规则；缺 app 方法就补 `internal/app`。
- MCP tool 不通过本机 HTTP client 调 `/api/v1`。
- MCP business error 返回 `CallToolResult.IsError=true`；protocol error 才走 JSON-RPC error。
- MCP stdio 的 stdout 只能是 MCP JSON-RPC，启动提示、warning、日志全部走 stderr 或禁用。
- `context.set` 只切 active context，使用 `context:write + PermissionContextUse`；不新增 context define/delete tool。
- `task.depends` 是 `task.modify` 的窄包装：`depends` add-only，`clear_depends` 先清再加。
- `agent.*` project config value 上限 16KB，超长返回 `config_value_too_large`。
- `agent.*` 16KB 是 project config 的独立上限；M7 不修改 audit payload size 行为，除非 spec 后续单独要求。
- `config.get scope=local` 仅 stdio MCP 读取本机 TOML；HTTP MCP 永远不读客户端 TOML。
- `/mcp` 不进 OpenAPI；REST 新增 endpoint 必须进 OpenAPI。
- 每个 chunk 完成后至少跑局部测试；跨 chunk 后跑完整验证。

## 文件结构

新增文件：

- `internal/mcpserver/server.go`
  构造 MCP server、集中注册 tools/resources、暴露 `NewServer(opts Options)`。
- `internal/mcpserver/options.go`
  MCP server options、mode、clock/store/logger/version。
- `internal/mcpserver/SDK_NOTES.md`
  Phase 0c 固定官方 MCP Go SDK 版本、关键 API 签名、transport/handler 行为；后续 MCP task 必须按此文件实现，不再保留猜测式伪 API。
- `internal/mcpserver/auth.go`
  stdio/http actor、token、workspace/project scope 解析；构造已授权 app service。
- `internal/mcpserver/result.go`
  `{data, rendered}` result、business error `IsError=true`、protocol/internal error 映射。
- `internal/mcpserver/tools_task.go`
  `task.add/query/get/modify/done/delete/annotate/depends/start/stop`。
- `internal/mcpserver/tools_report.go`
  `report.run`、`urgency.explain`。
- `internal/mcpserver/tools_workspace.go`
  `workspace.list`、`workspace.current`。
- `internal/mcpserver/tools_project.go`
  `project.list`、`project.get`、`project.current`。
- `internal/mcpserver/tools_context.go`
  `context.show`、`context.set`。
- `internal/mcpserver/tools_config.go`
  `config.get`、`config.set`。
- `internal/mcpserver/resources.go`
  workspace/project/context resources。
- `internal/mcpserver/schema_test.go`
  tool schema golden tests。
- `internal/mcpserver/result_test.go`
  result/error envelope tests。
- `internal/mcpserver/auth_test.go`
  stdio/http scope resolver tests。
- `internal/mcpserver/integration_test.go`
  MCP SDK client in-memory / HTTP integration tests。
- `internal/mcpserver/testdata/*.schema.json`
  每个 tool 的 input schema golden 文件。
- `internal/remote/workspace.go`
  remote workspace APIs，包括 `UseWorkspace`。
- `internal/remote/user.go`
  remote user APIs。
- `internal/remote/member.go`
  remote member APIs，如现有 client 未覆盖。
- `internal/remote/show.go`
  remote show/config convenience API，如现有 config client 不足。
- `internal/httpapi/users.go`
  user management REST handlers。
- `internal/httpapi/me_state.go`
  actor state REST handlers，例如 active workspace。
- `internal/httpapi/workspace_archive.go`
  workspace archive REST handler，如不合并进 `workspaces.go`。
- `internal/cli/mcp.go`
  `taskg mcp stdio` 命令。

修改文件：

- `go.mod` / `go.sum`
  Go 版本升级；Phase 0c 引入官方 MCP SDK。
- `AGENTS.md`
  Go 版本从 1.22 改为 1.25；保留 CGO-free SQLite 约束。
- `README.md`
  M7 能力、Go 版本、MCP 运行示例、远程管理命令状态。
- `ROADMAP.md`
  M7 状态与已完成范围同步。
- `docs/requirements.md`
  如仍写 Go 1.22 或 MCP 草案，更新到 M7 选型。
- `docs/openapi/taskg-v1.yaml`
  新增 users、active_workspace、task list limit。
  同步新增 REST error enum：`workspace_required`（若 app shared error 进入 HTTP/OpenAPI）和 workspace archive endpoint。
- `cmd/taskg/main.go`
  确保 `mcp stdio` 跳过 migration warning 或不污染 stdout。
- `internal/cli/root.go`
  注册 `mcp` 命令；确认 split flags 支持 MCP 所需 flags。
- `internal/cli/server.go`
  `taskg server` 挂载 `/mcp` handler。
- `internal/cli/config.go`
  `show [key]` 本地支持；remote show 接线。
- `internal/cli/workspace.go`
  remote workspace 命令接线。
- `internal/cli/user.go`
  remote user 命令接线；`user use` remote 继续 unsupported。
- `internal/cli/member.go`
  remote member 命令接线。
- `internal/httpapi/router.go`
  注册 users、active_workspace、`/mcp`。
- `internal/httpapi/tasks.go`
  task list limit 默认 200 / max 1000。
- `internal/storage/sqlite/task_repo.go`
  `ListOptions.Limit` 与 SQL `LIMIT`。
- `internal/httpapi/workspaces.go`
  复用 workspace response DTO；新增 workspace archive endpoint。
- `internal/httpapi/me.go`
  如 actor state handler 更适合放这里，可合并 `me_state.go`。
- `internal/httpapi/app_service.go`
  抽出或复用 request scope 构造，供 MCP HTTP adapter 使用。
- `internal/app/workspace.go`
  如 `UseWorkspace` 需要返回 view，新增安全 wrapper；user endpoint 复用现有 `ListUsers/AddUser/UserInfo`。
- `internal/app/project_config.go`
  `agent.*` key 白名单与 16KB 上限。
- `internal/app/service.go`
  task list/report limit 支持；MCP tools 需要的 app 方法补齐。
- `internal/remote/task.go`
  task list limit query 参数。
- `tests/integration/cli_test.go`
  远程管理命令、show key、stdio MCP 黑盒测试。

---

## Chunk 1: Phase 0a Go 1.25 升级

### Task 1: 升级 Go 版本文档与模块

**Files:**
- Modify: `go.mod`
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/requirements.md`

- [x] **Step 1: 确认当前工具链**

Run:

```bash
go version
```

Expected: Go version is `go1.25.x` or newer. If local toolchain is older, install/switch Go before editing. On machines using the Go toolchain downloader, use:

```bash
go install golang.org/dl/go1.25@latest
go1.25 download
go1.25 version
```

Then run subsequent Go commands with `go1.25` or update `PATH` so `go version` reports `go1.25.x`。

- [x] **Step 2: 修改 `go.mod`**

Change:

```go
go 1.25
```

Do not add MCP SDK in this task.

- [x] **Step 3: 同步文档里的 Go 版本**

Update exact references that describe current stack:

- `AGENTS.md`: `Go: 1.22` -> `Go: 1.25`。
- `README.md`: 如果写当前 Go 版本，改为 1.25。
- `ROADMAP.md`: 如果写当前 Go 版本，改为 1.25。
- `docs/requirements.md`: 如果写当前 Go 版本，改为 1.25。

Keep `github.com/glebarez/sqlite` and `CGO_ENABLED=0` language unchanged.

- [x] **Step 4: Run module tidy**

Run:

```bash
go mod tidy
```

Expected: no new MCP dependency appears; `go.sum` changes only if Go version causes tidy normalization.

- [x] **Step 5: Run Phase 0a full verification**

Run:

```bash
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' && exit 1 || true
```

Expected: all commands exit 0. Remove generated `taskg` binary if `go build` creates it:

```bash
rm -f taskg
```

If `gofmt -l` reports files, run `gofmt -w` on those files first, then re-run the full Phase 0a verification before committing.

- [x] **Step 6: Commit**

```bash
git add go.mod go.sum AGENTS.md README.md ROADMAP.md docs/requirements.md
git commit -m "chore: 升级 Go 版本到 1.25"
```

---

## Chunk 2: Phase 0b REST 管理 API 与远程 CLI 收口

### Task 2: 为 `/api/v1/tasks` 增加 limit 默认值和上限

**Files:**
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/tasks_test.go`
- Modify: `internal/remote/task.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [x] **Step 1: 写 HTTP limit 测试**

Add tests in `internal/httpapi/tasks_test.go`:

```go
func TestTaskListRejectsInvalidLimit(t *testing.T) {
    fixture := newHTTPFixture(t)
    token := fixture.createToken(t, "task:read")

    resp := fixture.request(t, http.MethodGet, "/api/v1/tasks?limit=1001", nil, token)
    if resp.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
    }
    assertErrorCode(t, resp.Body, "api_bad_limit")
}

func TestTaskListDefaultLimitDoesNotRejectEmptyLimit(t *testing.T) {
    fixture := newHTTPFixture(t)
    token := fixture.createToken(t, "task:read")

    resp := fixture.request(t, http.MethodGet, "/api/v1/tasks", nil, token)
    if resp.Code != http.StatusOK {
        t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
    }
}
```

Adjust fixture helper names to match existing tests.

- [x] **Step 2: Run test and confirm failure**

Run:

```bash
go test ./internal/httpapi -run 'TestTaskList.*Limit' -count=1
```

Expected: FAIL because limit is not implemented or invalid limit is accepted.

- [x] **Step 3: Implement parser**

In `internal/httpapi/tasks.go`, add constants:

```go
const (
    taskListDefaultLimit = 200
    taskListMaxLimit     = 1000
)
```

Parse `limit` query:

- empty -> 200。
- non-integer / <=0 -> 400 `api_bad_limit`。
- >1000 -> 400 `api_bad_limit`。

Thread the parsed limit into app task list/report input. If current app list input lacks limit, add it in `internal/app` and storage query path.

Concrete changes:

- add `Limit int` to `app.ListInput`。
- add `Limit int` to `sqlite.ListOptions`。
- pass `Limit` from `Service.List` and `RunReport` / `ListReport` into `TaskRepository.List`。
- apply `q = q.Limit(opts.Limit)` only when `opts.Limit > 0` inside `TaskRepository.List` after sorting。
- keep internal app callers with zero limit as unlimited unless an HTTP/MCP entry point sets default 200 explicitly。

- [x] **Step 4: Update remote client**

In `internal/remote/task.go`, add `Limit int` to `ListTasksInput` and include `limit` query parameter only when non-zero. Existing CLI calls can leave it zero and use server default.

- [x] **Step 5: Update OpenAPI**

In `docs/openapi/taskg-v1.yaml`, document `limit` on `GET /api/v1/tasks`:

- default 200。
- maximum 1000。
- minimum 1。
- `api_bad_limit` 400 response。

- [x] **Step 6: Verify**

Run:

```bash
go test ./internal/httpapi -run 'TestTaskList.*Limit' -count=1
go test ./internal/remote ./internal/app -count=1
```

Expected: PASS.

### Task 3: 新增 user REST API

**Files:**
- Create: `internal/httpapi/users.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/server_test.go` or `internal/httpapi/auth_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [x] **Step 1: Write handler tests**

Add tests:

```go
func TestUserListCreateInfoHTTP(t *testing.T) {
    fixture := newHTTPFixture(t)
    token := fixture.createToken(t, "workspace:write", "workspace:read")

    createBody := `{"name":"agent-user","email":"agent@example.com"}`
    created := fixture.request(t, http.MethodPost, "/api/v1/users", strings.NewReader(createBody), token)
    if created.Code != http.StatusCreated {
        t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
    }
    assertSnakeCaseJSON(t, created.Body.Bytes())

    list := fixture.request(t, http.MethodGet, "/api/v1/users", nil, token)
    if list.Code != http.StatusOK {
        t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
    }

    info := fixture.request(t, http.MethodGet, "/api/v1/users/agent-user", nil, token)
    if info.Code != http.StatusOK {
        t.Fatalf("info status=%d body=%s", info.Code, info.Body.String())
    }
}
```

Also add permission test: viewer/member without adequate role gets 403 for `POST /api/v1/users` if current app permission requires it. If current app service lacks explicit permission for user management, add one in app layer rather than allowing every token.

Use existing test helpers from `internal/httpapi` (`newHTTPServerWithTokenFixture`, `requestHTTP`, `requestHTTPBody`, `assertHTTPErrorCode`, `assertSnakeCaseResponse`) rather than introducing a second fixture API.

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/httpapi -run 'TestUser.*HTTP' -count=1
```

Expected: FAIL with route not found.

- [x] **Step 3: Implement response DTOs**

Create `internal/httpapi/users.go`:

```go
type userResponse struct {
    ID                 string  `json:"id"`
    Name               string  `json:"name"`
    Email              *string `json:"email"`
    DefaultWorkspaceID *string `json:"default_workspace_id"`
    Active             bool    `json:"active"`
    CreatedAt          int64   `json:"created_at"`
    ModifiedAt         int64   `json:"modified_at"`
}
```

Map from `app.UserView`.

- [x] **Step 4: Implement handlers**

Handlers:

- `handleUserList`: `GET /api/v1/users`。
- `handleUserCreate`: `POST /api/v1/users` with `{name,email}`。
- `handleUserInfo`: `GET /api/v1/users/{user}`。

Use `scopedService` and keep user management mapped to existing workspace-level management permissions for M7:

- `GET /api/v1/users` and `GET /api/v1/users/{user}` require token capability `workspace:read` and app permission `PermissionWorkspaceRead`。
- `POST /api/v1/users` requires token capability `workspace:write` and app permission `PermissionWorkspaceModify`。
- Do not introduce new `user:*` token capabilities in M7; if the app layer has no dedicated user permission, keep role evaluation through the workspace permission mapping above.
- Current `Service.ListUsers` / `AddUser` / `UserInfo` do not call `Require` internally. For M7, HTTP handler `scopedService` is the permission boundary; do not expose these methods from MCP tools unless equivalent permission wrapping is added there too.

- [x] **Step 5: Register routes**

In `internal/httpapi/router.go`:

```go
api.With(s.authMiddleware).Get("/api/v1/users", s.handleUserList)
api.With(s.authMiddleware).Post("/api/v1/users", s.handleUserCreate)
api.With(s.authMiddleware).Get("/api/v1/users/{user}", s.handleUserInfo)
```

- [x] **Step 6: Update OpenAPI**

Add paths and schemas:

- `User`
- `UserCreateRequest`
- `GET /api/v1/users`
- `POST /api/v1/users`
- `GET /api/v1/users/{user}`

- [x] **Step 7: Verify**

Run:

```bash
go test ./internal/httpapi -run 'TestUser.*HTTP' -count=1
go test ./internal/app -run 'Test.*User' -count=1
```

Expected: PASS.

### Task 4: 新增 actor active workspace REST API

**Files:**
- Create: `internal/httpapi/me_state.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/server_test.go` or `internal/httpapi/auth_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [x] **Step 1: Write failing test**

Add test:

```go
func TestPutMeActiveWorkspaceUpdatesServerState(t *testing.T) {
    fixture := newHTTPServerWithTokenFixture(t, "workspace:read")
    ownerSvc := app.NewService(...)
    ws, err := ownerSvc.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"})
    if err != nil {
        t.Fatal(err)
    }

    body := fmt.Sprintf(`{"workspace":%q}`, ws.Slug)
    resp := requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/me/active_workspace", body, map[string]string{
        "Authorization": "Bearer " + fixture.token,
    })
    if resp.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
    }
    assertSnakeCaseResponse(t, resp.Body.String())
    if !strings.Contains(resp.Body.String(), `"active":true`) {
        t.Fatalf("active workspace response is not active: %s", resp.Body.String())
    }

    me := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{
        "Authorization": "Bearer " + fixture.token,
    })
    if !strings.Contains(me.Body.String(), `"effective_workspace"`) || !strings.Contains(me.Body.String(), ws.ID) {
        t.Fatalf("effective workspace not updated: %s", me.Body.String())
    }
}
```

Adapt only constructor details to the actual fixture shape; keep assertions against existing `/api/v1/me.effective_workspace`, not a new `active_workspace` field.

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/httpapi -run TestPutMeActiveWorkspaceUpdatesServerState -count=1
```

Expected: FAIL with route not found.

- [x] **Step 3: Implement handler**

Create `internal/httpapi/me_state.go` with:

```go
type activeWorkspaceRequest struct {
    Workspace string `json:"workspace"`
}
```

Handler:

- require auth。
- use `scopedService` with capability `workspace:read` and app permission `PermissionWorkspaceRead`。切换 active workspace 是 actor 自我状态变更，不是 workspace 级 mutation；能读取该 workspace 且 membership 有效即可切换。
- call `svc.UseWorkspace(req.Workspace)`。
- construct a fresh scoped service after `UseWorkspace`, because the original service runtime still points at the previous workspace。
- call `freshSvc.WorkspaceInfo(req.Workspace)` and return `workspaceResponseFromView(view)` through the standard REST envelope；the returned workspace must have `active=true`。

- [x] **Step 4: Register route**

In `internal/httpapi/router.go`:

```go
api.With(s.authMiddleware).Put("/api/v1/me/active_workspace", s.handleMeActiveWorkspacePut)
```

- [x] **Step 5: Update OpenAPI**

Add `PUT /api/v1/me/active_workspace`:

- request body `{workspace:string}`。
- response uses the standard REST success envelope with body `{ "data": <Workspace> }` where `<Workspace>` is the existing `workspaceResponse` from `internal/httpapi/workspaces.go`。Remote `workspace use` parses this shape into `remote.Workspace`.
- update OpenAPI `ErrorCode` enum with `workspace_required` if not already present, because M7 moves this code into shared app/request-scope logic.

- [x] **Step 6: Verify**

Run:

```bash
go test ./internal/httpapi -run 'TestPutMeActiveWorkspace|TestMe' -count=1
```

Expected: PASS.

### Task 5: 远程 workspace/user/member/show 接线

**Files:**
- Create: `internal/remote/workspace.go`
- Create: `internal/remote/user.go`
- Create or Modify: `internal/remote/member.go`
- Create or Modify: `internal/remote/show.go`
- Modify: `internal/cli/workspace.go`
- Modify: `internal/cli/user.go`
- Modify: `internal/cli/member.go`
- Modify: `internal/cli/config.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/workspaces.go`
- Modify: `tests/integration/cli_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`

- [x] **Step 1: Split old unsupported-management test**

Refactor `TestCLIRemoteUnsupportedManagementCommandsDoNotTouchLocalDB` in `tests/integration/cli_test.go`:

- keep only `user use` in the unsupported test。
- move workspace cases into `TestCLIRemoteWorkspaceCommands`。
- move user list/info/add cases into `TestCLIRemoteUserCommands`。
- move member cases into `TestCLIRemoteMemberCommands`。
- move `show` / `show <key>` into `TestCLIRemoteShowCommand`。
- keep the assertion that remote commands do not create or mutate the client-side local DB path.
- confirm route coverage before moving each case: existing M6 routes cover workspace list/add/info/modify and member list/add/role; M7 must add user routes, active workspace route, show wiring, and workspace archive route.

- [x] **Step 2: Write integration tests**

In `tests/integration/cli_test.go`, add black-box tests that start `taskg server` with temp DB and token:

- remote `workspace list` succeeds。
- remote `workspace add/info/modify/use/archive` succeeds。
- remote `user add/list/info` succeeds。
- remote `user use` returns `remote_unsupported_command` and does not touch local DB。
- remote `member list/add/role` succeeds。
- remote `show` and `show date.format` succeed。
- remote commands do not create or modify local DB path passed via `--db` on client side.

Use existing M6 remote integration helpers if present.

- [x] **Step 3: Add workspace archive REST endpoint**

M6 has `Service.ArchiveWorkspace(ref)` but no REST route for workspace archive. Add:

```go
api.With(s.authMiddleware).Post("/api/v1/workspaces/{workspace}/archive", s.handleWorkspaceArchive)
```

Handler rules:

- use `scopedServiceWithWorkspace` with capability `workspace:write` and app permission `PermissionWorkspaceArchive`。
- call `svc.ArchiveWorkspace(ref)`。
- return the archived `Workspace` view in `{ "data": <Workspace> }` by creating a fresh scoped service or querying the archive result in a way that does not report stale active state。
- update OpenAPI with `POST /api/v1/workspaces/{workspace}/archive` and documented errors.

- [x] **Step 4: Run and confirm failures**

Run:

```bash
go test ./tests/integration -run 'Remote.*(Workspace|User|Member|Show)' -count=1
```

Expected: FAIL because commands are still unsupported.

- [x] **Step 5: Implement remote clients**

Implement methods:

```go
func (c *Client) ListWorkspaces(ctx context.Context, includeArchived bool) ([]Workspace, error)
func (c *Client) AddWorkspace(ctx context.Context, input WorkspaceAddInput) (Workspace, error)
func (c *Client) WorkspaceInfo(ctx context.Context, ref string) (Workspace, error)
func (c *Client) ModifyWorkspace(ctx context.Context, ref string, input WorkspaceModifyInput) error
func (c *Client) UseWorkspace(ctx context.Context, ref string) (Workspace, error)
func (c *Client) ArchiveWorkspace(ctx context.Context, ref string) error

func (c *Client) ListUsers(ctx context.Context) ([]User, error)
func (c *Client) AddUser(ctx context.Context, input UserAddInput) (User, error)
func (c *Client) UserInfo(ctx context.Context, ref string) (User, error)

func (c *Client) ListMembers(ctx context.Context, workspace string) ([]Member, error)
func (c *Client) AddMember(ctx context.Context, input MemberAddInput) error
func (c *Client) ChangeMemberRole(ctx context.Context, input MemberRoleInput) error
```

Use snake_case JSON structs matching REST DTOs. `UseWorkspace` must decode `PUT /api/v1/me/active_workspace` from `{ "data": <Workspace> }` and return that `Workspace`. Add `func (c *Client) put(ctx context.Context, path string, body any, out any) error` to `internal/remote/client.go`; do not hand-roll PUT only in `UseWorkspace`.

Remote workspace mapping:

- list -> `GET /api/v1/workspaces?all=<bool>`。
- add -> `POST /api/v1/workspaces`。
- info -> `GET /api/v1/workspaces/{workspace}`。
- modify -> `PATCH /api/v1/workspaces/{workspace}`。
- use -> `PUT /api/v1/me/active_workspace`。
- archive -> `POST /api/v1/workspaces/{workspace}/archive`。

- [x] **Step 6: Wire CLI workspace/user/member**

Replace `remoteUnsupported` in remote-capable commands with remote branch:

```go
if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
    return err
} else if remoteMode {
    client, err := buildRemoteClient(currentOpts)
    if err != nil { return err }
    // call remote client, render same shape as local
}
```

Keep `user use` remote unsupported.

- [x] **Step 7: Upgrade `show [key]` local and remote**

Change `newShowCommand`:

```go
Use:  "show [key]",
Args: cobra.MaximumNArgs(1),
```

Local:

- no arg: existing human list；in `--json` mode return a JSON object map of public keys to values。
- key: human mode prints the raw value only, matching `_show <key>` semantics; JSON mode returns `{ "key": "<key>", "value": "<value>" }`; missing key returns existing `unknown config key` error behavior。

Remote:

- no arg: use remote config list / me as needed。
- key: same output contract as local；use remote config get for business keys and `/api/v1/me` for actor-state keys if needed。

- [x] **Step 8: Verify**

Run:

```bash
go test ./internal/remote ./internal/cli -count=1
go test ./tests/integration -run 'Remote.*(Workspace|User|Member|Show)' -count=1
```

Expected: PASS.

### Task 6: Phase 0b full verification and commit

**Files:**
- All files touched in Chunk 2.

- [x] **Step 1: Run full verification**

Run:

```bash
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' && exit 1 || true
rm -f taskg
```

Expected: all verification commands exit 0.

- [x] **Step 2: Commit**

```bash
git add internal/app internal/httpapi internal/remote internal/cli tests/integration docs/openapi/taskg-v1.yaml
git commit -m "feat: 收口 M7 远程管理命令并统一 task 列表 limit"
```

---

## Chunk 3: Phase 0c 官方 MCP SDK 骨架

### Task 7: 引入官方 MCP SDK

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/mcpserver/options.go`
- Create: `internal/mcpserver/server.go`
- Create: `internal/mcpserver/SDK_NOTES.md`
- Create: `internal/mcpserver/schema_test.go`

- [x] **Step 1: Add dependency**

Run:

```bash
go get github.com/modelcontextprotocol/go-sdk@<selected-version>
go mod tidy
```

Select the newest official SDK version compatible with Go 1.25 at execution time, then pin that exact version in `go.mod` and record it in `SDK_NOTES.md`。Do not leave `@latest` in scripts or docs after Phase 0c. Expected: `go.mod` includes `github.com/modelcontextprotocol/go-sdk`; no CGO SQLite dependency appears.

- [x] **Step 2: Record SDK API notes**

Read the installed module docs and examples from the module cache:

```bash
go list -m github.com/modelcontextprotocol/go-sdk
go doc github.com/modelcontextprotocol/go-sdk/mcp
```

Create `internal/mcpserver/SDK_NOTES.md` with:

- exact module version。
- server constructor signature。
- tool registration signature。
- in-memory/client transport signature for tests。
- stdio server transport signature。
- Streamable HTTP handler constructor and whether its factory receives `*http.Request` with the original `Context`。
- how SDK exposes input schema from `ListTools`。
- Streamable HTTP session ownership: whether one `mcp.Server` may be reused across requests or the SDK expects/request-scopes a server per session/request。
- body limit semantics for Streamable HTTP: whether `bodyLimitMiddleware` constrains each POST request body, GET/SSE setup, or any longer-lived session payload.

Do not proceed to Step 3 until this file names the concrete APIs later tasks must use. If SDK examples disagree with pseudo-code in this plan, follow `SDK_NOTES.md` and update the pseudo-code in the same commit.

- [x] **Step 3: Create options**

Create `internal/mcpserver/options.go`:

```go
package mcpserver

import (
    "io"

    "github.com/dajee/taskg/internal/app"
    "github.com/dajee/taskg/internal/storage/sqlite"
)

type Mode string

const (
    ModeStdio Mode = "stdio"
    ModeHTTP  Mode = "http"
)

type Options struct {
    Store   *sqlite.Store
    Clock   app.Clock
    Version string
    Mode    Mode
    Stderr  io.Writer
}
```

- [x] **Step 4: Create empty server constructor**

Create `internal/mcpserver/server.go`:

```go
package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

func NewServer(opts Options) *mcp.Server {
    version := opts.Version
    if version == "" {
        version = "dev"
    }
    srv := mcp.NewServer(&mcp.Implementation{
        Name:    "taskg",
        Version: version,
    }, nil)
    return srv
}
```

Do not implement stdio/HTTP transport, auth, or scope adapter in Phase 0c.

- [x] **Step 5: Add empty tools/list smoke test**

In `internal/mcpserver/schema_test.go`, use the SDK in-memory client/server transport recorded in `SDK_NOTES.md` to assert empty `tools/list` result or SDK-equivalent empty list. Also create the reusable golden comparison helper and `-update` flag now; with zero tools it should pass without writing files. This smoke test verifies `NewServer` and the selected SDK transport can run, and Task 13 will add actual tool goldens on top of the same framework.

Skeleton:

```go
func TestNewServerStartsWithNoTools(t *testing.T) {
    ctx := context.Background()
    server := NewServer(Options{Version: "test"})
    client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
    serverTransport, clientTransport := mcp.NewInMemoryTransports()
    if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
        t.Fatalf("server connect: %v", err)
    }
    session, err := client.Connect(ctx, clientTransport, nil)
    if err != nil {
        t.Fatalf("client connect: %v", err)
    }
    defer session.Close()
    got, err := session.ListTools(ctx, nil)
    if err != nil {
        t.Fatalf("ListTools: %v", err)
    }
    if len(got.Tools) != 0 {
        t.Fatalf("tools = %d, want 0", len(got.Tools))
    }
}
```

Use exact signatures recorded in `internal/mcpserver/SDK_NOTES.md`; do not leave guessed SDK calls in committed code.

- [x] **Step 6: Verify Phase 0c**

Run:

```bash
go test ./internal/mcpserver -count=1
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' && exit 1 || true
rm -f taskg
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add go.mod go.sum internal/mcpserver
git commit -m "feat: 引入 MCP SDK 骨架"
```

---

## Chunk 4: Phase 1 MCP transport、auth、scope、result

### Task 8: 实现 MCP result/error adapter

**Files:**
- Create: `internal/mcpserver/result.go`
- Create: `internal/mcpserver/result_test.go`

- [x] **Step 1: Write tests**

Tests:

- success result returns `data` and `rendered`。
- app `RuntimeError{Code:"task_not_found"}` returns `CallToolResult.IsError=true` with rendered text containing code/message。
- non-app error maps to `mcp_internal` without stack trace。

Example:

```go
func TestToolErrorPreservesRuntimeCode(t *testing.T) {
    result := toolError(app.RuntimeError{Code: "task_not_found", Message: "task not found"})
    if !result.IsError {
        t.Fatal("IsError = false, want true")
    }
    if !strings.Contains(renderedText(result), "task_not_found") {
        t.Fatalf("rendered = %q", renderedText(result))
    }
}
```

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run TestToolError -count=1
```

Expected: FAIL because helper does not exist.

- [x] **Step 3: Implement helpers**

Implement helpers:

```go
type ToolEnvelope struct {
    Data     any    `json:"data"`
    Rendered string `json:"rendered"`
}

func successResult(data any, rendered string) (*mcp.CallToolResult, any, error)
func businessErrorResult(err error) *mcp.CallToolResult
```

Use SDK content types. If SDK supports structured output return `ToolEnvelope`; otherwise return text content and keep `ToolEnvelope` for future structured data tests.

- [x] **Step 4: Verify**

Run:

```bash
go test ./internal/mcpserver -run 'TestTool.*Result|TestToolError' -count=1
```

Expected: PASS.

### Task 9: 实现 MCP auth/scope factory

**Files:**
- Create: `internal/mcpserver/auth.go`
- Create: `internal/mcpserver/auth_test.go`
- Modify: `internal/httpapi/app_service.go` if useful helpers need exporting/moving.
- Modify: `internal/app/request_scope.go` only if reusable API is missing.

- [x] **Step 1: Write stdio scope tests**

Tests:

- stdio default uses local active user/workspace from SQLite。
- stdio `workspace` override resolves within same DB。
- stdio project slug + project_id mismatch returns stable app error。
- stdio with no active user/workspace returns stable MCP business/protocol error and does not auto-create an anonymous actor or workspace。

- [x] **Step 2: Write HTTP scope tests**

Tests:

- missing/invalid token maps to auth error。
- project-scoped token denies allowlist outside project。
- token visible multiple workspaces + project slug without workspace returns `workspace_required`。

- [x] **Step 3: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run 'Test.*Scope|Test.*Auth' -count=1
```

Expected: FAIL because auth factory does not exist.

- [x] **Step 4: Implement request model**

Create:

```go
type RequestScopeInput struct {
    Workspace string
    Project   string
    ProjectID string
}

type RuntimeFactory struct {
    Store *sqlite.Store
    Clock app.Clock
}
```

Implement:

```go
func (f RuntimeFactory) ServiceForStdio(input RequestScopeInput, capability string, permission app.Permission) (*app.Service, error)
func (f RuntimeFactory) ServiceForHTTP(r *http.Request, input RequestScopeInput, capability string, permission app.Permission) (*app.Service, error)
```

Reuse M6 app request scope authorization. If helper currently lives inside `internal/httpapi`, move generic logic to `internal/app` rather than importing `httpapi` from `mcpserver`.

When HTTP MCP receives a project slug without `workspace` / `workspace_id` and the token can see multiple workspaces, return app `RuntimeError{Code:"workspace_required", Message:"workspace is required to resolve project slug"}`. Do not invent another MCP-only error code.

- [x] **Step 5: Verify**

Run:

```bash
go test ./internal/mcpserver -run 'Test.*Scope|Test.*Auth' -count=1
go test ./internal/app ./internal/httpapi -count=1
```

Expected: PASS.

### Task 10: 实现 stdio MCP CLI

**Files:**
- Create: `internal/cli/mcp.go`
- Modify: `internal/cli/root.go`
- Modify: `cmd/taskg/main.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: Write CLI integration test**

Add test that starts:

```bash
taskg --db <db> mcp stdio
```

and sends MCP initialize/listTools JSON-RPC through stdin using SDK client command transport if possible. Assert:

- stdout contains only JSON-RPC messages。
- stderr may contain logs but no protocol data。
- no M5 migration warning appears on stdout。

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./tests/integration -run TestMCPStdio -count=1
```

Expected: FAIL because command missing.

- [x] **Step 3: Implement `taskg mcp stdio`**

Create `newMCPCommand(opts)` and `newMCPStdioCommand(opts)`:

- resolve config like local CLI。
- open SQLite store。
- construct `mcpserver.NewServer(Options{Mode: ModeStdio, ...})`。
- run `server.Run(cmd.Context(), &mcp.StdioTransport{})`。

Register in `internal/cli/root.go`.

- [x] **Step 4: Suppress stdout warning**

In `cmd/taskg/main.go`, update `skipsMigrationWarning` to skip `mcp` command. Warning can be omitted entirely for MCP protocol safety.

- [x] **Step 5: Verify**

Run:

```bash
go test ./internal/cli ./tests/integration -run 'TestMCPStdio|TestExecute' -count=1
```

Expected: PASS.

### Task 11: 挂载 HTTP `/mcp`

**Files:**
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/middleware.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `internal/cli/server.go` if options need version/mcp setup.

- [x] **Step 1: Write HTTP MCP auth tests**

Tests:

- `POST /mcp` without token returns 401。
- `GET /mcp` without token returns 401, so Streamable HTTP/SSE entry is also protected。
- `/mcp` receives request id/access log/panic/body limit middleware。
- `POST /mcp` body over limit returns 413。
- `GET /mcp` Streamable HTTP/SSE setup is not used for body-limit assertions unless SDK docs show GET carries a request body。
- SDK Streamable HTTP body/session limit conclusion is recorded in `SDK_NOTES.md`。

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/httpapi -run 'TestMCP' -count=1
```

Expected: FAIL because `/mcp` route missing.

- [x] **Step 3: Confirm middleware and SDK request context**

Before coding, append a short section to `internal/mcpserver/SDK_NOTES.md`:

- current `internal/httpapi/router.go` middleware order is `requestIDMiddleware -> recovererMiddleware -> accessLogMiddleware -> bodyLimitMiddleware -> authMiddleware` for protected routes。
- Streamable HTTP handler registration style required by the installed SDK。
- whether the SDK server factory receives `*http.Request` and preserves the original `r.Context()`。

- [x] **Step 4: Mount SDK handler**

In router:

```go
mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
    return mcpserver.NewServer(mcpserver.Options{Store: s.store, Clock: s.effectiveClock(), Mode: mcpserver.ModeHTTP, Stderr: s.stderr})
}, nil)
api.With(s.authMiddleware).Handle("/mcp", mcpHandler)
```

Use the concrete handler registration recorded in `SDK_NOTES.md` rather than guessed SDK calls. `/mcp` must be mounted on the same `api` chi router so it receives, in order, request id, recoverer, access log, body limit, and then auth middleware. Register every HTTP method required by the installed SDK, including GET if Streamable HTTP uses GET for SSE.

Construct the `mcp.Server` according to `SDK_NOTES.md` session guidance. If the SDK expects a long-lived server for Streamable HTTP sessions, create/reuse one server instance from `httpapi.Server` options instead of constructing a new one for every request.

- [x] **Step 5: Preserve auth context for MCP**

Implement exactly one of these verified paths and record which one in `SDK_NOTES.md`:

- Path A: SDK server factory receives the original `*http.Request`。`RuntimeFactory.ServiceForHTTP` reads auth from `r.Context()` inside tool handlers。
- Path B: SDK does not preserve request context。Wrap the SDK handler after `authMiddleware`, capture authenticated request auth in the factory closure, and pass it into `mcpserver.Options` or `RuntimeFactory` without using package globals。

- [x] **Step 6: Verify**

Run:

```bash
go test ./internal/httpapi ./internal/mcpserver -run 'TestMCP|Test.*Auth' -count=1
```

Expected: PASS.

### Task 12: Phase 1 full verification and commit

**Files:**
- All files touched in Chunk 4.

- [x] **Step 1: Run verification**

Run:

```bash
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test ./internal/mcpserver ./internal/httpapi ./internal/cli ./tests/integration -count=1
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
rm -f taskg
```

Expected: PASS.

- [x] **Step 2: Commit**

```bash
git add internal/mcpserver internal/httpapi internal/cli cmd/taskg/main.go tests/integration
git commit -m "feat: 添加 MCP 传输与鉴权基础"
```

---

## Chunk 5: Phase 2 核心 task/report/urgency tools

### Task 13: 建立 tool schema golden 测试框架

**Files:**
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/schema_test.go`
- Create: `internal/mcpserver/testdata/*.schema.json`

- [x] **Step 1: Write golden test**

In `schema_test.go`, connect SDK client and call `ListTools`. Serialize each tool input schema to deterministic JSON and compare to:

```text
internal/mcpserver/testdata/<tool-name>.schema.json
```

Tool filename convention: replace `.` with `_`, e.g. `task_add.schema.json`. Reuse the golden helper and `-update` flag created in Task 7; this task should add tool registrations and golden files, not a second golden framework.

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run TestToolSchemasMatchGolden -count=1
```

Expected: FAIL until tools/goldens exist.

- [x] **Step 3: Add registration helper**

In `server.go`:

```go
func registerTools(s *mcp.Server, opts Options) {
    registerTaskTools(s, opts)
    registerReportTools(s, opts)
}
```

For this task, register schema-only tool definitions with input structs. If a handler is required by the SDK before the real implementation lands, return a business error result with code `tool_not_implemented` and message `not implemented`; replace these handlers in the following tasks. Do not use `mcp_internal` for intentional temporary handlers.

- [x] **Step 4: Generate golden files**

Use the golden update flag in `schema_test.go`:

```go
var updateSchemaGoldens = flag.Bool("update", false, "update schema golden files")
```

Inside the schema comparison helper if Task 7 did not already add this exact helper:

```go
if *updateSchemaGoldens {
    if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
        t.Fatalf("update golden %s: %v", goldenPath, err)
    }
    return
}
```

Generate files with:

```bash
go test ./internal/mcpserver -run TestToolSchemasMatchGolden -update -count=1
```

- [x] **Step 5: Verify**

Run:

```bash
go test ./internal/mcpserver -run TestToolSchemasMatchGolden -count=1
```

Expected: PASS.

### Task 14: 实现 task.add/query/get tools

**Files:**
- Create/Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/integration_test.go`
- Update: `internal/mcpserver/testdata/task_add.schema.json`
- Update: `internal/mcpserver/testdata/task_query.schema.json`
- Update: `internal/mcpserver/testdata/task_get.schema.json`

- [x] **Step 1: Write integration tests**

Tests:

- `task.add` creates task with explicit `project_id` and returns UUID。
- project-scoped token with a single project allowlist and missing `project`/`project_id` returns error, not auto-fill。
- `task.query` default excludes deleted, limit defaults 200 and max 1000。
- `task.query include_completed=true` includes completed tasks。
- `task.query include_deleted=true` includes deleted tasks。
- `task.get` by UUID returns task。
- project allowlist outside task returns `task_not_found` as business error (`IsError=true`)。
- HTTP MCP `task.get` rejects numeric working-set IDs with existing `task_uuid_invalid` business error；stdio MCP may resolve working-set IDs through the local working set。

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCPTask(Add|Query|Get)' -count=1
```

Expected: FAIL.

- [x] **Step 3: Implement input structs**

Define:

```go
type TaskAddInput struct {
    Workspace   string   `json:"workspace,omitempty" jsonschema:"workspace slug or UUID"`
    Project     string   `json:"project,omitempty" jsonschema:"project slug in the effective workspace"`
    ProjectID   string   `json:"project_id,omitempty" jsonschema:"stable project UUID"`
    Description string   `json:"description" jsonschema:"task description"`
    Tags        []string `json:"tags,omitempty"`
    Priority    string   `json:"priority,omitempty"`
    Due         *int64   `json:"due,omitempty" jsonschema:"unix seconds"`
    Wait        *int64   `json:"wait,omitempty" jsonschema:"unix seconds"`
    Scheduled   *int64   `json:"scheduled,omitempty" jsonschema:"unix seconds"`
    Until       *int64   `json:"until,omitempty" jsonschema:"unix seconds"`
    Annotations []string `json:"annotations,omitempty"`
}
```

Keep date/time fields aligned with `internal/httpapi/tasks.go:addTaskRequest` and `app.AddInput`: `due` / `wait` / `scheduled` / `until` are nullable Unix seconds (`*int64`)。M7 MCP must not accept human-readable date strings here; if that is needed later, add a separate parsing task. Do not include `recur` in M7 `task.add` unless the spec is updated first.

`annotations` is in the M7 spec. Implement it atomically: prefer extending `app.AddInput` so task creation and initial annotations are written in one app transaction and one coherent audit flow. If that is too invasive, wrap `app.Add` + annotation writes in a new app-layer method such as `AddWithAnnotations`; do not implement two independent MCP handler calls where task creation can succeed and annotation writing can fail.

Also define concrete input structs for:

```go
type TaskQueryInput struct {
    Workspace        string `json:"workspace,omitempty"`
    Project          string `json:"project,omitempty"`
    ProjectID        string `json:"project_id,omitempty"`
    Query            string `json:"query,omitempty"`
    Status           string `json:"status,omitempty"`
    Limit            int    `json:"limit,omitempty"`
    IncludeCompleted bool   `json:"include_completed,omitempty"`
    IncludeDeleted   bool   `json:"include_deleted,omitempty"`
}

type TaskGetInput struct {
    Workspace string `json:"workspace,omitempty"`
    Project   string `json:"project,omitempty"`
    ProjectID string `json:"project_id,omitempty"`
    ID        string `json:"id"`
}
```

- [x] **Step 4: Implement handlers through app service**

Each handler:

- build scope input from workspace/project/project_id。
- call factory with correct capability/permission。
- call app service method。
- render using existing render helpers or concise text。
- return `ToolEnvelope{Data: map[string]any{"task": taskView}, Rendered: rendered}`。

- [x] **Step 5: Verify**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCPTask(Add|Query|Get)|TestToolSchemasMatchGolden' -count=1
```

Expected: PASS.

### Task 15: 实现 task.modify/done/delete/annotate/depends/start/stop tools

**Files:**
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/integration_test.go`
- Update: corresponding schema golden files.

- [x] **Step 1: Write integration tests**

Tests:

- `task.modify` changes description and add-only depends。
- `task.modify` with `clear_depends=true` clears then adds。
- `task.depends` is equivalent to narrow `task.modify` depends behavior。
- `task.done` completes task and writes audit。
- `task.delete` returns `data.task.status = deleted`。
- `task.annotate` adds annotation。
- `task.start` / `task.stop` update start state。
- project-scoped token cannot move task outside allowlist。

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCPTask(Modify|Done|Delete|Annotate|Depends|Start|Stop)' -count=1
```

Expected: FAIL.

- [x] **Step 3: Implement handlers**

Use app methods already used by CLI:

- `Modify` for modify/depends。
- `Done`。
- `Delete`。
- `Annotate`。
- `Start` / `Stop`。

Do not add `ReplaceDepends`.

Define and golden-test input structs that include all spec fields:

- `task.modify`: `id`, `workspace`, `project`, `project_id`, `description`, `priority`, `due`, `wait`, `scheduled`, `until`, `tags`, `remove_tags`, `udas`, `clear`, `depends`, `clear_depends`。
- `task.depends`: `id`, `workspace`, `depends`, `clear_depends`。

Use this struct shape as the starting point:

```go
type TaskModifyInput struct {
    Workspace      string            `json:"workspace,omitempty"`
    Project        string            `json:"project,omitempty"`
    ProjectID      string            `json:"project_id,omitempty"`
    ID             string            `json:"id"`
    Description    *string           `json:"description,omitempty"`
    Priority       *string           `json:"priority,omitempty"`
    Due            *int64            `json:"due,omitempty"`
    Wait           *int64            `json:"wait,omitempty"`
    Scheduled      *int64            `json:"scheduled,omitempty"`
    Until          *int64            `json:"until,omitempty"`
    Tags           []string          `json:"tags,omitempty"`
    RemoveTags     []string          `json:"remove_tags,omitempty"`
    UDAs           map[string]string `json:"udas,omitempty"`
    Clear          []string          `json:"clear,omitempty"`
    Depends        []string          `json:"depends,omitempty"`
    ClearDepends   bool              `json:"clear_depends,omitempty"`
}
```

Map `clear` entries onto existing `app.ModifyInput` clear booleans / clear slices. Current `app.ModifyInput` already has `UDAs` and `ClearUDAs`; if more clear targets are accepted by the MCP schema, add explicit mapping and tests rather than interpreting arbitrary field names dynamically.

If `project_id` is present in `task.modify`, extend `app.ModifyInput` or add a resolver before calling `Modify` so project slug/id mismatch and project scope checks remain in app/service logic. Do not silently convert `project_id` to a slug in the MCP handler without checking workspace ownership and allowlist.

- [x] **Step 4: Verify audit**

In tests, after write tool call, query `svc.ListAudit(AuditListInput{Limit: 10})` or DB audit repository to assert action exists. If the test uses `svc.ListAudit`, create the fixture token/service with `audit:read` capability and an app role that can read audit; otherwise the audit assertion should fail with permission denial instead of testing the write behavior.

- [x] **Step 5: Verify**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCPTask(Modify|Done|Delete|Annotate|Depends|Start|Stop)|TestToolSchemasMatchGolden' -count=1
```

Expected: PASS.

### Task 16: 实现 report.run 与 urgency.explain

**Files:**
- Create/Modify: `internal/mcpserver/tools_report.go`
- Modify: `internal/mcpserver/integration_test.go`
- Update: `internal/mcpserver/testdata/report_run.schema.json`
- Update: `internal/mcpserver/testdata/urgency_explain.schema.json`

- [x] **Step 1: Write tests**

Tests:

- `report.run name=next` returns tasks and rendered text。
- `report.run limit=1001` returns `api_bad_limit` business error。
- `report.run limit=1` returns at most one task。
- project-scoped token report only sees allowlist project。
- `urgency.explain` returns `data.urgency` and `data.factors`。

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCP(ReportRun|UrgencyExplain)' -count=1
```

Expected: FAIL.

- [x] **Step 3: Implement handlers**

Use existing app/report/urgency paths. Do not recalculate formulas in MCP layer.

`report.run` result shape must be built explicitly:

- `data.report.name` is the requested report name。
- `data.tasks` is the task array。
- `rendered` uses existing report/list rendering helpers or a concise summary。

Current `app.ReportResult` only carries tasks; do not assume it already contains report metadata.

For limit:

- default 200。
- max 1000。
- invalid -> `api_bad_limit` business error。
- REST report limit is not required by the M7 spec. Do not add `limit` to `/api/v1/reports/{name}` unless Task 2 intentionally adds report limit while threading `app.ListInput.Limit`; if REST report limit is added, document it in OpenAPI and add REST tests in Task 2.

- [x] **Step 4: Verify**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCP(ReportRun|UrgencyExplain)|TestToolSchemasMatchGolden' -count=1
```

Expected: PASS.

### Task 17: Phase 2 full verification and commit

**Files:**
- All files touched in Chunk 5.

- [x] **Step 1: Run verification**

Run:

```bash
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test ./internal/mcpserver -count=1
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
rm -f taskg
```

Expected: PASS.

- [x] **Step 2: Commit**

```bash
git add internal/mcpserver internal/app internal/httpapi internal/remote docs/openapi/taskg-v1.yaml
git commit -m "feat: 添加 MCP 核心任务工具"
```

---

## Chunk 6: Phase 3 企业上下文 tools/resources

### Task 18: 实现 workspace/project/context/config tools

**Files:**
- Create/Modify: `internal/mcpserver/tools_workspace.go`
- Create/Modify: `internal/mcpserver/tools_project.go`
- Create/Modify: `internal/mcpserver/tools_context.go`
- Create/Modify: `internal/mcpserver/tools_config.go`
- Modify: `internal/mcpserver/integration_test.go`
- Update: schema golden files.

- [x] **Step 1: Write tests**

Tests:

- `workspace.list` respects token workspace allowlist。
- `workspace.current` returns current effective workspace。
- `project.list` respects project allowlist。
- `project.get` includes allowed `agent.*` summary only。
- `project.get` for project outside project allowlist returns `project_scope_denied`, matching HTTP project info behavior。
- `project.current` returns null when no effective project。
- `context.show` reads service active context。
- `context.set name=none` clears active context and uses `context:write + PermissionContextUse`。
- `config.get scope=local` works in stdio mode and reads TOML/runtime config。
- `config.get scope=local` fails in HTTP mode。
- `config.set scope=workspace agent.background` returns `config_key_unsupported`。
- `config.set scope=workspace color=auto` returns `config_key_unsupported`。
- `config.set scope=project agent.background` succeeds if value <=16KB。
- `config.get scope=project agent.background` succeeds。
- `config.get scope=project agent.handoff` succeeds。
- oversized `agent.background` returns `config_value_too_large`。

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCP(Workspace|Project|Context|Config)' -count=1
```

Expected: FAIL.

- [x] **Step 3: Implement workspace/project tools**

Use app methods:

- `ListWorkspaces`
- `WorkspaceInfo`
- `ListProjects`
- `ProjectInfo`
- `ProjectConfigList/Get`

Ensure DTO matches REST response fields.

- [x] **Step 4: Implement context tools**

Use app methods:

- `ContextShow`
- `UseContext`
- `ContextNone`

Do not implement define/delete.

- [x] **Step 5: Implement config tools**

Rules:

- HTTP `scope=local`: business error。
- stdio `scope=local`: read local runtime config/TOML only。
- confirm `app.IsBusinessConfigKey` exists and is exported; if its signature changes before M7 implementation, add/adjust the app helper first and cover it with unit tests。
- `scope=workspace`: in the MCP tool handler, first reject unsupported write keys with `app.IsBusinessConfigKey(key)` and return `config_key_unsupported`; then call app `GetConfig/SetConfig`。Do not rely only on `Service.SetConfig` because it also permits personal render keys for local CLI compatibility.
- `scope=project`: app `ProjectConfigGet/Set` with project ref。
- `agent.*`: project only。

Current app has `ConfigValues()` but no public `GetConfig` / `SetConfig` pair. Add focused app methods for workspace config get/set or reuse existing config internals behind app-layer methods with `PermissionProjectConfigRead/Write` or the existing workspace config permissions; do not have MCP handlers write SQLite meta directly.

Update project config whitelist in `internal/app/project_config.go` to match M7 spec:

- keep `agent.background`。
- keep `agent.constraints`。
- add `agent.default_context`。
- add `agent.handoff`。
- decide compatibility for existing `context.default`: keep as legacy alias if existing tests depend on it, but M7 Agent tools/resources should emit `agent.default_context`。

- [x] **Step 6: Add 16KB `agent.*` limit**

In `internal/app/project_config.go`, reject values longer than 16KB for `agent.*` keys:

```go
const agentConfigValueMaxBytes = 16 * 1024

if strings.HasPrefix(key, "agent.") && len(value) > agentConfigValueMaxBytes {
    return RuntimeError{Code: "config_value_too_large", Message: "config value too large"}
}
```

Add app service unit test.

- [x] **Step 7: Verify**

Run:

```bash
go test ./internal/app -run 'TestProjectConfig.*TooLarge|TestProjectConfig' -count=1
go test ./internal/mcpserver -run 'TestMCP(Workspace|Project|Context|Config)|TestToolSchemasMatchGolden' -count=1
```

Expected: PASS.

### Task 19: 实现 MCP resources

**Files:**
- Create/Modify: `internal/mcpserver/resources.go`
- Modify: `internal/mcpserver/integration_test.go`

- [x] **Step 1: Write resource tests**

Tests:

- `ReadResource taskg://workspace/current` returns workspace id/slug/name/role/context summary。
- `ReadResource taskg://workspace/{workspace_id}` respects workspace allowlist。
- `ReadResource taskg://project/{project_id}` returns project metadata and allowed `agent.*` keys。
- `ReadResource taskg://context/current` returns active context/effective workspace/project scope。
- list-style URI like `taskg://project/{id}/tasks` is not registered.

- [x] **Step 2: Run and confirm failure**

Run:

```bash
go test ./internal/mcpserver -run TestMCPResources -count=1
```

Expected: FAIL.

- [x] **Step 3: Register resources**

In `resources.go`, implement:

```go
func registerResources(s *mcp.Server, opts Options)
```

Register exact resources/templates allowed by spec.

- [x] **Step 4: Implement resource handlers through app service**

Each handler:

- parse URI。
- build scope。
- use app service。
- return text or JSON resource content supported by SDK。

Do not include token, audit details, member private data, or full arbitrary config.

- [x] **Step 5: Verify**

Run:

```bash
go test ./internal/mcpserver -run TestMCPResources -count=1
```

Expected: PASS.

### Task 20: Full MCP flow integration

**Files:**
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: Add end-to-end Agent flow test**

Test sequence over HTTP MCP:

1. `project.get` reads project background。
2. `task.query` returns empty list。
3. `task.add` creates explicit project task。
4. `urgency.explain` returns factors。
5. `task.done` completes task。
6. audit list confirms write audit via existing REST/API or app service.

- [x] **Step 2: Add project-scope denial flow**

Create token allowlisted to project A, task in project B:

- `task.query project_id=B` returns scope error。
- `task.get` B task returns `task_not_found` business error。
- `project.list` only shows project A。

- [x] **Step 3: Run and verify**

Run:

```bash
go test ./internal/mcpserver -run 'TestMCPAgentFlow|TestMCPProjectScope' -count=1
go test ./tests/integration -run TestMCPStdio -count=1
```

Expected: PASS.

### Task 21: Phase 3 full verification and commit

**Files:**
- All files touched in Chunk 6.

- [x] **Step 1: Run verification**

Run:

```bash
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test -race ./internal/mcpserver ./internal/app ./internal/httpapi
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' && exit 1 || true
rm -f taskg
```

Expected: PASS.

- [x] **Step 2: Commit**

```bash
git add internal/mcpserver internal/app tests/integration
git commit -m "feat: 添加 MCP 企业上下文工具"
```

If Task 18 touches `internal/app/project_config.go` or shared OpenAPI/schema docs, include those files in this commit as well; do not leave app whitelist changes unstaged.

---

## Chunk 7: 文档同步与 M7 收尾

### Task 22: README / ROADMAP / requirements 同步

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/requirements.md`
- Modify: `docs/superpowers/plans/2026-06-01-taskg-m7-implementation.md`

- [x] **Step 1: Update README**

Add:

- Go 1.25。
- `taskg mcp stdio` example。
- `taskg server --listen :8080` exposes `/mcp`。
- HTTP MCP Bearer token example。
- list of core MCP tools。
- note that `/mcp` is not OpenAPI。

- [x] **Step 2: Update ROADMAP**

Mark M7 as completed or in-progress per actual state. Include:

- official MCP SDK。
- stdio / Streamable HTTP。
- M6 remote management closure。
- non-goals remain M8/backlog。

- [x] **Step 3: Update requirements**

Ensure final product architecture includes MCP implementation and Go 1.25.

- [x] **Step 4: Check plan boxes**

Only mark tasks complete if actually implemented and verified. Do not mass-check boxes without evidence.

- [x] **Step 5: Verify docs**

Run:

```bash
rg -n 'Go 1\\.22|M7 \\| 待规划|TODO|TBD' README.md ROADMAP.md docs/requirements.md docs/superpowers/plans/2026-06-01-taskg-m7-implementation.md
git diff --check
```

Expected: no stale Go 1.22 or M7 pending state unless intentionally historical.

### Task 23: Final acceptance

**Files:**
- Entire repository.

- [x] **Step 1: Run final verification bundle**

Run:

```bash
test -z "$(gofmt -l internal cmd tests)" && go vet ./...
go test -race ./internal/mcpserver ./internal/app ./internal/httpapi
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' && exit 1 || true
rm -f taskg
```

Expected: all commands exit 0.

- [x] **Step 2: Manual smoke test stdio MCP**

Run a small SDK client or existing integration helper against:

```bash
taskg --db <tmpdb> mcp stdio
```

Expected:

- initialize succeeds。
- `tools/list` returns all required tools。
- stdout contains only MCP protocol frames。

- [x] **Step 3: Manual smoke test HTTP MCP**

Run:

```bash
taskg server --listen 127.0.0.1:0 --db <tmpdb>
```

Use SDK Streamable HTTP client with Bearer token.

Expected:

- no token -> 401。
- valid token -> initialize/listTools/callTool succeeds。
- project-scoped token cannot access other project。

- [x] **Step 4: Commit docs sync**

```bash
git add README.md ROADMAP.md docs/requirements.md docs/superpowers/specs/2026-06-01-taskg-m7-design.md docs/superpowers/plans/2026-06-01-taskg-m7-implementation.md
git commit -m "docs: 同步 M7 完成状态"
```

---

## 推荐子代理拆分

执行时建议使用 `superpowers:subagent-driven-development`，按不重叠写入范围分派：

- Agent A：Chunk 1，Go 升级与文档版本同步。
- Agent B：Chunk 2 的 REST/remote CLI，不碰 `internal/mcpserver`。
- Agent C：Chunk 3 / Phase 0c 的 MCP SDK 骨架和 `SDK_NOTES.md`，提交后停止，不进入 Phase 1。
- Agent C2：Chunk 4 / Phase 1 的 MCP transport、auth、scope、result，不碰 task tools。
- Agent D：Chunk 5 的 task/report/urgency tools；必须等 Agent B 的 app list limit 和 Agent C2 的 RuntimeFactory 合入后开始。
- Agent E：Chunk 6 的 enterprise tools/resources；必须等 Agent D 合入后开始，避免同时修改 `internal/mcpserver/server.go` 和 shared app config。
- Main agent：集成、冲突处理、OpenAPI/docs、最终验证。

不要让多个 agent 同时编辑同一文件；如果必须共享 `internal/mcpserver/server.go`，先让一个 agent 建立注册接口，其他 agent 只新增各自 `tools_*.go` 并在最后由 main agent 集成注册。

## 最终验收清单

- [x] Phase 0a、0b、0c 均独立提交且通过完整验证。
- [x] `go.mod` 为 Go 1.25。
- [x] 没有 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`。
- [x] `/api/v1/tasks` limit 默认 200、最大 1000。
- [x] 远程 management CLI 不触碰本地 DB。
- [x] `taskg mcp stdio` stdout 协议安全。
- [x] `taskg server` 暴露 `/mcp`。
- [x] HTTP MCP Bearer token 鉴权。
- [x] MCP tool schema golden tests 覆盖所有 tools。
- [x] project-scoped token 对 MCP task/query/get/report/resources 生效。
- [x] 业务错误走 `IsError=true`。
- [x] 写操作写 audit。
- [x] README、ROADMAP、requirements、OpenAPI 同步。
- [x] Final verification bundle 全部通过。
