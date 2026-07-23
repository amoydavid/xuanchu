import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"

export type CustomFieldType = "string" | "numeric" | "date" | "duration"
export type CustomFieldSource = "database" | "runtime" | "database_override"

export type WorkspaceCustomField = {
  name: string
  type: CustomFieldType
  label: string
  values: string[]
  default: string
  source: CustomFieldSource
  task_value_count: number
  active_series_value_count: number
}

export type WorkspaceCustomFieldInput = {
  type: CustomFieldType
  label: string
  values: string[]
  default: string
}

export function workspaceCustomFieldsPath(workspaceSlug: string): string {
  return `/api/v1/udas?workspace=${encodeURIComponent(workspaceSlug)}`
}

export function workspaceCustomFieldPath(
  workspaceSlug: string,
  name: string
): string {
  return `/api/v1/udas/${encodeURIComponent(name)}?workspace=${encodeURIComponent(workspaceSlug)}`
}

export function listWorkspaceCustomFields(
  workspaceSlug: string
): Promise<WorkspaceCustomField[]> {
  return workspaceApiGet<WorkspaceCustomField[]>(
    workspaceCustomFieldsPath(workspaceSlug)
  )
}

export function setWorkspaceCustomField(
  workspaceSlug: string,
  name: string,
  input: WorkspaceCustomFieldInput
): Promise<WorkspaceCustomField> {
  return workspaceApiPut<WorkspaceCustomField>(
    workspaceCustomFieldPath(workspaceSlug, name),
    input
  )
}

export function deleteWorkspaceCustomField(
  workspaceSlug: string,
  name: string
): Promise<void> {
  return workspaceApiDelete<void>(workspaceCustomFieldPath(workspaceSlug, name))
}
