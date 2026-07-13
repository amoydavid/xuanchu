import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

vi.mock("@/features/workspace/project-workbench/api/task-series-api", () => ({
  createTaskSeries: vi.fn(),
  modifyTaskSeries: vi.fn(),
}))

import { modifyTaskSeries } from "@/features/workspace/project-workbench/api/task-series-api"
import { TaskSeriesDialog } from "./task-series-dialog"

const mockedModifyTaskSeries = vi.mocked(modifyTaskSeries)

describe("TaskSeriesDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("编辑时可以清除循环结束、优先级和标签", async () => {
    mockedModifyTaskSeries.mockResolvedValue({} as never)
    render(
      <TaskSeriesDialog
        mode="edit"
        onClose={vi.fn()}
        open
        projectSlug="ops"
        series={{
          id: "series-1",
          workspace_id: "ws",
          project_id: "project-1",
          title: "每日巡检",
          status: "active",
          recurrence_rule: "daily",
          first_due: 1_893_542_399,
          until: 1_896_220_799,
          priority: "H",
          tags: ["ops"],
          open_occurrence_count: 1,
          completed_count: 0,
          skipped_count: 0,
          overdue_count: 0,
          created_by: { id: "local", name: "local", display_name: "本地用户" },
          created_at: 1,
          modified_at: 1,
        }}
        workspaceSlug="ws"
      />
    )

    fireEvent.change(screen.getByLabelText("循环结束日期"), { target: { value: "" } })
    fireEvent.change(screen.getByLabelText("优先级"), { target: { value: "" } })
    fireEvent.change(screen.getByLabelText("标签"), { target: { value: "" } })
    fireEvent.click(screen.getByRole("button", { name: "保存循环设置" }))

    await waitFor(() => {
      expect(mockedModifyTaskSeries).toHaveBeenCalledWith(
        "ws",
        "series-1",
        expect.objectContaining({ clear: expect.arrayContaining(["until", "priority", "tags"]) })
      )
    })
  })
})
