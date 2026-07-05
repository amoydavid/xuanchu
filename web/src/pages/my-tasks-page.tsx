import { useTranslation } from "react-i18next"

import { useMe } from "@/features/workspace/session/useMe"
import type { MeResponse } from "@/features/workspace/session/useMe"

export function MyTasksPage({
  actor,
  actorType,
  workspaceSlug,
}: {
  actor: MeResponse["actor"] | undefined
  actorType: MeResponse["actor_type"] | undefined
  workspaceSlug: string | undefined
}) {
  const { t } = useTranslation()
  // tenant actor 没有自然人「我的任务」语义：显示空状态和解释，不伪造 assignee。
  const isSystemActor = actorType === "tenant_access_token"
  const actorId = actor?.id

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-normal">
            {t("nav.myTasks")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("myTasks.subtitle")}
          </p>
        </div>
      </div>
      {isSystemActor || !actorId ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("myTasks.systemActorEmpty")}
        </div>
      ) : (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("myTasks.loading")}
        </div>
      )}
    </div>
  )
}

// 兼容直接通过 useMe 读取的调用方（无需外部传参）
export function MyTasksPageConnected() {
  const me = useMe()
  return (
    <MyTasksPage
      actor={me.data?.actor}
      actorType={me.data?.actor_type}
      workspaceSlug={me.data?.effective_workspace.slug}
    />
  )
}
