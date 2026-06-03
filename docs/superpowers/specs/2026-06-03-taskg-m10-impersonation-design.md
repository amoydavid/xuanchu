# M10 设计规格：Token 委托与 Impersonation

**日期**：2026-06-03  
**状态**：草稿  
**作者**：taskg team

---

## 1. 背景与动机

### 1.1 场景

taskg 已经支持 PAT（Personal Access Token）和 Agent token，能覆盖以下场景：

- 个人本地 CLI 操作
- 团队统一 PM Agent（service account 视角）
- HTTP MCP 以固定身份接入

但有一类场景当前无法覆盖：

**员工个人助理 Agent**。员工和 Agent 对话，Agent 代表员工完成任务管理操作，audit log 里的 actor 应该是员工自己，而不是 Agent 平台的 service account。

同时还有**团队 PM Agent**：以统一的 `pm-agent` service account 视角管理多个 workspace 的任务进展。

### 1.2 两种 Agent 角色对比

| | 个人助理 Agent | 团队 PM Agent |
|---|---|---|
| 触发者 | 员工本人对话 | 团队调度 / 自动化 |
| Audit actor | **员工自己** | pm-agent service account |
| `assignee:me` 语义 | 员工自己 | 无意义（pm-agent 视角） |
| 权限边界 | 员工在 workspace 的 membership | pm-agent 的 membership |
| Token 来源 | 员工自己授权给 Agent 平台 | pm-agent 的长期 token |

### 1.3 当前缺口

当前没有机制让 Agent 平台：

1. **代表员工**发起请求（impersonation）
2. **引导员工**在 Agent 平台侧授权 taskg token，而不需要员工手动跑 CLI

---

## 2. 目标

- Agent 平台（外部 HTTP 服务）能持有一个 **带 `impersonate` scope 的 Agent token**，以该 token 的权限代任意 workspace 成员发起请求
- Audit log 清晰区分：**谁发起了操作（delegator）** 和 **谁是操作的名义 actor（subject）**
- 权限模型：impersonation 不能越过 subject 在 workspace 的 membership 权限
- 不引入完整 OAuth，保持 taskg token 体系的简单性

---

## 3. 核心概念

### 3.1 Impersonating Agent Token（带委托能力的 Agent token）

M10 不新增第三种 token type，也不允许普通 PAT 做 impersonation。  
沿用 M6 已有的 `agent` token 模型，在 capability 集合上新增 `impersonate` scope。

```
type: agent
scopes: [task:read, task:write, ..., impersonate]
workspace_ids: [ws-dajee]
```

`impersonate` 是一个明确的 scope，没有这个 scope 的 token 不能做 impersonation。

这样做的原因：

- 与 M6/M7 已有的 `pat` / `agent` 二元 token 模型保持一致
- 复用现有“Agent token 必须显式 workspace + scope”的安全边界
- 避免在 M10 同时引入新的 token type、prefix、迁移和管理语义

### 3.2 Impersonation 请求

带 `impersonate` scope 的 Agent token 在请求时附加 `X-Taskg-As` header，值为目标 user 的 name、email 或 UUID：

```
Authorization: Bearer taskg_agent_...
X-Taskg-As: alice
```

taskg 服务端：
1. 验证 Bearer token 是有效 Agent token，且有 `impersonate` scope
2. 先按 M6 既有规则解析 request workspace / project scope
3. 再解析 `X-Taskg-As` 找到目标 user
4. 验证目标 user 是该 effective workspace 的成员
5. 以目标 user 的 membership role 作为权限边界执行请求
6. audit log 和 access log 记录双重 actor

重要约束：

- `X-Taskg-As` 只替换 actor，不参与 workspace 选择
- workspace 继续遵守 M6/M7 现有解析顺序：显式 `workspace` / `project_id` 优先，其次是 token 可见范围内的默认选择
- 如果请求带 `X-Taskg-As`，但未显式指定 `workspace` 或 `project_id`，且 token 可见多个 workspace，应直接返回 `workspace_required`，不要静默落到 delegator 或 subject 的默认 workspace

### 3.3 双重 Actor Runtime / Audit

```json
{
  "actor_user_id": "alice-uuid",
  "actor_name": "alice",
  "delegator_token_id": "agent-token-uuid",
  "delegator_user_id": "pm-agent-uuid",
  "action": "task.create",
  "workspace_id": "...",
  "project_id": "..."
}
```

- `actor_user_id` / `actor_name`：名义操作者（员工），权限按此人在 workspace 的 role 决定
- `delegator_token_id` / `delegator_user_id`：实际持有 token 的 Agent/service account，用于追溯是哪个平台发起的

M10 不仅扩展 audit 表，还要冻结运行时契约：

- subject 驱动 `actor_user_id`、`actor_name`、`assignee:me`、active context 和权限判定
- delegator 只用于追责、日志和审计，不参与业务权限放大
- HTTP access log、`audit list --json`、后续 MCP/HTTP 结构化返回都要能暴露 subject 与 delegator 两组信息

### 3.4 Token / Scope 扩展

M10 采用以下冻结方案：

- token type 继续只有 `pat` / `agent`
- 仅 `agent` token 允许携带 `impersonate` scope
- 不新增 `service` type
- 不允许 `pat` 携带 `impersonate`

考虑到安全性，M10 要求：
- `impersonate` scope 只能由 workspace `admin` 或 `owner` 创建
- 创建 impersonating token 时，创建者当前 bearer token 本身也必须已经拥有 `impersonate`
- 新 token 的 capability / workspace allowlist / project allowlist 仍必须是当前 bearer token 的子集
- impersonating Agent token 绑定的 user 仍必须是 workspace 成员（有 membership）

---

## 4. API 设计

### 4.1 Impersonation 请求头

```
X-Taskg-As: <user-name | user-email | user-uuid>
```

只有 `agent` token 且拥有 `impersonate` scope 时，此 header 才生效。  
没有 `impersonate` scope 时，server 收到此 header 应返回 `token_scope_denied`。  
请求缺少明确 workspace，且 token 可见多个 workspace 时，返回 `workspace_required`。  
workspace 已确定后，如果目标 user 不存在，或存在但不是该 workspace 成员，对外统一返回 `membership_not_found`。

### 4.2 Audit Log 扩展

`audit_logs` 表新增字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `delegator_token_id` | string nullable | 发起 impersonation 的 Agent token ID |
| `delegator_user_id` | string nullable | delegator token 绑定的 user ID |

非 impersonation 请求这两个字段为 null。

同时需要同步扩展：

- audit app view / HTTP DTO / CLI `audit list --json`
- 服务端 access log 上下文
- 后续 Hook / MCP / HTTP 统一 actor envelope（如适用）

### 4.3 Token 创建

```bash
# 管理员创建可 impersonate 的 Agent token
taskg --workspace dajee token create pm-agent-service \
  --type agent \
  --scope task:read,task:write,project:read,workspace:read,impersonate \
  --expires-in 8760h
```

或通过 HTTP API：

```
POST /api/v1/tokens
{
  "name": "pm-agent-service",
  "type": "agent",
  "scopes": ["task:read", "task:write", "project:read", "workspace:read", "impersonate"],
  "workspace_ids": ["dajee"],
  "expires_in": 31536000
}
```

创建规则：

- 本地 CLI：当前 actor 必须在目标 workspace 中具备 admin/owner 角色，且若指定 `impersonate` scope，必须满足新的 M10 约束
- 远程 HTTP API：除 admin/owner + `token:write` 外，当前 bearer token 也必须已经拥有 `impersonate`，且新 token scope 必须是其子集
- M10 不提供匿名 bootstrap，也不绕开 M6 既有 token 子集约束

### 4.4 MCP 工具扩展

HTTP MCP 下，`X-Taskg-As` 是 request-scoped metadata，不是持久连接状态。  
客户端必须保证承载 tool call 的每个 HTTP 请求都带上 `Authorization` 和 `X-Taskg-As` header。  
stdio MCP 不支持 impersonation。

---

## 5. 权限模型

Impersonation 下的最终权限是以下所有条件的**交集**：

```
effective permission = 
  subject.membership_role        ∩
  agent_token.scopes             ∩
  agent_token.workspace_allowlist ∩
  agent_token.project_allowlist
```

举例：
- Alice 是 `member` role（无 `workspace:write`）
- Agent token 有 `workspace:write` scope
- 结果：以 Alice 身份操作时，**没有** `workspace:write`，因为 Alice 自身 role 不允许

反之：
- Alice 是 `admin` role（有 `task:write`）
- Agent token 只有 `task:read` scope
- 结果：以 Alice 身份操作时，**没有** `task:write`，因为 token scope 不包含

还要叠加 M6 既有的派生约束：

- 远程/API 创建 impersonating token 时，新 token 仍必须是当前 bearer token 的 capability / workspace / project 子集
- impersonation 只改变“本次请求的 actor”，不改变 token 自己可访问的 workspace/project 上界

---

## 6. 安全边界

- **`impersonate` scope 只能由 `admin` 或 `owner` 创建给 token**：普通 member 无法拿到可 impersonate 的 token
- **只有 `agent` token 可以带 `impersonate`**：M10 不允许 `pat` 做 impersonation
- **不能越过 subject 的 membership role**：impersonation 是降权代理，不是提权
- **Impersonation 必须在 token 的 workspace allowlist 内**：不能 impersonate 一个 token 无权访问的 workspace 里的 user
- **若 token 可见多个 workspace，impersonation 请求必须显式带 `workspace` 或 `project_id`**：避免默认 workspace 歧义
- **Audit 完整记录 delegator**：任何 impersonation 操作都可以追溯到 delegator token 和 service account
- **`X-Taskg-As` 对无 `impersonate` scope 的 token 不生效**：不能静默降级，直接返回错误

---

## 7. CLI 适配

本地 CLI 不需要 impersonation（本地直接 `user use` 切换身份）。

远程 CLI 新增 `--as` flag，对应 `X-Taskg-As` header：

```bash
taskg --server https://taskg.example.com \
  --token "$PM_AGENT_TOKEN" \
  --workspace dajee \
  --as alice \
  list assignee:me
```

规则：

- `--as` 只在 remote mode 生效；本地 CLI 不支持该 flag 驱动 impersonation
- `--as` 是 remote client 级别配置，而不是单 endpoint 的临时参数
- 同一条 CLI invocation 内产生的所有 HTTP 子请求都必须携带同一个 `X-Taskg-As`
- 对 `assignee:me`、active context、working-set ID 两跳解析等行为，都应以 subject 为准，而不是 delegator

---

## 8. Token 委托流程（Agent 平台集成）

Agent 平台要支持个人助理 Agent，推荐以下流程：

```
1. Agent 平台管理员持有带 `impersonate` scope 的 Agent token
2. 员工在 Agent 平台登录，平台记录其 taskg username / email
3. 员工和 Agent 对话时，Agent 平台在请求里加 X-Taskg-As: <员工 name>
4. taskg 以员工身份执行，audit actor 是员工
5. Agent 平台无需持有员工的个人 token
```

这消除了"每个员工单独持有 token"的管理负担，同时保持 audit 清晰。

---

## 9. 不进入 M10

- OAuth 2.0 / OIDC / PKCE 完整授权流程
- 跨 workspace 的全局 impersonation（必须限定在 token 的 workspace allowlist 内）
- impersonation 审批流 / 二次确认
- impersonation 的时间窗口限制（session-level impersonation）
- 用于员工自助授权的 Web UI（可以是 M11+）

---

## 10. 验收标准

- Agent token 携带 `impersonate` scope，加 `X-Taskg-As` header 后可以以目标 user 身份执行请求
- Audit log 同时记录 `actor_user_id`（subject）和 `delegator_token_id` / `delegator_user_id`
- 权限交集模型：subject role ∩ token scope ∩ token workspace/project allowlist，不能通过 impersonation 提权
- token 可见多个 workspace 且请求未显式指定 workspace / project_id 时返回 `workspace_required`
- 目标 user 不存在或不是 workspace 成员时统一返回 `membership_not_found`
- token 无 `impersonate` scope 时携带 `X-Taskg-As` 返回 `token_scope_denied`
- 普通 member 无法创建带 `impersonate` scope 的 token
- PAT 无法创建或持有 `impersonate` scope
- 远程 CLI `--as` flag 透传为 `X-Taskg-As` header
- MCP HTTP transport 的每个 tool-call request 都可透传 `X-Taskg-As`
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 通过

---

## 11. 对后续 milestone 的影响

- M11 可以在此基础上实现 Web UI 让员工自助授权 Agent 平台（页面引导员工点击，平台调用 API 获取 token）
- 长期若引入 OIDC，`X-Taskg-As` 可以直接对接 OIDC subject claim，impersonation 层逻辑不变
