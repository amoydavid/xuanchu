import { describe, expect, it } from "vitest"

import { auditPath } from "./audit-api"

describe("auditPath", () => {
  it("builds minimal path with default limit", () => {
    expect(auditPath({})).toBe("/api/v1/audit?limit=50")
  })

  it("encodes project and custom limit", () => {
    expect(auditPath({ project: "proj-a", limit: 100 })).toBe(
      "/api/v1/audit?limit=100&project=proj-a"
    )
  })

  it("omits empty project", () => {
    expect(auditPath({ project: "", limit: 10 })).toBe(
      "/api/v1/audit?limit=10"
    )
  })
})
