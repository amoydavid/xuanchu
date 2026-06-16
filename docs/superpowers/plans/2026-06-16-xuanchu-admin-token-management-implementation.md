# Xuanchu Admin 工作台 Token 管理 Implementation Plan

> **For agentic workers:** REQUIRED: 使用 superpowers:subagent-driven-development（若可用）或 superpowers:executing-plans 执行本计划。步骤用 checkbox（`- [ ]`）跟踪。

**Goal:** 在 server admin 工作台 `/admin/tokens` 提供跨 workspace 的 token 管控能力：列出全部 token（含完整 user info）、吊销、修改 name/scope/过期。

**Architecture:** Storage 加 `ListAll` + `UserRepository.ListByIDs`（批量，避免 N+1）。App 新增 `admin_token.go`，复用 `appendAdminAuditInTx` 审计模式，绕过 workspace role（admin 即最高权限）。HTTP 加 3 个 admin token 路由。前端新增 `admin/tokens/` 模块，复用普通 console 的 `scope-editor`，扩展 `adminApiPatch/Delete`。

**安全/内存要点：**
- `ListAll` 全量加载，但单实例 token 数量有限（运维场景非海量数据）；user 解析用 `ListByIDs` 单次批量查询，O(1) DB round-trip，内存仅多一张 `map[userID]User`。
- admin modify **不接 workspace/project**（避免跨 workspace 重绑定的语义风险）。
- 所有操作经 `adminAuthMiddleware`，审计 payload 标记 `admin:true`。

**Tech Stack:** Go 1.25、GORM、github.com/glebarez/sqlite、chi；React 19、TanStack Query、shadcn/ui、Vitest、i18next

**Spec:** `docs/superpowers/specs/2026-06-16-xuanchu-admin-token-management-design.md`

---

## Chunk 1：后端 Storage 层（批量查询基础）

### Task 1：TokenRepository.ListAll + UserRepository.ListByIDs

**Files:**
- Modify: `internal/storage/token_repo.go`
- Modify: `internal/storage/user_repo.go`

- [ ] **Step 1：** `token_repo.go` 新增 `ListAll(includeRevoked bool) ([]ApiTokenEntry, error)`：`SELECT *` 按 `created_at DESC, id DESC`，`includeRevoked=false` 时 `WHERE revoked_at IS NULL`，复用现有 `apiTokenEntry` 转换。无分页（token 总量有限，admin 需全貌）。
- [ ] **Step 2：** `user_repo.go` 新增 `ListByIDs(ids []string) ([]User, error)`：`WHERE id IN (?)`，GORM 自动参数绑定（防注入）。空 ids 返回空切片不查 DB。用于 admin list 批量解析 user，避免 N+1。
- [ ] **Step 3：** 验证 `CGO_ENABLED=0 go build ./internal/storage/...`

### Task 2：Storage 层测试

**Files:**
- Modify: `internal/storage/token_repo_test.go`
- Modify: `internal/storage/user_repo_test.go`（若不存在则新建）

- [ ] **Step 1：** `TestTokenRepositoryListAll`：建多个 token（含已吊销），验证 `ListAll(true)` 返回全部、`ListAll(false)` 过滤已吊销、排序正确。
- [ ] **Step 2：** `TestUserRepositoryListByIDs`：建多个 user，验证按 ID 批量查返回正确子集、空 ids 不报错。
- [ ] **Step 3：** 验证 `CGO_ENABLED=0 go test ./internal/storage/... -run 'TestTokenRepository|TestUserRepository'`

---

## Chunk 2：后端 App 层（admin token 操作）

### Task 3：admin_token.go — list/revoke/modify

**Files:**
- Create: `internal/app/admin_token.go`

- [ ] **Step 1：** `AdminListTokens(includeRevoked bool) ([]TokenView, error)`：
  - 调 `tokenRepo.ListAll(includeRevoked)`
  - **批量 user 解析**（内存/性能关键）：收集 distinct `UserID` → `userRepo.ListByIDs(ids)` 一次查询 → 建 `map[string]storage.User` → 填充每个 `TokenView.User`（name/email）。未找到的 user fallback `{ID, Name: ID}`（与 AGENTS.md §用户信息规范一致）。
  - 解析 scopes/workspace_ids/project_ids（复用 `unmarshalStringSlice`）。
- [ ] **Step 2：** `AdminRevokeToken(tokenRef, adminTokenName string) error`：
  - `store.Transaction` 内：`tokenRepo.GetByIDOrPrefix(ref)` → 校验未吊销（已吊销返回 `token_revoked`）→ `Revoke(id, clock.Unix())`
  - 审计 `appendAdminAuditInTx`，动作 `admin.token.revoke`，`ActorUserID: nil`，payload `{admin:true, admin_token_name, token_id, token_name, user_id}`
- [ ] **Step 3：** `AdminModifyToken(input AdminModifyTokenInput) (*TokenView, error)`：
  ```go
  type AdminModifyTokenInput struct {
      TokenID         string
      Name            *string
      Scopes          *[]string
      ExpiresIn       *time.Duration
      AdminTokenName  string
  }
  ```
  - `GetByID` → 校验未吊销/未过期（与普通 ModifyToken 一致）
  - **不校验 owner**（admin 即最高权限）
  - scope 校验：`ValidateTokenCreate` 用 existing workspace（admin 不改 workspace），impersonate 恒允许
  - expires 逻辑与普通 ModifyToken 一致（0=清除，负数拒绝）
  - 审计 `admin.token.modify`，payload 含 `changes: ChangedFields()`
  - **不支持** WorkspaceRefs/ProjectRefs（非目标）
- [ ] **Step 4：** 复用 `tokenEntryToView` + 批量 user 填充（抽一个 `fillUserInfos(rows) []TokenView` 内部 helper，供 list 和 modify 返回共用）
- [ ] **Step 5：** 验证 `CGO_ENABLED=0 go build ./internal/app/...`

### Task 4：App 层测试

**Files:**
- Create: `internal/app/admin_token_test.go`

- [ ] **Step 1：** `TestAdminListTokens`：建多个 workspace + user + token（PAT/agent/已吊销），验证返回全部、user info 含 name/email、IncludeRevoked 过滤。
- [ ] **Step 2：** `TestAdminListTokensBatchUserResolution`：多个 token 属于同一 user，验证 user 只查一次（可通过 token 数 > user 数验证 map 复用），fallback 用户正确。
- [ ] **Step 3：** `TestAdminRevokeToken`：吊销成功 + 审计 `admin.token.revoke` 含 `admin:true`；已吊销再吊销返回 `token_revoked`。
- [ ] **Step 4：** `TestAdminModifyTokenScopes`：改 scope 成功，审计含 changes；空 scope 被拒。
- [ ] **Step 5：** `TestAdminModifyTokenExpires`：设/清过期时间。
- [ ] **Step 6：** `TestAdminModifyTokenRejectsRevoked`：已吊销 token 修改被拒。
- [ ] **Step 7：** 验证 `CGO_ENABLED=0 go test ./internal/app/... -run 'TestAdmin'`

---

## Chunk 3：后端 HTTP 层

### Task 5：admin token handler + 路由 + capabilities

**Files:**
- Modify: `internal/httpapi/admin.go`
- Modify: `internal/httpapi/router.go`

- [ ] **Step 1：** `admin.go` 新增 handler（沿用 `app.NewService({DisableScopeBootstrap: true})` 模式）：
  - `handleAdminTokenList`：`GET /api/v1/admin/tokens?all=true` → `AdminListTokens(all=="true")`，返回 `[]tokenResponse`
  - `handleAdminTokenModify`：`PATCH /api/v1/admin/tokens/{tokenRef}`，body `{name?, scopes?, expires_in_seconds?}`（`*[]string` 指针语义，复用普通 modify 的请求结构和 `mergeRefs` 思路）→ `AdminModifyToken`
  - `handleAdminTokenRevoke`：`DELETE /api/v1/admin/tokens/{tokenRef}` → `AdminRevokeToken`
- [ ] **Step 2：** 复用普通 `tokenResponse`（含 user/workspace_ids/scopes），但 user 字段需填完整 info——确认 `tokenResponseFromView` 已用 `task.UserInfoToJSON`，admin list 返回的 view 已含完整 user。
- [ ] **Step 3：** `router.go` 注册 3 路由（`api.With(s.adminAuthMiddleware)` 组内）：
  ```go
  r.With(s.adminAuthMiddleware).Get("/api/v1/admin/tokens", s.handleAdminTokenList)
  r.With(s.adminAuthMiddleware).Patch("/api/v1/admin/tokens/{tokenRef}", s.handleAdminTokenModify)
  r.With(s.adminAuthMiddleware).Delete("/api/v1/admin/tokens/{tokenRef}", s.handleAdminTokenRevoke)
  ```
- [ ] **Step 4：** `handleAdminSession`（`admin.go` 硬编码 capabilities 数组）补 `"token:list"`、`"token:modify"`、`"token:revoke"`。
- [ ] **Step 5：** 验证 `CGO_ENABLED=0 go build ./internal/httpapi/...`

### Task 6：HTTP 层测试

**Files:**
- Modify: `internal/httpapi/admin_test.go`

- [ ] **Step 1：** `TestAdminTokenListHTTP`：admin token 列出全部 token，验证响应含 user name、workspace_ids。
- [ ] **Step 2：** `TestAdminTokenRevokeHTTP`：admin 吊销 token，验证 200 + 再次 list 显示已吊销。
- [ ] **Step 3：** `TestAdminTokenModifyHTTP`：admin PATCH 改 scope，验证返回更新。
- [ ] **Step 4：** `TestAdminTokenRequiresAdminAuth`：普通 token 访问 `/api/v1/admin/tokens` 返回 401。
- [ ] **Step 5：** 验证 `CGO_ENABLED=0 go test ./internal/httpapi/... -run 'TestAdminToken'`

---

## Chunk 4：前端 admin-api 扩展

### Task 7：adminApiPatch / adminApiDelete

**Files:**
- Modify: `web/src/features/admin/session/admin-api.ts`

- [ ] **Step 1：** 参照现有 `adminApiGet/Post` + `adminRequest`，新增 `adminApiPatch<T>(path, body)` / `adminApiDelete<T>(path)`，沿用 `assertAdminPath`（强制 `/api/v1/admin/` 前缀）+ `getAdminToken` + 401 清除。
- [ ] **Step 2：** 补 `admin-api.test.ts` 测试（Patch/Delete 路径校验 + header 注入）。
- [ ] **Step 3：** 验证 `cd web && pnpm test -- admin-api && pnpm typecheck`

---

## Chunk 5：前端 admin token 页面

### Task 8：admin-token-api.ts 类型与 helper

**Files:**
- Create: `web/src/features/admin/tokens/admin-token-api.ts`

- [ ] **Step 1：** 定义 `AdminTokenRow`（对齐 `tokenResponse`，user 含 name+email）。
- [ ] **Step 2：** 复用普通 console 的 `deriveTokenStatus`、`presetToExpiresSeconds`、`expiresSecondsToPreset`（从 `@/features/workspace/tokens/token-api` import，避免重复）。
- [ ] **Step 3：** 定义 `AdminTokenModifyInput`（name/scopes/expires_in_seconds，均可选，undefined=不改）。
- [ ] **Step 4：** 验证 `cd web && pnpm typecheck`

### Task 9：admin-tokens-page.tsx + Dialog + mutations

**Files:**
- Create: `web/src/features/admin/tokens/use-admin-token-mutations.ts`
- Create: `web/src/features/admin/tokens/admin-token-edit-dialog.tsx`
- Create: `web/src/features/admin/tokens/admin-token-revoke-dialog.tsx`
- Create: `web/src/features/admin/tokens/admin-tokens-page.tsx`

- [ ] **Step 1：** `use-admin-token-mutations.ts`：`useAdminModifyTokenMutation`（adminApiPatch）、`useAdminRevokeTokenMutation`（adminApiDelete），invalidate `["admin","tokens"]`。
- [ ] **Step 2：** `admin-token-edit-dialog.tsx`：复用普通 console 的 `ScopeEditor`（import from `@/features/workspace/tokens/scope-editor`）+ 过期时间选择。**无 workspace/project 编辑**。显示只读的 user/workspace 信息。`canImpersonate` 恒 true（admin）。
- [ ] **Step 3：** `admin-token-revoke-dialog.tsx`：AlertDialog 二次确认，显示 name/prefix/type/user + admin 审计提示。
- [ ] **Step 4：** `admin-tokens-page.tsx`：
  - `useQuery({queryKey:["admin","tokens"], queryFn: adminApiGet})` 拉 `GET /api/v1/admin/tokens?all=true`
  - 表格列：name / type / user（name+email 两行）/ workspaces（slug 或 ID，PAT 显示「全局」）/ scopes 数量 / 状态 / 最后使用 / 操作
  - 「显示已吊销」复选框（控制 all 参数，默认 true）
  - 已吊销行置灰、操作禁用
  - **无创建按钮**
- [ ] **Step 5：** 验证 `cd web && pnpm typecheck`

### Task 10：路由 + 导航 + i18n

**Files:**
- Create: `web/src/routes/admin/AdminTokensRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/admin/components/AdminShell.tsx`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/locales/zh-CN.ts`

- [ ] **Step 1：** `AdminTokensRoute.tsx`（极简，渲染 `<AdminTokensPage />`）。
- [ ] **Step 2：** `router.tsx`：lazy import + 加到 `adminGuardRoute.addChildren([adminDashboardRoute, adminTokensRoute])`，path `/admin/tokens`。
- [ ] **Step 3：** `AdminShell.tsx`：侧边栏 `<nav>` 加「Token 管理」链接（`/admin/tokens`，KeyRound 图标），与现有 bootstrap 链接同样式。
- [ ] **Step 4：** i18n（en/zh）：`admin.nav.tokens`、「显示已吊销」、token 字段标签（复用 `token.field.*`）、admin 审计提示、错误码（复用 `token.errors.*`）。新增 `admin.token.*` 命名空间存放 admin 专属文案（如「全局」PAT 标识、admin 审计提示）。
- [ ] **Step 5：** 验证 `cd web && pnpm typecheck && pnpm build`

---

## Chunk 6：前端测试

### Task 11：admin-tokens-page 测试

**Files:**
- Create: `web/src/features/admin/tokens/admin-tokens-page.test.tsx`

- [ ] **Step 1：** mock `adminApiGet` 返回 token 列表（含 user name+email、多 workspace、已吊销），验证列渲染。
- [ ] **Step 2：** 验证已吊销行置灰、操作禁用。
- [ ] **Step 3：** 验证「显示已吊销」复选框切换 query 参数。
- [ ] **Step 4：** 验证无创建按钮。
- [ ] **Step 5：** 验证 `cd web && pnpm test -- admin-tokens-page`

---

## Chunk 7：全量验证 + 文档同步

### Task 12：全量验证

- [ ] **Step 1：** 后端全量
  ```bash
  go test ./...
  CGO_ENABLED=0 go test ./...
  CGO_ENABLED=0 go build ./cmd/xuanchu
  ```
- [ ] **Step 2：** 前端全量
  ```bash
  cd web && pnpm test && pnpm typecheck && pnpm build
  ```
- [ ] **Step 3：** 重点回归：
  - `internal/app/admin_token_test.go`（新增）
  - `internal/httpapi/admin_test.go`（admin token 用例 + 现有 bootstrap 不回归）
  - `internal/storage/token_repo_test.go`（ListAll）
  - 普通 console `/tokens` 功能不回归（`web/src/features/workspace/tokens/`）

### Task 13：文档同步

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`

- [ ] **Step 1：** `README.md`：admin 工作台新增「Token 管理」页说明（跨 workspace 管控）。
- [ ] **Step 2：** `ROADMAP.md`：v0.4.4 补充 admin token 管理能力。
- [ ] **Step 3：** Commit `feat: Admin 工作台 Token 管理（跨 workspace 管控）`

---

## 验收 Checklist（对应 spec §9）

**后端：**
- [ ] `GET /api/v1/admin/tokens` 返回全部 token，含完整 user info
- [ ] `DELETE /api/v1/admin/tokens/{ref}` 可吊销任意 token，写审计
- [ ] `PATCH /api/v1/admin/tokens/{ref}` 可改 name/scope/expires，不改 workspace/project
- [ ] admin session capabilities 含 token:list/modify/revoke
- [ ] 非 admin token 访问被拒（401）
- [ ] user 解析用批量查询（ListByIDs），无 N+1

**前端：**
- [ ] `/admin/tokens` 列表含 user name+email、workspace、scopes、状态
- [ ] 可编辑 name/scope/过期，可吊销
- [ ] admin 侧边栏有「Token 管理」导航
- [ ] 已吊销行置灰、操作禁用
- [ ] 无创建按钮

**全局：**
- [ ] `go test ./...` / `CGO_ENABLED=0 go test ./...` / `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
- [ ] `cd web && pnpm test && pnpm build` 通过
- [ ] admin bootstrap 与普通 console token 管理不回归
