import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { AutomationPreviewDialog } from "./automation-preview-dialog"

describe("AutomationPreviewDialog", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("renders masked authorization and body JSON", async () => {
    const writeText = vi.fn()
    Object.assign(navigator, { clipboard: { writeText } })
    render(
      <AutomationPreviewDialog
        open
        onOpenChange={() => undefined}
        preview={{
          method: "POST",
          url: "https://agent.example.com/v1/chat/completions",
          headers: { Authorization: "Bearer ****", "Content-Type": "application/json" },
          body: { model: "project-operator", messages: [] },
          warnings: [],
        }}
      />
    )
    expect(screen.getByText("Authorization: Bearer ****")).toBeTruthy()
    expect(screen.getByText(/"model": "project-operator"/)).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "复制 JSON" }))
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('"model": "project-operator"'))
    expect(writeText.mock.calls[0][0]).not.toContain("sk-")
  })
})
