import { useParams } from "@tanstack/react-router"

import { ProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { ProjectOverviewPage } from "@/features/workspace/project-workbench/project/project-overview-page"

// /workspaces/$workspaceSlug/projects/$projectSlug
// 项目概览页（Overview）：回答「项目现在怎么样，是否需要介入」。
// 历史命名 ProjectReadonlyRoute 是旧产品边界遗留；现在默认显示概览，
// 任务执行移到 /tasks，活动记录移到 /activity。
export function ProjectWorkbenchRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectLayout
      activeTab="overview"
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    >
      <ProjectOverviewPage
        projectSlug={params.projectSlug}
        workspaceSlug={params.workspaceSlug}
      />
    </ProjectLayout>
  )
}
