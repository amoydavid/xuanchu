import { useParams } from "@tanstack/react-router"

import { useMe } from "@/features/workspace/session/useMe"
import { TaskDetailPage } from "@/features/workspace/project-workbench/task-detail/task-detail-page"
import { ApiError } from "@/lib/api"

export function TaskDetailRoute() {
  const params = useParams({ strict: false }) as { taskRef: string }
  const me = useMe()

  // workspace 来自当前 effective workspace（全局任务入口不绑定具体 workspace 路径）。
  if (me.isError) {
    return (
      <section className="rounded-lg border bg-card p-6 text-sm text-destructive">
        {me.error instanceof ApiError ? me.error.code : "unknown"}
      </section>
    )
  }

  const workspaceSlug = me.data?.effective_workspace.slug
  if (!workspaceSlug || !me.data) {
    return (
      <section className="rounded-lg border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }

  // 不传 projectSlug：TaskDetailPage 会从 task.project 字段兜底，
  // 适用于「我的任务」/ `/tasks/:ref` 等无项目上下文的入口。
  return (
    <TaskDetailPage taskRef={params.taskRef} workspaceSlug={workspaceSlug} />
  )
}
