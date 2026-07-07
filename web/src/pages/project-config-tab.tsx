import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import { formatConfigDisplayValue } from "@/features/workspace/config/config-display"
import { ConfigValueControl } from "@/features/workspace/config/config-value-control"
import {
  type ConfigEffectiveValue,
  listProjectEffectiveConfig,
} from "@/features/workspace/config/config-definition-api"
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

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey })

  const saveMut = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setProjectConfig(workspaceSlug, projectSlug, key, value),
    onSuccess: invalidate,
  })

  const deleteMut = useMutation({
    mutationFn: (key: string) =>
      deleteProjectConfig(workspaceSlug, projectSlug, key),
    onSuccess: invalidate,
  })

  const rows = effectiveQuery.data ?? []
  const writable = canManage && !closed

  return (
    <section className="space-y-3 border bg-card p-4">
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

      {!effectiveQuery.isLoading && rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t("configDefinitions.noValues")}
        </p>
      ) : null}

      <div className="divide-y">
        {rows.map((row) => (
          <EffectiveRow
            canEdit={writable && canEditProjectScope(row)}
            key={row.key}
            row={row}
            onSave={(value) => saveMut.mutateAsync({ key: row.key, value })}
            onRestore={() => deleteMut.mutateAsync(row.key)}
          />
        ))}
      </div>

      {writable ? (
        <AddValueDialog
          rows={rows}
          onSave={async (key, value) => {
            await saveMut.mutateAsync({ key, value })
          }}
        />
      ) : null}
    </section>
  )
}

// project 显式值可写条件：定义允许 project scope（workspace-only 的 key 不能被 project 覆盖）。
function canEditProjectScope(row: ConfigEffectiveValue): boolean {
  return (row.definition.allowed_scopes as string[]).includes("project")
}

function sourceLabel(source: string, t: (k: string) => string): string {
  switch (source) {
    case "project":
      return t("configDefinitions.sourceProject")
    case "workspace":
      return t("configDefinitions.sourceWorkspace")
    case "default":
      return t("configDefinitions.sourceDefault")
    default:
      return t("configDefinitions.sourceMissing")
  }
}

function EffectiveRow({
  row,
  canEdit,
  onSave,
  onRestore,
}: {
  row: ConfigEffectiveValue
  canEdit: boolean
  onSave: (value: string) => Promise<void>
  onRestore: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(row.value ?? "")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const isSecret = row.definition.secret
  const [revealed, setRevealed] = useState(false)
  const isProjectSource = row.source === "project"
  const allowsProject = canEdit

  const startEdit = () => {
    setDraft(row.value ?? "")
    setError(null)
    setEditing(true)
  }

  const save = async () => {
    setBusy(true)
    setError(null)
    try {
      await onSave(draft)
      setEditing(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setBusy(false)
    }
  }

  const restore = async () => {
    setBusy(true)
    setError(null)
    try {
      await onRestore()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setBusy(false)
    }
  }

  const displayValue =
    row.value === null
      ? "—"
      : isSecret && !revealed
        ? "••••••"
        : formatConfigDisplayValue(row.definition.value_type, row.value)

  return (
    <div className="py-2">
      <div className="flex flex-wrap items-center gap-2">
        <code className="text-xs">{row.key}</code>
        {row.definition.label ? (
          <span className="text-xs text-muted-foreground">
            {row.definition.label}
          </span>
        ) : null}
        <Badge variant="outline">{sourceLabel(row.source, t)}</Badge>
        {row.missing_required ? (
          <Badge variant="destructive">
            {t("configDefinitions.statusMissingRequired")}
          </Badge>
        ) : null}
        {!allowsProject ? (
          <Badge variant="secondary">
            {t("configDefinitions.statusReadonly")}
          </Badge>
        ) : null}
        {isProjectSource ? (
          <Badge variant="secondary">
            {t("configDefinitions.statusOverridden")}
          </Badge>
        ) : row.source === "workspace" || row.source === "default" ? (
          <Badge variant="secondary">
            {t("configDefinitions.statusInherited")}
          </Badge>
        ) : null}
      </div>

      {editing ? (
        <div className="mt-2 space-y-2">
          <ConfigValueControl
            definition={row.definition}
            onChange={setDraft}
            value={draft}
          />
          {error ? <p className="text-xs text-destructive">{error}</p> : null}
          <div className="flex gap-2">
            <Button disabled={busy} onClick={save} size="sm" type="button">
              {t("configDefinitions.save")}
            </Button>
            <Button
              onClick={() => setEditing(false)}
              size="sm"
              type="button"
              variant="outline"
            >
              {t("configDefinitions.cancel")}
            </Button>
          </div>
        </div>
      ) : (
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <span className="truncate text-sm text-muted-foreground">
            {displayValue}
          </span>
          {isSecret && row.value !== null ? (
            <Button
              aria-label={
                revealed
                  ? t("configDefinitions.hideSecret")
                  : t("configDefinitions.revealSecret")
              }
              onClick={() => setRevealed((v) => !v)}
              size="icon-sm"
              type="button"
              variant="ghost"
            >
              {revealed ? "隐藏" : "显示"}
            </Button>
          ) : null}
          {canEdit ? (
            <div className="flex gap-1">
              <Button
                onClick={startEdit}
                size="sm"
                type="button"
                variant="outline"
              >
                {t("configDefinitions.edit")}
              </Button>
              {isProjectSource ? (
                <Button
                  disabled={busy}
                  onClick={restore}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  {t("configDefinitions.restoreInherited")}
                </Button>
              ) : null}
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}

function AddValueDialog({
  rows,
  onSave,
}: {
  rows: ConfigEffectiveValue[]
  onSave: (key: string, value: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [value, setValue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // 可写：定义允许 project scope 且当前无 project_value（尚未覆盖）。
  const writable = rows.filter(
    (r) =>
      (r.definition.allowed_scopes as string[]).includes("project") &&
      r.project_value == null
  )
  const workspaceOnly = rows.filter(
    (r) => !(r.definition.allowed_scopes as string[]).includes("project")
  )
  const selectedDef = selectedKey
    ? rows.find((r) => r.key === selectedKey)?.definition
    : undefined

  const submit = async () => {
    if (!selectedKey) return
    setBusy(true)
    setError(null)
    try {
      await onSave(selectedKey, value)
      setOpen(false)
      setSelectedKey(null)
      setValue("")
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Button onClick={() => setOpen(true)} size="sm" type="button">
        {t("configDefinitions.addValue")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("configDefinitions.addValueTitle")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <Select onValueChange={(k) => setSelectedKey(k)} value={selectedKey ?? ""}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t("configDefinitions.addValueKey")} />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectLabel>
                    {t("configDefinitions.addValueProjectWritable")}
                  </SelectLabel>
                  {writable.map((r) => (
                    <SelectItem key={r.key} value={r.key}>
                      {r.key} ({r.definition.value_type})
                    </SelectItem>
                  ))}
                  {workspaceOnly.length > 0 ? (
                    <>
                      <SelectLabel>
                        {t("configDefinitions.addValueWorkspaceOnly")}
                      </SelectLabel>
                      {workspaceOnly.map((r) => (
                        <SelectItem disabled key={r.key} value={r.key}>
                          {r.key} ({r.definition.value_type})
                        </SelectItem>
                      ))}
                    </>
                  ) : null}
                </SelectGroup>
              </SelectContent>
            </Select>
            {selectedDef ? (
              <ConfigValueControl
                definition={selectedDef}
                onChange={setValue}
                value={value}
              />
            ) : null}
            {error ? <p className="text-xs text-destructive">{error}</p> : null}
          </div>
          <DialogFooter>
            <Button
              disabled={busy || !selectedKey}
              onClick={submit}
              size="sm"
              type="button"
            >
              {t("configDefinitions.save")}
            </Button>
            <Button
              onClick={() => setOpen(false)}
              size="sm"
              type="button"
              variant="outline"
            >
              {t("configDefinitions.cancel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
