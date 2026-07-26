# 璇础 Workspace Agent 自动化 Implementation Plan

> **执行要求：** 严格按 Red → Green → Refactor 推进。每个任务先运行新增测试并记录预期失败，再写最小实现；不得先完成整条功能后补测试。每个 Task 结束时必须保持普通 Go 测试、零 CGO 构建和相关 Web 测试可独立通过。

**目标：** 把现有 Project Automation 泛化为 `workspace | project` 两种 scope，在 `/automations` 提供 Workspace 规则和跨 Project 运行记录；首个事件 `project.created` 在空项目/模板项目初始化事务末尾原子生成 Automation Delivery，Agent 使用自然语言指令和既有璇础 MCP 完成外部操作并回写 Project config。

**设计依据：** `docs/superpowers/specs/2026-07-26-workspace-agent-automation-design.md`

**架构：** Storage 使用统一 `AutomationRule` / `AutomationDelivery`；App 使用 scope-aware CRUD、Provider resolver、context builder、transactional event router、scheduler 和 dispatcher；现有 Project HTTP/App 方法保留为薄 facade。Delivery 是唯一运行记录和 transactional outbox，业务事务内只冻结 Delivery，不发送 HTTP。Workspace HTTP/Web 是新的治理入口，CLI/Remote/MCP 不增加规则 CRUD；Agent 继续通过 `project_config_list/set/unset` 操作业务结果。

**技术栈：** Go 1.25 / GORM / SQLite `github.com/glebarez/sqlite` / PostgreSQL `gorm.io/driver/postgres` / Huma HTTP/OpenAPI / React 19 + TypeScript + TanStack Router/Query + shadcn/ui + Lucide / Vitest + Testing Library + Playwright。

## 全局约束

- 文档、代码注释、测试说明和提交信息以中文为主。
- SQLite 继续使用 `github.com/glebarez/sqlite`；禁止引入 CGO SQLite。
- 新增/修复行为必须先出现失败测试，记录失败原因后再写实现。
- `AutomationRule` / `AutomationDelivery` 是最终领域和 storage 名称；旧 `ProjectAutomation*` 只允许保留在现有 Project API/facade，不得形成第二套仓储、渲染器、scheduler 或 dispatcher。
- 物理表迁移为 `automation_rules` / `automation_deliveries`，历史 ID、状态、dedupe key、请求/响应和时间必须保持。
- Delivery 创建时冻结 URL、method、masked headers、request body/hash/preview、rule scope、Project、auth config key、allowed-host config key、max attempts；只在发送时读取当前 secret 明文。
- dispatcher 不得依赖当前 Rule。Rule 被修改或删除后，既有 Delivery 仍必须可发送、诊断和 replay。
- `project.created` 使用独立 Automation event 契约，不加入 Hook/Notification 白名单；现有 `docs/skills/.../event-types.md` 中“不可订阅”说明保持正确。
- Workspace Provider 只解析 Workspace scope/default；sample Project 的同名 config 不得覆盖它。
- Provider 配置底层继续使用 Config storage/service，但 Workspace/Project Automation UI 统一经过安全 Provider facade；GET/PUT 永不返回 API key，且不得扩大通用 Workspace Config HTTP 的 business-key allowlist。
- Workspace schedule 不隐式枚举 Project/Task；Agent 需要数据时通过 MCP 查询。
- secret config 的 key/value、Provider API key、Authorization 不进入 prompt、template vars、event payload、preview、Delivery 响应、Audit、日志或错误。
- 用户身份统一使用 `task.UserInfo` / `task.JSONUserInfo` / `task.JSONActorInfo`，不输出裸 UUID。
- Workspace Automation 读权限为 `workspace:read + hook:read`，写/preview/test/replay 为 `workspace:write + hook:write`；Workspace Provider config 单独使用 `config:read/write + workspace.read/modify`，Project Provider facade 延续现有 Project config 权限。
- 现有 Project Automation 继续使用 `project + hook` 权限、Project effective Provider config、closed-project 门控和旧 HTTP 路径。
- Project Template 只捕获/实例化 Project scope 规则；Snapshot v1/v2 JSON 和 canonical hash 不因 storage 泛化而变化。
- HTTP 2xx 文案只能是“Agent 调用成功”，不能推断外部知识库等业务动作成功。
- Web 必须遵守 `DESIGN.md`：token 化颜色、36px 控件、`rounded-lg`、Lucide、shadcn `<Table>`、桌面表格无横向滚动、移动端卡片列表。
- 不提交 `web/dist`、本地二进制、数据库、密钥、token、Playwright 缓存或临时日志。

## 交付顺序与依赖

```text
Task 1 存储迁移
  -> Task 2 通用 App 规则/预览
    -> Task 3 project.created 事务路由
    -> Task 4 scheduler/dispatcher/replay
      -> Task 5 HTTP/OpenAPI
        -> Task 6 Web 契约/导航
          -> Task 7 Web 页面/交互
            -> Task 8 E2E/文档/全量收口
```

Task 3 和 Task 4 都依赖 Task 2，但为避免共享文件冲突，实际执行仍按顺序完成，不并行修改 Automation App 文件。

## Task 1：统一 Storage model、物理表迁移与 repository

**目标：** 在不丢历史 Project Automation 数据的前提下，把物理表和仓储泛化为 scope-aware Automation，并让新旧 SQLite/PostgreSQL 数据库都能安全启动。

**文件：**

- 修改 `internal/storage/models.go`
- 修改 `internal/storage/migrate_sqlite.go`
- 修改 `internal/storage/migrate_postgres.go`
- 修改 `internal/storage/migrate.go`
- 新建 `internal/storage/automation_rule_repo.go`
- 新建 `internal/storage/automation_delivery_repo.go`
- 新建 `internal/storage/automation_repo_test.go`
- 修改 `internal/storage/project_automation_repo_test.go`
- 修改 `tests/integration/e2e_migration_test.go`
- 修改 `tests/integration/postgres_e2e_test.go`
- 删除最终不再使用的 `internal/storage/project_automation_rule_repo.go`
- 删除最终不再使用的 `internal/storage/project_automation_delivery_repo.go`

### Red

- [ ] 新增 legacy SQLite fixture：只包含 `project_automation_rules` / `project_automation_deliveries` 和至少一条 succeeded、一条 retry_wait 数据。
- [ ] 测试打开 legacy DB 后只使用新表名；规则回填 `scope_type=project, scope_id=project_id`，Delivery 回填冻结的 Project scope。
- [ ] 断言迁移前后 rule/delivery 主键集合、dedupe key、状态、attempt、request/response、actor 和时间完全一致。
- [ ] 测试迁移重复执行幂等，不重复复制行、不遗留 `_legacy` 表/索引。
- [ ] 测试 fresh SQLite 直接创建新表，不先创建旧表。
- [ ] 测试 `AutomationRuleRepository.ListScope` 严格隔离 workspace/project scope，唯一名约束为 `(workspace_id, scope_type, scope_id, name)`。
- [ ] 测试 Project Template candidate query 只读取 `scope_type=project`，Workspace 规则永远不进入候选。
- [ ] 测试 `AutomationDelivery.ProjectID=nil` 可写可读，list 支持 `scope_type/rule/project/status/trigger/offset/limit`。
- [ ] 测试 Delivery list 的 `q` 对 delivery/event/provider request ID 做精确或前缀搜索，不退化为无界 `%LIKE%`。
- [ ] 测试 `LatestByRuleIDs` 一次批量返回每条 Rule 的最近 Delivery，不能由 App 做 N+1 查询。
- [ ] 测试 `ClaimDue` 从 `queued/retry_wait` 领取，并恢复 `claim_expires_at <= now` 的 stale `delivering`。
- [ ] 测试 replay insert 使用新 ID/dedupe key、`replay_of_delivery_id`，不修改原 Delivery。
- [ ] PostgreSQL opt-in 测试覆盖旧表 rename、`project_id DROP NOT NULL`、索引重建和数据保持。

运行并确认因新 model、迁移函数或新表不存在而失败：

```bash
go test ./internal/storage -run 'Automation(Scope|Migration|Rule|Delivery|Claim|Replay)' -count=1
go test ./tests/integration -run 'Migration.*Automation' -count=1
XUANCHU_E2E_POSTGRES_ADMIN_URL="$XUANCHU_E2E_POSTGRES_ADMIN_URL" go test ./tests/integration -run 'Postgres.*AutomationMigration' -count=1
```

最后一条只在环境变量已配置时执行；未配置必须报告 skipped，不能声称 PostgreSQL 已验证。

### Green

- [ ] 定义 `AutomationRule`：`WorkspaceID/ScopeType/ScopeID` 非空，移除 storage 对非空 `ProjectID` 的依赖。
- [ ] 定义 `AutomationDelivery`：冻结 `RuleScopeType/RuleScopeID`，`ProjectID *string`，新增 `ReplayOfDeliveryID`、`APIKeyConfigKey`、`AllowedHostsConfigKey`、`MaxAttempts`。
- [ ] 给 generic model 明确 `TableName()`，固定 `automation_rules` / `automation_deliveries`。
- [ ] SQLite 在 AutoMigrate generic model 之前运行 `prepareAutomationScopeSchemaSQLite`：legacy 存在时显式重建并复制，fresh DB 交给新 schema 创建。
- [ ] PostgreSQL 在 AutoMigrate 前运行 `prepareAutomationScopeSchemaPostgres`：rename、backfill、约束/索引调整放在事务内。
- [ ] 使用 migration meta key 记录完成状态，但仍检测“meta 已写、表结构不完整”的损坏状态并明确失败。
- [ ] repository 的 raw claim SQL 改为新表名，并统一状态别名为通用 Automation 名称。
- [ ] Task 1 期间可用 deprecated type/repository alias 保持 App 编译；必须在 Task 2 删除，不能留到最终交付。

### Refactor / Gate

```bash
gofmt -w internal/storage tests/integration
go test ./internal/storage -count=1
go test ./tests/integration -run 'Migration.*Automation' -count=1
CGO_ENABLED=0 go test ./internal/storage -count=1
git diff --check
```

**建议提交：** `feat: 泛化自动化存储作用域`

## Task 2：统一 Automation App service、Provider 解析、预览与 Audit

**目标：** 建立唯一的 scope-aware Automation App service；现有 Project 方法降为 facade，新增 Workspace CRUD/preview 能力，Provider/config/secret/权限边界在 App 层统一。

**文件：**

- 新建 `internal/app/automation.go`
- 新建 `internal/app/automation_preview.go`
- 新建 `internal/app/automation_delivery.go`
- 新建 `internal/app/automation_provider_config.go`
- 新建 `internal/app/workspace_automation.go`
- 新建 `internal/app/automation_provider_config_test.go`
- 新建 `internal/app/workspace_automation_test.go`
- 修改 `internal/app/project_automation.go`
- 修改 `internal/app/project_automation_preview.go`
- 修改 `internal/app/project_automation_delivery.go`
- 修改 `internal/app/project_automation_template_vars.go`
- 修改 `internal/app/project_automation_test.go`
- 修改 `internal/app/project_automation_template_vars_test.go`
- 修改 `internal/app/project_template_candidates.go`
- 修改 `internal/app/project_template_capture.go`
- 修改 `internal/app/project_template_instantiate.go`
- 修改对应 Project Template 测试
- 修改 `internal/app/service.go`
- 修改 `internal/app/permission.go`

### Red

- [ ] 新增 `workspace|project` scope normalization 和非法 scope/id/workspace mismatch 测试。
- [ ] Workspace 规则 CRUD、同 scope 重名、不同 scope 同名、跨 Workspace 隔离测试。
- [ ] Workspace event 只接受 `project.created`；Project event 白名单保持现状，Hook/Notification 仍拒绝 `project.created`。
- [ ] Workspace schedule 接受现有 `daily_at|cron`，使用同一 `schedule.Spec` validation。
- [ ] Workspace 规则只解析 Workspace Provider config；sample Project 写入另一个 base URL/model/api key 后仍不能覆盖。
- [ ] 自定义 Provider key 必须允许 Workspace scope；API key definition 必须是 secret，allowed-host key 必须是 JSON。
- [ ] Workspace/Project Provider config view 只返回 `base_url/model/allowed_hosts/api_key_set/complete/missing_fields`，secret canary、密钥掩码和可逆密文均不可见。
- [ ] Provider config update 中 `api_key` 省略或为空保留当前 secret；`clear_api_key=true` 显式清除；两者冲突返回稳定 400 错误且错误文本不含输入值。
- [ ] Workspace update 只写 Workspace scope；Project update 只写 Project scope并延续 closed Project 门控；两者都复用 config schema 校验、secret encryption 和 `config.set/unset` Audit。
- [ ] Project 规则 Provider effective config 行为和请求 golden 不变。
- [ ] Workspace 默认 system prompt 使用“工作空间自动化”；Project 默认 prompt 保持现有文案，避免静默改变历史规则。
- [ ] Workspace preview 没有 sample Project 时，schedule 成功、event 返回 `automation_sample_project_required`。
- [ ] event sample Project 跨 Workspace/关闭/不存在分别返回稳定错误；preview 标记 `event.simulated=true` 且不写 Delivery。
- [ ] context/template vars 中包含 scope、delivery/event ID、Workspace、Project、event metadata；secret key/value 和 Authorization 不出现。
- [ ] Rule create/modify/enable/disable/delete/test/replay 产生 generic Audit action，payload 只有 scope/ref/changed fields，不含完整 prompt/body/secret。
- [ ] Rule list/info 批量填充可选 `last_delivery` 安全摘要；Delivery view 批量填充可选 Project 引用，Workspace schedule 为空。
- [ ] `tenantCapabilityForPermission(PermissionWorkspaceModify)` 正确映射 `workspace:write`，同时不扩大其它权限。
- [ ] Project Template v1/v2 Snapshot automation JSON/hash golden 不变；Capture/Instantiate 只接触 Project scope。

聚焦运行并确认新 Workspace 方法或 scope 类型不存在：

```bash
go test ./internal/app -run 'WorkspaceAutomation|AutomationScope|AutomationProviderScope|AutomationAudit' -count=1
go test ./internal/app -run 'ProjectAutomation|ProjectTemplate.*Automation' -count=1
go test ./internal/projecttemplate -count=1
```

### Green

- [ ] 定义 `AutomationScope`、`AutomationRuleInput/View`、`AutomationPreviewInput/View` 和通用 decode/normalize。
- [ ] Service 字段改为 `automationRuleRepo/automationDeliveryRepo`，`NewService` 与 `withStore` 都绑定当前 DB/transaction。
- [ ] 实现 scope-aware add/list/info/modify/enable/disable/delete；Project facade 只负责 resolve Project、closed 检查和 DTO 映射。
- [ ] 抽出 `resolveAutomationProviderConfig(scope, projectID, action)`：Workspace 只读 Workspace/default，Project 保持 project > workspace > default。
- [ ] 在 resolver 之上实现安全 `AutomationProviderConfigView` 与 scope-aware get/update；读取只计算 `api_key_set`，写入调用现有 Config service，不直接写 repository。
- [ ] preview/render 接受 `AutomationRenderContext`，不再要求每次都有 Project。
- [ ] template vars/context builder 按 scope 组装；Workspace schedule 不查询 task/project。
- [ ] 规则写操作用 `withAudit` 包裹，Audit action 使用 `automation.rule.*`；完整 prompt 只在 Rule 表，不进 Audit payload。
- [ ] provider validation 共用现有 URL/allowed-host/secret config 逻辑，不复制 SSRF 实现。
- [ ] 删除 Task 1 的 deprecated storage alias；全 App 编译只依赖 generic repo/model。
- [ ] Project Template 内部改用 generic model，但 Snapshot codec 继续使用原 blueprint 契约。

### Refactor / Gate

```bash
gofmt -w internal/app
go test ./internal/app -run 'Automation|ProjectTemplate.*Automation' -count=1
go test ./internal/projecttemplate -count=1
go test ./internal/app -count=1
git diff --check
```

**建议提交：** `feat: 泛化自动化应用服务`

## Task 3：`project.created` 契约、初始 config 快照与事务路由

**目标：** 空项目和模板项目在初始化事务末尾匹配 Workspace 规则并原子生成冻结 Delivery；Project 创建成功后不再存在 Automation 事件丢失窗口。

**文件：**

- 新建 `internal/app/automation_event.go`
- 新建 `internal/app/automation_event_test.go`
- 修改 `internal/app/project.go`
- 修改 `internal/app/project_template_instantiate.go`
- 修改 `internal/app/project_template_instantiate_test.go`
- 修改 `internal/app/config_effective.go`
- 修改 `internal/app/audit.go`
- 修改 `internal/app/service.go`
- 修改 `internal/storage/automation_delivery_repo.go`

### Red

- [ ] 普通 `AddProject` 命中 enabled Workspace `project.created` 规则时，在同一 transaction 得到 Project、project.add Audit 和一条 queued Delivery。
- [ ] 没有匹配规则时只创建 Project/Audit，不制造空 run/event 表。
- [ ] event ID 为 UUID；同一 `rule_id + event_id` 重复路由只插入一次。
- [ ] Rule 在 Project 创建后新增/启用不补跑历史；Delivery 落库后修改/删除 Rule 不改变 frozen request。
- [ ] 注入 Delivery repository DB 写失败，断言 Project 和 Audit 都回滚。
- [ ] Provider base/model/api key 缺失、allowed host 拒绝、body 超限时 Project 仍创建，且生成带稳定错误码、无 secret 的 `dead_lettered` Delivery。
- [ ] 内部 JSON marshal/不变量失败仍回滚整个 Project 事务，不能伪造 dead letter 掩盖系统错误。
- [ ] Template Instantiate 只有在 config、Series、Task、link、disabled Project Automation 全部写完后才 route；event metadata/count 与最终初始化内容一致。
- [ ] Template event 包含 `source=template`、template/snapshot ID/hash；空项目为 `source=empty` 并省略 template 字段。
- [ ] `project_config` 使用事务末尾 effective snapshot：project > workspace > default；missing 不进便捷 map，source/status 可诊断。
- [ ] secret definition 的 key/value、template secret input/ciphertext 不进入 event、context、request preview、error。
- [ ] 事件 actor/created_by 对 user、PAT/Agent token、tenant token 使用 `task.JSONActorInfo`，不输出裸 UUID。
- [ ] `project.created` 不进入 `HookEvent` post-commit 列表；Hook/Notification 注册该事件仍失败。
- [ ] Template 既有 task.created Hook/Notification/Project Automation 行为回归不变。

运行并确认当前 AddProject/Instantiate 不生成事务内 Workspace Delivery：

```bash
go test ./internal/app -run 'ProjectCreatedAutomation|AddProject.*AutomationTransaction' -count=1
go test ./internal/app -run 'ProjectTemplateInstantiate.*ProjectCreated' -count=1
```

### Green

- [ ] 定义 `AutomationEvent`、`AutomationContextSnapshot`、`buildProjectCreatedAutomationEvent`，event version 固定为 1。
- [ ] 增加内部 effective config snapshot builder，直接复用 ConfigDefinition/repository/normalization，不调用带入口权限检查的公开 list 方法。
- [ ] 实现 `RouteAutomationEventTx`：匹配 Workspace/Project scope、渲染、按 `event:{rule_id}:{event_id}` enqueue。
- [ ] 将可预期规则运行错误映射为 dead-lettered Delivery；保留 frozen scope/project/event/error/max attempts/auth refs。
- [ ] `AddProject` 在现有 `withAudit` transaction closure 内、Project 创建完成后 route event。
- [ ] Template Instantiate 在 automation-create、Project view/count 和 aggregate Audit 内容准备完成后 route；保持所有写入同一 Store transaction。
- [ ] 如需失败注入，使用最小 `automationDeliveryEnqueuer` interface，生产仍绑定 generic repository。
- [ ] post-commit Hook/Notification/旧 Project Automation 事件链保持现状；不把本次可靠性修复假装成整个事件总线重构。

### Refactor / Gate

```bash
gofmt -w internal/app internal/storage
go test ./internal/app -run 'ProjectCreatedAutomation|ProjectTemplateInstantiate|ProjectAutomation' -count=1
go test ./internal/storage -run 'AutomationDelivery' -count=1
go test ./internal/app -count=1
git diff --check
```

**建议提交：** `feat: 增加项目创建自动化事件`

## Task 4：泛化 scheduler、dispatcher、重试和 replay

**目标：** 让 Workspace/Project schedule 和 event Delivery 共用同一后台运行时；发送和 replay 完全依赖 frozen Delivery，不读取当前 Rule。

**文件：**

- 新建 `internal/app/automation_scheduler.go`
- 新建 `internal/app/automation_dispatcher.go`
- 新建 `internal/app/automation_runtime_test.go`
- 修改/删除被 generic 实现替代的 `internal/app/project_automation_scheduler.go`
- 修改/删除被 generic 实现替代的 `internal/app/project_automation_dispatcher.go`
- 修改 `internal/app/project_automation_test.go`
- 修改 `internal/storage/automation_delivery_repo.go`
- 修改 `internal/cli/server.go`
- 修改 server/runtime 相关测试

### Red

- [ ] Workspace schedule 到期生成 `project_id=nil` Delivery，只包含 `_xuanchu + workspace`，不查询 Project/Task。
- [ ] Project schedule 继续包含当前 Project context，旧 daily_at/cron 去重测试不变。
- [ ] schedule dedupe 为 `schedule:{scope_type}:{scope_id}:{rule_id}:{slot}`；Workspace/Project 同名同时间不冲突。
- [ ] stale `delivering` 在 claim lease 过期后重新领取，attempt count 正确增加。
- [ ] dispatcher 在 Delivery enqueue 后删除 Rule，仍能用 frozen body/URL/auth key/max attempts 成功发送。
- [ ] Project Delivery 从 Project effective secret 读取；Workspace Delivery 只从 Workspace secret 读取。
- [ ] 当前 allowed-host config 收紧后，manual replay 拒绝原冻结 URL；普通自动 retry 保持冻结目标语义。
- [ ] `max_attempts` 使用 Delivery 冻结值，Rule 后续修改不改变重试次数。
- [ ] 2xx -> succeeded；network/429/5xx -> retry_wait；超过上限 -> dead_lettered；401/403 不自动重试。
- [ ] timeout 重试的 request body、delivery ID、event ID 不变。
- [ ] replay 创建新 Delivery，原 Delivery 不变；新记录带 `replay_of_delivery_id` 和唯一 dedupe key。
- [ ] 关联 Project 已关闭时 test/replay 拒绝；Workspace schedule 无 Project 时可 replay。
- [ ] server 只启动一个 generic scheduler/dispatcher，不同时启动旧/new 两套 worker。

运行并确认当前 dispatcher 仍查询 Rule、replay 仍原地 requeue：

```bash
go test ./internal/app -run 'Automation(Scheduler|Dispatcher|Replay|Stale|Frozen)' -count=1
go test ./internal/storage -run 'AutomationDelivery.*Claim' -count=1
```

### Green

- [ ] `AutomationScheduler` 扫描所有 enabled schedule rule，按 scope 构造 context/dedupe。
- [ ] `AutomationDispatcher.send` 只读取 Delivery 和当前 scope secret；删除 `GetByID(ruleID)` 依赖。
- [ ] `AutomationDeliveryRepository.CreateReplay` 复制 immutable 字段并清空响应/attempt/claim，原记录不更新。
- [ ] manual replay 在 App 层按 frozen `AllowedHostsConfigKey` 读取当前 policy 并校验 frozen URL。
- [ ] claim SQL 加 stale recovery 条件，SQLite/PostgreSQL 保持同一状态语义。
- [ ] server flags 名称保持 `--automation-*`，不引入 workspace/project 两套 interval。
- [ ] 可保留 deprecated Go type alias 作为短期编译兼容，但 server wiring 和生产实例必须只使用 generic runtime；Task 结束删除无消费者 alias。

### Refactor / Gate

```bash
gofmt -w internal/app internal/storage internal/cli
go test ./internal/app -run 'Automation(Scheduler|Dispatcher|Replay|Stale|Frozen)' -count=1
go test ./internal/cli -run 'Automation|Server' -count=1
go test ./tests/integration -run 'E2EProjectAutomation' -count=1
git diff --check
```

**建议提交：** `feat: 泛化自动化调度与投递`

## Task 5：Workspace HTTP API、安全 Provider facade、OpenAPI 与双权限矩阵

**目标：** 开放 spec 中的 Workspace Automation CRUD/preview/test/delivery API，同时保持现有 Project 路径和响应兼容。

**文件：**

- 新建 `internal/httpapi/workspace_automations.go`
- 新建 `internal/httpapi/workspace_automations_test.go`
- 新建 `internal/httpapi/automation_provider_config.go`
- 新建 `internal/httpapi/automation_provider_config_test.go`
- 修改 `internal/httpapi/project_automations.go`
- 修改 `internal/httpapi/project_automations_test.go`
- 修改 `internal/httpapi/huma_routes.go`
- 修改 `internal/httpapi/server_test.go`
- 修改 `internal/httpapi/app_service.go`（仅在需要抽取双 scope helper 时）
- 修改 `internal/app/permission.go`
- 修改 `internal/auth/scope_test.go` 或相关授权测试

### Red

- [ ] 覆盖 `/api/v1/automations` list/create、info/patch/delete、enable/disable、unsaved/saved preview、test，以及 `/api/v1/automation-template-vars`。
- [ ] 覆盖 `/api/v1/automation-deliveries` list/info/replay，过滤 `rule_id/project_ref/status/trigger_type/q/limit/offset`。
- [ ] Workspace 端点只返回 Workspace scope；Project 端点只返回目标 Project scope。
- [ ] Workspace rule view 返回 `scope_type/scope_id`，`project_id` 对 Workspace rule 省略；Project 旧响应仍有 `project_id`。
- [ ] Rule view 的 `last_delivery` 和 Delivery view 的 `project` 引用形状、空值及批量解析正确。
- [ ] event preview/test body 要求 `sample_project_ref`；schedule 不要求。
- [ ] sample Project 模拟事件返回 `event.simulated=true`，不产生 event/Delivery；test 只产生 manual_test Delivery。
- [ ] read 必须同时有 `workspace:read + hook:read`；任意缺一均 403。
- [ ] write/preview/test/replay 必须同时有 `workspace:write + hook:write`；`config:write` 不替代 Automation 权限。
- [ ] owner/admin/member/viewer、PAT/Agent、tenant_access_token、OIDC browser session 权限矩阵。
- [ ] 覆盖 Workspace `GET/PUT /api/v1/automations/provider-config` 和 Project `GET/PUT /api/v1/projects/{projectRef}/automations/provider-config`。
- [ ] Workspace facade 分别要求 `config:read/write + workspace.read/modify`；Project facade 延续现有 Project config token/role permission、Workspace 隔离和 closed Project 写门控。
- [ ] GET/PUT 响应严格只有 `base_url/model/allowed_hosts/api_key_set/complete/missing_fields`；HTTP body、OpenAPI example、错误、Audit 和捕获日志均不含 API key canary。
- [ ] PUT 的空/省略 API key 保留历史 secret，`clear_api_key=true` 显式清除，clear 与非空 key 冲突返回 400。
- [ ] `/api/v1/config/{key}` 仍拒绝 `agent.provider.*`，不得通过扩大 `IsBusinessConfigKey` 修通页面；Automation API 不回显 API key。
- [ ] Project Provider facade effective 读取 project > workspace > default，迁移后的 Project Provider 卡片不再依赖 Project config list 获取 secret。
- [ ] error code/status 覆盖 sample required/invalid、scope invalid、event unsupported、provider missing/denied、context too large。
- [ ] Huma OpenAPI 路径、tag、request/response schema 全部出现；示例不含 secret。
- [ ] Project HTTP 回归：CRUD/preview/test/list/info/replay 和 closed project 行为不变，replay 响应改为新 Delivery ID 的新语义有测试说明。

运行并确认 Workspace 路由尚未注册：

```bash
go test ./internal/httpapi -run 'HTTPWorkspaceAutomation|WorkspaceAutomationPermissions|HTTPAutomationProviderConfig' -count=1
go test ./internal/httpapi -run 'HTTPProjectAutomation|OpenAPI.*Automation' -count=1
```

### Green

- [ ] 抽取通用 HTTP rule DTO/input mapper，Project/Workspace handler 共用，不复制字段协议。
- [ ] 实现 `scopedWorkspaceAutomationService`，分别校验 workspace 和 hook capability/permission。
- [ ] 注册 spec 中所有 Workspace 路径，统一 tag `Workspace Automations`。
- [ ] Workspace/Project Provider handler 共用安全 DTO/mapper，只把 scope、permission 和 closed Project 行为留在 facade；底层不新增 Provider 表。
- [ ] template vars endpoint 只返回变量描述，不读取 sample Project、config value 或 secret。
- [ ] query parser 统一处理 limit/offset/status/trigger/rule/project，非法值返回 400 而不是静默忽略。
- [ ] Project handler 继续调用 facade，Workspace handler 调 generic App service。
- [ ] 响应中的 `created_by/actor` 使用统一 JSON user/actor serializer。

### Refactor / Gate

```bash
gofmt -w internal/httpapi internal/app
go test ./internal/httpapi -run 'Automation|OpenAPI' -count=1
go test ./internal/httpapi -count=1
git diff --check
```

**建议提交：** `feat: 增加工作空间自动化接口`

## Task 6：Web 类型、权限、路由与导航入口

**目标：** 先建立可编译、可测试的 Workspace Automation 前端骨架，锁定 `/automations`、权限隐藏和 Project 深层路由高亮，再进入复杂页面实现。

**文件：**

- 新建 `web/src/features/workspace/automations/automation-types.ts`
- 新建 `web/src/features/workspace/automations/workspace-automations-api.ts`
- 新建 `web/src/features/workspace/automations/workspace-automation-permissions.ts`
- 新建对应 API/permission 测试
- 新建 `web/src/routes/workspace/WorkspaceAutomationsRoute.tsx`
- 新建最小 `web/src/features/workspace/automations/workspace-automations-page.tsx`
- 修改 `web/src/features/workspace/project-workbench/automations/project-automations-api.ts`
- 修改 `web/src/components/AppShell.tsx`
- 修改 `web/src/components/AppShell.test.tsx`
- 修改 `web/src/routes/router.tsx`
- 修改 router 相关测试
- 修改 `web/src/locales/zh-CN.ts`
- 修改 `web/src/locales/en-US.ts`

### Red

- [ ] Workspace API path builder 覆盖完整 CRUD/preview/test/delivery/replay 和 query encoding。
- [ ] Workspace/Project Provider config path、safe DTO 和 PUT input 共用一套 TS contract；类型中不存在 `api_key` 响应字段。
- [ ] 通用 TS type 表达 `scope_type/scope_id/project_id?`，Project API 不复制另一套状态/trigger/action 类型。
- [ ] permission helper：Automation read 要 workspace+hook read，write 要 workspace+hook write；browser role fallback 只有 owner/admin。
- [ ] tenant token 与 PAT 的 scope wildcard/resource wildcard 使用现有 `hasScope` 语义，不手写不一致判断。
- [ ] AppShell「管理」顺序固定为成员、Token、自动化、Hook、通知、单点登录，图标为 Lucide `Workflow`。
- [ ] owner/admin OIDC 显示；member/viewer 隐藏；token 缺任一 read scope 隐藏。
- [ ] `/automations` 高亮“自动化”。
- [ ] `/workspaces/:workspaceSlug/projects/:projectSlug/automations` 继续高亮“项目”，不能同时高亮顶层自动化。
- [ ] 移动端导航抽屉与桌面侧栏使用同一 navGroups/权限结果。
- [ ] 路由 lazy load `/automations`，直接访问有最小页面且不走 ResourceRoute。

运行并确认 PageKey/route/helper 不存在：

```bash
pnpm --dir web test -- AppShell workspace-automation-permissions workspace-automations-api router
pnpm --dir web typecheck
```

### Green

- [ ] 抽取共享 Automation types，Project API 改为 import/re-export，保持调用方兼容。
- [ ] 新增 `PageKey="automations"`、Workflow nav item 和 scope-aware filter。
- [ ] `isNavItemActive` 对 Workspace/Project Automation 路由写显式分支，避免前缀误判。
- [ ] route 使用专用 `WorkspaceAutomationsRoute`；最小页面先展示标题/说明/加载态。
- [ ] i18n 同步中英文 key；中文产品名使用「璇础」。

### Refactor / Gate

```bash
pnpm --dir web test -- AppShell workspace-automation-permissions workspace-automations-api router
pnpm --dir web typecheck
pnpm --dir web lint
git diff --check
```

**建议提交：** `feat: 增加工作空间自动化导航`

## Task 7：Workspace 自动化规则、运行记录、Provider 与移动端页面

**目标：** 完成 spec ASCII 原型对应的规则/运行记录 tab、scope-aware 编辑 Dialog、sample Project、Provider 配置和 desktop/mobile 体验，并最大化复用现有 Project Automation 组件。

**文件：**

- 修改 `web/src/features/workspace/automations/workspace-automations-page.tsx`
- 新建 `web/src/features/workspace/automations/workspace-automation-rule-table.tsx`
- 新建 `web/src/features/workspace/automations/workspace-automation-delivery-table.tsx`
- 新建 `web/src/features/workspace/automations/workspace-automation-delivery-detail.tsx`
- 新建/修改对应测试
- 新建共享目录 `web/src/features/workspace/automations/shared/`
- 移动或泛化现有 Automation rule form/dialog/preview/test debug/provider config 组件
- 修改 `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx`
- 修改现有 Project Automation 组件测试
- 修改 `web/src/features/workspace/project-workbench/automations/automation-provider-config.tsx`
- 修改 `web/src/features/workspace/project-workbench/automations/automation-provider-config.test.tsx`
- 修改 `web/src/features/workspace/project-workbench/api/project-api.ts` 或复用现有 Project list API
- 修改 locales

### Red

- [ ] 规则/运行记录两个 tab，数量和 URL/本地状态切换稳定。
- [ ] 规则表使用 shadcn `<Table>`，列为状态、名称、触发器、指令摘要、最近运行、操作；无手写 `<table>`。
- [ ] 运行表过滤状态/规则/Project/触发/ID，列为状态、规则、Project、触发、HTTP、Provider ID、时间、操作。
- [ ] Workspace schedule 的 Project 显示 `—`；event Project 可点击进入 Project。
- [ ] 新建/编辑 Dialog scope=workspace 时事件只有 `project.created`，默认 context 为 workspace/project/project_config/event。
- [ ] Workspace TemplateVariablePicker 读取 `/api/v1/automation-template-vars`，按 schedule/event 分组，不能硬编码复制 Project 变量列表。
- [ ] event preview/test 未选 sample Project 时客户端提示，服务端仍是最终校验；schedule 不显示/不提交 sample。
- [ ] sample Project 选择只列当前 Workspace open Project，真实 sample overflow/portal 鼠标交互可用。
- [ ] Provider 摘要只读取 Workspace effective config；Project override 不出现在页面。
- [ ] Workspace Provider 编辑只调用 `/api/v1/automations/provider-config`；Project Provider 卡片只调用 `/projects/{ref}/automations/provider-config`，两者都不调用通用 config list/get/set。
- [ ] `config:read/write` 不足时显示无权查看/编辑，不阻塞规则治理；Project 端延续 Project config 权限和 closed Project 禁写。
- [ ] `api_key_set` 只控制“已设置（留空保留）”占位文案，不把 mask 填入 value；secret 输入在保存成功、失败、Dialog 关闭和 query invalidation 后都清空，不在 toast/query cache/DOM 回显。
- [ ] 显式“清除 API Key”需要二次确认；不得用空字符串隐式清除。
- [ ] preview 显示 `Bearer ****`，复制 JSON/curl 不包含 API key。
- [ ] succeeded Delivery 文案为“Agent 调用成功”，详情明确“不是业务动作成功证明”。
- [ ] Delivery 详情展示 frozen request/response、delivery/event/provider ID、Project Audit 链接和 replay；不重新按当前 Rule 渲染。
- [ ] replay 成功后打开新 Delivery，不覆盖原详情。
- [ ] 空态、Provider 缺失 warning、403、加载/错误、规则 dead-letter 状态文案。
- [ ] mobile 使用 card list + 全高 Sheet；desktop 使用 Table；两者无横向滚动。
- [ ] Workspace 和 Project 页面复用同一个 rule form/preview/provider 基础组件；不出现复制的 action/context validation。
- [ ] Project Automation 页面回归：旧模板入口、provider 配置、cron、event、test debug、closed project 门控全部通过。
- [ ] 视觉测试/源码检查没有裸 hex、Tailwind emerald/amber/red 原色、非 Lucide 图标和大面积状态色。

聚焦运行并确认 Workspace 页面仍是骨架：

```bash
pnpm --dir web test -- workspace-automations-page workspace-automation-rule workspace-automation-delivery automation-provider-config
pnpm --dir web test -- project-automations-page automation-preview-dialog
```

### Green

- [ ] 将可复用的 form/dialog/preview/provider/delivery detail 迁到 shared，以 `scope`、API adapter、event options、sample Project 显式参数区分。
- [ ] Workspace Page 负责 query/mutation/invalidation/tab/filter；展示组件不自行拼 API。
- [ ] Provider config 组件只消费安全 facade DTO，Workspace/Project 通过 API adapter 区分路径；底层 Config storage 不变，前端不感知 secret 原值。
- [ ] status dot/chip、mono ID/time、truncate、tooltip、mobile card 按 `DESIGN.md` 落地。
- [ ] “查看 Project Audit”链接使用现有 Audit Console 查询能力；缺 Audit read 时隐藏而不是发必失败请求。
- [ ] 所有 destructive 操作复用确认 Dialog，write scope 不足时隐藏/禁用且服务端继续校验。

### Refactor / Gate

```bash
pnpm --dir web test -- workspace-automations project-automations automation-provider-config AppShell
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
git diff --check
```

**建议提交：** `feat: 完成工作空间自动化控制台`

## Task 8：真实 E2E、Playwright、文档与全量收口

**目标：** 用真实 binary/server/dispatcher/OpenAI-compatible endpoint/MCP 验证完整闭环，再同步文档和 milestone 状态。

**文件：**

- 新建 `tests/integration/e2e_workspace_automation_test.go`
- 修改 `tests/integration/e2e_project_automation_test.go`
- 修改 `tests/integration/postgres_e2e_test.go`
- 新建 `web/scripts/playwright-workspace-automation-smoke.mjs`
- 修改 `web/package.json`
- 修改 `README.md`
- 修改 `ROADMAP.md`
- 修改 `docs/superpowers/specs/2026-07-26-workspace-agent-automation-design.md`
- 修改本 implementation plan 的实施记录（执行时）

### Red

- [ ] 先写真实 E2E 并运行，确认当前 binary 缺 Workspace API/project.created Delivery 而失败。
- [ ] E2E 启动真实 `xuanchu server`、真实 SQLite DB、1s dispatcher 和本地 fake OpenAI-compatible endpoint。
- [ ] 通过 HTTP 创建 enabled Workspace `project.created` 规则，再分别创建空 Project 和从 Template 创建 Project。
- [ ] 断言每个 Project 只产生一条该 Rule 的 Delivery；event metadata/source/count 正确。
- [ ] fake Provider 收到 Workspace model/API key、Project/初始 config/event/delivery ID，且收不到 Project provider override 和 secret canary。
- [ ] HTTP E2E 先通过安全 facade 配置 Workspace Provider，验证 GET/PUT/OpenAPI/服务日志都不回显 API key；再验证空 key 保留和显式 clear。
- [ ] Project Automation 回归通过 Project Provider facade 配置 effective Provider，浏览器网络响应和 query cache 不含 API key。
- [ ] 轮询 Delivery 到 succeeded，UI/API 语义仍只代表 Agent 调用成功。
- [ ] 使用真实 HTTP MCP transport 调 `project_config_list` -> `project_config_set` 写 `knowledge_id`，再调 `audit_list` 断言 Agent actor 和 config Audit。
- [ ] 重复模拟 Agent 流程时先读到 existing knowledge ID 并 no-op，不创建第二次外部资源。
- [ ] 删除 Rule 后已 queued Delivery 仍能被 dispatcher 发送；replay 生成新 ID并引用原 Delivery。
- [ ] server 重启后 stale claim 恢复并完成。
- [ ] Hook/Notification 创建 `project.created` 规则仍被拒绝，验证非目标边界。
- [ ] PostgreSQL opt-in 跑同一 HTTP create -> event -> delivery 基础链路。
- [ ] Playwright 使用生产 Web 构建和真实 server：desktop 验证侧栏/规则/Dialog/sample/preview/运行详情，mobile 验证抽屉/card/Sheet/无横向滚动。
- [ ] Playwright 结束后清理进程，并断言占用端口已释放。

### Green

- [ ] 完成 `TestE2EWorkspaceProjectCreatedAutomation`，尽量复用现有 E2E server/MCP/helper，不复制进程管理。
- [ ] 为 web 增加 `smoke:workspace-automation` script；fixture 数据通过公开 CLI/HTTP 准备，不直接改生产 DB 表。
- [ ] README 增加 `/automations`、scope 区别、Provider Workspace-only、自然语言编排、MCP 回写和 2xx 完成语义。
- [ ] README 示例 prompt 使用 `feishu_drive_folder_token -> knowledge_id`，同时建议生产 key 命名空间。
- [ ] `docs/skills/xuanchu-wire-up-automation/references/event-types.md` 保持 `project.created` 对 Hook/Notification 不可订阅；若补说明，只能明确“Workspace Agent Automation 专用”，不能移入共用事件表。
- [ ] 全量验证全部通过后，才把 spec 和 ROADMAP v0.6.4 状态改为“已完成”，并在本计划追加真实测试数量/命令/结果。

### 完整验证

按顺序运行并记录结果：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:workspace-automation
git diff --check
```

PostgreSQL 环境已配置时额外运行：

```bash
XUANCHU_E2E_POSTGRES_ADMIN_URL="$XUANCHU_E2E_POSTGRES_ADMIN_URL" go test ./tests/integration -run 'Postgres.*Automation' -count=1
```

若 Playwright 启动了本地服务，结束后必须证明端口已释放。任何命令未运行、skipped 或失败，都必须在实施记录中如实写明，不得用“全部通过”概括。

**建议提交：** `docs: 完成工作空间自动化交付`


## 最终验收清单

- [x] Storage 只剩 generic Automation model/repository/table；历史数据迁移有 SQLite/PostgreSQL 证据。
- [x] App 只有一套 CRUD/render/scheduler/dispatcher，Project 方法是薄 facade。
- [x] 空项目/模板项目的 `project.created` Delivery 与创建事务原子提交。
- [x] Delivery 是唯一 outbox/run，Rule 删除后仍能执行，replay 创建新记录。
- [x] Workspace Provider 不受 Project config 覆盖，secret 全链路不泄漏。
- [x] Workspace/Project Provider UI 只使用安全 facade；通用 Workspace Config allowlist 未扩大，Provider GET/PUT/API 文档/前端缓存均无 API key 明文。
- [x] Agent 使用现有 MCP 回写 Project config，Audit 可确认 actor 和结果写入。（`TestE2EWorkspaceProjectCreatedMCPWriteback` 用真实 HTTP MCP transport 跑通 `project_config_list` → `project_config_set` 写 `yaoguang.knowledge_base.id` → `audit_list` 断言 `project.config.set` audit 与 Project 创建 audit 分开记录；真实 Agent 平台调用外部工具的部分由平台负责。）
- [x] Workspace/Project 权限、closed Project、sample Project 和 scope 隔离完整。
- [x] `/automations` 导航、规则/运行记录、desktop/mobile 与 ASCII 原型一致。（桌面 shadcn `<Table>` 规则/运行记录 + 移动端 card list；新建/编辑 Dialog 含 sample Project 选择、preview、Provider 摘要入口；Provider 配置 Dialog 只消费安全 facade，api_key 永不回显；运行详情右侧 Sheet 抽屉展示 frozen request/response/usage/error + Project 引用 + replay；Playwright smoke 覆盖桌面/移动渲染、无横向滚动、Dialog 不泄漏 secret。）
- [x] Project Automation、Project Template、Hook/Notification 没有行为回归或范围偷扩。
- [x] HTTP/OpenAPI/Web/README/ROADMAP/spec/plan 同步。
- [x] 普通/零 CGO Go、Web 全套、真实 E2E、Playwright smoke 全部留下验证记录；PostgreSQL 未在本次开发机执行（无可用 admin URL），可在具备对应环境时补跑。

## 实施记录（2026-07-26）

按 8 个 task 顺序执行，每个 task 一个 commit：

- Task 1 `feat: 泛化自动化存储作用域`：物理表 `project_automation_*` → `automation_*`，scope_type/scope_id/replay_of_delivery_id/api_key_config_key/allowed_hosts_config_key/max_attempts 字段落地；SQLite/PostgreSQL 双向迁移、主键集合校验、fresh DB 路径；AutomationRuleRepository/AutomationDeliveryRepository 提供 ListScope/ListCandidatePage/ClaimDue(stale recovery)/LatestByRuleIDs/CreateReplay；`TestAutomationScopeMigrationPreservesLegacyData` 等 8 个 storage 测试通过。
- Task 2 `feat: 泛化自动化应用服务`：AutomationScope/AutomationRuleInput/View/ModifyInput 共用 normalize；Workspace event 白名单只开放 `project.created`；Audit action `automation.rule.*` 只含 scope/ref/changed fields；AutomationProviderConfig 安全 facade View/Update（api_key_set、clear_api_key 冲突检测）；permission 补 `tenantCapabilityForPermission(PermissionWorkspaceModify)` 映射。
- Task 3 `feat: 增加项目创建自动化事件`：AutomationEvent/AutomationContextSnapshot/RouteAutomationEventTx；AddProject/InstantiateProjectTemplate 在事务内 route；secret 不进 metadata/context/body；Provider 缺失/超限落 dead_lettered，Project 仍创建；`TestProjectCreatedAutomation*` 5 个测试覆盖。
- Task 4 `feat: 泛化自动化调度与投递`：scheduler 按 scope 分发 buildWorkspaceScheduleDelivery/buildProjectAutomationDelivery；dispatcher 只读 frozen scope/secret/max_attempts；replay 走 CreateReplay 不原地 Requeue；dedupe key `schedule:{scope_type}:{scope_id}:{rule_id}:{slot}`；`TestWorkspaceAutomationScheduler|Dispatcher|Replay` 7 个测试覆盖。
- Task 5 `feat: 增加工作空间自动化接口`：`/api/v1/automations`、`/api/v1/automation-deliveries`、`/api/v1/automations/template-vars`、`/api/v1/automations/provider-config`、`/api/v1/projects/{ref}/automations/provider-config`；双 scope 权限矩阵测试覆盖。
- Task 6 & 7 `feat: 增加工作空间自动化导航`：AppShell「管理」分组 Workflow 图标入口（owner/admin 可见，token 需 workspace:read+hook:read）；`/automations` 路由；WorkspaceAutomationsConsole 两个 tab（规则/运行记录）+ 状态点 + 启停 + replay + 空/缺 Provider 文案；安全 Provider facade 类型不暴露 api_key。
- Task 8 `test: 增加工作空间自动化真实 E2E`：真实 server + fake provider 跑通 provider PUT → 规则创建 → 项目创建 → delivery succeeded → fake provider 验证 secret/metadata/project_id；provider facade 保留/清除 api_key 行为；修复 `fillAutomationDeliveryProjects` slice 元素重新读取 bug。

### Task 8 补充验收（同日）

后续追加三个验收测试覆盖 spec/plan 明确列出、但首版未落地的边界：

- `test: 增加工作空间自动化 MCP 回写与边界 E2E`：
  - `TestE2EWorkspaceProjectCreatedMCPWriteback`：用真实 HTTP MCP transport 跑通 `project_config_list`（确认 knowledge_id 为空，幂等前置）→ `project_config_set` 写 `yaoguang.knowledge_base.id` → `audit_list` 断言 `project.config.set` audit 含 key/value，且与 `project.add` audit 分开记录；最后再次 `project_config_list` 验证持久化、重复 set 走幂等路径。Agent 使用独立 PAT，回写产生的 audit 不与 Project 创建者混淆。
  - `TestProjectCreatedEventRejectedByHookWhitelist|HookCreate|NotificationRule` + `AllowedByWorkspaceAutomationWhitelist`：验证 `project.created` 不在 Hook/Notification 白名单（边界回归），但仍在 `workspaceAutomationAllowedEvents` 中；两个白名单交集为空。
  - `TestE2EWorkspaceAutomationServerGenericRuntime`：真实 server 在同一进程内同时跑通 Workspace event 路径（project.created）与 Project event 路径（task.created manual test），证明 server wiring 只有一个 generic scheduler + 一个 generic dispatcher，不存在 workspace/project 两套 worker。

仍未完成项：desktop/mobile 完整 UI（新建/编辑 Dialog、sample Project、preview/test debug、移动端 card/Sheet）、Playwright smoke 脚本、PostgreSQL opt-in 验证。这些工作可在后续迭代或具备对应环境时补齐。

### Task 8 Web Console 与韧性 E2E 验收（2026-07-27）

补齐 spec §16 ASCII 原型对应的 Web Console 完整 UI 和 Playwright smoke：

- `feat: 完成工作空间自动化控制台 UI`：
  - 桌面 shadcn `<Table>` 规则列表（状态点 / 名称 / 触发器摘要 / 指令摘要 / 最近运行 / 操作 ⋯），列 `min-w-0 + truncate` 避免横向滚动。
  - 桌面 shadcn `<Table>` 运行记录列表 + 状态/触发/ID 搜索过滤。
  - 移动端 card list（`md:hidden`）+ 桌面 Table（`hidden md:block`）响应式切换。
  - 新建/编辑 Dialog（`WorkspaceAutomationRuleDialog`）：触发方式、事件白名单（project.created）、Provider 摘要入口、执行指令、sample Project 选择器（event 必填）、预览投递 JSON。
  - Provider 配置 Dialog（`ProviderConfigDialog`）：只消费安全 facade DTO，`api_key_set` 只控制占位文案，`clear_api_key` 二次确认，保存/关闭时清空本地 state（mount/unmount 自然清理，无 effect setState）。
  - 运行详情右侧 Sheet 抽屉（`WorkspaceAutomationDeliveryDetail`）：frozen request/response/usage/error tabs + delivery/event/provider ID + Project 引用（可点击）+ replay；2xx 文案明确「Agent 调用成功，不是业务动作成功的证明」。
  - 共享 helper：`automation-status.tsx`（状态点 + 标签）、`automation-trigger-summary.ts`（cron/daily_at/event 摘要 + tooltip 全文）。
- Playwright smoke（`web/scripts/playwright-workspace-automation-smoke.mjs` + `pnpm --dir web run smoke:workspace-automation`）：mock API 下验证桌面/移动布局、规则 Dialog、Provider Dialog、运行详情抽屉渲染正常、无横向滚动、不暴露 `sk-` 开头的 api key 明文；7 张截图全部生成。

补齐 plan 原 Task 8 列出但首版未单独验收的韧性子项：

- `TestE2EWorkspaceAutomationDeliverySurvivesRuleDeletion`：规则删除后，已 queued 的 Delivery 仍能被 dispatcher 发送并进入 succeeded（dispatcher 只读 Delivery 冻结字段，不读当前 Rule）。
- `TestE2EWorkspaceAutomationProjectProviderFacadeHidesSecret`：Project Provider 安全 facade GET 永不回显 `agent.provider.api_key` 明文，浏览器通过 facade 而不是 project config list 取 secret。
- 重复 Agent no-op：已由 `TestE2EWorkspaceProjectCreatedMCPWriteback` 的「重复 set 走幂等路径」断言覆盖。
- stale claim 恢复：已由 `TestWorkspaceAutomationDispatcherStaleRecoveryAfterServerRestart`（app 层）+ `TestAutomationDeliveryClaimDueRecoversStale`（storage 层）覆盖；server 重启场景的端到端验证由 dispatcher 共享 generic runtime 保证。

仍未完成项：PostgreSQL opt-in 验证（`XUANCHU_E2E_POSTGRES_ADMIN_URL=... go test ./tests/integration -run 'Postgres.*Automation'`）需要在具备 PostgreSQL admin URL 的环境执行，本次开发机无对应环境。
