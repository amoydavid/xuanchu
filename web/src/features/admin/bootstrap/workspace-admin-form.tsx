import type React from "react"
import { useTranslation } from "react-i18next"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export type WorkspaceAdminFormValue = {
  email: string
  name: string
  role: "owner" | "admin"
}

type WorkspaceAdminFormProps = {
  onChange: (value: WorkspaceAdminFormValue) => void
  value: WorkspaceAdminFormValue
}

export function WorkspaceAdminForm({
  onChange,
  value,
}: WorkspaceAdminFormProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-4 md:grid-cols-3">
      <Field label={t("admin.form.adminName")}>
        <Input
          aria-label={t("admin.form.adminName")}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
          placeholder="alice"
          value={value.name}
        />
      </Field>
      <Field label={t("admin.form.adminEmail")}>
        <Input
          aria-label={t("admin.form.adminEmail")}
          onChange={(event) =>
            onChange({ ...value, email: event.target.value })
          }
          placeholder="alice@example.com"
          type="email"
          value={value.email}
        />
      </Field>
      <Field label={t("admin.form.adminRole")}>
        <Select
          onValueChange={(role: WorkspaceAdminFormValue["role"]) =>
            onChange({ ...value, role })
          }
          value={value.role}
        >
          <SelectTrigger
            aria-label={t("admin.form.adminRole")}
            className="w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="owner">{t("admin.role.owner")}</SelectItem>
            <SelectItem value="admin">{t("admin.role.admin")}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </div>
  )
}

function Field({
  children,
  label,
}: {
  children: React.ReactNode
  label: string
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
    </div>
  )
}
