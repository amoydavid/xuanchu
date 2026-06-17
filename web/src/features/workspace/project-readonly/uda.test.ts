import { describe, expect, it } from "vitest"

import { extractUDAs, formatUDAValue } from "./uda"

describe("extractUDAs", () => {
  it("returns only non-reserved, non-empty fields", () => {
    const task = {
      uuid: "u1",
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
