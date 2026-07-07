import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import {
  getProject,
  getProjectTaskSummary,
  getProjectTasks,
  getProjectTimeline,
  type ProjectWorkbenchProject,
  type ProjectWorkbenchTask,
} from "../api/project-api"
import { getWorkspaceMembers } from "../api/users-api"
import { ProjectLayout } from "../project/project-layout"
import { ProjectTasksPage } from "./project-tasks-page"

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({
    data: {
      effective_role: "owner",
      token: {
        scopes: ["project:write", "task:read", "task:write"],
        type: "pat",
      },
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

vi.mock("../api/users-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/users-api")>("../api/users-api")
  return {
    ...actual,
    getWorkspaceMembers: vi.fn(),
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
    task_count: 1,
    pending_count: 1,
    completed_count: 0,
    created_at: 1,
    modified_at: 1,
    ...overrides,
  }
}

function task(
  overrides: Partial<ProjectWorkbenchTask> = {}
): ProjectWorkbenchTask {
  return {
    uuid: "task-uuid-1",
    task_slug: "ops-1",
    title: "写投放日报",
    status: "pending",
    project: "ops",
    priority: "M",
    due: 1783036800,
    ...overrides,
  }
}

describe("ProjectTasksPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getProject).mockResolvedValue(project())
    vi.mocked(getProjectTasks).mockResolvedValue([task()])
    vi.mocked(getProjectTimeline).mockResolvedValue([])
    vi.mocked(getProjectTaskSummary).mockResolvedValue({
      overdue_count: 0,
      overdue_refs: [],
      high_priority_open_count: 0,
      high_priority_open_refs: [],
      wait_ready_count: 0,
      wait_ready_refs: [],
      unassigned_open_count: 0,
      unassigned_open_refs: [],
      workload: [],
    })
    vi.mocked(getWorkspaceMembers).mockResolvedValue([
      {
        display_name: "刘玮",
        email: "liuwei@example.com",
        joined_at: 1,
        modified_at: 1,
        name: "liuwei",
        role: "member",
        user_id: "user-1",
      },
    ])
  })

  it("renders task toolbar and simple table without grouping headings", async () => {
    render(
      <ProjectLayout
        activeTab="tasks"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectTasksPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    expect(await screen.findByRole("table")).toBeTruthy()
    // ops-1 在任务表和右栏都可能出现
    expect(screen.getAllByText("ops-1").length).toBeGreaterThan(0)
    expect(screen.getAllByText("写投放日报").length).toBeGreaterThan(0)
    // 第一阶段不做分组
    expect(screen.queryByText("按状态")).toBeNull()
    expect(screen.queryByText("按负责人")).toBeNull()
  })

  it("shows create and import icon actions when project is writable", async () => {
    render(
      <ProjectLayout
        activeTab="tasks"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectTasksPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    await screen.findByRole("table")
    // 图标按钮（aria-label）+ 工具栏文字按钮都应存在
    expect(screen.getAllByRole("button", { name: "新建任务" }).length).toBeGreaterThan(0)
    expect(screen.getAllByRole("button", { name: "导入任务" }).length).toBeGreaterThan(0)
  })

  it("hides create/import actions when project is closed", async () => {
    vi.mocked(getProject).mockResolvedValue(project({ status: "archived" }))
    render(
      <ProjectLayout
        activeTab="tasks"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectTasksPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    await screen.findByRole("table")
    // 关闭项目：工具栏和图标按钮的新建/导入都隐藏
    expect(screen.queryAllByRole("button", { name: "新建任务" }).length).toBe(0)
    expect(screen.queryAllByRole("button", { name: "导入任务" }).length).toBe(0)
  })
})
