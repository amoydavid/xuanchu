import { useMemo, useState } from "react"
import { useSearch } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import { ProjectActivity } from "@/features/workspace/project-readonly/project-activity"
import { ProjectFilterToolbar } from "@/features/workspace/project-readonly/project-filter-toolbar"
import {
  filterToTaskQuery,
  type TaskFilter,
} from "@/features/workspace/project-readonly/project-filter"
import type { ProjectReadonlyTask } from "@/features/workspace/project-readonly/project-readonly-api"
import {
  buildProjectStats,
  summarizeAssignees,
} from "@/features/workspace/project-readonly/project-stats"
import type { ProjectWorkbenchTask } from "../api/project-api"
import { canProjectManage, canTaskWrite } from "../permissions/permissions"
import {
  useProjectQuery,
  useProjectTasksQuery,
  useProjectTimelineQuery,
} from "../hooks/use-project-data"
import { TaskQuickCreate } from "../tasks/task-quick-create"
import { TaskTable } from "../tasks/task-table"
import { ProjectClosedBanner } from "./project-closed-banner"
import { ProjectHeaderEditor } from "./project-header-editor"

type ProjectWorkbenchPageProps = {
  projectSlug: string
  workspaceSlug: string
}

export function ProjectWorkbenchPage({
  projectSlug,
  workspaceSlug,
}: ProjectWorkbenchPageProps) {
  const [copied, setCopied] = useState(false)
  const [now] = useState(() => Math.floor(Date.now() / 1000))
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
  const taskRows = useMemo(() => tasks.data ?? [], [tasks.data])
  const stats = useMemo(
    () => buildProjectStats(taskRows.map(normalizeReadonlyTask), now),
    [now, taskRows]
  )
  const assignees = useMemo(
    () =>
      summarizeAssignees(
        taskRows.map(normalizeReadonlyTask),
        now,
        "未分配"
      ),
    [now, taskRows]
  )

  const accessError = project.error ?? tasks.error
  if (isPermissionError(accessError)) {
    return (
      <ProjectState
        actionLabel="返回概览"
        description={`${workspaceSlug} / ${projectSlug} 需要 project:read 和 task:read 权限。`}
        detail="project:read, task:read"
        title="没有项目访问权限"
      />
    )
  }
  if (isNotFoundError(project.error)) {
    return (
      <ProjectState
        actionLabel="返回概览"
        description="项目不存在，或当前 token 不在项目允许范围内。"
        title="项目不存在"
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
        actionLabel="刷新"
        description={code}
        onAction={() => window.location.reload()}
        title="加载失败"
      />
    )
  }

  return (
    <div className="space-y-5">
      <ProjectHeaderEditor
        canManage={canManage}
        onCopyLink={() => {
          void navigator.clipboard?.writeText(window.location.href)
          setCopied(true)
        }}
        project={project.data}
        workspaceSlug={workspaceSlug}
      />
      {copied ? (
        <div className="text-xs text-muted-foreground">链接已复制</div>
      ) : null}
      <ProjectClosedBanner canManage={canManage} status={project.data.status} />
      <ProjectStatsGrid stats={stats} />
      <ProjectFilterToolbar
        filter={filter}
        toParams={{ workspaceSlug, projectSlug }}
      />
      <TaskQuickCreate
        canCreate={canCreateTask}
        filters={filterQuery}
        projectSlug={projectSlug}
        projectStatus={project.data.status}
        workspaceSlug={workspaceSlug}
      />
      <TaskTable
        canWrite={canCreateTask}
        projectSlug={projectSlug}
        tasks={taskRows}
        workspaceSlug={workspaceSlug}
      />
      <ProjectAssigneeSummary assignees={assignees} />
      <ProjectActivity
        entries={timeline.isError ? [] : timeline.data}
        title="最近动态"
      />
    </div>
  )
}

function ProjectStatsGrid({
  stats,
}: {
  stats: ReturnType<typeof buildProjectStats>
}) {
  const items = [
    ["待处理", stats.pending],
    ["进行中", stats.active],
    ["已完成", stats.completed],
    ["已逾期", stats.overdue],
    ["高优先级", stats.highPriority],
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

function ProjectAssigneeSummary({
  assignees,
}: {
  assignees: ReturnType<typeof summarizeAssignees>
}) {
  if (assignees.length === 0) {
    return null
  }
  return (
    <section className="border bg-card p-3">
      <h2 className="text-sm font-medium">负责人概览</h2>
      <div className="mt-3 grid gap-2 md:grid-cols-2 lg:grid-cols-3">
        {assignees.slice(0, 6).map((assignee) => (
          <div className="grid grid-cols-[1fr_auto_auto] gap-3 text-xs" key={assignee.key}>
            <span className="truncate">{assignee.label}</span>
            <span className="text-muted-foreground">{assignee.open} open</span>
            <span className="text-muted-foreground">
              {assignee.overdue} overdue
            </span>
          </div>
        ))}
      </div>
    </section>
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
        onClick={onAction ?? (() => (window.location.href = "/"))}
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

function normalizeReadonlyTask(task: ProjectWorkbenchTask): ProjectReadonlyTask {
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
