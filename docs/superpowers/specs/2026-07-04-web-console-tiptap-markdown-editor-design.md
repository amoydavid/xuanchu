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
| `web/package.json` 依赖 | 无 tiptap、无 markdown 渲染器 | 新增 `@tiptap/react`、`@tiptap/starter-kit`、`@tiptap/markdown`、`@tiptap/static-renderer`、`@tiptap/extension-task-list`、`@tiptap/extension-task-item`、`@tiptap/extension-link`、`@tiptap/extension-table`（v3，当前 3.27.x） |

**版本基线**：本期统一采用 **tiptap v3**（2025-06-24 GA，2026 年迭代至 v3.27.x），不再使用 v2。v3 的官方 `@tiptap/markdown` 与 `@tiptap/static-renderer` 是关键依赖，社区包 `tiptap-markdown`（aguingand）已被官方取代并标记弃用，本项目不引入。

**核心新增组件**：`web/src/components/markdown/` 下新增两个可复用组件：

- `MarkdownEditor`：受控 WYSIWYG 编辑器，输入/输出均为 markdown 字符串。
- `MarkdownView`：只读 markdown 渲染器，与编辑器视觉风格一致。

## 3. 设计决策

### 3.1 选型：为什么是 tiptap v3

- **WYSIWYG 优先**：项目用户多为非技术成员（项目协作场景），源码模式体验差。tiptap 的 ProseMirror 内核是 React 生态里最成熟的 WYSIWYG 框架。
- **官方 markdown 双向序列化**：v3.7.0+ 起官方提供 `@tiptap/markdown`（当前 3.27.x），编辑器内容可双向序列化为 markdown 字符串，与后端纯字符串存储契约一致。社区包 `tiptap-markdown` 已弃用，不引入。
- **统一 schema 的只读渲染**：v3 的 `@tiptap/static-renderer` 可在不实例化 editor 的情况下，用与编辑器相同的 extensions 把 ProseMirror JSON 渲染成 React 元素。编辑态与只读态共享同一份 node/mark schema，从根本上消除「编辑时看到的表格/待办列表在展示时被另一种 parser 渲染走样」的双 parser 风险。
- **React 19 + SSR 官方支持**：v3 明确支持 React 19 与 SSR，无版本阻塞。
- **StarterKit 已含 Link / ListKeymap / Underline**：v3 的 StarterKit 比扩展列表更简洁。

**备选方案与放弃理由**：

- **CodeMirror 6 + markdown 语法高亮**：源码编辑体验好，但不是 WYSIWYG，不符合本期目标。
- **react-markdown 双 parser 方案**：编辑用 tiptap、只读用 react-markdown。问题：两套独立 markdown parser（ProseMirror vs remark），表格/待办列表/GFM 扩展语法在两边语义不完全一致，编辑保存后展示可能走样；维护两套样式。v3 的 static-renderer 让这套方案失去意义。
- **Lexical（Meta）**：能力对等，但 markdown 序列化生态弱于 tiptap。
- **存储 HTML（如 milkdown/Quill 默认）**：破坏 CLI/Taskwarrior 兼容，已排除。
- **tiptap v2**：v3 已 GA 且能力全面超越 v2（官方 markdown、static-renderer、React 19 一等支持），新项目无理由停留在 v2。

### 3.2 存储格式：存 markdown 源码

- 后端零改动：`description`、`annotation.description` 仍是 `string`。
- `MarkdownEditor` 内部用 `@tiptap/markdown` 把 ProseMirror doc 序列化为 markdown 字符串，在 `onChange`/`onSave` 时上报。
- 受控 `value` 写回编辑器时，仅当 `value` 与编辑器当前序列化结果不一致才 `setContent(md, { contentType: 'markdown' })`，避免光标跳动与循环更新。
- 读取历史纯文本时：旧数据（无 markdown 语法）会被 markdown parser 视为普通段落，正常显示，无迁移成本。
- **round-trip 校验**：开发环境断言「序列化 → 再解析 → 再序列化」幂等，防止表格/待办列表等边缘语法丢内容；生产环境不阻塞保存。

### 3.3 共享 schema（编辑器与只读渲染统一）

`web/src/components/markdown/extensions.ts` 导出唯一一份 extensions 配置，编辑器与 static-renderer 共享：

```ts
import StarterKit from "@tiptap/starter-kit"
import { TaskList, TaskItem } from "@tiptap/extension-task-list"
// TaskItem 从 @tiptap/extension-task-item 导入（按 v3 实际包结构调整）
import Link from "@tiptap/extension-link"
import Table from "@tiptap/extension-table"

export const markdownExtensions = [
  StarterKit,                       // v3 内含 Heading/Bold/Italic/Strike/Code/CodeBlock/
                                    //   BulletList/OrderedList/ListItem/Blockquote/
                                    //   HorizontalRule/HardBreak/Link/ListKeymap/Underline
  TaskList,
  TaskItem,
  Link.configure({
    openOnClick: false,
    HTMLAttributes: { rel: "noopener noreferrer" },
    validate: (url) => /^https?:|^mailto:/.test(url),  // 协议白名单
  }),
  Table,
]
```

**显式不支持**（通过不引入对应 extension 实现）：

- 图片（不引入 `@tiptap/extension-image`；粘贴图片由编辑器配置降级为链接文本或不处理）。
- 原生 HTML 内嵌：`@tiptap/markdown` 默认会把 markdown 中的内联 HTML 通过对应 node 的 `parseHTML` 渲染。本期**显式禁用** HTML 解析——通过 `Markdown` 扩展配置关闭 HTML 透传（`html: false`，具体选项名按 v3.27.x 实际 API 确认），保证 `<script>` 等标签以纯文本显示，不会被解析执行。
- 数学公式（不引入 KaTeX）。

### 3.4 工具栏与交互

`MarkdownEditor` 顶部工具栏提供：粗体、斜体、删除线、H1/H2/H3、无序列表、有序列表、待办列表、引用、代码、代码块、链接、表格、撤销/重做。

- 工具栏按钮使用 `lucide-react`（项目已用）。
- 快捷键沿用 tiptap 默认（`Cmd/Ctrl+B` 加粗等），与主流编辑器一致。
- `Cmd/Ctrl+Enter` 提交保存（沿用现有 Dialog 行为，由父组件监听）。
- 受控模式：父组件持有 markdown 字符串状态，`onChange` 实时上报。

### 3.5 渲染展示（MarkdownView，基于 static-renderer）

- 只读场景使用 `@tiptap/static-renderer` 的 `renderToReactElement`，输入为 markdown 字符串经 `@tiptap/markdown` parse 后的 ProseMirror JSON，extensions 复用 §3.3 的 `markdownExtensions`。
- 编辑态与只读态视觉一致：共享同一份 `prose` 样式类（手写最小 CSS，覆盖 `h1/h2/ul/ol/li/code/pre/blockquote/table/a`，不引入 `@tailwindcss/typography`）。
- **不再使用 `react-markdown` / `remark-gfm`**：统一 tiptap schema，避免双 parser 差异。
- 代码块本期最小自实现 `<pre><code>` 渲染（不引高亮库），如需语法高亮后续 milestone 再加。
- **链接安全**：static-renderer 输出的 `<a>` 由 Link 扩展的 `HTMLAttributes.rel/validate` 控制，`javascript:` 等非法协议在 parse 阶段即被 `Link.validate` 拒绝，不会进入 JSON。

## 4. 组件接口

### 4.1 MarkdownEditor

```tsx
type MarkdownEditorProps = {
  value: string                 // markdown 源码
  onChange: (markdown: string) => void
  placeholder?: string
  ariaLabel?: string
  minHeight?: number            // 默认 240
  className?: string
}
```

- 受控组件：`value` 变化时，仅当与编辑器当前序列化结果不一致时才 `setContent(md, { contentType: "markdown" })`，避免光标跳动与循环更新。
- 内部维护 `EditorContent`，所有 tiptap 状态不外泄。
- 不提供 `editable:false` 退化——只读场景统一用 `MarkdownView`，职责分离。

### 4.2 MarkdownView

```tsx
type MarkdownViewProps = {
  children: string              // markdown 源码
  className?: string
}
```

- 基于 `@tiptap/static-renderer`：把 markdown 字符串经 `@tiptap/markdown` parse 成 ProseMirror JSON，再用 `renderToReactElement` + 共享 `markdownExtensions` 渲染。
- 纯渲染，无可变状态，不实例化 editor。
- 空字符串返回 `null`，由调用方负责占位提示。

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

- **链接协议白名单**：`Link` 扩展配置 `validate: (url) => /^https?:|^mailto:/.test(url)` 与 `HTMLAttributes.rel = "noopener noreferrer"`，`javascript:` 等非法协议在 parse 阶段即被拒绝，不会进入 ProseMirror JSON，编辑态与 static-renderer 只读态都不会输出可点击的危险链接。
- **禁用内联 HTML**：`@tiptap/markdown` 默认会通过各 node 的 `parseHTML` 把 markdown 中的内联 HTML 当结构解析。本期显式关闭 HTML 透传（在 `Markdown` 扩展配置中设 `html: false`，具体选项名按 v3.27.x 实际 API 在实施时确认），保证 `<script>`、`<iframe>` 等标签以纯文本字符显示，不被解析执行。
- **不引入 Image 扩展**：粘贴/输入 `![]()` 不渲染为 `<img>`，避免通过图片 URL 触发请求或外链跟踪。
- **代码块**：仅 `<pre><code>` 渲染，不执行任何高亮脚本。
- **static-renderer 不挂载 editor**：只读渲染不实例化 ProseMirror editor，无编辑副作用、无 `contenteditable`，进一步缩小攻击面。

## 6. 测试策略

### 6.1 单元测试（vitest + testing-library）

- `markdown-editor.test.tsx`：
  - 受控 `value` 渲染后内容正确。
  - 输入触发 `onChange` 上报 markdown 字符串。
  - 工具栏粗体按钮对选区加粗，输出 `**text**`。
  - 表格/待办列表 round-trip：`value` 含 GFM 表格/`- [x]` 时，编辑器序列化结果与输入一致。
- `markdown-view.test.tsx`：
  - 渲染常见 markdown 元素（标题、列表、代码块、表格、待办列表、链接）。
  - 内联 HTML（如 `<script>alert(1)</script>`）以纯文本显示，不被解析。
  - `javascript:` 链接不渲染为可点击 `<a href>`。
  - 编辑器与只读渲染对同一份 markdown 输出 DOM 结构一致（共享 schema 的回归断言）。

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

- 描述：编辑含标题、列表、代码块、表格、待办列表的 markdown，保存后在详情页正确渲染（编辑态与只读态视觉一致）。
- 注解：新增一条含粗体+链接的注解，列表展示正确。
- 老数据兼容：已有纯文本描述（无 markdown 语法）正常显示为段落。
- 安全：描述中粘贴 `<script>alert(1)</script>` 与 `[x](javascript:alert(1))`，保存后查看源码与渲染均不触发执行。
- 面包屑：workspace、project 链接可跳转，task 段不可点。
- 移动端：三 Tab 切换正常，编辑器高度自适应。

## 7. 实施步骤（概览，详细 plan 由 writing-plans 拆分）

1. 引入依赖（v3）：`@tiptap/react`、`@tiptap/starter-kit`、`@tiptap/pm`、`@tiptap/markdown`、`@tiptap/static-renderer`、`@tiptap/extension-task-list`、`@tiptap/extension-task-item`、`@tiptap/extension-link`、`@tiptap/extension-table`。固定到 v3.27.x 系列。
2. 新增 `web/src/components/markdown/extensions.ts`（共享 schema，§3.3）。
3. 新增 `markdown-view.tsx`：基于 `@tiptap/static-renderer` + `@tiptap/markdown` 的只读渲染（先做，可独立合入）。
4. 新增 `markdown-editor.tsx`：基于 `@tiptap/react` 的 WYSIWYG 编辑器 + 工具栏，受控 markdown 字符串。
5. 替换 `TaskDescriptionBlock` 展示与编辑。
6. 替换 `TaskAnnotationsEditor` 新增/编辑/展示。
7. 详情页面包屑 + 栅格调整。
8. 更新现有测试 + 新增组件测试。
9. 手测清单验收。

## 8. 风险与权衡

| 风险 | 缓解 |
|---|---|
| v3 仍在快速迭代（v3.27.x），API 可能在小版本间调整 | 锁定到具体小版本；`@tiptap/markdown` 与 `@tiptap/static-renderer` 视为关键依赖，升级前跑全套测试 |
| `@tiptap/markdown` 官方扩展文档标注 Beta | round-trip 测试覆盖每种语法；生产路径不依赖未稳定 API |
| static-renderer 对部分扩展（表格嵌套、待办列表）渲染不完整 | 实施时针对表格/待办列表写专项回归测试；如确有缺陷，该语法降级为只读纯文本展示并记录到风险 |
| 包体积增长 | tiptap 按需引扩展；编辑器走 Vite code-split 懒加载（只在 Dialog 打开时加载），只读 static-renderer 体积小 |
| 注解文本纯文本历史被当 markdown 解析出现意外格式 | markdown 对纯文本宽容，多数情况渲染为段落；测试用例覆盖老数据 |
| 任务标题误用富文本 | 显式不覆盖标题，保留 `InlineTextEditor` |
| React 19 与 v3 集成边界问题 | 官方已支持；落地前先跑 `pnpm typecheck && pnpm build`，参考官方 React 集成示例 |

## 9. 文档同步

- `ROADMAP.md`：新增 v0.5.1「Web Console Tiptap Markdown 编辑器与 Task 详情页 UX 改进」。
- `README.md`：如用户可见行为变化（描述/注解现在支持 markdown 渲染），在 web console 段落补一句说明。
- 本 spec 通过后，用 `superpowers:writing-plans` 拆实施计划。
