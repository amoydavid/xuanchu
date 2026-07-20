import { QueryClient } from "@tanstack/react-query"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"

import {
  appendProjectTemplateSnapshot,
  archiveProjectTemplate,
  createProjectTemplate,
  getProjectTemplate,
  instantiateProjectTemplate,
  listProjectTemplateTaskCandidates,
  invalidateProjectTemplateMutation,
  listProjectTemplates,
  modifyProjectTemplate,
  projectTemplateDetailQueryKey,
  projectTemplateListQueryKey,
  reactivateProjectTemplate,
  resolveProjectTemplateCandidateSelection,
} from "./project-template-api"

function ok(data: unknown = {}) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), {
      headers: { "Content-Type": "application/json" },
      status: 200,
    })
  )
}

describe("project template api", () => {
  beforeEach(() => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("keeps list and detail query keys stable", () => {
    expect(
      projectTemplateListQueryKey("acme", "all", "launch", 20, 40)
    ).toEqual(["project-templates", "acme", "all", "launch", 20, 40])
    expect(projectTemplateDetailQueryKey("acme", "launch", "snap-1")).toEqual([
      "project-template",
      "acme",
      "launch",
      "snap-1",
    ])
  })

  it("keeps template API scoped to workspace and never requests raw JSON", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => ok())

    await getProjectTemplate("acme", "launch", "snap-1")

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/project-templates/launch?workspace=acme&snapshot_id=snap-1",
      expect.objectContaining({ method: "GET" })
    )
    expect(String(fetchMock.mock.calls[0]?.[0])).not.toContain("raw")
    expect(String(fetchMock.mock.calls[0]?.[0])).not.toContain("snapshot_json")
  })

  it("encodes list filters and pagination without dropping zero offset", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => ok())

    await listProjectTemplates("acme", {
      status: "archived",
      q: "上线 模板",
      limit: 20,
      offset: 0,
    })

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/project-templates?workspace=acme&status=archived&q=%E4%B8%8A%E7%BA%BF+%E6%A8%A1%E6%9D%BF&limit=20&offset=0",
      expect.objectContaining({ method: "GET" })
    )
  })

  it("uses typed POST/PATCH bodies for lifecycle mutations", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => ok())
    const capture = {
      source_project: "ops",
      anchor_date: "2026-07-20",
      selection: {
        task_refs: ["task-1"],
        series_refs: [],
        config_keys: [],
        automation_rule_ids: [],
      },
    }

    await createProjectTemplate("acme", {
      key: "launch",
      name: "Launch",
      description: "",
      capture,
    })
    await appendProjectTemplateSnapshot("acme", "launch", capture)
    await modifyProjectTemplate("acme", "launch", { name: "Launch v2" })
    await archiveProjectTemplate("acme", "launch")
    await reactivateProjectTemplate("acme", "launch")

    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      "/api/v1/project-templates?workspace=acme",
      "/api/v1/project-templates/launch/snapshots?workspace=acme",
      "/api/v1/project-templates/launch?workspace=acme",
      "/api/v1/project-templates/launch/archive?workspace=acme",
      "/api/v1/project-templates/launch/reactivate?workspace=acme",
    ])
    expect(fetchMock.mock.calls[0]?.[1]).toEqual(
      expect.objectContaining({
        body: JSON.stringify({
          key: "launch",
          name: "Launch",
          description: "",
          capture,
        }),
        method: "POST",
      })
    )
    expect(fetchMock.mock.calls[2]?.[1]).toEqual(
      expect.objectContaining({
        body: JSON.stringify({ name: "Launch v2" }),
        method: "PATCH",
      })
    )
  })

  it("uses only Task 8 candidate and selection endpoints", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => ok())

    await listProjectTemplateTaskCandidates("acme", "ops/core", {
      q: "上线",
      status: "waiting",
      assignees: ["alice", "bob"],
      tags: ["launch"],
      limit: 50,
      offset: 100,
    })
    await resolveProjectTemplateCandidateSelection("acme", "ops/core", {
      kind: "task",
      task: { q: "上线", status: "waiting" },
    })

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      "/api/v1/projects/ops%2Fcore/template-candidates/tasks?workspace=acme&q=%E4%B8%8A%E7%BA%BF&status=waiting&assignee=alice&assignee=bob&tags=launch&limit=50&offset=100"
    )
    expect(fetchMock.mock.calls[1]).toEqual([
      "/api/v1/projects/ops%2Fcore/template-candidates/resolve-selection?workspace=acme",
      expect.objectContaining({
        body: JSON.stringify({
          kind: "task",
          task: { q: "上线", status: "waiting" },
        }),
        method: "POST",
      }),
    ])
  })

  it("keeps instantiate secrets in the request body instead of the URL", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => ok())
    const input = {
      snapshot_id: "snap-1",
      expected_snapshot_hash: "hash-1",
      project_slug: "new-project",
      project_name: "New Project",
      start_date: "2026-08-01",
      secret_inputs: { "provider.api_key": "sk-secret" },
    }

    await instantiateProjectTemplate("acme", "launch", input)

    const [url, options] = fetchMock.mock.calls[0] ?? []
    expect(String(url)).toBe(
      "/api/v1/project-templates/launch/instantiate?workspace=acme"
    )
    expect(String(url)).not.toContain("sk-secret")
    expect(options).toEqual(
      expect.objectContaining({ body: JSON.stringify(input), method: "POST" })
    )
  })

  it("invalidates only the affected template collections and project list", async () => {
    const queryClient = new QueryClient()
    const invalidate = vi
      .spyOn(queryClient, "invalidateQueries")
      .mockResolvedValue()

    await invalidateProjectTemplateMutation(queryClient, "acme", {
      kind: "modify",
      ref: "launch",
    })
    expect(invalidate).toHaveBeenNthCalledWith(1, {
      queryKey: ["project-templates", "acme"],
    })
    expect(invalidate).toHaveBeenNthCalledWith(2, {
      queryKey: ["project-template", "acme", "launch"],
    })
    expect(invalidate).toHaveBeenCalledTimes(2)

    invalidate.mockClear()
    await invalidateProjectTemplateMutation(queryClient, "acme", {
      kind: "instantiate",
      ref: "launch",
    })
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["project-templates", "acme"],
    })
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["project-template", "acme", "launch"],
    })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["projects", "acme"] })
    expect(invalidate).toHaveBeenCalledTimes(3)
  })
})
