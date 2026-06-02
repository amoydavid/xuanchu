---
title: "部署指南"
weight: 100
---

# 部署指南

## 启动服务

```bash
taskg server --listen :8080
taskg server --listen 127.0.0.1:8080 --db ./taskg.db
```

服务端不内置 TLS。生产部署应放在可信网络内，或使用反向代理做 TLS termination。

## 反向代理 / TLS

nginx 示例：

```nginx
server {
    listen 443 ssl;
    server_name taskg.example.com;

    ssl_certificate     /etc/ssl/certs/taskg.pem;
    ssl_certificate_key /etc/ssl/private/taskg.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Token 配置

建议使用最小 scope 原则：

```bash
taskg token create hook-admin \
  --scope hook:read,hook:write \
  --expires-in 720h

taskg token create readonly \
  --scope task:read,project:read,context:read,config:read \
  --expires-in 720h
```

避免使用全 scope token，减少 token 泄露时的攻击面。

## Secret 安全

建议使用 `--secret-stdin` 或 `--secret-file`，避免 secret 进入 shell history、process list 或 CI log。

```bash
echo "my-secret-key" | taskg hook add my-hook \
  --secret-stdin \
  --event task.created \
  --url https://example.test/hook

taskg hook add my-hook \
  --secret-file /run/secrets/hook-secret \
  --event task.created \
  --url https://example.test/hook
```

数据库文件和备份文件建议权限为 `0600`：

```bash
chmod 600 ~/.local/share/taskg/taskg.db
chmod 600 /path/to/backup.db
```

## Webhook 出站网络

dispatcher 需要访问外部 webhook URL。默认禁止投递到私网、loopback、link-local、multicast、unspecified 等地址。

HTTP 3xx redirect 不会被自动跟随。

## 服务端运行注意事项

- 服务端运行期间 SQLite 支持多进程读写排队，但生产建议同一时间只有一个主要写入口。
- `taskg server` 启动后会自动运行 webhook dispatcher。
- Hook 投递失败不会回滚已提交的 task/project 事务。
- Dead-lettered delivery 可通过 `taskg hook replay <delivery-id>` 手动重试。

## 发布构建

项目保持零 CGO。发布构建应验证：

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```

如需交叉编译，可使用仓库中的发布脚本：

```bash
scripts/release-build.sh
```

