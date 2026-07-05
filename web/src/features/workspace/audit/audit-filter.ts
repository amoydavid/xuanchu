// 当前结果筛选与 CSV 导出。
// 后端不支持 actor/action/time range 服务端搜索，因此这里只对当前已加载 rows 过滤，
// 不声称全量审计搜索（spec §2.2）。
import type { AuditRow } from "./audit-api"

export type AuditFilter = {
  actor?: string
  action?: string
  target?: string
  after?: number
  before?: number
}

export function filterAuditRows(rows: AuditRow[], filter: AuditFilter): AuditRow[] {
  const actor = filter.actor?.trim().toLowerCase()
  const action = filter.action?.trim().toLowerCase()
  const target = filter.target?.trim().toLowerCase()
  return rows.filter((row) => {
    if (actor) {
      const name =
        row.actor?.display_name || row.actor?.name || row.actor?.id || ""
      if (!name.toLowerCase().includes(actor)) return false
    }
    if (action && !row.action.toLowerCase().includes(action)) return false
    if (target && !row.target_id.toLowerCase().includes(target)) return false
    if (filter.after && row.created_at < filter.after) return false
    if (filter.before && row.created_at > filter.before) return false
    return true
  })
}

function csvEscape(value: string): string {
  if (value.includes(",") || value.includes('"') || value.includes("\n")) {
    return `"${value.replace(/"/g, '""')}"`
  }
  return value
}

function actorName(row: AuditRow): string {
  return row.actor?.display_name || row.actor?.name || row.actor?.id || ""
}

function formatTime(ts: number): string {
  try {
    return new Date(ts * 1000).toISOString()
  } catch {
    return String(ts)
  }
}

export function rowsToCSV(rows: AuditRow[]): string {
  const header = ["time", "actor", "action", "target_type", "target_id"]
  const lines = [header.join(",")]
  for (const row of rows) {
    lines.push(
      [
        formatTime(row.created_at),
        actorName(row),
        row.action,
        row.target_type,
        row.target_id,
      ]
        .map(csvEscape)
        .join(",")
    )
  }
  return lines.join("\n")
}
