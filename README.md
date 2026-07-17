# 璇础 (Xuán Chǔ) — 面向企业项目与 Agent MCP 的 Taskwarrior 风格任务运行时（Go 版）

`璇`：取自《尚书·舜典》 “在璇玑玉衡，以齐七政”，指北斗星中运转、校准的枢机，象征规则与秩序的核心。
`础`：取自《淮南子·说林训》 “山云蒸，柱础润”，指房屋柱下的基石。汉代《说文解字》注：“础，磶也”，即承载重量的基础。

命令行输出为 `xuanchu`，Xuanchu 是一个用 **纯 Go** 实现的企业任务运行时。它借鉴 Taskwarrior 的 CLI、查询语言、任务字段和 urgency 思路，但产品目标不是做完整 Taskwarrior clone，而是服务企业项目协作和 Agent MCP：

- 单一二进制：同时承担 **本地 CLI / 远程 CLI 客户端 / HTTP API 服务端 / MCP Server** 四种形态
- 嵌入式 Web Admin Console：同一 server 在 `/` 提供普通运维入口，`/admin/login` 提供 server admin bootstrap 入口
- 数据库：**SQLite（GORM + `github.com/glebarez/sqlite`，零 CGO）**，可跨平台交叉编译
- `workspace` 作为企业 / 租户级隔离边界；`project` 表示企业内的真实项目
- 支持多用户、权限、审计、行级隔离，并为 Agent token 和 MCP scope 预留边界
- 支持服务端 Hook、定时通知、事件通知规则、动态 endpoint 和 HTTP request template sink
- 借鉴 Taskwarrior 的命令、查询、recurrence 与 urgency 思路；公开 JSON、数据库和跨入口契约采用璇础原生模型，不承诺 Taskwarrior 兼容

## 详细需求

参见 `docs/requirements.md`。该文档梳理了：

- 上游 Taskwarrior 的数据模型 / CLI 语法 / 报表 / Urgency / DOM / Hook / 同步语义
- 企业 workspace、project、Agent MCP、外部系统触发等扩展需求
- SQLite 表结构草案、CLI 命令分级、MCP 工具 JSON Schema 草案
- 所有结论附参考来源链接

## 状态

README 只描述当前代码已经具备的能力和常用入口。完整 milestone、版本历史和后续计划见 [ROADMAP.md](./ROADMAP.md)；更细的变更历史以 git 记录为准。

## 快速开始

```bash
go build -o xuanchu ./cmd/xuanchu

# 先注册一个项目，再创建第一条任务
./xuanchu project add agentapi name:"AI Agent Platform"
./xuanchu add "Write MCP task docs" project:agentapi +docs due:tomorrow

# 默认只看 pending 任务
./xuanchu list

# 查看详情。1 是列表里的 working-set ID；agentapi-1 是稳定短任务引用 task_slug
./xuanchu info 1
./xuanchu info agentapi-1

# 修改、完成、删除、重新打开都可以用 <target> <action> 写法
./xuanchu agentapi-1 modify priority:H +next
./xuanchu agentapi-1 done
./xuanchu agentapi-1 reopen
./xuanchu agentapi-1 delete
```

`project.slug` 会统一转成小写，只允许 3-10 位 ASCII 英文字母和数字，且必须以字母开头；同一 workspace 内不能重复，跨 workspace 可以重复。带 project 的任务会输出短标识 `task_slug`，格式为 `<projectSlug>-<seq>`，例如 `agentapi-1`。`--json` 输出中也会包含 `"task_slug":"agentapi-1"`。

本地 CLI 的 `target` 可以是列表里的数字 ID、完整 UUID、足够长的 UUID 前缀、`task_slug`，或循环实例的 `occurrence_ref`。HTTP API 和 MCP tool 属于协议入口，接受完整 UUID、已物化任务的 `task_slug` 或 `occurrence_ref`；projected 循环实例只有 `occurrence_ref`。协议入口不会接受本地 working-set 数字 ID。常用命令既支持 `xuanchu <subcommand> ...`，也支持 Taskwarrior 风格的 `xuanchu <target> <action> ...`。

常用全局参数：

```bash
./xuanchu --db ./xuanchu.db list          # 使用指定 SQLite 文件
./xuanchu --data-dir ./data list        # 数据库放到 ./data/xuanchu.db
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable" list
./xuanchu --config /path/to/config.toml list  # 指定 TOML 配置文件
./xuanchu --json list                   # 输出 JSON
./xuanchu --no-color list               # 关闭颜色
./xuanchu --no-context list             # 本次命令忽略 active context
./xuanchu --workspace dajee list        # 本次命令切到 dajee workspace
./xuanchu --version                     # 显示版本号
```

默认数据库路径是 `~/.local/share/xuanchu/xuanchu.db`。配置优先级按“本次命令参数优先”理解即可：CLI flag / `rc.*` 覆盖 > 环境变量 > SQLite meta > `xuanchu.toml` > 默认值。

`--config` 可以指定任意 TOML 文件路径（环境变量 `XUANCHU_CONFIG` 等价），不指定时仍从 XDG 默认路径加载。版本号可通过 `--version` 查看，`go install` 构建的版本会从 git 信息自动推断。

## PostgreSQL

Xuanchu 支持 PostgreSQL 作为数据库后端，通过 `--db-url` 指定连接字符串：

```bash
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable" server
```

也可以通过环境变量或 TOML 配置：

```bash
export XUANCHU_DB_URL="postgres://user:pass@localhost:5432/xuanchu"
./xuanchu list
```

```toml
[database]
url = "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable"
```

`--db-url` 和 `--db` 互斥。未指定 `--db-url` 时使用 SQLite（默认行为不变）。

## Web Console

`xuanchu server` 默认在 `/` 提供嵌入式 Web Console。Console 登录支持 PAT、Agent token 和 `tenant_access_token`；其中 `tenant_access_token` 作为 workspace 系统身份进入。token 只保存在当前浏览器 tab 的 `sessionStorage`，后续请求继续走 `/api/v1/*`，不绕过 token scope、workspace allowlist 或 project allowlist；自然人 PAT / Agent token 仍会校验 workspace membership。

v0.5.0 起，Web Console 还支持 workspace 级 OIDC 单点登录（SSO）。workspace owner 或具备 `workspace:write` 的 tenant actor 在「单点登录」配置页填入 yaoguang IdP 的 issuer / client 信息并触发通讯录同步后，成员可通过登录页的「OIDC 单点登录」入口完成浏览器登录。OIDC 登录使用服务端可撤销的 opaque session cookie（`xuanchu_session`，HttpOnly），写操作必须同时携带 CSRF token（`xuanchu_csrf` cookie + `X-Xuanchu-CSRF` 头）。CLI、Remote Client、MCP 和外部自动化仍只接受 Bearer token，不接受 browser session。SSO `client_secret` 在落库前经 AES-256-GCM envelope 加密，密钥从 TOML 配置 `[security].config_secret_key`（base64 编码的 32 字节）读取。通讯录同步复用 OIDC `client_id` + `client_secret`，通过 `client_credentials` grant 自动向 IdP 换取访问 token，无需单独配置 directory token。为防止 SSO 用户用 PAT 绕过 SSO 登录 Console，通过 SSO browser session 在 `/tokens` 创建的 PAT / Agent token 会标记 `web_login_disabled`，不能再用于 Web Console 登录页登录（登录页会明确提示「请使用 SSO 登录」）；但这些 token 在 HTTP API、HTTP MCP、远程 CLI、stdio MCP 等场景仍正常可用。tenant token 与 CLI/Bearer 创建的 PAT 不受影响。

v0.5.x 的「Web Console 能力桥接」把浏览器控制面从单项目工作台扩展为「任务协作入口 + 治理控制台」：

- **侧栏归位**：左侧栏底部固定展示当前登录身份（actor / role / token type / workspace / 风险状态 / 退出 / 返回超管），主内容顶栏改为页面上下文，不再放 actor/token 信息。
- **用户首页**（`/`）：登录后首先展示「我的今日」，聚合当前用户已开始、逾期、今日到期和高优未完成任务，并可直接开始、停止或完成；随后展示权限内的项目关注和管理员明确放到首页的工作区信息。身份、Token、Scope、失败投递和审计不再占用首页，分别留在侧栏或治理专页。任务与项目计数由 `GET /api/v1/home` 基于权限内全量数据聚合，不再从 `limit=200` 的分页列表推断；tenant/system actor 使用明确的系统身份降级首页，不伪造个人任务。
- **「我的任务」入口**（`/my-tasks`）：跨项目聚合分配给当前用户的任务，提供「全部 / 已开始 / 今日到期 / 逾期 / 无截止」预设视图、状态/优先级/排序 toolbar 和负载摘要。系统身份进入时显示空状态。
- **任务详情解耦**：新增 `/tasks/$taskRef` 入口，从「我的任务」可直接进入任务详情；详情页 `projectSlug` 可选，缺失时从 task.project 兜底并隐藏「返回项目」入口。
- **已删除任务可见性**：项目工作台 toolbar 的状态筛选支持 `deleted`，deleted 任务行灰显并禁用写控件（恢复能力待后端支持）。
- **Hook 控制台**（`/hooks`）：从只读 DataTable 升级为带 CRUD、enable/disable、行内投递历史和重放的运维控制台。
- **审计日志控制台**（`/audit`）：支持按 project/limit 服务端查询，actor/action/target 在当前结果上二次筛选，并支持导出 CSV。普通成员和 viewer 现在也能读取 workspace 全量审计（后端 `audit.read` 角色权限对齐）。
- **项目设置页**（`/projects/$slug/settings`）：集中管理项目状态流转、配置值、配置定义和项目备注。配置值 tab（`/config`）展示 project effective 配置，读取顺序为 project 显式值 > workspace 值 > schema 默认值，secret 值遮掩、workspace-only key 只读、可「恢复继承」；配置定义 tab（`/definitions`）在 project 上下文里管理 workspace 中允许 project scope 的 `ConfigDefinition`。项目列表行操作菜单支持快速归档/取消/恢复。
- **配置定义控制台**（`/settings`）：workspace 级 `ConfigDefinition` 控制面，管理哪些 config key 可被写入、值的类型/作用域/默认值/枚举，以及 `show_on_console_home` 开关。已有配置值时收紧 type/scope/enum 变更（返回 `config_definition_type_locked` / `config_definition_scope_locked` / `config_definition_enum_locked`）；删除定义走 `purge=true` 两段确认。Web Console 首页据此展示面向普通成员的「工作区信息」；没有可展示值时整块隐藏。
- **成员外部身份**：成员详情页支持绑定/解绑 external-id（`POST/DELETE /users/{ref}/external-ids`），与 SSO 通讯录同步形成闭环。
- **任务紧迫度**：任务详情属性栏展示 urgency 分数和各分项贡献（`GET /tasks/{ref}/urgency`）。终态任务（`completed`/`deleted`）urgency 恒为 0，不参与优先级排序，与 Taskwarrior 行为一致。
- **Workspace / 通知控制台**：workspace 列表支持归档；通知页明确为「管控台」（sink/rule/delivery），不是个人消息收件箱。
- **出站集成控制台**（v0.5.3，`/hooks` = `/integrations` = `/notifications`）：把 `/hooks` 升级为统一控制台，覆盖 Sinks（webhook / http_template CRUD）、Hooks（sink 下拉 + 分组事件 checkbox + project 选择器）、通知规则、定时规则和概览。新增后端 `POST /api/v1/notification-sinks/{sinkID}/test` 真实测试投递，写 audit 不污染 delivery 表，受 SSRF / allowed hosts / secret 防护。事件名统一为 `task.completed` / `project.archived` 等白名单，旧的 `task.done` 已废弃。
- **项目自动化页**（`/workspaces/<workspace-slug>/projects/<project-slug>/automations`）：项目详情新增「自动化」tab，可配置 project-scoped 定时规则和事件规则。首版动作统一为调用 OpenAI 兼容 Agent Provider，默认投递到 `{agent.provider.base_url}/v1/chat/completions`，凭据和默认 model 从 project effective config 读取；规则编辑页可点击「预览投递 JSON」查看脱敏后的最终请求体。璇础只负责触发、上下文构造、投递和记录，不直接调用飞书，也不判断 Agent 是否完成外部动作。

所有改动只复用现有 `/api/v1/*`、authz、CSRF、`task.UserInfo` 和 closed-project 规则；未引入新 UI 库或状态管理库。后端能力缺口（task restore、workspace unarchive、project annotation PATCH、audit actor/action/time 全量服务端搜索）在前端以置灰、说明文案或「当前结果筛选」明确标注，不静默失败。

仓库只跟踪 `internal/webconsole/dist/.gitkeep`，不跟踪前端构建产物。日常前端开发使用 `pnpm --dir web dev` 并代理到 Go HTTP API；发布二进制必须使用 `make build-release`，或先运行 `make web-console-build` 再执行 Go 构建，确保真实 Web Console 静态资源被 embed 进二进制。

普通 Console 支持项目深链，适合放进飞书卡片、企业门户或内部系统消息中：

```text
http://127.0.0.1:8080/workspaces/<workspace-slug>/projects/<project-slug>
http://127.0.0.1:8080/workspaces/<workspace-slug>/projects/<project-slug>/tasks
http://127.0.0.1:8080/workspaces/<workspace-slug>/projects/<project-slug>/activity
http://127.0.0.1:8080/workspaces/<workspace-slug>/projects/<project-slug>/automations
```

项目页由「概览 / 任务 / 活动 / 自动化」四个子页面组成，共享同一套项目 Header、子页面 Tabs 和可开合的右侧项目信息栏（`ProjectContextRail`）。概览页回答「项目现在怎么样、下一步该看哪里」：展示最新项目更新、当前重点（逾期 / 高优未完成 / 等待已到期 / 未分配任务）、项目附属信息、负责人负载和最近活动；任务页承接任务筛选工具栏、普通/循环任务创建、融合任务列表、普通任务导入和行内编辑；循环任务不增加独立 Tab，而是从任务页的“循环任务 N”进入右侧治理面板。活动页是项目事实流，合并项目更新、任务注解和有 `audit:read` 权限时可见的项目审计。右侧项目信息栏在三个子页面保持一致，可由图标按钮收起/展开；状态、任务数、进度、风险计数、负责人负载和最近活动都来自后端聚合，前端不用当前任务列表冒充全量统计。具备 `project:write` 的 owner/admin 可以 inline 修改项目名称和描述、转移项目状态、在设置中修改项目 slug；转入 `archived` / `cancelled` 会要求二次确认。具备 `task:write` 的 owner/admin/member 可以在任务子页面内创建任务、上传 JSON / XLSX 批量导入普通任务，并 inline 修改任务 title、priority、due 等常用字段。任务列表默认按 urgency（紧急度，综合 due/priority/阻塞/active/age 等因素的优先级计算）降序排列，可在工具栏或表头切换为创建顺序、截止日期、暂缓到、开始时间、完成时间；按 urgency 排序时列表额外展示每条任务的 urgency 分数。导入弹窗会先在浏览器端预检标题、指派人和依赖，并可打开完整 JSON Schema 弹窗核对字段说明：导入文件可用可选的 `id` / `import_id` 标记临时任务引用，只要求是批次内不重复的字符串，不要求 UUID 格式；`blocked_by` 表示当前任务被哪些任务阻塞；导入后会生成新的任务 UUID，临时 ID 不入库；指派人必须能解析为当前 workspace 成员；JSON assignee 对象可包含 `id`、`name`、`display_name`、`email`，XLSX 可用 `assignee_display_names` / `assignee_emails` 为 `assignees` 按顺序补充展示姓名和邮箱；若检测到普通邮箱或姓名尚未加入 workspace，可在预检区一键创建/加入为 `member`，并保留 `display_name`；预检区会用分页表格展示解析和本地引用映射后的导入成果，最终通过版本化 `xuanchu.task-import/v1` 对象提交到 `/api/v1/task-imports` 并整批原子写入。跨环境迁移另使用 `/api/v1/import` 的 `xuanchu.task-bundle/v1`，两者不接收裸 Taskwarrior 数组。概览页和右栏的全量任务摘要来自新增的 `GET /api/v1/projects/{projectRef}/task-summary`，同时要求 `project:read` 与 `task:read`；逾期与等待已到期判断复用璇础任务日期边界（date-only `due/until` 按本地日末、`wait/scheduled` 按本地日初），前端不用浏览器时区重算。

```text
http://127.0.0.1:8080/workspaces/<workspace-slug>/projects/<project-slug>/tasks/<task-ref>
```

任务详情页提供“返回项目”入口，并支持 title inline 编辑、description 完整展示和弹窗编辑、start / stop / done / reopen / delete 操作、注解添加/编辑/删除、链接添加/删除，以及右侧属性栏编辑。description 和注解在 Web Console 中使用 Tiptap Markdown WYSIWYG 编辑与同 schema 只读渲染，支持标题、列表、引用、代码、表格、待办列表和安全链接；后端契约不变，仍把这些内容作为普通字符串保存，CLI、Remote Client、MCP 和 HTTP JSON 输出继续看到 Markdown 源码。写操作仍然全部通过 `/api/v1/*` 执行，继续受 membership role、token scope、workspace allowlist、project allowlist 和 closed project 状态约束；前端只隐藏明显不可用的写控件，服务端 403/404 仍是最终裁决。未登录用户会先看到普通 token 登录页（或 OIDC 单点登录入口），登录成功后回到原项目页或任务详情页。任务详情页还展示字段级变更历史：每次字段修改（title / description / assignees / due / priority / project / tags / wait / scheduled / until / depends / udas）都会在 `audit_logs` 的 `task.modify` payload 里记录 before/after，前端通过 `GET /api/v1/tasks/{taskRef}/audit`（只要求 `task:read`，不要求 `audit:read`）读取并以自然语言渲染，例如「Alice 将标题从 A 改为 B」。变更历史只持久化机器语义，人类文案由前端 i18n 模板生成；历史无字段级明细的旧行显示为空。

侧边栏以「项目」为任务浏览主入口：`/projects` 列出所有项目（含任务进度与计数），并支持新建项目；点击某行进入项目概览页，再通过 Tabs 切到「任务」子页面，可在任务表格上方用 status / 优先级 / 负责人 / 关键字过滤，过滤条件同步到 URL 便于分享；点击任务行进入任务详情页，完整查看 Markdown description，并在弹窗中编辑描述、注解、关联链接、属性与已有自定义字段（UDAs）。任务子页面的“导入任务”支持下载完整字段 XLSX 模板、上传填写后的 XLSX 或标准 JSON，也可以在弹窗内查看带 `description` 注释的完整 JSON Schema；模板包含「字段说明」sheet，逐列列出 required、type、allowed values、format 和 example，Tasks sheet 只保留字段列、示例行、筛选和日期格式提示，不使用表头批注或文本框承载字段说明；description 默认按 Markdown 编写，技术上仍作为字符串保存；`blocked_by` 支持引用导入文件内的临时 `id` 或已有任务 UUID；预检发现缺失普通指派人时可直接创建用户并加入当前 workspace，导入文件中的 `display_name` 会作为用户展示姓名保留；上传解析后会分页预览归一化后的导入成果。`archived` / `cancelled` 项目会显示 closed banner，并隐藏任务写入口与项目更新写入入口；具备项目管理权限的用户仍可通过状态菜单恢复到 `planning` 或 `active`。项目记录（project annotation）已从设置页迁到活动子页面，旧 `/projects/<slug>/settings/notes` 入口兼容重定向到活动页。

普通 Console 的 `/members` 页面是 workspace 成员管理入口。owner/admin 可以搜索和筛选成员、添加已有用户或创建最小用户后加入 workspace、编辑成员 `display_name`、调整角色、移出成员；所有弹窗和危险确认都走 shadcn 组件。`/members/:userRef` 承载次级成员详情：身份快照、当前 membership、外部身份摘要、关联 token 跳转和最近成员审计。admin 只能管理非 owner 成员；owner 可以授予/降级 owner 或移出 owner，但服务端会保护最后一个 owner。member/viewer 只能查看成员名册和详情。`users.name` 仍是稳定引用名，`display_name` 只用于展示姓名；成员页修改展示姓名不会改变稳定名。浏览器 OIDC session 允许普通 `/api/v1/*` 的成员管理与 token 管理（创建/修改/吊销 PAT/Agent/tenant token）写入，但仍必须经过 CSRF、membership role 和 scope 检查，且不会因此获得 `impersonate`；token 管理动作最终仍受 app 层 `tokenManageAllowed(role)` 约束，仅 owner/admin 可执行。


普通 Console 的 `/tokens` 页面支持 PAT / Agent token 的完整生命周期管理：创建（选择类型、scope、工作空间范围、过期时间）、编辑（名称、scope、过期时间、工作空间与项目范围）、吊销。列表行只展示 prefix，不返回完整明文；但每一行都带「MCP 配置」按钮，点击后弹窗按需 reveal 完整 token 与可复制的 HTTP MCP 配置片段。认证仍只使用 `token_hash` 和短 prefix；当服务端配置了 `[security].config_secret_key` 时，HTTP/Web Console 创建的 token 还会保存加密后的 raw token envelope，供 `/tokens` 行内按需展示。reveal 要求当前凭证具备 `token:read`，并受当前 token 的 workspace / project allowlist 限制；缺少可恢复密文的旧 token 会明确提示重新签发，不会降级展示。scope 编辑按资源分组勾选，提交展开后的具体 scope；agent token 必须绑定至少一个工作空间。owner/admin 还可在创建 PAT 时选择「归属用户」，为当前 workspace 的其它成员代为创建 token（默认「我自己」）；列表顶部的「查看用户」筛选器允许 owner/admin 切换查看不同成员的 token。代为创建仍受调用凭证的 scope/workspace 上限约束，member/viewer 看不到这两个入口。

Server admin bootstrap 使用独立入口：

```text
http://127.0.0.1:8080/admin/login
```

启用 `[server.admin]` 后，如果数据库中还没有有效的 server admin token，`xuanchu server` 启动时会在控制台输出一次性 setup-code。打开 `/admin/setup` 输入该 setup-code，即可生成第一个 `xuanchu_admin_...` token；token 明文只显示一次，数据库只保存 SHA-256 verifier。

admin token 只用于 `/api/v1/admin/*` 控制面接口，可以在页面中创建 workspace、创建或提升 workspace 管理员，并为该管理员创建 workspace-scoped Agent token。它不能访问普通任务、项目、通知、Hook 或 MCP 接口。`xuanchu admin token generate/hash` 仍保留为兼容和运维工具，但不再是首选初始化路径。

Admin 工作台的 `/admin/workspaces` 提供 workspace 控制面：列出全部 workspace（含已归档）、查看 owner/admin/member 摘要与 token 计数、编辑成员 `display_name`、从任意未归档 workspace 创建短期 acting session 进入该 workspace 的管理员视角。`users.name` 仍是稳定用户引用名，`display_name` 只用于展示姓名，可与 `name` 不同。acting token（`xuanchu_act_` 前缀）只面向浏览器 Workspace Console 的普通 `/api/v1/*`，默认 TTL 2 小时，绑定单一 workspace，实际权限由绑定 actor 的当前 workspace membership role 决定；它不能用于 HTTP MCP、stdio MCP，remote CLI client 也会在客户端侧直接拒绝该前缀。acting mode 下普通操作仍记录到审计日志，并额外带上 `admin_acting_session_id`、`delegator_admin_token_id`、`delegator_admin_token_name`，可追溯到发起委托的 server admin。Workspace Console 顶部持续显示 acting banner，用户点击「返回超管界面」清理 acting session 但保留 admin token。

Admin 工作台的 `/admin/tokens` 页面提供跨 workspace 的 token 管控：列出所有 workspace 的全部 token（含 user、workspace、scope、状态）、吊销失控 token、修改 token 的 name / scope / 过期时间。这是运维管控视角，与普通用户在 `/tokens` 的自服务管理是两个边界——admin 操作绕过 workspace role 校验，但所有变更记录到审计日志（`admin.token.modify` / `admin.token.revoke`，payload 标记 `admin:true`）。admin 不创建 token（创建走 bootstrap 或普通 console），也不修改 token 的 workspace/project 绑定（由 token owner 自服务）。

前端开发和构建：

```bash
pnpm --dir web dev
pnpm --dir web build
```

`pnpm --dir web build` 会刷新 `internal/webconsole/dist`，发布二进制通过 Go `embed` 打包这些产物，运行时不需要 Node.js。

## 开发测试

默认测试不依赖 PostgreSQL 或外部服务：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

真实进程 E2E 集中在 `tests/integration`，覆盖本地 CLI、远程 CLI / HTTP API、HTTP MCP、stdio MCP、server dispatcher 运行时和 SQLite migration smoke：

```bash
go test ./tests/integration -count=1
```

PostgreSQL E2E 是显式 opt-in。测试会用 admin URL 创建并删除 `xuanchu_e2e_*` 临时数据库：

```bash
XUANCHU_E2E_POSTGRES_ADMIN_URL='postgres://mac@127.0.0.1:5432/postgres?sslmode=disable' \
  go test ./tests/integration -run TestPostgresE2E -count=1
```

## 命令和参数怎么写

任务属性一般写成 `key:value`，标签写成 `+tag` 或 `-tag`：

```bash
./xuanchu add "Ship MCP API" project:agentapi priority:H +next due:friday
./xuanchu add "Review release note" @alice
./xuanchu 1 modify project:agentapi priority:M +review -next
./xuanchu 1 modify +@alice -@bob
./xuanchu 1 modify due:                  # 清空 due
```

多 assignee 查询与展示：

```bash
./xuanchu list assignee:alice
./xuanchu next assignee:me
./xuanchu info 1
```

查询可以放在报表命令前，也可以放在报表命令后：

```bash
./xuanchu +next list
./xuanchu list +next
./xuanchu '(project:agentapi and +review) or priority:H' next
```

Shell 会吃掉括号、空格和 `+` 等字符，复杂查询建议加引号。`/text/` 是 title 子串匹配，不是正则。

脚本里建议优先使用这些稳定接口：

```bash
./xuanchu --json export
./xuanchu _ids +next
./xuanchu _uuids project:agentapi
./xuanchu _get 1.uuid 1.title 1.urgency
./xuanchu _show database.path active.user active.workspace active.context
```

## 本地 CLI 基础用法

```bash
go build -o xuanchu ./cmd/xuanchu

# 添加任务
./xuanchu add "Write project spec" project:agentapi +planning due:tomorrow
./xuanchu add "Review PR" priority:H +review

# 查看任务列表
./xuanchu list

# 查看任务详情
./xuanchu info 1

# 修改任务
./xuanchu 1 modify priority:H +next
./xuanchu 1 modify project:agentapi

# 完成任务
./xuanchu 1 done

# 重新打开已完成任务
./xuanchu 1 reopen

# 删除任务
./xuanchu 1 delete

# 导出 xuanchu.task-bundle/v1
./xuanchu export

# 导入同一原生 bundle（不接受 Taskwarrior 数组）
./xuanchu import xuanchu-task-bundle.json

# 查看配置
./xuanchu show

# 设置配置
./xuanchu config set date.format rfc3339
./xuanchu config get date.format
```

默认数据库路径为 `~/.local/share/xuanchu/xuanchu.db`，可用 `--db` 或 `XUANCHU_DB` 环境变量覆盖。

## 查询、报表、Urgency 与 Helper

```bash
# 布尔组合查询
./xuanchu '+next or due.before:tomorrow' list
./xuanchu '(project:agentapi and +urgent) or priority:H' list

# 报表命令
./xuanchu all
./xuanchu completed
./xuanchu deleted
./xuanchu overdue

# 查看任务 urgency（human 或 JSON）
./xuanchu urgency 1
./xuanchu urgency 1 --json
./xuanchu _urgency 1

# DOM helper
./xuanchu _get 1.title 1.uuid 1.urgency 1.tag.next
./xuanchu _ids +next
./xuanchu _uuids project:agentapi
./xuanchu _projects
./xuanchu _tags

# 表达式计算
./xuanchu calc '1 + 2 * 3'
```

报表名等价于 `(默认 filter) AND (用户 filter)`。要绕过默认 status 限制，使用 `all`。

### 日期与 deadline 语义

`due:` 和 `end:` 表达的是「某天截止/结束」，写入时会自动落在**当地时区的当天 `23:59:59`**：

```bash
./xuanchu add "deadline" due:2030-01-01
# due 实际存储为 2030-01-01 23:59:59（本地时区），而非 00:00:00
```

查询时日期等值也用自然日范围，例如 `due:2030-01-01` 等价于 `[2030-01-01 00:00:00, 2030-01-01 23:59:59]`，跨 DST 与时区也稳定。

### title / description 子串匹配

`title:spec`、`title:/spec/` 和裸 `/spec/` **语义一致**，都按标题子串匹配；`description:` 只匹配可选的详细描述，也不走字面相等。

## 核心任务模型

```bash
# waiting / active / ready / blocked / blocking 报表
./xuanchu add "Call vendor" wait:tomorrow scheduled:eow until:eom
./xuanchu waiting
./xuanchu 1 modify wait:
./xuanchu 1 start
./xuanchu active
./xuanchu 1 stop

# 注释与描述编辑
./xuanchu 1 annotate "called, left voicemail"
./xuanchu _get 1.annotations
./xuanchu 1 denotate <annotation-id>
./xuanchu 1 append "with examples"
./xuanchu 1 prepend "[draft]"
./xuanchu 1 edit

# 依赖与 blocked / blocking
./xuanchu add "Prepare API"
./xuanchu add "Write docs" depends:<uuid-or-id>
./xuanchu blocked
./xuanchu blocking

# 循环任务系列（v0.5.7）：日历驱动实例生成
./xuanchu series add "每日检查投放消耗" --project ops --recur daily --first-due 2026-07-11 --until 2026-07-31
./xuanchu series list --project ops
./xuanchu series info <series-ref>
./xuanchu series occurrences <series-ref> --status pending
./xuanchu series stop <series-ref>
```

当前支持这些任务能力：

- 任务字段：`start`、`wait`、`scheduled`、`until`、`annotations`、`depends`、`parent`（仅手工父子任务）
- 报表命令：`waiting`、`active`、`ready`、`blocked`、`blocking`
- 动作命令：`start`、`stop`、`done`、`reopen`、`delete`、`annotate`、`denotate`、`append`、`prepend`、`edit`
- 查询 / DOM / urgency / JSON import-export 对上述字段的贯通支持
- 循环任务系列：`xuanchu series add/list/info/modify/occurrences/stop/skip`，canonical 规则 `daily`、`weekly`、`monthly`、`<N>days`、`<N>weeks`、`<N>months`

CLI 表格里的 `ID` 是默认 working set ID，跨 `list` / `next` / `ready` / `blocked` 等报表稳定，与排序无关；隐藏的 waiting 任务仍可用该 ID 操作，所以如果前面有 waiting 任务，`list` 中第一条可见 pending 任务可能显示为 `2`。`completed` / `deleted` 等不在 working set 中的报表，ID 列显示为 `-`。projected occurrence（尚未物化的循环实例）不进入 working set，ID 列显示 `-`，需用 `occurrence_ref` 操作。

`edit` 会打开缩进 JSON，保存后执行校验；非法日期、非法 status、换行 annotation 等错误不会写回。

v0.5.7 按 [循环任务系列规格](./docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md) 实现独立 `task_series` 聚合、范围投影与按需物化：

- 循环任务使用独立 `task_series` 表，不再用 `status=recurring` 隐藏任务或循环用途的 `parent`
- 每个日历槽位是独立任务，按日历驱动生成（即使上一条未完成，下一天仍生成新实例）
- 有界日期范围查询合并普通任务、projected occurrence 和 materialized exception
- occurrence 的公开 `id` 为 `occ:<series_uuid>:<recurrence_at_unix>`，物化前后不变
- HTTP `/api/v1/task-series`、MCP `task_series_*` tools、Remote client 提供完整 CRUD
- HTTP `GET /tasks`、`GET /reports/{name}` 统一返回 `TaskViewPage`（`{items, total, limit, offset, occurrence_mode, range}`），不再返回裸任务数组；Remote client 用 `QueryTasks`/`GetTaskView` 替换旧 `ListTasks`/`GetTask`
- `GET /tasks` 与 MCP `task_query` 使用同名的 `due_after`、`due_before`、`occurrence_mode=auto|materialized|expand`、`task_type=all|normal|occurrence`；完整日期范围下 `auto` 展开普通任务与 projected/materialized occurrence，显式 `expand` 缺任一边界会报错
- `GET /tasks` 与 MCP `task_query` 默认返回所有非 deleted 任务（包括 completed）；显式 status 条件优先，MCP 可用 `include_deleted=true` 在默认集合上追加 deleted
- 项目任务摘要（`GET /projects/{ref}/task-summary`）将普通进度和循环运行情况分开：普通进度与风险计数排除 occurrence，`series_metrics` 返回循环系列/实例运行数；成员待办统计当前真实待办，会包含已物化、未完成的循环实例，但不包含 projected 的未来实例
- Web Console 任务页内可深链的循环任务管理面板（不新增全局导航或 ProjectTab），支持创建、状态/负责人筛选、排序、分页、详情实例分组、编辑/清空共享字段与停止；编辑系列负责人只影响未来实例，已物化实例必须单独修改；普通 JSON/XLSX 导入会拒绝 `recur/mask/imask` 和 `status=recurring`
- 旧 `recur`/`mask`/`imask` 字段和 `add ... recur:*` 命令不再支持；跨环境迁移使用 `xuanchu.task-bundle/v1` 原生 bundle
- CLI `export/import`、HTTP `/export|/import`、MCP `task_export/task_import` 与 Remote `ExportTaskBundle/ImportTaskBundle` 使用同一 `xuanchu.task-bundle/v1` object；Web 普通任务表格导入使用独立的 `xuanchu.task-import/v1`
- **破坏性 schema 变更**：开发数据库需重建——检测到旧 `status=recurring` 数据会拒绝启动

## 配置、Context、UDA 与 `.taskrc`

```bash
# TOML / config / rc 覆盖
./xuanchu show
./xuanchu config set date.format rfc3339
./xuanchu config list
./xuanchu rc.date.format=epoch list

# context
./xuanchu context define agent 'project:agentapi status:pending'
./xuanchu context use agent
./xuanchu list
./xuanchu --no-context list
./xuanchu context show
./xuanchu context none

# UDA
./xuanchu config set uda.estimate.type numeric
./xuanchu config set uda.estimate.label Estimate
./xuanchu config set uda.estimate.values 1,2,3,5,8
./xuanchu add "Implement API" estimate:3
./xuanchu estimate:3 list
./xuanchu _get 1.estimate
./xuanchu _udas
./xuanchu _unique estimate

# .taskrc 只读导入
./xuanchu config import-taskrc ~/.taskrc --dry-run --json
./xuanchu config import-taskrc ~/.taskrc

# completion 与脚本 helper
./xuanchu completion zsh > ~/.zfunc/_xuanchu
./xuanchu _show date.format active.user active.workspace active.context
./xuanchu _version
```

Xuanchu 支持用 `~/.config/xuanchu/xuanchu.toml` 作为文件配置来源。TOML 使用标准解析器，支持普通 TOML 字符串、数组、dotted key 和多行字符串。一个最小示例：

```toml
[database]
path = "/Users/me/.local/share/xuanchu/xuanchu.db"

[display]
color = true

[date]
format = "rfc3339"

[server.shutdown]
timeout = "30s"
force_timeout = "5s"
```

### `xuanchu.toml` 可配置项

`xuanchu.toml` 是**本机配置**：它描述这台机器如何启动和显示 Xuanchu，不描述某个企业 workspace 的业务规则。TOML 的 key 会被展开成点分格式，例如 `[database] path = "..."` 等价于 `database.path = "..."`。

当前建议只把这些 key 写进 TOML：

| TOML 写法 | 展开后的 key | 值 | 说明 |
|---|---|---|---|
| `[database] path = "..."` | `database.path` | 文件路径 | SQLite 数据库路径。只在启动打开数据库前生效；运行后不能用 `config set database.path` 修改。 |
| `[display] color = true` | `color` | `true` / `false` | 是否启用 human 输出颜色。也可直接写 `color = true`。 |
| `[display] json = false` | `json` | `true` / `false` | 默认是否输出 JSON。CLI 的 `--json` 优先级更高。也可直接写 `json = false`。 |
| `[date] format = "rfc3339"` | `date.format` | `rfc3339` / `epoch` | `_show`、`config get` 和部分脚本输出使用的日期格式。 |
| `[server.shutdown] timeout = "30s"` | `server.shutdown.timeout` | Go duration | `xuanchu server` 或 `xuanchu mcp stdio` 收到 SIGTERM / SIGINT 后等待已开始工作完成的整体 drain 时间。 |
| `[server.shutdown] force_timeout = "5s"` | `server.shutdown.force_timeout` | Go duration | drain 超时后强制取消剩余工作，再等待运行时清理资源的时间。 |
| `[log] level = "info"` | `log.level` | `debug` / `info` / `warn` / `error` | 日志级别。环境变量 `XUANCHU_LOG_LEVEL` 优先。 |
| `[log] format = "text"` | `log.format` | `text` / `json` | 日志格式。 |
| `[log.file] path = "..."` | `log.file.path` | 文件路径 | 推荐写法。日志文件路径，支持 `~` 展开。环境变量 `XUANCHU_LOG_FILE` 优先。 |
| `[log.file] rotate = "daily"` | `log.file.rotate` | `daily` / `size` / `none` | 推荐写法。日志轮转模式。`daily` 按日期切割，`size` 按 10MB 切割，`none` 不轮转。 |
| `[log] file = "..."` | `log.file` | 文件路径 | 兼容旧文档写法，等价于 `log.file.path`。如果两种写法同时存在，`[log.file] path` 优先。 |
| `[server.mcp] trusted_proxy_hosts = [...]` | `server.mcp.trusted_proxy_hosts` | hostname 列表 | 仅用于 HTTP MCP 反向代理。后端监听 loopback 且 nginx 保留公网 Host 时，把可信公网域名加入 allowlist。 |

一个完整的本机配置例子：

```toml
[database]
path = "/Users/me/.local/share/xuanchu/xuanchu.db"

[display]
color = true
json = false

[date]
format = "rfc3339"

[server.shutdown]
timeout = "30s"
force_timeout = "5s"

[log]
level = "info"
format = "text"

[log.file]
path = "~/.local/share/xuanchu/logs/xuanchu.log"
rotate = "daily"

[server.mcp]
trusted_proxy_hosts = ["xuanchu.example.com"]
```

`xuanchu server` 的日志会同时写 stderr 和配置的日志文件。结构化 operation log 覆盖 server lifecycle、HTTP access、HTTP MCP tool/resource 调用、notification/hook dispatcher 投递和 reminder scheduler 扫描。日志字段包含 `component`、`operation`、`request_id`、`actor_user_id`、`token_id`、`delivery_id`、`rule_id`、`result` 等运维排障字段；不会记录 Bearer token、webhook secret、HTTP request/response body 或渲染后的 Authorization header。

不要把这些业务配置长期写进 TOML：

- `uda.*`
- `urgency.uda.*`
- `context.<name>`
- 未来的 report、hook、project defaults

这些配置属于 workspace，应写入 SQLite，由权限和 audit 管理：

```bash
./xuanchu --workspace dajee config set uda.estimate.type numeric
./xuanchu --workspace dajee config set uda.estimate.values 1,2,3,5,8
./xuanchu --workspace dajee config set urgency.uda.estimate.coefficient 1.5
./xuanchu --workspace dajee context define agent 'project:agentapi status:pending'
```

不要把这些状态或内部 key 当作 TOML 配置写入：

- `active.user`、`active.workspace`、`active.context`：只读状态输出，用 `user use`、`workspace use`、`context use/none` 修改。
- `context.active`：兼容旧 key，但不作为运行时 active context 来源。
- `active_user_id`、`active_workspace.<user>`、`active_context.<user>.<workspace>`：内部 SQLite meta，只用于迁移和运行时状态。

想确认当前最终生效值，用：

```bash
./xuanchu config list
./xuanchu _show database.path color json date.format active.user active.workspace active.context
```

数据库路径仍按 `--db`、`XUANCHU_DB`、`--data-dir`、TOML、XDG data、home fallback 的顺序解析。显示类配置按 CLI flag、`rc.*`、环境变量、SQLite meta、TOML、默认值合并。workspace 业务配置以数据库中的 workspace 记录为准；TOML 只作为本机默认值或迁移辅助。

`rc.*` 只影响本次命令，适合脚本临时覆盖：

```bash
./xuanchu rc.date.format=epoch list
./xuanchu rc.json:on --workspace missing list
./xuanchu rc.context=none list
```

`context` 会自动叠加到 `list`、`next`、各类报表和 `_ids/_uuids/_projects/_tags/_unique`。`--no-context` 和 `rc.context=none` 只影响本次运行；`context none` 会持久化清空 active context。

UDA 支持 `string`、`numeric`、`date`、`duration` 四种类型。date UDA 写入为 RFC3339 UTC 字符串；查询 `estimate:3`、`reviewed:2026-05-28` 会结合当前 schema 编译。未定义的 JSON top-level 字段会作为 orphan UDA 保留并导出，普通 `modify` 不能修改 orphan UDA。

`.taskrc` 的作用是**迁移和兼容性导入**：Xuanchu 只读解析它，把支持的 key 导入到 SQLite 配置、context 或 UDA schema，并生成 imported/skipped/unknown 报告。Xuanchu 不会修改原 `.taskrc`，也不会把 `.taskrc` 当成每次运行的完整配置源。

当前 `.taskrc` 支持范围：

- 支持导入：`color`、`dateformat`、`context.<name>`、`uda.<name>.type/label/values/default`、`urgency.uda.*`
- 识别但跳过：`data.location`、`report.*`、`calendar.*`、`burndown.*`、`news.*`、`sync.*`、`hooks.*`
- 其它 key 进入 unknown 报告，不会让导入失败

其中 `data.location` 会被识别但不会导入，因为数据库路径只在启动前通过 `--db`、`XUANCHU_DB`、`--data-dir` 或 TOML 决定。Xuanchu 不导入自定义 Taskwarrior report DSL，也不运行本地 shell hooks。

## 企业 Workspace、权限与审计

```bash
# user
./xuanchu user list
./xuanchu user add alice email:alice@example.test
./xuanchu user use alice
./xuanchu user info

# workspace。这里的 dajee 表示企业 / 租户级工作空间
./xuanchu workspace list
./xuanchu workspace add dajee name:Dajee visibility:team
./xuanchu workspace use dajee
./xuanchu workspace info dajee
./xuanchu workspace modify dajee description:"Dajee enterprise workspace"
./xuanchu workspace archive old

# project 表示 workspace 内的真实企业项目
./xuanchu --workspace dajee project add agentapi name:"AI Agent Platform"
./xuanchu --workspace dajee project add erpflow name:"ERP Rewrite"
./xuanchu --workspace dajee add "Design MCP task.query schema" project:agentapi +mcp
./xuanchu --workspace dajee add "Migrate invoice workflow" project:erpflow +migration

# 在指定 workspace 中执行一次命令
./xuanchu --workspace local list
./xuanchu --workspace dajee _projects

# member
./xuanchu member list
./xuanchu member add bob role:viewer
./xuanchu member role bob member

# audit
./xuanchu audit list
./xuanchu audit list --limit 20 --json
```

企业运行时支持：

- `user list/add/use/info`
- `workspace list/add/use/info/modify/archive`
- `member list/add/role`
- `audit list`
- 全局 `--workspace <slug|uuid>` 一次性切到指定 workspace 执行命令

`workspace` 是企业 / 租户级隔离边界；`project` 是 workspace 内的一等实体。任务上仍保留 `project` 字符串字段做人类可读输出，但运行时写入、查询、权限、审计和 API/MCP scope 都以稳定 `project_id` 为准。

同一个 project slug 可以出现在不同 workspace 中。也就是说，`dajee/agentapi` 和 `partner/agentapi` 是两个不同项目；权限、token 和 MCP scope 必须以 `workspace + project` 或稳定 `project_id` 为准，不能把 slug 当全局唯一标识。

当前 CLI 的 `--workspace <slug|uuid>` 使用裸 workspace slug，所以 workspace slug 在同一个 Xuanchu 实例内应保持唯一。project slug 只在当前 workspace 内解析：

```bash
./xuanchu --workspace dajee list project:agentapi
./xuanchu --workspace partner list project:agentapi
```

上面两条命令访问的是两个不同 workspace 里的同名 project。Xuanchu 采用严格 project 注册：`xuanchu add ... project:<slug>` 和 `xuanchu 1 modify project:<slug>` 只能引用当前 workspace 内已存在、未归档的 project，不会运行时自动创建。脚本、远程 API 和 MCP 应优先保存 `project_id`。如果同时指定 `--workspace` 和 `project_id`，该 project 必须属于这个 workspace；否则命令会报错，避免把任务写进错误租户。

权限模型：

- `viewer` 可以读任务、报表、helper、member list，并能切换自己的 active context
- `member` 额外可以写任务、import、定义/删除 context
- `admin` 额外可以改 workspace metadata、管理非 owner 成员、查看 audit、管理 UDA schema、管理服务端 Hook
- `owner` 额外可以授予/降级 owner、归档 workspace

`config list` 和 `_show` 只暴露用户可用的 public key，例如 `active.user`、`active.workspace`、`active.context`、`uda.*`、`urgency.*`。`active_user_id`、`active_workspace.<user>`、`active_context.<user>.<workspace>` 是内部 meta，不作为 CLI 接口使用。

`audit list` 的 human 输出包含时间、actor、action、target type 和 target id。`--json` 会额外输出 `actor_user_id`、`actor_name`、`workspace_id`、`payload` 等字段，适合脚本处理。

当前 CLI 没有 `member delete`；CLI 中临时撤销写权限仍可用 `member role <user> viewer`。Web Console 和 HTTP API 支持把成员移出 workspace，但不会删除 user。无论通过哪种入口，`viewer` 仍能读取该 workspace 的任务、project、tag、UDA 和成员信息。

## Project 实体与配置边界

Project 与配置边界的关键规则：

- 任务引用 project 前，必须先在当前 workspace 注册 project。
- `project` 查询、context、helper、audit 都会先在当前 workspace 内把 slug 解析成稳定 `project_id`。
- `project config` 继续是 project 级业务配置入口，但 key 的合法性现在由 workspace 内 `config schema` 决定，不再靠代码白名单。
- JSON export 继续输出可读的 `project` slug，但脚本、API、token、MCP 应优先持有 `project_id`。

一个完整的日常流大致是这样：

```bash
# 先建 workspace，再注册 project
./xuanchu workspace add dajee name:Dajee
./xuanchu --workspace dajee project add agentapi name:"AI Agent Platform" description:"Owns MCP work"
./xuanchu --workspace dajee project add erpflow name:"ERP Rewrite"

# 用 project slug 创建和查询任务
./xuanchu --workspace dajee add "Design task.query schema" project:agentapi +mcp
./xuanchu --workspace dajee add "Review ERP migration" project:erpflow
./xuanchu --workspace dajee list project:agentapi

# 管理 project 元数据
./xuanchu --workspace dajee project list
./xuanchu --workspace dajee project info agentapi --json
./xuanchu --workspace dajee project modify agentapi description:"Owns Xuanchu MCP and API work"
./xuanchu --workspace dajee project archive erpflow

# 归档后不能再被新任务引用
./xuanchu --workspace dajee add "Should fail" project:erpflow
```

如果你直接写一个不存在的 project，命令会失败，而不是偷偷创建：

```bash
./xuanchu add "Ghost task" project:ghost
# xuanchu: project_not_found: project "ghost" not found ...
```

### `project` 命令组

当前支持这些命令：

```bash
./xuanchu project list [--all] [--status open|planning|active|archived|cancelled|all]
./xuanchu project add <slug> name:<name> [description:<text>]
./xuanchu project info <slug|project-id>
./xuanchu project modify <slug|project-id> [name:<name>] [description:<text>]
./xuanchu project archive <slug|project-id>
./xuanchu project transition <slug|project-id> <planning|active|archived|cancelled>
```

说明：

- project 有四种状态：`planning`（预立项）/ `active`（立项在跑）/ `archived`（结束归档）/ `cancelled`（取消）。新建 project 默认 `planning`。
- `project transition` 在任意状态间自由转移（含 `archived → active` 重新激活），转移后自动追加一条项目变更注解并写入审计；触发 `project.transitioned` 事件。
- `planning` / `active` 可写（add task、annotate、config）；`archived` / `cancelled` 禁止写操作，但始终允许 `transition`。
- `project list` 默认只列进行中的 project（`open` = planning + active）；`--all` 或 `--status all` 列全部；`--status` 可按具体状态过滤。
- `project info` / `modify` / `archive` / `transition` 既接受 slug，也接受稳定 `project_id`。
- slug 只在当前 effective workspace 内解析，不做跨 workspace 搜索。

### `config schema` 与 `project config`

shared config 现在分成两层：

- `config schema`
  - 定义 key、类型、允许作用域、默认值等 schema
- `config`
  - 写 workspace scope 的显式值
- `project config`
  - 写 project scope 的显式值

**project 扩展信息的承载方式**：project 目前没有类似 task UDA 的自由扩展属性机制（`Project.SettingsJSON` 是未启用的死字段），也没有 project 级别的外部链接表。要给 project 附加自定义信息（例如关联的外部系统、业务属性、Agent 背景等），统一使用 `project config`：

- schema（`config schema`）是 workspace 级契约，决定哪些 key 能在 project 上使用、值类型、是否 secret。
- 具体值存在 `configs` 表，按 `(workspace, scope=project, scope_id=project.ID, key)` 四元组隔离，**每个 project 独立**，互不影响。
- 读取链是三级回退：`project 显式值 > workspace 显式值 > schema default`，因此 workspace 级的值会作为所有 project 的默认值。
- 约束：每个 key 必须先有允许 `project` scope 的 schema 才能写入；单个 value 是字符串，结构化数据需自行编码（如把 JSON 字符串存进去）。
- 归档/取消的 project 禁止写 config。

典型流程：

```bash
./xuanchu --workspace dajee config schema set agent.background type:string scopes:project label:"Agent Background"
./xuanchu --workspace dajee config schema set ads.roi_threshold type:number scopes:workspace,project default:1.8

./xuanchu --workspace dajee config set ads.roi_threshold 2.0
./xuanchu project config set agentapi agent.background "Owns Xuanchu MCP integration."
./xuanchu project config get agentapi agent.background
./xuanchu project config list agentapi
./xuanchu project config unset agentapi agent.background
```

也可以直接管理 schema：

```bash
./xuanchu --workspace dajee config schema list
./xuanchu --workspace dajee config schema get agent.background
./xuanchu --workspace dajee config schema delete ads.roi_threshold
./xuanchu --workspace dajee config schema delete ads.roi_threshold --purge
```

说明：

- schema 严格按 workspace 隔离；不同 workspace 可以定义同名 key，但定义互不影响。
- `project config get` 的读取链是：`project 显式值 > workspace 显式值 > schema default`。
- `project config list` 和 `config list` 仍然只列当前 scope 的显式值，不展开继承结果。
- 删除 schema 时，如果当前 workspace 下还存在对应 workspace/project 值，默认返回 `config_definition_in_use`；只有显式 `--purge` 才会连值一起删。

当前系统会自动为这些 key 预置 schema：

- `agent.background`
- `agent.constraints`
- `agent.default_context`
- `agent.handoff`
- `context.default`
- `agent.provider.base_url`（workspace/project，OpenAI 兼容 Agent Provider base URL）
- `agent.provider.api_key`（workspace/project，secret，HTTP Authorization Bearer token）
- `agent.provider.model`（workspace/project，默认 model / agent id）
- `agent.provider.protocol`（workspace/project，默认 `chat_completions`）
- `agent.provider.allowed_hosts`（workspace/project，json，允许投递的 hostname 列表）
- `feishu.chat_id`（project，业务上下文，由 Agent 自行解释）

项目自动化 tab 会读取这些 config key 构造 OpenAI 兼容投递请求；`allowed_hosts` 是 SSRF 防护白名单，preview、立即测试、正式投递和 replay 都必须命中。示例：

```bash
./xuanchu --workspace dajee project config set agentapi agent.provider.base_url "https://agent.example.com"
./xuanchu --workspace dajee project config set agentapi agent.provider.api_key "sk-..."
./xuanchu --workspace dajee project config set agentapi agent.provider.model "project-operator"
./xuanchu --workspace dajee project config set agentapi agent.provider.allowed_hosts '["agent.example.com"]'
```

如果你误用无 scope 的 `config`：

```bash
./xuanchu config set agent.background "..."
```

现在会返回 `config_scope_not_allowed`，因为 `agent.background` 已有 schema，但只允许 `project` scope。

### Helper 与 audit 行为

```bash
./xuanchu _projects           # 当前 workspace 的 active project slug
./xuanchu _projects --all     # 包括 archived
./xuanchu _unique project     # 当前查询结果中实际被任务引用到的 project slug
./xuanchu audit list --project agentapi --json
```

这里有两个容易混的点：

- `_projects` 看的是 project 表，所以“已注册但暂时没有任务”的 project 也会出现。
- `_unique project` 看的是当前查询结果里的任务绑定，所以只会输出实际被命中的 project。

`audit list --project` 支持 slug 或 `project_id`，JSON 输出里会带 `project_id`，方便脚本继续串联。

## Server、Token 与远程 CLI

HTTP/JSON API、PAT / Agent token 和远程 CLI 接到同一套 app service 上。本地 CLI 可以直接打开 SQLite 或 PostgreSQL；远程 CLI 通过 HTTP API 访问服务端，不会在 remote mode 下写本机任务库。

启动服务端：

```bash
./xuanchu server --listen :8080
./xuanchu server --listen 127.0.0.1:8080 --db ./xuanchu.db
```

服务端默认启用 Web Admin Console，浏览器访问：

```text
http://127.0.0.1:8080/
```

Console 使用现有 PAT / Agent token 或 workspace 级 `tenant_access_token` 登录，token 只保存在当前浏览器 tab 的 `sessionStorage`，后续请求仍走 `/api/v1/*`。workspace owner/admin 还可以在 `/tokens` 管理租户访问令牌，供外部自动化通过 HTTP API / HTTP MCP 使用。如果需要关闭 Console：

```bash
./xuanchu server --listen :8080 --console=false
```

前端源码在 `web`，开发态可使用：

```bash
make web-console-dev
```

发布构建会先刷新 `internal/webconsole/dist`，再把静态资源嵌入单个 `xuanchu` 二进制：

```bash
make build-release
```

服务端不内置 TLS。生产部署应放在可信网络内，或使用 Nginx / Caddy 等反向代理做 TLS termination；不要把裸 HTTP token 服务直接暴露公网。服务端运行期间 SQLite 支持多进程读写排队，但生产建议同一时间只有一个主要写入口。

收到 SIGTERM / SIGINT 时，`xuanchu server` 会先停止接收新 HTTP/MCP 请求、停止 dispatcher 领取新 delivery，再等待已开始的 HTTP handler、MCP tool call、hook / notification 投递完成。`--shutdown-timeout` 控制整体 drain 时间，`--shutdown-force-timeout` 控制超时后强制取消的清理等待时间；两者也可以写在 `[server.shutdown]` 中。

### Server Admin Bootstrap

Server admin token 是服务端控制面 bootstrap token，只能访问 `/api/v1/admin/*`，不会写入 `api_tokens`，也不能访问普通任务、workspace、token API。它用于在自动化部署或 Agent 平台初始化时创建 workspace，并给指定 workspace 创建受限 Agent token。

首选初始化流程：

```toml
[server.admin]
enabled = true
```

启动 server 后，如果还没有有效 admin token，stderr 会出现类似输出：

```text
xuanchu: server admin setup required
setup-code: 9d4u...
open: http://127.0.0.1:8080/admin/setup
```

打开 `/admin/setup` 输入 setup-code，系统会生成第一个 `xuanchu_admin_...` token，并把 verifier 写入 `server_admin_tokens`。新 token 可立即用于 `/admin/login`，不需要重启服务。

旧的配置文件 verifier 仍作为兼容路径保留，但不建议作为新部署的主路径：

```toml
[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = true
```

明文丢失后不能从数据库或配置恢复，只能重新生成新的 admin token verifier。普通 API token 不能访问 admin endpoint；admin token 也不能访问普通 API。

典型 bootstrap 流程是先创建 workspace，再为该 workspace 创建 Agent token：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/admin/workspaces \
  -H "Authorization: Bearer $XUANCHU_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"team","owner":{"name":"alice","email":"alice@example.com"}}'

curl -X POST http://127.0.0.1:8080/api/v1/admin/workspaces/team/agent-tokens \
  -H "Authorization: Bearer $XUANCHU_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"openclaw","user":"alice@example.com","scopes":["*"],"expires_in":"24h"}'
```

通用 workspace Agent token 建议直接授予 `*` scope。这类 token 通常代表一个 Agent 平台在某个 workspace 内执行项目、任务、用户、成员、通知、token 管理等完整工作流；权限仍会被 token 的 workspace/project allowlist 和绑定用户的 membership role 继续收窄。创建 Agent token 的响应只会返回一次明文 `xuanchu_agent_...`，之后只能重新签发。常见错误包括：未启用 admin bootstrap 时返回 `route_not_found`，重复 workspace 返回 `admin_workspace_exists`，owner name/email 指向不同用户返回 `admin_owner_invalid`，非法 scope 返回 `token_scope_invalid`。

创建第一个 token 建议在 server 启动前用本地 CLI 完成：

```bash
./xuanchu --workspace local token create cli \
  --scope '*' \
  --expires-in 720h
```

PAT raw token 以 `xuanchu_pat_` 开头，Agent token raw token 以 `xuanchu_agent_` 开头，tenant access token raw token 以 `xuanchu_tenant_` 开头。raw token 只在创建时输出一次；数据库只保存 hash 和短 prefix。后续可以通过 HTTP/远程 CLI 管理普通 PAT / Agent token：

```bash
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" token list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" token revoke <token-id-or-prefix>
```

`tenant_access_token` 是 workspace 级 API key，也可作为 Web Console 的系统 owner 凭证使用。它不绑定用户、不支持 impersonation，也不支持 `/me`、`assignee:me`、active context 等依赖自然人 actor 的能力。它可按 scope 管理当前 workspace 内的 user、member、tenant token、workspace 设置、task link、project annotation、hook、notification sink、reminder rule 和 event notification rule；这些资源的 `created_by` / `actor` 字段会输出为系统 actor，例如 `{"type":"tenant_access_token","token":{"id":"...","name":"runtime","prefix":"xuanchu_tenant_..."}}`。它复用 `api_tokens` 表，`type=tenant_access_token` 且 `user_id=NULL`，可绑定一个 workspace、可选 project allowlist、scope、过期时间和吊销状态。HTTP API 路径：

```text
GET    /api/v1/tenant-access-tokens
POST   /api/v1/tenant-access-tokens
PATCH  /api/v1/tenant-access-tokens/{tokenRef}
DELETE /api/v1/tenant-access-tokens/{tokenRef}
```

Server admin 可在 `/api/v1/admin/tenant-access-tokens` 跨 workspace 列表、修改、吊销 tenant token；也可以在 `/api/v1/admin/workspaces/{workspace}/tenant-access-sessions` 为某个 workspace 签发默认 2 小时、最长 24 小时的短期 tenant switch token。普通长期 tenant token 仍从 workspace 视角创建。tenant token 可用于 Web Console、HTTP API 和 HTTP MCP；审计中会记录 `actor_type=tenant_access_token` 与 token id/name/prefix，不记录 raw token。

远程 CLI：

```bash
export XUANCHU_SERVER=http://127.0.0.1:8080
export XUANCHU_TOKEN=xuanchu_pat_xxx

./xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" --workspace local list
./xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" add "Review API docs" --project-id <project-id>
./xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" 1 done
```

`--server` / `--token` 也可以来自环境变量 `XUANCHU_SERVER` / `XUANCHU_TOKEN`，或本机 `xuanchu.toml`：

```toml
[remote]
server = "http://127.0.0.1:8080"
token = "xuanchu_pat_xxx"
```

如果 `xuanchu.toml` 包含 `remote.token` 且权限比 `0600` 更宽，CLI 会向 stderr 输出 warning，但不会阻止执行。推荐优先用环境变量或系统 secret manager 注入 token，不要把含 token 的 TOML 提交到公共仓库。

PAT / Agent token 的 scope 是收窄，不是放大。最终权限是：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

tenant token 没有 user principal，也不读取 membership role。它的最终权限是：

```text
tenant token capability scope ∩ token workspace scope ∩ token project scope ∩ tenant 工具/接口禁止清单
```

服务端把这条交集规则统一表达为授权决策（Authorization Decision），HTTP API、HTTP MCP、远程 CLI 共用同一个决策入口：

| 概念 | 说明 |
|---|---|
| Principal | 本次业务 user actor。普通 token 为 token 绑定用户；impersonation 为 `X-Xuanchu-As` 目标用户；tenant token 没有 Principal。 |
| Actor | 审计 actor。普通路径是 user；tenant token 路径是 `tenant_access_token` 和 token id/name/prefix。 |
| Credential | 请求凭证。PAT / Agent token 只提供 capability 和 allowlist，不能放大 membership role；tenant token 是 workspace API key。 |
| TenantScope | effective workspace 和可选 project。workspace 是隔离边界，project 是收窄边界。 |
| Decision | `principal + credential + tenant + role + request scope` 的最终授权结果。 |

授权边界错误有集中定义，HTTP status 映射也集中维护：

| 错误码 | HTTP | 语义 |
|---|---|---|
| `auth_missing_token` | 401 | 缺少 Bearer token |
| `auth_invalid_token` | 401 | token 不存在或 hash 不匹配 |
| `auth_token_revoked` | 401 | token 已吊销 |
| `auth_token_expired` | 401 | token 已过期 |
| `token_scope_denied` | 403 | token capability 不允许本操作（含 PAT/Agent 类型不满足 impersonation 要求） |
| `workspace_scope_denied` | 403 | token workspace allowlist 不允许 |
| `project_scope_denied` | 403 | token project allowlist 不允许 |
| `membership_not_found` | 403 | principal 不是 workspace 成员，或 impersonation subject 不可用 |
| `permission_denied` | 403 | membership role 不允许本操作 |
| `workspace_required` | 400 | 多 workspace 可见场景下无法安全推断 workspace |
| `tenant_actor_not_user` | 400 | tenant token 调用了依赖用户 actor 的接口或 MCP tool |

常用 capability 可通过 `xuanchu scope list` 查看。通用 workspace Agent token 推荐使用 `*`，专用集成 token 再按场景收窄：

```text
task:read task:write project:read project:write context:read context:write config:read config:write audit:read token:read token:write workspace:read workspace:write hook:read hook:write notification:read notification:write reminder:read reminder:write impersonate
```

tenant token 的 `*` 只展开 tenant 白名单：任务、项目、上下文、配置、workspace read/write、audit read、user read/write、member read/write、token read/write，以及 hook/notification/reminder read/write。它不会包含 `impersonate`。

project-scoped token 只能看 allowlist 内的任务和 audit。单任务读取如果任务存在但不在 token project allowlist 内，HTTP/远程 CLI 返回 404 `task_not_found`，避免泄露资源存在性。HTTP path 中的 `{taskRef}` 接受完整 UUID、已物化任务的 `task_slug` 或 URL 编码后的 `occurrence_ref`；projected 实例只能使用 occurrence_ref。纯数字 working-set ID 会返回 `task_ref_invalid`；远程 `info 1` 和 `1 done` 这类 working-set ID 会先由客户端两跳解析，再调用 HTTP API。

远程 CLI 覆盖核心任务、报表、project、project config、context、config、import/export、audit、token 和 helper 命令。`edit`、`config import-taskrc` 等需要本地编辑器或本机文件语义的命令在 remote mode 下暂不支持。`_unique`、`_tags` 等 helper 通过已有 list/export endpoint 在客户端后处理，大 workspace 上可能较慢。

OpenAPI 3 文档由 `internal/httpapi` 的 Huma code-first route 注册在运行时生成，不提交静态 YAML 产物。启动 server 后可以直接访问：

```text
http://127.0.0.1:8080/docs
http://127.0.0.1:8080/openapi.json
http://127.0.0.1:8080/openapi.yaml
http://127.0.0.1:8080/openapi-3.0.json
http://127.0.0.1:8080/openapi-3.0.yaml
```

## MCP Server

Agent 可以通过 MCP 协议以结构化方式使用 Xuanchu。MCP 支持 stdio 和 HTTP 两种传输方式，所有 tool 调用都经过与 CLI/API 相同的 `internal/app` service、权限和审计路径。

### MCP stdio 模式

本地 Agent 直接通过标准输入输出连接 Xuanchu：

```bash
# 本地 MCP，使用默认本地数据库
./xuanchu mcp stdio

# 指定数据库
./xuanchu --db ./xuanchu.db mcp stdio
```

stdio 模式使用本地 actor 和 workspace，不需要 token。stdout 只输出 MCP JSON-RPC 协议帧，不会混入迁移 warning 或日志。收到 SIGTERM / SIGINT 后，stdio MCP 会停止开始新的 tool call，等待已开始 tool 完成；超时后才强制取消，shutdown 日志写 stderr。

### MCP HTTP 模式

`xuanchu server` 在 `/mcp` 路径暴露 Streamable HTTP MCP endpoint：

```bash
# 启动服务端
./xuanchu server --listen :8080

# MCP 客户端连接
# POST http://127.0.0.1:8080/mcp
# Authorization: Bearer xuanchu_pat_xxx
```

HTTP MCP 需要 Bearer token 鉴权，权限规则与 REST API 一致。需要代表某个成员或使用 impersonation 时，使用 workspace-scoped Agent token；需要类似 OpenAI API key 的机器凭证时，使用 `tenant_access_token`。tenant token 可按 scope 调用任务、项目、配置、审计、user/member/token/workspace，以及 Hook、通知、提醒、task link、project annotation 等 workspace 能力；系统 actor 会进入 `created_by` / `actor` 输出。`/mcp` 不在 OpenAPI 文档中。

Web Console 的 `/tokens` 页面可从每行「MCP 配置」按钮复制 endpoint、完整 Bearer token 和客户端配置片段（服务端需配置 `[security].config_secret_key`）；admin token 和 acting token 仍不能用于 `/mcp`。

如果 HTTP MCP 通过 nginx 等反向代理暴露公网域名，推荐保留真实 Host：

```nginx
proxy_set_header Host $host;
```

当后端监听 `127.0.0.1:<port>` 或 `[::1]:<port>` 时，MCP SDK 的 localhost protection 会拒绝未配置的外部 Host。生产部署应显式配置可信域名，而不是把 Host 改写成后端地址：

```toml
[server.mcp]
trusted_proxy_hosts = ["xuanchu.example.com"]
```

### MCP tools 列表

| Tool | 说明 |
|---|---|
| `task_add` | 添加任务 |
| `task_modify` | 修改任务 |
| `task_done` | 完成任务 |
| `task_reopen` | 重新打开已完成任务 |
| `task_delete` | 删除任务 |
| `task_query` | 通用查询，支持 filter、status、limit |
| `task_get` | 按 UUID、已物化 `task_slug` 或 `occurrence_ref` 读取任务 |
| `task_annotate` | 添加注释 |
| `task_depends` | 添加依赖 |
| `task_start` | 开始任务 |
| `task_stop` | 停止任务 |
| `report_run` | 运行预定义报表 |
| `urgency_explain` | 解释 urgency 构成 |
| `workspace_list` | 列出可见 workspace |
| `workspace_get_current` | 当前 workspace |
| `project_list` | 列出当前 workspace 项目 |
| `project_get` | 读取单个项目 |
| `project_get_current` | 当前 project scope |
| `context_set` | 设置 active context |
| `context_get` | 显示 active context |
| `config_get` | 读取配置 |
| `config_set` | 写入配置 |
| `notification_sink_add` | 创建通知 sink |
| `reminder_rule_add` | 创建定时提醒规则 |
| `notification_delivery_replay` | 重放失败通知投递 |

每个 tool 成功返回 `{data, rendered}` 双格式：`data` 是结构化 JSON，`rendered` 是人类可读文本。MCP `structuredContent` 保存同一信封；`content[0].text` 也输出完整 JSON 字符串，方便只读取文本内容的 Agent 继续解析 `data`。

### MCP resources

- `xuanchu://workspace/current` — 当前 workspace 概要
- `xuanchu://workspace/{workspace_id}` — 指定 workspace 信息
- `xuanchu://project/{project_id}` — 项目元数据与 Agent 背景
- `xuanchu://context/current` — 当前 context 与 scope

### 远程管理命令

以下管理命令支持远程模式，不会触碰客户端本地数据库：

```bash
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" workspace list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" workspace add team name:Team
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" user list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" member list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" show date.format
```

## 服务端 Webhook Hook

`xuanchu server` 支持服务端 post-commit webhook hook。这里的 hook 是服务端出站 webhook，不是 Taskwarrior 的本地 shell hook。Hook 使用 workspace 级 outbound sink；`--sink <sink-ref>` 可以是当前 workspace 内的 sink 名称或 ID，不能跨 workspace 引用。

```bash
# 先创建出站 sink
./xuanchu notification sink add audit-stream \
  --type webhook \
  --url https://example.test/xuanchu

# 创建 workspace 级 hook
./xuanchu hook add task-webhook --event task.created --event task.completed --sink audit-stream

# 创建 project 级 hook
./xuanchu hook add proj-webhook --scope project --project myproject --event task.modified --sink audit-stream

# 查看 hook 列表
./xuanchu hook list

# 查看投递记录
./xuanchu hook deliveries <hook-id>

# 重试失败投递
./xuanchu hook replay <delivery-id>
```

Hook 支持的 event type：`task.created`、`task.modified`、`task.completed`、`task.deleted`、`task.started`、`task.stopped`、`task.reopened`、`task.assigned`、`task.unassigned`、`task.blocked`、`task.due_changed`、`task.priority_changed`、`task.project_changed`、`task.tags_changed`、`task.unblocked`、`project.archived`、`project.annotated`、`project.denotated`、`project.transitioned`。投递失败不会回滚已提交的 task/project 事务。生成 delivery 时会冻结 sink 渲染后的请求快照，后续 retry/replay 不重新渲染当前 sink。所有 hook 配置变更和人工 replay 都会写入 audit log。

迁移提示：Hook 不再直接保存 URL 或 secret，旧的直接 URL hook 需要先创建 notification sink，再用 `--sink <sink-ref>` 绑定。`start` 只触发 `task.started`，`stop` 只触发 `task.stopped`；如果旧集成只监听 `task.modified` 来捕获开始或停止任务，需要补充订阅这两个事件。字段级变化可以订阅对应细粒度事件，例如 `task.due_changed`、`task.priority_changed`、`task.tags_changed`、`task.blocked`、`task.unblocked`。

## 通知、提醒与第三方通知

通知系统复用 notification sink，但规则分两类：

- reminder rule：定时扫描任务过滤器，适合到期前和逾期后的提醒。
- notification rule：监听事件并解析 audience，适合 `task.unblocked`、项目 annotation 等事件通知。

管理员先创建 notification sink，再创建 reminder rule 或 notification rule；`xuanchu server` 的后台 scheduler / dispatcher 命中规则后生成 delivery。

```bash
./xuanchu notification sink add openclaw \
  --type webhook \
  --url https://openclaw.example.com/xuanchu/notifications \
  --secret "$WEBHOOK_SECRET" \
  --max-concurrency 0

./xuanchu reminder rule add due-soon-24h \
  --schedule daily@08:50 \
  --filter 'end.isnull and start.isnull and due.after:now and due.before:now+24h' \
  --audience assignees \
  --sink openclaw

./xuanchu reminder rule add overdue-daily \
  --schedule daily@09:00 \
  --filter 'status:pending and end.isnull and due.before:now' \
  --repeat every:24h \
  --audience assignees \
  --sink openclaw

./xuanchu notification rule add task-unblocked \
  --event task.unblocked \
  --audience assignees \
  --sink openclaw

./xuanchu notification delivery list --status dead_lettered
./xuanchu notification delivery replay <delivery-id>
```

Notification rule 支持的事件类型和 Hook 当前白名单一致：`task.created`、`task.modified`、`task.completed`、`task.deleted`、`task.started`、`task.stopped`、`task.reopened`、`task.assigned`、`task.unassigned`、`task.blocked`、`task.due_changed`、`task.priority_changed`、`task.project_changed`、`task.tags_changed`、`task.unblocked`、`project.archived`、`project.annotated`、`project.denotated`、`project.transitioned`。第三方固定 Web API 使用 `http_template` sink。header/body 模板保存在数据库中，secret 通过 secret config 引用；生成 delivery 时会冻结 `resolved_url`、header、body 和 content type，retry/replay 不重新渲染当前模板。

notification / hook delivery 表是出站投递的可靠队列；进程内 worker 只做短暂执行协调。dispatcher 默认 `max_concurrency=1`，`batch_size` 只是每轮查询上限。sink 的 `max_concurrency=0` 表示继承默认 sink 并发；`xuanchu server` 内 notification dispatcher 和 hook dispatcher 共享同一个 sink limiter。同一个 delivery payload 会带稳定 `delivery_id`，接收方可据此幂等去重；本次 HTTP 请求真实尝试次数看 `X-Xuanchu-Attempt` header。详见 [定时通知与第三方通知](docs/manual/notifications.md)。

事件通知建议优先订阅具体语义事件，减少对宽泛 `task.modified` 的依赖。`assignees` 和 `assignees_and_explicit_users` audience 只适用于 `task.*` 事件；project 事件应使用 `actor` 或 `explicit_users`。

服务停机时，dispatcher 不会再领取新的 delivery；已经开始的投递会在 `server.shutdown.timeout` 内继续执行。已领取但尚未开始的 delivery 会尽快回到队列，超时强制取消时仍由数据库中的 stale recovery 兜底。

## Impersonation

Agent 平台可以持有一个带 `impersonate` scope 的 Agent token，以 workspace 成员的身份发起请求。

```bash
# 创建可 impersonate 的 Agent token（需要 admin/owner）
./xuanchu --workspace local token create pm-agent \
  --type agent \
  --scope '*' \
  --expires-in 8760h

# 远程 CLI 使用 impersonation
./xuanchu --server http://127.0.0.1:8080 \
  --token "$AGENT_TOKEN" \
  --workspace local \
  --as alice \
  list assignee:me
```

HTTP API 通过 `X-Xuanchu-As` header：

```
GET /api/v1/tasks
Authorization: Bearer xuanchu_agent_...
X-Xuanchu-As: alice
```

权限以目标用户的 membership role 与 token scope 的交集为准，impersonation 不能提权。Token 可见多个 workspace 且请求未显式指定 workspace 时返回 `workspace_required`。目标用户不存在或不是成员时返回 `membership_not_found`。

Audit log 同时记录 `actor_user_id`（目标用户）和 `delegator_token_id`/`delegator_user_id`（发起方 Agent token）。

HTTP MCP 的每个 tool call 都支持 `X-Xuanchu-As` header 透传。stdio MCP 不支持 impersonation。

部署、TLS、备份恢复请参考 [`docs/deployment.md`](./docs/deployment.md) 和 [`docs/backup-restore.md`](./docs/backup-restore.md)。
