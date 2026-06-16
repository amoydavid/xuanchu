import { useMutation, useQueryClient } from "@tanstack/react-query"

import {
  adminApiDelete,
  adminApiPatch,
} from "@/features/admin/session/admin-api"

import type {
  AdminTokenModifyInput,
  AdminTokenRow,
} from "./admin-token-api"

const ADMIN_TOKEN_LIST_KEY = ["admin", "tokens"] as const

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
