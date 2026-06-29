import { describe, expect, it, vi } from "vitest"
import { utils } from "xlsx"

import {
  buildTaskImportTemplateWorkbook,
  workbookToTaskImportPayload,
} from "./task-import-xlsx"

const NOW = "2026-06-28T00:00:00.000Z"

describe("task import xlsx helpers", () => {
  it("builds an xlsx workbook template with task import columns", () => {
    const workbook = buildTaskImportTemplateWorkbook()
    const sheet = workbook.Sheets.Tasks
    const rows = utils.sheet_to_json<Record<string, unknown>>(sheet)
    const helpRows = utils.sheet_to_json<Record<string, unknown>>(
      workbook.Sheets["字段说明"]
    )

    expect(workbook.SheetNames).toContain("Tasks")
    expect(workbook.SheetNames).toContain("字段说明")
    expect(rows[0]).toMatchObject({
      id: "task-1",
      title: "示例任务",
      description: "可使用 Markdown 记录详细说明",
      assignees: "alice, bob@example.com",
      blocked_by: "",
    })
    expect(Object.keys(rows[0])).toEqual(
      expect.arrayContaining([
        "id",
        "title",
        "description",
        "status",
        "priority",
        "tags",
        "assignees",
        "blocked_by",
        "due",
        "wait",
        "scheduled",
        "until",
        "start",
        "end",
        "recur",
        "parent",
        "task_slug",
        "annotations",
        "links",
        "uda.estimate",
      ])
    )
    const taskColumns = Object.keys(rows[0])
    const helpFields = helpRows.map((row) => row.field)
    expect(helpFields).toEqual(expect.arrayContaining(taskColumns))
    expect(helpRows[0]).toEqual(
      expect.objectContaining({
        allowed_values: expect.any(String),
        example: expect.any(String),
        format: expect.any(String),
        type: expect.any(String),
      })
    )
    expect(helpRows).toContainEqual(
      expect.objectContaining({
        allowed_values: "pending, completed, deleted, waiting, recurring",
        field: "status",
        type: "enum",
      })
    )
    expect(helpRows).toContainEqual(
      expect.objectContaining({
        allowed_values: "H, M, L",
        field: "priority",
        type: "enum",
      })
    )
    expect(sheet["!autofilter"]).toEqual({ ref: "A1:T2" })
    expect(sheet["!cols"]?.[0]).toEqual(expect.objectContaining({ wch: 18 }))
    expect(sheet["I2"]).toEqual(
      expect.objectContaining({
        t: "d",
        z: "yyyy-mm-dd",
      })
    )
    for (const cell of ["A1", "D1", "E1", "H1"]) {
      expect(sheet[cell]?.c).toBeUndefined()
    }
    expect(helpRows).toContainEqual(
      expect.objectContaining({
        description: expect.stringContaining("当前任务被哪些任务阻塞"),
        field: "blocked_by",
      })
    )
  })

  it("converts the first worksheet to import payload", () => {
    vi.spyOn(crypto, "randomUUID").mockReturnValue(
      "33333333-3333-4333-8333-333333333333"
    )
    const workbook = utils.book_new()
    const sheet = utils.json_to_sheet([
      {
        id: "task-a",
        title: "批量任务",
        description: "来自 Excel",
        due: "2026-07-01",
        assignees: "alice",
        blocked_by: "ads-1",
        "uda.estimate": 5,
      },
    ])
    utils.book_append_sheet(workbook, sheet, "Tasks")

    const payload = workbookToTaskImportPayload(workbook, {
      nowISO: NOW,
      projectSlug: "adsops",
    })

    expect(payload.tasks).toMatchObject([
      {
        uuid: "33333333-3333-4333-8333-333333333333",
        title: "批量任务",
        description: "来自 Excel",
        due: "2026-07-01T00:00:00.000Z",
        assignees: ["alice"],
        depends: ["ads-1"],
        estimate: "5",
      },
    ])
  })
})
