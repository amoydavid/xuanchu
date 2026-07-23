/**
 * cron 表达式预设、校验和中文解读的纯函数模块。
 *
 * 与后端 internal/schedule/schedule.go 的 Describe() 保持同样的预设映射，
 * 供前端 cron 输入组件实时展示中文解读。后端是权威校验源，这里只做轻量校验。
 */

/** 常用 cron 预设，点击后回填表达式。顺序对应 UI 展示顺序。 */
export const CRON_PRESETS: ReadonlyArray<{ label: string; expr: string }> = [
  { label: "每天 9 点", expr: "0 9 * * *" },
  { label: "每工作日 9 点", expr: "0 9 * * 1-5" },
  { label: "每周一 9 点", expr: "0 9 * * 1" },
  { label: "每小时整点", expr: "0 * * * *" },
  { label: "每 15 分钟", expr: "*/15 * * * *" },
  { label: "每月 1 号 9 点", expr: "0 9 1 * *" },
]

/** 常用时区选项。 */
export const TIMEZONE_OPTIONS: ReadonlyArray<string> = [
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Asia/Singapore",
  "UTC",
  "America/Los_Angeles",
  "America/New_York",
  "Europe/London",
]

/**
 * 轻量校验 cron 表达式是否为合法的 5 段格式。
 * 仅做结构校验（5 段、字符合法），精确语义校验由后端 schedule.Spec.Validate 完成。
 */
export function isValidCronShape(expr: string): boolean {
  const trimmed = expr.trim()
  if (trimmed === "") return false
  const fields = trimmed.split(/\s+/)
  if (fields.length !== 5) return false
  // 每段只允许数字、*、,、-、/ 和月/周别名（字母）
  return fields.every((field) => /^[0-9*,/\-A-Za-z]+$/.test(field))
}

/**
 * 把常见 cron 模式映射为中文描述，未匹配回退为表达式本身。
 * 与后端 schedule.Spec.Describe() 逻辑保持一致。
 */
export function describeCron(expr: string): string {
  const fields = expr.trim().split(/\s+/)
  if (fields.length !== 5) return expr
  const [minute, hour, dayOfMonth, month, week] = fields
  // 带月维度的表达式不匹配时间类预设
  if (month !== "*") return expr
  const timeOK = isSimpleField(hour) && isSimpleField(minute)
  const hhmm = `${pad2(hour)}:${pad2(minute)}`
  // 只有日期/周是通配或简单周模式时才尝试时间类解读；带具体几号走月维度分支。
  const wdLabel = weekdayLabel(week) // "每天 "|"每工作日 "|"每周X "|null
  const dayWildcard = dayOfMonth === "*" && wdLabel !== null

  // 每 N 分钟：*/N * * * *  （高频全频次，不加日期前缀）
  if (minute.startsWith("*/") && hour === "*" && dayWildcard) {
    return `每 ${minute.slice(2)} 分钟触发`
  }
  // 每 N 小时整点：0 */N * * *  （如 0 */6 * * 1-5 = 工作日每 6 小时）
  // 非"每天"时加范围前缀（"工作日每 6 小时"），"每天"时省略（"每 6 小时"）。
  if (minute === "0" && hour.startsWith("*/") && dayWildcard) {
    const prefix = weekdayScopePrefix(week)
    return `${prefix}每 ${hour.slice(2)} 小时触发`
  }
  // 每小时整点：0 * * * *
  if (minute === "0" && hour === "*" && dayWildcard) {
    const prefix = weekdayScopePrefix(week)
    return `${prefix}每小时整点触发`
  }
  // 简单时刻（HH:MM）
  if (timeOK && dayWildcard) {
    return `${wdLabel}${hhmm} 触发`
  }
  // 每月 N 日 HH:MM
  if (timeOK && dayOfMonth !== "*" && week === "*") {
    return `每月 ${dayOfMonth} 日 ${hhmm} 触发`
  }
  return expr
}

/**
 * weekdayLabel 把 cron 周字段映射为中文范围前缀（带尾空格，便于拼接）。
 * "*" → "每天 "；"1-5" → "每工作日 "；单数字 → "每周X "；其它 → null（无法简化，调用方应回退）。
 */
function weekdayLabel(week: string): string | null {
  if (week === "*") return "每天 "
  if (week === "1-5") return "每工作日 "
  const wd = parseWeekday(week)
  if (wd) return `每周${wd} `
  return null
}

/**
 * weekdayScopePrefix 用于"每 N 小时/每小时整点"这类高频场景的范围前缀。
 * 与 weekdayLabel 不同：这里"每天"省略（高频本身隐含每天），"1-5"用"工作日"避免"每工作日 每 6 小时"拗口。
 */
function weekdayScopePrefix(week: string): string {
  if (week === "*") return ""
  if (week === "1-5") return "工作日"
  const wd = parseWeekday(week)
  if (wd) return `周${wd} `
  return ""
}

function isSimpleField(s: string): boolean {
  if (s === "") return false
  return /^[0-9]+$/.test(s)
}

const WEEKDAY_NAMES: Record<string, string> = {
  "0": "日",
  "1": "一",
  "2": "二",
  "3": "三",
  "4": "四",
  "5": "五",
  "6": "六",
  "7": "日",
}

function parseWeekday(s: string): string | null {
  return WEEKDAY_NAMES[s] ?? null
}

function pad2(s: string): string {
  return s.length === 1 ? `0${s}` : s
}
