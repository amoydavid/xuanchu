import { describe, expect, it } from "vitest"

import {
  isCanonicalRecurrenceRule,
  nextRecurrenceDate,
  previewRecurrenceDates,
  parseDateToEndOfDay,
  formatDateShort,
} from "./recurrence-preview"

describe("isCanonicalRecurrenceRule", () => {
  it("accepts canonical rules", () => {
    expect(isCanonicalRecurrenceRule("daily")).toBe(true)
    expect(isCanonicalRecurrenceRule("weekly")).toBe(true)
    expect(isCanonicalRecurrenceRule("monthly")).toBe(true)
    expect(isCanonicalRecurrenceRule("2weeks")).toBe(true)
    expect(isCanonicalRecurrenceRule("3months")).toBe(true)
    expect(isCanonicalRecurrenceRule("12months")).toBe(true)
    expect(isCanonicalRecurrenceRule("1days")).toBe(true)
  })

  it("rejects aliases and invalid", () => {
    expect(isCanonicalRecurrenceRule("biweekly")).toBe(false)
    expect(isCanonicalRecurrenceRule("quarterly")).toBe(false)
    expect(isCanonicalRecurrenceRule("annual")).toBe(false)
    expect(isCanonicalRecurrenceRule("0days")).toBe(false)
    expect(isCanonicalRecurrenceRule("abc")).toBe(false)
  })
})

describe("nextRecurrenceDate", () => {
  it("daily advances by 1 day", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const next = nextRecurrenceDate(from, "daily")
    expect(formatDateShort(next)).toBe("2030-01-16")
  })

  it("weekly advances by 7 days", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const next = nextRecurrenceDate(from, "weekly")
    expect(formatDateShort(next)).toBe("2030-01-22")
  })

  it("2weeks advances by 14 days", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const next = nextRecurrenceDate(from, "2weeks")
    expect(formatDateShort(next)).toBe("2030-01-29")
  })

  it("monthly advances by 1 month", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const next = nextRecurrenceDate(from, "monthly")
    expect(formatDateShort(next)).toBe("2030-02-15")
  })

  it("3months advances by 3 months", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const next = nextRecurrenceDate(from, "3months")
    expect(formatDateShort(next)).toBe("2030-04-15")
  })

  it("12months advances by 12 months (yearly)", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const next = nextRecurrenceDate(from, "12months")
    expect(formatDateShort(next)).toBe("2031-01-15")
  })
})

describe("previewRecurrenceDates", () => {
  it("generates next 3 slots", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const dates = previewRecurrenceDates(from, "daily", 3)
    expect(dates).toHaveLength(3)
    expect(formatDateShort(dates[0])).toBe("2030-01-16")
    expect(formatDateShort(dates[1])).toBe("2030-01-17")
    expect(formatDateShort(dates[2])).toBe("2030-01-18")
  })

  it("stops at until boundary", () => {
    const from = parseDateToEndOfDay("2030-01-15")!
    const until = parseDateToEndOfDay("2030-01-17")!
    const dates = previewRecurrenceDates(from, "daily", 5, until)
    expect(formatDateShort(dates[0])).toBe("2030-01-16")
    expect(formatDateShort(dates[1])).toBe("2030-01-17")
    // 01-18 超过 until 01-17，停止
    expect(dates).toHaveLength(2)
  })
})

describe("parseDateToEndOfDay", () => {
  it("parses YYYY-MM-DD to 23:59:59", () => {
    const date = parseDateToEndOfDay("2030-06-15")
    expect(date).not.toBeNull()
    expect(date!.getHours()).toBe(23)
    expect(date!.getMinutes()).toBe(59)
  })

  it("returns null for invalid", () => {
    expect(parseDateToEndOfDay("not-a-date")).toBeNull()
    expect(parseDateToEndOfDay("2030-13-45")).toBeNull()
  })
})
