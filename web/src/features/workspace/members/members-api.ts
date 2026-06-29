import {
  workspaceApiGet,
  workspaceApiPatch,
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

export function listWorkspaceMembers(workspaceSlug: string) {
  return workspaceApiGet<WorkspaceMemberRow[]>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceSlug)}/members`
  )
}

export function modifyWorkspaceUserDisplayName(
  userID: string,
  displayName: string
) {
  return workspaceApiPatch<WorkspaceUserRow>(
    `/api/v1/users/${encodeURIComponent(userID)}`,
    { display_name: displayName }
  )
}
