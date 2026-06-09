# Workspace 管理

通过 Xuanchu MCP 管理工作区（workspace）——企业/租户级隔离边界。

## 基本概念

- workspace 是最高级隔离单元，包含项目、成员、配置
- 每个用户有一个 personal workspace（创建用户时自动生成）
- workspace 支持可见性：`private`（默认）、`team`、`public`
- **所有 MCP 调用都应通过 `workspace` 参数显式指定目标 workspace**，不要依赖隐式状态

## Workspace 操作

### workspace_list — 列出 workspace

只读。

```json
// 输入
{}

// 输入：包含已归档
{"include_archived": true}

// 返回
{
  "data": {
    "workspaces": [
      {"id": "ws-uuid-xxx", "slug": "local", "name": "local", "visibility": "private", "role": "owner", "active": true},
      {"id": "ws-uuid-yyy", "slug": "engineering", "name": "工程团队", "visibility": "team", "role": "admin"}
    ],
    "count": 2
  },
  "rendered": "2 workspace(s)"
}
```

### workspace_get_current — 读取当前 workspace

只读。可通过 `workspace` 参数显式指定。

```json
// 输入
{}

// 输入：显式指定
{"workspace": "engineering"}
```

### workspace_info — 查看 workspace 详情

只读。`workspace` 必填。

```json
// 输入
{"workspace": "engineering"}

// 返回
{
  "data": {
    "workspace": {
      "id": "ws-uuid-xxx",
      "slug": "engineering",
      "name": "工程团队",
      "description": "公司核心工程团队",
      "visibility": "team",
      "created_by": {"id": "...", "name": "alice"},
      "role": "admin"
    }
  },
  "rendered": "workspace engineering"
}
```

### workspace_add — 创建 workspace

`slug` 必填，仅允许 `^[a-z0-9][a-z0-9_-]*$`。

注意：这是 workspace slug 规则。project slug 更严格，只允许 3-10 位小写字母或数字，必须以字母开头，不能包含 `-` 或 `_`。

```json
// 输入
{"slug": "engineering", "name": "工程团队", "visibility": "team"}

// 返回
{
  "data": {"workspace": {"id": "ws-uuid-new", "slug": "engineering", "name": "工程团队", "visibility": "team"}},
  "rendered": "workspace engineering created"
}
```

### workspace_modify — 修改 workspace

```json
{"workspace": "engineering", "name": "工程部", "description": "合并后的工程部门"}
```

### workspace_use — 切换 active workspace

**注意：** 此操作仅影响 stdio MCP 的隐式状态。HTTP MCP 不受影响。Agent 应优先通过参数显式传 `workspace`，而非依赖此操作。

```json
{"workspace": "engineering"}
```

### workspace_archive — 归档 workspace

```json
{"workspace": "old-team"}
```

## 典型 Agent 工作流

**场景：用户说"帮我创建一个团队 workspace 并把 alice 加进去"**

```json
// Step 1: 创建 workspace
workspace_add({"slug": "engineering", "name": "工程团队", "visibility": "team"})

// Step 2: 确保 alice 用户存在（如需要）
user_add({"name": "alice"})

// Step 3: 把 alice 加入 workspace
member_add({"workspace": "engineering", "user": "alice", "role": "member"})

// Step 4: 在 workspace 下创建项目
project_add({"workspace": "engineering", "slug": "backend", "name": "后端服务"})
```
