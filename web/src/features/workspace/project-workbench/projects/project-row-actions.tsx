import { useMutation, useQueryClient } from "@tanstack/react-query"
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
import { transitionProject } from "../api/project-api"

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
  const queryClient = useQueryClient()
  const transition = useMutation({
    mutationFn: (next: string) =>
      transitionProject(workspaceSlug, projectSlug, next),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["projects"] })
    },
  })

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
              {status !== "archived" ? (
                <DropdownMenuItem
                  onSelect={() => {
                    if (
                      window.confirm(
                        t("projectSettings.transitionConfirm", {
                          status: "archived",
                        })
                      )
                    ) {
                      transition.mutate("archived")
                    }
                  }}
                >
                  {t("projectSettings.archive", { defaultValue: "归档" })}
                </DropdownMenuItem>
              ) : null}
              {status !== "cancelled" ? (
                <DropdownMenuItem
                  onSelect={() => {
                    if (
                      window.confirm(
                        t("projectSettings.transitionConfirm", {
                          status: "cancelled",
                        })
                      )
                    ) {
                      transition.mutate("cancelled")
                    }
                  }}
                >
                  {t("projectSettings.cancel", { defaultValue: "取消" })}
                </DropdownMenuItem>
              ) : null}
              {status === "archived" || status === "cancelled" ? (
                <DropdownMenuItem onSelect={() => transition.mutate("active")}>
                  {t("projectSettings.restoreActive", {
                    defaultValue: "恢复到 active",
                  })}
                </DropdownMenuItem>
              ) : null}
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
