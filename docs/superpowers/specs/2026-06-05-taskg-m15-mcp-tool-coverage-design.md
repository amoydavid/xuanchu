# M15：MCP Tool 全量覆盖

## 1. 目标

把 CLI/HTTP 已有但 MCP 未暴露的 41 项操作全部补齐为 MCP tool，使 Agent 通过 MCP 即可完成所有 CLI/HTTP 能完成的操作。

当前 33 个 MCP tool → 补齐后 74 个。

## 2. 现状

当前 MCP tool 覆盖情况：

| 域 | 已有 | 缺失 |
|---|---|---|
| Task | 10（add/query/get/modify/done/delete/start/stop/annotate/depends/link_add/link_remove） | 4（denotate/link_list/export/import） |
| Project | 8（list/get/get_current/annotate/denotate/list_annotations/list_timeline） | 6（add/modify/archive/config_set/config_unset/config_list） |
| Workspace | 2（list/get_current） | 5（add/info/modify/archive/use） |
| User | 4（list/get/bind/unbind） | 3（add/use/list_external_ids） |
| Member | 2（list/add） | 1（role） |
| Context | 2（get/set） | 3（none/list/delete） |
| Config | 2（get/set） | 2（unset/list） |
| Hook | 0 | 10（list/add/info/modify/delete/enable/disable/list_deliveries/delivery_info/delivery_replay） |
| Token | 0 | 4（create/list/modify/revoke） |
| Audit | 0 | 1（list） |
| Scope | 0 | 1（list） |
| Me | 0 | 1（get） |

## 3. 设计原则

1. **纯薄壳**：每个 MCP tool 只做三件事——鉴权（`serviceForTool`）、调用 app service 方法、格式化输出。不引入任何业务逻辑。
2. **复用 app service**：所有 tool 直接调用 `internal/app` 已有方法。不需要新增 app 方法。
3. **遵循现有模式**：Input struct + `addTool` + `serviceForTool` + `successWithEnvelope` / `businessErrorWithEnvelope`。
4. **权限映射**：读操作对应 `*:read` scope + `PermissionXxxRead`，写操作对应 `*:write` scope + `PermissionXxxWrite`/`PermissionXxxManage`。Hook 和 Token 使用现有权限常量。
5. **命名规范**：`{资源}_{动作}`，用 `_` 分隔。读子资源用 `list` 前缀。

## 4. 新增 Tool 清单

### 4.1 Task 补充（4 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `task_denotate` | task:write | PermissionTaskWrite | `Denotate(target, index)` |
| `task_link_list` | task:read | PermissionTaskRead | `Info(target)` → 提取 links |
| `task_export` | task:read | PermissionTaskRead | `ExportWithInput(input)` |
| `task_import` | task:write | PermissionTaskWrite | `Import(tasks)` |

`task_denotate` 的 index 参数：现有 `Denotate(target, index int)` 接受 annotation 的序号。MCP 输入用 `annotation_index` int 字段。

`task_link_list` 复用 `task_get` 模式，返回 task 的 links 字段。可以单独提供一个轻量 tool，也可以从 `task_get` 的结果中提取。考虑到 Agent 明确想查 links 的场景，单独提供更友好。

`task_export` / `task_import` 输入/输出为 JSON task 数组。

### 4.2 Project 补充（6 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `project_add` | project:write | PermissionProjectManage | `AddProject(input)` |
| `project_modify` | project:write | PermissionProjectManage | `ModifyProject(ref, input)` |
| `project_archive` | project:write | PermissionProjectManage | `ArchiveProject(ref)` |
| `project_config_set` | config:write | PermissionProjectManage | `ProjectConfigSet(ref, key, value)` |
| `project_config_unset` | config:write | PermissionProjectManage | `ProjectConfigUnset(ref, key)` |
| `project_config_list` | config:read | PermissionProjectRead | `ProjectConfigList(ref)` |

### 4.3 Workspace 补充（5 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `workspace_add` | workspace:write | PermissionWorkspaceManage | `AddWorkspace(input)` |
| `workspace_info` | workspace:read | PermissionWorkspaceRead | `WorkspaceInfo(ref)` |
| `workspace_modify` | workspace:write | PermissionWorkspaceManage | `ModifyWorkspace(ref, input)` |
| `workspace_archive` | workspace:write | PermissionWorkspaceManage | `ArchiveWorkspace(ref)` |
| `workspace_use` | workspace:write | PermissionWorkspaceManage | `UseWorkspace(ref)` |

### 4.4 User 补充（3 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `user_add` | workspace:write | PermissionWorkspaceManage | `AddUser(input)` |
| `user_use` | workspace:write | PermissionWorkspaceManage | `UseUser(ref)` |
| `user_list_external_ids` | workspace:read | PermissionWorkspaceRead | `ListExternalIDs(userID)` |

`user_add` 需要 workspace admin/owner 权限。`user_use` 切换活跃用户。`user_list_external_ids` 需要先解析 user ref 为 user ID。

### 4.5 Member 补充（1 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `member_role` | workspace:write | PermissionWorkspaceManage | `ChangeMemberRole(input)` |

### 4.6 Context 补充（3 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `context_none` | config:write | PermissionConfigWrite | `ContextNone()` |
| `context_list` | config:read | PermissionConfigRead | `ContextList()` |
| `context_delete` | config:write | PermissionConfigWrite | `ContextDelete(name)` |

### 4.7 Config 补充（2 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `config_unset` | config:write | PermissionConfigWrite | `UnsetConfig(key)` |
| `config_list` | config:read | PermissionConfigRead | `ConfigValues()` |

### 4.8 Hook 全域（10 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `hook_list` | hook:read | PermissionHookRead | `ListHooks(projectRef)` |
| `hook_add` | hook:write | PermissionHookManage | `AddHook(input)` |
| `hook_info` | hook:read | PermissionHookRead | `HookInfo(hookID)` |
| `hook_modify` | hook:write | PermissionHookManage | `ModifyHook(hookID, input)` |
| `hook_delete` | hook:write | PermissionHookManage | `DeleteHook(hookID)` |
| `hook_enable` | hook:write | PermissionHookManage | `EnableHook(hookID)` |
| `hook_disable` | hook:write | PermissionHookManage | `DisableHook(hookID)` |
| `hook_list_deliveries` | hook:read | PermissionHookRead | `ListHookDeliveries(hookID, status, limit)` |
| `hook_delivery_info` | hook:read | PermissionHookRead | `HookDeliveryInfo(deliveryID)` |
| `hook_delivery_replay` | hook:write | PermissionHookManage | `ReplayHookDelivery(deliveryID)` |

Hook 的权限映射需要确认现有常量。Hook 管理（写）需要 admin/owner。Hook 读需要 member 以上。

Hook tool 的 project scope：`hook_list`、`hook_add` 可通过 workspace + project 参数限定范围。其他 hook 操作通过 hookID 直接定位。

`hook_add` 的 secret 字段：MCP tool 不暴露 secret。`HookAddInput` 已有 `SecretStdin` 等字段，MCP 应使用空 secret 或要求通过 CLI 预设。建议 MCP `hook_add` 的 secret 通过 `secret` 字段传入（与 HTTP API 一致），但 MCP 文档应提示安全性。

### 4.9 Token 全域（4 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `token_create` | token:write | PermissionTokenWrite | `CreateToken(input)` |
| `token_list` | token:read | PermissionTokenRead | `ListTokens(input)` |
| `token_modify` | token:write | PermissionTokenWrite | `ModifyToken(input)` |
| `token_revoke` | token:write | PermissionTokenWrite | `RevokeToken(ref)` |

Token tool 不传入 `ParentToken`（MCP 不支持委托校验），由 app 层根据 actor 自动决定。Agent token 可创建子 token 但 scope 不超过自身。

`token_create` 返回明文 token，与 CLI/HTTP 行为一致。MCP 文档应提醒 Agent 妥善保管。

### 4.10 Audit（1 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `audit_list` | audit:read | PermissionAuditRead | `ListAudit(input)` |

### 4.11 Scope（1 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `scope_list` | 无需 scope | 无需 permission | `auth.ScopeRegistryValues()` |

`scope_list` 不需要鉴权。它只返回静态 scope 列表，不访问任何 workspace 数据。直接调用 `auth.ScopeRegistryValues()` 即可。

### 4.12 Me（1 个）

| Tool Name | Scope | Permission | App Method |
|---|---|---|---|
| `me_get` | 无需 scope | 最低权限 | 返回当前 actor 身份信息 |

`me_get` 返回当前 token 绑定的用户信息、workspace、scope 列表。Agent 用它确认自己是谁、在哪个 workspace。不调用 app service，直接从 request scope 的认证结果中提取。

## 5. 权限常量检查

需要确认以下权限常量是否存在：

- `PermissionHookRead` / `PermissionHookManage`
- `PermissionTokenRead` / `PermissionTokenWrite`
- `PermissionAuditRead`
- `PermissionConfigRead` / `PermissionConfigWrite`

如果不存在需要新增。task/user/workspace/project/member/context 的权限常量已存在。

## 6. 不做的事情

- **不新增 app service 方法**：所有 tool 复用现有方法。
- **不暴露 `edit`/`append`/`prepend`**：这些是 CLI 交互式命令，MCP 不适用。
- **不暴露 `token create` 的 `ParentToken` 参数**：MCP 不做委托校验。
- **不暴露 `server`/`mcp stdio`**：基础设施命令，不属于 tool。
- **不暴露 `completion`/`show`/`_version`/`_ids` 等 helper**：脚本化命令，Agent 不需要。

## 7. 文件组织

每个域一个文件，延续现有模式：

| 文件 | 新增/修改 |
|---|---|
| `internal/mcpserver/tools_task.go` | 新增 4 个 tool |
| `internal/mcpserver/tools_project.go` | 新增 6 个 tool |
| `internal/mcpserver/tools_workspace.go` | 新增 5 个 tool |
| `internal/mcpserver/tools_user.go` | 新增 3 个 tool |
| `internal/mcpserver/tools_member.go` | 新增 1 个 tool |
| `internal/mcpserver/tools_context.go` | 新增 3 个 tool |
| `internal/mcpserver/tools_config.go` | 新增 2 个 tool |
| `internal/mcpserver/tools_hook.go` | **新文件**，10 个 tool |
| `internal/mcpserver/tools_token.go` | **新文件**，4 个 tool |
| `internal/mcpserver/tools_audit.go` | **新文件**，1 个 tool |
| `internal/mcpserver/tools_misc.go` | **新文件**，`scope_list` + `me_get` |
| `internal/mcpserver/server.go` | 注册新 tool 组 |
| `docs/manual/mcp.md` | 更新 tool 列表 |

## 8. 验收标准

- 74 个 MCP tool 全部注册并可调用。
- 每个 tool 有 schema golden test。
- 所有写操作经过权限检查。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 通过。
- `docs/manual/mcp.md` 工具列表更新为 74 个。
