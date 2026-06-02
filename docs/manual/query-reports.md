---
title: "查询与报表"
weight: 40
---

# 查询与报表

taskg 的 CLI 也是查询语言。你可以把 filter 放在报表命令前，也可以放在报表命令后。

```bash
taskg +next list
taskg list +next
taskg '(project:ai-agent-platform and +review) or priority:H' next
```

## 常用查询

| 查询 | 说明 |
|---|---|
| `project:ai-agent-platform` | 查询 project |
| `+review` | 包含 tag |
| `-review` | 不包含 tag |
| `status:pending` | 按状态查询 |
| `priority:H` | 高优先级 |
| `due:today` | 今天截止 |
| `due.before:tomorrow` | 明天前截止 |
| `due.after:2days` | 两天后之后截止 |
| `/schema/` | description 子串匹配 |

`description:spec`、`description:/spec/` 和裸 `/spec/` 都按 description 子串匹配，不是正则。

## 布尔组合

```bash
taskg '+next or due.before:tomorrow' list
taskg '(project:ai-agent-platform and +urgent) or priority:H' list
taskg 'not +waiting' list
```

支持：

- `and`
- `or`
- `xor`
- `not`
- 括号分组

复杂查询建议整体加引号。

## 报表

```bash
taskg list
taskg next
taskg all
taskg completed
taskg deleted
taskg waiting
taskg active
taskg ready
taskg overdue
taskg blocked
taskg blocking
```

报表会把内置 filter 和用户 filter 组合起来。比如 `completed +review` 表示“已完成且带 review tag 的任务”。

如果想绕过默认 pending 限制，用 `all`。

## Urgency

`next` 默认按 urgency 排序。

```bash
taskg urgency 1
taskg urgency 1 --json
taskg _urgency 1
```

urgency 会考虑：

- `+next`
- due / overdue
- priority
- age
- active
- blocked / blocking
- annotations
- UDA urgency coefficient

## Helper 命令

以下划线开头的命令适合脚本和补全，输出无装饰：

```bash
taskg _ids +next
taskg _uuids project:ai-agent-platform
taskg _projects
taskg _tags
taskg _udas
taskg _unique project
taskg _get 1.uuid 1.description 1.urgency
taskg _show database.path active.user active.workspace active.context
taskg _version
```

脚本里建议优先使用：

- UUID，而不是 working-set ID。
- `--json`，而不是解析 human 输出。
- helper 命令，而不是解析表格。

## Calc

```bash
taskg calc '1 + 2 * 3'
taskg calc '10 > 2 and 3 < 5'
```

`calc` 暴露表达式求值能力，主要用于调试和脚本。

