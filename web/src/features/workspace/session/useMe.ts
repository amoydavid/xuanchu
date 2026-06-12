import { useQuery } from "@tanstack/react-query"

import { workspaceApiGet } from "./workspace-api"

export type MeResponse = {
  actor: { name: string }
  token: { type: string; scopes: string[] }
  effective_workspace: { slug: string }
}

export function useMe(enabled = true) {
  return useQuery({
    enabled,
    queryKey: ["me"],
    queryFn: () => workspaceApiGet<MeResponse>("/api/v1/me"),
    retry: false,
  })
}
