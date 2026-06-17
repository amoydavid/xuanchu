// 后端 UDAs 平铺为 JSONTask 的顶层字段（非嵌套在 `udas` 对象里）。
// extractUDAs 从任务对象中识别出非保留字段，作为自定义字段返回。
const RESERVED_KEYS = new Set([
  "uuid",
  "description",
  "status",
  "entry",
  "modified",
  "end",
  "due",
  "project",
  "project_id",
  "project_seq",
  "task_slug",
  "priority",
  "tags",
  "start",
  "wait",
  "scheduled",
  "until",
  "annotations",
  "depends",
  "recur",
  "parent",
  "mask",
  "imask",
  "assignees",
  "links",
])

export type UDAEntry = [key: string, value: unknown]

export function extractUDAs(task: Record<string, unknown>): UDAEntry[] {
  return Object.entries(task).filter(
    ([key, value]) =>
      !RESERVED_KEYS.has(key) && value !== undefined && value !== null && value !== ""
  )
}

// formatUDAValue 把任意 UDA 值渲染为可读字符串。
export function formatUDAValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "-"
  }
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return String(value)
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}
