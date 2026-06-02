---
title: "快速使用"
weight: 10
---

# 快速使用

这一页给你一条最短路径：构建 taskg，创建项目，添加任务，查询任务，完成任务。

## 1. 构建二进制

```bash
go build -o taskg ./cmd/taskg
```

默认数据库路径是：

```text
~/.local/share/taskg/taskg.db
```

也可以临时指定数据库：

```bash
./taskg --db ./taskg.db list
./taskg --data-dir ./data list
```

## 2. 创建第一个项目

taskg 要求先注册 project，再把任务放进 project。

```bash
./taskg project add ai-agent-platform name:"AI Agent Platform"
```

查看项目：

```bash
./taskg project list
```

## 3. 添加任务

```bash
./taskg add "Write MCP task docs" project:ai-agent-platform +docs due:tomorrow
./taskg add "Review API schema" project:ai-agent-platform priority:H +review
```

常见写法：

- `project:ai-agent-platform` 表示任务所属 project。
- `+docs` 添加标签。
- `priority:H` 设置高优先级。
- `due:tomorrow` 设置截止日期。

## 4. 查看任务

```bash
./taskg list
./taskg next
./taskg info 1
```

`list` 和 `next` 输出里的 `ID` 是 working-set ID。你可以用它操作任务，也可以用完整 UUID。

```bash
./taskg 1 modify priority:M +next
./taskg 1 done
```

## 5. 查询任务

```bash
./taskg +review list
./taskg project:ai-agent-platform list
./taskg '(project:ai-agent-platform and +review) or priority:H' next
```

复杂查询建议加引号，避免 shell 处理括号和空格。

## 6. 输出 JSON

```bash
./taskg --json list
./taskg --json info 1
./taskg --json export
```

脚本里优先使用 JSON 或 helper 命令：

```bash
./taskg _ids +review
./taskg _uuids project:ai-agent-platform
./taskg _get 1.uuid 1.description 1.urgency
```

## 7. 一次完整日常流程

```bash
./taskg project add docs-site name:"Docs Site"
./taskg add "Draft user guide" project:docs-site +writing due:tomorrow
./taskg add "Review hook docs" project:docs-site +review priority:H
./taskg +review list
./taskg 2 annotate "Need to mention signature verification"
./taskg 2 done
./taskg completed
```

## 8. 下一步读什么

- 想了解完整任务操作：读 [任务操作](tasks.md)
- 想学查询和报表：读 [查询与报表](query-reports.md)
- 想团队使用：读 [团队、Workspace 与 Project](team-workspaces-projects.md)

