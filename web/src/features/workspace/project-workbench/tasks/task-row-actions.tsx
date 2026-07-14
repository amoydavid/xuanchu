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
import type { ProjectWorkbenchTask } from "../api/project-api"
import { formatTaskSeriesTimestamp } from "../task-series/recurrence-preview"

type TaskRowActionsProps = {
  canWrite: boolean
  displayRef: string
  myTasksReturnSearch?: string
  onNavigateFromList?: () => void
  projectSlug: string
  recurrenceInfo?: ProjectWorkbenchTask["recurrence_info"]
  start?: string | number | null
  status: string
  taskRef: string
  workspaceSlug: string
}

export function TaskRowActions({
  canWrite,
  displayRef,
  myTasksReturnSearch,
  onNavigateFromList,
  projectSlug,
  recurrenceInfo,
  start,
  status,
  taskRef,
  workspaceSlug,
}: TaskRowActionsProps) {
  const { i18n, t } = useTranslation()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const startMutation = useTaskActionMutation(workspaceSlug, projectSlug, "start")
  const stopMutation = useTaskActionMutation(workspaceSlug, projectSlug, "stop")
  const doneMutation = useTaskActionMutation(workspaceSlug, projectSlug, "done")
  const reopenMutation = useTaskActionMutation(workspaceSlug, projectSlug, "reopen")
  const remove = useTaskActionMutation(workspaceSlug, projectSlug, "delete")
  const isCompleted = status === "completed" || status === "deleted"
  const isOpen = status === "pending" || status === "waiting"
  const canStartOrStop = status === "pending"
  const isStarted = status === "pending" && start !== undefined && start !== null
  const occurrenceDate = recurrenceInfo
    ? formatTaskSeriesTimestamp(recurrenceInfo.recurrence_at, i18n.language)
    : ""

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
      {canWrite && canStartOrStop ? (
        <>
          {isStarted ? (
            <Button
              aria-label={
                recurrenceInfo
                  ? t("taskSeries.aria.stopOccurrence", { date: occurrenceDate })
                  : t("projectWorkbench.project.stopTask", {
                      taskRef: displayRef,
                    })
              }
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
              aria-label={
                recurrenceInfo
                  ? t("taskSeries.aria.startOccurrence", { date: occurrenceDate })
                  : t("projectWorkbench.project.startTask", {
                      taskRef: displayRef,
                    })
              }
              disabled={startMutation.isPending}
              onClick={() => runAction("start")}
              size="icon-xs"
              type="button"
              variant="ghost"
            >
              <Play />
            </Button>
          )}
        </>
      ) : null}
      {canWrite && isOpen ? (
        <Button
          aria-label={
            recurrenceInfo
              ? t("taskSeries.aria.completeOccurrence", {
                  date: occurrenceDate,
                })
              : t("projectWorkbench.project.completeTask", {
                  taskRef: displayRef,
                })
          }
          disabled={doneMutation.isPending}
          onClick={() => runAction("done")}
          size="icon-xs"
          type="button"
          variant="ghost"
        >
          <Check />
        </Button>
      ) : null}
      {canWrite && status === "completed" ? (
        <Button
          aria-label={
            recurrenceInfo
              ? t("taskSeries.aria.reopenOccurrence", { date: occurrenceDate })
              : t("projectWorkbench.project.reopenTask", {
                  taskRef: displayRef,
                })
          }
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
              taskRef: displayRef,
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
              onClick={onNavigateFromList}
              params={{ workspaceSlug, projectSlug, taskRef }}
              search={
                myTasksReturnSearch === undefined
                  ? undefined
                  : {
                      from: "my-tasks",
                      my_tasks_search: myTasksReturnSearch,
                    }
              }
              to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
            >
              {t("projectWorkbench.project.openTaskDetails")}
            </Link>
          </DropdownMenuItem>
          {recurrenceInfo?.series_id ? (
            <DropdownMenuItem asChild>
              <Link
                onClick={onNavigateFromList}
                params={{
                  workspaceSlug,
                  projectSlug,
                  seriesRef: recurrenceInfo.series_id,
                }}
                search={
                  myTasksReturnSearch === undefined
                    ? undefined
                    : {
                        panel_return_scope: "project",
                        panel_return_search: myTasksReturnSearch,
                        panel_return_source: "my-tasks",
                        panel_return_task: taskRef,
                      }
                }
                to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/series/$seriesRef"
              >
                {t("taskSeries.detail.viewSeries")}
              </Link>
            </DropdownMenuItem>
          ) : null}
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
                {recurrenceInfo
                  ? t("taskSeries.actions.skipOccurrence")
                  : t("projectWorkbench.project.deleteTask")}
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        confirmLabel={
          recurrenceInfo
            ? t("taskSeries.actions.skipOccurrence")
            : t("projectWorkbench.project.delete")
        }
        description={
          recurrenceInfo
            ? t("taskSeries.actions.confirmSkipDescription")
            : t("projectWorkbench.project.deleteTaskDescription", {
                taskRef: displayRef,
              })
        }
        onConfirm={async () => {
          await remove.mutateAsync(taskRef)
          setConfirmDelete(false)
        }}
        onOpenChange={setConfirmDelete}
        open={confirmDelete}
        pending={remove.isPending}
        title={
          recurrenceInfo
            ? t("taskSeries.actions.confirmSkipTitle", { date: occurrenceDate })
            : t("projectWorkbench.project.deleteTaskTitle")
        }
      />
    </div>
  )
}
