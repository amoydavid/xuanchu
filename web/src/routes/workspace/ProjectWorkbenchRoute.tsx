import { useParams } from "@tanstack/react-router"

import { ProjectWorkbenchPage } from "@/features/workspace/project-workbench/project/project-workbench-page"

// /workspaces/$workspaceSlug/projects/$projectSlug
// 项目工作台页（可编辑）。历史命名 ProjectReadonlyRoute 是旧产品边界遗留，
// 现在项目详情已从只读浏览迁到可编辑 workbench，路由名随之对齐。
export function ProjectWorkbenchRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectWorkbenchPage
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    />
  )
}
