import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { transitionProject } from "../api/project-api"
import { ProjectStatusMenu } from "./project-status-menu"

vi.mock("../api/project-api", async () => {
  const actual = await vi.importActual<typeof import("../api/project-api")>(
    "../api/project-api"
  )
  return {
    ...actual,
    transitionProject: vi.fn(),
  }
})

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

function project(status = "planning") {
  return {
    id: "project-1",
    workspace_id: "workspace-1",
    slug: "adsops",
    name: "广告投放自动化",
    status,
    task_count: 0,
    pending_count: 0,
    completed_count: 0,
    created_at: 1,
    modified_at: 1,
  }
}

describe("ProjectStatusMenu", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(transitionProject).mockResolvedValue(project("active"))
  })

  it("transitions from planning to active directly", async () => {
    render(
      <ProjectStatusMenu
        canManage={true}
        project={project("planning")}
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getByRole("button", { name: "项目状态 规划中" })).toBeTruthy()
    expect(screen.queryByText("planning")).toBeNull()

    await userEvent.click(screen.getByRole("button", { name: /项目状态/ }))
    await userEvent.click(screen.getByRole("menuitem", { name: "进行中" }))

    expect(transitionProject).toHaveBeenCalledWith("acme", "adsops", "active")
  })

  it("asks for confirmation before moving into archived", async () => {
    render(
      <ProjectStatusMenu
        canManage={true}
        project={project("active")}
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: /项目状态/ }))
    await userEvent.click(screen.getByRole("menuitem", { name: "已归档" }))

    expect(screen.getByText("确认关闭项目")).toBeTruthy()
    expect(transitionProject).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole("button", { name: "关闭项目" }))

    expect(transitionProject).toHaveBeenCalledWith("acme", "adsops", "archived")
  })

  it("shows restore copy and transitions from archived to active", async () => {
    render(
      <ProjectStatusMenu
        canManage={true}
        project={project("archived")}
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getByRole("button", { name: "项目状态 已归档" })).toBeTruthy()

    await userEvent.click(screen.getByRole("button", { name: /项目状态/ }))

    expect(screen.getByText("恢复后项目内任务将重新可写")).toBeTruthy()

    await userEvent.click(screen.getByRole("menuitem", { name: "进行中" }))

    expect(transitionProject).toHaveBeenCalledWith("acme", "adsops", "active")
  })
})
