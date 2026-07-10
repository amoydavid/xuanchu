# 统一 user 输出结构 + external_id 增加 user_type — 实施记录

日期：2026-07-10
状态：已完成

## 改动清单

### A. external_id 增加 user_type（存储 → 同步 → 序列化）

| 层 | 文件 | 改动 |
|---|---|---|
| 存储 | `internal/storage/models.go` | `UserExternalID` 加 `UserType` 字段，唯一索引 `(provider, external_id)` → `(provider, user_type, external_id)` |
| 领域 | `internal/task/model.go` | `ExternalIDInfo` 加 `UserType` |
| JSON | `internal/task/json.go` | `JSONExternalID` 加 `user_type,omitempty`；`externalIDsToJSON`/`externalIDsFromJSON` 带 UserType |
| 同步 | `internal/auth/directory/client.go` | `Identity` 加 `UserType`；不再过滤非 user_id 类型（open_id/union_id 不再丢弃） |
| 同步 | `internal/app/directory_sync.go` | `syncExtID` 加 `userType` 参数 |
| 手动绑定 | `internal/app/workspace.go` | `BindExternalID` 加 `userType` 参数（默认 `user_id`）；`ListExternalIDs`/`loadExternalIDsByUsers` 带 UserType |
| 存储→领域 | `internal/storage/task_repo.go` | `loadAssigneeUsers` 带 UserType |
| 通知 | `internal/app/notification_scheduler.go` | `schedulerUserInfos` 带 UserType |
| HTTP | `internal/httpapi/users.go` | `bindExternalIDRequest` 加 `UserType`；bind 调用传 `UserType` |
| MCP | `internal/mcpserver/tools_user.go` | `UserBindInput` 加 `UserType`；描述改为 `provider=feishu, user_type=user_id`（不再用 `feishu_user_id`） |
| Remote | `internal/remote/user.go` | `BindExternalID` 加 `userType` 参数 |
| CLI | `internal/cli/user.go` | `parseProviderExternalID` 支持 `provider:user_type:external_id` 三段格式 |

### B. 统一 user 输出结构

| 改动 | 文件 | 说明 |
|---|---|---|
| 收敛 external_id 类型 | 删除 `externalIDView`/`externalIDResponse`/`externalIDDTO`，统一用 `task.JSONExternalID` | `mcpserver/tools_views.go`、`httpapi/users.go`、`httpapi/admin.go`、`remote/user.go` |
| JSONAssignee → JSONUserInfo | `internal/task/json.go` | 删除 `JSONAssignee`，`JSONTask.Assignees` 改为 `[]JSONUserInfo`；assignee 构建复用 `externalIDsToJSON`；wire 字段 `user_id` → `id` |
| memberView 统一 | `internal/mcpserver/tools_views.go` | `user_id` → `id`；补 `display_name` + `external_ids` |
| memberResponse 统一 | `internal/httpapi/workspaces.go` | 同上 |
| MemberView 补字段 | `internal/app/workspace.go` | `MemberView` 加 `ExternalIDs`；`ListMembers`/`memberView()` 加载 external_ids |
| meView 补 display_name | `internal/mcpserver/tools_misc.go` | `meView` 加 `DisplayName` |

### C. 测试与文档

- 更新 `internal/task/json_test.go`、`internal/app/hook_test.go`、`internal/app/service_test.go`、`internal/httpapi/auth_test.go`、`internal/httpapi/users_test.go`：`user_id` → `id`、`JSONAssignee` → `JSONUserInfo`、`BindExternalID` 加 user_type。
- 更新 `internal/auth/directory/client_test.go`：断言 `Identity.UserType` 保留；新增 open_id 类型不再被丢弃的覆盖。
- 更新 MCP golden 文件（`testdata/*.schema.json`、`list-tools-default.json`）。
- 更新 `AGENTS.md` 第 10 节「用户信息输出规范」。

## 验证

```bash
go test ./...                     # 全部通过
CGO_ENABLED=0 go test ./...       # 全部通过
CGO_ENABLED=0 go build ./cmd/xuanchu  # 通过
go vet ./...                      # 通过
```

重点关注：
- `internal/task/json_test.go` — assignee 序列化/反序列化（`id` 字段）
- `internal/storage/` — AutoMigrate 加 user_type 列 + 重建唯一索引
- `internal/auth/directory/` — user_type 保留、open_id 不再丢弃
- `tests/integration/` — CLI 黑盒
- MCP schema golden 文件
