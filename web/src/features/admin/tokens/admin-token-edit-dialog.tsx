import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ScopeEditor } from "@/features/workspace/tokens/scope-editor"
import {
  expiresSecondsToPreset,
  presetToExpiresSeconds,
  sameSet,
  tokenErrorMessage,
  type ExpiresPreset,
} from "@/features/workspace/tokens/token-api"

import type { AdminTokenRow, AdminTokenModifyInput } from "./admin-token-api"
import { useAdminModifyTokenMutation } from "./use-admin-token-mutations"

type AdminTokenEditDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: AdminTokenRow
}

export function AdminTokenEditDialog({
  open,
  onOpenChange,
  token,
}: AdminTokenEditDialogProps) {
  const { t } = useTranslation()
  const mutation = useAdminModifyTokenMutation()
  const [error, setError] = useState<string | null>(null)

  // 表单状态
  const { preset: initPreset, customIso: initCustom } = expiresSecondsToPreset(
    token.expires_at
  )
  const [name, setName] = useState(token.name)
  const [scopes, setScopes] = useState<string[]>([...token.scopes])
  const [expiresPreset, setExpiresPreset] = useState<ExpiresPreset>(initPreset)
  const [expiresAt, setExpiresAt] = useState(initCustom)

  const close = () => {
    setError(null)
    onOpenChange(false)
  }

  const submit = () => {
    setError(null)
    const input: AdminTokenModifyInput = {}
    if (name.trim() !== token.name) {
      input.name = name.trim()
    }
    if (!sameSet(scopes, token.scopes)) {
      input.scopes = scopes
    }
    const newExpires = presetToExpiresSeconds(expiresPreset, expiresAt)
    const oldExpires = token.expires_at ?? null
    if ((newExpires ?? null) !== (oldExpires ?? null)) {
      input.expires_in_seconds = newExpires
    }
    mutation.mutate(
      { ref: token.id, input },
      {
        onSuccess: close,
        onError: (err) => setError(tokenErrorMessage(err, t)),
      }
    )
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("token.edit")}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {/* 只读信息 */}
          <ReadonlyField label={t("token.field.type")} value={token.type} />
          <ReadonlyField
            label={t("common.actor")}
            value={`${token.user.name}${token.user.email ? ` (${token.user.email})` : ""}`}
          />
          <ReadonlyField
            label={t("token.field.workspaces")}
            value={
              token.workspace_ids.length > 0
                ? token.workspace_ids.join(", ")
                : t("admin.token.global")
            }
          />

          {/* 可编辑字段 */}
          <div className="space-y-2">
            <Label>{t("token.field.name")}</Label>
            <Input
              onChange={(e) => setName(e.target.value)}
              value={name}
            />
          </div>

          <div className="space-y-2">
            <Label>{t("token.field.scopes")}</Label>
            <ScopeEditor
              canImpersonate
              onChange={setScopes}
              value={scopes}
            />
          </div>

          <div className="space-y-2">
            <Label>{t("token.field.expires")}</Label>
            <Select
              onValueChange={(v) => setExpiresPreset(v as ExpiresPreset)}
              value={expiresPreset}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="never">{t("token.expiresPreset.never")}</SelectItem>
                <SelectItem value="7d">{t("token.expiresPreset.7d")}</SelectItem>
                <SelectItem value="30d">{t("token.expiresPreset.30d")}</SelectItem>
                <SelectItem value="90d">{t("token.expiresPreset.90d")}</SelectItem>
                <SelectItem value="custom">{t("token.expiresPreset.custom")}</SelectItem>
              </SelectContent>
            </Select>
            {expiresPreset === "custom" ? (
              <Input
                className="mt-2"
                onChange={(e) => setExpiresAt(e.target.value)}
                type="datetime-local"
                value={expiresAt}
              />
            ) : null}
          </div>

          <p className="text-xs text-muted-foreground">
            {t("admin.token.modifyAuditHint")}
          </p>

          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <Button onClick={close} type="button" variant="outline">
            {t("token.cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={mutation.isPending}
            type="button"
          >
            {mutation.isPending ? "..." : t("token.save")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ReadonlyField({ label, value }: { label: string; value: string }) {
  return (
    <div className="space-y-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <div className="rounded-none border bg-muted/30 px-2 py-1.5 text-sm">
        {value}
      </div>
    </div>
  )
}
