import { AlertTriangleIcon } from "lucide-react"

import { isClosedProjectStatus } from "./project-status-menu"

type ProjectClosedBannerProps = {
  canManage: boolean
  status: string
}

export function ProjectClosedBanner({ canManage, status }: ProjectClosedBannerProps) {
  if (!isClosedProjectStatus(status)) {
    return null
  }

  return (
    <section className="flex items-start gap-3 border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-400/30 dark:bg-amber-400/10 dark:text-amber-100">
      <AlertTriangleIcon className="mt-0.5 size-4 shrink-0" />
      <div>
        <div className="font-medium">项目已关闭</div>
        <div className="mt-1 text-xs">
          {canManage
            ? "当前项目内任务处于禁写状态。恢复到 planning 或 active 后可以继续编辑。"
            : "当前项目内任务处于禁写状态。"}
        </div>
      </div>
    </section>
  )
}
