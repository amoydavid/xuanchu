import { useQuery } from "@tanstack/react-query"

import { workspaceApiGet } from "./workspace-api"

export type MeResponse = {
  actor_type: "user" | "tenant_access_token"
  actor: { name: string; display_name?: string }
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
