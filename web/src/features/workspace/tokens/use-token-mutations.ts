import { useMutation, useQueryClient } from "@tanstack/react-query"

import {
  workspaceApiDelete,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

import type {
  CreatedTokenRow,
  CreatedTenantAccessTokenRow,
  TenantAccessTokenCreateInput,
  TenantAccessTokenModifyInput,
  TenantAccessTokenRow,
  TokenCreateInput,
  TokenModifyInput,
  TokenRow,
} from "./token-api"

// 列表查询的 queryKey，与 ResourcePage / TokensPage 保持一致以复用缓存。
const TOKEN_LIST_KEY = ["resource", "/api/v1/tokens"] as const
const TENANT_ACCESS_TOKEN_LIST_KEY = [
  "resource",
  "/api/v1/tenant-access-tokens",
] as const

export function useCreateTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TokenCreateInput) =>
      workspaceApiPost<CreatedTokenRow>("/api/v1/tokens", input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: TOKEN_LIST_KEY })
    },
  })
}

export function useModifyTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ref, input }: { ref: string; input: TokenModifyInput }) =>
      workspaceApiPatch<TokenRow>(`/api/v1/tokens/${ref}`, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: TOKEN_LIST_KEY })
    },
  })
}

export function useRevokeTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (ref: string) =>
      workspaceApiDelete<{ ok: boolean }>(`/api/v1/tokens/${ref}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: TOKEN_LIST_KEY })
    },
  })
}

export function useCreateTenantAccessTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TenantAccessTokenCreateInput) =>
      workspaceApiPost<CreatedTenantAccessTokenRow>(
        "/api/v1/tenant-access-tokens",
        input
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: TENANT_ACCESS_TOKEN_LIST_KEY,
      })
    },
  })
}

export function useModifyTenantAccessTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      ref,
      input,
    }: {
      ref: string
      input: TenantAccessTokenModifyInput
    }) =>
      workspaceApiPatch<TenantAccessTokenRow>(
        `/api/v1/tenant-access-tokens/${ref}`,
        input
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: TENANT_ACCESS_TOKEN_LIST_KEY,
      })
    },
  })
}

export function useRevokeTenantAccessTokenMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (ref: string) =>
      workspaceApiDelete<{ ok: boolean }>(
        `/api/v1/tenant-access-tokens/${ref}`
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: TENANT_ACCESS_TOKEN_LIST_KEY,
      })
    },
  })
}
