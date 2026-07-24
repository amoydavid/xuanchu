# Web Console 任务 Activity 语义时间线实施计划

**执行状态：** 已实现，待提交（2026-07-24）

| Task | 状态 | 结果 |
|---|---|---|
| Task 1：迁移任务注解 actor 与 Activity 时间字段 | 已完成 | SQLite 迁移与 Taskwarrior JSON 兼容测试通过；PostgreSQL 条件测试已加入，但当前环境未配置 `XUANCHU_TEST_DB_URL`，运行时 skip |
| Task 2：写入注解原作者并补齐 audit payload | 已完成 | user/token actor、原作者保留、事务回滚与 link snapshot 已覆盖 |
| Task 3：安全游标候选查询 | 已完成 | audit/annotation action、workspace scope 与 keyset cursor 已实现 |
| Task 4：App 语义归并 | 已完成 | lifecycle/change/relation/annotation、actor、fallback、去重和跨源分页已实现 |
| Task 5：HTTP API 与 OpenAPI | 已完成 | `GET /api/v1/tasks/{taskRef}/activity`、参数错误、权限和 schema 已实现 |
| Task 6：Web 数据层与 mutation 刷新 | 已完成 | infinite query、query key 与相关 mutation 失效已实现 |
| Task 7：纵向时间线 UI | 已完成 | `<ol>/<li>`、圆点连线、分页、注解操作、description 展开、loading/error/empty 已实现 |
| Task 8：详情切换、回归与文档 | 已完成 | 详情页只请求 Activity；Go/Web/零 CGO/build/smoke 全部通过 |
| Task 9：清理退役字段历史 API | 已完成 | 删除旧 route、App 入口、Web fetch/query/key、OpenAPI 和旧专项文档；通用 Audit Console 保留 |

说明：上表与文末“最终验收清单”是当前状态源；下方各 Task 的 step checkbox 保留为实施前的原始执行脚本，不再用于表达完成度。计划中的逐 Task `Commit` 步骤未执行，因为本轮未被要求创建提交；实现与验证状态不因此记为未完成。

> **For agentic workers:** 按任务顺序执行，使用 Red → Green → Refactor；每个 Task 完成并验证后再进入下一项。若环境提供 `superpowers:executing-plans`，实现阶段应使用该 skill 逐项执行。

**Goal:** 让任务详情页的“活动”成为一条真实、统一、可分页的产品时间线，覆盖任务创建、开始、停止、完成、重新打开、字段变化、链接变化和当前注解，并以左侧圆点与连接线的单列纵向 timeline 展示。

**Architecture:** `audit_logs` 继续保存生命周期、字段和链接动作事实，`task_annotations` 继续保存当前可见注解正文；不新增第二套 append-only Activity 表，也不读取 Hook delivery。Storage 提供 workspace/task scoped 的游标候选查询，App 层负责 action 映射、actor 解析、snapshot fallback、归并排序和去重，HTTP 只暴露稳定的结构化 Activity DTO，Web 通过 infinite query 和统一 `TaskActivityTimeline` 渲染。

**Tech Stack:** Go 1.25 / GORM / `github.com/glebarez/sqlite` / PostgreSQL / chi + Huma；React / TypeScript / TanStack Query / Vitest / Testing Library / shadcn-ui。

**承接 spec：** `docs/superpowers/specs/2026-07-07-web-console-task-detail-redesign-design.md` §9.4、§11.3、§11.4、§12 阶段三。

---

## 实施边界

本计划交付：

- `GET /api/v1/tasks/{taskRef}/activity`，默认 30 条、最大 100 条，使用不透明 cursor。
- lifecycle / change / relation / annotation 四类稳定 Activity 读模型。
- 任务注解 actor 与 `created_at` 的 SQLite、PostgreSQL 迁移和写入契约。
- App 层跨 `audit_logs`、`task_annotations`、创建快照的归并、排序、分页和去重。
- 任务详情页的单列纵向 timeline、加载更多、局部错误、空态、移动端和无障碍行为。
- 所有会产生 Activity 的 task / annotation / link mutation 的 query 失效。

本计划不做：

- 不改变通用 Audit Console `/api/v1/audit` 的 `audit:read` 权限和行为。
- 不新增 `task_activity_entries` 或其他重复事件表。
- 不从 Hook delivery、通知 delivery 或自动化 job 构建任务历史。
- 不展示注解编辑/删除历史；当前注解删除后正文必须从 Activity 消失。
- 不从任务当前 `status/end` 猜测完成、停止或重新打开历史。
- 不在本阶段实现任务列表父子树。

## 跨任务固定契约

1. `TaskActivityEntry.ID` 使用 `audit:<id>`、`annotation:<uuid>` 或 `snapshot:created`，前端直接用它作为 key。
2. 固定排序为 `occurred_at DESC, source_rank DESC, source_id DESC`。首版固定 rank：audit `30`、annotation `20`、snapshot `10`；发布后不能无迁移地改值。
3. cursor 使用带版本号的服务端编码，至少包含 `occurred_at/source_rank/source_id`；Web 只能回传，不能解析。解码、版本或 source ID 不合法时返回 `api_bad_cursor`。
4. lifecycle action 只输出 `created/generated/started/stopped/completed/reopened/deleted`；Web 不接触 `task.add` 等内部 action。
5. 一行 `task.modify` 对应一条 `fields_changed`，其 `changes[]` 保持稳定顺序；不能按字段拆成多条时间线记录。
6. 当前注解只来自 `task_annotations`；`task.annotate` audit 不再产生第二条可见记录。
7. actor 始终使用 `task.ActorInfo` / `task.JSONActorInfo`。自然人必须是完整 `task.UserInfo`，不能输出裸 UUID；内部事件使用 `system`，无法确认的历史主体使用 `unknown`。
8. `task.JSONAnnotation` 继续只有 `id/entry/description`，actor 元数据不进入 Taskwarrior 兼容 JSON。
9. 所有数据库条件使用参数绑定；SQLite 继续使用 `github.com/glebarez/sqlite`，并始终满足 `CGO_ENABLED=0`。
10. 每个 Task 的“提交”步骤只提交该 Task 相关文件；工作区现有 spec 修改属于本功能上下文，不得覆盖或丢弃。

---

# Task 1：迁移任务注解 actor 与 Activity 时间字段

**依赖：** 无。

**Files:**

- Modify: `internal/storage/models.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`
- Test: `internal/storage/db_test.go`
- Test: `internal/storage/postgres_test.go`
- Test: `internal/task/json_test.go`

- [ ] **Step 1：写失败测试——SQLite 旧注解迁移**

在 `internal/storage/db_test.go` 建立只含 `id/task_uuid/entry/description` 的旧 `task_annotations` fixture，再通过 `storage.Open` 迁移，断言：

- 新增 `created_by_actor_type/created_by_user_id/created_by_token_id/created_by_token_name/created_by_token_prefix/created_at`。
- 历史行 `created_by_actor_type = "unknown"`，所有 actor ID/name/prefix 均为 `NULL`。
- 历史行 `created_at = entry`，不是迁移时的当前时间。
- 重复打开数据库幂等，行数和 annotation ID 不变。
- 存在 `(task_uuid, created_at, id)` Activity 查询索引。

- [ ] **Step 2：写失败测试——PostgreSQL 迁移**

在 `internal/storage/postgres_test.go` 增加使用 `XUANCHU_TEST_DB_URL` 的条件测试：先建旧表和历史行，再执行迁移，验证与 SQLite 相同的列、回填和幂等结果。未配置 PostgreSQL 时测试按仓库惯例 skip，不能把 skip 当成已验证 PostgreSQL。

- [ ] **Step 3：写失败测试——Taskwarrior JSON 不变**

在 `internal/task/json_test.go` 固定 annotation JSON golden，确认序列化仍为：

```json
{"id":"annotation-1","entry":100,"description":"保留正文"}
```

不得出现 actor 或 `created_at` 新字段。

- [ ] **Step 4：运行 Red**

Run:

```bash
go test ./internal/storage ./internal/task -run 'Test.*TaskAnnotation.*(Migration|JSON|Actor)' -v
```

Expected: FAIL——迁移列、历史回填或新索引尚不存在；JSON 兼容测试用于锁定边界。

- [ ] **Step 5：实现最小迁移**

在 `TaskAnnotation` 增加：

- `CreatedByActorType string`，默认 `unknown`。
- 可空的 user/token actor 列。
- `CreatedAt int64`。

SQLite 使用显式、幂等迁移：旧行 actor 回填为 `unknown`，`created_at` 回填为 `entry`，并建立 Activity 复合索引。更新 `createTaskAnnotationsWithIDs`，保证全新数据库直接得到终态 schema；已有 v0.2.0 表则只补列和索引，不能重新生成 annotation ID。

PostgreSQL 使用显式 `ADD COLUMN IF NOT EXISTS`、回填、`SET NOT NULL` 和 `CREATE INDEX IF NOT EXISTS`。不要依赖 AutoMigrate 给非空旧表凭空补 `created_at NOT NULL`。

- [ ] **Step 6：运行 Green**

Run:

```bash
go test ./internal/storage ./internal/task -run 'Test.*TaskAnnotation.*(Migration|JSON|Actor)' -v
CGO_ENABLED=0 go test ./internal/storage ./internal/task
```

Expected: PASS。若配置了 `XUANCHU_TEST_DB_URL`，PostgreSQL 迁移测试也必须 PASS。

- [ ] **Step 7：Refactor 与提交**

抽取 SQLite/PostgreSQL 迁移中的列名常量或小 helper，避免同一列清单在多个分支漂移；运行 `gofmt` 和 `git diff --check`。

Commit:

```text
feat: 迁移任务注解活动元数据
```

**验收标准：** 新旧 SQLite 均能打开；PostgreSQL 迁移可重复执行；历史注解明确是 `unknown`；Taskwarrior JSON 无变化；零 CGO 测试通过。

---

# Task 2：写入注解原作者并补齐可追踪 audit payload

**依赖：** Task 1。

**Files:**

- Modify: `internal/storage/task_repo.go`
- Modify: `internal/app/actor.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/task_bundle.go`
- Modify: `internal/app/workspace.go`
- Modify: `internal/app/project_template_instantiate.go`
- Test: `internal/storage/task_repo_test.go`
- Test: `internal/app/service_test.go`（同时覆盖 service 注解与 `workspace.go` 链接 audit）
- Test: `internal/app/task_bundle_test.go`

- [ ] **Step 1：写失败测试——runtime actor 与事务 payload**

覆盖 user actor 和 tenant token actor：

- `Annotate` 后注解行保存 `RuntimeContext.actorColumns()` 和 `created_at`。
- 同一事务写出的 `task.annotate` audit payload 包含准确 `annotation_id`。
- 人为让 audit append 失败时，注解行也回滚，证明不是先提交注解再补 audit。
- `AddWithAnnotations` 创建的注解使用当前 runtime actor。

- [ ] **Step 2：写失败测试——编辑和任务更新不改原作者**

先创建有 actor 的注解，再执行：

- `UpdateAnnotation` 修改正文。
- 普通 `Modify` 导致 `TaskRepository.Update`。

两次操作后均断言 annotation ID、原 actor 和原 `created_at` 不变。这个测试必须先失败，因为当前 `TaskRepository.Update` 会删除并重建注解行，导致 actor 元数据丢失。

- [ ] **Step 3：写失败测试——不可信导入使用 unknown**

对 task bundle/import 及任何直接通过 `repo.Create/Update` 恢复 annotation 的路径，断言导入正文和 `entry` 保留，但 actor 为 `unknown`、`created_at = entry`；不得把执行导入的当前用户冒充为历史作者。

- [ ] **Step 4：写失败测试——链接 audit 有稳定摘要**

锁定 add/update/remove payload 至少包含 `link_id/type/url/title`。remove 必须在删除前读取并保存摘要；旧 audit 缺字段时后续 Activity 只能返回已有字段，不能用当前链接内容伪造历史。

- [ ] **Step 5：运行 Red**

Run:

```bash
go test ./internal/storage ./internal/app -run 'Test.*(AnnotationActor|AnnotationAudit|Annotation.*Preserve|Bundle.*Annotation|Link.*Audit)' -v
```

Expected: FAIL——注解行没有 actor 字段、audit 没有 `annotation_id`、普通 task update 会清空作者，链接 remove payload 也没有摘要。

- [ ] **Step 6：实现最小写入契约**

- 让 storage 的注解创建入参能携带 actor columns 和 `CreatedAt`，但对无可信来源的调用统一写 `unknown`。
- `annotateLocked` 返回本次创建的 annotation ID/row，使 `Annotate`、`AddWithAnnotations` 能在同一外层事务构建含 `annotation_id` 的 audit。
- `UpdateAnnotation` 只更新 description 和任务 modified，不更新原作者、entry、created_at。
- 重构 `TaskRepository.Update` 的 annotation 同步：已有 ID 只更新兼容字段；新增但无作者元数据的 annotation 写 `unknown`；删除缺失 ID 时保持原有语义。禁止再用“全部删除后无元数据重建”的实现。
- task bundle/import 没有作者字段，保持 `unknown`；当前用户直接创建的模板/普通注解走 runtime actor 路径。
- link add/update/remove audit 统一构建结构化摘要，remove 在删行前取快照。

- [ ] **Step 7：运行 Green**

Run:

```bash
go test ./internal/storage ./internal/app -run 'Test.*(AnnotationActor|AnnotationAudit|Annotation.*Preserve|Bundle.*Annotation|Link.*Audit)' -v
CGO_ENABLED=0 go test ./internal/storage ./internal/app
```

Expected: PASS。

- [ ] **Step 8：Refactor 与提交**

把 annotation actor row 转换和 link audit 摘要构建收敛到各自单一 helper，避免写路径复制字段清单。

Commit:

```text
feat: 记录任务注解原作者
```

**验收标准：** 新注解能区分 user/token；编辑和任意 task update 不改作者；导入历史不冒充操作者；annotation 与 audit 原子提交；链接删除后仍有真实历史摘要。

---

# Task 3：为 audit 与 annotation 增加安全的游标候选查询

**依赖：** Task 1。

**Files:**

- Modify: `internal/storage/audit_repo.go`
- Modify: `internal/storage/task_repo.go`
- Test: `internal/storage/audit_repo_test.go`
- Test: `internal/storage/task_repo_test.go`
- Test: `internal/storage/postgres_test.go`

- [ ] **Step 1：写失败测试——audit actions 与稳定 cursor**

为 `AuditRepository` 增加测试：

- `Actions []string` 只返回 Activity 白名单 action，使用参数绑定的 `IN`，空白名单不等于“查全部”。
- 现有 `Action *string` 和 offset 调用继续工作，旧 Audit Console 不受影响。
- 同秒 audit 按 `created_at DESC, id DESC`；给定边界后严格返回更小排序键。
- workspace、target type、target ID 同时约束；跨 workspace 同 UUID 数据不可见。
- 提供轻量 existence 查询，用于判断 task 是否已有 `task.add` 或 `task.recurrence.generated`，不能靠当前页猜 snapshot fallback。

- [ ] **Step 2：写失败测试——annotation Activity row 与 cursor**

新增只供 Activity 使用的 annotation row 查询，断言：

- 返回正文、actor columns、`created_at`，不改变现有 `ListAnnotations(offset, limit)` 的公开行为。
- 按 `created_at DESC, id DESC` 稳定排序；同秒分页无重复、无漏项。
- 查询必须通过 `EXISTS tasks(workspace_id, uuid)` 或等价 join 同时绑定 workspace 和 task UUID。
- cursor 位于不同 source rank 时，正确决定同秒数据是全含、全排除还是按 source ID 继续。

- [ ] **Step 3：写 PostgreSQL 条件测试**

复用同一 repository API 在 PostgreSQL 验证 actions、workspace scope、同秒 cursor；未配置数据库时 skip。

- [ ] **Step 4：运行 Red**

Run:

```bash
go test ./internal/storage -run 'Test(AuditRepository|TaskRepository).*Activity' -v
```

Expected: FAIL——现有 audit 仅支持单 action + offset，annotation 只返回 domain JSON 形状且没有 cursor/actor。

- [ ] **Step 5：实现最小查询能力**

- 给 `AuditListOptions` 增加多 action 和游标边界；保留旧字段兼容，明确拒绝同时传入互相冲突的单 action/多 action。
- 使用 GORM 参数绑定构建 `action IN ?` 和 keyset predicate，禁止拼接前端输入。
- 新增 storage-only annotation Activity row/view，包含 actor columns。
- source rank 比较由固定服务端值决定；repository 只接受已经校验的边界结构，不负责解码 HTTP cursor。
- 每个 source 查询 `limit + 1` 个候选，不做 offset 后再跨源拼接。

- [ ] **Step 6：运行 Green**

Run:

```bash
go test ./internal/storage -run 'Test(AuditRepository|TaskRepository).*Activity' -v
CGO_ENABLED=0 go test ./internal/storage
```

Expected: PASS；配置 PostgreSQL 时相同测试通过。

- [ ] **Step 7：Refactor 与提交**

抽取共用 keyset 条件构建器时，只抽排序边界，不把 Activity action 映射下沉到 storage。

Commit:

```text
feat: 增加任务活动游标查询
```

**验收标准：** 两类事实源都能按统一边界取候选；同秒顺序稳定；跨 workspace 不泄漏；旧 audit/annotation offset API 保持兼容。

---

# Task 4：实现 App 层 TaskActivity 语义归并

**依赖：** Task 2、Task 3。

**Files:**

- New: `internal/app/task_activity.go`
- Modify: `internal/app/actor.go`
- Modify: `internal/app/audit.go`
- Modify: `internal/app/task_audit_payload.go`
- New: `internal/app/task_activity_test.go`
- New: `internal/app/actor_test.go`

- [ ] **Step 1：写失败测试——action 语义映射和去重**

覆盖完整白名单：

- `task.add → lifecycle/created`
- `task.recurrence.generated → lifecycle/generated`
- `task.start/stop/done/reopen/delete → lifecycle` 对应 action
- 非空 `task.modify → change/fields_changed`
- `task.link.add/update/remove → relation` 对应 action
- 当前 annotation row → `annotation/commented`

同时断言空 changes modify、annotate/update/denotate、append/prepend/edit、Hook delivery 均不出现；一个 modify 的多个 changes 保持在同一 entry；annotation 与 `task.annotate` audit 只出现一次。

- [ ] **Step 2：写失败测试——完整 actor**

插入 user、tenant token、system、unknown 四类事实，断言：

- user 经一次批量 `resolveUserInfos` 得到 `id/name/display_name/email/external_ids`。
- token 返回 token ID/name/prefix，不伪造 user。
- system/unknown 只设置正确 type，不生成空 ID 的自然人对象。
- 历史 annotation actor 为 unknown；不得 fallback 到当前用户、负责人或 workspace 创建者。

- [ ] **Step 3：写失败测试——创建 fallback**

- 有 `task.add`：只有一条 created。
- recurrence occurrence 有 `task.recurrence.generated`：只有 generated，不再加 created。
- 两者都没有：从 task `entry` 合成唯一 `snapshot:created`，actor unknown。
- 当前任务即使 status=completed 且有 end，没有 `task.done` 时也不能合成 completed。

- [ ] **Step 4：写失败测试——跨源排序和 cursor**

构造同秒 audit、annotation、snapshot 以及前后时间数据，分页大小设为 2，逐页断言：

- 总顺序严格按 `(occurred_at, source_rank, source_id)` 倒序。
- `next_cursor` 非空时继续查询无重复、无漏项。
- 第一页返回后插入更新活动，再取下一页仍不重复已读项。
- 非法 base64、未知版本、错误 source rank、与 rank 不匹配的 source ID 返回 `RuntimeError{Code: "api_bad_cursor"}`。

- [ ] **Step 5：写失败测试——权限与 scope**

viewer/member 只有 `PermissionTaskRead` 时可以读取 Activity；没有 task read 时拒绝。传 task UUID、slug 或来自其他任务/工作区的 cursor 都不能改变已解析 task 的 workspace/target scope。

- [ ] **Step 6：运行 Red**

Run:

```bash
go test ./internal/app -run 'Test.*TaskActivity' -v
```

Expected: FAIL——`TaskActivityEntry/Page` 和 `ListTaskActivity` 尚不存在。

- [ ] **Step 7：实现最小 App 读模型**

在 `task_activity.go` 定义：

- `TaskActivityInput{Limit, Cursor}`。
- `TaskActivityEntry`、`TaskActivityAnnotation`、`TaskActivityLink`、`TaskActivityPage`。
- versioned cursor encode/decode 和固定 source rank。
- 内部 audit action → 公共 kind/action 的显式 map。

`ListTaskActivity(taskRef, input)`：

1. 只检查 `PermissionTaskRead`，用 `ResolveTaskReferenceForRead` 固定当前 workspace 的 task UUID。
2. 对 cursor 做完整校验，再向 audit/annotation repositories 各取 `limit + 1` 候选。
3. 用 existence 查询决定是否需要 `snapshot:created`。
4. 批量收集所有 actor user ID，一次解析为 `task.UserInfo`。
5. 解析现有 `TaskFieldChange` 和 link audit 摘要；历史缺失字段保持空，不向前端暴露原始 payload。
6. 归并、稳定排序、截取 limit，并由最后一条生成 next cursor。

扩展 `actorInfoFromColumns`：明确处理 `system/unknown`，不能沿用“非 token 一律 user”的旧 fallback。

- [ ] **Step 8：运行 Green**

Run:

```bash
go test ./internal/app -run 'Test.*TaskActivity' -v
CGO_ENABLED=0 go test ./internal/app
```

Expected: PASS。

- [ ] **Step 9：Refactor 与提交**

保持 `auditLogViewsFromRows` 与 Activity mapper 分工：共用纯 payload parser，但不要让通用 Audit view 承担产品语义映射。

Commit:

```text
feat: 实现任务活动语义读模型
```

**验收标准：** 创建和完成等事件真实出现；没有伪造历史；actor 完整；同秒/跨源/插入后翻页稳定；普通 task reader 可读。

---

# Task 5：暴露 task-read Activity HTTP API 与 OpenAPI

**依赖：** Task 4。

**Files:**

- New: `internal/httpapi/task_activity.go`
- Modify: `internal/httpapi/huma_routes.go`
- New: `internal/httpapi/task_activity_test.go`
- Modify: `internal/httpapi/server_test.go`

- [ ] **Step 1：写失败测试——响应契约**

请求：

```text
GET /api/v1/tasks/{taskRef}/activity?workspace=<workspace>&limit=2&cursor=<opaque>
```

断言 envelope `data.entries` 包含：

- `id/kind/action/actor/occurred_at`。
- 可选的 `changes/annotation/link` 结构，缺失时不输出无意义空对象。
- `occurred_at` 为 UTC RFC3339 字符串。
- 自然人 actor 为 `task.JSONActorInfo` 内完整 `JSONUserInfo`，字段名使用 `id`，不是 `user_id`。
- `next_cursor` 原样可用于下一页。

- [ ] **Step 2：写失败测试——参数、权限和兼容**

- limit 默认 30，最大 100；0、负数、非数字、超过 100 返回 `api_bad_limit`。
- 非法 cursor 返回 400 + `api_bad_cursor`。
- 只有 task read 的 viewer 能 200；缺 task read 返回既有权限错误。
- cursor、annotation ID 或 task UUID 不能读取另一 workspace 数据。
- 字段变化继续通过 Activity 返回稳定 `changes`；不存在第二套任务详情历史路由。
- 无 `audit:read` 仍不能访问通用 Audit Console。

- [ ] **Step 3：写失败测试——OpenAPI schema**

断言 Huma 文档包含新 GET route、query 参数、page/entry/actor schemas，不把内部 audit payload schema 暴露给 Activity。

- [ ] **Step 4：运行 Red**

Run:

```bash
go test ./internal/httpapi -run 'Test.*TaskActivity' -v
```

Expected: FAIL——路由返回 404 或 DTO 不存在。

- [ ] **Step 5：实现最小 HTTP 层**

- handler 只做 path/query 解析、scoped service 创建、App 调用和 DTO 序列化。
- 使用 `auth.ScopeTaskRead` + `app.PermissionTaskRead`，不调用 `PermissionAuditRead`。
- actor 统一调用 `task.ActorInfoToJSON()`。
- `occurred_at` 在 HTTP 边界转 UTC RFC3339；App 保持 int64 便于排序。
- 在 `huma_routes.go` 注册 `/api/v1/tasks/{taskRef}/activity` 及准确 schema。

- [ ] **Step 6：运行 Green**

Run:

```bash
go test ./internal/httpapi -run 'Test.*TaskActivity' -v
CGO_ENABLED=0 go test ./internal/httpapi
```

Expected: PASS。

- [ ] **Step 7：Refactor 与提交**

复用现有 actor OpenAPI schema；不要复制第二套 user schema。

Commit:

```text
feat: 暴露任务活动接口
```

**验收标准：** route、权限、分页、错误码、时间和 actor JSON 全部符合 spec；字段 changes 与通用 Audit Console 不退化。

---

# Task 6：接入 Web Activity API、infinite query 与 mutation 刷新

**依赖：** Task 5。

**Files:**

- Modify: `web/src/features/workspace/project-workbench/api/task-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/task-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.test.tsx`
- New: `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.test.tsx`

- [ ] **Step 1：写失败测试——types、path 和 fetch**

定义 TypeScript discriminated types，并测试：

- `taskActivityPath(workspace, taskRef, {limit, cursor})` 正确编码 path、workspace 和 opaque cursor。
- `getTaskActivity` 返回 `{entries, next_cursor}`，不把 cursor 解析为客户端状态。
- `TaskActivityActor` 覆盖 user/token/system/unknown，user 字段复用现有 `UserInfo`。

- [ ] **Step 2：写失败测试——infinite query**

`useTaskActivityQuery` 使用 `useInfiniteQuery`：首屏不带 cursor，`getNextPageParam` 只读 `next_cursor`，加载下一页追加而不清空旧 pages；workspace/task ref 为空时 disabled。

- [ ] **Step 3：写失败测试——所有写操作失效 Activity**

在 mutation 测试逐项断言以下成功路径失效 `taskQueryKeys.activity(workspace, taskRef)`：

- modify。
- start / stop / done / reopen / delete。
- annotation add / update / remove。
- link add / update / remove。

保留原本 task、project tasks、home、my-tasks 等失效，不得为加 Activity 刷新而删掉现有行为。

- [ ] **Step 4：运行 Red**

Run:

```bash
pnpm --dir web test -- task-api.test.ts use-task-detail-data.test.tsx use-task-mutations.test.tsx
```

Expected: FAIL——Activity types/path/query key 尚不存在，mutation 仍只刷新 audit 或其他页面。

- [ ] **Step 5：实现最小数据层**

- 新增 `TaskActivityPage/Entry` types、path 和 fetch。
- 在 `taskQueryKeys` 新增稳定的 `activity` key；不保留重复的任务详情历史 query key。
- 新增 `useTaskActivityQuery` infinite query。
- 抽取 `invalidateTaskActivity` 小 helper，覆盖所有会产生或移除 Activity 的 mutation。
- modify 从详情页视角以 Activity 为主；旧 audit 若仍有其他消费者可继续一并失效，不要删除 API 兼容。

- [ ] **Step 6：运行 Green**

Run:

```bash
pnpm --dir web test -- task-api.test.ts use-task-detail-data.test.tsx use-task-mutations.test.tsx
pnpm --dir web typecheck
```

Expected: PASS。

- [ ] **Step 7：Refactor 与提交**

统一 alias ref 的 Activity 失效策略，避免 UUID、task slug、route ref 各留一份陈旧缓存。

Commit:

```text
feat: 接入任务活动分页数据
```

**验收标准：** 前端只消费稳定 Activity contract；加载更多保留旧数据；任何相关写操作完成后 Activity 自动刷新。

---

# Task 7：实现单列纵向 TaskActivityTimeline

**依赖：** Task 6。

**Files:**

- New: `web/src/features/workspace/project-workbench/task-detail/task-activity-timeline.tsx`
- New: `web/src/features/workspace/project-workbench/task-detail/task-activity-entry.tsx`
- New: `web/src/features/workspace/project-workbench/task-detail/task-activity-timeline.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/activity-section.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.test.tsx`
- Delete after moving reusable renderers: `web/src/features/workspace/project-workbench/task-detail/task-change-history.tsx`
- Delete after replacing coverage: `web/src/features/workspace/project-workbench/task-detail/task-change-history.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1：写失败测试——语义条目渲染**

构造交错 Activity page，断言：

- lifecycle 使用公共 action i18n，如“完成了任务”，DOM 不出现 `task.done`。
- actor fallback 为 `display_name → name → id`；token/system/unknown 分别显示 token name/“系统”/“未知主体”。
- 一次多字段 modify 只渲染一个 `<li>`，内部列出全部 changes。
- description change 继续使用可展开 dialog，不在主线展开完整长 Markdown。
- annotation 正文使用现有 Markdown renderer；其编辑/删除操作复用当前 mutation 和权限逻辑。
- link 只渲染 API 提供的结构化摘要，不读取 raw payload。

- [ ] **Step 2：写失败测试——timeline 结构与视觉状态**

使用 Testing Library 断言：

- 列表为 `<ol>`，每条为 `<li>`，DOM 顺序最新在上。
- 每条都有固定轨道列；圆点和连接线 `aria-hidden="true"`，不进入条目 accessible name。
- 首条节点上方无线；无下一页时末条下方无线；有下一页时末条保留延伸线。
- “加载更多”与内容列对齐，点击后调用 `fetchNextPage`，加载期间保留已有条目。
- 内容不是逐条大卡片；桌面和移动端共用同一组件和 DOM 语义。

样式实现应满足：轨道约 16px、节点 6–8px、线 1px、条目间距 16–20px；具体 class 可使用现有 token 微调，测试优先锁结构和状态，不锁每个 Tailwind 数值。

- [ ] **Step 3：写失败测试——composer、loading/error/empty**

- composer 在 `<ol>` 上方，不带圆点或连接线。
- 初始 loading 使用含节点和短线的 skeleton，避免数据返回时横向跳动。
- empty 显示“暂无活动”，不显示孤立节点/线，也不再显示“暂无字段级变更记录”。
- error 显示“活动暂不可用”和重试；`ActivitySection` 之外的任务详情仍正常存在。
- 注解 hover/focus 操作区预留固定宽度，按钮出现不改变轨道位置。

- [ ] **Step 4：运行 Red**

Run:

```bash
pnpm --dir web test -- task-activity-timeline.test.tsx task-annotations-editor.test.tsx task-change-history.test.tsx
```

Expected: FAIL——当前 `ActivitySection` 仍是 `TaskAnnotationsEditor + TaskChangeHistory` 两段式布局。

- [ ] **Step 5：实现最小 timeline**

- 将 `TaskAnnotationsEditor` 拆为“composer”和可复用的 annotation row actions；列表正文只由 Activity API 提供，避免详情 task snapshot 与 Activity 各渲染一次。
- 把 `TaskChangeHistory` 中的 field formatter、description dialog 迁移到可供 `TaskActivityEntry` 复用的纯组件/helper；迁移测试覆盖后删除旧组件和旧专属测试。
- `TaskActivityTimeline` flatten infinite pages，并以 `entry.id` 去重防御重复 page；后端排序仍是权威，不在前端按时间重排。
- 使用 grid/flex 两列布局：左轨道、右内容。线在节点后方，首尾/next cursor 状态由条目位置决定。
- `ActivitySection` 只组合 composer + query state + timeline。
- 补齐中英文 action、actor、空态、错误、重试、加载更多文案。

- [ ] **Step 6：运行 Green**

Run:

```bash
pnpm --dir web test -- task-activity-timeline.test.tsx task-annotations-editor.test.tsx task-change-history.test.tsx
pnpm --dir web typecheck
pnpm --dir web lint
```

Expected: PASS。

- [ ] **Step 7：Refactor 与提交**

清除 `activity-section.tsx` 的“过渡态/TODO”注释；保持 actor/action formatter 和轨道结构各自单一职责。

Commit:

```text
feat: 重构任务活动纵向时间线
```

**验收标准：** 用户能在一条线中看到创建、完成、字段、链接和注解；小点连续、首尾正确；composer 不伪装成事件；移动端和无障碍语义一致。

---

# Task 8：详情页切换、回归测试、文档同步与全量验证

**依赖：** Task 7。

**Files:**

- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/activity-section.tsx`
- Modify: `docs/superpowers/specs/2026-07-07-web-console-task-detail-redesign-design.md`
- Modify: `ROADMAP.md`
- Modify: `README.md`

- [ ] **Step 1：写失败集成测试——详情页只取 Activity**

更新 `task-detail-page.test.tsx` 的 API mocks：

- 页面加载只调用 `getTaskActivity`，不在前端拼接第二套字段历史。
- lifecycle、change、relation、annotation 按服务端顺序交错出现。
- Activity 首屏失败只影响活动区，不影响标题、正文、子任务和属性栏。
- 桌面与移动端活动 tab 使用同一个 `TaskActivityTimeline`。
- 加载更多追加条目；无 next cursor 时不显示入口。

- [ ] **Step 2：运行 Red**

Run:

```bash
pnpm --dir web test -- task-detail-page.test.tsx
```

Expected: FAIL——旧 mocks 和详情页仍假定字段历史 + task annotations 两个数据源。

- [ ] **Step 3：完成切换和兼容清理**

- 删除 TaskDetailPage/ActivitySection 对旧字段历史 query 的依赖。
- 全仓 `rg` 确认旧 route、App 入口、TypeScript fetch/query/query key 和 OpenAPI 均已删除。
- 确认已删除的 `TaskChangeHistory` 没有残留 import；字段 changes 只由 Activity contract 提供。
- 确认没有前端代码直接映射 `task.add/task.done/task.modify` 到 Activity 文案。

- [ ] **Step 4：同步文档**

代码和测试全部通过后才更新 spec：

- 将阶段三状态改为已实现，记录最终文件/API 名称和任何经实现验证的细节。
- 更新 ROADMAP v0.5.5 中“Activity 视觉合并过渡态”的描述，记录阶段三语义时间线完成，并把后续阶段明确为任务列表父子树。
- 更新 README 任务详情段落：将 `/tasks/{ref}/activity` 明确为唯一的任务详情历史接口，并说明通用 Audit Console 不受影响。
- 不把本计划未实现的阶段四写成已完成。

- [ ] **Step 5：运行前端 Green**

Run:

```bash
pnpm --dir web test -- task-detail-page.test.tsx
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

Expected: 全部 PASS。

- [ ] **Step 6：运行后端与零 CGO 全量验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
```

若配置 PostgreSQL：

```bash
go test ./internal/storage ./tests/integration -run 'Postgres|TaskActivity' -v
```

Expected: 全部 PASS。没有配置 PostgreSQL 时，交付说明必须明确“PostgreSQL 测试因未配置而 skip”，不能写成已通过。

- [ ] **Step 7：最终审查**

使用 `rg` 和 `git diff --stat` 检查：

- 没有新增 CGO SQLite driver。
- 没有 Activity append-only 表。
- 没有 Hook delivery → Activity 读取路径。
- 没有裸用户 UUID actor JSON。
- 没有从 status/end 合成 completed。
- 没有在 Web 渲染内部 audit action/raw payload。
- 只包含本阶段相关文件，未覆盖用户其他改动。

- [ ] **Step 8：提交**

Commit:

```text
docs: 完成任务活动时间线交付
```

**验收标准：** 详情页完全切到 Activity API；重复的任务详情历史入口已删除；Go/Web/零 CGO 验证通过；spec/roadmap 与实现一致；工作区无意外产物。

---

# Task 9：清理退役字段历史 API

**状态：已完成。**

- [x] Red：OpenAPI 测试先断言字段级任务审计 route 不再发布，并确认旧实现下失败。
- [x] Green：删除 HTTP route/handler、App 专用读取入口、Web fetch/query/query key 和无效 mutation 刷新。
- [x] Refactor：把原端点的字段 JSON 与跨 HTTP/MCP/CLI 审计验证迁移到 Activity；保留显式 null、空集合和标量结构覆盖。
- [x] 文档：删除已被 Activity 完全取代的专项 spec/plan，并同步 README、ROADMAP、当前 spec/plan。
- [x] 边界：保留 `task.modify` 审计写入、Activity 审计候选查询和通用 `/api/v1/audit`。

**验收标准：** 旧 route 返回 404 且不出现在 OpenAPI；仓库无旧 App/Web 消费代码；Activity 与通用 Audit Console 行为不退化。

---

## 最终验收清单

- [x] 创建任务后出现唯一 `created`；循环实例出现 `generated` 而非伪造 created。
- [x] start/stop/done/reopen/delete 使用稳定产品 action，不暴露内部 audit action。
- [x] 一次多字段修改只占一个 timeline 节点。
- [x] 当前注解只出现一次，删除后正文完全消失。
- [x] user actor 包含完整 `id/name/display_name/email/external_ids`；token/system/unknown 正确。
- [x] 历史无创建 audit 时只有 `snapshot:created`；不从快照伪造 completed。
- [x] SQLite 注解迁移回填 unknown 和 `created_at=entry`；PostgreSQL 实现与条件测试已完成，但当前环境未配置数据库，运行时 skip。
- [x] 同秒跨源排序固定，逐页无重复、无漏项，新事件插入不破坏下一页。
- [x] viewer 有 task read 即可读 Activity，但无 `audit:read` token scope 时仍不能读通用 Audit Console。
- [x] Activity 是 `<ol>/<li>` 单列纵向 timeline，小点和细线为装饰，无下一页时末尾不拖线。
- [x] composer、empty 不显示伪节点；loading 保持轨道布局；error 不阻塞详情其他区域。
- [x] 所有相关 mutation 成功后刷新 Activity。
- [x] 旧字段级任务审计 route、App 入口和 Web 兼容代码已删除；Activity 保留完整字段变化结构。
- [x] `go test ./...`、零 CGO test/build、Web typecheck/test/lint/build/smoke 与 `git diff --check` 全部通过。

## 推荐提交序列

```text
feat: 迁移任务注解活动元数据
feat: 记录任务注解原作者
feat: 增加任务活动游标查询
feat: 实现任务活动语义读模型
feat: 暴露任务活动接口
feat: 接入任务活动分页数据
feat: 重构任务活动纵向时间线
refactor: 删除退役任务字段历史接口
docs: 完成任务活动时间线交付
```
