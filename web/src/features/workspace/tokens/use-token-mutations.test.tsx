import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  workspaceApiDelete,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

import {
  useCreateTenantAccessTokenMutation,
  useModifyTenantAccessTokenMutation,
  useRevokeTenantAccessTokenMutation,
} from "./use-token-mutations"

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiDelete: vi.fn(),
  workspaceApiPatch: vi.fn(),
  workspaceApiPost: vi.fn(),
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

describe("tenant access token workspace mutations", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workspaceApiPost).mockResolvedValue({ id: "tenant-1" })
    vi.mocked(workspaceApiPatch).mockResolvedValue({ id: "tenant-1" })
    vi.mocked(workspaceApiDelete).mockResolvedValue({ ok: true })
  })

  it("creates tenant access tokens through the tenant endpoint", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(() => useCreateTenantAccessTokenMutation(), {
      wrapper: makeWrapper(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({
        name: "runtime",
        scopes: ["task:read"],
        projects: ["api"],
        expires_in_seconds: null,
      })
    })

    expect(workspaceApiPost).toHaveBeenCalledWith(
      "/api/v1/tenant-access-tokens",
      {
        name: "runtime",
        scopes: ["task:read"],
        projects: ["api"],
        expires_in_seconds: null,
      }
    )
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["resource", "/api/v1/tenant-access-tokens"],
      })
    })
  })

  it("modifies and revokes tenant access tokens through tenant endpoints", async () => {
    const queryClient = makeQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const { result: modify } = renderHook(
      () => useModifyTenantAccessTokenMutation(),
      { wrapper: makeWrapper(queryClient) }
    )
    const { result: revoke } = renderHook(
      () => useRevokeTenantAccessTokenMutation(),
      { wrapper: makeWrapper(queryClient) }
    )

    await act(async () => {
      await modify.current.mutateAsync({
        ref: "tenant-1",
        input: { projects: ["api"], expires_in_seconds: null },
      })
      await revoke.current.mutateAsync("tenant-1")
    })

    expect(workspaceApiPatch).toHaveBeenCalledWith(
      "/api/v1/tenant-access-tokens/tenant-1",
      { projects: ["api"], expires_in_seconds: null }
    )
    expect(workspaceApiDelete).toHaveBeenCalledWith(
      "/api/v1/tenant-access-tokens/tenant-1"
    )
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["resource", "/api/v1/tenant-access-tokens"],
      })
    })
  })
})
