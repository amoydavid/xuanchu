import { afterEach, describe, expect, it, vi } from "vitest"

import * as workspaceApi from "@/features/workspace/session/workspace-api"
import { suggestContentReferences } from "./content-reference-api"

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
  workspaceApiPost: vi.fn(),
}))

afterEach(() => {
  vi.restoreAllMocks()
})

describe("suggestContentReferences", () => {
  it("uses the GET suggestion endpoint required by the HTTP contract", async () => {
    vi.mocked(workspaceApi.workspaceApiGet).mockResolvedValue({
      results: [{
        type: "user",
        user: { id: "u1", name: "alice", display_name: "Alice" },
      }],
    })

    const controller = new AbortController()
    const results = await suggestContentReferences(
      { type: "user", query: "Alice", limit: 20 },
      { signal: controller.signal }
    )

    expect(workspaceApi.workspaceApiGet).toHaveBeenCalledWith(
      "/api/v1/content-references/suggestions?type=user&q=Alice&limit=20",
      { signal: controller.signal }
    )
    expect(workspaceApi.workspaceApiPost).not.toHaveBeenCalled()
    expect(results).toEqual([
      {
        type: "user",
        id: "u1",
        status: "resolved",
        user: { id: "u1", name: "alice", display_name: "Alice" },
      },
    ])
  })
})
