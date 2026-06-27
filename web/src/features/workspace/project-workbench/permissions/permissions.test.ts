import { describe, expect, it } from "vitest"

import { canProjectManage, canTaskWrite, hasScope } from "./permissions"

describe("project workbench permissions", () => {
  it("allows wildcard scopes", () => {
    expect(hasScope(["*"], "task:write")).toBe(true)
  })

  it("allows task writes for member with task:write", () => {
    expect(canTaskWrite({ role: "member", scopes: ["task:write"] })).toBe(true)
  })

  it("denies task writes for viewer even with task:write", () => {
    expect(canTaskWrite({ role: "viewer", scopes: ["task:write"] })).toBe(false)
  })

  it("allows project manage for owner/admin only", () => {
    expect(canProjectManage({ role: "owner", scopes: ["project:write"] })).toBe(
      true
    )
    expect(canProjectManage({ role: "admin", scopes: ["project:write"] })).toBe(
      true
    )
    expect(
      canProjectManage({ role: "member", scopes: ["project:write"] })
    ).toBe(false)
  })
})
