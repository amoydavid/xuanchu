import { Navigate, useParams } from "@tanstack/react-router"

import { useMe } from "@/features/workspace/session/useMe"

// /projects/$projectSlug/settings/notes
// 项目 notes/annotations 是项目事实流，不再是设置项。
// 旧入口重定向到 Activity 子页面，保持兼容。
export function ProjectSettingsNotesRoute() {
  const params = useParams({ strict: false }) as { projectSlug: string }
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug
  if (!workspaceSlug || !me.data) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        Loading…
      </section>
    )
  }
  return (
    <Navigate
      params={{ projectSlug: params.projectSlug, workspaceSlug }}
      replace
      to="/workspaces/$workspaceSlug/projects/$projectSlug/activity"
    />
  )
}
