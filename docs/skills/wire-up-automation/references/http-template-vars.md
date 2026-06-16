# HTTP Template 模板变量

`http_template` sink 的 URL、header、body 模板用 `{{变量名}}` 引用。变量分两组：endpoint 模板（URL）白名单较严，body/header 模板可用范围更广。

## endpoint 模板白名单（URL）

endpoint template（`endpoint_mode: "template"` 的 `url_template`）只允许以下变量：

| 变量 | 说明 |
|---|---|
| `workspace.id` / `workspace.slug` | 工作区 |
| `project.id` / `project.slug` | 项目 |
| `rule.id` / `rule.name` | 规则 |
| `recipient.id` | 接收者 |
| `event.id` / `event.type` / `event.object_kind` / `event.object_id` | 事件 |
| `actor.id` | 操作者 |
| `delivery.id` / `delivery.attempt` / `delivery.workspace_id` / `delivery.sink_id` | 投递 |

> **禁止**：endpoint template 中使用 `secret.*` 和 `task.*` 会报错（`endpoint_template_invalid`）。

## body / header 模板变量

body 和 header 模板可用变量更丰富：

- `workspace.id` / `workspace.slug`
- `project.id` / `project.slug`
- `task.uuid` / `task.task_slug` / `task.description` / `task.due`
- `recipient.id` / `recipient.name` / `recipient.email`
- `reminder.sequence` / `reminder.overdue_sequence` / `reminder.window_start` / `reminder.window_end`
- `secret.<alias>` — 通过 `secret_refs` 声明别名引用 secret config

## secret 引用

secret 不要直接写入 URL/body。在 sink 的 `secret_refs` 中声明别名到 config key 的映射，模板里用 `{{secret.<别名>}}` 引用：

```json
"secret_refs": [
  {"alias": "feishu_bot_token", "config_key": "integrations.feishu.bot_token"}
]
```

模板中：`"Authorization": "Bearer {{secret.feishu_bot_token}}"`

## body_content_type

`http_template` sink 的 `body_content_type` 常用 `application/json`。body 模板需产出合法的对应格式（如 JSON 模板里字符串需正确转义引号）。
