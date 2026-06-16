import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api"

import { adminApiDelete, adminApiGet, adminApiPatch } from "./admin-api"
import { setAdminToken } from "./admin-token"

describe("admin api client", () => {
  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("rejects non-admin API paths", async () => {
    await expect(adminApiGet("/api/v1/me")).rejects.toMatchObject({
      code: "admin_api_path_invalid",
    } satisfies Partial<ApiError>)
  })

  it("rejects non-admin paths for patch and delete", async () => {
    await expect(
      adminApiPatch("/api/v1/tokens/x", { name: "y" })
    ).rejects.toMatchObject({ code: "admin_api_path_invalid" })
    await expect(
      adminApiDelete("/api/v1/tokens/x")
    ).rejects.toMatchObject({ code: "admin_api_path_invalid" })
  })

  it("attaches admin bearer token only for admin paths", async () => {
    setAdminToken("xuanchu_admin_test")
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({ data: { token_name: "ops", capabilities: [] } }),
        {
          status: 200,
        }
      )
    )

    await expect(adminApiGet("/api/v1/admin/session")).resolves.toEqual({
      token_name: "ops",
      capabilities: [],
    })
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      headers: expect.objectContaining({
        Authorization: "Bearer xuanchu_admin_test",
      }),
    })
  })
})
