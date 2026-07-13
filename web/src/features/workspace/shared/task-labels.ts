import type { TFunction } from "i18next"

const statusKeys: Record<string, string> = {
  active: "projectReadonly.active",
  archived: "projectReadonly.statusArchived",
  cancelled: "projectReadonly.statusCancelled",
  completed: "projectReadonly.completed",
  deleted: "projectReadonly.statusDeleted",
  pending: "projectReadonly.pending",
  planning: "projectReadonly.statusPlanning",
  waiting: "projectReadonly.statusWaiting",
}

export function taskStatusLabel(status: string, t: TFunction): string {
  const key = statusKeys[status]
  return key ? t(key) : status
}
