import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  TenantAccessTokenForm,
  tenantValuesToModifyInput,
} from "@/features/workspace/tokens/tenant-access-token-form"
import { tokenErrorMessage } from "@/features/workspace/tokens/token-api"

import type { AdminTenantAccessTokenRow } from "./admin-token-api"
import { useAdminModifyTenantAccessTokenMutation } from "./use-admin-token-mutations"

type AdminTenantAccessTokenEditDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: AdminTenantAccessTokenRow
}

export function AdminTenantAccessTokenEditDialog({
  open,
  onOpenChange,
  token,
}: AdminTenantAccessTokenEditDialogProps) {
  const { t } = useTranslation()
  const [error, setError] = useState<string | null>(null)
  const mutation = useAdminModifyTenantAccessTokenMutation()

  const close = () => {
    setError(null)
    onOpenChange(false)
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("token.edit")}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="rounded-none border bg-muted/30 p-3 text-sm text-muted-foreground">
            <div>
              {t("token.field.workspaces")}: {token.workspace_id}
            </div>
            <div>
              {t("token.field.prefix")}: {token.prefix}
            </div>
          </div>
          <TenantAccessTokenForm
            initial={token}
            mode="edit"
            onSubmit={(values) => {
              setError(null)
              mutation.mutate(
                {
                  ref: token.id,
                  input: tenantValuesToModifyInput(values, token),
                },
                {
                  onSuccess: close,
                  onError: (err) => setError(tokenErrorMessage(err, t)),
                }
              )
            }}
            submitting={mutation.isPending}
          />
          <p className="text-xs text-muted-foreground">
            {t("admin.token.modifyAuditHint")}
          </p>
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <Button onClick={close} type="button" variant="outline">
            {t("token.cancel")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
