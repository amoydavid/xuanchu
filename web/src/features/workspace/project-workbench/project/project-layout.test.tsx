import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useEffect, type ReactNode } from "react"
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
import { ProjectLayout, useProjectLayout } from "./project-layout"

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

vi.mock("@/features/workspace/config/config-definition-api", async () => {
  const actual = await vi.importActual<
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
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    })
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
    const overviewLink = screen.getByRole("link", {
      name: "概览",
    }) as HTMLAnchorElement
    expect(overviewLink.getAttribute("href")).toBe(
      "/workspaces/local/projects/ops"
    )
    const tasksLink = screen.getByRole("link", {
      name: "任务",
    }) as HTMLAnchorElement
    expect(tasksLink.getAttribute("href")).toBe(
      "/workspaces/local/projects/ops/tasks"
    )
    const activityLink = screen.getByRole("link", {
      name: "活动",
    }) as HTMLAnchorElement
    expect(activityLink.getAttribute("href")).toBe(
      "/workspaces/local/projects/ops/activity"
    )
  })

  it("shows the shared project header with status and copy action", async () => {
    render(
      <ProjectLayout activeTab="tasks" projectSlug="ops" workspaceSlug="local">
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

  it("renders project automation tab link", async () => {
    render(
      <ProjectLayout
        activeTab="automations"
        projectSlug="ops"
        workspaceSlug="local"
      >
        <div />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )
    const automationLink = (await screen.findByRole("link", {
      name: "自动化",
    })) as HTMLAnchorElement
    expect(automationLink.getAttribute("href")).toBe(
      "/workspaces/local/projects/ops/automations"
    )
    expect(automationLink.getAttribute("aria-current")).toBe("page")
  })

  it("renders a custom context panel as a Sheet on narrow screens", async () => {
    const onClose = vi.fn()
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: () => ({
        matches: true,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    })

    render(
      <ProjectLayout activeTab="tasks" projectSlug="ops" workspaceSlug="local">
        <ContextPanelRegistrar onClose={onClose} />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )

    const dialog = await screen.findByRole("dialog", { name: "循环任务" })
    expect(dialog.className).toContain("w-screen")
    expect(dialog.className).toContain("h-dvh")
    expect(dialog.getAttribute("aria-describedby")).toBeTruthy()
    expect(
      dialog.querySelector('[data-slot="sheet-description"]')?.textContent
    ).toBe(i18n.t("taskSeries.description"))
    await userEvent.click(screen.getByRole("button", { name: "关闭循环任务" }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it("restores a collapsed rail after the custom panel content changes", async () => {
    const user = userEvent.setup()
    render(
      <ProjectLayout activeTab="tasks" projectSlug="ops" workspaceSlug="local">
        <ContextPanelLifecycle />
      </ProjectLayout>,
      { wrapper: Wrapper }
    )

    await screen.findByText("项目信息")
    await user.click(screen.getByRole("button", { name: "收起右栏" }))
    await user.click(screen.getByRole("button", { name: "打开循环面板" }))
    expect(screen.getByText("循环面板 1")).toBeTruthy()

    await user.click(screen.getByRole("button", { name: "更新循环面板" }))
    expect(screen.getByText("循环面板 2")).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "关闭测试面板" }))

    expect(screen.queryByText("项目信息")).toBeNull()
    expect(screen.getByRole("button", { name: "展开右栏" })).toBeTruthy()
  })
})

function ContextPanelRegistrar({ onClose }: { onClose: () => void }) {
  const { setContextPanel } = useProjectLayout()
  useEffect(() => {
    setContextPanel({ node: <div>循环面板内容</div>, onClose })
    return () => setContextPanel(null)
    // setContextPanel 由 layout 提供；只在测试组件挂载/卸载时注册。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [onClose])
  return <div>任务主体</div>
}

function ContextPanelLifecycle() {
  const { setContextPanel } = useProjectLayout()

  const openPanel = (revision: number) => {
    setContextPanel({
      node: <div>循环面板 {revision}</div>,
      onClose: () => setContextPanel(null),
    })
  }

  return (
    <div>
      <button onClick={() => openPanel(1)} type="button">
        打开循环面板
      </button>
      <button onClick={() => openPanel(2)} type="button">
        更新循环面板
      </button>
      <button onClick={() => setContextPanel(null)} type="button">
        关闭测试面板
      </button>
    </div>
  )
}
