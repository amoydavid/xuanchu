import { useMe } from "@/features/workspace/session/useMe"
import { canTaskWrite } from "@/features/workspace/project-workbench/permissions/permissions"
import { MyTasksPage } from "@/pages/my-tasks-page"

export function MyTasksRoute() {
  const me = useMe()

  return (
    <MyTasksPage
      actor={me.data?.actor}
      actorType={me.data?.actor_type}
      canWrite={canTaskWrite({
        role: me.data?.effective_role,
        scopes: me.data?.token.scopes,
      })}
      workspaceSlug={me.data?.effective_workspace.slug}
    />
  )
}
