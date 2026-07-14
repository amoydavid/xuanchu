import { Link } from "@tanstack/react-router"
import { ArrowUpDownIcon } from "lucide-react"
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
import { useModifyTaskMutation } from "../hooks/use-task-mutations"
import { InlineDatePicker } from "../shared/inline-date-picker"
import { InlineSelectEditor } from "../shared/inline-select-editor"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { TaskRowActions } from "./task-row-actions"
import { recurrenceRuleLabel } from "../task-series/recurrence-preview"
import { taskDisplayRef, taskRouteRef } from "./task-reference"

type TaskTableProps = {
  canWrite: boolean
  onSortChange?: (sort: string) => void
  projectSlug: string
  sort?: string
  tasks: ProjectWorkbenchTask[]
  workspaceSlug: string
}

const priorityOptions = [
  { label: "-", value: "none" },
  { label: "H", value: "H" },
  { label: "M", value: "M" },
  { label: "L", value: "L" },
]

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
      <section className="border bg-card p-6">
        <h2 className="text-base font-medium">
          {t("projectReadonly.emptyTitle")}
        </h2>
        <code className="mt-4 block border bg-background p-3 text-xs break-all">
          xuanchu --workspace {workspaceSlug} add "Design API" project:
          {projectSlug}
        </code>
      </section>
    )
  }

  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{t("projectReadonly.tasks")}</h2>
      <div className="hidden border bg-card md:block">
        <Table>
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
      </div>
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
  const modify = useModifyTaskMutation(workspaceSlug, projectSlug, taskRef)

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
      <TableCell className="max-w-lg min-w-64">
        <InlineTextEditor
          ariaLabel={t("projectReadonly.editTaskTitle", { taskRef: displayRef })}
          disabled={!rowWritable}
          displayClassName="max-w-lg"
          onSave={async (title) => {
            await modify.mutateAsync({ title })
          }}
          value={task.title}
          validate={(title) =>
            title.trim() ? null : t("projectReadonly.taskTitleRequired")
          }
        />
        {isDeleted ? (
          <Badge className="ml-2" variant="outline">
            {t("myTasks.deleted")}
          </Badge>
        ) : null}
        {task.recurrence_info ? (
          <Badge
            className="ml-1"
            data-testid="recurrence-badge"
            title={t(
              `taskSeries.occurrence.${task.recurrence_info.materialization}`
            )}
            variant="outline"
          >
            {t("taskSeries.occurrence.badge", {
              rule: recurrenceRuleLabel(task.recurrence_info.rule, t),
            })}
          </Badge>
        ) : null}
      </TableCell>
      <TableCell>
		<Badge variant="outline">
		  {task.recurrence_info?.materialization === "projected"
			? t("taskSeries.occurrence.projected")
			: taskStatusLabel(task.status, t)}
		</Badge>
      </TableCell>
      <TableCell>
        <InlineSelectEditor
          ariaLabel={t("projectReadonly.taskPriority", { taskRef: displayRef })}
          className="w-20"
          disabled={!rowWritable}
          onSave={async (priority) => {
            await modify.mutateAsync(
              priority === "none" ? { clear_priority: true } : { priority }
            )
          }}
          options={priorityOptions}
          placeholder="-"
          triggerSize="sm"
          value={task.priority ?? "none"}
        />
      </TableCell>
      <TableCell className="max-w-48 truncate">{assigneeNames(task)}</TableCell>
      <TableCell>
        <InlineDatePicker
          ariaLabel={t("projectReadonly.taskDueDate", { taskRef: displayRef })}
          boundary="end"
          className="w-36"
          disabled={!rowWritable}
          onSave={async (due) => {
            await modify.mutateAsync(
              due === null ? { clear_due: true } : { due }
            )
          }}
          value={unixLikeToNumber(task.due)}
        />
      </TableCell>
      <TableCell>
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
  const modify = useModifyTaskMutation(workspaceSlug, projectSlug, taskRef)
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
        <InlineTextEditor
          ariaLabel={t("projectReadonly.editMobileTaskTitle", { taskRef: displayRef })}
          disabled={!rowWritable}
          displayClassName="max-w-full text-sm font-medium"
          onSave={async (title) => {
            await modify.mutateAsync({ title })
          }}
          value={task.title}
          validate={(title) =>
            title.trim() ? null : t("projectReadonly.taskTitleRequired")
          }
        />
      </div>
      <div className="mt-2 grid grid-cols-[5rem_minmax(0,1fr)] gap-2">
        <InlineSelectEditor
          ariaLabel={t("projectReadonly.mobileTaskPriority", { taskRef: displayRef })}
          className="w-full"
          disabled={!rowWritable}
          onSave={async (priority) => {
            await modify.mutateAsync(
              priority === "none" ? { clear_priority: true } : { priority }
            )
          }}
          options={priorityOptions}
          placeholder="-"
          triggerSize="sm"
          value={task.priority ?? "none"}
        />
        <InlineDatePicker
          ariaLabel={t("projectReadonly.mobileTaskDueDate", { taskRef: displayRef })}
          boundary="end"
          className="w-full"
          disabled={!rowWritable}
          onSave={async (due) => {
            await modify.mutateAsync(
              due === null ? { clear_due: true } : { due }
            )
          }}
          value={unixLikeToNumber(task.due)}
        />
      </div>
      <div className="mt-2 truncate text-xs text-muted-foreground">
        {assigneeNames(task)}
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
  projectSlug,
  task,
  workspaceSlug,
}: {
  children: ReactNode
  projectSlug: string
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  return (
    <Link
      className="text-foreground underline-offset-4 hover:underline"
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
