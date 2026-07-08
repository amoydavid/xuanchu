import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import type { ProjectWorkbenchTask } from "@/features/workspace/project-workbench/api/project-api"

type MyTasksTableProps = {
  sort?: string
  tasks: ProjectWorkbenchTask[]
  workspaceSlug: string
  onSortChange?: (sort: string) => void
}

const priorityLabel: Record<string, string> = {
  H: "高",
  M: "中",
  L: "低",
}

export function MyTasksTable({
  onSortChange,
  sort,
  tasks,
  workspaceSlug,
}: MyTasksTableProps) {
  const { t } = useTranslation()
  if (tasks.length === 0) {
    return (
      <section className="border bg-card p-6 text-sm text-muted-foreground">
        {t("myTasks.empty")}
      </section>
    )
  }

  return (
    <section className="space-y-2">
      <div className="hidden border bg-card md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("projectReadonly.identifier")}</TableHead>
              <TableHead>{t("projectReadonly.title")}</TableHead>
              <TableHead>{t("common.status")}</TableHead>
              <TableHead>{t("projectReadonly.priority")}</TableHead>
              <SortHead
                active={sort === "due"}
                label={t("projectReadonly.due")}
                onSortChange={onSortChange}
                sort="due"
              />
              <TableHead>{t("projectReadonly.project")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tasks.map((task) => (
              <MyTasksTableRow
                key={task.uuid}
                task={task}
                workspaceSlug={workspaceSlug}
              />
            ))}
          </TableBody>
        </Table>
      </div>
      <div className="space-y-2 md:hidden">
        {tasks.map((task) => (
          <MyTasksTaskCard
            key={task.uuid}
            task={task}
            workspaceSlug={workspaceSlug}
          />
        ))}
      </div>
    </section>
  )
}

function SortHead({
  active,
  label,
  onSortChange,
  sort: sortKey,
}: {
  active: boolean
  label: string
  onSortChange?: (sort: string) => void
  sort: string
}) {
  if (!onSortChange) {
    return <TableHead>{label}</TableHead>
  }
  return (
    <TableHead>
      <button
        aria-label={`按${label}排序`}
        aria-pressed={active}
        className="font-medium underline-offset-4 hover:underline"
        onClick={() => onSortChange(sortKey)}
        type="button"
      >
        {label}
      </button>
    </TableHead>
  )
}

function MyTasksTableRow({
  task,
  workspaceSlug,
}: {
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const isDeleted = task.status === "deleted"
  return (
    <TableRow className={isDeleted ? "opacity-50" : undefined}>
      <TableCell>
        <TaskDetailLink task={task} workspaceSlug={workspaceSlug}>
          {taskRef(task)}
        </TaskDetailLink>
      </TableCell>
      <TableCell className="max-w-lg min-w-48 truncate">
        {task.title}
        {isDeleted ? (
          <Badge className="ml-2" variant="outline">
            {t("myTasks.deleted")}
          </Badge>
        ) : null}
      </TableCell>
      <TableCell>
        <Badge variant="outline">{taskStatusLabel(task.status, t)}</Badge>
      </TableCell>
      <TableCell>{task.priority ? (priorityLabel[task.priority] ?? task.priority) : "-"}</TableCell>
      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
        {formatDue(task.due)}
      </TableCell>
      <TableCell className="max-w-32 truncate">
        {task.project ? (
          <Link
            className="text-foreground underline-offset-4 hover:underline"
            params={{
              workspaceSlug,
              projectSlug: task.project,
            }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug"
          >
            {task.project}
          </Link>
        ) : (
          "-"
        )}
      </TableCell>
    </TableRow>
  )
}

function MyTasksTaskCard({
  task,
  workspaceSlug,
}: {
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  return (
    <article className="border bg-card p-3">
      <div className="flex items-center justify-between gap-2 text-xs">
        <TaskDetailLink task={task} workspaceSlug={workspaceSlug}>
          {taskRef(task)}
        </TaskDetailLink>
        <Badge variant="outline">{taskStatusLabel(task.status, t)}</Badge>
      </div>
      <div className="mt-2 text-sm font-medium">{task.title}</div>
      <div className="mt-1 truncate text-xs text-muted-foreground">
        {task.project ? (
          <Link
            className="text-foreground underline-offset-4 hover:underline"
            params={{
              workspaceSlug,
              projectSlug: task.project,
            }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug"
          >
            {task.project}
          </Link>
        ) : (
          "-"
        )}
      </div>
    </article>
  )
}

function TaskDetailLink({
  children,
  task,
  workspaceSlug,
}: {
  children: React.ReactNode
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  if (!task.project) {
    return <span className="text-foreground">{children}</span>
  }

  return (
    <Link
      className="text-foreground underline-offset-4 hover:underline"
      params={{
        projectSlug: task.project,
        taskRef: taskRef(task),
        workspaceSlug,
      }}
      to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
    >
      {children}
    </Link>
  )
}

function taskRef(task: ProjectWorkbenchTask): string {
  return task.task_slug || task.uuid
}

function formatDue(due: ProjectWorkbenchTask["due"]): string {
  if (due === null || due === undefined) return "-"
  const ts =
    typeof due === "number"
      ? due
      : (() => {
          const parsed = Date.parse(due)
          return Number.isNaN(parsed) ? null : Math.floor(parsed / 1000)
        })()
  if (ts === null) return "-"
  const date = new Date(ts * 1000)
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, "0")
  const d = String(date.getDate()).padStart(2, "0")
  return `${y}-${m}-${d}`
}
