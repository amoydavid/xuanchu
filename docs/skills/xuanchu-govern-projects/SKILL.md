---
name: xuanchu-govern-projects
description: 建立和归档项目、从已有模板创建项目、给项目分配成员、绑定飞书身份、查看项目时间线和成员结构、巡查项目健康。用户提到新建项目、从模板创建、开群、加人、分派角色、绑定外部身份、归档项目、看项目动态时使用。
---

# 项目与团队治理

围绕 workspace 建立项目结构、维护成员关系、绑定外部身份，并巡查项目健康。项目是 workspace 下的工作单元，workspace 是最高隔离边界。

## 何时使用

当你要新建项目、把人加进项目、绑定飞书身份、归档项目、查看项目动态或巡查团队结构时，用本 skill。它覆盖 workspace、project、user、member 四类资源。

## 核心原则

- **先遵循 `xuanchu-mcp-base` 的工具名解析规则。** 本文中的 `project_add` 等是 canonical tool name，真实运行时可能带 MCP server 前缀。
- **每次调用都显式传 `workspace`。** 不要依赖隐式状态；用参数定位，不用 `workspace_use`/`user_use` 切换隐式上下文（那两个只影响 stdio MCP）。
- project 用 `project`(slug) 或 `project_id`(UUID) 定位。不知道 project_id 时先 `project_list`。
- MCP 只提供 `project_template_list` 和 `project_template_instantiate`。模板 Capture、候选项筛选、Preview、归档和版本治理只能在 Web Console 完成，Agent 不要尝试寻找或拼造治理 tool。
- 成员操作必须指定 `workspace`；用户操作在全局范围，但创建用户后会自动生成 personal workspace。
- `project_get` 的 `config_summary` 暴露该 project 全部非 secret 配置（含 agent 指令、群绑定等集成键）；secret 键（schema 标 `secret:true`）不回显。

## 标准工作流

### 场景 A：新项目开工并开群

新项目开工后通常要在对应 IM 群里跟进通知，分三步：建项目 → 拉人 → 接群通知（接群见 xuanchu-wire-up-automation skill）。

```
1. project_add({"workspace":"dajee","slug":"apiplat","name":"API 平台"})
2. member_add({"workspace":"dajee","user":"alice","role":"member"})   // 相关人逐个加
3. user_bind({"user":"alice","provider":"feishu_user_id","external_id":"ou_xxx"})  // 绑飞书
4. // 群通知接线见 xuanchu-wire-up-automation：给 project 配 feishu webhook config + 建 sink/rule
```

### 场景 B：巡查项目健康

定期巡查项目状态、动态和成员结构。

```
1. project_list({"workspace":"dajee"})
2. project_get({"workspace":"dajee","project":"apiplat"})   // 看 config_summary 里的 agent.* 指令
3. project_list_timeline({"workspace":"dajee","project":"apiplat","limit":10})
4. member_list({"workspace":"dajee"})
```

### 场景 C：归档项目

```
1. project_archive({"workspace":"dajee","project":"apiplat"})
```

### 场景 D：从模板创建项目

Agent 流程固定为 list → 收集输入 → instantiate：

```
1. project_template_list({"workspace":"dajee","q":"发布"})
2. member_list({"workspace":"dajee"})   // 收集可用成员的 User ID
3. 从返回项读取 template key、current_snapshot.id/hash/config_inputs；按 descriptor 向用户收集新项目 slug/name/start_date、必填或需要填写的普通/secret config 和 assignee replacement
4. project_template_instantiate({
     "workspace":"dajee",
     "template":"release",
     "snapshot_id":"current-snapshot-uuid",
     "expected_snapshot_hash":"current-64-char-lowercase-hash",
     "project_slug":"release26",
     "project_name":"2026 发布项目",
     "start_date":"2026-08-01",
     "config_inputs":{"launch.region":"cn-east-1","agent.api_key":"用户安全提供的值"},
     "assignee_replacements":{"source-user-id":"target-user-id"}
   })
```

每次调用都显式传 `workspace`。`snapshot_id`、`expected_snapshot_hash` 和 `config_inputs` descriptor 必须来自同一次、最新的 `project_template_list`；required 项必须显式收集，optional 留空时省略 key。普通和 secret prompt 都放入 `config_inputs`，不得把 secret 记录到普通消息或日志。如果返回 `project_template_snapshot_hash_mismatch`，重新 list 并再次确认输入，不要用旧版本重试。`assignee_replacements` 的 key 是 unavailable template member 的 **source user ID**，非空 value 是 active workspace member 的 **target user ID**，两端都不能填 name、slug 或其它 ref；source ID 从 validation issue 的 `source_ref` 取得，target ID 先用 `member_list` 查询。模板内容变更、Capture、Preview、归档和 Snapshot 版本管理交给 Web Console。

## 易错点

- **slug 规则不同**：workspace slug 宽松（`^[a-z0-9][a-z0-9_-]*$`，允许 `-` `_`）；project slug 严格（3-10 位小写字母/数字，必须字母开头，不能含 `-` `_` 中文）。详见 references/slug-rules.md。
- `workspace_use` / `user_use` 只影响 stdio MCP 的隐式状态，HTTP MCP 不受影响。默认显式传参，不依赖隐式切换。
- 创建项目时 slug 会自动转小写；`api-platform`、`ai_agent`、`p1`、`1api` 都不是合法 project slug。
- 角色层级：`viewer` < `member` < `admin` < `owner`。成员治理通常需要 `admin` 或 `owner`。
- `project_template_list` 只返回 active Template 的 current Snapshot 摘要和安全的 `config_inputs` descriptor，不返回 Snapshot JSON、历史版本、候选项、字段值或 default。实例化前不要依据缓存猜测版本。
- v2 的普通和 secret prompt 都使用 `config_inputs`；`secret_inputs` 只兼容 v1 历史 `secret_input`。响应和错误不回显 secret。缺少成员替换或其它输入时，根据结构化 `issues` 补齐后再调用。

## 参考文档

| 文件 | 何时读 |
|---|---|
| references/workspace-project-tools.md | 需要 workspace/project 操作的完整 JSON 输入输出 |
| references/user-member-tools.md | 需要 user/member 操作的完整 JSON 输入输出 |
| references/slug-rules.md | 记不住 slug 规则时 |
