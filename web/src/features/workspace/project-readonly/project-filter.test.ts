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
    const filter: TaskFilter = { status: "pending", priority: "H", q: "needle" }
    const qs = filterToTaskQuery(filter)
    expect(qs).toContain("status=pending")
    expect(qs).toContain("priority=H")
    expect(qs).toContain("q=needle")
  })

  it("returns empty string for empty filter", () => {
    expect(filterToTaskQuery({})).toBe("")
  })

  it("omits empty values", () => {
    const filter: TaskFilter = { status: "pending", priority: "" }
    expect(filterToTaskQuery(filter)).toBe("status=pending")
  })
})

describe("activeFilterEntries", () => {
  it("returns only entries with values", () => {
    const filter: TaskFilter = { status: "pending", priority: undefined, q: "x" }
    expect(activeFilterEntries(filter)).toEqual([
      ["status", "pending"],
      ["q", "x"],
    ])
  })
})
