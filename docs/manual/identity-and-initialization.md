---
title: "身份与初始化"
weight: 15
---

# 身份与初始化

taskg 有 `user`、`workspace` 和 `member` 概念。每次读写任务时，taskg 都需要知道：

- 当前是谁在操作，也就是 actor user。
- 当前在哪个 workspace 操作。
- 这个 user 在该 workspace 里是什么角色。
- 如果是远程或 MCP HTTP，请求 token 还允许访问哪些 workspace/project。

这页是身份模型的入口。建议所有团队管理员、Agent 集成者都先读这一页。

## 首次运行的默认状态

第一次运行本地 CLI 时，taskg 会自动创建：

- `local` user
- `local` workspace
- `local -> local` owner membership

所以单人本地使用可以直接开始：

```bash
taskg project add inbox name:"Inbox"
taskg add "Try taskg" project:inbox
taskg list
```

这个默认身份适合本机试用和单人使用。团队或服务端部署建议显式创建自己的 user 和 workspace。

## 推荐的团队初始化流程

假设你要初始化一个名为 `dajee` 的 workspace，并以 `alice` 作为第一个管理员：

```bash
taskg user add alice email:alice@example.test
taskg user use alice

taskg workspace add dajee name:Dajee visibility:team
taskg workspace use dajee

taskg project add ai-agent-platform name:"AI Agent Platform"
```

此时本机 active user 是 `alice`，active workspace 是 `dajee`。后续不带 `--workspace` 的本地命令会默认在 `dajee` 中执行。

查看当前身份：

```bash
taskg show
taskg _show active.user active.workspace active.context
```

## 本地 CLI 的身份来源

本地 CLI 直接读写 SQLite，身份来自本机 active state：

```bash
taskg user use alice
taskg workspace use dajee
taskg list
```

也可以用 `--workspace` 只影响本次命令：

```bash
taskg --workspace partner list
```

本地 CLI 的规则：

- `user use <user>` 修改 active user。
- `workspace use <workspace>` 修改该 user 的 active workspace。
- `--workspace <slug|uuid>` 只覆盖本次命令。
- actor user 必须是 effective workspace 的 member。
- context 只是默认过滤器，不改变权限。

## 添加成员

在当前 workspace 添加成员：

```bash
taskg member add bob role:viewer
taskg member role bob member
taskg member list
```

角色从低到高：

- `viewer`
- `member`
- `admin`
- `owner`

本地 CLI 切换到另一个 user：

```bash
taskg user use bob
taskg list
```

如果 `bob` 不是当前 workspace 成员，会得到 `membership_not_found` 或权限错误。

## 远程 CLI 的身份来源

远程 CLI 的身份不来自本机 `user use`。远程 CLI 使用 Bearer token，actor 是 token 绑定的 user。

```bash
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" --workspace dajee list
```

远程 CLI 的规则：

- actor user 来自 token。
- `--workspace` 选择 effective workspace，但不能突破 token workspace scope。
- `--project` / `--project-id` 选择 project scope，但不能突破 token project allowlist。
- 本机 `taskg user use alice` 不会改变远程请求里的 actor。

查看远程请求身份：

```bash
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" --workspace dajee show
```

或直接调用 HTTP：

```bash
curl -H "Authorization: Bearer $TASKG_TOKEN" \
  "https://taskg.example.com/api/v1/me"
```

## 创建远程 token

第一个远程 token 建议在 server 启动前用本地 CLI 创建。

> **注意**：`--workspace` 仅覆盖本次命令的 workspace，actor 仍是本机 active user。如果 active user 不是目标 workspace 的成员，会报 `membership_not_found`。请先确认身份：
>
> ```bash
> taskg _show active.user active.workspace
> # 如果 active user 不是目标 workspace 成员，先切换：
> taskg user use <workspace-owner>
> taskg workspace use dajee
> ```

```bash
taskg --workspace dajee token create admin \
  --type pat \
  --scope task:read,task:write,project:read,project:write,workspace:read,workspace:write,token:read,token:write,audit:read,hook:read,hook:write \
  --expires-in 720h
```

给 Agent 创建 project-scoped token：

```bash
taskg --workspace dajee token create mcp-agent \
  --type agent \
  --scope task:read,task:write,project:read,context:read,config:read \
  --project ai-agent-platform \
  --expires-in 720h
```

token 创建后 raw token 只显示一次。后续只能撤销重建，不能再次查看原文。

## MCP 下的身份来源

MCP 有两种运行模式，身份来源不同。

### stdio MCP

```bash
taskg mcp stdio
```

stdio MCP 使用本机 runtime 身份，等价于本地 CLI：

- actor 来自本机 active user。
- workspace 来自本机 active workspace，或 tool input 中的 workspace scope。
- project scope 来自 tool input 和本地权限校验。
- 会读取本机 SQLite 和本机可用配置。

因此，在启动 stdio MCP 前应先确认：

```bash
taskg user use alice
taskg workspace use dajee
taskg _show active.user active.workspace
```

如果你在 Claude Code、OpenClaw、Hermes Agent 中用 stdio MCP，本质上就是让该客户端以这台机器上的 active user 身份操作本地 taskg 数据库。

### HTTP MCP

HTTP MCP 通过 server 的 `/mcp` endpoint 访问：

```text
https://taskg.example.com/mcp
```

HTTP MCP 使用 Bearer token 身份：

- actor 来自 token 绑定的 user。
- workspace 来自请求 scope / token default / token allowlist。
- project scope 来自请求参数和 token project allowlist。
- 不读取调用者本机 TOML。
- 不依赖调用者本机 `user use` 或 `workspace use`。

因此，Agent 的真实权限必须通过 token 控制，而不是通过提示词控制。

## 三种入口的身份对照

| 入口 | Actor 来源 | Workspace 来源 | Project 限制来源 | 是否读取本机 TOML |
|---|---|---|---|---|
| 本地 CLI | `user use` active user | `workspace use` 或 `--workspace` | 当前 workspace 内解析 | 是 |
| 远程 CLI | Bearer token 绑定 user | `--workspace` / token scope | `--project` / `--project-id` / token scope | 只读远程连接本机配置，不读业务规则 |
| MCP stdio | 本机 active user | 本机 active workspace / tool scope | tool scope + 本地权限 | 是 |
| MCP HTTP | Bearer token 绑定 user | token scope / tool scope | token project allowlist + tool scope | 否 |

## 常见建议

- 单人本地使用，可以接受默认 `local` user/workspace。
- 团队使用，请显式创建 user、workspace 和 project。
- 服务端部署，请先创建 admin token，再启动长期服务。
- Agent 使用 HTTP MCP 时，优先创建 project-scoped Agent token。
- 不要让 Agent 通过提示词“声明自己是谁”；真实身份必须来自 stdio runtime 或 HTTP token。
