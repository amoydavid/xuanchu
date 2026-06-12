import { afterEach, describe, expect, it, vi } from "vitest"

import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"

import { ApiError } from "./api"

describe("api client", () => {
  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("unwraps data envelopes and attaches bearer token", async () => {
    setWorkspaceToken("xuanchu_pat_test")
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ data: { ok: true } }), { status: 200 })
      )
    await expect(
      workspaceApiGet<{ ok: boolean }>("/api/v1/me")
    ).resolves.toEqual({
      ok: true,
    })
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      headers: expect.objectContaining({
        Authorization: "Bearer xuanchu_pat_test",
      }),
    })
  })

  it("throws sanitized API errors", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          error: {
            code: "token_scope_denied",
            message: "scope denied for secret token",
          },
        }),
        { status: 403 }
      )
    )
    await expect(workspaceApiGet("/api/v1/tokens")).rejects.toMatchObject({
      code: "token_scope_denied",
      status: 403,
    } satisfies Partial<ApiError>)
  })
})
