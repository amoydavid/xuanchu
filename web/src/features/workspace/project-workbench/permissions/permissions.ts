export type WorkbenchPermissionInput = {
  role?: string | null
  scopes?: string[] | null
}

export function hasScope(
  scopes: string[] | null | undefined,
  scope: string
): boolean {
  return Array.isArray(scopes) && (scopes.includes("*") || scopes.includes(scope))
}

export function canTaskWrite(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "task:write") &&
    ["owner", "admin", "member"].includes(input.role ?? "")
  )
}

export function canProjectManage(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "project:write") &&
    ["owner", "admin"].includes(input.role ?? "")
  )
}

// canConfigManage 控制 workspace 级 ConfigDefinition 写入。
// config 定义是 workspace 控制面，权限收紧到 owner/admin + config:write scope。
export function canConfigManage(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "config:write") &&
    ["owner", "admin"].includes(input.role ?? "")
  )
}
