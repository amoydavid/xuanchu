import { describe, expect, it } from "vitest"

import {
  activeFilterEntries,
  emptyFilter,
  filterToTaskQuery,
  type TaskFilter,
} from "./project-filter"

describe("emptyFilter", () => {
  it("returns true for empty filter", () => {
    expect(emptyFilter({})).toBe(true)
  })

  it("returns false when any field set", () => {
    expect(emptyFilter({ status: "pending" })).toBe(false)
    expect(emptyFilter({ q: "x" })).toBe(false)
  })
})

describe("filterToTaskQuery", () => {
  it("encodes set fields as query string", () => {
    const filter: TaskFilter = {
      status: "pending",
      priority: "H",
      q: "needle",
      sort: "due",
    }
    const qs = filterToTaskQuery(filter)
    expect(qs).toContain("status=pending")
    expect(qs).toContain("priority=H")
    expect(qs).toContain("q=needle")
    expect(qs).toContain("sort=due")
  })

  it("returns empty string for empty filter", () => {
    expect(filterToTaskQuery({})).toBe("")
  })

  it("omits empty values", () => {
    const filter: TaskFilter = { status: "pending", priority: "" }
    expect(filterToTaskQuery(filter)).toBe("status=pending")
  })

  it("compiles advanced filters into backend query expressions", () => {
    const filter: TaskFilter = {
      assignee_empty: "true",
      due_empty: "true",
      query: "annotations:blocked",
      scheduled_before: "2026-07-15",
      until_before: "2026-07-31",
      wait_before: "2026-07-10",
    }
    const params = new URLSearchParams(filterToTaskQuery(filter))

    expect(params.getAll("query")).toEqual([
      "annotations:blocked",
      "assignee.isnull",
      "due.isnull",
      "wait.before:2026-07-10",
      "scheduled.before:2026-07-15",
      "until.before:2026-07-31",
    ])
  })

  it("compiles multiple assignees into an OR query expression", () => {
    const params = new URLSearchParams(
      filterToTaskQuery({
        assignee: "user-1,user-2",
        status: "pending",
      })
    )

    expect(params.get("status")).toBe("pending")
    expect(params.get("assignee")).toBeNull()
    expect(params.getAll("query")).toEqual([
      "(assignee:\"user-1\" or assignee:\"user-2\")",
    ])
  })
})

describe("activeFilterEntries", () => {
  it("returns only entries with values", () => {
    const filter: TaskFilter = {
      status: "pending",
      priority: undefined,
      q: "x",
    }
    expect(activeFilterEntries(filter)).toEqual([
      ["status", "pending"],
      ["q", "x"],
    ])
  })
})
