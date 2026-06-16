# Slug 规则

Xuanchu 中有两类 slug，规则不同，CIO 高频踩坑。

## 对比表

| 类型 | 规则 | 合法示例 | 非法示例 |
|---|---|---|---|
| workspace slug | `^[a-z0-9][a-z0-9_-]*$`，允许 `-` `_`，无额外字符约束 | `engineering`、`api-platform`、`ai_agent` | `API`、`工程`、`-api` |
| project slug | 3-10 位小写字母或数字，**必须以字母开头**，不能含 `-` `_` 中文 | `api`、`apiplat`、`api9` | `api-platform`、`ai_agent`、`1api`、`p1` |

## 关键差异

- workspace slug 宽松：允许连字符和下划线。
- project slug 严格：只有字母数字，且必须字母开头，长度 3-10。
- 创建项目时 slug 会自动转小写，但仍需满足字符与开头约束。
- `project_id`（UUID）不受 slug 规则限制，随时可用。

## 用户名

`user_add` 的 `name` 支持**中文等非 ASCII 字符**——系统会自动生成 personal workspace slug。所以不要用 project slug 规则去约束用户名。

## 常见误用

| 错误写法 | 问题 | 正确做法 |
|---|---|---|
| `project_add({"slug":"api-platform"})` | project slug 含 `-` | 用 `apiplat` 或 `apiplatform` |
| `project_add({"slug":"AI"})` | 大写不合法（会转小写但若含其他约束仍可能失败） | 用合法的 `aix` 或 `aipro` |
| `project_add({"slug":"1api"})` | 数字开头不合法 | 改为字母开头，如 `api1` |
| `project_add({"slug":"p1"})` | 只有 2 位，短于 3-10 要求 | 至少 3 位，如 `px1` |
| `workspace_add({"slug":"工程"})` | workspace slug 不允许非 ASCII | 用拼音或英文，如 `engineering` |
