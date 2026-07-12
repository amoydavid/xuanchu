import { useQuery } from "@tanstack/react-query"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { ApiError } from "@/lib/api"
import { useMe } from "@/features/workspace/session/useMe"
import type { MeResponse } from "@/features/workspace/session/useMe"
import type { ProjectWorkbenchTask } from "@/features/workspace/project-workbench/api/project-api"
import { MyTasksTable } from "@/features/workspace/my-tasks/my-tasks-table"
import { MY_TASK_TABS, tabFilter, type MyTaskTabKey } from "@/features/workspace/my-tasks/my-task-tabs"
import { myTasksPath, type MyTasksFilter } from "@/features/workspace/my-tasks/my-tasks-api"

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
  const isSystemActor = actorType === "tenant_access_token"
  const actorId = actor?.id

  const [tab, setTab] = useState<MyTaskTabKey>("incomplete")
  const [status, setStatus] = useState<string>("pending")
  const [priority, setPriority] = useState<string>("")
  const [q, setQ] = useState<string>("")
  const [sort, setSort] = useState<string>("due")

  // tab 切换会覆盖 status/due 等字段；用户在 toolbar 中的二次选择仍可继续修改。
  const filter: MyTasksFilter = useMemo(() => {
    const base: MyTasksFilter = {
      assignee: actorId ?? "",
      status,
      priority,
      q,
      sort,
    }
    return { ...base, ...tabFilter(tab, new Date()) }
    // 注意：tabFilter 会覆盖 status 为 pending（与各 tab 语义一致）
  }, [actorId, status, priority, q, sort, tab])

  const enabled = !isSystemActor && !!actorId && !!workspaceSlug
  const query = useQuery<ProjectWorkbenchTask[]>({
    enabled,
    queryKey: ["my-tasks", workspaceSlug, filter],
    queryFn: () => workspaceApiGet<ProjectWorkbenchTask[]>(myTasksPath(workspaceSlug!, filter)),
  })

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

      {/* tab 条 */}
      <div className="flex flex-wrap gap-1 border-b">
        {MY_TASK_TABS.map((item) => (
          <button
            aria-pressed={tab === item.key}
            className={
              "border-b-2 px-3 py-2 text-sm transition-colors " +
              (tab === item.key
                ? "border-foreground font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground")
            }
            key={item.key}
            onClick={() => setTab(item.key)}
            type="button"
          >
            {t(`myTasks.tab.${item.key}`)}
          </button>
        ))}
      </div>

      {/* toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <Input
          aria-label={t("common.search")}
          className="h-8 max-w-xs"
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("common.search")}
          value={q}
        />
        <Select onValueChange={setStatus} value={status}>
          <SelectTrigger aria-label={t("common.status")} className="h-8 w-28" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="pending">{t("projectReadonly.pending")}</SelectItem>
            <SelectItem value="active">{t("projectReadonly.active")}</SelectItem>
            <SelectItem value="completed">{t("projectReadonly.completed")}</SelectItem>
            <SelectItem value="deleted">{t("projectReadonly.statusDeleted")}</SelectItem>
          </SelectContent>
        </Select>
        <Select onValueChange={(v) => setPriority(v === "any" ? "" : v)} value={priority || "any"}>
          <SelectTrigger aria-label={t("projectReadonly.priority")} className="h-8 w-28" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">{t("common.filter")}</SelectItem>
            <SelectItem value="H">H</SelectItem>
            <SelectItem value="M">M</SelectItem>
            <SelectItem value="L">L</SelectItem>
          </SelectContent>
        </Select>
        <Select onValueChange={setSort} value={sort}>
          <SelectTrigger aria-label={t("common.sort")} className="h-8 w-28" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="due">{t("projectReadonly.due")}</SelectItem>
            <SelectItem value="priority">{t("projectReadonly.priority")}</SelectItem>
            <SelectItem value="entry">{t("projectReadonly.identifier")}</SelectItem>
            <SelectItem value="next">next</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {!enabled ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("myTasks.systemActorEmpty")}
        </div>
      ) : query.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {query.error instanceof ApiError
            ? query.error.message
            : t("common.error")}
        </div>
      ) : query.isLoading ? (
        <div className="border bg-card p-6 text-sm text-muted-foreground">
          {t("myTasks.loading")}
        </div>
      ) : (
        <>
          <MyTasksTable
            onSortChange={setSort}
            sort={sort}
            tasks={query.data ?? []}
            workspaceSlug={workspaceSlug!}
          />
          <MyTasksSummary tasks={query.data ?? []} />
        </>
      )}
    </div>
  )
}

function MyTasksSummary({ tasks }: { tasks: ProjectWorkbenchTask[] }) {
  const { t } = useTranslation()
  // 取当前时间一次，存到 state，避免在 render 中调用 impure 的 Date.now()。
  const [nowSnapshot] = useState(() => Date.now())
  if (tasks.length === 0) return null
  const now = Math.floor(nowSnapshot / 1000)
  const overdue = tasks.filter(
    (task) =>
      task.status === "pending" &&
      typeof task.due === "number" &&
      task.due > 0 &&
      task.due < now
  ).length
  const today = tasks.filter((task) => {
    if (task.status !== "pending") return false
    if (typeof task.due !== "number" || task.due <= 0) return false
    const dueDate = new Date(task.due * 1000)
    const todayDate = new Date(nowSnapshot)
    return (
      dueDate.getFullYear() === todayDate.getFullYear() &&
      dueDate.getMonth() === todayDate.getMonth() &&
      dueDate.getDate() === todayDate.getDate()
    )
  }).length
  const active = tasks.filter((task) => task.status === "active").length
  return (
    <div className="px-1 text-xs text-muted-foreground">
      {t("myTasks.summary", { overdue, today, active })}
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
