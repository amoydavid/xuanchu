import { useState } from "react"
import { CheckIcon, ChevronDownIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
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
  const [confirmStatus, setConfirmStatus] = useState<ProjectStatus | null>(null)
  const transition = useTransitionProjectMutation(workspaceSlug, project.slug)
  const closed = isClosedProjectStatus(project.status)

  const submit = async (status: ProjectStatus) => {
    await transition.mutateAsync(status)
  }

  if (!canManage) {
    return (
      <Button aria-label="项目状态" disabled size="sm" variant="outline">
        {project.status}
      </Button>
    )
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label={`项目状态 ${project.status}`} size="sm" variant="outline">
            {project.status}
            <ChevronDownIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-44">
          {closed ? (
            <DropdownMenuLabel>
              恢复后项目内任务将重新可写
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
              {status}
              {status === project.status ? <CheckIcon className="ml-auto" /> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        confirmLabel="关闭项目"
        description="进入 archived 或 cancelled 后，项目内任务会进入禁写状态；需要恢复到 planning 或 active 后才能继续编辑。"
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
        title="确认关闭项目"
      />
    </>
  )
}

export function isClosedProjectStatus(status: string | null | undefined) {
  return status === "archived" || status === "cancelled"
}
