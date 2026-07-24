import { cn } from "@/lib/utils"

/**
 * 状态徽章：7px 圆点 + 文字。
 *
 * 设计语言约束：状态色（success/warn/info/danger/meta）只以圆点或小 chip 出现，
 * 绝不作大面积底色铺色。这里统一为「点 + 文字」，颜色仅落在 7px dot 上，
 * 文字保持 foreground/muted，避免一屏出现多处彩色块。
 */
export type StatusTone = "success" | "warn" | "info" | "danger" | "muted"

const toneDot: Record<StatusTone, string> = {
  success: "bg-success",
  warn: "bg-warn",
  info: "bg-info",
  danger: "bg-destructive",
  muted: "bg-muted-foreground",
}

export function StatusBadge({
  children,
  className,
  tone = "muted",
}: {
  children: React.ReactNode
  className?: string
  tone?: StatusTone
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-xs font-medium whitespace-nowrap",
        className
      )}
    >
      <span
        aria-hidden="true"
        className={cn("size-[7px] shrink-0 rounded-full", toneDot[tone])}
      />
      {children}
    </span>
  )
}
