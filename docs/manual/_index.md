---
title: "Xuanchu 用户手册"
weight: 1
---

# Xuanchu 用户手册

Xuanchu 的命令行名是 `xuanchu`。它是一个 Taskwarrior 风格的企业任务运行时，既可以作为本地 CLI 使用，也可以作为 HTTP 服务端、远程 CLI 客户端和 MCP Server 使用。

这份手册面向使用者和管理员，重点回答三个问题：

- 我怎样快速开始管理任务？
- 团队、项目、权限、远程访问应该怎么配置？
- HTTP、MCP、Webhook Hook、部署和备份该怎么用？

## 推荐阅读路径

如果你只是想开始用 xuanchu：

1. 读 [快速使用](quick-start.md)
2. 读 [身份与初始化](identity-and-initialization.md)
3. 读 [用户指南](user-guide.md)
4. 需要更多细节时再查 [任务操作](tasks.md) 和 [查询与报表](query-reports.md)

如果你要在团队里使用：

1. 读 [团队、Workspace 与 Project](team-workspaces-projects.md)
2. 读 [远程 CLI 与 HTTP API](remote-cli-and-api.md)
3. 读 [部署指南](deployment.md)

如果你要给 Agent 或外部系统接入：

1. 读 [远程 CLI 与 HTTP API](remote-cli-and-api.md)
2. 读 [MCP 使用指南](mcp.md)
3. 读 [Webhook Hook 使用指南](hooks.md)
4. 需要定时提醒时读 [定时通知与第三方通知](notifications.md)

## 手册章节

- [快速使用](quick-start.md)
- [身份与初始化](identity-and-initialization.md)
- [用户指南](user-guide.md)
- [任务操作](tasks.md)
- [查询与报表](query-reports.md)
- [配置、Context 与 UDA](config-context-uda.md)
- [团队、Workspace 与 Project](team-workspaces-projects.md)
- [远程 CLI 与 HTTP API](remote-cli-and-api.md)
- [MCP 使用指南](mcp.md)
- [Webhook Hook 使用指南](hooks.md)
- [定时通知与第三方通知](notifications.md)
- [Web Admin Console](web-console.md)
- [部署指南](deployment.md)
- [备份与恢复](backup-restore.md)
- [常见问题与排障](troubleshooting.md)
- [命令速查](reference/commands.md)
- [错误码速查](reference/errors.md)

## 核心概念

- `workspace` 是企业、团队或租户级隔离边界。
- `project` 是 workspace 内的真实项目，任务引用 project 前必须先注册 project。
- `user` 是执行操作的 actor；本地 CLI 使用 active user，远程 CLI / HTTP MCP 使用 token 绑定的 user。
- `context` 是默认查询过滤器，不是权限边界。
- `token` 是远程 CLI、HTTP API 和 HTTP MCP 的访问凭证。
- `hook` 是服务端内部事件发生后的异步 webhook 投递能力。
- `notification sink` 是定时提醒投递目标；`reminder rule` 是基于 due 的定时提醒规则。

## 当前能力范围

当前 Xuanchu 已支持：

- 本地 CLI 与 SQLite / PostgreSQL 存储
- Taskwarrior 风格任务字段、查询、报表、urgency 和 helper 命令
- 配置、context、UDA、`.taskrc` 只读导入
- 多 user、多 workspace、member role、audit
- project 实体化与 project 级配置
- HTTP/JSON API、远程 CLI、PAT / Agent token
- 嵌入式 Web Admin Console（`/`）与 Server Admin Bootstrap（`/admin/login`）
- MCP stdio 与 HTTP transport
- 服务端 Webhook Hook、投递重试、dead-letter、manual replay
- 定时通知、HTTP request template sink、动态 endpoint 与 delivery replay
- PostgreSQL 后端支持（`--db-url`）
