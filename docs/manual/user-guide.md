---
title: "用户指南"
weight: 20
---

# 用户指南

这一章按日常使用顺序介绍 xuanchu。你可以把它当成“正常一天怎么用 xuanchu”的主线。

## Xuanchu 的工作方式

xuanchu 把任务当作结构化对象保存到数据库（默认 SQLite，也支持 PostgreSQL）。任务可以有 project、tag、priority、due、depends、annotations、UDA 等字段。CLI 既是操作入口，也是查询入口。

最常用的模式是：

```bash
xuanchu add "任务描述" project:<project-slug> +tag due:tomorrow
xuanchu list
xuanchu <id> modify priority:H +next
xuanchu <id> done
```

## 先创建 project

project 是 workspace 内的真实项目。任务引用 project 前必须先注册：

```bash
xuanchu project add agentapi name:"AI Agent Platform"
xuanchu project add erpflow name:"ERP Rewrite"
```

然后创建任务：

```bash
xuanchu add "Design task.query schema" project:agentapi +mcp
xuanchu add "Review ERP migration" project:erpflow +review
```

如果 project 不存在，命令会失败，而不是自动创建：

```bash
xuanchu add "Ghost task" project:ghost
```

## 使用任务引用

Human 报表里的 `ID` 是当前 working set 里的数字 ID，方便手工操作；带 project 的任务还会有稳定短引用 `task_slug`，例如 `agentapi-1`：

```bash
xuanchu list
xuanchu info 1
xuanchu info agentapi-1
xuanchu 1 modify +next
xuanchu agentapi-1 done
```

脚本和远程系统应使用 UUID、已物化 `task_slug` 或 occurrence_ref；projected 循环实例只有 occurrence_ref。HTTP API 与 MCP tool 不接受纯数字 working-set ID：

```bash
xuanchu _uuids +next
xuanchu info <uuid>
```

## 用 tag 和 priority 组织任务

```bash
xuanchu add "Fix webhook retry" project:agentapi +backend +urgent priority:H
xuanchu +backend list
xuanchu priority:H next
```

标签用 `+tag` 添加，用 `-tag` 移除：

```bash
xuanchu 1 modify +review -urgent
```

## 用 context 聚焦当前工作

context 是一个命名过滤器。启用后，`list`、`next`、报表和 helper 会自动叠加它。

```bash
xuanchu context define mcp 'project:agentapi +mcp status:pending'
xuanchu context use mcp
xuanchu list
```

临时忽略 context：

```bash
xuanchu --no-context list
xuanchu rc.context=none list
```

清空 active context：

```bash
xuanchu context none
```

## 查看 urgency

`next` 默认按 urgency 排序。你也可以查看某个任务 urgency 的解释：

```bash
xuanchu urgency 1
xuanchu urgency 1 --json
xuanchu _urgency 1
```

urgency 会考虑 `+next`、due、priority、age、active、blocked/blocking、UDA 系数等因素。

## 导入和导出

导出任务 JSON：

```bash
xuanchu export > tasks.json
xuanchu --json export > tasks.json
```

导入任务：

```bash
xuanchu import tasks.json
```

注意：JSON export/import 只覆盖任务数据，不是完整数据库备份。Hook、token、audit、workspace、membership 等数据请用数据库备份（SQLite 文件或 `pg_dump`），见 [备份与恢复](backup-restore.md)。

## 本地与远程的区别

本地模式直接读写本地数据库（默认 SQLite）：

```bash
xuanchu list
```

远程模式通过 HTTP API 访问 server：

```bash
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" list
```

远程模式下，权限由 token、workspace scope、project scope 和 membership 共同决定。
