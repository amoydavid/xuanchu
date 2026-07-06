import { useParams } from "@tanstack/react-router"

import { useProjectQuery } from "@/features/workspace/project-workbench/hooks/use-project-data"
import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { ProjectConfigTab } from "@/pages/project-config-tab"
import { isClosedProjectStatus } from "@/features/workspace/project-workbench/project/project-status-menu"
import { canProjectManage } from "@/features/workspace/project-workbench/permissions/permissions"

export function ProjectSettingsConfigRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug
  const project = useProjectQuery(workspaceSlug ?? "", params.projectSlug)

  if (!workspaceSlug || !me.data) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }
  if (project.isError) {
    return (
      <section className="border bg-card p-6 text-sm text-destructive">
        {project.error instanceof ApiError ? project.error.message : "error"}
      </section>
    )
  }
  if (project.isPending) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }

  return (
    <ProjectConfigTab
      canManage={canProjectManage({
        role: me.data.effective_role,
        scopes: me.data.token.scopes,
      })}
      closed={isClosedProjectStatus(project.data.status)}
      projectSlug={params.projectSlug}
      workspaceSlug={workspaceSlug}
    />
  )
}
