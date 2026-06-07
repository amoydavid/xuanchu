# Xuanchu 服务端部署指南

## 启动服务

```bash
xuanchu server --listen :8080
xuanchu server --listen 127.0.0.1:8080 --db ./xuanchu.db
```

服务端不内置 TLS。生产部署应放在可信网络内，或使用反向代理做 TLS termination。

## 反向代理 / TLS

建议使用 nginx 或 caddy 做 TLS termination：

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

## Webhook 出站网络

xuanchu dispatcher 需要访问外部 webhook URL。默认禁止投递到以下地址：

- Loopback：`127.0.0.0/8`、`::1/128`
- Link-local：`169.254.0.0/16`、`fe80::/10`
- RFC1918：`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`
- RFC6598：`100.64.0.0/10`
- Multicast：`224.0.0.0/4`、`ff00::/8`
- Unspecified：`0.0.0.0/8`、`::`

HTTP 3xx redirect 不会被自动跟随。

## Token 配置

建议使用最小 scope 原则：

```bash
# 管理 Hook
./xuanchu token create hook-admin --scope hook:read,hook:write --expires-in 720h

# 只读 Hook
./xuanchu token create hook-viewer --scope hook:read --expires-in 720h
```

避免使用全 scope token，减少 token 泄露时的攻击面。

## Secret 安全

- Secret 不会出现在 CLI/HTTP response、audit log、server log 中
- 建议使用 `--secret-stdin` 或 `--secret-file`，避免 `--secret` 参数进入 shell history / process list / CI log：

```bash
# 从 stdin 读取 secret
echo "my-secret-key" | ./xuanchu hook add my-hook --secret-stdin --event task.created --url https://example.test/hook

# 从文件读取 secret
./xuanchu hook add my-hook --secret-file /run/secrets/hook-secret --event task.created --url https://example.test/hook
```

- Hook secret 会随 SQLite 数据库和 `VACUUM INTO` 备份保存
- 建议数据库文件和备份文件权限为 `0600`：

```bash
chmod 600 ~/.local/share/xuanchu/xuanchu.db
chmod 600 /path/to/backup.db
```

## Webhook 签名验证

消费方应使用 `X-Xuanchu-Signature-256` header 验证请求真实性。

签名输入：

```
<delivery_id>.<timestamp_unix_seconds>.<body>
```

签名格式：

```
sha256=<hex>
```

示例验证代码：

```python
import hmac, hashlib

def verify_signature(secret, delivery_id, timestamp, body, signature):
    signing_input = f"{delivery_id}.{timestamp}.{body}"
    expected = "sha256=" + hmac.new(
        secret.encode(), signing_input.encode(), hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)
```

建议拒绝超过 5 分钟窗口的请求，防止重放攻击。

## 服务端运行注意事项

- 服务端运行期间 SQLite 支持多进程读写排队，但生产建议同一时间只有一个主要写入口
- `xuanchu dispatcher` 在 server 启动后自动运行，负责 webhook 出站投递
- Hook 投递失败不会回滚已提交的 task/project 事务
- Dead-lettered 投递可通过 `xuanchu hook replay <delivery-id>` 手动重试
