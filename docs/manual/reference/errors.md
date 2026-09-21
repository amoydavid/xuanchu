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
| `task_ref_invalid` | MCP/HTTP 场景中的任务引用无效，或传入了纯数字 working-set ID |
| `task_uuid_invalid` | 旧版 task UUID 格式错误码；新协议入口优先使用 `task_ref_invalid` |
| `task_clear_field_unknown` | MCP `clear` 字段名未知 |

## Config / Context

| 错误码 | 含义 |
|---|---|
| `config_not_found` | config key 不存在 |
| `config_definition_not_found` | 当前 workspace 下没有该 shared config key 的 schema 定义 |
| `config_scope_not_allowed` | schema 存在，但当前 workspace/project 作用域不允许使用该 key |
| `config_definition_in_use` | 删除 schema 时发现当前 workspace 下仍有对应 workspace/project 值 |
| `config_definition_type_locked` | 已有配置值时修改 schema 的 value_type 被拒绝 |
| `config_definition_scope_locked` | 移除仍存在配置值的 workspace/project scope 被拒绝 |
| `config_definition_enum_locked` | 新枚举不包含已有配置值被拒绝 |
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
| `hook_sink_required` | hook 缺少 workspace 级 sink 引用 |
| `hook_url_not_supported` | hook 使用 sink 模型，不支持直接传 `url` 或 `endpoint_url` |
| `hook_timeout_invalid` | timeout 不在允许范围 |
| `hook_max_attempts_invalid` | max attempts 不在允许范围 |
| `hook_delivery_not_found` | delivery 不存在 |
| `hook_delivery_not_replayable` | delivery 当前状态不可 replay |

## Notification / Reminder

| 错误码 | 含义 |
|---|---|
| `notification_sink_invalid` | notification sink 参数非法，例如缺少 URL、动态 endpoint 缺少 allowed host、HTTP template body JSON 非法 |
| `notification_sink_not_found` | notification sink 不存在，或不属于当前 workspace |
| `notification_rule_invalid` | notification rule 参数非法，例如事件类型、filter 或 audience 不支持 |
| `notification_rule_not_found` | notification rule 不存在，或不属于当前 workspace/project scope |
| `notification_rule_url_not_supported` | notification rule 使用 sink 模型，不支持直接传 URL |
| `reminder_rule_invalid` | reminder rule 参数非法，例如 `due_before` 缺少正数 offset、trigger/audience/repeat 不支持 |
| `reminder_rule_not_found` | reminder rule 不存在，或不属于当前 workspace/project scope |
| `audience_unsupported` | 当前版本不支持该 reminder audience，例如 project owner/maintainer |
| `audience_unsupported_for_event` | 当前事件类型不支持所选 notification rule audience |
| `notification_delivery_not_found` | notification delivery 不存在，或不属于当前 workspace |
| `notification_delivery_not_replayable` | notification delivery 当前状态不可 replay |
| `endpoint_template_invalid` | 动态 endpoint 模板非法，例如引用了 secret 变量 |
| `endpoint_unresolved` | endpoint 无法解析，例如 config_value 模式缺少配置值或 URL 非法 |
| `endpoint_mode_invalid` | endpoint mode 不支持 |
| `endpoint_host_denied` | endpoint host 不在 allowed host 列表内，或被出站网络防护拒绝 |

## API / 远程 CLI

| 错误码 | 含义 |
|---|---|
| `api_bad_limit` | limit 非法或超过上限 |
| `api_internal` | 服务端内部错误 |
| `remote_server_invalid` | remote server URL 非法 |
| `remote_unsupported_command` | 远程模式不支持该命令 |
| `server_listen_required` | server 缺少 `--listen` |

## 循环任务系列

| 错误码 | 含义 |
|---|---|
| `task_series_not_found` | 循环系列不存在 |
| `task_series_invalid` | 系列参数非法 |
| `task_series_invalid_rule` | 循环规则非法 |
| `task_series_due_required` | 缺少首次截止日期 |
| `task_series_invalid_until` | 循环结束日期非法 |
| `task_series_invalid_effective_from` | 新规则生效日期非法 |
| `task_series_invalid_clear` | `--clear` 字段名不支持 |
| `task_series_unsupported_field` | 该字段不允许在系列上修改 |
| `task_series_occurrence_not_found` | 循环实例不存在 |
| `task_series_endpoint_required` | 循环任务必须使用 `task_series_*` 入口，普通 task 端点拒绝 |
| `task_occurrence_not_found` | occurrence_ref 对应的实例不存在 |
| `task_occurrence_project_immutable` | 循环实例不能改 project |
| `task_occurrence_range_required` | 展开循环实例缺少时间范围 |
| `task_occurrence_range_too_large` | 展开时间范围超过上限 |

## 任务附件

| 错误码 | 含义 |
|---|---|
| `attachment_not_found` | 附件不存在 |
| `attachment_in_use` | 附件仍被 description 引用，需先删除引用 |
| `attachment_quota_exceeded` | 附件大小或数量超过配额 |
| `attachment_state_invalid` | 附件状态不允许该操作 |
| `attachment_image_invalid` | 图片附件内容非法 |
| `attachment_storage_unavailable` | 附件存储后端不可用 |
| `attachment_remote_url_invalid` | 远程抓取 URL 非法 |
| `attachment_remote_fetch_disabled` | 远程抓取未启用 |
| `attachment_remote_fetch_failed` | 远程抓取失败 |
| `attachment_draft_creator_mismatch` | 草稿附件只能由创建者操作 |

## 项目模板

| 错误码 | 含义 |
|---|---|
| `project_template_not_found` | 模板不存在 |
| `project_template_archived` | 模板已归档 |
| `project_template_key_conflict` | 同标识 active 模板冲突 |
| `project_template_key_invalid` | 模板标识非法 |
| `project_template_invalid_name` | 模板名称非法 |
| `project_template_snapshot_not_found` | Snapshot 不存在 |
| `project_template_snapshot_invalid` | Snapshot 内容非法 |
| `project_template_snapshot_hash_mismatch` | current Snapshot 已变化，需重新 list 确认 |
| `project_template_snapshot_schema_unsupported` | Snapshot schema 版本不受支持 |
| `project_template_snapshot_version_conflict` | Snapshot 版本冲突 |
| `project_template_candidate_invalid` | 候选项选择非法 |
| `project_template_candidate_limit_exceeded` | 候选项数量超过上限 |
| `project_template_selection_invalid` | 选择内容非法 |
| `project_template_dependency_missing` | 自动化依赖的 config 未闭包 |
| `project_template_config_invalid` | 模板内 config 非法 |
| `project_template_config_policy_invalid` | config 策略（fixed/inherit/prompt）非法 |
| `project_template_config_input_required` | 必填 prompt 未填写 |
| `project_template_config_input_invalid` | config_inputs 含未声明的 key 或值非法 |
| `project_template_secret_required` | 必填 secret prompt 未提供 |
| `project_template_secret_copy_unavailable` | 快照内 secret_copy 无法解密复制 |
| `project_template_member_unavailable` | 快照成员在当前 workspace 不可用且未提供替换 |
| `project_template_date_out_of_range` | 快照内日期超出允许范围 |
| `project_template_ref_cycle` | 任务依赖在快照内成环 |
| `project_template_uda_invalid` | 快照内 UDA 值与当前定义不兼容 |
| `project_template_automation_invalid` | 快照内自动化规则非法 |
| `project_template_attachment_unsupported` | 模板不支持捕获附件 |
| `project_template_series_schedule_confirmation_required` | 含 schedule 的 Series 需显式确认 |
| `project_template_source_changed` | 来源项目在 Capture 期间发生变化 |
| `project_template_concurrency_conflict` | 并发写冲突，需重试 |
| `project_template_no_changes` | 追加 Snapshot 与 current 无差异 |
| `project_template_status_unchanged` | 模板状态未变化 |

## 自动化（Automation）

| 错误码 | 含义 |
|---|---|
| `automation_rule_not_found` | 自动化规则不存在 |
| `automation_rule_invalid` | 规则参数非法（trigger、模板、scope 等） |
| `automation_scope_invalid` | scope 非法或越权（workspace 规则需 workspace scope） |
| `automation_delivery_not_found` | 投递记录不存在 |
| `automation_delivery_url_missing` | 投递缺少目标 URL |
| `automation_delivery_body_missing` | 投递缺少请求体 |
| `automation_provider_config_missing` | Agent Provider 配置缺失（`agent.provider.*`） |
| `automation_provider_config_invalid` | Provider 配置非法 |
| `automation_provider_allowed_hosts_invalid` | Provider allowed hosts 配置非法 |
| `automation_provider_target_denied` | Provider 目标地址被出站防护拒绝 |

## UDA / 自定义字段

| 错误码 | 含义 |
|---|---|
| `uda_not_defined` | 字段未在 workspace UDADefinition 中定义 |
| `uda_definition_invalid` | 字段定义非法（name/type/values） |
| `uda_value_invalid` | 字段值与定义类型不兼容 |
| `uda_orphan_readonly` | 孤儿字段值只读，需先补定义 |
| `uda_runtime_readonly` | 运行时字段不允许写入 |
| `uda_active_series_in_use` | 字段仍被活跃循环系列使用 |
| `uda_active_series_incompatible` | 字段定义变更与活跃系列值不兼容 |

## SSO / 目录同步 / Admin / Tenant

| 错误码 | 含义 |
|---|---|
| `identity_not_found` | OIDC sub 未命中本地用户映射 |
| `id_token_invalid` | OIDC id_token 校验失败 |
| `sso_failed` / `sso_start_failed` | SSO 流程失败 / 发起失败 |
| `session_expired` | browser session 过期 |
| `csrf_invalid` | 缺少或不匹配的 `X-Xuanchu-CSRF` 头 |
| `directory_sync_failed` | 通讯录同步失败 |
| `directory_token_fetch_failed` | 目录同步换取访问 token 失败 |
| `job_not_found` | 同步任务不存在 |
| `login_required` | 需要登录 |
| `token_web_login_disabled` | SSO 创建的 token 不允许用于 Console 登录 |
| `token_secret_unavailable` | token 密文不可恢复，需重新签发 |
| `config_secret_key_missing` / `config_secret_key_invalid` | 服务端缺少或非法的 `[security].config_secret_key` |
| `admin_auth_required` / `admin_auth_invalid` | admin 接口缺少或非法凭证 |
| `admin_setup_required` / `admin_setup_invalid` | 需要或非法的 setup-code |
| `admin_acting_session_expired` / `admin_acting_session_not_found` | acting session 过期或不存在 |
| `admin_acting_target_invalid` | acting 目标 workspace/用户非法 |
| `tenant_token_not_found` / `tenant_token_expired` / `tenant_token_revoked` | 租户 token 不存在 / 过期 / 已吊销 |
| `tenant_token_scope_invalid` / `tenant_token_workspace_invalid` / `tenant_token_project_scope_invalid` | 租户 token scope 或 allowlist 非法 |
| `tenant_token_management_denied` | 无权管理租户 token |
| `tenant_actor_not_user` | 租户 token 没有自然人 principal，不支持该操作 |

## 导入导出 / 其他

| 错误码 | 含义 |
|---|---|
| `task_import_invalid_schema` | 导入 bundle schema 不支持 |
| `task_import_invalid_status` | 导入任务状态非法 |
| `task_bundle_invalid_schema` | bundle schema 不支持 |
| `task_bundle_series_missing` | bundle 引用的系列缺失 |
| `task_bundle_series_project_mismatch` | bundle 系列与项目不匹配 |
| `task_query_date_invalid` | 查询日期参数非法 |
| `task_type_invalid` | task_type 取值非法 |
| `assignee_not_found` | assignee 引用无法解析为用户 |
| `annotation_not_found` / `annotation_conflict` / `annotation_content_required` | 注解不存在 / 冲突 / 内容为空 |
| `link_not_found` / `link_type_required` / `link_url_required` | 链接不存在 / 缺类型 / 缺 URL |
| `route_not_found` | HTTP 路由不存在 |
| `sqlite_busy` / `sqlite_locked` | SQLite 忙或锁冲突，可重试 |
| `activity_unavailable` | 活动时间线暂不可用 |
