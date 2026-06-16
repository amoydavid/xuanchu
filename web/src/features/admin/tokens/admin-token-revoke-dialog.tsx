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

import type { AdminTokenRow } from "./admin-token-api"
import { useAdminRevokeTokenMutation } from "./use-admin-token-mutations"

type AdminTokenRevokeDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: AdminTokenRow
}

export function AdminTokenRevokeDialog({
  open,
  onOpenChange,
  token,
}: AdminTokenRevokeDialogProps) {
  const { t } = useTranslation()
  const mutation = useAdminRevokeTokenMutation()

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
            <br />
            {t("common.actor")}: {token.user.name}
            <br />
            {t("admin.token.revokeAuditHint")}
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
