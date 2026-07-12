import { describe, expect, it, vi } from "vitest"
import { fireEvent, render, screen } from "@testing-library/react"

import { TaskSeriesList } from "./task-series-list"
import { TaskSeriesStopDialog } from "./task-series-stop-dialog"
import type { TaskSeriesView } from "@/features/workspace/project-workbench/api/task-series-api"

function makeSeries(overrides: Partial<TaskSeriesView> = {}): TaskSeriesView {
  return {
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
    ...overrides,
  }
}

describe("TaskSeriesList", () => {
  it("renders series rows with counts", () => {
    render(
      <TaskSeriesList
        items={[makeSeries()]}
        total={1}
        loading={false}
        error={null}
        onSelect={vi.fn()}
        statusFilter="active"
        onStatusFilterChange={vi.fn()}
        query=""
        onQueryChange={vi.fn()}
        canManage
      />,
    )
    expect(screen.getByText("每日巡检")).toBeTruthy()
    expect(screen.getByTestId("task-series-row")).toBeTruthy()
    expect(screen.getByText(/未完成 3/)).toBeTruthy()
  })

  it("renders empty state", () => {
    render(
      <TaskSeriesList
        items={[]}
        total={0}
        loading={false}
        error={null}
        onSelect={vi.fn()}
        statusFilter="active"
        onStatusFilterChange={vi.fn()}
        query=""
        onQueryChange={vi.fn()}
        canManage
      />,
    )
    expect(screen.getByText(/还没有循环任务/)).toBeTruthy()
  })

  it("renders error", () => {
    render(
      <TaskSeriesList
        items={[]}
        total={0}
        loading={false}
        error="加载失败"
        onSelect={vi.fn()}
        statusFilter="active"
        onStatusFilterChange={vi.fn()}
        query=""
        onQueryChange={vi.fn()}
        canManage
      />,
    )
    expect(screen.getByRole("alert")).toBeTruthy()
  })

  it("calls onSelect when row clicked", () => {
    const onSelect = vi.fn()
    render(
      <TaskSeriesList
        items={[makeSeries()]}
        total={1}
        loading={false}
        error={null}
        onSelect={onSelect}
        statusFilter="active"
        onStatusFilterChange={vi.fn()}
        query=""
        onQueryChange={vi.fn()}
        canManage
      />,
    )
    fireEvent.click(screen.getByLabelText("查看循环任务 每日巡检"))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "s1" }))
  })
})

describe("TaskSeriesStopDialog", () => {
  it("renders nothing when closed", () => {
    const { container } = render(
      <TaskSeriesStopDialog open={false} series={null} openCount={0} onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(container.querySelector('[data-testid="task-series-stop-dialog"]')).toBeNull()
  })

  it("shows delete-open checkbox with count", () => {
    render(
      <TaskSeriesStopDialog open={true} series={{ title: "每日巡检" }} openCount={5} onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(screen.getByText(/同时跳过当前 5 条未完成实例/)).toBeTruthy()
  })

  it("disables delete-open when over 1000", () => {
    render(
      <TaskSeriesStopDialog open={true} series={{ title: "x" }} openCount={1500} onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(screen.getByTestId("delete-open-checkbox")).toHaveProperty("disabled", true)
    expect(screen.getByText(/超过 1000 条/)).toBeTruthy()
  })

  it("calls onConfirm with deleteOpen flag", () => {
    const onConfirm = vi.fn()
    render(
      <TaskSeriesStopDialog open={true} series={{ title: "x" }} openCount={3} onConfirm={onConfirm} onCancel={vi.fn()} />,
    )
    fireEvent.click(screen.getByTestId("delete-open-checkbox"))
    fireEvent.click(screen.getByTestId("confirm-stop-btn"))
    expect(onConfirm).toHaveBeenCalledWith(true)
  })
})
