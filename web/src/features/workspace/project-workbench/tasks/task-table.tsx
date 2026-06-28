import { Link } from "@tanstack/react-router"
import type { ReactNode } from "react"

import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { ProjectWorkbenchTask } from "../api/project-api"
import { useModifyTaskMutation } from "../hooks/use-task-mutations"
import { InlineDateEditor } from "../shared/inline-date-editor"
import { InlineSelectEditor } from "../shared/inline-select-editor"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { TaskRowActions } from "./task-row-actions"

type TaskTableProps = {
  canWrite: boolean
  projectSlug: string
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
  projectSlug,
  tasks,
  workspaceSlug,
}: TaskTableProps) {
  if (tasks.length === 0) {
    return (
      <section className="border bg-card p-6">
        <h2 className="text-base font-medium">这个项目还没有任务</h2>
        <code className="mt-4 block break-all border bg-background p-3 text-xs">
          xuanchu --workspace {workspaceSlug} add "Design API" project:{projectSlug}
        </code>
      </section>
    )
  }

  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">任务</h2>
      <div className="hidden border bg-card md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>标题</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>优先级</TableHead>
              <TableHead>负责人</TableHead>
              <TableHead>截止</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tasks.map((task) => (
              <TaskTableRow
                canWrite={canWrite}
                key={task.uuid}
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
            key={task.uuid}
            projectSlug={projectSlug}
            task={task}
            workspaceSlug={workspaceSlug}
          />
        ))}
      </div>
    </section>
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
  const taskRef = taskReference(task)
  const modify = useModifyTaskMutation(workspaceSlug, projectSlug, taskRef)

  return (
    <TableRow>
      <TableCell>
        <TaskLink projectSlug={projectSlug} task={task} workspaceSlug={workspaceSlug}>
          <code>{task.task_slug || task.uuid.slice(0, 8)}</code>
        </TaskLink>
      </TableCell>
      <TableCell className="min-w-64 max-w-lg">
        <InlineTextEditor
          ariaLabel={`编辑任务标题 ${taskRef}`}
          disabled={!canWrite}
          displayClassName="max-w-lg"
          onSave={async (title) => {
            await modify.mutateAsync({ title })
          }}
          value={task.title}
          validate={(title) => (title.trim() ? null : "标题不能为空")}
        />
      </TableCell>
      <TableCell>
        <Badge variant="outline">{task.status}</Badge>
      </TableCell>
      <TableCell>
        <InlineSelectEditor
          ariaLabel={`任务优先级 ${taskRef}`}
          className="h-7 w-20"
          disabled={!canWrite}
          onSave={async (priority) => {
            await modify.mutateAsync(
              priority === "none" ? { clear_priority: true } : { priority }
            )
          }}
          options={priorityOptions}
          placeholder="-"
          value={task.priority ?? "none"}
        />
      </TableCell>
      <TableCell className="max-w-48 truncate">{assigneeNames(task)}</TableCell>
      <TableCell>
        <InlineDateEditor
          ariaLabel={`任务截止日期 ${taskRef}`}
          className="h-7 w-36"
          disabled={!canWrite}
          onSave={async (due) => {
            await modify.mutateAsync(due === null ? { clear_due: true } : { due })
          }}
          value={unixLikeToNumber(task.due)}
        />
      </TableCell>
      <TableCell>
        <TaskRowActions
          canWrite={canWrite}
          projectSlug={projectSlug}
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
  const taskRef = taskReference(task)
  const modify = useModifyTaskMutation(workspaceSlug, projectSlug, taskRef)
  return (
    <article className="border bg-card p-3">
      <div className="flex items-center justify-between gap-2 text-xs">
        <TaskLink projectSlug={projectSlug} task={task} workspaceSlug={workspaceSlug}>
          <code>{task.task_slug || task.uuid.slice(0, 8)}</code>
        </TaskLink>
        <Badge variant="outline">{task.status}</Badge>
      </div>
      <div className="mt-2">
        <InlineTextEditor
          ariaLabel={`编辑移动任务标题 ${taskRef}`}
          disabled={!canWrite}
          displayClassName="max-w-full text-sm font-medium"
          onSave={async (title) => {
            await modify.mutateAsync({ title })
          }}
          value={task.title}
          validate={(title) => (title.trim() ? null : "标题不能为空")}
        />
      </div>
      <div className="mt-2 grid grid-cols-[5rem_minmax(0,1fr)] gap-2">
        <InlineSelectEditor
          ariaLabel={`移动任务优先级 ${taskRef}`}
          className="h-7 w-full"
          disabled={!canWrite}
          onSave={async (priority) => {
            await modify.mutateAsync(
              priority === "none" ? { clear_priority: true } : { priority }
            )
          }}
          options={priorityOptions}
          placeholder="-"
          value={task.priority ?? "none"}
        />
        <InlineDateEditor
          ariaLabel={`移动任务截止日期 ${taskRef}`}
          className="h-7 w-full"
          disabled={!canWrite}
          onSave={async (due) => {
            await modify.mutateAsync(due === null ? { clear_due: true } : { due })
          }}
          value={unixLikeToNumber(task.due)}
        />
      </div>
      <div className="mt-2 truncate text-xs text-muted-foreground">
        {assigneeNames(task)}
      </div>
      <div className="mt-3">
        <TaskRowActions
          canWrite={canWrite}
          projectSlug={projectSlug}
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
        taskRef: taskReference(task),
      }}
      to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
    >
      {children}
    </Link>
  )
}

function taskReference(task: ProjectWorkbenchTask): string {
  return task.task_slug || task.uuid
}

function assigneeNames(task: ProjectWorkbenchTask): string {
  if (!task.assignees || task.assignees.length === 0) {
    return "-"
  }
  return task.assignees
    .map((assignee) => assignee.name || assignee.email || assignee.user_id || assignee.id)
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
