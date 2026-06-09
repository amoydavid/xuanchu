# Xuanchu 定时通知实现计划

> **给 agentic workers 的要求：** 必须使用 `superpowers:subagent-driven-development`（如果当前环境支持子代理）或 `superpowers:executing-plans` 执行本计划。步骤使用 checkbox（`- [ ]`）语法跟踪进度。

> **现状补充：** 本计划覆盖 M16 第一阶段 `trigger_type/offset/after` 兼容模型。后续已按 `docs/superpowers/plans/2026-06-08-xuanchu-scheduled-notification-rule-filter-extension-implementation.md` 扩展为 `schedule + task filter`，并要求 CLI、HTTP API、Remote Client、MCP 都暴露 `schedule_type/schedule_value/filter_source`。

**目标：** 实现规则驱动的定时通知能力，让 Xuanchu 能在任务到期前和逾期后生成幂等通知，并通过动态、安全的 notification sink 投递给 OpenClaw 等外部 Agent 平台，或通过数据库保存的 HTTP request template 调用第三方固定 Web API。

**架构：** 新增独立于 Hook 的通知域：`notification_sinks`、`reminder_rules`、`notification_deliveries` 三类存储模型；`internal/app` 负责 sink/rule 控制面、endpoint 与 request template 解析、scheduler 评估、recipient hydration 和 delivery view；`internal/notificationruntime` 负责按 delivery 冻结的 URL、method、header、body 快照投递。CLI、HTTP、Remote Client、MCP 都只是 `internal/app` 的薄壳，不复制业务逻辑。

**技术栈：** Go 1.25、GORM、`github.com/glebarez/sqlite`、`gorm.io/driver/postgres`、Cobra、chi HTTP、现有 MCP server 模式、现有 Hook SSRF / 签名 / 重试策略。

---

## 规格与范围

规格文档：`docs/superpowers/specs/2026-06-08-xuanchu-scheduled-notification-design.md`

必须进入范围：

- `notification sink` CRUD，支持 `static_url`、`template`、`config_value` 三种 endpoint 解析模式。
- `notification sink` 支持 `webhook` 与 `http_template` 两类：`webhook` 投递 Xuanchu 标准 envelope，`http_template` 使用数据库保存的 method/header/body 模板渲染第三方 HTTP 请求。
- `reminder rule` CRUD，支持 `due_before` 和 `overdue` 两种 trigger。
- audience 首版只支持 `assignees`、`explicit_users`、`assignees_and_explicit_users`。
- scheduler 根据 pending task + due + rule 生成幂等 notification delivery。
- endpoint resolver 在生成 delivery 时固化 `resolved_url`，并校验 scheme、allowed host、SSRF。
- endpoint/template resolver 在生成 delivery 时固化 `rendered_method`、`rendered_headers_json`、`rendered_body`、`rendered_content_type`；retry/replay 不重新渲染当前 sink 模板。
- dispatcher 使用冻结的请求快照投递，支持 retry、dead-letter、disabled-skip、replay。
- CLI、HTTP、Remote Client、MCP 全部贯通。
- payload 里的 recipient 必须使用完整 `task.UserInfo`，包含 external IDs。

明确不进入范围：

- 不内置 OpenClaw、飞书、Slack、邮件 adapter。
- 不执行任意 shell、JS 或本地脚本；首版只做受控的 HTTP request template。
- 不让 Agent 自己扫描任务。
- 不支持 project owner / maintainer audience；不能把 `project.created_by` 当成项目负责人。
- 不做 UI。
- 不做复杂工作流引擎。
- 不因为发送提醒而修改任务状态或 urgency。

## 文件责任图

新增文件：

- `internal/storage/notification_sink_repo.go`：`NotificationSink` CRUD。
- `internal/storage/reminder_rule_repo.go`：`ReminderRule` CRUD 和 enabled rule 列表。
- `internal/storage/notification_delivery_repo.go`：通知 outbox 的 enqueue、claim、retry、dead-letter、requeue。
- `internal/app/notification.go`：sink/rule/delivery 的 app view、校验、CRUD、replay。
- `internal/app/notification_endpoint.go`：endpoint resolver、受控模板展开、allowed host 校验、config lookup。
- `internal/app/notification_scheduler.go`：定时规则评估和 delivery 生成。
- `internal/notificationruntime/dispatcher.go`：通知投递运行时。
- `internal/notificationruntime/headers.go`：通知签名和请求 header。
- `internal/cli/notification.go`：`notification` 和 `reminder rule` CLI。
- `internal/httpapi/notifications.go`：HTTP API handler 和 DTO。
- `internal/remote/notification.go`：Remote Client DTO 和调用方法。
- `internal/mcpserver/tools_notification.go`：通知相关 MCP tools。
- `docs/manual/notifications.md`：用户手册。

修改文件：

- `internal/storage/models.go`：新增三类 model。
- `internal/storage/migrate_sqlite.go`：纳入 AutoMigrate。
- `internal/app/service.go`：挂载通知相关 repo。
- `internal/app/permission.go`：新增通知和提醒权限常量。
- `internal/app/workspace.go`：新增 role -> permission 映射。
- `internal/auth/scope.go`：新增 token scope。
- `internal/auth/scope_test.go`：更新 scope registry / wildcard 测试。
- `internal/app/hook_endpoint.go`：复用或抽取 webhook URL/SSRF 校验 helper，不能破坏 Hook 行为。
- `internal/cli/root.go`：注册新命令。
- `internal/cli/server.go`：启动 reminder scheduler 和 notification dispatcher。
- `internal/httpapi/router.go`：注册 HTTP routes。
- `internal/httpapi/envelope.go`：映射新错误码。
- `internal/mcpserver/server.go`：注册 MCP tools。
- `internal/mcpserver/integration_test.go` 和 testdata schema：更新 tool 列表与 golden schema。
- `docs/manual/mcp.md`、`docs/manual/remote-cli-and-api.md`、`README.md`、`ROADMAP.md`：同步用户可见能力。

## Chunk 1：存储模型与 App 控制面

### Task 1：新增存储模型和 Repository

**文件：**
- 修改：`internal/storage/models.go`
- 修改：`internal/storage/migrate_sqlite.go`
- 新增：`internal/storage/notification_sink_repo.go`
- 新增：`internal/storage/reminder_rule_repo.go`
- 新增：`internal/storage/notification_delivery_repo.go`
- 测试：`internal/storage/notification_sink_repo_test.go`
- 测试：`internal/storage/reminder_rule_repo_test.go`
- 测试：`internal/storage/notification_delivery_repo_test.go`

- [ ] **Step 1：先写失败测试**

新增测试：

```go
func TestNotificationSinkRepositoryCRUD(t *testing.T) {}
func TestReminderRuleRepositoryListEnabledByWorkspace(t *testing.T) {}
func TestNotificationDeliveryRepositoryDedupeKeyUnique(t *testing.T) {}
func TestNotificationDeliveryRepositoryClaimDue(t *testing.T) {}
func TestNotificationDeliveryRepositoryReplayKeepsResolvedURL(t *testing.T) {}
```

关键断言：

- sink name 在同一 workspace 内唯一。
- `http_template` sink 的 `HeaderTemplatesJSON`、`BodyTemplate`、`BodyContentType`、`SecretRefsJSON` 能完整保存和读取。
- rule name 在同一 workspace 内唯一。
- `NotificationDelivery.DedupeKey` 唯一，重复 enqueue 不报错、不新增重复记录。
- `ClaimDue` 只领取 `queued` / `retry_wait` 且到期的 delivery。
- `Requeue` 不改变 `ResolvedURL`、`RenderedMethod`、`RenderedHeadersJSON`、`RenderedBody`、`RenderedContentType`。

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./internal/storage -run 'TestNotification|TestReminder'
```

预期：失败，原因是 model/repo 尚不存在。

- [ ] **Step 3：实现 model**

在 `internal/storage/models.go` 新增：

```go
type NotificationSink struct {
    ID               string `gorm:"primaryKey"`
    WorkspaceID      string `gorm:"not null;index:idx_notification_sinks_workspace;uniqueIndex:idx_notification_sinks_ws_name,priority:1"`
    Name             string `gorm:"not null;uniqueIndex:idx_notification_sinks_ws_name,priority:2"`
    Type             string `gorm:"not null"`
    EndpointMode     string `gorm:"not null"`
    URL              string `gorm:"not null;default:''"`
    URLTemplate      string `gorm:"not null;default:''"`
    ConfigKey        string `gorm:"not null;default:''"`
    AllowedHostsJSON string `gorm:"not null;default:'[]'"`
    HTTPMethod       string `gorm:"not null;default:'POST'"`
    HeaderTemplatesJSON string `gorm:"not null;default:'[]'"`
    BodyTemplate     string `gorm:"not null;default:''"`
    BodyContentType  string `gorm:"not null;default:''"`
    SecretRefsJSON   string `gorm:"not null;default:'{}'"`
    Secret           string `gorm:"not null;default:''"`
    Enabled          *bool  `gorm:"not null;default:true;index"`
    TimeoutSeconds   int    `gorm:"not null;default:10"`
    MaxAttempts      int    `gorm:"not null;default:5"`
    CreatedBy        string `gorm:"not null;index"`
    CreatedAt        int64  `gorm:"not null"`
    ModifiedAt       int64  `gorm:"not null"`
}
```

```go
type ReminderRule struct {
    ID                   string  `gorm:"primaryKey"`
    WorkspaceID          string  `gorm:"not null;index:idx_reminder_rules_workspace;uniqueIndex:idx_reminder_rules_ws_name,priority:1"`
    ProjectID            *string `gorm:"index"`
    Name                 string  `gorm:"not null;uniqueIndex:idx_reminder_rules_ws_name,priority:2"`
    Enabled              *bool   `gorm:"not null;default:true;index"`
    TriggerType          string  `gorm:"not null;index"`
    OffsetSeconds        int64   `gorm:"not null;default:0"`
    AfterSeconds         int64   `gorm:"not null;default:0"`
    RepeatPolicy         string  `gorm:"not null;default:'once'"`
    AudienceType         string  `gorm:"not null"`
    RecipientUserIDsJSON string  `gorm:"not null;default:'[]'"`
    SinkID               string  `gorm:"not null;index"`
    CreatedBy            string  `gorm:"not null;index"`
    CreatedAt            int64   `gorm:"not null"`
    ModifiedAt           int64   `gorm:"not null"`
}
```

```go
type NotificationDelivery struct {
    ID                          string  `gorm:"primaryKey"`
    WorkspaceID                 string  `gorm:"not null;index"`
    ProjectID                   *string `gorm:"index"`
    RuleID                      string  `gorm:"not null;index"`
    SinkID                      string  `gorm:"not null;index"`
    TaskUUID                    string  `gorm:"not null;index"`
    RecipientUserID             string  `gorm:"not null;index"`
    EventID                     string  `gorm:"not null;index"`
    EventType                   string  `gorm:"not null;index"`
    DedupeKey                   string  `gorm:"not null;uniqueIndex"`
    ResolvedURL                 string  `gorm:"not null"`
    ResolvedEndpointSource      string  `gorm:"not null;default:''"`
    ResolvedEndpointFingerprint string  `gorm:"not null;default:''"`
    RenderedMethod              string  `gorm:"not null;default:'POST'"`
    RenderedHeadersJSON         string  `gorm:"not null;default:'{}'"`
    RenderedBody                string  `gorm:"not null;default:''"`
    RenderedContentType         string  `gorm:"not null;default:''"`
    PayloadJSON                 string  `gorm:"not null"`
    Status                      string  `gorm:"not null;index:idx_notification_deliveries_due,priority:1"`
    AttemptCount                int     `gorm:"not null;default:0"`
    NextAttemptAt               *int64  `gorm:"index:idx_notification_deliveries_due,priority:2"`
    ClaimExpiresAt              *int64  `gorm:"index"`
    LastAttemptAt               *int64
    LastStatusCode              *int
    LastError                   string
    CreatedAt                   int64 `gorm:"not null;index"`
    ModifiedAt                  int64 `gorm:"not null"`
}
```

把三类 model 加入 `AutoMigrate`。

- [ ] **Step 4：实现 Repository**

`NotificationDeliveryRepository.Enqueue` 必须使用 `dedupe_key` conflict do nothing。推荐使用 GORM `clause.OnConflict`，并在 SQLite/PostgreSQL 测试路径都覆盖。

`ClaimDue` 可以镜像现有 `HookDeliveryRepository.ClaimDue` 的 `UPDATE ... RETURNING *` 模式，因为当前项目已在 Hook 投递中使用该 SQLite/PostgreSQL 兼容路径。不要新增方言专属 DDL，不要引入 CGO SQLite。

- [ ] **Step 5：运行存储测试**

```bash
go test ./internal/storage -run 'TestNotification|TestReminder|TestMigrate|TestPostgres'
```

预期：通过。

## Chunk 2：App 控制面、权限与 Endpoint Resolver

### Task 2：实现 app 层 CRUD、权限和 scope

**文件：**
- 修改：`internal/app/service.go`
- 修改：`internal/app/permission.go`
- 修改：`internal/app/workspace.go`
- 修改：`internal/auth/scope.go`
- 修改：`internal/auth/scope_test.go`
- 新增：`internal/app/notification.go`
- 测试：`internal/app/notification_test.go`
- 修改：`internal/httpapi/envelope.go`

- [ ] **Step 1：先写失败测试**

新增测试：

```go
func TestAddNotificationSinkStaticURL(t *testing.T) {}
func TestAddNotificationSinkConfigValueRequiresConfigKeyAndAllowedHost(t *testing.T) {}
func TestAddNotificationSinkRejectsUnsafeURL(t *testing.T) {}
func TestAddNotificationSinkHTTPTemplateStoresHeaderAndBodyTemplates(t *testing.T) {}
func TestAddNotificationSinkHTTPTemplateRejectsSecretInURLTemplate(t *testing.T) {}
func TestAddNotificationSinkHTTPTemplateRejectsInvalidJSONBodyTemplate(t *testing.T) {}
func TestAddReminderRuleDueBeforeRequiresOffset(t *testing.T) {}
func TestAddReminderRuleRejectsUnsupportedProjectOwnerAudience(t *testing.T) {}
func TestNotificationSinkSecretNotInView(t *testing.T) {}
func TestNotificationDeliveryReplayWritesAudit(t *testing.T) {}
func TestNotificationScopeIsolation(t *testing.T) {}
```

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./internal/app ./internal/auth -run 'Test.*Notification|Test.*Reminder|TestScope'
```

预期：失败，原因是 app 能力和 scope 尚不存在。

- [ ] **Step 3：新增权限和 token scope**

`internal/app/permission.go` 新增：

```go
PermissionNotificationRead  Permission = "notification.read"
PermissionNotificationWrite Permission = "notification.write"
PermissionReminderRead      Permission = "reminder.read"
PermissionReminderWrite     Permission = "reminder.write"
```

`internal/auth/scope.go` 新增：

```go
"notification:read", "notification:write",
"reminder:read", "reminder:write",
```

`internal/app/workspace.go` 映射规则：

- owner/admin：notification/reminder read/write。
- member：开放 reminder rule read，方便项目成员理解为什么会收到提醒；不开放 notification sink/delivery read，避免泄露 endpoint、secret ref 和投递历史。
- viewer：首版不开放 notification sink/delivery/rule read；后续若要让 viewer 读取规则，需要单独设计。

- [ ] **Step 4：实现 app 输入、视图和校验**

输入结构：

```go
type NotificationSinkAddInput struct {
    Name             string
    Type             string
    EndpointMode     string
    URL              string
    URLTemplate      string
    ConfigKey        string
    AllowedHosts     []string
    HTTPMethod       string
    HeaderTemplates  []HTTPHeaderTemplateInput
    BodyTemplate     string
    BodyContentType  string
    SecretRefs       []HTTPTemplateSecretRefInput
    Secret           string
    TimeoutSeconds   int
    MaxAttempts      int
}

type HTTPHeaderTemplateInput struct {
    Name  string
    Value string
}

type HTTPTemplateSecretRefInput struct {
    Alias     string
    ConfigKey string
}

type ReminderRuleAddInput struct {
    Name          string
    ProjectRef    string
    TriggerType   string
    OffsetSeconds int64
    AfterSeconds  int64
    RepeatPolicy  string
    AudienceType  string
    Recipients    []string
    SinkRef       string
}
```

校验规则：

- `Type` 只允许 `webhook`、`http_template`。
- `EndpointMode` 默认 `static_url`。
- `static_url` 必须有 `URL`。
- `template` 必须有 `URLTemplate`。
- `config_value` 必须有 `ConfigKey`。
- 动态 endpoint 模式必须至少有一个 `AllowedHosts`。
- `AllowedHosts` 首版只支持精确 host，不支持 wildcard。
- `webhook` 固定使用 `POST`，最终 body 是 Xuanchu 标准 notification envelope。
- `http_template` 首版只允许 `HTTPMethod=POST`。
- `http_template` 必须把 `HeaderTemplates`、`BodyTemplate`、`BodyContentType`、`SecretRefs` 保存在 `notification_sinks` 表；不能依赖本地文件或临时 CLI 参数。
- `HeaderTemplates` 必须是结构化 name/value 数组；header name 必须是合法 HTTP header，不能包含换行。
- `BodyContentType=application/json` 时，`BodyTemplate` 保存前必须能通过“模板占位符替换为 JSON 字符串哨兵值”后的 JSON 语法校验。
- `SecretRefs` 是“模板别名 -> shared config key”映射，例如 `feishu_bot_token=integrations.feishu.bot_token`；目标 config schema 必须存在、允许 workspace/project scope 读取，并且 `secret=true`。
- `SecretRefs` 只声明 header/body 模板可引用的 secret 来源；view、audit、日志、delivery view 都不能返回 secret 值。
- URL 模板不允许使用 `secret.*`、`task.description`、annotation、用户回复文本等非路由变量。
- `TriggerType` 只允许 `due_before`、`overdue`。
- `due_before` 必须 `OffsetSeconds > 0`。
- `RepeatPolicy` 只允许 `once` 或 `every:<duration>`。
- `AudienceType` 只允许 `assignees`、`explicit_users`、`assignees_and_explicit_users`。
- `project_owner`、`project_maintainer`、`assignees_and_project_owner` 必须返回 `audience_unsupported`。
- `Recipients` 解析为当前 workspace member 的 user ID。
- 所有 view 都不能返回 secret；audit payload 只允许 `secret_fingerprint`、`secret_refs` alias 和 config key，不允许 secret 值。

- [ ] **Step 5：实现 CRUD 方法**

需要实现：

```go
func (s *Service) AddNotificationSink(input NotificationSinkAddInput) (NotificationSinkView, error)
func (s *Service) ListNotificationSinks(includeDisabled bool) ([]NotificationSinkView, error)
func (s *Service) NotificationSinkInfo(ref string) (NotificationSinkView, error)
func (s *Service) ModifyNotificationSink(ref string, input NotificationSinkModifyInput) (NotificationSinkView, error)
func (s *Service) EnableNotificationSink(ref string) (NotificationSinkView, error)
func (s *Service) DisableNotificationSink(ref string) (NotificationSinkView, error)
func (s *Service) DeleteNotificationSink(ref string) error

func (s *Service) AddReminderRule(input ReminderRuleAddInput) (ReminderRuleView, error)
func (s *Service) ListReminderRules(projectRef string, includeDisabled bool) ([]ReminderRuleView, error)
func (s *Service) ReminderRuleInfo(ref string) (ReminderRuleView, error)
func (s *Service) ModifyReminderRule(ref string, input ReminderRuleModifyInput) (ReminderRuleView, error)
func (s *Service) EnableReminderRule(ref string) (ReminderRuleView, error)
func (s *Service) DisableReminderRule(ref string) (ReminderRuleView, error)
func (s *Service) DeleteReminderRule(ref string) error

func (s *Service) ListNotificationDeliveries(status string, limit int, offset int) ([]NotificationDeliveryView, error)
func (s *Service) NotificationDeliveryInfo(id string) (NotificationDeliveryView, error)
func (s *Service) ReplayNotificationDelivery(id string) (NotificationDeliveryView, error)
```

Audit action：

- `notification.sink.create`
- `notification.sink.modify`
- `notification.sink.delete`
- `notification.delivery.replay`
- `reminder.rule.create`
- `reminder.rule.modify`
- `reminder.rule.delete`

- [ ] **Step 6：运行测试**

```bash
go test ./internal/app ./internal/auth -run 'Test.*Notification|Test.*Reminder|TestScope'
```

预期：通过。

### Task 3：实现 Endpoint 与 Request Template Resolver

**文件：**
- 新增：`internal/app/notification_endpoint.go`
- 测试：`internal/app/notification_endpoint_test.go`
- 可能修改：`internal/app/hook_endpoint.go`

- [ ] **Step 1：先写失败测试**

新增测试：

```go
func TestNotificationEndpointStaticURL(t *testing.T) {}
func TestNotificationEndpointTemplateWorkspaceAndProject(t *testing.T) {}
func TestNotificationEndpointTemplateRecipientExternalID(t *testing.T) {}
func TestNotificationEndpointConfigValueProjectOverridesWorkspace(t *testing.T) {}
func TestNotificationEndpointAllowedHostRequiredForDynamicModes(t *testing.T) {}
func TestNotificationEndpointRejectsDisallowedHost(t *testing.T) {}
func TestNotificationEndpointRejectsTaskDescriptionVariable(t *testing.T) {}
func TestNotificationEndpointRejectsSecretVariableInURL(t *testing.T) {}
func TestNotificationEndpointRejectsMissingVariable(t *testing.T) {}
func TestNotificationEndpointRejectsSSRF(t *testing.T) {}
func TestNotificationRequestTemplateWebhookUsesEnvelopeBody(t *testing.T) {}
func TestNotificationRequestTemplateHTTPTemplateRendersHeadersAndBody(t *testing.T) {}
func TestNotificationRequestTemplateMissingSecretDeadLettersRecipient(t *testing.T) {}
func TestNotificationRequestTemplateRenderedSnapshotFrozen(t *testing.T) {}
func TestNotificationRequestTemplateRedactsSecretsInView(t *testing.T) {}
```

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./internal/app -run TestNotificationEndpoint
```

预期：失败。

- [ ] **Step 3：实现 endpoint resolver**

不要使用任意模板引擎。实现一个小型显式替换器，只允许这些变量：

- `workspace.id`
- `workspace.slug`
- `project.id`
- `project.slug`
- `rule.id`
- `rule.name`
- `recipient.id`
- `recipient.external_ids.<provider>`

`config_value` 模式：

- 使用 sink 存储的 `ConfigKey`，不能从 rule、task、HTTP、MCP 参数动态传入。
- 如果有 project，先读 project config，再 fallback workspace config。
- 测试里必须先创建 scoped config schema，确保 key 允许 project scope。
- 解析出的 URL 必须继续校验 scheme、allowed host、SSRF。

错误码：

- `endpoint_unresolved`
- `endpoint_host_denied`
- `endpoint_mode_invalid`
- `endpoint_template_invalid`
- 底层 URL/SSRF 错误可复用 `hook_endpoint_invalid`。

- [ ] **Step 4：实现 request template resolver**

接口建议：

```go
type NotificationResolvedRequest struct {
    ResolvedURL                 string
    ResolvedEndpointSource      string
    ResolvedEndpointFingerprint string
    RenderedMethod              string
    RenderedHeadersJSON         string
    RenderedBody                string
    RenderedContentType         string
    PayloadJSON                 string
}
```

规则：

- 先生成标准 Xuanchu notification envelope，并保存到 `PayloadJSON`。
- `webhook`：
  - `RenderedMethod=POST`。
  - `RenderedBody=PayloadJSON`。
  - `RenderedContentType=application/json`。
  - `RenderedHeadersJSON` 包含签名所需的固定 header 基础信息；最终签名值可在 dispatcher 投递前按 timestamp 生成。
- `http_template`：
  - `RenderedMethod` 来自 sink 的 `HTTPMethod`，首版只允许 `POST`。
  - `RenderedHeadersJSON` 来自数据库里的 `HeaderTemplatesJSON` 渲染结果。
  - `RenderedBody` 来自数据库里的 `BodyTemplate` 渲染结果。
  - `RenderedContentType` 来自 `BodyContentType`，同时应补到最终请求 header。
- header/body 模板允许变量：`workspace.*`、`project.*`、`rule.*`、`recipient.*`、`task.uuid`、`task.task_slug`、`task.description`、`task.due`、`task.status`、`secret.<name>`。
- `secret.<name>` 必须出现在 sink 的 `SecretRefsJSON` 中。`SecretRefsJSON` 中的 config key 必须指向 shared config schema 里 `secret=true` 的 key。
- secret 读取复用 scoped config 读取链：有 project 时先读 project 显式值，再 fallback workspace 显式值和 schema default；没有可用值返回 `template_unresolved`。
- 渲染出的 request snapshot 写入 delivery，retry/replay 只读 delivery，不读当前 sink 模板。
- App view、HTTP response、CLI `--json`、MCP response、audit 和普通日志必须对 `RenderedHeadersJSON` / `RenderedBody` 做 secret 脱敏。

- [ ] **Step 5：运行 resolver 测试**

```bash
go test ./internal/app -run TestNotificationEndpoint
```

预期：通过。

## Chunk 3：Scheduler 与 Dispatcher

### Task 4：实现 Reminder Scheduler

**文件：**
- 新增：`internal/app/notification_scheduler.go`
- 测试：`internal/app/notification_scheduler_test.go`
- 修改：`internal/app/service.go`

- [ ] **Step 1：先写失败测试**

新增测试：

```go
func TestReminderSchedulerDueBeforeEnqueuesDelivery(t *testing.T) {}
func TestReminderSchedulerDueBeforeDoesNotBackfillAfterDue(t *testing.T) {}
func TestReminderSchedulerOverdueOnceEnqueuesDelivery(t *testing.T) {}
func TestReminderSchedulerOverdueRepeatUsesWindowDedupe(t *testing.T) {}
func TestReminderSchedulerSkipsCompletedDeletedAndNoDue(t *testing.T) {}
func TestReminderSchedulerUsesAssigneeRecipients(t *testing.T) {}
func TestReminderSchedulerUsesExplicitRecipients(t *testing.T) {}
func TestReminderSchedulerUsesAssigneesAndExplicitRecipientsDeduped(t *testing.T) {}
func TestReminderSchedulerEndpointFailureDoesNotBlockOtherRecipients(t *testing.T) {}
func TestReminderSchedulerPayloadUsesFullUserInfo(t *testing.T) {}
func TestReminderSchedulerDedupePreventsDuplicateDelivery(t *testing.T) {}
```

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./internal/app -run TestReminderScheduler
```

预期：失败。

- [ ] **Step 3：实现 scheduler**

接口建议：

```go
type ReminderSchedulerOptions struct {
    Store     *storage.Store
    Clock     Clock
    BatchSize int
    Logger    optionalLogger
}

type ReminderSchedulerRunResult struct {
    RulesChecked       int
    DeliveriesEnqueued int
    RecipientsSkipped  int
}

func NewReminderScheduler(opts ReminderSchedulerOptions) *ReminderScheduler
func (s *ReminderScheduler) RunOnce(ctx context.Context) (ReminderSchedulerRunResult, error)
```

评估规则：

- 读取 enabled rules。
- 按 rule 的 workspace/project scope 查询 pending + due tasks。
- `due_before`：`now >= due - offset && now < due`。
- `overdue`：`now >= due + after`。
- completed、deleted、无 due、无 recipient 的任务不生成 delivery。
- scheduler 不 impersonate 人类用户，不使用任何用户 active context。
- 若 `Service.Require` 不适合 scheduler，scheduler 直接用 repo 做只读评估和 enqueue。
- recipient 解析后必须 hydration 为完整 `task.UserInfo`，包含 external IDs。
- 每个 recipient 独立解析 endpoint；某个 recipient endpoint 失败，不阻塞其他 recipient。
- 通过 `ReminderSchedulerRunResult` 暴露 checked/enqueued/skipped 数量，便于测试和日志。

dedupe key：

```text
<workspace_id>:<rule_id>:<task_uuid>:<recipient_user_id>:<event_type>:<window_start>
```

- [ ] **Step 4：运行 scheduler 测试**

```bash
go test ./internal/app -run TestReminderScheduler
```

预期：通过。

### Task 5：实现 Notification Dispatcher

**文件：**
- 新增：`internal/notificationruntime/dispatcher.go`
- 新增：`internal/notificationruntime/headers.go`
- 测试：`internal/notificationruntime/dispatcher_test.go`
- 修改：`internal/cli/server.go`

- [ ] **Step 1：先写失败测试**

新增测试：

```go
func TestNotificationDispatcherDeliversFrozenResolvedURL(t *testing.T) {}
func TestNotificationDispatcherDoesNotUseCurrentSinkURLOnRetry(t *testing.T) {}
func TestNotificationDispatcherDisabledSinkSkipped(t *testing.T) {}
func TestNotificationDispatcherCompletedTaskSkipped(t *testing.T) {}
func TestNotificationDispatcherHTTP500Retries(t *testing.T) {}
func TestNotificationDispatcherHTTP400DeadLetters(t *testing.T) {}
func TestNotificationDispatcherMaxAttemptsDeadLetters(t *testing.T) {}
func TestNotificationDispatcherNoRedirect(t *testing.T) {}
func TestNotificationDispatcherDNSRebindingBlocked(t *testing.T) {}
func TestNotificationDispatcherSignatureHeadersMatch(t *testing.T) {}
func TestNotificationDispatcherSecretNotInDeliveryView(t *testing.T) {}
func TestNotificationDispatcherHTTPTemplateUsesRenderedRequestSnapshot(t *testing.T) {}
func TestNotificationDispatcherHTTPTemplateDoesNotUseCurrentSinkTemplateOnRetry(t *testing.T) {}
```

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./internal/notificationruntime
```

预期：失败。

- [ ] **Step 3：实现 dispatcher**

可以参考 `internal/hookruntime`，但不能改坏 Hook 语义。

行为要求：

- `RunOnce`：recover stale -> claim due -> 逐条投递。
- 每次投递加载 sink。
- sink disabled 时标记 `disabled_skipped`。
- 投递前加载 task；如果 task 已 completed/deleted 或不再 pending，标记 `disabled_skipped`，错误说明为 `condition no longer matches`。
- 请求 URL 必须使用 delivery 上冻结的 `ResolvedURL`，不能重新从 sink/config/template 解析。
- 请求 method/header/body/content type 必须使用 delivery 上冻结的 `RenderedMethod`、`RenderedHeadersJSON`、`RenderedBody`、`RenderedContentType`。
- `webhook` delivery 的 `RenderedBody` 是标准 envelope；dispatcher 在投递前追加/覆盖 Xuanchu 签名 header。
- `http_template` delivery 的 `RenderedBody` 是第三方 API 请求体；dispatcher 不把标准 envelope 当作最终 body，但 `PayloadJSON` 仍用于审计和排查。
- `http_template` 的 header/body 可能包含 secret，dispatcher 错误日志、delivery view、失败记录都必须脱敏。
- 发送前再次做 SSRF 校验和 allowed host 校验。
- 不跟随 redirect。
- 2xx 成功；3xx dead-letter；非 429 的 4xx dead-letter；5xx/429 retry。
- retry/backoff/max attempts 复用 Hook dispatcher 的策略。

Header：

- `X-Xuanchu-Event`
- `X-Xuanchu-Event-Id`
- `X-Xuanchu-Event-Version`
- `X-Xuanchu-Signature-256`
- `X-Xuanchu-Timestamp`
- `X-Xuanchu-Delivery`
- `User-Agent`

签名输入：

```text
<delivery_id>.<timestamp_unix_seconds>.<body>
```

- [ ] **Step 4：启动 server 后台循环**

修改 `internal/cli/server.go`：

- 保留现有 hook dispatcher。
- 新增 reminder scheduler loop。
- 新增 notification dispatcher loop。
- 增加 flag：
  - `--reminder-scheduler-interval`，默认 `60s`
  - `--notification-dispatcher-interval`，默认 `5s`
- 错误前缀：
  - `reminder scheduler: ...`
  - `notification dispatcher: ...`

- [ ] **Step 5：运行 runtime/server 测试**

```bash
go test ./internal/notificationruntime ./internal/cli -run 'TestNotification|TestServer'
```

预期：通过。

## Chunk 4：CLI、HTTP 与 Remote Client

### Task 6：实现 CLI

**文件：**
- 新增：`internal/cli/notification.go`
- 修改：`internal/cli/root.go`
- 测试：`tests/integration/cli_test.go`

- [ ] **Step 1：先写失败集成测试**

新增测试：

```go
func TestCLINotificationSinkConfigValueLifecycle(t *testing.T) {}
func TestCLINotificationSinkHTTPTemplateLifecycle(t *testing.T) {}
func TestCLIReminderRuleLifecycle(t *testing.T) {}
func TestCLINotificationDeliveryListAndReplayJSON(t *testing.T) {}
func TestCLIRejectsProjectOwnerAudience(t *testing.T) {}
```

重点断言：

- stdout/stderr 分离。
- `--json` 输出稳定且不包含 secret。
- 动态 endpoint 参数能 round-trip。
- `http_template` 的 method/header/body/content type 能 round-trip，且输出只展示模板和 secret ref 名称，不展示 secret 值。

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./tests/integration -run 'TestCLI.*Notification|TestCLI.*Reminder'
```

预期：失败。

- [ ] **Step 3：实现命令树**

命令：

```text
notification
  sink
    add <name>
    list
    info <sink>
    modify <sink>
    enable <sink>
    disable <sink>
    delete <sink>
  delivery
    list
    info <delivery>
    replay <delivery>
reminder
  rule
    add <name>
    list
    info <rule>
    modify <rule>
    enable <rule>
    disable <rule>
    delete <rule>
```

flag：

- `--type`
- `--endpoint-mode`
- `--url`
- `--url-template`
- `--config-key`
- `--allowed-host` repeatable
- `--method`
- `--header-template` repeatable，格式为 `Name=template`
- `--body-template`
- `--body-template-file`，只在 CLI 读取文件内容后写入数据库，不在后续运行时依赖该文件
- `--body-content-type`
- `--secret-ref` repeatable，格式为 `<alias>=<shared-config-key>`
- `--secret-stdin`
- `--timeout`
- `--max-attempts`
- `--project`
- `--trigger`
- `--offset`
- `--after`
- `--repeat`
- `--audience`
- `--recipient` repeatable
- `--sink`
- `--status`
- `--limit`

`http_template` 示例：

```bash
xuanchu notification sink add feishu-bot \
  --type http_template \
  --endpoint-mode config_value \
  --config-key integrations.feishu.webhook_url \
  --allowed-host open.feishu.cn \
  --method POST \
  --header-template 'Content-Type=application/json' \
  --header-template 'Authorization=Bearer {{secret.feishu_bot_token}}' \
  --body-content-type application/json \
  --body-template '{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}}' \
  --secret-ref feishu_bot_token=integrations.feishu.bot_token
```

- [ ] **Step 4：运行 CLI 测试**

```bash
go test ./tests/integration -run 'TestCLI.*Notification|TestCLI.*Reminder'
```

预期：通过。

### Task 7：实现 HTTP API 与 Remote Client

**文件：**
- 新增：`internal/httpapi/notifications.go`
- 修改：`internal/httpapi/router.go`
- 修改：`internal/httpapi/envelope.go`
- 测试：`internal/httpapi/notifications_test.go`
- 新增：`internal/remote/notification.go`
- 测试：`internal/remote/notification_test.go`

- [ ] **Step 1：先写失败 HTTP 测试**

新增测试：

```go
func TestHTTPNotificationSinkLifecycle(t *testing.T) {}
func TestHTTPNotificationSinkHTTPTemplateLifecycle(t *testing.T) {}
func TestHTTPReminderRuleLifecycle(t *testing.T) {}
func TestHTTPNotificationDeliveryReplay(t *testing.T) {}
func TestHTTPNotificationScopeIsolation(t *testing.T) {}
func TestHTTPNotificationSecretNotReturned(t *testing.T) {}
func TestHTTPNotificationDynamicEndpointValidationErrors(t *testing.T) {}
```

routes：

```text
GET    /api/v1/notification-sinks
POST   /api/v1/notification-sinks
GET    /api/v1/notification-sinks/{sinkID}
PATCH  /api/v1/notification-sinks/{sinkID}
POST   /api/v1/notification-sinks/{sinkID}/enable
POST   /api/v1/notification-sinks/{sinkID}/disable
DELETE /api/v1/notification-sinks/{sinkID}

GET    /api/v1/reminder-rules
POST   /api/v1/reminder-rules
GET    /api/v1/reminder-rules/{ruleID}
PATCH  /api/v1/reminder-rules/{ruleID}
POST   /api/v1/reminder-rules/{ruleID}/enable
POST   /api/v1/reminder-rules/{ruleID}/disable
DELETE /api/v1/reminder-rules/{ruleID}

GET    /api/v1/notification-deliveries
GET    /api/v1/notification-deliveries/{deliveryID}
POST   /api/v1/notification-deliveries/{deliveryID}/replay
```

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./internal/httpapi -run 'TestHTTPNotification|TestHTTPReminder'
```

预期：失败。

- [ ] **Step 3：实现 HTTP handler**

要求：

- 使用现有 `scopedService` 模式。
- HTTP scope / permission 映射：
  - sink 读：`notification:read` + `PermissionNotificationRead`
  - sink 写：`notification:write` + `PermissionNotificationWrite`
  - delivery 读：`notification:read` + `PermissionNotificationRead`
  - delivery replay：`notification:write` + `PermissionNotificationWrite`
  - rule 读：`reminder:read` + `PermissionReminderRead`
  - rule 写：`reminder:write` + `PermissionReminderWrite`
- DTO 与 CLI JSON 保持一致。
- response 不返回 secret。
- request/response schema 必须包含 `type`、`http_method`、`header_templates`、`body_template`、`body_content_type`、`secret_refs`。
- `http_template` response 可以返回模板原文和 secret ref 名称，但不能返回已渲染 secret 值；delivery response 中的 rendered request snapshot 必须脱敏。
- 错误码映射：
  - `notification_sink_not_found` -> 404
  - `reminder_rule_not_found` -> 404
  - `notification_delivery_not_found` -> 404
  - `endpoint_unresolved`、`endpoint_host_denied`、`template_unresolved`、`audience_unsupported`、`reminder_rule_invalid`、`notification_sink_invalid` -> 400

- [ ] **Step 4：实现 Remote Client**

新增方法覆盖 sink/rule/delivery 全生命周期，供 remote CLI 复用。DTO 到 app view 的转换必须保留 `task.UserInfo` 结构，不能降级为裸 UUID。

- [ ] **Step 5：运行 HTTP/remote 测试**

```bash
go test ./internal/httpapi ./internal/remote -run 'Test.*Notification|Test.*Reminder'
```

预期：通过。

## Chunk 5：MCP、文档与全量验证

### Task 8：实现 MCP tools

**文件：**
- 新增：`internal/mcpserver/tools_notification.go`
- 修改：`internal/mcpserver/server.go`
- 修改：`internal/mcpserver/integration_test.go`
- 新增/更新：`internal/mcpserver/testdata/notification_*.schema.json`
- 新增/更新：`internal/mcpserver/testdata/reminder_*.schema.json`

- [ ] **Step 1：先写失败 MCP 测试**

必须注册这些 tool name：

```text
notification_sink_add
notification_sink_list
notification_sink_info
notification_sink_modify
notification_sink_enable
notification_sink_disable
notification_sink_remove
reminder_rule_add
reminder_rule_list
reminder_rule_info
reminder_rule_modify
reminder_rule_enable
reminder_rule_disable
reminder_rule_remove
notification_delivery_list
notification_delivery_info
notification_delivery_replay
```

新增集成测试：

```go
func TestMCPNotificationSinkConfigValueLifecycle(t *testing.T) {}
func TestMCPReminderRuleLifecycle(t *testing.T) {}
func TestMCPNotificationDeliveryReplay(t *testing.T) {}
func TestMCPNotificationRejectsDotToolNames(t *testing.T) {}
```

- [ ] **Step 2：运行测试确认失败**

```bash
go test ./internal/mcpserver -run 'Test.*Notification|Test.*Reminder|TestListTools'
```

预期：失败。

- [ ] **Step 3：实现 MCP tools**

要求：

- 参考 `internal/mcpserver/tools_hook.go`。
- 每个 tool 通过 `serviceForTool` 获取 app service。
- MCP scope / permission 映射与 HTTP 一致：
  - `notification_sink_*` 和 `notification_delivery_*` 使用 `notification:read/write`。
  - `reminder_rule_*` 使用 `reminder:read/write`。
- MCP 层不解析 recipient、不解析 endpoint、不做 scheduler 逻辑。
- schema 描述必须包含 `endpoint_mode`、`allowed_hosts`、`config_key`、`recipient`、`audience`、`status`。
- schema 描述必须包含 `type=webhook|http_template`、`http_method`、`header_templates`、`body_template`、`body_content_type`、`secret_refs`。
- tool name 全部用下划线，禁止点号。

- [ ] **Step 4：运行 MCP 测试**

```bash
go test ./internal/mcpserver -run 'Test.*Notification|Test.*Reminder|TestListTools|TestSchema'
```

预期：通过。

### Task 9：更新中文文档

**文件：**
- 新增：`docs/manual/notifications.md`
- 修改：`docs/manual/_index.md`
- 修改：`docs/manual/mcp.md`
- 修改：`docs/manual/remote-cli-and-api.md`
- 修改：`README.md`
- 修改：`ROADMAP.md`
- 必要时修改：`docs/requirements.md`

- [ ] **Step 1：写中文文档**

必须写清楚：

- reminder rule 是显式配置，没有默认逾期提醒。
- Xuanchu 负责 scheduler、幂等、审计、delivery。
- OpenClaw 负责发消息、理解用户回复、回调 Xuanchu。
- endpoint 模式：
  - `static_url`
  - `template`
  - `config_value`
- sink 类型：
  - `webhook`：投递 Xuanchu 标准 envelope，推荐给 OpenClaw 这类 Agent。
  - `http_template`：使用数据库保存的 method/header/body 模板直接调用第三方固定 Web API。
- `http_template` 的 header/body 模板必须写入数据库；`--body-template-file` 只是导入入口，不是运行时依赖。
- secret 只能作为 `secret_refs` 声明和 `{{secret.name}}` 模板引用使用，不能出现在 URL 模板中，输出必须脱敏。
- 动态 endpoint 安全规则：
  - 动态 endpoint 必须配置 allowed host。
  - 不能从 task description / 用户回复拼 URL。
  - delivery 固化 `resolved_url`。
  - retry/replay 不重新解析 endpoint。
  - 不跟随 redirect。

示例必须包含 config schema：

```bash
xuanchu --workspace dajee config schema set integrations.openclaw.notification_url \
  type:string \
  scopes:project \
  label:"OpenClaw Notification URL"

xuanchu --workspace dajee project config set agentapi \
  integrations.openclaw.notification_url \
  https://openclaw.example.com/apps/agentapi/xuanchu/notifications

xuanchu notification sink add openclaw-project \
  --type webhook \
  --endpoint-mode config_value \
  --config-key integrations.openclaw.notification_url \
  --allowed-host openclaw.example.com \
  --secret-stdin
```

第三方固定 Web API 示例必须包含：

```bash
xuanchu --workspace dajee config schema set integrations.feishu.webhook_url \
  type:string \
  scopes:project \
  label:"Feishu Bot Webhook URL"

xuanchu --workspace dajee config schema set integrations.feishu.bot_token \
  type:string \
  scopes:workspace,project \
  secret:true \
  label:"Feishu Bot Token"

xuanchu notification sink add feishu-bot \
  --type http_template \
  --endpoint-mode config_value \
  --config-key integrations.feishu.webhook_url \
  --allowed-host open.feishu.cn \
  --method POST \
  --header-template 'Content-Type=application/json' \
  --header-template 'Authorization=Bearer {{secret.feishu_bot_token}}' \
  --body-content-type application/json \
  --body-template '{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}}' \
  --secret-ref feishu_bot_token=integrations.feishu.bot_token
```

- [ ] **Step 2：检查文档没有误称 project owner audience 已实现**

```bash
rg -n 'project_owner|assignees_and_project_owner|项目负责人' docs README.md ROADMAP.md
```

预期：没有声称 project owner audience 已实现；如出现，只能是“后续扩展”且表述清楚。

### Task 10：全量验证

- [ ] **Step 1：运行聚焦测试**

```bash
go test ./internal/storage ./internal/app ./internal/notificationruntime ./internal/httpapi ./internal/remote ./internal/mcpserver -run 'Test.*Notification|Test.*Reminder|TestHook|TestMCP|TestSchema'
```

预期：通过。

- [ ] **Step 2：运行全量测试**

```bash
go test ./...
```

预期：通过。

- [ ] **Step 3：运行零 CGO 测试**

```bash
CGO_ENABLED=0 go test ./...
```

预期：通过。

- [ ] **Step 4：运行零 CGO 构建**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

预期：通过。

- [ ] **Step 5：手工 smoke test**

```bash
tmp="$(mktemp -d)"
go build -o "$tmp/xuanchu" ./cmd/xuanchu
"$tmp/xuanchu" --data-dir "$tmp/data" project add agentapi name:"AI Agent Platform"
"$tmp/xuanchu" --data-dir "$tmp/data" user add alice --email alice@example.com
"$tmp/xuanchu" --data-dir "$tmp/data" config schema set integrations.openclaw.notification_url type:string scopes:project label:"OpenClaw Notification URL"
"$tmp/xuanchu" --data-dir "$tmp/data" project config set agentapi integrations.openclaw.notification_url https://example.com/xuanchu/notifications
"$tmp/xuanchu" --data-dir "$tmp/data" notification sink add openclaw-project --type webhook --endpoint-mode config_value --config-key integrations.openclaw.notification_url --allowed-host example.com --secret-stdin
"$tmp/xuanchu" --data-dir "$tmp/data" config schema set integrations.feishu.webhook_url type:string scopes:project label:"Feishu Bot Webhook URL"
"$tmp/xuanchu" --data-dir "$tmp/data" project config set agentapi integrations.feishu.webhook_url https://open.feishu.cn/open-apis/bot/v2/hook/test
"$tmp/xuanchu" --data-dir "$tmp/data" config schema set integrations.feishu.bot_token type:string scopes:workspace,project secret:true label:"Feishu Bot Token"
"$tmp/xuanchu" --data-dir "$tmp/data" config set integrations.feishu.bot_token test-token
"$tmp/xuanchu" --data-dir "$tmp/data" notification sink add feishu-bot --type http_template --endpoint-mode config_value --config-key integrations.feishu.webhook_url --allowed-host open.feishu.cn --method POST --header-template 'Content-Type=application/json' --header-template 'Authorization=Bearer {{secret.feishu_bot_token}}' --body-content-type application/json --body-template '{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}}' --secret-ref feishu_bot_token=integrations.feishu.bot_token
"$tmp/xuanchu" --data-dir "$tmp/data" reminder rule add due-before-4h --project agentapi --trigger due_before --offset 4h --audience assignees --sink openclaw-project
```

预期：

- 命令成功。
- JSON 输出包含 sink/rule。
- 输出、audit、delivery view 都不包含 secret。
- `feishu-bot` 的 header/body 模板已经保存在数据库，删除本地 shell 历史或模板文件不影响后续 scheduler 渲染。

## 实现风险与禁止事项

- 不要把通知业务逻辑写到 CLI、HTTP、MCP、Remote Client。
- 不要复用 `HookDefinition` 表达 reminder rule。
- 不要让动态 endpoint 模板读取任意 task 字段。
- 不要把 `http_template` 做成任意脚本执行器；首版只能渲染数据库保存的 HTTP request template。
- 不要把 header/body 模板只保存在 CLI 参数、本地文件或环境变量中。
- 不要实现 wildcard allowed host。
- 不要把 `project.created_by` 当 project owner。
- replay 必须使用 delivery 冻结的 `resolved_url`，不能按当前 sink/config 重新解析。
- replay 必须使用 delivery 冻结的 method/header/body/content type，不能按当前 sink 模板重新渲染。
- 所有用户引用必须使用 `task.UserInfo`，不能输出裸 UUID。
- 不要引入 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`。

## 完成标准

- 本地 CLI、远程 CLI、HTTP API、MCP 均可管理 sink/rule/delivery。
- scheduler 能生成 due-soon / overdue delivery。
- dispatcher 能投递冻结 URL，并正确 retry/dead-letter/replay。
- OpenClaw 能消费 webhook payload，并通过既有 impersonation 边界回调 Xuanchu。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 全部通过。
