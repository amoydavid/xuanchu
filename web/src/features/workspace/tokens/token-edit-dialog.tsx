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
  TokenForm,
  tokenErrorMessage,
  valuesToModifyInput,
} from "./token-form"
import type { TokenRow } from "./token-api"
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
        <div className="flex justify-end gap-2 pt-2">
          <Button onClick={close} type="button" variant="outline">
            {t("token.cancel")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
