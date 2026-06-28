import { CopyIcon } from "lucide-react"
import type { TFunction } from "i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import type { ProjectReadonlyProject } from "./project-readonly-api"
import type { AssigneeSummary, ProjectStats } from "./project-stats"

type ProjectSummaryProps = {
  assignees: AssigneeSummary[]
  copyLabel: string
  copiedLabel: string
  onCopy: () => void
  project: ProjectReadonlyProject
  readonlyLabel: string
  stats: ProjectStats
  t: TFunction
  workspaceSlug: string
}

export function ProjectSummary({
  assignees,
  copyLabel,
  copiedLabel,
  onCopy,
  project,
  readonlyLabel,
  stats,
  t,
  workspaceSlug,
}: ProjectSummaryProps) {
  const statItems = [
    [t("projectReadonly.pending"), stats.pending],
    [t("projectReadonly.active"), stats.active],
    [t("projectReadonly.completed"), stats.completed],
    [t("projectReadonly.overdue"), stats.overdue],
    [t("projectReadonly.highPriority"), stats.highPriority],
  ] as const

  return (
    <section className="space-y-4">
      <div className="flex flex-col gap-3 border-b pb-4 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="text-xs text-muted-foreground">
            {workspaceSlug} / {project.slug}
          </div>
          <h1 className="mt-1 text-2xl font-semibold tracking-normal">
            {project.name}
          </h1>
          {project.description ? (
            <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
              {project.description}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Badge variant="outline">{readonlyLabel}</Badge>
          <Button
            className="text-muted-foreground hover:text-foreground"
            onClick={onCopy}
            size="sm"
            type="button"
            variant="outline"
          >
            <CopyIcon className="size-3.5" />
            {copyLabel || copiedLabel}
          </Button>
        </div>
      </div>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        {statItems.map(([label, value]) => (
          <div className="border bg-card p-3" key={label}>
            <div className="text-xs text-muted-foreground">{label}</div>
            <div className="mt-1 text-2xl font-semibold">{value}</div>
          </div>
        ))}
      </div>
      <div className="grid gap-3 lg:grid-cols-[1.1fr_0.9fr]">
        <div className="border bg-card p-3">
          <h2 className="text-sm font-medium">{t("projectReadonly.status")}</h2>
          <div className="mt-3 space-y-2">
            {[
              [t("projectReadonly.pending"), stats.pending],
              [t("projectReadonly.active"), stats.active],
              [t("projectReadonly.completed"), stats.completed],
            ].map(([label, value]) => (
              <div
                className="grid grid-cols-[5rem_1fr_2rem] items-center gap-2 text-xs"
                key={label}
              >
                <span className="text-muted-foreground">{label}</span>
                <div className="h-1.5 bg-muted">
                  <div
                    className="h-full bg-foreground"
                    style={{
                      width: `${stats.total === 0 ? 0 : Math.max(8, (Number(value) / stats.total) * 100)}%`,
                    }}
                  />
                </div>
                <span className="text-right tabular-nums">{value}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="border bg-card p-3">
          <h2 className="text-sm font-medium">
            {t("projectReadonly.assigneeSummary")}
          </h2>
          <div className="mt-3 space-y-2">
            {assignees.length === 0 ? (
              <div className="text-xs text-muted-foreground">
                {t("common.empty")}
              </div>
            ) : (
              assignees.slice(0, 5).map((assignee) => (
                <div
                  className="grid grid-cols-[1fr_auto_auto] gap-3 text-xs"
                  key={assignee.key}
                >
                  <span className="truncate">{assignee.label}</span>
                  <span className="text-muted-foreground">
                    {t("projectWorkbench.project.openTasks", {
                      count: assignee.open,
                    })}
                  </span>
                  <span className="text-muted-foreground">
                    {t("projectWorkbench.project.overdueTasks", {
                      count: assignee.overdue,
                    })}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </section>
  )
}
