# 内容引用与 Mention 事件 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成 `ref://user|task|attachment/{uuid}` 解析、用户/任务 suggestion 与批量 resolve、Tiptap 语义节点、`task.user_mentioned`、`mentioned_users` 通知受众及 Hook/自动化复用。

**Architecture:** `internal/task` 用 Goldmark AST 解析 Markdown 并只表达语法/集合差异，App 层批量解析实体、执行 workspace/request scope 权限和生成事件。Web 用一个 `XuanchuReference` atom extension 表达 user/task，渲染前通过 batch resolve 获取当前展示和 canonical URL；不可读目标退回保存 label。

**Tech Stack:** Go 1.25、Goldmark、GORM、Huma/OpenAPI、React 19、TypeScript 6、Tiptap 3.27.1、TanStack Query、Vitest、Playwright。

## Global Constraints

- 前置条件：通用附件 foundation 已完成；若富文本图片计划已执行，必须扩展其 `internal/task/content_reference.go` 和 Web `markdownExtensions`，不得新建第二套 parser/schema。
- 内部协议固定为 `ref://user/{uuid}`、`ref://task/{uuid}`、`ref://attachment/{uuid}`；scheme/host 小写，UUID canonical lowercase 且无 userinfo/port/query/fragment/额外 path。
- description 仍是 Markdown 字符串；CLI、MCP、Remote、HTTP、导入导出原样看到可读 Markdown。
- `name` 是用户稳定查找键，`display_name` 只用于展示；引用身份只由 URI UUID 决定，label 是保存时快照。
- 首期选择器只开放 user/task；project/series 等未知保留 host 在写操作中返回 `description_reference_invalid`。
- description 最大 512 KiB、单次最多 200 个内部引用；普通文本 `@Alice/#slug` 不自动改写。
- 新增/替换引用必须重新校验；调用者未改变的历史不可读引用允许保留并退回 label。
- 同一用户重复 mention、移动节点、改 label、同次删除再插入都不触发重复事件；先保存删除、后再次添加并保存会再次触发。
- 所有 mention payload、suggest user 和 recipient 必须使用完整 `task.UserInfo`/`task.JSONUserInfo`，不得使用裸 UUID。
- `mentioned_users` 只允许 `task.user_mentioned`；默认排除 actor 自己；Hook 和 automation 仍接收自我 mention 的完整事件。
- 不为 task reference 生成通知事件，不新增 mention 专用 dispatcher，不直连 IM/邮件。

---

## Shared Interfaces

```go
// internal/task/content_reference.go
type ContentReferenceKind string
const (
    ContentReferenceUser ContentReferenceKind = "user"
    ContentReferenceTask ContentReferenceKind = "task"
    ContentReferenceAttachment ContentReferenceKind = "attachment"
)
type ContentReference struct { Kind ContentReferenceKind; ID, Label string; Image bool }
type ContentReferenceKey struct { Kind ContentReferenceKind; ID string }
func ParseContentReferences(string) ([]ContentReference, error)
func DiffContentReferenceKeys(before, after []ContentReference) (added, removed []ContentReferenceKey)

// internal/app/content_reference.go
type ContentReferenceSuggestionInput struct { Type, Query, ProjectRef string; Limit int }
type ContentReferenceKeyInput struct { Type, ID string }
type ContentReferenceResolution struct {
    Type, ID, Status string
    User *task.UserInfo
    Task *TaskReferenceView
    Attachment *AttachmentView
}
func (s *Service) SuggestContentReferences(ContentReferenceSuggestionInput) ([]ContentReferenceSuggestion, error)
func (s *Service) ResolveContentReferences([]ContentReferenceKeyInput) ([]ContentReferenceResolution, error)
```

---

### Task 1: 完成保留 URI 的 Goldmark AST 语法与集合差异

**Files:**
- Modify: `internal/task/content_reference.go`
- Modify: `internal/task/content_reference_test.go`

**Interfaces:**
- Consumes: foundation 的 attachment-only parser scaffold。
- Produces: Shared Interfaces 中 domain 类型与函数，支持 user/task/attachment。

- [ ] **Step 1: 写失败表驱动测试**

```go
func TestParseContentReferences(t *testing.T) {
    source := "请 [@Alice](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001) 看 [#agentapi-17 · 补齐接口](ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84)\n\n![图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
    got, err := ParseContentReferences(source)
    if err != nil || len(got) != 3 { t.Fatalf("got=%#v err=%v", got, err) }
    if got[0].Kind != ContentReferenceUser || got[2].Kind != ContentReferenceAttachment || !got[2].Image { t.Fatalf("got=%#v", got) }
}

func TestParseContentReferencesRejectsMalformedReservedURI(t *testing.T) {
    cases := []string{
        "[@A](REF://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)",
        "[@A](ref://user/8C8B1BED-2E75-4DE8-8D5F-C94CBF2B3001)",
        "[@A](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001?q=1)",
        "[P](ref://project/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)",
    }
    for _, source := range cases { if _, err := ParseContentReferences(source); errorCode(err) != "description_reference_invalid" { t.Fatalf("%q err=%v", source, err) } }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task -run ContentReference -count=1`

Expected: FAIL，user/task 尚不被接受或 diff API 不存在。

- [ ] **Step 3: 实现 URI parser 和稳定集合 diff**

```go
func parseReferenceDestination(raw string) (ContentReferenceKind, string, bool, error) {
    u, err := url.Parse(raw)
    if err != nil || u.Scheme != "ref" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" { return "", "", false, invalidReference() }
    kind := ContentReferenceKind(u.Host)
    if kind != ContentReferenceUser && kind != ContentReferenceTask && kind != ContentReferenceAttachment { return "", "", false, invalidReference() }
    id := strings.TrimPrefix(u.EscapedPath(), "/")
    parsed, err := uuid.Parse(id)
    if err != nil || parsed.String() != id || strings.Contains(id, "%") { return "", "", false, invalidReference() }
    return kind, id, true, nil
}
```

用 Goldmark Link/Image AST destination，不能用正则解释 Markdown。diff 对 `(kind,id)` 去重并排序，label/image 不参与 identity；总引用计数保留重复节点数量，供 200 上限使用。

- [ ] **Step 4: 运行测试并提交**

Run: `go test ./internal/task -run ContentReference -count=1`

Expected: PASS，包括 escaped label、普通链接、代码块中的伪 URI、重复 mention 和 label change。

```bash
git add internal/task/content_reference.go internal/task/content_reference_test.go
git commit -m "feat: 解析正文内部语义引用"
```

### Task 2: 在所有 description 写路径执行增量权限校验

**Files:**
- Create: `internal/app/content_reference.go`
- Create: `internal/app/content_reference_test.go`
- Modify: `internal/app/description_attachment.go`
- Modify: `internal/app/service.go`
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/app/task_bundle.go`

**Interfaces:**
- Consumes: Task 1 parser、task/member/user repositories、attachment target handler。
- Produces: `validateDescriptionReferences(before, after task.Task) (DescriptionReferenceChange, error)`。

- [ ] **Step 1: 写失败权限和历史保留测试**

```go
func TestDescriptionReferenceValidationAllowsUnchangedUnavailableTarget(t *testing.T) {
    before := taskWithDescription(taskID, "[#旧任务](ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84)")
    next := before
    next.Description = ptr("补充文本\n\n" + *before.Description)
    svc.requestScope = scopeWithoutReferencedProject()
    if _, err := svc.validateDescriptionReferences(before, next); err != nil { t.Fatalf("unchanged ref rejected: %v", err) }
}

func TestDescriptionReferenceValidationRejectsNewUnreadableTaskAndCrossWorkspaceUser(t *testing.T) {
    // next 新增 scope 外 task、其它 workspace user，分别断言 description_reference_invalid。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run DescriptionReference -count=1`

Expected: FAIL，统一 validator 尚不存在。

- [ ] **Step 3: 实现变更对象和校验顺序**

```go
type DescriptionReferenceChange struct {
    Previous, Current []task.ContentReference
    Added, Removed []task.ContentReferenceKey
    AddedMentionedUserIDs, CurrentMentionedUserIDs []string
    CurrentAttachmentIDs []string
}
```

先检查 UTF-8 byte 长度 ≤ 512 KiB 和 AST reference nodes ≤ 200：前者返回 `description_too_large`，后者返回 `description_reference_limit_exceeded`。再 diff，只对 Added 做 target 校验：user 必须存在且是当前 workspace active member；task 必须是当前 workspace 实际任务、`resolveTargetForRead` 可读且非 projected occurrence；attachment 必须属于当前 task。然后激活 current attachment drafts。任何失败回滚 task/audit/event。

- [ ] **Step 4: 接入 Add/Modify/import/bundle**

Modify 必须保留 before；Add 使用空 before。Taskwarrior JSON、XLSX ordinary import 和 task bundle 同 workspace 导入都走 validator；跨 workspace attachment URI 返回 `description_reference_invalid`，不得猜测重映射 user/task。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/app -run 'DescriptionReference|Import.*Reference|Bundle.*Reference' -count=1`

Expected: PASS。

```bash
git add internal/app/content_reference.go internal/app/content_reference_test.go internal/app/description_attachment.go internal/app/service.go internal/httpapi/import_audit.go internal/app/task_bundle.go
git commit -m "feat: 校验描述语义引用权限"
```

### Task 3: 实现 suggestion 和 batch resolve App API

**Files:**
- Modify: `internal/app/content_reference.go`
- Modify: `internal/app/content_reference_test.go`
- Modify: `internal/storage/member_repo.go`
- Create: `internal/storage/member_repo_test.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/storage/task_repo_test.go`

**Interfaces:**
- Consumes: request scope、`resolveUserInfos`、task URL/slug view 和 `AttachmentView`。
- Produces: Shared Interfaces 中 Suggest/Resolve 方法及 typed view。

- [ ] **Step 1: 写失败 suggestion 排序和权限测试**

```go
func TestSuggestTaskReferencesPrefersCurrentProjectAndOmitsDescription(t *testing.T) {
    got, err := svc.SuggestContentReferences(ContentReferenceSuggestionInput{Type: "task", Query: "接口", ProjectRef: "agentapi", Limit: 20})
    if err != nil { t.Fatal(err) }
    if got[0].Task.Project.Slug != "agentapi" { t.Fatalf("order=%#v", got) }
    if strings.Contains(mustJSON(got), "description") { t.Fatal("suggestion leaked description") }
}

func TestResolveReferencesPreservesOrderAndHidesExistence(t *testing.T) {
    got, _ := svc.ResolveContentReferences([]ContentReferenceKeyInput{{Type:"user", ID:aliceID}, {Type:"task", ID:scopeDeniedID}, {Type:"attachment", ID:imageID}})
    if got[0].Status != "resolved" || got[1].Status != "unavailable" || got[2].Attachment == nil { t.Fatalf("got=%#v", got) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app ./internal/storage -run 'SuggestContent|ResolveContent|ReferenceSuggestion' -count=1`

Expected: FAIL，query API 尚不存在。

- [ ] **Step 3: 实现稳定 view 和 repository 查询**

```go
type TaskReferenceView struct {
    ID, Title, TaskSlug, Status, URL string
    Project ProjectReferenceView
}
type ContentReferenceSuggestion struct {
    Type string
    User *task.UserInfo
    Task *TaskReferenceView
}
```

user suggestion 要求 `PermissionMemberRead`，按 display_name/name/email case-insensitive 匹配 active member；task suggestion 要求 `PermissionTaskRead`，应用 workspace/project allowlist，只含 actual/materialized task，当前项目优先，其次 modified desc/title asc。query trim 至少 1 字符，limit 默认20/最大50。

- [ ] **Step 4: 实现 batch resolve**

请求最大 200；逐项独立权限判断，缺失/越权/缺少某类 capability 都返回 unavailable 而不是中止整个 batch。user 返回完整 UserInfo，task 返回 current title/canonical URL，attachment 通过 target handler read 校验并返回完整 AttachmentView。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/app ./internal/storage -run 'SuggestContent|ResolveContent|ReferenceSuggestion' -count=1`

Expected: PASS，包括跨 workspace、project allowlist、projected occurrence、inactive member、混合批次和稳定排序。

```bash
git add internal/app/content_reference.go internal/app/content_reference_test.go internal/storage/member_repo.go internal/storage/member_repo_test.go internal/storage/task_repo.go internal/storage/task_repo_test.go
git commit -m "feat: 查询和解析正文引用"
```

### Task 4: 暴露 suggestion/resolve HTTP 与 OpenAPI

**Files:**
- Create: `internal/httpapi/content_references.go`
- Create: `internal/httpapi/content_references_test.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes: Task 3 App API。
- Produces: `GET /api/v1/content-references/suggestions`、`POST /api/v1/content-references/resolve`。

- [ ] **Step 1: 写失败 HTTP 测试**

```go
func TestContentReferenceResolveDoesNotEnumerateDeniedTargets(t *testing.T) {
    body := fmt.Sprintf(`{"references":[{"type":"task","id":%q},{"type":"task","id":%q}]}`, missingID, deniedID)
    rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/content-references/resolve?workspace=local", body, auth(fixture.token))
    if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
    if strings.Count(rr.Body.String(), `"status":"unavailable"`) != 2 { t.Fatalf("body=%s", rr.Body.String()) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi -run ContentReference -count=1`

Expected: FAIL，routes 尚未注册。

- [ ] **Step 3: 实现 handler 和 DTO**

suggestion 根据 type 选择 `member:read` 或 `task:read` capability/permission；resolve 构造可用的最小 scoped service，并由每项解析决定 unavailable，不能因为其中一项缺权限返回整个请求 403。参数格式错误统一 `content_reference_query_invalid` 400；超过200项同码。

- [ ] **Step 4: 更新 OpenAPI schema**

OpenAPI 明确 one-of typed object、UserInfo 字段、AttachmentView、unavailable，不包含 description/storage backend/key；server route completeness test 加两条 path。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/httpapi -run 'ContentReference|OpenAPI' -count=1`

Expected: PASS。

```bash
git add internal/httpapi/content_references.go internal/httpapi/content_references_test.go internal/httpapi/huma_routes.go internal/httpapi/server_test.go
git commit -m "feat: 暴露内容引用查询接口"
```

### Task 5: 实现 `task.user_mentioned` 事件

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/task_change_events.go`
- Modify: `internal/app/task_change_events_test.go`
- Modify: `internal/app/hook_event.go`
- Modify: `internal/app/hook.go`
- Modify: `internal/app/hook_test.go`

**Interfaces:**
- Consumes: Task 2 `DescriptionReferenceChange` 和 `resolveUserInfos`。
- Produces: `buildUserMentionedEvent(...) HookEvent`、Hook 白名单 `task.user_mentioned`。

- [ ] **Step 1: 写失败语义测试**

```go
func TestUserMentionEventDiffSemantics(t *testing.T) {
    cases := []struct{name, before, after string; want int}{
        {"new", "", mention(aliceID, "Alice"), 1},
        {"duplicate", mention(aliceID,"Alice"), mention(aliceID,"Alice")+mention(aliceID,"A"), 0},
        {"label", mention(aliceID,"Alice"), mention(aliceID,"Alice Zhang"), 0},
        {"remove and readd same save", mention(aliceID,"Alice"), mention(aliceID,"Alice"), 0},
    }
    // 每个 case 修改并断言 task.user_mentioned delivery 数。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'UserMention|SemanticEventTypes' -count=1`

Expected: FAIL，事件尚不存在。

- [ ] **Step 3: 构造完整 payload 并加入 Modify closure**

```go
func buildUserMentionedEvent(tsk task.Task, added, current []task.UserInfo, runtime RuntimeContext, now int64) HookEvent {
    event := buildTaskHookEvent("task.user_mentioned", tsk, runtime, now)
    event.Data["source_field"] = "description"
    event.Data["mentioned_users"] = userInfosToJSONList(added)
    event.Data["current_mentioned_users"] = userInfosToJSONList(current)
    return event
}
```

新增事件与 `task.modified`、其它 fine-grained events 同 transaction/outbox；只有 added 非空时 append。删除保存后再次添加会再次生成新 EventID。

- [ ] **Step 4: 加 Hook 白名单和集成断言**

Hook payload 要包含 actor、自我 mention 和完整 UserInfo；`allSemanticEventTypes()` 加新事件。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/app -run 'UserMention|Hook.*Mention|SemanticEventTypes' -count=1`

Expected: PASS。

```bash
git add internal/app/service.go internal/app/task_change_events.go internal/app/task_change_events_test.go internal/app/hook_event.go internal/app/hook.go internal/app/hook_test.go
git commit -m "feat: 生成用户提及语义事件"
```

### Task 6: 增加 `mentioned_users` Notification audience

**Files:**
- Modify: `internal/app/event_notification.go`
- Modify: `internal/app/event_notification_test.go`
- Modify: `internal/httpapi/notifications_test.go`
- Modify: `internal/cli/notification.go`
- Modify: `internal/cli/notification_test.go`
- Modify: `internal/mcpserver/tools_notification.go`
- Modify: `internal/mcpserver/integration_test.go`

**Interfaces:**
- Consumes: `task.user_mentioned` HookEvent Data。
- Produces: notification rule `AudienceType="mentioned_users"`。

- [ ] **Step 1: 写失败 audience 验证和 recipient 测试**

```go
func TestMentionedUsersAudienceOnlySupportsMentionEvent(t *testing.T) {
    if err := validateEventNotificationAudience("task.modified", "mentioned_users"); errorCode(err) != "audience_unsupported_for_event" { t.Fatalf("err=%v", err) }
    if err := validateEventNotificationAudience("task.user_mentioned", "mentioned_users"); err != nil { t.Fatal(err) }
}

func TestMentionedUsersAudienceExcludesActorAndDedupes(t *testing.T) {
    event := mentionEvent(actorAlice, alice, bob, bob)
    ids, err := svc.eventNotificationRecipientIDs(rule("mentioned_users"), event)
    if err != nil || !reflect.DeepEqual(ids, []string{bob.ID}) { t.Fatalf("ids=%v err=%v", ids, err) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app ./internal/httpapi ./internal/cli ./internal/mcpserver -run MentionedUsers -count=1`

Expected: FAIL，audience 尚不支持。

- [ ] **Step 3: 实现 audience 解析**

只有 event type 精确等于 `task.user_mentioned` 才允许；从 `event.Data["mentioned_users"]` 解析 JSONUserInfo IDs，去重、过滤非 active member并排除 `event.ActorUserID`。actor 是 tenant/agent token 时 ActorUserID 为空，不排除。空 audience 返回空 delivery、写 debug 结构化日志，不阻断 task 修改。

- [ ] **Step 4: 更新 CLI/HTTP/MCP 枚举和 golden**

不新增专用命令，只让现有 notification rule add/modify 接受 `--audience mentioned_users`；OpenAPI/MCP input schema 枚举同步。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/app ./internal/httpapi ./internal/cli ./internal/mcpserver -run MentionedUsers -count=1`

Expected: PASS，dedupe key 仍为 workspace/rule/event/recipient。

```bash
git add internal/app/event_notification.go internal/app/event_notification_test.go internal/httpapi/notifications_test.go internal/cli/notification.go internal/cli/notification_test.go internal/mcpserver/tools_notification.go internal/mcpserver/integration_test.go
git commit -m "feat: 通知新增提及用户受众"
```

### Task 7: 让项目自动化消费 mention 上下文

**Files:**
- Modify: `internal/app/project_automation.go`
- Modify: `internal/app/project_automation_preview.go`
- Modify: `internal/app/project_automation_template_vars.go`
- Modify: `internal/app/project_automation_template_vars_test.go`
- Modify: `internal/app/project_automation_test.go`
- Modify: `internal/app/project_automation_scheduler.go`

**Interfaces:**
- Consumes: 新 Hook event 和 `event.Data["mentioned_users"]`。
- Produces: include/template var `mentioned_users`，event trigger 白名单自然复用 `allowedHookEventTypes`。

- [ ] **Step 1: 写失败 context/template 测试**

```go
func TestProjectAutomationMentionedUsersContext(t *testing.T) {
    event := mentionEvent(tokenActor, alice, bob)
    input := ProjectAutomationRuleAddInput{TriggerType: "event", TriggerConfig: ProjectAutomationTriggerConfig{EventType: "task.user_mentioned"}, Context: ProjectAutomationContextConfig{Include: []string{"mentioned_users"}}, InstructionTemplate: "通知 {{mentioned_users}}"}
    rendered := mustRenderAutomation(t, svc, input, &event)
    if !strings.Contains(rendered.Instruction, alice.ID) || !strings.Contains(rendered.Instruction, bob.DisplayName) { t.Fatalf("instruction=%s", rendered.Instruction) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'Automation.*Mentioned|TemplateVar' -count=1`

Expected: FAIL，变量尚不存在。

- [ ] **Step 3: 实现 include 和模板变量**

在 `buildProjectAutomationContext` 中，当 include 含 `mentioned_users` 时原样放入完整 JSON 数组；在 template vars 中总是对 mention event 提供 `{{mentioned_users}}` JSON。变量元数据描述为“新增提及用户完整 JSON 数组（task.user_mentioned）”。

- [ ] **Step 4: 运行测试并提交**

Run: `go test ./internal/app -run 'Automation.*Mentioned|TemplateVar' -count=1`

Expected: PASS；不新增 dispatcher 或通道逻辑。

```bash
git add internal/app/project_automation.go internal/app/project_automation_preview.go internal/app/project_automation_template_vars.go internal/app/project_automation_template_vars_test.go internal/app/project_automation_test.go internal/app/project_automation_scheduler.go
git commit -m "feat: 自动化支持提及用户上下文"
```

### Task 8: 实现 Web content-reference API 和 batch cache

**Files:**
- Create: `web/src/features/workspace/content-references/content-reference-api.ts`
- Create: `web/src/features/workspace/content-references/content-reference-api.test.ts`
- Create: `web/src/features/workspace/content-references/content-reference-cache.ts`
- Create: `web/src/features/workspace/content-references/content-reference-cache.test.ts`
- Create: `web/src/features/workspace/content-references/index.ts`
- Modify: `web/src/components/markdown/attachment-node-view.tsx`

**Interfaces:**
- Consumes: Task 4 HTTP API。
- Produces: suggest/resolve TS types、保持输入顺序的 batch loader、附件 node 批量 metadata。

- [ ] **Step 1: 写失败 API/cache 测试**

```ts
it("batches mixed references once and preserves unavailable", async () => {
  const loader = new ContentReferenceLoader(resolveReferences)
  const [user, task, image] = await Promise.all([
    loader.load({ type: "user", id: userID }),
    loader.load({ type: "task", id: taskID }),
    loader.load({ type: "attachment", id: attachmentID }),
  ])
  expect(resolveReferences).toHaveBeenCalledTimes(1)
  expect([user.status, task.status, image.status]).toEqual(["resolved", "unavailable", "resolved"])
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- content-reference-api content-reference-cache`

Expected: FAIL，模块尚不存在。

- [ ] **Step 3: 实现 API 和 microtask batch**

```ts
export type ContentReferenceKey = { type: "user" | "task" | "attachment"; id: string }
export type ContentReferenceResolution =
  | { type: ContentReferenceKey["type"]; id: string; status: "unavailable" }
  | { type: "user"; id: string; status: "resolved"; user: UserInfo }
  | { type: "task"; id: string; status: "resolved"; task: TaskReference }
  | { type: "attachment"; id: string; status: "resolved"; attachment: Attachment }
```

同一 microtask 内最多200项一批，按 type/id cache；workspace/session 变化清空。把附件 NodeView 的逐项 metadata GET 替换为此 loader，正文一次渲染不产生 N+1。

- [ ] **Step 4: 运行测试并提交**

Run: `pnpm --dir web test -- content-reference-api content-reference-cache attachment-node-view`

Expected: PASS。

```bash
git add web/src/features/workspace/content-references web/src/components/markdown/attachment-node-view.tsx
git commit -m "feat: 批量解析正文引用"
```

### Task 9: 实现 user/task Tiptap reference extension

**Files:**
- Create: `web/src/components/markdown/reference-extension.ts`
- Create: `web/src/components/markdown/reference-extension.test.ts`
- Create: `web/src/components/markdown/reference-node-view.tsx`
- Create: `web/src/components/markdown/reference-node-view.test.tsx`
- Modify: `web/src/components/markdown/extensions.ts`
- Modify: `web/src/components/markdown/markdown-safety.ts`
- Modify: `web/src/components/markdown/markdown-view.test.tsx`

**Interfaces:**
- Consumes: batch loader 和 canonical ref URI。
- Produces: `XuanchuReference` inline atom，attrs `{kind,id,label}`。

- [ ] **Step 1: 写失败 round-trip/不可用测试**

```ts
it.each([
  ["[@Alice](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)", "user"],
  ["[#agentapi-17 · 补齐接口](ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84)", "task"],
])("round trips references", (source, kind) => {
  const doc = parseMarkdownToJSON(source)
  expect(findNode(doc, "xuanchuReference").attrs.kind).toBe(kind)
  expect(markdownManager.serialize(doc).trim()).toBe(source)
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- reference-extension reference-node-view markdown-view`

Expected: FAIL，extension 尚不存在。

- [ ] **Step 3: 实现 node、当前展示与 fallback**

user resolved 显示当前 display_name 优先、name 辅助；task resolved 显示当前 slug/title 并链接 canonical relative URL；unavailable 显示保存 label、不可点击。node 是 atom，Backspace 一次删除。编辑和 static renderer 使用同一个 extension。

- [ ] **Step 4: 实现三种 clipboard 表达**

复制 reference node 时写内部 Markdown MIME、可读 text/plain 和带受控 `data-xuanchu-reference-*` 的 HTML；粘贴同应用内部格式保留 ID，外部 HTML 不信任 data attribute，仍经 sanitizer。

- [ ] **Step 5: 运行测试并提交**

Run: `pnpm --dir web test -- reference-extension reference-node-view markdown-view markdown-editor`

Expected: PASS。

```bash
git add web/src/components/markdown/reference-extension.ts web/src/components/markdown/reference-extension.test.ts web/src/components/markdown/reference-node-view.tsx web/src/components/markdown/reference-node-view.test.tsx web/src/components/markdown/extensions.ts web/src/components/markdown/markdown-safety.ts web/src/components/markdown/markdown-view.test.tsx
git commit -m "feat: 渲染用户和任务引用节点"
```

### Task 10: 实现 @/# suggestion 交互

**Files:**
- Create: `web/src/components/markdown/reference-suggestion.ts`
- Create: `web/src/components/markdown/reference-suggestion.test.ts`
- Create: `web/src/components/markdown/reference-suggestion-menu.tsx`
- Create: `web/src/components/markdown/reference-suggestion-menu.test.tsx`
- Modify: `web/src/components/markdown/markdown-editor.tsx`
- Modify: `web/src/components/markdown/markdown-editor.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: suggestion API、editor `workspaceSlug/taskRef/projectRef` context。
- Produces: 普通文本位置 @/# suggestion 和 atom insertion command。

- [ ] **Step 1: 写失败 keyboard/IME/abort 测试**

```tsx
it("debounces, aborts stale query and does not commit while composing", async () => {
  renderEditorWithReferences()
  fireEvent.compositionStart(editor)
  await user.type(editor, "@爱")
  expect(suggest).not.toHaveBeenCalled()
  fireEvent.compositionEnd(editor)
  await advanceTimersByTime(150)
  expect(suggest).toHaveBeenCalledWith(expect.objectContaining({ type: "user", q: "爱" }), expect.any(AbortSignal))
})

it("does not trigger inside code, codeBlock or link", async () => { /* 分别设置 selection/node 后输入 @/#，断言无请求 */ })
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- reference-suggestion markdown-editor`

Expected: FAIL，suggestion 尚未注册。

- [ ] **Step 3: 实现 trigger 和菜单**

空 query 不请求；150ms debounce；每次新 query abort 旧 request；支持上下键/Enter/Esc/鼠标，composing 阶段不提交。user label 保存当前 display_name 或 name，Markdown 为 `[@label](ref://user/id)`；task label 保存 `#task_slug · title`。

- [ ] **Step 4: 验证权限和 fallback UI**

member:read/task:read 缺失时菜单显示无权限且不暴露结果；projectRef 传当前 project 仅用于排序，不能改变后端 scope。

- [ ] **Step 5: 运行测试并提交**

Run: `pnpm --dir web test -- reference-suggestion reference-suggestion-menu markdown-editor`

Expected: PASS。

```bash
git add web/src/components/markdown/reference-suggestion.ts web/src/components/markdown/reference-suggestion.test.ts web/src/components/markdown/reference-suggestion-menu.tsx web/src/components/markdown/reference-suggestion-menu.test.tsx web/src/components/markdown/markdown-editor.tsx web/src/components/markdown/markdown-editor.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 支持选择用户和任务引用"
```

### Task 11: 补齐导出 warning 与跨协议兼容测试

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/task_bundle.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/httpapi/import_audit_test.go`
- Modify: `internal/remote/task.go`
- Modify: `internal/remote/task_test.go`
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `tests/integration/cli_test.go`

**Interfaces:**
- Consumes: parser 和现有 export envelope。
- Produces: `warnings[]` 中 attachment IDs 未包含二进制提示；所有协议 description 保留 Markdown。

- [ ] **Step 1: 写失败 round-trip 测试**

```go
func TestExportKeepsReferenceMarkdownAndWarnsAboutBinary(t *testing.T) {
    exported, err := svc.Export(ExportInput{})
    if err != nil { t.Fatal(err) }
    if got := exported.Tasks[0].Description; got == nil || *got != sourceMarkdown { t.Fatalf("description=%v", got) }
    if !slices.Contains(exported.Warnings[0].AttachmentIDs, attachmentID) { t.Fatalf("warnings=%#v", exported.Warnings) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app ./internal/httpapi ./internal/remote ./internal/mcpserver ./tests/integration -run 'ReferenceMarkdown|Export.*Binary' -count=1`

Expected: FAIL，warnings 尚不存在或跨入口断言未覆盖。

- [ ] **Step 3: 实现非阻断 warning**

warning 只列 unique/sorted attachment IDs 和“binary_not_included”；不改变 Taskwarrior JSON/XLSX/task-bundle v1 的 description 字段，不嵌入二进制。user/task ref 不产生 binary warning。

- [ ] **Step 4: 增加 CLI/HTTP/MCP/Remote 断言**

所有入口得到完全相同的 Markdown string，不出现 ProseMirror JSON/HTML/blob URL/预签名 URL；label 不作为 lookup key。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./internal/app ./internal/httpapi ./internal/remote ./internal/mcpserver ./tests/integration -run 'ReferenceMarkdown|Export.*Binary' -count=1`

Expected: PASS。

```bash
git add internal/app/service.go internal/app/task_bundle.go internal/app/service_test.go internal/httpapi/import_audit.go internal/httpapi/import_audit_test.go internal/remote/task.go internal/remote/task_test.go internal/mcpserver/integration_test.go tests/integration/cli_test.go
git commit -m "feat: 保留引用导出并提示附件未携带"
```

### Task 12: 同步通知、MCP、README、路线图和 smoke

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/notifications.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/skills/xuanchu-wire-up-automation/references/notification-tools.md`
- Modify: `docs/skills/xuanchu-mcp-base/SKILL.md`
- Modify: `web/scripts/playwright-editing-smoke.mjs`

**Interfaces:**
- Consumes: Tasks 1–11。
- Produces: v0.5.11 完整用户/Agent 文档与端到端回归。

- [ ] **Step 1: 更新中文文档**

必须写明四种 Markdown 示例、label 快照/UUID identity、@/# 权限边界、`task.user_mentioned` payload、`mentioned_users` 只支持该事件且排除 actor、Hook/automation 仍含自我 mention、普通文本不会自动变引用、附件二进制不随 export 携带。

- [ ] **Step 2: 更新 roadmap 状态**

只有四份计划均已完成并验证后，才把 v0.5.11 从“待实施”改为“已完成”；否则仅标记本子项完成并保留里程碑未完成。

- [ ] **Step 3: 增加 smoke**

覆盖 @ user 键盘选择、# task 鼠标选择、中文 IME、保存刷新、源码往返、不可用引用 fallback、duplicate mention 只产生一次 delivery、task reference 不产生通知、batch resolve 不是 N+1。

- [ ] **Step 4: 顺序运行完整验证矩阵**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
```

Expected: 全部退出 0；不要并行启动两套全量 E2E。

- [ ] **Step 5: 重复运行事件关键链路**

Run:

```bash
go test ./internal/app -run 'UserMention|MentionedUsers|Automation.*Mentioned' -count=20
go test ./internal/httpapi -run ContentReference -count=10
pnpm --dir web exec vitest run reference-suggestion content-reference-cache --pool=forks --maxWorkers=1
```

Expected: 全部 PASS，无重复 delivery、排序漂移、abort race 或偶发 500。

- [ ] **Step 6: 提交**

```bash
git add README.md ROADMAP.md docs/manual/notifications.md docs/manual/mcp.md docs/skills/xuanchu-wire-up-automation/references/notification-tools.md docs/skills/xuanchu-mcp-base/SKILL.md web/scripts/playwright-editing-smoke.mjs
git commit -m "docs: 完成内容引用与提及事件说明"
```
