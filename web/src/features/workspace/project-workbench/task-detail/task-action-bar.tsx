import { useState } from "react"
import { PlayIcon, SquareIcon, Trash2Icon, CheckIcon, RotateCcwIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { ProjectTask } from "../api/task-api"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"
import { useTaskActionMutation } from "../hooks/use-task-mutations"

type TaskActionBarProps = {
  // 权限层面的可写（不含任务状态判断）。
  permissionCanWrite: boolean
  projectSlug: string
  task: ProjectTask
  taskRef: string
  workspaceSlug: string
}

export function TaskActionBar({
  permissionCanWrite,
  projectSlug,
  task,
  taskRef,
  workspaceSlug,
}: TaskActionBarProps) {
  const [confirmDelete, setConfirmDelete] = useState(false)
  const start = useTaskActionMutation(workspaceSlug, projectSlug, "start")
  const stop = useTaskActionMutation(workspaceSlug, projectSlug, "stop")
  const done = useTaskActionMutation(workspaceSlug, projectSlug, "done")
  const reopen = useTaskActionMutation(workspaceSlug, projectSlug, "reopen")
  const remove = useTaskActionMutation(workspaceSlug, projectSlug, "delete")
  const completed = task.status === "completed"
  const deleted = task.status === "deleted"
  const pending =
    start.isPending ||
    stop.isPending ||
    done.isPending ||
    reopen.isPending ||
    remove.isPending

  // deleted 是真正的终态，无可执行动作；无权限也直接隐藏。
  if (deleted || !permissionCanWrite) {
    return null
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {!completed && task.start ? (
          <Button
            disabled={pending}
            onClick={() => {
              void stop.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <SquareIcon />
            停止
          </Button>
        ) : null}
        {!completed && !task.start ? (
          <Button
            disabled={pending}
            onClick={() => {
              void start.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <PlayIcon />
            开始
          </Button>
        ) : null}
        {!completed ? (
          <Button
            disabled={pending}
            onClick={() => {
              void done.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <CheckIcon />
            完成
          </Button>
        ) : null}
        {completed ? (
          <Button
            disabled={pending}
            onClick={() => {
              void reopen.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            <RotateCcwIcon />
            重新打开
          </Button>
        ) : (
          <Button
            disabled={pending}
            onClick={() => setConfirmDelete(true)}
            size="sm"
            type="button"
            variant="destructive"
          >
            <Trash2Icon />
            删除
          </Button>
        )}
      </div>
      <DestructiveConfirmDialog
        confirmLabel="删除任务"
        description="删除后任务会从当前项目视图中移除。这个操作不可撤销。"
        onConfirm={async () => {
          await remove.mutateAsync(taskRef)
          setConfirmDelete(false)
        }}
        onOpenChange={setConfirmDelete}
        open={confirmDelete}
        pending={remove.isPending}
        title="确认删除任务"
      />
    </>
  )
}
