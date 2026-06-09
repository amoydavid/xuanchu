---
title: "任务操作"
weight: 30
---

# 任务操作

## 添加任务

```bash
xuanchu add "Write project spec" project:agentapi +planning due:tomorrow
xuanchu add "Review PR" project:agentapi priority:H +review
xuanchu add "Ship docs" @alice @bob
```

任务描述可以作为第一个参数，也可以和修改项一起出现。常见修改项：

| 写法 | 说明 |
|---|---|
| `project:<slug>` | 设置 project，必须已注册 |
| `priority:H` | 设置优先级，常用 `H`、`M`、`L` |
| `due:tomorrow` | 设置截止时间 |
| `wait:tomorrow` | 等到指定时间前隐藏 |
| `scheduled:eow` | 计划开始时间 |
| `until:eom` | 到期后自动消失 |
| `+next` | 添加标签 |
| `-next` | 移除标签 |
| `@alice` | 为新任务追加 assignee |

## 任务引用

本地 CLI 可以用四种方式定位任务：

| 引用 | 说明 |
|---|---|
| `1` | 当前报表里的 working-set ID，只适合人手工操作 |
| `<uuid>` | 任务永久 UUID |
| `<uuid-prefix>` | 足够长且不歧义的 UUID 前缀 |
| `agentapi-1` | `task_slug`，由 `<projectSlug>-<seq>` 派生 |

HTTP API、HTTP MCP 和 stdio MCP 属于协议入口，只接受 UUID 或 `task_slug`。纯数字 working-set ID 不进入协议契约；远程 CLI 如果收到 `info 1` 这类输入，会先在客户端按当前 working set 两跳解析。

## 查看任务

```bash
xuanchu list
xuanchu next
xuanchu all
xuanchu info 1
xuanchu info agentapi-1
xuanchu info <uuid>
xuanchu list assignee:alice
xuanchu next assignee:me
```

常用报表：

| 命令 | 说明 |
|---|---|
| `list` | 默认 pending 任务 |
| `next` | 按 urgency 排序的下一批任务 |
| `all` | 包含更多状态 |
| `completed` | 已完成任务 |
| `deleted` | 已删除任务 |
| `waiting` | 等待中任务 |
| `active` | 已 start 的任务 |
| `ready` | 已到 scheduled 的任务 |
| `overdue` | 已过 due 的任务 |
| `blocked` | 被依赖阻塞的任务 |
| `blocking` | 正在阻塞其他任务的任务 |

## 修改任务

```bash
xuanchu 1 modify priority:H +next
xuanchu 1 modify due:
xuanchu 1 modify project:agentapi
xuanchu 1 modify +@alice -@bob
```

`key:` 表示清空字段，例如 `due:` 清空 due。
`@ref` 只用于 `add`；`+@ref` / `-@ref` 用于 `modify`。`assignee:me` 只表示当前 effective workspace 中的当前 actor。

## 完成和删除

```bash
xuanchu 1 done
xuanchu 1 delete
```

`delete` 是软删除。要查看 deleted 任务：

```bash
xuanchu deleted
```

## Start / Stop

```bash
xuanchu 1 start
xuanchu active
xuanchu 1 stop
```

`start` 会写入 `start` 字段，任务进入 active 报表，并影响 urgency。

## 注释

```bash
xuanchu 1 annotate "called vendor, waiting for reply"
xuanchu _get 1.annotations
xuanchu 1 denotate <annotation-id>
```

annotation 包含稳定 ID、时间戳和描述。删除 annotation 时必须使用 annotation ID；可以通过 `xuanchu _get 1.annotations`、`xuanchu 1 annotations` 或 JSON 输出中的 `annotations[].id` 获取。

## 描述编辑

```bash
xuanchu 1 append "with examples"
xuanchu 1 prepend "[draft]"
xuanchu 1 edit
```

`edit` 会打开 `$EDITOR`，以 JSON 形式编辑任务。保存后 xuanchu 会校验字段；非法日期、非法 status、换行 annotation 等不会写回。

远程 CLI 不支持 `edit`，因为它依赖本机编辑器。

## 依赖

```bash
xuanchu add "Prepare API" project:agentapi
xuanchu add "Write docs" project:agentapi depends:<uuid-or-id>
xuanchu blocked
xuanchu blocking
```

被其他 pending 任务依赖的任务会出现在 `blocking`。依赖未完成的任务会出现在 `blocked`。

## Links

为任务添加外部关联链接（文档、PR、工单等）：

```bash
xuanchu link add <task-ref> --type document --url https://... --title "设计文档"
xuanchu link list <task-ref>
xuanchu link remove <task-ref> --link-id <link-id>
```

## 循环任务

```bash
xuanchu add "Submit weekly report" project:agentapi recur:weekly due:2030-01-05 until:2030-02-01
xuanchu list
xuanchu 1 done
xuanchu list
```

当前支持：

- `daily`
- `weekly`
- `monthly`
- `<N>days`
- `<N>weeks`
- `<N>months`

循环任务约束：

- recurring parent 默认隐藏。
- 创建 parent 时会立即生成第一个 child。
- 完成 child 后自动生成下一个 child。
- `until` 会阻止生成超过截止时间的新 child。
- recurring parent 不接受 `wait`、`scheduled`、`depends`。
- `monthly` 使用 Go `time.AddDate(0, n, 0)` 的月末滚动语义。

## 日期语义

`due:` 和 `end:` 表示某天截止或结束，写入时会落在当地时区当天 `23:59:59`。

```bash
xuanchu add "deadline" project:agentapi due:2030-01-01
```

查询 `due:2030-01-01` 表示这个自然日范围，而不是只匹配零点。
