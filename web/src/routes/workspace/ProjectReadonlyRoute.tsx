import { useParams } from "@tanstack/react-router"

import { ProjectWorkbenchPage } from "@/features/workspace/project-workbench/project/project-workbench-page"

export function ProjectReadonlyRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectWorkbenchPage
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    />
  )
}
