---
name: xuanchu-mcp-base
description: 使用璇础 MCP skill 或把文档里的 canonical tool name 映射到真实 Agent 工具名时使用，尤其是运行时工具名带有 mcp server 前缀或双下划线前缀时。
---

# 璇础 MCP 基础约定

本 skill 只定义通用调用规则。业务流程分别见 xuanchu-capture-and-track-work、xuanchu-govern-projects、xuanchu-manage-access-and-config、xuanchu-report-and-review。通知/Hook/自动化接线（sink、reminder rule、notification rule、project automation）的完整工具说明见 `docs/manual/mcp.md` 的「Hook」「通知与提醒」「Project Automation」三节，端到端流程见 `docs/manual/notifications.md` 与 `docs/manual/hooks.md`。

## 工具名解析

文档中的 `task_add`、`project_get`、`notification_rule_add` 等都是 canonical tool name。真实 Agent 运行时可能会给 MCP 工具加 server 前缀，例如：

| 文档写法 | 真实工具名示例 |
|---|---|
| `task_add` | `task_add` |
| `task_add` | `mcp_xuanchu__task_add` |
| `task_add` | `mcp_xuanchu_task_add` |

调用工具前按当前运行时的实际工具列表解析：

1. 优先使用完全同名的工具，如 `task_add`。
2. 没有完全同名时，查找以 `__<canonical>` 结尾的工具，如 `mcp_xuanchu__task_add`。
3. 仍没有时，查找以 `_<canonical>` 结尾的工具，如 `mcp_xuanchu_task_add`。
4. 如果存在多个候选，优先选择当前配置的璇础 server；不确定时先列出候选并说明歧义，不要臆造工具名。

## 调用规则

- 示例里的 `tool_name({...})` 表示调用对应 MCP tool，不是 shell 命令。
- 每次写入或读取 workspace 数据时显式传 `workspace`；需要收窄到项目时传 `project` 或 `project_id`。
- MCP 接口里的任务引用使用 UUID、已物化任务的 `task_slug` 或循环实例 occurrence_ref；projected 实例只有 occurrence_ref。不要使用 CLI working-set 数字 ID。
- `workspace_use`、`user_use`、`context_set` 只影响 stdio MCP 的隐式状态；HTTP MCP 不受影响。默认显式传参。
- 以工具返回的 structured content 为准；rendered text 只用于人读摘要。

## 任务附件

附件 tool 只暴露 metadata：`task_attachment_list`、`task_attachment_get`、
`task_attachment_rename`、`task_attachment_remove`。**MCP 不提供 base64 upload/download**。

下载附件二进制内容时，使用 `task_attachment_get` 返回的 `content_url`，配合同一 Bearer
token 走 HTTP `GET`：

```text
GET /api/v1/attachments/{attachment_id}/content?workspace=<slug>
Authorization: Bearer <同一调用 token>
```

`task_attachment_remove` 在附件仍被 description 引用时返回 `attachment_in_use`，需要先
删除正文引用并保存。

