import { describe, expect, it } from "vitest"

import { dateToUnix } from "./date-boundary"

describe("dateToUnix", () => {
  it("uses end of local day for deadline fields", () => {
    const date = new Date(2030, 5, 15, 12, 0, 0)
    const got = new Date(dateToUnix(date, "end") * 1000)

    expect(got.getFullYear()).toBe(2030)
    expect(got.getMonth()).toBe(5)
    expect(got.getDate()).toBe(15)
    expect(got.getHours()).toBe(23)
    expect(got.getMinutes()).toBe(59)
    expect(got.getSeconds()).toBe(59)
  })

  it("uses start of local day for start fields", () => {
    const date = new Date(2030, 5, 15, 12, 0, 0)
    const got = new Date(dateToUnix(date, "start") * 1000)

    expect(got.getFullYear()).toBe(2030)
    expect(got.getMonth()).toBe(5)
    expect(got.getDate()).toBe(15)
    expect(got.getHours()).toBe(0)
    expect(got.getMinutes()).toBe(0)
    expect(got.getSeconds()).toBe(0)
  })
})
