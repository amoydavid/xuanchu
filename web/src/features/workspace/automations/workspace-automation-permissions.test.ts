import { describe, expect, it } from "vitest"

import {
  canReadAutomation,
  canWriteAutomation,
  hasScope,
} from "@/features/workspace/automations/workspace-automation-permissions"

describe("canReadAutomation", () => {
  it("returns true for owner via OIDC browser session", () => {
    expect(
      canReadAutomation({ role: "owner", actorType: "user", scopes: null })
    ).toBe(true)
  })

  it("returns true for admin via OIDC browser session", () => {
    expect(
      canReadAutomation({ role: "admin", actorType: "user", scopes: null })
    ).toBe(true)
  })

  it("returns false for member (no hook.read permission)", () => {
    expect(
      canReadAutomation({ role: "member", actorType: "user", scopes: null })
    ).toBe(false)
  })

  it("returns false for viewer", () => {
    expect(
      canReadAutomation({ role: "viewer", actorType: "user", scopes: null })
    ).toBe(false)
  })

  it("tenant token requires both workspace:read and hook:read", () => {
    expect(
      canReadAutomation({
        role: "admin",
        actorType: "tenant_access_token",
        scopes: ["workspace:read", "hook:read"],
      })
    ).toBe(true)
    expect(
      canReadAutomation({
        role: "admin",
        actorType: "tenant_access_token",
        scopes: ["workspace:read"],
      })
    ).toBe(false)
    expect(
      canReadAutomation({
        role: "admin",
        actorType: "tenant_access_token",
        scopes: ["hook:read"],
      })
    ).toBe(false)
  })

  it("wildcard scope grants access", () => {
    expect(
      canReadAutomation({
        role: "admin",
        actorType: "tenant_access_token",
        scopes: ["*"],
      })
    ).toBe(true)
  })

  it("admin role with both scopes but missing role returns false", () => {
    expect(
      canReadAutomation({
        role: "member",
        actorType: "user",
        scopes: ["workspace:read", "hook:read"],
      })
    ).toBe(false)
  })

  it("returns true for owner browser_session regardless of missing workspace scope", () => {
    expect(
      canReadAutomation({
        role: "owner",
        actorType: "user",
        tokenType: "browser_session",
        scopes: ["workspace:read", "hook:read"],
      })
    ).toBe(true)
  })
})

describe("canWriteAutomation", () => {
  it("returns true for owner via OIDC browser session", () => {
    expect(
      canWriteAutomation({ role: "owner", actorType: "user", scopes: null })
    ).toBe(true)
  })

  it("returns true for owner browser_session even without workspace:write scope", () => {
    // browser session 的 scope 集合是交互层人为收紧（httpapi.browserSessionScopes
    // 刻意不放 workspace:write），授权由 membership role 决定。
    expect(
      canWriteAutomation({
        role: "owner",
        actorType: "user",
        tokenType: "browser_session",
        scopes: ["workspace:read", "hook:read", "hook:write"],
      })
    ).toBe(true)
  })

  it("returns false for member browser_session", () => {
    expect(
      canWriteAutomation({
        role: "member",
        actorType: "user",
        tokenType: "browser_session",
        scopes: ["workspace:read", "hook:read", "hook:write"],
      })
    ).toBe(false)
  })

  it("tenant token requires both workspace:write and hook:write", () => {
    expect(
      canWriteAutomation({
        role: "admin",
        actorType: "tenant_access_token",
        scopes: ["workspace:write", "hook:write"],
      })
    ).toBe(true)
    expect(
      canWriteAutomation({
        role: "admin",
        actorType: "tenant_access_token",
        scopes: ["workspace:write"],
      })
    ).toBe(false)
  })
})

describe("hasScope", () => {
  it("treats null/undefined as no scope", () => {
    expect(hasScope(null, "workspace:read")).toBe(false)
    expect(hasScope(undefined, "workspace:read")).toBe(false)
  })

  it("matches exact scope", () => {
    expect(hasScope(["workspace:read"], "workspace:read")).toBe(true)
    expect(hasScope(["workspace:write"], "workspace:read")).toBe(false)
  })

  it("treats '*' as wildcard", () => {
    expect(hasScope(["*"], "anything")).toBe(true)
  })
})
