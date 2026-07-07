import { useTranslation } from "react-i18next"

import type { ProjectTimelineEntry } from "../api/project-api"
import type { AuditRow } from "@/features/workspace/audit/audit-api"

// ActivityItem 统一 project annotation、task annotation 和 audit 三类记录。
// 去重 key 为 `${kind}:${id}:${entry}`；不能把 audit 与 annotation 合并成无来源的摘要。
export type ActivityItem =
  | {
      kind: "project"
      id: string
      entry: number
      content: string
      actorLabel: string
    }
  | {
      kind: "task"
      id: string
      entry: number
      content: string
      sourceLabel: string
      actorLabel: string
    }
  | {
      kind: "audit"
      id: string
      entry: number
      action: string
      actorLabel: string
      summary: string
    }

type ProjectActivityTimelineProps = {
  items: ActivityItem[]
  emptyLabel?: string
}

// mergeActivity 合并 timeline 与 audit，按 entry 倒序，按 kind:id:entry 去重。
export function mergeActivity(
  timeline: ProjectTimelineEntry[],
  audit: AuditRow[] | undefined,
  filter: ActivityFilter
): ActivityItem[] {
  const items: ActivityItem[] = []
  if (filter.project || filter.all) {
    for (const entry of timeline) {
      if (entry.source_type === "project") {
        items.push({
          kind: "project",
          id: entry.source_id ?? "",
          entry: entry.entry ?? 0,
          content: entry.content ?? "",
          actorLabel: actorLabelFromTimeline(entry),
        })
      }
    }
  }
  if (filter.task || filter.all) {
    for (const entry of timeline) {
      if (entry.source_type === "task") {
        items.push({
          kind: "task",
          id: entry.source_id ?? "",
          entry: entry.entry ?? 0,
          content: entry.content ?? "",
          sourceLabel: entry.source_label ?? "",
          actorLabel: actorLabelFromTimeline(entry),
        })
      }
    }
  }
  if ((filter.audit || filter.all) && audit) {
    for (const row of audit) {
      items.push({
        kind: "audit",
        id: `${row.action}:${row.id}`,
        entry: row.created_at,
        action: row.action,
        actorLabel: row.actor?.name ?? row.actor_type ?? "",
        summary: summarizeAuditPayload(row),
      })
    }
  }
  // 去重：kind + id + entry
  const seen = new Set<string>()
  const deduped: ActivityItem[] = []
  for (const item of items) {
    const key = `${item.kind}:${item.id}:${item.entry}`
    if (seen.has(key)) continue
    seen.add(key)
    deduped.push(item)
  }
  deduped.sort((a, b) => b.entry - a.entry)
  return deduped
}

export type ActivityFilter = {
  all?: boolean
  project?: boolean
  task?: boolean
  audit?: boolean
}

function actorLabelFromTimeline(entry: ProjectTimelineEntry): string {
  const by = entry.created_by
  if (by?.user?.name) return by.user.name
  if (by?.token?.name) return by.token.name
  if (by?.name) return by.name
  if (entry.actor?.name) return entry.actor.name
  return ""
}

// summarizeAuditPayload 仅展示 payload 里已有的字段，不解释成无来源的自然语言。
function summarizeAuditPayload(row: AuditRow): string {
  const payload = row.payload
  if (payload && typeof payload === "object") {
    const obj = payload as Record<string, unknown>
    const from = obj["from"]
    const to = obj["to"]
    if (from !== undefined && to !== undefined) {
      return `${formatValue(from)} → ${formatValue(to)}`
    }
  }
  return ""
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "—"
  }
  return String(value)
}

// ProjectActivityTimeline 按 entry 倒序渲染合并后的 ActivityItem。
export function ProjectActivityTimeline({
  items,
  emptyLabel,
}: ProjectActivityTimelineProps) {
  const { t } = useTranslation()
  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {emptyLabel ?? t("projectSubpages.activityEmpty")}
      </p>
    )
  }
  return (
    <ol className="space-y-3">
      {items.map((item, index) => (
        <li className="rounded-lg border bg-card p-3 text-sm" key={`${item.kind}:${item.id}:${item.entry}:${index}`}>
          <ActivityItemView item={item} />
        </li>
      ))}
    </ol>
  )
}

function ActivityItemView({ item }: { item: ActivityItem }) {
  const { t } = useTranslation()
  const entryTime = formatEntryTime(item.entry)
  if (item.kind === "project") {
    return (
      <div className="space-y-1">
        <div className="text-xs text-muted-foreground">
          {t("projectSubpages.activityFilterProject")} · {item.actorLabel} · {entryTime}
        </div>
        <div>{item.content}</div>
      </div>
    )
  }
  if (item.kind === "task") {
    return (
      <div className="space-y-1">
        <div className="text-xs text-muted-foreground">
          {t("projectSubpages.activityFilterTask")} · {item.sourceLabel} · {entryTime}
        </div>
        <div>{item.content}</div>
      </div>
    )
  }
  return (
    <div className="space-y-1">
      <div className="text-xs text-muted-foreground">
        {t("projectSubpages.activityFilterAudit")} · {item.action} · {item.actorLabel} · {entryTime}
      </div>
      {item.summary ? <div>{item.summary}</div> : null}
    </div>
  )
}

function formatEntryTime(unix: number): string {
  if (!unix) return ""
  const date = new Date(unix * 1000)
  if (Number.isNaN(date.getTime())) return ""
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  const hours = String(date.getHours()).padStart(2, "0")
  const minutes = String(date.getMinutes()).padStart(2, "0")
  return `${year}-${month}-${day} ${hours}:${minutes}`
}
