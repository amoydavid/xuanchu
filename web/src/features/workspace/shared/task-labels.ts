import type { TFunction } from "i18next"

const statusKeys: Record<string, string> = {
  active: "projectReadonly.active",
  archived: "projectReadonly.statusArchived",
  cancelled: "projectReadonly.statusCancelled",
  completed: "projectReadonly.completed",
  deleted: "projectReadonly.statusDeleted",
  pending: "projectReadonly.pending",
  planning: "projectReadonly.statusPlanning",
  recurring: "projectReadonly.statusRecurring",
  waiting: "projectReadonly.statusWaiting",
}

const recurrenceLabels: Record<string, string> = {
  annual: "projectReadonly.recurAnnual",
  biweekly: "projectReadonly.recurBiweekly",
  daily: "projectReadonly.recurDaily",
  monthly: "projectReadonly.recurMonthly",
  quarterly: "projectReadonly.recurQuarterly",
  weekly: "projectReadonly.recurWeekly",
  yearly: "projectReadonly.recurAnnual",
}

export function taskStatusLabel(status: string, t: TFunction): string {
  const key = statusKeys[status]
  return key ? t(key) : status
}

export function recurrenceLabel(
  value: string | null | undefined,
  t: TFunction
): string {
  if (!value) {
    return "-"
  }
  const normalized = value.trim().toLowerCase()
  return recurrenceLabels[normalized] ? t(recurrenceLabels[normalized]) : value
}

export const recurrenceOptions = [
  { labelKey: "projectReadonly.recurNone", value: "none" },
  { labelKey: "projectReadonly.recurDaily", value: "daily" },
  { labelKey: "projectReadonly.recurWeekly", value: "weekly" },
  { labelKey: "projectReadonly.recurBiweekly", value: "biweekly" },
  { labelKey: "projectReadonly.recurMonthly", value: "monthly" },
  { labelKey: "projectReadonly.recurQuarterly", value: "quarterly" },
  { labelKey: "projectReadonly.recurAnnual", value: "annual" },
]
