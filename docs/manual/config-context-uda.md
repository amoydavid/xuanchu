---
title: "配置、Context 与 UDA"
weight: 50
---

# 配置、Context 与 UDA

## 配置来源

Xuanchu 的配置可以来自：

- CLI flag / `rc.*`
- 环境变量
- SQLite meta/config
- `~/.config/xuanchu/xuanchu.toml`
- 默认值

本机配置和业务配置要分开理解：

- `xuanchu.toml` 是本机启动和显示配置。
- workspace/project 业务配置存数据库，并受权限和 audit 管理。

## xuanchu.toml

最小示例：

```toml
[database]
path = "/Users/me/.local/share/xuanchu/xuanchu.db"

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
xuanchu show
xuanchu config get date.format
xuanchu config set date.format rfc3339
xuanchu config list
xuanchu config unset date.format
```

远程模式下，`config` 只访问服务端 workspace 业务配置，不读取调用者本机 TOML。

## rc 临时覆盖

`rc.*` 只影响本次命令：

```bash
xuanchu rc.date.format=epoch list
xuanchu rc.json:on list
xuanchu rc.context=none list
```

## Context

context 是命名过滤器，不是权限边界。

```bash
xuanchu context define agent 'project:agentapi status:pending'
xuanchu context use agent
xuanchu context show
xuanchu context list
xuanchu context delete agent
xuanchu context none
```

启用后，`list`、`next`、报表、helper 都会叠加 active context。

临时绕过：

```bash
xuanchu --no-context list
xuanchu rc.context=none list
```

## UDA

UDA 是用户自定义属性，采用两层模型：

- **Workspace `UDADefinition`** 保存 `name / type / label / values / default`，是 workspace 级定义，同一 workspace 内所有 project 共用。
- **Task / TaskSeries 只保存具体 value**。Project 只负责任务归属，不维护字段 allowlist、override 或可用范围。

支持的类型：`string`、`numeric`、`date`、`duration`。

定义管理走 workspace 级 API（Web Console 在 `/workspaces/{slug}/settings/custom-fields` 提供管理页）：

```text
GET    /api/v1/udas          列出当前 workspace 的字段定义
PUT    /api/v1/udas/{name}   创建或替换定义（body: type/label/values/default）
DELETE /api/v1/udas/{name}   删除定义
```

`default` 只作为输入提示，不自动写入 Task / TaskSeries。删除仍被活跃循环系列使用的定义会返回 `uda_active_series_in_use`；把定义改成与现存值不兼容的类型会返回 `uda_active_series_incompatible`。

任务上使用：

```bash
xuanchu add "Implement API" project:agentapi estimate:3
xuanchu estimate:3 list
xuanchu _get 1.estimate
xuanchu _udas
xuanchu _unique estimate
```

filter 表达式里任何非内置字段名都按 UDA 处理：`estimate:3` 相等、`estimate.notnull` 非空、`estimate:` 判空（UDA 没有 `.isnull` 后缀）。值类型自动推断：能解析为 RFC3339 按日期比较，都能 ParseFloat 按数字比较，否则按字符串子串匹配。

UDA 可以参与 urgency：

```bash
xuanchu config set urgency.uda.estimate.coefficient 1.5
```

未定义 schema 的 JSON top-level 字段会作为 orphan UDA 保留并导出，但普通 `modify` 不能修改 orphan UDA（`uda_orphan_readonly`）。

旧的 `config set uda.<name>.type/label/values/default` 写法仍可用于本地 CLI 兼容路径，但服务端 workspace 的权威定义以 `UDADefinition` 为准。

## .taskrc 只读导入

```bash
xuanchu config import-taskrc ~/.taskrc --dry-run --json
xuanchu config import-taskrc ~/.taskrc
```

Xuanchu 不会修改原 `.taskrc`。当前支持导入：

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

