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

// canTaskRead 判断是否可读任务，用于 ProjectSummary 是否请求、Tasks 页是否可读。
// 所有可登录成员（owner/admin/member/viewer）在持有 task:read scope 时均可读任务。
export function canTaskRead(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "task:read") &&
    ["owner", "admin", "member", "viewer"].includes(input.role ?? "")
  )
}

// canAuditRead 判断是否可读审计。audit:read 仅 owner/admin 在持有 scope 时获得。
export function canAuditRead(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "audit:read") &&
    ["owner", "admin"].includes(input.role ?? "")
  )
}

export function canProjectManage(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "project:write") &&
    ["owner", "admin"].includes(input.role ?? "")
  )
}

// canProjectAutomationRead 校验项目自动化读权限：project:read + hook:read。
// 与后端 requireProjectAutomationRead 对齐，所有可登录成员在持有双 scope 时可读。
export function canProjectAutomationRead(input: WorkbenchPermissionInput): boolean {
  return (
    canProjectRead(input) &&
    hasScope(input.scopes, "hook:read") &&
    ["owner", "admin", "member", "viewer"].includes(input.role ?? "")
  )
}

// canProjectAutomationWrite 校验项目自动化写权限：project:write + hook:write。
// 与后端 requireProjectAutomationWrite 对齐，收紧到 owner/admin。
export function canProjectAutomationWrite(input: WorkbenchPermissionInput): boolean {
  return canProjectManage(input) && hasScope(input.scopes, "hook:write")
}

// canProjectRead 判断是否可读项目，用于自动化读权限组合。
export function canProjectRead(input: WorkbenchPermissionInput): boolean {
  return (
    hasScope(input.scopes, "project:read") &&
    ["owner", "admin", "member", "viewer"].includes(input.role ?? "")
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
