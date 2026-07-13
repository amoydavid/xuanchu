// recurrence-preview 提供前端循环规则预览（spec §15.15、§15.16）。
// 使用本地日历 setFullYear/setMonth/setDate，不用固定毫秒运算，避免时区漂移。

import type { TFunction } from "i18next"

export type CanonicalRecurrenceRule =
  | "daily"
  | "weekly"
  | "monthly"
  | `${number}days`
  | `${number}weeks`
  | `${number}months`

/** Web UI 选项到 canonical 规则的映射（spec §12）。 */
export const RECURRENCE_OPTIONS: Array<{ value: CanonicalRecurrenceRule }> = [
  { value: "daily" },
  { value: "weekly" },
  { value: "2weeks" },
  { value: "monthly" },
  { value: "3months" },
  { value: "12months" },
]

/** 把 canonical 规则映射为本地化用户文案；未知规则原样回退，便于诊断数据。 */
export function recurrenceRuleLabel(rule: string, t: TFunction): string {
  if (rule === "daily" || rule === "weekly" || rule === "monthly") {
    return t(`taskSeries.rule.${rule}`)
  }
  const match = rule.match(/^(\d+)(days|weeks|months)$/)
  if (!match) return rule
  const key = {
    days: "everyDays",
    weeks: "everyWeeks",
    months: "everyMonths",
  }[match[2]]
  return t(`taskSeries.rule.${key}`, { count: Number.parseInt(match[1], 10) })
}

/** 把系列状态映射为本地化用户文案；未知状态原样回退。 */
export function taskSeriesStatusLabel(status: string, t: TFunction): string {
  if (status === "active" || status === "ended" || status === "stopped") {
    return t(`taskSeries.status.${status}`)
  }
  return status
}

/** 把 API Unix 秒转换为本地化日期，空值统一显示占位符。 */
export function formatTaskSeriesTimestamp(
  timestamp: number | null | undefined,
  language: string
): string {
  if (timestamp == null || !Number.isFinite(timestamp)) return "—"
  return new Intl.DateTimeFormat(language, {
    year: "numeric",
    month: "short",
    day: "numeric",
  }).format(new Date(timestamp * 1000))
}

/** 校验规则字符串是否为 canonical 形式。 */
export function isCanonicalRecurrenceRule(
  value: string
): value is CanonicalRecurrenceRule {
  if (value === "daily" || value === "weekly" || value === "monthly") {
    return true
  }
  const match = value.match(/^(\d+)(days|weeks|months)$/)
  if (!match) return false
  const n = Number.parseInt(match[1], 10)
  return n > 0
}

/**
 * 计算从给定日期起的下一槽位。使用本地日历运算：
 * - days: setDate(getDate() + n)
 * - weeks: setDate(getDate() + 7*n)
 * - months: setMonth(getMonth() + n)（Go AddDate 月末语义在 JS 中接近，但 JS 会 clamp 月底）
 *
 * 注意：JS 的 setMonth 对月底会溢出到下月（如 1/31 + 1 月 = 3/3），
 * 与 Go AddDate 行为一致，符合 spec §8.5 月末滚动语义。
 */
export function nextRecurrenceDate(
  from: Date,
  rule: CanonicalRecurrenceRule
): Date {
  const next = new Date(
    from.getFullYear(),
    from.getMonth(),
    from.getDate(),
    23,
    59,
    59,
    0
  )
  if (rule === "daily") {
    next.setDate(next.getDate() + 1)
    return next
  }
  if (rule === "weekly") {
    next.setDate(next.getDate() + 7)
    return next
  }
  if (rule === "monthly") {
    next.setMonth(next.getMonth() + 1)
    return next
  }
  const match = rule.match(/^(\d+)(days|weeks|months)$/)
  if (!match) return next
  const n = Number.parseInt(match[1], 10)
  const unit = match[2]
  if (unit === "days") {
    next.setDate(next.getDate() + n)
  } else if (unit === "weeks") {
    next.setDate(next.getDate() + 7 * n)
  } else {
    next.setMonth(next.getMonth() + n)
  }
  return next
}

/** 生成未来 N 个槽位（默认 3），用于创建/编辑弹窗预览。 */
export function previewRecurrenceDates(
  from: Date,
  rule: CanonicalRecurrenceRule,
  count = 3,
  until?: Date | null
): Date[] {
  const out: Date[] = []
  let current = new Date(from)
  for (let i = 0; i < count; i++) {
    if (until && current.getTime() > until.getTime()) break
    out.push(new Date(current))
    current = nextRecurrenceDate(current, rule)
  }
  return out
}

/** 格式化日期为 YYYY-MM-DD。 */
export function formatDateShort(date: Date): string {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, "0")
  const d = String(date.getDate()).padStart(2, "0")
  return `${y}-${m}-${d}`
}

/** 把 YYYY-MM-DD 解析为本地当天 23:59:59 的 Date。 */
export function parseDateToEndOfDay(value: string): Date | null {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (!match) return null
  const y = Number.parseInt(match[1], 10)
  const m = Number.parseInt(match[2], 10) - 1
  const d = Number.parseInt(match[3], 10)
  const date = new Date(y, m, d, 23, 59, 59, 0)
  if (Number.isNaN(date.getTime())) return null
  // JS 会溢出滚动非法日期（如 2030-13-45），校验解析后的字段一致。
  if (date.getFullYear() !== y || date.getMonth() !== m || date.getDate() !== d)
    return null
  return date
}
