import { useMe } from "@/features/workspace/session/useMe"
import { MyTasksPage } from "@/pages/my-tasks-page"

export function MyTasksRoute() {
  const me = useMe()

  return (
    <MyTasksPage
      actor={me.data?.actor}
      actorType={me.data?.actor_type}
      workspaceSlug={me.data?.effective_workspace.slug}
    />
  )
}
