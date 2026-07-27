import { cn } from "@/lib/utils"
import type { AutomationDeliveryStatus } from "@/features/workspace/project-workbench/automations/project-automations-api"

// 状态点：7px 圆点，颜色按状态区分。遵循 DESIGN.md：状态只用小圆点，绝不大面积铺色。
export function RuleStatusDot({ enabled }: { enabled: boolean }) {
  return (
    <span
      aria-label={enabled ? "启用" : "停用"}
      className={cn(
        "inline-block size-[7px] shrink-0 rounded-full",
        enabled ? "bg-primary" : "bg-muted-foreground/40"
      )}
    />
  )
}

// Delivery 状态点：颜色按 delivery 状态语义区分。
export function DeliveryStatusDot({ status }: { status: AutomationDeliveryStatus }) {
  const color = deliveryDotColor(status)
  return (
    <span
      aria-label={status}
      className={cn("inline-block size-[7px] shrink-0 rounded-full", color)}
    />
  )
}

export function deliveryDotColor(status: AutomationDeliveryStatus): string {
  switch (status) {
    case "succeeded":
      return "bg-primary"
    case "dead_lettered":
      return "bg-destructive"
    case "retry_wait":
      return "bg-amber-500"
    case "delivering":
      return "bg-primary/60 animate-automation-delivering"
    default:
      return "bg-muted-foreground/40"
  }
}

// deliveryStatusLabel 把状态码翻译成中文短标签，桌面表与移动 card 共用。
export function deliveryStatusLabel(status: AutomationDeliveryStatus): string {
  switch (status) {
    case "queued":
      return "排队中"
    case "delivering":
      return "投递中"
    case "retry_wait":
      return "等待重试"
    case "succeeded":
      return "成功"
    case "dead_lettered":
      return "失败"
    default:
      return status
  }
}
