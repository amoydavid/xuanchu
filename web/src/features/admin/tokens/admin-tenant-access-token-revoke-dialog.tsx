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

import type { AdminTenantAccessTokenRow } from "./admin-token-api"
import { useAdminRevokeTenantAccessTokenMutation } from "./use-admin-token-mutations"

type AdminTenantAccessTokenRevokeDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  token: AdminTenantAccessTokenRow
}

export function AdminTenantAccessTokenRevokeDialog({
  open,
  onOpenChange,
  token,
}: AdminTenantAccessTokenRevokeDialogProps) {
  const { t } = useTranslation()
  const mutation = useAdminRevokeTenantAccessTokenMutation()

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
            <br />
            {t("token.field.workspaces")}: {token.workspace_id}
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
