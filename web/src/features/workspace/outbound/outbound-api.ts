import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

// 出站集成控制台 API 封装。
// 把 sink、hook、notification rule、reminder rule、delivery、test 的 path 与 DTO
// 统一在一个模块，供 outbound feature 复用，避免与旧 hooks-api 重复。

// ---------------------------------------------------------------------------
// Notification sink
// ---------------------------------------------------------------------------

export type NotificationSinkType = "webhook" | "http_template"
export type NotificationEndpointMode =
  | "static_url"
  | "template"
  | "config_value"

export type HTTPHeaderTemplate = { name: string; value: string }
export type HTTPTemplateSecretRef = { alias: string; config_key: string }

export type NotificationSink = {
  id: string
  workspace_id: string
  name: string
  type: NotificationSinkType | string
  endpoint_mode: NotificationEndpointMode | string
  url?: string
  url_template?: string
  config_key?: string
  allowed_hosts?: string[]
  http_method?: string
  header_templates?: HTTPHeaderTemplate[]
  body_template?: string
  body_content_type?: string
  secret_refs?: HTTPTemplateSecretRef[]
  enabled: boolean
  timeout_seconds: number
  max_attempts: number
  max_concurrency: number
  created_at: number
  modified_at: number
}

export type NotificationSinkCreateInput = {
  name: string
  type: string
  endpoint_mode: string
  url?: string
  url_template?: string
  config_key?: string
  allowed_hosts?: string[]
  http_method?: string
  header_templates?: HTTPHeaderTemplate[]
  body_template?: string
  body_content_type?: string
  secret_refs?: HTTPTemplateSecretRef[]
  secret?: string
  timeout_seconds?: number
  max_attempts?: number
  max_concurrency?: number
}

export type NotificationSinkModifyInput = {
  name?: string
  type?: string
  endpoint_mode?: string
  url?: string
  url_template?: string
  config_key?: string
  allowed_hosts?: string[]
  http_method?: string
  header_templates?: HTTPHeaderTemplate[]
  body_template?: string
  body_content_type?: string
  secret_refs?: HTTPTemplateSecretRef[]
  secret?: string
  timeout_seconds?: number
  max_attempts?: number
  max_concurrency?: number
}

export function notificationSinkPath(sinkId?: string): string {
  if (!sinkId) return "/api/v1/notification-sinks"
  return `/api/v1/notification-sinks/${encodeURIComponent(sinkId)}`
}

export function notificationSinkEnablePath(sinkId: string): string {
  return `/api/v1/notification-sinks/${encodeURIComponent(sinkId)}/enable`
}

export function notificationSinkDisablePath(sinkId: string): string {
  return `/api/v1/notification-sinks/${encodeURIComponent(sinkId)}/disable`
}

export function notificationSinkTestPath(sinkId: string): string {
  return `/api/v1/notification-sinks/${encodeURIComponent(sinkId)}/test`
}

export function listNotificationSinks(options?: {
  includeDisabled?: boolean
}): Promise<NotificationSink[]> {
  const path = options?.includeDisabled
    ? `${notificationSinkPath()}?include_disabled=true`
    : notificationSinkPath()
  return workspaceApiGet<NotificationSink[]>(path)
}

export function createNotificationSink(
  input: NotificationSinkCreateInput
): Promise<NotificationSink> {
  return workspaceApiPost<NotificationSink>(notificationSinkPath(), input)
}

export function modifyNotificationSink(
  sinkId: string,
  input: NotificationSinkModifyInput
): Promise<NotificationSink> {
  return workspaceApiPatch<NotificationSink>(notificationSinkPath(sinkId), input)
}

export function deleteNotificationSink(sinkId: string): Promise<void> {
  return workspaceApiDelete<void>(notificationSinkPath(sinkId))
}

export function enableNotificationSink(
  sinkId: string
): Promise<NotificationSink> {
  return workspaceApiPost<NotificationSink>(
    notificationSinkEnablePath(sinkId),
    {}
  )
}

export function disableNotificationSink(
  sinkId: string
): Promise<NotificationSink> {
  return workspaceApiPost<NotificationSink>(
    notificationSinkDisablePath(sinkId),
    {}
  )
}

// ---------------------------------------------------------------------------
// Sink test
// ---------------------------------------------------------------------------

export type NotificationSinkTestInput = {
  kind?: "hook" | "notification"
  event_type?: string
  sample?: string
  project_ref?: string
}

export type NotificationSinkTestView = {
  status: "succeeded" | "failed" | string
  status_code?: number | null
  duration_ms: number
  resolved_endpoint_source: string
  resolved_endpoint_fingerprint: string
  rendered_method: string
  rendered_headers: Record<string, string[]>
  rendered_body_preview: string
  error?: string
}

export function testNotificationSink(
  sinkId: string,
  input: NotificationSinkTestInput
): Promise<NotificationSinkTestView> {
  return workspaceApiPost<NotificationSinkTestView>(
    notificationSinkTestPath(sinkId),
    input
  )
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export type Hook = {
  id: string
  name: string
  scope_type: string
  workspace_id: string
  project_id?: string | null
  event_types: string[]
  sink_id: string
  sink_name: string
  sink_type: string
  enabled: boolean
  timeout_seconds: number
  max_attempts: number
  created_at: number
  modified_at: number
}

export type HookDelivery = {
  id: string
  hook_id: string
  event_id: string
  event_type: string
  status: string
  attempt_count: number
  next_attempt_at?: number | null
  claim_expires_at?: number | null
  last_status_code?: number | null
  last_error?: string
  last_attempt_at?: number | null
  headers?: Record<string, string[]>
  payload?: Record<string, unknown> | null
  actor?: unknown
  workspace_id?: string
  project_id?: string | null
  created_at: number
  modified_at: number
}

export type HookCreateInput = {
  name: string
  scope_type: string
  project_ref?: string
  event_types: string[]
  sink: string
  timeout_seconds?: number
  max_attempts?: number
}

export type HookModifyInput = {
  name?: string
  event_types?: string[]
  sink?: string
  timeout_seconds?: number
  max_attempts?: number
}

export function hookPath(hookId?: string): string {
  if (!hookId) return "/api/v1/hooks"
  return `/api/v1/hooks/${encodeURIComponent(hookId)}`
}

export function hookEnablePath(hookId: string): string {
  return `/api/v1/hooks/${encodeURIComponent(hookId)}/enable`
}

export function hookDisablePath(hookId: string): string {
  return `/api/v1/hooks/${encodeURIComponent(hookId)}/disable`
}

export function hookDeliveriesPath(hookId: string): string {
  return `/api/v1/hooks/${encodeURIComponent(hookId)}/deliveries`
}

export function hookDeliveryPath(deliveryId: string): string {
  return `/api/v1/hook-deliveries/${encodeURIComponent(deliveryId)}`
}

export function hookDeliveryReplayPath(deliveryId: string): string {
  return `/api/v1/hook-deliveries/${encodeURIComponent(deliveryId)}/replay`
}

export function listHooks(): Promise<Hook[]> {
  return workspaceApiGet<Hook[]>(hookPath())
}

export function createHook(input: HookCreateInput): Promise<Hook> {
  return workspaceApiPost<Hook>(hookPath(), input)
}

export function modifyHook(
  hookId: string,
  input: HookModifyInput
): Promise<Hook> {
  return workspaceApiPatch<Hook>(hookPath(hookId), input)
}

export function deleteHook(hookId: string): Promise<void> {
  return workspaceApiDelete<void>(hookPath(hookId))
}

export function enableHook(hookId: string): Promise<Hook> {
  return workspaceApiPost<Hook>(hookEnablePath(hookId), {})
}

export function disableHook(hookId: string): Promise<Hook> {
  return workspaceApiPost<Hook>(hookDisablePath(hookId), {})
}

export function listHookDeliveries(hookId: string): Promise<HookDelivery[]> {
  return workspaceApiGet<HookDelivery[]>(hookDeliveriesPath(hookId))
}

export function getHookDelivery(deliveryId: string): Promise<HookDelivery> {
  return workspaceApiGet<HookDelivery>(hookDeliveryPath(deliveryId))
}

export function replayHookDelivery(
  deliveryId: string
): Promise<{ status: string }> {
  return workspaceApiPost<{ status: string }>(
    hookDeliveryReplayPath(deliveryId),
    {}
  )
}

// ---------------------------------------------------------------------------
// Notification delivery
// ---------------------------------------------------------------------------

export type NotificationDelivery = {
  id: string
  workspace_id: string
  project_id?: string | null
  rule_id: string
  sink_id: string
  task_uuid: string
  object_kind: string
  object_id: string
  recipient?: unknown
  event_id: string
  event_type: string
  resolved_url: string
  resolved_endpoint_source: string
  resolved_endpoint_fingerprint: string
  rendered_method: string
  rendered_headers: Record<string, string[]>
  rendered_body: string
  rendered_content_type: string
  payload?: Record<string, unknown> | null
  status: string
  attempt_count: number
  next_attempt_at?: number | null
  last_attempt_at?: number | null
  last_status_code?: number | null
  last_error?: string
  created_at: number
  modified_at: number
}

export type NotificationDeliveryQuery = {
  sink?: string
  status?: string
  event?: string
  limit?: number
}

export function notificationDeliveriesPath(
  query: NotificationDeliveryQuery = {}
): string {
  const params = new URLSearchParams()
  if (query.sink) params.set("sink", query.sink)
  if (query.status) params.set("status", query.status)
  if (query.event) params.set("event", query.event)
  if (query.limit) params.set("limit", String(query.limit))
  const qs = params.toString()
  return qs ? `/api/v1/notification-deliveries?${qs}` : "/api/v1/notification-deliveries"
}

export function notificationDeliveryPath(deliveryId: string): string {
  return `/api/v1/notification-deliveries/${encodeURIComponent(deliveryId)}`
}

export function notificationDeliveryReplayPath(deliveryId: string): string {
  return `/api/v1/notification-deliveries/${encodeURIComponent(deliveryId)}/replay`
}

export function listNotificationDeliveries(
  query: NotificationDeliveryQuery = {}
): Promise<NotificationDelivery[]> {
  return workspaceApiGet<NotificationDelivery[]>(
    notificationDeliveriesPath(query)
  )
}

export function getNotificationDelivery(
  deliveryId: string
): Promise<NotificationDelivery> {
  return workspaceApiGet<NotificationDelivery>(
    notificationDeliveryPath(deliveryId)
  )
}

export function replayNotificationDelivery(
  deliveryId: string
): Promise<{ status: string }> {
  return workspaceApiPost<{ status: string }>(
    notificationDeliveryReplayPath(deliveryId),
    {}
  )
}

// ---------------------------------------------------------------------------
// Notification rule / Reminder rule（Phase 3 使用）
// ---------------------------------------------------------------------------

export type NotificationRule = {
  id: string
  workspace_id: string
  project_id?: string | null
  name: string
  event_type: string
  filter_source?: string
  audience_type?: string
  recipient_user_ids?: string[]
  sink_id?: string
  template_subject?: string
  template_body?: string
  enabled: boolean
  created_at: number
  modified_at: number
}

export type NotificationRuleCreateInput = {
  name: string
  project_ref?: string
  event_type: string
  filter_source?: string
  audience_type?: string
  recipients?: string[]
  sink?: string
  template_subject?: string
  template_body?: string
}

export type NotificationRuleModifyInput = Partial<NotificationRuleCreateInput>

export function notificationRulePath(ruleId?: string): string {
  if (!ruleId) return "/api/v1/notification-rules"
  return `/api/v1/notification-rules/${encodeURIComponent(ruleId)}`
}

export function notificationRuleEnablePath(ruleId: string): string {
  return `/api/v1/notification-rules/${encodeURIComponent(ruleId)}/enable`
}

export function notificationRuleDisablePath(ruleId: string): string {
  return `/api/v1/notification-rules/${encodeURIComponent(ruleId)}/disable`
}

export function listNotificationRules(): Promise<NotificationRule[]> {
  return workspaceApiGet<NotificationRule[]>(notificationRulePath())
}

export function createNotificationRule(
  input: NotificationRuleCreateInput
): Promise<NotificationRule> {
  return workspaceApiPost<NotificationRule>(notificationRulePath(), input)
}

export function modifyNotificationRule(
  ruleId: string,
  input: NotificationRuleModifyInput
): Promise<NotificationRule> {
  return workspaceApiPatch<NotificationRule>(notificationRulePath(ruleId), input)
}

export function deleteNotificationRule(ruleId: string): Promise<void> {
  return workspaceApiDelete<void>(notificationRulePath(ruleId))
}

export function enableNotificationRule(
  ruleId: string
): Promise<NotificationRule> {
  return workspaceApiPost<NotificationRule>(
    notificationRuleEnablePath(ruleId),
    {}
  )
}

export function disableNotificationRule(
  ruleId: string
): Promise<NotificationRule> {
  return workspaceApiPost<NotificationRule>(
    notificationRuleDisablePath(ruleId),
    {}
  )
}

export type ReminderRule = {
  id: string
  workspace_id: string
  project_id?: string | null
  name: string
  trigger_type?: string
  offset_seconds?: number
  after_seconds?: number
  repeat_policy?: string
  schedule_type?: string
  schedule_value?: string
  timezone?: string
  filter_source?: string
  audience_type?: string
  recipient_user_ids?: string[]
  sink_id?: string
  enabled: boolean
  created_at: number
  modified_at: number
}

export type ReminderRuleCreateInput = {
  name: string
  project_ref?: string
  trigger_type?: string
  offset_seconds?: number
  after_seconds?: number
  repeat_policy?: string
  schedule_type?: string
  schedule_value?: string
  timezone?: string
  filter_source?: string
  audience_type?: string
  recipients?: string[]
  sink_ref?: string
}

export type ReminderRuleModifyInput = Partial<ReminderRuleCreateInput>

export function reminderRulePath(ruleId?: string): string {
  if (!ruleId) return "/api/v1/reminder-rules"
  return `/api/v1/reminder-rules/${encodeURIComponent(ruleId)}`
}

export function reminderRuleEnablePath(ruleId: string): string {
  return `/api/v1/reminder-rules/${encodeURIComponent(ruleId)}/enable`
}

export function reminderRuleDisablePath(ruleId: string): string {
  return `/api/v1/reminder-rules/${encodeURIComponent(ruleId)}/disable`
}

export function listReminderRules(): Promise<ReminderRule[]> {
  return workspaceApiGet<ReminderRule[]>(reminderRulePath())
}

export function createReminderRule(
  input: ReminderRuleCreateInput
): Promise<ReminderRule> {
  return workspaceApiPost<ReminderRule>(reminderRulePath(), input)
}

export function modifyReminderRule(
  ruleId: string,
  input: ReminderRuleModifyInput
): Promise<ReminderRule> {
  return workspaceApiPatch<ReminderRule>(reminderRulePath(ruleId), input)
}

export function deleteReminderRule(ruleId: string): Promise<void> {
  return workspaceApiDelete<void>(reminderRulePath(ruleId))
}

export function enableReminderRule(ruleId: string): Promise<ReminderRule> {
  return workspaceApiPost<ReminderRule>(reminderRuleEnablePath(ruleId), {})
}

export function disableReminderRule(ruleId: string): Promise<ReminderRule> {
  return workspaceApiPost<ReminderRule>(reminderRuleDisablePath(ruleId), {})
}

// ---------------------------------------------------------------------------
// Project helper（hook project scope 选择器使用）
// ---------------------------------------------------------------------------

export type ProjectSummary = {
  id: string
  workspace_id?: string
  slug: string
  name: string
  status?: string
  archived?: boolean
}

export function projectsListPath(workspaceSlug?: string): string {
  const params = new URLSearchParams()
  if (workspaceSlug) params.set("workspace", workspaceSlug)
  params.set("status", "all")
  const qs = params.toString()
  return qs ? `/api/v1/projects?${qs}` : "/api/v1/projects"
}

export function listProjects(workspaceSlug?: string): Promise<ProjectSummary[]> {
  return workspaceApiGet<ProjectSummary[]>(projectsListPath(workspaceSlug))
}
