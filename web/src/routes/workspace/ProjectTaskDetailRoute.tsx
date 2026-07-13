import { useParams, useSearch } from "@tanstack/react-router"

import { TaskDetailPage } from "@/features/workspace/project-workbench/task-detail/task-detail-page"

export function ProjectTaskDetailRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    taskRef: string
    workspaceSlug: string
  }
  const search = useSearch({ strict: false }) as {
    from?: string
    my_tasks_search?: string
  }

  return (
    <TaskDetailPage
      projectSlug={params.projectSlug}
      myTasksReturnSearch={
        search.from === "my-tasks" ? search.my_tasks_search ?? "" : undefined
      }
      taskRef={params.taskRef}
      workspaceSlug={params.workspaceSlug}
    />
  )
}
