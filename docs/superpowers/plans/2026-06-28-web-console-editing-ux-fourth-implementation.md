# Web Console 编辑体验第四轮 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在已具备 project/task 基础编辑能力之后，补齐失败重试、行级操作可发现性、移动端头部拥挤和 dialog 焦点恢复，让编辑体验更接近成熟项目管理工具。

**Architecture:** 不新增后端资源面，优先复用当前 `EditFeedbackProvider`、inline editor、picker、dialog 和 task mutation hooks。组件继续按职责拆到 `task-detail/fields/`、`task-detail/pickers/`、`task-detail/dialogs/` 等子目录，减少详情页单文件复杂度。

**Tech Stack:** React 19、TanStack Query/Router、Radix/shadcn UI、Vitest、Playwright smoke。

---

## 当前基线

本计划建立在第三轮完成后的状态上：

- 任务详情支持标题、描述、优先级、日期、负责人、标签、依赖、UDA、链接、注解编辑。
- 负责人、标签、依赖已是 picker，不再要求手写逗号。
- 失败有字段级错误和页面级失败汇总；成功反馈已避开 sticky header。
- 移动详情页已有 `属性 / 注解 / 链接` tabs。
- `pnpm --dir web run smoke:editing` 已覆盖桌面和 375px 移动视口。

下一轮不做：

- 看板。
- 拖拽排序。
- 批量编辑。
- 富文本编辑器。
- 项目成员管理。

## ASCII 原型

### A. 失败汇总可定位

```text
+--------------------------------------------------------------+
| 2 个编辑未保存                                      [全部关闭] |
| - 负责人：permission_denied                  [定位] [关闭]    |
| - 链接 URL：link_duplicate                   [定位] [关闭]    |
+--------------------------------------------------------------+

点击 [定位]：

+--------------------------------------------------------------+
| 负责人                                                       |
| [alice x] [+]                                                |
| 保存失败：permission_denied                                  |
+--------------------------------------------------------------+
```

交互要点：

- 失败条目携带 `targetId`，定位时滚动并聚焦对应控件。
- 没有可定位目标时只显示关闭按钮。
- 页面级汇总不实现自动重试，避免重复提交过期草稿；重试仍在字段原位完成。

### B. 行级任务操作菜单

```text
桌面行：

+------+------------+---------+------+----------+------------+------+
| ID   | 标题       | 状态    | P    | 负责人   | 截止       |  ... |
+------+------------+---------+------+----------+------------+------+
| ads1 | 投放日报   | pending | H    | alice    | 07/03/2026 |  ... |
+------+------------+---------+------+----------+------------+------+

点击 ...：

+----------------+
| 打开详情       |
| 复制任务链接   |
| 删除任务       |
+----------------+

移动卡片：

+------------------------------------+
| ads-1                       pending |
| 投放日报 ✎                          |
| [H v] [07/03/2026]                  |
| alice                               |
|                       [▶] [✓] [...] |
+------------------------------------+
```

交互要点：

- `...` 使用菜单而不是裸图标堆叠，减少移动端误触。
- 删除仍走 destructive confirm。
- 复制链接用统一成功反馈，不在卡片里新增永久文案。

### C. 移动端头部压缩

```text
当前问题：375px 下 header 控件过多，工作区名、刷新、语言、主题、退出拥挤。

目标：

+------------------------------------+
| acme              [刷新] [更多]     |
| Alice · workspace                   |
+------------------------------------+

点击 [更多]：

+----------------+
| English        |
| Theme          |
| Log out        |
+----------------+
```

交互要点：

- 桌面仍保持当前完整 header。
- 移动端将低频项放入菜单，刷新保留为直接操作。
- acting mode 的“返回超管界面”保留为显性按钮或菜单首项，不能隐藏语义。

### D. Dialog 焦点恢复

```text
打开前：

负责人 [alice x] [+]
                  ^
                  focus

Dialog 关闭后：

负责人 [alice x] [+]
                  ^
                  focus restored
```

交互要点：

- picker/dialog 关闭后焦点回到触发按钮。
- `Esc` 关闭 dialog 后不触发保存。
- 保存成功关闭 dialog 后也恢复焦点，便于键盘连续编辑。

### E. 组件目录整理

```text
task-detail/
  task-detail-page.tsx
  task-property-panel.tsx
  fields/
    uda-field-editor.tsx
    task-date-field.tsx
  pickers/
    assignee-picker.tsx
    tag-picker.tsx
    task-dependency-picker.tsx
  dialogs/
    task-link-dialog.tsx
    task-annotation-dialog.tsx
```

交互要点：

- 只移动当前任务详情相关组件。
- 不改公共 UI 组件路径。
- 每次移动后跑局部测试，避免 alias/import 大面积破坏。

---

## 文件结构

- Modify: `web/src/features/workspace/project-workbench/shared/edit-feedback.tsx`
  - 为失败条目增加可选 `targetId`、定位回调和全部关闭。
- Modify: `web/src/features/workspace/project-workbench/shared/inline-text-editor.tsx`
  - 保存失败时向 feedback 上报 target，并保留原字段错误。
- Modify: `web/src/features/workspace/project-workbench/shared/inline-date-editor.tsx`
  - 同步失败 target。
- Modify: `web/src/features/workspace/project-workbench/shared/inline-select-editor.tsx`
  - 同步失败 target。
- Modify: `web/src/components/AppShell.tsx`
  - 增加移动端更多菜单，缓解 header 拥挤。
- Modify: `web/src/features/workspace/project-workbench/tasks/task-row-actions.tsx`
  - 将任务行/卡片操作收敛到菜单。
- Create/Move: `web/src/features/workspace/project-workbench/task-detail/fields/*`
  - 拆出 UDA 和日期字段编辑器。
- Create/Move: `web/src/features/workspace/project-workbench/task-detail/pickers/*`
  - 移动负责人、标签、依赖 picker。
- Create/Move: `web/src/features/workspace/project-workbench/task-detail/dialogs/*`
  - 拆出链接和注解 dialog 表单。
- Modify: `web/scripts/playwright-editing-smoke.mjs`
  - 增加移动 header 菜单、焦点恢复和失败定位 smoke。

---

## Chunk 1: 失败汇总定位

### Task 1: 让失败条目可定位

**Files:**

- Modify: `web/src/features/workspace/project-workbench/shared/edit-feedback.tsx`
- Modify: `web/src/features/workspace/project-workbench/shared/edit-feedback.test.tsx`

- [ ] **Step 1: 写失败测试**

在 `edit-feedback.test.tsx` 增加测试：

```tsx
it("focuses the failed field from the failure summary", async () => {
  const target = document.createElement("button")
  target.id = "field-owner"
  document.body.appendChild(target)

  render(<FailureHarness />)
  await userEvent.click(screen.getByRole("button", { name: "制造失败" }))
  await userEvent.click(screen.getByRole("button", { name: "定位 负责人" }))

  expect(target).toHaveFocus()
})
```

- [ ] **Step 2: 运行红灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/shared/edit-feedback.test.tsx
```

Expected: `定位 负责人` 不存在。

- [ ] **Step 3: 扩展 feedback API**

将 `failure(title, message)` 扩展为：

```ts
failure: (title: string, message: string, options?: { targetId?: string }) => void
```

渲染失败条目时，如果有 `targetId`：

- 显示 `定位` 按钮。
- 点击后 `document.getElementById(targetId)?.focus({ preventScroll: false })`。
- 同时调用 `scrollIntoView({ block: "center" })`。

- [ ] **Step 4: 运行绿灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/shared/edit-feedback.test.tsx
```

Expected: PASS。

### Task 2: inline 编辑器上报 target

**Files:**

- Modify: `web/src/features/workspace/project-workbench/shared/inline-text-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/shared/inline-date-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/shared/inline-select-editor.tsx`
- Modify: corresponding `*.test.tsx`

- [ ] **Step 1: 给 inline editor 增加 `feedbackTargetId` prop 测试**

测试失败后失败汇总出现 `定位 任务标题`，点击后焦点回到原输入。

- [ ] **Step 2: 跑局部红灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/shared/inline-text-editor.test.tsx
```

- [ ] **Step 3: 实现 prop 透传**

保存失败时：

```ts
feedback.failure(ariaLabel, message, { targetId: feedbackTargetId })
```

- [ ] **Step 4: 跑局部绿灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/shared/inline-text-editor.test.tsx
```

---

## Chunk 2: 行级任务操作菜单

### Task 3: 收敛任务行操作

**Files:**

- Modify: `web/src/features/workspace/project-workbench/tasks/task-row-actions.tsx`
- Modify: `web/src/features/workspace/project-workbench/tasks/task-table.test.tsx`

- [ ] **Step 1: 写行为测试**

覆盖：

- 桌面 `...` 菜单包含“打开详情 / 复制任务链接 / 删除任务”。
- 删除点击后仍出现 destructive confirm。
- 移动卡片操作不会水平溢出。

- [ ] **Step 2: 运行红灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/tasks/task-table.test.tsx
```

- [ ] **Step 3: 改造 `TaskRowActions`**

保留开始/完成为直接 icon 操作，将打开详情、复制链接、删除放入 dropdown menu。

- [ ] **Step 4: 运行绿灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/tasks/task-table.test.tsx
```

---

## Chunk 3: 移动端 header

### Task 4: AppShell 移动端更多菜单

**Files:**

- Modify: `web/src/components/AppShell.tsx`
- Modify: `web/src/components/AppShell.test.tsx`

- [ ] **Step 1: 写移动端菜单测试**

测试在窄视口下：

- 直接显示刷新按钮。
- 语言、主题、退出在“更多”菜单里。
- acting mode 显示“返回超管界面”。

- [ ] **Step 2: 运行红灯**

```bash
pnpm --dir web test -- --run src/components/AppShell.test.tsx
```

- [ ] **Step 3: 实现 responsive header**

桌面使用现有布局；移动端：

- 工作区和用户信息左侧两行。
- 右侧保留刷新。
- 低频操作放入 dropdown menu。

- [ ] **Step 4: 跑局部和 smoke**

```bash
pnpm --dir web test -- --run src/components/AppShell.test.tsx
pnpm --dir web run smoke:editing
```

---

## Chunk 4: Dialog 焦点恢复与组件拆分

### Task 5: picker/dialog 焦点恢复

**Files:**

- Modify: `web/src/features/workspace/project-workbench/task-detail/assignee-picker.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/tag-picker.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-dependency-picker.tsx`
- Modify: related tests

- [ ] **Step 1: 写焦点恢复测试**

打开 picker，按 `Escape` 关闭，断言触发按钮重新获得焦点。

- [ ] **Step 2: 运行红灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx
```

- [ ] **Step 3: 实现焦点恢复**

每个 picker 的触发按钮保存 ref，`onOpenChange(false)` 时：

```ts
triggerRef.current?.focus()
```

- [ ] **Step 4: 跑局部绿灯**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx
```

### Task 6: 拆分任务详情组件目录

**Files:**

- Move: `task-detail/assignee-picker.tsx` -> `task-detail/pickers/assignee-picker.tsx`
- Move: `task-detail/tag-picker.tsx` -> `task-detail/pickers/tag-picker.tsx`
- Move: `task-detail/task-dependency-picker.tsx` -> `task-detail/pickers/task-dependency-picker.tsx`
- Create: `task-detail/fields/uda-field-editor.tsx`
- Create: `task-detail/dialogs/task-link-dialog.tsx`
- Create: `task-detail/dialogs/task-annotation-dialog.tsx`

- [ ] **Step 1: 先移动 picker，不改行为**

更新 import 后运行：

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx
```

- [ ] **Step 2: 拆出 UDA 编辑器**

从 `task-property-panel.tsx` 移出 `UDAFieldEditor`、`UDABooleanEditor`、`UDAInputEditor`、`inferUDAKind`、`toBoolean`。

- [ ] **Step 3: 拆出链接/注解 dialog 表单**

保留列表容器在原文件，dialog 表单移到 `dialogs/`。

- [ ] **Step 4: 跑任务详情测试**

```bash
pnpm --dir web test -- --run src/features/workspace/project-workbench/task-detail/task-property-panel.test.tsx src/features/workspace/project-workbench/task-detail/task-links-editor.test.tsx src/features/workspace/project-workbench/task-detail/task-annotations-editor.test.tsx
```

---

## Chunk 5: 浏览器验收扩展

### Task 7: 扩展 Playwright smoke

**Files:**

- Modify: `web/scripts/playwright-editing-smoke.mjs`

- [ ] **Step 1: 增加断言**

新增：

- 移动 header 更多菜单可打开。
- 失败汇总 `定位` 会聚焦字段。
- picker 关闭后焦点回到触发按钮。
- 任务行 `...` 菜单能打开详情和复制链接。

- [ ] **Step 2: 运行 smoke**

```bash
pnpm --dir web run smoke:editing
```

Expected: PASS，截图仍写入系统临时目录。

---

## 最终验证

完成全部任务后运行：

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web build
pnpm --dir web run smoke:editing
git diff --check
```

如果实现过程中触及 Go API 或 storage，再补：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
```
