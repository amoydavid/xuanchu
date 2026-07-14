import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
}))

vi.mock("@/features/workspace/project-workbench/api/task-series-api", () => ({
  createTaskSeries: vi.fn(),
  modifyTaskSeries: vi.fn(),
}))

vi.mock("@/features/workspace/project-workbench/api/users-api", () => ({
  getWorkspaceMembers: vi.fn(),
}))

import { modifyTaskSeries } from "@/features/workspace/project-workbench/api/task-series-api"
import { getWorkspaceMembers } from "@/features/workspace/project-workbench/api/users-api"
import { TaskSeriesDialog } from "./task-series-dialog"

const mockedModifyTaskSeries = vi.mocked(modifyTaskSeries)

describe("TaskSeriesDialog", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getWorkspaceMembers).mockResolvedValue([
      {
        id: "u1",
        name: "liuwei",
        display_name: "刘玮",
        role: "member",
        joined_at: 1,
        modified_at: 1,
      },
    ])
    vi.mocked(workspaceApiGet).mockResolvedValue({})
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

    await userEvent.click(screen.getByRole("button", { name: "循环结束日期" }))
    await userEvent.click(screen.getByRole("button", { name: "清除日期" }))
    await userEvent.click(screen.getByRole("combobox", { name: "优先级" }))
    await userEvent.click(screen.getByRole("option", { name: "无优先级" }))
    fireEvent.change(screen.getByLabelText("标签", { exact: true }), {
      target: { value: "" },
    })
    fireEvent.click(screen.getByRole("button", { name: "保存循环设置" }))

    await waitFor(() => {
      expect(mockedModifyTaskSeries).toHaveBeenCalledWith(
        "ws",
        "series-1",
        expect.objectContaining({
          clear: expect.arrayContaining(["until", "priority", "tags"]),
        })
      )
    })
  })

  it("编辑时可以清除说明、负责人和单个自定义字段", async () => {
    mockedModifyTaskSeries.mockResolvedValue({} as never)
    vi.mocked(workspaceApiGet).mockResolvedValue({
      "uda.channel.type": "string",
      "uda.channel.label": "渠道",
      "uda.region.type": "string",
      "uda.region.label": "区域",
    })
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
          description: "检查异常",
          status: "active",
          recurrence_rule: "daily",
          first_due: 1_893_542_399,
          assignees: [{ id: "u1", name: "liuwei", display_name: "刘玮" }],
          udas: { channel: "search", region: "cn" },
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

    await userEvent.clear(screen.getByLabelText("任务内容"))
    await userEvent.clear(await screen.findByLabelText("渠道"))
    await userEvent.click(screen.getByRole("button", { name: /u1/ }))
    await userEvent.click(screen.getByRole("button", { name: "清空负责人" }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    await userEvent.click(screen.getByRole("button", { name: "保存循环设置" }))

    await waitFor(() => {
      expect(mockedModifyTaskSeries).toHaveBeenCalledWith(
        "ws",
        "series-1",
        expect.objectContaining({
          clear: expect.arrayContaining([
            "description",
            "assignees",
            "uda.channel",
          ]),
          udas: { region: "cn" },
        })
      )
    })
  }, 10_000)

  it("uses a single shadcn dialog and shared date controls", () => {
    render(
      <TaskSeriesDialog
        mode="create"
        onClose={vi.fn()}
        open
        projectSlug="ops"
        workspaceSlug="ws"
      />
    )

    expect(screen.getAllByRole("dialog")).toHaveLength(1)
    expect(screen.getByTestId("task-series-dialog").className).toContain(
      "sm:max-w-2xl"
    )
    expect(screen.getByTestId("task-series-dialog").className).toContain(
      "max-h-[90vh]"
    )
    expect(screen.getByRole("combobox", { name: "循环规则" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "首次截止日期" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "循环结束日期" })).toBeTruthy()
  })

  it("explains how shared-field edits propagate to occurrences", () => {
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
          open_occurrence_count: 2,
          completed_count: 1,
          skipped_count: 1,
          overdue_count: 0,
          created_by: { id: "local", name: "local", display_name: "本地用户" },
          created_at: 1,
          modified_at: 1,
        }}
        workspaceSlug="ws"
      />
    )

    expect(
      screen.getByText(/未来实例使用新设置；负责人只影响未来实例，不会改动已生成实例/)
    ).toBeTruthy()
    expect(
      screen.getByText(/其它未完成且未单独修改的字段也会同步/)
    ).toBeTruthy()
  })

  it("prevents closing the editor while a save is pending", async () => {
    mockedModifyTaskSeries.mockImplementation(() => new Promise(() => {}))
    const onClose = vi.fn()
    render(
      <TaskSeriesDialog
        mode="edit"
        onClose={onClose}
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
          open_occurrence_count: 0,
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

    await userEvent.click(screen.getByRole("button", { name: "保存循环设置" }))
    await waitFor(() => expect(mockedModifyTaskSeries).toHaveBeenCalledOnce())
    fireEvent.keyDown(document, { key: "Escape" })

    expect(onClose).not.toHaveBeenCalled()
  })

  it("previews upcoming occurrences from the next recurrence when editing", () => {
    const unix = (date: Date) => Math.floor(date.getTime() / 1000)
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
          title: "每周巡检",
          status: "active",
          recurrence_rule: "weekly",
          first_due: unix(new Date(2026, 6, 1, 23, 59, 59)),
          next_recurrence_at: unix(new Date(2026, 6, 8, 23, 59, 59)),
          open_occurrence_count: 0,
          completed_count: 1,
          skipped_count: 0,
          overdue_count: 0,
          created_by: { id: "local", name: "local", display_name: "本地用户" },
          created_at: 1,
          modified_at: 1,
        }}
        workspaceSlug="ws"
      />
    )

    expect(
      screen.getByText("2026-07-08 · 2026-07-15 · 2026-07-22")
    ).toBeTruthy()
  })

  it("uses the server suggested slot when changing the recurrence rule", async () => {
    mockedModifyTaskSeries.mockResolvedValue({} as never)
    const unix = (date: Date) => Math.floor(date.getTime() / 1000)
    const nextRecurrence = unix(new Date(2026, 6, 8, 23, 59, 59))
    const suggestedEffectiveFrom = unix(new Date(2026, 6, 15, 23, 59, 59))
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
          title: "每周巡检",
          status: "active",
          recurrence_rule: "weekly",
          first_due: unix(new Date(2026, 6, 1, 23, 59, 59)),
          next_recurrence_at: nextRecurrence,
          suggested_rule_effective_from: suggestedEffectiveFrom,
          open_occurrence_count: 0,
          completed_count: 1,
          skipped_count: 0,
          overdue_count: 0,
          created_by: { id: "local", name: "local", display_name: "本地用户" },
          created_at: 1,
          modified_at: 1,
        }}
        workspaceSlug="ws"
      />
    )

    await userEvent.click(screen.getByRole("combobox", { name: "循环规则" }))
    await userEvent.click(screen.getByRole("option", { name: "每天" }))

    expect(
      screen.getByRole("button", { name: "新规则生效日期" }).textContent
    ).toContain("2026-07-15")
    expect(
      screen.getByText("2026-07-15 · 2026-07-16 · 2026-07-17")
    ).toBeTruthy()

    await userEvent.click(screen.getByRole("button", { name: "保存循环设置" }))
    await waitFor(() => {
      expect(mockedModifyTaskSeries).toHaveBeenCalledWith(
        "ws",
        "series-1",
        expect.objectContaining({
          recurrence_rule: "daily",
          effective_from: suggestedEffectiveFrom,
        })
      )
    })
  })
})
