# Config 工具

管理 workspace / project 级配置和过滤上下文。配置分三个作用域。

## 作用域

| scope | 说明 | 可写 |
|---|---|---|
| `workspace` | workspace 级（默认），支持业务键 `urgency.*` / `date.*`，**不支持 `agent.*`** | 是 |
| `project` | project 级，支持 `agent.*` 指令 | 是 |
| `local` | 仅 stdio，本地配置 | 否（只读） |

## config_get — 读取配置（只读）

不传 `scope` 时默认读 workspace 级。

```json
// 输入：workspace 级别
{"workspace": "dajee", "key": "urgency.priority.coeff"}

// 输入：project 级别
{"workspace": "dajee", "scope": "project", "project": "apiplat", "key": "agent.background"}

// 输入：local 级别（仅 stdio）
{"scope": "local", "key": "remote.server"}

// 返回
{
  "data": {"key": "agent.background", "value": "你是一个 API 开发助手", "scope": "project"},
  "rendered": "agent.background=你是一个 API 开发助手"
}
```

## config_set — 写入配置

`scope` 必填，取值通常是 `workspace` 或 `project`。workspace 级别只支持业务配置键（如 `urgency.*` / `date.*`），不支持 `agent.*`。`local` scope 不可写。

```json
// workspace 级别
{
  "workspace": "dajee",
  "scope": "workspace",
  "key": "urgency.priority.coeff",
  "value": "6.0"
}

// project 级别
{
  "workspace": "dajee",
  "scope": "project",
  "project": "apiplat",
  "key": "agent.handoff",
  "value": "交接说明：所有 API 变更需要经过 review"
}
```

## config_list — 列出配置（只读）

```json
{"workspace": "dajee"}
```

## config_unset — 删除配置

仅删除 workspace 级配置。删除 project 级配置请用 `project_config_unset`。

```json
{"workspace": "dajee", "key": "urgency.priority.coeff"}
```

## 项目配置快捷方式

项目配置也可通过专门 tool 操作（效果等同 `config_get/set` + `scope="project"`）：

```json
// 设置
project_config_set({"workspace": "dajee", "project": "apiplat", "key": "agent.background", "value": "..."})

// 列出
project_config_list({"workspace": "dajee", "project": "apiplat"})

// 删除
project_config_unset({"workspace": "dajee", "project": "apiplat", "key": "agent.handoff"})
```

## Context（过滤上下文）

Context 是预定义的查询过滤器，供 CLI 交互使用。CIO 可以**读取** context 了解用户偏好过滤，但**不应修改** active context。

### context_get — 查看上下文（只读）

```json
{"workspace": "dajee"}

// 查看指定 context
{"workspace": "dajee", "name": "sprint"}
```

### context_list — 列出所有上下文（只读）

```json
{"workspace": "dajee"}
```

### context_delete — 删除上下文

仅在用户明确要求删除 context 时使用。

```json
{"workspace": "dajee", "name": "release"}
```

### context_set / context_none — 修改 active context

> 只影响 stdio MCP 的隐式状态。CIO 默认不要使用；只有用户明确要求"设置当前 context"或"清除当前 context"时才调用。

```json
context_set({"workspace": "dajee", "name": "sprint"})
context_none({"workspace": "dajee"})
```
