# Admin 工作台 Token 管理设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-06-16
**状态：** 草案
**背景需求：** Web Console 已支持普通 workspace 用户在 `/tokens` 管理 token（创建/修改/吊销），但 server admin 工作台（`/admin`）目前只有 bootstrap 引导向导，无法跨 workspace 统一管控所有 token。server admin 需要一个全局视角：查看所有 workspace 的 token、吊销失控 token、调整过期策略——这是运维管控职责，与普通用户的自服务管理是两个边界。

## 1. 背景与现状（基于代码）

### 1.1 Admin 控制面当前能力

- **路由**（`internal/httpapi/router.go:37-42`）：仅 create 操作，无任何 list/revoke/modify。
  - `GET /api/v1/admin/status`（公开探活）
  - `POST /api/v1/admin/setup`（首次 bootstrap）
  - `GET /api/v1/admin/session`（鉴权后返回 capabilities）
  - `POST /api/v1/admin/workspaces`（建 workspace）
  - `POST /api/v1/admin/workspaces/{workspace}/admins`（建 workspace admin）
  - `POST /api/v1/admin/workspaces/{workspace}/agent-tokens`（建 agent token）

- **handler 模式**（`internal/httpapi/admin.go`）：统一 `app.NewService({DisableScopeBootstrap: true, Runtime: {ActorName: "server-admin"}})`，绕过普通 workspace role 校验；鉴权由 `adminAuthMiddleware` 完成。

- **agent token 创建**（`app/admin_bootstrap.go:253` `AdminCreateWorkspaceAgentToken`）：直接调底层 `createTokenStored`，不走 `tokenManageAllowed`。**但没有对应的 list/revoke/modify 方法。**

### 1.2 Storage 层

`internal/storage/token_repo.go`：现有 `ListByUser(userID, includeRevoked)`，**无 `ListAll`**。`ApiToken.WorkspaceIDsJSON` 是 JSON 字符串列（`models.go:108`），非关联表——按 workspace 过滤需 JSON 查询函数，跨数据库（SQLite/PG）兼容性复杂。

### 1.3 Admin 前端结构

- `AdminDashboardRoute`（`web/src/routes/admin/AdminDashboardRoute.tsx`）= `BootstrapWizard` 单页。
- `AdminShell`（`web/src/features/admin/components/AdminShell.tsx`）：左侧 `w-56` 固定侧边栏（导航 + 高危徽章），右侧顶栏 + `max-w-7xl` 内容区。导航 `<nav>` 目前只有 bootstrap 一个硬编码链接——**新增页面只需在此加导航项**。
- `adminApiGet/Post`（`web/src/features/admin/session/admin-api.ts`）：**无 Patch/Delete 封装**。
- `AdminGuardRoute` 已做好 admin 会话保护 + 套 AdminShell，是父路由——新增页面加到 `addChildren` 即自动套壳。

### 1.4 普通 Console token 管理（已实现，参照对象）

`web/src/features/workspace/tokens/`：`token-api.ts` 类型、`scope-editor.tsx` 分组 Checkbox、`token-form.tsx`、Dialog、`tokens-page.tsx`。走 `workspaceApi*` 封装，后端 `/api/v1/tokens`。

## 2. 核心判断

### 2.1 Admin 用「全部 token 列表」，不做按 workspace JSON 查询

server admin 需要全局视角（谁有什么 token、哪些快过期、哪些已吊销），而非逐 workspace 翻。`ListAll` 是简单 `SELECT *`，避免 JSON 列跨库查询的复杂度。workspace 维度信息在返回行里已有（`workspace_ids`），前端可展示和过滤。

### 2.2 Admin modify 只做 name/scope/expires，不做 workspace/project

admin 跨 workspace 操作 workspace/project 绑定的语义复杂（属于哪个 workspace？token 的 owner 是否同意？），且普通 console 已支持 token owner 自服务修改。admin 工作台聚焦「运维管控」：吊销失控 token、调整 scope/过期，不做细粒度 workspace/project 重绑定。

### 2.3 复用普通 console 的 scope-editor 组件

scope 编辑器是纯展示+交互组件，与 API 层无关。admin token 页直接 import `web/src/features/workspace/tokens/scope-editor.tsx`，保持 21 个 scope 分组一致，避免重复实现。

### 2.4 两套 token API 保持隔离

admin 控制面（`/api/v1/admin/tokens`）与普通 API（`/api/v1/tokens`）是刻意隔离的边界：admin 用 server admin token + 绕过 workspace role；普通用 PAT/Agent token + workspace role。不合并，避免权限模型混淆。

## 3. 目标

1. Server admin 可在 `/admin/tokens` 查看所有 workspace 的全部 token（含已吊销）。
2. 可吊销任意 token（跨 workspace，绕过 owner 校验）。
3. 可修改 token 的 name / scope / 过期时间。
4. 列表展示 user（name+email）、workspace_ids、scopes、状态、最后使用时间。
5. Admin 侧边栏新增「Token 管理」导航入口。

## 4. 非目标

- **不做按 workspace 过滤的 JSON 查询**（全量列表 + 前端展示 workspace_ids 足够）
- **admin modify 不支持改 workspace/project**（语义复杂，普通 console 已覆盖）
- **不在 admin 工作台创建 token**（创建走 bootstrap 或普通 console；admin 工作台聚焦管控已有 token）
- **不合并两套 token API**
- **不做 token 批量操作**（逐个操作足够，批量引入确认复杂度）

## 5. 后端改动

### 5.1 Storage 层 `internal/storage/token_repo.go`

新增 `ListAll`：

```go
func (r *TokenRepository) ListAll(includeRevoked bool) ([]ApiTokenEntry, error)
```

按 `created_at DESC, id DESC` 排序，`includeRevoked=false` 时过滤 `revoked_at IS NULL`。返回所有 token（PAT + Agent）。

### 5.2 App 层（新文件 `internal/app/admin_token.go`）

参考 `admin_bootstrap.go` 的 `AdminCreateWorkspaceAgentToken` 模式。3 个方法：

- **`AdminListTokens(includeRevoked bool) ([]TokenView, error)`**
  - 调 `tokenRepo.ListAll`
  - **批量解析 user info**：现有 `tokenEntryToView` 只填了 user ID，admin 视角需要 name/email。收集所有 `UserID` 去重，调 `userRepo` 批量查，填充 `TokenView.User`。

- **`AdminRevokeToken(tokenRef string) error`**
  - `tokenRepo.GetByIDOrPrefix` → `Revoke`
  - 审计走 `appendAdminAuditInTx`，动作 `admin.token.revoke`，`ActorUserID: nil`，payload `{admin:true, admin_token_name, token_id, token_name}`

- **`AdminModifyToken(input AdminModifyTokenInput) (*TokenView, error)`**
  - 与普通 `ModifyToken` 类似（指针语义 nil/空切片），但**去掉 owner 校验**（admin 即最高权限）
  - 保留：revoked/expired 拦截、scope 校验（`ValidateTokenCreate`）、impersonate 校验（admin role 恒真，允许）、agent token workspace 约束（**但 admin modify 不接 workspace 修改**，所以校验用 existing workspace）
  - **不支持** `WorkspaceRefs` / `ProjectRefs`（非目标）
  - 审计 `admin.token.modify`

```go
type AdminModifyTokenInput struct {
    TokenID   string
    Name      *string
    Scopes    *[]string
    ExpiresIn *time.Duration
}
```

### 5.3 HTTP 层 `internal/httpapi/admin.go`

新增 3 个 handler + 路由（`router.go`）：

```
GET    /api/v1/admin/tokens             → handleAdminTokenList
PATCH  /api/v1/admin/tokens/{tokenRef}  → handleAdminTokenModify
DELETE /api/v1/admin/tokens/{tokenRef}  → handleAdminTokenRevoke
```

沿用 `app.NewService({DisableScopeBootstrap: true})` 模式。请求/响应复用普通 token 的 `tokenResponse` 结构（含 user/workspace_ids/scopes 等），但 user 字段填完整 info。

### 5.4 capabilities 同步

`handleAdminSession`（`admin.go:51-61`）的硬编码数组补 `token:list`、`token:revoke`、`token:modify`。

### 5.5 测试

- `internal/storage/token_repo_test.go`：`ListAll` 含/不含已吊销
- `internal/app/admin_token_test.go`（新）：list 返回完整 user info、revoke 写审计、modify 改 scope/expires、modify 拒绝改 workspace（不支持的路径）
- `internal/httpapi/admin_test.go`：admin token list/modify/revoke HTTP 用例，验证跨 workspace 可操作

## 6. 原型图

以下原型贴合现有 AdminShell 布局（左侧 w-56 侧边栏 + 右侧 max-w-7xl 内容区）。

### 6.1 Admin 侧边栏（新增导航项）

```
┌──────────────────────────┐
│  [Logo] 璇础             │  ← 顶部品牌区 (h-12)
├──────────────────────────┤
│                          │
│  🛡 工作区初始化          │  ← 现有: /admin (bootstrap)
│  🔑 Token 管理            │  ← 新增: /admin/tokens
│                          │
│                          │
│                          │
│                          │
│                          │
├──────────────────────────┤
│ 高危操作                 │  ← 底部固定区
│ [Risk Badge]             │
└──────────────────────────┘
       w-56 侧边栏
```

### 6.2 Token 管理列表页（`/admin/tokens`）

```
                                                              max-w-7xl 内容区
┌────────────────────────────────────────────────────────────────────────────────────┐
│  服务端超管控制面 · admin-token-name                                                 │  ← 顶栏 (sticky)
├────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                    │
│  Token 管理                                                                         │
│                                                                                    │
│  [搜索 name/prefix/user...]                                          [显示已吊销 ☐] │
│                                                                                    │
│  ┌──────────┬────────┬─────────────┬──────────────┬──────┬─────────┬─────────┬────┐ │
│  │ 名称     │ 类型   │ 用户        │ 工作空间     │Scope │ 状态    │ 最后使用│操作│ │
│  ├──────────┼────────┼─────────────┼──────────────┼──────┼─────────┼─────────┼────┤ │
│  │ ci-deploy│ agent  │ alice       │ acme         │ 3 个 │ 有效    │ 2小时前 │ ⋯  │ │
│  │          │        │ alice@x.com │ platform     │      │         │         │    │ │
│  ├──────────┼────────┼─────────────┼──────────────┼──────┼─────────┼─────────┼────┤ │
│  │ admin-pat│ pat    │ bob         │ (全局)       │ 5 个 │30天后过期│ 昨天   │ ⋯  │ │
│  │          │        │ bob@x.com   │              │      │         │         │    │ │
│  ├──────────┼────────┼─────────────┼──────────────┼──────┼─────────┼─────────┼────┤ │
│  │ legacy   │ agent  │ carol       │ acme         │ 2 个 │ 已吊销  │ —       │ —  │ │
│  │ (置灰)   │        │ carol@x.com │              │      │ (置灰)  │         │    │ │
│  └──────────┴────────┴─────────────┴──────────────┴──────┴─────────┴─────────┴────┘ │
│                                                                                    │
│  操作菜单 (⋯ 展开，已吊销行禁用):                                                    │
│  ┌──────────────┐                                                                  │
│  │ ✏️  编辑      │  → name / scope / 过期（不含 workspace/project）                   │
│  │ 🗑️  吊销      │                                                                  │
│  └──────────────┘                                                                  │
└────────────────────────────────────────────────────────────────────────────────────┘
```

**说明：**
- 「用户」列两行：name + email（admin 视角需要知道 token 属于谁）。
- 「工作空间」列：显示 workspace slug（PAT 显示「全局」），多 workspace 换行或逗号分隔。
- 已吊销行整行置灰，操作菜单禁用。
- 「显示已吊销」复选框控制是否包含已吊销 token（默认包含，admin 需要全貌）。
- 无「创建」按钮（非目标，admin 工作台不创建 token）。

### 6.3 编辑 Dialog（admin 修改 token）

```
┌────────────────────────────────────────────────────────────┐
│  编辑 Token                                           [×]  │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  名称                                                      │
│  ┌──────────────────────────────────────────────────┐     │
│  │ ci-deploy                                         │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  类型 (只读)                  Agent                        │
│                                                            │
│  用户 (只读)                  alice (alice@x.com)          │
│                                                            │
│  工作空间 (只读)              acme, platform               │
│                                                            │
│  Scope (按资源勾选)                                        │
│  ┌────────────────────────────────────────────────────┐   │
│  │ 任务 (task)        [全选] [清空]                    │   │
│  │   [✓] read          [✓] write                      │   │
│  │                                                    │   │
│  │ 项目 (project)     [全选] [清空]                    │   │
│  │   [✓] read          [ ] write                      │   │
│  │                                                    │   │
│  │ 工作空间/Token/成员/Hook/通知/提醒/审计/配置/上下文 │   │
│  │   (同样分组)                                       │   │
│  │                                                    │   │
│  │ 特殊                                               │   │
│  │   [ ] impersonate                                  │   │
│  └────────────────────────────────────────────────────┘   │
│                                                            │
│  过期时间                                                  │
│  ┌──────────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌────────────┐  │
│  │● 永不过期│ │○ 7天 │ │○ 30天│ │○ 90天│ │○ 自定义... │  │
│  └──────────┘ └──────┘ └──────┘ └──────┘ └────────────┘  │
│                                                            │
│  ⚠️ admin 修改将记录到审计日志（admin.token.modify）        │
│                                                            │
├────────────────────────────────────────────────────────────┤
│                              [取消]            [保存]      │
└────────────────────────────────────────────────────────────┘
```

**与普通 console 编辑 Dialog 的差异：**
- 多「用户（只读）」「工作空间（只读）」两行（admin 需确认操作对象）。
- 无 workspace/project 编辑区（admin 不改绑定）。
- 底部提示 admin 审计记录。

### 6.4 吊销确认（AlertDialog）

```
┌────────────────────────────────────────────┐
│  确认吊销 Token                        [×] │
├────────────────────────────────────────────┤
│                                            │
│  ⚠️ 吊销后此 Token 立即失效且不可恢复。     │
│                                            │
│  名称:   ci-deploy                         │
│  Prefix: xuanchu_age                       │
│  类型:   agent                             │
│  用户:   alice (alice@x.com)               │
│                                            │
│  使用此 Token 的客户端将无法再访问 API。    │
│  此操作以 admin 身份执行，将记录审计日志。  │
│                                            │
├────────────────────────────────────────────┤
│              [取消]        [吊销]          │
└────────────────────────────────────────────┘
```

## 7. 前端改动

### 7.1 新增模块 `web/src/features/admin/tokens/`

```
tokens/
  admin-token-api.ts        # 类型 + admin token 状态派生（复用 workspace/tokens/token-api 的 helper）
  admin-tokens-page.tsx     # 列表页（复用 scope-editor）
  admin-token-edit-dialog.tsx
  admin-token-revoke-dialog.tsx
  use-admin-token-mutations.ts
```

### 7.2 admin-api 扩展

`admin-api.ts` 加 `adminApiPatch` / `adminApiDelete`（沿用 `assertAdminPath`）。

### 7.3 路由 + 导航

- `AdminTokensRoute.tsx` 加到 `adminGuardRoute.addChildren`
- `AdminShell.tsx` 侧边栏 `<nav>` 加「Token 管理」链接（`/admin/tokens`）
- 复用 `scope-editor`（从 `@/features/workspace/tokens/scope-editor` import）

### 7.4 i18n

`admin` 命名空间加 `nav.tokens`、token 管理相关文案（标签/状态/错误码）。scope 分组名复用普通 console 的 `token.scopeGroup.*`。

## 8. 安全考量

- **admin 鉴权兜底**：所有 admin token API 经 `adminAuthMiddleware`，仅 server admin token 可访问。
- **绕过 workspace role 是预期行为**：admin 跨 workspace 管控是其职责，与 bootstrap 创建 agent token 同一权限模型。
- **审计完整**：revoke/modify 走 `appendAdminAuditInTx`，payload 标记 `admin:true` + `admin_token_name`，可追溯是哪个 admin 操作。
- **impersonate scope**：admin role 恒允许（`tokenManageAllowed` 对 admin 返回 true），admin 可给任意 token 配/撤 impersonate。
- **不暴露明文 token**：admin list/modify 响应不含 raw token（创建才返回，admin 不创建）。

## 9. 验收标准

**后端：**
- `GET /api/v1/admin/tokens` 返回所有 workspace 的全部 token，含完整 user info。
- `DELETE /api/v1/admin/tokens/{ref}` 可吊销任意 workspace 的 token，写审计。
- `PATCH /api/v1/admin/tokens/{ref}` 可改 name/scope/expires，不改 workspace/project。
- admin session capabilities 含 `token:list` / `token:revoke` / `token:modify`。
- 非 admin token 访问 admin token API 被拒（401）。

**前端：**
- `/admin/tokens` 列表显示所有 token（含 user name+email、workspace_ids、scopes、状态）。
- 可编辑 name/scope/过期，可吊销。
- admin 侧边栏有「Token 管理」导航。
- 已吊销行置灰、操作禁用。

**全局：**
- `go test ./...` / `CGO_ENABLED=0 go test ./...` / `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。
- `cd web && pnpm test && pnpm build` 通过。
- 现有 admin bootstrap 与普通 console token 管理不回归。

## 10. 实施顺序

1. 后端：Storage `ListAll` → App `admin_token.go`（list/revoke/modify）→ HTTP handler + 路由 → capabilities → 测试。
2. 前端：admin-api Patch/Delete → admin-token-api → 页面/Dialog → 路由+导航 → i18n → 测试。
3. 全量验证。
