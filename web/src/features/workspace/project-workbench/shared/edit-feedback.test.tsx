import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it } from "vitest"

import { i18n } from "@/i18n"
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

function SecondFailureTrigger() {
  const feedback = useEditFeedback()
  return (
    <button
      onClick={() => feedback.failure("描述", "network_error")}
      type="button"
    >
      第二个失败
    </button>
  )
}

describe("EditFeedbackProvider", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

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

  it("uses English one and other plural forms for failed edit counts", async () => {
    await i18n.changeLanguage("en-US")
    render(
      <EditFeedbackProvider>
        <FailureTrigger />
        <SecondFailureTrigger />
      </EditFeedbackProvider>
    )

    await userEvent.click(screen.getByRole("button", { name: "失败" }))
    expect((await screen.findByRole("alert")).textContent).toContain(
      "1 edit not saved"
    )
    await userEvent.click(screen.getByRole("button", { name: "第二个失败" }))
    expect(screen.getByRole("alert").textContent).toContain("2 edits not saved")
  })
})
