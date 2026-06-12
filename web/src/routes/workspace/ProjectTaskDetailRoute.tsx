import { useParams } from "@tanstack/react-router"

import { ProjectTaskDetailPage } from "@/features/workspace/project-readonly/project-task-detail-page"

export function ProjectTaskDetailRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    taskRef: string
    workspaceSlug: string
  }

  return (
    <ProjectTaskDetailPage
      projectSlug={params.projectSlug}
      taskRef={params.taskRef}
      workspaceSlug={params.workspaceSlug}
    />
  )
}
