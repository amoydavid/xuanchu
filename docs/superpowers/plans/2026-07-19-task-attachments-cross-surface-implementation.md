# Task 附件跨入口 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在通用附件 foundation 上完成 task 附件的 CLI、Remote、MCP、Web Console 面板、鉴权下载、审计和运维文档闭环。

**Architecture:** Local CLI 直接调用 `app.Service`，Remote CLI 使用同一 HTTP API 且流式传输，MCP 只暴露 metadata 工具。Web 通过统一 workspace credential fetch 获取附件内容，并在任务详情中提供列表、上传、重命名、下载和移除，不复制后端权限判断。

**Tech Stack:** Go 1.25、Cobra、HTTP multipart、MCP Go SDK、React 19、TanStack Query、TypeScript 6、Vitest、Testing Library、Tiptap 3.27.1。

## Global Constraints

- 前置条件：`2026-07-19-generic-attachments-storage-implementation.md` 已完整执行并通过验证；不得在本计划重新定义 `AttachmentView`、`AttachmentRuntime`、`blobstore.Store` 或 target resolver。
- CLI stdout 只放结果；进度和诊断写 stderr；`--json` 只用于 metadata/list/info/write result，不与二进制 stdout 混用。
- Remote 上传和下载必须流式处理，不得把二进制放进 JSON/base64，也不得 `io.ReadAll` 整个文件。
- MCP tool 名固定为 `task_attachment_list|get|rename|remove`，不得新增 MCP base64 upload/download tool。
- Web 的 PAT、acting token 和 OIDC cookie 必须复用同一 credential 选择逻辑；受保护图片不能直接使用裸 `<img src=content_url>`。
- `created_by` 必须输出完整 `task.ActorInfo`，用户内层必须是完整 `task.UserInfo`。
- completed/deleted task、archived/cancelled project、viewer、token scope 和 allowlist 的最终裁决仍在 App 层；前端只隐藏明显不可用入口。
- SQLite、PostgreSQL 与 `CGO_ENABLED=0` 基线不得退化。

---

## Prerequisite Interfaces

本计划只消费第一份计划定义的签名：

```go
func (s *Service) UploadAttachment(context.Context, string, string, AttachmentUploadInput) (AttachmentView, error)
func (s *Service) ListAttachments(string, string, bool) ([]AttachmentView, error)
func (s *Service) GetAttachment(string) (AttachmentView, error)
func (s *Service) OpenAttachmentContent(context.Context, string) (AttachmentContent, error)
func (s *Service) RenameAttachment(string, string) (AttachmentView, error)
func (s *Service) RemoveAttachment(context.Context, string) error
```

---

### Task 1: 为 Local CLI 装配附件 runtime

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`
- Create: `internal/cli/attachment_runtime.go`
- Create: `internal/cli/attachment_runtime_test.go`

**Interfaces:**
- Consumes: `config.Config.Attachments`、`app.NewAttachmentRuntime`、`app.ServiceOptions.Attachments`。
- Produces: `buildAttachmentRuntime(ctx, cfg, logger)` 和带 runtime cleanup 的本地 service 构造。

- [ ] **Step 1: 写失败测试锁定本地 CLI 默认目录和启动失败**

```go
func TestBuildServiceFromOptsInjectsAttachmentRuntime(t *testing.T) {
    opts := Options{DataDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard}
    svc, closeFn, err := buildServiceFromOpts(opts)
    if err != nil { t.Fatal(err) }
    defer closeFn()
    if svc.AttachmentRuntime() == nil { t.Fatal("attachment runtime missing") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cli -run AttachmentRuntime -count=1`

Expected: FAIL，service 尚未注入 attachment runtime。

- [ ] **Step 3: 实现一次构造、一次关闭**

```go
func buildAttachmentRuntime(ctx context.Context, cfg config.Config, logger *logging.Logger) (*app.AttachmentRuntime, error) {
    return app.NewAttachmentRuntime(ctx, cfg.Attachments, logger)
}
```

`buildServiceFromOpts` 在 `storage.Open` 后构造 runtime 并传给 `ServiceOptions.Attachments`；任何后续失败都关闭 store/logger。不要为每个子命令重复创建 S3 client。

本地 CLI 不启动常驻 janitor；每次附件写命令完成后调用 `CleanupAttachments(ctx, 100)` opportunistic 清理一批，清理失败只写 stderr/结构化日志，不把已经成功的用户写操作改成失败。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/cli -run AttachmentRuntime -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/cli/root.go internal/cli/root_test.go internal/cli/attachment_runtime.go internal/cli/attachment_runtime_test.go
git commit -m "feat: 为本地命令装配附件存储"
```

### Task 2: 实现 Remote Client 流式附件协议

**Files:**
- Modify: `internal/remote/client.go`
- Create: `internal/remote/attachment.go`
- Create: `internal/remote/attachment_test.go`

**Interfaces:**
- Consumes: 第一份计划的 HTTP API。
- Produces: `remote.AttachmentDTO` 和 Upload/List/Get/Download/Rename/Remove 方法。

- [ ] **Step 1: 写失败测试验证 multipart、Bearer 和字节一致**

```go
func TestUploadAndDownloadAttachmentStreamsBytes(t *testing.T) {
    srv := newAttachmentFixtureServer(t)
    client, _ := NewClient(Options{BaseURL: srv.URL, Token: "token", HTTPClient: srv.Client()})
    got, err := client.UploadTaskAttachment(ctx, "local", "task-1", UploadAttachmentInput{
        Reader: strings.NewReader("payload"), Size: 7, FileName: "需求.txt", Mode: "attachment",
    })
    if err != nil || got.DisplayName != "需求.txt" { t.Fatalf("got=%#v err=%v", got, err) }
    var out bytes.Buffer
    _, err = client.DownloadAttachment(ctx, "local", got.ID, &out)
    if err != nil || out.String() != "payload" { t.Fatalf("out=%q err=%v", out.String(), err) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/remote -run Attachment -count=1`

Expected: FAIL，remote attachment API 尚不存在。

- [ ] **Step 3: 实现流式方法**

```go
type UploadAttachmentInput struct { Reader io.Reader; Size int64; FileName, DisplayName, Mode string; Progress func(int64) }
type DownloadAttachmentResult struct { DisplayName, MediaType, SHA256 string; SizeBytes int64 }

func (c *Client) UploadTaskAttachment(ctx context.Context, workspace, taskRef string, in UploadAttachmentInput) (AttachmentDTO, error)
func (c *Client) ListTaskAttachments(ctx context.Context, workspace, taskRef string) ([]AttachmentDTO, error)
func (c *Client) GetAttachment(ctx context.Context, workspace, id string) (AttachmentDTO, error)
func (c *Client) DownloadAttachment(ctx context.Context, workspace, id string, dst io.Writer) (DownloadAttachmentResult, error)
func (c *Client) RenameAttachment(ctx context.Context, workspace, id, name string) (AttachmentDTO, error)
func (c *Client) RemoveAttachment(ctx context.Context, workspace, id string) error
```

上传使用 `io.Pipe` + `multipart.Writer`，writer goroutine 的错误必须传播并关闭 pipe；`doStream` 复用 Authorization/X-Xuanchu-As，下载先检查 status/error envelope，再 `io.Copy` 到 dst。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/remote -run Attachment -count=1`

Expected: PASS，慢 reader/提前断流/401/413/503 都能返回稳定错误且不死锁。

- [ ] **Step 5: 提交**

```bash
git add internal/remote/client.go internal/remote/attachment.go internal/remote/attachment_test.go
git commit -m "feat: 增加远程附件流式客户端"
```

### Task 3: 增加 attachment CLI 命令组

**Files:**
- Create: `internal/cli/attachment.go`
- Create: `internal/cli/attachment_test.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

**Interfaces:**
- Consumes: Local App API 与 Task 2 Remote Client。
- Produces: `xuanchu attachment add|list|info|download|rename|remove`。

```text
xuanchu attachment add <task-ref> <file> [--display-name <name>]
xuanchu attachment list <task-ref>
xuanchu attachment info <attachment-id>
xuanchu attachment download <attachment-id> [--output <path>]
xuanchu attachment rename <attachment-id> <display-name>
xuanchu attachment remove <attachment-id>
```

- [ ] **Step 1: 写失败命令测试**

```go
func TestAttachmentDownloadStdoutDoesNotMixProgress(t *testing.T) {
    var stdout, stderr bytes.Buffer
    root := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr, Server: server.URL, Token: "token"})
    err := Execute(root, Options{}, []string{"attachment", "download", attachmentID, "--output", "-"})
    if err != nil { t.Fatal(err) }
    if stdout.String() != "payload" { t.Fatalf("stdout=%q", stdout.String()) }
    if strings.Contains(stdout.String(), "Downloading") { t.Fatal("progress leaked to stdout") }
}

func TestAttachmentDownloadRejectsJSONAndBinaryStdout(t *testing.T) {
    err := Execute(root, Options{}, []string{"--json", "attachment", "download", attachmentID, "--output", "-"})
    assertRuntimeCode(t, err, "attachment_binary_json_conflict")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cli ./tests/integration -run Attachment -count=1`

Expected: FAIL，命令尚不存在。

- [ ] **Step 3: 实现命令和安全输出文件行为**

```go
func newAttachmentCommand(opts Options) *cobra.Command
func newAttachmentAddCommand(opts Options) *cobra.Command
func newAttachmentDownloadCommand(opts Options) *cobra.Command
```

`add` 用 `os.Open` + `Stat.Size`；本地/远程都传 reader。`download` 默认使用服务端 `display_name` 的安全 basename，在当前目录以 `O_CREATE|O_EXCL|O_WRONLY` 创建，已存在则返回 `attachment_output_exists`；`--output -` 才写 stdout。失败时删除本次创建的部分文件，但不得删除调用前已存在文件。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/cli ./tests/integration -run Attachment -count=1`

Expected: PASS，local/remote 字节一致，human/JSON 输出稳定，stderr/stdout 分离。

- [ ] **Step 5: 提交**

```bash
git add internal/cli/attachment.go internal/cli/attachment_test.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 增加附件命令行管理"
```

### Task 4: 暴露四个 MCP metadata 工具

**Files:**
- Create: `internal/mcpserver/tools_attachment.go`
- Create: `internal/mcpserver/tools_attachment_test.go`
- Modify: `internal/mcpserver/options.go`
- Modify: `internal/mcpserver/auth.go`
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `internal/cli/mcp.go`
- Modify: `internal/httpapi/router.go`

**Interfaces:**
- Consumes: `mcpserver.Options.Attachments *app.AttachmentRuntime`，RuntimeFactory 构造 scoped Service 时透传。
- Produces: `task_attachment_list|get|rename|remove`。

- [ ] **Step 1: 写失败 schema/structuredContent 测试**

```go
func TestTaskAttachmentToolsAreMetadataOnly(t *testing.T) {
    tools := listToolNames(t, session)
    for _, name := range []string{"task_attachment_list", "task_attachment_get", "task_attachment_rename", "task_attachment_remove"} {
        if !slices.Contains(tools, name) { t.Fatalf("missing %s", name) }
    }
    for _, forbidden := range []string{"task_attachment_upload", "task_attachment_download"} {
        if slices.Contains(tools, forbidden) { t.Fatalf("forbidden %s", forbidden) }
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/mcpserver -run TaskAttachment -count=1`

Expected: FAIL，tools 尚未注册。

- [ ] **Step 3: 实现输入和 handler**

```go
type TaskAttachmentListInput struct { RequestScopeInput; Task string `json:"task"` }
type TaskAttachmentGetInput struct { RequestScopeInput; AttachmentID string `json:"attachment_id"` }
type TaskAttachmentRenameInput struct { RequestScopeInput; AttachmentID, DisplayName string }
type TaskAttachmentRemoveInput struct { RequestScopeInput; AttachmentID string `json:"attachment_id"` }
```

list/get 使用 `task:read`，rename/remove 使用 `task:write`；get 的 rendered 文案明确说明使用同一 Bearer token 请求 `content_url`。`AttachmentView` 序列化必须含完整 `created_by`，不得含 storage backend/key。

- [ ] **Step 4: 注入 runtime 并运行测试**

Run: `go test ./internal/mcpserver ./internal/cli ./internal/httpapi -run 'TaskAttachment|MCP' -count=1`

Expected: PASS，stdio/HTTP MCP 走各自既有 auth 路径，acting token 边界不变。

- [ ] **Step 5: 提交**

```bash
git add internal/mcpserver/tools_attachment.go internal/mcpserver/tools_attachment_test.go internal/mcpserver/options.go internal/mcpserver/auth.go internal/mcpserver/server.go internal/mcpserver/integration_test.go internal/cli/mcp.go internal/httpapi/router.go
git commit -m "feat: 增加任务附件 MCP 工具"
```

### Task 5: 验证附件面板所需的 Web 传输契约

**Files:**
- Modify: `web/src/features/workspace/session/workspace-api.test.ts`
- Modify: `web/src/features/workspace/attachments/attachment-api.ts`
- Modify: `web/src/features/workspace/attachments/attachment-api.test.ts`

**Interfaces:**
- Consumes: `getAdminActingToken/getWorkspaceToken` 与 OIDC cookie fallback。
- Produces: 面板需要的多文件进度、独立失败和下载错误契约；底层 DTO/API 已由 foundation 创建。

- [ ] **Step 1: 写失败测试锁定 PAT/acting/OIDC 三种凭证**

```ts
it.each([
  ["workspace", "pat-token"],
  ["acting", "act-token"],
  ["oidc", null],
])("downloads blob with %s credentials", async (_mode, token) => {
  const blob = await workspaceApiBlob("/api/v1/attachments/a/content")
  expect(await blob.text()).toBe("payload")
  expect(lastRequest.headers.get("Authorization")).toBe(token ? `Bearer ${token}` : null)
  expect(lastRequest.credentials).toBe("same-origin")
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- workspace-api attachment-api`

Expected: foundation 完成后基础用例 PASS；新增的多文件进度/独立取消断言先 FAIL。

- [ ] **Step 3: 扩充而不复制 foundation helper**

```ts
export function workspaceApiBlob(path: string, signal?: AbortSignal): Promise<Blob>
export function workspaceApiMultipart<T>(path: string, body: FormData, options?: { signal?: AbortSignal; onProgress?: (sent: number, total: number) => void }): Promise<T>
```

保持 JSON/blob/multipart 共用 token 选择和 unauthorized cleanup；为多文件面板补充逐 request progress/AbortController 测试。不得新建第二个 multipart helper 或 Attachment DTO。

- [ ] **Step 4: 验证 foundation 附件 API 形状**

```ts
export type Attachment = {
  id: string
  attached_to: { type: "task"; id: string }
  state: "draft" | "active" | "deleted"
  original_name: string
  display_name: string
  media_type: string
  size_bytes: number
  sha256: string
  inline_capable: boolean
  source_type: "upload" | "remote_url"
  content_url: string
  created_by: ActorInfo
  created_at: number
  modified_at: number
}
```

确认 `listTaskAttachments/uploadTaskAttachment/getAttachmentBlob/renameAttachment/removeAttachment` 保持上述形状，并为 409/413/415/503 error envelope 添加前端断言。

- [ ] **Step 5: 运行测试并提交**

Run: `pnpm --dir web test -- workspace-api attachment-api`

Expected: PASS。

```bash
git add web/src/features/workspace/session/workspace-api.test.ts web/src/features/workspace/attachments/attachment-api.ts web/src/features/workspace/attachments/attachment-api.test.ts
git commit -m "test: 完善 Web 附件面板传输契约"
```

### Task 6: 验证附件面板的鉴权图片缓存

**Files:**
- Modify: `web/src/features/workspace/attachments/authenticated-attachment-image.tsx`
- Modify: `web/src/features/workspace/attachments/authenticated-attachment-image.test.tsx`
- Modify: `web/src/features/workspace/attachments/attachment-blob-cache.ts`
- Modify: `web/src/features/workspace/attachments/attachment-blob-cache.test.ts`

**Interfaces:**
- Consumes: `workspaceApiBlob` 和 `Attachment`。
- Produces: 面板缩略图场景的 loading/error/alt/尺寸约束；共享组件已由 foundation 创建。

- [ ] **Step 1: 写失败测试锁定去重和 revoke**

```tsx
it("deduplicates by id and sha256 and revokes after last consumer", async () => {
  const first = render(<AuthenticatedAttachmentImage attachment={attachment} alt="图" />)
  const second = render(<AuthenticatedAttachmentImage attachment={attachment} alt="图" />)
  await screen.findAllByAltText("图")
  expect(fetchBlob).toHaveBeenCalledTimes(1)
  first.unmount()
  expect(URL.revokeObjectURL).not.toHaveBeenCalled()
  second.unmount()
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:test")
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- authenticated-attachment-image attachment-blob-cache`

Expected: foundation 基础用例 PASS；新增的面板 loading/error 断言先 FAIL。

- [ ] **Step 3: 扩充共享组件的面板状态**

```ts
type CacheEntry = { promise: Promise<string>; refs: number; objectURL?: string }
export function acquireAttachmentBlob(attachment: Attachment, signal?: AbortSignal): Promise<{ url: string; release: () => void }>
```

保持 key `${attachment.id}:${attachment.sha256}`、最后 consumer revoke 和 session change 清理；新增 skeleton、可读 alt、加载失败重试按钮和固定 thumbnail box，不另建图片 fetch 路径。

- [ ] **Step 4: 运行测试并提交**

Run: `pnpm --dir web test -- authenticated-attachment-image attachment-blob-cache`

Expected: PASS，原生 `<img>` 的 src 只出现 blob URL，不出现受保护 content URL。

```bash
git add web/src/features/workspace/attachments/authenticated-attachment-image.tsx web/src/features/workspace/attachments/authenticated-attachment-image.test.tsx web/src/features/workspace/attachments/attachment-blob-cache.ts web/src/features/workspace/attachments/attachment-blob-cache.test.ts
git commit -m "feat: 完善附件面板鉴权缩略图"
```

### Task 7: 实现任务附件面板和 Query hooks

**Files:**
- Create: `web/src/features/workspace/attachments/use-task-attachments.ts`
- Create: `web/src/features/workspace/attachments/task-attachment-panel.tsx`
- Create: `web/src/features/workspace/attachments/task-attachment-panel.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: Tasks 5–6 API/组件和 `taskWritable/pageCanWrite`。
- Produces: `TaskAttachmentPanel({workspaceSlug, taskRef, canWrite})`。

- [ ] **Step 1: 写失败交互测试**

```tsx
it("shows active attachments and keeps other uploads running after one failure", async () => {
  renderPanel({ canWrite: true })
  await user.upload(screen.getByLabelText("添加附件"), [goodFile, rejectedFile])
  expect(await screen.findByText("good.pdf")).toBeInTheDocument()
  expect(await screen.findByText("不允许上传此文件类型")).toBeInTheDocument()
  expect(uploadTaskAttachment).toHaveBeenCalledTimes(2)
})

it("hides mutations for read-only and displays attachment_in_use", async () => {
  renderPanel({ canWrite: false })
  expect(screen.queryByRole("button", { name: "移除" })).not.toBeInTheDocument()
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- task-attachment-panel task-detail-page`

Expected: FAIL，面板尚不存在。

- [ ] **Step 3: 实现列表、队列、重命名、下载和删除**

面板默认展示 active 数量和最近 3 个，展开后完整列表；图片用 `AuthenticatedAttachmentImage`，其他文件显示类型/大小/创建者/时间。每个 file 单独 mutation/state；删除 409 `attachment_in_use` 映射为“正文仍在引用，请先删除引用并保存”。下载通过 blob + 临时 `<a download>`，完成后 revoke 临时 URL。

- [ ] **Step 4: 接入任务详情权限和移动端布局**

```tsx
<TaskAttachmentPanel
  canWrite={taskWritable}
  taskRef={taskData.uuid}
  workspaceSlug={workspaceSlug}
/>
```

closed project、completed/deleted task 不显示写入口，但列表与下载仍可用。

- [ ] **Step 5: 运行测试并提交**

Run:

```bash
pnpm --dir web test -- task-attachment-panel task-detail-page
pnpm --dir web typecheck
```

Expected: PASS。

```bash
git add web/src/features/workspace/attachments web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 增加任务附件面板"
```

### Task 8: 完善附件审计

**Files:**
- Modify: `internal/app/attachment.go`
- Modify: `internal/app/attachment_test.go`
- Modify: `internal/app/audit.go`

**Interfaces:**
- Consumes: 现有 audit transaction、`AttachmentView` 与 `AuthenticatedAttachmentImage`。
- Produces: `attachment.add|rename|remove` audit payload；历史 description 的附件节点降级由富文本/图片计划的共享 renderer 完成。

- [ ] **Step 1: 写失败审计测试**

```go
func TestAttachmentAuditOmitsStorageAndSourceURL(t *testing.T) {
    view, _ := svc.ImportAttachmentURL(ctx, "task", taskID, AttachmentImportURLInput{SourceURL: "https://cdn.example/a.png?token=secret", Mode: "description_draft"})
    rows, _ := svc.ListAudit(AuditListInput{TargetType: strPtr("attachment"), TargetID: &view.ID})
    payload := rows[0].PayloadJSON
    for _, secret := range []string{"token=secret", "storage_key", "filesystem", "s3 endpoint"} {
        if strings.Contains(payload, secret) { t.Fatalf("audit leaked %q: %s", secret, payload) }
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run AttachmentAudit -count=1`

Expected: FAIL，audit payload 尚未完整锁定。

- [ ] **Step 3: 实现审计 payload**

每个写操作在业务 transaction 内写 audit；`TargetType="attachment"`，payload 只含 attachment ID、`attached_to.type/id`、display name、media type、size、SHA-256、source type/host 和 actor。不得保存完整 source URL、storage key、bucket、endpoint 或文件内容。

- [ ] **Step 4: 运行测试并提交**

Run: `go test ./internal/app -run AttachmentAudit -count=1`

Expected: PASS。

```bash
git add internal/app/attachment.go internal/app/attachment_test.go internal/app/audit.go
git commit -m "feat: 记录附件资源审计"
```

### Task 9: 同步用户、MCP、部署和备份文档

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/remote-cli-and-api.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/manual/deployment.md`
- Modify: `docs/deployment.md`
- Create: `docs/manual/attachment-security.md`
- Modify: `docs/skills/xuanchu-mcp-base/SKILL.md`

**Interfaces:**
- Consumes: 已实现的 CLI/MCP/Web/S3 行为。
- Produces: 可直接操作的中文文档，不描述未实现的 project/series 附件入口。

- [ ] **Step 1: 写文档契约测试/扫描**

Run: `rg -n 'attachment add|task_attachment_list|attachments\.backend|AWS credential|备份' README.md docs/manual docs/skills/xuanchu-mcp-base/SKILL.md`

Expected: 当前至少一类关键字缺失，证明文档尚未同步。

- [ ] **Step 2: 更新文档为实际命令和配置**

必须包含：六个 CLI 命令、四个 MCP metadata tool、local/remote 流式说明、filesystem volume/权限/备份恢复、S3/MinIO private bucket 与默认 credential chain、切换 backend 不自动迁移、数据库与 blob 必须一致备份、允许/拒绝文件类型和配额、首期只有 task target。新安全文档必须明确 remote fetch egress 开关、禁止网段、DNS 全结果校验和连接 IP pinning、逐跳 redirect 重验、HTTPS 降级拒绝、timeout/并发/大小限制、无 Cookie/Authorization/代理以及失败占位降级。

- [ ] **Step 3: 更新路线图状态但不提前标记整个 v0.5.11 完成**

`ROADMAP.md` 只把“通用附件 foundation/task 跨入口”标为已实现；富文本图片和 mention 子项仍保持待实施，直到第三/四份计划完成。

- [ ] **Step 4: 验证链接和提交**

Run: `git diff --check && rg -n 'task_attachment_(list|get|rename|remove)' docs/manual/mcp.md docs/skills/xuanchu-mcp-base/SKILL.md`

Expected: 退出 0，文档中无 MCP upload/download 工具。

```bash
git add README.md ROADMAP.md docs/manual/remote-cli-and-api.md docs/manual/mcp.md docs/manual/deployment.md docs/deployment.md docs/manual/attachment-security.md docs/skills/xuanchu-mcp-base/SKILL.md
git commit -m "docs: 补充任务附件使用和部署说明"
```

### Task 10: 跨入口全量验收

**Files:**
- Modify: `web/scripts/playwright-editing-smoke.mjs`

**Interfaces:**
- Consumes: Tasks 1–9。
- Produces: task 附件跨入口可回归交付物。

- [ ] **Step 1: 增加 Web smoke 场景**

在 `playwright-editing-smoke.mjs` 增加：上传 PNG/PDF、刷新后列表仍在、图片鉴权显示、重命名、下载字节一致、删除、viewer/closed project 不显示写入口。fixture API 必须返回真实 envelope，不绕过 workspace credential helper。

- [ ] **Step 2: 顺序运行 Web 验证**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

Expected: 全部 PASS。

- [ ] **Step 3: 顺序运行 Go 和工作树验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
```

Expected: 全部退出 0；不要同时启动另一套全量 E2E。

- [ ] **Step 4: S3 opt-in contract**

Run: `XUANCHU_TEST_S3_ENDPOINT="$MINIO_ENDPOINT" XUANCHU_TEST_S3_BUCKET="$MINIO_BUCKET" go test ./internal/app ./internal/httpapi -run AttachmentS3Contract -count=1`

Expected: 未配置时 SKIP；配置本地 MinIO/CI service container 时与 filesystem contract 同样 PASS。

- [ ] **Step 5: 提交 smoke 修正**

```bash
git add web/scripts/playwright-editing-smoke.mjs
git commit -m "test: 覆盖任务附件跨入口流程"
```

如果该文件无变化，不创建空提交。
