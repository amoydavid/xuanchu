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
  getProjectTasks,
  getProjectTimeline,
  type ProjectWorkbenchProject,
} from "../api/project-api"
import { getWorkspaceMembers } from "../api/users-api"
import { ProjectWorkbenchPage } from "./project-workbench-page"

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
    await vi.importActual<typeof import("../api/project-api")>(
      "../api/project-api"
    )
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
    task_count: 1,
    pending_count: 1,
    completed_count: 0,
    created_at: 1,
    modified_at: 1,
    ...overrides,
  }
}

describe("ProjectWorkbenchPage (Overview)", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getProject).mockResolvedValue(project())
    vi.mocked(getProjectTasks).mockResolvedValue([])
    vi.mocked(getProjectTimeline).mockResolvedValue([])
    vi.mocked(getProjectTaskSummary).mockResolvedValue({
      overdue_count: 1,
      overdue_refs: [
        { uuid: "u1", task_slug: "ops-7", title: "回调地址确认", label: "ops-7" },
      ],
      high_priority_open_count: 1,
      high_priority_open_refs: [],
      wait_ready_count: 0,
      wait_ready_refs: [],
      unassigned_open_count: 0,
      unassigned_open_refs: [],
      workload: [],
    })
    vi.mocked(getWorkspaceMembers).mockResolvedValue([])
    vi.mocked(listProjectEffectiveConfig).mockResolvedValue([
      {
        key: "model.provider",
        value: "openai",
        source: "project",
        definition: {
          key: "model.provider",
          value_type: "string",
          allowed_scopes: ["project", "workspace"],
          label: "模型供应商",
          description: "",
          enum_values: [],
          default_value: null,
          required: false,
          secret: false,
          show_on_console_home: true,
          created_at: 0,
          modified_at: 0,
        },
        show_on_console_home: true,
        missing_required: false,
      },
      {
        key: "callback.secret",
        value: "••••••",
        source: "project",
        definition: {
          key: "callback.secret",
          value_type: "string",
          allowed_scopes: ["project"],
          label: "回调密钥",
          description: "",
          enum_values: [],
          default_value: null,
          required: true,
          secret: true,
          show_on_console_home: true,
          created_at: 0,
          modified_at: 0,
        },
        show_on_console_home: true,
        missing_required: false,
      },
    ])
  })

  it("renders overview blocks and whole-project focus from task summary", async () => {
    render(<ProjectWorkbenchPage projectSlug="ops" workspaceSlug="local" />, {
      wrapper: Wrapper,
    })

    expect(await screen.findByText("当前重点")).toBeTruthy()
    // 逾期/高优/等待/未分配 在 Overview 当前重点和右栏都会出现，用 getAllByText
    expect(screen.getAllByText("逾期").length).toBeGreaterThan(0)
    expect(screen.getAllByText("高优未完成").length).toBeGreaterThan(0)
    expect(screen.getAllByText("等待已到期").length).toBeGreaterThan(0)
    expect(screen.getAllByText("未分配任务").length).toBeGreaterThan(0)
    // 当前重点的任务引用来自 ProjectSummary，不是任务列表
    expect(screen.getByText("ops-7")).toBeTruthy()
    // 不应出现「当前加载范围内」这类用列表冒充全量的文案
    expect(screen.queryByText("当前加载范围内")).toBeNull()
  })

  it("shows project facts as label/value and masks secrets", async () => {
    render(<ProjectWorkbenchPage projectSlug="ops" workspaceSlug="local" />, {
      wrapper: Wrapper,
    })
    // 项目附属信息在 Overview 和右栏都会出现
    expect((await screen.findAllByText("项目附属信息")).length).toBeGreaterThan(0)
    // 模型供应商 label 在 Overview 和右栏都会出现
    expect(screen.getAllByText("模型供应商").length).toBeGreaterThan(0)
    expect(screen.getAllByText("openai").length).toBeGreaterThan(0)
    // secret 只展示「已设置」，不展示原始值（Overview + 右栏各一处）
    expect(screen.getAllByText("回调密钥").length).toBeGreaterThan(0)
    expect(screen.getAllByText("已设置").length).toBeGreaterThan(0)
    expect(screen.queryByText("••••••")).toBeNull()
    // 不使用「运行配置」标题
    expect(screen.queryByText("运行配置")).toBeNull()
  })

  it("shows latest project update or empty hint", async () => {
    render(<ProjectWorkbenchPage projectSlug="ops" workspaceSlug="local" />, {
      wrapper: Wrapper,
    })
    expect(await screen.findByText("最新项目更新")).toBeTruthy()
    // 无 recent_annotations 时显示「暂无项目更新」
    expect(screen.getByText("暂无项目更新")).toBeTruthy()
  })

  it("does not render the legacy task toolbar or import action on overview", async () => {
    render(<ProjectWorkbenchPage projectSlug="ops" workspaceSlug="local" />, {
      wrapper: Wrapper,
    })
    await screen.findByText("当前重点")
    // 概览页不应出现任务工具栏的导入/新建主动作（这些属于 Tasks 页）
    expect(screen.queryByRole("button", { name: "导入任务" })).toBeNull()
    expect(screen.queryByRole("button", { name: "新建任务" })).toBeNull()
  })
})
