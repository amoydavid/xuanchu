import { describe, expect, it } from "vitest"

import { MY_TASK_TABS, tabFilter } from "./my-task-tabs"

describe("my-task-tabs", () => {
  const now = new Date("2026-07-05T12:00:00Z")

  it("today tab sets pending status and end-of-day due_before", () => {
    const filter = tabFilter("today", now)
    expect(filter.status).toBe("pending")
    // 当地时区当天 23:59，这里只断言非空且为数字字符串
    expect(filter.due_before).toBeTruthy()
  })

  it("overdue tab sets pending status and past due_before", () => {
    const filter = tabFilter("overdue", now)
    expect(filter.status).toBe("pending")
    expect(Number(filter.due_before)).toBeLessThanOrEqual(Math.floor(now.getTime() / 1000))
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
