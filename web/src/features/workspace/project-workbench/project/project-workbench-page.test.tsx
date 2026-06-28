import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import {
  getProject,
  getProjectTasks,
  getProjectTimeline,
  type ProjectWorkbenchProject,
  type ProjectWorkbenchTask,
} from "../api/project-api"
import { ProjectWorkbenchPage } from "./project-workbench-page"

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({
    data: {
      effective_role: "owner",
      token: { scopes: ["project:write", "task:write"], type: "pat" },
    },
  }),
}))

vi.mock("../api/project-api", async () => {
  const actual = await vi.importActual<typeof import("../api/project-api")>(
    "../api/project-api"
  )
  return {
    ...actual,
    getProject: vi.fn(),
    getProjectTasks: vi.fn(),
    getProjectTimeline: vi.fn(),
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
    slug: "adsops",
    name: "广告投放自动化",
    status: "active",
    task_count: 1,
    pending_count: 1,
    completed_count: 0,
    created_at: 1,
    modified_at: 1,
    ...overrides,
  }
}

function task(overrides: Partial<ProjectWorkbenchTask> = {}): ProjectWorkbenchTask {
  return {
    uuid: "task-uuid-1",
    task_slug: "ads-1",
    title: "写投放日报",
    status: "pending",
    project: "adsops",
    priority: "M",
    due: 1783036800,
    ...overrides,
  }
}

describe("ProjectWorkbenchPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getProject).mockResolvedValue(project())
    vi.mocked(getProjectTasks).mockResolvedValue([task()])
    vi.mocked(getProjectTimeline).mockResolvedValue([])
  })

  it("keeps project management available but disables task editing when project is closed", async () => {
    vi.mocked(getProject).mockResolvedValue(project({ status: "archived" }))

    render(
      <ProjectWorkbenchPage projectSlug="adsops" workspaceSlug="acme" />,
      { wrapper: Wrapper }
    )

    await screen.findByText("项目已关闭")
    expect(
      (screen.getByRole("button", { name: "项目名称" }) as HTMLButtonElement)
        .disabled
    ).toBe(false)
    expect(
      (screen.getByRole("button", { name: "编辑任务标题 ads-1" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("combobox", { name: "任务优先级 ads-1" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", {
        name: "任务截止日期 ads-1",
      }) as HTMLButtonElement).disabled
    ).toBe(true)
    expect(within(screen.getByRole("table")).queryByRole("button", {
      name: "开始 ads-1",
    })).toBeNull()
    expect(within(screen.getByRole("table")).queryByRole("button", {
      name: "完成 ads-1",
    })).toBeNull()
  })

  it("localizes project workbench summary labels", async () => {
    await i18n.changeLanguage("en-US")
    vi.mocked(getProjectTimeline).mockResolvedValue([
      {
        id: "evt-1",
        action: "task.updated",
        created_at: 1782600000,
        actor: { id: "u1", name: "Alice", external_ids: [] },
      },
    ])

    render(
      <ProjectWorkbenchPage projectSlug="adsops" workspaceSlug="acme" />,
      { wrapper: Wrapper }
    )

    await screen.findByText("广告投放自动化")
    expect(screen.getAllByText("Pending").length).toBeGreaterThan(0)
    expect(screen.getByText("In progress")).toBeTruthy()
    expect(screen.getAllByText("Completed").length).toBeGreaterThan(0)
    expect(screen.getByText("Overdue")).toBeTruthy()
    expect(screen.getByText("High priority")).toBeTruthy()
    expect(screen.getByText("Assignee summary")).toBeTruthy()
    expect(screen.getByText(/open/)).toBeTruthy()
    expect(screen.getByText(/overdue/)).toBeTruthy()
    expect(screen.getByText("Recent activity")).toBeTruthy()
  })
})
