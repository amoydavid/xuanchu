import { STANDARD_TASK_FIELDS } from "./project-readonly-api"

// 后端 UDAs 平铺为 JSONTask 的顶层字段（非嵌套在 `udas` 对象里）。
// extractUDAs 从任务对象中识别出非标准字段，作为自定义字段返回。
// 标准字段清单（STANDARD_TASK_FIELDS）与 ProjectReadonlyTask 类型同处维护，
// 保证后端新增字段时前端识别同步——避免新字段被误判为 UDA。

export type UDAEntry = [key: string, value: unknown]

export function extractUDAs(task: Record<string, unknown>): UDAEntry[] {
  if (task.udas && typeof task.udas === "object" && !Array.isArray(task.udas)) {
    return Object.entries(task.udas as Record<string, unknown>).filter(
      ([, value]) => value !== undefined && value !== null && value !== ""
    )
  }
  const flattened = Object.entries(task).filter(
    ([key, value]) =>
      !STANDARD_TASK_FIELDS.has(key) &&
      value !== undefined &&
      value !== null &&
      value !== ""
  )
  return flattened
}

// formatUDAValue 把任意 UDA 值渲染为可读字符串。
export function formatUDAValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "-"
  }
  if (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return String(value)
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}
