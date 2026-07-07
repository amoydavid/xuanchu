import { useParams } from "@tanstack/react-router"

import { ProjectActivityPage } from "@/features/workspace/project-workbench/activity/project-activity-page"
import { ProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"

// /workspaces/$workspaceSlug/projects/$projectSlug/activity
// 项目活动页：项目更新输入框 + 时间线 + 可选审计。
export function ProjectActivityRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectLayout
      activeTab="activity"
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    >
      <ProjectActivityPage
        projectSlug={params.projectSlug}
        workspaceSlug={params.workspaceSlug}
      />
    </ProjectLayout>
  )
}
