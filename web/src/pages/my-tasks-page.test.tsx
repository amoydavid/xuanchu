import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { cleanup, render, screen, waitFor } from "@testing-library/react"
import type { MouseEvent, ReactNode } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { getProjects } from "@/features/workspace/project-workbench/api/project-api"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { saveMyTasksReturnState } from "@/features/workspace/my-tasks/my-tasks-return-state"
import { MyTasksPage } from "./my-tasks-page"

const navigateMock = vi.fn()

vi.mock("@tanstack/react-router", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-router")>()
  return {
    ...actual,
    Link: ({
      children,
      onClick,
      params,
      search,
      to,
      ...props
    }: {
      children: ReactNode
      onClick?: (event: MouseEvent<HTMLAnchorElement>) => void
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
        onClick={(event) => {
          event.preventDefault()
          onClick?.(event)
        }}
        {...props}
      >
        {children}
      </a>
    ),
    useNavigate: () => navigateMock,
    useSearch: () => ({
      project: "ops",
      priority: "H",
      q: "复盘",
      sort: "priority",
      tab: "completed",
      task_type: "occurrence",
    }),
  }
})

vi.mock("@/features/workspace/project-workbench/api/project-api", () => ({
  getProjects: vi.fn(),
}))

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
}))

function renderPage(personal = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MyTasksPage
        actor={personal ? { id: "user-1", name: "alice" } : undefined}
        actorType={personal ? "user" : "tenant_access_token"}
        canWrite
        workspaceSlug="local"
      />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
})

describe("MyTasksPage", () => {
  beforeEach(() => {
    sessionStorage.clear()
    vi.clearAllMocks()
    vi.mocked(workspaceApiGet).mockResolvedValue({ items: [] })
    vi.mocked(getProjects).mockResolvedValue([
      {
        id: "project-1",
        workspace_id: "workspace-1",
        slug: "ops",
        name: "运营项目",
        status: "active",
        task_count: 1,
        pending_count: 1,
        completed_count: 0,
        created_at: 1,
        modified_at: 1,
      },
    ])
  })

  it.each([
    ["zh-CN", ["未完成", "今日到期", "逾期", "无截止", "已完成"]],
    ["en-US", ["Incomplete", "Due today", "Overdue", "No due", "Completed"]],
  ])("renders localized preset tabs in %s", async (language, labels) => {
    await i18n.changeLanguage(language)
    renderPage()

    const tablist = screen.getByRole("tablist", {
      name: i18n.t("myTasks.tabsLabel"),
    })
    expect(tablist).toBeTruthy()
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(
      labels
    )
    expect(document.body.textContent).not.toContain("myTasks.tab.")
  })

  it("does not render a status selector that conflicts with the preset", async () => {
    await i18n.changeLanguage("zh-CN")
    renderPage()

    expect(screen.queryByLabelText(i18n.t("common.status"))).toBeNull()
  })

  it("restores filters from route search", async () => {
    await i18n.changeLanguage("zh-CN")
    renderPage()

    expect(
      screen.getByRole("tab", { name: "已完成" }).getAttribute("data-state")
    ).toBe("active")
    expect(
      (screen.getByRole("textbox", { name: "搜索" }) as HTMLInputElement)
        .value
    ).toBe("复盘")
    expect(
      screen.getByRole("combobox", { name: "优先级" }).textContent
    ).toContain("H")
    expect(screen.getByRole("combobox", { name: "排序" }).textContent).toContain(
      "优先级"
    )
    await waitFor(() => {
      expect(screen.getByRole("combobox", { name: "项目" }).textContent).toContain(
        "运营项目"
      )
    })
    expect(
      screen.getByRole("combobox", { name: "任务类型" }).textContent
    ).toContain("循环任务")
  })

  it("restores selection, scroll, and row focus after task data reloads", async () => {
    await i18n.changeLanguage("zh-CN")
    const returnSearch =
      "project=ops&priority=H&q=%E5%A4%8D%E7%9B%98&sort=priority&tab=completed&task_type=occurrence"
    saveMyTasksReturnState(returnSearch, {
      focusId: "task-uuid-1",
      scrollTop: 720,
      selectedIds: ["task-uuid-1"],
    })
    vi.mocked(workspaceApiGet).mockResolvedValue({
      items: [
        {
          id: "task-uuid-1",
          uuid: "task-uuid-1",
          task_slug: "ops-1",
          title: "复盘循环任务",
          status: "completed",
          project: "ops",
          priority: "H",
          due: 1_783_036_800,
          recurrence_info: {
            role: "occurrence",
            series_id: "series-1",
            series_status: "active",
            rule: "daily",
            recurrence_at: 1_783_036_800,
            materialization: "materialized",
          },
        },
      ],
    })
    const scrollTo = vi
      .spyOn(window, "scrollTo")
      .mockImplementation(() => undefined)
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0)
      return 1
    })

    renderPage(true)

    await waitFor(() => {
      expect(
        screen.getAllByRole("checkbox", { name: "选择任务 ops-1" })[0]
          .getAttribute("aria-checked")
      ).toBe("true")
    })
    expect(document.activeElement).toBe(
      document.querySelector('[data-my-task-focus="task-uuid-1"]')
    )
    expect(scrollTo).toHaveBeenCalledWith(0, 720)
  })
})
