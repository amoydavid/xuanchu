import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { ProjectReadonlyPage } from "./project-page"

function renderPage(workspaceSlug = "acme", projectSlug = "agentapi") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <TooltipProvider>
            <ProjectReadonlyPage
              projectSlug={projectSlug}
              workspaceSlug={workspaceSlug}
            />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
  )
}

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

function apiError(status: number, code: string) {
  return Promise.resolve(
    new Response(JSON.stringify({ error: { code } }), { status })
  )
}

describe("ProjectReadonlyPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("loads project details, tasks, and activity from workspace-scoped APIs", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const path = String(input)
        if (path === "/api/v1/projects/agentapi?workspace=acme") {
          return ok({
            id: "project-1",
            workspace_id: "workspace-1",
            slug: "agentapi",
            name: "AI Agent Platform",
            description: "Owns Xuanchu MCP and API work",
            status: "active",
          })
        }
        if (
          path === "/api/v1/tasks?workspace=acme&project=agentapi&limit=200"
        ) {
          return ok([
            {
              uuid: "task-1",
              task_slug: "ag-23",
              title: "Design task.query schema",
              status: "pending",
              priority: "H",
              due: 1_900_000_000,
              assignees: [{ user_id: "u1", name: "张三" }],
            },
            {
              uuid: "task-2",
              task_slug: "ag-24",
              title: "Review SSO redirect flow",
              status: "active",
              assignees: [{ user_id: "u2", name: "李四" }],
            },
          ])
        }
        if (
          path ===
          "/api/v1/projects/agentapi/timeline?workspace=acme&limit=20"
        ) {
          return ok([
            {
              id: "event-1",
              action: "task.created",
              created_by: { name: "张三" },
            },
          ])
        }
        return apiError(404, "route_not_found")
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("acme / agentapi")).toBeTruthy()
    })
    expect(screen.getByText("AI Agent Platform")).toBeTruthy()
    expect(screen.getByText("只读")).toBeTruthy()
    expect(screen.getAllByText("Design task.query schema").length).toBeGreaterThan(0)
    expect(screen.getAllByText("Review SSO redirect flow").length).toBeGreaterThan(0)
    expect(
      screen
        .getAllByRole("link", { name: /Design task.query schema/ })[0]
        ?.getAttribute("href")
    ).toBe("/workspaces/acme/projects/agentapi/tasks/ag-23")
    expect(screen.getByText("负责人摘要")).toBeTruthy()
    expect(screen.getByText("最近动态")).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/projects/agentapi?workspace=acme",
      expect.any(Object)
    )
  })

  it("shows an empty project state when the project has no tasks", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const path = String(input)
      if (path === "/api/v1/projects/agentapi?workspace=acme") {
        return ok({
          id: "project-1",
          workspace_id: "workspace-1",
          slug: "agentapi",
          name: "AI Agent Platform",
          status: "active",
        })
      }
      if (path === "/api/v1/tasks?workspace=acme&project=agentapi&limit=200") {
        return ok([])
      }
      return ok([])
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("这个项目还没有任务")).toBeTruthy()
    })
    expect(
      screen.getByText(
        'xuanchu --workspace acme add "Design API" project:agentapi'
      )
    ).toBeTruthy()
  })

  it("shows a permission state when the project cannot be read", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      if (String(input) === "/api/v1/projects/agentapi?workspace=acme") {
        return apiError(403, "permission_denied")
      }
      return ok([])
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("不能访问这个项目")).toBeTruthy()
    })
    expect(screen.getByText("project:read, task:read")).toBeTruthy()
  })

  it("shows a not found state when the project is not visible", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      if (String(input) === "/api/v1/projects/agentapi?workspace=acme") {
        return apiError(404, "project_not_found")
      }
      return ok([])
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("项目不存在或不可见")).toBeTruthy()
    })
  })
})
