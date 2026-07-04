# Web Console Tiptap Markdown Editor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Web Console 任务详情页引入 Tiptap v3 Markdown WYSIWYG 编辑与同 schema 只读渲染，并修正详情页面包屑和主区布局。

**Architecture:** 后端保持零改动，仍保存普通字符串；前端新增 `web/src/components/markdown/` 作为通用 Markdown 能力层，先接入任务描述和任务注解。编辑器、只读渲染和安全归一化共享同一套 Tiptap v3 extensions，避免双 parser 和 HTML 透传风险。

**Tech Stack:** React 19, TypeScript, Vite 8, Vitest, Testing Library, Playwright smoke, Tiptap v3.27.1, shadcn/Radix UI, Tailwind CSS v4, lucide-react, Go 1.25.

**Status:** 已完成（2026-07-04）。

**Execution note:** 本次按计划完成实现与验证，但没有按每个 task 拆分中间 commit；下方 Commit 步骤保留为后续整理提交时的建议切分，不代表本轮已经创建这些提交。

**Verified commands:**

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
git diff --check
```

**Delivered notes:**

- `MarkdownEditor` / `MarkdownView` 已落在 `web/src/components/markdown/`，共享 Tiptap v3.27.1 schema。
- 任务 description 和 annotation 已接入 Markdown WYSIWYG 编辑与只读渲染；后端仍保存字符串。
- 原始 HTML 在 parser 前转义，危险链接协议不渲染为可点击链接。
- 任务详情页已修正面包屑和 `md:grid-cols-[minmax(0,1fr)_280px]` 主区布局。
- Playwright `smoke:editing` 已覆盖 Markdown 渲染、编辑保存、安全链接和移动端 tab 交互。
- 验证中发现并修复了两个既有阻断项：SSO 页面 hooks lint 问题，以及 server 在未配置 `config_secret_key` 时错误阻断普通启动的问题。

---

## File Structure

- Create: `web/src/components/markdown/markdown-safety.ts`
  - Markdown 输入安全归一化；转义原始 HTML 标签；链接协议白名单。
- Create: `web/src/components/markdown/markdown-safety.test.ts`
  - 覆盖 HTML 转义和链接协议。
- Create: `web/src/components/markdown/extensions.ts`
  - Tiptap v3 extensions、`MarkdownManager`、parse helper。
- Create: `web/src/components/markdown/markdown-view.tsx`
  - 只读 Markdown 渲染。
- Create: `web/src/components/markdown/markdown-view.test.tsx`
  - 覆盖常见 Markdown、安全渲染和非法链接。
- Create: `web/src/components/markdown/markdown-editor.tsx`
  - 受控 WYSIWYG Markdown 编辑器和工具栏。
- Create: `web/src/components/markdown/markdown-editor.test.tsx`
  - 覆盖初始值、onChange、工具栏和快捷保存。
- Create: `web/src/components/markdown/markdown.css`
  - 编辑态与只读态共享的最小排版样式。
- Create: `web/src/components/markdown/index.ts`
  - 对外导出 `MarkdownEditor` / `MarkdownView`。
- Modify: `web/package.json`
  - 新增 Tiptap v3.27.1 依赖。
- Modify: `web/pnpm-lock.yaml`
  - 由 `pnpm --dir web add ...` 更新。
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
  - 任务描述接入 Markdown 组件；面包屑和栅格调整。
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
  - 更新描述、面包屑、布局断言。
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.tsx`
  - 注解新增/编辑/展示接入 Markdown 组件。
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.test.tsx`
  - 更新注解 Markdown 行为断言。
- Modify: `web/scripts/playwright-editing-smoke.mjs`
  - 增加 Markdown 描述、注解和安全 smoke。
- Modify: `ROADMAP.md`
  - 标记 v0.5.1 规划项。

---

## Chunk 1: Tiptap 依赖与 Markdown 安全层

### Task 1: 锁定 Tiptap v3.27.1 依赖

**Files:**
- Modify: `web/package.json`
- Modify: `web/pnpm-lock.yaml`

- [x] **Step 1: 安装依赖**

Run:

```bash
pnpm --dir web add --save-exact @tiptap/react@3.27.1 @tiptap/starter-kit@3.27.1 @tiptap/pm@3.27.1 @tiptap/markdown@3.27.1 @tiptap/static-renderer@3.27.1 @tiptap/extension-link@3.27.1 @tiptap/extension-list@3.27.1 @tiptap/extension-table@3.27.1
```

Expected: `web/package.json` 和 `web/pnpm-lock.yaml` 更新；所有 Tiptap 包版本均为 `3.27.1`。

- [x] **Step 2: 快速确认版本**

Run:

```bash
rg -n '"@tiptap/' web/package.json
```

Expected: 输出的 Tiptap dependency 全部固定为 `3.27.1`，不要使用 `^3.27.1`。

- [x] **Step 3: 记录建议提交切分（未执行 commit）**

```bash
git add web/package.json web/pnpm-lock.yaml
git commit -m "chore: 添加 Tiptap Markdown 依赖"
```

### Task 2: 实现 Markdown 安全归一化

**Files:**
- Create: `web/src/components/markdown/markdown-safety.ts`
- Create: `web/src/components/markdown/markdown-safety.test.ts`

- [x] **Step 1: 写失败测试**

Create `web/src/components/markdown/markdown-safety.test.ts`:

```ts
import { describe, expect, it } from "vitest"

import { escapeMarkdownHtml, isAllowedMarkdownHref } from "./markdown-safety"

describe("markdown safety", () => {
  it("escapes raw html tags outside code", () => {
    expect(escapeMarkdownHtml("<script>alert(1)</script>")).toBe(
      "&lt;script&gt;alert(1)&lt;/script&gt;"
    )
    expect(escapeMarkdownHtml('<img src=x onerror="alert(1)">')).toBe(
      '&lt;img src=x onerror="alert(1)"&gt;'
    )
  })

  it("keeps inline code and fenced code untouched", () => {
    expect(escapeMarkdownHtml("`<script>`")).toBe("`<script>`")
    expect(escapeMarkdownHtml("```html\n<script>\n```")).toBe(
      "```html\n<script>\n```"
    )
  })

  it("does not rewrite normal markdown links", () => {
    expect(escapeMarkdownHtml("[官网](https://example.com?a=<b>)")).toBe(
      "[官网](https://example.com?a=<b>)"
    )
  })

  it("allows only explicit safe protocols", () => {
    expect(isAllowedMarkdownHref("https://example.com")).toBe(true)
    expect(isAllowedMarkdownHref("http://example.com")).toBe(true)
    expect(isAllowedMarkdownHref("mailto:ops@example.com")).toBe(true)
    expect(isAllowedMarkdownHref("javascript:alert(1)")).toBe(false)
    expect(isAllowedMarkdownHref("data:text/html,evil")).toBe(false)
    expect(isAllowedMarkdownHref("/relative")).toBe(false)
  })
})
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- markdown-safety
```

Expected: FAIL，模块尚不存在。

- [x] **Step 3: 实现安全函数**

Create `web/src/components/markdown/markdown-safety.ts`:

```ts
const RAW_HTML_TAG_RE = /<\/?[A-Za-z][A-Za-z0-9:-]*(?:\s[^<>]*)?\s*\/?>/g

export function isAllowedMarkdownHref(url: string): boolean {
  return /^(https?:\/\/|mailto:)/i.test(url.trim())
}

export function escapeMarkdownHtml(source: string): string {
  if (!source) {
    return ""
  }

  return source
    .split(/(```[\s\S]*?```|`[^`\n]*`|\[[^\]\n]+\]\([^\)\n]*\))/g)
    .map((part) => {
      if (
        part.startsWith("```") ||
        part.startsWith("`") ||
        /^\[[^\]\n]+\]\([^\)\n]*\)$/.test(part)
      ) {
        return part
      }
      return part.replace(RAW_HTML_TAG_RE, (tag) =>
        tag.replaceAll("<", "&lt;").replaceAll(">", "&gt;")
      )
    })
    .join("")
}
```

Note: 这是首版保守规则，目标是阻断标准 HTML 标签进入 Tiptap parser。后续如要支持 HTML 源码块、自动链接 `<https://...>` 等高级 Markdown 语法，应另开 spec。

- [x] **Step 4: 运行测试**

Run:

```bash
pnpm --dir web test -- markdown-safety
```

Expected: PASS.

- [x] **Step 5: 记录建议提交切分（未执行 commit）**

```bash
git add web/src/components/markdown/markdown-safety.ts web/src/components/markdown/markdown-safety.test.ts
git commit -m "feat: 添加 Markdown 安全归一化"
```

---

## Chunk 2: Markdown schema、只读渲染与编辑器

### Task 3: 建立 Tiptap extensions 和 parse helper

**Files:**
- Create: `web/src/components/markdown/extensions.ts`
- Test: `web/src/components/markdown/markdown-safety.test.ts`

- [x] **Step 1: 扩展测试覆盖 parse helper**

Append to `markdown-safety.test.ts` or create a focused test if preferred:

```ts
import { parseMarkdownToJSON } from "./extensions"

it("parses markdown through the shared manager", () => {
  const json = parseMarkdownToJSON("# 标题\n\n- [x] 完成")
  expect(json.type).toBe("doc")
  expect(JSON.stringify(json)).toContain("标题")
})
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- markdown-safety
```

Expected: FAIL，`extensions.ts` 尚不存在。

- [x] **Step 3: 实现 extensions**

Create `web/src/components/markdown/extensions.ts`:

```ts
import Link from "@tiptap/extension-link"
import { TaskItem, TaskList } from "@tiptap/extension-list"
import { Table, TableCell, TableHeader, TableRow } from "@tiptap/extension-table"
import { Markdown, MarkdownManager } from "@tiptap/markdown"
import StarterKit from "@tiptap/starter-kit"

import { escapeMarkdownHtml, isAllowedMarkdownHref } from "./markdown-safety"

export const markdownExtension = Markdown.configure({
  markedOptions: {
    gfm: true,
    breaks: false,
  },
})

export const markdownExtensions = [
  StarterKit.configure({
    link: false,
  }),
  markdownExtension,
  Link.configure({
    openOnClick: false,
    autolink: true,
    linkOnPaste: true,
    protocols: ["http", "https", "mailto"],
    validate: isAllowedMarkdownHref,
    HTMLAttributes: {
      rel: "noopener noreferrer",
      target: "_blank",
    },
  }),
  TaskList,
  TaskItem.configure({
    nested: true,
  }),
  Table.configure({
    resizable: false,
  }),
  TableRow,
  TableHeader,
  TableCell,
]

export const markdownManager = new MarkdownManager({
  extensions: markdownExtensions,
  markedOptions: {
    gfm: true,
    breaks: false,
  },
})

export function normalizeMarkdownSource(source: string): string {
  return escapeMarkdownHtml(source)
}

export function parseMarkdownToJSON(source: string) {
  return markdownManager.parse(normalizeMarkdownSource(source))
}
```

- [x] **Step 4: 运行测试和类型检查**

Run:

```bash
pnpm --dir web test -- markdown-safety
pnpm --dir web typecheck
```

Expected: PASS。若 Tiptap 类型要求调整 import 形式，以实际 v3.27.1 类型为准，但不得退回 Tiptap v2 或社区 `tiptap-markdown`。

- [x] **Step 5: 记录建议提交切分（未执行 commit）**

```bash
git add web/src/components/markdown/extensions.ts web/src/components/markdown/markdown-safety.test.ts
git commit -m "feat: 配置 Tiptap Markdown schema"
```

### Task 4: 实现 MarkdownView

**Files:**
- Create: `web/src/components/markdown/markdown-view.tsx`
- Create: `web/src/components/markdown/markdown-view.test.tsx`
- Create: `web/src/components/markdown/markdown.css`
- Create: `web/src/components/markdown/index.ts`
- Modify: `web/src/main.tsx` or nearest global CSS entry if needed

- [x] **Step 1: 写失败测试**

Create `web/src/components/markdown/markdown-view.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { MarkdownView } from "./markdown-view"

describe("MarkdownView", () => {
  it("renders common markdown nodes", () => {
    render(
      <MarkdownView>{`# 标题\n\n- 列表\n\n> 引用\n\n\`\`\`ts\nconst a = 1\n\`\`\``}</MarkdownView>
    )

    expect(screen.getByRole("heading", { name: "标题" })).toBeTruthy()
    expect(screen.getByText("列表")).toBeTruthy()
    expect(screen.getByText("引用")).toBeTruthy()
    expect(screen.getByText("const a = 1")).toBeTruthy()
  })

  it("renders raw html as text", () => {
    const { container } = render(
      <MarkdownView>{"<script>alert(1)</script>"}</MarkdownView>
    )

    expect(container.querySelector("script")).toBeNull()
    expect(screen.getByText("<script>alert(1)</script>")).toBeTruthy()
  })

  it("does not render javascript links as clickable hrefs", () => {
    const { container } = render(
      <MarkdownView>{"[bad](javascript:alert(1)) [good](https://example.com)"}</MarkdownView>
    )

    expect(container.querySelector('a[href^="javascript:"]')).toBeNull()
    expect(container.querySelector('a[href="https://example.com"]')).toBeTruthy()
  })
})
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- markdown-view
```

Expected: FAIL，组件尚不存在。

- [x] **Step 3: 实现 MarkdownView**

Create `web/src/components/markdown/markdown-view.tsx`:

```tsx
import { renderToReactElement } from "@tiptap/static-renderer/pm/react"

import { cn } from "@/lib/utils"

import { markdownExtensions, parseMarkdownToJSON } from "./extensions"
import "./markdown.css"

type MarkdownViewProps = {
  children: string
  className?: string
}

export function MarkdownView({ children, className }: MarkdownViewProps) {
  if (!children.trim()) {
    return null
  }

  const content = parseMarkdownToJSON(children)
  return (
    <div className={cn("markdown-prose", className)}>
      {renderToReactElement({
        extensions: markdownExtensions,
        content,
      })}
    </div>
  )
}
```

Create `web/src/components/markdown/index.ts`:

```ts
export { MarkdownView } from "./markdown-view"
```

Create `web/src/components/markdown/markdown.css` with compact styles:

```css
.markdown-prose {
  overflow-wrap: anywhere;
}

.markdown-prose :where(p, ul, ol, blockquote, pre, table) {
  margin-block: 0.625rem;
}

.markdown-prose :where(h1, h2, h3) {
  margin-block: 0.75rem 0.375rem;
  font-weight: 600;
  line-height: 1.25;
}

.markdown-prose h1 {
  font-size: 1.25rem;
}

.markdown-prose h2 {
  font-size: 1.125rem;
}

.markdown-prose h3 {
  font-size: 1rem;
}

.markdown-prose :where(ul, ol) {
  padding-left: 1.25rem;
}

.markdown-prose ul {
  list-style: disc;
}

.markdown-prose ol {
  list-style: decimal;
}

.markdown-prose blockquote {
  border-left: 2px solid hsl(var(--border));
  padding-left: 0.75rem;
  color: hsl(var(--muted-foreground));
}

.markdown-prose code {
  border-radius: 0.25rem;
  background: hsl(var(--muted));
  padding: 0.125rem 0.25rem;
  font-size: 0.875em;
}

.markdown-prose pre {
  overflow-x: auto;
  border-radius: 0.375rem;
  background: hsl(var(--muted));
  padding: 0.75rem;
}

.markdown-prose pre code {
  background: transparent;
  padding: 0;
}

.markdown-prose table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

.markdown-prose :where(th, td) {
  border: 1px solid hsl(var(--border));
  padding: 0.375rem 0.5rem;
  text-align: left;
}
```

- [x] **Step 4: 运行测试**

Run:

```bash
pnpm --dir web test -- markdown-view
pnpm --dir web typecheck
```

Expected: PASS.

- [x] **Step 5: 记录建议提交切分（未执行 commit）**

```bash
git add web/src/components/markdown
git commit -m "feat: 添加 Markdown 只读渲染"
```

### Task 5: 实现 MarkdownEditor

**Files:**
- Create: `web/src/components/markdown/markdown-editor.tsx`
- Create: `web/src/components/markdown/markdown-editor.test.tsx`
- Modify: `web/src/components/markdown/index.ts`
- Modify: `web/src/components/markdown/markdown.css`

- [x] **Step 1: 写失败测试**

Create `web/src/components/markdown/markdown-editor.test.tsx`:

```tsx
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { MarkdownEditor } from "./markdown-editor"

describe("MarkdownEditor", () => {
  it("renders initial markdown content", async () => {
    render(<MarkdownEditor onChange={() => undefined} value={"# 标题"} />)
    expect(await screen.findByText("标题")).toBeTruthy()
  })

  it("calls onModEnter for keyboard save", async () => {
    const onModEnter = vi.fn()
    render(
      <MarkdownEditor
        ariaLabel="任务描述"
        onChange={() => undefined}
        onModEnter={onModEnter}
        value="内容"
      />
    )

    await userEvent.click(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")
    expect(onModEnter).toHaveBeenCalled()
  })

  it("reports sanitized markdown changes", async () => {
    const onChange = vi.fn()
    render(<MarkdownEditor ariaLabel="任务描述" onChange={onChange} value="" />)

    await userEvent.click(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("<script>alert(1)</script>")

    await waitFor(() => {
      expect(onChange).toHaveBeenCalled()
    })
    expect(onChange.mock.calls.at(-1)?.[0]).not.toContain("<script>")
  })
})
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- markdown-editor
```

Expected: FAIL，组件尚不存在。

- [x] **Step 3: 实现编辑器**

Implement `web/src/components/markdown/markdown-editor.tsx` with these boundaries:

```tsx
import { EditorContent, useEditor } from "@tiptap/react"
import {
  BoldIcon,
  CodeIcon,
  Heading1Icon,
  Heading2Icon,
  Heading3Icon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  QuoteIcon,
  Redo2Icon,
  StrikethroughIcon,
  TableIcon,
  Undo2Icon,
} from "lucide-react"
import { useEffect } from "react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import { markdownExtensions, normalizeMarkdownSource } from "./extensions"
import "./markdown.css"
```

Required implementation details:

- `useEditor({ extensions: markdownExtensions, content: normalizeMarkdownSource(value), contentType: "markdown", editable: !disabled, immediatelyRender: false, onUpdate })`
- Use `editor.getMarkdown()` for output.
- Compare normalized external value with normalized `editor.getMarkdown()` in `useEffect` before `setContent`.
- Toolbar buttons call `editor.chain().focus()` commands:
  - `toggleBold`, `toggleItalic`, `toggleStrike`
  - `toggleHeading({ level: 1 | 2 | 3 })`
  - `toggleBulletList`, `toggleOrderedList`, `toggleTaskList`
  - `toggleBlockquote`, `toggleCode`, `toggleCodeBlock`
  - `setLink` with `window.prompt` for first version; reject unsafe URL before calling command.
  - `insertTable({ rows: 3, cols: 3, withHeaderRow: true })`
  - `undo`, `redo`
- Add `aria-label` on the content element through `editorProps.attributes`.
- Handle `Ctrl/Cmd+Enter` in `editorProps.handleKeyDown`.

Update `index.ts`:

```ts
export { MarkdownEditor } from "./markdown-editor"
export { MarkdownView } from "./markdown-view"
```

- [x] **Step 4: 运行测试和类型检查**

Run:

```bash
pnpm --dir web test -- markdown-editor
pnpm --dir web typecheck
```

Expected: PASS. If jsdom cannot exercise every ProseMirror key path reliably, keep unit tests focused and cover complex editor behavior in smoke.

- [x] **Step 5: 记录建议提交切分（未执行 commit）**

```bash
git add web/src/components/markdown
git commit -m "feat: 添加 Markdown 编辑器组件"
```

---

## Chunk 3: 任务详情页接入

### Task 6: 接入任务描述 Markdown 展示和编辑

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`

- [x] **Step 1: 写失败测试**

Update `task-detail-page.test.tsx`:

```tsx
vi.mocked(getTask).mockResolvedValue(task({
  description: "# 复盘\n\n- 素材\n- 预算",
}))

renderPage()

expect(await screen.findByRole("heading", { name: "复盘" })).toBeTruthy()
expect(screen.getByText("素材")).toBeTruthy()
```

Add breadcrumb/layout assertions:

```tsx
const workspaceLink = screen.getByRole("link", { name: "acme" })
expect(workspaceLink.getAttribute("href")).toBe("/projects")
expect(screen.getByRole("link", { name: "agentapi" }).getAttribute("href")).toBe(
  "/workspaces/acme/projects/agentapi"
)
expect(document.querySelector(".md\\:grid-cols-\\[minmax\\(0\\,1fr\\)_280px\\]")).toBeTruthy()
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- task-detail-page
```

Expected: FAIL，页面仍是纯文本展示。

- [x] **Step 3: 修改页面**

Implementation requirements:

- Replace `Textarea` import with `MarkdownEditor` and `MarkdownView` imports.
- In `TaskDescriptionBlock`, replace pure text block with:

```tsx
<MarkdownView className="max-w-3xl text-sm leading-6 text-muted-foreground">
  {value}
</MarkdownView>
```

- Replace Dialog `Textarea` with:

```tsx
<MarkdownEditor
  ariaLabel={t("projectReadonly.taskDescription")}
  minHeight={260}
  onChange={setDraft}
  onModEnter={() => {
    void save()
  }}
  value={draft}
/>
```

- Change breadcrumb to `/projects` and `projectHref`.
- Change grid class to `grid gap-5 md:grid-cols-[minmax(0,1fr)_280px]`.
- Ensure main column has `min-w-0` where needed.

- [x] **Step 4: 运行测试**

Run:

```bash
pnpm --dir web test -- task-detail-page
pnpm --dir web typecheck
```

Expected: PASS.

- [x] **Step 5: 记录建议提交切分（未执行 commit）**

```bash
git add web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx
git commit -m "feat: 任务描述支持 Markdown 编辑"
```

### Task 7: 接入任务注解 Markdown 编辑和展示

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.test.tsx`

- [x] **Step 1: 写失败测试**

Update `task-annotations-editor.test.tsx`:

```tsx
it("renders annotation markdown", () => {
  renderEditor({
    annotations: [{ id: "note-1", description: "**更新**\n\n- 截图" }],
  })

  expect(screen.getByText("更新")).toBeTruthy()
  expect(screen.getByText("截图")).toBeTruthy()
})
```

Update add/edit tests to interact with `aria-label="新增注解"` and `aria-label="编辑注解内容"` even though the element is no longer `textarea`.

- [x] **Step 2: 运行失败测试**

Run:

```bash
pnpm --dir web test -- task-annotations-editor
```

Expected: FAIL，注解仍是 textarea 和纯文本展示。

- [x] **Step 3: 修改注解组件**

Implementation requirements:

- Replace `Textarea` with `MarkdownEditor`.
- Use:

```tsx
<MarkdownEditor
  ariaLabel="新增注解"
  disabled={mutations.add.isPending}
  minHeight={120}
  onChange={(value) => {
    setDraft(value)
    setError(null)
  }}
  value={draft}
/>
```

- In list item, replace `{annotation.description}` with:

```tsx
<MarkdownView className="text-sm">{annotation.description}</MarkdownView>
```

- Edit dialog uses `MarkdownEditor minHeight={120}` and keeps current failure behavior.

- [x] **Step 4: 运行测试**

Run:

```bash
pnpm --dir web test -- task-annotations-editor
pnpm --dir web typecheck
```

Expected: PASS.

- [x] **Step 5: 记录建议提交切分（未执行 commit）**

```bash
git add web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.tsx web/src/features/workspace/project-workbench/task-detail/task-annotations-editor.test.tsx
git commit -m "feat: 任务注解支持 Markdown 编辑"
```

---

## Chunk 4: Smoke、文档同步与完整验证

### Task 8: 更新 Web Console 编辑 smoke

**Files:**
- Modify: `web/scripts/playwright-editing-smoke.mjs`

- [x] **Step 1: 增加 Markdown fixture**

Update the fake task description and annotations to include Markdown:

```js
description: "# 投放复盘\n\n- 素材\n- 预算\n\n```text\nchannel=search\n```\n\n| 字段 | 值 |\n|---|---|\n| ROI | 1.2 |",
annotations: [
  {
    id: "ann-1",
    description: "**初版素材**已同步\n\n[素材库](https://example.com/assets)",
    entry: "2026-07-04T10:00:00Z",
  },
],
```

- [x] **Step 2: 增加断言**

Add assertions after task detail loads:

```js
await expect(page.getByRole("heading", { name: "投放复盘" })).toBeVisible()
await expect(page.getByText("channel=search")).toBeVisible()
await expect(page.getByRole("link", { name: "素材库" })).toHaveAttribute(
  "href",
  "https://example.com/assets"
)
```

Add a safety edit path:

```js
await page.getByRole("button", { name: "编辑描述" }).click()
await page.getByLabel("任务描述").fill("<script>alert(1)</script>\n\n[x](javascript:alert(1))")
await page.getByRole("button", { name: "保存" }).click()
await expect(page.locator("script")).toHaveCount(0)
await expect(page.locator('a[href^="javascript:"]')).toHaveCount(0)
await expect(page.getByText("<script>alert(1)</script>")).toBeVisible()
```

- [x] **Step 3: 运行 smoke**

Run:

```bash
pnpm --dir web run smoke:editing
```

Expected: PASS. If the script starts a dev server, ensure it also stops it or document the process cleanup.

- [x] **Step 4: 记录建议提交切分（未执行 commit）**

```bash
git add web/scripts/playwright-editing-smoke.mjs
git commit -m "test: 补充 Markdown 编辑 smoke"
```

### Task 9: 同步 ROADMAP 和 README

**Files:**
- Modify: `ROADMAP.md`
- Modify: `README.md`

- [x] **Step 1: 更新 ROADMAP 状态**

If not already done before implementation, add v0.5.1 to the status table and include:

```text
## v0.5.1：Web Console Markdown 编辑器与任务详情 UX 改进

**状态：已完成。**

...
```

After implementation is complete, mark it as `已完成` and list delivered behavior.

- [x] **Step 2: 更新 README**

Only after implementation passes, update Web Console paragraphs to say task description and annotations support Markdown WYSIWYG editing/rendering while storage remains string.

- [x] **Step 3: 记录建议提交切分（未执行 commit）**

```bash
git add ROADMAP.md README.md
git commit -m "docs: 更新 Markdown 编辑器文档"
```

### Task 10: 完整验证

**Files:**
- No source changes unless verification finds a bug.

- [x] **Step 1: 前端验证**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```

Expected: all PASS.

- [x] **Step 2: Go / CGO 验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: all PASS. This feature is frontend-only, but repository completion still requires CGO-free build verification.

- [x] **Step 3: Diff hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only intended files modified or committed.

- [x] **Step 4: Final commit decision（未执行 commit）**

If verification fixes created additional changes:

```bash
git add <changed-files>
git commit -m "fix: 修正 Markdown 编辑器验收问题"
```

---

## Execution Notes

- Do not implement business logic in `internal/cli` or backend layers for this feature; backend contract intentionally stays unchanged.
- Do not introduce `react-markdown`, `remark-gfm`, `tiptap-markdown`, `@tailwindcss/typography`, image upload, or HTML rendering.
- Do not update README before the implementation actually works.
- If Tiptap v3.27.1 type signatures differ from snippets above, adjust to the installed package types while preserving these invariants:
  - one shared schema source,
  - HTML escaped before Markdown parse,
  - Link protocol whitelist,
  - no duplicate StarterKit Link registration,
  - Table includes row/cell/header schema.
