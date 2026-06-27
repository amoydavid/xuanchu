import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  deleteTask,
  doneTask,
  getTask,
  modifyTask,
  startTask,
  stopTask,
} from "../api/task-api"
import { TaskDetailPage } from "./task-detail-page"

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({
    data: {
      effective_role: "member",
      token: { scopes: ["task:write"], type: "pat" },
    },
  }),
}))

vi.mock("../api/task-api", async () => {
  const actual = await vi.importActual<typeof import("../api/task-api")>(
    "../api/task-api"
  )
  return {
    ...actual,
    deleteTask: vi.fn(),
    doneTask: vi.fn(),
    getTask: vi.fn(),
    modifyTask: vi.fn(),
    startTask: vi.fn(),
    stopTask: vi.fn(),
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

function task(overrides: Record<string, unknown> = {}) {
  return {
    uuid: "task-1",
    task_slug: "ag-23",
    title: "写投放日报",
    description: "整理素材表现",
    status: "pending",
    project: "agentapi",
    priority: "H",
    tags: ["ads"],
    annotations: [{ id: "note-1", description: "需要素材截图" }],
    links: [{ id: "link-1", type: "spec", url: "https://example.com" }],
    ...overrides,
  }
}

function renderPage() {
  render(
    <TaskDetailPage
      projectSlug="agentapi"
      taskRef="ag-23"
      workspaceSlug="acme"
    />,
    { wrapper: makeWrapper(makeQueryClient()) }
  )
}

describe("TaskDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getTask).mockResolvedValue(task())
    vi.mocked(modifyTask).mockResolvedValue(task())
    vi.mocked(startTask).mockResolvedValue(task({ start: 1_900_000_000 }))
    vi.mocked(stopTask).mockResolvedValue(task({ start: null }))
    vi.mocked(doneTask).mockResolvedValue(task({ status: "completed" }))
    vi.mocked(deleteTask).mockResolvedValue(task({ status: "deleted" }))
  })

  it("saves the title inline", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    await userEvent.click(screen.getByRole("button", { name: "任务标题" }))
    await userEvent.clear(screen.getByLabelText("任务标题"))
    await userEvent.type(screen.getByLabelText("任务标题"), "写周报{Enter}")

    expect(modifyTask).toHaveBeenCalledWith("acme", "ag-23", {
      title: "写周报",
    })
  })

  it("clears description with clear_description", async () => {
    renderPage()

    await screen.findByText("整理素材表现")
    await userEvent.click(screen.getByRole("button", { name: "任务描述" }))
    await userEvent.clear(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    expect(modifyTask).toHaveBeenCalledWith("acme", "ag-23", {
      clear_description: true,
    })
  })

  it("runs start, done, and delete actions", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    await userEvent.click(screen.getByRole("button", { name: "开始" }))
    expect(startTask).toHaveBeenCalledWith("acme", "ag-23")

    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(doneTask).toHaveBeenCalledWith("acme", "ag-23")

    await userEvent.click(screen.getByRole("button", { name: "删除" }))
    expect(screen.getByText("确认删除任务")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "删除任务" }))
    expect(deleteTask).toHaveBeenCalledWith("acme", "ag-23")
  })

  it("does not show lifecycle actions for completed tasks", async () => {
    vi.mocked(getTask).mockResolvedValue(task({ status: "completed" }))

    renderPage()

    await screen.findByText("写投放日报")
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "开始" })).toBeNull()
      expect(screen.queryByRole("button", { name: "停止" })).toBeNull()
      expect(screen.queryByRole("button", { name: "完成" })).toBeNull()
    })
  })
})
