# Task Query 表达式

`task_query` 的 `query` 参数、reminder rule 的 `filter_source`、notification rule 的 `filter` 都使用璇础任务过滤表达式。本语法仅参考 Taskwarrior 的设计思路，不承诺兼容其命令或 JSON 格式。

## 字段过滤

| 写法 | 含义 |
|---|---|
| `title:关键词` | 标题子串匹配 |
| `description:关键词` | 详细描述子串匹配 |
| `关键词`（裸字符串） | 自动按 title 子串匹配 |
| `tag:xxx` | 含某标签 |
| `assignee:me` | 分配给当前用户 |
| `assignee:<用户名>` | 分配给指定用户 |
| `project:apiplat` | 属某 project |
| `priority:H` | 某优先级（H/M/L） |
| `due.before:today` / `due.before:now` | due 早于今天/现在 |
| `due.before:now+24h` | due 在未来 24 小时内 |
| `status:pending` / `status:waiting` / `status:completed` / `status:deleted` | 状态过滤（waiting = 等待中，如设了未来 wait；deleted 需显式查询） |
| `annotations contains "备注文本"` | 注释含某文本 |
| `+urgent` | 含某标签（标签简写） |

## 字段存在性

| 写法 | 含义 |
|---|---|
| `start.isnull` | 未开始（start 为空） |
| `start.notnull` | 已开始 |
| `end.isnull` | 未结束 |

## 组合

- `and` / `or` / `not`
- 括号分组：`(status:pending or status:waiting) and priority:H`

## 在 reminder / notification rule 中

`filter_source` 常用模式：

```
// 未开始、未结束、未来 24 小时内到期
end.isnull and start.isnull and due.after:now and due.before:now+24h

// 已到期、未完成
status:pending and end.isnull and due.before:now

// 高优先级或带 urgent 标签
priority:H or +urgent
```

`status:pending and end.isnull` 覆盖未完成任务；区分未开始/进行中可叠加 `start.isnull` / `start.notnull`。

## duration

`now+24h` / `now-2h` 中的 duration 用 Go `time.ParseDuration`：

- ✅ 支持：`24h`、`90m`、`2h30m`
- ❌ 不支持：`1d`（必须写成 `24h`）

`repeat_policy` 的 `every:<duration>` 同样受此约束。
