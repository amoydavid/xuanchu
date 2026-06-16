# 飞书群机器人端到端配置

CIO 的核心场景：把一个项目对应的飞书群接上通知，任务被认领/完成/due 变更时机器人群里发消息，每天扫描到期/逾期任务。

## 前置：群绑定机制

xuanchu 没有"群/chat"实体。用 **project config 键**记录群信息：

| config key | scope | 说明 |
|---|---|---|
| `integrations.feishu.webhook_url` | project | 本群飞书机器人地址（sink 用 `config_value` 读） |
| `integrations.feishu.bot_token` | workspace/project（secret） | 飞书机器人 token，模板用 `secret_refs` 引用 |
| `im.group_id` | project | 群 ID（CIO agent 自身逻辑用，用 `config_get` scope=project 或 `project_config_list` 读；不在 `project_get` 的 config_summary 白名单内） |

**关键**：一个 sink 被多个 project 共享，投递时按当前 project 解析各自的 webhook URL。这样建一次 sink、给每个 project 各配一个群地址即可。

## Step 1: 定义 config schema

给 `integrations.feishu.webhook_url`、`integrations.feishu.bot_token`、`im.group_id` 定义 schema（schema 规则详见 manage-access-and-config skill）。每个 workspace 只需定义一次。

```json
config_schema_set({
  "workspace": "dajee",
  "key": "integrations.feishu.webhook_url",
  "value_type": "string",
  "allowed_scopes": ["project"],
  "label": "飞书群机器人 webhook 地址"
})

config_schema_set({
  "workspace": "dajee",
  "key": "integrations.feishu.bot_token",
  "value_type": "string",
  "allowed_scopes": ["workspace", "project"],
  "secret": true,
  "label": "飞书机器人 token"
})

config_schema_set({
  "workspace": "dajee",
  "key": "im.group_id",
  "value_type": "string",
  "allowed_scopes": ["project"],
  "label": "IM 群 ID"
})
```

## Step 2: 给 project 配群信息

每个 project 配自己群对应的地址和群 ID。可用 `project_config_set`（等价于 `config_set` + `scope:"project"`）。

```json
project_config_set({
  "workspace": "dajee",
  "project": "apiplat",
  "key": "integrations.feishu.webhook_url",
  "value": "https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxx"
})

project_config_set({
  "workspace": "dajee",
  "project": "apiplat",
  "key": "im.group_id",
  "value": "oc_xxxxxxxxxxxxxxxx"
})

// bot_token 放 workspace 级（整个 workspace 共用一个飞书应用 token）
config_set({
  "workspace": "dajee",
  "scope": "workspace",
  "key": "integrations.feishu.bot_token",
  "value": "t-xxxxxxxxxxxx"
})
```

## Step 3: 建 http_template sink

建一个 workspace 级共享 sink，`endpoint_mode: config_value`，从当前 project 的 config 解析 webhook URL；body 模板渲染飞书消息体。

```json
notification_sink_add({
  "workspace": "dajee",
  "name": "feishu-bot",
  "type": "http_template",
  "endpoint_mode": "config_value",
  "config_key": "integrations.feishu.webhook_url",
  "allowed_hosts": ["open.feishu.cn"],
  "header_templates": [
    {"name": "Content-Type", "value": "application/json"}
  ],
  "body_content_type": "application/json",
  "body_template": "{\"msg_type\":\"text\",\"content\":{\"text\":\"[{{project.slug}}] 任务 {{task.task_slug}} {{event.type}}：{{task.description}}\"}}",
  "max_concurrency": 0
})
```

> 若机器人需要鉴权，加 `secret_refs` 引用 bot_token，并在 header 模板里用 `{{secret.feishu_bot_token}}`。

## Step 4: 建 notification rule（事件通知）

订阅任务相关事件，audience 为 assignees。

```json
notification_rule_add({
  "workspace": "dajee",
  "name": "feishu-task-events",
  "event": "task.assigned",
  "audience": "assignees",
  "sink": "feishu-bot"
})

notification_rule_add({
  "workspace": "dajee",
  "name": "feishu-task-completed",
  "event": "task.completed",
  "audience": "assignees",
  "sink": "feishu-bot"
})

notification_rule_add({
  "workspace": "dajee",
  "name": "feishu-task-due-changed",
  "event": "task.due_changed",
  "audience": "assignees",
  "sink": "feishu-bot"
})
```

## Step 5: 建 reminder rule（每日扫描）

每天 9:00 扫描到期/逾期任务，发给 assignees。

```json
// 到期预警：未来 24 小时内到期、未完成
reminder_rule_add({
  "workspace": "dajee",
  "name": "feishu-due-soon",
  "schedule_type": "daily_at",
  "schedule_value": "08:50",
  "filter_source": "end.isnull and start.isnull and due.after:now and due.before:now+24h",
  "audience_type": "assignees",
  "sink": "feishu-bot"
})

// 逾期通知：已到期、未完成
reminder_rule_add({
  "workspace": "dajee",
  "name": "feishu-overdue",
  "schedule_type": "daily_at",
  "schedule_value": "09:00",
  "filter_source": "status:pending and end.isnull and due.before:now",
  "repeat_policy": "every:24h",
  "audience_type": "assignees",
  "sink": "feishu-bot"
})
```

## 排错

配好后如果群里收不到消息：

1. `notification_delivery_list({"workspace":"dajee","status":"dead_lettered","limit":20})` — 看是否有失败投递
2. `notification_delivery_info({"workspace":"dajee","delivery_id":"..."})` — 看具体失败原因（URL/host/body 格式）
3. 确认 project 是否配了 `integrations.feishu.webhook_url`（用 `config_get` scope=project 读，**不**在 `project_get` 的 config_summary 白名单内）
4. 修复后对 dead-lettered delivery 调 `notification_delivery_replay` 重试（注意：不重新渲染模板，只重发已冻结内容；新内容要等下次触发）
