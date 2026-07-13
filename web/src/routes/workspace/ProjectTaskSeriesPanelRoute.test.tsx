import { beforeEach, describe, expect, it, vi } from "vitest"
import { render } from "@testing-library/react"

const navigateMock = vi.fn()
let routeSearch: Record<string, string> = {
  panel_return_scope: "project",
  panel_return_task: "ops-7",
}

// Mock useParams 返回 seriesRef。
vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>("@tanstack/react-router")
  return {
    ...actual,
    useParams: vi.fn(({ strict }) => {
      if (strict === false) {
        return { workspaceSlug: "ws", projectSlug: "ops", seriesRef: "s1" }
      }
      return {}
    }),
    useNavigate: () => navigateMock,
    useSearch: () => routeSearch,
  }
})

vi.mock("@/features/workspace/project-workbench/api/task-series-api", () => ({
  listTaskSeries: vi.fn().mockResolvedValue({ items: [], total: 0, limit: 0, offset: 0 }),
  getTaskSeries: vi.fn().mockResolvedValue({
    id: "s1", workspace_id: "ws", project_id: "p1", title: "每日巡检",
    status: "active", recurrence_rule: "daily", first_due: 100,
    open_occurrence_count: 0, completed_count: 0, skipped_count: 0, overdue_count: 0,
    created_by: { id: "u1", name: "local", display_name: "local" },
    created_at: 1, modified_at: 1,
  }),
}))

// Mock useProjectLayout 提供 setContextPanel。
const mockSetContextPanel = vi.fn()
vi.mock("@/features/workspace/project-workbench/project/project-layout", () => ({
  useProjectLayout: () => ({
    setContextPanel: mockSetContextPanel,
    workspaceSlug: "ws",
    projectSlug: "ops",
  }),
}))

import { ProjectTaskSeriesPanelRoute } from "./ProjectTaskSeriesPanelRoute"

describe("ProjectTaskSeriesPanelRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeSearch = {
      panel_return_scope: "project",
      panel_return_task: "ops-7",
    }
  })

  it("registers panel via setContextPanel", () => {
    render(<ProjectTaskSeriesPanelRoute />)
    // 应调用 setContextPanel 注册面板（非 null）。
    expect(mockSetContextPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        node: expect.anything(),
        onClose: expect.any(Function),
      }),
    )
  })

  it("component renders null (panel goes to context slot)", () => {
    const { container } = render(<ProjectTaskSeriesPanelRoute />)
    // 组件本身返回 null，面板通过 setContextPanel 渲染到右栏。
    expect(container.firstChild).toBeNull()
  })

  it("closes back to the source task detail when opened from an occurrence", () => {
    render(<ProjectTaskSeriesPanelRoute />)

    const registration = mockSetContextPanel.mock.calls
      .map(([value]) => value)
      .find((value) => value?.onClose)
    registration.onClose()

    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef",
      params: {
        workspaceSlug: "ws",
        projectSlug: "ops",
        taskRef: "ops-7",
      },
    })
  })

  it("preserves the My Tasks source when closing back to an occurrence", () => {
    routeSearch = {
      panel_return_scope: "project",
      panel_return_search: "tab=completed&priority=H&q=review&sort=due",
      panel_return_source: "my-tasks",
      panel_return_task: "ops-7",
    }
    render(<ProjectTaskSeriesPanelRoute />)

    const registration = mockSetContextPanel.mock.calls
      .map(([value]) => value)
      .find((value) => value?.onClose)
    registration.onClose()

    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef",
      params: {
        workspaceSlug: "ws",
        projectSlug: "ops",
        taskRef: "ops-7",
      },
      search: {
        from: "my-tasks",
        my_tasks_search: "tab=completed&priority=H&q=review&sort=due",
      },
    })
  })
})
