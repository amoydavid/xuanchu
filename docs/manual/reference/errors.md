---
title: "错误码速查"
weight: 210
---

# 错误码速查

本页列出用户最常遇到的错误码。HTTP API 的错误响应形如：

```json
{
  "error": {
    "code": "project_not_found",
    "message": "project not found"
  }
}
```

## 鉴权与权限

| 错误码 | 含义 |
|---|---|
| `auth_missing_token` | 请求缺少 Bearer token |
| `auth_invalid_token` | token 无效 |
| `auth_token_expired` | token 已过期 |
| `auth_token_revoked` | token 已撤销 |
| `permission_denied` | membership role 权限不足 |
| `token_scope_denied` | token capability 不允许该操作 |
| `workspace_scope_denied` | token 不能访问 workspace |
| `project_scope_denied` | token 不能访问 project |

## Workspace / User / Member

| 错误码 | 含义 |
|---|---|
| `workspace_not_found` | workspace 不存在 |
| `workspace_archived` | workspace 已归档 |
| `membership_not_found` | actor 不是 workspace 成员，或 impersonation 目标不存在/不是成员 |
| `user_not_found` | user 不存在 |
| `workspace_required` | impersonation 请求未指定 workspace，且 token 可见多个 workspace |

## Project

| 错误码 | 含义 |
|---|---|
| `project_not_found` | project 不存在，或缺少 project 参数 |
| `project_archived` | project 已归档 |
| `project_already_exists` | project slug 已存在 |
| `project_invalid_slug` | project slug 格式非法 |
| `project_name_required` | project name 不能为空 |
| `project_slug_immutable` | project slug 不能修改 |
| `project_mismatch` | project 与 project_id 不一致 |
| `project_workspace_mismatch` | project 不属于指定 workspace |

## Task

| 错误码 | 含义 |
|---|---|
| `task_not_found` | 任务不存在，或 project-scoped token 无权看到该任务 |
| `task_uuid_invalid` | MCP/HTTP 场景中的 task UUID 无效 |
| `task_clear_field_unknown` | MCP `clear` 字段名未知 |

## Config / Context

| 错误码 | 含义 |
|---|---|
| `config_not_found` | config key 不存在 |
| `config_definition_not_found` | 当前 workspace 下没有该 shared config key 的 schema 定义 |
| `config_scope_not_allowed` | schema 存在，但当前 workspace/project 作用域不允许使用该 key |
| `config_definition_in_use` | 删除 schema 时发现当前 workspace 下仍有对应 workspace/project 值 |
| `config_scope_invalid` | config scope 不合法 |
| `config_value_invalid` | config value 与 schema 类型或枚举约束不匹配 |
| `config_value_too_large` | config value 太大 |
| `project_config_scope_required` | 该 key 只能走 `project config` 入口，不能用无 scope 的 `config` 访问 |
| `context_not_found` | context 不存在 |

## Token

| 错误码 | 含义 |
|---|---|
| `token_not_found` | token 不存在 |
| `token_update_failed` | token 更新失败 |
| `token_name_required` | token name 不能为空 |
| `token_scope_invalid` | token scope 格式非法 |
| `token_project_scope_invalid` | token project scope 非法 |
| `token_workspace_scope_invalid` | token workspace scope 非法 |

## Hook

| 错误码 | 含义 |
|---|---|
| `hook_not_found` | hook 不存在 |
| `hook_name_invalid` | hook name 为空或太长 |
| `hook_scope_invalid` | hook scope 不是 workspace/project |
| `hook_project_required` | project-scoped hook 缺少 project |
| `hook_event_types_invalid` | event type 为空或不支持 |
| `hook_endpoint_invalid` | endpoint URL 非法或被 SSRF 防护拦截 |
| `hook_secret_invalid` | secret 非法或过长 |
| `hook_timeout_invalid` | timeout 不在允许范围 |
| `hook_max_attempts_invalid` | max attempts 不在允许范围 |
| `hook_delivery_not_found` | delivery 不存在 |
| `hook_delivery_not_replayable` | delivery 当前状态不可 replay |

## API / 远程 CLI

| 错误码 | 含义 |
|---|---|
| `api_bad_limit` | limit 非法或超过上限 |
| `api_internal` | 服务端内部错误 |
| `remote_server_invalid` | remote server URL 非法 |
| `remote_unsupported_command` | 远程模式不支持该命令 |
| `server_listen_required` | server 缺少 `--listen` |
