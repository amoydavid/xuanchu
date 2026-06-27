import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

export type WorkspaceMemberCandidate = {
  user_id: string
  name: string
  email?: string | null
  role: string
  joined_at: number
  modified_at: number
}

export function workspaceMembersPath(workspaceSlug: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceSlug)}/members`
}

export function getWorkspaceMembers(
  workspaceSlug: string
): Promise<WorkspaceMemberCandidate[]> {
  return workspaceApiGet<WorkspaceMemberCandidate[]>(
    workspaceMembersPath(workspaceSlug)
  )
}
