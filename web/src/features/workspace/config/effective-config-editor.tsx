import { useState } from "react"
import { useTranslation } from "react-i18next"

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
  type ConfigAllowedScope,
  type ConfigEffectiveValue,
} from "@/features/workspace/config/config-definition-api"

// sourceLabel 把后端来源字符串翻译成展示文案。
export function sourceLabel(source: string, t: (k: string) => string): string {
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

// EffectiveConfigList 渲染 effective 配置行列表。scope 决定可写判定：
// project 侧传 "project"，workspace 侧传 "workspace"；definition.allowed_scopes 不含该 scope 的行只读。
export function EffectiveConfigList({
  rows,
  scope,
  canManage,
  loading,
  onSave,
  onRestore,
}: {
  rows: ConfigEffectiveValue[]
  scope: ConfigAllowedScope
  canManage: boolean
  // loading 为 true 时不渲染「无值」空态，避免加载期与父组件 loading 提示同时闪烁。
  loading?: boolean
  onSave: (key: string, value: string) => Promise<void>
  onRestore: (key: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const canEditRow = (row: ConfigEffectiveValue): boolean =>
    canManage && (row.definition.allowed_scopes as string[]).includes(scope)

  return (
    <div className="divide-y">
      {rows.map((row) => (
        <EffectiveRow
          canEdit={canEditRow(row)}
          isOverriddenSource={row.source === scope}
          key={row.key}
          row={row}
          onSave={(value) => onSave(row.key, value)}
          onRestore={() => onRestore(row.key)}
        />
      ))}
      {!loading && !rows.length ? (
        <p className="py-2 text-sm text-muted-foreground">
          {t("configDefinitions.noValues")}
        </p>
      ) : null}
    </div>
  )
}

function EffectiveRow({
  row,
  canEdit,
  isOverriddenSource,
  onSave,
  onRestore,
}: {
  row: ConfigEffectiveValue
  canEdit: boolean
  isOverriddenSource: boolean
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
        {!canEdit ? (
          <Badge variant="secondary">
            {t("configDefinitions.statusReadonly")}
          </Badge>
        ) : null}
        {isOverriddenSource ? (
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
              {isOverriddenSource ? (
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

// AddConfigValueDialog 新增配置值。只列出 allowed_scopes 含当前 scope、且尚未在该 scope 被覆盖的 key。
export function AddConfigValueDialog({
  rows,
  scope,
  canManage,
  onSave,
}: {
  rows: ConfigEffectiveValue[]
  scope: ConfigAllowedScope
  canManage: boolean
  onSave: (key: string, value: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [value, setValue] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (!canManage) return null

  // 可写：定义允许当前 scope 且当前 scope 尚未覆盖（用 source 判断：source===scope 表示已被本 scope 覆盖）。
  const scopeValueKey = `${scope}_value` as "project_value" | "workspace_value"
  const writable = rows.filter(
    (r) =>
      (r.definition.allowed_scopes as string[]).includes(scope) &&
      r[scopeValueKey] == null
  )
  const scopeOnly = rows.filter(
    (r) => !(r.definition.allowed_scopes as string[]).includes(scope)
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
                  {scopeOnly.length > 0 ? (
                    <>
                      <SelectLabel>
                        {t("configDefinitions.addValueWorkspaceOnly")}
                      </SelectLabel>
                      {scopeOnly.map((r) => (
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
