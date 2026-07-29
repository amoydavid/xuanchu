import { useMemo } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AddConfigValueDialog,
  EffectiveConfigList,
} from "@/features/workspace/config/effective-config-editor"
import {
  deleteWorkspaceConfig,
  listWorkspaceEffectiveConfigForSlug,
  setWorkspaceConfig,
} from "@/features/workspace/config/workspace-config-api"
import { useMe } from "@/features/workspace/session/useMe"
import { canConfigManage } from "@/features/workspace/project-workbench/permissions/permissions"
import { navigateToDocument } from "@/lib/browser-navigation"

type WorkspaceConfigPageProps = {
  workspaceSlug: string
}

// WorkspaceConfigPage 编辑当前 workspace 的 config 值。
// 仅当 URL slug 等于当前 effective workspace 时可写；否则降级为只读提示（普通 token 不能跨 workspace 写）。
export function WorkspaceConfigPage({ workspaceSlug }: WorkspaceConfigPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const me = useMe()
  const effectiveSlug = me.data?.effective_workspace.slug ?? ""
  const isCurrent = workspaceSlug === effectiveSlug && workspaceSlug !== ""
  const canManage =
    isCurrent &&
    canConfigManage({
      role: me.data?.effective_role,
      scopes: me.data?.token.scopes,
    })

  const queryKey = useMemo(
    () => ["workspace", workspaceSlug, "config", "effective"] as const,
    [workspaceSlug]
  )

  const effectiveQuery = useQuery({
    queryKey,
    queryFn: () => listWorkspaceEffectiveConfigForSlug(workspaceSlug),
    enabled: isCurrent,
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey })

  const saveMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setWorkspaceConfig(workspaceSlug, key, value),
    onSuccess: invalidate,
  })

  const deleteMut = useMutation({
    mutationFn: (key: string) => deleteWorkspaceConfig(workspaceSlug, key),
    onSuccess: invalidate,
  })

  if (!isCurrent) {
    return (
      <Alert>
        <AlertDescription>
          {t("workspaceConfig.readonlyNotCurrent")}（
          <button
            className="font-medium text-primary hover:underline"
            onClick={() => navigateToDocument("/workspaces")}
            type="button"
          >
            {t("page.workspaces")}
          </button>
          ）
        </AlertDescription>
      </Alert>
    )
  }

  const rows = effectiveQuery.data ?? []

  return (
    <section className="space-y-3 rounded-lg border bg-card p-4">
      <div>
        <h2 className="text-sm font-medium">{t("workspaceConfig.title")}</h2>
        <p className="text-xs text-muted-foreground">
          {t("workspaceConfig.description")}
        </p>
      </div>

      {effectiveQuery.isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : null}

      {effectiveQuery.isError ? (
        <p className="text-sm text-destructive">{t("common.error")}</p>
      ) : null}

      <EffectiveConfigList
        rows={rows}
        scope="workspace"
        canManage={canManage}
        loading={effectiveQuery.isLoading}
        onSave={(key, value) => saveMut.mutateAsync({ key, value })}
        onRestore={(key) => deleteMut.mutateAsync(key)}
      />

      <AddConfigValueDialog
        rows={rows}
        scope="workspace"
        canManage={canManage}
        onSave={async (key, value) => {
          await saveMut.mutateAsync({ key, value })
        }}
      />
    </section>
  )
}
