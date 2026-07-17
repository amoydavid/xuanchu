import { useState } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import {
  PlayIcon,
  SquareIcon,
  Trash2Icon,
  CheckIcon,
  RotateCcwIcon,
  CopyIcon,
  Link2Icon,
  MoreHorizontalIcon,
  ListPlusIcon,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { ProjectTask } from "../api/task-api"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"
import { useTaskActionMutation } from "../hooks/use-task-mutations"
import { taskDisplayRef } from "../tasks/task-reference"

type TaskActionBarProps = {
  canCreateRelatedContent: boolean
  onAddLink: () => void
  onAddSubTask: () => void
  // 权限层面的可写（不含任务状态判断）。
  permissionCanWrite: boolean
  projectSlug: string
  myTasksReturnSearch?: string
  task: ProjectTask
  taskRef: string
  workspaceSlug: string
}

export function TaskActionBar({
  canCreateRelatedContent,
  onAddLink,
  onAddSubTask,
  permissionCanWrite,
  projectSlug,
  myTasksReturnSearch,
  task,
  taskRef,
  workspaceSlug,
}: TaskActionBarProps) {
  const { i18n, t } = useTranslation()
  const navigate = useNavigate()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const start = useTaskActionMutation(workspaceSlug, projectSlug, "start")
  const stop = useTaskActionMutation(workspaceSlug, projectSlug, "stop")
  const done = useTaskActionMutation(workspaceSlug, projectSlug, "done")
  const reopen = useTaskActionMutation(workspaceSlug, projectSlug, "reopen")
  const remove = useTaskActionMutation(workspaceSlug, projectSlug, "delete")
  const occurrence = task.recurrence_info
  const occurrenceDate = occurrence
    ? new Intl.DateTimeFormat(i18n.language, { dateStyle: "long" }).format(
        new Date(occurrence.recurrence_at * 1000)
      )
    : ""
  const displayRef = taskDisplayRef(task, i18n.language) || taskRef
  const completed = task.status === "completed"
  const deleted = task.status === "deleted"
  const canMutate = permissionCanWrite && !deleted
  const pending =
    start.isPending ||
    stop.isPending ||
    done.isPending ||
    reopen.isPending ||
    remove.isPending

  if ((!permissionCanWrite || deleted) && !occurrence) {
    return null
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {canMutate && !completed && task.start ? (
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
            {occurrence
              ? t("taskSeries.actions.stopOccurrence")
              : t("common.stop")}
          </Button>
        ) : null}
        {canMutate && !completed && !task.start ? (
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
            {occurrence
              ? t("taskSeries.actions.startOccurrence")
              : t("common.start")}
          </Button>
        ) : null}
        {canMutate && !completed ? (
          <Button
            disabled={pending}
            onClick={() => {
              void done.mutateAsync(taskRef)
            }}
            size="sm"
            type="button"
            variant="default"
          >
            <CheckIcon />
            {occurrence
              ? t("taskSeries.actions.completeOccurrence")
              : t("taskDetail.completeTask")}
          </Button>
        ) : null}
        {canMutate && completed ? (
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
            {occurrence
              ? t("taskSeries.actions.reopenOccurrence")
              : t("taskDetail.reopenTask")}
          </Button>
        ) : null}
        <DropdownMenu onOpenChange={setMenuOpen} open={menuOpen}>
          <DropdownMenuTrigger asChild>
            <Button
              aria-label={t("projectWorkbench.project.moreTaskActions", {
                taskRef: displayRef,
              })}
              disabled={pending}
              size="icon-sm"
              type="button"
              variant="ghost"
            >
              <MoreHorizontalIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {occurrence?.series_id && projectSlug ? (
              <DropdownMenuItem asChild>
                <Link
                  params={{
                    workspaceSlug,
                    projectSlug,
                    seriesRef: occurrence.series_id,
                  }}
                  to="/workspaces/$workspaceSlug/projects/$projectSlug/series/$seriesRef"
                >
                  {t("taskSeries.detail.viewSeries")}
                </Link>
              </DropdownMenuItem>
            ) : null}
            <DropdownMenuItem
              onSelect={() => {
                void navigator.clipboard?.writeText(
                  `${window.location.origin}${window.location.pathname}`
                )
              }}
            >
              <CopyIcon />
              {occurrence
                ? t("taskSeries.actions.copyOccurrenceLink")
                : t("projectWorkbench.project.copyTaskLink")}
            </DropdownMenuItem>
            {canCreateRelatedContent ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={onAddLink}>
                  <Link2Icon />
                  {t("taskDetail.addRelatedResource")}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={onAddSubTask}>
                  <ListPlusIcon />
                  {t("taskDetail.addSubTask")}
                </DropdownMenuItem>
              </>
            ) : null}
            {canMutate && !completed ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={(event) => {
                    event.preventDefault()
                    setMenuOpen(false)
                    setConfirmDelete(true)
                  }}
                >
                  <Trash2Icon />
                  {occurrence
                    ? t("taskSeries.actions.skipOccurrence")
                    : t("projectWorkbench.project.deleteTask")}
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <DestructiveConfirmDialog
        confirmLabel={
          occurrence
            ? t("taskSeries.actions.skipOccurrence")
            : t("common.deleteTask")
        }
        description={
          occurrence
            ? t("taskSeries.actions.confirmSkipDescription")
            : t("common.confirmDeleteTaskDescription")
        }
        onConfirm={async () => {
          await remove.mutateAsync(taskRef)
          setConfirmDelete(false)
          if (occurrence && projectSlug) {
            if (myTasksReturnSearch !== undefined) {
              void navigate({
                to: "/my-tasks",
                search: Object.fromEntries(
                  new URLSearchParams(myTasksReturnSearch)
                ),
              })
              return
            }
            void navigate({
              to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks",
              params: { workspaceSlug, projectSlug },
            })
          }
        }}
        onOpenChange={setConfirmDelete}
        open={confirmDelete}
        pending={remove.isPending}
        title={
          occurrence
            ? t("taskSeries.actions.confirmSkipTitle", { date: occurrenceDate })
            : t("common.confirmDeleteTaskTitle")
        }
      />
    </>
  )
}
