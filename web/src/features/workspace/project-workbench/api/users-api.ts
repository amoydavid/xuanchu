import {
  workspaceApiGet,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

export type WorkspaceMemberCandidate = {
  user_id: string
  name: string
  email?: string | null
  role: string
  joined_at: number
  modified_at: number
}

export type WorkspaceUserCandidate = {
  id: string
  name: string
  email?: string | null
  external_ids?: Array<{ provider: string; external_id: string }>
  active: boolean
  created_at: number
  modified_at: number
}

export type WorkspaceUserCreateInput = {
  name: string
  email?: string | null
}

export type WorkspaceMemberAddResult = {
  ok: boolean
}

export function userListPath(): string {
  return "/api/v1/users"
}

export function workspaceMembersPath(workspaceSlug: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceSlug)}/members`
}

export function workspaceMemberAddPath(workspaceSlug: string): string {
  return workspaceMembersPath(workspaceSlug)
}

export function listWorkspaceUsers(): Promise<WorkspaceUserCandidate[]> {
  return workspaceApiGet<WorkspaceUserCandidate[]>(userListPath())
}

export function createWorkspaceUser(
  input: WorkspaceUserCreateInput
): Promise<WorkspaceUserCandidate> {
  return workspaceApiPost<WorkspaceUserCandidate>(userListPath(), input)
}

export function getWorkspaceMembers(
  workspaceSlug: string
): Promise<WorkspaceMemberCandidate[]> {
  return workspaceApiGet<WorkspaceMemberCandidate[]>(
    workspaceMembersPath(workspaceSlug)
  )
}

export function addWorkspaceMember(
  workspaceSlug: string,
  userRef: string,
  role: string
): Promise<WorkspaceMemberAddResult> {
  return workspaceApiPost<WorkspaceMemberAddResult>(
    workspaceMemberAddPath(workspaceSlug),
    { role, user: userRef }
  )
}
