import { Outlet, useParams } from "@tanstack/react-router"

import { ProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { ProjectTasksPage } from "@/features/workspace/project-workbench/tasks/project-tasks-page"

// /workspaces/$workspaceSlug/projects/$projectSlug/tasks
// 项目任务页：承接任务筛选、简单任务列表、新建、导入、行内编辑。
//
// 作为持久父路由（spec §15.12）：series 子路由通过 Outlet 共存，
// 任务页不因 series 面板打开/关闭/切换而重挂载。
export function ProjectTasksRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectLayout
      activeTab="tasks"
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    >
      <ProjectTasksPage
        projectSlug={params.projectSlug}
        workspaceSlug={params.workspaceSlug}
      />
      {/* series 子路由的 panel 在此 Outlet 渲染，不替换 ProjectTasksPage */}
      <Outlet />
    </ProjectLayout>
  )
}
