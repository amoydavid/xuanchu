import { useEffect } from "react"
import { useNavigate, useParams, useSearch } from "@tanstack/react-router"

import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"
import { TaskSeriesPanelShell } from "@/features/workspace/project-workbench/task-series/task-series-panel-shell"

// /workspaces/$workspaceSlug/projects/$projectSlug/tasks/series
// /workspaces/$workspaceSlug/projects/$projectSlug/tasks/series/$seriesRef
//
// 循环任务管理面板子路由（spec §15.12）：
// - 通过 setContextPanel 注册到 ProjectLayout 右栏槽位（替换 ProjectContextRail）
// - list/detail 切换通过 seriesRef path param
// - ProjectTabs 始终高亮"任务"
// - 关闭面板时恢复原右栏状态
export function ProjectTaskSeriesPanelRoute() {
  const params = useParams({ strict: false }) as {
    workspaceSlug: string
    projectSlug: string
    seriesRef?: string
  }
  const layout = useProjectLayout()
  const navigate = useNavigate()
  const search = useSearch({ strict: false }) as {
    panel_return_scope?: string
    panel_return_search?: string
    panel_return_source?: string
    panel_return_task?: string
  }
  const canManageSeries = !layout.closed && layout.canWriteTasks

  // 注册/注销面板到右栏。
  useEffect(() => {
    layout.setContextPanel({
      node: (
        <TaskSeriesPanelShell
          workspaceSlug={params.workspaceSlug}
          projectSlug={params.projectSlug}
          seriesRef={params.seriesRef}
          canManage={canManageSeries}
        />
      ),
      onClose: () => {
        if (search.panel_return_task && search.panel_return_scope === "project") {
          void navigate({
            to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef",
            params: {
              workspaceSlug: params.workspaceSlug,
              projectSlug: params.projectSlug,
              taskRef: search.panel_return_task,
            },
            ...(search.panel_return_source === "my-tasks"
              ? {
                  search: {
                    from: "my-tasks",
                    my_tasks_search: search.panel_return_search ?? "",
                  },
                }
              : {}),
          })
          return
        }
        if (search.panel_return_task && search.panel_return_scope === "global") {
          void navigate({
            to: "/tasks/$taskRef",
            params: { taskRef: search.panel_return_task },
          })
          return
        }
        void navigate({
          to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks",
          params: {
            workspaceSlug: params.workspaceSlug,
            projectSlug: params.projectSlug,
          },
          search: (previous) =>
            Object.fromEntries(
              Object.entries(previous).filter(
                (entry): entry is [string, string] =>
                  typeof entry[1] === "string" &&
                  !entry[0].startsWith("panel_return_")
              )
            ),
        })
      },
    })
    return () => {
      layout.setContextPanel(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- layout.setContextPanel 是稳定回调
  }, [
    params.workspaceSlug,
    params.projectSlug,
    params.seriesRef,
    canManageSeries,
    search.panel_return_scope,
    search.panel_return_search,
    search.panel_return_source,
    search.panel_return_task,
  ])

  // 面板内容通过 setContextPanel 渲染到右栏，此组件本身不输出 DOM。
  return null
}
