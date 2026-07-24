# Web Console 应用「冷静的工程绿」运营控制台设计语言

**日期：** 2026-07-24
**状态：** 已实现
**范围：** 仅调整 Workspace Web Console 的视觉设计语言（颜色 token、字体、密度、组件姿态、外壳布局）；保留 shadcn/ui 作为组件底座，不替换组件库、不改业务逻辑、不改后端契约。

## 1. 背景与动机

仓库根目录的 `.skills/Project-As-Complete-Open-Design-Design` 是一套成熟、可复用的运营控制台设计系统（原为 GetRank 设计），核心是一种「冷静的工程绿」语言：

- 翡翠绿 `oklch(0.54 0.15 150)` 作为唯一品牌色；
- 深色侧栏 + 浅色画布的运营控制台骨架；
- 数据密集、信息优先、专业但不冷漠；
- Lucide 作为唯一图标集、JetBrains Mono 用于一切数字/ID/时间。

当前 Xuanchu Web Console 的设计现状：

- 技术栈健康：shadcn/ui（radix-lyra style）+ Tailwind v4 + lucide-react + Inter Variable，已用 `oklch()` 表达颜色，`primary` 也已落在绿系（`oklch(0.527 0.154 150.069)`）。
- 但与该设计系统相比仍有明显差距：
  1. **侧栏是浅色的**（`--sidebar: oklch(0.985 0 0)`），不是控制台骨架应有的深色侧栏；激活态只有 `border-l-foreground` 灰条，没有品牌绿的「3px 左边条 + 软绿底」决定性笔触。
  2. **绿色分散、不统一**：`primary` 绿之外，代码里散落 `text-emerald-600` / `bg-emerald-500/5` / `bg-amber-500` 等 Tailwind 原色，未收敛到 token；违反「绿是唯一品牌色、一屏至多两处主绿」。
  3. **字体只有 Inter 一族**：缺少 display 字体与 mono 字体。所有数字、ID、时间用比例字体，丢失数据密集场景的可读性与「工程感」。
  4. **密度姿态不清**：shadcn button 默认 `h-8`、`rounded-none`，icon `size-4`，整体偏小且直角，既不像紧凑数据控制台（36/34），也不够克制。
  5. **状态色滥用**：状态用大面积底色块（`bg-amber-500/5`、`border-amber-500/30`），而非设计系统规定的「仅 7px 圆点或小 chip」。
  6. **没有图表规范**：没有 KPI 卡 / 填充型趋势图 / 任务队列的统一形态。

目标：把这套「冷静的工程绿」语言完整、克制地落到 Web Console，让控制台具备专业运营工具的视觉气质，同时**严格保留 shadcn/ui 组件**，只在 token 层、布局层、少量封装组件层做调整。

## 2. 设计原则（硬规则，ship-blocker）

直接采纳设计系统的 6 条锁定姿态，并按 shadcn 语境改写：

1. **绿（`--accent`）是唯一品牌色**：主操作、激活导航、选中行、进度、链接使用；一屏至多两处主绿。状态色（success/warn/danger/info）**仅作 7px 圆点或小 chip**，绝不作大面积底色铺色。
2. **深色侧栏 + 浅色画布，单一主题**：默认不做全局深色模式；保留现有浅/深切换作为画布的可选暗色态，但侧栏恒为深色骨架。
3. **所有颜色都是 `oklch()` 并收敛进 `:root` token**：禁止在组件里写裸 hex、禁止散用 Tailwind 原色（`text-emerald-600` 等）。组件里只引用 token。
4. **紧凑数据密度**：主按钮 `h-9`（36px）/ 输入 `h-9`/图标按钮 `size-9` 为基准；表格行 40/32。**不要膨胀到 44px**。
5. **图标只用 Lucide**：`stroke-width` 默认 1.5–2，`currentColor`，16–18px 视觉盒。禁止 emoji 当功能图标。
6. **图表一律填充**（面积/柱/堆叠），不画裸线；面积图从 0 起算。

## 3. Token 层调整（`web/src/index.css`）

这是本次改动的核心，所有视觉变化都收敛到 token，不动组件结构。

### 3.1 字体

新增 display 与 mono 字体族，沿用 Inter 作 body：

```css
/* @theme inline 内 */
--font-sans: 'Inter Variable', 'Inter', system-ui, -apple-system, sans-serif;
--font-heading: 'Space Grotesk', 'Inter Variable', system-ui, sans-serif; /* display，替代 Geist 的工程几何无衬线 */
--font-mono: 'JetBrains Mono Variable', 'JetBrains Mono', ui-monospace, Menlo, monospace;
```

新增 npm 依赖（pnpm）：

- `@fontsource-variable/space-grotesk`（display 标题、品牌字、按钮）
- `@fontsource-variable/jetbrains-mono`（数字 / ID / 时间 / KPI 数值）

在 `index.css` 顶部 `@import`，与现有 `@fontsource-variable/inter` 并列。

> 选用 Space Grotesk 作 display：设计系统指定的 Geist 不在 Google Fonts / fontsource，Space Grotesk 是其官方渲染兜底，同为技术几何无衬线。自托管可平滑替换为 Geist。

### 3.2 颜色 token 重映射

把设计系统的 oklch 值映射到 shadcn 既有 token 名，**不改 shadcn token 名**（保持组件兼容），只改值：

| shadcn token | 现值 | 新值（对齐设计系统） | 说明 |
|---|---|---|---|
| `--primary` | `0.527 0.154 150.069` | `0.54 0.15 150` | 对齐 `--accent`（Geo Green ≈ #00863b） |
| `--primary-foreground` | `0.982 0.018 155.826` | `0.99 0 0` | 对齐 `--accent-on`（纯白） |
| `--background` | `1 0 0` | `0.99 0.002 150` | 画布：极淡的绿冷白 |
| `--foreground` | `0.145 0 0` | `0.24 0.01 155` | 主文字（绿冷炭） |
| `--card` | `1 0 0` | `0.985 0.003 150` | 卡片底 |
| `--muted` | `0.97 0 0` | `0.965 0.004 150` | 表头 / hover / 隔行（`--surface-2`） |
| `--muted-foreground` | `0.556 0 0` | `0.52 0.012 155` | 次文字 |
| `--border` | `0.922 0 0` | `0.92 0.004 150` | 标准分割 |
| `--input` | `0.922 0 0` | `0.86 0.006 150` | 输入框预聚焦边（`--border-strong`） |
| `--ring` | `0.708 0 0` | `0.54 0.15 150` | 焦点环用品牌绿 |
| `--destructive` | `0.577 0.245 27.325` | `0.58 0.20 25` | 对齐 `--danger` |

**侧栏 token（核心变化，改为深色骨架）**：

```css
--sidebar: 0.22 0.012 160;                 /* --side-bg 深绿炭 */
--sidebar-foreground: 0.93 0.008 150;       /* --side-fg */
--sidebar-muted-foreground: 0.62 0.014 155; /* --side-muted（新增 token，需在 @theme inline 注册 color 别名） */
--sidebar-primary: 0.54 0.15 150;           /* --side-active = 品牌绿 */
--sidebar-primary-foreground: 0.99 0 0;
--sidebar-accent: 0.30 0.05 155;            /* --side-active-soft 激活项软底 */
--sidebar-accent-foreground: 0.54 0.15 150; /* 激活项文字绿 */
--sidebar-border: 0.3 0.01 160;             /* --side-border */
--sidebar-ring: 0.54 0.15 150;
```

**新增语义 token**（在 `:root` 增补，供组件引用，shadcn 不内置）：

```css
--success: var(--primary);      /* success = 品牌绿 */
--warn: 0.78 0.14 70;           /* ≈ #d99417 */
--info: 0.54 0.12 240;          /* ≈ #0d76ad */
--accent-soft: 0.96 0.03 150;   /* 选中行 / 软徽章底 */
--side-hover: 0.27 0.012 160;   /* 侧栏分组/hover */
```

这些需在 `@theme inline` 注册 `--color-success` / `--color-warn` / `--color-info` / `--color-accent-soft` 别名，以便 Tailwind 生成 `bg-success` / `text-warn` 等工具类。

### 3.3 圆角

设计系统：卡片 10、按钮 8、chip 6、pill 9999。shadcn 现状 `--radius: 0.45rem`（7.2px），偏小且按钮 `rounded-none`。调整为：

```css
--radius: 0.625rem; /* 10px 卡片基准 */
/* @theme inline 内的 radius 派生保持，按钮/输入由组件 size 控制 */
```

button 组件的 `rounded-none` 改为按尺寸：默认 `rounded-md`（≈8px），icon 同。

### 3.4 密度尺寸

不引入额外 CSS 变量，直接在组件 cva 里落实（见 §4）。

## 4. 组件层调整（保留 shadcn，只调 cva）

### 4.1 `button.tsx`

- 默认 `size.default` 从 `h-8` 提到 `h-9`（36px），`px-3.5`；`size.sm` → `h-8`（32px）；新增/保留 `lg` → `h-10`。
- 去掉 `rounded-none`，默认 `rounded-md`。
- 主按钮（`default` variant）保持实底绿、无阴影（设计系统要求 primary 是 flat fill，无底部 inset/外阴影）。
- `destructive` 保留 `bg-destructive/10 text-destructive`（克制，符合「危险不作大面积实底」）。
- icon 默认 `size-9`、`size-4` 图标。

### 4.2 `badge.tsx` / 状态徽章

新增一个 `StatusBadge` 封装（放 `src/components/StatusBadge.tsx`），强制「7px 圆点 + 文字」形态，映射任务/文章/分发状态到语义色：

```
pending/draft  → meta（灰）
waiting        → warn
started/生成中 → info
completed/已发布 → success（绿）
failed/已退回   → danger
```

圆点用 `bg-success/warn/info/danger/meta`，文字用对应深色。**禁止**把状态色用作整块背景。

### 4.3 `AppShell.tsx`（外壳骨架，重点）

- **侧栏变深色**：`<aside>` 从 `bg-background` 改为 `bg-sidebar text-sidebar-foreground`，移除浅色 `border-r`，改 `border-r border-sidebar-border`。
- **侧栏宽度**：从 `w-56`（224px）提到 `w-64`（256px，贴近设计系统 248px）。
- **激活态决定性笔触**：当前 `border-l-foreground bg-muted`。改为 `bg-sidebar-accent text-sidebar-accent-foreground border-l-2 border-sidebar-primary`，并在文字/图标上加 `text-sidebar-primary`（绿），还原设计系统「3px 绿左边条 + 软绿底 + 绿字」。
- **分组标签**：`uppercase tracking-wide` 保留，改 `text-sidebar-muted-foreground`。
- **顶部 bar**：高度从 `h-12` 提到 `h-14`（56px），贴近设计系统 `--topbar-h`；保留刷新/语言/主题/身份块。
- **内容区**：`main` padding 从 `px-4 py-5` 调到 `px-6 py-5`，更贴近运营画布留白。
- **Logo 区**：品牌字用 `font-heading`（Space Grotesk），绿点缀（如「璇**础**」后半段或绿点 glyph），符合「wordmark = 中性 + 一抹绿」。

### 4.4 `table.tsx`（数据表）

- 表头：`text-xs uppercase tracking-wide text-muted-foreground bg-muted font-medium`（对齐「11px 大写 +0.05em」）。
- 行高：默认 `h-10`（40px），提供 `data-density="compact"` → `h-8`（32px）。
- ID / 数字 / 时间列：内容包 `<span className="font-mono tabular-nums">`。提供 `TableCell` 的 mono 变体或约定 class。
- hover 行：`hover:bg-muted`。

### 4.5 新增 KPI 封装（可选，按需）

设计系统的 KPI 卡（label + 26px mono 数值 + 同比箭头 + 74×24 填充 sparkline）。当前 Home 页 `MyTodaySection` 的计数网格已是雏形。本次：

- 把计数数字加 `font-mono tabular-nums text-2xl`。
- 后续若引入趋势，sparkline 用 SVG 面积填充（fill + stroke），数据驱动坐标，从 0 起。

## 5. 既有代码收敛（去裸色）

扫描并替换以下散用模式，统一到 token：

| 现状 | 替换为 |
|---|---|
| `text-emerald-600` / `bg-emerald-600` | `text-primary` / `bg-primary` |
| `bg-emerald-500/5` / `border-emerald-500/30` | `bg-accent-soft` / `border-primary/30` |
| `bg-amber-500` / `text-amber-600` / `bg-amber-500/5` | `bg-warn` / `text-warn` / `bg-warn/10`（且仅小面积） |
| `bg-destructive`（状态点） | `bg-destructive`（保留，作 dot） |
| 裸 `#hex` | 禁止 |

涉及文件（已定位）：`home-page.tsx`、`cron-schedule-input.tsx`、`project-template-instantiate-wizard.tsx`、`edit-feedback.tsx`、`assignee-workload-summary.tsx` 等约 10 处。逐个替换并验证视觉一致。

## 6. 约束与边界

**不改动**：

- 任何后端 API、数据模型、路由结构、权限逻辑。
- shadcn 组件库的引入方式（仍是 `npx shadcn add`，仍 `radix-lyra` style）。
- 业务组件的 DOM 结构与交互行为（只改 className / token / 字体 / 圆角 / 尺寸）。
- i18n 文案。

**保留兼容**：

- `--json` / stdout/stderr / CLI 行为完全不受影响（纯前端 `web/` 内改动）。
- `CGO_ENABLED=0` 与 Go 构建不受影响（前端只是被 embed 的产物）。
- 深色画布模式作为可选态保留（侧栏恒深色）。

## 7. 验收清单

- [ ] `:root` 全部颜色为 `oklch()`；组件内裸 hex = 0，Tailwind 原色（emerald/amber/red）= 0（destructive 除外）。
- [ ] 侧栏呈深绿炭骨架，激活项有绿左边条 + 软绿底 + 绿字，跨 1440/1024/768/390 不塌、无横向滚动。
- [ ] 顶部 bar 56px，主按钮 36px 实底绿无阴影，输入 36px，图标按钮 36px；未膨胀到 44px。
- [ ] 数字 / ID / 时间列使用 mono + `tabular-nums`；KPI 数值 mono。
- [ ] 状态色仅以 7px 圆点或小 chip 出现，无大面积底色铺色。
- [ ] 绿色（primary）每屏至多两处主绿；其余靠灰阶。
- [ ] 标题/品牌字使用 `font-heading`；图表若出现均为填充型。
- [ ] `pnpm --dir web typecheck`、`test`、`lint`、`build` 全绿。
- [ ] 既有视觉回归测试通过；新增 token/字体不破坏 `AppShell.test.tsx` 等快照。
- [ ] `git diff --check` 无空白错误。

## 8. 实施顺序（建议分提交）

1. **字体依赖 + token 重映射**（`index.css`、`package.json`）：只动 token 与字体 import，验证整站色调与字体平滑切换。
2. **`AppShell` 深色侧栏 + 激活态**：骨架视觉落地。
3. **`button.tsx` 密度与圆角**：控件姿态统一。
4. **`StatusBadge` 封装 + 状态色收敛**：替换散用的 emerald/amber。
5. **`table.tsx` mono 列 + 行高**：数据表密度。
6. **Home 页 KPI 计数 mono 化**（验证 KPI 形态）。
7. 全量回归（typecheck/test/lint/build）+ 视觉走查。

每步都可独立提交、独立回退。

## 9. 非目标

- 不引入图表库（趋势图等真正有数据需求时再评估，遵循「填充型、从 0 起」规范）。
- 不重写任何业务页面，不做信息架构调整。
- 不做多 workspace 切换器 UI（侧栏账户胶囊的完整形态留待后续，本次只把现有身份块迁到深色侧栏语境）。
- 不改组件库（不上 Radix Themes 原生 UI、不换非 shadcn 组件）。
- 不动 admin 子站（`features/admin`）的独立 shell，除非验收发现明显割裂，再单独评估。
