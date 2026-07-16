# SaaS 自助 Onboarding 设计（个人开放注册 + 团队邀请制）

- 日期：2026-07-16
- 状态：草案
- 范围：往 SaaS 方向迈一步——打通"陌生人 → 注册 → 个人 workspace → 邀请加入团队 workspace"的获客闭环。
- 非范围：计费、计划、配额、席位、Organization 层。本期只解决"能不能让陌生人自助进来"，不解决"按什么计费"。

## 0. 背景与第一性判断

### 0.1 现状：多租户隔离已落地，缺的是获客闭环

代码现状（带证据）：

- **Workspace 就是租户**，行级隔离已落在 storage 最底层。每张业务表带 `WorkspaceID`，`query_scope.go` 把所有查询包成 `workspace_id = ? AND ...`。
- **User 与 Workspace 多对多**，通过 `Membership(userID, workspaceID, role)` 关联（`internal/storage/models.go:31`），role = `owner/admin/member/viewer`。"多用户共享 workspace"的入口就是 `AddMember`（`internal/app/workspace.go:626`）。
- **资源寻址已支持多 workspace**：MCP tool 的 `workspace` 参数、CLI `--workspace`、HTTP `X-Xuanchu-Workspace` header 都有；project slug 解析强制带 workspace scope（`internal/storage/project_repo.go:132`，`WHERE workspace_id = ? AND slug = ?`）；当 token 对 >1 个 workspace 可见却只给 project slug 时，MCP 层主动抛 `CodeWorkspaceRequired`（`internal/mcpserver/auth.go:87-93`）。**寻址不需要为 SaaS 改造。**
- **身份/认证已具备企业 SSO 形态**：OIDC（`internal/app/oidc_auth.go`）、PAT/Agent/Tenant access token、browser session、directory sync、server admin token。

结论：SaaS 化的第一步**不是从 0 搭租户**，而是拆掉"admin 预置"的墙。当前系统的第一道墙是身份墙——

- OIDC 登录回调强制要求"必须是已 provisioned 的 member"才能进（`internal/app/oidc_auth.go:127`，匹配不到 sub 直接 `ErrIdentityNotFound`）。
- user/workspace 创建都是 owner/admin/server-admin 驱动，没有访客自助注册。
- 没有"邀请"流——`AddMember` 是管理员当场把已存在（或现场建）的 user 加进来，没有"发邀请、对方接受后生效"。
- **邮件发送能力为零**（`go.mod` 无任何 mail/smtp 依赖，NotificationSink 只支持 HTTP，`internal/app/notification.go:19-24`）。

### 0.2 产品形态决策（已确认）

1. **租户模型**：一人可加入多个团队 workspace（复用现有 Membership 多对多）。
2. **开放程度**：个人开放注册 + 团队邀请制。
   - 任何陌生人可注册账号 + 自动获得一个**个人 workspace**（免费、不 invite 他人）。
   - 进入**团队 workspace** 的唯一路径是接受有效邀请。陌生人注册不会自动混进任何团队。
3. **OIDC 归属**：OIDC 保持 per-workspace 可配，企业 workspace 各走各的专用 SSO；**个人 workspace 不走 OIDC**（个人用户没有 admin 去配 SSO，且全局唯一 OIDC 约束 `internal/httpapi/sso_auth.go:36-61` 不适合多租户）。
4. **个人 vs 团队区分**：新增 `Workspace.Kind` 字段（`personal`/`team`），与现有 `Visibility`（`private/team/public`，当前是死字段、`docs/superpowers/specs/2026-05-29-taskg-m4-design.md:235` 明确"只保存不消费"）正交。

### 0.3 核心设计原则：安全闸

整个 SaaS onboarding 的安全边界只有一条规则，必须贯穿所有实现：

> **开放注册的自动 provisioning 只建 `Kind=personal` workspace；进入任何 team workspace 的唯一路径是接受有效邀请。** `resolveWorkspaceForActor`（`workspace.go:961`）的 membership 强校验一行不动——这道墙就是边界，邀请制只是在这道墙之内开了一扇可控的门。

## 1. 总体架构

```
┌─────────────────────────────────────────────────────────────┐
│  公共入口（Public，无鉴权）                                    │
│  /auth/register/request   请求 magic link（提交邮箱）          │
│  /auth/register/verify    校验 magic link，完成注册/登录       │
│  /auth/invitations/accept 公开接受邀请（凭 invitation token）  │
└──────────┬──────────────────────────┬───────────────────────┘
           │ 系统级身份路径             │ 邀请路径
           ▼                          ▼
┌─────────────────────┐    ┌──────────────────────────────────┐
│ RegistrationFlow    │    │ WorkspaceInvitation（新表）        │
│ （新表，仿 Browser  │    │ pending/accepted/revoked/expired  │
│  AuthFlow）         │    │                                  │
└──────────┬──────────┘    └──────────────┬───────────────────┘
           │ verify 成功                  │ accept 成功
           ▼                              ▼
┌─────────────────────────────────────────────────────────────┐
│ Provisioning（app 层新方法，事务内组合现有 storage 原语）        │
│  • 首次：Create User + Create Workspace(Kind=personal) +     │
│          Membership(owner) + DefaultWorkspaceID              │
│  • 已有账号：跳过 provisioning                                │
│  • 邀请接受：额外 addMemberLocked(team_ws, user, role)        │
└─────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────┐
│ BrowserSession（复用现有）                                     │
│ 绑定到 effective workspace（个人或刚接受的 team）              │
└─────────────────────────────────────────────────────────────┘
```

两条路径完全隔离：
- **系统级身份路径**（magic link）与现有 per-workspace OIDC 零交集，不破坏"全局唯一 OIDC workspace"约束。
- **邀请路径**只在 team workspace 内部生效，复用现有 `addMemberLocked` 转 active membership。

## 2. 数据模型变更

### 2.1 `Workspace` 新增 `Kind` 字段

```go
// internal/storage/models.go
type Workspace struct {
    // ... 现有字段 ...
    Kind string `gorm:"not null;default:'team'"` // personal | team
}
```

- 取值：`personal`（个人 workspace）/ `team`（团队 workspace）。
- 默认 `team`：现有存量 workspace 全部按 team 处理（向后兼容）。
- `addUserLocked`（`workspace.go:186`）建个人 workspace 时设 `Kind: "personal"`。
- 新增 `normalizeWorkspaceKind(kind string) (string, error)`，仿 `normalizeWorkspaceVisibility`（`workspace.go:1005`）。
- 迁移：`migrate_sqlite.go` / `migrate_postgres.go` 的 AutoMigrate 自动加列，`default:'team'` 保证存量行。

**Kind 的消费点**（本期最小集合）：
1. 个人 workspace（`Kind=personal`）**禁止 invite 他人**——创建邀请时校验 `ws.Kind == "team"`，否则报错。
2. 个人注册 provisioning 只建 `Kind=personal`。
3. 未来计费/配额按 Kind 分流（本期不实现，但字段先就位）。

### 2.2 `RegistrationFlow` 表（个人注册凭证）

```go
// internal/storage/models.go
type RegistrationFlow struct {
    ID            string  `gorm:"primaryKey"`
    TokenHash     string  `gorm:"not null;uniqueIndex"`   // sha256 hex，仿 BrowserSession
    Email         string  `gorm:"not null;index"`         // 待注册邮箱
    Purpose       string  `gorm:"not null;default:'register'"` // register | login | invite_accept
    InviteID      *string `gorm:"index"`                  // 邀请接受场景关联的 invitation
    ExpiresAt     int64   `gorm:"not null;index"`
    ConsumedAt    *int64                                 // 一次性消费标记
    CreatedAt     int64   `gorm:"not null"`
}
```

- 复用 `BrowserAuthFlow` 的"生成 token + ExpiresAt + 一次性消费"模式（`internal/app/oidc_auth.go:59-98`、`internal/storage/session_repo.go:52-78`）。
- `TokenHash` 用 `hashHex`（`oidc_auth.go:188`）存 sha256，**不存明文**，仿 BrowserSession 的安全范式。
- TTL：`registerRegistrationFlowTTLSeconds = 600`（10 分钟），仿 `authFlowTTLSeconds`。
- `Purpose` 区分两种用途：
  - `register`：纯个人注册，verify 后建 user + personal workspace。
  - `invite_accept`：邀请接受，verify 后建/复用 user + 加入 team workspace。`InviteID` 指向对应 `WorkspaceInvitation`。

### 2.3 `WorkspaceInvitation` 表（团队邀请）

```go
// internal/storage/models.go
type WorkspaceInvitation struct {
    ID              string  `gorm:"primaryKey"`
    WorkspaceID     string  `gorm:"not null;index"`
    InviteeEmail    *string `gorm:"index"`                 // 定向邮箱邀请（可空）
    InviteeUserID   *string `gorm:"index"`                 // 定向已有用户邀请（可空）
    Role            string  `gorm:"not null;default:'member'"` // member | admin（owner 不允许邀请）
    TokenHash       string  `gorm:"not null;uniqueIndex"`  // sha256 hex
    Status          string  `gorm:"not null;default:'pending';index"` // pending|accepted|revoked|expired
    InvitedByUserID string  `gorm:"not null"`
    ExpiresAt       int64   `gorm:"not null;index"`
    AcceptedAt      *int64
    AcceptedUserID  *string                                 // 实际接受者，落审计
    CreatedAt       int64   `gorm:"not null"`
    ModifiedAt      int64   `gorm:"not null"`
}
```

- **为什么独立表，不复用 Membership + pending**（事实依据）：
  - "memberships 有行 = 已加入"是全局契约，`member_repo.go:22` 的 `Get` 被 9 个鉴权调用点依赖（`runtime.go:106`、`request_scope.go:145`、`oidc_auth.go:128`、`workspace.go:961`、通知/提及校验等）。加 `status` 过滤漏一处即越权。
  - Membership 主键 `(UserID, WorkspaceID)` 两者非空，**表达不了"邀请一个还没注册的邮箱"**。
  - `directory_sync.go` 的差集删除会被 pending 行污染。
  - 独立表：Membership 与所有鉴权路径**零改动**，Accept 时调现成 `addMemberLocked`（`workspace.go:677`）转 active membership。
- `TokenHash` 存 hash（`hashHex`），与 `RegistrationFlow` 一致。
- 约束：`InviteeEmail` 与 `InviteeUserID` 至少一个非空。
- 角色限制：只允许 `member`/`admin`；`owner` 邀请必须走 `AddMember`（带 last-owner 保护）。

### 2.4 身份存储：邮箱作为身份键

- 注册邮箱落 `User.Email`（`uniqueIndex`，`models.go:25`），**已足够**，无需新增字段。
- 可选增强：同时在 `UserExternalID` 写一行 `provider="local_email", user_type="email", external_id=email`，复用现有 external_id 统一身份抽象与绑定/解绑链路（`internal/httpapi/users.go:141`）。**本期采用此增强**，保证所有身份来源收敛到同一张表，未来加 SSO 绑定时不需特殊处理本地账号。
- `User.Name` 唯一（`uniqueIndex`），首次注册无可靠用户名，**用邮箱前缀 + 随机后缀兜底**（仿 `directory_sync.go:133` 的 `uniqueUserName`）。

## 3. 系统级身份路径：Magic Link 注册

### 3.1 配置（`ConfigScopeServer`）

系统级注册配置存在 `Config(ConfigScopeServer)`（`config_repo.go:14,174`，全局、无 workspace 归属）。新增 key（仿 `OIDCConfigService` 写一个 `RegistrationConfigService`）：

| Key | 说明 | 默认 |
|---|---|---|
| `auth.register.enabled` | 个人开放注册总开关 | `false`（默认关闭，显式开启） |
| `auth.register.smtp.host` | SMTP/API host | 空 |
| `auth.register.smtp.port` | 端口 | `587` |
| `auth.register.smtp.username` | 用户名 | 空 |
| `auth.register.smtp.password` | 密码（`EncryptConfigSecret` 加密，复用 `[security].config_secret_key`） | 空 |
| `auth.register.smtp.from` | 发件地址 | 空 |
| `auth.register.link_base_url` | magic link 跳转基础 URL（如 `https://app.example.com`） | 空 |

- **为什么用 `ConfigScopeServer`**：它是现成的全局 scope（`normalizeConfigKey` 对 `server` scope 清空 workspace_id/scope_id），适合系统级开关。目前无业务先例，本期是首个使用者——需自行写 Service 层读写，仿 `OIDCConfigService` 但 `wk` 换成 server scope。
- secret 字段复用 `EncryptConfigSecret`（`config_secret.go`）。

### 3.2 Magic Link 流程

**请求 link**（`POST /auth/register/request`，Public）：
1. 入参：`{ email }`。
2. 校验 `auth.register.enabled`，关闭则 404/409。
3. 归一化 email；查 `User.Email` 是否已存在：
   - 已存在：**仍发 link**（避免账号枚举），但 `Purpose=login`（verify 后不发 provisioning，直接建 session）。
   - 不存在：`Purpose=register`。
4. 生成 `rawToken := randomToken(32)`，`tokenHash := hashHex(rawToken)`，写 `RegistrationFlow{TokenHash, Email, Purpose, ExpiresAt=now+600}`。
5. 调邮件发送器（见 3.3）发 magic link：`{link_base_url}/auth/register/verify?token={rawToken}`。
6. 始终返回 202（不泄露邮箱是否存在）。

**校验 link**（`GET /auth/register/verify`，Public）：
1. 入参：`?token=`。
2. `hashHex(token)` → 查 `RegistrationFlow`；命中且未过期且未消费。
3. 标记 `ConsumedAt=now`（一次性）。
4. 按 `Purpose` 分流：
   - `register`：事务内调 `provisionPersonalAccount(email)`（见 3.4）→ 拿到 user + personal workspace。
   - `login`：查 user，校验 user 仍有效。
   - `invite_accept`：事务内调 `acceptInvitationByFlow(flow)`（见 4.3）。
5. 建 `BrowserSession`（复用 `sessionRepo.CreateSession`，`session_repo.go:17`），绑定到 effective workspace：
   - `register`：绑定到刚建的个人 workspace（= user.DefaultWorkspaceID）。
   - `login`：绑定到 user.DefaultWorkspaceID（已是个人 workspace，或用户曾切换过的 active workspace）。
   - `invite_accept`：绑定到邀请对应的 team workspace（用户主动接受的，落 team）。
6. 302 到 Web Console，set session + CSRF cookie（仿 `sso_auth.go:166`）。

### 3.3 邮件发送器（从零搭，邮件 API 路径）

- 新增 `internal/app/mailer.go`，定义接口：
  ```go
  type Mailer interface {
      SendMagicLink(ctx context.Context, to, rawToken string) error
      SendInvitation(ctx context.Context, to string, inv WorkspaceInvitation) error
  }
  ```
- 实现：`httpMailer`，用 HTTP POST 调第三方邮件 API（SendGrid/SES/Resend 之一，HTTP POST，零 CGO，贴合现有 NotificationSink 的 HTTP 调用习惯）。
  - 配置从 `ConfigScopeServer` 的 smtp/API key 读取。
  - **不走 SMTP 标准库**（`net/smtp`），避免引入额外复杂度；统一 HTTP API。
- 开发环境：`logMailer`（把 magic link 打到 stderr/日志），不依赖外部服务，保证本期可独立跑通注册闭环的端到端测试。
- 接口注入到 `Service`/auth handler，测试用 mock。

### 3.4 `provisionPersonalAccount(email)`（app 层新方法）

事务内组合现有 storage 原语（**不直接复用 `addUserLocked`**——它是 unexported 且依赖 `s.runtime.ActorUserID`，而注册时 actor 还不存在）：

```go
func (s *Service) provisionPersonalAccount(email string) (storage.User, storage.Workspace, error) {
    // store.DB().Transaction 内：
    // 1. userRepo.Create(User{ID: uuid, Email: &email, Name: uniqueUserName(email), DisplayName: emailLocalPart})
    // 2. workspaceRepo.Create(Workspace{ID: uuid, Slug: uniqueSlug(email), Kind: "personal", Visibility: "private", CreatedByUserID: &user.ID})
    // 3. memberRepo.Upsert(Membership{UserID: user.ID, WorkspaceID: ws.ID, Role: RoleOwner, JoinedAt: now})
    // 4. userRepo.UpdateDefaultWorkspace(user.ID, ws.ID)
    // 5. extRepo.Create(UserExternalID{Provider: "local_email", UserType: "email", ExternalID: email, UserID: user.ID})
}
```

- 参考模板：`directory_sync.go:57-87`（建 user + 写 ext + 写 membership 的现成组合），再补上 `addUserLocked`（`workspace.go:186-231`）的 workspace + default workspace 三步。
- `CreatedByUserID` 设为新 user 自己的 ID（自注册自建），或 nil。
- 与现有 `addUserLocked` 的差异：不依赖 runtime actor、不产 actor-driven 审计（改为系统审计）、`Kind=personal`。
- 并发安全：`User.Email` 的 `uniqueIndex` 保证同一邮箱只 provisioning 一次，事务内 `GetByEmail` 失败再 create（防竞态）。

## 4. 团队邀请路径

### 4.1 创建邀请（`team workspace` 的 owner/admin 发起）

新增 app 方法 `CreateInvitation`（仿 `AddMember` `workspace.go:626` 的编排模式）：
1. `resolveWorkspaceForActor(input.WorkspaceRef)`（复用 `workspace.go:946`）。
2. **校验 `ws.Kind == "team"`**（新 Kind 字段的第一个消费点）——personal workspace 报错 `invitation_not_allowed_for_personal_workspace`。
3. 归档 workspace 拒绝。
4. `normalizeRole(input.Role)`，限制 `member`/`admin`；`requireMemberManagement`（复用 `workspace.go:978`）校验当前 actor 有权。
5. 互斥校验：`InviteeEmail` / `InviteeUserID` 二选一（可支持定向用户邀请）。
6. 生成 `rawToken` + `tokenHash`，写 `WorkspaceInvitation{Status: "pending", ExpiresAt: now+defaultInvitationTTL}`。
7. 可选：若 `InviteeEmail` 非空，调 Mailer 发邀请邮件。
8. `withAudit("invitation.create", ...)`。

### 4.2 邀请的生命周期状态机

```
pending ──accept──▶ accepted（终态）
   │
   ├──revoke──▶ revoked（终态）
   │
   └──expire(time)──▶ expired（终态，懒标记）
```

- `accept`：校验 token 未过期、status=pending、（若 InviteeEmail）邮箱匹配。
- `revoke`：inviter 或更高权限者发起。
- `expire`：查询时惰性标记（`ExpiresAt < now` 且 status=pending → 视为 expired），不写后台任务。

### 4.3 接受邀请（`/auth/invitations/accept`，Public）

这是唯一需要新建身份逻辑的入口。两种接受者状态：

**路径 A：接受者已有账号**（凭现有 session 或重新 magic-link 登录后接受）：
1. 校验 invitation token → status=pending。
2. 调 `addMemberLocked(ws.ID, user.ID, role)`（现成，`workspace.go:677`，已带"已是成员"检查）。
3. invitation 标 `accepted` + `AcceptedUserID` + `AcceptedAt`。
4. 建/续 session，绑定到该 team workspace。

**路径 B：接受者没账号**（邀请邮件里的链接，需先注册）：
1. 校验 invitation token → status=pending。
2. **不直接建 user**——改为写一个 `RegistrationFlow{Purpose: "invite_accept", InviteID: &inv.ID, Email: inv.InviteeEmail}`，生成 magic link 发给邀请邮箱。
3. 用户点 magic link 走 `verify`（3.2 的 `invite_accept` 分支）：
   - 事务内 `provisionPersonalAccount(email)` 建个人 workspace（**符合安全闸：注册即得个人 workspace**）。
   - 紧接着 `addMemberLocked(team_ws, user.ID, inv.Role)` 加入团队。
   - invitation 标 `accepted`。
4. session 绑定到该 team workspace（用户主动接受的，落 team）。

**为什么邀请接受要走 magic link 而非直接建 user**：
- 保证邮箱真实性（邀请可能被转发，接受者必须能收该邮箱）。
- 复用 3.2 的 verify 流程，不重复造身份验证。
- 邀请 token 与注册 token 职责分离：invitation token 证明"有权加入"，registration flow token 证明"邮箱真实"。

### 4.4 邀请权限与防越权

- 创建邀请：当前 actor 必须是 team workspace 的 owner/admin（`requireMemberManagement`，复用 `workspace.go:978`）。
- 邀请角色不能高于邀请者角色。
- owner 邀请**不开放**（必须走 `AddMember`，带 last-owner 保护 `workspace.go:720-728`）。
- 撤销邀请：inviter 本人或更高权限者。

## 5. 现有代码改动清单（最小化）

### 5.1 storage 层

| 改动 | 文件 | 说明 |
|---|---|---|
| `Workspace.Kind` 字段 | `models.go` + migrate | 见 2.1 |
| `RegistrationFlow` model | `models.go` + migrate | 见 2.2 |
| `WorkspaceInvitation` model | `models.go` + migrate | 见 2.3 |
| `registration_flow_repo.go` | 新建 | 仿 `session_repo.go` 的 AuthFlow 部分：Create/GetByTokenHash/Consume/Delete/PurgeExpired |
| `invitation_repo.go` | 新建 | 仿 `member_repo.go`：Create/GetByTokenHash/GetByID/ListByWorkspace/ListByInvitee/UpdateStatus/Delete |
| migrate 登记 | `migrate_sqlite.go` / `migrate_postgres.go` AutoMigrate 列表加新 model | 存量 workspace 默认 Kind=team |

### 5.2 app 层

| 改动 | 文件 | 说明 |
|---|---|---|
| `provisionPersonalAccount` | `workspace.go`（或新 `registration.go`） | 见 3.4 |
| `normalizeWorkspaceKind` | `workspace.go` | 仿 `normalizeWorkspaceVisibility` |
| `addUserLocked` 建个人 ws 时设 `Kind:"personal"` | `workspace.go:207` | 一行改动 |
| `RegistrationConfigService` | 新 `registration_config.go` | 仿 `OIDCConfigService`，server scope |
| `Mailer` 接口 + `httpMailer`/`logMailer` | 新 `mailer.go` | 见 3.3 |
| `CreateInvitation`/`AcceptInvitation`/`RevokeInvitation`/`ListInvitations` | 新 `invitation.go` | 见第 4 节 |
| `RequestMagicLink`/`VerifyMagicLink` | 新 `registration.go` | 见 3.2 |
| **不动** | `member_repo.go`、所有鉴权路径 | Membership 契约不变 |

### 5.3 httpapi 层

| 改动 | 文件 | 说明 |
|---|---|---|
| 注册/邀请 handler | 新 `auth_register.go` / `invitations.go` | 仿 `sso_auth.go` |
| 公共路由登记 | `huma_routes.go` | 加 `Public: true` 行：`/auth/register/request`、`/auth/register/verify`、`/auth/invitations/accept` |
| 鉴权路由登记 | `huma_routes.go` | `POST /api/v1/workspaces/{ref}/invitations`、`GET .../invitations`、`POST .../invitations/{id}/revoke`（需鉴权） |
| 系统级配置端点 | 仿 `sso.go` | `GET/PUT /api/v1/admin/registration-config`（server admin 权限） |

### 5.4 CLI / MCP（本期最小集，不阻塞主流程）

- CLI：`xuanchu invitation create/list/revoke`（仿 `cli/member.go`）。
- MCP：`workspace_invitation_create` / `workspace_invitation_list` / `workspace_invitation_revoke`（命名遵循 `资源_动作` 规范，`AGENTS.md` MCP 命名规范）。
- 注册本身**不暴露 CLI/MCP**（注册是浏览器交互，CLI/MCP 面向已认证用户）。
- 以上 CLI/MCP 命令本期可后置到后续小版本，主流程（注册闭环 + 邀请闭环）不依赖它们——邀请创建/撤销也可先经 Web Console 或 HTTP API 完成。

## 6. 安全考量

1. **账号枚举防护**：`/auth/register/request` 无论邮箱是否已注册，都返回 202 并发 link（已注册发 login link，未注册发 register link）。
2. **Token 安全**：所有 token（magic link、invitation）存 sha256 hash（`hashHex`），constant-time 比对（仿 `VerifyTokenHash` `token.go:93`），一次性消费。
3. **安全闸**（重申）：开放注册 provisioning 只建 `Kind=personal`；进入 team workspace 唯一路径是有效邀请 + `addMemberLocked`。`resolveWorkspaceForActor` 的 membership 校验不动。
4. **邀请 token 不可枚举**：32 字节随机，未接受前不暴露 invitation ID 给被邀请人（邮件只带 token）。
5. **邮箱撞库**：`User.Email` `uniqueIndex` 保证唯一；provisioning 事务内 `GetByEmail` 防竞态。
6. **Rate limit**（后置，不阻塞本期交付）：`/auth/register/request` 按 IP + email 限频，防滥用。本期不实现，在后续安全加固版本补；部署文档需提示运维侧（反向代理层）做基础限频。
7. **注册开关默认关闭**：`auth.register.enabled` 默认 `false`，部署方显式开启，避免意外开放。

## 7. 不做的事（YAGNI）

- **Organization 层**：本期不引入。个人 workspace 直接归 user，团队 workspace 归 team，计费/席位留到后续 milestone。引入 Org 是结构性大改，超出"一步"范围。
- **计费/计划/配额/席位**：本期不做。`Kind` 字段先就位，消费逻辑后续接。
- **密码登录**：magic link 已满足零门槛获客，密码需额外哈希存储与找回流程，YAGNI。
- **社交 SSO（Google 等）**：现有 OIDC per-workspace 架构可扩展，但本期不接。个人注册统一走 magic link。
- **公开 workspace discovery**：`Visibility` 的 `public` 语义本期不消费（与现状一致）。
- **邮件 webhook/退订**：本期邮件只发 magic link/邀请，不做接收处理。

## 8. 验收标准

- [ ] 关闭注册开关时，`/auth/register/*` 返回 404/409，且不影响现有 OIDC 登录。
- [ ] 开启后，陌生邮箱请求 magic link → 收到 link（logMailer 可见）→ verify 后建 user + `Kind=personal` workspace + owner membership + DefaultWorkspaceID + local_email external id。
- [ ] 已注册邮箱请求 link → 收到 login link → verify 后不发 provisioning，直接建 session。
- [ ] team workspace owner 可创建邀请；personal workspace 创建邀请被拒。
- [ ] 无账号者接受邀请 → magic link → 注册 + 加入 team workspace（双重效果），invitation 标 accepted。
- [ ] 有账号者接受邀请 → 直接 addMemberLocked，不重复建个人 workspace。
- [ ] 过期/撤销/已接受的 invitation token 无法再 accept。
- [ ] 现有鉴权路径全部回归通过：`member_repo.Get` 的 9 个调用点行为不变；OIDC 登录、PAT/Agent/Tenant token、CLI 单机模式不受影响。
- [ ] `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 全绿。
- [ ] 集成测试覆盖：注册闭环、邀请闭环、安全闸（注册不混入 team）。

## 9. 对后续 milestone 的衔接

- **计费/席位**：`Workspace.Kind` 已就位，team workspace 计席位、personal 免费可直接消费此字段。
- **Organization 层**（若后续需要）：在 user 与 team workspace 之间引入 Org，`Workspace` 加 `OrgID` 外键，不破坏本期 Kind 语义。
- **社交 SSO**：个人注册路径若后续要加 Google 登录，可在 `RegistrationFlow` 之外新增 OIDC 全局路径，复用 `provisionPersonalAccount`。
- **邮件通道扩展**：`Mailer` 接口可扩展为通知通道之一，未来可考虑接入 NotificationSink 体系。

## 10. 关键代码证据索引

| 结论 | 证据 |
|---|---|
| 行级隔离在 storage 层 | `internal/storage/query_scope.go:52` |
| Membership 多对多 + role | `internal/storage/models.go:31`，`internal/authz/model.go:10` |
| AddMember 入口 | `internal/app/workspace.go:626` |
| addMemberLocked（Accept 复用） | `internal/app/workspace.go:677` |
| OIDC 强制 member | `internal/app/oidc_auth.go:127` |
| addUserLocked 建私人 ws | `internal/app/workspace.go:186`，硬编码 `Visibility:"private"` 在 `:207` |
| addUserOnlyLocked（仅建 user） | `internal/app/workspace.go:171` |
| requireMemberManagement | `internal/app/workspace.go:978` |
| resolveWorkspaceForActor（安全闸） | `internal/app/workspace.go:961` |
| BrowserAuthFlow（magic link 模板） | `internal/app/oidc_auth.go:59-98`，`internal/storage/session_repo.go:52-78` |
| hashHex（token 哈希） | `internal/app/oidc_auth.go:188` |
| directory_sync 建用户模板 | `internal/app/directory_sync.go:57-87` |
| uniqueUserName 兜底 | `internal/app/directory_sync.go:133` |
| ConfigScopeServer（系统级配置） | `internal/storage/config_repo.go:14,174` |
| EncryptConfigSecret | `internal/app/config_secret.go` |
| Public 路由登记 | `internal/httpapi/huma_routes.go:14-23,95-101,568-574` |
| OIDCConfigService（配置 Service 模板） | `internal/app/oidc_config.go:70` |
| Visibility 是死字段 | `docs/superpowers/specs/2026-05-29-taskg-m4-design.md:235` |
| 邮件发送为零 | `go.mod` 无 mail 依赖，`internal/app/notification.go:19-24` 只支持 HTTP |
| member Get 的 9 个鉴权调用点 | `runtime.go:106`、`request_scope.go:145,158`、`workspace.go:961`、`oidc_auth.go:128`、`event_notification.go:641`、`notification.go:1129`、`service.go:1789`、`admin_workspace.go:198`、`admin_bootstrap.go:180,272` |
| 全局唯一 OIDC workspace 约束 | `internal/httpapi/sso_auth.go:36-61` |
