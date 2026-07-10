import { useState } from "react"
import { Link } from "@tanstack/react-router"
import { Check, Copy, MoreHorizontal, Play, RotateCcw, Square, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

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
  const { t } = useTranslation()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const startMutation = useTaskActionMutation(workspaceSlug, projectSlug, "start")
  const stopMutation = useTaskActionMutation(workspaceSlug, projectSlug, "stop")
  const doneMutation = useTaskActionMutation(workspaceSlug, projectSlug, "done")
  const reopenMutation = useTaskActionMutation(workspaceSlug, projectSlug, "reopen")
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
      return
    }
    if (action === "reopen") {
      void reopenMutation.mutateAsync(taskRef)
    }
  }

  return (
    <div className="flex items-center justify-end gap-1">
      {canWrite && !isCompleted ? (
        <>
          {isStarted ? (
            <Button
              aria-label={t("projectWorkbench.project.stopTask", { taskRef })}
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
              aria-label={t("projectWorkbench.project.startTask", { taskRef })}
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
            aria-label={t("projectWorkbench.project.completeTask", { taskRef })}
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
      {canWrite && status === "completed" ? (
        <Button
          aria-label={t("projectWorkbench.project.reopenTask", { taskRef })}
          disabled={reopenMutation.isPending}
          onClick={() => runAction("reopen")}
          size="icon-xs"
          type="button"
          variant="ghost"
        >
          <RotateCcw />
        </Button>
      ) : null}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={t("projectWorkbench.project.moreTaskActions", {
              taskRef,
            })}
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
              {t("projectWorkbench.project.openTaskDetails")}
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => {
              const path = `/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks/${taskRef}`
              void navigator.clipboard?.writeText(`${window.location.origin}${path}`)
            }}
          >
            <Copy />
            {t("projectWorkbench.project.copyTaskLink")}
          </DropdownMenuItem>
          {canWrite && !isCompleted ? (
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
                {t("projectWorkbench.project.deleteTask")}
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        confirmLabel={t("projectWorkbench.project.delete")}
        description={t("projectWorkbench.project.deleteTaskDescription", {
          taskRef,
        })}
        onConfirm={async () => {
          await remove.mutateAsync(taskRef)
          setConfirmDelete(false)
        }}
        onOpenChange={setConfirmDelete}
        open={confirmDelete}
        pending={remove.isPending}
        title={t("projectWorkbench.project.deleteTaskTitle")}
      />
    </div>
  )
}
