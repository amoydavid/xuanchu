# Web Console 自动化页面三处体验修复

- 日期：2026-07-27
- 类型：前端体验修复（Web Console）
- 涉及路由：`/workspaces/:ws/projects/:project/automations`、`/workspaces/:ws/automations`

## 1. 背景

排查发现自动化两个页面存在三处体验缺陷：

1. **项目自动化页面缺少启动/暂停入口**：`/workspaces/:ws/projects/:project/automations` 每行只有「编辑 / 立即测试 / 删除」三个按钮，无法切换规则启用状态（事件触发和定时任务皆无）。后端 `enableProjectAutomationRule` / `disableProjectAutomationRule`（`project-automations-api.ts:150-156`）已就绪，纯前端漏接。
2. **工作区自动化「指令摘要」列未截断**：`workspace-automations-page.tsx:269` 虽用 `line-clamp-1`，但 `<TableCell>` 无 `max-w` 约束，列被 `instruction_template` 全文撑开，把所有内容放出来了，违反 DESIGN.md「避免横向滚动」。
3. **投递中状态点无动画反馈**：`DeliveryStatusDot` 在 `delivering` 状态只是静态的 `bg-primary/60`，用户看不出「正在投递」。且项目页 `automation-delivery-list.tsx:44` 干脆渲染裸 `row.status` 字符串（连中文翻译都没有），动画无处可加。

## 2. 目标与非目标

### 目标

- 项目自动化页面每条规则提供两处可见的启用/停用入口（状态列 Switch + 操作列文字按钮）。
- 工作区「指令摘要」列单行截断 + 完整内容悬停可见。
- 投递中状态点呈现克制、可感知的呼吸动画，覆盖项目页与工作区页所有投递状态点位置。
- 全部满足 DESIGN.md 约束（状态色只作小圆点、尊重 `prefers-reduced-motion`、表格不横向滚动）。

### 非目标

- 不把项目页面原生 `<table>` 改造为 shadcn `<Table>`（超出本次范围，单独立项）。
- 不调整后端 API、自动化执行链路、provider 配置。
- 不动 workspace 页面已有的下拉菜单「启用/停用」（保留为既有入口）。
- 不调整「立即测试」「删除」等其他操作。

## 3. 设计方案

### 3.1 项目自动化页面：启动/暂停入口

**入口形态**（两条等价入口，都调用同一个 toggle）：

1. **状态列改为可点 Switch**：把 `project-automations-page.tsx:137` 的纯文本 `rule.enabled ? "启用" : "停用"` 替换为：
   ```
   <RuleStatusDot enabled={rule.enabled} /> + <Switch checked={rule.enabled} onCheckedChange={...} disabled={writeDisabled} />
   ```
   - `RuleStatusDot` 从 `@/features/workspace/automations/shared/automation-status` 引入，与 workspace 页对齐。
   - Switch 组件已存在：`web/src/components/ui/switch.tsx`，Radix 实现，`h-5 w-9`，`data-checked:bg-primary`。
2. **操作列新增「启用/停用」文字按钮**：在「编辑」与「立即测试」之间插入一个 ghost 按钮，文案随 `rule.enabled` 切换。

**数据层**：

- 新增 mutation hook `useToggleProjectAutomationRule(projectSlug)`，封装 `enableProjectAutomationRule` / `disableProjectAutomationRule` 二选一。
- 参考 workspace 版 `useToggleWorkspaceAutomationRule`（`workspace-automations-api.ts:211-225`）的形态。
- mutation 入参：`{ ruleId: string; enable: boolean }`。
- 成功后 `invalidateQueries({ queryKey: ["project", projectSlug, "automations"] })`（与现有 `invalidateRules` 一致），并刷新投递列表 key。
- 失败用现有 `useEditFeedback().failure(...)` 提示。
- pending 时禁用对应按钮（参考 workspace 的 `toggle.isPending && toggle.variables?.ruleId === rule.id` 判定模式）。

**权限**：与现有按钮一致，受 `writeDisabled`（= `layout.closed`）控制。项目关闭时 Switch 与按钮都 disabled。

### 3.2 工作区自动化：指令摘要列截断

**根因**：`<TableCell>` 默认 `min-width: auto`，`line-clamp-1` 只能在有宽度的盒子里生效。修复：

- 指令摘要列单元格（`workspace-automations-page.tsx:268-270`）改为：
  ```
  <TableCell className="min-w-0">
    <span className="block truncate text-muted-foreground" title={rule.instruction_template}>
      {rule.instruction_template}
    </span>
  </TableCell>
  ```
  - `min-w-0` 覆盖 table-cell 默认 `min-width: auto`。
  - `block truncate` 让 `<span>` 成为块级、单行省略。
  - `title` 提供原生 tooltip 显示全文。
- 「触发器」列（第 265-267 行、表头第 241 行）保持 `w-[180px]` 不变，但内层 `<span>` 同样补 `min-w-0 block truncate`，避免长 cron 表达式撑开。
- 移动端 card 版的指令摘要不受影响（保持现状）。

**验证**：在 360/768/1366 三个断点下，指令摘要列单行省略、hover 显示全文、整表无横向滚动。

### 3.3 投递中状态点动画

**动画形态**：opacity 呼吸（pulse），约 1.4s 一个循环。

**实现位置**：`web/src/features/workspace/automations/shared/automation-status.tsx`

- `deliveryDotColor`（第 28-41 行）的 `delivering` 分支，在 `bg-primary/60` 基础上追加一个语义化 class `animate-automation-delivering`：
  ```
  case "delivering":
    return "bg-primary/60 animate-automation-delivering"
  ```
- 其它状态（succeeded / dead_lettered / retry_wait / 默认）不加动画。

**keyframes 定义**（`web/src/index.css`）：

- 新增 `@keyframes automation-delivering`：`opacity` 在 0.4 → 1.0 → 0.4 之间循环。
- 新增 utility `.animate-automation-delivering { animation: automation-delivering 1.4s ease-in-out infinite; }`。
- 不用 Tailwind 内置 `animate-pulse`：它的时长（2s）和 opacity 范围（0.5→1）对 7px 小点不够明显，自定义更可控。
- `@media (prefers-reduced-motion: reduce)` 下 `.animate-automation-delivering { animation: none; }`（满足 DESIGN.md 第 7 节）。

**覆盖范围补全**（重要）：

项目自动化页面 `automation-delivery-list.tsx:44` 当前是裸 `<td className="p-2">{row.status}</td>`，既无翻译也无状态点。改为：

```
<td className="p-2">
  <div className="flex items-center gap-2">
    <DeliveryStatusDot status={row.status} />
    <span>{deliveryStatusLabel(row.status)}</span>
  </div>
</td>
```

`DeliveryStatusDot` 和 `deliveryStatusLabel` 从 `@/features/workspace/automations/shared/automation-status` 引入。这样投递中动画在项目页投递列表也能看到。

**覆盖的所有调用点**（动画通过 `deliveryDotColor` 单点生效，自动覆盖）：

| 位置 | 文件 | 行号 |
|---|---|---|
| workspace 投递表格行 | `workspace-automations-page.tsx` | 559 |
| workspace 投递卡片 | `workspace-automations-page.tsx` | 626 |
| workspace 投递详情抽屉 | `workspace-automation-delivery-detail.tsx` | 75 |
| 项目页投递列表（本次改造） | `automation-delivery-list.tsx` | 44 |

## 4. DESIGN.md 合规性

| 规范条目 | 本次落地 |
|---|---|
| 状态色只作 7px 圆点 / 小 chip | 启用/停用通过 Switch + 圆点 + 文字按钮，无大面积铺色 |
| 避免横向滚动 / 列 `min-w-0 + truncate` | 指令摘要列补 `min-w-0 + block truncate + title` |
| 动画克制、尊重 `prefers-reduced-motion` | 投递中动画 1.4s，`reduced-motion` 下禁用 |
| 状态色色相（success=绿 / danger=红 / warn=琥珀 / info=蓝） | 沿用现有 `deliveryDotColor`，不新增色相 |
| 字体（数字/ID/时间 mono） | 不涉及新增字段 |

## 5. 跨目录依赖说明

本次会在 `project-workbench` 模块新引入对 `automations/shared` 的依赖：

- `project-automations-page.tsx` 引入 `RuleStatusDot`。
- `automation-delivery-list.tsx` 引入 `DeliveryStatusDot` + `deliveryStatusLabel`。

当前 `workspace-automations-api.ts:10-17` 已经反向依赖 `project-workbench/automations/project-automations-api` 的类型，所以这是可接受的对称引用，`shared` 子目录就是为跨模块共享而设。**不**新增循环依赖（`project-workbench` 引 `automations/shared`，`automations/shared` 不引 `project-workbench`）。

## 6. 验收标准

1. 项目自动化页面：
   - 每行状态列显示 `RuleStatusDot` + Switch，Switch 点击立即切换启用态并刷新列表。
   - 操作列「启用/停用」按钮文案随状态切换，点击后状态同步。
   - 项目关闭时（`writeDisabled=true`）Switch 和按钮均 disabled。
   - toggle 进行中按钮 disabled，完成后恢复。
2. 工作区自动化页面：
   - 「指令摘要」列单行截断，hover 显示完整 `instruction_template`。
   - 360/768/1366 断点下表格无横向滚动。
3. 投递中动画：
   - 项目页投递列表、workspace 投递表格行/卡片/详情抽屉四处，投递中状态点呈呼吸动画。
   - 其它状态（成功/失败/等待重试）无动画。
   - `prefers-reduced-motion: reduce` 下动画消失。
4. `pnpm --dir web typecheck && lint && build && test` 全绿。

## 7. 风险与回退

- **风险**：Switch 在原生 `<table>` 的 `<td>` 内高度对齐可能需要微调。缓解：状态列内用 `flex items-center gap-2` 包裹圆点 + Switch。
- **风险**：自定义 keyframes 名字与第三方冲突概率低（前缀 `automation-delivering` 已足够特化）。
- **回退**：三处改动彼此独立，可分别 revert。

## 8. 修订记录（2026-07-27 实现后回炉）

初版实现后用户反馈两个问题，本节记录根因与修订。

### 8.1 指令摘要列截断未生效

**现象**：工作区页面改完后，浏览器里指令摘要列仍然很长、没有省略号。

**根因**：shadcn `<Table>` 与项目页原生 `<table>` 都是默认 `table-layout: auto`，列宽由内容撑开。在 auto 布局下，table-cell 的 `min-w-0` 与子元素的 `truncate` 都**无法触发省略号**——因为列宽本身就被内容撑开了，子元素 `clientWidth === scrollWidth`。我用 playwright 实测：auto 布局下指令摘要列 `clientWidth=904, scrollWidth=904, truncated=false`，整张表被撑到 1974px（超出 1280 视口）。

**修订**：给两处表格都加 `table-layout: fixed`（Tailwind `table-fixed`），并用 `<colgroup>` 定义列宽比例。fixed 布局下列宽由 `<col>` 决定、不被内容撑开，`truncate` 才真正生效。实测：fixed 布局下指令摘要列 `clientWidth=486, scrollWidth=904, truncated=true`。列宽分配：状态 48px / 触发器 180px / 最近运行 120px / 操作 48px 为固定，名称与指令摘要为弹性列。

**验证手段**：新增 `web/scripts/verify-truncate-css.mjs`，用 playwright 渲染 auto 与 fixed 两张表，断言 `scrollWidth > clientWidth`。jsdom 无法验证布局（clientWidth/scrollWidth 恒为 0），所以截断类视觉回归必须用真实浏览器测量。

### 8.2 项目页启停入口冗余

**现象**：初版给项目页同时加了「状态列 Switch」和「操作列启用/停用文字按钮」，两个入口干同一件事，视觉吵闹。用户指出优秀数据表格不应这样设计。

**根因**：设计决策时把「可见性」和「操作密度」混在一起，没有遵循「高频外露、低频收起」的原则。

**修订**：去掉 Switch，状态列只保留 `RuleStatusDot`；操作列的所有行操作（编辑 / 启用-停用 / 立即测试 / 删除）收进一个 `⋯` 下拉菜单（项目页新增 `ProjectRuleActions` 组件，结构与工作区页 `RuleActions` 一致，仅多一个「立即测试」项）。这同时让两个页面的行操作交互完全统一。

**收益**：状态列一眼看到启用态，操作列只有一个 `⋯` 不抢戏，与工作区页一致。

### 8.3 对验收标准与实现计划的同步影响

- 验收标准 1「状态列 Switch」作废，改为「状态列状态点 + 操作收进 ⋯ 菜单，菜单含启用/停用项」。
- 实现计划 Task 3 的 Switch 步骤作废，替换为 `ProjectRuleActions` 下拉菜单。
- 指令摘要截断的验收从「检查 className 含 truncate」升级为「playwright 测量 `scrollWidth > clientWidth`」。

