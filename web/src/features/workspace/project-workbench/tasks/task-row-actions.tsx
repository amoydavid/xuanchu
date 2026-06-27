import { useState } from "react"
import { Link } from "@tanstack/react-router"
import { Check, Copy, MoreHorizontal, Play, Square, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"
import {
  useTaskActionMutation,
  type TaskAction,
} from "../hooks/use-task-mutations"

type TaskRowActionsProps = {
  canWrite: boolean
  projectSlug: string
  start?: string | number | null
  status: string
  taskRef: string
  workspaceSlug: string
}

export function TaskRowActions({
  canWrite,
  projectSlug,
  start,
  status,
  taskRef,
  workspaceSlug,
}: TaskRowActionsProps) {
  const [confirmDelete, setConfirmDelete] = useState(false)
  const startMutation = useTaskActionMutation(workspaceSlug, projectSlug, "start")
  const stopMutation = useTaskActionMutation(workspaceSlug, projectSlug, "stop")
  const doneMutation = useTaskActionMutation(workspaceSlug, projectSlug, "done")
  const remove = useTaskActionMutation(workspaceSlug, projectSlug, "delete")
  const isCompleted = status === "completed" || status === "deleted"
  const isStarted = status === "pending" && start !== undefined && start !== null

  const runAction = (action: TaskAction) => {
    if (action === "start") {
      void startMutation.mutateAsync(taskRef)
      return
    }
    if (action === "stop") {
      void stopMutation.mutateAsync(taskRef)
      return
    }
    if (action === "done") {
      void doneMutation.mutateAsync(taskRef)
    }
  }

  return (
    <div className="flex items-center justify-end gap-1">
      {canWrite && !isCompleted ? (
        <>
          {isStarted ? (
            <Button
              aria-label={`停止 ${taskRef}`}
              disabled={stopMutation.isPending}
              onClick={() => runAction("stop")}
              size="icon-xs"
              type="button"
              variant="ghost"
            >
              <Square />
            </Button>
          ) : (
            <Button
              aria-label={`开始 ${taskRef}`}
              disabled={startMutation.isPending}
              onClick={() => runAction("start")}
              size="icon-xs"
              type="button"
              variant="ghost"
            >
              <Play />
            </Button>
          )}
          <Button
            aria-label={`完成 ${taskRef}`}
            disabled={doneMutation.isPending}
            onClick={() => runAction("done")}
            size="icon-xs"
            type="button"
            variant="ghost"
          >
            <Check />
          </Button>
        </>
      ) : null}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={`更多操作 ${taskRef}`}
            size="icon-xs"
            type="button"
            variant="ghost"
          >
            <MoreHorizontal />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem asChild>
            <Link
              params={{ workspaceSlug, projectSlug, taskRef }}
              to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
            >
              打开详情
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => {
              const path = `/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks/${taskRef}`
              void navigator.clipboard?.writeText(`${window.location.origin}${path}`)
            }}
          >
            <Copy />
            复制任务链接
          </DropdownMenuItem>
          {canWrite ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onSelect={(event) => {
                  event.preventDefault()
                  setConfirmDelete(true)
                }}
              >
                <Trash2 />
                删除任务
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        confirmLabel="删除"
        description={`删除任务 ${taskRef} 后不可恢复。`}
        onConfirm={async () => {
          await remove.mutateAsync(taskRef)
          setConfirmDelete(false)
        }}
        onOpenChange={setConfirmDelete}
        open={confirmDelete}
        pending={remove.isPending}
        title="确认删除任务"
      />
    </div>
  )
}
