# HTTP Template 模板变量

`http_template` sink 的 URL、header、body 模板用 `{{变量名}}` 引用。变量由系统在投递时按当前事件/任务/接收者注入。

## body / header 模板变量（完整白名单）

body 和 header 模板可用以下全部变量：

| 分组 | 变量 | 说明 |
|---|---|---|
| 工作区 | `workspace.id` / `workspace.slug` | 当前 workspace |
| 项目 | `project.id` / `project.slug` | 当前 project（无 project 时报错 `template_unresolved`） |
| 规则 | `rule.id` / `rule.name` | 触发的 rule |
| 接收者 | `recipient.id` | 接收用户 ID |
| 接收者 | `recipient.external_ids.<provider>` | 接收者外部 ID，按 provider 取，如 `recipient.external_ids.feishu_user_id` |
| 操作者 | `actor.id` / `actor.name` | 触发事件的用户（事件通知才有） |
| 事件 | `event.id` / `event.type` / `event.version` / `event.occurred_at` / `event.object_kind` / `event.object_id` / `event.json` | 事件上下文（事件通知才有；`event.json` 是整条事件 JSON） |
| 投递 | `delivery.id` / `delivery.attempt` / `delivery.workspace_id` / `delivery.sink_id` | 本次投递 |
| 对象 | `object.kind` / `object.id` | 事件对象 |
| 任务 | `task.uuid` / `task.task_slug` / `task.description` / `task.status` / `task.due` | 任务字段（`task.due` 为空时输出空串） |
| 提醒 | `reminder.sequence` / `reminder.overdue_sequence` / `reminder.window_start` / `reminder.window_end` | 提醒上下文（reminder 才有） |
| 密钥 | `secret.<alias>` | 通过 `secret_refs` 声明别名引用 secret config |

> reminder delivery 没有事件/操作者上下文，`event.*` / `actor.*` 为空；事件通知 delivery 没有提醒上下文，`reminder.*` 为空。按场景取用。

## endpoint 模板变量（URL，更严）

endpoint template（`endpoint_mode: "template"` 的 `url_template`）是更窄的白名单。**禁止 `secret.*` 和 `task.*`**（会报 `endpoint_template_invalid`）。

允许：`workspace.id` / `workspace.slug`、`project.id` / `project.slug`、`rule.id` / `rule.name`、`recipient.id`、`event.id` / `event.type` / `event.object_kind` / `event.object_id`、`actor.id`、`delivery.id` / `delivery.attempt` / `delivery.workspace_id` / `delivery.sink_id`。

## secret 引用

secret 不要直接写 URL/body。在 sink 的 `secret_refs` 中声明别名到 config key 的映射，body/header 模板里用 `{{secret.<别名>}}` 引用（endpoint 模板禁止 secret）：

```json
"secret_refs": [
  {"alias": "feishu_bot_token", "config_key": "integrations.feishu.bot_token"}
]
```

模板中：`"Authorization": "Bearer {{secret.feishu_bot_token}}"`

未在 `secret_refs` 声明的别名、或 secret config 无值，都会报 `template_unresolved`。

## 未识别变量

不在白名单内的 `{{变量}}` 会报 `template_unresolved`（template variable is unsupported）。

## body_content_type

`http_template` sink 的 `body_content_type` 常用 `application/json`。系统会对 `application/json` 的 body 模板做格式校验（用占位符替换后必须仍是合法 JSON），所以 JSON 模板里要正确转义引号。
