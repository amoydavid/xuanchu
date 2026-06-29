import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  adminApiDelete,
  adminApiPatch,
} from "@/features/admin/session/admin-api"

import {
  useAdminModifyTenantAccessTokenMutation,
  useAdminRevokeTenantAccessTokenMutation,
} from "./use-admin-token-mutations"

vi.mock("@/features/admin/session/admin-api", () => ({
  adminApiDelete: vi.fn(),
  adminApiPatch: vi.fn(),
}))

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

describe("tenant access token admin mutations", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(adminApiPatch).mockResolvedValue({ id: "tenant-1" })
    vi.mocked(adminApiDelete).mockResolvedValue({ ok: true })
  })

  it("modifies and revokes tenant access tokens through admin endpoints", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result: modify } = renderHook(
      () => useAdminModifyTenantAccessTokenMutation(),
      { wrapper: makeWrapper(queryClient) }
    )
    const { result: revoke } = renderHook(
      () => useAdminRevokeTenantAccessTokenMutation(),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await modify.current.mutateAsync({
        ref: "tenant-1",
        input: { projects: ["api"], expires_in_seconds: null },
      })
      await revoke.current.mutateAsync("tenant-1")
    })

    expect(adminApiPatch).toHaveBeenCalledWith(
      "/api/v1/admin/tenant-access-tokens/tenant-1",
      { projects: ["api"], expires_in_seconds: null }
    )
    expect(adminApiDelete).toHaveBeenCalledWith(
      "/api/v1/admin/tenant-access-tokens/tenant-1"
    )
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["admin", "tenant-access-tokens"],
      })
    })
  })
})
