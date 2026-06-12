import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { ProjectTaskDetailPage } from "./project-task-detail-page"

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <ProjectTaskDetailPage
            projectSlug="agentapi"
            taskRef="ag-23"
            workspaceSlug="acme"
          />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

describe("ProjectTaskDetailPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("loads a task detail and provides a link back to the project", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      ok({
        uuid: "task-1",
        task_slug: "ag-23",
        description: "Design task.query schema",
        status: "pending",
        priority: "H",
        due: 1_900_000_000,
        project: "agentapi",
        tags: ["mcp", "api"],
        assignees: [{ user_id: "u1", name: "张三" }],
        annotations: [{ id: "note-1", description: "Need schema review" }],
        depends: ["ag-12"],
      })
    )

    renderPage()

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Design task.query schema" })).toBeTruthy()
    })
    expect(screen.getByText("acme / agentapi / ag-23")).toBeTruthy()
    expect(screen.getAllByText("pending").length).toBeGreaterThan(0)
    expect(screen.getAllByText("H").length).toBeGreaterThan(0)
    expect(screen.getByText("张三")).toBeTruthy()
    expect(screen.getByText("Need schema review")).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "返回项目" }).getAttribute("href")
    ).toBe("/workspaces/acme/projects/agentapi")
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/tasks/ag-23?workspace=acme",
      expect.any(Object)
    )
  })

  it("does not render a task from a different project under this project URL", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      ok({
        uuid: "task-1",
        task_slug: "other-23",
        description: "Other project task",
        status: "pending",
        project: "other",
      })
    )

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("任务不存在或不可见")).toBeTruthy()
    })
    expect(screen.queryByRole("heading", { name: "Other project task" })).toBeNull()
    expect(
      screen.getByRole("link", { name: "返回项目" }).getAttribute("href")
    ).toBe("/workspaces/acme/projects/agentapi")
  })
})
