# Xuanchu Project 生命周期状态机设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-06-15
**状态：** 草案
**背景需求：** 电商众筹业务中每个产品对应一个 project，需要表达「预立项 / 立项在跑 / 结束归档 / 取消」四种生命周期阶段，当前系统仅支持 active/archived 两态，无法覆盖。

## 1. 背景与现状（基于代码）

当前 Project 只有两种状态：

- `storage/project_repo.go:14-15`：`ProjectStatusActive = "active"` / `ProjectStatusArchived = "archived"`
- `app/project.go:236`：`AddProject` 默认创建为 `active`
- 归档判断逻辑散落在 7+ 处，统一模式为 `project.Status == archived || project.ArchivedAt != nil`：`project.go:149,177,429,490`、`project_config.go:187`、`project_query.go:47`、`service.go:1957`
- `project_repo.go:71`：`List` 默认只返回 `status=active`

业务上「预立项 vs 立项在跑」「取消」无法表达。

## 2. 目标

1. Project 支持 `planning / active / archived / cancelled` 四态
2. 提供显式状态转移操作，**任意状态可转到任意状态**，系统不限制方向（含 `archived → active` 重新激活）
3. 转移后的写权限由目标状态固定约束（与转移方向无关）：`planning` / `active` 可写，`archived` / `cancelled` 禁写
4. 每次转移自动追加一条**项目变更注解**，进入项目 timeline 形成完整变更历史
5. 新建 project 默认 `planning`
6. 状态变更触发 webhook 与审计

## 3. 非目标

- 不引入 project 级 RBAC 覆盖 workspace role
- 不为历史 active 数据做迁移（status 是 string，新值即用，旧 active 仍是 active）
- 不改变 slug 校验规则（`project.go:347-362` 仍要求 3-10 小写字母数字）
- 不实现项目模板/克隆
- 不涉及产品级业务属性（定价/财务等）的数据建模，那属于 scoped config 的使用范畴，应在使用说明中提及，不进入本规格

## 4. 状态设计

四态：`planning`（预立项）/ `active`（立项在跑）/ `archived`（结束归档）/ `cancelled`（取消）。

### 4.1 状态转移：任意方向，不限制

任意状态可转移到任意其它状态，系统不做白名单限制（含 `archived → active` 重新激活、`archived → cancelled`、`cancelled → planning` 等）。转移方向由用户/管理员自行判断，系统只负责：

1. 校验目标状态是四个合法值之一
2. 记录变更（审计 + hook + 项目变更注解）
3. 约束转移后的写操作权限

理由：状态机白名单会过度限制真实运营（归档后重新激活、误取消后恢复）。把「能否转」交给人的判断，把「转之后能做什么」交给系统约束，更贴合实际。

### 4.2 各状态写权限（固定，与如何到达该状态无关）

| 状态 | add task | annotate（手动） | config | modify | transition |
|---|---|---|---|---|---|
| planning | ✅ | ✅ | ✅ | ✅ | ✅ |
| active | ✅ | ✅ | ✅ | ✅ | ✅ |
| archived | ❌ | ❌ | ❌ | ❌ | ✅ |
| cancelled | ❌ | ❌ | ❌ | ❌ | ✅ |

`planning` 视同 `active` 可写。判断「禁写」统一收敛为 `isProjectClosed(project)` = status ∈ {`archived`, `cancelled`}。

`transition` 本身在任何状态都允许——它是状态管理操作，不是业务写操作。

### 4.3 转移记入项目变更注解

每次 `transition` 在更新 status 后，自动追加一条 `ProjectAnnotation`，content 记录状态变更（如「状态变更：planning → active」）。该注解由系统写入，**绕过目标状态的写权限限制**（因为 transition 在任何状态都允许），进入项目 timeline，与手动 annotate 的注解一并按时间序展示，形成完整的项目变更历史。

## 5. 数据模型

**无 schema 变更、无迁移。** `Project.Status` 已是 string，新增状态值即用。

- `storage/project_repo.go` 新增常量：`ProjectStatusPlanning = "planning"`、`ProjectStatusCancelled = "cancelled"`
- `ArchivedAt` 字段语义不变：仅 `archived` 设置；`cancelled` 不设 `ArchivedAt`，靠 status 判别
- `List` 过滤语义：「进行中」= `planning ∪ active`（当前仅 active，需调整）

## 6. App 层

- 新增 `TransitionProject(ref, toStatus string) (ProjectView, error)`：校验 `toStatus` 是合法四态之一、更新 status、自动追加一条项目变更注解（4.3）、触发审计与 hook。**不校验转移方向**（任意方向允许）
- 抽取 `isProjectClosed(project) bool`，替换现有 7 处 `status == archived || ArchivedAt != nil` 散落判断
- `AddProject` 默认 status 改为 `planning`（破坏性变更，符合业务诉求）
- `recurringArchivedProjectWarning`（`service.go:1949`）扩展：cancelled project 上的 recurring 同样告警

## 7. CLI / HTTP / MCP

**CLI：** `xuanchu project transition <ref> <planning|active|archived|cancelled>`
- `project list` 增加 `--status <open|planning|active|archived|cancelled|all>`（`open` = planning+active）

**HTTP：**
- `POST /api/v1/projects/{projectRef}/transition` body `{"status":"..."}`
- `GET /api/v1/projects?status=open`（默认 open）

**MCP：** `project_transition` tool（命名遵循 AGENTS.md §MCP 规范，下划线分隔）

## 8. Hook 与审计

- 新增事件类型 `project.transitioned`，payload 含 `from_status` / `to_status`
- 审计动作 `project.transition`
- 保留现有 `project.archived` 事件（转移至 archived 时同时触发，向后兼容）

## 9. 错误码

- `project_invalid_status`：未知 status 值
- 保留 `project_archived` 向后兼容；对 cancelled 状态的写操作复用同类「已关闭」错误，错误码沿用 `project_archived` 以减少对现有错误处理链路的冲击（payload 区分具体 status）

## 10. 验收标准

- 四态可创建；任意状态间可自由转移（含 `archived → active`）
- planning / active 可写；archived / cancelled 禁写
- 每次 transition 自动写入一条项目变更注解，可在 timeline 查询到
- 新建 project 默认 planning
- `project list --status open` 返回 planning+active
- `project.transitioned` webhook 触发；审计记录 from/to
- 现有归档相关测试不回归
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
- SQLite 与 PostgreSQL 行为一致
