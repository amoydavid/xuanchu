import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"
import { type ConfigEffectiveValue } from "@/features/workspace/config/config-definition-api"

// 按 slug 寻址的 workspace config。后端 /api/v1/config* 原生支持 ?workspace= query（requestWorkspaceRef）。
// 与现有 listWorkspaceEffectiveConfig（无 slug，依赖 token）区分，用 ForSlug 后缀。
function workspaceQuery(workspaceSlug: string): string {
  return `workspace=${encodeURIComponent(workspaceSlug)}`
}

// path helper 用 ForSlug 后缀，与 config-definition-api.ts 里无 slug 版（依赖 token）的
// workspaceConfigEffectivePath 区分，避免重名混淆。
export function workspaceConfigEffectivePathForSlug(workspaceSlug: string): string {
  return `/api/v1/config/effective?${workspaceQuery(workspaceSlug)}`
}

export function workspaceConfigKeyPathForSlug(
  workspaceSlug: string,
  key: string
): string {
  return `/api/v1/config/${encodeURIComponent(key)}?${workspaceQuery(workspaceSlug)}`
}

export function listWorkspaceEffectiveConfigForSlug(
  workspaceSlug: string
): Promise<ConfigEffectiveValue[]> {
  return workspaceApiGet<ConfigEffectiveValue[]>(
    workspaceConfigEffectivePathForSlug(workspaceSlug)
  )
}

export function setWorkspaceConfig(
  workspaceSlug: string,
  key: string,
  value: string
): Promise<void> {
  return workspaceApiPut<void>(workspaceConfigKeyPathForSlug(workspaceSlug, key), {
    value,
  })
}

export function deleteWorkspaceConfig(
  workspaceSlug: string,
  key: string
): Promise<void> {
  return workspaceApiDelete<void>(workspaceConfigKeyPathForSlug(workspaceSlug, key))
}
