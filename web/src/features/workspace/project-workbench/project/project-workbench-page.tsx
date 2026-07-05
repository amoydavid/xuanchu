import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useNavigate, useSearch } from "@tanstack/react-router"
import { UploadIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { navigateToDocument } from "@/lib/browser-navigation"
import { ProjectActivity } from "@/features/workspace/project-readonly/project-activity"
import {
  filterToTaskQuery,
  type TaskFilter,
} from "@/features/workspace/project-readonly/project-filter"
import type { ProjectReadonlyTask } from "@/features/workspace/project-readonly/project-readonly-api"
import { buildProjectStats } from "@/features/workspace/project-readonly/project-stats"
import type { ProjectWorkbenchTask } from "../api/project-api"
import { getWorkspaceMembers } from "../api/users-api"
import { TaskImportDialog } from "../import/task-import-dialog"
import { canProjectManage, canTaskWrite } from "../permissions/permissions"
import {
  useProjectQuery,
  useProjectTasksQuery,
  useProjectTimelineQuery,
} from "../hooks/use-project-data"
import { EditFeedbackProvider, useEditFeedback } from "../shared/edit-feedback"
import { ProjectTaskToolbar } from "../tasks/project-task-toolbar"
import { TaskCreateDialog } from "../tasks/task-create-dialog"
import { TaskTable } from "../tasks/task-table"
import { AssigneeWorkloadSummary } from "./assignee-workload-summary"
import { ProjectClosedBanner } from "./project-closed-banner"
import { ProjectHeaderEditor } from "./project-header-editor"
import { isClosedProjectStatus } from "./project-status-menu"

type ProjectWorkbenchPageProps = {
  projectSlug: string
  workspaceSlug: string
}

export function ProjectWorkbenchPage({
  projectSlug,
  workspaceSlug,
}: ProjectWorkbenchPageProps) {
  return (
    <EditFeedbackProvider>
      <ProjectWorkbenchPageContent
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />
    </EditFeedbackProvider>
  )
}

function ProjectWorkbenchPageContent({
  projectSlug,
  workspaceSlug,
}: ProjectWorkbenchPageProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const feedback = useEditFeedback()
  const [now] = useState(() => Math.floor(Date.now() / 1000))
  const [importOpen, setImportOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const search = useSearch({ strict: false }) as Partial<TaskFilter>
  const filter: TaskFilter = useMemo(
    () => ({
      status: typeof search.status === "string" ? search.status : undefined,
      priority:
        typeof search.priority === "string" ? search.priority : undefined,
      assignee:
        typeof search.assignee === "string" ? search.assignee : undefined,
      due_after:
        typeof search.due_after === "string" ? search.due_after : undefined,
      due_before:
        typeof search.due_before === "string" ? search.due_before : undefined,
      due_empty:
        typeof search.due_empty === "string" ? search.due_empty : undefined,
      assignee_empty:
        typeof search.assignee_empty === "string"
          ? search.assignee_empty
          : undefined,
      wait_before:
        typeof search.wait_before === "string" ? search.wait_before : undefined,
      scheduled_before:
        typeof search.scheduled_before === "string"
          ? search.scheduled_before
          : undefined,
      until_before:
        typeof search.until_before === "string"
          ? search.until_before
          : undefined,
      tags: typeof search.tags === "string" ? search.tags : undefined,
      q: typeof search.q === "string" ? search.q : undefined,
      query: typeof search.query === "string" ? search.query : undefined,
      sort: typeof search.sort === "string" ? search.sort : undefined,
    }),
    [
      search.assignee,
      search.assignee_empty,
      search.due_after,
      search.due_before,
      search.due_empty,
      search.priority,
      search.q,
      search.query,
      search.scheduled_before,
      search.sort,
      search.status,
      search.tags,
      search.until_before,
      search.wait_before,
    ]
  )
  const filterQuery = useMemo(() => filterToTaskQuery(filter), [filter])
  const me = useMe()
  const canManage = canProjectManage({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const canCreateTask = canTaskWrite({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const project = useProjectQuery(workspaceSlug, projectSlug)
  const tasks = useProjectTasksQuery(workspaceSlug, projectSlug, filterQuery)
  const timeline = useProjectTimelineQuery(workspaceSlug, projectSlug)
  const members = useQuery({
    queryKey: ["workspace-members", workspaceSlug],
    queryFn: () => getWorkspaceMembers(workspaceSlug),
  })
  const taskRows = useMemo(() => tasks.data ?? [], [tasks.data])
  const assigneeOptions = useMemo(
    () =>
      (members.data ?? []).map((member) => ({
        email: member.email,
        id: member.user_id,
        label: member.display_name || member.name || member.email || member.user_id,
        name: member.name,
      })),
    [members.data]
  )
  const stats = useMemo(
    () => buildProjectStats(taskRows.map(normalizeReadonlyTask), now),
    [now, taskRows]
  )
  const accessError = project.error ?? tasks.error
  if (isPermissionError(accessError)) {
    return (
      <ProjectState
        actionLabel={t("projectReadonly.backToOverview")}
        description={t("projectWorkbench.project.permissionDescription", {
          project: `${workspaceSlug} / ${projectSlug}`,
        })}
        detail="project:read, task:read"
        title={t("projectWorkbench.project.permissionTitle")}
      />
    )
  }
  if (isNotFoundError(project.error)) {
    return (
      <ProjectState
        actionLabel={t("projectReadonly.backToOverview")}
        description={t("projectWorkbench.project.notFoundDescription")}
        title={t("projectWorkbench.project.notFoundTitle")}
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
        title={t("projectWorkbench.project.loadFailed")}
      />
    )
  }

  const canEditTasks =
    canCreateTask && !isClosedProjectStatus(project.data.status)
  const setTaskFilters = (
    values: Partial<Record<keyof TaskFilter, string>>
  ) => {
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug, projectSlug },
      search: (prev) => {
        const next = { ...(prev as Record<string, string>) }
        for (const [key, value] of Object.entries(values)) {
          if (value) {
            next[key] = value
          } else {
            delete next[key]
          }
        }
        return next
      },
    })
  }

  return (
    <div className="space-y-5">
      <ProjectHeaderEditor
        canManage={canManage}
        importAction={
          canEditTasks ? (
            <Button
              onClick={() => setImportOpen(true)}
              size="sm"
              type="button"
              variant="outline"
            >
              <UploadIcon />
              {t("projectWorkbench.import.button")}
            </Button>
          ) : null
        }
        onCopyLink={() => {
          void navigator.clipboard?.writeText(window.location.href)
          feedback.success(t("projectReadonly.copied"))
        }}
        project={project.data}
        workspaceSlug={workspaceSlug}
      />
      <TaskImportDialog
        existingTasks={taskRows}
        onOpenChange={setImportOpen}
        open={importOpen}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />
      <TaskCreateDialog
        filters={filterQuery}
        onOpenChange={setCreateOpen}
        open={createOpen}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />
      <ProjectClosedBanner canManage={canManage} status={project.data.status} />
      <ProjectStatsGrid stats={stats} />
      <ProjectTaskToolbar
        assigneeOptions={assigneeOptions}
        canCreateTask={canEditTasks}
        filter={filter}
        onCreateTask={() => setCreateOpen(true)}
        toParams={{ workspaceSlug, projectSlug }}
      />
      <TaskTable
        canWrite={canEditTasks}
        onSortChange={(sort) => setTaskFilters({ sort })}
        projectSlug={projectSlug}
        sort={filter.sort}
        tasks={taskRows}
        workspaceSlug={workspaceSlug}
      />
      <AssigneeWorkloadSummary
        now={now}
        onFilterAssignee={(assignee) =>
          setTaskFilters({ assignee, assignee_empty: "" })
        }
        onFilterUnassigned={() =>
          setTaskFilters({ assignee: "", assignee_empty: "true" })
        }
        tasks={taskRows}
      />
      <ProjectActivity
        entries={timeline.isError ? [] : timeline.data}
        title={t("projectReadonly.recentActivity")}
      />
    </div>
  )
}

function ProjectStatsGrid({
  stats,
}: {
  stats: ReturnType<typeof buildProjectStats>
}) {
  const { t } = useTranslation()
  const items = [
    [t("projectReadonly.pending"), stats.pending],
    [t("projectWorkbench.project.inProgress"), stats.active],
    [t("projectReadonly.completed"), stats.completed],
    [t("projectReadonly.overdue"), stats.overdue],
    [t("projectReadonly.highPriority"), stats.highPriority],
  ] as const
  return (
    <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
      {items.map(([label, value]) => (
        <div className="border bg-card p-3" key={label}>
          <div className="text-xs text-muted-foreground">{label}</div>
          <div className="mt-1 text-2xl font-semibold">{value}</div>
        </div>
      ))}
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

function normalizeReadonlyTask(
  task: ProjectWorkbenchTask
): ProjectReadonlyTask {
  return {
    ...task,
    description: task.description ?? undefined,
    project: task.project ?? undefined,
    parent: task.parent ?? undefined,
    recur: task.recur ?? undefined,
    due: unixLikeToNumber(task.due),
    scheduled: unixLikeToNumber(task.scheduled),
    start: unixLikeToNumber(task.start),
    until: unixLikeToNumber(task.until),
    wait: unixLikeToNumber(task.wait),
    assignees: task.assignees?.map((assignee) => ({
      email: assignee.email ?? undefined,
      id: assignee.id,
      name: assignee.name,
      user_id: assignee.user_id,
    })),
    links: task.links?.map((link) => ({
      ...link,
      created_by: link.created_by
        ? {
            email: link.created_by.email ?? undefined,
            id: link.created_by.id,
            name: link.created_by.name,
          }
        : undefined,
    })),
  }
}

function unixLikeToNumber(value: string | number | null | undefined) {
  if (typeof value === "number") {
    return value
  }
  if (typeof value === "string") {
    const parsed = Date.parse(value)
    if (!Number.isNaN(parsed)) {
      return Math.floor(parsed / 1000)
    }
  }
  return null
}
