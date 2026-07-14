import { Outlet, useParams } from "@tanstack/react-router"

import { ProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"

// /workspaces/$workspaceSlug/projects/$projectSlug/series
// /workspaces/$workspaceSlug/projects/$projectSlug/series/$seriesRef
//
// 循环任务 tab（独立一级 tab）的持久父路由：
// - ProjectTabs 高亮「循环任务」
// - list/detail 子路由通过 Outlet 共存，切换 seriesRef 时不重挂载 ProjectLayout
export function ProjectSeriesRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectLayout
      activeTab="series"
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    >
      <Outlet />
    </ProjectLayout>
  )
}
