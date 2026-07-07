import { useParams } from "@tanstack/react-router"

import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { ProjectSettingsLayout } from "@/pages/project-settings-layout"

export function ProjectSettingsLayoutRoute() {
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
    <ProjectSettingsLayout
      projectSlug={params.projectSlug}
      workspaceSlug={workspaceSlug}
    />
  )
}
