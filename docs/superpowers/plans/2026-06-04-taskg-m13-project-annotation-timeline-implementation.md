# M13：项目 Annotation 与 Timeline Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 xuanchu 增加项目级别 annotation 和聚合 timeline 能力，让 Agent 能记录项目级信息并查看完整项目时间线。

**Architecture:** 新增 `project_annotations` 表和 `ProjectAnnotationRepository`，在 `app` 层提供 `ProjectAnnotate`/`ProjectDenotate`/`ProjectAnnotations`/`ProjectTimeline` 四个方法，通过 CLI、HTTP API、MCP 三端暴露。Timeline 通过 SQL UNION 聚合 project + task annotations。同时强化 `normalizeProjectSlug` 拒绝数字开头的 slug。

**Tech Stack:** Go 1.25、GORM、github.com/glebarez/sqlite、Cobra、chi、MCP SDK

**Spec:** `docs/superpowers/specs/2026-06-04-xuanchu-m13-project-annotation-timeline-design.md`

---

## Chunk 1：数据层

### Task 1：ProjectAnnotation model + migration

**Files:**
- Modify: `internal/storage/models.go` — 新增 `ProjectAnnotation` struct，`Project` 增加 `Annotations` 关联
- Modify: `internal/storage/db.go` — AutoMigrate 加入 `ProjectAnnotation`，M5 relation schema 加入 `project_annotations`

- [ ] **Step 1：新增 ProjectAnnotation model**

在 `models.go` 中 `Project` struct 之后添加：

```go
type ProjectAnnotation struct {
	ID        string `gorm:"primaryKey"`
	ProjectID string `gorm:"not null;index:idx_project_annotations_project;uniqueIndex:idx_project_annotations_entry,priority:1"`
	Entry     int64  `gorm:"not null;uniqueIndex:idx_project_annotations_entry,priority:2"`
	Content   string `gorm:"not null;type:text"`
	CreatedBy string `gorm:"not null"`
	CreatedAt int64  `gorm:"not null"`
}
```

在 `Project` struct 中增加关联字段：

```go
Annotations []ProjectAnnotation `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
```

- [ ] **Step 2：AutoMigrate 注册**

在 `db.go` 的 `AutoMigrate` 调用中加入 `&ProjectAnnotation{}`。

- [ ] **Step 3：M5 relation schema 加入 project_annotations**

在 `db.go` 的 `m5TaskRelationSchemas` 变量之后（或末尾），参考 `task_links` 的模式添加 `project_annotations` 的 schema 定义：

```go
{
    table:   "project_annotations",
    columns: []string{"id", "project_id", "entry", "content", "created_by", "created_at"},
    ddl: `CREATE TABLE project_annotations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    entry INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    CONSTRAINT fk_project_annotations_project FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
)`,
    indexes: []string{
        "CREATE UNIQUE INDEX IF NOT EXISTS idx_project_annotations_entry ON project_annotations(project_id, entry)",
        "CREATE INDEX IF NOT EXISTS idx_project_annotations_project ON project_annotations(project_id)",
    },
},
```

注意：M5 relation schema 的 rebuild 函数目前只检查 `task_uuid` 外键到 `tasks` 表。`project_annotations` 用 `project_id` 外键到 `projects` 表，需要检查是否需要扩展 rebuild 逻辑。如果 `rebuildM5TaskRelationForeignKeys` 只处理 task 相关的表，这里不需要改动——GORM AutoMigrate 会自动建表，M5 rebuild 只是补充外键约束。对于 project_annotations，GORM 的 `constraint:OnDelete:CASCADE` 已经在模型上标注，AutoMigrate 时会创建约束。

- [ ] **Step 4：验证**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 5：新增 migration 测试**

在 `internal/storage/db_test.go` 中添加测试验证 `project_annotations` 表和索引被正确创建。

- [ ] **Step 6：Commit**

```bash
git add -A
git commit -m "feat(m13): ProjectAnnotation model 和 migration"
```

### Task 2：ProjectAnnotationRepository CRUD

**Files:**
- Create: `internal/storage/project_annotation_repo.go`
- Create: `internal/storage/project_annotation_repo_test.go`

- [ ] **Step 1：实现 ProjectAnnotationRepository**

新建 `project_annotation_repo.go`，参考 `task_link_repo.go` 的模式：

```go
package sqlite

import (
    "errors"
    "gorm.io/gorm"
)

type ProjectAnnotationRepository struct {
    db *gorm.DB
}

func NewProjectAnnotationRepository(db *gorm.DB) *ProjectAnnotationRepository {
    return &ProjectAnnotationRepository{db: db}
}

func (r *ProjectAnnotationRepository) Create(annotation ProjectAnnotation) (ProjectAnnotation, error) {
    if err := r.db.Create(&annotation).Error; err != nil {
        return ProjectAnnotation{}, err
    }
    return annotation, nil
}

func (r *ProjectAnnotationRepository) Delete(id string) error {
    result := r.db.Where("id = ?", id).Delete(&ProjectAnnotation{})
    if result.Error != nil {
        return result.Error
    }
    if result.RowsAffected == 0 {
        return ErrNotFound
    }
    return nil
}

func (r *ProjectAnnotationRepository) ListByProject(projectID string) ([]ProjectAnnotation, error) {
    var annotations []ProjectAnnotation
    if err := r.db.Where("project_id = ?", projectID).Order("entry ASC").Find(&annotations).Error; err != nil {
        return nil, err
    }
    return annotations, nil
}

func (r *ProjectAnnotationRepository) GetByID(id string) (ProjectAnnotation, error) {
    var annotation ProjectAnnotation
    err := r.db.Where("id = ?", id).First(&annotation).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return ProjectAnnotation{}, ErrNotFound
    }
    if err != nil {
        return ProjectAnnotation{}, err
    }
    return annotation, nil
}

func (r *ProjectAnnotationRepository) RecentByProject(projectID string, limit int) ([]ProjectAnnotation, error) {
    var annotations []ProjectAnnotation
    if err := r.db.Where("project_id = ?", projectID).Order("entry DESC").Limit(limit).Find(&annotations).Error; err != nil {
        return nil, err
    }
    for i, j := 0, len(annotations)-1; i < j; i, j = i+1, j-1 {
        annotations[i], annotations[j] = annotations[j], annotations[i]
    }
    return annotations, nil
}
```

- [ ] **Step 2：实现 TimelineByProjectID 聚合查询**

在同一个 repo 文件中添加：

```go
type TimelineRow struct {
    SourceType  string
    SourceID    string
    SourceLabel string
    Entry       int64
    Content     string
    CreatedBy   string
}

func (r *ProjectAnnotationRepository) TimelineByProjectID(projectID string, limit, offset int) ([]TimelineRow, error) {
    var rows []TimelineRow
    projectSlug := ""
    r.db.Model(&Project{}).Where("id = ?", projectID).Select("slug").Row().Scan(&projectSlug)
    sql := `
        SELECT source_type, source_id, source_label, entry, content, created_by FROM (
            SELECT 'project' AS source_type, ? AS source_id, ? AS source_label,
                   entry, content, created_by
            FROM project_annotations WHERE project_id = ?
            UNION ALL
            SELECT 'task' AS source_type, t.uuid AS source_id, t.description AS source_label,
                   ta.entry, ta.description AS content, '' AS created_by
            FROM task_annotations ta
            JOIN tasks t ON t.uuid = ta.task_uuid
            WHERE t.project_id = ?
        )
        ORDER BY entry ASC
        LIMIT ? OFFSET ?
    `
    if err := r.db.Raw(sql, projectID, projectSlug, projectID, projectID, limit, offset).Scan(&rows).Error; err != nil {
        return nil, err
    }
    return rows, nil
}
```

注意：task_annotations 没有 `created_by` 字段，需要用空字符串填充。如果后续需要 task annotation 的创建者信息，需要在 task_annotations 表中加字段，但 M13 不做这个。

- [ ] **Step 3：编写测试**

新建 `project_annotation_repo_test.go`，参考 `task_link_repo_test.go`，覆盖：
- Create 成功
- Create 后 ListByProject 验证
- Delete 成功
- Delete 不存在的 ID 返回 ErrNotFound
- GetByID 成功
- GetByID 不存在返回 ErrNotFound
- RecentByProject 限制条数
- TimelineByProjectID 合并 project + task annotations
- TimelineByProjectID limit/offset 分页

- [ ] **Step 4：验证**

```bash
CGO_ENABLED=0 go test ./internal/storage/... -v -run TestProjectAnnotation
CGO_ENABLED=0 go test ./internal/storage/... -v -run TestTimeline
```

- [ ] **Step 5：Commit**

```bash
git add -A
git commit -m "feat(m13): ProjectAnnotationRepository CRUD 和 timeline 聚合查询"
```

### Task 3：强化 normalizeProjectSlug

**Files:**
- Modify: `internal/app/project.go` — `normalizeProjectSlug` 拒绝数字开头
- Modify: `internal/app/service_test.go` — 补充 slug 校验测试

- [ ] **Step 1：修改 normalizeProjectSlug**

在 `internal/app/project.go` 的 `normalizeProjectSlug` 函数中，在现有校验之后添加数字开头检查：

```go
func normalizeProjectSlug(slug string) (string, error) {
    slug = strings.TrimSpace(strings.ToLower(slug))
    if slug == "" {
        return "", RuntimeError{Code: "project_invalid_slug", Message: "project slug is invalid"}
    }
    if slug[0] >= '0' && slug[0] <= '9' {
        return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q must not start with a digit", slug)}
    }
    for _, ch := range slug {
        if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
            continue
        }
        return "", RuntimeError{Code: "project_invalid_slug", Message: fmt.Sprintf("project slug %q is invalid", slug)}
    }
    return slug, nil
}
```

- [ ] **Step 2：补充测试**

在 `service_test.go` 中添加：

```go
func TestNormalizeProjectSlugRejectsDigitStart(t *testing.T) {
    for _, slug := range []string{"123", "1abc", "0project"} {
        _, err := normalizeProjectSlug(slug)
        if err == nil {
            t.Errorf("normalizeProjectSlug(%q) should reject digit-starting slug", slug)
        }
    }
}
```

注意：`normalizeProjectSlug` 不是导出函数，测试需要写在 `app` 包内。如果现有测试中没有直接测试这个函数，可以通过 `AddProject` 间接测试。

- [ ] **Step 3：验证**

```bash
CGO_ENABLED=0 go test ./internal/app/... -v -run TestProject
```

- [ ] **Step 4：Commit**

```bash
git add -A
git commit -m "feat(m13): normalizeProjectSlug 拒绝数字开头的 slug"
```

---

## Chunk 2：App 层

### Task 4：Domain model 扩展

**Files:**
- Modify: `internal/app/project.go` — `ProjectView` 增加 `RecentAnnotations` 字段

- [ ] **Step 1：扩展 ProjectView**

在 `internal/app/project.go` 的 `ProjectView` struct 中添加：

```go
type ProjectView struct {
    // ... 现有字段 ...
    RecentAnnotations []ProjectAnnotationInfo
}

type ProjectAnnotationInfo struct {
    ID        string
    ProjectID string
    Entry     int64
    Content   string
    CreatedBy string
    CreatedAt int64
}
```

- [ ] **Step 2：Commit**

```bash
git add -A
git commit -m "feat(m13): ProjectView 增加 RecentAnnotations"
```

### Task 5：App 层 — ProjectAnnotate / ProjectDenotate / ProjectAnnotations / ProjectTimeline

**Files:**
- Modify: `internal/app/project.go` — 新增 4 个 service 方法
- Modify: `internal/app/hook_event.go` — 新增 `buildProjectAnnotatedHookEvent` / `buildProjectDenotatedHookEvent`

- [ ] **Step 1：新增 hook event 构建函数**

在 `internal/app/hook_event.go` 中，参考 `buildProjectArchivedHookEvent`，新增：

```go
func buildProjectAnnotatedHookEvent(project ProjectView, annotation ProjectAnnotationInfo, runtime RuntimeContext, now int64) HookEvent {
    return HookEvent{
        EventID:       uuid.NewString(),
        EventType:     "project.annotated",
        EventVersion:  1,
        OccurredAt:    now,
        ActorUserID:   runtime.ActorUserID,
        WorkspaceID:   runtime.WorkspaceID,
        WorkspaceSlug: runtime.WorkspaceSlug,
        ProjectID:     &project.ID,
        ProjectSlug:   &view.Slug,
        ObjectKind:    "project",
        ObjectID:      project.ID,
        Data:          map[string]any{"annotation_id": annotation.ID, "content_preview": truncateString(annotation.Content, 200)},
    }
}

func buildProjectDenotatedHookEvent(project ProjectView, annotationID string, runtime RuntimeContext, now int64) HookEvent {
    return HookEvent{
        EventID:       uuid.NewString(),
        EventType:     "project.denotated",
        EventVersion:  1,
        OccurredAt:    now,
        ActorUserID:   runtime.ActorUserID,
        WorkspaceID:   runtime.WorkspaceID,
        WorkspaceSlug: runtime.WorkspaceSlug,
        ProjectID:     &project.ID,
        ProjectSlug:   &view.Slug,
        ObjectKind:    "project",
        ObjectID:      project.ID,
        Data:          map[string]any{"annotation_id": annotationID},
    }
}
```

需要辅助函数：

```go
func truncateString(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen]
}
```

注意：`stringPtr` 已存在于 `audit.go:179`，可直接使用。hook_event.go 中 `ProjectSlug` 字段直接用 `&pv.Slug` 赋值即可（参考 `buildProjectArchivedHookEvent`）。`strPtr` 只在 test 文件中存在，app 层不要重复定义。

- [ ] **Step 2：实现 ProjectAnnotate**

在 `project.go` 中添加，参考 `Annotate`（`service.go:743`）的 withAuditAndEvents 模式：

```go
func (s *Service) ProjectAnnotate(projectRef, content string) (ProjectAnnotationInfo, error) {
    if err := s.Require(PermissionProjectManage); err != nil {
        return ProjectAnnotationInfo{}, err
    }
    var result ProjectAnnotationInfo
    err := s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
        annotation, project, err := tx.projectAnnotateLocked(projectRef, content)
        if err != nil {
            return nil, nil, err
        }
        result = annotation
        view := projectViewFromRow(project, 0)
        entry := AuditEntry{
            Action:     "project.annotate",
            WorkspaceID: &project.WorkspaceID,
            ProjectID:   &project.ID,
            TargetType:  "project",
            TargetID:    project.ID,
            Payload:     map[string]any{"annotation_id": annotation.ID, "content_preview": truncateString(annotation.Content, 200)},
        }
        event := buildProjectAnnotatedHookEvent(view, annotation, tx.runtime, tx.clock.Unix())
        return []AuditEntry{entry}, []HookEvent{event}, nil
    })
    return result, err
}

func (s *Service) projectAnnotateLocked(projectRef, content string) (ProjectAnnotationInfo, sqlite.Project, error) {
    content = strings.TrimSpace(content)
    if content == "" {
        return ProjectAnnotationInfo{}, sqlite.Project{}, RuntimeError{Code: "annotation_content_required", Message: "annotation content is required"}
    }
    project, err := s.ResolveProject(projectRef)
    if err != nil {
        return ProjectAnnotationInfo{}, sqlite.Project{}, err
    }
    if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
        return ProjectAnnotationInfo{}, sqlite.Project{}, RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
    }
    now := s.clock.Unix()
    entry := now
    repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
    for attempts := 0; attempts < 3; attempts++ {
        annotation := sqlite.ProjectAnnotation{
            ID:        uuid.NewString(),
            ProjectID: project.ID,
            Entry:     entry,
            Content:   content,
            CreatedBy: s.runtime.ActorUserID,
            CreatedAt: now,
        }
        created, err := repo.Create(annotation)
        if err != nil {
            if sqlite.IsUniqueConstraintError(err) {
                entry = now + int64(attempts+1)
                continue
            }
            return ProjectAnnotationInfo{}, sqlite.Project{}, err
        }
        project.ModifiedAt = now
        if err := s.projectRepo.Update(project); err != nil {
            return ProjectAnnotationInfo{}, sqlite.Project{}, err
        }
        return projectAnnotationInfoFromModel(created), project, nil
    }
    return ProjectAnnotationInfo{}, sqlite.Project{}, RuntimeError{Code: "annotation_conflict", Message: "could not resolve annotation entry conflict"}
}
```

需要辅助函数：

```go
func projectAnnotationInfoFromModel(m sqlite.ProjectAnnotation) ProjectAnnotationInfo {
    return ProjectAnnotationInfo{
        ID:        m.ID,
        ProjectID: m.ProjectID,
        Entry:     m.Entry,
        Content:   m.Content,
        CreatedBy: m.CreatedBy,
        CreatedAt: m.CreatedAt,
    }
}
```

- [ ] **Step 3：实现 ProjectDenotate**

```go
func (s *Service) ProjectDenotate(projectRef, annotationID string) error {
    if err := s.Require(PermissionProjectManage); err != nil {
        return err
    }
    return s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
        project, err := tx.projectDenotateLocked(projectRef, annotationID)
        if err != nil {
            return nil, nil, err
        }
        view := projectViewFromRow(project, 0)
        entry := AuditEntry{
            Action:      "project.denotate",
            WorkspaceID: &project.WorkspaceID,
            ProjectID:   &project.ID,
            TargetType:  "project",
            TargetID:    project.ID,
            Payload:     map[string]any{"annotation_id": annotationID},
        }
        event := buildProjectDenotatedHookEvent(view, annotationID, tx.runtime, tx.clock.Unix())
        return []AuditEntry{entry}, []HookEvent{event}, nil
    })
}

func (s *Service) projectDenotateLocked(projectRef, annotationID string) (sqlite.Project, error) {
    project, err := s.ResolveProject(projectRef)
    if err != nil {
        return sqlite.Project{}, err
    }
    if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
        return sqlite.Project{}, RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
    }
    repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
    annotation, err := repo.GetByID(annotationID)
    if err != nil {
        return sqlite.Project{}, RuntimeError{Code: "annotation_not_found", Message: fmt.Sprintf("annotation %q not found", annotationID)}
    }
    if annotation.ProjectID != project.ID {
        return sqlite.Project{}, RuntimeError{Code: "annotation_not_found", Message: fmt.Sprintf("annotation %q not found", annotationID)}
    }
    if err := repo.Delete(annotationID); err != nil {
        return sqlite.Project{}, err
    }
    project.ModifiedAt = s.clock.Unix()
    if err := s.projectRepo.Update(project); err != nil {
        return sqlite.Project{}, err
    }
    return project, nil
}
```

- [ ] **Step 4：实现 ProjectAnnotations（读取）**

```go
func (s *Service) ProjectAnnotations(projectRef string) ([]ProjectAnnotationInfo, error) {
    if err := s.Require(PermissionProjectRead); err != nil {
        return nil, err
    }
    project, err := s.ResolveProject(projectRef)
    if err != nil {
        return nil, err
    }
    repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
    rows, err := repo.ListByProject(project.ID)
    if err != nil {
        return nil, err
    }
    result := make([]ProjectAnnotationInfo, 0, len(rows))
    for _, row := range rows {
        result = append(result, projectAnnotationInfoFromModel(row))
    }
    return result, nil
}
```

- [ ] **Step 5：实现 ProjectTimeline（聚合）**

```go
type TimelineOptions struct {
    Limit  int
    Offset int
}

type TimelineEntry struct {
    SourceType  string `json:"source_type"`
    SourceID    string `json:"source_id"`
    SourceLabel string `json:"source_label"`
    Entry       int64  `json:"entry"`
    Content     string `json:"content"`
    CreatedBy   string `json:"created_by"`
}

func (s *Service) ProjectTimeline(projectRef string, opts TimelineOptions) ([]TimelineEntry, error) {
    if err := s.Require(PermissionProjectRead); err != nil {
        return nil, err
    }
    project, err := s.ResolveProject(projectRef)
    if err != nil {
        return nil, err
    }
    if opts.Limit <= 0 {
        opts.Limit = 100
    }
    repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
    rows, err := repo.TimelineByProjectID(project.ID, opts.Limit, opts.Offset)
    if err != nil {
        return nil, err
    }
    result := make([]TimelineEntry, 0, len(rows))
    for _, row := range rows {
        result = append(result, TimelineEntry(row))
    }
    return result, nil
}
```

- [ ] **Step 6：修改 ProjectInfo 加载 RecentAnnotations**

修改 `project.go` 中的 `ProjectInfo` 和 `projectViewForRow`：

```go
func (s *Service) projectViewForRow(project sqlite.Project) (ProjectView, error) {
    counts, err := s.projectRepo.TaskCounts(project.WorkspaceID, []string{project.ID})
    if err != nil {
        return ProjectView{}, err
    }
    view := projectViewFromRow(project, counts[project.ID])
    repo := sqlite.NewProjectAnnotationRepository(s.store.DB())
    recent, err := repo.RecentByProject(project.ID, 5)
    if err != nil {
        return ProjectView{}, err
    }
    for _, a := range recent {
        view.RecentAnnotations = append(view.RecentAnnotations, projectAnnotationInfoFromModel(a))
    }
    return view, nil
}
```

- [ ] **Step 7：验证**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./internal/app/... -v -run TestProject
```

- [ ] **Step 8：Commit**

```bash
git add -A
git commit -m "feat(m13): App 层 ProjectAnnotate/Denotate/Annotations/Timeline"
```

### Task 6：App 层测试

**Files:**
- Modify: `internal/app/service_test.go`

- [ ] **Step 1：编写测试**

覆盖场景：
- `TestServiceProjectAnnotate` — 创建 project，添加 annotation，验证返回字段和通过 `ProjectAnnotations` 能读到
- `TestServiceProjectAnnotateDuplicateEntry` — 添加 annotation，验证时间戳冲突自动解决（不报错）
- `TestServiceProjectDenotate` — 添加后删除，验证列表为空
- `TestServiceProjectDenotateNotFound` — 删除不存在的 annotation，报错
- `TestServiceProjectAnnotateRejectsArchived` — archived 项目不能添加
- `TestServiceProjectAnnotateRejectsEmptyContent` — 空 content 报错
- `TestServiceProjectTimeline` — 添加 project annotation + 创建 task 并 annotate，验证 timeline 合并两条记录
- `TestServiceProjectAnnotateUpdatesModifiedAt` — 验证 modified_at 被更新

参考 `TestServiceBindAndUnbindExternalID` 的风格。

- [ ] **Step 2：验证**

```bash
CGO_ENABLED=0 go test ./internal/app/... -v -run TestServiceProject
```

- [ ] **Step 3：Commit**

```bash
git add -A
git commit -m "test(m13): App 层 project annotation 测试"
```

---

## Chunk 3：HTTP API + Remote Client

### Task 7：HTTP API annotation + timeline 端点

**Files:**
- Modify: `internal/httpapi/projects.go` — 新增 handler
- Modify: `internal/httpapi/router.go` — 注册路由

- [ ] **Step 1：新增 request/response DTO**

在 `projects.go` 中添加：

```go
type addProjectAnnotationRequest struct {
    Content string `json:"content"`
}

type projectAnnotationResponse struct {
    ID        string `json:"id"`
    ProjectID string `json:"project_id"`
    Entry     int64  `json:"entry"`
    Content   string `json:"content"`
    CreatedBy string `json:"created_by"`
    CreatedAt int64  `json:"created_at"`
}

type timelineEntryResponse struct {
    SourceType  string `json:"source_type"`
    SourceID    string `json:"source_id"`
    SourceLabel string `json:"source_label"`
    Entry       int64  `json:"entry"`
    Content     string `json:"content"`
    CreatedBy   string `json:"created_by"`
}
```

- [ ] **Step 2：实现 handler**

```go
func (s *Server) handleProjectAnnotationAdd(w http.ResponseWriter, r *http.Request) {
    var req addProjectAnnotationRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
        return
    }
    ref := chi.URLParam(r, "projectRef")
    scoped, _, err := s.scopedService(r, "project:write", app.PermissionProjectManage, ref)
    if err != nil {
        writeAppError(w, err)
        return
    }
    annotation, err := scoped.ProjectAnnotate(ref, req.Content)
    if err != nil {
        writeAppError(w, err)
        return
    }
    writeSuccess(w, http.StatusCreated, projectAnnotationToJSON(annotation), nil)
}

func (s *Server) handleProjectAnnotationList(w http.ResponseWriter, r *http.Request) {
    ref := chi.URLParam(r, "projectRef")
    scoped, _, err := s.scopedService(r, "project:read", app.PermissionProjectRead, ref)
    if err != nil {
        writeAppError(w, err)
        return
    }
    annotations, err := scoped.ProjectAnnotations(ref)
    if err != nil {
        writeAppError(w, err)
        return
    }
    writeSuccess(w, http.StatusOK, projectAnnotationsToJSON(annotations), nil)
}

func (s *Server) handleProjectAnnotationDelete(w http.ResponseWriter, r *http.Request) {
    ref := chi.URLParam(r, "projectRef")
    annotationID := chi.URLParam(r, "annotationID")
    if annotationID == "" {
        writeError(w, http.StatusBadRequest, "annotation_id_required", "annotation ID is required", nil)
        return
    }
    scoped, _, err := s.scopedService(r, "project:write", app.PermissionProjectManage, ref)
    if err != nil {
        writeAppError(w, err)
        return
    }
    if err := scoped.ProjectDenotate(ref, annotationID); err != nil {
        writeAppError(w, err)
        return
    }
    writeSuccess(w, http.StatusOK, map[string]any{"deleted": true}, nil)
}

func (s *Server) handleProjectTimeline(w http.ResponseWriter, r *http.Request) {
    ref := chi.URLParam(r, "projectRef")
    scoped, _, err := s.scopedService(r, "project:read", app.PermissionProjectRead, ref)
    if err != nil {
        writeAppError(w, err)
        return
    }
    limit := 100
    if raw := r.URL.Query().Get("limit"); raw != "" {
        if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
            limit = parsed
        }
    }
    offset := 0
    if raw := r.URL.Query().Get("offset"); raw != "" {
        if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
            offset = parsed
        }
    }
    entries, err := scoped.ProjectTimeline(ref, app.TimelineOptions{Limit: limit, Offset: offset})
    if err != nil {
        writeAppError(w, err)
        return
    }
    writeSuccess(w, http.StatusOK, timelineEntriesToJSON(entries), nil)
}
```

辅助函数：

```go
func projectAnnotationToJSON(a app.ProjectAnnotationInfo) projectAnnotationResponse {
    return projectAnnotationResponse{ID: a.ID, ProjectID: a.ProjectID, Entry: a.Entry, Content: a.Content, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt}
}

func projectAnnotationsToJSON(annotations []app.ProjectAnnotationInfo) []projectAnnotationResponse {
    out := make([]projectAnnotationResponse, 0, len(annotations))
    for _, a := range annotations {
        out = append(out, projectAnnotationToJSON(a))
    }
    return out
}

func timelineEntriesToJSON(entries []app.TimelineEntry) []timelineEntryResponse {
    out := make([]timelineEntryResponse, 0, len(entries))
    for _, e := range entries {
        out = append(out, timelineEntryResponse(e))
    }
    return out
}
```

- [ ] **Step 3：注册路由**

在 `router.go` 中添加：

```go
api.With(s.authMiddleware).Post("/api/v1/projects/{projectRef}/annotations", s.handleProjectAnnotationAdd)
api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}/annotations", s.handleProjectAnnotationList)
api.With(s.authMiddleware).Delete("/api/v1/projects/{projectRef}/annotations/{annotationID}", s.handleProjectAnnotationDelete)
api.With(s.authMiddleware).Get("/api/v1/projects/{projectRef}/timeline", s.handleProjectTimeline)
```

- [ ] **Step 4：验证**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 5：Commit**

```bash
git add -A
git commit -m "feat(m13): HTTP API project annotation 和 timeline 端点"
```

### Task 8：HTTP API 测试

**Files:**
- Modify: `internal/httpapi/projects_test.go`（新建或追加到 `tasks_test.go`）

- [ ] **Step 1：编写测试**

覆盖：
- `TestProjectAnnotationAddAndList` — 创建 project，POST 添加 annotation，GET 列出验证
- `TestProjectAnnotationDelete` — 添加后 DELETE，验证列表为空
- `TestProjectTimeline` — 添加 project annotation + 创建 task 并 annotate，GET timeline 验证合并

- [ ] **Step 2：验证**

```bash
CGO_ENABLED=0 go test ./internal/httpapi/... -v -run TestProjectAnnotation
```

- [ ] **Step 3：Commit**

```bash
git add -A
git commit -m "test(m13): HTTP API project annotation 测试"
```

### Task 9：Remote Client

**Files:**
- Modify: `internal/remote/project.go` — 新增 4 个 client 方法

- [ ] **Step 1：新增 DTO 和方法**

```go
type ProjectAnnotationDTO struct {
    ID        string `json:"id"`
    ProjectID string `json:"project_id"`
    Entry     int64  `json:"entry"`
    Content   string `json:"content"`
    CreatedBy string `json:"created_by"`
    CreatedAt int64  `json:"created_at"`
}

type TimelineEntryDTO struct {
    SourceType  string `json:"source_type"`
    SourceID    string `json:"source_id"`
    SourceLabel string `json:"source_label"`
    Entry       int64  `json:"entry"`
    Content     string `json:"content"`
    CreatedBy   string `json:"created_by"`
}
```

```go
func (c *Client) AnnotateProject(ctx context.Context, workspace, projectRef, content string) (ProjectAnnotationDTO, error) {
    path := projectPathWithSuffix(workspace, projectRef, "/annotations")
    body := map[string]string{"content": content}
    var envelope apiEnvelope[ProjectAnnotationDTO]
    if err := c.post(ctx, path, body, &envelope); err != nil {
        return ProjectAnnotationDTO{}, err
    }
    return envelope.Data, nil
}

func (c *Client) DenotateProject(ctx context.Context, workspace, projectRef, annotationID string) error {
    path := projectPathWithSuffix(workspace, projectRef, "/annotations/"+url.PathEscape(annotationID))
    var envelope apiEnvelope[map[string]any]
    if err := c.delete(ctx, path, &envelope); err != nil {
        return err
    }
    return nil
}

func (c *Client) ListProjectAnnotations(ctx context.Context, workspace, projectRef string) ([]ProjectAnnotationDTO, error) {
    path := projectPathWithSuffix(workspace, projectRef, "/annotations")
    var envelope apiEnvelope[[]ProjectAnnotationDTO]
    if err := c.get(ctx, path, nil, &envelope); err != nil {
        return nil, err
    }
    return envelope.Data, nil
}

func (c *Client) ProjectTimeline(ctx context.Context, workspace, projectRef string, limit int) ([]TimelineEntryDTO, error) {
    path := projectPathWithSuffix(workspace, projectRef, "/timeline")
    values := url.Values{}
    if limit > 0 {
        values.Set("limit", strconv.Itoa(limit))
    }
    var envelope apiEnvelope[[]TimelineEntryDTO]
    if err := c.get(ctx, path, values, &envelope); err != nil {
        return nil, err
    }
    return envelope.Data, nil
}
```

- [ ] **Step 2：验证**

```bash
CGO_ENABLED=0 go build ./...
```

- [ ] **Step 3：Commit**

```bash
git add -A
git commit -m "feat(m13): Remote Client project annotation 和 timeline 方法"
```

---

## Chunk 4：CLI + MCP + Render

### Task 10：CLI — project annotate/annotations/timeline 子命令

**Files:**
- Modify: `internal/cli/project.go` — 新增子命令

- [ ] **Step 1：注册子命令**

在 `newProjectCommand` 中添加：

```go
cmd.AddCommand(newProjectAnnotateCommand(opts))
cmd.AddCommand(newProjectAnnotationsCommand(opts))
cmd.AddCommand(newProjectTimelineCommand(opts))
```

- [ ] **Step 2：实现 newProjectAnnotateCommand**

```go
func newProjectAnnotateCommand(opts Options) *cobra.Command {
    return &cobra.Command{
        Use:  "annotate <project-ref> <content...>",
        Args: cobra.MinimumNArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            currentOpts := optionsFromCmd(cmd, opts)
            if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
                return err
            } else if remoteMode {
                client, err := buildRemoteClient(currentOpts)
                if err != nil {
                    return err
                }
                _, err = client.AnnotateProject(context.Background(), currentOpts.Workspace, args[0], strings.Join(args[1:], "\n"))
                if err != nil {
                    return err
                }
                fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", args[0])
                return nil
            }
            svc, closeFn, err := buildServiceFromCmd(cmd, opts)
            if err != nil {
                return err
            }
            defer closeFn()
            if _, err := svc.ProjectAnnotate(args[0], strings.Join(args[1:], "\n")); err != nil {
                return err
            }
            fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", args[0])
            return nil
        },
    }
}
```

注意：content 用 `\n` 连接参数，允许多行（每个参数是一行）。或者用空格连接——与 task annotate 一致。确认 task annotate 用的是 `strings.Join(args[1:], " ")`，project annotate 也保持一致。但如果用户要输入多行内容，需要通过 stdin 或文件。M13 先用空格连接，与 task annotate 一致。

- [ ] **Step 3：实现 newProjectAnnotationsCommand**

```go
func newProjectAnnotationsCommand(opts Options) *cobra.Command {
    return &cobra.Command{
        Use:  "annotations <project-ref>",
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            currentOpts := optionsFromCmd(cmd, opts)
            if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
                return err
            } else if remoteMode {
                client, err := buildRemoteClient(currentOpts)
                if err != nil {
                    return err
                }
                annotations, err := client.ListProjectAnnotations(context.Background(), currentOpts.Workspace, args[0])
                if err != nil {
                    return err
                }
                return renderProjectAnnotations(cmd, currentOpts.JSON, annotations)
            }
            svc, closeFn, err := buildServiceFromCmd(cmd, opts)
            if err != nil {
                return err
            }
            defer closeFn()
            annotations, err := svc.ProjectAnnotations(args[0])
            if err != nil {
                return err
            }
            return renderProjectAnnotationsJSON(cmd, currentOpts.JSON, annotations)
        },
    }
}
```

- [ ] **Step 4：实现 newProjectTimelineCommand**

```go
func newProjectTimelineCommand(opts Options) *cobra.Command {
    var limit int
    cmd := &cobra.Command{
        Use:  "timeline <project-ref>",
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            currentOpts := optionsFromCmd(cmd, opts)
            if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
                return err
            } else if remoteMode {
                client, err := buildRemoteClient(currentOpts)
                if err != nil {
                    return err
                }
                entries, err := client.ProjectTimeline(context.Background(), currentOpts.Workspace, args[0], limit)
                if err != nil {
                    return err
                }
                return renderTimelineEntries(cmd, currentOpts.JSON, entries)
            }
            svc, closeFn, err := buildServiceFromCmd(cmd, opts)
            if err != nil {
                return err
            }
            defer closeFn()
            entries, err := svc.ProjectTimeline(args[0], app.TimelineOptions{Limit: limit})
            if err != nil {
                return err
            }
            return renderTimelineEntriesApp(cmd, currentOpts.JSON, entries)
        },
    }
    cmd.Flags().IntVar(&limit, "limit", 50, "maximum timeline entries")
    return cmd
}
```

- [ ] **Step 5：实现 render 函数**

在 `project.go` 底部添加 render 辅助函数：

```go
func renderProjectAnnotations(cmd *cobra.Command, asJSON bool, annotations []remote.ProjectAnnotationDTO) error {
    if asJSON {
        return render.JSON(cmd.OutOrStdout(), annotations)
    }
    if len(annotations) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "No annotations.")
        return nil
    }
    for i, a := range annotations {
        fmt.Fprintf(cmd.OutOrStdout(), "%d [%s] %s\n", i+1, formatUnixTime(a.Entry), a.Content)
    }
    return nil
}

func renderProjectAnnotationsJSON(cmd *cobra.Command, asJSON bool, annotations []app.ProjectAnnotationInfo) error {
    if asJSON {
        return render.JSON(cmd.OutOrStdout(), annotations)
    }
    if len(annotations) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "No annotations.")
        return nil
    }
    for i, a := range annotations {
        fmt.Fprintf(cmd.OutOrStdout(), "%d [%s] %s\n", i+1, formatUnixTime(a.Entry), a.Content)
    }
    return nil
}

func renderTimelineEntries(cmd *cobra.Command, asJSON bool, entries []remote.TimelineEntryDTO) error {
    if asJSON {
        return render.JSON(cmd.OutOrStdout(), entries)
    }
    if len(entries) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "No timeline entries.")
        return nil
    }
    for _, e := range entries {
        label := e.SourceLabel
        if len(label) > 40 {
            label = label[:40] + "..."
        }
        fmt.Fprintf(cmd.OutOrStdout(), "[%s] <%s> %s: %s\n", formatUnixTime(e.Entry), e.SourceType, label, e.Content)
    }
    return nil
}

func renderTimelineEntriesApp(cmd *cobra.Command, asJSON bool, entries []app.TimelineEntry) error {
    if asJSON {
        return render.JSON(cmd.OutOrStdout(), entries)
    }
    if len(entries) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "No timeline entries.")
        return nil
    }
    for _, e := range entries {
        label := e.SourceLabel
        if len(label) > 40 {
            label = label[:40] + "..."
        }
        fmt.Fprintf(cmd.OutOrStdout(), "[%s] <%s> %s: %s\n", formatUnixTime(e.Entry), e.SourceType, label, e.Content)
    }
    return nil
}
```

需要 `formatUnixTime` — 检查 render 包中是否已有类似函数。

经确认，`formatUnixTime` 不存在于代码库中。在 `project.go` 的 render 辅助函数区域添加：

```go
func formatUnixTime(unix int64) string {
    return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}
```

需要导入 `time` 包。

- [ ] **Step 6：修改 project info 渲染，展示 RecentAnnotations**

在 `newProjectInfoCommand` 的文本输出部分，在 `TaskCount` 之后添加：

```go
if len(project.RecentAnnotations) > 0 {
    fmt.Fprintln(cmd.OutOrStdout(), "Recent Annotations:")
    for _, a := range project.RecentAnnotations {
        preview := a.Content
        if len(preview) > 100 {
            preview = preview[:100] + "..."
        }
        fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s\n", formatUnixTime(a.Entry), preview)
    }
}
```

- [ ] **Step 7：验证**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 8：Commit**

```bash
git add -A
git commit -m "feat(m13): CLI project annotate/annotations/timeline 子命令"
```

### Task 11：CLI — 目标风格 annotate 回退

**Files:**
- Modify: `internal/cli/root.go` — `handleTargetAction` 和 `handleRemoteTargetAction` 中增加 project 回退

- [ ] **Step 1：添加 project 回退辅助函数**

在 `root.go` 中添加：

```go
func isPossibleProjectSlug(ref string) bool {
    if len(ref) == 0 {
        return false
    }
    if ref[0] >= '0' && ref[0] <= '9' {
        return false
    }
    for _, ch := range ref {
        if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
            continue
        }
        return false
    }
    return true
}
```

- [ ] **Step 2：修改 handleTargetAction 的 annotate case**

将现有的 `case "annotate"` 修改为：先尝试 task annotate，如果 task 不存在且 ref 是合法 project slug，回退到 project annotate：

```go
case "annotate":
    if len(actionArgs) == 0 {
        return fmt.Errorf("annotate requires a description")
    }
    content := strings.Join(actionArgs, " ")
    err := svc.Annotate(target, content)
    if err != nil {
        if isPossibleProjectSlug(target) {
            _, projectErr := svc.ProjectAnnotate(target, content)
            if projectErr == nil {
                fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", target)
                return nil
            }
        }
        return err
    }
    fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", target)
```

类似处理 `case "annotations"` 和 `case "timeline"`（新增这两个 case）。

- [ ] **Step 3：添加 annotations 和 timeline case**

```go
case "annotations":
    // 先尝试 task
    tsk, err := svc.Info(target)
    if err == nil {
        return renderAnnotations(cmd, currentOpts.JSON, tsk.Annotations)
    }
    // 回退到 project
    if isPossibleProjectSlug(target) {
        annotations, err := svc.ProjectAnnotations(target)
        if err == nil {
            return renderProjectAnnotationsJSON(cmd, currentOpts.JSON, annotations)
        }
    }
    return fmt.Errorf("target %q not found", target)
case "timeline":
    if !isPossibleProjectSlug(target) {
        return fmt.Errorf("timeline is only available for projects")
    }
    entries, err := svc.ProjectTimeline(target, app.TimelineOptions{Limit: 50})
    if err != nil {
        return err
    }
    return renderTimelineEntriesApp(cmd, currentOpts.JSON, entries)
```

- [ ] **Step 4：修改 handleRemoteTargetAction**

在远程模式的 `handleRemoteTargetAction` 中：

修改 `case "annotate"` — 先尝试 task annotate，失败后回退 project：

```go
case "annotate":
    if len(actionArgs) == 0 {
        return fmt.Errorf("annotate requires a description")
    }
    content := strings.Join(actionArgs, " ")
    _, err := client.AnnotateTask(ctx, opts.Workspace, target, content)
    if err != nil {
        if isPossibleProjectSlug(positional[0]) {
            _, projectErr := client.AnnotateProject(ctx, opts.Workspace, target, content)
            if projectErr == nil {
                fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", positional[0])
                return nil
            }
        }
        return err
    }
    fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", positional[0])
```

新增 `case "annotations"`：

```go
case "annotations":
    tsk, err := client.GetTask(ctx, opts.Workspace, target)
    if err == nil {
        return renderAnnotations(cmd, opts.JSON, tsk.Annotations)
    }
    if isPossibleProjectSlug(positional[0]) {
        annotations, err := client.ListProjectAnnotations(ctx, opts.Workspace, target)
        if err == nil {
            return renderProjectAnnotations(cmd, opts.JSON, annotations)
        }
    }
    return fmt.Errorf("target %q not found", positional[0])
```

新增 `case "timeline"`：

```go
case "timeline":
    if !isPossibleProjectSlug(positional[0]) {
        return fmt.Errorf("timeline is only available for projects")
    }
    entries, err := client.ProjectTimeline(ctx, opts.Workspace, target, 50)
    if err != nil {
        return err
    }
    return renderTimelineEntries(cmd, opts.JSON, entries)
```

- [ ] **Step 5：验证**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 6：Commit**

```bash
git add -A
git commit -m "feat(m13): CLI 目标风格 annotate 回退到 project"
```

### Task 12：MCP tools

**Files:**
- Modify: `internal/mcpserver/tools_project.go` — 新增 4 个 MCP tools
- Modify: `internal/mcpserver/integration_test.go` — tool count 更新
- Modify: `internal/mcpserver/testdata/list-tools-default.json` — golden file 更新

- [ ] **Step 1：新增 Input struct**

```go
type ProjectAnnotateInput struct {
    Workspace string `json:"workspace,omitempty"`
    Project   string `json:"project,omitempty"`
    ProjectID string `json:"project_id,omitempty"`
    Content   string `json:"content"`
}

type ProjectDenotateInput struct {
    Workspace    string `json:"workspace,omitempty"`
    Project      string `json:"project,omitempty"`
    ProjectID    string `json:"project_id,omitempty"`
    AnnotationID string `json:"annotation_id"`
}

type ProjectAnnotationsInput struct {
    Workspace string `json:"workspace,omitempty"`
    Project   string `json:"project,omitempty"`
    ProjectID string `json:"project_id,omitempty"`
}

type ProjectTimelineInput struct {
    Workspace string `json:"workspace,omitempty"`
    Project   string `json:"project,omitempty"`
    ProjectID string `json:"project_id,omitempty"`
    Limit     int    `json:"limit,omitempty"`
}
```

- [ ] **Step 2：注册 tools**

在 `registerProjectTools` 中添加：

```go
addTool(s, &mcp.Tool{Name: "project.annotate", Description: "Add an annotation to a project. Annotations are multiline notes that form a project diary."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAnnotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
    ref := projectRefForScope(in.Project, in.ProjectID)
    if strings.TrimSpace(ref) == "" {
        return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
    }
    svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
    if err != nil {
        return businessErrorWithEnvelope(err)
    }
    annotation, err := svc.ProjectAnnotate(ref, in.Content)
    if err != nil {
        return businessErrorWithEnvelope(err)
    }
    return successWithEnvelope(annotation, "annotated project")
})

addTool(s, &mcp.Tool{Name: "project.denotate", Description: "Remove an annotation from a project."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectDenotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
    ref := projectRefForScope(in.Project, in.ProjectID)
    if strings.TrimSpace(ref) == "" {
        return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
    }
    svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
    if err != nil {
        return businessErrorWithEnvelope(err)
    }
    if err := svc.ProjectDenotate(ref, in.AnnotationID); err != nil {
        return businessErrorWithEnvelope(err)
    }
    return successWithEnvelope(map[string]any{"deleted": true}, "removed annotation")
})

addTool(s, &mcp.Tool{Name: "project.annotations", Description: "List all annotations for a project."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAnnotationsInput) (*mcp.CallToolResult, ToolEnvelope, error) {
    ref := projectRefForScope(in.Project, in.ProjectID)
    if strings.TrimSpace(ref) == "" {
        return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
    }
    svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
    if err != nil {
        return businessErrorWithEnvelope(err)
    }
    annotations, err := svc.ProjectAnnotations(ref)
    if err != nil {
        return businessErrorWithEnvelope(err)
    }
    return successWithEnvelope(annotations, fmt.Sprintf("%d annotation(s)", len(annotations)))
})

addTool(s, &mcp.Tool{Name: "project.timeline", Description: "Get aggregated timeline for a project, combining project annotations and task annotations."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectTimelineInput) (*mcp.CallToolResult, ToolEnvelope, error) {
    ref := projectRefForScope(in.Project, in.ProjectID)
    if strings.TrimSpace(ref) == "" {
        return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
    }
    svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
    if err != nil {
        return businessErrorWithEnvelope(err)
    }
    limit := in.Limit
    if limit <= 0 {
        limit = 50
    }
    entries, err := svc.ProjectTimeline(ref, app.TimelineOptions{Limit: limit})
    if err != nil {
        return businessErrorWithEnvelope(err)
    }
    return successWithEnvelope(entries, fmt.Sprintf("%d timeline entry(s)", len(entries)))
})
```

- [ ] **Step 3：更新 golden file 和 tool count**

运行测试更新 `list-tools-default.json` 和 tool count。

- [ ] **Step 4：验证**

```bash
CGO_ENABLED=0 go test ./internal/mcpserver/... -v
```

- [ ] **Step 5：Commit**

```bash
git add -A
git commit -m "feat(m13): MCP project.annotate/denotate/annotations/timeline tools"
```

---

## Chunk 5：集成测试 + 收尾

### Task 13：CLI 集成测试

**Files:**
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1：编写测试**

覆盖：
- `TestCLIProjectAnnotateDenotate` — 创建 project，annotate，列出，denotate
- `TestCLIProjectTimeline` — 创建 project + task，各自 annotate，timeline 展示合并结果
- `TestCLIProjectAnnotateTargetStyle` — 使用 `xuanchu <slug> annotate <content>` 目标风格
- `TestCLIProjectAnnotateRejectsArchived` — archived 项目不能 annotate
- `TestCLIProjectSlugRejectsDigitStart` — 创建数字开头的 project slug 报错

- [ ] **Step 2：验证**

```bash
CGO_ENABLED=0 go test ./tests/integration/... -v -run TestCLIProject
```

- [ ] **Step 3：Commit**

```bash
git add -A
git commit -m "test(m13): CLI 集成测试"
```

### Task 14：全量验证 + ROADMAP 更新

**Files:**
- Modify: `ROADMAP.md`

- [ ] **Step 1：全量验证**

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 2：更新 ROADMAP**

在 ROADMAP 中添加 M13 章节，状态总览更新 M13 为"已完成"，更新"当前下一步"。

- [ ] **Step 3：Commit**

```bash
git add -A
git commit -m "docs(m13): 更新 ROADMAP"
```
