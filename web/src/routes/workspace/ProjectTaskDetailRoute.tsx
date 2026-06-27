import { useParams } from "@tanstack/react-router"

import { TaskDetailPage } from "@/features/workspace/project-workbench/task-detail/task-detail-page"

export function ProjectTaskDetailRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    taskRef: string
    workspaceSlug: string
  }

  return (
    <TaskDetailPage
      projectSlug={params.projectSlug}
      taskRef={params.taskRef}
      workspaceSlug={params.workspaceSlug}
    />
  )
}
