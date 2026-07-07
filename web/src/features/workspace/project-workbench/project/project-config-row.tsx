import { useState } from "react"
import { EyeIcon, EyeOffIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

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
import type { ProjectConfigEntry } from "../api/project-api"
import type { ConfigSchemaDefinition } from "../api/config-schema-api"

type ProjectConfigRowProps = {
  entry: ProjectConfigEntry
  schema?: ConfigSchemaDefinition
  canManage: boolean
  onSave: (value: string) => Promise<void>
  onDelete: () => void
}

export function ProjectConfigRow({
  entry,
  schema,
  canManage,
  onSave,
  onDelete,
}: ProjectConfigRowProps) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(entry.value)
  const [revealed, setRevealed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isSecret = schema?.secret === true
  const enumValues = schema?.enum_values ?? []

  const startEdit = () => {
    setDraft(entry.value)
    setError(null)
    setEditing(true)
  }

  const cancel = () => {
    setEditing(false)
    setError(null)
  }

  const save = async () => {
    setSaving(true)
    setError(null)
    try {
      await onSave(draft)
      setEditing(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setSaving(false)
    }
  }

  if (editing) {
    return (
      <div className="space-y-2 border-b py-2">
        <div className="flex items-center justify-between gap-2">
          <code className="text-xs">{entry.key}</code>
          {schema ? null : (
            <span className="text-xs text-muted-foreground">
              {t("projectSettings.configNoSchema")}
            </span>
          )}
        </div>
        {enumValues.length > 0 ? (
          <Select onValueChange={setDraft} value={draft}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {enumValues.map((v) => (
                <SelectItem key={v} value={v}>
                  {v}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Input
            onChange={(e) => setDraft(e.target.value)}
            type={isSecret ? "password" : "text"}
            value={draft}
          />
        )}
        {schema?.description ? (
          <p className="text-xs text-muted-foreground">{schema.description}</p>
        ) : null}
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
        <div className="flex gap-2">
          <Button disabled={saving} onClick={save} size="sm" type="button">
            {t("projectSettings.configSave")}
          </Button>
          <Button onClick={cancel} size="sm" type="button" variant="outline">
            {t("projectSettings.configCancel")}
          </Button>
        </div>
      </div>
    )
  }

  const displayValue = isSecret && !revealed ? "••••••" : entry.value

  return (
    <div className="flex items-center justify-between gap-2 border-b py-1 text-sm last:border-b-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <code className="text-xs">{entry.key}</code>
          {isSecret ? (
            <span className="text-xs text-muted-foreground">
              {t("projectSettings.configSecret")}
            </span>
          ) : null}
          {schema ? null : (
            <span className="text-xs text-muted-foreground">
              {t("projectSettings.configNoSchema")}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          <span className="truncate text-muted-foreground">{displayValue}</span>
          {isSecret ? (
            <Button
              aria-label={
                revealed
                  ? t("projectSettings.configHide")
                  : t("projectSettings.configReveal")
              }
              onClick={() => setRevealed((v) => !v)}
              size="icon-sm"
              type="button"
              variant="ghost"
            >
              {revealed ? <EyeOffIcon /> : <EyeIcon />}
            </Button>
          ) : null}
        </div>
      </div>
      {canManage ? (
        <div className="flex shrink-0 gap-1">
          <Button onClick={startEdit} size="sm" type="button" variant="outline">
            {t("projectSettings.configEdit")}
          </Button>
          <Button
            onClick={() => {
              if (window.confirm(t("projectSettings.configDeleteConfirm"))) {
                onDelete()
              }
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            {t("common.delete")}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
