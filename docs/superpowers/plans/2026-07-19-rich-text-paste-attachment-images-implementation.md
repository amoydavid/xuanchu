# 富文本粘贴与附件图片节点 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 Tiptap Markdown 编辑器中实现安全富文本粘贴、截图/拖拽/文件选择上传、公网图片服务端转存、`ref://attachment/{uuid}` 节点、draft 原子绑定和鉴权渲染。

**Architecture:** 浏览器先提取 `<img>` 候选，再用 DOMPurify 白名单清洗剩余 HTML；图片位置以异步占位节点保留，上传或远程转存成功后替换为附件节点。后端保存 description 时用共享 Goldmark parser 校验附件归属，并在 task modify/audit/events 的同一数据库 transaction 内激活本 actor 的 draft。

**Tech Stack:** React 19、TypeScript 6、Tiptap 3.27.1、DOMPurify、TanStack Query、Vitest、Testing Library、Playwright、Go 1.25、Goldmark。

## Global Constraints

- 前置条件：通用附件 foundation 已完成；本计划直接复用 foundation 的 `attachment-api.ts` 与 `AuthenticatedAttachmentImage`，不依赖 task 附件面板计划。
- description 的唯一持久化格式仍是 Markdown 字符串，不保存 HTML、ProseMirror JSON、data URL、blob URL、远程 `<img src>` 或预签名 URL。
- 持久图片语法固定为 `![label](ref://attachment/{canonical-lowercase-uuid})`，普通附件链接语法固定为 `[label](ref://attachment/{uuid})`。
- 粘贴 HTML 必须先提取图片再清洗；DOMPurify 完成后不得残留任何原始 `img`、style、class、id、`on*`、script、iframe/object/embed/form/input/video/audio。
- 剪贴板 File/data image 优先于同位置远程 URL；公网 URL 必须调用服务端 import-url，不允许浏览器直接加载或抓取原图。
- 单次粘贴远程 URL 按规范化 URL 去重，并发固定为 3；保留 query、移除 fragment，不能跨请求或跨 task 全局去重。
- 失败占位必须要求用户选择重试、移除或转为普通 HTTPS 链接；未解决失败和进行中上传时禁止保存。
- Tiptap 编辑和只读渲染继续共享 `markdownExtensions`，不得引入 react-markdown/remark 第二条 parser。
- description draft 绑定、task.modify audit 和 semantic events 必须在同一数据库 transaction 中完成。
- Web 验证必须包含 `pnpm --dir web run smoke:editing`。

---

### Task 1: 把 description 附件校验和 draft 激活纳入 Modify transaction

**Files:**
- Modify: `internal/task/content_reference.go`
- Modify: `internal/task/content_reference_test.go`
- Create: `internal/app/description_attachment.go`
- Create: `internal/app/description_attachment_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/audit.go`
- Modify: `internal/app/task_bundle.go`
- Modify: `internal/httpapi/import_audit.go`

**Interfaces:**
- Consumes: foundation 的 `AttachmentReferenceIDs`、`ActivateDescriptionDrafts` 内部能力和 task target handler。
- Produces: `validateAndBindDescriptionAttachments(before, after task.Task) error`，由所有 description 写路径复用。

- [ ] **Step 1: 写失败测试锁定同事务、归属和 actor**

```go
func TestModifyBindsDraftAndTaskChangeAtomically(t *testing.T) {
    draft := mustUploadDraft(t, svc, taskID)
    next := fmt.Sprintf("![图](ref://attachment/%s)", draft.ID)
    if err := svc.Modify(taskID, ModifyInput{Description: &next}); err != nil { t.Fatal(err) }
    got, _ := svc.GetAttachment(draft.ID)
    if got.State != "active" { t.Fatalf("state = %q", got.State) }
    assertTaskModifyAuditContains(t, svc, taskID, next)
}

func TestModifyRollsBackTaskWhenDraftActivationFails(t *testing.T) {
    foreignDraft := mustUploadDraft(t, otherActorSvc, taskID)
    next := fmt.Sprintf("![图](ref://attachment/%s)", foreignDraft.ID)
    err := svc.Modify(taskID, ModifyInput{Description: &next})
    assertRuntimeCode(t, err, "attachment_draft_creator_mismatch")
    if got := mustTask(t, svc, taskID).Description; got != nil { t.Fatalf("description changed: %v", got) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task ./internal/app -run 'DescriptionAttachment|ModifyBindsDraft' -count=1`

Expected: FAIL，Modify 尚未绑定 draft。

- [ ] **Step 3: 实现统一 validator**

```go
func (s *Service) validateAndBindDescriptionAttachments(before, after task.Task) error {
    nextIDs, err := task.AttachmentReferenceIDs(optionalTextValue(after.Description))
    if err != nil { return err }
    for _, id := range nextIDs {
        row, err := s.attachmentRepo.GetByID(id)
        if err != nil || row.WorkspaceID != s.workspaceID || row.AttachedToType != "task" || row.AttachedToID != after.UUID {
            return RuntimeError{Code: "description_reference_invalid", Message: "attachment reference is unavailable"}
        }
    }
    return s.activateDescriptionDraftsLocked(after.UUID, nextIDs)
}
```

在 `Modify` 的 `withAuditAndEvents` closure 中，`modifyLocked` 后、构造 audit/event 前调用；Add/import/task bundle 的 description 写入也调用同一校验。description 超过 512 KiB 时返回 `description_too_large`。历史无保留 URI无需迁移。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/task ./internal/app -run 'DescriptionAttachment|ModifyBindsDraft|Import.*Attachment' -count=1`

Expected: PASS，包括跨 workspace/跨 task/不存在/已 purge/普通链接和 source mode 手写合法 URI。

- [ ] **Step 5: 提交**

```bash
git add internal/task/content_reference.go internal/task/content_reference_test.go internal/app/description_attachment.go internal/app/description_attachment_test.go internal/app/service.go internal/app/audit.go internal/app/task_bundle.go internal/httpapi/import_audit.go
git commit -m "feat: 原子绑定描述中的附件草稿"
```

### Task 2: 安装 DOMPurify 并实现 HTML 白名单清洗

**Files:**
- Modify: `web/package.json`
- Modify: `web/pnpm-lock.yaml`
- Create: `web/src/components/markdown/paste-sanitizer.ts`
- Create: `web/src/components/markdown/paste-sanitizer.test.ts`

**Interfaces:**
- Consumes: Clipboard HTML/string 和可选 clipboard image files。
- Produces: `sanitizeRichPaste(input): SanitizedPaste`，包含安全 HTML 与按 DOM 顺序的图片候选。

- [ ] **Step 1: 安装精确依赖**

Run: `pnpm --dir web add --save-exact dompurify`

Expected: `web/package.json` 中 `dompurify` 没有 `^`/`~`。

- [ ] **Step 2: 写失败测试覆盖白名单和图片提取优先级**

```ts
it("keeps markdown schema html and removes active content", () => {
  const result = sanitizeRichPaste({
    html: `<h1 style="color:red">标题</h1><script>x()</script><table><tr><td onclick="x()">A</td></tr></table>`,
    files: [],
  })
  expect(result.html).toContain("<h1>标题</h1>")
  expect(result.html).toContain("<table>")
  expect(result.html).not.toMatch(/style|script|onclick/)
})

it("replaces images with controlled markers and prefers clipboard files", () => {
  const result = sanitizeRichPaste({ html: `<p>A<img src="https://cdn/x.png" alt="X">B</p>`, files: [pngFile] })
  expect(result.images[0].kind).toBe("file")
  expect(result.html).toContain(`data-xuanchu-paste-image="${result.images[0].key}"`)
  expect(result.html).not.toContain("<img")
})
```

- [ ] **Step 3: 运行测试确认失败**

Run: `pnpm --dir web test -- paste-sanitizer`

Expected: FAIL，模块尚不存在。

- [ ] **Step 4: 实现 DOM 预处理和 DOMPurify 配置**

```ts
export type PasteImageCandidate = {
  key: string
  kind: "file" | "data" | "remote"
  alt: string
  file?: File
  sourceURL?: string
}

const ALLOWED_TAGS = ["p", "br", "h1", "h2", "h3", "strong", "b", "em", "i", "s", "del", "ul", "ol", "li", "blockquote", "pre", "code", "table", "thead", "tbody", "tr", "th", "td", "a"]
const ALLOWED_ATTR = ["href", "title", "colspan", "rowspan", "data-xuanchu-paste-image"]
```

候选 URL 顺序 `src` → `data-src/data-original` → `srcset` 最大 descriptor；remote URL 规范化为 scheme/host 小写、移除 fragment、保留 path/query。每个 `<img>` 替换为允许的 `<a data-xuanchu-paste-image=...>alt</a>`，清洗后再次断言 DOM 中没有 `img/script/style`。

- [ ] **Step 5: 运行测试并提交**

Run: `pnpm --dir web test -- paste-sanitizer`

Expected: PASS。

```bash
git add web/package.json web/pnpm-lock.yaml web/src/components/markdown/paste-sanitizer.ts web/src/components/markdown/paste-sanitizer.test.ts
git commit -m "feat: 清洗粘贴富文本"
```

### Task 3: 实现附件 Tiptap extension 的 Markdown 往返

**Files:**
- Create: `web/src/components/markdown/attachment-extension.ts`
- Create: `web/src/components/markdown/attachment-extension.test.ts`
- Modify: `web/src/components/markdown/extensions.ts`
- Modify: `web/src/components/markdown/markdown-safety.ts`
- Modify: `web/src/components/markdown/markdown-safety.test.ts`

**Interfaces:**
- Consumes: 标准 Markdown image/link 与 canonical `ref://attachment` URI。
- Produces: `XuanchuAttachment` atom node，attrs 固定为 `{id,label,image,state}`。

- [ ] **Step 1: 写失败 round-trip 测试**

```ts
it.each([
  ["![架构图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)", true],
  ["[需求.pdf](ref://attachment/a801f977-c745-4f47-95a4-7893a9317aba)", false],
])("round trips attachment markdown", (source, image) => {
  const json = parseMarkdownToJSON(source)
  expect(findNode(json, "xuanchuAttachment").attrs.image).toBe(image)
  expect(markdownManager.serialize(json).trim()).toBe(source)
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- attachment-extension markdown-safety`

Expected: FAIL，ref scheme 当前被 Link protocol 拒绝且 node 不存在。

- [ ] **Step 3: 实现严格 node 和 parser/serializer**

```ts
export type XuanchuAttachmentAttrs = {
  id: string
  label: string
  image: boolean
  state: "resolved" | "loading" | "failed" | "gone"
}
```

extension 只把 canonical attachment UUID 的 link/image 转为 atom；普通 http(s)/mailto 仍由 Link 处理；任何格式非法的 `ref://` 在 parse helper 中返回结构化错误，不降级为普通链接。把 extension 加入唯一的 `markdownExtensions`。

- [ ] **Step 4: 运行测试并提交**

Run: `pnpm --dir web test -- attachment-extension markdown-safety markdown-view markdown-editor`

Expected: PASS，普通 Markdown 回归不变。

```bash
git add web/src/components/markdown/attachment-extension.ts web/src/components/markdown/attachment-extension.test.ts web/src/components/markdown/extensions.ts web/src/components/markdown/markdown-safety.ts web/src/components/markdown/markdown-safety.test.ts
git commit -m "feat: 增加附件 Markdown 节点"
```

### Task 4: 实现附件 NodeView、鉴权渲染和 purge 降级

**Files:**
- Create: `web/src/components/markdown/attachment-node-view.tsx`
- Create: `web/src/components/markdown/attachment-node-view.test.tsx`
- Modify: `web/src/components/markdown/attachment-extension.ts`
- Modify: `web/src/components/markdown/markdown-view.tsx`
- Modify: `web/src/components/markdown/markdown-view.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-change-history.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`

**Interfaces:**
- Consumes: `AuthenticatedAttachmentImage`、attachment metadata API。
- Produces: 富文本/只读共用的图片、文件卡片、loading/failed/gone NodeView。

- [ ] **Step 1: 写失败渲染测试**

```tsx
it("loads protected image through authenticated fetch", async () => {
  renderMarkdown("![架构](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)")
  const img = await screen.findByRole("img", { name: "架构" })
  expect(img).toHaveAttribute("src", "blob:test")
  expect(img).not.toHaveAttribute("src", expect.stringContaining("/api/v1/attachments"))
})

it("renders purged history without crashing", async () => {
  metadataMock.rejects(new ApiError(410, "attachment_content_gone", "gone"))
  renderHistory("![旧图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)")
  expect(await screen.findByText("附件内容已清理")).toBeInTheDocument()
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- attachment-node-view markdown-view task-detail-page`

Expected: FAIL，NodeView 尚未注册。

- [ ] **Step 3: 实现 node view**

图片 metadata resolved 后交给 `AuthenticatedAttachmentImage`；文件卡显示 display name/media type/size/下载；404/unavailable 显示保存 label 且不可点击，410 显示“附件内容已清理”。本阶段按 attachment ID 使用 query cache 去重 metadata；第四份计划将解析入口切换到 `/content-references/resolve` 批量 API。

- [ ] **Step 4: 运行测试并提交**

Run: `pnpm --dir web test -- attachment-node-view markdown-view task-detail-page`

Expected: PASS。

```bash
git add web/src/components/markdown/attachment-node-view.tsx web/src/components/markdown/attachment-node-view.test.tsx web/src/components/markdown/attachment-extension.ts web/src/components/markdown/markdown-view.tsx web/src/components/markdown/markdown-view.test.tsx web/src/features/workspace/project-workbench/task-detail/task-change-history.tsx web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx
git commit -m "feat: 渲染鉴权附件节点"
```

### Task 5: 实现上传队列和远程抓取去重

**Files:**
- Create: `web/src/components/markdown/attachment-upload-queue.ts`
- Create: `web/src/components/markdown/attachment-upload-queue.test.ts`
- Modify: `web/src/features/workspace/attachments/attachment-api.ts`
- Modify: `web/src/features/workspace/attachments/attachment-api.test.ts`

**Interfaces:**
- Consumes: task upload/import-url API。
- Produces: `AttachmentUploadQueue`，逐 item 进度、取消、重试、删除 draft 和远程并发 3。

- [ ] **Step 1: 写失败队列测试**

```ts
it("deduplicates repeated remote URLs within one paste and limits concurrency to three", async () => {
  const queue = new AttachmentUploadQueue(api, { remoteConcurrency: 3 })
  const items = [urlA, urlA, urlB, urlC, urlD].map(remoteCandidate)
  const promises = items.map((item) => queue.enqueue(item))
  expect(api.importURL).toHaveBeenCalledTimes(4)
  expect(maxObservedConcurrency).toBe(3)
  await Promise.all(promises)
  expect((await promises[0]).attachment.id).toBe((await promises[1]).attachment.id)
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- attachment-upload-queue attachment-api`

Expected: FAIL，queue/import URL API 尚未完成。

- [ ] **Step 3: 实现队列状态机**

```ts
export type UploadQueueItem = {
  key: string
  status: "queued" | "uploading" | "resolved" | "failed" | "cancelled"
  progress: number
  attachment?: Attachment
  error?: ApiError
}
```

remote promise map 的 key 使用规范化完整 URL；file/data 不跨位置去重。每项持有 AbortController；`cancelAll` 取消未完成请求，`cleanupDrafts` 对已成功但未保存 draft 调用 DELETE；一个失败不取消其他项。

- [ ] **Step 4: 运行测试并提交**

Run: `pnpm --dir web test -- attachment-upload-queue attachment-api`

Expected: PASS。

```bash
git add web/src/components/markdown/attachment-upload-queue.ts web/src/components/markdown/attachment-upload-queue.test.ts web/src/features/workspace/attachments/attachment-api.ts web/src/features/workspace/attachments/attachment-api.test.ts
git commit -m "feat: 管理描述图片上传队列"
```

### Task 6: 接管 paste/drop/file picker 并处理失败占位

**Files:**
- Modify: `web/src/components/markdown/markdown-editor.tsx`
- Modify: `web/src/components/markdown/markdown-editor.test.tsx`
- Create: `web/src/components/markdown/attachment-node-commands.ts`
- Create: `web/src/components/markdown/attachment-node-commands.test.ts`
- Modify: `web/src/components/markdown/markdown.css`

**Interfaces:**
- Consumes: sanitizer、extension、queue。
- Produces: `MarkdownEditorProps.attachmentContext` 与 `onPendingStateChange`。

- [ ] **Step 1: 写失败编辑器测试**

```tsx
it("pastes rich text and replaces remote image in place", async () => {
  renderEditorWithAttachments()
  pasteHTML(editor, `<h2>标题</h2><p>A<img src="https://cdn/x.png" alt="图">B</p>`)
  expect(await screen.findByText("标题")).toBeInTheDocument()
  resolveImportURL(attachment)
  await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(expect.stringContaining(`![图](ref://attachment/${attachment.id})`)))
})

it("blocks save while a failed image is unresolved", async () => {
  rejectImportURL("attachment_remote_fetch_failed")
  expect(await screen.findByRole("button", { name: "重试" })).toBeInTheDocument()
  expect(onPendingStateChange).toHaveBeenLastCalledWith({ blocking: true })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- markdown-editor attachment-node-commands`

Expected: FAIL，editor 尚未接管 paste/drop。

- [ ] **Step 3: 扩展 editor props 和 ProseMirror handlers**

```ts
type MarkdownAttachmentContext = { workspaceSlug: string; taskRef: string }
type MarkdownEditorPendingState = { blocking: boolean; pending: number; failed: number; draftIDs: string[] }
```

`handlePaste` 优先处理 `text/html`，只有 plain text 时沿用原逻辑；`handleDrop` 和工具栏图片按钮只接受 PNG/JPEG/GIF/WebP。每个 marker 先替换为 async placeholder，成功后同位置替换为 `XuanchuAttachment`，失败保留三动作：重试、移除、转普通 HTTPS 链接。普通外链动作仅对原 URL 是 HTTPS 时可用，并序列化为普通 `[alt](https://...)` link，不生成 `<img>`。

- [ ] **Step 4: 实现内部复制格式**

选区包含附件节点时，clipboard 同时写 `text/plain`、`text/html` 和 `application/x-xuanchu-markdown`；同一璇础编辑器优先读取内部 Markdown MIME，从而保留 attachment ID；外部应用至少得到可读 alt/文件名。

- [ ] **Step 5: 运行测试并提交**

Run: `pnpm --dir web test -- markdown-editor attachment-node-commands paste-sanitizer`

Expected: PASS，包括 IME 不受影响、代码/链接内普通粘贴、data/blob 不进入 onChange。

```bash
git add web/src/components/markdown/markdown-editor.tsx web/src/components/markdown/markdown-editor.test.tsx web/src/components/markdown/attachment-node-commands.ts web/src/components/markdown/attachment-node-commands.test.ts web/src/components/markdown/markdown.css
git commit -m "feat: 支持粘贴和上传描述图片"
```

### Task 7: 在任务描述 dialog 管理保存、取消和源码错误

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
- Modify: `web/src/components/markdown/markdown-editor.tsx`
- Modify: `web/src/components/markdown/markdown-editor.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: `MarkdownEditorPendingState` 和 task modify API。
- Produces: 保存激活、取消 cleanup、关闭中断请求、source parse error UI。

- [ ] **Step 1: 写失败 dialog 测试**

```tsx
it("disables save until uploads settle and cleans drafts on cancel", async () => {
  renderTaskDetail()
  await openDescriptionEditor()
  pasteImage(file)
  expect(screen.getByRole("button", { name: "保存" })).toBeDisabled()
  resolveUpload(draftAttachment)
  expect(screen.getByRole("button", { name: "保存" })).toBeEnabled()
  await user.click(screen.getByRole("button", { name: "取消" }))
  expect(removeAttachment).toHaveBeenCalledWith(draftAttachment.id)
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- task-detail-page markdown-editor`

Expected: FAIL，dialog 尚不知道 pending drafts。

- [ ] **Step 3: 实现保存/取消边界**

保存只提交 Markdown，成功后清空“本 dialog 创建的 draft”集合（服务端已原子激活）；失败保留 dialog/drafts 以便重试。取消/关闭/unmount 调用 `cancelAll` 后 best-effort 删除本 dialog draft；删除失败不阻塞关闭，由 24h janitor 兜底。

- [ ] **Step 4: 实现源码模式严格回切**

从 source 切回 rich text 时调用 `parseMarkdownToJSON`；非法/非 canonical `ref://` 显示行内错误并保持 source mode，保存按钮 disabled；不得用 normalize 静默删掉 URI。

- [ ] **Step 5: 运行测试并提交**

Run: `pnpm --dir web test -- task-detail-page markdown-editor`

Expected: PASS，注解 editor 没有 `attachmentContext` 时维持原行为。

```bash
git add web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx web/src/components/markdown/markdown-editor.tsx web/src/components/markdown/markdown-editor.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 完成描述图片保存与取消流程"
```

### Task 8: 更新 CSP 和浏览器安全回归

**Files:**
- Modify: `internal/webconsole/console.go`
- Modify: `internal/webconsole/console_test.go`
- Modify: `web/src/components/markdown/markdown-view.test.tsx`

**Interfaces:**
- Consumes: blob URL/data 本地预览语义。
- Produces: `img-src 'self' blob: data:`，其它 CSP 指令不放宽。

- [ ] **Step 1: 写失败 CSP 测试**

```go
func TestConsoleCSPAllowsOnlyLocalAttachmentImageSources(t *testing.T) {
    csp := consoleResponse(t).Header().Get("Content-Security-Policy")
    if !strings.Contains(csp, "img-src 'self' blob: data:") { t.Fatalf("CSP=%q", csp) }
    if strings.Contains(csp, "img-src *") || strings.Contains(csp, "https:") { t.Fatalf("CSP too broad: %q", csp) }
}
```

- [ ] **Step 2: 运行测试确认失败并实现**

Run: `go test ./internal/webconsole -run ConsoleCSP -count=1`

Expected: 修改前 FAIL，修改后 PASS。

- [ ] **Step 3: 验证持久化输出无 data/blob/remote image**

Run: `pnpm --dir web test -- markdown-view markdown-editor paste-sanitizer`

Expected: PASS，测试明确断言 onChange 不含 `data:`、`blob:` 或 `![...](https://...)` 的远程图片语法。

- [ ] **Step 4: 提交**

```bash
git add internal/webconsole/console.go internal/webconsole/console_test.go web/src/components/markdown/markdown-view.test.tsx
git commit -m "fix: 收紧附件图片浏览器策略"
```

### Task 9: 完成编辑 smoke 和文档同步

**Files:**
- Modify: `web/scripts/playwright-editing-smoke.mjs`
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-07-19-task-description-rich-content-attachments-mentions-design.md`（仅同步真实实现差异）

**Interfaces:**
- Consumes: Tasks 1–8。
- Produces: 富文本/图片用户流程回归和实际行为文档。

- [ ] **Step 1: 增加 smoke 场景**

顺序覆盖：粘贴 Word 风格标题/列表/表格/链接并保存刷新；粘贴截图；粘贴重复公网 URL 只 import 一次；远程失败三种动作；拖拽和文件选择；保存后 Markdown 仅含 `ref://attachment`；取消后 draft 不在 active 列表；PAT/OIDC/acting 三种身份鉴权显示。

- [ ] **Step 2: 更新 README/ROADMAP**

README 明确“粘贴公网图片会尽可能由服务端转存；登录态/私网/失败图片不会静默直连”，并列出支持结构和图片类型。ROADMAP 只把富文本/图片子项标为完成；mention 仍待第四份计划。

- [ ] **Step 3: 顺序运行全量验证**

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

- [ ] **Step 4: 提交**

```bash
git add web/scripts/playwright-editing-smoke.mjs README.md ROADMAP.md docs/superpowers/specs/2026-07-19-task-description-rich-content-attachments-mentions-design.md
git commit -m "docs: 完成富文本粘贴和图片说明"
```

spec 无真实偏差时不要为了提交而改写 spec。
