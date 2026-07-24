# Xuanchu Web Console 设计规范

> 本文件是 Xuanchu Web Console（嵌入式 Web Admin Console 前端，`web/`）的权威设计规范。
> 一句话系统：**冷静的工程绿** —— 翡翠绿唯一品牌色、深色侧栏 + 浅色画布的运营控制台骨架、数据密集、信息优先、专业但不冷漠。

所有在 `web/` 内工作的代理和人协作者，构建任何控制台界面（表格、表单、卡片、外壳、图表）前都应先读本规范。Token 绑定见 `web/src/index.css`。

---

## 0. 适用范围与约束

- **适用面**：Workspace Web Console（`web/src`）与 Admin 子站（`web/src/features/admin`）的全部运营界面。
- **技术栈**：React + TypeScript + Tailwind v4 + shadcn/ui（`radix-lyra` style）+ lucide-react + i18next。
- **核心约束**：保留 shadcn/ui 作为组件底座，**不替换组件库**。所有视觉变化收敛到 token 层（`web/src/index.css` 的 `:root`）、组件 cva、外壳布局，不复制业务逻辑。
- **字体**：Inter（body / 表格）+ Space Grotesk（display，标题/品牌/按钮）+ JetBrains Mono（数字 / ID / 时间 / KPI 数值）。

---

## 1. 锁定姿态（硬规则，ship-blocker）

这 6 条是设计系统的锁定决策，违反即视为不合规：

1. **绿（`--primary`）是唯一品牌色**。用于主操作、激活导航、选中行、进度、链接。**一屏至多两处主绿**。状态色（success/warn/danger/info）**仅以 7px 圆点或小 chip 出现，绝不作大面积底色铺色**。
2. **深色侧栏 + 浅色画布，单一主题**。侧栏恒为深色骨架，不随画布明暗切换；`.dark` 提供可选暗色画布态，用户保留 light/dark/system 切换。
3. **所有颜色都是 `oklch()` 并收敛进 `:root` token**。禁止组件内裸 hex、禁止散用 Tailwind 原色（`text-emerald-600`、`bg-amber-500` 等）。组件里只引用 token（`text-primary`、`bg-warn` 等）。
4. **紧凑数据密度**。主按钮 `h-9`（36px）、输入 `h-9`、图标按钮 `size-9`；表格行 40px。**不要膨胀到 44px** —— 那会破坏数据密度姿态。
5. **图标只用 Lucide**。`stroke-width` 默认 1.5–2，`currentColor`，16–18px 视觉盒。禁止 emoji 当功能图标，禁止手写 SVG 图标。
6. **图表一律填充**（面积/柱/堆叠），不画裸线；面积图从 0 起算，坐标由数据驱动，不目测。

---

## 2. 颜色 Token

全部颜色为 `oklch()`，定义在 `web/src/index.css` 的 `:root`，组件通过 Tailwind 工具类引用（`bg-primary`、`text-muted-foreground` 等）。下表给出语义、oklch 值与角色。

### 2.1 品牌 / 语义

| Token（shadcn 名） | oklch | ≈ hex | 角色 |
|---|---|---|---|
| `--primary` | `0.54 0.15 150` | `#00863b` | **Geo Green** —— 唯一品牌色，主 CTA / 激活 / 链接 / 进度；AA 配 `--primary-foreground` |
| `--primary-foreground` | `0.99 0 0` | `#ffffff` | 绿色上的文字 |
| `--destructive` | `0.58 0.20 25` | `#d73337` | 失败 / 拒绝 / 删除；AA 可作文字 |
| `--success` | `var(--primary)` | — | 成功 = 品牌绿（命中为正） |
| `--warn` | `0.78 0.14 70` | `#d99417` | 警告 / 待审 |
| `--info` | `0.54 0.12 240` | `#0d76ad` | 信息 / 进行中 |
| `--accent-soft` | `0.96 0.03 150` | `#e7f4ee` | 选中行 / 软徽章底 |

> `--success` / `--warn` / `--info` / `--accent-soft` 已在 `@theme inline` 注册别名，可用 `bg-success`、`text-warn`、`bg-accent-soft` 等。

### 2.2 画布 / 表面（浅色，带极淡绿冷）

| Token | oklch | ≈ hex | 角色 |
|---|---|---|---|
| `--background` | `0.99 0.002 150` | `#fbfcfb` | 画布（极淡绿冷白） |
| `--foreground` | `0.24 0.01 155` | `#2a2f2d` | 主文字（绿冷炭） |
| `--card` | `0.985 0.003 150` | `#f6f8f6` | 卡片 / 面板底 |
| `--muted` | `0.965 0.004 150` | `#eef1ef` | 表头 / hover / 隔行（surface-2） |
| `--muted-foreground` | `0.52 0.012 155` | `#6b7470` | 次文字 |
| `--border` | `0.92 0.004 150` | `#e2e6e3` | 标准分割 / 卡片边 |
| `--input` | `0.86 0.006 150` | `#cdd3cf` | 输入框预聚焦边 |
| `--ring` | `0.54 0.15 150` | `#00863b` | 焦点环（品牌绿） |

### 2.3 侧栏（深色骨架，恒深色）

| Token | oklch | ≈ hex | 角色 |
|---|---|---|---|
| `--sidebar` | `0.22 0.012 160` | `#1c2624` | 侧栏底（深绿炭） |
| `--sidebar-surface` | `0.27 0.012 160` | `#27332f` | 侧栏浮层 / hover 面板（比底亮一档的中性深色） |
| `--sidebar-foreground` | `0.93 0.008 150` | `#e6ebe8` | 侧栏主文字 |
| `--sidebar-muted-foreground` | `0.62 0.014 155` | `#8a948f` | 侧栏次文字 / 分组标签 |
| `--sidebar-primary` | `0.54 0.15 150` | `#00863b` | 激活项文字 + 左边条 |
| `--sidebar-accent` | `0.30 0.05 155` | `#24403a` | 激活项软底（带绿色调） |
| `--sidebar-border` | `0.30 0.01 160` | — | 侧栏内部分割 |

**状态色只作 dot/chip**：success/warn/danger/info 仅以 7px 圆点或小徽章出现，绝不大面积铺色。

---

## 3. 字体与字号

三族字体，在 `@theme inline` 注册为 `--font-heading` / `--font-sans` / `--font-mono`，依赖 `@fontsource-variable` 自托管。

| 角色 | 字体 | Tailwind 别名 | 用途 |
|---|---|---|---|
| Display | Space Grotesk | `font-heading` | 标题、导航、按钮、品牌字 |
| Body | Inter | `font-sans`（默认） | 正文、表格（数据可读性最佳） |
| Mono | JetBrains Mono | `font-mono` | **一切 ID / 数字 / 时间 / KPI 数值**，`tnum` 开启 |

### 字号阶梯（数据密集，比消费级更紧）

| 角色 | 字号 / 行高 / 字重 | 说明 |
|---|---|---|
| 页面标题 H1 | 24 / 1.25 / 600 | `font-heading`，`letter-spacing -.01em` |
| 分区标题 H2 | 18 / 1.3 / 600 | |
| 卡片标题 H3 | 15 / 1.35 / 600 | `font-heading` |
| 正文 | 14 / 1.5 / 400 | |
| 说明文字 | 12 / 1.4 / 500 | 次文字 |
| 表头 / 微标签 | 12 / 1.3 / 600 | **大写、`+0.05em` 字距**（uppercase tracking） |
| KPI 数值 | 26 / 1.1 / 600 | **mono**，`letter-spacing -.01em` |

**数字纪律**：所有数字、ID、时间、百分比、KPI 指标，统一 `font-mono` + `tabular-nums`（等宽数字），不要用比例字体。

---

## 4. 间距与密度

4px 基础刻度：`--space-1..10` = 4 / 8 / 12 / 16 / 20 / 24 / 32 / 40。

- **布局节奏**：侧栏 248px；顶栏 56px（`h-14`）；内容区 padding `px-4 py-5 md:px-6`；网格间距 14px。
- **卡片**：padding 14–16px；圆角 10px（`rounded-lg`）；1px `border` 边。
- **控件密度（紧凑，不是消费级）**：主/标准按钮 36px（`h-9`）、输入 36px（`h-9`）、图标按钮 36px（`size-9`）。表格行 40px 标准 / 32px 紧凑切换。
- **圆角**：卡片 10px（`rounded-lg` = `--radius`）· 按钮 8px（`rounded-md`）· chip 6px · pill 9999px · logo 7px。**全站卡片/表格容器统一 `rounded-lg`，禁止混用 `rounded-xl`/直角**。
- **阴影**：主按钮是 flat 实底绿，**无底部 inset、无外阴影**；按下可 `translateY(1px)` 但不加阴影。卡片 hover 可轻微 `shadow-card-hover`。其余装饰阴影一律禁止。

---

## 5. 布局与组合

### 5.1 应用骨架（AppShell / AdminShell）

- CSS Grid：`248px minmax(0,1fr)`，全高。侧栏 `sticky`、内部滚动；主列 `flex-col`；顶栏 `sticky top-0 z-20`；内容滚动。
- **桌面侧栏 248px**（`w-[248px]`），`md:pl-[248px]` 让出空间。
- 移动端（<md）侧栏收为抽屉（`Sheet`），**抽屉也用深色侧栏样式**（`bg-sidebar text-sidebar-foreground border-sidebar-border`），与桌面一致。

### 5.2 侧栏导航

- 分组项（个人 / 管理 / 系统），分组标签 `uppercase tracking-wide text-sidebar-muted-foreground`。
- 导航项 36px 高，`rounded-md`，hover `bg-sidebar-accent/50`。
- **激活态 = 系统唯一决定性笔触**：软绿底（`bg-sidebar-accent`）+ 绿字（`text-sidebar-accent-foreground`）+ **`::before` 3px 品牌绿左边条**。通过语义类 `is-nav-active` + CSS `::before` 实现（不依赖具体颜色类名，便于测试）。

### 5.3 账户切换器（侧栏底部）

参考 `.ws-trigger` / `.ws-pop` 形态：
- **触发器**：横向一行 `[工作空间 tile（首字母）] [工作空间名 + 角色] [用户头像（绿渐变 + 首字母）] [chevron]`。
- **浮层**（DropdownMenu，`side="top"`、深色 `bg-sidebar-surface`、向上展开）：当前用户信息块（头像 + 显示名 + 邮箱 mono）→ metadata（tokenType / workspaceSlug）→ 危险操作（返回超管、登出，用 `variant="destructive"`，文字与图标同为克制红）。

### 5.4 顶栏（56px）

面包屑（`--muted` + 当前项 `--fg`）+ 右侧操作（刷新、语言、主题切换、图标按钮）。`≤760px` 隐藏搜索、显示 hamburger。

### 5.5 内容区模式

- 页面头（标题 + 副标题 + 右对齐控件）。
- KPI 行（5 列 → 3 → 2 → 1 跨断点）。
- 卡片网格、趋势图（2 列 → 1）、下方分区（表格 + 队列）。

### 5.6 响应式断点（任何断点都不允许横向滚动）

1280 / 1024 / 760 / 420。侧栏 ≤1024px 折叠为 72px 图标条；≤760px 变抽屉 + scrim。KPI 网格 5→3→2→1。

---

## 6. 组件规范

### 6.1 按钮（`button.tsx`）

- primary（`default`）= `bg-primary text-primary-foreground`，**flat 实底绿，无阴影**，hover `bg-primary/90`，按下 `translate-y-px`。
- ghost（`outline`）= bordered，`bg-background`。
- destructive = 克制红文字（`variant="destructive"`），hover 红调暗底；**不作大面积实底红**。
- icon-only = 36px（`size-9`）。
- 圆角 `rounded-md`（8px），不再 `rounded-none`。

### 6.2 状态徽章（StatusBadge）

强制「7px 圆点 + 文字」形态，颜色仅落在圆点上：

```
draft/pending   → muted（灰）
waiting         → warn
started/进行中   → info
completed/已发布 → success（绿）
failed/已退回   → danger
```

禁止用状态色作整块背景。循环来源用单个 `Repeat` 图标（size-3）+ `title` 提示规则，不渲染「循环 · 每天」文字 badge。

### 6.3 数据表（`table.tsx`）

- **表格容器不可手写**：直接用 `<Table>`，容器样式（`overflow-x-auto rounded-lg border bg-card`）由 shadcn Table 组件内置（`card` prop 默认 true）。页面禁止再手写 `<div class="border bg-card">` 包裹表格。
- 表头：`text-xs uppercase tracking-wide text-muted-foreground bg-muted`，**粘性**。
- 行：标准 40px（`h-10`），hover 高亮 `bg-muted/50`。首列批量勾选，末列行操作。
- **ID / 数字 / 时间列用 `font-mono tabular-nums`**。
- **避免横向滚动**：各列用 `min-w-0` + `max-w-*` + `truncate` 弹性收缩（覆盖 table-cell 默认 `min-width: auto`），让文字列优先压缩，操作列 `whitespace-nowrap` 始终右贴边可见。`overflow-x-auto` 仅作极端窄屏兜底。

### 6.4 KPI 卡

标签（12/600 `--muted-foreground`）+ 26px mono 数值 + 同比箭头（up=success / down=danger / flat=muted）+ 填充型 sparkline（74×24，面积 + 描边，绝不裸线）。数值/单位/百分比统一 mono + tnum。

### 6.5 表单

- label 在上方（12/600）；输入 `h-9`，聚焦 = `--ring` 绿色焦点环；错误态红描边 + 说明文字。
- switch 开启 = `--primary`。
- 所有控件保持紧凑密度，不上 44px。

### 6.6 卡片 / 面板

全站卡片/面板容器统一 `rounded-lg border bg-card`（10px）。错误态/空态/详情面板/工具栏同理。禁止 `rounded-xl`/`rounded-none` 混用。

### 6.7 抽屉 / 模态

详情抽屉右侧；批量操作前用确认模态。

---

## 7. 交互与动效

- 过渡短而功能性：nav/hover `background .15s, color .15s`；主按钮按下 `translateY(1px)` + 阴影减弱；抽屉 `transform .2s ease`。
- 焦点：内容区绿色 2px 环（`--ring`）。
- 尊重 `prefers-reduced-motion`，保持过渡克制、非必要。
- 交互态必须真实：hover 边框加深、激活导航点亮、分段切换、移动抽屉带 scrim 滑入。无纯装饰动画。

---

## 8. 文案与品牌

- **语气**：专业、精准、数据优先；冷静不冷漠；工程级而非营销腔。中文（zh-CN）默认。
- **文案风格**：短标签、流程用箭头（`配置 → 生产 → 审核 → 分发 → 分析`）、数字/ID/时间一律 mono。技术术语小写（`pgvector`、`Worker-1`、`GPT-4o`）。
- **大小写**：中文无大小写；拉丁标签仅表头/微标签用大写 + 字距。
- **品牌字**：`font-heading`，可配一抹绿点缀（如「璇**础**」后半段），符合「wordmark = 中性 + 一抹绿」。

---

## 9. 反模式（不得上线）

1. **禁止 `:root` 外的裸 hex** —— 所有颜色经 token。
2. **禁止一屏超过两处主绿** —— 绿是唯一品牌色，状态色只作 dot/chip。
3. **禁止裸线图表** —— 图表必须填充。
4. **禁止 emoji 当功能图标、禁止手写或混用非 Lucide 图标**。
5. **禁止消费级 44px 触控膨胀** —— 保持紧凑密度（36/36/36）。
6. **禁止散用 Tailwind 原色**（`emerald`/`amber`/`red` 等），统一到 token。
7. **禁止手写表格容器** —— 用 shadcn `<Table>` 组件。
8. **禁止全站圆角混用** —— 卡片/表格统一 `rounded-lg`。
9. **禁止任何断点出现横向滚动**（360/390/430/600/768/1024/1366/1440/1920）。
10. **禁止产品界面内放设计/演示控件、假数据** —— 缺值用诚实占位符。

---

## 10. 验收清单

- [ ] 所有颜色为 `oklch()` 且收敛进 `:root`；组件内裸 hex = 0，Tailwind 原色 = 0。
- [ ] 绿（`--primary`）每屏至多两处主绿；状态色仅作 dot/chip。
- [ ] 深色侧栏 + 浅色画布骨架在 1440 / 1024 / 768 / 390 不塌、无横向滚动。
- [ ] 数据表用 shadcn `<Table>`（无手写容器）；ID/数字/时间列 mono；行 40px；窄屏弹性收缩不溢出。
- [ ] 所有图表填充型；面积图从 0 起。
- [ ] 所有图标为 Lucide（`stroke-width` 1.5、`currentColor`）；零 emoji、零手写图标。
- [ ] 控件紧凑（按钮 36 / 输入 36 / 图标 36），未膨胀到 44px。
- [ ] 卡片/表格容器圆角统一 `rounded-lg`。
- [ ] 数字 / ID / 时间用 `font-mono tabular-nums`；KPI 数值 mono。

---

## 附：Token 落点

权威 token 定义在 `web/src/index.css`：

- `@theme inline`：注册 Tailwind 颜色/字体别名（`--color-primary`、`--font-heading` 等）。
- `:root`：浅色画布 + 深色侧栏的全部 token 值。
- `.dark`：可选暗色画布态（侧栏保持深色，略加深以与画布区分）。

新增任何颜色/字体/间距需求，先加 token，再在组件引用，不要在组件内写裸值。改动 token 后跑 `pnpm --dir web typecheck && pnpm --dir web lint && pnpm --dir web build && pnpm --dir web test` 确认全绿。
