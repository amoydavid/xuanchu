import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { ScopeEditor } from "./scope-editor"
import {
  expiresSecondsToPreset,
  presetToExpiresSeconds,
  sameExpiresSelection,
  sameSet,
  type ExpiresPreset,
  type TenantAccessTokenFormValues,
  type TenantAccessTokenRow,
} from "./token-api"

type TenantAccessTokenFormProps = {
  mode: "create" | "edit"
  initial?: TenantAccessTokenRow
  onSubmit: (values: TenantAccessTokenFormValues) => void
  submitting?: boolean
}

function defaultValues(
  mode: "create" | "edit",
  initial?: TenantAccessTokenRow
): TenantAccessTokenFormValues {
  if (mode === "edit" && initial) {
    const { preset, customIso } = expiresSecondsToPreset(initial.expires_at)
    return {
      name: initial.name,
      scopes: [...(initial.scopes ?? [])],
      projects: [...(initial.project_ids ?? [])],
      expiresPreset: preset,
      expiresAt: customIso,
    }
  }
  return {
    name: "",
    scopes: [],
    projects: [],
    expiresPreset: "never",
    expiresAt: "",
  }
}

export function TenantAccessTokenForm({
  mode,
  initial,
  onSubmit,
  submitting = false,
}: TenantAccessTokenFormProps) {
  const { t } = useTranslation()
  const [values, setValues] = useState<TenantAccessTokenFormValues>(() =>
    defaultValues(mode, initial)
  )
  const [formError, setFormError] = useState<string | null>(null)

  const update = <K extends keyof TenantAccessTokenFormValues>(
    key: K,
    next: TenantAccessTokenFormValues[K]
  ) => setValues((prev) => ({ ...prev, [key]: next }))

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault()
    setFormError(null)
    if (!values.name.trim()) {
      setFormError(t("token.nameRequired"))
      return
    }
    if (values.scopes.length === 0) {
      setFormError(t("token.scopeRequired"))
      return
    }
    onSubmit(values)
  }

  return (
    <form className="space-y-4" onSubmit={handleSubmit}>
      <Field label={t("token.field.name")}>
        <Input
          aria-label={t("token.field.name")}
          onChange={(e) => update("name", e.target.value)}
          placeholder="tenant-ci"
          value={values.name}
        />
      </Field>

      <Field label={t("token.field.scopes")}>
        <ScopeEditor
          onChange={(scopes) => update("scopes", scopes)}
          tenantAccessToken
          value={values.scopes}
        />
      </Field>

      <Field label={t("token.field.expires")}>
        <Select
          onValueChange={(v) => update("expiresPreset", v as ExpiresPreset)}
          value={values.expiresPreset}
        >
          <SelectTrigger aria-label={t("token.field.expires")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="never">
              {t("token.expiresPreset.never")}
            </SelectItem>
            <SelectItem value="7d">{t("token.expiresPreset.7d")}</SelectItem>
            <SelectItem value="30d">{t("token.expiresPreset.30d")}</SelectItem>
            <SelectItem value="90d">{t("token.expiresPreset.90d")}</SelectItem>
            <SelectItem value="custom">
              {t("token.expiresPreset.custom")}
            </SelectItem>
          </SelectContent>
        </Select>
        {values.expiresPreset === "custom" ? (
          <Input
            aria-label={t("token.field.expires")}
            className="mt-2"
            onChange={(e) => update("expiresAt", e.target.value)}
            type="datetime-local"
            value={values.expiresAt}
          />
        ) : null}
      </Field>

      <Field label={t("token.field.projects")}>
        <Input
          aria-label={t("token.field.projects")}
          onChange={(e) => {
            update(
              "projects",
              e.target.value
                .split(",")
                .map((s) => s.trim())
                .filter(Boolean)
            )
          }}
          placeholder="project-slug-a, project-slug-b"
          value={values.projects.join(", ")}
        />
      </Field>

      {formError ? (
        <p className="text-sm text-destructive">{formError}</p>
      ) : null}

      <div className="flex justify-end gap-2 pt-2">
        <Button disabled={submitting} size="sm" type="submit">
          {submitting
            ? "..."
            : mode === "create"
              ? t("token.tenant.create")
              : t("token.save")}
        </Button>
      </div>
    </form>
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

export function tenantValuesToCreateInput(
  values: TenantAccessTokenFormValues
) {
  return {
    name: values.name.trim(),
    scopes: values.scopes,
    projects: values.projects,
    expires_in_seconds: presetToExpiresSeconds(
      values.expiresPreset,
      values.expiresAt
    ),
  }
}

export function tenantValuesToModifyInput(
  values: TenantAccessTokenFormValues,
  initial?: TenantAccessTokenRow
) {
  const input: {
    name?: string
    scopes?: string[]
    projects?: string[]
    expires_in_seconds?: number | null
  } = {}
  if (!initial || values.name.trim() !== initial.name) {
    input.name = values.name.trim()
  }
  if (!initial || !sameSet(values.scopes, initial.scopes ?? [])) {
    input.scopes = values.scopes
  }
  if (!initial || !sameSet(values.projects, initial.project_ids ?? [])) {
    input.projects = values.projects
  }
  const newExpires = presetToExpiresSeconds(
    values.expiresPreset,
    values.expiresAt
  )
  if (
    !initial ||
    !sameExpiresSelection(initial.expires_at, values.expiresPreset, values.expiresAt)
  ) {
    input.expires_in_seconds = newExpires
  }
  return input
}
