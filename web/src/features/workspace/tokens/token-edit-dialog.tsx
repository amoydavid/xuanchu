import { useTranslation } from "react-i18next"
import { useState } from "react"

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

import { TokenForm, valuesToModifyInput } from "./token-form"
import { tokenErrorMessage, type TokenRow } from "./token-api"
import { useModifyTokenMutation } from "./use-token-mutations"

type TokenEditDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: TokenRow
  canImpersonate?: boolean
}

export function TokenEditDialog({
  open,
  onOpenChange,
  token,
  canImpersonate = false,
}: TokenEditDialogProps) {
  const { t } = useTranslation()
  const [error, setError] = useState<string | null>(null)
  const mutation = useModifyTokenMutation()

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
        <TokenForm
          canImpersonate={canImpersonate}
          initial={token}
          mode="edit"
          onCancel={close}
          onSubmit={(values) => {
            setError(null)
            const input = valuesToModifyInput(values, token)
            mutation.mutate(
              { ref: token.id, input },
              {
                onSuccess: close,
                onError: (err) => {
                  setError(tokenErrorMessage(err, t))
                },
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
