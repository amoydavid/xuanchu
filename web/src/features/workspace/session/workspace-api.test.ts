import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api"

import { workspaceApiGet } from "./workspace-api"
import {
  clearAdminActingSession,
  getAdminActingToken,
  getWorkspaceToken,
  setAdminActingToken,
  setAdminActingContext,
  setWorkspaceToken,
} from "./workspace-token"

describe("workspace api client", () => {
  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("rejects admin API paths", async () => {
    await expect(workspaceApiGet("/api/v1/admin/session")).rejects.toMatchObject({
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

  it("uses acting token first when acting context exists", async () => {
    setWorkspaceToken("xuanchu_pat_normal")
    setAdminActingToken("xuanchu_act_acting")
    setAdminActingContext({
      workspaceSlug: "dajee",
      workspaceName: "Dajee",
      actorName: "alice",
      role: "owner",
      adminTokenName: "ops",
    })
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ data: { ok: true } }), { status: 200 })
      )

    await workspaceApiGet("/api/v1/me")
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      headers: expect.objectContaining({
        Authorization: "Bearer xuanchu_act_acting",
      }),
    })
  })

  it("clears acting session (not normal token or admin) on acting 401", async () => {
    setWorkspaceToken("xuanchu_pat_normal")
    setAdminActingToken("xuanchu_act_acting")
    setAdminActingContext({
      workspaceSlug: "dajee",
      workspaceName: "Dajee",
      actorName: "alice",
      role: "owner",
      adminTokenName: "ops",
    })
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: { code: "admin_acting_session_expired" } }), {
        status: 401,
      })
    )

    await expect(workspaceApiGet("/api/v1/me")).rejects.toThrow()
    expect(getAdminActingToken()).toBeNull()
    // 普通 workspace token 必须保留。
    expect(getWorkspaceToken()).toBe("xuanchu_pat_normal")
  })

  it("clears normal workspace token on non-acting 401", async () => {
    setWorkspaceToken("xuanchu_pat_normal")
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: { code: "auth_invalid_token" } }), {
        status: 401,
      })
    )

    await expect(workspaceApiGet("/api/v1/me")).rejects.toThrow()
    expect(getWorkspaceToken()).toBeNull()
  })

  it("clearAdminActingSession removes both acting token and context", () => {
    setAdminActingToken("xuanchu_act_x")
    setAdminActingContext({
      workspaceSlug: "dajee",
      workspaceName: "Dajee",
      actorName: "alice",
      role: "owner",
      adminTokenName: "ops",
    })
    clearAdminActingSession()
    expect(getAdminActingToken()).toBeNull()
  })
})
