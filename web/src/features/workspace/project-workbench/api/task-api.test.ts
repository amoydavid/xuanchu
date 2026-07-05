import { describe, expect, it } from "vitest"

import {
  importTasksPath,
  taskAnnotationItemPath,
  taskAnnotationPath,
  taskAuditPath,
  taskDonePath,
  taskLinkItemPath,
  taskLinkPath,
  taskPath,
  taskStartPath,
  taskUrgencyPath,
} from "./task-api"

describe("project workbench task api paths", () => {
  it("builds workspace-scoped task mutation paths", () => {
    expect(taskPath("local", "ads-1")).toBe(
      "/api/v1/tasks/ads-1?workspace=local"
    )
    expect(taskDonePath("local", "ads-1")).toBe(
      "/api/v1/tasks/ads-1/done?workspace=local"
    )
    expect(taskStartPath("local", "ads-1")).toBe(
      "/api/v1/tasks/ads-1/start?workspace=local"
    )
    expect(taskAnnotationPath("local", "ads-1")).toBe(
      "/api/v1/tasks/ads-1/annotations?workspace=local"
    )
    expect(taskLinkPath("local", "ads-1")).toBe(
      "/api/v1/tasks/ads-1/links?workspace=local"
    )
    expect(importTasksPath("local", "adsops")).toBe(
      "/api/v1/import?workspace=local&project=adsops"
    )
  })

  it("encodes task refs and nested item ids safely", () => {
    expect(taskPath("workspace 1", "ads/1")).toBe(
      "/api/v1/tasks/ads%2F1?workspace=workspace%201"
    )
    expect(taskAnnotationItemPath("workspace 1", "ads/1", "note/7")).toBe(
      "/api/v1/tasks/ads%2F1/annotations/note%2F7?workspace=workspace%201"
    )
    expect(taskLinkItemPath("workspace 1", "ads/1", "link/7")).toBe(
      "/api/v1/tasks/ads%2F1/links/link%2F7?workspace=workspace%201"
    )
    expect(importTasksPath("workspace 1", "ops/team")).toBe(
      "/api/v1/import?workspace=workspace%201&project=ops%2Fteam"
    )
  })

  it("builds task audit path with encoded workspace and task ref", () => {
    expect(taskAuditPath("workspace 1", "ads/1")).toBe(
      "/api/v1/tasks/ads%2F1/audit?workspace=workspace%201"
    )
  })

  it("builds task urgency path", () => {
    expect(taskUrgencyPath("local", "ads-1")).toBe(
      "/api/v1/tasks/ads-1/urgency?workspace=local"
    )
    expect(taskUrgencyPath("workspace 1", "ads/1")).toBe(
      "/api/v1/tasks/ads%2F1/urgency?workspace=workspace%201"
    )
  })
})
