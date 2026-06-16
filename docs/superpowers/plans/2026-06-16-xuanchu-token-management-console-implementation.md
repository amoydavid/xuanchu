# Xuanchu Token 管理 Web Console Implementation Plan

> **For agentic workers:** REQUIRED: 使用 superpowers:subagent-driven-development（若可用）或 superpowers:executing-plans 执行本计划。步骤用 checkbox（`- [ ]`）跟踪。

**Goal:** 为 Web Console `/tokens` 页面补齐完整 CRUD（创建、修改、吊销），并扩展后端 `PATCH /api/v1/tokens/{ref}` 支持 workspace_ids / project_ids 修改，统一 nil/空切片语义，补齐审计。

**Architecture:** Storage 层已就绪（`TokenUpdates` 已支持 workspace/project 字段），本次只接通 App 层 `ModifyToken` 和 HTTP 层 `modifyTokenRequest`。前端新增 `web/src/features/workspace/tokens/` 模块，引入项目首个 `useMutation` 写操作范式，用专用 `TokensPage` 替换 `/tokens` 的通用 ResourcePage。

**Tech Stack:** Go 1.25、GORM、github.com/glebarez/sqlite、chi；React 19、TanStack Query/Router、shadcn/ui、Tailwind v4、Vitest、i18next

**Spec:** `docs/superpowers/specs/2026-06-16-xuanchu-token-management-console-design.md`

---

## Chunk 1：后端 — Storage 审计补齐

### Task 1：ChangedFields 补齐 workspace/project 输出

**Files:**
- Modify: `internal/storage/token_repo.go`

- [ ] **Step 1：** 在 `ChangedFields()`（`token_repo.go:29-43`）补上：
  ```go
  if u.WorkspaceIDsJSON != nil {
      attrs["workspace_ids"] = *u.WorkspaceIDsJSON
  }
  if u.ProjectIDsJSON != nil {
      attrs["project_ids"] = *u.ProjectIDsJSON
  }
  ```
- [ ] **Step 2：** 验证 `CGO_ENABLED=0 go build ./internal/storage/...`

### Task 2：storage 层测试

**Files:**
- Modify: `internal/storage/token_repo_test.go`

- [ ] **Step 1：** 覆盖 `ChangedFields` 含 workspace_ids / project_ids 时的输出
- [ ] **Step 2：** 验证 `CGO_ENABLED=0 go test ./internal/storage/... -run TestToken`

---

## Chunk 2：后端 — App 层 ModifyToken 扩展

### Task 3：ModifyTokenInput 改指针语义 + 接通 workspace/project

**Files:**
- Modify: `internal/app/token.go`

- [ ] **Step 1：** `ModifyTokenInput`（line 534）改为指针语义以区分「不改」与「清空」：
  ```go
  type ModifyTokenInput struct {
      TokenID       string
      Name          *string
      Scopes        *[]string   // 改为指针：nil=不改，非nil=替换（含空切片=清空）
      WorkspaceRefs *[]string   // 新增
      ProjectRefs   *[]string   // 新增
      ExpiresIn     *time.Duration
  }
  ```
- [ ] **Step 2：** `ModifyToken`（line 541）逻辑扩展：
  - 解析 workspace refs：若 `input.WorkspaceRefs != nil`，调用 `s.resolveTokenWorkspaces(*input.WorkspaceRefs)` 得到 `workspaceIDs`；为空切片且 token 是 agent 类型 → 返回 `token_agent_requires_workspace`
  - 解析 project refs：若 `input.ProjectRefs != nil`，调用 `s.resolveTokenProjects(workspaces, *input.ProjectRefs)`，project 必须属于解析出的 workspace 集合（沿用 create 约束）；project 解析所用的 workspaces 是**修改后的最终集合**（若 WorkspaceRefs 为 nil 则用 existing）
  - 写 `TokenUpdates.WorkspaceIDsJSON` / `ProjectIDsJSON`（nil 时不写）
- [ ] **Step 3：** 修改 scopes 校验逻辑（line 562-574）：当 `input.Scopes != nil` 时，`ValidateTokenCreate` 传入的 `WorkspaceIDs` 必须用**最终 workspace 集合**（修改后或 existing），保证 scope 与 workspace 一致性。`*input.Scopes` 为空切片时走 `ValidateTokenCreate` 校验（PAT 空切片会被拒，agent 空切片也会被拒）
- [ ] **Step 4：** 复用 `enforceTokenCreateLimit` 防止越权放大 scopes/workspace/project 范围（传入最终 scopes、workspaceIDs、projectIDs）
- [ ] **Step 5：** 审计 payload 通过 `updates.ChangedFields()` 自动包含 workspace_ids/project_ids（Task 1 已补齐）
- [ ] **Step 6：** 验证 `CGO_ENABLED=0 go build ./internal/app/...`

### Task 4：App 层测试

**Files:**
- Modify: `internal/app/token_test.go`

- [ ] **Step 1：** `TestModifyTokenWorkspaces`：修改 agent token 的 workspace 集合，验证成功 + 返回 view 含新 workspace_ids + 审计记录
- [ ] **Step 2：** `TestModifyTokenProjects`：修改 project 范围，验证成功
- [ ] **Step 3：** `TestModifyTokenAgentRejectsEmptyWorkspace`：agent token 把 workspace 改为空切片被拒（`token_agent_requires_workspace`）
- [ ] **Step 4：** `TestModifyTokenProjectNotInWorkspace`：project 不属于任何已选 workspace 被拒（`token_project_scope_invalid`）
- [ ] **Step 5：** `TestModifyTokenClearScopes`：显式清空 scope（`Scopes: &[]string{}`）被拒（scope 不可为空）
- [ ] **Step 6：** `TestModifyTokenNoChangeByNil`：所有指针字段为 nil（不改）时返回当前 view，不写库
- [ ] **Step 7：** 修复现有 `TestModifyTokenScopes` / `TestModifyTokenScopesWildcard` 等用例，使其传入 `*[]string` 而非 `[]string`（签名变更影响）
- [ ] **Step 8：** 验证 `CGO_ENABLED=0 go test ./internal/app/... -run TestModifyToken`

---

## Chunk 3：后端 — HTTP 层

### Task 5：modifyTokenRequest 补字段 + 透传

**Files:**
- Modify: `internal/httpapi/tokens.go`

- [ ] **Step 1：** `modifyTokenRequest`（line 29）新增字段，字段名与 create 对齐：
  ```go
  type modifyTokenRequest struct {
      Name             *string  `json:"name,omitempty"`
      Scopes           *[]string `json:"scopes,omitempty"`
      Workspaces       *[]string `json:"workspaces,omitempty"`
      WorkspaceIDs     *[]string `json:"workspace_ids,omitempty"`
      Projects         *[]string `json:"projects,omitempty"`
      ProjectIDs       *[]string `json:"project_ids,omitempty"`
      ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
  }
  ```
  > 注：用 `*[]string` 让 JSON 的 `null`/缺省 = nil（不改），`[]` = 非 nil 空切片（清空）。Go encoding/json 对 `*[]string` 的 omitempty 在 nil 时不输出、非 nil 时输出（含空数组）。
- [ ] **Step 2：** `handleTokenModify`（line 125）合并 `Workspaces + WorkspaceIDs` → 单个 `*[]string`（两者都 nil 则传 nil；任一非 nil 则合并去重传非 nil），`Projects + ProjectIDs` 同理。透传给 `app.ModifyTokenInput`
- [ ] **Step 3：** 验证 `CGO_ENABLED=0 go build ./internal/httpapi/...`

### Task 6：HTTP 层测试

**Files:**
- Create: `internal/httpapi/tokens_test.go`

- [ ] **Step 1：** 参考现有 `newHTTPServerWithTokenFixture(t, "token:write")` + `requestHTTPBody` + `assertHTTPErrorCode` 模式（见 `admin_test.go:14`）
- [ ] **Step 2：** `TestModifyTokenWorkspacesHTTP`：PATCH 带 `workspaces` 字段，验证 200 + 返回 workspace_ids 更新
- [ ] **Step 3：** `TestModifyTokenClearWorkspacesHTTP`：PATCH 带 `workspaces: []`，agent token 被拒（`token_agent_requires_workspace`）
- [ ] **Step 4：** `TestModifyTokenProjectsHTTP`：PATCH 带 `projects` 字段透传成功
- [ ] **Step 5：** `TestModifyTokenOmitFieldHTTP`：请求体只带 `name`（其它字段缺省），验证 workspace/project 不被清空
- [ ] **Step 6：** 验证 `CGO_ENABLED=0 go test ./internal/httpapi/... -run TestModifyToken`

---

## Chunk 4：前端 — 数据层与基础组件

### Task 7：token-api.ts 类型与封装

**Files:**
- Create: `web/src/features/workspace/tokens/token-api.ts`

- [ ] **Step 1：** 定义 `TokenRow` 类型（对齐 `tokenResponse`：id/prefix/name/type/user/workspace_ids/project_ids/scopes/created_at/expires_at?/revoked_at?/last_used_at?）
- [ ] **Step 2：** 定义 `TokenFormValues`（name/type/workspaces/scopes/projects/expiresPreset/expiresAt?）
- [ ] **Step 3：** 定义 `TokenCreateInput` / `TokenModifyInput`（modify 字段全可选，`*[]string` 语义在前端用 `undefined` = 不改、`[]` = 清空）
- [ ] **Step 4：** 定义 token 状态派生 helper：`deriveTokenStatus(row)` → `'active' | 'expired' | 'revoked'`
- [ ] **Step 5：** 验证 `cd web && pnpm typecheck`

### Task 8：scope-editor.tsx

**Files:**
- Create: `web/src/features/workspace/tokens/scope-editor.tsx`

- [ ] **Step 1：** 定义 scope 分组常量（从后端 `scopeRegistry` 派生：task/project/context/config/workspace/audit/token/hook/notification/reminder + 独立的 impersonate），分组中文名走 i18n key
- [ ] **Step 2：** 组件 props：`value: string[]` / `onChange: (scopes: string[]) => void` / `canImpersonate: boolean`
- [ ] **Step 3：** 按 resource 分组渲染 Checkbox（每组 read/write 两个 + 「全选/清空」快捷），`impersonate` 单独一组，仅 `canImpersonate` 时渲染
- [ ] **Step 4：** 创建 `scope-editor.test.tsx`：覆盖勾选/取消、全选/清空、impersonate 隐藏、onChange 输出具体 scope 数组
- [ ] **Step 5：** 验证 `cd web && pnpm test -- scope-editor`

---

## Chunk 5：前端 — 表单与 Dialog

### Task 9：token-form.tsx（创建/编辑共用）

**Files:**
- Create: `web/src/features/workspace/tokens/token-form.tsx`

- [ ] **Step 1：** 受控组件（参考 `bootstrap-wizard.tsx` 模式），props：`mode: 'create' | 'edit'` / `initial?: TokenRow` / `onSubmit` / `submitting` / `error`
- [ ] **Step 2：** 字段：name（Input）、type（Select：PAT/Agent，edit 模式 disabled）、workspaces（multi-select，拉 `GET /api/v1/workspaces`，agent 必填）、scopes（`ScopeEditor`）、projects（multi-select，依赖已选 workspaces）、expires（单选：永不过期/7天/30天/90天/自定义 datetime）
- [ ] **Step 3：** edit 模式用 `initial` 预填，type 字段灰显不可改
- [ ] **Step 4：** 表单校验：name 非空、agent 必填 workspace、scope 非空；错误内联提示
- [ ] **Step 5：** 验证 `cd web && pnpm typecheck`

### Task 10：use-token-mutations.ts（首个 useMutation 范式）

**Files:**
- Create: `web/src/features/workspace/tokens/use-token-mutations.ts`

- [ ] **Step 1：** `useCreateTokenMutation`：`mutationFn: workspaceApiPost("/api/v1/tokens", input)`，`onSuccess` invalidate `["resource", "/api/v1/tokens"]`
- [ ] **Step 2：** `useModifyTokenMutation`：`workspaceApiPatch("/api/v1/tokens/" + ref, input)`，同样 invalidate
- [ ] **Step 3：** `useRevokeTokenMutation`：`workspaceApiDelete("/api/v1/tokens/" + ref)`，同样 invalidate
- [ ] **Step 4：** 验证 `cd web && pnpm typecheck`

### Task 11：token-create-dialog.tsx + token-created-result.tsx

**Files:**
- Create: `web/src/features/workspace/tokens/token-create-dialog.tsx`
- Create: `web/src/features/workspace/tokens/token-created-result.tsx`

- [ ] **Step 1：** `TokenCreateDialog`：Dialog 包裹 `TokenForm`（mode=create），提交调 `useCreateTokenMutation`，成功后切换到 `TokenCreatedResult` 视图
- [ ] **Step 2：** `TokenCreatedResult`：`<code>` 显示明文 token（来自响应 `token` 字段）+ 复制按钮（复用 `navigator.clipboard.writeText`，参考 bootstrap-wizard）+ 警示文案 + 「完成」按钮关闭并刷新
- [ ] **Step 3：** 错误处理：捕获 `ApiError`，`t("token.errors." + code)` i18n，未知 code 回退通用文案
- [ ] **Step 4：** 验证 `cd web && pnpm typecheck`

### Task 12：token-edit-dialog.tsx + token-revoke-dialog.tsx

**Files:**
- Create: `web/src/features/workspace/tokens/token-edit-dialog.tsx`
- Create: `web/src/features/workspace/tokens/token-revoke-dialog.tsx`

- [ ] **Step 1：** `TokenEditDialog`：Dialog 包裹 `TokenForm`（mode=edit，传入 row 作为 initial），提交调 `useModifyTokenMutation`，成功关闭刷新
- [ ] **Step 2：** `TokenRevokeDialog`：用 `alert-dialog`，显示 name/prefix/type，确认调 `useRevokeTokenMutation`
- [ ] **Step 3：** 验证 `cd web && pnpm typecheck`

---

## Chunk 6：前端 — 列表页接入

### Task 13：TokensPage 专用页面

**Files:**
- Create: `web/src/features/workspace/tokens/tokens-page.tsx`

- [ ] **Step 1：** 复用 `useQuery({ queryKey: ["resource", "/api/v1/tokens"], queryFn: workspaceApiGet<TokenRow[]> })`（保持 query key 一致复用缓存）
- [ ] **Step 2：** 顶部「+ 创建 Token」按钮 → 打开 `TokenCreateDialog`
- [ ] **Step 3：** `DataTable` 列：name / type（Badge）/ prefix / scopes（「N 个」+ tooltip）/ 状态（`deriveTokenStatus`，Badge 颜色区分）/ last_used_at / 操作（dropdown-menu：编辑/吊销）
- [ ] **Step 4：** 已吊销行整行置灰（row className）
- [ ] **Step 5：** loading/error/empty 状态沿用现有 ResourcePage 模式（Skeleton + 错误提示）
- [ ] **Step 6：** 验证 `cd web && pnpm typecheck`

### Task 14：路由接入 TokensPage

**Files:**
- Modify: `web/src/routes/router.tsx`
- Create: `web/src/routes/workspace/TokensRoute.tsx`

- [ ] **Step 1：** 新建 `TokensRoute.tsx`（导出 `TokensRoute` 组件，渲染 `<TokensPage />`）
- [ ] **Step 2：** `router.tsx` 把 `createResourceRoute("tokens", "/tokens")`（line 116）替换为 lazy 加载的 `TokensRoute`（用 `createRoute` + `lazyRoute` 模式）
- [ ] **Step 3：** `resourceConfig` 的 `tokens` case 可保留（不再被路由引用，但避免破坏其它调用）或删除——确认无其它引用后删除
- [ ] **Step 4：** 验证 `cd web && pnpm typecheck`

---

## Chunk 7：前端 — i18n 与测试

### Task 15：i18n 文案

**Files:**
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/locales/zh-CN.ts`

- [ ] **Step 1：** 新增 `token` 命名空间，en/zh 双语：
  - 字段标签：name / type / scopes / workspaces / projects / expires / prefix / lastUsed
  - scope 分组名：task / project / workspace / token / member / hook / notification / reminder / audit / config / context / impersonate
  - 操作：create / edit / revoke / copy / copied / save / cancel / done
  - 状态：active / expired / revoked / neverExpires / days（「N 天后过期」）
  - 错误码映射：token_scope_invalid / token_agent_requires_workspace / token_not_found / token_revoked / token_expired / token_update_failed / token_project_scope_invalid
  - 警示：createdWarning（「仅显示一次…」）/ revokeWarning（「吊销不可恢复…」）
- [ ] **Step 2：** 验证 `cd web && pnpm typecheck`（i18n key 引用不报错）

### Task 16：前端集成测试

**Files:**
- Create: `web/src/features/workspace/tokens/tokens-page.test.tsx`

- [ ] **Step 1：** 参考 `bootstrap-wizard.test.tsx` 的 Testing Library 模式
- [ ] **Step 2：** mock `workspaceApiGet` 返回 token 列表，验证列渲染（name/type/scopes 数量/状态）
- [ ] **Step 3：** 验证「创建 Token」按钮打开 Dialog
- [ ] **Step 4：** 验证操作菜单（编辑/吊销）渲染
- [ ] **Step 5：** 验证已吊销行置灰
- [ ] **Step 6：** 验证 `cd web && pnpm test -- tokens`

---

## Chunk 8：全量验证 + 文档同步

### Task 17：全量验证

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
  - `internal/app/token_test.go` 所有 ModifyToken 用例
  - `internal/httpapi/tokens_test.go`（新增）
  - `tests/integration/cli_test.go` token 相关不回归
  - `internal/storage/token_repo_test.go`

### Task 18：文档同步

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`

- [ ] **Step 1：** `README.md`：Web Console `/tokens` 页面说明更新（支持完整 CRUD）
- [ ] **Step 2：** `ROADMAP.md`：记录 token 管理 console 完成
- [ ] **Step 3：** Commit `feat: Web Console 支持 Token 完整管理（创建/修改/吊销）`

---

## 验收 Checklist（对应 spec §9）

**后端：**
- [ ] `PATCH /api/v1/tokens/{ref}` 接受 workspaces / projects 字段
- [ ] agent token 清空 workspace 被拒（`token_agent_requires_workspace`）
- [ ] project 不属于 workspace 被拒（`token_project_scope_invalid`）
- [ ] 修改 scopes 时校验基于最终 workspace 集合
- [ ] 审计日志记录 workspace/project 变更
- [ ] nil（不改）与空数组（清空）语义正确区分

**前端：**
- [ ] `/tokens` 列表显示 scopes、状态、最后使用时间
- [ ] 可创建 PAT / Agent token，创建后明文 token 一次性展示并可复制
- [ ] 可编辑 name / scopes / 过期 / workspaces / projects
- [ ] 可吊销 token，二次确认
- [ ] scope 编辑器按资源分组，提交具体 scope 数组
- [ ] 编辑时 type 只读
- [ ] 错误有 i18n 文案

**全局：**
- [ ] `go test ./...` / `CGO_ENABLED=0 go test ./...` / `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
- [ ] `cd web && pnpm test && pnpm build` 通过
- [ ] 现有 token 相关测试不回归
