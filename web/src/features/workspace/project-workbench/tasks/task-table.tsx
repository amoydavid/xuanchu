import { Link } from "@tanstack/react-router"
import { ArrowUpDownIcon, Repeat } from "lucide-react"
import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import { cn } from "@/lib/utils"
import type { ProjectWorkbenchTask } from "../api/project-api"
import { recurrenceRuleLabel } from "../task-series/recurrence-preview"
import { TaskRowActions } from "./task-row-actions"
import { taskDisplayRef, taskRouteRef } from "./task-reference"

type TaskTableProps = {
  canWrite: boolean
  onSortChange?: (sort: string) => void
  projectSlug: string
  sort?: string
  tasks: ProjectWorkbenchTask[]
  workspaceSlug: string
}


export function TaskTable({
  canWrite,
  onSortChange,
  projectSlug,
  sort,
  tasks,
  workspaceSlug,
}: TaskTableProps) {
  const { t } = useTranslation()
  if (tasks.length === 0) {
    return (
      <section className="rounded-lg border bg-card p-6">
        <h2 className="text-base font-medium">
          {t("projectReadonly.emptyTitle")}
        </h2>
        <code className="rounded-lg mt-4 block border bg-background p-3 text-xs break-all">
          xuanchu --workspace {workspaceSlug} add "Design API" project:
          {projectSlug}
        </code>
      </section>
    )
  }

  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{t("projectReadonly.tasks")}</h2>
        <Table containerClassName="hidden md:block">
          <TableHeader>
            <TableRow>
              <SortableHead
                active={sort === "entry"}
                label={t("projectReadonly.identifier")}
                onSortChange={onSortChange}
                sort="entry"
              />
              <TableHead>{t("projectReadonly.title")}</TableHead>
              <TableHead>{t("common.status")}</TableHead>
              <TableHead>{t("projectReadonly.priority")}</TableHead>
              <SortableHead
                active={sort === "urgency"}
                label={t("projectReadonly.urgency")}
                onSortChange={onSortChange}
                sort="urgency"
              />
              <TableHead>{t("projectReadonly.assignee")}</TableHead>
              <SortableHead
                active={sort === "due"}
                label={t("projectReadonly.due")}
                onSortChange={onSortChange}
                sort="due"
              />
              <TableHead className="text-right">
                {t("common.actions")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tasks.map((task) => (
              <TaskTableRow
                canWrite={canWrite}
                key={taskKey(task)}
                projectSlug={projectSlug}
                task={task}
                workspaceSlug={workspaceSlug}
              />
            ))}
          </TableBody>
        </Table>
      <div className="space-y-2 md:hidden">
        {tasks.map((task) => (
          <TaskCard
            canWrite={canWrite}
            key={taskKey(task)}
            projectSlug={projectSlug}
            task={task}
            workspaceSlug={workspaceSlug}
          />
        ))}
      </div>
    </section>
  )
}

function SortableHead({
  active,
  label,
  onSortChange,
  sort,
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
      <Button
        aria-label={`按${label}排序`}
        aria-pressed={active}
        className="h-7 gap-1 px-0 font-medium"
        onClick={() => onSortChange(sort)}
        size="sm"
        type="button"
        variant="ghost"
      >
        {label}
        <ArrowUpDownIcon className="size-3" />
      </Button>
    </TableHead>
  )
}

function TaskTableRow({
  canWrite,
  projectSlug,
  task,
  workspaceSlug,
}: {
  canWrite: boolean
  projectSlug: string
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  const { i18n, t } = useTranslation()
  const isDeleted = task.status === "deleted"
  const rowWritable = canWrite && !isDeleted
  const taskRef = taskReference(task)
  const displayRef = taskDisplayRef(task, i18n.language)

  return (
    <TableRow className={isDeleted ? "opacity-50" : undefined}>
      <TableCell>
        <TaskLink
          projectSlug={projectSlug}
          task={task}
          workspaceSlug={workspaceSlug}
        >
          <code>{displayRef}</code>
        </TaskLink>
      </TableCell>
      <TableCell className="min-w-0 max-w-[420px]">
        <TaskLink
          className="inline-flex min-w-0 max-w-full items-center gap-1.5 truncate text-foreground underline-offset-4 hover:underline"
          projectSlug={projectSlug}
          task={task}
          workspaceSlug={workspaceSlug}
        >
          <span className="truncate">{task.title}</span>
          {task.recurrence_info ? (
            <span
              className="inline-flex shrink-0 text-muted-foreground"
              data-testid="recurrence-badge"
              title={recurrenceRuleLabel(task.recurrence_info.rule, t)}
            >
              <Repeat className="size-3" />
            </span>
          ) : null}
          {isDeleted ? (
            <Badge className="shrink-0" variant="outline">
              {t("myTasks.deleted")}
            </Badge>
          ) : null}
        </TaskLink>
      </TableCell>
      <TableCell>
		<Badge variant="outline">
		  {task.recurrence_info?.materialization === "projected"
			? t("taskSeries.occurrence.projected")
			: taskStatusLabel(task.status, t)}
		</Badge>
      </TableCell>
      <TableCell className="min-w-0 text-xs text-muted-foreground">
        {task.priority ?? "-"}
      </TableCell>
      <TableCell className="tabular-nums text-xs text-muted-foreground">
        <UrgencyScore title={t("projectReadonly.urgencyHelp")} value={task.urgency} />
      </TableCell>
      <TableCell className="min-w-0 max-w-48 truncate">{assigneeNames(task)}</TableCell>
      <TableCell className="min-w-0 whitespace-nowrap font-mono text-xs text-muted-foreground">
        {formatDue(task.due)}
      </TableCell>
      <TableCell className="whitespace-nowrap">
        <TaskRowActions
          canWrite={rowWritable}
          displayRef={displayRef}
          projectSlug={projectSlug}
          recurrenceInfo={task.recurrence_info}
          start={task.start}
          status={task.status}
          taskRef={taskRef}
          workspaceSlug={workspaceSlug}
        />
      </TableCell>
    </TableRow>
  )
}

function TaskCard({
  canWrite,
  projectSlug,
  task,
  workspaceSlug,
}: {
  canWrite: boolean
  projectSlug: string
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  const { i18n, t } = useTranslation()
  const taskRef = taskReference(task)
  const displayRef = taskDisplayRef(task, i18n.language)
  const isDeleted = task.status === "deleted"
  const rowWritable = canWrite && !isDeleted
  return (
    <article className={cn("border bg-card p-3", isDeleted && "opacity-50")}>
      <div className="flex items-center justify-between gap-2 text-xs">
        <TaskLink
          projectSlug={projectSlug}
          task={task}
          workspaceSlug={workspaceSlug}
        >
          <code>{displayRef}</code>
        </TaskLink>
		<Badge variant="outline">
		  {task.recurrence_info?.materialization === "projected"
			? t("taskSeries.occurrence.projected")
			: taskStatusLabel(task.status, t)}
		</Badge>
      </div>
      <div className="mt-2">
        <TaskLink
          className="inline-flex min-w-0 max-w-full items-center gap-1.5 text-sm font-medium underline-offset-4 hover:underline"
          projectSlug={projectSlug}
          task={task}
          workspaceSlug={workspaceSlug}
        >
          <span className="truncate">{task.title}</span>
          {task.recurrence_info ? (
            <span
              className="inline-flex shrink-0 text-muted-foreground"
              data-testid="recurrence-badge"
              title={recurrenceRuleLabel(task.recurrence_info.rule, t)}
            >
              <Repeat className="size-3" />
            </span>
          ) : null}
        </TaskLink>
      </div>
      <div className="mt-2 flex items-center gap-3 text-xs text-muted-foreground">
        <span>{task.priority ?? "-"}</span>
        <span className="font-mono">{formatDue(task.due)}</span>
      </div>
      <div className="mt-2 truncate text-xs text-muted-foreground">
        {assigneeNames(task)}
      </div>
      <div className="mt-1 text-xs text-muted-foreground">
        <UrgencyScore title={t("projectReadonly.urgencyHelp")} value={task.urgency} />
      </div>
      <div className="mt-3">
        <TaskRowActions
          canWrite={rowWritable}
          displayRef={displayRef}
          projectSlug={projectSlug}
          recurrenceInfo={task.recurrence_info}
          start={task.start}
          status={task.status}
          taskRef={taskRef}
          workspaceSlug={workspaceSlug}
        />
      </div>
    </article>
  )
}

function TaskLink({
  children,
  className,
  projectSlug,
  task,
  workspaceSlug,
}: {
  children: ReactNode
  className?: string
  projectSlug: string
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  return (
    <Link
      className={cn(
        "text-foreground underline-offset-4 hover:underline",
        className
      )}
      params={{
        workspaceSlug,
        projectSlug,
        taskRef: taskRouteRef(task),
      }}
      to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
    >
      {children}
    </Link>
  )
}

function taskReference(task: ProjectWorkbenchTask): string {
  return taskRouteRef(task)
}

/** 稳定 key：projected occurrence 用 id，普通任务用 uuid。 */
function taskKey(task: ProjectWorkbenchTask): string {
  return task.id || task.uuid || ""
}

// 到期时间纯显示：unix 秒或 ISO 字符串 → YYYY-MM-DD；无值显示「-」。
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

function assigneeNames(task: ProjectWorkbenchTask): string {
  if (!task.assignees || task.assignees.length === 0) {
    return "-"
  }
  return task.assignees
    .map(
      (assignee) =>
        assignee.display_name || assignee.name || assignee.email || assignee.id
    )
    .filter(Boolean)
    .join(", ")
}

// UrgencyScore 展示后端计算的 urgency 总分；未计算（null/undefined）时显示占位符。
// 只在按 urgency/next 排序时后端会填充该分数（spec §13.3）。
function UrgencyScore({
  title,
  value,
}: {
  title?: string
  value: number | null | undefined
}) {
  if (value === null || value === undefined || Number.isNaN(value)) {
    return <span aria-hidden>-</span>
  }
  return <span title={title}>{value.toFixed(1)}</span>
}
