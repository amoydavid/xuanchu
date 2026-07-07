import { describe, expect, it } from "vitest"

import { formatLocalDate } from "@/features/workspace/project-workbench/shared/date-boundary"

import { MY_TASK_TABS, tabFilter } from "./my-task-tabs"

describe("my-task-tabs", () => {
  const now = new Date("2026-07-05T12:00:00Z")

  it("today tab sets pending status and YYYY-MM-DD due_before for today", () => {
    const filter = tabFilter("today", now)
    expect(filter.status).toBe("pending")
    // 后端只接受 YYYY-MM-DD；今日到期用今天日期（含当天整天）
    expect(filter.due_before).toBe(formatLocalDate(now))
    expect(/\d{4}-\d{2}-\d{2}/.test(filter.due_before ?? "")).toBe(true)
  })

  it("overdue tab sets pending status and YYYY-MM-DD due_before for yesterday", () => {
    const filter = tabFilter("overdue", now)
    expect(filter.status).toBe("pending")
    // 逾期 = 今天之前（昨天及更早），今天到期的不算逾期
    const yesterday = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate() - 1
    )
    expect(filter.due_before).toBe(formatLocalDate(yesterday))
    expect(/\d{4}-\d{2}-\d{2}/.test(filter.due_before ?? "")).toBe(true)
  })

  it("noDue tab sets pending status and due_empty=true", () => {
    const filter = tabFilter("noDue", now)
    expect(filter.status).toBe("pending")
    expect(filter.due_empty).toBe("true")
  })

  it("all tab only sets nothing beyond defaults", () => {
    const filter = tabFilter("all", now)
    expect(filter.status).toBe("pending")
    expect(filter.due_before).toBeUndefined()
    expect(filter.due_empty).toBeUndefined()
  })

  it("MY_TASK_TABS lists all expected tabs", () => {
    const keys = MY_TASK_TABS.map((tab) => tab.key)
    expect(keys).toEqual(["all", "today", "overdue", "noDue"])
  })
})
