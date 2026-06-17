# Web Console 项目-任务浏览体验重构

**日期：** 2026-06-17
**状态：** 草案
**版本：** v0.4.3（承接 v0.4.2 project-readonly view）
**对应 ROADMAP：** v0.4.0 Web Admin Console 后续增强

## 1. 背景

当前 Web Console 的项目/任务展示存在两套并存的实现，且没有串起来：

1. **平铺资源表格**（`/tasks`、`/projects`，由 `ResourcePage` + `features/workspace/resources/resource-config.tsx` 实现）：纯只读列表，列很少，**行不能点进详情**，没有项目→任务钻取。
2. **只读项目视图**（`features/workspace/project-readonly/`，走 deep-link `/workspaces/$ws/projects/$p[/tasks/$ref]`）：功能较全，含项目摘要、项目下任务表格、任务详情页、项目 timeline。但**没挂进侧边栏**，平时只能靠 URL 访问。

结果：用户在侧边栏看到的 `/projects`、`/tasks` 是简陋表格，而真正好用的项目→任务→详情视图藏在 deep-link 里。任务详情页也较单薄——注解只显示纯文本（缺 entry 时间），links / entry / modified / recur / UDAs 都没展示，注解也没有"查看更多"。

本次目标：**把 project-readonly 视图扶正为主体验，补全任务详情，加上 shadcn 过滤工具栏，并打通从侧边栏到项目→任务→详情的完整浏览路径。**

## 2. 核心决策

| 维度 | 决策 |
|---|---|
| 两套视图取舍 | 扶正 project-readonly，退役平铺 projects/tasks 表格 |
| 导航组织 | 项目优先；移除 `/tasks` 侧边栏入口，任务藏在项目里 |
| 注解加载 | 详情页懒加载：默认显示最近 3 条，"查看更多"分页加载剩余 |
| 过滤 | 工具栏内联下拉 + 活跃 chips，过滤条件同步到 URL |
| 详情字段范围 | 常用字段 + UDAs 动态展示 |
| 实现路径 | 前端 + 后端同步改（路径 B） |
| HTTP 接口前缀 | 所有新增/变更接口统一在 `/api/v1/` 下 |

### 2.1 为什么前端 + 后端同步改

纯前端方案（路径 A）能复用现有 `GET /tasks?project=` 和内嵌 annotations 快速达成，但有三个硬缺口：

- **注解无法真分页**：annotations 全量嵌在 `GET /tasks/{ref}` 返回体里，无独立列表端点、无 limit/offset。注解多的任务请求体臃肿。
- **tasks filter 无 restful 参数**：后端只有 taskwarrior 风格 `query=` DSL，没有 `?status=`、`?priority=` 等可读参数。前端若直接拼 DSL，URL 不可读、控件逻辑泄漏到前端。
- **项目列表无聚合统计**：`GET /projects` 不返回 task_count / 进度，列表页要么 N+1 请求要么不显示。

路径 B 把这三处后端能力补齐，前端才能做出干净的体验。改动都复用 `internal/app`，不复制业务逻辑。

### 2.2 为什么移除 `/tasks` 侧边栏入口

任务的实际归属是项目。保留全局 `/tasks` 表会与"项目→任务"的钻取主路径形成两套入口，造成认知负担。跨项目任务检索是低频需求，未来如有需要可作项目列表页的次级功能，不在本次范围。

## 3. 前端设计

### 3.1 导航与路由

**侧边栏（`web/src/components/AppShell.tsx`）**：

- 移除 Tasks 导航项。
- Projects 作为主入口保留，指向项目列表页。
- Overview 保留。

**路由（`web/src/routes/router.tsx`）**：

- `/projects` → 项目列表页（新实现，替换原 ResourcePage 的 projects 配置）。
- `/workspaces/$ws/projects/$p` → 项目详情页（复用现有 `ProjectReadonlyPage`，补全任务表格交互）。
- `/workspaces/$ws/projects/$p/tasks/$ref` → 任务详情页（复用现有 `ProjectTaskDetailPage`，按 3.3 增强）。
- 平铺的 `/tasks` 路由移除（或重定向到 `/projects`，避免外部书签 404）。

### 3.2 项目列表页（`/projects`）

**布局：项目表格**（已确认）。列：

| 列 | 内容 | 数据来源 |
|---|---|---|
| 项目名 | name / slug | `GET /projects` |
| 状态 | active / pending / archived badge | `GET /projects` |
| 进度 | 进度条 + 百分比（completed / total） | 新增聚合字段 |
| 负责人 | UserInfo（主负责人） | `GET /projects` |
| 任务数 | total / pending 计数 | 新增聚合字段 |

行点击 → 项目详情页。支持按列排序。

### 3.3 任务详情页

**布局：主体 + 右侧属性栏**（已确认）。

**顶部区（全宽）**：
- 面包屑：`项目 / 任务`，可点回项目页。
- 任务标题：description。
- 徽章：status、priority、task_slug。

**左主区域（纵向叙事）**：
- **描述**：description 全文。
- **注解**：首屏注解来自 `GET /api/v1/tasks/{ref}` 内嵌的 annotations（按 entry 倒序），前端默认只渲染最近 3 条。每条显示 description + entry 时间。若内嵌注解总数 > 3，下方显示"查看更多 N 条 ↓"按钮；点击调用 `GET /api/v1/tasks/{ref}/annotations?offset=<已显示数>&limit=10` 分页加载，追加显示，直到返回数 < limit 或累计达 total。
- **关联任务（links）**：列出 links 数组，每条显示 title / type / url（可点）/ created_by（UserInfo）。links 是富信息，放主区域而非窄属性栏。

**右侧属性栏（窄栏）**：
- **常用字段**：status、priority、assignees（多指派人拼合）、due、tags、depends、entry（创建时间）、modified（最后修改）、recur（循环，仅当存在时显示）。
- **自定义字段（UDAs）**：分隔线下方动态遍历。后端 UDAs 平铺为 JSONTask 顶层字段（非嵌套 `udas` 对象），前端需识别已知保留字段（见 4.3 列表）外的顶层字段，归类为 UDA 逐行展示 `字段名 / 值`。

**数据来源**：
- 主体描述、属性、UDAs、links、初始注解：`GET /api/v1/tasks/{ref}`（现有，已含全部字段）。
- 注解懒加载：新增 `GET /api/v1/tasks/{ref}/annotations?offset=&limit=`。

### 3.4 任务过滤工具栏（项目详情页内，任务表格上方）

**布局：内联下拉 + 活跃 chips**（已确认）。

工具栏一行排开：状态下拉、优先级下拉、负责人下拉、截止日期范围选择、标签多选、搜索框。已选条件在下方显示为可移除 chip（点 ✕ 清除单个），附"清除全部"。

**控件 → 后端映射**：前端把 UI 选择翻译成 restful 风格 query 参数发请求（见 4.2 新增参数），由后端内部转成 query DSL。这样 URL 可读、前端不直接写 DSL。

**URL 同步**：过滤条件同步到浏览器 URL，如 `?status=pending&priority=H&assignee=xxx&q=关键字&due_after=2026-06-01&due_before=2026-06-30&tags=bug,refactor`。进入页面时从 URL 重建控件状态并发起请求。刷新 / 分享链接均保留过滤。

## 4. 后端设计

所有改动统一在 `/api/v1/` 前缀下，复用 `internal/app`，不引入 CGO，用户身份字段用 `task.UserInfo`。

### 4.1 新增 annotations 列表端点

```
GET /api/v1/tasks/{taskRef}/annotations?offset=<int>&limit=<int>
```

- **路由**：`internal/httpapi/router.go` 注册。
- **handler**：`internal/httpapi/tasks.go` 新增 `handleTaskAnnotationList`。复用现有 task ref 解析逻辑，调 app 层新方法。
- **app 层**：`internal/app/service.go` 新增 `ListAnnotations(taskUUID string, offset, limit int) ([]task.JSONAnnotation, int, error)`，返回注解列表和总数。
- **storage 层**：`internal/storage/task_repo.go` 新增按 task uuid 分页查询注解的方法，复用 `TaskAnnotation` model。按 `entry` 倒序。
- **响应**：`{ annotations: [...], total: <int>, offset: <int>, limit: <int> }`。注解结构同现有 `JSONAnnotation`（id、entry、description）。
- **默认值**：offset=0，limit=20，上限 100。

### 4.2 tasks filter restful 参数增强

`internal/httpapi/tasks.go` 的 `handleTaskList` 新增识别以下 query 参数，**内部翻译成现有 query DSL 表达式**（复用 `internal/query` 包的 `ParseFilterExpr`），与原有 `query=` / `filter=` 用 `query.And` 合并：

| restful 参数 | 翻译为 DSL |
|---|---|
| `status=pending` | `status:pending` |
| `priority=H` | `priority:H` |
| `assignee=<user_id>` | `assignee:<user_id>` |
| `due_after=2026-06-01` | `due.after:2026-06-01` |
| `due_before=2026-06-30` | `due.before:2026-06-30` |
| `tags=bug,refactor` | `+bug +refactor` |
| `q=关键字` | 裸词（description LIKE） |

- 翻译逻辑集中在 handler 内一个辅助函数，参数校验失败返回 400。
- 原有 `query=` / `filter=` 参数保留不变，向后兼容。
- 不改变 app 层 `ListInput` 签名——handler 把翻译好的 `query.Expr` 通过现有 `ListInput.Query` 传入即可。

### 4.3 projects 列表聚合统计

`GET /api/v1/projects` 响应中每个项目对象补上聚合字段：

| 字段 | 说明 |
|---|---|
| `task_count` | 项目下任务总数 |
| `pending_count` | pending 状态数 |
| `active_count` | active（started）状态数 |
| `completed_count` | completed 状态数 |

- **handler**：`internal/httpapi/projects.go` `handleProjectList`。
- **app 层**：扩展 project view struct，新增聚合方法（一次查询按 project 分组聚合，避免 N+1）。
- **storage 层**：新增按 project uuid 批量聚合任务状态计数的查询。
- 进度百分比由前端算（`completed_count / task_count`），后端只返原始计数。

### 4.4 JSONTask 字段说明（前端实现参考）

后端 `JSONTask`（`internal/task/json.go`）输出字段，前端按此对接：

- **存在**：uuid、description、status、entry、modified、end、due、project、task_slug、priority、tags、start、wait、scheduled、until、annotations、depends、recur、parent、mask、imask、assignees、links。
- **UDAs 平铺**：每个 UDA 作为 JSONTask 顶层字段输出（非嵌套在 `udas` key 下）。reserved 字段 `project_id`、`project_seq` 跳过。
- **无 `owner`**：用 `assignees`（多指派人）。
- **无 `recurrence`**：字段名是 `recur`。
- **无独立 `udas` key**：前端识别顶层未知字段作为 UDA。

UDAs 前端归类时需排除的已知保留字段集合：上述所有标准字段名 + `project_id` + `project_seq`。

## 5. 不在本次范围

- 任务编辑/状态变更（本次只读浏览）。
- 全局跨项目任务检索（已移除 `/tasks` 入口，未来如需再做）。
- 拖拽看板 / 任务批量操作。
- 飞书 SSO / 授权中心（属 v0.4.2 预留边界）。
- MCP tool 暴露（annotations 列表等后端能力未来可挂 MCP，命名遵循下划线规范如 `task_list_annotations`，但本次不实现）。

## 6. 验收标准

### 前端

- 侧边栏无 Tasks 项，Projects 为项目主入口。
- `/projects` 展示项目表格，含状态/进度/负责人/任务数，行可点进项目详情。
- 项目详情页任务表格上方有过滤工具栏，支持 status/priority/assignee/due 范围/tags/搜索，过滤条件同步 URL，刷新保留。
- 任务详情页展示：描述、注解（最近 3 条 + 查看更多懒加载）、links、右侧常用字段 + UDAs。
- 注解每条含 entry 时间。
- 旧的 deep-link `/workspaces/.../projects/...` 行为不变。

### 后端

- `GET /api/v1/tasks/{ref}/annotations?offset=&limit=` 返回分页注解 + total。
- `GET /api/v1/tasks?status=&priority=&assignee=&due_after=&due_before=&tags=&q=` 正确过滤，与原 `query=` 共存。
- `GET /api/v1/projects` 每项目含 task_count / pending_count / active_count / completed_count。
- 所有新接口在 `/api/v1/` 下。
- 用户身份字段用 `task.UserInfo`。

### 工程

- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 全绿。
- 不引入 CGO 依赖。
- web 工程单测（vitest）通过。
