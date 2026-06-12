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

普通 Console 入口是 `/`。登录页要求输入 Xuanchu PAT 或 Agent token，前端会把 token 放入当前 tab 的 `sessionStorage["xuanchu.console.token"]`，不会写入 `localStorage`。

后续 API 请求使用：

```text
Authorization: Bearer <token>
```

Console 没有特殊超级权限。实际权限仍然是：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

通用 workspace Agent token 可以使用 `--scope '*'`，再通过 workspace/project allowlist 和成员角色收窄实际权限。只做单一自动化的 token 仍应使用最小 scope。

## 项目只读链接

普通 Console 支持面向企业协作工具的项目只读页：

```text
http://127.0.0.1:8080/workspaces/{workspaceSlug}/projects/{projectSlug}
```

该 URL 明确带上 workspace 层级，因为 project slug 只在同一 workspace 内唯一。页面只读取现有 HTTP API：

```text
GET /api/v1/projects/{projectSlug}?workspace={workspaceSlug}
GET /api/v1/tasks?workspace={workspaceSlug}&project={projectSlug}&limit=200
GET /api/v1/tasks/{taskRef}?workspace={workspaceSlug}
```

项目只读页展示项目元数据、任务状态摘要、负责人摘要和任务列表。用户可以点击任务标识或任务描述进入只读任务详情页：

```text
http://127.0.0.1:8080/workspaces/{workspaceSlug}/projects/{projectSlug}/tasks/{taskRef}
```

任务详情页展示任务字段、负责人、标签、依赖和注记，并提供“返回项目”链接回到项目页。项目页和任务详情页都不提供任务编辑、完成、删除或拖拽看板。页面数据必须来自真实 API 响应；空项目展示空状态，不使用假数据。

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
