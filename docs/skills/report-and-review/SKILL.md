---
name: report-and-review
description: 在 IM 群里发日报/周报、解释任务 urgency 排序理由、查审计日志排查谁改了或删了什么。用户提到周报、今天该做什么、为什么先做这个、谁删了任务、审计、排查操作记录时使用。
---

# 汇报与审计

CIO agent 在项目群里广播状态、解释排序、排查问题。这是 agent 对外"说话"的出口——把系统状态变成群里可读的汇报。

## 何时使用

当你要发今日待办/逾期汇总、解释某个任务为什么排在前面、或排查"谁动了什么"时，用本 skill。它覆盖 report（报表）、urgency（排序解释）、audit（审计日志）。

## 核心原则

- 所有操作只读，不修改任何数据。
- `report_run` 的 `name` 必填，内置报表名固定（见 references/report-tools.md）；可用 `query` 在报表上叠加过滤。
- `urgency_explain` 回答"为什么这个任务排前面"，返回各因素的贡献值。
- `audit_list` 记录所有写操作，按时间倒序，支持 `actor` 过滤。
- 每次调用显式传 `workspace`；需收窄到 project 时传 `project_id` / `project`。

## 标准工作流

### 场景 A：群里发今日待办汇总

```
1. report_run({"workspace":"dajee","name":"next","limit":10})   // 今日该做的
2. report_run({"workspace":"dajee","name":"overdue"})            // 逾期
3. // CIO 汇总成消息发群
```

### 场景 B：解释排序

用户问"为什么先做这个"。

```
1. report_run({"workspace":"dajee","name":"next","limit":1})          // 拿排第一的
2. urgency_explain({"workspace":"dajee","id":"task-slug"})            // 返回 factors
```

### 场景 C：排查"谁删了任务"

```
1. audit_list({"workspace":"dajee","limit":20,"actor":"bob"})
2. // 过滤 action 为 task.delete 的条目，看 payload 和 target_id
```

## 易错点

- 内置报表名固定：`list`、`next`、`all`、`completed`、`deleted`、`waiting`、`active`、`ready`、`overdue`、`blocked`、`blocking`。写错名字会失败。
- `urgency_explain` 的 `id` 用 UUID 或 `task_slug`，不用数字 ID。
- `audit_list` 的 `action` 值用点号分隔（如 `task.delete`），完整清单见 references/audit-actions.md。
- report 是只读快照，不发通知；要广播到群需配合 wire-up-automation 的 notification/reminder。

## 参考文档

| 文件 | 何时读 |
|---|---|
| references/report-tools.md | report_run / urgency_explain 完整 JSON + 报表名 |
| references/urgency-factors.md | urgency 因素清单与系数配置 |
| references/audit-actions.md | audit action 完整清单 |
