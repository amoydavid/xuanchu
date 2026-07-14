import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { listProjectEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import { renderWithRouter } from "@/test/router-wrapper"

import {
  getProject,
  getProjectTaskSummary,
  getProjectTimeline,
  type ProjectWorkbenchProject,
} from "../api/project-api"
import { ProjectLayout } from "./project-layout"
import { ProjectOverviewPage } from "./project-overview-page"

// 这里的 useMe 默认 owner；通过单独的 describe 覆盖无 task:read 场景。
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
    task_count: 0,
    pending_count: 0,
    completed_count: 0,
    created_at: 1,
    modified_at: 1,
    ...overrides,
  }
}

describe("ProjectOverviewPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getProject).mockResolvedValue(project())
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
    vi.mocked(listProjectEffectiveConfig).mockResolvedValue([])
  })

  it("renders focus from task summary with task refs", async () => {
    vi.mocked(getProjectTaskSummary).mockResolvedValue({
      overdue_count: 2,
      overdue_refs: [
        { uuid: "u7", task_slug: "ops-7", title: "回调", label: "ops-7" },
        { uuid: "u9", task_slug: "ops-9", title: "验收", label: "ops-9" },
      ],
      high_priority_open_count: 0,
      high_priority_open_refs: [],
      wait_ready_count: 0,
      wait_ready_refs: [],
      unassigned_open_count: 0,
      unassigned_open_refs: [],
      workload: [],
    })
    render(
      <ProjectLayout
        activeTab="overview"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectOverviewPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    expect(await screen.findByText("当前重点")).toBeTruthy()
    // 两个 ref 用「、」连接
    expect(screen.getByText("ops-7、ops-9")).toBeTruthy()
  })

  it("shows normal progress and recurring runtime as separate execution signals", async () => {
    vi.mocked(getProject).mockResolvedValue(
      project({ task_count: 10, completed_count: 4, pending_count: 6 })
    )
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
      series_metrics: {
        recurring_series_count: 2,
        active_recurring_series_count: 1,
        open_recurring_occurrence_count: 3,
        overdue_recurring_occurrence_count: 1,
      },
    })
    render(
      <ProjectLayout
        activeTab="overview"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectOverviewPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )

    await screen.findByText("执行概览")
    expect(screen.getByText("任务：4/10 已完成（40%）")).toBeTruthy()
    expect(
      screen.getByText("循环任务：1 个运行中系列，3 条未完成实例")
    ).toBeTruthy()
  })

  it("hides execution overview when no normal or recurring work exists", async () => {
    render(
      <ProjectLayout
        activeTab="overview"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectOverviewPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )

    await screen.findByText("最新项目更新")
    expect(screen.queryByText("执行概览")).toBeNull()
  })

  it("does not request project task summary without task read", async () => {
    // 临时把 useMe 改成只有 project:read，没有 task:read
    vi.resetModules()
    const useMeMod = await import("@/features/workspace/session/useMe")
    vi.spyOn(useMeMod, "useMe").mockReturnValue({
      data: {
        actor_type: "user",
        actor: { id: "u", name: "u" },
        token: { type: "pat", scopes: ["project:read"] },
        effective_workspace: { slug: "local" },
        effective_role: "viewer",
      },
    } as never)

    render(
      <ProjectLayout
        activeTab="overview"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectOverviewPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    // 等待基础内容渲染
    await screen.findByText("最新项目更新")
    // 无 task:read 时不展示当前重点，也不应调用 task summary
    expect(screen.queryByText("当前重点")).toBeNull()
    expect(vi.mocked(getProjectTaskSummary)).not.toHaveBeenCalled()
  })
})
