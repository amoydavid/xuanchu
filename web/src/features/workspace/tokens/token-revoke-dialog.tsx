import { useTranslation } from "react-i18next"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"

import type { TokenRow } from "./token-api"
import { useRevokeTokenMutation } from "./use-token-mutations"

type TokenRevokeDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: TokenRow
}

export function TokenRevokeDialog({
  open,
  onOpenChange,
  token,
}: TokenRevokeDialogProps) {
  const { t } = useTranslation()
  const mutation = useRevokeTokenMutation()

  return (
    <AlertDialog onOpenChange={onOpenChange} open={open}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("token.revoke")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("token.revokeWarning")}
            <br />
            {t("token.field.name")}: {token.name}
            <br />
            {t("token.field.prefix")}: {token.prefix}
            <br />
            {t("token.field.type")}: {token.type}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("token.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              mutation.mutate(token.id, {
                onSuccess: () => onOpenChange(false),
              })
            }}
          >
            {t("token.revoke")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
