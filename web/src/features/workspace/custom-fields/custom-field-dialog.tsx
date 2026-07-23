import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type {
  CustomFieldType,
  WorkspaceCustomField,
  WorkspaceCustomFieldInput,
} from "./custom-field-api"

export function CustomFieldDialog({
  field,
  onOpenChange,
  onSave,
  open,
  pending,
  error,
}: {
  field?: WorkspaceCustomField
  onOpenChange: (open: boolean) => void
  onSave: (name: string, input: WorkspaceCustomFieldInput) => Promise<void>
  open: boolean
  pending: boolean
  error?: string | null
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(field?.name ?? "")
  const [type, setType] = useState<CustomFieldType>(field?.type ?? "string")
  const [label, setLabel] = useState(field?.label ?? "")
  const [values, setValues] = useState((field?.values ?? []).join(", "))
  const [defaultValue, setDefaultValue] = useState(field?.default ?? "")

  const runtimeOverride = field?.source === "runtime"
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const normalizedName = name.trim()
    if (!normalizedName) return
    await onSave(normalizedName, {
      type,
      label: label.trim(),
      values: values
        .split(",")
        .map((value) => value.trim())
        .filter(Boolean),
      default: defaultValue.trim(),
    })
  }

  return (
    <Dialog open={open} onOpenChange={pending ? undefined : onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form className="grid gap-4" onSubmit={(event) => void submit(event)}>
          <DialogHeader>
            <DialogTitle>
              {field
                ? t("customFields.editTitle")
                : t("customFields.createTitle")}
            </DialogTitle>
            <DialogDescription>
              {runtimeOverride
                ? t("customFields.runtimeOverrideDescription")
                : t("customFields.formDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="custom-field-name">{t("customFields.name")}</Label>
            <Input
              disabled={!!field || pending}
              id="custom-field-name"
              onChange={(event) => setName(event.target.value)}
              placeholder="estimate"
              value={name}
            />
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label>{t("customFields.type")}</Label>
              <Select
                disabled={pending}
                onValueChange={(value) => setType(value as CustomFieldType)}
                value={type}
              >
                <SelectTrigger aria-label={t("customFields.type")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(["string", "numeric", "date", "duration"] as const).map(
                    (value) => (
                      <SelectItem key={value} value={value}>
                        {t(`customFields.types.${value}`)}
                      </SelectItem>
                    )
                  )}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="custom-field-label">
                {t("customFields.label")}
              </Label>
              <Input
                disabled={pending}
                id="custom-field-label"
                onChange={(event) => setLabel(event.target.value)}
                value={label}
              />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="custom-field-values">
              {t("customFields.values")}
            </Label>
            <Input
              disabled={pending}
              id="custom-field-values"
              onChange={(event) => setValues(event.target.value)}
              placeholder={t("customFields.valuesPlaceholder")}
              value={values}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="custom-field-default">
              {t("customFields.default")}
            </Label>
            <Input
              disabled={pending}
              id="custom-field-default"
              onChange={(event) => setDefaultValue(event.target.value)}
              value={defaultValue}
            />
            <p className="text-xs text-muted-foreground">
              {t("customFields.defaultHint")}
            </p>
          </div>
          {error ? (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              disabled={pending}
              onClick={() => onOpenChange(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button disabled={pending || !name.trim()} type="submit">
              {pending ? t("common.loading") : t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
