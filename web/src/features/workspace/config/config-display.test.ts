import { describe, expect, it } from "vitest"

import { formatConfigDisplayValue } from "./config-display"

describe("formatConfigDisplayValue", () => {
  it("returns empty string for null", () => {
    expect(formatConfigDisplayValue("string", null)).toBe("")
  })

  it("returns raw value for string type", () => {
    expect(formatConfigDisplayValue("string", "hello")).toBe("hello")
  })

  it("returns raw value for number type", () => {
    expect(formatConfigDisplayValue("number", "100")).toBe("100")
  })

  it("formats date as YYYY-MM-DD", () => {
    expect(formatConfigDisplayValue("date", "2026-07-07")).toBe("2026-07-07")
  })

  it("formats datetime in local time without RFC3339 markers", () => {
    const result = formatConfigDisplayValue("datetime", "2026-07-07T12:00:00Z")
    // 不应包含 RFC3339 的 T 或 Z 分隔符
    expect(result).not.toMatch(/T/)
    expect(result).not.toMatch(/Z/)
    // 应包含日期段
    expect(result).toMatch(/2026-07/)
  })

  it("returns empty string when date value is null", () => {
    expect(formatConfigDisplayValue("date", null)).toBe("")
  })

  it("falls back to raw value on invalid date", () => {
    // 解析失败时不应抛错，回退原值
    expect(formatConfigDisplayValue("date", "not-a-date")).toBe("not-a-date")
  })

  it("falls back to raw value on invalid datetime", () => {
    expect(formatConfigDisplayValue("datetime", "garbage")).toBe("garbage")
  })

  it("returns raw value for unknown type", () => {
    expect(formatConfigDisplayValue("custom-type", "xxx")).toBe("xxx")
  })
})
