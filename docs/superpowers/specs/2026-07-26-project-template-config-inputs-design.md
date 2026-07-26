# 璇础项目模板配置输入设计

> **给 agentic workers 的要求：** 本规格通过评审后，先使用 `superpowers:writing-plans` 拆成实施计划，再开始编码。实现必须遵循 Red → Green → Refactor，并同步 HTTP/OpenAPI、Web Console、CLI、Remote、MCP 和用户文档。

**日期：** 2026-07-26
**状态：** 已完成（2026-07-26）
**目标版本：** v0.6.3
**依赖规格：** [璇础项目模板与版本化快照设计](2026-07-20-project-template-snapshot-design.md)
**背景需求：** 模板作者可以把部分 project config 声明为“从模板创建项目时填写”的表单项，并决定是否必填；创建者必须在创建流程中完成这些输入，系统校验通过后再原子创建项目。

## 1. 结论

本功能不是给 Project 增加另一套字段系统，也不是把所有 `ConfigDefinition.required` 强制塞进项目创建页。它是在某个不可变 Template Snapshot 中，为已经选择的 project config 增加“实例化时如何取得值”的规则。

每个被模板选中的 config 只能采用一种策略：

| 策略 | 产品含义 | 新项目中的 project config row |
|---|---|---|
| `fixed` | 使用模板保存的值，保持当前行为 | 一定创建 |
| `prompt` + `required=true` | 创建者本次必须显式填写 | 校验通过后创建 |
| `prompt` + `required=false` | 创建页展示选填项 | 填写时创建；留空时不创建 |

锁定以下决策：

- 规则属于 **Snapshot**，不属于 `project_templates` 元数据，也不属于全局 `config_definitions`。
- 新 Snapshot 使用 `xuanchu.project-template-snapshot/v2`；旧 v1 Snapshot 保持不可变并继续可读、可实例化。
- v2 仍保存在现有 `project_template_snapshots.snapshot_json TEXT` 中，不新增模板子表，不修改模板关系表结构。
- `required=true` 表示“创建者必须在本次表单中显式填写”，workspace 继承值和 schema default 都不能替代这次输入。
- `ConfigDefinition.required` 继续表示全局配置完整性提示；它不自动等于模板表单必填，也不自动改变普通项目创建行为。
- 表单控件、类型、枚举、secret 属性、默认 label 和说明来自实例化时当前 workspace 的 `ConfigDefinition`；Snapshot 不复制 definition。
- 所有输入都由 App 层按当前 definition 归一化和校验，Web/CLI/MCP 不自行决定合法性。
- prompt 输入统一使用 `config_inputs`；不再把非 secret 输入塞进现有 `secret_inputs`。
- secret 输入不进入 Snapshot、公开响应、Preview、审计、日志或错误；浏览器提交结束后清空本地 secret state。
- 从模板创建仍是一笔事务：任何必填项缺失或 config 不合法时，一个 Project row 都不能留下。

## 2. 现状核对

### 2.1 当前代码链路

当前实现已经具备本功能所需的大部分边界：

- `internal/projecttemplate/model.go` 的 `ConfigBlueprintV1` 使用 `literal|secret_input|secret_copy` 表达 Snapshot 内 config 值的保存方式。
- `internal/app/project_template_capture.go` Capture 时把普通值保存为 `literal`，把 secret 加密后保存为 `secret_copy`。
- `internal/app/project_template_instantiate.go` 的 `planInstantiateConfigs` 在 Preview 和最终创建前重新读取当前 `ConfigDefinition`，校验 project scope、类型、枚举和 secret。
- `InstantiateInput` 目前只有 `secret_inputs`，只解决历史 `secret_input` 的补值，没有通用 config 表单输入。
- Web 的实例化向导已经有“选择模板 → 项目信息 → 处理问题 → 确认创建”四步，但配置区只渲染 secret resolution，不渲染普通 config 输入。
- Capture 候选项已经返回 config `key/label/mode/value_type`，可以延续已有 config 选择界面；但当前候选只含源 Project 显式 row，本功能还需让允许 project scope、尚无显式 row 的 ConfigDefinition 以“仅可创建时填写”方式加入。

### 2.2 当前 ConfigDefinition 语义

`config_definitions` 已经保存：

```text
workspace_id
key
value_type          // string|number|boolean|json|date|datetime
allowed_scopes_json
label
description
enum_values_json
default_value
has_default
required
secret
show_on_console_home
```

现有 `required` 只在 effective config 读取时产生 `missing_required` 状态，供 Web Console 提示项目仍缺配置；它不会阻止 `AddProject`，也不要求用户必须在某个创建表单内显式输入。

因此不能直接复用它作为本功能唯一开关：

- 同一个 config 可能只在某个模板中必须逐项目填写，在其它模板或普通项目中允许继承。
- `ConfigDefinition.required=true` 可能已有存量语义，直接升级为创建阻断会改变普通项目创建和既有模板行为。
- 模板必填必须随 Snapshot 版本冻结，definition 的后续修改不能悄悄重写历史模板意图。

### 2.3 当前数据库边界

模板数据目前由两张关系表承载：

```text
project_templates
project_template_snapshots
  └─ snapshot_json TEXT
```

Project config 仍使用：

```text
configs PK(workspace_id, scope, scope_id, key)
```

这意味着 prompt 规则只需进入 Snapshot JSON；实例化成功后仍按现有主键写入 `scope=project, scope_id=<new-project-id>` 的 config row。SQLite 与 PostgreSQL 都不需要新增列或方言 JSON 类型。

存量 SQLite 可能停留在模板表出现之前，或缺少后来由 AutoMigrate 增加的 ConfigDefinition 列。实现验证必须包含旧版 fixture，使用当前 `storage.Open` 升级后再验 schema、外键和原数据，不能只在全新数据库上测试。

## 3. 目标

1. 模板作者可以对每个已选 project config 指定 `fixed` 或 `prompt`；没有源项目显式值的 project-scoped definition 也可以直接声明为 prompt。
2. `prompt` 可以设为必填或选填。
3. 创建者在从模板创建的同一向导内看到按当前 ConfigDefinition 生成的 typed form。
4. 必填 prompt 必须由本次请求显式提交，不能被继承值或 default 隐式满足。
5. 普通和 secret config 使用同一输入契约，但 secret 始终按敏感数据边界处理。
6. HTTP、Web、CLI、Remote、MCP 对同一 Snapshot 的字段要求一致。
7. 旧 v1 Snapshot 行为不变；升级不修改旧 Snapshot JSON/hash。
8. Snapshot 变化仍由 ID/hash 固定，表单字段变化不能绕过 expected hash 校验。
9. 创建事务继续保证 Project、config、task、series、automation 全部成功或全部回滚。

## 4. 非目标

- 不让模板新增 workspace 中不存在的 ConfigDefinition。
- 不把 ConfigDefinition 复制进 Snapshot，也不冻结其类型、枚举或默认值。
- 不改变普通“新建空项目”的表单和 `AddProject` 行为。
- 不把所有 `ConfigDefinition.required=true` 自动加入每个模板。
- 不提供条件显示、字段联动、正则表达式、任意 JSON Schema 或脚本校验。
- 不支持一个 prompt 写入多个 config key。
- 不支持从模板创建后继续保留“待填写的动态字段”；创建完成后只留下普通 project config row。
- 不在 Project 表增加模板表单答案 JSON。
- 不回显 secret 当前值、模板密文、继承值或用户刚提交的 secret。
- 不允许实例化时提交 Snapshot 未声明的任意 config，避免把模板入口变成隐藏的通用 config 批量写接口。
- 不改变 project automation 创建后固定 disabled 的规则。

## 5. 术语

| 术语 | 含义 |
|---|---|
| Config Policy / 配置策略 | Snapshot 对某个 config key 的取值规则 |
| Fixed Config / 固定配置 | Snapshot 保存值，实例化时直接写入新项目 |
| Prompt Config / 创建时填写配置 | Snapshot 只保存输入要求，不保存来源项目值 |
| Required Prompt / 必填项 | 本次实例化请求必须显式提交非空、合法值 |
| Optional Prompt / 选填项 | 创建页展示，但允许不提交；不提交时不写 project row |
| Form Descriptor / 表单描述 | App 根据 Snapshot 规则和当前 ConfigDefinition 生成的安全只读字段描述 |
| Explicit Input / 显式输入 | 当前 Instantiate 请求的 `config_inputs[key]`，不是 workspace/default 解析结果 |

## 6. 方案比较

### 6.1 直接使用 `ConfigDefinition.required`

优点是无需改 Snapshot；缺点是它是 workspace 全局定义，不能表达“只对某个模板要求填写”，也不能随 Snapshot 版本审计。还会把普通项目创建和所有历史模板一起改变。不采用。

### 6.2 在 `project_templates` 增加 `required_config_keys`

优点是查询直接；缺点是 Template 元数据会与不可变 Snapshot 内容分叉：切换 current Snapshot、实例化历史版本或追加 Snapshot 时都可能读到错误规则。也无法表达选填项和后续输入扩展。不采用。

### 6.3 新建 `project_template_config_inputs` 子表

优点是关系查询方便；缺点是输入规则只在 Snapshot 整体读取和实例化时使用，会破坏现有“Snapshot JSON 是完整初始化定义”的原子边界，增加版本复制、hash 和外键一致性成本。不采用。

### 6.4 Snapshot v2 内建 config policy

输入规则与值、任务、Series、Automation 一起进入不可变 JSON 和 canonical hash。App 仍按当前 ConfigDefinition 做运行时校验，数据库无需迁表。采用此方案。

## 7. Snapshot v2 契约

### 7.1 Schema

新增：

```go
const SnapshotSchemaV2 = "xuanchu.project-template-snapshot/v2"

type SnapshotV2 struct {
    Schema      string                   `json:"schema"`
    AnchorDate  string                   `json:"anchor_date"`
    Project     ProjectBlueprintV2       `json:"project"`
    Configs     []ConfigBlueprintV2      `json:"configs"`
    Tasks       []TaskBlueprintV2        `json:"tasks"`
    Series      []SeriesBlueprintV2      `json:"series"`
    Automations []AutomationBlueprintV2  `json:"automations"`
}
```

本功能只改变 config blueprint；其它 v2 结构与 v1 等价。仍应定义独立 v2 struct 和显式升级函数，不使用 type alias 偷渡持久协议。

### 7.2 Config blueprint

```go
type ConfigBlueprintV2 struct {
    Key              string         `json:"key"`
    Mode             string         `json:"mode"` // literal|secret_copy|prompt
    Value            *string        `json:"value,omitempty"`
    SecretCiphertext *string        `json:"secret_ciphertext,omitempty"`
    Prompt           *ConfigPromptV2 `json:"prompt,omitempty"`
}

type ConfigPromptV2 struct {
    Required bool `json:"required"`
}
```

字段组合必须严格校验：

| mode | value | secret_ciphertext | prompt |
|---|---|---|---|
| `literal` | 必须存在 | 必须为空 | 必须为空 |
| `secret_copy` | 必须为空 | 必须存在且非空 | 必须为空 |
| `prompt` | 必须为空 | 必须为空 | 必须存在 |

同一 Snapshot 内 config key 必须唯一。`prompt.required=false` 是有效值，不能因为 Go 零值在 codec 中丢失整个 `prompt` 对象。

### 7.3 为什么 Snapshot 不保存 label/type/enum

Snapshot 只冻结“这个 key 要不要创建者填写、是否必填”。字段的合法值由当前 workspace 的 ConfigDefinition 决定：

- definition 已删除：Preview 阻断。
- 不再允许 project scope：Preview 阻断。
- type 改变但输入仍可归一化：按新 type 接受。
- enum 改变：按新 enum 校验。
- label/description 改变：创建表单展示新文案，不影响 Snapshot hash。
- secret 标志改变：按当前 secret 边界处理；任何历史 literal 都不能因此被公开。

这与现有 v1 Instantiate “使用当前 ConfigDefinition 重新验证”的边界一致，避免两套 schema 长期漂移。

### 7.4 v1 升级

Decoder 流程保持 header 分派和 `DisallowUnknownFields()`：

```text
decode v1 → validate v1 → upgradeV1ToCurrent
decode v2 → validate v2 → current internal model
```

v1 映射：

| v1 mode | 当前内部策略 | 行为 |
|---|---|---|
| `literal` | fixed literal | 不变 |
| `secret_copy` | fixed secret copy | 不变 |
| `secret_input` | legacy secret resolution | 保持“显式输入优先，否则 workspace/default”兼容行为 |

不能把历史 `secret_input` 自动升级为 v2 required prompt，否则会改变旧 Snapshot 的可实例化条件。新 Capture 不再写 v1 或 `secret_input`；默认写 v2。

旧 Snapshot JSON、schema、hash 和 version 不原地修改。模板追加新 Snapshot 后，只有新版本采用 v2。

## 8. Capture 规则

### 8.1 候选项边界

原 v1 规格只列源 Project 显式 config row，是为了防止把 workspace 继承值或 default 伪装成“项目值”。这个边界对 fixed 仍然成立，但不能限制 prompt：prompt 本来就不保存源值，要求模板作者先在源项目造一个占位 row 没有产品意义。

Config 候选列表扩展为当前 workspace 中所有允许 project scope 的 ConfigDefinition，并返回：

```go
type ConfigCandidateView struct {
    Ref             string `json:"ref"`
    Key             string `json:"key"`
    Label           string `json:"label"`
    ValueType       string `json:"value_type"`
    Secret          bool   `json:"secret"`
    HasProjectValue bool   `json:"has_project_value"`
    EffectiveSource string `json:"effective_source"` // project|workspace|default|missing
    CanFixed        bool   `json:"can_fixed"`
    WarningCount    int    `json:"warning_count"`
}
```

- `has_project_value=true`：可以选 fixed 或 prompt。
- `has_project_value=false`：只能选 prompt；不能把 workspace/default 值复制为 fixed。
- 候选响应只返回 effective source，不返回 effective value。
- secret 只返回是否已配置的安全状态，不返回值。
- 现有 `q/mode/refs/limit/offset` 继续使用稳定分页；`mode` 支持 `all|fixed_available|prompt_available|secret|non_secret`，旧 `literal|secret` filter 做兼容映射。
- `selection.config_keys` 可以包含没有源 project row 的 definition key；Capture 必须结合 policy 决定是否合法。
- prompt-only definition 的 schema 指纹或 `modified_at` 必须进入 Capture `source_hash`，避免 Preview 后 definition 改变却仍被当作同一来源状态保存。

### 8.2 输入契约

在现有 `CaptureInput` 增加：

```go
type CaptureConfigPolicyInput struct {
    Key      string `json:"key"`
    Strategy string `json:"strategy"` // fixed|prompt
    Required bool   `json:"required,omitempty"`
}

type CaptureInput struct {
    // 既有字段省略
    ConfigPolicies []CaptureConfigPolicyInput `json:"config_policies,omitempty"`
}
```

规则：

- `config_policies[].key` 必须属于最终 `selection.config_keys`，不能给未选 config 配规则。
- 同一 key 不能重复。
- `strategy=fixed` 时 `required` 必须为 false。
- `strategy=prompt` 时才允许 required true/false。
- 未提供 policy 的已选 key 默认 `fixed`，兼容旧 Web/API 调用方。
- 服务端完成 automation config 依赖闭包后，再验证最终 selection 与 policy。

### 8.3 Fixed

保持当前行为：

- key 必须存在源 Project 显式 config row；否则返回 policy invalid。
- 非 secret：保存源 Project 显式值为 `literal`。
- secret：使用 `[security].config_secret_key` 加密为 `secret_copy`。
- fixed secret 没有可用加密 key 时 Capture 阻断。

### 8.4 Prompt

- Snapshot 不保存来源项目的该 config 值，普通值和 secret 都不得进入 `value/secret_ciphertext`。
- Capture 时必须确认当前 ConfigDefinition 存在并允许 project scope。
- 不要求源 Project 已有显式 config row；没有 row 的 definition 也能成为 prompt。
- `required=true` 只冻结模板要求，不复制 definition 的全局 required。
- `required=false` 表示“这个模板希望在创建页提供快捷填写，但允许跳过”。
- 即使源 Project 当前值为空字符串，只要其 row 存在并被选择，也可以改成 prompt；来源值不会成为表单默认值。
- 表单不预填源项目值，避免复制项目特有标识，也避免 secret 泄漏。

### 8.5 Automation 依赖

选择 Automation 后，服务端仍按现有规则闭包显式 project config 依赖并锁定 key。模板作者可以把被锁定 key 从 fixed 改为 prompt，但：

- 被 automation 依赖的 prompt 必须 `required=true`。
- Web 禁用其“选填”开关并说明“自动化依赖此配置”。
- 后端独立验证，不能信任 Web 锁定状态。
- 取消所有依赖该 key 的 automation 后，才能改回选填或移除该 config。

原因是当前 Instantiate Preview 会验证 automation provider/config contract。允许依赖字段选填只会把“选填”变成最终仍被 automation 阻断的假选项。

### 8.6 Source hash 与 Snapshot hash

- `source_hash` 继续描述源项目选择内容及其并发漂移，不把 UI policy 当作源数据；对没有源 row 的 prompt，还要覆盖当前 definition 指纹，避免 Capture Preview 与保存之间的 schema 漂移。
- `config_policies` 必须参与最终 Snapshot JSON，因此自然参与 canonical `snapshot_hash`。
- Capture Preview 与最终 Capture 都使用请求中的 policy 重新构造 Snapshot；最终 hash 不接受客户端提交。

## 9. 实例化输入与解析

### 9.1 InstantiateInput

```go
type InstantiateInput struct {
    SnapshotID           string             `json:"snapshot_id,omitempty"`
    ExpectedHash         string             `json:"expected_snapshot_hash"`
    ProjectSlug          string             `json:"project_slug"`
    ProjectName          string             `json:"project_name"`
    Description          *string            `json:"description,omitempty"`
    StartDate            string             `json:"start_date"`
    ConfigInputs         map[string]string  `json:"config_inputs,omitempty"`
    SecretInputs         map[string]string  `json:"secret_inputs,omitempty"` // 仅兼容 v1
    AssigneeReplacements map[string]*string `json:"assignee_replacements,omitempty"`
}
```

`config_inputs` 的 value 统一是字符串，复用 `normalizeScopedConfigValue` 转换 number/boolean/json/date/datetime。这样 HTTP、CLI、MCP 和 Web 共享一个稳定输入形状，不在协议层制造多套 JSON scalar 规则。

### 9.2 Prompt 解析

对每个 `mode=prompt`：

1. 读取当前 workspace ConfigDefinition。
2. 校验仍允许 project scope。
3. 判断请求是否显式包含 `config_inputs[key]`。
4. required 且缺 key：返回 blocking issue。
5. required 且值为空：返回 blocking issue。这里的空值判断在 type normalize 之前执行；空白 string 也视为未填写。
6. optional 且缺 key或空白：不创建 project config row。
7. 有值：按当前 type/enum/secret 规则归一化和校验，写入计划中的 project config map。

required prompt 不调用 effective config resolver 来满足自身要求。即使 workspace/default 已有有效值，创建者仍必须填写，因为模板作者要求的是“为这个新项目明确给值”。

optional prompt 留空时不写 row，但后续 Automation 校验和项目 effective config 仍可按既有顺序读取 workspace/default。由于 automation 依赖 prompt 已被强制 required，这里不会出现产品文案与阻断行为矛盾。

### 9.3 输入白名单

- `config_inputs` 只能包含目标 Snapshot 的 prompt key。
- v2 prompt key 不能同时出现在 `secret_inputs`。
- fixed key、Snapshot 未声明 key、其它 workspace key一律返回 `project_template_config_input_unknown`。
- v1 `secret_input` 继续只接受 `secret_inputs`，不要求旧调用方迁移请求字段。
- 最终 Instantiate 必须重复完整 Preview 校验，不能只信任前一次 Preview。

### 9.4 ConfigDefinition.required 的关系

| Snapshot 规则 | Definition.required | 创建时行为 |
|---|---:|---|
| fixed | 任意 | 写模板值 |
| prompt required | 任意 | 必须显式填写 |
| prompt optional | false | 可跳过 |
| prompt optional | true | 仍可跳过；创建后 effective config 若缺失，继续显示现有 `missing_required` |
| Snapshot 未包含 | true | 不进入模板表单；创建后按现有完整性提示处理 |

这条边界避免通过模板功能偷偷升级全局 required 的含义。若未来要让所有项目创建都阻断缺失的全局 required，应单独立项。

## 10. 表单描述与 Preview

### 10.1 安全描述对象

App 层根据 Snapshot prompt 和当前 ConfigDefinition 生成：

```go
type ProjectTemplateConfigInputView struct {
    Key         string   `json:"key"`
    Label       string   `json:"label"`
    Description string   `json:"description"`
    ValueType   string   `json:"value_type"`
    EnumValues  []string `json:"enum_values"`
    Required    bool     `json:"required"`
    Secret      bool     `json:"secret"`
    Status      string   `json:"status"` // ready|definition_missing|scope_invalid
}
```

约束：

- `label` 为空时展示 key。
- `enum_values` 只返回 definition 的公开枚举，不返回 default value。
- 不返回 fixed literal、secret ciphertext、workspace value、default value或用户输入。
- definition 已失效时仍返回 key 和 status，便于 UI 解释，但 Preview 必须阻断。
- 顺序按 Snapshot `configs` 顺序，不能依赖 map 遍历。

### 10.2 Snapshot 摘要

`ProjectTemplateSnapshotSummaryView` 增加：

```json
{
  "config_inputs": [
    {
      "key": "feishu.chat_id",
      "label": "项目飞书群",
      "description": "填写当前项目使用的飞书群 chat_id",
      "value_type": "string",
      "enum_values": [],
      "required": true,
      "secret": false,
      "status": "ready"
    }
  ]
}
```

CLI/Remote/MCP 只有 list + instantiate，无法调用治理 detail，因此 list 必须返回这组安全描述，而不是只返回 key。现有 `required_secret_keys` 为 v1 兼容保留；v2 客户端应使用 `config_inputs`。

### 10.3 Preview 响应

`InstantiatePreview` 增加：

```go
type ConfigInputResolutionView struct {
    Key      string `json:"key"`
    Required bool   `json:"required"`
    Secret   bool   `json:"secret"`
    Status   string `json:"status"` // provided|omitted|invalid
}
```

- Preview 返回 descriptor 与 resolution，但不回显输入值。
- required 未填或输入非法时，`issues` 返回对应 key/field 的 blocking issue。
- optional 留空返回 `status=omitted`，不是 warning。
- Web 本地保存非 secret/secret 输入用于返回上一步编辑；服务端响应不承担表单回填。
- Snapshot hash mismatch 时，Web 清空所有 secret 输入并要求重新选择版本；不能把旧输入自动提交给字段契约可能不同的新 Snapshot。

## 11. HTTP、CLI、Remote 与 MCP

### 11.1 HTTP

不新增 route，扩展现有请求/响应：

```text
POST /api/v1/project-templates/capture-preview
POST /api/v1/project-templates
POST /api/v1/project-templates/{templateRef}/snapshots/capture-preview
POST /api/v1/project-templates/{templateRef}/snapshots
GET  /api/v1/project-templates
GET  /api/v1/project-templates/{templateRef}
POST /api/v1/project-templates/{templateRef}/instantiate-preview
POST /api/v1/project-templates/{templateRef}/instantiate
```

- Capture 请求增加 `config_policies`。
- Template list/detail 与 Preview 返回安全 `config_inputs` descriptor。
- Instantiate 请求增加 `config_inputs`。
- OpenAPI runtime schema、required 字段、enum 和 example 同步更新。
- 任何响应都不得出现 prompt 提交值。

### 11.2 CLI 与 Remote

命令不新增：

```text
xuanchu project template list
xuanchu project template instantiate ... --input <path|->
```

list 的 `--json` 输出包含 `current_snapshot.config_inputs`。human 输出至少标出：

```text
创建时填写：项目飞书群(feishu.chat_id, 必填)、广告账户(ads.account_id, 选填)
```

实例化输入文件：

```json
{
  "description": "从发布流程模板创建",
  "config_inputs": {
    "feishu.chat_id": "oc_xxx",
    "ads.account_id": "act_123"
  },
  "assignee_replacements": {}
}
```

- 不增加 `--config key=value`，避免 shell escaping 和 secret 进入 history。
- `--input` 继续支持 stdin，并保留 1 MiB 上限。
- Remote typed DTO 与本地 CLI 使用完全相同的 `config_inputs`。
- human error 走 stderr，stdout 只放结果。

### 11.3 MCP

仍只保留：

```text
project_template_list
project_template_instantiate
```

- list tool schema/result 增加 `config_inputs` descriptor。
- instantiate tool input 增加 `config_inputs: object<string,string>`。
- 不新增 `project_template_config_form` 或同义 tool。
- tool description 明确：先 list 读取 current Snapshot 的字段要求，再收集输入并携带相同 snapshot ID/hash 调用 instantiate。
- MCP 结果、错误、`content[0].text` 和 `structuredContent` 都不得回显 secret 输入。
- Agent token 仍受 `config:write`、workspace/project allowlist 和模板 current-only 边界约束。

## 12. Web Console

Web 继续复用现有 Capture/Instantiate Sheet，遵循 [DESIGN.md](../../../DESIGN.md) 的“冷静的工程绿”：36px 控件、token 色、Lucide 图标、`rounded-lg` 面板，不新增大面积状态色。

### 12.1 保存模板：配置策略

配置候选被选中后，在“已选配置”中直接设置策略：

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ 选择模板内容 / 配置                                      已选 4            │
├────────────────────────────────────────────────────────────────────────────┤
│ 项目飞书群                                                               │
│ feishu.chat_id · string                                                   │
│ ( ) 使用当前项目值                                                       │
│ (●) 创建项目时填写                    [✓] 必填                            │
│                                                                            │
│ Agent API Key                                        自动化依赖 · 已锁定  │
│ agent.provider.api_key · secret                                            │
│ ( ) 使用当前项目值（加密复制）                                            │
│ (●) 创建项目时填写                    [✓] 必填（不可取消）                │
│                                                                            │
│ 广告账户                                                                  │
│ ads.account_id · string                                                    │
│ (●) 创建项目时填写                    [ ] 必填                            │
├────────────────────────────────────────────────────────────────────────────┤
│                                        [返回] [检查选择]                   │
└────────────────────────────────────────────────────────────────────────────┘
```

交互：

- 有源项目显式值的新选 config 默认 `fixed`，保持既有 Capture 行为；没有显式值的候选默认 `prompt`，且不展示不可用的 fixed 选项。
- 切到 prompt 后出现“必填”开关。
- automation 依赖 key 切到 prompt 时自动设为 required，并禁用取消。
- secret fixed 文案明确“加密复制”；secret prompt 不显示来源值。
- Preview 摘要按“固定配置 / 创建时填写（必填 N、选填 N）”区分，不只显示一个 config 总数。

### 12.2 从模板创建：项目信息与配置同页

配置输入是正常创建资料，不应伪装成 Preview 后才出现的“问题”。实例化步骤调整为：

```text
1 选择模板 → 2 项目与配置 → 3 处理问题 → 4 确认创建
```

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ 从模板创建项目                                                    02/04   │
│ 项目与配置                                                                 │
├────────────────────────────────────────────────────────────────────────────┤
│ 项目名称 *                    项目 Slug *                                  │
│ [ 直播增长项目            ]   [ live-growth ]                              │
│                                                                            │
│ 项目启动日 *                  项目描述                                     │
│ [ 2026-08-01 ]                [ ...                                    ]   │
│                                                                            │
│ 创建时填写配置                                                             │
│ 项目飞书群 *                                                               │
│ [ oc_xxxxxxxxxxxxxxxxx ]                                                   │
│ 填写当前项目使用的飞书群 chat_id                                           │
│                                                                            │
│ 投放地区 *                                                                 │
│ [ 中国大陆                                              ▾ ]                │
│                                                                            │
│ 回调参数（选填）                                                           │
│ [ {"source":"launch"}                                               ]   │
│                                                                            │
│ Agent API Key *                                                            │
│ [ ••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••• ]      │
│ 机密值仅用于本次创建，不会在预览中显示                                     │
├────────────────────────────────────────────────────────────────────────────┤
│                                     [返回] [生成预览]                       │
└────────────────────────────────────────────────────────────────────────────┘
```

控件映射：

| Definition | Web 控件 |
|---|---|
| `secret=true` | password Input，关闭自动完成，不提供 reveal；优先级高于 enum |
| 非 secret 且 enum 非空 | Select |
| `boolean` | Select（`true/false` 的明确文本），不使用会混淆“未填写”的二态 Switch |
| `number` | `input type=number`，提交仍为字符串 |
| `date` | 复用日期选择控件，输出 `YYYY-MM-DD` |
| `datetime` | 复用日期 + 时间控件，输出带时区的 RFC3339 |
| `json` | 等宽字体 Textarea，失焦或提交时做 JSON 语法提示，服务端最终裁决 |
| 其它 string | Input |

实现应优先复用并小步扩展现有 `web/src/features/workspace/config/config-value-control.tsx`，为模板 prompt 增加“允许空值”“secret 禁止 reveal”和“boolean 未选择态”能力，不复制另一套 number/date/datetime/json 解析控件。

规则：

- 必填项用 `*` 和 label 明确表达，不用大面积红色。
- 字段按 Snapshot 顺序展示；label 缺失时用等宽 key。
- optional 留空不提交 key。
- 前端可做即时格式提示，但“能否创建”只由服务端 Preview 决定。
- 进入第 3 步后，prompt 输入不再重复出现在“机密值来源”；第 3 步只处理失效成员、definition 漂移及其它 blocking issue。
- 返回第 2 步保留本次 Sheet 会话输入；关闭 Sheet、创建成功、hash drift 或请求失败后的安全重置必须清空 secret。
- 确认页只显示“已填写 N 项 / 跳过 N 项”，secret 只显示“已填写”，不显示值。

### 12.3 模板库详情

Snapshot 详情的 config 区展示：

```text
固定配置 2
创建时填写 3 · 必填 2 · 选填 1
```

逐项只展示 key、label、type、required 和 fixed/prompt。fixed secret 显示“加密保存”，不显示密文；fixed 非 secret 是否展示值继续沿用现有详情权限和安全策略，本功能不扩大 reveal。

### 12.4 移动端与可访问性

- 移动端 Sheet 全屏，字段单列，不出现横向滚动。
- label 与 input 使用稳定 `htmlFor/id`；错误提示通过 `aria-describedby` 关联。
- 第一个 blocking 字段在 Preview 返回后获得焦点。
- required 状态同时用文本和原生 `required/aria-required`，不只靠颜色。
- secret 不支持浏览器回退后的持久恢复。

## 13. App 与分层

建议复用现有文件边界：

- `internal/projecttemplate`：v2 struct、strict codec、升级、canonical hash、纯规则校验。
- `internal/app/project_template_capture.go`：解析 `config_policies`，从源 config 生成 fixed/prompt blueprint。
- `internal/app/project_template_instantiate.go`：生成 form descriptor、白名单校验、归一化、Preview issue 和事务计划。
- `internal/storage`：继续保存 opaque SnapshotJSON；不解释 prompt。
- HTTP/CLI/MCP/Web：只做 DTO 与展示，不复制业务校验。

建议在 App 内把当前 `planInstantiateConfigs` 拆成可测试的三个步骤：

```text
resolveConfigDefinitions
      ↓
planFixedAndPromptConfigs
      ↓
buildConfigInputDescriptors
```

不要在 Web 中根据 `value_type` 之外的信息推导 required，也不要在 HTTP handler 直接写 config row。

## 14. 事务、权限与审计

### 14.1 事务

保持现有顺序：

```text
固定 Snapshot ID/hash
  → 完整 Preview（含 prompt）
  → 创建 planning Project
  → 写 fixed + 已填写 prompt config
  → 创建 Series/Task/Automation
  → audit/event
  → COMMIT
```

required 缺失、输入非法、definition 漂移或未知 key 都发生在创建 Project 前。最终事务内仍重复校验可漂移条件。

### 14.2 权限

- Capture prompt 仍要求读取源 config 和管理模板所需权限。
- 只要 Snapshot 含任意 fixed 或 prompt config，实例化仍要求 `config:write` / `project_config.write`。
- optional prompt 即使本次留空也不降低权限要求；权限由 Snapshot 能力决定，不能由请求是否碰巧省略字段决定。
- workspace、project allowlist 和 current-only CLI/MCP 边界不变。

### 14.3 审计与事件

- Snapshot hash 已覆盖 prompt 规则，不另建 mutable audit 字段。
- `project_template.snapshot.create` payload 可增加 `config_fixed_count/config_prompt_required_count/config_prompt_optional_count`。
- `project_template.instantiate` 只记录提交了哪些 config key 和数量，不记录任何值。
- project config 既有审计不得因本功能开始记录 secret 或普通输入明文。
- 领域事件继续只标注 source template/snapshot 元数据。

## 15. 安全

- v2 prompt 永不保存来源值到 Snapshot。
- Preview/list/detail 不回显提交值；secret 与非 secret 都遵守该规则，减少日志和客户端缓存面。
- 服务端错误只包含 key、field、code 和安全消息。
- Web secret state 不进入 URL、React Query cache、localStorage、sessionStorage 或持久表单草稿。
- CLI 推荐 stdin；不新增 secret flag。
- MCP tool 参数可能进入调用方自己的观测系统，tool description 必须提醒 secret 风险；璇础自身日志仍不得输出 arguments 中的值。
- `config_inputs` map 大小不得超过 Snapshot prompt 数量和 `MaxConfigs`；单值沿用 config/schema 与请求体大小限制。
- JSON 输入继续使用服务端安全解析与 canonical normalization，不执行表达式或模板。

## 16. 错误码

新增：

| Code | 语义 |
|---|---|
| `project_template_config_policy_invalid` | Capture policy key/strategy/required 组合非法 |
| `project_template_config_input_required` | v2 required prompt 未显式提交非空值 |
| `project_template_config_input_invalid` | prompt 值不符合当前 type/enum/secret 规则 |
| `project_template_config_input_unknown` | 请求提交了 Snapshot 未声明的 prompt key或字段冲突 |

继续复用：

| Code | 场景 |
|---|---|
| `project_template_config_invalid` | definition 缺失、scope 漂移、fixed 值不兼容 |
| `project_template_snapshot_schema_unsupported` | 二进制不支持 v2 |
| `project_template_snapshot_invalid` | v2 JSON/字段组合非法 |
| `project_template_snapshot_hash_mismatch` | Snapshot 在表单期间发生切换 |
| `project_template_automation_invalid` | prompt 后的 effective config 仍不能满足 automation contract |
| `project_template_secret_required` | 仅保留 v1 legacy `secret_input` 兼容语义 |

HTTP 状态继续沿用现有 project-template error mapping；校验问题必须同时进入稳定 `issues[]`，Web 不解析英文 message 做逻辑判断。

## 17. 兼容与迁移

### 17.1 数据库

- 不新增表或列。
- 不重写历史 Snapshot。
- 旧数据库仍由现有迁移创建模板表；本功能只要求 codec 能读写 v2 TEXT。
- SQLite/PostgreSQL 都必须覆盖 v1/v2 混合数据、current 指向任意版本和历史版本实例化。

### 17.2 协议

- Capture 未传 `config_policies` 时全部按 fixed，避免旧 Web/API 客户端突然生成必填项。
- Instantiate 未传 `config_inputs` 时，v1 行为完全不变；v2 required prompt 返回稳定缺失错误。
- `secret_inputs` 暂不删除，只为 v1 legacy Snapshot 服务。
- list 新增字段是向后兼容扩展；现有 `required_secret_keys` 暂保留。
- 新 binary 写 v2；旧 binary 遇到 v2 必须明确返回 schema unsupported，不能宽松忽略 prompt 后继续创建错误项目。

### 17.3 降级

一旦 workspace 创建了 v2 Snapshot，旧版本服务不能实例化该版本。发布说明必须明确这一点。由于 Snapshot 是不可变的，回滚 binary 后仍可读取 Template 元数据，但 v2 current Snapshot 的实例化会被阻断；运维需要恢复新 binary 或显式把 current 切回受支持版本。当前产品没有“切换 current 到旧 Snapshot”的管理动作，实施计划必须决定是否接受仅恢复 binary 的回滚路径；本功能不顺带新增版本切换能力。

## 18. 测试策略

### 18.1 `internal/projecttemplate`

- v2 literal/secret_copy/prompt required/prompt optional round-trip。
- mode/value/ciphertext/prompt 非法组合全部拒绝。
- config key 重复拒绝。
- unknown field、trailing JSON、未知 schema 拒绝。
- v1 三种 mode 升级保持原语义。
- canonical hash 对 policy/required 变化敏感，对 map/输入顺序按规范稳定。
- v1 hash 不发生变化。

### 18.2 Storage 与数据库

- v1/v2 Snapshot 同表保存和读取。
- SQLite/PostgreSQL 混合版本 append/current/历史读取。
- 旧 SQLite fixture 经 `storage.Open` 迁移后，template schema、config definition 新列和外键检查通过。
- prompt 不增加任何独立 row；实例化后只出现实际填写的 optional config row。

### 18.3 App Capture

- policy 未传默认 fixed。
- 没有源 project row 的 definition 可以保存为 prompt，但不能保存为 fixed。
- 普通/secret fixed 保持现状。
- prompt 不把源值写入 Snapshot、view、audit。
- required/optional 正确进入 v2。
- 未选 key、重复 policy、fixed+required 拒绝。
- automation 依赖 prompt 强制 required；后端不能被伪造请求绕过。
- policy 变化导致 Snapshot hash 变化。

### 18.4 App Instantiate

- required prompt 缺失、空字符串、纯空白均阻断且无 Project row。
- required prompt 不被 workspace/default 满足。
- optional 留空不写 project row；填写后写 row。
- string/number/boolean/json/date/datetime/enum 全类型归一化。
- secret prompt 写入但 Preview/audit/error/JSON 不泄漏。
- unknown/fixed key 输入拒绝。
- `config_inputs` 与 `secret_inputs` 冲突拒绝。
- definition 删除、scope/type/enum/secret 漂移行为符合本规格。
- Preview 后 Snapshot hash 漂移，最终 Instantiate 阻断。
- 中途 task/series/automation 创建失败时 prompt config 与 Project 一起回滚。
- optional prompt 留空时仍按 Snapshot 要求检查 `config:write` 权限。

### 18.5 HTTP、CLI、Remote、MCP

- OpenAPI schema 包含 policy、descriptor、config_inputs 和稳定 enum。
- list/detail/preview 不出现任何输入值或 secret ciphertext。
- CLI list human/JSON 能发现必填/选填项。
- CLI stdin 输入贯通本地与 Remote。
- MCP list schema/result 暴露 descriptor，instantiate 接收 inputs。
- MCP ToolEnvelope 的 text/structuredContent 均不泄漏 secret。
- v1 调用契约继续通过。

### 18.6 Web

- Capture 默认 fixed，切换 prompt 后 required 可编辑。
- automation 依赖 prompt 的 required 不可取消。
- typed 控件按 definition 正确渲染。
- required 前端提示与服务端 issue 对齐。
- optional 空值不进请求 map。
- Preview 返回后不回填服务端值。
- 确认页不展示 secret。
- close/success/hash drift 清空 secret；返回上一步在同一会话保留。
- 390/768/1024/1440 无横向滚动，键盘和读屏 label/error 关联正确。

完整验证：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web test
pnpm --dir web run smoke:project-template
```

## 19. 验收标准

1. 模板作者能把选中的普通或 secret config 切为创建时填写，并设置必填/选填。
2. 新 Snapshot 使用 v2，规则进入 canonical hash，旧 v1 Snapshot 不变。
3. 必填项只能由本次 `config_inputs` 显式满足，继承/default 不能替代。
4. 选填项留空时不创建 project config row，填写时按当前 definition 校验并创建。
5. 从模板创建页在生成 Preview 前展示完整 typed form，不把正常输入伪装成错误修复。
6. automation 依赖的 prompt 一定必填，前后端都不能绕过。
7. HTTP、CLI、Remote、MCP 都能在实例化前发现字段要求并提交相同输入。
8. list/detail/preview/audit/log/error 不回显输入值，secret 边界不退化。
9. Snapshot hash 漂移或任一校验失败时不产生半成品 Project。
10. SQLite/PostgreSQL、零 CGO 测试和完整 Web 验证全部通过。

## 20. 实施切片建议

本节只定义依赖顺序，不代替 implementation plan：

1. `internal/projecttemplate` v2 codec、upgrade、validation、hash 测试。
2. Capture policy App 契约和 v2 Snapshot 生成。
3. Instantiate descriptor、config input planning、事务与安全测试。
4. HTTP/OpenAPI DTO。
5. CLI/Remote/MCP 收窄契约同步。
6. Web Capture 策略控件。
7. Web Instantiate typed form、Preview/secret lifecycle。
8. README、ROADMAP、Agent Skill、release/downgrade 说明同步。

实现计划必须按上述切片逐项列出 failing test、最小实现、重构点和验收命令。
