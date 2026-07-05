import { useParams } from "@tanstack/react-router"

import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { ProjectSettingsPage } from "@/pages/project-settings-page"

export function ProjectSettingsRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug

  if (me.isError) {
    return (
      <section className="border bg-card p-6 text-sm text-destructive">
        {me.error instanceof ApiError ? me.error.code : "unknown"}
      </section>
    )
  }
  if (!workspaceSlug || !me.data) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }

  return (
    <ProjectSettingsPage
      canManage={
        me.data.effective_role === "owner" || me.data.effective_role === "admin"
      }
      projectSlug={params.projectSlug}
      workspaceSlug={workspaceSlug}
    />
  )
}
