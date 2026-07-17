// Token scope 常量。与后端 internal/auth/scope.go scopeRegistry 对齐。
// 所有 scope 字符串集中定义，避免散落各处的字面量拼写错误。

// 特殊 scope
export const SCOPE_IMPERSONATE = "impersonate" as const

// resource + action 组合
export const SCOPE_TASK_READ = "task:read" as const
export const SCOPE_TASK_WRITE = "task:write" as const
export const SCOPE_PROJECT_READ = "project:read" as const
export const SCOPE_PROJECT_WRITE = "project:write" as const
export const SCOPE_CONTEXT_READ = "context:read" as const
export const SCOPE_CONTEXT_WRITE = "context:write" as const
export const SCOPE_CONFIG_READ = "config:read" as const
export const SCOPE_CONFIG_WRITE = "config:write" as const
export const SCOPE_WORKSPACE_READ = "workspace:read" as const
export const SCOPE_WORKSPACE_WRITE = "workspace:write" as const
export const SCOPE_AUDIT_READ = "audit:read" as const
export const SCOPE_USER_READ = "user:read" as const
export const SCOPE_USER_WRITE = "user:write" as const
export const SCOPE_MEMBER_READ = "member:read" as const
export const SCOPE_MEMBER_WRITE = "member:write" as const
export const SCOPE_TOKEN_READ = "token:read" as const
export const SCOPE_TOKEN_WRITE = "token:write" as const
export const SCOPE_HOOK_READ = "hook:read" as const
export const SCOPE_HOOK_WRITE = "hook:write" as const
export const SCOPE_NOTIFICATION_READ = "notification:read" as const
export const SCOPE_NOTIFICATION_WRITE = "notification:write" as const
export const SCOPE_REMINDER_READ = "reminder:read" as const
export const SCOPE_REMINDER_WRITE = "reminder:write" as const

// scope 分组定义，供 ScopeEditor 渲染。与后端 scopeRegistry 资源对齐。
export type ScopeGroup = {
  /** 该组下的所有 scope（如 ["task:read", "task:write"]） */
  scopes: string[]
  /** i18n 分组名 key */
  i18nKey: string
}

export const SCOPE_GROUPS: ScopeGroup[] = [
  { scopes: [SCOPE_TASK_READ, SCOPE_TASK_WRITE], i18nKey: "token.scopeGroup.task" },
  { scopes: [SCOPE_PROJECT_READ, SCOPE_PROJECT_WRITE], i18nKey: "token.scopeGroup.project" },
  { scopes: [SCOPE_CONTEXT_READ, SCOPE_CONTEXT_WRITE], i18nKey: "token.scopeGroup.context" },
  { scopes: [SCOPE_CONFIG_READ, SCOPE_CONFIG_WRITE], i18nKey: "token.scopeGroup.config" },
  { scopes: [SCOPE_WORKSPACE_READ, SCOPE_WORKSPACE_WRITE], i18nKey: "token.scopeGroup.workspace" },
  { scopes: [SCOPE_AUDIT_READ], i18nKey: "token.scopeGroup.audit" },
  { scopes: [SCOPE_USER_READ, SCOPE_USER_WRITE], i18nKey: "token.scopeGroup.user" },
  { scopes: [SCOPE_MEMBER_READ, SCOPE_MEMBER_WRITE], i18nKey: "token.scopeGroup.member" },
  { scopes: [SCOPE_TOKEN_READ, SCOPE_TOKEN_WRITE], i18nKey: "token.scopeGroup.token" },
  { scopes: [SCOPE_HOOK_READ, SCOPE_HOOK_WRITE], i18nKey: "token.scopeGroup.hook" },
  { scopes: [SCOPE_NOTIFICATION_READ, SCOPE_NOTIFICATION_WRITE], i18nKey: "token.scopeGroup.notification" },
  { scopes: [SCOPE_REMINDER_READ, SCOPE_REMINDER_WRITE], i18nKey: "token.scopeGroup.reminder" },
]

export const TENANT_ACCESS_TOKEN_SCOPES: ReadonlySet<string> = new Set([
  SCOPE_TASK_READ,
  SCOPE_TASK_WRITE,
  SCOPE_PROJECT_READ,
  SCOPE_PROJECT_WRITE,
  SCOPE_CONTEXT_READ,
  SCOPE_CONTEXT_WRITE,
  SCOPE_CONFIG_READ,
  SCOPE_CONFIG_WRITE,
  SCOPE_WORKSPACE_READ,
  SCOPE_WORKSPACE_WRITE,
  SCOPE_AUDIT_READ,
  SCOPE_USER_READ,
  SCOPE_USER_WRITE,
  SCOPE_MEMBER_READ,
  SCOPE_MEMBER_WRITE,
  SCOPE_TOKEN_READ,
  SCOPE_TOKEN_WRITE,
  SCOPE_HOOK_READ,
  SCOPE_HOOK_WRITE,
  SCOPE_NOTIFICATION_READ,
  SCOPE_NOTIFICATION_WRITE,
  SCOPE_REMINDER_READ,
  SCOPE_REMINDER_WRITE,
])

/** 所有已知 scope（分组定义的 + impersonate），用于识别未知/历史遗留 scope。 */
export const KNOWN_SCOPES: ReadonlySet<string> = new Set([
  ...SCOPE_GROUPS.flatMap((g) => g.scopes),
  SCOPE_IMPERSONATE,
])
