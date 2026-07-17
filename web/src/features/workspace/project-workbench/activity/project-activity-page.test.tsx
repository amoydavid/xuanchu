import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import {
  addProjectAnnotation,
  getProject,
  getProjectTaskSummary,
  getProjectTasks,
  getProjectTimeline,
  type ProjectWorkbenchProject,
} from "../api/project-api"
import { getWorkspaceMembers } from "../api/users-api"
import { ProjectLayout } from "../project/project-layout"
import { ProjectActivityPage } from "./project-activity-page"

// 通过共享 mock state，便于单个测试覆盖角色/scope。
const meState = {
  data: {
    actor_type: "user" as const,
    actor: { id: "u1", name: "owner" },
    token: {
      scopes: ["project:write", "task:read", "task:write", "audit:read"],
      type: "pat",
    },
    effective_workspace: { slug: "local" },
    effective_role: "owner",
  },
}

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => meState,
}))

vi.mock("../api/project-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/project-api")>(
      "../api/project-api"
    )
  return {
    ...actual,
    getProject: vi.fn(),
    getProjectTasks: vi.fn(),
    getProjectTimeline: vi.fn(),
    getProjectTaskSummary: vi.fn(),
    addProjectAnnotation: vi.fn(),
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

let activeQueryClient = makeQueryClient()

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={activeQueryClient}>
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

describe("ProjectActivityPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    activeQueryClient = makeQueryClient()
    await i18n.changeLanguage("zh-CN")
    // 重置 meState 为 owner + audit:read
    meState.data = {
      actor_type: "user",
      actor: { id: "u1", name: "owner" },
      token: {
        scopes: ["project:write", "task:read", "task:write", "audit:read"],
        type: "pat",
      },
      effective_workspace: { slug: "local" },
      effective_role: "owner",
    }
    vi.mocked(getProject).mockResolvedValue(project())
    vi.mocked(getProjectTasks).mockResolvedValue([])
    vi.mocked(getProjectTimeline).mockResolvedValue([
      {
        source_type: "project",
        source_id: "anno-1",
        source_label: "ops",
        entry: 1800000100,
        content: "完成 token mcp-config 验证",
        created_by: {
          id: "u1",
          name: "王五",
          user: { id: "u1", name: "王五" },
        },
      },
      {
        source_type: "task",
        source_id: "task-uuid-1",
        source_label: "ops-12",
        entry: 1800000200,
        content: "线上回调地址已确认",
        created_by: {
          id: "u2",
          name: "李四",
          user: { id: "u2", name: "李四" },
        },
      },
    ])
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
    vi.mocked(getWorkspaceMembers).mockResolvedValue([])
    vi.mocked(addProjectAnnotation).mockResolvedValue({
      id: "annotation-2",
      project_id: "project-1",
      entry: 1800000300,
      content: "上线窗口已确认",
      created_by: { id: "u1", name: "owner" },
      created_at: 1800000300,
    })
  })

  it("uses timeline as the only list source for project updates", async () => {
    render(
      <ProjectLayout
        activeTab="activity"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectActivityPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    // project annotation 从 timeline 渲染（活动页 + 右栏最近活动都会出现）
    expect(
      (await screen.findAllByText("完成 token mcp-config 验证")).length
    ).toBeGreaterThan(0)
    // addProjectAnnotation 仅在用户点击发布时调用，首屏不应被调用
    expect(vi.mocked(addProjectAnnotation)).not.toHaveBeenCalled()
  })

  it("does not request audit and hides audit filter without audit read", async () => {
    // 覆盖 meState：member 无 audit:read
    meState.data = {
      actor_type: "user",
      actor: { id: "u", name: "member" },
      token: { type: "pat", scopes: ["project:write", "task:read"] },
      effective_workspace: { slug: "local" },
      effective_role: "member",
    }

    render(
      <ProjectLayout
        activeTab="activity"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectActivityPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    // 等待时间线内容渲染
    await screen.findAllByText("完成 token mcp-config 验证")
    // 无 audit:read 时不展示审计筛选按钮
    expect(screen.queryByRole("button", { name: "审计" })).toBeNull()
  })

  it("shows audit filter for owner with audit:read", async () => {
    render(
      <ProjectLayout
        activeTab="activity"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectActivityPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    await screen.findAllByText("完成 token mcp-config 验证")
    expect(screen.getByRole("button", { name: "审计" })).toBeTruthy()
  })

  it("shows publish form for owner of active project", async () => {
    render(
      <ProjectLayout
        activeTab="activity"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectActivityPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    await screen.findAllByText("完成 token mcp-config 验证")
    expect(screen.getByRole("button", { name: "发布更新" })).toBeTruthy()
  })

  it("invalidates the home summary after publishing a project update", async () => {
    const invalidateSpy = vi.spyOn(activeQueryClient, "invalidateQueries")
    render(
      <ProjectLayout
        activeTab="activity"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectActivityPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    await screen.findAllByText("完成 token mcp-config 验证")

    await userEvent.type(screen.getByLabelText("发布更新"), "上线窗口已确认")
    await userEvent.click(screen.getByRole("button", { name: "发布更新" }))

    await waitFor(() => {
      expect(addProjectAnnotation).toHaveBeenCalledWith(
        "local",
        "ops",
        "上线窗口已确认"
      )
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["home", "local"],
      })
    })
  })

  it("hides publish form and shows readonly hint when project is closed", async () => {
    vi.mocked(getProject).mockResolvedValue(project({ status: "archived" }))
    render(
      <ProjectLayout
        activeTab="activity"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <ProjectActivityPage projectSlug="ops" workspaceSlug="local" />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    // closed banner 出现
    await screen.findAllByText(/项目已关闭/)
    expect(screen.queryByRole("button", { name: "发布更新" })).toBeNull()
  })
})
