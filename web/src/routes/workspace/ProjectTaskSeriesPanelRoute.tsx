import { useEffect } from "react"
import { useParams } from "@tanstack/react-router"

import { TaskSeriesPanelShell } from "@/features/workspace/project-workbench/task-series/task-series-panel-shell"

// /workspaces/$workspaceSlug/projects/$projectSlug/tasks/series
// /workspaces/$workspaceSlug/projects/$projectSlug/tasks/series/$seriesRef
//
// 循环任务管理面板子路由（spec §15.12）：
// - 只注册/注销面板，不重挂载 ProjectTasksPage
// - list/detail 切换通过 seriesRef search param
// - ProjectTabs 始终高亮"任务"
export function ProjectTaskSeriesPanelRoute() {
  const params = useParams({ strict: false }) as {
    workspaceSlug: string
    projectSlug: string
  }
  // 从 path 提取 seriesRef（若在 detail 路由）。
  const seriesRef = (useParams({ strict: false }) as { seriesRef?: string }).seriesRef

  return (
    <TaskSeriesPanelShell
      workspaceSlug={params.workspaceSlug}
      projectSlug={params.projectSlug}
      seriesRef={seriesRef}
    />
  )
}

// useEffect 占位避免未使用警告（实际面板内部用 effect 注册）。
void useEffect
