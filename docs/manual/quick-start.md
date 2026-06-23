---
title: "快速使用"
weight: 10
---

# 快速使用

这一页给你一条最短路径：构建 xuanchu，创建项目，添加任务，查询任务，完成任务。

## 1. 构建二进制

```bash
go build -o xuanchu ./cmd/xuanchu
```

默认数据库路径是：

```text
~/.local/share/xuanchu/xuanchu.db
```

也可以临时指定数据库：

```bash
./xuanchu --db ./xuanchu.db list
./xuanchu --data-dir ./data list
```

也可以使用 PostgreSQL：

```bash
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable" list
```

## 2. 创建第一个项目

xuanchu 要求先注册 project，再把任务放进 project。

首次本地运行时，xuanchu 会自动创建 `local` user、`local` workspace 和 owner membership。单人试用可以直接使用这个默认身份。团队或服务端使用建议先读 [身份与初始化](identity-and-initialization.md)，显式创建自己的 user/workspace。

```bash
./xuanchu project add agentapi name:"AI Agent Platform"
```

查看项目：

```bash
./xuanchu project list
```

## 3. 添加任务

```bash
./xuanchu add "Write MCP task docs" project:agentapi +docs due:tomorrow
./xuanchu add "Review API schema" project:agentapi priority:H +review
```

常见写法：

- `project:agentapi` 表示任务所属 project。
- `+docs` 添加标签。
- `priority:H` 设置高优先级。
- `due:tomorrow` 设置截止日期。

## 4. 查看任务

```bash
./xuanchu list
./xuanchu next
./xuanchu info 1
./xuanchu info agentapi-1
```

`list` 和 `next` 输出里的 `ID` 是 working-set ID。带 project 的任务还会有 `task_slug`，例如 `agentapi-1`。本地 CLI 可以用 working-set ID、UUID、UUID 前缀或 `task_slug` 操作任务；HTTP API 和 MCP tool 只接受 UUID 或 `task_slug`。

```bash
./xuanchu 1 modify priority:M +next
./xuanchu agentapi-1 done
```

## 5. 查询任务

```bash
./xuanchu +review list
./xuanchu project:agentapi list
./xuanchu '(project:agentapi and +review) or priority:H' next
```

复杂查询建议加引号，避免 shell 处理括号和空格。

## 6. 输出 JSON

```bash
./xuanchu --json list
./xuanchu --json info 1
./xuanchu --json export
```

脚本里优先使用 JSON 或 helper 命令：

```bash
./xuanchu _ids +review
./xuanchu _uuids project:agentapi
./xuanchu _get 1.uuid 1.title 1.urgency
```

## 7. 一次完整日常流程

```bash
./xuanchu project add docsite name:"Docs Site"
./xuanchu add "Draft user guide" project:docsite +writing due:tomorrow
./xuanchu add "Review hook docs" project:docsite +review priority:H
./xuanchu +review list
./xuanchu 2 annotate "Need to mention signature verification"
./xuanchu 2 done
./xuanchu completed
```

## 8. 下一步读什么

- 想搞清楚当前是谁在操作、在哪个 workspace：读 [身份与初始化](identity-and-initialization.md)
- 想了解完整任务操作：读 [任务操作](tasks.md)
- 想学查询和报表：读 [查询与报表](query-reports.md)
- 想团队使用：读 [团队、Workspace 与 Project](team-workspaces-projects.md)
