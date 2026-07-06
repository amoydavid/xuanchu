import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import { ProjectConfigRow } from "@/features/workspace/project-workbench/project/project-config-row"
import {
  deleteProjectConfig,
  listProjectConfig,
  setProjectConfig,
  type ProjectConfigEntry,
} from "@/features/workspace/project-workbench/api/project-api"
import {
  listConfigSchema,
  type ConfigSchemaDefinition,
} from "@/features/workspace/project-workbench/api/config-schema-api"

type ProjectConfigTabProps = {
  canManage: boolean
  closed: boolean
  projectSlug: string
  workspaceSlug: string
}

export function ProjectConfigTab({
  canManage,
  closed,
  projectSlug,
  workspaceSlug,
}: ProjectConfigTabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const configQuery = useQuery<ProjectConfigEntry[]>({
    queryKey: ["project", workspaceSlug, projectSlug, "config"],
    queryFn: () => listProjectConfig(workspaceSlug, projectSlug),
  })
  const schemaQuery = useQuery({
    queryKey: ["config-schema", workspaceSlug],
    queryFn: () => listConfigSchema(),
  })

  const schemaMap = useMemo(() => {
    const m = new Map<string, ConfigSchemaDefinition>()
    for (const def of schemaQuery.data ?? []) {
      m.set(def.key, def)
    }
    return m
  }, [schemaQuery.data])

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["project", workspaceSlug, projectSlug, "config"],
    })

  const saveMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setProjectConfig(workspaceSlug, projectSlug, key, value),
    onSuccess: invalidate,
  })

  const delMut = useMutation({
    mutationFn: (key: string) =>
      deleteProjectConfig(workspaceSlug, projectSlug, key),
    onSuccess: invalidate,
  })

  const [newKey, setNewKey] = useState("")
  const [newValue, setNewValue] = useState("")
  const [addError, setAddError] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)

  const existingKeys = useMemo(
    () => new Set((configQuery.data ?? []).map((e) => e.key)),
    [configQuery.data]
  )
  const trimmedKey = newKey.trim()
  const newSchema = trimmedKey ? schemaMap.get(trimmedKey) : undefined
  const newEnumValues = newSchema?.enum_values ?? []
  const newIsSecret = newSchema?.secret === true
  const keyDuplicate = existingKeys.has(trimmedKey)

  const submitAdd = async () => {
    const key = trimmedKey
    if (!key) return
    setAdding(true)
    setAddError(null)
    try {
      await setProjectConfig(workspaceSlug, projectSlug, key, newValue)
      setNewKey("")
      setNewValue("")
      await invalidate()
    } catch (err) {
      setAddError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setAdding(false)
    }
  }

  return (
    <section className="space-y-3 border bg-card p-4">
      <div>
        <h2 className="text-sm font-medium">
          {t("projectSettings.configTitle")}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t("projectSettings.configDescription")}
        </p>
      </div>

      {closed ? (
        <Alert>
          <AlertDescription>
            {t("projectSettings.configClosedReadonly")}
          </AlertDescription>
        </Alert>
      ) : null}

      {configQuery.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : (configQuery.data ?? []).length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t("projectSettings.configEmpty")}
        </p>
      ) : (
        <div>
          {(configQuery.data ?? []).map((entry) => (
            <ProjectConfigRow
              canManage={canManage && !closed}
              entry={entry}
              key={entry.key}
              onDelete={() => delMut.mutate(entry.key)}
              onSave={async (value) => {
                await saveMut.mutateAsync({ key: entry.key, value })
              }}
              schema={schemaMap.get(entry.key)}
            />
          ))}
        </div>
      )}

      {canManage && !closed ? (
        <form
          className="grid gap-2 border-t pt-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (keyDuplicate || !trimmedKey) return
            void submitAdd()
          }}
        >
          <h3 className="text-sm font-medium">{t("projectSettings.configAdd")}</h3>
          <Input
            aria-label={t("projectSettings.configKey")}
            onChange={(e) => setNewKey(e.target.value)}
            placeholder={t("projectSettings.configKey")}
            value={newKey}
          />
          {newSchema?.description ? (
            <p className="text-xs text-muted-foreground">
              {newSchema.description}
            </p>
          ) : null}
          {newEnumValues.length > 0 ? (
            <Select onValueChange={setNewValue} value={newValue}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t("projectSettings.configValue")} />
              </SelectTrigger>
              <SelectContent>
                {newEnumValues.map((v) => (
                  <SelectItem key={v} value={v}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              aria-label={t("projectSettings.configValue")}
              onChange={(e) => setNewValue(e.target.value)}
              placeholder={t("projectSettings.configValue")}
              type={newIsSecret ? "password" : "text"}
              value={newValue}
            />
          )}
          {keyDuplicate ? (
            <p className="text-xs text-destructive">
              {t("projectSettings.configKeyExists")}
            </p>
          ) : null}
          {!newSchema && trimmedKey ? (
            <p className="text-xs text-muted-foreground">
              {t("projectSettings.configNoSchema")}
            </p>
          ) : null}
          {addError ? (
            <Alert variant="destructive">
              <AlertDescription>{addError}</AlertDescription>
            </Alert>
          ) : null}
          <Button
            disabled={keyDuplicate || !trimmedKey || adding}
            size="sm"
            type="submit"
          >
            {t("common.save")}
          </Button>
        </form>
      ) : null}
    </section>
  )
}
