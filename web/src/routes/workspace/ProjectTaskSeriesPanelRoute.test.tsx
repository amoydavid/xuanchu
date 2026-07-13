import { describe, expect, it, vi } from "vitest"
import { render } from "@testing-library/react"

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
})
