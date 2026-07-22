import { useState } from "react"
import { MoreHorizontalIcon } from "lucide-react"
import { useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { ProjectStatus } from "../api/project-api"
import { useTransitionProjectMutation } from "../hooks/use-project-mutations"
import { isClosedProjectStatus } from "../project/project-status-menu"
import { DestructiveConfirmDialog } from "../shared/destructive-confirm-dialog"

type ProjectRowActionsProps = {
  canManage: boolean
  onOpen: (projectSlug: string) => void
  projectSlug: string
  status: string
  workspaceSlug: string
}

export function ProjectRowActions({
  canManage,
  onOpen,
  projectSlug,
  status,
  workspaceSlug,
}: ProjectRowActionsProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [confirmStatus, setConfirmStatus] = useState<ProjectStatus | null>(null)
  const transition = useTransitionProjectMutation(workspaceSlug, projectSlug)
  const closed = isClosedProjectStatus(status)

  function gotoSettings() {
    void navigate({
      to: "/projects/$projectSlug/settings/config",
      params: { projectSlug },
    })
  }

  return (
    <div onClick={(event) => event.stopPropagation()}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={t("projectWorkbench.projects.actionsLabel")}
            size="icon-sm"
            variant="ghost"
          >
            <MoreHorizontalIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => onOpen(projectSlug)}>
            {t("projectWorkbench.projects.open")}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={gotoSettings}>
            {t("projectSettings.settingsLink")}
          </DropdownMenuItem>
          {canManage ? (
            <>
              <DropdownMenuSeparator />
              {!closed ? (
                <DropdownMenuItem
                  disabled={transition.isPending}
                  onSelect={(event) => {
                    event.preventDefault()
                    setConfirmStatus("archived")
                  }}
                >
                  {t("projectSettings.archive")}
                </DropdownMenuItem>
              ) : null}
              {!closed ? (
                <DropdownMenuItem
                  disabled={transition.isPending}
                  onSelect={(event) => {
                    event.preventDefault()
                    setConfirmStatus("cancelled")
                  }}
                >
                  {t("projectSettings.cancel")}
                </DropdownMenuItem>
              ) : null}
              {closed ? (
                <DropdownMenuItem
                  disabled={transition.isPending}
                  onSelect={() => transition.mutate("active")}
                >
                  {t("projectSettings.restoreActive")}
                </DropdownMenuItem>
              ) : null}
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        confirmLabel={t("projectWorkbench.project.closeProject")}
        description={t("projectWorkbench.project.closeProjectDescription")}
        onConfirm={async () => {
          if (!confirmStatus) return
          await transition.mutateAsync(confirmStatus)
          setConfirmStatus(null)
        }}
        onOpenChange={(open) => {
          if (!open) setConfirmStatus(null)
        }}
        open={confirmStatus !== null}
        pending={transition.isPending}
        title={t("projectWorkbench.project.closeProjectTitle")}
      />
    </div>
  )
}
