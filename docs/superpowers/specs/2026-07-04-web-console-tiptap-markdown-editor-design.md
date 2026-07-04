# Web Console Tiptap Markdown 编辑器与 Task 详情页 UX 改进

- 日期：2026-07-04
- 状态：草案
- 里程碑：v0.5.1
- 关联文档：[ROADMAP.md](../../../ROADMAP.md)、[README.md](../../../README.md)、`docs/superpowers/specs/2026-06-22-xuanchu-task-title-description-design.md`、`docs/superpowers/specs/2026-06-27-xuanchu-web-console-editing-design.md`、`docs/superpowers/specs/2026-06-17-web-console-project-task-browsing-design.md`

## 1. 背景与目标

### 1.1 目标

Web Console 当前对长文本（任务描述、注解）只提供纯 `<textarea>`，展示用 `whitespace-pre-wrap`，不支持 markdown 渲染，也无法所见即所得地编辑。本期引入 [tiptap.dev](https://tiptap.dev) 作为通用 markdown 编辑器，并顺带修正 task 详情页两处明显的 UX 短板：

1. **通用 `MarkdownEditor` 组件**：基于 tiptap 的 WYSIWYG 编辑器，覆盖 web 端所有长文本编辑入口（任务描述、注解，以及未来 project 说明、评论等）。
2. **markdown 渲染展示**：只读场景（描述、注解列表）由纯文本切换为 markdown 渲染，与编辑态视觉一致。
3. **详情页 UX 局部改进**：面包屑加可点击链接、调整主区与属性面板的栅格比例。

### 1.2 既有约束

- **存储格式不变**：`description`、`annotation.description` 在后端仍是纯字符串，与 Taskwarrior JSON、CLI、MCP、Remote Client 全部兼容。Web 端只是把这个字符串当作 markdown 来编辑与渲染；CLI 用户看到的仍是 markdown 源码（多行文本），语义不变。
- **不引入图片/附件**：本期不支持图片上传、附件存储、HTML 内嵌、数学公式。仅中等 markdown 子集。
- **不改变编辑触发方式**：描述仍由「编辑按钮」打开（Dialog 路径保留），注解仍是 textarea-style 的新增/编辑流。inline 原地编辑、评论区等不在本期范围。
- **`CGO_ENABLED=0` 不受影响**：本期仅前端改动，后端无改动。

### 1.3 非目标

- 不做 inline 原地编辑（点描述直接进入编辑态），保留 Dialog 路径以控制本 spec 范围。
- 不引入图片/附件上传服务。
- 不做实时多人协同编辑。
- 不做 WYSIWYG ↔ 源码双模式切换（仅 WYSIWYG）。
- 不改后端 `description`/`annotation` 模型与 API 契约。
- 不覆盖任务标题（单行 inline 编辑器，不适合富文本）。
- 不改 CLI / MCP / Remote Client 的文本输出格式。

## 2. 现状与扩展点

| 既有位置 | 现状 | 本期改动 |
|---|---|---|
| `task-detail-page.tsx` `TaskDescriptionBlock` | `<Textarea>` 编辑（Dialog）+ `whitespace-pre-wrap` 纯文本展示 | 改用 `MarkdownEditor`；展示改用 `MarkdownView` 渲染 |
| `task-annotations-editor.tsx` | 新增/编辑注解都是 `<Textarea>`，展示纯文本 | 改用 `MarkdownEditor`；列表展示改用 `MarkdownView` |
| 详情页栅格 `md:grid-cols-[1fr_240px]` | 主区窄、属性面板固定 240px | 调整为 `md:grid-cols-[minmax(0,1fr)_280px]`，属性面板 280px，并允许窄屏塌缩 |
| 顶部 `workspaceSlug / projectSlug / task_slug` 面包屑 | 纯文本无链接 | workspace、project 段改为 `<a>`，task 段保持纯文本 |
| `web/package.json` 依赖 | 无 tiptap、无 markdown 渲染器 | 新增 `@tiptap/*`、`tiptap-markdown`、`react-markdown`、`remark-gfm` |

**核心新增组件**：`web/src/components/markdown/` 下新增两个可复用组件：

- `MarkdownEditor`：受控 WYSIWYG 编辑器，输入/输出均为 markdown 字符串。
- `MarkdownView`：只读 markdown 渲染器，与编辑器视觉风格一致。

## 3. 设计决策

### 3.1 选型：为什么是 tiptap

- **WYSIWYG 优先**：项目用户多为非技术成员（项目协作场景），源码模式体验差。tiptap 的 ProseMirror 内核是 React 生态里最成熟的 WYSIWYG 框架。
- **可序列化为 markdown**：通过 `tiptap-markdown` 扩展，编辑器内容可双向序列化为 markdown 字符串，与后端纯字符串存储契约一致。
- **可裁剪语法**：通过按需引入扩展（StarterKit + TaskList + Link + Table + Underline），精确控制支持的 markdown 子集，避免滥用。
- **React 19 兼容**：tiptap v2 官方支持 React 19，无版本阻塞。

**备选方案与放弃理由**：

- **CodeMirror 6 + markdown 语法高亮**：源码编辑体验好，但不是 WYSIWYG，不符合本期目标。
- **react-markdown + 自建 textarea**：仅解决渲染，编辑仍是纯文本，体验无提升。
- **Lexical（Meta）**：能力对等，但 markdown 序列化生态弱于 tiptap，社区文档以英文为主且偏少。
- **存储 HTML（如 milkdown/Quill 默认）**：破坏 CLI/Taskwarrior 兼容，已排除。

### 3.2 存储格式：存 markdown 源码

- 后端零改动：`description`、`annotation.description` 仍是 `string`。
- `MarkdownEditor` 在 `onChange`/`onSave` 时通过 `tiptap-markdown` 的 `editor.storage.markdown.getMarkdown()` 输出 markdown 字符串。
- 写入前做一次「幂等净化」：序列化后再解析回 ProseMirror，确认 round-trip 不丢内容（仅在开发环境断言，生产不阻塞保存）。
- 读取历史纯文本时：旧数据（无 markdown 语法）会被 markdown 解析器视为普通段落，正常显示，无迁移成本。

### 3.3 markdown 子集（中等）

启用的 tiptap 扩展：

| 扩展 | 对应 markdown |
|---|---|
| StarterKit（Document/Paragraph/Text/Heading/Bold/Italic/Strike/Code/CodeBlock/BulletList/OrderedList/ListItem/Blockquote/HorizontalRule/HardBreak） | 标题、粗/斜/删除线、行内代码、代码块、有序/无序列表、引用、分隔线 |
| TaskList + TaskItem | `- [ ]` / `- [x]` 待办列表 |
| Link | `[text](url)` 链接（仅 http/https/mailto） |
| Table | GFM 表格 |
| Underline | `<u>` 下划线（兼容旧 GFM） |

**显式不支持**：

- 图片（Image 扩展不启用；粘贴图片转为链接文本）
- 原生 HTML（输入 HTML 字符串会被当作纯文本，不解析）
- 数学公式（不引入 KaTeX）

### 3.4 工具栏与交互

`MarkdownEditor` 顶部工具栏提供：粗体、斜体、删除线、H1/H2/H3、无序列表、有序列表、待办列表、引用、代码、代码块、链接、表格、撤销/重做。

- 工具栏按钮使用 `lucide-react`（项目已用）。
- 快捷键沿用 tiptap 默认（`Cmd/Ctrl+B` 加粗等），与主流编辑器一致。
- `Cmd/Ctrl+Enter` 提交保存（沿用现有 Dialog 行为）。
- 受控模式：父组件持有 markdown 字符串状态，`onChange` 实时上报。

### 3.5 渲染展示（MarkdownView）

- 只读场景使用 `react-markdown` + `remark-gfm`（而非 tiptap 的只读模式），原因：渲染静态字符串更轻量、SSR 安全、无需挂载 ProseMirror。
- 样式与编辑器内容区一致：通过共享 `prose` class（Tailwind Typography 风格的自定义类，不引入 `@tailwindcss/typography` 依赖，手写最小样式表覆盖 `h1/h2/ul/ol/code/pre/blockquote/table`）。
- 代码块用 `react-syntax-highlighter` 或最小自实现 `<pre><code>`；本期选最小自实现（不引高亮库），如需高亮后续 milestone 再加。

## 4. 组件接口

### 4.1 MarkdownEditor

```tsx
type MarkdownEditorProps = {
  value: string                 // markdown 源码
  onChange: (markdown: string) => void
  placeholder?: string
  editable?: boolean            // 默认 true；false 时退化为只读渲染（与 MarkdownView 一致）
  ariaLabel?: string
  minHeight?: number            // 默认 240
  className?: string
}
```

- 受控组件：`value` 变化时，仅当与编辑器当前序列化结果不一致时才 `setContent`，避免光标跳动。
- 内部维护 `EditorContent`，所有 tiptap 状态不外泄。

### 4.2 MarkdownView

```tsx
type MarkdownView = (props: {
  children: string              // markdown 源码
  className?: string
}) => JSX.Element
```

- 纯渲染，无可变状态。空字符串渲染占位提示由调用方负责。

### 4.3 详情页改动点

### 4.3.1 描述块（TaskDescriptionBlock）

- 展示：`whitespace-pre-wrap` 纯文本 → `<MarkdownView>`。
- 空描述占位文案不变。
- 编辑 Dialog 内的 `<Textarea>` → `<MarkdownEditor>`，保留 Dialog 外壳、保存/取消按钮、`Cmd+Enter` 提交。
- `draft` 状态从 `string` 改为受 `MarkdownEditor.onChange` 上报的 markdown 字符串。

### 4.3.2 注解编辑器（TaskAnnotationsEditor）

- 新增注解的 `<Textarea>` → `<MarkdownEditor minHeight={120}>`。
- 编辑注解 Dialog 内的 `<Textarea>` → `<MarkdownEditor minHeight={120}>`。
- 注解列表展示：纯文本 `<div>` → `<MarkdownView>`。
- 注解为空的校验改为 `markdown.trim()` 去空白后非空（去除 markdown 语法符号后仍需有可见字符；最小校验：trim 后非空，不过度严格）。

### 4.3.3 面包屑

`workspaceSlug / projectSlug / task_slug` 改为：

```tsx
<nav className="text-xs text-muted-foreground">
  <a href={`/workspaces/${workspaceSlug}`}>{workspaceSlug}</a>
  {" / "}
  <a href={projectHref}>{projectSlug}</a>
  {" / "}
  {taskData.task_slug || taskData.uuid.slice(0, 8)}
</nav>
```

- workspace 段链接到 `/workspaces/${workspaceSlug}`（workspace overview）。
- project 段链接到现有 `projectHref`。
- task 段保持纯文本（自身所在页）。

### 4.3.4 栅格布局

- 现状：`md:grid-cols-[1fr_240px]`，属性面板固定 240px，主区在宽屏下偏窄。
- 改为：`md:grid-cols-[minmax(0,1fr)_280px]`，属性面板 280px（容纳日期/标签等两列字段更舒展），主区 `minmax(0,1fr)` 防止内容溢出。
- 移动端三 Tab 行为不变。

## 5. 安全考虑

- **链接协议白名单**：`Link` 扩展配置 `HTMLAttributes.rel = "noopener noreferrer"`，并在 `react-markdown` 的 `url` 转换处过滤仅允许 `http/https/mailto`，避免 `javascript:` URL。
- **不渲染任意 HTML**：`MarkdownView` 不启用 `rehype-raw`，所有输入按纯文本/markdown 语法解析。
- **代码块**：仅 `<pre><code>` 渲染，不执行任何高亮脚本。
- **XSS**：依赖 `react-markdown` 默认转义，输入的 HTML 字符串以纯文本显示。

## 6. 测试策略

### 6.1 单元测试（vitest + testing-library）

- `MarkdownEditor.test.tsx`：
  - 受控 `value` 渲染后内容正确。
  - 输入触发 `onChange` 上报 markdown 字符串。
  - 工具栏粗体按钮对选区加粗，输出 `**text**`。
  - `editable={false}` 时退化为只读渲染。
- `MarkdownView.test.tsx`：
  - 渲染常见 markdown 元素（标题、列表、代码块、表格、链接）。
  - 不渲染 HTML 标签（XSS 防护）。
  - `javascript:` 链接被过滤。

### 6.2 组件测试（更新现有测试）

- `task-detail-page.test.tsx`：
  - 描述展示用 `MarkdownView`（断言渲染出 markdown 元素，如 `<h1>`）。
  - 描述编辑 Dialog 打开后包含 `MarkdownEditor`。
  - 面包屑包含 workspace、project 的 `<a>` 链接。
  - 栅格 class 含 `md:grid-cols-[minmax(0,1fr)_280px]`。
- `task-annotations-editor.test.tsx`：
  - 新增/编辑注解使用 `MarkdownEditor`。
  - 注解列表项用 `MarkdownView` 渲染。

### 6.3 验证命令

```bash
cd web && pnpm typecheck && pnpm test && pnpm build
CGO_ENABLED=0 go build ./cmd/xuanchu   # web dist 嵌入构建（如 Makefile 依赖）
```

### 6.4 手测清单（写入 spec，验收时执行）

- 描述：编辑含标题、列表、代码块、表格、待办列表的 markdown，保存后在详情页正确渲染。
- 注解：新增一条含粗体+链接的注解，列表展示正确。
- 老数据兼容：已有纯文本描述（无 markdown 语法）正常显示为段落。
- 面包屑：workspace、project 链接可跳转，task 段不可点。
- 移动端：三 Tab 切换正常，编辑器高度自适应。

## 7. 实施步骤（概览，详细 plan 由 writing-plans 拆分）

1. 引入依赖：`@tiptap/react`、`@tiptap/starter-kit`、`@tiptap/extension-task-list`、`@tiptap/extension-task-item`、`@tiptap/extension-link`、`@tiptap/extension-table`、`@tiptap/extension-underline`、`tiptap-markdown`、`react-markdown`、`remark-gfm`。
2. 新增 `web/src/components/markdown/markdown-view.tsx`（先做只读渲染，可独立合入）。
3. 新增 `web/src/components/markdown/markdown-editor.tsx`（含工具栏）。
4. 替换 `TaskDescriptionBlock` 展示与编辑。
5. 替换 `TaskAnnotationsEditor` 新增/编辑/展示。
6. 详情页面包屑 + 栅格调整。
7. 更新现有测试 + 新增组件测试。
8. 手测清单验收。

## 8. 风险与权衡

| 风险 | 缓解 |
|---|---|
| tiptap v2 与 React 19 兼容性 | 官方已支持；落地前先跑 `pnpm typecheck && pnpm build` |
| 包体积增长（tiptap + react-markdown） | tiptap 按需引扩展，`react-markdown` 仅用于只读；Vite code-split 编辑器懒加载 |
| round-trip 序列化丢内容（表格/待办边缘语法） | 开发期断言；测试覆盖每种语法 |
| 注解文本纯文本历史被当 markdown 解析出现意外格式 | markdown 对纯文本宽容，多数情况渲染为段落；测试用例覆盖老数据 |
| 任务标题误用富文本 | 显式不覆盖标题，保留 `InlineTextEditor` |

## 9. 文档同步

- `ROADMAP.md`：新增 v0.5.1「Web Console Tiptap Markdown 编辑器与 Task 详情页 UX 改进」。
- `README.md`：如用户可见行为变化（描述/注解现在支持 markdown 渲染），在 web console 段落补一句说明。
- 本 spec 通过后，用 `superpowers:writing-plans` 拆实施计划。
