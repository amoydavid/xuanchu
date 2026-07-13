import { describe, expect, it, vi, beforeEach } from "vitest"
import { fireEvent, render, screen, waitFor } from "@testing-library/react"

// Mock task-series-api。
vi.mock("@/features/workspace/project-workbench/api/task-series-api", () => ({
  listTaskSeries: vi.fn(),
  getTaskSeries: vi.fn(),
}))

import { TaskSeriesPanelShell } from "./task-series-panel-shell"
import {
  listTaskSeries,
  getTaskSeries,
} from "@/features/workspace/project-workbench/api/task-series-api"

const mockedListTaskSeries = vi.mocked(listTaskSeries)
const mockedGetTaskSeries = vi.mocked(getTaskSeries)

describe("TaskSeriesPanelShell", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("renders active series list with total count", async () => {
    mockedListTaskSeries.mockResolvedValue({
      items: [
        {
          id: "s1",
          workspace_id: "ws",
          project_id: "p1",
          title: "每日巡检",
          status: "active",
          recurrence_rule: "daily",
          first_due: 100,
          open_occurrence_count: 3,
          completed_count: 5,
          skipped_count: 1,
          overdue_count: 2,
          next_recurrence_at: 200,
          created_by: { id: "u1", name: "local", display_name: "local" },
          created_at: 1,
          modified_at: 1,
        },
      ],
      total: 1,
      limit: 1,
      offset: 0,
    })

    render(<TaskSeriesPanelShell workspaceSlug="ws" projectSlug="ops" />)

    await waitFor(() => {
      expect(screen.getByText("每日巡检")).toBeTruthy()
    })
    expect(screen.getByTestId("task-series-row")).toBeTruthy()
    expect(screen.queryByTestId("task-series-detail")).toBeNull()
  })

  it("支持按负责人和排序筛选并翻页", async () => {
    mockedListTaskSeries.mockResolvedValue({
      items: [],
      total: 25,
      limit: 20,
      offset: 0,
    })
    render(<TaskSeriesPanelShell workspaceSlug="ws" projectSlug="ops" />)

    await waitFor(() => expect(screen.getByRole("button", { name: "下一页" })).toBeTruthy())
    fireEvent.change(screen.getByLabelText("循环任务负责人筛选"), {
      target: { value: "alice" },
    })
    fireEvent.change(screen.getByLabelText("循环任务排序"), {
      target: { value: "title" },
    })
    fireEvent.click(screen.getByRole("button", { name: "下一页" }))

    await waitFor(() => {
      expect(mockedListTaskSeries).toHaveBeenLastCalledWith(
        "ws",
        expect.objectContaining({
          assignee: "alice",
          limit: 20,
          offset: 20,
          sort: "title",
        })
      )
    })
  })

  it("renders empty state when no series", async () => {
    mockedListTaskSeries.mockResolvedValue({ items: [], total: 0, limit: 0, offset: 0 })

    render(<TaskSeriesPanelShell workspaceSlug="ws" projectSlug="ops" />)

    await waitFor(() => {
      expect(screen.getByText(/还没有循环任务/)).toBeTruthy()
    })
  })

  it("renders detail when seriesRef provided", async () => {
    mockedListTaskSeries.mockResolvedValue({ items: [], total: 0, limit: 0, offset: 0 })
    mockedGetTaskSeries.mockResolvedValue({
      id: "s1",
      workspace_id: "ws",
      project_id: "p1",
      title: "每日巡检",
      status: "active",
      recurrence_rule: "daily",
      first_due: 100,
      open_occurrence_count: 3,
      completed_count: 5,
      skipped_count: 1,
      overdue_count: 2,
      created_by: { id: "u1", name: "local", display_name: "local" },
      created_at: 1,
      modified_at: 1,
    })

    render(<TaskSeriesPanelShell workspaceSlug="ws" projectSlug="ops" seriesRef="s1" />)

    await waitFor(() => {
      expect(screen.getByTestId("task-series-detail")).toBeTruthy()
    })
  })

  it("renders error state on load failure", async () => {
    mockedListTaskSeries.mockRejectedValue(new Error("网络错误"))

    render(<TaskSeriesPanelShell workspaceSlug="ws" projectSlug="ops" />)

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeTruthy()
    })
  })

  it("panel does not render a tablist (not a new tab)", async () => {
    mockedListTaskSeries.mockResolvedValue({ items: [], total: 0, limit: 0, offset: 0 })
    const { container } = render(<TaskSeriesPanelShell workspaceSlug="ws" projectSlug="ops" />)
    await waitFor(() => {
      expect(container.querySelector('[data-testid="task-series-panel"]')).toBeTruthy()
    })
    expect(container.querySelector('[role="tablist"]')).toBeNull()
  })
})
