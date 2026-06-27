import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { addTaskAnnotation, deleteTaskAnnotation } from "../api/task-api"
import { TaskAnnotationsEditor } from "./task-annotations-editor"

vi.mock("../api/task-api", async () => {
  const actual = await vi.importActual<typeof import("../api/task-api")>(
    "../api/task-api"
  )
  return {
    ...actual,
    addTaskAnnotation: vi.fn(),
    deleteTaskAnnotation: vi.fn(),
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

describe("TaskAnnotationsEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(addTaskAnnotation).mockResolvedValue({
      uuid: "task-1",
      title: "任务",
      status: "pending",
    })
    vi.mocked(deleteTaskAnnotation).mockResolvedValue({
      uuid: "task-1",
      title: "任务",
      status: "pending",
    })
  })

  it("adds an annotation and clears the composer", async () => {
    render(
      <TaskAnnotationsEditor
        annotations={[]}
        canWrite={true}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("新增注解"), "已联系素材团队")
    await userEvent.click(screen.getByRole("button", { name: "添加注解" }))

    expect(addTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", {
      description: "已联系素材团队",
    })
    expect((screen.getByLabelText("新增注解") as HTMLTextAreaElement).value).toBe("")
  })

  it("does not submit empty annotation and keeps failed input", async () => {
    vi.mocked(addTaskAnnotation).mockRejectedValue(new Error("scope denied"))
    render(
      <TaskAnnotationsEditor
        annotations={[]}
        canWrite={true}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "添加注解" }))
    expect(addTaskAnnotation).not.toHaveBeenCalled()
    expect(screen.getByText("注解不能为空")).toBeTruthy()

    await userEvent.type(screen.getByLabelText("新增注解"), "失败保留")
    await userEvent.click(screen.getByRole("button", { name: "添加注解" }))

    expect(await screen.findByText("scope denied")).toBeTruthy()
    expect((screen.getByLabelText("新增注解") as HTMLTextAreaElement).value).toBe(
      "失败保留"
    )
  })

  it("deletes an annotation after confirmation", async () => {
    render(
      <TaskAnnotationsEditor
        annotations={[{ id: "note-1", description: "旧注解" }]}
        canWrite={true}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "删除注解" }))
    expect(screen.getByText("确认删除注解")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "删除" }))

    expect(deleteTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", "note-1")
  })
})
