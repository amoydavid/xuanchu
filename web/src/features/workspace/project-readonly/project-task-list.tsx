import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { ProjectReadonlyTask } from "./project-readonly-api"

type ProjectTaskListProps = {
  emptyExample: string
  emptyTitle: string
  projectSlug: string
  tasks: ProjectReadonlyTask[]
  t: (key: string) => string
  workspaceSlug: string
}

export function ProjectTaskList({
  emptyExample,
  emptyTitle,
  projectSlug,
  tasks,
  t,
  workspaceSlug,
}: ProjectTaskListProps) {
  if (tasks.length === 0) {
    return (
      <section className="border bg-card p-6">
        <h2 className="text-base font-medium">{emptyTitle}</h2>
        <code className="mt-4 block break-all border bg-background p-3 text-xs">
          {emptyExample}
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
              <TableHead>{t("projectReadonly.identifier")}</TableHead>
              <TableHead>{t("projectReadonly.description")}</TableHead>
              <TableHead>{t("common.status")}</TableHead>
              <TableHead>{t("projectReadonly.assignee")}</TableHead>
              <TableHead>{t("projectReadonly.due")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tasks.map((task) => (
              <TableRow key={task.uuid}>
                <TableCell>
                  <a
                    className="text-foreground underline-offset-4 hover:underline"
                    href={taskDetailHref(workspaceSlug, projectSlug, task)}
                  >
                    <code>{task.task_slug || task.uuid.slice(0, 8)}</code>
                  </a>
                </TableCell>
                <TableCell>
                  <a
                    className="text-foreground underline-offset-4 hover:underline"
                    href={taskDetailHref(workspaceSlug, projectSlug, task)}
                  >
                    {task.description}
                  </a>
                </TableCell>
                <TableCell>
                  <Badge variant="outline">{task.status}</Badge>
                </TableCell>
                <TableCell>{assigneeNames(task)}</TableCell>
                <TableCell>{formatUnixDate(task.due)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <div className="space-y-2 md:hidden">
        {tasks.map((task) => (
          <a
            className="block border bg-card p-3 text-foreground transition-colors hover:bg-muted"
            href={taskDetailHref(workspaceSlug, projectSlug, task)}
            key={task.uuid}
          >
            <div className="flex items-center justify-between gap-2 text-xs">
              <code>{task.task_slug || task.uuid.slice(0, 8)}</code>
              <span className="text-muted-foreground">
                {task.status}
                {task.priority ? ` ${task.priority}` : ""}
              </span>
            </div>
            <div className="mt-2 line-clamp-2 text-sm">{task.description}</div>
            <div className="mt-2 text-xs text-muted-foreground">
              {assigneeNames(task)} · {formatUnixDate(task.due)}
            </div>
          </a>
        ))}
      </div>
    </section>
  )
}

function taskDetailHref(
  workspaceSlug: string,
  projectSlug: string,
  task: ProjectReadonlyTask
): string {
  const taskRef = task.task_slug || task.uuid
  return `/workspaces/${encodeURIComponent(workspaceSlug)}/projects/${encodeURIComponent(projectSlug)}/tasks/${encodeURIComponent(taskRef)}`
}

function assigneeNames(task: ProjectReadonlyTask): string {
  if (!task.assignees || task.assignees.length === 0) {
    return "-"
  }
  return task.assignees
    .map((assignee) => assignee.name || assignee.email || assignee.user_id)
    .filter(Boolean)
    .join(", ")
}

function formatUnixDate(value?: number | null): string {
  if (typeof value !== "number") {
    return "-"
  }
  return new Date(value * 1000).toISOString().slice(0, 10)
}
