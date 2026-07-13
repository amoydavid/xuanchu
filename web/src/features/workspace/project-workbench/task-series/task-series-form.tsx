import { CalendarClockIcon, CircleAlertIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { DialogFooter } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  createTaskSeries,
  modifyTaskSeries,
  type TaskSeriesView,
} from "@/features/workspace/project-workbench/api/task-series-api"
import { InlineDatePicker } from "@/features/workspace/project-workbench/shared/inline-date-picker"
import {
  TaskCommonFields,
  type TaskCommonFieldValue,
} from "../tasks/task-common-fields"
import {
  RECURRENCE_OPTIONS,
  formatDateShort,
  isCanonicalRecurrenceRule,
  previewRecurrenceDates,
  recurrenceRuleLabel,
  type CanonicalRecurrenceRule,
} from "./recurrence-preview"

type TaskSeriesFormProps = {
  mode: "create" | "edit"
  workspaceSlug: string
  projectSlug: string
  series?: TaskSeriesView | null
  onCancel: () => void
  onCreated?: (series: TaskSeriesView) => void
  onSaved?: () => void
  onSubmittingChange?: (submitting: boolean) => void
  commonValue?: TaskCommonFieldValue
  onCommonChange?: (value: TaskCommonFieldValue) => void
  firstDueValue?: number | null
  onFirstDueChange?: (value: number | null) => void
}

export function TaskSeriesForm({
  mode,
  workspaceSlug,
  projectSlug,
  series,
  onCancel,
  onCreated,
  onSaved,
  onSubmittingChange,
  commonValue,
  onCommonChange,
  firstDueValue,
  onFirstDueChange,
}: TaskSeriesFormProps) {
  const { t } = useTranslation()
  const [internalCommon, setInternalCommon] = useState<TaskCommonFieldValue>(
    () => ({
      title: series?.title ?? "",
      description: series?.description ?? "",
      priority: series?.priority ?? "",
      assignees: (series?.assignees ?? []).map((assignee) => assignee.id),
      tags: (series?.tags ?? []).join(", "),
      udas: { ...(series?.udas ?? {}) },
    })
  )
  const common = commonValue ?? internalCommon
  const setCommon = onCommonChange ?? setInternalCommon
  const [rule, setRule] = useState<CanonicalRecurrenceRule>(() =>
    series && isCanonicalRecurrenceRule(series.recurrence_rule)
      ? series.recurrence_rule
      : "daily"
  )
  const [internalFirstDue, setInternalFirstDue] = useState<number | null>(
    () => series?.first_due ?? null
  )
  const firstDue =
    firstDueValue === undefined ? internalFirstDue : firstDueValue
  const setFirstDue = onFirstDueChange ?? setInternalFirstDue
  const [until, setUntil] = useState<number | null>(() => series?.until ?? null)
  const [effectiveFrom, setEffectiveFrom] = useState<number | null>(
    () => series?.suggested_rule_effective_from ?? null
  )
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const ruleChanged =
    mode === "edit" && series != null && rule !== series.recurrence_rule
  const untilError =
    firstDue != null && until != null && until < firstDue
      ? t("taskSeries.errors.untilBeforeFirstDue")
      : null
  const previewDates = useMemo(() => {
    const previewStart =
      mode === "create"
        ? firstDue
        : ruleChanged
          ? effectiveFrom
          : (series?.next_recurrence_at ?? firstDue)
    if (previewStart == null) return []
    return previewRecurrenceDates(
      new Date(previewStart * 1000),
      rule,
      3,
      until == null ? null : new Date(until * 1000)
    )
  }, [
    effectiveFrom,
    firstDue,
    mode,
    rule,
    ruleChanged,
    series?.next_recurrence_at,
    until,
  ])

  const setPending = (next: boolean) => {
    setSubmitting(next)
    onSubmittingChange?.(next)
  }

  const submit = async () => {
    const normalizedTitle = common.title.trim()
    if (!normalizedTitle) {
      setError(t("taskSeries.errors.titleRequired"))
      return
    }
    if (firstDue == null) {
      setError(t("taskSeries.errors.firstDueRequired"))
      return
    }
    if (untilError) {
      setError(untilError)
      return
    }
    if (ruleChanged && effectiveFrom == null) {
      setError(t("taskSeries.errors.effectiveFromRequired"))
      return
    }

    setError(null)
    setPending(true)
    try {
      if (mode === "create") {
        const result = await createTaskSeries(workspaceSlug, {
          title: normalizedTitle,
          ...(common.description.trim()
            ? { description: common.description.trim() }
            : {}),
          project: projectSlug,
          recurrence_rule: rule,
          first_due: firstDue,
          ...(until != null ? { until } : {}),
          ...(common.priority ? { priority: common.priority } : {}),
          ...(common.assignees.length > 0
            ? { assignees: common.assignees }
            : {}),
          ...(splitCSV(common.tags).length > 0
            ? { tags: splitCSV(common.tags) }
            : {}),
          ...(Object.keys(common.udas).length > 0 ? { udas: common.udas } : {}),
        })
        onCreated?.(result.series)
      } else if (series) {
        const clear: string[] = []
        if (!common.description.trim() && series.description)
          clear.push("description")
        if (until == null && series.until != null) clear.push("until")
        if (!common.priority && series.priority) clear.push("priority")
        if (
          common.assignees.length === 0 &&
          (series.assignees?.length ?? 0) > 0
        )
          clear.push("assignees")
        if (!common.tags.trim() && (series.tags?.length ?? 0) > 0)
          clear.push("tags")
        for (const name of Object.keys(series.udas ?? {})) {
          if (!(name in common.udas)) clear.push(`uda.${name}`)
        }
        await modifyTaskSeries(workspaceSlug, series.id, {
          title: normalizedTitle,
          ...(common.description.trim()
            ? { description: common.description.trim() }
            : {}),
          ...(common.priority ? { priority: common.priority } : {}),
          ...(common.assignees.length > 0
            ? { assignees: common.assignees }
            : {}),
          ...(until != null ? { until } : {}),
          ...(splitCSV(common.tags).length > 0
            ? { tags: splitCSV(common.tags) }
            : {}),
          ...(Object.keys(common.udas).length > 0 ? { udas: common.udas } : {}),
          ...(ruleChanged
            ? { recurrence_rule: rule, effective_from: effectiveFrom! }
            : {}),
          ...(clear.length > 0 ? { clear } : {}),
        })
        onSaved?.()
      }
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : t("taskSeries.errors.operation")
      )
      setPending(false)
    }
  }

  return (
    <div className="grid gap-5" data-testid="task-series-form">
      <TaskCommonFields
        autoFocus={mode === "create"}
        disabled={submitting}
        onChange={(next) => {
          setCommon(next)
          setError(null)
        }}
        onSubmit={() => void submit()}
        value={common}
        workspaceSlug={workspaceSlug}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label>{t("taskSeries.form.recurrenceRule")}</Label>
          <Select
            disabled={submitting}
            onValueChange={(value) => setRule(value as CanonicalRecurrenceRule)}
            value={rule}
          >
            <SelectTrigger
              aria-label={t("taskSeries.aria.rule")}
              className="w-full"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RECURRENCE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {recurrenceRuleLabel(option.value, t)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-2">
          <Label>{t("taskSeries.form.firstDue")}</Label>
          <InlineDatePicker
            ariaLabel={t("taskSeries.aria.firstDue")}
            boundary="end"
            disabled={mode === "edit" || submitting}
            emptyLabel={t("taskSeries.form.firstDue")}
            onSave={setFirstDue}
            value={firstDue}
          />
        </div>

        <div className="grid gap-2">
          <Label>{t("taskSeries.form.until")}</Label>
          <InlineDatePicker
            ariaLabel={t("taskSeries.aria.until")}
            boundary="end"
            disabled={submitting}
            emptyLabel={t("taskSeries.form.noUntil")}
            onSave={setUntil}
            value={until}
          />
        </div>

        {ruleChanged ? (
          <div className="grid gap-2 sm:col-span-2">
            <Label>{t("taskSeries.form.effectiveFrom")}</Label>
            <InlineDatePicker
              ariaLabel={t("taskSeries.aria.effectiveFrom")}
              boundary="end"
              disabled={submitting}
              emptyLabel={t("taskSeries.form.effectiveFrom")}
              onSave={setEffectiveFrom}
              value={effectiveFrom}
            />
            <p className="text-xs text-muted-foreground">
              {t("taskSeries.edit.effectiveFromHint")}
            </p>
          </div>
        ) : null}
      </div>

      {previewDates.length > 0 ? (
        <Alert>
          <CalendarClockIcon />
          <AlertDescription>
            <span className="font-medium text-foreground">
              {t("taskSeries.form.preview")}
            </span>
            <span className="mt-1 block tabular-nums">
              {previewDates.map(formatDateShort).join(" · ")}
            </span>
          </AlertDescription>
        </Alert>
      ) : null}

      {untilError || error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertDescription>{untilError ?? error}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button
          disabled={submitting}
          onClick={onCancel}
          type="button"
          variant="outline"
        >
          {t("common.cancel")}
        </Button>
        <Button
          disabled={submitting}
          onClick={() => void submit()}
          type="button"
          data-testid="series-submit-btn"
        >
          {mode === "create"
            ? t("taskSeries.create.submit")
            : t("taskSeries.edit.submit")}
        </Button>
      </DialogFooter>
    </div>
  )
}

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
}
