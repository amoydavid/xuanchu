// Admin workspace 管理 API 类型，对齐后端 admin_workspace handler 响应。
// UserInfo 使用统一 JSON 形态（id/name/email/external_ids），不输出裸 UUID。

import {
  adminApiGet,
  adminApiPatch,
  adminApiPost,
} from "@/features/admin/session/admin-api"

export type AdminWorkspaceSummary = {
  id: string
  slug: string
  name: string
  description?: string
  visibility: string
  created_by?: AdminUserInfo | null
  member_counts: {
    owner: number
    admin: number
    member: number
    viewer: number
  }
  token_counts: {
    active: number
    expired: number
    revoked: number
  }
  archived_at?: number | null
  created_at: number
  modified_at: number
}

export type AdminUserInfo = {
  id: string
  name: string
  display_name?: string
  email?: string | null
  external_ids?: { provider: string; external_id: string }[]
}

export type AdminWorkspaceMember = {
  user: AdminUserInfo
  role: string
  joined_at: number
  modified_at: number
}

export type AdminActingCandidate = {
  user: AdminUserInfo
  role: string
}

export type AdminWorkspaceDetail = {
  workspace: {
    id: string
    slug: string
    name: string
    description?: string
    visibility: string
    archived_at?: number | null
  }
  members: AdminWorkspaceMember[]
  token_counts: {
    active: number
    expired: number
    revoked: number
  }
  acting_candidates: AdminActingCandidate[]
}

export type CreatedAdminActingSession = {
  token: string
  expires_at: number
  workspace: {
    id: string
    slug: string
    name: string
  }
  actor: AdminUserInfo
  role: string
  admin_token_name: string
}

export type CreatedAdminTenantAccessSession = {
  token: string
  id: string
  prefix: string
  name: string
  type: "tenant_access_token"
  workspace_id: string
  project_ids: string[] | null
  scopes: string[] | null
  created_at: number
  expires_at?: number | null
  revoked_at?: number | null
  last_used_at?: number | null
  issued_via?: string
  issued_by_admin_token?: { id?: string; name?: string } | null
  purpose?: string
  workspace: {
    id: string
    slug: string
    name: string
  }
}

export function listAdminWorkspaces(all: boolean) {
  return adminApiGet<AdminWorkspaceSummary[]>(
    `/api/v1/admin/workspaces?all=${all ? "true" : "false"}`
  )
}

export function fetchAdminWorkspaceDetail(workspace: string) {
  return adminApiGet<AdminWorkspaceDetail>(
    `/api/v1/admin/workspaces/${encodeURIComponent(workspace)}`
  )
}

export type AdminWorkspaceUserModifyInput = {
  display_name?: string
}

export function modifyAdminWorkspaceUser(
  workspace: string,
  user: string,
  input: AdminWorkspaceUserModifyInput
) {
  return adminApiPatch<AdminUserInfo>(
    `/api/v1/admin/workspaces/${encodeURIComponent(workspace)}/users/${encodeURIComponent(user)}`,
    input
  )
}

export type CreateActingSessionInput = {
  user?: string
  expires_in?: string
}

export function createAdminActingSession(
  workspace: string,
  input: CreateActingSessionInput = {}
) {
  return adminApiPost<CreatedAdminActingSession>(
    `/api/v1/admin/workspaces/${encodeURIComponent(workspace)}/acting-sessions`,
    input
  )
}

export function createAdminTenantAccessSession(workspace: string) {
  return adminApiPost<CreatedAdminTenantAccessSession>(
    `/api/v1/admin/workspaces/${encodeURIComponent(workspace)}/tenant-access-sessions`,
    {}
  )
}
