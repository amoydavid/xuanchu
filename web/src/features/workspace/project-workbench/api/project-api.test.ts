import { describe, expect, it } from "vitest"

import {
  projectPath,
  projectTasksPath,
  projectTimelinePath,
  projectTransitionPath,
  projectsPath,
} from "./project-api"

describe("project workbench project api paths", () => {
  it("builds workspace-scoped project paths", () => {
    expect(projectsPath("local", "open")).toBe(
      "/api/v1/projects?workspace=local&status=open"
    )
    expect(projectPath("local", "adsops")).toBe(
      "/api/v1/projects/adsops?workspace=local"
    )
    expect(projectTransitionPath("local", "adsops")).toBe(
      "/api/v1/projects/adsops/transition?workspace=local"
    )
  })

  it("keeps task list filters readable", () => {
    expect(
      projectTasksPath("local", "adsops", { status: "pending", q: "copy" })
    ).toBe(
      "/api/v1/tasks?workspace=local&project=adsops&limit=200&status=pending&q=copy"
    )
  })

  it("encodes path and query values safely", () => {
    expect(projectPath("workspace 1", "ads/ops")).toBe(
      "/api/v1/projects/ads%2Fops?workspace=workspace%201"
    )
    expect(projectsPath("workspace 1", "needs review")).toBe(
      "/api/v1/projects?workspace=workspace%201&status=needs+review"
    )
    expect(projectTimelinePath("workspace 1", "ads/ops")).toBe(
      "/api/v1/projects/ads%2Fops/timeline?workspace=workspace%201&limit=20"
    )
    expect(
      projectTasksPath("workspace 1", "ads/ops", {
        q: "a&b",
        status: "needs review",
      })
    ).toBe(
      "/api/v1/tasks?workspace=workspace%201&project=ads%2Fops&limit=200&q=a%26b&status=needs+review"
    )
  })

  it("accepts URLSearchParams for task filters", () => {
    const filters = new URLSearchParams()
    filters.set("tags", "web,console")
    filters.set("sort", "due")
    expect(projectTasksPath("local", "adsops", filters)).toBe(
      "/api/v1/tasks?workspace=local&project=adsops&limit=200&tags=web%2Cconsole&sort=due"
    )
  })
})
