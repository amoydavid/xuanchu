import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState, type MouseEvent, type ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import type { ProjectWorkbenchTask } from "@/features/workspace/project-workbench/api/project-api"
import {
  deleteTask,
  doneTask,
  modifyTask,
} from "@/features/workspace/project-workbench/api/task-api"
import { MyTasksTable } from "./my-tasks-table"
import { takeMyTasksReturnState } from "./my-tasks-return-state"

vi.mock("@/features/workspace/project-workbench/api/task-api", () => ({
  deleteTask: vi.fn(),
  doneTask: vi.fn(),
  modifyTask: vi.fn(),
}))

vi.mock("@tanstack/react-router", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-router")>()
  return {
    ...actual,
    Link: ({
      children,
      params,
      search,
      to,
      onClick,
      ...props
    }: {
      children: ReactNode
      params: Record<string, string>
      search?: Record<string, string>
      to: string
      onClick?: (event: MouseEvent<HTMLAnchorElement>) => void
    }) => (
      <a
        href={to
          .replace("$workspaceSlug", params.workspaceSlug)
          .replace("$projectSlug", params.projectSlug)
          .replace("$taskRef", params.taskRef)}
        data-search={search ? JSON.stringify(search) : undefined}
        onClick={(event) => {
          event.preventDefault()
          onClick?.(event)
        }}
        {...props}
      >
        {children}
      </a>
    ),
  }
})

function task(
  overrides: Partial<ProjectWorkbenchTask> = {}
): ProjectWorkbenchTask {
  return {
    uuid: "task-uuid-1",
    task_slug: "ops-1",
    title: "跟进客户回访",
    status: "pending",
    project: "ops",
    priority: "M",
    due: 1783036800,
    ...overrides,
  }
}

function renderTable(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>{node}</QueryClientProvider>
  )
}

function SelectableTable({
  returnSearch = "tab=incomplete&sort=due",
  tasks,
}: {
  returnSearch?: string
  tasks: ProjectWorkbenchTask[]
}) {
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  return (
    <MyTasksTable
      canWrite
      onSelectedIdsChange={setSelectedIds}
      returnSearch={returnSearch}
      selectedIds={selectedIds}
      tasks={tasks}
      workspaceSlug="local"
    />
  )
}

describe("MyTasksTable", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    vi.clearAllMocks()
    vi.mocked(doneTask).mockImplementation(async (_workspace, taskRef) =>
      task({ uuid: taskRef, status: "completed" })
    )
    vi.mocked(deleteTask).mockImplementation(async (_workspace, taskRef) =>
      task({ uuid: taskRef, status: "deleted" })
    )
    vi.mocked(modifyTask).mockImplementation(async (_workspace, taskRef) =>
      task({ uuid: taskRef })
    )
    await i18n.changeLanguage("zh-CN")
  })

  it("keeps selected rows in return state before opening task details", async () => {
    const user = userEvent.setup()
    const tasks = [
      task(),
      task({ uuid: "task-uuid-2", task_slug: "ops-2", title: "每日巡检" }),
    ]
    renderTable(<SelectableTable tasks={tasks} />)

    await user.click(
      screen.getAllByRole("checkbox", { name: "选择任务 ops-1" })[0]
    )
    await user.click(
      screen.getAllByRole("checkbox", { name: "选择任务 ops-2" })[0]
    )
    await user.click(screen.getAllByRole("link", { name: "ops-1" })[0])
    expect(takeMyTasksReturnState("tab=incomplete&sort=due")).toEqual({
      focusId: "task-uuid-1",
      scrollTop: window.scrollY,
      selectedIds: ["task-uuid-1", "task-uuid-2"],
    })
  })

  it("completes every selected open task", async () => {
    const user = userEvent.setup()
    renderTable(
      <SelectableTable
        tasks={[
          task(),
          task({ uuid: "task-uuid-2", task_slug: "ops-2", title: "每日巡检" }),
        ]}
      />
    )

    await user.click(
      screen.getAllByRole("checkbox", { name: "选择任务 ops-1" })[0]
    )
    await user.click(
      screen.getAllByRole("checkbox", { name: "选择任务 ops-2" })[0]
    )
    await user.click(screen.getByRole("button", { name: "完成选中的 2 项任务" }))

    expect(doneTask).toHaveBeenCalledTimes(2)
    expect(doneTask).toHaveBeenNthCalledWith(1, "local", "ops-1")
    expect(doneTask).toHaveBeenNthCalledWith(2, "local", "ops-2")
  })

  it("explains mixed delete scope before deleting tasks and skipping occurrences", async () => {
    const user = userEvent.setup()
    renderTable(
      <SelectableTable
        tasks={[
          task(),
          task({
            id: "occ:series-1:1784476799",
            uuid: undefined,
            task_slug: undefined,
            title: "每日巡检",
            recurrence_info: {
              role: "occurrence",
              series_id: "series-1",
              series_status: "active",
              rule: "daily",
              recurrence_at: 1_784_476_799,
              materialization: "projected",
            },
          }),
        ]}
      />
    )

    await user.click(
      screen.getAllByRole("checkbox", { name: "选择任务 ops-1" })[0]
    )
    await user.click(
      screen.getAllByRole("checkbox", { name: "选择任务 ↻07-19" })[0]
    )
    await user.click(screen.getByRole("button", { name: "删除或跳过选中的 2 项" }))

    expect(
      screen.getByText("将删除 1 个普通任务，并跳过 1 次循环任务。")
    ).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "确认删除或跳过" }))

    expect(deleteTask).toHaveBeenCalledTimes(2)
    expect(deleteTask).toHaveBeenNthCalledWith(1, "local", "ops-1")
    expect(deleteTask).toHaveBeenNthCalledWith(
      2,
      "local",
      "occ:series-1:1784476799"
    )
  })

  it("captures scroll and row focus before opening task details", async () => {
    Object.defineProperty(window, "scrollY", {
      configurable: true,
      value: 640,
    })
    const returnSearch = "tab=overdue&project=ops&sort=due"
    renderTable(
      <MyTasksTable
        returnSearch={returnSearch}
        tasks={[task()]}
        workspaceSlug="local"
      />
    )

    await userEvent.click(screen.getAllByRole("link", { name: "ops-1" })[0])

    expect(takeMyTasksReturnState(returnSearch)).toEqual({
      focusId: "task-uuid-1",
      scrollTop: 640,
      selectedIds: [],
    })
  })

  it("links task refs to the project-scoped task detail route", () => {
    renderTable(
      <MyTasksTable
        canWrite
        returnSearch="tab=completed&priority=H&q=review&sort=due"
        tasks={[task()]}
        workspaceSlug="local"
      />
    )

    const taskLinks = screen.getAllByRole("link", { name: "ops-1" })
    expect(taskLinks).toHaveLength(2)
    for (const link of taskLinks) {
      expect(link.getAttribute("href")).toBe(
        "/workspaces/local/projects/ops/tasks/ops-1"
      )
      expect(JSON.parse(link.getAttribute("data-search") ?? "{}")).toEqual({
        from: "my-tasks",
        my_tasks_search: "tab=completed&priority=H&q=review&sort=due",
      })
    }
  })

  it("does not expose occurrence_ref as the projected instance label", () => {
    const occurrenceRef = "occ:series-1:1784476799"
    renderTable(
      <MyTasksTable
        canWrite
        tasks={[
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
        ]}
        workspaceSlug="local"
      />
    )

    const links = screen.getAllByRole("link", { name: "↻07-19" })
    expect(links).toHaveLength(2)
	for (const link of links)
	  expect(link.getAttribute("href")).toContain(occurrenceRef)
	expect(screen.getAllByText("计划实例").length).toBeGreaterThan(0)
    expect(screen.queryByText(occurrenceRef)).toBeNull()
  })

  it("shows occurrence actions and preserves the My Tasks source in the Series link", async () => {
    renderTable(
      <MyTasksTable
        canWrite
        returnSearch="tab=incomplete&project=ops&task_type=occurrence&sort=due"
        tasks={[
          task({
            status: "waiting",
            recurrence_info: {
              role: "occurrence",
              series_id: "series-1",
              series_title: "每日巡检",
              series_status: "active",
              rule: "daily",
              recurrence_at: 1_783_036_800,
              materialization: "materialized",
            },
          }),
        ]}
        workspaceSlug="local"
      />
    )

    expect(screen.queryAllByRole("button", { name: /开始本次/ })).toHaveLength(0)
    expect(screen.getAllByRole("button", { name: /完成本次/ }).length).toBeGreaterThan(0)

    await userEvent.click(screen.getAllByRole("button", { name: /更多操作/ })[0])
    const seriesLink = screen.getByRole("menuitem", { name: "查看循环任务" })
    expect(JSON.parse(seriesLink.getAttribute("data-search") ?? "{}")).toEqual({
      panel_return_scope: "project",
      panel_return_search:
        "tab=incomplete&project=ops&task_type=occurrence&sort=due",
      panel_return_source: "my-tasks",
      panel_return_task: "ops-1",
    })
  })
})
