import { Link } from "@tanstack/react-router"
import { Repeat2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import type { ProjectTask } from "../api/task-api"
import {
  formatTaskSeriesTimestamp,
  recurrenceRuleLabel,
} from "../task-series/recurrence-preview"

export function RecurrenceContextAlert({
  projectSlug,
  task,
  workspaceSlug,
}: {
  projectSlug?: string
  task: ProjectTask
  workspaceSlug: string
}) {
  const { i18n, t } = useTranslation()
  const recurrence = task.recurrence_info
  if (!recurrence) return null

  const date = formatTaskSeriesTimestamp(
    recurrence.recurrence_at,
    i18n.language
  )
  const projected = recurrence.materialization === "projected"

  return (
    <Alert className="mt-3" data-testid="occurrence-banner">
      <Repeat2Icon />
      <AlertTitle>
        {projected
          ? t("taskSeries.occurrence.plannedTitle", { date })
          : t("taskSeries.occurrence.banner", {
              rule: recurrenceRuleLabel(recurrence.rule, t),
            })}
      </AlertTitle>
      <AlertDescription className="space-y-1">
        {!projected ? <p>{t("taskSeries.occurrence.date", { date })}</p> : null}
        {recurrence.series_title ? (
          <p>
            {t("taskSeries.occurrence.seriesName", {
              title: recurrence.series_title,
            })}
          </p>
        ) : null}
        <p>
          {projected
            ? t("taskSeries.occurrence.plannedDescription")
            : t("taskSeries.occurrence.description")}
        </p>
        {recurrence.series_status === "stopped" ? (
          <p className="font-medium">
            {t("taskSeries.occurrence.seriesStopped")}
          </p>
        ) : null}
        {recurrence.series_status === "ended" ? (
          <p className="font-medium">
            {t("taskSeries.occurrence.seriesEnded")}
          </p>
        ) : null}
        {projectSlug && recurrence.series_id ? (
          <Link
            className="mt-2 inline-flex font-medium text-primary underline underline-offset-4"
            params={{
              workspaceSlug,
              projectSlug,
              seriesRef: recurrence.series_id,
            }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug/series/$seriesRef"
          >
            {t("taskSeries.detail.viewSeries")} →
          </Link>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
