import { describe, expect, it } from "vitest"

import { formatLocalDate } from "@/features/workspace/project-workbench/shared/date-boundary"

import { MY_TASK_TABS, tabFilter } from "./my-task-tabs"

describe("my-task-tabs", () => {
  const now = new Date("2026-07-12T12:00:00Z")

  it("exposes 6 mutually exclusive presets", () => {
    expect(MY_TASK_TABS.map((t) => t.key)).toEqual([
      "incomplete",
      "started",
      "today",
      "overdue",
      "noDue",
      "completed",
    ])
  })

  it("incomplete tab queries pending OR waiting", () => {
    const filter = tabFilter("incomplete", now)
    expect(filter.query).toBe("(status:pending or status:waiting)")
  })

  it("maps started to open tasks with a start timestamp", () => {
    expect(tabFilter("started", now)).toEqual({
      query: "(status:pending or status:waiting) and start.notnull",
    })
  })

  it("today tab queries open with today date range", () => {
    const filter = tabFilter("today", now)
    expect(filter.query).toBe("(status:pending or status:waiting)")
    expect(filter.due_after).toBe(formatLocalDate(now))
    expect(filter.due_before).toBe(formatLocalDate(now))
  })

  it("overdue tab queries open with yesterday due_before", () => {
    const filter = tabFilter("overdue", now)
    expect(filter.query).toBe("(status:pending or status:waiting)")
    const yesterday = new Date(2026, 6, 11)
    expect(filter.due_before).toBe(formatLocalDate(yesterday))
  })

  it("noDue tab queries open with due_empty", () => {
    const filter = tabFilter("noDue", now)
    expect(filter.query).toBe("(status:pending or status:waiting)")
    expect(filter.due_empty).toBe("true")
  })

  it("completed tab sets status=completed", () => {
    const filter = tabFilter("completed", now)
    expect(filter.status).toBe("completed")
  })

  it("never produces an 'active' status", () => {
    for (const tab of MY_TASK_TABS) {
      const filter = tabFilter(tab.key, now)
      expect(filter.status).not.toBe("active")
    }
  })
})
