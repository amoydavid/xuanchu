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

export type WorkspaceFormValue = {
  description: string
  name: string
  slug: string
  visibility: "private" | "team" | "public"
}

type WorkspaceFormProps = {
  onChange: (value: WorkspaceFormValue) => void
  value: WorkspaceFormValue
}

export function WorkspaceForm({ onChange, value }: WorkspaceFormProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Field label={t("admin.form.workspaceSlug")}>
        <Input
          aria-label={t("admin.form.workspaceSlug")}
          onChange={(event) => onChange({ ...value, slug: event.target.value })}
          placeholder="dajee"
          value={value.slug}
        />
      </Field>
      <Field label={t("admin.form.workspaceName")}>
        <Input
          aria-label={t("admin.form.workspaceName")}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
          placeholder="Dajee"
          value={value.name}
        />
      </Field>
      <Field label={t("admin.form.workspaceDescription")}>
        <Input
          aria-label={t("admin.form.workspaceDescription")}
          onChange={(event) =>
            onChange({ ...value, description: event.target.value })
          }
          placeholder={t("admin.form.workspaceDescriptionPlaceholder")}
          value={value.description}
        />
      </Field>
      <Field label={t("admin.form.workspaceVisibility")}>
        <Select
          onValueChange={(visibility: WorkspaceFormValue["visibility"]) =>
            onChange({ ...value, visibility })
          }
          value={value.visibility}
        >
          <SelectTrigger
            aria-label={t("admin.form.workspaceVisibility")}
            className="w-full"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="private">
              {t("admin.visibility.private")}
            </SelectItem>
            <SelectItem value="team">{t("admin.visibility.team")}</SelectItem>
            <SelectItem value="public">
              {t("admin.visibility.public")}
            </SelectItem>
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
