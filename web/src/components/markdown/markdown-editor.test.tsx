import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { MarkdownEditor } from "./markdown-editor"

describe("MarkdownEditor", () => {
  it("renders initial markdown content", async () => {
    render(<MarkdownEditor onChange={() => undefined} value="# 标题" />)

    expect(await screen.findByText("标题")).toBeTruthy()
  })

  it("calls onModEnter for keyboard save", async () => {
    const onModEnter = vi.fn()
    render(
      <MarkdownEditor
        ariaLabel="任务描述"
        onChange={() => undefined}
        onModEnter={onModEnter}
        value="内容"
      />
    )

    await userEvent.click(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    expect(onModEnter).toHaveBeenCalled()
  })

  it("reports sanitized markdown changes", async () => {
    const onChange = vi.fn()
    render(<MarkdownEditor ariaLabel="任务描述" onChange={onChange} value="" />)

    await userEvent.click(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("<script>alert(1)</script>")

    await waitFor(() => {
      expect(onChange).toHaveBeenCalled()
    })
    expect(onChange.mock.calls.at(-1)?.[0]).not.toContain("<script>")
  })
})
