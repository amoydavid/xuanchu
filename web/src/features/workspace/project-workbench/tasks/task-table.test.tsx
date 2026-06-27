import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { ProjectWorkbenchTask } from "../api/project-api"
import {
  deleteTask,
  doneTask,
  modifyTask,
  startTask,
  stopTask,
} from "../api/task-api"
import { TaskTable } from "./task-table"

vi.mock("@tanstack/react-router", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-router")>()
  return {
    ...actual,
    Link: ({
      children,
      params,
      to,
      ...props
    }: {
      children: ReactNode
      params: Record<string, string>
      to: string
    }) => (
      <a
        href={to
          .replace("$workspaceSlug", params.workspaceSlug)
          .replace("$projectSlug", params.projectSlug)
          .replace("$taskRef", params.taskRef)}
        {...props}
      >
        {children}
      </a>
    ),
  }
})

vi.mock("../api/task-api", () => ({
  addTaskAnnotation: vi.fn(),
  addTaskLink: vi.fn(),
  createTask: vi.fn(),
  deleteTask: vi.fn(),
  deleteTaskAnnotation: vi.fn(),
  deleteTaskLink: vi.fn(),
  doneTask: vi.fn(),
  modifyTask: vi.fn(),
  startTask: vi.fn(),
  stopTask: vi.fn(),
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

function task(overrides: Partial<ProjectWorkbenchTask> = {}): ProjectWorkbenchTask {
  return {
    uuid: "task-uuid-1",
    task_slug: "ads-1",
    title: "写投放日报",
    status: "pending",
    project: "adsops",
    priority: "M",
    due: 1783036800,
    assignees: [{ user_id: "user-1", name: "李雷" }],
    ...overrides,
  }
}

function renderTaskTable(tasks: ProjectWorkbenchTask[] = [task()]) {
  return render(
    <TaskTable
      canWrite={true}
      projectSlug="adsops"
      tasks={tasks}
      workspaceSlug="acme"
    />,
    { wrapper: makeWrapper(makeQueryClient()) }
  )
}

describe("TaskTable", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(deleteTask).mockResolvedValue(task())
    vi.mocked(doneTask).mockResolvedValue(task({ status: "completed" }))
    vi.mocked(modifyTask).mockResolvedValue(task())
    vi.mocked(startTask).mockResolvedValue(task({ start: 1782950400 }))
    vi.mocked(stopTask).mockResolvedValue(task())
  })

  it("saves an edited title with task slug ref", async () => {
    renderTaskTable()

    await userEvent.click(screen.getByRole("button", { name: "编辑任务标题 ads-1" }))
    await userEvent.clear(screen.getByRole("textbox", { name: "编辑任务标题 ads-1" }))
    await userEvent.type(
      screen.getByRole("textbox", { name: "编辑任务标题 ads-1" }),
      "写周报{Enter}"
    )

    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", { title: "写周报" })
  })

  it("saves priority from the inline selector", async () => {
    renderTaskTable()

    await userEvent.click(screen.getByRole("combobox", { name: "任务优先级 ads-1" }))
    await userEvent.click(screen.getByRole("option", { name: "H" }))

    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", { priority: "H" })
  })

  it("clears due date with clear_due payload", async () => {
    renderTaskTable()

    await userEvent.clear(screen.getByLabelText("任务截止日期 ads-1"))
    await userEvent.tab()

    await waitFor(() => {
      expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
        clear_due: true,
      })
    })
  })

  it("runs done action for pending tasks", async () => {
    renderTaskTable()

    await userEvent.click(
      within(screen.getByRole("table")).getByRole("button", {
        name: "完成 ads-1",
      })
    )

    expect(doneTask).toHaveBeenCalledWith("acme", "ads-1")
  })

  it("requires confirmation before deleting a task", async () => {
    renderTaskTable()

    await userEvent.click(
      within(screen.getByRole("table")).getByRole("button", {
        name: "更多操作 ads-1",
      })
    )
    await userEvent.click(screen.getByRole("menuitem", { name: "删除任务" }))

    expect(deleteTask).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole("button", { name: "删除" }))

    expect(deleteTask).toHaveBeenCalledWith("acme", "ads-1")
  })
})
