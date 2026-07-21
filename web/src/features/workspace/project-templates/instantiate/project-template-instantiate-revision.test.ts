import { describe, expect, it } from "vitest"

import { isCurrentPreviewRevision } from "./project-template-instantiate-revision"

describe("isCurrentPreviewRevision", () => {
  it("rejects late successful and failed preview outcomes after an input change", () => {
    expect(isCurrentPreviewRevision(3, 4)).toBe(false)
  })

  it("accepts only the outcome for the current form revision", () => {
    expect(isCurrentPreviewRevision(4, 4)).toBe(true)
  })
})
