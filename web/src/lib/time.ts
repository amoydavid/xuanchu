// relativeTime 把 unix 秒渲染成简短的相对时间。
// 例如 "2 分钟前"、"昨天"、"3 天前"。不依赖 i18n 库；UI 文案以中文为主，
// 复杂格式化仍由各组件自行 Intl 处理。
export function relativeTime(unixSeconds: number): string {
  if (!unixSeconds || unixSeconds <= 0) return "—"
  const now = Math.floor(Date.now() / 1000)
  const diff = now - unixSeconds
  if (diff < 0) return "刚刚"
  if (diff < 60) return `${diff} 秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 86400 * 7) return `${Math.floor(diff / 86400)} 天前`
  const date = new Date(unixSeconds * 1000)
  return date.toISOString().slice(0, 10)
}
