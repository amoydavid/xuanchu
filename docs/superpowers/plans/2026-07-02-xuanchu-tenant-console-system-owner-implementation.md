# 璇础 Tenant Console System Owner 实现计划

> **面向编码代理：** 本计划执行时必须使用 `superpowers:executing-plans`。步骤使用 checkbox（`- [ ]`）跟踪进度。

**目标：** 完整实现 `tenant_access_token` 作为 workspace system owner credential 登录 Web Console、管理 workspace 内 user/member/token/workspace，并在 P2 支持 user-shaped actor 资源。

**当前实现状态（2026-07-02）：** P1/P2 均已实现。tenant token 作为 owner-equivalent runtime actor 进入 HTTP/MCP/Web Console，可管理 user/member/token/workspace；admin workspace detail 可签发短期 tenant switch token；P2 actor schema 已覆盖 task link、project annotation、hook、notification sink、reminder rule、event notification rule、hook delivery 和 notification delivery。

**架构：** 继续复用 `api_tokens` 表，不新增 tenant session 表，不创建 system user。P1 让 tenant token 作为 owner-equivalent runtime actor 进入 HTTP/MCP/Web Console；P2 把仍依赖 `created_by_user_id` 的资源升级为 actor model，允许 system actor 创建 hook、notification、reminder、annotation、task link 等资源。

**技术栈：** Go 1.25、GORM、SQLite/PostgreSQL、chi/Huma HTTP、MCP Go SDK、React/Vite、TanStack Query、Vitest。

---

## 来源 Spec

- `docs/superpowers/specs/2026-07-02-xuanchu-tenant-console-system-owner-design.md`

## 文件范围

后端 P1:

- Modify: `internal/auth/scope.go`, `internal/auth/token_test.go`
- Modify: `internal/storage/models.go`, `internal/storage/token_repo.go`, `internal/storage/migrate_sqlite.go`, `internal/storage/migrate_postgres.go`
- Modify: `internal/app/token.go`, `internal/app/request_scope.go`, `internal/app/runtime.go`, `internal/app/user.go`, `internal/app/workspace.go`
- Modify: `internal/httpapi/me.go`, `internal/httpapi/huma_routes.go`, `internal/httpapi/users.go`, `internal/httpapi/workspaces.go`, `internal/httpapi/tenant_tokens.go`, `internal/httpapi/admin.go`
- Modify: `internal/mcpserver/tools_user.go`, `internal/mcpserver/tools_member.go`, `internal/mcpserver/tools_token.go`, `internal/mcpserver/tools_workspace.go`

Web P1:

- Modify: `web/src/features/workspace/session/useMe.ts`, `web/src/features/workspace/session/workspace-token.ts`, `web/src/features/workspace/session/workspace-api.ts`
- Modify: `web/src/pages/LoginPage.tsx`, `web/src/routes/workspace/WorkspaceRootRoute.tsx`, `web/src/components/AppShell.tsx`
- Modify: `web/src/features/admin/workspaces/admin-workspace-detail-page.tsx`
- Modify: `web/src/features/workspace/tokens/*`, `web/src/features/admin/tokens/*`

后端 P2:

- Modify: `internal/storage/models.go`, migrations, and repos for task links, project annotations, hooks, notification sinks, reminder rules, event notification rules, delivery rows.
- Modify: `internal/app/workspace.go`, `internal/app/project.go`, `internal/app/hook*.go`, `internal/app/notification*.go`, `internal/app/hook_event.go`
- Modify: HTTP/MCP response shaping for `created_by` / `actor`.

文档:

- Modify: `README.md`, `ROADMAP.md`, `docs/manual/web-console.md`, `docs/manual/mcp.md`, `docs/manual/team-workspaces-projects.md`, this plan, and the source spec status.

---

## Chunk 1: P1 后端 Scope、Current Credential 与 Tenant 管理权限

### Task 1: Tenant scope 白名单匹配 P1/P2

- [x] Write failing tests in `internal/auth/token_test.go` proving tenant wildcard includes `user:read/write`, `member:read/write`, `token:read/write`, `workspace:read/write`, and P2 `hook:write`, `notification:write`, `reminder:write`; reject forbidden `impersonate`.
- [x] Run `go test ./internal/auth -run Tenant -count=1` and confirm failure.
- [x] Update `internal/auth/scope.go`.
- [x] Re-run focused test.

### Task 2: 新增 `/api/v1/credentials/current`

- [x] Write failing HTTP tests proving tenant token returns `actor_type=tenant_access_token`, owner effective role, bound workspace, and P2 capabilities including hook/notification/reminder write scopes.
- [x] Implement app/http response helpers without changing `/api/v1/me`; tenant token must still get `tenant_actor_not_user` from `/me`.
- [x] Register Huma route.
- [x] Re-run focused HTTP tests.
- [ ] Add explicit PAT/Agent/acting response tests if the Web Console migration needs stronger regression coverage.

### Task 3: Tenant token 可管理 user/member/workspace/token 资源

- [x] Write failing HTTP tests for tenant token with `user:write` and `member:write`.
- [x] Remove fixed `rejectTenantActor` guards from P1 user/member paths and rely on `scopedService` + owner-equivalent runtime.
- [x] Keep `/me` and active context use/none forbidden.
- [x] Ensure normal tenant token lists exclude `purpose=admin_tenant_switch`.
- [ ] Add explicit HTTP tests for tenant `workspace:write` and `token:write`.
- [ ] Keep `workspace_list`, `assignee:me`, and `X-Xuanchu-As` forbidden where applicable.

### Task 4: Admin tenant switch token

- [x] Write failing HTTP tests for `POST /api/v1/admin/workspaces/{workspace}/tenant-access-sessions`.
- [x] Add `api_tokens` metadata fields: `issued_via`, `issued_by_admin_token_id`, `issued_by_admin_token_name`, `purpose`.
- [x] Implement short-lived tenant token creation with default `2h`, max `24h`, `purpose=admin_tenant_switch`, raw token returned once.
- [x] Add `include_admin_switch=true` support to admin tenant token list.
- [ ] Add explicit admin-switch revocation-path regression test.

## Chunk 2: P1 MCP 与 Web Console

### Task 5: MCP system owner tools

- [x] Write integration tests proving tenant token can call `user_*`, `member_*`, `token_*`, `workspace_info`, `workspace_modify`, and still cannot call `me_get`, `user_use`, `workspace_use`, `workspace_list`, `context_set`, `context_none`.
- [x] Update tool guards and service construction.

### Task 6: Web Console credential current 与 tenant 登录

- [x] Update LoginPage to validate with `/api/v1/credentials/current` and accept `xuanchu_tenant_`.
- [x] Replace `useMe` with credential-current query in WorkspaceRootRoute.
- [x] Show tenant actor as “系统身份”; keep `/me` semantics for user-only surfaces.
- [x] Add tests for tenant login and unauthorized cleanup.

### Task 7: Admin switch UI

- [x] Add “以 Tenant 身份进入” action to admin workspace detail.
- [x] Store raw tenant token in `xuanchu.console.token`; store tenant context in `xuanchu.console.tenant_context`; clear acting context.
- [x] Show return-to-admin action while preserving admin token.
- [x] Add tests.

## Chunk 3: P2 Actor Schema 升级

### Task 8: Storage actor model

- [x] Add actor columns for user-shaped resources.
- [x] Migrate existing rows to `actor_type=user`.
- [x] Update repos to read/write actor fields while preserving legacy compatibility.

### Task 9: App/HTTP/MCP 支持 tenant actor 创建资源

- [x] Update task links, project annotations, hook definitions, notification sinks, reminder rules, event notification rules, and delivery actor rows to accept tenant actor.
- [x] Update JSON/MCP response shape to include system actor instead of `task.UserInfo` where applicable.
- [x] Add tests for tenant-created resources and event delivery actor output.

## Chunk 4: 文档与验证

### Task 10: 文档对齐

- [x] Update `README.md`, `ROADMAP.md`, `docs/manual/web-console.md`, `docs/manual/mcp.md`, `docs/manual/team-workspaces-projects.md`.
- [x] Mark source spec and this plan with implementation status.

### Task 11: 最终验证

- [x] Run `git diff --check`.
- [x] Run `go test ./internal/auth ./internal/app ./internal/httpapi ./internal/mcpserver ./internal/storage -count=1`.
- [x] Run `go test ./...`.
- [x] Run `CGO_ENABLED=0 go test ./...`.
- [x] Run `CGO_ENABLED=0 go build ./cmd/xuanchu`.
- [x] Run `go vet ./...`.
- [x] Run `pnpm --dir web typecheck`.
- [x] Run `pnpm --dir web lint`.
- [x] Run `pnpm --dir web test`.
- [x] Run `pnpm --dir web build`.
- [x] Run `pnpm --dir web run smoke:editing`.
