import { useQuery } from "@tanstack/react-query"

import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

export type AutomationTriggerType = "schedule" | "event"
export type AutomationActionType = "openai_compatible"
export type AutomationDeliveryStatus =
  | "queued"
  | "delivering"
  | "retry_wait"
  | "succeeded"
  | "dead_lettered"

export type AutomationTriggerConfig = {
  schedule_type?: "daily_at" | "cron"
  schedule_value?: string
  timezone?: string
  event_type?: string
}

export type AutomationCondition = {
  task_filter?: string
  max_tasks?: number
  only_added_assignees?: boolean
}

export type AutomationActionConfig = {
  protocol: "chat_completions"
  base_url_config_key: string
  api_key_config_key: string
  model_config_key: string
  allowed_hosts_config_key?: string
  model_override?: string
  temperature: number
  max_attempts?: number
  attach_metadata?: boolean
}

export type AutomationContextConfig = {
  include: string[]
}

export type ProjectAutomationRule = {
  id: string
  workspace_id: string
  project_id: string
  name: string
  description: string
  enabled: boolean
  trigger_type: AutomationTriggerType
  trigger_config: AutomationTriggerConfig
  condition: AutomationCondition
  action_type: AutomationActionType
  action: AutomationActionConfig
  context: AutomationContextConfig
  instruction_template: string
  system_prompt: string
  created_at: number
  modified_at: number
}

export type ProjectAutomationRuleInput = Omit<
  ProjectAutomationRule,
  "id" | "workspace_id" | "project_id" | "action_type" | "created_at" | "modified_at"
>

export type ProjectAutomationPreview = {
  method: string
  url: string
  headers: Record<string, string>
  body: unknown
  warnings: string[]
}

export type ProjectAutomationDelivery = {
  id: string
  workspace_id: string
  project_id: string
  rule_id: string
  trigger_type: AutomationTriggerType | "manual_test"
  event_id: string
  event_type: string
  status: AutomationDeliveryStatus
  resolved_url: string
  rendered_method: string
  rendered_headers: Record<string, string[]>
  request_body_preview: string
  request_body_hash: string
  response_status_code?: number
  response_body_preview: string
  provider_request_id: string
  usage: Record<string, unknown>
  attempt_count: number
  next_attempt_at?: number
  last_error: string
  created_at: number
  modified_at: number
}

export async function listProjectAutomations(projectSlug: string, includeDisabled = true) {
  const q = includeDisabled ? "?all=true" : ""
  return workspaceApiGet<ProjectAutomationRule[]>(`/api/v1/projects/${projectSlug}/automations${q}`)
}

// AutomationProviderConfigKey 是项目自动化需要的 Agent Provider 配置 key。
export const AUTOMATION_PROVIDER_KEYS = [
  "agent.provider.base_url",
  "agent.provider.api_key",
  "agent.provider.model",
  "agent.provider.allowed_hosts",
] as const

// AutomationProviderConfig 是 Agent Provider 配置卡片读取/保存的最小结构。
// api_key 展示时遮掩为布尔「已设置」，不回显明文。
export type AutomationProviderConfig = {
  base_url: string
  api_key_set: boolean
  model: string
  allowed_hosts: string
}

export const EMPTY_AUTOMATION_PROVIDER_CONFIG: AutomationProviderConfig = {
  base_url: "",
  api_key_set: false,
  model: "",
  allowed_hosts: "",
}

export async function previewProjectAutomation(projectSlug: string, input: ProjectAutomationRuleInput) {
  return workspaceApiPost<ProjectAutomationPreview>(`/api/v1/projects/${projectSlug}/automations/preview`, input)
}

export async function createProjectAutomation(projectSlug: string, input: ProjectAutomationRuleInput) {
  return workspaceApiPost<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations`, input)
}

export async function updateProjectAutomation(projectSlug: string, ruleID: string, input: Partial<ProjectAutomationRuleInput>) {
  return workspaceApiPatch<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations/${ruleID}`, input)
}

export async function deleteProjectAutomation(projectSlug: string, ruleID: string) {
  return workspaceApiDelete<{ deleted: true }>(`/api/v1/projects/${projectSlug}/automations/${ruleID}`)
}

export async function enableProjectAutomationRule(projectSlug: string, ruleID: string) {
  return workspaceApiPost<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations/${ruleID}/enable`)
}

export async function disableProjectAutomationRule(projectSlug: string, ruleID: string) {
  return workspaceApiPost<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations/${ruleID}/disable`)
}

export async function testProjectAutomationRule(projectSlug: string, ruleID: string) {
  return workspaceApiPost<ProjectAutomationDelivery>(`/api/v1/projects/${projectSlug}/automations/${ruleID}/test`)
}

export async function listProjectAutomationDeliveries(projectSlug: string, ruleID?: string) {
  const q = ruleID ? `?rule=${encodeURIComponent(ruleID)}` : ""
  return workspaceApiGet<ProjectAutomationDelivery[]>(`/api/v1/projects/${projectSlug}/automation-deliveries${q}`)
}

export async function getProjectAutomationDelivery(projectSlug: string, deliveryID: string) {
  return workspaceApiGet<ProjectAutomationDelivery>(`/api/v1/projects/${projectSlug}/automation-deliveries/${deliveryID}`)
}

export type AutomationTemplateVar = {
  name: string
  description: string
  is_prefix?: boolean
}

export type AutomationTemplateVarsView = {
  triggers: Array<{
    trigger: string
    vars: AutomationTemplateVar[]
  }>
}

export async function getAutomationTemplateVars(projectSlug: string) {
  return workspaceApiGet<AutomationTemplateVarsView>(`/api/v1/projects/${projectSlug}/automation-template-vars`)
}

export function useAutomationTemplateVars(projectSlug: string) {
  return useQuery({
    queryKey: ["automation-template-vars", projectSlug],
    queryFn: () => getAutomationTemplateVars(projectSlug),
    staleTime: Infinity,
  })
}
