import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useSearch } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"
import { navigateToDocument } from "@/lib/browser-navigation"
import {
  getProjectReadonlyProject,
  getProjectReadonlyTasks,
  getProjectReadonlyTimeline,
} from "./project-readonly-api"
import { ProjectActivity } from "./project-activity"
import { ProjectFilterToolbar } from "./project-filter-toolbar"
import { filterToTaskQuery, type TaskFilter } from "./project-filter"
import { ProjectSummary } from "./project-summary"
import { ProjectTaskList } from "./project-task-list"
import { buildProjectStats, summarizeAssignees } from "./project-stats"

type ProjectReadonlyPageProps = {
  projectSlug: string
  workspaceSlug: string
}

export function ProjectReadonlyPage({
  projectSlug,
  workspaceSlug,
}: ProjectReadonlyPageProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const [now] = useState(() => Math.floor(Date.now() / 1000))
  // filter 来自 URL search（validateSearch 已限定为字符串字段）。
  const search = useSearch({ strict: false }) as Partial<TaskFilter>
  const filter: TaskFilter = useMemo(
    () => ({
      status: typeof search.status === "string" ? search.status : undefined,
      priority: typeof search.priority === "string" ? search.priority : undefined,
      assignee: typeof search.assignee === "string" ? search.assignee : undefined,
      due_after:
        typeof search.due_after === "string" ? search.due_after : undefined,
      due_before:
        typeof search.due_before === "string" ? search.due_before : undefined,
      tags: typeof search.tags === "string" ? search.tags : undefined,
      q: typeof search.q === "string" ? search.q : undefined,
    }),
    [
      search.assignee,
      search.due_after,
      search.due_before,
      search.priority,
      search.q,
      search.status,
      search.tags,
    ]
  )
  const filterQuery = useMemo(() => filterToTaskQuery(filter), [filter])

  const project = useQuery({
    queryKey: ["project-readonly", workspaceSlug, projectSlug, "project"],
    queryFn: () => getProjectReadonlyProject(workspaceSlug, projectSlug),
  })
  const tasks = useQuery({
    queryKey: ["project-readonly", workspaceSlug, projectSlug, "tasks", filterQuery],
    queryFn: () => getProjectReadonlyTasks(workspaceSlug, projectSlug, filterQuery),
  })
  const timeline = useQuery({
    queryKey: ["project-readonly", workspaceSlug, projectSlug, "timeline"],
    queryFn: () => getProjectReadonlyTimeline(workspaceSlug, projectSlug),
  })
  const taskRows = useMemo(() => tasks.data ?? [], [tasks.data])
  const stats = useMemo(() => buildProjectStats(taskRows, now), [now, taskRows])
  const assignees = useMemo(
    () => summarizeAssignees(taskRows, now, t("projectReadonly.unassigned")),
    [now, taskRows, t]
  )

  const accessError = project.error ?? tasks.error
  if (isPermissionError(accessError)) {
    return (
      <ProjectState
        actionLabel={t("projectReadonly.backToOverview")}
        description={t("projectReadonly.permissionDescription", {
          project: `${workspaceSlug} / ${projectSlug}`,
        })}
        detail="project:read, task:read"
        title={t("projectReadonly.permissionTitle")}
      />
    )
  }
  if (isNotFoundError(project.error)) {
    return (
      <ProjectState
        actionLabel={t("projectReadonly.backToOverview")}
        description={t("projectReadonly.notFoundDescription")}
        title={t("projectReadonly.notFoundTitle")}
      />
    )
  }

  if (project.isPending) {
    return <ProjectSkeleton />
  }

  if (project.isError || tasks.isError) {
    const code = accessError instanceof ApiError ? accessError.code : "unknown"
    return (
      <ProjectState
        actionLabel={t("common.refresh")}
        description={code}
        onAction={() => window.location.reload()}
        title={t("common.error")}
      />
    )
  }

  return (
    <div className="space-y-5">
      <ProjectSummary
        assignees={assignees}
        copiedLabel={t("projectReadonly.copied")}
        copyLabel={
          copied ? t("projectReadonly.copied") : t("projectReadonly.copyLink")
        }
        onCopy={() => {
          void navigator.clipboard?.writeText(window.location.href)
          setCopied(true)
        }}
        project={project.data}
        readonlyLabel={t("projectReadonly.readonly")}
        stats={stats}
        t={t}
        workspaceSlug={workspaceSlug}
      />
      <ProjectFilterToolbar
        filter={filter}
        toParams={{ workspaceSlug, projectSlug }}
      />
      <ProjectTaskList
        emptyExample={`xuanchu --workspace ${workspaceSlug} add "Design API" project:${projectSlug}`}
        emptyTitle={t("projectReadonly.emptyTitle")}
        projectSlug={projectSlug}
        tasks={taskRows}
        t={t}
        workspaceSlug={workspaceSlug}
      />
      <ProjectActivity
        entries={timeline.isError ? [] : timeline.data}
        title={t("projectReadonly.recentActivity")}
      />
    </div>
  )
}

function ProjectState({
  actionLabel,
  description,
  detail,
  onAction,
  title,
}: {
  actionLabel: string
  description: string
  detail?: string
  onAction?: () => void
  title: string
}) {
  return (
    <section className="max-w-2xl border bg-card p-6">
      <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
      <p className="mt-3 text-sm text-muted-foreground">{description}</p>
      {detail ? <code className="mt-4 block text-xs">{detail}</code> : null}
      <Button
        className="mt-5"
        onClick={onAction ?? (() => navigateToDocument("/"))}
        variant="outline"
      >
        {actionLabel}
      </Button>
    </section>
  )
}

function ProjectSkeleton() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-10 w-72" />
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        {Array.from({ length: 5 }).map((_, index) => (
          <Skeleton className="h-20" key={index} />
        ))}
      </div>
      <Skeleton className="h-64 w-full" />
    </div>
  )
}

function isPermissionError(error: Error | null): boolean {
  return error instanceof ApiError && error.status === 403
}

function isNotFoundError(error: Error | null): boolean {
  return error instanceof ApiError && error.status === 404
}
