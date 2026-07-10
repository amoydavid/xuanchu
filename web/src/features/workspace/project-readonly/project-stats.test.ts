import { describe, expect, it } from "vitest"

import {
  buildProjectStats,
  summarizeAssignees,
  type ProjectTask,
} from "./project-stats"

const now = 1_800_000_000

describe("project readonly stats", () => {
  it("counts task statuses, overdue tasks, and high priority tasks", () => {
    const tasks: ProjectTask[] = [
      { uuid: "1", title: "pending", status: "pending", due: now - 10 },
      { uuid: "2", title: "active", status: "active", priority: "H" },
      {
        uuid: "3",
        title: "completed",
        status: "completed",
        due: now - 20,
        priority: "H",
      },
      { uuid: "4", title: "deleted", status: "deleted", due: now - 30 },
    ]

    expect(buildProjectStats(tasks, now)).toEqual({
      active: 1,
      completed: 1,
      highPriority: 2,
      overdue: 1,
      pending: 1,
      total: 4,
    })
  })

  it("summarizes task load by assignee and includes unassigned tasks", () => {
    const tasks: ProjectTask[] = [
      {
        uuid: "1",
        title: "alice overdue",
        status: "pending",
        due: now - 10,
        assignees: [{ id: "u1", name: "Alice" }],
      },
      {
        uuid: "2",
        title: "alice active",
        status: "active",
        assignees: [{ id: "u1", name: "Alice" }],
      },
      {
        uuid: "3",
        title: "bob completed",
        status: "completed",
        assignees: [{ id: "u2", name: "Bob" }],
      },
      { uuid: "4", title: "unassigned", status: "pending" },
    ]

    expect(summarizeAssignees(tasks, now, "未分配")).toEqual([
      { key: "u1", label: "Alice", open: 2, overdue: 1, total: 2 },
      { key: "unassigned", label: "未分配", open: 1, overdue: 0, total: 1 },
      { key: "u2", label: "Bob", open: 0, overdue: 0, total: 1 },
    ])
  })
})
