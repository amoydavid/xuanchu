import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

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
  last_status_code?: number | null
  last_error?: string
  last_attempt_at?: number | null
  created_at: number
  modified_at: number
}

export type HookCreateInput = {
  name: string
  scope_type: string
  project_ref?: string
  event_types: string[]
  sink: string
  url?: string
  endpoint_url?: string
  timeout_seconds?: number
  max_attempts?: number
}

export type HookModifyInput = {
  name?: string
  event_types?: string[]
  sink?: string
  url?: string
  endpoint_url?: string
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

export function hookDeliveryReplayPath(deliveryId: string): string {
  return `/api/v1/hook-deliveries/${encodeURIComponent(deliveryId)}/replay`
}

export function listHooks(): Promise<Hook[]> {
  return workspaceApiGet<Hook[]>(hookPath())
}

export function createHook(input: HookCreateInput): Promise<Hook> {
  return workspaceApiPost<Hook>(hookPath(), input)
}

export function modifyHook(hookId: string, input: HookModifyInput): Promise<Hook> {
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

export function replayHookDelivery(deliveryId: string): Promise<{ status: string }> {
  return workspaceApiPost<{ status: string }>(hookDeliveryReplayPath(deliveryId), {})
}
