import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import type { AuditRow } from "@/features/workspace/audit/audit-api"
import { renderWithRouter } from "@/test/router-wrapper"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

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

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
}))

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
    // audit 默认返回空数组；个别测试覆盖具体审计行。
    vi.mocked(workspaceApiGet).mockResolvedValue([] as unknown as never)
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

  it("renders task lifecycle events with action and clickable title", async () => {
    vi.mocked(getProjectTimeline).mockResolvedValue([
      {
        source_type: "task",
        source_id: "task-uuid-done",
        source_label: "对接支付回调",
        entry: 1800000500,
        action: "completed",
        kind: "lifecycle",
        created_by: {
          id: "u2",
          name: "李四",
          user: { id: "u2", name: "李四" },
        },
      },
      {
        source_type: "task",
        source_id: "task-uuid-modify",
        source_label: "修复登录页",
        entry: 1800000400,
        action: "fields_changed",
        kind: "change",
        changes: [
          {
            kind: "scalar" as const,
            field: "priority",
            label_key: "projectWorkbench.taskHistory.field.priority",
            previous: { raw: null, text: "—" },
            current: { raw: "H", text: "H" },
          },
        ],
        created_by: {
          id: "u3",
          name: "王五",
          user: { id: "u3", name: "王五" },
        },
      },
    ])

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

    // 完成事件渲染「李四 完成了任务《对接支付回调》」
    const completedText = await screen.findAllByText(/完成了任务/)
    expect(completedText.length).toBeGreaterThan(0)
    // 任务标题作为可点击链接渲染（活动页 + 右栏可能各出现一次）
    const doneLinks = await screen.findAllByText("对接支付回调")
    expect(doneLinks.length).toBeGreaterThan(0)

    // 字段修改事件渲染「王五 修改了任务 · 修复登录页」
    const modifiedText = await screen.findAllByText(/修改了任务/)
    expect(modifiedText.length).toBeGreaterThan(0)
    const modifyLinks = await screen.findAllByText("修复登录页")
    expect(modifyLinks.length).toBeGreaterThan(0)
  })

  it("does not duplicate task lifecycle or annotation events between timeline and audit", async () => {
    // timeline 提供：任务完成事件 + 项目更新注释。
    vi.mocked(getProjectTimeline).mockResolvedValue([
      {
        source_type: "task",
        source_id: "task-uuid-done",
        source_label: "每日广告汇报",
        entry: 1800000600,
        action: "completed",
        kind: "lifecycle",
        created_by: {
          id: "u2",
          name: "刘玮",
          user: { id: "u2", name: "刘玮" },
        },
      },
      {
        source_type: "project",
        source_id: "anno-1",
        source_label: "ops",
        entry: 1800000500,
        content: "这是项目更新记录",
        created_by: {
          id: "u2",
          name: "刘玮",
          user: { id: "u2", name: "刘玮" },
        },
      },
    ])
    // audit 返回同源的审计行：
    //   - task.done（target_type=task，已被 timeline 任务活动覆盖）
    //   - project.annotate（注释类，已被 timeline 项目注释覆盖）
    // 这两条都不应再以「审计 · ...」的形式重复出现。
    vi.mocked(workspaceApiGet).mockImplementation(async (path: string) => {
      if (path.includes("/api/v1/audit")) {
        return [
          {
            id: 9001,
            action: "task.done",
            target_type: "task",
            target_id: "task-uuid-done",
            actor: { id: "u2", name: "刘玮" },
            actor_type: "user",
            created_at: 1800000600,
          },
          {
            id: 9002,
            action: "project.annotate",
            target_type: "project",
            target_id: "project-1",
            actor: { id: "u2", name: "刘玮" },
            actor_type: "user",
            created_at: 1800000500,
          },
        ] as AuditRow[]
      }
      return [] as unknown as never
    })

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

    // 等待 timeline 渲染出任务完成事件与项目注释。
    await screen.findAllByText(/完成了任务/)
    await screen.findAllByText("这是项目更新记录")
    // 同源的动作不应再以审计行的形式重复出现。
    expect(screen.queryByText("task.done")).toBeNull()
    expect(screen.queryByText("project.annotate")).toBeNull()
  })
})
