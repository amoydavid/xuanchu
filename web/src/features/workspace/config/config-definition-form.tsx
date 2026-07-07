import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api"
import {
  type ConfigAllowedScope,
  type ConfigSchemaDefinition,
  type ConfigSchemaInput,
  type ConfigSchemaUsage,
  type ConfigValueType,
} from "./config-definition-api"
import { ConfigValueControl } from "./config-value-control"

type ConfigDefinitionFormProps = {
  mode: "create" | "edit"
  initial?: ConfigSchemaDefinition
  usage?: ConfigSchemaUsage
  defaultScopes: ConfigAllowedScope[]
  canManage: boolean
  onSubmit: (key: string, input: ConfigSchemaInput) => Promise<void>
  onDelete?: (key: string) => Promise<void>
}

const VALUE_TYPES: ConfigValueType[] = ["string", "number", "boolean", "json"]

function emptyInput(defaultScopes: ConfigAllowedScope[]): ConfigSchemaInput {
  return {
    value_type: "string",
    allowed_scopes: [...defaultScopes],
    label: "",
    description: "",
    enum_values: [],
    default_value: null,
    required: false,
    secret: false,
    show_on_console_home: false,
  }
}

function inputFromDefinition(def: ConfigSchemaDefinition): ConfigSchemaInput {
  return {
    value_type: def.value_type as ConfigValueType,
    allowed_scopes: def.allowed_scopes as ConfigAllowedScope[],
    label: def.label,
    description: def.description,
    enum_values: def.enum_values,
    default_value: def.default_value,
    required: def.required,
    secret: def.secret,
    show_on_console_home: def.show_on_console_home,
  }
}

export function ConfigDefinitionForm({
  mode,
  initial,
  usage,
  defaultScopes,
  canManage,
  onSubmit,
  onDelete,
}: ConfigDefinitionFormProps) {
  const { t } = useTranslation()
  const editingKey = initial?.key ?? ""
  const [key, setKey] = useState(editingKey)
  const [draft, setDraft] = useState<ConfigSchemaInput>(
    initial ? inputFromDefinition(initial) : emptyInput(defaultScopes)
  )
  const [enumText, setEnumText] = useState(
    initial && initial.enum_values.length > 0
      ? initial.enum_values.join("\n")
      : ""
  )
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteConfirm, setDeleteConfirm] = useState("")
  const [deleting, setDeleting] = useState(false)

  const totalUsage = usage?.total_values ?? 0
  const typeLocked = mode === "edit" && totalUsage > 0
  const workspaceScopeLocked =
    mode === "edit" && (usage?.workspace_values ?? 0) > 0
  const projectScopeLocked =
    mode === "edit" && (usage?.project_values ?? 0) > 0

  const hasScope = draft.allowed_scopes.length > 0
  const canSubmit =
    canManage &&
    !submitting &&
    (mode === "edit" || key.trim() !== "") &&
    hasScope

  const patch = (partial: Partial<ConfigSchemaInput>) =>
    setDraft((prev) => ({ ...prev, ...partial }))

  const toggleScope = (scope: ConfigAllowedScope, checked: boolean) => {
    setDraft((prev) => {
      const set = new Set(prev.allowed_scopes as ConfigAllowedScope[])
      if (checked) set.add(scope)
      else set.delete(scope)
      return { ...prev, allowed_scopes: Array.from(set) }
    })
  }

  const submit = async () => {
    if (!canSubmit) return
    const resolvedKey = (mode === "edit" ? editingKey : key.trim())
    if (!resolvedKey) return
    const enumValues = enumText
      .split("\n")
      .map((s) => s.trim())
      .filter((s) => s !== "")
    const input: ConfigSchemaInput = {
      ...draft,
      enum_values: enumValues,
    }
    setSubmitting(true)
    setSubmitError(null)
    try {
      await onSubmit(resolvedKey, input)
    } catch (err) {
      setSubmitError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setSubmitting(false)
    }
  }

  const confirmDelete = async () => {
    if (!onDelete || deleteConfirm !== editingKey || deleting) return
    setDeleting(true)
    try {
      await onDelete(editingKey)
      setDeleteOpen(false)
    } catch (err) {
      setSubmitError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="space-y-4">
      <Field label={t("configDefinitions.key")}>
        <Input
          disabled={mode === "edit" || !canManage}
          aria-label={t("configDefinitions.key")}
          onChange={(e) => setKey(e.target.value)}
          value={key}
        />
      </Field>

      <Field label={t("configDefinitions.type")}>
        <Select
          disabled={typeLocked || !canManage}
          onValueChange={(v) => patch({ value_type: v as ConfigValueType })}
          value={draft.value_type}
        >
          <SelectTrigger aria-label={t("configDefinitions.type")} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {VALUE_TYPES.map((vt) => (
              <SelectItem key={vt} value={vt}>
                {vt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {typeLocked ? (
          <p className="text-xs text-muted-foreground">
            {t("configDefinitions.typeLocked")}
          </p>
        ) : null}
      </Field>

      <Field label={t("configDefinitions.scopes")}>
        <div className="space-y-2">
          <ScopeCheckbox
            label={t("configDefinitions.scopeWorkspace")}
            checked={draft.allowed_scopes.includes("workspace")}
            disabled={!canManage || workspaceScopeLocked}
            locked={workspaceScopeLocked}
            lockedHint={t("configDefinitions.scopeLocked")}
            onChange={(c) => toggleScope("workspace", c)}
          />
          <ScopeCheckbox
            label={t("configDefinitions.scopeProject")}
            checked={draft.allowed_scopes.includes("project")}
            disabled={!canManage || projectScopeLocked}
            locked={projectScopeLocked}
            lockedHint={t("configDefinitions.scopeLocked")}
            onChange={(c) => toggleScope("project", c)}
          />
        </div>
      </Field>

      <Field label={t("configDefinitions.label")}>
        <Input
          aria-label={t("configDefinitions.label")}
          disabled={!canManage}
          onChange={(e) => patch({ label: e.target.value })}
          value={draft.label}
        />
      </Field>

      <Field label={t("configDefinitions.descriptionField")}>
        <Textarea
          aria-label={t("configDefinitions.descriptionField")}
          disabled={!canManage}
          onChange={(e) => patch({ description: e.target.value })}
          rows={2}
          value={draft.description}
        />
      </Field>

      <Field label={t("configDefinitions.enumValues")}>
        <Textarea
          aria-label={t("configDefinitions.enumValues")}
          disabled={!canManage}
          onChange={(e) => setEnumText(e.target.value)}
          rows={3}
          value={enumText}
        />
      </Field>

      <Field label={t("configDefinitions.defaultValue")}>
        <ConfigValueControl
          definition={{
            value_type: draft.value_type,
            enum_values: draft.enum_values,
            secret: draft.secret,
          }}
          onChange={(v) => patch({ default_value: v })}
          disabled={!canManage}
          value={draft.default_value ?? ""}
        />
      </Field>

      <div className="space-y-2">
        <CheckRow
          label={t("configDefinitions.required")}
          checked={draft.required}
          disabled={!canManage}
          onChange={(c) => patch({ required: c })}
        />
        <CheckRow
          label={t("configDefinitions.secret")}
          checked={draft.secret}
          disabled={!canManage}
          onChange={(c) => patch({ secret: c })}
        />
        <CheckRow
          label={t("configDefinitions.showOnHome")}
          checked={draft.show_on_console_home}
          disabled={!canManage}
          onChange={(c) => patch({ show_on_console_home: c })}
          help={t("configDefinitions.showOnHomeHelp")}
        />
      </div>

      {submitError ? (
        <p className="text-xs text-destructive">{submitError}</p>
      ) : null}

      {canManage ? (
        <div className="flex gap-2">
          <Button
            disabled={!canSubmit}
            onClick={submit}
            size="sm"
            type="button"
          >
            {t("configDefinitions.save")}
          </Button>
          {mode === "edit" && onDelete ? (
            <Button
              onClick={() => {
                setDeleteConfirm("")
                setDeleteOpen(true)
              }}
              size="sm"
              type="button"
              variant="destructive"
            >
              {t("configDefinitions.delete")}
            </Button>
          ) : null}
        </div>
      ) : null}

      {mode === "edit" && onDelete ? (
        <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {t("configDefinitions.deleteTitle")}
              </AlertDialogTitle>
              <AlertDialogDescription asChild>
                <div className="space-y-2">
                  <p>
                    {t("configDefinitions.deleteConfirmPrompt", {
                      key: editingKey,
                    })}
                  </p>
                  <p>
                    {t("configDefinitions.deleteConfirmCounts", {
                      workspace: usage?.workspace_values ?? 0,
                      project: usage?.project_values ?? 0,
                    })}
                  </p>
                  <p>{t("configDefinitions.deleteConfirmTypeHint")}</p>
                  <Input
                    aria-label={t("configDefinitions.deleteConfirmInput")}
                    onChange={(e) => setDeleteConfirm(e.target.value)}
                    placeholder={t("configDefinitions.deleteConfirmInput")}
                    value={deleteConfirm}
                  />
                </div>
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("configDefinitions.cancel")}</AlertDialogCancel>
              <AlertDialogAction
                disabled={deleteConfirm !== editingKey || deleting}
                onClick={confirmDelete}
              >
                {t("configDefinitions.deleteConfirmAction")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      ) : null}
    </div>
  )
}

function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1">
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function ScopeCheckbox({
  label,
  checked,
  disabled,
  locked,
  lockedHint,
  onChange,
}: {
  label: string
  checked: boolean
  disabled: boolean
  locked: boolean
  lockedHint: string
  onChange: (checked: boolean) => void
}) {
  const id = useMemo(() => `scope-${label}`, [label])
  return (
    <div className="flex items-center gap-2">
      <Checkbox
        checked={checked}
        disabled={disabled}
        id={id}
        onCheckedChange={(v) => onChange(v === true)}
      />
      <Label htmlFor={id} className="text-sm font-normal">
        {label}
      </Label>
      {locked ? (
        <span className="text-xs text-muted-foreground">{lockedHint}</span>
      ) : null}
    </div>
  )
}

function CheckRow({
  label,
  checked,
  disabled,
  onChange,
  help,
}: {
  label: string
  checked: boolean
  disabled: boolean
  onChange: (checked: boolean) => void
  help?: string
}) {
  const id = useMemo(() => `row-${label}`, [label])
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <Checkbox
          aria-label={label}
          checked={checked}
          disabled={disabled}
          id={id}
          onCheckedChange={(v) => onChange(v === true)}
        />
        <Label htmlFor={id} className="text-sm font-normal">
          {label}
        </Label>
      </div>
      {help ? <p className="text-xs text-muted-foreground">{help}</p> : null}
    </div>
  )
}
