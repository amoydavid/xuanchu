# Agent Skills 重写设计

> 状态：设计稿，待评审
> 日期：2026-06-16
> 背景：`docs/skills/` 下现有 10 个 SKILL.md 严重不符合 [agentskills.io 规范](https://agentskills.io/specification)，需删除并按"使用者角色"重新组织。

## 1. 问题诊断

### 1.1 现有文档不符合规范

| 问题 | 规范要求 | 现状 |
|---|---|---|
| 缺 YAML frontmatter | `SKILL.md` 必须以 `name` + `description` 的 frontmatter 起头，这是被发现/激活的唯一依据 | 10 个文件全是 `# 标题` 开头，一个 frontmatter 都没有 |
| 写成 API 手册而非 skill | body 是"每次运行都要读的核心指令"，建议 < 500 行 / 5000 token | 全是逐接口罗列 + 大段 JSON，task-management 320 行、notification 256 行 |
| 按资源分而非按场景分 | `description` 决定何时激活，应按使用者意图切分 | 按 DB 实体切（task/project/workspace/token/hook…），是 schema 视角 |
| 细节无处下沉 | 规范支持 `references/*.md` 按需加载 | 所有 JSON 示例堆在 SKILL.md 主文件 |

一句话：现在这 10 个文件是"xuanchu MCP 的接口文档"，不是"agent 用来完成任务的 skill"。

### 1.2 使用者角色

使用者是 **OpenClaw / Hermes 类自主 agent**，被赋予 **CIO 角色**——所有项目的总管。其工作特征：

- **长驻运行**：在一个项目对应的 IM 群里常驻，持续治理而非问答。
- **自主执行多步任务**：收到意图后规划并执行。
- **全权操作**：使用 xuanchu 后台手动建好的 `["*"]` agent_token，拥有所有操作权限。
- **治理为主，执行为辅**：高频是建项目/分人/配通知/巡检/排查/广播；低频是亲自领活干完。
- **跨通道**：从 IM / CLI / MCP 进入，对外通过飞书群机器人广播。

### 1.3 群绑定需求

"一个项目建一个 IM 群跟进，CIO 机器人在群里发通知"。需要把群与 project 对应。

**结论：无需改系统。** 现有 `project config` + `config_value` sink 机制已完整支撑（见 §4.3）。本次只改 skill 文档，不动 Go 代码。

## 2. 设计目标

1. 全部符合 agentskills.io 规范（frontmatter + 分层 + 体积控制）。
2. 按 CIO agent 的**行为模式**切分，激活描述精准、不重叠。
3. 主 SKILL.md 精简（流程/决策/工作流/易错点），逐工具 JSON 下沉到 `references/`。
4. 覆盖群绑定、CIO 全权 token、广播等角色专属场景。
5. 中文为主（遵循 AGENTS.md）。

## 3. Skill 切分

5 个 skill，按 CIO 的高频→低频排列：

### 3.1 `xuanchu-govern-projects` — 项目与团队治理 🟢重

```yaml
name: xuanchu-govern-projects
description: 建立和归档项目、给项目分配成员、绑定飞书身份、查看项目时间线和成员结构、巡查项目健康。用户提到新建项目、开群、加人、分派角色、绑定外部身份、归档项目、看项目动态时使用。
```

覆盖：workspace 增改归档、project 增改归档 + 注释/时间线、user 创建/外部ID绑定、member 角色、project ↔ IM 群绑定（config）。

主体内容：
- 新项目标准开群流程：`project_add` → `member_add` 相关人 → `user_bind` 绑飞书 → 配 IM 群 webhook（config_value，详见 xuanchu-wire-up-automation）
- slug 规则差异：workspace 宽松 `^[a-z0-9][a-z0-9_-]*$`；project 严格 3-10 位小写字母/数字、字母开头
- 角色层级 viewer < member < admin < owner
- 每次调用显式传 workspace

references：`workspace-project-tools.md`、`user-member-tools.md`、`slug-rules.md`

### 3.2 `xuanchu-wire-up-automation` — 通知/提醒/Hook 接线 🟢重

```yaml
name: xuanchu-wire-up-automation
description: 把项目 IM 群接上飞书机器人通知、设定时到期/逾期提醒、配置事件 webhook、排查和重试投递失败。用户提到通知、提醒、飞书机器人、webhook、hook、群消息、投递失败、重试时使用。
```

覆盖：notification sink + reminder rule + notification rule + hook + delivery 查看/重试 + 依赖的 config schema。

主体内容：
- **飞书群机器人标准接法**（CIO 核心场景）：
  1. `config_schema_set` 定义 `integrations.feishu.webhook_url`（project scope）和 `im.group_id`
  2. 每个 project 用 `project_config_set` 配本群 webhook URL 和群 ID
  3. `notification_sink_add` 建 `http_template` sink，`endpoint_mode: config_value`，`config_key` 指向上面的键，body 模板渲染任务信息
  4. `notification_rule_add` 订阅 `task.assigned`/`task.completed`/`task.due_changed` 等事件
  5. `reminder_rule_add` 设每日到期/逾期扫描
- sink = 投递目标 / rule = 规则（reminder 定时 vs notification 事件）/ hook = 出站集成
- secret 走 `secret_refs`，不写死 URL/body
- 投递失败用 `notification_delivery_list(status:dead_lettered)` + `notification_delivery_replay`

references：`notification-tools.md`、`hook-tools.md`、`event-types.md`、`http-template-vars.md`、`feishu-bot-setup.md`

### 3.3 `xuanchu-capture-and-track-work` — 捕获与跟踪任务 🟡中

```yaml
name: xuanchu-capture-and-track-work
description: 把群里冒出来的工作变成结构化任务、设依赖、关联 PR/ticket、记录进度、认领并完成任务、导出导入。用户提到记一下这件事、建任务、加备注、设依赖、关联 PR、开始/完成、导出导入任务时使用。
```

覆盖：task 全部 CRUD + query + annotate/denotate + depends + link + start/stop + done + export/import。

主体内容：
- 群聊转任务：`task_add` 归到当前 project → `task_annotate` 记上下文 → `task_link_add` 关联 PR/ticket → CIO 自己认领就 `task_start`/`task_done`
- 任务引用只用 UUID / task_slug，不用本地 working-set 数字 ID
- 显式传 workspace + project_id/project
- query 表达式：`description:`、`tag:`、`assignee:me`、`priority:H`、`due.before:today`

references：`task-tools.md`、`query-syntax.md`、`task-workflows.md`

### 3.4 `xuanchu-report-and-review` — 汇报与审计 🟡中

```yaml
name: xuanchu-report-and-review
description: 在 IM 群里发日报/周报、解释任务 urgency 排序理由、查审计日志排查谁改了或删了什么。用户提到周报、今天该做什么、为什么先做这个、谁删了任务、审计、排查操作记录时使用。
```

覆盖：report_run + urgency_explain + audit_list。

主体内容：
- 群里广播：每日 `report_run(name:"overdue")` + `report_run(name:"next")` 汇总
- "为什么先做这个" → `urgency_explain` 返回 factors
- "谁删了任务" → `audit_list(actor/filter)`，按 action 过滤
- 内置报表：list/next/all/completed/deleted/waiting/active/ready/overdue/blocked/blocking

references：`report-tools.md`、`urgency-factors.md`、`audit-actions.md`

### 3.5 `xuanchu-manage-access-and-config` — 权限与配置运维 🔵轻

```yaml
name: xuanchu-manage-access-and-config
description: 管理 API token（创建/轮换/撤销）、查可用权限、调整 urgency 排序权重、给项目配 agent 指令、定义自定义配置项与 schema、查看过滤上下文。用户提到 token、权限、调权重、配 agent 指令、定义配置键、看 context、轮换密钥时使用。
```

覆盖：token CRUD + scope_list + config get/set/unset/list + config schema（set/get/list/delete）+ project_config 快捷方式 + context 只读。

主体内容：
- CIO 自身用后台手动建的 `["*"]` agent_token，skill 不教它给自己建 token
- 给别的系统/agent 发 token：通用 `["*"]` 或最小化专用 scope；raw_token 只出现一次
- 三级 config scope：workspace（业务键 urgency.*/date.*，不支持 agent.*）/ project / local（仅 stdio 只读）
- agent 指令走 project 级 `agent.*`；写自定义键前先 `config_schema_set`
- context 是只读偏好，Agent 不改 active context

references：`token-tools.md`、`scopes.md`、`config-tools.md`、`config-schema-tools.md`、`config-keys.md`

## 4. 关键设计决策

### 4.1 为什么 CIO 视角下 `xuanchu-capture-and-track-work` 比"执行循环"更合适

OpenClaw/Hermes 这类 agent 高频是被 CIO 委派去干活的 worker，但**本项目里 agent 本身就是 CIO**，治理和广播才是高频。亲自领活是低频动作，融进 xuanchu-capture-and-track-work（建完任务顺手 start/done），不单列"领活干活回报"skill，避免与 capture 重叠。

### 4.2 为什么 report 独立成 skill

CIO 在 IM 群里**广播**是核心职责。report_run + urgency_explain + audit_list 这套"汇报与排查"组合是独立高频场景，塞进别的 skill 会让那个 skill 职责膨胀。

### 4.3 群绑定：纯约定，零系统改动

经代码核查（`internal/app/notification_endpoint.go`、`project_config.go`、`config_schema.go`）：

- xuanchu 无"群/chat/group"实体，但 `config_value` endpoint mode 支持从 project config 键解析 sink URL。
- 模板变量白名单（`allowedEndpointVariable`）含 `project.id/slug`，body 模板可用任务字段。
- 因此群绑定 = 约定两个 project config 键：
  - `integrations.feishu.webhook_url`：本群飞书机器人地址（sink 用 config_value 读）
  - `im.group_id`：群 ID（CIO agent 自身逻辑用，project_get 的 config_summary 可读）
- sink 被多 project 共享，投递时按当前 project 解析各自 URL。
- 反查（群→project）不需要：CIO 始终从 project 出发。

**本次不改 Go 代码**。若未来需要系统级群实体或 by-value 反查，另起 spec。

### 4.4 skill 间交叉处理

`xuanchu-wire-up-automation` 配飞书 token 需先 `config_schema_set` 定义密钥——"schema 规则详见 xuanchu-manage-access-and-config"。wire-up 在工作流里**直接演示**完整例子（建 schema→建 sink→建 rule）并标注交叉引用，避免激活"配通知"时同时依赖两个 skill，又不重复维护规则。

## 5. 文件结构

```
docs/skills/
├── xuanchu-govern-projects/
│   ├── SKILL.md
│   └── references/
│       ├── workspace-project-tools.md
│       ├── user-member-tools.md
│       └── slug-rules.md
├── xuanchu-wire-up-automation/
│   ├── SKILL.md
│   └── references/
│       ├── notification-tools.md
│       ├── hook-tools.md
│       ├── event-types.md
│       ├── http-template-vars.md
│       └── feishu-bot-setup.md
├── xuanchu-capture-and-track-work/
│   ├── SKILL.md
│   └── references/
│       ├── task-tools.md
│       ├── query-syntax.md
│       └── task-workflows.md
├── xuanchu-report-and-review/
│   ├── SKILL.md
│   └── references/
│       ├── report-tools.md
│       ├── urgency-factors.md
│       └── audit-actions.md
└── xuanchu-manage-access-and-config/
    ├── SKILL.md
    └── references/
        ├── token-tools.md
        ├── scopes.md
        ├── config-tools.md
        ├── config-schema-tools.md
        └── config-keys.md
```

删除现有 10 个目录：notification-and-reminder、task-management、system-and-audit、project-management、report-and-urgency、workspace-management、user-and-member、context-and-config、token-management、hook-management。

## 6. 单个 SKILL.md 写作规范

每个 SKILL.md 控制在 150 行内，包含：

1. **frontmatter**（`name` + `description`）
2. **何时使用**（2-3 句，对应 description）
3. **核心概念/原则**（角色专属的判断准则，如"每次调用显式传 workspace"）
4. **标准工作流**（2-3 个 CIO 常见场景的 step-by-step，带 tool 调用，不含完整 JSON）
5. **易错点**（角色最容易踩的坑）
6. **references 索引**（指向 references/*.md，标注何时读哪个）

逐工具的完整 JSON 输入输出、字段清单、长枚举表，全部放 references。

## 7. 验收标准

- [ ] 5 个目录，每个含合规 frontmatter（name 小写连字符 ≤64 字符；description 1-500 字符）
- [ ] 每个 SKILL.md ≤ 150 行
- [ ] 所有逐工具 JSON 在 references/ 下
- [ ] 群绑定工作流在 xuanchu-govern-projects 和 xuanchu-wire-up-automation 中可执行
- [ ] CIO 全权 token、广播、飞书群机器人场景全覆盖
- [ ] 旧 10 个目录已删除
- [ ] 不改任何 Go 代码（git diff 仅 docs/skills）

## 8. 范围外

- 不新增 xuanchu 功能、不改 Go 代码、不动 MCP tool。
- 群实体、by-value config 反查等系统增强若需要，另起 spec。
- 不涉及 skill 的自动发现/加载机制（由运行时负责）。
