import { useTranslation } from "react-i18next"
import { useState } from "react"

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

import {
  TenantAccessTokenForm,
  tenantValuesToModifyInput,
} from "./tenant-access-token-form"
import { tokenErrorMessage, type TenantAccessTokenRow } from "./token-api"
import { useModifyTenantAccessTokenMutation } from "./use-token-mutations"

type TenantAccessTokenEditDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: TenantAccessTokenRow
}

export function TenantAccessTokenEditDialog({
  open,
  onOpenChange,
  token,
}: TenantAccessTokenEditDialogProps) {
  const { t } = useTranslation()
  const [error, setError] = useState<string | null>(null)
  const mutation = useModifyTenantAccessTokenMutation()

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
        <TenantAccessTokenForm
          initial={token}
          mode="edit"
          onCancel={close}
          onSubmit={(values) => {
            setError(null)
            mutation.mutate(
              { ref: token.id, input: tenantValuesToModifyInput(values, token) },
              {
                onSuccess: close,
                onError: (err) => setError(tokenErrorMessage(err, t)),
              }
            )
          }}
          submitting={mutation.isPending}
        />
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
      </DialogContent>
    </Dialog>
  )
}
