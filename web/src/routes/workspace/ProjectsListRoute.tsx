import { useMe } from "@/features/workspace/session/useMe"

import { ProjectsListPage } from "@/features/workspace/projects/projects-list-page"

export function ProjectsListRoute() {
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug ?? ""

  return <ProjectsListPage workspaceSlug={workspaceSlug} />
}
