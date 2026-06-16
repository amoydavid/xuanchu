# Agent Skills 重写 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除 `docs/skills/` 下不符合规范的 10 个 SKILL.md，按 CIO agent 视角重写成 5 个符合 agentskills.io 规范的 skill（含 references 分层）。

**Architecture:** 纯文档重组，不改任何 Go 代码。每个 skill 是一个目录：`SKILL.md`（≤150 行，含 frontmatter + 何时用 + 核心原则 + 标准工作流 + 易错点 + references 索引）+ `references/*.md`（逐工具完整 JSON、字段清单、长枚举表）。旧文档作为内容来源，按 spec §3 的内容映射重新组织。

**Tech Stack:** Markdown。所有内容来自现有 10 个旧 SKILL.md + spec 的角色专属工作流，不新造 MCP 接口事实。

**关联 spec:** `docs/superpowers/specs/2026-06-16-agent-skills-rewrite-design.md`

---

## 写作规范（适用于所有 Task）

### frontmatter 规范

```markdown
---
name: <skill-name>
description: <1-500 字符，以"何时使用"为导向，覆盖该 skill 的触发场景关键词>
---
```

- `name` 小写连字符，≤64 字符
- `description` 单段，不要换行，列举用户意图关键词（建任务、周报、通知、token…）

### SKILL.md 章节顺序（每个 skill 统一）

1. frontmatter（如上）
2. `# <中文标题>` — 一级标题
3. `## 何时使用`（2-3 句，对应 description）
4. `## 核心原则`（角色专属判断准则，bullets）
5. `## 标准工作流`（2-3 个 CIO 场景，step-by-step，只写 tool 名 + 关键参数，**不含完整 JSON**——完整 JSON 在 references）
6. `## 易错点`（bullets，该场景最常见的坑）
7. `## 参考文档`（references 索引表：文件名 → 何时读）

**约束：** 主文件 ≤ 150 行。完整 JSON、逐工具字段、长枚举一律进 references。

### 内容来源映射

每个 references 文件都从指定旧文档抽取，**不新造接口事实**。旧文档清单：
- `task-management/` → task CRUD/query/annotate/depends/link/start-stop/export-import JSON
- `report-and-urgency/` → report_run/urgency_explain JSON + 内置报表名
- `project-management/` → project 增改归档/注释/时间线/config JSON
- `workspace-management/` → workspace 操作 JSON
- `user-and-member/` → user/member JSON
- `token-management/` → token JSON + scope 清单
- `context-and-config/` → config/schema/context JSON
- `notification-and-reminder/` → sink/rule/delivery JSON + 事件清单 + 模板变量
- `hook-management/` → hook JSON + 事件清单
- `system-and-audit/` → me_get/audit_list/scope_list JSON + action 清单

### references 文件通用规范

每个 references 文件：
- 一级标题 `# <主题>`
- 每个工具一个二级标题 `## <tool_name> — <简述>`
- 标注只读/写
- 给完整 JSON 输入输出示例（从旧文档原样搬运，不改字段）
- 必填字段、易错约束用 bullets

---

## Task 1: 删除旧 skills 目录

**Files:**
- Delete: `docs/skills/notification-and-reminder/`（及内部 SKILL.md）
- Delete: `docs/skills/task-management/`
- Delete: `docs/skills/system-and-audit/`
- Delete: `docs/skills/project-management/`
- Delete: `docs/skills/report-and-urgency/`
- Delete: `docs/skills/workspace-management/`
- Delete: `docs/skills/user-and-member/`
- Delete: `docs/skills/context-and-config/`
- Delete: `docs/skills/token-management/`
- Delete: `docs/skills/hook-management/`

> ⚠️ 内容搬运在新 Task 写完之后再删，避免删了才发现要抄的内容。**本 Task 先空操作占位，放在最后一个 Task 执行**。但为让计划清晰，放在文档顺序首位说明删除清单。

实际执行：此 Task 在 Task 7 执行。这里仅记录删除目标。

---

## Task 2: 写 `govern-projects` skill

**Files:**
- Create: `docs/skills/govern-projects/SKILL.md`
- Create: `docs/skills/govern-projects/references/workspace-project-tools.md`
- Create: `docs/skills/govern-projects/references/user-member-tools.md`
- Create: `docs/skills/govern-projects/references/slug-rules.md`

### Step 1: 写 `SKILL.md`

frontmatter：
```markdown
---
name: govern-projects
description: 建立和归档项目、给项目分配成员、绑定飞书身份、查看项目时间线和成员结构、巡查项目健康。用户提到新建项目、开群、加人、分派角色、绑定外部身份、归档项目、看项目动态时使用。
---
```

正文 `# 项目与团队治理`，按写作规范的 7 节。核心原则要点：
- 每次调用显式传 `workspace`
- project 用 `project`(slug) 或 `project_id`(UUID) 定位；不知道先 `project_list`
- CIO 通常拥有 owner/admin 角色

标准工作流写 3 个场景（**只写 tool 名+关键参数，不写完整 JSON**）：

**场景 A：新项目开工并开群**
```
1. project_add({"workspace":"dajee","slug":"apiplat","name":"API 平台"})
2. member_add({"workspace":"dajee","user":"alice","role":"member"})  // 相关人逐个加
3. user_bind({"user":"alice","provider":"feishu_user_id","external_id":"..."})  // 绑飞书
4. // 群通知接线见 wire-up-automation：给 project 配 feishu webhook config + 建 sink/rule
```

**场景 B：巡查项目健康**
```
1. project_list({"workspace":"dajee"})
2. project_get({"workspace":"dajee","project":"apiplat"})  // 看 config_summary 里的 agent.* 指令
3. project_list_timeline({"workspace":"dajee","project":"apiplat","limit":10})
4. member_list({"workspace":"dajee"})
```

**场景 C：归档项目**
```
1. project_archive({"workspace":"dajee","project":"apiplat"})
```

易错点：
- workspace slug 宽松（`^[a-z0-9][a-z0-9_-]*$`），project slug 严格（3-10 位小写字母/数字、字母开头，不能含 `-`/`_`/中文）。详见 slug-rules.md。
- `project_get` 的 `config_summary` 只暴露以 `agent.` 开头的配置项。
- workspace_use / user_use 只影响 stdio 隐式状态，HTTP MCP 不受影响；CIO 应显式传参，不依赖隐式切换。

参考文档索引表：
| 文件 | 何时读 |
|---|---|
| workspace-project-tools.md | 需要 workspace/project 操作的完整 JSON |
| user-member-tools.md | 需要 user/member 操作的完整 JSON |
| slug-rules.md | 记不住 slug 规则时 |

### Step 2: 写 `references/workspace-project-tools.md`

从旧 `workspace-management/SKILL.md` + `project-management/SKILL.md` 原样搬运所有 JSON 示例。工具清单：
- workspace_list（只读）、workspace_get_current（只读）、workspace_info（只读）、workspace_add、workspace_modify、workspace_use、workspace_archive
- project_add、project_list（只读）、project_get（只读）、project_get_current（只读）、project_modify、project_archive
- project_annotate、project_list_annotations（只读）、project_denotate
- project_list_timeline（只读）
- project_config_set / project_config_list（只读）/ project_config_unset

每个工具：`## <name> — <简述>` + 只读/写标注 + 完整 JSON 输入输出（从旧文档原样搬）。

### Step 3: 写 `references/user-member-tools.md`

从旧 `user-and-member/SKILL.md` 搬运。工具清单：
- user_list（只读）、user_get（只读）、user_add、user_use
- user_bind、user_list_external_ids（只读）、user_unbind
- member_list（只读）、member_add、member_role

每个工具同上格式。补充：admin/owner 可为他人绑外部 ID，普通用户只能绑自己；推荐 `feishu_user_id` 作 provider。

### Step 4: 写 `references/slug-rules.md`

对比表（核心内容，CIO 高频踩坑）：

```markdown
# Slug 规则

| 类型 | 规则 | 合法示例 | 非法示例 |
|---|---|---|---|
| workspace slug | `^[a-z0-9][a-z0-9_-]*$`，允许 `-` `_` | engineering、api-platform | API、工程 |
| project slug | 3-10 位小写字母/数字，必须字母开头，不能含 `-` `_` 中文 | api、apiplat、api9 | api-platform、ai_agent、1api、p1 |
```

补充：user_add 时用户名支持中文等非 ASCII，系统自动生成 personal workspace slug。

### Step 5: 提交

```bash
git add docs/skills/govern-projects
git commit -m "docs(skills): 新增 govern-projects skill（项目与团队治理）"
```

---

## Task 3: 写 `wire-up-automation` skill

**Files:**
- Create: `docs/skills/wire-up-automation/SKILL.md`
- Create: `docs/skills/wire-up-automation/references/notification-tools.md`
- Create: `docs/skills/wire-up-automation/references/hook-tools.md`
- Create: `docs/skills/wire-up-automation/references/event-types.md`
- Create: `docs/skills/wire-up-automation/references/http-template-vars.md`
- Create: `docs/skills/wire-up-automation/references/feishu-bot-setup.md`

### Step 1: 写 `SKILL.md`

frontmatter：
```markdown
---
name: wire-up-automation
description: 把项目 IM 群接上飞书机器人通知、设定时到期/逾期提醒、配置事件 webhook、排查和重试投递失败。用户提到通知、提醒、飞书机器人、webhook、hook、群消息、投递失败、重试时使用。
---
```

正文 `# 通知/提醒/Hook 接线`。核心原则：
- sink = 投递目标（webhook / http_template）；rule = 规则（reminder 定时 vs notification 事件）；hook = 出站集成
- sink 是 workspace 级引用，不能跨 workspace
- secret 走 `secret_refs`，不直接写 URL/body
- 投递失败用 `notification_delivery_list(status:"dead_lettered")` + `notification_delivery_replay`
- config schema 规则详见 manage-access-and-config

标准工作流写 2 个场景（**完整 JSON 放 feishu-bot-setup.md，主文件只写流程要点**）：

**场景 A：给项目群接飞书机器人通知（CIO 核心）**
```
1. config_schema_set 定义 integrations.feishu.webhook_url（project scope）和 im.group_id
2. project_config_set 给本项目配本群 webhook URL 和群 ID
3. notification_sink_add 建 http_template sink，endpoint_mode:config_value，config_key 指向 webhook_url，body 模板渲染任务信息
4. notification_rule_add 订阅 task.assigned/completed/due_changed 等事件
5. reminder_rule_add 设每日到期/逾期扫描（schedule_type:daily_at）
（完整 JSON 见 references/feishu-bot-setup.md）
```

**场景 B：排查投递失败**
```
1. notification_delivery_list({"workspace":"dajee","status":"dead_lettered","limit":20})
2. notification_delivery_info({"workspace":"dajee","delivery_id":"..."})  // 看失败原因
3. notification_delivery_replay({"workspace":"dajee","delivery_id":"..."})  // 仅 dead_lettered/skipped 可 replay
```

易错点：
- `notification_delivery_replay` 不重新渲染 URL/header/body（delivery 生成时已冻结）
- reminder 用 `schedule_type`+`schedule_value`+`filter_source`（新），`trigger_type/offset_seconds`（旧）只作兼容
- `filter_source` 的 duration 用 Go `time.ParseDuration`（`24h`/`90m`/`2h30m`），**不支持 `1d`**
- dispatcher 默认 `max_concurrency=1`；sink `max_concurrency=0` 表示继承 dispatcher
- `assignees`/`assignees_and_explicit_users` audience 只支持 `task.*` 事件，project 事件用 `actor`/`explicit_users`

参考文档索引表：
| 文件 | 何时读 |
|---|---|
| notification-tools.md | sink/rule/delivery 完整 JSON |
| hook-tools.md | hook 完整 JSON |
| event-types.md | 可订阅事件完整清单 |
| http-template-vars.md | 模板变量清单 |
| feishu-bot-setup.md | 飞书群机器人端到端配置示例 |

### Step 2: 写 `references/notification-tools.md`

从旧 `notification-and-reminder/SKILL.md` 搬运所有 JSON。工具清单：
- notification_sink_add、notification_sink_list（只读）、notification_sink_info（只读）、notification_sink_modify、notification_sink_disable、notification_sink_enable、notification_sink_remove
- reminder_rule_add、reminder_rule_list（只读）、reminder_rule_info（只读）、reminder_rule_modify、reminder_rule_disable、reminder_rule_enable、reminder_rule_remove
- notification_rule_add、notification_rule_list（只读）、notification_rule_info（只读）、notification_rule_modify、notification_rule_disable、notification_rule_enable、notification_rule_remove
- notification_delivery_list（只读）、notification_delivery_info（只读）、notification_delivery_replay

每个同格式。含 http_template sink 的 header/body 模板示例、`filter_source` 语法说明（`status:pending and end.isnull and due.before:now`、`now+24h`）。

### Step 3: 写 `references/hook-tools.md`

从旧 `hook-management/SKILL.md` 搬运。工具清单：
- hook_add、hook_list（只读）、hook_info（只读）、hook_modify、hook_remove
- hook_delivery_list（只读）、hook_delivery_info（只读）、hook_delivery_redeliver

补充：hook 只引用 sink 不存 URL/secret；workspace 级或 project 级（传 project/project_id）；dispatcher 共享 sink limiter。

### Step 4: 写 `references/event-types.md`

合并旧 `notification-and-reminder/SKILL.md` 和 `hook-management/SKILL.md` 的事件清单（两处一致）。内容：

```markdown
# 可订阅事件类型

notification rule 和 hook 共用以下事件：

## task.* 事件
- task.created / task.modified / task.completed / task.deleted
- task.started / task.stopped / task.assigned / task.unassigned
- task.blocked / task.unblocked
- task.due_changed / task.priority_changed / task.project_changed / task.tags_changed

## project.* 事件
- project.archived / project.annotated / project.denotated

## 当前不允许注册的候选事件（参考，不可用）
- task.annotated / task.denotated / task.link_added / task.link_removed
- project.created / project.updated
- workspace.member_added / workspace.member_removed / workspace.member_role_changed

## 订阅建议
- start 只触发 task.started，stop 只触发 task.stopped；开始/停止通知分别建规则
- 普通编辑用 task.modified；字段级通知优先用 task.due_changed 等细粒度事件
- task.blocked = 进入阻塞，task.unblocked = 解除阻塞
- assignees/assignees_and_explicit_users audience 只支持 task.* 事件
```

### Step 5: 写 `references/http-template-vars.md`

从旧文档的"模板变量常用"清单搬运。内容：

```markdown
# HTTP Template 模板变量

endpoint template 白名单（allowedEndpointVariable）：workspace.id/slug、project.id/slug、rule.id/name、recipient.id、event.id/type/object_kind/object_id、actor.id、delivery.id/attempt/workspace_id/sink_id。

注意：endpoint template 中禁止使用 `secret.*` 和 `task.*`（会报错）。

body/header 模板可用变量：
- workspace.id / workspace.slug
- project.id / project.slug
- task.uuid / task.task_slug / task.description / task.due
- recipient.id / recipient.name / recipient.email
- reminder.sequence / reminder.overdue_sequence / reminder.window_start / reminder.window_end
- secret.<alias>（通过 secret_refs 声明别名引用 secret config）
```

### Step 6: 写 `references/feishu-bot-setup.md`（CIO 核心端到端示例）

这是新增的、旧文档没有但 spec §3.2 和 §4.3 明确要求的端到端示例。写完整的 5 步 JSON：

```markdown
# 飞书群机器人端到端配置

适用：把一个项目对应的飞书群接上通知，task.assigned/completed/due_changed 时机器人发消息。

## 前置：群绑定
xuanchu 无"群"实体。用 project config 键记录群信息：
- integrations.feishu.webhook_url — 本群飞书机器人地址（sink 用 config_value 读）
- im.group_id — 群 ID（CIO agent 自身逻辑用，project_get 的 config_summary 可读）

一个 sink 被多 project 共享，投递时按当前 project 解析各自 URL。

## Step 1: 定义 config schema
config_schema_set integrations.feishu.webhook_url（project scope，string）和 im.group_id（project scope，string）
[完整 JSON：从 manage-access-and-config/config-schema-tools.md 的格式，key 分别为这两个]

## Step 2: 给 project 配群信息
project_config_set integrations.feishu.webhook_url = "https://open.feishu.cn/open-apis/bot/v2/hook/xxx"
project_config_set im.group_id = "oc_xxx"
[完整 JSON]

## Step 3: 建 http_template sink
notification_sink_add type:http_template, endpoint_mode:config_value, config_key:integrations.feishu.webhook_url, allowed_hosts:["open.feishu.cn"], body_template 渲染飞书消息体
[完整 JSON：含 secret_refs 引用飞书 token、body_content_type:application/json]

## Step 4: 建 notification rule
notification_rule_add 订阅 task.assigned/completed/due_changed，audience:assignees，sink 指向上面建的
[完整 JSON]

## Step 5: 建 reminder rule（每日到期/逾期扫描）
reminder_rule_add schedule_type:daily_at, schedule_value:"09:00", filter_source:"status:pending and end.isnull and due.before:now", audience_type:assignees, sink 同上
[完整 JSON]
```

> 完整 JSON 中的 config_schema_set/project_config_set 字段格式参照 Task 6 的 config-schema-tools.md / config-tools.md 保持一致（执行 Task 6 前可先写这部分，本文件先引用占位说明）。

### Step 7: 提交

```bash
git add docs/skills/wire-up-automation
git commit -m "docs(skills): 新增 wire-up-automation skill（通知/提醒/Hook 接线）"
```

---

## Task 4: 写 `capture-and-track-work` skill

**Files:**
- Create: `docs/skills/capture-and-track-work/SKILL.md`
- Create: `docs/skills/capture-and-track-work/references/task-tools.md`
- Create: `docs/skills/capture-and-track-work/references/query-syntax.md`
- Create: `docs/skills/capture-and-track-work/references/task-workflows.md`

### Step 1: 写 `SKILL.md`

frontmatter：
```markdown
---
name: capture-and-track-work
description: 把群里冒出来的工作变成结构化任务、设依赖、关联 PR/ticket、记录进度、认领并完成任务、导出导入。用户提到记一下这件事、建任务、加备注、设依赖、关联 PR、开始/完成、导出导入任务时使用。
---
```

正文 `# 捕获与跟踪任务`。核心原则：
- 每次调用显式传 `workspace`，任务归 project 时带 `project_id`/`project`
- 任务引用只用 UUID 或 `task_slug`（如 `api-1`），**不用本地 working-set 数字 ID**
- task_slug 形如 `api-1`

标准工作流写 2 个场景：

**场景 A：群聊转任务（CIO 高频）**
```
1. task_add({"workspace":"dajee","project_id":"...","description":"修复白屏","priority":"H"})
2. task_annotate({"workspace":"dajee","project_id":"...","id":"新任务uuid","annotation":"客户反馈 Chrome 121 必现"})
3. task_link_add({"workspace":"dajee","project_id":"...","task":"...","type":"pr","url":"https://github.com/.../pull/42"})
4. // CIO 自己认领就 task_start，做完 task_done
```

**场景 B：查询自己的活**
```
1. task_query({"workspace":"dajee","query":"assignee:me status:pending"})
2. // 详见 report-and-review skill 做报表级汇总
```

易错点：
- MCP 接口不能用 working-set 数字 ID，只能 UUID/task_slug
- `task_denotate` 用 `annotation_id`（从 task_get/query 读取的稳定 ID），不用显示顺序
- `task_modify` 清空字段用 `clear:["priority","assignees"]`

参考文档索引表：
| 文件 | 何时读 |
|---|---|
| task-tools.md | task 全部操作完整 JSON |
| query-syntax.md | query 表达式语法 |
| task-workflows.md | 建任务→记录→完成的端到端示例 |

### Step 2: 写 `references/task-tools.md`

从旧 `task-management/SKILL.md` 搬运所有 JSON。工具清单：
- task_add、task_query（只读）、task_get（只读）、task_modify、task_done、task_delete
- task_start、task_stop
- task_annotate、task_denotate
- task_depends
- task_link_add、task_link_list（只读）、task_link_remove
- task_export（只读）、task_import

每个同格式，含 `clear`、`udas`、`clear_depends` 等变体示例。

### Step 3: 写 `references/query-syntax.md`

从旧 task-management 的 query 说明抽取并扩充。内容：

```markdown
# Task Query 表达式

task_query 的 query 参数支持 Taskwarrior 风格表达式。

## 字段过滤
- description:关键词（描述子串匹配；裸字符串自动按 description 匹配）
- tag:xxx
- assignee:me（当前用户）/ assignee:<用户名>
- project:apiplat
- priority:H
- due.before:today / due.before:now / due.before:now+24h
- status:pending / status:completed
- annotations contains "备注文本"

## 组合
- and / or / not
- 括号分组

## 用在 reminder/notification rule
filter_source 也用此语法：
- status:pending and end.isnull and due.before:now
- end.isnull and start.isnull and due.after:now and due.before:now+24h
- priority:H or +urgent

## duration
now+24h / now-2h，用 Go time.ParseDuration（24h/90m/2h30m），不支持 1d。
```

### Step 4: 写 `references/task-workflows.md`

端到端示例，从旧 task-management 的"典型 Agent 工作流"扩展。3 个场景：
- 群聊转任务（add→annotate→link→start→done 完整链）
- 查询并修改（query→modify 改优先级/标签→clear 清空）
- 导入导出（export 某项目 → import 到另一项目）

每个 step 用完整 JSON（从旧文档搬运）。

### Step 5: 提交

```bash
git add docs/skills/capture-and-track-work
git commit -m "docs(skills): 新增 capture-and-track-work skill（捕获与跟踪任务）"
```

---

## Task 5: 写 `report-and-review` skill

**Files:**
- Create: `docs/skills/report-and-review/SKILL.md`
- Create: `docs/skills/report-and-review/references/report-tools.md`
- Create: `docs/skills/report-and-review/references/urgency-factors.md`
- Create: `docs/skills/report-and-review/references/audit-actions.md`

### Step 1: 写 `SKILL.md`

frontmatter：
```markdown
---
name: report-and-review
description: 在 IM 群里发日报/周报、解释任务 urgency 排序理由、查审计日志排查谁改了或删了什么。用户提到周报、今天该做什么、为什么先做这个、谁删了任务、审计、排查操作记录时使用。
---
```

正文 `# 汇报与审计`。核心原则：
- report_run 只读，name 必填，可叠加 query
- urgency_explain 用于解释"为什么这个任务排前面"
- audit_list 按时间倒序，支持 actor 过滤

标准工作流写 3 个场景：

**场景 A：群里发今日待办汇总**
```
1. report_run({"workspace":"dajee","name":"next","limit":10})  // 今日该做的
2. report_run({"workspace":"dajee","name":"overdue"})  // 逾期
3. // 汇总成消息发群（CIO 自身把结果转发到飞书群）
```

**场景 B：解释排序**
```
1. report_run({"workspace":"dajee","name":"next","limit":1})  // 拿排第一的
2. urgency_explain({"workspace":"dajee","id":"task-slug"})  // 返回 factors
```

**场景 C：排查"谁删了任务"**
```
1. audit_list({"workspace":"dajee","limit":20,"actor":"bob"})
2. // 过滤 action 为 task.delete 的条目，看 payload 和 target_id
```

易错点：
- 内置报表名固定：list/next/all/completed/deleted/waiting/active/ready/overdue/blocked/blocking
- urgency 因素：priority、due 等（见 urgency-factors.md）

参考文档索引表：
| 文件 | 何时读 |
|---|---|
| report-tools.md | report_run/urgency_explain 完整 JSON + 报表名 |
| urgency-factors.md | urgency 因素清单与系数 |
| audit-actions.md | audit action 完整清单 |

### Step 2: 写 `references/report-tools.md`

从旧 `report-and-urgency/SKILL.md` 搬运。含 report_run（name 枚举 + query 叠加 + limit）、urgency_explain（返回 factors 数组）完整 JSON。列内置报表名表。

### Step 3: 写 `references/urgency-factors.md`

```markdown
# Urgency 因素

urgency_explain 返回 data.factors，每个因素 {name, value}。

常见因素：
- priority — 任务优先级贡献
- due — 临近 due 的贡献

调整系数：在 workspace 级 config 设 urgency.* 键（如 urgency.priority.coeff）。详见 manage-access-and-config/config-keys.md。
```

### Step 4: 写 `references/audit-actions.md`

从旧 `system-and-audit/SKILL.md` 的"常见 action 值"完整搬运。按资源分组列出所有 action：task.* / project.* / workspace.* / user.* / member.* / context.* / config.* / hook.* / notification.* / reminder.* / token.*。

### Step 5: 提交

```bash
git add docs/skills/report-and-review
git commit -m "docs(skills): 新增 report-and-review skill（汇报与审计）"
```

---

## Task 6: 写 `manage-access-and-config` skill

**Files:**
- Create: `docs/skills/manage-access-and-config/SKILL.md`
- Create: `docs/skills/manage-access-and-config/references/token-tools.md`
- Create: `docs/skills/manage-access-and-config/references/scopes.md`
- Create: `docs/skills/manage-access-and-config/references/config-tools.md`
- Create: `docs/skills/manage-access-and-config/references/config-schema-tools.md`
- Create: `docs/skills/manage-access-and-config/references/config-keys.md`

### Step 1: 写 `SKILL.md`

frontmatter：
```markdown
---
name: manage-access-and-config
description: 管理 API token（创建/轮换/撤销）、查可用权限、调整 urgency 排序权重、给项目配 agent 指令、定义自定义配置项与 schema、查看过滤上下文。用户提到 token、权限、调权重、配 agent 指令、定义配置键、看 context、轮换密钥时使用。
---
```

正文 `# 权限与配置运维`。核心原则：
- **CIO 自身用后台手动建的 `["*"]` agent_token**，本 skill 不教它给自己建 token
- 给别的系统/agent 发 token：通用 `["*"]` 或最小化专用 scope；raw_token 只出现一次必须保存
- config 三级 scope：workspace（业务键 urgency.*/date.*，**不支持 agent.***）/ project / local（仅 stdio 只读）
- agent 指令走 project 级 `agent.*`；写自定义键前先 `config_schema_set`
- context 是只读偏好，CIO 不改 active context（context_set/context_none 仅用户明确要求时用）

标准工作流写 2 个场景：

**场景 A：给新系统发 token**
```
1. token_create({"workspace":"dajee","name":"ci-token","scope":["*"],"expires_in_seconds":86400})
2. // raw_token 只返回一次，保存后告诉使用方
```

**场景 B：给项目配 CIO 指令 + 调 urgency 权重**
```
1. project_config_set({"workspace":"dajee","project":"apiplat","key":"agent.background","value":"你是 API 平台 CIO..."})
2. config_set({"workspace":"dajee","scope":"workspace","key":"urgency.priority.coeff","value":"6.0"})
```

易错点：
- workspace 级 config 只支持业务键（urgency.*/date.*），不支持 agent.*；agent 指令必须 project 级
- config_set 必须显式传 scope；config_get 不传 scope 默认读 workspace 级
- config_schema_set 的 value_type 支持 string/number/boolean/json；枚举约束用 enum_values，不要把 value_type 写成 enum
- 内置 schema 键：agent.background/agent.constraints/agent.default_context/agent.handoff/context.default（均 project scope，无需自己定义）
- token scope 通配符：`*`（含 impersonate）、`task:*`、`*:read`

参考文档索引表：
| 文件 | 何时读 |
|---|---|
| token-tools.md | token 操作完整 JSON |
| scopes.md | scope 完整清单与通配符 |
| config-tools.md | config get/set/unset/list 完整 JSON |
| config-schema-tools.md | config schema 操作完整 JSON |
| config-keys.md | 配置键语义表 |

### Step 2: 写 `references/token-tools.md`

从旧 `token-management/SKILL.md` 搬运。工具：token_list（只读）、token_create、token_modify、token_revoke。完整 JSON。强调 raw_token 只出现一次。

### Step 3: 写 `references/scopes.md`

从旧 token-management 的 Scope 说明搬运。完整 scope 清单 + 通配符扩展规则（`*`/`task:*`/`*:read`）。说明 `*` 是 token capability 上限，实际权限仍被 workspace/project allowlist 和 membership role 收窄。

### Step 4: 写 `references/config-tools.md`

从旧 `context-and-config/SKILL.md` 的 Config 节搬运。工具：config_get、config_set、config_list（只读）、config_unset、project_config_set/list/unset。完整 JSON，含三级 scope 说明、workspace 不支持 agent.* 的约束。

### Step 5: 写 `references/config-schema-tools.md`

从旧 context-and-config 的 Config Schema 节搬运。工具：config_schema_list（只读）、config_schema_get（只读）、config_schema_set、config_schema_delete（含 purge）。完整 JSON。补充内置 schema 键清单。

### Step 6: 写 `references/config-keys.md`

```markdown
# 配置键语义表

## 内置键（无需自定义，已预置 schema）
| 键 | scope | 说明 |
|---|---|---|
| agent.background | project | 项目级 agent 背景指令 |
| agent.constraints | project | agent 约束 |
| agent.default_context | project | agent 默认 context |
| agent.handoff | project | 交接说明 |
| context.default | project | 默认过滤 context |

## 约定键（本系统群绑定/集成用，需 config_schema_set 定义）
| 键 | scope | 说明 |
|---|---|---|
| integrations.feishu.webhook_url | project | 飞书群机器人地址（sink 用 config_value 读） |
| integrations.feishu.bot_token | workspace/project (secret) | 飞书机器人 token |
| im.group_id | project | IM 群 ID（CIO agent 自身逻辑用） |

## 业务键（workspace 级）
| 键 | scope | 说明 |
|---|---|---|
| urgency.priority.coeff | workspace | priority 对 urgency 的贡献系数 |
| date.* | workspace | 日期相关配置 |

## 规则
- workspace 级只支持业务键，不支持 agent.*
- agent 指令必须 project 级
- 写自定义键前必须 config_schema_set 定义
```

### Step 7: 提交

```bash
git add docs/skills/manage-access-and-config
git commit -m "docs(skills): 新增 manage-access-and-config skill（权限与配置运维）"
```

---

## Task 7: 删除旧 skills 目录

> 现在新 skill 已全部写完，旧文档内容已搬运，可安全删除。

**Files:**
- Delete: `docs/skills/notification-and-reminder/`
- Delete: `docs/skills/task-management/`
- Delete: `docs/skills/system-and-audit/`
- Delete: `docs/skills/project-management/`
- Delete: `docs/skills/report-and-urgency/`
- Delete: `docs/skills/workspace-management/`
- Delete: `docs/skills/user-and-member/`
- Delete: `docs/skills/context-and-config/`
- Delete: `docs/skills/token-management/`
- Delete: `docs/skills/hook-management/`

### Step 1: 删除

```bash
git rm -r docs/skills/notification-and-reminder docs/skills/task-management docs/skills/system-and-audit docs/skills/project-management docs/skills/report-and-urgency docs/skills/workspace-management docs/skills/user-and-member docs/skills/context-and-config docs/skills/token-management docs/skills/hook-management
```

### Step 2: 提交

```bash
git commit -m "docs(skills): 删除旧的非规范 skill 文档"
```

---

## Task 8: 最终验收

### Step 1: 结构校验

```bash
ls -la docs/skills/
```
预期：只有 govern-projects、wire-up-automation、capture-and-track-work、report-and-review、manage-access-and-config 五个目录。

```bash
find docs/skills -name SKILL.md | sort
```
预期：5 个 SKILL.md，每个以 `---\nname:` frontmatter 开头。

### Step 2: frontmatter 合规校验

```bash
for f in $(find docs/skills -name SKILL.md); do
  echo "=== $f ==="
  head -1 "$f"   # 预期 ---
  head -5 "$f" | grep -E "^(name|description):"
done
```
预期：每个文件首行 `---`，前 5 行含 `name:` 和 `description:`。

### Step 3: SKILL.md 行数校验（≤150）

```bash
for f in $(find docs/skills -name SKILL.md); do wc -l "$f"; done
```
预期：每个 ≤ 150 行。超了就精简，把细节移到 references。

### Step 4: 未改 Go 代码校验

```bash
git diff --stat main -- internal/ cmd/ | tail -5
```
预期：空输出（本次不改任何 Go 代码）。

### Step 5: description 长度校验（≤500 字符）

```bash
for f in $(find docs/skills -name SKILL.md); do
  desc=$(awk '/^description:/{sub(/^description: /,""); print}' "$f")
  echo "$f: ${#desc} chars"
done
```
预期：每个 ≤ 500。

### Step 6: 更新相关文档（如需要）

检查 README.md / ROADMAP.md 是否引用了旧 docs/skills 路径。若引用则更新为新结构。

```bash
grep -rn "docs/skills" README.md ROADMAP.md AGENTS.md 2>/dev/null
```

如有引用，更新为新路径；无则跳过。

---

## Self-Review 记录

**Spec coverage（逐条核对）：**
- spec §3.1 govern-projects → Task 2 ✅
- spec §3.2 wire-up-automation（含群绑定 §4.3）→ Task 3 ✅
- spec §3.3 capture-and-track-work → Task 4 ✅
- spec §3.4 report-and-review → Task 5 ✅
- spec §3.5 manage-access-and-config → Task 6 ✅
- spec §5 文件结构 → Task 2-6 文件清单完全对应 ✅
- spec §6 写作规范（7 节、≤150 行、references 下沉）→ 写作规范节 + Task 8 校验 ✅
- spec §7 验收标准 → Task 8 ✅
- 删除旧 10 目录 → Task 7 ✅
- spec §4.3 群绑定零系统改动 → 全程不改 Go 代码，Task 8 Step 4 校验 ✅

**Placeholder scan：** 无 TBD/TODO；所有 references 内容来源明确映射到旧文档或 spec 明确给定的事实。feishu-bot-setup.md 的 config_schema_set/project_config_set 字段格式参照 Task 6 同源，已在 Task 3 Step 6 标注。

**类型一致性：** tool 名称、config key（integrations.feishu.webhook_url / im.group_id / urgency.priority.coeff / agent.*）在各 Task 间一致；事件清单 Task 3 Step 4 合并自旧文档两处一致内容。内置 schema 键经代码核实（config_schema.go:14-20）。
