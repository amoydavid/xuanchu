import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { listProjectEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import { renderWithRouter } from "@/test/router-wrapper"

import {
  getProject,
  getProjectTaskSummary,
  getProjectTasks,
  getProjectTimeline,
  type ProjectWorkbenchProject,
} from "../api/project-api"
import { ProjectLayout } from "./project-layout"

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({
    data: {
      effective_role: "owner",
      token: { scopes: ["project:write", "task:read", "task:write"], type: "pat" },
    },
  }),
}))

vi.mock("../api/project-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/project-api")>("../api/project-api")
  return {
    ...actual,
    getProject: vi.fn(),
    getProjectTasks: vi.fn(),
    getProjectTimeline: vi.fn(),
    getProjectTaskSummary: vi.fn(),
  }
})

vi.mock("@/features/workspace/config/config-definition-api", async () => {
  const actual =
    await vi.importActual<
      typeof import("@/features/workspace/config/config-definition-api")
    >("@/features/workspace/config/config-definition-api")
  return {
    ...actual,
    listProjectEffectiveConfig: vi.fn(),
  }
})

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={makeQueryClient()}>
      {renderWithRouter(<>{children}</>)}
    </QueryClientProvider>
  )
}

function project(
  overrides: Partial<ProjectWorkbenchProject> = {}
): ProjectWorkbenchProject {
  return {
    id: "project-1",
    workspace_id: "workspace-1",
    slug: "ops",
    name: "运营项目",
    status: "active",
    task_count: 3,
    pending_count: 2,
    completed_count: 1,
    created_at: 1_800_000_000,
    modified_at: 1_800_001_000,
    ...overrides,
  }
}

describe("ProjectLayout", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getProject).mockResolvedValue(project())
    vi.mocked(getProjectTasks).mockResolvedValue([])
    vi.mocked(getProjectTimeline).mockResolvedValue([])
    vi.mocked(getProjectTaskSummary).mockResolvedValue({
      overdue_count: 1,
      overdue_refs: [],
      high_priority_open_count: 2,
      high_priority_open_refs: [],
      wait_ready_count: 0,
      wait_ready_refs: [],
      unassigned_open_count: 1,
      unassigned_open_refs: [],
      workload: [],
    })
    vi.mocked(listProjectEffectiveConfig).mockResolvedValue([])
  })

  it("renders header, tabs and child content", async () => {
    render(
      <ProjectLayout
        activeTab="overview"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <div>概览主体</div>
      </ProjectLayout>,
      { wrapper: Wrapper }
    )

    expect(await screen.findByText("概览主体")).toBeTruthy()
    const overviewLink = screen.getByRole("link", { name: "概览" }) as HTMLAnchorElement
    expect(overviewLink.getAttribute("href")).toBe("/workspaces/local/projects/ops")
    const tasksLink = screen.getByRole("link", { name: "任务" }) as HTMLAnchorElement
    expect(tasksLink.getAttribute("href")).toBe("/workspaces/local/projects/ops/tasks")
    const activityLink = screen.getByRole("link", { name: "活动" }) as HTMLAnchorElement
    expect(activityLink.getAttribute("href")).toBe("/workspaces/local/projects/ops/activity")
  })

  it("shows the shared project header with status and copy action", async () => {
    render(
      <ProjectLayout
        activeTab="tasks"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <div />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    expect(await screen.findByText("运营项目")).toBeTruthy()
    // 状态菜单按钮 aria-label 含项目状态
    expect(screen.getByRole("button", { name: /项目状态/ })).toBeTruthy()
  })

  it("renders the context rail with summary risk counts", async () => {
    render(
      <ProjectLayout
        activeTab="overview"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <div />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    expect(await screen.findByText("项目信息")).toBeTruthy()
    expect(screen.getByText("逾期")).toBeTruthy()
    expect(screen.getByText("高优未完成")).toBeTruthy()
  })

  it("collapses and expands the rail from the tabs row button", async () => {
    const user = userEvent.setup()
    render(
      <ProjectLayout
        activeTab="overview"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <div />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    // 初始 rail 打开：项目信息可见，收起按钮存在
    expect(await screen.findByText("项目信息")).toBeTruthy()
    expect(screen.getByRole("button", { name: "收起右栏" })).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "收起右栏" }))
    // 收起后右栏整块不渲染，只保留展开按钮
    expect(screen.queryByText("项目信息")).toBeNull()
    expect(screen.getByRole("button", { name: "展开右栏" })).toBeTruthy()
    // 再次点击展开恢复右栏
    await user.click(screen.getByRole("button", { name: "展开右栏" }))
    expect(screen.getByText("项目信息")).toBeTruthy()
  })
})
