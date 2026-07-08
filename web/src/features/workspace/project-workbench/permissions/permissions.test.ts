import { describe, expect, it } from "vitest"

import {
  canAuditRead,
  canProjectAutomationRead,
  canProjectAutomationWrite,
  canProjectManage,
  canTaskRead,
  canTaskWrite,
  hasScope,
} from "./permissions"

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

  it("allows task read for owner/admin/member/viewer with task:read", () => {
    expect(canTaskRead({ role: "owner", scopes: ["task:read"] })).toBe(true)
    expect(canTaskRead({ role: "admin", scopes: ["task:read"] })).toBe(true)
    expect(canTaskRead({ role: "member", scopes: ["task:read"] })).toBe(true)
    expect(canTaskRead({ role: "viewer", scopes: ["task:read"] })).toBe(true)
  })

  it("denies task read when missing scope", () => {
    expect(canTaskRead({ role: "owner", scopes: ["project:read"] })).toBe(false)
    expect(canTaskRead({ role: "member", scopes: [] })).toBe(false)
  })

  it("allows audit read only for owner/admin with audit:read", () => {
    expect(canAuditRead({ role: "owner", scopes: ["audit:read"] })).toBe(true)
    expect(canAuditRead({ role: "admin", scopes: ["audit:read"] })).toBe(true)
  })

  it("denies audit read for member or missing scope", () => {
    expect(canAuditRead({ role: "member", scopes: ["audit:read"] })).toBe(false)
    expect(canAuditRead({ role: "viewer", scopes: ["audit:read"] })).toBe(false)
    expect(canAuditRead({ role: "owner", scopes: ["task:read"] })).toBe(false)
  })

  it("allows project automation write only with project and hook write", () => {
    expect(
      canProjectAutomationWrite({ role: "admin", scopes: ["project:write", "hook:write"] })
    ).toBe(true)
    expect(
      canProjectAutomationWrite({ role: "admin", scopes: ["project:write"] })
    ).toBe(false)
    expect(
      canProjectAutomationWrite({ role: "viewer", scopes: ["project:write", "hook:write"] })
    ).toBe(false)
  })

  it("allows project automation read for members with project and hook read", () => {
    expect(
      canProjectAutomationRead({ role: "member", scopes: ["project:read", "hook:read"] })
    ).toBe(true)
    expect(
      canProjectAutomationRead({ role: "member", scopes: ["project:read"] })
    ).toBe(false)
  })
})
