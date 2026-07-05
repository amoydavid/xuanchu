import { describe, expect, it } from "vitest"

import {
  hookPath,
  hookEnablePath,
  hookDisablePath,
  hookDeliveriesPath,
  hookDeliveryReplayPath,
} from "./hooks-api"

describe("hook paths", () => {
  it("list path", () => {
    expect(hookPath()).toBe("/api/v1/hooks")
  })

  it("single hook path encodes id", () => {
    expect(hookPath("h1")).toBe("/api/v1/hooks/h1")
  })

  it("enable/disable paths", () => {
    expect(hookEnablePath("h1")).toBe("/api/v1/hooks/h1/enable")
    expect(hookDisablePath("h1")).toBe("/api/v1/hooks/h1/disable")
  })

  it("deliveries list path", () => {
    expect(hookDeliveriesPath("h1")).toBe("/api/v1/hooks/h1/deliveries")
  })

  it("delivery replay path", () => {
    expect(hookDeliveryReplayPath("d1")).toBe("/api/v1/hook-deliveries/d1/replay")
  })
})
