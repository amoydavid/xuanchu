import { useQuery } from "@tanstack/react-query"

import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

// 模板变量触发来源。
export type TemplateVarTrigger = "reminder" | "event"

// 模板变量字段位置。
export type TemplateVarField = "endpoint" | "body"

// 单个变量描述。
export type TemplateVar = {
  name: string
  description: string
  dynamic: boolean
  prefix_group?: string
}

// API 返回的完整视图。
export type NotificationTemplateVarsView = {
  triggers: Array<{
    trigger: TemplateVarTrigger
    fields: Array<{
      field: TemplateVarField
      vars: TemplateVar[]
    }>
  }>
}

export function notificationTemplateVarsPath(): string {
  return "/api/v1/notification-template-vars"
}

export function getNotificationTemplateVars(): Promise<NotificationTemplateVarsView> {
  return workspaceApiGet<NotificationTemplateVarsView>(notificationTemplateVarsPath())
}

// react-query hook：缓存模板变量视图。
export function useNotificationTemplateVars() {
  return useQuery({
    queryKey: ["outbound", "template-vars"],
    queryFn: getNotificationTemplateVars,
    staleTime: Infinity, // 变量集稳定，永久缓存
  })
}

// 从视图中提取某 trigger × field 的变量列表。
export function selectVars(
  view: NotificationTemplateVarsView | undefined,
  trigger: TemplateVarTrigger,
  field: TemplateVarField,
): TemplateVar[] {
  if (!view) return []
  for (const t of view.triggers) {
    if (t.trigger !== trigger) continue
    for (const f of t.fields) {
      if (f.field === field) return f.vars
    }
  }
  return []
}
