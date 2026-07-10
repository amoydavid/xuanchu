# 统一 user 输出结构 + external_id 增加 user_type

日期：2026-07-10
状态：已实施

## 背景

项目早期（M0/M1）对外输出了 11+ 个 user 相关 JSON 结构，分散在 task / MCP / HTTP / remote 四层，存在三类问题：

1. **user 对象 id 字段不统一**：`task.JSONAssignee` 和 `memberView`/`memberResponse` 用 `user_id`，其余 13 处用 `id`。
2. **结构重复**：external_id 元素结构（provider + external_id）在四层各定义了一份（`JSONExternalID` / `externalIDView` / `externalIDResponse` / `externalIDDTO`），字段完全相同；`userResponse` 与 `adminUserResponse` 几乎重复。
3. **external_id 缺 user_type**：飞书等 IM 平台一个用户有多种 id（user_id / open_id / union_id），数值不重叠、不可互换。当前存储层只存 `provider="feishu"` + 值，没存这是哪种 id。directory client 在 `auth/directory/client.go:107` 把 user_type 当过滤器直接丢弃（只保留 user_id，丢弃 open_id/union_id）。

附带问题：`tools_user.go` 手动绑定路径建议用 `provider=feishu_user_id`（把 id 种类编进 provider），与 directory 同步写入的 `provider=feishu` 不一致，两者不可互换。

## 目标

- 所有 user 输出对象统一形态：`id`（不是 `user_id`）+ `name` + `display_name` + `email` + `external_ids`。
- external_id 元素统一为 `{provider, user_type, external_id}`，新增 `user_type` 区分同 provider 下的 id 种类。
- provider 只写 IM 平台名，不再把 id 种类编进 provider。
- 收敛重复的 external_id 类型定义到单一 `task.JSONExternalID`。

## 决策

- **直接改（breaking）**：`assignees[].user_id` → `id`、`member.user_id` → `id`。项目早期，webhook 消费者主要是内部系统，不保留兼容期（符合 AGENTS.md「不保留临时兼容」）。
- **provider + user_type 分开字段**：不把 user_type 编进 provider key（`feishu_user_id`），而是保持 `provider=feishu` + 独立 `user_type=user_id` 字段。
- **一次全做**：external_id + user_type 和 user 结构统一在同一个改动里完成。

## 影响

- **Webhook payload**：`assignees[].user_id` → `assignees[].id`。内部系统需同步更新。
- **Task import/export JSON**：含 `"user_id"` 的旧导出文件无法导入（字符串简写 `"assignees":["user-1"]` 仍可用）。
- **唯一索引变更**：`user_external_ids` 表 `(provider, external_id)` → `(provider, user_type, external_id)`。AutoMigrate 加列 + 重建索引，历史数据 user_type 默认回填为 `user_id`。
