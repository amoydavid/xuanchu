import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
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
          .replace("$taskRef", params.taskRef)
          .replace("$seriesRef", params.seriesRef)}
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

function task(
  overrides: Partial<ProjectWorkbenchTask> = {}
): ProjectWorkbenchTask {
  return {
    uuid: "task-uuid-1",
    task_slug: "ads-1",
    title: "写投放日报",
    status: "pending",
    project: "adsops",
    priority: "M",
    due: 1783036800,
    assignees: [{ id: "user-1", name: "李雷" }],
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
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(deleteTask).mockResolvedValue(task())
    vi.mocked(doneTask).mockResolvedValue(task({ status: "completed" }))
    vi.mocked(modifyTask).mockResolvedValue(task())
    vi.mocked(startTask).mockResolvedValue(task({ start: 1782950400 }))
    vi.mocked(stopTask).mockResolvedValue(task())
  })

  it("prefers display name when rendering assignees", () => {
    renderTaskTable([
      task({
        assignees: [
          { id: "user-1", name: "stable-name", display_name: "李雷" },
        ],
      }),
    ])

    expect(screen.getAllByText("李雷").length).toBeGreaterThan(0)
    expect(screen.queryByText("stable-name")).toBeNull()
  })

  it("saves an edited title with task slug ref", async () => {
    renderTaskTable()

    await userEvent.click(
      screen.getByRole("button", { name: "编辑任务标题 ads-1" })
    )
    await userEvent.clear(
      screen.getByRole("textbox", { name: "编辑任务标题 ads-1" })
    )
    await userEvent.type(
      screen.getByRole("textbox", { name: "编辑任务标题 ads-1" }),
      "写周报{Enter}"
    )

    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      title: "写周报",
    })
  })

  it("saves priority from the inline selector", async () => {
    renderTaskTable()

    await userEvent.click(
      screen.getByRole("combobox", { name: "任务优先级 ads-1" })
    )
    await userEvent.click(screen.getByRole("option", { name: "H" }))

    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", { priority: "H" })
  })

  it("clears due date with clear_due payload", async () => {
    renderTaskTable()

    await userEvent.click(
      within(screen.getByRole("table")).getByRole("button", {
        name: "任务截止日期 ads-1",
      })
    )
    await userEvent.click(
      await screen.findByRole("button", { name: "清除日期" })
    )

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

  it("supports high frequency edits from the mobile task card", async () => {
    renderTaskTable()

    await userEvent.click(
      screen.getByRole("button", { name: "编辑移动任务标题 ads-1" })
    )
    await userEvent.clear(
      screen.getByRole("textbox", { name: "编辑移动任务标题 ads-1" })
    )
    await userEvent.type(
      screen.getByRole("textbox", { name: "编辑移动任务标题 ads-1" }),
      "手机改标题{Enter}"
    )
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      title: "手机改标题",
    })

    await userEvent.click(
      screen.getByRole("combobox", { name: "移动任务优先级 ads-1" })
    )
    await userEvent.click(screen.getByRole("option", { name: "H" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", { priority: "H" })

    await userEvent.click(
      screen.getByRole("button", { name: "移动任务截止日期 ads-1" })
    )
    await userEvent.click(
      await screen.findByRole("button", { name: "清除日期" })
    )
    await waitFor(() => {
      expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
        clear_due: true,
      })
    })
  })

  it("uses localized task action labels", async () => {
    await i18n.changeLanguage("en-US")
    renderTaskTable()

    await userEvent.click(
      within(screen.getByRole("table")).getByRole("button", {
        name: "More actions ads-1",
      })
    )
    expect(screen.getByRole("menuitem", { name: "Open details" })).toBeTruthy()
    expect(
      screen.getByRole("menuitem", { name: "Copy task link" })
    ).toBeTruthy()
    expect(screen.getByRole("menuitem", { name: "Delete task" })).toBeTruthy()
  })

  it("localizes recurring-task badges in English", async () => {
    await i18n.changeLanguage("en-US")
    renderTaskTable([
      task({
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "projected",
        },
      }),
    ])

    const badges = screen.getAllByTestId("recurrence-badge")
    expect(badges[0].textContent).toContain("Recurring · Daily")
    expect(badges[0].getAttribute("title")).toBe("Planned instance")
    expect(screen.queryByText("每天")).toBeNull()
  })

  it("uses a planned date in projected occurrence labels while keeping the stable id only in routes", async () => {
    const occurrenceRef = "occ:series-1:1784476799"
    renderTaskTable([
      task({
        id: occurrenceRef,
        uuid: undefined,
        task_slug: undefined,
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_784_476_799,
          materialization: "projected",
        },
      }),
    ])

    const links = screen.getAllByRole("link", { name: "↻07-19" })
    expect(links).toHaveLength(2)
	for (const link of links) {
	  expect(link.getAttribute("href")).toContain(occurrenceRef)
	}
	expect(screen.getAllByText("计划实例").length).toBeGreaterThan(0)
	expect(screen.queryByText(occurrenceRef)).toBeNull()
    expect(
      within(screen.getByRole("table")).getByRole("button", {
        name: "编辑任务标题 ↻07-19",
      })
    ).toBeTruthy()
    expect(
      within(screen.getByRole("table")).getByRole("button", {
        name: "完成本次：2026年7月19日",
      })
    ).toBeTruthy()

    await userEvent.click(
      within(screen.getByRole("table")).getByRole("button", {
        name: "更多操作 ↻07-19",
      })
    )
    expect(screen.getByRole("menuitem", { name: "跳过本次" })).toBeTruthy()
    const seriesLink = screen.getByRole("menuitem", { name: "查看循环任务" })
    expect(seriesLink.getAttribute("href")).toContain("/series/series-1")

    await userEvent.click(screen.getByRole("menuitem", { name: "跳过本次" }))
    expect(screen.getByText("跳过 2026年7月19日 这一次？")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "跳过本次" }))
    expect(deleteTask).toHaveBeenCalledWith("acme", occurrenceRef)
  })

  it("exposes sortable table headers", async () => {
    const onSortChange = vi.fn()
    render(
      <TaskTable
        canWrite={true}
        onSortChange={onSortChange}
        projectSlug="adsops"
        sort="due"
        tasks={[task()]}
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(
      screen
        .getByRole("button", { name: "按到期排序" })
        .getAttribute("aria-pressed")
    ).toBe("true")
    await userEvent.click(screen.getByRole("button", { name: "按标识排序" }))
    expect(onSortChange).toHaveBeenCalledWith("entry")
  })
})
