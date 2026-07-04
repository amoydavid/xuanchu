# Web Console Tiptap Markdown 编辑器与 Task 详情页 UX 改进

- 日期：2026-07-04
- 状态：已完成
- 里程碑：v0.5.1
- 关联文档：[ROADMAP.md](../../../ROADMAP.md)、[README.md](../../../README.md)、`docs/superpowers/specs/2026-06-22-xuanchu-task-title-description-design.md`、`docs/superpowers/specs/2026-06-27-xuanchu-web-console-editing-design.md`、`docs/superpowers/specs/2026-06-17-web-console-project-task-browsing-design.md`

## 1. 背景与目标

Web Console 当前对任务详情页里的长文本只提供纯 `<textarea>` 编辑，展示用 `whitespace-pre-wrap`。这会让导入文档中已经约定的 Markdown 详情只能以源码形式显示，也让非技术成员编辑复杂描述和注解时成本偏高。

本期引入 Tiptap v3，目标是把任务详情页的任务描述和任务注解升级为 WYSIWYG Markdown 编辑和展示，同时顺手修正详情页的面包屑与主区/属性栏比例。组件应设计成可复用能力，但首期只接入任务详情页两个入口，避免把项目描述、评论等未来入口一起扩大进本里程碑。

目标：

1. **通用 `MarkdownEditor` 组件**：基于 Tiptap v3 的 WYSIWYG 编辑器，输入和输出都保持 Markdown 字符串。
2. **通用 `MarkdownView` 组件**：只读场景用同一套 Tiptap schema 渲染 Markdown，不引入第二套 parser。
3. **任务详情页接入**：任务描述、任务注解新增/编辑/展示改用 Markdown 组件。
4. **详情页 UX 局部改进**：面包屑增加有效链接，主区和属性面板调整为更适合长文本阅读的比例。

## 2. 范围与约束

### 2.1 进入本期

| 位置 | 现状 | 本期改动 |
|---|---|---|
| `task-detail-page.tsx` `TaskDescriptionBlock` | `<Textarea>` 编辑（Dialog）+ `whitespace-pre-wrap` 纯文本展示 | 编辑改用 `MarkdownEditor`；展示改用 `MarkdownView` |
| `task-annotations-editor.tsx` | 新增/编辑注解都是 `<Textarea>`，展示纯文本 | 新增/编辑改用 `MarkdownEditor`；列表展示改用 `MarkdownView` |
| 详情页栅格 `md:grid-cols-[1fr_240px]` | 主区偏窄、属性面板固定 240px | 调整为 `md:grid-cols-[minmax(0,1fr)_280px]` |
| 顶部 `workspaceSlug / projectSlug / task_slug` 面包屑 | 纯文本无链接 | workspace 段链接到现有 `/projects`，project 段链接到现有 `projectHref`，task 段保持纯文本 |
| `web/package.json` | 无 Tiptap / Markdown 渲染依赖 | 新增 Tiptap v3.27.1 系列依赖 |

### 2.2 不进入本期

- 不改变后端模型和 API 契约：`task.description`、`annotation.description` 仍是普通字符串。
- 不改 CLI、MCP、Remote Client 的文本输出格式；这些入口继续看到 Markdown 源码。
- 不接入项目描述。`ProjectHeaderEditor` 的 project description 仍沿用当前 `InlineTextEditor`，后续可复用本期组件另开小任务接入。
- 不做 inline 原地编辑任务描述，保留现有 Dialog 触发方式。
- 不做 WYSIWYG / Markdown 源码双模式切换。
- 不引入图片、附件、HTML 内嵌、数学公式、语法高亮或多人协同。
- 不覆盖任务标题。标题继续使用单行 `InlineTextEditor`。

### 2.3 持久化兼容

- 后端零改动；数据库仍保存字符串。
- 历史纯文本会被 Markdown parser 当作普通段落展示，无迁移。
- 用户通过 Web Console 编辑保存后，服务端存储的是 Markdown 源码。
- 空描述继续按现有逻辑提交 `{ clear_description: true }`；空注解继续禁止提交。

## 3. 选型与版本

本期使用 Tiptap v3.27.1 系列。需要锁定同一小版本，避免 `@tiptap/markdown` 与 `@tiptap/static-renderer` API 在小版本间不一致。

依赖：

```text
@tiptap/react@3.27.1
@tiptap/starter-kit@3.27.1
@tiptap/pm@3.27.1
@tiptap/markdown@3.27.1
@tiptap/static-renderer@3.27.1
@tiptap/extension-link@3.27.1
@tiptap/extension-list@3.27.1
@tiptap/extension-table@3.27.1
```

说明：

- v3 的 `StarterKit` 已包含 Link、Underline、ListKeymap。为了配置链接协议白名单，本期禁用 StarterKit 内置 Link，再显式注册 `@tiptap/extension-link`。
- TaskList / TaskItem 使用 v3 推荐的 `@tiptap/extension-list` 聚合包导入。
- Table 使用 `@tiptap/extension-table` 中的 `Table`、`TableRow`、`TableHeader`、`TableCell` 四个 extension；只注册 `Table` 不足以完整支持表格 schema。
- 不引入社区包 `tiptap-markdown`。
- 不引入 `react-markdown` / `remark-gfm`，避免编辑和展示使用两套 Markdown parser。

备选方案放弃理由：

- CodeMirror 6：源码编辑好，但不是 WYSIWYG。
- Lexical：能力接近，但 Markdown 双向序列化和静态渲染闭环不如 Tiptap v3 明确。
- 存 HTML：破坏 CLI / Taskwarrior 风格字符串契约。
- `react-markdown` 只读渲染：会形成 Tiptap parser 与 remark parser 的双语义风险。

## 4. Markdown 架构

### 4.1 文件边界

新增 `web/src/components/markdown/`：

| 文件 | 职责 |
|---|---|
| `markdown-safety.ts` | Markdown 输入安全归一化：转义原始 HTML 标签、校验链接协议 |
| `extensions.ts` | 导出 Tiptap extensions、`MarkdownManager`、Markdown parse/serialize helper |
| `markdown-view.tsx` | 只读 Markdown 渲染 |
| `markdown-editor.tsx` | 受控 WYSIWYG Markdown 编辑器 |
| `markdown.css` | 编辑态与只读态共享的最小排版样式 |
| `*.test.tsx` / `*.test.ts` | 安全、序列化、渲染和交互测试 |

### 4.2 共享 schema

`extensions.ts` 使用单一 schema：

```ts
import Link from "@tiptap/extension-link"
import { TaskItem, TaskList } from "@tiptap/extension-list"
import { Table, TableCell, TableHeader, TableRow } from "@tiptap/extension-table"
import { Markdown, MarkdownManager } from "@tiptap/markdown"
import StarterKit from "@tiptap/starter-kit"

import { isAllowedMarkdownHref } from "./markdown-safety"

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
```

实施时如果 Tiptap 类型要求 extension 数组拆分为 editor-only / renderer-only，应保留同一份配置来源，不允许编辑器与只读渲染维护两套不同 schema。

### 4.3 HTML 禁用策略

Tiptap v3.27.1 的 `@tiptap/markdown` 会把 Markdown 中的原始 HTML token 交给 extension 的 `parseHTML` 规则解析，当前 API 没有可依赖的 `html: false` 选项。因此本期不能把“禁用 HTML”写成待确认配置，而必须在进入 Tiptap parser 前做显式归一化。

新增 `escapeMarkdownHtml(source: string): string`：

- 在非代码块、非行内代码的普通 Markdown 文本中，识别形如 `<tag ...>`、`</tag>`、`<tag />` 的原始 HTML 标签。
- 将这些标签的尖括号转义为 `&lt;` / `&gt;`，使其作为文本进入 Tiptap。
- 不把 Markdown 链接、普通 URL、代码块、行内代码改写。
- 该函数用于：
  - `MarkdownView` parse 前。
  - `MarkdownEditor` 初始化 / 外部 `value` 写回前。
  - `MarkdownEditor` `onChange` 上报前。

这意味着 Web Console 保存用户新输入的 HTML 标签时，会保存转义后的 Markdown 文本。历史数据只读展示时不会写回数据库，除非用户打开编辑器并保存。

最小规则示例：

```ts
escapeMarkdownHtml("<script>alert(1)</script>")
// "&lt;script&gt;alert(1)&lt;/script&gt;"

escapeMarkdownHtml("`<script>`")
// "`<script>`"

escapeMarkdownHtml("```html\n<script>\n```")
// "```html\n<script>\n```"
```

### 4.4 链接安全

新增 `isAllowedMarkdownHref(url: string): boolean`：

```ts
export function isAllowedMarkdownHref(url: string): boolean {
  return /^(https?:\/\/|mailto:)/i.test(url.trim())
}
```

要求：

- `javascript:`、`data:`、空协议、相对路径均不渲染为可点击链接。
- 编辑态和只读态共用同一 Link extension 配置。
- 只读渲染后补充测试断言非法链接没有 `href`。

## 5. 组件接口

### 5.1 `MarkdownEditor`

```tsx
type MarkdownEditorProps = {
  value: string
  onChange: (markdown: string) => void
  placeholder?: string
  ariaLabel?: string
  minHeight?: number
  className?: string
  disabled?: boolean
  onModEnter?: () => void
}
```

行为：

- 受控组件。`value` 变化时，先经过 `escapeMarkdownHtml`，再用 `editor.commands.setContent(nextValue, { contentType: "markdown" })` 写回。
- 为避免光标跳动，只有当外部 `value` 与 `editor.getMarkdown()` 的安全归一化结果不一致时才写回。
- `onUpdate` 中使用 `editor.getMarkdown()` 取 Markdown 字符串，经过 `escapeMarkdownHtml` 后上报。
- `Cmd/Ctrl+Enter` 调用可选 `onModEnter`，由父组件决定是否保存。
- 工具栏提供：粗体、斜体、删除线、H1/H2/H3、无序列表、有序列表、待办列表、引用、代码、代码块、链接、插入 3x3 表格、撤销、重做。
- 工具栏按钮使用 `lucide-react` 图标；不使用纯文本按钮表达已有通用图标的动作。
- 不提供源码模式，不提供图片按钮。

### 5.2 `MarkdownView`

```tsx
type MarkdownViewProps = {
  children: string
  className?: string
}
```

行为：

- 空字符串返回 `null`，占位文案由调用方负责。
- 渲染前调用 `escapeMarkdownHtml(children)`。
- 使用 `markdownManager.parse()` 得到 ProseMirror JSON。
- 使用 `renderToReactElement`，导入路径为 `@tiptap/static-renderer/pm/react`。
- 使用与编辑器相同的 `markdownExtensions`。
- 不实例化 editor，不产生 `contenteditable`。

### 5.3 样式

新增最小 `.markdown-prose` 样式，覆盖：

- `p`、`h1`、`h2`、`h3`
- `ul`、`ol`、`li`
- task list checkbox 对齐
- `blockquote`
- `code`、`pre`
- `table`、`thead`、`tbody`、`th`、`td`
- `a`

不引入 `@tailwindcss/typography`，避免为了两个入口扩大样式依赖。

## 6. 任务详情页改动

### 6.1 描述块

- 展示：`whitespace-pre-wrap` 纯文本改为 `<MarkdownView>{value}</MarkdownView>`。
- 空描述占位文案不变。
- Dialog 内 `<Textarea>` 改为 `<MarkdownEditor>`。
- 保存逻辑不变：归一化后为空提交 `{ clear_description: true }`，否则提交 `{ description }`。
- Dialog 的 `Cmd/Ctrl+Enter` 改由 `MarkdownEditor.onModEnter` 触发。

### 6.2 注解编辑器

- 新增注解 composer 改为 `<MarkdownEditor minHeight={120}>`。
- 编辑注解 Dialog 改为 `<MarkdownEditor minHeight={120}>`。
- 注解列表展示改为 `<MarkdownView>`。
- 空注解校验：`markdown.trim()` 非空即可，不做“去除 Markdown 符号后仍有可见字符”的复杂校验。
- 新增和编辑失败时继续保留当前草稿。

### 6.3 面包屑

当前路由没有 `/workspaces/:workspaceSlug` workspace overview，因此 workspace 段不能链接到不存在的页面。

改为：

```tsx
<nav className="text-xs text-muted-foreground">
  <a href="/projects">{workspaceSlug}</a>
  {" / "}
  <a href={projectHref}>{projectSlug}</a>
  {" / "}
  <span>{taskData.task_slug || taskData.uuid.slice(0, 8)}</span>
</nav>
```

- workspace 段链接到当前 workspace 下的项目列表 `/projects`。
- project 段链接到现有 `projectHref`。
- task 段保持纯文本。

### 6.4 栅格布局

- `md:grid-cols-[1fr_240px]` 改为 `md:grid-cols-[minmax(0,1fr)_280px]`。
- 主区长文本容器使用 `min-w-0`，防止表格、代码块撑破布局。
- 移动端三 Tab 行为不变。

## 7. 测试策略

### 7.1 Markdown 安全与序列化测试

`markdown-safety.test.ts`：

- 原始 `<script>`、`<iframe>`、`<img onerror>` 被转义。
- 行内代码和 fenced code block 中的 HTML 不被转义。
- `https://`、`http://`、`mailto:` 通过链接校验。
- `javascript:`、`data:`、相对路径不通过链接校验。

`markdown-view.test.tsx`：

- 渲染标题、段落、粗体、列表、引用、代码块、表格、待办列表。
- `<script>alert(1)</script>` 以文本出现，不产生 `script` 节点。
- `[x](javascript:alert(1))` 不产生带危险 `href` 的链接。

`markdown-editor.test.tsx`：

- 初始 `value` 能渲染。
- 输入/工具栏操作触发 `onChange`，输出 Markdown 字符串。
- 粗体按钮对选区输出 `**text**`。
- 表格和待办列表 round-trip 不丢内容。
- `Cmd/Ctrl+Enter` 调用 `onModEnter`。

### 7.2 业务组件测试

`task-detail-page.test.tsx`：

- 描述展示渲染 Markdown 元素。
- 描述 Dialog 中出现 `MarkdownEditor`。
- 清空描述仍提交 `{ clear_description: true }`。
- 面包屑 workspace 链接到 `/projects`，project 链接到现有项目页。
- 主布局 class 包含 `md:grid-cols-[minmax(0,1fr)_280px]`。

`task-annotations-editor.test.tsx`：

- 新增注解使用 `MarkdownEditor`，提交 Markdown 字符串。
- 编辑注解 Dialog 使用 `MarkdownEditor`。
- 注解列表用 `MarkdownView` 渲染。
- 空注解不提交，失败时保留草稿。

### 7.3 Smoke

更新 `web/scripts/playwright-editing-smoke.mjs`：

- 任务描述保存一段含标题、列表、代码块、表格、待办列表的 Markdown，详情页展示结构化结果。
- 新增注解含粗体和合法链接，列表展示为 Markdown。
- 粘贴 `<script>alert(1)</script>` 与 `[x](javascript:alert(1))` 后不执行脚本、不产生危险链接。
- 移动端三 Tab 切换仍正常。

### 7.4 验证命令

从仓库根目录执行：

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

如果 smoke 需要真实 server，应在计划中明确启动和停止方式，不能只跑单元测试就声称 Web Console 编辑流完成。

## 8. 实施步骤概览

1. 锁定 Tiptap v3.27.1 依赖。
2. 新增 `markdown-safety.ts` 与安全测试。
3. 新增 `extensions.ts`，打通 Markdown parse/render helper。
4. 新增 `MarkdownView` 和样式。
5. 新增 `MarkdownEditor` 和工具栏。
6. 替换 `TaskDescriptionBlock`。
7. 替换 `TaskAnnotationsEditor`。
8. 调整详情页面包屑和栅格布局。
9. 更新单元测试、组件测试和 smoke。
10. 运行完整验证。

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| `@tiptap/markdown` 仍在迭代 | 锁定 3.27.1；升级前跑完整 round-trip、view、editor、smoke 测试 |
| 原始 HTML 进入 Tiptap parser 被解析 | 不依赖不存在的 `html:false`；进入 parser 前统一 `escapeMarkdownHtml` |
| 表格 schema 不完整 | 使用 `Table`、`TableRow`、`TableHeader`、`TableCell` 全量注册，并写表格测试 |
| StarterKit 与 Link 重复注册 | `StarterKit.configure({ link:false })` 后显式注册自定义 Link |
| 包体积增长 | 首期接受任务详情页加载成本；如 build 体积明显异常，再在实现计划中把 description Dialog 的 `MarkdownEditor` 动态 import，注解 composer 保持常驻 |
| 历史纯文本被 Markdown 解析出意外样式 | 保持 Markdown 标准行为；测试覆盖纯文本、多行文本、特殊符号 |
| 编辑器测试在 jsdom 中不稳定 | 工具栏核心命令用组件测试覆盖，复杂交互交给 Playwright smoke |

## 10. 文档同步

- `ROADMAP.md`：v0.5.1 已标记为已完成，并指向本 spec 和 implementation plan。
- `README.md`：Web Console 段落已说明任务 description / 注解支持 Markdown WYSIWYG 编辑与同 schema 只读渲染，同时后端仍保存普通字符串。
- 本 spec 已拆分对应 implementation plan。
