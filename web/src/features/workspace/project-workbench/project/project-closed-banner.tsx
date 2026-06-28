import { AlertTriangleIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { isClosedProjectStatus } from "./project-status-menu"

type ProjectClosedBannerProps = {
  canManage: boolean
  status: string
}

export function ProjectClosedBanner({ canManage, status }: ProjectClosedBannerProps) {
  const { t } = useTranslation()

  if (!isClosedProjectStatus(status)) {
    return null
  }

  return (
    <section className="flex items-start gap-3 border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-400/30 dark:bg-amber-400/10 dark:text-amber-100">
      <AlertTriangleIcon className="mt-0.5 size-4 shrink-0" />
      <div>
        <div className="font-medium">
          {t("projectWorkbench.project.closedTitle")}
        </div>
        <div className="mt-1 text-xs">
          {canManage
            ? t("projectWorkbench.project.closedManageDescription")
            : t("projectWorkbench.project.closedReadonlyDescription")}
        </div>
      </div>
    </section>
  )
}
