import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"

// 配置值类型。对应后端 app.ConfigValueType。
export type ConfigValueType = "string" | "number" | "boolean" | "json"

// 配置允许的作用域。对应后端 app.ConfigAllowedScope。
export type ConfigAllowedScope = "workspace" | "project"

// 对应后端 app.ConfigDefinitionView。
export type ConfigSchemaDefinition = {
  key: string
  value_type: ConfigValueType | string
  allowed_scopes: ConfigAllowedScope[] | string[]
  label: string
  description: string
  enum_values: string[]
  default_value: string | null
  required: boolean
  secret: boolean
  show_on_console_home: boolean
  created_at: number
  modified_at: number
}

// 创建/更新 schema 的输入。对应后端 configSchemaRequest。
export type ConfigSchemaInput = {
  value_type: ConfigValueType | string
  allowed_scopes: ConfigAllowedScope[] | string[]
  label: string
  description: string
  enum_values: string[]
  default_value: string | null
  required: boolean
  secret: boolean
  show_on_console_home: boolean
}

// 对应后端 app.ConfigSchemaUsageView。
export type ConfigSchemaUsage = {
  key: string
  workspace_values: number
  project_values: number
  total_values: number
}

// 有效值来源。对应后端 app.ConfigEffectiveValueView.source。
export type ConfigEffectiveSource = "project" | "workspace" | "default" | "missing"

// 对应后端 app.ConfigEffectiveValueView。
export type ConfigEffectiveValue = {
  key: string
  value: string | null
  source: ConfigEffectiveSource
  project_value?: string | null
  workspace_value?: string | null
  default_value?: string | null
  definition: ConfigSchemaDefinition
  show_on_console_home: boolean
  missing_required: boolean
}

// 后端 config-schema 路由不带 workspace query，workspace 由 scoped service 从 token 解析。
export function configSchemaPath(): string {
  return "/api/v1/config-schema"
}

export function configSchemaKeyPath(key: string): string {
  return `/api/v1/config-schema/${encodeSchemaKey(key)}`
}

export function configSchemaUsagePath(key: string): string {
  return `/api/v1/config-schema/${encodeSchemaKey(key)}/usage`
}

export function workspaceConfigEffectivePath(options: {
  consoleHome?: boolean
}): string {
  const base = "/api/v1/config/effective"
  if (options.consoleHome) {
    return `${base}?console_home=true`
  }
  return base
}

export function projectConfigEffectivePath(projectRef: string): string {
  return `/api/v1/projects/${encodeSchemaKey(projectRef)}/config/effective`
}

// schema key 含点号，按段编码（与 project-api.encodeSegment 一致），点号不转义。
function encodeSchemaKey(value: string): string {
  return encodeURIComponent(value)
}

export function listConfigSchema(): Promise<ConfigSchemaDefinition[]> {
  return workspaceApiGet<ConfigSchemaDefinition[]>(configSchemaPath())
}

export function getConfigSchema(key: string): Promise<ConfigSchemaDefinition> {
  return workspaceApiGet<ConfigSchemaDefinition>(configSchemaKeyPath(key))
}

export function setConfigSchema(
  key: string,
  input: ConfigSchemaInput
): Promise<ConfigSchemaDefinition> {
  return workspaceApiPut<ConfigSchemaDefinition>(configSchemaKeyPath(key), input)
}

export function deleteConfigSchema(
  key: string,
  purge: boolean
): Promise<void> {
  const path = purge
    ? `${configSchemaKeyPath(key)}?purge=true`
    : configSchemaKeyPath(key)
  return workspaceApiDelete<void>(path)
}

export function getConfigSchemaUsage(key: string): Promise<ConfigSchemaUsage> {
  return workspaceApiGet<ConfigSchemaUsage>(configSchemaUsagePath(key))
}

export function listWorkspaceEffectiveConfig(options: {
  consoleHome?: boolean
}): Promise<ConfigEffectiveValue[]> {
  return workspaceApiGet<ConfigEffectiveValue[]>(
    workspaceConfigEffectivePath(options)
  )
}

export function listProjectEffectiveConfig(
  projectRef: string
): Promise<ConfigEffectiveValue[]> {
  return workspaceApiGet<ConfigEffectiveValue[]>(
    projectConfigEffectivePath(projectRef)
  )
}
