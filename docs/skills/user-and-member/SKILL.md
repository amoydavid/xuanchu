# 用户与成员管理

通过 taskg MCP 管理用户账户、外部 ID 绑定和 workspace 成员角色。

## 重要原则

- 成员操作必须指定 `workspace`，不要依赖隐式状态
- 用户操作在全局范围执行，但创建用户后会自动生成 personal workspace

## 用户管理

### user_list — 列出所有用户

只读。

```json
// 输入
{}

// 返回
{
  "data": {
    "users": [
      {"id": "user-uuid-xxx", "name": "local", "email": null},
      {"id": "user-uuid-yyy", "name": "alice", "email": "alice@example.com"}
    ],
    "count": 2
  },
  "rendered": "2 user(s)"
}
```

### user_get — 查看单个用户

只读。支持按 `name`、`email` 或 `UUID` 查找。

```json
// 输入
{"user": "alice"}

// 返回
{
  "data": {
    "user": {
      "id": "user-uuid-yyy",
      "name": "alice",
      "email": "alice@example.com",
      "external_ids": [
        {"provider": "feishu", "external_id": "ou_12345"}
      ]
    }
  },
  "rendered": "user alice"
}
```

### user_add — 创建用户

用户名支持中文等非 ASCII 字符——系统会自动生成 workspace slug。

```json
// 输入
{"name": "alice"}

// 输入：带邮箱 + 中文名
{"name": "张三", "email": "zhangsan@example.com"}

// 返回
{
  "data": {"user": {"id": "user-uuid-new", "name": "alice"}},
  "rendered": "user alice"
}
```

### user_use — 切换用户

**注意：** 此操作仅影响 stdio MCP 的隐式状态。Agent 应优先通过参数传 workspace/user，而非依赖此操作。

```json
{"user": "alice"}
```

## 外部 ID 绑定

admin/owner 可为他人绑定，普通用户只能绑定自己。

### user_bind — 绑定外部 ID

```json
{
  "user": "alice",
  "provider": "feishu",
  "external_id": "ou_36093ec6279eebf7f9ae75a30dca12fc"
}

// 返回
{
  "data": {"provider": "feishu", "external_id": "ou_36093ec6279eebf7f9ae75a30dca12fc"},
  "rendered": "Bound feishu:ou_36093ec6279eebf7f9ae75a30dca12fc to alice"
}
```

### user_list_external_ids — 列出外部 ID

只读。

```json
{"user": "alice"}

// 返回
{
  "data": {
    "external_ids": [{"provider": "feishu", "external_id": "ou_xxx"}],
    "count": 1
  }
}
```

### user_unbind — 解绑

```json
{
  "user": "alice",
  "provider": "feishu",
  "external_id": "ou_36093ec6279eebf7f9ae75a30dca12fc"
}
```

## 成员管理

角色层级：`viewer` < `member` < `admin` < `owner`。

### member_list — 列出成员

只读。

```json
// 输入
{"workspace": "dajee"}

// 返回
{
  "data": {
    "members": [
      {"user_id": "user-uuid-xxx", "name": "alice", "role": "owner"},
      {"user_id": "user-uuid-yyy", "name": "bob", "role": "member"}
    ],
    "count": 2
  }
}
```

### member_add — 添加成员

`role` 可选，默认 `member`。

```json
{"workspace": "dajee", "user": "bob", "role": "member"}
```

### member_role — 修改成员角色

```json
{"workspace": "dajee", "user": "bob", "role": "admin"}
```

## 典型 Agent 工作流

**场景：用户说"把张三加入 dajee workspace"**

```json
// Step 1: 创建用户（如不存在）
user_add({"name": "张三"})

// Step 2: 绑定飞书 ID
user_bind({"user": "张三", "provider": "feishu", "external_id": "ou_xxx"})

// Step 3: 加入 workspace
member_add({"workspace": "dajee", "user": "张三", "role": "member"})
```

**场景：查看 dajee 团队成员**

```json
// Step 1: 列出成员
member_list({"workspace": "dajee"})

// Step 2: 查看某个成员详情
user_get({"user": "bob"})
```
