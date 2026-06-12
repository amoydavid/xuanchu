import { useParams } from "@tanstack/react-router"

import { ProjectReadonlyPage } from "@/features/workspace/project-readonly/project-page"

export function ProjectReadonlyRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectReadonlyPage
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    />
  )
}
