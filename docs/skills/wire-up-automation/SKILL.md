---
name: wire-up-automation
description: 把项目 IM 群接上飞书机器人通知、设定时到期/逾期提醒、配置事件 webhook、排查和重试投递失败。用户提到通知、提醒、飞书机器人、webhook、hook、群消息、投递失败、重试时使用。
---

# 通知/提醒/Hook 接线

CIO agent 把每个项目对应的 IM 群（外部飞书群）接上通知，让任务事件、到期提醒能自动投递到群里。

## 何时使用

当你要给项目群配飞书机器人、设到期/逾期提醒、配事件 webhook、或排查投递失败时，用本 skill。它覆盖 notification sink、reminder rule、notification rule、hook、delivery 五类资源。

## 核心原则

- **每次调用都显式传 `workspace`**，不依赖隐式状态。
- **三层模型**：sink = 投递目标（`webhook` / `http_template`）；rule = 规则（`reminder` 定时扫描 vs `notification` 事件触发）；hook = 出站集成。三者都通过 sink 实际投递。
- sink 是 workspace 级资源引用（可用名称或 ID），**不能跨 workspace 引用**。
- **secret 走 `secret_refs`**，不直接写 URL/body。HTTP template 中通过别名引用 secret config。
- 投递失败用 `notification_delivery_list(status:"dead_lettered")` 查看，`notification_delivery_replay` 重试（仅 dead-lettered/skipped 可重试，且不重新渲染 URL/header/body）。
- config schema 的定义规则详见 manage-access-and-config skill；本 skill 直接演示飞书集成的完整流程。

## 标准工作流

### 场景 A：给项目群接飞书机器人通知（CIO 核心）

把一个项目对应的飞书群接上通知：任务被认领/完成/due 变更时机器人发消息，每天扫描到期/逾期任务。完整 JSON 见 references/feishu-bot-setup.md。

```
1. config_schema_set 定义 integrations.feishu.webhook_url（project scope）和 im.group_id
2. project_config_set 给本项目配本群 webhook URL 和群 ID
3. notification_sink_add 建 http_template sink，endpoint_mode:config_value，config_key 指向 webhook_url
4. notification_rule_add 订阅 task.assigned / task.completed / task.due_changed 等事件
5. reminder_rule_add 设每日到期/逾期扫描（schedule_type:daily_at）
```

### 场景 B：排查投递失败

```
1. notification_delivery_list({"workspace":"dajee","status":"dead_lettered","limit":20})
2. notification_delivery_info({"workspace":"dajee","delivery_id":"..."})  // 看失败原因
3. notification_delivery_replay({"workspace":"dajee","delivery_id":"..."})  // 重试，不重新渲染
```

## 易错点

- **`notification_delivery_replay` 不重新渲染** URL/header/body——delivery 生成时已冻结。改模板后只影响之后的新投递。
- reminder 优先用 `schedule_type` + `schedule_value` + `filter_source`（新）；`trigger_type`/`offset_seconds`/`after_seconds` 只作兼容路径。
- **duration 不支持 `1d`**：`filter_source` 里 `now+24h` / `now-2h` 用 Go `time.ParseDuration`，支持 `24h`/`90m`/`2h30m`，不支持 `1d`。
- dispatcher 默认 `max_concurrency=1`；sink `max_concurrency=0` 表示继承 dispatcher 默认 sink 并发，显式大于 0 才限制。
- `assignees` / `assignees_and_explicit_users` audience 只支持 `task.*` 事件；project 事件没有 assignee 语义，用 `actor` 或 `explicit_users`。
- `start` 只触发 `task.started`，`stop` 只触发 `task.stopped`——不要只靠 `task.modified` 捕获开始/停止。
- **字段名差一字**：reminder rule 用 `audience_type`，notification rule 用 `audience`，二者同义。
- notification rule 的 `recipients` 在 audience 为 `explicit_users` / `assignees_and_explicit_users` 时必填。
- `repeat_policy` 用 `every:<duration>` 格式（如 `every:24h`），duration 同样受 Go `time.ParseDuration` 约束，**不支持 `1d`**。

## 参考文档

| 文件 | 何时读 |
|---|---|
| references/notification-tools.md | sink/rule/delivery 操作完整 JSON |
| references/hook-tools.md | hook 操作完整 JSON |
| references/event-types.md | 可订阅事件完整清单 |
| references/http-template-vars.md | 模板变量白名单与用法 |
| references/feishu-bot-setup.md | 飞书群机器人端到端配置示例 |
