# Token 管理 Web Console 设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-06-16
**状态：** 草案
**背景需求：** Web Console 的 `/tokens` 页面当前是只读列表，只能看 name/type/prefix 三列。用户创建、修改、吊销 token 都得退回命令行，体验割裂。需要把 token 的完整生命周期管理（创建、修改 scope / 过期 / 工作空间范围、吊销）搬到 Web Console。

## 1. 背景与现状（基于代码）

### 1.1 后端能力已基本就绪

- **Storage 层** `internal/storage/token_repo.go`
  - `ApiToken` 表（`models.go:100-114`）：user token 与 agent token 共表，靠 `Type` 区分 `pat`/`agent`。字段含 `scopes_json` / `workspace_ids_json` / `project_ids_json` / `expires_at` / `revoked_at`，均无 schema 变更需求。
  - `TokenUpdates`（`token_repo.go:142-149`）已包含 `WorkspaceIDsJSON` / `ProjectIDsJSON` 字段，`Update()`（`token_repo.go:151-181`）已支持写入这两个字段。**底层完全就绪，只是上层没接通。**
  - 缺口：`ChangedFields()`（`token_repo.go:29-43`）未输出 workspace/project 变化，审计日志记录不全。

- **App 层** `internal/app/token.go`
  - `CreateToken`（line 68）：已支持 name / type / scopes / workspace refs / project refs / expires。
  - `ModifyToken`（line 541）+ `ModifyTokenInput`（line 534）：**只支持 name / scopes / expires_in**，不支持改 workspace_ids / project_ids。
  - 已有可复用的解析方法 `resolveTokenWorkspaces`（line 401）/ `resolveTokenProjects`（line 422），带角色权限校验。
  - `RevokeToken`（line 240）：软删除，设 `revoked_at`，已完成。

- **HTTP 层** `internal/httpapi/tokens.go`
  - `GET /api/v1/tokens`、`POST /api/v1/tokens`、`PATCH /api/v1/tokens/{tokenRef}`、`DELETE /api/v1/tokens/{tokenRef}` 已注册（`router.go:105-108`）。
  - `modifyTokenRequest`（line 29）：**只接受 name / scopes / expires_in_seconds**，无 workspace/project 字段。
  - `createTokenRequest`（line 35）：已有 `workspaces` / `projects` 字段约定（同时接受 `workspace_ids` / `project_ids`）。

- **Scope 体系** `internal/auth/scope.go`
  - `scopeRegistry`（line 9-21）：21 个 scope，格式 `resource:action`，外加独立的 `impersonate`。
  - `ParseScopes`（line 40）支持通配符展开（`*` / `resource:*` / `*:action`），存储的是展开后的具体 scope。
  - `ValidateTokenCreate`（`auth/token.go:48`）：agent token 强制至少 1 个 workspace + 显式 scopes；PAT 自动剥离 `impersonate`。

- **Token 明文**仅在创建时返回一次（`createdTokenResponse.Token`，`tokens.go:47-50`），之后仅靠 `prefix` 识别。UI 必须当场展示并提示。

### 1.2 前端仅有只读列表

- `/tokens` 由通用 `ResourcePage` 渲染（`web/src/pages/ResourcePage.tsx`），列配置在 `resource-config.tsx:83-100`：只有 name / type / prefix 三列。
- 无任何创建 / 编辑 / 吊销 UI。唯一接近的 token 创建 UI 是 admin 引导向导（`web/src/features/admin/bootstrap/`），但它走 admin 路径、只建 agent token，不复用普通 `/api/v1/tokens`。
- 技术栈：React 19 + TanStack Router/Query + shadcn/ui + Tailwind v4 + i18next（en/zh）。
- API 封装 `workspaceApiGet/Post/Patch/Delete` 已齐备（`workspace-api.ts`），PATCH/DELETE 封装存在但**项目内从未被调用**（目前无任何写操作走 useMutation）。
- 表单范式可参考 `bootstrap-wizard.tsx` 的受控组件 + submit 状态机模式。

## 2. 核心判断

### 2.1 后端只做最小扩展：接通 workspace/project 的 modify

Storage 层已支持，只需在 App 层 `ModifyToken` 接通 `resolveTokenWorkspaces`/`resolveTokenProjects`，并在 HTTP 层 `modifyTokenRequest` 补字段。这是「填坑」而非「新设计」，改动集中在 3 个文件，风险低。

**不扩后端、不加 schema、不加 migration 的字段**：本次不做 `description`（`ApiToken` 表无此列，加它要 migration，超出范围）。本次聚焦用户明确要的 scope / 过期 / 工作空间范围。

### 2.2 修改 token 类型（PAT↔Agent）禁止

PAT 与 Agent 的 token 前缀（`xuanchu_pat_` / `xuanchu_agent_`）、scope 约束、workspace 要求都不同，互转会破坏既有 hash/prefix 语义且无业务必要。创建后类型锁定，编辑表单中类型字段只读。

### 2.3 Scope 编辑用「资源分组 Checkbox」，不用通配符输入

终端用户（运维 / 管理员）在浏览器里操作，逐个勾选 `task:read`、`task:write` 比手敲 `task:*` 直观且不易出错。前端把 21 个 scope 按 resource 分组渲染 Checkbox 组，提交时传展开后的具体 scope 数组。通配符逻辑仍由后端 `ParseScopes` 兜底（直接调 API 时仍可用）。

### 2.4 创建后明文 token 仅展示一次

复用 bootstrap wizard 的模式：创建成功 Dialog 显示完整 token 字符串 + 复制按钮 + 警示「关闭后无法再查看」。列表里只能看到 prefix。

## 3. 目标

1. Web Console `/tokens` 支持完整 CRUD：创建、修改、吊销 token。
2. 修改支持：name、scopes、过期时间、workspace_ids（agent）、project_ids。
3. 创建支持：name、type（PAT/Agent）、scopes、workspaces、projects、过期时间。
4. 后端 `PATCH /api/v1/tokens/{ref}` 补齐 workspace/project 修改能力。
5. 列表页信息增强：显示 scopes、过期/状态、最后使用时间、操作菜单。
6. 审计日志完整记录 workspace/project 变更。

## 4. 非目标

- **不加 `description` 字段**（需 schema migration，超出范围）
- **不动 server-admin token**（独立表、独立语义，bootstrap 专用）
- **不支持 token 类型互转**（PAT↔Agent）
- **不实现 token 轮换 / 自动续期**（后续需求）
- **不改变 scope 通配符后端语义**（前端用 Checkbox，但 API 仍接受通配符字符串）
- **不引入 cookie web session**（仍用 sessionStorage 里的 token，沿用现有鉴权）

## 5. 后端改动

### 5.1 App 层 `internal/app/token.go`

`ModifyTokenInput` 新增字段：

```go
type ModifyTokenInput struct {
    TokenID       string
    Name          *string
    Scopes        []string
    WorkspaceRefs []string   // 新增
    ProjectRefs   []string   // 新增
    ExpiresIn     *time.Duration
}
```

`ModifyToken` 逻辑扩展：

1. 解析 workspace refs → IDs：复用 `s.resolveTokenWorkspaces(input.WorkspaceRefs)`，得到 `workspaceIDs`。
2. 解析 project refs → IDs：复用 `s.resolveTokenProjects(workspaces, input.ProjectRefs)`，project 必须属于解析出的 workspace 集合（沿用 create 的约束）。
3. 写 `TokenUpdates.WorkspaceIDsJSON` / `ProjectIDsJSON`。
4. **安全校验**（关键）：
   - 若最终 workspace 集合为空且 token 是 agent 类型 → 拒绝（agent token 必须至少 1 个 workspace，沿用 `ValidateTokenCreate` 约束）。
   - 修改 scopes 时，`ValidateTokenCreate` 传入的 `WorkspaceIDs` 必须用**修改后的最终 workspace 集合**，而非 existing，保证 scope 与 workspace 一致性校验。
   - PAT 不允许持有 `impersonate`（`ValidateTokenCreate` 已处理，modify 时同样走该校验）。
   - 复用 `enforceTokenCreateLimit` 防止越权放大 scopes/workspace/project 范围。

### 5.2 HTTP 层 `internal/httpapi/tokens.go`

`modifyTokenRequest` 新增字段，复用 create 的字段名约定：

```go
type modifyTokenRequest struct {
    Name             *string  `json:"name,omitempty"`
    Scopes           []string `json:"scopes,omitempty"`
    Workspaces       []string `json:"workspaces,omitempty"`       // 新增
    WorkspaceIDs     []string `json:"workspace_ids,omitempty"`    // 新增
    Projects         []string `json:"projects,omitempty"`         // 新增
    ProjectIDs       []string `json:"project_ids,omitempty"`      // 新增
    ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}
```

`handleTokenModify` 把 workspace/project refs（`Workspaces + WorkspaceIDs`、`Projects + ProjectIDs`）合并后透传给 `ModifyTokenInput`，与 `handleTokenCreate` 的合并逻辑一致。

**字段语义约定（与 create 对齐，避免歧义）**：
- `nil`（字段缺省）= 不修改该属性。
- `[]`（显式空数组）= 清空该属性。
- 由于 JSON 里 `nil` 和 `[]` 都解码成 Go 的 `nil`/空切片，**App 层需用指针或显式标记区分「不改」与「清空」**。采用方案：HTTP 层把 `Workspaces`/`WorkspaceIDs` 合并成一个 `*[]string`（nil = 不改，非 nil = 替换为该集合，空切片 = 清空），`ModifyTokenInput.WorkspaceRefs` 改为 `*[]string`。`ProjectRefs` 同理。`Scopes` 已有同样的歧义问题，本次一并修正为 `*[]string`。

### 5.3 Storage 层 `internal/storage/token_repo.go`

`ChangedFields()` 补齐 workspace/project 输出，保证审计完整：

```go
if u.WorkspaceIDsJSON != nil {
    attrs["workspace_ids"] = *u.WorkspaceIDsJSON
}
if u.ProjectIDsJSON != nil {
    attrs["project_ids"] = *u.ProjectIDsJSON
}
```

### 5.4 错误码

沿用现有错误码，新增仅当必要：

- `token_not_found` / `token_revoked` / `token_expired`（已有）
- `token_scope_invalid`（已有，scope 校验失败）
- `token_workspace_scope_invalid`（已有，workspace 校验失败）
- `token_agent_requires_workspace`（新增，agent token 清空 workspace 时）
- `token_update_failed`（已有，写入失败）

### 5.5 测试

`internal/app/token_test.go` 仿照 `TestModifyTokenScopes` 新增：

- `TestModifyTokenWorkspaces`：修改 agent token 的 workspace 集合，验证成功 + 审计记录。
- `TestModifyTokenProjects`：修改 project 范围，验证归属 workspace 校验。
- `TestModifyTokenAgentRejectsEmptyWorkspace`：agent token 清空 workspace 被拒。
- `TestModifyTokenProjectNotInWorkspace`：project 不属于任何 workspace 被拒。
- `TestModifyTokenClearScope`：显式清空 scope（验证 nil vs 空切片语义）。

`internal/httpapi` 补 modify 的 HTTP 集成用例（workspace/project 字段透传）。

## 6. 原型图

以下 ASCII 原型展示关键页面的布局与交互，实际实现使用 shadcn/ui + Tailwind v4，视觉风格遵循现有 Web Console。

### 6.1 Token 列表页（`/tokens`）

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Tokens                                                          [+ 创建 Token] │
├──────────────────────────────────────────────────────────────────────────────┤
│  [搜索...]                                          [筛选]                      │
├──────────────┬────────┬─────────────┬──────────┬───────────┬──────────┬──────┤
│ 名称         │ 类型   │ Prefix      │ Scope    │ 状态      │ 最后使用 │ 操作 │
├──────────────┼────────┼─────────────┼──────────┼───────────┼──────────┼──────┤
│ ci-deploy    │ agent  │ xuanchu_age │ 3 个     │ 有效      │ 2 小时前 │ ⋯    │
│ admin-pat    │ pat    │ xuanchu_pat │ 5 个     │ 30天后过期│ 昨天     │ ⋯    │
│ legacy-hook  │ agent  │ xuanchu_age │ 2 个     │ 已吊销    │ —        │ ⋯    │
│ ci-read-only │ agent  │ xuanchu_age │ 1 个     │ 已过期    │ 3 天前   │ ⋯    │
└──────────────┴────────┴─────────────┴──────────┴───────────┴──────────┴──────┘

操作菜单 (⋯ 展开):
┌──────────────┐
│ ✏️  编辑      │
│ 🗑️  吊销      │
└──────────────┘
```

**说明：**
- Scope 列：显示数量（hover/tooltip 展开具体 scope），避免列宽爆炸。
- 状态列：由 `revoked_at`（已吊销）→ `expires_at`（已过期/有效/N天后过期）计算，颜色区分。
- 已吊销行整行置灰。
- 「+ 创建 Token」打开创建 Dialog（见 6.2）。

### 6.2 创建 / 编辑 Token Dialog

创建与编辑共用 `token-form`，编辑模式下 type 字段锁定只读。

```
┌────────────────────────────────────────────────────────────┐
│  创建 Token                                           [×]  │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  名称 *                                                    │
│  ┌──────────────────────────────────────────────────┐     │
│  │ ci-deploy                                         │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  类型 *            ┌──────┐ ┌───────┐                      │
│                    │○ PAT  │ │● Agent │  (编辑时禁用)        │
│                    └──────┘ └───────┘                      │
│                                                            │
│  工作空间 * (Agent 必填)                                   │
│  ┌──────────────────────────────────────────────────┐     │
│  │ [acme ✓] [platform ✓] [internal ✓]      + 添加   │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
│  Scope (按资源勾选)                                        │
│  ┌────────────────────────────────────────────────────┐   │
│  │ 任务 (task)        [全选] [清空]                    │   │
│  │   [✓] read          [✓] write                      │   │
│  │                                                    │   │
│  │ 项目 (project)     [全选] [清空]                    │   │
│  │   [✓] read          [ ] write                      │   │
│  │                                                    │   │
│  │ 工作空间 (workspace) [全选] [清空]                  │   │
│  │   [✓] read          [ ] write                      │   │
│  │                                                    │   │
│  │ Token             [全选] [清空]                     │   │
│  │   [ ] read          [ ] write                      │   │
│  │                                                    │   │
│  │ 成员 / Hook / 通知 / 提醒 / 审计 / 配置 / 上下文 ... │   │
│  │   (同样分组折叠)                                   │   │
│  │                                                    │   │
│  │ 特殊                                               │   │
│  │   [ ] impersonate  (仅 admin/owner 可见)            │   │
│  └────────────────────────────────────────────────────┘   │
│                                                            │
│  过期时间                                                  │
│  ┌──────────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌────────────┐  │
│  │● 永不过期│ │○ 7天 │ │○ 30天│ │○ 90天│ │○ 自定义... │  │
│  └──────────┘ └──────┘ └──────┘ └──────┘ └────────────┘  │
│  (选「自定义」时展开 datetime 选择器)                       │
│                                                            │
│  项目 (可选，须属于已选工作空间)                           │
│  ┌──────────────────────────────────────────────────┐     │
│  │ [agentapi ✓] [web-console ✓]            + 添加   │     │
│  └──────────────────────────────────────────────────┘     │
│                                                            │
├────────────────────────────────────────────────────────────┤
│                          [取消]            [创建]          │
└────────────────────────────────────────────────────────────┘
```

**编辑模式差异：**
- 标题改为「编辑 Token」。
- 「类型」整行禁用（灰显，不可改 PAT↔Agent）。
- 各字段用 token 现有值预填。
- 底部按钮文案改为「保存」。
- 不显示明文 token（编辑不返回 raw token）。

### 6.3 创建成功结果（明文 token 一次性展示）

提交成功后，创建 Dialog 内容切换为结果视图：

```
┌────────────────────────────────────────────────────────────┐
│  Token 创建成功                                       [×]  │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  ⚠️  此 Token 仅显示一次，关闭后无法再次查看，请立即保存。   │
│                                                            │
│  明文 Token                                                │
│  ┌──────────────────────────────────────────┬───────────┐ │
│  │ xuanchu_agent_aBcDeFgHiJkLmNoPqRsTuVwXyZ │ 📋 复制   │ │
│  └──────────────────────────────────────────┴───────────┘ │
│                                                            │
│  Prefix: xuanchu_age   (列表中将显示此前缀)                 │
│                                                            │
│  ✓ 已复制到剪贴板                                          │
│                                                            │
├────────────────────────────────────────────────────────────┤
│                                          [完成]            │
└────────────────────────────────────────────────────────────┘
```

### 6.4 吊销确认（AlertDialog）

```
┌────────────────────────────────────────────┐
│  确认吊销 Token                        [×] │
├────────────────────────────────────────────┤
│                                            │
│  ⚠️  吊销后此 Token 立即失效，且不可恢复。  │
│                                            │
│  名称:   ci-deploy                         │
│  Prefix: xuanchu_age                       │
│  类型:   agent                             │
│                                            │
│  使用此 Token 的客户端将无法再访问 API。    │
│                                            │
├────────────────────────────────────────────┤
│              [取消]        [吊销]          │
└────────────────────────────────────────────┘
```

## 7. 前端改动

### 7.1 新增模块 `web/src/features/workspace/tokens/`

```
tokens/
  token-api.ts          # 类型定义 + API 调用封装
  token-form.tsx        # 创建/编辑共用表单（受控组件）
  scope-editor.tsx      # 按 resource 分组的 scope Checkbox 组件
  token-create-dialog.tsx
  token-edit-dialog.tsx
  token-revoke-dialog.tsx
  token-created-result.tsx  # 创建后明文 token 一次性展示
  use-token-mutations.ts # useMutation 封装（create/modify/revoke）
```

### 7.2 列表页升级

修改 `resource-config.tsx` 的 `tokens` case：
- 增加列：`scopes`（Badge 或「N 个 scope」）、状态（有效 / 过期 / 已吊销，由 `expires_at`/`revoked_at` 计算）、`last_used_at`。
- 由于 `ResourcePage` 当前不支持行内操作按钮和顶部创建按钮，需要给 `tokens` case 一个**专用页面组件** `TokensPage`（不再走通用 `ResourcePage`），或扩展 `ResourcePage` 支持 `actions` 渲染。**推荐专用页面**——token 管理交互比其它资源重，专用页面更清晰，也避免污染通用 ResourcePage 的简单假设。

`TokensPage` 职责：
- 复用 `useQuery(["resource","/api/v1/tokens"])` 拉列表（保持 query key 一致以复用缓存）。
- 顶部「创建 Token」按钮 → 打开 `TokenCreateDialog`。
- 每行操作列（dropdown-menu）：编辑 / 吊销。

### 7.3 表单组件 `token-form.tsx`

创建/编辑共用，受控组件（参考 bootstrap-wizard 模式）。字段：

| 字段 | 创建 | 编辑 | 控件 |
|---|---|---|---|
| name | 必填 | 可改 | Input |
| type | 必填（PAT/Agent） | 只读 | Select（编辑时 disabled） |
| workspaces | agent 必填 | agent 可改 | multi-select（拉 `GET /api/v1/workspaces`） |
| scopes | 必填 | 可改 | `ScopeEditor` 分组 Checkbox |
| projects | 可选 | 可改 | multi-select（依赖已选 workspaces） |
| expires | 可选（默认永不过期） | 可改 | 单选：永不过期 / 7天 / 30天 / 90天 / 自定义 datetime |

### 7.4 Scope 编辑器 `scope-editor.tsx`

把 21 个 scope 按 resource 分组：

```
任务 (task):     [✓] read  [ ] write
项目 (project):  [✓] read  [✓] write
工作空间 (workspace): ...
Token:          ...
成员 / Hook / 通知 / 提醒 / 审计 / 配置 / 上下文 ...
特殊:           [ ] impersonate（仅 admin/owner 可见）
```

- 每组提供「全选 / 全不选」快捷。
- 提交时输出展开后的具体 scope 数组（如 `["task:read","project:read","project:write"]`）。
- 编辑模式用 token 现有 scopes 预填勾选。
- `impersonate` 仅当当前用户 role 为 admin/owner 时显示（前端按登录态判断，后端兜底校验）。

### 7.5 创建结果 `token-created-result.tsx`

创建成功后切换到此视图：
- `<code>` 显示完整明文 token + 复制按钮（复用 bootstrap 的 clipboard 逻辑）。
- 警示文案：「此 token 仅显示一次，关闭后无法再次查看，请立即保存。」
- 「完成」按钮关闭 Dialog 并刷新列表。

### 7.6 数据层 `use-token-mutations.ts`

引入 `useMutation`（项目首个写操作范式）：

```ts
const createMutation = useMutation({
  mutationFn: (input) => workspaceApiPost("/api/v1/tokens", input),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: ["resource", "/api/v1/tokens"] }),
})
// modify / revoke 同理
```

错误处理：捕获 `ApiError`，用 `t("token.errors." + code)` 做 i18n，未知 code 回退到通用错误文案。

### 7.7 吊销确认 `token-revoke-dialog.tsx`

用 `alert-dialog` 二次确认，显示 token name + prefix，确认后调 `DELETE /api/v1/tokens/{ref}`。

### 7.8 i18n（`web/src/locales/en-US.ts` + `zh-CN.ts`）

新增 `token` 命名空间：
- 字段标签：`name` / `type` / `scopes` / `workspaces` / `projects` / `expires` / `prefix` / `lastUsed`
- scope 分组名：`task` / `project` / `workspace` / `token` / `member` / `hook` / `notification` / `reminder` / `audit` / `config` / `context` / `impersonate`
- 操作：`create` / `edit` / `revoke` / `copy` / `copied`
- 状态：`active` / `expired` / `revoked` / `neverExpires`
- 错误码映射：`token_scope_invalid` / `token_agent_requires_workspace` / `token_not_found` 等

## 8. 安全考量

- **权限收窄允许，放大需校验**：修改 workspace/project/scopes 时，复用 `resolveTokenWorkspaces` 的 role 校验（`requireRolePermission(role, PermissionTokenWrite)`）和 `enforceTokenCreateLimit`，确保不能把 token 范围放大到操作者无权的 workspace。
- **impersonate scope 仅 admin/owner**：前端隐藏 + 后端 `tokenManageAllowed` 兜底。
- **明文 token 不落审计**：审计 payload 只记 prefix / scopes / workspace_ids 等元数据，不记 raw token（现有实现已如此，保持）。
- **吊销不可逆**：revoke 后该 token 进入 `revoked_at` 状态，不可再 modify（现有 `ModifyToken` 已拦截 revoked token，保持）。

## 9. 验收标准

**后端：**
- `PATCH /api/v1/tokens/{ref}` 接受 `workspaces` / `projects` 字段并正确更新。
- agent token 清空 workspace 被拒（`token_agent_requires_workspace`）。
- project 不属于 workspace 被拒（`token_project_scope_invalid`）。
- 修改 scopes 时校验基于最终 workspace 集合。
- 审计日志记录 workspace/project 变更。
- `nil`（不改）与空数组（清空）语义正确区分。

**前端：**
- `/tokens` 列表显示 scopes、状态、最后使用时间。
- 可创建 PAT / Agent token，创建后明文 token 一次性展示并可复制。
- 可编辑 name / scopes / 过期 / workspaces / projects。
- 可吊销 token，二次确认。
- scope 编辑器按资源分组，提交具体 scope 数组。
- 编辑时 type 只读。
- 错误有 i18n 文案。

**全局：**
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。
- `cd web && pnpm test && pnpm build` 通过。
- 现有 token 相关测试不回归。

## 10. 实施顺序

1. 后端：Storage `ChangedFields` 补齐 → App `ModifyToken` 扩展（含 nil/空切片语义）→ HTTP `modifyTokenRequest` 补字段 → 后端测试。
2. 前端：`token-api.ts` 类型 → `scope-editor` → `token-form` → 各 Dialog → `TokensPage` → 接入路由 → i18n。
3. 全量验证（Go 测试 + 前端测试 + 构建）。

每步独立可提交、可回退。后端先行，前端依赖后端字段就绪。
