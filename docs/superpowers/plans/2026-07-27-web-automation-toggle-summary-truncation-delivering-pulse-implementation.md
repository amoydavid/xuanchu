# Web Console 自动化页面三处体验修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给项目自动化页面补启动/暂停入口、修工作区指令摘要列溢出、给投递中状态点加呼吸动画，三处改动都带单元/组件测试。

**Architecture:** 纯前端改动。复用现有 `Switch` 组件与 `automation-status` 共享模块；新增一个项目级 toggle mutation hook；CSS 层加一个自定义 keyframes + `prefers-reduced-motion` 兜底。任务按依赖排序：先做动画（最底层、零依赖），再做摘要列截断，最后做项目页启停（依赖新 hook）和项目页投递列表状态点。

**Tech Stack:** React 19 + TanStack Query + Radix UI (Switch) + Tailwind v4 + Vitest + @testing-library/react。

**Spec:** `docs/superpowers/specs/2026-07-27-web-automation-toggle-summary-truncation-delivering-pulse-design.md`

---

## File Structure

| 文件 | 操作 | 责任 |
|---|---|---|
| `web/src/index.css` | Modify (末尾追加) | 新增 `@keyframes automation-delivering` + utility + `prefers-reduced-motion` 兜底 |
| `web/src/features/workspace/automations/shared/automation-status.tsx` | Modify | `deliveryDotColor` 的 `delivering` 分支加动画 class |
| `web/src/features/workspace/automations/shared/automation-status.test.tsx` | Create | 纯函数 + 展示组件测试（含动画 class 断言） |
| `web/src/features/workspace/automations/workspace-automations-page.tsx` | Modify | 指令摘要列 + 触发器列截断 |
| `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx` | Modify | 状态列加 Switch + 操作列加启停按钮 |
| `web/src/features/workspace/project-workbench/automations/project-automations-api.ts` | Modify | 新增 `useToggleProjectAutomationRule` hook |
| `web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx` | Modify | 补启停测试用例 |
| `web/src/features/workspace/project-workbench/automations/automation-delivery-list.tsx` | Modify | 状态列接入 `DeliveryStatusDot` + `deliveryStatusLabel` |
| `web/src/features/workspace/project-workbench/automations/automation-delivery-list.test.tsx` | Create | 投递列表状态点 + 中文标签测试 |

---

## Task 1: 投递中状态点呼吸动画

**Files:**
- Modify: `web/src/index.css` (末尾追加)
- Modify: `web/src/features/workspace/automations/shared/automation-status.tsx:28-41`
- Test: `web/src/features/workspace/automations/shared/automation-status.test.tsx` (Create)

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/workspace/automations/shared/automation-status.test.tsx`：

```tsx
import { render } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import {
  DeliveryStatusDot,
  RuleStatusDot,
  deliveryDotColor,
  deliveryStatusLabel,
} from "./automation-status"

describe("deliveryDotColor", () => {
  it("succeeded 落 primary 色，无动画", () => {
    expect(deliveryDotColor("succeeded")).toBe("bg-primary")
  })
  it("dead_lettered 落 destructive 色，无动画", () => {
    expect(deliveryDotColor("dead_lettered")).toBe("bg-destructive")
  })
  it("retry_wait 落琥珀色，无动画", () => {
    expect(deliveryDotColor("retry_wait")).toBe("bg-amber-500")
  })
  it("delivering 落 primary/60 且带呼吸动画 class", () => {
    expect(deliveryDotColor("delivering")).toBe("bg-primary/60 animate-automation-delivering")
  })
  it("queued / 未知 落 muted，无动画", () => {
    expect(deliveryDotColor("queued")).toBe("bg-muted-foreground/40")
  })
})

describe("deliveryStatusLabel", () => {
  it("把状态码翻译成中文", () => {
    expect(deliveryStatusLabel("queued")).toBe("排队中")
    expect(deliveryStatusLabel("delivering")).toBe("投递中")
    expect(deliveryStatusLabel("retry_wait")).toBe("等待重试")
    expect(deliveryStatusLabel("succeeded")).toBe("成功")
    expect(deliveryStatusLabel("dead_lettered")).toBe("失败")
  })
  it("未知状态原样返回", () => {
    expect(deliveryStatusLabel("unknown")).toBe("unknown")
  })
})

describe("DeliveryStatusDot", () => {
  it("delivering 状态渲染含动画 class 的圆点", () => {
    const { container } = render(<DeliveryStatusDot status="delivering" />)
    const dot = container.querySelector("span")
    expect(dot?.className).toContain("animate-automation-delivering")
    expect(dot?.className).toContain("bg-primary/60")
  })
  it("succeeded 状态无动画 class", () => {
    const { container } = render(<DeliveryStatusDot status="succeeded" />)
    const dot = container.querySelector("span")
    expect(dot?.className).not.toContain("animate-automation-delivering")
  })
})

describe("RuleStatusDot", () => {
  it("启用态落 primary 色", () => {
    const { container } = render(<RuleStatusDot enabled={true} />)
    const dot = container.querySelector("span")
    expect(dot?.className).toContain("bg-primary")
  })
  it("停用态落 muted 色", () => {
    const { container } = render(<RuleStatusDot enabled={false} />)
    const dot = container.querySelector("span")
    expect(dot?.className).toContain("bg-muted-foreground/40")
  })
})
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `pnpm --dir web test src/features/workspace/automations/shared/automation-status.test.tsx`
Expected: FAIL，`deliveryDotColor("delivering")` 期望 `"bg-primary/60 animate-automation-delivering"` 实际 `"bg-primary/60"`。

- [ ] **Step 3: 改 `automation-status.tsx` 让 `delivering` 分支带动画**

打开 `web/src/features/workspace/automations/shared/automation-status.tsx`，把第 36-37 行的 `delivering` 分支：

```tsx
    case "delivering":
      return "bg-primary/60"
```

改成：

```tsx
    case "delivering":
      return "bg-primary/60 animate-automation-delivering"
```

- [ ] **Step 4: 在 `index.css` 末尾追加 keyframes 与 utility**

在 `web/src/index.css` 末尾（第 199 行 `@layer base` 闭合 `}` 之后）追加：

```css

/*
 * 投递中状态点的呼吸动画：7px 圆点 opacity 在 0.4~1.0 之间循环，
 * 让用户感知「正在投递」。尊重 prefers-reduced-motion。
 */
@keyframes automation-delivering {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}
.animate-automation-delivering {
  animation: automation-delivering 1.4s ease-in-out infinite;
}
@media (prefers-reduced-motion: reduce) {
  .animate-automation-delivering {
    animation: none;
  }
}
```

- [ ] **Step 5: 运行测试，确认通过**

Run: `pnpm --dir web test src/features/workspace/automations/shared/automation-status.test.tsx`
Expected: PASS，全部用例绿。

- [ ] **Step 6: 提交**

```bash
git add web/src/index.css web/src/features/workspace/automations/shared/automation-status.tsx web/src/features/workspace/automations/shared/automation-status.test.tsx
git commit -m "feat: 投递中状态点增加呼吸动画并补测试"
```

---

## Task 2: 工作区自动化指令摘要列截断

**Files:**
- Modify: `web/src/features/workspace/automations/workspace-automations-page.tsx:265-270`
- Test: `web/src/features/workspace/automations/workspace-automations-page.test.tsx` (Create)

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/workspace/automations/workspace-automations-page.test.tsx`。这个测试聚焦指令摘要列截断 + hover tooltip，按既有 `workspace-automation-rule-dialog.test.tsx` 的 mock 模式：

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { WorkspaceAutomationsPage } from "./workspace-automations-page"

// useWorkspaceAutomationRules 等返回 { data } 形态。
vi.mock("@/features/workspace/automations/workspace-automations-api", () => ({
  useWorkspaceAutomationRules: vi.fn(() => ({ data: [], isLoading: false, isError: false })),
  useToggleWorkspaceAutomationRule: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useDeleteWorkspaceAutomationRule: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useWorkspaceAutomationDeliveries: vi.fn(() => ({ data: [] })),
  useReplayWorkspaceAutomationDelivery: vi.fn(() => ({ mutate: vi.fn() })),
  useWorkspaceAutomationProviderConfig: vi.fn(() => ({ data: null })),
}))

vi.mock("@/features/workspace/session/use-workspace-session", () => ({
  useWorkspaceSession: () => ({ workspaceSlug: "local" }),
}))

vi.mock("@/features/workspace/automations/workspace-automation-permissions", () => ({
  canWriteAutomation: () => true,
  canReadAutomation: () => true,
}))

function renderPage() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <WorkspaceAutomationsPage />
    </QueryClientProvider>,
  )
}

const longInstruction =
  "为新建任务自动指派飞书群组里的负责人，需要先查询项目配置中的默认群组再拉取成员列表并匹配任务 assignee 字段，这是一段很长的指令模板"

describe("WorkspaceAutomationsPage 指令摘要列", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    const { useWorkspaceAutomationRules } = await import(
      "@/features/workspace/automations/workspace-automations-api"
    )
    vi.mocked(useWorkspaceAutomationRules).mockReturnValue({
      data: [
        {
          id: "rule-1",
          workspace_id: "ws",
          scope_type: "workspace",
          scope_id: "ws",
          project_id: null,
          name: "长指令规则",
          description: "",
          enabled: true,
          trigger_type: "event",
          trigger_config: { event_type: "project.created" },
          instruction_template: longInstruction,
          action: { protocol: "chat_completions" },
          created_at: 1,
          modified_at: 1,
        },
      ],
      isLoading: false,
      isError: false,
    } as never)
    await i18n.changeLanguage("zh-CN")
  })

  it("指令摘要列单行截断且 title 属性含完整内容", async () => {
    renderPage()
    const summary = await screen.findByText(longInstruction)
    // span 元素本身渲染了完整文本（截断由 CSS 完成）
    expect(summary).toBeTruthy()
    // title 属性提供 hover tooltip 的完整内容
    expect(summary.getAttribute("title")).toBe(longInstruction)
    // 截断 class 存在
    expect(summary.className).toContain("truncate")
  })
})
```

> 注：上面 mock 的返回结构和实际 `useWorkspaceAutomationRules` 返回值需要在 Step 2 核对。如果页面 import 的模块路径或 hook 名不一致，以 Step 2 确认的为准调整测试 import。

- [ ] **Step 2: 核对 `WorkspaceAutomationsPage` 的 props 与依赖**

Read `web/src/features/workspace/automations/workspace-automations-page.tsx` 第 1-90 行，确认：
1. `WorkspaceAutomationsPage` 是无 props 组件还是接收 `{ workspaceSlug }`？据此调整 `renderPage`。
2. 页面顶部 `useWorkspaceAutomationRules`、`canWriteAutomation` 的实际 import 路径。
3. `WorkspaceAutomationRule` 的必填字段（特别是 `scope_type` / `scope_id` / `project_id` / `action` 的形状）。

如果 mock 返回值缺字段导致渲染报错，补齐字段直到能正常渲染。

- [ ] **Step 3: 运行测试，确认失败**

Run: `pnpm --dir web test src/features/workspace/automations/workspace-automations-page.test.tsx`
Expected: FAIL——`title` 属性为 `null`（当前 `<span>` 没加 title），或 className 不含 `block`。

- [ ] **Step 4: 改指令摘要列与触发器列截断**

打开 `web/src/features/workspace/automations/workspace-automations-page.tsx`，找到桌面表格行第 265-270 行：

```tsx
              <TableCell title={summarizeTriggerFull(rule.trigger_type, rule.trigger_config)}>
                <span className="truncate">{summarizeTrigger(rule.trigger_type, rule.trigger_config)}</span>
              </TableCell>
              <TableCell>
                <span className="line-clamp-1 text-muted-foreground">{rule.instruction_template}</span>
              </TableCell>
```

替换为：

```tsx
              <TableCell className="min-w-0" title={summarizeTriggerFull(rule.trigger_type, rule.trigger_config)}>
                <span className="block truncate">{summarizeTrigger(rule.trigger_type, rule.trigger_config)}</span>
              </TableCell>
              <TableCell className="min-w-0">
                <span
                  className="block truncate text-muted-foreground"
                  title={rule.instruction_template}
                >
                  {rule.instruction_template}
                </span>
              </TableCell>
```

要点：
- `TableCell` 加 `min-w-0`（覆盖 table-cell 默认 `min-width: auto`，让 `truncate` 真正生效）。
- `<span>` 改为 `block`（块级元素才能被父级宽度约束触发省略号）。
- 指令摘要列把 `title` 从 TableCell 移到内层 `<span>`，让 hover 文本只覆盖省略号区域（更精确）。

移动端 card 版（第 332-334 行 `line-clamp-2`）保持不变。

- [ ] **Step 5: 运行测试，确认通过**

Run: `pnpm --dir web test src/features/workspace/automations/workspace-automations-page.test.tsx`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add web/src/features/workspace/automations/workspace-automations-page.tsx web/src/features/workspace/automations/workspace-automations-page.test.tsx
git commit -m "fix: 工作区自动化指令摘要列单行截断并补 hover tooltip"
```

---

## Task 3: 项目自动化页面启动/暂停入口

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/project-automations-api.ts` (末尾追加 hook)
- Modify: `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx:127-176`
- Test: `web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx` (Modify, 补用例)

- [ ] **Step 1: 在 api 文件追加 toggle hook**

打开 `web/src/features/workspace/project-workbench/automations/project-automations-api.ts`，把第 1 行的 import 改为同时引入 `useMutation`：

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
```

在文件末尾（第 194 行 `useAutomationTemplateVars` 函数之后）追加：

```ts
// useToggleProjectAutomationRule 封装 enable/disable 二选一，与 workspace 侧 useToggleWorkspaceAutomationRule 对称。
export function useToggleProjectAutomationRule(projectSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (vars: { ruleId: string; enable: boolean }) => {
      return vars.enable
        ? enableProjectAutomationRule(projectSlug, vars.ruleId)
        : disableProjectAutomationRule(projectSlug, vars.ruleId)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["project", projectSlug, "automations"] })
      void queryClient.invalidateQueries({
        queryKey: ["project", projectSlug, "automation-deliveries"],
      })
    },
  })
}
```

- [ ] **Step 2: 给页面测试补启停用例（先红）**

打开 `web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx`。

在文件顶部 import 段第 8-17 行追加 `enableProjectAutomationRule`、`disableProjectAutomationRule`：

```tsx
import {
  createProjectAutomation,
  deleteProjectAutomation,
  disableProjectAutomationRule,
  enableProjectAutomationRule,
  getProjectAutomationDelivery,
  listProjectAutomationDeliveries,
  listProjectAutomations,
  previewProjectAutomation,
  testProjectAutomationRule,
  updateProjectAutomation,
} from "./project-automations-api"
```

在 `beforeEach` 内（第 128 行 `deleteProjectAutomation` mock 之后）追加 enable/disable 的默认 mock：

```tsx
    vi.mocked(enableProjectAutomationRule).mockResolvedValue({ id: "rule-1" } as never)
    vi.mocked(disableProjectAutomationRule).mockResolvedValue({ id: "rule-1" } as never)
```

在最后一个 `it` 块之后（第 278 行 `})` 之前）追加两个新测试：

```tsx
  it("toggles a rule off via the row switch", async () => {
    renderPage()
    await screen.findByText("每日项目巡检")
    // Switch 的可访问名来自 RuleStatusDot 的 aria-label「启用」。
    const sw = screen.getByRole("switch", { name: "启用" })
    await userEvent.click(sw)
    await waitFor(() =>
      expect(disableProjectAutomationRule).toHaveBeenCalledWith("adsops", "rule-1"),
    )
    expect(feedback.success).toHaveBeenCalledWith("规则已停用")
  })

  it("toggles a rule on via the row action button", async () => {
    // 把默认规则改成停用态，测「启用」分支。
    vi.mocked(listProjectAutomations).mockResolvedValue([
      {
        id: "rule-1",
        workspace_id: "ws",
        project_id: "proj",
        name: "每日项目巡检",
        description: "每天检查",
        enabled: false,
        trigger_type: "schedule",
        trigger_config: { schedule_type: "daily_at", schedule_value: "09:30", timezone: "Asia/Shanghai" },
        condition: { task_filter: "status:pending", max_tasks: 50 },
        action_type: "openai_compatible",
        action: {
          protocol: "chat_completions",
          base_url_config_key: "agent.provider.base_url",
          api_key_config_key: "agent.provider.api_key",
          model_config_key: "agent.provider.model",
          temperature: 0.2,
        },
        context: { include: ["workspace", "project", "task_summary", "matched_tasks", "project_config"] },
        instruction_template: "生成巡检",
        system_prompt: "",
        created_at: 1,
        modified_at: 1,
      },
    ])
    renderPage()
    await screen.findByText("每日项目巡检")
    await userEvent.click(screen.getByRole("button", { name: "启用" }))
    await waitFor(() =>
      expect(enableProjectAutomationRule).toHaveBeenCalledWith("adsops", "rule-1"),
    )
    expect(feedback.success).toHaveBeenCalledWith("规则已启用")
  })

  it("disables toggle switch for closed projects", async () => {
    layoutState.closed = true
    renderPage()
    await screen.findByText("每日项目巡检")
    const sw = screen.getByRole("switch", { name: "启用" })
    expect((sw as HTMLButtonElement).disabled).toBe(true)
  })
```

- [ ] **Step 3: 运行测试，确认失败**

Run: `pnpm --dir web test src/features/workspace/project-workbench/automations/project-automations-page.test.tsx`
Expected: FAIL——找不到 `role="switch"` 元素（页面还没加 Switch），且 `enable/disableProjectAutomationRule` 还没在 mock 工厂里（mock 工厂第 40-61 行需要补这两个导出，否则 import 会拿到 undefined）。

补 mock 工厂：打开测试文件第 40-61 行的 `vi.mock("./project-automations-api", ...)`，确保工厂里已经有 `enableProjectAutomationRule: vi.fn()` 和 `disableProjectAutomationRule: vi.fn()`（从原文件看第 46-47 行已有，无需改）。再跑测试确认仍 FAIL 在找不到 switch。

- [ ] **Step 4: 改页面，加 Switch + 启停按钮**

打开 `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx`。

**4a. 加 import。** 第 1-23 行 import 段追加：

```tsx
import { Switch } from "@/components/ui/switch"
import {
  RuleStatusDot,
} from "@/features/workspace/automations/shared/automation-status"
```

并在 api import 块（第 12-18 行）追加 `useToggleProjectAutomationRule`：

```tsx
import {
  deleteProjectAutomation,
  listProjectAutomations,
  testProjectAutomationRule,
  useToggleProjectAutomationRule,
  type ProjectAutomationRule,
  type ProjectAutomationRuleInput,
} from "./project-automations-api"
```

**4b. 在组件里加 toggle hook。** 第 89 行 `deleteMutation` 之后追加：

```tsx
  const toggle = useToggleProjectAutomationRule(projectSlug)
```

**4c. 改状态列。** 把第 137 行：

```tsx
                <td className="p-2">{rule.enabled ? "启用" : "停用"}</td>
```

替换为：

```tsx
                <td className="p-2">
                  <div className="flex items-center gap-2">
                    <RuleStatusDot enabled={rule.enabled} />
                    <Switch
                      checked={rule.enabled}
                      disabled={writeDisabled}
                      aria-label={rule.enabled ? "启用" : "停用"}
                      onCheckedChange={(checked) => {
                        toggle.mutate(
                          { ruleId: rule.id, enable: checked },
                          {
                            onSuccess: () => {
                              invalidateRules()
                              feedback.success(checked ? "规则已启用" : "规则已停用")
                            },
                            onError: (err) => feedback.failure("操作失败", errorMessage(err)),
                          },
                        )
                      }}
                    />
                  </div>
                </td>
```

**4d. 改操作列，加启停按钮。** 把第 142-170 行的 `<div className="flex gap-1">` 内部，在「编辑」按钮之后、「立即测试」之前，插入启停按钮。最终操作列结构：

```tsx
                  <div className="flex gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={writeDisabled}
                      onClick={() => setEditing(rule)}
                    >
                      编辑
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={
                        writeDisabled ||
                        (toggle.isPending && toggle.variables?.ruleId === rule.id)
                      }
                      onClick={() => {
                        toggle.mutate(
                          { ruleId: rule.id, enable: !rule.enabled },
                          {
                            onSuccess: () => {
                              invalidateRules()
                              feedback.success(!rule.enabled ? "规则已启用" : "规则已停用")
                            },
                            onError: (err) => feedback.failure("操作失败", errorMessage(err)),
                          },
                        )
                      }}
                    >
                      {rule.enabled ? "停用" : "启用"}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={writeDisabled || testMutation.isPending}
                      onClick={() => testMutation.mutate(rule.id)}
                    >
                      立即测试
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={writeDisabled}
                      onClick={() => setDeleting(rule)}
                    >
                      删除
                    </Button>
                  </div>
```

> 注意：「停用/启用」文字按钮现在会与「删除」确认弹窗里的「删除」按钮共存。测试里删第二个测试用的 `screen.getByRole("button", { name: "启用" })` 取第一个匹配即可——但要注意当规则是 `enabled: true` 时操作列按钮文字是「停用」，不是「启用」。第二个测试用例把规则设为 `enabled: false` 正是为了让按钮文字变成「启用」。

- [ ] **Step 5: 运行测试，确认通过**

Run: `pnpm --dir web test src/features/workspace/project-workbench/automations/project-automations-page.test.tsx`
Expected: PASS，含新增的 3 个 toggle 用例。

如果「禁用项目时 toggle switch disabled」用例失败（断言 `disabled` 为 true），检查 Switch 的 `disabled` prop 是否在 `writeDisabled` 为 true 时正确传入。

- [ ] **Step 6: typecheck 确认类型正确**

Run: `pnpm --dir web typecheck`
Expected: 无错误。重点看 `useToggleProjectAutomationRule` 的返回类型与 `toggle.variables` 推断是否正确。

- [ ] **Step 7: 提交**

```bash
git add web/src/features/workspace/project-workbench/automations/project-automations-api.ts web/src/features/workspace/project-workbench/automations/project-automations-page.tsx web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx
git commit -m "feat: 项目自动化页面增加启动/暂停入口（Switch + 行按钮）"
```

---

## Task 4: 项目页投递列表接入状态点

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/automation-delivery-list.tsx:42-48`
- Test: `web/src/features/workspace/project-workbench/automations/automation-delivery-list.test.tsx` (Create)

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/workspace/project-workbench/automations/automation-delivery-list.test.tsx`：

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { AutomationDeliveryList } from "./automation-delivery-list"
import { listProjectAutomationDeliveries } from "./project-automations-api"

vi.mock("./project-automations-api", () => ({
  listProjectAutomationDeliveries: vi.fn(),
}))

function renderList() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <AutomationDeliveryList projectSlug="adsops" />
    </QueryClientProvider>,
  )
}

describe("AutomationDeliveryList 状态列", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("用状态点 + 中文标签渲染投递中状态", async () => {
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([
      {
        id: "d-1",
        workspace_id: "ws",
        project_id: "proj",
        rule_id: "r-1",
        trigger_type: "manual_test",
        event_id: "",
        event_type: "",
        status: "delivering",
        resolved_url: "https://agent.example.com",
        rendered_method: "POST",
        rendered_headers: {},
        request_body_preview: "{}",
        request_body_hash: "",
        response_body_preview: "",
        provider_request_id: "",
        usage: {},
        attempt_count: 0,
        last_error: "",
        created_at: 1,
        modified_at: 1,
      },
    ])
    renderList()
    expect(await screen.findByText("投递中")).toBeTruthy()
  })

  it("用状态点 + 中文标签渲染成功状态", async () => {
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([
      {
        id: "d-2",
        workspace_id: "ws",
        project_id: "proj",
        rule_id: "r-1",
        trigger_type: "schedule",
        event_id: "",
        event_type: "",
        status: "succeeded",
        resolved_url: "https://agent.example.com",
        rendered_method: "POST",
        rendered_headers: {},
        request_body_preview: "{}",
        request_body_hash: "",
        response_body_preview: "",
        provider_request_id: "",
        usage: {},
        attempt_count: 1,
        last_error: "",
        created_at: 1,
        modified_at: 2,
      },
    ])
    renderList()
    expect(await screen.findByText("成功")).toBeTruthy()
  })

  it("空状态展示提示", async () => {
    vi.mocked(listProjectAutomationDeliveries).mockResolvedValue([])
    renderList()
    expect(await screen.findByText("暂无运行记录")).toBeTruthy()
  })
})
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `pnpm --dir web test src/features/workspace/project-workbench/automations/automation-delivery-list.test.tsx`
Expected: FAIL——找不到「投递中」文本（当前渲染的是裸 `row.status` 字符串 `"delivering"`）。

- [ ] **Step 3: 改投递列表，接入状态点 + 中文标签**

打开 `web/src/features/workspace/project-workbench/automations/automation-delivery-list.tsx`。

第 1-5 行 import 段追加：

```tsx
import {
  DeliveryStatusDot,
  deliveryStatusLabel,
} from "@/features/workspace/automations/shared/automation-status"
```

把第 42-48 行的状态单元格：

```tsx
                <tr key={row.id} className="border-b">
                  <td className="p-2">{row.status}</td>
```

改为：

```tsx
                <tr key={row.id} className="border-b">
                  <td className="p-2">
                    <div className="flex items-center gap-2">
                      <DeliveryStatusDot status={row.status} />
                      <span>{deliveryStatusLabel(row.status)}</span>
                    </div>
                  </td>
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `pnpm --dir web test src/features/workspace/project-workbench/automations/automation-delivery-list.test.tsx`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add web/src/features/workspace/project-workbench/automations/automation-delivery-list.tsx web/src/features/workspace/project-workbench/automations/automation-delivery-list.test.tsx
git commit -m "feat: 项目页投递列表接入状态点与中文标签"
```

---

## Task 5: 最终验证

**Files:** 无（仅运行验证命令）

- [ ] **Step 1: 全量 typecheck**

Run: `pnpm --dir web typecheck`
Expected: 无错误。

- [ ] **Step 2: 全量 lint**

Run: `pnpm --dir web lint`
Expected: 无错误。如果报「跨模块依赖」相关警告（`project-workbench` 引 `automations/shared`），这是 spec 第 5 节确认过的对称依赖，可接受。

- [ ] **Step 3: 全量 build**

Run: `pnpm --dir web build`
Expected: 构建成功，无 TypeScript / 打包错误。

- [ ] **Step 4: 全量 test**

Run: `pnpm --dir web test`
Expected: 全绿，含本次新增的 4 个测试文件：
- `automation-status.test.tsx`
- `workspace-automations-page.test.tsx`
- `project-automations-page.test.tsx`（扩充）
- `automation-delivery-list.test.tsx`

- [ ] **Step 5: 手动浏览器验证（可选但推荐）**

启动 dev server，在浏览器里验证：
1. 项目自动化页面：Switch 切换生效、操作列按钮文案随状态变化、关闭项目时禁用。
2. 工作区自动化页面：长指令被截断成一行，hover 显示完整内容。
3. 任一投递记录进入 `delivering` 状态时，状态点呈呼吸动画。

---

## Self-Review

**Spec 覆盖核对：**
- spec 3.1 项目页启停入口（Switch + 行按钮 + hook）→ Task 3 ✅
- spec 3.2 工作区指令摘要列截断 → Task 2 ✅
- spec 3.3 投递中动画（keyframes + reduced-motion + 覆盖 4 处调用点）→ Task 1（动画本体）+ Task 4（项目页投递列表接线）✅
- spec §6 验收标准 1-4 → Task 1-5 ✅

**类型一致性核对：**
- `useToggleProjectAutomationRule(projectSlug)` 入参 `string`，返回 mutation，`variables` 形状 `{ ruleId: string; enable: boolean }`，与 workspace 侧 `useToggleWorkspaceAutomationRule` 对称（Task 3 Step 1 vs `workspace-automations-api.ts:211-225`）✅
- `deliveryDotColor` 返回值字符串拼接，测试断言精确匹配 ✅
- `DeliveryStatusDot` / `RuleStatusDot` / `deliveryStatusLabel` 的 import 路径统一为 `@/features/workspace/automations/shared/automation-status` ✅

**Placeholder 扫描：** 无 TBD/TODO，所有代码块完整 ✅
