---
title: "用户指南"
weight: 20
---

# 用户指南

这一章按日常使用顺序介绍 taskg。你可以把它当成“正常一天怎么用 taskg”的主线。

## taskg 的工作方式

taskg 把任务当作结构化对象保存到 SQLite。任务可以有 project、tag、priority、due、depends、annotations、UDA 等字段。CLI 既是操作入口，也是查询入口。

最常用的模式是：

```bash
taskg add "任务描述" project:<project-slug> +tag due:tomorrow
taskg list
taskg <id> modify priority:H +next
taskg <id> done
```

## 先创建 project

project 是 workspace 内的真实项目。任务引用 project 前必须先注册：

```bash
taskg project add ai-agent-platform name:"AI Agent Platform"
taskg project add erp-rewrite name:"ERP Rewrite"
```

然后创建任务：

```bash
taskg add "Design task.query schema" project:ai-agent-platform +mcp
taskg add "Review ERP migration" project:erp-rewrite +review
```

如果 project 不存在，命令会失败，而不是自动创建：

```bash
taskg add "Ghost task" project:ghost
```

## 使用 working-set ID

Human 报表里的 `ID` 是当前 working set 里的数字 ID，方便手工操作：

```bash
taskg list
taskg info 1
taskg 1 modify +next
taskg 1 done
```

脚本和远程系统应优先使用 UUID：

```bash
taskg _uuids +next
taskg info <uuid>
```

## 用 tag 和 priority 组织任务

```bash
taskg add "Fix webhook retry" project:ai-agent-platform +backend +urgent priority:H
taskg +backend list
taskg priority:H next
```

标签用 `+tag` 添加，用 `-tag` 移除：

```bash
taskg 1 modify +review -urgent
```

## 用 context 聚焦当前工作

context 是一个命名过滤器。启用后，`list`、`next`、报表和 helper 会自动叠加它。

```bash
taskg context define mcp 'project:ai-agent-platform +mcp status:pending'
taskg context use mcp
taskg list
```

临时忽略 context：

```bash
taskg --no-context list
taskg rc.context=none list
```

清空 active context：

```bash
taskg context none
```

## 查看 urgency

`next` 默认按 urgency 排序。你也可以查看某个任务 urgency 的解释：

```bash
taskg urgency 1
taskg urgency 1 --json
taskg _urgency 1
```

urgency 会考虑 `+next`、due、priority、age、active、blocked/blocking、UDA 系数等因素。

## 导入和导出

导出任务 JSON：

```bash
taskg export > tasks.json
taskg --json export > tasks.json
```

导入任务：

```bash
taskg import tasks.json
```

注意：JSON export/import 只覆盖任务数据，不是完整数据库备份。Hook、token、audit、workspace、membership 等数据请用 SQLite 备份，见 [备份与恢复](backup-restore.md)。

## 本地与远程的区别

本地模式直接读写 SQLite：

```bash
taskg list
```

远程模式通过 HTTP API 访问 server：

```bash
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" list
```

远程模式下，权限由 token、workspace scope、project scope 和 membership 共同决定。

