import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"

import { WorkspaceTemplateVariablePicker } from "./workspace-template-variable-picker"

vi.mock("@/features/workspace/automations/workspace-automations-api", () => ({
  useWorkspaceAutomationTemplateVars: () => ({
    data: {
      triggers: [
        {
          trigger: "schedule",
          vars: [
            { name: "workspace.name", description: "Workspace 名称" },
            { name: "tasks", description: "匹配任务列表" },
          ],
        },
      ],
    },
  }),
}))

function renderPicker() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <WorkspaceTemplateVariablePicker
        trigger="schedule"
        onInsert={vi.fn()}
      />
    </QueryClientProvider>
  )
}

describe("WorkspaceTemplateVariablePicker", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("keeps the command list scrollable inside a Dialog scroll lock", async () => {
    renderPicker()
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))

    const list = document.querySelector<HTMLElement>('[data-slot="command-list"]')
    expect(list).toBeTruthy()
    if (!list) return

    // jsdom 不计算布局，手动构造 CommandList 内容溢出的真实条件。
    Object.defineProperty(list, "scrollHeight", { value: 300, configurable: true })
    Object.defineProperty(list, "clientHeight", { value: 100, configurable: true })

    // 模拟 Radix Dialog 底层 react-remove-scroll 的 document wheel 锁。
    const documentWheel = vi.fn((event: WheelEvent) => event.preventDefault())
    document.addEventListener("wheel", documentWheel)
    try {
      const event = new WheelEvent("wheel", {
        bubbles: true,
        cancelable: true,
        deltaY: 80,
      })
      list.dispatchEvent(event)

      expect(list.scrollTop).toBe(80)
      expect(documentWheel).not.toHaveBeenCalled()
      expect(event.defaultPrevented).toBe(true)
    } finally {
      document.removeEventListener("wheel", documentWheel)
    }
  })
})
