import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { EditFeedbackProvider } from "../shared/edit-feedback"
import {
  addTaskAnnotation,
  deleteTaskAnnotation,
  updateTaskAnnotation,
} from "../api/task-api"
import { TaskAnnotationsEditor } from "./task-annotations-editor"

vi.mock("../api/task-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/task-api")>("../api/task-api")
  return {
    ...actual,
    addTaskAnnotation: vi.fn(),
    deleteTaskAnnotation: vi.fn(),
    updateTaskAnnotation: vi.fn(),
  }
})

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <EditFeedbackProvider>{children}</EditFeedbackProvider>
      </QueryClientProvider>
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
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
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
    vi.mocked(updateTaskAnnotation).mockResolvedValue({
      uuid: "task-1",
      title: "任务",
      status: "pending",
      annotations: [{ id: "note-1", description: "更新注解" }],
    })
  })

  it("adds an annotation and collapses the composer", async () => {
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

    await userEvent.click(screen.getByRole("button", { name: "写更新" }))
    await userEvent.type(
      screen.getByLabelText("新增注解"),
      "Material team ready"
    )
    await userEvent.click(screen.getByRole("button", { name: "添加注解" }))

    expect(addTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", {
      description: "Material team ready",
    })
    expect(screen.queryByLabelText("新增注解")).toBeNull()
    expect(screen.getByRole("button", { name: "写更新" })).toBeTruthy()
  }, 10_000)

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

    await userEvent.click(screen.getByRole("button", { name: "写更新" }))
    await userEvent.click(screen.getByRole("button", { name: "添加注解" }))
    expect(addTaskAnnotation).not.toHaveBeenCalled()
    expect(screen.getByText("注解不能为空")).toBeTruthy()

    await userEvent.type(screen.getByLabelText("新增注解"), "failed input")
    await userEvent.click(screen.getByRole("button", { name: "添加注解" }))

    expect(await screen.findByText("scope denied")).toBeTruthy()
    expect(screen.getByLabelText("新增注解").textContent).toContain(
      "failed input"
    )
  })

  it("keeps the annotation composer collapsed until the user starts an update", async () => {
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

    expect(screen.queryByLabelText("新增注解")).toBeNull()

    await userEvent.click(screen.getByRole("button", { name: "写更新" }))
    expect(screen.getByLabelText("新增注解")).toBeTruthy()
  })

  it("localizes the collapsed composer entry in English", async () => {
    await i18n.changeLanguage("en-US")
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

    expect(screen.getByRole("button", { name: "Write update" })).toBeTruthy()
  })

  it("shows the English annotation success feedback after adding", async () => {
    await i18n.changeLanguage("en-US")
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

    await userEvent.click(screen.getByRole("button", { name: "Write update" }))
    await userEvent.type(screen.getByLabelText("New annotation"), "Ready")
    await userEvent.click(
      screen.getByRole("button", { name: "Add annotation" })
    )

    expect((await screen.findByRole("status")).textContent).toContain(
      "Annotation added"
    )
  })

  it("localizes the English annotation failure alert", async () => {
    await i18n.changeLanguage("en-US")
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

    await userEvent.click(screen.getByRole("button", { name: "Write update" }))
    await userEvent.type(screen.getByLabelText("New annotation"), "Ready")
    await userEvent.click(
      screen.getByRole("button", { name: "Add annotation" })
    )

    const alert = await screen.findByRole("alert")
    expect(alert.textContent).toContain("1 edit not saved")
    expect(alert.textContent).not.toContain("个编辑未保存")
    expect(
      screen.getByRole("button", { name: "Dismiss failed Add annotation" })
    ).toBeTruthy()
  })

  it("does not expose annotation controls without write permission", () => {
    render(
      <TaskAnnotationsEditor
        annotations={[{ id: "note-1", description: "只读注解" }]}
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.queryByRole("button", { name: "写更新" })).toBeNull()
    expect(screen.queryByRole("button", { name: "编辑注解" })).toBeNull()
    expect(screen.queryByRole("button", { name: "删除注解" })).toBeNull()
  })

  it("renders annotation markdown", () => {
    render(
      <TaskAnnotationsEditor
        annotations={[{ id: "note-1", description: "**更新**\n\n- 截图" }]}
        canWrite={true}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getByText("更新").tagName.toLowerCase()).toBe("strong")
    expect(screen.getByText("截图")).toBeTruthy()
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
    await userEvent.click(screen.getByRole("button", { name: "删除注解" }))

    expect(deleteTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", "note-1")
  })

  it("edits an annotation and keeps dialog open on failure", async () => {
    vi.mocked(updateTaskAnnotation).mockRejectedValueOnce(new Error("denied"))
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

    await userEvent.click(screen.getByRole("button", { name: "编辑注解" }))
    await userEvent.click(screen.getByLabelText("编辑注解内容"))
    await userEvent.keyboard("{Control>}a{/Control}{Backspace}")
    await userEvent.type(screen.getByLabelText("编辑注解内容"), "更新注解")
    await userEvent.click(screen.getByRole("button", { name: "保存注解" }))

    expect(updateTaskAnnotation).toHaveBeenCalledWith(
      "acme",
      "ads-1",
      "note-1",
      { description: "更新注解" }
    )
    expect(await screen.findByText("denied")).toBeTruthy()
    expect(screen.getByLabelText("编辑注解内容").textContent).toContain(
      "更新注解"
    )
  })
})
