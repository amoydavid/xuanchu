import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import type { ProjectWorkbenchTask } from "../api/project-api"
import { AssigneeWorkloadSummary } from "./assignee-workload-summary"

function task(
  overrides: Partial<ProjectWorkbenchTask> = {}
): ProjectWorkbenchTask {
  return {
    uuid: "task-1",
    title: "任务",
    status: "pending",
    priority: "M",
    due: 1_783_036_800,
    assignees: [{ id: "u1", name: "liuwei", display_name: "刘玮" }],
    ...overrides,
  }
}

describe("AssigneeWorkloadSummary", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("renders workload signals and filters by assignee", async () => {
    const onFilterAssignee = vi.fn()

    render(
      <AssigneeWorkloadSummary
        now={1_783_036_800}
        onFilterAssignee={onFilterAssignee}
        onFilterUnassigned={vi.fn()}
        tasks={[
          task({ uuid: "t1", priority: "H" }),
          task({ uuid: "t2", due: 1_782_950_400 }),
          task({ uuid: "t3", status: "completed" }),
        ]}
      />
    )

    expect(screen.getByText("负责人摘要")).toBeTruthy()
    expect(screen.getByText("刘玮")).toBeTruthy()
    expect(screen.getByText("未完成 2")).toBeTruthy()
    expect(screen.getByText("已逾期 1")).toBeTruthy()
    expect(screen.getByText("高优先级 1")).toBeTruthy()
    expect(screen.getByText("即将到期 1")).toBeTruthy()

    await userEvent.click(screen.getByRole("button", { name: /筛选 刘玮/ }))
    expect(onFilterAssignee).toHaveBeenCalledWith("u1")
  })

  it("shows and filters unassigned tasks", async () => {
    const onFilterUnassigned = vi.fn()

    render(
      <AssigneeWorkloadSummary
        now={1_783_036_800}
        onFilterAssignee={vi.fn()}
        onFilterUnassigned={onFilterUnassigned}
        tasks={[task({ assignees: [] })]}
      />
    )

    expect(screen.getByText("未分配")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: /筛选 未分配/ }))
    expect(onFilterUnassigned).toHaveBeenCalled()
  })
})
