// 出站集成事件白名单。第一阶段硬编码，必须与 internal/app/hook.go allowedHookEventTypes 对齐。
// 后端白名单稳定后再考虑新增 GET /api/v1/hook-event-types。

export const TASK_BASIC_EVENTS = [
  "task.created",
  "task.modified",
  "task.completed",
  "task.deleted",
] as const

export const TASK_STATUS_EVENTS = [
  "task.started",
  "task.stopped",
  "task.blocked",
  "task.unblocked",
] as const

export const TASK_FIELDS_EVENTS = [
  "task.assigned",
  "task.unassigned",
  "task.due_changed",
  "task.priority_changed",
  "task.project_changed",
  "task.tags_changed",
] as const

export const PROJECT_EVENTS = [
  "project.archived",
  "project.transitioned",
  "project.annotated",
  "project.denotated",
] as const

export const ALL_HOOK_EVENT_TYPES: readonly string[] = [
  ...TASK_BASIC_EVENTS,
  ...TASK_STATUS_EVENTS,
  ...TASK_FIELDS_EVENTS,
  ...PROJECT_EVENTS,
]

export type HookEventGroup = {
  /** i18n key 后缀，例如 "task.basic" */
  id: string
  events: readonly string[]
}

export const HOOK_EVENT_GROUPS: HookEventGroup[] = [
  { id: "task.basic", events: TASK_BASIC_EVENTS },
  { id: "task.status", events: TASK_STATUS_EVENTS },
  { id: "task.fields", events: TASK_FIELDS_EVENTS },
  { id: "project", events: PROJECT_EVENTS },
]

export function isValidHookEventType(eventType: string): boolean {
  return ALL_HOOK_EVENT_TYPES.includes(eventType)
}
