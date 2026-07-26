import { describeCron } from "@/features/workspace/shared/cron-schedule"

import type {
  AutomationTriggerConfig,
  AutomationTriggerType,
} from "@/features/workspace/project-workbench/automations/project-automations-api"

// summarizeTrigger 把 trigger_config 渲染成单行摘要，桌面表格和移动 card 共用。
// 不在列表暴露完整 prompt 或原始 cron 表达式；cron 解读失败时回退为原表达式。
export function summarizeTrigger(
  triggerType: AutomationTriggerType | "manual_test",
  config: AutomationTriggerConfig
): string {
  if (triggerType === "event" || triggerType === "manual_test") {
    return config.event_type ?? triggerType
  }
  if (triggerType === "schedule") {
    if (config.schedule_type === "cron") {
      return `cron · ${describeCron(config.schedule_value ?? "")}`
    }
    return `每天 ${config.schedule_value ?? ""}`
  }
  return triggerType
}

// summarizeTriggerFull 返回带原表达式的完整摘要，tooltip 用。
export function summarizeTriggerFull(
  triggerType: AutomationTriggerType | "manual_test",
  config: AutomationTriggerConfig
): string {
  if (triggerType === "schedule") {
    const tz = config.timezone ?? "Asia/Shanghai"
    if (config.schedule_type === "cron") {
      return `${describeCron(config.schedule_value ?? "")}（cron: ${config.schedule_value ?? ""}, ${tz}）`
    }
    return `每天 ${config.schedule_value ?? ""}（${tz}）`
  }
  return summarizeTrigger(triggerType, config)
}
