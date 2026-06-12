import { useQuery } from "@tanstack/react-query"

import { adminApiGet } from "./admin-api"

export type AdminSession = {
  capabilities: string[]
  token_name: string
}

export function useAdminSession(enabled = true) {
  return useQuery({
    enabled,
    queryKey: ["admin", "session"],
    queryFn: () => adminApiGet<AdminSession>("/api/v1/admin/session"),
    retry: false,
  })
}
