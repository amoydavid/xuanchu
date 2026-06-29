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
import {
  TenantAccessTokenForm,
  tenantValuesToCreateInput,
} from "./tenant-access-token-form"
import { tokenErrorMessage } from "./token-api"
import { useCreateTenantAccessTokenMutation } from "./use-token-mutations"

type TenantAccessTokenCreateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TenantAccessTokenCreateDialog({
  open,
  onOpenChange,
}: TenantAccessTokenCreateDialogProps) {
  const { t } = useTranslation()
  const [rawToken, setRawToken] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const mutation = useCreateTenantAccessTokenMutation()

  const close = () => {
    setRawToken(null)
    setError(null)
    onOpenChange(false)
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("token.tenant.create")}</DialogTitle>
        </DialogHeader>
        {rawToken ? (
          <TokenCreatedResult onDone={close} rawToken={rawToken} />
        ) : (
          <>
            <TenantAccessTokenForm
              mode="create"
              onSubmit={(values) => {
                setError(null)
                mutation.mutate(tenantValuesToCreateInput(values), {
                  onSuccess: (created) => setRawToken(created.token),
                  onError: (err) => setError(tokenErrorMessage(err, t)),
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
