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

function task(overrides: Partial<ProjectWorkbenchTask> = {}): ProjectWorkbenchTask {
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
    }
  })
})
