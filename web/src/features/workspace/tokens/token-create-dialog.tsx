import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

import { TokenCreatedResult } from "./token-created-result"
import { TokenForm, tokenErrorMessage, valuesToCreateInput } from "./token-form"
import { useCreateTokenMutation } from "./use-token-mutations"

type TokenCreateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  canImpersonate?: boolean
}

export function TokenCreateDialog({
  open,
  onOpenChange,
  canImpersonate = false,
}: TokenCreateDialogProps) {
  const { t } = useTranslation()
  const [rawToken, setRawToken] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const mutation = useCreateTokenMutation()

  const close = () => {
    setRawToken(null)
    setError(null)
    onOpenChange(false)
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("token.create")}</DialogTitle>
        </DialogHeader>
        {rawToken ? (
          <TokenCreatedResult onDone={close} rawToken={rawToken} />
        ) : (
          <>
            <TokenForm
              canImpersonate={canImpersonate}
              mode="create"
              onSubmit={(values) => {
                setError(null)
                mutation.mutate(valuesToCreateInput(values), {
                  onSuccess: (created) => {
                    setRawToken(created.token)
                  },
                  onError: (err) => {
                    setError(tokenErrorMessage(err, t))
                  },
                })
              }}
              submitting={mutation.isPending}
            />
            {error ? (
              <p className="text-sm text-destructive">{error}</p>
            ) : null}
            <div className="flex justify-end pt-2">
              <Button onClick={close} type="button" variant="outline">
                {t("token.cancel")}
              </Button>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
