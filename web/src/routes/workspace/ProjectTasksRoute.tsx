import { useParams } from "@tanstack/react-router"

import { ProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { ProjectTasksPage } from "@/features/workspace/project-workbench/tasks/project-tasks-page"

// /workspaces/$workspaceSlug/projects/$projectSlug/tasks
// 项目任务页：承接任务筛选、简单任务列表、新建、导入、行内编辑。
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
    </ProjectLayout>
  )
}
