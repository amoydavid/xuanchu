import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

import { ScopeEditor } from "./scope-editor"
import {
  expiresSecondsToPreset,
  presetToExpiresSeconds,
  sameSet,
  type ExpiresPreset,
  type TokenFormValues,
  type TokenRow,
} from "./token-api"

type WorkspaceItem = { id: string; slug: string; name: string }
type WorkspaceListResponse = WorkspaceItem[]

type TokenFormProps = {
  mode: "create" | "edit"
  initial?: TokenRow
  onSubmit: (values: TokenFormValues) => void
  submitting?: boolean
  canImpersonate?: boolean
}

function defaultValues(mode: "create" | "edit", initial?: TokenRow): TokenFormValues {
  if (mode === "edit" && initial) {
    const { preset, customIso } = expiresSecondsToPreset(initial.expires_at)
    return {
      name: initial.name,
      type: initial.type,
      workspaces: [...(initial.workspace_ids ?? [])],
      scopes: [...(initial.scopes ?? [])],
      projects: [...(initial.project_ids ?? [])],
      expiresPreset: preset,
      expiresAt: customIso,
    }
  }
  return {
    name: "",
    type: "pat",
    workspaces: [],
    scopes: [],
    projects: [],
    expiresPreset: "never",
    expiresAt: "",
  }
}

export function TokenForm({
  mode,
  initial,
  onSubmit,
  submitting = false,
  canImpersonate = false,
}: TokenFormProps) {
  const { t } = useTranslation()
  const [values, setValues] = useState<TokenFormValues>(() =>
    defaultValues(mode, initial)
  )
  const [formError, setFormError] = useState<string | null>(null)

  const workspacesQuery = useQuery({
    queryKey: ["workspaces-for-token-form"],
    queryFn: () => workspaceApiGet<WorkspaceListResponse>("/api/v1/workspaces"),
  })
  const workspaces: WorkspaceItem[] = workspacesQuery.data ?? []

  const update = <K extends keyof TokenFormValues>(
    key: K,
    next: TokenFormValues[K]
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
    if (values.type === "agent" && values.workspaces.length === 0) {
      setFormError(t("token.agentRequiresWorkspace"))
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
          placeholder="ci-deploy"
          value={values.name}
        />
      </Field>

      <Field
        label={t("token.field.type")}
        hint={mode === "edit" ? t("token.typeLocked") : undefined}
      >
        <Select
          disabled={mode === "edit"}
          onValueChange={(v) => update("type", v)}
          value={values.type}
        >
          <SelectTrigger aria-label={t("token.field.type")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="pat">PAT</SelectItem>
            <SelectItem value="agent">Agent</SelectItem>
          </SelectContent>
        </Select>
      </Field>

      <Field label={t("token.field.workspaces")}>
        <WorkspacePicker
          loading={workspacesQuery.isLoading}
          onChange={(refs) => update("workspaces", refs)}
          options={workspaces}
          selected={values.workspaces}
        />
      </Field>

      <Field label={t("token.field.scopes")}>
        <ScopeEditor
          canImpersonate={canImpersonate}
          onChange={(scopes) => update("scopes", scopes)}
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
            <SelectItem value="never">{t("token.expiresPreset.never")}</SelectItem>
            <SelectItem value="7d">{t("token.expiresPreset.7d")}</SelectItem>
            <SelectItem value="30d">{t("token.expiresPreset.30d")}</SelectItem>
            <SelectItem value="90d">{t("token.expiresPreset.90d")}</SelectItem>
            <SelectItem value="custom">{t("token.expiresPreset.custom")}</SelectItem>
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
        <ProjectPicker
          onChange={(refs) => update("projects", refs)}
          selected={values.projects}
        />
      </Field>

      {formError ? (
        <p className="text-sm text-destructive">{formError}</p>
      ) : null}

      <div className="flex justify-end gap-2 pt-2">
        <button
          className="text-sm text-muted-foreground underline-offset-2 hover:underline"
          disabled={submitting}
          type="submit"
        >
          {submitting ? "..." : mode === "create" ? t("token.create") : t("token.save")}
        </button>
      </div>
    </form>
  )
}

function Field({
  children,
  hint,
  label,
}: {
  children: React.ReactNode
  hint?: string
  label: string
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
      {hint ? (
        <p className="text-xs text-muted-foreground">{hint}</p>
      ) : null}
    </div>
  )
}

function WorkspacePicker({
  loading,
  onChange,
  options,
  selected,
}: {
  loading?: boolean
  onChange: (refs: string[]) => void
  options: WorkspaceItem[]
  selected: string[]
}) {
  const { t } = useTranslation()
  if (loading) {
    return <p className="text-xs text-muted-foreground">...</p>
  }
  if (options.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("common.empty")}</p>
  }
  // 把 selected 里的 workspace ID 归一化为 slug，保证编辑预填（ID）与
  // toggle 输出（slug）一致，避免混用导致 sameSet 误判。
  const idToSlug = new Map(options.map((ws) => [ws.id, ws.slug]))
  const normalized = selected.map((ref) => idToSlug.get(ref) ?? ref)
  const selectedSet = new Set(normalized)
  const toggle = (slug: string) => {
    const next = selectedSet.has(slug)
      ? normalized.filter((ref) => ref !== slug)
      : [...normalized, slug]
    onChange(next)
  }
  return (
    <div className="flex flex-wrap gap-3 rounded-none border p-2">
      {options.map((ws) => {
        const isSelected = selectedSet.has(ws.slug)
        return (
          <label
            className="flex cursor-pointer items-center gap-1.5 text-sm"
            key={ws.id}
          >
            <input
              checked={isSelected}
              onChange={() => toggle(ws.slug)}
              type="checkbox"
            />
            <span>{ws.name || ws.slug}</span>
          </label>
        )
      })}
    </div>
  )
}

function ProjectPicker({
  selected,
  onChange,
}: {
  selected: string[]
  onChange: (refs: string[]) => void
}) {
  const { t } = useTranslation()
  // 项目范围依赖工作空间；用逗号分隔输入，离开/输入时同步为 refs 数组。
  return (
    <Input
      aria-label={t("token.field.projects")}
      onChange={(e) => {
        const refs = e.target.value
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean)
        onChange(refs)
      }}
      placeholder="project-slug-a, project-slug-b"
      value={selected.join(", ")}
    />
  )
}

/** 把表单值转为创建请求体。 */
export function valuesToCreateInput(values: TokenFormValues) {
  const expires = presetToExpiresSeconds(values.expiresPreset, values.expiresAt)
  return {
    name: values.name.trim(),
    type: values.type,
    scopes: values.scopes,
    workspaces: values.workspaces,
    projects: values.projects,
    expires_in_seconds: expires,
  }
}

/** 把表单值转为修改请求体（仅含实际变动的字段）。 */
export function valuesToModifyInput(
  values: TokenFormValues,
  initial?: TokenRow
) {
  const input: {
    name?: string
    scopes?: string[]
    workspaces?: string[]
    projects?: string[]
    expires_in_seconds?: number | null
  } = {}
  if (!initial || values.name.trim() !== initial.name) {
    input.name = values.name.trim()
  }
  if (!initial || !sameSet(values.scopes, initial.scopes)) {
    input.scopes = values.scopes
  }
  if (!initial || !sameSet(values.workspaces, initial.workspace_ids)) {
    input.workspaces = values.workspaces
  }
  if (!initial || !sameSet(values.projects, initial.project_ids)) {
    input.projects = values.projects
  }
  const newExpires = presetToExpiresSeconds(values.expiresPreset, values.expiresAt)
  const oldExpires = initial?.expires_at ?? null
  // null = 永不过期；比较是否一致
  if ((newExpires ?? null) !== (oldExpires ?? null)) {
    input.expires_in_seconds = newExpires
  }
  return input
}
