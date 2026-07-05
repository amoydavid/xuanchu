import type { FormEvent } from "react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription } from "@/components/ui/alert"
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
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api"

import {
  createNotificationSink,
  modifyNotificationSink,
  type HTTPHeaderTemplate,
  type HTTPTemplateSecretRef,
  type NotificationSink,
  type NotificationSinkCreateInput,
  type NotificationSinkModifyInput,
} from "../outbound-api"

type EndpointMode = "static_url" | "template" | "config_value"
type SinkType = "webhook" | "http_template"

type FormState = {
  name: string
  type: SinkType
  endpoint_mode: EndpointMode
  url: string
  url_template: string
  config_key: string
  allowed_hosts: string
  secret: string
  timeout_seconds: number
  max_attempts: number
  max_concurrency: number
  body_template: string
  body_content_type: string
  header_templates: HTTPHeaderTemplate[]
  secret_refs: HTTPTemplateSecretRef[]
}

function emptyForm(): FormState {
  return {
    name: "",
    type: "webhook",
    endpoint_mode: "static_url",
    url: "",
    url_template: "",
    config_key: "",
    allowed_hosts: "",
    secret: "",
    timeout_seconds: 10,
    max_attempts: 3,
    max_concurrency: 0,
    body_template: "",
    body_content_type: "application/json",
    header_templates: [],
    secret_refs: [],
  }
}

function fromSink(sink: NotificationSink): FormState {
  return {
    name: sink.name ?? "",
    type: (sink.type as SinkType) ?? "webhook",
    endpoint_mode: (sink.endpoint_mode as EndpointMode) ?? "static_url",
    url: sink.url ?? "",
    url_template: sink.url_template ?? "",
    config_key: sink.config_key ?? "",
    allowed_hosts: (sink.allowed_hosts ?? []).join(", "),
    secret: "",
    timeout_seconds: sink.timeout_seconds ?? 10,
    max_attempts: sink.max_attempts ?? 3,
    max_concurrency: sink.max_concurrency ?? 0,
    body_template: sink.body_template ?? "",
    body_content_type: sink.body_content_type ?? "application/json",
    header_templates: sink.header_templates ?? [],
    secret_refs: sink.secret_refs ?? [],
  }
}

function parseList(value: string): string[] {
  return value
    .split(/[,\n]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

type SinkFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: () => void
  initial?: NotificationSink
}

export function SinkFormDialog({
  initial,
  onOpenChange,
  onSaved,
  open,
}: SinkFormDialogProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState<FormState>(() =>
    initial ? fromSink(initial) : emptyForm()
  )
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // 当 initial 变化或弹窗打开时，重置 form。
  useStateSync(open, () => {
    setForm(initial ? fromSink(initial) : emptyForm())
    setError(null)
  })

  function patch(partial: Partial<FormState>) {
    setForm((prev) => ({ ...prev, ...partial }))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)

    if (!form.name.trim()) {
      setError(`${t("outbound.sinkNameLabel")}: required`)
      return
    }
    if (form.endpoint_mode === "static_url" && !form.url.trim()) {
      setError(`${t("outbound.sinkUrlLabel")}: required`)
      return
    }
    if (form.endpoint_mode === "template" && !form.url_template.trim()) {
      setError(`${t("outbound.sinkUrlTemplateLabel")}: required`)
      return
    }
    if (form.endpoint_mode === "config_value" && !form.config_key.trim()) {
      setError(`${t("outbound.sinkConfigKeyLabel")}: required`)
      return
    }
    const dynamicMode =
      form.endpoint_mode === "template" || form.endpoint_mode === "config_value"
    const hosts = parseList(form.allowed_hosts)
    if (dynamicMode && hosts.length === 0) {
      setError(`${t("outbound.sinkAllowedHostsLabel")}: required`)
      return
    }

    setSubmitting(true)
    try {
      if (initial) {
        await modifyNotificationSink(initial.id, buildModifyPayload(form))
      } else {
        await createNotificationSink(buildCreatePayload(form))
      }
      onSaved()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {initial ? t("outbound.editSink") : t("outbound.createSink")}
          </DialogTitle>
          <DialogDescription>{t("outbound.sinkHint")}</DialogDescription>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <Field label={t("outbound.sinkNameLabel")}>
            <Input
              aria-label={t("outbound.sinkNameLabel")}
              onChange={(e) => patch({ name: e.target.value })}
              value={form.name}
            />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field label={t("outbound.sinkTypeLabel")}>
              <select
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) => patch({ type: e.target.value as SinkType })}
                value={form.type}
              >
                <option value="webhook">webhook</option>
                <option value="http_template">http_template</option>
              </select>
            </Field>
            <Field label={t("outbound.sinkEndpointModeLabel")}>
              <select
                className="h-9 w-full rounded-md border bg-transparent px-2 text-sm"
                onChange={(e) =>
                  patch({ endpoint_mode: e.target.value as EndpointMode })
                }
                value={form.endpoint_mode}
              >
                <option value="static_url">static_url</option>
                <option value="template">template</option>
                <option value="config_value">config_value</option>
              </select>
            </Field>
          </div>

          {form.endpoint_mode === "static_url" ? (
            <Field label={t("outbound.sinkUrlLabel")}>
              <Input
                aria-label={t("outbound.sinkUrlLabel")}
                onChange={(e) => patch({ url: e.target.value })}
                placeholder="https://example.com/webhook"
                value={form.url}
              />
            </Field>
          ) : null}
          {form.endpoint_mode === "template" ? (
            <Field label={t("outbound.sinkUrlTemplateLabel")}>
              <Input
                onChange={(e) => patch({ url_template: e.target.value })}
                value={form.url_template}
              />
            </Field>
          ) : null}
          {form.endpoint_mode === "config_value" ? (
            <Field label={t("outbound.sinkConfigKeyLabel")}>
              <Input
                onChange={(e) => patch({ config_key: e.target.value })}
                value={form.config_key}
              />
            </Field>
          ) : null}
          {form.endpoint_mode !== "static_url" ? (
            <Field label={t("outbound.sinkAllowedHostsLabel")}>
              <Input
                onChange={(e) => patch({ allowed_hosts: e.target.value })}
                placeholder="example.com, hooks.example.com"
                value={form.allowed_hosts}
              />
            </Field>
          ) : null}

          {form.type === "webhook" ? (
            <Field label={t("outbound.sinkSecretLabel")}>
              {initial ? (
                <p className="text-xs text-muted-foreground">
                  {t("outbound.sinkSecretConfigured")}
                </p>
              ) : null}
              <Input
                autoComplete="new-password"
                onChange={(e) => patch({ secret: e.target.value })}
                placeholder={initial ? "••••••" : "secret"}
                type="password"
                value={form.secret}
              />
            </Field>
          ) : null}

          {form.type === "http_template" ? (
            <>
              <Field label={t("outbound.sinkHeaderTemplatesLabel")}>
                <RepeatableRows
                  rows={form.header_templates}
                  onChange={(rows) => patch({ header_templates: rows })}
                  nameLabel="name"
                  valueLabel="value"
                />
              </Field>
              <Field label={t("outbound.sinkBodyContentTypeLabel")}>
                <Input
                  onChange={(e) => patch({ body_content_type: e.target.value })}
                  value={form.body_content_type}
                />
              </Field>
              <Field label={t("outbound.sinkBodyTemplateLabel")}>
                <Textarea
                  className="min-h-[120px] font-mono text-xs"
                  onChange={(e) => patch({ body_template: e.target.value })}
                  value={form.body_template}
                />
              </Field>
              <Field label={t("outbound.sinkSecretRefsLabel")}>
                <RepeatableSecretRefs
                  rows={form.secret_refs}
                  onChange={(rows) => patch({ secret_refs: rows })}
                />
              </Field>
            </>
          ) : null}

          <div className="grid grid-cols-3 gap-3">
            <Field label={t("outbound.sinkTimeoutLabel")}>
              <Input
                min={1}
                onChange={(e) =>
                  patch({ timeout_seconds: Number(e.target.value) || 0 })
                }
                type="number"
                value={form.timeout_seconds}
              />
            </Field>
            <Field label={t("outbound.sinkMaxAttemptsLabel")}>
              <Input
                min={1}
                onChange={(e) =>
                  patch({ max_attempts: Number(e.target.value) || 0 })
                }
                type="number"
                value={form.max_attempts}
              />
            </Field>
            <Field label={t("outbound.sinkMaxConcurrencyLabel")}>
              <Input
                min={0}
                onChange={(e) =>
                  patch({ max_concurrency: Number(e.target.value) || 0 })
                }
                type="number"
                value={form.max_concurrency}
              />
            </Field>
          </div>

          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              onClick={() => onOpenChange(false)}
              type="button"
              variant="outline"
            >
              {t("common.cancel")}
            </Button>
            <Button disabled={submitting} type="submit">
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
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
    <div className="space-y-1">
      <Label className="text-xs">{label}</Label>
      {children}
    </div>
  )
}

function RepeatableRows({
  rows,
  onChange,
  nameLabel,
  valueLabel,
}: {
  rows: HTTPHeaderTemplate[]
  onChange: (rows: HTTPHeaderTemplate[]) => void
  nameLabel: string
  valueLabel: string
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-2">
      {rows.map((row, idx) => (
        <div className="flex gap-2" key={idx}>
          <Input
            onChange={(e) => {
              const next = [...rows]
              next[idx] = { ...next[idx], name: e.target.value }
              onChange(next)
            }}
            placeholder={nameLabel}
            value={row.name}
          />
          <Input
            onChange={(e) => {
              const next = [...rows]
              next[idx] = { ...next[idx], value: e.target.value }
              onChange(next)
            }}
            placeholder={valueLabel}
            value={row.value}
          />
          <Button
            onClick={() => onChange(rows.filter((_, i) => i !== idx))}
            type="button"
            variant="outline"
          >
            ×
          </Button>
        </div>
      ))}
      <Button
        onClick={() => onChange([...rows, { name: "", value: "" }])}
        size="sm"
        type="button"
        variant="outline"
      >
        {t("outbound.addRow")}
      </Button>
    </div>
  )
}

function RepeatableSecretRefs({
  rows,
  onChange,
}: {
  rows: HTTPTemplateSecretRef[]
  onChange: (rows: HTTPTemplateSecretRef[]) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-2">
      {rows.map((row, idx) => (
        <div className="flex gap-2" key={idx}>
          <Input
            onChange={(e) => {
              const next = [...rows]
              next[idx] = { ...next[idx], alias: e.target.value }
              onChange(next)
            }}
            placeholder={t("outbound.secretRefAlias")}
            value={row.alias}
          />
          <Input
            onChange={(e) => {
              const next = [...rows]
              next[idx] = { ...next[idx], config_key: e.target.value }
              onChange(next)
            }}
            placeholder={t("outbound.secretRefConfigKey")}
            value={row.config_key}
          />
          <Button
            onClick={() => onChange(rows.filter((_, i) => i !== idx))}
            type="button"
            variant="outline"
          >
            ×
          </Button>
        </div>
      ))}
      <Button
        onClick={() => onChange([...rows, { alias: "", config_key: "" }])}
        size="sm"
        type="button"
        variant="outline"
      >
        {t("outbound.addRow")}
      </Button>
    </div>
  )
}

function buildCreatePayload(form: FormState): NotificationSinkCreateInput {
  const payload: NotificationSinkCreateInput = {
    name: form.name.trim(),
    type: form.type,
    endpoint_mode: form.endpoint_mode,
    timeout_seconds: form.timeout_seconds,
    max_attempts: form.max_attempts,
    max_concurrency: form.max_concurrency,
  }
  if (form.endpoint_mode === "static_url") payload.url = form.url.trim()
  if (form.endpoint_mode === "template")
    payload.url_template = form.url_template.trim()
  if (form.endpoint_mode === "config_value")
    payload.config_key = form.config_key.trim()
  if (form.endpoint_mode !== "static_url")
    payload.allowed_hosts = parseList(form.allowed_hosts)
  if (form.type === "webhook" && form.secret.trim()) {
    payload.secret = form.secret.trim()
  }
  if (form.type === "http_template") {
    payload.header_templates = form.header_templates.filter(
      (r) => r.name && r.value
    )
    payload.body_template = form.body_template
    payload.body_content_type = form.body_content_type
    payload.secret_refs = form.secret_refs.filter(
      (r) => r.alias && r.config_key
    )
  }
  return payload
}

function buildModifyPayload(form: FormState): NotificationSinkModifyInput {
  const payload: NotificationSinkModifyInput = buildCreatePayload(form)
  if (form.type === "webhook" && !form.secret.trim()) {
    // 编辑时不提交空 secret，避免覆盖已有 secret。
    delete payload.secret
  }
  return payload
}

// useStateSync 在 open 由 false 变 true 时调用 effect 重置 form。
function useStateSync(open: boolean, fn: () => void) {
  useEffect(() => {
    if (open) fn()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])
}
