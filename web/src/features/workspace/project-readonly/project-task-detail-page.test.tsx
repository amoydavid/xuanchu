import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { ProjectTaskDetailPage } from "./project-task-detail-page"

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    renderWithRouter(
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

  it("shows first 3 annotations with load-more, and renders links + UDAs", async () => {
    const annotations = Array.from({ length: 5 }, (_, i) => ({
      id: `note-${i}`,
      entry: "2026-06-01T00:00:00Z",
      description: `note-${i}`,
    }))
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const path = String(input)
      if (path.startsWith("/api/v1/tasks/ag-23/annotations")) {
        // 模拟第二页：offset=3 limit=10 → 返回剩余 2 条
        return ok({
          annotations: annotations.slice(3),
          total: 5,
          offset: 3,
          limit: 10,
        })
      }
      return ok({
        uuid: "task-1",
        task_slug: "ag-23",
        description: "Task with links and UDAs",
        status: "pending",
        project: "agentapi",
        annotations,
        links: [
          {
            id: "link-1",
            type: "spec",
            url: "https://example.com/spec.md",
            title: "Design doc",
            created_by: { id: "u1", name: "张三" },
          },
        ],
        estimate: "4h",
        sprint: "26W24",
      })
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("Task with links and UDAs")).toBeTruthy()
    })
    // 首屏只渲染前 3 条注解
    expect(screen.getByText("note-0")).toBeTruthy()
    expect(screen.getByText("note-2")).toBeTruthy()
    expect(screen.queryByText("note-3")).toBeNull()
    // 「查看更多 2 条」按钮存在
    const moreBtn = screen.getByText("查看更多 2 条 ↓")
    expect(moreBtn).toBeTruthy()
    // links 渲染
    expect(screen.getByText("Design doc")).toBeTruthy()
    // UDA 渲染
    expect(screen.getByText("4h")).toBeTruthy()
    expect(screen.getByText("26W24")).toBeTruthy()

    // 点击查看更多，触发分页请求并显示剩余注解
    await act(async () => {
      moreBtn.click()
    })
    await waitFor(() => {
      expect(screen.getByText("note-3")).toBeTruthy()
      expect(screen.getByText("note-4")).toBeTruthy()
    })
  })

  it("renders depends as clickable links using depends_info, not raw UUIDs", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      ok({
        uuid: "task-1",
        task_slug: "ag-23",
        description: "Task with dependency",
        status: "pending",
        project: "agentapi",
        depends: ["dep-uuid-1"],
        depends_info: [
          {
            uuid: "dep-uuid-1",
            description: "Dependency task",
            task_slug: "ag-12",
          },
        ],
      })
    )

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("Task with dependency")).toBeTruthy()
    })
    // 依赖以描述作标题链接渲染，而非裸 UUID
    const depLink = screen.getByText("Dependency task").closest("a")
    expect(depLink).toBeTruthy()
    expect(depLink?.getAttribute("href")).toBe(
      "/workspaces/acme/projects/agentapi/tasks/ag-12"
    )
    // task_slug 作为小字 label 展示
    expect(screen.getByText("ag-12")).toBeTruthy()
    // 不应直接渲染裸 UUID
    expect(screen.queryByText("dep-uuid-1")).toBeNull()
  })

  it("renders reverse dependency (blocking) section when blocked_by_info present", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      ok({
        uuid: "task-1",
        task_slug: "ag-23",
        description: "Task that blocks others",
        status: "pending",
        project: "agentapi",
        blocked_by_info: [
          {
            uuid: "blocked-uuid-1",
            description: "Task waiting on me",
            task_slug: "ag-12",
          },
        ],
      })
    )

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("Task that blocks others")).toBeTruthy()
    })
    // 「阻塞了」反向关系渲染为可点击链接
    const blockedLink = screen.getByText("Task waiting on me").closest("a")
    expect(blockedLink).toBeTruthy()
    expect(blockedLink?.getAttribute("href")).toBe(
      "/workspaces/acme/projects/agentapi/tasks/ag-12"
    )
    expect(screen.queryByText("blocked-uuid-1")).toBeNull()
  })
})
