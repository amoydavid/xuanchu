import { format, parseISO } from "date-fns"

// formatConfigDisplayValue 把 config value 按 valueType 格式化为人类可读的展示字符串。
// - null / 空值返回空字符串（由调用方决定占位文案）。
// - date：YYYY-MM-DD。
// - datetime：按浏览器本地时区展示 YYYY-MM-DD HH:mm（存储是 UTC RFC3339）。
// - 其它类型：原样返回。
// 解析失败时回退原值，不抛错（展示层不应因脏数据崩溃）。
export function formatConfigDisplayValue(
  valueType: string,
  value: string | null
): string {
  if (value === null || value === "") {
    return ""
  }
  if (valueType === "date") {
    try {
      return format(parseISO(value), "yyyy-MM-dd")
    } catch {
      return value
    }
  }
  if (valueType === "datetime") {
    try {
      return format(parseISO(value), "yyyy-MM-dd HH:mm")
    } catch {
      return value
    }
  }
  return value
}
