import { MoreHorizontalIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

type ProjectRowActionsProps = {
  onOpen: (projectSlug: string) => void
  projectSlug: string
}

export function ProjectRowActions({ onOpen, projectSlug }: ProjectRowActionsProps) {
  const { t } = useTranslation()

  return (
    <div onClick={(event) => event.stopPropagation()}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={t("projectWorkbench.projects.actionsLabel")}
            variant="ghost"
            size="icon-sm"
          >
            <MoreHorizontalIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => onOpen(projectSlug)}>
            {t("projectWorkbench.projects.open")}
          </DropdownMenuItem>
          <DropdownMenuItem disabled>{projectSlug}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
