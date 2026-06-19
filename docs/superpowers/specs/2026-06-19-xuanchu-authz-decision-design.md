# Xuanchu 授权决策层重构设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-06-19
**状态：** 草案
**背景需求：** 当前用户、workspace、token、impersonation 的底层模型基本正确，但授权链路分散在 HTTP、MCP、App service 和 token 模块中。需要先通过内部架构重构提高可理解性，再为后续错误语义整理、浏览器 SSO、飞书身份绑定和企业成员导入保留干净边界。

---

## 1. 背景与现状

### 1.1 当前模型

Xuanchu 当前已经形成一套本地授权模型：

- `users`：本地用户主体，包含 `id/name/email/default_workspace_id`。
- `memberships`：用户在 workspace 内的角色，角色为 `owner/admin/member/viewer`。
- `api_tokens`：普通 API 凭证，分为 PAT 和 Agent token，保存 hash、prefix、scope、workspace allowlist、project allowlist。
- `server_admin_tokens`：服务端控制面 bootstrap token，独立于普通 `api_tokens`。
- `user_external_ids`：外部系统标识绑定，例如飞书 `feishu_user_id`，用于关联而不是登录。
- `audit_logs`：记录 actor，并在 impersonation 下记录 delegator。

当前最终权限规则是：

```text
effective permission =
  membership role 权限
  ∩ token capability scope
  ∩ token workspace allowlist
  ∩ token project allowlist
```

这个规则是正确的，应继续保留。

### 1.2 当前实现分散点

授权相关逻辑主要散落在以下路径：

- `internal/app/runtime.go`
  - 解析本地 CLI / stdio MCP 的 active user、active workspace、membership role。
- `internal/app/token.go`
  - 创建、修改、验证 PAT / Agent token。
  - 验证 raw token hash、revoked、expired、last_used_at。
- `internal/app/request_scope.go`
  - 解析 HTTP token request 的 workspace/project scope。
  - 处理 `X-Xuanchu-As` impersonation。
  - 组合 role permission、token capability、workspace/project allowlist。
- `internal/app/workspace.go`
  - 定义 role permission matrix。
- `internal/httpapi/middleware.go`
  - HTTP Bearer token middleware，计算 visible/effective workspace。
- `internal/httpapi/app_service.go`
  - 每个 HTTP handler 构造 scoped service。
- `internal/mcpserver/auth.go`
  - HTTP MCP / stdio MCP 分别构造 service。

这些逻辑行为上能工作，但概念边界不够集中。后续如果直接接入浏览器 OIDC、飞书 OAuth 或企业成员同步，很容易在现有链路旁边再长出一套平行授权逻辑。

### 1.3 需要解决的问题

1. **概念不够显式**：代码里有 actor、delegator、token user、subject user、effective workspace，但没有统一的结构表达。
2. **授权链路需要跨文件追踪**：理解一次 HTTP 请求如何变成 app runtime，需要跳过多个模块。
3. **scope 与 permission 映射分散**：token scope 是 `task:read`，app permission 是 `task.read`，映射靠调用点约定。
4. **错误边界不够集中**：`permission_denied`、`token_scope_denied`、`workspace_scope_denied`、`membership_not_found` 等错误由多个位置返回，长期容易漂移。
5. **未来浏览器登录缺少落点**：Web Console 当前使用 Bearer token + `sessionStorage`，未来 browser session 应接入同一授权决策，而不是复用机器 token 或复制逻辑。

## 2. 核心判断

### 2.1 不重写用户体系

本次不改变 `users / memberships / api_tokens / user_external_ids / server_admin_tokens` 的底层模型。它们的职责划分是合理的：

- User 是本地业务主体。
- Membership 决定用户在 workspace 中能做什么。
- Token 是凭证和能力收窄工具。
- External ID 是外部身份绑定，不是登录态。
- Server admin token 是控制面凭证，不是业务用户。

### 2.2 先重构授权表达

最值得做的是新增一个小而明确的授权决策层，让 HTTP、MCP、Remote CLI 后续都围绕同一套概念说话。

建议新增包：

```text
internal/authz
```

该包只表达“授权决策”本身，不直接持有 HTTP、MCP、Cobra、GORM 细节。数据读取仍通过 app/storage 现有 repository 或由 app 层组装输入。

### 2.3 分阶段实施

本 spec 覆盖两个目标，但实施必须分阶段：

1. **Phase 1：内部结构整理**
   - 抽出 Authorization Decision 概念。
   - 保持外部行为、错误码、表结构、API、token 格式不变。
   - 用测试证明行为等价。

2. **Phase 2：错误语义整理**
   - 在 Phase 1 稳定后，再集中整理错误分类和映射。
   - 允许改进错误信息，但默认不破坏现有错误码，除非有明确兼容策略。

## 3. 目标

1. 建立统一的授权概念模型：Credential、Principal、Delegator、TenantScope、Capability、Permission、Decision。
2. 将 HTTP/MCP 的授权流程收敛到同一个授权决策入口，减少重复分支。
3. 保持现有行为等价：角色权限、token scope、workspace/project allowlist、impersonation 结果不变。
4. 为未来浏览器 session 接入预留 Credential 类型，不实现 SSO。
5. 为后续错误语义整理建立集中出口。
6. 不破坏 `CGO_ENABLED=0` 测试和构建要求。

## 4. 非目标

- 不新增数据库表或 migration。
- 不引入 policy engine、OPA、Casbin 或复杂 DSL。
- 不接入 OIDC、OAuth、飞书登录、cookie session。
- 不改变 PAT / Agent / Server Admin token 格式。
- 不改变 HTTP API 路径、MCP tool name 或 CLI 参数。
- 不引入 organization / group / SCIM。
- 不把 server admin token 变成业务用户或超级 membership。

## 5. 概念模型

### 5.1 Credential

Credential 表示“请求带来的凭证”，而不是业务用户。

```go
type CredentialKind string

const (
    CredentialPAT            CredentialKind = "pat"
    CredentialAgent          CredentialKind = "agent"
    CredentialServerAdmin    CredentialKind = "server_admin"
    CredentialBrowserSession CredentialKind = "browser_session" // 预留，不在本次实现
)

type Credential struct {
    Kind         CredentialKind
    TokenID      string
    TokenUserID  string
    Capabilities []string
    WorkspaceIDs []string
    ProjectIDs   []string
}
```

规则：

- PAT / Agent 对应 `api_tokens`。
- Server Admin 对应 `server_admin_tokens`，只进入 admin 控制面，不进入普通业务授权。
- Browser Session 只预留类型，后续 SSO/OIDC 使用，不在本次实现。

### 5.2 Principal

Principal 表示“本次业务操作的名义 actor”。

```go
type Principal struct {
    UserID   string
    UserName string
}
```

普通请求：

```text
principal = credential.token_user
```

Impersonation 请求：

```text
principal = X-Xuanchu-As 解析出的 subject user
delegator = credential.token_user + credential.token_id
```

### 5.3 Delegator

Delegator 表示“实际持有凭证并发起 impersonation 的一方”。

```go
type Delegator struct {
    UserID  string
    TokenID string
}
```

只有 impersonation 请求才有 delegator。Delegator 只用于审计和日志，不参与业务权限放大。

### 5.4 TenantScope

TenantScope 表示请求最终落在哪个 workspace / project 范围内。

```go
type TenantScope struct {
    WorkspaceID   string
    WorkspaceSlug string
    ProjectID     *string
}
```

规则：

- workspace 是当前实际租户边界。
- project 是 workspace 内的可选细分边界。
- token allowlist 为空表示不按该维度收窄。
- token project allowlist 非空时，无 project 的任务对该 token 不可写，读侧按现有行为继续隐藏。

### 5.5 Role Permission

Role permission matrix 继续来自现有 `owner/admin/member/viewer` 规则。建议从 `internal/app/workspace.go` 搬到 `internal/authz/policy.go` 或由 app 层包装，但不改变矩阵内容。

角色含义保持：

| Role | 含义 |
|---|---|
| `owner` | workspace 内最高业务权限 |
| `admin` | workspace 管理员，但不拥有 owner 级成员管理能力 |
| `member` | 普通协作成员，能读写任务，不能管理 workspace/project 核心结构 |
| `viewer` | 只读 |

### 5.6 Capability 与 Permission

Capability 是 token scope，格式 `resource:action`，例如 `task:read`。

Permission 是 app 内部权限，格式 `resource.action`，例如 `task.read`。

本次建议新增集中映射表：

```go
type Requirement struct {
    Capability string
    Permission app.Permission
}
```

每个 HTTP endpoint、MCP tool、remote operation 仍声明自己需要的 Requirement，但授权入口统一消费这个 Requirement。

### 5.7 Authorization Decision

Authorization Decision 是本次重构的核心输出。

```go
type Decision struct {
    Principal Principal
    Delegator *Delegator
    Credential Credential
    Tenant TenantScope
    Role app.Role
    RequestScope RequestScope
}
```

Decision 必须能直接构造现有 `app.RuntimeContext`：

```go
RuntimeContext{
    ActorUserID:      Decision.Principal.UserID,
    ActorName:        Decision.Principal.UserName,
    WorkspaceID:      Decision.Tenant.WorkspaceID,
    WorkspaceSlug:    Decision.Tenant.WorkspaceSlug,
    Role:             Decision.Role,
    DelegatorTokenID: Decision.Delegator.TokenID,
    DelegatorUserID:  Decision.Delegator.UserID,
}
```

## 6. 目标架构

### 6.1 包结构

建议新增：

```text
internal/authz/
  model.go          // Credential / Principal / Delegator / TenantScope / Decision
  requirement.go    // Capability + Permission requirement
  policy.go         // role permission matrix
  errors.go         // authz error code 分类，Phase 2 才收敛错误码
```

Phase 1 先不要求 `authz` 包直接读数据库。由 `internal/app` 继续负责 repository 查询，`authz` 负责纯规则判断和结构表达。

可以先保留 `internal/app/request_scope.go` 作为编排层，但把其中概念输出改为 `authz.Decision`。

### 6.2 App 层职责

App 层继续负责：

- 通过 token 解析 credential。
- 解析 user/workspace/project。
- 查询 membership。
- 调用 authz policy 判断 role permission。
- 将 Decision 转换为 `ServiceOptions.Runtime` 和 `RequestScope`。

目标是让 `AuthorizeTokenRequest` 从“混合查询、判断、构造 runtime 的大函数”逐步变成：

```text
1. BuildCredential
2. ResolveTenant
3. ResolvePrincipal
4. CheckAuthorization
5. BuildDecision
6. NewScopedService(decision)
```

### 6.3 HTTP 层职责

HTTP 层只负责：

- 从 `Authorization` header 读取 raw token。
- 从 query/header 读取 workspace/project。
- 从 `X-Xuanchu-As` 读取 subject user ref。
- 调用 app 授权入口得到 Decision。
- 用 Decision 构造 scoped service。
- 把 Decision 中的 actor/token/workspace/delegator 写入 access log state。

HTTP handler 不应自己解释 role、scope 或 allowlist。

### 6.4 MCP 层职责

HTTP MCP 复用 HTTP Bearer token 和 `X-Xuanchu-As` 语义。MCP RuntimeFactory 只负责将 tool input 中的 workspace/project ref 传给同一个授权入口。

stdio MCP 继续沿用本地 runtime 解析，不支持 impersonation。后续如果要支持，也必须先设计新的本地凭证或交互模型。

### 6.5 Server Admin 控制面

Server Admin token 不进入普通 authz decision。它应保持独立中间件：

```text
admin token -> adminAuthMiddleware -> /api/v1/admin/*
```

原因：

- Server Admin 是部署/控制面 bootstrap 能力。
- 它不代表某个 workspace membership。
- 它不能访问普通 task/workspace/token API。

Phase 2 可以给 admin 控制面也定义一个轻量 `AdminDecision`，但不能混入普通业务 Decision。

## 7. 授权流程

### 7.1 普通 Bearer token 请求

```text
Authorization: Bearer xuanchu_pat_...
```

流程：

1. 验证 token prefix/hash。
2. 检查 revoked/expired。
3. 读取 token user。
4. 构造 Credential。
5. 根据显式 workspace/project 或 token/default workspace 解析 TenantScope。
6. 验证 token workspace/project allowlist。
7. principal = token user。
8. 查询 principal 在 workspace 的 membership。
9. 验证 role permission。
10. 验证 token capability。
11. 输出 Decision。

### 7.2 Agent impersonation 请求

```text
Authorization: Bearer xuanchu_agent_...
X-Xuanchu-As: alice
```

流程：

1. 先按 token 自身解析 Credential 和 TenantScope。
2. 要求 Credential.Kind 是 Agent。
3. 要求 capabilities 包含 `impersonate`。
4. 若 token 可见多个 workspace 且请求没有显式 workspace/project，返回 `workspace_required`。
5. 解析 subject user。
6. 验证 subject user 是 TenantScope.workspace 的成员。
7. principal = subject user。
8. delegator = token user + token id。
9. 使用 subject membership role 验证 app permission。
10. 使用 token capabilities / allowlist 验证 capability 和范围。
11. 输出 Decision。

### 7.3 本地 CLI / stdio MCP

本地模式没有 Bearer token。流程保持：

1. 解析 explicit `--user` / active user / `local`。
2. 解析 explicit `--workspace` / active workspace / default workspace / `local`。
3. 查询 membership。
4. 构造本地 Decision 或 RuntimeContext。

Phase 1 可以先不强制本地模式走完整 Credential 模型，但建议使用同一个 role permission policy，避免 role matrix 分叉。

## 8. 错误语义整理

Phase 1 不改错误码，避免同时重构结构和行为。

Phase 2 再集中整理错误边界。建议先冻结以下分类：

| 错误码 | 语义 |
|---|---|
| `auth_missing_token` | 缺少 Bearer token |
| `auth_invalid_token` | token 不存在或 hash 不匹配 |
| `auth_token_revoked` | token 已吊销 |
| `auth_token_expired` | token 已过期 |
| `token_scope_denied` | token capability 不允许本操作，或 PAT/Agent 类型不满足 impersonation 要求 |
| `workspace_scope_denied` | token workspace allowlist 不允许 |
| `project_scope_denied` | token project allowlist 不允许 |
| `membership_not_found` | principal 不是 workspace 成员，或 impersonation subject 不可用 |
| `permission_denied` | membership role 不允许本操作 |
| `workspace_required` | 多 workspace 可见场景下无法安全推断 workspace |
| `workspace_archived` | workspace 已归档 |
| `project_not_found` | project 不存在或不属于请求 workspace |

整理原则：

- 不把“目标不存在”和“无权限”随意混用，除非已有兼容要求必须隐藏资源。
- 对 impersonation target，继续保持不存在或非成员统一返回 `membership_not_found`，避免暴露过多身份枚举信息。
- 对 project-scope 下不可见任务，读侧可以继续返回 `task_not_found` 以保持现有隐藏语义。
- HTTP status 映射集中维护，不散落在 handler。

## 9. 与未来 SSO / 飞书的边界

本次不做 SSO，但必须为未来留好边界：

- OIDC / 飞书 OAuth 只负责认证和外部身份映射。
- 登录成功后映射到本地 `users.id`。
- 浏览器态使用服务端可撤销 opaque session cookie。
- Browser session 未来作为 `CredentialBrowserSession` 输入 authz decision。
- 权限仍由本地 membership role 决定。
- 机器集成继续使用 PAT / Agent token，不复用浏览器 session。
- 飞书稳定绑定使用 `provider=feishu_user_id`，`external_id=<Feishu user_id>`。
- 飞书 `open_id` 只适合作为消息可达性信息，不作为稳定主体。

## 10. 迁移计划

### Phase 1：等价重构

1. 新增 `internal/authz` 纯模型和 policy。
2. 将 role permission matrix 从 app 层迁入或包装进 authz。
3. 在 app 层新增 Decision 构造函数。
4. 改造 `AuthorizeTokenRequest`，保持函数签名和返回语义尽量不变。
5. HTTP `scopedServiceFor` 改为消费 Decision。
6. MCP HTTP RuntimeFactory 改为消费同一授权入口。
7. 本地 CLI / stdio MCP 先复用 authz role policy，不强行引入假 Credential。
8. 补测试证明现有用例行为不变。

### Phase 2：错误边界整理

1. 梳理当前 auth/request scope 相关错误码。
2. 在 authz 定义错误分类和 HTTP status 映射。
3. 将错误构造收敛到少数 helper。
4. 更新测试 golden 和文档。
5. 如需改变错误码，必须在 plan 中列出兼容影响。

### Phase 3：未来浏览器认证接入（只预留）

本 spec 不进入实现。后续单独写 spec：

- OIDC / 飞书 Auth Broker assertion 接入。
- server-side browser sessions。
- CSRF 和 logout/revoke。
- 用户导入或 JIT provisioning 策略。

## 11. 测试策略

### 11.1 必须保持的回归测试

- PAT 正常访问 `/api/v1/me`、tasks、projects。
- Agent token workspace allowlist 生效。
- Agent token project allowlist 生效。
- 无 scope 返回 `token_scope_denied`。
- role 不足返回 `permission_denied`。
- impersonation 需要 Agent token + `impersonate` scope。
- impersonation target 不存在或非成员返回 `membership_not_found`。
- token 可见多个 workspace 且 impersonation 未显式 workspace 返回 `workspace_required`。
- audit log 记录 actor 和 delegator。
- HTTP MCP 与 HTTP API 行为一致。
- stdio MCP 继续走本地 active user/workspace。
- Server Admin token 不能访问普通 API，普通 token 不能访问 admin API。

### 11.2 推荐新增测试

- `internal/authz` policy 单元测试：
  - owner/admin/member/viewer 权限矩阵。
  - capability 判断。
  - workspace/project allowlist 判断。
- app 授权入口测试：
  - 普通 token decision。
  - impersonation decision。
  - 多 workspace ambiguity。
  - project scope filter。
- HTTP/MCP 集成测试：
  - 同一个 token 和同一个 workspace/project 下，HTTP API 与 HTTP MCP 产生一致授权结果。

### 11.3 验证命令

实现完成前不得声称通过，至少运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
```

如果 Phase 2 改动错误语义，还需要补充相关 HTTP / MCP golden 或集成测试。

## 12. 风险与控制

### 12.1 风险：等价重构引入行为漂移

控制：

- Phase 1 不改错误码。
- 先补授权入口测试，再迁移实现。
- 每次迁移一个入口：app -> HTTP -> MCP。

### 12.2 风险：authz 包反向依赖 app 导致分层混乱

控制：

- `internal/authz` 尽量只放纯模型和纯规则。
- 如果需要使用 `app.Permission`，可以先保留在 app 内部，或把 permission 类型迁出到更底层包。
- 不让 authz 直接依赖 HTTP/MCP/Cobra。

### 12.3 风险：错误语义整理破坏客户端

控制：

- Phase 2 单独实施。
- 变更前列出旧码 -> 新码映射。
- 默认保留现有 code，只集中构造和文档化。

### 12.4 风险：未来 SSO 误用 token 模型

控制：

- 在 spec 和后续 plan 中明确 CredentialBrowserSession 是独立凭证类型。
- 不把浏览器 session 写入 `api_tokens`。
- 不让 browser session 拥有 token scope；它只代表登录用户，权限由 membership role 决定。

## 13. 验收标准

Phase 1 完成时：

- 代码中存在清晰的 Authorization Decision 概念。
- HTTP API、HTTP MCP 授权入口复用同一套决策流程或同一套核心 helper。
- role permission matrix 不再散落。
- 现有外部 API、token 格式、错误码、数据库 schema 不变。
- 所有既有测试通过，并新增 authz 决策单元测试。

Phase 2 完成时：

- 授权相关错误码语义有集中定义。
- HTTP status 映射有集中规则。
- README 或 manual 文档说明 token scope、membership role、impersonation、server admin token 的边界。
- 错误语义相关测试覆盖主要分支。

## 14. 推荐实施顺序

1. 写 implementation plan，仅覆盖 Phase 1。
2. Phase 1 完成并验证后，再写 Phase 2 plan。
3. Phase 2 完成后，再评估是否需要浏览器 SSO / 飞书登录 spec。

不要把 SSO、organization、SCIM 或飞书导入混入本次重构。先把授权决策层变清楚，再接外部身份。
