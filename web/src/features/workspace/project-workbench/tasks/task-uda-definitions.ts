import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

export type TaskUDAType = "string" | "numeric" | "date" | "duration"

export type TaskUDADefinition = {
  name: string
  type: TaskUDAType
  label: string
  values: string[]
  defaultValue: string | null
}

const supportedTypes = new Set<TaskUDAType>([
  "string",
  "numeric",
  "date",
  "duration",
])

export function parseTaskUDADefinitions(
  config: Record<string, string>
): TaskUDADefinition[] {
  const definitions = new Map<
    string,
    Partial<TaskUDADefinition> & { name: string }
  >()
  for (const [key, raw] of Object.entries(config)) {
    if (!key.startsWith("uda.")) continue
    const body = key.slice("uda.".length)
    const splitAt = body.lastIndexOf(".")
    if (splitAt <= 0) continue
    const name = body.slice(0, splitAt).trim()
    const field = body.slice(splitAt + 1)
    if (!name) continue
    const definition = definitions.get(name) ?? { name }
    switch (field) {
      case "type":
        if (supportedTypes.has(raw as TaskUDAType)) {
          definition.type = raw as TaskUDAType
        }
        break
      case "label":
        definition.label = raw.trim()
        break
      case "values":
        definition.values = raw
          .split(",")
          .map((value) => value.trim())
          .filter(Boolean)
        break
      case "default":
        definition.defaultValue = raw
        break
    }
    definitions.set(name, definition)
  }
  return Array.from(definitions.values())
    .filter(
      (definition): definition is TaskUDADefinition =>
        definition.type != null && supportedTypes.has(definition.type)
    )
    .map((definition) => ({
      name: definition.name,
      type: definition.type,
      label: definition.label || definition.name,
      values: definition.values ?? [],
      defaultValue: definition.defaultValue ?? null,
    }))
    .sort((left, right) =>
      left.label.localeCompare(right.label, undefined, { sensitivity: "base" })
    )
}

export async function listWorkspaceTaskUDADefinitions(
  workspaceSlug: string
): Promise<TaskUDADefinition[]> {
  const config = await workspaceApiGet<Record<string, string>>(
    `/api/v1/config?workspace=${encodeURIComponent(workspaceSlug)}`
  )
  return parseTaskUDADefinitions(config)
}
