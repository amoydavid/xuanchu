import { describe, expect, it } from "vitest"

import {
  taskSeriesPath,
  taskSeriesItemPath,
  taskSeriesOccurrencesPath,
  taskSeriesSkipPath,
  tasksRangePath,
} from "./task-series-api"

describe("taskSeriesPath", () => {
  it("builds basic list URL with workspace and project", () => {
    expect(taskSeriesPath("acme", { project: "ops", status: "active" })).toBe(
      "/api/v1/task-series?workspace=acme&project=ops&status=active",
    )
  })

  it("encodes q parameter", () => {
    const path = taskSeriesPath("acme", { project: "ops", status: "active", q: "巡检", assignee: "zhangsan", sort: "next", limit: 20, offset: 20 })
    expect(path).toContain("q=%E5%B7%A1%E6%A3%80")
    expect(path).toContain("assignee=zhangsan")
    expect(path).toContain("sort=next")
    expect(path).toContain("limit=20")
    expect(path).toContain("offset=20")
  })
})

describe("taskSeriesItemPath", () => {
  it("encodes seriesRef", () => {
    expect(taskSeriesItemPath("acme", "series:1")).toBe(
      "/api/v1/task-series/series%3A1?workspace=acme",
    )
  })
})

describe("taskSeriesOccurrencesPath", () => {
  it("builds occurrences URL with status and pagination", () => {
    const path = taskSeriesOccurrencesPath("acme", "s1", { status: "pending", limit: 10, offset: 5 })
    expect(path).toContain("/api/v1/task-series/s1/occurrences")
    expect(path).toContain("status=pending")
    expect(path).toContain("limit=10")
    expect(path).toContain("offset=5")
  })
})

describe("taskSeriesSkipPath", () => {
  it("encodes occurrenceRef with colons", () => {
    const path = taskSeriesSkipPath("acme", "s1", "occ:s1:123")
    expect(path).toContain("/api/v1/task-series/s1/occurrences/")
    expect(path).toContain("occ%3As1%3A123")
    expect(path).toContain("/skip")
  })
})

describe("tasksRangePath", () => {
  it("builds tasks URL with occurrence_mode and date range", () => {
    const path = tasksRangePath("acme", { due_after: "2026-07-12", due_before: "2026-07-12", occurrence_mode: "expand" })
    expect(path).toContain("/api/v1/tasks?")
    expect(path).toContain("occurrence_mode=expand")
    expect(path).toContain("due_after=2026-07-12")
    expect(path).toContain("due_before=2026-07-12")
  })
})
