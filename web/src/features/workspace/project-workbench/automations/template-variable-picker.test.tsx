import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
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

describe("TemplateVariablePicker", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("opens popover and inserts variable on click", async () => {
    const onInsert = vi.fn()
    render(
      <TemplateVariablePicker
        projectSlug="adsops"
        trigger="schedule"
        onInsert={onInsert}
        disabled={false}
      />
    )
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))
    // Command item 展示的是 {{project.slug}} 文本。
    const item = await screen.findByText(/project\.slug/)
    await userEvent.click(item)
    expect(onInsert).toHaveBeenCalledWith("{{project.slug}}")
  })

  it("filters variables by trigger type", async () => {
    const onInsert = vi.fn()
    render(
      <TemplateVariablePicker
        projectSlug="adsops"
        trigger="event"
        onInsert={onInsert}
        disabled={false}
      />
    )
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))
    expect(await screen.findByText(/event\.type/)).toBeTruthy()
    expect(screen.queryByText(/tasks/)).toBeNull()
  })
})
