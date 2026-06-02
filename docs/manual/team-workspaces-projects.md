---
title: "团队、Workspace 与 Project"
weight: 60
---

# 团队、Workspace 与 Project

taskg 的企业边界由 workspace、project、user、membership 和 audit 组成。

## 概念

- `workspace`：企业、团队或租户级隔离边界。
- `project`：workspace 内的真实项目。
- `user`：本地或服务端 actor。
- `member`：user 在 workspace 内的角色。
- `audit`：写操作审计记录。

同名 project 可以存在于不同 workspace。`dajee/ai-agent-platform` 和 `partner/ai-agent-platform` 是两个不同项目。

## User

```bash
taskg user list
taskg user add alice email:alice@example.test
taskg user use alice
taskg user info
```

本地模式会自动创建 `local` user。

## Workspace

```bash
taskg workspace list
taskg workspace add dajee name:Dajee visibility:team
taskg workspace use dajee
taskg workspace info dajee
taskg workspace modify dajee description:"Dajee enterprise workspace"
taskg workspace archive old
```

一次性指定 workspace：

```bash
taskg --workspace dajee list
```

## Member 与角色

```bash
taskg member list
taskg member add bob role:viewer
taskg member role bob member
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
taskg --workspace dajee project add ai-agent-platform name:"AI Agent Platform"
taskg --workspace dajee project list
taskg --workspace dajee project info ai-agent-platform
taskg --workspace dajee project modify ai-agent-platform description:"Owns MCP work"
taskg --workspace dajee project archive ai-agent-platform
```

project 归档后不能被新任务引用，但已有任务仍可读取、完成和删除。

## Project 配置

project 级业务配置走 `project config`：

```bash
taskg project config set ai-agent-platform agent.background "Owns taskg MCP integration."
taskg project config get ai-agent-platform agent.background
taskg project config list ai-agent-platform
taskg project config unset ai-agent-platform agent.background
```

无 scope 的 `config` 不读写 project 配置。

## Audit

```bash
taskg audit list
taskg audit list --limit 20 --json
taskg audit list --project ai-agent-platform
```

audit 记录写操作，例如 task、context、workspace、member、project、config、token、hook、manual replay 等。

远程 API 中读取 audit 需要 `audit:read` capability，并且 actor 角色需要 admin 或 owner。

