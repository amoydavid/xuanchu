import { useParams, useSearch } from "@tanstack/react-router"

import { TaskDetailPage } from "@/features/workspace/project-workbench/task-detail/task-detail-page"
import { useProjectQuery } from "@/features/workspace/project-workbench/hooks/use-project-data"
import { isClosedProjectStatus } from "@/features/workspace/project-workbench/project/project-status-menu"

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
  const project = useProjectQuery(params.workspaceSlug, params.projectSlug)
  // 项目状态尚未确认或读取失败时先保持只读，避免关闭项目详情短暂出现写入口。
  const projectClosed =
    !project.data || isClosedProjectStatus(project.data.status)

  return (
    <TaskDetailPage
      projectClosed={projectClosed}
      returnToHome={search.from === "home"}
      projectSlug={params.projectSlug}
      myTasksReturnSearch={
        search.from === "my-tasks" ? (search.my_tasks_search ?? "") : undefined
      }
      taskRef={params.taskRef}
      workspaceSlug={params.workspaceSlug}
    />
  )
}
