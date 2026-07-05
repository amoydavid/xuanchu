import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { getTaskUrgency } from "../api/task-api"

// TaskUrgencyPanel 展示任务紧迫度分数和各分项贡献。
// 失败时不阻断任务详情页，只显示轻量错误占位。
export function TaskUrgencyPanel({
  taskRef,
  workspaceSlug,
}: {
  taskRef: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ["task", workspaceSlug, taskRef, "urgency"],
    queryFn: () => getTaskUrgency(workspaceSlug, taskRef),
  })

  if (query.isError) {
    return (
      <div className="text-xs text-muted-foreground">
        {t("taskDetail.urgencyUnavailable")}
      </div>
    )
  }
  if (query.isLoading || !query.data) {
    return (
      <div className="text-xs text-muted-foreground">
        {t("common.loading")}
      </div>
    )
  }

  const data = query.data
  const pct = Math.max(0, Math.min(100, (data.total / 10) * 100))

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium">{data.total.toFixed(1)}</span>
        <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full bg-primary"
            style={{ width: `${pct}%` }}
          />
        </div>
      </div>
      <ul className="space-y-0.5 text-[11px] text-muted-foreground">
        {data.items.map((item) => (
          <li className="flex items-center justify-between gap-2" key={item.name}>
            <span>{item.name}</span>
            <span>
              {item.contribution >= 0 ? "+" : ""}
              {item.contribution.toFixed(1)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
