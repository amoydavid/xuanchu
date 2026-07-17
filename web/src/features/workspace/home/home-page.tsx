import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import {
  AlertTriangle,
  ArrowRight,
  Check,
  CircleDot,
  Clock3,
  FolderKanban,
  Plus,
  RefreshCw,
  Square,
  Play,
} from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import type { ConfigEffectiveValue } from "@/features/workspace/config/config-definition-api"
import { listWorkspaceEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import { formatConfigDisplayValue } from "@/features/workspace/config/config-display"
import {
  getProjects,
  type ProjectWorkbenchProject,
  type ProjectWorkbenchTask,
} from "@/features/workspace/project-workbench/api/project-api"
import { useTaskActionMutation } from "@/features/workspace/project-workbench/hooks/use-task-mutations"
import {
  canConfigManage,
  canTaskWrite,
  hasScope,
} from "@/features/workspace/project-workbench/permissions/permissions"
import { EditFeedbackProvider } from "@/features/workspace/project-workbench/shared/edit-feedback"
import { TaskCreateDialog } from "@/features/workspace/project-workbench/tasks/task-create-dialog"
import {
  taskDisplayRef,
  taskRouteRef,
} from "@/features/workspace/project-workbench/tasks/task-reference"
import type { MeResponse } from "@/features/workspace/session/useMe"
import { cn } from "@/lib/utils"
import {
  getHome,
  type HomeMyWork,
  type HomeProjectAttention,
  type HomeTaskItem,
  type HomeTaskReason,
} from "./home-api"
import {
  restoreHomeReturnState,
  saveHomeReturnState,
  takeHomeReturnState,
} from "./home-return-state"

type HomePageProps = {
  me?: MeResponse
}

export function HomePage({ me }: HomePageProps) {
  return (
    <EditFeedbackProvider>
      <HomePageContent me={me} />
    </EditFeedbackProvider>
  )
}

function HomePageContent({ me }: HomePageProps) {
  const { i18n } = useTranslation()
  const queryClient = useQueryClient()
  const workspaceSlug = me?.effective_workspace.slug ?? ""
  const actorID = me?.actor.id ?? ""
  const permissionInput = {
    role: me?.effective_role,
    scopes: me?.token.scopes,
  }
  const canWriteTasks = canTaskWrite(permissionInput)
  const canReadConfig = hasScope(me?.token.scopes, "config:read")
  const canManageConfig = canConfigManage(permissionInput)
  const pageRef = useRef<HTMLElement>(null)
  const [projectPickerOpen, setProjectPickerOpen] = useState(false)
  const [selectedProject, setSelectedProject] =
    useState<ProjectWorkbenchProject | null>(null)
  const [createdTask, setCreatedTask] = useState<{
    projectSlug: string
    task: ProjectWorkbenchTask
  } | null>(null)

  const home = useQuery({
    enabled: Boolean(workspaceSlug && actorID),
    queryKey: ["home", workspaceSlug, actorID],
    queryFn: getHome,
    staleTime: 30_000,
    refetchOnWindowFocus: true,
  })
  const workspaceInfo = useQuery({
    enabled: Boolean(workspaceSlug && actorID && canReadConfig),
    queryKey: [
      "home",
      workspaceSlug,
      actorID,
      "config-effective",
      "console-home",
    ],
    queryFn: () => listWorkspaceEffectiveConfig({ consoleHome: true }),
    staleTime: 30_000,
  })
  const writableProjects = useQuery({
    enabled: Boolean(workspaceSlug && canWriteTasks),
    queryKey: ["home", workspaceSlug, actorID, "writable-projects"],
    queryFn: () => getProjects(workspaceSlug, "open"),
    staleTime: 30_000,
  })

  useEffect(() => {
    if (!home.data || !pageRef.current || !workspaceSlug || !actorID) return
    const state = takeHomeReturnState(workspaceSlug, actorID)
    if (!state) return
    const frame = window.requestAnimationFrame(() => {
      if (pageRef.current) restoreHomeReturnState(pageRef.current, state)
    })
    return () => window.cancelAnimationFrame(frame)
  }, [actorID, home.data, workspaceSlug])

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["home", workspaceSlug] })
  }
  const projects = writableProjects.data ?? []
  const showCreate = canWriteTasks && projects.length > 0
  const displayName = me?.actor.display_name || me?.actor.name || ""
  const workspaceName =
    me?.effective_workspace.name || me?.effective_workspace.slug || ""

  return (
    <main
      className="mx-auto max-w-7xl space-y-6 outline-none"
      ref={pageRef}
      tabIndex={-1}
    >
      <HomeHeader
        displayName={displayName}
        onCreate={showCreate ? () => setProjectPickerOpen(true) : undefined}
        onRefresh={refresh}
        today={home.data?.today}
        workspaceName={workspaceName}
      />

      {createdTask ? (
        <CreatedTaskNotice
          onDismiss={() => setCreatedTask(null)}
          projectSlug={createdTask.projectSlug}
          task={createdTask.task}
          workspaceSlug={workspaceSlug}
        />
      ) : null}

      {home.isPending ? (
        <HomeSkeleton />
      ) : home.isError ? (
        <HomeError error={home.error} onRetry={() => void home.refetch()} />
      ) : home.data ? (
        <>
          {home.data.actor_type === "tenant_access_token" ? (
            <SystemIdentityPanel me={me} />
          ) : home.data.my_work ? (
            <MyTodaySection
              actorID={actorID}
              canWrite={canWriteTasks}
              data={home.data.my_work}
              locale={i18n.language}
              onCreate={
                showCreate ? () => setProjectPickerOpen(true) : undefined
              }
              workspaceSlug={workspaceSlug}
            />
          ) : null}

          <div
            className={cn(
              "grid gap-6",
              canReadConfig &&
                (workspaceInfo.isPending ||
                  workspaceInfo.isError ||
                  (workspaceInfo.data?.length ?? 0) > 0)
                ? "lg:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]"
                : "grid-cols-1"
            )}
          >
            {home.data.project_attention.length > 0 ? (
              <ProjectAttentionSection
                items={home.data.project_attention}
                locale={i18n.language}
                today={home.data.today}
                workspaceSlug={workspaceSlug}
              />
            ) : null}
            {canReadConfig ? (
              <WorkspaceInfoSection
                canManage={canManageConfig}
                error={workspaceInfo.error}
                isError={workspaceInfo.isError}
                isPending={workspaceInfo.isPending}
                onRetry={() => void workspaceInfo.refetch()}
                rows={workspaceInfo.data ?? []}
              />
            ) : null}
          </div>
        </>
      ) : null}

      <ProjectPickerDialog
        isPending={writableProjects.isPending}
        onOpenChange={setProjectPickerOpen}
        onSelect={(project) => {
          setProjectPickerOpen(false)
          setSelectedProject(project)
        }}
        open={projectPickerOpen}
        projects={projects}
      />
      {selectedProject ? (
        <TaskCreateDialog
          onCreated={(task) => {
            setCreatedTask({ projectSlug: selectedProject.slug, task })
            void queryClient.invalidateQueries({
              queryKey: ["home", workspaceSlug],
            })
          }}
          onOpenChange={(open) => {
            if (!open) setSelectedProject(null)
          }}
          open={true}
          projectSlug={selectedProject.slug}
          workspaceSlug={workspaceSlug}
        />
      ) : null}
    </main>
  )
}

function HomeHeader({
  displayName,
  onCreate,
  onRefresh,
  today,
  workspaceName,
}: {
  displayName: string
  onCreate?: () => void
  onRefresh: () => void
  today?: string
  workspaceName: string
}) {
  const { i18n, t } = useTranslation()
  return (
    <header className="flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <div className="mb-2 flex items-center gap-2 text-xs font-medium tracking-[0.18em] text-muted-foreground uppercase">
          <CircleDot className="size-3 text-emerald-600" />
          {t("home.title")}
        </div>
        <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
          {t("home.greeting", { name: displayName })}
        </h1>
        <p className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
          <span>{workspaceName}</span>
          {today ? <span aria-hidden="true">·</span> : null}
          {today ? <span>{formatToday(today, i18n.language)}</span> : null}
        </p>
      </div>
      <div className="flex items-center gap-2">
        {onCreate ? (
          <Button onClick={onCreate}>
            <Plus />
            {t("home.create.action")}
          </Button>
        ) : null}
        <Button
          aria-label={t("home.refresh")}
          onClick={onRefresh}
          size="icon"
          variant="outline"
        >
          <RefreshCw />
        </Button>
      </div>
    </header>
  )
}

function MyTodaySection({
  actorID,
  canWrite,
  data,
  locale,
  onCreate,
  workspaceSlug,
}: {
  actorID: string
  canWrite: boolean
  data: HomeMyWork
  locale: string
  onCreate?: () => void
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const counts = [
    {
      count: data.started_count,
      href: "/my-tasks?tab=started",
      key: "started" as const,
    },
    {
      count: data.overdue_count,
      href: "/my-tasks?tab=overdue",
      key: "overdue" as const,
    },
    {
      count: data.due_today_count,
      href: "/my-tasks?tab=today",
      key: "dueToday" as const,
    },
    {
      count: data.high_priority_open_count,
      href: "/my-tasks?priority=H&tab=incomplete",
      key: "highPriority" as const,
    },
  ]

  return (
    <section aria-labelledby="home-my-today" className="space-y-4">
      <SectionHeading
        action={
          <Link
            className="group inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
            to="/my-tasks"
          >
            {t("home.myToday.viewAll")}
            <ArrowRight className="size-3 transition-transform group-hover:translate-x-0.5" />
          </Link>
        }
        icon={<Clock3 className="size-4" />}
        id="home-my-today"
        title={t("home.myToday.title")}
      />
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border bg-border lg:grid-cols-4">
        {counts.map((item) => (
          <a
            aria-label={`${t(`home.myToday.${item.key}`)} ${item.count}`}
            className="group bg-card px-4 py-3 transition-colors outline-none hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
            href={item.href}
            key={item.key}
          >
            <div className="text-xs text-muted-foreground">
              {t(`home.myToday.${item.key}`)}
            </div>
            <div className="mt-1 flex items-end justify-between">
              <span className="text-2xl font-semibold tabular-nums">
                {item.count}
              </span>
              <ArrowRight className="size-3 text-muted-foreground opacity-0 transition-all group-hover:translate-x-0.5 group-hover:opacity-100" />
            </div>
          </a>
        ))}
      </div>

      {data.items.length === 0 ? (
        <div className="border bg-card px-5 py-8 text-center">
          <p className="font-medium">{t("home.myToday.emptyTitle")}</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("home.myToday.emptyDescription")}
          </p>
          <div className="mt-4 flex justify-center gap-2">
            <Button asChild variant="outline">
              <Link to="/my-tasks">{t("home.myToday.viewMine")}</Link>
            </Button>
            {onCreate ? (
              <Button onClick={onCreate}>{t("home.create.action")}</Button>
            ) : null}
          </div>
        </div>
      ) : (
        <div className="divide-y overflow-hidden rounded-xl border bg-card">
          {data.items.map((item) => (
            <HomeTaskRow
              actorID={actorID}
              canWrite={canWrite}
              item={item}
              key={taskStableID(item.task)}
              locale={locale}
              workspaceSlug={workspaceSlug}
            />
          ))}
          {data.open_count > data.items.length ? (
            <div className="flex items-center justify-between bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
              <span>
                {t("home.myToday.more", {
                  count: data.open_count - data.items.length,
                })}
              </span>
              <Link
                className="font-medium text-foreground hover:underline"
                to="/my-tasks"
              >
                {t("home.myToday.enter")}
              </Link>
            </div>
          ) : null}
        </div>
      )}
    </section>
  )
}

function HomeTaskRow({
  actorID,
  canWrite,
  item,
  locale,
  workspaceSlug,
}: {
  actorID: string
  canWrite: boolean
  item: HomeTaskItem
  locale: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const [error, setError] = useState<string | null>(null)
  const task = item.task
  const projectSlug = typeof task.project === "string" ? task.project : ""
  const displayRef = taskDisplayRef(task, locale) || taskStableID(task)
  const taskRef = taskRouteRef(task) || taskStableID(task)
  const start = useTaskActionMutation(workspaceSlug, projectSlug, "start")
  const stop = useTaskActionMutation(workspaceSlug, projectSlug, "stop")
  const done = useTaskActionMutation(workspaceSlug, projectSlug, "done")
  const isStarted = task.start !== null && task.start !== undefined
  const isPending = start.isPending || stop.isPending || done.isPending

  const run = async (mutation: typeof start | typeof stop | typeof done) => {
    setError(null)
    try {
      await mutation.mutateAsync(taskRef)
    } catch (mutationError) {
      setError(
        mutationError instanceof Error
          ? mutationError.message
          : t("common.error")
      )
    }
  }
  const saveReturnState = () => {
    saveHomeReturnState(workspaceSlug, actorID, {
      focusId: taskStableID(task),
      scrollTop: window.scrollY,
    })
  }

  return (
    <article className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
      <Link
        className="min-w-0 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        data-home-task-focus={taskStableID(task)}
        onClick={saveReturnState}
        params={{ workspaceSlug, projectSlug, taskRef }}
        search={{ from: "home" }}
        to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
      >
        <div className="flex min-w-0 items-start gap-3">
          <span
            aria-hidden="true"
            className={cn(
              "mt-1.5 size-2 shrink-0 rounded-full",
              item.reasons.includes("overdue")
                ? "bg-destructive"
                : item.reasons.includes("started")
                  ? "bg-emerald-600"
                  : "bg-amber-500"
            )}
          />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
              <span className="text-xs font-medium text-muted-foreground">
                {displayRef}
              </span>
              <span className="leading-snug font-medium">{task.title}</span>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              {projectSlug ? <span>{projectSlug}</span> : null}
              {item.reasons.map((reason) => (
                <Badge key={reason} variant="outline">
                  {reasonLabel(reason, t)}
                </Badge>
              ))}
              {task.priority ? <span>{task.priority}</span> : null}
              {task.due ? <span>{formatDue(task.due, locale)}</span> : null}
            </div>
          </div>
        </div>
      </Link>
      <div className="flex items-center justify-end gap-1">
        {canWrite && task.status === "pending" ? (
          isStarted ? (
            <Button
              aria-label={t("projectWorkbench.project.stopTask", {
                taskRef: displayRef,
              })}
              className="hidden sm:inline-flex"
              disabled={isPending}
              onClick={() => void run(stop)}
              size="icon-sm"
              variant="ghost"
            >
              <Square />
            </Button>
          ) : (
            <Button
              aria-label={t("projectWorkbench.project.startTask", {
                taskRef: displayRef,
              })}
              className="hidden sm:inline-flex"
              disabled={isPending}
              onClick={() => void run(start)}
              size="icon-sm"
              variant="ghost"
            >
              <Play />
            </Button>
          )
        ) : null}
        {canWrite && ["pending", "waiting"].includes(task.status) ? (
          <Button
            aria-label={t("projectWorkbench.project.completeTask", {
              taskRef: displayRef,
            })}
            disabled={isPending}
            onClick={() => void run(done)}
            size="sm"
            variant="outline"
          >
            <Check />
            <span className="hidden sm:inline">{t("common.done")}</span>
          </Button>
        ) : null}
      </div>
      {error ? (
        <p className="text-xs text-destructive sm:col-span-2" role="alert">
          {error}
        </p>
      ) : null}
    </article>
  )
}

function ProjectAttentionSection({
  items,
  locale,
  today,
  workspaceSlug,
}: {
  items: HomeProjectAttention[]
  locale: string
  today: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  return (
    <section
      aria-labelledby="home-project-attention"
      className="min-w-0 space-y-3"
    >
      <SectionHeading
        action={
          <Link
            className="group inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
            to="/projects"
          >
            {t("home.projects.viewAll")}
            <ArrowRight className="size-3 transition-transform group-hover:translate-x-0.5" />
          </Link>
        }
        icon={<FolderKanban className="size-4" />}
        id="home-project-attention"
        title={t("home.projects.title")}
      />
      <div className="space-y-3">
        {items.map((item) => (
          <ProjectAttentionCard
            item={item}
            key={item.project.id}
            locale={locale}
            today={today}
            workspaceSlug={workspaceSlug}
          />
        ))}
      </div>
    </section>
  )
}

function ProjectAttentionCard({
  item,
  locale,
  today,
  workspaceSlug,
}: {
  item: HomeProjectAttention
  locale: string
  today: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const project = item.project
  const completion =
    project.task_count > 0
      ? Math.round((project.completed_count / project.task_count) * 100)
      : 0
  const metrics = projectMetrics(item, workspaceSlug, today, t)
  const actor = item.latest_update?.created_by.user
  const actorName = actor?.display_name || actor?.name

  return (
    <article className="rounded-xl border bg-card p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <Link
            className="font-medium hover:underline"
            params={{ workspaceSlug, projectSlug: project.slug }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug"
          >
            {project.name || project.slug}
          </Link>
          <div className="mt-1 text-xs text-muted-foreground">
            {t("home.projects.completion", { percent: completion })}
          </div>
        </div>
        <div className="w-24 pt-1">
          <div className="h-1.5 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-foreground transition-[width]"
              style={{ width: `${completion}%` }}
            />
          </div>
        </div>
      </div>
      {metrics.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {metrics.map((metric) => (
            <a
              className="rounded-full border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              href={metric.href}
              key={metric.key}
            >
              {metric.label}
            </a>
          ))}
        </div>
      ) : (
        <p className="mt-3 text-xs text-muted-foreground">
          {t("home.projects.noRisk")}
        </p>
      )}
      {item.series_metrics.overdue_recurring_occurrence_count > 0 ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {t("home.projects.recurringOverdue", {
            count: item.series_metrics.overdue_recurring_occurrence_count,
          })}
        </p>
      ) : null}
      <div className="mt-3 border-t pt-3 text-sm text-muted-foreground">
        {item.latest_update ? (
          <>
            <p className="line-clamp-2 text-foreground/80">
              {item.latest_update.content}
            </p>
            <p className="mt-1 text-xs">
              {[
                actorName,
                formatTimestamp(item.latest_update.created_at, locale),
              ]
                .filter(Boolean)
                .join(" · ")}
            </p>
          </>
        ) : (
          <p className="text-xs">
            {t("home.projects.updated", {
              time: formatTimestamp(project.modified_at, locale),
            })}
          </p>
        )}
      </div>
    </article>
  )
}

function WorkspaceInfoSection({
  canManage,
  error,
  isError,
  isPending,
  onRetry,
  rows,
}: {
  canManage: boolean
  error: Error | null
  isError: boolean
  isPending: boolean
  onRetry: () => void
  rows: ConfigEffectiveValue[]
}) {
  const { t } = useTranslation()
  if (!isPending && !isError && rows.length === 0) return null
  return (
    <section
      aria-labelledby="home-workspace-info"
      className="min-w-0 space-y-3"
    >
      <SectionHeading
        action={
          canManage ? (
            <Link
              className="text-sm text-muted-foreground hover:text-foreground"
              to="/settings"
            >
              {t("home.workspaceInfo.manage")}
            </Link>
          ) : null
        }
        id="home-workspace-info"
        title={t("home.workspaceInfo.title")}
      />
      {isPending ? (
        <div
          aria-busy="true"
          className="space-y-2 rounded-xl border bg-card p-4"
        >
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-4/5" />
        </div>
      ) : isError ? (
        <div className="rounded-xl border bg-card p-4 text-sm">
          <p className="text-muted-foreground">
            {error?.message || t("common.error")}
          </p>
          <Button
            className="mt-3"
            onClick={onRetry}
            size="sm"
            variant="outline"
          >
            {t("common.retry")}
          </Button>
        </div>
      ) : (
        <div className="divide-y overflow-hidden rounded-xl border bg-card">
          {rows.map((row) => {
            const value =
              row.value === null
                ? t("configDefinitions.sourceMissing")
                : formatConfigDisplayValue(row.definition.value_type, row.value)
            return (
              <div className="p-4" key={row.key}>
                <div className="text-sm font-medium">
                  {row.definition.label || row.key}
                </div>
                <div className="mt-1 text-sm break-words text-foreground/80">
                  {value}
                </div>
                <code className="mt-1 block text-[11px] break-all text-muted-foreground">
                  {row.key}
                </code>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

function SystemIdentityPanel({ me }: { me?: MeResponse }) {
  const { t } = useTranslation()
  const scopes = me?.token.scopes
  const links = [
    {
      href: "/projects",
      label: t("home.system.projects"),
      scope: "project:read",
    },
    { href: "/members", label: t("home.system.members"), scope: "member:read" },
    { href: "/hooks", label: t("home.system.hooks"), scope: "hook:read" },
    {
      href: "/notifications",
      label: t("home.system.notifications"),
      scope: "notification:read",
    },
    { href: "/audit", label: t("home.system.audit"), scope: "audit:read" },
    {
      href: "/settings",
      label: t("home.system.settings"),
      scope: "config:read",
    },
  ].filter((item) => hasScope(scopes, item.scope))
  return (
    <section className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-5">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600" />
        <div>
          <h2 className="font-semibold">{t("home.system.title")}</h2>
          <p className="mt-1 text-sm text-foreground/80">
            {t("home.system.noPersonalWork")}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("home.system.switchHint")}
          </p>
          {links.length > 0 ? (
            <div className="mt-4 flex flex-wrap gap-2">
              {links.map((link) => (
                <Button asChild key={link.href} size="sm" variant="outline">
                  <Link to={link.href}>{link.label}</Link>
                </Button>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </section>
  )
}

function ProjectPickerDialog({
  isPending,
  onOpenChange,
  onSelect,
  open,
  projects,
}: {
  isPending: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (project: ProjectWorkbenchProject) => void
  open: boolean
  projects: ProjectWorkbenchProject[]
}) {
  const { t } = useTranslation()
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("home.create.chooseProject")}</DialogTitle>
          <DialogDescription>
            {t("home.create.chooseProjectDescription")}
          </DialogDescription>
        </DialogHeader>
        {isPending ? (
          <div aria-busy="true" className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : (
          <div className="grid gap-2">
            {projects.map((project) => (
              <Button
                aria-label={project.name || project.slug}
                className="h-auto justify-start px-3 py-2 text-left"
                key={project.id}
                onClick={() => onSelect(project)}
                variant="outline"
              >
                <span>
                  <span className="block font-medium">
                    {project.name || project.slug}
                  </span>
                  <span className="block text-xs font-normal text-muted-foreground">
                    {project.slug}
                  </span>
                </span>
              </Button>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function CreatedTaskNotice({
  onDismiss,
  projectSlug,
  task,
  workspaceSlug,
}: {
  onDismiss: () => void
  projectSlug: string
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const taskRef = taskRouteRef(task) || taskStableID(task)
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-emerald-500/30 bg-emerald-500/5 px-4 py-3 text-sm"
      role="status"
    >
      <span>{t("home.create.created", { title: task.title })}</span>
      <div className="flex items-center gap-2">
        <Button asChild size="sm" variant="outline">
          <Link
            params={{ workspaceSlug, projectSlug, taskRef }}
            search={{ from: "home" }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
          >
            {t("home.create.openTask")}
          </Link>
        </Button>
        <Button onClick={onDismiss} size="sm" variant="ghost">
          {t("common.close")}
        </Button>
      </div>
    </div>
  )
}

function HomeSkeleton() {
  return (
    <div aria-busy="true" className="space-y-6">
      <section className="space-y-3">
        <Skeleton className="h-6 w-32" />
        <div className="grid grid-cols-2 gap-1 lg:grid-cols-4">
          {[0, 1, 2, 3].map((item) => (
            <Skeleton className="h-20" key={item} />
          ))}
        </div>
        <Skeleton className="h-44 w-full" />
      </section>
      <Skeleton className="h-56 w-full" />
    </div>
  )
}

function HomeError({ error, onRetry }: { error: Error; onRetry: () => void }) {
  const { t } = useTranslation()
  return (
    <section className="rounded-xl border bg-card p-6">
      <h2 className="font-semibold">{t("home.error.title")}</h2>
      <p className="mt-2 text-sm text-muted-foreground">{error.message}</p>
      <Button className="mt-4" onClick={onRetry} variant="outline">
        {t("common.retry")}
      </Button>
    </section>
  )
}

function SectionHeading({
  action,
  icon,
  id,
  title,
}: {
  action?: React.ReactNode
  icon?: React.ReactNode
  id: string
  title: string
}) {
  return (
    <div className="flex min-h-8 items-center justify-between gap-4">
      <h2 className="flex items-center gap-2 text-base font-semibold" id={id}>
        {icon}
        {title}
      </h2>
      {action}
    </div>
  )
}

function reasonLabel(
  reason: HomeTaskReason,
  t: (key: string) => string
): string {
  return t(`home.reason.${reason}`)
}

function taskStableID(task: ProjectWorkbenchTask): string {
  return String(task.id || task.uuid || task.task_slug || task.title)
}

function projectMetrics(
  item: HomeProjectAttention,
  workspaceSlug: string,
  today: string,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  const openQuery = "(status:pending or status:waiting)"
  const base = `/workspaces/${encodeURIComponent(workspaceSlug)}/projects/${encodeURIComponent(item.project.slug)}/tasks`
  const rows: Array<{
    count: number
    key: string
    params: Record<string, string>
  }> = [
    {
      count: item.overdue_count,
      key: "overdue",
      params: {
        due_before: previousDate(today),
        query: openQuery,
      },
    },
    {
      count: item.high_priority_open_count,
      key: "highPriority",
      params: { priority: "H", query: openQuery },
    },
    {
      count: item.wait_ready_count,
      key: "waitReady",
      params: { query: openQuery, wait_before: today },
    },
    {
      count: item.unassigned_open_count,
      key: "unassigned",
      params: { assignee_empty: "true", query: openQuery },
    },
  ]
  return rows
    .filter((row) => row.count > 0)
    .map((row) => {
      const query = new URLSearchParams(row.params)
      return {
        href: `${base}?${query.toString()}`,
        key: row.key,
        label: t(`home.projects.metric.${row.key}`, { count: row.count }),
      }
    })
}

function previousDate(today: string): string {
  const date = new Date(`${today}T12:00:00`)
  date.setDate(date.getDate() - 1)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

function formatToday(today: string, locale: string): string {
  const date = new Date(`${today}T12:00:00`)
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "long",
  }).format(date)
}

function formatDue(value: string | number, locale: string): string {
  const numeric = typeof value === "number" ? value : Number(value)
  const date = Number.isFinite(numeric)
    ? new Date(numeric * 1000)
    : new Date(String(value))
  if (Number.isNaN(date.getTime())) return String(value)
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date)
}

function formatTimestamp(value: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
  }).format(new Date(value * 1000))
}
