# 璇础 Workspace Agent 自动化设计

> **给 agentic workers 的要求：** 本规格通过评审后，先使用 `superpowers:writing-plans` 拆成实施计划，再开始编码。实现必须遵循 Red → Green → Refactor，并同步 HTTP/OpenAPI、Web Console 和用户文档。

**日期：** 2026-07-26
**状态：** 已完成
**目标版本：** v0.6.4
**依赖规格：** [Web Console 项目自动化与 OpenAI 兼容投递设计](2026-07-08-web-console-project-automation-openai-compatible-design.md)
**首要场景：** Project 创建完成后，由 Workspace 规则把 Project、初始 config 和 `project.created` 事件交给 Yaoguang Agent；Agent 使用已配置的璇础 MCP 完成外部资源创建，并用 `project_config_set` 回写结果。

## 1. 产品结论

Workspace 自动化不是一套新的脚本系统，也不是给“集成 Yaoguang”增加一个硬编码 setup hook。它是现有 Project Automation 的另一个作用域：璇础可靠地产生事件、构造上下文并调用 OpenAI-compatible Agent Provider，具体业务流程继续由 Agent 根据自然语言指令和 MCP 工具完成。

```text
Project 创建事务
  -> Project / config / Task / Series / Project Automation 全部写入
  -> 匹配当前 Workspace 的 project.created 规则
  -> 在同一事务内生成冻结的 Automation Delivery（transactional outbox）
  -> 事务提交
  -> 现有 dispatcher 调用 Workspace 配置的 Agent Provider
  -> Agent 阅读自然语言指令并调用自身工具 / 璇础 MCP
  -> Agent 通过 project_config_set 回写业务结果
  -> 璇础 Audit 记录事实
```

锁定以下边界：

- 复用现有 OpenAI-compatible action、提示词、上下文渲染、预览、dispatcher、重试、replay 和 Delivery 运行记录。
- 规则统一为 `workspace | project` 两种 scope，不新增 `SetupAction`、用户脚本、工作流 DSL、资源绑定表或 Setup Run 表。
- Workspace 事件规则首个开放事件是 `project.created`；分支、判断、调用顺序和回写逻辑写在自然语言指令中。
- Workspace 定时规则复用现有 cron/schedule 能力，只提供 Workspace 上下文；Agent 如需处理多个 Project，通过 MCP 自行查询。
- Workspace 规则的 Provider 配置只从 **Workspace scope** 解析。事件中的 Project config 是业务上下文，不能反向覆盖执行该规则的 Provider。
- HTTP 2xx 只证明本次 Agent Provider 调用成功，不证明知识库等外部业务动作已经完成。外部资源 ID 的 MCP 回写和对应 Audit 才是业务完成事实。
- 系统提供 at-least-once，不承诺 exactly-once。指令和工具必须按 `event_id`、稳定 external ref 和结果 config 共同实现幂等。

## 2. 为什么复用 Agent，而不是新增 Setup Action

用户希望表达的流程通常是：

```text
如果存在 feishu_drive_folder_token
且当前 Project 还没有 knowledge_id
则调用 Yaoguang 能力创建知识库
成功后把知识库 ID 写回 knowledge_id
```

这不是稳定到值得固化为璇础领域代码的项目生命周期规则，而是一个会随外部平台、工具能力和业务政策变化的编排。已有 Agent Provider 和 MCP 已经分别解决“理解指令”和“受控地读写璇础”，因此最短路径是：

- 璇础负责确定何时运行、给出可信上下文、可靠调用和记录。
- Agent 负责理解自然语言条件、选择工具、执行多步操作和处理外部 API 差异。
- MCP 负责结构化写入、权限校验、审计和错误返回。

不采用用户脚本有三个直接收益：不需要在璇础内建设代码沙箱，不需要定义另一套 SDK/运行时，也不会让外部系统细节进入 Project 创建事务。

## 3. 现状与问题

### 3.1 已有可复用能力

- `ProjectAutomationRule` 已支持 event/schedule trigger、OpenAI-compatible action、instruction/system prompt、上下文选择和 Provider config key。
- `ProjectAutomationDelivery` 已是可领取、可重试、可 replay 的可靠运行记录，状态为 `queued / delivering / retry_wait / succeeded / dead_lettered`。
- v0.6.1 已把 Project Automation 的定时规则扩展到统一 `internal/schedule` cron 语义。
- MCP 已暴露 `project_config_list`、`project_config_set`、`project_config_unset`，写操作进入 Audit。
- `agent.provider.base_url/api_key/model/protocol/allowed_hosts` 已允许 Workspace scope 配置。
- Project Template 实例化已经能在同一事务中创建 Project、project config、Task、Series 和 disabled Project Automation。

### 3.2 当前缺口

- `ProjectAutomationRule.ProjectID` 当前为非空，规则只能属于具体 Project。
- 事件匹配只查当前 Project 的规则，不能匹配 Workspace 规则。
- 当前没有 `project.created` 语义事件；普通 `AddProject` 只写 Audit。
- 模板实例化提交后只发送 Task 事件，没有“初始化内容全部完成”的 Project 事件。
- 现有部分事件在业务事务提交后才 enqueue，且调用方会忽略 enqueue 错误，存在事件永久丢失窗口。
- Project Automation 当前按 Project effective config 解析 Provider。若直接照搬到 Workspace 规则，新 Project 自己的 config 会改变 Workspace 规则调用哪个 Agent，边界错误。

## 4. 目标

1. 为当前 Workspace 提供统一自动化控制台，管理 Workspace scope 规则及运行记录。
2. 把现有 Project Automation 的领域模型泛化为 `AutomationRule` 和 `AutomationDelivery`，保留现有项目页体验。
3. 支持 Workspace `project.created` 事件规则和 Workspace schedule 规则。
4. `project.created` 必须表示整个 Project 初始化事务已经完成，而不是刚插入 Project row。
5. Project 创建和匹配到的 Delivery 必须原子落库，消除提交后 enqueue 丢事件窗口。
6. Agent 能收到稳定、脱敏、可追踪的 Workspace、Project、初始 config 和事件上下文。
7. Agent 能通过现有 MCP 回写 Project config，并由 Audit 证明结果。
8. 现有 Project Automation API、项目页入口、模板捕获与实例化语义保持兼容。

## 5. 非目标

- 不执行用户提供的 shell、JavaScript、Go、Python 或其它脚本。
- 不新增可视化节点编排器、条件 DSL、循环、审批、人工作业或补偿事务引擎。
- 不在璇础内置“创建 Yaoguang 知识库”“同步飞书文件夹”等特定 action。
- 不让 Workspace 自动化直接访问 Yaoguang MCP；璇础只调用 Agent Provider，由 Agent 使用其已配置工具。
- 不新增 completion callback，不轮询 Agent 的外部业务状态。
- 不把 Agent 的文本回答自动解析成 `knowledge_id`；结果必须通过 MCP 结构化写入。
- 不把 Workspace Automation 复制进 Project Template；模板仍只捕获 Project scope 规则。
- 首版不把 Workspace 规则 CRUD 暴露为 MCP tool，避免 Agent 默认具备修改自身触发器的能力。
- 首版不把 `project.created` 同时开放给 Hook 和 Notification Rule；它先进入统一 Automation 事件路由，后续消费者可复用同一事件契约。
- 不保证外部动作 exactly-once，也不对外部系统做跨系统事务回滚。

## 6. 统一概念模型

### 6.1 Scope

```go
type AutomationScopeType string

const (
    AutomationScopeWorkspace AutomationScopeType = "workspace"
    AutomationScopeProject   AutomationScopeType = "project"
)

type AutomationScope struct {
    Type AutomationScopeType
    ID   string
}
```

规则匹配：

| 规则 scope | event 条件 | schedule 上下文 |
|---|---|---|
| `project` | `event.workspace_id` 相同且 `event.project_id == scope_id` | 当前 Project |
| `workspace` | `event.workspace_id == scope_id`，再匹配事件类型 | 当前 Workspace，不隐式遍历 Project |

`scope_id` 始终非空，避免用 nullable `project_id` 表达唯一性时在 SQLite/PostgreSQL 出现不同的 NULL unique 语义。

### 6.2 Trigger

继续支持两类 trigger：

- `event`：结构化 matcher 只负责事件类型等稳定路由条件。
- `schedule`：复用 `internal/schedule` 的 schedule type/value/timezone、cron 解析、去重和停机补偿语义。

首版 Workspace event 白名单只有：

- `project.created`

首版不增加 config 表达式条件，例如“不为空”“等于某值”。这些业务判断由 Agent 使用事件中的 `project_config` 或 MCP 完成，避免一边使用自然语言 Agent，一边又维护功能重叠的条件 DSL。

### 6.3 Action

动作继续固定为 `openai_compatible`。Workspace 规则不增加 `yaoguang` action type；Yaoguang 只是一个可用的 OpenAI-compatible Agent Provider。

### 6.4 Delivery 就是运行记录和 outbox

不新增 `AutomationRun` 或 `EventOutbox` 表。`AutomationDelivery` 同时承担：

- 事件匹配结果；
- 冻结的目标 URL、method、headers、request body 和脱敏 preview；
- dispatcher 的可靠队列；
- 尝试次数、响应摘要、Provider request ID、错误和 replay 来源；
- Web Console 的运行记录。

业务事务内只做规则匹配、上下文渲染和 Delivery 落库，不做外部 HTTP。dispatcher 在事务提交后异步领取 Delivery。

## 7. `project.created` 事件契约

### 7.1 发生时点

`project.created` 只在创建事务末尾产生。此时同一事务内的以下内容必须已经写完：

- Project 基础字段；
- 显式 project config；
- 从模板创建的 Task；
- 从模板创建的 Task Series 和规则版本；
- 从模板创建且默认 disabled 的 Project Automation；
- Project 创建 Audit。

普通空项目创建也必须改为事务：`Project + Audit + Automation Delivery` 全部提交或全部回滚。

### 7.2 只触发一次

- 创建成功产生一个全局唯一 UUID `event_id`。
- Project 修改、补填 config、模板保存、Project reopen 都不再次产生 `project.created`。
- 事务回滚不留下事件或 Delivery。
- 同一规则与事件使用唯一去重键 `event:{rule_id}:{event_id}`；数据库 unique constraint 保证重试路由不会重复生成 Delivery。

### 7.3 来源 metadata

事件 metadata 至少包含：

```json
{
  "source": "empty|template",
  "source_template_id": "optional-template-id",
  "source_template_snapshot_id": "optional-snapshot-id",
  "source_template_snapshot_hash": "optional-sha256",
  "initial_task_count": 12,
  "initial_series_count": 2,
  "initial_project_automation_count": 1
}
```

空项目省略三个 template 字段，count 为 `0`。事件不携带 Snapshot JSON、secret input、模板密文或创建请求原文。

### 7.4 事件 payload

```json
{
  "event_id": "...",
  "event_type": "project.created",
  "event_version": 1,
  "occurred_at": 1785056400,
  "workspace": { "id": "...", "slug": "product", "name": "产品研发" },
  "project": {
    "id": "...",
    "slug": "atlas",
    "name": "Atlas",
    "description": "...",
    "status": "planning",
    "created_by": { "id": "...", "name": "alice", "display_name": "Alice", "email": null, "external_ids": [] },
    "created_at": 1785056400
  },
  "actor": {
    "type": "user",
    "user": { "id": "...", "name": "alice", "display_name": "Alice", "email": null, "external_ids": [] }
  },
  "metadata": { "source": "template", "initial_task_count": 12 }
}
```

用户引用必须继续使用 `task.UserInfo` / `task.JSONUserInfo`，不输出裸 UUID。`project_config` 不直接塞进领域事件 payload，而由 Automation context builder 以脱敏后的创建时快照加入 Agent 请求。

## 8. 事件路由与可靠性

新增通用 App 层入口，概念签名为：

```go
func (s *Service) RouteAutomationEventTx(
    txRepo AutomationRepository,
    event AutomationEvent,
    context AutomationContextSnapshot,
) error
```

它不属于 `internal/cli`、HTTP 或 storage 业务逻辑。执行顺序：

1. 校验 event version、workspace/project scope 和 payload 大小。
2. 查询 `workspace_id` 下、`enabled=true`、`trigger_type=event` 且 matcher 命中的规则。
3. 同时支持 Project scope 精确匹配和 Workspace scope 匹配；`project.created` 首版只会命中 Workspace 白名单。
4. 使用事件发生时的规则、Workspace Provider config 和 context snapshot 渲染请求。
5. 为每条规则插入冻结的 Delivery，去重键为 `event:{rule_id}:{event_id}`。
6. 返回给外层事务；只有所有必要 Delivery 都成功落库，Project 创建事务才提交。

规则在事件之后新增或启用，不补跑历史 Project；规则在 Delivery 落库后被编辑、禁用或删除，不改变已经冻结的 Delivery。

Provider config 缺失、allowed host 拒绝、body 超限等规则运行错误不能吞掉 Project 创建：系统仍创建一条 `dead_lettered` Delivery，记录稳定错误码和脱敏错误。只有数据库写入、内部序列化不变量破坏等无法形成可靠记录的错误才回滚 Project 创建。

现有 post-commit `enqueueProjectAutomationEvents` 不能再成为新事件的唯一可靠路径。后续把其它 Automation 事件迁入 `RouteAutomationEventTx` 时沿用同一契约；Hook/Notification 的事件可靠性另行设计，不能通过复制本功能的 Delivery 解决。

## 9. Agent 请求上下文

### 9.1 默认上下文

Workspace `project.created` 规则默认包含：

```json
{
  "_xuanchu": {
    "automation_rule_id": "...",
    "automation_scope": "workspace",
    "delivery_id": "...",
    "event_id": "...",
    "trigger_type": "event",
    "triggered_at": 1785056400
  },
  "workspace": { "id": "...", "slug": "product", "name": "产品研发" },
  "project": { "id": "...", "slug": "atlas", "name": "Atlas", "status": "planning" },
  "project_config": {
    "feishu_drive_folder_token": "fldcn..."
  },
  "event": { "event_type": "project.created", "event_version": 1, "metadata": {} }
}
```

`project.created` 的 `project_config` 使用创建事务末尾的 effective config 快照，包含来源于 project/workspace/default 的普通值；每项内部保留 source/status 信息用于调试，投递给模型的便捷 map 只给合法有效值，缺失项不出现在 map 中。这样延迟投递时不会误读创建后刚被修改的值。

所有 `ConfigDefinition.secret=true` 的 key/value 都不得进入 instruction variables、event payload、request body、preview、Delivery response、日志或错误。Provider API key 只在发送时解密并写入 Authorization，不冻结明文 header。

### 9.2 Workspace schedule 上下文

Workspace schedule 不绑定 Project，默认只包含 `_xuanchu` 和 `workspace`。不默认把 Workspace 内全部 Project、Task 或 config 塞进请求。Agent 根据指令使用 MCP 的查询工具分页读取，避免请求体随 Workspace 规模无界增长。

### 9.3 Provider 配置解析

Workspace 规则的以下 key **只读取 Workspace scope row/default**，不走 Project effective config：

- `agent.provider.base_url`
- `agent.provider.api_key`
- `agent.provider.model`
- `agent.provider.protocol`
- `agent.provider.allowed_hosts`

规则可以保存 `model_override` 等现有非 secret override。自定义 Provider config key 也必须由 ConfigDefinition 明确允许 Workspace scope；不能通过选择 Project 样本让它从 Project scope 取值。

Project scope 规则继续保持当前 effective config 语义，避免改变既有规则行为。

Provider 配置仍存放在现有 ConfigDefinition / Config repository 中，不新增 Provider 表。但 Automation 控制台不得直接通过通用 config list/get 读取 `agent.provider.api_key`：通用 Workspace Config HTTP 当前只开放业务 config key，而 Project config list 又会返回原始值，两者都不适合作为密钥配置界面契约。App 层提供统一的安全 Provider config facade，Workspace 与 Project 只在解析 scope 和权限上不同。

安全读取 DTO 只返回：

```json
{
  "base_url": "https://agent.example.com",
  "model": "yaoguang-operator",
  "allowed_hosts": ["agent.example.com"],
  "api_key_set": true,
  "complete": true,
  "missing_fields": []
}
```

它永远不返回 API key、密钥掩码或可逆密文。安全写入 DTO 接受 `base_url/model/allowed_hosts` 和可选 `api_key`；`api_key` 省略或为空表示保留已有值，`clear_api_key=true` 才显式清除，两者同时出现时返回 400。保存必须复用现有 config schema 校验、secret encryption、scope 校验和 `config.set/unset` Audit，不允许绕过 Config service 直接写 repository。

## 10. 自然语言编排契约

Workspace Automation 的核心可扩展点是 instruction，而不是新 action。控制台在指令输入框旁提示作者把四类事实写清楚：

1. **目标**：最终要创建或同步什么。
2. **前置条件**：缺少哪些 config 时直接结束。
3. **幂等条件**：哪些已有值或 external ref 表示已经处理。
4. **结果写入**：成功后用哪个 MCP tool 写到哪个 config key；失败时不得写半成品。

知识库初始化示例：

```text
你负责初始化新项目的 Yaoguang 知识库。

1. 读取事件中的 workspace、project 和 project_config。
2. 如果 feishu_drive_folder_token 为空，不创建任何资源，直接说明跳过原因。
3. 先用 project_config_list 再次检查 knowledge_id；若已有非空值，视为已完成，不重复创建，也不覆盖。
4. 使用稳定 external ref `xuanchu:{workspace.id}:{project.id}:knowledge-base` 调用可用工具创建或取得知识库，
   并把 feishu_drive_folder_token 作为其飞书目录来源。
5. 只有外部工具明确返回有效知识库 ID 后，才调用 project_config_set，
   project 使用 project.slug，key 为 knowledge_id，value 为知识库 ID。
6. 外部调用失败或结果不确定时，不写 knowledge_id；返回简短错误供运行记录诊断。
```

生产命名建议使用带归属的 config key，例如 `yaoguang.knowledge_base.id`，避免通用的 `knowledge_id` 与其它知识库提供方冲突；这只是 schema 建议，不进入 Automation 引擎硬编码。

## 11. 幂等、完成语义与失败处理

### 11.1 三层幂等

| 层 | 机制 |
|---|---|
| 璇础路由 | `event:{rule_id}:{event_id}` unique dedupe，不重复创建 Delivery |
| Agent 指令 | 先读结果 config；已有值立即 no-op |
| 外部系统 | 使用 `xuanchu:{workspace_id}:{project_id}:{purpose}` 稳定 external ref 做 create-or-get |

`project_config_set` 是 upsert，但 upsert 本身不能防止 Agent 先创建两个外部资源，因此仍必须先检查 config，并让外部创建接口支持稳定 external ref。

### 11.2 完成语义

- `Delivery.status=succeeded`：OpenAI-compatible HTTP 返回可接受的 2xx，Provider 调用成功。
- `project_config_set` Audit：Agent 已把结构化结果写回璇础。
- 外部知识库真实存在且可用：由外部工具契约保证，不由璇础推断。

控制台不能把 `succeeded` 翻译成“知识库创建成功”。运行详情应显示“Agent 调用成功”；管理员通过 Project config 和 Audit 查看业务结果。

### 11.3 重试与 replay

- 429/5xx/网络错误沿用 `retry_wait` 和最大尝试次数；超过后 `dead_lettered`。
- 网络超时可能发生在 Provider 已接受请求之后，因此每次重试仍传相同 `delivery_id/event_id`，请求 body 保持冻结。
- replay 创建新的 Delivery ID，保存 `replay_of_delivery_id`，但继承原事件 ID、冻结 request body 和原目标；Agent 的业务幂等检查必须让重复执行安全。
- 关联 Project 已关闭时禁止手动 test/replay；历史记录仍可读。Workspace schedule 没有关联 Project，不受此限制。

## 12. 数据模型与迁移

### 12.1 物理表泛化

把物理表和 Go model 一次性泛化，避免长期保留名为 `project_automation_*`、实际又装 Workspace 规则的误导结构：

```text
project_automation_rules       -> automation_rules
project_automation_deliveries  -> automation_deliveries
```

`automation_rules` 关键字段：

```text
id
workspace_id
scope_type               // workspace|project
scope_id                 // workspace_id 或 project_id
name
description
enabled
trigger_type
trigger_config_json
condition_json
action_type
action_config_json
context_config_json
instruction_template
system_prompt
created_by_*
created_at / modified_at

UNIQUE(workspace_id, scope_type, scope_id, name)
INDEX(workspace_id, scope_type, scope_id, enabled, trigger_type)
```

`automation_deliveries` 在现有字段上调整：

```text
id
workspace_id
rule_scope_type           // 冻结历史 scope
rule_scope_id
project_id NULL           // event 有具体 Project 时填写；Workspace schedule 为 NULL
rule_id
trigger_type
event_id / event_type
dedupe_key UNIQUE
replay_of_delivery_id NULL
api_key_config_key          // 冻结 secret 引用，不冻结 secret 明文
allowed_hosts_config_key    // replay 时按当前策略重新校验冻结 URL
max_attempts                // 冻结本次 Delivery 的重试上限
...现有冻结请求、响应和重试字段
```

dispatcher 发送 Delivery 时不得重新读取 Rule。它只根据 Delivery 冻结的 scope、Project、`api_key_config_key` 和 `allowed_hosts_config_key` 读取当前 secret/安全策略；因此规则被修改或删除后，已生成 Delivery 仍可投递和诊断。`max_attempts` 也不能随当前 Rule 漂移。

迁移必须：

1. 在 SQLite/PostgreSQL 手工 migration 中重命名旧表并增加字段，不能只依赖 AutoMigrate 猜测 nullable/unique 变化。
2. 旧规则回填 `scope_type=project, scope_id=project_id`。
3. 旧 Delivery 回填 `rule_scope_type=project, rule_scope_id=project_id`，保留原 ID、dedupe key、状态和时间。
4. 校验迁移前后规则数、Delivery 数和主键集合一致。
5. 移除 App/storage 对 `ProjectAutomationRule.ProjectID not null` 的假设。
6. 迁移是 pre-1.0 的一次性前向 schema 变更；发布前必须备份，回滚旧 binary 需要同时恢复迁移前数据库。

### 12.2 App 命名与兼容 facade

领域和 storage 使用 `AutomationRule/AutomationDelivery`。现有 Project API 可以保留 `ProjectAutomation*` HTTP DTO 和 service facade，但内部必须调用统一 Automation service，不复制 CRUD、渲染或 dispatcher。

Project Template Snapshot 中的 Automation blueprint 继续表示 Project scope 规则；Capture 只选择当前 Project 的规则，Instantiate 写入 `scope_type=project, scope_id=<new-project-id>` 且 disabled。Workspace 规则不进入 Snapshot 闭包，也不因模板实例化被复制。

## 13. 权限、安全与审计

### 13.1 权限

不新增 `automation:*` token scope。首版延续 Automation 属于“作用域治理 + 出站执行”的双权限边界：

| 操作 | token scope | role permission |
|---|---|---|
| Workspace 规则/运行记录读取 | `workspace:read` + `hook:read` | `workspace.read` + `hook.read` |
| 新建、编辑、启停、删除 | `workspace:write` + `hook:write` | `workspace.modify` + `hook.write` |
| preview/test/replay | `workspace:write` + `hook:write` | `workspace.modify` + `hook.write` |
| 查看/编辑 Workspace Provider config | 复用 `config:read/write` | 复用现有 Workspace config 的 `workspace.read/modify` |

按当前角色矩阵，owner/admin 可治理 Workspace 自动化；member/viewer 没有 `hook.read`，因此不显示顶层入口，也不能读取运行内容。服务端是最终权限事实，前端隐藏入口只用于减少噪声。

Project Automation 继续使用 `project:read|write + hook:read|write`，不因本规格放宽。

### 13.2 安全

- preview、test、正式投递和 replay 共用 allowed host / SSRF 校验。
- Provider secret 只在 send time 解密，不写入冻结 headers、request preview、响应、Audit、日志或错误。
- Project config 中 secret definition 的 key/value 均不进 Agent 上下文。
- instruction/system prompt 有大小限制，最终 request body 沿用现有上限。
- Agent MCP token 的 scope、Workspace/Project allowlist 独立校验；Automation 创建者权限不会透传为 Agent 权限。
- 外部 Agent 回写必须以其自身 Actor/token 进入 Audit，不能伪装成 Project 创建者。

### 13.3 Audit action

统一记录：

```text
automation.rule.created
automation.rule.modified
automation.rule.enabled
automation.rule.disabled
automation.rule.deleted
automation.rule.tested
automation.delivery.replayed
```

Audit metadata 保存 `scope_type/scope_id`、rule/delivery/project 引用和变更字段名，不保存完整 prompt、request body、response body 或 secret。Project 与 Workspace scope 共用同一 action 词汇。Agent 的 `project_config_set` 继续产生既有 config Audit，两个 Audit 通过时间、Project 和 Agent actor 关联，不新增业务结果绑定表。

## 14. HTTP、OpenAPI、CLI、Remote 与 MCP

### 14.1 Workspace HTTP API

当前 effective Workspace 下新增：

```text
GET    /api/v1/automations
POST   /api/v1/automations
POST   /api/v1/automations/preview
GET    /api/v1/automations/{ruleID}
PATCH  /api/v1/automations/{ruleID}
DELETE /api/v1/automations/{ruleID}
POST   /api/v1/automations/{ruleID}/enable
POST   /api/v1/automations/{ruleID}/disable
POST   /api/v1/automations/{ruleID}/preview
POST   /api/v1/automations/{ruleID}/test
GET    /api/v1/automation-template-vars

GET    /api/v1/automation-deliveries
GET    /api/v1/automation-deliveries/{deliveryID}
POST   /api/v1/automation-deliveries/{deliveryID}/replay
```

这些端点只管理/返回 `scope_type=workspace`。Project 端点路径保持不变，只返回 `scope_type=project`：

```text
/api/v1/projects/{projectRef}/automations
/api/v1/projects/{projectRef}/automation-deliveries
```

Workspace Delivery list 支持 `rule_id / project_ref / status / trigger_type / q / limit / offset` 过滤，`q` 对 delivery/event/provider request ID 做精确或前缀搜索。所有端点进入 Huma OpenAPI；错误码、pagination 和用户对象沿用现有规范。

规则 list/info 返回可选 `last_delivery` 安全摘要（ID、状态、HTTP 状态、时间），由 App/Storage 批量查询，前端不得按规则发 N+1 请求。Delivery view 除保留兼容用 `project_id` 外，还返回可选 Project 引用（ID、slug、name、status、URL）；Workspace schedule 两者都为空。

`GET /api/v1/automation-template-vars` 返回按 Workspace schedule / `project.created` 分组的变量描述，只描述可用变量，不读取 Project 值或 secret。Project 原有 `/projects/{projectRef}/automation-template-vars` 保持不变。

### 14.2 安全 Provider 配置 API

Automation 控制台使用专用的安全 facade，底层仍复用现有 Config storage/service：

```text
GET /api/v1/automations/provider-config
PUT /api/v1/automations/provider-config

GET /api/v1/projects/{projectRef}/automations/provider-config
PUT /api/v1/projects/{projectRef}/automations/provider-config
```

- Workspace GET/PUT 分别要求 `config:read/config:write` 和 `workspace.read/workspace.modify`，只解析/写入 Workspace scope/default。
- Project GET/PUT 延续现有 Project config 的 token scope、role permission、Workspace 隔离和 closed Project 写门控，并保持 project > workspace > default 的 effective 解析。
- Automation rule/delivery 权限不隐式授予 Provider config 权限；有 Automation 权限但无 config 权限时，页面只显示“无权查看/编辑 Provider 配置”。
- GET 只返回 9.3 的安全 DTO；PUT 的响应仍是安全 DTO。API key 不得进入响应、OpenAPI example、日志、Audit payload、错误或前端 query cache。
- 不扩大 `/api/v1/config/{key}` 的 business-key allowlist，也不让 Workspace 页面退回通用 config endpoint。
- 现有 Project Automation Provider 卡片迁移到同一个 facade，停止通过 Project config list 把 `agent.provider.api_key` 原始值取进浏览器；其它通用 Project config API 的产品边界不在本 milestone 重写。

### 14.3 Preview 与 test 样本

- Workspace schedule preview/test 不选择 Project。
- Workspace event preview/test 必须提交 `sample_project_ref`，只接受当前 Workspace 的现有 Project。
- `project.created` 样本以 Project 当前数据构造“模拟创建事件”，明确标记 `event.simulated=true`，不会写 event、不触发其它规则，也不会改变 Project。
- preview 只返回脱敏请求；test 会真实调用 Agent，因此关闭 Project 不可作为样本。

### 14.4 CLI、Remote 与 MCP 边界

首版不新增 Workspace Automation CLI/Remote/MCP CRUD，管理面只做 HTTP/OpenAPI/Web，与现有 Project Automation 产品入口保持一致。后续如有 headless 治理需求，再基于统一 App service 增加入口。

Agent 执行业务流程复用现有 MCP tools，至少包括：

```text
project_get
project_config_list
project_config_set
project_config_unset
audit_list
```

不得增加 `yaoguang_create_knowledge` 之类属于外部平台的璇础 MCP tool，也不得增加点号分隔 tool name。

## 15. Web Console 信息架构

### 15.1 顶层导航

Workspace 自动化是跨 Project 的治理能力，不能藏在任意一个 Project 里。桌面侧栏和移动导航抽屉都在「管理」分组增加顶层入口：

```text
个人
  概览
  我的任务
  项目

管理
  成员
  Token
  自动化        <- 新增，Lucide Workflow
  Hook
  通知
  单点登录

系统
  工作空间
  审计
  设置
```

- 路由：`/automations`
- `PageKey`：`automations`
- 只有同时具备 Workspace Automation 读取权限时显示。
- `/automations` 及其详情/查询态高亮「自动化」。
- `/workspaces/:workspaceSlug/projects/:projectSlug/automations` 继续高亮「项目」，绝不能被顶层自动化入口抢走。
- Project 详情的「自动化」tab 保留，管理当前 Project 自己的规则。

### 15.2 页面结构

顶层页只有两个 tab：

- **规则**：Workspace scope 规则治理。
- **运行记录**：跨规则、跨 Project 查看 Delivery。

页面顶部用一句直接说明区分两个作用域：

```text
工作空间自动化监听整个工作空间。只处理单个项目的规则，请进入对应项目的「自动化」。
```

不再增加“集成”“Setup”“Hook”子 tab，避免让同一执行能力分散到多个入口。

## 16. ASCII 界面原型

所有实现遵循 `DESIGN.md`：深色侧栏 + 浅色画布、一屏至多两处主绿、36px 控件、`rounded-lg`、Lucide、状态只用小圆点/chip、所有数字/ID/时间使用 JetBrains Mono。桌面表格必须用 shadcn `<Table>`，列使用 `min-w-0 + truncate`，不得横向滚动。

### 16.1 桌面端：规则列表

```text
┌──────────────────────┬────────────────────────────────────────────────────────────────────────────┐
│  璇础                │ 自动化                                                                     │
│                      ├────────────────────────────────────────────────────────────────────────────┤
│  个人                │ 工作空间自动化                                      [ + 新建自动化 ]       │
│   概览               │ 监听整个工作空间。单项目规则请进入对应项目的「自动化」。                    │
│   我的任务           │                                                                            │
│   项目               │ [ 规则  3 ]   [ 运行记录 ]                              [刷新]             │
│                      │                                                                            │
│  管理                │ ┌────────────────────────────────────────────────────────────────────────┐ │
│   成员               │ │ 状态  名称               触发器             指令摘要       最近运行  ⋯ │ │
│   Token              │ ├────────────────────────────────────────────────────────────────────────┤ │
│ ▌ 自动化             │ │  ●   新项目知识库初始化   project.created    检查飞书目录…   2 分钟前  ⋯ │ │
│   Hook               │ │  ○   每周项目治理巡检     cron 周一 09:00    汇总规划中…     从未运行  ⋯ │ │
│   通知               │ │  ●   新项目默认负责人     project.created    按配置设置…     昨天      ⋯ │ │
│   单点登录           │ └────────────────────────────────────────────────────────────────────────┘ │
│                      │                                                                            │
│  系统                │ ● 启用   ○ 停用                         共 3 条                            │
│   工作空间           │                                                                            │
│   审计               │                                                                            │
│   设置               │                                                                            │
└──────────────────────┴────────────────────────────────────────────────────────────────────────────┘
```

规则表列：

- 状态：7px 状态点 + 屏幕阅读器文本。
- 名称：主信息，点击进入编辑。
- 触发器：`project.created` 或中文 cron 摘要，原表达式放 tooltip。
- 指令摘要：单行截断，不在列表暴露完整 system prompt。
- 最近运行：相对时间，tooltip 显示绝对时间。
- 操作：编辑、立即测试、启停、删除；删除为二次确认。

### 16.2 桌面端：新建/编辑 Dialog

```text
┌──────────────────────────────── 新建工作空间自动化 ────────────────────────────────┐
│                                                                                   │
│ 名称 *                                                                            │
│ [ 新项目知识库初始化________________________________________________________ ]     │
│                                                                                   │
│ 触发方式 *                                                                        │
│ (●) 工作空间事件      ( ) 定时                                                    │
│                                                                                   │
│ 事件                                                                              │
│ [ project.created · 项目创建完成                                             v ]  │
│                                                                                   │
│ Agent Provider                                                                    │
│ Workspace config · https://agent.example.com · model yaoguang-operator  [查看配置] │
│ Project 样本不会覆盖这里的 Provider。                                              │
│                                                                                   │
│ 执行指令 *                                                                        │
│ ┌───────────────────────────────────────────────────────────────────────────────┐ │
│ │如果 feishu_drive_folder_token 为空则跳过。                                    │ │
│ │先检查 knowledge_id；已有值不得覆盖……                                          │ │
│ │                                                                               │ │
│ └───────────────────────────────────────────────────────────────────────────────┘ │
│ 提示：写清目标、前置条件、幂等条件和 project_config_set 回写规则。                │
│                                                                                   │
│ 上下文                                                                            │
│ [✓] Workspace  [✓] Project  [✓] 初始 Project config  [✓] Event                   │
│ Secret config 永不进入上下文。                                                     │
│                                                                                   │
│ 测试样本                                                                          │
│ [ 选择一个现有 Project 用于预览/立即测试                                    v ]  │
│                                                                                   │
│ [高级设置 v]                                                                      │
│                                                                                   │
│                              [取消]  [预览投递 JSON]  [保存并启用]                 │
└───────────────────────────────────────────────────────────────────────────────────┘
```

设计要求：

- 复用现有 Automation form/dialog 的字段组件和 preview dialog，不复制表单实现。
- `project.created` 只有一个事件选项时仍显示选择器，明确这是可扩展白名单。
- Preview/Test 时 sample Project 必填；正常保存规则不需要 sample。
- Provider 区只读展示 Workspace 配置解析结果，错误时用小型 warning chip 和直达 Workspace 配置的链接。
- Workspace Provider 值通过 14.2 的安全 facade 读取和修改；控制台复用 Provider config 组件并以 Dialog/折叠区承载，不新增第二套 Provider 存储。没有 `config:read/write` 时只显示无权查看/编辑，不影响已具备 Automation 权限的用户治理规则。
- 高级设置收纳 model override、temperature、system prompt、重试和上下文限额等低频字段。

### 16.3 桌面端：Provider 配置 Dialog

```text
┌──────────────────────────── Agent Provider 配置 ────────────────────────────┐
│ 来源：Workspace config                                                      │
│                                                                             │
│ Base URL *                                                                  │
│ [ https://agent.example.com____________________________________________ ]   │
│                                                                             │
│ API Key                                                                     │
│ [ 已设置（留空保留原值）_______________________________________________ ]   │
│ [ ] 清除已有 API Key                                                        │
│                                                                             │
│ Model *                                                                     │
│ [ yaoguang-operator____________________________________________________ ]   │
│                                                                             │
│ Allowed Hosts                                                               │
│ [ agent.example.com____________________________________________________ ]   │
│ 仅填写 hostname；留空表示沿用当前可选白名单语义。                           │
│                                                                             │
│ API Key 永不回显。保存后输入框立即清空。                 [取消] [保存配置] │
└─────────────────────────────────────────────────────────────────────────────┘
```

Dialog 只能消费安全 Provider DTO。`api_key_set=true` 只控制占位文案，不得把掩码当值写入输入框；保存成功、失败、关闭和 query invalidation 时都清除 API Key 本地 state。Project 页面复用同一组件，但标题显示 Project effective config，并延续 closed Project 门控。

### 16.4 桌面端：运行记录与详情抽屉

```text
┌────────────────────────────────────────────────────────────────────────────────────┐
│ 工作空间自动化                                                                     │
│ [ 规则 ]   [ 运行记录  18 ]                                                        │
│                                                                                    │
│ [状态：全部 v] [规则：全部 v] [Project：全部 v] [触发：全部 v] [搜索 ID____]       │
│                                                                                    │
│ ┌────────────────────────────────────────────────────────────────────────────────┐ │
│ │ 状态        规则                 Project    触发              HTTP     时间    ⋯ │ │
│ ├────────────────────────────────────────────────────────────────────────────────┤ │
│ │ ● 成功      新项目知识库初始化   atlas      project.created   200      10:32   › │ │
│ │ ● 重试中    新项目默认负责人     atlas      project.created   502      10:32   › │ │
│ │ ● 失败      每周项目治理巡检     —          cron              401      周一    › │ │
│ └────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                    │
│                                              ┌──────────── 运行详情 ─────────────┐ │
│                                              │ Agent 调用成功                    │ │
│                                              │ 不是“知识库创建成功”的证明        │ │
│                                              │                                  │ │
│                                              │ rule      新项目知识库初始化       │ │
│                                              │ project   atlas                    │ │
│                                              │ delivery  dly_…                    │ │
│                                              │ event     evt_…                    │ │
│                                              │ provider  req_yg_…                 │ │
│                                              │ HTTP      200                      │ │
│                                              │                                  │ │
│                                              │ [查看冻结请求] [查看 Project Audit]│ │
│                                              │ [重新投递]                         │ │
│                                              └──────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────────────────────┘
```

运行列表默认按 `created_at desc`。Project 名称可点击进入 Project；schedule 没有 Project 时显示 `—`。详情展示的是 Delivery 当时冻结的脱敏请求和响应摘要，不按当前配置重新渲染。

### 16.5 移动端

```text
┌──────────────────────────────┐
│ ☰  自动化              ↻     │
├──────────────────────────────┤
│ 工作空间自动化               │
│ 监听整个工作空间             │
│                              │
│ [ 规则 3 ] [ 运行记录 18 ]   │
│                              │
│ ┌──────────────────────────┐ │
│ │ ● 新项目知识库初始化   ⋯ │ │
│ │ project.created          │ │
│ │ 检查飞书目录并创建…      │ │
│ │ 最近运行  2 分钟前       │ │
│ └──────────────────────────┘ │
│ ┌──────────────────────────┐ │
│ │ ○ 每周项目治理巡检     ⋯ │ │
│ │ 周一 09:00 · Asia/Shanghai│ │
│ │ 从未运行                 │ │
│ └──────────────────────────┘ │
│                              │
│ [ + 新建自动化 ]             │
└──────────────────────────────┘
```

移动端不用压缩桌面 Table，而使用纵向 card list；新建按钮固定在内容末尾，不使用遮挡列表的悬浮大按钮。编辑使用全高 Sheet，字段顺序与桌面 Dialog 一致。

## 17. 页面状态与文案

| 状态 | 展示 |
|---|---|
| 没有规则 | `还没有工作空间自动化。创建规则，在项目创建或定时时让 Agent 处理工作空间任务。` |
| 无读取权限 | 导航不显示；直接访问返回 403 页面 |
| Provider 未配置 | 顶部 warning：`工作空间 Agent Provider 配置不完整，规则不会成功投递。` |
| 规则配置错误 | 保存前 validation；运行时变化则创建 dead-lettered Delivery |
| 没有运行记录 | `规则触发后，Agent 调用会显示在这里。` |
| 调用 2xx | `Agent 调用成功` |
| 业务结果 | 引导 `查看 Project config` / `查看 Project Audit`，不推断成功 |

## 18. 测试策略

### 18.1 App/storage

- Automation rule scope normalization、唯一性和跨 Workspace 隔离。
- 旧 Project rule/Delivery 迁移前后 ID、数量、状态和 dedupe key 不变。
- SQLite/PostgreSQL 都覆盖 `scope_type/scope_id` 查询和 nullable delivery project。
- 普通 AddProject 的 `Project + Audit + Delivery` 同事务；任一步失败无半成品。
- Template Instantiate 完成 config/Task/Series/Automation 后再生成 `project.created` Delivery。
- event source/metadata/count 正确，不包含 Snapshot JSON 或 secret。
- 同一 `rule_id + event_id` 重复路由只得到一个 Delivery。
- 规则在事件后新增不补跑；Delivery 创建后改规则不改变冻结请求。
- Workspace Provider 只读 Workspace config，Project override 不生效。
- Workspace/Project Provider facade 的 GET/PUT 都不返回 API key；空 key 保留、显式 clear、权限和 closed Project 语义有测试。
- Workspace 页面不调用通用 `/api/v1/config/{key}`，Project Provider 卡片不再从 Project config list 读取密钥原值。
- Provider 规则错误生成 dead-lettered Delivery，不阻断 Project 创建。
- internal serialization/DB 错误回滚 Project 创建。
- Workspace schedule context 不隐式加载 Project。

### 18.2 HTTP/OpenAPI/权限

- Workspace Automation 完整 CRUD、preview、test、enable/disable/delete、delivery list/info/replay。
- Workspace 端点不能读取 Project scope rule，Project 端点不能读取 Workspace scope rule。
- `workspace:read + hook:read` 与 `workspace:write + hook:write` 双 scope 矩阵。
- owner/admin/member/viewer 角色矩阵。
- sample Project 跨 Workspace、关闭、缺失的错误。
- secret、Authorization 和完整 prompt 不进入 Audit/OpenAPI example/error。
- 所有用户对象保持 `task.JSONUserInfo` 形状。

### 18.3 Dispatcher

- 复用现有 claim、stale recovery、429/5xx retry、dead letter、replay。
- Workspace event Delivery 的 Project 可选语义。
- network timeout 后请求 body、delivery/event id 保持不变。
- 规则修改/删除后 dispatcher 不读取当前 Rule，仍使用 Delivery 冻结的 auth key、安全策略 key 和 max attempts。
- replay 记录 `replay_of_delivery_id` 且不重渲染当前规则。

### 18.4 Web Console

- 管理分组出现「自动化」，图标、顺序、权限隐藏正确。
- `/automations` 高亮自动化；Project Automation 深层路由仍高亮项目。
- 规则/运行记录 tab、过滤、空态、错误态。
- 新建/编辑复用 Automation form，event preview/test 要求 sample Project。
- Provider 来源明确显示 Workspace config。
- Delivery 2xx 文案是“Agent 调用成功”，不显示“业务执行成功”。
- 桌面 Table 无横向滚动；移动端 card/Sheet 可用。
- 遵循 `DESIGN.md` token，不出现组件内裸 hex 或 Tailwind 原色。

### 18.5 完整验证

实现完成前至少运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
git diff --check
```

还需增加真实 server E2E：从模板创建 Project，等待 Workspace Delivery 被 dispatcher 领取，使用假的 OpenAI-compatible endpoint 断言请求中有正确 event/config 且没有 secret，再通过 MCP `project_config_set` 模拟 Agent 回写并断言 Audit。

## 19. 错误码

沿用现有 Automation 错误码并泛化 resource 文案，新增或明确：

| code | 含义 |
|---|---|
| `automation_rule_not_found` | 当前 scope 下规则不存在 |
| `automation_scope_invalid` | scope type/id 不合法或不匹配当前 Workspace |
| `automation_event_unsupported` | Workspace event 不在白名单 |
| `automation_sample_project_required` | event preview/test 未选择样本 Project |
| `automation_sample_project_invalid` | 样本不存在、跨 Workspace 或不允许测试 |
| `automation_provider_config_missing` | Workspace Provider 必需 config 缺失 |
| `automation_provider_target_denied` | allowed host / SSRF 拒绝 |
| `automation_context_too_large` | 冻结 request body 超限 |

错误消息不得包含 config secret、Authorization、完整请求体或响应体。

## 20. 验收标准

1. owner/admin 能从侧栏「管理 → 自动化」进入 `/automations`，创建 `project.created` 规则。
2. 创建空 Project 和从 Template 创建 Project 都只触发一次，且 Agent 看到的是初始化事务完成后的 Project/config 上下文。
3. 删除/错误的 Workspace Provider 配置不会悄悄丢事件；运行记录出现可诊断的 dead-lettered Delivery。
4. Project scope 与 Workspace scope 规则共用同一 App/storage/dispatcher，不存在第二套 Setup Action/Run 实现。
5. Agent 可用自然语言指令调用已有 MCP 回写 config，璇础不硬编码 Yaoguang 或知识库 action。
6. Delivery 2xx 与业务结果在 UI 和文档中明确分开；Project config + Audit 是回写事实。
7. retry/replay 不会因为规则或 Project config 后续变化而重渲染历史请求。
8. secret config 在请求上下文、preview、Delivery、Audit、日志和错误中均不可见。
9. 现有 Project Automation 路由、项目 tab、模板行为和历史 Delivery 迁移后保持可用。
10. Desktop/mobile 原型对应的导航、规则、运行记录和编辑体验全部有自动化测试及真实浏览器 smoke。

## 21. 后续扩展顺序

本规格完成后，按真实需求逐步扩展，不提前建设工作流引擎：

1. 把更多稳定领域事件接入同一个 transactional Automation event router，例如 `workspace.member_added`、`project.transitioned`。
2. 为 Workspace event matcher 增加少量结构化、可索引条件；只有自然语言 no-op 带来明显成本时才引入。
3. 支持 Responses API，但继续复用同一 Rule/Delivery。
4. 为 Agent Provider 的异步 run 增加标准化状态回调；只有 Provider 契约稳定后才做，不解析自然语言响应。
5. 如出现 headless 管理需求，再为统一 App service 增加 CLI/MCP 管理入口，并单独设计 automation scope，避免让业务 Agent 默认可修改触发器。
6. 如需要人工批准、长事务补偿或可视化多节点状态，再评估独立 Workflow 产品；不要把这些能力塞进当前 instruction 字段。
