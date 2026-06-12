import type React from "react"
import { useTranslation } from "react-i18next"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export type AgentTokenFormValue = {
  expiresIn: string
  name: string
  scopes: string
}

type AgentTokenFormProps = {
  onChange: (value: AgentTokenFormValue) => void
  value: AgentTokenFormValue
}

export function AgentTokenForm({ onChange, value }: AgentTokenFormProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-4 md:grid-cols-3">
      <Field label={t("admin.form.tokenName")}>
        <Input
          aria-label={t("admin.form.tokenName")}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
          placeholder="alice-admin-agent"
          value={value.name}
        />
      </Field>
      <Field label={t("admin.form.tokenScopes")}>
        <Input
          aria-label={t("admin.form.tokenScopes")}
          onChange={(event) =>
            onChange({ ...value, scopes: event.target.value })
          }
          placeholder="*"
          value={value.scopes}
        />
      </Field>
      <Field label={t("admin.form.tokenExpiresIn")}>
        <Input
          aria-label={t("admin.form.tokenExpiresIn")}
          onChange={(event) =>
            onChange({ ...value, expiresIn: event.target.value })
          }
          placeholder="720h"
          value={value.expiresIn}
        />
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
