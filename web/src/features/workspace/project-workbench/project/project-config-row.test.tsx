import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { ProjectConfigRow } from "./project-config-row"

describe("ProjectConfigRow", () => {
  it("通过 shadcn 确认框删除配置项", async () => {
    await i18n.changeLanguage("zh-CN")
    const onDelete = vi.fn()
    const nativeConfirm = vi.spyOn(window, "confirm")

    render(
      <ProjectConfigRow
        canManage={true}
        entry={{ key: "notify.channel", value: "project-alerts" }}
        onDelete={onDelete}
        onSave={vi.fn()}
      />
    )

    await userEvent.click(screen.getByRole("button", { name: "删除" }))

    expect(screen.getByRole("alertdialog")).toBeTruthy()
    expect(screen.getByText("确认删除该配置项？")).toBeTruthy()
    expect(nativeConfirm).not.toHaveBeenCalled()
    expect(onDelete).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole("button", { name: "删除" }))
    expect(onDelete).toHaveBeenCalledOnce()
  })
})
