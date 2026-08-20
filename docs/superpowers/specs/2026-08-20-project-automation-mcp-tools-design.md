# Project Automation MCP Tools 暴露设计

- 日期：2026-08-20
- 状态：已实施
- 关联：[2026-07-26-workspace-agent-automation-design.md](2026-07-26-workspace-agent-automation-design.md)、[2026-07-08-web-console-project-automation-openai-compatible-design.md](2026-07-08-web-console-project-automation-openai-compatible-design.md)

## 1. 背景与决策

Project Automation 此前只有 HTTP/OpenAPI 与 Web Console 两个管理入口。workspace-agent-automation spec §14.4 曾约定「首版不新增 Automation MCP CRUD」。本次按产品决策调整边界：**Project scope 的自动化管理全量暴露为 MCP tools**，供 headless 治理与 Agent 平台编排使用。

边界保持不变的部分：

- **Workspace scope 自动化仍不暴露 MCP CRUD**。spec 反对理由依然成立：Workspace 规则承载 Agent 自身触发器，暴露写能力会让 Agent 默认可修改自身触发器，形成自指风险。
- **provider-config facade 不暴露**。Agent 继续通过既有 `project_config_list` / `project_config_set` / `project_config_unset` 管理 `agent.provider.*` 配置，避免同一能力出现两套 tool。

## 2. Tool 清单（14 个）

命名遵循仓库规范 `{资源}_{动作}`、子资源用 `hook_delivery_*` 先例：

| Tool | 说明 | 权限 |
|---|---|---|
| `project_automation_list` | 列出项目规则，`include_disabled` 控制是否含停用规则 | 读 |
| `project_automation_get` | 单条规则详情 | 读 |
| `project_automation_add` | 创建规则（schedule/event 触发 + OpenAI 兼容投递） | 写 |
| `project_automation_modify` | 部分更新，nil 字段不更新 | 写 |
| `project_automation_remove` | 删除规则 | 写 |
| `project_automation_enable` / `project_automation_disable` | 启用/停用 | 写 |
| `project_automation_test` | 入队一条 `manual_test` 投递，由后台 dispatcher 真实发送 | 写 |
| `project_automation_preview` | 未保存规则渲染预览，secret 脱敏，不写库 | 写 |
| `project_automation_preview_saved` | 已保存规则按当前配置预览（不支持 override，假设性变更用 `project_automation_preview`） | 写 |
| `project_automation_delivery_list` | 投递记录列表，支持 `rule_id`/`status`/`limit`/`offset` | 读 |
| `project_automation_delivery_get` | 单条投递记录 | 读 |
| `project_automation_delivery_replay` | 基于原记录新建 queued 投递，原记录不变 | 写 |
| `project_automation_list_template_vars` | 模板变量清单（schedule/event 两组） | 读 |

preview 语义上只读，但服务层复用 `requireProjectAutomationWrite`（渲染需解析 provider config），权限与 HTTP 一致。

## 3. 实现约束

- **纯薄壳**：只调用 `internal/app` 与 HTTP handler 相同的 project-specific service 方法（`AddProjectAutomationRule` 等），不复制业务逻辑，不改变 app 层行为。
- **双 scope 鉴权**：复刻 HTTP `scopedProjectAutomationService`——读操作要求 `project:read + hook:read`（PermissionProjectRead + PermissionHookRead），写操作要求 `project:write + hook:write`（PermissionProjectManage + PermissionHookWrite）。MCP 侧 helper `automationProjectServiceForTool` 依次做两次授权。
- **输入形状镜像 HTTP DTO**：`trigger_config` / `condition` / `action` / `context` 嵌套结构与 `/api/v1/projects/{ref}/automations` 请求体一致。
- **用户输出规范**：规则视图的 `created_by` 使用 `task.UserInfoToJSON` 输出 `JSONUserInfo` 形状。
- **project/project_id 必填**：全部 14 个 input 加入 `projectRefRequired`，schema 生成 anyOf 约束。
- 附带修复：app 层 project automation 各读写方法把仓储 `ErrNotFound` 归一为 `automation_rule_not_found` / `automation_delivery_not_found` 业务错误码（此前 HTTP 对不存在规则会误报 500 internal）。

## 4. 测试与验收

- 单元：`internal/mcpserver/tools_project_automation_test.go` 覆盖生命周期、事件规则条件、非法输入、closed project、预览脱敏、投递查询/重放、模板变量；schema 回归测试补 14 个 input 的 anyOf 断言。
- Golden：`testdata/list-tools-default.json` 与 per-tool schema 文件重新生成。
- E2E：`tests/integration/e2e_project_automation_mcp_test.go` 用真实二进制 + HTTP MCP 跑通「建规则 → 预览脱敏 → 测试投递 → dispatcher 投递成功 → 投递查询/重放 → 修改/停用/删除」，并验证受限 token（只有 `project:read`）被双 scope 校验拒绝（`token_scope_denied`）。
