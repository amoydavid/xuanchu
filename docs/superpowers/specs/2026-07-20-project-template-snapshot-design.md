# 璇础项目模板与版本化快照设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-20
**状态：** 待用户审阅
**目标版本：** v0.6.0
**背景需求：** workspace 内的真实项目可以选择部分 config、普通 task、task series 和 project automation，另存为可复用模板；用户随后从模板快速创建一个全新项目。

## 1. 背景与现状

璇础已经具备项目、普通任务、独立 TaskSeries、project-scoped config 和项目自动化规则，但这些资源只能分别创建。团队反复启动结构相似的项目时，需要重新录入任务、循环任务、配置和自动化规则。

当前代码中有几块可以复用，但都不能直接当作项目模板：

- `internal/app/task_bundle.go` 的 `xuanchu.task-bundle/v1` 面向跨环境迁移，会保留任务 UUID、运行状态、Series 历史和 occurrence，不适合生成一个干净的新项目。
- `internal/app/project_config.go` 已经区分 project 显式值、workspace 继承值和 schema default，模板必须沿用这个边界。
- `internal/app/task_series.go` 和 `internal/taskseries` 已经把 Series 建模为独立资源，模板不能退回到隐藏 recurring task。
- `internal/app/project_automation.go` 的规则引用 config key，delivery 则是运行历史；模板只能保存规则定义，不能保存 delivery。
- task description 是 Markdown 字符串，可能包含 `ref://user/...`、`ref://task/...` 和 `ref://attachment/...` 语义引用。模板必须显式处理引用，不能盲目复制源 UUID。

本功能是一个新的跨层能力，必须由 App 层统一编排，CLI、HTTP、Remote、MCP 和 Web Console 复用同一套用例。

## 2. 第一性原理结论

项目模板不是“源项目的实时别名”，也不是“数据库行复制器”。它是一次经过用户选择、验证和日期归一化后的项目初始化定义。

本规格锁定以下结论：

| 主题 | 决策 |
|---|---|
| 使用范围 | 只允许在同一个 workspace 内复用 |
| 核心模型 | Template 是长期入口；Snapshot 是不可变内容版本 |
| 内容存储 | Snapshot 的全部可演进内容保存在单个逻辑 JSON 字段中 |
| Go 契约 | 使用带 schema 版本的 Go struct 严格编解码，不使用无约束 `map[string]any` |
| 数据库兼容 | SQLite/PostgreSQL 都用 TEXT 保存 JSON 字节，不依赖数据库方言特有 JSON 类型 |
| 内容更新 | 更新模板时追加新 Snapshot，不覆盖旧 Snapshot |
| 普通任务 | 保存为初始化 blueprint；创建新项目时生成新的开放任务 |
| Series | 保存当前有效定义；创建时生成新的 active Series 和一条初始 RuleVersion |
| Config | 只保存用户选择的 project 显式值；不冻结 workspace 继承值/default |
| Secret | Snapshot 永不保存 secret 值；只保存占位要求 |
| Automation | 只保存规则定义；新项目中的规则一律 disabled |
| 引用 | Snapshot 使用本地 ref；创建时统一分配新 ID 并重建引用 |
| 日期 | 按模板基准日保存相对本地日期；按新项目启动日恢复 |
| 创建一致性 | 预检后整体事务创建；任一失败全部回滚 |
| 源项目关系 | Snapshot 创建后与源项目断开；源项目变化不影响 Snapshot |

## 3. 目标

1. 允许具备权限的用户把一个 workspace 内的项目另存为模板。
2. 保存时按 config、task、series、automation 四类选择内容。
3. 普通 task 和 series 必须支持逐项勾选，不要求整类全选。
4. config 和 automation 同样支持逐项选择，避免复制项目特有信息。
5. 模板内容可演进，但 JSON 结构变化不要求修改数据库表结构。
6. 从模板创建的新项目拥有全新 ID、序号、运行状态和审计记录。
7. 保留选中任务之间的 parent、depends 和 task semantic reference。
8. 防止 secret、attachment、历史 occurrence 和 automation delivery 泄漏到模板。
9. 创建前提供可读预检；创建过程具备完整事务原子性。
10. CLI、HTTP、Remote、MCP 和 Web Console 使用同一 App 能力和相同 JSON 语义。

## 4. 非目标

- 不支持跨 workspace 共享、复制或导出项目模板。
- 不支持把模板合并到已有项目。
- 不支持模板继承、模板组合或多模板叠加。
- 不支持模板与源项目实时同步。
- 不支持从源项目后续变更自动刷新模板。
- 不复制 task/series attachment 二进制。
- 不复制 task annotation、audit、completion history、project timeline。
- 不复制 Series occurrence、tombstone、skip、backlog 或历史 RuleVersion。
- 不复制 automation delivery、retry、request snapshot 或 response。
- 不自动启用由模板创建的 automation。
- 不支持在模板库中实现一套完整 Task/Series 编辑器。
- 不改变普通 task 与 TaskSeries 的既有领域边界。
- 不使用 Taskwarrior JSON 或 `xuanchu.task-bundle/v1` 作为模板公开格式。

## 5. 术语

| 术语 | 含义 |
|---|---|
| Project Template / 项目模板 | workspace 内可复用的模板入口，保存名称、稳定 key、状态和当前 Snapshot |
| Snapshot / 快照 | 某次从项目选择内容后生成的不可变版本 |
| Capture / 保存快照 | 从源项目读取选中资源、做校验和归一化并生成 Snapshot |
| Instantiate / 从模板创建 | 使用一个确定 Snapshot 创建全新项目及其子资源 |
| Blueprint | 不是运行记录、只描述如何创建新资源的定义 |
| Anchor Date / 模板基准日 | Capture 时用于把绝对时间转换为相对本地日期的日期 |
| Start Date / 项目启动日 | Instantiate 时用于恢复相对日期的日期 |
| Local Ref / 本地引用 | Snapshot 内部使用的 `task-1`、`series-1` 等稳定局部标识 |
| Capture Preview | 保存 Snapshot 前的选择、依赖和日期预检 |
| Instantiate Preview | 创建项目之前的权限、schema、成员、日期和 secret 预检 |

## 6. 方案比较

### 6.1 源项目实时引用

Template 只保存 `source_project_id` 和筛选条件，Instantiate 时重新读取源项目。

优点是初始表结构少；缺点是同一个模板每次创建结果不同，源项目归档、删除或改名后行为漂移，无法审计“当时用了什么”。不采用。

### 6.2 直接扩展 TaskBundle

在 `xuanchu.task-bundle/v1` 上增加 project/config/automation。

它会把迁移语义和初始化语义绑定在一起：bundle 需要保留身份和历史，template 需要生成新身份并清除历史。长期会出现大量模式开关。只复用内部映射和事务思路，不复用公开 schema。

### 6.3 Template 元数据 + 版本化 JSON Snapshot

关系型列只保存 workspace、template、version、hash、来源和审计字段；所有组件内容放在版本化 JSON 中，由 Go struct 约束。

优点：

- 新增可选字段或新组件时不修改模板表。
- Snapshot 可复现、可校验、可计算 hash。
- Template 更新不会破坏已经创建的项目和旧版本。
- JSON 仍有强类型边界，不会退化为散乱动态对象。

本规格采用此方案。

## 7. 总体架构

```text
Web / CLI / HTTP / MCP / Remote
                |
                v
        internal/app
  capture preview / capture
  instantiate preview / instantiate
                |
       +--------+---------+
       |                  |
       v                  v
internal/projecttemplate  internal/storage
typed snapshot + codec    template repositories
validation + date shift   raw SnapshotJSON
       |                  |
       +--------+---------+
                |
          SQLite / PostgreSQL
```

分层约束：

- `internal/projecttemplate` 只负责 Snapshot 领域模型、严格 codec、版本升级、hash 和纯函数校验，不依赖 GORM、Cobra、HTTP 或 App Service。
- `internal/storage` 只保存模板元数据和原始 Snapshot JSON，不解释 task/series/config/automation 业务语义。
- `internal/app` 负责权限、源资源读取、用户解析、Capture、Instantiate、事务、审计和事件。
- 协议层只做 DTO 转换，不自行克隆项目资源。
- Web Console 只消费 preview/result，不在浏览器内构造权威 Snapshot。

## 8. 数据模型

### 8.1 `project_templates`

```text
id                    UUID PK
workspace_id          UUID NOT NULL
key                   TEXT NOT NULL
name                  TEXT NOT NULL
description           TEXT NOT NULL DEFAULT ''
status                TEXT NOT NULL        // active|archived
current_snapshot_id   UUID NULL
created_by_actor_type TEXT NOT NULL
created_by_user_id    UUID NULL
created_by_token_id   UUID NULL
created_by_token_name TEXT NULL
created_by_token_prefix TEXT NULL
created_at            BIGINT NOT NULL
modified_at           BIGINT NOT NULL
archived_at           BIGINT NULL
```

约束：

- `UNIQUE(workspace_id, key)`。
- `key` 是稳定查找键，规范为 3–32 位小写 ASCII 字母、数字和连字符，必须以字母开头。
- `name` 是可修改展示名，允许中文，不承担稳定引用职责。
- Modify 只允许修改 name/description；key 创建后不可修改。
- `status` 只允许 `active|archived`。
- archived template 不允许追加 Snapshot 或 Instantiate，但可以读取历史和重新激活。
- 不硬删除 Template；Snapshot 和由其创建的项目需要保留来源语义。

### 8.2 `project_template_snapshots`

```text
id                    UUID PK
workspace_id          UUID NOT NULL
template_id           UUID NOT NULL
version               BIGINT NOT NULL
source_project_id     UUID NOT NULL
snapshot_json         TEXT NOT NULL
snapshot_hash         TEXT NOT NULL
created_by_actor_type TEXT NOT NULL
created_by_user_id    UUID NULL
created_by_token_id   UUID NULL
created_by_token_name TEXT NULL
created_by_token_prefix TEXT NULL
created_at            BIGINT NOT NULL
```

约束：

- `UNIQUE(template_id, version)`。
- `UNIQUE(template_id, snapshot_hash)`，同一模板不重复追加完全相同的 Snapshot。
- `workspace_id` 必须与 Template 和 source Project 一致。
- Snapshot 一经写入不可更新；更新模板只能追加新 version。
- `snapshot_json` 在 SQLite/PostgreSQL 均保存为 TEXT。JSON 是否合法、属于哪个 schema、字段是否完整全部由 Go codec 判断。
- `snapshot_hash` 为 canonical JSON 字节的 SHA-256 小写十六进制值。
- Template 与 Snapshot 使用 `RESTRICT` 关系；产品不提供硬删除。

### 8.3 为什么不为组件建模板子表

本功能不新增 `project_template_tasks`、`project_template_series`、`project_template_configs` 或 `project_template_automations`。

原因：

- 这些对象只在 Capture 和 Instantiate 时整体读写，不存在高频独立查询。
- 模板内容不是运行实体，不需要按状态或日期做数据库查询。
- 每新增一个 task/series 字段都修改模板表会造成两套 schema 长期同步。
- 数量摘要可以在读取 Template 时解码 Snapshot 后计算；不为缓存摘要增加稳定性负担。

关系型列只承载必须由数据库保证的隔离、版本、唯一性、状态和审计信息。

## 9. JSON 与 Go struct 契约

### 9.1 顶层 schema

`internal/projecttemplate/model.go` 定义：

```go
const SnapshotSchemaV1 = "xuanchu.project-template-snapshot/v1"

type SnapshotV1 struct {
    Schema      string                 `json:"schema"`
    AnchorDate  string                 `json:"anchor_date"`
    Project     ProjectBlueprintV1     `json:"project"`
    Configs     []ConfigBlueprintV1    `json:"configs"`
    Tasks       []TaskBlueprintV1      `json:"tasks"`
    Series      []SeriesBlueprintV1    `json:"series"`
    Automations []AutomationBlueprintV1 `json:"automations"`
}

type ProjectBlueprintV1 struct {
    Description string `json:"description"`
}
```

Snapshot 不保存源 Project 的 slug、name、status、task count 或时间线。新项目 name/slug 必须在 Instantiate 时提供；description 可以用 Snapshot 默认值预填并允许覆盖。

### 9.2 相对时间

```go
type RelativeLocalTimeV1 struct {
    DayOffset int    `json:"day_offset"`
    LocalTime string `json:"local_time"` // HH:MM:SS
}

type TaskDatesV1 struct {
    Due       *RelativeLocalTimeV1 `json:"due,omitempty"`
    Wait      *RelativeLocalTimeV1 `json:"wait,omitempty"`
    Scheduled *RelativeLocalTimeV1 `json:"scheduled,omitempty"`
    Until     *RelativeLocalTimeV1 `json:"until,omitempty"`
}
```

不能保存纯秒数 offset。跨 DST 时，秒数偏移会让本地 09:00 漂移；`day_offset + local_time` 必须按 workspace timezone 用日历运算恢复。

### 9.3 普通 Task blueprint

```go
type UDABlueprintV1 struct {
    Raw  string `json:"raw"`
    Type string `json:"type,omitempty"`
}

type TaskLinkBlueprintV1 struct {
    Type  string `json:"type"`
    URL   string `json:"url"`
    Title string `json:"title,omitempty"`
}

type TaskBlueprintV1 struct {
    Ref         string                    `json:"ref"`
    Title       string                    `json:"title"`
    Description *string                   `json:"description,omitempty"`
    Priority    *string                   `json:"priority,omitempty"`
    Tags        []string                  `json:"tags,omitempty"`
    AssigneeIDs []string                  `json:"assignee_ids,omitempty"`
    UDAs        map[string]UDABlueprintV1 `json:"udas,omitempty"`
    Dates       TaskDatesV1               `json:"dates"`
    ParentRef   *string                   `json:"parent_ref,omitempty"`
    DependsRefs []string                  `json:"depends_refs,omitempty"`
    Links       []TaskLinkBlueprintV1     `json:"links,omitempty"`
}
```

不保存 UUID、project ID、project seq、status、entry、modified、end、start、annotation、created_by、occurrence 字段或 attachment。

### 9.4 Config blueprint

```go
type ConfigBlueprintV1 struct {
    Key   string  `json:"key"`
    Mode  string  `json:"mode"` // literal|secret_input
    Value *string `json:"value,omitempty"`
}
```

约束：

- `mode=literal` 时 `value` 必填，且 Capture 时 schema 必须不是 secret。
- `mode=secret_input` 时 `value` 必须为空。
- Snapshot 不保存 workspace 继承值或 default。
- 即使调用方持有 secret read 能力，也不能把 secret 放入 Snapshot。

### 9.5 Series blueprint

```go
type SeriesBlueprintV1 struct {
    Ref             string                    `json:"ref"`
    Title           string                    `json:"title"`
    Description     *string                   `json:"description,omitempty"`
    Priority        *string                   `json:"priority,omitempty"`
    Tags            []string                  `json:"tags,omitempty"`
    AssigneeIDs     []string                  `json:"assignee_ids,omitempty"`
    UDAs            map[string]UDABlueprintV1 `json:"udas,omitempty"`
    RecurrenceRule  string                    `json:"recurrence_rule"`
    FirstDue        RelativeLocalTimeV1       `json:"first_due"`
    Until           *RelativeLocalTimeV1      `json:"until,omitempty"`
}
```

不保存 Series ID、project seq、status、effective_end_at、stop_reason、RuleVersion ID/history 或 occurrence。

### 9.6 Automation blueprint

Automation 不能使用 `json.RawMessage` 逃避类型约束。`internal/projecttemplate` 定义与公开规则字段等价的 snapshot-specific struct：

```go
type AutomationBlueprintV1 struct {
    Ref                 string                    `json:"ref"`
    Name                string                    `json:"name"`
    Description         string                    `json:"description,omitempty"`
    TriggerType         string                    `json:"trigger_type"`
    TriggerConfig       AutomationTriggerV1       `json:"trigger_config"`
    Condition           AutomationConditionV1     `json:"condition"`
    Action              AutomationActionV1        `json:"action"`
    Context             AutomationContextV1       `json:"context"`
    InstructionTemplate string                    `json:"instruction_template"`
    SystemPrompt        string                    `json:"system_prompt,omitempty"`
}
```

Automation blueprint 不保存 source rule ID、enabled、created_by、delivery 或任何已渲染请求。新规则固定以 `enabled=false` 创建。

### 9.7 严格 codec 与版本升级

解码流程：

1. 先只解码 `{"schema":"..."}` header。
2. 按精确 schema 分派到对应 Go struct。
3. 对已知 schema 使用 `json.Decoder.DisallowUnknownFields()`。
4. 检查 trailing token，拒绝拼接多个 JSON document。
5. 做结构验证、ref 完整性、数组上限和字段长度验证。
6. 转为当前内部模型。

未知 schema 返回 `project_template_snapshot_schema_unsupported`，不能猜测性宽松读取。

未来出现 v2 时：

- 新二进制默认写 v2。
- 保留 `decodeV1 -> upgradeV1ToCurrent`。
- 旧 Snapshot 不原地重写。
- 新字段放进 JSON struct，不修改 Snapshot 表。
- 如果只是新增当前二进制可选字段，也必须评估旧二进制 downgrade 行为；默认通过新 schema 版本显式隔离。

### 9.8 Canonical JSON 与 hash

Capture 在写库前必须：

- trim/normalize 字段；
- tags、assignee IDs、depends refs、config keys 等集合稳定排序并去重；
- tasks、series、automation 按用户确认的顺序保存；该顺序同时决定新项目中的 seq 分配；
- 使用标准 codec 序列化最终 struct；
- 对最终字节计算 SHA-256。

`snapshot_hash` 同时用于去重、预检防漂移和项目来源审计。

原始 `snapshot_json` 不通过 HTTP/MCP/Remote 直接返回。输出层返回 typed/redacted view，secret placeholder 只显示 key 和解析状态。

## 10. Capture 选择与归一化

### 10.1 允许的源项目

- 源项目必须属于当前 workspace。
- planning、active、archived、cancelled 都可以 Capture；Capture 是只读操作。
- project-scoped token 不允许创建或更新 workspace 模板。
- source Project 后续变更、关闭或重新激活都不改变已有 Snapshot。

### 10.2 选择输入

```go
type CaptureSelection struct {
    ConfigKeys       []string `json:"config_keys"`
    TaskRefs         []string `json:"task_refs"`
    SeriesRefs       []string `json:"series_refs"`
    AutomationRuleIDs []string `json:"automation_rule_ids"`
}
```

所有选择必须显式提交。空数组表示不保存该类内容；不能用“字段缺失”推断全选。
HTTP/OpenAPI/MCP schema 必须把四个数组都标为 required；App 输入解析还要记录字段是否出现，不能让 Go 的 nil/empty 零值掩盖调用方漏传。

Capture 对依赖和日期的人工处理使用结构化 resolution，不接收自由文本指令：

```go
type CaptureResolution struct {
    DropParentTaskRefs       []string                 `json:"drop_parent_task_refs"`
    DropDepends             []TaskRelationResolution `json:"drop_depends"`
    DropContentTaskRefs      []ContentRefResolution   `json:"drop_content_task_refs"`
    TaskDateOverrides       []TaskDateOverride        `json:"task_date_overrides"`
    SeriesScheduleOverrides []SeriesScheduleOverride  `json:"series_schedule_overrides"`
}
```

每个 resolution 都以 source task/series ref 和具体字段为键；服务端必须验证它对应当前 Preview 中的 issue，不能用 resolution 删除一个原本有效、未提示的关系。

Capture Preview 返回：

- 每类可选条目和当前选择；
- normalized selection；
- source hash；
- 日期转换预览；
- blocking issues；
- warnings；
- component counts。

Capture 请求必须携带 Preview 返回的 `expected_source_hash`。若选中资源在 Preview 后发生变化，返回 `project_template_source_changed`，要求重新预览。

### 10.3 普通 Task

可选范围：

- 只列出 `series_id IS NULL` 的普通 task。
- deleted task 不可选择。
- pending、waiting、completed 均可选择。
- 默认勾选 pending/waiting；completed 由用户主动勾选。

归一化：

- 所有选中 task 在新项目中作为新任务创建。
- completed 不保留完成状态，恢复为开放任务。
- `wait` 在新启动日仍处于未来时，由现有领域规则决定 waiting 表现。
- 不复制 `start`，因为它是运行动作而不是计划字段。
- TaskLink 只保存 type、URL、title；created_by 改为 Instantiate actor。
- UDA 在 Capture 和 Instantiate 都按当前 workspace definition 校验。

### 10.4 Series

可选择 active、ended、stopped Series；UI 默认只勾选 active。

归一化：

- 新 Series 固定为 active。
- 使用源 Series 当前 `recurrence_rule`，不复制历史 RuleVersion。
- 新 Series 创建时只写一条 initial RuleVersion。
- active Series 的默认 first_due 是模板基准日当天或之后的第一个合法槽位。
- active Series 的 until 如果仍晚于该 first_due，转换为相对日期；已经过去则默认清空并显示 warning。
- ended/stopped Series 没有未来槽位时，first_due 默认使用 anchor date offset 0，until 清空，并要求用户在 Preview 中确认。
- 用户可以在 Capture Preview 中覆盖某个 Series 的 first_due/until 相对值。

### 10.5 Config

候选列表只包含源 Project 的显式 config rows：

```text
configs.workspace_id = current workspace
configs.scope        = project
configs.scope_id     = source project id
```

不把 effective workspace value 或 schema default 伪装成项目配置。

- 非 secret key 保存 literal。
- secret key 保存 `secret_input` placeholder。
- Snapshot 不保存 ConfigDefinition；Instantiate 使用当前 workspace definition 重新验证。
- 如果 key 已被删除、scope 不再允许 project 或 literal 的类型不再合法，Instantiate Preview 阻断。
- 如果一个历史 literal key 后来被改为 secret，Instantiate 不得返回或使用 Snapshot literal，必须按 secret_input 处理并要求用户确认新的值或继承来源。历史 Snapshot 仍不可修改。

### 10.6 Automation

- 候选列表来自 source Project 的 automation rule，包含 enabled/disabled；两者都可选。
- Snapshot 保存规则结构，不保存 enabled 状态。
- Instantiate 前验证 action config 引用的 config key、allowed host 配置和 provider contract。
- 规则创建时全部 disabled。
- delivery、dedupe key、attempt、response 和 frozen request 全部排除。
- instruction/system prompt 中的自由文本不做 source project slug/UUID 字符串替换。

## 11. 引用、依赖与附件

### 11.1 Local Ref

Capture 按用户顺序分配：

```text
source task UUID A  -> task-1
source task UUID B  -> task-2
source series UUID C -> series-1
automation UUID D   -> automation-1
```

Snapshot 不把 source task/series UUID 当作运行引用。

### 11.2 Parent 与 Depends

如果引用目标也被选择：

- `parent` 转为 `parent_ref`；
- `depends` 转为 `depends_refs`；
- Instantiate 时映射到新 UUID。

如果引用目标未选择，Capture Preview 返回 blocking issue：

```json
{
  "code": "project_template_dependency_missing",
  "source_ref": "ops-8",
  "target_ref": "ops-2",
  "relation": "depends"
}
```

用户只能：

1. 补选目标 task；
2. 在 Capture 请求中显式提交 drop resolution。

服务端不得自动补选，也不得静默删除关系。

如果普通 task 依赖的是 materialized occurrence，occurrence 本身不允许进入模板，因此只能显式 drop 该依赖；UI 不显示“同时选择”动作。

### 11.3 Markdown 语义引用

- `ref://task/{uuid}`：目标被选择时，在 Snapshot Markdown 中改写为模板专用的 `ref://task/task-1`；Instantiate 后再改回 `ref://task/{new-uuid}`。
- 目标未选择时按 dependency missing 阻断，用户可补选或显式移除该引用。
- `ref://user/{uuid}`：同 workspace 下保留 user ID；Instantiate Preview 重新验证 active membership 和可读性。
- `ref://attachment/{uuid}`：v1 不复制附件，出现时阻断 Capture。用户必须取消选择该 task/series，或回源项目处理 description；不能生成失效引用。
- 普通 HTTP/Markdown 链接原样保留。
- code block 中看起来像 ref URI 的文字继续遵守现有 parser，不当作引用。

`ref://task/task-1` 只存在于 Snapshot JSON。它不满足普通 task content reference 的 UUID 约束，不能直接传给 Task App 用例；`internal/projecttemplate` 使用专用 parser 校验 local ref，Instantiate 必须在调用普通 task validation 之前完成 UUID 改写。

## 12. 日期语义

### 12.1 Anchor Date

Capture 必须提交 `anchor_date=YYYY-MM-DD`，Web 默认 workspace 本地当天。它是模板计划的“第 0 天”，不是源 Project 的 created_at。

转换：

```text
source local date 2026-07-23 09:30:00
anchor date       2026-07-20
        -> day_offset=3, local_time=09:30:00
```

负 offset 允许保存，但 Capture Preview 必须显示 warning。系统不擅自清空或改为 0。

### 12.2 Instantiate

Instantiate 必须提交 `start_date=YYYY-MM-DD`，Web 默认 workspace 本地当天。

恢复：

```text
start date        2026-08-01
day_offset        3
local_time        09:30:00
        -> 2026-08-04 09:30:00 in workspace timezone
```

- 使用 workspace timezone 做 calendar add，不使用固定 `24h * days`。
- DST 不得改变 local_time。
- `due/until` 的 date-only 来源仍落到本地日末。
- `wait/scheduled` 的 date-only 来源仍落到本地日初。
- 超出安全时间范围返回 `project_template_date_out_of_range`。

## 13. Instantiate Preview

Instantiate Preview 输入：

```go
type InstantiateInput struct {
    TemplateRef     string            `json:"template"`
    SnapshotID      string            `json:"snapshot_id,omitempty"`
    ExpectedHash    string            `json:"expected_snapshot_hash"`
    ProjectSlug     string            `json:"project_slug"`
    ProjectName     string            `json:"project_name"`
    Description     *string           `json:"description,omitempty"`
    StartDate       string            `json:"start_date"`
    SecretInputs    map[string]string `json:"secret_inputs,omitempty"`
    AssigneeReplacements map[string]*string `json:"assignee_replacements,omitempty"`
}
```

未提交 `snapshot_id` 时解析 Template 当前 Snapshot；Preview 响应必须返回确定的 snapshot ID/hash，最终 Instantiate 必须原样携带。

Preview 校验：

- Template active、Snapshot 属于当前 workspace。
- project slug/name 合法且 slug 未占用。
- 当前 actor 拥有全部写权限。
- 当前 config schema 仍兼容。
- assignee 仍是 workspace active member。
- UDA definition 仍存在且值合法。
- automation provider/config key 可解析。
- secret placeholder 已由显式输入或有效 workspace/default 满足。
- 日期恢复合法。
- Snapshot ref 图无环、无缺失。

Preview 只返回 secret 的 `resolved_from=input|workspace|default|missing`，不回显值。
`assignee_replacements` 的 key 是 Snapshot 中不可用的原 user ID，value 是替代 user ID；value 为 null 表示显式移除。替代用户必须是当前 workspace active member。不存在 Preview issue 的 assignee 不允许被这个字段改写，避免 Instantiate 变成隐藏的模板编辑入口。

## 14. Instantiate 事务

### 14.1 顺序

```text
校验 expected snapshot hash
        |
        v
再次执行完整 Preview 校验
        |
        v
创建 planning Project
        |
        +--> 写 project config
        |
        +--> 预分配 Task/Series UUID 与项目 seq
        |
        +--> 创建 Series + initial RuleVersion
        |
        +--> 创建普通 Task
        |
        +--> 重建 parent / depends / TaskLink / ref://task
        |
        +--> 创建 disabled automation
        |
        +--> 写 audit / enqueue existing domain events
        |
        v
      COMMIT
```

全部操作位于同一 `Store.Transaction`。任何一步失败，Project 和所有子资源都不得留下。

### 14.2 新资源身份

- 新 Project status 固定为 planning。
- Project slug/name 来自 Instantiate 输入。
- Task、Series、RuleVersion、TaskLink、Automation 全部生成新 ID。
- Task/Series project seq 按 Snapshot 顺序分配。
- created_by 使用 Instantiate actor；用户身份输出走 `task.UserInfo` / `task.ActorInfo`。
- 新 Project 不保存到源 Project 的运行链接；来源通过 audit payload 记录。

### 14.3 事件与审计

Capture 是读源资源、写 Template/Snapshot：

- audit action：`project_template.create`、`project_template.snapshot.create`、`project_template.modify`、`project_template.archive`。
- payload 包含 template ID、snapshot ID/version/hash、source project ID 和 component counts，不含 secret。

Instantiate：

- aggregate audit action：`project_template.instantiate`。
- 创建出来的 Project/Task/Series/config/automation 继续走现有审计与已存在的领域事件，不伪装成“没有创建”。
- 事件 payload 增加 `source_template_id`、`source_template_snapshot_id`、`source_template_snapshot_hash`。
- v1 不新增可订阅 Hook event type；只增强实际资源创建事件的来源字段。
- 新 automation 创建时 disabled，因此不会在初始化阶段自动运行。

## 15. Template 生命周期

```text
                 append snapshot
       +--------------------------------+
       |                                v
   active v1 ------ append --------> active v2
       |                                |
       +----------- archive ------------+
                         |
                         v
                     archived
                         |
                     reactivate
                         |
                         v
                      active
```

- Template 创建与第一个 Snapshot 必须在同一事务完成。
- 追加 Snapshot 可以来自同 workspace 的任意可读 Project，不要求永远使用最初 source Project。
- 追加成功后原子更新 `current_snapshot_id`。
- archived Template 不允许 Capture 或 Instantiate。
- 重新激活不改变 current Snapshot。
- v1 不硬删除 Template/Snapshot。

## 16. 权限与隔离

不新增 `project_template:*` 权限，复用现有能力组合：

| 操作 | 基础权限 | 按内容追加权限 |
|---|---|---|
| list template metadata | `project:read` | 无 |
| get snapshot detail | `project:read` | 含 task/series 要 `task:read`；含 config 要 `config:read`；含 automation 要 `hook:read` |
| capture preview/create snapshot | `project:write` + source project 可读 | 同上对应 read |
| modify/archive/reactivate template | `project:write` | 无 |
| instantiate preview/create | `project:write` | 含 task/series 要 `task:write`；含 config 要 `config:write`；含 automation 要 `hook:write` |

附加规则：

- project-scoped token 对所有 Template API 返回 `project_scope_denied`；模板是 workspace 资源，不能借它越过 project allowlist。
- tenant actor 必须拥有上表全部 scope。
- 自然人继续受 workspace membership/role 约束。
- metadata list 不返回 Snapshot 内容。
- detail 权限不足时整体拒绝，不对同一个 Snapshot 做字段级残缺返回。
- 所有 repository 查询必须同时约束 workspace ID，不能只凭 template/snapshot UUID。

## 17. App 层用例

建议新增 `internal/app/project_template.go`，按职责拆分而不是堆入 `project.go`：

```go
PreviewProjectTemplateCapture(input CaptureInput) (CapturePreview, error)
CreateProjectTemplate(input CreateTemplateInput) (ProjectTemplateView, error)
CreateProjectTemplateSnapshot(templateRef string, input CaptureInput) (ProjectTemplateView, error)
ListProjectTemplates(includeArchived bool) ([]ProjectTemplateSummaryView, error)
ProjectTemplateInfo(templateRef string, snapshotID *string) (ProjectTemplateView, error)
ModifyProjectTemplate(templateRef string, input ModifyTemplateInput) (ProjectTemplateView, error)
ArchiveProjectTemplate(templateRef string) (ProjectTemplateView, error)
ReactivateProjectTemplate(templateRef string) (ProjectTemplateView, error)
PreviewProjectTemplateInstantiation(templateRef string, input InstantiateInput) (InstantiatePreview, error)
InstantiateProjectTemplate(templateRef string, input InstantiateInput) (ProjectView, error)
```

辅助逻辑拆分：

- `project_template_capture.go`：读取源资源、生成 local ref、转换日期。
- `project_template_reference.go`：依赖闭包、Markdown ref 重写和 conflict。
- `project_template_instantiate.go`：预分配身份、事务创建和来源 metadata。
- `internal/projecttemplate`：纯 Snapshot 模型/codec/hash/validation。

不要把 GORM row 或 App automation input 直接序列化进 Snapshot。Snapshot struct 是独立的持久契约，App 负责显式映射。

## 18. HTTP / Remote / CLI / MCP

### 18.1 HTTP

```text
POST /api/v1/project-templates/capture-preview
POST /api/v1/project-templates
GET  /api/v1/project-templates
GET  /api/v1/project-templates/{templateRef}
PATCH /api/v1/project-templates/{templateRef}
POST /api/v1/project-templates/{templateRef}/archive
POST /api/v1/project-templates/{templateRef}/reactivate
POST /api/v1/project-templates/{templateRef}/snapshots/capture-preview
POST /api/v1/project-templates/{templateRef}/snapshots
POST /api/v1/project-templates/{templateRef}/instantiate-preview
POST /api/v1/project-templates/{templateRef}/instantiate
```

- create Template 时同时提交第一个 Capture。
- Snapshot raw JSON 不进入公开响应。
- list 支持 `status=active|archived|all`、`limit`、`offset`，返回稳定分页元数据。
- Instantiate 响应返回标准 ProjectView 和 component create counts。
- OpenAPI runtime schema 必须同步。

### 18.2 Remote

Remote Client 提供与 HTTP 等价的 typed 方法，不能让 CLI 拼 HTTP body：

```go
PreviewProjectTemplateCapture(...)
CreateProjectTemplate(...)
ListProjectTemplates(...)
GetProjectTemplate(...)
CreateProjectTemplateSnapshot(...)
PreviewProjectTemplateInstantiation(...)
InstantiateProjectTemplate(...)
```

### 18.3 CLI

```text
xuanchu project template list
xuanchu project template info <template-ref>
xuanchu project template save <source-project> key:<key> name:<name>
xuanchu project template snapshot <template-ref> --from <source-project>
xuanchu project template instantiate <template-ref> <new-project-slug> name:<name>
xuanchu project template archive <template-ref>
xuanchu project template reactivate <template-ref>
```

选择参数：

- `--task <ref>`、`--series <ref>`、`--config <key>`、`--automation <id>` 可重复。
- `--all-open-tasks`、`--all-active-series` 是显式 convenience flag。
- 不传某一类选择参数表示该类为空，不表示全选。
- `--anchor-date` / `--start-date` 使用 `YYYY-MM-DD`。
- 非交互模式遇到 conflict 直接失败；drop resolution 必须通过显式 JSON input 文件提交。
- stdout 只输出结果，preview warning/human error 走 stderr；`--json` 返回稳定 DTO。

### 18.4 MCP

MCP tool 名称：

```text
project_template_capture_preview
project_template_create
project_template_list
project_template_get
project_template_modify
project_template_archive
project_template_reactivate
project_template_snapshot_capture_preview
project_template_snapshot_create
project_template_instantiate_preview
project_template_instantiate
```

规则：

- 名称全部使用下划线。
- 每次显式传 workspace 和 template/project ref，不依赖 active context。
- result 的 `content[0].text` 与 `structuredContent` 继续遵守现有 ToolEnvelope 契约。
- created_by/assignee 等用户对象统一为 `task.JSONUserInfo`；actor 使用 `task.JSONActorInfo`。
- tool schema golden、list-tools snapshot 和 Agent Skill 文档必须同步。

## 19. 错误码

| Code | 语义 |
|---|---|
| `project_template_not_found` | Template/Snapshot 不存在或不属于当前 workspace |
| `project_template_key_invalid` | key 不合法 |
| `project_template_key_conflict` | workspace 内 key 已存在 |
| `project_template_archived` | archived Template 禁止 Capture/Instantiate |
| `project_template_snapshot_schema_unsupported` | 当前二进制不支持 Snapshot schema |
| `project_template_snapshot_invalid` | JSON 或内部结构不合法 |
| `project_template_snapshot_hash_mismatch` | expected hash 与目标 Snapshot 不一致 |
| `project_template_source_changed` | Capture Preview 后源选择内容发生变化 |
| `project_template_selection_invalid` | 选择了外项目、deleted task 或 occurrence |
| `project_template_dependency_missing` | parent/depends/task ref 指向未选择 task |
| `project_template_attachment_unsupported` | description 含 attachment ref |
| `project_template_member_unavailable` | assignee 已不可用 |
| `project_template_config_invalid` | config schema/scope/value 已不兼容 |
| `project_template_secret_required` | secret 无输入且无有效继承来源 |
| `project_template_uda_invalid` | UDA definition/value 不兼容 |
| `project_template_automation_invalid` | automation provider/config 引用无效 |
| `project_template_date_out_of_range` | 日期恢复越界 |
| `project_template_ref_cycle` | parent/ref 图存在非法环 |

权限错误继续复用 `permission_denied` / `project_scope_denied`，不另造同义错误码。

## 20. Web Console 信息架构

### 20.1 入口

- Project Header “更多”菜单：`另存为模板`。
- Workspace Projects 页面主按钮旁：`从模板创建`。
- Workspace 设置新增 `项目模板` 页面，用于浏览、改名、追加 Snapshot、归档和查看版本。
- closed Project 仍可另存为模板；只读用户不显示入口。

### 20.2 Projects 页面

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 项目                                                    [从模板创建] [+ 新建] │
├──────────────────────────────────────────────────────────────────────────────┤
│ 搜索项目…                     状态 [进行中 v]                  12 个项目       │
├──────────────────────────────────────────────────────────────────────────────┤
│ 项目                     状态      任务       最近更新                  操作   │
│ 众筹耳机                 进行中    18/42      今天 10:32                [···] │
│ 夏季广告投放             预立项     0/16      昨天 18:10                [···] │
└──────────────────────────────────────────────────────────────────────────────┘
```

`从模板创建` 是与空白新建并列的明确入口，不把模板塞进普通创建表单的次要下拉项。

### 20.3 另存为模板：步骤一

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 另存为项目模板                                                       [×]     │
│ 1 基本信息 ── 2 选择内容 ── 3 检查 ── 4 完成                                │
├──────────────────────────────────────────────────────────────────────────────┤
│ 模板名称 *                                                                  │
│ [ 众筹产品标准启动流程                                                   ]   │
│                                                                            │
│ 稳定 Key *                                                                 │
│ [ crowdfunding-launch                                                   ]   │
│ 创建后不可直接修改，用于 CLI / MCP 稳定引用                                │
│                                                                            │
│ 模板说明                                                                   │
│ [ 包含预热、上线、交付阶段的任务和循环检查                              ]   │
│                                                                            │
│ 模板基准日 *                                                               │
│ [ 2026-07-20 ]  所有计划日期将转换为相对这个日期的偏移                      │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                        [取消] [下一步 →]    │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 20.4 选择内容

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 另存为项目模板                                                       [×]     │
│ 1 基本信息 ── 2 选择内容 ── 3 检查 ── 4 完成                                │
├──────────────────────────────────────────────────────────────────────────────┤
│ [任务 8] [循环任务 2] [配置 4] [自动化 1]                   搜索… [       ] │
├──────────────────────────────────────────────────────────────────────────────┤
│ [✓] 选择未完成任务（8）                         已选择 8 / 可选 21           │
│                                                                            │
│ [✓] 产品资料准备       待办      D+0  负责人：小王                          │
│     └─ [✓] 整理卖点     待办      D+1  依赖：产品资料准备                    │
│ [✓] 预热素材评审       等待      D+3  负责人：设计组                        │
│ [✓] 上线检查           已完成    D+7  新项目中将恢复为待办                  │
│ [ ] 历史数据复盘       已完成   D-12                                      │
│                                                                            │
│ 说明：循环实例、已删除任务和附件不会进入模板                               │
├──────────────────────────────────────────────────────────────────────────────┤
│                                              [← 上一步] [检查选择 →]         │
└──────────────────────────────────────────────────────────────────────────────┘
```

四个 Tab 都必须显示已选数量。切换 Tab 不丢选择；搜索只过滤当前列表，不改变 selection。Series 行同时显示规则、首个相对日期和 until。Config 行显示来源；workspace/default 行只读且不可选择。Secret 行显示“创建项目时填写”，不展示值。Automation 行显示触发方式并标注“创建后默认停用”。

### 20.5 引用冲突

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 检查模板内容                                                         [×]     │
│ 发现 2 个必须处理的问题，1 个提醒                                            │
├──────────────────────────────────────────────────────────────────────────────┤
│ ! 缺少依赖                                                                  │
│   “上线检查”依赖“支付回归”，但“支付回归”未选择                              │
│                                      [同时选择支付回归] [移除这条依赖]        │
│                                                                            │
│ ! 不支持附件                                                                │
│   “产品资料准备”的描述引用了 2 个附件                                       │
│                                      [取消选择该任务] [返回项目处理]          │
│                                                                            │
│ △ 日期提醒                                                                  │
│   “历史数据复盘”的截止日期为 D-12，新项目创建后会立即逾期                    │
│                                      [保留 D-12] [编辑为 D+0]                │
├──────────────────────────────────────────────────────────────────────────────┤
│ 0 个未处理问题                                       [← 返回选择] [保存模板] │
└──────────────────────────────────────────────────────────────────────────────┘
```

blocking issue 未清零时 `保存模板` 禁用。任何 drop/edit 都进入 Capture request 的显式 resolution/override，服务端重新验证。

### 20.6 模板库

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 工作区设置 / 项目模板                                      [+ 保存新模板]   │
├───────────────────────────────┬──────────────────────────────────────────────┤
│ 搜索模板…                     │ 众筹产品标准启动流程                         │
│                               │ key: crowdfunding-launch                    │
│ ● 众筹产品标准启动流程   v3   │ 当前版本 v3 · 2026-07-20 14:30              │
│   任务 12 · 循环 3 · 配置 4  │                                              │
│                               │ 任务 12   循环任务 3                          │
│ ○ 广告周报项目          v1   │ 配置 4    自动化 2（创建后停用）             │
│   任务 4 · 自动化 1          │                                              │
│                               │ 来源：众筹耳机                               │
│                               │ [从模板创建] [从项目更新快照] [···]          │
│                               ├──────────────────────────────────────────────┤
│                               │ 版本                                         │
│                               │ v3 当前   07-20  12/3/4/2                    │
│                               │ v2        07-18  10/2/4/1                    │
│                               │ v1        07-10   8/2/3/1                    │
└───────────────────────────────┴──────────────────────────────────────────────┘
```

旧 Snapshot 默认只读。v1 可以查看摘要并从指定版本创建，不提供直接编辑 Snapshot JSON。

### 20.7 从模板创建

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 从模板创建项目                                                       [×]     │
│ 1 选择模板 ── 2 项目信息 ── 3 配置检查 ── 4 确认                             │
├──────────────────────────────────────────────────────────────────────────────┤
│ 模板：众筹产品标准启动流程 / v3                       [更换]                  │
│                                                                            │
│ 项目名称 *                  项目 Slug *                                     │
│ [ 智能翻译耳机          ]   [ transbuds     ]                               │
│                                                                            │
│ 项目启动日 *                                                               │
│ [ 2026-08-01 ]  任务与循环日期按这个日期重新计算                            │
│                                                                            │
│ 项目描述                                                                   │
│ [ 包含预热、上线、交付阶段……                                            ]   │
├──────────────────────────────────────────────────────────────────────────────┤
│ 将创建：12 个任务 · 3 个循环任务 · 4 项配置 · 2 条停用自动化                 │
│                                                  [取消] [检查配置 →]         │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 20.8 Secret 与最终确认

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 配置检查                                                             [×]     │
├──────────────────────────────────────────────────────────────────────────────┤
│ ✓ agent.provider.base_url      继承 workspace                               │
│ ✓ agent.provider.model         模板值：gpt-5                                │
│ ! agent.provider.api_key       当前无有效值                                  │
│   [ ••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••• ]       │
│                                                                            │
│ △ 负责人“李雷”已不在 workspace                                              │
│   受影响：素材评审、上线检查                         [替换负责人] [移除]     │
│                                                                            │
│ 自动化                                                                     │
│ 2 条规则将以停用状态创建；完成配置检查后可在项目“自动化”页手动启用          │
├──────────────────────────────────────────────────────────────────────────────┤
│                                            [← 上一步] [确认并创建项目]        │
└──────────────────────────────────────────────────────────────────────────────┘
```

提交期间 Dialog 不可关闭和重复提交。成功后跳转到新项目 Overview，Toast 明确显示“已从模板创建：12 个任务、3 个循环任务、2 条停用自动化”。

### 20.9 移动端

移动端使用全屏 Sheet：

```text
┌──────────────────────────────┐
│ ← 选择模板内容          2/4  │
├──────────────────────────────┤
│ [任务 8] [循环 2] [配置 4]  │
│ [自动化 1]                  │
│                              │
│ [✓] 产品资料准备             │
│     D+0 · 小王               │
│ [✓] └ 整理卖点               │
│     D+1 · 依赖 1 项           │
│ [ ] 历史数据复盘             │
│     已完成 · D-12             │
│                              │
├──────────────────────────────┤
│ 已选择 8          [检查选择] │
└──────────────────────────────┘
```

选择状态放在上层 wizard state，不因 Tab 或 Sheet 层级切换丢失。错误项点击后跳回对应组件并聚焦条目。

## 21. 状态、空态和可访问性

- Template list 区分 loading、error、empty、no-search-result，不能把 API 失败显示成“暂无模板”。
- source Project 没有可选内容时仍可保存只含 Project description 的空结构模板。
- archived Template 显示只读 Banner 和“重新激活”；隐藏 Capture/Instantiate。
- Snapshot schema 不支持时显示当前二进制版本不支持，不能渲染成空 Snapshot。
- Dialog/Sheet 使用现有 shadcn primitives；一个流程只存在一个 modal root。
- 所有 checkbox、Tab、issue action、日期输入和关闭按钮有明确 label/aria-label。
- 键盘支持 Tab 顺序、Space 勾选、Esc 关闭非提交态、Cmd/Ctrl+Enter 提交当前可提交步骤。
- pending 时禁用关闭、返回和重复提交。
- 文案使用真实行为：“创建后自动化保持停用”“不会复制附件”，不使用模糊类比。

## 22. 安全与资源上限

默认上限：

| 项目 | 上限 |
|---|---:|
| 单 Snapshot task | 1,000 |
| 单 Snapshot series | 200 |
| 单 Snapshot config | 500 |
| 单 Snapshot automation | 200 |
| Snapshot canonical JSON | 8 MiB |
| 单 description/instruction/system prompt | 现有字段上限；若当前无统一上限，计划中先补 |
| 单次 secret input | 现有 project config value 上限 |

安全要求：

- request body 在 HTTP 层设置合理上限。
- Snapshot 解码先检查总字节，再检查数组和字符串长度。
- secret input 不进入 Snapshot、audit、日志、error message 或 preview response。
- raw Snapshot JSON 不暴露给普通客户端。
- Instantiate 只允许当前 workspace，不能接受 workspace override。
- Automation validation 继续复用 provider allowlist/SSRF 边界。
- description 中的 attachment ref 不允许跨项目借用。
- hash 比较使用固定长度值并做规范化验证。

## 23. SQLite / PostgreSQL 与并发

- 两种数据库都使用 TEXT 保存 Snapshot JSON，避免 JSON/JSONB 操作差异。
- Template version 分配在事务中锁定 Template row；PostgreSQL 使用行锁，SQLite 复用写事务串行化。
- 并发追加 Snapshot 时 version 不能重复；唯一约束是最终兜底，App 将冲突映射为可重试错误，不能泄漏方言错误。
- 并发 Instantiate 相同 Template 是合法的，只要新 Project slug 不冲突。
- expected hash 防止 Preview 与 Instantiate 之间 current Snapshot 被切换。
- 创建 Project 及全部子资源必须在同一数据库事务内完成。

## 24. 测试策略

### 24.1 `internal/projecttemplate`

- V1 round-trip。
- unknown schema、unknown field、trailing JSON、invalid ref 拒绝。
- canonical order/hash 稳定。
- v1 -> current upgrade。
- relative date 在 DST 边界仍保持 local time。
- dependency cycle/missing ref。
- secret literal invariant。
- size/count limits。

### 24.2 Storage

- Template/Snapshot CRUD 和 workspace 隔离。
- key/version/hash unique。
- Snapshot immutable。
- archive/reactivate。
- SQLite/PostgreSQL version concurrency。
- migration forward compatibility。

### 24.3 App

- 四类内容任意组合。
- 精确选择，不误带未选择资源。
- completed task 重置，occurrence 排除。
- Series history/occurrence 排除并生成单一初始 RuleVersion。
- parent/depends/Markdown task ref 映射。
- missing dependency resolution。
- attachment 阻断。
- config 显式值与 workspace/default 边界。
- secret 从不进入 Snapshot/audit/response。
- automation 全部 disabled，delivery 不复制。
- assignee/Config/UDA 在 Instantiate 时重新验证。
- project-scoped token 拒绝。
- 任一步失败整体回滚。
- actor/user 输出结构统一。

### 24.4 协议与 Web

- HTTP/Remote/MCP/CLI DTO 等价。
- OpenAPI 与 MCP schema golden。
- `content[0].text` / `structuredContent` 等价。
- Web 四类选择、搜索不丢 selection、冲突处理、secret 不回显。
- Snapshot 版本切换和 expected hash。
- desktop/mobile ASCII 原型对应的关键交互 E2E。
- closed/archived/permission/loading/error/empty 状态。

### 24.5 完整验证

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check

pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
```

新增 project-template Playwright smoke；如果实现改动任务创建/编辑共享组件，同时运行 `pnpm --dir web run smoke:editing`。

## 25. 验收标准

1. 用户能从任意同 workspace Project 保存新 Template 和第一个 Snapshot。
2. Capture 时能分别、逐项选择 config/task/series/automation。
3. 未选择的资源不会进入 Snapshot。
4. Snapshot 内容只存在一个版本化 JSON 字段，组件字段变化不要求新增模板子表。
5. JSON 有明确 Go struct、strict codec、schema dispatch、canonical hash 和 unknown schema 错误。
6. Template 更新追加 Snapshot version，不覆盖旧版本。
7. 普通 task 生成新 UUID 和开放状态，内部依赖和 task ref 正确映射。
8. Series 生成新 ID、active 状态和单一初始 RuleVersion，不携带 occurrence/history。
9. project config 只复制显式值；secret 值从未进入 Snapshot。
10. Automation 只复制定义且全部 disabled，不复制 delivery。
11. 日期按 anchor/start date 和 workspace timezone 正确平移。
12. attachment ref、缺失依赖、不可用 member、失效 config/UDA/automation 在 Preview 中明确阻断。
13. Instantiate 任一步失败不留下 Project 或子资源。
14. Template/Snapshot 严格 workspace 隔离，project-scoped token 不能访问。
15. CLI、HTTP、Remote、MCP、Web Console 行为一致。
16. 用户身份输出符合 `task.UserInfo` / `task.JSONUserInfo` 规范。
17. SQLite/PostgreSQL 与 `CGO_ENABLED=0` 验证通过。

## 26. 实施切片建议

implementation plan 应按以下顺序拆分，但本文不代替计划：

1. `internal/projecttemplate` V1 model、codec、hash、relative date 与纯函数测试。
2. Template/Snapshot storage model、migration、repository 与双数据库测试。
3. Capture Preview/Capture App 用例、引用归一化和审计。
4. Instantiate Preview/Instantiate App 事务、日期恢复和 ID 映射。
5. HTTP + OpenAPI + Remote。
6. CLI。
7. MCP + golden + Agent Skill 文档。
8. Web Template Library、Capture wizard、Instantiate wizard。
9. E2E、README、ROADMAP 和 release 文档同步。

每一步必须先写失败测试，再补实现；不得等到最后才验证 secret、workspace 隔离和回滚。

## 27. 文档同步

实现完成后同步：

- `README.md`：项目模板入口、行为边界、CLI 示例。
- `ROADMAP.md`：v0.6.0 状态与交付内容。
- OpenAPI runtime spec。
- `docs/skills`：MCP tool 名称、输入输出和 Agent 使用边界。
- 对应 implementation plan 的完成状态。

在用户审阅并确认本规格前，不进入 implementation plan 或代码实现。
