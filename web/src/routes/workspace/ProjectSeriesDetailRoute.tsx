import { useParams } from "@tanstack/react-router"

import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { TaskSeriesPage } from "@/features/workspace/project-workbench/task-series/task-series-page"

// /workspaces/$workspaceSlug/projects/$projectSlug/series/$seriesRef
// 循环任务详情。seriesRef 由路由 param 提供。
export function ProjectSeriesDetailRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
    seriesRef: string
  }
  const layout = useProjectLayout()
  const canManage = !layout.closed && layout.canWriteTasks

  return (
    <TaskSeriesPage
      workspaceSlug={params.workspaceSlug}
      projectSlug={params.projectSlug}
      seriesRef={params.seriesRef}
      canManage={canManage}
    />
  )
}
