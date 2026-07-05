import { describe, expect, it } from "vitest"

import {
  canWriteHooks,
  canWriteReminderRules,
  canWriteSinks,
  hasScope,
  isOutboundReadonly,
} from "./outbound-permissions"

describe("hasScope", () => {
  it("matches explicit scope", () => {
    expect(hasScope(["hook:read", "hook:write"], "hook:write")).toBe(true)
  })

  it("wildcard grants all scopes", () => {
    expect(hasScope(["*"], "hook:write")).toBe(true)
    expect(hasScope(["*"], "anything:write")).toBe(true)
  })

  it("returns false on missing scope or empty list", () => {
    expect(hasScope(["hook:read"], "hook:write")).toBe(false)
    expect(hasScope(null, "hook:write")).toBe(false)
    expect(hasScope(undefined, "hook:write")).toBe(false)
  })
})

describe("canWriteHooks", () => {
  it("owner with hook:write can write hooks", () => {
    expect(
      canWriteHooks({ role: "owner", actorType: "user", scopes: ["hook:write"] })
    ).toBe(true)
  })

  it("admin with hook:write can write hooks", () => {
    expect(
      canWriteHooks({ role: "admin", actorType: "user", scopes: ["hook:write"] })
    ).toBe(true)
  })

  it("wildcard scope grants hooks for admin", () => {
    expect(
      canWriteHooks({ role: "admin", actorType: "user", scopes: ["*"] })
    ).toBe(true)
  })

  it("member with hook:write scope still cannot write hooks", () => {
    expect(
      canWriteHooks({ role: "member", actorType: "user", scopes: ["hook:write"] })
    ).toBe(false)
  })

  it("tenant access token with hook:write can write hooks", () => {
    expect(
      canWriteHooks({
        role: "member",
        actorType: "tenant_access_token",
        scopes: ["hook:write"],
      })
    ).toBe(true)
  })

  it("missing scope means read-only", () => {
    expect(
      canWriteHooks({ role: "owner", actorType: "user", scopes: ["hook:read"] })
    ).toBe(false)
  })

  it("OIDC session (no scopes) falls back to role check", () => {
    expect(canWriteHooks({ role: "owner", actorType: "user" })).toBe(true)
    expect(canWriteHooks({ role: "member", actorType: "user" })).toBe(false)
  })
})

describe("canWriteSinks", () => {
  it("admin with notification:write can write sinks", () => {
    expect(
      canWriteSinks({
        role: "admin",
        actorType: "user",
        scopes: ["notification:write"],
      })
    ).toBe(true)
  })

  it("tenant access token with notification:write can write sinks", () => {
    expect(
      canWriteSinks({
        role: "viewer",
        actorType: "tenant_access_token",
        scopes: ["notification:write"],
      })
    ).toBe(true)
  })
})

describe("canWriteReminderRules", () => {
  it("owner with reminder:write can write reminder rules", () => {
    expect(
      canWriteReminderRules({
        role: "owner",
        actorType: "user",
        scopes: ["reminder:write"],
      })
    ).toBe(true)
  })
})

describe("isOutboundReadonly", () => {
  it("returns true when no write permissions at all", () => {
    expect(
      isOutboundReadonly({ role: "viewer", actorType: "user", scopes: [] })
    ).toBe(true)
  })

  it("returns false when at least one write permission exists", () => {
    expect(
      isOutboundReadonly({
        role: "owner",
        actorType: "user",
        scopes: ["hook:write"],
      })
    ).toBe(false)
  })
})
