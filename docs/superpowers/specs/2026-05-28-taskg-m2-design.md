# xuanchu M2 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 在 M1 查询、报表、urgency、DOM/helper 基础上，补齐 Taskwarrior 日常核心任务模型：状态字段、起停、等待/计划、注释、依赖、描述编辑、循环任务，并让这些能力贯通查询、报表、urgency、DOM、JSON import/export。

**范围策略：** M2 是一个“大里程碑”，但必须分层交付和验收。实现计划应拆为 Core、Editing、Recurrence 三个层次，每个层次都能独立测试，避免一次性改动不可控。

**需求来源：** 本规格从 [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[README.md](/Users/mac/code/projects/dajee/task/README.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 和 M1 当前实现中收束。

---

## 1. 当前基础

M1 已完成并合并到 `main`。当前项目已有：

- Go 1.22 module。
- Cobra CLI。
- GORM + `github.com/glebarez/sqlite`，保持零 CGO。
- 隐式 `local` workspace。
- 核心任务生命周期：`add`、`list`、`next`、`info`、`modify`、`done`、`delete`。
- 查询 AST、UTF-8 tokenizer、lazy date value、AST 到 SQL 编译。
- 内置报表：`list`、`next`、`all`、`completed`、`deleted`、`overdue`。
- urgency explain 基础。
- DOM/helper：`_get`、`_ids`、`_uuids`、`_projects`、`_tags`、`_urgency`。
- `calc` 第一版。
- JSON import/export 覆盖 M0/M1 字段。

M2 的主要工作不是重写 M1 基础设施，而是在其上扩展字段、命令、报表和派生语义。

## 2. M2 产品体验

M2 完成后，应支持这些用户体验：

```bash
# 起停与 active
xuanchu 1 start
xuanchu active
xuanchu 1 stop

# 等待、计划和 ready/waiting
xuanchu add "Call vendor" wait:tomorrow
xuanchu waiting
xuanchu ready
xuanchu 1 modify wait:

# until 自动过期
xuanchu add "temporary reminder" until:eow
xuanchu all

# 注释
xuanchu 1 annotate "called, left voicemail"
xuanchu _get 1.annotations
xuanchu 1 denotate 1

# 依赖与 blocked/blocking
xuanchu add "Prepare API" project:xuanchu
xuanchu add "Write docs" depends:<uuid-or-id>
xuanchu blocked
xuanchu blocking

# 描述编辑
xuanchu 1 append "with examples"
xuanchu 1 prepend "[draft]"
xuanchu 1 edit

# 循环任务
xuanchu add "Submit weekly report" recur:weekly due:friday until:2030-12-31
xuanchu next
xuanchu 1 done
xuanchu next
```

M2 仍需保持脚本友好：

- stdout 只输出结果。
- stderr 输出错误。
- helper 命令无装饰、每行一个值。
- `--json` 输出稳定结构。
- 所有新增字段必须能 import/export 往返。

## 3. 分层范围

### Layer 1：Core 状态模型

必须进入 M2：

- 新增任务字段：
  - `start`
  - `wait`
  - `scheduled`
  - `until`
  - `annotations`
  - `depends`
- 新增命令：
  - `start`
  - `stop`
  - `annotate`
  - `denotate`
- 新增报表：
  - `waiting`
  - `active`
  - `ready`
  - `blocked`
  - `blocking`
- 查询语言支持新增字段：
  - `start`
  - `wait`
  - `scheduled`
  - `until`
  - `depends`
  - `annotations`
- urgency 增加 M2 可计算项：
  - active。
  - waiting。
  - blocked。
  - blocking。
  - annotations。
- DOM/helper 支持新增字段。
- JSON import/export 覆盖新增字段。

### Layer 2：Editing 描述编辑

必须进入 M2：

- `append`
- `prepend`
- `edit` 基础版

`edit` 只做本地 CLI 基础能力：打开 `$EDITOR` 编辑一个稳定格式的临时文件，保存后解析、校验并写回。M2 不实现图形交互、不实现远程编辑会话、不实现复杂合并冲突。

### Layer 3：Recurrence 循环任务

必须进入 M2：

- 新增 recurring 字段：
  - `recur`
  - `parent`
  - `mask`
  - `imask`
- 支持周期：
  - `daily`
  - `weekly`
  - `monthly`
  - `<N>days`
  - `<N>weeks`
  - `<N>months`
- 父任务隐藏。
- 子任务可见。
- 完成子任务时生成下一个子任务。
- `until` 控制循环终止。
- JSON import/export 覆盖 recurring 字段。

## 4. 非目标

M2 不做：

- UDA。
- context。
- `.taskrc` 导入。
- shell completion 增强。
- 多 workspace。
- 多用户。
- HTTP API。
- MCP Server。
- Hook。
- op-log sync。
- full Taskwarrior calendar / burndown。

这些留给 M3+。

## 5. 数据模型

### Task 新增字段

M2 扩展 `internal/task.Task`：

```go
Start       *int64
Wait        *int64
Scheduled   *int64
Until       *int64
Annotations []Annotation
Depends     []string
Recur       *string
Parent      *string
Mask        *string
IMask       *int
```

建议新增：

```go
type Annotation struct {
	Entry       int64
	Description string
}
```

### Status

M2 扩展状态：

- `pending`
- `completed`
- `deleted`
- `waiting`
- `recurring`

建议状态使用规则：

- 普通可见任务默认 `pending`。
- 设置 `wait` 到未来时，任务状态可置为 `waiting`，并从默认 `list/next` 隐藏。
- `wait` 到期后，查询/list 前由 app 层或 repository 层进行 lightweight refresh：清空 `wait`，状态改回 `pending`。
- recurring 父任务状态为 `recurring`，默认报表隐藏。
- completed/deleted 继续沿用 `end`。

### SQLite schema

建议：

- `tasks` 表新增 nullable 列：
  - `start`
  - `wait`
  - `scheduled`
  - `until`
  - `recur`
  - `parent`
  - `mask`
  - `imask`
- 新增 `task_annotations` 表：
  - `task_uuid`
  - `entry`
  - `description`
  - primary key 建议 `(task_uuid, entry, description)` 或增加自增序号。
- 新增 `task_dependencies` 表：
  - `task_uuid`
  - `depends_on`
  - primary key `(task_uuid, depends_on)`。

所有新增表和查询必须通过 workspace 过滤，避免跨 workspace 泄漏。M2 仍然只有隐式 local workspace，但边界要提前正确。

## 6. 日期与自动状态

M2 复用 M1 lazy date parser。

### wait

- `wait:<date>`：任务在该时间前隐藏于默认 `list`、`next`、`ready`。
- `wait:`：清空 wait，并将 waiting 任务恢复为 pending。
- `waiting` 报表显示 wait 在未来的任务。
- 查询或报表执行前，应刷新当前 workspace 中到期 waiting 任务。

### scheduled

- `scheduled:<date>`：任务在该时间前不进入 `ready`。
- `scheduled:`：清空 scheduled。
- `ready` 报表显示：
  - status 为 pending。
  - 非 waiting。
  - scheduled 为空或已到期。
  - 不 blocked。

### until

- `until:<date>`：到期后任务应从普通报表隐藏。
- M2 建议不物理删除，而是将任务标记为 deleted 并设置 `end`，或在查询默认 filter 中排除 until 到期任务。implementation plan 必须选择一种并补测试。
- recurring 父/子任务都应尊重 until。

## 7. Dependencies

### 修改语法

M2 支持：

```bash
xuanchu 2 modify depends:1
xuanchu 2 modify depends:<uuid>
xuanchu 2 modify depends:
```

规则：

- `depends:<target>` 添加依赖。
- 多个 depends 可重复传入。
- `depends:` 清空依赖。
- target 可为 working-set ID 或 UUID。
- 禁止任务依赖自己。
- 禁止明显循环依赖。M2 至少要检测新增依赖会直接或间接形成环。

### blocked/blocking

- blocked：任务有未完成、未删除的依赖。
- blocking：任务被其他 pending/waiting/active 任务依赖。
- completed/deleted 任务不作为 blocker。
- recurring 父任务不参与 blocked/blocking。

### urgency

- blocked 贡献负分。
- blocking 贡献正分。
- 如果 dependency graph 支持，M2 可计算 inherited urgency；若实现复杂，M2 可以只在 explain 中标记 inherited 为 not_applicable，但必须预留结构。

## 8. Annotations

### 命令

```bash
xuanchu 1 annotate "called customer"
xuanchu 1 denotate 1
```

规则：

- annotate 添加 `{entry: now, description}`。
- description 不能为空，不允许换行。
- denotate 参数为 1-based annotation index，按 entry 升序显示。
- 找不到 annotation 返回非零 exit code。

### DOM/JSON

- `_get 1.annotations` 输出稳定文本格式；建议每行或 JSON 字符串由 implementation plan 决定。
- JSON import/export 使用 Taskwarrior 兼容字段：

```json
"annotations": [
  {"entry": "2026-05-28T10:00:00Z", "description": "called customer"}
]
```

### urgency

- annotations 贡献按数量修正规则：
  - 1 条：0.8
  - 2 条：0.9
  - 3 条及以上：1.0

## 9. Start / Stop / Active

### 命令

```bash
xuanchu 1 start
xuanchu 1 stop
```

规则：

- start：设置 `start = now`，状态保持 pending，清空 wait。
- 已 start 的任务再次 start 应幂等或返回清晰错误；implementation plan 必须选择一种并测试。
- stop：清空 `start`。
- completed/deleted/recurring 父任务不能 start。
- active 报表显示 `start != nil` 且 status 为 pending 的任务。
- urgency active 项贡献正分。

## 10. Append / Prepend / Edit

### append/prepend

```bash
xuanchu 1 append "with examples"
xuanchu 1 prepend "[draft]"
```

规则：

- append 在 description 后追加空格和文本。
- prepend 在 description 前追加文本和空格。
- 输入不能为空。
- 修改后更新 `modified`。

### edit

M2 `edit` 是基础版：

```bash
xuanchu 1 edit
```

规则：

- 使用 `$EDITOR`，若为空则使用 `vi`。
- 写入临时文件，格式建议 TOML 或 JSON；implementation plan 应选择一种。
- 保存退出后解析并校验。
- 允许修改 M2 已支持字段。
- 不允许修改 `uuid`、`workspace_id`、`entry`。
- 解析失败或校验失败时不写回，并返回非零 exit code。
- 如果编辑器退出码非 0，不写回。

M2 不要求支持交互式冲突合并。

## 11. Recurrence

循环任务是 M2 最复杂部分，必须单独建包或清晰拆分，建议新增 `internal/recurrence`。

### 字段语义

- parent task：
  - `status = recurring`
  - `recur != nil`
  - 普通报表隐藏。
- child task：
  - `parent = <parent uuid>`
  - 继承 parent 的 description/project/priority/tags/recur/until。
  - 有自己的 uuid、entry、modified、due/scheduled/wait/start/end/status。
- `mask` / `imask`：
  - M2 可以保留字段并导入导出。
  - 若完整 Taskwarrior mask 语义太重，M2 只需保证生成不重复，并在 spec/README 说明 mask 兼容为基础版。

### 支持周期

- `daily`
- `weekly`
- `monthly`
- `<N>days`
- `<N>weeks`
- `<N>months`

不支持：

- yearly。
- weekdays 列表。
- complex calendar expression。
- business day。

### 生成时机

M2 采用按需生成：

- 添加 recurring 任务时，创建 parent，并立即创建第一个 child。
- 完成 child 时，如果 parent 未到 until，创建下一个 child。
- 查询报表前可以执行 lightweight ensure：如果某 recurring parent 没有可见 pending child，则生成下一个。

implementation plan 必须精确定义“下一个 due/scheduled/wait”的计算规则。

### 终止

- `until` 到期后不再生成新 child。
- parent 可 delete，删除 parent 后不再生成 child；已有 child 的处理由 implementation plan 明确。推荐保留已有 child，不级联删除。

## 12. Query / Report 扩展

### Query 属性

M2 查询语言新增：

```text
start:today
start.before:tomorrow
wait:
wait.after:today
scheduled.before:eow
until.after:today
depends:<uuid>
recur:weekly
parent:<uuid>
```

`field:` 空值继续表示 `IS NULL`。

### 新增报表

- waiting：
  - `status:waiting or wait.after:now`，具体语义由实现统一。
  - sort：`wait ASC`。
- active：
  - `status:pending start.notnull` 或等价 predicate。
  - sort：`start DESC`。
- ready：
  - pending、非 waiting、scheduled 到期或为空、非 blocked。
  - sort：urgency DESC。
- blocked：
  - 有未完成依赖。
  - sort：urgency DESC。
- blocking：
  - 被未完成任务依赖。
  - sort：urgency DESC。

若 M1 query AST 尚无 `notnull` operator，M2 可以使用专用 report scope 或新增 operator。implementation plan 需要明确选择，不要临时拼 SQL。

## 13. DOM / Helper 扩展

`_get` 支持：

```bash
xuanchu _get 1.start
xuanchu _get 1.wait
xuanchu _get 1.scheduled
xuanchu _get 1.until
xuanchu _get 1.annotations
xuanchu _get 1.depends
xuanchu _get 1.recur
xuanchu _get 1.parent
```

helper 行为：

- 空字段输出空字符串并成功。
- unknown DOM field 仍返回非零 exit code。
- `depends` 输出 UUID 列表，建议逗号分隔。
- `annotations` 输出稳定格式；implementation plan 必须选定并测试。

## 14. JSON Import / Export

M2 JSON DTO 必须覆盖：

- `start`
- `wait`
- `scheduled`
- `until`
- `annotations`
- `depends`
- `recur`
- `parent`
- `mask`
- `imask`

要求：

- export 后 import 到空库，字段不丢失。
- import M0/M1 JSON 仍兼容。
- unknown future fields 暂不要求保留；UDA 留到 M3。
- Taskwarrior 兼容字段名优先，不发明不必要的新字段名。

## 15. 数据流

M2 写操作数据流：

```text
CLI args
  -> query/modify parser
  -> app service
  -> domain validation
  -> storage transaction
  -> dependent state refresh / recurrence generation
  -> output render
```

M2 查询数据流：

```text
CLI args
  -> refresh due waiting/until/recurrence
  -> query AST / report definition
  -> storage query
  -> app-level derived filters if needed
  -> urgency/report/render
```

关键点：

- 自动状态刷新必须可测试，依赖可注入 clock。
- recurrence 生成必须在事务中避免重复 child。
- dependencies 和 annotations 更新必须与 task 更新同事务。
- 不允许用户输入直拼 SQL。

## 16. 错误处理

新增错误示例：

```text
xuanchu: task 1 is already active
xuanchu: cannot start completed task
xuanchu: invalid dependency: task cannot depend on itself
xuanchu: invalid dependency: cycle detected
xuanchu: annotation 3 not found
xuanchu: invalid recurrence "fortnightly"
xuanchu: edit aborted: invalid TOML
xuanchu: editor exited with status 1
```

错误继续遵守：

- stdout 只输出结果。
- stderr 输出错误。
- 非零 exit code 表示失败。
- JSON 模式不把错误混进 stdout。

## 17. 测试策略

### Domain 单元测试

- Validate 支持新增 status。
- start/stop 状态规则。
- wait/scheduled/until 状态判断。
- annotation 排序和删除。
- dependency cycle 检测。
- recurrence 下一个时间计算。

### Query/Compiler 测试

- 新字段 eq/before/after/null。
- depends 查询。
- parent/recur 查询。
- waiting/active/ready/blocked/blocking 报表 filter。
- workspace 隔离。

### Storage 测试

- 新字段 create/update/list。
- annotations transaction。
- dependencies transaction。
- recurring parent/child create。
- import/export roundtrip。

### CLI 集成测试

- `start` / `stop` / `active`。
- `wait` / `waiting` / 到期恢复。
- `scheduled` / `ready`。
- `until` 到期隐藏或删除。
- `annotate` / `denotate` / `_get annotations`。
- `depends` / `blocked` / `blocking`。
- `append` / `prepend`。
- `edit` 使用测试 editor 脚本。
- recurring daily/weekly/monthly/`3days`。

### Urgency 测试

- active 贡献。
- waiting 负贡献。
- blocked 负贡献。
- blocking 正贡献。
- annotations 数量贡献。
- explain total 等于各项之和。

### 必跑验收

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 18. 验收标准

M2 完成时应满足：

- M0/M1 全部测试继续通过。
- 新增字段可写入、查询、显示、导入导出。
- `start/stop`、`annotate/denotate`、`append/prepend/edit` 可用。
- `waiting`、`active`、`ready`、`blocked`、`blocking` 报表可用。
- dependencies 支持 blocked/blocking 和 cycle 防护。
- recurring 支持 M2 周期并按需生成 child。
- urgency explain 包含 M2 新增贡献项。
- helper/DOM 支持 M2 字段。
- README 更新 M2 用法。
- ROADMAP 将 M2 标记为已完成，并更新下一步。

## 19. 后续衔接

M2 完成后，M3 应聚焦：

- 配置系统升级。
- context。
- UDA。
- `.taskrc` 导入。
- helper 补齐。
- shell completion。

M2 的字段、report、DOM、recurrence 和 JSON 结构应作为 M3+ 的基础设施复用，不应重写。

## 20. 审阅说明

本文档根据当前 M1 实现、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[README.md](/Users/mac/code/projects/dajee/task/README.md)、[AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md) 和 [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 编写。

当前环境未暴露 spec-document-reviewer subagent 工具；本文档先作为人工审阅草案提交给用户确认。后续写 implementation plan 前，如环境可用，应补充 spec review loop。
