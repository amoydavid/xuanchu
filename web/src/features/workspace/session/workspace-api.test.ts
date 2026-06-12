import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api"

import { workspaceApiGet } from "./workspace-api"
import { setWorkspaceToken } from "./workspace-token"

describe("workspace api client", () => {
  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("rejects admin API paths", async () => {
    await expect(
      workspaceApiGet("/api/v1/admin/session")
    ).rejects.toMatchObject({
      code: "workspace_api_path_invalid",
    } satisfies Partial<ApiError>)
  })

  it("attaches workspace bearer token for normal API paths", async () => {
    setWorkspaceToken("xuanchu_pat_test")
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ data: { ok: true } }), { status: 200 })
      )

    await expect(workspaceApiGet("/api/v1/me")).resolves.toEqual({ ok: true })
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      headers: expect.objectContaining({
        Authorization: "Bearer xuanchu_pat_test",
      }),
    })
  })
})
