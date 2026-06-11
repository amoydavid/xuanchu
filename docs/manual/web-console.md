---
title: "Web Admin Console"
weight: 85
---

# Web Admin Console

Xuanchu Web Admin Console 是 `xuanchu server` 内置的运维入口。它不引入第二个服务进程，也不绕过 HTTP API、权限、workspace / project allowlist、token scope 或 audit。

默认访问地址：

```text
http://127.0.0.1:8080/console
```

默认配置：

```toml
[server.console]
enabled = true
base_path = "/console"
assets_cache = "1h"
auth_mode = "bearer"
```

也可以用 server flag 覆盖：

```bash
xuanchu server --listen :8080 --console=false
xuanchu server --listen :8080 --console-base-path /admin
```

`base_path` 必须和前端构建时的 Vite base 一致。官方默认构建只保证 `/console`。如果要挂到 `/admin`，需要重新生成 dist 后再构建二进制：

```bash
cd web
VITE_XUANCHU_CONSOLE_BASE=/admin/ pnpm build
cd ..
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 登录与 token

v0.4.0 只支持 Bearer token 登录。登录页要求输入 Xuanchu PAT 或 Agent token，前端会把 token 放入当前 tab 的 `sessionStorage`，不会写入 `localStorage`。

后续 API 请求使用：

```text
Authorization: Bearer <token>
```

Console 没有特殊超级权限。实际权限仍然是：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

通用 workspace Agent token 可以使用 `--scope '*'`，再通过 workspace/project allowlist 和成员角色收窄实际权限。只做单一自动化的 token 仍应使用最小 scope。

## 前端开发

前端源码在仓库根目录的 `web`，使用 pnpm、Vite 8、React、shadcn/ui 和 i18n。

常用命令：

```bash
make web-console-dev
make web-console-check
make web-console-build
```

开发服务器会代理 `/api/v1/*` 到 `VITE_XUANCHU_API_TARGET`，默认是 `http://127.0.0.1:8080`。

## 部署安全

- 生产环境必须放在可信网络内，或通过反向代理提供 TLS。
- 不要把裸 HTTP token 服务直接暴露到公网。
- 不要在共享浏览器或非可信设备中输入高权限 token。
- 如果反向代理同时暴露 `/mcp`，仍需要按 MCP Host allowlist 配置可信公网 Host。
- Console 静态资源通过 Go `embed` 打入二进制，运行时不需要 Node.js、pnpm 或 Vite。
