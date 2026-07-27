import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"
import type {
  AutomationActionConfig,
  AutomationCondition,
  AutomationContextConfig,
  AutomationDeliveryStatus,
  AutomationTriggerConfig,
  AutomationTriggerType,
} from "@/features/workspace/project-workbench/automations/project-automations-api"

// Workspace Automation 类型与 Project Automation 共享 action/condition/context/trigger
// 形状；唯一区别是 scope_type/scope_id 和可选 project_id。

export type AutomationScopeType = "workspace" | "project"

export type WorkspaceAutomationRule = {
  id: string
  workspace_id: string
  scope_type: AutomationScopeType
  scope_id: string
  project_id?: string
  name: string
  description: string
  enabled: boolean
  trigger_type: AutomationTriggerType
  trigger_config: AutomationTriggerConfig
  condition: AutomationCondition
  action_type: "openai_compatible"
  action: AutomationActionConfig
  context: AutomationContextConfig
  instruction_template: string
  system_prompt: string
  created_by: unknown
  created_at: number
  modified_at: number
  last_delivery?: {
    delivery_id: string
    status: AutomationDeliveryStatus
    response_status_code?: number
    created_at: number
  }
}

export type WorkspaceAutomationRuleInput = Omit<
  WorkspaceAutomationRule,
  | "id"
  | "workspace_id"
  | "scope_type"
  | "scope_id"
  | "project_id"
  | "action_type"
  | "created_by"
  | "created_at"
  | "modified_at"
  | "last_delivery"
>

export type WorkspaceAutomationDelivery = {
  id: string
  workspace_id: string
  rule_scope_type: AutomationScopeType
  rule_scope_id: string
  project_id?: string
  rule_id: string
  trigger_type: AutomationTriggerType
  event_id: string
  event_type: string
  dedupe_key: string
  replay_of_delivery_id?: string
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
  max_attempts: number
  next_attempt_at?: number
  last_error: string
  created_at: number
  modified_at: number
  project?: {
    id: string
    slug: string
    name: string
    status: string
  }
}

// 安全 Provider config DTO：永远不暴露 api_key 原值。
export type AutomationProviderConfig = {
  base_url: string
  model: string
  allowed_hosts: string[]
  api_key_set: boolean
  complete: boolean
  missing_fields: string[]
}

export type AutomationProviderConfigInput = {
  base_url: string
  model: string
  allowed_hosts: string[]
  api_key?: string
  clear_api_key?: boolean
}

const RULES_KEY = ["workspace-automations", "rules"] as const
const DELIVERIES_KEY = ["workspace-automations", "deliveries"] as const
const PROVIDER_CONFIG_KEY = ["workspace-automations", "provider-config"] as const
const TEMPLATE_VARS_KEY = ["workspace-automations", "template-vars"] as const

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

export function useWorkspaceAutomationTemplateVars() {
  return useQuery({
    queryKey: TEMPLATE_VARS_KEY,
    queryFn: () =>
      workspaceApiGet<AutomationTemplateVarsView>(
        "/api/v1/automations/template-vars",
      ),
    staleTime: Infinity,
  })
}

export function useWorkspaceAutomationRules(enabledOnly: boolean = false) {
  return useQuery({
    queryKey: [...RULES_KEY, enabledOnly] as const,
    queryFn: () =>
      workspaceApiGet<WorkspaceAutomationRule[]>(
        `/api/v1/automations${enabledOnly ? "" : "?all=true"}`,
      ),
  })
}

export function useWorkspaceAutomationRule(ruleId: string | undefined) {
  return useQuery({
    queryKey: [...RULES_KEY, ruleId] as const,
    queryFn: () =>
      workspaceApiGet<WorkspaceAutomationRule>(
        `/api/v1/automations/${ruleId}`,
      ),
    enabled: Boolean(ruleId),
  })
}

export function useCreateWorkspaceAutomationRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: WorkspaceAutomationRuleInput) =>
      workspaceApiPost<WorkspaceAutomationRule>(
        "/api/v1/automations",
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: RULES_KEY })
    },
  })
}

export function useModifyWorkspaceAutomationRule(ruleId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: WorkspaceAutomationRuleInput) =>
      workspaceApiPatch<WorkspaceAutomationRule>(
        `/api/v1/automations/${ruleId}`,
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: RULES_KEY })
    },
  })
}

export function useDeleteWorkspaceAutomationRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (ruleId: string) =>
      workspaceApiDelete<{ deleted: boolean }>(
        `/api/v1/automations/${ruleId}`,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: RULES_KEY })
    },
  })
}

export function useToggleWorkspaceAutomationRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (vars: { ruleId: string; enable: boolean }) => {
      const action = vars.enable ? "enable" : "disable"
      return workspaceApiPost<WorkspaceAutomationRule>(
        `/api/v1/automations/${vars.ruleId}/${action}`,
        {},
      )
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: RULES_KEY })
    },
  })
}

export type WorkspaceAutomationDeliveryListInput = {
  rule_id?: string
  project_ref?: string
  status?: AutomationDeliveryStatus
  trigger_type?: AutomationTriggerType
  q?: string
  limit?: number
  offset?: number
}

export function useWorkspaceAutomationDeliveries(
  input: WorkspaceAutomationDeliveryListInput,
) {
  const params = new URLSearchParams()
  if (input.rule_id) params.set("rule_id", input.rule_id)
  if (input.project_ref) params.set("project_ref", input.project_ref)
  if (input.status) params.set("status", input.status)
  if (input.trigger_type) params.set("trigger_type", input.trigger_type)
  if (input.q) params.set("q", input.q)
  if (input.limit) params.set("limit", String(input.limit))
  if (input.offset) params.set("offset", String(input.offset))
  const query = params.toString()
  return useQuery({
    queryKey: [...DELIVERIES_KEY, input] as const,
    queryFn: async () => {
      const data = await workspaceApiGet<WorkspaceAutomationDelivery[]>(
        `/api/v1/automation-deliveries${query ? `?${query}` : ""}`,
      )
      return data
    },
  })
}

export function useReplayWorkspaceAutomationDelivery() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (deliveryId: string) =>
      workspaceApiPost<WorkspaceAutomationDelivery>(
        `/api/v1/automation-deliveries/${deliveryId}/replay`,
        {},
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: DELIVERIES_KEY })
    },
  })
}

export function useWorkspaceAutomationProviderConfig() {
  return useQuery({
    queryKey: PROVIDER_CONFIG_KEY,
    queryFn: () =>
      workspaceApiGet<AutomationProviderConfig>(
        "/api/v1/automations/provider-config",
      ),
  })
}

export function useUpdateWorkspaceAutomationProviderConfig() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: AutomationProviderConfigInput) =>
      workspaceApiPut<AutomationProviderConfig>(
        "/api/v1/automations/provider-config",
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: PROVIDER_CONFIG_KEY })
    },
  })
}

// Workspace Automation 允许的事件白名单：首版只有 project.created。
export const WORKSPACE_AUTOMATION_EVENTS = [
  { value: "project.created", label: "project.created · 项目创建完成" },
] as const
