import { useMe } from "@/features/workspace/session/useMe"
import { MembersPage } from "@/features/workspace/members/members-page"

export function MembersRoute() {
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug ?? ""

  return <MembersPage workspaceSlug={workspaceSlug} />
}
