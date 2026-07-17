import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { InlineTextEditor } from "./inline-text-editor"
import { EditFeedbackProvider } from "./edit-feedback"

describe("InlineTextEditor", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("saves edited value with Enter", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    function ControlledEditor() {
      const [value, setValue] = useState("旧标题")
      return (
        <InlineTextEditor
          ariaLabel="标题"
          value={value}
          onSave={async (next) => {
            await onSave(next)
            setValue(next)
          }}
        />
      )
    }
    render(<ControlledEditor />)

    expect(
      screen.getByRole("button", { name: "标题" }).getAttribute("data-slot")
    ).toBe("button")
    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.clear(screen.getByLabelText("标题"))
    await userEvent.type(screen.getByLabelText("标题"), "新标题{Enter}")

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith("新标题")
    })
    expect(screen.getByText("新标题")).toBeTruthy()
  })

  it("trims saved values and skips unchanged blur saves", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(<InlineTextEditor ariaLabel="标题" value="旧标题" onSave={onSave} />)

    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.tab()
    expect(onSave).not.toHaveBeenCalled()

    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.clear(screen.getByLabelText("标题"))
    await userEvent.type(screen.getByLabelText("标题"), "  新标题  {Enter}")

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledTimes(1)
    })
    expect(onSave).toHaveBeenCalledWith("新标题")
  })

  it("keeps editing state when save fails", async () => {
    const onSave = vi.fn().mockRejectedValue(new Error("scope denied"))
    render(
      <EditFeedbackProvider>
        <InlineTextEditor ariaLabel="标题" value="旧标题" onSave={onSave} />
      </EditFeedbackProvider>
    )

    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.clear(screen.getByLabelText("标题"))
    await userEvent.type(screen.getByLabelText("标题"), "新标题{Enter}")

    await waitFor(() => {
      expect(screen.getAllByText(/scope denied/).length).toBeGreaterThan(0)
    })
    expect((screen.getByLabelText("标题") as HTMLInputElement).value).toBe(
      "新标题"
    )
    expect(screen.getByRole("alert").textContent).toContain(
      "标题：scope denied"
    )
  })

  it("cancels editing with Escape", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(<InlineTextEditor ariaLabel="标题" value="旧标题" onSave={onSave} />)

    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.clear(screen.getByLabelText("标题"))
    await userEvent.type(screen.getByLabelText("标题"), "临时值{Escape}")

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText("旧标题")).toBeTruthy()
  })

  it("validates before saving", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(
      <InlineTextEditor
        ariaLabel="标题"
        value="旧标题"
        validate={(value) => (value.trim() ? null : "标题不能为空")}
        onSave={onSave}
      />
    )

    await userEvent.click(screen.getByText("旧标题"))
    await userEvent.clear(screen.getByLabelText("标题"))
    await userEvent.keyboard("{Enter}")

    expect(await screen.findByText("标题不能为空")).toBeTruthy()
    expect(onSave).not.toHaveBeenCalled()
  })

  it("saves multiline value with Ctrl or Meta Enter only", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(
      <InlineTextEditor
        ariaLabel="描述"
        value="旧描述"
        multiline
        onSave={onSave}
      />
    )

    await userEvent.click(screen.getByText("旧描述"))
    await userEvent.clear(screen.getByLabelText("描述"))
    await userEvent.type(screen.getByLabelText("描述"), "第一行{Enter}第二行")
    expect(onSave).not.toHaveBeenCalled()

    await userEvent.keyboard("{Control>}{Enter}{/Control}")
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith("第一行\n第二行")
    })
  })

  it("does not enter editing when disabled", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(
      <InlineTextEditor
        ariaLabel="标题"
        value="旧标题"
        disabled
        onSave={onSave}
      />
    )

    await userEvent.click(screen.getByText("旧标题"))

    expect(screen.queryByLabelText("标题")).toBeTruthy()
    expect(screen.queryByRole("textbox", { name: "标题" })).toBeNull()
  })
})
