import { render, screen } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import type { ProjectWorkbenchTask } from "@/features/workspace/project-workbench/api/project-api"
import { MyTasksTable } from "./my-tasks-table"

vi.mock("@tanstack/react-router", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-router")>()
  return {
    ...actual,
    Link: ({
      children,
      params,
      search,
      to,
      ...props
    }: {
      children: ReactNode
      params: Record<string, string>
      search?: Record<string, string>
      to: string
    }) => (
      <a
        href={to
          .replace("$workspaceSlug", params.workspaceSlug)
          .replace("$projectSlug", params.projectSlug)
          .replace("$taskRef", params.taskRef)}
        data-search={search ? JSON.stringify(search) : undefined}
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

describe("MyTasksTable", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("links task refs to the project-scoped task detail route", () => {
    render(
      <MyTasksTable
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
    render(
      <MyTasksTable
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
})
