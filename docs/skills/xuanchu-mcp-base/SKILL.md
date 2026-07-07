---
name: xuanchu-mcp-base
description: 使用璇础 MCP skill 或把文档里的 canonical tool name 映射到真实 Agent 工具名时使用，尤其是运行时工具名带有 mcp server 前缀或双下划线前缀时。
---

# 璇础 MCP 基础约定

本 skill 只定义通用调用规则。业务流程分别见 capture-and-track-work、govern-projects、manage-access-and-config、report-and-review、wire-up-automation。

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
- MCP 接口里的任务引用优先用 UUID 或 `task_slug`，不要使用 CLI working-set 数字 ID。
- `workspace_use`、`user_use`、`context_set` 只影响 stdio MCP 的隐式状态；HTTP MCP 不受影响。默认显式传参。
- 以工具返回的 structured content 为准；rendered text 只用于人读摘要。
