import { describe, expect, it } from "vitest"

import {
  hookDeliveriesPath,
  hookDeliveryPath,
  hookPath,
  notificationDeliveriesPath,
  notificationSinkDisablePath,
  notificationSinkEnablePath,
  notificationSinkPath,
  notificationSinkTestPath,
} from "./outbound-api"

describe("notification sink paths", () => {
  it("list path", () => {
    expect(notificationSinkPath()).toBe("/api/v1/notification-sinks")
  })

  it("single sink path encodes id", () => {
    expect(notificationSinkPath("s1")).toBe("/api/v1/notification-sinks/s1")
  })

  it("enable/disable paths", () => {
    expect(notificationSinkEnablePath("s1")).toBe(
      "/api/v1/notification-sinks/s1/enable"
    )
    expect(notificationSinkDisablePath("s1")).toBe(
      "/api/v1/notification-sinks/s1/disable"
    )
  })

  it("test path", () => {
    expect(notificationSinkTestPath("s1")).toBe(
      "/api/v1/notification-sinks/s1/test"
    )
  })
})

describe("hook paths", () => {
  it("list path", () => {
    expect(hookPath()).toBe("/api/v1/hooks")
  })

  it("deliveries list path", () => {
    expect(hookDeliveriesPath("h1")).toBe("/api/v1/hooks/h1/deliveries")
  })

  it("single delivery path", () => {
    expect(hookDeliveryPath("d1")).toBe("/api/v1/hook-deliveries/d1")
  })
})

describe("notification delivery query path", () => {
  it("builds query string from provided filters", () => {
    expect(
      notificationDeliveriesPath({ sink: "s1", status: "dead_lettered", limit: 20 })
    ).toBe("/api/v1/notification-deliveries?sink=s1&status=dead_lettered&limit=20")
  })

  it("omits empty filters", () => {
    expect(notificationDeliveriesPath({})).toBe("/api/v1/notification-deliveries")
  })
})
