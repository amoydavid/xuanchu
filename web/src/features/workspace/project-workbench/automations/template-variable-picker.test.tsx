import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ComponentProps } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { TemplateVariablePicker } from "./template-variable-picker"

vi.mock("./project-automations-api", () => ({
  useAutomationTemplateVars: () => ({
    data: {
      triggers: [
        {
          trigger: "schedule",
          vars: [
            { name: "project.slug", description: "项目 slug" },
            { name: "tasks", description: "匹配任务列表" },
            { name: "project_config.*", description: "项目配置项", is_prefix: true },
          ],
        },
        {
          trigger: "event",
          vars: [
            { name: "event.type", description: "事件类型" },
            { name: "added_assignees", description: "新增负责人" },
          ],
        },
      ],
    },
  }),
}))

vi.mock("@/features/workspace/project-workbench/api/project-api", () => ({
  listProjectConfig: vi.fn(async () => [
    { key: "feishu.chat_id", value: "oc_xxx" },
    { key: "agent.provider.model", value: "project-operator" },
  ]),
}))

function renderPicker(props: ComponentProps<typeof TemplateVariablePicker>) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <TemplateVariablePicker {...props} />
    </QueryClientProvider>
  )
}

describe("TemplateVariablePicker", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("opens popover and inserts generic variable on click", async () => {
    const onInsert = vi.fn()
    renderPicker({
      projectSlug: "adsops",
      workspaceSlug: "local",
      trigger: "schedule",
      onInsert,
      disabled: false,
    })
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))
    // is_prefix 的 project_config.* 不应出现在通用变量里。
    const item = await screen.findByText(/project\.slug/)
    await userEvent.click(item)
    expect(onInsert).toHaveBeenCalledWith("{{project.slug}}")
  })

  it("shows project config keys in a separate group", async () => {
    const onInsert = vi.fn()
    renderPicker({
      projectSlug: "adsops",
      workspaceSlug: "local",
      trigger: "schedule",
      onInsert,
      disabled: false,
    })
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))
    // 等待 config key 出现。
    const configItem = await screen.findByText(/feishu\.chat_id/)
    await userEvent.click(configItem)
    expect(onInsert).toHaveBeenCalledWith("{{project_config:feishu.chat_id}}")
  })

  it("filters variables by trigger type", async () => {
    const onInsert = vi.fn()
    renderPicker({
      projectSlug: "adsops",
      workspaceSlug: "local",
      trigger: "event",
      onInsert,
      disabled: false,
    })
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))
    expect(await screen.findByText(/event\.type/)).toBeTruthy()
    expect(screen.queryByText(/tasks/)).toBeNull()
  })

  it("keeps the command list scrollable inside a Dialog scroll lock", async () => {
    renderPicker({
      projectSlug: "adsops",
      workspaceSlug: "local",
      trigger: "schedule",
      onInsert: vi.fn(),
      disabled: false,
    })
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
