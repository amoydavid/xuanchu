import { useMutation, useQueryClient } from "@tanstack/react-query"

import {
  adminApiDelete,
  adminApiPatch,
} from "@/features/admin/session/admin-api"

import type {
  AdminTenantAccessTokenModifyInput,
  AdminTenantAccessTokenRow,
  AdminTokenModifyInput,
  AdminTokenRow,
} from "./admin-token-api"

const ADMIN_TOKEN_LIST_KEY = ["admin", "tokens"] as const
const ADMIN_TENANT_ACCESS_TOKEN_LIST_KEY = [
  "admin",
  "tenant-access-tokens",
] as const

export function useAdminModifyTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ref, input }: { ref: string; input: AdminTokenModifyInput }) =>
      adminApiPatch<AdminTokenRow>(`/api/v1/admin/tokens/${ref}`, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ADMIN_TOKEN_LIST_KEY })
    },
  })
}

export function useAdminRevokeTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (ref: string) =>
      adminApiDelete<{ ok: boolean }>(`/api/v1/admin/tokens/${ref}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ADMIN_TOKEN_LIST_KEY })
    },
  })
}

export function useAdminModifyTenantAccessTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      ref,
      input,
    }: {
      ref: string
      input: AdminTenantAccessTokenModifyInput
    }) =>
      adminApiPatch<AdminTenantAccessTokenRow>(
        `/api/v1/admin/tenant-access-tokens/${ref}`,
        input
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ADMIN_TENANT_ACCESS_TOKEN_LIST_KEY,
      })
    },
  })
}

export function useAdminRevokeTenantAccessTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (ref: string) =>
      adminApiDelete<{ ok: boolean }>(
        `/api/v1/admin/tenant-access-tokens/${ref}`
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ADMIN_TENANT_ACCESS_TOKEN_LIST_KEY,
      })
    },
  })
}
