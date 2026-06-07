---
title: "常见问题与排障"
weight: 120
---

# 常见问题与排障

## project_not_found

任务引用 project 前必须先注册 project：

```bash
xuanchu project add agentapi name:"AI Agent Platform"
xuanchu add "Write docs" project:agentapi
```

如果你指定了 `--workspace`，project 会在该 workspace 内解析：

```bash
xuanchu --workspace dajee project list
```

## project_archived

归档 project 不能再被新任务引用。已有任务仍可读取、完成和删除。

解决方式：

- 使用未归档 project。
- 或创建新 project。

## task_not_found

可能原因：

- UUID 不存在。
- `task_slug` 不存在，或任务已经被移动到其他 project 后旧 `task_slug` 失效。
- working-set ID 不是当前报表里的有效 ID。
- 远程 project-scoped token 没有权限访问该任务。

远程模式下，越界单任务读取会返回 `task_not_found`，这是为了避免泄露资源存在性。

## workspace_scope_denied

token 不允许访问该 workspace，或 token 可见多个 workspace 但请求没有明确 workspace。

解决方式：

```bash
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" --workspace dajee list
```

或创建带正确 workspace scope 的 token。

## project_scope_denied

token 带 project allowlist，不能访问 scope 外 project。

解决方式：

- 使用允许该 project 的 token。
- 或在创建 token 时加入 `--project <slug>` / `--project-id <uuid>`。

## remote_unsupported_command

远程模式不支持依赖本机文件或编辑器语义的命令，例如：

- `edit`
- `config import-taskrc`

请在本地模式运行这些命令，或改用 HTTP/API 支持的操作。

## auth_missing_token / auth_invalid_token

HTTP API、远程 CLI、HTTP MCP 都需要 Bearer token。

```bash
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" list
curl -H "Authorization: Bearer $XUANCHU_TOKEN" https://xuanchu.example.com/api/v1/me
```

如果 token 过期或被撤销，请重新创建 token。

## hook_endpoint_invalid

常见原因：

- URL 为空。
- scheme 不是 `http` 或 `https`。
- host 无法解析。
- endpoint 解析到 loopback、私网、link-local、multicast 或 unspecified 地址。

Hook endpoint 必须是 dispatcher 可以访问的公网或允许的外部地址。

## hook_delivery_not_replayable

只有可 replay 的 delivery 状态才能手动重试。先查看 delivery：

```bash
xuanchu hook deliveries <hook-id> --json
```

## JSON 输出和 human 输出不一致

human 输出为了阅读友好，可能省略或格式化字段。脚本应使用：

```bash
xuanchu --json <command>
xuanchu _get ...
xuanchu _ids ...
xuanchu _uuids ...
```

## 数据库路径不对

查看当前路径：

```bash
xuanchu _show database.path
```

常见覆盖方式：

```bash
xuanchu --db ./xuanchu.db list
XUANCHU_DB=./xuanchu.db xuanchu list
xuanchu --data-dir ./data list
```

## PostgreSQL 连接失败

如果使用 `--db-url` 连接 PostgreSQL 失败：

- 检查连接字符串格式：`postgres://user:pass@host:5432/dbname?sslmode=disable`
- 检查 PostgreSQL 是否运行：`pg_isready -h localhost -p 5432`
- 检查用户权限：数据库必须已创建，用户必须有 CREATE TABLE 权限
- 查看当前配置：`xuanchu _show database.url`

`--db-url` 和 `--db` 互斥，同时指定会报错。
