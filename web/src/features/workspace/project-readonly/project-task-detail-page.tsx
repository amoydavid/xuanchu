import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"
import {
  getProjectReadonlyAnnotations,
  getProjectReadonlyTask,
  type ProjectReadonlyTask,
  type ProjectReadonlyTaskLink,
} from "./project-readonly-api"
import { extractUDAs, formatUDAValue } from "./uda"

type ProjectTaskDetailPageProps = {
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

// INITIAL_ANNOTATIONS 控制首屏从任务详情里直接渲染的注解条数，
// 超出部分通过「查看更多」懒加载 GET /tasks/{ref}/annotations。
const INITIAL_ANNOTATIONS = 3
const ANNOTATION_PAGE_SIZE = 10

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

      <div className="grid gap-5 md:grid-cols-[1fr_220px]">
        <div className="space-y-5">
          <TaskAnnotationsLazy
            task={taskData}
            workspaceSlug={workspaceSlug}
            taskRef={taskRef}
            title={t("projectReadonly.annotations")}
          />
          <TaskLinks links={taskData.links} title={t("projectReadonly.links")} />
        </div>
        <TaskSidePanel task={taskData} t={t} />
      </div>
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

// TaskAnnotationsLazy 首屏渲染任务内嵌注解的前 INITIAL_ANNOTATIONS 条，
// 超出部分通过「查看更多」调用分页端点懒加载。
function TaskAnnotationsLazy({
  task,
  workspaceSlug,
  taskRef,
  title,
}: {
  task: ProjectReadonlyTask
  workspaceSlug: string
  taskRef: string
  title: string
}) {
  const all = task.annotations ?? []
  const [shown, setShown] = useState(all.slice(0, INITIAL_ANNOTATIONS))
  const [offset, setOffset] = useState(Math.min(all.length, INITIAL_ANNOTATIONS))
  const [loading, setLoading] = useState(false)
  const total = all.length
  const hasMore = offset < total

  if (total === 0) {
    return null
  }

  const loadMore = async () => {
    setLoading(true)
    try {
      const page = await getProjectReadonlyAnnotations(
        workspaceSlug,
        taskRef,
        offset,
        ANNOTATION_PAGE_SIZE
      )
      setShown((prev) => [...prev, ...page.annotations])
      setOffset((prev) => prev + page.annotations.length)
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">
        {title}（{shown.length} / {total}）
      </h2>
      <div className="space-y-2">
        {shown.map((annotation, index) => (
          <div className="border bg-card p-3 text-sm" key={annotation.id || index}>
            <div>{annotation.description}</div>
            {annotation.entry ? (
              <div className="mt-1 text-xs text-muted-foreground">{annotation.entry}</div>
            ) : null}
          </div>
        ))}
      </div>
      {hasMore ? (
        <Button
          onClick={loadMore}
          disabled={loading}
          size="sm"
          variant="ghost"
        >
          {loading
            ? "..."
            : `查看更多 ${total - shown.length} 条 ↓`}
        </Button>
      ) : null}
    </section>
  )
}

function TaskLinks({
  links,
  title,
}: {
  links?: ProjectReadonlyTaskLink[]
  title: string
}) {
  if (!links || links.length === 0) {
    return null
  }
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{title}</h2>
      <ul className="space-y-1">
        {links.map((link) => (
          <li className="text-sm" key={link.id}>
            <a
              className="text-primary underline-offset-4 hover:underline"
              href={link.url}
              rel="noreferrer"
              target="_blank"
            >
              {link.title || link.url}
            </a>
            <span className="text-muted-foreground">
              {" · "}
              {link.type}
              {link.created_by?.name ? ` · ${link.created_by.name}` : ""}
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

// TaskSidePanel 是右侧属性栏：常用字段 + UDAs。
function TaskSidePanel({
  task,
  t,
}: {
  task: ProjectReadonlyTask
  t: (key: string) => string
}) {
  const udas = extractUDAs(task)
  const fields: Array<[string, string]> = [
    [t("common.status"), task.status],
    [t("projectReadonly.priority"), task.priority || "-"],
    [t("projectReadonly.assignee"), assigneeNames(task)],
    [t("projectReadonly.due"), formatUnixDate(task.due)],
    [t("projectReadonly.tags"), task.tags?.join(", ") || "-"],
    [t("projectReadonly.depends"), task.depends?.join(", ") || "-"],
    [t("projectReadonly.entry"), formatRFCDate(task.entry)],
    [t("projectReadonly.modified"), formatRFCDate(task.modified)],
    ...(task.recur ? [[t("projectReadonly.recur"), task.recur] as [string, string]] : []),
  ]

  return (
    <aside className="space-y-3 border bg-card p-4 text-sm">
      <h2 className="text-xs font-medium uppercase text-muted-foreground">
        {t("projectReadonly.attributes")}
      </h2>
      {fields.map(([label, value]) => (
        <div key={label}>
          <div className="text-xs text-muted-foreground">{label}</div>
          <div className="font-medium">{value}</div>
        </div>
      ))}
      {udas.length > 0 ? (
        <>
          <Separator />
          <h2 className="text-xs font-medium uppercase text-muted-foreground">
            {t("projectReadonly.customFields")}
          </h2>
          {udas.map(([key, value]) => (
            <div key={key}>
              <div className="text-xs text-muted-foreground">{key}</div>
              <div className="font-medium">{formatUDAValue(value)}</div>
            </div>
          ))}
        </>
      ) : null}
    </aside>
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

// formatUnixDate 处理后端 due 字段（unix 秒，number）。
function formatUnixDate(value?: number | null): string {
  if (typeof value !== "number") {
    return "-"
  }
  return new Date(value * 1000).toISOString().slice(0, 10)
}

// formatRFCDate 处理后端 entry/modified（RFC3339 字符串），只取日期部分。
function formatRFCDate(value?: string): string {
  if (!value) {
    return "-"
  }
  return value.slice(0, 10)
}

function taskBelongsToProject(
  task: ProjectReadonlyTask,
  projectSlug: string
): boolean {
  return !task.project || task.project === projectSlug
}
