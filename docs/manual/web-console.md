---
title: "Web Admin Console"
weight: 85
---

# Web Admin Console

Xuanchu Web Admin Console 是 `xuanchu server` 内置的运维入口。它不引入第二个服务进程，也不绕过 HTTP API、权限、workspace / project allowlist、token scope 或 audit。

默认访问地址：

```text
http://127.0.0.1:8080/
```

默认配置：

```toml
[server.console]
enabled = true
base_path = "/"
assets_cache = "1h"
auth_mode = "bearer"
```

也可以用 server flag 覆盖：

```bash
xuanchu server --listen :8080 --console=false
xuanchu server --listen :8080 --console-base-path /admin
```

`base_path` 必须和前端构建时的 Vite base 一致。官方默认构建只保证 `/`。如果要挂到 `/admin`，需要重新生成 dist 后再构建二进制：

```bash
cd web
VITE_XUANCHU_CONSOLE_BASE=/admin/ pnpm build
cd ..
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 登录与 token

普通 Console 入口是 `/`。登录页要求输入 Xuanchu PAT 或 Agent token，前端会把 token 放入当前 tab 的 `sessionStorage["xuanchu.console.token"]`，不会写入 `localStorage`。`tenant_access_token` 是机器访问用 API key，不作为浏览器登录凭证。

后续 API 请求使用：

```text
Authorization: Bearer <token>
```

Console 没有特殊超级权限。实际权限仍然是：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

普通 Console 的 token 登录不是浏览器 SSO。它只是把 PAT / Agent token 放入当前 tab 的 `sessionStorage`；服务端仍按 Bearer token 走同一套 Authorization Decision，没有独立的浏览器会话或 cookie。未来的 OIDC / 飞书 OAuth 登录会引入独立的 browser session 凭证类型，权限仍由本地 membership role 决定。

Server admin token（`xuanchu_admin_` 前缀）走独立的 `/api/v1/admin/*` 控制面中间件，不进入普通业务 Authorization Decision：它不能访问任务、项目、通知、Hook 或 MCP 接口，只用于部署期创建 workspace 和 workspace-scoped Agent token。普通 PAT / Agent token 同样不能访问 admin 控制面。

通用 workspace Agent token 可以使用 `--scope '*'`，再通过 workspace/project allowlist 和成员角色收窄实际权限。只做单一自动化的 token 仍应使用最小 scope。

`/tokens` 页面包含两个 tab：

- 普通 API Tokens：管理 PAT / Agent token，适合用户或需要绑定成员身份的 Agent。
- 租户访问令牌：管理 `tenant_access_token`。它绑定当前 workspace，不绑定用户，raw token 前缀为 `xuanchu_tenant_`，用于 HTTP API / HTTP MCP 的机器访问；明文只在创建成功后显示一次。

tenant token 的 scope 选择器只展示后端允许的租户白名单。它不能获得 `token:*`、`user:*`、`member:*`、`workspace:write` 或 `impersonate`，也不能调用 `/me`、`me_get`、active context 写入、创建带用户 `created_by` / `actor` 的资源等依赖用户 actor 的接口。

## 项目工作台链接

普通 Console 支持面向企业协作工具的项目工作台深链：

```text
http://127.0.0.1:8080/workspaces/{workspaceSlug}/projects/{projectSlug}
```

该 URL 明确带上 workspace 层级，因为 project slug 只在同一 workspace 内唯一。页面只读取现有 HTTP API：

```text
GET /api/v1/projects/{projectSlug}?workspace={workspaceSlug}
GET /api/v1/tasks?workspace={workspaceSlug}&project={projectSlug}&limit=200
GET /api/v1/tasks/{taskRef}?workspace={workspaceSlug}
```

项目工作台展示项目元数据、任务状态摘要、负责人摘要、过滤工具栏和任务列表。具备 `task:write` 的 owner/admin/member 可以在项目内快速创建任务，也可以点击“导入任务”上传 JSON / XLSX 批量导入。导入弹窗会先做浏览器端预检：

- JSON 支持标准任务数组，或包含 `tasks` 数组的对象。
- 导入弹窗提供“查看 JSON Schema”，展示带 `description` 注释的完整 JSON 字段约束，方便上传前核对。
- XLSX 可以先下载模板，填写 `id`、`title`、`description`、`assignees`、`assignee_display_names`、`assignee_emails`、`blocked_by` 和 `uda.*` 字段后上传；模板还包含 `status`、日期、循环、父任务、注解、链接等可选字段。`assignees` 是稳定用户引用，`assignee_display_names` / `assignee_emails` 会按顺序补充展示姓名和邮箱，预检创建缺失用户时会写入 `display_name`。
- XLSX 模板包含「字段说明」sheet，逐列列出 required、type、allowed values、format 和 example；Tasks sheet 只保留字段列、示例行、筛选和日期格式提示，不使用表头批注或文本框承载字段说明。
- `description` 默认按 Markdown 编写，技术上仍是普通字符串。
- `id` / `import_id` 是导入文件内的临时引用，只要求是批次内不重复的字符串，不要求 UUID 格式；导入后会生成新的任务 UUID，临时 ID 不入库。
- 指派人必须能解析为当前 workspace 成员；如果普通邮箱或姓名对应的用户尚未创建或尚未加入 workspace，预检区可选择直接创建/加入为 `member`。JSON 中的 assignee 对象可以包含 `name`、`display_name`、`email`、`user_id`；`display_name` 只用于展示和创建用户时保留昵称，不替代稳定引用名。
- `blocked_by` 表示当前任务被哪些任务阻塞，可填写导入文件内的临时 `id` 或当前项目已有任务 UUID，多个值用逗号分隔。
- 上传解析后，预检区会以分页表格预览解析和本地引用映射后的导入成果；最终导入内容应与该预览一致，服务端生成的 `id` / `seq` 除外。
- 最终写入仍由 `/api/v1/import` 整批原子提交，服务端权限和项目禁写状态是最终裁决。

用户可以点击任务标识或任务描述进入任务详情页：

```text
http://127.0.0.1:8080/workspaces/{workspaceSlug}/projects/{projectSlug}/tasks/{taskRef}
```

任务详情页展示任务字段、负责人、标签、依赖和注记，并提供“返回项目”链接回到项目页。description 在详情页完整展示，具备写权限时点击“编辑描述”打开弹窗编辑完整内容。具备写权限时，项目页和任务详情页还提供任务编辑、完成、删除、注解和链接操作；不提供拖拽看板。页面数据必须来自真实 API 响应；空项目展示空状态，不使用假数据。

未登录访问项目 deep link 时，登录页会显示“登录后继续访问”的相对路径。用户输入 PAT / Agent token 登录成功后回到原项目页。当前版本仅预留 redirect 语义，不接入企业 SSO、飞书 OAuth、cookie session 或匿名分享链接。

## Server Admin Bootstrap

Server admin 入口是独立页面：

```text
http://127.0.0.1:8080/admin/login
```

Server admin token 不是 PAT，也不是 Agent token。它只用于 server control-plane bootstrap，前端保存在当前 tab 的 `sessionStorage["xuanchu.console.admin_token"]`，并且只会发送到 `/api/v1/admin/*`。

当前 admin 页面只覆盖初始化闭环：

- 创建 workspace。
- 创建或提升 workspace 管理员。
- 为该管理员创建 workspace-scoped Agent token。

首选初始化方式是启用 `[server.admin]` 后启动 server。如果数据库里还没有有效 admin token，server 会在 stderr 输出一次性 setup-code，并提示打开 `/admin/setup`。在页面输入 setup-code 后，服务端生成第一个 `xuanchu_admin_...` token，把 SHA-256 verifier 写入数据库；raw token 只在页面显示一次，不会自动写入浏览器 session。

普通 PAT / Agent token 不能访问 `/api/v1/admin/*`；admin token 也不能访问 `/api/v1/tasks`、`/api/v1/projects`、`/api/v1/me` 等普通 workspace API。

`xuanchu admin token generate/hash` 仍保留为兼容和运维工具。旧的 `[[server.admin.tokens]]` / `hash_env` 配置 verifier 可以继续使用，但新部署优先使用 `/admin/setup`，避免为了新增 admin token 修改配置并重启。

Admin 页面创建的 Agent token 明文只在创建结果里显示一次。页面不会把 raw Agent token 写入 storage；关闭结果或刷新页面后无法恢复。

## Workspace 管理与 acting mode

server admin 登录后，`/admin/workspaces` 提供 workspace 控制面：

- 列出全部 workspace（默认只显示未归档，可勾选「显示已归档」）。
- workspace 详情展示 owner/admin/member 摘要和 token 计数（有效 / 已吊销 / 已过期），并允许编辑成员 `display_name`。`users.name` 仍是稳定用户引用名，`display_name` 只用于展示姓名，可与 `name` 不同。
- 从任意未归档 workspace 创建短期 acting session，以该 workspace 的 owner/admin 身份进入普通 Workspace Console。

acting session 是**短期浏览器委托凭证**，不是长期 token：

- server admin 调用 `POST /api/v1/admin/workspaces/{workspace}/acting-sessions` 创建。服务端签发 `xuanchu_act_...` token，raw token 只在创建响应中返回一次，数据库只保存 SHA-256 hash。
- acting token 只保存在当前浏览器 tab 的 `sessionStorage["xuanchu.console.admin_acting_token"]`，与普通 workspace token（`xuanchu.console.token`）和 server admin token（`xuanchu.console.admin_token`）分开清理。
- acting token 绑定单一 workspace；普通 HTTP API 接受它，但 HTTP MCP、stdio MCP 和 remote CLI client 都拒绝它（remote CLI 在客户端侧直接拒绝 `xuanchu_act_` 前缀，不发请求）。
- acting token 的能力上限是 Workspace Console 的全部 scope（剥离 `impersonate`）；实际访问还由绑定 actor 的**当前** workspace membership role 和 project allowlist 收窄。session 中保存的 role 只是审计展示用的快照，不作为后续授权来源。
- 默认 TTL 2 小时。
- acting mode 下普通 workspace 操作仍以被切换的用户作为 actor，但 audit 额外记录 `admin_acting_session_id`、`delegator_admin_token_id`、`delegator_admin_token_name`，可追溯到发起委托的 server admin。
- Workspace Console 顶部持续显示 acting banner，提醒当前以哪个 workspace 的哪个管理员身份操作，并提供「返回超管界面」按钮。
- 「返回超管界面」清理 acting token + acting context，**保留** server admin token，并跳回 `/admin/workspaces/{workspace}`。
- acting token 过期或被吊销后，普通 API 返回 `admin_acting_session_expired` 或 `auth_invalid_token`，前端清理 acting session。

## Admin Token 管控

server admin 登录后，`/admin/tokens` 提供跨 workspace 的 token 管控：

- 普通 API Tokens tab 列出 PAT / Agent token（含 user、workspace、scope、状态），可吊销失控 token、修改 name / scope / 过期时间。
- 租户访问令牌 tab 列出 `tenant_access_token`（含 workspace、prefix、scope、状态），可修改 name / scope / project allowlist / 过期时间并吊销。
- admin 不创建普通 token 或 tenant token；普通 token 创建走 bootstrap 或 Workspace Console，tenant token 创建走 Workspace Console。
- admin 操作绕过 workspace role 校验，但所有变更记录到审计日志（`admin.token.modify` / `admin.token.revoke` / `admin.tenant_token.modify` / `admin.tenant_token.revoke`）。

## 前端开发

前端源码在仓库根目录的 `web`，使用 pnpm、Vite 8、React、shadcn/ui 和 i18n。

仓库不跟踪 `internal/webconsole/dist` 下的真实构建产物，只保留 `internal/webconsole/dist/.gitkeep` 让 Go embed 路径稳定存在。这样可以避免 Vite hash 文件和拆包产物给 Git 带来噪音。

常用命令：

```bash
make web-console-dev
make web-console-check
make web-console-build
```

开发服务器会代理 `/api/v1/*` 到 `VITE_XUANCHU_API_TARGET`，默认是 `http://127.0.0.1:8080`。

日常开发建议使用 `make web-console-dev`，前端由 Vite dev server 提供，Go server 只需要提供 HTTP API。此时不需要生成 `internal/webconsole/dist`。

发布或验证嵌入式 Console 时必须运行：

```bash
make web-console-build
```

该命令会重新生成被 Git 忽略的 `internal/webconsole/dist`。随后执行 `go build` 或 `make build-release` 时，Go `embed` 会把当前 dist 内容打进二进制。`make build-release` 已经依赖 `web-console-build`，因此 release 包会自动包含最新前端资源。

## 部署安全

- 生产环境必须放在可信网络内，或通过反向代理提供 TLS。
- 不要把裸 HTTP token 服务直接暴露到公网。
- 不要在共享浏览器或非可信设备中输入高权限 token。
- 不要把 server admin token 当成普通用户 token 使用；它只应该用于初始化和紧急控制面操作。
- 如果反向代理同时暴露 `/mcp`，仍需要按 MCP Host allowlist 配置可信公网 Host。
- Console 静态资源通过 Go `embed` 打入二进制，运行时不需要 Node.js、pnpm 或 Vite。
