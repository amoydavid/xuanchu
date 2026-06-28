import { useState } from "react"
import { CheckIcon, ChevronDownIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import type {
  ProjectStatus,
  ProjectWorkbenchProject,
} from "../api/project-api"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"
import { useTransitionProjectMutation } from "../hooks/use-project-mutations"

const STATUSES: ProjectStatus[] = [
  "planning",
  "active",
  "archived",
  "cancelled",
]

type ProjectStatusMenuProps = {
  canManage: boolean
  project: ProjectWorkbenchProject
  workspaceSlug: string
}

export function ProjectStatusMenu({
  canManage,
  project,
  workspaceSlug,
}: ProjectStatusMenuProps) {
  const { t } = useTranslation()
  const [confirmStatus, setConfirmStatus] = useState<ProjectStatus | null>(null)
  const transition = useTransitionProjectMutation(workspaceSlug, project.slug)
  const closed = isClosedProjectStatus(project.status)
  const currentStatusLabel = taskStatusLabel(project.status, t)

  const submit = async (status: ProjectStatus) => {
    await transition.mutateAsync(status)
  }

  if (!canManage) {
    return (
      <Button
        aria-label={t("projectWorkbench.project.statusLabel")}
        disabled
        size="sm"
        variant="outline"
      >
        {currentStatusLabel}
      </Button>
    )
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={t("projectWorkbench.project.statusAria", {
              status: currentStatusLabel,
            })}
            size="sm"
            variant="outline"
          >
            {currentStatusLabel}
            <ChevronDownIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-44">
          {closed ? (
            <DropdownMenuLabel>
              {t("projectWorkbench.project.reopenWritableHint")}
            </DropdownMenuLabel>
          ) : null}
          {STATUSES.map((status) => (
            <DropdownMenuItem
              disabled={transition.isPending || status === project.status}
              key={status}
              onSelect={(event) => {
                event.preventDefault()
                if (isClosedProjectStatus(status) && !closed) {
                  setConfirmStatus(status)
                  return
                }
                void submit(status)
              }}
            >
              {taskStatusLabel(status, t)}
              {status === project.status ? <CheckIcon className="ml-auto" /> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        confirmLabel={t("projectWorkbench.project.closeProject")}
        description={t("projectWorkbench.project.closeProjectDescription")}
        onConfirm={async () => {
          if (!confirmStatus) {
            return
          }
          await submit(confirmStatus)
          setConfirmStatus(null)
        }}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmStatus(null)
          }
        }}
        open={confirmStatus !== null}
        pending={transition.isPending}
        title={t("projectWorkbench.project.closeProjectTitle")}
      />
    </>
  )
}

export function isClosedProjectStatus(status: string | null | undefined) {
  return status === "archived" || status === "cancelled"
}
