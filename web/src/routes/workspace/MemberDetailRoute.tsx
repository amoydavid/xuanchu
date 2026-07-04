import { useParams } from "@tanstack/react-router"

import { MembersDetailPage } from "@/features/workspace/members/members-page"
import { useMe } from "@/features/workspace/session/useMe"

export function MemberDetailRoute() {
  const params = useParams({ strict: false }) as { userRef: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug ?? ""

  return (
    <MembersDetailPage
      credential={me.data}
      userRef={params.userRef}
      workspaceSlug={workspaceSlug}
    />
  )
}
