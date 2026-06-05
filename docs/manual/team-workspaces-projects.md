---
title: "团队、Workspace 与 Project"
weight: 60
---

# 团队、Workspace 与 Project

xuanchu 的企业边界由 workspace、project、user、membership 和 audit 组成。

如果你还不确定“当前 CLI/MCP 到底以哪个 user 身份运行”，先读 [身份与初始化](identity-and-initialization.md)。本页更偏团队对象和管理命令。

## 概念

- `workspace`：企业、团队或租户级隔离边界。
- `project`：workspace 内的真实项目。
- `user`：本地或服务端 actor。
- `member`：user 在 workspace 内的角色。
- `audit`：写操作审计记录。

同名 project 可以存在于不同 workspace。`dajee/ai-agent-platform` 和 `partner/ai-agent-platform` 是两个不同项目。

## User

```bash
xuanchu user list
xuanchu user add alice email:alice@example.test
xuanchu user use alice
xuanchu user info
```

本地模式会自动创建 `local` user。

本地 CLI 的 actor 来自 `user use` 设置的 active user。远程 CLI 和 HTTP MCP 的 actor 来自 token 绑定的 user，不受本机 `user use` 影响。

## Workspace

```bash
xuanchu workspace list
xuanchu workspace add dajee name:Dajee visibility:team
xuanchu workspace use dajee
xuanchu workspace info dajee
xuanchu workspace modify dajee description:"Dajee enterprise workspace"
xuanchu workspace archive old
```

一次性指定 workspace：

```bash
xuanchu --workspace dajee list
```

`--workspace` 只选择本次命令的 effective workspace，不会改变 actor，也不会突破 token 的 workspace scope。

## Member 与角色

```bash
xuanchu member list
xuanchu member add bob role:viewer
xuanchu member role bob member
```

角色：

| 角色 | 说明 |
|---|---|
| `viewer` | 读任务、报表、helper、成员列表，切换自己的 active context |
| `member` | 额外可写任务、import、管理 context |
| `admin` | 额外可管理 workspace metadata、成员、audit、UDA schema、Hook |
| `owner` | 额外可授予/降级 owner、归档 workspace |

当前没有 `member delete`。如需撤销写权限，可以把成员降为 `viewer`。

## Project

```bash
xuanchu --workspace dajee project add ai-agent-platform name:"AI Agent Platform"
xuanchu --workspace dajee project list
xuanchu --workspace dajee project info ai-agent-platform
xuanchu --workspace dajee project modify ai-agent-platform description:"Owns MCP work"
xuanchu --workspace dajee project archive ai-agent-platform
```

project 归档后不能被新任务引用，但已有任务仍可读取、完成和删除。

## Project 配置

project 级业务配置走 `project config`：

```bash
xuanchu project config set ai-agent-platform agent.background "Owns xuanchu MCP integration."
xuanchu project config get ai-agent-platform agent.background
xuanchu project config list ai-agent-platform
xuanchu project config unset ai-agent-platform agent.background
```

无 scope 的 `config` 不读写 project 配置。

## Audit

```bash
xuanchu audit list
xuanchu audit list --limit 20 --json
xuanchu audit list --project ai-agent-platform
```

audit 记录写操作，例如 task、context、workspace、member、project、config、token、hook、manual replay 等。

远程 API 中读取 audit 需要 `audit:read` capability，并且 actor 角色需要 admin 或 owner。
