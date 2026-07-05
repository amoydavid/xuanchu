import { describe, expect, it } from "vitest"

import { filterAuditRows, rowsToCSV, type AuditRow } from "./audit-filter"

const rows: AuditRow[] = [
  {
    id: 1,
    action: "task.modify",
    target_type: "task",
    target_id: "TASK-1",
    actor: { id: "u1", name: "alice" },
    created_at: 1756000000,
  },
  {
    id: 2,
    action: "project.transition",
    target_type: "project",
    target_id: "proj-a",
    actor: { id: "u2", name: "bob" },
    created_at: 1756100000,
  },
  {
    id: 3,
    action: "task.done",
    target_type: "task",
    target_id: "TASK-2",
    actor: { id: "u1", name: "alice" },
    created_at: 1756200000,
  },
]

describe("filterAuditRows", () => {
  it("filters by actor substring", () => {
    const out = filterAuditRows(rows, { actor: "alice" })
    expect(out.length).toBe(2)
  })

  it("filters by action substring", () => {
    const out = filterAuditRows(rows, { action: "task." })
    expect(out.length).toBe(2)
  })

  it("filters by target substring", () => {
    const out = filterAuditRows(rows, { target: "TASK-1" })
    expect(out.length).toBe(1)
  })

  it("filters by time range", () => {
    const out = filterAuditRows(rows, {
      after: 1756050000,
      before: 1756150000,
    })
    expect(out.length).toBe(1)
    expect(out[0].id).toBe(2)
  })

  it("returns all when no filter", () => {
    expect(filterAuditRows(rows, {}).length).toBe(3)
  })
})

describe("rowsToCSV", () => {
  it("produces header and rows, escaping commas and quotes", () => {
    const csv = rowsToCSV([
      {
        id: 1,
        action: "task,modify",
        target_type: "task",
        target_id: 'TASK-"1"',
        actor: { id: "u1", name: "alice" },
        created_at: 1756000000,
      },
    ])
    const lines = csv.split("\n")
    expect(lines[0]).toContain("time")
    expect(lines[0]).toContain("action")
    expect(lines[1]).toContain('"task,modify"')
    expect(lines[1]).toContain('"TASK-""1"""')
  })

  it("handles empty rows", () => {
    const csv = rowsToCSV([])
    expect(csv.split("\n")[0]).toContain("time")
  })
})
