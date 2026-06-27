import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useMe } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"
import type { ProjectTask } from "../api/task-api"
import { useModifyTaskMutation } from "../hooks/use-task-mutations"
import { useTaskDetailQuery } from "../hooks/use-task-detail-data"
import { canTaskWrite } from "../permissions/permissions"
import { InlineTextEditor } from "../shared/inline-text-editor"
import { TaskActionBar } from "./task-action-bar"
import { TaskAnnotationsEditor } from "./task-annotations-editor"
import { TaskLinksEditor } from "./task-links-editor"
import { TaskPropertyPanel } from "./task-property-panel"

type TaskDetailPageProps = {
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

export function TaskDetailPage({
  projectSlug,
  taskRef,
  workspaceSlug,
}: TaskDetailPageProps) {
  const me = useMe()
  const canWrite = canTaskWrite({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const task = useTaskDetailQuery(workspaceSlug, taskRef)
  const modifyTask = useModifyTaskMutation(workspaceSlug, projectSlug, taskRef)
  const projectHref = `/workspaces/${workspaceSlug}/projects/${projectSlug}`

  if (task.isPending) {
    return <TaskDetailSkeleton />
  }

  if (task.isError) {
    const title =
      task.error instanceof ApiError && task.error.status === 404
        ? "任务不存在或不可见"
        : "加载失败"
    return (
      <section className="max-w-2xl border bg-card p-6">
        <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
        <p className="mt-3 text-sm text-muted-foreground">
          {task.error instanceof ApiError ? task.error.code : "unknown"}
        </p>
        <Button asChild className="mt-5" variant="outline">
          <a href={projectHref}>返回项目</a>
        </Button>
      </section>
    )
  }

  const taskData = task.data
  if (!taskBelongsToProject(taskData, projectSlug)) {
    return (
      <section className="max-w-2xl border bg-card p-6">
        <h1 className="text-xl font-semibold tracking-normal">
          任务不存在或不可见
        </h1>
        <Button asChild className="mt-5" variant="outline">
          <a href={projectHref}>返回项目</a>
        </Button>
      </section>
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
          <div className="min-w-0 flex-1">
            <InlineTextEditor
              ariaLabel="任务标题"
              disabled={!canWrite || taskData.status === "completed"}
              displayClassName="text-2xl font-semibold tracking-normal"
              onSave={async (title) => {
                await modifyTask.mutateAsync({ title })
              }}
              validate={(value) => (value.trim() ? null : "任务标题不能为空")}
              value={taskData.title}
            />
            <InlineTextEditor
              ariaLabel="任务描述"
              disabled={!canWrite || taskData.status === "completed"}
              displayClassName="mt-3 max-w-3xl whitespace-pre-wrap text-sm leading-6 text-muted-foreground"
              emptyLabel="添加任务描述"
              multiline
              onSave={async (description) => {
                await modifyTask.mutateAsync(
                  description
                    ? { description }
                    : { clear_description: true }
                )
              }}
              value={taskData.description ?? ""}
            />
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
          <div className="flex shrink-0 flex-col items-start gap-2 md:items-end">
            <Button asChild variant="outline">
              <a href={projectHref}>返回项目</a>
            </Button>
            <TaskActionBar
              canWrite={canWrite}
              projectSlug={projectSlug}
              task={taskData}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
          </div>
        </div>
      </section>

      <div className="grid gap-5 md:grid-cols-[1fr_240px]">
        <div className="space-y-5">
          <TaskAnnotationsEditor
            annotations={taskData.annotations}
            canWrite={canWrite}
            projectSlug={projectSlug}
            taskRef={taskRef}
            workspaceSlug={workspaceSlug}
          />
          <TaskLinksEditor
            canWrite={canWrite}
            links={taskData.links}
            projectSlug={projectSlug}
            taskRef={taskRef}
            workspaceSlug={workspaceSlug}
          />
        </div>
        <TaskPropertyPanel
          canWrite={canWrite}
          projectSlug={projectSlug}
          task={taskData}
          taskRef={taskRef}
          workspaceSlug={workspaceSlug}
        />
      </div>
    </div>
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

function taskBelongsToProject(task: ProjectTask, projectSlug: string): boolean {
  return !task.project || task.project === projectSlug
}
