import { useParams } from "@tanstack/react-router"

import { ProjectAutomationsPage } from "@/features/workspace/project-workbench/automations/project-automations-page"
import { ProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"

// /workspaces/$workspaceSlug/projects/$projectSlug/automations
// 项目自动化页：规则列表、编辑表单、投递 JSON 预览和运行记录。
export function ProjectAutomationsRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectLayout
      activeTab="automations"
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    >
      <ProjectAutomationsPage
        projectSlug={params.projectSlug}
        workspaceSlug={params.workspaceSlug}
      />
    </ProjectLayout>
  )
}
