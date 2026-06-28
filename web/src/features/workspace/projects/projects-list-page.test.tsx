import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { ProjectsListPage } from "./projects-list-page"

function renderPage(workspaceSlug = "acme") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <TooltipProvider>
            <ProjectsListPage workspaceSlug={workspaceSlug} />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
  )
}

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

describe("ProjectsListPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("renders project rows with name, status and task counts", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      if (String(input).startsWith("/api/v1/projects")) {
        return ok([
          {
            id: "p1",
            slug: "api",
            name: "API Platform",
            status: "active",
            task_count: 4,
            pending_count: 2,
            completed_count: 2,
          },
          {
            id: "p2",
            slug: "web",
            name: "Web Console",
            status: "pending",
            task_count: 0,
            pending_count: 0,
            completed_count: 0,
          },
        ])
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })

    renderPage()

    await waitFor(() => expect(screen.getByText("API Platform")).toBeTruthy())
    expect(screen.getByText("Web Console")).toBeTruthy()
    expect(screen.getByText("进行中")).toBeTruthy()
    expect(screen.getByText("待处理")).toBeTruthy()
    expect(screen.queryByText("active")).toBeNull()
    expect(screen.queryByText("pending")).toBeNull()
    // 待处理/总数：API Platform → 2 / 4
    expect(screen.getByText("2 / 4")).toBeTruthy()
    // 完成进度 50%
    expect(screen.getByText("50%")).toBeTruthy()
  })

  it("shows empty state when no projects", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() => ok([]))
    renderPage()
    await waitFor(() => expect(screen.getByText("暂无项目")).toBeTruthy())
  })
})
