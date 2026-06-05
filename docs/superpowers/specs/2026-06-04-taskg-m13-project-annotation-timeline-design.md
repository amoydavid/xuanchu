# M13：项目 Annotation 与 Timeline

## 概述

本 milestone 为 xuanchu 增加项目级别的 annotation 能力，并提供聚合时间线接口，将项目 annotation 与该项目下所有 task 的 annotation 合并为完整的项目时间线。

## 动机

Agent 在执行任务过程中，会产生大量与项目相关的信息：

- 观察和发现："API 有 rate limit，每天最多调 1000 次"
- 决策记录："决定从 REST 迁移到 gRPC"
- 上下文片段："客户要求在 6 月底前完成 Phase 1"
- 环境变更："生产环境升级到 Go 1.25"

这些信息不属于某个具体任务，但和整个项目强相关。当前没有地方存放。

现有的 `project.description` 是静态长文本，不适合追加式的时间线记录。Task annotation 只能挂在任务上。需要项目级别的 annotation。

核心认知：**project annotation + 该项目下所有 task annotation = 项目完整时间线**。

## 设计

### 一、数据模型

新增 `project_annotations` 表：

```go
type ProjectAnnotation struct {
    ID          string `gorm:"primaryKey"`
    ProjectID   string `gorm:"not null;index:idx_project_annotations_project;uniqueIndex:idx_project_annotations_entry,priority:1"`
    Entry       int64  `gorm:"not null;uniqueIndex:idx_project_annotations_entry,priority:2"`
    Content     string `gorm:"not null;type:text"`
    CreatedBy   string `gorm:"not null"`
    CreatedAt   int64  `gorm:"not null"`
}
```

设计决策：

- `ID` 是 UUID，作为外部引用主键（删除、MCP 引用）。
- `ProjectID` 关联 projects 表，带 `ON DELETE CASCADE`。
- `Entry` 是 Unix 时间戳，用于排序和时间线合并。与 task annotation 的 `Entry` 语义一致。
- `(ProjectID, Entry)` 联合唯一索引，同一时间戳不允许重复（与 task annotation 冲突处理一致）。
- `Content` 是多行文本，支持 Markdown。与 task annotation 的单行限制不同，project annotation 允许更丰富的内容。
- `CreatedBy` 记录写入者的 user ID。
- `CreatedAt` 记录实际写入时间（Unix 时间戳）。与 `Entry` 分离——`Entry` 是语义上的"记录时间"（可由调用方指定或自动填充），`CreatedAt` 是数据库实际写入时间。

在 `Project` GORM 模型中增加关联：

```go
type Project struct {
    // ... 现有字段 ...
    Annotations []ProjectAnnotation `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
}
```

### 二、Domain 层

```go
type ProjectAnnotationInfo struct {
    ID        string
    ProjectID string
    Entry     int64
    Content   string
    CreatedBy string
    CreatedAt int64
}
```

不引入独立的 JSON DTO——project annotation 通过 HTTP API 直接以 snake_case JSON 返回，与现有 project API 风格一致。Remote Client 层可定义自己的传输结构体（`ProjectAnnotationDTO` 等），与 domain model 分离。

### 三、Repository 层

新增 `ProjectAnnotationRepository`：

- `Create(annotation ProjectAnnotation) (ProjectAnnotation, error)` — 创建，冲突时返回错误
- `Delete(id string) error` — 按 ID 删除
- `ListByProject(projectID string) ([]ProjectAnnotation, error)` — 按项目列出，entry 升序
- `GetByID(id string) (ProjectAnnotation, error)` — 按 ID 查询
- `TimelineByProjectID(projectID string, limit, offset int) ([]TimelineRow, error)` — 聚合查询

### 四、App 层

#### 写入

- `ProjectAnnotate(projectRef, content string) (ProjectAnnotationInfo, error)` — 添加项目 annotation
  - 需要 `project:manage` 权限（与 ModifyProject 一致）
  - 项目不能是 archived 状态
  - `content` 不能为空
  - `Entry` 冲突处理：与 task annotation 一致，最多重试 3 次（timestamp 自增）避免并发写入时唯一约束冲突
  - 写入审计条目（`project.annotate`）
  - 触发 `project.annotated` hook 事件
  - 更新 project 的 `modified_at`

- `ProjectDenotate(projectRef, annotationID string) error` — 删除项目 annotation
  - 需要 `project:manage` 权限
  - 写入审计条目（`project.denotate`）
  - 触发 `project.denotated` hook 事件
  - 更新 project 的 `modified_at`

#### 读取

- `ProjectAnnotations(projectRef string) ([]ProjectAnnotationInfo, error)` — 列出项目 annotation
  - 需要 `project:read` 权限
  - archived 项目允许读取 annotation

#### Timeline 聚合

- `ProjectTimeline(projectRef string, opts TimelineOptions) ([]TimelineEntry, error)` — 聚合时间线
  - 需要 `project:read` 权限
  - archived 项目允许读取 timeline
  - 合并 project annotations + 该项目下所有 task annotations（通过 tasks.project_id 过滤）
  - 按 entry（时间戳）升序排列
  - 每条记录标记来源类型（`project` / `task`）
  - 实现策略：repository 层通过 SQL UNION 合并两个查询结果，在 UNION 后做 ORDER BY + LIMIT/OFFSET。先 UNION 再分页，保证跨来源的全局排序正确

```go
type TimelineOptions struct {
    Limit  int
    Offset int
}

type TimelineEntry struct {
    SourceType  string `json:"source_type"`  // "project" 或 "task"
    SourceID    string `json:"source_id"`    // project ID 或 task UUID
    SourceLabel string `json:"source_label"` // project slug 或 task description
    Entry       int64  `json:"entry"`
    Content     string `json:"content"`
    CreatedBy   string `json:"created_by"`
}
```

### 五、CLI

```
xuanchu project annotate <project-ref> <content...>
xuanchu project annotations <project-ref>
xuanchu project timeline <project-ref> [--limit N]
xuanchu <project-ref> annotate <content...>
xuanchu <project-ref> annotations
xuanchu <project-ref> timeline [--limit N]
```

两种入口风格：
- 子命令风格：`xuanchu project annotate <ref> <content>`
- 目标风格：`xuanchu <ref> annotate <content>`

目标风格的歧义处理：`xuanchu <ref> annotate` 中的 ref 可能是 task 也可能是 project。当前 `handleTargetAction` 只处理 task。解决方案：**先尝试作为 task 解析，如果 task 不存在且 ref 看起来像 project slug（小写字母+数字+横线+下划线），则回退为 project**。在 `handleTargetAction` 中增加 `annotate`/`annotations`/`timeline` 的 project 回退分支。如果 ref 同时匹配 task ID 和 project slug，task 优先（保持向后兼容）。

这个歧义处理的可靠性依赖于一个前置约束：**project slug 不允许纯数字、不允许数字开头**。这样 task 的数字 working-set ID（`1`、`23`）和 project slug（`api`、`web-v2`）在词法层面就不会冲突。此约束需要修改现有的 `normalizeProjectSlug` 函数——在创建和修改 project 时拒绝纯数字和数字开头的 slug。

`xuanchu project info <ref>` 输出中展示最近几条 annotation（如最近 5 条）。实现方式：`ProjectInfo` 方法额外加载最近 5 条 annotation，`ProjectView` 增加 `RecentAnnotations []ProjectAnnotationInfo` 字段。`--json` 输出和 HTTP `GET /projects/{ref}` 响应均包含此字段。

### 六、HTTP API

- `POST /api/v1/projects/{projectRef}/annotations` — 添加 annotation（body: `{content}`)
- `GET /api/v1/projects/{projectRef}/annotations` — 列出 annotation
- `DELETE /api/v1/projects/{projectRef}/annotations/{annotationID}` — 删除 annotation
- `GET /api/v1/projects/{projectRef}/timeline` — 聚合时间线（query: `limit`, `offset`）

### 七、MCP

- `project.annotate` — 添加项目 annotation（参数：`project`、`content`）
- `project.denotate` — 删除项目 annotation（参数：`project`、`annotation_id`）
- `project.annotations` — 列出项目 annotation（参数：`project`）
- `project.timeline` — 聚合时间线（参数：`project`、`limit?`）

### 八、Hook 事件

- `project.annotated` — 新增 annotation 时触发，payload 包含 project 信息和 annotation 内容
- `project.denotated` — 删除 annotation 时触发

两个事件参考 `buildProjectArchivedHookEvent` 的模式，新建 `buildProjectAnnotatedHookEvent` 和 `buildProjectDenotatedHookEvent`，`ObjectKind` 为 `project`，`ObjectID` 为 project ID。

### 九、Render

- `xuanchu project info <ref>` 输出中增加 Annotations 段落，展示最近 5 条 annotation（entry 时间 + content 前 100 字符）
- `xuanchu project annotations <ref>` 列表输出：序号 + 时间 + content
- `xuanchu project timeline <ref>` 列表输出：来源标记 + 时间 + content

### 十、审计

- `project.annotate` — 审计条目包含 project ID、annotation ID、content 前 200 字符
- `project.denotate` — 审计条目包含 project ID、annotation ID

### 十一、迁移与兼容

GORM AutoMigrate 自动处理：新增 `project_annotations` 表。

同时将 `project_annotations` 加入 M5 relation schema rebuild 系统（与 `task_links` 一致），确保外键约束正确。

无破坏性变更：
- 现有 project API 响应不变（annotations 是新增的独立端点）
- 现有 hook payload 不受影响（新事件类型是独立的）

### 十二、Remote Client

在 `internal/remote/task.go`（或新建 `project.go`）中增加：
- `AnnotateProject(ctx, workspace, projectRef, content string) (ProjectAnnotationDTO, error)`
- `DenotateProject(ctx, workspace, projectRef, annotationID string) error`
- `ListProjectAnnotations(ctx, workspace, projectRef string) ([]ProjectAnnotationDTO, error)`
- `ProjectTimeline(ctx, workspace, projectRef string, limit int) ([]TimelineEntryDTO, error)`

## 验收标准

1. 项目能添加/删除/查看 annotation（多行文本）
2. CLI 能操作项目 annotation（子命令风格 `xuanchu project annotate` 和目标风格 `xuanchu <ref> annotate`）
3. HTTP API 能操作项目 annotation（CRUD 3 个端点）
4. MCP 能操作项目 annotation（`project.annotate` / `project.denotate` / `project.annotations`）
5. Timeline 聚合接口合并 project + task annotations，按 entry 排序，limit/offset 生效
6. Timeline 通过 CLI（`xuanchu project timeline`）、HTTP API、MCP（`project.timeline`）均可访问
7. 写入触发审计条目和 hook 事件
8. `project info` 展示最近 5 条 annotation（`ProjectView.RecentAnnotations`）
9. Archived 项目允许读取 annotation 和 timeline，不允许写入
10. 所有现有测试继续通过
11. `CGO_ENABLED=0 go build ./cmd/xuanchu` 和 `CGO_ENABLED=0 go test ./...` 通过

## 不做什么

- 不做 annotation 的编辑/更新（追加式记录，不修改历史）
- 不做 annotation 的富文本渲染（存什么输出什么）
- 不做 timeline 的过滤/搜索（M13 只做基础聚合）
- 不做跨项目 timeline
- 不做 annotation 的评论/回复（扁平列表）
- 不限制 content 长度（由调用方自行控制）
