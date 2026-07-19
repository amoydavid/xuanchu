# 项目、任务与循环系列统一 URL 输出设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-18

**状态：** 已确认

**目标：** 让 HTTP、MCP、CLI、Remote client 等对外接口返回项目、任务或循环系列信息时，统一包含可直接打开 Web Console 的绝对 URL。

## 1. 结论

项目、任务和循环系列的对外 view 统一包含 `url` 字段。配置 `server.public_base_url` 后，URL 是包含 scheme、host、可选端口和 Console 部署前缀的绝对 HTTP(S) URL；未配置时返回空字符串，绝不退化为相对路径，也不让资源读取失败。

例如 `server.public_base_url = "https://xuanchu.example.com"` 且 `server.console.base_path = "/"` 时，项目 URL 为：

```text
https://xuanchu.example.com/workspaces/{workspaceSlug}/projects/{projectSlug}
```

三类 canonical URL 为：

```text
/workspaces/{workspaceSlug}/projects/{projectSlug}
/workspaces/{workspaceSlug}/projects/{projectSlug}/tasks/{taskRef}
/workspaces/{workspaceSlug}/projects/{projectSlug}/series/{seriesSlug}
```

其中项目内普通任务和已物化 occurrence 的 `{taskRef}` 使用 `task_slug`；尚未物化的 projected occurrence 没有 `task_slug`，使用稳定的 `occurrence_ref`。无项目任务使用现有的独立详情入口 `/tasks/{uuid}`。

循环系列继续使用当前已经存在的 `/series/{seriesSlug}` 路由，不新增 `/tasks/{taskSlug}/series/{seriesSlug}` 别名。系列是独立资源，不从属于某一个 occurrence。

## 2. 背景与问题

当前 Web Console 已有稳定的项目、项目任务、无项目任务和循环系列详情路由，但后端对外 view 没有统一暴露这些 URL：

- HTTP 的 project/task/series DTO 没有 `url`。
- MCP 的 project/task/series 结果没有 `url`。
- CLI JSON 与人类可读详情没有 `url`。
- Remote client DTO 不认识 `url`，因此远程 CLI 即使服务端返回也会丢失。
- Web Console 首页等复合响应复用 project/task DTO，调用方仍需自行拼路径。

如果各协议层各自拼接，会重复处理 workspace slug、project slug、task 引用、series slug 和 path segment 编码，也容易让 projected occurrence 在不同入口得到不同路径。因此 URL 必须由 App 层统一生成，再由输出层透传。

## 3. 目标

1. `app.ProjectView`、`app.TaskOccurrenceView`、`app.TaskSeriesView` 都包含 `URL string`；其值要么为空，要么是绝对 HTTP(S) URL。
2. HTTP、MCP、CLI JSON、CLI 人类可读输出和 Remote client 对同一资源返回相同 URL。
3. 项目列表/详情、任务列表/详情、series 列表/详情以及复用这些 DTO 的聚合响应都包含 `url`。
4. 配置 public base URL 后，projected occurrence 不因缺少持久 task row 或 `task_slug` 而缺失 URL，也不得为生成 URL 提前物化。
5. URL 只使用稳定、可路由的公开引用，不暴露数据库内部序号或依赖调用方传入的原始 ref。
6. 所有动态 path segment 必须编码，不能让 `/`、`?`、`#` 等字符改变路径结构。

## 4. 非目标

- 不新增或迁移 Web Console 路由。
- 不从 HTTP `Host`、转发头、监听地址或 Remote CLI 的 `remote.server` 动态推导 canonical origin。
- 不让资源 URL 依赖 workspace SSO 的 `sso.external_base_url`。
- 不把 `server.public_base_url` 写入 workspace 数据库；它是本机/部署级 TOML 配置。
- 不改变项目 slug、task slug、series slug 或 occurrence ref 的生成规则。
- 不为了 URL 物化 projected occurrence。
- 不把 URL 写入数据库；URL 是读取时派生字段。
- 不给 hook、notification delivery 或 automation 模板上下文新增字段；它们不是本次“返回资源信息”的协议响应面。

## 5. URL 契约

### 5.1 项目

```text
/workspaces/{workspaceSlug}/projects/{projectSlug}
```

示例：

```text
/workspaces/dajee/projects/agentapi
```

项目始终属于一个 workspace，且有稳定 slug。配置 public base URL 后 `ProjectView.URL` 必须非空；未配置时必须为空字符串。

### 5.2 项目内普通任务

```text
/workspaces/{workspaceSlug}/projects/{projectSlug}/tasks/{taskSlug}
```

示例：

```text
/workspaces/dajee/projects/agentapi/tasks/agentapi-17
```

项目内普通任务已经满足 `project`、`project_id`、`project_seq` 同时成立的不变量，App 层使用派生后的 `task_slug`，不使用任务 UUID。

### 5.3 已物化 occurrence

已物化 occurrence 已经有 `task_slug`，使用与普通项目任务相同的详情 URL：

```text
/workspaces/dajee/projects/agentapi/tasks/agentapi-18
```

虽然 `TaskOccurrenceView.ID` 仍是稳定 `occurrence_ref`，URL 中优先使用 `task_slug`，使路径保持短且便于人工识别。

### 5.4 Projected occurrence

projected occurrence 没有 UUID、`project_seq` 或 `task_slug`，但有稳定 `occurrence_ref`。URL 使用现有 task detail 路由接受的公开引用：

```text
/workspaces/{workspaceSlug}/projects/{projectSlug}/tasks/{encodedOccurrenceRef}
```

示例：

```text
/workspaces/dajee/projects/agentapi/tasks/occ%3A2d87d074-8b41-4bb3-9052-916f66ba215c%3A1784303999
```

生成 URL 时只读取 series 已回填的 `ProjectSlug`，不得创建 task row。projected 和 materialized 状态切换后，URL 允许从 occurrence ref 形式变为 task slug 形式；资源的稳定身份仍由响应中的 `id` 表达，`url` 表达当前首选导航路径。

### 5.5 无项目任务

无项目任务没有 project slug 和 task slug，使用当前独立任务详情入口：

```text
/tasks/{uuid}
```

示例：

```text
/tasks/7b4d901e-5e4f-4cfa-b42c-38fc7089880a
```

该规则保证配置 public base URL 后，所有实际返回的任务 view 都有绝对 URL，而不是因为缺少项目而省略字段。

### 5.6 循环系列

```text
/workspaces/{workspaceSlug}/projects/{projectSlug}/series/{seriesSlug}
```

示例：

```text
/workspaces/dajee/projects/agentapi/series/agentapi-s-3
```

series 创建时已经分配 project 内序号，repository 会回填 project slug，因此配置 public base URL 后 `TaskSeriesView.URL` 必须非空。

## 6. 架构设计

### 6.1 配置层生成唯一 Resource Base URL

新增本机/部署级 TOML 配置：

```toml
[server]
public_base_url = "https://xuanchu.example.com"
```

`server.public_base_url` 只允许 `http` 或 `https` 绝对 origin，不允许 userinfo、query、fragment 或额外 path；尾部 `/` 归一化删除。空值合法。配置层将它与现有 `server.console.base_path` 合成 `Config.ResourceBaseURL()`：

- public base 为空：返回 `""`。
- Console base path 为 `/`：返回规范化 public base。
- Console base path 为 `/admin`：返回 `{public_base_url}/admin`。

环境变量 `XUANCHU_PUBLIC_BASE_URL` 覆盖 TOML，便于部署系统注入。`config.toml` 与 `config.example.toml` 都必须展示该配置。

### 6.2 App 层是资源路径所有者

在 `internal/app` 增加一个专门的资源 URL 构造文件，提供项目、任务和系列的纯函数。构造函数只接收已经解析完成的稳定字段，不查数据库、不读取 HTTP request，也不依赖 CLI 或 MCP。

建议接口：

```go
func ProjectURL(resourceBaseURL, workspaceSlug, projectSlug string) string
func ProjectTaskURL(resourceBaseURL, workspaceSlug, projectSlug, taskRef string) string
func StandaloneTaskURL(resourceBaseURL, taskRef string) string
func TaskSeriesURL(resourceBaseURL, workspaceSlug, projectSlug, seriesSlug string) string
```

`resourceBaseURL` 为空时四个函数都返回空字符串。非空时追加 canonical Web Console 路径；所有动态 segment 使用统一的 segment 编码辅助函数。编码结果必须与 Web Console 使用 `encodeURIComponent` 生成的路径一致，尤其要把 occurrence ref 中的 `:` 编码为 `%3A`。

### 6.3 View 装配

三个 App view 增加字段：

```go
type ProjectView struct {
    URL string
    // existing fields
}

type TaskOccurrenceView struct {
    URL string
    // existing fields
}

type TaskSeriesView struct {
    URL string
    // existing fields
}
```

`Config.ResourceBaseURL()` 经 CLI、HTTP Server 和 MCP Server 构造链路注入 `app.ServiceOptions.ResourceBaseURL`，URL 在 view 构造阶段填充：

- project row 转 `ProjectView` 时使用 service runtime 的 workspace slug。
- task row 转 `TaskOccurrenceView` 时，项目任务优先使用 `task_slug`，无项目任务使用 UUID。
- projected occurrence 构造时同时带上 series 的 project slug，并使用 occurrence ref。
- series 转 `TaskSeriesView` 时使用 `SeriesSlugOf`。

如果持久数据破坏既有 project/task/series slug 不变量，沿用现有 invariant error；只有 public base URL 未配置时才允许返回空 URL。

### 6.4 输出层只透传

各输出层不得重新拼接路径：

- HTTP DTO 增加 `json:"url"` 并复制 App view 的 `URL`。
- MCP project/task/series map 或 typed view 增加 `url`。
- CLI `--json` 增加 `url`；项目、任务和系列的人类可读详情增加 `URL:` 行，列表中的每条资源也展示 URL。
- Remote DTO 增加 `url`，转换回 App view 时保留 URL，保证本地 CLI 和 remote CLI 一致。
- OpenAPI/Huma schema 与 Web TypeScript view 类型同步声明 `url: string`。

HTTP 首页等复合响应继续复用现有 project/task DTO 转换，因此不新增单独的 URL 拼接逻辑。

## 7. 输出示例

### 7.1 项目

```json
{
  "id": "43789d19-499c-4cf7-a146-12f5050278d0",
  "slug": "agentapi",
  "name": "Agent API",
  "url": "https://xuanchu.example.com/workspaces/dajee/projects/agentapi"
}
```

### 7.2 普通任务

```json
{
  "id": "7b4d901e-5e4f-4cfa-b42c-38fc7089880a",
  "task_slug": "agentapi-17",
  "project": "agentapi",
  "title": "补齐 URL 契约",
  "url": "https://xuanchu.example.com/workspaces/dajee/projects/agentapi/tasks/agentapi-17"
}
```

### 7.3 Projected occurrence

```json
{
  "id": "occ:2d87d074-8b41-4bb3-9052-916f66ba215c:1784303999",
  "task_slug": null,
  "project": "agentapi",
  "url": "https://xuanchu.example.com/workspaces/dajee/projects/agentapi/tasks/occ%3A2d87d074-8b41-4bb3-9052-916f66ba215c%3A1784303999",
  "recurrence_info": {
    "materialization": "projected"
  }
}
```

### 7.4 循环系列

```json
{
  "id": "2d87d074-8b41-4bb3-9052-916f66ba215c",
  "series_slug": "agentapi-s-3",
  "project_slug": "agentapi",
  "title": "每日巡检",
  "url": "https://xuanchu.example.com/workspaces/dajee/projects/agentapi/series/agentapi-s-3"
}
```

## 8. 兼容性

`url` 是只读新增字段：

- HTTP、MCP 和 CLI JSON 的现有字段不改名、不删除。
- Remote client 接受新增字段并向 App view 透传。
- task import/export 的持久化 `task.JSONTask` 不增加 `url`；URL 不是可导入数据。
- URL 读取时派生，不写数据库；部署域名变化只需修改 `server.public_base_url` 并重启对应进程。
- 未配置 public base URL 时，所有输出层仍保留 `url` 字段并返回 `""`。
- projected occurrence 的 URL 不保证在物化后保持字符串不变，但物化前后的路径都能打开同一个 occurrence 语义下的任务详情。

## 9. 错误与不变量

- public base URL 为空：`url` 返回空字符串，不报错，不返回相对路径。
- public base URL 非法：配置加载失败，错误明确指向 `server.public_base_url`。
- 配置 public base URL 后，项目 view 缺少 workspace slug 或 project slug：视为 App 装配错误，不允许输出伪造 URL。
- 项目任务同时缺少可用 project slug 或 task slug：沿用任务项目绑定 invariant error。
- projected occurrence 缺少 series project slug：视为 series repository 回填错误。
- series 缺少 project slug 或 series slug：沿用 series/project sequence invariant error。
- URL 构造函数不吞掉错误字段，不退化为数据库 UUID 路由，只有无项目任务明确使用 UUID。

## 10. 测试与验收

### 10.1 App 单元测试

- 项目 URL。
- 项目普通任务 URL。
- 已物化 occurrence 使用 task slug。
- projected occurrence 使用编码后的 occurrence ref，且构造过程不物化。
- 无项目任务使用 `/tasks/{uuid}`。
- series 使用当前 `/series/{seriesSlug}` 路由。
- 特殊 path segment 被编码，不能改变路径层级。
- public base URL 为空时四类 URL 都为空。
- public base URL 与 `/admin` Console base path 正确合成。

### 10.2 协议测试

- HTTP project list/detail/create/modify/transition/archive 返回 `url`。
- HTTP task list/detail/create/write-action 和 series list/detail/create/modify/stop 返回 `url`。
- MCP project、task、task-series tools 的结构化结果返回相同 `url`。
- CLI local 与 remote 的 JSON 输出完全一致，且人类可读项目、任务、系列输出包含 URL。
- Remote DTO round-trip 不丢失 URL。
- Huma/OpenAPI schema 和 Web TypeScript 类型包含必填 `url`。
- 首页等复合响应中的 project/task 子对象自然包含 `url`。

### 10.3 全量验证

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
```

如果任务详情或列表渲染发生变化，额外运行：

```bash
pnpm --dir web run smoke:editing
```

## 11. 验收标准

1. 任一真实 `ProjectView`、`TaskOccurrenceView`、`TaskSeriesView` 的 `URL` 要么为空，要么是绝对 HTTP(S) URL，永远不是相对路径。
2. 配置 `server.public_base_url` 后，同一资源经 HTTP、MCP、CLI local、CLI remote 返回完全相同的绝对 URL。
3. 未配置 `server.public_base_url` 时，同一资源经各协议都返回 `url: ""`，资源读取本身仍成功。
4. 返回的项目、普通任务、无项目任务、projected occurrence、materialized occurrence 和系列 URL 都能由当前 Web Console 路由打开。
5. projected occurrence 仅为读取 URL 不产生数据库写入。
6. 不存在 `/tasks/{taskSlug}/series/{seriesSlug}` 输出，也不新增该 Web 路由。
7. SQLite 与 PostgreSQL 行为一致，且继续满足零 CGO 测试和构建要求。
