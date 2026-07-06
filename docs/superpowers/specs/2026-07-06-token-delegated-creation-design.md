# Owner/Admin 代为其它用户创建 PAT

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-06
**状态：** 草案
**背景需求：** Web Console `/tokens` 页面下，当登录角色为 owner/admin 时，允许为当前 workspace 的其它成员创建 personal access token（PAT），用于配置 MCP。创建后 admin 还能在列表里筛选并管理这些代为创建的 token。

## 1. 背景与现状

### 1.1 后端已具备能力

经代码核查，**后端完整支持 owner/admin 代为创建 token，本次后端零改动**：

- `POST /api/v1/tokens` 请求体 `createTokenRequest` 已有 `User string` 字段（`internal/httpapi/tokens.go:44`）。
- handler `handleTokenCreate` 把 `req.User` 映射为 `CreateTokenInput.UserRef`（`internal/httpapi/tokens.go:135`）。
- App 层 `resolveTokenTargetUser(UserRef)`（`internal/app/token.go:1231-1243`）：
  - `UserRef` 为空 → 当前登录 actor。
  - `UserRef` 非空 → 解析目标用户，**当目标用户 ≠ 当前 actor 时，要求 `tokenManageAllowed(role)`**（仅 owner/admin）。
- `tokenManageAllowed(role) = role == RoleOwner || role == RoleAdmin`（`internal/app/token.go:1436-1438`）。
- 列表查询 `resolveTokenListUser`（`internal/app/token.go:1245-1257`）同样支持 owner/admin 传 `UserRef` 查看他人 token。

因此本需求的本质是**前端能力补全**：暴露后端已有的 `user` 字段和列表筛选能力。

### 1.2 前端缺口

当前前端没有任何"代为创建"或"查看他人 token"的入口：

- `TokenCreateInput` 类型（`token-api.ts:60-67`）无 `user` 字段。
- `valuesToCreateInput`（`token-form.tsx:306-316`）不发送 `user`。
- `TokenForm`（`token-form.tsx`）无目标用户选择器，全仓库无 `UserPicker` / `Combobox` 组件，`package.json` 也未引入 `cmdk`。
- `handleTokenList` 前端调用（`tokens-page.tsx:56`）查询 key 为 `["resource", "/api/v1/tokens"]`，未带 `?user=` 参数，admin 只能看到自己的 token。

### 1.3 角色与权限边界

- 角色定义在 `internal/authz/model.go:10-15`：`owner / admin / member / viewer`。
- 权限矩阵 `authz.AllowedForRole`（`internal/authz/policy.go:10-40`）：`PermissionTokenRead/Write` 仅 owner/admin 拥有。
- 前端 `useMe()` 返回 `effective_role`（`session/useMe.ts`），取值为 `"owner"|"admin"|"member"|"viewer"`。
- 现有 `canImpersonate = role === "admin" || role === "owner"` 判断（`tokens-page.tsx:80-83`）已是 owner/admin 门控，可直接复用。

## 2. 目标

1. owner/admin 在 `/tokens` 创建 token 时，表单内可选择「归属用户」，默认「我自己」。
2. 「归属用户」选择器仅对 owner/admin 可见；member/viewer 看不到该字段，行为不变。
3. 选择器支持按姓名/邮箱搜索，数据源为当前 workspace 成员列表。
4. 仅 PAT 类型展示归属用户选择器；Agent token 表单不变。
5. 创建成功后，admin 可在 `/tokens` 列表顶部用筛选器切换查看不同成员的 token。
6. 后端无改动；所有鉴权仍走现有 `resolveTokenTargetUser` / `resolveTokenListUser`。

## 3. 非目标

- 不改动 Agent token 的归属逻辑（Agent 是机器身份，代创建语义不清）。
- 不改动 tenant access token（它不绑定自然人用户）。
- 不改动 `X-Xuanchu-As` impersonation 机制（那是请求级 header，与本需求的"token 归属"是不同概念）。
- 不改动后端任何代码。
- 不引入跨 workspace 的代为创建（仅在当前 effective workspace 成员范围内）。
- 不新增审计字段或审计事件（创建 token 走现有 `token.create` 审计，已记录 actor 和 token 归属 user）。
- 不做 token 批量代创建。

## 4. 核心产品决策

### 4.1 仅前端补全，后端零改动

后端 `POST /api/v1/tokens` 的 `user` 字段、`resolveTokenTargetUser` 的 owner/admin 校验、`resolveTokenListUser` 的 owner/admin 查询能力都已就绪。本需求只需让前端在请求体带上 `user` 字段、在列表请求带上 `?user=` 参数。

### 4.2 选择器使用 Combobox（Popover + cmdk）

owner/admin 通常为特定成员创建 token，成员数量可能从几个到几十个。Combobox 搜索体验最好：
- 支持按姓名、display_name、email 模糊搜索。
- 单选语义清晰，与 PAT 归属单用户一致。
- 默认选中「我自己」，保持向后兼容。

代价：需新增 `cmdk` 依赖和 `command.tsx` 组件文件。考虑到后续成员选择场景会复用（如 assignee 选择、hook 配置等），这个投入值得。

### 4.3 仅 PAT 类型展示选择器

Agent token 是机器身份，归属某自然人语义不清；且 admin 创建 agent token 给他人用，他人无法管理（agent token 的管理边界更复杂）。本需求只覆盖 PAT。Agent token 表单的 type 字段切换为 agent 时，归属用户字段隐藏且清空。

### 4.4 列表筛选器复用 Combobox

列表顶部新增「查看用户」筛选器，复用同一个 UserPicker 组件（但默认「我自己」而非可清空）。admin 切换筛选器后，token 列表请求带 `?user=<ref>`。普通成员不显示该筛选器。

## 5. 前端设计

### 5.1 数据流

```
┌─────────────────────────────────────────────────────────────┐
│  admin 打开「创建 Token」弹窗 (type=pat)                      │
│                                                              │
│  「归属用户」Combobox:                                        │
│    默认 = 我自己 (admin 的 user id)                           │
│    数据源 = GET /api/v1/workspaces/{slug}/members            │
│                                                              │
│  admin 选「张三」→ 提交                                       │
└────────────────────┬─────────────────────────────────────────┘
                     │ POST /api/v1/tokens
                     │ { name, type:"pat", user:"zhangsan-id", scopes, ... }
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  后端 (已有逻辑, 无改动):                                      │
│                                                              │
│  handleTokenCreate                                           │
│    └─ scoped.CreateToken(UserRef: req.User)                  │
│         └─ resolveTokenTargetUser("zhangsan-id"):            │
│              ├─ resolveUser → storage.User{ID: zhangsan-id}  │
│              └─ actor.Role==admin → tokenManageAllowed ✓     │
│         └─ UserID = zhangsan-id 写入 api_tokens              │
│                                                              │
│  返回 201 { token:"xuanchu_pat_xxx", user:{张三}, ... }      │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 创建弹窗原型（admin 视角）

普通成员看到的表单与现在完全一致（无「归属用户」字段）。

```
┌─ 创建 Token ──────────────────────────────────────[ × ]┐
│                                                        │
│  名称                                                   │
│  ┌──────────────────────────────────────────────┐      │
│  │ ci-deploy                                     │      │
│  └──────────────────────────────────────────────┘      │
│                                                        │
│  类型                                                   │
│  ┌──────────────────────────────────────────────┐      │
│  │ PAT                                       ▾ │      │
│  └──────────────────────────────────────────────┘      │
│                                                        │
│  归属用户                            ★仅 owner/admin   │
│  ┌──────────────────────────────────────────────┐      │
│  │ ● 我自己 (admin)                          ▾ │      │  ← 默认
│  └──────────────────────────────────────────────┘      │
│  为当前 workspace 成员创建，token 将归属所选用户           │
│                                                        │
│  工作空间                                                │
│  ┌──────────────────────────────────────────────┐      │
│  │ ☑ default  ☐ staging                         │      │
│  └──────────────────────────────────────────────┘      │
│                                                        │
│  权限范围 (scopes)                                      │
│  [ task:read ] [ task:write ] [ + 添加 ]                │
│                                                        │
│  过期时间                                                │
│  ┌──────────────────────────────────────────────┐      │
│  │ 90 天                                     ▾ │      │
│  └──────────────────────────────────────────────┘      │
│                                                        │
│              [ 取消 ]  [ 创建 ]                          │
└────────────────────────────────────────────────────────┘
```

### 5.3 Combobox 展开态原型

```
┌──────────────────────────────────────────────┐
│ 🔍 搜索成员姓名或邮箱…                        │
├──────────────────────────────────────────────┤
│ ✓ 我自己                          admin      │  ← 当前选中,置顶
│   张三           zhangsan@example.com        │
│   李四           lisi@example.com   viewer   │
│   王五           wangwu@example.com  member  │
└──────────────────────────────────────────────┘
```

- 列表项展示：display_name（或 name）+ email（次要）+ role（次要 Badge）。
- 「我自己」始终置顶，标注当前角色。
- 搜索匹配 display_name / name / email（不区分大小写，子串匹配）。
- 选中后 Combobox trigger 显示「我自己 (admin)」或「张三」。

### 5.4 列表筛选器原型（admin 视角）

```
┌─ API Tokens ───────────────────────────────────────────────┐
│                                                            │
│  查看用户                                                   │
│  ┌────────────────────────────────┐  [ + 创建 Token ]      │
│  │ ● 我自己 (admin)            ▾ │                        │
│  └────────────────────────────────┘                        │
│                                                            │
│  ┌────────────────────────────────────────────────────────┐│
│  │ 名称     类型  prefix        权限   状态  最后使用 操作 ││
│  ├────────────────────────────────────────────────────────┤│
│  │ ci-deploy PAT  xuanchu_pat.. 2      有效  —      ⋯    ││
│  └────────────────────────────────────────────────────────┘│
└────────────────────────────────────────────────────────────┘
```

- 筛选器仅 owner/admin 可见（普通成员不显示，列表行为不变）。
- 默认「我自己」；切换为「张三」后，请求 `GET /api/v1/tokens?user=zhangsan-id`。
- query key 变为 `["resource", "/api/v1/tokens", { user: selectedUserRef }]`，切换时重新请求。
- MCP 配置按钮、编辑、撤销等行操作在筛选他人 token 时仍可用（权限由后端 `PermissionTokenRead/Write` + scope 兜底）。

### 5.5 新增组件与文件

```text
web/src/components/ui/command.tsx          # 新建: shadcn Command (基于 cmdk)
web/src/features/workspace/tokens/
  user-picker.tsx                          # 新建: UserPicker Combobox (创建+列表复用)
  user-picker.test.tsx                     # 新建: 单测
```

修改的现有文件：

```text
web/package.json                           # 新增 cmdk 依赖
web/src/features/workspace/tokens/
  token-api.ts                             # TokenFormValues / TokenCreateInput 增 user 字段
  token-form.tsx                           # 增归属用户 Field (仅 canManageUsers && type==pat)
  tokens-page.tsx                          # 增列表筛选器 state + UserPicker
  tokens-page.test.tsx                     # 增 admin 代创建 + 筛选测试
  token-create-dialog.tsx                  # 透传 canManageUsers 到 TokenForm (新增 prop)
web/src/locales/zh-CN.ts                   # 新增文案
web/src/locales/en-US.ts                   # 新增文案
```

### 5.6 类型与 API 扩展

`token-api.ts`：

```ts
export type TokenFormValues = {
  name: string
  type: string
  user?: string              // ★ 新增: 归属用户 ref (user_id), 空=我自己
  workspaces: string[]
  scopes: string[]
  projects: string[]
  expiresPreset: ExpiresPreset
  expiresAt: string
}

export type TokenCreateInput = {
  name: string
  type?: string
  user?: string              // ★ 新增
  scopes?: string[]
  workspaces?: string[]
  projects?: string[]
  expires_in_seconds?: number | null
}
```

`valuesToCreateInput`（`token-form.tsx:306`）扩展：

```ts
export function valuesToCreateInput(values: TokenFormValues) {
  const expires = presetToExpiresSeconds(values.expiresPreset, values.expiresAt)
  return {
    name: values.name.trim(),
    type: values.type,
    ...(values.user ? { user: values.user } : {}),   // 仅非空时发送
    scopes: values.scopes,
    workspaces: values.workspaces,
    projects: values.projects,
    expires_in_seconds: expires,
  }
}
```

列表查询（`tokens-page.tsx`）：

```ts
const [viewUser, setViewUser] = useState<string>("")  // 空=我自己
const tokensQueryKey = ["resource", "/api/v1/tokens",
  ...(viewUser ? [{ user: viewUser }] : [])]
// fetch 时拼 ?user=<ref>
```

### 5.7 UserPicker 组件契约

```ts
type UserPickerProps = {
  value: string                       // 选中的 user_id, "" 表示"我自己"
  onChange: (userId: string) => void
  placeholder?: string
  includeSelf?: boolean               // 默认 true, 列表筛选和创建都用 true
  disabled?: boolean
}
```

- 内部用 `useQuery` 拉 `listWorkspaceMembers(me.data.effective_workspace.slug)`，`queryKey: ["workspace", "members", slug]`。
- value="" 始终表示「我自己」语义。**UserPicker 不负责把 "" 转成 admin 的 user id**；调用方（表单提交 / 列表筛选）负责：创建表单提交时空字符串导致请求体不带 `user` 字段（后端默认归属 actor），列表筛选时空字符串导致不带 `?user=` 参数（后端默认查 actor 自己）。这样 UserPicker 保持无状态、可复用。
- 数据源复用 `web/src/features/workspace/members/members-api.ts` 的 `listWorkspaceMembers` 和 `WorkspaceMemberRow` 类型。
- 渲染用 `Popover` + 新建的 `Command`（cmdk）。

### 5.8 表单显隐规则

在 `TokenForm` 中：

```tsx
const showUserField = canManageUsers && values.type === "pat"
```

- `canManageUsers` 由 `TokensPage` 通过 `me.data?.effective_role === "admin" || "owner"` 计算后透传。
- 注意：它与现有 `canImpersonate`（控制能否给新 token 分配 impersonate scope）的判断条件恰好相同，但语义不同。**不复用同一 prop**，避免"修改其中一个判断时不小心影响另一个"。两者都从 `TokensPage` 分别透传到 `TokenForm`。
- `type` 切换为 `agent` 时，隐藏字段并把 `values.user` 重置为 ""（即我自己），避免遗留脏值。
- 编辑模式（`mode === "edit"`）：不显示归属用户字段（token 归属不可变更），且 `valuesToModifyInput` 不发送 `user`。

### 5.9 错误处理

后端可能返回的错误码与前端内联呈现（复用现有 `formError` state，`token-form.tsx:91-107`）：

| 错误码 | HTTP | 场景 | 前端文案 |
|---|---:|---|---|
| `permission_denied` | 403 | 非 owner/admin 传 user，或被降权 | "你没有为其它用户创建 token 的权限" |
| `user_not_found` | 404 | 目标用户 ref 无法解析 | "找不到所选用户，请重新选择" |
| `token_scope_denied` | 403 | 调用凭证缺 `token:write` | 现有文案 |
| 其它 | - | - | 现有 `token.errors.unknown` 兜底 |

`TokenCreateDialog` 的 mutation `onError` 已把错误写入 dialog 内 `error` state（`token-create-dialog.tsx`）。本次新增把后端 error code 映射到中文文案，在表单顶部 `formError` 位置展示。

## 6. 安全边界

1. **前端显隐不等于鉴权。** 即便前端被绕过，后端 `resolveTokenTargetUser` 仍强制 owner/admin 校验。
2. **仅当前 workspace 成员可选。** UserPicker 数据源是 `/api/v1/workspaces/{slug}/members`，非该 workspace 成员不会出现在列表中（后端 `resolveUser` 虽能解析任意用户，但前端不暴露）。
3. **scope 上限校验不变。** 代为创建的 token 仍受 `enforceTokenCreateLimit` 约束，scopes/workspace/project 必须是当前调用凭证（admin 自己的 token）的子集。
4. **审计不变。** 创建走现有 `token.create` 审计，已记录 actor（admin）和 token 归属 user（张三），无需新增审计事件。
5. **列表筛选受同样的 owner/admin 校验。** member/viewer 即便手动构造 `?user=` 请求，也会被 `resolveTokenListUser` 拒绝。
6. **明文 token 只在创建响应中返回一次。** 代为创建时，明文展示给当前操作的 admin（这是预期行为：admin 需要把 token 交给被代理用户）。

## 7. 文案

新增中文文案（`zh-CN.ts` 的 `token` 块）：

```ts
token.field.user = "归属用户"
token.field.userHint = "为当前 workspace 成员创建，token 将归属所选用户"
token.user.self = "我自己"
token.user.searchPlaceholder = "搜索成员姓名或邮箱"
token.user.viewFilter = "查看用户"
token.errors.permission_denied = "你没有为其它用户创建 token 的权限"
token.errors.user_not_found = "找不到所选用户，请重新选择"
```

英文文案（`en-US.ts`）：

```ts
token.field.user = "Owner"
token.field.userHint = "Create for a workspace member; the token belongs to the selected user"
token.user.self = "Myself"
token.user.searchPlaceholder = "Search by name or email"
token.user.viewFilter = "View user"
token.errors.permission_denied = "You do not have permission to create tokens for other users"
token.errors.user_not_found = "Selected user not found, please choose again"
```

## 8. 测试计划

### 8.1 前端单测

新增 `user-picker.test.tsx`：

- 渲染时默认显示「我自己」。
- 输入关键字过滤成员列表。
- 选中成员后 trigger 显示对应姓名。
- `includeSelf=false` 时不显示「我自己」。

更新 `tokens-page.test.tsx`：

- admin（`effective_role: "owner"`）创建弹窗显示「归属用户」字段。
- member（`effective_role: "member"`）创建弹窗不显示该字段。
- type 切换为 agent 时，归属用户字段隐藏。
- admin 切换列表筛选器后，fetch 请求带 `?user=` 参数。
- 后端返回 `permission_denied` 时，表单内联展示错误文案。
- 普通成员不显示列表筛选器。

更新现有 token-form / token-create 相关测试：确保 `user` 字段为空时请求体不含 `user`（向后兼容）。

### 8.2 后端测试

**后端无代码改动，不新增后端测试。** 现有 `token_test.go` 已覆盖 `resolveTokenTargetUser` 的 owner/admin 校验和拒绝路径。

### 8.3 验证命令

前端：

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
```

后端（确认无回归，因本次不改后端）：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 9. 实施顺序建议

1. 新增 `cmdk` 依赖，创建 `web/src/components/ui/command.tsx`（shadcn 风格）。
2. 创建 `UserPicker` 组件 + 单测。
3. 扩展 `TokenFormValues` / `TokenCreateInput` / `valuesToCreateInput`。
4. `TokenForm` 增加归属用户 Field（仅 `canManageUsers && type==="pat"`）。
5. `TokenCreateDialog` 透传 `canManageUsers`。
6. `TokensPage` 增加列表筛选器（admin 可见）和 query key 参数化。
7. 补 i18n 文案。
8. 补前端测试，跑验证命令。
9. 更新 `docs/manual/web-console.md`（若已有 /tokens 说明）和 `README.md` 中 token 创建相关段落。

## 10. 验收标准

- owner/admin 打开创建弹窗，type=pat 时可见「归属用户」Combobox，默认「我自己」。
- admin 选择张三并提交，网络请求体含 `user: "zhangsan-id"`，创建成功后返回的 token 归属张三。
- member/viewer 打开创建弹窗，看不到「归属用户」字段，提交的请求体不含 `user`。
- type 切换为 agent 时，归属用户字段隐藏。
- admin 在列表顶部切换「查看用户」为张三，列表刷新并显示张三的 token。
- 普通成员看不到列表筛选器，且无法通过任何前端入口查看他人 token。
- 后端返回 `permission_denied` 时，表单内联显示中文错误文案。
- `pnpm --dir web typecheck / test / lint / build` 通过。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过（后端无改动，应无回归）。
