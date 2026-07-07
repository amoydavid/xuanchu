import { ProjectLayout } from "./project-layout"
import { ProjectOverviewPage } from "./project-overview-page"

type ProjectWorkbenchPageProps = {
  projectSlug: string
  workspaceSlug: string
}

// ProjectWorkbenchPage 是项目根路由的 Overview 包装器。
// 历史上是单页混合工作台，已拆为 概览 / 任务 / 活动 三个子页面。
// 保留导出名以兼容既有引用；实际渲染由 ProjectLayout + ProjectOverviewPage 承担。
export function ProjectWorkbenchPage({
  projectSlug,
  workspaceSlug,
}: ProjectWorkbenchPageProps) {
  return (
    <ProjectLayout
      activeTab="overview"
      projectSlug={projectSlug}
      workspaceSlug={workspaceSlug}
    >
      <ProjectOverviewPage
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />
    </ProjectLayout>
  )
}
