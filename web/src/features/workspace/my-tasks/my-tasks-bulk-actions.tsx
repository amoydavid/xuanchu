import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { CheckCheckIcon, Trash2Icon, XIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { ProjectWorkbenchTask } from "@/features/workspace/project-workbench/api/project-api"
import {
  deleteTask,
  doneTask,
  modifyTask,
  type TaskModifyInput,
} from "@/features/workspace/project-workbench/api/task-api"
import { projectQueryKeys } from "@/features/workspace/project-workbench/hooks/use-project-data"
import { AssigneePicker } from "@/features/workspace/project-workbench/task-detail/assignee-picker"
import { InlineDatePicker } from "@/features/workspace/project-workbench/shared/inline-date-picker"
import { useEditFeedback } from "@/features/workspace/project-workbench/shared/edit-feedback"
import { taskRouteRef } from "@/features/workspace/project-workbench/tasks/task-reference"

type MyTasksBulkActionsProps = {
  onRemoveSucceeded: (ids: string[]) => void
  selectedTasks: ProjectWorkbenchTask[]
  workspaceSlug: string
}

type BulkOperationResult = {
  failed: number
  succeededIds: string[]
}

export function MyTasksBulkActions({
  onRemoveSucceeded,
  selectedTasks,
  workspaceSlug,
}: MyTasksBulkActionsProps) {
  const { t } = useTranslation()
  const feedback = useEditFeedback()
  const queryClient = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const selectedCount = selectedTasks.length
  const openTasks = selectedTasks.filter(
    (task) => task.status === "pending" || task.status === "waiting"
  )
  const normalCount = selectedTasks.filter(
    (task) => !task.recurrence_info
  ).length
  const occurrenceCount = selectedCount - normalCount

  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: ["my-tasks", workspaceSlug],
    })
    const projects = new Set(
      selectedTasks.map((task) => task.project).filter(Boolean) as string[]
    )
    for (const projectSlug of projects) {
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTasksPrefix(
          workspaceSlug,
          projectSlug
        ),
      })
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.project(workspaceSlug, projectSlug),
      })
    }
  }

  const run = async (
    tasks: ProjectWorkbenchTask[],
    operation: (taskRef: string) => Promise<unknown>,
    successMessage: string,
    removeSucceeded = false
  ): Promise<BulkOperationResult> => {
    if (tasks.length === 0 || busy) return { failed: 0, succeededIds: [] }
    setBusy(true)
    try {
      const results = await Promise.allSettled(
        tasks.map((task) => operation(taskRouteRef(task)))
      )
      const succeededIds = tasks
        .filter((_task, index) => results[index]?.status === "fulfilled")
        .map(taskStableID)
      const failed = results.length - succeededIds.length
      if (succeededIds.length > 0) {
        invalidate()
        feedback.success(successMessage)
        if (removeSucceeded) onRemoveSucceeded(succeededIds)
      }
      if (failed > 0) {
        feedback.failure(
          t("myTasks.bulk.failureTitle"),
          t("myTasks.bulk.failureDescription", {
            failed,
            total: results.length,
          })
        )
      }
      return { failed, succeededIds }
    } finally {
      setBusy(false)
    }
  }

  const modifySelected = (input: TaskModifyInput, successMessage: string) =>
    run(
      selectedTasks,
      (taskRef) => modifyTask(workspaceSlug, taskRef, input),
      successMessage
    )

  if (selectedCount === 0) return null

  return (
    <>
      <div
        aria-label={t("myTasks.bulk.toolbarLabel")}
        className="rounded-lg flex flex-wrap items-center gap-2 border bg-muted/30 p-2"
        role="toolbar"
      >
        <span className="min-w-24 text-sm font-medium">
          {t("myTasks.bulk.selected", { count: selectedCount })}
        </span>
        <Button
          aria-label={t("myTasks.bulk.completeAria", {
            count: openTasks.length,
          })}
          disabled={busy || openTasks.length === 0}
          onClick={() =>
            void run(
              openTasks,
              (taskRef) => doneTask(workspaceSlug, taskRef),
              t("myTasks.bulk.completed", { count: openTasks.length }),
              true
            )
          }
          size="sm"
          type="button"
          variant="outline"
        >
          <CheckCheckIcon data-icon="inline-start" />
          {t("myTasks.bulk.complete")}
        </Button>
        <Select
          disabled={busy}
          onValueChange={(value) =>
            void modifySelected(
              value === "none"
                ? { clear_priority: true }
                : { priority: value },
              t("myTasks.bulk.priorityUpdated", { count: selectedCount })
            )
          }
        >
          <SelectTrigger
            aria-label={t("myTasks.bulk.priorityAria", {
              count: selectedCount,
            })}
            className="h-8 w-32"
            size="sm"
          >
            <SelectValue placeholder={t("myTasks.bulk.priority")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="H">H</SelectItem>
            <SelectItem value="M">M</SelectItem>
            <SelectItem value="L">L</SelectItem>
            <SelectItem value="none">{t("myTasks.bulk.noPriority")}</SelectItem>
          </SelectContent>
        </Select>
        <div className="w-36">
          <InlineDatePicker
            ariaLabel={t("myTasks.bulk.dueAria", { count: selectedCount })}
            boundary="end"
            disabled={busy}
            emptyLabel={t("myTasks.bulk.due")}
            onSave={async (due) => {
              await modifySelected(
                due === null ? { clear_due: true } : { due },
                t("myTasks.bulk.dueUpdated", { count: selectedCount })
              )
            }}
          />
        </div>
        <div className="flex items-center gap-1">
          <span className="text-xs text-muted-foreground">
            {t("myTasks.bulk.assignees")}
          </span>
          <AssigneePicker
            disabled={busy}
            onSave={async (assignees) => {
              await modifySelected(
                assignees.length === 0
                  ? { clear_assignees: true }
                  : { assignees },
                t("myTasks.bulk.assigneesUpdated", { count: selectedCount })
              )
            }}
            workspaceSlug={workspaceSlug}
          />
        </div>
        <Button
          aria-label={t("myTasks.bulk.deleteAria", { count: selectedCount })}
          disabled={busy}
          onClick={() => setDeleteOpen(true)}
          size="sm"
          type="button"
          variant="destructive"
        >
          <Trash2Icon data-icon="inline-start" />
          {t("myTasks.bulk.delete")}
        </Button>
        <Button
          aria-label={t("myTasks.bulk.clearSelection")}
          className="ml-auto"
          disabled={busy}
          onClick={() => onRemoveSucceeded(selectedTasks.map(taskStableID))}
          size="icon-sm"
          type="button"
          variant="ghost"
        >
          <XIcon />
        </Button>
      </div>

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("myTasks.bulk.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("myTasks.bulk.deleteDescription", {
                normal: normalCount,
                occurrence: occurrenceCount,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              aria-label={t("myTasks.bulk.deleteConfirm")}
              disabled={busy}
              onClick={(event) => {
                event.preventDefault()
                void run(
                  selectedTasks,
                  (taskRef) => deleteTask(workspaceSlug, taskRef),
                  t("myTasks.bulk.deleted", { count: selectedCount }),
                  true
                ).then(() => setDeleteOpen(false))
              }}
              variant="destructive"
            >
              {t("myTasks.bulk.deleteConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function taskStableID(task: ProjectWorkbenchTask): string {
  return task.id || task.uuid || ""
}
