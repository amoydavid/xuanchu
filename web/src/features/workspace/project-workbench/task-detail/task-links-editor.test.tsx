import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { addTaskLink, deleteTaskLink } from "../api/task-api"
import { TaskLinksEditor } from "./task-links-editor"

vi.mock("../api/task-api", async () => {
  const actual = await vi.importActual<typeof import("../api/task-api")>(
    "../api/task-api"
  )
  return {
    ...actual,
    addTaskLink: vi.fn(),
    deleteTaskLink: vi.fn(),
  }
})

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

describe("TaskLinksEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(addTaskLink).mockResolvedValue({
      id: "link-1",
      type: "spec",
      url: "https://example.com/spec",
    })
    vi.mocked(deleteTaskLink).mockResolvedValue({
      uuid: "task-1",
      title: "任务",
      status: "pending",
    })
  })

  it("adds a link from dialog", async () => {
    render(
      <TaskLinksEditor
        canWrite={true}
        links={[]}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "添加链接" }))
    await userEvent.type(screen.getByLabelText("类型"), "spec")
    await userEvent.type(screen.getByLabelText("URL"), "https://example.com/spec")
    await userEvent.type(screen.getByLabelText("标题"), "设计文档")
    await userEvent.click(screen.getByRole("button", { name: "保存链接" }))

    expect(addTaskLink).toHaveBeenCalledWith("acme", "ads-1", {
      title: "设计文档",
      type: "spec",
      url: "https://example.com/spec",
    })
  })

  it("does not submit invalid url", async () => {
    render(
      <TaskLinksEditor
        canWrite={true}
        links={[]}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "添加链接" }))
    await userEvent.type(screen.getByLabelText("类型"), "spec")
    await userEvent.type(screen.getByLabelText("URL"), "notaurl")
    await userEvent.click(screen.getByRole("button", { name: "保存链接" }))

    expect(addTaskLink).not.toHaveBeenCalled()
    expect(screen.getByText("请输入有效 URL")).toBeTruthy()
  })

  it("deletes a link after confirmation", async () => {
    render(
      <TaskLinksEditor
        canWrite={true}
        links={[{ id: "link-1", type: "spec", url: "https://example.com" }]}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "删除链接" }))
    expect(screen.getByText("确认删除链接")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "删除" }))

    expect(deleteTaskLink).toHaveBeenCalledWith("acme", "ads-1", "link-1")
  })
})
