import { adminApiGet, adminApiPost } from "@/features/admin/session/admin-api"

export type AdminStatus = {
  enabled: boolean
  setup_code_expires_at?: number
  setup_required: boolean
  status: "disabled" | "login_required" | "setup_required"
}

export type AdminSetupResult = {
  token: string
  token_name: string
  token_prefix: string
}

export function getAdminStatus(): Promise<AdminStatus> {
  return adminApiGet<AdminStatus>("/api/v1/admin/status")
}

export function completeAdminSetup(input: {
  name?: string
  setup_code: string
}): Promise<AdminSetupResult> {
  return adminApiPost<AdminSetupResult>("/api/v1/admin/setup", input)
}
