import { useQuery } from "@tanstack/react-query"

import { workspaceApiGet } from "./workspace-api"

export type ExternalID = { provider: string; external_id: string }

export type MeResponse = {
  actor_type: "user" | "tenant_access_token"
  actor: {
    id: string
    name: string
    display_name?: string
    email?: string | null
    external_ids?: ExternalID[]
  }
  token: { type: string; scopes: string[] | null }
  effective_workspace: { slug: string; name?: string }
  effective_role: string
  capabilities?: string[]
}

export function useMe(enabled = true) {
  return useQuery({
    enabled,
    queryKey: ["credentials", "current"],
    queryFn: () => workspaceApiGet<MeResponse>("/api/v1/credentials/current"),
    retry: false,
  })
}
