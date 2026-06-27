import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { createTask } from "../api/task-api"
import { TaskQuickCreate } from "./task-quick-create"

vi.mock("../api/task-api", () => ({
  createTask: vi.fn(),
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

describe("TaskQuickCreate", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(createTask).mockResolvedValue({
      uuid: "task-1",
      task_slug: "ads-1",
      title: "写日报",
      status: "pending",
      project: "adsops",
    })
  })

  it("creates a project task with Enter and clears the input", async () => {
    render(
      <TaskQuickCreate
        canCreate={true}
        projectSlug="adsops"
        projectStatus="active"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("新任务标题"), "写日报{Enter}")

    expect(createTask).toHaveBeenCalledWith("acme", {
      title: "写日报",
      project: "adsops",
    })
    await waitFor(() => {
      expect((screen.getByLabelText("新任务标题") as HTMLInputElement).value).toBe(
        ""
      )
    })
  })

  it("includes priority and due date in the create payload", async () => {
    render(
      <TaskQuickCreate
        canCreate={true}
        projectSlug="adsops"
        projectStatus="active"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("新任务标题"), "复盘素材")
    await userEvent.click(screen.getByRole("combobox", { name: "任务优先级" }))
    await userEvent.click(screen.getByRole("option", { name: "H" }))
    await userEvent.type(screen.getByLabelText("截止日期"), "2026-07-03")
    await userEvent.click(screen.getByRole("button", { name: "创建任务" }))

    expect(createTask).toHaveBeenCalledWith("acme", {
      due: 1783036800,
      priority: "H",
      project: "adsops",
      title: "复盘素材",
    })
  })

  it("does not render for closed projects or users without task write permission", () => {
    const { rerender } = render(
      <TaskQuickCreate
        canCreate={false}
        projectSlug="adsops"
        projectStatus="active"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.queryByLabelText("新任务标题")).toBeNull()

    rerender(
      <TaskQuickCreate
        canCreate={true}
        projectSlug="adsops"
        projectStatus="archived"
        workspaceSlug="acme"
      />
    )

    expect(screen.queryByLabelText("新任务标题")).toBeNull()
  })

  it("keeps the draft when create fails", async () => {
    vi.mocked(createTask).mockRejectedValue(new Error("project closed"))
    render(
      <TaskQuickCreate
        canCreate={true}
        projectSlug="adsops"
        projectStatus="active"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("新任务标题"), "补预算{Enter}")

    expect(await screen.findByText("project closed")).toBeTruthy()
    expect((screen.getByLabelText("新任务标题") as HTMLInputElement).value).toBe(
      "补预算"
    )
  })
})
