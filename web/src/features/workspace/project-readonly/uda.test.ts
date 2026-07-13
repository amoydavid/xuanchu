import { describe, expect, it } from "vitest"

import { extractUDAs, formatUDAValue } from "./uda"

describe("extractUDAs", () => {
  it("returns only non-reserved, non-empty fields", () => {
    const task = {
      uuid: "u1",
      title: "title",
      description: "desc",
      status: "pending",
      due: 100,
      estimate: "4h",
      sprint: "26W24",
      empty_uda: "",
      null_uda: null,
    }
    const udas = extractUDAs(task)
    const keys = udas.map(([k]) => k)
    expect(keys).toEqual(["estimate", "sprint"])
  })

  it("returns empty array when only reserved fields present", () => {
    expect(extractUDAs({ uuid: "u1", status: "pending" })).toEqual([])
  })

  it("excludes relation expansion fields (depends_info/parent_info) added by backend", () => {
    // 回归保护：后端给 JSONTask 新增的 depends_info/parent_info 必须被识别为标准字段，
    // 否则会被误显示为自定义 UDA（曾出现过的真实 bug）。
    const task = {
      uuid: "u1",
      status: "pending",
      depends: ["dep-1"],
      depends_info: [{ uuid: "dep-1", title: "依赖任务" }],
      parent: "p1",
      parent_info: { uuid: "p1", title: "父任务" },
      estimate: "4h",
    }
    expect(extractUDAs(task)).toEqual([["estimate", "4h"]])
  })

  it("uses explicit nested UDAs without inferring protocol metadata as custom fields", () => {
    expect(
      extractUDAs({
        id: "occ:series-1:1784044799",
        workspace_id: "workspace-1",
        project_id: "project-1",
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          recurrence_at: 1_784_044_799,
          materialization: "projected",
        },
        udas: { channel: "browser-e2e" },
      })
    ).toEqual([["channel", "browser-e2e"]])
  })
})

describe("formatUDAValue", () => {
  it("formats primitives as strings", () => {
    expect(formatUDAValue("4h")).toBe("4h")
    expect(formatUDAValue(42)).toBe("42")
    expect(formatUDAValue(true)).toBe("true")
  })

  it("renders nullish as dash", () => {
    expect(formatUDAValue(null)).toBe("-")
    expect(formatUDAValue(undefined)).toBe("-")
  })

  it("json-stringifies objects", () => {
    expect(formatUDAValue({ a: 1 })).toBe('{"a":1}')
  })
})
