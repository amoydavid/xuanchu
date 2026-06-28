import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { EditFeedbackProvider, useEditFeedback } from "./edit-feedback"

function FeedbackTrigger() {
  const feedback = useEditFeedback()
  return (
    <button onClick={() => feedback.success("已保存：任务标题")} type="button">
      保存
    </button>
  )
}

function FailureTrigger() {
  const feedback = useEditFeedback()
  return (
    <button
      onClick={() => feedback.failure("负责人", "permission_denied")}
      type="button"
    >
      失败
    </button>
  )
}

describe("EditFeedbackProvider", () => {
  it("shows a short lived success message", async () => {
    render(
      <EditFeedbackProvider timeoutMs={200}>
        <FeedbackTrigger />
      </EditFeedbackProvider>
    )

    await userEvent.click(screen.getByRole("button", { name: "保存" }))
    expect((await screen.findByRole("status")).textContent).toContain(
      "已保存：任务标题"
    )

    await waitFor(() => {
      expect(screen.queryByRole("status")).toBeNull()
    })
  })

  it("shows dismissible failed edit summaries", async () => {
    render(
      <EditFeedbackProvider>
        <FailureTrigger />
      </EditFeedbackProvider>
    )

    await userEvent.click(screen.getByRole("button", { name: "失败" }))
    const alert = screen.getByRole("alert")
    expect(alert.getAttribute("data-slot")).toBe("alert")
    expect(alert.textContent).toContain("1 个编辑未保存")
    expect(screen.getByText("负责人：permission_denied")).toBeTruthy()

    const closeButton = screen.getByRole("button", { name: "关闭失败 负责人" })
    expect(closeButton.getAttribute("data-slot")).toBe("button")
    await userEvent.click(closeButton)
    expect(screen.queryByRole("alert")).toBeNull()
  })
})
