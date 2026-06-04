---
title: "配置、Context 与 UDA"
weight: 50
---

# 配置、Context 与 UDA

## 配置来源

taskg 的配置可以来自：

- CLI flag / `rc.*`
- 环境变量
- SQLite meta/config
- `~/.config/taskg/taskg.toml`
- 默认值

本机配置和业务配置要分开理解：

- `taskg.toml` 是本机启动和显示配置。
- workspace/project 业务配置存数据库，并受权限和 audit 管理。

## taskg.toml

最小示例：

```toml
[database]
path = "/Users/me/.local/share/taskg/taskg.db"

[display]
color = true
json = false

[date]
format = "rfc3339"
```

建议写入 TOML 的 key：

| TOML | 展开后 | 说明 |
|---|---|---|
| `[database] path = "..."` | `database.path` | SQLite 路径 |
| `[database] url = "postgres://..."` | `database.url` | PostgreSQL 连接字符串，与 `database.path` 互斥 |
| `[display] color = true` | `color` | human 输出颜色 |
| `[display] json = false` | `json` | 默认 JSON 输出 |
| `[date] format = "rfc3339"` | `date.format` | 日期输出格式 |

不要把 `uda.*`、`urgency.uda.*`、`context.<name>`、project defaults、hook defaults 长期写进 TOML。这些属于业务配置，应通过 CLI/API 写入数据库。

## config 命令

```bash
taskg show
taskg config get date.format
taskg config set date.format rfc3339
taskg config list
taskg config unset date.format
```

远程模式下，`config` 只访问服务端 workspace 业务配置，不读取调用者本机 TOML。

## rc 临时覆盖

`rc.*` 只影响本次命令：

```bash
taskg rc.date.format=epoch list
taskg rc.json:on list
taskg rc.context=none list
```

## Context

context 是命名过滤器，不是权限边界。

```bash
taskg context define agent 'project:ai-agent-platform status:pending'
taskg context use agent
taskg context show
taskg context list
taskg context delete agent
taskg context none
```

启用后，`list`、`next`、报表、helper 都会叠加 active context。

临时绕过：

```bash
taskg --no-context list
taskg rc.context=none list
```

## UDA

UDA 是用户自定义属性。支持：

- `string`
- `numeric`
- `date`
- `duration`

示例：

```bash
taskg config set uda.estimate.type numeric
taskg config set uda.estimate.label Estimate
taskg config set uda.estimate.values 1,2,3,5,8

taskg add "Implement API" project:ai-agent-platform estimate:3
taskg estimate:3 list
taskg _get 1.estimate
taskg _udas
taskg _unique estimate
```

UDA 可以参与 urgency：

```bash
taskg config set urgency.uda.estimate.coefficient 1.5
```

未定义 schema 的 JSON top-level 字段会作为 orphan UDA 保留并导出，但普通 `modify` 不能修改 orphan UDA。

## .taskrc 只读导入

```bash
taskg config import-taskrc ~/.taskrc --dry-run --json
taskg config import-taskrc ~/.taskrc
```

taskg 不会修改原 `.taskrc`。当前支持导入：

- `color`
- `dateformat`
- `context.<name>`
- `uda.<name>.type/label/values/default`
- `urgency.uda.*`

以下 key 会识别但跳过：

- `data.location`
- `report.*`
- `calendar.*`
- `burndown.*`
- `news.*`
- `sync.*`
- `hooks.*`

其它 key 会进入 unknown 报告，不会让导入失败。

