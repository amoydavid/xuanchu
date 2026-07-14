import { useQuery } from "@tanstack/react-query"
import { useNavigate, useSearch } from "@tanstack/react-router"
import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
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
import { canTaskWrite } from "@/features/workspace/project-workbench/permissions/permissions"
import type { ProjectWorkbenchTask } from "@/features/workspace/project-workbench/api/project-api"
import { getProjects } from "@/features/workspace/project-workbench/api/project-api"
import { MyTasksTable } from "@/features/workspace/my-tasks/my-tasks-table"
import {
  MY_TASK_TABS,
  tabFilter,
  type MyTaskTabKey,
} from "@/features/workspace/my-tasks/my-task-tabs"
import {
  myTasksPath,
  type MyTasksFilter,
} from "@/features/workspace/my-tasks/my-tasks-api"
import {
  restoreMyTasksReturnState,
  takeMyTasksReturnState,
} from "@/features/workspace/my-tasks/my-tasks-return-state"

export function MyTasksPage({
  actor,
  actorType,
  canWrite,
  workspaceSlug,
}: {
  actor: MeResponse["actor"] | undefined
  actorType: MeResponse["actor_type"] | undefined
  canWrite: boolean
  workspaceSlug: string | undefined
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const routeSearch = useSearch({ strict: false }) as {
    project?: string
    priority?: string
    q?: string
    sort?: string
    tab?: string
    task_type?: string
  }
  const isSystemActor = actorType === "tenant_access_token"
  const actorId = actor?.id

  const tab = isMyTaskTabKey(routeSearch.tab)
    ? routeSearch.tab
    : "incomplete"
  const priority = ["H", "M", "L"].includes(routeSearch.priority ?? "")
    ? routeSearch.priority!
    : ""
  const project = routeSearch.project ?? ""
  const taskType = ["normal", "occurrence"].includes(
    routeSearch.task_type ?? ""
  )
    ? routeSearch.task_type!
    : ""
  const q = routeSearch.q ?? ""
  const sort = ["due", "priority", "entry", "next"].includes(
    routeSearch.sort ?? ""
  )
    ? routeSearch.sort!
    : "due"
  const listRef = useRef<HTMLDivElement>(null)
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const returnSearch = useMemo(() => {
    const values = {
      project,
      priority,
      q,
      sort,
      tab,
      task_type: taskType,
    }
    return new URLSearchParams(
      Object.entries(values).filter((entry): entry is [string, string] =>
        Boolean(entry[1])
      )
    ).toString()
  }, [priority, project, q, sort, tab, taskType])

  const updateRouteSearch = (
    patch: Partial<
      Record<"priority" | "project" | "q" | "sort" | "tab" | "task_type", string>
    >
  ) => {
    const next = { priority, project, q, sort, tab, task_type: taskType, ...patch }
    void navigate({
      to: "/my-tasks",
      search: Object.fromEntries(
        Object.entries(next).filter(([, value]) => value !== "")
      ),
    })
  }

  // 预设视图统一决定状态与到期范围，避免 toolbar 与 tab 产生冲突条件。
  const filter: MyTasksFilter = useMemo(() => {
    const base: MyTasksFilter = {
      assignee: actorId ?? "",
      project,
      priority,
      q,
      sort,
      task_type: taskType,
    }
    return { ...base, ...tabFilter(tab, new Date()) }
  }, [actorId, priority, project, q, sort, tab, taskType])

  const enabled = !isSystemActor && !!actorId && !!workspaceSlug
  const query = useQuery<ProjectWorkbenchTask[]>({
    enabled,
    queryKey: ["my-tasks", workspaceSlug, filter],
    queryFn: async () => {
      const page = await workspaceApiGet<{ items: ProjectWorkbenchTask[] }>(
        myTasksPath(workspaceSlug!, filter)
      )
      return page.items ?? []
    },
  })
  const projects = useQuery({
    enabled: !!workspaceSlug,
    queryKey: ["my-tasks", workspaceSlug, "projects"],
    queryFn: () => getProjects(workspaceSlug!, "all"),
  })

  useEffect(() => {
    if (!enabled || query.isLoading) return
    const state = takeMyTasksReturnState(returnSearch)
    const visibleIDs = new Set((query.data ?? []).map(myTaskStableID))
    const frame = window.requestAnimationFrame(() => {
      setSelectedIds((current) =>
        (state?.selectedIds ?? current).filter((id) => visibleIDs.has(id))
      )
      if (state && listRef.current) {
        restoreMyTasksReturnState(listRef.current, state)
      }
    })
    return () => window.cancelAnimationFrame(frame)
  }, [enabled, query.data, query.isLoading, returnSearch])

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

      <Tabs
        onValueChange={(value) => updateRouteSearch({ tab: value })}
        value={tab}
      >
        <TabsList
          aria-label={t("myTasks.tabsLabel")}
          className="w-full justify-start overflow-x-auto border-b px-0 pb-1"
          variant="line"
        >
          {MY_TASK_TABS.map((item) => (
            <TabsTrigger
              className="flex-none px-3"
              key={item.key}
              value={item.key}
            >
              {t(`myTasks.tab.${item.key}`)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {/* toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <Input
          aria-label={t("common.search")}
          className="h-8 max-w-xs"
          onChange={(e) => updateRouteSearch({ q: e.target.value })}
          placeholder={t("common.search")}
          value={q}
        />
        <Select
          onValueChange={(value) =>
            updateRouteSearch({ project: value === "any" ? "" : value })
          }
          value={project || "any"}
        >
          <SelectTrigger
            aria-label={t("projectReadonly.project")}
            className="h-8 w-40"
            size="sm"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">{t("myTasks.allProjects")}</SelectItem>
            {(projects.data ?? []).map((item) => (
              <SelectItem key={item.id} value={item.slug}>
                {item.name || item.slug}
              </SelectItem>
            ))}
            {project && !(projects.data ?? []).some((item) => item.slug === project) ? (
              <SelectItem value={project}>{project}</SelectItem>
            ) : null}
          </SelectContent>
        </Select>
        <Select
          onValueChange={(v) =>
            updateRouteSearch({ priority: v === "any" ? "" : v })
          }
          value={priority || "any"}
        >
          <SelectTrigger
            aria-label={t("projectReadonly.priority")}
            className="h-8 w-28"
            size="sm"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">{t("common.filter")}</SelectItem>
            <SelectItem value="H">H</SelectItem>
            <SelectItem value="M">M</SelectItem>
            <SelectItem value="L">L</SelectItem>
          </SelectContent>
        </Select>
        <Select
          onValueChange={(value) =>
            updateRouteSearch({ task_type: value === "all" ? "" : value })
          }
          value={taskType || "all"}
        >
          <SelectTrigger
            aria-label={t("taskCreate.typeLabel")}
            className="h-8 w-32"
            size="sm"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("myTasks.allTaskTypes")}</SelectItem>
            <SelectItem value="normal">{t("taskSeries.mode.normal")}</SelectItem>
            <SelectItem value="occurrence">
              {t("taskSeries.mode.recurring")}
            </SelectItem>
          </SelectContent>
        </Select>
        <Select
          onValueChange={(value) => updateRouteSearch({ sort: value })}
          value={sort}
        >
          <SelectTrigger
            aria-label={t("common.sort")}
            className="h-8 w-28"
            size="sm"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="due">{t("projectReadonly.due")}</SelectItem>
            <SelectItem value="priority">
              {t("projectReadonly.priority")}
            </SelectItem>
            <SelectItem value="entry">
              {t("projectReadonly.identifier")}
            </SelectItem>
            <SelectItem value="next">{t("myTasks.sortNext")}</SelectItem>
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
        <div ref={listRef} tabIndex={-1}>
          <MyTasksTable
            canWrite={canWrite}
            onSortChange={(value) => updateRouteSearch({ sort: value })}
            onSelectedIdsChange={setSelectedIds}
            returnSearch={returnSearch}
            selectedIds={selectedIds}
            sort={sort}
            tasks={query.data ?? []}
            workspaceSlug={workspaceSlug!}
          />
          <MyTasksSummary tasks={query.data ?? []} />
        </div>
      )}
    </div>
  )
}

function myTaskStableID(task: ProjectWorkbenchTask): string {
  return task.id || task.uuid || ""
}

function isMyTaskTabKey(value: string | undefined): value is MyTaskTabKey {
  return MY_TASK_TABS.some((tab) => tab.key === value)
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
  // "进行中"= 已开始（start 非空）且未完成/未删除（模型无 active status，spec §7.2）。
  const active = tasks.filter(
    (task) =>
      task.start != null &&
      task.status !== "completed" &&
      task.status !== "deleted"
  ).length
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
      canWrite={canTaskWrite({
        role: me.data?.effective_role,
        scopes: me.data?.token.scopes,
      })}
      workspaceSlug={me.data?.effective_workspace.slug}
    />
  )
}
