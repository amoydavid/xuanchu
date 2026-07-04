import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

export type WorkspaceMemberRow = {
  user_id: string
  name: string
  display_name: string
  email?: string | null
  role: string
  joined_at: number
  modified_at: number
}

export type WorkspaceUserRow = {
  id: string
  name: string
  display_name: string
  email?: string | null
  external_ids?: { provider: string; external_id: string }[]
  active: boolean
  created_at: number
  modified_at: number
}

export type WorkspaceAuditRow = {
  id: number
  actor?: { id: string; name: string; display_name?: string; email?: string | null } | null
  action: string
  target_type: string
  target_id: string
  created_at: number
}

export type WorkspaceTokenRow = {
  id: string
  name: string
  type: string
  prefix: string
  user: { id: string; name: string; email?: string | null }
  revoked_at?: number | null
}

export function listWorkspaceMembers(workspaceSlug: string) {
  return workspaceApiGet<WorkspaceMemberRow[]>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceSlug)}/members`
  )
}

export function getWorkspaceUser(userRef: string) {
  return workspaceApiGet<WorkspaceUserRow>(
    `/api/v1/users/${encodeURIComponent(userRef)}`
  )
}

export function listWorkspaceAudit(limit = 50) {
  return workspaceApiGet<WorkspaceAuditRow[]>(
    `/api/v1/audit?limit=${encodeURIComponent(String(limit))}`
  )
}

export function listWorkspaceTokens() {
  return workspaceApiGet<WorkspaceTokenRow[]>("/api/v1/tokens")
}

export type AddWorkspaceMemberInput =
  | {
      user: string
      role: string
    }
  | {
      new_user: {
        name: string
        display_name?: string
        email?: string
      }
      role: string
    }

export type ModifyWorkspaceMemberInput = {
  role?: string
  display_name?: string
}

export function addWorkspaceMember(
  workspaceSlug: string,
  input: AddWorkspaceMemberInput
) {
  return workspaceApiPost<{ ok: boolean }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceSlug)}/members`,
    input
  )
}

export function modifyWorkspaceMember(
  workspaceSlug: string,
  userID: string,
  input: ModifyWorkspaceMemberInput
) {
  return workspaceApiPatch<WorkspaceMemberRow>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceSlug)}/members/${encodeURIComponent(userID)}`,
    input
  )
}

export function removeWorkspaceMember(workspaceSlug: string, userID: string) {
  return workspaceApiDelete<{ ok: boolean }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceSlug)}/members/${encodeURIComponent(userID)}`
  )
}
