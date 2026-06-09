---
title: "查询与报表"
weight: 40
---

# 查询与报表

Xuanchu 的 CLI 也是查询语言。你可以把 filter 放在报表命令前，也可以放在报表命令后。

```bash
xuanchu +next list
xuanchu list +next
xuanchu '(project:agentapi and +review) or priority:H' next
```

## 常用查询

| 查询 | 说明 |
|---|---|
| `project:agentapi` | 查询 project |
| `+review` | 包含 tag |
| `-review` | 不包含 tag |
| `status:pending` | 按状态查询 |
| `priority:H` | 高优先级 |
| `due:today` | 今天截止 |
| `due.before:tomorrow` | 明天前截止 |
| `due.after:2days` | 两天后之后截止 |
| `/schema/` | description 子串匹配 |

`description:spec`、`description:/spec/` 和裸 `/spec/` 都按 description 子串匹配，不是正则。

日期字段支持 `today`、`tomorrow`、`eod`、`eow`、`eom`、`Ndays`、RFC3339、`YYYY-MM-DD`，也支持以当前时间为基准的 `now` 和 `now +/- duration`。查询语法中的规范写法不带空格，例如：

```bash
xuanchu 'due.before:now+24h' list
xuanchu 'due.after:now-2h' list
xuanchu 'scheduled.before:now+2h30m' list
```

其中 `duration` 使用 Go `time.ParseDuration` 语法，支持 `h`、`m`、`s` 等单位；`now+1d`、`now+90min` 不是合法写法。

## 布尔组合

```bash
xuanchu '+next or due.before:tomorrow' list
xuanchu '(project:agentapi and +urgent) or priority:H' list
xuanchu 'not +waiting' list
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
xuanchu list
xuanchu next
xuanchu all
xuanchu completed
xuanchu deleted
xuanchu waiting
xuanchu active
xuanchu ready
xuanchu overdue
xuanchu blocked
xuanchu blocking
```

报表会把内置 filter 和用户 filter 组合起来。比如 `completed +review` 表示“已完成且带 review tag 的任务”。

如果想绕过默认 pending 限制，用 `all`。

## Urgency

`next` 默认按 urgency 排序。

```bash
xuanchu urgency 1
xuanchu urgency 1 --json
xuanchu _urgency 1
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
xuanchu _ids +next
xuanchu _uuids project:agentapi
xuanchu _projects
xuanchu _tags
xuanchu _udas
xuanchu _unique project
xuanchu _get 1.uuid 1.description 1.urgency
xuanchu _show database.path active.user active.workspace active.context
xuanchu _version
```

脚本里建议优先使用：

- UUID 或 `task_slug`，而不是 working-set ID。
- `--json`，而不是解析 human 输出。
- helper 命令，而不是解析表格。

## Calc

```bash
xuanchu calc '1 + 2 * 3'
xuanchu calc '10 > 2 and 3 < 5'
```

`calc` 暴露表达式求值能力，主要用于调试和脚本。
