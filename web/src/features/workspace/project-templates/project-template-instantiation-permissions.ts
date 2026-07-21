import type { ComponentCounts } from "./api/project-template-api"
import { hasScope } from "../project-workbench/permissions/permissions"

export function canInstantiateProjectTemplate(
  canManage: boolean,
  writeScopes: string[] | null | undefined,
  counts: ComponentCounts
) {
  if (!canManage) return false
  if (
    (counts.tasks > 0 || counts.series > 0) &&
    !hasScope(writeScopes, "task:write")
  ) {
    return false
  }
  if (counts.configs > 0 && !hasScope(writeScopes, "config:write")) {
    return false
  }
  return counts.automations === 0 || hasScope(writeScopes, "hook:write")
}
