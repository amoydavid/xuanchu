import { describe, expect, it } from "vitest"

import {
  normalizeProjectConfigEntries,
  projectAnnotationPath,
  projectAnnotationsPath,
  projectConfigKeyPath,
  projectConfigPath,
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

  it("builds project config and annotation paths", () => {
    expect(projectConfigPath("local", "adsops")).toBe(
      "/api/v1/projects/adsops/config?workspace=local"
    )
    expect(projectConfigKeyPath("local", "adsops", "review.required")).toBe(
      "/api/v1/projects/adsops/config/review.required?workspace=local"
    )
    expect(projectConfigKeyPath("local", "ads/ops", "a/b")).toBe(
      "/api/v1/projects/ads%2Fops/config/a%2Fb?workspace=local"
    )
    expect(projectAnnotationsPath("local", "adsops")).toBe(
      "/api/v1/projects/adsops/annotations?workspace=local"
    )
    expect(projectAnnotationPath("local", "adsops", "an-1")).toBe(
      "/api/v1/projects/adsops/annotations/an-1?workspace=local"
    )
  })
})

describe("normalizeProjectConfigEntries", () => {
  it("归一化后端返回的对象形态为按 key 排序的数组", () => {
    const entries = normalizeProjectConfigEntries({
      "agent.background": "Owns MCP",
      "ads.roi": "1.8",
    })
    expect(entries).toEqual([
      { key: "ads.roi", value: "1.8" },
      { key: "agent.background", value: "Owns MCP" },
    ])
  })

  it("已是数组时原样返回", () => {
    const input = [{ key: "agent.background", value: "Owns MCP" }]
    expect(normalizeProjectConfigEntries(input)).toBe(input)
  })

  it("null/undefined 返回空数组", () => {
    expect(normalizeProjectConfigEntries(null)).toEqual([])
    expect(normalizeProjectConfigEntries(undefined)).toEqual([])
  })

  it("非对象原始值返回空数组", () => {
    expect(normalizeProjectConfigEntries("hello")).toEqual([])
    expect(normalizeProjectConfigEntries(42)).toEqual([])
  })
})
