import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { LinkDialog } from "./link-dialog"

describe("LinkDialog", () => {
  it("回填已有链接", () => {
    render(
      <LinkDialog
        initialHref="https://example.com/old"
        onOpenChange={() => undefined}
        onSubmit={() => undefined}
        open
      />
    )

    expect(
      (screen.getByLabelText("链接地址") as HTMLInputElement).value
    ).toBe("https://example.com/old")
  })

  it("提交合法 https 链接", async () => {
    const onSubmit = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <LinkDialog
        onOpenChange={onOpenChange}
        onSubmit={onSubmit}
        open
      />
    )

    await userEvent.clear(screen.getByLabelText("链接地址"))
    await userEvent.type(
      screen.getByLabelText("链接地址"),
      "https://example.com/new"
    )
    await userEvent.click(screen.getByRole("button", { name: "确定" }))

    expect(onSubmit).toHaveBeenCalledWith("https://example.com/new")
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("非法协议不提交并显示错误", async () => {
    const onSubmit = vi.fn()
    render(
      <LinkDialog
        onOpenChange={() => undefined}
        onSubmit={onSubmit}
        open
      />
    )

    await userEvent.clear(screen.getByLabelText("链接地址"))
    await userEvent.type(screen.getByLabelText("链接地址"), "ftp://bad")
    await userEvent.click(screen.getByRole("button", { name: "确定" }))

    expect(onSubmit).not.toHaveBeenCalled()
    expect(
      screen.getByText("仅支持 http:// / https:// / mailto: 链接")
    ).toBeTruthy()
  })
})
