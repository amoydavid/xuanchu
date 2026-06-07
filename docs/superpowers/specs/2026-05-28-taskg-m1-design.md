# Xuanchu M1 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 在 M0 本地 CLI 的基础上，实现 Taskwarrior 风格的查询核心、内置报表、urgency 计算、DOM/helper 基础和 `calc` 第一版，让 `xuanchu` 从“能管理任务”升级为“能查询和解释任务”。

**范围：** M1 只基于 M0 已有任务字段实现可真实工作的能力。`start`、`wait`、`scheduled`、`depends`、`annotations`、`recurring`、UDA、多 workspace、HTTP API、MCP Server、op-log sync 和 Hook 均不进入 M1。

**需求来源：** 本规格从 [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 和 M0 当前实现中收束出第二个可独立交付的切片。

---

## 1. 当前基础

M0 已完成并合并到 `main`。当前项目已有：

- Go 1.22 module。
- Cobra CLI。
- GORM + `github.com/glebarez/sqlite`，保持零 CGO。
- 本地 SQLite 自动初始化。
- 隐式 `local` workspace。
- 核心命令：`add`、`list`、`next`、`info`、`modify`、`done`、`delete`、`import`、`export`、`show`、`config`。
- 简单 `Filter` 结构与 `ParseFilters`。
- 基础日期解析。
- 基础 `next` 排序：due、priority、entry。
- CLI 集成测试。

M1 的核心任务是替换“简单 Filter + 临时排序”的查询基础设施，同时保持 M0 CLI 行为不退化。

## 2. 产品边界

M1 应支持这些用户体验：

```bash
xuanchu '+next or due.before:tomorrow' list
xuanchu '(project:work and +urgent) or priority:H' list
xuanchu due.after:today due.before:eow list
xuanchu /spec/ all
xuanchu overdue
xuanchu completed
xuanchu _get 1.description 1.uuid
xuanchu _ids +next
xuanchu _projects
xuanchu calc '1 + 2 * 3'
xuanchu urgency 1
```

CLI 仍需兼容 M0 的“filter 出现在命令名前”形态，例如 `xuanchu +next list`；当查询包含空格、括号或布尔操作符时，用户应使用引号把查询表达式作为一个 shell 参数传入。
`/spec/` 在 M1 中只表示 description 子串匹配的简写，不承诺正则语义；完整正则匹配留到后续兼容阶段。

M1 的查询和报表能力必须仍然保持脚本友好：

- stdout 只输出结果。
- stderr 输出错误。
- `--json` 输出稳定结构。
- helper 命令输出无装饰文本。

### 范围内

- 查询 AST。
- 查询 parser。
- AST 到 GORM/SQL 的安全编译层。
- 当前字段可支持的内置报表。
- urgency 计算与 explain 数据结构。
- `next` 改为基于 urgency 排序。
- DOM/helper 第一版。
- `calc` 第一版。
- 对 M0 list/next/filter 行为的兼容。
- 单元测试、仓储测试、CLI 集成测试。

### 范围外

- `start` / `stop`。
- `wait`、`scheduled`、`until`。
- `annotations`。
- `dependencies`。
- `recurring`。
- UDA。
- context。
- `.taskrc` 导入。
- shell completion 增强。
- 多 workspace。
- HTTP API。
- MCP Server。
- Hook。
- op-log sync。

## 3. M1 支持字段

M1 查询、报表、DOM 和 urgency 只依赖这些 M0 已有字段：

- `uuid`
- 派生 working-set `id`，只在 CLI/app 目标解析阶段使用，不作为 SQL 查询属性直接编译
- `description`
- `status`
- `entry`
- `modified`
- `end`
- `due`
- `project`
- `priority`
- `tags`
- `workspace_id`

M1 不新增任务表核心字段。若 implementation plan 发现必须调整 schema，只能添加索引或非破坏性辅助列，并必须说明原因。

## 4. 查询语言

M1 将 `internal/query` 从简单字段结构升级为 AST。

### 支持语法

属性匹配：

```text
status:pending
status:completed
status:deleted
project:work
priority:H
uuid:<uuid>
```

标签：

```text
+urgent
-later
```

日期：

```text
due:today
due:tomorrow
due.before:tomorrow
due.after:2days
entry.after:2026-05-01
modified.before:eow
```

文本：

```text
/spec/
description:/spec/
```

M1 对 `/spec/`、`description:spec` 和 `description:/spec/` 的语义统一为“description 包含 `spec`”。`description:abc` **不**走字面相等（`description = 'abc'`），编译层始终翻译为 `description LIKE '%abc%'`，以避免与裸 token `/abc/` 的子串语义割裂。这里的斜杠只是 Taskwarrior 风格的子串语法糖，不是正则表达式；`/^Buy/` 在 M1 会按字面子串处理，不会匹配“以 Buy 开头”。

字符串：

```text
project:'Home & Garden'
description:'Write project spec'
```

布尔组合：

```text
+next or due.before:tomorrow
project:work and +urgent
not +later
(project:work and +urgent) or priority:H
```

多个相邻 filter 默认 AND：

```bash
xuanchu +work status:pending list
```

等价于：

```text
+work and status:pending
```

裸 token 不再由 parser 直接解释成 `uuid = token`。parser 应保留 `BareToken`，由 CLI/app 根据命令上下文判断：

- `xuanchu 1 list`、`xuanchu <uuid> list`：作为 target 解析。
- `xuanchu 1 done`、`xuanchu <uuid> modify ...`：沿用 M0 target action。
- 在纯查询位置中无法判定的裸 token：M1 转为 description 子串过滤，保持 M0 `/text/` 搜索体验；若后续发现这与 Taskwarrior 兼容性冲突，再单独调整。

M1 的 target 判定规则保持保守：只有单个纯整数 working-set ID、完整 36 字符 UUID、完整 32 字符无连字符 UUID 会被 `list`/`info` 等命令当成 target；其它裸 token 一律进入 query AST，作为 description 子串过滤。UUID 短前缀留到 M2 再评估。

### 语法优先级

从高到低：

1. 括号。
2. `not`。
3. `and` 或相邻 filter 隐式 AND。
4. `xor`。
5. `or`。

如果实现中发现 Taskwarrior 实际优先级与此处不同，优先保证本文档一致，并在后续兼容阶段调整。

### 错误处理

parser 应返回清晰错误：

- 未闭合括号。
- 未闭合引号。
- 未闭合 `/text/` 子串表达式。
- 未知属性。
- 未知日期修饰符。
- 布尔操作符缺少左右操作数。

错误示例：

```text
xuanchu: invalid query: expected expression after "or"
xuanchu: invalid query: unknown attribute "foo"
xuanchu: invalid query: unclosed quote
```

## 5. AST 与编译层

建议 AST 包含这些节点：

- `And`
- `Or`
- `Xor`
- `Not`
- `Predicate`

`Predicate` 应描述：

- attribute，例如 `project`、`due`、`description`。
- operator，例如 `eq`、`before`、`after`、`contains`、`has_tag`、`missing_tag`、`is_null`。
- value，保留原始值和解析后的 typed value；date value 必须 lazy，parser 只保存 `today`、`tomorrow`、`eow`、`2026-05-01` 等原始表达式，不在 parse 阶段调用真实 `time.Now()` 固化时间戳。

编译层职责：

- 将 AST 编译为 GORM query scope 或参数化 SQL 条件。
- 所有外部输入必须使用绑定参数。
- 不允许将用户输入直接拼进 SQL 字符串。
- tag 查询继续通过 `task_tags` 表完成。
- 默认自动注入当前 `workspace_id`。
- 在 `CompileOptions{Now, Location}` 中解析 lazy date value，确保长进程跨午夜后 `today`、`eow` 等相对日期仍使用执行时刻。
- `due:`、`entry:`、`modified:`、`end:` 的空值语义是 `IS NULL`；对应非空日期值才解析为时间戳。
- 日期等值谓词使用自然日范围，而不是精确秒匹配：`due:2030-01-01` 编译为 `[2030-01-01 00:00:00, 2030-01-01 23:59:59]`。
- 对 `due`、`end` 这类用户理解为“某天截止/结束”的日期值，**写入时也必须落在当地时区的当天 `23:59:59`**：CLI 入口（`add due:2030-01-01`、`modify due:tomorrow` 等）须经 `ResolveDeadlineDateValue`，不得直接用 `ParseDate` 写入 `00:00:00`，避免 export/格式化后漂移到第二天，也保证 `overdue` 等基于「`due.before:today`」的报表在跨午夜后行为稳定。
- `xor` 必须按布尔 xor 编译，SQL 中使用 `COALESCE((expr), 0) <> COALESCE((expr), 0)` 之类形式规避 `NULL` 三值逻辑。

建议新增边界：

- `internal/query/ast.go`
- `internal/query/parser.go`
- `internal/query/compiler.go`
- `internal/query/date.go`
- `internal/storage/query_scope.go`

`internal/app` 只接收结构化查询，不直接处理 SQL。

## 6. 报表系统

M1 增加内置 report 层，但不做用户自定义 report。

### M1 必须真实支持的报表

- `list`
  - 默认 filter：`status:pending`。
  - 默认 sort：`entry ASC`。
- `next`
  - 默认 filter：`status:pending`。
  - 默认 sort：`urgency DESC`，同分时 `due ASC NULLS LAST`、`priority DESC`、`entry ASC`。
- `all`
  - 默认 filter：无状态限制。
  - 默认 sort：`entry ASC`。
- `completed`
  - 默认 filter：`status:completed`。
  - 默认 sort：`end DESC`、`modified DESC`。
- `deleted`
  - 默认 filter：`status:deleted`。
  - 默认 sort：`end DESC`、`modified DESC`。
- `overdue`
  - 默认 filter：`status:pending and due.before:today`。
  - 默认 sort：`due ASC`。

### M1 不注册的报表

这些报表依赖 M2 字段，不在 M1 注册命令：

- `waiting`
- `active`
- `ready`
- `blocked`
- `blocking`

等 M2 补齐 `wait`、`start`、`scheduled`、`depends` 后再实现。M1 不做“占位空结果”，避免用户误以为这些能力已经可用。

### Report 结构

建议新增 `internal/report`：

- `Definition`
- `Registry`
- `Run`

每个 report 至少包含：

- name
- description
- default filter AST，或可在 registry 初始化时解析成 lazy date AST 的 filter source
- default sort
- columns

内置 registry 可以复用解析后的 AST，但不能保存已经求值的相对日期时间戳。`overdue` 的 `due.before:today` 必须在每次运行报表时按当前 clock 编译。

M1 的 columns 可以保持简单，human 输出继续使用现有 table renderer；JSON 输出返回完整任务 DTO，并可附加 `urgency` 字段。

## 7. Urgency 计算

M1 新增 `internal/urgency`，基于当前字段实现可解释 urgency。

### M1 参与项

| 项 | 默认系数 | M1 行为 |
|---|---:|---|
| `+next` | 15.0 | 任务含 `next` tag 时贡献 15.0 |
| due | 12.0 | 临近或超期任务贡献，越近越高 |
| priority H | 6.0 | priority 为 H 时贡献 6.0 |
| priority M | 3.9 | priority 为 M 时贡献 3.9 |
| priority L | 1.8 | priority 为 L 时贡献 1.8 |
| age | 2.0 | 根据 entry 到当前时间，最多按 365 天归一 |
| tags | 1.0 | 1 个 tag 为 0.8，2 个为 0.9，3 个及以上为 1.0 |
| project | 1.0 | project 非空时贡献 1.0 |

M1 不计算：

- blocking。
- blocked。
- scheduled。
- active。
- waiting。
- annotations。
- UDA。
- inherited urgency。

这些项必须在 explain 中显示为“not_applicable”或不返回，implementation plan 需要明确选择一种稳定格式。

### Explain 结构

`Explain(task)` 应返回：

- task uuid。
- total urgency。
- 每一项贡献：
  - name
  - coefficient
  - raw value
  - contribution
  - reason

示例 JSON：

```json
{
  "uuid": "…",
  "total": 22.8,
  "items": [
    {"name": "tag.next", "coefficient": 15.0, "contribution": 15.0, "reason": "task has +next"},
    {"name": "priority.H", "coefficient": 6.0, "contribution": 6.0, "reason": "priority is H"},
    {"name": "tags", "coefficient": 1.0, "contribution": 0.8, "reason": "one tag"}
  ]
}
```

### CLI

M1 增加：

```bash
xuanchu urgency 1
xuanchu urgency <uuid>
xuanchu _urgency 1
```

建议行为：

- `urgency`：human 输出 explain。
- `urgency --json`：输出 explain JSON。
- `_urgency`：只输出数字，适合脚本。

## 8. DOM 与 Helper

M1 新增 `internal/dom`，提供最小可复用 DOM 查询能力。

### `_get`

支持：

```bash
xuanchu _get 1.description
xuanchu _get 1.uuid
xuanchu _get 1.status
xuanchu _get 1.entry
xuanchu _get 1.modified
xuanchu _get 1.due
xuanchu _get 1.project
xuanchu _get 1.priority
xuanchu _get 1.tags
xuanchu _get 1.urgency
xuanchu _get 1.tag.next
```

规则：

- 数字 ID 按当前默认 working set 解析。
- UUID 可直接使用。
- 多表达式按输入顺序逐行输出。
- 不存在字段返回非零 exit code。
- 虚拟 tag：`1.tag.next` 满足时输出 `next`，不满足时输出空字符串并返回 0；只有真正未知的 DOM 字段才返回非零 exit code。
- `_get 1.urgency` 必须先计算该任务的 `urgency.Explain(...).Total`，再交给 DOM resolver。

### `_ids`

输出当前查询结果的 working-set ID：

```bash
xuanchu _ids +next
```

### `_uuids`

输出当前查询结果的 UUID：

```bash
xuanchu _uuids project:work
```

### `_projects`

输出去重 project 列表，按字母排序。

### `_tags`

输出去重 tag 列表，按字母排序。

### 输出约束

- helper 命令不输出表头。
- 每行一个值。
- 不使用颜色。
- 错误只写 stderr。

## 9. Calc 第一版

M1 增加 `internal/expr` 和 CLI：

```bash
xuanchu calc '1 + 2 * 3'
xuanchu calc '3 > 2 and 1 < 2'
```

支持：

- 整数。
- 浮点数。
- `+`、`-`、`*`、`/`。
- `%`。
- `^`。
- 比较：`==`、`!=`、`<`、`<=`、`>`、`>=`。
- 布尔：`and`、`or`、`xor`、`not`。
- 括号。

M1 不支持：

- 正则比较 `~`、`!~`。
- DOM 引用。
- 字符串运算。
- 日期运算。
- 函数调用。

这些留给后续兼容阶段。

## 10. CLI 路由调整

M0 root command 已经支持：

- `xuanchu <subcommand> ...`
- `xuanchu <filters...> <command> ...`
- `xuanchu <target> <action> ...`

M1 增加 `_get`、`_ids`、`_uuids`、`_projects`、`_tags`、`_urgency`、`calc`、`urgency`、`all`、`completed`、`deleted`、`overdue` 后，需要同步更新 root command 的 known subcommands。

要求：

- prefix filter 仍能工作。
- target action 仍能工作。
- dash tag 处理不退化。
- `--json` 仍可在命令前出现。
- root reorder 必须识别 M1 query token：括号表达式、`/.../`、`+tag`、`-tag`、`attr:value`、`attr.before:value`、`attr.after:value`、布尔关键字。它只负责把“filter 在前、命令在后”的 argv 调整为 Cobra 可执行形态，不负责理解查询语义。

## 11. 数据流

M1 查询数据流：

```text
CLI args
  -> query parser
  -> query AST
  -> report default filter merge
  -> app service
  -> storage compiler
  -> GORM query
  -> domain task list
  -> urgency/report/render
```

关键点：

- report 默认 filter 与用户 filter 通过 AND 合并。
- 若用户 filter 与 report 默认 filter 冲突，例如 `xuanchu completed status:pending`，M1 返回空结果；用户若要完全绕开默认状态限制，应使用 `all` 报表。
- `all` 不注入默认 status。
- `list` 和 `next` 注入 `status:pending`。
- `overdue` 注入 `status:pending and due.before:today`。
- urgency 需要可注入 clock，测试不能依赖真实时间。

## 12. 错误处理

M1 错误继续遵守 M0 约定：

- human 模式：stderr 短错误，非零 exit code。
- JSON 模式：stderr 结构化错误对象，如果 implementation plan 决定补齐该能力。
- stdout 只输出命令结果。

新增错误类型：

- query parse error。
- unknown report。
- unknown DOM field。
- calc parse error。
- calc evaluation error。

错误示例：

```text
xuanchu: invalid query: unclosed /text/ expression
xuanchu: unknown report "waiting"; this report requires M2 task fields
xuanchu: unknown DOM field "1.foo"
xuanchu: calc: division by zero
```

虽然 M1 不注册 `waiting` 命令，但如果用户显式请求未知报表或未来配置残留，应给清晰错误。

## 13. 测试策略

### Query 单元测试

- tokenization。
- UTF-8 空白和中文输入。
- 引号字符串。
- `/text/` 子串语法。
- `not (a or b)`、`not +work`。
- 空查询、纯空白查询。
- 隐式 AND。
- 布尔优先级。
- 括号。
- 不平衡括号。
- 日期修饰符。
- lazy date value 不在 parse 阶段固化。
- 裸 token 产出 `BareToken` 或按 CLI/app 规则转换。
- parser 错误。

### Query 编译测试

- status/project/priority/tag/text。
- due.before/due.after。
- OR 组合。
- NOT 组合。
- XOR 真值表与 NULL 语义。
- `due:` 空值编译为 `IS NULL`。
- workspace_id 自动注入。
- 绑定参数防注入。

### Report 测试

- `list` 只显示 pending。
- `all` 显示 pending/completed/deleted。
- `completed` 按 end/modified 排序。
- `overdue` 只显示 due 在 today 前的 pending。
- `overdue` 跨不同 clock 运行时能重新计算 today。
- `next` 按 urgency 排序。
- report 默认 filter 与用户 filter 冲突时返回空结果。

### Urgency 测试

- `+next` 贡献。
- priority H/M/L 贡献。
- due/overdue 贡献。
- due 距当前 0/3/7/30 天的贡献边界。
- age 归一。
- tags 数量修正规则。
- 多 tag。
- explain total 等于各项 contribution 之和。

### DOM/helper 测试

- `_get 1.description`。
- `_get 1.urgency`。
- `_ids +tag`。
- `_uuids project:work`。
- `_projects` 去重排序。
- `_projects` 空任务时输出空且成功。
- `_tags` 去重排序。
- `_tags` 空任务时输出空且成功。
- `_get 1.tag.missing` 输出空且成功。
- unknown field 失败。

### Calc 测试

- 算术优先级。
- 括号。
- 比较。
- 布尔。
- 除零错误。
- parse error。

### CLI 集成测试

- `xuanchu '+next or due.before:tomorrow' list`。
- `xuanchu '(project:work and +urgent) or priority:H' list`。
- `xuanchu overdue`。
- `xuanchu completed --json`。
- `xuanchu _get 1.description 1.uuid`。
- `xuanchu calc '1 + 2 * 3'`。
- `xuanchu urgency 1 --json`。

### 必跑验收

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 14. 验收标准

M1 完成时应满足：

- M0 全部测试继续通过。
- 查询 parser 支持本文档列出的 M1 语法。
- AST 编译层没有用户输入直拼 SQL。
- `list`、`next`、`all`、`completed`、`deleted`、`overdue` 可用。
- `waiting`、`active`、`ready`、`blocked`、`blocking` 不作为 M1 命令交付。
- `next` 使用 urgency 排序。
- `urgency` 和 `_urgency` 可用。
- `_get`、`_ids`、`_uuids`、`_projects`、`_tags` 可用。
- `calc` 支持 M1 表达式范围。
- human 输出和 `--json` 输出不退化。
- README 更新 M1 用法。
- ROADMAP 将 M1 标记为已完成，并更新下一步。

## 15. 后续衔接

M1 完成后，M2 应补齐任务模型字段：

- start/stop。
- wait/scheduled/until。
- annotations。
- dependencies。
- recurring。

届时 M2 应启用：

- `waiting`。
- `active`。
- `ready`。
- `blocked`。
- `blocking`。
- urgency 中的 active、waiting、blocked、blocking、annotations。

M1 的 AST、report、urgency explain、DOM 和 calc 都应作为 M2 的基础设施复用，不应重写。

## 16. 审阅说明

本文档根据当前 M0 实现、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md) 和 [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 编写。当前没有执行 subagent review；如后续需要严格执行 superpowers 审阅环节，请在进入 implementation plan 前补充审阅。

2026-05-28 评审修订：本文档已吸收 M1 implementation plan review 中的阻塞反馈，明确 lazy 日期、`/text/` 子串语义、裸 token 职责边界、DOM 虚拟 tag 返回规则、root reorder 覆盖范围和关键测试用例。
