import { describe, expect, it, vi } from "vitest"
import { render, waitFor } from "@testing-library/react"
import { useParams } from "@tanstack/react-router"

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

import { ProjectTaskSeriesPanelRoute } from "./ProjectTaskSeriesPanelRoute"

describe("ProjectTaskSeriesPanelRoute", () => {
  it("renders panel shell with seriesRef from params", async () => {
    const { container } = render(<ProjectTaskSeriesPanelRoute />)
    await waitFor(() => {
      expect(container.querySelector('[data-testid="task-series-panel"]')).toBeTruthy()
    })
    // seriesRef 存在时应进入 detail 模式。
    expect(container.querySelector('[data-testid="task-series-detail"]')).toBeTruthy()
  })

  it("uses mocked useParams", () => {
    render(<ProjectTaskSeriesPanelRoute />)
    expect(vi.mocked(useParams)).toHaveBeenCalled()
  })
})
