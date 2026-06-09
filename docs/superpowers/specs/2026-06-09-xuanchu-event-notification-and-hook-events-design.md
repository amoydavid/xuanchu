# Xuanchu 事件通知与 Hook 事件扩展设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 补齐 Xuanchu 的事件模型，让项目 annotation、依赖解除阻塞等变化可以作为稳定事件被 Hook 和通知规则消费；同时把 Hook 和用户通知的外部投递都收敛到 outbound sink，例如 `--sink openclaw` 或 `--sink audit-stream`，避免长期依赖单一 URL 字段。

**范围策略：** 本规格只设计事件类型、事件通知规则、sink 化投递路径，以及与现有 Hook / reminder rule 的边界。它建立在现有 Hook outbox、notification sink、notification delivery、dispatcher、workspace/project 隔离和权限模型之上，不重新设计定时通知。由于当前能力尚未上线，不需要兼容旧的 Hook URL 模型；Hook 新模型直接使用 `--sink`。产品概念上统一称为 outbound sink；现有 `notification sink` 数据表和命令可以作为实现基础继续复用。

---

## 1. 背景与现状

Xuanchu 当前已经具备三类相关能力：

- Hook：事件驱动的原始 webhook，当前模型里有直接 URL 配置，但尚未上线；本规格要求改为 `hook add --event ... --sink ...`，投递到 `hook_deliveries`。
- 定时通知：使用现有 `notification sink` + `reminder rule`，通过 `--sink openclaw` 或其他 sink 生成 `notification_deliveries`。
- 项目 annotation：`project annotate` / `project denotate` 已在 app 层构造 `project.annotated` / `project.denotated` 事件。

当前缺口：

- Hook 事件白名单只允许 `task.created`、`task.modified`、`task.completed`、`task.deleted`、`project.archived`，导致已经构造的 `project.annotated` / `project.denotated` 不能被用户注册消费。
- 依赖解除阻塞没有事件。任务 1 完成后，依赖任务 1 的任务 2 从 blocked 变成 ready，这个变化对负责人很重要，但当前只能靠人或 Agent 查询发现。
- “通知用户”现在只有定时 reminder rule。事件触发的用户通知还没有规则模型。
- 如果 Hook 和事件通知继续各自直接维护 URL，会绕开现有 sink 的动态 endpoint、模板、secret、delivery 冻结、重试和 replay 机制，也无法优雅支持固定第三方 Web API。

---

## 2. 核心决策

### 2.1 Hook 和 Notification Rule 分工

Hook 表示“把原始事件 envelope 推给外部系统”。它应该使用 sink，而不是长期绑定一个裸 URL：

```bash
xuanchu notification sink add audit-stream \
  --type http_template \
  --url-template 'https://example.test/xuanchu/events' \
  --header-template 'X-Xuanchu-Event: {{event.type}}' \
  --body-template '{{event.json}}'

xuanchu hook add audit-stream \
  --event project.annotated \
  --sink audit-stream
```

Notification Rule 表示“基于事件或定时扫描给人发通知”：

```bash
xuanchu notification rule add task-unblocked-openclaw \
  --event task.unblocked \
  --audience assignees \
  --sink openclaw
```

两者不混用：

- Hook 使用 `--sink`，服务机器到机器的原始事件集成。
- Notification Rule 使用 `--sink`，服务用户通知。
- 二者的区别不是出口配置，而是投递内容和受众模型：Hook 投递事件 envelope；Notification Rule 解析 audience 并投递面向人的消息。
- Hook 不再接受 `--url`。目标 endpoint、header、body、secret 都由 sink 描述。

### 2.2 事件是统一原语，不等于 Hook

事件应是 app 层生成的统一事实记录，可以同时被两类消费者使用：

- Hook consumer：根据 hook definition 匹配事件，绑定 sink，生成 `hook_deliveries`。
- Notification consumer：根据 notification rule 匹配事件，解析 audience 和模板，生成 `notification_deliveries`。

因此 `HookEvent` 这个内部类型概念上应演进为通用 `Event`。实施计划可以选择先小步重命名，也可以先新增事件通知 consumer，避免一次性大改。

### 2.3 Outbound Sink 是统一外部出口

OpenClaw 通常有向用户发送消息的能力，但在 Xuanchu 内不应写死 OpenClaw adapter。OpenClaw 只是一个 sink：

```bash
xuanchu notification sink add openclaw \
  --type webhook \
  --url https://openclaw.example.com/xuanchu/notifications \
  --secret "$OPENCLAW_WEBHOOK_SECRET"
```

当事件通知规则命中时，Xuanchu 只负责生成标准 notification delivery；OpenClaw 负责把消息投递给最终用户，并在用户回复后通过 HTTP/MCP/Remote CLI 回写 Xuanchu。

同理，第三方审计系统、项目管理系统、固定 HTTP API 也应通过 sink 表达。区别只在 sink 的模板、header、secret 和 body 是否面向机器事件，还是面向用户消息。

### 2.4 命名

当前实现已经有 `notification sink`。当 sink 被 Hook 和 Notification Rule 共同使用后，“notification” 这个名字会过窄。规格上的长期概念应改为 `outbound sink`：

- `outbound sink`：产品概念，表示任何外部投递出口。
- `notification sink`：现有 CLI/API/DB 名称，首版可作为底层实现继续使用。
- `xuanchu sink ...`：建议新增的短别名，和 `xuanchu notification sink ...` 指向同一套资源。

实施时不要求立刻重命名数据库表。可以继续复用 `notification_sinks`，但 app 层和文档应避免把它解释成“只能用于通知用户”。

### 2.5 Hook 定义也要引用 sink

Hook definition 应增加 sink 引用：

- `sink_id`：新字段，指向同 workspace 的 outbound sink。

新建 Hook 时的规则：

- 必须传入 `--sink <sink-ref>`。
- 不接受 `--url`。
- sink 必须属于同一 workspace。

Hook delivery 仍然可以保留独立的 `hook_deliveries` 表，因为它承载的是事件 envelope 的投递历史；但 endpoint 解析、header/body 渲染、secret 引用、host allowlist、retry 策略应复用 sink 能力，不再在 Hook 里维护一套平行配置。

### 2.6 sink-ref 是 workspace 级资源引用

`--sink <sink-ref>` 的隔离级别必须是 workspace，不允许跨 workspace 引用。

解析规则：

- `sink-ref` 可以是 sink name 或 sink UUID。
- 以 name 解析时，只在当前 request/runtime workspace 内查找。
- 以 UUID 解析时，也必须校验该 sink 的 `workspace_id` 等于当前 workspace。
- project 范围 Hook / Notification Rule 可以进一步限制事件来源 project，但不能引用其他 workspace 的 sink。
- 不存在、已删除、跨 workspace、无权限读取的 sink 都对调用者表现为 `notification_sink_not_found` 或统一的 `sink_not_found`，避免泄漏其他 workspace 的资源存在性。

数据约束：

- sink name 应在 `(workspace_id, name)` 上唯一。
- Hook / Notification Rule 保存 `sink_id`，不保存 sink name。
- delivery 生成时冻结已解析 sink 的 endpoint/template 结果；后续 sink 改名不影响历史 delivery。

---

## 3. 设计目标

首版完成后应支持：

- 用户可以注册 `project.annotated` 和 `project.denotated` Hook。
- 用户可以为事件创建 notification rule，并通过 `--sink openclaw` 或其他 sink 发送通知。
- 用户可以为 Hook 配置 `--sink audit-stream`。
- 任务依赖解除阻塞时触发 `task.unblocked` 事件。
- 事件通知复用现有 notification delivery 的重试、dead-letter、replay、endpoint 安全、模板渲染和 delivery 冻结。
- 事件通知具备 workspace/project 隔离、权限校验、审计和幂等。
- 文档中清晰区分 Hook、reminder rule、event notification rule。

---

## 4. 非目标

本规格不做：

- 不保留 `hook add --url` 兼容入口。当前能力尚未上线，应直接收敛到 `hook add --sink`。
- 不做任意脚本执行。
- 不内置飞书、Slack、邮件、OpenClaw 等具体 adapter。
- 不做复杂工作流引擎、条件分支、审批、状态自动推进。
- 不让事件通知自动启用。没有显式 notification rule 就不发用户通知。
- 不把 event notification rule 混入 reminder rule 的 schedule/filter 语义。
- 不修改 Taskwarrior 兼容的任务生命周期语义。

---

## 5. 事件类型

### 5.1 首版新增或补齐的事件

| 事件类型 | 对象 | 触发时机 | 首要用途 |
|---|---|---|---|
| `project.annotated` | project | 新增项目 annotation 后 | 通知项目关注者、同步项目时间线 |
| `project.denotated` | project | 删除项目 annotation 后 | 审计同步、通知项目关注者 |
| `task.unblocked` | task | 任务因依赖完成而从 blocked 变为非 blocked | 通知负责人可以开始处理 |

说明：

- `project.denotate` 是 CLI / audit action 的动作名。
- `project.denotated` 是事件名，使用过去式表示事实已经发生。
- 如果用户口头说 `project.denotate`，实现和文档应引导到事件名 `project.denotated`，不要新增两个重复事件。

### 5.2 暂缓的事件

`task.blocked` 有价值，但首版暂缓。原因是“变 blocked”的来源更多：

- 添加依赖。
- 依赖任务从 completed 被改回 pending。
- 批量导入或外部同步改变 depends。

首版先做 `task.unblocked`，因为它对应明确的用户通知场景：阻塞解除后提醒负责人继续处理。`task.blocked` 可在后续规格中补充。

### 5.3 下一大版本候选事件（不进入当前实现）

下一大版本可以把日常工作中高频、可触发通知或 Agent 接手的事件补齐。它们不进入本轮实现计划，但应在下个版本中单独落规格和计划。

设计原则：

- 不为每个字段变化都新增事件；低价值变化继续由 `task.modified` 覆盖。
- 只有能驱动用户通知、Agent 行动、外部系统同步的变化，才升格为语义事件。
- 每个事件都必须带 enough context，不让消费者再立刻反查才能知道发生了什么。
- 所有用户字段继续使用 `task.UserInfo`，不输出裸 UUID。

#### 任务事件 payload 基线

下一大版本里的所有 `task.*` 语义事件，只要表格中写到 `task`，都表示必须携带一份标准 task 快照，而不是只给 task UUID。标准 task 快照用于通知模板、OpenClaw 消息、第三方系统同步和 Agent 决策，至少包含：

```json
{
  "uuid": "task-uuid",
  "working_id": 12,
  "workspace_id": "workspace-id",
  "description": "联调客户端",
  "status": "pending",
  "project": {
    "id": "project-id",
    "slug": "api",
    "name": "API"
  },
  "priority": "H",
  "entry": 1780992000,
  "modified": 1780995600,
  "due": 1781082000,
  "start": null,
  "end": null,
  "wait": null,
  "scheduled": null,
  "until": null,
  "tags": ["backend", "urgent"],
  "assignees": [
    {
      "id": "user-id",
      "name": "Alice",
      "email": "alice@example.com",
      "external_ids": []
    }
  ],
  "depends": [
    {
      "uuid": "dependency-task-uuid",
      "description": "准备接口文档",
      "status": "completed"
    }
  ],
  "blocked": false,
  "urgency": 12.3
}
```

字段约定：

- `working_id` 是当前 workspace 内的数字 ID，便于消息里直接展示 `#12`；跨 workspace 不保证唯一。
- `project` 没有项目时为 `null`；不要只输出 project slug。
- `assignees`、`changed_by`、`created_by`、`deleted_by` 等用户字段都使用 `task.UserInfo`。
- 时间字段统一使用 Unix 秒；未设置时为 `null`，不要输出本地化字符串。
- `depends` 默认使用轻量 task ref，包含 `uuid`、`description`、`status`，必要时可补 `project`、`due`、`assignees`。
- `urgency` 如果当前上下文可以稳定计算就输出；不能稳定计算时可以省略，但不允许输出错误值。

事件自己的差异信息放在 `task` 之外，例如 `previous_due` / `current_due`、`added_tags` / `removed_tags`、`previous_assignees` / `current_assignees`。这样消费者既能拿到当前任务状态，也能明确知道本次变化是什么。

#### 优先级 1：日常协作和通知核心事件

| 事件类型 | 触发时机 | 必带差异信息 |
|---|---|---|
| `task.assigned` | 任务新增负责人 | `task`、`assignee`、`assigned_by`、`previous_assignees`、`current_assignees` |
| `task.unassigned` | 任务移除负责人 | `task`、`assignee`、`unassigned_by`、`previous_assignees`、`current_assignees` |
| `task.started` | 任务从未开始进入进行中 | `task`、`started_by`、`start`、`previous_start` |
| `task.stopped` | 任务停止进行 | `task`、`stopped_by`、`previous_start`、`current_start` |
| `task.blocked` | 任务从非 blocked 变为 blocked | `task`、`blockers`、`changed_dependency`、`blocked_reason` |
| `task.due_changed` | 任务 due 被新增、修改或清空 | `task`、`previous_due`、`current_due`、`changed_by` |
| `task.priority_changed` | 任务 priority 被新增、修改或清空 | `task`、`previous_priority`、`current_priority`、`changed_by` |
| `task.project_changed` | 任务所属 project 变化 | `task`、`previous_project`、`current_project`、`changed_by` |
| `task.tags_changed` | 任务标签集合变化 | `task`、`added_tags`、`removed_tags`、`current_tags`、`changed_by` |

说明：

- `task.blocked` 与 `task.unblocked` 要按状态跃迁触发，而不是每次修改 depends 都触发。
- `task.assigned` / `task.unassigned` 面向人非常高频，应优先支持 notification rule 的 `audience=assignee` 或 `actor`。
- `task.due_changed` 不替代 `task.due_soon` / `task.overdue`；前者是字段变化事件，后者是时间条件事件。

#### 优先级 2：上下文、知识沉淀和项目同步事件

| 事件类型 | 触发时机 | 必带差异信息 |
|---|---|---|
| `task.annotated` | 任务新增 annotation | `task`、`annotation`、`created_by` |
| `task.denotated` | 任务删除 annotation | `task`、`annotation.id`、`deleted_by` |
| `task.link_added` | 任务新增外部链接 | `task`、`link`、`created_by` |
| `task.link_removed` | 任务删除外部链接 | `task`、`link.id`、`removed_by` |
| `project.created` | 新建项目 | `project`、`created_by` |
| `project.updated` | 项目名称、描述或设置变化 | `project`、`changed_fields`、`previous_values`、`current_values`、`changed_by` |
| `workspace.member_added` | workspace 新增成员 | `workspace`、`member`、`role`、`added_by` |
| `workspace.member_removed` | workspace 移除成员 | `workspace`、`member`、`removed_by` |
| `workspace.member_role_changed` | workspace 成员角色变化 | `workspace`、`member`、`previous_role`、`current_role`、`changed_by` |

说明：

- `task.annotated` / `task.denotated` 与本轮的 `project.annotated` / `project.denotated` 保持命名一致。
- `project.updated` 不建议拆成 `project.name_changed`、`project.description_changed` 等小事件；通过 `changed_fields` 表达即可。
- workspace member 事件主要面向审计、Agent 权限同步和外部系统成员同步，不默认给所有成员发通知。

#### 定时条件事件的边界

`task.due_soon`、`task.overdue` 这类事件本质上不是业务写操作产生的事实，而是 scheduler 对时间条件的判断结果。下一大版本如果要把它们纳入统一事件体系，应从 reminder rule 生成，至少带：

- `task`
- `rule`
- `recipient`
- `window_start`
- `window_end`
- `sequence`
- `overdue_sequence`

这类事件不应由任务 `due` 字段变化直接触发；`due` 字段变化只触发 `task.due_changed`。

### 5.4 事件 envelope

事件 envelope 应保持稳定结构：

```json
{
  "event_id": "uuid",
  "event_type": "task.unblocked",
  "event_version": 1,
  "occurred_at": 1780995600,
  "actor_user": {
    "id": "user-id",
    "name": "Alice",
    "email": "alice@example.com",
    "external_ids": []
  },
  "workspace_id": "workspace-id",
  "workspace_slug": "local",
  "project_id": "project-id",
  "project_slug": "api",
  "object_kind": "task",
  "object_id": "task-uuid",
  "data": {}
}
```

对外 JSON 中涉及用户身份时必须使用 `task.UserInfo` 语义，不输出裸 UUID。当前内部事件里如果只有 `actor_user_id`，应在 HTTP/CLI/MCP/delivery payload 输出层解析成完整 `actor_user`。

---

## 6. Project Annotation 事件

### 6.1 Hook 可注册

Hook 事件白名单必须补充：

- `project.annotated`
- `project.denotated`

创建示例：

```bash
xuanchu hook add project-timeline \
  --event project.annotated \
  --event project.denotated \
  --sink audit-stream
```

验收要求：

- `hook add --event project.annotated` 成功。
- `hook add --event project.denotated` 成功。
- 未知事件仍然拒绝。
- README、manual hooks、MCP/HTTP 文档中的事件列表同步更新。

### 6.2 Payload 要求

`project.annotated` 的 `data` 至少包含：

```json
{
  "project": {
    "id": "project-id",
    "workspace_id": "workspace-id",
    "slug": "api",
    "name": "API",
    "status": "active"
  },
  "annotation": {
    "id": "annotation-id",
    "entry": 1780995600,
    "content": "完整内容",
    "content_preview": "前 200 字",
    "created_by": {
      "id": "user-id",
      "name": "Alice",
      "email": "alice@example.com",
      "external_ids": []
    },
    "created_at": 1780995600
  }
}
```

`project.denotated` 的 `data` 至少包含：

```json
{
  "project": {
    "id": "project-id",
    "workspace_id": "workspace-id",
    "slug": "api",
    "name": "API",
    "status": "active"
  },
  "annotation": {
    "id": "annotation-id"
  }
}
```

删除事件不要求保留完整已删内容。若删除前已经读取到 annotation，可把 `content_preview` 作为额外字段放入 payload，但不要为此引入复杂软删除。

---

## 7. `task.unblocked` 事件

### 7.1 触发语义

当某个任务完成后，Xuanchu 应检查依赖它的未结束任务。对于每个候选任务：

1. 业务变更前它处于 blocked。
2. 当前依赖变更后，它不再 blocked。
3. 它仍属于当前 workspace。
4. 它不是 completed/deleted。

同时满足以上条件时，生成 `task.unblocked`。

典型场景：

```bash
# 任务 2 依赖任务 1
xuanchu add "准备接口文档"
xuanchu add "联调客户端" depends:1

# 任务 1 完成后，任务 2 不再 blocked
xuanchu 1 done

# Xuanchu 生成 task.unblocked，OpenClaw 可通知任务 2 负责人
```

### 7.2 多依赖场景

如果任务 2 同时依赖任务 1 和任务 3：

- 完成任务 1 后，任务 3 仍未完成，任务 2 仍 blocked，不生成 `task.unblocked`。
- 完成任务 3 后，所有依赖都完成，任务 2 由 blocked 变为非 blocked，生成一次 `task.unblocked`。

### 7.3 Payload 要求

`task.unblocked` 的 `data` 至少包含：

```json
{
  "task": {
    "uuid": "dependent-task-uuid",
    "description": "联调客户端",
    "status": "pending",
    "project": "api",
    "assignees": []
  },
  "dependency": {
    "completed_task": {
      "uuid": "dependency-task-uuid",
      "description": "准备接口文档",
      "status": "completed"
    },
    "remaining_blockers": []
  },
  "unblocked": true
}
```

字段说明：

- `task` 是被解除阻塞的任务，也是 notification audience 默认解析的 primary task。
- `dependency.completed_task` 是刚刚完成并导致解除阻塞的任务。
- `remaining_blockers` 首版可以为空数组；如果实现方便，也可以填入未完成依赖列表。对 `task.unblocked` 来说它应为空。

---

## 8. Event Notification Rule

### 8.1 为什么不复用 reminder rule

reminder rule 的核心是 `schedule + task filter`，适合“每天 9:00 扫逾期任务”。事件通知规则的核心是“某个事件发生时立即通知”，不应该伪装成 schedule。

建议新增 `notification rule` 管理入口，并让 rule 支持两类 trigger：

- `trigger_kind=schedule`：现有 reminder rule 的长期归宿，可后续迁移。
- `trigger_kind=event`：本规格新增。

为了减少实现风险，首版可以只新增 event notification rule 表/服务，保持现有 `reminder rule` CLI 不动；但对外文档要说明两者同属 notification rule 家族。

### 8.2 规则字段

event notification rule 至少包含：

- `id`
- `workspace_id`
- `project_id`：可选；为空表示 workspace 范围。
- `name`
- `enabled`
- `trigger_kind`：固定为 `event`。
- `event_type`：如 `task.unblocked`。
- `filter_source`：可选，首版用于 task/project 的附加过滤。
- `audience_type`
- `recipient_user_ids_json`
- `sink_id`
- `template_subject`：可选。
- `template_body`：可选。
- `created_by`
- `created_at`
- `modified_at`

如果首版不引入自定义模板字段，可以先使用内置模板，但数据模型应预留模板字段，避免下一步迁移。

### 8.3 CLI

建议新增：

```bash
xuanchu notification rule add <name> \
  --event <event-type> \
  --audience <audience> \
  --sink <sink-ref> \
  [--project <project-ref>] \
  [--filter <query>] \
  [--template-body <template>]

xuanchu notification rule list [--all] [--event <event-type>]
xuanchu notification rule info <rule-ref>
xuanchu notification rule modify <rule-ref> [...]
xuanchu notification rule enable <rule-ref>
xuanchu notification rule disable <rule-ref>
xuanchu notification rule remove <rule-ref>
```

不允许：

```bash
xuanchu notification rule add task-unblocked --event task.unblocked --url https://example.test
```

应返回类似：

```text
notification rule uses --sink; direct --url is not supported
```

### 8.4 示例：OpenClaw 通知解除阻塞

```bash
xuanchu notification sink add openclaw \
  --type webhook \
  --url https://openclaw.example.com/xuanchu/notifications \
  --secret "$OPENCLAW_WEBHOOK_SECRET"

xuanchu notification rule add task-unblocked-openclaw \
  --event task.unblocked \
  --audience assignees \
  --sink openclaw \
  --template-body '{{recipient.name}}，任务「{{task.description}}」已解除阻塞，可以开始处理。阻塞项「{{dependency.completed_task.description}}」已完成。'
```

当任务 1 完成后：

1. Xuanchu 生成 `task.completed`。
2. Xuanchu 检查依赖关系，发现任务 2 从 blocked 变为 ready。
3. Xuanchu 生成 `task.unblocked`。
4. Hook consumer 如有匹配 Hook，生成 `hook_delivery`。
5. Notification consumer 匹配 `task.unblocked-openclaw`，解析任务 2 的 assignees。
6. Xuanchu 为每个负责人生成 `notification_delivery`，sink 为 `openclaw`。
7. notification dispatcher 投递给 OpenClaw。
8. OpenClaw 向用户发消息，用户可回复“开始处理”，OpenClaw 再通过 Xuanchu API/MCP 代表用户更新任务。

---

## 9. Audience

event notification rule 首版支持：

- `assignees`：primary task 的负责人。
- `explicit_users`：规则中显式指定的用户。
- `assignees_and_explicit_users`：二者合并去重。
- `actor`：事件操作者；默认不推荐用于 `task.unblocked`，但对 annotation 事件有用。

事件没有 primary task 时：

- `project.annotated` / `project.denotated` 使用 `actor` 或 `explicit_users`。
- 如果规则对 project 事件配置 `assignees`，应拒绝创建，或者在触发时标记为 `audience_empty`。推荐创建时拒绝，错误码 `audience_unsupported_for_event`。

---

## 10. 模板变量

event notification delivery 的模板上下文应包含：

- `event.id`
- `event.type`
- `event.occurred_at`
- `actor.id/name/email/external_ids`
- `workspace.id/slug`
- `project.id/slug/name/status`
- `recipient.id/name/email/external_ids`
- `task.*`：当事件 primary object 是 task，或 payload 中包含 primary task。
- `annotation.*`：project annotation 事件。
- `dependency.completed_task.*`：`task.unblocked`。

模板渲染继续沿用 notification sink 的现有机制。生成 delivery 时必须冻结：

- resolved URL
- rendered method
- rendered headers
- rendered body
- rendered content type
- payload JSON

retry/replay 不重新按当前模板渲染，除非已有 notification delivery replay 语义明确支持“重新渲染”选项。

---

## 11. 数据流

### 11.1 写操作事务

业务写操作应先完成领域变更，再生成事件。推荐流程：

1. app service 在同一业务上下文中完成 task/project 修改。
2. 计算需要生成的事件列表。
3. 写入 audit。
4. enqueue hook deliveries。
5. enqueue notification deliveries。

现有 `withAuditEntriesAndEvents` 已能把 audit 与 hook event 串起来。实现时应复用或扩展它，不要在 CLI/HTTP/MCP 层重复生成事件。

### 11.2 投递失败不回滚业务变更

外部通知失败不应让任务完成、项目 annotation 等业务写入失败。

首版可以保持“enqueue delivery 失败则返回错误”的现有实现风格，但规格目标是：

- 业务事实已经写入后，不因为远端不可达而回滚业务事实。
- dispatcher 投递失败只影响 delivery 状态。
- 如果 enqueue 阶段数据库写入失败，应返回错误并保留可追查日志；实施计划需要评估现有事务边界是否会回滚业务写入，并给出明确调整。

---

## 12. 幂等与去重

event notification delivery 的 dedupe key 建议包含：

- workspace_id
- rule_id
- event_id
- recipient_user_id

同一个事件、同一条规则、同一 recipient 只生成一条 notification delivery。

对于 `task.unblocked`，不要只用 `task_uuid` 做去重。任务可能经历：

1. 被解除阻塞。
2. 又新增依赖变 blocked。
3. 再次解除阻塞。

这应是两个不同事件，可以分别通知。

---

## 13. 权限与作用域

权限建议：

- `notification:read`：读取 notification rule、delivery。
- `notification:write`：新增、修改、启停、删除 notification rule。
- `hook:read` / `hook:write`：保持现有 Hook 权限。

作用域要求：

- workspace 范围 rule 只能匹配同 workspace 的事件。
- project 范围 rule 只匹配该 project 的事件。
- `task.unblocked` 如果 primary task 没有 project，则只匹配 workspace 范围 rule。
- sink 必须属于同一 workspace；sink-ref 的 name/UUID 解析都不能越过当前 workspace。
- rule 创建者必须有对应 workspace 的管理权限。

---

## 14. HTTP 与 MCP

### 14.1 HTTP

建议新增或扩展：

- `GET /api/v1/notification-rules`
- `POST /api/v1/notification-rules`
- `GET /api/v1/notification-rules/{ruleID}`
- `PATCH /api/v1/notification-rules/{ruleID}`
- `POST /api/v1/notification-rules/{ruleID}/enable`
- `POST /api/v1/notification-rules/{ruleID}/disable`
- `DELETE /api/v1/notification-rules/{ruleID}`

请求中使用 `sink` 或 `sink_id`，不接受 `url`。

HTTP 中的 `sink` / `sink_id` 都按当前请求解析出的 workspace 进行校验；即使传入其他 workspace 的 sink UUID，也必须返回 not found / permission denied 风格错误，不允许成功绑定。

Hook HTTP API 也应补充 sink 字段：

- 新建 Hook 时必须接受 `sink` 或 `sink_id`。
- 不接受 `url`。
- 如果传入 `url`，返回 `hook_url_not_supported`。
- `sink` / `sink_id` 必须属于当前 workspace。

### 14.2 MCP

MCP tool 必须使用下划线命名：

- `notification_rule_list`
- `notification_rule_add`
- `notification_rule_info`
- `notification_rule_modify`
- `notification_rule_enable`
- `notification_rule_disable`
- `notification_rule_remove`

不要新增 `notification.rule.add` 这种点号命名。

MCP 工具里的 `sink` 字段同样按 request scope 的 workspace 解析。Agent 即使知道其他 workspace 的 sink UUID，也不能跨 workspace 绑定。

---

## 15. 错误码

建议错误码：

- `event_type_unsupported`：事件类型不支持。
- `notification_rule_invalid`：规则字段非法。
- `notification_rule_not_found`：规则不存在。
- `notification_rule_url_not_supported`：notification rule 不接受直接 URL。
- `notification_sink_not_found`：sink 不存在。
- `notification_sink_disabled`：sink 已禁用。
- `sink_cross_workspace`：内部错误分类，表示实现发现跨 workspace 引用；对外可映射为 `notification_sink_not_found`，避免泄漏资源存在性。
- `hook_sink_required`：Hook 新接口缺少 sink。
- `hook_url_not_supported`：Hook 不接受直接 URL。
- `audience_unsupported_for_event`：audience 不适用于该事件。
- `audience_empty`：事件触发时没有解析到收件人。

`audience_empty` 不应导致业务操作失败。可以不生成 delivery，或生成状态为 skipped 的 delivery。首版推荐不生成 delivery，并记录 debug/audit 级事件，避免 delivery 表被空受众刷屏。

---

## 16. 文档同步要求

实现时必须同步更新：

- `README.md`
  - Hook 支持事件列表。
  - notification rule 示例。
- `ROADMAP.md`
  - 当前版本能力说明。
- `docs/manual/hooks.md`
  - Hook 和 notification rule 边界。
  - Hook 示例使用 `--sink`，不再出现 `--url` 配置方式。
  - 新增 `project.annotated` / `project.denotated` / `task.unblocked`。
- `docs/manual/notifications.md`
  - event notification rule 的 CLI、HTTP、MCP 和 OpenClaw 示例。
- `docs/manual/mcp.md`
  - 新增 MCP tool。
- `docs/skills/*`
  - 如果有涉及 Hook、notification、OpenClaw、MCP 调用的 skill，要同步避免旧的点号 tool 或直接 URL 通知表达。

---

## 17. 测试要求

### 17.1 App 层

- `AddHook` 接受 `project.annotated`。
- `AddHook` 接受 `project.denotated`。
- 未知事件仍被拒绝。
- `ProjectAnnotate` 生成 `project.annotated` delivery payload，包含 project 和 annotation。
- `ProjectDenotate` 生成 `project.denotated` delivery payload，包含 project 和 annotation id。
- 完成最后一个 blocker 时生成 `task.unblocked`。
- 仍有其他未完成依赖时不生成 `task.unblocked`。
- 已完成/已删除的 dependent task 不生成 `task.unblocked`。
- 同一 event/rule/recipient 不重复生成 notification delivery。

### 17.2 CLI

- `xuanchu hook add ... --event project.annotated` 成功。
- `xuanchu hook add ... --event project.denotated` 成功。
- `xuanchu notification rule add ... --event task.unblocked --sink openclaw` 成功。
- `xuanchu hook add ... --event project.annotated --sink audit-stream` 成功。
- 在 workspace A 中传入 workspace B 的 sink name/UUID 时失败。
- `xuanchu hook add ... --event project.annotated --url ...` 失败，并提示使用 `--sink`。
- `xuanchu notification rule add ... --event task.unblocked --url ...` 失败，并提示使用 `--sink`。
- `notification rule list/info/enable/disable/remove` 可用。

### 17.3 HTTP / MCP

- HTTP 创建 event notification rule 时接受 `sink`，拒绝 `url`。
- HTTP / MCP 传入跨 workspace sink UUID 时不能绑定成功。
- MCP `notification_rule_add` 可创建 event notification rule。
- MCP tool 名全部使用下划线。

### 17.4 Dispatcher / Delivery

- event notification rule 命中后生成 `notification_delivery`。
- delivery 使用指定 sink 的 endpoint/template。
- retry/replay 继续沿用 notification delivery 机制。
- sink disabled 时不生成可投递 delivery，或生成明确失败状态；实施计划必须固定一种行为并测试。

### 17.5 数据库

- SQLite 和 PostgreSQL 均通过迁移。
- `CGO_ENABLED=0 go test ./...` 通过。
- `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。

---

## 18. 验收标准

1. `project.annotated` / `project.denotated` 可以作为 Hook event 注册并实际投递。
2. `task.unblocked` 在依赖全部完成时生成，且多依赖场景不会提前生成。
3. 可以创建基于事件的 notification rule，并用 `--sink openclaw` 投递到 OpenClaw。
4. 可以创建基于 sink 的 Hook，并用 `--sink audit-stream` 投递原始事件。
5. notification rule 和 Hook 都不接受直接 URL；外部 endpoint 必须通过 sink 表达。
6. OpenClaw 这类 Agent 平台无需在 Xuanchu 里写死，只需作为 sink 配置。
7. 所有新增用户可见能力都有 README、manual、MCP 文档同步。
8. SQLite、PostgreSQL、`CGO_ENABLED=0` 测试和构建仍可通过。
