import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { createTask } from "../api/task-api"
import { getWorkspaceMembers } from "../api/users-api"
import { TaskCreateDialog } from "./task-create-dialog"

vi.mock("../api/task-api", () => ({
  createTask: vi.fn(),
}))

vi.mock("../api/users-api", () => ({
  getWorkspaceMembers: vi.fn(),
}))

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

describe("TaskCreateDialog", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(createTask).mockResolvedValue({
      uuid: "task-1",
      task_slug: "ads-1",
      title: "写日报",
      status: "pending",
      project: "adsops",
    })
    vi.mocked(getWorkspaceMembers).mockResolvedValue([
      {
        user_id: "u1",
        name: "liuwei",
        display_name: "刘玮",
        email: "liuwei@example.com",
        role: "member",
        joined_at: 1,
        modified_at: 1,
      },
    ])
  })

  it("creates a task with markdown description, assignee, priority, due date, and tags", async () => {
    render(
      <TaskCreateDialog
        filters="status=pending"
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("任务标题"), "复盘素材")
    expect(screen.getByRole("button", { name: "加粗" })).toBeTruthy()
    await userEvent.type(
      screen.getByLabelText("任务内容"),
      "# 整理异常原因{Enter}{Enter}- 素材"
    )
    await userEvent.click(screen.getByRole("combobox", { name: "优先级" }))
    await userEvent.click(screen.getByRole("option", { name: "H" }))
    await userEvent.click(screen.getByRole("button", { name: "截止日期" }))
    await screen.findByRole("grid")
    await userEvent.click(
      screen.getByRole("button", { name: /Friday, July 3rd, 2026/i })
    )
    await userEvent.click(screen.getByRole("button", { name: "选择负责人" }))
    expect(await screen.findByText("刘玮")).toBeTruthy()
    await userEvent.click(screen.getByRole("checkbox", { name: /刘玮/ }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    await userEvent.type(screen.getByLabelText("标签"), "ops,review")
    await userEvent.click(screen.getByRole("button", { name: "创建任务" }))

    await waitFor(() => {
      expect(createTask).toHaveBeenCalled()
    })
    const payload = vi.mocked(createTask).mock.calls[0]?.[1]
    expect(payload).toMatchObject({
      assignees: ["u1"],
      due: 1783036800,
      priority: "H",
      project: "adsops",
      tags: ["ops", "review"],
      title: "复盘素材",
    })
    expect(payload.description).toContain("# 整理异常原因")
    expect(payload.description).toContain("- 素材")
  })

  it("keeps the dialog open and shows validation when title is empty", async () => {
    render(
      <TaskCreateDialog
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "创建任务" }))

    expect(createTask).not.toHaveBeenCalled()
    expect(screen.getByText("任务标题不能为空")).toBeTruthy()
  })
})
