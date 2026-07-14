import { useParams } from "@tanstack/react-router"

import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { TaskSeriesPage } from "@/features/workspace/project-workbench/task-series/task-series-page"

// /workspaces/$workspaceSlug/projects/$projectSlug/series
// 循环任务列表。canManage 来自 ProjectLayout context（写权限 + 非关闭项目）。
export function ProjectSeriesListRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }
  const layout = useProjectLayout()
  const canManage = !layout.closed && layout.canWriteTasks

  return (
    <TaskSeriesPage
      workspaceSlug={params.workspaceSlug}
      projectSlug={params.projectSlug}
      canManage={canManage}
    />
  )
}
