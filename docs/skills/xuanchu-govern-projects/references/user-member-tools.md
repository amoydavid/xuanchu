# 用户与成员工具

用户操作在全局范围执行，但创建用户后会自动生成 personal workspace。成员操作必须指定 `workspace`。

## 用户管理

### user_list — 列出所有用户（只读）

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

### user_get — 查看单个用户（只读）

支持按 `name`、`email` 或 UUID 查找。

```json
// 输入：按用户名
{"user": "alice"}

// 输入：按邮箱
{"user": "alice@example.com"}

// 返回
{
  "data": {
    "user": {
      "id": "user-uuid-yyy",
      "name": "alice",
      "email": "alice@example.com",
      "external_ids": [
        {"provider": "feishu_user_id", "external_id": "d8c6g9xx"}
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

> 仅影响 stdio MCP 的隐式状态。默认优先通过参数传 workspace/user，而非依赖此操作。

```json
{"user": "alice"}
```

## 外部 ID 绑定

admin/owner 可为他人绑定，普通用户只能绑定自己。

绑定飞书用户时，推荐使用 `feishu_user_id` 作为 `provider`，并将飞书 `user_id` 作为 `external_id`，便于跨应用统一身份。

### user_bind — 绑定外部 ID

```json
// 输入
{
  "user": "alice",
  "provider": "feishu_user_id",
  "external_id": "d8c6g9xx"
}

// 返回
{
  "data": {"provider": "feishu_user_id", "external_id": "d8c6g9xx"},
  "rendered": "Bound feishu_user_id:d8c6g9xx to alice"
}
```

### user_list_external_ids — 列出外部 ID（只读）

```json
// 输入
{"user": "alice"}

// 返回
{
  "data": {
    "external_ids": [{"provider": "feishu_user_id", "external_id": "d8c6g9xx"}],
    "count": 1
  }
}
```

### user_unbind — 解绑

```json
{
  "user": "alice",
  "provider": "feishu_user_id",
  "external_id": "d8c6g9xx"
}
```

## 成员管理

角色层级：`viewer` < `member` < `admin` < `owner`。

### member_list — 列出成员（只读）

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

## 典型工作流

### 把张三加入 dajee workspace

```json
// Step 1: 创建用户（如不存在）
user_add({"name": "张三"})

// Step 2: 绑定飞书 ID
user_bind({"user": "张三", "provider": "feishu_user_id", "external_id": "d8c6g9xx"})

// Step 3: 加入 workspace
member_add({"workspace": "dajee", "user": "张三", "role": "member"})
```

### 查看 dajee 团队成员

```json
// Step 1: 列出成员
member_list({"workspace": "dajee"})

// Step 2: 查看某个成员详情
user_get({"user": "bob"})
```
