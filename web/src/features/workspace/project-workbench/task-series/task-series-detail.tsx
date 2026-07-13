import { ArrowLeftIcon, PencilIcon, StopCircleIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import type {
  TaskOccurrenceView,
  TaskSeriesView,
} from "@/features/workspace/project-workbench/api/task-series-api"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import {
  taskDisplayRef,
  taskRouteRef,
} from "@/features/workspace/project-workbench/tasks/task-reference"
import {
  formatTaskSeriesTimestamp,
  recurrenceRuleLabel,
  taskSeriesStatusLabel,
} from "./recurrence-preview"

export function TaskSeriesDetail({
  series,
  onBack,
  onEdit,
  onStop,
  canManage,
  workspaceSlug,
  projectSlug,
}: {
  series: TaskSeriesView
  onBack: () => void
  onEdit?: () => void
  onStop?: () => void
  canManage: boolean
  workspaceSlug: string
  projectSlug: string
}) {
  const { i18n, t } = useTranslation()
  const active = series.status === "active"
  const formatTimestamp = (value: number | null | undefined) =>
    formatTaskSeriesTimestamp(value, i18n.language)

  return (
    <article
      className="space-y-4"
      data-testid="task-series-detail"
      data-series-id={series.id}
    >
      <header className="space-y-3">
        <Button
          className="-ml-2"
          onClick={onBack}
          size="sm"
          type="button"
          variant="ghost"
        >
          <ArrowLeftIcon />
          {t("taskSeries.detail.backToList")}
        </Button>
        <div className="space-y-2">
          <div className="flex items-start justify-between gap-2">
            <h3 className="min-w-0 text-base leading-6 font-semibold">
              {series.title}
            </h3>
            <Badge variant={active ? "secondary" : "outline"}>
              {taskSeriesStatusLabel(series.status, t)}
            </Badge>
          </div>
          {canManage && active ? (
            <div className="flex gap-2">
              {onEdit ? (
                <Button
                  data-testid="series-edit-btn"
                  onClick={onEdit}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  <PencilIcon />
                  {t("taskSeries.actions.edit")}
                </Button>
              ) : null}
              {onStop ? (
                <Button
                  data-testid="series-stop-btn"
                  onClick={onStop}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  <StopCircleIcon />
                  {t("taskSeries.actions.stop")}
                </Button>
              ) : null}
            </div>
          ) : null}
        </div>
      </header>

      <Separator />

      <dl className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-2 text-sm">
        <dt className="text-muted-foreground">{t("taskSeries.detail.rule")}</dt>
        <dd className="text-right font-medium">
          {recurrenceRuleLabel(series.recurrence_rule, t)}
        </dd>
        <dt className="text-muted-foreground">
          {t("taskSeries.detail.firstDue")}
        </dt>
        <dd className="text-right tabular-nums">
          {formatTimestamp(series.first_due)}
        </dd>
        <dt className="text-muted-foreground">
          {t("taskSeries.detail.until")}
        </dt>
        <dd className="text-right tabular-nums">
          {series.until == null
            ? t("taskSeries.detail.neverEnds")
            : formatTimestamp(series.until)}
        </dd>
        {series.next_recurrence_at != null ? (
          <>
            <dt className="text-muted-foreground">
              {t("taskSeries.detail.nextAt")}
            </dt>
            <dd className="text-right tabular-nums">
              {formatTimestamp(series.next_recurrence_at)}
            </dd>
          </>
        ) : null}
      </dl>

      <div className="grid grid-cols-2 gap-2">
        <CountCard
          label={t("taskSeries.list.openCount", {
            count: series.open_occurrence_count,
          })}
          value={series.open_occurrence_count}
        />
        <CountCard
          destructive={series.overdue_count > 0}
          label={t("taskSeries.list.overdueCount", {
            count: series.overdue_count,
          })}
          value={series.overdue_count}
        />
        <CountCard
          label={t("taskSeries.list.completedCount", {
            count: series.completed_count,
          })}
          value={series.completed_count}
        />
        <CountCard
          label={t("taskSeries.list.skippedCount", {
            count: series.skipped_count,
          })}
          value={series.skipped_count}
        />
      </div>

      <OccurrenceGroup
        empty={t("taskSeries.detail.noOpenOccurrences")}
        items={series.open_occurrences ?? []}
        projectSlug={projectSlug}
        title={t("taskSeries.detail.openOccurrences")}
        workspaceSlug={workspaceSlug}
      />
      <OccurrenceGroup
        empty={t("taskSeries.detail.noRecentCompleted")}
        items={series.recent_completed ?? []}
        projectSlug={projectSlug}
        title={t("taskSeries.detail.recentCompleted")}
        workspaceSlug={workspaceSlug}
      />
      <OccurrenceGroup
        empty={t("taskSeries.detail.noRecentSkipped")}
        items={series.recent_skipped ?? []}
        projectSlug={projectSlug}
        title={t("taskSeries.detail.recentSkipped")}
        workspaceSlug={workspaceSlug}
      />
    </article>
  )
}

function CountCard({
  label,
  value,
  destructive = false,
}: {
  label: string
  value: number
  destructive?: boolean
}) {
  return (
    <div className="rounded-lg border bg-card px-3 py-2">
      <div
        className={
          destructive
            ? "text-lg font-semibold text-destructive tabular-nums"
            : "text-lg font-semibold tabular-nums"
        }
      >
        {value}
      </div>
      <div className="truncate text-xs text-muted-foreground">{label}</div>
    </div>
  )
}

function OccurrenceGroup({
  items,
  projectSlug,
  title,
  empty,
  workspaceSlug,
}: {
  items: TaskOccurrenceView[]
  projectSlug: string
  title: string
  empty: string
  workspaceSlug: string
}) {
  const { i18n, t } = useTranslation()
  return (
    <section className="space-y-2">
      <h4 className="text-sm font-medium">{title}</h4>
      {items.length === 0 ? (
        <p className="rounded-lg border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
          {empty}
        </p>
      ) : (
        <ul className="divide-y overflow-hidden rounded-lg border bg-card">
          {items.map((item) => {
            const routeRef = taskRouteRef(item)
            const displayRef = taskDisplayRef(item, i18n.language)
            return (
              <li
                className="flex items-center justify-between gap-2 px-3 py-2 text-sm"
                key={item.id}
              >
                <a
                  aria-label={t("taskSeries.detail.viewOccurrence", {
                    title: item.title,
                  })}
                  className="flex min-w-0 items-baseline gap-2 truncate font-medium hover:underline"
                  href={`/workspaces/${encodeURIComponent(workspaceSlug)}/projects/${encodeURIComponent(projectSlug)}/tasks/${encodeURIComponent(routeRef)}`}
                >
                  <span className="shrink-0 font-mono text-xs">
                    {displayRef}
                  </span>
                  {item.due && item.task_slug ? (
                    <span className="truncate text-xs font-normal text-muted-foreground">
                      {formatTaskSeriesTimestamp(item.due, i18n.language)}
                    </span>
                  ) : null}
                </a>
                <Badge className="shrink-0" variant="outline">
                  {taskStatusLabel(item.status, t)}
                </Badge>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
