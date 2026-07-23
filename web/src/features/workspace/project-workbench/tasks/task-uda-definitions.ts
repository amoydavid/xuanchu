import { listWorkspaceCustomFields } from "@/features/workspace/custom-fields/custom-field-api"

export type TaskUDAType = "string" | "numeric" | "date" | "duration"

export type TaskUDADefinition = {
  name: string
  type: TaskUDAType
  label: string
  values: string[]
  defaultValue: string | null
}

export async function listWorkspaceTaskUDADefinitions(
  workspaceSlug: string
): Promise<TaskUDADefinition[]> {
  const fields = await listWorkspaceCustomFields(workspaceSlug)
  return fields
    .map((field) => ({
      name: field.name,
      type: field.type,
      label: field.label || field.name,
      values: field.values,
      defaultValue: field.default || null,
    }))
    .sort((left, right) =>
      left.label.localeCompare(right.label, undefined, { sensitivity: "base" })
    )
}
