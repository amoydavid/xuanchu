import { useSearch } from "@tanstack/react-router"

import { ProjectsListPage } from "@/features/workspace/project-workbench/projects/projects-list-page"
import { useMe } from "@/features/workspace/session/useMe"

export function ClosedProjectsRoute() {
  const me = useMe()
  const search = useSearch({ strict: false }) as {
    status?: "archived" | "cancelled"
  }
  const workspaceSlug = me.data?.effective_workspace.slug ?? ""

  return (
    <ProjectsListPage
      closedStatus={search.status ?? "archived"}
      view="closed"
      workspaceSlug={workspaceSlug}
    />
  )
}
