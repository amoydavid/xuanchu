# Scope 清单与通配符

用 `scope_list` 查看所有可用 scope（无需鉴权）。Token 创建/修改时用 `scope` 数组指定权限。

## 完整 scope 清单

| Scope | 含义 |
|---|---|
| `task:read` / `task:write` | 任务读写 |
| `project:read` / `project:write` | 项目读写 |
| `context:read` / `context:write` | context 读写 |
| `workspace:read` / `workspace:write` | workspace 读写 |
| `config:read` / `config:write` | 配置读写 |
| `hook:read` / `hook:write` | Hook 读写 |
| `notification:read` / `notification:write` | notification sink 与 delivery 读写 |
| `reminder:read` / `reminder:write` | reminder rule 读写 |
| `token:read` / `token:write` | Token 读写 |
| `audit:read` | 审计日志 |
| `impersonate` | 委托操作 |

## scope_list — 列出所有可用 scope（只读）

无需鉴权。

```json
// 输入
{}

// 返回
{
  "data": {
    "scopes": [
      "task:read", "task:write",
      "project:read", "project:write",
      "context:read", "context:write",
      "workspace:read", "workspace:write",
      "config:read", "config:write",
      "hook:read", "hook:write",
      "notification:read", "notification:write",
      "reminder:read", "reminder:write",
      "token:read", "token:write",
      "audit:read",
      "impersonate"
    ]
  }
}
```

## 通配符扩展

| 写法 | 展开为 |
|---|---|
| `*` | 所有 scope，包括 `impersonate` |
| `task:*` | `task:read` + `task:write` |
| `*:read` | 所有 `:read` scope |

## 选用建议

- 通用 HTTP MCP / workspace Agent token 默认用 `["*"]`，让 Agent 覆盖用户、成员、项目、任务、配置、通知、token 等完整工具集。
- `*` 是 token capability 上限，**实际权限仍会被 workspace/project allowlist 和绑定用户的 membership role 收窄**。
- 只有专用自动化 token 才按场景改成最小 scope。
