import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

// 对应后端 app.ConfigDefinitionView。
export type ConfigSchemaDefinition = {
  key: string
  value_type: string
  allowed_scopes: string[]
  label: string
  description: string
  enum_values: string[]
  default_value: string | null
  required: boolean
  secret: boolean
  created_at: number
  modified_at: number
}

export function configSchemaPath(): string {
  return "/api/v1/config-schema"
}

export function listConfigSchema(): Promise<ConfigSchemaDefinition[]> {
  return workspaceApiGet<ConfigSchemaDefinition[]>(configSchemaPath())
}
