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

import type { TenantAccessTokenRow } from "./token-api"
import { useRevokeTenantAccessTokenMutation } from "./use-token-mutations"

type TenantAccessTokenRevokeDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: TenantAccessTokenRow
}

export function TenantAccessTokenRevokeDialog({
  open,
  onOpenChange,
  token,
}: TenantAccessTokenRevokeDialogProps) {
  const { t } = useTranslation()
  const mutation = useRevokeTenantAccessTokenMutation()

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
            {t("token.field.type")}: {t("token.tenant.label")}
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
