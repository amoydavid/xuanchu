# Web Console 成员管理页增强设计

- 日期：2026-07-04
- 状态：已实现
- 关联页面：`/members`
- 关联文档：[README.md](../../../README.md)、[ROADMAP.md](../../../ROADMAP.md)、[Workspace OIDC 接入设计](./2026-07-03-workspace-oidc-design.md)、[Tenant Console System Owner 设计](./2026-07-02-xuanchu-tenant-console-system-owner-design.md)

## 0. 实现状态

本规格已经落地到 Web Console、HTTP API、app service 和 storage 层：

- `/members` 主页面支持成员搜索、角色筛选、添加已有用户、创建最小用户并加入 workspace、编辑展示姓名、调整角色和移出成员。
- `/members/:userRef` 次级页面展示成员身份快照、membership 元数据、外部身份摘要、关联 token 跳转和最近成员审计。
- 添加成员、编辑展示姓名、调整角色、owner 风险确认、移出成员等弹窗全部使用 shadcn/Radix UI 组件。
- 服务端新增成员移出能力，并把 `PATCH /api/v1/workspaces/{workspace}/members/{user}` 扩展为可同时更新 `role` 与 `display_name`。
- 浏览器 OIDC session 具备 `member:write`，但继续排除 `token:write` 与 `impersonate`；实际授权仍由 membership role、scope、workspace/project allowlist 和 CSRF 共同约束。
- README 与 ROADMAP 已同步当前用户可见行为；CLI 仍没有 `member delete`，Web Console/HTTP API 支持移出 membership 且不删除 user。

## 1. 背景

当前 Web Console 的 `/members` 页面已经能列出当前 workspace 成员，并支持编辑成员的 `display_name`。后端已有：

- `GET /api/v1/workspaces/{workspace}/members`
- `POST /api/v1/workspaces/{workspace}/members`
- `PATCH /api/v1/workspaces/{workspace}/members/{user}` 修改角色
- `PATCH /api/v1/users/{user}` 修改 `display_name`

但页面还没有覆盖日常成员管理闭环：新增成员、调整角色、移出成员、明确 owner/admin/member 各自能做什么。当前也没有 `member delete`，README 仍写着“临时撤销写权限可降级为 viewer”。这会让 workspace owner/admin 在 Web Console 中无法完成常见组织维护动作。

本设计目标是把 `/members` 从只读名册增强为 workspace 级成员管理入口，同时避免把 SSO 配置、token 管控、server admin 控制面等能力塞进同一个页面。

## 2. 设计原则

1. **成员页只处理 workspace membership。** 用户身份、外部身份、SSO 同步、token 管控可以在成员页露出摘要和跳转，但不在主列表里展开复杂配置。
2. **权限以服务端为准，前端只做显式提示。** 所有写操作仍通过 app/authz 层校验：membership role、token scope、workspace/project allowlist、browser session/tenant actor 边界共同生效。
3. **owner 处理 owner 级风险，admin 处理日常成员维护，member 只读协作名册。**
4. **保护最后一个 owner。** 任何移出、降级、禁用 owner 的操作都不能让 workspace 没有 owner。
5. **稳定名与展示名分离。** `users.name` 仍是稳定引用；`display_name` 是展示字段。页面文案必须明确“修改展示姓名，不修改稳定引用名”。

## 3. 角色与权限矩阵

本节只描述 workspace 成员管理页的交互权限。实际执行还要同时满足 capability scope。例如自然人 owner 需要凭证具备 `member:write` 才能写成员；本规格要求 browser OIDC session 增加 `member:write`，但最终仍由用户的 workspace role 决定能否执行成员写操作。

| 操作 | owner | admin | member | viewer |
|---|---|---|---|---|
| 查看成员列表 | 可以 | 可以 | 可以 | 可以 |
| 搜索/筛选/复制成员标识 | 可以 | 可以 | 可以 | 可以 |
| 查看成员详情页 | 可以 | 可以 | 可以 | 可以 |
| 新增 viewer/member/admin | 可以 | 可以 | 不可以 | 不可以 |
| 新增 owner | 可以 | 不可以 | 不可以 | 不可以 |
| 调整非 owner 角色 | 可以 | 可以 | 不可以 | 不可以 |
| 授予 owner | 可以 | 不可以 | 不可以 | 不可以 |
| 降级 owner | 可以，但不能降级最后一个 owner | 不可以 | 不可以 | 不可以 |
| 移出非 owner 成员 | 可以 | 可以 | 不可以 | 不可以 |
| 移出 owner | 可以，但不能移出最后一个 owner | 不可以 | 不可以 | 不可以 |
| 编辑非 owner 展示姓名 | 可以 | 可以 | 不可以 | 不可以 |
| 编辑 owner 展示姓名 | 可以 | 不可以 | 不可以 | 不可以 |
| 查看外部身份摘要 | 可以 | 可以 | 可以 | 可以 |
| 绑定/解绑外部身份 | 二期：成员详情页，仅 owner | 不可以 | 不可以 | 不可以 |

补充规则：

- `tenant_access_token` 是 workspace system owner credential。它不出现在成员列表中；登录 Console 后按 owner 等价能力执行，但仍受 token scope 约束。
- server admin 不能直接访问普通 `/api/v1/*` 成员 API；必须通过现有 acting session 或 tenant switch 进入 workspace Console。acting session 的权限取决于被委托用户当前 membership role。
- OIDC browser session 是浏览器凭证，不是 PAT。本规格选择让浏览器会话具备 `member:write`，再由 membership role 决定 owner/admin/member 的实际能力；不因此开放 `token:write` 或 `impersonate`。

## 4. 页面边界

### 4.1 `/members` 主页面

主页面放高频、低上下文的成员维护动作：

- 成员总览：owner/admin/member/viewer 计数。
- 成员列表：姓名、稳定名、邮箱、角色、外部身份摘要、加入时间、最近修改时间。
- 搜索与筛选：按姓名、稳定名、邮箱、角色、外部身份 provider。
- 新增成员：弹窗完成“选择已有 user 或创建最小 user，再加入 workspace”。
- 角色调整：非危险角色调整可在行内完成；owner 授予/降级必须二次确认。
- 展示姓名编辑：行内弹窗，文案说明不改变 `users.name`。
- 移出成员：危险操作，二次确认；不能移出最后一个 owner。

不放在主页面：

- SSO/OIDC 配置和通讯录同步配置，继续放 `/sso`。
- token 创建、吊销、scope 管理，继续放 `/tokens` 或 `/admin/tokens`。
- server admin 跨 workspace 管控，继续放 `/admin/workspaces/:workspace`。
- 项目成员、任务 assignee 批量调整，本期不做；仍在项目/任务相关页面处理。

### 4.2 `/members/:userRef` 次级成员详情页

详情页放低频、需要更多上下文的内容：

- 成员身份快照：`UserInfo`、稳定名、展示名、邮箱、外部身份。
- 当前 workspace membership：角色、加入时间、修改时间。
- 角色变更历史和成员相关审计记录。
- 关联 token 摘要：只展示数量和跳转到 `/tokens?user=...`，不在这里直接管 token。
- 危险区：移出 workspace、owner 降级等需要说明后果的动作。
- 二期能力：外部身份绑定/解绑。该能力涉及登录与目录同步边界，默认不在本期主列表展开。

### 4.3 弹窗

主页面只使用弹窗处理轻量任务：

- “添加成员”弹窗。
- “编辑展示姓名”弹窗。
- “调整为 owner / 降级 owner / 移出成员”确认弹窗。

如果一个流程需要查看审计、token、外部身份详情，就跳详情页，不在弹窗里堆信息。

## 5. ASCII 原型

### 5.1 owner 视角主页面

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 成员                                                     [添加成员]          │
│ owner 2 · admin 3 · member 18 · viewer 5                                     │
├──────────────────────────────────────────────────────────────────────────────┤
│ 搜索姓名/邮箱/稳定名...        角色 [全部 ▾]      外部身份 [全部 ▾]          │
├──────────────────────────────────────────────────────────────────────────────┤
│ 成员                  角色          外部身份        加入时间        操作       │
├──────────────────────────────────────────────────────────────────────────────┤
│ Alice Chen            owner         yaoguang        2026-06-10      [···]     │
│ alice                 alice@example.com                                      │
│                                                                              │
│ Bob Li                admin ▾       yaoguang        2026-06-12      [···]     │
│ bob                   bob@example.com                                        │
│                                                                              │
│ Carol Wang            member ▾      -               2026-06-20      [···]     │
│ carol                 carol@example.com                                      │
└──────────────────────────────────────────────────────────────────────────────┘

行菜单：
  查看详情
  编辑展示姓名
  调整角色
  移出 workspace
```

owner 可以看到所有行操作。涉及 owner 的变更使用确认弹窗，并在服务端保护最后一个 owner。

### 5.2 admin 视角主页面

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 成员                                                     [添加成员]          │
│ owner 2 · admin 3 · member 18 · viewer 5                                     │
├──────────────────────────────────────────────────────────────────────────────┤
│ 搜索姓名/邮箱/稳定名...        角色 [全部 ▾]      外部身份 [全部 ▾]          │
├──────────────────────────────────────────────────────────────────────────────┤
│ 成员                  角色          外部身份        加入时间        操作       │
├──────────────────────────────────────────────────────────────────────────────┤
│ Alice Chen            owner         yaoguang        2026-06-10      [查看]     │
│ alice                 owner 由 owner 管理                                    │
│                                                                              │
│ Bob Li                admin ▾       yaoguang        2026-06-12      [···]     │
│ bob                                                                          │
│                                                                              │
│ Carol Wang            member ▾      -               2026-06-20      [···]     │
│ carol                                                                        │
└──────────────────────────────────────────────────────────────────────────────┘
```

admin 能管理非 owner 成员，包括新增 admin/member/viewer、调整非 owner 角色、移出非 owner 成员。owner 行只允许查看详情，所有 owner 级操作隐藏或置灰并说明原因。

### 5.3 member 视角主页面

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 成员                                                                         │
│ owner 2 · admin 3 · member 18 · viewer 5                                     │
├──────────────────────────────────────────────────────────────────────────────┤
│ 搜索姓名/邮箱/稳定名...        角色 [全部 ▾]      外部身份 [全部 ▾]          │
├──────────────────────────────────────────────────────────────────────────────┤
│ 成员                  角色          外部身份        加入时间                  │
├──────────────────────────────────────────────────────────────────────────────┤
│ Alice Chen            owner         yaoguang        2026-06-10                │
│ Bob Li                admin         yaoguang        2026-06-12                │
│ Carol Wang            member        -               2026-06-20                │
└──────────────────────────────────────────────────────────────────────────────┘
```

member 只需要知道“这个 workspace 里有哪些协作者以及谁能处理管理问题”。不提供成员写操作，不在这个页面放“退出 workspace”自助能力；如果未来需要自助退出，应放个人设置或成员详情危险区，并单独设计 active workspace、任务 assignee、最后 owner 等后果。

### 5.4 添加成员弹窗

```text
┌──────────────────────────────────────────────┐
│ 添加成员                                      │
├──────────────────────────────────────────────┤
│ 成员来源                                      │
│ (●) 选择已有用户   ( ) 创建新用户             │
│                                              │
│ 用户                                          │
│ [ 搜索 name / email / external id...      ]  │
│                                              │
│ 角色                                          │
│ [ member ▾ ]                                 │
│                                              │
│ owner 角色只能由 owner 授予。                 │
│                                              │
│                         [取消] [添加]        │
└──────────────────────────────────────────────┘
```

创建新用户时显示 `name`、`display_name`、`email` 三个字段。`name` 是稳定引用名，必须唯一；`display_name` 可为空，空时用 `name` 展示。

### 5.5 成员详情页

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ ← 成员                                                                       │
│ Alice Chen                                       owner                       │
│ alice · alice@example.com · yaoguang:ou_xxx                                   │
├──────────────────────────────┬───────────────────────────────────────────────┤
│ 成员信息                      │ 管理动作                                      │
│ 稳定名      alice             │ 角色                         [owner ▾]        │
│ 展示姓名    Alice Chen        │                                               │
│ 邮箱        alice@example.com │ [编辑展示姓名]                                │
│ 加入时间    2026-06-10        │ [移出 workspace]                              │
│ 修改时间    2026-07-01        │                                               │
├──────────────────────────────┴───────────────────────────────────────────────┤
│ 外部身份                                                                      │
│ yaoguang / ou_xxx                                                             │
│ feishu / user_xxx                                                             │
├──────────────────────────────────────────────────────────────────────────────┤
│ 最近成员审计                                                                  │
│ 2026-07-01 Bob Li changed role member -> admin                                │
│ 2026-06-20 Alice Chen added Carol Wang as member                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

member/viewer 访问详情页时只看到只读信息，不出现右侧管理动作。

## 6. 后端契约调整

### 6.1 保留既有接口

继续使用：

```http
GET /api/v1/workspaces/{workspace}/members
POST /api/v1/workspaces/{workspace}/members
PATCH /api/v1/workspaces/{workspace}/members/{user}
```

`PATCH /members/{user}` 建议从只支持 `role` 扩展为同时支持：

```json
{
  "role": "admin",
  "display_name": "Bob Li"
}
```

服务端分开校验：

- `role` 变更走 `member:write` + `PermissionMemberManage` / `PermissionMemberManageOwner`。
- `display_name` 变更走成员管理规则，不再让成员页直接依赖宽泛的 `PATCH /api/v1/users/{user}` + `workspace.modify`。

这样可以避免“为了改展示名必须给整套 workspace metadata 修改权限”的耦合，也能阻止 admin 修改 owner 展示名。

### 6.2 新增移出成员接口

新增：

```http
DELETE /api/v1/workspaces/{workspace}/members/{user}
```

语义：

- 删除该 user 在该 workspace 的 membership。
- 不删除 `users`，不删除任务、审计、token 记录。
- 如果目标是最后一个 owner，返回 `permission_denied` 或更具体的 `last_owner_required`。
- 如果目标是 owner，只有 owner 可执行。
- 如果目标是非 owner，owner/admin 可执行。
- archived workspace 拒绝成员写操作。
- 审计 action：`member.remove`，target type：`member`，target id：目标 `user_id`。

移出后的连带处理：

- 目标用户再访问该 workspace 时返回 `membership_not_found`。
- 若目标用户 active workspace 指向该 workspace，下一次解析 active context 时应落到其他未归档 workspace；没有可用 workspace 时返回明确错误。这里可复用现有 `OtherUnarchivedWorkspaces` 能力。
- 目标用户仍可能拥有绑定该 workspace 的 PAT/Agent token；token 不立即删除，但后续请求因 membership 缺失或 workspace scope 校验失败而不可用。成员详情页可给 owner/admin 一个跳转到 `/tokens?user=...` 的入口。

### 6.3 添加成员行为

新增成员在既有 `POST /members` 上扩展请求体，支持两种互斥输入。

加入已有 user：

```json
{
  "user": "bob",
  "role": "member"
}
```

创建新 user 并加入 workspace：

```json
{
  "new_user": {
    "name": "bob",
    "display_name": "Bob Li",
    "email": "bob@example.com"
  },
  "role": "member"
}
```

服务端语义：

- `user` 路径保留当前行为：解析已有 user ref，然后创建 membership。
- `new_user` 路径在同一个 app/storage transaction 内创建 user 与 membership，避免前端先创建 user、后加成员失败的半成品。
- 权限仍按成员管理判断：`member:write` + `PermissionMemberManage` / `PermissionMemberManageOwner`。不要求成员页额外持有宽泛的 `user:write`。
- 如果 `user` 与 `new_user` 同时出现，返回 `invalid_input`。
- 如果 `new_user.name` 已存在或 email 指向另一个 user，返回现有 user 冲突错误，不隐式合并。

## 7. 前端行为

### 7.1 能力判断

页面通过 `GET /api/v1/credentials/current` 获取：

- `actor_type`
- `effective_role`
- `capabilities`
- `effective_workspace`

前端派生：

```text
canReadMembers = capabilities includes member:read
canWriteMembers = capabilities includes member:write
isOwner = effective_role == owner
isAdmin = effective_role == admin
isManager = isOwner || isAdmin
```

按钮展示规则：

- `!canWriteMembers`：所有写按钮隐藏，页面顶部显示只读提示。
- `admin + owner row`：只显示“查看详情”。
- `member/viewer`：不显示“添加成员”和行操作菜单。
- `tenant_access_token`：按 owner 等价展示，但页面顶部标记“系统身份”。
- `browser_session`：按本规格应带 `member:write`；如果运行时缺少该 capability，页面进入只读降级并说明原因。

### 7.2 失败处理

前端不把置灰逻辑当安全边界。任何写操作失败时：

- `permission_denied`：提示“当前角色或凭证不允许执行该操作”。
- `membership_not_found`：提示“目标成员已不在当前 workspace”并刷新列表。
- `workspace_archived`：提示“已归档 workspace 不允许修改成员”。
- `last_owner_required` 或文本包含 last owner：提示“必须至少保留一个 owner”。

### 7.3 列表状态

列表加载与空状态沿用现有 shadcn 表格风格，不做卡片堆叠：

- loading：表格 skeleton。
- empty：保留表头，正文显示“暂无成员”。
- error：显示可重试错误条。
- filter 后无结果：显示“没有匹配成员”，不等同于 workspace 无成员。

## 8. 非目标

- 不实现邮件邀请、注册码、邀请链接。
- 不实现 workspace 外的全局用户目录页。
- 不在 `/members` 管理 token scope、PAT/Agent token 或 tenant token。
- 不在 `/members` 配置 OIDC、SCIM 或通讯录同步策略。
- 不实现项目级成员角色；当前权限仍是 workspace role。
- 不删除 user；移出成员只删除 membership。

## 9. 验收标准

功能验收：

- owner 可以在 `/members` 添加成员、授予 owner、降级 owner、移出成员，且不能移出/降级最后一个 owner。
- admin 可以添加 admin/member/viewer，调整和移出非 owner 成员；owner 行不可写。
- member 只能查看、搜索、进入只读详情页。
- tenant token 登录时按 owner 等价展示和操作，但仍受 scope 限制。
- SSO browser session 下的 owner/admin 可以执行成员写操作；实现时需要把 browser session capability 增加 `member:write`，但继续排除 `token:write` 与 `impersonate`。

技术验收：

- 成员写操作复用 `internal/app`，不在 `internal/cli` 或 `internal/httpapi` 写业务规则。
- 新增移出成员能力需要 app/storage/httpapi/remote/MCP 是否跟进由 implementation plan 明确；若只做 Web Console/HTTP，MCP tool 命名必须遵守下划线规范。
- 所有 JSON 用户引用继续使用统一 `task.UserInfo` 规则；成员列表现有扁平字段若要升级，应在 spec/plan 中明确兼容策略。
- 写操作全部进入 audit：`member.add`、`member.role`、`member.remove`、`user.modify` 或新的 `member.profile.modify`。

验证命令：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
git diff --check
```

当前实现验证记录：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web test
pnpm --dir web build
git diff --check
```
