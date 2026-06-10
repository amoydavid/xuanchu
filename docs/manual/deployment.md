---
title: "部署指南"
weight: 100
---

# 部署指南

## 启动服务

```bash
xuanchu server --listen :8080
xuanchu server --listen 127.0.0.1:8080 --db ./xuanchu.db
xuanchu server --listen :8080 --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable"
```

服务端不内置 TLS。生产部署应放在可信网络内，或使用反向代理做 TLS termination。

## 反向代理 / TLS

nginx 示例：

```nginx
server {
    listen 443 ssl;
    server_name xuanchu.example.com;

    ssl_certificate     /etc/ssl/certs/xuanchu.pem;
    ssl_certificate_key /etc/ssl/private/xuanchu.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Token 配置

通用 workspace Agent token 建议使用全量 scope，再用 workspace/project allowlist 和绑定用户的 membership role 收窄实际权限：

```bash
xuanchu --workspace dajee token create mcp-agent \
  --type agent \
  --scope '*' \
  --expires-in 720h
```

专用自动化 token 仍建议使用最小 scope 原则：

```bash
xuanchu token create hook-admin \
  --scope hook:read,hook:write \
  --expires-in 720h

xuanchu token create readonly \
  --scope task:read,project:read,context:read,config:read \
  --expires-in 720h
```

避免把全 scope token 用在只做单一工作的脚本里，减少 token 泄露时的攻击面。

使用 `xuanchu scope list` 查看所有可用 scope。通配符可以简化 scope 配置：

```bash
xuanchu token create admin-token --scope '*' --expires-in 720h
xuanchu token create reader --scope '*:read' --expires-in 720h
xuanchu token create task-agent --scope 'task:*' --expires-in 720h
```

## Secret 安全

Hook 本身不再保存 URL 或 secret；出站 endpoint 和签名 secret 由 workspace 级 notification sink 管理。避免 secret 进入 shell history、process list 或 CI log，建议通过环境变量、secret manager 或 secret config 注入。

```bash
xuanchu notification sink add audit-stream \
  --type webhook \
  --url https://example.test/hook \
  --secret "$WEBHOOK_SECRET"

xuanchu hook add my-hook \
  --event task.created \
  --sink audit-stream
```

数据库文件和备份文件建议权限为 `0600`：

```bash
chmod 600 ~/.local/share/xuanchu/xuanchu.db
chmod 600 /path/to/backup.db
```

## 出站网络

hook dispatcher 和 notification dispatcher 需要访问外部 webhook / HTTP template URL。默认禁止投递到私网、loopback、link-local、multicast、unspecified 等地址。

HTTP 3xx redirect 不会被自动跟随。

## 服务端运行注意事项

- 服务端运行期间 SQLite 支持多进程读写排队，但生产建议同一时间只有一个主要写入口。
- `xuanchu server` 启动后会自动运行 webhook dispatcher。
- `xuanchu server` 启动后会自动运行 reminder scheduler 和 notification dispatcher。
- Hook 投递失败不会回滚已提交的 task/project 事务。
- Notification 投递失败不会修改任务状态。
- Hook / Notification delivery 表是可靠队列；进程内 worker 只做短暂执行协调。`batch_size` 是每轮查询上限，`max_concurrency` 才是同时出站 HTTP 请求数。
- 单实例默认 `max_concurrency=1`。多实例部署时，总体出站并发约等于单实例配置乘以副本数；首版不提供跨实例严格全局并发。
- notification dispatcher 和 hook dispatcher 在同一个 server 进程内共享 sink limiter，同一 sink 的单进程并发不会因为两个 runtime 同时运行而翻倍。
- Dead-lettered delivery 可通过 `xuanchu hook replay <delivery-id>` 手动重试。
- Dead-lettered notification delivery 可通过 `xuanchu notification delivery replay <delivery-id>` 手动重试。

## 可靠停机

`xuanchu server` 收到 SIGTERM / SIGINT 后会进入两阶段停机：

1. 停止接收新 HTTP / MCP 请求，停止 hook / notification dispatcher 领取新的 delivery。
2. 在 `server.shutdown.timeout` 内等待已开始的 HTTP handler、MCP tool call 和出站投递自然完成。
3. 超时后触发强制取消，再用 `server.shutdown.force_timeout` 等待运行时清理。

示例配置：

```toml
[server.shutdown]
timeout = "30s"
force_timeout = "5s"
```

systemd 示例：

```ini
[Unit]
Description=Xuanchu task server
After=network-online.target

[Service]
ExecStart=/usr/local/bin/xuanchu --config /etc/xuanchu/config.toml server --listen :8080
KillSignal=SIGTERM
TimeoutStopSec=45s
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

`TimeoutStopSec` 应大于 `server.shutdown.timeout + server.shutdown.force_timeout`，否则 systemd 可能在 xuanchu 自己完成 drain 前发送 SIGKILL。容器平台的 termination grace period 也按同样规则设置。

已领取但尚未开始投递的 delivery 会尽快回到队列；进程异常退出或强制取消时，仍由数据库 stale recovery 兜底，因此接收方应继续按 `delivery_id` 做幂等。

## PostgreSQL 部署

使用 `--db-url` 指定 PostgreSQL 连接字符串：

```bash
xuanchu server --listen :8080 --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable"
```

也可以通过环境变量或 TOML 配置：

```bash
export XUANCHU_DB_URL="postgres://user:pass@localhost:5432/xuanchu"
xuanchu server --listen :8080
```

```toml
[database]
url = "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable"
```

`--db-url` 和 `--db` 互斥。PostgreSQL 模式下会跳过 SQLite 历史迁移（M4/M5），使用 `AutoMigrate` 直接建表。

PostgreSQL 备份请使用 `pg_dump`，不要使用 SQLite 备份命令。

## 发布构建

项目保持零 CGO。发布构建应验证：

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

如需交叉编译，可使用仓库中的发布脚本：

```bash
scripts/release-build.sh
```
