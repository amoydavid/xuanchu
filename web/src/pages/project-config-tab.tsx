import { useMemo } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  EffectiveConfigList,
  AddConfigValueDialog,
} from "@/features/workspace/config/effective-config-editor"
import { listProjectEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import {
  deleteProjectConfig,
  setProjectConfig,
} from "@/features/workspace/project-workbench/api/project-api"

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
  const queryKey = useMemo(
    () => ["project", workspaceSlug, projectSlug, "config", "effective"] as const,
    [workspaceSlug, projectSlug]
  )

  const effectiveQuery = useQuery({
    queryKey,
    queryFn: () => listProjectEffectiveConfig(projectSlug),
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey })

  const saveMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setProjectConfig(workspaceSlug, projectSlug, key, value),
    onSuccess: invalidate,
  })

  const deleteMut = useMutation({
    mutationFn: (key: string) => deleteProjectConfig(workspaceSlug, projectSlug, key),
    onSuccess: invalidate,
  })

  const rows = effectiveQuery.data ?? []
  const writable = canManage && !closed

  return (
    <section className="rounded-lg space-y-3 border bg-card p-4">
      <div>
        <h2 className="text-sm font-medium">
          {t("projectSettings.configTitle")}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t("configDefinitions.effectiveDescription")}
        </p>
      </div>

      {closed ? (
        <Alert>
          <AlertDescription>
            {t("projectSettings.configClosedReadonly")}
          </AlertDescription>
        </Alert>
      ) : null}

      {effectiveQuery.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : null}

      {effectiveQuery.isError ? (
        <p className="text-sm text-destructive">{t("common.error")}</p>
      ) : null}

      <EffectiveConfigList
        rows={rows}
        scope="project"
        canManage={writable}
        onSave={(key, value) => saveMut.mutateAsync({ key, value })}
        onRestore={(key) => deleteMut.mutateAsync(key)}
      />

      <AddConfigValueDialog
        rows={rows}
        scope="project"
        canManage={writable}
        onSave={async (key, value) => {
          await saveMut.mutateAsync({ key, value })
        }}
      />
    </section>
  )
}
