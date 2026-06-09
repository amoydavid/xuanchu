# Xuanchu 定时通知规则过滤扩展实现计划

> **给 agentic workers 的要求：** 执行本计划时必须使用 `superpowers:subagent-driven-development`（如果当前环境支持子代理）或 `superpowers:executing-plans`。步骤使用 checkbox（`- [ ]`）语法跟踪进度。

**目标：** 让 Xuanchu 的定时通知支持每天固定时刻执行的规则，并通过可组合的 task filter 精确表达“8:50 预警”和“9:00 逾期通知（带第 N 次计数）”。

**架构：** 在现有 M16 定时通知基础上，保留 sink / delivery / dispatcher 不变，新增 rule 的 schedule + filter 表达。过滤条件直接复用 `internal/query` 的 AST 和 SQL 编译层，并扩展日期表达式以支持 `now +/- duration` 这类相对时间。scheduler 负责按墙上时间窗口评估规则、计算重复次数、冻结 delivery 快照，CLI/HTTP/MCP 只负责暴露新字段。

**技术栈：** Go 1.25、Cobra、GORM、`github.com/glebarez/sqlite`、PostgreSQL、`internal/query` AST、现有 notification runtime。

---

## 阶段 1: 扩展 query/date 语义，支持相对时间表达式

**文件：**
- 修改：`internal/query/date.go`
- 修改：`internal/query/parser_ast.go`
- 修改：`internal/query/parser.go`
- 修改：`internal/query/date_test.go`
- 修改：`internal/query/parser_ast_test.go`
- 修改：`internal/storage/query_scope.go`
- 修改：`internal/storage/query_scope_test.go`
- 修改：`docs/manual/query-reports.md`

- [ ] **步骤 1: 写失败测试**

补测试覆盖这些表达式：

```go
expr, err := query.ParseQuery(`due.before:now+24h`)
```

```go
expr, err := query.ParseQuery(`due.after:now-2h`)
```

```go
expr, err := query.ParseQuery(`due.before:now+2h30m`)
```

```go
expr, err := query.ParseQuery(`(start.isnull or start.notnull) and due.after:now`)
```

```go
expr, err := query.ParseQuery(`end.isnull and due.before:now+24h`)
```

要求：
- `now+24h` 被解析为相对当前时间的可编译值
- `now-2h` 也应可用，方便后续扩展
- `now+2h30m` 这类复合 duration 应可用
- duration 直接使用 Go `time.ParseDuration` 语法
- duration 支持 `ns`、`us`、`µs`、`μs`、`ms`、`s`、`m`、`h`；`24h`、`90m`、`2h30m` 合法
- `1d`、`90min` 不合法，应分别写成 `24h`、`90m`
- 不接受裸数字 duration，也不接受未知单位
- query/filter 的规范写法是无空格形式，例如 `due.before:now+24h`；本次不要求支持 `due.before:now + 24h`
- SQL 编译层在 SQLite / PostgreSQL 下都能生成正确范围条件

- [ ] **步骤 2: 跑测试确认失败**

运行：
```bash
go test ./internal/query ./internal/storage -run 'Test.*Date|Test.*Query|TestCompileQuery' -count=1
```

预期：失败，原因是相对时间或 duration 表达式尚未支持。

- [ ] **步骤 3: 写最小实现**

实现相对时间解析，优先复用现有 `ParseDate` / `ResolveDateValue` 入口，不新增第二套路由。

建议：
- 让 `now+24h`、`now-2h`、`now+2h30m` 进入 date value 解析
- 把 `now` 当成当前 clock 基准
- 只支持 `now +/- duration`，不支持任意算式
- 使用 `time.ParseDuration` 解析 duration，不做 Xuanchu 自定义单位扩展
- 不用 reminder rule 专属字段绕过 query/date 语法

- [ ] **步骤 4: 跑测试确认通过**

运行：
```bash
go test ./internal/query ./internal/storage -run 'Test.*Date|Test.*Query|TestCompileQuery' -count=1
```

预期：通过。

- [ ] **步骤 5: 更新查询文档**

在 `docs/manual/query-reports.md` 补一小段相对时间示例，明确 `now+24h` / `now-2h` / `now+2h30m` 是支持的，并说明 duration 使用 Go `time.ParseDuration` 语法，不支持 `1d` / `90min`。

---

## 阶段 2: 把 reminder rule 从固定 trigger 升级成 schedule + filter

**文件：**
- 修改：`internal/storage/models.go`
- 修改：`internal/storage/migrate_sqlite.go`
- 修改：`internal/storage/migrate_postgres.go`
- 修改：`internal/app/notification.go`
- 修改：`internal/app/notification_scheduler.go`
- 修改：`internal/app/notification_test.go`
- 修改：`internal/app/notification_scheduler_test.go`
- 修改：`internal/storage/reminder_rule_repo.go`
- 修改：`internal/storage/reminder_rule_repo_test.go`

- [ ] **步骤 1: 写失败测试**

补规则创建和调度测试，覆盖：
- `daily@08:50` 规则
- `daily@09:00` 规则
- filter 里能组合 `start is null`、`start is not null`、`end is null`、`due.before:now`、`due.before:now+24h`
- filter 支持 status/状态类条件 OR 组合，例如 `(start.isnull or start.notnull) and end.isnull`
- 逾期通知能输出 sequence / overdue_sequence

测试要明确验证：
- 规则存储里有 schedule 字段
- scheduler 不再只依赖 `trigger_type`
- 同一 task / recipient 在同一日不会重复发送

- [ ] **步骤 2: 跑测试确认失败**

运行：
```bash
go test ./internal/app ./internal/storage -run 'Test.*Notification|Test.*Reminder|Test.*Scheduler' -count=1
```

预期：失败，原因是 schedule/filter 字段或 sequence 字段尚不存在。

- [ ] **步骤 3: 写最小实现**

在存储层新增规则字段，保留旧字段兼容：
- `schedule_type`
- `schedule_value`
- `filter_source`
- `repeat_policy`

在 app 层：
- 规则创建/修改优先写新字段
- 旧 `trigger/offset/after` 只作为兼容层保留
- scheduler 读取 rule 后按 `schedule_type` 选择执行时刻
- 规则命中时，按 filter 选 task，再按 audience 解析 recipient
- delivery 写入提醒序号到 payload 或独立列

- [ ] **步骤 4: 跑测试确认通过**

运行：
```bash
go test ./internal/app ./internal/storage -run 'Test.*Notification|Test.*Reminder|Test.*Scheduler' -count=1
```

预期：通过。

- [ ] **步骤 5: 更新模型迁移说明**

在相关迁移测试里确认 SQLite / PostgreSQL 都能启动并读写新字段。

---

## 阶段 3: 扩展通知模板上下文，暴露提醒序号与窗口

**文件：**
- 修改：`internal/app/notification_endpoint.go`
- 修改：`internal/app/notification_scheduler.go`
- 修改：`internal/app/notification.go`
- 修改：`internal/app/notification_endpoint_test.go`
- 修改：`internal/app/notification_scheduler_test.go`
- 修改：`internal/notificationruntime/dispatcher.go`（如需消费冻结字段）

- [ ] **步骤 1: 写失败测试**

补测试覆盖模板变量：
- `reminder.sequence`
- `reminder.overdue_sequence`
- `reminder.window_start`
- `reminder.window_end`

验证：
- 逾期规则第二次发送时，模板能看到 sequence = 2
- 8:50 预警规则不会误用 overdue sequence
- delivery 仍冻结最终结果，不在 replay 时重新渲染

- [ ] **步骤 2: 跑测试确认失败**

运行：
```bash
go test ./internal/app ./internal/notificationruntime -run 'Test.*Notification|Test.*Dispatcher|Test.*Template' -count=1
```

预期：失败，原因是模板变量缺失或 sequence 不匹配。

- [ ] **步骤 3: 写最小实现**

把 scheduler 的派生上下文补到 resolver 输入里：
- schedule 窗口起止
- rule-level sequence
- overdue sequence

模板解析仍沿用现有受控替换器，不引入新模板引擎。

- [ ] **步骤 4: 跑测试确认通过**

运行：
```bash
go test ./internal/app ./internal/notificationruntime -run 'Test.*Notification|Test.*Dispatcher|Test.*Template' -count=1
```

预期：通过。

---

## 阶段 4: 暴露新规则到 CLI / HTTP / MCP / 文档

**文件：**
- 修改：`internal/cli/notification.go`
- 修改：`internal/httpapi/notifications.go`
- 修改：`internal/httpapi/router.go`
- 修改：`internal/mcpserver/tools_notification.go`
- 修改：`internal/mcpserver/integration_test.go`
- 修改：`internal/mcpserver/testdata/list-tools-default.json`
- 修改：`internal/mcpserver/testdata/*.schema.json`
- 修改：`docs/manual/notifications.md`
- 修改：`docs/manual/mcp.md`
- 修改：`docs/manual/reference/commands.md`
- 修改：`docs/manual/reference/errors.md`
- 修改：`README.md`
- 修改：`ROADMAP.md`

- [ ] **步骤 1: 写失败测试**

补 CLI / HTTP / MCP 测试，验证：
- 新规则能带 schedule/filter 字段创建
- list/info 输出包含 schedule/filter/sequence 相关字段
- MCP schema 与 list-tools 默认清单同步

- [ ] **步骤 2: 跑测试确认失败**

运行：
```bash
go test ./internal/cli ./internal/httpapi ./internal/mcpserver -run 'Test.*Notification|TestListTools|TestSchema' -count=1
```

预期：失败，原因是字段缺失或 schema 不匹配。

- [ ] **步骤 3: 写最小实现**

把 CLI / HTTP / MCP 参数和返回值同步到新规则模型。
文档里明确：
- `overdue` 不再是唯一表达
- 日常提醒应优先用 schedule + filter
- 8:50 / 9:00 的实际场景推荐示例

- [ ] **步骤 4: 跑测试确认通过**

运行：
```bash
go test ./internal/cli ./internal/httpapi ./internal/mcpserver -run 'Test.*Notification|TestListTools|TestSchema' -count=1
```

预期：通过。

- [ ] **步骤 5: 全量验证**

运行：
```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

预期：全部通过。
