import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  workspaceApiGet,
  workspaceApiPost,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"

export interface SsoConfigResponse {
  enabled: boolean
  config?: SsoConfig
}

export interface SsoConfig {
  provider: string
  issuer_base_url: string
  org_id: string
  client_id: string
  client_secret_masked: string
  scopes: string
  redirect_path: string
  external_base_url: string
  session_ttl: string
  sync_interval: string
  insecure_cookie: boolean
}

export interface SsoConfigInput {
  issuer_base_url: string
  org_id: string
  client_id: string
  client_secret: string
  sync_interval: string
  external_base_url: string
  session_ttl: string
}

export interface SyncJobResponse {
  job_id: string
  status: string
}

const SSO_CONFIG_KEY = ["workspace", "sso", "config"] as const

export function useSsoConfigQuery(workspaceSlug: string) {
  return useQuery({
    queryKey: SSO_CONFIG_KEY,
    queryFn: () =>
      workspaceApiGet<SsoConfigResponse>(`/api/v1/workspaces/${workspaceSlug}/sso/config`),
  })
}

export function useSaveSsoConfigMutation(workspaceSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SsoConfigInput) =>
      workspaceApiPut<SsoConfigResponse>(
        `/api/v1/workspaces/${workspaceSlug}/sso/config`,
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: SSO_CONFIG_KEY })
    },
  })
}

export function useTriggerSyncMutation(workspaceSlug: string) {
  return useMutation({
    mutationFn: () =>
      workspaceApiPost<SyncJobResponse>(
        `/api/v1/workspaces/${workspaceSlug}/sso/sync`,
        {},
      ),
  })
}
