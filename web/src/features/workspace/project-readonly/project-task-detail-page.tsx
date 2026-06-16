import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"
import {
  getProjectReadonlyTask,
  type ProjectReadonlyTask,
} from "./project-readonly-api"

type ProjectTaskDetailPageProps = {
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

export function ProjectTaskDetailPage({
  projectSlug,
  taskRef,
  workspaceSlug,
}: ProjectTaskDetailPageProps) {
  const { t } = useTranslation()
  const task = useQuery({
    queryKey: ["project-readonly", workspaceSlug, projectSlug, "task", taskRef],
    queryFn: () => getProjectReadonlyTask(workspaceSlug, taskRef),
  })
  const projectLinkProps = {
    to: "/workspaces/$workspaceSlug/projects/$projectSlug",
    params: { workspaceSlug, projectSlug },
  } as const

  if (task.isPending) {
    return <TaskDetailSkeleton />
  }

  if (task.isError) {
    const title =
      task.error instanceof ApiError && task.error.status === 404
        ? t("projectReadonly.taskNotFoundTitle")
        : t("common.error")
    return (
      <section className="max-w-2xl border bg-card p-6">
        <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
        <p className="mt-3 text-sm text-muted-foreground">
          {task.error instanceof ApiError ? task.error.code : "unknown"}
        </p>
        <Button asChild className="mt-5" variant="outline">
          <Link {...projectLinkProps}>{t("projectReadonly.backToProject")}</Link>
        </Button>
      </section>
    )
  }

  const taskData = task.data

  if (!taskBelongsToProject(taskData, projectSlug)) {
    return (
      <TaskUnavailable
        linkProps={projectLinkProps}
        title={t("projectReadonly.taskNotFoundTitle")}
        backLabel={t("projectReadonly.backToProject")}
      />
    )
  }

  return (
    <div className="space-y-5">
      <section className="border-b pb-4">
        <div className="text-xs text-muted-foreground">
          {workspaceSlug} / {projectSlug} /{" "}
          {taskData.task_slug || taskData.uuid.slice(0, 8)}
        </div>
        <div className="mt-3 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="min-w-0">
            <h1 className="text-2xl font-semibold tracking-normal">
              {taskData.description}
            </h1>
            <div className="mt-3 flex flex-wrap gap-2">
              <Badge variant="outline">{taskData.status}</Badge>
              {taskData.priority ? (
                <Badge variant="outline">{taskData.priority}</Badge>
              ) : null}
              {taskData.task_slug ? (
                <Badge variant="outline">{taskData.task_slug}</Badge>
              ) : null}
            </div>
          </div>
          <Button asChild variant="outline">
            <Link {...projectLinkProps}>{t("projectReadonly.backToProject")}</Link>
          </Button>
        </div>
      </section>
      <TaskFields task={taskData} t={t} />
      <TaskAnnotations task={taskData} title={t("projectReadonly.annotations")} />
    </div>
  )
}

function TaskUnavailable({
  backLabel,
  linkProps,
  title,
}: {
  backLabel: string
  linkProps: { to: string; params: Record<string, string> }
  title: string
}) {
  return (
    <section className="max-w-2xl border bg-card p-6">
      <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
      <Button asChild className="mt-5" variant="outline">
        <Link {...linkProps}>{backLabel}</Link>
      </Button>
    </section>
  )
}

function TaskFields({
  task,
  t,
}: {
  task: ProjectReadonlyTask
  t: (key: string) => string
}) {
  const fields = [
    [t("common.status"), task.status],
    [t("projectReadonly.priority"), task.priority || "-"],
    [t("projectReadonly.assignee"), assigneeNames(task)],
    [t("projectReadonly.due"), formatUnixDate(task.due)],
    [t("projectReadonly.tags"), task.tags?.join(", ") || "-"],
    [t("projectReadonly.depends"), task.depends?.join(", ") || "-"],
  ] as const

  return (
    <section className="grid gap-2 md:grid-cols-3">
      {fields.map(([label, value]) => (
        <div className="border bg-card p-3" key={label}>
          <div className="text-xs text-muted-foreground">{label}</div>
          <div className="mt-1 text-sm font-medium">{value}</div>
        </div>
      ))}
    </section>
  )
}

function TaskAnnotations({
  task,
  title,
}: {
  task: ProjectReadonlyTask
  title: string
}) {
  if (!task.annotations || task.annotations.length === 0) {
    return null
  }
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{title}</h2>
      <div className="space-y-2">
        {task.annotations.map((annotation, index) => (
          <div className="border bg-card p-3 text-sm" key={annotation.id || index}>
            {annotation.description}
          </div>
        ))}
      </div>
    </section>
  )
}

function TaskDetailSkeleton() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-10 w-96 max-w-full" />
      <div className="grid gap-2 md:grid-cols-3">
        {Array.from({ length: 6 }).map((_, index) => (
          <Skeleton className="h-20" key={index} />
        ))}
      </div>
    </div>
  )
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

function taskBelongsToProject(
  task: ProjectReadonlyTask,
  projectSlug: string
): boolean {
  return !task.project || task.project === projectSlug
}
